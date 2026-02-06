package domain

import (
	"context"
	"log/slog"
	"time"
)

// loggingService is the interface required for logging middleware.
type loggingService interface {
	Publish(ctx context.Context, name, version string, ownerID string, req PublishRequest) error
	Get(ctx context.Context, name, version string) (*Package, error)
	GetVersions(ctx context.Context, name string, includePrerelease bool) (*VersionsResult, error)
	List(ctx context.Context, filter ListFilter, pagination PaginationParams) (*ListResult, error)
	Delete(ctx context.Context, name, version string, ownerID string) error
	GetContracts(ctx context.Context, name, version string) ([]Contract, error)
	GetContract(ctx context.Context, name, version, contractName string) (*Contract, error)
	GetArtifact(ctx context.Context, name, version, contractName, artifactType string) ([]byte, error)
	GetArchive(ctx context.Context, name, version string) ([]byte, error)
}

// LoggingMiddleware returns a service middleware that logs all operations.
func LoggingMiddleware(logger *slog.Logger) func(loggingService) *loggingMiddleware {
	return func(next loggingService) *loggingMiddleware {
		return &loggingMiddleware{
			next:   next,
			logger: logger,
		}
	}
}

type loggingMiddleware struct {
	next   loggingService
	logger *slog.Logger
}

func (m *loggingMiddleware) Publish(ctx context.Context, name, version string, ownerID string, req PublishRequest) error {
	start := time.Now()
	err := m.next.Publish(ctx, name, version, ownerID, req)
	m.logger.Info("Publish",
		"name", name,
		"version", version,
		"chain", req.Chain,
		"artifacts", len(req.Artifacts),
		"duration", time.Since(start),
		"error", err,
	)
	return err
}

func (m *loggingMiddleware) Get(ctx context.Context, name, version string) (*Package, error) {
	start := time.Now()
	pkg, err := m.next.Get(ctx, name, version)
	m.logger.Debug("Get",
		"name", name,
		"version", version,
		"duration", time.Since(start),
		"error", err,
	)
	return pkg, err
}

func (m *loggingMiddleware) GetVersions(ctx context.Context, name string, includePrerelease bool) (*VersionsResult, error) {
	start := time.Now()
	result, err := m.next.GetVersions(ctx, name, includePrerelease)
	m.logger.Debug("GetVersions",
		"name", name,
		"includePrerelease", includePrerelease,
		"duration", time.Since(start),
		"error", err,
	)
	return result, err
}

func (m *loggingMiddleware) List(ctx context.Context, filter ListFilter, pagination PaginationParams) (*ListResult, error) {
	start := time.Now()
	result, err := m.next.List(ctx, filter, pagination)
	m.logger.Debug("List",
		"filter", filter,
		"limit", pagination.Limit,
		"duration", time.Since(start),
		"error", err,
	)
	return result, err
}

func (m *loggingMiddleware) Delete(ctx context.Context, name, version string, ownerID string) error {
	start := time.Now()
	err := m.next.Delete(ctx, name, version, ownerID)
	m.logger.Info("Delete",
		"name", name,
		"version", version,
		"duration", time.Since(start),
		"error", err,
	)
	return err
}

func (m *loggingMiddleware) GetContracts(ctx context.Context, name, version string) ([]Contract, error) {
	start := time.Now()
	contracts, err := m.next.GetContracts(ctx, name, version)
	m.logger.Debug("GetContracts",
		"name", name,
		"version", version,
		"count", len(contracts),
		"duration", time.Since(start),
		"error", err,
	)
	return contracts, err
}

func (m *loggingMiddleware) GetContract(ctx context.Context, name, version, contractName string) (*Contract, error) {
	start := time.Now()
	contract, err := m.next.GetContract(ctx, name, version, contractName)
	m.logger.Debug("GetContract",
		"name", name,
		"version", version,
		"contract", contractName,
		"duration", time.Since(start),
		"error", err,
	)
	return contract, err
}

func (m *loggingMiddleware) GetArtifact(ctx context.Context, name, version, contractName, artifactType string) ([]byte, error) {
	start := time.Now()
	content, err := m.next.GetArtifact(ctx, name, version, contractName, artifactType)
	m.logger.Debug("GetArtifact",
		"name", name,
		"version", version,
		"contract", contractName,
		"artifactType", artifactType,
		"size", len(content),
		"duration", time.Since(start),
		"error", err,
	)
	return content, err
}

func (m *loggingMiddleware) GetArchive(ctx context.Context, name, version string) ([]byte, error) {
	start := time.Now()
	content, err := m.next.GetArchive(ctx, name, version)
	m.logger.Info("GetArchive",
		"name", name,
		"version", version,
		"size", len(content),
		"duration", time.Since(start),
		"error", err,
	)
	return content, err
}
