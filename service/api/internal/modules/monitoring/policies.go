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
	AttachedUnits(
		ctx context.Context,
		c *ops.Call,
		policies []uuid.UUID,
	) (map[uuid.UUID][]uuid.UUID, error)
	PoliciesOfUnits(ctx context.Context, c *ops.Call, units []uuid.UUID) ([]uuid.UUID, error)
	DetachPolicy(ctx context.Context, c *ops.Call, policy uuid.UUID) error
	UnitsOfProperty(ctx context.Context, c *ops.Call, property uuid.UUID) ([]uuid.UUID, error)
	OrgsOfUnits(
		ctx context.Context,
		c *ops.Call,
		units []uuid.UUID,
	) (map[uuid.UUID]uuid.UUID, error)
	CustomerOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) (uuid.UUID, bool, error)
	// CustomerState returns the organization of a customer and whether the customer is active.
	CustomerState(
		ctx context.Context,
		c *ops.Call,
		customer uuid.UUID,
	) (org uuid.UUID, active bool, found bool, err error)
}

// Recipients checks alert recipients in Identity & access.
type Recipients interface {
	// NonReaders returns the IDs that are not active memberships able to read the customer organization's alerts.
	NonReaders(
		ctx context.Context,
		c *ops.Call,
		ids []uuid.UUID,
		customerOrg uuid.UUID,
	) ([]uuid.UUID, error)
}

// Policies is the policy operation set; the zero value answers Owners only.
type Policies struct {
	Units      PolicyUnits
	Recipients Recipients
	Caps       UnitCaps
}

// Owners returns owner and kind for the given policy IDs (missing IDs are absent from the map).
func (Policies) Owners(
	ctx context.Context,
	c *ops.Call,
	ids []uuid.UUID,
) (map[uuid.UUID]PolicyOwner, error) {
	out := map[uuid.UUID]PolicyOwner{}

	rows, err := c.Tx.Query(
		ctx,
		`SELECT id, customer_id, kind FROM monitoring.alert_policies WHERE id = ANY($1)`,
		ids,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select monitoring.alert_policies")
	}
	defer rows.Close()

	for rows.Next() {
		var (
			policyID    uuid.UUID
			policyOwner PolicyOwner
		)

		err := rows.Scan(&policyID, &policyOwner.CustomerID, &policyOwner.Kind)
		if err != nil {
			return nil, apperr.Wrap(err, "select monitoring.alert_policies")
		}

		out[policyID] = policyOwner
	}

	return out, apperr.Wrap(rows.Err(), "select monitoring.alert_policies")
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

func rule(
	key, name, cat, metric, operation string,
	threshold, rec float64,
	dur int,
	sev string,
) DefaultAlertRule {
	return DefaultAlertRule{
		AlertCondition: AlertCondition{
			Metric:            metric,
			Operator:          operation,
			Threshold:         threshold,
			RecoveryThreshold: rec,
			DurationSeconds:   dur,
			Severity:          sev, ActiveWindow: nil,
		},
		RuleKey:  key,
		Name:     name,
		Category: cat,
	}
}

// DefaultRules are the six IR120 default rules in their fixed order.
var DefaultRules = []DefaultAlertRule{
	rule("ventilation_co2", "Ventilation", "air_quality", "co2", "gte", 1000, 900, 600, "warning"),
	rule("dust_pm25", "Dust / filter", "air_quality", "pm25", "gte", 35, 30, 1800, "warning"),
	rule(
		"refrigerant_low_pressure",
		"Refrigerant low pressure",
		"fault",
		"refrigerant_pressure",
		"lte",
		350,
		380,
		120,
		"critical",
	),
	rule(
		"compressor_short_cycling",
		"Compressor short-cycling",
		"fault",
		"compressor_cycles",
		"gte",
		4,
		3,
		10800,
		"warning",
	),
	rule(
		"clogged_filter",
		"Clogged filter",
		"maintenance",
		"airflow_drop",
		"gte",
		30,
		20,
		3600,
		"normal",
	),
	rule(
		"ac_offline",
		"AC offline",
		"connection",
		"heartbeat_gap",
		"gte",
		15,
		1,
		1,
		"warning",
	), // heartbeat_gap is in minutes (UnitSymbol min): 15 minutes without a heartbeat
}

var (
	hqMetrics = map[string]bool{
		"temperature":          true,
		"humidity":             true,
		"co2":                  true,
		"pm25":                 true,
		"refrigerant_pressure": true,
		"vibration":            true,
		"power":                true,
	}
	clientMetrics = map[string]bool{
		"temperature": true,
		"humidity":    true,
		"co2":         true,
		"pm25":        true,
		"power":       true,
	}
	operators  = map[string]bool{"gt": true, "gte": true, "lt": true, "lte": true}
	channelSet = map[string]bool{"inApp": true, "email": true, "whatsapp": true}
	hhmm       = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)
)

// validate checks an AlertCondition (IR120 item 3); prefix names the field path.
func (a *AlertCondition) validate(
	metrics map[string]bool,
	prefix string,
	fieldErrs map[string]string,
) {
	if !metrics[a.Metric] {
		fieldErrs[prefix+"metric"] = apperr.KeyInvalid
	}

	if !operators[a.Operator] {
		fieldErrs[prefix+"operator"] = apperr.KeyInvalid
	}

	if math.IsNaN(a.Threshold) || math.IsInf(a.Threshold, 0) {
		fieldErrs[prefix+"threshold"] = apperr.KeyInvalid
	}

	up := a.Operator == "gt" || a.Operator == "gte"
	if (up && !(a.RecoveryThreshold < a.Threshold)) ||
		(!up && !(a.RecoveryThreshold > a.Threshold)) {
		fieldErrs[prefix+"recoveryThreshold"] = "error.recoveryDirection"
	}

	if a.DurationSeconds < 1 || a.DurationSeconds > 86400 {
		fieldErrs[prefix+"durationSeconds"] = apperr.KeyRange
	}

	if !severities[a.Severity] {
		fieldErrs[prefix+"severity"] = apperr.KeyInvalid
	}

	if activeWindow := a.ActiveWindow; activeWindow != nil {
		seen := map[int]bool{}

		valid := len(activeWindow.Weekdays) >= 1 && len(activeWindow.Weekdays) <= 7
		for _, d := range activeWindow.Weekdays {
			if d < 1 || d > 7 || seen[d] {
				valid = false
			}

			seen[d] = true
		}

		if !valid {
			fieldErrs[prefix+"activeWindow.weekdays"] = apperr.KeyInvalid
		}

		if !hhmm.MatchString(activeWindow.StartLocal) || !hhmm.MatchString(activeWindow.EndLocal) ||
			activeWindow.StartLocal == activeWindow.EndLocal {
			fieldErrs[prefix+"activeWindow"] = "error.window"
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

// Policy is Policy of service-contracts.ts (the fields of the three kinds; MarshalJSON emits the kind's shape, the json
// tags name the wire fields for the API description).
type Policy struct {
	ID                uuid.UUID   `json:"id"`
	TenantID          uuid.UUID   `json:"tenantId"`
	Version           int         `json:"version"`
	CreatedAt         time.Time   `json:"createdAt"`
	UpdatedAt         time.Time   `json:"updatedAt"`
	Kind              string      `json:"kind"`
	Name              string      `json:"name"`
	UnitIDs           []uuid.UUID `json:"unitIds"`
	OwnerMembershipID uuid.UUID   `json:"ownerMembershipId"`
	CreatedByUserID   uuid.UUID   `json:"createdByUserId"`
	Timezone          string      `json:"timezone"`
	Enabled           bool        `json:"enabled"`
	Priority          int         `json:"priority"`
	DisabledReason    *string     `json:"disabledReason"`
	// alert
	CustomerID             *uuid.UUID     `json:"customerId"`
	Condition              AlertCondition `json:"-"` // flattened into metric, operator, threshold, … by MarshalJSON
	RecipientMembershipIDs []uuid.UUID    `json:"recipientMembershipIds"`
	Channels               []string       `json:"channels"`
	EscalateAfterMinutes   int            `json:"escalateAfterMinutes"`
	CooldownMinutes        int            `json:"cooldownMinutes"`
	// default_alert
	Rules        []DefaultAlertRule `json:"rules"`
	RuleSettings []RuleSetting      `json:"ruleSettings"`
	// automation
	AutoCondition json.RawMessage `json:"condition" swaggertype:"object"`
	Action        json.RawMessage `json:"action"    swaggertype:"object"`
}

// MarshalJSON implements json.Marshaler.
func (p Policy) MarshalJSON() ([]byte, error) {
	fields := map[string]any{
		"id":                p.ID,
		"tenantId":          p.TenantID,
		"version":           p.Version,
		"createdAt":         p.CreatedAt,
		"updatedAt":         p.UpdatedAt,
		"kind":              p.Kind,
		"name":              p.Name,
		"unitIds":           p.UnitIDs,
		"ownerMembershipId": p.OwnerMembershipID,
		"createdByUserId":   p.CreatedByUserID,
		"timezone":          p.Timezone,
		"enabled":           p.Enabled,
		"priority":          p.Priority,
		"disabledReason":    p.DisabledReason,
	}
	switch p.Kind {
	case "alert":
		fields["customerId"], fields["recipientMembershipIds"], fields["channels"] = p.CustomerID, p.RecipientMembershipIDs, p.Channels
		fields["escalateAfterMinutes"], fields["cooldownMinutes"] = p.EscalateAfterMinutes, p.CooldownMinutes
		fields["metric"], fields["operator"] = p.Condition.Metric, p.Condition.Operator
		fields["threshold"], fields["recoveryThreshold"] = p.Condition.Threshold, p.Condition.RecoveryThreshold
		fields["durationSeconds"], fields["activeWindow"] = p.Condition.DurationSeconds, p.Condition.ActiveWindow
		fields["severity"] = p.Condition.Severity
	case "default_alert":
		fields["customerId"], fields["rules"], fields["ruleSettings"] = nil, p.Rules, p.RuleSettings
	case "automation":
		fields["condition"], fields["action"] = p.AutoCondition, p.Action
	}

	out, err := json.Marshal(fields)

	return out, apperr.Wrap(err, "json.Marshal")
}

const alertPolicyCols = `p.id, p.tenant_id, p.version, p.created_at, p.updated_at, p.kind, p.customer_id, p.name, ` +
	`p.owner_membership_id, p.created_by_user_id,
	p.timezone, p.enabled, p.priority, p.condition, p.rules, p.recipient_membership_ids, p.channels, ` +
	`COALESCE(p.escalate_after_minutes, 0), COALESCE(p.cooldown_minutes, 0)`

func scanAlertPolicy(row pgx.Row) (Policy, error) {
	var (
		policy      Policy
		cond, rules []byte
	)

	err := row.Scan(
		&policy.ID,
		&policy.TenantID,
		&policy.Version,
		&policy.CreatedAt,
		&policy.UpdatedAt,
		&policy.Kind,
		&policy.CustomerID,
		&policy.Name,
		&policy.OwnerMembershipID,
		&policy.CreatedByUserID,
		&policy.Timezone,
		&policy.Enabled,
		&policy.Priority,
		&cond,
		&rules,
		&policy.RecipientMembershipIDs,
		&policy.Channels,
		&policy.EscalateAfterMinutes,
		&policy.CooldownMinutes,
	)
	if err != nil {
		return policy, apperr.Wrap(err, "scan (scanAlertPolicy)")
	}

	policy.UnitIDs, policy.RuleSettings = []uuid.UUID{}, []RuleSetting{}
	if len(cond) > 0 {
		err := json.Unmarshal(cond, &policy.Condition)
		if err != nil {
			return policy, apperr.Wrap(err, "json.Unmarshal")
		}
	}

	if len(rules) > 0 {
		err := json.Unmarshal(rules, &policy.Rules)
		if err != nil {
			return policy, apperr.Wrap(err, "json.Unmarshal")
		}
	}

	return policy, nil
}

const autoPolicyCols = `p.id, p.tenant_id, p.version, p.created_at, p.updated_at, p.name, p.owner_membership_id, ` +
	`p.created_by_user_id, p.timezone, p.enabled,
	p.priority, p.disabled_reason, p.condition, p.action,
	COALESCE((SELECT array_agg(u.unit_id ORDER BY u.unit_id) FROM control.automation_policy_units u WHERE u.policy_id = ` +
	`p.id), '{}')`

func scanAutoPolicy(row pgx.Row) (Policy, error) {
	policy := Policy{
		Kind:              "automation",
		ID:                uuid.UUID{},
		TenantID:          uuid.UUID{},
		Version:           0,
		CreatedAt:         time.Time{},
		UpdatedAt:         time.Time{},
		Name:              "",
		UnitIDs:           nil,
		OwnerMembershipID: uuid.UUID{},
		CreatedByUserID:   uuid.UUID{},
		Timezone:          "",
		Enabled:           false,
		Priority:          0,
		DisabledReason:    nil,
		CustomerID:        nil,
		Condition: AlertCondition{
			Metric:            "",
			Operator:          "",
			Threshold:         0,
			RecoveryThreshold: 0,
			DurationSeconds:   0,
			ActiveWindow:      nil,
			Severity:          "",
		},
		RecipientMembershipIDs: nil,
		Channels:               nil,
		EscalateAfterMinutes:   0,
		CooldownMinutes:        0,
		Rules:                  nil,
		RuleSettings:           nil,
		AutoCondition:          nil,
		Action:                 nil,
	}
	err := row.Scan(
		&policy.ID,
		&policy.TenantID,
		&policy.Version,
		&policy.CreatedAt,
		&policy.UpdatedAt,
		&policy.Name,
		&policy.OwnerMembershipID,
		&policy.CreatedByUserID,
		&policy.Timezone,
		&policy.Enabled,
		&policy.Priority,
		&policy.DisabledReason,
		&policy.AutoCondition,
		&policy.Action,
		&policy.UnitIDs,
	)

	return policy, apperr.Wrap(err, "scan (scanAutoPolicy)")
}

// ---- scope ----

// grantedKinds returns the policy kinds the caller's granted alternatives allow.
func grantedKinds(c *ops.Call) map[string]bool {
	out := map[string]bool{}

	for _, a := range c.Candidates {
		for _, pr := range a.Predicates {
			if v, ok := strings.CutPrefix(pr, "kind="); ok {
				for k := range strings.SplitSeq(v, "-or-") {
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
		return uuid.Nil, apperr.Wrap(err, "monitoring.PolicyUnits.CustomerOfOrg")
	}

	return id, nil
}

// decorate fills derived fields: attached units of alert policies and the visible rule settings of the default policy.
func (m Policies) decorate(
	ctx context.Context,
	call *ops.Call,
	policies []Policy,
	settingsFor *uuid.UUID,
) error {
	var alertIDs []uuid.UUID

	for _, p := range policies {
		if p.Kind == "alert" {
			alertIDs = append(alertIDs, p.ID)
		}
	}

	if len(alertIDs) > 0 {
		att, err := m.Units.AttachedUnits(ctx, call, alertIDs)
		if err != nil {
			return apperr.Wrap(err, "monitoring.PolicyUnits.AttachedUnits")
		}

		for i := range policies {
			if u, ok := att[policies[i].ID]; ok {
				policies[i].UnitIDs = u
			}
		}
	}

	for idx := range policies {
		if policies[idx].Kind != "default_alert" {
			continue
		}

		stmt := `SELECT id, tenant_id, version, created_at, updated_at, policy_id, rule_key, customer_id, enabled,
			changed_by_membership_id, reason
			FROM monitoring.default_rule_settings WHERE policy_id = $1`
		args := []any{policies[idx].ID}

		if settingsFor != nil {
			stmt += " AND customer_id = $2"

			args = append(args, *settingsFor)
		}

		rows, err := call.Tx.Query(ctx, stmt+" ORDER BY customer_id, rule_key", args...)
		if err != nil {
			return apperr.Wrap(err, "query (decorate)")
		}

		for rows.Next() {
			var ruleSetting RuleSetting

			err := rows.Scan(
				&ruleSetting.ID,
				&ruleSetting.TenantID,
				&ruleSetting.Version,
				&ruleSetting.CreatedAt,
				&ruleSetting.UpdatedAt,
				&ruleSetting.PolicyID,
				&ruleSetting.RuleKey,
				&ruleSetting.CustomerID,
				&ruleSetting.Enabled,
				&ruleSetting.ChangedByMembershipID,
				&ruleSetting.Reason,
			)
			if err != nil {
				rows.Close()

				return apperr.Wrap(err, "query (decorate)")
			}

			policies[idx].RuleSettings = append(policies[idx].RuleSettings, ruleSetting)
		}

		rows.Close()

		rowsErr := rows.Err()
		if rowsErr != nil {
			return apperr.Wrap(rowsErr, "query (decorate)")
		}
	}

	return nil
}

// ---- policies.list ----

var kindRank = map[string]int{"default_alert": 0, "alert": 1, "automation": 2}

// list serves policies.list.
//
//	@Summary		policies.list (read)
//	@ID				policies.list
//	@Description	Authorization: client:self-customer:kind=alert-or-default_alert | \
//	@Description	admin:alert.policy.read:kind=alert-or-default_alert | admin:automation.policy.read:kind=automation
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A05, DD-A11, DD-A12, DD-C15, DD-C03, DD-A02 · Query: filters \
//	@Description	unitId,customerId,propertyId,kind,enabled · sort id,name,priority,createdAt,updatedAt (default kind \
//	@Description	(default_alert, alert, automation) asc;name asc;id asc)
//	@Tags			policies
//	@Accept			json
//	@Produce		json
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,name,priority,createdAt,updatedAt; default kind (default_alert, alert, automation) asc;name asc;id asc"
//	@Param			unitId		query		string	false	"filter → alert kind: id in ACUnit.alertPolicyIds of that unit plus the default policy"
//	@Param			customerId	query		string	false	"filter → alert kind: Policy.customerId (default policy always included)"
//	@Param			propertyId	query		string	false	"filter → attached or target units in the property"
//	@Param			kind		query		string	false	"filter → kind"
//	@Param			enabled		query		boolean	false	"filter → enabled"
//	@Success		200			{object}	ops.Envelope{data=PolicyPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/policies [get]
func (m Policies) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Policy], error) {
	var filters struct {
		Kind       *string    `json:"kind,omitempty"`
		CustomerID *uuid.UUID `json:"customerId,omitempty"`
		PropertyID *uuid.UUID `json:"propertyId,omitempty"`
		UnitID     *uuid.UUID `json:"unitId,omitempty"`
		Enabled    *bool      `json:"enabled,omitempty"`
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filters) != nil {
			return paging.Page[Policy]{}, apperr.Fields(
				map[string]string{"filters": apperr.KeyInvalid},
			)
		}
	}

	if filters.Kind != nil && kindRank[*filters.Kind] == 0 && *filters.Kind != "default_alert" {
		return paging.Page[Policy]{}, apperr.Fields(
			map[string]string{"filters.kind": apperr.KeyInvalid},
		)
	}

	var less func(a, b Policy) int

	if query.Sort != nil {
		dir := 1
		if query.Sort.Direction == "desc" {
			dir = -1
		}

		switch query.Sort.Field {
		case "id":
			less = func(a, b Policy) int { return dir * strings.Compare(a.ID.String(), b.ID.String()) }
		case "createdAt":
			less = func(a, b Policy) int { return dir * a.CreatedAt.Compare(b.CreatedAt) }
		case "name":
			less = func(a, b Policy) int { return dir * strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name)) }
		case "priority":
			less = func(a, b Policy) int { return dir * (a.Priority - b.Priority) }
		case "updatedAt":
			less = func(a, b Policy) int { return dir * a.UpdatedAt.Compare(b.UpdatedAt) }
		default:
			return paging.Page[Policy]{}, apperr.Fields(
				map[string]string{"sort.field": apperr.KeyInvalid},
			)
		}
	} else {
		less = func(a, b Policy) int {
			if d := kindRank[a.Kind] - kindRank[b.Kind]; d != 0 {
				return d
			}

			return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
		}
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Policy]{}, apperr.Wrap(err, "paging.Resolve")
	}

	kinds := grantedKinds(call)
	if filters.Kind != nil {
		kinds = map[string]bool{*filters.Kind: kinds[*filters.Kind]}
	}

	own, err := m.clientCustomer(ctx, call)
	if err != nil {
		return paging.Page[Policy]{}, err
	}
	// unit filters: the units in question (nil = no unit filter)
	var units []uuid.UUID
	if filters.UnitID != nil {
		units = []uuid.UUID{*filters.UnitID}
	}

	if filters.PropertyID != nil {
		propertyUnits, err := m.Units.UnitsOfProperty(ctx, call, *filters.PropertyID)
		if err != nil {
			return paging.Page[Policy]{}, apperr.Wrap(err, "monitoring.PolicyUnits.UnitsOfProperty")
		}

		if units == nil {
			units = propertyUnits
		} else {
			units = intersect(units, propertyUnits)
		}
	}

	var all []Policy

	if kinds["alert"] || kinds["default_alert"] {
		var args []any

		add := func(v any) string {
			args = append(args, v)

			return fmt.Sprintf("$%d", len(args))
		}

		var alertKinds []string

		for _, k := range []string{"alert", "default_alert"} {
			if kinds[k] && (k != "default_alert" || units == nil) {
				alertKinds = append(alertKinds, k)
			}
		}

		conds := []string{"p.kind = ANY(" + add(alertKinds) + ")"}
		if own != uuid.Nil || call.Principal.Role == "client" {
			conds = append(conds, "(p.kind = 'default_alert' OR p.customer_id = "+add(own)+")")
		}

		if filters.CustomerID != nil {
			conds = append(
				conds,
				"(p.kind = 'default_alert' OR p.customer_id = "+add(*filters.CustomerID)+")",
			)
		}

		if filters.Enabled != nil {
			conds = append(conds, "p.enabled = "+add(*filters.Enabled))
		}

		if units != nil {
			ids, err := m.Units.PoliciesOfUnits(ctx, call, units)
			if err != nil {
				return paging.Page[Policy]{}, apperr.Wrap(
					err,
					"monitoring.PolicyUnits.PoliciesOfUnits",
				)
			}

			conds = append(conds, "p.id = ANY("+add(ids)+")")
		}

		rows, err := call.Tx.Query(
			ctx,
			"SELECT "+alertPolicyCols+" FROM monitoring.alert_policies p WHERE "+strings.Join(
				conds,
				" AND ",
			),
			args...)
		if err != nil {
			return paging.Page[Policy]{}, apperr.Wrap(err, "select monitoring.alert_policies")
		}

		for rows.Next() {
			policy, err := scanAlertPolicy(rows)
			if err != nil {
				rows.Close()

				return paging.Page[Policy]{}, err
			}

			all = append(all, policy)
		}

		rows.Close()

		rowsErr := rows.Err()
		if rowsErr != nil {
			return paging.Page[Policy]{}, apperr.Wrap(rowsErr, "select monitoring.alert_policies")
		}
	}

	if kinds["automation"] {
		autos, err := m.automationPolicies(ctx, call, filters.Enabled)
		if err != nil {
			return paging.Page[Policy]{}, err
		}

		var custOrg uuid.UUID

		if filters.CustomerID != nil {
			org, _, found, err := m.Units.CustomerState(ctx, call, *filters.CustomerID)
			if err != nil {
				return paging.Page[Policy]{}, apperr.Wrap(
					err,
					"monitoring.PolicyUnits.CustomerState",
				)
			}

			if found {
				custOrg = org
			}
		}

		for _, policy := range autos {
			if units != nil && len(intersect(policy.UnitIDs, units)) == 0 {
				continue
			}

			if filters.CustomerID != nil {
				orgs, err := m.Units.OrgsOfUnits(ctx, call, policy.UnitIDs)
				if err != nil {
					return paging.Page[Policy]{}, apperr.Wrap(
						err,
						"monitoring.PolicyUnits.OrgsOfUnits",
					)
				}

				match := false
				for _, o := range orgs {
					match = match || o == custOrg
				}

				if !match {
					continue
				}
			}

			all = append(all, policy)
		}
	}

	sort.SliceStable(all, func(i, j int) bool {
		if d := less(all[i], all[j]); d != 0 {
			return d < 0
		}

		return all[i].ID.String() < all[j].ID.String()
	})
	total := len(all)
	end := min(window.Offset+window.Limit, total)

	page := []Policy{}
	if window.Offset < total {
		page = append(page, all[window.Offset:end]...)
	}

	settingsFor := filters.CustomerID
	if call.Principal.Role == "client" {
		settingsFor = &own
	}

	decorateErr := m.decorate(ctx, call, page, settingsFor)
	if decorateErr != nil {
		return paging.Page[Policy]{}, decorateErr
	}

	return paging.Page[Policy]{
		Items:           page,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

func intersect(ids, others []uuid.UUID) []uuid.UUID {
	input := map[uuid.UUID]bool{}
	for _, x := range others {
		input[x] = true
	}

	out := []uuid.UUID{}

	for _, x := range ids {
		if input[x] {
			out = append(out, x)
		}
	}

	return out
}

func (m Policies) automationPolicies(
	ctx context.Context,
	call *ops.Call,
	enabled *bool,
) ([]Policy, error) {
	q, args := "SELECT "+autoPolicyCols+" FROM control.automation_policies p", []any{}
	if enabled != nil {
		q, args = q+" WHERE p.enabled = $1", append(args, *enabled)
	}

	rows, err := call.Tx.Query(ctx, q, args...)
	if err != nil {
		return nil, apperr.Wrap(err, "select control.automation_policies")
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

	return out, apperr.Wrap(rows.Err(), "select control.automation_policies")
}

// ---- policies.get ----

// PolicyIDInput is the {policyId} input of policies.get / policies.delete.
type PolicyIDInput struct {
	PolicyID uuid.UUID `json:"policyId"`
}

// Validate implements ops.Validator.
func (in *PolicyIDInput) Validate() map[string]string {
	if in.PolicyID == uuid.Nil {
		return map[string]string{"policyId": apperr.KeyRequired}
	}

	return nil
}

// load reads one policy in the caller's scope (kind granted, client: own customer or default); lock adds FOR UPDATE.
func (m Policies) load(ctx context.Context, c *ops.Call, id uuid.UUID, lock bool) (Policy, error) {
	return m.loadKinds(ctx, c, id, lock, grantedKinds(c))
}

func (m Policies) loadKinds(
	ctx context.Context,
	call *ops.Call,
	policyID uuid.UUID,
	lock bool,
	kinds map[string]bool,
) (Policy, error) {
	suffix := ""
	if lock {
		suffix = " FOR UPDATE"
	}

	policy, err := scanAlertPolicy(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+alertPolicyCols+" FROM monitoring.alert_policies p WHERE p.id = $1"+suffix,
			policyID,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		policy, err = scanAutoPolicy(
			call.Tx.QueryRow(
				ctx,
				"SELECT "+autoPolicyCols+" FROM control.automation_policies p WHERE p.id = $1"+suffix,
				policyID,
			),
		)
	}

	if errors.Is(err, pgx.ErrNoRows) {
		return policy, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return policy, err
	}

	if !kinds[policy.Kind] {
		return policy, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if call.Principal.Role == "client" && policy.Kind == "alert" {
		own, err := m.clientCustomer(ctx, call)
		if err != nil {
			return policy, err
		}

		if policy.CustomerID == nil || *policy.CustomerID != own {
			return policy, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}
	}

	return policy, nil
}

// get serves policies.get.
//
//	@Summary		policies.get (read)
//	@ID				policies.get
//	@Description	Authorization: client:self-customer:kind=alert-or-default_alert | \
//	@Description	admin:alert.policy.read:kind=alert-or-default_alert | admin:automation.policy.read:kind=automation
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A05, DD-A11, DD-A12, DD-C15
//	@Tags			policies
//	@Accept			json
//	@Produce		json
//	@Param			policyId	path		string	true	"input field policyId"
//	@Success		200			{object}	ops.Envelope{data=Policy}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/policies/{policyId} [get]
func (m Policies) get(ctx context.Context, call *ops.Call, in *PolicyIDInput) (Policy, error) {
	policy, err := m.load(ctx, call, in.PolicyID, false)
	if err != nil {
		return policy, err
	}

	var settingsFor *uuid.UUID

	if call.Principal.Role == "client" {
		own, err := m.clientCustomer(ctx, call)
		if err != nil {
			return policy, err
		}

		settingsFor = &own
	}

	policies := []Policy{policy}

	decorateErr := m.decorate(ctx, call, policies, settingsFor)
	if decorateErr != nil {
		return policy, decorateErr
	}

	return policies[0], nil
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
	Condition json.RawMessage `json:"condition,omitempty" swaggertype:"object"`
	Action    json.RawMessage `json:"action,omitempty"    swaggertype:"object"`
}

// Validate implements ops.Validator for the fields that need no lookup.
func (in *SaveInput) Validate() map[string]string {
	fieldErrs := map[string]string{}

	switch in.Kind {
	case "default_alert":
		if in.ID == nil {
			fieldErrs["id"] = apperr.KeyRequired
		}

		if len(in.Rules) != len(DefaultRules) {
			fieldErrs["rules"] = apperr.KeyFixedRules
		}

		for idx, defaultAlertRule := range in.Rules {
			if idx < len(DefaultRules) &&
				(defaultAlertRule.RuleKey != DefaultRules[idx].RuleKey || defaultAlertRule.Name != DefaultRules[idx].Name ||
					defaultAlertRule.Category != DefaultRules[idx].Category) {
				fieldErrs["rules"] = apperr.KeyFixedRules
			}

			if idx < len(DefaultRules) {
				metrics := map[string]bool{DefaultRules[idx].Metric: true}
				defaultAlertRule.validate(metrics, fmt.Sprintf("rules.%d.", idx), fieldErrs)
			}
		}

		return fieldErrs
	case "alert", "automation":
	default:
		fieldErrs["kind"] = apperr.KeyInvalid

		return fieldErrs
	}

	if in.Name == nil {
		fieldErrs["name"] = apperr.KeyRequired
	} else {
		*in.Name = strings.TrimSpace(*in.Name)
		if n := utf8.RuneCountInString(*in.Name); n < 1 || n > 120 {
			fieldErrs["name"] = apperr.KeyLength
		}
	}

	if in.Timezone == nil {
		fieldErrs["timezone"] = apperr.KeyRequired
	} else {
		_, err := time.LoadLocation(*in.Timezone)
		if err != nil || *in.Timezone == "" || *in.Timezone == "Local" {
			fieldErrs["timezone"] = apperr.KeyInvalid
		}
	}

	if in.Enabled == nil {
		fieldErrs["enabled"] = apperr.KeyRequired
	}

	if in.Priority == nil || *in.Priority < 0 || *in.Priority > 100 {
		fieldErrs["priority"] = apperr.KeyRange
	}

	if in.Kind == "automation" {
		if n := len(in.UnitIDs); n < 1 || n > 500 || hasDup(in.UnitIDs) {
			fieldErrs["unitIds"] = apperr.KeyInvalid
		}

		if !validAutoCondition(in.Condition) {
			fieldErrs["condition"] = apperr.KeyInvalid
		}

		if _, ok := control.ParseAction(in.Action); !ok {
			fieldErrs["action"] = apperr.KeyInvalid
		}

		if in.CustomerID != nil || in.Metric != nil || in.RecipientMembershipIDs != nil ||
			in.Channels != nil ||
			in.Rules != nil {
			fieldErrs["kind"] = "error.fieldsOfOtherKind"
		}
	}

	if in.Kind == "alert" {
		if in.Condition != nil || in.Action != nil {
			fieldErrs["kind"] = "error.fieldsOfOtherKind"
		}

		if in.CustomerID == nil {
			fieldErrs["customerId"] = apperr.KeyRequired
		}

		if in.UnitIDs != nil {
			fieldErrs["unitIds"] = "error.unitsAttachOnUnit"
		}

		if in.Metric == nil || in.Operator == nil || in.Threshold == nil ||
			in.RecoveryThreshold == nil ||
			in.DurationSeconds == nil ||
			in.Severity == nil {
			fieldErrs["condition"] = apperr.KeyRequired
		} else {
			cond := in.condition()
			cond.validate(hqMetrics, "", fieldErrs)
		}

		if n := len(
			in.RecipientMembershipIDs,
		); n < 1 || n > 20 ||
			hasDup(in.RecipientMembershipIDs) {
			fieldErrs["recipientMembershipIds"] = apperr.KeyInvalid
		}

		seen := map[string]bool{}

		valid := len(in.Channels) >= 1 && len(in.Channels) <= 3
		for _, ch := range in.Channels {
			valid = valid && channelSet[ch] && !seen[ch]
			seen[ch] = true
		}

		if !valid || !seen["inApp"] {
			fieldErrs["channels"] = apperr.KeyInvalid
		}

		if in.EscalateAfterMinutes == nil || *in.EscalateAfterMinutes < 1 ||
			*in.EscalateAfterMinutes > 1440 {
			fieldErrs["escalateAfterMinutes"] = apperr.KeyRange
		}

		if in.CooldownMinutes == nil || *in.CooldownMinutes < 1 || *in.CooldownMinutes > 1440 {
			fieldErrs["cooldownMinutes"] = apperr.KeyRange
		}
	}

	return fieldErrs
}

func (in *SaveInput) condition() AlertCondition {
	return AlertCondition{
		Metric:            *in.Metric,
		Operator:          *in.Operator,
		Threshold:         *in.Threshold,
		RecoveryThreshold: *in.RecoveryThreshold,
		DurationSeconds:   *in.DurationSeconds,
		ActiveWindow:      in.ActiveWindow,
		Severity:          *in.Severity,
	}
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

// save serves policies.save.
//
//	@Summary		policies.save (write)
//	@ID				policies.save
//	@Description	Authorization: client:self-customer:kind=alert | \
//	@Description	admin:alert.policy.write:kind=alert-or-default_alert | admin:automation.policy.write:kind=automation
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR120 default_alert limits HQ-only
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A05, DD-A11, DD-A12, DD-C15
//	@Tags			policies
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		SaveInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Policy}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/policies [post]
func (m Policies) save(ctx context.Context, call *ops.Call, input *SaveInput) (Policy, error) {
	if !grantedKinds(call)[input.Kind] {
		return Policy{}, apperr.E(apperr.Forbidden, apperr.KeyForbidden)
	}

	switch input.Kind {
	case "default_alert":
		return m.saveDefault(ctx, call, input)
	case "automation":
		return m.saveAutomation(ctx, call, input)
	}

	return m.saveAlert(ctx, call, input)
}

func (m Policies) saveDefault(
	ctx context.Context,
	call *ops.Call,
	input *SaveInput,
) (Policy, error) {
	policy, err := m.load(ctx, call, *input.ID, true)
	if err != nil {
		return policy, err
	}

	if policy.Kind != "default_alert" {
		return policy, apperr.Fields(map[string]string{"kind": apperr.KeyKindFixed})
	}

	if policy.Version != *call.ExpectedVersion {
		return policy, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	rules, _ := json.Marshal(input.Rules)

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE monitoring.alert_policies SET rules = $2, version = version + 1, updated_at = platform.app_now() WHERE id =
			$1`,
		policy.ID,
		rules,
	)
	if execErr != nil {
		return policy, apperr.Wrap(execErr, "update monitoring.alert_policies")
	}

	return m.finishSave(ctx, call, policy.ID, "PolicyChanged", call.ExpectedVersion)
}

func (m Policies) saveAlert(ctx context.Context, call *ops.Call, input *SaveInput) (Policy, error) {
	cond := input.condition()

	if call.Principal.Role == "client" {
		fe := map[string]string{}
		cond.validate(clientMetrics, "", fe)

		if len(fe) > 0 {
			return Policy{}, apperr.Fields(fe)
		}

		own, err := m.clientCustomer(ctx, call)
		if err != nil {
			return Policy{}, err
		}

		if *input.CustomerID != own {
			return Policy{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}
	}

	org, active, found, err := m.Units.CustomerState(ctx, call, *input.CustomerID)
	if err != nil {
		return Policy{}, apperr.Wrap(err, "monitoring.PolicyUnits.CustomerState")
	}

	if !found {
		return Policy{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if !active {
		return Policy{}, apperr.Fields(map[string]string{"customerId": apperr.KeyInactiveCustomer})
	}

	bad, err := m.Recipients.NonReaders(ctx, call, input.RecipientMembershipIDs, org)
	if err != nil {
		return Policy{}, apperr.Wrap(err, "monitoring.Recipients.NonReaders")
	}

	if len(bad) > 0 {
		return Policy{}, apperr.Fields(
			map[string]string{"recipientMembershipIds": "error.recipientCannotRead"},
		)
	}

	condJSON, _ := json.Marshal(cond)

	if input.ID == nil {
		var policyID uuid.UUID

		err := call.Tx.QueryRow(ctx, `INSERT INTO monitoring.alert_policies (tenant_id, kind, customer_id, name,
			owner_membership_id, created_by_user_id, timezone,
			enabled, priority, condition, recipient_membership_ids, channels, escalate_after_minutes, cooldown_minutes)
			VALUES (current_setting('app.tenant_id')::uuid, 'alert', $1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12) RETURNING
			id`,
			*input.CustomerID, *input.Name, call.Principal.MembershipID, call.Principal.UserID, *input.Timezone, *input.Enabled,
			*input.Priority,
			condJSON,
			input.RecipientMembershipIDs, input.Channels, *input.EscalateAfterMinutes, *input.CooldownMinutes).
			Scan(&policyID)
		if err != nil {
			return Policy{}, apperr.Wrap(err, "insert monitoring.alert_policies")
		}

		return m.finishSave(ctx, call, policyID, "PolicyCreated", nil)
	}

	policy, err := m.load(ctx, call, *input.ID, true)
	if err != nil {
		return policy, err
	}

	if policy.Kind != "alert" {
		return policy, apperr.Fields(map[string]string{"kind": apperr.KeyKindFixed})
	}

	if policy.CustomerID == nil || *policy.CustomerID != *input.CustomerID {
		return policy, apperr.Fields(map[string]string{"customerId": "error.ownerFixed"})
	}

	if policy.Version != *call.ExpectedVersion {
		return policy, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE monitoring.alert_policies SET name = $2, timezone = $3, enabled = $4, priority = $5, condition = $6,
		recipient_membership_ids = $7, channels = $8, escalate_after_minutes = $9, cooldown_minutes = $10, version = version +
		1, updated_at = platform.app_now()
		WHERE id = $1`,
		policy.ID,
		*input.Name,
		*input.Timezone,
		*input.Enabled,
		*input.Priority,
		condJSON,
		input.RecipientMembershipIDs,
		input.Channels,
		*input.EscalateAfterMinutes,
		*input.CooldownMinutes,
	)
	if execErr != nil {
		return policy, apperr.Wrap(execErr, "update monitoring.alert_policies")
	}

	return m.finishSave(ctx, call, policy.ID, "PolicyChanged", call.ExpectedVersion)
}

// UnitCaps returns the action capabilities of an active unit (found=false for unknown or archived units).
type UnitCaps interface {
	UnitCaps(ctx context.Context, c *ops.Call, unit uuid.UUID) (control.Caps, bool, error)
}

var autoConditionTypes = map[string]bool{
	"occupancy": true,
	"tariff":    true,
	"peak":      true,
	"solar":     true,
	"battery":   true,
}

// validAutoCondition checks an automation Condition of DD-A11 (occupancy, tariff, peak, solar, battery).
func validAutoCondition(raw json.RawMessage) bool {
	var condition struct {
		Type     string   `json:"type"`
		Occupied *bool    `json:"occupied,omitempty"`
		Active   *bool    `json:"active,omitempty"`
		Operator *string  `json:"operator,omitempty"`
		Value    *float64 `json:"value,omitempty"`
		Unit     *string  `json:"unit,omitempty"`
	}

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	if len(raw) == 0 || dec.Decode(&condition) != nil || !autoConditionTypes[condition.Type] {
		return false
	}

	compare := func(unit string) bool {
		return condition.Operator != nil && operators[*condition.Operator] &&
			condition.Value != nil &&
			!math.IsNaN(*condition.Value) &&
			!math.IsInf(*condition.Value, 0) &&
			condition.Unit != nil &&
			*condition.Unit == unit &&
			condition.Occupied == nil &&
			condition.Active == nil
	}

	switch condition.Type {
	case "occupancy":
		return condition.Occupied != nil && condition.Active == nil && condition.Operator == nil &&
			condition.Value == nil &&
			condition.Unit == nil
	case "peak":
		return condition.Active != nil && condition.Occupied == nil && condition.Operator == nil &&
			condition.Value == nil &&
			condition.Unit == nil
	case "tariff":
		return compare("MYR_per_kWh") && *condition.Value >= 0
	default: // solar, battery
		return compare("kW") && *condition.Value >= 0
	}
}

// saveAutomation stores an HQ automation policy (DD-A11, IR120 item 9).
func (m Policies) saveAutomation(
	ctx context.Context,
	call *ops.Call,
	input *SaveInput,
) (Policy, error) {
	action, _ := control.ParseAction(input.Action)
	for _, u := range input.UnitIDs {
		caps, found, err := m.Caps.UnitCaps(ctx, call, u)
		if err != nil {
			return Policy{}, apperr.Wrap(err, "monitoring.UnitCaps.UnitCaps")
		}

		if !found {
			return Policy{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}

		if !action.Supports(caps) {
			return Policy{}, apperr.Fields(map[string]string{"action": apperr.KeyUnsupportedAction})
		}
	}

	policyID := uuid.Nil

	var prev *int

	if input.ID == nil {
		err := call.Tx.QueryRow(ctx, `INSERT INTO control.automation_policies (tenant_id, name, owner_membership_id,
			created_by_user_id, timezone, enabled, priority, condition, action)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, $7, $8) RETURNING id`,
			*input.Name, call.Principal.MembershipID, call.Principal.UserID, *input.Timezone, *input.Enabled, *input.Priority,
			[]byte(input.Condition), []byte(input.Action)).
			Scan(&policyID)
		if err != nil {
			return Policy{}, apperr.Wrap(err, "insert control.automation_policies")
		}
	} else {
		policy, err := m.load(ctx, call, *input.ID, true)
		if err != nil {
			return policy, err
		}

		if policy.Kind != "automation" {
			return policy, apperr.Fields(map[string]string{"kind": apperr.KeyKindFixed})
		}

		if policy.Version != *call.ExpectedVersion {
			return policy, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
		}

		_, execErr := call.Tx.Exec(
			ctx,
			`UPDATE control.automation_policies SET name = $2, timezone = $3, enabled = $4, priority = $5, condition = $6, action
				= $7,
			disabled_reason = NULL, version = version + 1, updated_at = platform.app_now() WHERE id = $1`,
			policy.ID,
			*input.Name,
			*input.Timezone,
			*input.Enabled,
			*input.Priority,
			[]byte(input.Condition),
			[]byte(input.Action),
		)
		if execErr != nil {
			return policy, apperr.Wrap(execErr, "update control.automation_policies")
		}

		_, execErr = call.Tx.Exec(
			ctx,
			`DELETE FROM control.automation_policy_units WHERE policy_id = $1`,
			policy.ID,
		)
		if execErr != nil {
			return policy, apperr.Wrap(execErr, "delete control.automation_policy_units")
		}

		policyID, prev = policy.ID, call.ExpectedVersion
	}

	for _, unitID := range input.UnitIDs {
		_, err := call.Tx.Exec(
			ctx,
			`INSERT INTO control.automation_policy_units (tenant_id, policy_id, unit_id) VALUES
				(current_setting('app.tenant_id')::uuid, $1, $2)`,
			policyID,
			unitID,
		)
		if err != nil {
			return Policy{}, apperr.Wrap(err, "insert control.automation_policy_units")
		}
	}

	event := "PolicyCreated"
	if prev != nil {
		event = "PolicyChanged"
	}

	return m.finishSave(ctx, call, policyID, event, prev)
}

func (m Policies) finishSave(
	ctx context.Context,
	call *ops.Call,
	id uuid.UUID,
	event string,
	prev *int,
) (Policy, error) {
	policy, err := m.get(ctx, call, &PolicyIDInput{PolicyID: id})
	if err != nil {
		return policy, err
	}

	call.Emit(
		ops.Event{
			AggregateType: "policy",
			AggregateID:   policy.ID,
			Type:          event,
			Payload:       map[string]any{"kind": policy.Kind},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "policies.save",
			TargetKind:      "policy",
			TargetID:        policy.ID.String(),
			PreviousVersion: prev,
			NextVersion:     &policy.Version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return policy, nil
}

// ---- policies.delete ----

// Deleted is DeletedResource of service-contracts.ts.
type Deleted struct {
	ID      uuid.UUID `json:"id"`
	Deleted bool      `json:"deleted"`
}

// delete serves policies.delete.
//
//	@Summary		policies.delete (write)
//	@ID				policies.delete
//	@Description	Authorization: client:self-customer:kind=alert | admin:alert.policy.write:kind=alert | \
//	@Description	admin:automation.policy.write:kind=automation
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR108 detaches the policy from every unit first; default_alert cannot be deleted (VALIDATION)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C15, DD-A05, DD-A11
//	@Tags			policies
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target policies, read policies.get)"
//	@Param			policyId			path		string	true	"input field policyId"
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
//	@Router			/v1/policies/{policyId} [delete]
func (m Policies) delete(ctx context.Context, call *ops.Call, in *PolicyIDInput) (Deleted, error) {
	kinds := grantedKinds(call)
	// the default policy is visible to alert writers so its delete is VALIDATION (IR108)
	kinds["default_alert"] = kinds["alert"]

	policy, err := m.loadKinds(ctx, call, in.PolicyID, true, kinds)
	if err != nil {
		return Deleted{}, err
	}

	if policy.Kind == "default_alert" {
		return Deleted{}, apperr.Fields(map[string]string{"policyId": "error.defaultPolicyFixed"})
	}

	if policy.Version != *call.ExpectedVersion {
		return Deleted{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	switch policy.Kind {
	case "alert":
		err := m.Units.DetachPolicy(ctx, call, policy.ID)
		if err != nil {
			return Deleted{}, apperr.Wrap(err, "monitoring.PolicyUnits.DetachPolicy")
		}

		_, err = call.Tx.Exec(
			ctx,
			`DELETE FROM monitoring.alert_policies WHERE id = $1`,
			policy.ID,
		)
		if err != nil {
			return Deleted{}, apperr.Wrap(err, "delete monitoring.alert_policies")
		}
	case "automation":
		_, err := call.Tx.Exec(
			ctx,
			`DELETE FROM control.automation_policies WHERE id = $1`,
			policy.ID,
		)
		if err != nil {
			return Deleted{}, apperr.Wrap(err, "delete control.automation_policies")
		}
	}

	call.Emit(
		ops.Event{
			AggregateType: "policy",
			AggregateID:   policy.ID,
			Type:          "PolicyDeleted",
			Payload:       map[string]any{"kind": policy.Kind},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "policies.delete",
			TargetKind:      "policy",
			TargetID:        policy.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     nil,
			Reason:          "",
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)

	return Deleted{ID: policy.ID, Deleted: true}, nil
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
	fieldErrs := map[string]string{}
	if in.PolicyID == uuid.Nil {
		fieldErrs["policyId"] = apperr.KeyRequired
	}

	if in.CustomerID == uuid.Nil {
		fieldErrs["customerId"] = apperr.KeyRequired
	}

	if in.Enabled == nil {
		fieldErrs["enabled"] = apperr.KeyRequired
	}

	known := false
	for _, r := range DefaultRules {
		known = known || r.RuleKey == in.RuleKey
	}

	if !known {
		fieldErrs["ruleKey"] = apperr.KeyInvalid
	}

	if in.Reason != nil {
		*in.Reason = strings.TrimSpace(*in.Reason)
		if utf8.RuneCountInString(*in.Reason) > 500 {
			fieldErrs["reason"] = apperr.KeyLength
		}
	}

	return fieldErrs
}

// setDefaultRule serves policies.setDefaultRule.
//
//	@Summary		policies.setDefaultRule (write)
//	@ID				policies.setDefaultRule
//	@Description	Authorization: client:self-customer:owner | admin:alert.policy.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR108 toggles one default rule for one customer; limits stay HQ-only
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C15, DD-A05
//	@Tags			policies
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer				true	"all: required (target policies, read policies.get)"
//	@Param			policyId			path		string				true	"input field policyId"
//	@Param			ruleKey				path		string				true	"input field ruleKey"
//	@Param			request				body		SetDefaultRuleInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=RuleSetting}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/policies/{policyId}/default-rules/{ruleKey} [put]
func (m Policies) setDefaultRule(
	ctx context.Context,
	call *ops.Call,
	input *SetDefaultRuleInput,
) (RuleSetting, error) {
	if call.Principal.Role == "client" {
		own, err := m.clientCustomer(ctx, call)
		if err != nil {
			return RuleSetting{}, err
		}

		if input.CustomerID != own {
			return RuleSetting{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}

		if call.Principal.ClientRole != "owner" {
			return RuleSetting{}, apperr.E(apperr.Forbidden, "error.ownerOnly")
		}
	}

	var kind string

	err := call.Tx.QueryRow(ctx, `SELECT kind FROM monitoring.alert_policies WHERE id = $1`, input.PolicyID).
		Scan(&kind)
	if errors.Is(err, pgx.ErrNoRows) {
		return RuleSetting{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return RuleSetting{}, apperr.Wrap(err, "select monitoring.alert_policies")
	}

	if kind != "default_alert" {
		return RuleSetting{}, apperr.Fields(map[string]string{"policyId": "error.notDefaultPolicy"})
	}

	_, _, found, customerStateErr := m.Units.CustomerState(ctx, call, input.CustomerID)
	if customerStateErr != nil {
		return RuleSetting{}, apperr.Wrap(customerStateErr, "monitoring.PolicyUnits.CustomerState")
	} else if !found {
		return RuleSetting{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	var cur int

	err = call.Tx.QueryRow(ctx, `SELECT version FROM monitoring.default_rule_settings WHERE policy_id = $1 AND rule_key =
		$2
		AND customer_id = $3 FOR UPDATE`,
		input.PolicyID, input.RuleKey, input.CustomerID).
		Scan(&cur)
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return RuleSetting{}, apperr.Wrap(err, "select monitoring.default_rule_settings")
	}

	if cur != *call.ExpectedVersion {
		return RuleSetting{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	var ruleSetting RuleSetting

	err = call.Tx.QueryRow(ctx, `INSERT INTO monitoring.default_rule_settings (tenant_id, policy_id, rule_key, customer_id,
		enabled, changed_by_membership_id, reason)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, NULLIF($6, ''))
		ON CONFLICT (policy_id, rule_key, customer_id) DO UPDATE SET enabled = EXCLUDED.enabled, changed_by_membership_id =
		EXCLUDED.changed_by_membership_id,
			reason = EXCLUDED.reason, version = monitoring.default_rule_settings.version + 1, updated_at = platform.app_now()
		RETURNING id, tenant_id, version, created_at, updated_at, policy_id, rule_key, customer_id, enabled,
		changed_by_membership_id, reason`,
		input.PolicyID, input.RuleKey, input.CustomerID, *input.Enabled, call.Principal.MembershipID, deref(input.Reason)).
		Scan(&ruleSetting.ID, &ruleSetting.TenantID, &ruleSetting.Version, &ruleSetting.CreatedAt, &ruleSetting.UpdatedAt,
			&ruleSetting.PolicyID, &ruleSetting.RuleKey, &ruleSetting.CustomerID, &ruleSetting.Enabled,
			&ruleSetting.ChangedByMembershipID, &ruleSetting.Reason)
	if err != nil {
		return ruleSetting, apperr.Wrap(err, "insert monitoring.default_rule_settings")
	}

	call.Emit(
		ops.Event{
			AggregateType: "policy",
			AggregateID:   input.PolicyID,
			Type:          "DefaultRuleChanged",
			Payload: map[string]any{
				"ruleKey":    input.RuleKey,
				"customerId": input.CustomerID,
				"enabled":    ruleSetting.Enabled,
			},
		},
	)
	call.Audit(ops.AuditEntry{
		Action:          "policies.setDefaultRule",
		TargetKind:      "default_rule_setting",
		TargetID:        ruleSetting.ID.String(),
		PreviousVersion: call.ExpectedVersion,
		NextVersion:     &ruleSetting.Version,
		Reason:          deref(input.Reason), Result: "", Before: nil, After: nil,
	})

	return ruleSetting, nil
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
