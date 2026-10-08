// Command migrate applies the embedded schema migrations (database design §9) as the schema owner.
//
//	migrate up          apply pending migrations
//	migrate version     print the applied version
//	migrate force N     record version N as clean without running SQL
//
// Local and CI only: APP_LOGIN_PASSWORD creates or updates the ac_app_login role, and SEED_FIXTURE
// loads the demo fixture once (skipped when identity.users already has rows).
package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"strconv"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/internal/migrate"
	"github.com/pradita/ac-project/service/internal/migrations"
	"github.com/pradita/ac-project/service/internal/seed"
)

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
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
		return err
	}
	defer conn.Close(context.WithoutCancel(ctx))

	switch cmd {
	case "up":
		ms, err := migrate.Load(migrations.FS)
		if err != nil {
			return err
		}
		applied, err := migrate.Up(ctx, conn, ms)
		if err != nil {
			return err
		}
		v, _, err := migrate.Version(ctx, conn)
		if err != nil {
			return err
		}
		log.Printf("migrate: applied %d migration(s); version %d", len(applied), v)
		return local(ctx, conn)
	case "version":
		v, dirty, err := migrate.Version(ctx, conn)
		if err != nil {
			return err
		}
		fmt.Printf("%d dirty=%t\n", v, dirty)
		return nil
	case "force":
		if len(args) < 2 {
			return fmt.Errorf("usage: migrate force VERSION")
		}
		v, err := strconv.ParseInt(args[1], 10, 64)
		if err != nil {
			return err
		}
		return migrate.Force(ctx, conn, v)
	}
	return fmt.Errorf("unknown command %q (want up, version or force)", cmd)
}

// local applies the development-only steps controlled by environment variables.
func local(ctx context.Context, conn *pgx.Conn) error {
	if pw := os.Getenv("APP_LOGIN_PASSWORD"); pw != "" {
		if err := migrate.EnsureLogin(ctx, conn, "ac_app_login", pw, "ac_app"); err != nil {
			return err
		}
		log.Printf("migrate: login role ac_app_login ready")
	}
	path := os.Getenv("SEED_FIXTURE")
	if path == "" {
		return nil
	}
	done, err := migrate.Seeded(ctx, conn)
	if err != nil || done {
		if done {
			log.Printf("migrate: fixture already loaded")
		}
		return err
	}
	f, err := seed.Load(path)
	if err != nil {
		return err
	}
	if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error { return seed.Apply(ctx, tx, f) }); err != nil {
		return err
	}
	log.Printf("migrate: seeded %d actors, %d units", len(f.Actors), len(f.DemoSeed.Units))
	return nil
}
