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

// Score is the single current relevance score of a scoreable entity. It targets
// either a post (post_id) or a cumulative digest (summary_id) — exactly one is set
// (FT-020 CTR-01). Exactly one row per entity is enforced by two partial UNIQUE
// indexes (INV-02, symmetric); ranking reads the precomputed scalar value descending.
// DDL authority is goose migrations; this schema mirrors them for the generated model.
type Score struct {
	ent.Schema
}

func (Score) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(func() uuid.UUID {
				return uuid.Must(uuid.NewV7())
			}).
			Immutable(),
		field.UUID("post_id", uuid.UUID{}).
			Optional().
			Nillable(),
		field.UUID("summary_id", uuid.UUID{}).
			Optional().
			Nillable(),
		field.Float("value"),
		field.String("model").
			NotEmpty(),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			SchemaType(map[string]string{
				dialect.Postgres: "timestamptz",
			}),
		field.Time("updated_at").
			Default(time.Now).
			UpdateDefault(time.Now).
			SchemaType(map[string]string{
				dialect.Postgres: "timestamptz",
			}),
	}
}

func (Score) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("post_id").Unique().
			Annotations(entsql.IndexWhere("post_id IS NOT NULL")),
		index.Fields("summary_id").Unique().
			Annotations(entsql.IndexWhere("summary_id IS NOT NULL")),
		index.Fields("value"),
	}
}
