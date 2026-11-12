package integration_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

const installationType = `"installation"`

// installDraft builds the draft of an installation report: the seven checklist items with the result, the confirmed
// equipment and the photos named.
func installDraft(job, report, result, model, serial string, photos []string) string {
	group := maintenance.InstallationKeys()
	items := make([]string, 0, len(group))

	for _, key := range group {
		items = append(
			items,
			`{"componentGroup":"installation","componentKey":"`+key+`","result":"`+result+
				`","reason":null,"evidenceIds":[]}`,
		)
	}

	body := `{"jobId":"` + job + `",`
	if report != "" {
		body += `"reportId":"` + report + `",`
	}

	return body + `"items":[` + strings.Join(
		items,
		",",
	) + `],"measurements":[],"parts":[],"refrigerant":[],` +
		`"workText":"` + strings.Repeat(
		"w",
		50,
	) + `","nextAction":{"kind":"none"},` +
		`"installation":{"confirmedModelId":"` + model + `","serial":"` + serial + `"},` +
		`"attachmentIds":` + idList(
		photos,
	) + `}`
}

// TestInstallationFlow (DEC-106, IR789): HQ plans a unit and creates its installation job; the technician reports the
// installation; HQ accepts and the unit is installed.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestInstallationFlow(t *testing.T) {
	srv := server(t)
	unitOrg, prop, model := installBase(t)

	name := "New split AC " + uuid.NewString()[:8]

	save := func(extra string) (int, map[string]any) {
		return write(
			srv,
			hq(),
			"units.save",
			`{"customerOrgId":"`+unitOrg+`","propertyId":"`+prop+`","spaceId":null,"displayName":"`+name+
				`","modelId":"`+model+`","type":"split","serviceScope":["indoor","outdoor"]`+extra+`}`,
			0,
		)
	}

	// a planned unit has no installation date and no warranty end
	code, res := save(
		`,"installState":"planned","installedAt":"` + clock().Add(-time.Hour).
			Format(time.RFC3339) +
			`"`,
	)
	refusal(t, "planned with an installation date", code, res, "installedAt="+apperr.KeyNotAllowed)

	code, res = save(`,"installState":"unknown","installedAt":null`)
	refusal(t, "unknown state", code, res, "installState="+apperr.KeyInvalid)

	code, res = save(`,"installState":"planned","installedAt":null`)
	if code != 200 || data(res)["installState"] != "planned" || data(res)["installedAt"] != nil {
		t.Fatalf("plan a unit: %d %v", code, res)
	}

	unit := asStr(t, data(res)["id"])

	create := func(who *actor, unitID, kind string) (int, map[string]any) {
		return write(srv, who, "jobs.create", jobBody(unitID, map[string]string{
			fieldType: kind, fieldAlternativeSlots: "[]",
		}), 0)
	}

	// a unit that is not installed yet has no maintenance to plan
	code, res = write(
		srv,
		hq(),
		"plans.save",
		`{"unitId":"`+unit+`","recurrence":{"kind":"monthly","intervalMonths":3},`+
			`"nextDueAt":"`+clock().Add(48*time.Hour).Format(time.RFC3339)+`"}`,
		0,
	)
	refusal(t, "a plan for a planned unit", code, res, "unitId="+apperr.KeyInvalidState)

	code, res = create(hq(), unit, `"reactive"`)
	refusal(t, "another job on a planned unit", code, res, "type="+apperr.KeyInvalidState)

	code, res = create(hq(), seed.ID("unit-non-rto").String(), installationType)
	refusal(t, "installation of an installed unit", code, res, "type="+apperr.KeyInvalidState)

	code, res = create(customerA(), unit, installationType)
	if code == 200 {
		t.Errorf("a client requested an installation: %v", res)
	}

	code, res = create(hq(), unit, installationType)
	if code != 200 || data(res)["type"] != "installation" || data(res)["origin"] != "hq_request" {
		t.Fatalf("installation job: %d %v", code, res)
	}

	job := asStr(t, data(res)["id"])

	if code, _ = create(hq(), unit, installationType); code != 409 {
		t.Errorf("a second unfinished installation job: %d", code)
	}

	owner(
		t,
		`UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'
		AND scheduled && tstzrange($2, $3)`,
		seed.ID("tech-internal-a"),
		clock().Add(-2*time.Hour),
		clock().Add(30*time.Hour),
	)

	// the technician works in the unit's scope, and starts at the site inside the work window
	owner(
		t,
		`INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES ($1, $2, 'unit', $3)
		ON CONFLICT DO NOTHING`,
		seed.ID("tenant-a"),
		seed.ID("tech-internal-a"),
		unit,
	)
	t.Cleanup(func() {
		owner(t, `DELETE FROM identity.membership_scopes WHERE membership_id = $1 AND ref_id = $2`,
			seed.ID("tech-internal-a"), unit)
	})

	stamp := func(h int) string { return clock().Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	if code, res = write(srv, hq(), "jobs.assign", `{"jobId":"`+job+`","technicianMembershipId":"`+
		seed.ID("tech-internal-a").
			String()+
		`","startAt":"`+stamp(25)+`","endAt":"`+stamp(27)+`"}`, 1); code != 200 {
		t.Fatalf("assign: %d %v", code, res)
	}

	owner(
		t,
		`UPDATE maintenance.assignments SET valid_from = $2, valid_until = $3, scheduled = tstzrange($2, $3),
		created_at = $2 WHERE job_id = $1 AND status = 'active'`,
		job,
		clock().Add(-time.Hour),
		clock().Add(time.Hour),
	)
	drainEvents(t)

	if code, res = write(
		srv,
		techInt(),
		"jobs.start",
		`{"jobId":"`+job+`","startConfirmed":true}`,
		2,
	); code != 200 {
		t.Fatalf("start: %d %v", code, res)
	}

	code, res = write(
		srv,
		techInt(),
		"jobs.saveDraft",
		installDraft(job, "", "normal", model, "", nil),
		0,
	)
	if code != 200 || len(asArr(t, data(res)["items"])) != 7 {
		t.Fatalf("installation draft: %d %v", code, res)
	}

	report := asStr(t, data(res)["id"])

	// everything an installation report needs is named by its own field
	code, res = write(srv, techInt(), "jobs.submit", `{"jobId":"`+job+`","reportVersion":1}`, 3)
	for field, key := range map[string]string{
		"installation":  apperr.KeyRequired,
		"attachmentIds": "errors.photos_required",
		"signOff":       apperr.KeyRequired,
	} {
		if fe, _ := res["fieldErrors"].(map[string]any); code != 422 || fe[field] != key {
			t.Errorf("submit without %s: %d %v", field, code, res)
		}
	}

	photos := make([]string, 0, 3)
	img := tinyPNG(t)
	version := 1

	for _, tag := range []string{"name_plate", "indoor_unit", "outdoor_unit"} {
		code, res = write(
			srv,
			techInt(),
			"attachments.add",
			`{"jobId":"`+job+`","reportId":"`+report+`","tag":"`+tag+
				`","file":`+blobJSON(tag+".png", "image/png", img, len(img))+`}`,
			version,
		)
		if code != 200 || data(res)["tag"] != tag {
			t.Fatalf("photo %s: %d %v", tag, code, res)
		}

		photos = append(photos, asStr(t, data(res)["id"]))
		version++
	}

	code, res = write(srv, techInt(), "attachments.add", `{"jobId":"`+job+`","reportId":"`+report+
		`","tag":"selfie","file":`+blobJSON("a.png", "image/png", img, len(img))+`}`, version)
	refusal(t, "unknown photo tag", code, res, "tag="+apperr.KeyInvalid)

	// the wrong model is refused, the unit's own model is accepted
	code, res = write(
		srv,
		techInt(),
		"jobs.saveDraft",
		installDraft(job, report, "normal", uuid.NewString(), "SN-1", photos),
		version,
	)
	if code != 200 {
		t.Fatalf("draft with another model: %d %v", code, res)
	}

	version++

	sig := blobJSON("sig.png", "image/png", img, len(img))

	code, res = write(
		srv,
		techInt(),
		"reports.signOff",
		`{"jobId":"`+job+`","reportId":"`+report+`","reportVersion":`+
			itoa(version)+`,"signerName":"Aisha","signature":`+sig+`}`,
		version,
	)
	if code != 200 {
		t.Fatalf("sign-off: %d %v", code, res)
	}

	code, res = write(
		srv,
		techInt(),
		"jobs.submit",
		`{"jobId":"`+job+`","reportVersion":`+itoa(version)+`}`,
		3,
	)
	if fe, _ := res["fieldErrors"].(map[string]any); code != 422 ||
		fe["installation"] != "errors.model_mismatch" {
		t.Fatalf("submit with another model: %d %v", code, res)
	}

	code, res = write(
		srv,
		techInt(),
		"jobs.saveDraft",
		installDraft(job, report, "normal", model, " SN-1 ", photos),
		version,
	)
	if code != 200 {
		t.Fatalf("draft with the unit's model: %d %v", code, res)
	}

	version++

	code, res = write(
		srv,
		techInt(),
		"reports.signOff",
		`{"jobId":"`+job+`","reportId":"`+report+`","reportVersion":`+
			itoa(version)+`,"signerName":"Aisha","signature":`+sig+`}`,
		version,
	)
	if code != 200 {
		t.Fatalf("sign-off again: %d %v", code, res)
	}

	if code, res = write(
		srv,
		techInt(),
		"jobs.submit",
		`{"jobId":"`+job+`","reportVersion":`+itoa(version)+`}`,
		3,
	); code != 200 {
		t.Fatalf("submit: %d %v", code, res)
	}

	if got := installState(t, unit); got != "planned" {
		t.Errorf("unit state before acceptance: %s", got)
	}

	code, res = write(srv, hq(), "jobs.review", `{"jobId":"`+job+`","reportVersion":`+itoa(version)+
		`,"decision":"accept","reviewMode":"normal"}`, 4)
	if code != 200 {
		t.Fatalf("accept: %d %v", code, res)
	}

	drainEvents(t)

	var installedAt *time.Time

	ownerScan(t, `SELECT installed_at FROM assets.units WHERE id = $1`, []any{unit}, &installedAt)

	if got := installState(t, unit); got != "installed" || installedAt == nil {
		t.Errorf("unit after acceptance: %s %v", got, installedAt)
	}

	// an installed unit takes the usual jobs, and its installation date and state are no form's
	code, res = create(hq(), unit, `"reactive"`)
	if code != 200 {
		t.Errorf("a reactive job on the installed unit: %d %v", code, res)
	}
}

func installState(t *testing.T, unit string) string {
	t.Helper()

	var state string

	ownerScan(t, `SELECT install_state FROM assets.units WHERE id = $1`, []any{unit}, &state)

	return state
}

// installBase is the customer organization, property and model of the fixture unit that installation tests plan
// another unit beside.
func installBase(t *testing.T) (string, string, string) {
	t.Helper()

	var org, prop, model string

	ownerScan(
		t,
		`SELECT customer_org_id::text, property_id::text, model_id::text FROM assets.units WHERE id = $1`,
		[]any{seed.ID("unit-non-rto")},
		&org,
		&prop,
		&model,
	)

	return org, prop, model
}

// idList is a JSON array of the IDs.
func idList(ids []string) string {
	if len(ids) == 0 {
		return "[]"
	}

	return `["` + strings.Join(ids, `","`) + `"]`
}
