#!/usr/bin/env bash
#
# Deploy Feedium to a remote VPS from your laptop, without building on the server.
#
# The image is built locally for the server's platform, streamed over SSH
# (docker save | gzip | ssh docker load), and started there with
# docker-compose.remote.yml. Migrations are baked into the image; the server
# never needs the source tree — only the compose file and the secrets.
#
# Usage:
#   scripts/deploy-remote.sh <ssh-host> [remote-dir]
#   scripts/deploy-remote.sh feedium                       # ~/.ssh/config alias "feedium"
#   scripts/deploy-remote.sh feedium /opt/feedium
#   scripts/deploy-remote.sh feedium --seed "LLMs, AI agents"
#
# <ssh-host>   anything ssh/scp accept: an alias from ~/.ssh/config, or user@host.
# remote-dir   where files live on the server (default: feedium, i.e. ~/feedium).
#
# Flags:
#   --seed [INTEREST]  after the app is up, build feediumctl for the server,
#                      copy it over, create INTEREST (if given) and seed the
#                      curated RSS list — all run on the server.
#   --no-build         reuse the image already loaded on the server (just restart).
#   -h, --help         show this help.
#
# Secrets never leave the server: this script copies only the *.example templates
# and scaffolds .env.prod / configs/prod/config.yaml on the server on first run,
# then stops so you fill them in there. Re-run once they are real.
#
# Env overrides:
#   PLATFORM   target image platform        (default: linux/amd64)
#   SSH_OPTS   extra args passed to ssh/scp  (default: empty)

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
ROOT="$(cd "$SCRIPT_DIR/.." && pwd)"
cd "$ROOT"

PLATFORM="${PLATFORM:-linux/amd64}"
COMPOSE_FILE="${COMPOSE_FILE:-docker-compose.remote.yml}"
IMAGE="feedium:latest"

bold() { printf '\033[1m%s\033[0m\n' "$1"; }
info() { printf '  \033[36m›\033[0m %s\n' "$1"; }
ok()   { printf '  \033[32m✓\033[0m %s\n' "$1"; }
warn() { printf '  \033[33m!\033[0m %s\n' "$1" >&2; }
die()  { printf '\033[31merror:\033[0m %s\n' "$1" >&2; exit 1; }

# --- args ---------------------------------------------------------------------
HOST=""
REMOTE_DIR="feedium"
DO_SEED=0
SEED_INTEREST=""
DO_BUILD=1

while [ $# -gt 0 ]; do
  case "$1" in
    --seed)
      DO_SEED=1
      if [ $# -ge 2 ] && [ "${2#-}" = "$2" ] && [ -n "$HOST" ]; then SEED_INTEREST="$2"; shift; fi
      ;;
    --no-build) DO_BUILD=0 ;;
    -h|--help)  awk 'NR>1 && /^#/{sub(/^# ?/,""); print; next} NR>1{exit}' "$0"; exit 0 ;;
    -*) die "unknown flag: $1 (try --help)" ;;
    *)
      if [ -z "$HOST" ]; then HOST="$1"
      else REMOTE_DIR="$1"; fi
      ;;
  esac
  shift
done

[ -n "$HOST" ] || die "missing <ssh-host> (e.g. an alias from ~/.ssh/config). Try --help"

# shellcheck disable=SC2206
SSH_ARGS=(${SSH_OPTS:-})
rsh()  { ssh "${SSH_ARGS[@]}" "$HOST" "$@"; }
# Run a command in the remote dir.
rdir() { rsh "cd $(printf %q "$REMOTE_DIR") && $*"; }

# --- 0. local preflight -------------------------------------------------------
bold "0/6 Preflight (local)"
command -v docker >/dev/null 2>&1 || die "docker not found locally"
command -v ssh    >/dev/null 2>&1 || die "ssh not found locally"
command -v scp    >/dev/null 2>&1 || die "scp not found locally"
[ -f "$COMPOSE_FILE" ] || die "missing $COMPOSE_FILE (run from the repo)"
info "checking SSH access to '$HOST'"
rsh "true" || die "cannot SSH to '$HOST' — check ~/.ssh/config / keys / SSH_OPTS"
rsh "docker compose version" >/dev/null 2>&1 || die "remote host lacks docker + compose v2"
ok "local docker + SSH to '$HOST' (docker compose present there)"

# --- 1. build image locally for the server's platform -------------------------
bold "1/6 Build image (local, $PLATFORM)"
if [ "$DO_BUILD" = "1" ]; then
  info "docker build --platform $PLATFORM -t $IMAGE ."
  docker build --platform "$PLATFORM" -t "$IMAGE" .
  ok "image built"
else
  info "skipped (--no-build)"
fi

# --- 2. ship the compose file + templates -------------------------------------
bold "2/6 Sync compose file + templates → $HOST:$REMOTE_DIR"
rsh "mkdir -p $(printf %q "$REMOTE_DIR")/configs"
scp "${SSH_ARGS[@]}" "$COMPOSE_FILE"            "$HOST:$REMOTE_DIR/"          >/dev/null
scp "${SSH_ARGS[@]}" ".env.prod.example"        "$HOST:$REMOTE_DIR/"          >/dev/null
scp "${SSH_ARGS[@]}" "configs/prod.example.yaml" "$HOST:$REMOTE_DIR/configs/" >/dev/null
ok "compose + templates copied"

# --- 3. transfer the image ----------------------------------------------------
bold "3/6 Transfer image → $HOST"
if [ "$DO_BUILD" = "1" ]; then
  info "docker save $IMAGE | gzip | ssh $HOST docker load  (this can take a minute)"
  docker save "$IMAGE" | gzip | rsh "gunzip | docker load"
  ok "image loaded on the server"
else
  info "skipped (--no-build) — using image already on the server"
fi

# --- 4. secrets on the server: scaffold on first run --------------------------
bold "4/6 Secrets (on the server)"
HAS_ENV=$(rdir "test -f .env.prod && echo yes || echo no")
HAS_CFG=$(rdir "test -f configs/prod/config.yaml && echo yes || echo no")
SCAFFOLDED=0

if [ "$HAS_ENV" != "yes" ]; then
  rdir "cp .env.prod.example .env.prod"
  warn "created $REMOTE_DIR/.env.prod on the server from template"
  SCAFFOLDED=1
else
  ok ".env.prod present on the server"
fi
if [ "$HAS_CFG" != "yes" ]; then
  rdir "mkdir -p configs/prod && cp configs/prod.example.yaml configs/prod/config.yaml"
  warn "created $REMOTE_DIR/configs/prod/config.yaml on the server from template"
  SCAFFOLDED=1
else
  ok "configs/prod/config.yaml present on the server"
fi

if [ "$SCAFFOLDED" = "1" ]; then
  cat >&2 <<EOF

$(bold "MANUAL STEPS — fill in the secrets ON THE SERVER, then re-run this script")

  ssh $HOST
  cd $REMOTE_DIR
  \$EDITOR .env.prod                  # POSTGRES_PASSWORD = strong password
                                      # TG_SESSION_KEY = \$(openssl rand -base64 32)
  \$EDITOR configs/prod/config.yaml   # data.database.password = same password
                                      # summary.llm...openrouter.api_key   = sk-or-...
                                      # scoring.embedding...openrouter.api_key = sk-or-...
                                      # keep data.database.host = postgres; durations in seconds (300s)

EOF
  exit 1
fi

# Reject leftover template placeholders before starting anything.
PLACEHOLDERS="CHANGE_ME_TO_A_STRONG_PASSWORD|REPLACE_WITH_YOUR_OPENROUTER_API_KEY|GENERATE_WITH_OPENSSL_RAND"
if rdir "grep -Eq '$PLACEHOLDERS' .env.prod configs/prod/config.yaml"; then
  warn "template placeholders still present in the server's secrets:"
  rdir "grep -EnH '$PLACEHOLDERS' .env.prod configs/prod/config.yaml" >&2 || true
  die "fill in the real values on the server, then re-run"
fi
ok "secrets filled in"

# --- 5. start the stack -------------------------------------------------------
bold "5/6 Start on the server"
rdir "docker compose -f $COMPOSE_FILE --env-file .env.prod up -d"
ok "stack started (postgres → migrate → app)"

info "waiting for /healthz (up to 120s)"
healthy=0
for _ in $(seq 1 60); do
  if rsh "curl -fsS -o /dev/null http://127.0.0.1:8000/healthz" 2>/dev/null; then healthy=1; break; fi
  sleep 2
done
if [ "$healthy" = "1" ]; then
  ok "app is healthy"
else
  warn "app did not become healthy in time. Recent app logs:"
  rdir "docker compose -f $COMPOSE_FILE --env-file .env.prod logs --tail 40 app" >&2 || true
  die "deploy did not reach a healthy state — inspect the logs above"
fi

# --- 6. optional seeding (runs on the server) ---------------------------------
bold "6/6 Seeding"
if [ "$DO_SEED" != "1" ]; then
  info "skipped (pass --seed to create an interest + seed RSS sources)"
else
  command -v go >/dev/null 2>&1 || die "go toolchain required locally for --seed (to build feediumctl)"
  GOARCH_TARGET="${PLATFORM##*/}"
  info "building feediumctl for $PLATFORM"
  GOOS=linux GOARCH="$GOARCH_TARGET" CGO_ENABLED=0 go build -o bin/feediumctl ./cmd/feediumctl
  info "copying feediumctl + seed script to the server"
  rdir "mkdir -p bin scripts/sources"
  scp "${SSH_ARGS[@]}" bin/feediumctl                          "$HOST:$REMOTE_DIR/bin/"            >/dev/null
  scp "${SSH_ARGS[@]}" scripts/sources/seed-sources.sh         "$HOST:$REMOTE_DIR/scripts/sources/" >/dev/null
  scp "${SSH_ARGS[@]}" scripts/sources/ai-tech-news.feeds      "$HOST:$REMOTE_DIR/scripts/sources/" >/dev/null
  rdir "chmod +x bin/feediumctl scripts/sources/seed-sources.sh"

  if [ -n "$SEED_INTEREST" ]; then
    info "creating interest: $SEED_INTEREST"
    rdir "./bin/feediumctl --endpoint 127.0.0.1:9000 interest create $(printf %q "$SEED_INTEREST")"
    ok "interest created"
  else
    warn "no interest given — every score stays 0 until you create at least one:"
    warn "  ssh $HOST 'cd $REMOTE_DIR && ./bin/feediumctl --endpoint 127.0.0.1:9000 interest create \"AI, LLMs\"'"
  fi

  info "seeding curated RSS sources (idempotent)"
  rdir "ENDPOINT=127.0.0.1:9000 FEEDIUMCTL=./bin/feediumctl scripts/sources/seed-sources.sh"
  ok "sources seeded"
fi

echo
bold "Done — Feedium is running on $HOST."
cat <<EOF
  Status : ssh $HOST 'cd $REMOTE_DIR && docker compose -f $COMPOSE_FILE --env-file .env.prod ps'
  Logs   : ssh $HOST 'cd $REMOTE_DIR && docker compose -f $COMPOSE_FILE --env-file .env.prod logs -f app'
  Feed   : ssh $HOST 'cd $REMOTE_DIR && ./bin/feediumctl --endpoint 127.0.0.1:9000 feed list'

  Ports are bound to 127.0.0.1 on the server (no built-in auth). Reach the HTTP API
  from your laptop with an SSH tunnel:
    ssh -L 8000:127.0.0.1:8000 $HOST     # then open http://127.0.0.1:8000 locally
  Update later:  scripts/deploy-remote.sh $HOST $REMOTE_DIR     (rebuilds + reships)
EOF
