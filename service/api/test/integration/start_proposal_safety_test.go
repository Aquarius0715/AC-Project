package integration_test

import (
	"net/http"
	"reflect"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
	apiserver "github.com/pradita/ac-project/service/api/internal/server"
)

func seedThreeProposalStarts(t *testing.T, fixture startEvidenceFixture, last time.Time) {
	t.Helper()

	for offset := range 3 {
		fixture.transition(t, last.AddDate(0, 0, -7*offset), "ui")
	}
}

//nolint:paralleltest // this scenario mutates a schedule in the shared integration database
func TestStartProposalsRecheckEvidenceAndCompetingRules(t *testing.T) {
	srv := server(t)
	fixture := proposalDevice(t, newUnit(t, srv, "Proposal conflicts AC"))
	rule := startProposalRule(t, srv, fixture.unit, "[3]")
	ruleID := asStr(t, rule["id"])
	before := scheduleByID(t, srv, fixture.unit, ruleID)
	last := time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC)
	seedThreeProposalStarts(t, fixture, last)

	initial := proposalFor(t, srv, customerB(), rule)
	approval := approvalBody(t, initial)
	setObservedPower := func(power bool) {
		t.Helper()

		owner(
			t,
			`UPDATE control.power_observations SET power = $3 WHERE binding_id = $1 AND observed_at = $2`,
			fixture.binding,
			last.Add(time.Minute),
			power,
		)
	}
	setObservedPower(false)
	checkProposalApprovalConflict(t, srv, approval, "errors.automation_proposal_changed")
	setObservedPower(true)

	competitor := startProposalRule(t, srv, fixture.unit, "[5]")
	if got := proposalFor(t, srv, customerB(), rule); got["id"] != initial["id"] {
		t.Fatalf("another weekday blocked the proposal: %v", got)
	}

	competitorID := asStr(t, competitor["id"])
	owner(t, `UPDATE control.automations SET timezone = 'UTC' WHERE id = $1`, competitorID)

	if got := proposalFor(t, srv, customerB(), rule); got != nil {
		t.Fatalf("a conflicting timezone left a proposal: %v", got)
	}

	owner(t, `UPDATE control.automations SET timezone = $2, definition = definition ||
		'{"endLocal":"06:00","endsNextDay":true}'::jsonb WHERE id = $1`, competitorID, competitor["timezone"])

	if got := proposalFor(t, srv, customerB(), rule); got != nil {
		t.Fatalf("an overnight competing rule left a proposal: %v", got)
	}

	owner(
		t,
		`UPDATE control.automations SET definition = definition ||
		'{"weekdays":[3],"endLocal":"23:30","endsNextDay":false}'::jsonb WHERE id = $1`,
		competitorID,
	)

	if got := proposalFor(t, srv, customerB(), rule); got != nil {
		t.Fatalf("a competing rule on the same weekday left a proposal: %v", got)
	}

	if after := scheduleByID(t, srv, fixture.unit, ruleID); !reflect.DeepEqual(after, before) {
		t.Fatalf("proposal conflict changed the schedule: before %v, after %v", before, after)
	}
}

//nolint:paralleltest // the test temporarily changes the shared tenant's scheduler watermark
func TestStartProposalsRefuseAnActiveSchedule(t *testing.T) {
	srv := server(t)
	fixture := proposalDevice(t, newUnit(t, srv, "Proposal during running schedule AC"))
	body := scheduleRule(fixture.unit, map[string]string{
		fieldWeekdays: jsonMondays, fieldEnabled: jsonTrue,
		fieldStartLocal: `"08:05"`, fieldEndLocal: `"10:00"`,
	})

	code, res := write(srv, customerB(), "automations.save", body, 0)
	if code != http.StatusOK {
		t.Fatalf("save Monday's schedule: %d %v", code, res)
	}

	rule := data(res)

	seedThreeProposalStarts(t, fixture, clock().AddDate(0, 0, -7).Add(30*time.Minute))

	proposal := proposalFor(t, srv, customerB(), rule)
	if proposal["to"] != "09:30" {
		t.Fatalf("want a proposed start at 09:30: %v", proposal)
	}

	preserveProposalWatermark(t)
	owner(t, `DELETE FROM control.schedule_watermarks WHERE tenant_id = $1`, seed.ID("tenant-a"))
	checkProposalApprovalConflict(
		t,
		srv,
		approvalBody(t, proposal),
		"errors.automation_proposal_timing",
	)

	ruleID := asStr(t, rule["id"])
	after := scheduleByID(t, srv, fixture.unit, ruleID)
	assertPreservedSchedule(t, rule, after, []any{float64(1)}, "08:05")

	if after["version"] != rule["version"] {
		t.Fatalf("rejected approval changed the version: %v", after)
	}
}

//nolint:paralleltest // this scenario mutates a schedule in the shared integration database
func TestStartProposalsRejectOvernightAndInvalidStoredSchedules(t *testing.T) {
	srv := server(t)
	fixture := proposalDevice(t, newUnit(t, srv, "Proposal from legacy schedule AC"))
	rule := startProposalRule(t, srv, fixture.unit, "[3,5]")
	ruleID := asStr(t, rule["id"])
	seedThreeProposalStarts(t, fixture, time.Date(2026, 9, 9, 12, 0, 0, 0, time.UTC))

	// An older rule whose unused weekday is invalid can still present a valid Wednesday.
	owner(
		t,
		`UPDATE control.automations SET definition = jsonb_set(definition, '{weekdays}', '[3,8]')
		WHERE id = $1`,
		ruleID,
	)
	proposal := proposalFor(t, srv, customerB(), rule)

	code, res := write(srv, customerB(), approveStartProposal, approvalBody(t, proposal), 1)
	if code != http.StatusUnprocessableEntity || asObj(t, res["fieldErrors"])["weekdays"] == nil {
		t.Fatalf("approval did not reject the invalid remaining weekday: %d %v", code, res)
	}

	owner(t, `UPDATE control.automations SET definition = definition ||
		'{"weekdays":[3],"endLocal":"06:00","endsNextDay":true}'::jsonb WHERE id = $1`, ruleID)

	if got := proposalFor(t, srv, customerB(), rule); got != nil {
		t.Fatalf("an overnight rule produced a proposal: %v", got)
	}

	owner(
		t,
		`UPDATE control.automations SET timezone = 'Invalid/Timezone', definition = definition ||
		'{"endLocal":"23:30","endsNextDay":false}'::jsonb WHERE id = $1`,
		ruleID,
	)

	code, res = post(srv, customerB(), listStartProposals, "{}")
	if code != http.StatusServiceUnavailable {
		t.Fatalf("list with an invalid stored timezone: %d %v", code, res)
	}
}

func checkProposalApprovalConflict(t *testing.T, srv *apiserver.Server, body, wantKey string) {
	t.Helper()

	code, res := write(srv, customerB(), approveStartProposal, body, 1)
	if code != http.StatusConflict || res["messageKey"] != wantKey {
		t.Fatalf("approval: %d %v, want conflict %s", code, res, wantKey)
	}
}

func preserveProposalWatermark(t *testing.T) {
	t.Helper()

	tenant := seed.ID("tenant-a")

	var previous *time.Time

	ownerScan(
		t,
		`SELECT (SELECT evaluated_until FROM control.schedule_watermarks WHERE tenant_id = $1)`,
		[]any{tenant},
		&previous,
	)
	t.Cleanup(func() {
		if previous == nil {
			owner(t, `DELETE FROM control.schedule_watermarks WHERE tenant_id = $1`, tenant)

			return
		}

		setWatermark(t, *previous)
	})
}

//nolint:paralleltest // the subtests inject failures against the same saved schedule
func TestStartProposalsHandleDatabaseFailuresWithoutPartialApproval(t *testing.T) {
	const (
		evidenceQuery  = "WITH manual AS ("
		competingQuery = "SELECT a.timezone, a.definition FROM control.automations"
		listQuery      = "a.customer_org_id = $1 AND a.kind = 'schedule' AND a.enabled"
	)

	srv := server(t)
	fixture := proposalDevice(t, newUnit(t, srv, "Proposal rollback AC"))
	rule := startProposalRule(t, srv, fixture.unit, "[3,5]")
	startProposalRule(t, srv, fixture.unit, "[5]") // the competing-schedule query has a row to scan
	seedThreeProposalStarts(t, fixture, clock().AddDate(0, 0, -5).Add(11*time.Hour))

	ruleID := asStr(t, rule["id"])
	owner(
		t,
		`UPDATE control.automations SET name = $2 WHERE id = $1`,
		ruleID,
		"Proposal rollback schedule "+ruleID,
	)

	before := scheduleByID(t, srv, fixture.unit, ruleID)
	approval := approvalBody(t, proposalFor(t, srv, customerB(), rule))

	for _, test := range []struct {
		name, match   string
		approve, scan bool
		skipExec      int
	}{
		{"schedules query", listQuery, false, false, 0},
		{"schedules scan", listQuery, false, true, 0},
		{"evidence query", evidenceQuery, false, false, 0},
		{"competing schedules query", competingQuery, false, false, 0},
		{"competing schedule scan", competingQuery, false, true, 0},
		{"watermark lock", "SELECT evaluated_until FROM control.schedule_watermarks", true, false, 0},
		{"approval evidence query", evidenceQuery, true, false, 0},
		{"update remaining weekdays", "UPDATE control.automations SET name = $2", true, false, 0},
		{"insert approved weekday", "INSERT INTO control.automations (tenant_id, customer_org_id", true, false, 0},
		// The first link is for the updated original rule; the second is for the newly inserted rule.
		{"link approved weekday", "INSERT INTO control.automation_units", true, false, 1},
	} {
		t.Run(test.name, func(t *testing.T) {
			broken := serverCfg(t, func(c *apiserver.Config) {
				c.WrapTx = func(tx pgx.Tx) pgx.Tx {
					return &failing{
						Tx:       tx,
						match:    test.match,
						scan:     test.scan,
						skipExec: test.skipExec,
					}
				}
			})

			if !test.approve {
				code, res := post(broken, customerB(), listStartProposals, "{}")
				if code != http.StatusServiceUnavailable {
					t.Errorf("list: %d %v, want 503", code, res)
				}

				return
			}

			code, res := write(broken, customerB(), approveStartProposal, approval, 1)
			if code != http.StatusServiceUnavailable {
				t.Fatalf("approval: %d %v, want 503", code, res)
			}

			if after := scheduleByID(
				t,
				srv,
				fixture.unit,
				ruleID,
			); !reflect.DeepEqual(
				after,
				before,
			) {
				t.Errorf("failed approval changed the schedule: before %v, after %v", before, after)
			}

			var total int

			ownerScan(t, `SELECT count(*) FROM control.automations
				WHERE customer_org_id = $1 AND name = $2 AND id NOT IN (
					SELECT automation_id FROM control.automation_units
				)`, []any{seed.ID("org-customer-b"), before["name"]}, &total)

			if total != 0 {
				t.Errorf("failed approval left %d unlinked rules", total)
			}
		})
	}

	code, res := write(srv, customerB(), approveStartProposal, approval, 1)
	if code != http.StatusOK {
		t.Fatalf("retry after the database recovers: %d %v", code, res)
	}

	assertPreservedSchedule(t, before, data(res), []any{float64(3)}, "20:00")
}
