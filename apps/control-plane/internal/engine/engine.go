// Package engine runs k6 as a subprocess and owns everything that happens
// while a test is live: streaming metrics, driving the VU schedule, and
// applying live-control changes.
//
// Metrics come from k6's `--out json` stream, which the engine tails and
// aggregates into one point per second (see aggregator.go and
// docs/decisions/0010-run-metrics-and-live-control.md). VU-based scenarios
// run under k6's externally-controlled executor and the engine moves VUs
// through k6's REST API to follow the plan's stages, so users can override the
// profile mid-run. RPS limits and the stop flag reach VUs through an xk6-kv
// store that an internal control scenario keeps in sync with the control plane.
package engine

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// ErrUnsupported is returned for live-control changes the running test cannot accept.
var ErrUnsupported = errors.New("unsupported")

// ErrNotActive is returned when a run is not (or no longer) executing.
var ErrNotActive = errors.New("run not active")

// flushLag is how many seconds behind wall-clock time snapshots are emitted,
// giving k6's JSON output time to flush a second's samples to disk.
const flushLag = 1

// stopGrace is how long a graceful stop may take before the process is killed.
const stopGrace = 30 * time.Second

// Capabilities describes the k6 binary the engine found.
type Capabilities struct {
	Version string `json:"version"`
	// KV reports whether xk6-kv is compiled in; RPS control needs it.
	KV bool `json:"kv"`
}

// RunSpec describes one run to execute.
type RunSpec struct {
	RunID  string
	Script string
	// Env is exposed to the script as __ENV (environment variables, secrets).
	Env map[string]string
	// Schedule is set when the engine drives VUs for an externally-controlled
	// scenario. Nil means the script's native executors decide everything.
	Schedule *Schedule
	MaxVUs   int
	// PlannedDuration is the effective run length the engine enforces; zero
	// when native executors decide when the run ends.
	PlannedDuration time.Duration
	// MaxDuration is the hard bound live "extend" requests cannot exceed.
	MaxDuration time.Duration
	RPSTarget   int
	// ControlURL and ControlToken are set when the script includes the
	// internal control scenario (requires xk6-kv).
	ControlURL   string
	ControlToken string
	// RawSamplesPath, if set, receives a gzip archive of the raw k6 samples.
	RawSamplesPath string
}

// LiveState is the control-panel view of a running test.
type LiveState struct {
	TargetVUs        int  `json:"target_vus"`
	ActiveVUs        int  `json:"active_vus"`
	MaxVUs           int  `json:"max_vus"`
	RPSTarget        int  `json:"rps_target"`
	Paused           bool `json:"paused"`
	ManualVUs        bool `json:"manual_vus"`
	ElapsedSeconds   int  `json:"elapsed_seconds"`
	RemainingSeconds int  `json:"remaining_seconds"` // -1 when the executor decides
	MaxDurationSecs  int  `json:"max_duration_seconds"`
	Stopping         bool `json:"stopping"`
	CanControlVUs    bool `json:"can_control_vus"`
	CanControlRPS    bool `json:"can_control_rps"`
	CanControlDur    bool `json:"can_control_duration"`
}

// Callbacks receive a run's output. They are invoked from engine goroutines.
type Callbacks struct {
	// OnMetrics is called about once a second with any newly completed
	// seconds (possibly none) and the current live state.
	OnMetrics func(points []db.MetricPoint, state LiveState)
	// OnErrors is called with failed requests recorded since the last call.
	OnErrors func(errs []db.RunError)
	// OnComplete is called exactly once when the k6 process has exited.
	OnComplete func(res Result)
}

// Result is the outcome of a finished run.
type Result struct {
	Status   string // completed | failed | aborted
	ExitCode int
	Summary  *db.RunSummary
	Message  string
}

// Engine manages k6 subprocesses.
type Engine struct {
	k6Path string
	caps   Capabilities

	mu     sync.Mutex
	active map[string]*runProcess
}

// New locates k6 (OPENSYNAPSE_K6_PATH, then PATH) and probes its capabilities.
func New() (*Engine, error) {
	k6Path := os.Getenv("OPENSYNAPSE_K6_PATH")
	if k6Path == "" {
		p, err := exec.LookPath("k6")
		if err != nil {
			return nil, fmt.Errorf("k6 binary not found in PATH")
		}
		k6Path = p
	}
	e := &Engine{k6Path: k6Path, active: make(map[string]*runProcess)}
	e.caps = probeCapabilities(k6Path)
	return e, nil
}

// versionRe matches "k6 v1.8.2 (...)" and, on Windows, "k6.exe v1.7.1 (...)".
var versionRe = regexp.MustCompile(`k6(?:\.exe)? (v[0-9][^\s]*)`)

func probeCapabilities(k6Path string) Capabilities {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	cmd := exec.CommandContext(ctx, k6Path, "version")
	cmd.Env = baseEnv()
	out, err := cmd.Output()
	if err != nil {
		log.Printf("k6 version probe failed: %v", err)
		return Capabilities{}
	}
	return parseCapabilities(string(out))
}

func parseCapabilities(out string) Capabilities {
	c := Capabilities{}
	if m := versionRe.FindStringSubmatch(out); m != nil {
		c.Version = m[1]
	}
	c.KV = strings.Contains(out, "k6/x/kv")
	return c
}

// Capabilities reports what the bundled k6 supports.
func (e *Engine) Capabilities() Capabilities { return e.caps }

// noSummaryFlag disables k6's end-of-test summary (we compute our own).
// k6 v2 replaced --no-summary with --summary-mode=disabled.
func (e *Engine) noSummaryFlag() string {
	if strings.HasPrefix(e.caps.Version, "v0.") || strings.HasPrefix(e.caps.Version, "v1.") {
		return "--no-summary"
	}
	return "--summary-mode=disabled"
}

// baseEnv is the minimal environment k6 runs with. The control plane's own
// environment (database DSNs, object-store keys) is deliberately not passed
// through, because k6 exposes every environment variable to scripts as __ENV.
func baseEnv() []string {
	keep := []string{"PATH", "HOME", "USERPROFILE", "TMP", "TEMP", "TMPDIR", "SYSTEMROOT", "SystemRoot",
		"WINDIR", "LANG", "LC_ALL", "SSL_CERT_FILE", "SSL_CERT_DIR", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
		"ROD_BROWSER_BIN"}
	env := []string{
		// Never contact Grafana services: no usage report, no automatic
		// download of k6 builds for unknown extensions.
		"K6_NO_USAGE_REPORT=true",
		"K6_AUTO_EXTENSION_RESOLUTION=false",
	}
	for _, k := range keep {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

func findFreePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

type runProcess struct {
	spec    RunSpec
	cb      Callbacks
	cmd     *exec.Cmd
	dir     string
	apiPort int
	agg     *aggregator
	client  *http.Client

	exited   chan struct{} // closed when the k6 process has exited
	finished chan struct{} // closed after OnComplete has run

	mu            sync.Mutex
	start         time.Time
	paused        bool
	pausedAt      time.Time
	pausedTotal   time.Duration
	manualVUs     bool
	targetVUs     int
	sentVUs       int
	vusMax        int
	rpsTarget     int
	plannedEnd    time.Duration // planned duration (moves with live duration changes)
	stopRequested bool
	killed        bool
	stderrTail    []string
}

// StartRun launches k6 for the spec and returns once the process has started.
func (e *Engine) StartRun(spec RunSpec, cb Callbacks) error {
	dir, err := os.MkdirTemp("", "opensynapse-run-")
	if err != nil {
		return fmt.Errorf("create run dir: %w", err)
	}
	scriptPath := filepath.Join(dir, "script.js")
	if err := os.WriteFile(scriptPath, []byte(spec.Script), 0o600); err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("write script: %w", err)
	}
	metricsPath := filepath.Join(dir, "metrics.json")

	apiPort, err := findFreePort()
	if err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("allocate k6 API port: %w", err)
	}

	args := []string{
		"run",
		"--no-color",
		"--quiet",
		e.noSummaryFlag(),
		"--no-usage-report",
		fmt.Sprintf("--address=127.0.0.1:%d", apiPort),
		"--out", "json=" + metricsPath,
		scriptPath,
	}
	cmd := exec.Command(e.k6Path, args...)
	cmd.Dir = dir
	cmd.Env = baseEnv()
	for k, v := range spec.Env {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	cmd.Env = append(cmd.Env, fmt.Sprintf("OPENSYNAPSE_RPS_TARGET=%d", spec.RPSTarget))
	if spec.ControlURL != "" {
		cmd.Env = append(cmd.Env,
			"OPENSYNAPSE_CONTROL_URL="+spec.ControlURL,
			"OPENSYNAPSE_CONTROL_TOKEN="+spec.ControlToken)
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("stderr pipe: %w", err)
	}
	cmd.Stdout = io.Discard

	vuOffset := 0
	if spec.ControlURL != "" {
		vuOffset = 1
	}
	rp := &runProcess{
		spec:       spec,
		cb:         cb,
		cmd:        cmd,
		dir:        dir,
		apiPort:    apiPort,
		agg:        newAggregator(vuOffset),
		client:     &http.Client{Timeout: 3 * time.Second},
		exited:     make(chan struct{}),
		finished:   make(chan struct{}),
		vusMax:     spec.MaxVUs,
		rpsTarget:  spec.RPSTarget,
		plannedEnd: spec.PlannedDuration,
	}
	if spec.Schedule != nil {
		rp.targetVUs = spec.Schedule.TargetAt(0)
		rp.sentVUs = rp.targetVUs
	}

	if err := cmd.Start(); err != nil {
		os.RemoveAll(dir)
		return fmt.Errorf("start k6: %w", err)
	}
	rp.start = time.Now()

	e.mu.Lock()
	e.active[spec.RunID] = rp
	e.mu.Unlock()

	go rp.readStderr(stderr)
	tailDone := make(chan struct{})
	go func() {
		defer close(tailDone)
		rp.tail(metricsPath)
	}()
	go rp.loop()
	go func() {
		exitCode := 0
		if err := cmd.Wait(); err != nil {
			var exitErr *exec.ExitError
			if errors.As(err, &exitErr) {
				exitCode = exitErr.ExitCode()
			} else {
				exitCode = -1
			}
		}
		close(rp.exited)
		<-tailDone
		rp.complete(exitCode)

		e.mu.Lock()
		delete(e.active, spec.RunID)
		e.mu.Unlock()
		os.RemoveAll(dir)
		close(rp.finished)
	}()
	return nil
}

func (rp *runProcess) readStderr(r io.Reader) {
	s := bufio.NewScanner(r)
	s.Buffer(make([]byte, 64*1024), 1024*1024)
	for s.Scan() {
		rp.mu.Lock()
		rp.stderrTail = append(rp.stderrTail, s.Text())
		if len(rp.stderrTail) > 40 {
			rp.stderrTail = rp.stderrTail[len(rp.stderrTail)-40:]
		}
		rp.mu.Unlock()
	}
}

// tail follows k6's JSON output file until the process has exited and the
// file is fully drained.
func (rp *runProcess) tail(path string) {
	var f *os.File
	for {
		var err error
		if f, err = os.Open(path); err == nil {
			break
		}
		select {
		case <-rp.exited:
			if f, err = os.Open(path); err != nil {
				return // k6 exited before producing output
			}
		case <-time.After(100 * time.Millisecond):
			continue
		}
		break
	}
	defer f.Close()

	var raw *gzip.Writer
	if rp.spec.RawSamplesPath != "" {
		if err := os.MkdirAll(filepath.Dir(rp.spec.RawSamplesPath), 0o755); err == nil {
			if out, err := os.Create(rp.spec.RawSamplesPath); err == nil {
				defer out.Close()
				raw = gzip.NewWriter(out)
				defer raw.Close()
			}
		}
	}

	r := bufio.NewReaderSize(f, 256*1024)
	var partial []byte
	exited := false
	for {
		chunk, err := r.ReadBytes('\n')
		partial = append(partial, chunk...)
		if err == nil {
			if rp.agg.ingest(partial) && raw != nil {
				raw.Write(partial)
			}
			partial = partial[:0]
			continue
		}
		if err != io.EOF {
			log.Printf("run %s: reading k6 output: %v", rp.spec.RunID, err)
			return
		}
		if exited {
			if len(bytes.TrimSpace(partial)) > 0 && rp.agg.ingest(partial) && raw != nil {
				raw.Write(append(partial, '\n'))
			}
			return
		}
		select {
		case <-rp.exited:
			exited = true // one more pass to drain what k6 flushed on exit
		case <-time.After(100 * time.Millisecond):
		}
	}
}

// loop runs once a second for the life of the process: it advances the VU
// schedule, enforces the end time, and publishes metrics.
func (rp *runProcess) loop() {
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-rp.exited:
			return
		case now := <-ticker.C:
			rp.driveSchedule(now)
			rp.enforceDeadline(now)
			points := rp.agg.snapshots(now.Unix() - flushLag - 1)
			state := rp.state(now)
			if rp.cb.OnMetrics != nil {
				rp.cb.OnMetrics(points, state)
			}
			if errs := rp.agg.takeErrors(); len(errs) > 0 && rp.cb.OnErrors != nil {
				rp.cb.OnErrors(errs)
			}
		}
	}
}

func (rp *runProcess) elapsed(now time.Time) time.Duration {
	paused := rp.pausedTotal
	if rp.paused {
		paused += now.Sub(rp.pausedAt)
	}
	return now.Sub(rp.start) - paused
}

func (rp *runProcess) driveSchedule(now time.Time) {
	rp.mu.Lock()
	if rp.spec.Schedule == nil || rp.manualVUs || rp.paused || rp.stopRequested {
		rp.mu.Unlock()
		return
	}
	target := rp.spec.Schedule.TargetAt(rp.elapsed(now))
	rp.targetVUs = target
	need := target != rp.sentVUs
	rp.mu.Unlock()
	if need {
		if err := rp.applyVUs(target); err != nil {
			log.Printf("run %s: schedule VU change to %d: %v", rp.spec.RunID, target, err)
		}
	}
}

func (rp *runProcess) enforceDeadline(now time.Time) {
	rp.mu.Lock()
	due := rp.plannedEnd > 0 && !rp.stopRequested && rp.elapsed(now) >= rp.plannedEnd
	rp.mu.Unlock()
	if due {
		rp.requestStop()
	}
}

// applyVUs sets the active VU count through k6's REST API, raising vus-max first if needed.
func (rp *runProcess) applyVUs(n int) error {
	rp.mu.Lock()
	vusMax := rp.vusMax
	rp.mu.Unlock()
	if n > vusMax {
		if err := rp.patchStatus(map[string]any{"vus-max": n}); err != nil {
			return fmt.Errorf("raise vus-max to %d: %w", n, err)
		}
		rp.mu.Lock()
		rp.vusMax = n
		rp.mu.Unlock()
	}
	if err := rp.patchStatus(map[string]any{"vus": n}); err != nil {
		return err
	}
	rp.mu.Lock()
	rp.sentVUs = n
	rp.mu.Unlock()
	return nil
}

func (rp *runProcess) patchStatus(attrs map[string]any) error {
	body, _ := json.Marshal(map[string]any{
		"data": map[string]any{"type": "status", "id": "default", "attributes": attrs},
	})
	req, _ := http.NewRequest(http.MethodPatch, fmt.Sprintf("http://127.0.0.1:%d/v1/status", rp.apiPort), bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	resp, err := rp.client.Do(req)
	if err != nil {
		return fmt.Errorf("k6 REST API: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode >= 400 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 2048))
		var parsed struct {
			Errors []struct {
				Detail string `json:"detail"`
				Title  string `json:"title"`
			} `json:"errors"`
		}
		if json.Unmarshal(msg, &parsed) == nil && len(parsed.Errors) > 0 {
			d := parsed.Errors[0].Detail
			if d == "" {
				d = parsed.Errors[0].Title
			}
			return fmt.Errorf("k6 rejected the change: %s", d)
		}
		return fmt.Errorf("k6 REST API returned %d: %s", resp.StatusCode, strings.TrimSpace(string(msg)))
	}
	return nil
}

// requestStop asks k6 to stop gracefully and kills it if that hangs.
func (rp *runProcess) requestStop() {
	rp.mu.Lock()
	if rp.stopRequested {
		rp.mu.Unlock()
		return
	}
	rp.stopRequested = true
	rp.mu.Unlock()

	if err := rp.patchStatus(map[string]any{"stopped": true}); err != nil {
		log.Printf("run %s: graceful stop via REST failed (%v); interrupting", rp.spec.RunID, err)
		if runtime.GOOS == "windows" || rp.cmd.Process.Signal(os.Interrupt) != nil {
			rp.cmd.Process.Kill()
		}
	}
	go func() {
		select {
		case <-rp.exited:
		case <-time.After(stopGrace):
			log.Printf("run %s: graceful stop timed out; killing k6", rp.spec.RunID)
			rp.cmd.Process.Kill()
		}
	}()
}

func (rp *runProcess) state(now time.Time) LiveState {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	el := rp.elapsed(now)
	st := LiveState{
		TargetVUs:       rp.targetVUs,
		ActiveVUs:       rp.agg.vus(),
		MaxVUs:          rp.vusMax,
		RPSTarget:       rp.rpsTarget,
		Paused:          rp.paused,
		ManualVUs:       rp.manualVUs,
		ElapsedSeconds:  int(el.Seconds()),
		RemainingSeconds: -1,
		MaxDurationSecs: int(rp.spec.MaxDuration.Seconds()),
		Stopping:        rp.stopRequested,
		CanControlVUs:   rp.spec.Schedule != nil,
		CanControlRPS:   rp.spec.ControlURL != "",
		CanControlDur:   rp.spec.PlannedDuration > 0,
	}
	if rp.plannedEnd > 0 {
		rem := rp.plannedEnd - el
		if rem < 0 {
			rem = 0
		}
		st.RemainingSeconds = int(rem.Seconds())
	}
	if rp.spec.Schedule == nil {
		st.TargetVUs = st.ActiveVUs
	}
	return st
}

func (rp *runProcess) complete(exitCode int) {
	// Emit every remaining second, including the final partial one.
	points := rp.agg.snapshots(time.Now().Unix() + 1)
	final := rp.state(time.Now())
	if len(points) > 0 && rp.cb.OnMetrics != nil {
		rp.cb.OnMetrics(points, final)
	}
	if errs := rp.agg.takeErrors(); len(errs) > 0 && rp.cb.OnErrors != nil {
		rp.cb.OnErrors(errs)
	}

	summary := rp.agg.summary()
	rp.mu.Lock()
	killed, stopRequested := rp.killed, rp.stopRequested
	tail := append([]string(nil), rp.stderrTail...)
	rp.mu.Unlock()

	res := Result{ExitCode: exitCode, Summary: summary}
	switch {
	case killed:
		res.Status = "aborted"
		res.Message = "Run was killed."
	case exitCode == 0:
		res.Status = "completed"
	case exitCode == 99:
		res.Status = "completed"
		summary.ThresholdsPassed = false
		res.Message = "One or more thresholds failed."
	case stopRequested && (exitCode == 103 || exitCode == 105 || exitCode == 108 || exitCode < 0):
		// k6 exits 103 ("test run stopped from REST API") after a graceful
		// stop we requested; that is a normal end, not a failure.
		res.Status = "completed"
	default:
		res.Status = "failed"
		res.Message = failureMessage(exitCode, tail)
	}
	if rp.cb.OnComplete != nil {
		rp.cb.OnComplete(res)
	}
}

var k6ErrorRe = regexp.MustCompile(`level=error msg="((?:[^"\\]|\\.)*)"`)

// failureMessage turns k6's exit code and last log lines into a readable reason.
func failureMessage(exitCode int, stderr []string) string {
	var msgs []string
	for _, l := range stderr {
		if m := k6ErrorRe.FindStringSubmatch(l); m != nil {
			msgs = append(msgs, strings.ReplaceAll(m[1], `\"`, `"`))
		}
	}
	reason := map[int]string{
		100: "setup() timed out",
		101: "teardown() timed out",
		103: "k6 engine error",
		104: "invalid test configuration",
		106: "k6 could not start its REST API",
		107: "the script threw an exception",
		108: "the script called test.abort()",
		109: "k6 crashed",
	}[exitCode]
	if reason == "" {
		reason = "k6 failed"
	}
	out := fmt.Sprintf("%s (exit code %d)", reason, exitCode)
	if len(msgs) > 0 {
		if len(msgs) > 3 {
			msgs = msgs[len(msgs)-3:]
		}
		out += ": " + strings.Join(msgs, " | ")
	} else if n := len(stderr); n > 0 {
		last := stderr
		if n > 3 {
			last = stderr[n-3:]
		}
		out += ": " + strings.Join(last, " | ")
	}
	return out
}

// --- Live control ---

func (e *Engine) get(runID string) (*runProcess, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	rp, ok := e.active[runID]
	if !ok {
		return nil, ErrNotActive
	}
	return rp, nil
}

// IsRunning reports whether a run's k6 process is still executing.
func (e *Engine) IsRunning(runID string) bool {
	_, err := e.get(runID)
	return err == nil
}

// State returns the live state of a running test.
func (e *Engine) State(runID string) (LiveState, error) {
	rp, err := e.get(runID)
	if err != nil {
		return LiveState{}, err
	}
	return rp.state(time.Now()), nil
}

// SetVUs overrides the VU schedule with a fixed count.
func (e *Engine) SetVUs(runID string, vus int) error {
	rp, err := e.get(runID)
	if err != nil {
		return err
	}
	if rp.spec.Schedule == nil {
		return fmt.Errorf("%w: live VU changes need a VU-based scenario (constant-vus, ramping-vus or externally-controlled); this plan uses an arrival-rate or iteration executor", ErrUnsupported)
	}
	if vus < 0 {
		return fmt.Errorf("vus must be zero or more")
	}
	if err := rp.applyVUs(vus); err != nil {
		return err
	}
	rp.mu.Lock()
	rp.manualVUs = true
	rp.targetVUs = vus
	rp.mu.Unlock()
	return nil
}

// ResumeSchedule hands VU control back to the plan's load profile.
func (e *Engine) ResumeSchedule(runID string) error {
	rp, err := e.get(runID)
	if err != nil {
		return err
	}
	if rp.spec.Schedule == nil {
		return fmt.Errorf("%w: this run has no VU schedule", ErrUnsupported)
	}
	rp.mu.Lock()
	rp.manualVUs = false
	rp.mu.Unlock()
	return nil
}

// SetPaused pauses or resumes all VUs. With the control scenario present the
// pause goes through the kv store (VUs finish their current iteration and
// wait), because k6 only supports pausing mid-run when every scenario is
// externally-controlled; otherwise k6's REST API pauses the executor.
func (e *Engine) SetPaused(runID string, paused bool) error {
	rp, err := e.get(runID)
	if err != nil {
		return err
	}
	if rp.spec.ControlURL == "" {
		if err := rp.patchStatus(map[string]any{"paused": paused}); err != nil {
			return err
		}
	}
	rp.mu.Lock()
	defer rp.mu.Unlock()
	now := time.Now()
	if paused && !rp.paused {
		rp.paused, rp.pausedAt = true, now
	} else if !paused && rp.paused {
		rp.paused = false
		rp.pausedTotal += now.Sub(rp.pausedAt)
	}
	return nil
}

// SetRPS changes the request-rate ceiling (0 removes it).
func (e *Engine) SetRPS(runID string, rps int) error {
	rp, err := e.get(runID)
	if err != nil {
		return err
	}
	if rp.spec.ControlURL == "" {
		return fmt.Errorf("%w: RPS control needs a k6 build with the xk6-kv extension (bundled in the Docker image)", ErrUnsupported)
	}
	if rps < 0 {
		return fmt.Errorf("rps must be zero or more")
	}
	rp.mu.Lock()
	rp.rpsTarget = rps
	rp.mu.Unlock()
	return nil
}

// SetDuration moves the run's end to `seconds` after it started (excluding paused time).
func (e *Engine) SetDuration(runID string, seconds int) error {
	rp, err := e.get(runID)
	if err != nil {
		return err
	}
	if rp.spec.PlannedDuration <= 0 {
		return fmt.Errorf("%w: this plan's executors decide when the run ends", ErrUnsupported)
	}
	d := time.Duration(seconds) * time.Second
	if d <= 0 {
		return fmt.Errorf("duration must be positive")
	}
	if rp.spec.MaxDuration > 0 && d > rp.spec.MaxDuration {
		return fmt.Errorf("duration can be extended to at most %s for this run", rp.spec.MaxDuration)
	}
	rp.mu.Lock()
	rp.plannedEnd = d
	rp.mu.Unlock()
	rp.enforceDeadline(time.Now())
	return nil
}

// ControlState is what the in-script control scenario reads each second.
type ControlState struct {
	RPSTarget  int  `json:"rps_target"`
	ShouldStop bool `json:"should_stop"`
	Paused     bool `json:"paused"`
}

// ControlStateFor returns the control state for a run if token matches.
func (e *Engine) ControlStateFor(runID, token string) (ControlState, error) {
	rp, err := e.get(runID)
	if err != nil {
		return ControlState{}, err
	}
	if rp.spec.ControlToken == "" || token != rp.spec.ControlToken {
		return ControlState{}, fmt.Errorf("invalid control token")
	}
	rp.mu.Lock()
	defer rp.mu.Unlock()
	return ControlState{RPSTarget: rp.rpsTarget, ShouldStop: rp.stopRequested, Paused: rp.paused}, nil
}

// StopRun stops a run gracefully; it completes normally with what it has.
func (e *Engine) StopRun(runID string) error {
	rp, err := e.get(runID)
	if err != nil {
		return err
	}
	rp.requestStop()
	return nil
}

// KillRun terminates k6 immediately; the run is marked aborted.
func (e *Engine) KillRun(runID string) error {
	rp, err := e.get(runID)
	if err != nil {
		return err
	}
	rp.mu.Lock()
	rp.killed = true
	rp.mu.Unlock()
	return rp.cmd.Process.Kill()
}

// Wait blocks until the run has finished and its completion callback has run.
func (e *Engine) Wait(runID string) {
	rp, err := e.get(runID)
	if err != nil {
		return
	}
	<-rp.finished
}
