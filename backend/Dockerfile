# ── Stage 1: Builder ──────────────────────────────────────────
FROM golang:1.22-alpine AS builder

# install build dependencies
RUN apk add --no-cache git ca-certificates tzdata

WORKDIR /app

# copy dependency files first (layer cache optimization)
COPY go.mod go.sum ./
RUN go mod download

# copy source code
COPY . .

# build the binary
RUN CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build \
    -ldflags="-w -s -X main.version=$(git describe --tags --always --dirty 2>/dev/null || echo dev)" \
    -o souqiq_api \
    ./main.go

# ── Stage 2: Runtime ──────────────────────────────────────────
FROM alpine:3.19

# install runtime dependencies
RUN apk add --no-cache \
    ca-certificates \
    tzdata \
    curl

# set timezone
ENV TZ=Africa/Tripoli

# create non-root user for security
RUN addgroup -g 1001 -S souqiq && \
    adduser  -u 1001 -S souqiq -G souqiq

WORKDIR /app

# copy binary from builder
COPY --from=builder /app/souqiq_api .

# copy migrations (needed for DB init at startup)
COPY --from=builder /app/migrations ./migrations

# set ownership
RUN chown -R souqiq:souqiq /app

# switch to non-root user
USER souqiq

# expose API port
EXPOSE 8080

# health check
HEALTHCHECK --interval=15s --timeout=5s --start-period=10s --retries=3 \
    CMD curl -f http://localhost:8080/health || exit 1

# run
ENTRYPOINT ["./souqiq_api"]