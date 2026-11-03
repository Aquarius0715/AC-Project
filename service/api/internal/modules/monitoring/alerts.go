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
const alertRule = `(SELECT jsonb_build_object('name', x.name, 'metric', x.c->>'metric', 'operator', x.c->>'operator',` +
	` 'threshold', x.c->'threshold',
		'recoveryThreshold', x.c->'recoveryThreshold', 'durationSeconds', x.c->'durationSeconds')
	FROM (SELECT p.name, p.condition AS c FROM monitoring.alert_policies p WHERE p.id = a.policy_id AND p.kind = 'alert'
		UNION ALL
		SELECT r->>'name', r FROM monitoring.alert_policies p CROSS JOIN LATERAL jsonb_array_elements(p.rules) r
		WHERE p.id = a.policy_id AND p.kind = 'default_alert' AND r->>'ruleKey' = a.rule_key) x LIMIT 1)`

const alertCols = `a.id, a.tenant_id, a.version, a.created_at, a.updated_at, a.unit_id, a.policy_id, a.type, ` +
	`a.severity, a.status, a.cause_code,
	a.evidence_kind, a.evidence_text, a.observed_at, a.evidence_ids, a.detected_at, a.acknowledged_at, a.resolved_at, ` +
	`a.resolution_reason,
	a.previous_alert_id, a.delivery_failures, ` + alertRule

func scanAlert(r pgx.Row) (Alert, error) {
	var alert Alert

	err := r.Scan(
		&alert.ID,
		&alert.TenantID,
		&alert.Version,
		&alert.CreatedAt,
		&alert.UpdatedAt,
		&alert.UnitID,
		&alert.PolicyID,
		&alert.Type,
		&alert.Severity,
		&alert.Status,
		&alert.CauseCode,
		&alert.EvidenceKind,
		&alert.EvidenceText,
		&alert.ObservedAt,
		&alert.EvidenceIDs,
		&alert.DetectedAt,
		&alert.AcknowledgedAt,
		&alert.ResolvedAt,
		&alert.ResolutionReason,
		&alert.PreviousAlertID,
		&alert.DeliveryFailures,
		&alert.Rule,
	)

	return alert, apperr.Wrap(err, "scan (scanAlert)")
}

// UnitLocator resolves property membership of units for the propertyId filter (Assets).
type UnitLocator interface {
	UnitsOfProperty(ctx context.Context, c *ops.Call, property uuid.UUID) ([]uuid.UUID, error)
	OrgOfCustomer(ctx context.Context, c *ops.Call, customer uuid.UUID) (uuid.UUID, bool, error)
}

// TechAccess is the IR94 check for jobId-less technician writes (maintenance.Access).
type TechAccess interface {
	TechnicianUnit(
		ctx context.Context,
		c *ops.Call,
		unit uuid.UUID,
		inScope bool,
		internal bool,
	) error
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

var (
	severities    = map[string]bool{"critical": true, "warning": true, "normal": true}
	alertStatuses = map[string]bool{"open": true, "acknowledged": true, "resolved": true}
)

const severityRank = `CASE a.severity WHEN 'normal' THEN 0 WHEN 'warning' THEN 1 ELSE 2 END`

// GetInput is alerts.get input.
type GetInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *GetInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": apperr.KeyRequired}
	}

	return nil
}

// AckInput is alerts.acknowledge input.
type AckInput struct {
	AlertID uuid.UUID `json:"alertId"`
}

// Validate implements ops.Validator.
func (in *AckInput) Validate() map[string]string {
	if in.AlertID == uuid.Nil {
		return map[string]string{"alertId": apperr.KeyRequired}
	}

	return nil
}

// ResolveInput is alerts.resolve input.
type ResolveInput struct {
	AlertID               uuid.UUID   `json:"alertId"`
	ResolutionReason      string      `json:"resolutionReason"`
	ResolutionEvidenceIDs []uuid.UUID `json:"resolutionEvidenceIds"`
}

// Validate implements ops.Validator (IR87: 1–1000 after trimming).
func (in *ResolveInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.AlertID == uuid.Nil {
		fieldErrs["alertId"] = apperr.KeyRequired
	}

	in.ResolutionReason = strings.TrimSpace(in.ResolutionReason)
	if n := utf8.RuneCountInString(in.ResolutionReason); n < 1 || n > 1000 {
		fieldErrs["resolutionReason"] = apperr.KeyLength
	}

	if in.ResolutionEvidenceIDs == nil {
		fieldErrs["resolutionEvidenceIds"] = apperr.KeyRequired
	} else if len(in.ResolutionEvidenceIDs) > 20 || !distinct(in.ResolutionEvidenceIDs) {
		fieldErrs["resolutionEvidenceIds"] = apperr.KeyInvalid // at most 20, each once
	}

	return fieldErrs
}

// ActiveAlertCounts implements assets.Monitoring: open + acknowledged critical/warning alerts per unit (IR51).
func (Alerts) ActiveAlertCounts(
	ctx context.Context,
	c *ops.Call,
	units []uuid.UUID,
) (map[uuid.UUID]int, error) {
	out := map[uuid.UUID]int{}

	rows, err := c.Tx.Query(
		ctx,
		`SELECT unit_id, count(*) FROM monitoring.alerts WHERE unit_id = ANY($1) AND status IN ('open','acknowledged')
		AND severity IN ('critical','warning') GROUP BY unit_id`,
		units,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select monitoring.alerts")
	}
	defer rows.Close()

	for rows.Next() {
		var (
			unitID uuid.UUID
			count  int
		)

		err := rows.Scan(&unitID, &count)
		if err != nil {
			return nil, apperr.Wrap(err, "select monitoring.alerts")
		}

		out[unitID] = count
	}

	return out, apperr.Wrap(rows.Err(), "select monitoring.alerts")
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
func (Alerts) UnitSeverities(
	ctx context.Context,
	c *ops.Call,
	units []uuid.UUID,
) (map[uuid.UUID]string, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT a.unit_id, (array_agg(a.severity ORDER BY `+severityRank+` DESC))[1] FROM monitoring.alerts a
		WHERE a.unit_id = ANY($1) AND a.status <> 'resolved' GROUP BY a.unit_id`,
		units,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select monitoring.alerts")
	}
	defer rows.Close()

	out := map[uuid.UUID]string{}

	for rows.Next() {
		var (
			unitID   uuid.UUID
			severity string
		)

		err := rows.Scan(&unitID, &severity)
		if err != nil {
			return nil, apperr.Wrap(err, "select monitoring.alerts")
		}

		out[unitID] = severity
	}

	return out, apperr.Wrap(rows.Err(), "select monitoring.alerts")
}

// list serves alerts.list.
//
//	@Summary		alerts.list (read)
//	@ID				alerts.list
//	@Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | \
//	@Description	admin:alert.read | admin:alert.policy.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A05, DD-C01, DD-C08, DD-P04, DD-T01, DD-T07, DD-T02 · Query: filters \
//	@Description	unitId,unitIds,severity,status,from,to,customerId,propertyId · sort id,createdAt,updatedAt,severity \
//	@Description	(default severity desc;id asc)
//	@Tags			alerts
//	@Accept			json
//	@Produce		json
//	@Param			cursor		query		string		false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer		false	"page size 1–100, default 25"
//	@Param			sort		query		string		false	"field:direction — fields id,createdAt,updatedAt,severity; default severity desc;id asc"
//	@Param			unitId		query		string		false	"filter → unitId"
//	@Param			unitIds		query		[]string	false	"filter → unitIds (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
//	@Param			severity	query		string		false	"filter → severity"
//	@Param			status		query		string		false	"filter → status"
//	@Param			from		query		string		false	"filter → [from,to) on observedAt"
//	@Param			to			query		string		false	"filter → [from,to) on observedAt"
//	@Param			customerId	query		string		false	"filter"
//	@Param			propertyId	query		string		false	"filter"
//	@Success		200			{object}	ops.Envelope{data=AlertPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/alerts [get]
func (m Alerts) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Alert], error) {
	var filters struct {
		UnitID     *uuid.UUID   `json:"unitId,omitempty"`
		UnitIDs    *[]uuid.UUID `json:"unitIds,omitempty"`
		Severity   *string      `json:"severity,omitempty"`
		Status     *string      `json:"status,omitempty"`
		From       *time.Time   `json:"from,omitempty"`
		To         *time.Time   `json:"to,omitempty"`
		CustomerID *uuid.UUID   `json:"customerId,omitempty"`
		PropertyID *uuid.UUID   `json:"propertyId,omitempty"`
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filters) != nil {
			return paging.Page[Alert]{}, apperr.Fields(
				map[string]string{"filters": apperr.KeyInvalid},
			)
		}
	}

	switch {
	case filters.Severity != nil && !severities[*filters.Severity]:
		return paging.Page[Alert]{}, apperr.Fields(
			map[string]string{"filters.severity": apperr.KeyInvalid},
		)
	case filters.Status != nil && !alertStatuses[*filters.Status]:
		return paging.Page[Alert]{}, apperr.Fields(
			map[string]string{"filters.status": apperr.KeyInvalid},
		)
	case filters.From != nil && filters.To != nil && !filters.From.Before(*filters.To):
		return paging.Page[Alert]{}, apperr.Fields(map[string]string{"filters.to": apperr.KeyRange})
	}
	// default sort severity desc (critical → warning → normal), id asc; severity asc = normal, warning, critical
	order := severityRank + " DESC, a.id ASC"

	if query.Sort != nil {
		if query.Sort.Field == "severity" {
			dir, err := paging.Direction(query.Sort)
			if err != nil {
				return paging.Page[Alert]{}, apperr.Wrap(err, "paging.Direction")
			}

			order = severityRank + " " + dir + ", a.id ASC"
		} else {
			fieldOrder, err := paging.OrderBy(
				query.Sort,
				map[string]string{
					"id":        "a.id",
					"createdAt": "a.created_at",
					"updatedAt": "a.updated_at",
				},
				"",
			)
			if err != nil {
				return paging.Page[Alert]{}, apperr.Wrap(err, "paging.OrderBy")
			}

			order = fieldOrder
		}
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Alert]{}, apperr.Wrap(err, "paging.Resolve")
	}

	var args []any

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{alertScope(call, &args, unitscope.Equipment)}
	if filters.UnitID != nil {
		conds = append(conds, "a.unit_id = "+add(*filters.UnitID))
	}

	if filters.UnitIDs != nil {
		conds = append(conds, "a.unit_id = ANY("+add(*filters.UnitIDs)+")")
	}

	if filters.Severity != nil {
		conds = append(conds, "a.severity = "+add(*filters.Severity))
	}

	if filters.Status != nil {
		conds = append(conds, "a.status = "+add(*filters.Status))
	}

	if filters.From != nil {
		conds = append(conds, "a.observed_at >= "+add(*filters.From))
	}

	if filters.To != nil {
		conds = append(conds, "a.observed_at < "+add(*filters.To))
	}

	if filters.CustomerID != nil {
		org, ok, err := m.Units.OrgOfCustomer(ctx, call, *filters.CustomerID)
		if err != nil {
			return paging.Page[Alert]{}, apperr.Wrap(err, "monitoring.UnitLocator.OrgOfCustomer")
		}

		if !ok {
			org = uuid.Nil
		}

		conds = append(conds, "a.customer_org_id = "+add(org))
	}

	if filters.PropertyID != nil {
		units, err := m.Units.UnitsOfProperty(ctx, call, *filters.PropertyID)
		if err != nil {
			return paging.Page[Alert]{}, apperr.Wrap(err, "monitoring.UnitLocator.UnitsOfProperty")
		}

		conds = append(conds, "a.unit_id = ANY("+add(units)+")")
	}

	where := strings.Join(conds, " AND ")

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM monitoring.alerts a WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Alert]{}, apperr.Wrap(scanErr, "select monitoring.alerts")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM monitoring.alerts a WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			alertCols,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[Alert]{}, apperr.Wrap(err, "select monitoring.alerts")
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

	return paging.Page[Alert]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, apperr.Wrap(rows.Err(), "select monitoring.alerts")
}

// get serves alerts.get.
//
//	@Summary		alerts.get (read)
//	@ID				alerts.get
//	@Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | \
//	@Description	admin:alert.read | admin:alert.policy.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-T07, DD-A05, DD-T12, DD-P05, DD-P02
//	@Tags			alerts
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"input field id"
//	@Success		200	{object}	ops.Envelope{data=Alert}
//	@Failure		401	{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403	{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404	{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409	{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422	{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429	{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503	{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504	{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/alerts/{id} [get]
func (m Alerts) get(ctx context.Context, call *ops.Call, in *GetInput) (Alert, error) {
	args := []any{in.ID}

	alert, err := scanAlert(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+alertCols+" FROM monitoring.alerts a WHERE a.id = $1 AND "+alertScope(
				call,
				&args,
				unitscope.List,
			),
			args...),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return alert, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return alert, err
	}

	return alert, apperr.Wrap(
		unitscope.Gate(ctx, call, []uuid.UUID{alert.UnitID}),
		"unitscope.Gate",
	)
}

// lockAlert loads an alert in the caller's scope for update, applies the version and the technician rule (IR94 for
// operations without jobId).
func (m Alerts) lockAlert(
	ctx context.Context,
	call *ops.Call,
	id uuid.UUID,
) (string, uuid.UUID, error) {
	args := []any{id}

	var (
		status  string
		unit    uuid.UUID
		version int
	)

	err := call.Tx.QueryRow(ctx, "SELECT a.status, a.unit_id, a.version FROM monitoring.alerts a WHERE a.id = $1 AND "+
		alertScope(call, &args, unitscope.List)+" FOR UPDATE", args...).
		Scan(&status, &unit, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", unit, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return "", unit, apperr.Wrap(err, "select monitoring.alerts")
	}

	if call.Principal.Role == "technician" {
		err := m.Access.TechnicianUnit(
			ctx,
			call,
			unit,
			true,
			call.Principal.Employment == "internal",
		)
		if err != nil {
			return "", unit, apperr.Wrap(err, "monitoring.TechAccess.TechnicianUnit")
		}
	}

	if version != *call.ExpectedVersion {
		return "", unit, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	return status, unit, nil
}

// acknowledge serves alerts.acknowledge.
//
//	@Summary		alerts.acknowledge (write)
//	@ID				alerts.acknowledge
//	@Description	Authorization: technician:alert.read:assigned | admin:alert.resolve
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR94 technician write table (assignment and work window, jobId required when typed)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T07, DD-T12, DD-A05
//	@Tags			alerts
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target alerts, read alerts.get)"
//	@Param			alertId				path		string		true	"input field alertId"
//	@Param			request				body		AckInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Alert}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/alerts/{alertId}/acknowledge [post]
func (m Alerts) acknowledge(ctx context.Context, call *ops.Call, input *AckInput) (Alert, error) {
	status, unit, err := m.lockAlert(ctx, call, input.AlertID)
	if err != nil {
		return Alert{}, err
	}

	if status != "open" {
		return Alert{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	alert, err := scanAlert(
		call.Tx.QueryRow(
			ctx,
			`UPDATE monitoring.alerts a SET status = 'acknowledged', acknowledged_at = $2, acknowledged_by = $3,
		version = version + 1, updated_at = platform.app_now() WHERE a.id = $1 RETURNING `+alertCols,
			input.AlertID,
			call.Now,
			call.Principal.MembershipID,
		),
	)
	if err != nil {
		return Alert{}, err
	}

	call.Emit(
		ops.Event{
			AggregateType: "alert",
			AggregateID:   alert.ID,
			Type:          "AlertAcknowledged",
			Payload:       map[string]any{"unitId": unit},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "alerts.acknowledge",
			TargetKind:      "alert",
			TargetID:        alert.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &alert.Version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return alert, nil
}

// resolve serves alerts.resolve.
//
//	@Summary		alerts.resolve (write)
//	@ID				alerts.resolve
//	@Description	Authorization: technician:alert.resolve:assigned | admin:alert.resolve
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR87 resolutionReason 1-1000; IR94 technician write table (assignment and work window, jobId \
//	@Description	required when typed); IR327 a policy-free alert needs at least one evidence ID, every ID a candidate \
//	@Description	of alerts.evidence
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T07, DD-A05
//	@Tags			alerts
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target alerts, read alerts.get)"
//	@Param			alertId				path		string			true	"input field alertId"
//	@Param			request				body		ResolveInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Alert}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/alerts/{alertId}/resolve [post]
func (m Alerts) resolve(ctx context.Context, call *ops.Call, input *ResolveInput) (Alert, error) {
	status, unit, err := m.lockAlert(ctx, call, input.AlertID)
	if err != nil {
		return Alert{}, err
	}

	if status == "resolved" {
		return Alert{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	cur, err := scanAlert(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+alertCols+" FROM monitoring.alerts a WHERE a.id = $1",
			input.AlertID,
		),
	)
	if err != nil {
		return Alert{}, err
	}

	checkEvidenceErr := m.checkEvidence(ctx, call, cur, input.ResolutionEvidenceIDs)
	if checkEvidenceErr != nil {
		return Alert{}, checkEvidenceErr
	}

	alert, err := scanAlert(
		call.Tx.QueryRow(
			ctx,
			`UPDATE monitoring.alerts a SET status = 'resolved', resolved_at = $2, resolved_by = $3, resolution_reason = $4,
		resolution_evidence_ids = $5, version = version + 1, updated_at = platform.app_now() WHERE a.id = $1 RETURNING
		`+alertCols,
			input.AlertID,
			call.Now,
			call.Principal.MembershipID,
			input.ResolutionReason,
			input.ResolutionEvidenceIDs,
		),
	)
	if err != nil {
		return Alert{}, err
	}

	call.Emit(
		ops.Event{
			AggregateType: "alert",
			AggregateID:   alert.ID,
			Type:          "AlertResolved",
			Payload:       map[string]any{"unitId": unit},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "alerts.resolve",
			TargetKind:      "alert",
			TargetID:        alert.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &alert.Version,
			Reason:          input.ResolutionReason, Result: "", Before: nil, After: nil,
		},
	)

	return alert, nil
}
