// Package migrations embeds the SQL migration files so the binary can apply
// them at startup via goose, with no external files needed at runtime (GPC4:
// the schema ships with the code).
package migrations

import "embed"

// FS holds all *.sql migrations in this directory.
//
//go:embed *.sql
var FS embed.FS
