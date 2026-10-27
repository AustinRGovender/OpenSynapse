// Package config reads the control plane's runtime configuration from
// environment variables. Every variable is documented in README.md.
package config

import (
	"os"
	"path/filepath"
	"strings"
)

// Mode represents the deployment mode.
type Mode string

const (
	ModeDesktop Mode = "desktop"
	ModeCluster Mode = "cluster"
)

// Config holds runtime configuration derived from environment variables.
type Config struct {
	Mode Mode
	Port string
	// Bind is the listen address; loopback unless OPENSYNAPSE_BIND says otherwise.
	Bind string
	// DataDir holds the SQLite database, secret key, and raw run samples.
	DataDir string
	DBPath  string // SQLite path (desktop mode)
	DBDSN   string // Postgres DSN (cluster mode)

	MinIOEndpoint  string
	MinIOBucket    string
	MinIOAccessKey string
	MinIOSecretKey string
	MinIOUseSSL    bool

	TeamMode bool // true if user accounts are enabled

	// AllowedHosts / AllowedOrigins feed the request-security middleware.
	AllowedHosts   []string
	AllowedOrigins []string

	// ControlURL is the base URL k6 uses to reach the control plane's
	// control-state endpoint. Defaults to the loopback listener.
	ControlURL string
}

// Load reads configuration from environment variables.
func Load() (*Config, error) {
	c := &Config{
		Mode: ModeDesktop,
		Port: envOr("PORT", "8090"),
		Bind: envOr("OPENSYNAPSE_BIND", "127.0.0.1"),
	}

	if os.Getenv("OPENSYNAPSE_MODE") == "cluster" {
		c.Mode = ModeCluster
	}

	c.DataDir = os.Getenv("OPENSYNAPSE_DATA_DIR")
	if c.DataDir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return nil, err
		}
		c.DataDir = filepath.Join(home, ".opensynapse")
	}
	if err := os.MkdirAll(c.DataDir, 0o700); err != nil {
		return nil, err
	}

	c.DBPath = os.Getenv("DB_PATH")
	if c.DBPath == "" {
		c.DBPath = filepath.Join(c.DataDir, "opensynapse.db")
	}
	c.DBDSN = os.Getenv("DB_DSN")
	c.MinIOEndpoint = os.Getenv("MINIO_ENDPOINT")
	c.MinIOBucket = envOr("MINIO_BUCKET", "opensynapse")
	c.MinIOAccessKey = os.Getenv("MINIO_ACCESS_KEY")
	c.MinIOSecretKey = os.Getenv("MINIO_SECRET_KEY")
	c.MinIOUseSSL = os.Getenv("MINIO_USE_SSL") == "true"
	c.TeamMode = os.Getenv("TEAM_MODE") == "true"

	c.AllowedHosts = splitList(os.Getenv("OPENSYNAPSE_ALLOWED_HOSTS"))
	c.AllowedOrigins = splitList(envOr("OPENSYNAPSE_ALLOWED_ORIGINS", "http://localhost:5173,http://127.0.0.1:5173"))

	c.ControlURL = os.Getenv("OPENSYNAPSE_CONTROL_URL")
	if c.ControlURL == "" {
		c.ControlURL = "http://127.0.0.1:" + c.Port
	}
	c.ControlURL = strings.TrimRight(c.ControlURL, "/")

	return c, nil
}

// IsCluster returns true if running in cluster (Kubernetes) mode.
func (c *Config) IsCluster() bool {
	return c.Mode == ModeCluster
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func splitList(s string) []string {
	var out []string
	for _, p := range strings.Split(s, ",") {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
