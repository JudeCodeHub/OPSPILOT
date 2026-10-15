package booking

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus/testutil"

	"github.com/git4jude/opspilot/services/internal/holds"
	"github.com/git4jude/opspilot/services/internal/messages"
	"github.com/git4jude/opspilot/services/internal/platform/httpx"
	"github.com/git4jude/opspilot/services/internal/platform/metrics"
)

const (
	eventID = "6f1c2b9e-1d1a-4a4e-9a51-0a8d7a1f0e01"
	email   = "jude@example.com"
)

// --- fakes -------------------------------------------------------------------

type fakeStore struct {
	mu       sync.Mutex
	seats    map[string]Seat // key: seatID
	bookings map[string]Booking
	failNext error
}

func newFakeStore() *fakeStore {
	return &fakeStore{
		seats: map[string]Seat{
			"A1":  {EventID: eventID, SeatID: "A1", PriceCents: 2_250_000, Currency: "LKR", Status: "available"},
			"J12": {EventID: eventID, SeatID: "J12", PriceCents: 750_000, Currency: "LKR", Status: "sold"},
		},
		bookings: map[string]Booking{},
	}
}

func (s *fakeStore) Seat(_ context.Context, _, seatID string) (Seat, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seat, ok := s.seats[seatID]
	if !ok {
		return Seat{}, ErrNotFound
	}
	return seat, nil
}

func (s *fakeStore) ConfirmBooking(_ context.Context, _, seatID string, pay func(Seat) (Booking, error)) (Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	seat, ok := s.seats[seatID]
	if !ok {
		return Booking{}, ErrNotFound
	}
	if seat.Status == "sold" {
		return Booking{}, ErrConflict
	}
	b, err := pay(seat)
	if err != nil {
		return Booking{}, err
	}
	if err := s.failNext; err != nil {
		s.failNext = nil
		return Booking{}, err
	}
	seat.Status = "sold"
	s.seats[seatID] = seat
	s.bookings[b.ID] = b
	return b, nil
}

func (s *fakeStore) Booking(_ context.Context, id string) (Booking, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	b, ok := s.bookings[id]
	if !ok {
		return Booking{}, ErrNotFound
	}
	return b, nil
}

// fakeHolds mimics Redis SET NX semantics in memory.
type fakeHolds struct {
	mu     sync.Mutex
	bySeat map[string]string // seat key → hold ID
	byID   map[string]holds.Hold
}

func newFakeHolds() *fakeHolds {
	return &fakeHolds{bySeat: map[string]string{}, byID: map[string]holds.Hold{}}
}

func (h *fakeHolds) Create(_ context.Context, hold holds.Hold, _ time.Duration) (bool, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := holds.SeatKey(hold.EventID, hold.SeatID)
	if _, taken := h.bySeat[key]; taken {
		return false, nil
	}
	h.bySeat[key] = hold.HoldID
	h.byID[hold.HoldID] = hold
	return true, nil
}

func (h *fakeHolds) Get(_ context.Context, id string) (holds.Hold, error) {
	h.mu.Lock()
	defer h.mu.Unlock()
	hold, ok := h.byID[id]
	if !ok {
		return holds.Hold{}, ErrNotFound
	}
	return hold, nil
}

func (h *fakeHolds) Delete(_ context.Context, hold holds.Hold) error {
	h.mu.Lock()
	defer h.mu.Unlock()
	key := holds.SeatKey(hold.EventID, hold.SeatID)
	if h.bySeat[key] == hold.HoldID {
		delete(h.bySeat, key)
	}
	delete(h.byID, hold.HoldID)
	return nil
}

// expire simulates the TTL running out.
func (h *fakeHolds) expire(id string) {
	h.mu.Lock()
	defer h.mu.Unlock()
	hold := h.byID[id]
	delete(h.bySeat, holds.SeatKey(hold.EventID, hold.SeatID))
	delete(h.byID, id)
}

type fakePayments struct {
	err   error
	calls []ChargeRequest
}

func (p *fakePayments) Charge(_ context.Context, req ChargeRequest) (string, error) {
	p.calls = append(p.calls, req)
	if p.err != nil {
		return "", p.err
	}
	return "ch_test", nil
}

type fakePublisher struct {
	err       error
	published []messages.BookingConfirmed
}

func (p *fakePublisher) BookingConfirmed(_ context.Context, evt messages.BookingConfirmed) error {
	p.published = append(p.published, evt)
	return p.err
}

// --- helpers -----------------------------------------------------------------

type fixture struct {
	svc       *Service
	store     *fakeStore
	holds     *fakeHolds
	payments  *fakePayments
	publisher *fakePublisher
}

func newFixture() *fixture {
	f := &fixture{
		store:     newFakeStore(),
		holds:     newFakeHolds(),
		payments:  &fakePayments{},
		publisher: &fakePublisher{},
	}
	n := 0
	f.svc = &Service{
		Store:     f.store,
		Holds:     f.holds,
		Payments:  f.payments,
		Publisher: f.publisher,
		Metrics:   NewMetrics(metrics.New("booking-test")),
		HoldTTL:   5 * time.Minute,
		Now:       func() time.Time { return time.Date(2026, 3, 1, 10, 0, 0, 0, time.UTC) },
		NewID: func() string {
			n++
			return fmt.Sprintf("00000000-0000-4000-8000-%012d", n)
		},
	}
	return f
}

func (f *fixture) hold(t *testing.T, seatID string) HoldResponse {
	t.Helper()
	resp, err := f.svc.CreateHold(context.Background(), HoldRequest{EventID: eventID, SeatID: seatID, Email: email})
	if err != nil {
		t.Fatalf("CreateHold: %v", err)
	}
	return resp
}

var validCard = CheckoutRequest{CardNumber: "4242 4242 4242 4242", CardName: "Jude Test"}

// apiCode returns the API status and code carried by err.
func apiCode(err error) (int, string) {
	var e *httpx.Error
	if errors.As(err, &e) {
		return e.Status, e.Code
	}
	return 0, ""
}

func failures(f *fixture, reason string) float64 {
	return testutil.ToFloat64(f.svc.Metrics.CheckoutFailures.WithLabelValues(reason))
}

// --- holds -------------------------------------------------------------------

func TestCreateHold(t *testing.T) {
	f := newFixture()
	resp := f.hold(t, "a1") // lower case is normalised

	if resp.SeatID != "A1" || resp.PriceCents != 2_250_000 || resp.Currency != "LKR" {
		t.Errorf("unexpected response: %+v", resp)
	}
	if want := f.svc.Now().Add(5 * time.Minute); !resp.ExpiresAt.Equal(want) {
		t.Errorf("ExpiresAt = %v, want %v", resp.ExpiresAt, want)
	}
	if got := testutil.ToFloat64(f.svc.Metrics.HoldsCreated); got != 1 {
		t.Errorf("booking_holds_created_total = %v", got)
	}
}

func TestSecondHoldOnSameSeatConflicts(t *testing.T) {
	f := newFixture()
	f.hold(t, "A1")

	_, err := f.svc.CreateHold(context.Background(), HoldRequest{EventID: eventID, SeatID: "A1", Email: "other@example.com"})
	if status, code := apiCode(err); status != http.StatusConflict || code != CodeSeatUnavailable {
		t.Fatalf("got %d %s, want 409 SEAT_UNAVAILABLE", status, code)
	}
	if got := testutil.ToFloat64(f.svc.Metrics.HoldConflicts); got != 1 {
		t.Errorf("booking_hold_conflicts_total = %v", got)
	}
}

func TestHoldValidation(t *testing.T) {
	f := newFixture()
	cases := []struct {
		name   string
		req    HoldRequest
		status int
		code   string
	}{
		{"bad event id", HoldRequest{EventID: "nope", SeatID: "A1", Email: email}, 400, httpx.CodeBadRequest},
		{"bad seat id", HoldRequest{EventID: eventID, SeatID: "A1;DROP", Email: email}, 400, httpx.CodeBadRequest},
		{"bad email", HoldRequest{EventID: eventID, SeatID: "A1", Email: "jude"}, 400, httpx.CodeBadRequest},
		{"unknown seat", HoldRequest{EventID: eventID, SeatID: "Z9", Email: email}, 404, CodeSeatNotFound},
		{"sold seat", HoldRequest{EventID: eventID, SeatID: "J12", Email: email}, 409, CodeSeatUnavailable},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := f.svc.CreateHold(context.Background(), tc.req)
			if status, code := apiCode(err); status != tc.status || code != tc.code {
				t.Errorf("got %d %s, want %d %s", status, code, tc.status, tc.code)
			}
		})
	}
}

func TestReleaseHoldFreesSeat(t *testing.T) {
	f := newFixture()
	h := f.hold(t, "A1")
	if err := f.svc.ReleaseHold(context.Background(), h.HoldID); err != nil {
		t.Fatal(err)
	}
	f.hold(t, "A1") // can be held again
	if err := f.svc.ReleaseHold(context.Background(), "unknown"); err != nil {
		t.Errorf("releasing an unknown hold should succeed, got %v", err)
	}
}

// --- checkout ----------------------------------------------------------------

func TestCheckoutSuccess(t *testing.T) {
	f := newFixture()
	h := f.hold(t, "A1")

	b, err := f.svc.Checkout(context.Background(), h.HoldID, validCard)
	if err != nil {
		t.Fatalf("Checkout: %v", err)
	}
	if b.Status != "confirmed" || b.AmountCents != 2_250_000 || b.ChargeID != "ch_test" || b.Email != email {
		t.Errorf("unexpected booking: %+v", b)
	}

	// The card was charged once with the hold ID as idempotency reference.
	if len(f.payments.calls) != 1 || f.payments.calls[0].Reference != h.HoldID || f.payments.calls[0].CardNumber != "4242424242424242" {
		t.Errorf("unexpected payment calls: %+v", f.payments.calls)
	}
	// Seat sold, hold removed, event published.
	if f.store.seats["A1"].Status != "sold" {
		t.Error("seat not marked sold")
	}
	if _, err := f.holds.Get(context.Background(), h.HoldID); !errors.Is(err, ErrNotFound) {
		t.Error("hold should be deleted after checkout")
	}
	if len(f.publisher.published) != 1 || f.publisher.published[0].BookingID != b.ID {
		t.Errorf("booking.confirmed not published: %+v", f.publisher.published)
	}
	if got := testutil.ToFloat64(f.svc.Metrics.BookingsConfirmed); got != 1 {
		t.Errorf("bookings_confirmed_total = %v", got)
	}
}

func TestCheckoutExpiredHold(t *testing.T) {
	f := newFixture()
	h := f.hold(t, "A1")
	f.holds.expire(h.HoldID)

	_, err := f.svc.Checkout(context.Background(), h.HoldID, validCard)
	if status, code := apiCode(err); status != http.StatusGone || code != CodeHoldExpired {
		t.Fatalf("got %d %s, want 410 HOLD_EXPIRED", status, code)
	}
	if len(f.payments.calls) != 0 {
		t.Error("an expired hold must not be charged")
	}
	if failures(f, "expired") != 1 {
		t.Error("expected checkout failure reason=expired")
	}
}

func TestCheckoutPaymentErrors(t *testing.T) {
	cases := []struct {
		err    error
		status int
		code   string
		reason string
	}{
		{ErrDeclined, http.StatusPaymentRequired, CodePaymentDeclined, "declined"},
		{fmt.Errorf("%w: deadline exceeded", ErrPaymentTimeout), http.StatusGatewayTimeout, CodePaymentTimeout, "payment_timeout"},
		{fmt.Errorf("%w: connection refused", ErrPaymentUnavailable), http.StatusBadGateway, CodePaymentUnavailable, "payment_unavailable"},
	}
	for _, tc := range cases {
		t.Run(tc.reason, func(t *testing.T) {
			f := newFixture()
			h := f.hold(t, "A1")
			f.payments.err = tc.err

			_, err := f.svc.Checkout(context.Background(), h.HoldID, validCard)
			if status, code := apiCode(err); status != tc.status || code != tc.code {
				t.Fatalf("got %d %s, want %d %s", status, code, tc.status, tc.code)
			}
			if failures(f, tc.reason) != 1 {
				t.Errorf("expected checkout failure reason=%s", tc.reason)
			}
			// Nothing was sold and the hold is kept so the user can retry.
			if f.store.seats["A1"].Status != "available" {
				t.Error("seat must stay available after a failed payment")
			}
			if _, err := f.holds.Get(context.Background(), h.HoldID); err != nil {
				t.Error("hold must survive a failed payment")
			}
		})
	}
}

func TestCheckoutConflictOnInsert(t *testing.T) {
	f := newFixture()
	h := f.hold(t, "A1")
	f.store.failNext = ErrConflict

	_, err := f.svc.Checkout(context.Background(), h.HoldID, validCard)
	if status, code := apiCode(err); status != http.StatusConflict || code != CodeSeatUnavailable {
		t.Fatalf("got %d %s, want 409 SEAT_UNAVAILABLE", status, code)
	}
	if failures(f, "conflict") != 1 {
		t.Error("expected checkout failure reason=conflict")
	}
}

func TestCheckoutSoldSeatIsNotCharged(t *testing.T) {
	f := newFixture()
	h := f.hold(t, "A1")
	f.store.seats["A1"] = Seat{EventID: eventID, SeatID: "A1", PriceCents: 1, Currency: "LKR", Status: "sold"}

	_, err := f.svc.Checkout(context.Background(), h.HoldID, validCard)
	if status, code := apiCode(err); status != http.StatusConflict || code != CodeSeatUnavailable {
		t.Fatalf("got %d %s, want 409 SEAT_UNAVAILABLE", status, code)
	}
	if len(f.payments.calls) != 0 {
		t.Error("a sold seat must not be charged")
	}
}

func TestCheckoutSucceedsWhenPublishFails(t *testing.T) {
	f := newFixture()
	h := f.hold(t, "A1")
	f.publisher.err = errors.New("nats: timeout")

	if _, err := f.svc.Checkout(context.Background(), h.HoldID, validCard); err != nil {
		t.Fatalf("booking must succeed even if publishing fails: %v", err)
	}
}

func TestCheckoutValidatesCard(t *testing.T) {
	f := newFixture()
	h := f.hold(t, "A1")
	_, err := f.svc.Checkout(context.Background(), h.HoldID, CheckoutRequest{CardNumber: "4242", CardName: "X"})
	if status, code := apiCode(err); status != http.StatusBadRequest || code != CodeInvalidCard {
		t.Fatalf("got %d %s, want 400 INVALID_CARD", status, code)
	}
	_, err = f.svc.Checkout(context.Background(), h.HoldID, CheckoutRequest{CardNumber: "4242424242424242"})
	if status, _ := apiCode(err); status != http.StatusBadRequest {
		t.Fatalf("missing card name: got %d, want 400", status)
	}
}
