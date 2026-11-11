// Package migrations embeds the sequential SQL migrations (golang-migrate naming: NNNNNN_name.up.sql).
// 000001_init.up.sql is the baseline of docs/02-design/db/schema.sql (database design §9) and is frozen: a change of the
// schema after it is a new numbered migration that alters (IR785), and docs/02-design/db/schema.sql stays the schema
// the migrations add up to (TestMigrationsMatchSchema). An applied file is never edited, and no database is rebuilt for it.
package migrations

import "embed"

// FS holds the migration files.
//
//go:embed *.up.sql
var FS embed.FS
