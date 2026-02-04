// Package transport provides HTTP handlers for the packages domain.
package transport

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"

	"github.com/go-chi/chi/v5"
	"github.com/pendergraft/contrafactory/internal/auth"
	"github.com/pendergraft/contrafactory/internal/packages/domain"
)

// DeploymentLister is an interface for listing deployments by package
type DeploymentLister interface {
	ListByPackage(ctx context.Context, packageName, version string) ([]DeploymentSummary, error)
}

// DeploymentSummary is a summary of a deployment
type DeploymentSummary struct {
	ChainID      string `json:"chainId"`
	Address      string `json:"address"`
	ContractName string `json:"contractName"`
	Verified     bool   `json:"verified"`
	TxHash       string `json:"txHash,omitempty"`
}

// Handler handles HTTP requests for packages.
type Handler struct {
	svc         domain.Service
	deployments DeploymentLister
}

// NewHandler creates a new packages HTTP handler.
func NewHandler(svc domain.Service) *Handler {
	return &Handler{svc: svc}
}

// SetDeploymentLister sets the deployment lister for version deployments endpoint
func (h *Handler) SetDeploymentLister(dl DeploymentLister) {
	h.deployments = dl
}

// RegisterRoutes registers all package routes on a chi router.
// Deprecated: Use RegisterReadRoutes and RegisterWriteRoutes for proper auth separation.
func (h *Handler) RegisterRoutes(r chi.Router) {
	h.RegisterReadRoutes(r)
	h.RegisterWriteRoutes(r)
}

// RegisterReadRoutes registers read-only package routes (no auth required).
func (h *Handler) RegisterReadRoutes(r chi.Router) {
	r.Get("/", h.handleList)
	r.Get("/{name}", h.handleGetVersions)
	r.Get("/{name}/{version}", h.handleGet)

	// Archive route
	r.Get("/{name}/{version}/archive", h.handleGetArchive)

	// Deployments for version
	r.Get("/{name}/{version}/deployments", h.handleGetVersionDeployments)

	// Contract routes
	r.Get("/{name}/{version}/contracts", h.handleListContracts)
	r.Get("/{name}/{version}/contracts/{contract}", h.handleGetContract)
	r.Get("/{name}/{version}/contracts/{contract}/abi", h.handleGetABI)
	r.Get("/{name}/{version}/contracts/{contract}/bytecode", h.handleGetBytecode)
	r.Get("/{name}/{version}/contracts/{contract}/deployed-bytecode", h.handleGetDeployedBytecode)
	r.Get("/{name}/{version}/contracts/{contract}/standard-json-input", h.handleGetStandardJSON)
}

// RegisterWriteRoutes registers write package routes (auth required).
func (h *Handler) RegisterWriteRoutes(r chi.Router) {
	r.Post("/{name}/{version}", h.handlePublish)
	r.Delete("/{name}/{version}", h.handleDelete)
}

func (h *Handler) handleList(w http.ResponseWriter, r *http.Request) {
	limit := 20
	if l := r.URL.Query().Get("limit"); l != "" {
		if parsed, err := strconv.Atoi(l); err == nil && parsed > 0 && parsed <= 100 {
			limit = parsed
		}
	}

	result, err := h.svc.List(r.Context(), domain.ListFilter{
		Query: r.URL.Query().Get("q"),
		Chain: r.URL.Query().Get("chain"),
		Sort:  r.URL.Query().Get("sort"),
		Order: r.URL.Query().Get("order"),
	}, domain.PaginationParams{
		Limit:  limit,
		Cursor: r.URL.Query().Get("cursor"),
	})
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list packages")
		return
	}

	// Convert to response format
	data := make([]map[string]any, len(result.Packages))
	for i, p := range result.Packages {
		data[i] = map[string]any{
			"name":     p.Name,
			"chain":    p.Chain,
			"builder":  p.Builder,
			"versions": p.Versions,
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

func (h *Handler) handleGetVersions(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	includePrerelease := r.URL.Query().Get("include_prerelease") == "true"

	result, err := h.svc.GetVersions(r.Context(), name, includePrerelease)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Package not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get package")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"name":     result.Name,
		"chain":    result.Chain,
		"builder":  result.Builder,
		"versions": result.Versions,
	})
}

func (h *Handler) handleGet(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	version := chi.URLParam(r, "version")

	pkg, err := h.svc.Get(r.Context(), name, version)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Package version not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get package")
		return
	}

	contracts, err := h.svc.GetContracts(r.Context(), name, version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list contracts")
		return
	}

	contractNames := make([]string, len(contracts))
	for i, c := range contracts {
		contractNames[i] = c.Name
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"name":            pkg.Name,
		"version":         pkg.Version,
		"chain":           pkg.Chain,
		"builder":         pkg.Builder,
		"compilerVersion": pkg.CompilerVersion,
		"contracts":       contractNames,
		"createdAt":       pkg.CreatedAt,
	})
}

func (h *Handler) handlePublish(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	version := chi.URLParam(r, "version")

	// Check size limit (50MB)
	r.Body = http.MaxBytesReader(w, r.Body, 50*1024*1024)

	body, err := io.ReadAll(r.Body)
	if err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Failed to read request body")
		return
	}

	var req domain.PublishRequest
	if err := json.Unmarshal(body, &req); err != nil {
		writeError(w, http.StatusBadRequest, "INVALID_REQUEST", "Invalid JSON")
		return
	}

	ownerID := auth.GetOwnerIDFromContext(r.Context())

	if err := h.svc.Publish(r.Context(), name, version, ownerID, req); err != nil {
		switch {
		case errors.Is(err, domain.ErrInvalidName):
			writeError(w, http.StatusBadRequest, "INVALID_REQUEST", err.Error())
		case errors.Is(err, domain.ErrInvalidVersion):
			writeError(w, http.StatusBadRequest, "INVALID_VERSION", err.Error())
		case errors.Is(err, domain.ErrVersionExists):
			writeError(w, http.StatusConflict, "VERSION_EXISTS", "Version already exists and is immutable")
		case errors.Is(err, domain.ErrForbidden):
			writeError(w, http.StatusForbidden, "FORBIDDEN", "Package owned by another user")
		default:
			writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to publish package")
		}
		return
	}

	writeJSON(w, http.StatusCreated, map[string]any{
		"name":    name,
		"version": version,
		"message": "Package published successfully",
	})
}

func (h *Handler) handleDelete(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	version := chi.URLParam(r, "version")

	ownerID := auth.GetOwnerIDFromContext(r.Context())

	if err := h.svc.Delete(r.Context(), name, version, ownerID); err != nil {
		if errors.Is(err, domain.ErrForbidden) {
			writeError(w, http.StatusForbidden, "FORBIDDEN", "Package owned by another user")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to delete package")
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) handleGetArchive(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	version := chi.URLParam(r, "version")

	content, err := h.svc.GetArchive(r.Context(), name, version)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Package version not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to generate archive")
		return
	}

	filename := fmt.Sprintf("%s-%s.tar.gz", name, version)
	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Disposition", fmt.Sprintf("attachment; filename=%q", filename))
	w.Header().Set("Content-Length", strconv.Itoa(len(content)))
	w.WriteHeader(http.StatusOK)
	w.Write(content)
}

func (h *Handler) handleGetVersionDeployments(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	version := chi.URLParam(r, "version")

	// First verify the package exists
	_, err := h.svc.Get(r.Context(), name, version)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Package version not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get package")
		return
	}

	// Check if deployment lister is configured
	if h.deployments == nil {
		writeJSON(w, http.StatusOK, map[string]any{
			"deployments": []any{},
		})
		return
	}

	deployments, err := h.deployments.ListByPackage(r.Context(), name, version)
	if err != nil {
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list deployments")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"deployments": deployments,
	})
}

func (h *Handler) handleListContracts(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	version := chi.URLParam(r, "version")

	contracts, err := h.svc.GetContracts(r.Context(), name, version)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Package not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to list contracts")
		return
	}

	data := make([]map[string]any, len(contracts))
	for i, c := range contracts {
		data[i] = map[string]any{
			"name":       c.Name,
			"sourcePath": c.SourcePath,
			"chain":      c.Chain,
		}
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"contracts": data,
	})
}

func (h *Handler) handleGetContract(w http.ResponseWriter, r *http.Request) {
	name := chi.URLParam(r, "name")
	version := chi.URLParam(r, "version")
	contractName := chi.URLParam(r, "contract")

	contract, err := h.svc.GetContract(r.Context(), name, version, contractName)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Contract not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get contract")
		return
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"name":       contract.Name,
		"sourcePath": contract.SourcePath,
		"chain":      contract.Chain,
		"license":    contract.License,
	})
}

func (h *Handler) handleGetABI(w http.ResponseWriter, r *http.Request) {
	h.handleGetArtifact(w, r, "abi")
}

func (h *Handler) handleGetBytecode(w http.ResponseWriter, r *http.Request) {
	h.handleGetArtifact(w, r, "bytecode")
}

func (h *Handler) handleGetDeployedBytecode(w http.ResponseWriter, r *http.Request) {
	h.handleGetArtifact(w, r, "deployed-bytecode")
}

func (h *Handler) handleGetStandardJSON(w http.ResponseWriter, r *http.Request) {
	h.handleGetArtifact(w, r, "standard-json-input")
}

func (h *Handler) handleGetArtifact(w http.ResponseWriter, r *http.Request, artifactType string) {
	name := chi.URLParam(r, "name")
	version := chi.URLParam(r, "version")
	contractName := chi.URLParam(r, "contract")

	content, err := h.svc.GetArtifact(r.Context(), name, version, contractName, artifactType)
	if err != nil {
		if errors.Is(err, domain.ErrNotFound) {
			writeError(w, http.StatusNotFound, "NOT_FOUND", "Artifact not found")
			return
		}
		writeError(w, http.StatusInternalServerError, "INTERNAL_ERROR", "Failed to get artifact")
		return
	}

	// For JSON artifacts, set proper content type
	if artifactType == "abi" || artifactType == "standard-json-input" {
		w.Header().Set("Content-Type", "application/json")
	} else {
		w.Header().Set("Content-Type", "text/plain")
	}
	w.WriteHeader(http.StatusOK)
	w.Write(content)
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
