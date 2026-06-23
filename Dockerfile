# syntax=docker/dockerfile:1

# ---------- ui builder ----------
FROM node:22-alpine AS ui-builder
WORKDIR /src/ui
# Install deps in a separate layer for cache efficiency.
COPY ui/package.json ui/package-lock.json ./
RUN npm ci
COPY ui/ ./
RUN npm run build

# ---------- builder ----------
FROM golang:1.26-alpine AS builder
WORKDIR /src

# Warm the module cache as its own layer.
COPY go.mod go.sum ./
RUN go mod download

# Build the server, the migrate helper and the one-shot tg-login tool. lib/pq is
# pure Go, so CGO stays off and the binaries are fully static (run on a minimal
# base, no libc needed). tg-login shares the image so it can be run once against
# the same config + session volume to provision the userbot session (AG-02).
COPY . .
# Copy compiled SPA from the ui-builder stage (required by //go:embed ui/dist).
COPY --from=ui-builder /src/ui/dist ./ui/dist
ENV CGO_ENABLED=0 GOOS=linux
RUN go build -trimpath -ldflags="-s -w" -o /out/feedium ./cmd/feedium \
 && go build -trimpath -ldflags="-s -w" -o /out/migrate ./cmd/migrate \
 && go build -trimpath -ldflags="-s -w" -o /out/tg-login ./cmd/tg-login

# ---------- runtime ----------
FROM alpine:3.20
RUN apk add --no-cache ca-certificates tzdata \
 && adduser -D -u 10001 -h /app feedium
WORKDIR /app

COPY --from=builder /out/feedium /usr/local/bin/feedium
COPY --from=builder /out/migrate /usr/local/bin/migrate
COPY --from=builder /out/tg-login /usr/local/bin/tg-login
COPY migrations /app/migrations

# The Telegram session dir is a mounted volume written by the non-root app user
# (and by tg-login). Own it as feedium so a fresh named volume inherits writable
# permissions — otherwise gotd's StoreSession fails and the userbot can't persist
# its session.
RUN mkdir -p /var/lib/feedium && chown feedium:feedium /var/lib/feedium

USER feedium
EXPOSE 8000 9000

# Default to the server; the migrate step overrides command (see
# docker-compose.prod.yml). The config is mounted read-only at runtime.
CMD ["feedium", "-conf", "/etc/feedium/config.yaml"]
