package config

import "os"

// Mode represents the deployment mode.
type Mode string

const (
	ModeDesktop Mode = "desktop"
	ModeCluster Mode = "cluster"
)

// Config holds runtime configuration derived from environment variables.
type Config struct {
	Mode          Mode
	Port          string
	DBPath        string   // SQLite path (desktop mode)
	DBDSN         string   // Postgres DSN (cluster mode)
	MinIOEndpoint string
	MinIOBucket   string
	MinIOAccessKey string
	MinIOSecretKey string
	TeamMode      bool     // true if user accounts are enabled
}

// Load reads configuration from environment variables.
func Load() *Config {
	c := &Config{
		Mode: ModeDesktop,
		Port: envOr("PORT", "8090"),
	}

	if os.Getenv("OPENSYNAPSE_MODE") == "cluster" {
		c.Mode = ModeCluster
	}

	c.DBPath = os.Getenv("DB_PATH")
	c.DBDSN = os.Getenv("DB_DSN")
	c.MinIOEndpoint = envOr("MINIO_ENDPOINT", "localhost:9000")
	c.MinIOBucket = envOr("MINIO_BUCKET", "opensynapse")
	c.MinIOAccessKey = os.Getenv("MINIO_ACCESS_KEY")
	c.MinIOSecretKey = os.Getenv("MINIO_SECRET_KEY")
	c.TeamMode = os.Getenv("TEAM_MODE") == "true"

	return c
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
