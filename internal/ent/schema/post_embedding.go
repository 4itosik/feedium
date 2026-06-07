package schema

import (
	"time"

	"entgo.io/ent"
	"entgo.io/ent/dialect"
	"entgo.io/ent/schema/field"
	"entgo.io/ent/schema/index"

	"github.com/google/uuid"
)

// PostEmbedding is the persisted embedding vector of a post (FT-013 CTR-01).
// Exactly one row per post (UNIQUE(post_id)); reused by scoring while input_hash
// and model match the current input text and embedding model (INV-02).
type PostEmbedding struct {
	ent.Schema
}

func (PostEmbedding) Fields() []ent.Field {
	return []ent.Field{
		field.UUID("id", uuid.UUID{}).
			Default(func() uuid.UUID { return uuid.Must(uuid.NewV7()) }).
			Immutable(),
		field.UUID("post_id", uuid.UUID{}),
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
		index.Fields("post_id").Unique(),
	}
}
