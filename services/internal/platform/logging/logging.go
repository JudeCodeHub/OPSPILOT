// Package logging configures structured JSON logging with log/slog.
//
// Every line carries time, level, msg, service and version. Request-scoped
// lines also carry request_id: store it in the context with WithRequestID and
// get a logger for that request with FromContext.
package logging

import (
	"context"
	"io"
	"log/slog"
	"os"
	"strings"

	"github.com/git4jude/opspilot/services/internal/platform/version"
)

// New builds the service logger (JSON to stdout), installs it as the slog
// default and returns it.
func New(service string, level slog.Level) *slog.Logger {
	return newLogger(os.Stdout, service, level)
}

func newLogger(w io.Writer, service string, level slog.Level) *slog.Logger {
	handler := slog.NewJSONHandler(w, &slog.HandlerOptions{Level: level})
	logger := slog.New(handler).With("service", service, "version", version.Version)
	slog.SetDefault(logger)
	return logger
}

type requestIDKey struct{}

// WithRequestID returns a copy of ctx that carries the request ID.
func WithRequestID(ctx context.Context, id string) context.Context {
	return context.WithValue(ctx, requestIDKey{}, id)
}

// RequestID returns the request ID stored in ctx, or "" if there is none.
func RequestID(ctx context.Context) string {
	id, _ := ctx.Value(requestIDKey{}).(string)
	return id
}

// FromContext returns the default logger, enriched with request_id when ctx carries one.
// Use it in handlers and message consumers so every line can be correlated.
func FromContext(ctx context.Context) *slog.Logger {
	logger := slog.Default()
	if id := RequestID(ctx); id != "" {
		logger = logger.With("request_id", id)
	}
	return logger
}

// MaskEmail hides most of the local part of an email address:
// "jude@example.com" becomes "j***@example.com".
func MaskEmail(email string) string {
	at := strings.LastIndex(email, "@")
	if at <= 0 {
		return "***"
	}
	return email[:1] + "***" + email[at:]
}

// MaskCard keeps only the last four digits of a card number: "************4242".
func MaskCard(number string) string {
	digits := make([]byte, 0, len(number))
	for i := 0; i < len(number); i++ {
		if number[i] >= '0' && number[i] <= '9' {
			digits = append(digits, number[i])
		}
	}
	if len(digits) <= 4 {
		return strings.Repeat("*", len(digits))
	}
	return strings.Repeat("*", len(digits)-4) + string(digits[len(digits)-4:])
}

// Last4 returns the last four digits of a card number ("4242"), for log fields.
func Last4(number string) string {
	masked := MaskCard(number)
	if len(masked) < 4 {
		return ""
	}
	return masked[len(masked)-4:]
}
