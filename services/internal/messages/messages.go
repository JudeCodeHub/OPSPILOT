// Package messages defines the NATS JetStream contract between services:
// the stream, the subjects, the payloads, and helpers to publish and consume
// with X-Request-ID propagation.
package messages

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/git4jude/opspilot/services/internal/platform/logging"
)

// Stream and subjects.
const (
	StreamName              = "TICKETING"
	SubjectBookingConfirmed = "booking.confirmed"
	SubjectTicketIssued     = "ticket.issued"

	// HeaderRequestID carries the originating HTTP request ID through NATS.
	HeaderRequestID = "X-Request-ID"
)

// BookingConfirmed is published by booking after a successful checkout.
type BookingConfirmed struct {
	BookingID   string    `json:"bookingId"`
	EventID     string    `json:"eventId"`
	SeatID      string    `json:"seatId"`
	Email       string    `json:"email"`
	AmountCents int       `json:"amountCents"`
	Currency    string    `json:"currency"`
	ConfirmedAt time.Time `json:"confirmedAt"`
}

// TicketIssued is published by ticketing once the ticket is stored.
type TicketIssued struct {
	TicketCode string    `json:"ticketCode"`
	BookingID  string    `json:"bookingId"`
	EventID    string    `json:"eventId"`
	SeatID     string    `json:"seatId"`
	Email      string    `json:"email"`
	IssuedAt   time.Time `json:"issuedAt"`
}

// EnsureStream creates the TICKETING stream or updates it to the expected
// configuration. Every publisher and consumer calls it at startup, so start
// order does not matter.
func EnsureStream(ctx context.Context, js jetstream.JetStream) error {
	_, err := js.CreateOrUpdateStream(ctx, jetstream.StreamConfig{
		Name:     StreamName,
		Subjects: []string{"booking.>", "ticket.>"},
		Storage:  jetstream.FileStorage,
		MaxAge:   24 * time.Hour,
	})
	if err != nil {
		return fmt.Errorf("ensure stream %s: %w", StreamName, err)
	}
	return nil
}

// EnsureConsumer creates or updates a durable pull consumer with explicit
// acks, at most 5 deliveries and a 30s ack wait.
func EnsureConsumer(ctx context.Context, js jetstream.JetStream, durable string, subjects ...string) (jetstream.Consumer, error) {
	cons, err := js.CreateOrUpdateConsumer(ctx, StreamName, jetstream.ConsumerConfig{
		Durable:        durable,
		AckPolicy:      jetstream.AckExplicitPolicy,
		MaxDeliver:     5,
		AckWait:        30 * time.Second,
		FilterSubjects: subjects,
	})
	if err != nil {
		return nil, fmt.Errorf("ensure consumer %s: %w", durable, err)
	}
	return cons, nil
}

// Publish marshals v and publishes it on subject. msgID enables JetStream
// de-duplication (the same business event published twice is stored once).
// The request ID from ctx travels in the X-Request-ID header.
func Publish(ctx context.Context, js jetstream.JetStream, subject, msgID string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return fmt.Errorf("encode %s: %w", subject, err)
	}
	msg := &nats.Msg{Subject: subject, Data: data, Header: nats.Header{}}
	if id := logging.RequestID(ctx); id != "" {
		msg.Header.Set(HeaderRequestID, id)
	}
	ctx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if _, err := js.PublishMsg(ctx, msg, jetstream.WithMsgID(msgID)); err != nil {
		return fmt.Errorf("publish %s: %w", subject, err)
	}
	return nil
}

// Context returns ctx carrying the request ID from the message headers (a new
// one if the publisher did not set it), so consumer logs can be correlated.
func Context(ctx context.Context, msg jetstream.Msg) context.Context {
	id := ""
	if h := msg.Headers(); h != nil {
		id = h.Get(HeaderRequestID)
	}
	if id == "" {
		id = uuid.NewString()
	}
	return logging.WithRequestID(ctx, id)
}

// Handler processes one message. It must Ack, Nak or Term the message.
type Handler func(ctx context.Context, msg jetstream.Msg)

// FetchLoop pulls up to batch messages at a time and hands them to handle,
// one after another, until ctx is cancelled. Messages are processed
// sequentially, so a slow handler makes the consumer's backlog grow.
//
// Small batches matter: a fetched message's AckWait clock starts at delivery,
// so fetching more than can be processed within AckWait causes redeliveries.
func FetchLoop(ctx context.Context, cons jetstream.Consumer, batch int, handle Handler) {
	logger := logging.FromContext(ctx)
	for ctx.Err() == nil {
		msgs, err := cons.Fetch(batch, jetstream.FetchMaxWait(2*time.Second))
		if err != nil {
			logger.Error("failed to fetch messages", "error", err.Error())
			sleep(ctx, time.Second)
			continue
		}
		for msg := range msgs.Messages() {
			handle(ctx, msg)
		}
		if err := msgs.Error(); err != nil && !errors.Is(err, nats.ErrTimeout) && !errors.Is(err, context.DeadlineExceeded) {
			logger.Error("failed to fetch messages", "error", err.Error())
			sleep(ctx, time.Second)
		}
	}
}

func sleep(ctx context.Context, d time.Duration) {
	select {
	case <-ctx.Done():
	case <-time.After(d):
	}
}

// Backoff returns the redelivery delay after the given number of deliveries:
// 1s, 2s, 4s, 8s, capped at 30s.
func Backoff(numDelivered uint64) time.Duration {
	if numDelivered < 1 {
		numDelivered = 1
	}
	d := time.Second << min(numDelivered-1, 5)
	return min(d, 30*time.Second)
}
