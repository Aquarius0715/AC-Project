package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Fact is Fact of service-contracts.ts.
type Fact struct {
	UnitID     uuid.UUID       `json:"unitId"`
	Metric     string          `json:"metric"`
	Value      json.RawMessage `json:"value"      swaggertype:"object"`
	Unit       string          `json:"unit"`
	ObservedAt time.Time       `json:"observedAt"`
	Quality    string          `json:"quality"`
}

// EvaluationInput is EvaluationInput of service-contracts.ts.
type EvaluationInput struct {
	EventID    uuid.UUID   `json:"eventId"`
	OccurredAt time.Time   `json:"occurredAt"`
	UnitIDs    []uuid.UUID `json:"unitIds"`
	Facts      []Fact      `json:"facts"`
	Phase      string      `json:"phase"`
}

// factSpec is the IR16 metric table: the unit, value type and TTL (normal metrics use 120 s, D07).
var factSpec = map[string]struct {
	unit string
	kind string // number | boolean | location
	ttl  time.Duration
}{
	"temperature": {
		"°C",
		"number",
		120 * time.Second,
	},
	"humidity": {"%", "number", 120 * time.Second},
	"co2":      {"ppm", "number", 120 * time.Second},
	"pm25": {
		"µg/m³",
		"number",
		120 * time.Second,
	},
	"power":                {"kW", "number", 120 * time.Second},
	"vibration":            {"mm/s", "number", 120 * time.Second},
	"refrigerant_pressure": {"kPa", "number", 120 * time.Second},
	// the contract's other Metric values (D07 units), which default rules watch (IR120, IR529)
	"compressor_cycles": {
		"cycles/h",
		"number",
		120 * time.Second,
	},
	"airflow_drop":  {"%", "number", 120 * time.Second},
	"heartbeat_gap": {"min", "number", 120 * time.Second},
	"weather_temperature": {
		"°C",
		"number",
		1800 * time.Second,
	},
	"tariff":  {"MYR_per_kWh", "number", 300 * time.Second},
	"solar":   {"kW", "number", 120 * time.Second},
	"battery": {"kW", "number", 120 * time.Second},
	"occupied": {
		"boolean",
		"boolean",
		120 * time.Second,
	},
	"peak":     {"boolean", "boolean", 120 * time.Second},
	"location": {"event", "location", 0},
}

// Validate implements ops.Validator (D02 / IR16 / IR21).
func (in *EvaluationInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.EventID == uuid.Nil {
		fieldErrs["eventId"] = "error.required"
	}

	if in.OccurredAt.IsZero() {
		fieldErrs["occurredAt"] = "error.required"
	}

	if in.Phase != "schedule_start" && in.Phase != "schedule_end" && in.Phase != "condition" {
		fieldErrs["phase"] = "error.invalid"
	}

	units := map[uuid.UUID]bool{}
	for _, u := range in.UnitIDs {
		if u == uuid.Nil || units[u] {
			fieldErrs["unitIds"] = "error.invalid"
		}

		units[u] = true
	}

	if len(in.UnitIDs) == 0 || len(in.UnitIDs) > 100 {
		fieldErrs["unitIds"] = "error.invalid"
	}

	seen := map[string]bool{}

	for _, fact := range in.Facts {
		spec, ok := factSpec[fact.Metric]

		key := fact.UnitID.String() + "/" + fact.Metric
		if !ok || fact.Unit != spec.unit || !units[fact.UnitID] || seen[key] ||
			(fact.Quality != "valid" && fact.Quality != "missing" && fact.Quality != "stale" && fact.Quality != "suspect") {
			fieldErrs["facts"] = "error.invalid"

			continue
		}

		seen[key] = true

		if string(fact.Value) == "null" {
			continue
		}

		switch spec.kind {
		case "number":
			var v float64
			if json.Unmarshal(fact.Value, &v) != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				fieldErrs["facts"] = "error.invalid"
			}
		case "boolean":
			var v bool
			if json.Unmarshal(fact.Value, &v) != nil {
				fieldErrs["facts"] = "error.invalid"
			}
		default:
			var v string
			if json.Unmarshal(fact.Value, &v) != nil || (v != "arrival" && v != "departure") {
				fieldErrs["facts"] = "error.invalid"
			}
		}
	}

	return fieldErrs
}

// Decision is Decision / the FireResult row.
type Decision struct {
	UnitID    uuid.UUID  `json:"unitId"`
	Decision  string     `json:"decision"`
	RuleID    *uuid.UUID `json:"ruleId"`
	Reason    *string    `json:"reason"`
	CommandID *uuid.UUID `json:"commandId,omitempty"`
	fresh     bool       // the Command was created by this evaluation (not an earlier one of the same trigger)
}

// Result is SimulationResult / FireResult.
type Result struct {
	EventID       uuid.UUID         `json:"eventId"`
	Results       []Decision        `json:"results"`
	Notifications []json.RawMessage `json:"notifications" swaggertype:"object"`
}

type candidate struct {
	hq       bool
	id       uuid.UUID
	priority int
	enabled  bool
	owner    uuid.UUID
	action   json.RawMessage
	cond     json.RawMessage // nil for schedule actions
	extras   json.RawMessage // onlyIf (IR215), customer rules only
	tz       string
	kind     string // automation | automation_policy
}

// conditionMetric is the fact a condition reads ("" for a local-time pattern).
func conditionMetric(raw json.RawMessage) string {
	var c struct {
		Type string `json:"type"`
	}

	_ = json.Unmarshal(raw, &c)
	switch c.Type {
	case "occupancy":
		return "occupied"
	case "weather":
		return "weather_temperature"
	case "location", "peak", "tariff", "solar", "battery":
		return c.Type
	}

	return ""
}

// extrasMatch checks the rule's onlyIf conditions (IR215) in order: "" when all hold, else the first reason —
// no_match, or missing_data / stale for occupancy and weather (missing data never counts as a match).
func extrasMatch(
	raw json.RawMessage,
	unit uuid.UUID,
	facts map[string]Fact,
	when time.Time,
	tz string,
) string {
	var xs []json.RawMessage
	if json.Unmarshal(raw, &xs) != nil {
		return ""
	}

	for _, rawMessage := range xs {
		var w struct {
			Type     string `json:"type"`
			Weekdays []int  `json:"weekdays"`
		}

		_ = json.Unmarshal(rawMessage, &w)
		if w.Type == "weekday" {
			loc, err := time.LoadLocation(tz)
			if err != nil {
				return "no_match"
			}

			wd := int(when.In(loc).Weekday())
			if wd == 0 {
				wd = 7
			}

			if !slices.Contains(w.Weekdays, wd) {
				return "no_match"
			}

			continue
		}

		if why := match(rawMessage, unit, facts, when, tz); why != "" {
			return why
		}
	}

	return ""
}

// skip is a candidate that could not run, with its D02 reason.
type skip struct {
	cand     *candidate
	reason   string
	recorded bool // worth a run-log row: its own trigger held (or its own data came in incomplete), IR215
}

// factState evaluates the fact of a unit for a metric: value, or the D02 reason (missing_data / stale).
func factState(
	facts map[string]Fact,
	unit uuid.UUID,
	metric string,
	now time.Time,
) (json.RawMessage, string) {
	fact, ok := facts[unit.String()+"/"+metric]
	if !ok || string(fact.Value) == "null" || fact.ObservedAt.After(now) ||
		(fact.Quality != "valid" && fact.Quality != "stale") {
		return nil, "missing_data"
	}

	spec := factSpec[metric]
	if fact.Quality == "stale" || (metric != "location" && now.Sub(fact.ObservedAt) > spec.ttl) ||
		(metric == "location" && !fact.ObservedAt.Truncate(time.Minute).Equal(now.Truncate(time.Minute))) {
		return nil, "stale"
	}

	return fact.Value, ""
}

func compare(op string, a, b float64) bool {
	switch op {
	case "gt":
		return a > b
	case "gte":
		return a >= b
	case "lt":
		return a < b
	}

	return a <= b
}

// match evaluates a Condition for a unit: "" when it matches, else no_match / missing_data / stale.
func match(
	raw json.RawMessage,
	unit uuid.UUID,
	facts map[string]Fact,
	when time.Time,
	tz string,
) string {
	var c struct {
		Type      string   `json:"type"`
		Occupied  *bool    `json:"occupied"`
		Active    *bool    `json:"active"`
		Event     *string  `json:"event"`
		LocalTime *string  `json:"localTime"`
		Operator  *string  `json:"operator"`
		Value     *float64 `json:"value"`
	}

	_ = json.Unmarshal(raw, &c)
	boolFact := func(metric string, want bool) string {
		rawMessage, why := factState(facts, unit, metric, when)
		if why != "" {
			return why
		}

		var b bool

		_ = json.Unmarshal(rawMessage, &b)
		if b == want {
			return ""
		}

		return "no_match"
	}
	numFact := func(metric string) string {
		rawMessage, why := factState(facts, unit, metric, when)
		if why != "" {
			return why
		}

		var x float64

		_ = json.Unmarshal(rawMessage, &x)
		if compare(*c.Operator, x, *c.Value) {
			return ""
		}

		return "no_match"
	}

	switch c.Type {
	case "occupancy":
		return boolFact("occupied", *c.Occupied)
	case "peak":
		return boolFact("peak", *c.Active)
	case "location":
		rawMessage, why := factState(facts, unit, "location", when)
		if why != "" {
			return why
		}

		var e string

		_ = json.Unmarshal(rawMessage, &e)
		if e == *c.Event {
			return ""
		}

		return "no_match"
	case "pattern": // IR52: local time of occurredAt equals localTime
		loc, err := time.LoadLocation(tz)
		if err != nil || when.In(loc).Format("15:04") != *c.LocalTime {
			return "no_match"
		}

		return ""
	case "weather":
		return numFact("weather_temperature")
	case "tariff":
		return numFact("tariff")
	case "solar", "battery":
		return numFact(c.Type)
	}

	return "no_match"
}

// ownerValid re-authorizes a rule owner at evaluation time (D02): an active membership with control.execute (client
// rules, same organization) or automation.policy.write (HQ policies).
func ownerValid(
	ctx context.Context,
	call *ops.Call,
	owner uuid.UUID,
	hq bool,
	org uuid.UUID,
) (bool, error) {
	perm := "control.execute"
	if hq {
		perm = "automation.policy.write"
	}

	membersInput := identity.MembersInput{
		IDs:        []uuid.UUID{owner},
		Permission: perm, Role: "", OrganizationID: nil, ClientRole: "",
	} // identity's memberships (IR194)
	if !hq {
		membersInput.OrganizationID = &org
	}

	ids, err := identity.Members(ctx, call, membersInput)

	return len(ids) == 1, apperr.Wrap(err, "identity.Members")
}

func (m Automations) candidates(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	input *EvaluationInput,
) ([]candidate, error) {
	var out []candidate

	rows, err := call.Tx.Query(
		ctx,
		`SELECT p.id, p.priority, p.enabled, p.owner_membership_id, p.action, p.condition, p.timezone FROM
			control.automation_policies p
		JOIN control.automation_policy_units u ON u.policy_id = p.id WHERE u.unit_id = $1`,
		unit,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select control.automation_policies")
	}

	for rows.Next() {
		x := candidate{
			hq:       true,
			kind:     "automation_policy",
			id:       uuid.UUID{},
			priority: 0,
			enabled:  false,
			owner:    uuid.UUID{},
			action:   nil,
			cond:     nil,
			extras:   nil,
			tz:       "",
		}

		err := rows.Scan(
			&x.id,
			&x.priority,
			&x.enabled,
			&x.owner,
			&x.action,
			&x.cond,
			&x.tz,
		)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select control.automation_policies")
		}

		if input.Phase == "condition" {
			out = append(out, x)
		}
	}

	rows.Close()

	rows, err = call.Tx.Query(
		ctx,
		`SELECT a.id, a.priority, a.enabled, a.owner_membership_id, a.kind, a.definition, a.timezone FROM control.automations
			a
		JOIN control.automation_units u ON u.automation_id = a.id WHERE u.unit_id = $1`,
		unit,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select control.automations")
	}
	defer rows.Close()

	for rows.Next() {
		x := candidate{
			kind:     "automation",
			hq:       false,
			id:       uuid.UUID{},
			priority: 0,
			enabled:  false,
			owner:    uuid.UUID{},
			action:   nil,
			cond:     nil,
			extras:   nil,
			tz:       "",
		}

		var (
			kind string
			raw  []byte
		)

		err := rows.Scan(
			&x.id,
			&x.priority,
			&x.enabled,
			&x.owner,
			&kind,
			&raw,
			&x.tz,
		)
		if err != nil {
			return nil, apperr.Wrap(err, "select control.automations")
		}

		var d definition

		_ = json.Unmarshal(raw, &d)
		x.extras = d.OnlyIf

		switch {
		case kind == "event" && input.Phase == "condition":
			x.cond, x.action = d.Condition, d.Action
		case kind == "schedule" && input.Phase != "condition": // due when an occurrence of this phase is at the tick
			loc, err := time.LoadLocation(x.tz)
			if err != nil {
				continue
			}

			occ, _ := occurrences(d, loc, input.OccurredAt.In(loc).AddDate(0, 0, -1), 3)
			due := false

			for _, o := range occ {
				if o.Phase == input.Phase && o.At.Equal(input.OccurredAt.Truncate(time.Minute)) {
					due, x.action = true, o.Action
				}
			}

			if !due {
				continue
			}
		default:
			continue
		}

		out = append(out, x)
	}

	return out, apperr.Wrap(rows.Err(), "select control.automations")
}

// decide arbitrates one unit (D02): candidates in HQ-policy then customer order, priority desc, ID asc; the first
// eligible wins. Otherwise the first exclusion reason in candidate-ID order, or no_match without candidates. The
// candidates that could not run are returned with their reasons (the run log of IR215).
func (m Automations) decide(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	input *EvaluationInput,
	facts map[string]Fact,
) (Decision, *candidate, []skip, error) {
	cands, err := m.candidates(ctx, call, unit, input)
	if err != nil {
		return Decision{}, nil, nil, err
	}

	sort.SliceStable(cands, func(i, j int) bool {
		if cands[i].hq != cands[j].hq {
			return cands[i].hq
		}

		if cands[i].priority != cands[j].priority {
			return cands[i].priority > cands[j].priority
		}

		return cands[i].id.String() < cands[j].id.String()
	})

	target, found, err := m.Units.Target(ctx, call, unit)
	if err != nil {
		return Decision{}, nil, nil, apperr.Wrap(err, "control.Units.Target")
	}

	if input.Phase == "schedule_end" &&
		slices.ContainsFunc(cands, func(x candidate) bool { return x.enabled }) {
		// a schedule that starts on the unit in the same minute wins over the end (IR326 item 4, IR504): when its
		// start would run, the end is not sent and each enabled end rule records superseded. A start that would not
		// run (owner, restriction, capability, offline …) leaves the end as before.
		start := *input
		start.Phase = "schedule_start"

		sd, _, _, err := m.decide(ctx, call, unit, &start, facts)
		if err != nil {
			return Decision{}, nil, nil, err
		}

		if sd.Decision == "selected" {
			r := "superseded"

			var skips []skip

			for i := range cands {
				if cands[i].enabled {
					skips = append(skips, skip{cand: &cands[i], reason: r, recorded: true})
				}
			}

			return Decision{
				UnitID:    unit,
				Decision:  "suppressed",
				Reason:    &r,
				RuleID:    nil,
				CommandID: nil,
				fresh:     false,
			}, nil, skips, nil
		}
	}

	reasons := map[uuid.UUID]string{}

	var skips []skip

	for i := range cands {
		x := &cands[i]
		reason := ""

		var cond struct {
			Type string `json:"type"`
		}

		_ = json.Unmarshal(x.cond, &cond)
		own := false // the rule's own trigger was decided (a reason after it is worth logging)

		switch {
		case !x.enabled:
			reason = "disabled"
		case x.cond != nil && match(x.cond, unit, facts, input.OccurredAt, x.tz) != "":
			reason = match(x.cond, unit, facts, input.OccurredAt, x.tz)
			_, present := facts[unit.String()+"/"+conditionMetric(x.cond)] // its own data arrived, but missing or stale
			own = present && (reason == "missing_data" || reason == "stale")
		default:
			own = true
			reason = extrasMatch(x.extras, unit, facts, input.OccurredAt, x.tz)
		}

		if reason == "" {
			ok, err := ownerValid(ctx, call, x.owner, x.hq, target.OrgID)
			if err != nil {
				return Decision{}, nil, nil, err
			}

			if !ok {
				reason = "owner_forbidden"
			}
		}

		if reason == "" && cond.Type == "location" {
			granted, err := identity.ConsentGranted(ctx, call, x.owner, "location_automation")
			if err != nil {
				return Decision{}, nil, nil, apperr.Wrap(err, "identity.ConsentGranted")
			}

			if !granted {
				reason = "consent_revoked"
			}
		}

		if reason == "" {
			unitAction, ok := ParseAction(x.action)
			switch {
			case !ok || !found || !unitAction.Supports(target.Caps):
				reason = "invalid_capability"
			default:
				policy, err := m.Cmd.Restrictions.UnitPolicy(ctx, call, unit)
				if err != nil {
					return Decision{}, nil, nil, apperr.Wrap(err, "control.Restrictions.UnitPolicy")
				}

				if !Allowed(unitAction, policy) {
					reason = "restricted"
				} else {
					_, err := m.Cmd.ready(ctx, call, unit)
					if err != nil {
						var domainErr *apperr.DomainError
						if errors.As(err, &domainErr) &&
							domainErr.MessageKey == "errors.reconciliation_required" {
							reason = "reconciliation_required"
						} else if errors.As(err, &domainErr) &&
							domainErr.Code == apperr.Conflict {
							reason = "busy"
						} else if errors.As(err, &domainErr) &&
							domainErr.Code == apperr.Offline {
							reason = "offline"
						} else {
							return Decision{}, nil, nil, err
						}
					}
				}
			}
		}

		if reason == "" {
			id := x.id

			return Decision{
				UnitID:    unit,
				Decision:  "selected",
				RuleID:    &id,
				Reason:    nil,
				CommandID: nil,
				fresh:     false,
			}, x, skips, nil
		}

		reasons[x.id] = reason
		skips = append(
			skips,
			skip{
				cand:   x,
				reason: reason,
				recorded: reason != "disabled" &&
					(own || (reason != "no_match" && reason != "missing_data" && reason != "stale")),
			},
		)
	}

	if len(cands) == 0 {
		r := "no_match"

		return Decision{
			UnitID:    unit,
			Decision:  "suppressed",
			Reason:    &r,
			RuleID:    nil,
			CommandID: nil,
			fresh:     false,
		}, nil, nil, nil
	}

	ids := make([]uuid.UUID, 0, len(reasons))
	for id := range reasons {
		ids = append(ids, id)
	}

	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	r := reasons[ids[0]]

	return Decision{
		UnitID:    unit,
		Decision:  "suppressed",
		Reason:    &r,
		RuleID:    nil,
		CommandID: nil,
		fresh:     false,
	}, nil, skips, nil
}

// prepare authorizes the caller for every target unit (D02: out of scope NOT_FOUND) and merges the facts.
func (m Automations) prepare(
	ctx context.Context,
	c *ops.Call,
	input *EvaluationInput,
) (map[string]Fact, error) {
	err := m.targets(ctx, c, input)
	if err != nil {
		return nil, err
	}

	facts := map[string]Fact{}
	for _, f := range input.Facts {
		facts[f.UnitID.String()+"/"+f.Metric] = f
	}

	return facts, nil
}

// targets checks that the caller may evaluate every target unit: one outside the tenant, or a client's unit of another
// customer, is NOT_FOUND (D02).
func (m Automations) targets(ctx context.Context, call *ops.Call, in *EvaluationInput) error {
	for _, u := range in.UnitIDs {
		t, found, err := m.Units.Target(ctx, call, u)
		if err != nil {
			return apperr.Wrap(err, "control.Units.Target")
		}

		if !found || (call.Principal.Role == "client" && t.OrgID != call.Principal.OrgID) {
			return apperr.E(apperr.NotFound, "error.notFound")
		}
	}

	return nil
}

// evaluate decides every unit; fire creates the winners' Commands (one per rule, unit and trigger: a schedule
// occurrence has the same trigger for the scheduler and a UI fire, IR54) and notify adds the alert-policy decisions.
func (m Automations) evaluate(
	ctx context.Context,
	call *ops.Call,
	input *EvaluationInput,
	fire, notify bool,
) (Result, error) {
	facts, err := m.prepare(ctx, call, input)
	if err != nil {
		return Result{}, err
	}

	units := append([]uuid.UUID{}, input.UnitIDs...)
	sort.Slice(units, func(i, j int) bool { return units[i].String() < units[j].String() })

	out := Result{EventID: input.EventID, Results: []Decision{}, Notifications: []json.RawMessage{}}
	for _, u := range units {
		decision, win, skips, err := m.decide(ctx, call, u, input, facts)
		if err != nil {
			return Result{}, err
		}

		ref := input.EventID.String() + "/" + input.Phase
		if input.Phase != "condition" {
			ref = scheduleRef(input.Phase, input.OccurredAt)
		}

		if fire { // the run log keeps why a rule could not run (IR215: missing data, consent, restriction, offline …)
			for _, sk := range skips {
				if !sk.recorded {
					continue
				}

				_, err := call.Tx.Exec(
					ctx,
					`INSERT INTO control.automation_runs (tenant_id, rule_kind, rule_id, unit_id, trigger_ref, outcome, skip_reason,
						occurred_at)
					VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, 'skipped', $5, $6) ON CONFLICT DO NOTHING`,
					sk.cand.kind,
					sk.cand.id,
					u,
					ref,
					sk.reason,
					input.OccurredAt,
				)
				if err != nil {
					return Result{}, apperr.Wrap(err, "insert control.automation_runs")
				}
			}
		}

		if fire && win != nil {
			var earlier uuid.UUID

			err := call.Tx.QueryRow(ctx, `SELECT command_id FROM control.automation_runs WHERE rule_id = $1 AND unit_id = $2 AND
				trigger_ref = $3 AND command_id IS NOT NULL`,
				win.id, u, ref).
				Scan(&earlier)
			switch {
			case err == nil: // this trigger already ran for the rule and unit
				decision.Decision, decision.CommandID = "requested", &earlier
			case !errors.Is(err, pgx.ErrNoRows):
				return Result{}, apperr.Wrap(err, "select control.automation_runs")
			default:
				id, err := m.automationCommand(ctx, call, u, win, input)
				if err != nil {
					return Result{}, err
				}

				decision.Decision, decision.CommandID, decision.fresh = "requested", &id, true

				_, execErr := call.Tx.Exec(
					ctx,
					`INSERT INTO control.automation_runs (tenant_id, rule_kind, rule_id, unit_id, trigger_ref, outcome, command_id,
						occurred_at)
					VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, 'command_created', $5, $6)
					ON CONFLICT (rule_id, unit_id, trigger_ref) DO UPDATE SET outcome = 'command_created', skip_reason = NULL,
					command_id = EXCLUDED.command_id
					WHERE control.automation_runs.command_id IS NULL`,
					win.kind,
					win.id,
					u,
					ref,
					id,
					input.OccurredAt,
				)
				if execErr != nil {
					return Result{}, apperr.Wrap(execErr, "insert control.automation_runs")
				}
			}
		}

		if notify &&
			m.Notifier != nil { // notification results are independent of the control decision (SR21/SR25)
			factsJSON, _ := json.Marshal(input.Facts)

			ns, err := m.Notifier.Evaluate(
				ctx,
				call,
				u,
				factsJSON,
				input.OccurredAt,
				input.EventID,
				fire,
			)
			if err != nil {
				return Result{}, apperr.Wrap(err, "control.Notifier.Evaluate")
			}

			matched := false

			for _, n := range ns {
				var x struct {
					Decision string `json:"decision"`
				}

				_ = json.Unmarshal(n, &x)
				matched = matched || x.Decision != "suppressed"
			}

			if matched && decision.Decision == "suppressed" && decision.Reason != nil &&
				*decision.Reason == "no_match" {
				r := "no_control_action"
				decision.Reason = &r
			}

			out.Notifications = append(out.Notifications, ns...)
		}

		out.Results = append(out.Results, decision)
	}

	return out, nil
}

// automationCommand creates the winner's Command (source automation, actor = rule owner).
func (m Automations) automationCommand(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	win *candidate,
	in *EvaluationInput,
) (uuid.UUID, error) {
	dev, err := m.Cmd.ready(ctx, call, unit)
	if err != nil {
		return uuid.Nil, err
	}

	var id uuid.UUID

	err = call.Tx.QueryRow(ctx, `INSERT INTO control.commands (tenant_id, unit_id, device_id, actor_membership_id, source,
		action, status, delivery, requested_at, sent_at, expires_at,
		correlation_id, created_at, updated_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, 'automation', $4,
		'requested', 'not_sent', $5, NULL, $6, $7, $5, $5) RETURNING id`,
		unit, dev, win.owner, []byte(win.action), call.Now, call.Now.Add(CommandTTL), call.CorrelationID).
		Scan(&id)
	if err == nil {
		call.Emit(
			ops.Event{
				AggregateType: "command",
				AggregateID:   id,
				Type:          "CommandRequested",
				Payload: map[string]any{
					"unitId": unit,
					"source": "automation",
					"ruleId": win.id,
				},
			},
		)
	}

	return id, apperr.Wrap(err, "insert control.commands")
}

func checkTick(c *ops.Call, in *EvaluationInput) error {
	if !in.OccurredAt.Truncate(time.Minute).
		Equal(c.Now.Truncate(time.Minute)) { // IR21: the current demo clock tick
		return apperr.Fields(map[string]string{"occurredAt": "errors.not_current_tick"})
	}

	return nil
}

// simulate serves automations.simulate.
//
//	@Summary		automations.simulate (read)
//	@ID				automations.simulate
//	@Description	Authorization: client:control.execute:self | admin:automation.policy.read | admin:alert.policy.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A11, DD-A12, DD-C04, DD-C05, DD-A05
//	@Tags			automations
//	@Accept			json
//	@Produce		json
//	@Param			request	body		EvaluationInput	true	"input"
//	@Success		200		{object}	ops.Envelope{data=Result}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/automations/simulate [post]
func (m Automations) simulate(
	ctx context.Context,
	call *ops.Call,
	input *EvaluationInput,
) (Result, error) {
	err := checkTick(call, input)
	if err != nil {
		return Result{}, err
	}

	return m.evaluate(ctx, call, input, false, true)
}

// fire evaluates and creates Commands (D02): a repeated tenant/eventId/phase returns the stored result (checked
// before the tick, IR21), a reused eventId with other input or another input for the same tick is CONFLICT.
//
//	@Summary		automations.fire (write)
//	@ID				automations.fire
//	@Description	Authorization: client:control.execute:self | admin:automation.policy.write | \
//	@Description	admin:alert.policy.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR215 records skips whose own trigger held (reason in automation_runs)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A11, DD-A12, DD-C04, DD-C05, DD-A05
//	@Tags			automations
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		EvaluationInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Result}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/automations/fire [post]
func (m Automations) fire(
	ctx context.Context,
	c *ops.Call,
	input *EvaluationInput,
) (Result, error) {
	// the target units first: a stored result is returned only after authorization (IR21), and a caller outside the
	// units' scope gets NOT_FOUND, not the result of another caller's identical input or a tick CONFLICT (IR366)
	targetsErr := m.targets(ctx, c, input)
	if targetsErr != nil {
		return Result{}, targetsErr
	}

	norm, _ := json.Marshal(struct {
		O time.Time
		U []uuid.UUID
		F []Fact
	}{input.OccurredAt.UTC(), input.UnitIDs, input.Facts})
	sum := sha256.Sum256(norm)
	hash := hex.EncodeToString(sum[:])

	var (
		stored     []byte
		storedHash string
	)

	err := c.Tx.QueryRow(ctx, `SELECT input_hash, result FROM control.evaluation_events WHERE event_id = $1 AND phase =
		$2`, input.EventID, input.Phase).
		Scan(&storedHash, &stored)
	if err == nil {
		if storedHash != hash {
			return Result{}, apperr.E(apperr.Conflict, "errors.event_reused")
		}

		var r Result

		_ = json.Unmarshal(stored, &r)

		return r, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, apperr.Wrap(err, "select control.evaluation_events")
	}

	checkTickErr := checkTick(c, input)
	if checkTickErr != nil {
		return Result{}, checkTickErr
	}
	// one tick has one result (D02): the same input with another eventId reuses it; other input is CONFLICT
	var storedPhase string

	err = c.Tx.QueryRow(ctx, `SELECT input_hash, result, phase FROM control.evaluation_events WHERE date_trunc('minute',
		occurred_at) = date_trunc('minute', $1::timestamptz)
		ORDER BY created_at LIMIT 1`, input.OccurredAt).
		Scan(&storedHash, &stored, &storedPhase)
	if err == nil {
		if storedHash != hash || storedPhase != input.Phase {
			return Result{}, apperr.E(apperr.Conflict, "errors.tick_already_evaluated")
		}

		var r Result

		_ = json.Unmarshal(stored, &r)
		r.EventID = input.EventID

		return r, nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, apperr.Wrap(err, "select control.evaluation_events")
	}

	result, err := m.evaluate(ctx, c, input, true, true)
	if err != nil {
		return result, err
	}

	raw, _ := json.Marshal(result)

	_, execErr := c.Tx.Exec(
		ctx,
		`INSERT INTO control.evaluation_events (tenant_id, event_id, phase, occurred_at, input_hash, result) VALUES
			(current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5)`,
		input.EventID,
		input.Phase,
		input.OccurredAt,
		hash,
		raw,
	)
	if execErr != nil {
		return result, apperr.Wrap(execErr, "insert control.evaluation_events")
	}

	c.Audit(
		ops.AuditEntry{
			Action:     "automations.fire",
			TargetKind: "evaluation",
			TargetID:   input.EventID.String(),
			Reason: strings.ToLower(
				input.Phase,
			),
			PreviousVersion: nil,
			NextVersion:     nil,
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)

	return result, nil
}

// Notify evaluates only the alert policies of the units against facts a device gateway reports (DemoTrigger
// automation, IR529): a matching policy opens or reuses its Alert and notifies, a recovered one resolves it (D08). No
// automation rule is decided, so a sensor report never switches an air conditioner by itself, and no tick is taken
// (the gateway reports every minute beside the UI's fire). It returns the notification outcomes.
func (m Automations) Notify(
	ctx context.Context,
	call *ops.Call,
	input *EvaluationInput,
) ([]json.RawMessage, error) {
	_, err := m.prepare(ctx, call, input)
	if err != nil {
		return nil, err
	}

	out := []json.RawMessage{}
	if m.Notifier == nil {
		return out, nil
	}

	units := append([]uuid.UUID{}, input.UnitIDs...)
	sort.Slice(units, func(i, j int) bool { return units[i].String() < units[j].String() })

	facts, _ := json.Marshal(input.Facts)
	for _, u := range units {
		ns, err := m.Notifier.Evaluate(ctx, call, u, facts, input.OccurredAt, input.EventID, true)
		if err != nil {
			return nil, apperr.Wrap(err, "control.Notifier.Evaluate")
		}

		out = append(out, ns...)
	}

	return out, nil
}

// RegisterEvaluation binds automations.simulate / fire.
func RegisterEvaluation(r *ops.Registry, m Automations) {
	ops.Register(r, "automations.simulate", m.simulate)
	ops.Register(r, "automations.fire", m.fire)
}
