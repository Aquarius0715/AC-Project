package migrate

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"slices"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/migrations"
)

func TestLoadSortsAndValidates(t *testing.T) {
	t.Parallel()

	loaded, err := Load(fstest.MapFS{
		"000002_b.up.sql": {Data: []byte("SELECT 2"), Mode: 0, ModTime: time.Time{}, Sys: nil},
		"000001_a.up.sql": {Data: []byte("SELECT 1"), Mode: 0, ModTime: time.Time{}, Sys: nil},
		"README.md":       {Data: []byte("ignored"), Mode: 0, ModTime: time.Time{}, Sys: nil},
	})
	if err != nil || len(loaded) != 2 || loaded[0].Version != 1 || loaded[1].Name != "b" {
		t.Fatalf("Load = %+v, %v", loaded, err)
	}

	for name, fsys := range map[string]fstest.MapFS{
		"bad name": {"init.sql": {Data: nil, Mode: 0, ModTime: time.Time{}, Sys: nil}},
		"zero":     {"000000_zero.up.sql": {Data: nil, Mode: 0, ModTime: time.Time{}, Sys: nil}},
		"duplicate": {
			"000001_a.up.sql": {Data: nil, Mode: 0, ModTime: time.Time{}, Sys: nil},
			"1_b.up.sql":      {Data: nil, Mode: 0, ModTime: time.Time{}, Sys: nil},
		},
	} {
		_, err := Load(fsys)
		if err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

// baselineSum is the SHA-256 of 000001_init.up.sql as the shared test environment applied it (test1.0.0r16): the file
// is frozen (IR785). A change of the schema is a new migration with ALTERs, and docs/02-design/db/schema.sql follows
// it (test/integration TestMigrationsMatchSchema).
const baselineSum = "125ee737099ede72d1b39c2894c2cddf9a68834951b7a35845eeda39abf7a89d"

func TestBaselineIsFrozen(t *testing.T) {
	t.Parallel()

	got, err := migrations.FS.ReadFile("000001_init.up.sql")
	if err != nil {
		t.Fatal(err)
	}

	if sum := fmt.Sprintf("%x", sha256.Sum256(got)); sum != baselineSum {
		t.Fatalf(
			"000001_init.up.sql changed (sha256 %s): it is frozen, add a migration with the ALTERs instead (IR785)",
			sum,
		)
	}
}

// testDB creates an empty database and returns an owner connection to it.
func testDB(t *testing.T) *pgx.Conn {
	t.Helper()

	ctx := context.Background()

	adminURL := os.Getenv("MIGRATE_TEST_ADMIN_URL")
	if adminURL == "" {
		//nolint:gosec // local test database password, not a secret
		adminURL = "postgres://postgres:local@localhost:5432/postgres?sslmode=disable"
	}

	admin, err := pgx.Connect(ctx, adminURL)
	if err != nil {
		t.Skip("postgres not available:", err)
	}

	t.Cleanup(func() { _ = admin.Close(ctx) })

	const name = "ac_migrate_test"

	_, execErr := admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	if execErr != nil {
		t.Fatal(execErr)
	}

	_, execErr = admin.Exec(ctx, "CREATE DATABASE "+name)
	if execErr != nil {
		t.Fatal(execErr)
	}

	cfg, _ := pgx.ParseConfig(adminURL)
	cfg.Database = name

	conn, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		_ = conn.Close(ctx)
		_, _ = admin.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	})

	return conn
}

//nolint:paralleltest // every test rebuilds the same scratch database ac_migrate_test
func TestUpAppliesOnceAndRecordsVersion(t *testing.T) {
	ctx := context.Background()
	conn := testDB(t)

	loaded, err := Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}

	extra := slices.Concat(
		loaded,
		[]Migration{{Version: 900, Name: "probe", SQL: "CREATE TABLE public.probe (id int)"}},
	)

	got, err := Up(ctx, conn, extra)
	if err != nil || len(got) != len(extra) {
		t.Fatalf("Up = %v, %v", got, err)
	}

	if v, dirty, _ := Version(ctx, conn); v != 900 || dirty {
		t.Fatalf("version = %d dirty=%t", v, dirty)
	}

	again, upErr := Up(ctx, conn, extra)
	if upErr != nil || len(again) != 0 {
		t.Fatalf("second Up = %v, %v", again, upErr)
	}

	_, execErr := conn.Exec(ctx, "DELETE FROM public.schema_migrations")
	if execErr != nil {
		t.Fatal(execErr)
	}

	_, upErr = Up(ctx, conn, extra)
	if !errors.Is(upErr, ErrUnversioned) {
		t.Fatalf("want ErrUnversioned, got %v", upErr)
	}

	checkForceAndLogins(t, conn)
}

// checkForceAndLogins records version 900 by force on a database without the fixture and creates the login role
// twice.
func checkForceAndLogins(t *testing.T, conn *pgx.Conn) {
	t.Helper()

	ctx := context.Background()

	forceErr := Force(ctx, conn, 900)
	if forceErr != nil {
		t.Fatal(forceErr)
	}

	ok, seededErr := Seeded(ctx, conn)
	if seededErr != nil || ok {
		t.Fatalf("Seeded = %t, %v", ok, seededErr)
	}

	ensureLoginErr := EnsureLogin(ctx, conn, "ac_app_login", "it's-local", "ac_app")
	if ensureLoginErr != nil {
		t.Fatal(ensureLoginErr)
	}

	ensureLoginErr = EnsureLogin(ctx, conn, "ac_app_login", "local", "ac_app")
	if ensureLoginErr != nil {
		t.Fatal("EnsureLogin must be idempotent:", ensureLoginErr)
	}
}

//nolint:paralleltest // every test rebuilds the same scratch database ac_migrate_test
func TestFailedMigrationRollsBack(t *testing.T) {
	ctx := context.Background()
	conn := testDB(t)
	bad := []Migration{
		{Version: 1, Name: "ok", SQL: "CREATE TABLE public.a (id int)"},
		{Version: 2, Name: "bad", SQL: "CREATE TABLE public.b (id int); SELECT nope"},
	}

	applied, err := Up(ctx, conn, bad)
	if err == nil || len(applied) != 1 || !strings.Contains(err.Error(), "000002_bad") {
		t.Fatalf("Up = %v, %v", applied, err)
	}

	if v, dirty, _ := Version(ctx, conn); v != 1 || dirty {
		t.Fatalf("version = %d dirty=%t", v, dirty)
	}

	var table *string

	scanErr := conn.QueryRow(ctx, "SELECT to_regclass('public.b')::text").
		Scan(&table)
	if scanErr != nil ||
		table != nil {
		t.Fatalf("table b must not exist: %v %v", table, scanErr)
	}
}

//nolint:paralleltest // every test rebuilds the same scratch database ac_migrate_test
func TestNoTxFailureLeavesDirty(t *testing.T) {
	ctx := context.Background()
	conn := testDB(t)

	failing := []Migration{
		{
			Version: 3,
			Name:    "idx",
			SQL:     noTxMarker + "\nCREATE INDEX CONCURRENTLY x ON public.missing (id)",
		},
	}
	if !failing[0].NoTx() {
		t.Fatal("marker not detected")
	}

	_, err := Up(ctx, conn, failing)
	if err == nil {
		t.Fatal("want error")
	}

	_, err = Up(ctx, conn, failing)
	if !errors.Is(err, ErrDirty) {
		t.Fatalf("want ErrDirty, got %v", err)
	}

	err = Force(ctx, conn, 2)
	if err != nil {
		t.Fatal(err)
	}

	ok := []Migration{
		{Version: 3, Name: "idx", SQL: noTxMarker + "\nCREATE TABLE public.t (id int)"},
	}

	got, err := Up(ctx, conn, ok)
	if err != nil || len(got) != 1 {
		t.Fatalf("Up after force = %v, %v", got, err)
	}

	if v, dirty, _ := Version(ctx, conn); v != 3 || dirty {
		t.Fatalf("version = %d dirty=%t", v, dirty)
	}
}
