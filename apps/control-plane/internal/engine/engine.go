// Package engine runs k6 and owns everything that happens while a test is
// live: streaming metrics, driving the VU schedule, and applying live-control
// changes.
//
// k6 runs either as a local subprocess or, in cluster mode, as the runner
// pods of a k6-operator TestRun (see kubernetes.go). Either way each k6
// instance writes its `--out json` stream, which the engine aggregates into
// one point per second (see aggregator.go and
// docs/decisions/0010-run-metrics-and-live-control.md). VU-based scenarios
// run under k6's externally-controlled executor and the engine moves VUs
// through each instance's REST API to follow the plan's stages, so users can
// override the profile mid-run. RPS limits and the stop flag reach VUs through
// an xk6-kv store that an internal control scenario keeps in sync with the
// control plane.
package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
	"regexp"
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
// giving k6's JSON output time to flush a second's samples.
const flushLag = 1

// stopGrace is how long a graceful stop may take before k6 is killed.
const stopGrace = 30 * time.Second

// Capabilities describes the k6 binary the engine found.
type Capabilities struct {
	Version string `json:"version"`
	// KV reports whether xk6-kv is compiled in; RPS control needs it.
	KV bool `json:"kv"`
}

// Execution describes where runs execute.
type Execution struct {
	// Mode is "local" (a k6 subprocess) or "kubernetes" (k6-operator TestRuns).
	Mode string `json:"mode"`
	// MaxWorkers is the most k6 instances one run may use.
	MaxWorkers int `json:"max_workers"`
}

// RunSpec describes one run to execute.
type RunSpec struct {
	RunID  string
	Script string
	// Env is exposed to the script as __ENV (environment variables, secrets).
	Env map[string]string
	// Instances is how many k6 instances share the load (cluster mode only).
	Instances int
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
	// RawSamples, if set, opens where a gzip archive of the raw k6 samples goes.
	RawSamples func() (io.WriteCloser, error)
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
	// Workers is how many k6 instances share the run; Starting is true until
	// they all begin executing (cluster runs take a while to schedule).
	Workers  int  `json:"workers"`
	Starting bool `json:"starting"`
}

// Callbacks receive a run's output. They are invoked from engine goroutines.
type Callbacks struct {
	// OnMetrics is called about once a second with any newly completed
	// seconds (possibly none) and the current live state.
	OnMetrics func(points []db.MetricPoint, state LiveState)
	// OnErrors is called with failed requests recorded since the last call.
	OnErrors func(errs []db.RunError)
	// OnComplete is called exactly once when every k6 instance has exited.
	OnComplete func(res Result)
}

// Result is the outcome of a finished run.
type Result struct {
	Status   string // completed | failed | aborted
	ExitCode int
	Summary  *db.RunSummary
	Message  string
}

// backend runs the k6 instances of one run and feeds their output to the
// runProcess (ingest, addLog, markStarted).
type backend interface {
	// apis returns the base URLs of the instances' REST APIs.
	apis() []string
	// wait blocks until every instance has exited and its output is drained,
	// and returns the combined exit code.
	wait() int
	// interrupt asks the instances to stop when their REST APIs are unreachable.
	interrupt() error
	// kill stops every instance immediately.
	kill() error
	// cleanup releases what the run created, after it has finished.
	cleanup()
}

// Engine starts and tracks runs.
type Engine struct {
	k6Path string
	caps   Capabilities
	kube   *KubernetesConfig

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

// UseKubernetes switches the engine to cluster mode: runs become k6-operator
// TestRuns. The runner image must contain the same k6 build this process
// probed (the control-plane image does).
func (e *Engine) UseKubernetes(cfg KubernetesConfig) {
	if cfg.MaxWorkers <= 0 {
		cfg.MaxWorkers = 1
	}
	e.kube = &cfg
}

// Execution reports where runs execute and how many workers a run may use.
func (e *Engine) Execution() Execution {
	if e.kube != nil {
		return Execution{Mode: "kubernetes", MaxWorkers: e.kube.MaxWorkers}
	}
	return Execution{Mode: "local", MaxWorkers: 1}
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

// k6Settings are the environment variables every k6 instance runs with.
// Never contact Grafana services: no usage report, no automatic download of
// k6 builds for unknown extensions.
var k6Settings = []string{"K6_NO_USAGE_REPORT=true", "K6_AUTO_EXTENSION_RESOLUTION=false"}

// baseEnv is the minimal environment a local k6 runs with. The control
// plane's own environment (database DSNs, object-store keys) is deliberately
// not passed through, because k6 exposes every environment variable to
// scripts as __ENV.
func baseEnv() []string {
	keep := []string{"PATH", "HOME", "USERPROFILE", "TMP", "TEMP", "TMPDIR", "SYSTEMROOT", "SystemRoot",
		"WINDIR", "LANG", "LC_ALL", "SSL_CERT_FILE", "SSL_CERT_DIR", "HTTP_PROXY", "HTTPS_PROXY", "NO_PROXY",
		"ROD_BROWSER_BIN"}
	env := append([]string(nil), k6Settings...)
	for _, k := range keep {
		if v, ok := os.LookupEnv(k); ok {
			env = append(env, k+"="+v)
		}
	}
	return env
}

// runEnv is what the script sees as __ENV, besides the k6 settings: the
// environment's variables and the live-control wiring.
func runEnv(spec RunSpec) map[string]string {
	env := map[string]string{}
	for k, v := range spec.Env {
		env[k] = v
	}
	env["OPENSYNAPSE_RPS_TARGET"] = fmt.Sprint(spec.RPSTarget)
	env["OPENSYNAPSE_INSTANCES"] = fmt.Sprint(max(1, spec.Instances))
	if spec.ControlURL != "" {
		env["OPENSYNAPSE_CONTROL_URL"] = spec.ControlURL
		env["OPENSYNAPSE_CONTROL_TOKEN"] = spec.ControlToken
	}
	return env
}

type runProcess struct {
	spec    RunSpec
	cb      Callbacks
	backend backend
	agg     *aggregator
	client  *http.Client

	rawMu  sync.Mutex
	rawOut io.WriteCloser
	raw    *gzip.Writer

	exited   chan struct{} // closed when every instance has exited
	finished chan struct{} // closed after OnComplete has run

	mu            sync.Mutex
	started       bool
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
	logTail       []string
}

// StartRun launches k6 for the spec and returns once it has been started
// (locally) or submitted to the cluster.
func (e *Engine) StartRun(spec RunSpec, cb Callbacks) error {
	if spec.Instances <= 0 {
		spec.Instances = 1
	}
	if e.kube == nil && spec.Instances > 1 {
		return fmt.Errorf("running on %d workers needs cluster mode (OPENSYNAPSE_MODE=cluster)", spec.Instances)
	}
	if e.kube != nil && spec.Instances > e.kube.MaxWorkers {
		return fmt.Errorf("at most %d workers per run are allowed", e.kube.MaxWorkers)
	}

	vuOffset := 0
	if spec.ControlURL != "" {
		vuOffset = 1 // the internal control VU, once per instance
	}
	rp := &runProcess{
		spec:       spec,
		cb:         cb,
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
	if spec.RawSamples != nil {
		if out, err := spec.RawSamples(); err != nil {
			log.Printf("run %s: raw samples archive unavailable: %v", spec.RunID, err)
		} else {
			rp.rawOut, rp.raw = out, gzip.NewWriter(out)
		}
	}

	var err error
	if e.kube != nil {
		rp.backend, err = startKubernetes(*e.kube, rp)
	} else {
		rp.backend, err = e.startLocal(rp)
	}
	if err != nil {
		rp.closeRaw()
		return err
	}

	e.mu.Lock()
	e.active[spec.RunID] = rp
	e.mu.Unlock()

	go rp.loop()
	go func() {
		exitCode := rp.backend.wait()
		close(rp.exited)
		rp.complete(exitCode)
		rp.closeRaw()

		e.mu.Lock()
		delete(e.active, spec.RunID)
		e.mu.Unlock()
		rp.backend.cleanup()
		close(rp.finished)
	}()
	return nil
}

// markStarted records that every instance is executing; the schedule and the
// planned end are measured from here.
func (rp *runProcess) markStarted() {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	if !rp.started {
		rp.started, rp.start = true, time.Now()
	}
}

func (rp *runProcess) isStarted() bool {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	return rp.started
}

// ingest takes one line of an instance's `--out json` stream.
func (rp *runProcess) ingest(instance string, line []byte) {
	if !rp.agg.ingest(instance, line) {
		return
	}
	rp.rawMu.Lock()
	defer rp.rawMu.Unlock()
	if rp.raw != nil {
		rp.raw.Write(line)
		if len(line) == 0 || line[len(line)-1] != '\n' {
			rp.raw.Write([]byte{'\n'})
		}
	}
}

func (rp *runProcess) closeRaw() {
	rp.rawMu.Lock()
	defer rp.rawMu.Unlock()
	if rp.raw != nil {
		rp.raw.Close()
		if err := rp.rawOut.Close(); err != nil {
			log.Printf("run %s: store raw samples: %v", rp.spec.RunID, err)
		}
		rp.raw, rp.rawOut = nil, nil
	}
}

// addLog keeps the last lines of k6's log output for failure messages.
func (rp *runProcess) addLog(line string) {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	rp.logTail = append(rp.logTail, line)
	if len(rp.logTail) > 40 {
		rp.logTail = rp.logTail[len(rp.logTail)-40:]
	}
}

// loop runs once a second for the life of the run: it advances the VU
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
	if !rp.started {
		return 0
	}
	paused := rp.pausedTotal
	if rp.paused {
		paused += now.Sub(rp.pausedAt)
	}
	return now.Sub(rp.start) - paused
}

func (rp *runProcess) driveSchedule(now time.Time) {
	rp.mu.Lock()
	if !rp.started || rp.spec.Schedule == nil || rp.manualVUs || rp.paused || rp.stopRequested {
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
	due := rp.started && rp.plannedEnd > 0 && !rp.stopRequested && rp.elapsed(now) >= rp.plannedEnd
	rp.mu.Unlock()
	if due {
		rp.requestStop()
	}
}

// applyVUs sets the active VU count through k6's REST API, raising vus-max
// first if needed. With several instances every one receives the run-wide
// numbers; each scales them by its execution segment.
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

// patchStatus sends a status change to every instance's REST API at once.
func (rp *runProcess) patchStatus(attrs map[string]any) error {
	apis := rp.backend.apis()
	if len(apis) == 0 {
		return errors.New("k6 has not started yet")
	}
	body, _ := json.Marshal(map[string]any{
		"data": map[string]any{"type": "status", "id": "default", "attributes": attrs},
	})
	errs := make([]error, len(apis))
	var wg sync.WaitGroup
	for i, base := range apis {
		wg.Add(1)
		go func() {
			defer wg.Done()
			errs[i] = rp.patchOne(base, body)
		}()
	}
	wg.Wait()
	for _, err := range errs {
		if err != nil {
			return err
		}
	}
	return nil
}

func (rp *runProcess) patchOne(base string, body []byte) error {
	req, _ := http.NewRequest(http.MethodPatch, base+"/v1/status", bytes.NewReader(body))
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
		if err := rp.backend.interrupt(); err != nil {
			rp.backend.kill()
		}
	}
	go func() {
		select {
		case <-rp.exited:
		case <-time.After(stopGrace):
			log.Printf("run %s: graceful stop timed out; killing k6", rp.spec.RunID)
			rp.backend.kill()
		}
	}()
}

func (rp *runProcess) state(now time.Time) LiveState {
	rp.mu.Lock()
	defer rp.mu.Unlock()
	el := rp.elapsed(now)
	st := LiveState{
		TargetVUs:        rp.targetVUs,
		ActiveVUs:        rp.agg.vus(),
		MaxVUs:           rp.vusMax,
		RPSTarget:        rp.rpsTarget,
		Paused:           rp.paused,
		ManualVUs:        rp.manualVUs,
		ElapsedSeconds:   int(el.Seconds()),
		RemainingSeconds: -1,
		MaxDurationSecs:  int(rp.spec.MaxDuration.Seconds()),
		Stopping:         rp.stopRequested,
		CanControlVUs:    rp.spec.Schedule != nil,
		CanControlRPS:    rp.spec.ControlURL != "",
		CanControlDur:    rp.spec.PlannedDuration > 0,
		Workers:          rp.spec.Instances,
		Starting:         !rp.started,
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
	tail := append([]string(nil), rp.logTail...)
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

// combineExitCodes folds several instances' exit codes into one: a real
// failure wins over failed thresholds, which win over a requested stop.
func combineExitCodes(codes []int) int {
	worst := 0
	rank := func(c int) int {
		switch c {
		case 0:
			return 0
		case 103:
			return 1
		case 99:
			return 2
		default:
			return 3
		}
	}
	for _, c := range codes {
		if rank(c) > rank(worst) {
			worst = c
		}
	}
	return worst
}

var k6ErrorRe = regexp.MustCompile(`level=error msg="((?:[^"\\]|\\.)*)"`)

// failureMessage turns k6's exit code and last log lines into a readable reason.
func failureMessage(exitCode int, logs []string) string {
	var msgs []string
	for _, l := range logs {
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
	} else if n := len(logs); n > 0 {
		last := logs
		if n > 3 {
			last = logs[n-3:]
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

// IsRunning reports whether a run is still executing.
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
// RPSTarget is run-wide; each instance takes its share (OPENSYNAPSE_INSTANCES).
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
	return rp.backend.kill()
}

// ActiveCount is the number of runs executing now.
func (e *Engine) ActiveCount() int {
	e.mu.Lock()
	defer e.mu.Unlock()
	return len(e.active)
}

// Shutdown kills every running k6 (on control-plane exit) and waits briefly
// for their completion callbacks so their status is recorded.
func (e *Engine) Shutdown(wait time.Duration) {
	e.mu.Lock()
	procs := make([]*runProcess, 0, len(e.active))
	for _, rp := range e.active {
		procs = append(procs, rp)
	}
	e.mu.Unlock()
	for _, rp := range procs {
		rp.mu.Lock()
		rp.killed = true
		rp.mu.Unlock()
		rp.backend.kill()
	}
	deadline := time.After(wait)
	for _, rp := range procs {
		select {
		case <-rp.finished:
		case <-deadline:
			return
		}
	}
}

// Wait blocks until the run has finished and its completion callback has run.
func (e *Engine) Wait(runID string) {
	rp, err := e.get(runID)
	if err != nil {
		return
	}
	<-rp.finished
}
