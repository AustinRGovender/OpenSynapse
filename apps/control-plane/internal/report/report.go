// Package report renders run reports as self-contained HTML (charts are
// inline SVG, so the file opens offline and prints cleanly) and as PDF via
// headless Chromium.
package report

import (
	"bytes"
	"encoding/json"
	"fmt"
	"html/template"
	"math"
	"strings"
	"time"

	"github.com/go-rod/rod/lib/proto"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/browser"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

// Document is the full report for one run.
type Document struct {
	Run    *db.Run          `json:"run"`
	Series []db.MetricPoint `json:"series"`
	Events []db.RunEvent    `json:"events"`
	Errors []db.RunError    `json:"errors"`
}

type chartSeries struct {
	Label string
	Color string // CSS variable name, e.g. "--c-accent"
	Value func(p db.MetricPoint) float64
}

const (
	chartW = 880
	chartH = 220
	padL   = 56
	padR   = 16
	padT   = 16
	padB   = 28
)

// svgChart draws a line chart of the run's per-second series.
func svgChart(points []db.MetricPoint, unit string, series ...chartSeries) template.HTML {
	if len(points) == 0 {
		return template.HTML(`<p class="empty">No data recorded.</p>`)
	}
	t0 := points[0].TimestampMS
	tMax := float64(points[len(points)-1].TimestampMS-t0) / 1000
	if tMax <= 0 {
		tMax = 1
	}
	yMax := 0.0
	for _, s := range series {
		for _, p := range points {
			yMax = math.Max(yMax, s.Value(p))
		}
	}
	yMax = niceCeil(yMax)

	plotW := float64(chartW - padL - padR)
	plotH := float64(chartH - padT - padB)
	x := func(ms int64) float64 { return padL + float64(ms-t0)/1000/tMax*plotW }
	y := func(v float64) float64 { return padT + plotH - v/yMax*plotH }

	var b strings.Builder
	fmt.Fprintf(&b, `<svg viewBox="0 0 %d %d" role="img" class="chart">`, chartW, chartH)
	for i := 0; i <= 4; i++ {
		v := yMax * float64(i) / 4
		yy := y(v)
		fmt.Fprintf(&b, `<line x1="%d" x2="%d" y1="%.1f" y2="%.1f" class="grid"/>`, padL, chartW-padR, yy, yy)
		fmt.Fprintf(&b, `<text x="%d" y="%.1f" class="axis" text-anchor="end">%s</text>`, padL-6, yy+4, formatNum(v))
	}
	for i := 0; i <= 5; i++ {
		sec := tMax * float64(i) / 5
		xx := padL + sec/tMax*plotW
		fmt.Fprintf(&b, `<text x="%.1f" y="%d" class="axis" text-anchor="middle">%s</text>`, xx, chartH-8, formatElapsed(sec))
	}
	fmt.Fprintf(&b, `<text x="%d" y="%d" class="axis unit">%s</text>`, padL, padT-4, template.HTMLEscapeString(unit))
	for _, s := range series {
		var pts []string
		for _, p := range points {
			pts = append(pts, fmt.Sprintf("%.1f,%.1f", x(p.TimestampMS), y(s.Value(p))))
		}
		fmt.Fprintf(&b, `<polyline fill="none" style="stroke: var(%s)" stroke-width="1.8" points="%s"/>`, s.Color, strings.Join(pts, " "))
	}
	b.WriteString(`</svg>`)
	if len(series) > 1 {
		b.WriteString(`<div class="legend">`)
		for _, s := range series {
			fmt.Fprintf(&b, `<span><i style="background: var(%s)"></i>%s</span>`, s.Color, template.HTMLEscapeString(s.Label))
		}
		b.WriteString(`</div>`)
	}
	return template.HTML(b.String())
}

func niceCeil(v float64) float64 {
	if v <= 0 {
		return 1
	}
	mag := math.Pow(10, math.Floor(math.Log10(v)))
	for _, m := range []float64{1, 2, 2.5, 5, 10} {
		if v <= m*mag {
			return m * mag
		}
	}
	return 10 * mag
}

func formatNum(v float64) string {
	switch {
	case v >= 1e9:
		return fmt.Sprintf("%.1fG", v/1e9)
	case v >= 1e6:
		return fmt.Sprintf("%.1fM", v/1e6)
	case v >= 1e3:
		return fmt.Sprintf("%.1fk", v/1e3)
	case v == math.Trunc(v):
		return fmt.Sprintf("%.0f", v)
	default:
		return fmt.Sprintf("%.1f", v)
	}
}

func formatElapsed(sec float64) string {
	d := time.Duration(sec * float64(time.Second)).Round(time.Second)
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	return fmt.Sprintf("%d:%02d", m, s)
}

func formatBytes(n int64) string {
	f := float64(n)
	switch {
	case f >= 1<<30:
		return fmt.Sprintf("%.2f GB", f/(1<<30))
	case f >= 1<<20:
		return fmt.Sprintf("%.2f MB", f/(1<<20))
	case f >= 1<<10:
		return fmt.Sprintf("%.1f KB", f/(1<<10))
	}
	return fmt.Sprintf("%d B", n)
}

// eventText renders an event the same way the run view does.
func eventText(e db.RunEvent) string {
	var p map[string]any
	json.Unmarshal(e.Payload, &p)
	get := func(k string) string { return fmt.Sprint(p[k]) }
	switch e.Type {
	case "start":
		return fmt.Sprintf("Run started (%s VUs, %ss)", get("vus"), get("duration_seconds"))
	case "stop":
		if m, ok := p["message"]; ok {
			return fmt.Sprintf("Run ended: %s — %v", get("status"), m)
		}
		return "Run ended: " + get("status")
	case "vu_change":
		if p["schedule"] != nil {
			return "VUs handed back to the load profile"
		}
		return "VUs set to " + get("vus")
	case "rps_change":
		return "RPS limit set to " + get("rps")
	case "duration_change":
		return "Duration set to " + get("duration_seconds") + "s"
	case "pause":
		return "Paused"
	case "resume":
		return "Resumed"
	case "stop_requested":
		return "Stop requested"
	case "kill":
		return "Killed"
	case "error":
		return "Error: " + get("message")
	case "user_note":
		return "Note: " + get("message")
	}
	return e.Type
}

type view struct {
	Doc       *Document
	Title     string
	Generated string
	Duration  string
	S         *db.RunSummary
	Charts    []chartView
	Events    []eventView
	DataJSON  template.JS
	Fmt       map[string]any
}

type chartView struct {
	Title string
	SVG   template.HTML
}

type eventView struct {
	At   string
	Text string
}

var funcs = template.FuncMap{
	"f1":  func(v float64) string { return fmt.Sprintf("%.1f", v) },
	"f2":  func(v float64) string { return fmt.Sprintf("%.2f", v) },
	"num": func(v int64) string { return formatNum(float64(v)) },
	// endpoint shows "GET /path" once when the request is already named that way.
	"endpoint": func(method, name string) string {
		if method != "" && !strings.HasPrefix(name, method+" ") {
			return method + " " + name
		}
		return name
	},
	"bytes": formatBytes,
	"ts": func(ms int64) string {
		return time.UnixMilli(ms).UTC().Format("15:04:05")
	},
	"pct": func(pass, fail int64) string {
		if pass+fail == 0 {
			return "—"
		}
		return fmt.Sprintf("%.1f%%", float64(pass)/float64(pass+fail)*100)
	},
}

var page = template.Must(template.New("report").Funcs(funcs).Parse(reportTemplate))

// RenderHTML renders a self-contained HTML report.
func RenderHTML(doc *Document) ([]byte, error) {
	run := doc.Run
	v := view{
		Doc:       doc,
		Title:     run.PlanName,
		Generated: time.Now().UTC().Format("2006-01-02 15:04 MST"),
		S:         run.Summary,
	}
	if v.Title == "" {
		v.Title = "Run " + run.ID[:8]
	}
	if v.S == nil {
		v.S = &db.RunSummary{}
	}
	if run.StartedAt != nil {
		end := time.Now()
		if run.EndedAt != nil {
			end = *run.EndedAt
		}
		v.Duration = end.Sub(*run.StartedAt).Round(time.Second).String()
	}
	pts := doc.Series
	v.Charts = []chartView{
		{"Requests per second", svgChart(pts, "req/s", chartSeries{"RPS", "--c-1", func(p db.MetricPoint) float64 { return p.RPS }})},
		{"Response time percentiles", svgChart(pts, "ms",
			chartSeries{"p50", "--c-2", func(p db.MetricPoint) float64 { return p.P50MS }},
			chartSeries{"p90", "--c-3", func(p db.MetricPoint) float64 { return p.P90MS }},
			chartSeries{"p95", "--c-4", func(p db.MetricPoint) float64 { return p.P95MS }},
			chartSeries{"p99", "--c-5", func(p db.MetricPoint) float64 { return p.P99MS }})},
		{"Error rate", svgChart(pts, "%", chartSeries{"Errors", "--c-error", func(p db.MetricPoint) float64 { return p.ErrorRate }})},
		{"Active virtual users", svgChart(pts, "VUs", chartSeries{"VUs", "--c-6", func(p db.MetricPoint) float64 { return float64(p.ActiveVUs) }})},
		{"Throughput", svgChart(pts, "bytes/s",
			chartSeries{"Sent", "--c-2", func(p db.MetricPoint) float64 { return float64(p.BytesSent) }},
			chartSeries{"Received", "--c-1", func(p db.MetricPoint) float64 { return float64(p.BytesReceived) }})},
	}
	for _, e := range doc.Events {
		v.Events = append(v.Events, eventView{At: time.UnixMilli(e.TimestampMS).UTC().Format("15:04:05"), Text: eventText(e)})
	}
	data, err := json.Marshal(doc) // json.Marshal escapes <, > and & so this is safe inside <script>
	if err != nil {
		return nil, err
	}
	v.DataJSON = template.JS(data)

	var buf bytes.Buffer
	if err := page.Execute(&buf, v); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// RenderPDF prints an HTML report to PDF with headless Chromium.
func RenderPDF(html []byte) ([]byte, error) {
	b, cleanup, err := browser.Launch()
	if err != nil {
		return nil, err
	}
	defer cleanup()
	p, err := b.Page(proto.TargetCreateTarget{URL: "about:blank"})
	if err != nil {
		return nil, err
	}
	p = p.Timeout(60 * time.Second)
	if err := p.SetDocumentContent(string(html)); err != nil {
		return nil, err
	}
	if err := p.WaitLoad(); err != nil {
		return nil, err
	}
	r, err := p.PDF(&proto.PagePrintToPDF{PrintBackground: true, PreferCSSPageSize: true})
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if _, err := out.ReadFrom(r); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// The palette mirrors packages/ui/src/tokens.ts (chart and semantic colours).
const reportTemplate = `<!DOCTYPE html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} — OpenSynapse run report</title>
<style>
:root {
  --bg: #ffffff; --fg: #0f172a; --muted: #64748b; --line: #e2e8f0; --card: #f8fafc;
  --c-1: #0d9488; --c-2: #6366f1; --c-3: #0ea5e9; --c-4: #f59e0b; --c-5: #ef4444; --c-6: #14b8a6;
  --c-error: #dc2626; --c-ok: #16a34a;
}
@media (prefers-color-scheme: dark) {
  :root { --bg: #020617; --fg: #e2e8f0; --muted: #94a3b8; --line: #1e293b; --card: #0f172a; }
}
@page { size: A4; margin: 14mm; }
@media print { :root { --bg: #ffffff; --fg: #0f172a; --muted: #64748b; --line: #e2e8f0; --card: #f8fafc; } section { break-inside: avoid; } }
* { box-sizing: border-box; }
body { margin: 0; padding: 24px; background: var(--bg); color: var(--fg); font: 14px/1.5 Inter, system-ui, -apple-system, "Segoe UI", sans-serif; }
main { max-width: 920px; margin: 0 auto; }
h1 { font-size: 22px; margin: 0 0 4px; }
h2 { font-size: 15px; margin: 28px 0 10px; }
.meta { color: var(--muted); font-size: 12px; }
.status { display: inline-block; padding: 1px 8px; border-radius: 999px; border: 1px solid var(--line); font-weight: 600; }
.status.completed { color: var(--c-ok); } .status.failed, .status.aborted { color: var(--c-error); }
.cards { display: grid; grid-template-columns: repeat(auto-fit, minmax(150px, 1fr)); gap: 10px; margin-top: 18px; }
.card { background: var(--card); border: 1px solid var(--line); border-radius: 8px; padding: 10px 12px; }
.card .k { color: var(--muted); font-size: 11px; text-transform: uppercase; letter-spacing: .04em; }
.card .v { font-size: 20px; font-weight: 600; font-variant-numeric: tabular-nums; }
.chart { width: 100%; height: auto; background: var(--card); border: 1px solid var(--line); border-radius: 8px; }
.chart .grid { stroke: var(--line); stroke-width: 1; }
.chart .axis { fill: var(--muted); font-size: 11px; }
.legend { display: flex; gap: 14px; font-size: 12px; color: var(--muted); margin-top: 4px; }
.legend i { display: inline-block; width: 10px; height: 3px; margin-right: 5px; vertical-align: middle; }
table { width: 100%; border-collapse: collapse; font-size: 12px; font-variant-numeric: tabular-nums; }
th, td { text-align: left; padding: 6px 8px; border-bottom: 1px solid var(--line); }
th { color: var(--muted); font-weight: 500; }
td.n, th.n { text-align: right; }
.bad { color: var(--c-error); }
.empty { color: var(--muted); font-size: 12px; }
footer { margin-top: 32px; color: var(--muted); font-size: 11px; }
code { font-size: 11px; word-break: break-all; }
</style>
</head>
<body>
<main>
<header>
  <h1>{{.Title}}</h1>
  <div class="meta">
    Run <code>{{.Doc.Run.ID}}</code> · <span class="status {{.Doc.Run.Status}}">{{.Doc.Run.Status}}</span>
    {{if .Doc.Run.StartedAt}} · started {{.Doc.Run.StartedAt.UTC.Format "2006-01-02 15:04:05 MST"}}{{end}}
    {{if .Duration}} · {{.Duration}}{{end}}
    · {{.Doc.Run.Parameters.VUsTarget}} VUs peak target
  </div>
</header>

<section class="cards">
  <div class="card"><div class="k">Requests</div><div class="v">{{num .S.TotalRequests}}</div></div>
  <div class="card"><div class="k">Failed</div><div class="v {{if .S.FailedRequests}}bad{{end}}">{{num .S.FailedRequests}}</div></div>
  <div class="card"><div class="k">Error rate</div><div class="v {{if .S.FailedRequests}}bad{{end}}">{{f2 .S.ErrorRate}}%</div></div>
  <div class="card"><div class="k">Throughput</div><div class="v">{{f1 .S.ThroughputRPS}} rps</div></div>
  <div class="card"><div class="k">p50</div><div class="v">{{f1 .S.P50MS}} ms</div></div>
  <div class="card"><div class="k">p95</div><div class="v">{{f1 .S.P95MS}} ms</div></div>
  <div class="card"><div class="k">p99</div><div class="v">{{f1 .S.P99MS}} ms</div></div>
  <div class="card"><div class="k">Max</div><div class="v">{{f1 .S.MaxMS}} ms</div></div>
  <div class="card"><div class="k">Peak VUs</div><div class="v">{{.S.PeakVUs}}</div></div>
  <div class="card"><div class="k">Checks passed</div><div class="v {{if .S.AssertionFailures}}bad{{end}}">{{pct .S.AssertionPasses .S.AssertionFailures}}</div></div>
  <div class="card"><div class="k">Data sent</div><div class="v">{{bytes .S.BytesSent}}</div></div>
  <div class="card"><div class="k">Data received</div><div class="v">{{bytes .S.BytesReceived}}</div></div>
</section>

{{range .Charts}}<section><h2>{{.Title}}</h2>{{.SVG}}</section>{{end}}

<section>
<h2>Endpoints</h2>
{{if .S.Endpoints}}<table>
<tr><th>Endpoint</th><th class="n">Requests</th><th class="n">Errors</th><th class="n">Avg ms</th><th class="n">p95 ms</th><th class="n">p99 ms</th><th class="n">Max ms</th></tr>
{{range .S.Endpoints}}<tr><td><code>{{endpoint .Method .Name}}</code></td><td class="n">{{num .Requests}}</td><td class="n {{if .Failed}}bad{{end}}">{{f2 .ErrorRate}}%</td><td class="n">{{f1 .AvgMS}}</td><td class="n">{{f1 .P95MS}}</td><td class="n">{{f1 .P99MS}}</td><td class="n">{{f1 .MaxMS}}</td></tr>
{{end}}</table>{{else}}<p class="empty">No requests recorded.</p>{{end}}
</section>

<section>
<h2>Assertions</h2>
{{if .S.Checks}}<table>
<tr><th>Check</th><th>Group</th><th class="n">Passed</th><th class="n">Failed</th><th class="n">Pass rate</th></tr>
{{range .S.Checks}}<tr><td>{{.Name}}</td><td>{{.Group}}</td><td class="n">{{num .Passes}}</td><td class="n {{if .Fails}}bad{{end}}">{{num .Fails}}</td><td class="n">{{pct .Passes .Fails}}</td></tr>
{{end}}</table>{{else}}<p class="empty">This plan has no assertions.</p>{{end}}
</section>

<section>
<h2>Errors (most recent)</h2>
{{if .Doc.Errors}}<table>
<tr><th>Time</th><th class="n">Status</th><th>Request</th><th>Detail</th></tr>
{{range .Doc.Errors}}<tr><td>{{ts .TimestampMS}}</td><td class="n bad">{{.Status}}</td><td><code>{{.Method}} {{.URL}}</code></td><td>{{.Message}}</td></tr>
{{end}}</table>{{else}}<p class="empty">No failed requests.</p>{{end}}
</section>

<section>
<h2>Timeline</h2>
{{if .Events}}<table>{{range .Events}}<tr><td style="width: 90px">{{.At}}</td><td>{{.Text}}</td></tr>{{end}}</table>{{else}}<p class="empty">No events.</p>{{end}}
</section>

<footer>
Generated by OpenSynapse {{.Generated}}. Times are UTC. Response times are measured by k6
(<code>http_req_duration</code>: sending + waiting + receiving), which typically reads 10–20% lower than JMeter for the same server.
</footer>
</main>
<script type="application/json" id="opensynapse-report">{{.DataJSON}}</script>
</body>
</html>
`
