// Package runner applies the embedded SQL migrations in version order (database design §9).
// The bookkeeping table is golang-migrate compatible: public.schema_migrations(version bigint, dirty boolean)
// holding one row, so the golang-migrate CLI can inspect or force the same database.
package runner

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"regexp"
	"sort"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
)

// lockKey serialises concurrent runners (one-off ECS tasks may overlap during a deploy).
const lockKey int64 = 0x41435f6d696772 // "AC_migr"

// noTxMarker on the first line runs a migration outside a transaction (CREATE INDEX CONCURRENTLY).
const noTxMarker = "-- migrate:no-transaction"

var fileRE = regexp.MustCompile(`^(\d+)_([a-z0-9_]+)\.up\.sql$`)

// Migration is one numbered up migration.
type Migration struct {
	Version int64
	Name    string
	SQL     string
}

// NoTx reports whether the migration must run outside a transaction.
func (m Migration) NoTx() bool { return strings.HasPrefix(m.SQL, noTxMarker) }

// ErrDirty means an earlier non-transactional migration failed half way; fix the database and force the version.
var ErrDirty = errors.New("migrate: database is dirty")

// ErrUnversioned means the database already has the application schemas but no recorded version
// (for example one built by psql from schema.sql); record the matching version with Force.
var ErrUnversioned = errors.New("migrate: schema exists without schema_migrations; run migrate force 1 if it matches 000001_init")

// Load reads every *.up.sql file in fsys, sorted by version.
func Load(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, err
	}
	var out []Migration
	seen := map[int64]string{}
	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".sql") {
			continue
		}
		m := fileRE.FindStringSubmatch(e.Name())
		if m == nil {
			return nil, fmt.Errorf("migrate: bad file name %q (want NNNNNN_name.up.sql)", e.Name())
		}
		v, _ := strconv.ParseInt(m[1], 10, 64)
		if v <= 0 {
			return nil, fmt.Errorf("migrate: version must be positive in %q", e.Name())
		}
		if prev, dup := seen[v]; dup {
			return nil, fmt.Errorf("migrate: version %d used by %q and %q", v, prev, e.Name())
		}
		seen[v] = e.Name()
		b, err := fs.ReadFile(fsys, e.Name())
		if err != nil {
			return nil, err
		}
		out = append(out, Migration{Version: v, Name: m[2], SQL: string(b)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })
	return out, nil
}

// Version returns the applied version (0 when nothing has run) and the dirty flag.
func Version(ctx context.Context, conn *pgx.Conn) (int64, bool, error) {
	if err := ensureTable(ctx, conn); err != nil {
		return 0, false, err
	}
	var v int64
	var dirty bool
	err := conn.QueryRow(ctx, `SELECT version, dirty FROM public.schema_migrations LIMIT 1`).Scan(&v, &dirty)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}
	return v, dirty, err
}

// Up applies every migration newer than the recorded version and returns the versions it applied.
func Up(ctx context.Context, conn *pgx.Conn, ms []Migration) ([]int64, error) {
	if _, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey); err != nil {
		return nil, err
	}
	defer conn.Exec(context.WithoutCancel(ctx), `SELECT pg_advisory_unlock($1)`, lockKey) //nolint:errcheck
	cur, dirty, err := Version(ctx, conn)
	if err != nil {
		return nil, err
	}
	if dirty {
		return nil, fmt.Errorf("%w at version %d", ErrDirty, cur)
	}
	if cur == 0 {
		var exists bool
		if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'platform')`).Scan(&exists); err != nil {
			return nil, err
		}
		if exists {
			return nil, ErrUnversioned
		}
	}
	var applied []int64
	for _, m := range ms {
		if m.Version <= cur {
			continue
		}
		if err := apply(ctx, conn, m); err != nil {
			return applied, fmt.Errorf("migrate: %06d_%s: %w", m.Version, m.Name, err)
		}
		applied = append(applied, m.Version)
	}
	return applied, nil
}

// Force records version as applied and clean without running SQL (golang-migrate "force").
func Force(ctx context.Context, conn *pgx.Conn, version int64) error {
	if err := ensureTable(ctx, conn); err != nil {
		return err
	}
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error { return setVersion(ctx, tx, version, false) })
}

func apply(ctx context.Context, conn *pgx.Conn, m Migration) error {
	if m.NoTx() {
		if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error { return setVersion(ctx, tx, m.Version, true) }); err != nil {
			return err
		}
		if _, err := conn.Exec(ctx, m.SQL); err != nil {
			return err
		}
		return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error { return setVersion(ctx, tx, m.Version, false) })
	}
	return pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		if _, err := tx.Exec(ctx, m.SQL); err != nil {
			return err
		}
		return setVersion(ctx, tx, m.Version, false)
	})
}

func setVersion(ctx context.Context, tx pgx.Tx, v int64, dirty bool) error {
	if _, err := tx.Exec(ctx, `DELETE FROM public.schema_migrations`); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `INSERT INTO public.schema_migrations (version, dirty) VALUES ($1, $2)`, v, dirty)
	return err
}

func ensureTable(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(ctx, `CREATE TABLE IF NOT EXISTS public.schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)`)
	return err
}

// EnsureLogin creates a LOGIN role that inherits from group (local and CI only; production uses IAM auth).
func EnsureLogin(ctx context.Context, conn *pgx.Conn, login, password, group string) error {
	ident := pgx.Identifier{login}.Sanitize()
	var exists bool
	if err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, login).Scan(&exists); err != nil {
		return err
	}
	if !exists {
		if _, err := conn.Exec(ctx, fmt.Sprintf(`CREATE ROLE %s LOGIN IN ROLE %s`, ident, pgx.Identifier{group}.Sanitize())); err != nil {
			return err
		}
	}
	// ALTER ROLE ... PASSWORD takes no bind parameters; quote the literal.
	_, err := conn.Exec(ctx, fmt.Sprintf(`ALTER ROLE %s PASSWORD %s`, ident, quoteLiteral(password)))
	return err
}

func quoteLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// Seeded reports whether the demo fixture is already loaded (identity.users has rows).
func Seeded(ctx context.Context, conn *pgx.Conn) (bool, error) {
	var ok bool
	err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.users)`).Scan(&ok)
	return ok, err
}
