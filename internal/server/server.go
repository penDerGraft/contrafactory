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
	"github.com/pendergraft/contrafactory/internal/middleware/logging"
	"github.com/pendergraft/contrafactory/internal/middleware/ratelimit"
	"github.com/pendergraft/contrafactory/internal/middleware/realip"
	"github.com/pendergraft/contrafactory/internal/middleware/security"
	"github.com/pendergraft/contrafactory/internal/observability/metrics"
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

	// Services typed via transport interfaces
	packagesSvc     packagesTransport.Service
	deploymentsSvc  deploymentsTransport.Service
	verificationSvc verificationTransport.Service
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

	// Create domain services
	pkgImpl := packagesDomain.NewService(store, store)
	deployImpl := deploymentsDomain.NewService(store, store)
	verifyImpl := verificationDomain.NewService(store, store, registry)

	// Wrap packages service with logging middleware
	pkgSvc := packagesDomain.LoggingMiddleware(logger)(pkgImpl)
	s.packagesSvc = pkgSvc
	s.deploymentsSvc = deployImpl
	s.verificationSvc = verifyImpl

	s.setupMiddleware()
	s.setupRoutes()

	return s
}

// Handler returns the HTTP handler
func (s *Server) Handler() http.Handler {
	return s.router
}

// MetricsHandler returns the metrics HTTP handler for separate metrics server
func (s *Server) MetricsHandler() http.Handler {
	return metrics.Handler()
}

func (s *Server) setupMiddleware() {
	// Order matters! Security middleware runs first to block malicious requests early.

	// 1. Real IP extraction (must be first to set client IP for other middleware)
	s.router.Use(realip.Middleware(realip.Config{
		TrustProxy:     s.cfg.Proxy.TrustProxy,
		TrustedProxies: s.cfg.Proxy.TrustedProxies,
	}))

	// 2. Security filter (blocks malicious patterns, bypasses health checks)
	s.router.Use(security.FilterMiddleware(s.cfg.Security.FilterEnabled))

	// 3. Body size limit
	s.router.Use(security.MaxBodySizeMiddleware(s.cfg.Security.MaxBodySizeMB))

	// 4. Rate limiting (bypasses health checks)
	s.router.Use(ratelimit.Middleware(ratelimit.Config{
		Enabled:        s.cfg.RateLimit.Enabled,
		RequestsPerMin: s.cfg.RateLimit.RequestsPerMin,
		BurstSize:      s.cfg.RateLimit.BurstSize,
		CleanupMinutes: s.cfg.RateLimit.CleanupMinutes,
	}))

	// 5. Standard middleware
	s.router.Use(middleware.RequestID)
	s.router.Use(logging.Middleware(s.logger))
	s.router.Use(metrics.Middleware)
	s.router.Use(middleware.Recoverer)
	s.router.Use(middleware.Compress(5))

	// 6. CORS
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
	// OpenAPI spec
	s.router.Get("/api/openapi.yaml", s.handleOpenAPISpec)

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

// handleOpenAPISpec serves the OpenAPI specification.
func (s *Server) handleOpenAPISpec(w http.ResponseWriter, r *http.Request) {
	http.ServeFile(w, r, "spec/openapi.yaml")
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

// deploymentLister adapts the deployments service for listing by package
type deploymentLister interface {
	ListByPackage(ctx context.Context, packageName, version string) ([]deploymentsDomain.DeploymentSummary, error)
}

// deploymentListerAdapter adapts the deployments service to the packages transport's DeploymentLister interface
type deploymentListerAdapter struct {
	svc deploymentLister
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
