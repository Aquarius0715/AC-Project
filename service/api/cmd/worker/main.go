// Command worker runs background roles. --role=scheduler ticks the clock-driven transitions every second: with
// --domain=<domain> only that domain's (IR195; the domain's database role, other domains asked through their
// internal queries), without it every domain on an all-domain role (single-process development).
package main

import (
	"context"
	"flag"
	"log"
	"log/slog"
	"os"
	"os/signal"
	"slices"
	"syscall"
	"time"

	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/platform/db"
	"github.com/pradita/ac-project/service/api/internal/platform/democlock"
	"github.com/pradita/ac-project/service/api/internal/scheduler"
	"github.com/pradita/ac-project/service/api/internal/server"
)

func main() {
	role := flag.String("role", "scheduler", "worker role (compose.yaml): scheduler runs the clock-driven transitions")
	domain := flag.String("domain", "", "scheduler of one business domain (equipment, maintenance); empty runs every domain")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	if os.Getenv("DATABASE_URL") == "" {
		log.Fatal("DATABASE_URL is required")
	}
	if *role != "scheduler" {
		log.Printf("worker role %q has no jobs yet; idling", *role)
		<-ctx.Done()
		return
	}
	if *domain != "" {
		if !slices.Contains(scheduler.Domains, *domain) {
			log.Fatalf("no scheduler for domain %q (want one of %v)", *domain, scheduler.Domains)
		}
		logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", *domain+"-scheduler")
		cfg, err := server.ConfigFromEnv(logger, []string{*domain})
		if err != nil {
			log.Fatal(err)
		}
		// the domain's wiring without HTTP: its role, registry (internal queries) and the shared scenario clock
		srv, err := server.New(ctx, cfg, auth.StaticVerifier{})
		if err != nil {
			log.Fatal(err)
		}
		defer srv.DB.Close()
		scheduler.RunDomains(ctx, srv.DB, time.Second, srv.Registry.Clock, log.Printf, []string{*domain}, srv.Registry)
		return
	}
	m, err := db.Open(ctx, os.Getenv("DATABASE_URL"), "")
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()
	clock := func() time.Time { return time.Now().UTC() }
	if os.Getenv("DEMO_OPS") == "1" { // demo environment: the scenario clock shared with the API (IR168)
		start, err := democlock.StartFromEnv(os.Getenv)
		if err != nil {
			log.Fatal(err)
		}
		dc, err := democlock.Open(ctx, m.Writer, clock, start)
		if err != nil {
			log.Fatal(err)
		}
		clock = dc.Now
	}
	scheduler.Run(ctx, m, time.Second, clock, log.Printf)
}
