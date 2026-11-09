// Command loadseed fills the database ac_load of the compose project ac-load with the launch size of the platform (600
// customers, 3,000 units) and the synthetic accounts of the load test, and writes the account manifest. It runs once
// per load run, after migrate, as the schema owner; any other database name is refused.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/loadseed"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// manifestMode is the mode of the manifest: it holds the passwords of a throwaway stack, for the user only.
const manifestMode = 0o600

func main() {
	defaults := loadseed.LaunchConfig("", time.Now())
	url := flag.String(
		"db",
		os.Getenv("OWNER_DATABASE_URL"),
		"owner database URL; the database must be "+loadseed.Database,
	)
	manifest := flag.String("manifest", "manifest.json", "where the account manifest is written")
	manifestOnly := flag.Bool(
		"manifest-only",
		false,
		"write the manifest and touch no database (the realm needs it before the stack starts)",
	)
	password := flag.String(
		"password",
		os.Getenv("LOAD_PASSWORD"),
		"the password of every synthetic account",
	)
	customers := flag.Int("customers", defaults.Customers, "customers")
	units := flag.Int("units-per-customer", defaults.UnitsPerCustomer, "units per customer")
	contractors := flag.Int("contractors", defaults.Contractors, "contractor companies")
	customerUsers := flag.Int("customer-users", defaults.CustomerUsers, "customer accounts")
	partnerUsers := flag.Int("partner-users", defaults.PartnerUsers, "partner accounts")
	technicianUsers := flag.Int("technician-users", defaults.TechnicianUsers, "technician accounts")
	hqUsers := flag.Int("hq-users", defaults.HQUsers, "HQ accounts")

	flag.Parse()

	cfg := loadseed.Config{
		Customers:        *customers,
		UnitsPerCustomer: *units,
		Contractors:      *contractors,
		CustomerUsers:    *customerUsers,
		PartnerUsers:     *partnerUsers,
		TechnicianUsers:  *technicianUsers,
		HQUsers:          *hqUsers,
		Password:         *password,
		Now:              time.Now().UTC().Truncate(time.Second),
	}

	err := run(context.Background(), *url, *manifest, *manifestOnly, cfg)
	if err != nil {
		log.Fatal(err)
	}
}

// run guards the database name, applies the data in one transaction and writes the manifest.
func run(
	ctx context.Context,
	url, manifestPath string,
	manifestOnly bool,
	cfg loadseed.Config,
) error {
	result, err := loadseed.Build(cfg)
	if err != nil {
		return apperr.Wrap(err, "loadseed.Build")
	}

	if manifestOnly {
		return writeManifest(manifestPath, result.Manifest)
	}

	err = loadseed.Guard(url)
	if err != nil {
		return apperr.Wrap(err, "loadseed.Guard")
	}

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return apperr.Wrap(err, "pgx.Connect")
	}

	defer func() { _ = conn.Close(ctx) }()

	var current string

	err = conn.QueryRow(ctx, `SELECT current_database()`).Scan(&current)
	if err != nil {
		return apperr.Wrap(err, "current_database")
	}

	if current != loadseed.Database { // the URL can name one database and the server answer another
		return fmt.Errorf("connected to %q: %w", current, loadseed.ErrWrongDatabase)
	}

	err = pgx.BeginFunc(
		ctx,
		conn,
		func(txn pgx.Tx) error { return seed.Apply(ctx, txn, result.Fixture) },
	)
	if err != nil {
		return apperr.Wrap(err, "seed.Apply")
	}

	return writeManifest(manifestPath, result.Manifest)
}

// writeManifest writes the account manifest. The same configuration always makes the same one, so the manifest the
// realm was made from before the stack started is the one written again after the data is in.
func writeManifest(path string, manifest loadseed.Manifest) error {
	data, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return apperr.Wrap(err, "json.MarshalIndent")
	}

	err = os.WriteFile(path, data, manifestMode)
	if err != nil {
		return apperr.Wrap(err, "os.WriteFile")
	}

	log.Printf(
		"loadseed: %d customers, %d units, %d jobs, %d accounts",
		manifest.Customers,
		manifest.Units,
		manifest.Jobs,
		len(manifest.Users),
	)

	return nil
}
