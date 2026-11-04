// Command migrate applies the embedded schema migrations (database design §9) as the schema owner.
//
//	migrate up          apply pending migrations
//	migrate version     print the applied version
//	migrate force N     record version N as clean without running SQL
//	migrate bootstrap-admin -tenant NAME -email ADDRESS -name NAME [-organization NAME]
//	                    create the first HQ administrator of a tenant (DEC-84, IR509); refused once it has one
//
// Local and CI only: APP_LOGIN_PASSWORD creates or updates the ac_app_login role and the domain service logins
// ac_<domain>_login (IR194), and SEED_FIXTURE loads the demo fixture once (skipped when identity.users already has
// rows). With the fixture, SEED_MOCK_FLEET=1 adds the mock fleet and SEED_LIVE_SENSORS=1 the live sensor site
// (DEC-86); both keep what exists, so they run on every start. The shared test environment (DEC-96) loads the
// fixture's accounts only (SEED_ACCOUNTS_ONLY=1) and puts the live site at the customer of another client of the
// fixture (SEED_LIVE_CLIENT, customer-b when empty), as of now when the scenario runs on the real clock
// (DEMO_CLOCK_START=now).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/bootstrap"
	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/migrate"
	"github.com/pradita/ac-project/service/api/internal/migrations"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/democlock"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

var (
	// errForceUsage refuses force without the version to record.
	errForceUsage = errors.New("usage: migrate force VERSION")
	// errUnknownCommand refuses a command migrate does not have.
	errUnknownCommand = errors.New("unknown command")
)

func main() {
	err := run(context.Background(), os.Args[1:])
	if err != nil {
		log.Fatal(err)
	}
}

func run(ctx context.Context, args []string) error {
	cmd := "up"
	if len(args) > 0 {
		cmd = args[0]
	}

	url := os.Getenv("DATABASE_URL")
	if url == "" {
		//nolint:gosec // G101: the local development database of compose.yaml, not a secret
		url = "postgres://postgres:local@localhost:5432/ac?sslmode=disable"
	}

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return apperr.Wrap(err, "pgx.Connect")
	}

	defer func() {
		closeErr := conn.Close(context.WithoutCancel(ctx))
		if closeErr != nil {
			log.Printf("migrate: closing the connection: %v", closeErr)
		}
	}()

	switch cmd {
	case "up":
		return up(ctx, conn)
	case "version":
		return version(ctx, conn)
	case "force":
		return force(ctx, conn, args[1:])
	case "bootstrap-admin":
		return bootstrapAdmin(ctx, conn, args[1:])
	}

	return fmt.Errorf("%w %q (want up, version, force or bootstrap-admin)", errUnknownCommand, cmd)
}

// up applies the pending migrations, then the development-only steps.
func up(ctx context.Context, conn *pgx.Conn) error {
	ms, err := migrate.Load(migrations.FS)
	if err != nil {
		return apperr.Wrap(err, "migrate.Load")
	}

	applied, err := migrate.Up(ctx, conn, ms)
	if err != nil {
		return apperr.Wrap(err, "migrate.Up")
	}

	current, _, err := migrate.Version(ctx, conn)
	if err != nil {
		return apperr.Wrap(err, "migrate.Version")
	}

	//nolint:gosec // G706: two integers; the taint is DATABASE_URL reaching the connection they were read through
	log.Printf("migrate: applied %d migration(s); version %d", len(applied), current)

	return local(ctx, conn)
}

// version prints the applied version and whether it is dirty, the command's output on stdout.
func version(ctx context.Context, conn *pgx.Conn) error {
	current, dirty, err := migrate.Version(ctx, conn)
	if err != nil {
		return apperr.Wrap(err, "migrate.Version")
	}

	_, err = fmt.Fprintf(os.Stdout, "%d dirty=%t\n", current, dirty)

	return apperr.Wrap(err, "fmt.Fprintf")
}

// force records the version in args[0] as clean without running SQL.
func force(ctx context.Context, conn *pgx.Conn, args []string) error {
	if len(args) == 0 {
		return errForceUsage
	}

	recorded, err := strconv.ParseInt(args[0], 10, 64)
	if err != nil {
		return apperr.Wrap(err, "strconv.ParseInt")
	}

	return apperr.Wrap(migrate.Force(ctx, conn, recorded), "migrate.Force")
}

// bootstrapAdmin creates the first HQ administrator of a tenant (DEC-84, IR509). The identity service's consumer then
// opens the sign-in account, which asks for a one-time-code device at the first sign-in (IR468), and e-mails the
// set-up link (in development to the local Mailpit inbox).
func bootstrapAdmin(ctx context.Context, conn *pgx.Conn, args []string) error {
	flagSet := flag.NewFlagSet("bootstrap-admin", flag.ContinueOnError)

	var input bootstrap.Input
	flagSet.StringVar(
		&input.Tenant,
		"tenant",
		"",
		"the tenant's name (an existing tenant of that name, or a new one)",
	)
	flagSet.StringVar(
		&input.Organization,
		"organization",
		"HQ",
		"the HQ organization's name, when the tenant has none",
	)
	flagSet.StringVar(
		&input.Email,
		"email",
		"",
		"the administrator's e-mail address (the set-up link goes there)",
	)
	flagSet.StringVar(&input.Name, "name", "", "the administrator's display name")

	err := flagSet.Parse(args)
	if err != nil {
		return apperr.Wrap(err, "flag.FlagSet.Parse")
	}

	var result bootstrap.Result

	err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		var err error

		result, err = bootstrap.Admin(ctx, tx, input)

		return apperr.Wrap(err, "bootstrap.Admin")
	})
	if err != nil {
		return fmt.Errorf("bootstrap-admin: %w", err)
	}

	log.Printf(
		"bootstrap-admin: tenant %s (new %t), HQ organization %s (new %t), administrator %s, membership %s; the set-up "+
			"e-mail goes to %s",
		result.TenantID,
		result.NewTenant,
		result.OrganizationID,
		result.NewOrganization,
		result.UserID,
		result.MembershipID,
		input.Email,
	)

	return nil
}

// local applies the development-only steps controlled by environment variables.
func local(ctx context.Context, conn *pgx.Conn) error {
	if password := os.Getenv("APP_LOGIN_PASSWORD"); password != "" {
		err := ensureLogins(ctx, conn, password)
		if err != nil {
			return err
		}
	}

	path := os.Getenv("SEED_FIXTURE")
	if path == "" {
		return nil
	}

	fixture, err := seedFixture(ctx, conn, path, os.Getenv("SEED_ACCOUNTS_ONLY") == "1")
	if err != nil {
		return err
	}
	// SEED_MOCK_FLEET=1 (development only): one air conditioner and IoT module per state pattern, on the fixture's clock
	if os.Getenv("SEED_MOCK_FLEET") == "1" {
		err := seedFleet(ctx, conn, fixture)
		if err != nil {
			return err
		}
	}
	// SEED_LIVE_SENSORS=1 (development and the shared test environment): the live site that real sensor nodes report to
	// (DEC-86, IR524)
	if os.Getenv("SEED_LIVE_SENSORS") == "1" {
		return seedLive(ctx, conn, fixture)
	}

	return nil
}

// seedLive adds the live sensor site at the customer of the client SEED_LIVE_CLIENT names (customer-b when empty), as
// of the fixture's seed time, or of now when the scenario runs on the real clock (DEMO_CLOCK_START=now, DEC-96).
func seedLive(ctx context.Context, conn *pgx.Conn, fixture *seed.Fixture) error {
	site, err := seed.LiveSiteAt(fixture, os.Getenv("SEED_LIVE_CLIENT"))
	if err != nil {
		return apperr.Wrap(err, "seed.LiveSiteAt")
	}

	since := fixture.SeedCreatedAt
	if os.Getenv("DEMO_CLOCK_START") == democlock.RealTime {
		since = time.Now().UTC()
	}

	err = pgx.BeginFunc(
		ctx,
		conn,
		func(tx pgx.Tx) error { return seed.ApplyLive(ctx, tx, site, since) },
	)
	if err != nil {
		return apperr.Wrap(err, "transaction")
	}

	log.Printf( // quoted: SEED_LIVE_CLIENT chose the site's customer
		"migrate: live sensor site of %s ready (%s)",
		strconv.Quote(site.CustomerName),
		strconv.Quote(strings.Join(site.UnitNames(), ", ")),
	)

	return nil
}

// ensureLogins creates or updates the application's login role and one login per domain service (IR194).
func ensureLogins(ctx context.Context, conn *pgx.Conn, password string) error {
	err := migrate.EnsureLogin(ctx, conn, "ac_app_login", password, "ac_app")
	if err != nil {
		return apperr.Wrap(err, "migrate.EnsureLogin")
	}

	log.Printf("migrate: login role ac_app_login ready")

	for _, domain := range []string{"identity", "equipment", "maintenance", "billing", "energy"} {
		err := migrate.EnsureLogin(
			ctx,
			conn,
			"ac_"+domain+"_login",
			password,
			"ac_svc_"+domain,
		)
		if err != nil {
			return apperr.Wrap(err, "migrate.EnsureLogin")
		}
	}

	log.Printf("migrate: domain service logins ready")

	return nil
}

// seedFixture loads the demo fixture at path once, or its accounts only: a database whose identity.users has rows keeps
// what it has.
func seedFixture(
	ctx context.Context,
	conn *pgx.Conn,
	path string,
	accountsOnly bool,
) (*seed.Fixture, error) {
	done, err := migrate.Seeded(ctx, conn)
	if err != nil {
		return nil, apperr.Wrap(err, "migrate.Seeded")
	}

	fixture, err := seed.Load(path)
	if err != nil {
		return nil, apperr.Wrap(err, "seed.Load")
	}

	if done {
		log.Printf("migrate: fixture already loaded")

		return fixture, nil
	}

	apply, seeded := seed.Apply, fmt.Sprintf(
		"%d actors, %d units",
		len(fixture.Actors),
		len(fixture.DemoSeed.Units),
	)
	if accountsOnly {
		apply, seeded = seed.ApplyAccounts, fmt.Sprintf("the %d accounts only", len(fixture.Actors))
	}

	err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error { return apply(ctx, tx, fixture) })
	if err != nil {
		return nil, apperr.Wrap(err, "transaction")
	}

	log.Printf("migrate: seeded %s", seeded)

	return fixture, nil
}

// seedFleet adds the mock fleet on the fixture's clock; units that exist are kept.
func seedFleet(ctx context.Context, conn *pgx.Conn, fixture *seed.Fixture) error {
	fleet, err := devicesim.MockFleet()
	if err != nil {
		return apperr.Wrap(err, "devicesim.MockFleet")
	}

	var units int

	err = pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		var applyErr error

		units, applyErr = seed.ApplyFleet(ctx, tx, fleet, fixture.SeedCreatedAt)

		return apperr.Wrap(applyErr, "seed.ApplyFleet")
	})
	if err != nil {
		return apperr.Wrap(err, "transaction")
	}

	log.Printf("migrate: mock fleet of %d units ready", units)

	return nil
}
