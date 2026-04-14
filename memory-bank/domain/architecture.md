---
doc_kind: domain
doc_function: canonical
purpose: Каноничное место для архитектурных границ Feedium. Читать при изменениях, затрагивающих модули, фоновые процессы, интеграции или конфигурацию.
derived_from:
  - ../dna/governance.md
status: active
---

# Architecture Patterns

## Module Boundaries

Система — один Go-процесс (monolith) на go-kratos. Модули изолированы через интерфейсы и kratos layout.

| Context | Owns | Must not depend on directly |
| --- | --- | --- |
| `biz` | доменные сущности, бизнес-правила, usecase-ы, интерфейсы репозиториев и внешних сервисов | ent, HTTP, SQL, proto, конкретные клиенты |
| `data` | реализация репозиториев (Ent), внешние клиенты (LLM, Telegram, HTTP) | бизнес-логика, proto DTO |
| `service` | адаптер proto DTO → доменные объекты → biz, маппинг ответов | прямые вызовы data, бизнес-логика |
| `task` | воркеры-коллекторы (Telegram userbot, RSS, web scraping), summary worker; lifecycle через kratos transport.Server | прямые запросы к БД в обход biz, собственная бизнес-логика (вызывает biz/ usecases) |
| `server` | настройка HTTP/gRPC серверов, middleware, interceptors, регистрация сервисов | бизнес-логика |

Правила:

- модуль владеет своим state и публичными контрактами (интерфейсы в `biz/`);
- межмодульные зависимости — через явно названный интерфейс, определённый в `biz/`;
- `task/` вызывает `biz/` usecase-ы, не работает с `data/` напрямую;
- `service/` не знает про `data/` — только `biz/`.

## Concurrency And Critical Sections

### Воркеры (task/)

Воркеры-коллекторы реализуют `transport.Server` — kratos вызывает `Start()/Stop()`. Kratos управляет lifecycle.

- Каждый коллектор работает в своей горутине (kratos запускает `Start()` в отдельной горутине).
- Shared state — только через БД (PostgreSQL). In-process shared state минимален.
- Outbox pattern: `Post` + `SummaryEvent` в одной транзакции для гарантии доставки на обработку.

### Job queue / Scheduling

В MVP — polling-based: воркеры крутятся в цикле с configurable interval. Отдельная очередь (Redis, NATS) не планируется в MVP.

- Cumulative summary worker: опрашивает БД по крону (интервал из конфига), находит накопительные материалы, готовые к суммаризации.
- Идемпотентность: повторный запуск воркера не создаёт дубликаты — `SummaryEvent` уникален по `(post_id, event_type)`.

### Критические секции

- БД-транзакции (Ent Tx): `Post` + `SummaryEvent` создаются атомарно. Retry — на уровне БД, не в приложении.
- External API calls (Telegram, LLM): rate limiting, exponential backoff. Ретраи — в `task/`, не в leaf-функциях.

Запрещено:

- Shared mutable state между горутинами без явного mutex.
- Блокирующие вызовы к внешним API в request path (HTTP handler).

## Failure Handling And Error Tracking

Подход по слоям (см. `../engineering/coding-style.md`):

| Слой | Правило |
| --- | --- |
| `data/` | Только возвращает ошибку наверх, не логирует. Оборачивает: `fmt.Errorf("postRepo.Save: %w", err)` |
| `biz/` | Оборачивает бизнес-контекстом, не логирует. Конвертирует инфраструктурные ошибки в доменные |
| `service/` | Конвертирует доменные ошибки в kratos status errors. Логирует unexpected errors |
| `task/` | Точка логирования и принятия решения: логировать, ретраить, менять state. Leaf-функции не логируют |

Ответ на canonical вопрос:

> Нужно ли вручную логировать ошибку в воркере, если базовый код уже делает retries?

Нет. Логирование и retry — ответственность `task/`. Leaf-функции (`biz/`, `data/`) не логируют ошибки — только возвращают.

### Retry policy

- LLM API: exponential backoff, 3 попытки, настраиваемый timeout.
- Telegram API: rate limit aware, отдельный аккаунт.
- Web scraping: single retry, при повторной ошибке — skip source, логировать.
- DB: Ent retry не дублируется локальным `rescue` в `data/`.

## Configuration Ownership

См. [Coding Style — Конфигурация](../engineering/coding-style.md#конфигурация).
