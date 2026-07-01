package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/dialect/entsql"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/google/uuid"
)

// PostEmbedding is the persisted embedding vector of a scoreable entity (FT-013
// CTR-01, FT-020 CTR-01). It targets either a post (post_id) or a cumulative digest
// (summary_id) — exactly one is set. Exactly one row per entity (two partial UNIQUE
// indexes); reused by scoring while input_hash and model match (INV-02, ADR-002).
type PostEmbedding struct {
	ent.Schema
}

func (PostEmbedding) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).
			Immutable(),
		field.UUID("post_id", uuid.UUID{}).
			Optional().
			Nillable(),
		field.UUID("summary_id", uuid.UUID{}).
			Optional().
			Nillable(),
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
		index.Fields("post_id").Unique().
			Annotations(entsql.IndexWhere("post_id IS NOT NULL")),
		index.Fields("summary_id").Unique().
			Annotations(entsql.IndexWhere("summary_id IS NOT NULL")),
	}
}
