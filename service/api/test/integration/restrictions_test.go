package integration

import (
	"context"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/control"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// boundUnit creates a customer-b unit with a bound device in the given connection state.
func boundUnit(t *testing.T, s *apiserver.Server, connection string) string {
	t.Helper()
	u := newUnit(t, s, "Restr AC")
	dev, binding := uuid.NewString(), uuid.NewString()
	owner(t, `INSERT INTO devices.devices (id, tenant_id, serial, binding_id, unit_id, target_unit_id, created_by_membership_id, connection, last_seen_at, firmware_version, power_signal)
		VALUES ($1,$2,$3,$4,$5,$5,$6,$7,$8,'1.0','on')`, dev, seed.ID("tenant-a"), "SN-"+dev[:8], binding, u, seed.ID("hq-operator"), connection, clock)
	owner(t, `INSERT INTO devices.device_bindings (id, tenant_id, device_id, unit_id, customer_org_id, bound_at, reason, actor_membership_id)
		VALUES ($1,$2,$3,$4,$5,$6,'test',$7)`, binding, seed.ID("tenant-a"), dev, u, seed.ID("org-customer-b"), clock.Add(-time.Hour), seed.ID("hq-operator"))
	owner(t, `UPDATE assets.units SET connection = $2 WHERE id = $1`, u, connection)
	return u
}

// overdueContract creates an RTO contract on the units with one overdue unpaid invoice.
func overdueContract(t *testing.T, s *apiserver.Server, units ...string) (string, string) {
	t.Helper()
	ts := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	code, m := write(s, &hq, "contracts.save", `{"customerId":"`+seed.ID("cust-b").String()+`","unitIds":["`+strings.Join(units, `","`)+`"],"planType":"rto","startAt":"`+ts(-24*30)+
		`","endAt":"`+ts(24*365)+`","priceMinor":5000,"currency":"MYR","restrictionEligible":true,"rulesVersion":"rules-1"}`, 0)
	if code != 200 {
		t.Fatalf("contract: %d %v", code, m)
	}
	k := data(m)["id"].(string)
	code, m = write(s, &hq, "invoices.create", `{"amountMinor":5000,"currency":"MYR","contractId":"`+k+`","contractVersion":1,"period":{"from":"`+ts(-24*14)+`","to":"`+ts(24*16)+`"},"dueAt":"`+ts(48)+`"}`, 0)
	if code != 200 {
		t.Fatalf("invoice: %d %v", code, m)
	}
	inv := data(m)["id"].(string)
	owner(t, `UPDATE billing.invoices SET due_at = $2 WHERE id = $1`, inv, clock.Add(-time.Hour))
	return k, inv
}

func scheduleBody(k, inv string, units []string, extra map[string]string) string {
	f := map[string]string{"contractId": `"` + k + `"`, "expectedContractVersion": "1", "causeInvoiceIds": `["` + inv + `"]`, "unitIds": `["` + strings.Join(units, `","`) + `"]`,
		"policy": `{"kind":"temperature_limit","minimumCoolingSetpoint":24}`, "executeAfter": `"` + clock.Add(25*time.Hour).Format(time.RFC3339) + `"`,
		"reason": `"Invoice overdue"`, "rulesVersion": `"rules-1"`}
	for k, v := range extra {
		f[k] = v
	}
	parts := []string{}
	for k, v := range f {
		parts = append(parts, `"`+k+`":`+v)
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// due moves a scheduled restriction's notice into the past so it can be executed.
func due(t *testing.T, id string) {
	owner(t, `UPDATE restrictions.restrictions SET notice_at = $2, execute_after = $3 WHERE id = $1`, id, clock.Add(-26*time.Hour), clock.Add(-time.Hour))
	owner(t, `UPDATE notify.notifications SET occurred_at = $2 WHERE target->>'id' = $1`, id, clock.Add(-26*time.Hour))
}

func unitState(m map[string]any, unit string) map[string]any {
	for _, u := range data(m)["perUnit"].([]any) {
		if x := u.(map[string]any); x["unitId"] == unit {
			return x
		}
	}
	return nil
}

func expireAt(t *testing.T, now time.Time) {
	t.Helper()
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, "postgres://postgres:local@localhost:5432/ac_test?sslmode=disable")
	if err != nil {
		t.Skip(err)
	}
	defer conn.Close(ctx)
	if err := pgx.BeginFunc(ctx, conn, func(tx pgx.Tx) error { _, err := control.ExpireCommands(ctx, tx, now); return err }); err != nil {
		t.Fatal(err)
	}
}

func TestRestrictionLifecycle(t *testing.T) {
	s := server(t)
	online, offline := boundUnit(t, s, "online"), boundUnit(t, s, "offline")
	units := []string{online, offline}
	k, inv := overdueContract(t, s, online, offline)
	for name, tc := range map[string]struct {
		e    map[string]string
		code int
	}{
		"notice under 24h":   {map[string]string{"executeAfter": `"` + clock.Add(23*time.Hour).Format(time.RFC3339) + `"`}, 422},
		"missing cause":      {map[string]string{"causeInvoiceIds": `["` + uuid.NewString() + `"]`}, 422},
		"rules mismatch":     {map[string]string{"rulesVersion": `"rules-2"`}, 422},
		"setpoint too low":   {map[string]string{"policy": `{"kind":"temperature_limit","minimumCoolingSetpoint":15}`}, 422},
		"setpoint off step":  {map[string]string{"policy": `{"kind":"temperature_limit","minimumCoolingSetpoint":24.5}`}, 422},
		"power off setpoint": {map[string]string{"policy": `{"kind":"power_off","minimumCoolingSetpoint":24}`}, 422},
		"unit not in deal":   {map[string]string{"unitIds": `["` + seed.ID("unit-online-rto").String() + `"]`}, 422},
		"duplicate unit":     {map[string]string{"unitIds": `["` + online + `","` + online + `"]`}, 422},
		"blank reason":       {map[string]string{"reason": `"  "`}, 422},
		"stale contract":     {map[string]string{"expectedContractVersion": "2"}, 409},
		"unknown contract":   {map[string]string{"contractId": `"` + uuid.NewString() + `"`}, 404},
	} {
		if code, m := write(s, &restrMgr, "restrictions.schedule", scheduleBody(k, inv, units, tc.e), 0); code != tc.code {
			t.Errorf("schedule %s: %d want %d %v", name, code, tc.code, m)
		}
	}
	if code, _ := write(s, &hq, "restrictions.schedule", scheduleBody(k, inv, units, nil), 0); code != 403 {
		t.Error("schedule without restriction.write")
	}
	code, m := write(s, &restrMgr, "restrictions.schedule", scheduleBody(k, inv, units, nil), 0)
	if code != 200 || data(m)["state"] != "scheduled" || len(data(m)["noticeNotificationIds"].([]any)) == 0 || data(m)["noticeAt"] != clock.Format(time.RFC3339) {
		t.Fatalf("schedule: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	if code, _ := write(s, &restrMgr, "restrictions.schedule", scheduleBody(k, inv, []string{online}, nil), 0); code != 409 {
		t.Error("second active restriction on a unit")
	}
	if _, m := post(s, &customerB, "notifications.list", `{"limit":100}`); m == nil {
		t.Log("notifications.list not implemented yet")
	}
	exec := func(rules string, v int) (int, map[string]any) {
		return write(s, &restrMgr, "restrictions.execute", `{"restrictionId":"`+id+`","confirmedRulesVersion":"`+rules+`"}`, v)
	}
	if code, _ := exec("rules-1", 1); code != 409 {
		t.Error("execute before executeAfter")
	}
	due(t, id)
	if code, _ := exec("rules-2", 1); code != 422 {
		t.Error("execute with other rules version")
	}
	if code, _ := exec("rules-1", 5); code != 409 {
		t.Error("execute with stale version")
	}
	code, m = exec("rules-1", 1)
	if code != 200 || data(m)["state"] != "requested" || data(m)["version"].(float64) != 2 {
		t.Fatalf("execute: %d %v", code, m)
	}
	on, off := unitState(m, online), unitState(m, offline)
	if on["applyState"] != "sent_unknown" || off["applyState"] != "not_sent" || off["pendingReason"] != "offline" || len(off["applyCommandIds"].([]any)) != 1 {
		t.Fatalf("per unit: %v %v", on, off)
	}
	var status, delivery string
	ownerScan(t, `SELECT status, delivery FROM control.commands WHERE id = $1`, []any{off["applyCommandIds"].([]any)[0]}, &status, &delivery)
	if status != "requested" || delivery != "not_sent" {
		t.Errorf("offline intent %s/%s", status, delivery)
	}
	if code, _ := exec("rules-1", 2); code != 409 {
		t.Error("execute twice")
	}
	if !ackCommand(t, on["applyCommandIds"].([]any)[0].(string), clock.Add(time.Second)) {
		t.Fatal("ack apply")
	}
	_, m = post(s, &restrMgr, "restrictions.get", `{"id":"`+id+`"}`)
	if data(m)["state"] != "requested" || unitState(m, online)["applyState"] != "applied" || unitState(m, online)["observedRestriction"] == nil {
		t.Fatalf("after ack: %v", m)
	}
	var obs *string
	ownerScan(t, `SELECT observed_restriction->>'restrictionId' FROM assets.units WHERE id = $1`, []any{online}, &obs)
	if obs == nil || *obs != id {
		t.Error("unit observed restriction")
	}

	// projections
	if code, m := post(s, &overrider, "restrictions.get", `{"id":"`+id+`"}`); code != 200 || data(m)["projection"] != "release" || data(m)["contractId"] != nil {
		t.Fatalf("release projection: %d %v", code, m)
	}
	if code, _ := post(s, &overrider, "restrictions.list", `{"filters":{"contractId":"`+k+`"}}`); code != 403 {
		t.Error("override-only contract filter")
	}
	if _, m := post(s, &overrider, "restrictions.list", `{"filters":{"state":"requested"},"limit":100}`); len(items(m)) == 0 || items(m)[0]["projection"] != "release" {
		t.Error("override-only list")
	}
	if _, m := post(s, &restrMgr, "restrictions.list", `{"filters":{"contractId":"`+k+`","invoiceId":"`+inv+`"}}`); len(items(m)) != 1 || len(items(m)[0]["events"].([]any)) < 2 {
		t.Fatalf("manager list: %v", m)
	}
	if code, _ := post(s, &restrMgr, "restrictions.list", `{"filters":{"state":"paused"}}`); code != 422 {
		t.Error("bad state filter")
	}
	if code, _ := post(s, &customerB, "restrictions.get", `{"id":"`+id+`"}`); code != 403 {
		t.Error("client get")
	}
	_, m = post(s, &customerB, "restrictions.forInvoice", `{"invoiceId":"`+inv+`","query":{}}`)
	if len(items(m)) != 1 {
		t.Fatalf("client forInvoice: %v", m)
	}
	for _, e := range items(m)[0]["events"].([]any) {
		if ev := e.(map[string]any); ev["actorId"] != "masked" || ev["reason"] != nil {
			t.Errorf("unmasked event %v", ev)
		}
	}
	if code, _ := post(s, &customerA, "restrictions.forInvoice", `{"invoiceId":"`+inv+`","query":{}}`); code != 404 {
		t.Error("other customer's invoice")
	}
	if code, _ := post(s, &restrMgr, "restrictions.get", `{"id":"`+uuid.NewString()+`"}`); code != 404 {
		t.Error("unknown restriction")
	}

	// cancel requested → release_requested with D03 evaluation
	cancel := func(v int) (int, map[string]any) {
		return write(s, &restrMgr, "restrictions.cancel", `{"restrictionId":"`+id+`","reason":"customer agreed plan"}`, v)
	}
	code, m = cancel(3)
	if code != 200 || data(m)["state"] != "release_requested" || data(m)["releaseIntent"].(map[string]any)["source"] != "cancel" {
		t.Fatalf("cancel: %d %v", code, m)
	}
	on, off = unitState(m, online), unitState(m, offline)
	if on["releaseState"] != "requested" || len(on["releaseCommandIds"].([]any)) != 1 || off["applyState"] != "not_applied" || off["releaseState"] != "not_required" {
		t.Fatalf("release eval: %v %v", on, off)
	}
	ownerScan(t, `SELECT status FROM control.commands WHERE id = $1`, []any{off["applyCommandIds"].([]any)[0]}, &status)
	if status != "cancelled" {
		t.Errorf("undelivered apply %s", status)
	}
	v := ver(m)
	if code, m := cancel(v); code != 200 || ver(m) != v {
		t.Error("cancel idempotent while release_requested")
	}
	if !ackCommand(t, on["releaseCommandIds"].([]any)[0].(string), clock.Add(2*time.Second)) {
		t.Fatal("ack remove")
	}
	_, m = post(s, &restrMgr, "restrictions.get", `{"id":"`+id+`"}`)
	if data(m)["state"] != "released" || unitState(m, online)["releaseState"] != "released" {
		t.Fatalf("released: %v", m)
	}
	if code, _ := cancel(ver(m)); code != 409 {
		t.Error("cancel released")
	}
}

func TestRestrictionCancelAndPayment(t *testing.T) {
	s := server(t)
	u := boundUnit(t, s, "online")
	k, inv := overdueContract(t, s, u)
	// a scheduled restriction is cancelled without commands
	_, m := write(s, &restrMgr, "restrictions.schedule", scheduleBody(k, inv, []string{u}, map[string]string{"policy": `{"kind":"power_off"}`}), 0)
	id := data(m)["id"].(string)
	if code, m := write(s, &restrMgr, "restrictions.cancel", `{"restrictionId":"`+id+`","reason":"mistake"}`, 1); code != 200 || data(m)["state"] != "cancelled" {
		t.Fatalf("cancel scheduled: %d %v", code, m)
	}
	// all causes paid right before execution: cancelled with no apply request
	_, m = write(s, &restrMgr, "restrictions.schedule", scheduleBody(k, inv, []string{u}, nil), 0)
	id = data(m)["id"].(string)
	due(t, id)
	owner(t, `UPDATE billing.invoices SET status = 'paid', paid_at = $2 WHERE id = $1`, inv, clock)
	if code, m := write(s, &restrMgr, "restrictions.execute", `{"restrictionId":"`+id+`","confirmedRulesVersion":"rules-1"}`, 1); code != 200 || data(m)["state"] != "cancelled" ||
		len(unitState(m, u)["applyCommandIds"].([]any)) != 0 {
		t.Fatalf("execute after payment: %d %v", code, m)
	}
	owner(t, `UPDATE billing.invoices SET status = 'unpaid', paid_at = NULL WHERE id = $1`, inv)
	// exemption blocks execution; no recipients blocks schedule
	_, m = write(s, &restrMgr, "restrictions.schedule", scheduleBody(k, inv, []string{u}, nil), 0)
	id = data(m)["id"].(string)
	due(t, id)
	owner(t, `UPDATE restrictions.restrictions SET grace_until = $2 WHERE id = $1`, id, clock.Add(time.Hour))
	if code, _ := write(s, &restrMgr, "restrictions.execute", `{"restrictionId":"`+id+`","confirmedRulesVersion":"rules-1"}`, 1); code != 409 {
		t.Error("execute during grace")
	}
	owner(t, `UPDATE restrictions.restrictions SET grace_until = NULL WHERE id = $1`, id)
	owner(t, `UPDATE notify.notifications SET occurred_at = $2 WHERE target->>'id' = $1`, id, clock.Add(-25*time.Hour))
	if code, _ := write(s, &restrMgr, "restrictions.execute", `{"restrictionId":"`+id+`","confirmedRulesVersion":"rules-1"}`, 1); code != 409 {
		t.Error("execute with altered notice evidence")
	}
	due(t, id)
	_, m = write(s, &restrMgr, "restrictions.execute", `{"restrictionId":"`+id+`","confirmedRulesVersion":"rules-1"}`, 1)
	cmd := unitState(m, u)["applyCommandIds"].([]any)[0].(string)
	ackCommand(t, cmd, clock.Add(time.Second))
	_, m = post(s, &restrMgr, "restrictions.get", `{"id":"`+id+`"}`)
	if data(m)["state"] != "applied" {
		t.Fatalf("applied: %v", m)
	}
	// payment of all causes → release_requested with a remove command (IR35 ①); expiry → release failed
	code, m := write(s, &restrMgr, "payments.recordManual", `{"invoiceId":"`+inv+`","paymentReference":"BANK-`+id[:8]+`","confirmedAmountMinor":5000,"currency":"MYR","reason":"bank transfer"}`, 1)
	if code != 200 {
		t.Fatalf("record manual: %d %v", code, m)
	}
	_, m = post(s, &restrMgr, "restrictions.get", `{"id":"`+id+`"}`)
	if data(m)["state"] != "release_requested" || unitState(m, u)["releaseState"] != "requested" || data(m)["releaseIntent"].(map[string]any)["source"] != "payment" {
		t.Fatalf("payment release: %v", m)
	}
	expireAt(t, clock.Add(time.Minute))
	_, m = post(s, &restrMgr, "restrictions.get", `{"id":"`+id+`"}`)
	if data(m)["state"] != "release_requested" || unitState(m, u)["releaseState"] != "failed" {
		t.Fatalf("remove expired: %v", m)
	}
	owner(t, `UPDATE restrictions.restrictions SET state = 'released' WHERE id = $1`, id)
}

func TestRestrictionScheduleRecipients(t *testing.T) {
	s := server(t)
	u := boundUnit(t, s, "online")
	k, inv := overdueContract(t, s, u)
	owner(t, `UPDATE identity.memberships SET valid_until = $2 WHERE role = 'client' AND organization_id = $1 AND (valid_until IS NULL OR valid_until > $2)`,
		seed.ID("org-customer-b"), clock.Add(-time.Minute))
	t.Cleanup(func() {
		owner(t, `UPDATE identity.memberships SET valid_until = NULL WHERE id = $1`, seed.ID("customer-b"))
	})
	if code, m := write(s, &restrMgr, "restrictions.schedule", scheduleBody(k, inv, []string{u}, nil), 0); code != 422 {
		t.Errorf("no recipients: %d %v", code, m)
	}
}

// executed schedules and executes a temperature limit on the units, returning the restriction ID and response.
func executed(t *testing.T, s *apiserver.Server, k, inv string, units []string) (string, map[string]any) {
	t.Helper()
	code, m := write(s, &restrMgr, "restrictions.schedule", scheduleBody(k, inv, units, nil), 0)
	if code != 200 {
		t.Fatalf("schedule: %d %v", code, m)
	}
	id := data(m)["id"].(string)
	due(t, id)
	code, m = write(s, &restrMgr, "restrictions.execute", `{"restrictionId":"`+id+`","confirmedRulesVersion":"rules-1"}`, 1)
	if code != 200 {
		t.Fatalf("execute: %d %v", code, m)
	}
	return id, m
}

func cmdOf(m map[string]any, unit, field string) string {
	ids := unitState(m, unit)[field].([]any)
	return ids[len(ids)-1].(string)
}

func TestRestrictionExceptionReleaseRetry(t *testing.T) {
	s := server(t)
	online, offline := boundUnit(t, s, "online"), boundUnit(t, s, "offline")
	k, inv := overdueContract(t, s, online, offline)
	id, m := executed(t, s, k, inv, []string{online, offline})
	get := func() map[string]any { _, m := post(s, &restrMgr, "restrictions.get", `{"id":"`+id+`"}`); return m }
	retry := func(a *actor, phase, rules string, units ...string) (int, map[string]any) {
		return write(s, a, "restrictions.retry", `{"restrictionId":"`+id+`","unitIds":["`+strings.Join(units, `","`)+`"],"phase":"`+phase+`","confirmedRulesVersion":"`+rules+`","reason":"device back"}`, ver(get()))
	}
	ackCommand(t, cmdOf(m, online, "applyCommandIds"), clock.Add(time.Second))
	if code, _ := retry(&restrMgr, "apply", "rules-1", offline); code != 409 {
		t.Error("retry while the intent is open")
	}
	expireAt(t, clock.Add(time.Minute))
	if st := unitState(get(), offline); st["applyState"] != "not_sent" {
		t.Fatalf("expired intent: %v", st)
	}
	if code, _ := retry(&restrMgr, "apply", "rules-9", offline); code != 422 {
		t.Error("retry with another rules version")
	}
	if code, _ := retry(&restrMgr, "release", "rules-1", offline); code != 409 {
		t.Error("retry release while requested")
	}
	if code, _ := retry(&overrider, "apply", "rules-1", offline); code != 403 {
		t.Error("override-only apply retry")
	}
	if code, _ := retry(&restrMgr, "apply", "rules-1", uuid.NewString()); code != 422 {
		t.Error("retry unknown unit")
	}
	v := ver(get())
	if code, m := retry(&restrMgr, "apply", "rules-1", online); code != 200 || ver(m) != v {
		t.Error("retry of an applied unit returns the existing result")
	}
	owner(t, `UPDATE devices.devices SET connection = 'online' WHERE unit_id = $1`, offline)
	code, m := retry(&restrMgr, "apply", "rules-1", offline)
	if code != 200 || unitState(m, offline)["applyState"] != "sent_unknown" || len(unitState(m, offline)["applyCommandIds"].([]any)) != 2 {
		t.Fatalf("retry apply: %d %v", code, m)
	}
	ackCommand(t, cmdOf(m, offline, "applyCommandIds"), clock.Add(2*time.Second))
	if data(get())["state"] != "applied" {
		t.Fatal("applied after retry")
	}
	// explicit release needs payment or an exception
	if code, _ := write(s, &restrMgr, "restrictions.release", `{"restrictionId":"`+id+`"}`, ver(get())); code != 403 {
		t.Error("release while unpaid")
	}
	deferBody := func(until time.Time) string {
		return `{"restrictionId":"` + id + `","until":"` + until.Format(time.RFC3339) + `","reason":"payment plan agreed"}`
	}
	if code, _ := write(s, &restrMgr, "restrictions.defer", deferBody(clock.Add(-time.Hour)), ver(get())); code != 422 {
		t.Error("past grace")
	}
	if code, _ := write(s, &restrMgr, "restrictions.defer", deferBody(clock.Add(100*24*time.Hour)), ver(get())); code != 422 {
		t.Error("grace beyond 90 days")
	}
	code, m = write(s, &restrMgr, "restrictions.defer", deferBody(clock.Add(48*time.Hour)), ver(get()))
	if code != 200 || data(m)["state"] != "release_requested" || data(m)["graceUntil"] == nil || data(m)["releaseIntent"].(map[string]any)["source"] != "exception" ||
		unitState(m, online)["releaseState"] != "requested" || unitState(m, offline)["releaseState"] != "requested" {
		t.Fatalf("defer: %d %v", code, m)
	}
	if code, m2 := write(s, &restrMgr, "restrictions.release", `{"restrictionId":"`+id+`"}`, ver(m)); code != 200 || ver(m2) != ver(m) {
		t.Error("release idempotent while release_requested")
	}
	ackCommand(t, cmdOf(m, online, "releaseCommandIds"), clock.Add(3*time.Second))
	expireAt(t, clock.Add(time.Minute))
	m = get()
	if data(m)["state"] != "release_requested" || unitState(m, offline)["releaseState"] != "failed" || unitState(m, online)["releaseState"] != "released" {
		t.Fatalf("partial release: %v", m)
	}
	if code, _ := retry(&overrider, "release", "rules-1", offline); code != 403 {
		t.Error("override-only retry of a non-override release")
	}
	owner(t, `UPDATE devices.devices SET connection = 'offline' WHERE unit_id = $1`, offline)
	if code, m := retry(&restrMgr, "release", "rules-1", offline); code == 200 || m["code"] != "OFFLINE" {
		t.Errorf("offline retry release: %d %v", code, m)
	}
	owner(t, `UPDATE devices.devices SET connection = 'online' WHERE unit_id = $1`, offline)
	code, m = retry(&restrMgr, "release", "rules-1", offline, online)
	if code != 200 || unitState(m, offline)["releaseState"] != "requested" {
		t.Fatalf("retry release: %d %v", code, m)
	}
	if code, _ := retry(&restrMgr, "release", "rules-1", offline); code != 409 {
		t.Error("retry release with an unfinished remove")
	}
	ackCommand(t, cmdOf(m, offline, "releaseCommandIds"), clock.Add(4*time.Second))
	if data(get())["state"] != "released" {
		t.Fatal("released after retry")
	}
	for _, op := range []string{"restrictions.defer", "restrictions.exempt"} {
		if code, _ := write(s, &restrMgr, op, deferBody(clock.Add(time.Hour)), ver(get())); code != 409 {
			t.Errorf("%s on released", op)
		}
	}
	if code, _ := write(s, &restrMgr, "restrictions.override", `{"restrictionId":"`+id+`","reason":"x"}`, ver(get())); code != 409 {
		t.Error("override released")
	}
	if code, _ := write(s, &restrMgr, "restrictions.release", `{"restrictionId":"`+id+`"}`, ver(get())); code != 409 {
		t.Error("release released")
	}
}

func TestRestrictionOverrideReconcile(t *testing.T) {
	s := server(t)
	u := boundUnit(t, s, "online")
	k, inv := overdueContract(t, s, u)
	id, _ := executed(t, s, k, inv, []string{u})
	get := func() map[string]any { _, m := post(s, &restrMgr, "restrictions.get", `{"id":"`+id+`"}`); return m }
	reconcile := func(a *actor) (int, map[string]any) {
		return write(s, a, "restrictions.reconcile", `{"restrictionId":"`+id+`","unitIds":["`+u+`"]}`, ver(get()))
	}
	if code, _ := reconcile(&restrMgr); code != 409 {
		t.Error("reconcile while the apply is pending")
	}
	expireAt(t, clock.Add(time.Minute))
	if code, _ := write(s, &restrMgr, "restrictions.retry", `{"restrictionId":"`+id+`","unitIds":["`+u+`"],"phase":"apply","confirmedRulesVersion":"rules-1","reason":"r"}`, ver(get())); code != 409 {
		t.Error("retry apply of sent_unknown")
	}
	if code, _ := reconcile(&overrider); code != 403 {
		t.Error("override-only reconcile during application")
	}
	owner(t, `UPDATE assets.units SET last_seen_at = $2 WHERE id = $1`, u, clock.Add(-time.Minute))
	if code, m := reconcile(&restrMgr); code == 200 || m["code"] != "TIMEOUT" {
		t.Errorf("stale observation: %d %v", code, m)
	}
	owner(t, `UPDATE assets.units SET last_seen_at = $2, observed_restriction = $3 WHERE id = $1`, u, clock, `{"restrictionId":"`+uuid.NewString()+`","rulesVersion":"rules-1"}`)
	if code, _ := reconcile(&restrMgr); code != 409 {
		t.Error("another restriction observed")
	}
	owner(t, `UPDATE assets.units SET observed_restriction = $2 WHERE id = $1`, u, `{"restrictionId":"`+id+`","rulesVersion":"rules-1"}`)
	owner(t, `UPDATE devices.devices SET connection = 'offline' WHERE unit_id = $1`, u)
	if code, m := reconcile(&restrMgr); code == 200 || m["code"] != "OFFLINE" {
		t.Errorf("offline reconcile: %d %v", code, m)
	}
	owner(t, `UPDATE devices.devices SET connection = 'online' WHERE unit_id = $1`, u)
	code, m := reconcile(&restrMgr)
	if code != 200 || data(m)["state"] != "applied" || unitState(m, u)["applyState"] != "applied" {
		t.Fatalf("reconcile applied: %d %v", code, m)
	}
	if code, _ := write(s, &hq, "restrictions.override", `{"restrictionId":"`+id+`","reason":"hardship"}`, ver(get())); code != 403 {
		t.Error("override without permission")
	}
	code, m = write(s, &overrider, "restrictions.override", `{"restrictionId":"`+id+`","reason":"medical hardship"}`, ver(get()))
	if code != 200 || data(m)["projection"] != "release" || data(m)["state"] != "release_requested" || data(m)["releaseIntent"].(map[string]any)["source"] != "override" {
		t.Fatalf("override: %d %v", code, m)
	}
	if code, m2 := write(s, &overrider, "restrictions.override", `{"restrictionId":"`+id+`","reason":"again"}`, ver(m)); code != 200 || ver(m2) != ver(m) {
		t.Error("override idempotent")
	}
	expireAt(t, clock.Add(time.Minute))
	code, m = write(s, &overrider, "restrictions.retry", `{"restrictionId":"`+id+`","unitIds":["`+u+`"],"phase":"release","confirmedRulesVersion":"rules-1","reason":"retry remove"}`, ver(get()))
	if code != 200 || data(m)["projection"] != "release" || unitState(m, u)["releaseState"] != "requested" {
		t.Fatalf("override-only retry release: %d %v", code, m)
	}
	ackCommand(t, cmdOf(m, u, "releaseCommandIds"), clock.Add(time.Second))
	if data(get())["state"] != "released" {
		t.Fatal("released after override")
	}
	var status string
	ownerScan(t, `SELECT status FROM billing.invoices WHERE id = $1`, []any{inv}, &status)
	if status != "unpaid" {
		t.Error("override keeps the invoice unpaid")
	}
}

func TestRestrictionReconcileNotApplied(t *testing.T) {
	s := server(t)
	u := boundUnit(t, s, "online")
	k, inv := overdueContract(t, s, u)
	id, _ := executed(t, s, k, inv, []string{u})
	get := func() map[string]any { _, m := post(s, &restrMgr, "restrictions.get", `{"id":"`+id+`"}`); return m }
	expireAt(t, clock.Add(time.Minute))
	code, m := write(s, &restrMgr, "restrictions.exempt", `{"restrictionId":"`+id+`","until":"`+clock.Add(time.Hour).Format(time.RFC3339)+`","reason":"hospital"}`, ver(get()))
	if code != 200 || data(m)["exception"].(map[string]any)["reason"] != "hospital" || unitState(m, u)["releaseState"] != "waiting_reconcile" {
		t.Fatalf("exempt: %d %v", code, m)
	}
	if _, m := post(s, &customerB, "restrictions.forInvoice", `{"invoiceId":"`+inv+`","query":{"filters":{"state":"release_requested"}}}`); len(items(m)) != 1 ||
		items(m)[0]["exception"].(map[string]any)["reason"] != nil || items(m)[0]["releaseIntent"].(map[string]any)["actorMembershipId"] != "masked" {
		t.Fatalf("client mask: %v", m)
	}
	if code, _ := post(s, &customerB, "restrictions.forInvoice", `{"invoiceId":"`+inv+`","query":{"filters":{"contractId":"`+k+`"}}}`); code != 422 {
		t.Error("forInvoice contract filter")
	}
	owner(t, `UPDATE assets.units SET last_seen_at = $2, observed_restriction = NULL WHERE id = $1`, u, clock)
	code, m = write(s, &restrMgr, "restrictions.reconcile", `{"restrictionId":"`+id+`","unitIds":["`+u+`"]}`, ver(get()))
	if code != 200 || data(m)["state"] != "released" || unitState(m, u)["releaseState"] != "not_required" || unitState(m, u)["applyState"] != "not_applied" {
		t.Fatalf("reconcile not applied: %d %v", code, m)
	}
}
