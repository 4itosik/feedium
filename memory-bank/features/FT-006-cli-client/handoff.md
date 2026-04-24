---
title: "FT-006: CLI Client — Handoff (Core → Source)"
doc_kind: feature
doc_function: handoff
purpose: "Сводка состояния после реализации каркаса и `health` (implement.md). Точка входа для автора следующего плана — source-команды по spec-source.md."
derived_from:
  - spec.md
  - spec-source.md
  - implement.md
status: draft
---

# Handoff: Core CLI → Source commands

Этот документ фиксирует состояние после завершения `implement.md` (каркас `feediumctl` + команда `health`) и даёт следующему плану (`implementation-plan` по [`spec-source.md`](./spec-source.md)) точку входа: что уже собрано, что переиспользовать, какие контракты нельзя ломать, какие места потребуют решений.

## 1. Что уже реализовано

Состояние закреплено на ветке `feature/ft-006-cli-client` (worktree `.worktrees/ft-006-cli-client`). Все шаги `implement.md` (1–13) выполнены, `go test ./cmd/feediumctl/... -race` зелёный.

### 1.1 Покрытые требования spec.md

- **FR-01, FR-02..07, FR-08..11** — реализованы в пределах команды `health`.
- **NFR-01..06** — соблюдены (в том числе одна top-level новая зависимость `spf13/cobra` + допустимая `sigs.k8s.io/yaml`).
- **INV-01..06** — проверены unit-тестами и smoke-сценариями.
- **AC-01..07** — автоматическая часть (AC-03/05/06) покрыта тестами; ручная часть (AC-01/02/04/07) воспроизведена локально.

### 1.2 Созданные пакеты

```
cmd/feediumctl/
├── main.go                          # entry point + exit-логика
└── internal/
    ├── app/                         # cobra commands + единый репортёр ошибок
    │   ├── root.go                  # root cmd + 5 persistent-флагов
    │   ├── health.go                # sub-command health + runHealth pipeline
    │   ├── settings.go              # loadConfig / flagSourceFrom / osGetenv
    │   ├── errors.go                # FormatRPCError + WrapRPCError
    │   └── *_test.go, export_test.go
    ├── config/                      # YAML loader (FR-06 / EC-D/E/G)
    ├── resolve/                     # flag > env > config > default (INV-05)
    │   ├── resolve.go
    │   └── output.go                # ValidateOutput (FR-04 / EC-B)
    ├── render/                      # protojson + yaml.JSONToYAML + table
    └── transport/                   # insecure gRPC dial + endpoint-санити
```

## 2. Переиспользование для source-команд

Source-команды НЕ должны дублировать каркас. Ниже — публичные точки, на которые опирается следующая реализация.

### 2.1 Разрешение настроек (Step 5 / INV-05)

```go
settings, err := resolveSettings(rootCmd, globalFlags)
// settings.Endpoint, settings.Output, settings.Timeout, settings.PageSize
```

`resolveSettings` — непубличная функция внутри `app`, но её вызов уже «зашит» в `runHealth`. Для source-команд повторять ту же цепочку: `loadConfig → flagSourceFrom → resolve.Resolve → ValidateOutput`. Вынос в отдельный helper `app.resolveAndValidate` — задача следующего плана (мелкий рефакторинг, не расширение scope).

### 2.2 Транспорт (Step 8)

```go
conn, err := transport.Dial(settings.Endpoint)
defer conn.Close()
client := feediumapi.NewSourceServiceClient(conn)
```

Санити-проверка endpoint (`empty`, whitespace) уже внутри `transport.Dial`. `endpoint:`-префикс ошибок — ответственность `Dial`; не добавлять свой.

### 2.3 Рендер (Steps 9, 10)

Экспортированная сигнатура:

```go
render.Write(w io.Writer, format string, msg proto.Message) error
```

- `json` и `yaml` уже детерминированы и снабжены ровно одним trailing `\n` — ничего не менять.
- `table` в `render.go` **явно diskriminates по типу** (`case *feediumapi.V1CheckResponse`). Для source-команд потребуется добавить ветки для:
  - `*feediumapi.V1ListSourcesResponse` — вывод шапки + строк (SR-08).
  - `*feediumapi.V1GetSourceResponse`, `*V1CreateSourceResponse`, `*V1UpdateSourceResponse` — одна строка c теми же колонками (SR-09).
  - `*feediumapi.V1DeleteSourceResponse` — см. п. 3.3 ниже, table-форма жёстко специфицирована (`deleted: <id>`).
- Renderer — это единственная точка знания про форматы. Источники сортировок (CONFIG, CREATED_AT) оформить там же.

### 2.4 Ошибки (Step 7)

- Локальные ошибки — одна строка `<prefix>: <reason>`. Закрытый список префиксов — `config`, `flag`, `output`, `endpoint`. **Любой новый префикс — это изменение spec.md (NFR-03).**
- RPC-ошибки — через `app.WrapRPCError(err)`; форматирование `code=<Name> message=<msg>` централизовано в `FormatRPCError`.
- Для source-команд ничего нового добавлять не нужно: `NotFound`, `DeadlineExceeded`, `Unavailable` и прочие — уже покрыты unit-тестами на форматтер.

### 2.5 Инжекция клиента для тестов

`export_test.go` в пакете `app` показывает паттерн stub-клиента без сетевого соединения. Для source-команд завести аналогичные `StubSourceFactory` / `NewRootCommandWithSource` — не изобретать собственный DI.

## 3. Контракты, которые нельзя ломать

### 3.1 Поведение `--help`

`--help` и `<subcmd> --help` обязаны:
- выйти с кодом `0`;
- не открывать gRPC-соединение;
- не вызывать RPC.

`SilenceUsage: true` + `SilenceErrors: true` на всех командах. При ошибке разбора аргументов — единственный вывод в stderr формируется в `main.go`.

### 3.2 `stdout` пуст при ошибке (INV-02)

Любая команда на ошибке:
- stderr — ровно одна строка без stacktrace;
- stdout — пусто;
- exit — `1`.

Source-команды не должны частично печатать таблицу перед ошибкой. Для `source list` это означает: накопить список items в памяти, только после успеха пойти в renderer.

### 3.3 Жёсткий вывод `source delete`

[`spec-source.md` SR-05](./spec-source.md) фиксирует byte-exact вывод для всех трёх форматов. Это один из двух snapshot-тестов (AC-S4). Renderer для `V1DeleteSourceResponse` не должен зависеть от общих protojson-опций, иначе нарушится INV-06 и детерминизм snapshot'а. Держать отдельную ветку в `render.writeTable/JSON/YAML` или ввести специальный тип `deletePayload{ID string}` в пакете `app` и рендерить его общими средствами — решить в плане.

### 3.4 Порядок приоритетов

`flag > env > config > default` живёт в одном месте — `resolve.Resolve`. Source-команды **не** должны добавлять свои env-переменные или локальные конфиг-поля: весь закрытый список ключей конфига (`endpoint`, `output`, `timeout`, `page_size`) уже исчерпан spec.md FR-06. Любое расширение — правка spec.

### 3.5 Изоляция от server-side

```
grep -R "internal/\(biz\|data\|service\)" cmd/feediumctl/
```

— должен оставаться пустым (INV-01, AC-05). В частности, не импортировать `internal/conf` ради удобства парсинга — у CLI собственный mini-loader.

## 4. Что следующий план должен решить (Open Items)

| # | Вопрос | Комментарий |
|---|---|---|
| OI-1 | Маппинг позиционного `<type>` и oneof | `SR-03` фиксирует 4 значения (`telegram-channel`, `telegram-group`, `rss`, `html`) → `SourceType` и одноимённый вариант `SourceConfig`. Рекомендуется отдельный пакет `cmd/feediumctl/internal/sourcetype/` со справочником + валидатор required-флагов. |
| OI-2 | Общий helper `resolveAndValidate` | Сейчас код повторяется только в `runHealth`; при появлении 5+ RPC дубликат станет заметен. Вынести в `cmd/feediumctl/internal/app/pipeline.go` до реализации source-команд. |
| OI-3 | Рендер enum-значений без префиксов | `SourceType`/`ProcessingMode` (SR-08) требуют специальной логики (короткое имя + fallback `UNSPECIFIED`). Кандидат — отдельные `String()`-хелперы рядом с `render.go` (не в сгенерированном `.pb.go`). |
| OI-4 | `CREATED_AT` в RFC3339 UTC | [`spec-source.md` SR-08](./spec-source.md). Перевод `*timestamppb.Timestamp` → UTC должен происходить внутри renderer'а: ещё один аргумент в пользу инкапсуляции всей логики форматов в `render`. |
| OI-5 | CLI-side валидация запрещённых флагов по `<type>` (EC-I) | Ни cobra, ни proto такую проверку не обеспечивают; нужна явная таблица в `sourcetype/`. |
| OI-6 | Snapshot-тест AC-S4 | Технически — текстовый фикстур в `testdata/`. Решить в плане: по одному файлу на формат или один combined файл. |
| OI-7 | Нумерация шагов | `implementation-plan.md` по spec-source.md должен идти как отдельный файл или как новая глава к существующему `implement.md` — согласовать с автором feature-flow (OQ-04 в `implement.md`). |

## 5. Диагностика и проверки

Команды, пригодные для быстрой регрессии каркаса перед стартом source-плана:

```bash
# на ветке feature/ft-006-cli-client
go test ./cmd/feediumctl/... -race
go vet ./...
make feediumctl && ./bin/feediumctl --help                # AC-04
./bin/feediumctl --output=xml health                      # EC-B
./bin/feediumctl --timeout=abc health                     # flag:
./bin/feediumctl --config=/missing.yaml health            # EC-G
./bin/feediumctl health                                   # EC-A (сервер выключен)
grep -R "internal/\(biz\|data\|service\)" cmd/feediumctl/ # AC-05, пусто
```

Живой сервер (`make build && ./bin/feedium -conf configs/`) — для AC-01 и AC-07.

## 6. Риски перед следующим планом

- **R-1 Сложность table-рендера.** Пять подкоманд + `SourceConfig.config` oneof + форматирование enum. Без вынесения table-логики в отдельные функции пакета `render` пойдёт copy-paste. Решать структурно в первом же шаге плана.
- **R-2 Недетерминизм сортировки `items`.** SR-01 говорит «CLI не пересортировывает». Тесты должны фиксировать порядок `items`, полученный от mock-сервера. Любая перестраховочная сортировка на клиенте — баг.
- **R-3 YAML и `deleted: true`.** SR-05 фиксирует, что YAML-рендер для delete должен выдать `deleted: true\nid: <uuid>\n` без кавычек. Проверить, что `yaml.JSONToYAML` не добавит кавычек вокруг UUID (SR-10 явно ссылается на canonical UUID v4). Если добавит — нужен отдельный writer для delete.
- **R-4 Сохранение поведения `health` при рефакторинге.** После выноса `pipeline.go` (OI-2) прогнать все существующие тесты на `app` без изменений — они покрывают то, что ломать нельзя.

## 7. Где лежит state

| Артефакт | Путь |
|---|---|
| Реализация | `cmd/feediumctl/` (ветка `feature/ft-006-cli-client`) |
| Ветка | `feature/ft-006-cli-client` (ещё не закоммичена, работает в `.worktrees/ft-006-cli-client`) |
| Спека ядра | [`spec.md`](./spec.md) |
| Спека source | [`spec-source.md`](./spec-source.md) |
| План ядра (выполнен) | [`implement.md`](./implement.md) |
| Следующий план (TODO) | `implementation-plan-source.md` или продолжение `implement.md` — см. OI-7 |

Следующий шаг автора: прочитать этот handoff → прочитать `spec-source.md` → написать план по feature-flow на `implementation-plan`.
