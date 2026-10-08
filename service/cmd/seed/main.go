// Command seed loads docs/04-agentic-sdlc/fixture-contract.json into the database (run as the schema owner).
package main

import (
	"context"
	"flag"
	"log"
	"os"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/internal/seed"
)

func main() {
	fixture := flag.String("fixture", "../docs/04-agentic-sdlc/fixture-contract.json", "fixture file")
	url := flag.String("db", os.Getenv("OWNER_DATABASE_URL"), "owner database URL")
	flag.Parse()
	if *url == "" {
		*url = "postgres://postgres:local@localhost:5432/ac?sslmode=disable"
	}
	ctx := context.Background()
	f, err := seed.Load(*fixture)
	if err != nil {
		log.Fatal(err)
	}
	conn, err := pgx.Connect(ctx, *url)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(ctx)
	if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error { return seed.Apply(ctx, tx, f) }); err != nil {
		log.Fatal(err)
	}
	log.Printf("seeded %d actors, %d units", len(f.Actors), len(f.DemoSeed.Units))
}
