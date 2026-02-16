package storage

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"

	_ "modernc.org/sqlite"
)

// SQLiteStore implements Store using SQLite
type SQLiteStore struct {
	db     *sql.DB
	logger *slog.Logger
}

// NewSQLiteStore creates a new SQLite store
func NewSQLiteStore(path string, logger *slog.Logger) (*SQLiteStore, error) {
	// Ensure directory exists
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating data directory: %w", err)
	}

	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Enable WAL mode for better concurrency
	if _, err := db.Exec("PRAGMA journal_mode=WAL"); err != nil {
		return nil, fmt.Errorf("enabling WAL mode: %w", err)
	}

	// Enable foreign keys
	if _, err := db.Exec("PRAGMA foreign_keys=ON"); err != nil {
		return nil, fmt.Errorf("enabling foreign keys: %w", err)
	}

	return &SQLiteStore{db: db, logger: logger}, nil
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// Migrate runs database migrations
func (s *SQLiteStore) Migrate(ctx context.Context) error {
	schema := `
	-- Package ownership
	CREATE TABLE IF NOT EXISTS package_owners (
		id TEXT PRIMARY KEY,
		package_name TEXT NOT NULL UNIQUE,
		owner_key_id TEXT REFERENCES api_keys(id),
		created_at TEXT DEFAULT (datetime('now'))
	);

	-- Packages
	CREATE TABLE IF NOT EXISTS packages (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		version TEXT NOT NULL,
		project TEXT,
		chain TEXT NOT NULL,
		builder TEXT,
		compiler_version TEXT,
		compiler_settings TEXT,
		metadata TEXT,
		created_at TEXT DEFAULT (datetime('now')),
		UNIQUE(name, version)
	);

	-- Contracts
	CREATE TABLE IF NOT EXISTS contracts (
		id TEXT PRIMARY KEY,
		package_id TEXT REFERENCES packages(id) ON DELETE CASCADE,
		name TEXT NOT NULL,
		chain TEXT NOT NULL,
		source_path TEXT NOT NULL,
		license TEXT,
		primary_hash TEXT NOT NULL,
		metadata_hash TEXT,
		created_at TEXT DEFAULT (datetime('now')),
		UNIQUE(package_id, name, source_path)
	);

	-- Artifacts
	CREATE TABLE IF NOT EXISTS artifacts (
		id TEXT PRIMARY KEY,
		contract_id TEXT REFERENCES contracts(id) ON DELETE CASCADE,
		artifact_type TEXT NOT NULL,
		content_hash TEXT NOT NULL,
		content BLOB,
		blob_store_ref TEXT,
		size_bytes INTEGER NOT NULL,
		UNIQUE(contract_id, artifact_type)
	);

	-- Deployments
	CREATE TABLE IF NOT EXISTS deployments (
		id TEXT PRIMARY KEY,
		package_id TEXT REFERENCES packages(id),
		contract_name TEXT NOT NULL,
		chain TEXT NOT NULL,
		chain_id TEXT NOT NULL,
		address TEXT NOT NULL,
		deployer_address TEXT,
		tx_hash TEXT,
		block_number INTEGER,
		deployment_data TEXT,
		verified INTEGER DEFAULT 0,
		verified_at TEXT,
		verified_on TEXT,
		created_at TEXT DEFAULT (datetime('now')),
		UNIQUE(chain, chain_id, address)
	);

	-- API keys
	CREATE TABLE IF NOT EXISTS api_keys (
		id TEXT PRIMARY KEY,
		key_hash TEXT NOT NULL UNIQUE,
		name TEXT NOT NULL,
		scopes TEXT,
		created_at TEXT DEFAULT (datetime('now')),
		last_used_at TEXT,
		revoked_at TEXT
	);

	-- Blobs
	CREATE TABLE IF NOT EXISTS blobs (
		hash TEXT PRIMARY KEY,
		content BLOB NOT NULL,
		size_bytes INTEGER NOT NULL,
		created_at TEXT DEFAULT (datetime('now'))
	);

	-- Indexes
	CREATE INDEX IF NOT EXISTS idx_packages_name ON packages(name);
	CREATE INDEX IF NOT EXISTS idx_packages_chain ON packages(chain);
	CREATE INDEX IF NOT EXISTS idx_contracts_primary_hash ON contracts(primary_hash);
	CREATE INDEX IF NOT EXISTS idx_deployments_lookup ON deployments(chain, chain_id, address);
	CREATE INDEX IF NOT EXISTS idx_artifacts_content_hash ON artifacts(content_hash);
	`

	_, err := s.db.ExecContext(ctx, schema)
	if err != nil {
		return fmt.Errorf("running migrations: %w", err)
	}

	// Add project column if it doesn't exist (SQLite doesn't support IF NOT EXISTS for ADD COLUMN)
	// Ignore error if column already exists (duplicate column name)
	if _, err := s.db.ExecContext(ctx, "ALTER TABLE packages ADD COLUMN project TEXT"); err != nil {
		if !strings.Contains(err.Error(), "duplicate column name") {
			s.logger.Warn("adding project column (may already exist)", "error", err)
		}
	}

	s.logger.Info("database migrations complete")
	return nil
}

// CreatePackage creates a new package
func (s *SQLiteStore) CreatePackage(ctx context.Context, pkg *Package) error {
	// Serialize metadata as JSON
	var metadataJSON string
	if len(pkg.Metadata) > 0 {
		data, err := json.Marshal(pkg.Metadata)
		if err != nil {
			return fmt.Errorf("serializing metadata: %w", err)
		}
		metadataJSON = string(data)
	} else {
		metadataJSON = "{}"
	}

	// Serialize compiler settings as JSON
	compilerSettingsJSON := "{}"
	if len(pkg.CompilerSettings) > 0 {
		data, err := json.Marshal(pkg.CompilerSettings)
		if err != nil {
			return fmt.Errorf("serializing compiler settings: %w", err)
		}
		compilerSettingsJSON = string(data)
	}

	query := `
		INSERT INTO packages (id, name, version, project, chain, builder, compiler_version, compiler_settings, metadata, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`
	_, err := s.db.ExecContext(ctx, query, pkg.ID, pkg.Name, pkg.Version, nullIfEmpty(pkg.Project), pkg.Chain, pkg.Builder, pkg.CompilerVersion, compilerSettingsJSON, metadataJSON)
	return err
}

// GetPackage retrieves a package by name and version
func (s *SQLiteStore) GetPackage(ctx context.Context, name, version string) (*Package, error) {
	query := `
		SELECT id, name, version, project, chain, builder, compiler_version, compiler_settings, metadata, created_at
		FROM packages
		WHERE name = ? AND version = ?
	`
	var pkg Package
	var project sql.NullString
	var settings string
	var metadata sql.NullString
	err := s.db.QueryRowContext(ctx, query, name, version).Scan(
		&pkg.ID, &pkg.Name, &pkg.Version, &project, &pkg.Chain, &pkg.Builder, &pkg.CompilerVersion, &settings, &metadata, &pkg.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}

	if project.Valid {
		pkg.Project = project.String
	}

	// Deserialize compiler settings if present
	if settings != "" && settings != "{}" {
		if err := json.Unmarshal([]byte(settings), &pkg.CompilerSettings); err != nil {
			s.logger.Warn("failed to deserialize compiler settings", "error", err)
		}
	}

	// Deserialize metadata if present
	if metadata.Valid && metadata.String != "" && metadata.String != "{}" {
		if err := json.Unmarshal([]byte(metadata.String), &pkg.Metadata); err != nil {
			// Log but don't fail - metadata is optional
			s.logger.Warn("failed to deserialize metadata", "error", err)
		}
	}

	return &pkg, nil
}

// GetPackageVersions retrieves all versions of a package
func (s *SQLiteStore) GetPackageVersions(ctx context.Context, name string, includePrerelease bool) ([]string, error) {
	query := `SELECT version FROM packages WHERE name = ? ORDER BY created_at DESC`
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

// ListPackages lists packages with filtering and cursor-based pagination
func (s *SQLiteStore) ListPackages(ctx context.Context, filter PackageFilter, pagination PaginationParams) (*PaginatedResult[Package], error) {
	var whereClauses []string
	var args []any
	argIdx := 0
	addArg := func(v any) {
		argIdx++
		args = append(args, v)
	}

	tablePrefix := ""
	baseQuery := `
		SELECT name, chain, builder, GROUP_CONCAT(version, ',') as versions
		FROM packages
	`
	if filter.Contract != "" {
		tablePrefix = "p."
		baseQuery = `
		SELECT p.name, p.chain, p.builder, GROUP_CONCAT(p.version, ',') as versions
		FROM packages p
		INNER JOIN contracts c ON c.package_id = p.id AND LOWER(c.name) = LOWER(?)
		`
		addArg(filter.Contract)
	}

	whereClauses = buildListPackagesWhereClauses(&args, &argIdx, filter, pagination, tablePrefix)
	if len(whereClauses) > 0 {
		baseQuery += " WHERE " + strings.Join(whereClauses, " AND ")
	}
	baseQuery += " GROUP BY " + tablePrefix + "name, " + tablePrefix + "chain, " + tablePrefix + "builder ORDER BY " + tablePrefix + "name LIMIT ?"
	addArg(pagination.Limit + 1)

	rows, err := s.db.QueryContext(ctx, baseQuery, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var packages []Package
	for rows.Next() {
		var name, chain, builder, versions string
		if err := rows.Scan(&name, &chain, &builder, &versions); err != nil {
			return nil, err
		}
		var versionList []string
		if versions != "" {
			versionList = strings.Split(versions, ",")
		}
		// Apply latest filter: keep only the latest version by semver
		if filter.Latest && filter.Project != "" && len(versionList) > 1 {
			latest := latestVersionBySemver(versionList)
			versionList = []string{latest}
		}
		packages = append(packages, Package{
			Name:     name,
			Chain:    chain,
			Builder:  builder,
			Versions: versionList,
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

	return &PaginatedResult[Package]{
		Data:       packages,
		HasMore:    hasMore,
		NextCursor: nextCursor,
	}, rows.Err()
}

// buildListPackagesWhereClauses builds WHERE clauses for ListPackages (SQLite uses ? placeholders)
func buildListPackagesWhereClauses(args *[]any, argIdx *int, filter PackageFilter, pagination PaginationParams, tablePrefix string) []string {
	var whereClauses []string
	addArg := func(v any) {
		*argIdx++
		*args = append(*args, v)
	}

	if pagination.Cursor != "" {
		whereClauses = append(whereClauses, tablePrefix+"name > ?")
		addArg(pagination.Cursor)
	}
	if filter.Query != "" {
		whereClauses = append(whereClauses, tablePrefix+"name LIKE ?")
		addArg("%" + filter.Query + "%")
	}
	if filter.Chain != "" {
		whereClauses = append(whereClauses, tablePrefix+"chain = ?")
		addArg(filter.Chain)
	}
	if filter.Project != "" {
		whereClauses = append(whereClauses, tablePrefix+"project = ?")
		addArg(filter.Project)
	}
	if filter.Version != "" {
		whereClauses = append(whereClauses, tablePrefix+"version = ?")
		addArg(filter.Version)
	}
	return whereClauses
}

// DeletePackage deletes a package
func (s *SQLiteStore) DeletePackage(ctx context.Context, name, version string) error {
	_, err := s.db.ExecContext(ctx, "DELETE FROM packages WHERE name = ? AND version = ?", name, version)
	return err
}

// PackageExists checks if a package exists
func (s *SQLiteStore) PackageExists(ctx context.Context, name, version string) (bool, error) {
	var count int
	err := s.db.QueryRowContext(ctx, "SELECT COUNT(*) FROM packages WHERE name = ? AND version = ?", name, version).Scan(&count)
	return count > 0, err
}

// GetPackageOwner returns the owner ID of a package (first publisher)
func (s *SQLiteStore) GetPackageOwner(ctx context.Context, name string) (string, error) {
	var ownerID sql.NullString
	query := `SELECT owner_key_id FROM package_owners WHERE package_name = ?`
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
func (s *SQLiteStore) SetPackageOwner(ctx context.Context, name, ownerKeyID string) error {
	query := `INSERT OR IGNORE INTO package_owners (id, package_name, owner_key_id) VALUES (?, ?, ?)`
	_, err := s.db.ExecContext(ctx, query, generateID(), name, ownerKeyID)
	return err
}

// CreateContract creates a new contract
func (s *SQLiteStore) CreateContract(ctx context.Context, packageID string, contract *Contract) error {
	query := `
		INSERT INTO contracts (id, package_id, name, chain, source_path, license, primary_hash, metadata_hash, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`
	_, err := s.db.ExecContext(ctx, query, contract.ID, packageID, contract.Name, contract.Chain, contract.SourcePath, contract.License, contract.PrimaryHash, contract.MetadataHash)
	return err
}

// GetContract retrieves a contract
func (s *SQLiteStore) GetContract(ctx context.Context, packageID, contractName string) (*Contract, error) {
	query := `
		SELECT id, package_id, name, chain, source_path, license, primary_hash, metadata_hash, created_at
		FROM contracts
		WHERE package_id = ? AND name = ?
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
func (s *SQLiteStore) ListContracts(ctx context.Context, packageID string) ([]Contract, error) {
	query := `SELECT id, package_id, name, chain, source_path, license, primary_hash, metadata_hash, created_at FROM contracts WHERE package_id = ?`
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
func (s *SQLiteStore) StoreArtifact(ctx context.Context, contractID, artifactType string, content []byte) error {
	hash := computeHash(content)
	query := `
		INSERT INTO artifacts (id, contract_id, artifact_type, content_hash, content, size_bytes)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(contract_id, artifact_type) DO UPDATE SET content = excluded.content, content_hash = excluded.content_hash, size_bytes = excluded.size_bytes
	`
	_, err := s.db.ExecContext(ctx, query, generateID(), contractID, artifactType, hash, content, len(content))
	return err
}

// GetArtifact retrieves an artifact
func (s *SQLiteStore) GetArtifact(ctx context.Context, contractID, artifactType string) ([]byte, error) {
	var content []byte
	err := s.db.QueryRowContext(ctx, "SELECT content FROM artifacts WHERE contract_id = ? AND artifact_type = ?", contractID, artifactType).Scan(&content)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return content, err
}

// GetArtifactByHash retrieves an artifact by hash
func (s *SQLiteStore) GetArtifactByHash(ctx context.Context, hash string) ([]byte, error) {
	var content []byte
	err := s.db.QueryRowContext(ctx, "SELECT content FROM artifacts WHERE content_hash = ?", hash).Scan(&content)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return content, err
}

// RecordDeployment records a deployment
func (s *SQLiteStore) RecordDeployment(ctx context.Context, d *Deployment) error {
	query := `
		INSERT INTO deployments (id, package_id, contract_name, chain, chain_id, address, deployer_address, tx_hash, block_number, deployment_data, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, datetime('now'))
	`
	_, err := s.db.ExecContext(ctx, query, d.ID, d.PackageID, d.ContractName, d.Chain, d.ChainID, d.Address, d.DeployerAddress, d.TxHash, d.BlockNumber, "{}")
	return err
}

// GetDeployment retrieves a deployment
func (s *SQLiteStore) GetDeployment(ctx context.Context, chain, chainID, address string) (*Deployment, error) {
	query := `
		SELECT id, package_id, contract_name, chain, chain_id, address, deployer_address, tx_hash, block_number, verified, created_at
		FROM deployments
		WHERE chain = ? AND chain_id = ? AND address = ?
	`
	var d Deployment
	err := s.db.QueryRowContext(ctx, query, chain, chainID, address).Scan(
		&d.ID, &d.PackageID, &d.ContractName, &d.Chain, &d.ChainID, &d.Address, &d.DeployerAddress, &d.TxHash, &d.BlockNumber, &d.Verified, &d.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	return &d, err
}

// ListDeployments lists deployments
func (s *SQLiteStore) ListDeployments(ctx context.Context, filter DeploymentFilter, pagination PaginationParams) (*PaginatedResult[Deployment], error) {
	query := `SELECT id, package_id, contract_name, chain, chain_id, address, verified, created_at FROM deployments ORDER BY created_at DESC LIMIT ?`
	rows, err := s.db.QueryContext(ctx, query, pagination.Limit+1)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var deployments []Deployment
	for rows.Next() {
		var d Deployment
		if err := rows.Scan(&d.ID, &d.PackageID, &d.ContractName, &d.Chain, &d.ChainID, &d.Address, &d.Verified, &d.CreatedAt); err != nil {
			return nil, err
		}
		deployments = append(deployments, d)
	}

	hasMore := len(deployments) > pagination.Limit
	if hasMore {
		deployments = deployments[:pagination.Limit]
	}

	return &PaginatedResult[Deployment]{Data: deployments, HasMore: hasMore}, rows.Err()
}

// UpdateVerificationStatus updates a deployment's verification status
func (s *SQLiteStore) UpdateVerificationStatus(ctx context.Context, id string, verified bool, verifiedOn []string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE deployments SET verified = ?, verified_at = datetime('now') WHERE id = ?", verified, id)
	return err
}

// CreateAPIKey creates a new API key
func (s *SQLiteStore) CreateAPIKey(ctx context.Context, name string) (string, error) {
	key := generateAPIKey()
	hash := hashAPIKey(key)
	id := generateID()
	_, err := s.db.ExecContext(ctx, "INSERT INTO api_keys (id, key_hash, name, created_at) VALUES (?, ?, ?, datetime('now'))", id, hash, name)
	if err != nil {
		return "", err
	}
	return key, nil
}

// ValidateAPIKey validates an API key
func (s *SQLiteStore) ValidateAPIKey(ctx context.Context, key string) (*APIKey, error) {
	hash := hashAPIKey(key)
	var ak APIKey
	err := s.db.QueryRowContext(ctx, "SELECT id, key_hash, name, created_at FROM api_keys WHERE key_hash = ? AND revoked_at IS NULL", hash).Scan(
		&ak.ID, &ak.KeyHash, &ak.Name, &ak.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, ErrNotFound
	}
	// Update last used
	_, _ = s.db.ExecContext(ctx, "UPDATE api_keys SET last_used_at = datetime('now') WHERE id = ?", ak.ID)
	return &ak, err
}

// ListAPIKeys lists all API keys
func (s *SQLiteStore) ListAPIKeys(ctx context.Context) ([]APIKey, error) {
	rows, err := s.db.QueryContext(ctx, "SELECT id, name, created_at, last_used_at FROM api_keys WHERE revoked_at IS NULL")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var keys []APIKey
	for rows.Next() {
		var k APIKey
		var lastUsed sql.NullString
		if err := rows.Scan(&k.ID, &k.Name, &k.CreatedAt, &lastUsed); err != nil {
			return nil, err
		}
		if lastUsed.Valid {
			k.LastUsedAt = lastUsed.String
		}
		keys = append(keys, k)
	}
	return keys, rows.Err()
}

// RevokeAPIKey revokes an API key
func (s *SQLiteStore) RevokeAPIKey(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, "UPDATE api_keys SET revoked_at = datetime('now') WHERE id = ?", id)
	return err
}
