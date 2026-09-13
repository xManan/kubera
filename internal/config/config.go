// Package config loads runtime configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config is the validated runtime configuration (LLD section 3.1).
type Config struct {
	DatabasePath   string
	Transport      string
	LogLevel       string
	BusyTimeoutMS  int
	MaxPageSize    int
	MigrateOnStart bool
}

// Load reads configuration from the environment, failing startup when the
// database path is missing.
func Load() (Config, error) {
	c := Config{
		DatabasePath:   os.Getenv("KUBERA_DATABASE_PATH"),
		Transport:      getEnv("KUBERA_MCP_TRANSPORT", "stdio"),
		LogLevel:       getEnv("KUBERA_LOG_LEVEL", "info"),
		BusyTimeoutMS:  5000,
		MaxPageSize:    100,
		MigrateOnStart: true,
	}
	if v := os.Getenv("KUBERA_BUSY_TIMEOUT_MS"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return c, fmt.Errorf("KUBERA_BUSY_TIMEOUT_MS must be a positive integer")
		}
		c.BusyTimeoutMS = n
	}
	if v := os.Getenv("KUBERA_MAX_PAGE_SIZE"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n <= 0 {
			return c, fmt.Errorf("KUBERA_MAX_PAGE_SIZE must be a positive integer")
		}
		c.MaxPageSize = n
	}
	if v := os.Getenv("KUBERA_MIGRATE_ON_START"); v != "" {
		c.MigrateOnStart = strings.EqualFold(v, "true") || v == "1"
	}

	switch c.Transport {
	case "stdio":
		// v1 supports stdio only; other values are reserved for future transports.
	default:
		return c, fmt.Errorf("KUBERA_MCP_TRANSPORT %q is not supported (use stdio)", c.Transport)
	}
	switch c.LogLevel {
	case "debug", "info", "warn", "error":
	default:
		return c, fmt.Errorf("KUBERA_LOG_LEVEL must be debug, info, warn, or error")
	}
	if strings.TrimSpace(c.DatabasePath) == "" {
		return c, fmt.Errorf("KUBERA_DATABASE_PATH is required")
	}
	return c, nil
}

func getEnv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
