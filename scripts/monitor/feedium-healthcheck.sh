#!/usr/bin/env bash
#
# Hourly Feedium health check for the production VPS. Detects the failure modes
# that previously went unnoticed for weeks and reports them to Telegram via the
# OpenClaw gateway (same DM topic as the digest):
#
#   - /healthz down
#   - failed summary/score events in the last 2h (e.g. LLM 402 / out of credits)
#   - collector ERROR log lines in the last hour (e.g. RSS 429)
#   - cumulative digests stuck (group has fresh posts, no digest for 3h)
#   - sources that went silent (active over the past month, nothing for 4 days)
#   - Telegram sources that never collected a post (userbot not a member / bad tg_id)
#   - OpenRouter credit balance (embeddings) below a threshold
#
# Alerts when a new issue appears, or every REPEAT_HOURS while issues persist;
# sends a single "resolved" message when everything is clear again.
#
# Install (as user openclaw on the VPS):
#   crontab: 17 * * * * $HOME/feedium/scripts/monitor/feedium-healthcheck.sh >>$HOME/.local/state/feedium-healthcheck/cron.log 2>&1
#
# Flags: --dry-run   print the report instead of sending it (state is untouched).

set -euo pipefail

FEEDIUM_DIR="${FEEDIUM_DIR:-$HOME/feedium}"
APP_CONTAINER="${APP_CONTAINER:-feedium-app-1}"
PG_CONTAINER="${PG_CONTAINER:-core_storage_pg}"
HEALTH_URL="${HEALTH_URL:-http://127.0.0.1:8000/healthz}"
TG_TARGET="${TG_TARGET:-1753417}"
TG_THREAD="${TG_THREAD:-578177}"
REPEAT_HOURS="${REPEAT_HOURS:-24}"
MIN_OPENROUTER_USD="${MIN_OPENROUTER_USD:-2}"
OPENCLAW_NODE="${OPENCLAW_NODE:-/usr/local/nvm/versions/node/v24.19.0/bin/node}"
OPENCLAW_JS="${OPENCLAW_JS:-/usr/local/nvm/versions/node/v25.6.1/lib/node_modules/openclaw/dist/index.js}"
STATE_DIR="${STATE_DIR:-$HOME/.local/state/feedium-healthcheck}"

DRY_RUN=0
[ "${1:-}" = "--dry-run" ] && DRY_RUN=1

# Each issue is "key|text": key is stable across runs (dedup), text is for humans.
issues=()
add() { issues+=("$1|$2"); }

psql_q() {
  docker exec -i "$PG_CONTAINER" psql -U feedium -d feedium -At -F $'\t' -v ON_ERROR_STOP=1 \
    -c "set statement_timeout = '30s'" -c "$1" | sed '/^SET$/d'
}

# --- 1. health endpoint -------------------------------------------------------
if ! curl -fsS -m 10 -o /dev/null "$HEALTH_URL"; then
  add "health" "❌ $HEALTH_URL не отвечает (контейнер $APP_CONTAINER)"
fi
if ! psql_q "select 1" >/dev/null 2>&1; then
  add "db" "❌ БД feedium в $PG_CONTAINER недоступна — остальные проверки пропущены"
fi

# --- 2. failed events (last 2h) -----------------------------------------------
while IFS=$'\t' read -r kind n err; do
  [ -n "$kind" ] || continue
  add "failed:$kind:${err:0:40}" "❌ $kind: $n упавших за 2ч — ${err:0:160}"
done < <(psql_q "
  select 'summary/' || event_type, count(*), coalesce(left(error, 200), '')
  from summary_events where status = 'failed' and coalesce(processed_at, created_at) > now() - interval '2 hours'
  group by 1, 3
  union all
  select 'score', count(*), coalesce(left(error, 200), '')
  from score_events where status = 'failed' and coalesce(processed_at, created_at) > now() - interval '2 hours'
  group by 1, 3
  order by 2 desc limit 5")

# --- 3. collector errors in logs (last hour) ----------------------------------
if docker inspect "$APP_CONTAINER" >/dev/null 2>&1; then
  while IFS=$'\t' read -r n msg src err; do
    [ -n "$n" ] || continue
    url=""
    [ -n "$src" ] && url=$(psql_q "select coalesce(config->>'feed_url', config->>'url', config->>'username', '') from sources where id = '$src'" || true)
    add "log:$msg:$src" "⚠️ $msg ×$n за час ${url:+($url) }— ${err:0:120}"
  done < <(docker logs --since 1h "$APP_CONTAINER" 2>&1 \
    | jq -Rr 'fromjson? | select(.level == "ERROR") | [.msg, (.source_id // ""), (.err // "" | tostring)] | @tsv' \
    | sort | uniq -c | sort -rn | head -5 | sed -E 's/^ *([0-9]+) /\1\t/')
else
  add "container" "❌ контейнер $APP_CONTAINER не найден"
fi

# --- 4. stuck cumulative digests ----------------------------------------------
while IFS=$'\t' read -r name last; do
  [ -n "$name" ] || continue
  add "digest:$name" "⚠️ Дайджест «$name» не обновлялся с $last, хотя посты идут"
done < <(psql_q "
  select coalesce(nullif(s.config->>'username', ''), s.config->>'tg_id'),
         coalesce(to_char(max(su.created_at), 'DD.MM HH24:MI'), 'никогда')
  from sources s left join summaries su on su.source_id = s.id and su.post_id is null
  where s.type = 'telegram_group'
    and exists (select 1 from posts p where p.source_id = s.id and p.created_at > now() - interval '3 hours')
  group by s.id having coalesce(max(su.created_at), 'epoch') < now() - interval '3 hours'")

# --- 5. silent sources --------------------------------------------------------
silent=$(psql_q "
  select coalesce(s.config->>'feed_url', s.config->>'url', nullif(s.config->>'username', ''), s.config->>'tg_id')
  from sources s
  where exists (select 1 from posts p where p.source_id = s.id
                  and p.created_at between now() - interval '30 days' and now() - interval '4 days'
                having count(*) >= 10)
    and not exists (select 1 from posts p where p.source_id = s.id and p.created_at > now() - interval '4 days')
  order by 1" | paste -sd ',' - | sed 's/,/, /g')
[ -n "$silent" ] && add "silent:$silent" "🔇 Источники замолчали (>4 дней без постов): $silent"

# --- 6. Telegram sources that never collected ---------------------------------
never=$(psql_q "
  select coalesce(nullif(s.config->>'username', ''), s.config->>'tg_id')
  from sources s
  where s.type in ('telegram_channel', 'telegram_group') and s.created_at < now() - interval '3 days'
    and not exists (select 1 from posts p where p.source_id = s.id)
  order by 1")
if [ -n "$never" ]; then
  cnt=$(printf '%s\n' "$never" | wc -l | tr -d ' ')
  add "never:$(printf '%s' "$never" | md5sum | cut -c1-8)" \
    "📭 $cnt TG-источников ни разу не собрали пост (userbot не подписан или неверный tg_id): $(printf '%s\n' "$never" | head -15 | paste -sd ',' - | sed 's/,/, /g')"
fi

# --- 7. OpenRouter balance (embeddings) ---------------------------------------
or_key=$(awk '/^scoring:/{s=1} s && /api_key:/{gsub(/["[:space:]]/, "", $2); print $2; exit}' \
  "$FEEDIUM_DIR/configs/prod/config.yaml" 2>/dev/null || true)
if [ -n "$or_key" ]; then
  left=$(curl -fsS -m 15 https://openrouter.ai/api/v1/credits -H "Authorization: Bearer $or_key" 2>/dev/null \
    | jq -r '.data | (.total_credits - .total_usage) * 100 | floor / 100' 2>/dev/null || true)
  if [ -z "$left" ]; then
    add "openrouter:unknown" "⚠️ Не удалось получить баланс OpenRouter"
  elif awk -v l="$left" -v m="$MIN_OPENROUTER_USD" 'BEGIN { exit !(l < m) }'; then
    add "openrouter:low" "💸 OpenRouter: осталось \$$left (порог \$$MIN_OPENROUTER_USD) — скоринг встанет при нуле"
  fi
fi

# --- report -------------------------------------------------------------------
# Notify when a new issue key appears, every REPEAT_HOURS while issues persist,
# and once when everything clears. Issues that merely disappear stay quiet.
mkdir -p "$STATE_DIR"
keys=$(printf '%s\n' "${issues[@]+"${issues[@]}"}" | cut -d'|' -f1 | sed '/^$/d' | sort -u)
prev_keys=$(cat "$STATE_DIR/keys" 2>/dev/null || true)
prev_at=$(cat "$STATE_DIR/sent_at" 2>/dev/null || echo 0)
now=$(date +%s)
new_keys=$(comm -23 <(printf '%s\n' "$keys" | sed '/^$/d') <(printf '%s\n' "$prev_keys" | sed '/^$/d'))

if [ ${#issues[@]} -eq 0 ]; then
  if [ -z "$prev_keys" ]; then echo "$(date -Is) ok"; exit 0; fi
  body="✅ Feedium: все проверки снова в норме."
else
  if [ -z "$new_keys" ] && [ $((now - prev_at)) -lt $((REPEAT_HOURS * 3600)) ]; then
    [ "$DRY_RUN" = "1" ] || printf '%s\n' "$keys" >"$STATE_DIR/keys"
    echo "$(date -Is) no new issues (${#issues[@]} open)"; exit 0
  fi
  body="🩺 Feedium healthcheck — проблем: ${#issues[@]}"$'\n'
  for i in "${issues[@]}"; do body+=$'\n'"• ${i#*|}"; done
fi

if [ "$DRY_RUN" = "1" ]; then
  printf '%s\n' "$body"
  exit 0
fi

PATH="$(dirname "$OPENCLAW_NODE"):$PATH" "$OPENCLAW_NODE" "$OPENCLAW_JS" message send \
  --channel telegram --target "$TG_TARGET" --thread-id "$TG_THREAD" --message "$body" >/dev/null
printf '%s\n' "$keys" | sed '/^$/d' >"$STATE_DIR/keys"
echo "$now" >"$STATE_DIR/sent_at"
echo "$(date -Is) sent (${#issues[@]} issues)"
