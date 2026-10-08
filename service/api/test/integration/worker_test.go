package integration

import (
	"context"
	"testing"
	"time"

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
	if _, err := scheduler.Tick(context.Background(), s.DB, clock); err != nil {
		t.Fatal(err)
	}
	if _, m := post(s, &hq, "jobs.get", `{"jobId":"`+job+`"}`); data(m)["status"] != "offered" {
		t.Fatal("offer still open before its deadline")
	}
	r, err := scheduler.Tick(context.Background(), s.DB, clock.Add(time.Hour))
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
	if r, err := scheduler.Tick(context.Background(), s.DB, clock.Add(time.Hour)); err != nil || r.ExpiredOffers != 0 {
		t.Fatalf("idempotent tick: %+v %v", r, err)
	}
}

func TestWorkerRunStopsOnCancel(t *testing.T) {
	s := server(t)
	ctx, cancel := context.WithCancel(context.Background())
	var logs int
	done := make(chan struct{})
	go func() {
		scheduler.Run(ctx, s.DB, 10*time.Millisecond, func() time.Time { return clock.Add(1000 * time.Hour) }, func(string, ...any) { logs++ })
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
