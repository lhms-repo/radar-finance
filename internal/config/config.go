// Package config reads the service configuration from environment variables.
package config

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Location is the time zone used for every business date (America/Sao_Paulo).
const Location = "America/Sao_Paulo"

// Config holds every setting the service reads at startup.
type Config struct {
	// Addr is the HTTP listen address.
	Addr string
	// DBPath is the SQLite database file path.
	DBPath string
	// BackupDir is the folder that receives the backup copies (RNF-05).
	BackupDir string
	// BackupHour is the local hour of the daily backup.
	BackupHour int
	// BackupKeep is how many backup copies are kept.
	BackupKeep int
	// SessionIdle is the inactivity period after which a session expires (RNF-10).
	SessionIdle time.Duration
	// SessionLifetime is the absolute maximum age of a session.
	SessionLifetime time.Duration
	// CookieSecure marks the session cookie as HTTPS only.
	CookieSecure bool
	// Loc is the loaded business time zone.
	Loc *time.Location
}

// Load reads the configuration from the environment, applying defaults.
func Load() (Config, error) {
	loc, err := time.LoadLocation(Location)
	if err != nil {
		return Config{}, fmt.Errorf("load time zone: %w", err)
	}
	c := Config{
		Addr:      env("RADAR_ADDR", ":8080"),
		DBPath:    env("RADAR_DB_PATH", "/data/radar.db"),
		BackupDir: env("RADAR_BACKUP_DIR", "/backups"),
		Loc:       loc,
	}
	if c.BackupHour, err = envInt("RADAR_BACKUP_HOUR", 3); err != nil {
		return Config{}, err
	}
	if c.BackupHour < 0 || c.BackupHour > 23 {
		return Config{}, fmt.Errorf("RADAR_BACKUP_HOUR must be between 0 and 23")
	}
	if c.BackupKeep, err = envInt("RADAR_BACKUP_KEEP", 30); err != nil {
		return Config{}, err
	}
	if c.BackupKeep < 1 {
		return Config{}, fmt.Errorf("RADAR_BACKUP_KEEP must be at least 1")
	}
	if c.SessionIdle, err = envDuration("RADAR_SESSION_IDLE", 7*24*time.Hour); err != nil {
		return Config{}, err
	}
	if c.SessionLifetime, err = envDuration("RADAR_SESSION_LIFETIME", 90*24*time.Hour); err != nil {
		return Config{}, err
	}
	if c.CookieSecure, err = envBool("RADAR_COOKIE_SECURE", true); err != nil {
		return Config{}, err
	}
	return c, nil
}

// env returns the variable value or the fallback when it is empty.
func env(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

// envInt parses an integer variable, returning the fallback when it is empty.
func envInt(key string, fallback int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	return n, nil
}

// envBool parses a boolean variable, returning the fallback when it is empty.
func envBool(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("%s: %w", key, err)
	}
	return b, nil
}

// envDuration parses a Go duration variable, returning the fallback when it is empty.
func envDuration(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("%s: %w", key, err)
	}
	if d <= 0 {
		return 0, fmt.Errorf("%s must be positive", key)
	}
	return d, nil
}
