// Command mockhistory gives the mock fleet (internal/devicesim/fleet.json) days of past readings ending just before the
// Core API's scenario clock, so the charts show a long series that joins the simulator's live readings. Development
// only: it runs once after the stack is up (compose profile iot) with the owner login, and keeps rows that exist.
package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/iotbridge"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

func main() {
	days := flag.Int("days", envInt("MOCK_HISTORY_DAYS", 30), "days of history (1–35)")
	api := flag.String("api", envOr("CORE_API_URL", "http://localhost:8080"), "Core API base URL, for its scenario clock")
	flag.Parse()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL is required (the owner login)")
	}
	ctx := context.Background()
	fleet, err := devicesim.MockFleet()
	if err != nil {
		log.Fatal(err)
	}
	now, err := iotbridge.ScenarioNow(&http.Client{Timeout: 10 * time.Second}, *api)
	if err != nil {
		log.Fatalf("scenario clock: %v", err)
	}
	until := now.Add(-2 * time.Minute) // the live readings take over from here
	conn, err := pgx.Connect(ctx, url)
	if err != nil {
		log.Fatal(err)
	}
	defer conn.Close(ctx)
	var rows int64
	started := time.Now()
	if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error {
		var err error
		rows, err = seed.ApplyFleetHistory(ctx, tx, fleet, until, *days)
		return err
	}); err != nil {
		log.Fatal(err)
	}
	log.Printf("mockhistory: %d days up to %s, %d new rows in %s", *days, until.Format(time.RFC3339), rows, time.Since(started).Round(time.Second))
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) int {
	if v, err := strconv.Atoi(os.Getenv(key)); err == nil {
		return v
	}
	return def
}
