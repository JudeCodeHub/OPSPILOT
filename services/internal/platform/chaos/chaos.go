// Package chaos implements the demo app's failure switches.
//
// The switches are read once from the environment at startup and make the
// service misbehave the way real services misbehave: failed requests, slow
// responses, a growing heap, a saturated CPU.
//
// IMPORTANT: the package name is internal only. Nothing this package emits at
// runtime (logs, error messages, metric names, HTTP responses) may reveal that
// a failure is deliberate. An investigator must diagnose it from symptoms, as
// in a real incident. Switch values are never logged.
package chaos

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"runtime"
	"sync"
	"time"

	"github.com/git4jude/opspilot/services/internal/platform/config"
	"github.com/git4jude/opspilot/services/internal/platform/httpx"
	"github.com/git4jude/opspilot/services/internal/platform/logging"
)

// Config holds the generic switches available in every service.
type Config struct {
	FailRate     float64       // FAIL_RATE: fraction of requests answered with a 500 (0.0–1.0)
	Slow         time.Duration // SLOW_MS: extra latency per request (±20% jitter)
	LeakMemory   bool          // LEAK_MEMORY: continuously allocate and retain memory
	LeakMBPerMin int           // LEAK_MB_PER_MIN: leak speed
	CPUBurn      bool          // CPU_BURN: keep every available CPU busy
}

// Load reads the generic switches from the environment.
func Load() Config {
	c := Config{
		FailRate:     config.Float("FAIL_RATE", 0),
		Slow:         time.Duration(config.Int("SLOW_MS", 0)) * time.Millisecond,
		LeakMemory:   config.Bool("LEAK_MEMORY", false),
		LeakMBPerMin: config.Int("LEAK_MB_PER_MIN", 20),
		CPUBurn:      config.Bool("CPU_BURN", false),
	}
	if c.FailRate < 0 || c.FailRate > 1 {
		config.Fail("FAIL_RATE", fmt.Errorf("FAIL_RATE must be between 0 and 1, got %v", c.FailRate))
	}
	if c.Slow < 0 {
		config.Fail("SLOW_MS", errors.New("SLOW_MS must not be negative"))
	}
	if c.LeakMBPerMin < 0 {
		config.Fail("LEAK_MB_PER_MIN", errors.New("LEAK_MB_PER_MIN must not be negative"))
	}
	return c
}

// Switches applies a Config to a running service.
type Switches struct {
	cfg Config

	// failure is the error logged when a request is failed by FAIL_RATE. Each
	// service supplies a message that is plausible for what it does, e.g.
	// "payment processor error: upstream connection reset".
	failure error

	// OnFailure, if set, is called for each request failed by FAIL_RATE so the
	// service can count it in its own metrics (e.g. payment_charges_total{result="error"}).
	OnFailure func(r *http.Request)
}

// New returns the switches for a service. failure is what gets logged when a
// request is failed by FAIL_RATE.
func New(cfg Config, failure error) *Switches {
	return &Switches{cfg: cfg, failure: failure}
}

// Config returns the loaded switch values.
func (s *Switches) Config() Config { return s.cfg }

// Middleware applies FAIL_RATE and SLOW_MS to every route except /healthz,
// /readyz and /metrics.
func (s *Switches) Middleware(next http.Handler) http.Handler {
	if s.cfg.FailRate == 0 && s.cfg.Slow == 0 {
		return next
	}
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if httpx.IsProbe(r.URL.Path) {
			next.ServeHTTP(w, r)
			return
		}
		if s.cfg.Slow > 0 && !Sleep(r.Context(), Jitter(s.cfg.Slow)) {
			return // client went away while we were slow
		}
		if s.cfg.FailRate > 0 && rand.Float64() < s.cfg.FailRate {
			if s.OnFailure != nil {
				s.OnFailure(r)
			}
			logging.FromContext(r.Context()).Error("request failed",
				"method", r.Method, "path", r.URL.Path, "error", s.failure.Error())
			httpx.WriteError(w, r, http.StatusInternalServerError, httpx.CodeInternal,
				"Something went wrong on our side. Please try again.")
			return
		}
		next.ServeHTTP(w, r)
	})
}

// Jitter returns d ±20%.
func Jitter(d time.Duration) time.Duration {
	if d <= 0 {
		return 0
	}
	factor := 0.8 + rand.Float64()*0.4
	return time.Duration(float64(d) * factor)
}

// Sleep waits for d or until ctx is done. It reports whether the full duration elapsed.
func Sleep(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return true
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-t.C:
		return true
	case <-ctx.Done():
		return false
	}
}

// Start launches the background behaviours (CPU burn, memory growth) until ctx ends.
func (s *Switches) Start(ctx context.Context) {
	s.StartCPUBurn(ctx)
	if s.cfg.LeakMemory {
		go RetainAtRate(ctx, s.cfg.LeakMBPerMin, defaultStore.Retain)
	}
}

// StartCPUBurn keeps one goroutine per usable CPU busy hashing data.
func (s *Switches) StartCPUBurn(ctx context.Context) {
	if !s.cfg.CPUBurn {
		return
	}
	for range runtime.GOMAXPROCS(0) {
		go burn(ctx)
	}
}

func burn(ctx context.Context) {
	buf := make([]byte, 4096)
	for i := 0; ; i++ {
		sum := sha256.Sum256(buf)
		buf[i%len(buf)] = sum[0]
		if i%1024 == 0 && ctx.Err() != nil {
			return
		}
	}
}

// RetainAtRate calls retain once per second with enough bytes to reach
// mbPerMin megabytes per minute, until ctx ends.
func RetainAtRate(ctx context.Context, mbPerMin int, retain func([]byte)) {
	if mbPerMin <= 0 {
		return
	}
	perSecond := mbPerMin * (1 << 20) / 60
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			retain(Allocate(perSecond))
		}
	}
}

// Allocate returns n bytes that are actually resident in memory. A fresh
// []byte is backed by untouched zero pages that the OS may not count in RSS,
// so one byte per 4 KiB page is written.
func Allocate(n int) []byte {
	b := make([]byte, n)
	for i := 0; i < len(b); i += 4096 {
		b[i] = byte(i)
	}
	return b
}

// store keeps references forever so the garbage collector can never free them.
type store struct {
	mu     sync.Mutex
	chunks [][]byte
}

func (s *store) Retain(b []byte) {
	s.mu.Lock()
	s.chunks = append(s.chunks, b)
	s.mu.Unlock()
}

var defaultStore = &store{}
