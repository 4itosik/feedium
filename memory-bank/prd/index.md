---
doc_kind: prd
doc_function: index
purpose: Навигация по instantiated PRD проекта. Читать, чтобы найти существующий Product Requirements Document или завести новый по шаблону.
derived_from:
  - ../dna/governance.md
status: active
---

# Product Requirements Documents Index

Каталог `memory-bank/prd/` хранит instantiated PRD проекта.

PRD нужен, когда задача живет на уровне продуктовой инициативы или capability, а не одного vertical slice. Обычно PRD стоит между общим контекстом из [`../domain/problem.md`](../domain/problem.md) и downstream feature packages.

## Граница С `domain/problem.md`

- [`../domain/problem.md`](../domain/problem.md) остается project-wide документом и не превращается в PRD.
- PRD наследует этот контекст через `derived_from`, но фиксирует только initiative-specific проблему, users, goals и scope.
- Если документ нужен только для того, чтобы повторить общий background проекта, оставайся на уровне `domain/problem.md`.

## Когда Заводить PRD

- инициатива распадается на несколько feature packages;
- нужно зафиксировать users, goals, product scope и success metrics до проектирования реализации;
- есть риск смешать продуктовые требования с architecture/design detail.

## Когда PRD Не Нужен

- задача локальна и полностью помещается в один `feature.md`;
- общий продуктовый контекст уже покрыт [`../domain/problem.md`](../domain/problem.md), а feature не требует отдельного product-layer документа.

## Naming

- Формат файла: `PRD-XXX-short-name.md`
- Вместо `XXX` используй идентификатор, принятый в проекте: initiative id, epic id или другой стабильный ключ
- Один PRD может быть upstream для нескольких feature packages

## Active PRDs

| PRD | Initiative | Status |
|---|---|---|
| [PRD-001-mvp.md](PRD-001-mvp.md) | Feedium MVP — агрегатор контента с AI-суммаризацией и ранжированием | active |

## Template

```markdown
---
doc_kind: prd
doc_function: template
purpose: Governed wrapper-шаблон PRD. Читать, чтобы инстанцировать компактный Product Requirements Document без смешения wrapper-метаданных и frontmatter будущего PRD.
derived_from:
  - ../dna/governance.md
  - ../dna/frontmatter.md
  - ../domain/problem.md
status: active
---

# PRD-XXX: Product Initiative Name

Этот файл описывает wrapper-template. Инстанцируемый PRD живет ниже как embedded contract и копируется без wrapper frontmatter и history.

## Wrapper Notes

PRD в этом шаблоне intentionally lean. Он фиксирует продуктовую проблему, пользователей, goals, scope и success metrics, но не берет на себя implementation sequencing, architecture decisions или verify/evidence contracts downstream feature package.

PRD опирается на `domain/problem.md`, а не подменяет его. Не копируй в него весь project-wide контекст, если он уже стабильно описан upstream.

Используй PRD как upstream-слой между общим контекстом проекта и несколькими feature packages. Если инициатива локальна и не требует отдельного product-layer документа, PRD можно не создавать.

## Instantiated Frontmatter

```yaml
doc_kind: prd
doc_function: canonical
purpose: "Фиксирует продуктовую проблему, целевых пользователей, goals, scope и success metrics инициативы."
derived_from:
  - ../domain/problem.md
status: draft
```

## Instantiated Body

```markdown
# PRD-XXX: Product Initiative Name

## Problem

Какую пользовательскую или бизнес-проблему решает инициатива. Описывай язык проблемы, а не решение. Ссылайся на общий контекст из `../domain/problem.md` и фиксируй только delta этой инициативы.

## Users And Jobs

Кто является основным пользователем и какую работу он пытается выполнить.

| User / Segment | Job To Be Done | Current Pain |
| --- | --- | --- |
| `primary-user` | Что хочет сделать | Что мешает сегодня |

## Goals

- `G-01` Какой продуктовый outcome обязателен.
- `G-02` Какой дополнительный outcome желателен.

## Non-Goals

- `NG-01` Что сознательно не входит в инициативу.
- `NG-02` Что нельзя молча додумывать на уровне реализации.

## Product Scope

Опиши scope на уровне capability, а не change set.

### In Scope

- Что должно стать возможным для пользователя или системы.

### Out Of Scope

- Что остается за границами инициативы.

## UX / Business Rules

- `BR-01` Важное правило продукта или операции.
- `BR-02` Ограничение, которое должна уважать любая downstream feature.

## Success Metrics

| Metric ID | Metric | Baseline | Target | Measurement method |
| --- | --- | --- | --- | --- |
| `MET-01` | Что измеряем | От чего стартуем | Что считаем успехом | Как проверяем |

## Risks And Open Questions

- `RISK-01` Что может сорвать инициативу на уровне продукта.
- `OQ-01` Какая неизвестность еще не снята.

## Downstream Features

Перечисли ожидаемые feature packages, если они уже понятны.

| Feature | Why it exists | Status |
| --- | --- | --- |
| `FT-XXX` | Какой slice реализует | planned / draft / active |
```