// Package config reads service configuration from environment variables (12-factor).
//
// Every helper takes the variable name and a default. An unset or empty variable
// yields the default. A variable that is set but cannot be parsed is a fatal
// startup error: the process logs the problem and exits with status 1. Failing
// fast at startup is far easier to diagnose than a service that runs with a
// silently-ignored setting.
package config

import (
	"fmt"
	"log/slog"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/git4jude/opspilot/services/internal/platform/version"
)

// fatal reports an invalid configuration value and terminates the process.
// It is a variable so tests can replace it with something that does not exit.
var fatal = func(key string, err error) {
	// The application logger is not built yet when config is read, so build a
	// minimal JSON logger with the same base fields.
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With(
		"service", os.Getenv("SERVICE_NAME"),
		"version", version.Version,
	)
	logger.Error("invalid configuration", "key", key, "error", err.Error())
	os.Exit(1)
}

// Fail reports that the variable key holds an invalid value and exits.
// Use it for validation the typed helpers cannot express (e.g. ranges).
func Fail(key string, err error) {
	fatal(key, err)
}

// lookup returns the trimmed value of key and whether it is set to something non-empty.
func lookup(key string) (string, bool) {
	v, ok := os.LookupEnv(key)
	v = strings.TrimSpace(v)
	return v, ok && v != ""
}

// String returns the value of key, or def when it is unset.
func String(key, def string) string {
	if v, ok := lookup(key); ok {
		return v
	}
	return def
}

// Required returns the value of key and exits if it is unset.
func Required(key string) string {
	v, ok := lookup(key)
	if !ok {
		fatal(key, fmt.Errorf("%s is required but not set", key))
	}
	return v
}

// Int returns key parsed as a base-10 integer, or def when it is unset.
func Int(key string, def int) int {
	raw, ok := lookup(key)
	if !ok {
		return def
	}
	v, err := strconv.Atoi(raw)
	if err != nil {
		fatal(key, fmt.Errorf("%s must be an integer, got %q", key, raw))
		return def
	}
	return v
}

// Float returns key parsed as a float64, or def when it is unset.
func Float(key string, def float64) float64 {
	raw, ok := lookup(key)
	if !ok {
		return def
	}
	v, err := strconv.ParseFloat(raw, 64)
	if err != nil {
		fatal(key, fmt.Errorf("%s must be a number, got %q", key, raw))
		return def
	}
	return v
}

// Bool returns key parsed with strconv.ParseBool (1/0, true/false, t/f...),
// or def when it is unset.
func Bool(key string, def bool) bool {
	raw, ok := lookup(key)
	if !ok {
		return def
	}
	v, err := strconv.ParseBool(raw)
	if err != nil {
		fatal(key, fmt.Errorf("%s must be a boolean, got %q", key, raw))
		return def
	}
	return v
}

// Duration returns key parsed with time.ParseDuration ("500ms", "3s", "1m"),
// or def when it is unset.
func Duration(key string, def time.Duration) time.Duration {
	raw, ok := lookup(key)
	if !ok {
		return def
	}
	v, err := time.ParseDuration(raw)
	if err != nil {
		fatal(key, fmt.Errorf("%s must be a duration like 500ms or 3s, got %q", key, raw))
		return def
	}
	return v
}

// Base is the configuration shared by every service.
type Base struct {
	ServiceName       string
	Port              int
	LogLevel          slog.Level
	ShutdownTimeout   time.Duration
	StartupRetries    int
	StartupRetryDelay time.Duration
}

// LoadBase reads the common variables. defaultName is used when SERVICE_NAME is unset.
func LoadBase(defaultName string) Base {
	b := Base{
		ServiceName:       String("SERVICE_NAME", defaultName),
		Port:              Int("PORT", 8080),
		ShutdownTimeout:   Duration("SHUTDOWN_TIMEOUT", 10*time.Second),
		StartupRetries:    Int("STARTUP_RETRIES", 5),
		StartupRetryDelay: Duration("STARTUP_RETRY_DELAY", 2*time.Second),
	}
	if b.Port < 1 || b.Port > 65535 {
		fatal("PORT", fmt.Errorf("PORT must be between 1 and 65535, got %d", b.Port))
	}
	if b.StartupRetries < 1 {
		fatal("STARTUP_RETRIES", fmt.Errorf("STARTUP_RETRIES must be at least 1, got %d", b.StartupRetries))
	}
	level := String("LOG_LEVEL", "info")
	if err := b.LogLevel.UnmarshalText([]byte(level)); err != nil {
		fatal("LOG_LEVEL", fmt.Errorf("LOG_LEVEL must be one of debug, info, warn, error, got %q", level))
	}
	return b
}

// Addr returns the listen address for the HTTP server, e.g. ":8080".
func (b Base) Addr() string {
	return ":" + strconv.Itoa(b.Port)
}
