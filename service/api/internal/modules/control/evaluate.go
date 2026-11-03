package control

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"maps"
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
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
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

// metricSpec is a metric's unit, value type (number, boolean or location) and time to live.
type metricSpec struct {
	unit string
	kind string
	ttl  time.Duration
}

// The times to live of the facts (D07): normal metrics, weather, and tariffs.
const (
	normalTTL  = 120 * time.Second
	weatherTTL = 30 * time.Minute
	tariffTTL  = 5 * time.Minute
)

// maxEvaluationUnits is the most units one evaluation covers.
const maxEvaluationUnits = 100

// factSpecs is the IR16 metric table.
func factSpecs() map[string]metricSpec {
	return map[string]metricSpec{
		metricTemperature: {
			"°C",
			valueNumber,
			normalTTL,
		},
		"humidity": {"%", valueNumber, normalTTL},
		"co2":      {"ppm", valueNumber, normalTTL},
		"pm25": {
			"µg/m³",
			valueNumber,
			normalTTL,
		},
		"power":                {"kW", valueNumber, normalTTL},
		"vibration":            {"mm/s", valueNumber, normalTTL},
		"refrigerant_pressure": {"kPa", valueNumber, normalTTL},
		// the contract's other Metric values (D07 units), which default rules watch (IR120, IR529)
		"compressor_cycles": {
			"cycles/h",
			valueNumber,
			normalTTL,
		},
		"airflow_drop":  {"%", valueNumber, normalTTL},
		"heartbeat_gap": {"min", valueNumber, normalTTL},
		"weather_temperature": {
			"°C",
			valueNumber,
			weatherTTL,
		},
		condTariff:  {"MYR_per_kWh", valueNumber, tariffTTL},
		condSolar:   {"kW", valueNumber, normalTTL},
		condBattery: {"kW", valueNumber, normalTTL},
		metricOccupied: {
			valueBoolean,
			valueBoolean,
			normalTTL,
		},
		condPeak:     {valueBoolean, valueBoolean, normalTTL},
		condLocation: {unitEvent, valueLocation, 0},
	}
}

// Validate implements ops.Validator (D02 / IR16 / IR21).
func (in *EvaluationInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.EventID == uuid.Nil {
		fieldErrs["eventId"] = apperr.KeyRequired
	}

	if in.OccurredAt.IsZero() {
		fieldErrs["occurredAt"] = apperr.KeyRequired
	}

	if in.Phase != "schedule_start" && in.Phase != "schedule_end" && in.Phase != "condition" {
		fieldErrs["phase"] = apperr.KeyInvalid
	}

	units := map[uuid.UUID]bool{}
	for _, u := range in.UnitIDs {
		if u == uuid.Nil || units[u] {
			fieldErrs["unitIds"] = apperr.KeyInvalid
		}

		units[u] = true
	}

	if len(in.UnitIDs) == 0 || len(in.UnitIDs) > maxEvaluationUnits {
		fieldErrs["unitIds"] = apperr.KeyInvalid
	}

	seen := map[string]bool{}

	for _, fact := range in.Facts {
		key := fact.UnitID.String() + "/" + fact.Metric
		if !units[fact.UnitID] || seen[key] || !validFact(fact) {
			fieldErrs["facts"] = apperr.KeyInvalid
		}

		seen[key] = true
	}

	return fieldErrs
}

// validFact is whether a fact is one of the IR16 metrics, in its unit and with a known quality, and its value is of
// the metric's type or null.
func validFact(fact Fact) bool {
	spec, ok := factSpecs()[fact.Metric]
	if !ok || fact.Unit != spec.unit ||
		!slices.Contains([]string{"valid", "missing", qualityStale, "suspect"}, fact.Quality) {
		return false
	}

	if string(fact.Value) == jsonNull {
		return true
	}

	switch spec.kind {
	case valueNumber:
		var v float64

		return json.Unmarshal(fact.Value, &v) == nil && !math.IsNaN(v) && !math.IsInf(v, 0)
	case valueBoolean:
		var v bool

		return json.Unmarshal(fact.Value, &v) == nil
	}

	var v string

	return json.Unmarshal(fact.Value, &v) == nil && (v == "arrival" || v == "departure")
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
	var cond struct {
		Type string `json:"type"`
	}

	_ = json.Unmarshal(raw, &cond)
	switch cond.Type {
	case condOccupancy:
		return metricOccupied
	case condWeather:
		return "weather_temperature"
	case condLocation, condPeak, condTariff, condSolar, condBattery:
		return cond.Type
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
	timezone string,
) string {
	var xs []json.RawMessage
	if json.Unmarshal(raw, &xs) != nil {
		return ""
	}

	for _, rawMessage := range xs {
		var extra struct {
			Type     string `json:"type"`
			Weekdays []int  `json:"weekdays"`
		}

		_ = json.Unmarshal(rawMessage, &extra)
		if extra.Type == "weekday" {
			loc, err := time.LoadLocation(timezone)
			if err != nil {
				return reasonNoMatch
			}

			wd := int(when.In(loc).Weekday())
			if wd == 0 {
				wd = 7
			}

			if !slices.Contains(extra.Weekdays, wd) {
				return reasonNoMatch
			}

			continue
		}

		if why := match(rawMessage, unit, facts, when, timezone); why != "" {
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
	if !ok || string(fact.Value) == jsonNull || fact.ObservedAt.After(now) ||
		(fact.Quality != "valid" && fact.Quality != qualityStale) {
		return nil, reasonMissingData
	}

	spec := factSpecs()[metric]
	if fact.Quality == qualityStale ||
		(metric != condLocation && now.Sub(fact.ObservedAt) > spec.ttl) ||
		(metric == condLocation && !fact.ObservedAt.Truncate(time.Minute).Equal(now.Truncate(time.Minute))) {
		return nil, qualityStale
	}

	return fact.Value, ""
}

func compare(op string, value, threshold float64) bool {
	switch op {
	case "gt":
		return value > threshold
	case opGTE:
		return value >= threshold
	case "lt":
		return value < threshold
	}

	return value <= threshold
}

// match evaluates a Condition for a unit: "" when it matches, else no_match / missing_data / stale.
func match(
	raw json.RawMessage,
	unit uuid.UUID,
	facts map[string]Fact,
	when time.Time,
	timezone string,
) string {
	var cond struct {
		Type      string   `json:"type"`
		Occupied  *bool    `json:"occupied"`
		Active    *bool    `json:"active"`
		Event     *string  `json:"event"`
		LocalTime *string  `json:"localTime"`
		Operator  *string  `json:"operator"`
		Value     *float64 `json:"value"`
	}

	_ = json.Unmarshal(raw, &cond)
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

		return reasonNoMatch
	}
	numFact := func(metric string) string {
		rawMessage, why := factState(facts, unit, metric, when)
		if why != "" {
			return why
		}

		var x float64

		_ = json.Unmarshal(rawMessage, &x)
		if compare(*cond.Operator, x, *cond.Value) {
			return ""
		}

		return reasonNoMatch
	}

	switch cond.Type {
	case condOccupancy:
		return boolFact(metricOccupied, *cond.Occupied)
	case condPeak:
		return boolFact(condPeak, *cond.Active)
	case condLocation:
		rawMessage, why := factState(facts, unit, condLocation, when)
		if why != "" {
			return why
		}

		var e string

		_ = json.Unmarshal(rawMessage, &e)
		if e == *cond.Event {
			return ""
		}

		return reasonNoMatch
	case "pattern": // IR52: local time of occurredAt equals localTime
		loc, err := time.LoadLocation(timezone)
		if err != nil || when.In(loc).Format("15:04") != *cond.LocalTime {
			return reasonNoMatch
		}

		return ""
	case condWeather:
		return numFact("weather_temperature")
	case condTariff:
		return numFact(condTariff)
	case condSolar, condBattery:
		return numFact(cond.Type)
	}

	return reasonNoMatch
}

// ownerValid re-authorizes a rule owner at evaluation time (D02): an active membership with control.execute (client
// rules, same organization) or automation.policy.write (HQ policies).
func ownerValid(
	ctx context.Context,
	call *ops.Call,
	owner uuid.UUID,
	hqPolicy bool,
	org uuid.UUID,
) (bool, error) {
	perm := "control.execute"
	if hqPolicy {
		perm = "automation.policy.write"
	}

	membersInput := identity.MembersInput{
		IDs:        []uuid.UUID{owner},
		Permission: perm, Role: "", OrganizationID: nil, ClientRole: "",
	} // identity's memberships (IR194)
	if !hqPolicy {
		membersInput.OrganizationID = &org
	}

	ids, err := identity.Members(ctx, call, membersInput)

	return len(ids) == 1, apperr.Wrap(err, "identity.Members")
}

func checkTick(c *ops.Call, in *EvaluationInput) error {
	if !in.OccurredAt.Truncate(time.Minute).
		Equal(c.Now.Truncate(time.Minute)) { // IR21: the current demo clock tick
		return apperr.Fields(map[string]string{"occurredAt": "errors.not_current_tick"})
	}

	return nil
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

	facts, marshalErr := json.Marshal(input.Facts)
	if marshalErr != nil {
		return nil, apperr.Wrap(marshalErr, "json.Marshal")
	}

	for _, u := range units {
		ns, err := m.Notifier.Evaluate(ctx, call, u, facts, input.OccurredAt, input.EventID, true)
		if err != nil {
			return nil, apperr.Wrap(err, "control.Notifier.Evaluate")
		}

		out = append(out, ns...)
	}

	return out, nil
}

func (m Automations) candidates(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	input *EvaluationInput,
) ([]candidate, error) {
	var out []candidate

	if input.Phase == keyCondition { // HQ policies react to conditions only
		policies, err := policyCandidates(ctx, call, unit)
		if err != nil {
			return nil, err
		}

		out = policies
	}

	rules, err := ruleCandidates(ctx, call, unit, input)
	if err != nil {
		return nil, err
	}

	return append(out, rules...), nil
}

// policyCandidates are the HQ automation policies of the unit.
func policyCandidates(ctx context.Context, call *ops.Call, unit uuid.UUID) ([]candidate, error) {
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

	defer rows.Close()

	var policies []candidate

	for rows.Next() {
		cand := candidate{
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
			&cand.id,
			&cand.priority,
			&cand.enabled,
			&cand.owner,
			&cand.action,
			&cand.cond,
			&cand.tz,
		)
		if err != nil {
			return nil, apperr.Wrap(err, "select control.automation_policies")
		}

		policies = append(policies, cand)
	}

	return policies, apperr.Wrap(rows.Err(), "select control.automation_policies")
}

// ruleCandidates are the customer automations of the unit that apply to this evaluation: an event rule's condition
// and action when a condition is evaluated, or a schedule rule's action when one of its occurrences of this phase is
// at the tick.
func ruleCandidates(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	input *EvaluationInput,
) ([]candidate, error) {
	rows, err := call.Tx.Query(
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

	var out []candidate

	for rows.Next() {
		cand := candidate{
			kind:     kindAutomation,
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
			def  definition
		)

		err := rows.Scan(
			&cand.id,
			&cand.priority,
			&cand.enabled,
			&cand.owner,
			&kind,
			&raw,
			&cand.tz,
		)
		if err != nil {
			return nil, apperr.Wrap(err, "select control.automations")
		}

		err = json.Unmarshal(raw, &def)
		if err != nil {
			return nil, apperr.Wrap(err, "json.Unmarshal")
		}

		cand.extras = def.OnlyIf

		switch {
		case kind == kindEvent && input.Phase == keyCondition:
			cand.cond, cand.action = def.Condition, def.Action
		case kind == kindSchedule && input.Phase != keyCondition:
			action, due := scheduleDue(def, cand.tz, input)
			if !due {
				continue
			}

			cand.action = action
		default:
			continue
		}

		out = append(out, cand)
	}

	return out, apperr.Wrap(rows.Err(), "select control.automations")
}

// scheduleDue is the action of a schedule rule's occurrence of this phase at the tick, and whether it has one; a rule
// in a timezone this server does not know has none.
func scheduleDue(def definition, tz string, input *EvaluationInput) (json.RawMessage, bool) {
	loc, err := time.LoadLocation(tz)
	if err != nil {
		return nil, false
	}

	var (
		action json.RawMessage
		due    bool
	)

	occ, _ := occurrences(def, loc, input.OccurredAt.In(loc).AddDate(0, 0, -1), tickDays)
	for _, o := range occ {
		if o.Phase == input.Phase && o.At.Equal(input.OccurredAt.Truncate(time.Minute)) {
			due, action = true, o.Action
		}
	}

	return action, due
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

	sort.SliceStable(cands, func(left, right int) bool {
		if cands[left].hq != cands[right].hq {
			return cands[left].hq
		}

		if cands[left].priority != cands[right].priority {
			return cands[left].priority > cands[right].priority
		}

		return cands[left].id.String() < cands[right].id.String()
	})

	target, found, err := m.Units.Target(ctx, call, unit)
	if err != nil {
		return Decision{}, nil, nil, apperr.Wrap(err, "control.Units.Target")
	}

	if input.Phase == phaseScheduleEnd &&
		slices.ContainsFunc(cands, func(x candidate) bool { return x.enabled }) {
		superseded, err := m.startWins(ctx, call, unit, input, facts)
		if err != nil {
			return Decision{}, nil, nil, err
		}

		if superseded {
			return suppressedBy(unit, reasonSuperseded), nil, supersededSkips(cands), nil
		}
	}

	reasons := map[uuid.UUID]string{}

	var skips []skip

	for i := range cands {
		cand := &cands[i]

		reason, own := triggerReason(cand, unit, facts, input)
		if reason == "" {
			reason, err = m.eligibility(ctx, call, unit, cand, target, found)
			if err != nil {
				return Decision{}, nil, nil, err
			}
		}

		if reason == "" {
			id := cand.id

			return Decision{
				UnitID:    unit,
				Decision:  decisionSelected,
				RuleID:    &id,
				Reason:    nil,
				CommandID: nil,
				fresh:     false,
			}, cand, skips, nil
		}

		reasons[cand.id] = reason
		skips = append(
			skips,
			skip{
				cand:   cand,
				reason: reason,
				recorded: reason != "disabled" &&
					(own || (reason != reasonNoMatch && reason != reasonMissingData && reason != qualityStale)),
			},
		)
	}

	if len(cands) == 0 {
		return suppressedBy(unit, reasonNoMatch), nil, nil, nil
	}

	return suppressedBy(unit, firstReason(reasons)), nil, skips, nil
}

// startWins is whether a schedule that starts on the unit in the same minute as an end would run: the start wins
// over the end (IR326 item 4, IR504), so the end is not sent. A start that would not run (owner, restriction,
// capability, offline …) leaves the end as before.
func (m Automations) startWins(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	input *EvaluationInput,
	facts map[string]Fact,
) (bool, error) {
	start := *input
	start.Phase = phaseScheduleStart

	decision, _, _, err := m.decide(ctx, call, unit, &start, facts)

	return decision.Decision == decisionSelected, err
}

// supersededSkips records superseded for each enabled end rule.
func supersededSkips(cands []candidate) []skip {
	var skips []skip

	for i := range cands {
		if cands[i].enabled {
			skips = append(skips, skip{cand: &cands[i], reason: reasonSuperseded, recorded: true})
		}
	}

	return skips
}

// suppressedBy is the decision that suppresses the unit's automation for a reason.
func suppressedBy(unit uuid.UUID, reason string) Decision {
	return Decision{
		UnitID:    unit,
		Decision:  decisionSuppressed,
		Reason:    &reason,
		RuleID:    nil,
		CommandID: nil,
		fresh:     false,
	}
}

// firstReason is the exclusion reason of the candidate with the lowest ID.
func firstReason(reasons map[uuid.UUID]string) string {
	ids := slices.SortedFunc(maps.Keys(reasons), func(left, right uuid.UUID) int {
		return strings.Compare(left.String(), right.String())
	})

	return reasons[ids[0]]
}

// triggerReason is why a candidate's own trigger does not hold ("" when it holds): the rule is disabled, its condition
// is unmet, or its extra conditions are; and whether the rule's own trigger was decided, so that a reason is worth
// logging.
func triggerReason(
	cand *candidate,
	unit uuid.UUID,
	facts map[string]Fact,
	input *EvaluationInput,
) (string, bool) {
	if !cand.enabled {
		return "disabled", false
	}

	if cand.cond != nil {
		reason := match(cand.cond, unit, facts, input.OccurredAt, cand.tz)
		if reason != "" { // the rule's own data arrived, but missing or stale
			_, present := facts[unit.String()+"/"+conditionMetric(cand.cond)]

			return reason, present && (reason == reasonMissingData || reason == qualityStale)
		}
	}

	return extrasMatch(cand.extras, unit, facts, input.OccurredAt, cand.tz), true
}

// eligibility is why a candidate whose trigger holds still cannot run on the unit ("" when it can): its owner may not,
// the owner of a location rule revoked the location consent, the unit cannot take the action, a restriction forbids
// it, or the unit is not ready for a command.
func (m Automations) eligibility(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	cand *candidate,
	target Target,
	found bool,
) (string, error) {
	ok, err := ownerValid(ctx, call, cand.owner, cand.hq, target.OrgID)
	if err != nil {
		return "", err
	}

	if !ok {
		return "owner_forbidden", nil
	}

	kind, err := conditionType(cand.cond)
	if err != nil {
		return "", err
	}

	if kind == condLocation {
		granted, err := identity.ConsentGranted(ctx, call, cand.owner, "location_automation")
		if err != nil {
			return "", apperr.Wrap(err, "identity.ConsentGranted")
		}

		if !granted {
			return "consent_revoked", nil
		}
	}

	unitAction, ok := ParseAction(cand.action)
	if !ok || !found || !unitAction.Supports(target.Caps) {
		return "invalid_capability", nil
	}

	policy, err := m.Cmd.Restrictions.UnitPolicy(ctx, call, unit)
	if err != nil {
		return "", apperr.Wrap(err, "control.Restrictions.UnitPolicy")
	}

	if !Allowed(unitAction, policy) {
		return "restricted", nil
	}

	return m.readiness(ctx, call, unit)
}

// conditionType is the type of a rule's condition, "" for a rule without one.
func conditionType(cond json.RawMessage) (string, error) {
	if len(cond) == 0 {
		return "", nil
	}

	var typed struct {
		Type string `json:"type"`
	}

	err := json.Unmarshal(cond, &typed)

	return typed.Type, apperr.Wrap(err, "json.Unmarshal")
}

// readiness is why the unit cannot take a command now ("" when it can): a reconciliation is required, it is busy, or
// it is offline.
func (m Automations) readiness(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
) (string, error) {
	_, err := m.Cmd.ready(ctx, call, unit)
	if err == nil {
		return "", nil
	}

	var domainErr *apperr.DomainError
	if !errors.As(err, &domainErr) {
		return "", err
	}

	switch {
	case domainErr.MessageKey == "errors.reconciliation_required":
		return "reconciliation_required", nil
	case domainErr.Code == apperr.Conflict:
		return "busy", nil
	case domainErr.Code == apperr.Offline:
		return reasonOffline, nil
	}

	return "", err
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

		if !found || (call.Principal.Role == authz.RoleClient && t.OrgID != call.Principal.OrgID) {
			return apperr.E(apperr.NotFound, apperr.KeyNotFound)
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

	ref := triggerRef(input)
	out := Result{EventID: input.EventID, Results: []Decision{}, Notifications: []json.RawMessage{}}

	for _, unitID := range units {
		decision, win, skips, err := m.decide(ctx, call, unitID, input, facts)
		if err != nil {
			return Result{}, err
		}

		if fire {
			err := recordSkips(ctx, call, unitID, ref, skips, input.OccurredAt)
			if err != nil {
				return Result{}, err
			}
		}

		if fire && win != nil {
			err := m.fireWinner(ctx, call, unitID, ref, win, input, &decision)
			if err != nil {
				return Result{}, err
			}
		}

		if notify &&
			m.Notifier != nil { // notification results are independent of the control decision (SR21/SR25)
			notifications, err := m.notifyUnit(ctx, call, unitID, input, fire, &decision)
			if err != nil {
				return Result{}, err
			}

			out.Notifications = append(out.Notifications, notifications...)
		}

		out.Results = append(out.Results, decision)
	}

	return out, nil
}

// triggerRef names an evaluation's trigger in the run log: a condition's event and phase, or the schedule occurrence,
// which is the same for the scheduler and a UI fire (IR54).
func triggerRef(input *EvaluationInput) string {
	if input.Phase != keyCondition {
		return scheduleRef(input.Phase, input.OccurredAt)
	}

	return input.EventID.String() + "/" + input.Phase
}

// recordSkips keeps in the run log why a rule could not run on the unit (IR215: missing data, consent, restriction,
// offline …), for the skips worth recording.
func recordSkips(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	ref string,
	skips []skip,
	occurredAt time.Time,
) error {
	for _, skipped := range skips {
		if !skipped.recorded {
			continue
		}

		_, err := call.Tx.Exec(
			ctx,
			`INSERT INTO control.automation_runs (tenant_id, rule_kind, rule_id, unit_id, trigger_ref, outcome, skip_reason,
				occurred_at)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, 'skipped', $5, $6) ON CONFLICT DO NOTHING`,
			skipped.cand.kind,
			skipped.cand.id,
			unit,
			ref,
			skipped.reason,
			occurredAt,
		)
		if err != nil {
			return apperr.Wrap(err, "insert control.automation_runs")
		}
	}

	return nil
}

// fireWinner requests the winning rule's command for the unit, once per trigger: a trigger that already ran for the
// rule and unit answers with its command.
func (m Automations) fireWinner(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	ref string,
	win *candidate,
	input *EvaluationInput,
	decision *Decision,
) error {
	var earlier uuid.UUID

	err := call.Tx.QueryRow(ctx, `SELECT command_id FROM control.automation_runs WHERE rule_id = $1 AND unit_id = $2 AND
		trigger_ref = $3 AND command_id IS NOT NULL`,
		win.id, unit, ref).
		Scan(&earlier)
	if err == nil {
		decision.Decision, decision.CommandID = decisionRequested, &earlier

		return nil
	}

	if !errors.Is(err, pgx.ErrNoRows) {
		return apperr.Wrap(err, "select control.automation_runs")
	}

	commandID, err := m.automationCommand(ctx, call, unit, win)
	if err != nil {
		return err
	}

	decision.Decision, decision.CommandID, decision.fresh = decisionRequested, &commandID, true

	_, err = call.Tx.Exec(
		ctx,
		`INSERT INTO control.automation_runs (tenant_id, rule_kind, rule_id, unit_id, trigger_ref, outcome, command_id,
			occurred_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, 'command_created', $5, $6)
		ON CONFLICT (rule_id, unit_id, trigger_ref) DO UPDATE SET outcome = 'command_created', skip_reason = NULL,
		command_id = EXCLUDED.command_id
		WHERE control.automation_runs.command_id IS NULL`,
		win.kind,
		win.id,
		unit,
		ref,
		commandID,
		input.OccurredAt,
	)

	return apperr.Wrap(err, "insert control.automation_runs")
}

// notifyUnit evaluates the unit's alert policies on the facts. When a notification matched while no control rule did,
// the unit's no_match becomes no_control_action.
func (m Automations) notifyUnit(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	input *EvaluationInput,
	fire bool,
	decision *Decision,
) ([]json.RawMessage, error) {
	factsJSON, err := json.Marshal(input.Facts)
	if err != nil {
		return nil, apperr.Wrap(err, "json.Marshal")
	}

	notifications, err := m.Notifier.Evaluate(
		ctx,
		call,
		unit,
		factsJSON,
		input.OccurredAt,
		input.EventID,
		fire,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "control.Notifier.Evaluate")
	}

	matched := false

	for _, n := range notifications {
		var notification struct {
			Decision string `json:"decision"`
		}

		err := json.Unmarshal(n, &notification)
		if err != nil {
			return nil, apperr.Wrap(err, "json.Unmarshal")
		}

		matched = matched || notification.Decision != decisionSuppressed
	}

	if matched && decision.Decision == decisionSuppressed && decision.Reason != nil &&
		*decision.Reason == reasonNoMatch {
		noControl := "no_control_action"
		decision.Reason = &noControl
	}

	return notifications, nil
}

// inputHash identifies an evaluation's input: its time, units and facts, apart from the event ID and the phase.
func inputHash(input *EvaluationInput) (string, error) {
	// the keys are those of the hashes stored in control.evaluation_events: other names would change every hash, and a
	// retry of an event evaluated before would be refused as reused
	//
	//nolint:tagliatelle // the stored hashes' keys
	norm, err := json.Marshal(struct {
		O time.Time   `json:"O"`
		U []uuid.UUID `json:"U"`
		F []Fact      `json:"F"`
	}{input.OccurredAt.UTC(), input.UnitIDs, input.Facts})
	if err != nil {
		return "", apperr.Wrap(err, "json.Marshal")
	}

	sum := sha256.Sum256(norm)

	return hex.EncodeToString(sum[:]), nil
}

// eventResult is the stored result of an event ID evaluated before, and whether there is one: the same input answers
// with it, and other input is CONFLICT, the ID being reused.
func eventResult(
	ctx context.Context,
	call *ops.Call,
	input *EvaluationInput,
	hash string,
) (Result, bool, error) {
	var (
		none       Result
		stored     []byte
		storedHash string
	)

	err := call.Tx.QueryRow(ctx, `SELECT input_hash, result FROM control.evaluation_events WHERE event_id = $1 AND phase =
		$2`, input.EventID, input.Phase).
		Scan(&storedHash, &stored)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}

	if err != nil {
		return none, false, apperr.Wrap(err, "select control.evaluation_events")
	}

	if storedHash != hash {
		return none, false, apperr.E(apperr.Conflict, "errors.event_reused")
	}

	var result Result

	err = json.Unmarshal(stored, &result)

	return result, true, apperr.Wrap(err, "json.Unmarshal")
}

// tickResult is the stored result of the tick an evaluation falls in, and whether there is one. One tick has one
// result (D02): the same input under another event ID reuses it, and other input is CONFLICT.
func tickResult(
	ctx context.Context,
	call *ops.Call,
	input *EvaluationInput,
	hash string,
) (Result, bool, error) {
	var (
		none        Result
		stored      []byte
		storedHash  string
		storedPhase string
	)

	err := call.Tx.QueryRow(ctx, `SELECT input_hash, result, phase FROM control.evaluation_events
		WHERE date_trunc('minute', occurred_at) = date_trunc('minute', $1::timestamptz)
		ORDER BY created_at LIMIT 1`, input.OccurredAt).
		Scan(&storedHash, &stored, &storedPhase)
	if errors.Is(err, pgx.ErrNoRows) {
		return none, false, nil
	}

	if err != nil {
		return none, false, apperr.Wrap(err, "select control.evaluation_events")
	}

	if storedHash != hash || storedPhase != input.Phase {
		return none, false, apperr.E(apperr.Conflict, "errors.tick_already_evaluated")
	}

	var result Result

	err = json.Unmarshal(stored, &result)
	result.EventID = input.EventID

	return result, true, apperr.Wrap(err, "json.Unmarshal")
}

// automationCommand creates the winner's Command (source automation, actor = rule owner).
func (m Automations) automationCommand(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	win *candidate,
) (uuid.UUID, error) {
	dev, err := m.Cmd.ready(ctx, call, unit)
	if err != nil {
		return uuid.Nil, err
	}

	var commandID uuid.UUID

	err = call.Tx.QueryRow(ctx, `INSERT INTO control.commands (tenant_id, unit_id, device_id, actor_membership_id, source,
		action, status, delivery, requested_at, sent_at, expires_at,
		correlation_id, created_at, updated_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, 'automation', $4,
		'requested', 'not_sent', $5, NULL, $6, $7, $5, $5) RETURNING id`,
		unit, dev, win.owner, []byte(win.action), call.Now, call.Now.Add(CommandTTL), call.CorrelationID).
		Scan(&commandID)
	if err == nil {
		call.Emit(
			ops.Event{
				AggregateType: kindCommand,
				AggregateID:   commandID,
				Type:          eventCommandRequested,
				Payload: map[string]any{
					keyUnitID: unit,
					keySource: kindAutomation,
					"ruleId":  win.id,
				},
			},
		)
	}

	return commandID, apperr.Wrap(err, "insert control.commands")
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
	call *ops.Call,
	input *EvaluationInput,
) (Result, error) {
	// the target units first: a stored result is returned only after authorization (IR21), and a caller outside the
	// units' scope gets NOT_FOUND, not the result of another caller's identical input or a tick CONFLICT (IR366)
	targetsErr := m.targets(ctx, call, input)
	if targetsErr != nil {
		return Result{}, targetsErr
	}

	hash, err := inputHash(input)
	if err != nil {
		return Result{}, err
	}

	stored, found, err := eventResult(ctx, call, input, hash)
	if err != nil || found {
		return stored, err
	}

	checkTickErr := checkTick(call, input)
	if checkTickErr != nil {
		return Result{}, checkTickErr
	}

	stored, found, err = tickResult(ctx, call, input, hash)
	if err != nil || found {
		return stored, err
	}

	result, err := m.evaluate(ctx, call, input, true, true)
	if err != nil {
		return result, err
	}

	raw, marshalErr := json.Marshal(result)
	if marshalErr != nil {
		return Result{}, apperr.Wrap(marshalErr, "json.Marshal")
	}

	_, execErr := call.Tx.Exec(
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

	call.Audit(
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

// RegisterEvaluation binds automations.simulate / fire.
func RegisterEvaluation(r *ops.Registry, m Automations) {
	ops.Register(r, "automations.simulate", m.simulate)
	ops.Register(r, "automations.fire", m.fire)
}
