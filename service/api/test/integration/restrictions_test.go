package integration_test

import (
	"context"
	"maps"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/control"
	"github.com/pradita/ac-project/service/api/internal/modules/restrictions"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

const (
	fieldCauseInvoiceIDs   = "causeInvoiceIds"
	fieldPolicy            = "policy"
	projectionRelease      = "release"
	statusNotApplied       = "not_applied"
	statusSentUnknown      = "sent_unknown"
	statusWaitingReconcile = "waiting_reconcile"
)

// boundUnit creates a customer-b unit with a bound device in the given connection state.
func boundUnit(t *testing.T, s *apiserver.Server, connection string) string {
	t.Helper()
	unit := newUnit(t, s, "Restr AC")
	dev, binding := uuid.NewString(), uuid.NewString()
	owner(
		t,
		`INSERT INTO devices.devices (id, tenant_id, serial, binding_id, unit_id, target_unit_id, created_by_membership_id,
			connection, last_seen_at, firmware_version, power_signal)
		VALUES ($1,$2,$3,$4,$5,$5,$6,$7,$8,'1.0','on')`,
		dev,
		seed.ID("tenant-a"),
		"SN-"+dev[:8],
		binding,
		unit,
		seed.ID("hq-operator"),
		connection,
		clock(),
	)
	owner(
		t,
		`INSERT INTO devices.device_bindings (id, tenant_id, device_id, unit_id, customer_org_id, bound_at, reason,
			actor_membership_id)
		VALUES ($1,$2,$3,$4,$5,$6,'test',$7)`,
		binding,
		seed.ID("tenant-a"),
		dev,
		unit,
		seed.ID("org-customer-b"),
		clock().Add(-time.Hour),
		seed.ID("hq-operator"),
	)
	owner(t, `UPDATE assets.units SET connection = $2 WHERE id = $1`, unit, connection)

	return unit
}

// overdueContract creates an RTO contract on the units with one overdue unpaid invoice.
func overdueContract(t *testing.T, srv *apiserver.Server, units ...string) (string, string) {
	t.Helper()

	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }

	code, res := write(
		srv,
		hq(),
		"contracts.save",
		`{"customerId":"`+seed.ID("cust-b").
			String()+
			`","unitIds":["`+strings.Join(
			units,
			`","`,
		)+`"],"planType":"rto","startAt":"`+stamp(
			-24*30,
		)+
			`","endAt":"`+stamp(
			24*365,
		)+`","priceMinor":5000,"currency":"MYR","restrictionEligible":true,"rulesVersion":"rules-1"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("contract: %d %v", code, res)
	}

	key := asStr(t, data(res)["id"])

	code, res = write(
		srv,
		hq(),
		"invoices.create",
		`{"amountMinor":5000,"currency":"MYR","contractId":"`+key+`","contractVersion":1,"period":{"from":"`+stamp(
			-24*14,
		)+`","to":"`+stamp(
			24*16,
		)+`"},"dueAt":"`+stamp(
			48,
		)+`"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("invoice: %d %v", code, res)
	}

	inv := asStr(t, data(res)["id"])
	owner(t, `UPDATE billing.invoices SET due_at = $2 WHERE id = $1`, inv, clock().Add(-time.Hour))

	return key, inv
}

func scheduleBody(k, inv string, units []string, extra map[string]string) string {
	bodyFields := map[string]string{
		fieldContractID:              `"` + k + `"`,
		fieldExpectedContractVersion: "1",
		fieldCauseInvoiceIDs:         `["` + inv + `"]`,
		fieldUnitIDs:                 `["` + strings.Join(units, `","`) + `"]`,
		fieldPolicy:                  `{"kind":"temperature_limit","minimumCoolingSetpoint":24}`,
		fieldExecuteAfter:            `"` + clock().Add(25*time.Hour).Format(time.RFC3339) + `"`,
		fieldReason:                  `"Invoice overdue"`,
		fieldRulesVersion:            `"rules-1"`,
	}
	maps.Copy(bodyFields, extra)

	parts := make([]string, 0, len(bodyFields))
	for k, v := range bodyFields {
		parts = append(parts, `"`+k+`":`+v)
	}

	return "{" + strings.Join(parts, ",") + "}"
}

// due moves a scheduled restriction's notice into the past so it can be executed.
func due(t *testing.T, restrictionID string) {
	t.Helper()
	owner(
		t,
		`UPDATE restrictions.restrictions SET notice_at = $2, execute_after = $3 WHERE id = $1`,
		restrictionID,
		clock().Add(-26*time.Hour),
		clock().Add(-time.Hour),
	)
	owner(
		t,
		`UPDATE notify.notifications SET occurred_at = $2 WHERE target->>'id' = $1`,
		restrictionID,
		clock().Add(-26*time.Hour),
	)
}

func unitState(t *testing.T, m map[string]any, unit string) map[string]any {
	t.Helper()

	for _, u := range asArr(t, data(m)["perUnit"]) {
		if x := asObj(t, u); x["unitId"] == unit {
			return x
		}
	}

	return nil
}

func expireAt(t *testing.T, now time.Time) {
	t.Helper()

	ctx := context.Background()

	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer func() { _ = conn.Close(ctx) }()

	beginFuncErr := pgx.BeginFunc(
		ctx,
		conn,
		func(tx pgx.Tx) error {
			_, err := control.ExpireCommands(ctx, tx, now)

			return apperr.Wrap(err, "control.ExpireCommands")
		},
	)
	if beginFuncErr != nil {
		t.Fatal(beginFuncErr)
	}

	drainEvents(
		t,
	) // the worker tick is not an operation: apply CommandsEnded as billing's consumer would (IR185)
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRestrictionLifecycle(t *testing.T) {
	srv := server(t)
	online, offline := boundUnit(t, srv, "online"), boundUnit(t, srv, "offline")
	units := []string{online, offline}
	key, inv := overdueContract(t, srv, online, offline)
	refused := map[string]string{ // the field each VALIDATION case is refused for
		caseBlankReason:      refusedReasonLength,
		caseDuplicateUnit:    refusedUnitIDsInvalid,
		"missing cause":      "causeInvoiceIds=errors.cause_invoices_mismatch",
		"notice under 24h":   "executeAfter=errors.notice_24h",
		"power off setpoint": "policy=error.invalid",
		"rules mismatch":     "rulesVersion=errors.rules_version_mismatch",
		"setpoint off step":  "policy=errors.setpoint_out_of_range",
		"setpoint too low":   "policy=errors.setpoint_out_of_range",
		"unit not in deal":   "unitIds=errors.unit_not_in_contract",
	}

	for name, testCase := range map[string]struct {
		e    map[string]string
		code int
	}{
		"notice under 24h": {
			map[string]string{fieldExecuteAfter: `"` + clock().Add(23*time.Hour).Format(time.RFC3339) + `"`},
			422,
		},
		"missing cause":    {map[string]string{fieldCauseInvoiceIDs: `["` + uuid.NewString() + `"]`}, 422},
		"rules mismatch":   {map[string]string{fieldRulesVersion: `"rules-2"`}, 422},
		"setpoint too low": {map[string]string{fieldPolicy: `{"kind":"temperature_limit","minimumCoolingSetpoint":15}`}, 422},
		"setpoint off step": {map[string]string{fieldPolicy: `{"kind":"temperature_limit",` +
			`"minimumCoolingSetpoint":24.5}`}, 422},
		"power off setpoint": {map[string]string{fieldPolicy: `{"kind":"power_off","minimumCoolingSetpoint":24}`}, 422},
		"unit not in deal":   {map[string]string{fieldUnitIDs: `["` + seed.ID("unit-online-rto").String() + `"]`}, 422},
		caseDuplicateUnit:    {map[string]string{fieldUnitIDs: `["` + online + `","` + online + `"]`}, 422},
		caseBlankReason:      {map[string]string{fieldReason: `"  "`}, 422},
		"stale contract":     {map[string]string{fieldExpectedContractVersion: "2"}, 409},
		"unknown contract":   {map[string]string{fieldContractID: `"` + uuid.NewString() + `"`}, 404},
	} {
		code, res := write(
			srv,
			restrMgr(),
			"restrictions.schedule",
			scheduleBody(key, inv, units, testCase.e),
			0,
		)
		if w, ok := refused[name]; ok {
			refusal(t, name, code, res, w)
		} else if code != testCase.code {
			t.Errorf("schedule %s: %d want %d %v", name, code, testCase.code, res)
		}
	}

	if code, _ := write(
		srv,
		hq(),
		"restrictions.schedule",
		scheduleBody(key, inv, units, nil),
		0,
	); code != 403 {
		t.Error("schedule without restriction.write")
	}

	code, resp := write(
		srv,
		restrMgr(),
		"restrictions.schedule",
		scheduleBody(key, inv, units, nil),
		0,
	)
	if code != 200 || data(resp)["state"] != statusScheduled ||
		len(asArr(t, data(resp)["noticeNotificationIds"])) == 0 ||
		data(resp)["noticeAt"] != clock().Format(time.RFC3339) {
		t.Fatalf("schedule: %d %v", code, resp)
	}

	restrictionID := asStr(t, data(resp)["id"])

	if code, _ := write(
		srv,
		restrMgr(),
		"restrictions.schedule",
		scheduleBody(key, inv, []string{online}, nil),
		0,
	); code != 409 {
		t.Error("second active restriction on a unit")
	}
	// the customer's notice is in their inbox (noticeNotificationIds, FR-C12)
	notice := asArr(t, data(resp)["noticeNotificationIds"])
	inboxCode, inbox := post(
		srv,
		customerB(),
		"notifications.list",
		`{"filters":{"type":"restriction"},"limit":100}`,
	)
	found := 0

	for _, it := range items(t, inbox) {
		if slices.Contains(notice, it["id"]) {
			found++
		}
	}

	if inboxCode != 200 || found == 0 {
		t.Errorf(
			"the customer's restriction notice: %d, %d of %d notices in the inbox",
			inboxCode,
			found,
			len(notice),
		)
	}

	exec := func(rules string, version int) (int, map[string]any) {
		return write(
			srv,
			restrMgr(),
			"restrictions.execute",
			`{"restrictionId":"`+restrictionID+`","confirmedRulesVersion":"`+rules+`"}`,
			version,
		)
	}
	if code, _ := exec("rules-1", 1); code != 409 {
		t.Error("execute before executeAfter")
	}

	due(t, restrictionID)

	if code, m := exec(
		"rules-2",
		1,
	); code != 422 ||
		fields(m) != "confirmedRulesVersion=errors.rules_version_mismatch" {
		t.Errorf("execute with other rules version: %d %s", code, fields(m))
	}

	if code, _ := exec("rules-1", 5); code != 409 {
		t.Error("execute with stale version")
	}

	code, resp = exec("rules-1", 1)
	if code != 200 || data(resp)["state"] != statusRequested ||
		asNum(t, data(resp)["version"]) != 2 {
		t.Fatalf("execute: %d %v", code, resp)
	}

	onlineState, off := unitState(t, resp, online), unitState(t, resp, offline)
	if onlineState["applyState"] != statusSentUnknown || off["applyState"] != statusNotSent ||
		off["pendingReason"] != reasonOffline ||
		len(asArr(t, off["applyCommandIds"])) != 1 {
		t.Fatalf("per unit: %v %v", onlineState, off)
	}

	var status, delivery string
	ownerScan(
		t,
		`SELECT status, delivery FROM control.commands WHERE id = $1`,
		[]any{asArr(t, off["applyCommandIds"])[0]},
		&status,
		&delivery,
	)

	if status != statusRequested || delivery != statusNotSent {
		t.Errorf("offline intent %s/%s", status, delivery)
	}

	if code, _ := exec("rules-1", 2); code != 409 {
		t.Error("execute twice")
	}

	if !ackCommand(
		t,
		asStr(t, asArr(t, onlineState["applyCommandIds"])[0]),
		clock().Add(time.Second),
	) {
		t.Fatal("ack apply")
	}

	_, resp = post(srv, restrMgr(), "restrictions.get", `{"id":"`+restrictionID+`"}`)
	if data(resp)["state"] != statusRequested ||
		unitState(t, resp, online)["applyState"] != statusApplied ||
		unitState(t, resp, online)["observedRestriction"] == nil {
		t.Fatalf("after ack: %v", resp)
	}

	var obs *string
	ownerScan(
		t,
		`SELECT observed_restriction->>'restrictionId' FROM assets.units WHERE id = $1`,
		[]any{online},
		&obs,
	)

	if obs == nil || *obs != restrictionID {
		t.Error("unit observed restriction")
	}

	// projections
	if code, res := post(
		srv,
		overrider(),
		"restrictions.get",
		`{"id":"`+restrictionID+`"}`,
	); code != 200 || data(res)["projection"] != projectionRelease ||
		data(res)["contractId"] != nil {
		t.Fatalf("release projection: %d %v", code, res)
	}

	if code, _ := post(
		srv,
		overrider(),
		"restrictions.list",
		`{"filters":{"contractId":"`+key+`"}}`,
	); code != 403 {
		t.Error("override-only contract filter")
	}

	if _, res := post(
		srv,
		overrider(),
		"restrictions.list",
		`{"filters":{"status":"requested"},"limit":100}`,
	); len(items(t, res)) == 0 ||
		items(t, res)[0]["projection"] != projectionRelease {
		t.Error("override-only list")
	}

	if _, res := post(
		srv,
		restrMgr(),
		"restrictions.list",
		`{"filters":{"contractId":"`+key+`","invoiceId":"`+inv+`"}}`,
	); len(items(t, res)) != 1 ||
		len(asArr(t, items(t, res)[0]["events"])) < 2 {
		t.Fatalf("manager list: %v", res)
	}

	if code, res := post(
		srv,
		restrMgr(),
		"restrictions.list",
		`{"filters":{"status":"paused"}}`,
	); code != 422 ||
		fields(res) != refusedFiltersInvalid {
		t.Errorf("bad state filter: %d %s", code, fields(res))
	}

	if code, _ := post(
		srv,
		customerB(),
		"restrictions.get",
		`{"id":"`+restrictionID+`"}`,
	); code != 403 {
		t.Error("client get")
	}

	_, resp = post(
		srv,
		customerB(),
		"restrictions.forInvoice",
		`{"invoiceId":"`+inv+`","query":{}}`,
	)
	if len(items(t, resp)) != 1 {
		t.Fatalf("client forInvoice: %v", resp)
	}

	for _, e := range asArr(t, items(t, resp)[0]["events"]) {
		if ev := asObj(t, e); ev["actorId"] != "masked" || ev["actorName"] != nil ||
			ev["reason"] != nil {
			t.Errorf("unmasked event %v", ev)
		}
	}

	if code, _ := post(
		srv,
		customerA(),
		"restrictions.forInvoice",
		`{"invoiceId":"`+inv+`","query":{}}`,
	); code != 404 {
		t.Error("other customer's invoice")
	}

	if code, _ := post(
		srv,
		restrMgr(),
		"restrictions.get",
		`{"id":"`+uuid.NewString()+`"}`,
	); code != 404 {
		t.Error("unknown restriction")
	}

	// cancel requested → release_requested with D03 evaluation
	cancel := func(version int) (int, map[string]any) {
		return write(
			srv,
			restrMgr(),
			"restrictions.cancel",
			`{"restrictionId":"`+restrictionID+`","reason":"customer agreed plan"}`,
			version,
		)
	}

	code, resp = cancel(3)
	if code != 200 || data(resp)["state"] != statusReleaseRequested ||
		asObj(t, data(resp)["releaseIntent"])["source"] != "cancel" {
		t.Fatalf("cancel: %d %v", code, resp)
	}

	onlineState, off = unitState(t, resp, online), unitState(t, resp, offline)
	if onlineState["releaseState"] != statusRequested ||
		len(asArr(t, onlineState["releaseCommandIds"])) != 1 ||
		off["applyState"] != statusNotApplied ||
		off["releaseState"] != "not_required" {
		t.Fatalf("release eval: %v %v", onlineState, off)
	}

	ownerScan(
		t,
		`SELECT status FROM control.commands WHERE id = $1`,
		[]any{asArr(t, off["applyCommandIds"])[0]},
		&status,
	)

	if status != statusCancelled {
		t.Errorf("undelivered apply %s", status)
	}

	v := ver(t, resp)
	if code, m := cancel(v); code != 200 || ver(t, m) != v {
		t.Error("cancel idempotent while release_requested")
	}

	if !ackCommand(
		t,
		asStr(t, asArr(t, onlineState["releaseCommandIds"])[0]),
		clock().Add(2*time.Second),
	) {
		t.Fatal("ack remove")
	}

	_, resp = post(srv, restrMgr(), "restrictions.get", `{"id":"`+restrictionID+`"}`)
	if data(resp)["state"] != statusReleased ||
		unitState(t, resp, online)["releaseState"] != statusReleased {
		t.Fatalf("released: %v", resp)
	}

	if code, _ := cancel(ver(t, resp)); code != 409 {
		t.Error("cancel released")
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRestrictionCancelAndPayment(t *testing.T) {
	srv := server(t)
	unit := boundUnit(t, srv, "online")
	key, inv := overdueContract(t, srv, unit)
	// a scheduled restriction is cancelled without commands
	_, resp := write(
		srv,
		restrMgr(),
		"restrictions.schedule",
		scheduleBody(key, inv, []string{unit}, map[string]string{fieldPolicy: policyPowerOff}),
		0,
	)

	restrictionID := asStr(t, data(resp)["id"])
	if code, res := write(
		srv,
		restrMgr(),
		"restrictions.cancel",
		`{"restrictionId":"`+restrictionID+`","reason":"mistake"}`,
		1,
	); code != 200 ||
		data(res)["state"] != statusCancelled {
		t.Fatalf("cancel scheduled: %d %v", code, res)
	}
	// all causes paid right before execution: cancelled with no apply request
	_, resp = write(
		srv,
		restrMgr(),
		"restrictions.schedule",
		scheduleBody(key, inv, []string{unit}, nil),
		0,
	)
	restrictionID = asStr(t, data(resp)["id"])
	due(t, restrictionID)
	owner(
		t,
		`UPDATE billing.invoices SET status = 'paid', paid_at = $2 WHERE id = $1`,
		inv,
		clock(),
	)

	if code, res := write(
		srv,
		restrMgr(),
		"restrictions.execute",
		`{"restrictionId":"`+restrictionID+`","confirmedRulesVersion":"rules-1"}`,
		1,
	); code != 200 ||
		data(res)["state"] != statusCancelled ||
		len(asArr(t, unitState(t, res, unit)["applyCommandIds"])) != 0 {
		t.Fatalf("execute after payment: %d %v", code, res)
	}

	owner(t, `UPDATE billing.invoices SET status = 'unpaid', paid_at = NULL WHERE id = $1`, inv)
	// exemption blocks execution; no recipients blocks schedule
	_, resp = write(
		srv,
		restrMgr(),
		"restrictions.schedule",
		scheduleBody(key, inv, []string{unit}, nil),
		0,
	)
	restrictionID = asStr(t, data(resp)["id"])
	due(t, restrictionID)
	owner(
		t,
		`UPDATE restrictions.restrictions SET grace_until = $2 WHERE id = $1`,
		restrictionID,
		clock().Add(time.Hour),
	)

	if code, _ := write(
		srv,
		restrMgr(),
		"restrictions.execute",
		`{"restrictionId":"`+restrictionID+`","confirmedRulesVersion":"rules-1"}`,
		1,
	); code != 409 {
		t.Error("execute during grace")
	}

	owner(t, `UPDATE restrictions.restrictions SET grace_until = NULL WHERE id = $1`, restrictionID)
	owner(
		t,
		`UPDATE notify.notifications SET occurred_at = $2 WHERE target->>'id' = $1`,
		restrictionID,
		clock().Add(-25*time.Hour),
	)

	if code, _ := write(
		srv,
		restrMgr(),
		"restrictions.execute",
		`{"restrictionId":"`+restrictionID+`","confirmedRulesVersion":"rules-1"}`,
		1,
	); code != 409 {
		t.Error("execute with altered notice evidence")
	}

	due(t, restrictionID)
	_, resp = write(
		srv,
		restrMgr(),
		"restrictions.execute",
		`{"restrictionId":"`+restrictionID+`","confirmedRulesVersion":"rules-1"}`,
		1,
	)
	cmd := asStr(t, asArr(t, unitState(t, resp, unit)["applyCommandIds"])[0])
	ackCommand(t, cmd, clock().Add(time.Second))

	_, resp = post(srv, restrMgr(), "restrictions.get", `{"id":"`+restrictionID+`"}`)
	if data(resp)["state"] != statusApplied {
		t.Fatalf("applied: %v", resp)
	}
	// payment of all causes → release_requested with a remove command (IR35 ①); expiry → release failed
	code, resp := write(
		srv,
		restrMgr(),
		"payments.recordManual",
		`{"invoiceId":"`+inv+`","paymentReference":"BANK-`+restrictionID[:8]+`","confirmedAmountMinor":5000,`+
			`"currency":"MYR",`+
			`"reason":"bank transfer"}`,
		1,
	)
	if code != 200 {
		t.Fatalf("record manual: %d %v", code, resp)
	}

	_, resp = post(srv, restrMgr(), "restrictions.get", `{"id":"`+restrictionID+`"}`)
	if data(resp)["state"] != statusReleaseRequested ||
		unitState(t, resp, unit)["releaseState"] != statusRequested ||
		asObj(t, data(resp)["releaseIntent"])["source"] != "payment" {
		t.Fatalf("payment release: %v", resp)
	}

	expireAt(t, clock().Add(time.Minute))

	_, resp = post(srv, restrMgr(), "restrictions.get", `{"id":"`+restrictionID+`"}`)
	if data(resp)["state"] != statusReleaseRequested ||
		unitState(t, resp, unit)["releaseState"] != statusFailed {
		t.Fatalf("remove expired: %v", resp)
	}

	owner(t, `UPDATE restrictions.restrictions SET state = 'released' WHERE id = $1`, restrictionID)
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRestrictionScheduleRecipients(t *testing.T) {
	srv := server(t)
	unit := boundUnit(t, srv, "online")
	key, inv := overdueContract(t, srv, unit)
	// accounts other tests added start at the clock (IR461): their start moves back too
	owner(
		t,
		`UPDATE identity.memberships SET valid_from = LEAST(valid_from, $2 - interval '1 hour'), valid_until = $2 WHERE role =
			'client' AND organization_id = $1 AND (valid_until IS NULL OR valid_until > $2)`,
		seed.ID("org-customer-b"),
		clock().Add(-time.Minute),
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE identity.memberships SET valid_until = NULL WHERE id = $1`,
			seed.ID("customer-b"),
		)
	})

	if code, res := write(
		srv,
		restrMgr(),
		"restrictions.schedule",
		scheduleBody(key, inv, []string{unit}, nil),
		0,
	); code != 422 ||
		fields(res) != "unitIds=errors.no_notice_recipient" {
		t.Errorf("no recipients: %d %v", code, res)
	}
}

// executed schedules and executes a temperature limit on the units, returning the restriction ID and response.
func executed(
	t *testing.T,
	srv *apiserver.Server,
	contract, inv string,
	units []string,
) (string, map[string]any) {
	t.Helper()

	code, res := write(
		srv,
		restrMgr(),
		"restrictions.schedule",
		scheduleBody(contract, inv, units, nil),
		0,
	)
	if code != 200 {
		t.Fatalf("schedule: %d %v", code, res)
	}

	restrictionID := asStr(t, data(res)["id"])
	due(t, restrictionID)

	code, res = write(
		srv,
		restrMgr(),
		"restrictions.execute",
		`{"restrictionId":"`+restrictionID+`","confirmedRulesVersion":"rules-1"}`,
		1,
	)
	if code != 200 {
		t.Fatalf("execute: %d %v", code, res)
	}

	return restrictionID, res
}

func cmdOf(t *testing.T, m map[string]any, unit, field string) string {
	t.Helper()

	ids := asArr(t, unitState(t, m, unit)[field])

	return asStr(t, ids[len(ids)-1])
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRestrictionExceptionReleaseRetry(t *testing.T) {
	srv := server(t)
	online, offline := boundUnit(t, srv, "online"), boundUnit(t, srv, "offline")
	k, inv := overdueContract(t, srv, online, offline)
	restrictionID, resp := executed(t, srv, k, inv, []string{online, offline})
	get := func() map[string]any {
		_, m := post(srv, restrMgr(), "restrictions.get", `{"id":"`+restrictionID+`"}`)

		return m
	}
	retry := func(a *actor, phase, rules string, units ...string) (int, map[string]any) {
		return write(
			srv,
			a,
			"restrictions.retry",
			`{"restrictionId":"`+restrictionID+`","unitIds":["`+strings.Join(
				units,
				`","`,
			)+`"],"phase":"`+phase+`","confirmedRulesVersion":"`+rules+`","reason":"device back"}`,
			ver(t, get()),
		)
	}

	ackCommand(t, cmdOf(t, resp, online, "applyCommandIds"), clock().Add(time.Second))

	if code, _ := retry(restrMgr(), "apply", "rules-1", offline); code != 409 {
		t.Error("retry while the intent is open")
	}

	expireAt(t, clock().Add(time.Minute))

	if st := unitState(t, get(), offline); st["applyState"] != statusNotSent {
		t.Fatalf("expired intent: %v", st)
	}

	if code, res := retry(
		restrMgr(),
		"apply",
		"rules-9",
		offline,
	); code != 422 ||
		fields(res) != "confirmedRulesVersion=errors.rules_version_mismatch" {
		t.Errorf("retry with another rules version: %d %s", code, fields(res))
	}

	if code, _ := retry(restrMgr(), "release", "rules-1", offline); code != 409 {
		t.Error("retry release while requested")
	}

	if code, _ := retry(overrider(), "apply", "rules-1", offline); code != 403 {
		t.Error("override-only apply retry")
	}

	if code, res := retry(
		restrMgr(),
		"apply",
		"rules-1",
		uuid.NewString(),
	); code != 422 ||
		fields(res) != "unitIds=errors.unit_not_in_restriction" {
		t.Errorf("retry unknown unit: %d %s", code, fields(res))
	}

	v := ver(t, get())
	if code, m := retry(restrMgr(), "apply", "rules-1", online); code != 200 || ver(t, m) != v {
		t.Error("retry of an applied unit returns the existing result")
	}
	// SR26: with every cause invoice paid there is nothing left to apply
	owner(t, `UPDATE billing.invoices SET status = 'paid' WHERE id = $1`, inv)

	if code, res := retry(
		restrMgr(),
		"apply",
		"rules-1",
		offline,
	); code != 409 ||
		res["messageKey"] != "errors.restriction_not_applicable" {
		t.Errorf("apply retry after payment: %d %v", code, res)
	}

	owner(t, `UPDATE billing.invoices SET status = 'unpaid' WHERE id = $1`, inv)
	owner(t, `UPDATE devices.devices SET connection = 'online' WHERE unit_id = $1`, offline)

	code, resp := retry(restrMgr(), "apply", "rules-1", offline)
	if code != 200 || unitState(t, resp, offline)["applyState"] != statusSentUnknown ||
		len(asArr(t, unitState(t, resp, offline)["applyCommandIds"])) != 2 {
		t.Fatalf("retry apply: %d %v", code, resp)
	}

	ackCommand(t, cmdOf(t, resp, offline, "applyCommandIds"), clock().Add(2*time.Second))

	if data(get())["state"] != statusApplied {
		t.Fatal("applied after retry")
	}

	if code, res := retry(
		restrMgr(),
		"apply",
		"rules-1",
		offline,
	); code != 409 ||
		res["messageKey"] != "errors.restriction_state" {
		t.Errorf("apply retry once applied: %d %v", code, res)
	}
	// explicit release needs payment or an exception
	if code, _ := write(
		srv,
		restrMgr(),
		"restrictions.release",
		`{"restrictionId":"`+restrictionID+`"}`,
		ver(t, get()),
	); code != 403 {
		t.Error("release while unpaid")
	}

	deferBody := func(until time.Time) string {
		return `{"restrictionId":"` + restrictionID + `","until":"` + until.Format(
			time.RFC3339,
		) + `","reason":"payment plan agreed"}`
	}
	if code, res := write(
		srv,
		restrMgr(),
		"restrictions.defer",
		deferBody(clock().Add(-time.Hour)),
		ver(t, get()),
	); code != 422 ||
		fields(res) != "until=errors.until_range" {
		t.Errorf("past grace: %d %s", code, fields(res))
	}

	if code, res := write(
		srv,
		restrMgr(),
		"restrictions.defer",
		deferBody(clock().Add(100*24*time.Hour)),
		ver(t, get()),
	); code != 422 ||
		fields(res) != "until=errors.until_range" {
		t.Errorf("grace beyond 90 days: %d %s", code, fields(res))
	}

	code, resp = write(
		srv,
		restrMgr(),
		"restrictions.defer",
		deferBody(clock().Add(48*time.Hour)),
		ver(t, get()),
	)
	if code != 200 || data(resp)["state"] != statusReleaseRequested ||
		data(resp)["graceUntil"] == nil ||
		asObj(t, data(resp)["releaseIntent"])["source"] != "exception" ||
		unitState(t, resp, online)["releaseState"] != statusRequested ||
		unitState(t, resp, offline)["releaseState"] != statusRequested {
		t.Fatalf("defer: %d %v", code, resp)
	}

	if code, again := write(
		srv,
		restrMgr(),
		"restrictions.release",
		`{"restrictionId":"`+restrictionID+`"}`,
		ver(t, resp),
	); code != 200 ||
		ver(t, again) != ver(t, resp) {
		t.Error("release idempotent while release_requested")
	}

	ackCommand(t, cmdOf(t, resp, online, "releaseCommandIds"), clock().Add(3*time.Second))
	expireAt(t, clock().Add(time.Minute))

	resp = get()
	if data(resp)["state"] != statusReleaseRequested ||
		unitState(t, resp, offline)["releaseState"] != statusFailed ||
		unitState(t, resp, online)["releaseState"] != statusReleased {
		t.Fatalf("partial release: %v", resp)
	}

	if code, _ := retry(overrider(), "release", "rules-1", offline); code != 403 {
		t.Error("override-only retry of a non-override release")
	}
	// SR26: a unit waiting for reconciliation, or never confirmed applied, is reconciled, not retried
	for _, states := range [][2]string{{statusWaitingReconcile, statusApplied}, {statusFailed, statusSentUnknown}} {
		owner(
			t,
			`UPDATE restrictions.restriction_units SET release_state = $3, apply_state = $4 WHERE restriction_id = $1 AND unit_id
				= $2`,
			restrictionID,
			offline,
			states[0],
			states[1],
		)

		if code, res := retry(
			restrMgr(),
			"release",
			"rules-1",
			offline,
		); code != 409 ||
			res["messageKey"] != "errors.reconcile_required" {
			t.Errorf("release retry with %v: %d %v", states, code, res)
		}
	}

	owner(
		t,
		`UPDATE restrictions.restriction_units SET release_state = 'failed', apply_state = 'applied' WHERE restriction_id = $1
			AND unit_id = $2`,
		restrictionID,
		offline,
	)
	owner(t, `UPDATE devices.devices SET connection = 'offline' WHERE unit_id = $1`, offline)

	if code, res := retry(
		restrMgr(),
		"release",
		"rules-1",
		offline,
	); code == 200 ||
		res["code"] != codeOffline {
		t.Errorf("offline retry release: %d %v", code, res)
	}

	owner(t, `UPDATE devices.devices SET connection = 'online' WHERE unit_id = $1`, offline)

	code, resp = retry(restrMgr(), "release", "rules-1", offline, online)
	if code != 200 || unitState(t, resp, offline)["releaseState"] != statusRequested {
		t.Fatalf("retry release: %d %v", code, resp)
	}

	if code, _ := retry(restrMgr(), "release", "rules-1", offline); code != 409 {
		t.Error("retry release with an unfinished remove")
	}

	ackCommand(t, cmdOf(t, resp, offline, "releaseCommandIds"), clock().Add(4*time.Second))

	if data(get())["state"] != statusReleased {
		t.Fatal("released after retry")
	}

	for _, operation := range []string{"restrictions.defer", "restrictions.exempt"} {
		if code, _ := write(
			srv,
			restrMgr(),
			operation,
			deferBody(clock().Add(time.Hour)),
			ver(t, get()),
		); code != 409 {
			t.Errorf("%s on released", operation)
		}
	}

	if code, _ := write(
		srv,
		restrMgr(),
		"restrictions.override",
		`{"restrictionId":"`+restrictionID+`","reason":"x"}`,
		ver(t, get()),
	); code != 409 {
		t.Error("override released")
	}

	if code, _ := write(
		srv,
		restrMgr(),
		"restrictions.release",
		`{"restrictionId":"`+restrictionID+`"}`,
		ver(t, get()),
	); code != 409 {
		t.Error("release released")
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRestrictionOverrideReconcile(t *testing.T) {
	srv := server(t)
	unit := boundUnit(t, srv, "online")
	k, inv := overdueContract(t, srv, unit)
	restrictionID, _ := executed(t, srv, k, inv, []string{unit})
	get := func() map[string]any {
		_, m := post(srv, restrMgr(), "restrictions.get", `{"id":"`+restrictionID+`"}`)

		return m
	}

	reconcile := func(a *actor) (int, map[string]any) {
		return write(
			srv,
			a,
			"restrictions.reconcile",
			`{"restrictionId":"`+restrictionID+`","unitIds":["`+unit+`"]}`,
			ver(t, get()),
		)
	}
	if code, _ := reconcile(restrMgr()); code != 409 {
		t.Error("reconcile while the apply is pending")
	}

	expireAt(t, clock().Add(time.Minute))

	if code, _ := write(
		srv,
		restrMgr(),
		"restrictions.retry",
		`{"restrictionId":"`+restrictionID+`","unitIds":["`+unit+`"],"phase":"apply","confirmedRulesVersion":"rules-1",`+
			`"reason":"r"}`,
		ver(t, get()),
	); code != 409 {
		t.Error("retry apply of sent_unknown")
	}

	if code, _ := reconcile(overrider()); code != 403 {
		t.Error("override-only reconcile during application")
	}

	owner(
		t,
		`UPDATE assets.units SET last_seen_at = $2 WHERE id = $1`,
		unit,
		clock().Add(-time.Minute),
	)

	if code, m := reconcile(restrMgr()); code == 200 || m["code"] != codeTimeout {
		t.Errorf("stale observation: %d %v", code, m)
	}

	owner(
		t,
		`UPDATE assets.units SET last_seen_at = $2, observed_restriction = $3 WHERE id = $1`,
		unit,
		clock(),
		`{"restrictionId":"`+uuid.NewString()+`","rulesVersion":"rules-1"}`,
	)

	if code, _ := reconcile(restrMgr()); code != 409 {
		t.Error("another restriction observed")
	}

	owner(
		t,
		`UPDATE assets.units SET observed_restriction = $2 WHERE id = $1`,
		unit,
		`{"restrictionId":"`+restrictionID+`","rulesVersion":"rules-1"}`,
	)
	owner(t, `UPDATE devices.devices SET connection = 'offline' WHERE unit_id = $1`, unit)

	if code, m := reconcile(restrMgr()); code == 200 || m["code"] != codeOffline {
		t.Errorf("offline reconcile: %d %v", code, m)
	}

	owner(t, `UPDATE devices.devices SET connection = 'online' WHERE unit_id = $1`, unit)

	code, res := reconcile(restrMgr())
	if code != 200 || data(res)["state"] != statusApplied ||
		unitState(t, res, unit)["applyState"] != statusApplied {
		t.Fatalf("reconcile applied: %d %v", code, res)
	}

	if code, _ := write(
		srv,
		hq(),
		"restrictions.override",
		`{"restrictionId":"`+restrictionID+`","reason":"hardship"}`,
		ver(t, get()),
	); code != 403 {
		t.Error("override without permission")
	}

	code, res = write(
		srv,
		overrider(),
		"restrictions.override",
		`{"restrictionId":"`+restrictionID+`","reason":"medical hardship"}`,
		ver(t, get()),
	)
	if code != 200 || data(res)["projection"] != projectionRelease ||
		data(res)["state"] != statusReleaseRequested ||
		asObj(t, data(res)["releaseIntent"])["source"] != "override" {
		t.Fatalf("override: %d %v", code, res)
	}

	if code, again := write(
		srv,
		overrider(),
		"restrictions.override",
		`{"restrictionId":"`+restrictionID+`","reason":"again"}`,
		ver(t, res),
	); code != 200 ||
		ver(t, again) != ver(t, res) {
		t.Error("override idempotent")
	}

	expireAt(t, clock().Add(time.Minute))

	code, res = write(
		srv,
		overrider(),
		"restrictions.retry",
		`{"restrictionId":"`+restrictionID+`","unitIds":["`+unit+`"],"phase":"release","confirmedRulesVersion":"rules-1",`+
			`"reason":"retry `+
			`remove"}`,
		ver(t, get()),
	)
	if code != 200 || data(res)["projection"] != projectionRelease ||
		unitState(t, res, unit)["releaseState"] != statusRequested {
		t.Fatalf("override-only retry release: %d %v", code, res)
	}

	ackCommand(t, cmdOf(t, res, unit, "releaseCommandIds"), clock().Add(time.Second))

	if data(get())["state"] != statusReleased {
		t.Fatal("released after override")
	}

	var status string
	ownerScan(t, `SELECT status FROM billing.invoices WHERE id = $1`, []any{inv}, &status)

	if status != statusUnpaid {
		t.Error("override keeps the invoice unpaid")
	}
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRestrictionReconcileNotApplied(t *testing.T) {
	srv := server(t)
	unit := boundUnit(t, srv, "online")
	key, inv := overdueContract(t, srv, unit)
	restrictionID, _ := executed(t, srv, key, inv, []string{unit})
	get := func() map[string]any {
		_, m := post(srv, restrMgr(), "restrictions.get", `{"id":"`+restrictionID+`"}`)

		return m
	}

	expireAt(t, clock().Add(time.Minute))

	code, resp := write(
		srv,
		restrMgr(),
		"restrictions.exempt",
		`{"restrictionId":"`+restrictionID+`","until":"`+clock().Add(time.Hour).
			Format(time.RFC3339)+
			`","reason":"hospital"}`,
		ver(t, get()),
	)
	if code != 200 || asObj(t, data(resp)["exception"])["reason"] != "hospital" ||
		unitState(t, resp, unit)["releaseState"] != statusWaitingReconcile {
		t.Fatalf("exempt: %d %v", code, resp)
	}

	if _, res := post(
		srv,
		customerB(),
		"restrictions.forInvoice",
		`{"invoiceId":"`+inv+`","query":{"filters":{"status":"release_requested"}}}`,
	); len(items(t, res)) != 1 ||
		asObj(t, items(t, res)[0]["exception"])["reason"] != nil ||
		asObj(t, items(t, res)[0]["releaseIntent"])["actorMembershipId"] != "masked" {
		t.Fatalf("client mask: %v", res)
	}
	// query catalog: contractId => contractId, status => state; the invoice is the path, not a filter
	if _, res := post(
		srv,
		customerB(),
		"restrictions.forInvoice",
		`{"invoiceId":"`+inv+`","query":{"filters":{"contractId":"`+key+`"}}}`,
	); len(
		items(t, res),
	) != 1 {
		t.Errorf("forInvoice contract filter: %v", res)
	}

	if code, res := post(
		srv,
		customerB(),
		"restrictions.forInvoice",
		`{"invoiceId":"`+inv+`","query":{"filters":{"contractId":"`+uuid.NewString()+`"}}}`,
	); code != 200 ||
		len(items(t, res)) != 0 {
		t.Errorf("forInvoice other contract: %v", res)
	}

	code, resp = post(
		srv,
		customerB(),
		"restrictions.forInvoice",
		`{"invoiceId":"`+inv+`","query":{"filters":{"invoiceId":"`+uuid.NewString()+`"}}}`,
	)
	refusal(t, "forInvoice invoiceId filter", code, resp, "invoiceId=error.notAllowed")
	owner(
		t,
		`UPDATE assets.units SET last_seen_at = $2, observed_restriction = NULL WHERE id = $1`,
		unit,
		clock(),
	)

	code, resp = write(
		srv,
		restrMgr(),
		"restrictions.reconcile",
		`{"restrictionId":"`+restrictionID+`","unitIds":["`+unit+`"]}`,
		ver(t, get()),
	)
	if code != 200 || data(resp)["state"] != statusReleased ||
		unitState(t, resp, unit)["releaseState"] != "not_required" ||
		unitState(t, resp, unit)["applyState"] != statusNotApplied {
		t.Fatalf("reconcile not applied: %d %v", code, resp)
	}
	// the client reads the state changes only: the reconcile stays HQ's record (IR42)
	actions := func(events []any) map[string]bool {
		out := map[string]bool{}
		for _, e := range events {
			out[asStr(t, asObj(t, e)["action"])] = true
		}

		return out
	}

	_, clientView := post(
		srv,
		customerB(),
		"restrictions.forInvoice",
		`{"invoiceId":"`+inv+`","query":{}}`,
	)
	if hq := actions(
		asArr(t, data(get())["events"]),
	); !hq["restrictions.reconcile"] ||
		len(items(t, clientView)) != 1 {
		t.Fatalf("HQ history and client read: %v %v", hq, clientView)
	}

	if client := actions(
		asArr(t, items(t, clientView)[0]["events"]),
	); client["restrictions.reconcile"] ||
		!client["restrictions.exempt"] {
		t.Errorf("client history: %v", client)
	}
}

// TestRestrictionScheduleRefusals covers the contract and unit refusals of restrictions.schedule: a contract that is
// not restriction eligible, a contract without an overdue invoice, and an archived unit. The same request schedules
// once each cause is gone.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRestrictionScheduleRefusals(t *testing.T) {
	s := server(t)
	unit := boundUnit(t, s, "online")
	key, inv := overdueContract(t, s, unit)
	schedule := func() (int, map[string]any) {
		return write(
			s,
			restrMgr(),
			"restrictions.schedule",
			scheduleBody(key, inv, []string{unit}, nil),
			0,
		)
	}

	owner(t, `UPDATE billing.contracts SET restriction_eligible = false WHERE id = $1`, key)

	if code, m := schedule(); code != 422 ||
		asObj(t, m["fieldErrors"])["contractId"] != "errors.restriction_ineligible" {
		t.Errorf("ineligible contract: %d %v", code, m)
	}

	owner(t, `UPDATE billing.contracts SET restriction_eligible = true WHERE id = $1`, key)
	owner(
		t,
		`UPDATE billing.invoices SET due_at = $2 WHERE id = $1`,
		inv,
		clock().Add(48*time.Hour),
	)

	if code, m := schedule(); code != 409 || m["messageKey"] != "errors.no_overdue_invoice" {
		t.Errorf("nothing overdue: %d %v", code, m)
	}

	owner(t, `UPDATE billing.invoices SET due_at = $2 WHERE id = $1`, inv, clock().Add(-time.Hour))
	owner(t, `UPDATE assets.units SET archived = true WHERE id = $1`, unit)

	if code, m := schedule(); code != 409 || m["messageKey"] != "errors.unit_archived" {
		t.Errorf("archived unit: %d %v", code, m)
	}

	owner(t, `UPDATE assets.units SET archived = false WHERE id = $1`, unit)
	// a model without a temperature setpoint cannot carry a temperature limit
	var (
		model      string
		capVersion int
	)
	ownerScan(
		t,
		`SELECT model_id::text, capability_version FROM assets.units WHERE id = $1`,
		[]any{unit},
		&model,
		&capVersion,
	)

	fanOnly := uuid.NewString()
	owner(
		t,
		`INSERT INTO devices.capabilities (id, version, tenant_id, manufacturer, model, control, mode_control, fan_control,
			temperature)
		VALUES ($1, 1, $2, 'Demo', 'Fan only', true, false, true, NULL)`,
		fanOnly,
		seed.ID("tenant-a"),
	)
	owner(
		t,
		`UPDATE assets.units SET model_id = $2, capability_version = 1 WHERE id = $1`,
		unit,
		fanOnly,
	)

	code, res := schedule()

	owner(
		t,
		`UPDATE assets.units SET model_id = $2, capability_version = $3 WHERE id = $1`,
		unit,
		model,
		capVersion,
	)

	if code != 422 ||
		asObj(t, res["fieldErrors"])["unitIds"] != "errors.unsupported_capability" {
		t.Errorf("temperature limit on a fan-only model: %d %v", code, res)
	}

	if code, m := schedule(); code != 200 || data(m)["state"] != statusScheduled {
		t.Fatalf("schedule once the causes are gone: %d %v", code, m)
	}
}

// TestRestrictionReconcilePaths covers the other outcomes of restrictions.reconcile (SR26): a scheduled restriction has
// nothing to reconcile, a unit the device says carries no restriction becomes not applied (and is not reconciled
// twice), and a releasing unit that still carries the restriction gets its remove command at once.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRestrictionReconcilePaths(t *testing.T) {
	srv := server(t)
	reconcile := func(id, u string, version int) (int, map[string]any) {
		return write(
			srv,
			restrMgr(),
			"restrictions.reconcile",
			`{"restrictionId":"`+id+`","unitIds":["`+u+`"]}`,
			version,
		)
	}
	getter := func(id string) func() map[string]any {
		return func() map[string]any {
			_, m := post(srv, restrMgr(), "restrictions.get", `{"id":"`+id+`"}`)

			return m
		}
	}
	// scheduled: nothing has been sent yet
	unit1 := boundUnit(t, srv, "online")
	k1, inv1 := overdueContract(t, srv, unit1)
	_, resp := write(
		srv,
		restrMgr(),
		"restrictions.schedule",
		scheduleBody(k1, inv1, []string{unit1}, nil),
		0,
	)

	scheduled := asStr(t, data(resp)["id"])
	if code, res := reconcile(
		scheduled,
		unit1,
		ver(t, getter(scheduled)()),
	); code != 409 ||
		res["messageKey"] != "errors.restriction_state" {
		t.Errorf("reconcile a scheduled restriction: %d %v", code, res)
	}
	// the apply's answer was lost and the device carries no restriction: not applied, and once only
	unit2 := boundUnit(t, srv, "online")
	k2, inv2 := overdueContract(t, srv, unit2)
	id2, _ := executed(t, srv, k2, inv2, []string{unit2})
	get2 := getter(id2)

	expireAt(t, clock().Add(time.Minute))
	owner(
		t,
		`UPDATE assets.units SET last_seen_at = $2, observed_restriction = NULL WHERE id = $1`,
		unit2,
		clock(),
	)

	if code, res := reconcile(
		id2,
		unit2,
		ver(t, get2()),
	); code != 200 || unitState(t, res, unit2)["applyState"] != statusNotApplied ||
		unitState(t, res, unit2)["observedRestriction"] != nil {
		t.Fatalf("reconcile, nothing observed: %d %v", code, res)
	}

	if code, res := reconcile(
		id2,
		unit2,
		ver(t, get2()),
	); code != 422 ||
		asObj(t, res["fieldErrors"])["unitIds"] != "errors.reconcile_not_needed" {
		t.Errorf("reconcile twice: %d %v", code, res)
	}
	// releasing while the apply's answer is unknown, and the device still carries it: the remove goes out now
	unit3 := boundUnit(t, srv, "online")
	k3, inv3 := overdueContract(t, srv, unit3)
	id3, _ := executed(t, srv, k3, inv3, []string{unit3})
	get3 := getter(id3)

	expireAt(t, clock().Add(time.Minute))

	if code, res := write(
		srv,
		restrMgr(),
		"restrictions.exempt",
		`{"restrictionId":"`+id3+`","until":"`+clock().Add(time.Hour).
			Format(time.RFC3339)+
			`","reason":"hospital"}`,
		ver(t, get3()),
	); code != 200 ||
		unitState(t, res, unit3)["releaseState"] != statusWaitingReconcile {
		t.Fatalf("exempt: %d %v", code, res)
	}

	owner(
		t,
		`UPDATE assets.units SET last_seen_at = $2, observed_restriction = $3 WHERE id = $1`,
		unit3,
		clock(),
		`{"restrictionId":"`+id3+`","rulesVersion":"rules-1"}`,
	)

	code, resp := reconcile(id3, unit3, ver(t, get3()))
	if code != 200 || unitState(t, resp, unit3)["applyState"] != statusApplied ||
		unitState(t, resp, unit3)["releaseState"] != statusRequested ||
		len(asArr(t, unitState(t, resp, unit3)["releaseCommandIds"])) != 1 {
		t.Fatalf("reconcile while releasing: %d %v", code, resp)
	}

	ackCommand(t, cmdOf(t, resp, unit3, "releaseCommandIds"), clock().Add(2*time.Second))

	if data(get3())["state"] != statusReleased {
		t.Errorf("released after the remove: %v", data(get3())["state"])
	}
}

// TestRestrictionExplicitRelease: restrictions.release requests the release itself once every cause invoice is paid
// (IR35), on the current version only, and the audit log records it.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRestrictionExplicitRelease(t *testing.T) {
	srv := server(t)
	u := boundUnit(t, srv, "online")
	k, inv := overdueContract(t, srv, u)
	restrictionID, _ := executed(t, srv, k, inv, []string{u})
	get := func() map[string]any {
		_, m := post(srv, restrMgr(), "restrictions.get", `{"id":"`+restrictionID+`"}`)

		return m
	}
	release := func(v int) (int, map[string]any) {
		return write(
			srv,
			restrMgr(),
			"restrictions.release",
			`{"restrictionId":"`+restrictionID+`"}`,
			v,
		)
	}

	owner(
		t,
		`UPDATE billing.invoices SET status = 'paid' WHERE id = $1`,
		inv,
	) // paid outside the payment flow, which would release by itself

	version := ver(t, get())
	if code, m := release(
		version + 1,
	); code != 409 ||
		m["messageKey"] != apperr.KeyVersionConflict {
		t.Errorf("release on a stale version: %d %v", code, m)
	}

	if code, m := release(
		version,
	); code != 200 || data(m)["state"] != statusReleaseRequested ||
		ver(t, m) != version+1 {
		t.Fatalf("release with the causes paid: %d %v", code, m)
	}

	day := `"from":"` + clock().Add(-time.Hour).
		Format(time.RFC3339) +
		`","to":"` + clock().Add(time.Hour).
		Format(time.RFC3339) +
		`"`

	_, res := post(
		srv,
		restrMgr(),
		"audit.list",
		`{"filters":{`+day+`,"action":"restrictions.release","targetKind":"restriction",`+
			`"targetId":"`+restrictionID+`"},"limit":10}`,
	)
	if len(items(t, res)) != 1 {
		t.Errorf("audit of the release: %v", items(t, res))
	}
}

// TestRestrictionOnePerUnitAtOnce (DDC-08 §3): two RTO contracts may cover the same unit, but the unit holds one active
// restriction; scheduled from both contracts at the same moment, one succeeds and the other is CONFLICT
// errors.restriction_exists — the check and the insert of one wait for the other's.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRestrictionOnePerUnitAtOnce(t *testing.T) {
	srv := server(t)
	for round := range 3 {
		unit := boundUnit(t, srv, "online")
		k1, inv1 := overdueContract(t, srv, unit)
		k2, inv2 := overdueContract(t, srv, unit)
		codes := make(chan int, 2)

		var group sync.WaitGroup
		for _, pair := range [][2]string{{k1, inv1}, {k2, inv2}} {
			group.Add(1)
			go func(key, inv string) {
				defer group.Done()

				code, res := write(
					srv,
					restrMgr(),
					"restrictions.schedule",
					scheduleBody(key, inv, []string{unit}, nil),
					0,
				)
				if code == 409 && res["messageKey"] != "errors.restriction_exists" {
					t.Errorf("round %d: refused for another reason: %v", round, res)
				}

				codes <- code
			}(pair[0], pair[1])
		}

		group.Wait()
		close(codes)

		got := map[int]int{}
		for c := range codes {
			got[c]++
		}

		if got[200] != 1 || got[409] != 1 {
			t.Fatalf("round %d: one restriction per unit: %v", round, got)
		}
	}
}

// IR476: a restriction whose two cause invoices are paid at the same moment ends. Each payment checked for another
// unpaid cause invoice while the other payment was still open, so both saw one, both left the restriction, and the
// customer stayed restricted after paying everything. Five rounds of two manual payments sent at once.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRestrictionEndsWhenItsInvoicesArePaidAtOnce(t *testing.T) {
	srv := server(t)
	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }

	for round := range 5 {
		unit := boundUnit(t, srv, "online")
		key, inv1 := overdueContract(t, srv, unit)

		code, res := write(
			srv,
			hq(),
			"invoices.create",
			`{"amountMinor":5000,"currency":"MYR","contractId":"`+key+`","contractVersion":1,"period":{"from":"`+stamp(
				24*16,
			)+`","to":"`+stamp(
				24*46,
			)+`"},"dueAt":"`+stamp(
				24*47,
			)+`"}`,
			0,
		)
		if code != 200 {
			t.Fatalf("round %d: second invoice: %d %v", round, code, res)
		}

		inv2 := asStr(t, data(res)["id"])
		owner(
			t,
			`UPDATE billing.invoices SET due_at = $2 WHERE id = $1`,
			inv2,
			clock().Add(-time.Hour),
		)

		code, res = write(
			srv,
			restrMgr(),
			"restrictions.schedule",
			scheduleBody(
				key,
				inv1,
				[]string{unit},
				map[string]string{fieldCauseInvoiceIDs: `["` + inv1 + `","` + inv2 + `"]`},
			),
			0,
		)
		if code != 200 {
			t.Fatalf("round %d: schedule: %d %v", round, code, res)
		}

		restrictionID := asStr(t, data(res)["id"])
		bodies := map[string]int{}

		for _, inv := range []string{inv1, inv2} {
			_, m := post(srv, hq(), "invoices.get", `{"id":"`+inv+`"}`)
			bodies[`{"invoiceId":"`+inv+`","paymentReference":"BANK-`+inv[:8]+`","confirmedAmountMinor":5000,"currency":"MYR",`+
				`"reason":"bank transfer seen"}`] = ver(
				t,
				m,
			)
		}

		var group sync.WaitGroup
		for body, v := range bodies {
			group.Go(func() {
				if code, m := write(srv, hq(), "payments.recordManual", body, v); code != 200 {
					t.Errorf("round %d: payment: %d %v", round, code, m)
				}
			})
		}

		group.Wait()

		var state string
		ownerScan(
			t,
			`SELECT state FROM restrictions.restrictions WHERE id = $1`,
			[]any{restrictionID},
			&state,
		)

		if state != statusCancelled {
			t.Fatalf(
				"round %d: both cause invoices paid, the scheduled restriction is %s",
				round,
				state,
			)
		}
	}
}

// IR477: the apply answers of two units of one restriction, written at once — HQ's reconcile beside a device's answer,
// or two answers — make it applied. The restriction row was locked only after the unit's update, so the second writer
// waited and then judged "every unit applied" on what it had read before the first committed: its own unit applied,
// the other not yet, and the restriction stayed requested with both units applied. (A remove answer already locked the
// restriction first, in its recovery check.)
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestRestrictionAppliedWhenTwoUnitsAnswerAtOnce(t *testing.T) {
	srv := server(t)
	ctx := context.Background()
	unit1, unit2 := boundUnit(t, srv, "online"), boundUnit(t, srv, "online")
	k, inv := overdueContract(t, srv, unit1, unit2)
	_, res := write(
		srv,
		restrMgr(),
		"restrictions.schedule",
		scheduleBody(k, inv, []string{unit1, unit2}, nil),
		0,
	)
	restrictionID := asStr(t, data(res)["id"])
	due(t, restrictionID)

	code, res := write(
		srv,
		restrMgr(),
		"restrictions.execute",
		`{"restrictionId":"`+restrictionID+`","confirmedRulesVersion":"rules-1"}`,
		1,
	)
	if code != 200 || data(res)["state"] != statusRequested {
		t.Fatalf("execute: %d %v", code, res)
	}

	applies := make([]uuid.UUID, 0, 2)
	for _, u := range []string{unit1, unit2} {
		applies = append(
			applies,
			uuid.MustParse(asStr(t, asArr(t, unitState(t, res, u)["applyCommandIds"])[0])),
		)
	}

	conn := func() *pgx.Conn {
		conn, err := pgx.Connect(ctx, testDB("postgres"))
		if err != nil {
			t.Skip(err)
		}

		t.Cleanup(func() { _ = conn.Close(ctx) })

		return conn
	}
	c1, conn2 := conn(), conn()
	when := clock().Add(time.Second)

	tx1, err := c1.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}

	commandAcknowledgedErr := restrictions.CommandAcknowledged(ctx, tx1, applies[0], when)
	if commandAcknowledgedErr != nil {
		t.Fatal(commandAcknowledgedErr)
	}

	second := make(chan error, 1)

	go func() { // starts while the first answer is still open, and waits for it
		tx2, err := conn2.Begin(ctx)
		if err == nil {
			err = restrictions.CommandAcknowledged(ctx, tx2, applies[1], when)
			if err == nil {
				err = tx2.Commit(ctx)
			}
		}

		second <- err
	}()

	time.Sleep(300 * time.Millisecond)

	commitErr := tx1.Commit(ctx)
	if commitErr != nil {
		t.Fatal(commitErr)
	}

	opErr := <-second
	if opErr != nil {
		t.Fatal(opErr)
	}

	var state string
	ownerScan(
		t,
		`SELECT state FROM restrictions.restrictions WHERE id = $1`,
		[]any{restrictionID},
		&state,
	)

	if state != statusApplied {
		t.Fatalf("both units applied, the restriction is %s", state)
	}
}
