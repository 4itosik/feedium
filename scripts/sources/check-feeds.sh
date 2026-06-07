#!/usr/bin/env bash
#
# Liveness checker for a Feedium feeds file. For each feed URL it follows
# redirects and reports HTTP status + whether the body actually looks like a
# feed (XML/RSS/Atom) vs an HTML error page. Read-only — touches nothing.
#
# Usage:   scripts/sources/check-feeds.sh [feeds-file]
# Default feeds-file: ai-tech-news.feeds next to this script.
# Exit code is non-zero if any feed is not a live 2xx XML response.

set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
FEEDS_FILE="${1:-$SCRIPT_DIR/ai-tech-news.feeds}"
UA="${UA:-Mozilla/5.0 (compatible; FeediumBot/1.0; +https://feedium.local)}"

[ -f "$FEEDS_FILE" ] || { echo "error: feeds file not found: $FEEDS_FILE" >&2; exit 2; }

bad=0 total=0
while IFS= read -r raw || [ -n "$raw" ]; do
  case "$raw" in ''|\#*) continue ;; esac
  url="${raw%%[[:space:]]*}"
  [ -z "$url" ] && continue
  total=$((total + 1))

  meta="$(curl -sSL -m 25 -A "$UA" -o /tmp/feedium_feed_body \
            -w '%{http_code}|%{url_effective}' "$url" 2>/dev/null || echo 'ERR|')"
  code="${meta%%|*}"; final="${meta#*|}"
  head1="$(head -c 400 /tmp/feedium_feed_body 2>/dev/null | LC_ALL=C tr -dc '[:print:]' || true)"
  case "$head1" in
    *"<rss"*|*"<feed"*|*"<rdf:RDF"*|*"<?xml"*) kind="XML" ;;
    "") kind="EMPTY" ;;
    *) kind="HTML" ;;
  esac

  status="ok"
  case "$code" in 2*) [ "$kind" = "XML" ] || status="NOT-XML" ;; *) status="HTTP-$code" ;; esac
  [ "$status" = "ok" ] || bad=$((bad + 1))

  printf '%-8s %-4s %-5s %s\n' "$status" "$code" "$kind" "$url"
  [ "$final" != "$url" ] && [ -n "$final" ] && printf '         ↳ %s\n' "$final"
done < "$FEEDS_FILE"

echo
echo "summary: $((total - bad))/$total live, $bad problem(s)"
[ "$bad" -eq 0 ]
