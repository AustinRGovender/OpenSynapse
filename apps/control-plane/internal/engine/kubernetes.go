package engine

import (
	"bufio"
	"bytes"
	"context"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/kube"
)

// Cluster mode (docs/decisions/0012-cluster-mode.md): each run becomes a
// k6-operator TestRun with `parallelism` runner pods, each executing one
// execution segment of the script. The engine:
//
//   - puts the script in a ConfigMap and the run's environment (variables,
//     secrets, live-control wiring) in a Secret the runners load as env;
//   - follows every runner pod's log, where k6 writes its `--out json`
//     samples (K6_OUT=json=/dev/stdout), and feeds them to the same
//     aggregator a local run uses, so cluster metrics are computed from
//     every sample rather than merged from per-worker summaries;
//   - sends live-control changes to every runner's REST API (port 6565);
//     k6 scales run-wide VU numbers by each runner's segment, and the RPS
//     limit is split by OPENSYNAPSE_INSTANCES in the script.

// KubernetesConfig configures cluster runs through the k6-operator.
type KubernetesConfig struct {
	Client *kube.Client
	// RunnerImage must contain k6 with xk6-kv (the control-plane image does).
	RunnerImage     string
	ImagePullPolicy string
	// RunnerResources is passed to runner pods unchanged ({"requests": {...}, "limits": {...}}).
	RunnerResources map[string]map[string]string
	// MaxWorkers caps the runner pods per run.
	MaxWorkers int
	// StartTimeout bounds how long runners may take to begin executing.
	StartTimeout time.Duration
}

// k6APIPort is where runner pods serve k6's REST API (a variable for tests).
var k6APIPort = 6565

type runnerPod struct {
	ip         string
	container  string
	streaming  bool
	terminated bool
	exitCode   int
}

type kubeBackend struct {
	cfg    KubernetesConfig
	kc     *kube.Client
	rp     *runProcess
	name   string
	ctx    context.Context
	cancel context.CancelFunc

	mu      sync.Mutex
	pods    map[string]*runnerPod
	streams sync.WaitGroup

	done     chan struct{}
	once     sync.Once
	exitCode int
}

// resourceName is the TestRun's name: DNS-safe and unique per run.
func resourceName(runID string) string {
	id := strings.ToLower(strings.ReplaceAll(runID, "-", ""))
	if len(id) > 12 {
		id = id[:12]
	}
	return "opensynapse-" + id
}

func startKubernetes(cfg KubernetesConfig, rp *runProcess) (*kubeBackend, error) {
	if cfg.StartTimeout <= 0 {
		cfg.StartTimeout = 5 * time.Minute
	}
	kc := cfg.Client
	name := resourceName(rp.spec.RunID)
	labels := map[string]string{
		"app.kubernetes.io/managed-by": "opensynapse",
		"opensynapse.dev/run-id":       rp.spec.RunID,
	}

	env := runEnv(rp.spec)
	for _, kv := range k6Settings {
		k, v, _ := strings.Cut(kv, "=")
		env[k] = v
	}
	env["K6_OUT"] = "json=/dev/stdout"
	env["K6_NO_SUMMARY"] = "true"
	env["K6_NO_COLOR"] = "true"

	pod := map[string]any{
		"image":    cfg.RunnerImage,
		"envFrom":  []any{map[string]any{"secretRef": map[string]any{"name": name + "-env"}}},
		"metadata": map[string]any{"labels": labels},
	}
	if cfg.ImagePullPolicy != "" {
		pod["imagePullPolicy"] = cfg.ImagePullPolicy
	}
	if cfg.RunnerResources != nil {
		pod["resources"] = cfg.RunnerResources
	}

	resources := []struct {
		path string
		obj  map[string]any
	}{
		{kc.Path("", "v1", "secrets", ""), map[string]any{
			"apiVersion": "v1", "kind": "Secret", "type": "Opaque",
			"metadata":   map[string]any{"name": name + "-env", "labels": labels},
			"stringData": env,
		}},
		{kc.Path("", "v1", "configmaps", ""), map[string]any{
			"apiVersion": "v1", "kind": "ConfigMap",
			"metadata": map[string]any{"name": name + "-script", "labels": labels},
			"data":     map[string]string{"script.js": rp.spec.Script},
		}},
		{kc.Path("k6.io", "v1alpha1", "testruns", ""), map[string]any{
			"apiVersion": "k6.io/v1alpha1", "kind": "TestRun",
			"metadata": map[string]any{"name": name, "labels": labels},
			"spec": map[string]any{
				"parallelism": rp.spec.Instances,
				"script":      map[string]any{"configMap": map[string]any{"name": name + "-script", "file": "script.js"}},
				"arguments":   "--quiet",
				"runner":      pod,
				"initializer": pod,
			},
		}},
	}

	ctx, cancel := context.WithCancel(context.Background())
	b := &kubeBackend{cfg: cfg, kc: kc, rp: rp, name: name, ctx: ctx, cancel: cancel, pods: map[string]*runnerPod{}, done: make(chan struct{})}
	createCtx, done := context.WithTimeout(ctx, 30*time.Second)
	defer done()
	for _, r := range resources {
		if err := kc.Do(createCtx, http.MethodPost, r.path, r.obj, nil); err != nil {
			b.cleanup()
			return nil, fmt.Errorf("create %s for the run: %w", r.obj["kind"], err)
		}
	}
	rp.addLog(fmt.Sprintf("Submitted TestRun %s/%s with %d worker(s)", kc.Namespace(), name, rp.spec.Instances))
	go b.watch()
	return b, nil
}

// k8s object shapes the watch loop reads.
type podList struct {
	Items []struct {
		Metadata struct {
			Name   string            `json:"name"`
			Labels map[string]string `json:"labels"`
		} `json:"metadata"`
		Status struct {
			Phase             string            `json:"phase"`
			PodIP             string            `json:"podIP"`
			ContainerStatuses []containerStatus `json:"containerStatuses"`
		} `json:"status"`
	} `json:"items"`
}

func (b *kubeBackend) listPods(selector string) (*podList, error) {
	var pl podList
	err := b.kc.Do(b.ctx, http.MethodGet, b.kc.Path("", "v1", "pods", "")+"?labelSelector="+url.QueryEscape(selector), nil, &pl)
	return &pl, err
}

// watch follows the TestRun and its runner pods once a second until the run ends.
func (b *kubeBackend) watch() {
	deadline := time.Now().Add(b.cfg.StartTimeout)
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	lastWaiting := map[string]string{}
	for {
		select {
		case <-b.ctx.Done():
			b.finish(-1)
			return
		case <-ticker.C:
		}

		var tr struct {
			Status struct {
				Stage string `json:"stage"`
			} `json:"status"`
		}
		err := b.kc.Do(b.ctx, http.MethodGet, b.kc.Path("k6.io", "v1alpha1", "testruns", b.name), nil, &tr)
		if kube.IsNotFound(err) {
			// Deleted: killed, or removed by someone else.
			b.waitStreams(10 * time.Second)
			b.finish(-1)
			return
		}
		if err != nil {
			log.Printf("run %s: read TestRun: %v", b.rp.spec.RunID, err)
			continue
		}

		pods, err := b.listPods("k6_cr=" + b.name + ",runner=true")
		if err == nil {
			for _, p := range pods.Items {
				b.track(p.Metadata.Name, p.Status.PodIP, p.Status.ContainerStatuses, lastWaiting)
			}
		}

		switch tr.Status.Stage {
		case "error":
			b.reportInitializer()
			b.finish(104)
			return
		case "started", "stopped", "finished":
			b.rp.markStarted()
		}

		if !b.rp.isStarted() && time.Now().After(deadline) {
			b.rp.addLog(fmt.Sprintf("level=error msg=\"workers did not start within %s (TestRun stage %q)\"", b.cfg.StartTimeout, tr.Status.Stage))
			b.finish(-1)
			return
		}

		b.mu.Lock()
		terminated := 0
		codes := make([]int, 0, len(b.pods))
		for _, p := range b.pods {
			if p.terminated {
				terminated++
				codes = append(codes, p.exitCode)
			}
		}
		b.mu.Unlock()
		if terminated >= b.rp.spec.Instances {
			b.waitStreams(30 * time.Second)
			b.finish(combineExitCodes(codes))
			return
		}
	}
}

type containerStatus struct {
	Name  string `json:"name"`
	State struct {
		Waiting *struct {
			Reason  string `json:"reason"`
			Message string `json:"message"`
		} `json:"waiting"`
		Running    *struct{} `json:"running"`
		Terminated *struct {
			ExitCode int    `json:"exitCode"`
			Reason   string `json:"reason"`
		} `json:"terminated"`
	} `json:"state"`
}

// track records a runner pod's address and exit code, and starts following
// its log once its container has started.
func (b *kubeBackend) track(name, ip string, statuses []containerStatus, lastWaiting map[string]string) {
	b.mu.Lock()
	p := b.pods[name]
	if p == nil {
		p = &runnerPod{}
		b.pods[name] = p
	}
	p.ip = ip
	startFollow := false
	if len(statuses) > 0 {
		cs := statuses[0]
		p.container = cs.Name
		if w := cs.State.Waiting; w != nil && w.Reason != "" && lastWaiting[name] != w.Reason {
			// Surface scheduling problems (ImagePullBackOff…) in the run's log.
			lastWaiting[name] = w.Reason
			b.mu.Unlock()
			b.rp.addLog(fmt.Sprintf("%s: waiting: %s %s", name, w.Reason, w.Message))
			b.mu.Lock()
		}
		if (cs.State.Running != nil || cs.State.Terminated != nil) && !p.streaming {
			p.streaming, startFollow = true, true
		}
		if t := cs.State.Terminated; t != nil {
			p.terminated, p.exitCode = true, t.ExitCode
		}
	}
	container := p.container
	b.mu.Unlock()
	if startFollow {
		b.streams.Add(1)
		go b.follow(name, container)
	}
}

func (b *kubeBackend) isTerminated(pod string) bool {
	b.mu.Lock()
	defer b.mu.Unlock()
	p := b.pods[pod]
	return p != nil && p.terminated
}

// follow reads a runner's log to the end, reconnecting (and skipping lines
// already seen) if the API server closes a long-running stream.
func (b *kubeBackend) follow(pod, container string) {
	defer b.streams.Done()
	// last is the newest line's timestamp and atLast how many lines carried
	// it. Lines from one container write share a timestamp, so a resumed
	// stream (sinceTime has second precision) skips lines older than last
	// and the first atLast lines stamped exactly last.
	var last time.Time
	atLast := 0
	failures := 0
	for b.ctx.Err() == nil {
		rc, err := b.kc.FollowLogs(b.ctx, pod, container, last)
		if err != nil {
			if kube.IsNotFound(err) || failures >= 30 {
				return
			}
			failures++
			time.Sleep(time.Second)
			continue
		}
		resumeAt, skip := last, atLast
		r := bufio.NewReaderSize(rc, 256*1024)
		for {
			line, err := r.ReadBytes('\n')
			if len(line) > 0 {
				ts, rest := splitTimestamp(line)
				if !resumeAt.IsZero() && !ts.IsZero() {
					if ts.Before(resumeAt) {
						continue
					}
					if ts.Equal(resumeAt) && skip > 0 {
						skip--
						continue
					}
				}
				if ts.Equal(last) {
					atLast++
				} else if !ts.IsZero() {
					last, atLast = ts, 1
				}
				b.handleLine(pod, rest)
			}
			if err != nil {
				break
			}
		}
		rc.Close()
		if b.isTerminated(pod) {
			return
		}
		time.Sleep(time.Second)
	}
}

// splitTimestamp separates the RFC 3339 timestamp the log API prefixes.
func splitTimestamp(line []byte) (time.Time, []byte) {
	i := bytes.IndexByte(line, ' ')
	if i <= 0 {
		return time.Time{}, line
	}
	ts, err := time.Parse(time.RFC3339Nano, string(line[:i]))
	if err != nil {
		return time.Time{}, line
	}
	return ts, line[i+1:]
}

func (b *kubeBackend) handleLine(pod string, line []byte) {
	trimmed := bytes.TrimSpace(line)
	if len(trimmed) == 0 {
		return
	}
	if trimmed[0] == '{' {
		b.rp.ingest(pod, trimmed)
		return
	}
	b.rp.addLog(pod + ": " + string(trimmed))
}

// reportInitializer copies the initializer's output (usually a script error)
// into the run's log so the failure message explains it.
func (b *kubeBackend) reportInitializer() {
	pods, err := b.listPods("k6_cr=" + b.name)
	if err != nil {
		return
	}
	for _, p := range pods.Items {
		if p.Metadata.Labels["runner"] == "true" || p.Metadata.Labels["starter"] == "true" {
			continue
		}
		out, err := b.kc.TailLogs(b.ctx, p.Metadata.Name, 20)
		if err != nil {
			continue
		}
		for _, l := range strings.Split(strings.TrimSpace(out), "\n") {
			if l != "" {
				b.rp.addLog(p.Metadata.Name + ": " + l)
			}
		}
	}
}

func (b *kubeBackend) waitStreams(limit time.Duration) {
	done := make(chan struct{})
	go func() {
		b.streams.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(limit):
	}
}

func (b *kubeBackend) finish(code int) {
	b.once.Do(func() {
		b.exitCode = code
		close(b.done)
	})
}

func (b *kubeBackend) apis() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	var out []string
	for _, p := range b.pods {
		if p.ip != "" && !p.terminated {
			out = append(out, fmt.Sprintf("http://%s:%d", p.ip, k6APIPort))
		}
	}
	sort.Strings(out)
	return out
}

func (b *kubeBackend) wait() int {
	<-b.done
	return b.exitCode
}

// interrupt has no gentler form in a cluster than deleting the TestRun.
func (b *kubeBackend) interrupt() error { return b.kill() }

func (b *kubeBackend) kill() error {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return b.kc.Delete(ctx, b.kc.Path("k6.io", "v1alpha1", "testruns", b.name))
}

// cleanup deletes the TestRun (its jobs and pods follow) and the run's
// ConfigMap and Secret.
func (b *kubeBackend) cleanup() {
	b.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	for _, p := range []string{
		b.kc.Path("k6.io", "v1alpha1", "testruns", b.name),
		b.kc.Path("", "v1", "configmaps", b.name+"-script"),
		b.kc.Path("", "v1", "secrets", b.name+"-env"),
	} {
		if err := b.kc.Delete(ctx, p); err != nil {
			log.Printf("run %s: clean up %s: %v", b.rp.spec.RunID, p, err)
		}
	}
}

// CleanupOrphans deletes TestRuns (and their ConfigMaps and Secrets) left by
// a control plane that stopped mid-run; those runs are marked interrupted at
// startup, so their workers would otherwise keep generating load.
func CleanupOrphans(ctx context.Context, kc *kube.Client) (int, error) {
	selector := "?labelSelector=" + url.QueryEscape("app.kubernetes.io/managed-by=opensynapse")
	var list struct {
		Items []any `json:"items"`
	}
	if err := kc.Do(ctx, http.MethodGet, kc.Path("k6.io", "v1alpha1", "testruns", "")+selector, nil, &list); err != nil {
		return 0, err
	}
	for _, p := range []string{
		kc.Path("k6.io", "v1alpha1", "testruns", ""),
		kc.Path("", "v1", "configmaps", ""),
		kc.Path("", "v1", "secrets", ""),
	} {
		if err := kc.Delete(ctx, p+selector); err != nil {
			return len(list.Items), err
		}
	}
	return len(list.Items), nil
}
