// Package seed inserts the demo catalogue: 3 venues and 6 concerts, each with
// a 10 × 12 seat map. All venues and artists are fictional.
package seed

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type venue struct {
	name, city string
}

type event struct {
	name, artist, theme, description string
	venue                            int // index into venues
	daysFromNow                      int
	basePrice                        int // Standard price in LKR (whole rupees)
}

var venues = []venue{
	{name: "Harbourlight Arena", city: "Colombo"},
	{name: "Mistvale Amphitheatre", city: "Kandy"},
	{name: "Old Rampart Open-Air Stage", city: "Galle"},
}

var events = []event{
	{
		name: "Northern Lights Live", artist: "Aurora Vale", theme: "aurora", venue: 0, daysFromNow: 9, basePrice: 7500,
		description: "Shimmering synth-pop and soaring vocals under a ceiling of light. Aurora Vale brings the full live band for one night only.",
	},
	{
		name: "Neon Monsoon Tour", artist: "The Static Lanterns", theme: "neon", venue: 0, daysFromNow: 20, basePrice: 6500,
		description: "Retro-futurist rock with wall-to-wall visuals. Expect loud guitars, louder lasers and a crowd that never sits down.",
	},
	{
		name: "Golden Hour Sessions", artist: "Sahan & The Tidewalkers", theme: "sunset", venue: 2, daysFromNow: 33, basePrice: 5500,
		description: "An open-air evening of island soul and acoustic grooves on the ramparts, timed to the last light over the ocean.",
	},
	{
		name: "Deep Current", artist: "Mira Okonda", theme: "ocean", venue: 1, daysFromNow: 47, basePrice: 6000,
		description: "Downtempo electronica and live strings drifting through the hills. A slow, immersive set built for headphones-off listening.",
	},
	{
		name: "Ember Nights", artist: "Kavi & The Ember Line", theme: "ember", venue: 1, daysFromNow: 64, basePrice: 5000,
		description: "Fiery Sinhala-fusion jazz with a horn section that refuses to quit. Late-night energy from the first note.",
	},
	{
		name: "Prism Festival Night", artist: "Lumen District", theme: "prism", venue: 0, daysFromNow: 82, basePrice: 8500,
		description: "A one-night festival headliner set: kaleidoscopic house, guest vocalists and a production built around colour.",
	},
}

const (
	rows        = "ABCDEFGHIJ"
	seatsPerRow = 12
)

// sectionFor returns the section and the price multiplier for a row.
// A–B VIP, C–E Premium, F–J Standard.
func sectionFor(row byte) (string, int) {
	switch {
	case row <= 'B':
		return "VIP", 300 // 3.0×
	case row <= 'E':
		return "Premium", 180 // 1.8×
	default:
		return "Standard", 100
	}
}

// colombo is Sri Lanka time (UTC+05:30, no daylight saving). A fixed zone
// avoids depending on tzdata in the container image.
var colombo = time.FixedZone("+0530", 5*3600+1800)

// Run seeds the catalogue if the events table is empty. It reports whether
// anything was inserted. Everything happens in one transaction, and the
// events table is locked first so two concurrent runs cannot both seed.
func Run(ctx context.Context, pool *pgxpool.Pool, log *slog.Logger) (bool, error) {
	now := time.Now().In(colombo)
	today := time.Date(now.Year(), now.Month(), now.Day(), 19, 0, 0, 0, colombo)

	seeded := false
	err := pgx.BeginFunc(ctx, pool, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, "LOCK TABLE events IN SHARE ROW EXCLUSIVE MODE"); err != nil {
			return fmt.Errorf("lock events: %w", err)
		}
		var count int
		if err := tx.QueryRow(ctx, "SELECT count(*) FROM events").Scan(&count); err != nil {
			return fmt.Errorf("count events: %w", err)
		}
		if count > 0 {
			log.Info("seed data already present, skipping", "events", count)
			return nil
		}
		seeded = true

		venueIDs := make([]uuid.UUID, len(venues))
		for i, v := range venues {
			venueIDs[i] = uuid.New()
			if _, err := tx.Exec(ctx,
				"INSERT INTO venues (id, name, city, capacity) VALUES ($1, $2, $3, $4)",
				venueIDs[i], v.name, v.city, len(rows)*seatsPerRow); err != nil {
				return fmt.Errorf("insert venue: %w", err)
			}
		}

		var seatRows [][]any
		for _, e := range events {
			eventID := uuid.New()
			startsAt := today.AddDate(0, 0, e.daysFromNow)
			if _, err := tx.Exec(ctx,
				`INSERT INTO events (id, name, artist, venue_id, starts_at, description, theme)
				 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
				eventID, e.name, e.artist, venueIDs[e.venue], startsAt, e.description, e.theme); err != nil {
				return fmt.Errorf("insert event: %w", err)
			}
			for r := 0; r < len(rows); r++ {
				section, mult := sectionFor(rows[r])
				priceCents := e.basePrice * mult // basePrice × mult/100 rupees × 100 cents
				for n := 1; n <= seatsPerRow; n++ {
					seatID := fmt.Sprintf("%c%d", rows[r], n)
					seatRows = append(seatRows, []any{eventID, seatID, section, string(rows[r]), n, priceCents, "LKR", "available"})
				}
			}
		}

		_, err := tx.CopyFrom(ctx, pgx.Identifier{"seats"},
			[]string{"event_id", "seat_id", "section", "row_label", "number", "price_cents", "currency", "status"},
			pgx.CopyFromRows(seatRows))
		if err != nil {
			return fmt.Errorf("insert seats: %w", err)
		}
		return nil
	})
	if err != nil {
		return false, err
	}
	if seeded {
		log.Info("seed data inserted", "venues", len(venues), "events", len(events), "seats_per_event", len(rows)*seatsPerRow)
	}
	return seeded, nil
}
