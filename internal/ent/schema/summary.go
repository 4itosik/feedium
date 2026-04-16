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

type Summary struct {
	ent.Schema
}

func (Summary) Fields() []ent.Field {
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
		field.Text("text").
			NotEmpty(),
		field.Int("word_count"),
		field.Time("created_at").
			Default(time.Now).
			Immutable().
			SchemaType(map[string]string{
				dialect.Postgres: "timestamptz",
			}),
	}
}

func (Summary) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("post", Post.Type).
			Ref("summaries").
			Field("post_id").
			Unique(),
		edge.From("source", Source.Type).
			Ref("source_summaries").
			Field("source_id").
			Unique().
			Required(),
	}
}

func (Summary) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("post_id", "created_at"),
		index.Fields("source_id", "created_at", "id"),
	}
}
