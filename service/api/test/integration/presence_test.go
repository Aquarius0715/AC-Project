package integration_test

import (
	"strconv"
	"testing"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// presence.report (DEC-109, IR826): a member's device against the pins of the customer's properties. Only inside or
// outside is kept; the household's first arrival and last departure fire the property's location automations; a
// withdrawn consent forgets the member's states.
//
//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestPresenceReport(t *testing.T) {
	srv := server(t)
	unit := boundUnit(t, srv, "online")

	var property uuid.UUID

	ownerScan(t, `SELECT property_id FROM assets.units WHERE id = $1`, []any{unit}, &property)
	owner(
		t,
		`UPDATE assets.properties SET latitude = 3.1579, longitude = 101.7116, geofence_radius_m = 200 WHERE id = $1`,
		property,
	)
	t.Cleanup(func() {
		owner(
			t,
			`UPDATE assets.properties SET latitude = NULL, longitude = NULL, geofence_radius_m = NULL WHERE id = $1`,
			property,
		)
		owner(t, `DELETE FROM control.presence_states WHERE property_id = $1`, property)
	})

	report := func(latitude float64, accuracy int) (int, map[string]any) {
		t.Helper()

		return write(
			srv,
			customerB(),
			"presence.report",
			`{"latitude":`+strconv.FormatFloat(latitude, 'f', -1, 64)+
				`,"longitude":101.7116,"accuracyMeters":`+strconv.Itoa(accuracy)+`}`,
			0,
		)
	}
	transitions := func(latitude float64) []any {
		t.Helper()

		code, res := report(latitude, 10)
		if code != 200 {
			t.Fatalf("report %v: %d %v", latitude, code, res)
		}

		return asArr(t, data(res)["transitions"])
	}

	// the member's location consent first (SR02)
	_, consent := post(srv, customerB(), "consents.get", `{"purpose":"location_automation"}`)
	if data(consent)["granted"] != true {
		if code, res := write(
			srv,
			customerB(),
			"consents.update",
			`{"purpose":"location_automation","granted":true}`,
			ver(t, consent),
		); code != 200 {
			t.Fatalf("consent: %d %v", code, res)
		}
	}

	owner(
		t,
		`UPDATE identity.consents SET granted = false WHERE membership_id = $1 AND purpose = 'location_automation'`,
		seed.ID(membershipCustomerB),
	)

	if code, res := report(
		3.1579,
		10,
	); code != 409 ||
		res["messageKey"] != "errors.consent_required" {
		t.Fatalf("without consent: %d %v", code, res)
	}

	owner(
		t,
		`UPDATE identity.consents SET granted = true WHERE membership_id = $1 AND purpose = 'location_automation'`,
		seed.ID(membershipCustomerB),
	)

	for name, example := range map[string]struct {
		latitude float64
		accuracy int
		want     string
	}{
		"inaccurate fix": {3.1579, 150, "accuracyMeters=errors.position_inaccurate"},
		"off the globe":  {91, 10, "latitude=error.range"},
		"negative error": {3.1579, -1, "accuracyMeters=error.range"},
	} {
		code, res := report(example.latitude, example.accuracy)
		refusal(t, name, code, res, example.want)
	}

	// a location rule on the unit: arrival → power on
	code, res := write(
		srv,
		customerB(),
		"automations.save",
		`{"name":"Home again","unitIds":["`+unit+`"],`+
			`"timezone":"Asia/Kuala_Lumpur","enabled":true,"priority":10,"kind":"event",`+
			`"condition":{"type":"location","event":"arrival"},"action":{"kind":"set_power","power":true}}`,
		0,
	)
	if code != 200 {
		t.Fatalf("rule: %d %v", code, res)
	}

	ruleID, ruleVersion := asStr(t, data(res)["id"]), ver(t, res)
	t.Cleanup(
		func() { _, _ = write(srv, customerB(), "automations.delete", `{"id":"`+ruleID+`"}`, ruleVersion) },
	)

	if got := transitions(3.1700); len(got) != 0 { // 1.3 km away: outside, and nobody was home
		t.Fatalf("far: %v", got)
	}

	got := transitions(3.1590) // 122 m: inside the 200 m radius
	if len(got) != 1 {
		t.Fatalf("arrival: %v", got)
	}

	arrival := asObj(t, got[0])
	if arrival["propertyId"] != property.String() || arrival["event"] != "arrival" ||
		len(asArr(t, arrival["commandIds"])) != 1 {
		t.Fatalf("arrival: %v", arrival)
	}

	if got := transitions(3.1590); len(got) != 0 {
		t.Fatalf("still home: %v", got)
	}

	if got := transitions(3.1600); len(got) != 0 { // 233 m: past the radius, inside the 50 m band
		t.Fatalf("in the band: %v", got)
	}

	var inside bool

	ownerScan(
		t,
		`SELECT inside FROM control.presence_states WHERE property_id = $1 AND membership_id = $2`,
		[]any{property, seed.ID(membershipCustomerB)},
		&inside,
	)

	if !inside {
		t.Fatal("the band keeps the member inside")
	}

	// a second member at home: leaving is not the household's departure, and coming back is no arrival
	other := uuid.New()
	owner(
		t,
		`INSERT INTO control.presence_states (tenant_id, membership_id, property_id, inside, changed_at,
		reported_at) SELECT tenant_id, $1, id, true, now(), now() FROM assets.properties WHERE id = $2`,
		other,
		property,
	)

	if got := transitions(3.1700); len(got) != 0 {
		t.Fatalf("someone is still home: %v", got)
	}

	if got := transitions(3.1590); len(got) != 0 {
		t.Fatalf("someone was home already: %v", got)
	}

	owner(t, `DELETE FROM control.presence_states WHERE membership_id = $1`, other)

	got = transitions(3.1700)
	if len(got) != 1 || asObj(t, got[0])["event"] != "departure" ||
		len(asArr(t, asObj(t, got[0])["commandIds"])) != 0 {
		t.Fatalf("the last one left (no departure rule): %v", got)
	}

	// IR53: a withdrawn consent forgets the member's states
	_, consent = post(srv, customerB(), "consents.get", `{"purpose":"location_automation"}`)
	if code, res := write(
		srv,
		customerB(),
		"consents.update",
		`{"purpose":"location_automation","granted":false}`,
		ver(t, consent),
	); code != 200 {
		t.Fatalf("withdraw: %d %v", code, res)
	}

	var left int

	ownerScan(t, `SELECT count(*) FROM control.presence_states WHERE membership_id = $1`,
		[]any{seed.ID(membershipCustomerB)}, &left)

	if left != 0 {
		t.Fatalf("%d presence rows after the withdrawal", left)
	}

	_, consent = post(srv, customerB(), "consents.get", `{"purpose":"location_automation"}`)
	_, _ = write(
		srv,
		customerB(),
		"consents.update",
		`{"purpose":"location_automation","granted":true}`,
		ver(t, consent),
	)
}
