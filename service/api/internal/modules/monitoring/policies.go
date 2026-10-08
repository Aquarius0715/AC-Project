// Package monitoring implements the Monitoring & alerts module.
package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/control"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// PolicyOwner is the owner and kind of an alert policy.
type PolicyOwner struct {
	CustomerID *uuid.UUID
	Kind       string
}

// PolicyUnits is what the policy operations need from Assets (unit attachments live on units, IR108).
type PolicyUnits interface {
	AttachedUnits(ctx context.Context, c *ops.Call, policies []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error)
	PoliciesOfUnits(ctx context.Context, c *ops.Call, units []uuid.UUID) ([]uuid.UUID, error)
	DetachPolicy(ctx context.Context, c *ops.Call, policy uuid.UUID) error
	UnitsOfProperty(ctx context.Context, c *ops.Call, property uuid.UUID) ([]uuid.UUID, error)
	OrgsOfUnits(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID]uuid.UUID, error)
	CustomerOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) (uuid.UUID, bool, error)
	// CustomerState returns the organization of a customer and whether the customer is active.
	CustomerState(ctx context.Context, c *ops.Call, customer uuid.UUID) (org uuid.UUID, active bool, found bool, err error)
}

// Recipients checks alert recipients in Identity & access.
type Recipients interface {
	// NonReaders returns the IDs that are not active memberships able to read the customer organization's alerts.
	NonReaders(ctx context.Context, c *ops.Call, ids []uuid.UUID, customerOrg uuid.UUID) ([]uuid.UUID, error)
}

// Policies is the policy operation set; the zero value answers Owners only.
type Policies struct {
	Units      PolicyUnits
	Recipients Recipients
	Caps       UnitCaps
}

// Owners returns owner and kind for the given policy IDs (missing IDs are absent from the map).
func (Policies) Owners(ctx context.Context, c *ops.Call, ids []uuid.UUID) (map[uuid.UUID]PolicyOwner, error) {
	out := map[uuid.UUID]PolicyOwner{}
	rows, err := c.Tx.Query(ctx, `SELECT id, customer_id, kind FROM monitoring.alert_policies WHERE id = ANY($1)`, ids)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		var o PolicyOwner
		if err := rows.Scan(&id, &o.CustomerID, &o.Kind); err != nil {
			return nil, err
		}
		out[id] = o
	}
	return out, rows.Err()
}

// ---- condition model ----

// ActiveWindow is ActiveWindow of service-contracts.ts.
type ActiveWindow struct {
	Weekdays   []int  `json:"weekdays"`
	StartLocal string `json:"startLocal"`
	EndLocal   string `json:"endLocal"`
}

// AlertCondition is AlertCondition of service-contracts.ts.
type AlertCondition struct {
	Metric            string        `json:"metric"`
	Operator          string        `json:"operator"`
	Threshold         float64       `json:"threshold"`
	RecoveryThreshold float64       `json:"recoveryThreshold"`
	DurationSeconds   int           `json:"durationSeconds"`
	ActiveWindow      *ActiveWindow `json:"activeWindow"`
	Severity          string        `json:"severity"`
}

// DefaultAlertRule is DefaultAlertRule of service-contracts.ts.
type DefaultAlertRule struct {
	AlertCondition
	RuleKey  string `json:"ruleKey"`
	Name     string `json:"name"`
	Category string `json:"category"`
}

func rule(key, name, cat, metric, op string, th, rec float64, dur int, sev string) DefaultAlertRule {
	return DefaultAlertRule{AlertCondition: AlertCondition{Metric: metric, Operator: op, Threshold: th, RecoveryThreshold: rec, DurationSeconds: dur, Severity: sev},
		RuleKey: key, Name: name, Category: cat}
}

// DefaultRules are the six IR120 default rules in their fixed order.
var DefaultRules = []DefaultAlertRule{
	rule("ventilation_co2", "Ventilation", "air_quality", "co2", "gte", 1000, 900, 600, "warning"),
	rule("dust_pm25", "Dust / filter", "air_quality", "pm25", "gte", 35, 30, 1800, "warning"),
	rule("refrigerant_low_pressure", "Refrigerant low pressure", "fault", "refrigerant_pressure", "lte", 350, 380, 120, "critical"),
	rule("compressor_short_cycling", "Compressor short-cycling", "fault", "compressor_cycles", "gte", 4, 3, 10800, "warning"),
	rule("clogged_filter", "Clogged filter", "maintenance", "airflow_drop", "gte", 30, 20, 3600, "normal"),
	rule("ac_offline", "AC offline", "connection", "heartbeat_gap", "gte", 900, 60, 1, "warning"),
}

var (
	hqMetrics     = map[string]bool{"temperature": true, "humidity": true, "co2": true, "pm25": true, "refrigerant_pressure": true, "vibration": true, "power": true}
	clientMetrics = map[string]bool{"temperature": true, "humidity": true, "co2": true, "pm25": true, "power": true}
	operators     = map[string]bool{"gt": true, "gte": true, "lt": true, "lte": true}
	channelSet    = map[string]bool{"inApp": true, "email": true, "whatsapp": true}
	hhmm          = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
)

// validate checks an AlertCondition (IR120 item 3); prefix names the field path.
func (a *AlertCondition) validate(metrics map[string]bool, prefix string, fe map[string]string) {
	if !metrics[a.Metric] {
		fe[prefix+"metric"] = "error.invalid"
	}
	if !operators[a.Operator] {
		fe[prefix+"operator"] = "error.invalid"
	}
	if math.IsNaN(a.Threshold) || math.IsInf(a.Threshold, 0) {
		fe[prefix+"threshold"] = "error.invalid"
	}
	up := a.Operator == "gt" || a.Operator == "gte"
	if (up && !(a.RecoveryThreshold < a.Threshold)) || (!up && !(a.RecoveryThreshold > a.Threshold)) {
		fe[prefix+"recoveryThreshold"] = "error.recoveryDirection"
	}
	if a.DurationSeconds < 1 || a.DurationSeconds > 86400 {
		fe[prefix+"durationSeconds"] = "error.range"
	}
	if !severities[a.Severity] {
		fe[prefix+"severity"] = "error.invalid"
	}
	if w := a.ActiveWindow; w != nil {
		seen := map[int]bool{}
		ok := len(w.Weekdays) >= 1 && len(w.Weekdays) <= 7
		for _, d := range w.Weekdays {
			if d < 1 || d > 7 || seen[d] {
				ok = false
			}
			seen[d] = true
		}
		if !ok {
			fe[prefix+"activeWindow.weekdays"] = "error.invalid"
		}
		if !hhmm.MatchString(w.StartLocal) || !hhmm.MatchString(w.EndLocal) || w.StartLocal == w.EndLocal {
			fe[prefix+"activeWindow"] = "error.window"
		}
	}
}

// ---- views ----

// RuleSetting is DefaultRuleSetting of service-contracts.ts.
type RuleSetting struct {
	ID                    uuid.UUID `json:"id"`
	TenantID              uuid.UUID `json:"tenantId"`
	Version               int       `json:"version"`
	CreatedAt             time.Time `json:"createdAt"`
	UpdatedAt             time.Time `json:"updatedAt"`
	PolicyID              uuid.UUID `json:"policyId"`
	RuleKey               string    `json:"ruleKey"`
	CustomerID            uuid.UUID `json:"customerId"`
	Enabled               bool      `json:"enabled"`
	ChangedByMembershipID uuid.UUID `json:"changedByMembershipId"`
	Reason                *string   `json:"reason"`
}

// Policy is Policy of service-contracts.ts (the fields of the three kinds; MarshalJSON emits the kind's shape).
type Policy struct {
	ID                uuid.UUID
	TenantID          uuid.UUID
	Version           int
	CreatedAt         time.Time
	UpdatedAt         time.Time
	Kind              string
	Name              string
	UnitIDs           []uuid.UUID
	OwnerMembershipID uuid.UUID
	CreatedByUserID   uuid.UUID
	Timezone          string
	Enabled           bool
	Priority          int
	DisabledReason    *string
	// alert
	CustomerID             *uuid.UUID
	Condition              AlertCondition
	RecipientMembershipIDs []uuid.UUID
	Channels               []string
	EscalateAfterMinutes   int
	CooldownMinutes        int
	// default_alert
	Rules        []DefaultAlertRule
	RuleSettings []RuleSetting
	// automation
	AutoCondition json.RawMessage
	Action        json.RawMessage
}

// MarshalJSON implements json.Marshaler.
func (p Policy) MarshalJSON() ([]byte, error) {
	m := map[string]any{"id": p.ID, "tenantId": p.TenantID, "version": p.Version, "createdAt": p.CreatedAt, "updatedAt": p.UpdatedAt,
		"kind": p.Kind, "name": p.Name, "unitIds": p.UnitIDs, "ownerMembershipId": p.OwnerMembershipID, "createdByUserId": p.CreatedByUserID,
		"timezone": p.Timezone, "enabled": p.Enabled, "priority": p.Priority, "disabledReason": p.DisabledReason}
	switch p.Kind {
	case "alert":
		m["customerId"], m["recipientMembershipIds"], m["channels"] = p.CustomerID, p.RecipientMembershipIDs, p.Channels
		m["escalateAfterMinutes"], m["cooldownMinutes"] = p.EscalateAfterMinutes, p.CooldownMinutes
		m["metric"], m["operator"], m["threshold"], m["recoveryThreshold"] = p.Condition.Metric, p.Condition.Operator, p.Condition.Threshold, p.Condition.RecoveryThreshold
		m["durationSeconds"], m["activeWindow"], m["severity"] = p.Condition.DurationSeconds, p.Condition.ActiveWindow, p.Condition.Severity
	case "default_alert":
		m["customerId"], m["rules"], m["ruleSettings"] = nil, p.Rules, p.RuleSettings
	case "automation":
		m["condition"], m["action"] = p.AutoCondition, p.Action
	}
	return json.Marshal(m)
}

const alertPolicyCols = `p.id, p.tenant_id, p.version, p.created_at, p.updated_at, p.kind, p.customer_id, p.name, p.owner_membership_id, p.created_by_user_id,
	p.timezone, p.enabled, p.priority, p.condition, p.rules, p.recipient_membership_ids, p.channels, COALESCE(p.escalate_after_minutes, 0), COALESCE(p.cooldown_minutes, 0)`

func scanAlertPolicy(r pgx.Row) (Policy, error) {
	var p Policy
	var cond, rules []byte
	err := r.Scan(&p.ID, &p.TenantID, &p.Version, &p.CreatedAt, &p.UpdatedAt, &p.Kind, &p.CustomerID, &p.Name, &p.OwnerMembershipID, &p.CreatedByUserID,
		&p.Timezone, &p.Enabled, &p.Priority, &cond, &rules, &p.RecipientMembershipIDs, &p.Channels, &p.EscalateAfterMinutes, &p.CooldownMinutes)
	if err != nil {
		return p, err
	}
	p.UnitIDs, p.RuleSettings = []uuid.UUID{}, []RuleSetting{}
	if len(cond) > 0 {
		if err := json.Unmarshal(cond, &p.Condition); err != nil {
			return p, err
		}
	}
	if len(rules) > 0 {
		if err := json.Unmarshal(rules, &p.Rules); err != nil {
			return p, err
		}
	}
	return p, nil
}

const autoPolicyCols = `p.id, p.tenant_id, p.version, p.created_at, p.updated_at, p.name, p.owner_membership_id, p.created_by_user_id, p.timezone, p.enabled,
	p.priority, p.disabled_reason, p.condition, p.action,
	COALESCE((SELECT array_agg(u.unit_id ORDER BY u.unit_id) FROM control.automation_policy_units u WHERE u.policy_id = p.id), '{}')`

func scanAutoPolicy(r pgx.Row) (Policy, error) {
	p := Policy{Kind: "automation"}
	err := r.Scan(&p.ID, &p.TenantID, &p.Version, &p.CreatedAt, &p.UpdatedAt, &p.Name, &p.OwnerMembershipID, &p.CreatedByUserID, &p.Timezone, &p.Enabled,
		&p.Priority, &p.DisabledReason, &p.AutoCondition, &p.Action, &p.UnitIDs)
	return p, err
}

// ---- scope ----

// grantedKinds returns the policy kinds the caller's granted alternatives allow.
func grantedKinds(c *ops.Call) map[string]bool {
	out := map[string]bool{}
	for _, a := range c.Candidates {
		for _, pr := range a.Predicates {
			if v, ok := strings.CutPrefix(pr, "kind="); ok {
				for _, k := range strings.Split(v, "-or-") {
					out[k] = true
				}
			}
		}
	}
	return out
}

// clientCustomer returns the customer of a client caller (uuid.Nil for other roles).
func (m Policies) clientCustomer(ctx context.Context, c *ops.Call) (uuid.UUID, error) {
	if c.Principal.Role != "client" {
		return uuid.Nil, nil
	}
	id, ok, err := m.Units.CustomerOfOrg(ctx, c, c.Principal.OrgID)
	if err != nil || !ok {
		return uuid.Nil, err
	}
	return id, nil
}

// decorate fills derived fields: attached units of alert policies and the visible rule settings of the default policy.
func (m Policies) decorate(ctx context.Context, c *ops.Call, ps []Policy, settingsFor *uuid.UUID) error {
	var alertIDs []uuid.UUID
	for _, p := range ps {
		if p.Kind == "alert" {
			alertIDs = append(alertIDs, p.ID)
		}
	}
	if len(alertIDs) > 0 {
		att, err := m.Units.AttachedUnits(ctx, c, alertIDs)
		if err != nil {
			return err
		}
		for i := range ps {
			if u, ok := att[ps[i].ID]; ok {
				ps[i].UnitIDs = u
			}
		}
	}
	for i := range ps {
		if ps[i].Kind != "default_alert" {
			continue
		}
		q := `SELECT id, tenant_id, version, created_at, updated_at, policy_id, rule_key, customer_id, enabled, changed_by_membership_id, reason
			FROM monitoring.default_rule_settings WHERE policy_id = $1`
		args := []any{ps[i].ID}
		if settingsFor != nil {
			q += " AND customer_id = $2"
			args = append(args, *settingsFor)
		}
		rows, err := c.Tx.Query(ctx, q+" ORDER BY customer_id, rule_key", args...)
		if err != nil {
			return err
		}
		for rows.Next() {
			var s RuleSetting
			if err := rows.Scan(&s.ID, &s.TenantID, &s.Version, &s.CreatedAt, &s.UpdatedAt, &s.PolicyID, &s.RuleKey, &s.CustomerID, &s.Enabled,
				&s.ChangedByMembershipID, &s.Reason); err != nil {
				rows.Close()
				return err
			}
			ps[i].RuleSettings = append(ps[i].RuleSettings, s)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	return nil
}

// ---- policies.list ----

var kindRank = map[string]int{"default_alert": 0, "alert": 1, "automation": 2}

func (m Policies) list(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Policy], error) {
	var f struct {
		Kind       *string    `json:"kind,omitempty"`
		CustomerID *uuid.UUID `json:"customerId,omitempty"`
		PropertyID *uuid.UUID `json:"propertyId,omitempty"`
		UnitID     *uuid.UUID `json:"unitId,omitempty"`
		Enabled    *bool      `json:"enabled,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil {
			return paging.Page[Policy]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	if f.Kind != nil && kindRank[*f.Kind] == 0 && *f.Kind != "default_alert" {
		return paging.Page[Policy]{}, apperr.Fields(map[string]string{"filters.kind": "error.invalid"})
	}
	var less func(a, b Policy) int
	if in.Sort != nil {
		dir := 1
		if in.Sort.Direction == "desc" {
			dir = -1
		}
		switch in.Sort.Field {
		case "name":
			less = func(a, b Policy) int { return dir * strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) }
		case "priority":
			less = func(a, b Policy) int { return dir * (a.Priority - b.Priority) }
		case "updatedAt":
			less = func(a, b Policy) int { return dir * a.UpdatedAt.Compare(b.UpdatedAt) }
		default:
			return paging.Page[Policy]{}, apperr.Fields(map[string]string{"sort.field": "error.invalid"})
		}
	} else {
		less = func(a, b Policy) int {
			if d := kindRank[a.Kind] - kindRank[b.Kind]; d != 0 {
				return d
			}
			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Policy]{}, err
	}
	kinds := grantedKinds(c)
	if f.Kind != nil {
		kinds = map[string]bool{*f.Kind: kinds[*f.Kind]}
	}
	own, err := m.clientCustomer(ctx, c)
	if err != nil {
		return paging.Page[Policy]{}, err
	}
	// unit filters: the units in question (nil = no unit filter)
	var units []uuid.UUID
	if f.UnitID != nil {
		units = []uuid.UUID{*f.UnitID}
	}
	if f.PropertyID != nil {
		pu, err := m.Units.UnitsOfProperty(ctx, c, *f.PropertyID)
		if err != nil {
			return paging.Page[Policy]{}, err
		}
		if units == nil {
			units = pu
		} else {
			units = intersect(units, pu)
		}
	}
	var all []Policy
	if kinds["alert"] || kinds["default_alert"] {
		var args []any
		add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
		var ks []string
		for _, k := range []string{"alert", "default_alert"} {
			if kinds[k] && !(k == "default_alert" && units != nil) {
				ks = append(ks, k)
			}
		}
		conds := []string{"p.kind = ANY(" + add(ks) + ")"}
		if own != uuid.Nil || c.Principal.Role == "client" {
			conds = append(conds, "(p.kind = 'default_alert' OR p.customer_id = "+add(own)+")")
		}
		if f.CustomerID != nil {
			conds = append(conds, "(p.kind = 'default_alert' OR p.customer_id = "+add(*f.CustomerID)+")")
		}
		if f.Enabled != nil {
			conds = append(conds, "p.enabled = "+add(*f.Enabled))
		}
		if units != nil {
			ids, err := m.Units.PoliciesOfUnits(ctx, c, units)
			if err != nil {
				return paging.Page[Policy]{}, err
			}
			conds = append(conds, "p.id = ANY("+add(ids)+")")
		}
		rows, err := c.Tx.Query(ctx, "SELECT "+alertPolicyCols+" FROM monitoring.alert_policies p WHERE "+strings.Join(conds, " AND "), args...)
		if err != nil {
			return paging.Page[Policy]{}, err
		}
		for rows.Next() {
			p, err := scanAlertPolicy(rows)
			if err != nil {
				rows.Close()
				return paging.Page[Policy]{}, err
			}
			all = append(all, p)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return paging.Page[Policy]{}, err
		}
	}
	if kinds["automation"] {
		autos, err := m.automationPolicies(ctx, c, f.Enabled)
		if err != nil {
			return paging.Page[Policy]{}, err
		}
		var custOrg uuid.UUID
		if f.CustomerID != nil {
			org, _, found, err := m.Units.CustomerState(ctx, c, *f.CustomerID)
			if err != nil {
				return paging.Page[Policy]{}, err
			}
			if found {
				custOrg = org
			}
		}
		for _, p := range autos {
			if units != nil && len(intersect(p.UnitIDs, units)) == 0 {
				continue
			}
			if f.CustomerID != nil {
				orgs, err := m.Units.OrgsOfUnits(ctx, c, p.UnitIDs)
				if err != nil {
					return paging.Page[Policy]{}, err
				}
				match := false
				for _, o := range orgs {
					match = match || o == custOrg
				}
				if !match {
					continue
				}
			}
			all = append(all, p)
		}
	}
	sort.SliceStable(all, func(i, j int) bool {
		if d := less(all[i], all[j]); d != 0 {
			return d < 0
		}
		return all[i].ID.String() < all[j].ID.String()
	})
	total := len(all)
	end := min(w.Offset+w.Limit, total)
	page := []Policy{}
	if w.Offset < total {
		page = append(page, all[w.Offset:end]...)
	}
	settingsFor := f.CustomerID
	if c.Principal.Role == "client" {
		settingsFor = &own
	}
	if err := m.decorate(ctx, c, page, settingsFor); err != nil {
		return paging.Page[Policy]{}, err
	}
	return paging.Page[Policy]{Items: page, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

func intersect(a, b []uuid.UUID) []uuid.UUID {
	in := map[uuid.UUID]bool{}
	for _, x := range b {
		in[x] = true
	}
	out := []uuid.UUID{}
	for _, x := range a {
		if in[x] {
			out = append(out, x)
		}
	}
	return out
}

func (m Policies) automationPolicies(ctx context.Context, c *ops.Call, enabled *bool) ([]Policy, error) {
	q, args := "SELECT "+autoPolicyCols+" FROM control.automation_policies p", []any{}
	if enabled != nil {
		q, args = q+" WHERE p.enabled = $1", append(args, *enabled)
	}
	rows, err := c.Tx.Query(ctx, q, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []Policy
	for rows.Next() {
		p, err := scanAutoPolicy(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, rows.Err()
}

// ---- policies.get ----

// PolicyIDInput is the {policyId} input of policies.get / policies.delete.
type PolicyIDInput struct {
	PolicyID uuid.UUID `json:"policyId"`
}

// Validate implements ops.Validator.
func (in *PolicyIDInput) Validate() map[string]string {
	if in.PolicyID == uuid.Nil {
		return map[string]string{"policyId": "error.required"}
	}
	return nil
}

// load reads one policy in the caller's scope (kind granted, client: own customer or default); lock adds FOR UPDATE.
func (m Policies) load(ctx context.Context, c *ops.Call, id uuid.UUID, lock bool) (Policy, error) {
	return m.loadKinds(ctx, c, id, lock, grantedKinds(c))
}

func (m Policies) loadKinds(ctx context.Context, c *ops.Call, id uuid.UUID, lock bool, kinds map[string]bool) (Policy, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}
	p, err := scanAlertPolicy(c.Tx.QueryRow(ctx, "SELECT "+alertPolicyCols+" FROM monitoring.alert_policies p WHERE p.id = $1"+suffix, id))
	if errors.Is(err, pgx.ErrNoRows) {
		p, err = scanAutoPolicy(c.Tx.QueryRow(ctx, "SELECT "+autoPolicyCols+" FROM control.automation_policies p WHERE p.id = $1"+suffix, id))
	}
	if errors.Is(err, pgx.ErrNoRows) {
		return p, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return p, err
	}
	if !kinds[p.Kind] {
		return p, apperr.E(apperr.NotFound, "error.notFound")
	}
	if c.Principal.Role == "client" && p.Kind == "alert" {
		own, err := m.clientCustomer(ctx, c)
		if err != nil {
			return p, err
		}
		if p.CustomerID == nil || *p.CustomerID != own {
			return p, apperr.E(apperr.NotFound, "error.notFound")
		}
	}
	return p, nil
}

func (m Policies) get(ctx context.Context, c *ops.Call, in *PolicyIDInput) (Policy, error) {
	p, err := m.load(ctx, c, in.PolicyID, false)
	if err != nil {
		return p, err
	}
	var settingsFor *uuid.UUID
	if c.Principal.Role == "client" {
		own, err := m.clientCustomer(ctx, c)
		if err != nil {
			return p, err
		}
		settingsFor = &own
	}
	ps := []Policy{p}
	if err := m.decorate(ctx, c, ps, settingsFor); err != nil {
		return p, err
	}
	return ps[0], nil
}

// ---- policies.save ----

// SaveInput is PolicyInput (alert, default_alert or automation; the kind decides which fields apply).
type SaveInput struct {
	ID       *uuid.UUID `json:"id,omitempty"`
	Kind     string     `json:"kind"`
	Name     *string    `json:"name,omitempty"`
	Timezone *string    `json:"timezone,omitempty"`
	Enabled  *bool      `json:"enabled,omitempty"`
	Priority *int       `json:"priority,omitempty"`
	// alert
	CustomerID             *uuid.UUID    `json:"customerId,omitempty"`
	Metric                 *string       `json:"metric,omitempty"`
	Operator               *string       `json:"operator,omitempty"`
	Threshold              *float64      `json:"threshold,omitempty"`
	RecoveryThreshold      *float64      `json:"recoveryThreshold,omitempty"`
	DurationSeconds        *int          `json:"durationSeconds,omitempty"`
	ActiveWindow           *ActiveWindow `json:"activeWindow,omitempty"`
	Severity               *string       `json:"severity,omitempty"`
	RecipientMembershipIDs []uuid.UUID   `json:"recipientMembershipIds,omitempty"`
	Channels               []string      `json:"channels,omitempty"`
	EscalateAfterMinutes   *int          `json:"escalateAfterMinutes,omitempty"`
	CooldownMinutes        *int          `json:"cooldownMinutes,omitempty"`
	// default_alert
	Rules []DefaultAlertRule `json:"rules,omitempty"`
	// automation
	UnitIDs   []uuid.UUID     `json:"unitIds,omitempty"`
	Condition json.RawMessage `json:"condition,omitempty"`
	Action    json.RawMessage `json:"action,omitempty"`
}

// Validate implements ops.Validator for the fields that need no lookup.
func (in *SaveInput) Validate() map[string]string {
	fe := map[string]string{}
	switch in.Kind {
	case "default_alert":
		if in.ID == nil {
			fe["id"] = "error.required"
		}
		if len(in.Rules) != len(DefaultRules) {
			fe["rules"] = "error.fixedRules"
		}
		for i, r := range in.Rules {
			if i < len(DefaultRules) && (r.RuleKey != DefaultRules[i].RuleKey || r.Name != DefaultRules[i].Name || r.Category != DefaultRules[i].Category) {
				fe["rules"] = "error.fixedRules"
			}
			if i < len(DefaultRules) {
				metrics := map[string]bool{DefaultRules[i].Metric: true}
				r.validate(metrics, fmt.Sprintf("rules.%d.", i), fe)
			}
		}
		return fe
	case "alert", "automation":
	default:
		fe["kind"] = "error.invalid"
		return fe
	}
	if in.Name == nil {
		fe["name"] = "error.required"
	} else {
		*in.Name = strings.TrimSpace(*in.Name)
		if n := utf8.RuneCountInString(*in.Name); n < 1 || n > 120 {
			fe["name"] = "error.length"
		}
	}
	if in.Timezone == nil {
		fe["timezone"] = "error.required"
	} else if _, err := time.LoadLocation(*in.Timezone); err != nil || *in.Timezone == "" || *in.Timezone == "Local" {
		fe["timezone"] = "error.invalid"
	}
	if in.Enabled == nil {
		fe["enabled"] = "error.required"
	}
	if in.Priority == nil || *in.Priority < 0 || *in.Priority > 100 {
		fe["priority"] = "error.range"
	}
	if in.Kind == "automation" {
		if n := len(in.UnitIDs); n < 1 || n > 500 || hasDup(in.UnitIDs) {
			fe["unitIds"] = "error.invalid"
		}
		if !validAutoCondition(in.Condition) {
			fe["condition"] = "error.invalid"
		}
		if _, ok := control.ParseAction(in.Action); !ok {
			fe["action"] = "error.invalid"
		}
		if in.CustomerID != nil || in.Metric != nil || in.RecipientMembershipIDs != nil || in.Channels != nil || in.Rules != nil {
			fe["kind"] = "error.fieldsOfOtherKind"
		}
	}
	if in.Kind == "alert" {
		if in.Condition != nil || in.Action != nil {
			fe["kind"] = "error.fieldsOfOtherKind"
		}
		if in.CustomerID == nil {
			fe["customerId"] = "error.required"
		}
		if in.UnitIDs != nil {
			fe["unitIds"] = "error.unitsAttachOnUnit"
		}
		if in.Metric == nil || in.Operator == nil || in.Threshold == nil || in.RecoveryThreshold == nil || in.DurationSeconds == nil || in.Severity == nil {
			fe["condition"] = "error.required"
		} else {
			cond := in.condition()
			cond.validate(hqMetrics, "", fe)
		}
		if n := len(in.RecipientMembershipIDs); n < 1 || n > 20 || hasDup(in.RecipientMembershipIDs) {
			fe["recipientMembershipIds"] = "error.invalid"
		}
		seen := map[string]bool{}
		ok := len(in.Channels) >= 1 && len(in.Channels) <= 3
		for _, ch := range in.Channels {
			ok = ok && channelSet[ch] && !seen[ch]
			seen[ch] = true
		}
		if !ok || !seen["inApp"] {
			fe["channels"] = "error.invalid"
		}
		if in.EscalateAfterMinutes == nil || *in.EscalateAfterMinutes < 1 || *in.EscalateAfterMinutes > 1440 {
			fe["escalateAfterMinutes"] = "error.range"
		}
		if in.CooldownMinutes == nil || *in.CooldownMinutes < 1 || *in.CooldownMinutes > 1440 {
			fe["cooldownMinutes"] = "error.range"
		}
	}
	return fe
}

func (in *SaveInput) condition() AlertCondition {
	return AlertCondition{Metric: *in.Metric, Operator: *in.Operator, Threshold: *in.Threshold, RecoveryThreshold: *in.RecoveryThreshold,
		DurationSeconds: *in.DurationSeconds, ActiveWindow: in.ActiveWindow, Severity: *in.Severity}
}

func hasDup(ids []uuid.UUID) bool {
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if seen[id] {
			return true
		}
		seen[id] = true
	}
	return false
}

func (m Policies) save(ctx context.Context, c *ops.Call, in *SaveInput) (Policy, error) {
	if !grantedKinds(c)[in.Kind] {
		return Policy{}, apperr.E(apperr.Forbidden, "error.forbidden")
	}
	switch in.Kind {
	case "default_alert":
		return m.saveDefault(ctx, c, in)
	case "automation":
		return m.saveAutomation(ctx, c, in)
	}
	return m.saveAlert(ctx, c, in)
}

func (m Policies) saveDefault(ctx context.Context, c *ops.Call, in *SaveInput) (Policy, error) {
	p, err := m.load(ctx, c, *in.ID, true)
	if err != nil {
		return p, err
	}
	if p.Kind != "default_alert" {
		return p, apperr.Fields(map[string]string{"kind": "error.kindFixed"})
	}
	if p.Version != *c.ExpectedVersion {
		return p, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	rules, _ := json.Marshal(in.Rules)
	if _, err := c.Tx.Exec(ctx, `UPDATE monitoring.alert_policies SET rules = $2, version = version + 1, updated_at = platform.app_now() WHERE id = $1`, p.ID, rules); err != nil {
		return p, err
	}
	return m.finishSave(ctx, c, p.ID, "PolicyChanged", c.ExpectedVersion)
}

func (m Policies) saveAlert(ctx context.Context, c *ops.Call, in *SaveInput) (Policy, error) {
	cond := in.condition()
	if c.Principal.Role == "client" {
		fe := map[string]string{}
		cond.validate(clientMetrics, "", fe)
		if len(fe) > 0 {
			return Policy{}, apperr.Fields(fe)
		}
		own, err := m.clientCustomer(ctx, c)
		if err != nil {
			return Policy{}, err
		}
		if *in.CustomerID != own {
			return Policy{}, apperr.E(apperr.NotFound, "error.notFound")
		}
	}
	org, active, found, err := m.Units.CustomerState(ctx, c, *in.CustomerID)
	if err != nil {
		return Policy{}, err
	}
	if !found {
		return Policy{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if !active {
		return Policy{}, apperr.Fields(map[string]string{"customerId": "error.inactiveCustomer"})
	}
	bad, err := m.Recipients.NonReaders(ctx, c, in.RecipientMembershipIDs, org)
	if err != nil {
		return Policy{}, err
	}
	if len(bad) > 0 {
		return Policy{}, apperr.Fields(map[string]string{"recipientMembershipIds": "error.recipientCannotRead"})
	}
	condJSON, _ := json.Marshal(cond)
	if in.ID == nil {
		var id uuid.UUID
		err := c.Tx.QueryRow(ctx, `INSERT INTO monitoring.alert_policies (tenant_id, kind, customer_id, name, owner_membership_id, created_by_user_id, timezone,
			enabled, priority, condition, recipient_membership_ids, channels, escalate_after_minutes, cooldown_minutes)
			VALUES (current_setting('app.tenant_id')::uuid, 'alert', $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING id`,
			*in.CustomerID, *in.Name, c.Principal.MembershipID, c.Principal.UserID, *in.Timezone, *in.Enabled, *in.Priority, condJSON,
			in.RecipientMembershipIDs, in.Channels, *in.EscalateAfterMinutes, *in.CooldownMinutes).Scan(&id)
		if err != nil {
			return Policy{}, err
		}
		return m.finishSave(ctx, c, id, "PolicyCreated", nil)
	}
	p, err := m.load(ctx, c, *in.ID, true)
	if err != nil {
		return p, err
	}
	if p.Kind != "alert" {
		return p, apperr.Fields(map[string]string{"kind": "error.kindFixed"})
	}
	if p.CustomerID == nil || *p.CustomerID != *in.CustomerID {
		return p, apperr.Fields(map[string]string{"customerId": "error.ownerFixed"})
	}
	if p.Version != *c.ExpectedVersion {
		return p, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE monitoring.alert_policies SET name = $2, timezone = $3, enabled = $4, priority = $5, condition = $6,
		recipient_membership_ids = $7, channels = $8, escalate_after_minutes = $9, cooldown_minutes = $10, version = version + 1, updated_at = platform.app_now()
		WHERE id = $1`, p.ID, *in.Name, *in.Timezone, *in.Enabled, *in.Priority, condJSON, in.RecipientMembershipIDs, in.Channels,
		*in.EscalateAfterMinutes, *in.CooldownMinutes); err != nil {
		return p, err
	}
	return m.finishSave(ctx, c, p.ID, "PolicyChanged", c.ExpectedVersion)
}

// UnitCaps returns the action capabilities of an active unit (found=false for unknown or archived units).
type UnitCaps interface {
	UnitCaps(ctx context.Context, c *ops.Call, unit uuid.UUID) (control.Caps, bool, error)
}

var autoConditionTypes = map[string]bool{"occupancy": true, "tariff": true, "peak": true, "solar": true, "battery": true}

// validAutoCondition checks an automation Condition of DD-A11 (occupancy, tariff, peak, solar, battery).
func validAutoCondition(raw json.RawMessage) bool {
	var c struct {
		Type     string   `json:"type"`
		Occupied *bool    `json:"occupied,omitempty"`
		Active   *bool    `json:"active,omitempty"`
		Operator *string  `json:"operator,omitempty"`
		Value    *float64 `json:"value,omitempty"`
		Unit     *string  `json:"unit,omitempty"`
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if len(raw) == 0 || dec.Decode(&c) != nil || !autoConditionTypes[c.Type] {
		return false
	}
	compare := func(unit string) bool {
		return c.Operator != nil && operators[*c.Operator] && c.Value != nil && !math.IsNaN(*c.Value) && !math.IsInf(*c.Value, 0) &&
			c.Unit != nil && *c.Unit == unit && c.Occupied == nil && c.Active == nil
	}
	switch c.Type {
	case "occupancy":
		return c.Occupied != nil && c.Active == nil && c.Operator == nil && c.Value == nil && c.Unit == nil
	case "peak":
		return c.Active != nil && c.Occupied == nil && c.Operator == nil && c.Value == nil && c.Unit == nil
	case "tariff":
		return compare("MYR_per_kWh") && *c.Value >= 0
	default: // solar, battery
		return compare("kW") && *c.Value >= 0
	}
}

// saveAutomation stores an HQ automation policy (DD-A11, IR120 item 9).
func (m Policies) saveAutomation(ctx context.Context, c *ops.Call, in *SaveInput) (Policy, error) {
	action, _ := control.ParseAction(in.Action)
	for _, u := range in.UnitIDs {
		caps, found, err := m.Caps.UnitCaps(ctx, c, u)
		if err != nil {
			return Policy{}, err
		}
		if !found {
			return Policy{}, apperr.E(apperr.NotFound, "error.notFound")
		}
		if !action.Supports(caps) {
			return Policy{}, apperr.Fields(map[string]string{"action": "error.unsupportedAction"})
		}
	}
	id := uuid.Nil
	var prev *int
	if in.ID == nil {
		err := c.Tx.QueryRow(ctx, `INSERT INTO control.automation_policies (tenant_id, name, owner_membership_id, created_by_user_id, timezone, enabled, priority, condition, action)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			*in.Name, c.Principal.MembershipID, c.Principal.UserID, *in.Timezone, *in.Enabled, *in.Priority, []byte(in.Condition), []byte(in.Action)).Scan(&id)
		if err != nil {
			return Policy{}, err
		}
	} else {
		p, err := m.load(ctx, c, *in.ID, true)
		if err != nil {
			return p, err
		}
		if p.Kind != "automation" {
			return p, apperr.Fields(map[string]string{"kind": "error.kindFixed"})
		}
		if p.Version != *c.ExpectedVersion {
			return p, apperr.E(apperr.Conflict, "error.versionConflict")
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE control.automation_policies SET name = $2, timezone = $3, enabled = $4, priority = $5, condition = $6, action = $7,
			disabled_reason = NULL, version = version + 1, updated_at = platform.app_now() WHERE id = $1`,
			p.ID, *in.Name, *in.Timezone, *in.Enabled, *in.Priority, []byte(in.Condition), []byte(in.Action)); err != nil {
			return p, err
		}
		if _, err := c.Tx.Exec(ctx, `DELETE FROM control.automation_policy_units WHERE policy_id = $1`, p.ID); err != nil {
			return p, err
		}
		id, prev = p.ID, c.ExpectedVersion
	}
	for _, u := range in.UnitIDs {
		if _, err := c.Tx.Exec(ctx, `INSERT INTO control.automation_policy_units (tenant_id, policy_id, unit_id) VALUES (current_setting('app.tenant_id')::uuid, $1, $2)`,
			id, u); err != nil {
			return Policy{}, err
		}
	}
	event := "PolicyCreated"
	if prev != nil {
		event = "PolicyChanged"
	}
	return m.finishSave(ctx, c, id, event, prev)
}

func (m Policies) finishSave(ctx context.Context, c *ops.Call, id uuid.UUID, event string, prev *int) (Policy, error) {
	p, err := m.get(ctx, c, &PolicyIDInput{PolicyID: id})
	if err != nil {
		return p, err
	}
	c.Emit(ops.Event{AggregateType: "policy", AggregateID: p.ID, Type: event, Payload: map[string]any{"kind": p.Kind}})
	c.Audit(ops.AuditEntry{Action: "policies.save", TargetKind: "policy", TargetID: p.ID.String(), PreviousVersion: prev, NextVersion: &p.Version})
	return p, nil
}

// ---- policies.delete ----

// Deleted is DeletedResource of service-contracts.ts.
type Deleted struct {
	ID      uuid.UUID `json:"id"`
	Deleted bool      `json:"deleted"`
}

func (m Policies) delete(ctx context.Context, c *ops.Call, in *PolicyIDInput) (Deleted, error) {
	kinds := grantedKinds(c)
	kinds["default_alert"] = kinds["alert"] // the default policy is visible to alert writers so its delete is VALIDATION (IR108)
	p, err := m.loadKinds(ctx, c, in.PolicyID, true, kinds)
	if err != nil {
		return Deleted{}, err
	}
	if p.Kind == "default_alert" {
		return Deleted{}, apperr.Fields(map[string]string{"policyId": "error.defaultPolicyFixed"})
	}
	if p.Version != *c.ExpectedVersion {
		return Deleted{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	switch p.Kind {
	case "alert":
		if err := m.Units.DetachPolicy(ctx, c, p.ID); err != nil {
			return Deleted{}, err
		}
		if _, err := c.Tx.Exec(ctx, `DELETE FROM monitoring.alert_policies WHERE id = $1`, p.ID); err != nil {
			return Deleted{}, err
		}
	case "automation":
		if _, err := c.Tx.Exec(ctx, `DELETE FROM control.automation_policies WHERE id = $1`, p.ID); err != nil {
			return Deleted{}, err
		}
	}
	c.Emit(ops.Event{AggregateType: "policy", AggregateID: p.ID, Type: "PolicyDeleted", Payload: map[string]any{"kind": p.Kind}})
	c.Audit(ops.AuditEntry{Action: "policies.delete", TargetKind: "policy", TargetID: p.ID.String(), PreviousVersion: c.ExpectedVersion})
	return Deleted{ID: p.ID, Deleted: true}, nil
}

// ---- policies.setDefaultRule ----

// SetDefaultRuleInput is policies.setDefaultRule input.
type SetDefaultRuleInput struct {
	PolicyID   uuid.UUID `json:"policyId"`
	RuleKey    string    `json:"ruleKey"`
	CustomerID uuid.UUID `json:"customerId"`
	Enabled    *bool     `json:"enabled"`
	Reason     *string   `json:"reason,omitempty"`
}

// Validate implements ops.Validator.
func (in *SetDefaultRuleInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.PolicyID == uuid.Nil {
		fe["policyId"] = "error.required"
	}
	if in.CustomerID == uuid.Nil {
		fe["customerId"] = "error.required"
	}
	if in.Enabled == nil {
		fe["enabled"] = "error.required"
	}
	known := false
	for _, r := range DefaultRules {
		known = known || r.RuleKey == in.RuleKey
	}
	if !known {
		fe["ruleKey"] = "error.invalid"
	}
	if in.Reason != nil {
		*in.Reason = strings.TrimSpace(*in.Reason)
		if utf8.RuneCountInString(*in.Reason) > 500 {
			fe["reason"] = "error.length"
		}
	}
	return fe
}

func (m Policies) setDefaultRule(ctx context.Context, c *ops.Call, in *SetDefaultRuleInput) (RuleSetting, error) {
	if c.Principal.Role == "client" {
		own, err := m.clientCustomer(ctx, c)
		if err != nil {
			return RuleSetting{}, err
		}
		if in.CustomerID != own {
			return RuleSetting{}, apperr.E(apperr.NotFound, "error.notFound")
		}
		if c.Principal.ClientRole != "owner" {
			return RuleSetting{}, apperr.E(apperr.Forbidden, "error.ownerOnly")
		}
	}
	var kind string
	err := c.Tx.QueryRow(ctx, `SELECT kind FROM monitoring.alert_policies WHERE id = $1`, in.PolicyID).Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return RuleSetting{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return RuleSetting{}, err
	}
	if kind != "default_alert" {
		return RuleSetting{}, apperr.Fields(map[string]string{"policyId": "error.notDefaultPolicy"})
	}
	if _, _, found, err := m.Units.CustomerState(ctx, c, in.CustomerID); err != nil {
		return RuleSetting{}, err
	} else if !found {
		return RuleSetting{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	var cur int
	err = c.Tx.QueryRow(ctx, `SELECT version FROM monitoring.default_rule_settings WHERE policy_id = $1 AND rule_key = $2 AND customer_id = $3 FOR UPDATE`,
		in.PolicyID, in.RuleKey, in.CustomerID).Scan(&cur)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return RuleSetting{}, err
	}
	if cur != *c.ExpectedVersion {
		return RuleSetting{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	var s RuleSetting
	err = c.Tx.QueryRow(ctx, `INSERT INTO monitoring.default_rule_settings (tenant_id, policy_id, rule_key, customer_id, enabled, changed_by_membership_id, reason)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, NULLIF($6, ''))
		ON CONFLICT (policy_id, rule_key, customer_id) DO UPDATE SET enabled = EXCLUDED.enabled, changed_by_membership_id = EXCLUDED.changed_by_membership_id,
			reason = EXCLUDED.reason, version = monitoring.default_rule_settings.version + 1, updated_at = platform.app_now()
		RETURNING id, tenant_id, version, created_at, updated_at, policy_id, rule_key, customer_id, enabled, changed_by_membership_id, reason`,
		in.PolicyID, in.RuleKey, in.CustomerID, *in.Enabled, c.Principal.MembershipID, deref(in.Reason)).
		Scan(&s.ID, &s.TenantID, &s.Version, &s.CreatedAt, &s.UpdatedAt, &s.PolicyID, &s.RuleKey, &s.CustomerID, &s.Enabled, &s.ChangedByMembershipID, &s.Reason)
	if err != nil {
		return s, err
	}
	c.Emit(ops.Event{AggregateType: "policy", AggregateID: in.PolicyID, Type: "DefaultRuleChanged", Payload: map[string]any{"ruleKey": in.RuleKey, "customerId": in.CustomerID, "enabled": s.Enabled}})
	c.Audit(ops.AuditEntry{Action: "policies.setDefaultRule", TargetKind: "default_rule_setting", TargetID: s.ID.String(), PreviousVersion: c.ExpectedVersion,
		NextVersion: &s.Version, Reason: deref(in.Reason)})
	return s, nil
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// RegisterPolicies binds the policy operations.
func RegisterPolicies(r *ops.Registry, m Policies) {
	ops.Register(r, "policies.list", m.list)
	ops.Register(r, "policies.get", m.get)
	ops.Register(r, "policies.save", m.save)
	ops.Register(r, "policies.delete", m.delete)
	ops.Register(r, "policies.setDefaultRule", m.setDefaultRule)
}
