---
title: "FT-011: React UI Feature Package"
doc_kind: feature
doc_function: index
purpose: "Bootstrap-safe навигация по документации FT-011 React UI. Читать, чтобы сначала перейти к canonical feature.md, а optional derived docs добавлять только после их появления."
derived_from:
  - ../../dna/governance.md
  - feature.md
status: active
---

# FT-011: React UI Feature Package

## О разделе

Каталог feature package хранит canonical `feature.md` и execution `implementation-plan.md`. Сначала читай `feature.md`, затем расширяй routing по мере появления execution и decision artifacts.

## Аннотированный индекс

- [`feature.md`](feature.md)
  Читать, когда нужно: открыть instantiated canonical feature-документ для получения scope, design, verify, blockers и canonical IDs.
  Отвечает на вопрос: что входит в FT-011, как технически построена SPA, какие acceptance scenarios.

- [`implementation-plan.md`](implementation-plan.md)
  Читать, когда нужно: разложить реализацию по шагам, workstreams, checkpoints и traceability к canonical IDs.
  Отвечает на вопрос: как провести реализацию FT-011 от текущего состояния до приёмки.
