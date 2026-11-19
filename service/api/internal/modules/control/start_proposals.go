package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strconv"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

const (
	proposalOccurrences = 4
	proposalMinSupport  = 3
	proposalTolerance   = 15
	proposalConflict    = "errors.automation_proposal_changed"
)

// StartProposal describes one weekday's start change and the days that support it.
type StartProposal struct {
	ID           uuid.UUID `json:"id"`
	AutomationID uuid.UUID `json:"automationId"`
	UnitID       uuid.UUID `json:"unitId"`
	Version      int       `json:"version"`
	Weekday      int       `json:"weekday"`
	From         string    `json:"from"`
	To           string    `json:"to"`
	EndLocal     string    `json:"endLocal"`
	Occurrences  int       `json:"occurrences"`
	SupportDates []string  `json:"supportDates"`
}

// StartProposalApproval identifies the exact proposal the customer saw.
type StartProposalApproval struct {
	ID         uuid.UUID `json:"id"`
	ProposalID uuid.UUID `json:"proposalId"`
	Weekday    int       `json:"weekday"`
}

// Validate implements ops.Validator.
func (in *StartProposalApproval) Validate() map[string]string {
	fields := map[string]string{}
	if in.ID == uuid.Nil {
		fields["id"] = apperr.KeyRequired
	}

	if in.ProposalID == uuid.Nil {
		fields["proposalId"] = apperr.KeyRequired
	}

	if in.Weekday < isoMonday || in.Weekday > isoSunday {
		fields["weekday"] = apperr.KeyRange
	}

	return fields
}

// proposalRules reads the current organization's enabled, single-unit schedules. Column checks also keep archived
// units and units moved to another customer out of the proposal reader.
func proposalRules(ctx context.Context, call *ops.Call) ([]Automation, error) {
	rows, err := call.Tx.Query(ctx, "SELECT "+automationCols+` FROM control.automations a
		WHERE a.customer_org_id = $1 AND a.kind = 'schedule' AND a.enabled AND
		(SELECT count(*) FROM control.automation_units link WHERE link.automation_id = a.id) = 1 AND EXISTS (
			SELECT 1 FROM control.automation_units link JOIN assets.units u ON u.id = link.unit_id
			WHERE link.automation_id = a.id AND u.customer_org_id = a.customer_org_id AND NOT u.archived
		) ORDER BY a.id`, call.Principal.OrgID)
	if err != nil {
		return nil, apperr.Wrap(err, "list schedules for proposals")
	}
	defer rows.Close()

	var rules []Automation

	for rows.Next() {
		rule, err := scanAutomation(rows)
		if err != nil {
			return nil, err
		}

		rules = append(rules, rule)
	}

	return rules, apperr.Wrap(rows.Err(), "list schedules for proposals")
}

type startVote struct {
	Date   string
	Minute int
}

// pickStart matches the demo's ordering: most dates within 15 minutes, then least total deviation, then earliest.
func pickStart(votes []startVote) (int, []string) {
	best, deviation := 0, 0

	var dates []string

	for candidate := range hoursPerDay * minutesPerHour {
		var supported []string

		total := 0

		for _, vote := range votes {
			distance := max(vote.Minute-candidate, candidate-vote.Minute)
			if distance <= proposalTolerance {
				supported = append(supported, vote.Date)
				total += distance
			}
		}

		if len(supported) > len(dates) || (len(supported) == len(dates) && total < deviation) {
			best, dates, deviation = candidate, supported, total
		}
	}

	return best, dates
}

func isoWeekday(day time.Time) int {
	if day.Weekday() == time.Sunday {
		return isoSunday
	}

	return int(day.Weekday())
}

func firstStarts(starts []time.Time, loc *time.Location, today time.Time) map[int][]startVote {
	first := map[int][]startVote{}
	lastDate := ""

	for _, at := range starts {
		local := at.In(loc)

		date := local.Format(time.DateOnly)
		if local.Before(today) && date != lastDate {
			weekday := isoWeekday(local)
			first[weekday] = append(
				first[weekday],
				startVote{date, local.Hour()*minutesPerHour + local.Minute()},
			)
			lastDate = date
		}
	}

	return first
}

// startProposalsFor evaluates completed local days only. The first confirmed manual start on each date supplies one
// vote. It uses the current rule for the proposed edit and does not infer historical schedule state.
func startProposalsFor(
	ctx context.Context,
	call *ops.Call,
	rule Automation,
) ([]StartProposal, error) {
	if rule.Kind != kindSchedule || !rule.Enabled || len(rule.UnitIDs) != 1 ||
		rule.EndsNextDay == nil || *rule.EndsNextDay || !hhmm.MatchString(rule.StartLocal) ||
		!hhmm.MatchString(rule.EndLocal) {
		return nil, nil
	}

	loc, err := time.LoadLocation(rule.Timezone)
	if err != nil {
		return nil, apperr.Wrap(err, "load schedule timezone")
	}

	now := call.Now.In(loc)
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, loc)
	since := today.AddDate(0, 0, -7*proposalOccurrences)

	starts, err := confirmedManualStarts(ctx, call, rule, since)
	if err != nil {
		return nil, err
	}

	votes := firstStarts(starts, loc, today)
	if len(votes) == 0 {
		return nil, nil
	}

	blocked, err := overlappingScheduleDays(ctx, call, rule)
	if err != nil {
		return nil, err
	}

	var proposals []StartProposal

	for _, weekday := range rule.Weekdays {
		if blocked[weekday] {
			continue
		}

		proposal, ok, err := proposeStart(call, rule, weekday, votes[weekday])
		if err != nil {
			return nil, err
		}

		if ok {
			proposals = append(proposals, proposal)
		}
	}

	return proposals, nil
}

func proposeStart(
	call *ops.Call,
	rule Automation,
	weekday int,
	votes []startVote,
) (StartProposal, bool, error) {
	var proposal StartProposal

	if len(votes) < proposalMinSupport {
		return proposal, false, nil
	}

	best, dates := pickStart(votes)

	proposedStart := fmt.Sprintf("%02d:%02d", best/minutesPerHour, best%minutesPerHour)
	if len(dates) < proposalMinSupport || proposedStart == rule.StartLocal ||
		proposedStart >= rule.EndLocal {
		return proposal, false, nil
	}

	input := scheduleProposalInput(rule, []int{weekday}, proposedStart)
	if len(input.Validate()) != 0 {
		return proposal, false, nil
	}

	_, err := ruleDefinition(call, &input)
	if err != nil {
		var domain *apperr.DomainError
		if errors.As(err, &domain) && domain.Code == apperr.Validation {
			return proposal, false, nil // a newly proposed wall time may be ambiguous or nonexistent
		}

		return proposal, false, err
	}

	proposal = StartProposal{
		ID:           uuid.Nil,
		AutomationID: rule.ID,
		UnitID:       rule.UnitIDs[0],
		Version:      rule.Version,
		Weekday:      weekday,
		From:         rule.StartLocal,
		To:           proposedStart,
		EndLocal:     rule.EndLocal,
		Occurrences:  proposalOccurrences,
		SupportDates: dates,
	}

	evidence, err := json.Marshal(proposal)
	if err != nil {
		return proposal, false, apperr.Wrap(err, "encode start proposal")
	}

	proposal.ID = uuid.NewSHA1(rule.ID, evidence)

	return proposal, true, nil
}

func scheduleProposalInput(rule Automation, weekdays []int, start string) AutomationInput {
	var noCondition condition

	return AutomationInput{
		ID:          &rule.ID,
		Name:        rule.Name,
		UnitIDs:     rule.UnitIDs,
		Timezone:    rule.Timezone,
		Enabled:     rule.Enabled,
		Priority:    rule.Priority,
		Kind:        rule.Kind,
		Weekdays:    weekdays,
		StartLocal:  start,
		EndLocal:    rule.EndLocal,
		EndsNextDay: rule.EndsNextDay,
		StartAction: rule.StartAction,
		EndAction:   rule.EndAction,
		OnlyIf:      rule.OnlyIf,
		Condition:   nil,
		Action:      nil,
		actions:     nil,
		condition:   noCondition,
		extras:      nil,
	}
}

// startProposals serves automations.startProposals.
//
//	@Summary		automations.startProposals (read)
//	@ID				automations.startProposals
//	@Description	Authorization: client:control.execute:self
//	@Description	Validation: D01; completed local weekdays and independently measured power transitions
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-C04
//	@Tags			automations
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	ops.Envelope{data=[]StartProposal}
//	@Failure		401	{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403	{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404	{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409	{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422	{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429	{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503	{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504	{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/automations/start-proposals [get]
func (Automations) startProposals(
	ctx context.Context,
	call *ops.Call,
	_ *struct{},
) ([]StartProposal, error) {
	rules, err := proposalRules(ctx, call)
	if err != nil {
		return nil, err
	}

	out := []StartProposal{}

	for _, rule := range rules {
		proposals, err := startProposalsFor(ctx, call, rule)
		if err != nil {
			return nil, err
		}

		out = append(out, proposals...)
	}

	return out, nil
}

// approveStartProposal serves automations.approveStartProposal.
//
//	@Summary		automations.approveStartProposal (write)
//	@ID				automations.approveStartProposal
//	@Description	Authorization: client:control.execute:self
//	@Description	Validation: D01; expectedVersion required; rechecks evidence and applies one weekday atomically
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C04
//	@Tags			automations
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string					true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer					true	"all: required (target automations, read automations.startProposals)"
//	@Param			id					path		string					true	"input field id"
//	@Param			request				body		StartProposalApproval	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Automation}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/automations/{id}/start-proposals [post]
func (m Automations) approveStartProposal(
	ctx context.Context, call *ops.Call, input *StartProposalApproval,
) (Automation, error) {
	watermark, err := proposalWatermark(ctx, call)
	if err != nil {
		return Automation{}, err
	}

	rule, err := m.load(ctx, call, input.ID, true)
	if err != nil {
		return Automation{}, err
	}

	if rule.Version != *call.ExpectedVersion {
		return Automation{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	proposals, err := startProposalsFor(ctx, call, rule)
	if err != nil {
		return Automation{}, err
	}

	for _, proposal := range proposals {
		if proposal.ID == input.ProposalID && proposal.Weekday == input.Weekday {
			if !proposalTimingSafe(call.Now, watermark, rule, proposal) {
				return Automation{}, apperr.E(apperr.Conflict, "errors.automation_proposal_timing")
			}

			return m.applyStartProposal(ctx, call, rule, proposal)
		}
	}

	return Automation{}, apperr.E(apperr.Conflict, proposalConflict)
}

func (m Automations) applyStartProposal(
	ctx context.Context, call *ops.Call, rule Automation, proposal StartProposal,
) (Automation, error) {
	input := scheduleProposalInput(rule, []int{proposal.Weekday}, proposal.To)
	if fields := input.Validate(); len(fields) != 0 {
		return Automation{}, apperr.Fields(fields)
	}

	var (
		applied Automation
		err     error
	)
	if len(rule.Weekdays) == 1 {
		applied, err = m.save(ctx, call, &input)
	} else {
		applied, err = m.splitStartSchedule(ctx, call, rule, &input)
	}

	if err != nil {
		return applied, err
	}

	weekday, unit, proposalID := strconv.Itoa(
		proposal.Weekday,
	), proposal.UnitID.String(), proposal.ID.String()

	dates, err := json.Marshal(proposal.SupportDates)
	if err != nil {
		return Automation{}, apperr.Wrap(err, "encode approval support dates")
	}

	support := string(dates)
	call.Audit(ops.AuditEntry{
		Action:          "automations.approveStartProposal",
		TargetKind:      kindAutomation,
		TargetID:        rule.ID.String(),
		PreviousVersion: call.ExpectedVersion,
		NextVersion:     new(rule.Version + 1),
		Reason:          "",
		Result:          "",
		Before:          map[string]*string{fieldStartLocal: &proposal.From},
		After: map[string]*string{
			fieldStartLocal:   &proposal.To,
			"approvedWeekday": &weekday,
			"unitId":          &unit,
			"proposalId":      &proposalID,
			"appliedRuleId":   new(applied.ID.String()),
			"supportDates":    &support,
		},
	})

	return applied, nil
}

func (m Automations) splitStartSchedule(
	ctx context.Context, call *ops.Call, rule Automation, changed *AutomationInput,
) (Automation, error) {
	remaining := slices.DeleteFunc(
		slices.Clone(rule.Weekdays),
		func(day int) bool { return day == changed.Weekdays[0] },
	)

	original := scheduleProposalInput(rule, remaining, rule.StartLocal)
	if fields := original.Validate(); len(fields) != 0 {
		return Automation{}, apperr.Fields(fields)
	}

	_, err := m.save(ctx, call, &original)
	if err != nil {
		return Automation{}, err
	}

	raw, err := ruleDefinition(call, changed)
	if err != nil {
		return Automation{}, err
	}

	id, err := insertAutomationOwned(
		ctx,
		call,
		changed,
		raw,
		rule.OwnerMembershipID,
		rule.CreatedByUserID,
	)
	if err != nil {
		return Automation{}, err
	}

	err = linkAutomationUnits(ctx, call, id, changed.UnitIDs)
	if err != nil {
		return Automation{}, err
	}

	return m.load(ctx, call, id, false)
}

// overlappingScheduleDays keeps suggestions away from existing rule arbitration. Schedules in another zone or
// spanning midnight could overlap on a different local weekday, so they conservatively block all days.
func overlappingScheduleDays(
	ctx context.Context,
	call *ops.Call,
	rule Automation,
) (map[int]bool, error) {
	rows, err := call.Tx.Query(ctx, `SELECT a.timezone, a.definition FROM control.automations a
		JOIN control.automation_units link ON link.automation_id = a.id
		WHERE link.unit_id = $1 AND a.id <> $2 AND a.kind = 'schedule' AND a.enabled`, rule.UnitIDs[0], rule.ID)
	if err != nil {
		return nil, apperr.Wrap(err, "select competing schedules")
	}
	defer rows.Close()

	blocked := map[int]bool{}

	for rows.Next() {
		var (
			zone string
			def  definition
		)

		err := rows.Scan(&zone, &def)
		if err != nil {
			return nil, apperr.Wrap(err, "read competing schedules")
		}

		days := def.Weekdays
		if zone != rule.Timezone || def.EndsNextDay == nil || *def.EndsNextDay {
			days = rule.Weekdays
		}

		for _, day := range days {
			blocked[day] = true
		}
	}

	return blocked, apperr.Wrap(rows.Err(), "read competing schedules")
}

// Lock in scheduler order before touching the rule, so a tick cannot evaluate the old and new rule during a split.
func proposalWatermark(ctx context.Context, call *ops.Call) (time.Time, error) {
	var watermark time.Time

	err := call.Tx.QueryRow(ctx, `SELECT evaluated_until FROM control.schedule_watermarks
		WHERE tenant_id = $1 FOR UPDATE`, call.Principal.TenantID).Scan(&watermark)
	if errors.Is(err, pgx.ErrNoRows) {
		return time.Time{}, nil // the first scheduler tick initializes its watermark without replaying history
	}

	return watermark, apperr.Wrap(err, "lock schedule watermark for proposal")
}

// An affected window already in progress keeps its original start and end. Also refuse changes that would alter
// unevaluated past occurrences, including a scheduler that is catching up after downtime.
func proposalTimingSafe(now, watermark time.Time, rule Automation, proposal StartProposal) bool {
	loc, err := time.LoadLocation(rule.Timezone)
	if err != nil {
		return false
	}

	day := now.In(loc)
	daysBack := (isoWeekday(day) - proposal.Weekday + isoSunday) % isoSunday
	day = day.AddDate(0, 0, -daysBack)
	start := min(proposal.From, proposal.To)
	wall := func(date time.Time, clock string) time.Time {
		minutes := minutesOf(clock)

		return time.Date(
			date.Year(),
			date.Month(),
			date.Day(),
			minutes/minutesPerHour,
			minutes%minutesPerHour,
			0,
			0,
			loc,
		)
	}
	end := wall(day, rule.EndLocal)

	if daysBack == 0 {
		switch {
		case now.Before(wall(day, start)):
			end = wall(day.AddDate(0, 0, -isoSunday), rule.EndLocal)
		case !now.After(end):
			return false
		}
	}

	return watermark.IsZero() || !watermark.Before(end)
}
