// Package output is a k6 output extension ("--out opensynapse") that
// aggregates samples on the k6 instance and posts one compact batch per
// second to the OpenSynapse control plane. Cluster runs use it instead of
// streaming every sample, which does not scale past a few thousand
// requests per second (docs/decisions/0012-cluster-mode.md).
//
// Configuration comes from the environment the control plane gives each
// runner pod: OPENSYNAPSE_INGEST_URL, OPENSYNAPSE_CONTROL_TOKEN and HOSTNAME
// (the instance name). The extension contacts nothing else.
package output

import (
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"net/http"
	"strconv"
	"sync"
	"time"

	"go.k6.io/k6/metrics"
	"go.k6.io/k6/output"
)

func init() {
	output.RegisterExtension("opensynapse", New)
}

// controlScenario is the internal live-control scenario; its samples are not results.
const controlScenario = "__opensynapse_control"

const maxErrorsPerBatch = 50

// Batch is what one instance reports about once a second. The control plane
// (internal/engine/ingest.go) decodes the same shape.
type Batch struct {
	Instance string `json:"instance"`
	// Through is the time up to which this instance has reported all samples.
	Through   time.Time  `json:"through"`
	Seconds   []Second   `json:"seconds,omitempty"`
	Endpoints []Endpoint `json:"endpoints,omitempty"`
	Checks    []Check    `json:"checks,omitempty"`
	Errors    []Error    `json:"errors,omitempty"`
	// Final is set on the last batch, sent when the instance stops.
	Final bool `json:"final,omitempty"`
}

// Second holds the counts for one wall-clock second.
type Second struct {
	Unix          int64   `json:"t"`
	Requests      int64   `json:"requests,omitempty"`
	Failed        int64   `json:"failed,omitempty"`
	Iterations    int64   `json:"iterations,omitempty"`
	ChecksPassed  int64   `json:"checks_passed,omitempty"`
	ChecksFailed  int64   `json:"checks_failed,omitempty"`
	BytesSent     float64 `json:"bytes_sent,omitempty"`
	BytesReceived float64 `json:"bytes_received,omitempty"`
	// VUs is the instance's last VU gauge reading in this second, if any.
	VUs       *int64 `json:"vus,omitempty"`
	Durations *Hist  `json:"durations,omitempty"`
}

// Hist is a log-bucketed latency histogram (bucket = round(ln(ms) * 200)),
// identical to the control plane's, so histograms from several instances
// merge exactly.
type Hist struct {
	Buckets map[int32]int64 `json:"b"`
	Count   int64           `json:"n"`
	Sum     float64         `json:"s"`
	Min     float64         `json:"min"`
	Max     float64         `json:"max"`
}

func newHist() *Hist { return &Hist{Buckets: map[int32]int64{}, Min: math.Inf(1)} }

func (h *Hist) add(v float64) {
	if v < 0 {
		v = 0
	}
	h.Count++
	h.Sum += v
	h.Min = math.Min(h.Min, v)
	h.Max = math.Max(h.Max, v)
	h.Buckets[BucketOf(v)]++
}

// BucketOf is the histogram bucket for a duration in milliseconds.
func BucketOf(v float64) int32 {
	if v < 0.001 {
		v = 0.001
	}
	return int32(math.Round(math.Log(v) * 200))
}

// Endpoint holds one request name's deltas since the previous batch.
type Endpoint struct {
	Method    string `json:"method"`
	Name      string `json:"name"`
	Failed    int64  `json:"failed,omitempty"`
	Durations *Hist  `json:"durations,omitempty"`
}

// Check holds one assertion's deltas since the previous batch.
type Check struct {
	Group  string `json:"group"`
	Name   string `json:"name"`
	Passes int64  `json:"passes,omitempty"`
	Fails  int64  `json:"fails,omitempty"`
}

// Error is one failed request (a sample of them, not all).
type Error struct {
	TimestampMS int64  `json:"timestamp_ms"`
	Status      int    `json:"status"`
	Method      string `json:"method"`
	Name        string `json:"name"`
	URL         string `json:"url"`
	ErrorCode   string `json:"error_code"`
	Message     string `json:"message"`
}

// Output implements k6's output.Output.
type Output struct {
	output.SampleBuffer

	url      string
	token    string
	instance string
	client   *http.Client
	flusher  *output.PeriodicFlusher
	now      func() time.Time

	mu      sync.Mutex
	pending *Batch // samples folded in but not yet sent (kept on send failure)
}

// New is the extension's constructor.
func New(p output.Params) (output.Output, error) {
	url := p.Environment["OPENSYNAPSE_INGEST_URL"]
	if url == "" {
		return nil, errors.New("opensynapse output: OPENSYNAPSE_INGEST_URL is not set")
	}
	instance := p.Environment["HOSTNAME"]
	if instance == "" {
		instance = "k6"
	}
	return &Output{
		url:      url,
		token:    p.Environment["OPENSYNAPSE_CONTROL_TOKEN"],
		instance: instance,
		client:   &http.Client{Timeout: 10 * time.Second},
		now:      time.Now,
	}, nil
}

func (o *Output) Description() string { return "opensynapse (" + o.url + ")" }

func (o *Output) Start() error {
	f, err := output.NewPeriodicFlusher(time.Second, o.flush)
	if err != nil {
		return err
	}
	o.flusher = f
	return nil
}

func (o *Output) Stop() error {
	o.flusher.Stop() // runs a last flush
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.pending == nil {
		o.pending = &Batch{}
	}
	o.pending.Final = true
	o.pending.Through = o.now()
	return o.sendLocked()
}

// fold adds samples to a batch.
func fold(b *Batch, containers []metrics.SampleContainer) {
	seconds := map[int64]*Second{}
	for i := range b.Seconds {
		seconds[b.Seconds[i].Unix] = &b.Seconds[i]
	}
	endpoints := map[string]*Endpoint{}
	for i := range b.Endpoints {
		e := &b.Endpoints[i]
		endpoints[e.Method+" "+e.Name] = e
	}
	checks := map[string]*Check{}
	for i := range b.Checks {
		c := &b.Checks[i]
		checks[c.Group+"\x00"+c.Name] = c
	}
	second := func(t time.Time) *Second {
		s := seconds[t.Unix()]
		if s == nil {
			b.Seconds = append(b.Seconds, Second{Unix: t.Unix()})
			// Re-index: append may have moved the slice.
			for i := range b.Seconds {
				seconds[b.Seconds[i].Unix] = &b.Seconds[i]
			}
			s = seconds[t.Unix()]
		}
		return s
	}
	tag := func(s metrics.Sample, name string) string {
		if v, ok := s.Tags.Get(name); ok {
			return v
		}
		return s.Metadata[name]
	}
	endpoint := func(s metrics.Sample) *Endpoint {
		method, name := tag(s, "method"), tag(s, "name")
		if name == "" {
			name = tag(s, "url")
		}
		e := endpoints[method+" "+name]
		if e == nil {
			b.Endpoints = append(b.Endpoints, Endpoint{Method: method, Name: name})
			for i := range b.Endpoints {
				endpoints[b.Endpoints[i].Method+" "+b.Endpoints[i].Name] = &b.Endpoints[i]
			}
			e = endpoints[method+" "+name]
		}
		return e
	}

	for _, sc := range containers {
		for _, s := range sc.GetSamples() {
			if s.Metric == nil {
				continue
			}
			if tag(s, "scenario") == controlScenario {
				continue
			}
			sec := second(s.Time)
			switch s.Metric.Name {
			case "http_reqs":
				sec.Requests++
			case "http_req_duration":
				if sec.Durations == nil {
					sec.Durations = newHist()
				}
				sec.Durations.add(s.Value)
				e := endpoint(s)
				if e.Durations == nil {
					e.Durations = newHist()
				}
				e.Durations.add(s.Value)
			case "http_req_failed":
				if s.Value != 0 {
					sec.Failed++
					endpoint(s).Failed++
					if len(b.Errors) < maxErrorsPerBatch {
						status, _ := strconv.Atoi(tag(s, "status"))
						name := tag(s, "name")
						if name == "" {
							name = tag(s, "url")
						}
						b.Errors = append(b.Errors, Error{
							TimestampMS: s.Time.UnixMilli(), Status: status, Method: tag(s, "method"), Name: name,
							URL: tag(s, "url"), ErrorCode: tag(s, "error_code"), Message: tag(s, "error"),
						})
					}
				}
			case "iterations":
				sec.Iterations++
			case "checks":
				key := tag(s, "group") + "\x00" + tag(s, "check")
				c := checks[key]
				if c == nil {
					b.Checks = append(b.Checks, Check{Group: tag(s, "group"), Name: tag(s, "check")})
					for i := range b.Checks {
						checks[b.Checks[i].Group+"\x00"+b.Checks[i].Name] = &b.Checks[i]
					}
					c = checks[key]
				}
				if s.Value != 0 {
					sec.ChecksPassed++
					c.Passes++
				} else {
					sec.ChecksFailed++
					c.Fails++
				}
			case "data_sent":
				sec.BytesSent += s.Value
			case "data_received":
				sec.BytesReceived += s.Value
			case "vus":
				v := int64(s.Value)
				sec.VUs = &v
			}
		}
	}
}

func (o *Output) flush() {
	samples := o.GetBufferedSamples()
	o.mu.Lock()
	defer o.mu.Unlock()
	if o.pending == nil {
		o.pending = &Batch{}
	}
	fold(o.pending, samples)
	// k6 hands samples to outputs within moments of producing them, so
	// everything older than a second has arrived.
	o.pending.Through = o.now().Add(-time.Second)
	if err := o.sendLocked(); err != nil {
		// Keep the batch and retry with the next flush; nothing is lost while
		// the control plane is briefly unreachable.
		fmt.Printf("opensynapse output: %v (will retry)\n", err)
	}
}

func (o *Output) sendLocked() error {
	b := o.pending
	b.Instance = o.instance
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if err := json.NewEncoder(zw).Encode(b); err != nil {
		return err
	}
	zw.Close()
	req, err := http.NewRequest(http.MethodPost, o.url, &buf)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Content-Encoding", "gzip")
	req.Header.Set("X-OpenSynapse-Run-Token", o.token)
	resp, err := o.client.Do(req)
	if err != nil {
		return err
	}
	resp.Body.Close()
	if resp.StatusCode >= 400 && resp.StatusCode < 500 && resp.StatusCode != http.StatusTooManyRequests {
		// The run is over or the token is wrong: retrying cannot help.
		o.pending = nil
		return fmt.Errorf("control plane rejected the batch (%d)", resp.StatusCode)
	}
	if resp.StatusCode >= 300 {
		return fmt.Errorf("control plane answered %d", resp.StatusCode)
	}
	o.pending = nil
	return nil
}
