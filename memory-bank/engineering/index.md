---
doc_kind: engineering
doc_function: index
purpose: Навигация по engineering-level документации Feedium.
derived_from:
  - ../dna/governance.md
status: active
---

# Engineering Documentation Index

Каталог `memory-bank/engineering/` содержит инженерные правила проекта Feedium.

- [Autonomy Boundaries](autonomy-boundaries.md) — автопилот, супервизия, эскалация агента; правило TDD-цикла для biz/. Читать перед любым действием, которое может требовать согласования.
- [Coding Style](coding-style.md) — kratos layout, правила по слоям (biz/data/service/task), именование, error handling, logging, DI. Читать при написании любого кода в проекте.
- [API Contracts](api-contracts.md) — proto-файлы: структура, генерация, версионирование методов (V1/V2), HTTP annotations. Читать при добавлении или изменении API.
- [Testing Policy](testing-policy.md) — TDD для biz/, тесты-после для остального, стек (testify, mockgen, testcontainers, goleak), AAA-паттерн. Читать при написании тестов или оценке покрытия.
- [Database](database.md) — Ent ORM (схемы, генерация), goose-миграции, PostgreSQL conventions, constraints. Читать при создании или изменении схемы БД и написании миграций.
- [Git Workflow](git-workflow.md) — Conventional Commits, squash merge, требования к PR. Читать при оформлении коммитов и pull requests.
