---
title: "FT-006: CLI Client"
doc_kind: feature
doc_function: canonical
purpose: "Короткий canonical feature-документ: CLI-клиент `feediumctl`, вызывающий feedium через gRPC API."
derived_from:
  - ../../domain/problem.md
  - ../../prd/PRD-001-mvp.md
  - ../../engineering/api-contracts.md
status: draft
delivery_status: planned
---

# FT-006: CLI Client

## What

### Problem

У Feedium уже есть gRPC/HTTP API (`api/feedium/`: `health`, `source`, `post`, `summary`), но нет типизированного программного клиента. Сейчас каждая ops-операция требует curl/grpcurl с ручной сборкой payload либо кода на Go, что тормозит отладку и batch-автоматизацию поверх MVP API.

### Scope

- `REQ-01` Бинарь `feediumctl` в `cmd/feediumctl/`, собираемый общим `Makefile`.
- `REQ-02` Команды вызывают gRPC-методы сервисов `health` и `source` через сгенерированные `api/feedium/*_grpc.pb.go`.
- `REQ-03` Endpoint и формат вывода (`table|json|yaml`) задаются через CLI-флаги и env.

### Non-Scope

- `NS-01` Покрытие сервисов `post` и `summary` — отдельные фичи FT-007+.

### Constraints

- `CON-01` CLI обращается только к публичным gRPC-контрактам из `api/feedium/` и не импортирует `internal/biz`, `internal/data`, `internal/service`.

## How

### Solution

`feediumctl` живёт в том же репо и `go.mod`, что и сервер, импортирует сгенерированные gRPC-клиенты напрямую и собирается общим `Makefile`. Trade-off: клиент и сервер всегда одной версии (проще, но релиз-цикл у них общий).

### Change Surface

| Surface | Why |
| --- | --- |
| `cmd/feediumctl/` | Новая точка входа CLI. |
| `Makefile` | Цель сборки `feediumctl`. |

### Flow

1. Пользователь запускает `feediumctl <group> <command> [flags]`.
2. CLI резолвит endpoint, открывает gRPC-соединение, вызывает нужный метод.
3. Ответ форматируется в выбранный формат и пишется в stdout; ошибки — в stderr с non-zero exit code.

## Verify

### Exit Criteria

- `EC-01` `feediumctl health` и `feediumctl source list` возвращают корректный ответ при работающем сервере и non-zero exit code с сообщением в stderr при недоступном.

### Acceptance Scenarios

- `SC-01` При запущенном feedium-сервере `feediumctl health` завершает работу с exit code 0 и печатает ответ gRPC-метода `Health` в формате по умолчанию; `feediumctl source list --output=json` печатает валидный JSON с массивом источников.

### Traceability matrix

| Requirement ID | Design refs | Acceptance refs | Checks | Evidence IDs |
| --- | --- | --- | --- | --- |
| `REQ-01` | `CON-01` | `EC-01`, `SC-01` | `CHK-01` | `EVID-01` |
| `REQ-02` | `CON-01` | `EC-01`, `SC-01` | `CHK-01` | `EVID-01` |
| `REQ-03` | `CON-01` | `EC-01`, `SC-01` | `CHK-01` | `EVID-01` |

### Checks

| Check ID | Covers | How to check | Expected |
| --- | --- | --- | --- |
| `CHK-01` | `EC-01`, `SC-01` | Запустить feedium-сервер, выполнить `feediumctl health` и `feediumctl source list --output=json`, затем остановить сервер и повторить. | При работающем сервере — exit 0 и валидный вывод; при недоступном — non-zero exit и сообщение об ошибке в stderr. |

### Test matrix

| Check ID | Evidence IDs | Evidence path |
| --- | --- | --- |
| `CHK-01` | `EVID-01` | `artifacts/ft-006/verify/chk-01/` |

### Evidence

- `EVID-01` Лог выполнения `CHK-01`: stdout/stderr команд и exit codes для happy- и error-сценариев.

### Evidence contract

| Evidence ID | Artifact | Producer | Path contract | Reused by checks |
| --- | --- | --- | --- | --- |
| `EVID-01` | Лог команд и exit codes | verify-runner / human | `artifacts/ft-006/verify/chk-01/` | `CHK-01` |
