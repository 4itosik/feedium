---
doc_kind: feature
doc_function: readme
status: backlog
delivery_status: backlog
derived_from:
  - ../../prd/PRD-001-mvp.md
---

# FT-015: Source Name in Feed

## Problem

Feed view показывает `sourceId` как обрезанный UUID (`019eada8-0c88-7b...`). Пользователь не понимает, из какого источника пришёл пост — нет контекста канала/сайта.

## Outcome

В карточке ленты вместо UUID виден читаемый ярлык источника: `@channelname`, `HN RSS`, `example.com`.

## Scope

- При загрузке Feed view — один запрос `GET /v1/sources` для получения маппинга id → label
- Функция `sourceLabel(source)` уже есть в `PostsView.tsx` — переиспользовать
- Показывать label в `FeedCard` вместо `item.sourceId`
- Только фронтенд — бэкенд не меняется

## Notes

- Sources уже грузятся на странице (видно в Network: `sources?page_size=100`), но результат не пробрасывается в FeedCard
