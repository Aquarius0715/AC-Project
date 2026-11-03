package integration

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

func seedID(s string) uuid.UUID { return seed.ID(s) }

func count(m map[string]any, k string) int {
	return int(data(m)["counts"].(map[string]any)[k].(float64))
}

func TestSummariesCustomer(t *testing.T) {
	srv := server(t)

	code, resp := post(srv, &customerA, "summaries.get", `{"kind":"customer","filters":{}}`)
	if code != 200 || data(resp)["kind"] != "customer" {
		t.Fatalf("customer: %d %v", code, resp)
	}

	_, all := post(srv, &customerA, "units.list", `{"limit":1}`)

	_, on := post(srv, &customerA, "units.list", `{"filters":{"status":"online"},"limit":1}`)
	if count(resp, "total") != int(asNum(t, data(all)["total"])) ||
		count(resp, "online") != int(asNum(t, data(on)["total"])) {
		t.Errorf(
			"counts differ from units.list: %v / %v / %v",
			resp,
			data(all)["total"],
			data(on)["total"],
		)
	}

	if count(
		resp,
		"total",
	) != count(
		resp,
		"online",
	)+count(
		resp,
		"offline",
	)+count(
		resp,
		"unknown",
	) ||
		count(
			resp,
			"total",
		) != count(
			resp,
			"powerOn",
		)+count(
			resp,
			"powerOff",
		)+count(
			resp,
			"powerUnknown",
		) {
		t.Errorf("partitions: %v", resp)
	}

	if count(resp, "offerCount") != 0 || count(resp, "assignedCount") != 0 {
		t.Error("job counts are zero for customers")
	}

	var alerts int
	ownerScan(
		t,
		`SELECT count(*) FROM monitoring.alerts a JOIN assets.units u ON u.id = a.unit_id WHERE u.customer_org_id = (SELECT
			organization_id FROM identity.memberships WHERE id = $1)
		AND NOT u.archived AND a.status IN ('open','acknowledged') AND a.severity IN ('critical','warning')`,
		[]any{seedID("customer-a")},
		&alerts,
	)

	if count(resp, "alertCount") != alerts {
		t.Errorf("alertCount %d want %d", count(resp, "alertCount"), alerts)
	}

	unit := seedID("unit-online-rto").String()
	if _, res := post(
		srv,
		&customerA,
		"summaries.get",
		`{"kind":"customer","filters":{"unitId":"`+unit+`"}}`,
	); count(
		res,
		"total",
	) != 1 {
		t.Errorf("unit filter: %v", res)
	}

	if _, res := post(
		srv,
		&customerA,
		"summaries.get",
		`{"kind":"customer","filters":{"unitIds":[]}}`,
	); count(
		res,
		"total",
	) != 0 {
		t.Error("empty unitIds")
	}

	want := map[string]string{ // the field each case is refused for
		"{\"kind\":\"customer\",\"filters\":{\"overdueOnly\":true}}":    "filters=error.invalid",
		"{\"kind\":\"customer\",\"filters\":{\"status\":\"assigned\"}}": "filters=error.invalid",
		"{\"kind\":\"shop\",\"filters\":{}}":                            "kind=error.invalid",
	}

	for _, b := range []string{`{"kind":"customer",` +
		`"filters":{"status":"assigned"}}`, `{"kind":"customer",` +
		`"filters":{"overdueOnly":true}}`, `{"kind":"shop","filters":{}}`} {
		code, m := post(srv, &customerA, "summaries.get", b)
		refusal(t, b, code, m, want[b])
	}

	if code, _ := post(
		srv,
		&customerA,
		"summaries.get",
		`{"kind":"partner","filters":{}}`,
	); code != 403 {
		t.Error("client asks for partner summary")
	}

	if code, _ := post(srv, &hq, "summaries.get", `{"kind":"customer","filters":{}}`); code != 403 {
		t.Error("HQ summaries.get")
	}
}

func TestSummariesTechnicianPartner(t *testing.T) {
	srv := server(t)
	unit := boundUnit(t, srv, "online")
	job := assign(t, unit, "tech-external-b", clock.Add(-time.Hour), clock.Add(time.Hour), "active")
	owner(
		t,
		`UPDATE maintenance.assignments SET created_at = $2 WHERE job_id = $1`,
		job,
		clock.Add(-2*time.Hour),
	)
	owner(
		t,
		`UPDATE maintenance.jobs j SET assignment_id = a.id, scheduled_slot = a.scheduled FROM maintenance.assignments a WHERE
			a.job_id = j.id AND j.id = $1`,
		job,
	)
	owner(
		t,
		`INSERT INTO maintenance.offers (tenant_id, job_id, contractor_org_id, terms_version, visit_slot, offered_at,
			offer_expires_at, decision, decided_at, access_valid_from, access_valid_until)
		SELECT tenant_id, id, $2, 'terms-1', requested_slot, $3, $4, 'accept', $3, $3, $4 FROM maintenance.jobs WHERE id =
		$1`,
		job,
		seedID("org-contractor-b"),
		clock.Add(-3*time.Hour),
		clock.Add(24*time.Hour),
	)
	owner(
		t,
		`UPDATE maintenance.jobs SET contractor_org_id = $2 WHERE id = $1`,
		job,
		seedID("org-contractor-b"),
	)

	code, resp := post(srv, &techB, "summaries.get", `{"kind":"technician","filters":{}}`)
	if code != 200 || count(resp, "assignedCount") < 1 || count(resp, "scheduledCount") < 1 ||
		count(resp, "total") < 1 ||
		count(resp, "powerOn") != 0 ||
		count(resp, "alertCount") != 0 {
		t.Fatalf("technician: %d %v", code, resp)
	}

	_, m2 := post(
		srv,
		&techB,
		"summaries.get",
		`{"kind":"technician","filters":{"unitIds":["`+unit+`"],"status":"assigned"}}`,
	)
	if count(m2, "assignedCount") != 1 || count(m2, "total") != 1 || count(m2, "online") != 1 {
		t.Errorf("technician filtered: %v", m2)
	}

	_, list := post(
		srv,
		&techB,
		"jobs.list",
		`{"filters":{"unitIds":["`+unit+`"],"status":"assigned"}}`,
	)
	if int(asNum(t, data(list)["total"])) != count(m2, "assignedCount") {
		t.Error("summary and list disagree")
	}

	if code, res := post(
		srv,
		&techB,
		"summaries.get",
		`{"kind":"technician","filters":{"status":"assigned","statuses":["assigned"]}}`,
	); code != 422 ||
		fields(res) != "filters.statuses=error.invalid" {
		t.Errorf("status with statuses: %d %s", code, fields(res))
	}

	if code, res := post(
		srv,
		&techB,
		"summaries.get",
		`{"kind":"technician","filters":{"statuses":[]}}`,
	); code != 422 ||
		fields(res) != "filters.statuses=error.invalid" {
		t.Errorf("empty statuses: %d %s", code, fields(res))
	}

	code, resp = post(
		srv,
		&contrB,
		"summaries.get",
		`{"kind":"partner","filters":{"unitIds":["`+unit+`"]}}`,
	)
	if code != 200 || count(resp, "activeCount") != 1 || count(resp, "assignedCount") != 1 ||
		count(resp, "total") != 0 {
		t.Fatalf("partner: %d %v", code, resp)
	}

	owner(t, `UPDATE maintenance.jobs SET due_at = $2 WHERE id = $1`, job, clock.Add(-time.Minute))

	if _, res := post(
		srv,
		&contrB,
		"summaries.get",
		`{"kind":"partner","filters":{"unitIds":["`+unit+`"],"overdueOnly":true}}`,
	); count(
		res,
		"overdueCount",
	) != 1 {
		t.Errorf("overdue: %v", res)
	}

	if code, _ := post(
		srv,
		&contrB,
		"summaries.get",
		`{"kind":"technician","filters":{}}`,
	); code != 403 {
		t.Error("contractor asks for technician summary")
	}
}

// D07 / IR26: the job counts of a summary — scheduled (accepted or assigned), in progress, review (submitted), active
// (scheduled or in progress), overdue (due before now while open) and assigned (with an active Assignment) — leave
// completed and cancelled jobs out, and the technician's equipment counts its units once.
func TestSummaryJobCounts(t *testing.T) {
	srv := server(t)
	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`,
		seedID("tech-internal-a"),
	)
	unit := boundUnit(t, srv, "online")
	job := func(h int, status string, due time.Time) {
		j := assign(
			t,
			unit,
			"tech-internal-a",
			clock.Add(time.Duration(h)*time.Hour),
			clock.Add(time.Duration(h+1)*time.Hour),
			"active",
		) // windows do not overlap
		owner(
			t,
			`UPDATE maintenance.assignments SET created_at = $2 WHERE job_id = $1`,
			j,
			clock.Add(-2*time.Hour),
		)
		owner(
			t,
			`UPDATE maintenance.jobs j SET status = $2, due_at = $3, assignment_id = a.id, scheduled_slot = a.scheduled FROM
				maintenance.assignments a
			WHERE a.job_id = j.id AND j.id = $1`,
			j,
			status,
			due,
		)
	}
	job(0, "assigned", clock.Add(-time.Minute)) // overdue
	job(1, "in_progress", clock.Add(24*time.Hour))
	job(2, "submitted", clock.Add(24*time.Hour))
	job(3, "completed", clock.Add(-time.Minute)) // done: neither counted nor overdue
	job(4, "cancelled", clock.Add(-time.Minute))

	code, res := post(
		srv,
		&techInt,
		"summaries.get",
		`{"kind":"technician","filters":{"unitIds":["`+unit+`"]}}`,
	)
	if code != 200 {
		t.Fatalf("summary: %d %v", code, res)
	}

	for k, want := range map[string]int{
		"scheduledCount": 1, "inProgressCount": 1, "reviewCount": 1, "activeCount": 2,
		"overdueCount": 1, "assignedCount": 3, "total": 1, "online": 1,
	} {
		if got := count(res, k); got != want {
			t.Errorf("%s: %d, want %d", k, got, want)
		}
	}
}
