package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/google/uuid"
)

// Interest is the instance-global topic of interest (FT-010 CTR-12). Its free-text
// topic is encoded once into a cached embedding vector (DEC-04) stored as JSONB.
type Interest struct {
	ent.Schema
}

func (Interest) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(func() uuid.UUID {
				return uuid.Must(uuid.NewV7())
			}).
			Immutable(),
		field.Text("topic").
			NotEmpty(),
		field.JSON("embedding", []float32{}).
			SchemaType(map[string]string{
				dialect.Postgres: "jsonb",
			}),
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

func (Interest) Indexes() []ent.Index {
	return []ent.Index{
		index.Fields("created_at", "id"),
	}
}
