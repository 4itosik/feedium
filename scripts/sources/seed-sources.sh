#!/usr/bin/env bash
#
# Idempotent RSS source seeder for Feedium.
#
# Reads a feeds file (default: ai-tech-news.feeds next to this script),
# lists already-registered RSS sources via feediumctl, and creates only the
# feeds that are missing. Re-running never produces duplicates: dedup is by
# normalized feed_url (trailing slash + surrounding whitespace stripped).
#
# Usage:
#   scripts/sources/seed-sources.sh [feeds-file]
#
# Env overrides:
#   FEEDIUMCTL   path to the feediumctl binary   (default: ./bin/feediumctl)
#   ENDPOINT     gRPC endpoint host:port          (default: localhost:9000)
#   DRY_RUN=1    print actions, create nothing
#
# Requires: feediumctl (build with `make feediumctl`), jq.
# Note: this does NOT verify feed liveness — availability checking is out of
# FT-003 scope. Feeds marked [verify] in the list should be curl-checked first.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FEEDS_FILE="${1:-$SCRIPT_DIR/ai-tech-news.feeds}"
FEEDIUMCTL="${FEEDIUMCTL:-./bin/feediumctl}"
ENDPOINT="${ENDPOINT:-localhost:9000}"
DRY_RUN="${DRY_RUN:-0}"

die() { printf 'error: %s\n' "$1" >&2; exit 1; }

command -v jq >/dev/null 2>&1 || die "jq is required but not found in PATH"
[ -f "$FEEDS_FILE" ] || die "feeds file not found: $FEEDS_FILE"
if ! command -v "$FEEDIUMCTL" >/dev/null 2>&1 && [ ! -x "$FEEDIUMCTL" ]; then
  die "feediumctl not found at '$FEEDIUMCTL' (build it: make feediumctl)"
fi

# Strip a trailing slash and surrounding whitespace so http://x/ == http://x .
normalize() { sed -e 's/^[[:space:]]*//' -e 's/[[:space:]]*$//' -e 's#/$##'; }

echo "feediumctl : $FEEDIUMCTL"
echo "endpoint   : $ENDPOINT"
echo "feeds file : $FEEDS_FILE"
[ "$DRY_RUN" = "1" ] && echo "mode       : DRY RUN (no writes)"
echo

# Snapshot of feed_urls already registered as RSS sources.
echo "Fetching existing RSS sources..."
existing="$(
  "$FEEDIUMCTL" --endpoint "$ENDPOINT" -o json --page-size 1000 \
    source list --type RSS \
  | jq -r '.[]?.config.rss.feed_url // empty' \
  | normalize | sort -u
)"
existing_count="$(printf '%s' "$existing" | grep -c . || true)"
echo "  $existing_count already registered."
echo

created=0; skipped=0; failed=0
while IFS= read -r raw || [ -n "$raw" ]; do
  case "$raw" in ''|\#*) continue ;; esac           # skip blanks and comments
  url="$(printf '%s' "${raw%%[[:space:]]*}" | normalize)"
  [ -z "$url" ] && continue

  if printf '%s\n' "$existing" | grep -qxF "$url"; then
    echo "skip   : $url"
    skipped=$((skipped + 1))
    continue
  fi

  if [ "$DRY_RUN" = "1" ]; then
    echo "create*: $url"
  else
    if "$FEEDIUMCTL" --endpoint "$ENDPOINT" source create rss --feed-url "$url" >/dev/null; then
      echo "create : $url"
    else
      echo "FAIL   : $url" >&2
      failed=$((failed + 1))
      continue
    fi
  fi
  created=$((created + 1))
  existing="$existing"$'\n'"$url"                    # guard dups within the file
done < "$FEEDS_FILE"

echo
echo "summary: created=$created skipped=$skipped failed=$failed"
[ "$failed" -eq 0 ] || exit 1
