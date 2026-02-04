// Package server provides the HTTP server setup and wiring.
package server

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/pendergraft/contrafactory/internal/auth"
	"github.com/pendergraft/contrafactory/internal/chains"
	"github.com/pendergraft/contrafactory/internal/config"
	deploymentsDomain "github.com/pendergraft/contrafactory/internal/deployments/domain"
	deploymentsTransport "github.com/pendergraft/contrafactory/internal/deployments/transport"
	packagesDomain "github.com/pendergraft/contrafactory/internal/packages/domain"
	packagesTransport "github.com/pendergraft/contrafactory/internal/packages/transport"
	"github.com/pendergraft/contrafactory/internal/storage"
	verificationDomain "github.com/pendergraft/contrafactory/internal/verification/domain"
	verificationTransport "github.com/pendergraft/contrafactory/internal/verification/transport"
)

// Server is the HTTP server
type Server struct {
	cfg    *config.Config
	store  storage.Store
	logger *slog.Logger
	router *chi.Mux

	// Domain services
	packagesSvc     packagesDomain.Service
	deploymentsSvc  deploymentsDomain.Service
	verificationSvc verificationDomain.Service
}

// New creates a new server
func New(cfg *config.Config, store storage.Store, logger *slog.Logger) *Server {
	s := &Server{
		cfg:    cfg,
		store:  store,
		logger: logger,
		router: chi.NewRouter(),
	}

	// Create chain registry
	registry := chains.NewRegistry()

	// Create domain services with middleware
	var pkgSvc packagesDomain.Service
	pkgSvc = packagesDomain.NewService(store)
	pkgSvc = packagesDomain.LoggingMiddleware(logger)(pkgSvc)
	s.packagesSvc = pkgSvc

	s.deploymentsSvc = deploymentsDomain.NewService(store)
	s.verificationSvc = verificationDomain.NewService(store, registry)

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// Handler returns the HTTP handler
func (s *Server) Handler() http.Handler {
	return s.router
}

func (s *Server) setupMiddleware() {
	s.router.Use(middleware.RequestID)
	s.router.Use(NewLoggingMiddleware(s.logger))
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Compress(5))
	s.router.Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Access-Control-Allow-Origin", "*")
			w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, OPTIONS")
			w.Header().Set("Access-Control-Allow-Headers", "Accept, Authorization, Content-Type, X-API-Key")
			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusOK)
				return
			}
			next.ServeHTTP(w, r)
		})
	})
}

func (s *Server) setupRoutes() {
	// Health checks
	s.router.Get("/health", s.handleHealth)
	s.router.Get("/healthz", s.handleHealth)
	s.router.Get("/readyz", s.handleHealth)

	// Create HTTP handlers for each domain
	packagesHandler := packagesTransport.NewHandler(s.packagesSvc)
	deploymentsHandler := deploymentsTransport.NewHandler(s.deploymentsSvc)
	verificationHandler := verificationTransport.NewHandler(s.verificationSvc)

	// Wire up deployments lister to packages handler for version deployments endpoint
	packagesHandler.SetDeploymentLister(&deploymentListerAdapter{svc: s.deploymentsSvc})

	// Auth middleware for write operations
	requireAuth := func(r chi.Router) {
		if s.cfg.Auth.Type == "api-key" {
			r.Use(auth.Middleware(s.store, writeError))
		}
	}

	// API v1 routes
	s.router.Route("/api/v1", func(r chi.Router) {
		// Packages - split read/write
		r.Route("/packages", func(r chi.Router) {
			// Read operations - no auth required
			packagesHandler.RegisterReadRoutes(r)

			// Write operations - auth required
			r.Group(func(r chi.Router) {
				requireAuth(r)
				packagesHandler.RegisterWriteRoutes(r)
			})
		})

		// Deployments - split read/write
		r.Route("/deployments", func(r chi.Router) {
			// Read operations - no auth required
			deploymentsHandler.RegisterReadRoutes(r)

			// Write operations - auth required
			r.Group(func(r chi.Router) {
				requireAuth(r)
				deploymentsHandler.RegisterWriteRoutes(r)
			})
		})

		// Verification - read only (no auth)
		verificationHandler.RegisterRoutes(r)
	})
}

// Health check handler
func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
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

// deploymentListerAdapter adapts the deployments service to the packages transport's DeploymentLister interface
type deploymentListerAdapter struct {
	svc deploymentsDomain.Service
}

func (a *deploymentListerAdapter) ListByPackage(ctx context.Context, packageName, version string) ([]packagesTransport.DeploymentSummary, error) {
	summaries, err := a.svc.ListByPackage(ctx, packageName, version)
	if err != nil {
		return nil, err
	}

	result := make([]packagesTransport.DeploymentSummary, len(summaries))
	for i, s := range summaries {
		result[i] = packagesTransport.DeploymentSummary{
			ChainID:      s.ChainID,
			Address:      s.Address,
			ContractName: s.ContractName,
			Verified:     s.Verified,
			TxHash:       s.TxHash,
		}
	}
	return result, nil
}
