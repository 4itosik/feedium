---
doc_kind: governance
doc_function: index
purpose: Навигация по эталонным шаблонам feature-документации. Читать, чтобы завести новую фичу без изобретения новой структуры.
derived_from:
  - ../../dna/governance.md
  - feature/README.md
  - feature/short.md
  - feature/large.md
  - feature/implementation-plan.md
status: active
---

# Templates Index

Каталог `memory-bank/flows/templates/feature/` хранит эталонные шаблоны feature-пакета. Все шаблоны живут как governed wrapper-документы с `doc_function: template`: у wrapper-а есть собственные purpose, а frontmatter и body инстанцируемого документа — внутри embedded template contract.

## Feature templates

- [feature/README.md](feature/README.md) — шаблон feature-level README (routing-слой пакета). Создаётся вместе с `feature.md`.
- [feature/short.md](feature/short.md) — минимальный canonical feature для небольшой и локальной delivery-единицы.
- [feature/large.md](feature/large.md) — canonical feature с assumptions, blockers, contracts, расширенным verify-слоем.
- [feature/implementation-plan.md](feature/implementation-plan.md) — derived execution-план. Создаётся только после того, как sibling `feature.md` → `status: active`.

Выбор между `short.md` и `large.md` регламентирован правилами в [../feature-flow.md](../feature-flow.md#выбор-шаблона-featuremd).
