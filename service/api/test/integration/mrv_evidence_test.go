package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// IR349: attachments.list offers the evidence an MRV report may cite (DD-A14 item 6, IR203): the ready attachments of
// accepted work reports on the units, uploaded in [from, to), newest first; HQ with mrv.write or job.read only.
func TestMRVEvidenceAttachments(t *testing.T) {
	srv := server(t)
	tenant := seed.ID("tenant-a")
	unit, other := newUnit(t, srv, "MRV evidence AC"), newUnit(t, srv, "MRV other AC")
	from, until := clock().Add(-48*time.Hour), clock().Add(-24*time.Hour)
	input := from.Add(2 * time.Hour)

	var jobs []string

	job := func(unitID string) string {
		id := uuid.NewString()
		owner(
			t,
			`INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at,
				completed_at)
			VALUES ($1,$2,$3,$4,'reactive','completed','client_request',tstzrange($5,$6),$6,$6)`,
			id,
			tenant,
			unitID,
			seed.ID("org-customer-b"),
			input.Add(-time.Hour),
			input,
		)
		jobs = append(jobs, id)

		return id
	}
	report := func(j, state string) string {
		id := uuid.NewString()

		var accepted any
		if state == "accepted" {
			accepted = input.Add(time.Hour)
		}

		owner(
			t,
			`INSERT INTO maintenance.work_reports (id, version, tenant_id, job_id, author_id, state, accepted_at) VALUES ($1, 1,
				$2, $3, $4, $5, $6)`,
			id,
			tenant,
			j,
			uuid.New(),
			state,
			accepted,
		)

		return id
	}
	attach := func(j, r, name, status string, when time.Time) string {
		id := uuid.NewString()
		owner(
			t,
			`INSERT INTO maintenance.attachments (id, tenant_id, job_id, report_id, object_key, name, mime, size_bytes, status,
				uploaded_by, created_at, updated_at)
			VALUES ($1,$2,$3,$4,$5,$6,'image/png',1,$7,$8,$9,$9)`,
			id,
			tenant,
			j,
			r,
			"test/"+id,
			name,
			status,
			uuid.New(),
			when,
		)

		return id
	}

	t.Cleanup(func() { // later tests see none of these jobs
		owner(t, `DELETE FROM maintenance.attachments WHERE job_id = ANY($1::uuid[])`, jobs)
		owner(t, `DELETE FROM maintenance.work_reports WHERE job_id = ANY($1::uuid[])`, jobs)
		owner(t, `DELETE FROM maintenance.jobs WHERE id = ANY($1::uuid[])`, jobs)
	})

	j1 := job(unit)
	r1 := report(j1, "accepted")
	meter := attach(j1, r1, "Meter.png", "ready", input)
	panel := attach(j1, r1, "panel.png", "ready", input.Add(30*time.Minute))
	attach(j1, r1, "scanning.png", "processing", input.Add(10*time.Minute)) // not ready
	attach(j1, r1, "old.png", "ready", from.Add(-time.Minute))              // before the period
	attach(
		j1,
		r1,
		"edge.png",
		"ready",
		until,
	) // at its end: outside [from, to)

	j2 := job(unit)
	attach(
		j2,
		report(j2, "submitted"),
		"unaccepted.png",
		"ready",
		input,
	) // the report is not accepted

	j3 := job(other)
	elsewhere := attach(j3, report(j3, "accepted"), "elsewhere.png", "ready", input) // another unit

	body := func(extra string, units ...string) string {
		return `{"unitIds":["` + strings.Join(
			units,
			`","`,
		) + `"],"from":"` + from.Format(
			time.RFC3339,
		) + `","to":"` + until.Format(
			time.RFC3339,
		) + `","query":{` + extra + `}}`
	}
	ids := func(m map[string]any) []string {
		rows := items(t, m)

		out := make([]string, 0, len(rows))
		for _, it := range rows {
			out = append(out, asStr(t, it["id"]))
		}

		return out
	}

	code, resp := post(srv, hq(), "attachments.list", body("", unit))
	if code != 200 {
		t.Fatalf("list: %d %v", code, resp)
	}

	if got := ids(resp); len(got) != 2 || got[0] != panel || got[1] != meter { // newest first
		t.Fatalf("candidates: %v (panel %s, meter %s)", got, panel, meter)
	}

	first := items(t, resp)[1]
	if first["unitId"] != unit || first["reportId"] == nil || first["acceptedAt"] == nil ||
		first["status"] != "ready" ||
		first["name"] != "Meter.png" ||
		first["mime"] != "image/png" {
		t.Errorf("candidate fields: %v", first)
	}
	// both units; by name (case-insensitive) and a page of one
	if code, res := post(
		srv,
		hq(),
		"attachments.list",
		body(`"sort":{"field":"name","direction":"asc"}`, unit, other),
	); code != 200 ||
		strings.Join(ids(res), ",") != strings.Join([]string{elsewhere, meter, panel}, ",") {
		t.Errorf("by name: %d %v", code, ids(res))
	}

	if code, res := post(
		srv,
		hq(),
		"attachments.list",
		body(`"limit":1`, unit, other),
	); code != 200 || len(items(t, res)) != 1 || data(res)["total"] != float64(3) ||
		data(res)["nextCursor"] == nil {
		t.Errorf("first page: %d %v", code, res)
	}
	// the input rules, each refused for its field
	day := 24 * time.Hour
	for b, want := range map[string]string{
		`{"unitIds":[],` +
			`"from":"` + from.Format(time.RFC3339) + `","to":"` + until.Format(time.RFC3339) +
			`","query":{}}`: "unitIds=error.invalid",
		`{"unitIds":["` + unit + `","` + unit + `"],"from":"` + from.Format(time.RFC3339) + `","to":"` +
			until.Format(time.RFC3339) + `","query":{}}`: "unitIds=error.invalid",
		`{"unitIds":["` + unit + `"],"to":"` + until.Format(time.RFC3339) +
			`","query":{}}`: "from=error.required",
		`{"unitIds":["` + unit + `"],"from":"` + from.Format(time.RFC3339) +
			`","query":{}}`: "to=error.required",
		`{"unitIds":["` + unit + `"],"from":"` + until.Format(time.RFC3339) + `","to":"` + from.Format(time.RFC3339) +
			`","query":{}}`: "to=error.range",
		`{"unitIds":["` + unit + `"],"from":"` + from.Format(time.RFC3339) + `","to":"` +
			from.Add(367*day).Format(time.RFC3339) + `","query":{}}`: "to=error.range",
		body(`"filters":{"x":1}`, unit): "x=error.notAllowed",
	} {
		code, m := post(srv, hq(), "attachments.list", b)
		refusal(t, b, code, m, want)
	}
	// only HQ: a client and a contractor are refused
	for _, a := range []*actor{customerB(), contrA()} {
		if code, _ := post(srv, a, "attachments.list", body("", unit)); code != 403 {
			t.Errorf("%s lists evidence: %d", a.membership, code)
		}
	}
}
