package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/google/uuid"
)

// ScoreEvent is the scoring outbox row (FT-010 CTR-11), a structural copy of
// SummaryEvent. The score_due_scheduler enqueues one per summarized-unscored post;
// the score worker pool consumes it under a lease. At most one active event per
// post is enforced by a partial UNIQUE(post_id) index in the migration (INV-04).
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
		field.UUID("post_id", uuid.UUID{}),
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
	}
}
