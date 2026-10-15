package booking

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/git4jude/opspilot/services/internal/platform/logging"
)

// slowAcquire is the pool wait above which a warning is logged.
const slowAcquire = 500 * time.Millisecond

// PGStore implements Store with Postgres.
type PGStore struct {
	Pool    *pgxpool.Pool
	Metrics *Metrics
}

// acquire takes a connection from the pool and measures how long that took.
// When the pool is too small for the load, requests queue here: the wait shows
// up in booking_db_pool_acquire_wait_seconds and in a warning log line.
func (s *PGStore) acquire(ctx context.Context) (*pgxpool.Conn, error) {
	start := time.Now()
	conn, err := s.Pool.Acquire(ctx)
	wait := time.Since(start)
	s.Metrics.PoolAcquireWait.Observe(wait.Seconds())
	if wait > slowAcquire {
		logging.FromContext(ctx).Warn(fmt.Sprintf("waited %dms to acquire database connection", wait.Milliseconds()),
			"wait_ms", wait.Milliseconds(),
			"pool_max_conns", s.Pool.Stat().MaxConns(),
			"pool_acquired_conns", s.Pool.Stat().AcquiredConns())
	}
	if err != nil {
		return nil, fmt.Errorf("acquire database connection: %w", err)
	}
	return conn, nil
}

// Seat implements Store.
func (s *PGStore) Seat(ctx context.Context, eventID, seatID string) (Seat, error) {
	conn, err := s.acquire(ctx)
	if err != nil {
		return Seat{}, err
	}
	defer conn.Release()

	seat := Seat{EventID: eventID, SeatID: seatID}
	err = conn.QueryRow(ctx,
		`SELECT price_cents, currency, status FROM seats WHERE event_id = $1 AND seat_id = $2`,
		eventID, seatID).Scan(&seat.PriceCents, &seat.Currency, &seat.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return Seat{}, ErrNotFound
	}
	if err != nil {
		return Seat{}, fmt.Errorf("query seat: %w", err)
	}
	return seat, nil
}

// ConfirmBooking implements Store. The connection (and the row lock) is held
// for the whole payment call, so the pool must be sized for the number of
// checkouts in flight, not just for query time.
func (s *PGStore) ConfirmBooking(ctx context.Context, eventID, seatID string, pay func(Seat) (Booking, error)) (Booking, error) {
	conn, err := s.acquire(ctx)
	if err != nil {
		return Booking{}, err
	}
	defer conn.Release()

	var b Booking
	err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		seat := Seat{EventID: eventID, SeatID: seatID}
		err := tx.QueryRow(ctx,
			`SELECT price_cents, currency, status FROM seats
			 WHERE event_id = $1 AND seat_id = $2 FOR UPDATE`,
			eventID, seatID).Scan(&seat.PriceCents, &seat.Currency, &seat.Status)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrNotFound
		}
		if err != nil {
			return fmt.Errorf("lock seat: %w", err)
		}
		if seat.Status == "sold" {
			return ErrConflict
		}

		b, err = pay(seat)
		if err != nil {
			return err // rolls back; nothing was written
		}

		_, err = tx.Exec(ctx,
			`INSERT INTO bookings (id, event_id, seat_id, email, amount_cents, currency, charge_id, status, created_at)
			 VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9)`,
			b.ID, b.EventID, b.SeatID, b.Email, b.AmountCents, b.Currency, b.ChargeID, b.Status, b.CreatedAt)
		if isUniqueViolation(err) {
			return ErrConflict
		}
		if err != nil {
			return fmt.Errorf("insert booking: %w", err)
		}
		if _, err := tx.Exec(ctx,
			`UPDATE seats SET status = 'sold' WHERE event_id = $1 AND seat_id = $2`,
			eventID, seatID); err != nil {
			return fmt.Errorf("mark seat sold: %w", err)
		}
		return nil
	})
	if err != nil {
		return Booking{}, err
	}
	return b, nil
}

// Booking implements Store.
func (s *PGStore) Booking(ctx context.Context, id string) (Booking, error) {
	conn, err := s.acquire(ctx)
	if err != nil {
		return Booking{}, err
	}
	defer conn.Release()

	var b Booking
	err = conn.QueryRow(ctx,
		`SELECT id::text, event_id::text, seat_id, email, amount_cents, currency, charge_id, status, created_at
		 FROM bookings WHERE id = $1`, id).
		Scan(&b.ID, &b.EventID, &b.SeatID, &b.Email, &b.AmountCents, &b.Currency, &b.ChargeID, &b.Status, &b.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return Booking{}, ErrNotFound
	}
	if err != nil {
		return Booking{}, fmt.Errorf("query booking: %w", err)
	}
	return b, nil
}

func isUniqueViolation(err error) bool {
	var pgErr *pgconn.PgError
	return errors.As(err, &pgErr) && pgErr.Code == "23505"
}
