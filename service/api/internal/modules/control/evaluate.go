package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Fact is Fact of service-contracts.ts.
type Fact struct {
	UnitID     uuid.UUID       `json:"unitId"`
	Metric     string          `json:"metric"`
	Value      json.RawMessage `json:"value"`
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
	"temperature": {"°C", "number", 120 * time.Second}, "humidity": {"%", "number", 120 * time.Second}, "co2": {"ppm", "number", 120 * time.Second},
	"pm25": {"µg/m³", "number", 120 * time.Second}, "power": {"kW", "number", 120 * time.Second}, "vibration": {"mm/s", "number", 120 * time.Second},
	"refrigerant_pressure": {"kPa", "number", 120 * time.Second},
	"weather_temperature":  {"°C", "number", 1800 * time.Second}, "tariff": {"MYR_per_kWh", "number", 300 * time.Second},
	"solar": {"kW", "number", 120 * time.Second}, "battery": {"kW", "number", 120 * time.Second},
	"occupied": {"boolean", "boolean", 120 * time.Second}, "peak": {"boolean", "boolean", 120 * time.Second}, "location": {"event", "location", 0},
}

// Validate implements ops.Validator (D02 / IR16 / IR21).
func (in *EvaluationInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.EventID == uuid.Nil {
		fe["eventId"] = "error.required"
	}
	if in.OccurredAt.IsZero() {
		fe["occurredAt"] = "error.required"
	}
	if in.Phase != "schedule_start" && in.Phase != "schedule_end" && in.Phase != "condition" {
		fe["phase"] = "error.invalid"
	}
	units := map[uuid.UUID]bool{}
	for _, u := range in.UnitIDs {
		if u == uuid.Nil || units[u] {
			fe["unitIds"] = "error.invalid"
		}
		units[u] = true
	}
	if len(in.UnitIDs) == 0 || len(in.UnitIDs) > 100 {
		fe["unitIds"] = "error.invalid"
	}
	seen := map[string]bool{}
	for _, f := range in.Facts {
		spec, ok := factSpec[f.Metric]
		key := f.UnitID.String() + "/" + f.Metric
		if !ok || f.Unit != spec.unit || !units[f.UnitID] || seen[key] || (f.Quality != "valid" && f.Quality != "missing" && f.Quality != "stale" && f.Quality != "suspect") {
			fe["facts"] = "error.invalid"
			continue
		}
		seen[key] = true
		if string(f.Value) == "null" {
			continue
		}
		switch spec.kind {
		case "number":
			var v float64
			if json.Unmarshal(f.Value, &v) != nil || math.IsNaN(v) || math.IsInf(v, 0) {
				fe["facts"] = "error.invalid"
			}
		case "boolean":
			var v bool
			if json.Unmarshal(f.Value, &v) != nil {
				fe["facts"] = "error.invalid"
			}
		default:
			var v string
			if json.Unmarshal(f.Value, &v) != nil || (v != "arrival" && v != "departure") {
				fe["facts"] = "error.invalid"
			}
		}
	}
	return fe
}

// Decision is Decision / the FireResult row.
type Decision struct {
	UnitID    uuid.UUID  `json:"unitId"`
	Decision  string     `json:"decision"`
	RuleID    *uuid.UUID `json:"ruleId"`
	Reason    *string    `json:"reason"`
	CommandID *uuid.UUID `json:"commandId,omitempty"`
}

// Result is SimulationResult / FireResult.
type Result struct {
	EventID       uuid.UUID         `json:"eventId"`
	Results       []Decision        `json:"results"`
	Notifications []json.RawMessage `json:"notifications"`
}

type candidate struct {
	hq       bool
	id       uuid.UUID
	priority int
	enabled  bool
	owner    uuid.UUID
	action   json.RawMessage
	cond     json.RawMessage // nil for schedule actions
	tz       string
	kind     string // automation | automation_policy
}

// factState evaluates the fact of a unit for a metric: value, or the D02 reason (missing_data / stale).
func factState(facts map[string]Fact, unit uuid.UUID, metric string, now time.Time) (json.RawMessage, string) {
	f, ok := facts[unit.String()+"/"+metric]
	if !ok || string(f.Value) == "null" || f.ObservedAt.After(now) || (f.Quality != "valid" && f.Quality != "stale") {
		return nil, "missing_data"
	}
	spec := factSpec[metric]
	if f.Quality == "stale" || (metric != "location" && now.Sub(f.ObservedAt) > spec.ttl) || (metric == "location" && !f.ObservedAt.Truncate(time.Minute).Equal(now.Truncate(time.Minute))) {
		return nil, "stale"
	}
	return f.Value, ""
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
func match(raw json.RawMessage, unit uuid.UUID, facts map[string]Fact, at time.Time, tz string) string {
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
		v, why := factState(facts, unit, metric, at)
		if why != "" {
			return why
		}
		var b bool
		_ = json.Unmarshal(v, &b)
		if b == want {
			return ""
		}
		return "no_match"
	}
	numFact := func(metric string) string {
		v, why := factState(facts, unit, metric, at)
		if why != "" {
			return why
		}
		var x float64
		_ = json.Unmarshal(v, &x)
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
		v, why := factState(facts, unit, "location", at)
		if why != "" {
			return why
		}
		var e string
		_ = json.Unmarshal(v, &e)
		if e == *c.Event {
			return ""
		}
		return "no_match"
	case "pattern": // IR52: local time of occurredAt equals localTime
		loc, err := time.LoadLocation(tz)
		if err != nil || at.In(loc).Format("15:04") != *c.LocalTime {
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
func ownerValid(ctx context.Context, c *ops.Call, owner uuid.UUID, hq bool, org uuid.UUID) (bool, error) {
	perm := "control.execute"
	if hq {
		perm = "automation.policy.write"
	}
	var ok bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.memberships m JOIN identity.membership_permissions p ON p.membership_id = m.id AND p.permission = $2
		WHERE m.id = $1 AND m.valid_from <= $3 AND (m.valid_until IS NULL OR m.valid_until > $3) AND ($4 OR m.organization_id = $5))`, owner, perm, c.Now, hq, org).Scan(&ok)
	return ok, err
}

func (m Automations) candidates(ctx context.Context, c *ops.Call, unit uuid.UUID, in *EvaluationInput) ([]candidate, error) {
	var out []candidate
	rows, err := c.Tx.Query(ctx, `SELECT p.id, p.priority, p.enabled, p.owner_membership_id, p.action, p.condition, p.timezone FROM control.automation_policies p
		JOIN control.automation_policy_units u ON u.policy_id = p.id WHERE u.unit_id = $1`, unit)
	if err != nil {
		return nil, err
	}
	for rows.Next() {
		x := candidate{hq: true, kind: "automation_policy"}
		if err := rows.Scan(&x.id, &x.priority, &x.enabled, &x.owner, &x.action, &x.cond, &x.tz); err != nil {
			rows.Close()
			return nil, err
		}
		if in.Phase == "condition" {
			out = append(out, x)
		}
	}
	rows.Close()
	rows, err = c.Tx.Query(ctx, `SELECT a.id, a.priority, a.enabled, a.owner_membership_id, a.kind, a.definition, a.timezone FROM control.automations a
		JOIN control.automation_units u ON u.automation_id = a.id WHERE u.unit_id = $1`, unit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		x := candidate{kind: "automation"}
		var kind string
		var raw []byte
		if err := rows.Scan(&x.id, &x.priority, &x.enabled, &x.owner, &kind, &raw, &x.tz); err != nil {
			return nil, err
		}
		var d definition
		_ = json.Unmarshal(raw, &d)
		switch {
		case kind == "event" && in.Phase == "condition":
			x.cond, x.action = d.Condition, d.Action
		case kind == "schedule" && in.Phase != "condition": // due when an occurrence of this phase is at the tick
			loc, err := time.LoadLocation(x.tz)
			if err != nil {
				continue
			}
			occ, _ := occurrences(d, loc, in.OccurredAt.In(loc).AddDate(0, 0, -1), 3)
			due := false
			for _, o := range occ {
				if o.Phase == in.Phase && o.At.Equal(in.OccurredAt.Truncate(time.Minute)) {
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
	return out, rows.Err()
}

// decide arbitrates one unit (D02): candidates in HQ-policy then customer order, priority desc, ID asc; the first
// eligible wins. Otherwise the first exclusion reason in candidate-ID order, or no_match without candidates.
func (m Automations) decide(ctx context.Context, c *ops.Call, unit uuid.UUID, in *EvaluationInput, facts map[string]Fact) (Decision, *candidate, error) {
	cands, err := m.candidates(ctx, c, unit, in)
	if err != nil {
		return Decision{}, nil, err
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
	t, found, err := m.Units.Target(ctx, c, unit)
	if err != nil {
		return Decision{}, nil, err
	}
	reasons := map[uuid.UUID]string{}
	for i := range cands {
		x := &cands[i]
		reason := ""
		var cond struct {
			Type string `json:"type"`
		}
		_ = json.Unmarshal(x.cond, &cond)
		switch {
		case !x.enabled:
			reason = "disabled"
		case x.cond != nil && match(x.cond, unit, facts, in.OccurredAt, x.tz) != "":
			reason = match(x.cond, unit, facts, in.OccurredAt, x.tz)
		}
		if reason == "" {
			ok, err := ownerValid(ctx, c, x.owner, x.hq, t.OrgID)
			if err != nil {
				return Decision{}, nil, err
			}
			if !ok {
				reason = "owner_forbidden"
			}
		}
		if reason == "" && cond.Type == "location" {
			var granted bool
			if err := c.Tx.QueryRow(ctx, `SELECT COALESCE((SELECT granted FROM identity.consents WHERE membership_id = $1 AND purpose = 'location_automation'), false)`, x.owner).Scan(&granted); err != nil {
				return Decision{}, nil, err
			}
			if !granted {
				reason = "consent_revoked"
			}
		}
		if reason == "" {
			a, ok := ParseAction(x.action)
			switch {
			case !ok || !found || !a.Supports(t.Caps):
				reason = "invalid_capability"
			default:
				policy, err := m.Cmd.Restrictions.UnitPolicy(ctx, c, unit)
				if err != nil {
					return Decision{}, nil, err
				}
				if !Allowed(a, policy) {
					reason = "restricted"
				} else if _, err := m.Cmd.ready(ctx, c, unit); err != nil {
					var de *apperr.DomainError
					if errors.As(err, &de) && de.MessageKey == "errors.reconciliation_required" {
						reason = "reconciliation_required"
					} else if errors.As(err, &de) && de.Code == apperr.Conflict {
						reason = "busy"
					} else if errors.As(err, &de) && de.Code == apperr.Offline {
						reason = "offline"
					} else {
						return Decision{}, nil, err
					}
				}
			}
		}
		if reason == "" {
			id := x.id
			return Decision{UnitID: unit, Decision: "selected", RuleID: &id}, x, nil
		}
		reasons[x.id] = reason
	}
	if len(cands) == 0 {
		r := "no_match"
		return Decision{UnitID: unit, Decision: "suppressed", Reason: &r}, nil, nil
	}
	ids := make([]uuid.UUID, 0, len(reasons))
	for id := range reasons {
		ids = append(ids, id)
	}
	sort.Slice(ids, func(i, j int) bool { return ids[i].String() < ids[j].String() })
	r := reasons[ids[0]]
	return Decision{UnitID: unit, Decision: "suppressed", Reason: &r}, nil, nil
}

// prepare authorizes the caller for every target unit (D02: out of scope NOT_FOUND) and merges the facts.
func (m Automations) prepare(ctx context.Context, c *ops.Call, in *EvaluationInput) (map[string]Fact, error) {
	for _, u := range in.UnitIDs {
		t, found, err := m.Units.Target(ctx, c, u)
		if err != nil {
			return nil, err
		}
		if !found || (c.Principal.Role == "client" && t.OrgID != c.Principal.OrgID) {
			return nil, apperr.E(apperr.NotFound, "error.notFound")
		}
	}
	facts := map[string]Fact{}
	for _, f := range in.Facts {
		facts[f.UnitID.String()+"/"+f.Metric] = f
	}
	return facts, nil
}

func (m Automations) evaluate(ctx context.Context, c *ops.Call, in *EvaluationInput, fire bool) (Result, error) {
	facts, err := m.prepare(ctx, c, in)
	if err != nil {
		return Result{}, err
	}
	units := append([]uuid.UUID{}, in.UnitIDs...)
	sort.Slice(units, func(i, j int) bool { return units[i].String() < units[j].String() })
	out := Result{EventID: in.EventID, Results: []Decision{}, Notifications: []json.RawMessage{}}
	for _, u := range units {
		d, win, err := m.decide(ctx, c, u, in, facts)
		if err != nil {
			return Result{}, err
		}
		if fire {
			ref := in.EventID.String() + "/" + in.Phase
			if win != nil {
				id, err := m.automationCommand(ctx, c, u, win, in)
				if err != nil {
					return Result{}, err
				}
				d.Decision, d.CommandID = "requested", &id
				if _, err := c.Tx.Exec(ctx, `INSERT INTO control.automation_runs (tenant_id, rule_kind, rule_id, unit_id, trigger_ref, outcome, command_id, occurred_at)
					VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, 'command_created', $5, $6) ON CONFLICT DO NOTHING`, win.kind, win.id, u, ref, id, in.OccurredAt); err != nil {
					return Result{}, err
				}
			}
		}
		if m.Notifier != nil { // notification results are independent of the control decision (SR21/SR25)
			factsJSON, _ := json.Marshal(in.Facts)
			ns, err := m.Notifier.Evaluate(ctx, c, u, factsJSON, in.OccurredAt, in.EventID, fire)
			if err != nil {
				return Result{}, err
			}
			matched := false
			for _, n := range ns {
				var x struct {
					Decision string `json:"decision"`
				}
				_ = json.Unmarshal(n, &x)
				matched = matched || x.Decision != "suppressed"
			}
			if matched && d.Decision == "suppressed" && d.Reason != nil && *d.Reason == "no_match" {
				r := "no_control_action"
				d.Reason = &r
			}
			out.Notifications = append(out.Notifications, ns...)
		}
		out.Results = append(out.Results, d)
	}
	return out, nil
}

// automationCommand creates the winner's Command (source automation, actor = rule owner).
func (m Automations) automationCommand(ctx context.Context, c *ops.Call, unit uuid.UUID, win *candidate, in *EvaluationInput) (uuid.UUID, error) {
	dev, err := m.Cmd.ready(ctx, c, unit)
	if err != nil {
		return uuid.Nil, err
	}
	var id uuid.UUID
	err = c.Tx.QueryRow(ctx, `INSERT INTO control.commands (tenant_id, unit_id, device_id, actor_membership_id, source, action, status, delivery, requested_at, sent_at, expires_at,
		correlation_id, created_at, updated_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, 'automation', $4, 'sent', 'sent', $5, $5, $6, $7, $5, $5) RETURNING id`,
		unit, dev, win.owner, []byte(win.action), c.Now, c.Now.Add(CommandTTL), c.CorrelationID).Scan(&id)
	if err == nil {
		c.Emit(ops.Event{AggregateType: "command", AggregateID: id, Type: "CommandRequested", Payload: map[string]any{"unitId": unit, "source": "automation", "ruleId": win.id}})
	}
	return id, err
}

func checkTick(c *ops.Call, in *EvaluationInput) error {
	if !in.OccurredAt.Truncate(time.Minute).Equal(c.Now.Truncate(time.Minute)) { // IR21: the current demo clock tick
		return apperr.Fields(map[string]string{"occurredAt": "errors.not_current_tick"})
	}
	return nil
}

func (m Automations) simulate(ctx context.Context, c *ops.Call, in *EvaluationInput) (Result, error) {
	if err := checkTick(c, in); err != nil {
		return Result{}, err
	}
	return m.evaluate(ctx, c, in, false)
}

// fire evaluates and creates Commands (D02): a repeated tenant/eventId/phase returns the stored result (checked
// before the tick, IR21), a reused eventId with other input or another input for the same tick is CONFLICT.
func (m Automations) fire(ctx context.Context, c *ops.Call, in *EvaluationInput) (Result, error) {
	norm, _ := json.Marshal(struct {
		O time.Time
		U []uuid.UUID
		F []Fact
	}{in.OccurredAt.UTC(), in.UnitIDs, in.Facts})
	sum := sha256.Sum256(norm)
	hash := hex.EncodeToString(sum[:])
	var stored []byte
	var storedHash string
	err := c.Tx.QueryRow(ctx, `SELECT input_hash, result FROM control.evaluation_events WHERE event_id = $1 AND phase = $2`, in.EventID, in.Phase).Scan(&storedHash, &stored)
	if err == nil {
		if storedHash != hash {
			return Result{}, apperr.E(apperr.Conflict, "errors.event_reused")
		}
		var r Result
		_ = json.Unmarshal(stored, &r)
		return r, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, err
	}
	if err := checkTick(c, in); err != nil {
		return Result{}, err
	}
	// one tick has one result (D02): the same input with another eventId reuses it; other input is CONFLICT
	err = c.Tx.QueryRow(ctx, `SELECT input_hash, result FROM control.evaluation_events WHERE date_trunc('minute', occurred_at) = date_trunc('minute', $1::timestamptz)
		ORDER BY created_at LIMIT 1`, in.OccurredAt).Scan(&storedHash, &stored)
	if err == nil {
		var prev struct {
			Phase string
		}
		_ = c.Tx.QueryRow(ctx, `SELECT phase FROM control.evaluation_events WHERE date_trunc('minute', occurred_at) = date_trunc('minute', $1::timestamptz) ORDER BY created_at LIMIT 1`, in.OccurredAt).Scan(&prev.Phase)
		if storedHash != hash || prev.Phase != in.Phase {
			return Result{}, apperr.E(apperr.Conflict, "errors.tick_already_evaluated")
		}
		var r Result
		_ = json.Unmarshal(stored, &r)
		r.EventID = in.EventID
		return r, nil
	}
	if !errors.Is(err, pgx.ErrNoRows) {
		return Result{}, err
	}
	r, err := m.evaluate(ctx, c, in, true)
	if err != nil {
		return r, err
	}
	raw, _ := json.Marshal(r)
	if _, err := c.Tx.Exec(ctx, `INSERT INTO control.evaluation_events (tenant_id, event_id, phase, occurred_at, input_hash, result) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5)`,
		in.EventID, in.Phase, in.OccurredAt, hash, raw); err != nil {
		return r, err
	}
	c.Audit(ops.AuditEntry{Action: "automations.fire", TargetKind: "evaluation", TargetID: in.EventID.String(), Reason: strings.ToLower(in.Phase)})
	return r, nil
}

// RegisterEvaluation binds automations.simulate / fire.
func RegisterEvaluation(r *ops.Registry, m Automations) {
	ops.Register(r, "automations.simulate", m.simulate)
	ops.Register(r, "automations.fire", m.fire)
}
