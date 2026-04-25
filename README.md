# Feedium

Персональный агрегатор контента: Telegram / RSS / веб-сайты → AI-ранжирование → React UI и Telegram-бот OpenClaw.

Документация проекта: [`memory-bank/index.md`](memory-bank/index.md).

## Требования

- Go 1.26+
- PostgreSQL 18+ (или Docker)
- [goose](https://github.com/pressly/goose) для миграций: `go install github.com/pressly/goose/v3/cmd/goose@latest`
- `protoc` + `make proto` — только при изменении `.proto` файлов

## Быстрый локальный запуск

### 1. PostgreSQL через docker compose

```bash
docker compose up -d postgres
```

Параметры подключения и healthcheck — в [`docker-compose.yml`](docker-compose.yml). Данные хранятся в volume `feedium-pg-data`.

Альтернатива без Docker — создать руками:

```sql
CREATE USER feedium WITH PASSWORD 'feedium';
CREATE DATABASE feedium OWNER feedium;
```

### 2. Миграции

```bash
goose -dir migrations postgres \
  "postgres://feedium:feedium@127.0.0.1:5432/feedium?sslmode=disable" up
```

> В `Makefile` есть цель `make migrate` — проверь строку подключения перед использованием.

### 3. Локальный конфиг

Папка `configs/local/` в `.gitignore` — это твой приватный профиль. Скопируй шаблон и впиши ключ LLM:

```bash
mkdir -p configs/local
cp configs/local.example.yaml configs/local/config.yaml
```

```yaml
# configs/local/config.yaml
summary:
  llm:
    providers:
      openrouter:
        api_key: "sk-or-..."
```

> ⚠️ **Duration-поля** (`lease_ttl`, `cron.interval`, `max_window` и т.п.) обязаны быть в каноничном protojson-формате — только секунды с суффиксом `s`: `3600s`, а не `1h`. Go-стиль `5m`/`72h` не парсится.

### 4. Сборка и запуск

```bash
make build
./bin/feedium -conf configs/local/
```

HTTP: `0.0.0.0:8000`, gRPC: `0.0.0.0:9000`.

### 5. Smoke-тест API

```bash
# health
curl -s http://127.0.0.1:8000/healthz | jq .

# создать RSS-источник
curl -s -X POST http://127.0.0.1:8000/v1/sources \
  -H 'Content-Type: application/json' \
  -d '{"type":"SOURCE_TYPE_RSS","config":{"rss":{"feedUrl":"https://hnrss.org/frontpage"}}}' | jq .

# список источников
curl -s http://127.0.0.1:8000/v1/sources | jq .

# получить источник по id
curl -s http://127.0.0.1:8000/v1/sources/019dc0d3-aaa8-72d6-a33f-c7a947f3d4bd | jq .

# список постов
curl -s 'http://127.0.0.1:8000/v1/posts?pageSize=10' | jq .

# инициировать cumulative-суммаризацию источника
curl -s -X POST http://127.0.0.1:8000/v1/sources/019dc0d3-aaa8-72d6-a33f-c7a947f3d4bd/summarize \
  -H 'Content-Type: application/json' -d '{}' | jq .
```

Точные поля запросов — в [`api/feedium/*.proto`](api/feedium/).

### 6. End-to-end: Telegram-канал → пост → саммари

Полный сценарий «создать источник-канал → положить в него пост → получить итоговое саммари».

```bash
# 1. Создать источник типа Telegram-канал
SOURCE_ID=$(curl -s -X POST http://127.0.0.1:8000/v1/sources \
  -H 'Content-Type: application/json' \
  -d '{
        "type":"SOURCE_TYPE_TELEGRAM_CHANNEL",
        "config":{"telegramChannel":{"tgId":"-1001234567890","username":"durov"}}
      }' | jq -r '.source.id')
echo "source: $SOURCE_ID"

# 2. Отправить пост в этот канал (создаём запись поста для источника).
#    Тело собираем через jq — он сам экранирует переводы строк, кавычки и юникод
#    в произвольном тексте (heredoc безопаснее, чем подстановка в -d "...").
TEXT=$(cat <<'EOF'
Длинный пост канала, который нужно суммаризовать.
Может содержать переводы строк, "кавычки", юникод, ссылки и т.п.
EOF
)

POST_ID=$(jq -n \
    --arg sid    "$SOURCE_ID" \
    --arg ext    "tg-msg-1" \
    --arg pub    "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
    --arg author "durov" \
    --arg text   "$TEXT" \
    '{sourceId:$sid, externalId:$ext, publishedAt:$pub, author:$author, text:$text}' \
  | curl -s -X POST http://127.0.0.1:8000/v1/posts \
      -H 'Content-Type: application/json' -d @- \
  | jq -r '.post.id')
echo "post: $POST_ID"

# Альтернатива — текст из файла через --rawfile (без heredoc):
#   jq -n --arg sid "$SOURCE_ID" --arg ext "tg-msg-1" \
#         --arg pub "$(date -u +%Y-%m-%dT%H:%M:%SZ)" \
#         --arg author "durov" --rawfile text ./post.txt \
#      '{sourceId:$sid, externalId:$ext, publishedAt:$pub, author:$author, text:$text}' \
#     | curl -s -X POST http://127.0.0.1:8000/v1/posts \
#         -H 'Content-Type: application/json' -d @-

# 3a. Self-contained режим: саммари по конкретному посту создаётся автоматически —
#     просто получаем результат, когда событие обработано:
curl -s "http://127.0.0.1:8000/v1/posts/019dc3c4-ae9d-7641-82fd-a18dcf525372/summaries" | jq .

# 3b. Cumulative режим: запустить агрегатную суммаризацию по источнику и
#     прочитать итоговое саммари по источнику:
TASK_ID=$(curl -s -X POST "http://127.0.0.1:8000/v1/sources/$SOURCE_ID/summarize" \
  -H 'Content-Type: application/json' -d '{}' | jq -r '.taskId')
echo "event: $TASK_ID"

# опросить статус события (PENDING → PROCESSING → COMPLETED)
curl -s "http://127.0.0.1:8000/v1/summary-events/$TASK_ID" | jq '.event.status'

# получить итоговое саммари по источнику
curl -s "http://127.0.0.1:8000/v1/sources/$SOURCE_ID/summaries" | jq '.summaries[0].text'
```

> Режим обработки (`SELF_CONTAINED` / `CUMULATIVE`) задаётся для источника в конфиге; см. [`domain/glossary.md`](memory-bank/domain/glossary.md) и [`features/FT-005-ai-summarization`](memory-bank/features/FT-005-ai-summarization/brief.md).

## Полезное

```bash
make test                    # юнит-тесты
make test-coverage           # тесты с покрытием
make coverage                # HTML-отчёт покрытия
make lint                    # golangci-lint
make generate                # proto + wire
make feediumctl              # CLI-утилита bin/feediumctl

docker compose logs -f postgres   # логи БД
docker compose down               # остановить БД (данные сохранятся)
docker compose down -v            # остановить и снести volume
```

## Структура

- `cmd/feedium` — входная точка сервиса.
- `cmd/feediumctl` — CLI-утилита.
- `internal/biz` — доменная логика (без инфраструктуры).
- `internal/data` — репозитории на Ent.
- `internal/service` — API-слой из proto.
- `internal/task` — коллекторы и воркеры (реализуют `transport.Server`).
- `configs/` — конфигурация: `config.yaml` — дефолт, `local.example.yaml` — шаблон локального профиля, `local/` — локальный профиль (gitignored).
- `migrations/` — SQL-миграции goose.
- `api/feedium/` — proto-файлы API.
