package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/edge"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/google/uuid"
)

type SummaryEvent struct {
	ent.Schema
}

func (SummaryEvent) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(func() uuid.UUID {
				return uuid.Must(uuid.NewV7())
			}).
			Immutable(),
		field.UUID("post_id", uuid.UUID{}).
			Optional().
			Nillable(),
		field.UUID("source_id", uuid.UUID{}),
		field.String("event_type").
			NotEmpty(),
		field.String("status").
			NotEmpty().
			Default("pending"),
		field.UUID("summary_id", uuid.UUID{}).
			Optional().
			Nillable(),
		field.String("error").
			Optional().
			Nillable(),
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
		field.Time("locked_until").
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "timestamptz",
			}),
		field.String("locked_by").
			Optional().
			Nillable(),
		field.Int("attempt_count").
			Default(0),
		field.Time("next_attempt_at").
			Optional().
			Nillable().
			SchemaType(map[string]string{
				dialect.Postgres: "timestamptz",
			}),
	}
}

func (SummaryEvent) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("summary", Summary.Type).
			Field("summary_id").
			Unique(),
	}
}

func (SummaryEvent) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("status", "created_at"),
		index.Fields("source_id", "event_type", "status"),
	}
}
