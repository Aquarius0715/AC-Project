package integration

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/scheduler"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestWorkerTickExpiresOffers(t *testing.T) {
	s := server(t)
	unit := seed.ID("unit-non-rto").String()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, m := write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
	job := data(m)["id"].(string)
	write(s, &hq, "jobs.offer", `{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+
		`,"offerExpiresAt":"`+ts(1)+`","accessValidFrom":"`+ts(1)+`","accessValidUntil":"`+ts(100)+`","termsVersion":"t"}`, 1)
	// before the deadline nothing changes
	if _, err := schedTick(context.Background(), s, clock); err != nil {
		t.Fatal(err)
	}
	if _, m := post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`); data(m)["status"] != "offered" {
		t.Fatal("offer still open before its deadline")
	}
	r, err := schedTick(context.Background(), s, clock.Add(time.Hour))
	if err != nil || r.ExpiredOffers < 1 {
		t.Fatalf("tick: %+v %v", r, err)
	}
	_, m = post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`)
	if data(m)["status"] != "requested" || data(m)["contractorOrgId"] != nil || data(m)["version"].(float64) != 3 {
		t.Fatalf("expired: %v", m)
	}
	_, m = post(s, &hq, "jobs.events", `{"jobId":"`+job+`","query":{"sort":{"field":"occurredAt","direction":"desc"}}}`)
	if items(m)[0]["action"] != "offer_expired" || items(m)[0]["actorUserId"] != seed.ID("system-demo").String() {
		t.Fatalf("expiry event: %v", items(m)[0])
	}
	// HQ may offer again (same contractor allowed)
	if code, _ := write(s, &hq, "jobs.offer", `{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+
		`,"offerExpiresAt":"`+ts(12)+`","accessValidFrom":"`+ts(1)+`","accessValidUntil":"`+ts(100)+`","termsVersion":"t"}`, 3); code != 200 {
		t.Error("re-offer after expiry")
	}
	if r, err := schedTick(context.Background(), s, clock.Add(time.Hour)); err != nil || r.ExpiredOffers != 0 {
		t.Fatalf("idempotent tick: %+v %v", r, err)
	}
}

func TestWorkerRunStopsOnCancel(t *testing.T) {
	s := server(t)
	ctx, cancel := context.WithCancel(context.Background())
	var logs int
	done := make(chan struct{})
	go func() {
		scheduler.Run(ctx, s.DB, 10*time.Millisecond, func() time.Time { return clock.Add(1000 * time.Hour) }, func(string, ...any) { logs++ }, nil)
		close(done)
	}()
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("worker did not stop")
	}
}

// IR356: several instances of a domain's scheduler. The background tick skips a domain and tenant whose lock another
// instance holds, and its next tick catches up; the waiting tick (demo clock jumps, tests) waits for the lock; two
// background ticks at once apply a transition once.
func TestSchedulerInstancesShareTheWork(t *testing.T) {
	s := server(t)
	ctx := context.Background()
	ms := s // the maintenance scheduler's database role and registry: its own service in cluster mode
	if cl, ok := clusters[s]; ok {
		ms = cl.srv[ops.DomainMaintenance]
	}
	maint := []string{ops.DomainMaintenance}
	unit, tenant := seed.ID("unit-non-rto").String(), seed.ID("tenant-a").String()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	offered := func() string {
		_, m := write(s, &customerA, "jobs.create", jobBody(unit, nil), 0)
		job := data(m)["id"].(string)
		if code, m := write(s, &hq, "jobs.offer", `{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").String()+`","visitSlot":`+slotJSON(49, 2)+
			`,"offerExpiresAt":"`+ts(1)+`","accessValidFrom":"`+ts(1)+`","accessValidUntil":"`+ts(100)+`","termsVersion":"t"}`, 1); code != 200 {
			t.Fatalf("offer: %d %v", code, m)
		}
		return job
	}
	status := func(job string) any {
		_, m := post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`)
		return data(m)["status"]
	}
	expiries := func(job string) int {
		_, m := post(s, &hq, "jobs.events", `{"jobId":"`+job+`","query":{"limit":100}}`)
		n := 0
		for _, e := range items(m) {
			if e["action"] == "offer_expired" {
				n++
			}
		}
		return n
	}
	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer conn.Close(ctx)
	// another instance holds the maintenance lock of tenant-a
	job := offered()
	holder, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := holder.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('scheduler:maintenance'), hashtext($1))`, tenant); err != nil {
		t.Fatal(err)
	}
	r, err := scheduler.TryTickDomains(ctx, ms.DB, clock.Add(time.Hour), maint, ms.Registry)
	if err != nil || r.Skipped < 1 || r.ExpiredOffers != 0 || status(job) != "offered" {
		t.Fatalf("the background tick skips the held tenant: %+v %v %v", r, err, status(job))
	}
	done := make(chan scheduler.Result, 1)
	go func() {
		r, err := scheduler.TickDomains(ctx, ms.DB, clock.Add(time.Hour), maint, ms.Registry)
		if err != nil {
			t.Error(err)
		}
		done <- r
	}()
	select {
	case r := <-done:
		t.Fatalf("the waiting tick ran under another instance's lock: %+v", r)
	case <-time.After(300 * time.Millisecond):
	}
	if err := holder.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	if r := <-done; r.ExpiredOffers < 1 || status(job) != "requested" || expiries(job) != 1 {
		t.Fatalf("the waiting tick after the lock: %+v %v %d", r, status(job), expiries(job))
	}
	// two background ticks at once: the offer expires once
	job2 := offered()
	var wg sync.WaitGroup
	var mu sync.Mutex
	var total scheduler.Result
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := scheduler.TryTickDomains(ctx, ms.DB, clock.Add(time.Hour), maint, ms.Registry)
			if err != nil {
				t.Error(err)
			}
			mu.Lock()
			total.ExpiredOffers += r.ExpiredOffers
			total.Skipped += r.Skipped
			mu.Unlock()
		}()
	}
	wg.Wait()
	if total.ExpiredOffers != 1 || status(job2) != "requested" || expiries(job2) != 1 {
		t.Fatalf("two instances: %+v %v %d", total, status(job2), expiries(job2))
	}
	if _, err := schedTick(ctx, s, clock.Add(time.Hour)); err != nil { // apply the ticks' events as the consumers would
		t.Fatal(err)
	}
}
