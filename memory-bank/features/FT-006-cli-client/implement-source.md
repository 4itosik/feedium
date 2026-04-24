---
title: "FT-006: CLI Client — Implementation Plan (Source commands)"
doc_kind: feature
doc_function: implementation-plan
purpose: "Пошаговый план реализации `feediumctl source ...` поверх каркаса, зафиксированного в implement.md / handoff.md. Покрывает SR-01..SR-10, EC-A..EC-J, AC-S1..AC-S7 из spec-source.md."
derived_from:
  - spec-source.md
  - handoff.md
status: draft
---

# Implementation Plan

Базовые контракты (приоритет флагов, формат ошибок, детерминизм вывода, изоляция от `internal/biz|data|service`, поведение `--help`, пустой stdout при ошибке) заданы в [`spec.md`](./spec.md) и уже реализованы в каркасе. Этот план добавляет команды `source ...` без изменений этих контрактов.

## Steps

### Шаг 1. Вынос общего pipeline `resolveAndValidate`

- **Цель.** Исключить дублирование цепочки `loadConfig → flagSourceFrom → resolve.Resolve → ValidateOutput` при появлении 5 новых подкоманд (OI-2 из handoff).
- **Действия.**
  1. В пакете `cmd/feediumctl/internal/app` завести файл `pipeline.go` с непубличной функцией, принимающей `*cobra.Command` + ссылку на структуру глобальных флагов и возвращающей `settings` (endpoint, output, timeout, page_size) + ошибку.
  2. Перевести `runHealth` на этот helper; логика резолва должна остаться единой точкой (INV-05).
  3. Переместить текущую `resolveSettings` из `cmd/feediumctl/internal/app/health.go:75` в `pipeline.go` (или удалить её, если вся логика поглощена `resolveAndValidate`), чтобы `loadConfig(` и `flagSourceFrom(` вызывались ровно один раз в пакете `app`.
  4. Поведение и сообщения об ошибках (`config:`/`output:`/`flag:`/`endpoint:`) не менять — префиксы ошибок остаются из закрытого списка core NFR-03.
- **Зависимости.** Нет (работает с уже существующим каркасом).
- **Результат.** Один helper, используемый всеми будущими подкомандами; `runHealth` продолжает проходить все существующие тесты.
- **Как проверить.**
  - `go test ./cmd/feediumctl/... -race` — все тесты каркаса (включая health) зелёные без изменений ожиданий.
  - Поиск `loadConfig(` в пакете `app` находит одно место вызова (из pipeline-helper’а).

### Шаг 2. Пакет `sourcetype` — справочник `<type>` и валидация флагов

- **Цель.** Единая таблица соответствий `<type>` (CLI) ↔ `SourceType` (proto) ↔ вариант `SourceConfig.config` ↔ required/allowed флаги (OI-1, OI-5; SR-03, SR-04, EC-E, EC-I, EC-G).
- **Действия.**
  1. Завести пакет `cmd/feediumctl/internal/sourcetype` с закрытым реестром на 4 значения: `telegram-channel`, `telegram-group`, `rss`, `html`. Для каждого фиксируется `SourceType`-enum и список имён допустимых флагов.
  2. Реализовать lookup: по строковому `<type>` возвращать запись либо ошибку с сообщением `flag: unknown source type "<x>" (allowed: telegram-channel,telegram-group,rss,html)` (EC-E).
  3. Реализовать валидатор для `--type` флага (`source list`, `source update`): при неизвестном значении возвращать `flag: unknown --type "<x>"` (EC-G).
  4. Реализовать сборку `SourceConfig` из map’а прошедших флагов: для каждого `<type>` — собственный конструктор oneof (`TelegramChannelConfig`, `TelegramGroupConfig`, `RSSConfig`, `HTMLConfig`). Required-флаги из таблицы SR-03: `telegram-channel`/`telegram-group` — `--tg-id`, `--username`; `rss` — `--feed-url`; `html` — `--url`.
  5. Реализовать проверку «лишних» флагов: получив список установленных для команды флагов и выбранный `<type>`, вернуть ошибку `flag: --<name> is not allowed for type "<type>"` (EC-I) для первого несоответствия в стабильном порядке (по имени флага лексикографически, чтобы тесты были детерминированы).
  6. Реализовать проверку отсутствия required-флага: `flag: --<name> is required for type "<type>"` (EC-D). Если отсутствует несколько — сообщать о первом по тому же стабильному порядку.
  7. Все ошибки пакета используют только префикс `flag:` из закрытого списка core NFR-03. Новые префиксы не вводятся.
- **Зависимости.** Нет.
- **Результат.** Самодостаточный справочник, по которому команды `create` / `update` / `list` собирают запросы и валидируют флаги до RPC.
- **Как проверить.**
  - Unit-тесты на каждую запись реестра: lookup по валидному `<type>`, по неизвестному `<type>` (EC-E), построение `SourceConfig` с корректным oneof, отсутствие required-флага (EC-D — по одному кейсу на каждый `<type>`), лишний флаг (EC-I — по одному кейсу для `create` и `update`), неизвестный `--type` (EC-G).
  - Тесты фиксируют детерминированность сообщения при нескольких нарушениях одновременно.

### Шаг 3. Рендер enum’ов и timestamp’а для таблиц

- **Цель.** Обеспечить короткие имена `SourceType`/`ProcessingMode` и RFC3339 UTC представление `created_at` (SR-08, OI-3, OI-4).
- **Действия.**
  1. В новом файле `cmd/feediumctl/internal/render/source.go` добавить непубличные функции преобразования `SourceType` → короткое имя без префикса `SOURCE_TYPE_` и `ProcessingMode` → короткое имя без префикса `PROCESSING_MODE_`. Для `*_UNSPECIFIED` возвращать `UNSPECIFIED`. Тест-файл для них — `source_test.go` в том же пакете.
  2. Реализовать преобразование `*timestamppb.Timestamp` → строка в RFC3339 UTC. Нулевой/nil timestamp — пустая строка (на случай несуществующих полей в текущей протоколе — безопасный дефолт).
  3. Реализовать функцию форматирования однострочного представления `SourceConfig.config` (oneof) по правилам SR-08: `telegram_channel` → `tg_id=<n>,username=<u>`; `telegram_group` → `tg_id=<n>,username=<u>`; `rss` → `feed_url=<url>`; `html` → `url=<url>`; `config` не выставлен — пустая строка (EC-H в table).
  4. Новые функции приватные для пакета `render`; публичная сигнатура `render.Write(io.Writer, string, proto.Message)` не меняется.
- **Зависимости.** Нет.
- **Результат.** Готовый набор форматирующих утилит, на которые опираются шаги 4–5.
- **Как проверить.**
  - Unit-тесты на все значения `SourceType` и `ProcessingMode` (включая `UNSPECIFIED`) и по одному кейсу на каждый вариант oneof + случай «не выставлено».
  - Unit-тест на timestamp: конкретный `Timestamp` → ожидаемая RFC3339 UTC строка; nil → пустая строка.

### Шаг 4. Table-рендер для `V1ListSources`/`V1GetSource`/`V1CreateSource`/`V1UpdateSource`

- **Цель.** Расширить `render.writeTable` ветками для списка и одиночных ответов по SR-08, SR-09. Колонки: `ID | TYPE | MODE | CONFIG | CREATED_AT`.
- **Действия.**
  1. В `render.writeTable` добавить case-ветки для `*V1ListSourcesResponse`, `*V1GetSourceResponse`, `*V1CreateSourceResponse`, `*V1UpdateSourceResponse`.
  2. Завести непубличный helper `writeSourceRow(w, *Source)` — одна строка фиксированного формата `ID | TYPE | MODE | CONFIG | CREATED_AT`, поля в том же порядке, что и шапка. Внутри — использование утилит из Шага 3.
  3. Шапку выводить ровно одну: в `source list` — один раз перед циклом по `items`; в `source get`/`create`/`update` — один раз перед строкой c одиночным `Source`.
  4. Для `source list` порядок строк — как в ответе сервера. CLI не сортирует `items` (SR-01, R-2 из handoff).
  5. Пустой список (`items=[]`) печатает только шапку (EC-C).
  6. Для одиночных ответов, где сервер вернул nil `Source` (крайний случай), возвращать ошибку пакета `render` — валидные кейсы этого плана предполагают непустой `Source` от сервера.
  7. Каждая строка, включая последнюю, завершается одним `\n` (INV-06, NFR-06).
- **Зависимости.** Шаг 3.
- **Результат.** `render.Write` умеет рендерить все source-ответы, кроме `Delete`.
- **Как проверить.**
  - Unit-тесты на каждый case: `list` c 0/1/2 элементами, `get`/`create`/`update` с заполненным `Source`.
  - Byte-in-byte сверка заголовка и колонок (снапшотные ожидания в тесте); порядок `items` не пересортировывается (R-2).
  - Случай пустого `SourceConfig.config` → в колонке `CONFIG` пусто (EC-H).

### Шаг 5. Детерминированный рендер `V1DeleteSourceResponse`

- **Цель.** Byte-exact вывод для table/json/yaml по SR-05, независимо от общих protojson-опций (R-3, AC-S4).
- **Действия.**
  1. В пакете `render` добавить публичную функцию `WriteDelete(w io.Writer, format, id string) error`; существующая публичная `Write` остаётся без изменений. Специальный case в `Write` для `*V1DeleteSourceResponse` не добавляется.
  2. `WriteDelete` реализует форматирование самостоятельно, без делегирования в `Write` и без proto-сообщений (`V1DeleteSourceResponse` пуст, `id` приходит из аргумента команды).
  3. Форматы:
     - `table`: ровно строка `deleted: <id>\n`.
     - `json`: ровно `{"deleted":true,"id":"<id>"}\n` (одна строка + один `\n`, без отступов, фиксированный порядок ключей: `deleted`, затем `id`).
     - `yaml`: ровно `deleted: true\nid: <id>\n` (две строки, ключи отсортированы лексикографически; без кавычек вокруг UUID — SR-10).
  4. Не использовать общий `jsonMarshaler`/`writeYAML` для этого типа, чтобы избежать кавычек вокруг `id` и вариативности от protojson-опций (R-3).
  5. Функция, принимающая id, не выполняет никакой валидации формата UUID — это обязанность сервера; CLI рендерит ровно то, что пришло.
- **Зависимости.** Нет (не использует SR-08 колонки).
- **Результат.** Byte-stable вывод для `source delete` во всех трёх форматах, отвечающий AC-S4.
- **Как проверить.**
  - Snapshot-тесты (Шаг 13) сверяют вывод для фиксированного UUID `00000000-0000-4000-8000-000000000001` (SR-10) с файлами в `testdata/` — по одному на формат.
  - Unit-тест проверяет, что YAML-вывод не содержит символа `"` в строке с `id`.

### Шаг 6. Инжекция `SourceServiceClient` для тестов

- **Цель.** Завести паттерн DI, симметричный уже существующему для `HealthService` (см. `export_test.go` / handoff §2.5), без сетевого соединения в unit-тестах.
- **Действия.**
  1. В пакете `app` завести фабрику `sourceClientFactory`: интерфейс/тип-функция, возвращающий `SourceServiceClient` по подключенному `*grpc.ClientConn`.
  2. Продакшн-реализация — обёртка вокруг `feediumapi.NewSourceServiceClient`.
  3. В `export_test.go` (или новом `export_source_test.go`) экспортировать возможность подменить фабрику в тестах (`NewRootCommandWithSource(stubFactory)` или аналог существующего API для health).
  4. Тестовый stub-клиент реализует 5 методов `SourceService` с настраиваемыми ответами/ошибками (включая `codes.NotFound`, `codes.DeadlineExceeded`).
- **Зависимости.** Нет.
- **Результат.** Тестам source-команд не нужен реальный gRPC сервер; инжекция идёт через фабрику, как у health.
- **Как проверить.**
  - Существующий тест для health продолжает проходить (фабрика для health не изменилась).
  - Базовый smoke-тест: stub возвращает фиксированный ответ; команда `source get <id>` на нём печатает ожидаемую строку.

### Шаг 7. Подкоманда `feediumctl source` (группа)

- **Цель.** Ввести родительскую cobra-команду `source` и `--help`-поведение (AC-S6, INV-03).
- **Действия.**
  1. Добавить `source` как подкоманду root-команды, с коротким и длинным описанием группы; `SilenceUsage: true`, `SilenceErrors: true` (как у остальных команд).
  2. Родитель не делает RPC и не резолвит settings (все эффекты — в leaf-командах).
  3. Регистрировать её в `init`/конструкторе root-команды в одной точке.
- **Зависимости.** Нет.
- **Результат.** `feediumctl source --help` печатает usage с подкомандами `list|get|create|update|delete`, exit `0`.
- **Как проверить.**
  - Тест: `feediumctl source --help` → exit `0`, stdout содержит имена всех 5 подкоманд, stderr пуст, stub gRPC-клиент не был вызван.

### Шаг 8. Подкоманда `source list`

- **Цель.** Реализовать SR-01: `V1ListSources` c `page_size` из настроек, пустым `page_token`, опциональным `--type`.
- **Действия.**
  1. Добавить leaf-команду `list` под `source` без позиционных аргументов. Флаг `--type` — строка, опциональный; значение валидируется через пакет `sourcetype` (Шаг 2); при неизвестном значении — `flag: unknown --type "<x>"` (EC-G), RPC не выполняется.
  2. Ход выполнения: `resolveAndValidate` (Шаг 1) → `transport.Dial` (существующий helper) → `sourceClientFactory` (Шаг 6) → `context.WithTimeout` c `settings.Timeout` (INV-04) → `V1ListSources({page_size, page_token: "", type?})`.
  3. Успех: `render.Write(stdout, settings.Output, resp)` (Шаг 4). Накопление ответа — в памяти, печать в stdout только после успеха (INV-02, handoff §3.2).
  4. Ошибка от RPC: `app.WrapRPCError(err)`; формат `code=<Name> message=<...>` (NFR-03), stderr, exit `1`. Отдельно кейсов `NOT_FOUND` здесь нет (сервер для `List` их не возвращает по контракту), но обработчик универсален (EC-J).
  5. `next_page_token` игнорируется (Scope: «НЕ входит» auto-pagination).
- **Зависимости.** Шаги 1, 2, 4, 6, 7.
- **Результат.** Рабочая команда `feediumctl source list [--type=<t>] [--output=...]`.
- **Как проверить.**
  - Unit-тесты: сформированный `V1ListSourcesRequest` содержит `page_size=settings.PageSize`, `page_token=""`, `type` — nil если `--type` не задан, и конкретный `SourceType` — если задан.
  - Рендеринг `items=[]` в table/json/yaml соответствует EC-C.
  - `--type=unknown` → exit `1`, stderr `flag: unknown --type "unknown"`, RPC не вызван (EC-G).
  - Ошибка `DeadlineExceeded` от stub → EC-B.

### Шаг 9. Подкоманда `source get`

- **Цель.** Реализовать SR-02: `V1GetSource{id}` с позиционным `<id>`.
- **Действия.**
  1. Добавить leaf-команду `get` под `source` c обязательным позиционным `<id>` (ровно один аргумент через cobra `ExactArgs(1)`; при нарушении — cobra-ошибка через тот же репортёр с префиксом `flag:` — поведение уже обеспечено каркасом).
  2. Pipeline: `resolveAndValidate` → dial → клиент → `context.WithTimeout` → `V1GetSource{id}`.
  3. Успех: `render.Write(stdout, settings.Output, resp)` (Шаг 4).
  4. Ошибка `NOT_FOUND` (EC-A) и остальные (EC-B, EC-J) — через `WrapRPCError`, stderr, exit `1`.
- **Зависимости.** Шаги 1, 4, 6, 7.
- **Результат.** Рабочая `feediumctl source get <id>`.
- **Как проверить.**
  - Unit-тест: `V1GetSourceRequest.id` совпадает с переданным позиционным аргументом.
  - Тест на `NOT_FOUND` → stderr `code=NotFound message=<...>`, exit `1`, stdout пуст (EC-A).
  - Рендер одиночного `Source` совпадает с ожидаемой одной строкой `ID | TYPE | MODE | CONFIG | CREATED_AT` (SR-09).

### Шаг 10. Подкоманда `source create <type> [flags]`

- **Цель.** Реализовать SR-03: позиционный `<type>` → `SourceType` + oneof `SourceConfig.config`, required флаги по таблице.
- **Действия.**
  1. Добавить leaf-команду `create` под `source` c `ExactArgs(1)` для `<type>`. Зарегистрировать объединённое множество флагов: `--tg-id` (int64), `--username` (string), `--feed-url` (string), `--url` (string). Cobra-валидация на опечатки/неизвестные флаги даст ошибку c префиксом `flag:` без дополнительного кода.
  2. В `RunE`: по `<type>` найти запись в пакете `sourcetype` (Шаг 2). При неизвестном — возврат ошибки EC-E, RPC не выполняется.
  3. Проверить required-флаги (EC-D) и отсутствие лишних (EC-I) через валидаторы Шага 2. Обе проверки — до dial и RPC.
  4. Собрать `V1CreateSourceRequest{type: SourceType, config: SourceConfig{...oneof...}}` через конструктор из `sourcetype`.
  5. Pipeline: `resolveAndValidate` → dial → клиент → `context.WithTimeout` → `V1CreateSource`.
  6. Успех: `render.Write(stdout, settings.Output, resp)` (Шаг 4, SR-09).
  7. Ошибки: EC-B/EC-J — стандартный обработчик.
- **Зависимости.** Шаги 1, 2, 4, 6, 7.
- **Результат.** Рабочая `feediumctl source create <type> [flags]` для всех 4 типов.
- **Как проверить.**
  - Unit-тесты по одному кейсу на каждый `<type>`: запрос содержит ожидаемый `SourceType` и заполненный `SourceConfig.config` правильного варианта oneof (AC-S5, AC-S3).
  - Неизвестный `<type>` (EC-E), отсутствующий required-флаг (EC-D), лишний флаг (EC-I) — RPC не вызывается, exit `1`, ожидаемое сообщение с префиксом `flag:`.

### Шаг 11. Подкоманда `source update <id> --type=<type> [flags]`

- **Цель.** Реализовать SR-04: `V1UpdateSource{id, type, config}`; `<id>` позиционный, `--type` обязательный, набор флагов как у create.
- **Действия.**
  1. Добавить leaf-команду `update` под `source` c `ExactArgs(1)` для `<id>`. Зарегистрировать флаги как в Шаге 10 + `--type` (string), помеченный как required (`MarkFlagRequired`). При отсутствии — cobra-ошибка с префиксом `flag:`.
  2. В `RunE`: lookup `--type` → `sourcetype` (EC-G при неизвестном → `flag: unknown --type "<x>"`), проверки required/лишних флагов по выбранному типу (EC-D, EC-I).
  3. Собрать `V1UpdateSourceRequest{id, type, config}` тем же конструктором из `sourcetype`.
  4. Pipeline: стандартный; успех — rendring как SR-09.
  5. Ошибки EC-A/EC-B/EC-J/EC-F — стандартный обработчик (для delete несуществующего id см. Шаг 12, тут — `NOT_FOUND` при обновлении возможен аналогично).
- **Зависимости.** Шаги 1, 2, 4, 6, 7.
- **Результат.** Рабочая `feediumctl source update <id> --type=<type> [flags]`.
- **Как проверить.**
  - Unit-тест: сформированный запрос для каждого из 4 типов (`id`, `SourceType`, корректный oneof).
  - EC-I: `update <id> --type=rss --username=foo` → `flag: --username is not allowed for type "rss"`, RPC не вызывается.
  - Отсутствие `--type` → cobra-ошибка, exit `1`, RPC не вызывается.

### Шаг 12. Подкоманда `source delete <id>`

- **Цель.** Реализовать SR-05: `V1DeleteSource{id}` и byte-exact рендеринг во всех трёх форматах.
- **Действия.**
  1. Добавить leaf-команду `delete` под `source` c `ExactArgs(1)` для `<id>`.
  2. Pipeline стандартный, RPC — `V1DeleteSource{id}`.
  3. Успех: вызвать спец-рендер из Шага 5 с (format, id). Ничего из `V1DeleteSourceResponse` не читается (он пуст).
  4. Ошибки: `NOT_FOUND` (EC-F) — через `WrapRPCError`, exit `1`; EC-B/EC-J — аналогично.
- **Зависимости.** Шаги 1, 5, 6, 7.
- **Результат.** `feediumctl source delete <id> [--output=...]` с byte-stable выводом.
- **Как проверить.**
  - Unit-тест: `V1DeleteSourceRequest.id` совпадает с аргументом.
  - EC-F: stub возвращает `NotFound` → stderr `code=NotFound message=<...>`, exit `1`, stdout пуст.

### Шаг 13. Snapshot-тест AC-S4 для `source delete`

- **Цель.** Зафиксировать byte-exact вывод `source delete` для UUID `00000000-0000-4000-8000-000000000001` во всех трёх форматах (SR-05, SR-10, AC-S4, R-3).
- **Действия.**
  1. Создать три файла в `cmd/feediumctl/internal/app/testdata/source_delete/`: `table.txt`, `json.txt`, `yaml.txt` (по одному на формат, отдельные файлы — проще diff’ы при поломке).
  2. В тесте: запускать команду `source delete 00000000-0000-4000-8000-000000000001 --output=<fmt>` на stub-клиенте, возвращающем пустой `V1DeleteSourceResponse`, и сверять stdout побайтно с файлом.
  3. Тест отдельно проверяет, что в yaml-выводе нет символа `"` (защита от кавычек вокруг UUID).
- **Зависимости.** Шаги 5, 6, 12.
- **Результат.** AC-S4 автоматически зелёный; любое случайное изменение формата ломает тест.
- **Как проверить.**
  - `go test ./cmd/feediumctl/internal/app/... -run TestSourceDelete -race` — зелёный.
  - Ручное изменение любого из trailing `\n` в фикстурах немедленно ломает тест.

### Шаг 14. Unit-тесты AC-S3 / AC-S5 для остальных сценариев

- **Цель.** Покрыть unit-тестами все обязательные сценарии AC-S3 и AC-S5 из spec-source.md.
- **Действия.**
  1. Таблицы-драйвены тесты в `cmd/feediumctl/internal/app/source_*_test.go`:
     - Маппинг `<type>` → `SourceType` + oneof для всех 4 типов (через команды `create` и `update`).
     - Рендеринг enum `SourceType`/`ProcessingMode` без префиксов + `UNSPECIFIED`.
     - EC-H: пустой `SourceConfig.config` на `get`/`create`/`update`/`list` (в table — пустая колонка, в json/yaml — поле отсутствует, опция `EmitUnpopulated=false` уже в общем рендере).
     - EC-C: пустой `items` для `list` во всех трёх форматах.
     - EC-A (`NotFound`) на `get`.
     - EC-B (`DeadlineExceeded`) — одна команда из набора (чтобы исключить проверку повторения одной и той же логики 5 раз).
     - EC-E: неизвестный `<type>` на `create`.
     - EC-G: неизвестный `--type` на `list`.
     - EC-I: один кейс на `create`, один на `update`.
  2. Тесты используют stub-клиент из Шага 6 и не открывают gRPC-соединения.
  3. Во всех error-кейсах проверять: stdout пуст (INV-02), stderr — одна строка с ожидаемым префиксом, exit-код 1.
- **Зависимости.** Шаги 2–12.
- **Результат.** Чеклист AC-S3 и AC-S5 закрыт автоматикой.
- **Как проверить.**
  - `go test ./cmd/feediumctl/... -race` — зелёный.
  - Новое покрытие: проверить, что тесты подкоманд присутствуют для каждого из пунктов AC-S5 (один по одному).

### Шаг 15. Ручная верификация CHK-01 и артефакт

- **Цель.** Воспроизвести сценарий CHK-01 и сохранить логи — AC-S7, EVID-01.
- **Действия.**
  1. Запустить сервер локально (`make build && ./bin/feedium -conf configs/`).
  2. Выполнить последовательность сценариев (по одной happy- и одной error-команде на каждую подкоманду + проверка всех трёх форматов на `list` и `delete`). Команды и вывод фиксировать с `stdout`, `stderr`, exit-кодом.
  3. Сложить файлы в `artifacts/ft-006/verify/chk-01/` (имена по шаблону `<subcmd>-<case>.{stdout,stderr,exit}`).
  4. Параллельно ещё раз выполнить диагностические команды из handoff §5 (`grep -R "internal/\(biz\|data\|service\)" cmd/feediumctl/` остаётся пустым; `--help` на каждой подкоманде — exit `0`).
  5. Обновить `memory-bank/features/index.md` (строка FT-006: статус `delivery: done`) и проставить финальный `status` в `feature.md`, `spec.md`, `spec-source.md`, `implement.md`, `implement-source.md` в соответствии с `dna/governance.md`.
- **Зависимости.** Шаги 7–13.
- **Результат.** Каталог с артефактами верификации, ссылка на него — из раздела Delivery в README/feature.md; все governance-документы FT-006 переведены в финальный статус.
- **Как проверить.**
  - Наличие файла на каждый подсценарий; exit-код happy сценариев — `0`, error — `1`; stdout пуст у каждого error-сценария (INV-02); byte-exact `source delete` в каждом формате совпадает с фикстурами Шага 13.

## Edge Cases

Каждая ошибка ниже — stdout пуст, stderr ровно одна строка, exit `1`, RPC не выполняется там, где явно указано. Префиксы ошибок — только из закрытого списка core NFR-03.

- **EC-A `source get <id>` → `NOT_FOUND`.** Stderr: `code=NotFound message=<...>`, exit `1`. Покрытие: Шаг 9, unit-тест.
- **EC-B Таймаут RPC (`DeadlineExceeded`).** Любая подкоманда. Stderr: `code=DeadlineExceeded message=context deadline exceeded`, exit `1`. Покрытие: Шаг 14.
- **EC-C `V1ListSources.items=[]`.** `table` — только шапка; `json` — `[]\n`; `yaml` — `[]\n`; exit `0`. Покрытие: Шаги 4 и 14.
- **EC-D Отсутствие required-флага в `create`/`update`.** Стандартизованное сообщение `flag: --<name> is required for type "<type>"` (для `<type>=rss` без `--feed-url`: `flag: --feed-url is required for type "rss"`). RPC не выполняется. Покрытие: Шаги 2, 10, 11, 14.
- **EC-E Неизвестный `<type>` в `create` / `update`.** Stderr: `flag: unknown source type "<x>" (allowed: telegram-channel,telegram-group,rss,html)`. RPC не выполняется. Покрытие: Шаги 2, 10, 14.
- **EC-F `source delete <unknown-id>` / `source update <unknown-id>`.** Сервер отвечает `NOT_FOUND` → поведение EC-A. Покрытие: Шаги 11, 12, 14.
- **EC-G `source list --type=<unknown>`.** Stderr: `flag: unknown --type "<x>"`. RPC не выполняется. Покрытие: Шаги 2, 8, 14.
- **EC-H Сервер вернул `Source` c невыставленным `SourceConfig.config`.** `table` — колонка `CONFIG` пустая; `json`/`yaml` — поле `config` опускается (общая опция `EmitUnpopulated=false`). Покрытие: Шаги 3, 4, 14.
- **EC-I Флаг, не принадлежащий указанному `<type>`.** Stderr: `flag: --<name> is not allowed for type "<type>"`. RPC не выполняется. Покрытие: Шаги 2, 10, 11, 14.
- **EC-J Прочие gRPC status codes.** Generic-обработчик core NFR-03, отдельных source-тестов не требуется (явно зафиксировано в spec-source.md). Покрытие: Шаги 1 и ранее реализованный `FormatRPCError`.

Дополнительные пограничные требования за пределами EC-списка spec-source.md:

- **EC-UUID-YAML.** При `source delete` в формате `yaml` вокруг `id` не должно появляться кавычек (SR-10, R-3). Покрытие: Шаг 5 + snapshot-тест Шага 13.
- **EC-ORDER-LIST.** `source list` не пересортировывает `items`; порядок — как в ответе сервера (SR-01, R-2). Покрытие: Шаг 8, unit-тест сверяет порядок со stub-ответом.
- **EC-PARTIAL-TABLE.** Ни одна команда не печатает часть таблицы до появления ошибки (handoff §3.2). Для `source list` это означает буферизацию `items` целиком до вызова `render.Write`. Покрытие: Шаг 8 + ревью.

## Verification

Команды выполняются на ветке `feature/ft-006-cli-client` (worktree `.worktrees/ft-006-cli-client`).

- **V-1.** `go test ./cmd/feediumctl/... -race` — зелёный (покрывает AC-S3, AC-S4, AC-S5, часть AC-S6).
- **V-2.** `go vet ./...` — зелёный.
- **V-3.** `grep -R "internal/\(biz\|data\|service\)" cmd/feediumctl/` — пусто (INV-01, AC-S5 через изоляцию).
- **V-4.** `make feediumctl && ./bin/feediumctl source --help` — exit `0`, stdout содержит `list`, `get`, `create`, `update`, `delete` (AC-S6).
- **V-5.** `./bin/feediumctl source list --output=json` при работающем сервере — exit `0`, stdout — валидный JSON-массив (AC-S1).
- **V-6.** `./bin/feediumctl source list` при выключенном сервере — exit `1`, stderr соответствует core EC-A, stdout пуст (AC-S2).
- **V-7.** CHK-01: ручной прогон full-matrix (list/get/create×4/update/delete) с сохранением артефактов в `artifacts/ft-006/verify/chk-01/` (AC-S7).
- **V-8.** Byte-exact сверка `source delete` во всех трёх форматах c testdata-фикстурами (AC-S4; автоматически через V-1).
- **V-9.** Smoke на каждом `<subcmd> --help`: exit `0`, RPC не вызывается (INV-03).
- **V-10.** `go test ./cmd/feediumctl/internal/app/... -run Health` — существующие health-тесты остаются зелёными после рефакторинга Шага 1 (R-4).

## Open Questions

- **OQ-S1 [Закрыт].** Выбор зафиксирован в Шаге 5: в пакете `render` добавляется публичная функция `WriteDelete(w io.Writer, format, id string) error`; существующая `Write` остаётся без изменений. Обоснование: `V1DeleteSourceResponse` пуст, `id` берётся из аргументов команды — payload не является proto-сообщением.
- **OQ-S2 Стабильный порядок сообщений при нескольких нарушениях EC-D/EC-I одновременно.** Spec-source.md не фиксирует, какой флаг отчитывается первым, если отсутствуют несколько required-флагов или указано несколько лишних. План принимает лексикографический порядок имён флагов; если это нежелательно — нужна правка spec.
- **OQ-S3 Поведение `source list` при `page_size=0` / отрицательном.** Core spec FR-03 говорит, что валидирует сервер; CLI передаёт как есть. Unit-тест этого пограничного случая выходит за scope; фиксируется как «передаётся как есть» (следуя FR-03).
- **OQ-S4 UUID в позиционных аргументах.** CLI не валидирует формат `<id>` (SR-10 относится к выводу, не ко входу). Это решение мешает раннему feedback’у, но вписывается в spec («ответственность сервера»). Если в будущем захочется CLI-side валидации — отдельная правка spec.
- **OQ-S5 OI-7 из handoff.** Настоящий план оформлен отдельным файлом `implement-source.md`, а не главой в существующем `implement.md`. Это согласуется с тем, что core-план заморожен после реализации; при желании автор feature-flow может ссылаться на оба файла из `README.md`. Если по feature-flow требуется единый файл — нужна явная консолидация.
- **OQ-S6 Формат JSON-массива `source list`.** Core FR-10 фиксирует опции `protojson.MarshalOptions{Multiline: true, Indent: "  ", ...}` для proto-сообщений. CLI оформляет `source list` не envelope'ом `V1ListSourcesResponse`, а массивом `items` (AC-S1: «JSON-массив»), для чего используется отдельный `listItemMarshaler` без `Multiline`, чтобы не получить несогласованный отступ между уровнем массива `[]` и уровнем item'а. Spec напрямую не описывает формат массива (multiline vs. compact); выбран компактный `[{...},{...}]\n`. Если требуется multiline-массив — правка spec-source.md (явное описание формата оборачивающего массива и требуемого indent'а).
