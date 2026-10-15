// Package booking implements seat holds and checkout.
//
// Hold: a seat is reserved for a few minutes with SET NX in Redis, so two
// users cannot hold the same seat. Checkout: charge the card, then record the
// booking and mark the seat sold in one transaction. The UNIQUE(event_id,
// seat_id) constraint on bookings is the final guard against double-selling.
package booking

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/git4jude/opspilot/services/internal/holds"
	"github.com/git4jude/opspilot/services/internal/messages"
	"github.com/git4jude/opspilot/services/internal/platform/httpx"
	"github.com/git4jude/opspilot/services/internal/platform/logging"
)

// Error codes returned by booking.
const (
	CodeSeatUnavailable    = "SEAT_UNAVAILABLE"
	CodeSeatNotFound       = "SEAT_NOT_FOUND"
	CodeHoldExpired        = "HOLD_EXPIRED"
	CodeBookingNotFound    = "BOOKING_NOT_FOUND"
	CodePaymentDeclined    = "PAYMENT_DECLINED"
	CodePaymentTimeout     = "PAYMENT_TIMEOUT"
	CodePaymentUnavailable = "PAYMENT_UNAVAILABLE"
	CodeInvalidCard        = "INVALID_CARD"
)

// Sentinel errors returned by the dependencies.
var (
	ErrNotFound           = errors.New("not found")
	ErrConflict           = errors.New("conflict")
	ErrDeclined           = errors.New("payment declined")
	ErrPaymentTimeout     = errors.New("payment timed out")
	ErrPaymentUnavailable = errors.New("payment service unavailable")
	ErrPaymentRejected    = errors.New("payment request rejected")
)

// Seat is the subset of a seat booking needs.
type Seat struct {
	EventID    string
	SeatID     string
	PriceCents int
	Currency   string
	Status     string // available | sold
}

// Booking is a confirmed purchase.
type Booking struct {
	ID          string    `json:"id"`
	EventID     string    `json:"eventId"`
	SeatID      string    `json:"seatId"`
	Email       string    `json:"email"`
	AmountCents int       `json:"amountCents"`
	Currency    string    `json:"currency"`
	ChargeID    string    `json:"chargeId"`
	Status      string    `json:"status"`
	CreatedAt   time.Time `json:"createdAt"`
}

// ChargeRequest is sent to the payment service.
type ChargeRequest struct {
	AmountCents int
	Currency    string
	CardNumber  string
	Reference   string // the hold ID, also used as idempotency key
}

// Store is the Postgres side.
type Store interface {
	// Seat returns ErrNotFound for an unknown seat.
	Seat(ctx context.Context, eventID, seatID string) (Seat, error)
	// ConfirmBooking locks the seat row (SELECT ... FOR UPDATE) in a
	// transaction and calls pay with it. If pay succeeds, the booking it
	// returns is inserted and the seat marked sold before commit; if pay fails
	// nothing is written and its error is returned. ErrConflict means the seat
	// is already sold (pay is not called), ErrNotFound that it doesn't exist.
	ConfirmBooking(ctx context.Context, eventID, seatID string, pay func(Seat) (Booking, error)) (Booking, error)
	// Booking returns ErrNotFound for an unknown ID.
	Booking(ctx context.Context, id string) (Booking, error)
}

// HoldStore is the Redis side.
type HoldStore interface {
	// Create reserves the seat; it returns false if someone else holds it.
	Create(ctx context.Context, h holds.Hold, ttl time.Duration) (bool, error)
	// Get returns ErrNotFound if the hold does not exist or has expired.
	Get(ctx context.Context, holdID string) (holds.Hold, error)
	// Delete removes the hold (no error if it is already gone).
	Delete(ctx context.Context, h holds.Hold) error
}

// Payments charges cards. It returns the charge ID, or ErrDeclined,
// ErrPaymentTimeout, ErrPaymentUnavailable or ErrPaymentRejected.
type Payments interface {
	Charge(ctx context.Context, req ChargeRequest) (string, error)
}

// Publisher announces confirmed bookings.
type Publisher interface {
	BookingConfirmed(ctx context.Context, evt messages.BookingConfirmed) error
}

// Service holds the booking logic.
type Service struct {
	Store     Store
	Holds     HoldStore
	Payments  Payments
	Publisher Publisher
	Metrics   *Metrics
	HoldTTL   time.Duration

	Now   func() time.Time // replaceable in tests
	NewID func() string
}

// HoldRequest is the body of POST /holds.
type HoldRequest struct {
	EventID string `json:"eventId"`
	SeatID  string `json:"seatId"`
	Email   string `json:"email"`
}

// HoldResponse is returned by POST /holds.
type HoldResponse struct {
	HoldID     string    `json:"holdId"`
	EventID    string    `json:"eventId"`
	SeatID     string    `json:"seatId"`
	ExpiresAt  time.Time `json:"expiresAt"`
	PriceCents int       `json:"priceCents"`
	Currency   string    `json:"currency"`
}

// CheckoutRequest is the body of POST /holds/{holdId}/checkout.
type CheckoutRequest struct {
	CardNumber string `json:"cardNumber"`
	CardName   string `json:"cardName"`
}

var seatIDPattern = regexp.MustCompile(`^[A-Z][0-9]{1,3}$`)

func badRequest(msg string) error {
	return httpx.NewError(http.StatusBadRequest, httpx.CodeBadRequest, msg)
}

func seatUnavailable(seatID string) error {
	return httpx.NewError(http.StatusConflict, CodeSeatUnavailable, fmt.Sprintf("Seat %s is no longer available.", seatID))
}

// CreateHold validates the request, checks the seat in Postgres and reserves it in Redis.
func (s *Service) CreateHold(ctx context.Context, req HoldRequest) (HoldResponse, error) {
	req.SeatID = strings.ToUpper(strings.TrimSpace(req.SeatID))
	req.Email = strings.ToLower(strings.TrimSpace(req.Email))
	if _, err := uuid.Parse(req.EventID); err != nil {
		return HoldResponse{}, badRequest("eventId must be a valid event ID.")
	}
	if !seatIDPattern.MatchString(req.SeatID) {
		return HoldResponse{}, badRequest("seatId must look like A12.")
	}
	if addr, err := mail.ParseAddress(req.Email); err != nil || addr.Address != req.Email || !strings.Contains(req.Email, ".") {
		return HoldResponse{}, badRequest("Please enter a valid email address.")
	}

	seat, err := s.Store.Seat(ctx, req.EventID, req.SeatID)
	if errors.Is(err, ErrNotFound) {
		return HoldResponse{}, httpx.NewError(http.StatusNotFound, CodeSeatNotFound, fmt.Sprintf("Seat %s does not exist for this event.", req.SeatID))
	}
	if err != nil {
		return HoldResponse{}, fmt.Errorf("load seat: %w", err)
	}
	if seat.Status == "sold" {
		s.Metrics.HoldConflicts.Inc()
		return HoldResponse{}, seatUnavailable(req.SeatID)
	}

	hold := holds.Hold{
		HoldID:    s.NewID(),
		EventID:   req.EventID,
		SeatID:    req.SeatID,
		Email:     req.Email,
		ExpiresAt: s.Now().Add(s.HoldTTL).UTC(),
	}
	ok, err := s.Holds.Create(ctx, hold, s.HoldTTL)
	if err != nil {
		return HoldResponse{}, fmt.Errorf("create seat hold: %w", err)
	}
	if !ok {
		s.Metrics.HoldConflicts.Inc()
		return HoldResponse{}, seatUnavailable(req.SeatID)
	}

	s.Metrics.HoldsCreated.Inc()
	logging.FromContext(ctx).Info("seat held",
		"hold_id", hold.HoldID, "event_id", hold.EventID, "seat_id", hold.SeatID,
		"email", logging.MaskEmail(hold.Email), "expires_at", hold.ExpiresAt)
	return HoldResponse{
		HoldID:     hold.HoldID,
		EventID:    hold.EventID,
		SeatID:     hold.SeatID,
		ExpiresAt:  hold.ExpiresAt,
		PriceCents: seat.PriceCents,
		Currency:   seat.Currency,
	}, nil
}

// ReleaseHold deletes a hold. Releasing a hold that no longer exists succeeds.
func (s *Service) ReleaseHold(ctx context.Context, holdID string) error {
	hold, err := s.Holds.Get(ctx, holdID)
	if errors.Is(err, ErrNotFound) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("load hold: %w", err)
	}
	if err := s.Holds.Delete(ctx, hold); err != nil {
		return fmt.Errorf("release hold: %w", err)
	}
	logging.FromContext(ctx).Info("hold released", "hold_id", holdID, "event_id", hold.EventID, "seat_id", hold.SeatID)
	return nil
}

// Checkout turns a hold into a booking:
// load hold → lock seat, charge, store booking (one transaction) → delete hold → publish.
func (s *Service) Checkout(ctx context.Context, holdID string, req CheckoutRequest) (Booking, error) {
	log := logging.FromContext(ctx).With("hold_id", holdID)
	fail := func(reason string, err error) (Booking, error) {
		s.Metrics.CheckoutFailures.WithLabelValues(reason).Inc()
		log.Warn("checkout failed", "reason", reason, "error", err.Error())
		return Booking{}, err
	}

	card, ok := normalizeCard(req.CardNumber)
	if !ok {
		return Booking{}, httpx.NewError(http.StatusBadRequest, CodeInvalidCard, "Please enter a valid card number.")
	}
	if strings.TrimSpace(req.CardName) == "" {
		return Booking{}, badRequest("Please enter the name on the card.")
	}

	// 1. The hold must still exist.
	hold, err := s.Holds.Get(ctx, holdID)
	if errors.Is(err, ErrNotFound) {
		return fail("expired", httpx.NewError(http.StatusGone, CodeHoldExpired,
			"Your seat hold has expired. Please pick a seat again."))
	}
	if err != nil {
		return fail("internal", fmt.Errorf("load hold: %w", err))
	}
	log = log.With("event_id", hold.EventID, "seat_id", hold.SeatID)

	// 2–3. With the seat row locked: charge the card, then record the booking
	// and mark the seat sold, all in one transaction. The lock means nobody
	// can sell this seat while the payment is in flight, so a successful
	// charge is never followed by "seat already sold".
	var reason string // failure reason for metrics, set where the failure happens
	b, err := s.Store.ConfirmBooking(ctx, hold.EventID, hold.SeatID, func(seat Seat) (Booking, error) {
		// The price is always read from the database, never trusted from the client.
		// The hold ID is the idempotency key, so a retried checkout never charges twice.
		chargeID, err := s.Payments.Charge(ctx, ChargeRequest{
			AmountCents: seat.PriceCents,
			Currency:    seat.Currency,
			CardNumber:  card,
			Reference:   holdID,
		})
		switch {
		case err == nil:
		case errors.Is(err, ErrDeclined):
			reason = "declined"
			return Booking{}, httpx.NewError(http.StatusPaymentRequired, CodePaymentDeclined,
				"Your card was declined. Please try a different card.")
		case errors.Is(err, ErrPaymentTimeout):
			reason = "payment_timeout"
			return Booking{}, wrapAPI(err, http.StatusGatewayTimeout, CodePaymentTimeout,
				"Payment is taking longer than usual. Please try again.")
		case errors.Is(err, ErrPaymentRejected):
			reason = "declined"
			return Booking{}, wrapAPI(err, http.StatusBadRequest, CodeInvalidCard,
				"The card details were not accepted. Please check them and try again.")
		default:
			reason = "payment_unavailable"
			return Booking{}, wrapAPI(err, http.StatusBadGateway, CodePaymentUnavailable,
				"We couldn't reach the payment provider. Please try again.")
		}
		return Booking{
			ID:          s.NewID(),
			EventID:     hold.EventID,
			SeatID:      hold.SeatID,
			Email:       hold.Email,
			AmountCents: seat.PriceCents,
			Currency:    seat.Currency,
			ChargeID:    chargeID,
			Status:      "confirmed",
			CreatedAt:   s.Now().UTC(),
		}, nil
	})
	switch {
	case err == nil:
	case reason != "":
		return fail(reason, err)
	case errors.Is(err, ErrConflict):
		return fail("conflict", seatUnavailable(hold.SeatID))
	default:
		return fail("internal", fmt.Errorf("store booking: %w", err))
	}

	// 4. The hold has done its job.
	if err := s.Holds.Delete(ctx, hold); err != nil {
		log.Error("failed to delete hold after checkout", "error", err.Error())
	}

	// 5. Tell the rest of the system. The booking stands even if this fails.
	evt := messages.BookingConfirmed{
		BookingID:   b.ID,
		EventID:     b.EventID,
		SeatID:      b.SeatID,
		Email:       b.Email,
		AmountCents: b.AmountCents,
		Currency:    b.Currency,
		ConfirmedAt: b.CreatedAt,
	}
	if err := s.Publisher.BookingConfirmed(ctx, evt); err != nil {
		log.Error("failed to publish booking confirmation", "booking_id", b.ID, "error", err.Error())
	}

	s.Metrics.BookingsConfirmed.Inc()
	log.Info("booking confirmed", "booking_id", b.ID, "charge_id", b.ChargeID,
		"amount_cents", b.AmountCents, "currency", b.Currency, "email", logging.MaskEmail(b.Email))
	return b, nil
}

// GetBooking returns one booking.
func (s *Service) GetBooking(ctx context.Context, id string) (Booking, error) {
	if _, err := uuid.Parse(id); err != nil {
		return Booking{}, bookingNotFound()
	}
	b, err := s.Store.Booking(ctx, id)
	if errors.Is(err, ErrNotFound) {
		return Booking{}, bookingNotFound()
	}
	if err != nil {
		return Booking{}, fmt.Errorf("load booking: %w", err)
	}
	return b, nil
}

func bookingNotFound() error {
	return httpx.NewError(http.StatusNotFound, CodeBookingNotFound, "We couldn't find that booking.")
}

// apiError keeps the underlying cause for logs while rendering as an *httpx.Error.
// errors.Is/As see both the API error and the cause.
type apiError struct {
	api   *httpx.Error
	cause error
}

func (e *apiError) Error() string   { return e.cause.Error() }
func (e *apiError) Unwrap() []error { return []error{e.api, e.cause} }

func wrapAPI(cause error, status int, code, msg string) error {
	return &apiError{api: httpx.NewError(status, code, msg), cause: cause}
}

// normalizeCard strips spaces and dashes and checks the number is 12–19 digits.
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
