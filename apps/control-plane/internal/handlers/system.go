package handlers

import (
	"context"
	"encoding/json"
	"net/http"
	"runtime"

	"github.com/opensynapse/opensynapse/apps/control-plane/internal/artifacts"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/engine"
	"github.com/opensynapse/opensynapse/apps/control-plane/internal/wsserver"
)

// BuildVersion is set from main at startup (itself set via -ldflags).
var BuildVersion = "dev"

func Health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ok",
	})
}

func Ready(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"status": "ready",
	})
}

func Version(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{
		"version": BuildVersion,
	})
}

// SystemHandlers reports how this installation is set up.
type SystemHandlers struct {
	Engine  *engine.Engine
	WS      *wsserver.Server
	DataDir string
	// Database describes where data lives: a SQLite file, or Postgres.
	Database string
	// DatabaseSize reports the database's size in bytes (nil: unknown).
	DatabaseSize func() int64
	// Artifacts holds raw run samples (a directory, or an S3 bucket).
	Artifacts artifacts.Store
	Mode      string // desktop | cluster
	TeamMode  bool
}

func (h *SystemHandlers) databaseBytes() int64 {
	if h.DatabaseSize == nil {
		return 0
	}
	return h.DatabaseSize()
}

func (h *SystemHandlers) artifactBytes(ctx context.Context, prefix string) int64 {
	if h.Artifacts == nil {
		return 0
	}
	n, _ := h.Artifacts.Size(ctx, prefix)
	return n
}

// SystemInfo is the payload of GET /api/v1/system/info.
type SystemInfo struct {
	Version  string `json:"version"`
	Mode     string `json:"mode"`
	TeamMode bool   `json:"team_mode"`
	DataDir  string `json:"data_dir"`
	Database string `json:"database"`
	// Artifacts is where raw run samples are stored.
	Artifacts string `json:"artifacts"`
	GoOS      string `json:"os"`
	// Execution says whether runs use a local k6 or k6-operator workers.
	Execution engine.Execution `json:"execution"`
	K6        struct {
		Available bool   `json:"available"`
		Version   string `json:"version"`
		KV        bool   `json:"kv"`
	} `json:"k6"`
	LiveControl struct {
		VUs      bool `json:"vus"`
		RPS      bool `json:"rps"`
		Duration bool `json:"duration"`
	} `json:"live_control"`
	WebSocket struct {
		Clients       int   `json:"clients"`
		DroppedFrames int64 `json:"dropped_frames"`
	} `json:"websocket"`
}

func (h *SystemHandlers) Info(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, h.info())
}

func (h *SystemHandlers) info() SystemInfo {
	info := SystemInfo{
		Version:  BuildVersion,
		Mode:     h.Mode,
		TeamMode: h.TeamMode,
		DataDir:  h.DataDir,
		Database: h.Database,
		GoOS:     runtime.GOOS,
	}
	if h.Artifacts != nil {
		info.Artifacts = h.Artifacts.Location()
	}
	if h.Engine != nil {
		caps := h.Engine.Capabilities()
		info.K6.Available = true
		info.K6.Version = caps.Version
		info.K6.KV = caps.KV
		info.LiveControl.VUs = true
		info.LiveControl.Duration = true
		info.LiveControl.RPS = caps.KV
		info.Execution = h.Engine.Execution()
	} else {
		info.Execution = engine.Execution{Mode: "local", MaxWorkers: 1}
	}
	if h.WS != nil {
		info.WebSocket.Clients = h.WS.ClientCount()
		info.WebSocket.DroppedFrames = h.WS.DroppedFrames()
	}
	return info
}

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}
