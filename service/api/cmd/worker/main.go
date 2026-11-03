// Command worker runs background roles. --role=notification sends the pending e-mails through SMTP every few seconds
// (IR418; Mailpit in development). --role=scheduler ticks the clock-driven transitions every second, with the
// modules' jobs (IR54 schedule automations): with --domain=<domain> only that domain's (IR195; the domain's database
// role, other domains asked through their internal queries), without it every domain on an all-domain role
// (single-process development). --role=outbox keeps the platform tables to their retention every hour (IR428).
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"slices"
	"strconv"
	"syscall"
	"time"

	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/auth"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
	"github.com/pradita/ac-project/service/api/internal/platform/logging"
	"github.com/pradita/ac-project/service/api/internal/scheduler"
	"github.com/pradita/ac-project/service/api/internal/server"
)

func main() {
	role := flag.String(
		"role",
		"scheduler",
		"worker role (compose.yaml): scheduler runs the clock-driven transitions",
	)
	domain := flag.String(
		"domain",
		"",
		"scheduler of one business domain (equipment, maintenance, billing); empty runs every domain",
	)

	flag.Parse()

	err := run(*role, *domain)
	if err != nil {
		log.Fatal(err)
	}
}

var (
	// errNoDatabaseURL refuses to start without the role's database.
	errNoDatabaseURL = errors.New("DATABASE_URL is required")
	// errUnknownDomain refuses a scheduler domain that has no jobs.
	errUnknownDomain = errors.New("no scheduler for domain")
)

// run runs the role until a signal ends it; main exits on its error once its deferred clean-up has run.
func run(role, domain string) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	if os.Getenv("DATABASE_URL") == "" {
		return errNoDatabaseURL
	}

	switch role {
	case "notification":
		return runMail(ctx)
	case "outbox":
		return runHousekeeping(ctx)
	case "scheduler":
		return runScheduler(ctx, domain)
	}

	log.Printf("worker role %q has no jobs yet; idling", role)
	<-ctx.Done()

	return nil
}

// runScheduler ticks the clock-driven transitions every second: one domain's, or with no domain every domain's on an
// all-domain role (single-process development).
func runScheduler(ctx context.Context, domain string) error {
	var domains []string // nil: every domain on an all-domain role (single-process development)

	if domain != "" {
		if !slices.Contains(scheduler.Domains, domain) {
			return fmt.Errorf("%w %q (want one of %v)", errUnknownDomain, domain, scheduler.Domains)
		}

		domains = []string{domain}
	}

	name := "scheduler"
	if domain != "" {
		name = domain + "-scheduler"
	}

	logger := logging.New(os.Stdout, name)

	cfg, err := server.ConfigFromEnv(logger, domains)
	if err != nil {
		return apperr.Wrap(err, "server.ConfigFromEnv")
	}
	// the domains' wiring without HTTP: the role, the registry (internal queries, the modules' jobs such as the IR54
	// schedule automations) and the shared scenario clock
	srv, err := server.New(ctx, cfg, auth.StaticVerifier{})
	if err != nil {
		return apperr.Wrap(err, "server.New")
	}

	defer srv.DB.Close()

	scheduler.RunDomains(
		ctx,
		srv.DB,
		time.Second,
		srv.Registry.Clock,
		log.Printf,
		domains,
		srv.Registry,
	)

	return nil
}

// runMail sends the pending e-mails of every tenant every five seconds (IR418). Without MAIL_SMTP_ADDR it idles:
// e-mails stay previews, as before.
func runMail(ctx context.Context) error {
	addr := os.Getenv("MAIL_SMTP_ADDR")
	if addr == "" {
		log.Print("notification worker: MAIL_SMTP_ADDR is not set; e-mails stay previews")
		<-ctx.Done()

		return nil
	}

	logger := logging.New(os.Stdout, "notification")

	cfg, err := server.ConfigFromEnv(logger, nil)
	if err != nil {
		return apperr.Wrap(err, "server.ConfigFromEnv")
	}

	srv, err := server.New(ctx, cfg, auth.StaticVerifier{})
	if err != nil {
		return apperr.Wrap(err, "server.New")
	}

	defer srv.DB.Close()

	sender := notify.SMTP{
		Addr: addr,
		From: envOr("MAIL_FROM", "AC Project <no-reply@ac-project.local>"),
	}
	apps := notify.Apps{
		Customer: envOr(
			"APP_URL_CUSTOMER",
			"http://localhost:3000",
		),
		Partner: envOr("APP_URL_PARTNER", "http://localhost:3001"),
		Technician: envOr(
			"APP_URL_TECHNICIAN",
			"http://localhost:3002",
		),
		Admin: envOr("APP_URL_ADMIN", "http://localhost:3003"),
	}

	log.Printf("notification worker: sending e-mails through %s", strconv.Quote(addr))

	deliverEvery(ctx, srv, sender, apps)

	return nil
}

// mailEvery is how often the notification worker looks for pending e-mails, and mailBatch how many it sends at most
// each time.
const (
	mailEvery = 5 * time.Second
	mailBatch = 50
)

// deliverEvery sends the pending e-mails of every tenant every mailEvery until the context ends.
func deliverEvery(ctx context.Context, srv *server.Server, sender notify.SMTP, apps notify.Apps) {
	ticker := time.NewTicker(mailEvery)
	defer ticker.Stop()

	for {
		sent, err := notify.DeliverAll(
			ctx,
			srv.DB,
			sender,
			apps,
			srv.Registry.Clock(),
			mailBatch,
		)
		if err != nil {
			log.Printf("notification worker: %v", err)
		} else if sent > 0 {
			log.Printf("notification worker: %d e-mail(s) sent", sent)
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}

	return def
}

// runHousekeeping deletes, every hour, the outbox events every subscriber has applied, the records of events no
// longer in the outbox and the expired idempotency keys (events.Housekeep), and the notifications and sent e-mails
// older than two years (notify.SweepAll, IR428). It runs on the all-domain role so
// it knows every consumer's subscriptions; the outbox relay to SNS joins this role later.
func runHousekeeping(ctx context.Context) error {
	logger := logging.New(os.Stdout, "outbox")

	cfg, err := server.ConfigFromEnv(logger, nil)
	if err != nil {
		return apperr.Wrap(err, "server.ConfigFromEnv")
	}

	srv, err := server.New(ctx, cfg, auth.StaticVerifier{})
	if err != nil {
		return apperr.Wrap(err, "server.New")
	}

	defer srv.DB.Close()

	ticker := time.NewTicker(time.Hour)
	defer ticker.Stop()

	for {
		now := srv.Registry.Clock()

		swept, err := events.Housekeep(ctx, srv.DB, now, srv.Consumers)
		if err != nil {
			log.Printf("housekeeping: %v", err)
		} else {
			log.Printf(
				"housekeeping: removed %d outbox events, %d processed-event records, %d failure records, %d idempotency keys",
				swept.Outbox,
				swept.Processed,
				swept.Failures,
				swept.IdempotencyKeys,
			)
		}

		removed, err := notify.SweepAll(
			ctx,
			srv.DB,
			now,
		)
		if err != nil { // notifications and sent e-mails older than 2 years
			log.Printf("housekeeping: notifications: %v", err)
		} else if removed > 0 {
			log.Printf(
				"housekeeping: removed %d notification and e-mail rows older than 2 years",
				removed,
			)
		}

		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
		}
	}
}
