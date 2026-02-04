# Build stage
FROM golang:1.22-alpine AS builder

RUN apk add --no-cache git ca-certificates

WORKDIR /app

# Copy go mod files first for better caching
COPY go.mod go.sum ./
RUN go mod download

# Copy source code
COPY . .

# Build both binaries
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/contrafactory ./cmd/contrafactory
RUN CGO_ENABLED=0 GOOS=linux go build -ldflags="-w -s" -o /bin/contrafactory-server ./cmd/contrafactory-server

# Final stage
FROM alpine:3.20

RUN apk add --no-cache ca-certificates tzdata

# Create non-root user
RUN addgroup -S contrafactory && adduser -S contrafactory -G contrafactory

WORKDIR /app

# Copy binaries from builder
COPY --from=builder /bin/contrafactory /usr/local/bin/contrafactory
COPY --from=builder /bin/contrafactory-server /usr/local/bin/contrafactory-server

# Create data directory
RUN mkdir -p /app/data && chown -R contrafactory:contrafactory /app

USER contrafactory

# Default environment variables
ENV PORT=8080
ENV HOST=0.0.0.0
ENV LOG_LEVEL=info
ENV LOG_FORMAT=json
ENV STORAGE_TYPE=sqlite
ENV SQLITE_PATH=/app/data/contrafactory.db

EXPOSE 8080

# Health check
HEALTHCHECK --interval=30s --timeout=3s --start-period=5s --retries=3 \
    CMD wget --no-verbose --tries=1 --spider http://localhost:8080/health || exit 1

ENTRYPOINT ["contrafactory-server"]
