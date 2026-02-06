// Package transport provides HTTP handlers for the verification domain.
package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"

	"github.com/go-chi/chi/v5"

	"github.com/pendergraft/contrafactory/internal/verification/domain"
)

// Service defines the verification service interface for HTTP transport.
type Service interface {
	Verify(ctx context.Context, req domain.VerifyRequest) (*domain.VerifyResult, error)
}

// Handler handles HTTP requests for verification.
type Handler struct {
	svc Service
}

// NewHandler creates a new verification HTTP handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers the verification routes on a chi router.
func (h *Handler) RegisterRoutes(r chi.Router) {
	r.Post("/verify", h.handleVerify)
}

func (h *Handler) handleVerify(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to read request body")
		return
	}

	var req domain.VerifyRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON")
		return
	}

	result, err := h.svc.Verify(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrNotFound):
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Package or contract not found")
		case errors.Is(err, domain.ErrInvalidAddress):
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		case errors.Is(err, domain.ErrInvalidChainID):
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		case errors.Is(err, domain.ErrChainNotFound):
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Chain not supported")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to verify contract")
		}
		return
	}

	writeJSON(w, http.StatusOK, result)
}

// Helper functions

func writeJSON(w http.ResponseWriter, status int, data any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(data)
}

func writeError(w http.ResponseWriter, status int, code, message string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	json.NewEncoder(w).Encode(map[string]any{
		"error": map[string]any{
			"code":    code,
			"message": message,
		},
	})
}
