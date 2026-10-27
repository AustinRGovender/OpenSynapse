package handlers

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/artifacts"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/browser"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/report"
)

// ExportHandlers serve run exports in the four formats from F-08.
type ExportHandlers struct {
	runs *RunHandlers
}

func NewExportHandlers(runs *RunHandlers) *ExportHandlers {
	return &ExportHandlers{runs: runs}
}

type exportRequest struct {
	Format string `json:"format"` // json, csv, html, pdf
}

func (h *ExportHandlers) ExportRun(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	var req exportRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	format := strings.ToLower(req.Format)
	switch format {
	case "json", "csv", "html", "pdf":
	default:
		badRequest(w, "INVALID_FORMAT", "Supported formats: json, csv, html, pdf", nil)
		return
	}

	doc, err := h.runs.BuildReport(id)
	if err != nil {
		internalError(w, err)
		return
	}
	if doc == nil {
		notFound(w, "RUN", id)
		return
	}
	name := "opensynapse-run-" + id[:min(8, len(id))]

	switch format {
	case "json":
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.json"`, name))
		enc := json.NewEncoder(w)
		enc.SetIndent("", "  ")
		enc.Encode(doc)
	case "csv":
		h.exportCSV(w, doc, name)
	case "html":
		html, err := report.RenderHTML(doc)
		if err != nil {
			internalError(w, err)
			return
		}
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.html"`, name))
		w.Write(html)
	case "pdf":
		html, err := report.RenderHTML(doc)
		if err != nil {
			internalError(w, err)
			return
		}
		pdf, err := report.RenderPDF(html)
		if err != nil {
			if errors.Is(err, browser.ErrNotFound) {
				writeError(w, http.StatusServiceUnavailable, "PDF_UNAVAILABLE",
					"PDF export needs Chrome or Chromium on the control-plane host (bundled in the Docker image). Set ROD_BROWSER_BIN or use HTML export.", nil)
				return
			}
			writeError(w, http.StatusInternalServerError, "PDF_FAILED", "Rendering the PDF failed: "+err.Error(), nil)
			return
		}
		w.Header().Set("Content-Type", "application/pdf")
		w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.pdf"`, name))
		w.Write(pdf)
	}
}

// exportCSV streams one row per raw sample (timestamp, metric, value, tags)
// when the raw archive exists, falling back to the per-second series.
func (h *ExportHandlers) exportCSV(w http.ResponseWriter, doc *RunReport, name string) {
	w.Header().Set("Content-Type", "text/csv; charset=utf-8")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s.csv"`, name))
	cw := csv.NewWriter(w)
	defer cw.Flush()

	if h.runs.Artifacts != nil {
		if f, err := h.runs.Artifacts.Open(context.Background(), artifacts.RunSamplesKey(doc.Run.ID)); err == nil {
			defer f.Close()
			if gz, err := gzip.NewReader(f); err == nil {
				defer gz.Close()
				cw.Write([]string{"timestamp", "metric", "value", "tags"})
				s := bufio.NewScanner(gz)
				s.Buffer(make([]byte, 64*1024), 4*1024*1024)
				for s.Scan() {
					var line struct {
						Metric string `json:"metric"`
						Data   struct {
							Time  time.Time         `json:"time"`
							Value float64           `json:"value"`
							Tags  map[string]string `json:"tags"`
						} `json:"data"`
					}
					if json.Unmarshal(s.Bytes(), &line) != nil {
						continue
					}
					keys := make([]string, 0, len(line.Data.Tags))
					for k := range line.Data.Tags {
						keys = append(keys, k)
					}
					sort.Strings(keys)
					tags := make([]string, 0, len(keys))
					for _, k := range keys {
						tags = append(tags, k+"="+line.Data.Tags[k])
					}
					cw.Write([]string{
						line.Data.Time.UTC().Format(time.RFC3339Nano),
						line.Metric,
						strconv.FormatFloat(line.Data.Value, 'f', -1, 64),
						strings.Join(tags, ";"),
					})
				}
				return
			}
		}
	}

	// No raw archive (e.g. deleted or never written): per-second aggregates.
	cw.Write([]string{"timestamp", "requests", "failed", "rps", "error_rate_pct", "p50_ms", "p90_ms", "p95_ms", "p99_ms",
		"active_vus", "bytes_sent", "bytes_received", "iterations", "checks_passed", "checks_failed"})
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', 3, 64) }
	i := func(v int64) string { return strconv.FormatInt(v, 10) }
	for _, p := range doc.Series {
		cw.Write([]string{
			time.UnixMilli(p.TimestampMS).UTC().Format(time.RFC3339), i(p.Requests), i(p.Failed), f(p.RPS), f(p.ErrorRate),
			f(p.P50MS), f(p.P90MS), f(p.P95MS), f(p.P99MS), strconv.Itoa(p.ActiveVUs), i(p.BytesSent), i(p.BytesReceived),
			i(p.Iterations), i(p.ChecksPassed), i(p.ChecksFailed),
		})
	}
}
