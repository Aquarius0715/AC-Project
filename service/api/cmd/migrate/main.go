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
// (DEC-86); both keep what exists, so they run on every start.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/bootstrap"
	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/migrate"
	"github.com/pradita/ac-project/service/api/internal/migrations"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
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
		url = "postgres://postgres:local@localhost:5432/ac?sslmode=disable"
	}

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return apperr.Wrap(err, "pgx.Connect")
	}
	defer conn.Close(context.WithoutCancel(ctx))

	switch cmd {
	case "up":
		ms, err := migrate.Load(migrations.FS)
		if err != nil {
			return apperr.Wrap(err, "migrate.Load")
		}

		applied, err := migrate.Up(ctx, conn, ms)
		if err != nil {
			return apperr.Wrap(err, "migrate.Up")
		}

		v, _, err := migrate.Version(ctx, conn)
		if err != nil {
			return apperr.Wrap(err, "migrate.Version")
		}

		log.Printf("migrate: applied %d migration(s); version %d", len(applied), v)

		return local(ctx, conn)
	case "version":
		v, dirty, err := migrate.Version(ctx, conn)
		if err != nil {
			return apperr.Wrap(err, "migrate.Version")
		}

		fmt.Printf("%d dirty=%t\n", v, dirty)

		return nil
	case "force":
		if len(args) < 2 {
			return errForceUsage
		}

		v, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return apperr.Wrap(err, "strconv.ParseInt")
		}

		return apperr.Wrap(migrate.Force(ctx, conn, v), "migrate.Force")
	case "bootstrap-admin":
		return bootstrapAdmin(ctx, conn, args[1:])
	}

	return fmt.Errorf("%w %q (want up, version, force or bootstrap-admin)", errUnknownCommand, cmd)
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
		err := migrate.EnsureLogin(ctx, conn, "ac_app_login", password, "ac_app")
		if err != nil {
			return apperr.Wrap(err, "migrate.EnsureLogin")
		}

		log.Printf("migrate: login role ac_app_login ready")

		for _, domain := range []string{"identity", "equipment", "maintenance", "billing", "energy"} {
			// one login per domain service (IR194)
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
	}

	path := os.Getenv("SEED_FIXTURE")
	if path == "" {
		return nil
	}

	done, err := migrate.Seeded(ctx, conn)
	if err != nil {
		return apperr.Wrap(err, "migrate.Seeded")
	}

	fixture, err := seed.Load(path)
	if err != nil {
		return apperr.Wrap(err, "seed.Load")
	}

	if done {
		log.Printf("migrate: fixture already loaded")
	} else {
		err := pgx.BeginFunc(
			ctx,
			conn,
			func(tx pgx.Tx) error { return seed.Apply(ctx, tx, fixture) },
		)
		if err != nil {
			return apperr.Wrap(err, "transaction")
		}

		log.Printf(
			"migrate: seeded %d actors, %d units",
			len(fixture.Actors),
			len(fixture.DemoSeed.Units),
		)
	}
	// SEED_MOCK_FLEET=1 (development only): one air conditioner and IoT module per state pattern, on the fixture's clock
	if os.Getenv("SEED_MOCK_FLEET") == "1" {
		fleet, err := devicesim.MockFleet()
		if err != nil {
			return apperr.Wrap(err, "devicesim.MockFleet")
		}

		var units int

		beginFuncErr := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
			var err error

			units, err = seed.ApplyFleet(ctx, tx, fleet, fixture.SeedCreatedAt)

			return apperr.Wrap(err, "seed.ApplyFleet")
		})
		if beginFuncErr != nil {
			return apperr.Wrap(beginFuncErr, "transaction")
		}

		log.Printf("migrate: mock fleet of %d units ready", units)
	}
	// SEED_LIVE_SENSORS=1 (development only): the live site that real sensor nodes report to (DEC-86, IR524)
	if os.Getenv("SEED_LIVE_SENSORS") == "1" {
		err := pgx.BeginFunc(
			ctx,
			conn,
			func(tx pgx.Tx) error { return seed.ApplyLive(ctx, tx, fixture.SeedCreatedAt) },
		)
		if err != nil {
			return apperr.Wrap(err, "transaction")
		}

		log.Printf(
			"migrate: live sensor site of %s ready (%s)",
			seed.LiveSite.CustomerName,
			seed.LiveSite.UnitName,
		)
	}

	return nil
}
