package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
	"github.com/pradita/ac-project/service/api/internal/platform/unitscope"
)

// Alert is Alert of service-contracts.ts.
type Alert struct {
	ID               uuid.UUID       `json:"id"`
	TenantID         uuid.UUID       `json:"tenantId"`
	Version          int             `json:"version"`
	CreatedAt        time.Time       `json:"createdAt"`
	UpdatedAt        time.Time       `json:"updatedAt"`
	UnitID           uuid.UUID       `json:"unitId"`
	PolicyID         *uuid.UUID      `json:"policyId"`
	Type             string          `json:"type"`
	Severity         string          `json:"severity"`
	Status           string          `json:"status"`
	CauseCode        string          `json:"causeCode"`
	EvidenceKind     string          `json:"evidenceKind"`
	EvidenceText     string          `json:"evidenceText"`
	ObservedAt       time.Time       `json:"observedAt"`
	EvidenceIDs      []uuid.UUID     `json:"evidenceIds"`
	DetectedAt       time.Time       `json:"detectedAt"`
	AcknowledgedAt   *time.Time      `json:"acknowledgedAt"`
	ResolvedAt       *time.Time      `json:"resolvedAt"`
	ResolutionReason *string         `json:"resolutionReason"`
	PreviousAlertID  *uuid.UUID      `json:"previousAlertId"`
	DeliveryFailures json.RawMessage `json:"deliveryFailures" swaggertype:"object"`
	Rule             *AlertRule      `json:"rule"`
}

// AlertRule is AlertRule of service-contracts.ts: the condition that raised the alert, read with it (IR284).
type AlertRule struct {
	Name              string  `json:"name"`
	Metric            string  `json:"metric"`
	Operator          string  `json:"operator"`
	Threshold         float64 `json:"threshold"`
	RecoveryThreshold float64 `json:"recoveryThreshold"`
	DurationSeconds   int     `json:"durationSeconds"`
}

// alertRule reads the condition of the alert's policy — the policy's own for kind alert, the rule of rule_key for the
// default policy — so everyone who reads the alert sees what raised it and when it recovers, without reading the policy
// itself (policy reads stay with policy permission holders, IR284); NULL for an alert without a policy.
const alertRule = `(SELECT jsonb_build_object('name', x.name, 'metric', x.c->>'metric', 'operator', x.c->>'operator', 'threshold', x.c->'threshold',
		'recoveryThreshold', x.c->'recoveryThreshold', 'durationSeconds', x.c->'durationSeconds')
	FROM (SELECT p.name, p.condition AS c FROM monitoring.alert_policies p WHERE p.id = a.policy_id AND p.kind = 'alert'
		UNION ALL
		SELECT r->>'name', r FROM monitoring.alert_policies p CROSS JOIN LATERAL jsonb_array_elements(p.rules) r
		WHERE p.id = a.policy_id AND p.kind = 'default_alert' AND r->>'ruleKey' = a.rule_key) x LIMIT 1)`

const alertCols = `a.id, a.tenant_id, a.version, a.created_at, a.updated_at, a.unit_id, a.policy_id, a.type, a.severity, a.status, a.cause_code,
	a.evidence_kind, a.evidence_text, a.observed_at, a.evidence_ids, a.detected_at, a.acknowledged_at, a.resolved_at, a.resolution_reason,
	a.previous_alert_id, a.delivery_failures, ` + alertRule

func scanAlert(r pgx.Row) (Alert, error) {
	var a Alert
	err := r.Scan(&a.ID, &a.TenantID, &a.Version, &a.CreatedAt, &a.UpdatedAt, &a.UnitID, &a.PolicyID, &a.Type, &a.Severity, &a.Status, &a.CauseCode,
		&a.EvidenceKind, &a.EvidenceText, &a.ObservedAt, &a.EvidenceIDs, &a.DetectedAt, &a.AcknowledgedAt, &a.ResolvedAt, &a.ResolutionReason,
		&a.PreviousAlertID, &a.DeliveryFailures, &a.Rule)
	return a, err
}

// UnitLocator resolves property membership of units for the propertyId filter (Assets).
type UnitLocator interface {
	UnitsOfProperty(ctx context.Context, c *ops.Call, property uuid.UUID) ([]uuid.UUID, error)
	OrgOfCustomer(ctx context.Context, c *ops.Call, customer uuid.UUID) (uuid.UUID, bool, error)
}

// TechAccess is the IR94 check for jobId-less technician writes (maintenance.Access).
type TechAccess interface {
	TechnicianUnit(ctx context.Context, c *ops.Call, unit uuid.UUID, inScope bool, internal bool) error
}

// Alerts is the alert operation set.
type Alerts struct {
	Units   UnitLocator
	Access  TechAccess
	Devices DeviceRecoveries // the recovery events behind a resolution's evidence (IR327); nil offers none
}

// alertScope applies D01 to alert rows through the unit of the alert (unitscope, IR169). Lists use Equipment mode
// (external technicians only inside the work window); single reads use List mode followed by unitscope.Gate so a
// not-yet-started Assignment answers FORBIDDEN errors.assignment_not_started (IR49(b)).
func alertScope(c *ops.Call, args *[]any, mode unitscope.Mode) string {
	return unitscope.SQL(c, args, "a.unit_id", mode)
}

var severities = map[string]bool{"critical": true, "warning": true, "normal": true}
var alertStatuses = map[string]bool{"open": true, "acknowledged": true, "resolved": true}

const severityRank = `CASE a.severity WHEN 'normal' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END`

// @Summary		alerts.list (read)
// @ID				alerts.list
// @Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | admin:alert.read | admin:alert.policy.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A05, DD-C01, DD-C08, DD-P04, DD-T01, DD-T07, DD-T02 · Query: filters unitId,unitIds,severity,status,from,to,customerId,propertyId · sort id,createdAt,updatedAt,severity (default severity desc;id asc)
// @Tags			alerts
// @Accept			json
// @Produce		json
// @Param			cursor		query		string		false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit		query		integer		false	"page size 1–100, default 25"
// @Param			sort		query		string		false	"field:direction — fields id,createdAt,updatedAt,severity; default severity desc;id asc"
// @Param			unitId		query		string		false	"filter → unitId"
// @Param			unitIds		query		[]string	false	"filter → unitIds (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
// @Param			severity	query		string		false	"filter → severity"
// @Param			status		query		string		false	"filter → status"
// @Param			from		query		string		false	"filter → [from,to) on observedAt"
// @Param			to			query		string		false	"filter → [from,to) on observedAt"
// @Param			customerId	query		string		false	"filter"
// @Param			propertyId	query		string		false	"filter"
// @Success		200			{object}	ops.Envelope{data=AlertPage}
// @Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504			{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/alerts [get]
func (m Alerts) list(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Alert], error) {
	var f struct {
		UnitID     *uuid.UUID   `json:"unitId,omitempty"`
		UnitIDs    *[]uuid.UUID `json:"unitIds,omitempty"`
		Severity   *string      `json:"severity,omitempty"`
		Status     *string      `json:"status,omitempty"`
		From       *time.Time   `json:"from,omitempty"`
		To         *time.Time   `json:"to,omitempty"`
		CustomerID *uuid.UUID   `json:"customerId,omitempty"`
		PropertyID *uuid.UUID   `json:"propertyId,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil {
			return paging.Page[Alert]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	switch {
	case f.Severity != nil && !severities[*f.Severity]:
		return paging.Page[Alert]{}, apperr.Fields(map[string]string{"filters.severity": "error.invalid"})
	case f.Status != nil && !alertStatuses[*f.Status]:
		return paging.Page[Alert]{}, apperr.Fields(map[string]string{"filters.status": "error.invalid"})
	case f.From != nil && f.To != nil && !f.From.Before(*f.To):
		return paging.Page[Alert]{}, apperr.Fields(map[string]string{"filters.to": "error.range"})
	}
	// default sort severity desc (critical → warning → normal), id asc; severity asc = normal, warning, critical
	order := severityRank + " DESC, a.id ASC"
	if in.Sort != nil {
		if in.Sort.Field == "severity" {
			order = severityRank + " " + strings.ToUpper(in.Sort.Direction) + ", a.id ASC"
		} else {
			o, err := paging.OrderBy(in.Sort, map[string]string{"id": "a.id", "createdAt": "a.created_at", "updatedAt": "a.updated_at"}, "")
			if err != nil {
				return paging.Page[Alert]{}, err
			}
			order = o
		}
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Alert]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{alertScope(c, &args, unitscope.Equipment)}
	if f.UnitID != nil {
		conds = append(conds, "a.unit_id = "+add(*f.UnitID))
	}
	if f.UnitIDs != nil {
		conds = append(conds, "a.unit_id = ANY("+add(*f.UnitIDs)+")")
	}
	if f.Severity != nil {
		conds = append(conds, "a.severity = "+add(*f.Severity))
	}
	if f.Status != nil {
		conds = append(conds, "a.status = "+add(*f.Status))
	}
	if f.From != nil {
		conds = append(conds, "a.observed_at >= "+add(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "a.observed_at < "+add(*f.To))
	}
	if f.CustomerID != nil {
		org, ok, err := m.Units.OrgOfCustomer(ctx, c, *f.CustomerID)
		if err != nil {
			return paging.Page[Alert]{}, err
		}
		if !ok {
			org = uuid.Nil
		}
		conds = append(conds, "a.customer_org_id = "+add(org))
	}
	if f.PropertyID != nil {
		units, err := m.Units.UnitsOfProperty(ctx, c, *f.PropertyID)
		if err != nil {
			return paging.Page[Alert]{}, err
		}
		conds = append(conds, "a.unit_id = ANY("+add(units)+")")
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM monitoring.alerts a WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Alert]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM monitoring.alerts a WHERE %s ORDER BY %s LIMIT %d OFFSET %d", alertCols, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Alert]{}, err
	}
	defer rows.Close()
	items := []Alert{}
	for rows.Next() {
		a, err := scanAlert(rows)
		if err != nil {
			return paging.Page[Alert]{}, err
		}
		items = append(items, a)
	}
	return paging.Page[Alert]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, rows.Err()
}

// GetInput is alerts.get input.
type GetInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *GetInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": "error.required"}
	}
	return nil
}

// @Summary		alerts.get (read)
// @ID				alerts.get
// @Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | admin:alert.read | admin:alert.policy.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-T07, DD-A05, DD-T12, DD-P05, DD-P02
// @Tags			alerts
// @Accept			json
// @Produce		json
// @Param			id	path		string	true	"input field id"
// @Success		200	{object}	ops.Envelope{data=Alert}
// @Failure		401	{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403	{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404	{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409	{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422	{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429	{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503	{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504	{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/alerts/{id} [get]
func (m Alerts) get(ctx context.Context, c *ops.Call, in *GetInput) (Alert, error) {
	args := []any{in.ID}
	a, err := scanAlert(c.Tx.QueryRow(ctx, "SELECT "+alertCols+" FROM monitoring.alerts a WHERE a.id = $1 AND "+alertScope(c, &args, unitscope.List), args...))
	if errors.Is(err, pgx.ErrNoRows) {
		return a, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return a, err
	}
	return a, unitscope.Gate(ctx, c, []uuid.UUID{a.UnitID})
}

// lockAlert loads an alert in the caller's scope for update, applies the version and the technician rule (IR94 for
// operations without jobId).
func (m Alerts) lockAlert(ctx context.Context, c *ops.Call, id uuid.UUID) (string, uuid.UUID, error) {
	args := []any{id}
	var status string
	var unit uuid.UUID
	var v int
	err := c.Tx.QueryRow(ctx, "SELECT a.status, a.unit_id, a.version FROM monitoring.alerts a WHERE a.id = $1 AND "+alertScope(c, &args, unitscope.List)+" FOR UPDATE", args...).
		Scan(&status, &unit, &v)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", unit, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return "", unit, err
	}
	if c.Principal.Role == "technician" {
		if err := m.Access.TechnicianUnit(ctx, c, unit, true, c.Principal.Employment == "internal"); err != nil {
			return "", unit, err
		}
	}
	if v != *c.ExpectedVersion {
		return "", unit, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	return status, unit, nil
}

// AckInput is alerts.acknowledge input.
type AckInput struct {
	AlertID uuid.UUID `json:"alertId"`
}

// Validate implements ops.Validator.
func (in *AckInput) Validate() map[string]string {
	if in.AlertID == uuid.Nil {
		return map[string]string{"alertId": "error.required"}
	}
	return nil
}

// @Summary		alerts.acknowledge (write)
// @ID				alerts.acknowledge
// @Description	Authorization: technician:alert.read:assigned | admin:alert.resolve
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR94 technician write table (assignment and work window, jobId required when typed)
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-T07, DD-T12, DD-A05
// @Tags			alerts
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer		true	"all: required (target alerts, read alerts.get)"
// @Param			alertId				path		string		true	"input field alertId"
// @Param			request				body		AckInput	true	"input; the path parameters come from the route"
// @Success		200					{object}	ops.Envelope{data=Alert}
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/alerts/{alertId}/acknowledge [post]
func (m Alerts) acknowledge(ctx context.Context, c *ops.Call, in *AckInput) (Alert, error) {
	status, unit, err := m.lockAlert(ctx, c, in.AlertID)
	if err != nil {
		return Alert{}, err
	}
	if status != "open" {
		return Alert{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	a, err := scanAlert(c.Tx.QueryRow(ctx, `UPDATE monitoring.alerts a SET status = 'acknowledged', acknowledged_at = $2, acknowledged_by = $3,
		version = version + 1, updated_at = platform.app_now() WHERE a.id = $1 RETURNING `+alertCols, in.AlertID, c.Now, c.Principal.MembershipID))
	if err != nil {
		return Alert{}, err
	}
	c.Emit(ops.Event{AggregateType: "alert", AggregateID: a.ID, Type: "AlertAcknowledged", Payload: map[string]any{"unitId": unit}})
	c.Audit(ops.AuditEntry{Action: "alerts.acknowledge", TargetKind: "alert", TargetID: a.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &a.Version})
	return a, nil
}

// ResolveInput is alerts.resolve input.
type ResolveInput struct {
	AlertID               uuid.UUID   `json:"alertId"`
	ResolutionReason      string      `json:"resolutionReason"`
	ResolutionEvidenceIDs []uuid.UUID `json:"resolutionEvidenceIds"`
}

// Validate implements ops.Validator (IR87: 1–1000 after trimming).
func (in *ResolveInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.AlertID == uuid.Nil {
		fe["alertId"] = "error.required"
	}
	in.ResolutionReason = strings.TrimSpace(in.ResolutionReason)
	if n := utf8.RuneCountInString(in.ResolutionReason); n < 1 || n > 1000 {
		fe["resolutionReason"] = "error.length"
	}
	if in.ResolutionEvidenceIDs == nil {
		fe["resolutionEvidenceIds"] = "error.required"
	} else if len(in.ResolutionEvidenceIDs) > 20 || !distinct(in.ResolutionEvidenceIDs) {
		fe["resolutionEvidenceIds"] = "error.invalid" // at most 20, each once
	}
	return fe
}

// @Summary		alerts.resolve (write)
// @ID				alerts.resolve
// @Description	Authorization: technician:alert.resolve:assigned | admin:alert.resolve
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR87 resolutionReason 1-1000; IR94 technician write table (assignment and work window, jobId required when typed); IR327 a policy-free alert needs at least one evidence ID, every ID a candidate of alerts.evidence
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-T07, DD-A05
// @Tags			alerts
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer			true	"all: required (target alerts, read alerts.get)"
// @Param			alertId				path		string			true	"input field alertId"
// @Param			request				body		ResolveInput	true	"input; the path parameters come from the route"
// @Success		200					{object}	ops.Envelope{data=Alert}
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/alerts/{alertId}/resolve [post]
func (m Alerts) resolve(ctx context.Context, c *ops.Call, in *ResolveInput) (Alert, error) {
	status, unit, err := m.lockAlert(ctx, c, in.AlertID)
	if err != nil {
		return Alert{}, err
	}
	if status == "resolved" {
		return Alert{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	cur, err := scanAlert(c.Tx.QueryRow(ctx, "SELECT "+alertCols+" FROM monitoring.alerts a WHERE a.id = $1", in.AlertID))
	if err != nil {
		return Alert{}, err
	}
	if err := m.checkEvidence(ctx, c, cur, in.ResolutionEvidenceIDs); err != nil {
		return Alert{}, err
	}
	a, err := scanAlert(c.Tx.QueryRow(ctx, `UPDATE monitoring.alerts a SET status = 'resolved', resolved_at = $2, resolved_by = $3, resolution_reason = $4,
		resolution_evidence_ids = $5, version = version + 1, updated_at = platform.app_now() WHERE a.id = $1 RETURNING `+alertCols,
		in.AlertID, c.Now, c.Principal.MembershipID, in.ResolutionReason, in.ResolutionEvidenceIDs))
	if err != nil {
		return Alert{}, err
	}
	c.Emit(ops.Event{AggregateType: "alert", AggregateID: a.ID, Type: "AlertResolved", Payload: map[string]any{"unitId": unit}})
	c.Audit(ops.AuditEntry{Action: "alerts.resolve", TargetKind: "alert", TargetID: a.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &a.Version, Reason: in.ResolutionReason})
	return a, nil
}

// ActiveAlertCounts implements assets.Monitoring: open + acknowledged critical/warning alerts per unit (IR51).
func (Alerts) ActiveAlertCounts(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID]int, error) {
	out := map[uuid.UUID]int{}
	rows, err := c.Tx.Query(ctx, `SELECT unit_id, count(*) FROM monitoring.alerts WHERE unit_id = ANY($1) AND status IN ('open','acknowledged')
		AND severity IN ('critical','warning') GROUP BY unit_id`, units)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var u uuid.UUID
		var n int
		if err := rows.Scan(&u, &n); err != nil {
			return nil, err
		}
		out[u] = n
	}
	return out, rows.Err()
}

// RegisterAlerts binds the alert operations.
func RegisterAlerts(r *ops.Registry, m Alerts) {
	ops.Register(r, "alerts.list", m.list)
	ops.Register(r, "alerts.get", m.get)
	ops.Register(r, "alerts.acknowledge", m.acknowledge)
	ops.Register(r, "alerts.resolve", m.resolve)
	ops.Register(r, "alerts.evidence", m.evidence)
}

// UnitSeverities implements maintenance.UnitSeverity (IR23): the highest open/acknowledged alert severity per unit.
func (Alerts) UnitSeverities(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID]string, error) {
	rows, err := c.Tx.Query(ctx, `SELECT a.unit_id, (array_agg(a.severity ORDER BY `+severityRank+` DESC))[1] FROM monitoring.alerts a
		WHERE a.unit_id = ANY($1) AND a.status <> 'resolved' GROUP BY a.unit_id`, units)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]string{}
	for rows.Next() {
		var u uuid.UUID
		var s string
		if err := rows.Scan(&u, &s); err != nil {
			return nil, err
		}
		out[u] = s
	}
	return out, rows.Err()
}
