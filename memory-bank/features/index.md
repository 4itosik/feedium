---
doc_kind: feature
doc_function: index
purpose: Навигация по feature packages проекта. Читать, чтобы найти существующий feature package или завести новый.
derived_from:
  - ../dna/governance.md
status: active
---

# Feature Packages Index

Каталог `memory-bank/features/` хранит feature packages — downstream-документы от PRD.

## Структура feature package

Каждая фича — отдельный каталог `FT-XXX-short-name/`, внутри которого лежат три governed-документа:

| Файл | Назначение | Когда создаётся |
|---|---|---|
| `brief.md` | Задача, проблема, scope, ограничения, acceptance criteria. Что и зачем делаем. | Первым, до проектирования |
| `spec.md` | Технический контракт: API, данные, поведение, edge cases. Как фича выглядит снаружи. | После согласования brief |
| `implementation-plan.md` | Пошаговый план реализации: последовательность изменений, файлы, точки верификации. | После согласования spec, перед кодом |

Файлы создаются по мере готовности — отсутствие `spec.md` или `implementation-plan.md` означает, что соответствующий этап ещё не пройден.

## Naming

- Каталог фичи: `FT-XXX-short-name/`
- `XXX` — номер из таблицы Downstream Features в PRD
- Один каталог = один vertical slice

## Active Features

| Feature | Package | Upstream PRD | Delivery Status | Artifacts |
|---|---|---|---|---|
| `FT-001` Структура проекта | [FT-001-project-structure/](FT-001-project-structure/) | [PRD-001](../prd/PRD-001-mvp.md) | done | [brief](FT-001-project-structure/brief.md) · [spec](FT-001-project-structure/spec.md) · [implementation-plan](FT-001-project-structure/implementation-plan.md) |
| `FT-002` Health Check | [FT-002-health-check/](FT-002-health-check/) | [PRD-001](../prd/PRD-001-mvp.md) | done | [brief](FT-002-health-check/brief.md) · [spec](FT-002-health-check/spec.md) · [implementation-plan](FT-002-health-check/implementation-plan.md) |
| `FT-003` Source Management | [FT-003-source-management/](FT-003-source-management/) | [PRD-001](../prd/PRD-001-mvp.md) | done | [brief](FT-003-source-management/brief.md) |
| `FT-004` Posts Management | [FT-004-posts-management/](FT-004-posts-management/) | [PRD-001](../prd/PRD-001-mvp.md) | done | [brief](FT-004-posts-management/brief.md) |
| `FT-005` AI Summarization | [FT-005-ai-summarization/](FT-005-ai-summarization/) | [PRD-001](../prd/PRD-001-mvp.md) | done | [brief](FT-005-ai-summarization/brief.md) |
| `FT-006` CLI Client (feediumctl) | [FT-006-cli-client/](FT-006-cli-client/) | [PRD-001](../prd/PRD-001-mvp.md) | planned | [README](FT-006-cli-client/README.md) · [feature](FT-006-cli-client/feature.md) · [spec](FT-006-cli-client/spec.md) · [implement](FT-006-cli-client/implement.md) · [handoff](FT-006-cli-client/handoff.md) |
| `FT-007` Scalable Event Processing | [FT-007-scalable-event-processing/](FT-007-scalable-event-processing/) | [PRD-001](../prd/PRD-001-mvp.md) | done | [feature](FT-007-scalable-event-processing/feature.md) · [implementation-plan](FT-007-scalable-event-processing/implementation-plan.md) |
| `FT-008` Summarization Prompts Split | [FT-008/](FT-008/) | [PRD-001](../prd/PRD-001-mvp.md) | done | [feature](FT-008/feature.md) · [spec](FT-008/spec.md) · [implement](FT-008/implement.md) |
| `FT-009` Content Collectors | [FT-009-collectors/](FT-009-collectors/) | [PRD-001](../prd/PRD-001-mvp.md) | done | [brief](FT-009-collectors/brief.md) · [feature](FT-009-collectors/feature.md) · [spec](FT-009-collectors/spec.md) · [implementation-plan](FT-009-collectors/implementation-plan.md) · [tg-push-design](FT-009-collectors/2026-05-26-telegram-push-design.md) |
| `FT-011` React UI | [FT-011-react-ui/](FT-011-react-ui/) | [PRD-001](../prd/PRD-001-mvp.md) | done | [README](FT-011-react-ui/README.md) · [feature](FT-011-react-ui/feature.md) · [implementation-plan](FT-011-react-ui/implementation-plan.md) |
| `FT-013` Post Embedding Persistence | [FT-013-embedding-persistence/](FT-013-embedding-persistence/) | [PRD-001](../prd/PRD-001-mvp.md) | planned | [README](FT-013-embedding-persistence/README.md) · [brief](FT-013-embedding-persistence/brief.md) · [feature](FT-013-embedding-persistence/feature.md) · [implementation-plan](FT-013-embedding-persistence/implementation-plan.md) · [ADR-002](../adr/ADR-002-post-embedding-persistence.md) |
| `FT-014` Date quick filters | [FT-014-date-quick-filters/](FT-014-date-quick-filters/) | [PRD-001](../prd/PRD-001-mvp.md) | backlog | [README](FT-014-date-quick-filters/README.md) |
| `FT-015` Source name in feed | [FT-015-source-name-in-feed/](FT-015-source-name-in-feed/) | [PRD-001](../prd/PRD-001-mvp.md) | backlog | [README](FT-015-source-name-in-feed/README.md) |
| `FT-016` Feed stats | [FT-016-feed-stats/](FT-016-feed-stats/) | [PRD-001](../prd/PRD-001-mvp.md) | backlog | [README](FT-016-feed-stats/README.md) |
| `FT-017` Open original URL | [FT-017-open-original-url/](FT-017-open-original-url/) | [PRD-001](../prd/PRD-001-mvp.md) | backlog | [README](FT-017-open-original-url/README.md) |
| `FT-018` Mark as read | [FT-018-mark-as-read/](FT-018-mark-as-read/) | [PRD-001](../prd/PRD-001-mvp.md) | backlog | [README](FT-018-mark-as-read/README.md) |
| `FT-019` Cumulative Telegram Group Posts Missing From Feed | [FT-019-cumulative-source-visibility/](FT-019-cumulative-source-visibility/) | [PRD-001](../prd/PRD-001-mvp.md) | done | [README](FT-019-cumulative-source-visibility/README.md) · [brief](FT-019-cumulative-source-visibility/brief.md) · [feature](FT-019-cumulative-source-visibility/feature.md) · [implementation-plan](FT-019-cumulative-source-visibility/implementation-plan.md) |
| `FT-020` Cumulative Digest Scoring | [FT-020-cumulative-digest-scoring/](FT-020-cumulative-digest-scoring/) | [PRD-001](../prd/PRD-001-mvp.md) | done | [README](FT-020-cumulative-digest-scoring/README.md) · [feature](FT-020-cumulative-digest-scoring/feature.md) · [plan](FT-020-cumulative-digest-scoring/implementation-plan.md) · [ADR-003](../adr/ADR-003-cumulative-digest-scoring.md) |
| `FT-021` Anthropic LLM Provider | [FT-021-anthropic-llm-provider/](FT-021-anthropic-llm-provider/) | [PRD-001](../prd/PRD-001-mvp.md) | in_progress | [README](FT-021-anthropic-llm-provider/README.md) · [feature](FT-021-anthropic-llm-provider/feature.md) |
