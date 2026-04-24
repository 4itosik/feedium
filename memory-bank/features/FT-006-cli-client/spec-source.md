---
title: "FT-006: CLI Client — Spec (Source endpoints)"
doc_kind: feature
doc_function: spec
purpose: "Команды `feediumctl source ...` поверх каркаса CLI: маппинг на 5 RPC `SourceService`, флаги, рендеринг, edge cases."
derived_from:
  - feature.md
  - spec.md
status: draft
---

# FT-006: CLI Client — Spec (Source endpoints)

## Цель

Покрыть командами `feediumctl source ...` все 5 методов `SourceService`. Каркас (конфиг, транспорт, форматы вывода, обработка ошибок, инварианты) определён в [`spec.md`](./spec.md) и здесь не повторяется.

## Reference

- Core spec: [`spec.md`](./spec.md) — конфиг (FR-02..FR-07), вывод (FR-08, FR-10, FR-11), ошибки (NFR-03), инварианты, ограничения.
- Brief: [`feature.md`](./feature.md)
- Proto: [`api/feedium/source.proto`](../../../api/feedium/source.proto)
- API Contracts: [`memory-bank/engineering/api-contracts.md`](../../engineering/api-contracts.md)

## Scope

### Входит

- Группа команд `feediumctl source`:
  - `source list` — `V1ListSources`
  - `source get <id>` — `V1GetSource`
  - `source create <type> [flags]` — `V1CreateSource`
  - `source update <id> --type=<type> [flags]` — `V1UpdateSource`
  - `source delete <id>` — `V1DeleteSource`
- Table-рендеринг `Source` в одну строку (см. SR-08).
- JSON/YAML — через общий рендер из core spec (FR-10).
- Unit-тесты с mock gRPC-клиента: формирование `V1*Request`, рендеринг ответа, обработка `codes.NotFound`/`codes.DeadlineExceeded`.
- Ручной сценарий `CHK-01` с артефактом `artifacts/ft-006/verify/chk-01/`.

### НЕ входит

- Auto-pagination `source list` по `page_token` (выводится одна страница, `next_page_token` игнорируется).
- Команды для `post` и `summary` сервисов.
- Все требования core spec (TLS, аутентификация, таймауты, конфиг, рендеринг, обработка ошибок) — наследуются по ссылке из Reference без изменений.

## Функциональные требования

### Команды и маппинг на RPC

1. **SR-01 `feediumctl source list`** → `V1ListSources` с `page_size` из конфигурации (core FR-03) и пустым `page_token`. Выводит `items`, отсортированные **от новых к старым** по `created_at` (сортировка на сервере; CLI не пересортировывает). `next_page_token` игнорируется. Опциональный флаг `--type=<SourceType>` транслируется в `V1ListSourcesRequest.type` (поле — `optional`).
2. **SR-02 `feediumctl source get <id>`** → `V1GetSource{id}`. Выводит `source` из ответа.
3. **SR-03 `feediumctl source create <type> [flags]`** → `V1CreateSource`. Позиционный `<type>` определяет вариант oneof `SourceConfig` ([`source.proto:42`](../../../api/feedium/source.proto)) и `SourceType` в запросе:

   | `<type>` | `SourceType` | Required флаги |
   |---|---|---|
   | `telegram-channel` | `SOURCE_TYPE_TELEGRAM_CHANNEL` | `--tg-id` (int64), `--username` (string) |
   | `telegram-group` | `SOURCE_TYPE_TELEGRAM_GROUP` | `--tg-id` (int64), `--username` (string) |
   | `rss` | `SOURCE_TYPE_RSS` | `--feed-url` (string) |
   | `html` | `SOURCE_TYPE_HTML` | `--url` (string) |

   Выводит созданный `Source` из `V1CreateSourceResponse.source`.
4. **SR-04 `feediumctl source update <id> --type=<type> [flags]`** → `V1UpdateSource`. Семантика `--type` и набор флагов идентичны SR-03, но `--type` — обязательный флаг (позиционный аргумент занят `id`). Выводит обновлённый `Source`.
5. **SR-05 `feediumctl source delete <id>`** → `V1DeleteSource{id}`. `V1DeleteSourceResponse` пуст. Вывод по форматам:
   - `table`: одна строка `deleted: <id>`.
   - `json`: `{"deleted":true,"id":"<id>"}` (одна строка + `\n`).
   - `yaml`: ровно
     ```yaml
     deleted: true
     id: <id>
     ```
     (две строки, ключи отсортированы лексикографически — соответствует core FR-10).
6. **SR-06** Любая команда `source ...` с неизвестным флагом, отсутствующим required-флагом, неизвестным `<type>` или флагом, не принадлежащим указанному `<type>` (см. таблицу SR-03) → exit `1`, сообщение в stderr (формат — core NFR-03 как локальная ошибка), usage-подсказка cobra. RPC не выполняется.

### Рендеринг table для `Source`

7. **SR-07 Источник полей.** Все поля берутся из `Source` ([`source.proto:51`](../../../api/feedium/source.proto)).
8. **SR-08 Колонки `source list` / `source get`.** `ID | TYPE | MODE | CONFIG | CREATED_AT`:
   - `ID` — `Source.id`.
   - `TYPE` — `Source.type` (`SourceType`), короткое имя без префикса `SOURCE_TYPE_` (`TELEGRAM_CHANNEL`, `TELEGRAM_GROUP`, `RSS`, `HTML`). Для `SOURCE_TYPE_UNSPECIFIED` → `UNSPECIFIED`.
   - `MODE` — `Source.processing_mode` (enum `ProcessingMode`, [`source.proto:18`](../../../api/feedium/source.proto)), короткое имя без префикса `PROCESSING_MODE_` (`SELF_CONTAINED`, `CUMULATIVE`). Для `PROCESSING_MODE_UNSPECIFIED` → `UNSPECIFIED`.
   - `CONFIG` — однострочное представление активного варианта `SourceConfig.config` (oneof):
     - `telegram_channel` → `tg_id=<n>,username=<u>`
     - `telegram_group` → `tg_id=<n>,username=<u>`
     - `rss` → `feed_url=<url>`
     - `html` → `url=<url>`
     - не выставлено → пустая строка.
   - `CREATED_AT` — `Source.created_at` в RFC3339 (UTC).
9. **SR-09** `source create` и `source update` — те же колонки SR-08, одна строка с созданным/обновлённым `Source`.
10. **SR-10 Формат `<id>`.** `Source.id` — UUID v4 в canonical form (`8-4-4-4-12` hex lowercase). Рендерится во всех форматах без кавычек; YAML-рендер также выводит без кавычек (canonical UUID не совпадает с YAML-литералами `true`/`false`/`null`/числами). Snapshot-тесты (AC-S4) используют фиксированный UUID `00000000-0000-4000-8000-000000000001`.

## Edge cases

- **EC-A `source get <id>` → `NOT_FOUND`.** Stderr: `code=NotFound message=<...>` (core NFR-03), exit `1`.
- **EC-B Таймаут RPC.** Ответ не пришёл за `--timeout`. Stderr: `code=DeadlineExceeded message=context deadline exceeded`, exit `1`.
- **EC-C Пустой список.** `V1ListSources.items=[]` → `table`: только заголовок; `json`: `[]\n`; `yaml`: `[]\n`. Exit `0`.
- **EC-D Отсутствие required-флага.** `feediumctl source create rss` без `--feed-url` → cobra ошибка, stderr (NFR-03), exit `1`. RPC не выполняется.
- **EC-E Неизвестный `<type>`.** `feediumctl source create ftp ...` → stderr: `flag: unknown source type "ftp" (allowed: telegram-channel,telegram-group,rss,html)`, exit `1`. RPC не выполняется.
- **EC-F `source delete <unknown-id>`.** Сервер отвечает `NOT_FOUND` → как EC-A.
- **EC-G `source list --type=<unknown>`.** Если `--type` передан и значение не совпадает ни с одним валидным `SourceType` (без префикса) — exit `1`, stderr `flag: unknown --type "..."`. RPC не выполняется.
- **EC-H Сервер вернул `Source` с невыставленным `SourceConfig.config` (oneof пуст).** В `table` колонка `CONFIG` пустая; в `json`/`yaml` поле `config` опускается (`EmitUnpopulated=false`, core FR-10).
- **EC-I Флаг, не принадлежащий указанному `<type>`.** Например, `feediumctl source create rss --tg-id=42 --feed-url=https://...` или `feediumctl source update <id> --type=rss --username=foo`. Exit `1`, stderr: `flag: --<name> is not allowed for type "<type>"`. RPC не выполняется. (См. SR-06, таблица SR-03.)
- **EC-J Прочие gRPC status codes.** Любой статус, отличный от `OK`, `NotFound` (EC-A, EC-F) и `DeadlineExceeded` (EC-B), — рендерится generic-обработчиком core NFR-03 (`code=<Name> message=<...>`), exit `1`. Отдельные тесты на CLI-уровне не требуются (покрытие — ответственность core).

## Acceptance Criteria

- [ ] **AC-S1** `feediumctl source list --output=json` при работающем сервере → stdout валидный JSON-массив (возможно пустой), exit `0`.
- [ ] **AC-S2** При выключенном сервере `feediumctl source list` → exit `1`, stderr соответствует core EC-A, stdout пуст.
- [ ] **AC-S3** Каждая из `source get`, `source create` (для каждого из 4 типов), `source update`, `source delete` корректно формирует `V1*Request` (unit-тест на mock gRPC-клиенте) и рендерит ответ в `table|json|yaml`.
- [ ] **AC-S4** Для `source delete` все три формата вывода соответствуют SR-05 байт-в-байт (snapshot-тест).
- [ ] **AC-S5** Unit-тесты покрывают: маппинг `<type>` → `SourceType` + oneof для всех 4 типов; рендеринг enum `SourceType`/`ProcessingMode` без префиксов и для `UNSPECIFIED`; пустой `SourceConfig.config` (EC-H); пустой `items` (EC-C); ошибки `NotFound` и `DeadlineExceeded` (EC-A, EC-B); конфликт флагов с `<type>` (EC-I, по одному кейсу на `create` и `update`); неизвестный `<type>` (EC-E) и неизвестный `--type` для `list` (EC-G).
- [ ] **AC-S6** `feediumctl source --help` → exit `0`, usage с подкомандами `list|get|create|update|delete`.
- [ ] **AC-S7** Ручная верификация `CHK-01` воспроизведена; логи (stdout, stderr, exit codes) для happy- и error-сценариев сохранены в `artifacts/ft-006/verify/chk-01/` как `EVID-01`.

## Инварианты и ограничения

Применяются все INV-01..INV-06 и CON-01..CON-05 из [`spec.md`](./spec.md) без изменений. Дополнительных инвариантов в этом документе нет.
