package handlers

import (
	"encoding/json"
	"net/http"
	"runtime"

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
	Engine   *engine.Engine
	WS       *wsserver.Server
	DataDir  string
	DBPath   string
	Mode     string // desktop | cluster
	TeamMode bool
}

// SystemInfo is the payload of GET /api/v1/system/info.
type SystemInfo struct {
	Version  string `json:"version"`
	Mode     string `json:"mode"`
	TeamMode bool   `json:"team_mode"`
	DataDir  string `json:"data_dir"`
	Database string `json:"database"`
	GoOS     string `json:"os"`
	K6       struct {
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
		Database: h.DBPath,
		GoOS:     runtime.GOOS,
	}
	if h.Engine != nil {
		caps := h.Engine.Capabilities()
		info.K6.Available = true
		info.K6.Version = caps.Version
		info.K6.KV = caps.KV
		info.LiveControl.VUs = true
		info.LiveControl.Duration = true
		info.LiveControl.RPS = caps.KV
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
