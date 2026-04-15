package engine

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
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

// ControlRequest represents a live control change.
type ControlRequest struct {
	VUs             *int  `json:"vus,omitempty"`
	RPS             *int  `json:"rps,omitempty"`
	DurationSeconds *int  `json:"duration_seconds,omitempty"`
	Paused          *bool `json:"paused,omitempty"`
}

// MetricsCallback is called every second with aggregated metrics.
type MetricsCallback func(runID string, snapshot MetricSnapshot)

// CompletionCallback is called when the k6 process exits.
type CompletionCallback func(runID string, exitCode int, summary *db.RunSummary)

// Engine manages k6 subprocess execution.
type Engine struct {
	k6Path string
	mu     sync.Mutex
	active map[string]*runProcess
}

type runProcess struct {
	cmd          *exec.Cmd
	cancel       context.CancelFunc
	runID        string
	k6APIPort    int
	currentVUs   int
	currentRPS   int
	paused       bool
	effectiveEnd time.Time
	startTime    time.Time
	mu           sync.Mutex
}

// New creates a new engine. It auto-discovers k6 from PATH.
func New() (*Engine, error) {
	k6Path, err := exec.LookPath("k6")
	if err != nil {
		return nil, fmt.Errorf("k6 binary not found in PATH")
	}

	return &Engine{
		k6Path: k6Path,
		active: make(map[string]*runProcess),
	}, nil
}

// findFreePort returns a random available TCP port.
func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	port := l.Addr().(*net.TCPAddr).Port
	l.Close()
	return port, nil
}

// StartRun compiles the plan to a k6 script and runs it as a subprocess.
func (e *Engine) StartRun(runID string, script string, params db.RunParameters, onMetrics MetricsCallback, onComplete CompletionCallback) error {
	// Write script to temp file
	tmpDir := os.TempDir()
	scriptPath := filepath.Join(tmpDir, fmt.Sprintf("opensynapse-%s.js", runID))
	if err := os.WriteFile(scriptPath, []byte(script), 0644); err != nil {
		return fmt.Errorf("write script: %w", err)
	}

	// Find a free port for k6's REST API
	apiPort, err := findFreePort()
	if err != nil {
		apiPort = 6565 // fallback
	}

	ctx, cancel := context.WithCancel(context.Background())

	args := []string{
		"run",
		"--no-color",
		fmt.Sprintf("--address=localhost:%d", apiPort),
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

	now := time.Now()
	rp := &runProcess{
		cmd:          cmd,
		cancel:       cancel,
		runID:        runID,
		k6APIPort:    apiPort,
		currentVUs:   params.VUsTarget,
		currentRPS:   params.RPSTarget,
		effectiveEnd: now.Add(time.Duration(params.DurationSeconds) * time.Second),
		startTime:    now,
	}
	e.mu.Lock()
	e.active[runID] = rp
	e.mu.Unlock()

	// Background goroutine: wait for process completion
	go func() {
		defer os.Remove(scriptPath)
		defer func() {
			e.mu.Lock()
			delete(e.active, runID)
			e.mu.Unlock()
		}()

		var stderrLines []string
		go func() {
			scanner := bufio.NewScanner(stderr)
			for scanner.Scan() {
				stderrLines = append(stderrLines, scanner.Text())
			}
		}()

		var outputLines []string
		scanner := bufio.NewScanner(stdout)
		for scanner.Scan() {
			outputLines = append(outputLines, scanner.Text())
		}

		exitCode := 0
		if err := cmd.Wait(); err != nil {
			if exitErr, ok := err.(*exec.ExitError); ok {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}

		summary := parseK6Summary(outputLines)
		if onComplete != nil {
			onComplete(runID, exitCode, summary)
		}
	}()

	// Background goroutine: emit metrics every second
	go func() {
		ticker := time.NewTicker(1 * time.Second)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				rp.mu.Lock()
				vus := rp.currentVUs
				rps := rp.currentRPS
				rp.mu.Unlock()

				snapshot := MetricSnapshot{
					TimestampMS: time.Now().UnixMilli(),
					ActiveVUs:   vus,
					RPS:         float64(rps),
				}

				// Try to get real status from k6 API
				if status := e.getK6Status(rp.k6APIPort); status != nil {
					if v, ok := status["vus"].(float64); ok {
						snapshot.ActiveVUs = int(v)
					}
				}

				if onMetrics != nil {
					onMetrics(runID, snapshot)
				}
			}
		}
	}()

	return nil
}

// ControlRun applies live control changes to a running test.
func (e *Engine) ControlRun(runID string, req ControlRequest) error {
	e.mu.Lock()
	rp, ok := e.active[runID]
	e.mu.Unlock()
	if !ok {
		return fmt.Errorf("run %s not active", runID)
	}

	// VU changes: PATCH k6 REST API
	if req.VUs != nil {
		err := e.patchK6Status(rp.k6APIPort, map[string]interface{}{
			"vus": *req.VUs,
		})
		if err != nil {
			log.Printf("control VUs: %v (k6 API may not support externally-controlled for this script)", err)
		}
		rp.mu.Lock()
		rp.currentVUs = *req.VUs
		rp.mu.Unlock()
	}

	// RPS changes: update the target (in production, this writes to xk6-kv)
	if req.RPS != nil {
		rp.mu.Lock()
		rp.currentRPS = *req.RPS
		rp.mu.Unlock()
	}

	// Duration changes: adjust effective end time
	if req.DurationSeconds != nil {
		rp.mu.Lock()
		rp.effectiveEnd = rp.startTime.Add(time.Duration(*req.DurationSeconds) * time.Second)
		rp.mu.Unlock()
	}

	// Pause/resume
	if req.Paused != nil {
		if *req.Paused {
			e.patchK6Status(rp.k6APIPort, map[string]interface{}{"paused": true})
		} else {
			e.patchK6Status(rp.k6APIPort, map[string]interface{}{"paused": false})
		}
		rp.mu.Lock()
		rp.paused = *req.Paused
		rp.mu.Unlock()
	}

	return nil
}

// GetRunState returns the current state of a running test.
func (e *Engine) GetRunState(runID string) (vus int, rps int, paused bool, remainingSec int, ok bool) {
	e.mu.Lock()
	rp, exists := e.active[runID]
	e.mu.Unlock()
	if !exists {
		return 0, 0, false, 0, false
	}

	rp.mu.Lock()
	defer rp.mu.Unlock()
	remaining := int(time.Until(rp.effectiveEnd).Seconds())
	if remaining < 0 {
		remaining = 0
	}
	return rp.currentVUs, rp.currentRPS, rp.paused, remaining, true
}

// StopRun gracefully stops a running k6 process.
func (e *Engine) StopRun(runID string) error {
	e.mu.Lock()
	rp, ok := e.active[runID]
	e.mu.Unlock()
	if !ok {
		return fmt.Errorf("run %s not active", runID)
	}

	if rp.cmd.Process != nil {
		rp.cmd.Process.Signal(os.Interrupt)
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

// --- k6 REST API helpers ---

func (e *Engine) getK6Status(port int) map[string]interface{} {
	client := &http.Client{Timeout: 500 * time.Millisecond}
	resp, err := client.Get(fmt.Sprintf("http://localhost:%d/v1/status", port))
	if err != nil {
		return nil
	}
	defer resp.Body.Close()

	var result map[string]interface{}
	body, _ := io.ReadAll(resp.Body)
	json.Unmarshal(body, &result)

	// k6 wraps status in a "data" key
	if data, ok := result["data"].(map[string]interface{}); ok {
		if attrs, ok := data["attributes"].(map[string]interface{}); ok {
			return attrs
		}
	}
	return result
}

func (e *Engine) patchK6Status(port int, payload map[string]interface{}) error {
	body := map[string]interface{}{
		"data": map[string]interface{}{
			"type":       "status",
			"id":         "default",
			"attributes": payload,
		},
	}
	bodyJSON, _ := json.Marshal(body)

	client := &http.Client{Timeout: 2 * time.Second}
	req, _ := http.NewRequest("PATCH", fmt.Sprintf("http://localhost:%d/v1/status", port), bytes.NewReader(bodyJSON))
	req.Header.Set("Content-Type", "application/json")

	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("k6 API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("k6 API returned %d: %s", resp.StatusCode, string(respBody))
	}
	return nil
}

// parseK6Summary attempts to extract summary stats from k6 stdout output.
func parseK6Summary(lines []string) *db.RunSummary {
	summary := &db.RunSummary{ThresholdsPassed: true}
	output := strings.Join(lines, "\n")

	patterns := map[string]*float64{
		`http_reqs[.\s]+(\d+)`:                   nil,
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

	failedRe := regexp.MustCompile(`http_req_failed[.\s]+(\d+\.\d+)%`)
	if matches := failedRe.FindStringSubmatch(output); len(matches) > 1 {
		if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
			summary.ErrorRate = val / 100.0
			summary.FailedRequests = int64(float64(summary.TotalRequests) * summary.ErrorRate)
		}
	}

	if summary.TotalRequests > 0 {
		rpsRe := regexp.MustCompile(`http_reqs[.\s]+\d+\s+([0-9.]+)/s`)
		if matches := rpsRe.FindStringSubmatch(output); len(matches) > 1 {
			if val, err := strconv.ParseFloat(matches[1], 64); err == nil {
				summary.ThroughputRPS = val
			}
		}
	}

	return summary
}
