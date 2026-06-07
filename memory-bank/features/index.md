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
| `FT-013` Post Embedding Persistence | [FT-013-embedding-persistence/](FT-013-embedding-persistence/) | [PRD-001](../prd/PRD-001-mvp.md) | planned | [brief](FT-013-embedding-persistence/brief.md) |
