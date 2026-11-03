// Command mockhistory gives the mock fleet (internal/devicesim/fleet.json) days of past readings ending just before the
// Core API's scenario clock, so the charts show a long series that joins the simulator's live readings. Development
// only: it runs once after the stack is up (compose profile iot) with the owner login, and keeps rows that exist.
package main

import (
	"context"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/iotbridge"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

func main() {
	days := flag.Int("days", envInt("MOCK_HISTORY_DAYS", defaultDays), "days of history (1–35)")
	api := flag.String(
		"api",
		envOr("CORE_API_URL", "http://localhost:8080"),
		"Core API base URL, for its scenario clock",
	)

	flag.Parse()

	err := run(context.Background(), *days, *api)
	if err != nil {
		log.Fatal(err)
	}
}

// defaultDays is how much history a run writes unless MOCK_HISTORY_DAYS or -days says otherwise.
const defaultDays = 30

// clientTimeout bounds the scenario clock's request to the Core API.
const clientTimeout = 10 * time.Second

// liveTakeover is how long before the scenario's now the history stops: the live readings take over from there.
const liveTakeover = 2 * time.Minute

// errNoDatabaseURL refuses a run without the owner login.
var errNoDatabaseURL = errors.New("DATABASE_URL is required (the owner login)")

// run writes the history; main exits on its error once its deferred clean-up has run.
func run(ctx context.Context, days int, api string) error {
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		return errNoDatabaseURL
	}

	fleet, err := devicesim.MockFleet()
	if err != nil {
		return apperr.Wrap(err, "devicesim.MockFleet")
	}

	now, err := iotbridge.ScenarioNow(
		ctx,
		&http.Client{Timeout: clientTimeout, Transport: nil, CheckRedirect: nil, Jar: nil},
		api,
	)
	if err != nil {
		return apperr.Wrap(err, "scenario clock")
	}

	until := now.Add(-liveTakeover)

	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		return apperr.Wrap(err, "pgx.Connect")
	}

	defer func() { _ = conn.Close(ctx) }()

	var rows int64

	started := time.Now()

	beginFuncErr := pgx.BeginFunc(ctx, conn, func(txn pgx.Tx) error {
		var err error

		rows, err = seed.ApplyFleetHistory(ctx, txn, fleet, until, days)

		return apperr.Wrap(err, "seed.ApplyFleetHistory")
	})
	if beginFuncErr != nil {
		return apperr.Wrap(beginFuncErr, "transaction")
	}

	log.Printf(
		"mockhistory: %d days up to %s, %d new rows in %s",
		days,
		until.Format(time.RFC3339),
		rows,
		time.Since(started).Round(time.Second),
	)

	return nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

func envInt(key string, def int) int {
	v, err := strconv.Atoi(os.Getenv(key))
	if err == nil {
		return v
	}

	return def
}
