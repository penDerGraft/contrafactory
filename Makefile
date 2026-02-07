.PHONY: dev dev-postgres build build-all cli server test test-v test-cover test-int test-e2e lint fmt migrate migrate-down migrate-new docker seed clean help

# Build variables
BINARY_CLI=contrafactory
BINARY_SERVER=contrafactory-server
VERSION ?= $(shell git describe --tags --always --dirty 2>/dev/null || echo "dev")
LDFLAGS=-ldflags "-X main.version=$(VERSION)"
GOOS ?= $(shell go env GOOS)
GOARCH ?= $(shell go env GOARCH)

# Default target
.DEFAULT_GOAL := help

## Development

dev: ## Run server with hot-reload (air) and SQLite
	@if command -v air > /dev/null; then \
		air; \
	else \
		echo "Installing air..."; \
		go install github.com/air-verse/air@latest; \
		air; \
	fi

dev-postgres: ## Run server with local Postgres via Docker
	docker compose up -d postgres
	DATABASE_URL=postgres://contrafactory:contrafactory@localhost:5432/contrafactory?sslmode=disable go run ./cmd/contrafactory-server

## Building

cli: ## Build CLI binary to ./bin/contrafactory
	go build $(LDFLAGS) -o bin/$(BINARY_CLI) ./cmd/contrafactory

server: ## Build server binary to ./bin/contrafactory-server
	go build $(LDFLAGS) -o bin/$(BINARY_SERVER) ./cmd/contrafactory-server

build: cli server ## Build both binaries

install: build ## Install binaries to /usr/local/bin
	@echo "Installing to /usr/local/bin (may require sudo)..."
	sudo cp bin/$(BINARY_CLI) /usr/local/bin/
	sudo cp bin/$(BINARY_SERVER) /usr/local/bin/
	@echo "Installed: contrafactory, contrafactory-server"

install-local: build ## Install binaries to ~/go/bin (no sudo needed)
	@mkdir -p ~/go/bin
	cp bin/$(BINARY_CLI) ~/go/bin/
	cp bin/$(BINARY_SERVER) ~/go/bin/
	@echo "Installed to ~/go/bin"
	@echo "Make sure ~/go/bin is in your PATH"

build-all: ## Cross-compile for all platforms
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_CLI)-linux-amd64 ./cmd/contrafactory
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_CLI)-linux-arm64 ./cmd/contrafactory
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_CLI)-darwin-amd64 ./cmd/contrafactory
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_CLI)-darwin-arm64 ./cmd/contrafactory
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_CLI)-windows-amd64.exe ./cmd/contrafactory
	GOOS=linux GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_SERVER)-linux-amd64 ./cmd/contrafactory-server
	GOOS=linux GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_SERVER)-linux-arm64 ./cmd/contrafactory-server
	GOOS=darwin GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_SERVER)-darwin-amd64 ./cmd/contrafactory-server
	GOOS=darwin GOARCH=arm64 go build $(LDFLAGS) -o bin/$(BINARY_SERVER)-darwin-arm64 ./cmd/contrafactory-server
	GOOS=windows GOARCH=amd64 go build $(LDFLAGS) -o bin/$(BINARY_SERVER)-windows-amd64.exe ./cmd/contrafactory-server

## Testing

test: ## Run all unit tests
	go test ./...

test-v: ## Run unit tests with verbose output
	go test -v ./...

test-cover: ## Run tests with coverage report
	go test -coverprofile=coverage.out ./...
	go tool cover -html=coverage.out -o coverage.html
	@echo "Coverage report: coverage.html"

test-int: ## Run integration tests (requires Docker)
	go test -tags=integration -v ./...

test-e2e: ## Run E2E tests with testcontainers (requires Docker)
	go test -tags=e2e -v -timeout 10m ./test/e2e/

## Linting & Formatting

lint: ## Run golangci-lint
	@if command -v golangci-lint > /dev/null; then \
		golangci-lint run; \
	else \
		echo "Installing golangci-lint..."; \
		go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest; \
		golangci-lint run; \
	fi

fmt: ## Format code with gofmt
	gofmt -s -w .

## Database

migrate: ## Run database migrations
	go run ./cmd/contrafactory-server migrate up

migrate-down: ## Rollback last migration
	go run ./cmd/contrafactory-server migrate down

migrate-new: ## Create a new migration file (usage: make migrate-new NAME=create_users)
	@if [ -z "$(NAME)" ]; then echo "Usage: make migrate-new NAME=migration_name"; exit 1; fi
	migrate create -ext sql -dir internal/storage/migrations -seq $(NAME)

## Docker

docker: ## Build Docker image locally
	docker build -t contrafactory:$(VERSION) .

docker-push: docker ## Build and push Docker image
	docker tag contrafactory:$(VERSION) ghcr.io/pendergraft/contrafactory:$(VERSION)
	docker push ghcr.io/pendergraft/contrafactory:$(VERSION)

## Utilities

seed: build ## Seed the database with sample packages
	./scripts/seed.sh

clean: ## Remove build artifacts
	rm -rf bin/ tmp/ coverage.out coverage.html data/

deps: ## Download and verify dependencies
	go mod download
	go mod verify

tidy: ## Tidy go.mod and go.sum
	go mod tidy

## Help

help: ## Show this help
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | sort | awk 'BEGIN {FS = ":.*?## "}; {printf "\033[36m%-20s\033[0m %s\n", $$1, $$2}'
