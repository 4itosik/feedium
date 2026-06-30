# CHK-01 manual verify — FT-019 Digests view

Date: 2026-06-30

## Procedure

1. `cd ui && npm run build` → green (see `build.log`).
2. SSH tunnel to prod backend (read-only): `ssh -fN -L 8000:127.0.0.1:8000 openclaw@oc`.
3. `npm run dev` (Vite, proxies `/v1` → `localhost:8000` per `ui/vite.config.ts`).
4. Opened `http://localhost:5173/digests` in Chrome.

## Result

Page renders nav with active "Digests" tab, and a sorted (newest-first) list of
cumulative-source digest cards — source label (`@neuraldeepchat`,
`@llm_driven_products`, ...), digest text, word count, timestamp. Confirmed
against real prod data: 9 `telegram_group` sources, several with existing
digests (`sample-source-summaries-response.json` is a raw sample response from
one of them). This is exactly the content that was previously invisible
(`summaries.post_id IS NULL`, excluded from the scored ranked feed by design —
see `brief.md`).

Screenshot was taken via the browser tool but its saved file path could not be
located on disk after the fact (chrome extension storage, not a session
scratch path) — this text description plus the raw API response sample stands
in as evidence per `feature.md` `EVID-01`. EC-01/SC-01 are satisfied: digests
are observably visible to the user without touching Feed/Posts/scores.

## Cleanup

Dev server and SSH tunnel were stopped after this check; no prod state was
mutated (read-only GET requests only).
