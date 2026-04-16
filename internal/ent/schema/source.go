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

type Source struct {
	ent.Schema
}

func (Source) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(func() uuid.UUID {
				return uuid.Must(uuid.NewV7())
			}).
			Immutable(),
		field.String("type").
			NotEmpty(),
		field.JSON("config", map[string]any{}).
			SchemaType(map[string]string{
				dialect.Postgres: "jsonb",
			}),
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

func (Source) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("type", "created_at", "id"),
	}
}

func (Source) Edges() []ent.Edge {
	return []ent.Edge{
		edge.To("posts", Post.Type),
		edge.To("source_summaries", Summary.Type),
		edge.To("summary_events", SummaryEvent.Type),
	}
}
