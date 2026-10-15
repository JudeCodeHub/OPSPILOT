package ticketing

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/git4jude/opspilot/services/internal/messages"
	"github.com/git4jude/opspilot/services/internal/platform/chaos"
	"github.com/git4jude/opspilot/services/internal/platform/logging"
	"github.com/git4jude/opspilot/services/internal/platform/metrics"
)

// Processor turns booking.confirmed messages into tickets.
type Processor struct {
	DB *pgxpool.Pool
	JS jetstream.JetStream

	// ProcessDelay is spent on every message before it is handled.
	ProcessDelay time.Duration

	// cacheRenders keeps a rendered copy of every issued ticket in
	// renderCache. Nothing ever evicts it.
	cacheRenders bool
	renderCache  renderCache

	processed *prometheus.CounterVec
	duration  prometheus.Histogram
	issued    prometheus.Counter
}

// renderCache holds rendered ticket payloads.
type renderCache struct {
	mu      sync.Mutex
	entries [][]byte
}

func (c *renderCache) add(b []byte) {
	c.mu.Lock()
	c.entries = append(c.entries, b)
	c.mu.Unlock()
}

// renderSize is the size of one cached ticket render.
const renderSize = 512 << 10

// NewProcessor registers the processing metrics.
func NewProcessor(db *pgxpool.Pool, js jetstream.JetStream, delay time.Duration, cacheRenders bool, m *metrics.Registry) *Processor {
	p := &Processor{
		DB:           db,
		JS:           js,
		ProcessDelay: delay,
		cacheRenders: cacheRenders,
		processed: m.Factory.NewCounterVec(prometheus.CounterOpts{
			Name: "ticketing_messages_processed_total", Help: "booking.confirmed messages processed, by result.",
		}, []string{"result"}),
		duration: m.Factory.NewHistogram(prometheus.HistogramOpts{
			Name: "ticketing_processing_duration_seconds", Help: "Time to process one message.",
			Buckets: metrics.DurationBuckets,
		}),
		issued: m.Factory.NewCounter(prometheus.CounterOpts{
			Name: "ticketing_tickets_issued_total", Help: "New tickets issued.",
		}),
	}
	p.processed.WithLabelValues("success")
	p.processed.WithLabelValues("error")
	return p
}

// WarmCache keeps refreshing the render cache in the background at
// mbPerMin until ctx ends. It is a no-op unless render caching is enabled.
func (p *Processor) WarmCache(ctx context.Context, mbPerMin int) {
	if !p.cacheRenders {
		return
	}
	chaos.RetainAtRate(ctx, mbPerMin, p.renderCache.add)
}

// Handle processes one message: store the ticket (idempotent on booking_id),
// publish ticket.issued, then ack. On failure the message is redelivered
// with backoff, up to the consumer's MaxDeliver.
func (p *Processor) Handle(ctx context.Context, msg jetstream.Msg) {
	ctx = messages.Context(ctx, msg)
	log := logging.FromContext(ctx)
	start := time.Now()

	if !chaos.Sleep(ctx, p.ProcessDelay) {
		_ = msg.Nak() // shutting down: let another instance (or our restart) take it
		return
	}

	err := p.process(ctx, msg)
	p.duration.Observe(time.Since(start).Seconds())

	var poison *poisonError
	switch {
	case err == nil:
		p.processed.WithLabelValues("success").Inc()
		_ = msg.Ack()
	case errors.As(err, &poison):
		p.processed.WithLabelValues("error").Inc()
		log.Error("discarding malformed message", "subject", msg.Subject(), "error", err.Error())
		_ = msg.Term()
	default:
		p.processed.WithLabelValues("error").Inc()
		delivered := uint64(1)
		if md, mdErr := msg.Metadata(); mdErr == nil {
			delivered = md.NumDelivered
		}
		backoff := messages.Backoff(delivered)
		log.Error("failed to issue ticket, will retry",
			"subject", msg.Subject(), "delivery", delivered, "retry_in", backoff.String(), "error", err.Error())
		_ = msg.NakWithDelay(backoff)
	}
}

type poisonError struct{ err error }

func (e *poisonError) Error() string { return e.err.Error() }

func (p *Processor) process(ctx context.Context, msg jetstream.Msg) error {
	var evt messages.BookingConfirmed
	if err := json.Unmarshal(msg.Data(), &evt); err != nil {
		return &poisonError{fmt.Errorf("decode booking.confirmed: %w", err)}
	}
	if evt.BookingID == "" || evt.EventID == "" || evt.SeatID == "" {
		return &poisonError{errors.New("booking.confirmed is missing required fields")}
	}

	code, issuedAt, created, err := p.storeTicket(ctx, evt)
	if err != nil {
		return err
	}

	if p.cacheRenders {
		p.renderCache.add(chaos.Allocate(renderSize))
	}

	issued := messages.TicketIssued{
		TicketCode: code,
		BookingID:  evt.BookingID,
		EventID:    evt.EventID,
		SeatID:     evt.SeatID,
		Email:      evt.Email,
		IssuedAt:   issuedAt,
	}
	if err := messages.Publish(ctx, p.JS, messages.SubjectTicketIssued, "ticket-issued-"+evt.BookingID, issued); err != nil {
		return err
	}

	if created {
		p.issued.Inc()
		logging.FromContext(ctx).Info("ticket issued",
			"ticket_code", code, "booking_id", evt.BookingID, "event_id", evt.EventID, "seat_id", evt.SeatID)
	} else {
		logging.FromContext(ctx).Info("ticket already issued for booking", "ticket_code", code, "booking_id", evt.BookingID)
	}
	return nil
}

// storeTicket inserts a ticket for the booking, or returns the existing one if
// the message is a redelivery. created reports whether a new row was inserted.
func (p *Processor) storeTicket(ctx context.Context, evt messages.BookingConfirmed) (code string, issuedAt time.Time, created bool, err error) {
	for range 3 {
		code = NewCode()
		err = p.DB.QueryRow(ctx,
			`INSERT INTO tickets (code, booking_id, event_id, seat_id, email)
			 VALUES ($1, $2, $3, $4, $5)
			 ON CONFLICT (booking_id) DO NOTHING
			 RETURNING issued_at`,
			code, evt.BookingID, evt.EventID, evt.SeatID, evt.Email).Scan(&issuedAt)
		if err == nil {
			return code, issuedAt, true, nil
		}
		if errors.Is(err, pgx.ErrNoRows) {
			// A ticket already exists for this booking: return it.
			err = p.DB.QueryRow(ctx, `SELECT code, issued_at FROM tickets WHERE booking_id = $1`, evt.BookingID).
				Scan(&code, &issuedAt)
			if err != nil {
				return "", time.Time{}, false, fmt.Errorf("load existing ticket: %w", err)
			}
			return code, issuedAt, false, nil
		}
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "tickets_pkey" {
			continue // code collision: try another code
		}
		return "", time.Time{}, false, fmt.Errorf("insert ticket: %w", err)
	}
	return "", time.Time{}, false, fmt.Errorf("insert ticket: %w", err)
}
