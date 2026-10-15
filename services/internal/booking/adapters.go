package booking

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/nats-io/nats.go/jetstream"
	"github.com/redis/go-redis/v9"

	"github.com/git4jude/opspilot/services/internal/holds"
	"github.com/git4jude/opspilot/services/internal/messages"
	"github.com/git4jude/opspilot/services/internal/platform/httpx"
)

// RedisHolds implements HoldStore with Redis.
type RedisHolds struct {
	Client *redis.Client
}

// Create implements HoldStore. SET NX on the seat key is the lock: only one
// hold per seat can exist until it is deleted or expires.
func (h *RedisHolds) Create(ctx context.Context, hold holds.Hold, ttl time.Duration) (bool, error) {
	seatKey := holds.SeatKey(hold.EventID, hold.SeatID)
	ok, err := h.Client.SetNX(ctx, seatKey, hold.HoldID, ttl).Result()
	if err != nil || !ok {
		return false, err
	}
	data, err := json.Marshal(hold)
	if err != nil {
		return false, err
	}
	if err := h.Client.Set(ctx, holds.HoldKey(hold.HoldID), data, ttl).Err(); err != nil {
		// Do not leave the seat locked without details.
		h.Client.Del(context.WithoutCancel(ctx), seatKey)
		return false, err
	}
	return true, nil
}

// Get implements HoldStore.
func (h *RedisHolds) Get(ctx context.Context, holdID string) (holds.Hold, error) {
	data, err := h.Client.Get(ctx, holds.HoldKey(holdID)).Bytes()
	if errors.Is(err, redis.Nil) {
		return holds.Hold{}, ErrNotFound
	}
	if err != nil {
		return holds.Hold{}, err
	}
	var hold holds.Hold
	if err := json.Unmarshal(data, &hold); err != nil {
		return holds.Hold{}, fmt.Errorf("decode hold: %w", err)
	}
	return hold, nil
}

// releaseScript deletes the seat key only if it still belongs to this hold
// (it may have expired and been taken by someone else), then the details key.
var releaseScript = redis.NewScript(`
if redis.call("GET", KEYS[1]) == ARGV[1] then
  redis.call("DEL", KEYS[1])
end
redis.call("DEL", KEYS[2])
return 1
`)

// Delete implements HoldStore.
func (h *RedisHolds) Delete(ctx context.Context, hold holds.Hold) error {
	keys := []string{holds.SeatKey(hold.EventID, hold.SeatID), holds.HoldKey(hold.HoldID)}
	return releaseScript.Run(ctx, h.Client, keys, hold.HoldID).Err()
}

// PaymentClient implements Payments over HTTP.
type PaymentClient struct {
	Client *httpx.Client
}

type chargeBody struct {
	AmountCents int    `json:"amountCents"`
	Currency    string `json:"currency"`
	CardNumber  string `json:"cardNumber"`
	Reference   string `json:"reference"`
}

type chargeResult struct {
	ChargeID string `json:"chargeId"`
	Status   string `json:"status"`
}

// Charge implements Payments.
func (p *PaymentClient) Charge(ctx context.Context, req ChargeRequest) (string, error) {
	body, err := json.Marshal(chargeBody(req))
	if err != nil {
		return "", err
	}
	header := http.Header{"Idempotency-Key": {req.Reference}}
	resp, err := p.Client.Do(ctx, http.MethodPost, "/charges", body, header)
	if err != nil {
		if httpx.IsTimeout(err) {
			return "", fmt.Errorf("%w: %v", ErrPaymentTimeout, err)
		}
		return "", fmt.Errorf("%w: %v", ErrPaymentUnavailable, err)
	}

	switch {
	case resp.Status == http.StatusCreated || resp.Status == http.StatusOK:
		var res chargeResult
		if err := json.Unmarshal(resp.Body, &res); err != nil || res.ChargeID == "" {
			return "", fmt.Errorf("%w: unreadable charge response", ErrPaymentUnavailable)
		}
		return res.ChargeID, nil
	case resp.Status == http.StatusPaymentRequired:
		return "", ErrDeclined
	case resp.Status >= 500:
		return "", fmt.Errorf("%w: payment returned %d", ErrPaymentUnavailable, resp.Status)
	default:
		return "", fmt.Errorf("%w: payment returned %d", ErrPaymentRejected, resp.Status)
	}
}

// NATSPublisher implements Publisher with JetStream.
type NATSPublisher struct {
	JS jetstream.JetStream
}

// BookingConfirmed implements Publisher. The booking ID is the de-duplication ID.
func (p *NATSPublisher) BookingConfirmed(ctx context.Context, evt messages.BookingConfirmed) error {
	return messages.Publish(ctx, p.JS, messages.SubjectBookingConfirmed, "booking-confirmed-"+evt.BookingID, evt)
}
