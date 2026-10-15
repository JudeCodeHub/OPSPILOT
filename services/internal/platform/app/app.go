// Package app wires the platform packages together so that every service's
// main.go only contains what is specific to that service.
//
// A typical main:
//
//	a := app.New("events", failureErr)     // config, logger, metrics, health, switches
//	pool, err := deps.Postgres(...)         // connect dependencies
//	a.Fatal(err)
//	a.OnShutdown(pool.Close)                // closed after HTTP has drained
//	a.Health.Add("postgres", pool.Ping)     // readiness check
//	events.Register(a.Router, ...)          // routes
//	a.Run()                                 // serve until SIGTERM, then shut down
package app

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"github.com/git4jude/opspilot/services/internal/platform/chaos"
	"github.com/git4jude/opspilot/services/internal/platform/config"
	"github.com/git4jude/opspilot/services/internal/platform/deps"
	"github.com/git4jude/opspilot/services/internal/platform/health"
	"github.com/git4jude/opspilot/services/internal/platform/httpx"
	"github.com/git4jude/opspilot/services/internal/platform/logging"
	"github.com/git4jude/opspilot/services/internal/platform/metrics"
	"github.com/git4jude/opspilot/services/internal/platform/version"
)

// App holds the shared building blocks of a service.
type App struct {
	Config  config.Base
	Log     *slog.Logger
	Metrics *metrics.Registry
	Health  *health.Health
	Router  *httpx.Router
	Chaos   *chaos.Switches

	// Ctx is cancelled when SIGTERM or SIGINT is received. Background work
	// (consumers, tickers) should stop when it is done.
	Ctx context.Context

	stop      context.CancelFunc
	closers   []func()
	wrapRoute func(http.Handler) http.Handler
}

// New loads the base config and builds the logger, metrics, health and switches.
// failure is the error a service logs when a request fails due to FAIL_RATE.
//
// If the binary is invoked as "<binary> healthcheck" it probes its own
// /healthz endpoint and exits; container healthchecks use this because
// distroless images have no shell or curl.
func New(defaultName string, failure error) *App {
	cfg := config.LoadBase(defaultName)
	if len(os.Args) > 1 && os.Args[1] == "healthcheck" {
		os.Exit(health.Probe(cfg.Port))
	}

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	a := &App{
		Config:  cfg,
		Log:     logging.New(cfg.ServiceName, cfg.LogLevel),
		Metrics: metrics.New(cfg.ServiceName),
		Health:  health.New(),
		Router:  httpx.NewRouter(),
		Chaos:   chaos.New(chaos.Load(), failure),
		Ctx:     ctx,
		stop:    stop,
	}

	a.Router.HandleFunc("GET /healthz", a.Health.Liveness)
	a.Router.HandleFunc("GET /readyz", a.Health.Readiness)
	a.Router.Handle("GET /metrics", a.Metrics.Handler())
	return a
}

// Retry returns the startup retry policy from STARTUP_RETRIES / STARTUP_RETRY_DELAY.
func (a *App) Retry() deps.Retry {
	return deps.Retry{Attempts: a.Config.StartupRetries, Delay: a.Config.StartupRetryDelay}
}

// Fatal logs err and exits 1 if err is not nil. Use it for startup failures.
func (a *App) Fatal(err error) {
	if err == nil {
		return
	}
	a.Log.Error("startup failed", "error", err.Error())
	os.Exit(1)
}

// OnShutdown registers fn to run after the HTTP server has drained. Functions
// run in reverse registration order (like defer), so register connections
// first and the things that use them (e.g. consumers) after.
func (a *App) OnShutdown(fn func()) {
	a.closers = append(a.closers, fn)
}

// WrapRoutes adds a middleware between the standard chain and the router,
// e.g. CORS in the gateway.
func (a *App) WrapRoutes(mw func(http.Handler) http.Handler) {
	a.wrapRoute = mw
}

// Handler returns the router wrapped in the standard middleware chain:
// recover → request ID → access log → metrics → switches → (extra) → router.
func (a *App) Handler() http.Handler {
	var inner http.Handler = a.Router
	if a.wrapRoute != nil {
		inner = a.wrapRoute(inner)
	}
	return httpx.Chain(inner,
		httpx.Recover,
		httpx.RequestID,
		httpx.AccessLog,
		httpx.Metrics(a.Metrics),
		a.Chaos.Middleware,
	)
}

// Run logs the startup line, serves HTTP until a shutdown signal, drains,
// runs the OnShutdown functions and exits. extra holds the service's own
// non-secret settings for the startup log line.
func (a *App) Run(extra ...any) {
	attrs := append([]any{
		"port", a.Config.Port,
		"log_level", a.Config.LogLevel.String(),
		"shutdown_timeout", a.Config.ShutdownTimeout.String(),
		"commit", version.Commit,
	}, extra...)
	a.Log.Info("service starting", attrs...)

	srv := httpx.NewServer(a.Config.Addr(), a.Handler())
	err := httpx.Serve(a.Ctx, srv, a.Config.ShutdownTimeout, a.Health.SetShuttingDown)
	a.stop()

	for i := len(a.closers) - 1; i >= 0; i-- {
		a.closers[i]()
	}

	if err != nil {
		a.Log.Error("server stopped with error", "error", err.Error())
		os.Exit(1)
	}
	a.Log.Info("shutdown complete")
}
