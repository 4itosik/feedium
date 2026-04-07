package migrations

import "embed"

// Files contains SQL migration files in goose format.
//
//go:embed *.sql
var Files embed.FS
