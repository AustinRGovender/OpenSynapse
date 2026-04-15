package handlers

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/ai"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
)

type AIHandlers struct {
	aiStore  *db.AIStore
	runStore *db.RunStore
}

func NewAIHandlers(aiStore *db.AIStore, runStore *db.RunStore) *AIHandlers {
	return &AIHandlers{aiStore: aiStore, runStore: runStore}
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

	provider, err := ai.GetProvider(config.Provider)
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

// Analyse runs AI analysis on a run or comparison.
func (h *AIHandlers) Analyse(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RunID    string `json:"run_id"`
		ReportID string `json:"report_id"`
		Question string `json:"question"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}

	if req.RunID == "" && req.ReportID == "" {
		badRequest(w, "VALIDATION_ERROR", "run_id or report_id is required", nil)
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

	// Default question
	question := req.Question
	if question == "" {
		question = "What does this run tell me?"
	}

	// Check cache
	if req.RunID != "" {
		cached, _ := h.aiStore.GetCachedAnalysis(req.RunID, question)
		if cached != nil {
			writeJSON(w, http.StatusOK, cached)
			return
		}
	}

	// Build prompt from run data
	prompt, err := h.buildPrompt(req.RunID, req.ReportID, question)
	if err != nil {
		internalError(w, err)
		return
	}

	// Get provider and key
	key, _ := h.aiStore.GetRawKey()
	provider, err := ai.GetProvider(config.Provider)
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
	if req.RunID != "" {
		runIDPtr = &req.RunID
	}
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

func (h *AIHandlers) buildPrompt(runID, reportID, question string) (string, error) {
	var context string

	if runID != "" {
		run, err := h.runStore.Get(runID)
		if err != nil {
			return "", err
		}
		if run == nil {
			return "", fmt.Errorf("run not found")
		}

		context = fmt.Sprintf(`Performance Test Run Summary:
- Status: %s
- Plan: %s
`, run.Status, run.PlanID)

		if run.Summary != nil {
			s := run.Summary
			context += fmt.Sprintf(`- Total Requests: %d
- Failed Requests: %d
- Error Rate: %.2f%%
- Throughput: %.1f req/s
- Response Time p50: %.1f ms
- Response Time p90: %.1f ms
- Response Time p95: %.1f ms
- Response Time p99: %.1f ms
- Max Response Time: %.1f ms
- Bytes Sent: %d
- Bytes Received: %d
- Thresholds Passed: %v
`,
				s.TotalRequests, s.FailedRequests, s.ErrorRate*100,
				s.ThroughputRPS, s.P50MS, s.P90MS, s.P95MS, s.P99MS, s.MaxMS,
				s.BytesSent, s.BytesReceived, s.ThresholdsPassed)
		}
	}

	prompt := fmt.Sprintf(`You are a performance engineering expert analysing load test results from OpenSynapse, a performance testing platform.

%s
Question: %s

Provide a concise, actionable analysis. Use markdown formatting. Focus on:
1. Key observations from the metrics
2. Potential bottlenecks or concerns
3. Specific recommendations for next steps

Keep your response under 500 words.`, context, question)

	return prompt, nil
}
