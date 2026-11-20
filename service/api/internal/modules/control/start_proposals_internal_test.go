package control

import (
	"encoding/json"
	"errors"
	"maps"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

func TestStartProposalApprovalValidation(t *testing.T) {
	t.Parallel()

	approval := StartProposalApproval{ID: uuid.Nil, ProposalID: uuid.Nil, Weekday: 0}
	want := map[string]string{
		"id":         apperr.KeyRequired,
		"proposalId": apperr.KeyRequired,
		"weekday":    apperr.KeyRange,
	}

	if got := approval.Validate(); !maps.Equal(got, want) {
		t.Fatalf("missing approval fields: got %v, want %v", got, want)
	}

	approval.ID, approval.ProposalID, approval.Weekday = uuid.New(), uuid.New(), isoSunday+1
	if got := approval.Validate(); !maps.Equal(got, map[string]string{"weekday": apperr.KeyRange}) {
		t.Errorf("weekday beyond Sunday: got %v", got)
	}

	approval.Weekday = isoSunday
	if got := approval.Validate(); len(got) != 0 {
		t.Errorf("valid approval: got %v", got)
	}
}

func TestStartProposalRejectsInvalidSchedule(t *testing.T) {
	t.Parallel()

	// 01:30 occurs twice on the first Sunday in November in this timezone.
	var rule Automation

	rule.ID = uuid.New()
	rule.Name = "Sunday cooling"
	rule.UnitIDs = []uuid.UUID{uuid.New()}
	rule.Kind = kindSchedule
	rule.Weekdays = []int{isoSunday}
	rule.Timezone = "America/New_York"
	rule.StartLocal = "08:00"
	rule.EndLocal = "10:00"
	rule.EndsNextDay = new(false)
	rule.StartAction = json.RawMessage(`{"kind":"set_power","power":true}`)
	rule.EndAction = json.RawMessage(`{"kind":"set_power","power":false}`)

	var call ops.Call

	call.Now = time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC)
	votes := []startVote{{"2026-08-23", 90}, {"2026-08-30", 90}, {"2026-09-06", 90}}

	// The same votes are usable in a timezone without a repeated hour.
	regular := rule
	regular.Timezone = kualaLumpur

	valid, ok, err := proposeStart(&call, regular, isoSunday, votes)
	if err != nil || !ok || valid.To != "01:30" {
		t.Fatalf("ordinary wall time: proposal %+v, available %t, err %v", valid, ok, err)
	}

	candidate, ok, err := proposeStart(&call, rule, isoSunday, votes)
	if err != nil || ok {
		t.Fatalf("ambiguous wall time: proposal %+v, available %t, err %v", candidate, ok, err)
	}

	// An invalid stored action is also rejected before any schedule is proposed.
	regular.StartAction = json.RawMessage(`{"kind":"unknown"}`)

	candidate, ok, err = proposeStart(&call, regular, isoSunday, votes)
	if err != nil || ok {
		t.Fatalf("invalid action: proposal %+v, available %t, err %v", candidate, ok, err)
	}

	var (
		automations Automations
		domain      *apperr.DomainError
	)

	_, err = automations.applyStartProposal(t.Context(), &call, regular, valid)
	if !errors.As(err, &domain) || domain.Code != apperr.Validation ||
		domain.FieldErrors["startAction"] != apperr.KeyInvalid {
		t.Fatalf("applying an invalid action: want startAction validation, got %v", err)
	}

	// A malformed raw action must not be proposed even if the decoder read its first JSON value.
	regular.StartAction = json.RawMessage(`{"kind":"set_power","power":true} null`)

	var encodeErr *json.MarshalerError

	candidate, ok, err = proposeStart(&call, regular, isoSunday, votes)
	if ok || !errors.As(err, &encodeErr) {
		t.Fatalf("malformed raw action: proposal %+v, available %t, err %v", candidate, ok, err)
	}
}

func TestStartProposalTimingUsesLocalScheduleWindow(t *testing.T) {
	t.Parallel()

	loc, err := time.LoadLocation(kualaLumpur)
	if err != nil {
		t.Fatal(err)
	}

	sunday := time.Date(2026, 9, 13, 0, 0, 0, 0, loc)

	var rule Automation

	rule.Timezone, rule.EndLocal = kualaLumpur, "23:30"
	proposal := StartProposal{
		ID: uuid.Nil, AutomationID: uuid.Nil, UnitID: uuid.Nil, Version: 1, Weekday: isoSunday,
		From: "18:30", To: "20:00", EndLocal: rule.EndLocal, Occurrences: 4, SupportDates: nil,
	}
	previousEnd := sunday.AddDate(0, 0, -7).Add(23*time.Hour + 30*time.Minute)

	for _, test := range []struct {
		name           string
		now, watermark time.Time
		safe           bool
	}{
		{"before start with evaluated prior week", sunday.Add(18 * time.Hour), previousEnd, true},
		{"before start with unevaluated prior week", sunday.Add(18 * time.Hour), previousEnd.Add(-time.Second), false},
		{"at the original start", sunday.Add(18*time.Hour + 30*time.Minute), time.Time{}, false},
		{"exactly at the end", sunday.Add(23*time.Hour + 30*time.Minute), time.Time{}, false},
		{"after the end with unevaluated window", sunday.Add(24*time.Hour - time.Minute), previousEnd, false},
		{
			"after the end with evaluated window", sunday.Add(24*time.Hour - time.Minute),
			sunday.Add(23*time.Hour + 30*time.Minute), true,
		},
	} {
		if got := proposalTimingSafe(
			test.now.UTC(),
			test.watermark,
			rule,
			proposal,
		); got != test.safe {
			t.Errorf("%s: got %t, want %t", test.name, got, test.safe)
		}
	}

	rule.Timezone = "Invalid/Timezone"
	if proposalTimingSafe(sunday, time.Time{}, rule, proposal) {
		t.Error("a rule with an invalid timezone accepted approval")
	}
}
