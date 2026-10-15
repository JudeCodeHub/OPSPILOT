// Package deps connects to the infrastructure a service needs at startup
// (Postgres, Redis, NATS) with a limited number of retries.
//
// Retries are limited on purpose: if a dependency is still unreachable after
// STARTUP_RETRIES attempts the service logs the real error and exits 1. In
// Kubernetes a wrong DATABASE_URL then shows up as a CrashLoopBackOff with a
// clear "failed to connect to postgres: ..." line in the logs, instead of a
// pod that is Running but useless.
package deps

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/redis/go-redis/v9"
)

// Retry controls startup connection attempts.
type Retry struct {
	Attempts int
	Delay    time.Duration
}

// connect calls fn up to r.Attempts times, sleeping r.Delay between attempts.
func connect(ctx context.Context, name string, r Retry, fn func(ctx context.Context) error) error {
	var err error
	for attempt := 1; attempt <= r.Attempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(ctx, 5*time.Second)
		err = fn(attemptCtx)
		cancel()
		if err == nil {
			return nil
		}
		if attempt < r.Attempts {
			slog.Default().Warn("dependency not reachable, retrying",
				"dependency", name, "attempt", attempt, "max_attempts", r.Attempts,
				"retry_in", r.Delay.String(), "error", err.Error())
			select {
			case <-ctx.Done():
				return fmt.Errorf("failed to connect to %s: %w", name, ctx.Err())
			case <-time.After(r.Delay):
			}
		}
	}
	return fmt.Errorf("failed to connect to %s: %w", name, err)
}

// Postgres opens a pgx connection pool and verifies it with a ping.
// maxConns <= 0 keeps the pgx default.
func Postgres(ctx context.Context, url string, maxConns int32, r Retry) (*pgxpool.Pool, error) {
	cfg, err := pgxpool.ParseConfig(url)
	if err != nil {
		// pgx redacts the password in parse errors.
		return nil, fmt.Errorf("failed to connect to postgres: invalid DATABASE_URL: %w", err)
	}
	if maxConns > 0 {
		cfg.MaxConns = maxConns
	}
	pool, err := pgxpool.NewWithConfig(ctx, cfg)
	if err != nil {
		return nil, fmt.Errorf("failed to connect to postgres: %w", err)
	}
	if err := connect(ctx, "postgres", r, pool.Ping); err != nil {
		pool.Close()
		return nil, err
	}
	return pool, nil
}

// Redis creates a client and verifies it with PING.
func Redis(ctx context.Context, addr string, r Retry) (*redis.Client, error) {
	client := redis.NewClient(&redis.Options{
		Addr:         addr,
		DialTimeout:  2 * time.Second,
		ReadTimeout:  time.Second,
		WriteTimeout: time.Second,
	})
	err := connect(ctx, "redis", r, func(ctx context.Context) error {
		return client.Ping(ctx).Err()
	})
	if err != nil {
		_ = client.Close()
		return nil, err
	}
	return client, nil
}

// NATS connects to the NATS server. After the first successful connection the
// client reconnects forever on its own; disconnects are logged.
func NATS(ctx context.Context, url, clientName string, r Retry) (*nats.Conn, error) {
	var nc *nats.Conn
	err := connect(ctx, "nats", r, func(context.Context) error {
		var err error
		nc, err = nats.Connect(url,
			nats.Name(clientName),
			nats.Timeout(2*time.Second),
			nats.MaxReconnects(-1),
			nats.ReconnectWait(time.Second),
			nats.DisconnectErrHandler(func(_ *nats.Conn, err error) {
				if err != nil {
					slog.Default().Warn("disconnected from nats", "error", err.Error())
				}
			}),
			nats.ReconnectHandler(func(c *nats.Conn) {
				slog.Default().Info("reconnected to nats", "url", c.ConnectedUrlRedacted())
			}),
		)
		return err
	})
	if err != nil {
		return nil, err
	}
	return nc, nil
}

// NATSCheck is a readiness check that passes while the connection is up.
func NATSCheck(nc *nats.Conn) func(context.Context) error {
	return func(context.Context) error {
		if status := nc.Status(); status != nats.CONNECTED {
			return errors.New("nats connection is " + status.String())
		}
		return nil
	}
}
