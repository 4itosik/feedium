# FT-021 verify summary

| Check | Result | Evidence |
| --- | --- | --- |
| `CHK-01` `make test` | PASS — all packages `ok`, incl. testcontainers integration (`internal/data`, `internal/task`, `internal/service/health`) | `chk-01/test.log` |
| `CHK-01` `make lint` | PASS for FT-021 surface — 0 findings in changed files. The run exits non-zero on pre-existing findings in untouched files (`internal/data/telegram_session_storage*.go`, `internal/server/static.go`, `ui/ui.go`, `ui/node_modules/flatted`) | `chk-01/lint.log` |
| `CHK-01` config scan | PASS — `configs/local.example.yaml`, `configs/prod.example.yaml` scan into `conf.Bootstrap` and build the router (both modes → `anthropic`, `claude-haiku-5-5`, max_tokens 16000, effort low); a legacy config without `mode_providers`/`protocol` still routes both modes to `provider` | ad-hoc scan test (not kept) |
| `CHK-01` prod | PASS — both modes log `provider=anthropic model=claude-haiku-5-5`; post summaries produced via Anthropic after the switch, 0 failures; legacy config canary passed first. Cumulative digest pending its hourly cron | `chk-01/prod-verify.md` |
