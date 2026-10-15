// Package payment is a fake card processor with idempotent charges.
//
//   - Card 4000000000000002 is declined (402 CARD_DECLINED). A decline is a
//     normal business outcome, not an error.
//   - Any other well-formed card is approved (201).
//   - Repeating a request with the same Idempotency-Key returns the original
//     charge instead of charging again. Approved charges are kept in memory
//     for 1h. Declines are not remembered: nothing was charged, so the
//     customer can retry the same order with a different card.
//   - Full card numbers are never logged; only the last four digits.
package payment

import (
	"context"
	"math/rand/v2"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/prometheus/client_golang/prometheus"

	"github.com/git4jude/opspilot/services/internal/platform/httpx"
	"github.com/git4jude/opspilot/services/internal/platform/logging"
	"github.com/git4jude/opspilot/services/internal/platform/metrics"
)

// DeclinedCard is the test card that is always declined.
const DeclinedCard = "4000000000000002"

// IdempotencyTTL is how long a charge result is remembered.
const IdempotencyTTL = time.Hour

// Error codes.
const (
	CodeCardDeclined       = "CARD_DECLINED"
	CodeIdempotencyMissing = "IDEMPOTENCY_KEY_REQUIRED"
)

// ChargeRequest is the body of POST /charges.
type ChargeRequest struct {
	AmountCents int    `json:"amountCents"`
	Currency    string `json:"currency"`
	CardNumber  string `json:"cardNumber"`
	Reference   string `json:"reference"`
}

// Charge is an approved charge.
type Charge struct {
	ChargeID    string    `json:"chargeId"`
	Status      string    `json:"status"`
	AmountCents int       `json:"amountCents"`
	Currency    string    `json:"currency"`
	CardLast4   string    `json:"cardLast4"`
	Reference   string    `json:"reference"`
	CreatedAt   time.Time `json:"createdAt"`
}

// Outcome is the result of one charge attempt: an approved Charge or a decline.
type Outcome struct {
	Status int     // 201 or 402
	Charge *Charge // set when approved
}

type entry struct {
	outcome Outcome
	expires time.Time
}

// Processor decides charges and remembers results by idempotency key.
type Processor struct {
	mu      sync.Mutex
	results map[string]entry

	Now   func() time.Time
	NewID func() string
}

// NewProcessor returns an empty processor.
func NewProcessor() *Processor {
	return &Processor{
		results: make(map[string]entry),
		Now:     time.Now,
		NewID:   func() string { return "ch_" + strings.ReplaceAll(uuid.NewString(), "-", "")[:24] },
	}
}

// Validate checks a charge request and returns the normalised card number.
func Validate(req ChargeRequest) (string, error) {
	card, ok := normalizeCard(req.CardNumber)
	if !ok {
		return "", httpx.NewError(http.StatusBadRequest, httpx.CodeBadRequest, "cardNumber must be 12 to 19 digits.")
	}
	if req.AmountCents <= 0 {
		return "", httpx.NewError(http.StatusBadRequest, httpx.CodeBadRequest, "amountCents must be positive.")
	}
	if len(req.Currency) != 3 {
		return "", httpx.NewError(http.StatusBadRequest, httpx.CodeBadRequest, "currency must be a 3-letter code.")
	}
	return card, nil
}

// Charge processes req under key. replayed is true if the result comes from
// an earlier request with the same key. The request must be valid.
func (p *Processor) Charge(key string, req ChargeRequest, card string) (out Outcome, replayed bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	now := p.Now()
	if e, ok := p.results[key]; ok && now.Before(e.expires) {
		return e.outcome, true
	}

	if card == DeclinedCard {
		return Outcome{Status: http.StatusPaymentRequired}, false
	}
	out = Outcome{Status: http.StatusCreated, Charge: &Charge{
		ChargeID:    p.NewID(),
		Status:      "approved",
		AmountCents: req.AmountCents,
		Currency:    strings.ToUpper(req.Currency),
		CardLast4:   card[len(card)-4:],
		Reference:   req.Reference,
		CreatedAt:   now.UTC(),
	}}
	p.results[key] = entry{outcome: out, expires: now.Add(IdempotencyTTL)}
	return out, false
}

// Sweep removes expired idempotency entries.
func (p *Processor) Sweep() {
	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.Now()
	for k, e := range p.results {
		if !now.Before(e.expires) {
			delete(p.results, k)
		}
	}
}

// RunSweeper calls Sweep every minute until ctx ends.
func (p *Processor) RunSweeper(ctx context.Context) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			p.Sweep()
		}
	}
}

func normalizeCard(raw string) (string, bool) {
	var b strings.Builder
	for _, r := range raw {
		switch {
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == ' ' || r == '-':
		default:
			return "", false
		}
	}
	n := b.String()
	return n, len(n) >= 12 && len(n) <= 19
}

// Handler exposes the processor over HTTP.
type Handler struct {
	Processor *Processor
	Charges   *prometheus.CounterVec // result: approved|declined|error
	Duration  prometheus.Histogram

	// ProcessingTime is how long the card network takes to answer. Real
	// processors take tens to hundreds of milliseconds; nil means instant.
	ProcessingTime func() time.Duration
}

// RealisticProcessingTime returns a card-network delay between 40 and 120ms.
func RealisticProcessingTime() time.Duration {
	return 40*time.Millisecond + rand.N(80*time.Millisecond)
}

// NewHandler registers payment's metrics.
func NewHandler(p *Processor, m *metrics.Registry) *Handler {
	h := &Handler{
		Processor:      p,
		ProcessingTime: RealisticProcessingTime,
		Charges: m.Factory.NewCounterVec(prometheus.CounterOpts{
			Name: "payment_charges_total", Help: "Charge attempts by result.",
		}, []string{"result"}),
		Duration: m.Factory.NewHistogram(prometheus.HistogramOpts{
			Name:    "payment_charge_duration_seconds",
			Help:    "Time to process a charge.",
			Buckets: metrics.DurationBuckets,
		}),
	}
	for _, r := range []string{"approved", "declined", "error"} {
		h.Charges.WithLabelValues(r)
	}
	return h
}

// Register adds the routes.
func (h *Handler) Register(rt *httpx.Router) {
	rt.HandleFunc("POST /charges", h.handleCharge)
}

// RecordError counts a charge that failed with an internal error. It is used
// as the failure hook of the request middleware.
func (h *Handler) RecordError(r *http.Request) {
	if r.Method == http.MethodPost && r.URL.Path == "/charges" {
		h.Charges.WithLabelValues("error").Inc()
		h.Duration.Observe(time.Since(httpx.StartTime(r)).Seconds())
	}
}

func (h *Handler) handleCharge(w http.ResponseWriter, r *http.Request) {
	log := logging.FromContext(r.Context())

	key := strings.TrimSpace(r.Header.Get("Idempotency-Key"))
	if key == "" || len(key) > 200 {
		httpx.WriteError(w, r, http.StatusBadRequest, CodeIdempotencyMissing, "An Idempotency-Key header is required.")
		return
	}
	var req ChargeRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteErr(w, r, err)
		return
	}
	card, err := Validate(req)
	if err != nil {
		httpx.WriteErr(w, r, err)
		return
	}

	if h.ProcessingTime != nil {
		select {
		case <-time.After(h.ProcessingTime()):
		case <-r.Context().Done():
			return // the caller gave up
		}
	}

	out, replayed := h.Processor.Charge(key, req, card)
	fields := []any{
		"reference", req.Reference, "amount_cents", req.AmountCents,
		"currency", req.Currency, "card_last4", logging.Last4(card),
	}
	if replayed {
		log.Info("idempotent replay of earlier charge", append(fields, "status", out.Status)...)
	} else {
		h.Duration.Observe(time.Since(httpx.StartTime(r)).Seconds())
	}

	if out.Status == http.StatusPaymentRequired {
		if !replayed {
			h.Charges.WithLabelValues("declined").Inc()
			log.Info("charge declined", append(fields, "decline_reason", "card_declined")...)
		}
		httpx.WriteError(w, r, http.StatusPaymentRequired, CodeCardDeclined, "The card was declined by the issuer.")
		return
	}
	if !replayed {
		h.Charges.WithLabelValues("approved").Inc()
		log.Info("charge approved", append(fields, "charge_id", out.Charge.ChargeID)...)
	}
	httpx.WriteJSON(w, http.StatusCreated, out.Charge)
}
