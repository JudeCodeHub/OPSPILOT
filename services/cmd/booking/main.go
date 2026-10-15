// Command booking manages seat holds and checkout.
package main

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/git4jude/opspilot/services/internal/booking"
	"github.com/git4jude/opspilot/services/internal/messages"
	"github.com/git4jude/opspilot/services/internal/platform/app"
	"github.com/git4jude/opspilot/services/internal/platform/config"
	"github.com/git4jude/opspilot/services/internal/platform/deps"
	"github.com/git4jude/opspilot/services/internal/platform/httpx"
)

func main() {
	a := app.New("booking", errors.New("store booking: driver: bad connection"))

	var (
		databaseURL    = config.Required("DATABASE_URL")
		redisAddr      = config.String("REDIS_ADDR", "redis:6379")
		natsURL        = config.String("NATS_URL", "nats://nats:4222")
		paymentURL     = config.String("PAYMENT_URL", "http://payment:8080")
		paymentTimeout = config.Duration("PAYMENT_TIMEOUT", 2*time.Second)
		holdTTL        = time.Duration(config.Int("HOLD_TTL_SECONDS", 300)) * time.Second
		dbPoolMax      = config.Int("DB_POOL_MAX", 10)
	)
	if holdTTL <= 0 {
		config.Fail("HOLD_TTL_SECONDS", errors.New("HOLD_TTL_SECONDS must be positive"))
	}
	if dbPoolMax < 1 {
		config.Fail("DB_POOL_MAX", errors.New("DB_POOL_MAX must be at least 1"))
	}

	pool, err := deps.Postgres(a.Ctx, databaseURL, int32(dbPoolMax), a.Retry())
	a.Fatal(err)
	a.OnShutdown(pool.Close)

	rdb, err := deps.Redis(a.Ctx, redisAddr, a.Retry())
	a.Fatal(err)
	a.OnShutdown(func() { _ = rdb.Close() })

	nc, err := deps.NATS(a.Ctx, natsURL, a.Config.ServiceName, a.Retry())
	a.Fatal(err)
	a.OnShutdown(nc.Close)
	js, err := jetstream.New(nc)
	a.Fatal(err)
	a.Fatal(messages.EnsureStream(a.Ctx, js))

	a.Health.Add("postgres", pool.Ping)
	a.Health.Add("redis", func(ctx context.Context) error { return rdb.Ping(ctx).Err() })
	a.Health.Add("nats", deps.NATSCheck(nc))

	m := booking.NewMetrics(a.Metrics)
	booking.RegisterPoolGauges(a.Metrics, pool)

	svc := &booking.Service{
		Store:     &booking.PGStore{Pool: pool, Metrics: m},
		Holds:     &booking.RedisHolds{Client: rdb},
		Payments:  &booking.PaymentClient{Client: httpx.NewClient("payment", paymentURL, paymentTimeout, a.Metrics)},
		Publisher: &booking.NATSPublisher{JS: js},
		Metrics:   m,
		HoldTTL:   holdTTL,
		Now:       time.Now,
		NewID:     uuid.NewString,
	}
	svc.Register(a.Router)

	a.Chaos.Start(a.Ctx)
	a.Run(
		"db_host", pool.Config().ConnConfig.Host,
		"db_name", pool.Config().ConnConfig.Database,
		"redis_addr", redisAddr,
		"nats_url", nc.ConnectedUrlRedacted(),
		"payment_url", paymentURL,
		"payment_timeout", paymentTimeout.String(),
		"hold_ttl", holdTTL.String(),
	)
}
