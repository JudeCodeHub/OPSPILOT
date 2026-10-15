// Package health implements the liveness and readiness endpoints.
//
//   - GET /healthz (liveness) answers 200 whenever the process can serve HTTP.
//     It never checks dependencies: a database outage must not make Kubernetes
//     restart every pod, which would only add a second outage to the first.
//   - GET /readyz (readiness) runs the registered dependency checks and answers
//     200 or 503 with the result of each. It answers 503 while shutting down so
//     load balancers stop sending traffic before the server stops.
package health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"sync/atomic"
	"time"
)

// CheckTimeout bounds each readiness check.
const CheckTimeout = time.Second

// Check reports whether one dependency is usable. Return nil when healthy.
type Check func(ctx context.Context) error

type namedCheck struct {
	name  string
	check Check
}

// Health holds the readiness checks and the shutdown flag.
type Health struct {
	mu           sync.RWMutex
	checks       []namedCheck
	shuttingDown atomic.Bool
}

// New returns an empty Health with no checks (always ready).
func New() *Health {
	return &Health{}
}

// Add registers a readiness check, e.g. Add("postgres", pool.Ping).
func (h *Health) Add(name string, check Check) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.checks = append(h.checks, namedCheck{name: name, check: check})
}

// SetShuttingDown makes /readyz answer 503 from now on.
func (h *Health) SetShuttingDown() {
	h.shuttingDown.Store(true)
}

// Liveness is the GET /healthz handler.
func (h *Health) Liveness(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}

// readiness is the JSON body of GET /readyz.
type readiness struct {
	Status string            `json:"status"`
	Checks map[string]string `json:"checks"`
}

// Readiness is the GET /readyz handler. Checks run concurrently.
func (h *Health) Readiness(w http.ResponseWriter, r *http.Request) {
	if h.shuttingDown.Load() {
		writeJSON(w, http.StatusServiceUnavailable, readiness{Status: "shutting_down", Checks: map[string]string{}})
		return
	}

	h.mu.RLock()
	checks := h.checks
	h.mu.RUnlock()

	results := make(map[string]string, len(checks))
	var (
		mu     sync.Mutex
		wg     sync.WaitGroup
		failed atomic.Bool
	)
	for _, c := range checks {
		wg.Go(func() {
			ctx, cancel := context.WithTimeout(r.Context(), CheckTimeout)
			defer cancel()
			result := "ok"
			if err := c.check(ctx); err != nil {
				failed.Store(true)
				result = "error: " + err.Error()
			}
			mu.Lock()
			results[c.name] = result
			mu.Unlock()
		})
	}
	wg.Wait()

	if failed.Load() {
		writeJSON(w, http.StatusServiceUnavailable, readiness{Status: "unavailable", Checks: results})
		return
	}
	writeJSON(w, http.StatusOK, readiness{Status: "ok", Checks: results})
}

// Probe performs an HTTP GET against the local /healthz endpoint and returns a
// process exit code (0 healthy, 1 unhealthy). Distroless images have no shell
// or curl, so the binary probes itself: `/app healthcheck`.
func Probe(port int) int {
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get(fmt.Sprintf("http://127.0.0.1:%d/healthz", port))
	if err != nil {
		return 1
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return 1
	}
	return 0
}

func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.Header().Set("Cache-Control", "no-store")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}
