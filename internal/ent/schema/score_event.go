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

// ScoreEvent is the scoring outbox row (FT-010 CTR-11), a structural copy of
// SummaryEvent. It targets either a post (post_id) or a cumulative digest
// (summary_id) — exactly one is set (FT-020 CTR-04). The scheduler enqueues one per
// unscored entity; the worker pool consumes it under a lease. At most one active
// event per entity is enforced by two partial UNIQUE indexes in the migration (INV-04).
type ScoreEvent struct {
	ent.Schema
}

func (ScoreEvent) Fields() []ent.Field {
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
		field.String("status").
			NotEmpty().
			Default("pending"),
		field.String("error").
			Optional().
			Nillable(),
		field.Int("attempt_count").
			Default(0),
		field.String("locked_by").
			Optional().
			Nillable(),
		field.Time("locked_until").
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "timestamptz",
			}),
		field.Time("next_attempt_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "timestamptz",
			}),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			SchemaType(map[string]string{
				dialect.Postgres: "timestamptz",
			}),
		field.Time("processed_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "timestamptz",
			}),
	}
}

func (ScoreEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at"),
		index.Fields("post_id").Unique().
			Annotations(entsql.IndexWhere("post_id IS NOT NULL AND status IN ('pending', 'processing')")),
		index.Fields("summary_id").Unique().
			Annotations(entsql.IndexWhere("summary_id IS NOT NULL AND status IN ('pending', 'processing')")),
	}
}
