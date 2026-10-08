// Package migrations embeds the sequential SQL migrations (golang-migrate naming: NNNNNN_name.up.sql).
// 000001_init.up.sql is docs/02-design/db/schema.sql (database design §9); applied files are never edited.
package migrations

import "embed"

//go:embed *.up.sql
var FS embed.FS
