#!/usr/bin/env bash
#
# One-shot deploy helper for a single VPS. Wraps the procedure in docs/deploy.md:
# preflight checks → scaffold secrets (first run) → build & start the stack →
# wait until the app is healthy → optionally seed an interest + RSS sources.
#
# Usage:
#   scripts/deploy.sh                       # build + start + wait for health
#   scripts/deploy.sh --seed "AI, LLMs"     # also create that interest + seed feeds
#   scripts/deploy.sh --seed                # seed feeds only (assumes interest exists)
#   scripts/deploy.sh --no-build            # restart without rebuilding the image
#
# Flags:
#   --seed [INTEREST]  after the app is up, build feediumctl, create INTEREST
#                      (if given) and seed the curated RSS list (idempotent).
#   --no-build         skip the image rebuild (`up -d` instead of `up -d --build`).
#   -h, --help         show this help.
#
# This script never edits your secrets. On the first run it copies the example
# templates into place and stops so you can fill them in (see "MANUAL STEPS"
# printed at the end). Re-run it once the secrets are real.

set -euo pipefail

# --- locate repo root (this file lives in scripts/) ---------------------------
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT"

COMPOSE_FILE="docker-compose.prod.yml"
ENV_FILE=".env.prod"
ENV_EXAMPLE=".env.prod.example"
CONFIG_FILE="configs/prod/config.yaml"
CONFIG_EXAMPLE="configs/prod.example.yaml"
HEALTH_URL="http://127.0.0.1:8000/healthz"

# --- pretty output ------------------------------------------------------------
bold() { printf '\033[1m%s\033[0m\n' "$1"; }
info() { printf '  \033[36m›\033[0m %s\n' "$1"; }
ok()   { printf '  \033[32m✓\033[0m %s\n' "$1"; }
warn() { printf '  \033[33m!\033[0m %s\n' "$1" >&2; }
die()  { printf '\033[31merror:\033[0m %s\n' "$1" >&2; exit 1; }

# --- args ---------------------------------------------------------------------
DO_SEED=0
SEED_INTEREST=""
BUILD_FLAG="--build"

while [ $# -gt 0 ]; do
  case "$1" in
    --seed)
      DO_SEED=1
      # Optional inline interest text (anything not starting with "-").
      if [ $# -ge 2 ] && [ "${2#-}" = "$2" ]; then SEED_INTEREST="$2"; shift; fi
      ;;
    --no-build) BUILD_FLAG="" ;;
    -h|--help)  awk 'NR>1 && /^#/{sub(/^# ?/,""); print; next} NR>1{exit}' "$0"; exit 0 ;;
    *) die "unknown argument: $1 (try --help)" ;;
  esac
  shift
done

dc() { docker compose -f "$COMPOSE_FILE" --env-file "$ENV_FILE" "$@"; }

# --- 1. preflight: tools ------------------------------------------------------
bold "1/5 Preflight"
command -v docker >/dev/null 2>&1 || die "docker not found in PATH"
docker compose version >/dev/null 2>&1 || die "docker compose v2 not available (need 'docker compose')"
ok "docker + compose v2 present"

for f in "$COMPOSE_FILE" "$ENV_EXAMPLE" "$CONFIG_EXAMPLE"; do
  [ -f "$f" ] || die "missing committed file: $f (are you in the repo root?)"
done

# --- 2. secrets: scaffold on first run, then validate -------------------------
bold "2/5 Secrets"
SCAFFOLDED=0

if [ ! -f "$ENV_FILE" ]; then
  cp "$ENV_EXAMPLE" "$ENV_FILE"
  warn "created $ENV_FILE from template — fill in POSTGRES_PASSWORD"
  SCAFFOLDED=1
else
  ok "$ENV_FILE present"
fi

if [ ! -f "$CONFIG_FILE" ]; then
  mkdir -p "$(dirname "$CONFIG_FILE")"
  cp "$CONFIG_EXAMPLE" "$CONFIG_FILE"
  warn "created $CONFIG_FILE from template — fill in DB password + OpenRouter keys"
  SCAFFOLDED=1
else
  ok "$CONFIG_FILE present"
fi

if [ "$SCAFFOLDED" = "1" ]; then
  cat >&2 <<EOF

$(bold "MANUAL STEPS — do these, then re-run scripts/deploy.sh")

  1. Edit  $ENV_FILE
       POSTGRES_PASSWORD = <a strong password>

  2. Edit  $CONFIG_FILE
       data.database.password                            = <same password as above>
       summary.llm.providers.openrouter.api_key          = sk-or-...
       scoring.embedding.providers.openrouter.api_key    = sk-or-...
     (leave data.database.host = postgres; durations stay in seconds, e.g. 300s)

EOF
  exit 1
fi

# Reject leftover placeholders from the templates.
PLACEHOLDERS="CHANGE_ME_TO_A_STRONG_PASSWORD|REPLACE_WITH_YOUR_OPENROUTER_API_KEY"
if grep -Eq "$PLACEHOLDERS" "$ENV_FILE" "$CONFIG_FILE"; then
  warn "template placeholders still present in your secrets:"
  grep -EnH "$PLACEHOLDERS" "$ENV_FILE" "$CONFIG_FILE" >&2 || true
  die "fill in the real values above, then re-run"
fi

# Cross-check: the DB password in .env.prod must equal the one in the config.
ENV_PW="$(grep -E '^POSTGRES_PASSWORD=' "$ENV_FILE" | head -1 | cut -d= -f2-)"
CFG_PW="$(grep -E '^[[:space:]]*password:' "$CONFIG_FILE" | head -1 | sed -E 's/^[[:space:]]*password:[[:space:]]*//; s/^"//; s/"$//')"
if [ -n "$ENV_PW" ] && [ "$ENV_PW" != "$CFG_PW" ]; then
  die "POSTGRES_PASSWORD in $ENV_FILE != data.database.password in $CONFIG_FILE (they must match)"
fi
ok "secrets filled in and DB password matches"

# --- 3. build & start ---------------------------------------------------------
bold "3/5 Build & start"
[ -n "$BUILD_FLAG" ] && info "building images (this can take a few minutes the first time)"
dc up -d $BUILD_FLAG
ok "stack started (postgres → migrate → app)"

# --- 4. wait for health -------------------------------------------------------
bold "4/5 Health check"
info "polling $HEALTH_URL (up to 120s)"
healthy=0
for _ in $(seq 1 60); do
  if curl -fsS -o /dev/null "$HEALTH_URL" 2>/dev/null; then healthy=1; break; fi
  sleep 2
done
if [ "$healthy" = "1" ]; then
  ok "app is healthy"
else
  warn "app did not become healthy in time. Recent app logs:"
  dc logs --tail 40 app >&2 || true
  die "deploy did not reach a healthy state — inspect the logs above"
fi

# --- 5. optional seeding ------------------------------------------------------
bold "5/5 Seeding"
if [ "$DO_SEED" != "1" ]; then
  info "skipped (pass --seed to create an interest + seed RSS sources)"
else
  command -v go >/dev/null 2>&1 || die "go toolchain required for --seed (to build feediumctl)"
  info "building feediumctl"
  make feediumctl >/dev/null
  CTL=./bin/feediumctl
  EP=127.0.0.1:9000

  if [ -n "$SEED_INTEREST" ]; then
    info "creating interest: $SEED_INTEREST"
    "$CTL" --endpoint "$EP" interest create "$SEED_INTEREST"
    ok "interest created"
  else
    warn "no interest given — every score stays 0 until you create at least one:"
    warn "  $CTL --endpoint $EP interest create \"AI, LLMs, ML research\""
  fi

  info "seeding curated RSS sources (idempotent)"
  ENDPOINT="$EP" FEEDIUMCTL="$CTL" scripts/sources/seed-sources.sh
  ok "sources seeded"
fi

# --- done ---------------------------------------------------------------------
echo
bold "Done."
cat <<EOF
  Status : docker compose -f $COMPOSE_FILE --env-file $ENV_FILE ps
  Logs   : docker compose -f $COMPOSE_FILE --env-file $ENV_FILE logs -f app
  Feed   : ./bin/feediumctl --endpoint 127.0.0.1:9000 feed list

  Ports are bound to 127.0.0.1 only (no built-in auth). Reach them over an SSH
  tunnel, or put a TLS reverse proxy in front of :8000. Never expose :9000.
EOF
