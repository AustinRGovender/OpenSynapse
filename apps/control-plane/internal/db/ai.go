package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

type AIConfig struct {
	Provider       string   `json:"provider"`
	APIKeyMasked   string   `json:"api_key_masked"`   // Only last 4 chars shown
	Model          string   `json:"model"`
	MonthlyCap     *float64 `json:"monthly_cap_usd"`
	Enabled        bool     `json:"enabled"`
	AzureEndpoint  string   `json:"azure_endpoint,omitempty"`
}

type AIAnalysis struct {
	ID           string  `json:"id"`
	RunID        *string `json:"run_id,omitempty"`
	ReportID     *string `json:"report_id,omitempty"`
	Question     string  `json:"question"`
	Prompt       string  `json:"prompt"`
	Response     string  `json:"response"`
	Provider     string  `json:"provider"`
	Model        string  `json:"model"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostUSD      float64 `json:"cost_usd"`
	CreatedAt    time.Time `json:"created_at"`
}

type AIStore struct {
	db *sql.DB
}

func NewAIStore(db *sql.DB) *AIStore {
	return &AIStore{db: db}
}

// GetConfig returns the current AI configuration with the key masked.
func (s *AIStore) GetConfig() (*AIConfig, error) {
	var provider, keyEnc, model, azureEndpoint string
	var capUSD sql.NullFloat64
	var enabled int

	err := s.db.QueryRow(
		`SELECT provider, api_key_encrypted, model, monthly_cap_usd, enabled, azure_endpoint
		 FROM ai_config WHERE id = 'default'`,
	).Scan(&provider, &keyEnc, &model, &capUSD, &enabled, &azureEndpoint)
	if err == sql.ErrNoRows {
		return &AIConfig{}, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get ai config: %w", err)
	}

	config := &AIConfig{
		Provider:      provider,
		APIKeyMasked:  maskKey(keyEnc),
		Model:         model,
		Enabled:       enabled == 1,
		AzureEndpoint: azureEndpoint,
	}
	if capUSD.Valid {
		config.MonthlyCap = &capUSD.Float64
	}
	return config, nil
}

// GetRawKey returns the unmasked API key (for internal use only — never expose via API).
func (s *AIStore) GetRawKey() (string, error) {
	var key string
	err := s.db.QueryRow(`SELECT api_key_encrypted FROM ai_config WHERE id = 'default'`).Scan(&key)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return key, err
}

// SaveConfig saves the AI configuration. If apiKey is empty, the existing key is preserved.
func (s *AIStore) SaveConfig(provider, apiKey, model string, cap *float64, enabled bool, azureEndpoint string) error {
	now := time.Now().UTC().Format(time.RFC3339)
	enabledInt := 0
	if enabled {
		enabledInt = 1
	}

	// If no key provided, keep existing
	if apiKey == "" {
		existing, _ := s.GetRawKey()
		apiKey = existing
	}

	_, err := s.db.Exec(
		`INSERT INTO ai_config (id, provider, api_key_encrypted, model, monthly_cap_usd, enabled, azure_endpoint, updated_at)
		 VALUES ('default', ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(id) DO UPDATE SET provider=?, api_key_encrypted=?, model=?, monthly_cap_usd=?, enabled=?, azure_endpoint=?, updated_at=?`,
		provider, apiKey, model, cap, enabledInt, azureEndpoint, now,
		provider, apiKey, model, cap, enabledInt, azureEndpoint, now,
	)
	return err
}

// CacheAnalysis stores an AI analysis result.
func (s *AIStore) CacheAnalysis(runID, reportID *string, question, prompt, response, provider, model string, inputTokens, outputTokens int, costUSD float64) (*AIAnalysis, error) {
	id := uuid.New().String()
	now := time.Now().UTC()

	_, err := s.db.Exec(
		`INSERT INTO ai_analyses (id, run_id, report_id, question, prompt, response, provider, model, input_tokens, output_tokens, cost_usd, created_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		id, runID, reportID, question, prompt, response, provider, model, inputTokens, outputTokens, costUSD, now.Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("cache analysis: %w", err)
	}

	// Track usage
	s.db.Exec(
		`INSERT INTO ai_usage (id, provider, model, input_tokens, output_tokens, cost_usd, created_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		uuid.New().String(), provider, model, inputTokens, outputTokens, costUSD, now.Format(time.RFC3339),
	)

	return &AIAnalysis{
		ID:           id,
		RunID:        runID,
		ReportID:     reportID,
		Question:     question,
		Prompt:       prompt,
		Response:     response,
		Provider:     provider,
		Model:        model,
		InputTokens:  inputTokens,
		OutputTokens: outputTokens,
		CostUSD:      costUSD,
		CreatedAt:    now,
	}, nil
}

// GetCachedAnalysis looks up a cached analysis for a run/question combination.
func (s *AIStore) GetCachedAnalysis(runID, question string) (*AIAnalysis, error) {
	var a AIAnalysis
	var rID, repID sql.NullString
	var createdAt string

	err := s.db.QueryRow(
		`SELECT id, run_id, report_id, question, prompt, response, provider, model, input_tokens, output_tokens, cost_usd, created_at
		 FROM ai_analyses WHERE run_id = ? AND question = ? ORDER BY created_at DESC LIMIT 1`,
		runID, question,
	).Scan(&a.ID, &rID, &repID, &a.Question, &a.Prompt, &a.Response, &a.Provider, &a.Model, &a.InputTokens, &a.OutputTokens, &a.CostUSD, &createdAt)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("get cached analysis: %w", err)
	}

	if rID.Valid { a.RunID = &rID.String }
	if repID.Valid { a.ReportID = &repID.String }
	a.CreatedAt, _ = time.Parse(time.RFC3339, createdAt)
	return &a, nil
}

// GetMonthlyUsage returns total cost this month.
func (s *AIStore) GetMonthlyUsage() (float64, error) {
	monthStart := time.Now().UTC().Format("2006-01") + "-01"
	var total sql.NullFloat64
	err := s.db.QueryRow(
		`SELECT SUM(cost_usd) FROM ai_usage WHERE created_at >= ?`, monthStart,
	).Scan(&total)
	if err != nil {
		return 0, err
	}
	if total.Valid {
		return total.Float64, nil
	}
	return 0, nil
}

func maskKey(key string) string {
	if len(key) <= 4 {
		return "****"
	}
	return "****" + key[len(key)-4:]
}

// For JSON serialization in tests
func (a AIConfig) MarshalJSON() ([]byte, error) {
	type Alias AIConfig
	return json.Marshal(struct{ Alias }{Alias(a)})
}
