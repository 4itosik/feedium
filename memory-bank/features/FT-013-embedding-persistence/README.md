---
title: "FT-013: Feature Package"
doc_kind: feature
doc_function: index
purpose: "Bootstrap-safe навигация по документации FT-013 (персистентность эмбеддинга поста). Читать, чтобы перейти к upstream brief и canonical feature.md, а derived docs (ADR, implementation-plan) использовать по мере их появления."
derived_from:
  - ../../dna/governance.md
  - feature.md
status: active
---

# FT-013: Feature Package

## О разделе

Каталог feature package хранит upstream `brief.md`, canonical `feature.md` и ссылку на связанный ADR; optional derived docs (`implementation-plan.md`) добавляются только после их появления. Сначала читай `brief.md` (зачем фича), затем `feature.md` (scope, design, verify, canonical IDs), затем `ADR-002` (зафиксированное решение по хранению вектора и переоценке).

## Аннотированный индекс

- [`brief.md`](brief.md)
  Читать, когда нужно: понять проблему и целевой исход фичи до проектирования.
  Отвечает на вопрос: зачем сохранять эмбеддинг поста и какую боль это снимает.

- [`feature.md`](feature.md)
  Читать, когда нужно: открыть canonical feature-документ со scope, design, verify, blockers и canonical IDs.
  Отвечает на вопрос: что строим, как и как проверяем; от какого ADR зависит дизайн.

- [`../../adr/ADR-002-post-embedding-persistence.md`](../../adr/ADR-002-post-embedding-persistence.md)
  Читать, когда нужно: проверить зафиксированное решение по хранению вектора поста, инвалидации, согласованности модели и авто-переоценке и его `decision_status`.
  Отвечает на вопрос: почему вектор хранится именно так; `decision_status: accepted` (2026-06-07).

- [`implementation-plan.md`](implementation-plan.md)
  Читать, когда нужно: разложить реализацию по шагам (TDD), workstreams, checkpoints и traceability к canonical IDs.
  Отвечает на вопрос: как провести реализацию FT-013 от текущего состояния до приёмки.
