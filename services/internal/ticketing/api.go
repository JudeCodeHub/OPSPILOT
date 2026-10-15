// Package ticketing issues tickets asynchronously from booking.confirmed
// messages and serves them over HTTP.
package ticketing

import (
	"errors"
	"fmt"
	"net/http"
	"net/mail"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/git4jude/opspilot/services/internal/platform/httpx"
)

// CodeTicketNotFound is returned for unknown ticket codes.
const CodeTicketNotFound = "TICKET_NOT_FOUND"

// Ticket is a ticket joined with its event, venue and seat.
type Ticket struct {
	Code      string    `json:"code"`
	BookingID string    `json:"bookingId"`
	EventID   string    `json:"eventId"`
	EventName string    `json:"eventName"`
	Artist    string    `json:"artist"`
	StartsAt  time.Time `json:"startsAt"`
	Theme     string    `json:"theme"`
	VenueName string    `json:"venueName"`
	VenueCity string    `json:"venueCity"`
	SeatID    string    `json:"seatId"`
	Section   string    `json:"section"`
	Row       string    `json:"row"`
	Number    int       `json:"number"`
	Email     string    `json:"email"`
	IssuedAt  time.Time `json:"issuedAt"`
}

// API serves tickets.
type API struct {
	DB *pgxpool.Pool
}

// Register adds the routes.
func (a *API) Register(rt *httpx.Router) {
	rt.HandleFunc("GET /tickets", a.handleList)
	rt.HandleFunc("GET /tickets/{code}", a.handleGet)
}

const ticketQuery = `
SELECT t.code, t.booking_id, t.event_id, e.name, e.artist, e.starts_at, e.theme,
       v.name, v.city, t.seat_id, COALESCE(s.section, ''), COALESCE(s.row_label, ''),
       COALESCE(s.number, 0), t.email, t.issued_at
FROM tickets t
JOIN events e ON e.id = t.event_id
JOIN venues v ON v.id = e.venue_id
LEFT JOIN seats s ON s.event_id = t.event_id AND s.seat_id = t.seat_id`

func scanTicket(row pgx.Row) (Ticket, error) {
	var t Ticket
	var bookingID, eventID uuid.UUID
	err := row.Scan(&t.Code, &bookingID, &eventID, &t.EventName, &t.Artist, &t.StartsAt, &t.Theme,
		&t.VenueName, &t.VenueCity, &t.SeatID, &t.Section, &t.Row, &t.Number, &t.Email, &t.IssuedAt)
	t.BookingID, t.EventID = bookingID.String(), eventID.String()
	return t, err
}

func (a *API) handleList(w http.ResponseWriter, r *http.Request) {
	email := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("email")))
	if _, err := mail.ParseAddress(email); err != nil {
		httpx.WriteError(w, r, http.StatusBadRequest, httpx.CodeBadRequest, "Please enter a valid email address.")
		return
	}
	rows, err := a.DB.Query(r.Context(), ticketQuery+` WHERE t.email = $1 ORDER BY t.issued_at DESC LIMIT 100`, email)
	if err != nil {
		httpx.WriteErr(w, r, fmt.Errorf("query tickets: %w", err))
		return
	}
	tickets, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (Ticket, error) { return scanTicket(row) })
	if err != nil {
		httpx.WriteErr(w, r, fmt.Errorf("read tickets: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, map[string]any{"tickets": tickets})
}

func (a *API) handleGet(w http.ResponseWriter, r *http.Request) {
	code := strings.ToUpper(r.PathValue("code"))
	if !ValidCode(code) {
		writeNotFound(w, r)
		return
	}
	t, err := scanTicket(a.DB.QueryRow(r.Context(), ticketQuery+` WHERE t.code = $1`, code))
	if errors.Is(err, pgx.ErrNoRows) {
		writeNotFound(w, r)
		return
	}
	if err != nil {
		httpx.WriteErr(w, r, fmt.Errorf("query ticket: %w", err))
		return
	}
	httpx.WriteJSON(w, http.StatusOK, t)
}

func writeNotFound(w http.ResponseWriter, r *http.Request) {
	httpx.WriteError(w, r, http.StatusNotFound, CodeTicketNotFound, "We couldn't find that ticket.")
}
