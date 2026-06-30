---
title: "FT-019: Feature Package"
doc_kind: feature
doc_function: index
purpose: "Bootstrap-safe навигация по документации FT-019. Читать, чтобы сначала перейти к canonical feature.md, а derived-документы — по мере появления."
derived_from:
  - ../../dna/governance.md
  - feature.md
status: active
---

# FT-019: Feature Package

## О разделе

Каталог feature package хранит `brief.md` (Brief-стадия), canonical `feature.md` (Design Ready) и `implementation-plan.md` (Plan Ready, появится после согласования design).

## Аннотированный индекс

- [`brief.md`](brief.md)
  Читать, когда нужно: понять исходную проблему и почему она важна, до технического решения.
  Отвечает на вопрос: что сломано и для кого, без привязки к конкретному механизму.

- [`feature.md`](feature.md)
  Читать, когда нужно: открыть выбранное Design-решение, scope, change surface и verify-план для этой delivery-единицы.
  Отвечает на вопрос: как именно cumulative-источники становятся видимы пользователю.

- [`implementation-plan.md`](implementation-plan.md)
  Читать, когда нужно: после появления этого файла — пошаговая реализация с checkpoints.
  Отвечает на вопрос: как провести реализацию от текущего состояния до приёмки.
