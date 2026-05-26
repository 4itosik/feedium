---
title: "FT-009-collectors: Content Collectors — Technical Spec"
doc_kind: feature
doc_function: derived
purpose: "Технически исполнимая spec для FT-009: переводит product-ready feature.md в change surface, контракты, инварианты, failure modes и test strategy для implement-этапа. Не переопределяет scope/acceptance из feature.md — ссылается по ID."
derived_from:
  - feature.md
  - ../../domain/architecture.md
  - ../../engineering/coding-style.md
  - ../../engineering/go-style.md
  - ../../engineering/testing-policy.md
  - ../../engineering/autonomy-boundaries.md
  - ../FT-004-posts-management/spec.md
  - ../FT-005-ai-summarization/spec.md
  - ../FT-007-scalable-event-processing/feature.md
status: draft
delivery_status: planned
---

# FT-009-collectors: Content Collectors — Technical Spec

## Solution

Для каждого типа источника вводится отдельный коллектор в `internal/task/`, реализующий kratos `transport.Server` и работающий в polling-цикле по образцу `internal/task/source_due_scheduler.go:23` (`Start()` запускает горутину, `Stop()` закрывает `done`-канал). Коллектор перечисляет источники своего типа через `biz.SourceUsecase.List` (фильтр `Type`), для каждого источника вызывает доменную операцию сбора `biz.CollectUsecase.CollectSource`, которая обращается к внешней системе через порт `biz.Fetcher` (реализация в `internal/data/`), отфильтровывает нетекстовый материал (`REQ-10`) и сохраняет каждый текстовый item через существующий путь `biz.PostUsecase.Create` (`internal/biz/post.go:152`). Этот путь уже даёт идемпотентность по `(source_id, external_id)` (`REQ-06`), атомарное создание `SummaryEvent` для self-contained и отсутствие события для cumulative (`REQ-09`) — поэтому FT-009 не вводит ни новой персистентности, ни нового outbox-механизма. Ключевой trade-off (зафиксирован в feature.md → Solution): отдельный коллектор на тип ради изоляции отказов (`REQ-07`) и независимой rate-limit/retry-политики (`CON-02`) ценой повторяющегося lifecycle-каркаса в `task/`, который инкапсулируется общим вызовом `CollectUsecase.CollectSource`.

> **Design-delta (DEC-08, реализовано 2026-05-26):** изложенный polling-механизм относится к RSS (`REQ-01`) и web (`REQ-04`). Telegram (`REQ-02`/`REQ-03`) реализован **push-моделью** на `gotd/td` (`updates.Manager`): `task.TelegramCollector` не крутит polling-цикл и не использует порт `biz.Fetcher`, а потребляет порт `biz.TelegramSession` (реализация `internal/data/telegram_session.go`) и для каждого пришедшего сообщения резолвит источник по `TgID` и сохраняет через `biz.CollectUsecase.CollectItems` (тот же путь `PostUsecase.Create`, поэтому `REQ-06`/`REQ-09`/`REQ-10` сохраняются без изменений). Живой userbot-коннект/авторизация — за `AG-02` (ручной вход). Полное обоснование — [`2026-05-26-telegram-push-design.md`](2026-05-26-telegram-push-design.md).

## Change Surface

| Path/Component | Layer | Why | REQ |
|---|---|---|---|
| `internal/biz/collect.go` (new) | biz | `CollectedItem`, порт `Fetcher`, порт `PostCreator`, `CollectUsecase`, чистая функция `FilterTextItems`, sentinel-ошибки | `REQ-05`, `REQ-06`, `REQ-09`, `REQ-10` |
| `internal/biz/collect_test.go` (new) | biz | TDD-юниты для `CollectUsecase`/`FilterTextItems` | `REQ-10`, `REQ-11` |
| `internal/biz/mock/mock_collect.go` (new) | biz | mockgen для `Fetcher` и `PostCreator` | `REQ-11` |
| `internal/biz/wire.go:8` (edit) | biz | добавить `NewCollectUsecase` в `ProviderSet` | `REQ-01`..`REQ-04` |
| `internal/data/rss_fetcher.go` (new) | data | реализация `biz.Fetcher` для RSS (парсинг фида → `CollectedItem`) | `REQ-01` |
| `internal/data/telegram_fetcher.go` (new) | data | реализация `biz.Fetcher` для Telegram-канала и группы (userbot) | `REQ-02`, `REQ-03` |
| `internal/data/web_fetcher.go` (new) | data | реализация `biz.Fetcher` для веб-сайтов (HTML-извлечение) | `REQ-04` |
| `internal/data/rss_fetcher_test.go` (new) | data | `httptest.Server` + RSS-фикстуры | `REQ-01`, `REQ-11` |
| `internal/data/web_fetcher_test.go` (new) | data | `httptest.Server` + HTML-фикстуры (включая сломанную вёрстку) | `REQ-04`, `REQ-11` |
| `internal/data/telegram_fetcher_test.go` (new) | data | стаб/мок Telegram-клиента | `REQ-02`, `REQ-03`, `REQ-11` |
| `internal/data/wire.go:10` (edit) | data | провайдеры `New*Fetcher` + `wire.Bind` к `biz.Fetcher` | `REQ-01`..`REQ-04` |
| `internal/task/rss_collector.go` (new) | task | воркер `transport.Server`: polling, изоляция отказа, retry-policy RSS | `REQ-01`, `REQ-07`, `REQ-08` |
| `internal/task/telegram_collector.go` (new) | task | воркер: lifecycle userbot-сессии, rate-limit aware сбор каналов и групп | `REQ-02`, `REQ-03`, `REQ-07`, `REQ-08` |
| `internal/task/web_collector.go` (new) | task | воркер: polling, single-retry-then-skip для веб-источников | `REQ-04`, `REQ-07`, `REQ-08` |
| `internal/task/rss_collector_test.go` (new) | task | integration (testcontainers + httptest + goleak) | `REQ-01`, `REQ-06`, `REQ-08`, `REQ-11` |
| `internal/task/web_collector_test.go` (new) | task | integration: успех + extraction-failure skip + goleak | `REQ-04`, `REQ-07`, `REQ-11` |
| `internal/task/telegram_collector_test.go` (new) | task | integration через мок-клиент: канал/группа + изоляция отказа + goleak | `REQ-02`, `REQ-03`, `REQ-07`, `REQ-11` |
| `internal/task/wire.go:10` (edit) | task | добавить `New*Collector` в `ProviderSet` | `REQ-01`..`REQ-04` |
| `internal/conf/conf.proto:9` (edit) | configs | новое сообщение `Collectors` + поле в `Bootstrap` | `REQ-08` |
| `internal/conf/conf.pb.go` (regen) | configs | результат `make proto` | `REQ-08` |
| `configs/config.yaml:1` (edit) | configs | секция `collectors:` (интервалы опроса) | `REQ-08` |
| `configs/local.example.yaml:1` (edit) | configs | пример `collectors:` + плейсхолдеры Telegram-сессии/секретов | `REQ-08` |
| `cmd/feedium/wire.go` (edit) | cmd | регистрация коллекторов в `newApp` → `kratos.Server(...)` + провайдер конфига коллекторов | `REQ-01`..`REQ-04` |
| `cmd/feedium/wire_gen.go` (regen) | cmd | результат `make wire` | `REQ-01`..`REQ-04` |
| `go.mod`, `go.sum` (edit) | third_party | новые зависимости (RSS-парсер, HTML-извлечение, Telegram-клиент) — см. `DEC-01`..`DEC-03` | `REQ-01`, `REQ-02`, `REQ-04` |

Схема БД (`internal/ent/schema/post.go`, `internal/ent/schema/source.go`) и миграции в `migrations/` **не меняются** (`INV-T06`, `CON-05`, `ASM-02`). FT-009 наполняет существующие поля FT-003/FT-004.

## Files

Полный whitelist для handoff-верификации. При реализации не трогать пути вне списка.

**biz**
- `internal/biz/collect.go` (new)
- `internal/biz/collect_test.go` (new)
- `internal/biz/mock/mock_collect.go` (new)
- `internal/biz/wire.go` (edit)

**data**
- `internal/data/rss_fetcher.go` (new)
- `internal/data/telegram_fetcher.go` (new)
- `internal/data/web_fetcher.go` (new)
- `internal/data/rss_fetcher_test.go` (new)
- `internal/data/web_fetcher_test.go` (new)
- `internal/data/telegram_fetcher_test.go` (new)
- `internal/data/wire.go` (edit)

**task**
- `internal/task/rss_collector.go` (new)
- `internal/task/telegram_collector.go` (new)
- `internal/task/web_collector.go` (new)
- `internal/task/rss_collector_test.go` (new)
- `internal/task/web_collector_test.go` (new)
- `internal/task/telegram_collector_test.go` (new)
- `internal/task/wire.go` (edit)

**configs**
- `internal/conf/conf.proto` (edit)
- `internal/conf/conf.pb.go` (regen — `make proto`)
- `configs/config.yaml` (edit)
- `configs/local.example.yaml` (edit)

**cmd**
- `cmd/feedium/wire.go` (edit)
- `cmd/feedium/wire_gen.go` (regen — `make wire`)

**deps**
- `go.mod` (edit)
- `go.sum` (edit)

## Flow

1. **Вход (cron tick).** Kratos запускает `Start()` каждого коллектора (`internal/task/*_collector.go`) в отдельной горутине. Коллектор крутит polling-цикл с интервалом из `conf.Collectors` (`REQ-08`, `CTR-04`, `CTR-T01`); пауза и выход реализуются `select` по `time.After`/`ctx.Done()`/`done` (паттерн `source_due_scheduler.go:55`).
2. **Перечисление источников.** На каждом тике коллектор постранично вызывает `biz.SourceUsecase.List(ctx, ListSourcesFilter{Type: <свой тип>})` и итерирует источники своего типа. Источники не его типа им игнорируются (изоляция между коллекторами — `REQ-07`).
3. **Сбор одного источника.** Для каждого источника коллектор вызывает `biz.CollectUsecase.CollectSource(ctx, source, fetcher)`. Usecase обращается к порту `biz.Fetcher.Fetch(ctx, source)` (реализация в `data/`), получает `[]CollectedItem` (материал с момента предыдущего цикла, `NS-04`), применяет `FilterTextItems` (`REQ-10`) и для каждого текстового item вызывает `PostCreator.Create(...)` (= `biz.PostUsecase.Create`, `internal/biz/post.go:152`).
4. **Сохранение и downstream.** `PostUsecase.Create` в одной Ent-транзакции сохраняет пост (`postRepo.Save`, upsert по `(source_id, external_id)`): для нового self-contained поста атомарно создаётся `SummaryEvent` (`internal/biz/post.go:196`), для cumulative — событие не создаётся (его планирует `SourceDueScheduler`/FT-007), для дубликата `Save` возвращает `created=false` → ни события, ни ошибки (`REQ-06`, `REQ-09`).
5. **Выход (наблюдаемый результат).** Новые строки `posts` в PostgreSQL с источником/датой/external_id/метаданными (`REQ-05`); для self-contained — строка `summary_events`. Ошибка сбора отдельного источника логируется в `task/` (slog, ключ `source_id`/`err`) и не прерывает итерацию по остальным источникам и работу остальных коллекторов (`REQ-07`, `CTR-05`, `FM-T01`).

Описание дано через интерфейсы (`Fetcher`, `CollectUsecase`, `PostCreator`, `SourceUsecase`); конкретные сигнатуры и тела — задача implement-этапа.

## Contracts

Дополняют продуктовые `CTR-01`..`CTR-05` из feature.md техническими контрактами.

- `CTR-T01` **Config.** Новое сообщение `conf.Collectors` в `internal/conf/conf.proto` и поле `Collectors collectors = 4` в `Bootstrap`; yaml-ключ `collectors:` в `configs/config.yaml` и `configs/local.example.yaml`. Тип интервалов — `google.protobuf.Duration` (как `SummarySourceScheduler.poll_interval`, `conf.proto:68`). Регенерация — `make proto`. Правило: [coding-style.md §Конфигурация](../../engineering/coding-style.md#конфигурация) (секреты в YAML `configs/`, конфиг с секретами не коммитится; интервалы воркеров — owner `internal/task/`). Реализует `REQ-08`, `CTR-04`.
- `CTR-T02` **Порт `biz.Fetcher`.** Интерфейс определяется в `biz/`, реализации — в `data/`; обязателен compile-time assertion `var _ biz.Fetcher = (*rssFetcher)(nil)` и `wire.Bind` в `internal/data/wire.go`. Правило: [coding-style.md §Интерфейсы по слоям](../../engineering/coding-style.md#интерфейсы-по-слоям). `data/` только маппит внешний ответ в `CollectedItem` и не логирует ([coding-style.md §Error Handling](../../engineering/coding-style.md#error-handling)).
- `CTR-T03` **Переиспользование `PostUsecase.Create`.** `CollectUsecase` сохраняет посты через порт `PostCreator` (bound к `*biz.PostUsecase`), не пишет в БД напрямую и не создаёт `SummaryEvent` сам. Форма outbox-контракта FT-005/FT-007 не меняется. Правило: [architecture.md → Outbox/Concurrency](../../domain/architecture.md#concurrency-and-critical-sections); `NS-08`, `CTR-03`.
- `CTR-T04` **Kratos lifecycle.** Каждый коллектор регистрируется в `internal/task/wire.go` `ProviderSet` и добавляется аргументом в `newApp` → `kratos.Server(...)` (`cmd/feedium/wire.go`), после чего `make wire` регенерирует `wire_gen.go`. Правило: [coding-style.md §Graceful Shutdown](../../engineering/coding-style.md#graceful-shutdown-и-health-checks) (kratos вызывает `Stop()` сам, своё signal-handling запрещено).
- `CTR-T05` **Новые зависимости.** RSS-парсер, HTML-извлечение и Telegram-клиент добавляются в `go.mod`/`go.sum`. Правило: [go-style.md — minimal deps](../../engineering/go-style.md) + согласование выбора библиотеки ([coding-style.md §Общие правила](../../engineering/coding-style.md#общие-правила): «Выбор библиотеки — только после согласования»). Обоснование — `DEC-01`..`DEC-03`; финализация — на КТ/approval (см. `## Decisions`).

## Технические инварианты

Каждый `INV-T*` наблюдаем через указанный `CHK-*`.

- `INV-T01` Все DB-writes собранных постов идут через `biz.PostUsecase.Create`; `task/` и `data/`-fetcher'ы не пишут в `posts`/`summary_events` напрямую. Наблюдаем: `CHK-01`, `CHK-02`.
- `INV-T02` `internal/biz/collect.go` не импортирует `ent`/`http`/`sql`/конкретные клиенты — только порт `Fetcher` ([architecture.md → Module Boundaries](../../domain/architecture.md#module-boundaries)). Наблюдаем: `CHK-11` (build/lint).
- `INV-T03` Идемпотентность сбора обеспечивается единственным механизмом — уникальным индексом `(source_id, external_id)` FT-004; второй слой дедупликации не вводится. Наблюдаем: `CHK-05`.
- `INV-T04` Логирование и retry — только в коллекторах `task/`; `Fetcher` (`data/`) и `CollectUsecase` (`biz/`) ошибки только возвращают ([architecture.md → Failure Handling](../../domain/architecture.md#failure-handling-and-error-tracking)). Наблюдаем: `CHK-06`, `CHK-07`.
- `INV-T05` Нетекстовый материал отсеивается до вызова `Create` (`FilterTextItems`), а не порождает невалидный `Post` и не валит цикл (`REQ-10`). Наблюдаем: `CHK-09`.
- `INV-T06` Схемы `posts`/`sources` и существующие миграции не меняются (`CON-05`, `ASM-02`). Наблюдаем: `git diff` пуст по `internal/ent/schema/` и `migrations/`; `CHK-11`.
- `INV-T07` Каждый коллектор — отдельный `transport.Server`; общий state только через PostgreSQL, in-process shared mutable state между коллекторами отсутствует ([architecture.md → Concurrency](../../domain/architecture.md#concurrency-and-critical-sections)). Наблюдаем: `CHK-06` (изоляция) + goleak.

## Concurrency & lifecycle

- **Регистрация.** Коллекторы добавляются в `internal/task/wire.go` `ProviderSet` (рядом с `NewEventWorkerPool`, `NewSourceDueScheduler`, `NewStuckEventReaper`) и в `kratos.Server(...)` через `newApp` (`cmd/feedium/wire_gen.go:77` — текущий список серверов).
- **Старт.** `Start(ctx)` читает интервал из `conf.Collectors`, проверяет `<= 0` → fallback на дефолтную константу пакета (паттерн `source_due_scheduler.go:41`), запускает `go run(...)`. Telegram-коллектор дополнительно в `Start()` поднимает userbot-сессию (подключение/авторизация по сохранённой сессии) и хранит долгоживущий клиент.
- **Shutdown (graceful).** `Stop(ctx)` закрывает `done` (через `sync.Once`), цикл выходит по `select`. Telegram-коллектор в `Stop()` закрывает userbot-соединение. Таймаут graceful — конфиг сервера kratos (по умолчанию 10s, [coding-style.md §Graceful Shutdown](../../engineering/coding-style.md#graceful-shutdown-и-health-checks)).
- **Горутины.** Один polling-цикл на коллектор; обращения к внешним системам — внутри тика, не в request-path. Per-source ошибки не прерывают тик целиком (continue по источникам). Покрытие утечек — `go.uber.org/goleak` в каждом task-тесте (`goleak.IgnoreCurrent()`), обязательно для нового воркера ([testing-policy.md §Goroutine Leak Detection](../../engineering/testing-policy.md#goroutine-leak-detection)).

## Failure modes

Технические детали продуктовых `FM-01`..`FM-06` (feature.md). Retry-policy — по [architecture.md → Retry policy](../../domain/architecture.md#retry-policy).

- `FM-T01` **`Fetcher.Fetch` вернул ошибку (таймаут/обрыв/ошибка фида).** Detection: ненулевая ошибка от порта. Recovery: коллектор логирует (`source_id`, `err`), пропускает источник в этом тике, продолжает остальные; на следующем тике повторяет. Retry: для RSS/Telegram — следующий цикл; покрывает `FM-01`, `REQ-07`.
- `FM-T02` **Веб-извлечение не удалось (смена вёрстки, пустой текст).** Detection: extractor вернул ошибку/пустой текст. Recovery: single retry → при повторной ошибке skip source с логированием (web-policy архитектуры); `Post` не создаётся. Покрывает `FM-02`, `RISK-04`.
- `FM-T03` **Telegram rate-limit / FLOOD_WAIT.** Detection: соответствующая ошибка клиента. Recovery: rate-limit aware ожидание (floodwait), без падения коллектора; достижение лимита логируется, остальные коллекторы не затронуты (`INV-T07`). Покрывает `FM-03`, `RISK-03`, `PCON-02`.
- `FM-T04` **`PostUsecase.Create` вернул `biz.ErrPostSourceNotFound`** (источник удалён между листингом и сохранением). Detection: sentinel-ошибка (`internal/biz/post.go:63`). Recovery: item логируется и пропускается, цикл не падает.
- `FM-T05` **Частичный сбор.** Каждый `Create` — собственная транзакция, поэтому сохранённые посты остаются; упавший item логируется; следующий тик дособирает остаток, дубликаты исключены индексом `(source_id, external_id)` (`FM-04`/`FM-05`, `INV-T03`).
- `FM-T06` **Нетекстовый материал.** `FilterTextItems` отсеивает item без текстового тела до `Create` — без ошибки и без `Post` (`FM-06`, `REQ-10`, `INV-T05`).

## Tests

Стратегия по [testing-policy.md](../../engineering/testing-policy.md): TDD в `biz/`, тесты-после в `data/`/`task/`, AAA, table-driven, mockgen, testcontainers, goleak.

**Unit (biz/, TDD — Red-Green-Refactor):**
- `FilterTextItems` — чистая функция, table-driven: только текст / только нетекст / смешанный набор (`SC-09`, `REQ-10`).
- `CollectUsecase.CollectSource` с моками `Fetcher` + `PostCreator` (`internal/biz/mock/mock_collect.go`):
  - happy: N текстовых items → N вызовов `Create` с корректным маппингом полей (`SC-10`, `REQ-05`);
  - нетекстовый item не доходит до `Create` (`SC-09`);
  - дубликат: `Create` возвращает существующий пост без ошибки → цикл продолжается, без второго эффекта (`SC-05`, `REQ-06`);
  - `Fetcher.Fetch` вернул ошибку → проброшена наверх без логирования (`INV-T04`);
  - `Create` вернул `ErrPostSourceNotFound` → item пропущен, ошибка не фатальна (`FM-T04`).
- Создание `SummaryEvent` для self-contained / отсутствие для cumulative — поведение `PostUsecase.Create`; в biz-юнитах `CollectUsecase` проверяется делегирование, сам outbox-эффект подтверждается интеграционно (см. ниже, `SC-02`/`SC-03`).

**Integration (data/, тесты-после, testcontainers/httptest):**
- `rss_fetcher_test.go` — `httptest.Server` с RSS-фикстурой → корректные `CollectedItem` (external_id, published_at, author, text) (`REQ-01`).
- `web_fetcher_test.go` — `httptest.Server` с HTML-фикстурой → извлечённый текст; отдельный кейс «сломанная вёрстка» → ошибка/пустой текст (`REQ-04`, `FM-T02`).
- `telegram_fetcher_test.go` — стаб/мок Telegram-клиента → маппинг сообщений канала/группы в `CollectedItem` (`REQ-02`, `REQ-03`).

**Task-level (integration, testcontainers + httptest + goleak):**
- `rss_collector_test.go` — полный цикл RSS-коллектора: посты в БД, create-API не вызывался (`CHK-01`, `SC-01`); второй цикл без нового материала → без дубликатов (`CHK-05`, `SC-05`); число тиков соответствует интервалу из конфига через контролируемый интервал/fake-clock (`CHK-08`, `SC-08`).
- `web_collector_test.go` — успешный сбор (`CHK-04`, `SC-04`) и extraction-failure skip без создания постов и без влияния на остальные (`CHK-07`, `SC-07`).
- `telegram_collector_test.go` — канал → self-contained + созданное `summary_events` (`CHK-02`, `SC-02`); группа → cumulative без немедленного per-post события (`CHK-03`, `SC-03`); изоляция отказа: один источник падает, второй собирает, ошибка зафиксирована (`CHK-06`, `SC-06`).
- Во всех task-тестах — `defer goleak.VerifyNone(t, goleak.IgnoreCurrent())` (`INV-T07`).

**Service-level:** не применяется — FT-009 не добавляет и не меняет proto/service (`NS-02`, `api/feedium/*` не в whitelist).

**Build/lint gate:** `make build && make test && make lint` (`CHK-11`, `EVID-11`).

**Manual-only gaps (с причиной, фиксируются в plan через `AG-*`):**
- Авторизация и реальная сессия Telegram userbot против live API — live infra, недетерминированно ([testing-policy.md §Когда Manual-Only Допустим](../../engineering/testing-policy.md#когда-manual-only-допустим)). В автотестах — мок-клиент.
- Сбор с реальных живых веб-сайтов и RSS-эндпоинтов — внешняя сеть; в автотестах — `httptest.Server` с фикстурами.

## Decisions

- `DEC-01` **RSS-парсер.** Кандидат: `github.com/mmcdole/gofeed`. Альтернативы: ручной `encoding/xml` (хрупко к вариантам RSS/Atom/RSS1, переизобретение разбора дат), `gorilla/feeds` (только генерация). Почему: покрывает RSS2/Atom/RSS1 и нормализацию дат «из коробки», изолирован за портом `biz.Fetcher` (легко заменить). Обратимость: высокая (только `data/`). Новая зависимость → согласование (`CTR-T05`).
- `DEC-02` **HTML-извлечение.** Кандидат: `github.com/go-shiori/go-readability`. Альтернативы: `goquery` + ручные CSS-правила (хрупко к вёрстке, `RISK-04`), `colly` (crawler-фреймворк, избыточен). Почему: readability-алгоритм устойчивее ручных селекторов к смене вёрстки, отдаёт текст+заголовок. Обратимость: высокая (за портом). Новая зависимость → согласование.
- `DEC-03` **Telegram-клиент (`OQ-01`).** Кандидат: `github.com/gotd/td`. Альтернативы: `go-tdlib` (CGo + системная `libtdjson`, усложняет server-side сборку, `CON-04`), bot-API клиенты (не читают произвольные каналы/группы под userbot). Почему: чистый Go без CGo, floodwait-middleware для rate-limit контроля (`PCON-02`, `RISK-03`), абстракция хранения сессии. **Самое трудно-обратимое решение** (внешний аккаунт, операционный риск бана) и новая тяжёлая зависимость → **требует human approval** на implement-этапе (фиксируется как `AG-*` в plan), не финализируется этим spec.
- `DEC-04` **Персистентность через `PostUsecase.Create`, а не прямые writes в `CollectUsecase`.** Альтернатива: `CollectUsecase` пишет через `PostRepo` + `SummaryOutboxRepo` + `TxManager` сам. Почему: переиспользование единственного Create-пути даёт outbox/tx/режимы FT-004/FT-005 без дублирования (`INV-T01`, `INV-T03`, `CTR-T03`). Обратимость: высокая.
- `DEC-05` **Без нового поля/курсора сбора; идемпотентность только через `(source_id, external_id)` (`OQ-05`).** Альтернатива: `sources.last_collected_at` или отдельная курсор-таблица. Почему: `CON-05`/`ASM-02`/`INV-T06` требуют неизменности схемы; «новый материал с момента подключения» обеспечивается дедупликацией + (при необходимости) in-memory high-water mark в коллекторе, без персистентного курсора. Эскалация: если для Telegram backfill окажется неограниченным — поднять потребность в служебном поле обратно в feature.md (не silent expansion). **Трудно-обратимо при добавлении схемы → решение «без схемы» предпочтительно для MVP.**
- `DEC-06` **Коллектор на тип как отдельный `transport.Server`.** Альтернатива: единый мультиплексирующий воркер. Почему: изоляция отказов (`REQ-07`), независимые интервалы/retry, отдельный lifecycle userbot-сессии (`DEC-06` ↔ feature.md Solution). Обратимость: средняя (структура `task/`).

## Carried-forward Open Questions

Эти вопросы остаются за implement-этапом (`plan`); spec фиксирует их как явный input, не закрывая молча (см. feature.md `## Open Questions`). Ни один не блокирует начало работ — бизнес-контракт от них не зависит (`NS-05`).

- `OQ-01` Telegram-библиотека — лидирует `gotd/td` (`DEC-03`), финализация требует approval.
- `OQ-02` Единый интервал vs per-type, имена ключей `collectors.*`, дефолтные значения — этот spec задаёт структуру `conf.Collectors` (per-type интервалы), численный дефолт тюнингуется и не поднимается как KPI (`NS-09`).
- `OQ-03` Конкретный способ хранения сессии/секретов Telegram (файл сессии в `configs/`, env, секрет-стор) — server-side, без desktop-only (`CON-04`).
- `OQ-04` Точная стратегия HTML-извлечения и её настройки — лидирует `go-readability` (`DEC-02`).
- `OQ-05` Семантика «нового материала» — решена минимально через дедуп (`DEC-05`), с явной точкой эскалации.

Замечу также рассинхрон именования derived-документа: feature-flow.md называет execution-документ `implementation-plan.md`, а testing-policy.md и этот этап — `spec.md`/`plan.md`. Spec создан по адресу, заданному задачей (`spec.md`); расхождение терминологии в memory-bank — вне scope FT-009, сигнал владельцу процесса (ср. `OQ-06` feature.md).

## Traceability matrix (technical)

| REQ | Layer(s) | SC | CHK | EVID | Tech refs |
|---|---|---|---|---|---|
| `REQ-01` (RSS) | task, data, biz | `SC-01`, `SC-08` | `CHK-01`, `CHK-08` | `EVID-01`, `EVID-08` | `DEC-01`, `CTR-T02`, `FM-T01` |
| `REQ-02` (TG-канал) | task, data, biz | `SC-02` | `CHK-02` | `EVID-02` | `DEC-03`, `CTR-T03`, `FM-T03` |
| `REQ-03` (TG-группа) | task, data, biz | `SC-03` | `CHK-03` | `EVID-03` | `DEC-03`, `CTR-T03` |
| `REQ-04` (веб) | task, data, biz | `SC-04`, `SC-07` | `CHK-04`, `CHK-07` | `EVID-04`, `EVID-07` | `DEC-02`, `FM-T02` |
| `REQ-05` (поля поста) | biz | `SC-10` | `CHK-10` | `EVID-10` | `CTR-T03`, `INV-T01` |
| `REQ-06` (идемпотентность) | biz, data | `SC-05` | `CHK-05` | `EVID-05` | `INV-T03`, `FM-T05` |
| `REQ-07` (изоляция отказа) | task | `SC-06`, `SC-07` | `CHK-06`, `CHK-07` | `EVID-06`, `EVID-07` | `INV-T04`, `INV-T07`, `FM-T01` |
| `REQ-08` (частота из конфига) | configs, task | `SC-08` | `CHK-08` | `EVID-08` | `CTR-T01`, `CTR-T04` |
| `REQ-09` (режимы FT-005) | biz | `SC-02`, `SC-03` | `CHK-02`, `CHK-03` | `EVID-02`, `EVID-03` | `CTR-T03`, `INV-T01` |
| `REQ-10` (только текст) | biz | `SC-09` | `CHK-09` | `EVID-09` | `INV-T05`, `FM-T06` |
| `REQ-11` (автотесты) | biz, data, task | `SC-01`..`SC-10` | `CHK-01`..`CHK-11` | `EVID-01`..`EVID-11` | `## Tests` |

Канонические определения `SC-*`/`CHK-*`/`EVID-*` — в feature.md (`## Verify`); spec не переопределяет их, добавляет только layer-attribution и tech-refs.

## Out of scope

Повтор продуктовых `NS-*` (feature.md, по ID) + технический non-scope.

- `NS-01` AI-скоринг и ранжирование.
- `NS-02` React UI и OpenClaw-бот; FT-009 не добавляет proto/service (`api/feedium/*` вне whitelist).
- `NS-03` Сбор нетекстового контента (`REQ-10`).
- `NS-04` Импорт полного исторического архива.
- `NS-05` Финализация выбора библиотек/сессии/HTML-стратегии — лидеры зафиксированы как `DEC-*`, финализация Telegram-библиотеки — на approval.
- `NS-06` Аутентификация/авторизация пользователей.
- `NS-07` CRUD источников (FT-003) — FT-009 только читает источники.
- `NS-08` Изменение пайплайна FT-005/FT-007 (outbox, lease/retry, `next_summary_at`, cron, Summary Worker lifecycle).
- `NS-09` Подъём `MET-03` PRD-001 и фиксация дефолтной частоты как KPI.

**Технический non-scope:**
- Изменение схемы БД и миграций (`INV-T06`); добавление курсор-поля сбора (`DEC-05`).
- Изменение `.golangci.yml` ([autonomy-boundaries.md](../../engineering/autonomy-boundaries.md) — запрещено без согласования).
- Рефакторинг существующих воркеров FT-007 (`internal/task/event_worker_pool.go`, `source_due_scheduler.go`, `stuck_event_reaper.go`) и пути `PostUsecase.Create`.
- Изменение proto API и `service/` слоя.
- Оптимизация производительности сбора под 10 000 постов/день сверх корректности (`CON-03` — учитывается в plan, а не реализуется как отдельная задача здесь).
