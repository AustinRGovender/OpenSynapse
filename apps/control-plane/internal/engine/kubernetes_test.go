package engine

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/kube"
)

// fakeCluster imitates the API server and the k6-operator for one TestRun
// with two runner pods, and the runners' k6 REST APIs.
type fakeCluster struct {
	mu       sync.Mutex
	created  map[string]map[string]any // kind → object
	deleted  []string
	polls    int
	served   map[string]bool // pod logs fully served
	patches  []string
	logLines map[string][]string
}

func (f *fakeCluster) handler(t *testing.T) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		defer f.mu.Unlock()
		if r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("missing service-account token on %s", r.URL.Path)
		}
		switch {
		case r.Method == http.MethodPost:
			var obj map[string]any
			json.NewDecoder(r.Body).Decode(&obj)
			f.created[obj["kind"].(string)] = obj
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte("{}"))
		case r.Method == http.MethodDelete:
			f.deleted = append(f.deleted, r.URL.Path)
			w.Write([]byte("{}"))
		case strings.HasSuffix(r.URL.Path, "/testruns/opensynapse-run1abcdef12"):
			f.polls++
			stage := "initialization"
			if f.polls > 1 {
				stage = "started"
			}
			fmt.Fprintf(w, `{"status":{"stage":%q}}`, stage)
		case strings.HasSuffix(r.URL.Path, "/pods") && r.URL.Query().Get("labelSelector") == "k6_cr=opensynapse-run1abcdef12,runner=true":
			var items []string
			for _, pod := range []string{"runner-1", "runner-2"} {
				state := `{"running":{}}`
				if f.served[pod] {
					state = `{"terminated":{"exitCode":0}}`
				}
				items = append(items, fmt.Sprintf(`{"metadata":{"name":%q,"labels":{"runner":"true"}},"status":{"phase":"Running","podIP":"127.0.0.1","containerStatuses":[{"name":"k6","state":%s}]}}`, pod, state))
			}
			fmt.Fprintf(w, `{"items":[%s]}`, strings.Join(items, ","))
		case strings.HasSuffix(r.URL.Path, "/log"):
			pod := strings.TrimSuffix(strings.TrimPrefix(r.URL.Path, "/api/v1/namespaces/load/pods/"), "/log")
			if r.URL.Query().Get("container") != "k6" || r.URL.Query().Get("timestamps") != "true" {
				t.Errorf("log request %s", r.URL.RawQuery)
			}
			for _, l := range f.logLines[pod] {
				fmt.Fprintln(w, l)
			}
			f.served[pod] = true
		default:
			http.NotFound(w, r)
		}
	})
}

func point(metric string, ts time.Time, value float64, tags string) string {
	return fmt.Sprintf(`%s {"type":"Point","metric":%q,"data":{"time":%q,"value":%v,"tags":{%s}}}`,
		ts.Add(time.Millisecond).Format(time.RFC3339Nano), metric, ts.Format(time.RFC3339Nano), value, tags)
}

func TestKubernetesBackendAggregatesWorkersAndFansOutControl(t *testing.T) {
	now := time.Now().Truncate(time.Second).Add(-5 * time.Second)
	req := `"method":"GET","name":"GET /products","status":"200","scenario":"default"`
	f := &fakeCluster{
		created: map[string]map[string]any{},
		served:  map[string]bool{},
		logLines: map[string][]string{
			// Each runner: its own VU gauge (including one control VU) and requests.
			"runner-1": {
				now.Format(time.RFC3339Nano) + ` time="..." level=info msg="k6 starting"`,
				point("vus", now, 3, ""),
				point("http_reqs", now, 1, req),
				point("http_req_duration", now, 10, req),
				point("http_reqs", now.Add(time.Second), 1, req),
				point("http_req_duration", now.Add(time.Second), 20, req),
			},
			"runner-2": {
				point("vus", now, 4, ""),
				point("http_reqs", now, 1, req),
				point("http_req_duration", now, 30, req),
				point("http_reqs", now.Add(time.Second), 1, `"method":"GET","name":"GET /products","status":"500","scenario":"default"`),
				point("http_req_failed", now.Add(time.Second), 1, `"method":"GET","name":"GET /products","status":"500","scenario":"default"`),
				point("http_req_duration", now.Add(time.Second), 40, req),
			},
		},
	}
	api := httptest.NewServer(f.handler(t))
	defer api.Close()

	// Both runners' REST APIs resolve to this server (same IP and port).
	k6api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		f.mu.Lock()
		f.patches = append(f.patches, string(b))
		f.mu.Unlock()
		w.Write([]byte(`{}`))
	}))
	defer k6api.Close()
	_, port, _ := net.SplitHostPort(strings.TrimPrefix(k6api.URL, "http://"))
	old := k6APIPort
	k6APIPort, _ = strconv.Atoi(port)
	defer func() { k6APIPort = old }()

	e := &Engine{active: map[string]*runProcess{}}
	e.UseKubernetes(KubernetesConfig{
		Client:      kube.NewForTest(api.URL, "load", "test-token", api.Client()),
		RunnerImage: "opensynapse/control-plane:test",
		MaxWorkers:  4,
	})

	results := make(chan Result, 1)
	var peak int
	err := e.StartRun(RunSpec{
		RunID:        "run1abcdef12-0000",
		Script:       "export default function () {}",
		Env:          map[string]string{"BASE_URL": "http://shop"},
		Instances:    2,
		Schedule:     &Schedule{StartVUs: 5},
		MaxVUs:       10,
		ControlURL:   "http://control-plane:8090/api/v1/runs/run1/control-state",
		ControlToken: "tok",
	}, Callbacks{
		OnMetrics: func(_ []db.MetricPoint, s LiveState) {
			if s.ActiveVUs > peak {
				peak = s.ActiveVUs
			}
		},
		OnComplete: func(r Result) { results <- r },
	})
	if err != nil {
		t.Fatal(err)
	}

	// Live control reaches every runner once they are running.
	deadline := time.Now().Add(5 * time.Second)
	for {
		if err := e.SetVUs("run1abcdef12-0000", 8); err == nil {
			break
		} else if time.Now().After(deadline) {
			t.Fatalf("SetVUs never succeeded: %v", err)
		}
		time.Sleep(200 * time.Millisecond)
	}

	var res Result
	select {
	case res = <-results:
	case <-time.After(15 * time.Second):
		t.Fatal("run did not finish")
	}

	if res.Status != "completed" || res.Summary == nil {
		t.Fatalf("result = %+v", res)
	}
	if res.Summary.TotalRequests != 4 || res.Summary.FailedRequests != 1 {
		t.Errorf("requests = %d (failed %d), want 4 (1) from both workers", res.Summary.TotalRequests, res.Summary.FailedRequests)
	}
	if res.Summary.PeakVUs != 5 {
		t.Errorf("peak VUs = %d, want 5 (2 + 3, without each worker's control VU)", res.Summary.PeakVUs)
	}

	// Cleanup runs right after the completion callback.
	for i := 0; i < 50; i++ {
		f.mu.Lock()
		n := len(f.deleted)
		f.mu.Unlock()
		if n >= 3 {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}

	f.mu.Lock()
	defer f.mu.Unlock()
	tr := f.created["TestRun"]["spec"].(map[string]any)
	if tr["parallelism"].(float64) != 2 {
		t.Errorf("parallelism = %v", tr["parallelism"])
	}
	env := f.created["Secret"]["stringData"].(map[string]any)
	for k, want := range map[string]string{"BASE_URL": "http://shop", "OPENSYNAPSE_INSTANCES": "2", "OPENSYNAPSE_CONTROL_TOKEN": "tok", "K6_OUT": "json=/dev/stdout", "K6_NO_USAGE_REPORT": "true"} {
		if env[k] != want {
			t.Errorf("runner env %s = %v, want %q", k, env[k], want)
		}
	}
	if script := f.created["ConfigMap"]["data"].(map[string]any)["script.js"]; script != "export default function () {}" {
		t.Errorf("script = %v", script)
	}
	vuPatches := 0
	for _, p := range f.patches {
		if strings.Contains(p, `"vus":8`) {
			vuPatches++
		}
	}
	if vuPatches != 2 {
		t.Errorf("VU change sent %d times, want once per runner (2): %v", vuPatches, f.patches)
	}
	if len(f.deleted) < 3 {
		t.Errorf("cleanup deleted %v, want the TestRun, ConfigMap and Secret", f.deleted)
	}
}

func TestCombineExitCodes(t *testing.T) {
	for _, c := range []struct {
		codes []int
		want  int
	}{
		{[]int{0, 0}, 0},
		{[]int{0, 103}, 103},
		{[]int{103, 99}, 99},
		{[]int{99, 107, 0}, 107},
	} {
		if got := combineExitCodes(c.codes); got != c.want {
			t.Errorf("combineExitCodes(%v) = %d, want %d", c.codes, got, c.want)
		}
	}
}

// A log stream the API server closes mid-run is resumed from sinceTime
// without losing or repeating samples, even when many lines share one
// timestamp (lines from one container write do).
func TestFollowResumesWithoutLossOrDuplicates(t *testing.T) {
	ts := time.Date(2026, 10, 27, 12, 0, 0, 500, time.UTC)
	stamp := ts.Format(time.RFC3339Nano)
	var lines []string
	for i := 0; i < 6; i++ {
		lines = append(lines, fmt.Sprintf(`%s {"type":"Point","metric":"http_reqs","data":{"time":%q,"value":1,"tags":{}}}`, stamp, ts.Format(time.RFC3339Nano)))
	}
	var mu sync.Mutex
	calls := 0
	terminated := false
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		calls++
		if calls == 1 {
			// First connection: half the lines, then the server hangs up.
			for _, l := range lines[:3] {
				fmt.Fprintln(w, l)
			}
			return
		}
		if r.URL.Query().Get("sinceTime") != "2026-10-27T12:00:00Z" {
			t.Errorf("resumed with sinceTime=%q", r.URL.Query().Get("sinceTime"))
		}
		for _, l := range lines { // everything from that second again
			fmt.Fprintln(w, l)
		}
		terminated = true
	}))
	defer api.Close()

	rp := &runProcess{spec: RunSpec{RunID: "r"}, agg: newAggregator(0)}
	b := &kubeBackend{kc: kube.NewForTest(api.URL, "load", "t", api.Client()), rp: rp, pods: map[string]*runnerPod{"p": {}}}
	b.ctx, b.cancel = context.WithCancel(context.Background())
	defer b.cancel()
	go func() {
		// The watch loop would notice the container ending.
		for {
			mu.Lock()
			done := terminated
			mu.Unlock()
			if done {
				b.mu.Lock()
				b.pods["p"].terminated = true
				b.mu.Unlock()
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
	}()
	b.streams.Add(1)
	b.follow("p", "")
	if got := rp.agg.summary().TotalRequests; got != 6 {
		t.Fatalf("ingested %d requests, want exactly 6", got)
	}
}
