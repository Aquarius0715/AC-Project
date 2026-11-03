package integration_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func TestAutomationNotifications(t *testing.T) {
	srv := server(t)
	owner(t, `DELETE FROM control.evaluation_events`)
	unitID, lonely := newUnit(t, srv, "Notify AC"), newUnit(t, srv, "Lonely AC")
	cust := seed.ID("cust-b")
	policy := func(unit string, recipients []uuid.UUID) string {
		id := uuid.NewString()
		cond := jsonBytes(t,
			map[string]any{
				fieldMetric:            metricCO2,
				fieldOperator:          operatorAtLeast,
				fieldThreshold:         1000,
				fieldRecoveryThreshold: 900,
				fieldDurationSeconds:   60,
				fieldActiveWindow:      nil,
				fieldSeverity:          severityWarning,
			},
		)
		owner(
			t,
			`INSERT INTO monitoring.alert_policies (id, tenant_id, kind, customer_id, name, owner_membership_id,
				created_by_user_id, timezone, enabled, priority, condition,
			recipient_membership_ids, channels, escalate_after_minutes, cooldown_minutes) VALUES ($1,$2,'alert',$3,'CO2 high',$4,
			$5,'Asia/Kuala_Lumpur',true,50,$6,$7,'{inApp,email}',60,30)`,
			id,
			seed.ID("tenant-a"),
			cust,
			seed.ID("customer-b"),
			seed.ID("user-customer-b"),
			cond,
			recipients,
		)
		owner(
			t,
			`INSERT INTO assets.unit_alert_policies (tenant_id, unit_id, policy_id) VALUES ($1,$2,$3)`,
			seed.ID("tenant-a"),
			unit,
			id,
		)

		return id
	}
	withRecipient := policy(unitID, []uuid.UUID{seed.ID("customer-b")})
	noRecipient := policy(
		lonely,
		[]uuid.UUID{seed.ID("customer-a")},
	) // another customer's client cannot receive it
	def := defaultPolicyID(t)
	fact := func(unit, value string, observed time.Time, quality string) string {
		return `{"unitId":"` + unit + `","metric":"co2","value":` + value + `,"unit":"ppm","observedAt":"` + observed.Format(
			time.RFC3339,
		) + `","quality":"` + quality + `"}`
	}
	input := func(facts string) string {
		return `{"eventId":"` + uuid.NewString() + `","occurredAt":"` + clock().Format(
			time.RFC3339,
		) + `","unitIds":["` + unitID + `","` + lonely + `"],"facts":[` + facts + `],"phase":"condition"}`
	}
	outcome := func(m map[string]any, unit, pol string) map[string]any {
		for _, n := range asArr(t, data(m)["notifications"]) {
			if x := asObj(t, n); x["unitId"] == unit && x["policyId"] == pol {
				return x
			}
		}

		return nil
	}
	high := fact(unitID, "1200", clock(), "valid") + "," + fact(lonely, "1200", clock(), "valid")

	code, resp := post(srv, customerB(), "automations.simulate", input(high))
	if code != 200 || outcome(resp, unitID, withRecipient)["decision"] != decisionSelected ||
		outcome(resp, unitID, def)["decision"] != decisionSelected ||
		outcome(resp, lonely, noRecipient)["reason"] != "no_recipient" {
		t.Fatalf("simulate: %d %v", code, resp)
	}

	for _, r := range asArr(t, data(resp)["results"]) {
		if x := asObj(t, r); x["reason"] != "no_control_action" {
			t.Errorf("alert-only units: %v", x)
		}
	}

	if _, res := post(
		srv,
		customerB(),
		"automations.simulate",
		input(fact(unitID, "500", clock(), "valid")),
	); outcome(res, unitID, withRecipient)["reason"] != "not_due" {
		t.Errorf("below threshold: %v", res)
	}

	if _, res := post(
		srv,
		customerB(),
		"automations.simulate",
		input(fact(unitID, "1200", clock(), "suspect")),
	); outcome(res, unitID, withRecipient)["reason"] != "quality" {
		t.Errorf("quality: %v", res)
	}

	var before int
	ownerScan(
		t,
		`SELECT count(*) FROM notify.notifications WHERE target->>'id' = $1`,
		[]any{unitID},
		&before,
	)

	if before != 0 {
		t.Fatal("simulate saved notifications")
	}

	code, resp = write(srv, customerB(), "automations.fire", input(high), 0)

	created := outcome(resp, unitID, withRecipient)
	if code != 200 || created["decision"] != "created" ||
		len(asArr(t, created["notificationIds"])) != 2 ||
		len(asArr(t, created["failureIds"])) != 0 {
		t.Fatalf("fire: %d %v", code, resp)
	}

	failed := outcome(resp, lonely, noRecipient)
	if failed["decision"] != statusFailed || len(asArr(t, failed["failureIds"])) != 1 ||
		len(asArr(t, failed["notificationIds"])) != 0 {
		t.Errorf("no recipient: %v", failed)
	}

	var alerts, failures int
	ownerScan(
		t,
		`SELECT count(*), COALESCE(sum(jsonb_array_length(delivery_failures)), 0) FROM monitoring.alerts WHERE unit_id IN ($1,
			$2) AND status = 'open'`,
		[]any{unitID, lonely},
		&alerts,
		&failures,
	)

	if alerts < 2 || failures != 1 {
		t.Errorf("source alerts %d, failures %d", alerts, failures)
	}
	// a later event finds the unit's open alert of the policy again and records a failure of its own (SR12)
	var first string
	ownerScan(
		t,
		`SELECT id::text FROM monitoring.alerts WHERE unit_id = $1 AND policy_id = $2 AND status = 'open'`,
		[]any{lonely, noRecipient},
		&first,
	)
	owner(
		t,
		`DELETE FROM control.evaluation_events`,
	) // the test clock stands still: let the tick be fired again

	code, resp = write(
		srv,
		customerB(),
		"automations.fire",
		`{"eventId":"`+uuid.NewString()+`","occurredAt":"`+clock().Format(
			time.RFC3339,
		)+`","unitIds":["`+lonely+`"],"facts":[`+fact(
			lonely,
			"1300",
			clock(),
			"valid",
		)+`],"phase":"condition"}`,
		0,
	)
	if code != 200 {
		t.Fatalf("second event: %d %v", code, resp)
	}

	if again := outcome(
		resp,
		lonely,
		noRecipient,
	); again["decision"] != statusFailed ||
		len(asArr(t, again["failureIds"])) != 1 {
		t.Fatalf("second event: %v", again)
	}

	var open int
	ownerScan(
		t,
		`SELECT count(*), COALESCE(sum(jsonb_array_length(delivery_failures)), 0) FROM monitoring.alerts WHERE unit_id = $1
			AND policy_id = $2 AND status = 'open'`,
		[]any{lonely, noRecipient},
		&open,
		&failures,
	)

	var reused string
	ownerScan(
		t,
		`SELECT id::text FROM monitoring.alerts WHERE unit_id = $1 AND policy_id = $2 AND status = 'open'`,
		[]any{lonely, noRecipient},
		&reused,
	)

	if open != 1 || reused != first || failures != 2 {
		t.Errorf("one open alert with two failures: %d %s/%s %d", open, reused, first, failures)
	}
	// cooldown after a created notification (30 minutes); the default policy has none and notifies once per open alert,
	// so its alert of the unit, still open, sends nothing more either (IR600)
	if _, res := post(
		srv,
		customerB(),
		"automations.simulate",
		input(high),
	); outcome(res, unitID, withRecipient)["reason"] != "cooldown" ||
		outcome(res, unitID, def)["reason"] != "cooldown" {
		t.Errorf("cooldown: %v", res)
	}

	if _, res := post(
		srv,
		customerB(),
		"notifications.list",
		`{"filters":{"type":"fault"},"limit":100}`,
	); len(
		items(t, res),
	) == 0 {
		t.Error("customer receives the alert notification")
	}

	owner(t, `UPDATE monitoring.alert_policies SET enabled = false WHERE id = $1`, withRecipient)

	if _, res := post(
		srv,
		customerB(),
		"automations.simulate",
		input(high),
	); outcome(res, unitID, withRecipient)["reason"] != reasonDisabled {
		t.Errorf("disabled: %v", res)
	}
	// D02: a policy whose owner membership is gone notifies nobody, before any cooldown applies
	owner(
		t,
		`UPDATE monitoring.alert_policies SET enabled = true, owner_membership_id = $2 WHERE id = $1`,
		withRecipient,
		uuid.New(),
	)

	if _, res := post(
		srv,
		customerB(),
		"automations.simulate",
		input(high),
	); outcome(res, unitID, withRecipient)["reason"] != "owner_forbidden" {
		t.Errorf("owner gone: %v", outcome(res, unitID, withRecipient))
	}
	// IR115: the customer switched the default CO2 rule off, so the default policy no longer fires for its units
	owner(
		t,
		`INSERT INTO monitoring.default_rule_settings (tenant_id, policy_id, rule_key, customer_id, enabled,
			changed_by_membership_id) VALUES ($1, $2, 'ventilation_co2', $3, false, $4)`,
		seed.ID("tenant-a"),
		def,
		cust,
		seed.ID("customer-b"),
	)

	if code, res := post(
		srv,
		customerB(),
		"automations.simulate",
		input(high),
	); code != 200 ||
		outcome(res, unitID, def)["decision"] == decisionSelected {
		t.Errorf("default rule switched off: %v", outcome(res, unitID, def))
	}

	owner(
		t,
		`DELETE FROM monitoring.default_rule_settings WHERE policy_id = $1 AND customer_id = $2 AND rule_key =
			'ventilation_co2'`,
		def,
		cust,
	)
}

// D08 / IR66: fired facts past the recovery threshold resolve the open policy alerts of the unit — the attached
// policy's and the default rule's — with the recovery as evidence; a fact between the thresholds, a suspect one and a
// simulation change nothing, an alert without a policy stays open, and the next breach opens a new alert linked to the
// resolved one. Both policies have no cooldown, so each notifies once per open alert: a breach that goes on adds no
// notice, and the recurrence notifies again (IR600).
func TestAlertRecoveryResolvesPolicyAlerts(t *testing.T) {
	srv := server(t)
	owner(t, `DELETE FROM control.evaluation_events`)
	unit := newUnit(t, srv, "Recovery AC")
	pol := uuid.NewString()
	cond := jsonBytes(t,
		map[string]any{
			fieldMetric:            metricCO2,
			fieldOperator:          operatorAtLeast,
			fieldThreshold:         1000,
			fieldRecoveryThreshold: 900,
			fieldDurationSeconds:   60,
			fieldActiveWindow:      nil,
			fieldSeverity:          severityWarning,
		},
	)
	owner(
		t,
		`INSERT INTO monitoring.alert_policies (id, tenant_id, kind, customer_id, name, owner_membership_id,
			created_by_user_id, timezone, enabled, priority, condition,
		recipient_membership_ids, channels) VALUES ($1,$2,'alert',$3,'CO2 high',$4,$5,'Asia/Kuala_Lumpur',true,50,$6,$7,
		'{inApp}')`,
		pol,
		seed.ID("tenant-a"),
		seed.ID("cust-b"),
		seed.ID("customer-b"),
		seed.ID("user-customer-b"),
		cond,
		[]uuid.UUID{seed.ID("customer-b")},
	)
	owner(
		t,
		`INSERT INTO assets.unit_alert_policies (tenant_id, unit_id, policy_id) VALUES ($1,$2,$3)`,
		seed.ID("tenant-a"),
		unit,
		pol,
	)
	def := defaultPolicyID(t)
	manual := uuid.NewString() // an alert without a policy resolves only by hand (IR66)
	owner(
		t,
		`INSERT INTO monitoring.alerts (id, tenant_id, unit_id, customer_org_id, type, severity, status, cause_code,
			evidence_kind, evidence_text, observed_at, detected_at)
		VALUES ($1,$2,$3,$4,'tamper','warning','open','unknown','demo_observation','cover opened',$5,$5)`,
		manual,
		seed.ID("tenant-a"),
		unit,
		seed.ID("org-customer-b"),
		clock().Add(-time.Hour),
	)

	fire := func(value, quality string, simulate bool) string {
		owner(
			t,
			`DELETE FROM control.evaluation_events`,
		) // one result per tick (D02): each call stands for a later tick of the fixed test clock

		ev := uuid.NewString()
		body := `{"eventId":"` + ev + `","occurredAt":"` + clock().Format(
			time.RFC3339,
		) + `","unitIds":["` + unit + `"],"facts":[{"unitId":"` + unit + `","metric":"co2","value":` + value +
			`,"unit":"ppm","observedAt":"` + clock().Format(
			time.RFC3339,
		) + `","quality":"` + quality + `"}],"phase":"condition"}`

		var code int

		operation := "automations.fire"
		if simulate {
			operation = "automations.simulate"
			code, _ = post(srv, customerB(), operation, body)
		} else {
			code, _ = write(srv, customerB(), operation, body, 0)
		}

		if code != 200 {
			t.Fatalf("%s %s: %d", operation, value, code)
		}

		return ev
	}

	type row struct {
		id, status string
		reason     *string
		evidence   []uuid.UUID
		previous   *uuid.UUID
	}

	of := func(policy string) []row {
		ctx := context.Background()

		conn, err := pgx.Connect(ctx, testDB("postgres"))
		if err != nil {
			t.Skip(err)
		}
		defer func() { _ = conn.Close(ctx) }()
		// the test clock stands still, so a recurrence has its predecessor's times: the one it links to comes first
		rows, err := conn.Query(
			ctx,
			`SELECT id, status, resolution_reason, resolution_evidence_ids, previous_alert_id FROM monitoring.alerts WHERE
				unit_id = $1 AND policy_id = $2
			ORDER BY detected_at, created_at, previous_alert_id IS NOT NULL`,
			unit,
			policy,
		)
		if err != nil {
			t.Fatal(err)
		}
		defer rows.Close()

		var out []row

		for rows.Next() {
			var (
				r  row
				id uuid.UUID
			)

			err := rows.Scan(&id, &r.status, &r.reason, &r.evidence, &r.previous)
			if err != nil {
				t.Fatal(err)
			}

			r.id = id.String()
			out = append(out, r)
		}

		rowsErr := rows.Err()
		if rowsErr != nil {
			t.Fatal(rowsErr)
		}

		return out
	}
	told := func(policy string) int {
		var n int
		ownerScan(
			t,
			`SELECT count(*) FROM monitoring.alert_notifications WHERE unit_id = $1 AND policy_id = $2`,
			[]any{unit, policy},
			&n,
		)

		return n
	}

	fire("1200", "valid", false)

	first := of(pol)
	if len(first) != 1 || first[0].status != statusOpen || len(of(def)) != 1 {
		t.Fatalf(
			"the breach opens the policy's and the default rule's alerts: %v / %v",
			first,
			of(def),
		)
	}

	toldPol, toldDef := told(pol), told(def)
	if toldPol != 1 || toldDef < 1 {
		t.Fatalf("the breach notifies: policy %d, default %d", toldPol, toldDef)
	}

	fire("1250", "valid", false) // the breach goes on: the open alerts were told already

	if told(pol) != toldPol || told(def) != toldDef || len(of(pol)) != 1 {
		t.Errorf(
			"a breach that goes on notifies again: policy %d → %d, default %d → %d",
			toldPol,
			told(pol),
			toldDef,
			told(def),
		)
	}

	fire("950", "valid", false)   // between the thresholds
	fire("850", "suspect", false) // not a valid reading
	fire("850", "valid", true)    // a simulation saves nothing

	if r := of(pol); r[0].status != statusOpen {
		t.Fatalf("nothing recovers yet: %v", r)
	}

	ev := fire("850", "valid", false)

	r := of(pol)[0]
	if r.status != statusResolved || r.reason == nil ||
		*r.reason != "Recovered: co2 850 < 900 for 60 s (D08)" ||
		len(r.evidence) != 1 ||
		r.evidence[0].String() != ev {
		t.Fatalf("sustained recovery resolves with the recovery as evidence: %+v", r)
	}

	if d := of(def); d[0].status != statusResolved {
		t.Errorf("the default rule's alert recovers too: %+v", d)
	}

	_, res := post(srv, customerB(), "alerts.get", `{"id":"`+manual+`"}`)
	if data(res)["status"] != statusOpen {
		t.Errorf("an alert without a policy stays open: %v", data(res)["status"])
	}

	fire("1300", "valid", false)

	again := of(pol)
	if len(again) != 2 || again[1].status != statusOpen || again[1].previous == nil ||
		again[1].previous.String() != first[0].id {
		t.Fatalf("a recurrence opens a new alert linked to the resolved one: %+v", again)
	}

	if told(pol) != 2*toldPol || told(def) != 2*toldDef {
		t.Errorf("the recurrence notifies again: policy %d, default %d", told(pol), told(def))
	}

	_, res = post(srv, customerB(), "alerts.get", `{"id":"`+again[1].id+`"}`)
	if data(res)["previousAlertId"] != first[0].id {
		t.Errorf("alerts.get shows the link: %v", data(res)["previousAlertId"])
	}
}

// TestDemoAutomationNotifiesOnly: a gateway's facts (demo.trigger automation, IR529) open the unit's matching alert
// policy and notify, and a recovered reading resolves it, as the UI's fire does; but no automation is decided, so HQ's
// automation policy whose tariff condition holds sends nothing. heartbeat_gap, a contract Metric the default rule
// ac_offline watches (IR120), is taken. A schedule phase, an empty event and a report from the future are refused.
func TestDemoAutomationNotifiesOnly(t *testing.T) {
	srv := serverWith(t, true)
	unitID := newUnit(t, srv, "Gateway AC")
	_, sess := post(
		srv,
		customerB(),
		"session.get",
		`{}`,
	) // the business time of the server (a shared demo cluster may run ahead of clock)
	now, _ := time.Parse(time.RFC3339Nano, asStr(t, asObj(t, sess["meta"])["snapshotAt"]))
	tenant, cust := seed.ID("tenant-a"), seed.ID("cust-b")
	pol := uuid.NewString()
	cond := jsonBytes(t,
		map[string]any{
			fieldMetric:            metricTemperature,
			fieldOperator:          operatorAtLeast,
			fieldThreshold:         30,
			fieldRecoveryThreshold: 28,
			fieldDurationSeconds:   60,
			fieldActiveWindow:      nil,
			fieldSeverity:          severityWarning,
		},
	)
	owner(
		t,
		`INSERT INTO monitoring.alert_policies (id, tenant_id, kind, customer_id, name, owner_membership_id,
			created_by_user_id, timezone, enabled, priority, condition,
		recipient_membership_ids, channels, escalate_after_minutes, cooldown_minutes) VALUES ($1,$2,'alert',$3,'Too hot',$4,
		$5,'Asia/Kuala_Lumpur',true,50,$6,$7,'{inApp,email}',60,30)`,
		pol,
		tenant,
		cust,
		seed.ID("customer-b"),
		seed.ID("user-customer-b"),
		cond,
		[]uuid.UUID{seed.ID("customer-b")},
	)
	owner(
		t,
		`INSERT INTO assets.unit_alert_policies (tenant_id, unit_id, policy_id) VALUES ($1,$2,$3)`,
		tenant,
		unitID,
		pol,
	)
	// HQ's automation on the same unit sets it to 26 °C while the tariff is 0.5 MYR/kWh or more (reported below)
	if code, resp := write(
		srv,
		hq(),
		"policies.save",
		autoPolicyBody(map[string]string{fieldUnitIDs: `["` + unitID + `"]`}),
		0,
	); code != 200 {
		t.Fatalf("automation policy: %d %v", code, resp)
	}
	// the default rule ac_offline is on for the customer while the test runs
	def := defaultPolicyID(t)

	var was *bool
	ownerScan(
		t,
		`SELECT (SELECT enabled FROM monitoring.default_rule_settings WHERE policy_id = $1 AND rule_key = 'ac_offline' AND
			customer_id = $2)`,
		[]any{def, cust},
		&was,
	)
	owner(
		t,
		`INSERT INTO monitoring.default_rule_settings (tenant_id, policy_id, rule_key, customer_id, enabled,
			changed_by_membership_id) VALUES ($1,$2,'ac_offline',$3,true,$4)
		ON CONFLICT (policy_id, rule_key, customer_id) DO UPDATE SET enabled = true`,
		tenant,
		def,
		cust,
		seed.ID("customer-b"),
	)
	t.Cleanup(func() {
		if was == nil {
			owner(
				t,
				`DELETE FROM monitoring.default_rule_settings WHERE policy_id = $1 AND rule_key = 'ac_offline' AND customer_id =
					$2`,
				def,
				cust,
			)
		} else {
			owner(
				t,
				`UPDATE monitoring.default_rule_settings SET enabled = $3 WHERE policy_id = $1 AND rule_key = 'ac_offline' AND
					customer_id = $2`,
				def,
				cust,
				*was,
			)
		}
	})

	report := func(phase, facts string, at time.Time) (int, map[string]any) {
		ev := `{"eventId":"` + uuid.NewString() + `","occurredAt":"` + at.Format(
			time.RFC3339,
		) + `","unitIds":["` + unitID + `"],"facts":[` + facts + `],"phase":"` + phase + `"}`

		return write(
			srv,
			customerB(),
			"demo.trigger",
			`{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+at.Format(
				time.RFC3339,
			)+`","eventType":"automation","event":`+ev+`}`,
			0,
		)
	}
	fact := func(metric, value, unit string, at time.Time) string {
		return `{"unitId":"` + unitID + `","metric":"` + metric + `","value":` + value + `,"unit":"` + unit +
			`","observedAt":"` +
			at.Format(
				time.RFC3339,
			) + `","quality":"valid"}`
	}
	alerts := func(rule string, status string) int {
		var n int
		if rule == "" {
			ownerScan(
				t,
				`SELECT count(*) FROM monitoring.alerts WHERE unit_id = $1 AND policy_id = $2 AND status = $3`,
				[]any{unitID, pol, status},
				&n,
			)
		} else {
			ownerScan(
				t,
				`SELECT count(*) FROM monitoring.alerts WHERE unit_id = $1 AND rule_key = $2 AND status = $3`,
				[]any{unitID, rule, status},
				&n,
			)
		}

		return n
	}

	when := now.Add(-30 * time.Second)
	if code, m := report(
		"condition",
		fact("temperature", "31.5", "°C", when)+","+fact("tariff", "0.6", "MYR_per_kWh", when),
		when,
	); code != 200 {
		t.Fatalf("hot report: %d %v", code, m)
	}

	if alerts("", "open") != 1 {
		t.Error("the matching policy opened no alert")
	}

	var notes, commands, runs int
	ownerScan(
		t,
		`SELECT count(*) FROM notify.notifications WHERE target->>'id' = $1`,
		[]any{unitID},
		&notes,
	)
	ownerScan(
		t,
		`SELECT count(*) FROM control.commands WHERE unit_id = $1`,
		[]any{unitID},
		&commands,
	)
	ownerScan(
		t,
		`SELECT count(*) FROM control.automation_runs WHERE unit_id = $1`,
		[]any{unitID},
		&runs,
	)

	if notes == 0 || commands != 0 || runs != 0 {
		t.Errorf(
			"%d notifications, %d commands, %d automation runs; want notifications and no automation decided",
			notes,
			commands,
			runs,
		)
	}

	later := now.Add(-10 * time.Second)
	if code, m := report("condition", fact("temperature", "27", "°C", later), later); code != 200 {
		t.Fatalf("cool report: %d %v", code, m)
	}

	if alerts("", "open") != 0 || alerts("", "resolved") != 1 {
		t.Errorf(
			"a recovered reading left %d alerts open, %d resolved",
			alerts("", "open"),
			alerts("", "resolved"),
		)
	}

	if code, m := report(
		"condition",
		fact("heartbeat_gap", "20", "min", later),
		later,
	); code != 200 {
		t.Fatalf("gap report: %d %v", code, m)
	}

	if alerts("ac_offline", "open") != 1 {
		t.Error("20 minutes without a heartbeat opened no AC offline alert")
	}

	code, res := report("schedule_start", fact("temperature", "31", "°C", when), when)
	refusal(t, "schedule phase", code, res, "event.phase=error.invalid")
	code, res = write(
		srv,
		customerB(),
		"demo.trigger",
		`{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+when.Format(
			time.RFC3339,
		)+`","eventType":"automation"}`,
		0,
	)
	refusal(t, "no event", code, res, "event=error.required")

	future := now.Add(time.Hour)
	code, res = report("condition", fact("temperature", "31", "°C", future), future)
	refusal(t, "future", code, res, "event.occurredAt=error.range")
	// the UI's fire of the same facts decides the automation (here skipped: the unit has no module), so the runs check
	// above tells the two apart
	owner(t, `DELETE FROM control.evaluation_events`)

	if code, resp := write(
		srv,
		hq(),
		"automations.fire",
		`{"eventId":"`+uuid.NewString()+`","occurredAt":"`+now.Format(
			time.RFC3339,
		)+`","unitIds":["`+unitID+`"],"facts":[`+
			fact(
				"tariff",
				"0.6",
				"MYR_per_kWh",
				now,
			)+`],"phase":"condition"}`,
		0,
	); code != 200 {
		t.Fatalf("fire: %d %v", code, resp)
	}

	ownerScan(
		t,
		`SELECT count(*) FROM control.automation_runs WHERE unit_id = $1`,
		[]any{unitID},
		&runs,
	)

	if runs == 0 {
		t.Error("the UI's fire left no automation run, so the check above proves nothing")
	}
}
