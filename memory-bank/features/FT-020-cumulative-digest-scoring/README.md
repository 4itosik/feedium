---
title: "FT-020: Feature Package"
doc_kind: feature
doc_function: index
purpose: "Bootstrap-safe навигация по документации FT-020. Читать, чтобы сначала перейти к canonical feature.md, а derived-документы — по мере появления."
derived_from:
  - ../../dna/governance.md
  - feature.md
status: active
---

# FT-020: Feature Package

## О разделе

Каталог feature package хранит canonical `feature.md` (Design Ready) и `implementation-plan.md`
(появится после Plan Ready). Исходная проблема и путь B (уже реализованный, `delivery_status:
done`) — в `../FT-019-cumulative-source-visibility/brief.md`.

## Аннотированный индекс

- [`feature.md`](feature.md)
  Читать, когда нужно: понять, как cumulative-дайджесты получают собственный score и попадают в
  `/v1/feed`, что меняется в схеме и почему.
  Отвечает на вопрос: как OpenClaw-дайджест начинает включать контент групповых чатов без
  изменений на своей стороне.

- [`../../adr/ADR-003-cumulative-digest-scoring.md`](../../adr/ADR-003-cumulative-digest-scoring.md)
  Читать, когда нужно: понять governance-решение, которое расширяет ADR-001-инвариант
  «score = один пост».
  Отвечает на вопрос: почему выбран ключ `summary_id`, а не `source_id` или синтетический пост.

- [`implementation-plan.md`](implementation-plan.md)
  Читать, когда нужно: после появления этого файла — пошаговая реализация с checkpoints.
  Отвечает на вопрос: как провести миграцию и код-изменения от текущего состояния до приёмки.
