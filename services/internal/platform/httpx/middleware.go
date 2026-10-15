package httpx

import (
	"context"
	"fmt"
	"log/slog"
	"net/http"
	"runtime/debug"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/git4jude/opspilot/services/internal/platform/logging"
	"github.com/git4jude/opspilot/services/internal/platform/metrics"
)

// HeaderRequestID carries the request ID between clients and services.
const HeaderRequestID = "X-Request-ID"

// Middleware decorates a handler.
type Middleware func(http.Handler) http.Handler

// Chain applies middleware so that the first one listed is the outermost:
// Chain(h, a, b) == a(b(h)).
func Chain(h http.Handler, mws ...Middleware) http.Handler {
	for i := len(mws) - 1; i >= 0; i-- {
		h = mws[i](h)
	}
	return h
}

// IsProbe reports whether the path is an operational endpoint (health checks
// and metrics). These are excluded from break switches and logged at debug.
func IsProbe(path string) bool {
	return path == "/healthz" || path == "/readyz" || path == "/metrics"
}

// Recover turns a panic into a 500 JSON error and an error log with the stack.
// It is the outermost middleware, so it reads the request ID from the response
// header set by the RequestID middleware.
func Recover(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			p := recover()
			if p == nil {
				return
			}
			if p == http.ErrAbortHandler {
				panic(p) // deliberate abort: let net/http handle it
			}
			requestID := w.Header().Get(HeaderRequestID)
			slog.Default().Error("panic while handling request",
				"request_id", requestID,
				"method", r.Method,
				"error", fmt.Sprint(p),
				"stack", string(debug.Stack()),
			)
			ctx := logging.WithRequestID(r.Context(), requestID)
			WriteError(w, r.WithContext(ctx), http.StatusInternalServerError, CodeInternal,
				"Something went wrong on our side. Please try again.")
		}()
		next.ServeHTTP(w, r)
	})
}

// RequestID reads X-Request-ID (or generates a UUID), stores it in the request
// context, and echoes it in the response header.
func RequestID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get(HeaderRequestID)
		if !validRequestID(id) {
			id = uuid.NewString()
		}
		w.Header().Set(HeaderRequestID, id)

		ctx := logging.WithRequestID(r.Context(), id)
		ctx = context.WithValue(ctx, stateKey{}, &requestState{start: time.Now()})
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// validRequestID accepts client-supplied IDs only if they are short and
// printable, so they cannot be used to forge log lines.
func validRequestID(id string) bool {
	if id == "" || len(id) > 128 {
		return false
	}
	for i := 0; i < len(id); i++ {
		c := id[i]
		if c < 0x21 || c > 0x7e {
			return false
		}
	}
	return true
}

// AccessLog writes one line per request with method, route, status,
// duration_ms, bytes and request_id. Probe endpoints are logged at debug.
func AccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := newRecorder(w)
		defer func() {
			// Runs on panic too, so the log shows a 500 instead of nothing.
			p := recover()
			if p != nil {
				rec.status = http.StatusInternalServerError
			}
			level := slog.LevelInfo
			if IsProbe(r.URL.Path) {
				level = slog.LevelDebug
			}
			logging.FromContext(r.Context()).Log(r.Context(), level, "request completed",
				"method", r.Method,
				"route", Route(r),
				"status", rec.status,
				"duration_ms", time.Since(start).Milliseconds(),
				"bytes", rec.bytes,
			)
			if p != nil {
				panic(p)
			}
		}()
		next.ServeHTTP(rec, r)
	})
}

// Metrics records http_requests_total, http_request_duration_seconds and
// http_requests_in_flight, labelled by route pattern (never the raw path).
func Metrics(m *metrics.Registry) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			rec := newRecorder(w)

			// The route label is only known once the router has matched the
			// request, so the in-flight gauge is incremented from a hook the
			// router calls, and decremented here with the same labels.
			var inFlight prometheus.Gauge
			if s := stateFrom(r); s != nil {
				s.onRoute = func(route string) {
					inFlight = m.HTTPInFlight.WithLabelValues(r.Method, route)
					inFlight.Inc()
				}
			}

			defer func() {
				p := recover()
				if p != nil {
					rec.status = http.StatusInternalServerError
				}
				if inFlight != nil {
					inFlight.Dec()
				}
				route := Route(r)
				status := strconv.Itoa(rec.status)
				m.HTTPRequests.WithLabelValues(r.Method, route, status).Inc()
				m.HTTPDuration.WithLabelValues(r.Method, route, status).Observe(time.Since(start).Seconds())
				if p != nil {
					panic(p)
				}
			}()
			next.ServeHTTP(rec, r)
		})
	}
}

// recorder captures the status code and body size written by a handler.
type recorder struct {
	http.ResponseWriter
	status      int
	bytes       int
	wroteHeader bool
}

func newRecorder(w http.ResponseWriter) *recorder {
	return &recorder{ResponseWriter: w, status: http.StatusOK}
}

func (rec *recorder) WriteHeader(code int) {
	if !rec.wroteHeader {
		rec.status = code
		rec.wroteHeader = true
	}
	rec.ResponseWriter.WriteHeader(code)
}

func (rec *recorder) Write(b []byte) (int, error) {
	rec.wroteHeader = true
	n, err := rec.ResponseWriter.Write(b)
	rec.bytes += n
	return n, err
}

// Unwrap lets http.ResponseController reach the underlying writer.
func (rec *recorder) Unwrap() http.ResponseWriter {
	return rec.ResponseWriter
}
