package httpx

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/git4jude/opspilot/services/internal/platform/logging"
	"github.com/git4jude/opspilot/services/internal/platform/metrics"
)

func decodeError(t *testing.T, rec *httptest.ResponseRecorder) ErrorDetail {
	t.Helper()
	var body ErrorBody
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("response is not the standard error JSON: %v: %s", err, rec.Body.String())
	}
	return body.Error
}

func TestWriteErrorShape(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)
	r = r.WithContext(logging.WithRequestID(r.Context(), "req-123"))
	rec := httptest.NewRecorder()

	WriteError(rec, r, http.StatusConflict, "SEAT_UNAVAILABLE", "Seat A12 is no longer available.")

	if rec.Code != http.StatusConflict {
		t.Errorf("status = %d, want 409", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/json" {
		t.Errorf("Content-Type = %q", ct)
	}
	want := `{"error":{"code":"SEAT_UNAVAILABLE","message":"Seat A12 is no longer available.","request_id":"req-123"}}`
	if got := strings.TrimSpace(rec.Body.String()); got != want {
		t.Errorf("body =\n%s\nwant\n%s", got, want)
	}
}

func TestWriteErrMapsAPIErrorsAndHidesInternalOnes(t *testing.T) {
	r := httptest.NewRequest(http.MethodGet, "/", nil)

	rec := httptest.NewRecorder()
	WriteErr(rec, r, fmt.Errorf("wrapped: %w", NewError(http.StatusGone, "HOLD_EXPIRED", "expired")))
	if rec.Code != http.StatusGone || decodeError(t, rec).Code != "HOLD_EXPIRED" {
		t.Errorf("wrapped *Error not rendered: %d %s", rec.Code, rec.Body.String())
	}

	rec = httptest.NewRecorder()
	WriteErr(rec, r, errors.New("pq: password authentication failed for user admin"))
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("status = %d, want 500", rec.Code)
	}
	if d := decodeError(t, rec); d.Code != CodeInternal || strings.Contains(d.Message, "password") {
		t.Errorf("internal details leaked: %+v", d)
	}
}

func TestRequestIDMiddleware(t *testing.T) {
	var seen string
	h := RequestID(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = logging.RequestID(r.Context())
	}))

	t.Run("propagates incoming ID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set(HeaderRequestID, "abc-123")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if seen != "abc-123" || rec.Header().Get(HeaderRequestID) != "abc-123" {
			t.Errorf("context=%q header=%q, want abc-123", seen, rec.Header().Get(HeaderRequestID))
		}
	})

	t.Run("generates ID when missing", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if len(seen) != 36 || rec.Header().Get(HeaderRequestID) != seen {
			t.Errorf("expected generated UUID echoed in header, got context=%q header=%q", seen, rec.Header().Get(HeaderRequestID))
		}
	})

	t.Run("replaces unsafe ID", func(t *testing.T) {
		r := httptest.NewRequest(http.MethodGet, "/", nil)
		r.Header.Set(HeaderRequestID, "bad id\nwith newline")
		rec := httptest.NewRecorder()
		h.ServeHTTP(rec, r)
		if strings.ContainsAny(seen, " \n") || len(seen) != 36 {
			t.Errorf("unsafe ID was not replaced: %q", seen)
		}
	})
}

func TestChainRecordsRoutePatternAndRecoversPanics(t *testing.T) {
	rt := NewRouter()
	var route string
	rt.HandleFunc("GET /events/{id}", func(w http.ResponseWriter, r *http.Request) {
		route = Route(r)
	})
	rt.HandleFunc("GET /boom", func(http.ResponseWriter, *http.Request) {
		panic("kaboom")
	})
	h := Chain(rt, Recover, RequestID, AccessLog, Metrics(metrics.New("test")))

	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/events/123", nil))
	if route != "/events/{id}" {
		t.Errorf("route = %q, want the pattern /events/{id}", route)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))
	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("panic status = %d, want 500", rec.Code)
	}
	if d := decodeError(t, rec); d.RequestID == "" || d.RequestID != rec.Header().Get(HeaderRequestID) {
		t.Errorf("panic response should carry the request ID, got %+v", d)
	}

	rec = httptest.NewRecorder()
	h.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/nope", nil))
	if rec.Code != http.StatusNotFound || decodeError(t, rec).Code != CodeNotFound {
		t.Errorf("unknown route: %d %s", rec.Code, rec.Body.String())
	}
}
