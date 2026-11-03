// Command seed loads docs/04-agentic-sdlc/fixture-contract.json into the database (run as the schema owner).
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

func main() {
	fixture := flag.String(
		"fixture",
		"../../docs/04-agentic-sdlc/fixture-contract.json",
		"fixture file",
	)
	url := flag.String("db", os.Getenv("OWNER_DATABASE_URL"), "owner database URL")

	flag.Parse()

	if *url == "" {
		*url = "postgres://postgres:local@localhost:5432/ac?sslmode=disable"
	}

	err := run(context.Background(), *fixture, *url)
	if err != nil {
		log.Fatal(err)
	}
}

// run loads the fixture in one transaction; main exits on its error once its deferred clean-up has run.
func run(ctx context.Context, fixture, url string) error {
	loaded, err := seed.Load(fixture)
	if err != nil {
		return apperr.Wrap(err, "seed.Load")
	}

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return apperr.Wrap(err, "pgx.Connect")
	}

	defer func() { _ = conn.Close(ctx) }()

	beginFuncErr := pgx.BeginFunc(
		ctx,
		conn,
		func(txn pgx.Tx) error { return seed.Apply(ctx, txn, loaded) },
	)
	if beginFuncErr != nil {
		return apperr.Wrap(beginFuncErr, "transaction")
	}

	log.Printf("seeded %d actors, %d units", len(loaded.Actors), len(loaded.DemoSeed.Units))

	return nil
}
