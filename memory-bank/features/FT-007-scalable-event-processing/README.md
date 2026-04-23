---
title: "FT-007: Scalable Event Processing Package"
doc_kind: feature
doc_function: index
purpose: "Bootstrap-safe навигация по документации фичи FT-007. Читать, чтобы сначала перейти к canonical `feature.md`, а optional derived docs добавлять только после их появления."
derived_from:
  - ../../dna/governance.md
  - feature.md
status: active
---

# FT-007: Scalable Event Processing

## О разделе

Каталог feature package FT-007 хранит canonical `feature.md`, а optional derived/external routes добавляются только после появления соответствующих документов. Сначала читай `feature.md`, затем расширяй routing по мере появления execution и decision artifacts.

## Аннотированный индекс

- [`feature.md`](feature.md)
  Читать, когда нужно: открыть canonical feature-документ FT-007 после bootstrap пакета.
  Отвечает на вопрос: где находятся scope, design, verify, blockers и canonical IDs для перехода на pull-модель обработки summary-событий и per-source cron-планировщик.

- [`implementation-plan.md`](implementation-plan.md)
  Читать, когда нужно: после того как `feature.md` перешёл в `status: active` — разложить реализацию по шагам, workstreams, checkpoints и traceability к canonical IDs.
  Отвечает на вопрос: как провести реализацию FT-007 от текущего состояния кода до приёмки.
