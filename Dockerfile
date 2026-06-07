# syntax=docker/dockerfile:1

# ---------- builder ----------
FROM golang:1.26-alpine AS builder
WORKDIR /src

# Warm the module cache as its own layer.
COPY go.mod go.sum ./
RUN go mod download

# Build the server and the migrate helper. lib/pq is pure Go, so CGO stays off
# and the binaries are fully static (run on a minimal base, no libc needed).
COPY . .
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/feedium ./cmd/feedium \
 && go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate

# ---------- runtime ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 -h /app feedium
WORKDIR /app

COPY --from=builder /out/feedium /usr/local/bin/feedium
COPY --from=builder /out/migrate /usr/local/bin/migrate
COPY migrations /app/migrations

USER feedium
EXPOSE 8000 9000

# Default to the server; the migrate step overrides command (see
# docker-compose.prod.yml). The config is mounted read-only at runtime.
CMD ["feedium", "-conf", "/etc/feedium/config.yaml"]
