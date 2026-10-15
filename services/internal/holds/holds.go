// Package holds defines the Redis representation of seat holds. booking
// writes holds; events reads them to show seats as "held" on the seat map.
//
//	hold:seat:{eventId}:{seatId} → holdId   (SET NX, TTL = hold duration)
//	hold:{holdId}                → Hold JSON (same TTL)
//
// Both keys always carry a TTL so abandoned holds expire on their own.
package holds

import (
	"time"
)

// Hold is a temporary reservation of one seat.
type Hold struct {
	HoldID    string    `json:"holdId"`
	EventID   string    `json:"eventId"`
	SeatID    string    `json:"seatId"`
	Email     string    `json:"email"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// SeatKey is the per-seat lock key.
func SeatKey(eventID, seatID string) string {
	return "hold:seat:" + eventID + ":" + seatID
}

// HoldKey is the key holding the hold details.
func HoldKey(holdID string) string {
	return "hold:" + holdID
}
