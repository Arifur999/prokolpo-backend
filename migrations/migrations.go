// Package migrations embeds the SQL migrations, so the binary carries its own schema.
package migrations

import "embed"

// FS holds every goose migration in this directory.
//
//go:embed *.sql
var FS embed.FS
