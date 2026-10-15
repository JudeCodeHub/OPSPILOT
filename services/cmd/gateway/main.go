// Command gateway is the public API entrypoint: /api/* is forwarded to the
// events, booking and ticketing services.
package main

import (
	"errors"
	"time"

	"github.com/git4jude/opspilot/services/internal/gateway"
	"github.com/git4jude/opspilot/services/internal/platform/app"
	"github.com/git4jude/opspilot/services/internal/platform/config"
	"github.com/git4jude/opspilot/services/internal/platform/httpx"
)

func main() {
	a := app.New("gateway", errors.New("proxy error: read tcp: connection reset by peer"))

	var (
		eventsURL     = config.String("EVENTS_URL", "http://events:8080")
		bookingURL    = config.String("BOOKING_URL", "http://booking:8080")
		ticketingURL  = config.String("TICKETING_URL", "http://ticketing:8080")
		timeout       = config.Duration("UPSTREAM_TIMEOUT", 3*time.Second)
		allowedOrigin = config.String("CORS_ALLOWED_ORIGIN", "http://localhost:5173")
	)

	g := &gateway.Gateway{
		Events:    httpx.NewClient("events", eventsURL, timeout, a.Metrics),
		Booking:   httpx.NewClient("booking", bookingURL, timeout, a.Metrics),
		Ticketing: httpx.NewClient("ticketing", ticketingURL, timeout, a.Metrics),
	}
	g.Register(a.Router)
	a.WrapRoutes(gateway.CORS(allowedOrigin))

	// Readiness has no dependency checks on purpose: marking the gateway
	// unready when a backend is down would hide the real failure behind
	// "no healthy upstream" at the ingress.

	a.Chaos.Start(a.Ctx)
	a.Run(
		"events_url", eventsURL,
		"booking_url", bookingURL,
		"ticketing_url", ticketingURL,
		"upstream_timeout", timeout.String(),
		"cors_allowed_origin", allowedOrigin,
	)
}
