package payment

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/git4jude/opspilot/services/internal/platform/httpx"
	"github.com/git4jude/opspilot/services/internal/platform/metrics"
)

func newTestProcessor() *Processor {
	p := NewProcessor()
	n := 0
	p.NewID = func() string { n++; return "ch_" + string(rune('0'+n)) }
	return p
}

var validReq = ChargeRequest{AmountCents: 750000, Currency: "LKR", CardNumber: "4242 4242 4242 4242", Reference: "hold-1"}

func TestApprove(t *testing.T) {
	p := newTestProcessor()
	card, err := Validate(validReq)
	if err != nil {
		t.Fatal(err)
	}
	out, replayed := p.Charge("key-1", validReq, card)
	if replayed || out.Status != http.StatusCreated || out.Charge == nil || out.Charge.Status != "approved" {
		t.Fatalf("unexpected outcome: %+v replayed=%v", out, replayed)
	}
	if out.Charge.CardLast4 != "4242" {
		t.Errorf("CardLast4 = %q", out.Charge.CardLast4)
	}
}

func TestDecline(t *testing.T) {
	p := newTestProcessor()
	req := validReq
	req.CardNumber = "4000 0000 0000 0002"
	card, _ := Validate(req)
	out, _ := p.Charge("key-2", req, card)
	if out.Status != http.StatusPaymentRequired || out.Charge != nil {
		t.Fatalf("expected decline, got %+v", out)
	}

	// A decline is not remembered: the same order can be paid with another card.
	good, _ := Validate(validReq)
	retry, replayed := p.Charge("key-2", validReq, good)
	if replayed || retry.Status != http.StatusCreated {
		t.Fatalf("retry with a different card after a decline should be approved, got %+v replayed=%v", retry, replayed)
	}
}

func TestIdempotency(t *testing.T) {
	p := newTestProcessor()
	now := time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC)
	p.Now = func() time.Time { return now }
	card, _ := Validate(validReq)

	first, _ := p.Charge("same-key", validReq, card)
	second, replayed := p.Charge("same-key", validReq, card)
	if !replayed || second.Charge.ChargeID != first.Charge.ChargeID {
		t.Fatalf("same key must return the original charge: first=%s second=%s replayed=%v",
			first.Charge.ChargeID, second.Charge.ChargeID, replayed)
	}

	other, replayed := p.Charge("other-key", validReq, card)
	if replayed || other.Charge.ChargeID == first.Charge.ChargeID {
		t.Fatal("a different key must create a new charge")
	}

	// After the TTL the entry is gone and the key charges again.
	now = now.Add(IdempotencyTTL + time.Second)
	p.Sweep()
	again, replayed := p.Charge("same-key", validReq, card)
	if replayed || again.Charge.ChargeID == first.Charge.ChargeID {
		t.Fatal("expired idempotency entry must not be replayed")
	}
}

func TestValidate(t *testing.T) {
	bad := []ChargeRequest{
		{AmountCents: 100, Currency: "LKR", CardNumber: "abc"},
		{AmountCents: 100, Currency: "LKR", CardNumber: "4242"},
		{AmountCents: 0, Currency: "LKR", CardNumber: "4242424242424242"},
		{AmountCents: 100, Currency: "RUPEES", CardNumber: "4242424242424242"},
	}
	for _, req := range bad {
		if _, err := Validate(req); err == nil {
			t.Errorf("expected validation error for %+v", req)
		}
	}
}

func TestHandlerStatusCodes(t *testing.T) {
	h := NewHandler(newTestProcessor(), metrics.New("payment-test"))
	h.ProcessingTime = nil
	rt := httpx.NewRouter()
	h.Register(rt)

	post := func(key string, req ChargeRequest) *httptest.ResponseRecorder {
		body, _ := json.Marshal(req)
		r := httptest.NewRequest(http.MethodPost, "/charges", bytes.NewReader(body))
		if key != "" {
			r.Header.Set("Idempotency-Key", key)
		}
		rec := httptest.NewRecorder()
		rt.ServeHTTP(rec, r)
		return rec
	}

	if rec := post("k1", validReq); rec.Code != http.StatusCreated {
		t.Errorf("approve: status %d", rec.Code)
	}
	declined := validReq
	declined.CardNumber = DeclinedCard
	if rec := post("k2", declined); rec.Code != http.StatusPaymentRequired {
		t.Errorf("decline: status %d", rec.Code)
	}
	if rec := post("", validReq); rec.Code != http.StatusBadRequest {
		t.Errorf("missing idempotency key: status %d", rec.Code)
	}
}
