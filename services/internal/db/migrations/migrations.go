// Package migrations embeds the SQL migration files into the binary.
//
// Files are applied in lexical order, so name them NNNN_description.sql.
// Never edit a migration that has been applied; add a new one instead.
package migrations

import "embed"

// FS holds every *.sql file in this directory.
//
//go:embed *.sql
var FS embed.FS
