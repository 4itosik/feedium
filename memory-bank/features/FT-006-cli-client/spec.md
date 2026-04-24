---
title: "FT-006: CLI Client — Spec (Core: setup + health)"
doc_kind: feature
doc_function: spec
purpose: "Каркас CLI `feediumctl`: сборка, конфигурация, форматы вывода, транспорт, общая обработка ошибок и команда `health`. Source-команды — в `spec-source.md`."
derived_from:
  - feature.md
status: draft
---

# FT-006: CLI Client — Spec (Core)

## Цель

Поставить типизированный CLI-клиент `feediumctl`, заменяющий ручные `curl`/`grpcurl` в ops-сценариях. Этот документ описывает каркас (сборка, конфиг, вывод, ошибки) и команду `health`. Source-команды вынесены в [`spec-source.md`](./spec-source.md).

## Reference

- Brief: [`feature.md`](./feature.md)
- Proto: [`api/feedium/health.proto`](../../../api/feedium/health.proto)
- API Contracts: [`memory-bank/engineering/api-contracts.md`](../../engineering/api-contracts.md)
- Testing Policy: [`memory-bank/engineering/testing-policy.md`](../../engineering/testing-policy.md)
- Coding Style: [`memory-bank/engineering/coding-style.md`](../../engineering/coding-style.md)

## Scope

### Входит

- Бинарь `feediumctl` в `cmd/feediumctl/`, target `make feediumctl`.
- Команда `feediumctl health` — `HealthService.V1Check`.
- CLI-фреймворк `github.com/spf13/cobra`.
- Глобальные флаги/env-переменные: endpoint, output, timeout, page size, config.
- Конфиг-файл YAML (`$FEEDIUMCTL_CONFIG` или `~/.feediumctl.yaml`).
- Форматы вывода: `table` (default), `json`, `yaml`.
- Единая утилита рендеринга в подпакете `cmd/feediumctl/...`.
- Unit-тесты для конфигурации, рендеринга, ошибок (mock gRPC-клиент).

### НЕ входит

- Source-команды (см. [`spec-source.md`](./spec-source.md)).
- Команды для `post` и `summary` сервисов (отдельные фичи FT-007+).
- TLS, аутентификация, metadata-заголовки.
- Интерактивный режим, shell-completion, man-страницы, brew/apt.
- Версионная маршрутизация на `V2*` RPC (см. OQ-01).
- Импорт `internal/biz`, `internal/data`, `internal/service`.
- Использование сгенерированного HTTP-клиента (`*_http.pb.go`).

## Контекст

`feediumctl` живёт в одном репо и `go.mod` с сервером, импортирует сгенерированные `api/feedium/*_grpc.pb.go` напрямую. Это гарантирует одинаковую версию контрактов клиента и сервера (trade-off: общий релиз-цикл).

## Функциональные требования

### Команда `health`

1. **FR-01 `feediumctl health`** вызывает `HealthService.V1Check` с пустым `V1CheckRequest` и выводит `V1CheckResponse.status` в выбранном формате.

### Конфигурация

2. **FR-02 Endpoint.** `--endpoint` > env `FEEDIUMCTL_ENDPOINT` > поле `endpoint` конфига > default `localhost:9000`.
3. **FR-03 Page size.** `--page-size` > env `FEEDIUMCTL_PAGE_SIZE` > `page_size` конфига > default `50`. Передаётся в запросы как есть; верхнюю границу валидирует сервер.
4. **FR-04 Output.** `--output`/`-o` ∈ `{table, json, yaml}`. Приоритет: флаг > env `FEEDIUMCTL_OUTPUT` > `output` конфига > default `table`. Неизвестное значение → exit `1`.
5. **FR-05 Timeout.** `--timeout` (Go `time.Duration`). Приоритет: флаг > env `FEEDIUMCTL_TIMEOUT` > `timeout` конфига > default `1m`. Применяется через `context.WithTimeout` к каждому RPC.
6. **FR-06 Config-файл.** Путь: `--config` > env `FEEDIUMCTL_CONFIG` > `~/.feediumctl.yaml`. Поддерживаемые ключи: `endpoint`, `output`, `timeout`, `page_size`. Поведение по источнику пути:
   - **Дефолтный путь** (`--config`/env не заданы, используется `~/.feediumctl.yaml`): файла нет → silent fallback на env/default, exit определяется RPC. Если `$HOME` не определён или домашняя директория не резолвится — также silent fallback.
   - **Явно указанный путь** (через `--config` или `FEEDIUMCTL_CONFIG`): файла нет → stderr `config: <path>: not found`, exit `1`.
   - **Файл есть, но невалидный YAML или неизвестный ключ** (любой источник пути): stderr `config: <path>: <reason>`, exit `1`.
7. **FR-07 Транспорт.** gRPC без TLS (`insecure.NewCredentials()`) и без metadata-аутентификации. MVP-допущение.

### Вывод (общие правила)

8. **FR-08 Default формат.** Если `--output` не задан ни одним источником — `table`.
9. **FR-09 Table для `health`.** Две колонки `FIELD | VALUE`, одна строка `status | <value>`.
10. **FR-10 JSON/YAML.** Сериализация через `protojson` для JSON; YAML — конвертация из того же protojson-представления через `sigs.k8s.io/yaml.JSONToYAML`. Опции маршалера фиксированы:
    ```go
    protojson.MarshalOptions{
        Multiline:       true,
        Indent:          "  ",
        UseProtoNames:   true,  // snake_case: created_at, next_page_token
        EmitUnpopulated: false, // null-поля опускаются
    }
    ```
    Вывод завершается ровно одним `\n`. Ключи в YAML отсортированы лексикографически (детерминизм — INV-06).
11. **FR-11 Stdout/Stderr/Exit.** Успех → только stdout; ошибка → только stderr. Exit `0` — успех, `1` — любая ошибка (transport, RPC, validation, config, парсинг). gRPC-коды не маппятся на отдельные exit codes (см. NFR-03).

## Нефункциональные требования

1. **NFR-01 Зависимости.** Ровно одна новая dependency верхнего уровня — `github.com/spf13/cobra`. Допускается `sigs.k8s.io/yaml`, если ещё нет в `go.mod`. Любые другие — отдельное обсуждение.
2. **NFR-02 Сборка.** В `Makefile` добавляется target в `.PHONY`:
   ```makefile
   feediumctl:
   	go build -o bin/feediumctl ./cmd/feediumctl
   ```
   Существующий target `build` (бинарь сервера в `bin/feedium`) не модифицируется.
3. **NFR-03 Формат сообщений об ошибках.** Любая ошибка → stderr + exit `1`. Stacktrace в stderr не выводится. Stdout пуст. Точные шаблоны:
   - **RPC-ошибка:** `code=<CodeName> message=<msg>`, где `<CodeName>` — каноническое имя `codes.Code` (`NotFound`, `Unavailable`, `DeadlineExceeded`, `InvalidArgument`, `Internal`, и т.п. — любое значение из `google.golang.org/grpc/codes`), `<msg>` — `status.Message()`.
   - **Локальная ошибка:** одна строка вида `<context>: <reason>`, без переноса строк. `<context>` — префикс из закрытого списка:

     | Префикс    | Класс ошибок                                                          |
     |------------|-----------------------------------------------------------------------|
     | `config`   | Проблемы с конфиг-файлом: парсинг YAML, неизвестные ключи, отсутствие явно указанного файла (FR-06). |
     | `flag`     | Невалидное значение флага/env, не относящегося к `--output`/`--endpoint` (например, `--timeout=abc`, `--page-size=-1`). |
     | `output`   | Только нарушения FR-04 (неизвестное значение `--output`/env/конфига). |
     | `endpoint` | Невалидный формат endpoint на стадии парсинга (до gRPC dial; при dial применяется RPC-шаблон). |

     Иные префиксы использовать запрещено; новая категория ошибок — правка спеки.
4. **NFR-04 Совместимость версий.** CLI и сервер собираются из одного commit. Кросс-версионный запуск не поддерживается.
5. **NFR-05 Логирование.** CLI не использует `slog` сервера для прикладного вывода. Stdout — только полезная нагрузка в выбранном формате.
6. **NFR-06 Детерминированный вывод.** При одинаковом ответе сервера одинаковы: порядок колонок `table`, порядок ключей JSON (как в proto), порядок ключей YAML (лексикографический), разделитель, line endings (`\n`).

## Edge cases (setup)

- **EC-A Сервер недоступен.** Stderr matches regex `^code=Unavailable message=.*` ИЛИ содержит подстроку `connection refused`. Exit `1`.
- **EC-B Невалидный `--output=xml`.** Stderr: `output: invalid value "xml" (allowed: table,json,yaml)`. RPC не выполняется. Exit `1`.
- **EC-C Невалидный endpoint (`--endpoint=::::`).** Stderr: `endpoint: <причина>` или `code=Unavailable …` (зависит от стадии: парсинг/dial). Exit `1`.
- **EC-D Дефолтный конфиг отсутствует.** `~/.feediumctl.yaml` отсутствует, `--config`/env не переданы → CLI работает на env/default без предупреждения, exit определяется RPC. То же поведение, если `$HOME` не задан.
- **EC-E Конфиг-файл невалидный.** Неразбираемый YAML или неизвестный ключ → stderr: `config: <path>: <reason>`, exit `1`.
- **EC-F Несколько источников одного значения.** `--endpoint` и env `FEEDIUMCTL_ENDPOINT` заданы одновременно — побеждает флаг (FR-02). Аналогично для остальных приоритетов FR-03/04/05.
- **EC-G Явно указанный конфиг отсутствует.** `--config=/missing.yaml` или `FEEDIUMCTL_CONFIG=/missing.yaml` при отсутствующем файле → stderr: `config: /missing.yaml: not found`, exit `1`. RPC не выполняется.
- **EC-H Таймаут истёк.** Сервер не отвечает дольше `--timeout` → stderr matches regex `^code=DeadlineExceeded message=.*`, exit `1`, stdout пуст.

## Инварианты

- **INV-01** `cmd/feediumctl/...` не импортирует `internal/biz`, `internal/data`, `internal/service` (CON-01).
- **INV-02** Exit `0` ⇒ stderr пуст. Exit `1` ⇒ stderr содержит ≥1 строки.
- **INV-03** `--help` на любой команде → stdout, exit `0`, RPC не выполняется.
- **INV-04** Все RPC-вызовы выполняются с дочерним `context.Context` с deadline из `--timeout` (FR-05).
- **INV-05** Для (endpoint, output, timeout, page_size) приоритет всегда `flag > env > config > default`.
- **INV-06** Один и тот же ответ сервера → один и тот же байт-в-байт вывод (см. FR-10, NFR-06).

## Acceptance Criteria

- [ ] **AC-01** `feediumctl health` при работающем сервере → stdout содержит ответ `V1Check` в `table` (см. FR-09), exit `0`.
- [ ] **AC-02** При выключенном сервере `feediumctl health` → exit `1`, stderr соответствует EC-A, stdout пуст.
- [ ] **AC-03** Приоритет `flag > env > config > default` подтверждён unit-тестами для `endpoint`, `output`, `timeout`, `page_size`.
- [ ] **AC-04** `make feediumctl` собирает бинарь без ошибок; `feediumctl --help` → exit `0` и печатает usage.
- [ ] **AC-05** `go vet ./...` и существующие lint-проверки проходят. `grep -r "internal/\(biz\|data\|service\)" cmd/feediumctl` пуст.
- [ ] **AC-06** Unit-тесты покрывают: парсинг конфига (FR-06, EC-D, EC-E, EC-G), формирование `V1CheckRequest`, рендеринг `V1CheckResponse` во все три формата (включая байт-в-байт стабильность по INV-06), шаблоны ошибок (NFR-03) для всех префиксов из закрытого списка.
- [ ] **AC-07** `feediumctl health --timeout=1ms` против работающего сервера, не успевающего ответить → поведение EC-H: stderr matches `^code=DeadlineExceeded message=.*`, exit `1`, stdout пуст.

## Ограничения

- **CON-01** CLI обращается только к публичным gRPC-контрактам из `api/feedium/`; не импортирует `internal/biz|data|service`.
- **CON-02** Только gRPC-транспорт; HTTP-клиент из `*_http.pb.go` не используется.
- **CON-03** Plaintext без TLS и аутентификации (MVP).
- **CON-04** Клиент и сервер релизятся одной версией.
- **CON-05** CLI-фреймворк — `spf13/cobra`; смена требует ADR.

## Open Questions

- **OQ-01** Стратегия версионирования CLI-команд при появлении `V2*`-методов в proto (флаг `--api-version`, отдельные подкоманды, всегда latest и break-change). В MVP не решается.
