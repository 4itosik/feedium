---
title: "FT-021: Anthropic LLM Provider"
doc_kind: feature
doc_function: canonical
purpose: "Короткий canonical feature-документ: Anthropic (Claude Messages API) как второй LLM-провайдер суммаризации наряду с OpenAI-совместимым OpenRouter, с выбором провайдера через конфиг отдельно для каждого режима обработки."
derived_from:
  - ../../domain/problem.md
  - ../FT-005-ai-summarization/spec.md
  - ../FT-008/feature.md
status: active
delivery_status: done
---

# FT-021: Anthropic LLM Provider

## What

### Problem

Суммаризация (self-contained посты и cumulative дайджесты) умеет ходить только в OpenAI-совместимый
`/chat/completions` (OpenRouter). Ключ `summary.llm.provider` выбирает запись из `providers`, но
протокол у всех записей один, и выбор действует сразу на оба режима. Нельзя отдать суммаризацию
Claude через нативный API Anthropic и нельзя развести режимы по разным провайдерам.

### Scope

- `REQ-01` Провайдер с протоколом `anthropic` вызывает Claude Messages API через официальный Go SDK
  (`github.com/anthropics/anthropic-sdk-go`); протокол OpenAI-совместимого OpenRouter остаётся и
  остаётся default для записей без явного протокола.
- `REQ-02` Провайдер выбирается в конфиге отдельно для каждого `ProcessingMode`
  (`summary.llm.mode_providers.<mode>`); режим без override использует `summary.llm.provider`.
  Ошибка конфигурации (неизвестный провайдер / протокол / режим) валит старт процесса.
- `REQ-03` Example- и prod-конфиги переключают оба режима на Anthropic с моделью `claude-haiku-5-5`;
  OpenRouter-запись остаётся в `providers` для отката правкой конфига.

### Non-Scope

- `NS-01` Embeddings / скоринг (`scoring.embedding`) не меняются: у Anthropic нет embeddings API,
  скоринг остаётся на OpenRouter.

### Constraints

- `CON-01` Существующий контракт ретраев (FT-005 NS-04, `summary.llm.max_retries` в biz/) остаётся
  единственным: SDK-ретраи отключены, чтобы не умножать попытки. Старый конфиг без
  `mode_providers` / `protocol` продолжает работать без изменений.

## How

### Solution

В biz/ появляется порт `SummaryLLMResolver.For(mode) (LLMProvider, error)` — по образцу
`SummaryPromptResolver`; use case резолвит провайдера один раз на событие (вне retry-цикла), сам
`LLMProvider` по-прежнему не знает о режиме. В data/ `summaryLLMRouter` на старте строит по одному
клиенту на каждый используемый провайдер (switch по `protocol`: `openai` | `anthropic`) и
проверяет, что каждый режим из `AllProcessingModes()` резолвится. Trade-off: новая зависимость
(Anthropic SDK) вместо ручного HTTP, зато корректная обработка `stop_reason`, ошибок и типов
ответа без самописного клиента.

### Change Surface

| Surface | Why |
| --- | --- |
| `internal/conf/conf.proto` | `SummaryLLM.mode_providers`, `SummaryLLMProvider.protocol/max_tokens/effort` |
| `internal/biz/summary.go`, `summary_usecase.go` | порт `SummaryLLMResolver`, резолв провайдера по режиму |
| `internal/data/anthropic_provider.go` | клиент Claude Messages API (SDK) |
| `internal/data/llm_router.go`, `wire.go` | выбор провайдера по режиму, DI |
| `configs/*.yaml` | Anthropic по умолчанию для обоих режимов, timeout 60s |

### Flow

1. Событие суммаризации приходит в `SummaryUsecase` с известным `ProcessingMode`.
2. Use case берёт промпт (`SummaryPromptResolver`) и провайдера (`SummaryLLMResolver`) для режима.
3. Провайдер вызывает свой API; для Anthropic текст собирается из `text`-блоков ответа, а
   `stop_reason` `refusal` / `max_tokens` и пустой ответ превращаются в ошибку (её ретраит biz/).

## Verify

### Exit Criteria

- `EC-01` Каждый режим суммаризации вызывает провайдера, указанного для него в конфиге; Anthropic-
  провайдер шлёт корректный Messages API запрос и разбирает ответ; `make lint` и `make test`
  зелёные; на проде оба режима ходят в Anthropic.

### Acceptance Scenarios

- `SC-01` Конфиг `mode_providers: {self_contained: anthropic, cumulative: openrouter}`: пост
  суммаризируется через Anthropic (`POST /v1/messages`, `x-api-key`, `system` = промпт режима),
  дайджест — через OpenRouter (`POST /chat/completions`); конфиг без `mode_providers` гонит оба
  режима в `provider`.

### Traceability matrix

| Requirement ID | Design refs | Acceptance refs | Checks | Evidence IDs |
| --- | --- | --- | --- | --- |
| `REQ-01` | `CON-01` | `EC-01`, `SC-01` | `CHK-01` | `EVID-01` |
| `REQ-02` | `CON-01` | `EC-01`, `SC-01` | `CHK-01` | `EVID-01` |
| `REQ-03` | `CON-01` | `EC-01` | `CHK-01` | `EVID-01` |

### Checks

| Check ID | Covers | How to check | Expected |
| --- | --- | --- | --- |
| `CHK-01` | `EC-01`, `SC-01` | `make lint && make test` (unit: biz routing, data router + Anthropic provider на httptest); после деплоя — логи app без ошибок суммаризации и новые саммари в БД | всё зелёное; свежие саммари созданы после переключения |

### Test matrix

| Check ID | Evidence IDs | Evidence path |
| --- | --- | --- |
| `CHK-01` | `EVID-01` | `artifacts/ft-021/verify/chk-01/` |

### Evidence

- `EVID-01` Вывод `make lint` / `make test` и выдержка прод-логов / SQL о свежих саммари.

### Evidence contract

| Evidence ID | Artifact | Producer | Path contract | Reused by checks |
| --- | --- | --- | --- | --- |
| `EVID-01` | Логи lint/test + prod verify | verify-runner / human | `artifacts/ft-021/verify/chk-01/` | `CHK-01` |
