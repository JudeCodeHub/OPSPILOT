package booking

import (
	"net/http"

	"github.com/git4jude/opspilot/services/internal/platform/httpx"
)

// Register adds booking's routes.
func (s *Service) Register(rt *httpx.Router) {
	rt.HandleFunc("POST /holds", s.handleCreateHold)
	rt.HandleFunc("DELETE /holds/{holdId}", s.handleReleaseHold)
	rt.HandleFunc("POST /holds/{holdId}/checkout", s.handleCheckout)
	rt.HandleFunc("GET /bookings/{id}", s.handleGetBooking)
}

func (s *Service) handleCreateHold(w http.ResponseWriter, r *http.Request) {
	var req HoldRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteErr(w, r, err)
		return
	}
	resp, err := s.CreateHold(r.Context(), req)
	if err != nil {
		httpx.WriteErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, resp)
}

func (s *Service) handleReleaseHold(w http.ResponseWriter, r *http.Request) {
	if err := s.ReleaseHold(r.Context(), r.PathValue("holdId")); err != nil {
		httpx.WriteErr(w, r, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Service) handleCheckout(w http.ResponseWriter, r *http.Request) {
	var req CheckoutRequest
	if err := httpx.DecodeJSON(w, r, &req); err != nil {
		httpx.WriteErr(w, r, err)
		return
	}
	b, err := s.Checkout(r.Context(), r.PathValue("holdId"), req)
	if err != nil {
		httpx.WriteErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusCreated, b)
}

func (s *Service) handleGetBooking(w http.ResponseWriter, r *http.Request) {
	b, err := s.GetBooking(r.Context(), r.PathValue("id"))
	if err != nil {
		httpx.WriteErr(w, r, err)
		return
	}
	httpx.WriteJSON(w, http.StatusOK, b)
}
