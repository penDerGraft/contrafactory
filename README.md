# Contrafactory

A self-hosted artifact registry for smart contracts. Store versioned ABIs, bytecode, and verification metadata. Fetch them later.

```bash
# Publish after building
forge build --build-info
contrafactory publish --version 1.0.0

# Fetch from anywhere
contrafactory fetch my-token@1.0.0 --only abi
```

## Why Would I Want This?

You might not. Etherscan stores verified source code. NPM can hold ABIs. GitHub releases work for some teams. These are fine solutions.

Contrafactory is mostly about separating build and deploy into discrete steps.

Some contract workflows look like: build → deploy → verify → hope you saved the right files. The build artifacts live in a local `out/` folder until deployment, then scatter to block explorers, deployment logs, and wherever else you remember to put them.

Contrafactory flips this: build → publish → deploy whenever. Your artifacts exist independently of any deployment. This is a standard DevOps pattern that unlocks several key workflows:

- **Rollbacks** — Deploy a previous version without recreating the build environment
- **Version pinning** — CI can block deploys of versions with known issues
- **Minimal deploy scripts** — Just reference a version, no build step required
- **Consistent verification** — Same metadata for Etherscan, Sourcify, or any explorer
- **Audit snapshots** — Tag and store the exact bytecode that was audited

The core idea: your contract artifacts should be a versioned, immutable record that exists before deployment and persists after.

## What It Stores

For each contract version:

- ABI
- Bytecode and deployed bytecode
- Standard JSON Input (for block explorer verification)
- Compiler version and settings
- Source file mappings

Everything a block explorer needs, captured at build time.

## Quick Start

**Run the server:**

```bash
docker run -p 8080:8080 ghcr.io/pendergraft/contrafactory:latest
```

Or with SQLite locally:

```bash
./contrafactory-server serve
```

**Publish contracts:**

```bash
cd my-contracts
forge build --build-info
contrafactory publish --version 1.0.0
```

**Fetch artifacts:**

```bash
# Everything
contrafactory fetch my-token@1.0.0

# Just the ABI
contrafactory fetch my-token@1.0.0 --only abi
```

**Track deployments:**

```bash
contrafactory deployment record \
  --package my-token@1.0.0 \
  --chain-id 1 \
  --address 0x1234...
```

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `STORAGE_TYPE` | `sqlite` | `sqlite` or `postgres` |
| `AUTH_TYPE` | `none` | `none` or `api-key` |
| `PORT` | `8080` | Server port |

Reads are unauthenticated. Writes require an API key when `AUTH_TYPE=api-key`.

## Toolchain Support

- **Foundry** — Supported (requires `forge build --build-info`)
- **Hardhat** — Planned
- **Anchor (Solana)** — Planned

## Production Deployment

### Docker Compose

For small teams or single-server deployments:

```bash
curl -O https://raw.githubusercontent.com/pendergraft/contrafactory/main/docker-compose.yml
docker compose up -d
```

The default compose file uses SQLite. For Postgres:

```yaml
services:
  server:
    image: ghcr.io/pendergraft/contrafactory:latest
    environment:
      STORAGE_TYPE: postgres
      DATABASE_URL: postgres://contrafactory:secret@postgres:5432/contrafactory
      AUTH_TYPE: api-key
    ports:
      - "8080:8080"
    depends_on:
      - postgres

  postgres:
    image: postgres:16
    environment:
      POSTGRES_USER: contrafactory
      POSTGRES_PASSWORD: secret
      POSTGRES_DB: contrafactory
    volumes:
      - postgres_data:/var/lib/postgresql/data

volumes:
  postgres_data:
```

### Kubernetes with Helm

```bash
# Basic install with SQLite (ephemeral, for testing)
helm install contrafactory oci://ghcr.io/pendergraft/charts/contrafactory

# Production install with Postgres
helm install contrafactory oci://ghcr.io/pendergraft/charts/contrafactory \
  --set storage.type=postgres \
  --set postgresql.enabled=true \
  --set auth.type=api-key
```

With an external Postgres database:

```bash
helm install contrafactory oci://ghcr.io/pendergraft/charts/contrafactory \
  --set storage.type=postgres \
  --set postgresql.enabled=false \
  --set externalDatabase.host=your-postgres-host \
  --set externalDatabase.database=contrafactory \
  --set externalDatabase.user=contrafactory \
  --set externalDatabase.password=your-password \
  --set auth.type=api-key
```

Common Helm values:

| Value | Default | Description |
|-------|---------|-------------|
| `storage.type` | `sqlite` | `sqlite` or `postgres` |
| `auth.type` | `none` | `none` or `api-key` |
| `postgresql.enabled` | `false` | Deploy Postgres as a subchart |
| `ingress.enabled` | `false` | Create an Ingress resource |
| `ingress.hosts[0].host` | — | Hostname for ingress |
| `persistence.enabled` | `true` | Persistent volume for SQLite |
| `persistence.size` | `10Gi` | Volume size |

### Binary

For bare-metal or VM deployments:

```bash
# Download
curl -L https://github.com/pendergraft/contrafactory/releases/latest/download/contrafactory-server-linux-amd64 \
  -o /usr/local/bin/contrafactory-server
chmod +x /usr/local/bin/contrafactory-server

# Run with Postgres
export STORAGE_TYPE=postgres
export DATABASE_URL="postgres://user:pass@localhost:5432/contrafactory"
export AUTH_TYPE=api-key
contrafactory-server serve
```

Create an API key for CI/CD:

```bash
contrafactory-server keys create --name "github-actions" --show
```

### Storage Recommendations

| Use Case | Storage | Notes |
|----------|---------|-------|
| Local dev | SQLite | Zero config, single file |
| Small team | SQLite + persistent volume | Simple, sufficient for most |
| Production | Postgres | Concurrent writes, backups, replication |


## License

Apache 2.0
