package engine

import (
	"encoding/json"
	"math/rand/v2"
	"sort"
	"strconv"
	"sync"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// ControlScenario is the name of the internal scenario that feeds live-control
// state into the xk6-kv store. Its samples are excluded from all results.
const ControlScenario = "__opensynapse_control"

// maxBucketSamples bounds the per-second duration slice; beyond it we keep a
// uniform reservoir so per-second percentiles stay representative.
const maxBucketSamples = 20000

// maxEndpoints bounds per-endpoint tracking; further endpoints fold into "(other)".
const maxEndpoints = 200

// maxErrorSamples is how many recent errors are kept for the error log.
const maxErrorSamples = 50

// k6Line is one line of k6's `--out json` stream.
type k6Line struct {
	Type   string `json:"type"`
	Metric string `json:"metric"`
	Data   struct {
		Time  time.Time         `json:"time"`
		Value float64           `json:"value"`
		Tags  map[string]string `json:"tags"`
	} `json:"data"`
}

type secondBucket struct {
	requests     int64
	failed       int64
	iterations   int64
	checksPassed int64
	checksFailed int64
	bytesSent    float64
	bytesRecv    float64
	durations    []float64
	seen         int64 // durations observed (for reservoir sampling)
	vus          int
	hasVUs       bool
}

type endpointAgg struct {
	method string
	name   string
	failed int64
	hist   *histogram
}

type checkAgg struct {
	name   string
	group  string
	passes int64
	fails  int64
}

// aggregator turns k6's per-sample JSON stream into per-second snapshots, a
// run summary, per-endpoint and per-check breakdowns, and a recent-error log.
type aggregator struct {
	mu sync.Mutex

	buckets     map[int64]*secondBucket
	lastEmitted int64 // unix second of the last emitted snapshot

	total      *histogram
	requests   int64
	failed     int64
	iterations int64
	checksPass int64
	checksFail int64
	bytesSent  float64
	bytesRecv  float64
	firstTS    time.Time
	lastTS     time.Time

	endpoints map[string]*endpointAgg
	checks    map[string]*checkAgg

	errors      []db.RunError
	errorsTotal int64
	newErrors   []db.RunError

	currentVUs int
	peakVUs    int
	// vuOffset is subtracted from k6's VU gauge to hide the internal control VU.
	vuOffset int
}

func newAggregator(vuOffset int) *aggregator {
	return &aggregator{
		buckets:   make(map[int64]*secondBucket),
		total:     newHistogram(),
		endpoints: make(map[string]*endpointAgg),
		checks:    make(map[string]*checkAgg),
		vuOffset:  vuOffset,
	}
}

func (a *aggregator) bucket(sec int64) *secondBucket {
	b := a.buckets[sec]
	if b == nil {
		b = &secondBucket{}
		a.buckets[sec] = b
	}
	return b
}

// ingest parses one JSON line. It reports whether the line carried a sample
// that should be kept in the raw-sample archive.
func (a *aggregator) ingest(line []byte) bool {
	var l k6Line
	if err := json.Unmarshal(line, &l); err != nil || l.Type != "Point" {
		return false
	}
	tags := l.Data.Tags
	if tags["scenario"] == ControlScenario {
		return false
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	ts := l.Data.Time
	if a.firstTS.IsZero() || ts.Before(a.firstTS) {
		a.firstTS = ts
	}
	if ts.After(a.lastTS) {
		a.lastTS = ts
	}
	sec := ts.Unix()
	v := l.Data.Value

	switch l.Metric {
	case "http_reqs":
		a.requests++
		a.bucket(sec).requests++
	case "http_req_duration":
		a.total.add(v)
		b := a.bucket(sec)
		b.seen++
		if len(b.durations) < maxBucketSamples {
			b.durations = append(b.durations, v)
		} else if j := rand.Int64N(b.seen); j < maxBucketSamples {
			b.durations[j] = v
		}
		a.endpoint(tags).hist.add(v)
	case "http_req_failed":
		if v != 0 {
			a.failed++
			a.bucket(sec).failed++
			a.endpoint(tags).failed++
			a.recordError(ts, tags)
		}
	case "iterations":
		a.iterations++
		a.bucket(sec).iterations++
	case "checks":
		c := a.check(tags)
		b := a.bucket(sec)
		if v != 0 {
			a.checksPass++
			b.checksPassed++
			c.passes++
		} else {
			a.checksFail++
			b.checksFailed++
			c.fails++
		}
	case "data_sent":
		a.bytesSent += v
		a.bucket(sec).bytesSent += v
	case "data_received":
		a.bytesRecv += v
		a.bucket(sec).bytesRecv += v
	case "vus":
		n := int(v) - a.vuOffset
		if n < 0 {
			n = 0
		}
		b := a.bucket(sec)
		b.vus, b.hasVUs = n, true
		a.currentVUs = n
		if n > a.peakVUs {
			a.peakVUs = n
		}
	default:
		return false
	}
	return true
}

func endpointKey(tags map[string]string) (method, name string) {
	method = tags["method"]
	name = tags["name"]
	if name == "" {
		name = tags["url"]
	}
	return method, name
}

func (a *aggregator) endpoint(tags map[string]string) *endpointAgg {
	method, name := endpointKey(tags)
	key := method + " " + name
	e := a.endpoints[key]
	if e == nil {
		if len(a.endpoints) >= maxEndpoints {
			key, method, name = "(other)", "", "(other)"
			if e = a.endpoints[key]; e != nil {
				return e
			}
		}
		e = &endpointAgg{method: method, name: name, hist: newHistogram()}
		a.endpoints[key] = e
	}
	return e
}

func (a *aggregator) check(tags map[string]string) *checkAgg {
	key := tags["group"] + "\x00" + tags["check"]
	c := a.checks[key]
	if c == nil {
		c = &checkAgg{name: tags["check"], group: tags["group"]}
		a.checks[key] = c
	}
	return c
}

func (a *aggregator) recordError(ts time.Time, tags map[string]string) {
	a.errorsTotal++
	method, name := endpointKey(tags)
	status, _ := strconv.Atoi(tags["status"])
	e := db.RunError{
		TimestampMS: ts.UnixMilli(),
		Status:      status,
		Method:      method,
		Name:        name,
		URL:         tags["url"],
		ErrorCode:   tags["error_code"],
		Message:     tags["error"],
	}
	a.errors = append(a.errors, e)
	if len(a.errors) > maxErrorSamples {
		a.errors = a.errors[len(a.errors)-maxErrorSamples:]
	}
	if len(a.newErrors) < maxErrorSamples {
		a.newErrors = append(a.newErrors, e)
	}
}

// takeErrors returns errors recorded since the previous call.
func (a *aggregator) takeErrors() []db.RunError {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := a.newErrors
	a.newErrors = nil
	return out
}

// snapshots returns one snapshot per complete second up to (and including)
// `upTo`, filling quiet seconds with zeros so charts stay continuous.
func (a *aggregator) snapshots(upTo int64) []db.MetricPoint {
	a.mu.Lock()
	defer a.mu.Unlock()

	if a.lastEmitted == 0 {
		if a.firstTS.IsZero() {
			return nil
		}
		a.lastEmitted = a.firstTS.Unix() - 1
	}
	var out []db.MetricPoint
	for sec := a.lastEmitted + 1; sec <= upTo; sec++ {
		out = append(out, a.pointFor(sec))
		delete(a.buckets, sec)
		a.lastEmitted = sec
	}
	// Drop any stale buckets that arrived after their second was emitted;
	// their samples are already counted in the run totals.
	for sec := range a.buckets {
		if sec <= a.lastEmitted {
			delete(a.buckets, sec)
		}
	}
	return out
}

func (a *aggregator) pointFor(sec int64) db.MetricPoint {
	p := db.MetricPoint{TimestampMS: sec * 1000, ActiveVUs: a.currentVUs}
	b := a.buckets[sec]
	if b == nil {
		return p
	}
	p.Requests = b.requests
	p.Failed = b.failed
	p.RPS = float64(b.requests)
	if b.requests > 0 {
		p.ErrorRate = float64(b.failed) / float64(b.requests) * 100
	}
	if len(b.durations) > 0 {
		sort.Float64s(b.durations)
		p.P50MS = exactPercentile(b.durations, 0.50)
		p.P90MS = exactPercentile(b.durations, 0.90)
		p.P95MS = exactPercentile(b.durations, 0.95)
		p.P99MS = exactPercentile(b.durations, 0.99)
	}
	p.BytesSent = int64(b.bytesSent)
	p.BytesReceived = int64(b.bytesRecv)
	p.Iterations = b.iterations
	p.ChecksPassed = b.checksPassed
	p.ChecksFailed = b.checksFailed
	if b.hasVUs {
		p.ActiveVUs = b.vus
	}
	return p
}

// summary builds the final run summary from everything ingested.
func (a *aggregator) summary() *db.RunSummary {
	a.mu.Lock()
	defer a.mu.Unlock()

	s := &db.RunSummary{
		TotalRequests:     a.requests,
		FailedRequests:    a.failed,
		Iterations:        a.iterations,
		AssertionPasses:   a.checksPass,
		AssertionFailures: a.checksFail,
		BytesSent:         int64(a.bytesSent),
		BytesReceived:     int64(a.bytesRecv),
		PeakVUs:           a.peakVUs,
		ThresholdsPassed:  true,
		AvgMS:             a.total.avg(),
		P50MS:             a.total.percentile(0.50),
		P90MS:             a.total.percentile(0.90),
		P95MS:             a.total.percentile(0.95),
		P99MS:             a.total.percentile(0.99),
		MaxMS:             a.total.max,
	}
	if a.total.count > 0 {
		s.MinMS = a.total.min
	}
	if a.requests > 0 {
		s.ErrorRate = float64(a.failed) / float64(a.requests) * 100
	}
	if !a.firstTS.IsZero() {
		elapsed := a.lastTS.Sub(a.firstTS).Seconds()
		s.DurationSeconds = elapsed
		if elapsed >= 1 {
			s.ThroughputRPS = float64(a.requests) / elapsed
		} else {
			s.ThroughputRPS = float64(a.requests)
		}
	}

	for _, e := range a.endpoints {
		st := db.EndpointStat{
			Method:   e.method,
			Name:     e.name,
			Requests: e.hist.count,
			Failed:   e.failed,
			AvgMS:    e.hist.avg(),
			P50MS:    e.hist.percentile(0.50),
			P95MS:    e.hist.percentile(0.95),
			P99MS:    e.hist.percentile(0.99),
			MaxMS:    e.hist.max,
		}
		if st.Requests > 0 {
			st.ErrorRate = float64(st.Failed) / float64(st.Requests) * 100
			if s.DurationSeconds >= 1 {
				st.RPS = float64(st.Requests) / s.DurationSeconds
			}
		}
		s.Endpoints = append(s.Endpoints, st)
	}
	sort.Slice(s.Endpoints, func(i, j int) bool { return s.Endpoints[i].Requests > s.Endpoints[j].Requests })

	for _, c := range a.checks {
		s.Checks = append(s.Checks, db.CheckStat{Name: c.name, Group: c.group, Passes: c.passes, Fails: c.fails})
	}
	sort.Slice(s.Checks, func(i, j int) bool {
		if s.Checks[i].Group != s.Checks[j].Group {
			return s.Checks[i].Group < s.Checks[j].Group
		}
		return s.Checks[i].Name < s.Checks[j].Name
	})
	return s
}

func (a *aggregator) recentErrors() []db.RunError {
	a.mu.Lock()
	defer a.mu.Unlock()
	out := make([]db.RunError, len(a.errors))
	copy(out, a.errors)
	return out
}

func (a *aggregator) vus() int {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.currentVUs
}
