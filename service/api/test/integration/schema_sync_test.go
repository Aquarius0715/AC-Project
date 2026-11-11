package integration_test

import (
	"context"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/migrate"
	"github.com/pradita/ac-project/service/api/internal/migrations"
)

// TestMigrationsMatchSchema (IR785): the numbered migrations add up to docs/02-design/db/schema.sql. A schema change
// is a new migration with ALTERs (000001 is frozen, and no database is rebuilt for a change), and the same change in
// schema.sql; this test builds a database from each and compares their tables, columns, constraints and indexes.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestMigrationsMatchSchema(t *testing.T) {
	ctx := context.Background()

	admin, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = admin.Close(ctx) }()

	base := os.Getenv("AC_TEST_DB")
	if base == "" {
		base = "ac_test"
	}

	fromMigrations, fromSchema := base+"_mig", base+"_doc"
	for _, name := range []string{fromMigrations, fromSchema} {
		_, err := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
		if err != nil {
			t.Fatal(err)
		}

		_, err = admin.Exec(ctx, "CREATE DATABASE "+name)
		if err != nil {
			t.Fatal(err)
		}

		t.Cleanup(func() { _, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)") })
	}

	migrated := catalogOf(ctx, t, fromMigrations, func(conn *pgx.Conn) error {
		available, err := migrate.Load(migrations.FS)
		if err != nil {
			return fmt.Errorf("load: %w", err)
		}

		_, err = migrate.Up(ctx, conn, available)

		return err //nolint:wrapcheck // a test helper: the caller names the database
	})

	schema, err := os.ReadFile("../../../../docs/02-design/db/schema.sql")
	if err != nil {
		t.Fatal(err)
	}

	documented := catalogOf(ctx, t, fromSchema, func(conn *pgx.Conn) error {
		_, err := conn.Exec(ctx, string(schema))

		return err //nolint:wrapcheck // a test helper: the caller names the database
	})

	for _, kind := range []string{"column", "constraint", "index"} {
		only := difference(migrated[kind], documented[kind])
		other := difference(documented[kind], migrated[kind])

		if len(only) > 0 || len(other) > 0 {
			t.Errorf("%ss differ: only in the migrations %q; only in schema.sql %q", kind, only, other)
		}
	}
}

// catalogOf builds a database with setup and lists its columns, constraints (without their names) and indexes.
func catalogOf(
	ctx context.Context,
	t *testing.T,
	name string,
	setup func(*pgx.Conn) error,
) map[string][]string {
	t.Helper()

	conn, err := pgx.Connect(ctx, strings.Replace(testDB("postgres"), "/"+databaseOf(testDB("postgres"))+"?", "/"+name+"?", 1))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	err = setup(conn)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}

	out := map[string][]string{}

	for kind, query := range map[string]string{
		"column": `SELECT table_schema || '.' || table_name || '.' || column_name || ' ' || data_type || ' ' || udt_name || ' ' ||
				is_nullable || ' ' || coalesce(column_default, '')
			FROM information_schema.columns WHERE table_schema NOT IN ('pg_catalog', 'information_schema') AND table_name <>
				'schema_migrations'`,
		"constraint": `SELECT conrelid::regclass::text || ' ' || contype::text || ' ' || pg_get_constraintdef(oid) FROM pg_constraint
			WHERE connamespace NOT IN ('pg_catalog'::regnamespace, 'information_schema'::regnamespace) AND
			conrelid::regclass::text <> 'schema_migrations'`,
		"index": `SELECT schemaname || '.' || indexdef FROM pg_indexes WHERE schemaname NOT IN ('pg_catalog',
			'information_schema') AND tablename <> 'schema_migrations'`,
	} {
		rows, err := conn.Query(ctx, query)
		if err != nil {
			t.Fatal(err)
		}

		list, err := pgx.CollectRows(rows, pgx.RowTo[string])
		if err != nil {
			t.Fatal(err)
		}

		out[kind] = list
	}

	return out
}

func databaseOf(url string) string {
	rest := url[strings.LastIndex(url, "/")+1:]

	return rest[:strings.Index(rest, "?")]
}

// difference lists what is in a and not in b.
func difference(a, b []string) []string {
	have := map[string]bool{}
	for _, item := range b {
		have[item] = true
	}

	var out []string

	for _, item := range a {
		if !have[item] {
			out = append(out, item)
		}
	}

	return out
}
