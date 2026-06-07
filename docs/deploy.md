# Deploying Feedium on a single VPS

Feedium ships as one container image (the `feedium` server + a `migrate` helper)
plus a PostgreSQL container, orchestrated with Docker Compose. Secrets are
provided by a **mounted config file** and an **env file** — nothing secret is
baked into the image or committed to git.

Artifacts used here:

| File | Purpose | Committed? |
|---|---|---|
| `Dockerfile` | builds `feedium` + `migrate` (static, CGO-off) on `alpine` | yes |
| `docker-compose.prod.yml` | postgres + one-shot migrate + app | yes |
| `configs/prod.example.yaml` | config template (no secrets) | yes |
| `.env.prod.example` | postgres credentials template | yes |
| `configs/prod/config.yaml` | **real** mounted config (secrets) | no (gitignored) |
| `.env.prod` | **real** postgres credentials | no (gitignored) |

## Prerequisites

- A Linux VPS with Docker Engine + Compose v2 (`docker compose version`).
- An OpenRouter API key (used for both summarization and embeddings).
- The repository checked out on the server (or copied over).

## 1. Create the secrets

```bash
# Postgres credentials for the DB container and the migrate step.
cp .env.prod.example .env.prod
# edit .env.prod: set a strong POSTGRES_PASSWORD

# App config (DB connection + OpenRouter key, collector intervals, …).
mkdir -p configs/prod
cp configs/prod.example.yaml configs/prod/config.yaml
# edit configs/prod/config.yaml:
#   - data.database.password  == POSTGRES_PASSWORD in .env.prod
#   - data.database.host       == postgres   (already set)
#   - summary.llm.providers.openrouter.api_key       = sk-or-...
#   - scoring.embedding.providers.openrouter.api_key = sk-or-...
```

> ⚠️ **Duration fields are protojson** — seconds only (`300s`, `3600s`), never
> `5m`/`1h`/`72h`. The templates are already correct; keep that format if you edit.

## 2. Build and start

```bash
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build
```

Order is enforced by Compose:

1. `postgres` starts and becomes healthy (`pg_isready`).
2. `migrate` runs `goose up` against it and exits 0.
3. `app` starts only after migrate succeeds, mounting `configs/prod/config.yaml`.

Check status and logs:

```bash
docker compose -f docker-compose.prod.yml ps
docker compose -f docker-compose.prod.yml logs -f app
```

The app is healthy once you see `[HTTP] server listening on: [::]:8000` and
`[gRPC] server listening on: [::]:9000`.

## 3. Seed sources and an interest

Ports are bound to `127.0.0.1`, so run `feediumctl` on the server (build it with
`make feediumctl`, or `go run ./cmd/feediumctl`). Point it at the gRPC port:

```bash
# At least one interest, otherwise every score is 0 (scoring skips the embed).
./bin/feediumctl --endpoint 127.0.0.1:9000 interest create "LLMs, AI agents, ML research"

# Seed the curated RSS list (idempotent; needs jq).
ENDPOINT=127.0.0.1:9000 scripts/sources/seed-sources.sh
```

Within one RSS poll interval (`collectors.rss.poll_interval`, default `300s`)
posts start flowing: `collect → summarize → embed+score → ranked feed`.

```bash
./bin/feediumctl --endpoint 127.0.0.1:9000 post list      # collected posts
./bin/feediumctl --endpoint 127.0.0.1:9000 feed list      # ranked feed
```

## 4. Exposing the API (optional)

There is no built-in authentication. Keep the published ports on `127.0.0.1`
and reach them via an SSH tunnel, or put a TLS-terminating reverse proxy
(Caddy/nginx) in front of `8000` and add auth there. Do **not** publish `9000`
(gRPC) to the public internet.

## Operations

```bash
# Update to new code:
git pull
docker compose -f docker-compose.prod.yml --env-file .env.prod up -d --build

# Run a migration manually (e.g. status):
docker compose -f docker-compose.prod.yml run --rm migrate migrate -dir /app/migrations status

# Stop (data is kept in the named volume):
docker compose -f docker-compose.prod.yml down

# Stop and DELETE the database volume:
docker compose -f docker-compose.prod.yml down -v
```

### Cost note

Every (re)scoring re-embeds a post's summary via OpenRouter — post embeddings
are not cached yet (see the FT-013 brief). Tuning interests triggers re-scoring,
which costs embedding calls proportional to the number of re-scored posts.

### Telegram collector

Left idle by default (`collectors.telegram.api_id: 0`). Enabling the userbot
needs `api_id`/`api_hash`/`phone` from my.telegram.org and an interactive login,
which is gated (AG-02) — out of scope for this baseline deploy. RSS and web
collectors run without it.
