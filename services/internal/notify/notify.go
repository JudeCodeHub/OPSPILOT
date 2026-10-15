// Package notify "sends" emails for booking.confirmed and ticket.issued by
// writing a structured log line, and keeps the last 100 in memory for the
// GET /notifications debug endpoint.
package notify

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/git4jude/opspilot/services/internal/messages"
	"github.com/git4jude/opspilot/services/internal/platform/httpx"
	"github.com/git4jude/opspilot/services/internal/platform/logging"
	"github.com/git4jude/opspilot/services/internal/platform/metrics"
)

// Templates.
const (
	TemplateBookingConfirmed = "booking_confirmed"
	TemplateTicketIssued     = "ticket_issued"
)

// historySize is how many notifications GET /notifications returns.
const historySize = 100

// Notification is one sent email.
type Notification struct {
	Template   string    `json:"template"`
	To         string    `json:"to"` // masked
	Subject    string    `json:"subject"`
	BookingID  string    `json:"bookingId"`
	TicketCode string    `json:"ticketCode,omitempty"`
	SentAt     time.Time `json:"sentAt"`
}

// Notifier consumes messages and records notifications.
type Notifier struct {
	mu      sync.Mutex
	history []Notification // newest last, at most historySize

	processed     *prometheus.CounterVec // template, result
	lastProcessed prometheus.Gauge
}

// New registers notify's metrics.
func New(m *metrics.Registry) *Notifier {
	n := &Notifier{
		processed: m.Factory.NewCounterVec(prometheus.CounterOpts{
			Name: "notify_messages_processed_total", Help: "Messages processed, by template and result.",
		}, []string{"template", "result"}),
		lastProcessed: m.Factory.NewGauge(prometheus.GaugeOpts{
			Name: "notify_last_processed_timestamp_seconds",
			Help: "Unix time of the last processed message. 0 if none since start.",
		}),
	}
	for _, t := range []string{TemplateBookingConfirmed, TemplateTicketIssued} {
		n.processed.WithLabelValues(t, "success")
		n.processed.WithLabelValues(t, "error")
	}
	return n
}

// Handle processes one message from the notify consumer.
func (n *Notifier) Handle(ctx context.Context, msg jetstream.Msg) {
	ctx = messages.Context(ctx, msg)
	log := logging.FromContext(ctx)

	note, err := build(msg.Subject(), msg.Data())
	if err != nil {
		n.processed.WithLabelValues(note.Template, "error").Inc()
		log.Error("discarding malformed message", "subject", msg.Subject(), "error", err.Error())
		_ = msg.Term()
		return
	}

	note.SentAt = time.Now().UTC()
	fields := []any{"template", note.Template, "to", note.To, "booking_id", note.BookingID}
	if note.TicketCode != "" {
		fields = append(fields, "ticket_code", note.TicketCode)
	}
	log.Info("email sent", fields...)

	n.mu.Lock()
	n.history = append(n.history, note)
	if len(n.history) > historySize {
		n.history = n.history[len(n.history)-historySize:]
	}
	n.mu.Unlock()

	n.processed.WithLabelValues(note.Template, "success").Inc()
	n.lastProcessed.Set(float64(note.SentAt.Unix()))
	_ = msg.Ack()
}

// build turns a message into the notification to send.
func build(subject string, data []byte) (Notification, error) {
	switch subject {
	case messages.SubjectBookingConfirmed:
		var evt messages.BookingConfirmed
		if err := json.Unmarshal(data, &evt); err != nil || evt.Email == "" {
			return Notification{Template: TemplateBookingConfirmed}, fmt.Errorf("decode %s: invalid payload", subject)
		}
		return Notification{
			Template:  TemplateBookingConfirmed,
			To:        logging.MaskEmail(evt.Email),
			Subject:   fmt.Sprintf("Booking confirmed: seat %s", evt.SeatID),
			BookingID: evt.BookingID,
		}, nil
	case messages.SubjectTicketIssued:
		var evt messages.TicketIssued
		if err := json.Unmarshal(data, &evt); err != nil || evt.Email == "" {
			return Notification{Template: TemplateTicketIssued}, fmt.Errorf("decode %s: invalid payload", subject)
		}
		return Notification{
			Template:   TemplateTicketIssued,
			To:         logging.MaskEmail(evt.Email),
			Subject:    fmt.Sprintf("Your ticket %s is ready", evt.TicketCode),
			BookingID:  evt.BookingID,
			TicketCode: evt.TicketCode,
		}, nil
	default:
		return Notification{Template: "unknown"}, fmt.Errorf("unexpected subject %q", subject)
	}
}

// Register adds the debug endpoint.
func (n *Notifier) Register(rt *httpx.Router) {
	rt.HandleFunc("GET /notifications", n.handleList)
}

func (n *Notifier) handleList(w http.ResponseWriter, _ *http.Request) {
	n.mu.Lock()
	out := make([]Notification, len(n.history))
	for i, note := range n.history {
		out[len(out)-1-i] = note // newest first
	}
	n.mu.Unlock()
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"notifications": out})
}
