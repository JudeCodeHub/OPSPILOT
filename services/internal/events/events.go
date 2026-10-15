// Package events serves the concert catalogue and seat maps.
//
// Seat status comes from two places: "sold" is stored in Postgres, "held" is
// derived from the Redis hold keys written by booking (read with a single MGET
// for the whole seat map, never one call per seat).
package events

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/redis/go-redis/v9"

	"github.com/git4jude/opspilot/services/internal/holds"
	"github.com/git4jude/opspilot/services/internal/platform/httpx"
	"github.com/git4jude/opspilot/services/internal/platform/metrics"
)

// CodeEventNotFound is returned for unknown event IDs.
const CodeEventNotFound = "EVENT_NOT_FOUND"

// Venue is embedded in event responses.
type Venue struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	City string `json:"city"`
}

// Event is a concert with availability summary.
type Event struct {
	ID             string    `json:"id"`
	Name           string    `json:"name"`
	Artist         string    `json:"artist"`
	StartsAt       time.Time `json:"startsAt"`
	Description    string    `json:"description"`
	Theme          string    `json:"theme"`
	Venue          Venue     `json:"venue"`
	MinPriceCents  int       `json:"minPriceCents"`
	Currency       string    `json:"currency"`
	SeatsAvailable int       `json:"seatsAvailable"`
	SeatsTotal     int       `json:"seatsTotal"`
}

// Seat is one seat on the map. Status is available, held or sold.
type Seat struct {
	SeatID     string `json:"seatId"`
	Section    string `json:"section"`
	Row        string `json:"row"`
	Number     int    `json:"number"`
	PriceCents int    `json:"priceCents"`
	Currency   string `json:"currency"`
	Status     string `json:"status"`
}

// SeatMap is the response of GET /events/{id}/seats.
type SeatMap struct {
	EventID string `json:"eventId"`
	Seats   []Seat `json:"seats"`
}

// Service implements the events API.
type Service struct {
	DB    *pgxpool.Pool
	Redis *redis.Client

	// SlowQuery adds server-side latency to the seat query with pg_sleep, so
	// the time is spent inside the database, not in this process.
	SlowQuery time.Duration

	seatMapRequests prometheus.Counter
}

// New builds the service and registers its metrics.
func New(db *pgxpool.Pool, rdb *redis.Client, slowQuery time.Duration, m *metrics.Registry) *Service {
	return &Service{
		DB:        db,
		Redis:     rdb,
		SlowQuery: slowQuery,
		seatMapRequests: m.Factory.NewCounter(prometheus.CounterOpts{
			Name: "events_seat_map_requests_total",
			Help: "Seat map requests served.",
		}),
	}
}

// Register adds the routes.
func (s *Service) Register(rt *httpx.Router) {
	rt.HandleFunc("GET /events", s.handleList)
	rt.HandleFunc("GET /events/{id}", s.handleGet)
	rt.HandleFunc("GET /events/{id}/seats", s.handleSeats)
}

const eventColumns = `
	e.id, e.name, e.artist, e.starts_at, e.description, e.theme,
	v.id, v.name, v.city,
	COALESCE(min(s.price_cents), 0),
	COALESCE(min(s.currency), 'LKR'),
	count(s.seat_id) FILTER (WHERE s.status = 'available'),
	count(s.seat_id)
FROM events e
JOIN venues v ON v.id = e.venue_id
LEFT JOIN seats s ON s.event_id = e.id`

func scanEvent(row pgx.Row) (Event, error) {
	var e Event
	var id, venueID uuid.UUID
	err := row.Scan(&id, &e.Name, &e.Artist, &e.StartsAt, &e.Description, &e.Theme,
		&venueID, &e.Venue.Name, &e.Venue.City,
		&e.MinPriceCents, &e.Currency, &e.SeatsAvailable, &e.SeatsTotal)
	e.ID, e.Venue.ID = id.String(), venueID.String()
	return e, err
}

func (s *Service) handleList(w http.ResponseWriter, r *http.Request) {
	rows, err := s.DB.Query(r.Context(), "SELECT"+eventColumns+" GROUP BY e.id, v.id ORDER BY e.starts_at")
	if err != nil {
		httpx.WriteErr(w, r, fmt.Errorf("query events: %w", err))
		return
	}
	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Event, error) { return scanEvent(row) })
	if err != nil {
		httpx.WriteErr(w, r, fmt.Errorf("read events: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"events": list})
}

func (s *Service) handleGet(w http.ResponseWriter, r *http.Request) {
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	e, err := scanEvent(s.DB.QueryRow(r.Context(), "SELECT"+eventColumns+" WHERE e.id = $1 GROUP BY e.id, v.id", id))
	if errors.Is(err, pgx.ErrNoRows) {
		writeNotFound(w, r)
		return
	}
	if err != nil {
		httpx.WriteErr(w, r, fmt.Errorf("query event: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, e)
}

func (s *Service) handleSeats(w http.ResponseWriter, r *http.Request) {
	s.seatMapRequests.Inc()
	id, ok := parseID(w, r)
	if !ok {
		return
	}
	seats, err := s.seatMap(r.Context(), id)
	if err != nil {
		httpx.WriteErr(w, r, err)
		return
	}
	if len(seats) == 0 {
		writeNotFound(w, r)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, SeatMap{EventID: id.String(), Seats: seats})
}

// seatMap loads the seats from Postgres and overlays holds from Redis.
func (s *Service) seatMap(ctx context.Context, eventID uuid.UUID) ([]Seat, error) {
	query := `SELECT seat_id, section, row_label, number, price_cents, currency, status
		FROM seats WHERE event_id = $1 ORDER BY row_label, number`
	args := []any{eventID}
	if s.SlowQuery > 0 {
		query = `SELECT seat_id, section, row_label, number, price_cents, currency, status
			FROM seats CROSS JOIN (SELECT pg_sleep($2)) AS wait
			WHERE event_id = $1 ORDER BY row_label, number`
		args = append(args, s.SlowQuery.Seconds())
	}

	rows, err := s.DB.Query(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query seats: %w", err)
	}
	seats, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Seat, error) {
		var seat Seat
		err := row.Scan(&seat.SeatID, &seat.Section, &seat.Row, &seat.Number, &seat.PriceCents, &seat.Currency, &seat.Status)
		return seat, err
	})
	if err != nil {
		return nil, fmt.Errorf("read seats: %w", err)
	}
	if len(seats) == 0 {
		return nil, nil
	}

	keys := make([]string, len(seats))
	for i, seat := range seats {
		keys[i] = holds.SeatKey(eventID.String(), seat.SeatID)
	}
	held, err := s.Redis.MGet(ctx, keys...).Result()
	if err != nil {
		return nil, fmt.Errorf("read seat holds: %w", err)
	}
	for i := range seats {
		if held[i] != nil && seats[i].Status == "available" {
			seats[i].Status = "held"
		}
	}
	return seats, nil
}

func parseID(w http.ResponseWriter, r *http.Request) (uuid.UUID, bool) {
	id, err := uuid.Parse(r.PathValue("id"))
	if err != nil {
		writeNotFound(w, r)
		return uuid.UUID{}, false
	}
	return id, true
}

func writeNotFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, http.StatusNotFound, CodeEventNotFound, "We couldn't find that event.")
}
