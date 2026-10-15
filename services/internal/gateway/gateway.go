// Package gateway is the single public entrypoint (/api/*). It forwards
// requests to the backend services without business logic and without retries.
//
// Error mapping:
//   - upstream timed out                     → 504 UPSTREAM_TIMEOUT
//   - upstream unreachable                   → 502 UPSTREAM_UNAVAILABLE
//   - upstream 5xx without a standard body   → 502 UPSTREAM_UNAVAILABLE
//   - upstream 4xx, or 5xx with a standard
//     error body (e.g. 504 PAYMENT_TIMEOUT)  → passed through unchanged
package gateway

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"

	"github.com/git4jude/opspilot/services/internal/platform/httpx"
	"github.com/git4jude/opspilot/services/internal/platform/logging"
)

// Error codes produced by the gateway itself.
const (
	CodeUpstreamTimeout     = "UPSTREAM_TIMEOUT"
	CodeUpstreamUnavailable = "UPSTREAM_UNAVAILABLE"
)

// Gateway holds one client per backend.
type Gateway struct {
	Events    *httpx.Client
	Booking   *httpx.Client
	Ticketing *httpx.Client
}

// Register adds the /api routes to the router.
func (g *Gateway) Register(rt *httpx.Router) {
	rt.Handle("GET /api/events", g.forward(g.Events, func(*http.Request) string { return "/events" }))
	rt.Handle("GET /api/events/{id}", g.forward(g.Events, func(r *http.Request) string {
		return "/events/" + url.PathEscape(r.PathValue("id"))
	}))
	rt.Handle("GET /api/events/{id}/seats", g.forward(g.Events, func(r *http.Request) string {
		return "/events/" + url.PathEscape(r.PathValue("id")) + "/seats"
	}))
	rt.Handle("POST /api/holds", g.forward(g.Booking, func(*http.Request) string { return "/holds" }))
	rt.Handle("DELETE /api/holds/{holdId}", g.forward(g.Booking, func(r *http.Request) string {
		return "/holds/" + url.PathEscape(r.PathValue("holdId"))
	}))
	rt.Handle("POST /api/holds/{holdId}/checkout", g.forward(g.Booking, func(r *http.Request) string {
		return "/holds/" + url.PathEscape(r.PathValue("holdId")) + "/checkout"
	}))
	rt.Handle("GET /api/bookings/{id}", g.forward(g.Booking, func(r *http.Request) string {
		return "/bookings/" + url.PathEscape(r.PathValue("id"))
	}))
	rt.Handle("GET /api/tickets", g.forward(g.Ticketing, func(r *http.Request) string {
		return "/tickets?" + url.Values{"email": {r.URL.Query().Get("email")}}.Encode()
	}))
}

// forward returns a handler that sends the request to upstream at the path
// built by target, and relays the response.
func (g *Gateway) forward(upstream *httpx.Client, target func(*http.Request) string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body []byte
		if r.Body != nil && r.Method != http.MethodGet && r.Method != http.MethodDelete {
			var err error
			body, err = io.ReadAll(http.MaxBytesReader(w, r.Body, httpx.MaxBodyBytes))
			if err != nil {
				httpx.WriteError(w, r, http.StatusRequestEntityTooLarge, httpx.CodeBadRequest, "Request body is too large.")
				return
			}
		}
		header := http.Header{}
		if ct := r.Header.Get("Content-Type"); ct != "" {
			header.Set("Content-Type", ct)
		}

		log := logging.FromContext(r.Context())
		resp, err := upstream.Do(r.Context(), r.Method, target(r), body, header)
		if err != nil {
			if httpx.IsTimeout(err) {
				log.Error("upstream request timed out", "error", err.Error())
				httpx.WriteError(w, r, http.StatusGatewayTimeout, CodeUpstreamTimeout,
					"The service took too long to respond. Please try again.")
				return
			}
			log.Error("upstream request failed", "error", err.Error())
			httpx.WriteError(w, r, http.StatusBadGateway, CodeUpstreamUnavailable,
				"A backend service is unavailable. Please try again shortly.")
			return
		}

		if resp.Status >= 500 {
			if !isErrorBody(resp.Body) {
				log.Error("upstream returned an error", "status", resp.Status)
				httpx.WriteError(w, r, http.StatusBadGateway, CodeUpstreamUnavailable,
					"A backend service is unavailable. Please try again shortly.")
				return
			}
			log.Warn("upstream returned an error", "status", resp.Status)
		}

		if ct := resp.Header.Get("Content-Type"); ct != "" {
			w.Header().Set("Content-Type", ct)
		}
		w.Header().Set("Cache-Control", "no-store")
		w.WriteHeader(resp.Status)
		_, _ = w.Write(resp.Body)
	})
}

// isErrorBody reports whether b is a standard {"error":{"code":...}} body.
func isErrorBody(b []byte) bool {
	var body httpx.ErrorBody
	return json.Unmarshal(b, &body) == nil && body.Error.Code != ""
}

// CORS allows the Vite dev server (a different origin) to call the API
// directly. In Compose and Kubernetes the frontend uses the same origin.
func CORS(allowedOrigin string) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if origin := r.Header.Get("Origin"); origin != "" && origin == allowedOrigin {
				h := w.Header()
				h.Set("Access-Control-Allow-Origin", origin)
				h.Set("Access-Control-Allow-Methods", "GET, POST, DELETE, OPTIONS")
				h.Set("Access-Control-Allow-Headers", "Content-Type, X-Request-ID")
				h.Set("Access-Control-Expose-Headers", "X-Request-ID")
				h.Set("Access-Control-Max-Age", "600")
				h.Add("Vary", "Origin")
				if r.Method == http.MethodOptions {
					w.WriteHeader(http.StatusNoContent)
					return
				}
			}
			next.ServeHTTP(w, r)
		})
	}
}
