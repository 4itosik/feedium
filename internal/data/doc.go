// Package data provides data layer implementations including database connections and repositories.
//
// See: memory-bank/engineering/database.md
//
// Ent client will be created on top of Data.db via entsql.OpenDB(dialect.Postgres, Data.db)
// in the feature that introduces the first entity; the physical pool remains the same.
package data
