The document is verified and grounded — all `path:line` references confirmed against actual code, template choice (`large.md`) justified, full traceability with no empty cells, no blacklisted vague words. Per the output contract, here is the final `feature.md` body:

---

```markdown
---
title: "FT-006-cli-client: Read-only CLI client (feediumctl)"
doc_kind: feature
doc_function: canonical
purpose: "Canonical feature-документ: read-only CLI feediumctl, дающий типизированный доступ ко всем четырём MVP-сервисам Feedium (health, source, post, summary) через публичные gRPC-контракты."
derived_from:
  - ../../domain/problem.md
  - ../../prd/PRD-001-mvp.md
  - ../../engineering/api-contracts.md
status: draft
delivery_status: planned
---

# FT-006-cli-client: Read-only CLI client (feediumctl)

## What

> Template: `large.md`. Rationale: фича вводит/расширяет CLI-контракт и затрагивает несколько поверхностей (`api/feedium` × 4 сервиса, новые группы команд, render/transport), требует `DEC-*`, `CTR-*`, `FM-*` и более одного `SC-*`/`CHK-*` — все условия `short.md` нарушены ([feature-flow §Выбор шаблона](../../flows/feature-flow.md#выбор-шаблона-featuremd)).

### Problem

У Feedium есть gRPC/HTTP API (`api/feedium/`: `health`, `source`, `post`, `summary`), но типизированный программный клиент покрывает его лишь частично. Текущий бинарь `cmd/feediumctl/` регистрирует только две группы команд — `health` и `source` ([`cmd/feediumctl/internal/app/root.go:32-33`](../../../cmd/feediumctl/internal/app/root.go)); `post` и `summary` недоступны. Любая read- или debug-операция по `post`/`summary` сегодня требует `grpcurl`/`curl` с ручной сборкой payload либо одноразового кода на Go.

Это feature-specific delta относительно [PRD-001](../../prd/PRD-001-mvp.md): на момент исходной попытки FT-006 API `post`/`summary` ещё не были готовы и выносились в «FT-007+». Сейчас FT-004/005/007/009 завершены, контракты `V1ListPosts`/`V1GetPost` ([`api/feedium/post.proto:115-123`](../../../api/feedium/post.proto)) и `V1ListPostSummaries`/`V1ListSourceSummaries`/`V1GetSummary` ([`api/feedium/summary.proto:97-104`](../../../api/feedium/summary.proto)) доступны — поэтому CLI расширяется до read-доступа ко всем четырём сервисам сразу.

### Outcome

| Metric ID | Metric | Baseline | Target | Measurement method |
| --- | --- | --- | --- | --- |
| `MET-01` | MVP-сервисы с read-доступом (health/list/get) через `feediumctl` | 2 из 4 (`health`, `source` — `root.go:32-33`) | 4 из 4 (`health`, `source`, `post`, `summary`) | Инвентаризация зарегистрированных команд + `EVID-01` |
| `MET-02` | Ручная сборка gRPC/HTTP payload для read-операций по `post`/`summary` | Каждая операция (`grpcurl`/`curl`) | 0 операций (одна команда `feediumctl`) | Прогон `CHK-01` против живого сервера, `EVID-01` |

### Scope

- `REQ-01` `feediumctl` выполняет проверку здоровья сервиса: команда вызывает `HealthService.V1Check` и при работающем сервере завершается с exit code 0, печатая ответ.
- `REQ-02` `feediumctl` даёт read-операции list и get для каждого из сервисов `source`, `post`, `summary` через сгенерированные gRPC-клиенты из `api/feedium/`.
- `REQ-03` Для каждой команды доступен машиночитаемый формат вывода: при его выборе результат — валидный структурированный вывод, парсимый в скрипте без дополнительной нормализации.
- `REQ-04` Контракт завершения: при недоступном или ответившем ошибкой сервере команда завершается с ненулевым exit code и пишет человекочитаемое сообщение в stderr; при успехе — exit code 0, результат в stdout.
- `REQ-05` `feediumctl` остаётся одним бинарём в `cmd/feediumctl/`, живущим в общем `go.mod` (`github.com/4itosik/feedium`) и собираемым существующей целью `Makefile` `feediumctl`.

### Non-Scope

- `NS-01` Любые мутации: создание/изменение/удаление источников и постов, запуск суммаризации. CLI на этом этапе только читает (см. `DEC-01` про судьбу уже существующих мутирующих команд `source`).
- `NS-02` Аутентификация и авторизация сверх той, что уже предоставляет API.
- `NS-03` Изменение серверных контрактов: proto-файлы `api/feedium/`, методы сервисов, схемы БД. CLI только потребляет существующие контракты, не вводит новых.
- `NS-04` Выбор CLI-фреймворка, точного набора флагов, способа резолва endpoint и набора форматов вывода заново не пересматривается: переиспользуются уже зафиксированные в `cmd/feediumctl/` конвенции (`ASM-01`); их ревизия вне scope.

### Constraints / Assumptions

- `ASM-01` `cmd/feediumctl/` уже реализует cobra-based CLI с группами `health`/`source` и глобальными флагами `--endpoint`, `--output table|json|yaml`, `--timeout`, `--page-size`, `--config` ([`root.go:26-33`](../../../cmd/feediumctl/internal/app/root.go)), плюс слои `internal/transport` (gRPC dial), `internal/render`, `internal/resolve`, `internal/config` и mock-based тесты. Фича расширяет эти конвенции, а не вводит новые.
- `CON-01` CLI обращается только к публичным gRPC-контрактам из `api/feedium/` и не импортирует `internal/biz`, `internal/data`, `internal/service`.
- `CON-02` Соблюдаются `PCON-*` из [`domain/problem.md`](../../domain/problem.md): Go 1.25+, kratos-layout, общий модуль и `go.mod` с сервером; CLI — server-side инструмент для macOS/Linux dev, не desktop-only.
- `DEC-01` Существующие мутирующие команды группы `source` — `create`/`update`/`delete` ([`source.go:177,258,337`](../../../cmd/feediumctl/internal/app/source.go)) — противоречат read-only scope (`NS-01`). Блокирует Plan Ready: нужно решение — удалить их в рамках FT-006 или вынести в отдельную follow-up фичу. До разрешения план не фиксирует change set для группы `source`.

## How

### Solution

`feediumctl` живёт в том же репозитории и `go.mod`, что и сервер, импортирует сгенерированные gRPC-клиенты (`api/feedium/*_grpc.pb.go`) напрямую и расширяет существующее cobra-дерево команд группами `post` и `summary` плюс read-командами (list/get) поверх уже имеющихся `health`/`source`. Ответ форматируется в выбранный формат (`--output`) и пишется в stdout; ошибки соединения и gRPC-статусы пишутся в stderr с ненулевым exit code. Ключевой trade-off: клиент и сервер всегда одной версии и релиз-цикла (проще, единый источник контрактов, но клиент нельзя релизить независимо).

### Change Surface

| Surface | Type | Why it changes |
| --- | --- | --- |
| `cmd/feediumctl/internal/app/root.go` | code | Регистрация новых групп команд `post` и `summary` (`REQ-02`). |
| `cmd/feediumctl/internal/app/` (новые `post.go`, `summary.go`) | code | Read-команды list/get для `post` и `summary` поверх gRPC-клиентов (`REQ-02`, `REQ-04`). |
| `cmd/feediumctl/internal/app/source.go` | code | Приведение группы `source` к read-only (зависит от `DEC-01`). |
| `cmd/feediumctl/internal/transport/grpc.go` | code | Подключение клиентов `PostService`/`SummaryService` (`REQ-02`). |
| `cmd/feediumctl/internal/render/` (новые рендереры) | code | Человеко- и машиночитаемый вывод для `post`/`summary` (`REQ-03`). |
| `cmd/feediumctl/internal/app/mock/` | code | Моки клиентов `post`/`summary` для тестов (verify-слой). |
| `Makefile` (цель `feediumctl`) | config | Уже существует (`Makefile:6-7`); подтверждается без изменения как точка сборки (`REQ-05`). |

### Flow

1. Пользователь запускает `feediumctl <group> <command> [flags]`, где `group ∈ {health, source, post, summary}`.
2. CLI резолвит endpoint, output-формат, timeout и page-size (флаги → env → config-файл, по существующим правилам `internal/resolve`/`internal/config`), открывает gRPC-соединение и вызывает соответствующий read-метод сервиса.
3. При успехе ответ форматируется в выбранный формат и пишется в stdout с exit code 0; при недоступном сервере или gRPC-ошибке сообщение пишется в stderr, exit code — ненулевой.

### Contracts

| Contract ID | Input / Output | Producer / Consumer | Notes |
| --- | --- | --- | --- |
| `CTR-01` | `HealthService.V1Check` | сервер пишет / `feediumctl health` читает | [`api/feedium/health.proto:10`](../../../api/feedium/health.proto). |
| `CTR-02` | `V1ListSources` / `V1GetSource` | сервер / `feediumctl source list|get` | [`api/feedium/source.proto:126,132`](../../../api/feedium/source.proto). |
| `CTR-03` | `V1ListPosts` / `V1GetPost` | сервер / `feediumctl post list|get` | [`api/feedium/post.proto:121`](../../../api/feedium/post.proto) (`V1ListPosts`), [`:115`](../../../api/feedium/post.proto) (`V1GetPost`). |
| `CTR-04` | `V1ListPostSummaries` / `V1ListSourceSummaries` / `V1GetSummary` | сервер / `feediumctl summary list|get` | [`api/feedium/summary.proto:97-104`](../../../api/feedium/summary.proto). Какой из двух list-методов и нужен ли `V1GetSummaryEvent` (`summary.proto:94`) — см. `## Open Questions` OQ-01. |
| `CTR-05` | Машиночитаемый вывод (например `--output json`) | `feediumctl` пишет / скрипт читает | Структура стабильна и парсима без пост-обработки; диагностика и ошибки не смешиваются с данными в stdout. |

### Failure Modes

- `FM-01` Сервер недоступен (соединение не устанавливается / таймаут): команда завершается ненулевым exit code, сообщение в stderr, в stdout нет частичного «успешного» вывода.
- `FM-02` Сервер ответил gRPC-статусом ошибки (`NOT_FOUND`, `INVALID_ARGUMENT`, `INTERNAL` — см. [`error_reason.proto`](../../../api/feedium/error_reason.proto)): команда завершается ненулевым exit code, человекочитаемое сообщение об ошибке в stderr.
- `FM-03` Невалидные аргументы команды (отсутствует обязательный `id` для get, неизвестный формат вывода): команда завершается ненулевым exit code и сообщением в stderr до открытия соединения.

## Verify

`Verify` задаёт canonical test case inventory для FT-006: positive scenarios через `SC-*`, negative coverage через `NEG-*`, executable checks через `CHK-*` и evidence через `EVID-*`.

### Exit Criteria

- `EC-01` При работающем сервере `feediumctl health` → exit 0 + ответ; list и get для `source`, `post`, `summary` → exit 0 + корректные данные.
- `EC-02` При выборе машиночитаемого формата вывод list и get — валидный структурированный результат, который парсится скриптом.
- `EC-03` При недоступном или ответившем ошибкой сервере любая команда → ненулевой exit code + сообщение в stderr.
- `EC-04` Поведение покрыто автоматическими тестами; `go test ./cmd/feediumctl/...` и `make lint` зелёные локально и в CI.

### Acceptance Scenarios

- `SC-01` (happy) При запущенном feedium-сервере `feediumctl health` завершается с exit code 0 и печатает ответ `V1Check` в формате по умолчанию.
- `SC-02` (happy) При запущенном сервере `feediumctl source list`, `feediumctl post list`, `feediumctl summary list` и соответствующие `get <id>` возвращают корректные данные с exit code 0.
- `SC-03` (nfr) При `--output json` вывод list и get по каждому из `source`/`post`/`summary` — валидный JSON, успешно разбираемый парсером (например `jq`), без диагностики в stdout.
- `SC-04` (error) При недоступном сервере любая команда (`health`, `source list`, `post get`, …) завершается с ненулевым exit code и сообщением об ошибке в stderr.
- `SC-05` (error) `feediumctl <group> get <несуществующий-id>` при работающем сервере, вернувшем `NOT_FOUND`, завершается ненулевым exit code и человекочитаемым сообщением в stderr.
- `SC-06` (edge) `list` по пустой коллекции завершается с exit code 0 и валидным структурированным выводом (пустой список), парсимым в машиночитаемом формате.

### Traceability matrix

| Requirement ID | Design refs | Acceptance refs | Checks | Evidence IDs |
| --- | --- | --- | --- | --- |
| `REQ-01` | `CTR-01`, `CON-01`, `ASM-01` | `EC-01`, `SC-01` | `CHK-01` | `EVID-01` |
| `REQ-02` | `CTR-02`, `CTR-03`, `CTR-04`, `CON-01`, `DEC-01` | `EC-01`, `SC-02`, `SC-05` | `CHK-01`, `CHK-03` | `EVID-01`, `EVID-02` |
| `REQ-03` | `CTR-05`, `FM-01`, `FM-02` | `EC-02`, `SC-03`, `SC-06` | `CHK-01`, `CHK-03` | `EVID-01`, `EVID-02` |
| `REQ-04` | `FM-01`, `FM-02`, `FM-03` | `EC-03`, `SC-04`, `SC-05` | `CHK-02`, `CHK-03` | `EVID-01`, `EVID-02` |
| `REQ-05` | `ASM-01`, `CON-02` | `EC-01`, `EC-04`, `SC-01` | `CHK-01`, `CHK-04` | `EVID-01`, `EVID-03` |

### Checks

| Check ID | Covers | How to check | Expected result | Evidence path |
| --- | --- | --- | --- | --- |
| `CHK-01` | `EC-01`, `EC-02`, `SC-01`, `SC-02`, `SC-03`, `SC-06` | Поднять feedium-сервер; выполнить `feediumctl health`, затем list+get для `source`/`post`/`summary` в формате по умолчанию и с `--output json`; распарсить JSON. | exit 0; корректные данные; JSON валиден и парсится; пустой list даёт валидный пустой результат. | `artifacts/ft-006/verify/chk-01/` |
| `CHK-02` | `EC-03`, `SC-04`, `SC-05`, `FM-01`, `FM-02` | Выполнить команды при остановленном сервере; затем `get <несуществующий-id>` при работающем сервере. | Ненулевой exit code; сообщение в stderr; stdout без частичного успеха. | `artifacts/ft-006/verify/chk-02/` |
| `CHK-03` | `EC-04`, `REQ-02`, `REQ-03`, `REQ-04` | `go test ./cmd/feediumctl/...` | Все тесты проходят; покрыты read-команды `post`/`summary` и контракт ошибок. | `artifacts/ft-006/verify/chk-03/` |
| `CHK-04` | `EC-04`, `REQ-05` | `make feediumctl` и `make lint` | Бинарь собирается; lint зелёный. | `artifacts/ft-006/verify/chk-04/` |

### Test matrix

| Check ID | Evidence IDs | Evidence path |
| --- | --- | --- |
| `CHK-01` | `EVID-01` | `artifacts/ft-006/verify/chk-01/` |
| `CHK-02` | `EVID-01` | `artifacts/ft-006/verify/chk-02/` |
| `CHK-03` | `EVID-02` | `artifacts/ft-006/verify/chk-03/` |
| `CHK-04` | `EVID-03` | `artifacts/ft-006/verify/chk-04/` |

### Negative

- `NEG-01` `get <несуществующий-id>` → gRPC `NOT_FOUND` → ненулевой exit code + сообщение в stderr (покрывает `FM-02`, питает `SC-05`).
- `NEG-02` Команда без обязательного аргумента или с неизвестным `--output` → ненулевой exit code + сообщение в stderr до открытия соединения (покрывает `FM-03`).
- `NEG-03` Сервер недоступен → ненулевой exit code + сообщение в stderr, stdout пуст (покрывает `FM-01`, питает `SC-04`).

### Evidence

- `EVID-01` Лог прогона `CHK-01`/`CHK-02` против живого сервера: stdout/stderr команд и их exit codes для happy-, error- и edge-сценариев.
- `EVID-02` Вывод `go test ./cmd/feediumctl/...` с результатами тестов read-команд.
- `EVID-03` Лог `make feediumctl` и `make lint`.

### Evidence contract

| Evidence ID | Artifact | Producer | Path contract | Reused by checks |
| --- | --- | --- | --- | --- |
| `EVID-01` | Лог команд и exit codes (happy/error/edge) | verify-runner / human | `artifacts/ft-006/verify/chk-01/`, `artifacts/ft-006/verify/chk-02/` | `CHK-01`, `CHK-02` |
| `EVID-02` | Лог `go test ./cmd/feediumctl/...` | verify-runner / CI | `artifacts/ft-006/verify/chk-03/` | `CHK-03` |
| `EVID-03` | Лог сборки и линта | verify-runner / CI | `artifacts/ft-006/verify/chk-04/` | `CHK-04` |

## Open Questions

- `OQ-01` Маппинг команд группы `summary` на конкретные методы: `summary list` поверх `V1ListPostSummaries` ([`summary.proto:97`](../../../api/feedium/summary.proto)) и/или `V1ListSourceSummaries` (`summary.proto:100`); нужен ли отдельный доступ к `V1GetSummaryEvent` (`summary.proto:94`) помимо `V1GetSummary` (`summary.proto:103`). Не блокирует Design Ready (capability read-list/get зафиксирована в `REQ-02`); уточняется на Design-фазе.
```