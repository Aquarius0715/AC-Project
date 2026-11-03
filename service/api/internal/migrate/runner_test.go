package migrate

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/migrations"
)

func TestLoadSortsAndValidates(t *testing.T) {
	ms, err := Load(fstest.MapFS{
		"000002_b.up.sql": {Data: []byte("SELECT 2"), Mode: 0, ModTime: time.Time{}, Sys: nil},
		"000001_a.up.sql": {Data: []byte("SELECT 1"), Mode: 0, ModTime: time.Time{}, Sys: nil},
		"README.md":       {Data: []byte("ignored"), Mode: 0, ModTime: time.Time{}, Sys: nil},
	})
	if err != nil || len(ms) != 2 || ms[0].Version != 1 || ms[1].Name != "b" {
		t.Fatalf("Load = %+v, %v", ms, err)
	}

	for name, fsys := range map[string]fstest.MapFS{
		"bad name":  {"init.sql": {Data: nil, Mode: 0, ModTime: time.Time{}, Sys: nil}},
		"zero":      {"000000_zero.up.sql": {Data: nil, Mode: 0, ModTime: time.Time{}, Sys: nil}},
		"duplicate": {"000001_a.up.sql": {Data: nil, Mode: 0, ModTime: time.Time{}, Sys: nil}, "1_b.up.sql": {Data: nil, Mode: 0, ModTime: time.Time{}, Sys: nil}},
	} {
		_, err := Load(fsys)
		if err == nil {
			t.Errorf("%s: want error", name)
		}
	}
}

// The first migration is the design schema verbatim until the first release freezes it.
func TestInitMatchesDesignSchema(t *testing.T) {
	want, err := os.ReadFile("../../../../docs/02-design/db/schema.sql")
	if err != nil {
		t.Skip("design schema not available:", err)
	}

	got, err := migrations.FS.ReadFile("000001_init.up.sql")
	if err != nil {
		t.Fatal(err)
	}

	if string(got) != string(want) {
		t.Fatal(
			"migrations/000001_init.up.sql differs from docs/02-design/db/schema.sql; run make migrations",
		)
	}
}

// testDB creates an empty database and returns an owner connection to it.
func testDB(t *testing.T) *pgx.Conn {
	t.Helper()

	ctx := context.Background()

	admin := os.Getenv("MIGRATE_TEST_ADMIN_URL")
	if admin == "" {
		admin = "postgres://postgres:local@localhost:5432/postgres?sslmode=disable"
	}

	ac, err := pgx.Connect(ctx, admin)
	if err != nil {
		t.Skip("postgres not available:", err)
	}

	t.Cleanup(func() { ac.Close(ctx) })

	const name = "ac_migrate_test"
	_, execErr := ac.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)")
	if execErr != nil {
		t.Fatal(execErr)
	}

	_, execErr = ac.Exec(ctx, "CREATE DATABASE "+name)
	if execErr != nil {
		t.Fatal(execErr)
	}

	cfg, _ := pgx.ParseConfig(admin)
	cfg.Database = name

	c, err := pgx.ConnectConfig(ctx, cfg)
	if err != nil {
		t.Fatal(err)
	}

	t.Cleanup(func() {
		c.Close(ctx)
		ac.Exec(ctx, "DROP DATABASE IF EXISTS "+name+" WITH (FORCE)") //nolint:errcheck
	})

	return c
}

func TestUpAppliesOnceAndRecordsVersion(t *testing.T) {
	ctx := context.Background()
	c := testDB(t)

	ms, err := Load(migrations.FS)
	if err != nil {
		t.Fatal(err)
	}

	extra := append(
		ms,
		Migration{Version: 900, Name: "probe", SQL: "CREATE TABLE public.probe (id int)"},
	)

	got, err := Up(ctx, c, extra)
	if err != nil || len(got) != len(extra) {
		t.Fatalf("Up = %v, %v", got, err)
	}

	if v, dirty, _ := Version(ctx, c); v != 900 || dirty {
		t.Fatalf("version = %d dirty=%t", v, dirty)
	}

	again, upErr := Up(ctx, c, extra)
	if upErr != nil || len(again) != 0 {
		t.Fatalf("second Up = %v, %v", again, upErr)
	}

	_, execErr := c.Exec(ctx, "DELETE FROM public.schema_migrations")
	if execErr != nil {
		t.Fatal(execErr)
	}

	_, upErr = Up(ctx, c, extra)
	if !errors.Is(upErr, ErrUnversioned) {
		t.Fatalf("want ErrUnversioned, got %v", upErr)
	}

	forceErr := Force(ctx, c, 900)
	if forceErr != nil {
		t.Fatal(forceErr)
	}

	ok, seededErr := Seeded(ctx, c)
	if seededErr != nil || ok {
		t.Fatalf("Seeded = %t, %v", ok, seededErr)
	}

	ensureLoginErr := EnsureLogin(ctx, c, "ac_app_login", "it's-local", "ac_app")
	if ensureLoginErr != nil {
		t.Fatal(ensureLoginErr)
	}

	ensureLoginErr = EnsureLogin(ctx, c, "ac_app_login", "local", "ac_app")
	if ensureLoginErr != nil {
		t.Fatal("EnsureLogin must be idempotent:", ensureLoginErr)
	}
}

func TestFailedMigrationRollsBack(t *testing.T) {
	ctx := context.Background()
	c := testDB(t)
	bad := []Migration{
		{Version: 1, Name: "ok", SQL: "CREATE TABLE public.a (id int)"},
		{Version: 2, Name: "bad", SQL: "CREATE TABLE public.b (id int); SELECT nope"},
	}

	applied, err := Up(ctx, c, bad)
	if err == nil || len(applied) != 1 || !strings.Contains(err.Error(), "000002_bad") {
		t.Fatalf("Up = %v, %v", applied, err)
	}

	if v, dirty, _ := Version(ctx, c); v != 1 || dirty {
		t.Fatalf("version = %d dirty=%t", v, dirty)
	}

	var b *string
	scanErr := c.QueryRow(ctx, "SELECT to_regclass('public.b')::text").
		Scan(&b)
	if scanErr != nil ||
		b != nil {
		t.Fatalf("table b must not exist: %v %v", b, scanErr)
	}
}

func TestNoTxFailureLeavesDirty(t *testing.T) {
	ctx := context.Background()
	c := testDB(t)

	ms := []Migration{
		{
			Version: 3,
			Name:    "idx",
			SQL:     noTxMarker + "\nCREATE INDEX CONCURRENTLY x ON public.missing (id)",
		},
	}
	if !ms[0].NoTx() {
		t.Fatal("marker not detected")
	}

	_, err := Up(ctx, c, ms)
	if err == nil {
		t.Fatal("want error")
	}

	_, err = Up(ctx, c, ms)
	if !errors.Is(err, ErrDirty) {
		t.Fatalf("want ErrDirty, got %v", err)
	}

	err = Force(ctx, c, 2)
	if err != nil {
		t.Fatal(err)
	}

	ok := []Migration{
		{Version: 3, Name: "idx", SQL: noTxMarker + "\nCREATE TABLE public.t (id int)"},
	}
	got, err := Up(ctx, c, ok)
	if err != nil || len(got) != 1 {
		t.Fatalf("Up after force = %v, %v", got, err)
	}

	if v, dirty, _ := Version(ctx, c); v != 3 || dirty {
		t.Fatalf("version = %d dirty=%t", v, dirty)
	}
}
