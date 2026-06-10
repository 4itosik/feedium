---
doc_kind: feature
doc_function: readme
status: backlog
delivery_status: backlog
derived_from:
  - ../../prd/PRD-001-mvp.md
---

# FT-018: Mark as Read

## Problem

Каждый день при открытии Feed — те же посты, что вчера. Нет разграничения "уже видел / новое". Пользователь тратит время на повторный просмотр прочитанного контента.

## Outcome

При следующем открытии Feed виден только непрочитанный контент. Прочитанное помечено и скрыто по умолчанию (с возможностью показать).

## Scope

- Новый endpoint `POST /v1/posts/{id}/read` (или batch `POST /v1/posts/read`)
- Поле `read_at` на посте в БД
- Фильтр `unread_only=true` в `GET /v1/feed`
- В Feed view: клик по карточке / кнопка "✓" → пометить прочитанным, карточка гаснет/исчезает
- Опция "Показать прочитанное" (toggle)
- Затрагивает бэкенд (миграция + endpoint) и фронтенд

## Open Questions

- Хранить в БД или в localStorage? БД — надёжнее; localStorage — нет бэкенд-спринта.
- Вариант минимума: хранить список прочитанных ID в localStorage и фильтровать на клиенте.
