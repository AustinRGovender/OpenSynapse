package handlers

import (
	"archive/zip"
	"encoding/json"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/db"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/observability"
)

// SettingsHandlers serve Settings (F-11): preferences, storage, diagnostics and logs.
type SettingsHandlers struct {
	store  *db.SettingsStore
	runs   *db.RunStore
	system *SystemHandlers
}

func NewSettingsHandlers(store *db.SettingsStore, runs *db.RunStore, system *SystemHandlers) *SettingsHandlers {
	return &SettingsHandlers{store: store, runs: runs, system: system}
}

// maskURL hides the secret part of a webhook URL while keeping it recognisable.
func maskURL(u string) string {
	if u == "" {
		return ""
	}
	if len(u) <= 16 {
		return db.RedactedValue
	}
	return u[:strings.Index(u[8:], "/")+9] + "…" + u[len(u)-4:]
}

func (h *SettingsHandlers) Get(w http.ResponseWriter, r *http.Request) {
	st, err := h.store.Get()
	if err != nil {
		internalError(w, err)
		return
	}
	st.Integrations.WebhookURL = maskURL(st.Integrations.WebhookURL)
	st.Integrations.SlackWebhookURL = maskURL(st.Integrations.SlackWebhookURL)
	writeJSON(w, http.StatusOK, st)
}

func validChoice(v string, allowed ...string) bool {
	for _, a := range allowed {
		if v == a {
			return true
		}
	}
	return false
}

func (h *SettingsHandlers) Update(w http.ResponseWriter, r *http.Request) {
	current, err := h.store.Get()
	if err != nil {
		internalError(w, err)
		return
	}
	var st db.Settings
	if err := json.NewDecoder(r.Body).Decode(&st); err != nil {
		badRequest(w, "INVALID_JSON", "Request body is not valid JSON", nil)
		return
	}
	a := &st.Appearance
	if !validChoice(a.Theme, "system", "light", "dark") || !validChoice(a.Density, "comfortable", "compact") || !validChoice(a.Palette, "default", "high-contrast") {
		badRequest(w, "VALIDATION_ERROR", "appearance: theme must be system/light/dark, density comfortable/compact, palette default/high-contrast", nil)
		return
	}
	if !validChoice(st.Storage.RetentionMode, "keep_all", "keep_last_n", "keep_days") || st.Storage.RetentionCount < 0 || st.Storage.RetentionDays < 0 {
		badRequest(w, "VALIDATION_ERROR", "storage: retention_mode must be keep_all, keep_last_n or keep_days", nil)
		return
	}
	// Masked values sent back mean "unchanged".
	for _, p := range []struct{ in, cur *string }{
		{&st.Integrations.WebhookURL, &current.Integrations.WebhookURL},
		{&st.Integrations.SlackWebhookURL, &current.Integrations.SlackWebhookURL},
	} {
		if *p.in != "" && *p.in == maskURL(*p.cur) {
			*p.in = *p.cur
		}
		if *p.in != "" && !strings.HasPrefix(*p.in, "http://") && !strings.HasPrefix(*p.in, "https://") {
			badRequest(w, "VALIDATION_ERROR", "Webhook URLs must start with http:// or https://", nil)
			return
		}
	}
	if err := h.store.Save(st); err != nil {
		internalError(w, err)
		return
	}
	h.Get(w, r)
}

func dirSize(path string) int64 {
	var total int64
	filepath.WalkDir(path, func(_ string, d fs.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			if info, err := d.Info(); err == nil {
				total += info.Size()
			}
		}
		return nil
	})
	return total
}

func fileSize(path string) int64 {
	if info, err := os.Stat(path); err == nil {
		return info.Size()
	}
	return 0
}

// Storage reports where data lives, how big it is, and what the retention
// policy would remove right now.
func (h *SettingsHandlers) Storage(w http.ResponseWriter, r *http.Request) {
	st, _ := h.store.Get()
	prune, _ := h.runs.RunsToPrune(st.Storage.RetentionMode, st.Storage.RetentionCount, st.Storage.RetentionDays)
	var pruneBytes int64
	for _, id := range prune {
		pruneBytes += dirSize(filepath.Join(h.system.DataDir, "runs", id))
	}
	dbBytes := fileSize(h.system.DBPath) + fileSize(h.system.DBPath+"-wal")
	writeJSON(w, http.StatusOK, map[string]any{
		"data_dir":           h.system.DataDir,
		"database":           h.system.DBPath,
		"database_bytes":     dbBytes,
		"samples_bytes":      dirSize(filepath.Join(h.system.DataDir, "runs")),
		"runs":               h.runs.Count(),
		"retention":          st.Storage,
		"prunable_runs":      len(prune),
		"prunable_bytes_est": pruneBytes,
	})
}

// Cleanup applies the retention policy now.
func (h *SettingsHandlers) Cleanup(w http.ResponseWriter, r *http.Request) {
	n, err := h.ApplyRetention()
	if err != nil {
		internalError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted_runs": n})
}

// ApplyRetention deletes finished runs the policy no longer keeps.
func (h *SettingsHandlers) ApplyRetention() (int, error) {
	st, err := h.store.Get()
	if err != nil {
		return 0, err
	}
	ids, err := h.runs.RunsToPrune(st.Storage.RetentionMode, st.Storage.RetentionCount, st.Storage.RetentionDays)
	if err != nil {
		return 0, err
	}
	for _, id := range ids {
		if h.runs.Delete(id) == nil && h.system.DataDir != "" {
			os.RemoveAll(filepath.Join(h.system.DataDir, "runs", id))
		}
	}
	return len(ids), nil
}

// Logs returns recent control-plane log lines (the developer log drawer).
func (h *SettingsHandlers) Logs(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"lines": observability.Logs.Lines()})
}

// Diagnostics builds a zip with system info, settings (secrets removed),
// and recent logs, for attaching to a bug report. Contains no secrets.
func (h *SettingsHandlers) Diagnostics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/zip")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="opensynapse-diagnostics-%s.zip"`, time.Now().UTC().Format("20060102-150405")))
	zw := zip.NewWriter(w)
	defer zw.Close()

	add := func(name string, v any) {
		f, err := zw.Create(name)
		if err != nil {
			return
		}
		enc := json.NewEncoder(f)
		enc.SetIndent("", "  ")
		enc.Encode(v)
	}
	add("system.json", h.system.info())
	if st, err := h.store.Get(); err == nil {
		if st.Integrations.WebhookURL != "" {
			st.Integrations.WebhookURL = db.RedactedValue
		}
		if st.Integrations.SlackWebhookURL != "" {
			st.Integrations.SlackWebhookURL = db.RedactedValue
		}
		add("settings.json", st)
	}
	add("storage.json", map[string]any{"runs": h.runs.Count(), "database_bytes": fileSize(h.system.DBPath)})
	if f, err := zw.Create("logs.txt"); err == nil {
		for _, l := range observability.Logs.Lines() {
			fmt.Fprintln(f, redactLogLine(l))
		}
	}
}

// redactLogLine removes anything that looks like a credential.
func redactLogLine(l string) string {
	for _, p := range []string{"osl_", "oss_", "osk_", "sk-", "Bearer "} {
		for {
			i := strings.Index(l, p)
			if i < 0 {
				break
			}
			end := i + len(p)
			for end < len(l) && l[end] != ' ' && l[end] != '"' && l[end] != ',' {
				end++
			}
			l = l[:i] + "[redacted]" + l[end:]
		}
	}
	return l
}
