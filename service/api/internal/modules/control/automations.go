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
	Evaluate(ctx context.Context, c *ops.Call, unit uuid.UUID, facts []byte, at time.Time, eventID uuid.UUID, fire bool) ([]json.RawMessage, error)
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
	EndAction         json.RawMessage `json:"endAction,omitempty" swaggertype:"object"`
	Condition         json.RawMessage `json:"condition,omitempty" swaggertype:"object"`
	Action            json.RawMessage `json:"action,omitempty" swaggertype:"object"`
	OnlyIf            json.RawMessage `json:"onlyIf" swaggertype:"object"` // ExtraCondition[] (IR215), [] when none
	LastRun           *LastRun        `json:"lastRun"`                     // the latest automation_runs outcome of the rule (IR215)
	customerOrg       uuid.UUID
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
	EndAction   json.RawMessage `json:"endAction,omitempty" swaggertype:"object"`
	Condition   json.RawMessage `json:"condition,omitempty" swaggertype:"object"`
	Action      json.RawMessage `json:"action,omitempty" swaggertype:"object"`
	OnlyIf      json.RawMessage `json:"onlyIf,omitempty" swaggertype:"object"`
}

const automationCols = `a.id, a.tenant_id, a.version, a.created_at, a.updated_at, a.name, a.owner_membership_id, a.created_by_user_id, a.timezone, a.enabled, a.priority,
	a.disabled_reason, a.kind, a.definition, a.customer_org_id,
	COALESCE((SELECT array_agg(u.unit_id ORDER BY u.unit_id) FROM control.automation_units u WHERE u.automation_id = a.id), '{}'),
	(SELECT json_build_object('at', r.occurred_at, 'outcome', r.outcome, 'reason', r.skip_reason, 'commandId', r.command_id) FROM control.automation_runs r
		WHERE r.rule_id = a.id ORDER BY r.occurred_at DESC, r.seq DESC LIMIT 1)`

func scanAutomation(r pgx.Row) (Automation, error) {
	var x Automation
	var def, last []byte
	if err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.Name, &x.OwnerMembershipID, &x.CreatedByUserID, &x.Timezone, &x.Enabled, &x.Priority,
		&x.DisabledReason, &x.Kind, &def, &x.customerOrg, &x.UnitIDs, &last); err != nil {
		return x, err
	}
	var d definition
	_ = json.Unmarshal(def, &d)
	x.Weekdays, x.StartLocal, x.EndLocal, x.EndsNextDay, x.StartAction, x.EndAction, x.Condition, x.Action, x.OnlyIf =
		d.Weekdays, d.StartLocal, d.EndLocal, d.EndsNextDay, d.StartAction, d.EndAction, d.Condition, d.Action, d.OnlyIf
	if len(x.OnlyIf) == 0 {
		x.OnlyIf = json.RawMessage("[]")
	}
	if len(last) > 0 {
		var lr LastRun
		if err := json.Unmarshal(last, &lr); err == nil {
			lr.At = lr.At.UTC()
			x.LastRun = &lr
		}
	}
	return x, nil
}

func (m Automations) load(ctx context.Context, c *ops.Call, id uuid.UUID, lock bool) (Automation, error) {
	q := "SELECT " + automationCols + " FROM control.automations a WHERE a.id = $1"
	if lock {
		q += " FOR UPDATE OF a"
	}
	x, err := scanAutomation(c.Tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) || (err == nil && x.customerOrg != c.Principal.OrgID) {
		return Automation{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	return x, err
}

// @Summary		automations.list (read)
// @ID				automations.list
// @Description	Authorization: client:control.execute:self
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR215 lastRun = the latest run-log outcome (Command or skip with reason)
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-C04, DD-C01 · Query: filters unitId,enabled,kind · sort id,name,priority,createdAt,updatedAt (default createdAt desc;id desc)
// @Tags			automations
// @Accept			json
// @Produce		json
// @Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit	query		integer	false	"page size 1–100, default 25"
// @Param			sort	query		string	false	"field:direction — fields id,name,priority,createdAt,updatedAt; default createdAt desc;id desc"
// @Param			unitId	query		string	false	"filter → unitIds contains"
// @Param			enabled	query		boolean	false	"filter → enabled"
// @Param			kind	query		string	false	"filter → kind"
// @Success		200		{object}	ops.Envelope{data=AutomationPage}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/automations [get]
func (m Automations) list(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Automation], error) {
	var f struct {
		Kind    *string    `json:"kind,omitempty"`
		Enabled *bool      `json:"enabled,omitempty"`
		UnitID  *uuid.UUID `json:"unitId,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil || (f.Kind != nil && *f.Kind != "schedule" && *f.Kind != "event") {
			return paging.Page[Automation]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "a.id", "name": "a.name", "priority": "a.priority", "createdAt": "a.created_at", "updatedAt": "a.updated_at"}, "a.created_at DESC, a.id DESC")
	if err != nil {
		return paging.Page[Automation]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Automation]{}, err
	}
	args := []any{c.Principal.OrgID}
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"a.customer_org_id = $1"}
	if f.Kind != nil {
		conds = append(conds, "a.kind = "+add(*f.Kind))
	}
	if f.Enabled != nil {
		conds = append(conds, "a.enabled = "+add(*f.Enabled))
	}
	if f.UnitID != nil {
		conds = append(conds, "EXISTS (SELECT 1 FROM control.automation_units u WHERE u.automation_id = a.id AND u.unit_id = "+add(*f.UnitID)+")")
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM control.automations a WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Automation]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM control.automations a WHERE %s ORDER BY %s LIMIT %d OFFSET %d", automationCols, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Automation]{}, err
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
	if err := rows.Err(); err != nil {
		return paging.Page[Automation]{}, err
	}
	return paging.Page[Automation]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
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
	EndAction   json.RawMessage `json:"endAction,omitempty" swaggertype:"object"`
	Condition   json.RawMessage `json:"condition,omitempty" swaggertype:"object"`
	Action      json.RawMessage `json:"action,omitempty" swaggertype:"object"`
	OnlyIf      json.RawMessage `json:"onlyIf,omitempty" swaggertype:"object"`
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
	if len(raw) == 0 || string(raw) == "null" {
		return out, ""
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if dec.Decode(&out) != nil || len(out) > 3 {
		return nil, "error.invalid"
	}
	seen := map[string]bool{}
	for _, x := range out {
		if seen[x.Type] {
			return nil, "errors.duplicate_condition"
		}
		seen[x.Type] = true
		switch x.Type {
		case "weekday":
			if kind == "schedule" {
				return nil, "errors.weekday_on_schedule"
			}
			days := map[int]bool{}
			for _, d := range x.Weekdays {
				if d < 1 || d > 7 || days[d] {
					return nil, "error.invalid"
				}
				days[d] = true
			}
			if len(x.Weekdays) == 0 || x.Occupied != nil || x.Metric != nil || x.Operator != nil || x.Value != nil {
				return nil, "error.invalid"
			}
		case "occupancy":
			if x.Occupied == nil || x.Weekdays != nil || x.Metric != nil || x.Operator != nil || x.Value != nil {
				return nil, "error.invalid"
			}
		case "weather":
			if x.Metric == nil || *x.Metric != "temperature" || x.Operator == nil || !slices.Contains([]string{"gt", "gte", "lt", "lte"}, *x.Operator) ||
				x.Value == nil || *x.Value < -50 || *x.Value > 100 || x.Weekdays != nil || x.Occupied != nil {
				return nil, "error.invalid"
			}
		default:
			return nil, "error.invalid"
		}
		if x.Type == conditionType {
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
	var x condition
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if len(raw) == 0 || dec.Decode(&x) != nil {
		return x, false
	}
	switch x.Type {
	case "occupancy":
		return x, x.Occupied != nil && x.Event == nil && x.LocalTime == nil && x.Metric == nil && x.Operator == nil && x.Value == nil
	case "location":
		return x, x.Event != nil && (*x.Event == "arrival" || *x.Event == "departure") && x.Occupied == nil && x.LocalTime == nil && x.Metric == nil && x.Operator == nil && x.Value == nil
	case "pattern":
		return x, x.LocalTime != nil && hhmm.MatchString(*x.LocalTime) && x.Occupied == nil && x.Event == nil && x.Metric == nil && x.Operator == nil && x.Value == nil
	case "weather":
		return x, x.Metric != nil && *x.Metric == "temperature" && x.Operator != nil && slices.Contains([]string{"gt", "gte", "lt", "lte"}, *x.Operator) &&
			x.Value != nil && *x.Value >= -50 && *x.Value <= 100 && x.Occupied == nil && x.Event == nil && x.LocalTime == nil
	}
	return x, false
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
	fe := map[string]string{}
	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > 120 {
		fe["name"] = "error.length"
	}
	seen := map[uuid.UUID]bool{}
	for _, u := range in.UnitIDs {
		if u == uuid.Nil || seen[u] {
			fe["unitIds"] = "error.invalid"
		}
		seen[u] = true
	}
	if len(in.UnitIDs) == 0 || len(in.UnitIDs) > 50 {
		fe["unitIds"] = "error.invalid"
	}
	if !validTimezone(in.Timezone) {
		fe["timezone"] = "error.invalid"
	}
	if in.Priority < 0 || in.Priority > 100 {
		fe["priority"] = "error.range"
	}
	action := func(k string, raw json.RawMessage) {
		a, ok := ParseAction(raw)
		if !ok {
			fe[k] = "error.invalid"
			return
		}
		in.actions = append(in.actions, a)
	}
	switch in.Kind {
	case "schedule":
		if in.Condition != nil || in.Action != nil {
			fe["kind"] = "error.invalid"
		}
		validSchedule(fe, in.Weekdays, in.StartLocal, in.EndLocal, in.EndsNextDay)
		action("startAction", in.StartAction)
		action("endAction", in.EndAction)
	case "event":
		if in.Weekdays != nil || in.StartLocal != "" || in.EndLocal != "" || in.EndsNextDay != nil || in.StartAction != nil || in.EndAction != nil {
			fe["kind"] = "error.invalid"
		}
		cnd, ok := parseCondition(in.Condition)
		if !ok {
			fe["condition"] = "error.invalid"
		}
		in.condition = cnd
		action("action", in.Action)
	default:
		fe["kind"] = "error.invalid"
	}
	extras, why := parseExtras(in.OnlyIf, in.Kind, in.condition.Type)
	if why != "" {
		fe["onlyIf"] = why
	}
	in.extras = extras
	return fe
}

// validSchedule checks the schedule fields (DD-C04): ISO weekdays of the start day without duplicates, HH:mm times
// that differ, an end at or before the start only as an overnight schedule and never beyond 24 hours.
func validSchedule(fe map[string]string, weekdays []int, startLocal, endLocal string, endsNextDay *bool) {
	days := map[int]bool{}
	for _, d := range weekdays {
		if d < 1 || d > 7 || days[d] {
			fe["weekdays"] = "error.invalid"
		}
		days[d] = true
	}
	if len(weekdays) == 0 {
		fe["weekdays"] = "error.required"
	}
	if !hhmm.MatchString(startLocal) {
		fe["startLocal"] = "error.invalid"
	}
	if !hhmm.MatchString(endLocal) {
		fe["endLocal"] = "error.invalid"
	}
	if endsNextDay == nil {
		fe["endsNextDay"] = "error.required"
	}
	if fe["startLocal"] == "" && fe["endLocal"] == "" && endsNextDay != nil {
		s, e := minutesOf(startLocal), minutesOf(endLocal)
		switch {
		case s == e:
			fe["endLocal"] = "errors.same_as_start"
		case e < s && !*endsNextDay:
			fe["endsNextDay"] = "errors.overnight_required"
		case e > s && *endsNextDay:
			fe["endsNextDay"] = "errors.over_24_hours"
		}
	}
}

// occurrences lists the schedule's start/end instants from the start day d0 for days days, with the wall time
// problems found (nonexistent or ambiguous local times, D09).
func occurrences(def definition, loc *time.Location, d0 time.Time, days int) ([]ScheduledOccurrence, bool) {
	var out []ScheduledOccurrence
	bad := false
	at := func(day time.Time, hm string) time.Time {
		h, mi := minutesOf(hm)/60, minutesOf(hm)%60
		t := time.Date(day.Year(), day.Month(), day.Day(), h, mi, 0, 0, loc)
		lt := t.In(loc)
		if lt.Hour() != h || lt.Minute() != mi || lt.Day() != day.Day() { // nonexistent
			bad = true
		}
		for _, dd := range []time.Duration{-2 * time.Hour, -time.Hour, -30 * time.Minute, 30 * time.Minute, time.Hour, 2 * time.Hour} {
			u := t.Add(dd).In(loc)
			if u.Year() == day.Year() && u.Month() == day.Month() && u.Day() == day.Day() && u.Hour() == h && u.Minute() == mi { // ambiguous
				bad = true
			}
		}
		return t
	}
	for i := 0; i < days; i++ {
		day := time.Date(d0.Year(), d0.Month(), d0.Day()+i, 12, 0, 0, 0, loc)
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
		out = append(out, ScheduledOccurrence{Phase: "schedule_start", At: at(day, def.StartLocal).UTC(), Action: def.StartAction},
			ScheduledOccurrence{Phase: "schedule_end", At: at(end, def.EndLocal).UTC(), Action: def.EndAction})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At.Before(out[j].At) })
	return out, bad
}

// ScheduledOccurrence is ScheduledOccurrence of service-contracts.ts.
type ScheduledOccurrence struct {
	AutomationID *uuid.UUID      `json:"automationId"` // null for a draft preview
	Phase        string          `json:"phase"`
	At           time.Time       `json:"at"`
	Action       json.RawMessage `json:"action" swaggertype:"object"`
}

// @Summary		automations.save (write)
// @ID				automations.save
// @Description	Authorization: client:control.execute:self
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR215 onlyIf at most 3, distinct types, no weekday on a schedule, not the rule's own condition type
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-C04, DD-C05
// @Tags			automations
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		AutomationInput	true	"input"
// @Success		200				{object}	ops.Envelope{data=Automation}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/automations [post]
func (m Automations) save(ctx context.Context, c *ops.Call, in *AutomationInput) (Automation, error) {
	for _, u := range in.UnitIDs { // own, not archived units whose capabilities support every action
		t, found, err := m.Units.Target(ctx, c, u)
		if err != nil {
			return Automation{}, err
		}
		if !found || t.OrgID != c.Principal.OrgID {
			return Automation{}, apperr.E(apperr.NotFound, "error.notFound")
		}
		var archived bool
		if err := c.Tx.QueryRow(ctx, `SELECT archived FROM assets.units WHERE id = $1`, u).Scan(&archived); err != nil {
			return Automation{}, err
		}
		if archived {
			return Automation{}, apperr.E(apperr.Conflict, "errors.unit_archived")
		}
		for _, a := range in.actions {
			if !a.Supports(t.Caps) {
				return Automation{}, apperr.Fields(map[string]string{"unitIds": "error.unsupportedAction"})
			}
		}
	}
	if in.Kind == "event" && in.condition.Type == "location" { // location conditions need the owner's consent (SR02)
		granted, err := identity.ConsentGranted(ctx, c, c.Principal.MembershipID, "location_automation") // identity's record (IR194)
		if err != nil {
			return Automation{}, err
		}
		if !granted {
			return Automation{}, apperr.Fields(map[string]string{"condition": "errors.consent_required"})
		}
	}
	def := definition{Weekdays: in.Weekdays, StartLocal: in.StartLocal, EndLocal: in.EndLocal, EndsNextDay: in.EndsNextDay, StartAction: in.StartAction, EndAction: in.EndAction,
		Condition: in.Condition, Action: in.Action}
	if len(in.extras) > 0 {
		def.OnlyIf, _ = json.Marshal(in.extras)
	}
	if in.Kind == "schedule" {
		loc, _ := time.LoadLocation(in.Timezone)
		if _, bad := occurrences(def, loc, c.Now.In(loc), 367); bad {
			return Automation{}, apperr.Fields(map[string]string{"startLocal": "errors.local_time_invalid"})
		}
	}
	raw, _ := json.Marshal(def)
	var id uuid.UUID
	var prev *int
	version := 1
	if in.ID == nil {
		if err := c.Tx.QueryRow(ctx, `INSERT INTO control.automations (tenant_id, customer_org_id, kind, name, owner_membership_id, created_by_user_id, timezone, enabled, priority,
			definition, created_at, updated_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $10) RETURNING id`,
			c.Principal.OrgID, in.Kind, in.Name, c.Principal.MembershipID, c.Principal.UserID, in.Timezone, in.Enabled, in.Priority, raw, c.Now).Scan(&id); err != nil {
			return Automation{}, err
		}
	} else {
		cur, err := m.load(ctx, c, *in.ID, true)
		if err != nil {
			return cur, err
		}
		if cur.Version != *c.ExpectedVersion {
			return cur, apperr.E(apperr.Conflict, "error.versionConflict")
		}
		if cur.Kind != in.Kind {
			return cur, apperr.Fields(map[string]string{"kind": "error.immutable"})
		}
		// IR27: explicit enabling rechecks (done above) and clears the stop reason; editing while disabled keeps it
		if _, err := c.Tx.Exec(ctx, `UPDATE control.automations SET name = $2, timezone = $3, enabled = $4, priority = $5, definition = $6,
			disabled_reason = CASE WHEN $4 THEN NULL ELSE disabled_reason END, version = version + 1, updated_at = $7 WHERE id = $1`,
			cur.ID, in.Name, in.Timezone, in.Enabled, in.Priority, raw, c.Now); err != nil {
			return cur, err
		}
		if _, err := c.Tx.Exec(ctx, `DELETE FROM control.automation_units WHERE automation_id = $1`, cur.ID); err != nil {
			return cur, err
		}
		id, prev, version = cur.ID, c.ExpectedVersion, cur.Version+1
	}
	for _, u := range in.UnitIDs {
		if _, err := c.Tx.Exec(ctx, `INSERT INTO control.automation_units (tenant_id, automation_id, unit_id) VALUES (current_setting('app.tenant_id')::uuid, $1, $2)`, id, u); err != nil {
			return Automation{}, err
		}
	}
	c.Audit(ops.AuditEntry{Action: "automations.save", TargetKind: "automation", TargetID: id.String(), PreviousVersion: prev, NextVersion: &version})
	return m.load(ctx, c, id, false)
}

// ScheduleDraft is the unsaved schedule of the editor (automations.nextRuns draft, IR214).
type ScheduleDraft struct {
	Timezone    string          `json:"timezone"`
	Weekdays    []int           `json:"weekdays"`
	StartLocal  string          `json:"startLocal"`
	EndLocal    string          `json:"endLocal"`
	EndsNextDay *bool           `json:"endsNextDay"`
	StartAction json.RawMessage `json:"startAction" swaggertype:"object"`
	EndAction   json.RawMessage `json:"endAction" swaggertype:"object"`
}

// NextRunsInput is automations.nextRuns input: a saved schedule rule or an unsaved draft.
type NextRunsInput struct {
	AutomationID *uuid.UUID     `json:"automationId,omitempty"`
	Draft        *ScheduleDraft `json:"draft,omitempty"`
	Count        int            `json:"count"`
}

// Validate implements ops.Validator (count is fixed to 8; a draft is checked like automations.save, IR214).
func (in *NextRunsInput) Validate() map[string]string {
	fe := map[string]string{}
	switch {
	case (in.AutomationID == nil) == (in.Draft == nil):
		fe["automationId"] = "error.exactlyOneTarget"
	case in.AutomationID != nil && *in.AutomationID == uuid.Nil:
		fe["automationId"] = "error.required"
	case in.Draft != nil:
		d := in.Draft
		if !validTimezone(d.Timezone) {
			fe["draft.timezone"] = "error.invalid"
		}
		sub := map[string]string{}
		validSchedule(sub, d.Weekdays, d.StartLocal, d.EndLocal, d.EndsNextDay)
		for k, v := range sub {
			fe["draft."+k] = v
		}
		for k, raw := range map[string]json.RawMessage{"draft.startAction": d.StartAction, "draft.endAction": d.EndAction} {
			if _, ok := ParseAction(raw); !ok {
				fe[k] = "error.invalid"
			}
		}
	}
	if in.Count != 8 {
		fe["count"] = "error.invalid"
	}
	return fe
}

// nextRuns previews the next 8 start/end occurrences after now (D09): of a saved schedule rule — invalid local
// times skipped — or of an unsaved draft, whose nonexistent or ambiguous local time is VALIDATION as on save (IR214).
// Event rules have no schedule (VALIDATION).
//
//	@Summary		automations.nextRuns (read)
//	@ID				automations.nextRuns
//	@Description	Authorization: client:control.execute:self
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR214 draft preview validated like automations.save (nonexistent or ambiguous local time VALIDATION)
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
func (m Automations) nextRuns(ctx context.Context, c *ops.Call, in *NextRunsInput) ([]ScheduledOccurrence, error) {
	var id *uuid.UUID
	var def definition
	var tz string
	if in.Draft != nil {
		d := in.Draft
		def = definition{Weekdays: d.Weekdays, StartLocal: d.StartLocal, EndLocal: d.EndLocal, EndsNextDay: d.EndsNextDay, StartAction: d.StartAction, EndAction: d.EndAction}
		tz = d.Timezone
		loc, _ := time.LoadLocation(tz)
		if _, bad := occurrences(def, loc, c.Now.In(loc), 367); bad {
			return nil, apperr.Fields(map[string]string{"draft.startLocal": "errors.local_time_invalid"})
		}
	} else {
		x, err := m.load(ctx, c, *in.AutomationID, false)
		if err != nil {
			return nil, err
		}
		if x.Kind != "schedule" {
			return nil, apperr.Fields(map[string]string{"automationId": "errors.not_schedule"})
		}
		def = definition{Weekdays: x.Weekdays, StartLocal: x.StartLocal, EndLocal: x.EndLocal, EndsNextDay: x.EndsNextDay, StartAction: x.StartAction, EndAction: x.EndAction}
		tz, id = x.Timezone, &x.ID
	}
	loc, _ := time.LoadLocation(tz)
	occ, _ := occurrences(def, loc, c.Now.In(loc).AddDate(0, 0, -1), 30)
	out := []ScheduledOccurrence{}
	for _, o := range occ {
		if o.At.After(c.Now) && len(out) < in.Count {
			o.AutomationID = id
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
		return map[string]string{"id": "error.required"}
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
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR214 removes the rule and its unit links, keeps its Commands and run log
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
func (m Automations) delete(ctx context.Context, c *ops.Call, in *AutomationIDInput) (Deleted, error) {
	cur, err := m.load(ctx, c, in.ID, true)
	if err != nil {
		return Deleted{}, err
	}
	if cur.Version != *c.ExpectedVersion {
		return Deleted{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if _, err := c.Tx.Exec(ctx, `DELETE FROM control.automations WHERE id = $1`, cur.ID); err != nil { // units follow (cascade)
		return Deleted{}, err
	}
	c.Emit(ops.Event{AggregateType: "automation", AggregateID: cur.ID, Type: "AutomationDeleted", Payload: map[string]any{"kind": cur.Kind}})
	c.Audit(ops.AuditEntry{Action: "automations.delete", TargetKind: "automation", TargetID: cur.ID.String(), PreviousVersion: c.ExpectedVersion})
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
