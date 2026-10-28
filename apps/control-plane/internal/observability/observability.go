// Package observability gives the control plane structured JSON logs (with
// a recent-lines buffer for the in-app log viewer) and Prometheus metrics
// about itself at /metrics.
package observability

import (
	"bytes"
	"fmt"
	"io"
	"log"
	"log/slog"
	"net/http"
	"os"
	"sort"
	"strings"
	"sync"
	"time"
)

// --- Logs ---

// LogBuffer keeps the most recent log lines in memory.
type LogBuffer struct {
	mu    sync.Mutex
	lines []string
	max   int
}

func (b *LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	for _, l := range strings.Split(strings.TrimRight(string(p), "\n"), "\n") {
		b.lines = append(b.lines, l)
	}
	if over := len(b.lines) - b.max; over > 0 {
		b.lines = b.lines[over:]
	}
	return len(p), nil
}

// Lines returns a copy of the buffered lines, oldest first.
func (b *LogBuffer) Lines() []string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]string(nil), b.lines...)
}

// Logs is the process-wide log buffer.
var Logs = &LogBuffer{max: 1000}

// SetupLogging routes slog and the standard log package to JSON on stdout
// (or text when OPENSYNAPSE_LOG_FORMAT=text) and into the in-memory buffer.
func SetupLogging() {
	out := io.MultiWriter(os.Stdout, Logs)
	var h slog.Handler
	if os.Getenv("OPENSYNAPSE_LOG_FORMAT") == "text" {
		h = slog.NewTextHandler(out, nil)
	} else {
		h = slog.NewJSONHandler(out, nil)
	}
	slog.SetDefault(slog.New(h))
	log.SetFlags(0)
}

// --- Metrics ---

type counterKey struct {
	route, method string
	code          int
}

// Metrics is a tiny Prometheus registry for the control plane.
type Metrics struct {
	mu       sync.Mutex
	requests map[counterKey]int64
	latency  map[string]float64 // route -> total seconds
	runs     map[string]int64   // finished runs by status
	gauges   map[string]func() float64
}

// Default is the process-wide registry.
var Default = &Metrics{
	requests: map[counterKey]int64{},
	latency:  map[string]float64{},
	runs:     map[string]int64{},
	gauges:   map[string]func() float64{},
}

// Gauge registers a value read at scrape time.
func (m *Metrics) Gauge(name string, f func() float64) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.gauges[name] = f
}

// RunFinished counts a finished run.
func (m *Metrics) RunFinished(status string) {
	m.mu.Lock()
	m.runs[status]++
	m.mu.Unlock()
}

type statusRecorder struct {
	http.ResponseWriter
	code int
}

func (s *statusRecorder) WriteHeader(c int) { s.code = c; s.ResponseWriter.WriteHeader(c) }

// Hijack/Flush pass-throughs keep WebSockets and streaming working.
func (s *statusRecorder) Unwrap() http.ResponseWriter { return s.ResponseWriter }

// Instrument wraps a route's handler to count requests and time them.
func (m *Metrics) Instrument(route string, h http.HandlerFunc) http.HandlerFunc {
	if strings.HasSuffix(route, "/ws") {
		return h // long-lived; timing would be meaningless
	}
	return func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, code: 200}
		h(rec, r)
		m.mu.Lock()
		m.requests[counterKey{route: route, method: r.Method, code: rec.code}]++
		m.latency[route] += time.Since(start).Seconds()
		m.mu.Unlock()
	}
}

// Handler serves the Prometheus text exposition format.
func (m *Metrics) Handler(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var b bytes.Buffer
	b.WriteString("# HELP opensynapse_http_requests_total API requests by route, method and status.\n# TYPE opensynapse_http_requests_total counter\n")
	keys := make([]counterKey, 0, len(m.requests))
	for k := range m.requests {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if keys[i].route != keys[j].route {
			return keys[i].route < keys[j].route
		}
		return keys[i].code < keys[j].code
	})
	for _, k := range keys {
		fmt.Fprintf(&b, "opensynapse_http_requests_total{route=%q,method=%q,code=\"%d\"} %d\n", k.route, k.method, k.code, m.requests[k])
	}
	b.WriteString("# HELP opensynapse_http_request_seconds_total Time spent serving each route.\n# TYPE opensynapse_http_request_seconds_total counter\n")
	routes := make([]string, 0, len(m.latency))
	for r := range m.latency {
		routes = append(routes, r)
	}
	sort.Strings(routes)
	for _, r := range routes {
		fmt.Fprintf(&b, "opensynapse_http_request_seconds_total{route=%q} %g\n", r, m.latency[r])
	}
	b.WriteString("# HELP opensynapse_runs_finished_total Runs that finished, by status.\n# TYPE opensynapse_runs_finished_total counter\n")
	for _, s := range []string{"completed", "failed", "aborted"} {
		fmt.Fprintf(&b, "opensynapse_runs_finished_total{status=%q} %d\n", s, m.runs[s])
	}
	names := make([]string, 0, len(m.gauges))
	for n := range m.gauges {
		names = append(names, n)
	}
	sort.Strings(names)
	for _, n := range names {
		fmt.Fprintf(&b, "# TYPE %s gauge\n%s %g\n", n, n, m.gauges[n]())
	}
	w.Header().Set("Content-Type", "text/plain; version=0.0.4")
	w.Write(b.Bytes())
}
