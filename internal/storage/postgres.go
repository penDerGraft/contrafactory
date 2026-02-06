package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	_ "github.com/jackc/pgx/v5/stdlib"
)

// PostgresStore implements Store using PostgreSQL
type PostgresStore struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewPostgresStore creates a new Postgres store
func NewPostgresStore(url string, logger *slog.Logger) (*PostgresStore, error) {
	db, err := sql.Open("pgx", url)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("pinging database: %w", err)
	}

	return &PostgresStore{db: db, logger: logger}, nil
}

// Close closes the database connection
func (s *PostgresStore) Close() error {
	return s.db.Close()
}

// Migrate runs database migrations
func (s *PostgresStore) Migrate(ctx context.Context) error {
	schema := `
	-- Package ownership
	CREATE TABLE IF NOT EXISTS package_owners (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		package_name TEXT NOT NULL UNIQUE,
		owner_key_id UUID REFERENCES api_keys(id),
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	-- Packages
	CREATE TABLE IF NOT EXISTS packages (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		chain TEXT NOT NULL,
		builder TEXT,
		compiler_version TEXT,
		compiler_settings JSONB,
		metadata JSONB,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		UNIQUE(name, version)
	);

	-- Contracts
	CREATE TABLE IF NOT EXISTS contracts (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		package_id UUID REFERENCES packages(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		chain TEXT NOT NULL,
		source_path TEXT NOT NULL,
		license TEXT,
		primary_hash TEXT NOT NULL,
		metadata_hash TEXT,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		UNIQUE(package_id, name, source_path)
	);

	-- Artifacts
	CREATE TABLE IF NOT EXISTS artifacts (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		contract_id UUID REFERENCES contracts(id) ON DELETE CASCADE,
		artifact_type TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		content BYTEA,
		blob_store_ref TEXT,
		size_bytes INTEGER NOT NULL,
		UNIQUE(contract_id, artifact_type)
	);

	-- Deployments
	CREATE TABLE IF NOT EXISTS deployments (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		package_id UUID REFERENCES packages(id),
		contract_name TEXT NOT NULL,
		chain TEXT NOT NULL,
		chain_id TEXT NOT NULL,
		address TEXT NOT NULL,
		deployer_address TEXT,
		tx_hash TEXT,
		block_number BIGINT,
		deployment_data JSONB,
		verified BOOLEAN DEFAULT FALSE,
		verified_at TIMESTAMPTZ,
		verified_on TEXT[],
		created_at TIMESTAMPTZ DEFAULT NOW(),
		UNIQUE(chain, chain_id, address)
	);

	-- API keys (created first since package_owners references it)
	CREATE TABLE IF NOT EXISTS api_keys (
		id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
		key_hash TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		scopes JSONB,
		created_at TIMESTAMPTZ DEFAULT NOW(),
		last_used_at TIMESTAMPTZ,
		revoked_at TIMESTAMPTZ
	);

	-- Blobs
	CREATE TABLE IF NOT EXISTS blobs (
		hash TEXT PRIMARY KEY,
		content BYTEA NOT NULL,
		size_bytes INTEGER NOT NULL,
		created_at TIMESTAMPTZ DEFAULT NOW()
	);

	-- Indexes
	CREATE INDEX IF NOT EXISTS idx_packages_name ON packages(name);
	CREATE INDEX IF NOT EXISTS idx_packages_chain ON packages(chain);
	CREATE INDEX IF NOT EXISTS idx_contracts_primary_hash ON contracts(primary_hash);
	CREATE INDEX IF NOT EXISTS idx_deployments_lookup ON deployments(chain, chain_id, address);
	CREATE INDEX IF NOT EXISTS idx_artifacts_content_hash ON artifacts(content_hash);
	`

	// Need to create api_keys first since package_owners references it
	_, err := s.db.ExecContext(ctx, `
		CREATE TABLE IF NOT EXISTS api_keys (
			id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
			key_hash TEXT NOT NULL UNIQUE,
			name TEXT NOT NULL,
			scopes JSONB,
			created_at TIMESTAMPTZ DEFAULT NOW(),
			last_used_at TIMESTAMPTZ,
			revoked_at TIMESTAMPTZ
		);
	`)
	if err != nil {
		return fmt.Errorf("creating api_keys table: %w", err)
	}

	_, err = s.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	s.logger.Info("database migrations complete")
	return nil
}

// CreatePackage creates a new package
func (s *PostgresStore) CreatePackage(ctx context.Context, pkg *Package) error {
	// Serialize metadata as JSONB
	var metadataJSON []byte
	if len(pkg.Metadata) > 0 {
		data, err := json.Marshal(pkg.Metadata)
		if err != nil {
			return fmt.Errorf("serializing metadata: %w", err)
		}
		metadataJSON = data
	} else {
		metadataJSON = []byte("{}")
	}

	query := `
		INSERT INTO packages (id, name, version, chain, builder, compiler_version, compiler_settings, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := s.db.ExecContext(ctx, query, pkg.ID, pkg.Name, pkg.Version, pkg.Chain, pkg.Builder, pkg.CompilerVersion, "{}", metadataJSON)
	return err
}

// GetPackage retrieves a package by name and version
func (s *PostgresStore) GetPackage(ctx context.Context, name, version string) (*Package, error) {
	query := `
		SELECT id, name, version, chain, builder, compiler_version, metadata, created_at
		FROM packages
		WHERE name = $1 AND version = $2
	`
	var pkg Package
	var createdAt time.Time
	var metadataJSON []byte
	err := s.db.QueryRowContext(ctx, query, name, version).Scan(
		&pkg.ID, &pkg.Name, &pkg.Version, &pkg.Chain, &pkg.Builder, &pkg.CompilerVersion, &metadataJSON, &createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	// Deserialize metadata if present
	if len(metadataJSON) > 0 && string(metadataJSON) != "{}" {
		if err := json.Unmarshal(metadataJSON, &pkg.Metadata); err != nil {
			// Log but don't fail - metadata is optional
			s.logger.Warn("failed to deserialize metadata", "error", err)
		}
	}

	pkg.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
	return &pkg, nil
}

// GetPackageVersions retrieves all versions of a package
func (s *PostgresStore) GetPackageVersions(ctx context.Context, name string, includePrerelease bool) ([]string, error) {
	query := `SELECT version FROM packages WHERE name = $1 ORDER BY created_at DESC`
	rows, err := s.db.QueryContext(ctx, query, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var versions []string
	for rows.Next() {
		var v string
		if err := rows.Scan(&v); err != nil {
			return nil, err
		}
		versions = append(versions, v)
	}
	return versions, rows.Err()
}

// ListPackages lists packages with filtering and pagination
func (s *PostgresStore) ListPackages(ctx context.Context, filter PackageFilter, pagination PaginationParams) (*PaginatedResult[Package], error) {
	// Build base query with GROUP BY to aggregate versions
	baseQuery := `
		SELECT name, chain, builder, array_to_string(array_agg(version ORDER BY created_at DESC), ',') as versions
		FROM packages
	`

	var whereClauses []string
	var args []any
	argIdx := 1

	// Add filter conditions
	if pagination.Cursor != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("name > $%d", argIdx))
		args = append(args, pagination.Cursor)
		argIdx++
	}
	if filter.Query != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("name ILIKE $%d", argIdx))
		args = append(args, "%"+filter.Query+"%")
		argIdx++
	}
	if filter.Chain != "" {
		whereClauses = append(whereClauses, fmt.Sprintf("chain = $%d", argIdx))
		args = append(args, filter.Chain)
		argIdx++
	}

	// Build final query
	query := baseQuery
	if len(whereClauses) > 0 {
		query += " WHERE " + strings.Join(whereClauses, " AND ")
	}
	query += fmt.Sprintf(" GROUP BY name, chain, builder ORDER BY name LIMIT $%d", argIdx)
	args = append(args, pagination.Limit+1)

	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var packages []Package
	for rows.Next() {
		var name, chain, builder, versionsStr string
		if err := rows.Scan(&name, &chain, &builder, &versionsStr); err != nil {
			return nil, err
		}
		var versions []string
		if versionsStr != "" {
			versions = strings.Split(versionsStr, ",")
		}
		packages = append(packages, Package{
			Name:     name,
			Chain:    chain,
			Builder:  builder,
			Versions: versions,
		})
	}

	hasMore := len(packages) > pagination.Limit
	var nextCursor string
	if hasMore {
		packages = packages[:pagination.Limit]
	}
	if len(packages) > 0 {
		nextCursor = packages[len(packages)-1].Name
	}

	return &PaginatedResult[Package]{Data: packages, HasMore: hasMore, NextCursor: nextCursor}, rows.Err()
}

// DeletePackage deletes a package
func (s *PostgresStore) DeletePackage(ctx context.Context, name, version string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM packages WHERE name = $1 AND version = $2", name, version)
	return err
}

// PackageExists checks if a package exists
func (s *PostgresStore) PackageExists(ctx context.Context, name, version string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM packages WHERE name = $1 AND version = $2", name, version).Scan(&count)
	return count > 0, err
}

// GetPackageOwner returns the owner ID of a package (first publisher)
func (s *PostgresStore) GetPackageOwner(ctx context.Context, name string) (string, error) {
	var ownerID sql.NullString
	query := `SELECT owner_key_id FROM package_owners WHERE package_name = $1`
	err := s.db.QueryRowContext(ctx, query, name).Scan(&ownerID)
	if err == sql.ErrNoRows {
		return "", nil // No owner (new package)
	}
	if err != nil {
		return "", err
	}
	return ownerID.String, nil
}

// SetPackageOwner sets the owner of a package (first-come-first-served)
func (s *PostgresStore) SetPackageOwner(ctx context.Context, name, ownerKeyID string) error {
	query := `INSERT INTO package_owners (package_name, owner_key_id) VALUES ($1, $2) ON CONFLICT (package_name) DO NOTHING`
	_, err := s.db.ExecContext(ctx, query, name, ownerKeyID)
	return err
}

// CreateContract creates a new contract
func (s *PostgresStore) CreateContract(ctx context.Context, packageID string, contract *Contract) error {
	query := `
		INSERT INTO contracts (id, package_id, name, chain, source_path, license, primary_hash, metadata_hash)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
	`
	_, err := s.db.ExecContext(ctx, query, contract.ID, packageID, contract.Name, contract.Chain, contract.SourcePath, contract.License, contract.PrimaryHash, contract.MetadataHash)
	return err
}

// GetContract retrieves a contract
func (s *PostgresStore) GetContract(ctx context.Context, packageID, contractName string) (*Contract, error) {
	query := `
		SELECT id, package_id, name, chain, source_path, license, primary_hash, metadata_hash, created_at
		FROM contracts
		WHERE package_id = $1 AND name = $2
	`
	var c Contract
	err := s.db.QueryRowContext(ctx, query, packageID, contractName).Scan(
		&c.ID, &c.PackageID, &c.Name, &c.Chain, &c.SourcePath, &c.License, &c.PrimaryHash, &c.MetadataHash, &c.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &c, err
}

// ListContracts lists all contracts in a package
func (s *PostgresStore) ListContracts(ctx context.Context, packageID string) ([]Contract, error) {
	query := `SELECT id, package_id, name, chain, source_path, license, primary_hash, metadata_hash, created_at FROM contracts WHERE package_id = $1`
	rows, err := s.db.QueryContext(ctx, query, packageID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var contracts []Contract
	for rows.Next() {
		var c Contract
		if err := rows.Scan(&c.ID, &c.PackageID, &c.Name, &c.Chain, &c.SourcePath, &c.License, &c.PrimaryHash, &c.MetadataHash, &c.CreatedAt); err != nil {
			return nil, err
		}
		contracts = append(contracts, c)
	}
	return contracts, rows.Err()
}

// StoreArtifact stores an artifact
func (s *PostgresStore) StoreArtifact(ctx context.Context, contractID, artifactType string, content []byte) error {
	hash := computeHash(content)
	query := `
		INSERT INTO artifacts (id, contract_id, artifact_type, content_hash, content, size_bytes)
		VALUES ($1, $2, $3, $4, $5, $6)
		ON CONFLICT(contract_id, artifact_type) DO UPDATE SET content = EXCLUDED.content, content_hash = EXCLUDED.content_hash, size_bytes = EXCLUDED.size_bytes
	`
	_, err := s.db.ExecContext(ctx, query, generateID(), contractID, artifactType, hash, content, len(content))
	return err
}

// GetArtifact retrieves an artifact
func (s *PostgresStore) GetArtifact(ctx context.Context, contractID, artifactType string) ([]byte, error) {
	var content []byte
	err := s.db.QueryRowContext(ctx, "SELECT content FROM artifacts WHERE contract_id = $1 AND artifact_type = $2", contractID, artifactType).Scan(&content)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return content, err
}

// GetArtifactByHash retrieves an artifact by hash
func (s *PostgresStore) GetArtifactByHash(ctx context.Context, hash string) ([]byte, error) {
	var content []byte
	err := s.db.QueryRowContext(ctx, "SELECT content FROM artifacts WHERE content_hash = $1", hash).Scan(&content)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return content, err
}

// RecordDeployment records a deployment
func (s *PostgresStore) RecordDeployment(ctx context.Context, d *Deployment) error {
	query := `
		INSERT INTO deployments (id, package_id, contract_name, chain, chain_id, address, deployer_address, tx_hash, block_number, deployment_data)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
	`
	_, err := s.db.ExecContext(ctx, query, d.ID, d.PackageID, d.ContractName, d.Chain, d.ChainID, d.Address, d.DeployerAddress, d.TxHash, d.BlockNumber, "{}")
	return err
}

// GetDeployment retrieves a deployment
func (s *PostgresStore) GetDeployment(ctx context.Context, chain, chainID, address string) (*Deployment, error) {
	query := `
		SELECT id, package_id, contract_name, chain, chain_id, address, deployer_address, tx_hash, block_number, verified, created_at
		FROM deployments
		WHERE chain = $1 AND chain_id = $2 AND address = $3
	`
	var d Deployment
	var createdAt time.Time
	err := s.db.QueryRowContext(ctx, query, chain, chainID, address).Scan(
		&d.ID, &d.PackageID, &d.ContractName, &d.Chain, &d.ChainID, &d.Address, &d.DeployerAddress, &d.TxHash, &d.BlockNumber, &d.Verified, &createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err == nil {
		d.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
	}
	return &d, err
}

// ListDeployments lists deployments
func (s *PostgresStore) ListDeployments(ctx context.Context, filter DeploymentFilter, pagination PaginationParams) (*PaginatedResult[Deployment], error) {
	query := `SELECT id, package_id, contract_name, chain, chain_id, address, verified, created_at FROM deployments ORDER BY created_at DESC LIMIT $1`
	rows, err := s.db.QueryContext(ctx, query, pagination.Limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []Deployment
	for rows.Next() {
		var d Deployment
		var createdAt time.Time
		if err := rows.Scan(&d.ID, &d.PackageID, &d.ContractName, &d.Chain, &d.ChainID, &d.Address, &d.Verified, &createdAt); err != nil {
			return nil, err
		}
		d.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
		deployments = append(deployments, d)
	}

	hasMore := len(deployments) > pagination.Limit
	if hasMore {
		deployments = deployments[:pagination.Limit]
	}

	return &PaginatedResult[Deployment]{Data: deployments, HasMore: hasMore}, rows.Err()
}

// UpdateVerificationStatus updates a deployment's verification status
func (s *PostgresStore) UpdateVerificationStatus(ctx context.Context, id string, verified bool, verifiedOn []string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE deployments SET verified = $1, verified_at = NOW() WHERE id = $2", verified, id)
	return err
}

// CreateAPIKey creates a new API key
func (s *PostgresStore) CreateAPIKey(ctx context.Context, name string) (string, error) {
	key := generateAPIKey()
	hash := hashAPIKey(key)
	id := generateID()
	_, err := s.db.ExecContext(ctx, "INSERT INTO api_keys (id, key_hash, name) VALUES ($1, $2, $3)", id, hash, name)
	if err != nil {
		return "", err
	}
	return key, nil
}

// ValidateAPIKey validates an API key
func (s *PostgresStore) ValidateAPIKey(ctx context.Context, key string) (*APIKey, error) {
	hash := hashAPIKey(key)
	var ak APIKey
	var createdAt time.Time
	err := s.db.QueryRowContext(ctx, "SELECT id, key_hash, name, created_at FROM api_keys WHERE key_hash = $1 AND revoked_at IS NULL", hash).Scan(
		&ak.ID, &ak.KeyHash, &ak.Name, &createdAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err == nil {
		ak.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
	}
	// Update last used
	_, _ = s.db.ExecContext(ctx, "UPDATE api_keys SET last_used_at = NOW() WHERE id = $1", ak.ID)
	return &ak, err
}

// ListAPIKeys lists all API keys
func (s *PostgresStore) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, created_at, last_used_at FROM api_keys WHERE revoked_at IS NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		var createdAt time.Time
		var lastUsed sql.NullTime
		if err := rows.Scan(&k.ID, &k.Name, &createdAt, &lastUsed); err != nil {
			return nil, err
		}
		k.CreatedAt = createdAt.Format("2006-01-02 15:04:05")
		if lastUsed.Valid {
			k.LastUsedAt = lastUsed.Time.Format("2006-01-02 15:04:05")
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// RevokeAPIKey revokes an API key
func (s *PostgresStore) RevokeAPIKey(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE api_keys SET revoked_at = NOW() WHERE id = $1", id)
	return err
}
