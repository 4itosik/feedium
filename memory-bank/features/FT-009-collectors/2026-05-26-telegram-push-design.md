---
title: "FT-009: Telegram Push Collector — Design"
doc_kind: feature
doc_function: derived
purpose: "Design-решение по сбору Telegram: переход с polling на push (gotd updates.Manager). Фиксирует девиацию от polling-модели feature.md/spec.md для типов telegram_channel/telegram_group и её обоснование. RSS/web остаются на polling без изменений."
derived_from:
  - feature.md
  - spec.md
  - implementation-plan.md
status: active
supersedes_for_telegram:
  - "spec.md §Solution (polling-цикл) — только для TG-типов"
  - "implementation-plan.md STEP-04/05 (Fetcher/high-water polling) — переписываются"
---

# FT-009: Telegram Push Collector — Design

## Решение (DEC-08)

Telegram-материал собирается **push-моделью**: один долгоживущий userbot-коннект
(`gotd/td`) с `updates.Manager` получает новые сообщения каналов/групп в реальном
времени и прогоняет каждое через **существующий** biz-пайплайн сбора. Polling
(`poll_interval`, порт `Fetcher`, in-memory high-water mark из DEC-05) для Telegram
**не используется**.

RSS (`REQ-01`) и web (`REQ-04`) остаются на polling-модели (`Fetcher` +
`runCollectLoop`) — без изменений (`NS-08`, `INV-T06`).

### Почему push, а не polling

| Критерий | Push (выбрано) | Polling (feature.md/spec.md) |
|---|---|---|
| Латентность | реальное время | до `poll_interval` |
| Семантика TG | нативная — Telegram сам отдаёт новые сообщения | эмуляция через периодическое чтение «головы» |
| FLOOD_WAIT | минимум вызовов API (нет периодического `getHistory`) | растёт с числом источников × частота |

Пользователь явно выбрал push 2026-05-26 (ожидание: «получать новые сообщения от tg»,
а не опрашивать). Решение зафиксировано до реализации.

## Девиации от canonical

| Canonical факт | Статус для Telegram | Примечание |
|---|---|---|
| `REQ-08` per-type poll interval | не применяется к TG | `poll_interval` удаляется из `conf.TelegramCollector` |
| `DEC-05` in-memory high-water mark | заменён | push-модель: in-memory updates-state, без курсора и без backfill (см. Durability) |
| Порт `biz.Fetcher` для TG | не используется | TG не реализует `Fetcher`; персист идёт через новый biz-seam `CollectItems` |
| spec.md §Solution «polling-цикл» | только RSS/web | TG-коллектор не крутит `runCollectLoop` |

RSS/web сохраняют все перечисленные факты без изменений.

## Архитектура

### Компоненты

1. **`task.TelegramCollector`** (`transport.Server`, `internal/task/telegram_collector.go`)
   - `Start(ctx)`: поднимает gotd-клиент по `session_path`, регистрирует
     `tg.UpdateDispatcher` на `UpdateNewChannelMessage` (каналы/супергруппы) и
     `UpdateNewMessage` (обычные группы), запускает `updates.Manager` в фоновой
     горутине (`wg.Go`). **Живой коннект/авторизация — за `AG-02`** (внешний аккаунт,
     `RISK-03`); в тестах подменяется fake-стримом без сессии.
   - `Stop(ctx)`: останавливает менеджер, закрывает коннект, `close(done)` один раз,
     `wg.Wait()` — graceful shutdown как у прочих `transport.Server`.

2. **Обработчик сообщения** (чистая, тестируемая без сессии функция)
   - Вход: `tg.Message` + `tg.Entities` (users/chats из update-контейнера).
   - Резолвит peer → `TgID` (для `PeerChannel` — channel id, для `PeerChat` — chat id).
   - Находит сконфигурированный источник по `(SourceType, TgID)` через резолвер (см. ниже).
     Нет совпадения → сообщение игнорируется (userbot может состоять в чужих диалогах).
   - Строит `biz.CollectedItem`: `ExternalID = strconv(msg.ID)`, `PublishedAt = msg.Date`,
     `Author` (из entities, опционально), `Text = msg.Message`, `Metadata["chat_title"]`.

3. **Резолвер источника** (`SourceLister`-seam поверх `biz.SourceUsecase.List`)
   - In-memory индекс `TgID → biz.Source` для типов `telegram_channel`/`telegram_group`.
   - Refresh on cache-miss + короткий TTL → новые/удалённые источники подхватываются
     без рестарта процесса.

4. **Персист — переиспользование biz** (`internal/biz/collect.go`)
   - Из `CollectUsecase.CollectSource` выделяется экспортируемый метод
     `CollectItems(ctx, source, []CollectedItem) error`: `FilterTextItems` → `Create` →
     skip `ErrPostSourceNotFound`. `CollectSource` становится `Fetch` + `CollectItems`
     (поведение RSS/web не меняется — рефактор без смены контракта).
   - Push-путь зовёт `CollectItems` напрямую. Бесплатно сохраняются:
     `REQ-06` (idempotency по `(source_id, external_id)`),
     `REQ-09` (channel → `summarize_post`; group → cumulative без события),
     `REQ-10` (text-only).

### Поток данных

```
gotd updates.Manager
  └─ UpdateNewChannelMessage / UpdateNewMessage  (+ tg.Entities)
       └─ handleMessage(msg, entities)
            ├─ peer → TgID
            ├─ resolver: TgID → biz.Source  (miss → ignore)
            ├─ build biz.CollectedItem
            └─ CollectUsecase.CollectItems(ctx, source, []CollectedItem{item})
                 └─ FilterTextItems → PostUsecase.Create → (outbox per ProcessingMode)
```

### Durability (рестарт / оффлайн)

In-memory updates-state, **без backfill** (утверждено 2026-05-26, согласуется с `NS-04`).
После рестарта ловятся только сообщения с момента переподключения; пропущенные за
оффлайн — теряются. Схема БД и миграции не меняются (`PRE-03`, `INV-T06`). Персистентный
`pts/qts/seq`-store — явно вне scope; при необходимости поднимается отдельной фичей.

### Seam / тестируемость

Весь gotd-специфичный код (клиент, auth, `updates.Manager`, перевод `tg.Message` →
`biz.CollectedItem`) изолирован за узким интерфейсом стрима сообщений. Логика
коллектора (резолв источника, диспетч персиста, lifecycle) и сам персист тестируются
синтетическими сообщениями без живой сессии.

## Config

`conf.TelegramCollector` (proto): убрать `poll_interval`; оставить
`api_id`/`api_hash`/`phone`/`session_path`. Секреты — только в некоммитимом
`config.yaml`/env; `local.example.yaml` — плейсхолдеры (`OQ-03`, `CON-04`).

## Тесты

| Surface | Покрытие | Команда |
|---|---|---|
| `handleMessage` маппинг + резолв + игнор неизвестного peer | unit, fake source-lister | `go test ./internal/task/... -run TestTelegramCollector -race` |
| Персист по режимам (`REQ-09`): channel → `summarize_post`; group → cumulative без события | integration, testcontainers + реальный `CollectUsecase` | то же (Docker) |
| `CollectItems` рефактор не ломает RSS/web | существующие `collect_test.go`/RSS/web | `make test` |
| lifecycle goroutine leak | `goleak.VerifyNone` | в каждом task-тесте |
| Живой gotd-коннект/авторизация | manual-only, `AG-02` | человек |

## Риски

| Risk | Митигейшн |
|---|---|
| gotd-типы протекают в `biz`/`task` | gotd живёт только за seam; наружу — `biz.CollectedItem`/`tgMessage`. |
| FLOOD_WAIT при резолве entity | floodwait-middleware gotd; резолвер кэширует. |
| Оффлайн-пропуски | принято осознанно (in-memory, без backfill). |
| Живой вход в Telegram в тесте | запрещён; `AG-02`, fake-стрим в тестах. |
