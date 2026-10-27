package db

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/secrets"
)

// Settings are the installation-wide preferences from F-11.
type Settings struct {
	General struct {
		DefaultEnvironmentID string `json:"default_environment_id"`
		DefaultTemplate      string `json:"default_template"`
	} `json:"general"`
	Appearance struct {
		Theme   string `json:"theme"`   // system | light | dark
		Density string `json:"density"` // comfortable | compact
		Palette string `json:"palette"` // default | high-contrast
	} `json:"appearance"`
	Integrations struct {
		WebhookURL      string `json:"webhook_url"`
		SlackWebhookURL string `json:"slack_webhook_url"`
	} `json:"integrations"`
	Storage struct {
		RetentionMode  string `json:"retention_mode"` // keep_all | keep_last_n | keep_days
		RetentionCount int    `json:"retention_count"`
		RetentionDays  int    `json:"retention_days"`
	} `json:"storage"`
}

// DefaultSettings are used until the user saves anything.
func DefaultSettings() Settings {
	var s Settings
	s.Appearance.Theme = "dark"
	s.Appearance.Density = "comfortable"
	s.Appearance.Palette = "default"
	s.Storage.RetentionMode = "keep_all"
	s.Storage.RetentionCount = 100
	s.Storage.RetentionDays = 90
	return s
}

// SettingsStore persists Settings as one JSON document.
type SettingsStore struct {
	db  *sql.DB
	box *secrets.Box
}

func NewSettingsStore(db *sql.DB) *SettingsStore { return &SettingsStore{db: db} }

// UseSecrets encrypts webhook URLs at rest (they embed credentials).
func (s *SettingsStore) UseSecrets(b *secrets.Box) { s.box = b }

const settingsKey = "app"

// Get returns the current settings (defaults merged with saved values).
func (s *SettingsStore) Get() (Settings, error) {
	st := DefaultSettings()
	var raw string
	err := s.db.QueryRow(`SELECT value FROM settings WHERE key = ?`, settingsKey).Scan(&raw)
	if err == sql.ErrNoRows {
		return st, nil
	}
	if err != nil {
		return st, fmt.Errorf("get settings: %w", err)
	}
	if err := json.Unmarshal([]byte(raw), &st); err != nil {
		return st, fmt.Errorf("decode settings: %w", err)
	}
	st.Integrations.WebhookURL, _ = s.box.Open(st.Integrations.WebhookURL)
	st.Integrations.SlackWebhookURL, _ = s.box.Open(st.Integrations.SlackWebhookURL)
	return st, nil
}

// Save replaces the settings document.
func (s *SettingsStore) Save(st Settings) error {
	st.Integrations.WebhookURL = s.box.Seal(st.Integrations.WebhookURL)
	st.Integrations.SlackWebhookURL = s.box.Seal(st.Integrations.SlackWebhookURL)
	raw, _ := json.Marshal(st)
	_, err := s.db.Exec(`INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT (key) DO UPDATE SET value = excluded.value, updated_at = excluded.updated_at`,
		settingsKey, string(raw), time.Now().UTC().Format(time.RFC3339))
	return err
}

// RunsToPrune returns IDs of finished runs a retention policy would delete.
func (s *RunStore) RunsToPrune(mode string, count, days int) ([]string, error) {
	var rows *sql.Rows
	var err error
	finished := `status NOT IN ('queued', 'running')`
	switch mode {
	case "keep_last_n":
		if count < 1 {
			count = 1
		}
		rows, err = s.db.Query(`SELECT id FROM runs WHERE `+finished+` ORDER BY created_at DESC LIMIT -1 OFFSET ?`, count)
	case "keep_days":
		if days < 1 {
			days = 1
		}
		cutoff := time.Now().UTC().AddDate(0, 0, -days).Format(timestampFormat)
		rows, err = s.db.Query(`SELECT id FROM runs WHERE `+finished+` AND created_at < ?`, cutoff)
	default:
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var ids []string
	for rows.Next() {
		var id string
		if rows.Scan(&id) == nil {
			ids = append(ids, id)
		}
	}
	return ids, rows.Err()
}

// Count returns the number of runs.
func (s *RunStore) Count() int {
	var n int
	s.db.QueryRow(`SELECT COUNT(*) FROM runs`).Scan(&n)
	return n
}
