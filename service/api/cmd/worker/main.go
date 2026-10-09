// Command worker runs background roles. --role=scheduler ticks the clock-driven transitions every second, with the
// modules' jobs (IR54 schedule automations): with --domain=<domain> only that domain's (IR195; the domain's database
// role, other domains asked through their internal queries), without it every domain on an all-domain role
// (single-process development).
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
	var domains []string // nil: every domain on an all-domain role (single-process development)
	if *domain != "" {
		if !slices.Contains(scheduler.Domains, *domain) {
			log.Fatalf("no scheduler for domain %q (want one of %v)", *domain, scheduler.Domains)
		}
		domains = []string{*domain}
	}
	name := "scheduler"
	if *domain != "" {
		name = *domain + "-scheduler"
	}
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil)).With("service", name)
	cfg, err := server.ConfigFromEnv(logger, domains)
	if err != nil {
		log.Fatal(err)
	}
	// the domains' wiring without HTTP: the role, the registry (internal queries, the modules' jobs such as the IR54
	// schedule automations) and the shared scenario clock
	srv, err := server.New(ctx, cfg, auth.StaticVerifier{})
	if err != nil {
		log.Fatal(err)
	}
	defer srv.DB.Close()
	scheduler.RunDomains(ctx, srv.DB, time.Second, srv.Registry.Clock, log.Printf, domains, srv.Registry)
}
