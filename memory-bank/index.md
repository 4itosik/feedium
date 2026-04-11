---
doc_kind: governance
doc_function: index
purpose: Корневая навигация по memory-bank Feedium. Читать сначала, чтобы понять структуру и найти нужный документ.
derived_from:
  - dna/principles.md
status: active
---

# Feedium — Documentation Index

Каталог `memory-bank/` — SSoT проектной документации Feedium. Каждый факт принадлежит ровно одному canonical документу; дублирование — дефект.

## C4 Level 0 — System Context

**Feedium** — персональный агрегатор контента для одного пользователя. Собирает посты из Telegram (userbot), веб-сайтов и RSS → ранжирует через AI (embeddings + интересы) → отдаёт в React UI и Telegram-бот OpenClaw.

**Стек:** Go + go-kratos (Wire, lifecycle, HTTP/gRPC) · Protobuf · Ent + PostgreSQL 18.3 · goose (SQL-миграции) · React SPA (go:embed) · slog.

**Архитектура:** один процесс, kratos-layout. Коллекторы (task/) реализуют `transport.Server` — lifecycle управляется фреймворком. Доменная логика в biz/ без импорта инфраструктуры. Репозитории в data/ через Ent. Тонкий API-слой в service/ из proto-файлов.

```
[Telegram channels] ──┐
[Web sites]         ──┼──► [Collector workers] ──► [PostgreSQL + AI scoring] ──► [React UI / OpenClaw bot]
[RSS feeds]         ──┘         task/                  biz/ + data/                service/ + HTTP
```

**Внешние сервисы:** Telegram API (userbot), LLM-провайдер (embeddings + scoring), OpenClaw (отдельный репозиторий).

---

## Навигация по разделам

### [DNA — конституция документации](dna/index.md)

- [Principles](dna/principles.md) — 9 фундаментальных принципов: SSoT, атомарность, progressive disclosure, code vs docs, index-first. Читать первым для понимания правил, по которым живёт вся документация проекта.
- [Document Governance](dna/governance.md) — SSoT implementation (authority, status, dependency tree), поле `derived_from`, governance-specific frontmatter. Читать, чтобы узнать, кто владеет фактом и как разрешаются конфликты между документами.
- [Frontmatter Schema](dna/frontmatter.md) — Schema обязательных и условных полей YAML frontmatter (`status`, `derived_from`, `delivery_status`, `decision_status`). Читать при создании или обновлении любого документа.
- [Document Lifecycle](dna/lifecycle.md) — Maintenance rules (upstream first, downstream sync, README sync) и sync checklist. Читать при изменениях в документации для проверки consistency.
- [Cross-references](dna/cross-references.md) — Правила двусторонней навигации code ↔ docs: ссылки из кода на спеки, из документации на реализацию. Читать при расстановке ссылок между кодом и документацией.
- [Dependency Tree](dna/dependency-tree.md) — Визуализация дерева зависимостей (derived_from) всех governed-документов. Читать, чтобы понять иерархию authority и порядок обновления при изменениях.

### [Domain — продуктовый контекст и архитектура](domain/index.md)

- [Project Problem Statement](domain/problem.md) — Каноничное описание Feedium: продукт, проблемное пространство, core workflows (WF-01..03), outcomes (MET-01..05), constraints (PCON-01..06). Отправная точка для любой задачи — зачем существует продукт и что нельзя ломать.
- [Architecture Patterns](domain/architecture.md) — Архитектурные границы MVP: module boundaries (biz/data/service/task/server), concurrency model, failure handling по слоям, retry policy, configuration ownership. Читать при любом изменении, затрагивающем модули, воркеры или конфигурацию.
- [Глоссарий](domain/glossary.md) — Единый словарь 30+ терминов: доменные сущности (Source, Post, Summary, Score), режимы обработки (self-contained, cumulative), стек (go-kratos, Ent, goose, Wire). Читать при неоднозначности термина или разночтениях.

### [Engineering — инженерные правила](engineering/index.md)

- [Autonomy Boundaries](engineering/autonomy-boundaries.md) — Границы автономии агента: автопилот (без подтверждения), супервизия (покажи на КТ), эскалация (остановись и спроси). TDD-цикл в biz/. Читать перед любым действием, которое может требовать согласования.
- [Coding Style](engineering/coding-style.md) — kratos layout, правила по слоям (biz/data/service/task/server), именование, значения vs указатели, чистые функции в biz/, DI (Wire), error handling по слоям, logging (slog), graceful shutdown. Читать при написании любого кода в проекте.
- [API Contracts](engineering/api-contracts.md) — Proto-файлы: расположение (`api/feedium/`), генерация (`make proto`), версионирование методов (V1/V2 в имени, не в пакете), HTTP annotations, структура типичного proto, error reasons. Читать при добавлении или изменении API.
- [Database](engineering/database.md) — Ent ORM (схемы, генерация, паттерн использования в data/), goose-миграции (только SQL, формат), PostgreSQL conventions (BIGSERIAL, TIMESTAMPTZ, JSONB), constraints. Читать при создании или изменении схемы БД и написании миграций.
- [Testing Policy](engineering/testing-policy.md) — TDD для biz/ (Red-Green-Refactor), тесты-после для data/service/task/, стек (testify, mockgen, testcontainers, goleak), AAA-паттерн, table-driven tests, goroutine leak detection, naming конвенции. Читать при написании любого теста.
- [Git Workflow](engineering/git-workflow.md) — Conventional Commits (English, present tense, ≤72 chars), squash merge, требования к PR (зелёные проверки, предметный title, body с what/why/risks). Читать при оформлении коммитов и pull requests.

### [PRD — продуктовые инициативы](prd/index.md)

- [PRD-001: Feedium MVP](prd/PRD-001-mvp.md) — MVP initiative: проблема (10+ источников, 80%+ шум), goals (G-01..03), scope (источники, коллекторы, AI-суммаризация, скоринг, React UI, OpenClaw), business rules (BR-01..06), risks (RISK-01..05), downstream features (FT-001..011). Читать перед началом работы над любой фичей.

### [ADR — архитектурные решения](adr/index.md)

Индекс Architecture Decision Records: naming (ADR-XXX), statuses (proposed/accepted/superseded/rejected), полный шаблон ADR. Читать, чтобы найти принятое решение или завести новое ADR.
