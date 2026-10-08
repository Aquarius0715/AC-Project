// Command worker runs background roles; --role=scheduler ticks the clock-driven transitions (IR48 offer expiry,
// IR124 history freezing) every second.
package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/pradita/ac-project/service/internal/platform/db"
	"github.com/pradita/ac-project/service/internal/platform/democlock"
	"github.com/pradita/ac-project/service/internal/scheduler"
)

func main() {
	role := flag.String("role", "scheduler", "worker role (compose.yaml): scheduler runs the clock-driven transitions")
	flag.Parse()
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	url := os.Getenv("DATABASE_URL")
	if url == "" {
		log.Fatal("DATABASE_URL is required")
	}
	m, err := db.Open(ctx, url, "")
	if err != nil {
		log.Fatal(err)
	}
	defer m.Close()
	if *role != "scheduler" {
		log.Printf("worker role %q has no jobs yet; idling", *role)
		<-ctx.Done()
		return
	}
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
