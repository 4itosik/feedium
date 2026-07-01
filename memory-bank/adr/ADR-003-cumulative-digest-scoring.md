---
doc_kind: adr
doc_function: canonical
purpose: "Фиксирует решение FT-020 о том, как cumulative source-level дайджесты (summaries.post_id IS NULL) становятся самостоятельными scoreable-сущностями наравне с постами; частично supersede ADR-001 (инвариант «score принадлежит посту»). Фиксирует decision_status и последствия."
derived_from:
  - ../features/FT-020-cumulative-digest-scoring/feature.md
  - ../features/FT-019-cumulative-source-visibility/brief.md
  - ./ADR-001-scoring-method-and-vector-storage.md
  - ./ADR-002-post-embedding-persistence.md
  - ../domain/architecture.md
  - ../engineering/database.md
status: active
decision_status: accepted
date: 2026-07-01
---

# ADR-003: Cumulative digest scoring

## Контекст

FT-019 (2026-06-30) обнаружил и закрыл структурный разрыв: посты cumulative-источников
(`telegram_group`) агрегируются в один source-level `Summary` за окно (`post_id IS NULL`,
`SourceID` set), но этот summary никогда не скорится (`dueCandidatePostIDs` явно фильтрует
`post_id IS NOT NULL`) и никогда не попадает в ranked feed (`ListRanked` — `INNER JOIN posts ⋈
scores`). FT-019 сознательно выбрала **путь B**: отдельная read-only страница `/digests`,
читающая уже существующий `V1ListSourceSummaries`, без изменений scoring-инварианта FT-010
(`brief.md` §Non-Scope `NS-01`).

Операционально выяснилось, что этого недостаточно: Telegram-дайджест от OpenClaw (4-часовой
cron) читает **только** `GET /v1/feed?min_score=0.5` — тот же ranked feed, из которого digest'ы
структурно исключены. Оператор явно запросил **путь A**: cumulative-дайджесты должны получать
собственный `score` и попадать в `/v1/feed`, чтобы существующий OpenClaw-скилл подхватил их без
единого изменения на своей стороне.

Это напрямую переоткрывает инвариант `ADR-001`: `scores`/`score_events`/`post_embeddings`
спроектированы вокруг ровно одной scoreable-сущности — поста (`post_id NOT NULL`). Как и
`ADR-002` (FT-013, пересмотр `DEC-02`/`NS-05`), это архитектурно значимое расширение требует
отдельного `accepted` ADR до того, как `FT-020/feature.md` и `implementation-plan.md` считаются
finalized design (`project_ft010_adr_gate`).

Ограничения контекста: метод оценки, формат и агрегация (`ADR-001 DEC-01/03/04` — косинус,
скаляр `[0,1]`, max по интересам) не пересматриваются, только ключ, к которому относится
оценка. PostgreSQL-only, без новых инфра-зависимостей. `post_reactions` (FK → `posts(id)`) не
трогается — реакции для дайджестов вне scope этой итерации (оператор подтвердил явно).
Embedding-кэш дайджестов кэшируется в БД по аналогии с `ADR-002` (оператор подтвердил явно), а
не пересчитывается каждый раз.

## Драйверы решения

- **OpenClaw-интеграция должна остаться нулевым изменением** — единственный практичный способ
  добиться этого: чтобы `/v1/feed` сам начал отдавать дайджесты, ничего не меняя в контракте,
  который уже читает `feedium`-skill;
- **каждый часовой дайджест — самостоятельный контент**, не эволюционирующий статус источника:
  оператор явно выбрал независимое скорение и старение каждого дайджеста, симметрично постам;
  единственный «текущий скор на источник» проигрывал бы историю более старых, но всё ещё
  релевантных дайджестов;
  переиспользование существующей outbox/lease/retry-инфраструктуры (`FT-007`) и existing
  cosine/embedding-machinery (`ADR-001`) — не изобретать новый scoring-путь;
- обратимость и минимальный blast radius: существующие post-scored строки не должны измениться
  по семантике или поведению.

## Рассмотренные варианты

### Ключ для scoreable-сущности дайджеста (резолвит основной design-вопрос)

| Вариант | Плюсы | Минусы | Решение |
| --- | --- | --- | --- |
| **`summary_id` — по одному score на каждый дайджест-instance** | Симметрично `post_id`: каждый дайджест — независимая единица, ранжируется и «стареет» как пост; переиспользует существующий UNIQUE-per-entity паттерн | Три таблицы (`scores`, `score_events`, `post_embeddings`) получают доп. nullable-колонку и CHECK | **основной кандидат** — оператор выбрал явно |
| `source_id` — один «текущий» score на источник, перезаписывается каждым новым дайджестом | Меньше строк, проще | Теряет историю: старый, ещё релевантный дайджест исчезает из ленты в момент появления нового по тому же источнику | отклонён |
| Синтетический `Post`-ряд для каждого дайджеста (переиспользовать `posts`/`scores` без изменений схемы) | Ноль изменений в `scores`/`score_events` | Смешивает «сырой собранный контент» с производным агрегатом; ломает `REQ-06` (идемпотентность по `(source_id, external_id)`) и семантику `posts` как единственно сырых единиц; более инвазивно, чем добавление nullable-ключа | отклонён |

### Форма схемы (резолвит, как именно расширяются таблицы)

| Вариант | Плюсы | Минусы | Решение |
| --- | --- | --- | --- |
| **`post_id` → nullable, `+ summary_id` nullable, `CHECK (num_nonnulls(post_id, summary_id) = 1)`, два partial UNIQUE индекса вместо одного** | Один и тот же паттерн во всех трёх таблицах; существующие post-scored строки не меняются; обратимо (`DROP COLUMN` откатывает) | Каждый consumer (`ScoreRepo.Upsert`, `EnqueueDuePosts`, `resolvePostVector`, `ListRanked`) должен явно обрабатывать «какой ключ установлен» | **основной кандидат** |
| Отдельные параллельные таблицы (`digest_scores`, `digest_score_events`, `digest_embeddings`) | Не трогает существующие таблицы вообще | Дублирует lease/retry/upsert-логику вместо переиспользования; `ListRanked` всё равно должен объединять два источника | отклонён — дублирование без выигрыша, `NS-08`-подобный принцип «не плодить параллельные пайплайны» |

## Решение

Принято: `scores`, `score_events`, `post_embeddings` получают nullable `summary_id UUID
REFERENCES summaries(id) ON DELETE CASCADE`; `post_id` становится nullable; каждая таблица
получает `CHECK (num_nonnulls(post_id, summary_id) = 1)` и заменяет единственный
`UNIQUE(post_id)`-индекс на два partial-индекса (`WHERE post_id IS NOT NULL` /
`WHERE summary_id IS NOT NULL`). Каждый cumulative-дайджест (`summaries.post_id IS NULL`)
скорится независимо, тем же методом (косинус, `[0,1]`, max по интересам — `ADR-001` не
пересматривается), с тем же переиспользуемым embedding-кэшем (расширение `ADR-002`).
`ListRanked` — `UNION ALL` пост-ветки (без изменений) и новой digest-ветки. `FeedItem` (proto)
получает discriminator `kind` (`POST`/`DIGEST`), чтобы consumers могли отличать (в первую
очередь — прятать реакции для `DIGEST`, `post_reactions` не трогается). OpenClaw-скилл не
меняется — подхватывает дайджесты автоматически через уже читаемый `/v1/feed`.

Границы действия: только `FT-020` (scoring-конвейер cumulative-дайджестов). `ADR-001
DEC-01/03/04` (метод, формат, агрегация) и `ADR-002` (форма хранения вектора, инвалидация)
остаются в силе без изменений — расширяется только *какая сущность* может иметь score/вектор,
не *как* он считается. `post_reactions` и OpenClaw-репозиторий вне scope.

## Последствия

### Положительные

- OpenClaw-дайджест начинает включать контент группового чата без единого изменения на своей
  стороне — контракт `/v1/feed` расширяется аддитивно;
- полное переиспользование существующей scoring/outbox/embedding-инфраструктуры — не
  изобретается параллельный пайплайн;
- существующие post-scored строки и их поведение (значения, идемпотентность, устойчивость к
  сбою) не меняются — `CHECK`/partial-индексы обратно совместимы.

### Отрицательные

- три таблицы с live prod-данными получают schema surgery (`ALTER TABLE` + пересборка
  unique-индексов) — требует протестированной миграции перед прод-деплоем;
- каждый consumer этих таблиц (repo/usecase) должен явно различать post-ключ и summary-ключ —
  чуть выше сложность кода, чем один always-NOT-NULL `post_id`.

### Нейтральные / организационные

- `feature.md`/`implementation-plan.md` `FT-020` ссылаются на этот ADR;
- `adr/index.md` пополнен записью; `ADR-001` header помечен как частично superseded этим ADR
  (по аналогии с тем, как он уже помечен для `ADR-002`);
- реакции для дайджестов — явно открытый follow-up, не заведён как отдельная фича, пока не
  понадобится.

## Риски и mitigation

- **Ошибка миграции на прод-таблицах с живыми данными** → миграция и весь измененный код
  проходят `make test` (testcontainers) локально до деплоя; migration обратима (`DROP COLUMN
  summary_id`, `DROP CONSTRAINT`, восстановление единого `UNIQUE(post_id)`), задокументирована
  в `implementation-plan.md` `Stop Conditions`.
- **Рост стоимости embedding-провайдера от дайджестов** → пренебрежимо мал: ~9 источников ×
  ~раз в час (`cron.interval: 3600s`) на фоне текущего объёма постов на порядки выше; кэш
  (`summary_id`-ключ в `post_embeddings`) исключает повторный embed неизменившегося текста.

## Follow-up

- `FT-020/feature.md` и `implementation-plan.md` реализуют это решение;
- реакции для дайджестов (расширение `post_reactions`) — потенциальный будущий feature package,
  не заводится сейчас;
- `adr/index.md` и `ADR-001` header обновлены одним коммитом с этим ADR.

## Связанные ссылки

- [`../features/FT-020-cumulative-digest-scoring/feature.md`](../features/FT-020-cumulative-digest-scoring/feature.md)
- [`../features/FT-019-cumulative-source-visibility/brief.md`](../features/FT-019-cumulative-source-visibility/brief.md) — исходная проблема и путь B, реализованный первым
- [`ADR-001`](ADR-001-scoring-method-and-vector-storage.md) — метод/формат/агрегация оценки, не пересматриваются
- [`ADR-002`](ADR-002-post-embedding-persistence.md) — форма хранения embedding-кэша, расширяется тем же паттерном ключа
