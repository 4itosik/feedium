---
slug: FT-019-cumulative-source-visibility
title: "Cumulative Telegram Group Posts Missing From Feed"
doc_kind: feature
doc_function: brief
purpose: "Feature brief: посты cumulative-источников (telegram_group) собираются, но никогда не получают score и не попадают в ranked feed/выборку; агрегированные source-level дайджесты честно создаются, но нигде не читаются. Читать перед проектированием FT-019."
derived_from:
  - ../../prd/PRD-001-mvp.md
  - ../../domain/problem.md
  - ../FT-005-ai-summarization/brief.md
  - ../FT-009-collectors/feature.md
  - ../FT-010-ai-scoring/brief.md
status: active
delivery_status: planned
---

# FT-019: Cumulative Telegram Group Posts Missing From Feed

## Problem

Cumulative-режим (`FT-005`, назначается источникам `telegram_group` через `BR-01`) считает summary **на уровне source целиком** за окно времени (`SummaryUsecase.ProcessSourceEvent` сохраняет `Summary{PostID: nil}`), а не на уровне отдельного поста. Scoring outbox (`FT-007`/`FT-010`) ставит в очередь на оценку только посты, у которых есть summary с `post_id IS NOT NULL` (`dueCandidatePostIDs`, `internal/data/score_outbox_repo.go`) — то есть **ни один пост cumulative-источника никогда не получает score**. Ranked feed (`ListRanked`, `internal/data/score_repo.go`) — это INNER JOIN постов и scores: без score пост структурно не может туда попасть. При этом source-level cumulative-summary честно создаются и сохраняются, но ни один сервис или эндпоинт их не читает — пользователь никогда их не видит ни в каком виде.

Подтверждено на проде 2026-06-30 прямым SQL-запросом к БД: 9 источников `telegram_group` → 583 поста, из них 0 с привязанным к посту summary, 0 со score, 0 в фиде. Для сравнения: 16 источников `telegram_channel` (self-contained) → 38 постов, у всех есть и summary, и score — этот путь работает штатно. 61 cumulative source-level summary существует в БД и не используется ни в одном продуктовом сценарии.

Боль конкретна: целый класс источников пользователя — групповые Telegram-чаты — полностью невидим в продукте, при этом коллектор и LLM честно тратят ресурсы на их обработку.

## Task

Сделать так, чтобы контент, собранный из cumulative-источников, был наблюдаемо виден пользователю — либо как индивидуально оценённые посты в общей ranked-ленте, либо как читаемый дайджест источника в отдельном представлении. Конкретный путь — открытый вопрос, который должна закрыть Design-фаза.

## Context

- Обнаружено при разборе жалобы «посты из Telegram не попадают в выборку»; расследование подтвердило структурный (не временный, не конфигурационный) разрыв — коллектор и LLM-пайплайн работают исправно.
- Cumulative-режим введён в `FT-005`, чтобы объединять шумные групповые чаты в один дайджест вместо оценки каждого сообщения по отдельности. На момент его проектирования не было явно решено, что итоговый дайджест должен быть так же виден пользователю, как self-contained-посты — отсюда разрыв.
- `FT-009` DEC-08 (push-коллектор Telegram) опирается на то же разделение self-contained/cumulative по типу источника (`BR-01`) и корректно работает с обеих сторон сбора — разрыв целиком на стороне summary → score → feed, не в коллекторе.

## Users

- Пользователь Feedium — настроил источники типа «групповой чат» (9 шт. в проде), ожидает увидеть их контент в ленте или дайджесте наравне с каналами и RSS, но не видит ничего.

## Scope

### In Scope

- Путь от cumulative-источника до видимого пользователю результата (лента и/или отдельное дайджест-представление) — выбор конкретного механизма за Design-фазой.
- Сохранение текущего поведения self-contained-источников (`telegram_channel`, `rss`, `web`) без изменений.

### Out Of Scope

- Изменение алгоритма скоринга/релевантности (`FT-010`) для self-contained-источников.
- Изменение параметров cumulative-окна (`max_window`, `max_input_chars`), не связанное с видимостью результата.
- Конкретный UI-дизайн представления — детали за Design (и `FT-011`/`FT-012` при необходимости).

## Acceptance Criteria

- Контент, собранный из `telegram_group`-источника, наблюдаемо доступен пользователю (в ленте, дайджесте или ином виде, определённом Design) в пределах обычного цикла обработки, а не остаётся невидимым.
- Self-contained источники (`telegram_channel`, `rss`, `web`) продолжают работать без регрессий: поведение скоринга, идемпотентности и устойчивости к сбою, зафиксированное в `FT-010`/`FT-007`, не меняется.
- Сценарий покрыт автотестом согласно testing-policy проекта.

## Pre-existing Assets

- `FT-005` — AI Summarization: разделение self-contained/cumulative, `SummaryUsecase.ProcessSourceEvent` (cumulative) vs `ProcessPostEvent` (self-contained).
- `FT-007` — Scalable Event Processing: outbox/scheduler-паттерн, двигающий события через пайплайн.
- `FT-009` — Content Collectors: `BR-01` назначает cumulative-режим типу `telegram_group`; push-коллектор сохраняет посты независимо от режима обработки.
- `FT-010` — AI-скоринг: `ScoreUsecase.ScorePost`, `dueCandidatePostIDs` (`internal/data/score_outbox_repo.go`), `ListRanked` (`internal/data/score_repo.go`) — точка, где cumulative-посты структурно отсекаются.
