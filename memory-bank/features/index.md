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
| `FT-001` Структура проекта | [FT-001-project-structure/](FT-001-project-structure/) | [PRD-001](../prd/PRD-001-mvp.md) | planned | brief |
