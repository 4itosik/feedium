---
title: "FT-021: Feature Package"
doc_kind: feature
doc_function: index
purpose: "Bootstrap-safe навигация по документации FT-021. Читать, чтобы перейти к canonical feature.md."
derived_from:
  - ../../dna/governance.md
  - feature.md
status: active
---

# FT-021: Feature Package

## О разделе

Короткий feature package (шаблон `short.md`): Anthropic как второй LLM-провайдер суммаризации
и выбор провайдера по режиму обработки через конфиг.

## Аннотированный индекс

- [`feature.md`](feature.md)
  Читать, когда нужно: понять, как выбирается LLM-провайдер для self-contained и cumulative
  суммаризации, какие поля конфига за это отвечают и почему скоринг остался на OpenRouter.
  Отвечает на вопрос: как переключить суммаризацию между Anthropic и OpenRouter без правки кода.
