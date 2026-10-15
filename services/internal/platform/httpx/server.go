package httpx

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// NewServer builds an http.Server with timeouts suited to a JSON API.
// Without them a slow or malicious client can hold connections open forever.
func NewServer(addr string, h http.Handler) *http.Server {
	return &http.Server{
		Addr:              addr,
		Handler:           h,
		ReadHeaderTimeout: 5 * time.Second,  // slowloris protection
		ReadTimeout:       15 * time.Second, // whole request, including body
		WriteTimeout:      30 * time.Second, // must exceed the slowest legitimate response
		IdleTimeout:       60 * time.Second, // keep-alive connections
	}
}

// Serve runs srv until ctx is cancelled (normally by SIGTERM/SIGINT), then
// shuts down gracefully:
//
//  1. markNotReady() — /readyz starts answering 503
//  2. stop accepting new connections
//  3. wait for in-flight requests to finish, up to shutdownTimeout
//
// Closing databases and brokers is the caller's job, after Serve returns, so
// that in-flight requests can still use them while draining.
func Serve(ctx context.Context, srv *http.Server, shutdownTimeout time.Duration, markNotReady func()) error {
	errCh := make(chan error, 1)
	go func() {
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
		}
		close(errCh)
	}()

	select {
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("http server: %w", err)
		}
		return nil
	case <-ctx.Done():
	}

	slog.Default().Info("shutdown signal received, draining in-flight requests", "timeout", shutdownTimeout.String())
	markNotReady()

	shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("graceful shutdown: %w", err)
	}
	return nil
}
