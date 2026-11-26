package integration_test

import (
	"reflect"
	"strconv"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

const (
	listStartProposals   = "automations.startProposals"
	approveStartProposal = "automations.approveStartProposal"
)

type startEvidenceFixture struct {
	device, binding uuid.UUID
	unit            string
}

func proposalDevice(t *testing.T, unit string) startEvidenceFixture {
	t.Helper()

	fixture := startEvidenceFixture{device: uuid.New(), binding: uuid.New(), unit: unit}
	actor := seed.ID("customer-b")
	owner(t, `INSERT INTO devices.devices (id, tenant_id, serial, binding_id, unit_id,
		created_by_membership_id, firmware_version)
		SELECT $2, tenant_id, $2::uuid::text, $3, id, $4, 'test' FROM assets.units WHERE id = $1`,
		unit, fixture.device, fixture.binding, actor)
	owner(
		t,
		`INSERT INTO devices.device_bindings (id, tenant_id, device_id, unit_id, customer_org_id, bound_at,
		actor_membership_id)
		SELECT $2, tenant_id, $3, id, customer_org_id, $4, $5 FROM assets.units WHERE id = $1`,
		unit,
		fixture.binding,
		fixture.device,
		clock().AddDate(0, 0, -40),
		actor,
	)

	t.Cleanup(func() {
		owner(t, `DELETE FROM control.automations WHERE id IN (
			SELECT automation_id FROM control.automation_units WHERE unit_id = $1
		)`, unit)
		owner(t, `DELETE FROM control.power_observations WHERE binding_id = $1`, fixture.binding)
		owner(
			t,
			`DELETE FROM control.commands WHERE unit_id = $1 AND correlation_id = 'start-proposal-fixture'`,
			unit,
		)
		owner(t, `DELETE FROM devices.device_bindings WHERE id = $1`, fixture.binding)
		owner(t, `DELETE FROM devices.devices WHERE id = $1`, fixture.device)
	})

	return fixture
}

func (f startEvidenceFixture) command(t *testing.T, requestedAt time.Time, source, action string) {
	t.Helper()

	f.delayedCommand(t, requestedAt, requestedAt, source, action)
}

func (f startEvidenceFixture) delayedCommand(
	t *testing.T, requestedAt, sentAt time.Time, source, action string,
) {
	t.Helper()
	owner(
		t,
		`INSERT INTO control.commands (tenant_id, unit_id, device_id, actor_membership_id, source, action,
		status, delivery, requested_at, sent_at, acknowledged_at, expires_at, correlation_id)
		SELECT tenant_id, id, $2, $3, $4, $5, 'acknowledged', 'sent', $6, $7, $7::timestamptz + interval '1 second',
		$6::timestamptz + interval '30 seconds', 'start-proposal-fixture' FROM assets.units WHERE id = $1`,
		f.unit,
		f.device,
		seed.ID("customer-b"),
		source,
		action,
		requestedAt,
		sentAt,
	)
}

func (f startEvidenceFixture) observe(t *testing.T, observedAt, receivedAt time.Time, power *bool) {
	t.Helper()
	owner(
		t,
		`INSERT INTO control.power_observations (tenant_id, binding_id, observed_at, received_at, power)
		SELECT tenant_id, id, $2, $3, $4 FROM devices.device_bindings WHERE id = $1`,
		f.binding,
		observedAt,
		receivedAt,
		power,
	)
}

func (f startEvidenceFixture) transition(t *testing.T, at time.Time, source string) {
	t.Helper()
	f.command(t, at, source, actionPowerOn)
	f.observe(t, at.Add(-time.Minute), at.Add(-time.Minute), new(false))
	f.observe(t, at.Add(time.Minute), at.Add(time.Minute), new(true))
}

func startProposalRule(t *testing.T, srv *apiserver.Server, unit, weekdays string) map[string]any {
	t.Helper()

	body := scheduleRule(unit, map[string]string{
		fieldWeekdays:    weekdays,
		fieldEnabled:     "true",
		fieldStartLocal:  `"18:30"`,
		fieldEndLocal:    `"23:30"`,
		fieldStartAction: `{"kind":"set_temperature","celsius":24}`,
		fieldOnlyIf:      `[{"type":"occupancy","occupied":true}]`,
	})

	code, res := write(srv, customerB(), "automations.save", body, 0)
	if code != 200 {
		t.Fatalf("save schedule: %d %v", code, res)
	}

	return data(res)
}

func proposalFor(
	t *testing.T,
	srv *apiserver.Server,
	who *actor,
	rule map[string]any,
) map[string]any {
	t.Helper()

	code, res := post(srv, who, listStartProposals, "{}")
	if code != 200 {
		t.Fatalf("list schedule suggestions: %d %v", code, res)
	}

	for _, raw := range asArr(t, res["data"]) {
		p := asObj(t, raw)
		if p["automationId"] == rule["id"] {
			return p
		}
	}

	return nil
}

//nolint:paralleltest // the integration tests share one server and the seeded test database
func TestStartProposalsUseMeasuredHistoryAndApplyOneWeekday(t *testing.T) {
	srv := server(t)
	wednesday := proposalDevice(t, newUnit(t, srv, "Wednesday proposal AC"))
	friday := proposalDevice(t, newUnit(t, srv, "Friday proposal AC"))
	uncertain := proposalDevice(t, newUnit(t, srv, "Unconfirmed proposal AC"))
	wedRule := startProposalRule(t, srv, wednesday.unit, "[1,3,5]")
	friRule := startProposalRule(t, srv, friday.unit, "[5]")
	uncertainRule := startProposalRule(t, srv, uncertain.unit, "[3]")

	for i, minute := range []int{50, 0, 10} {
		hour := 12
		if i == 0 {
			hour = 11
		}

		wed := time.Date(2026, 8, 26+i*7, hour, minute, 0, 0, time.UTC)
		fri := time.Date(2026, 8, 28+i*7, 13, 5+i*10, 0, 0, time.UTC)

		wednesday.transition(t, wed, "ui")
		friday.transition(t, fri, "ui")
		uncertain.command(t, wed, "ui", actionPowerOn)
		uncertain.observe(t, wed.Add(time.Minute), wed.Add(time.Minute), new(true))
	}

	addUnconfirmedStarts(t, wednesday, uncertain)

	wed := proposalFor(t, srv, customerB(), wedRule)
	fri := proposalFor(t, srv, customerB(), friRule)
	assertStartProposal(
		t,
		wed,
		wednesday.unit,
		3,
		"20:00",
		[]any{"2026-08-26", "2026-09-02", "2026-09-09"},
	)
	assertStartProposal(
		t,
		fri,
		friday.unit,
		5,
		"21:15",
		[]any{"2026-08-28", "2026-09-04", "2026-09-11"},
	)

	if p := proposalFor(t, srv, customerB(), uncertainRule); p != nil {
		t.Fatalf("unconfirmed history yielded a suggestion: %v", p)
	}

	if p := proposalFor(t, srv, customerA(), wedRule); p != nil {
		t.Fatalf("other customer read a suggestion: %v", p)
	}

	wedApproval := approvalBody(t, wed)
	if code, res := write(srv, customerA(), approveStartProposal, wedApproval, 1); code != 404 {
		t.Fatalf("other customer approved suggestion: %d %v", code, res)
	}

	code, missingVersion := write(srv, customerB(), approveStartProposal, wedApproval, 0)
	refusal(t, "approval without version", code, missingVersion, requiredExpectedVersion)

	code, res := write(srv, customerB(), approveStartProposal, wedApproval, 1)
	if code != 200 {
		t.Fatalf("approve Wednesday: %d %v", code, res)
	}

	applied := data(res)
	assertPreservedSchedule(t, wedRule, applied, []any{float64(3)}, "20:00")

	if applied["id"] == wedRule["id"] {
		t.Fatal("partial approval did not split the schedule")
	}

	remainder := scheduleByID(t, srv, wednesday.unit, asStr(t, wedRule["id"]))
	assertPreservedSchedule(t, wedRule, remainder, []any{float64(1), float64(5)}, "18:30")

	if remainder["version"] != float64(2) || proposalFor(t, srv, customerB(), applied) != nil {
		t.Fatalf("unexpected remainder or repeated proposal: %v", remainder)
	}

	assertStaleStartProposal(t, srv, friday.unit, friRule, fri)
}

func addUnconfirmedStarts(t *testing.T, confirmed, uncertain startEvidenceFixture) {
	t.Helper()

	wed := time.Date(2026, 8, 19, 12, 0, 0, 0, time.UTC)
	// The fourth Wednesday has an actual transition, but it was triggered by another automation.
	confirmed.transition(t, wed, "automation")

	// A positive report alone, a stale/future report and an already-on resend are insufficient.
	uncertain.observe(t, wed.Add(-time.Minute), wed.Add(-time.Minute), new(true))
	uncertain.command(t, wed, "ui", actionPowerOn)
	uncertain.observe(t, wed.Add(time.Minute), wed.Add(time.Minute), new(true))
	late := wed.AddDate(0, 0, 7)
	uncertain.observe(t, late.Add(-time.Minute), clock().Add(time.Minute), new(false))

	interrupted := wed.AddDate(0, 0, 14)
	uncertain.observe(t, interrupted.Add(-time.Minute), interrupted.Add(-time.Minute), new(false))
	uncertain.command(t, interrupted.Add(30*time.Second), "automation", actionPowerOff)
}

func assertStartProposal(
	t *testing.T,
	proposal map[string]any,
	unit string,
	weekday int,
	start string,
	dates []any,
) {
	t.Helper()

	want := map[string]any{
		"unitId": unit, "weekday": float64(weekday), "from": "18:30", "to": start,
		"endLocal": "23:30", "occurrences": float64(4), "supportDates": dates,
	}

	for field, value := range want {
		if !reflect.DeepEqual(proposal[field], value) {
			t.Errorf("suggestion %s: got %v, want %v", field, proposal[field], value)
		}
	}
}

func approvalBody(t *testing.T, p map[string]any) string {
	t.Helper()

	return `{"id":"` + asStr(t, p["automationId"]) + `","proposalId":"` + asStr(t, p["id"]) +
		`","weekday":` + strconv.Itoa(int(asNum(t, p["weekday"]))) + `}`
}

func scheduleByID(t *testing.T, srv *apiserver.Server, unit, id string) map[string]any {
	t.Helper()

	code, res := post(srv, customerB(), "automations.list", `{"filters":{"unitId":"`+unit+`"}}`)
	if code != 200 {
		t.Fatalf("list schedules: %d %v", code, res)
	}

	for _, rule := range items(t, res) {
		if rule["id"] == id {
			return rule
		}
	}

	t.Fatalf("missing schedule %s", id)

	return nil
}

func assertPreservedSchedule(t *testing.T, before, after map[string]any, days []any, start string) {
	t.Helper()

	if after["startLocal"] != start || !reflect.DeepEqual(after["weekdays"], days) {
		t.Fatalf("expected %v at %s: %v", days, start, after)
	}

	for _, field := range []string{
		"name", "startAction", fieldEndAction, "endLocal", "endsNextDay", "timezone", "priority", "enabled",
		fieldOnlyIf, "ownerMembershipId", "createdByUserId", "unitIds",
	} {
		if !reflect.DeepEqual(before[field], after[field]) {
			t.Errorf("%s changed from %v to %v", field, before[field], after[field])
		}
	}
}

func assertStaleStartProposal(
	t *testing.T,
	srv *apiserver.Server,
	unit string,
	rule, proposal map[string]any,
) {
	t.Helper()
	id := asStr(t, rule["id"])
	body := scheduleRule(unit, map[string]string{
		"id": strconv.Quote(id), fieldWeekdays: "[5]", fieldEnabled: "true",
		fieldStartLocal: `"19:00"`, fieldEndLocal: `"23:30"`,
	})

	code, res := write(srv, customerB(), "automations.save", body, 1)
	if code != 200 {
		t.Fatalf("edit schedule: %d %v", code, res)
	}

	code, res = write(srv, customerB(), approveStartProposal, approvalBody(t, proposal), 1)
	if code != 409 || res["messageKey"] != apperr.KeyVersionConflict {
		t.Fatalf("stale approval: %d %v", code, res)
	}

	if latest := scheduleByID(
		t,
		srv,
		unit,
		id,
	); latest["startLocal"] != "19:00" ||
		latest["version"] != float64(2) {
		t.Fatalf("stale approval overwrote edit: %v", latest)
	}

	fresh := proposalFor(t, srv, customerB(), rule)
	if fresh["from"] != "19:00" || fresh["to"] != "21:15" || fresh["id"] == proposal["id"] {
		t.Fatalf("updated suggestion: %v", fresh)
	}

	code, res = write(srv, customerB(), approveStartProposal, approvalBody(t, fresh), 2)
	if code != 200 || data(res)["id"] != id || data(res)["startLocal"] != "21:15" ||
		ver(t, res) != 3 {
		t.Fatalf("approve current single-day schedule: %d %v", code, res)
	}
}
