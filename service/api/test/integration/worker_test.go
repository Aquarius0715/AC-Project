package integration_test

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

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestWorkerTickExpiresOffers(t *testing.T) {
	srv := server(t)
	unit := seed.ID("unit-non-rto").String()
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	_, res := write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)
	job := asStr(t, data(res)["id"])
	write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","visitSlot":`+slotJSON(
			49,
			2,
		)+
			`,"offerExpiresAt":"`+stamp(
			1,
		)+`","accessValidFrom":"`+stamp(
			1,
		)+`","accessValidUntil":"`+stamp(
			100,
		)+`","termsVersion":"t"}`,
		1,
	)
	// before the deadline nothing changes
	_, err := schedTick(context.Background(), srv, clock())
	if err != nil {
		t.Fatal(err)
	}

	if _, m := post(
		srv,
		hq(),
		"jobs.get",
		`{"jobId":"`+job+`"}`,
	); data(m)["status"] != statusOffered {
		t.Fatal("offer still open before its deadline")
	}

	r, err := schedTick(context.Background(), srv, clock().Add(time.Hour))
	if err != nil || r.ExpiredOffers < 1 {
		t.Fatalf("tick: %+v %v", r, err)
	}

	_, res = post(srv, hq(), "jobs.get", `{"jobId":"`+job+`"}`)
	if data(res)["status"] != statusRequested || data(res)["contractorOrgId"] != nil ||
		asNum(t, data(res)["version"]) != 3 {
		t.Fatalf("expired: %v", res)
	}

	_, res = post(
		srv,
		hq(),
		"jobs.events",
		`{"jobId":"`+job+`","query":{"sort":{"field":"occurredAt","direction":"desc"}}}`,
	)
	if items(t, res)[0]["action"] != "offer_expired" ||
		items(t, res)[0]["actorUserId"] != seed.ID("system-demo").String() {
		t.Fatalf("expiry event: %v", items(t, res)[0])
	}
	// HQ may offer again (same contractor allowed)
	if code, _ := write(
		srv,
		hq(),
		"jobs.offer",
		`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
			String()+
			`","visitSlot":`+slotJSON(
			49,
			2,
		)+
			`,"offerExpiresAt":"`+stamp(
			12,
		)+`","accessValidFrom":"`+stamp(
			1,
		)+`","accessValidUntil":"`+stamp(
			100,
		)+`","termsVersion":"t"}`,
		3,
	); code != 200 {
		t.Error("re-offer after expiry")
	}

	schedTickR, schedTickErr := schedTick(
		context.Background(),
		srv,
		clock().Add(time.Hour),
	)
	if schedTickErr != nil ||
		schedTickR.ExpiredOffers != 0 {
		t.Fatalf("idempotent tick: %+v %v", schedTickR, schedTickErr)
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestWorkerRunStopsOnCancel(t *testing.T) {
	srv := server(t)
	ctx, cancel := context.WithCancel(context.Background())

	var logs int

	done := make(chan struct{})

	go func() {
		scheduler.RunDomains(
			ctx,
			srv.DB,
			10*time.Millisecond,
			func() time.Time { return clock().Add(1000 * time.Hour) },
			func(string, ...any) { logs++ },
			nil,
			nil,
		)
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
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestSchedulerInstancesShareTheWork(t *testing.T) {
	srv := server(t)
	ctx := context.Background()

	ms := srv // the maintenance scheduler's database role and registry: its own service in cluster mode
	if cl, ok := clusters[srv]; ok {
		ms = cl.srv[ops.DomainMaintenance]
	}

	maint := []string{ops.DomainMaintenance}
	unit, tenant := seed.ID("unit-non-rto").String(), seed.ID("tenant-a").String()
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	offered := func() string {
		_, m := write(srv, customerA(), "jobs.create", jobBody(unit, nil), 0)

		job := asStr(t, data(m)["id"])
		if code, res := write(
			srv,
			hq(),
			"jobs.offer",
			`{"jobId":"`+job+`","contractorOrgId":"`+seed.ID("org-contractor-a").
				String()+
				`","visitSlot":`+slotJSON(
				49,
				2,
			)+
				`,"offerExpiresAt":"`+stamp(
				1,
			)+`","accessValidFrom":"`+stamp(
				1,
			)+`","accessValidUntil":"`+stamp(
				100,
			)+`","termsVersion":"t"}`,
			1,
		); code != 200 {
			t.Fatalf("offer: %d %v", code, res)
		}

		return job
	}
	status := func(job string) any {
		_, m := post(srv, hq(), "jobs.get", `{"jobId":"`+job+`"}`)

		return data(m)["status"]
	}
	expiries := func(job string) int {
		_, m := post(srv, hq(), "jobs.events", `{"jobId":"`+job+`","query":{"limit":100}}`)
		n := 0

		for _, e := range items(t, m) {
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
	defer func() { _ = conn.Close(ctx) }()
	// another instance holds the maintenance lock of tenant-a
	job := offered()

	holder, err := conn.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	_, execErr := holder.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtext('scheduler:maintenance'), hashtext($1))`,
		tenant,
	)
	if execErr != nil {
		t.Fatal(execErr)
	}

	r, err := scheduler.TryTickDomains(ctx, ms.DB, clock().Add(time.Hour), maint, ms.Registry)
	if err != nil || r.Skipped < 1 || r.ExpiredOffers != 0 || status(job) != statusOffered {
		t.Fatalf("the background tick skips the held tenant: %+v %v %v", r, err, status(job))
	}

	done := make(chan scheduler.Result, 1)

	go func() {
		r, err := scheduler.TickDomains(ctx, ms.DB, clock().Add(time.Hour), maint, ms.Registry)
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

	commitErr := holder.Commit(ctx)
	if commitErr != nil {
		t.Fatal(commitErr)
	}

	if r := <-done; r.ExpiredOffers < 1 || status(job) != statusRequested || expiries(job) != 1 {
		t.Fatalf("the waiting tick after the lock: %+v %v %d", r, status(job), expiries(job))
	}
	// two background ticks at once: the offer expires once
	job2 := offered()

	var (
		group sync.WaitGroup
		mutex sync.Mutex
		total scheduler.Result
	)

	for range 2 {
		group.Go(func() {
			result, err := scheduler.TryTickDomains(
				ctx,
				ms.DB,
				clock().Add(time.Hour),
				maint,
				ms.Registry,
			)
			if err != nil {
				t.Error(err)
			}

			mutex.Lock()
			total.ExpiredOffers += result.ExpiredOffers
			total.Skipped += result.Skipped
			mutex.Unlock()
		})
	}

	group.Wait()

	if total.ExpiredOffers != 1 || status(job2) != statusRequested || expiries(job2) != 1 {
		t.Fatalf("two instances: %+v %v %d", total, status(job2), expiries(job2))
	}

	_, schedTickErr := schedTick(
		ctx,
		srv,
		clock().Add(time.Hour),
	)
	if schedTickErr != nil { // apply the ticks' events as the consumers would
		t.Fatal(schedTickErr)
	}
}
