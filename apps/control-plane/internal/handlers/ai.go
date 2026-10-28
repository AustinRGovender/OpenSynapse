package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/ai"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

type AIHandlers struct {
	aiStore     *db.AIStore
	runStore    *db.RunStore
	reportStore *db.ReportStore

	mu    sync.Mutex
	calls []time.Time
}

func NewAIHandlers(aiStore *db.AIStore, runStore *db.RunStore, reportStore *db.ReportStore) *AIHandlers {
	return &AIHandlers{aiStore: aiStore, runStore: runStore, reportStore: reportStore}
}

// GetConfig returns the AI configuration with secrets redacted.
func (h *AIHandlers) GetConfig(w http.ResponseWriter, r *http.Request) {
	config, err := h.aiStore.GetConfig()
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, config)
}

// UpdateConfig saves the AI configuration.
func (h *AIHandlers) UpdateConfig(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Provider      string   `json:"provider"`
		APIKey        string   `json:"api_key"`
		Model         string   `json:"model"`
		MonthlyCap    *float64 `json:"monthly_cap_usd"`
		Enabled       bool     `json:"enabled"`
		AzureEndpoint string   `json:"azure_endpoint"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	if err := h.aiStore.SaveConfig(req.Provider, req.APIKey, req.Model, req.MonthlyCap, req.Enabled, req.AzureEndpoint); err != nil {
		internalError(w, err)
		return
	}

	config, _ := h.aiStore.GetConfig()
	writeJSON(w, http.StatusOK, config)
}

// TestConfig validates the configured API key with a minimal call.
func (h *AIHandlers) TestConfig(w http.ResponseWriter, r *http.Request) {
	config, err := h.aiStore.GetConfig()
	if err != nil {
		internalError(w, err)
		return
	}

	if config.Provider == "" {
		badRequest(w, "AI_NOT_CONFIGURED", "No AI provider configured", nil)
		return
	}

	key, err := h.aiStore.GetRawKey()
	if err != nil || key == "" {
		badRequest(w, "AI_NO_KEY", "No API key configured", nil)
		return
	}

	provider, err := ai.GetProvider(config.Provider, config.AzureEndpoint)
	if err != nil {
		badRequest(w, "AI_INVALID_PROVIDER", err.Error(), nil)
		return
	}

	if err := provider.TestKey(key, config.Model); err != nil {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"valid":   false,
			"message": err.Error(),
		})
		return
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"valid":   true,
		"message": "Key is valid",
	})
}

type analyseRequest struct {
	RunID    string   `json:"run_id,omitempty"`
	RunIDs   []string `json:"run_ids,omitempty"`
	ReportID string   `json:"report_id,omitempty"`
	Question string   `json:"question,omitempty"`
}

// resolve returns the runs to describe and the default question.
func (h *AIHandlers) resolve(req *analyseRequest) ([]string, error) {
	switch {
	case req.RunID != "":
		if req.Question == "" {
			req.Question = "What does this run tell me?"
		}
		return []string{req.RunID}, nil
	case len(req.RunIDs) > 0:
		if req.Question == "" {
			req.Question = "What changed between these runs?"
		}
		return req.RunIDs, nil
	case req.ReportID != "":
		rep, err := h.reportStore.Get(req.ReportID)
		if err != nil {
			return nil, err
		}
		if rep == nil {
			return nil, fmt.Errorf("report not found")
		}
		if req.Question == "" {
			req.Question = "What changed between these runs?"
		}
		return rep.RunIDs, nil
	}
	return nil, fmt.Errorf("run_id, run_ids, or report_id is required")
}

// Preview returns exactly the prompt Analyse would send, without sending it.
func (h *AIHandlers) Preview(w http.ResponseWriter, r *http.Request) {
	var req analyseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	ids, err := h.resolve(&req)
	if err != nil {
		badRequest(w, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	prompt, err := h.buildPrompt(ids, req.Question)
	if err != nil {
		badRequest(w, "VALIDATION_ERROR", err.Error(), nil)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"prompt": prompt, "question": req.Question})
}

// Analyse runs AI analysis on a run or comparison.
func (h *AIHandlers) Analyse(w http.ResponseWriter, r *http.Request) {
	var req analyseRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	runIDs, err := h.resolve(&req)
	if err != nil {
		badRequest(w, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	// Check AI is configured and enabled
	config, err := h.aiStore.GetConfig()
	if err != nil {
		internalError(w, err)
		return
	}
	if !config.Enabled || config.Provider == "" {
		badRequest(w, "AI_NOT_ENABLED", "AI analysis is not enabled. Configure it in Settings.", nil)
		return
	}

	// Check monthly cap
	monthlyUsage, _ := h.aiStore.GetMonthlyUsage()
	if config.MonthlyCap != nil && monthlyUsage >= *config.MonthlyCap {
		badRequest(w, "AI_CAP_EXCEEDED", fmt.Sprintf("Monthly spend cap of $%.2f has been reached (used: $%.2f)", *config.MonthlyCap, monthlyUsage), nil)
		return
	}

	question := req.Question

	// Cached per run (or per run set) and question, to avoid double billing.
	cacheKey := strings.Join(runIDs, ",")
	if cached, _ := h.aiStore.GetCachedAnalysis(cacheKey, question); cached != nil {
		writeJSON(w, http.StatusOK, cached)
		return
	}

	if !h.allow() {
		writeError(w, http.StatusTooManyRequests, "AI_RATE_LIMITED", fmt.Sprintf("At most %d AI analyses per hour; try again later.", aiRateLimit), nil)
		return
	}

	prompt, err := h.buildPrompt(runIDs, question)
	if err != nil {
		badRequest(w, "VALIDATION_ERROR", err.Error(), nil)
		return
	}

	// Get provider and key
	key, _ := h.aiStore.GetRawKey()
	provider, err := ai.GetProvider(config.Provider, config.AzureEndpoint)
	if err != nil {
		internalError(w, err)
		return
	}

	// Send to AI
	chatResp, err := provider.Chat(key, config.Model, prompt)
	if err != nil {
		writeError(w, http.StatusBadGateway, "AI_REQUEST_FAILED", err.Error(), nil)
		return
	}

	// Cache the result
	var runIDPtr, reportIDPtr *string
	runIDPtr = &cacheKey
	if req.ReportID != "" {
		reportIDPtr = &req.ReportID
	}

	analysis, err := h.aiStore.CacheAnalysis(
		runIDPtr, reportIDPtr, question, prompt,
		chatResp.Content, config.Provider, config.Model,
		chatResp.InputTokens, chatResp.OutputTokens, chatResp.CostUSD,
	)
	if err != nil {
		internalError(w, err)
		return
	}

	writeJSON(w, http.StatusOK, analysis)
}

// describeRun summarises one run for a prompt: metadata, headline numbers,
// the worst endpoints, failing assertions, and a few recent errors. Raw
// samples are never sent (F-09).
func (h *AIHandlers) describeRun(runID, label string) (string, error) {
	run, err := h.runStore.Get(runID)
	if err != nil {
		return "", err
	}
	if run == nil {
		return "", fmt.Errorf("run %s not found", runID)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%s — plan %q, status %s\n", label, run.PlanName, run.Status)
	p := run.Parameters
	fmt.Fprintf(&b, "- Load profile: peak %d VUs for %ds", p.VUsTarget, p.DurationSeconds)
	if len(p.Stages) > 0 {
		var st []string
		for _, s := range p.Stages {
			st = append(st, fmt.Sprintf("%s→%d", s.Duration, s.Target))
		}
		fmt.Fprintf(&b, " (stages: %s)", strings.Join(st, ", "))
	}
	if p.RPSTarget > 0 {
		fmt.Fprintf(&b, ", RPS limit %d", p.RPSTarget)
	}
	b.WriteString("\n")
	s := run.Summary
	if s == nil {
		b.WriteString("- No summary recorded.\n")
		return b.String(), nil
	}
	fmt.Fprintf(&b, "- Requests: %d total, %d failed (error rate %.2f%%), %.1f req/s over %.0fs, peak %d VUs\n",
		s.TotalRequests, s.FailedRequests, s.ErrorRate, s.ThroughputRPS, s.DurationSeconds, s.PeakVUs)
	fmt.Fprintf(&b, "- Response time: avg %.1f ms, p50 %.1f, p90 %.1f, p95 %.1f, p99 %.1f, max %.1f ms\n",
		s.AvgMS, s.P50MS, s.P90MS, s.P95MS, s.P99MS, s.MaxMS)
	fmt.Fprintf(&b, "- Assertions: %d passed, %d failed; thresholds passed: %v\n", s.AssertionPasses, s.AssertionFailures, s.ThresholdsPassed)

	eps := append([]db.EndpointStat(nil), s.Endpoints...)
	sort.Slice(eps, func(i, j int) bool { return eps[i].P95MS > eps[j].P95MS })
	if len(eps) > 0 {
		b.WriteString("- Slowest endpoints (p95):\n")
		for _, e := range eps[:min(5, len(eps))] {
			fmt.Fprintf(&b, "  - %s %s: %d requests, p95 %.1f ms, p99 %.1f ms, errors %.2f%%\n", e.Method, e.Name, e.Requests, e.P95MS, e.P99MS, e.ErrorRate)
		}
	}
	for _, c := range s.Checks {
		if c.Fails > 0 {
			fmt.Fprintf(&b, "- Failing assertion %q: %d of %d failed\n", c.Name, c.Fails, c.Passes+c.Fails)
		}
	}
	if errs, _ := h.runStore.ListErrors(runID, 5); len(errs) > 0 {
		b.WriteString("- Recent errors:\n")
		for _, e := range errs {
			fmt.Fprintf(&b, "  - %d %s %s %s\n", e.Status, e.Method, e.URL, e.Message)
		}
	}
	return b.String(), nil
}

func (h *AIHandlers) buildPrompt(runIDs []string, question string) (string, error) {
	var context strings.Builder
	for i, id := range runIDs {
		label := "Run"
		if len(runIDs) > 1 {
			label = fmt.Sprintf("Run %d", i+1)
			if i == 0 {
				label += " (baseline)"
			}
		}
		d, err := h.describeRun(id, label)
		if err != nil {
			return "", err
		}
		context.WriteString(d)
		context.WriteString("\n")
	}

	return fmt.Sprintf(`You are a performance engineering expert analysing load test results from OpenSynapse, a performance testing platform built on k6.

%sQuestion: %s

Provide a concise, actionable analysis. Use markdown formatting. Focus on:
1. Key observations from the metrics
2. Potential bottlenecks or concerns
3. Specific recommendations for next steps

Keep your response under 500 words.`, context.String(), question), nil
}

// aiRateLimit is the per-hour cap on analysis calls (docs/04 section 7).
const aiRateLimit = 20

func (h *AIHandlers) allow() bool {
	h.mu.Lock()
	defer h.mu.Unlock()
	cutoff := time.Now().Add(-time.Hour)
	kept := h.calls[:0]
	for _, t := range h.calls {
		if t.After(cutoff) {
			kept = append(kept, t)
		}
	}
	h.calls = kept
	if len(h.calls) >= aiRateLimit {
		return false
	}
	h.calls = append(h.calls, time.Now())
	return true
}
