---
title: "FT-013: Implementation Plan"
doc_kind: feature
doc_function: derived
purpose: "Execution-план реализации FT-013 (персистентность эмбеддинга поста и авто-переоценка при смене интересов). Фиксирует discovery context, шаги TDD, риски и test strategy без переопределения canonical feature-фактов из feature.md."
derived_from:
  - feature.md
status: active
---

# План имплементации

> **For agentic workers:** REQUIRED SUB-SKILL — используй `superpowers:subagent-driven-development` (рекоменд.) или `superpowers:executing-plans` для пошагового исполнения. Каждый `STEP-*` ниже разложен на TDD-substeps (Red → Green → Refactor → Commit). Canonical scope/verify — в sibling [`feature.md`](feature.md); план их не переопределяет.

**Goal:** сохранить эмбеддинг поста в PostgreSQL и переиспользовать его при переоценке (включая авто-переоценку при смене интересов), не вызывая платный embed для неизменившегося текста.

**Architecture:** новая таблица `post_embeddings` (1:1 к посту, `vector JSONB` + `model` + `input_hash`); `ScorePost` переиспользует сохранённый вектор по `(input_hash, model)` и пересчитывает иначе; смена набора интересов через `InterestUsecase` ставит существующие оценённые посты на переоценку через scoring-outbox; косинус только между векторами одной модели. Без pgvector — образ БД не меняется.

**Tech Stack:** Go 1.25 · go-kratos + Wire · Ent + PostgreSQL 18.3 · goose (SQL-миграции) · testify + go.uber.org/mock (gomock) + testcontainers + goleak.

---

## Цель текущего плана

Довести FT-013 до `delivery_status: done` по `feature.md §Verify`: персистентность вектора (`REQ-01`), инвалидация по тексту (`REQ-02`), согласованность модели (`REQ-03`), авто-переоценка при смене интересов (`REQ-04`), сохранность поведения FT-010 (`REQ-05`), PostgreSQL-only без расширений (`REQ-06`) и автотесты (`REQ-07`).

## Current State / Reference Points

| Path / module | Current role | Why relevant | Reuse / mirror |
| --- | --- | --- | --- |
| [`internal/biz/score.go:184`](../../../internal/biz/score.go) | `ScorePost` эмбеддит `summaries[0].Text` и отбрасывает вектор | Точка вставки reuse/invalidation/guard (`REQ-01`..`REQ-03`) | Расширить, сохранив `Relevance` чистой и upsert «одна оценка на пост» |
| [`internal/biz/score.go:38-83`](../../../internal/biz/score.go) | `Cosine`/`Relevance` — чистые функции, косинус + max | Косинус остаётся в `biz/` (`CON-01`); dimension mismatch → `ErrScoreValidation` (`NEG-01`) | Не менять сигнатуру `Relevance`; фильтровать интересы по модели до вызова |
| [`internal/biz/interest.go:84-132`](../../../internal/biz/interest.go) | `InterestUsecase.Create/Update/Delete`; embed топика через `Scorer` | Триггер авто-переоценки (`REQ-04`, `CTR-03`) | Обернуть мутацию + enqueue в транзакцию |
| [`internal/biz/score_outbox.go:42`](../../../internal/biz/score_outbox.go) | `ScoreOutboxRepo.EnqueueDuePosts` — посты без оценки | Нужен симметричный путь для уже **оценённых** постов | Добавить `EnqueueScoredPosts` |
| [`internal/data/score_outbox_repo.go:42-102`](../../../internal/data/score_outbox_repo.go) | SQL enqueue + `dueCandidatePostIDs` (партиал-уникальный индекс) | Образец для `EnqueueScoredPosts` | Скопировать insert, заменить candidate-query |
| [`internal/data/score_repo.go:36-75`](../../../internal/data/score_repo.go) | Raw-SQL upsert `scores` `ON CONFLICT (post_id)` | Образец raw-SQL upsert для `post_embeddings` | Скопировать паттерн, vector как `$N::jsonb` |
| [`internal/data/interest_repo.go:140-151`](../../../internal/data/interest_repo.go) | `ListEmbeddings` отдаёт `[][]float32` без модели | Для guard нужна модель интереса (`INV-01`) | Добавить `ListInterestVectors` с `Model` |
| [`internal/data/tx_manager.go:57`](../../../internal/data/tx_manager.go) | `txManager.InTx(ctx, fn)` — Ent и raw-SQL делят одну `sql.Tx` через контекст | Атомарность «мутация интереса + enqueue» (`OQ-02`) | Использовать порт `biz.TxManager` |
| [`internal/ent/schema/interest.go:29-34`](../../../internal/ent/schema/interest.go) | `field.JSON("embedding", []float32{})` + `field.String("model")` JSONB | Образец ent-схемы для вектора | Скопировать в `post_embedding.go` |
| [`migrations/20260602100100_create_scores_table.sql`](../../../migrations/20260602100100_create_scores_table.sql) | goose up/down, `REFERENCES posts(id) ON DELETE CASCADE`, unique index | Образец миграции | Скопировать, добавить `vector JSONB`, `input_hash` |
| [`internal/biz/wire.go:8`](../../../internal/biz/wire.go), [`internal/data/wire.go:10`](../../../internal/data/wire.go) | Wire provider sets; `NewScoreUsecase`/`NewInterestUsecase` уже в наборе; `wire.Bind(biz.TxManager)` есть | Регистрация нового repo + автопровязка новых параметров | Добавить `NewPostEmbeddingRepo` + `wire.Bind` |
| [`internal/biz/score_test.go`](../../../internal/biz/score_test.go) | gomock + AAA + goleak; pure-func tables | Образец unit-тестов | Мокать `Scorer.Embed`, считать вызовы через `.Times(n)` |
| [`internal/data/score_outbox_repo_test.go`](../../../internal/data/score_outbox_repo_test.go) | testcontainers integration | Образец интеграционного теста (`CHK-02`) | Поднять Postgres, прогнать миграции, проверить 0 `Embed` |
| [`Makefile:31-36`](../../../Makefile) | `make generate` = proto+wire; codegen mock/ent через `go generate` | Команды verify | `go generate ./...` → `make wire` → `make lint test` |

## Test Strategy

| Test surface | Canonical refs | Existing coverage | Planned automated coverage | Required local suites / commands | Required CI suites / jobs | Manual-only gap / justification | Manual-only approval ref |
| --- | --- | --- | --- | --- | --- | --- | --- |
| `biz` reuse/invalidation/guard (`ScorePost`) | `REQ-01`, `REQ-02`, `REQ-03`, `SC-01`, `SC-02`, `SC-04`, `NEG-01`, `CHK-01`, `CHK-03` | только `Cosine`/`Relevance` | unit с `MockScorer`, считающие вызовы `Embed`; model-mismatch → `ErrScoreValidation` | `go test ./internal/biz/...` | `go test ./...` | none | none |
| `data` `post_embeddings` repo | `REQ-01`, `CTR-01`, `CHK-02` | нет | integration (testcontainers): upsert/get, JSONB round-trip | `go test ./internal/data/...` | `go test ./...` | none | none |
| `data` `EnqueueScoredPosts` + interest-change trigger | `REQ-04`, `CTR-03`, `SC-03`, `CHK-02` | нет | integration: смена интереса → события переоценки, 0 `Embed` при неизменном тексте | `go test ./internal/data/... ./internal/biz/...` | `go test ./...` | none | none |
| миграция `post_embeddings` на `postgres:18.3-alpine` | `REQ-06`, `EC-06`, `CHK-04` | нет | применение goose up/down в testcontainers; отсутствие `CREATE EXTENSION` | `make lint && make test` | `go test ./...` | прод-миграция — ручной gate | `AG-01` |
| live e2e (реальный провайдер) | `MET-01`, `MET-02` | ручной e2e 2026-06-07 | повтор e2e: переоценка → 0 платных embed | `bin/feedium` + реальный OpenRouter | — | недетерминированный внешний сервис | `AG-02` |

## Open Questions / Ambiguities

| Open Question ID | Question | Why unresolved | Blocks | Default action / escalation owner |
| --- | --- | --- | --- | --- |
| `OQ-01` | Рассогласование модели интереса с текущей: пропускать stale-интересы или падать терминально? | `feature.md INV-01` требует «не считать кросс-модельный косинус» + наблюдаемость | `STEP-05` | **Resolved:** рассогласование → терминальная `ErrScoreValidation` (наблюдаемо через score-воркер; biz logger-free). Альтернатива (skip + метрика) и авто-ре-эмбеддинг интересов при смене модели — follow-up |
| `OQ-02` | Связывать ли мутацию интереса и enqueue переоценки одной транзакцией? | при сбое enqueue возможен пропуск переоценки | `STEP-07` | **Resolved:** мутация + enqueue в одной `TxManager.InTx`; сбой enqueue откатывает мутацию, ошибка идёт наверх. `Embed` — вне транзакции |
| `OQ-03` | Покрытие переоценки при >500 оценённых постов на одну смену интересов (`enqueueScanLimit = 500`) | один проход ограничен 500; полная лента может быть больше | `STEP-06`, `STEP-07` | Default: один проход на смену; добор — периодическим `EnqueueScoredPosts` из score-scheduler (как `EnqueueDuePosts`). Эскалация: operator |

## Environment Contract

| Area | Contract | Used by | Failure symptom |
| --- | --- | --- | --- |
| setup | Go toolchain; Docker для testcontainers; `go generate ./...` + `make wire` после изменения портов/схем | `STEP-01`..`STEP-09` | `wire_gen.go` не компилируется; мок-методы отсутствуют |
| test | Эталон verify — `make lint && make test` зелёные; интеграционные тесты сами поднимают Postgres (testcontainers) | `CHK-01`..`CHK-04` | падение на отсутствии Docker → verify недостоверен |
| access / network / secrets | Unit/integration — без сети и без реального провайдера (мок `Scorer`). Реальный OpenRouter — только в live e2e | `STEP-*` (none), live e2e | сетевые вызовы в unit/integration = неверный test design |

## Preconditions

| Precondition ID | Canonical ref | Required state | Used by steps | Blocks start |
| --- | --- | --- | --- | --- |
| `PRE-01` | [`ADR-002`](../../adr/ADR-002-post-embedding-persistence.md) `DEC-01` | `decision_status: accepted` (зафиксировано 2026-06-07) | все | no |
| `PRE-02` | `CON-01`, `REQ-06` | Образ `postgres:18.3-alpine`, без pgvector; Docker доступен | `STEP-01`, `STEP-08` | yes |
| `PRE-03` | `ASM-02`, `ASM-03` | Существуют `Interest.embedding` (JSONB), scoring-outbox lease/retry и `TxManager` | `STEP-05`..`STEP-07` | no |

## Workstreams

| Workstream | Implements | Result | Owner | Dependencies |
| --- | --- | --- | --- | --- |
| `WS-1` | `REQ-01`, `REQ-06`, `CTR-01` | Схема + миграция + data-repo `post_embeddings` | agent | `PRE-02` |
| `WS-2` | `REQ-01`, `REQ-02`, `REQ-03`, `INV-01`, `INV-02`, `CTR-02`, `CTR-04` | `ScorePost` reuse/invalidation + model-guard | agent | `WS-1` |
| `WS-3` | `REQ-04`, `CTR-03` | `EnqueueScoredPosts` + транзакционный триггер на смену интересов | agent | `WS-1` |
| `WS-4` | `REQ-05`, `REQ-07`, все `CHK-*` | Wiring + интеграционные тесты + verify | agent | `WS-2`, `WS-3` |

## Approval Gates

| Approval Gate ID | Trigger | Applies to | Why approval is required | Approver / evidence |
| --- | --- | --- | --- | --- |
| `AG-01` | Применение миграции `post_embeddings` к shared/prod БД | `STEP-08` (только на shared/prod) | Необратимое изменение схемы (feature-flow boundary 8) | operator; лог `goose up` |
| `AG-02` | Первый живой платный embed в реальном прогоне | live e2e | Внешний платный сервис, недетерминизм/стоимость | operator; лог e2e |

## Порядок работ

| Step ID | Actor | Implements | Goal | Touchpoints | Artifact | Verifies | Evidence IDs | Check command / procedure | Blocked by | Needs approval | Escalate if |
| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |
| `STEP-01` | agent | `CTR-01`, `REQ-06` | Ent-схема + goose-миграция `post_embeddings` | `internal/ent/schema/post_embedding.go`, `migrations/`, `internal/ent/*` (gen) | таблица + entity | `CHK-04` | `EVID-04` | `go generate ./...`; миграция применяется в testcontainers | `PRE-02` | none | миграция требует расширения БД |
| `STEP-02` | agent | `CTR-01`, `REQ-01` | biz-порт `PostEmbeddingRepo` + тип + `hashInput` | `internal/biz/score.go`, `internal/biz/mock/*` (gen) | порт + мок | `CHK-01` | `EVID-01` | `go generate ./internal/biz/...`; `go build ./...` | `STEP-01` | none | — |
| `STEP-03` | agent | `CTR-01` | data-repo `post_embeddings` (raw-SQL upsert/get) | `internal/data/post_embedding_repo.go`, `internal/data/post_embedding_repo_test.go` | repo + integration | `CHK-02` | `EVID-02` | `go test ./internal/data/ -run PostEmbedding` | `STEP-01` | none | JSONB round-trip ломается |
| `STEP-04` | agent | `INV-01`, `CTR-04` | `ListInterestVectors` (vector+model) | `internal/biz/interest.go`, `internal/data/interest_repo.go`, mocks | метод + мок | `CHK-03` | `EVID-03` | `go test ./internal/biz/... ./internal/data/...` | `STEP-01` | none | — |
| `STEP-05` | agent | `REQ-01`, `REQ-02`, `REQ-03`, `INV-02`, `CTR-02`, `CTR-04` | `ScorePost`: reuse/invalidation + model-guard | `internal/biz/score.go`, `internal/biz/score_test.go` | логика + unit | `CHK-01`, `CHK-03` | `EVID-01`, `EVID-03` | `go test ./internal/biz/ -run ScorePost` | `STEP-02`, `STEP-04` | none | — |
| `STEP-06` | agent | `REQ-04`, `CTR-03` | `EnqueueScoredPosts` (biz-порт + SQL) | `internal/biz/score_outbox.go`, `internal/data/score_outbox_repo.go`, mocks, test | метод + integration | `CHK-02` | `EVID-02` | `go test ./internal/data/ -run EnqueueScored` | `STEP-01` | none | `OQ-03` |
| `STEP-07` | agent | `REQ-04`, `CTR-03` | Транзакционный триггер переоценки в `InterestUsecase` | `internal/biz/interest.go`, `internal/biz/interest_test.go` | логика + unit | `CHK-02` | `EVID-02` | `go test ./internal/biz/ -run Interest` | `STEP-06` | none | — |
| `STEP-08` | agent | `REQ-05` | Wiring: provider + бинды + конструкторы | `internal/data/wire.go`, `internal/biz/score.go` (ctor), `cmd/feedium/wire_gen.go` (gen) | собранный граф | `CHK-04` | `EVID-04` | `make wire`; `go build ./...` | `STEP-03`, `STEP-05`, `STEP-06` | `AG-01` (prod) | wire не резолвит порт |
| `STEP-09` | agent | `REQ-05`, `REQ-07` | E2E-инвариант reuse и авто-переоценки | `internal/data/*_test.go` (integration) | integration suite | `CHK-02`, `CHK-04` | `EVID-02`, `EVID-04` | `make lint && make test` | `STEP-07`, `STEP-08` | none | значения оценок дрейфуют vs FT-010 |

### STEP-01 — Ent-схема + миграция `post_embeddings`

- [ ] **Red.** «Failing» для STEP-01 = `go build` не знает entity / репо-тест (`STEP-03`) не находит таблицу.
- [ ] **Создай** `internal/ent/schema/post_embedding.go` (зеркало `interest.go`):

```go
package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/google/uuid"
)

// PostEmbedding is the persisted embedding vector of a post (FT-013 CTR-01).
// Exactly one row per post (UNIQUE(post_id)); reused by scoring while input_hash
// and model match the current input text and embedding model (INV-02).
type PostEmbedding struct {
	ent.Schema
}

func (PostEmbedding) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).
			Immutable(),
		field.UUID("post_id", uuid.UUID{}),
		field.JSON("vector", []float32{}).
			SchemaType(map[string]string{dialect.Postgres: "jsonb"}),
		field.String("model").NotEmpty(),
		field.String("input_hash").NotEmpty(),
		field.Time("created_at").Default(time.Now).Immutable().
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
		field.Time("updated_at").Default(time.Now).UpdateDefault(time.Now).
			SchemaType(map[string]string{dialect.Postgres: "timestamptz"}),
	}
}

func (PostEmbedding) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("post_id").Unique(),
	}
}
```

- [ ] **Создай** `migrations/20260607120000_create_post_embeddings_table.sql`:

```sql
-- +goose Up
CREATE TABLE post_embeddings (
    id UUID PRIMARY KEY,
    post_id UUID NOT NULL REFERENCES posts(id) ON DELETE CASCADE,
    vector JSONB NOT NULL,
    model TEXT NOT NULL,
    input_hash TEXT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- Exactly one persisted embedding per post (CTR-01): upsert target.
CREATE UNIQUE INDEX idx_post_embeddings_post_id ON post_embeddings(post_id);

-- +goose Down
DROP TABLE IF EXISTS post_embeddings;
```

- [ ] **Green.** `go generate ./...` (регенерирует ent client с `PostEmbedding`), затем `go build ./...`.
- [ ] **Commit:** `git add internal/ent migrations && git commit -m "feat(ft-013): add post_embeddings schema and migration"`

### STEP-02 — biz-порт `PostEmbeddingRepo` + тип + `hashInput`

- [ ] **Создай** в `internal/biz/score.go` (рядом с `Score`) доменный тип, порт, sentinel и helper:

```go
// PostEmbedding is a post's persisted embedding vector, reused across scorings
// while (InputHash, Model) match the current input text and embedding model (FT-013 INV-02).
type PostEmbedding struct {
	PostID    string
	Vector    []float32
	Model     string
	InputHash string
	CreatedAt time.Time
	UpdatedAt time.Time
}

// ErrPostEmbeddingNotFound means no embedding is stored for the post yet.
var ErrPostEmbeddingNotFound = errors.New("post embedding not found")

// PostEmbeddingRepo persists exactly one embedding per post (CTR-01).
type PostEmbeddingRepo interface {
	Get(ctx context.Context, postID string) (PostEmbedding, error)
	Upsert(ctx context.Context, pe PostEmbedding) (PostEmbedding, error)
}

// hashInput is the content hash that keys vector validity (INV-02).
func hashInput(text string) string {
	sum := sha256.Sum256([]byte(text))
	return hex.EncodeToString(sum[:])
}
```

- [ ] Добавь импорты `crypto/sha256`, `encoding/hex` в `score.go`. Директива `//go:generate mockgen -source=score.go ...` уже есть — новый интерфейс попадёт в `mock/score_mock.go` автоматически.
- [ ] **Green:** `go generate ./internal/biz/...` (создаёт `MockPostEmbeddingRepo`), `go build ./...`.
- [ ] **Commit:** `git add internal/biz && git commit -m "feat(ft-013): add PostEmbeddingRepo port and input hashing"`

### STEP-03 — data-repo `post_embeddings`

- [ ] **Red.** `internal/data/post_embedding_repo_test.go` (testcontainers — зеркало setup из `score_repo_test.go`): вставить пост, затем `Upsert` вектора, `Get` → совпадение `Vector`/`Model`/`InputHash`; повторный `Upsert` с другим вектором → перезапись, одна строка.
- [ ] **Создай** `internal/data/post_embedding_repo.go`:

```go
package data

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"

	"github.com/4itosik/feedium/internal/biz"
)

type postEmbeddingRepo struct {
	data *Data
}

var _ biz.PostEmbeddingRepo = (*postEmbeddingRepo)(nil)

// NewPostEmbeddingRepo creates the persisted post-embedding repository (CTR-01).
//
//nolint:revive // unexported return type for wire DI
func NewPostEmbeddingRepo(data *Data) *postEmbeddingRepo {
	return &postEmbeddingRepo{data: data}
}

func (r *postEmbeddingRepo) Get(ctx context.Context, postID string) (biz.PostEmbedding, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)
	pid, err := uuid.Parse(postID)
	if err != nil {
		return biz.PostEmbedding{}, fmt.Errorf("invalid post id: %w", err)
	}
	const query = `
SELECT post_id, vector, model, input_hash, created_at, updated_at
FROM post_embeddings WHERE post_id = $1`
	var (
		outPostID  uuid.UUID
		vectorJSON []byte
		model      string
		inputHash  string
		createdAt  time.Time
		updatedAt  time.Time
	)
	err = ex.QueryRowContext(ctx, query, pid).
		Scan(&outPostID, &vectorJSON, &model, &inputHash, &createdAt, &updatedAt)
	if errors.Is(err, sql.ErrNoRows) {
		return biz.PostEmbedding{}, biz.ErrPostEmbeddingNotFound
	}
	if err != nil {
		return biz.PostEmbedding{}, fmt.Errorf("get post embedding: %w", err)
	}
	var vector []float32
	if uerr := json.Unmarshal(vectorJSON, &vector); uerr != nil {
		return biz.PostEmbedding{}, fmt.Errorf("decode vector: %w", uerr)
	}
	return biz.PostEmbedding{
		PostID: outPostID.String(), Vector: vector, Model: model,
		InputHash: inputHash, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}, nil
}

func (r *postEmbeddingRepo) Upsert(ctx context.Context, pe biz.PostEmbedding) (biz.PostEmbedding, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)
	pid, err := uuid.Parse(pe.PostID)
	if err != nil {
		return biz.PostEmbedding{}, fmt.Errorf("invalid post id: %w", err)
	}
	vectorJSON, err := json.Marshal(pe.Vector)
	if err != nil {
		return biz.PostEmbedding{}, fmt.Errorf("encode vector: %w", err)
	}
	const query = `
INSERT INTO post_embeddings (id, post_id, vector, model, input_hash, created_at, updated_at)
VALUES ($1, $2, $3::jsonb, $4, $5, now(), now())
ON CONFLICT (post_id) DO UPDATE
SET vector = EXCLUDED.vector, model = EXCLUDED.model,
    input_hash = EXCLUDED.input_hash, updated_at = now()
RETURNING post_id, vector, model, input_hash, created_at, updated_at`
	var (
		outPostID uuid.UUID
		outVector []byte
		model     string
		inputHash string
		createdAt time.Time
		updatedAt time.Time
	)
	err = ex.QueryRowContext(ctx, query,
		uuid.Must(uuid.NewV7()), pid, string(vectorJSON), pe.Model, pe.InputHash).
		Scan(&outPostID, &outVector, &model, &inputHash, &createdAt, &updatedAt)
	if err != nil {
		return biz.PostEmbedding{}, fmt.Errorf("upsert post embedding: %w", err)
	}
	var vector []float32
	if uerr := json.Unmarshal(outVector, &vector); uerr != nil {
		return biz.PostEmbedding{}, fmt.Errorf("decode vector: %w", uerr)
	}
	return biz.PostEmbedding{
		PostID: outPostID.String(), Vector: vector, Model: model,
		InputHash: inputHash, CreatedAt: createdAt, UpdatedAt: updatedAt,
	}, nil
}
```

- [ ] **Green:** `go test ./internal/data/ -run PostEmbedding -v` → PASS.
- [ ] **Commit:** `git add internal/data && git commit -m "feat(ft-013): add post_embeddings data repository"`

### STEP-04 — `ListInterestVectors` (vector + model)

- [ ] **В `internal/biz/interest.go`** добавь тип и метод порта (рядом с `InterestRepo`):

```go
// InterestVector is an interest embedding tagged with the model that produced it,
// so scoring never cosines vectors across models (FT-013 INV-01).
type InterestVector struct {
	Embedding []float32
	Model     string
}
```

в интерфейс `InterestRepo` добавь:

```go
	// ListInterestVectors returns every interest embedding with its model (INV-01).
	ListInterestVectors(ctx context.Context) ([]InterestVector, error)
```

- [ ] **В `internal/data/interest_repo.go`** реализуй (зеркало `ListEmbeddings`):

```go
// ListInterestVectors returns each interest embedding tagged with its model (INV-01).
func (r *interestRepo) ListInterestVectors(ctx context.Context) ([]biz.InterestVector, error) {
	entInterests, err := r.data.Ent.Interest.Query().All(ctx)
	if err != nil {
		return nil, fmt.Errorf("list interest vectors: %w", err)
	}
	vectors := make([]biz.InterestVector, 0, len(entInterests))
	for _, ei := range entInterests {
		vectors = append(vectors, biz.InterestVector{Embedding: ei.Embedding, Model: ei.Model})
	}
	return vectors, nil
}
```

- [ ] **Green:** `go generate ./internal/biz/...` (обновляет `MockInterestRepo`); `go build ./...`.
- [ ] **Commit:** `git add internal/biz internal/data && git commit -m "feat(ft-013): expose interest vectors with model"`

### STEP-05 — `ScorePost`: reuse / invalidation / model-guard

- [ ] **Red.** В `internal/biz/score_test.go` добавь таблицу `TestScorePost_VectorReuse` (мок `Scorer.Embed` со счётчиком через `gomock` `.Times(n)`):
  - «reuse»: `PostEmbeddingRepo.Get` возвращает вектор с совпавшими `(InputHash=hashInput(text), Model)` → `Embed` `Times(0)`, оценка как раньше;
  - «invalidation by text»: `Get` возвращает вектор с другим `InputHash` → `Embed` `Times(1)`, `Upsert` с новым `InputHash`;
  - «model mismatch (post)»: `Get` возвращает вектор с `Model="old"` → `Embed` `Times(1)`, `Upsert` под текущую модель;
  - «no stored vector»: `Get` → `ErrPostEmbeddingNotFound` → `Embed` `Times(1)`, `Upsert` once;
  - «interest model mismatch»: `ListInterestVectors` отдаёт интерес с `Model="old"` → `ScorePost` возвращает `ErrScoreValidation`, `Embed` `Times(0)`.
- [ ] **Измени** `ScoreUsecase` и конструктор — добавь зависимость `postEmbeddingRepo` (biz остаётся logger-free):

```go
type ScoreUsecase struct {
	scoreRepo         ScoreRepo
	interestRepo      InterestRepo
	summaryRepo       SummaryRepo
	outboxRepo        ScoreOutboxRepo
	scorer            Scorer
	postEmbeddingRepo PostEmbeddingRepo
	cfg               *conf.Scoring
}

func NewScoreUsecase(
	scoreRepo ScoreRepo, interestRepo InterestRepo, summaryRepo SummaryRepo,
	outboxRepo ScoreOutboxRepo, scorer Scorer, postEmbeddingRepo PostEmbeddingRepo,
	cfg *conf.Scoring,
) *ScoreUsecase {
	return &ScoreUsecase{
		scoreRepo: scoreRepo, interestRepo: interestRepo, summaryRepo: summaryRepo,
		outboxRepo: outboxRepo, scorer: scorer, postEmbeddingRepo: postEmbeddingRepo, cfg: cfg,
	}
}
```

- [ ] **Перепиши** тело `ScorePost` (после загрузки `summaries`) — модель-guard + reuse/invalidation:

```go
	model := uc.scorer.Model()
	interestVectors, err := uc.interestRepo.ListInterestVectors(ctx)
	if err != nil {
		return "", fmt.Errorf("list interest vectors: %w", err)
	}
	// Never cosine across embedding models (INV-01). A stale-model interest is a
	// permanent, observable failure (the worker logs the failed event via FM-08) —
	// never a silent wrong score. OQ-01 default: fail terminally; auto re-embedding
	// interests on model change is a follow-up.
	interestEmbeddings := make([][]float32, 0, len(interestVectors))
	for _, iv := range interestVectors {
		if iv.Model != model {
			return "", fmt.Errorf("%w: interest embedded with model %q, current %q",
				ErrScoreValidation, iv.Model, model)
		}
		interestEmbeddings = append(interestEmbeddings, iv.Embedding)
	}

	value := 0.0
	// No same-model interests means the lowest score; skip the paid embed (FM-09).
	if len(interestEmbeddings) > 0 {
		postEmbedding, embedErr := uc.resolvePostVector(ctx, event.PostID, summaries[0].Text, model)
		if embedErr != nil {
			return "", embedErr
		}
		value, err = Relevance(postEmbedding, interestEmbeddings)
		if err != nil {
			return "", err
		}
	}
```

- [ ] **Добавь** helper `resolvePostVector` (reuse vs recompute, INV-02/CTR-02):

```go
// resolvePostVector returns the post's embedding, reusing the persisted vector when
// (input_hash, model) still match (INV-02) and otherwise recomputing and persisting
// it. The paid Embed call happens only on the recompute path (REQ-01, REQ-02, REQ-03).
func (uc *ScoreUsecase) resolvePostVector(
	ctx context.Context, postID, text, model string,
) ([]float32, error) {
	inputHash := hashInput(text)
	stored, err := uc.postEmbeddingRepo.Get(ctx, postID)
	switch {
	case err == nil && stored.Model == model && stored.InputHash == inputHash:
		return stored.Vector, nil
	case err != nil && !errors.Is(err, ErrPostEmbeddingNotFound):
		return nil, fmt.Errorf("get post embedding: %w", err)
	}
	vector, embedErr := uc.scorer.Embed(ctx, text)
	if embedErr != nil {
		return nil, fmt.Errorf("embed summary: %w", embedErr)
	}
	if _, upErr := uc.postEmbeddingRepo.Upsert(ctx, PostEmbedding{
		PostID: postID, Vector: vector, Model: model, InputHash: inputHash,
	}); upErr != nil {
		return nil, fmt.Errorf("persist post embedding: %w", upErr)
	}
	return vector, nil
}
```

- [ ] **Green:** `go test ./internal/biz/ -run ScorePost -v` → PASS. Обнови существующие конструкторы `NewScoreUsecase` в тестах под новую сигнатуру.
- [ ] **Refactor + Commit:** `git add internal/biz && git commit -m "feat(ft-013): reuse persisted post vector with model-consistency guard"`

### STEP-06 — `EnqueueScoredPosts`

- [ ] **В `internal/biz/score_outbox.go`** добавь в интерфейс `ScoreOutboxRepo`:

```go
	// EnqueueScoredPosts inserts a pending score_event for every post that already
	// has a score and no active event — the re-scoring path on interest change
	// (FT-013 CTR-03). Idempotent via ON CONFLICT DO NOTHING. Returns the count.
	EnqueueScoredPosts(ctx context.Context) (int, error)
```

- [ ] **В `internal/data/score_outbox_repo.go`** реализуй (зеркало `EnqueueDuePosts`, другой candidate-query):

```go
// EnqueueScoredPosts enqueues a re-score event for each already-scored post without
// an active event (FT-013 CTR-03). Idempotent via the partial unique index (INV-04).
func (r *scoreOutboxRepo) EnqueueScoredPosts(ctx context.Context) (int, error) {
	ex := sqlExecerFromContext(ctx, r.data.DB)

	const candidateQuery = `
SELECT sc.post_id
FROM scores sc
WHERE NOT EXISTS (
    SELECT 1 FROM score_events se
    WHERE se.post_id = sc.post_id AND se.status IN ('pending', 'processing')
)
LIMIT $1`
	rows, err := ex.QueryContext(ctx, candidateQuery, enqueueScanLimit)
	if err != nil {
		return 0, fmt.Errorf("query scored posts: %w", err)
	}
	var postIDs []uuid.UUID
	for rows.Next() {
		var pid uuid.UUID
		if scanErr := rows.Scan(&pid); scanErr != nil {
			rows.Close()
			return 0, fmt.Errorf("scan scored post: %w", scanErr)
		}
		postIDs = append(postIDs, pid)
	}
	if rowsErr := rows.Err(); rowsErr != nil {
		rows.Close()
		return 0, fmt.Errorf("rows scored posts: %w", rowsErr)
	}
	rows.Close()

	const insertQuery = `
INSERT INTO score_events (id, post_id, status, attempt_count, created_at)
VALUES ($1, $2, 'pending', 0, now())
ON CONFLICT (post_id) WHERE status IN ('pending', 'processing') DO NOTHING`
	enqueued := 0
	for _, pid := range postIDs {
		res, insErr := ex.ExecContext(ctx, insertQuery, uuid.Must(uuid.NewV7()), pid)
		if insErr != nil {
			return enqueued, fmt.Errorf("insert rescore event: %w", insErr)
		}
		affected, affErr := res.RowsAffected()
		if affErr != nil {
			return enqueued, fmt.Errorf("rescore rows affected: %w", affErr)
		}
		enqueued += int(affected)
	}
	return enqueued, nil
}
```

- [ ] **Red→Green.** Integration-тест в `score_outbox_repo_test.go`: посеять пост+summary+score, вызвать `EnqueueScoredPosts` → ровно 1 событие; повторный вызов до завершения события → 0 (идемпотентность).
- [ ] **Green:** `go test ./internal/data/ -run EnqueueScored -v`; `go generate ./internal/biz/...` (обновит `MockScoreOutboxRepo`).
- [ ] **Commit:** `git add internal/biz internal/data && git commit -m "feat(ft-013): enqueue re-score events for already-scored posts"`

### STEP-07 — Транзакционный триггер переоценки в `InterestUsecase`

- [ ] **Измени** `InterestUsecase` — добавь зависимости `outbox ScoreOutboxRepo` и `tx TxManager`; оберни мутацию интереса и enqueue переоценки в одну транзакцию (атомарно, без biz-логгера):

```go
type InterestUsecase struct {
	repo   InterestRepo
	scorer Scorer
	outbox ScoreOutboxRepo
	tx     TxManager
}

func NewInterestUsecase(repo InterestRepo, scorer Scorer, outbox ScoreOutboxRepo, tx TxManager) *InterestUsecase {
	return &InterestUsecase{repo: repo, scorer: scorer, outbox: outbox, tx: tx}
}
```

`TxManager` — уже существующий biz-порт (`InTx(ctx, func(ctx) error) error`, реализация [`internal/data/tx_manager.go:57`](../../../internal/data/tx_manager.go)); и Ent-, и raw-SQL-репозитории подхватывают транзакцию из контекста.

- [ ] **Перепиши** `Create` (паттерн идентичен для `Update`/`Delete`): платный `Embed` — **вне** транзакции, затем мутация + enqueue атомарно:

```go
func (uc *InterestUsecase) Create(ctx context.Context, topic string) (Interest, error) {
	if err := ValidateInterestTopic(topic); err != nil {
		return Interest{}, err
	}
	embedding, err := uc.scorer.Embed(ctx, topic) // paid call OUTSIDE the tx
	if err != nil {
		return Interest{}, fmt.Errorf("embed topic: %w", err)
	}
	now := time.Now()
	interest := Interest{
		ID: uuid.Must(uuid.NewV7()).String(), Topic: topic, Embedding: embedding,
		Model: uc.scorer.Model(), CreatedAt: now, UpdatedAt: now,
	}
	var saved Interest
	err = uc.tx.InTx(ctx, func(ctx context.Context) error {
		var e error
		if saved, e = uc.repo.Save(ctx, interest); e != nil {
			return e
		}
		if _, e = uc.outbox.EnqueueScoredPosts(ctx); e != nil { // REQ-04, CTR-03
			return fmt.Errorf("enqueue rescore: %w", e)
		}
		return nil
	})
	if err != nil {
		return Interest{}, err
	}
	return saved, nil
}
```

`Update`: `Embed` вне tx, затем `InTx{ repo.Update + EnqueueScoredPosts }`. `Delete`: без embed, `InTx{ repo.Delete + EnqueueScoredPosts }`. Сбой enqueue откатывает мутацию интереса → ошибка идёт наверх (service-слой логирует); ни partial-success, ни biz-логгера (резолвит `OQ-02`).

- [ ] **Red→Green.** В `internal/biz/interest_test.go` — кейсы: `Create`/`Update`/`Delete` вызывают `outbox.EnqueueScoredPosts` `Times(1)` внутри транзакции (stub `TxManager.InTx` выполняет `fn(ctx)` инлайн, напр. `DoAndReturn(func(ctx context.Context, fn func(context.Context) error) error { return fn(ctx) })`); ошибка `EnqueueScoredPosts` откатывает и возвращается наверх (CRUD → error).
- [ ] **Green:** `go test ./internal/biz/ -run Interest -v` (директива `//go:generate` в `interest.go` уже мокает `TxManager`, если он объявлен в этом файле; иначе добавь `TxManager` в mock-source).
- [ ] **Commit:** `git add internal/biz && git commit -m "feat(ft-013): re-score existing posts on interest change"`

### STEP-08 — Wiring

- [ ] **В `internal/data/wire.go`** добавь провайдер и бинд:

```go
	NewPostEmbeddingRepo,
	// ...
	wire.Bind(new(biz.PostEmbeddingRepo), new(*postEmbeddingRepo)),
```

- [ ] `NewScoreUsecase` и `NewInterestUsecase` уже в `biz.ProviderSet`; Wire подставит новые параметры (`PostEmbeddingRepo` для `ScoreUsecase`; `ScoreOutboxRepo` + `TxManager` для `InterestUsecase`) — все уже в `data.ProviderSet` (`NewScoreOutboxRepo`, `wire.Bind(biz.TxManager)`). Biz остаётся logger-free.
- [ ] **Green:** `make wire` → `wire_gen.go` пересобран без ошибок; `go build ./...`.
- [ ] **Commit:** `git add internal/data internal/biz cmd/feedium && git commit -m "chore(ft-013): wire post-embedding repo and re-score trigger"`

### STEP-09 — E2E-инвариант reuse и авто-переоценки

- [ ] **Integration (testcontainers)** в `internal/data/`: посеять интерес + пост + summary; первичный скоринг → создан вектор и оценка; повторный `ScorePost` тем же текстом → `MockScorer.Embed` НЕ вызван (или счётчик провайдера = 0), оценка та же (`CTR-05`); смена интереса → `EnqueueScoredPosts` дал событие → переоценка не зовёт `Embed` при неизменном тексте; ровно одна актуальная оценка на пост (`INV-04`).
- [ ] **Verify полный:** `make lint && make test` → зелёные; сложить логи в `artifacts/ft-013/verify/chk-0{1..4}/`.
- [ ] **Commit:** `git add -A && git commit -m "test(ft-013): e2e vector reuse and interest-change re-scoring"`

## Parallelizable Work

- `PAR-01` `STEP-03` (data-repo) и `STEP-04` (interest vectors) можно вести параллельно после `STEP-01` — разные файлы.
- `PAR-02` `STEP-05` (ScorePost) и `STEP-06` (EnqueueScoredPosts) затрагивают разные методы, но оба меняют `internal/biz` мок-файлы (codegen) — не запускать `go generate` параллельно (общий write-surface моков).

## Checkpoints

| Checkpoint ID | Refs | Condition | Evidence IDs |
| --- | --- | --- | --- |
| `CP-01` | `STEP-01`, `STEP-03`, `CHK-02` | Миграция применяется, JSONB round-trip вектора работает | `EVID-02`, `EVID-04` |
| `CP-02` | `STEP-05`, `CHK-01`, `CHK-03` | Reuse не зовёт `Embed`; смена текста/модели → пересчёт; кросс-модельный косинус не считается | `EVID-01`, `EVID-03` |
| `CP-03` | `STEP-07`, `STEP-09`, `CHK-02` | Смена интереса переоценивает существующие посты без `Embed`, одна оценка на пост | `EVID-02` |
| `CP-04` | `STEP-09`, `CHK-04` | `make lint && make test` зелёные; образ БД неизменен | `EVID-04` |

## Execution Risks

| Risk ID | Risk | Impact | Mitigation | Trigger |
| --- | --- | --- | --- | --- |
| `ER-01` | `Embed` ошибочно вызван внутри `InTx` (сетевой вызов держит DB-транзакцию) | Долгая транзакция, локи, рост латентности | Эмбеддить ДО `InTx` (см. STEP-07); ревью | code review STEP-07 |
| `ER-02` | `json.Marshal([]float32)` + `$N::jsonb` даёт неверный тип в драйвере | repo-тест падает на round-trip | Передавать `string(jsonBytes)` с явным `::jsonb`; покрыть `STEP-03` | scan `vector` → ошибка декода |
| `ER-03` | Смена сигнатур `NewScoreUsecase`/`NewInterestUsecase` ломает существующие тесты | Красные старые тесты | Обновить все конструкторы в тестах в том же шаге | компиляция тестов падает |
| `ER-04` | Переоценка >500 постов не покрыта одним проходом (`OQ-03`) | Часть ленты не переоценена сразу | Добор периодическим `EnqueueScoredPosts` из scheduler; зафиксировать `OQ-03` | лента > 500 оценённых постов |

## Stop Conditions / Fallback

| Stop ID | Related refs | Trigger | Immediate action | Safe fallback state |
| --- | --- | --- | --- | --- |
| `STOP-01` | `CTR-05`, `REQ-05` | Значения оценок через reuse расходятся с пересчётом «с нуля» | Остановить, диффнуть reuse vs recompute | Откатить `STEP-05`, оставить транзиентный путь FT-010 |
| `STOP-02` | `AG-01` | Требуется прод-миграция без подтверждения | Остановиться, запросить `AG-01` | Миграция только в testcontainers/локально |
| `STOP-03` | `INV-01`, `OQ-01` | Обнаружен кросс-модельный косинус в проде | Остановить скоринг затронутых постов | Пересчёт под текущую модель / ре-эмбеддинг интересов |

## Готово для приемки

План исчерпан, когда все `STEP-01..09` закрыты, `CP-01..04` доказаны, `make lint && make test` зелёные локально и в CI, evidence `EVID-01..04` сложены по путям из `feature.md §Evidence contract`, а `simplify review` пройден. После этого — приёмка по `feature.md §Verify` (`SC-01..05`, `NEG-01..02`, `CHK-01..04`) и перевод `feature.md → delivery_status: done`, `implementation-plan.md → status: archived`.
