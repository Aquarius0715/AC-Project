package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// Automations is the customer automation rule operation set (FR-C04/C05, D09, IR151).
type Automations struct {
	Units    Units
	Cmd      Commands
	Notifier Notifier
}

// Notifier evaluates alert-policy notifications per unit (Monitoring, SR21/SR25).
type Notifier interface {
	Evaluate(
		ctx context.Context,
		c *ops.Call,
		unit uuid.UUID,
		facts []byte,
		at time.Time,
		eventID uuid.UUID,
		fire bool,
	) ([]json.RawMessage, error)
}

// Automation is Automation of service-contracts.ts (RuleBase + the kind's definition fields).
type Automation struct {
	ID                uuid.UUID       `json:"id"`
	TenantID          uuid.UUID       `json:"tenantId"`
	Version           int             `json:"version"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	Name              string          `json:"name"`
	UnitIDs           []uuid.UUID     `json:"unitIds"`
	OwnerMembershipID uuid.UUID       `json:"ownerMembershipId"`
	CreatedByUserID   uuid.UUID       `json:"createdByUserId"`
	Timezone          string          `json:"timezone"`
	Enabled           bool            `json:"enabled"`
	Priority          int             `json:"priority"`
	DisabledReason    *string         `json:"disabledReason"`
	Kind              string          `json:"kind"`
	Weekdays          []int           `json:"weekdays,omitempty"`
	StartLocal        string          `json:"startLocal,omitempty"`
	EndLocal          string          `json:"endLocal,omitempty"`
	EndsNextDay       *bool           `json:"endsNextDay,omitempty"`
	StartAction       json.RawMessage `json:"startAction,omitempty" swaggertype:"object"`
	EndAction         json.RawMessage `json:"endAction,omitempty"   swaggertype:"object"`
	Condition         json.RawMessage `json:"condition,omitempty"   swaggertype:"object"`
	Action            json.RawMessage `json:"action,omitempty"      swaggertype:"object"`
	// ExtraCondition[] (IR215), [] when none
	OnlyIf json.RawMessage `json:"onlyIf" swaggertype:"object"`
	// the latest automation_runs outcome of the rule (IR215)
	LastRun     *LastRun `json:"lastRun"`
	customerOrg uuid.UUID
}

// LastRun is AutomationRun of service-contracts.ts: the latest command or recorded skip of the rule.
type LastRun struct {
	At        time.Time  `json:"at"`
	Outcome   string     `json:"outcome"`
	Reason    *string    `json:"reason"`
	CommandID *uuid.UUID `json:"commandId"`
}

// definition is the stored kind-specific part.
type definition struct {
	Weekdays    []int           `json:"weekdays,omitempty"`
	StartLocal  string          `json:"startLocal,omitempty"`
	EndLocal    string          `json:"endLocal,omitempty"`
	EndsNextDay *bool           `json:"endsNextDay,omitempty"`
	StartAction json.RawMessage `json:"startAction,omitempty" swaggertype:"object"`
	EndAction   json.RawMessage `json:"endAction,omitempty"   swaggertype:"object"`
	Condition   json.RawMessage `json:"condition,omitempty"   swaggertype:"object"`
	Action      json.RawMessage `json:"action,omitempty"      swaggertype:"object"`
	OnlyIf      json.RawMessage `json:"onlyIf,omitempty"      swaggertype:"object"`
}

const automationCols = `a.id, a.tenant_id, a.version, a.created_at, a.updated_at, a.name, a.owner_membership_id, ` +
	`a.created_by_user_id, a.timezone, a.enabled, a.priority,
	a.disabled_reason, a.kind, a.definition, a.customer_org_id,
	COALESCE((SELECT array_agg(u.unit_id ORDER BY u.unit_id) FROM control.automation_units u WHERE u.automation_id = ` +
	`a.id), '{}'),
	(SELECT json_build_object('at', r.occurred_at, 'outcome', r.outcome, 'reason', r.skip_reason, 'commandId', ` +
	`r.command_id) FROM control.automation_runs r
		WHERE r.rule_id = a.id ORDER BY r.occurred_at DESC, r.seq DESC LIMIT 1)`

func scanAutomation(row pgx.Row) (Automation, error) {
	var (
		automation Automation
		def, last  []byte
	)

	err := row.Scan(
		&automation.ID,
		&automation.TenantID,
		&automation.Version,
		&automation.CreatedAt,
		&automation.UpdatedAt,
		&automation.Name,
		&automation.OwnerMembershipID,
		&automation.CreatedByUserID,
		&automation.Timezone,
		&automation.Enabled,
		&automation.Priority,
		&automation.DisabledReason,
		&automation.Kind,
		&def,
		&automation.customerOrg,
		&automation.UnitIDs,
		&last,
	)
	if err != nil {
		return automation, apperr.Wrap(err, "scan (scanAutomation)")
	}

	var decoded definition

	_ = json.Unmarshal(def, &decoded)

	automation.Weekdays = decoded.Weekdays
	automation.StartLocal = decoded.StartLocal
	automation.EndLocal = decoded.EndLocal
	automation.EndsNextDay = decoded.EndsNextDay
	automation.StartAction = decoded.StartAction
	automation.EndAction = decoded.EndAction
	automation.Condition = decoded.Condition
	automation.Action = decoded.Action

	automation.OnlyIf = decoded.OnlyIf
	if len(automation.OnlyIf) == 0 {
		automation.OnlyIf = json.RawMessage("[]")
	}

	if len(last) > 0 {
		var lr LastRun

		err := json.Unmarshal(last, &lr)
		if err == nil {
			lr.At = lr.At.UTC()
			automation.LastRun = &lr
		}
	}

	return automation, nil
}

func (m Automations) load(
	ctx context.Context,
	call *ops.Call,
	automationID uuid.UUID,
	lock bool,
) (Automation, error) {
	q := "SELECT " + automationCols + " FROM control.automations a WHERE a.id = $1"
	if lock {
		q += " FOR UPDATE OF a"
	}

	x, err := scanAutomation(call.Tx.QueryRow(ctx, q, automationID))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && x.customerOrg != call.Principal.OrgID) {
		return Automation{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return x, err
}

// list serves automations.list.
//
//	@Summary		automations.list (read)
//	@ID				automations.list
//	@Description	Authorization: client:control.execute:self
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR215 lastRun = \
//	@Description	the latest run-log outcome (Command or skip with reason)
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-C04, DD-C01 · Query: filters unitId,enabled,kind · sort \
//	@Description	id,name,priority,createdAt,updatedAt (default createdAt desc;id desc)
//	@Tags			automations
//	@Accept			json
//	@Produce		json
//	@Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer	false	"page size 1–100, default 25"
//	@Param			sort	query		string	false	"field:direction — fields id,name,priority,createdAt,updatedAt; default createdAt desc;id desc"
//	@Param			unitId	query		string	false	"filter → unitIds contains"
//	@Param			enabled	query		boolean	false	"filter → enabled"
//	@Param			kind	query		string	false	"filter → kind"
//	@Success		200		{object}	ops.Envelope{data=AutomationPage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/automations [get]
func (m Automations) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Automation], error) {
	var filters struct {
		Kind    *string    `json:"kind,omitempty"`
		Enabled *bool      `json:"enabled,omitempty"`
		UnitID  *uuid.UUID `json:"unitId,omitempty"`
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filters) != nil ||
			(filters.Kind != nil && *filters.Kind != kindSchedule && *filters.Kind != kindEvent) {
			return paging.Page[Automation]{}, apperr.Fields(
				map[string]string{"filters": apperr.KeyInvalid},
			)
		}
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":        "a.id",
			"name":      "a.name",
			"priority":  "a.priority",
			"createdAt": "a.created_at",
			"updatedAt": "a.updated_at",
		},
		"a.created_at DESC, a.id DESC",
	)
	if err != nil {
		return paging.Page[Automation]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Automation]{}, apperr.Wrap(err, "paging.Resolve")
	}

	args := []any{call.Principal.OrgID}
	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{"a.customer_org_id = $1"}
	if filters.Kind != nil {
		conds = append(conds, "a.kind = "+add(*filters.Kind))
	}

	if filters.Enabled != nil {
		conds = append(conds, "a.enabled = "+add(*filters.Enabled))
	}

	if filters.UnitID != nil {
		conds = append(
			conds,
			"EXISTS (SELECT 1 FROM control.automation_units u WHERE u.automation_id = a.id AND u.unit_id = "+add(
				*filters.UnitID,
			)+")",
		)
	}

	where := strings.Join(conds, " AND ")

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM control.automations a WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Automation]{}, apperr.Wrap(scanErr, "select control.automations")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM control.automations a WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			automationCols,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[Automation]{}, apperr.Wrap(err, "select control.automations")
	}
	defer rows.Close()

	items := []Automation{}

	for rows.Next() {
		x, err := scanAutomation(rows)
		if err != nil {
			return paging.Page[Automation]{}, err
		}

		items = append(items, x)
	}

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[Automation]{}, apperr.Wrap(rowsErr, "select control.automations")
	}

	return paging.Page[Automation]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// AutomationInput is automations.save input.
type AutomationInput struct {
	ID          *uuid.UUID      `json:"id,omitempty"`
	Name        string          `json:"name"`
	UnitIDs     []uuid.UUID     `json:"unitIds"`
	Timezone    string          `json:"timezone"`
	Enabled     bool            `json:"enabled"`
	Priority    int             `json:"priority"`
	Kind        string          `json:"kind"`
	Weekdays    []int           `json:"weekdays,omitempty"`
	StartLocal  string          `json:"startLocal,omitempty"`
	EndLocal    string          `json:"endLocal,omitempty"`
	EndsNextDay *bool           `json:"endsNextDay,omitempty"`
	StartAction json.RawMessage `json:"startAction,omitempty" swaggertype:"object"`
	EndAction   json.RawMessage `json:"endAction,omitempty"   swaggertype:"object"`
	Condition   json.RawMessage `json:"condition,omitempty"   swaggertype:"object"`
	Action      json.RawMessage `json:"action,omitempty"      swaggertype:"object"`
	OnlyIf      json.RawMessage `json:"onlyIf,omitempty"      swaggertype:"object"`
	actions     []UnitAction
	condition   condition
	extras      []extraCondition
}

// extraCondition is one “Only if …” condition (Figma Client 03b, IR215): every one must hold when the rule runs.
type extraCondition struct {
	Type     string   `json:"type"`
	Weekdays []int    `json:"weekdays,omitempty"`
	Occupied *bool    `json:"occupied,omitempty"`
	Metric   *string  `json:"metric,omitempty"`
	Operator *string  `json:"operator,omitempty"`
	Value    *float64 `json:"value,omitempty"`
}

// parseExtras checks onlyIf (IR215): at most 3 of weekday / occupancy / weather, each type once, no weekday on a
// schedule (it has its own weekdays) and not the type of the rule's own condition. Returns the error key or "".
func parseExtras(raw json.RawMessage, kind, conditionType string) ([]extraCondition, string) {
	out := []extraCondition{}
	if len(raw) == 0 || string(raw) == jsonNull {
		return out, ""
	}

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	if dec.Decode(&out) != nil || len(out) > 3 {
		return nil, apperr.KeyInvalid
	}

	seen := map[string]bool{}
	for _, extra := range out {
		if seen[extra.Type] {
			return nil, "errors.duplicate_condition"
		}

		seen[extra.Type] = true
		switch extra.Type {
		case "weekday":
			if kind == kindSchedule {
				return nil, "errors.weekday_on_schedule"
			}

			days := map[int]bool{}
			for _, d := range extra.Weekdays {
				if d < 1 || d > 7 || days[d] {
					return nil, apperr.KeyInvalid
				}

				days[d] = true
			}

			if len(extra.Weekdays) == 0 || extra.Occupied != nil || extra.Metric != nil ||
				extra.Operator != nil ||
				extra.Value != nil {
				return nil, apperr.KeyInvalid
			}
		case condOccupancy:
			if extra.Occupied == nil || extra.Weekdays != nil || extra.Metric != nil ||
				extra.Operator != nil ||
				extra.Value != nil {
				return nil, apperr.KeyInvalid
			}
		case condWeather:
			if extra.Metric == nil || *extra.Metric != metricTemperature || extra.Operator == nil ||
				!slices.Contains([]string{"gt", opGTE, "lt", "lte"}, *extra.Operator) ||
				extra.Value == nil ||
				*extra.Value < -50 ||
				*extra.Value > 100 ||
				extra.Weekdays != nil ||
				extra.Occupied != nil {
				return nil, apperr.KeyInvalid
			}
		default:
			return nil, apperr.KeyInvalid
		}

		if extra.Type == conditionType {
			return nil, "errors.duplicate_condition"
		}
	}

	return out, ""
}

var hhmm = regexp.MustCompile(`^([01][0-9]|2[0-3]):[0-5][0-9]$`)

// condition is the customer automation Condition subset (DD-C05): occupancy, location, pattern, weather.
type condition struct {
	Type      string   `json:"type"`
	Occupied  *bool    `json:"occupied,omitempty"`
	Event     *string  `json:"event,omitempty"`
	LocalTime *string  `json:"localTime,omitempty"`
	Metric    *string  `json:"metric,omitempty"`
	Operator  *string  `json:"operator,omitempty"`
	Value     *float64 `json:"value,omitempty"`
}

func parseCondition(raw json.RawMessage) (condition, bool) {
	var cond condition

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	if len(raw) == 0 || dec.Decode(&cond) != nil {
		return cond, false
	}

	switch cond.Type {
	case condOccupancy:
		return cond, cond.Occupied != nil && cond.Event == nil && cond.LocalTime == nil &&
			cond.Metric == nil &&
			cond.Operator == nil &&
			cond.Value == nil
	case condLocation:
		return cond, cond.Event != nil &&
			(*cond.Event == "arrival" || *cond.Event == "departure") &&
			cond.Occupied == nil &&
			cond.LocalTime == nil &&
			cond.Metric == nil &&
			cond.Operator == nil &&
			cond.Value == nil
	case "pattern":
		return cond, cond.LocalTime != nil && hhmm.MatchString(*cond.LocalTime) &&
			cond.Occupied == nil &&
			cond.Event == nil &&
			cond.Metric == nil &&
			cond.Operator == nil &&
			cond.Value == nil
	case condWeather:
		return cond, cond.Metric != nil && *cond.Metric == metricTemperature &&
			cond.Operator != nil &&
			slices.Contains([]string{"gt", opGTE, "lt", "lte"}, *cond.Operator) &&
			cond.Value != nil &&
			*cond.Value >= -50 &&
			*cond.Value <= 100 &&
			cond.Occupied == nil &&
			cond.Event == nil &&
			cond.LocalTime == nil
	}

	return cond, false
}

func minutesOf(s string) int {
	return int(s[0]-'0')*600 + int(s[1]-'0')*60 + int(s[3]-'0')*10 + int(s[4]-'0')
}

// validTimezone accepts UTC or an Area/City IANA name.
func validTimezone(tz string) bool {
	if tz != "UTC" && !strings.Contains(tz, "/") {
		return false
	}

	_, err := time.LoadLocation(tz)

	return err == nil
}

// Validate implements ops.Validator (DD-C04 / DD-C05 / D09).
func (in *AutomationInput) Validate() map[string]string {
	fieldErrs := map[string]string{}

	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > 120 {
		fieldErrs["name"] = apperr.KeyLength
	}

	seen := map[uuid.UUID]bool{}
	for _, u := range in.UnitIDs {
		if u == uuid.Nil || seen[u] {
			fieldErrs["unitIds"] = apperr.KeyInvalid
		}

		seen[u] = true
	}

	if len(in.UnitIDs) == 0 || len(in.UnitIDs) > 50 {
		fieldErrs["unitIds"] = apperr.KeyInvalid
	}

	if !validTimezone(in.Timezone) {
		fieldErrs["timezone"] = apperr.KeyInvalid
	}

	if in.Priority < 0 || in.Priority > 100 {
		fieldErrs["priority"] = apperr.KeyRange
	}

	action := func(k string, raw json.RawMessage) {
		unitAction, ok := ParseAction(raw)
		if !ok {
			fieldErrs[k] = apperr.KeyInvalid

			return
		}

		in.actions = append(in.actions, unitAction)
	}

	switch in.Kind {
	case kindSchedule:
		if in.Condition != nil || in.Action != nil {
			fieldErrs["kind"] = apperr.KeyInvalid
		}

		validSchedule(fieldErrs, in.Weekdays, in.StartLocal, in.EndLocal, in.EndsNextDay)
		action("startAction", in.StartAction)
		action("endAction", in.EndAction)
	case kindEvent:
		if in.Weekdays != nil || in.StartLocal != "" || in.EndLocal != "" ||
			in.EndsNextDay != nil ||
			in.StartAction != nil ||
			in.EndAction != nil {
			fieldErrs["kind"] = apperr.KeyInvalid
		}

		cnd, ok := parseCondition(in.Condition)
		if !ok {
			fieldErrs[keyCondition] = apperr.KeyInvalid
		}

		in.condition = cnd
		action(keyAction, in.Action)
	default:
		fieldErrs["kind"] = apperr.KeyInvalid
	}

	extras, why := parseExtras(in.OnlyIf, in.Kind, in.condition.Type)
	if why != "" {
		fieldErrs["onlyIf"] = why
	}

	in.extras = extras

	return fieldErrs
}

// validSchedule checks the schedule fields (DD-C04): ISO weekdays of the start day without duplicates, HH:mm times
// that differ, an end at or before the start only as an overnight schedule and never beyond 24 hours.
func validSchedule(
	fieldErrs map[string]string,
	weekdays []int,
	startLocal, endLocal string,
	endsNextDay *bool,
) {
	days := map[int]bool{}
	for _, d := range weekdays {
		if d < 1 || d > 7 || days[d] {
			fieldErrs["weekdays"] = apperr.KeyInvalid
		}

		days[d] = true
	}

	if len(weekdays) == 0 {
		fieldErrs["weekdays"] = apperr.KeyRequired
	}

	if !hhmm.MatchString(startLocal) {
		fieldErrs["startLocal"] = apperr.KeyInvalid
	}

	if !hhmm.MatchString(endLocal) {
		fieldErrs["endLocal"] = apperr.KeyInvalid
	}

	if endsNextDay == nil {
		fieldErrs["endsNextDay"] = apperr.KeyRequired
	}

	if fieldErrs["startLocal"] == "" && fieldErrs["endLocal"] == "" && endsNextDay != nil {
		startMinute, endMinute := minutesOf(startLocal), minutesOf(endLocal)
		switch {
		case startMinute == endMinute:
			fieldErrs["endLocal"] = "errors.same_as_start"
		case endMinute < startMinute && !*endsNextDay:
			fieldErrs["endsNextDay"] = "errors.overnight_required"
		case endMinute > startMinute && *endsNextDay:
			fieldErrs["endsNextDay"] = "errors.over_24_hours"
		}
	}
}

// occurrences lists the schedule's start/end instants from the start day firstDay for days days, with the wall time
// problems found (nonexistent or ambiguous local times, D09).
func occurrences(
	def definition,
	loc *time.Location,
	firstDay time.Time,
	days int,
) ([]ScheduledOccurrence, bool) {
	var out []ScheduledOccurrence

	bad := false

	instantOf := func(day time.Time, hm string) time.Time {
		h, mi := minutesOf(hm)/60, minutesOf(hm)%60
		instant := time.Date(day.Year(), day.Month(), day.Day(), h, mi, 0, 0, loc)

		lt := instant.In(loc)
		if lt.Hour() != h || lt.Minute() != mi || lt.Day() != day.Day() { // nonexistent
			bad = true
		}

		for _, dd := range []time.Duration{
			-2 * time.Hour, -time.Hour, -30 * time.Minute, 30 * time.Minute, time.Hour,
			2 * time.Hour,
		} {
			u := instant.Add(dd).In(loc)
			if u.Year() == day.Year() && u.Month() == day.Month() && u.Day() == day.Day() &&
				u.Hour() == h &&
				u.Minute() == mi { // ambiguous
				bad = true
			}
		}

		return instant
	}
	for i := range days {
		day := time.Date(firstDay.Year(), firstDay.Month(), firstDay.Day()+i, 12, 0, 0, 0, loc)

		wd := int(day.Weekday())
		if wd == 0 {
			wd = 7
		}

		if !slices.Contains(def.Weekdays, wd) {
			continue
		}

		end := day
		if def.EndsNextDay != nil && *def.EndsNextDay {
			end = time.Date(day.Year(), day.Month(), day.Day()+1, 12, 0, 0, 0, loc)
		}

		out = append(
			out,
			ScheduledOccurrence{
				Phase:  phaseScheduleStart,
				At:     instantOf(day, def.StartLocal).UTC(),
				Action: def.StartAction, AutomationID: nil,
			},
			ScheduledOccurrence{
				Phase:  phaseScheduleEnd,
				At:     instantOf(end, def.EndLocal).UTC(),
				Action: def.EndAction, AutomationID: nil,
			},
		)
	}

	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })

	return out, bad
}

// ScheduledOccurrence is ScheduledOccurrence of service-contracts.ts.
type ScheduledOccurrence struct {
	AutomationID *uuid.UUID      `json:"automationId"` // null for a draft preview
	Phase        string          `json:"phase"`
	At           time.Time       `json:"at"`
	Action       json.RawMessage `json:"action"       swaggertype:"object"`
}

// save serves automations.save.
//
//	@Summary		automations.save (write)
//	@ID				automations.save
//	@Description	Authorization: client:control.execute:self
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR215 onlyIf at most 3, distinct types, no weekday on a schedule, not the rule's own condition type
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C04, DD-C05
//	@Tags			automations
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		AutomationInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Automation}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/automations [post]
func (m Automations) save(
	ctx context.Context,
	call *ops.Call,
	input *AutomationInput,
) (Automation, error) {
	for _, unitID := range input.UnitIDs { // own, not archived units whose capabilities support every action
		target, found, err := m.Units.Target(ctx, call, unitID)
		if err != nil {
			return Automation{}, apperr.Wrap(err, "control.Units.Target")
		}

		if !found || target.OrgID != call.Principal.OrgID {
			return Automation{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}

		var archived bool

		scanErr := call.Tx.QueryRow(ctx, `SELECT archived FROM assets.units WHERE id = $1`, unitID).
			Scan(&archived)
		if scanErr != nil {
			return Automation{}, apperr.Wrap(scanErr, "select assets.units")
		}

		if archived {
			return Automation{}, apperr.E(apperr.Conflict, "errors.unit_archived")
		}

		for _, a := range input.actions {
			if !a.Supports(target.Caps) {
				return Automation{}, apperr.Fields(
					map[string]string{"unitIds": apperr.KeyUnsupportedAction},
				)
			}
		}
	}

	if input.Kind == kindEvent &&
		input.condition.Type == condLocation { // location conditions need the owner's consent (SR02)
		granted, err := identity.ConsentGranted(
			ctx,
			call,
			call.Principal.MembershipID,
			"location_automation",
		) // identity's record (IR194)
		if err != nil {
			return Automation{}, apperr.Wrap(err, "identity.ConsentGranted")
		}

		if !granted {
			return Automation{}, apperr.Fields(
				map[string]string{keyCondition: "errors.consent_required"},
			)
		}
	}

	def := definition{
		Weekdays:    input.Weekdays,
		StartLocal:  input.StartLocal,
		EndLocal:    input.EndLocal,
		EndsNextDay: input.EndsNextDay,
		StartAction: input.StartAction,
		EndAction:   input.EndAction,
		Condition:   input.Condition,
		Action:      input.Action, OnlyIf: nil,
	}
	if len(input.extras) > 0 {
		def.OnlyIf, _ = json.Marshal(input.extras)
	}

	if input.Kind == kindSchedule {
		loc, _ := time.LoadLocation(input.Timezone)
		if _, bad := occurrences(def, loc, call.Now.In(loc), 367); bad {
			return Automation{}, apperr.Fields(
				map[string]string{"startLocal": "errors.local_time_invalid"},
			)
		}
	}

	raw, _ := json.Marshal(def)

	var (
		automationID uuid.UUID
		prev         *int
	)

	version := 1

	if input.ID == nil {
		err := call.Tx.QueryRow(ctx, `INSERT INTO control.automations (tenant_id, customer_org_id, kind, name,
			owner_membership_id, created_by_user_id, timezone, enabled, priority,
			definition, created_at, updated_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, $7, $8,
			$9, $10, $10) RETURNING id`,
			call.Principal.OrgID, input.Kind, input.Name, call.Principal.MembershipID, call.Principal.UserID, input.Timezone,
			input.Enabled,
			input.Priority, raw, call.Now).
			Scan(&automationID)
		if err != nil {
			return Automation{}, apperr.Wrap(err, "insert control.automations")
		}
	} else {
		cur, err := m.load(ctx, call, *input.ID, true)
		if err != nil {
			return cur, err
		}

		if cur.Version != *call.ExpectedVersion {
			return cur, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
		}

		if cur.Kind != input.Kind {
			return cur, apperr.Fields(map[string]string{"kind": apperr.KeyImmutable})
		}
		// IR27: explicit enabling rechecks (done above) and clears the stop reason; editing while disabled keeps it
		_, execErr := call.Tx.Exec(
			ctx,
			`UPDATE control.automations SET name = $2, timezone = $3, enabled = $4, priority = $5, definition = $6,
			disabled_reason = CASE WHEN $4 THEN NULL ELSE disabled_reason END, version = version + 1, updated_at = $7 WHERE id =
			$1`,
			cur.ID,
			input.Name,
			input.Timezone,
			input.Enabled,
			input.Priority,
			raw,
			call.Now,
		)
		if execErr != nil {
			return cur, apperr.Wrap(execErr, "update control.automations")
		}

		_, execErr = call.Tx.Exec(
			ctx,
			`DELETE FROM control.automation_units WHERE automation_id = $1`,
			cur.ID,
		)
		if execErr != nil {
			return cur, apperr.Wrap(execErr, "delete control.automation_units")
		}

		automationID, prev, version = cur.ID, call.ExpectedVersion, cur.Version+1
	}

	for _, unitID := range input.UnitIDs {
		_, err := call.Tx.Exec(
			ctx,
			`INSERT INTO control.automation_units (tenant_id, automation_id, unit_id) VALUES
				(current_setting('app.tenant_id')::uuid, $1, $2)`,
			automationID,
			unitID,
		)
		if err != nil {
			return Automation{}, apperr.Wrap(err, "insert control.automation_units")
		}
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "automations.save",
			TargetKind:      kindAutomation,
			TargetID:        automationID.String(),
			PreviousVersion: prev,
			NextVersion:     &version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return m.load(ctx, call, automationID, false)
}

// ScheduleDraft is the unsaved schedule of the editor (automations.nextRuns draft, IR214).
type ScheduleDraft struct {
	Timezone    string          `json:"timezone"`
	Weekdays    []int           `json:"weekdays"`
	StartLocal  string          `json:"startLocal"`
	EndLocal    string          `json:"endLocal"`
	EndsNextDay *bool           `json:"endsNextDay"`
	StartAction json.RawMessage `json:"startAction" swaggertype:"object"`
	EndAction   json.RawMessage `json:"endAction"   swaggertype:"object"`
}

// NextRunsInput is automations.nextRuns input: a saved schedule rule or an unsaved draft.
type NextRunsInput struct {
	AutomationID *uuid.UUID     `json:"automationId,omitempty"`
	Draft        *ScheduleDraft `json:"draft,omitempty"`
	Count        int            `json:"count"`
}

// Validate implements ops.Validator (count is fixed to 8; a draft is checked like automations.save, IR214).
func (in *NextRunsInput) Validate() map[string]string {
	fieldErrs := map[string]string{}

	switch {
	case (in.AutomationID == nil) == (in.Draft == nil):
		fieldErrs["automationId"] = apperr.KeyExactlyOneTarget
	case in.AutomationID != nil && *in.AutomationID == uuid.Nil:
		fieldErrs["automationId"] = apperr.KeyRequired
	case in.Draft != nil:
		scheduleDraft := in.Draft
		if !validTimezone(scheduleDraft.Timezone) {
			fieldErrs["draft.timezone"] = apperr.KeyInvalid
		}

		sub := map[string]string{}
		validSchedule(
			sub,
			scheduleDraft.Weekdays,
			scheduleDraft.StartLocal,
			scheduleDraft.EndLocal,
			scheduleDraft.EndsNextDay,
		)

		for k, v := range sub {
			fieldErrs["draft."+k] = v
		}

		for k, raw := range map[string]json.RawMessage{
			"draft.startAction": scheduleDraft.StartAction,
			"draft.endAction":   scheduleDraft.EndAction,
		} {
			if _, ok := ParseAction(raw); !ok {
				fieldErrs[k] = apperr.KeyInvalid
			}
		}
	}

	if in.Count != 8 {
		fieldErrs["count"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// nextRuns previews the next 8 start/end occurrences after now (D09): of a saved schedule rule — invalid local
// times skipped — or of an unsaved draft, whose nonexistent or ambiguous local time is VALIDATION as on save (IR214).
// Event rules have no schedule (VALIDATION).
//
//	@Summary		automations.nextRuns (read)
//	@ID				automations.nextRuns
//	@Description	Authorization: client:control.execute:self
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR214 draft \
//	@Description	preview validated like automations.save (nonexistent or ambiguous local time VALIDATION)
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-C04, DD-C01
//	@Tags			automations
//	@Accept			json
//	@Produce		json
//	@Param			request	body		NextRunsInput	true	"input"
//	@Success		200		{object}	ops.Envelope{data=[]ScheduledOccurrence}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/automations/next-runs [post]
func (m Automations) nextRuns(
	ctx context.Context,
	call *ops.Call,
	input *NextRunsInput,
) ([]ScheduledOccurrence, error) {
	var (
		automationID *uuid.UUID
		def          definition
		timezone     string
	)

	if input.Draft != nil {
		scheduleDraft := input.Draft
		def = definition{
			Weekdays:    scheduleDraft.Weekdays,
			StartLocal:  scheduleDraft.StartLocal,
			EndLocal:    scheduleDraft.EndLocal,
			EndsNextDay: scheduleDraft.EndsNextDay,
			StartAction: scheduleDraft.StartAction,
			EndAction:   scheduleDraft.EndAction, Condition: nil, Action: nil, OnlyIf: nil,
		}
		timezone = scheduleDraft.Timezone

		loc, _ := time.LoadLocation(timezone)
		if _, bad := occurrences(def, loc, call.Now.In(loc), 367); bad {
			return nil, apperr.Fields(
				map[string]string{"draft.startLocal": "errors.local_time_invalid"},
			)
		}
	} else {
		automation, err := m.load(ctx, call, *input.AutomationID, false)
		if err != nil {
			return nil, err
		}

		if automation.Kind != kindSchedule {
			return nil, apperr.Fields(map[string]string{"automationId": "errors.not_schedule"})
		}

		def = definition{
			Weekdays:    automation.Weekdays,
			StartLocal:  automation.StartLocal,
			EndLocal:    automation.EndLocal,
			EndsNextDay: automation.EndsNextDay,
			StartAction: automation.StartAction,
			EndAction:   automation.EndAction, Condition: nil, Action: nil, OnlyIf: nil,
		}
		timezone, automationID = automation.Timezone, &automation.ID
	}

	loc, _ := time.LoadLocation(timezone)
	occ, _ := occurrences(def, loc, call.Now.In(loc).AddDate(0, 0, -1), 30)

	out := []ScheduledOccurrence{}
	for _, o := range occ {
		if o.At.After(call.Now) && len(out) < input.Count {
			o.AutomationID = automationID
			out = append(out, o)
		}
	}

	return out, nil
}

// AutomationIDInput is automations.delete input.
type AutomationIDInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *AutomationIDInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": apperr.KeyRequired}
	}

	return nil
}

// Deleted is DeletedResource of service-contracts.ts.
type Deleted struct {
	ID      uuid.UUID `json:"id"`
	Deleted bool      `json:"deleted"`
}

// delete removes a customer automation (Figma Client 03g–03i, IR214): it no longer runs from this transition on; the
// Commands it created and its run log stay as history, other automations and manual control are untouched.
//
//	@Summary		automations.delete (write)
//	@ID				automations.delete
//	@Description	Authorization: client:control.execute:self
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR214 removes the rule and its unit links, keeps its Commands and run log
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C04
//	@Tags			automations
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target automations, read automations.list)"
//	@Param			id					path		string	true	"input field id"
//	@Success		200					{object}	ops.Envelope{data=Deleted}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/automations/{id} [delete]
func (m Automations) delete(
	ctx context.Context,
	call *ops.Call,
	in *AutomationIDInput,
) (Deleted, error) {
	cur, err := m.load(ctx, call, in.ID, true)
	if err != nil {
		return Deleted{}, err
	}

	if cur.Version != *call.ExpectedVersion {
		return Deleted{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`DELETE FROM control.automations WHERE id = $1`,
		cur.ID,
	)
	if execErr != nil { // units follow (cascade)
		return Deleted{}, apperr.Wrap(execErr, "delete control.automations")
	}

	call.Emit(
		ops.Event{
			AggregateType: kindAutomation,
			AggregateID:   cur.ID,
			Type:          "AutomationDeleted",
			Payload:       map[string]any{"kind": cur.Kind},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "automations.delete",
			TargetKind:      kindAutomation,
			TargetID:        cur.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     nil,
			Reason:          "",
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)

	return Deleted{ID: cur.ID, Deleted: true}, nil
}

// RegisterAutomations binds automations.save / list / nextRuns / delete.
func RegisterAutomations(r *ops.Registry, m Automations) {
	ops.Register(r, "automations.save", m.save)
	ops.Register(r, "automations.list", m.list)
	ops.Register(r, "automations.nextRuns", m.nextRuns)
	ops.Register(r, "automations.delete", m.delete)
	RegisterEvaluation(r, m)
}
