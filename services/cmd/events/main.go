// Command events serves the concert catalogue and seat maps.
package main

import (
	"context"
	"errors"
	"time"

	"github.com/git4jude/opspilot/services/internal/events"
	"github.com/git4jude/opspilot/services/internal/platform/app"
	"github.com/git4jude/opspilot/services/internal/platform/config"
	"github.com/git4jude/opspilot/services/internal/platform/deps"
)

func main() {
	a := app.New("events", errors.New("query seats: conn closed: unexpected EOF"))

	var (
		databaseURL = config.Required("DATABASE_URL")
		redisAddr   = config.String("REDIS_ADDR", "redis:6379")
		slowQuery   = time.Duration(config.Int("SLOW_QUERY_MS", 0)) * time.Millisecond
	)

	pool, err := deps.Postgres(a.Ctx, databaseURL, 0, a.Retry())
	a.Fatal(err)
	a.OnShutdown(pool.Close)

	rdb, err := deps.Redis(a.Ctx, redisAddr, a.Retry())
	a.Fatal(err)
	a.OnShutdown(func() { _ = rdb.Close() })

	a.Health.Add("postgres", pool.Ping)
	a.Health.Add("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() })

	events.New(pool, rdb, slowQuery, a.Metrics).Register(a.Router)

	a.Chaos.Start(a.Ctx)
	a.Run(
		"db_host", pool.Config().ConnConfig.Host,
		"db_name", pool.Config().ConnConfig.Database,
		"redis_addr", redisAddr,
	)
}
