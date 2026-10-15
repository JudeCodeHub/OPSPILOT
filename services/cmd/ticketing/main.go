// Command ticketing issues tickets from booking.confirmed messages and serves them.
package main

import (
	"context"
	"errors"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/git4jude/opspilot/services/internal/messages"
	"github.com/git4jude/opspilot/services/internal/platform/app"
	"github.com/git4jude/opspilot/services/internal/platform/config"
	"github.com/git4jude/opspilot/services/internal/platform/deps"
	"github.com/git4jude/opspilot/services/internal/ticketing"
)

func main() {
	a := app.New("ticketing", errors.New("query tickets: conn busy"))

	var (
		databaseURL  = config.Required("DATABASE_URL")
		natsURL      = config.String("NATS_URL", "nats://nats:4222")
		processDelay = time.Duration(config.Int("PROCESS_DELAY_MS", 0)) * time.Millisecond
	)

	pool, err := deps.Postgres(a.Ctx, databaseURL, 0, a.Retry())
	a.Fatal(err)
	a.OnShutdown(pool.Close)

	nc, err := deps.NATS(a.Ctx, natsURL, a.Config.ServiceName, a.Retry())
	a.Fatal(err)
	a.OnShutdown(nc.Close)
	js, err := jetstream.New(nc)
	a.Fatal(err)
	a.Fatal(messages.EnsureStream(a.Ctx, js))
	consumer, err := messages.EnsureConsumer(a.Ctx, js, "ticketing", messages.SubjectBookingConfirmed)
	a.Fatal(err)

	a.Health.Add("postgres", pool.Ping)
	a.Health.Add("nats", deps.NATSCheck(nc))

	(&ticketing.API{DB: pool}).Register(a.Router)

	switches := a.Chaos.Config()
	processor := ticketing.NewProcessor(pool, js, processDelay, switches.LeakMemory, a.Metrics)
	go processor.WarmCache(a.Ctx, switches.LeakMBPerMin)
	a.Chaos.StartCPUBurn(a.Ctx)

	// Consume until shutdown. Registered after the connections so it is
	// stopped first (OnShutdown runs in reverse order).
	consumeCtx, stopConsuming := context.WithCancel(context.Background())
	done := make(chan struct{})
	go func() {
		defer close(done)
		// Batch of 1: messages are processed sequentially and each one's
		// AckWait only starts when it is fetched.
		messages.FetchLoop(consumeCtx, consumer, 1, processor.Handle)
	}()
	a.OnShutdown(func() {
		stopConsuming()
		<-done
	})

	a.Run(
		"db_host", pool.Config().ConnConfig.Host,
		"db_name", pool.Config().ConnConfig.Database,
		"nats_url", nc.ConnectedUrlRedacted(),
		"consumer", "ticketing",
	)
}
