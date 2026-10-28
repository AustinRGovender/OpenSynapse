package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/engine"
)

// Micro-test limits from F-03.
const (
	microMaxVUs      = 50
	microMaxDuration = 60
)

type microTestRequest struct {
	Request         PlaygroundRequest `json:"request"`
	VUs             int               `json:"vus"`
	DurationSeconds int               `json:"duration_seconds"`
}

// MicroTestResult is the mini-dashboard data; it is not stored as a run.
type MicroTestResult struct {
	Summary *db.RunSummary   `json:"summary"`
	Series  []db.MetricPoint `json:"series"`
	Errors  []db.RunError    `json:"errors"`
	Status  string           `json:"status"`
	Message string           `json:"message,omitempty"`
}

// microScript renders a single-request k6 script. Headers, auth and the
// body are embedded as JSON literals, so any value is safe to include.
func microScript(method, url string, headers http.Header, body string, vus, seconds int) string {
	flat := map[string]string{}
	for k, v := range headers {
		flat[k] = strings.Join(v, ", ")
	}
	h, _ := json.Marshal(flat)
	u, _ := json.Marshal(url)
	m, _ := json.Marshal(method)
	b := "null"
	if body != "" {
		bb, _ := json.Marshal(body)
		b = string(bb)
	}
	return fmt.Sprintf(`import http from 'k6/http';

export const options = {
  scenarios: { micro: { executor: 'constant-vus', vus: %d, duration: '%ds', gracefulStop: '5s' } },
};

const params = { headers: %s, tags: { name: %s } };

export default function () {
  http.request(%s, %s, %s, params);
}
`, vus, seconds, h, u, m, u, b)
}

// MicroTest runs the current request with several VUs for a short time.
func (h *PlaygroundHandlers) MicroTest(w http.ResponseWriter, r *http.Request) {
	if h.Engine == nil {
		writeError(w, http.StatusServiceUnavailable, "ENGINE_NOT_AVAILABLE", "k6 engine is not available. Install k6 to run micro-tests.", nil)
		return
	}
	var req microTestRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	if req.VUs < 1 || req.VUs > microMaxVUs || req.DurationSeconds < 1 || req.DurationSeconds > microMaxDuration {
		badRequest(w, "VALIDATION_ERROR", fmt.Sprintf("Micro-tests use 1–%d VUs for 1–%d seconds", microMaxVUs, microMaxDuration), nil)
		return
	}
	pr := req.Request
	vars, err := h.envVars(pr.EnvironmentID)
	if err != nil {
		badRequest(w, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	pr.resolve(vars)
	if pr.Method == "" {
		pr.Method = "GET"
	}
	pr.Method = strings.ToUpper(pr.Method)
	if pr.AuthType == "digest" {
		badRequest(w, "VALIDATION_ERROR", "Micro-tests do not support Digest auth yet; use Send to test it", nil)
		return
	}
	httpReq, err := buildRequest(&pr)
	if err != nil {
		badRequest(w, "INVALID_REQUEST", err.Error(), nil)
		return
	}
	if err := h.applyAuth(httpReq, pr.AuthType, pr.AuthConfig, vars); err != nil {
		badRequest(w, "AUTH_ERROR", err.Error(), nil)
		return
	}

	script := microScript(pr.Method, httpReq.URL.String(), httpReq.Header, pr.Body, req.VUs, req.DurationSeconds)
	res := &MicroTestResult{Series: []db.MetricPoint{}, Errors: []db.RunError{}}
	var mu sync.Mutex // engine callbacks run on different goroutines
	done := make(chan struct{})
	err = h.Engine.StartRun(engine.RunSpec{RunID: "micro-" + uuid.New().String(), Script: script}, engine.Callbacks{
		OnMetrics: func(points []db.MetricPoint, _ engine.LiveState) {
			mu.Lock()
			res.Series = append(res.Series, points...)
			mu.Unlock()
		},
		OnErrors: func(errs []db.RunError) {
			mu.Lock()
			defer mu.Unlock()
			if room := 50 - len(res.Errors); room > 0 {
				res.Errors = append(res.Errors, errs[:min(room, len(errs))]...)
			}
		},
		OnComplete: func(out engine.Result) {
			mu.Lock()
			res.Summary, res.Status, res.Message = out.Summary, out.Status, out.Message
			mu.Unlock()
			close(done)
		},
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "ENGINE_ERROR", "Failed to start k6: "+err.Error(), nil)
		return
	}
	select {
	case <-done:
		mu.Lock()
		defer mu.Unlock()
		writeJSON(w, http.StatusOK, res)
	case <-time.After(time.Duration(req.DurationSeconds)*time.Second + 90*time.Second):
		writeError(w, http.StatusGatewayTimeout, "MICROTEST_TIMEOUT", "The micro-test did not finish in time", nil)
	}
}
