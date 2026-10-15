# Deploying Feedium on a single VPS

Feedium ships as one container image (the `feedium` server + a `migrate` helper +
a one-shot `tg-login` tool) plus a PostgreSQL container, orchestrated with Docker
Compose. Secrets are
provided by a **mounted config file** and an **env file** — nothing secret is
baked into the image or committed to git.

Artifacts used here:

| File | Purpose | Committed? |
|---|---|---|
| `Dockerfile` | builds `feedium` + `migrate` + `tg-login` (static, CGO-off) on `alpine` | yes |
| `docker-compose.prod.yml` | postgres + one-shot migrate + app | yes |
| `configs/prod.example.yaml` | config template (no secrets) | yes |
| `.env.prod.example` | postgres credentials template | yes |
| `configs/prod/config.yaml` | **real** mounted config (secrets) | no (gitignored) |
| `.env.prod` | **real** postgres credentials | no (gitignored) |

## Quick start (scripted)

`scripts/deploy.sh` wraps the whole procedure below — preflight, secret
scaffolding, build & start, health wait, optional seeding:

```bash
scripts/deploy.sh            # first run scaffolds .env.prod + configs/prod/config.yaml
                             # then stops with MANUAL STEPS — fill in the secrets
scripts/deploy.sh            # re-run: builds, starts, waits until /healthz is green
scripts/deploy.sh --seed "LLMs, AI agents, ML research"   # also create the interest + seed RSS
```

The sections below document each step the script performs, for when you need to
do something by hand.

## Deploy from your laptop (no build on the server)

If you'd rather not build on the VPS, `scripts/deploy-remote.sh` builds the image
**locally** for the server's platform, streams it over SSH
(`docker save | gzip | ssh docker load`), and starts it there with
`docker-compose.remote.yml`. The server needs only Docker + Compose — no source
tree, no Go, no git. Migrations are baked into the image.

```bash
# <ssh-host> is an alias from ~/.ssh/config (or user@host).
scripts/deploy-remote.sh feedium               # first run scaffolds secrets ON the server, then stops
# ssh in, fill .env.prod + configs/prod/config.yaml, then:
scripts/deploy-remote.sh feedium               # build → ship image → start → wait for /healthz
scripts/deploy-remote.sh feedium --seed "LLMs, AI agents, ML research"
```

Notes:
- Building on Apple Silicon for an amd64 VPS is handled by `--platform linux/amd64`
  (the default; override with `PLATFORM=linux/arm64 ...` for an ARM server).
- Secrets never leave the server — only the `*.example` templates are copied; the
  real `.env.prod` / `configs/prod/config.yaml` are created and edited on the VPS.
- Reach the localhost-bound API from your laptop via a tunnel:
  `ssh -L 8000:127.0.0.1:8000 feedium`, then open `http://127.0.0.1:8000`.
- `--no-build` reuses the image already on the server (just restarts).

## Prerequisites

- A Linux VPS with Docker Engine + Compose v2 (`docker compose version`).
- An Anthropic API key (summarization, both modes, `claude-haiku-5-5`).
- An OpenRouter API key (embeddings; also the fallback summarization provider).
- The repository checked out on the server (or copied over).

## 1. Create the secrets

```bash
# Postgres credentials for the DB container and the migrate step.
cp .env.prod.example .env.prod
# edit .env.prod: set a strong POSTGRES_PASSWORD

# App config (DB connection + Anthropic/OpenRouter keys, collector intervals, …).
mkdir -p configs/prod
cp configs/prod.example.yaml configs/prod/config.yaml
# edit configs/prod/config.yaml:
#   - data.database.password  == POSTGRES_PASSWORD in .env.prod
#   - data.database.host       == postgres   (already set)
#   - summary.llm.providers.anthropic.api_key        = sk-ant-...
#   - summary.llm.providers.openrouter.api_key       = sk-or-...
#   - scoring.embedding.providers.openrouter.api_key = sk-or-...
```

> **Summarization provider per mode (FT-021).** `summary.llm.mode_providers`
> picks a `providers` entry for `self_contained` (posts) and `cumulative`
> (digests); a mode without an override uses `summary.llm.provider`. Each entry
> declares its wire `protocol`: `anthropic` (Claude Messages API) or `openai`
> (OpenAI-compatible, e.g. OpenRouter). To roll a mode back to OpenRouter, point
> it at `openrouter` and restart the app. Embeddings always use
> `scoring.embedding` (Anthropic has no embeddings API).

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

Left idle by default (`collectors.telegram.api_id: 0`); RSS and web collectors
run without it. The userbot **never logs in on its own** — first-time phone-code
authorization is a manual human step (AG-02). The image bundles a one-shot
`tg-login` tool for exactly this. Enabling it is a one-time provisioning:

1. **Get API credentials.** Sign in to <https://my.telegram.org> with the phone
   number of the account that will act as the userbot → *API development tools* →
   create an app → copy `api_id` and `api_hash`.

2. **Fill the config** (`configs/prod/config.yaml`, on the server) under
   `collectors.telegram`:

   ```yaml
   collectors:
     telegram:
       api_id: 1234567              # from my.telegram.org
       api_hash: "abcd...ef"        # from my.telegram.org
       phone: "+15551234567"        # the userbot account, E.164 format
       session_path: "/var/lib/feedium/tg.session"
   ```

   Keep `TG_SESSION_KEY` set in `.env.prod` (base64 32 bytes) — the session file
   is then encrypted at rest. The same key must be present when `tg-login` runs
   and when the app reads the session back.

3. **Run the interactive login once**, in the same image/volume the app uses, with
   a TTY attached (`-it`). The login code arrives in that account's Telegram app
   (or by SMS); if the account has 2FA, you'll also be prompted for the password:

   ```bash
   # remote deploy (docker-compose.remote.yml)
   docker compose -f docker-compose.remote.yml --env-file .env.prod \
     run --rm -it app tg-login -conf /etc/feedium/config.yaml

   # build-on-server deploy uses docker-compose.prod.yml instead
   ```

   On success it logs `login successful — session saved` and writes the authorized
   session to `/var/lib/feedium/tg.session` in the `feedium-tg-data` volume. Re-runs
   are idempotent (an already-authorized session is a no-op).

4. **Restart the app** so the collector reconnects with the now-authorized session:

   ```bash
   docker compose -f docker-compose.remote.yml --env-file .env.prod up -d app
   ```

5. **Add sources and join them.** The userbot only receives messages from chats it
   is a **member** of — join the target channels/groups from that account. Then
   register each as a source (via `feediumctl source create`, type
   `telegram_channel` / `telegram_group`, with the chat's `tg_id`). Unknown peers
   are silently ignored by the collector.

Troubleshooting: `collectors.telegram.api_id is 0 …` means the config wasn't
filled; `telegram session not authorized` in the app log means step 3 hasn't run
(or ran with a different `TG_SESSION_KEY`).

**A Telegram source never collects.** Check membership and the real `tg_id` with
the read-only inspect modes of `tg-login`. They never log in. Stop the app first so
two clients do not write the same session file. Any TG messages that arrive during
the stop are lost, because there is no backfill:

```bash
C="docker compose -f docker-compose.server.yml --env-file .env.prod"
$C stop app
$C run --rm --no-deps app tg-login -conf /etc/feedium/config.yaml -dialogs            # kind, tg_id, username, title
$C run --rm --no-deps app tg-login -conf /etc/feedium/config.yaml -resolve user1,user2 # username, kind, tg_id, member, title
$C start app
```

A source whose `tg_id` is not in `-dialogs` is a chat the userbot is not in. Join it
from the account, or fix the `tg_id` with `feediumctl source update`.

## Health monitoring

`scripts/monitor/feedium-healthcheck.sh` runs hourly from the `openclaw` user's
crontab on the VPS. It reports to the Feedium Telegram topic through OpenClaw.
It checks `/healthz`, failed summary/score events, collector `ERROR` log lines,
stuck cumulative digests, sources that went silent, Telegram sources that never
collected, and the OpenRouter balance. It sends a message only when a new issue
appears, repeats one every 24h while issues persist, and sends one "resolved"
message when everything clears. Run it with `--dry-run` to print the
report without sending it.
