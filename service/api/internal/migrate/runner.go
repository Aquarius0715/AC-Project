// Package migrate applies the embedded SQL migrations in version order (database design §9).
// The bookkeeping table is golang-migrate compatible: public.schema_migrations(version bigint, dirty boolean)
// holding one row, so the golang-migrate CLI can inspect or force the same database.
package migrate

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

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// lockKey serialises concurrent runners (one-off ECS tasks may overlap during a deploy).
const lockKey int64 = 0x41435f6d696772 // "AC_migr"

// noTxMarker on the first line runs a migration outside a transaction (CREATE INDEX CONCURRENTLY).
const noTxMarker = "-- migrate:no-transaction"

var fileRE = regexp.MustCompile(`^(\d+)_([a-z0-9_]+)\.up\.sql$`)

// The problems Load finds in the migration files.
var (
	errBadFileName        = errors.New("bad file name")
	errVersionNotPositive = errors.New("version must be positive")
	errVersionUsed        = errors.New("used by")
)

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
var ErrUnversioned = errors.New(
	"migrate: schema exists without schema_migrations; run migrate force 1 if it matches 000001_init",
)

// Load reads every *.up.sql file in fsys, sorted by version.
func Load(fsys fs.FS) ([]Migration, error) {
	entries, err := fs.ReadDir(fsys, ".")
	if err != nil {
		return nil, apperr.Wrap(err, "fs.ReadDir")
	}

	var out []Migration

	seen := map[int64]string{}

	for _, dirEntry := range entries {
		if dirEntry.IsDir() || !strings.HasSuffix(dirEntry.Name(), ".sql") {
			continue
		}

		match := fileRE.FindStringSubmatch(dirEntry.Name())
		if match == nil {
			return nil, fmt.Errorf(
				"migrate: %w %q (want NNNNNN_name.up.sql)",
				errBadFileName,
				dirEntry.Name(),
			)
		}

		version, _ := strconv.ParseInt(match[1], 10, 64)
		if version <= 0 {
			return nil, fmt.Errorf("migrate: %w in %q", errVersionNotPositive, dirEntry.Name())
		}

		if prev, dup := seen[version]; dup {
			return nil, fmt.Errorf(
				"migrate: version %d %w %q and %q",
				version,
				errVersionUsed,
				prev,
				dirEntry.Name(),
			)
		}

		seen[version] = dirEntry.Name()

		b, err := fs.ReadFile(fsys, dirEntry.Name())
		if err != nil {
			return nil, apperr.Wrap(err, "fs.ReadFile")
		}

		out = append(out, Migration{Version: version, Name: match[2], SQL: string(b)})
	}

	sort.Slice(out, func(i, j int) bool { return out[i].Version < out[j].Version })

	return out, nil
}

// Version returns the applied version (0 when nothing has run) and the dirty flag.
func Version(ctx context.Context, conn *pgx.Conn) (int64, bool, error) {
	ensureTableErr := ensureTable(ctx, conn)
	if ensureTableErr != nil {
		return 0, false, ensureTableErr
	}

	var (
		version int64
		dirty   bool
	)

	err := conn.QueryRow(ctx, `SELECT version, dirty FROM public.schema_migrations LIMIT 1`).
		Scan(&version, &dirty)
	if errors.Is(err, pgx.ErrNoRows) {
		return 0, false, nil
	}

	return version, dirty, apperr.Wrap(err, "select public.schema_migrations")
}

// Up applies every migration newer than the recorded version and returns the versions it applied.
func Up(ctx context.Context, conn *pgx.Conn, available []Migration) ([]int64, error) {
	_, err := conn.Exec(ctx, `SELECT pg_advisory_lock($1)`, lockKey)
	if err != nil {
		return nil, apperr.Wrap(err, "select pg_advisory_lock")
	}

	defer func() {
		_, _ = conn.Exec(
			context.WithoutCancel(ctx),
			`SELECT pg_advisory_unlock($1)`,
			lockKey,
		)
	}()

	cur, dirty, err := Version(ctx, conn)
	if err != nil {
		return nil, err
	}

	if dirty {
		return nil, fmt.Errorf("%w at version %d", ErrDirty, cur)
	}

	if cur == 0 {
		var exists bool

		err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_namespace WHERE nspname = 'platform')`).
			Scan(&exists)
		if err != nil {
			return nil, apperr.Wrap(err, "select pg_namespace")
		}

		if exists {
			return nil, ErrUnversioned
		}
	}

	var applied []int64

	for _, migration := range available {
		if migration.Version <= cur {
			continue
		}

		err := apply(ctx, conn, migration)
		if err != nil {
			return applied, fmt.Errorf(
				"migrate: %06d_%s: %w",
				migration.Version,
				migration.Name,
				err,
			)
		}

		applied = append(applied, migration.Version)
	}

	return applied, nil
}

// Force records version as applied and clean without running SQL (golang-migrate "force").
func Force(ctx context.Context, conn *pgx.Conn, version int64) error {
	err := ensureTable(ctx, conn)
	if err != nil {
		return err
	}

	return apperr.Wrap(pgx.BeginFunc(
		ctx,
		conn,
		func(tx pgx.Tx) error { return setVersion(ctx, tx, version, false) },
	), "transaction")
}

func apply(ctx context.Context, conn *pgx.Conn, migration Migration) error {
	if migration.NoTx() {
		err := pgx.BeginFunc(
			ctx,
			conn,
			func(tx pgx.Tx) error { return setVersion(ctx, tx, migration.Version, true) },
		)
		if err != nil {
			return apperr.Wrap(err, "transaction")
		}

		_, err = conn.Exec(ctx, migration.SQL)
		if err != nil {
			return apperr.Wrap(err, "exec (apply)")
		}

		return apperr.Wrap(pgx.BeginFunc(
			ctx,
			conn,
			func(tx pgx.Tx) error { return setVersion(ctx, tx, migration.Version, false) },
		), "transaction")
	}

	return apperr.Wrap(pgx.BeginFunc(ctx, conn, func(txn pgx.Tx) error {
		_, err := txn.Exec(ctx, migration.SQL)
		if err != nil {
			return apperr.Wrap(err, "exec (apply)")
		}

		return setVersion(ctx, txn, migration.Version, false)
	}), "transaction")
}

func setVersion(ctx context.Context, txn pgx.Tx, version int64, dirty bool) error {
	_, execErr := txn.Exec(ctx, `DELETE FROM public.schema_migrations`)
	if execErr != nil {
		return apperr.Wrap(execErr, "delete public.schema_migrations")
	}

	_, err := txn.Exec(
		ctx,
		`INSERT INTO public.schema_migrations (version, dirty) VALUES ($1, $2)`,
		version,
		dirty,
	)

	return apperr.Wrap(err, "insert public.schema_migrations")
}

func ensureTable(ctx context.Context, conn *pgx.Conn) error {
	_, err := conn.Exec(
		ctx,
		`CREATE TABLE IF NOT EXISTS public.schema_migrations (version bigint NOT NULL PRIMARY KEY, dirty boolean NOT NULL)`,
	)

	return apperr.Wrap(err, "exec (ensureTable)")
}

// EnsureLogin creates a LOGIN role that inherits from group (local and CI only; production uses IAM auth).
func EnsureLogin(ctx context.Context, conn *pgx.Conn, login, password, group string) error {
	ident := pgx.Identifier{login}.Sanitize()

	var exists bool

	scanErr := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = $1)`, login).
		Scan(&exists)
	if scanErr != nil {
		return apperr.Wrap(scanErr, "select pg_roles")
	}

	if !exists {
		_, err := conn.Exec(
			ctx,
			fmt.Sprintf(`CREATE ROLE %s LOGIN IN ROLE %s`, ident, pgx.Identifier{group}.Sanitize()),
		)
		if err != nil {
			return apperr.Wrap(err, "exec (EnsureLogin)")
		}
	}
	// ALTER ROLE ... PASSWORD takes no bind parameters; quote the literal.
	_, err := conn.Exec(
		ctx,
		fmt.Sprintf(`ALTER ROLE %s PASSWORD %s`, ident, quoteLiteral(password)),
	)

	return apperr.Wrap(err, "exec (EnsureLogin)")
}

func quoteLiteral(s string) string { return "'" + strings.ReplaceAll(s, "'", "''") + "'" }

// Seeded reports whether the demo fixture is already loaded (identity.users has rows).
func Seeded(ctx context.Context, conn *pgx.Conn) (bool, error) {
	var ok bool

	err := conn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.users)`).Scan(&ok)

	return ok, apperr.Wrap(err, "select identity.users")
}
