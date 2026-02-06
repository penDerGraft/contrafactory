// Package transport provides HTTP handlers for the deployments domain.
package transport

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"

	"github.com/pendergraft/contrafactory/internal/deployments/domain"
)

// Service defines the deployment service interface for HTTP transport.
type Service interface {
	Record(ctx context.Context, req domain.RecordRequest) (*domain.Deployment, error)
	Get(ctx context.Context, chainID, address string) (*domain.Deployment, error)
	List(ctx context.Context, filter domain.ListFilter, pagination domain.PaginationParams) (*domain.ListResult, error)
	ListByPackage(ctx context.Context, packageName, version string) ([]domain.DeploymentSummary, error)
}

// Handler handles HTTP requests for deployments.
type Handler struct {
	svc Service
}

// NewHandler creates a new deployments HTTP handler.
func NewHandler(svc Service) *Handler {
	return &Handler{svc: svc}
}

// RegisterRoutes registers all deployment routes on a chi router.
// Deprecated: Use RegisterReadRoutes and RegisterWriteRoutes for proper auth separation.
func (h *Handler) RegisterRoutes(r chi.Router) {
	h.RegisterReadRoutes(r)
	h.RegisterWriteRoutes(r)
}

// RegisterReadRoutes registers read-only deployment routes (no auth required).
func (h *Handler) RegisterReadRoutes(r chi.Router) {
	r.Get("/", h.handleList)
	r.Get("/{chainId}/{address}", h.handleGet)
}

// RegisterWriteRoutes registers write deployment routes (auth required).
func (h *Handler) RegisterWriteRoutes(r chi.Router) {
	r.Post("/", h.handleRecord)
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	var verified *bool
	if v := r.URL.Query().Get("verified"); v != "" {
		b := v == "true"
		verified = &b
	}

	result, err := h.svc.List(r.Context(), domain.ListFilter{
		Chain:    r.URL.Query().Get("chain"),
		ChainID:  r.URL.Query().Get("chain_id"),
		Package:  r.URL.Query().Get("package"),
		Verified: verified,
	}, domain.PaginationParams{
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list deployments")
		return
	}

	data := make([]map[string]any, len(result.Deployments))
	for i, d := range result.Deployments {
		data[i] = map[string]any{
			"chainId":      d.ChainID,
			"address":      d.Address,
			"contractName": d.ContractName,
			"verified":     d.Verified,
			"txHash":       d.TxHash,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"data": data,
		"pagination": map[string]any{
			"limit":      limit,
			"hasMore":    result.HasMore,
			"nextCursor": result.NextCursor,
		},
	})
}

func (h *Handler) handleRecord(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to read request body")
		return
	}

	var req domain.RecordRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON")
		return
	}

	deployment, err := h.svc.Record(r.Context(), req)
	if err != nil {
		switch {
		case errors.Is(err, domain.ErrPackageNotFound):
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Package not found")
		case errors.Is(err, domain.ErrInvalidAddress):
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		case errors.Is(err, domain.ErrInvalidChainID):
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to record deployment")
		}
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"id":       deployment.ID,
		"chainId":  deployment.ChainID,
		"address":  deployment.Address,
		"verified": deployment.Verified,
		"message":  "Deployment recorded successfully",
	})
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	chainID := chi.URLParam(r, "chainId")
	address := chi.URLParam(r, "address")

	deployment, err := h.svc.Get(r.Context(), chainID, address)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Deployment not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get deployment")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"id":              deployment.ID,
		"chainId":         deployment.ChainID,
		"address":         deployment.Address,
		"contractName":    deployment.ContractName,
		"deployerAddress": deployment.DeployerAddress,
		"txHash":          deployment.TxHash,
		"blockNumber":     deployment.BlockNumber,
		"verified":        deployment.Verified,
		"verifiedOn":      deployment.VerifiedOn,
		"createdAt":       deployment.CreatedAt,
	})
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
