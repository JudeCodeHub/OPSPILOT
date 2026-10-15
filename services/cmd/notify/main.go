// Command notify "sends" emails for confirmed bookings and issued tickets.
package main

import (
	"context"
	"errors"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/git4jude/opspilot/services/internal/messages"
	"github.com/git4jude/opspilot/services/internal/notify"
	"github.com/git4jude/opspilot/services/internal/platform/app"
	"github.com/git4jude/opspilot/services/internal/platform/config"
	"github.com/git4jude/opspilot/services/internal/platform/deps"
)

func main() {
	a := app.New("notify", errors.New("render template: context deadline exceeded"))

	var (
		natsURL = config.String("NATS_URL", "nats://nats:4222")
		paused  = config.Bool("PAUSE_CONSUMER", false)
	)

	nc, err := deps.NATS(a.Ctx, natsURL, a.Config.ServiceName, a.Retry())
	a.Fatal(err)
	a.OnShutdown(nc.Close)
	js, err := jetstream.New(nc)
	a.Fatal(err)
	a.Fatal(messages.EnsureStream(a.Ctx, js))
	// The durable consumer always exists, so messages accumulate for it even
	// when nothing is fetching them.
	consumer, err := messages.EnsureConsumer(a.Ctx, js, "notify",
		messages.SubjectBookingConfirmed, messages.SubjectTicketIssued)
	a.Fatal(err)

	a.Health.Add("nats", deps.NATSCheck(nc))

	notifier := notify.New(a.Metrics)
	notifier.Register(a.Router)

	if !paused {
		consumeCtx, stopConsuming := context.WithCancel(context.Background())
		done := make(chan struct{})
		go func() {
			defer close(done)
			messages.FetchLoop(consumeCtx, consumer, 10, notifier.Handle)
		}()
		a.OnShutdown(func() {
			stopConsuming()
			<-done
		})
	}

	a.Chaos.Start(a.Ctx)
	a.Run("nats_url", nc.ConnectedUrlRedacted(), "consumer", "notify")
}
