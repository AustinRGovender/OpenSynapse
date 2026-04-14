package engine

import (
	"bufio"
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// MetricSnapshot holds aggregated metrics for a 1-second tick.
type MetricSnapshot struct {
	TimestampMS int64   `json:"timestamp_ms"`
	RPS         float64 `json:"rps"`
	P95MS       float64 `json:"p95_ms"`
	ErrorRate   float64 `json:"error_rate"`
	ActiveVUs   int     `json:"active_vus"`
	BytesSent   int64   `json:"bytes_sent"`
	BytesRecv   int64   `json:"bytes_received"`
}

// MetricsCallback is called every second with aggregated metrics.
type MetricsCallback func(runID string, snapshot MetricSnapshot)

// CompletionCallback is called when the k6 process exits.
type CompletionCallback func(runID string, exitCode int, summary *db.RunSummary)

// Engine manages k6 subprocess execution.
type Engine struct {
	k6Path   string
	mu       sync.Mutex
	active   map[string]*runProcess
}

type runProcess struct {
	cmd    *exec.Cmd
	cancel context.CancelFunc
	runID  string
}

// New creates a new engine. It auto-discovers k6 from PATH.
func New() (*Engine, error) {
	k6Path, err := exec.LookPath("k6")
	if err != nil {
		// Try common locations
		for _, p := range []string{"k6", "k6.exe", "/usr/local/bin/k6"} {
			if _, err := os.Stat(p); err == nil {
				k6Path = p
				break
			}
		}
		if k6Path == "" {
			return nil, fmt.Errorf("k6 binary not found in PATH")
		}
	}

	return &Engine{
		k6Path: k6Path,
		active: make(map[string]*runProcess),
	}, nil
}

// StartRun compiles the plan to a k6 script and runs it as a subprocess.
func (e *Engine) StartRun(runID string, script string, params db.RunParameters, onMetrics MetricsCallback, onComplete CompletionCallback) error {
	// Write script to temp file
	tmpDir := os.TempDir()
	scriptPath := filepath.Join(tmpDir, fmt.Sprintf("opensynapse-%s.js", runID))
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		return fmt.Errorf("write script: %w", err)
	}

	ctx, cancel := context.WithCancel(context.Background())

	args := []string{
		"run",
		"--no-color",
		"--quiet",
		scriptPath,
	}

	// For non-externally-controlled executors, pass VU/duration overrides
	if params.VUsTarget > 0 {
		args = append(args, fmt.Sprintf("--vus=%d", params.VUsTarget))
	}
	if params.DurationSeconds > 0 {
		args = append(args, fmt.Sprintf("--duration=%ds", params.DurationSeconds))
	}

	cmd := exec.CommandContext(ctx, e.k6Path, args...)
	cmd.Env = append(os.Environ(),
		fmt.Sprintf("RPS_TARGET=%d", params.RPSTarget),
		"SHOULD_STOP=false",
	)

	// Capture stdout for summary parsing
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stdout pipe: %w", err)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		cancel()
		return fmt.Errorf("stderr pipe: %w", err)
	}

	if err := cmd.Start(); err != nil {
		cancel()
		os.Remove(scriptPath)
		return fmt.Errorf("start k6: %w", err)
	}

	rp := &runProcess{cmd: cmd, cancel: cancel, runID: runID}
	e.mu.Lock()
	e.active[runID] = rp
	e.mu.Unlock()

	// Background goroutine: parse k6 output for metrics
	go func() {
		defer os.Remove(scriptPath)
		defer func() {
			e.mu.Lock()
			delete(e.active, runID)
			e.mu.Unlock()
		}()

		// Collect stderr for error logging
		var stderrLines []string
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				stderrLines = append(stderrLines, scanner.Text())
			}
		}()

		// Parse stdout for k6 summary output
		var outputLines []string
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			line := scanner.Text()
			outputLines = append(outputLines, line)
		}

		exitCode := 0
		if err := cmd.Wait(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}

		// Parse the k6 summary from output
		summary := parseK6Summary(outputLines)

		if onComplete != nil {
			onComplete(runID, exitCode, summary)
		}
	}()

	// Background goroutine: emit synthetic metrics every second while running
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		startTime := time.Now()
		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				elapsed := time.Since(startTime).Seconds()
				// While k6 is running, emit placeholder metrics
				// (real metrics come from Prometheus remote-write in production;
				//  for now we emit synthetic ones based on elapsed time)
				snapshot := MetricSnapshot{
					TimestampMS: time.Now().UnixMilli(),
					ActiveVUs:   params.VUsTarget,
					RPS:         0,
					P95MS:       0,
					ErrorRate:   0,
				}
				_ = elapsed // used for synthetic generation later
				if onMetrics != nil {
					onMetrics(runID, snapshot)
				}
			}
		}
	}()

	return nil
}

// StopRun gracefully stops a running k6 process.
func (e *Engine) StopRun(runID string) error {
	e.mu.Lock()
	rp, ok := e.active[runID]
	e.mu.Unlock()
	if !ok {
		return fmt.Errorf("run %s not active", runID)
	}

	// Send interrupt for graceful shutdown
	if rp.cmd.Process != nil {
		rp.cmd.Process.Signal(os.Interrupt)
		// Give it 10 seconds then force kill
		go func() {
			time.Sleep(10 * time.Second)
			rp.cancel()
		}()
	}
	return nil
}

// KillRun forcefully kills a running k6 process.
func (e *Engine) KillRun(runID string) error {
	e.mu.Lock()
	rp, ok := e.active[runID]
	e.mu.Unlock()
	if !ok {
		return fmt.Errorf("run %s not active", runID)
	}
	rp.cancel()
	return nil
}

// IsRunning checks if a run is currently active.
func (e *Engine) IsRunning(runID string) bool {
	e.mu.Lock()
	defer e.mu.Unlock()
	_, ok := e.active[runID]
	return ok
}

// parseK6Summary attempts to extract summary stats from k6 stdout output.
func parseK6Summary(lines []string) *db.RunSummary {
	summary := &db.RunSummary{ThresholdsPassed: true}
	output := strings.Join(lines, "\n")

	// Parse common k6 summary patterns
	patterns := map[string]*float64{
		`http_reqs[.\s]+(\d+)`:                   nil, // special: int
		`http_req_duration.*p\(95\)=([0-9.]+)ms`: &summary.P95MS,
		`http_req_duration.*p\(90\)=([0-9.]+)ms`: &summary.P90MS,
		`http_req_duration.*p\(50\)=([0-9.]+)ms`: &summary.P50MS,
		`http_req_duration.*p\(99\)=([0-9.]+)ms`: &summary.P99MS,
		`http_req_duration.*max=([0-9.]+)ms`:      &summary.MaxMS,
	}

	for pattern, target := range patterns {
		re := regexp.MustCompile(pattern)
		if matches := re.FindStringSubmatch(output); len(matches) > 1 {
			if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
				if target != nil {
					*target = val
				} else {
					summary.TotalRequests = int64(val)
				}
			}
		}
	}

	// Parse http_req_failed
	failedRe := regexp.MustCompile(`http_req_failed[.\s]+(\d+\.\d+)%`)
	if matches := failedRe.FindStringSubmatch(output); len(matches) > 1 {
		if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
			summary.ErrorRate = val / 100.0
			summary.FailedRequests = int64(float64(summary.TotalRequests) * summary.ErrorRate)
		}
	}

	// Compute throughput (rough: reqs / duration inferred from output)
	if summary.TotalRequests > 0 {
		rpsRe := regexp.MustCompile(`http_reqs[.\s]+\d+\s+([0-9.]+)/s`)
		if matches := rpsRe.FindStringSubmatch(output); len(matches) > 1 {
			if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
				summary.ThroughputRPS = val
			}
		}
	}

	_ = log.Prefix // keep log imported

	return summary
}
