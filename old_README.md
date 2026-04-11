# Feedium

[![CI](https://github.com/4itosik/feedium/actions/workflows/ci.yml/badge.svg)](https://github.com/4itosik/feedium/actions/workflows/ci.yml)

Персональный агрегатор контента. Собирает посты из Telegram, RSS и веб-сайтов, ранжирует через AI — я читаю только результат.

## Зачем

Вместо ручного обхода 10+ источников каждый день — одна лента с тем, что реально важно. Feedium фильтрует шум, расставляет приоритеты по интересам и показывает всё в одном месте.

## Что умеет

- Собирает контент: Telegram userbot, RSS, веб-скрапинг
- Хранит историю в PostgreSQL
- Ранжирует посты через AI (embeddings + персональные интересы)
- Отдаёт ленту через HTTP API (Connect-go / HTTP JSON)
- React UI встроен прямо в бинарник

## Что не делает

- Нет мультитенантности — проект только для одного пользователя
- Не создаёт контент, только агрегирует
- Нет OPML-импорта, мобильного приложения, публичного API

---

## Быстрый старт

**Требования:** Go 1.26+, PostgreSQL

```bash
# Установить зависимости
go mod download

# Применить миграции
DATABASE_URL=postgres://user:pass@localhost/feedium go run ./cmd/feedium run migrate

# Запустить сервер
DATABASE_URL=postgres://user:pass@localhost/feedium go run ./cmd/feedium
```

По умолчанию сервер поднимается на `http://localhost:8080`.  
Переменная `PORT` переопределяет порт.

### Проверка работоспособности

```
GET /healthz → 200 OK
```

---

## Переменные окружения

| Переменная     | Обязательная | По умолчанию | Описание                          |
|----------------|:------------:|:------------:|-----------------------------------|
| `DATABASE_URL` | да           | —            | DSN подключения к PostgreSQL      |
| `PORT`         | нет          | `8080`       | Порт HTTP-сервера                 |

---

## Команды разработки

```bash
go run ./cmd/feedium              # запустить сервис
go run ./cmd/feedium run          # то же явно
go run ./cmd/feedium run migrate  # применить миграции

go build ./...                    # проверить сборку
go test ./...                     # все тесты
go vet ./...                      # статический анализ
go generate ./...                 # перегенерировать proto и моки
golangci-lint run ./... -c .golangci.yml  # линтер
```

---

## Структура проекта

```
cmd/feedium/          — точка входа (main + CLI)
internal/
  bootstrap/          — сборка зависимостей, lifecycle
  app/
    source/           — источники контента
    post/             — посты
    summary/          — AI-суммаризация и outbox worker
  platform/
    logger/           — slog
    postgres/         — GORM
api/                  — proto-файлы и сгенерированный Connect-go код
migrations/           — SQL-миграции (goose)
```

Слои не нарушают друг друга: `app` не знает про адаптеры, адаптеры не знают про друг друга. Интерфейсы объявляются там, где используются.

---

## Стек

| Слой        | Технология                  |
|-------------|-----------------------------|
| Язык        | Go 1.26                     |
| API         | Connect-go (HTTP/JSON + gRPC ready) |
| БД          | PostgreSQL + GORM           |
| Миграции    | goose (embedded)            |
| Тесты       | testify + go.uber.org/mock  |
| UI          | React (go:embed, в разработке) |
| Логирование | slog (структурированный)    |

---

## Roadmap

**MVP** (к июню 2026)
- [ ] Коллекторы: RSS, веб, Telegram userbot
- [ ] AI-скоринг и ранжирование
- [ ] Минимальный React UI с лентой

**Далее**
- Telegram-бот OpenClaw для потребления и управления
- Тонкая настройка скоринга (лайк/дизлайк)
- Суммаризация и группировка по темам
- Поиск по истории
