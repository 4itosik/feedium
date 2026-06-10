---
doc_kind: feature
doc_function: readme
status: backlog
delivery_status: backlog
derived_from:
  - ../../prd/PRD-001-mvp.md
---

# FT-017: Open Original URL

## Problem

В карточке ленты нет ссылки на оригинальный материал. Если summary заинтриговал — некуда кликнуть чтобы прочитать полную статью или открыть Telegram-пост.

## Outcome

В карточке Feed — иконка/ссылка "Открыть оригинал", открывающая источник в новой вкладке.

## Scope

- Добавить поле `url` (или `externalUrl`) в `FeedItem` API response
- Для Telegram: `https://t.me/{username}/{message_id}`
- Для RSS/HTML: URL статьи из фида
- В `FeedCard` — кнопка "↗ Оригинал" (только если url присутствует)
- Затрагивает бэкенд (поле в proto + feed service) и фронтенд

## Notes

- Это бэкенд-фича в отличие от FT-014, FT-015, FT-016
- `Post.externalId` уже есть — для Telegram это message ID; нужно добавить username источника и формировать URL
