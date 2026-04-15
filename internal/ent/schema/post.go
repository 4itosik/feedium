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

type Post struct {
	ent.Schema
}

func (Post) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(func() uuid.UUID {
				return uuid.Must(uuid.NewV7())
			}).
			Immutable(),
		field.UUID("source_id", uuid.UUID{}),
		field.String("external_id").
			NotEmpty(),
		field.Time("published_at").
			SchemaType(map[string]string{
				dialect.Postgres: "timestamptz",
			}),
		field.String("author").
			Optional().
			Nillable(),
		field.String("text").
			NotEmpty(),
		field.JSON("metadata", map[string]string{}).
			Default(map[string]string{}).
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

func (Post) Edges() []ent.Edge {
	return []ent.Edge{
		edge.From("source", Source.Type).
			Ref("posts").
			Field("source_id").
			Unique().
			Required(),
	}
}

func (Post) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("source_id", "published_at", "id"),
		index.Fields("source_id", "created_at", "id"),
	}
}
