package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/google/uuid"
)

// Score is the single current relevance score of a post (FT-010 CTR-10). Exactly
// one row per post is enforced by a UNIQUE(post_id) index (INV-02); ranking reads
// the precomputed scalar value ordered descending.
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
		field.UUID("post_id", uuid.UUID{}),
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
		index.Fields("post_id").Unique(),
		index.Fields("value"),
	}
}
