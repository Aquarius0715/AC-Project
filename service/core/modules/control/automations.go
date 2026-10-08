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

	"github.com/pradita/ac-project/service/core/ops"
	"github.com/pradita/ac-project/service/core/platform/apperr"
	"github.com/pradita/ac-project/service/core/platform/paging"
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
	StartAction       json.RawMessage `json:"startAction,omitempty"`
	EndAction         json.RawMessage `json:"endAction,omitempty"`
	Condition         json.RawMessage `json:"condition,omitempty"`
	Action            json.RawMessage `json:"action,omitempty"`
	customerOrg       uuid.UUID
}

// definition is the stored kind-specific part.
type definition struct {
	Weekdays    []int           `json:"weekdays,omitempty"`
	StartLocal  string          `json:"startLocal,omitempty"`
	EndLocal    string          `json:"endLocal,omitempty"`
	EndsNextDay *bool           `json:"endsNextDay,omitempty"`
	StartAction json.RawMessage `json:"startAction,omitempty"`
	EndAction   json.RawMessage `json:"endAction,omitempty"`
	Condition   json.RawMessage `json:"condition,omitempty"`
	Action      json.RawMessage `json:"action,omitempty"`
}

const automationCols = `a.id, a.tenant_id, a.version, a.created_at, a.updated_at, a.name, a.owner_membership_id, a.created_by_user_id, a.timezone, a.enabled, a.priority,
	a.disabled_reason, a.kind, a.definition, a.customer_org_id,
	COALESCE((SELECT array_agg(u.unit_id ORDER BY u.unit_id) FROM control.automation_units u WHERE u.automation_id = a.id), '{}')`

func scanAutomation(r pgx.Row) (Automation, error) {
	var x Automation
	var def []byte
	if err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.Name, &x.OwnerMembershipID, &x.CreatedByUserID, &x.Timezone, &x.Enabled, &x.Priority,
		&x.DisabledReason, &x.Kind, &def, &x.customerOrg, &x.UnitIDs); err != nil {
		return x, err
	}
	var d definition
	_ = json.Unmarshal(def, &d)
	x.Weekdays, x.StartLocal, x.EndLocal, x.EndsNextDay, x.StartAction, x.EndAction, x.Condition, x.Action =
		d.Weekdays, d.StartLocal, d.EndLocal, d.EndsNextDay, d.StartAction, d.EndAction, d.Condition, d.Action
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
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "a.id", "name": "a.name", "priority": "a.priority", "createdAt": "a.created_at"}, "a.created_at DESC, a.id DESC")
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
	StartAction json.RawMessage `json:"startAction,omitempty"`
	EndAction   json.RawMessage `json:"endAction,omitempty"`
	Condition   json.RawMessage `json:"condition,omitempty"`
	Action      json.RawMessage `json:"action,omitempty"`
	actions     []UnitAction
	condition   condition
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
		days := map[int]bool{}
		for _, d := range in.Weekdays {
			if d < 1 || d > 7 || days[d] {
				fe["weekdays"] = "error.invalid"
			}
			days[d] = true
		}
		if len(in.Weekdays) == 0 {
			fe["weekdays"] = "error.required"
		}
		if !hhmm.MatchString(in.StartLocal) {
			fe["startLocal"] = "error.invalid"
		}
		if !hhmm.MatchString(in.EndLocal) {
			fe["endLocal"] = "error.invalid"
		}
		if in.EndsNextDay == nil {
			fe["endsNextDay"] = "error.required"
		}
		if fe["startLocal"] == "" && fe["endLocal"] == "" && in.EndsNextDay != nil {
			s, e := minutesOf(in.StartLocal), minutesOf(in.EndLocal)
			switch {
			case s == e:
				fe["endLocal"] = "errors.same_as_start"
			case e < s && !*in.EndsNextDay:
				fe["endsNextDay"] = "errors.overnight_required"
			case e > s && *in.EndsNextDay:
				fe["endsNextDay"] = "errors.over_24_hours"
			}
		}
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
	return fe
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
	AutomationID uuid.UUID       `json:"automationId"`
	Phase        string          `json:"phase"`
	At           time.Time       `json:"at"`
	Action       json.RawMessage `json:"action"`
}

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
		var granted bool
		err := c.Tx.QueryRow(ctx, `SELECT granted FROM identity.consents WHERE membership_id = $1 AND purpose = 'location_automation'`, c.Principal.MembershipID).Scan(&granted)
		if err != nil && !errors.Is(err, pgx.ErrNoRows) {
			return Automation{}, err
		}
		if !granted {
			return Automation{}, apperr.Fields(map[string]string{"condition": "errors.consent_required"})
		}
	}
	def := definition{Weekdays: in.Weekdays, StartLocal: in.StartLocal, EndLocal: in.EndLocal, EndsNextDay: in.EndsNextDay, StartAction: in.StartAction, EndAction: in.EndAction,
		Condition: in.Condition, Action: in.Action}
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

// NextRunsInput is automations.nextRuns input.
type NextRunsInput struct {
	AutomationID uuid.UUID `json:"automationId"`
	Count        int       `json:"count"`
}

// Validate implements ops.Validator (count is fixed to 8).
func (in *NextRunsInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.AutomationID == uuid.Nil {
		fe["automationId"] = "error.required"
	}
	if in.Count != 8 {
		fe["count"] = "error.invalid"
	}
	return fe
}

// nextRuns previews the next 8 start/end occurrences after now of a schedule rule (D09); invalid local times are
// skipped. Event rules have no schedule (VALIDATION).
func (m Automations) nextRuns(ctx context.Context, c *ops.Call, in *NextRunsInput) ([]ScheduledOccurrence, error) {
	x, err := m.load(ctx, c, in.AutomationID, false)
	if err != nil {
		return nil, err
	}
	if x.Kind != "schedule" {
		return nil, apperr.Fields(map[string]string{"automationId": "errors.not_schedule"})
	}
	loc, _ := time.LoadLocation(x.Timezone)
	def := definition{Weekdays: x.Weekdays, StartLocal: x.StartLocal, EndLocal: x.EndLocal, EndsNextDay: x.EndsNextDay, StartAction: x.StartAction, EndAction: x.EndAction}
	occ, _ := occurrences(def, loc, c.Now.In(loc).AddDate(0, 0, -1), 30)
	out := []ScheduledOccurrence{}
	for _, o := range occ {
		if o.At.After(c.Now) && len(out) < in.Count {
			o.AutomationID = x.ID
			out = append(out, o)
		}
	}
	return out, nil
}

// RegisterAutomations binds automations.save / list / nextRuns.
func RegisterAutomations(r *ops.Registry, m Automations) {
	ops.Register(r, "automations.save", m.save)
	ops.Register(r, "automations.list", m.list)
	ops.Register(r, "automations.nextRuns", m.nextRuns)
	RegisterEvaluation(r, m)
}
