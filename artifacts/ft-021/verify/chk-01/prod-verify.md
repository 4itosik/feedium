# FT-021 prod verify (2026-10-14)

Target: `openclaw@oc`, `/home/openclaw/feedium`, `COMPOSE_FILE=docker-compose.server.yml`.

1. Image `feedium:latest` = `86f16fdd6cdb` (commit `370d862`) loaded; the previous image is kept as
   `feedium:rollback-ft020` (`b16be34a3c09`).
2. Canary: new binary started with the **old** prod config (no `mode_providers` / `protocol`) →
   `/healthz` OK, log `summary llm provider selected` mode=self_contained/cumulative
   provider=openrouter — legacy config is backward compatible.
3. Prod config `summary.llm` rewritten in place (backup `configs/prod/config.yaml.bak.1792003295`):
   `provider: anthropic`, `mode_providers: {self_contained: anthropic, cumulative: anthropic}`,
   anthropic = `claude-haiku-5-5`, max_tokens 16000, effort low, timeout 60s; openrouter entry kept.
4. `docker restart feedium-app-1` → healthy after 1s; log at 18:41:54Z:
   `summary llm provider selected` mode=self_contained provider=anthropic model=claude-haiku-5-5,
   mode=cumulative provider=anthropic model=claude-haiku-5-5.
5. `summary_events` processed after the switch: `summarize_post completed 2` (21:42:46+03), 0 failed;
   resulting summaries are well-formed Russian one-paragraph texts (156 and 240 chars). No
   summarization errors in app logs.

Not yet observed: a `summarize_source` (cumulative digest) event after the switch — it runs on the
per-source hourly cron; it uses the same provider instance type and request path as posts.

Pre-existing, unrelated: 24 `failed` events earlier the same day with
`summary validation failed: input text exceeds max_input_chars limit`; an RSS source returning 429.

Rollback: set `summary.llm.mode_providers.*: openrouter` (or restore the `.bak` config) and
`docker restart feedium-app-1`; for the binary, `docker tag feedium:rollback-ft020 feedium:latest`
and `docker compose -f docker-compose.server.yml --env-file .env.prod up -d`.
