package restrictions

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/pradita/ac-project/service/api/internal/platform/events"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Policy is RestrictionPolicy.
type Policy struct {
	Kind                   string   `json:"kind"`
	MinimumCoolingSetpoint *float64 `json:"minimumCoolingSetpoint,omitempty"`
}

func parsePolicy(raw json.RawMessage) (Policy, bool) {
	var p Policy
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if len(raw) == 0 || dec.Decode(&p) != nil {
		return p, false
	}
	switch p.Kind {
	case "power_off":
		return p, p.MinimumCoolingSetpoint == nil
	case "temperature_limit":
		return p, p.MinimumCoolingSetpoint != nil && !math.IsNaN(*p.MinimumCoolingSetpoint) && !math.IsInf(*p.MinimumCoolingSetpoint, 0)
	}
	return p, false
}

func trimReason(s *string, key string, fe map[string]string) {
	*s = strings.TrimSpace(*s)
	if n := utf8.RuneCountInString(*s); n < 1 || n > 1000 {
		fe[key] = "error.length"
	}
}

func uniqueIDs(ids []uuid.UUID) bool {
	seen := map[uuid.UUID]bool{}
	for _, id := range ids {
		if id == uuid.Nil || seen[id] {
			return false
		}
		seen[id] = true
	}
	return len(ids) > 0
}

// ScheduleInput is restrictions.schedule input.
type ScheduleInput struct {
	ContractID              uuid.UUID       `json:"contractId"`
	ExpectedContractVersion int             `json:"expectedContractVersion"`
	CauseInvoiceIDs         []uuid.UUID     `json:"causeInvoiceIds"`
	UnitIDs                 []uuid.UUID     `json:"unitIds"`
	Policy                  json.RawMessage `json:"policy" swaggertype:"object"`
	ExecuteAfter            time.Time       `json:"executeAfter"`
	Reason                  string          `json:"reason"`
	RulesVersion            string          `json:"rulesVersion"`
	policy                  Policy
}

// Validate implements ops.Validator.
func (in *ScheduleInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.ContractID == uuid.Nil {
		fe["contractId"] = "error.required"
	}
	if in.ExpectedContractVersion < 1 {
		fe["expectedContractVersion"] = "error.required"
	}
	if !uniqueIDs(in.CauseInvoiceIDs) {
		fe["causeInvoiceIds"] = "error.invalid"
	}
	if !uniqueIDs(in.UnitIDs) || len(in.UnitIDs) > 100 {
		fe["unitIds"] = "error.invalid"
	}
	p, ok := parsePolicy(in.Policy)
	if !ok {
		fe["policy"] = "error.invalid"
	}
	in.policy = p
	if in.ExecuteAfter.IsZero() {
		fe["executeAfter"] = "error.required"
	}
	trimReason(&in.Reason, "reason", fe)
	if in.RulesVersion = strings.TrimSpace(in.RulesVersion); in.RulesVersion == "" || len(in.RulesVersion) > 64 {
		fe["rulesVersion"] = "error.required"
	}
	return fe
}

// overdueInvoices returns the contract's overdue unpaid invoices (DD-A09: all of them are the causes).
func overdueInvoices(ctx context.Context, c *ops.Call, contract uuid.UUID) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, `SELECT id FROM billing.invoices WHERE contract_id = $1 AND status = 'unpaid' AND due_at < $2 ORDER BY id`, contract, c.Now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

func sameSet(a, b []uuid.UUID) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.SortFunc(x, func(p, q uuid.UUID) int { return strings.Compare(p.String(), q.String()) })
	slices.SortFunc(y, func(p, q uuid.UUID) int { return strings.Compare(p.String(), q.String()) })
	return slices.Equal(x, y)
}

// @Summary		restrictions.schedule (write)
// @ID				restrictions.schedule
// @Description	Authorization: admin:restriction.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A09 · Input versions: expectedContractVersion=Contract.version
// @Tags			restrictions
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		ScheduleInput	true	"input"
// @Success		200				{object}	ops.Envelope{data=Restriction}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/restrictions [post]
func (m Restrictions) schedule(ctx context.Context, c *ops.Call, in *ScheduleInput) (Restriction, error) {
	var (
		version              int
		rules                *string
		planType             string
		eligible             bool
		customer, org        uuid.UUID
		contractUnits        []uuid.UUID
		termStart, termUntil time.Time
	)
	err := c.Tx.QueryRow(ctx, `SELECT k.version, k.plan_type, k.restriction_eligible, k.rules_version, k.customer_id, k.customer_org_id, lower(k.term), upper(k.term),
		COALESCE((SELECT array_agg(cu.unit_id) FROM billing.contract_units cu WHERE cu.contract_id = k.id AND cu.contract_version = k.version), '{}')
		FROM billing.contracts k WHERE k.id = $1 AND k.is_current FOR UPDATE`, in.ContractID).
		Scan(&version, &planType, &eligible, &rules, &customer, &org, &termStart, &termUntil, &contractUnits)
	if errors.Is(err, pgx.ErrNoRows) {
		return Restriction{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return Restriction{}, err
	}
	if version != in.ExpectedContractVersion {
		return Restriction{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if planType != "rto" || !eligible || rules == nil || c.Now.Before(termStart) || !c.Now.Before(termUntil) {
		return Restriction{}, apperr.Fields(map[string]string{"contractId": "errors.restriction_ineligible"})
	}
	if *rules != in.RulesVersion {
		return Restriction{}, apperr.Fields(map[string]string{"rulesVersion": "errors.rules_version_mismatch"})
	}
	if in.ExecuteAfter.Before(c.Now.Add(24 * time.Hour)) {
		return Restriction{}, apperr.Fields(map[string]string{"executeAfter": "errors.notice_24h"})
	}
	overdue, err := overdueInvoices(ctx, c, in.ContractID)
	if err != nil {
		return Restriction{}, err
	}
	if len(overdue) == 0 {
		return Restriction{}, apperr.E(apperr.Conflict, "errors.no_overdue_invoice")
	}
	if !sameSet(overdue, in.CauseInvoiceIDs) {
		return Restriction{}, apperr.Fields(map[string]string{"causeInvoiceIds": "errors.cause_invoices_mismatch"})
	}
	for _, u := range in.UnitIDs {
		if !slices.Contains(contractUnits, u) {
			return Restriction{}, apperr.Fields(map[string]string{"unitIds": "errors.unit_not_in_contract"})
		}
		t, found, err := m.Units.RestrictionTarget(ctx, c, u)
		if err != nil {
			return Restriction{}, err
		}
		if !found || t.Archived {
			return Restriction{}, apperr.E(apperr.Conflict, "errors.unit_archived")
		}
		if !t.Control || (in.policy.Kind == "temperature_limit" && !t.HasTemperature) {
			return Restriction{}, apperr.Fields(map[string]string{"unitIds": "errors.unsupported_capability"})
		}
		if in.policy.Kind == "temperature_limit" {
			v := *in.policy.MinimumCoolingSetpoint
			if v < t.TempMin || v > t.TempMax || (t.Step > 0 && math.Abs(math.Remainder(v-t.TempMin, t.Step)) > 1e-9) {
				return Restriction{}, apperr.Fields(map[string]string{"policy": "errors.setpoint_out_of_range"})
			}
		}
	}
	var busy bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM restrictions.restriction_units ru JOIN restrictions.restrictions r ON r.id = ru.restriction_id
			WHERE ru.unit_id = ANY($1) AND (r.state NOT IN ('released','cancelled')
			  OR jsonb_path_exists(r.recovery_cases, '$[*] ? (@.state != "resolved" && @.unitId == $u)', jsonb_build_object('u', ru.unit_id::text))))`, in.UnitIDs).Scan(&busy); err != nil {
		return Restriction{}, err
	}
	if busy {
		return Restriction{}, apperr.E(apperr.Conflict, "errors.restriction_exists")
	}
	recipients, err := noticeRecipients(ctx, c, org)
	if err != nil {
		return Restriction{}, err
	}
	if len(recipients) == 0 {
		return Restriction{}, apperr.Fields(map[string]string{"unitIds": "errors.no_notice_recipient"})
	}
	id := uuid.New()
	policy, _ := json.Marshal(in.policy)
	if _, err := c.Tx.Exec(ctx, `INSERT INTO restrictions.restrictions (id, tenant_id, contract_id, contract_version, customer_id, rules_version, notice_at, execute_after, reason,
		policy, state, created_at, updated_at) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, 'scheduled', $6, $6)`,
		id, in.ContractID, version, customer, in.RulesVersion, c.Now, in.ExecuteAfter, in.Reason, policy); err != nil {
		return Restriction{}, err
	}
	for _, inv := range in.CauseInvoiceIDs {
		if _, err := c.Tx.Exec(ctx, `INSERT INTO restrictions.restriction_invoices (tenant_id, restriction_id, invoice_id) VALUES (current_setting('app.tenant_id')::uuid, $1, $2)`, id, inv); err != nil {
			return Restriction{}, err
		}
	}
	for _, u := range in.UnitIDs {
		if _, err := c.Tx.Exec(ctx, `INSERT INTO restrictions.restriction_units (tenant_id, restriction_id, unit_id, apply_state, release_state)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, 'not_sent', 'none')`, id, u); err != nil {
			return Restriction{}, err
		}
	}
	notices := []uuid.UUID{}
	for _, r := range recipients {
		n, err := m.Notify.Create(ctx, c, notify.New{RecipientMembershipID: r, Type: "restriction", TemplateKey: "restriction", TargetKind: "restriction", TargetID: id,
			Severity: "warning", Params: map[string]any{"executeAfter": in.ExecuteAfter, "policyKind": in.policy.Kind}})
		if err != nil {
			return Restriction{}, err
		}
		notices = append(notices, n)
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE restrictions.restrictions SET notice_notification_ids = $2 WHERE id = $1`, id, notices); err != nil {
		return Restriction{}, err
	}
	c.Emit(ops.Event{AggregateType: "restriction", AggregateID: id, Type: "RestrictionScheduled", Payload: map[string]any{"contractId": in.ContractID, "unitIds": in.UnitIDs}})
	v := 1
	c.Audit(ops.AuditEntry{Action: "restrictions.schedule", TargetKind: "restriction", TargetID: id.String(), NextVersion: &v, Reason: in.Reason})
	return load(ctx, c, id, false)
}

// noticeRecipients returns the active client memberships of the customer organization (IR05 / IR19: clients read
// every unit and contract of their organization).
func noticeRecipients(ctx context.Context, c *ops.Call, org uuid.UUID) ([]uuid.UUID, error) {
	return ops.Query[[]uuid.UUID](ctx, c, identity.QueryMembers, identity.MembersInput{Role: "client", OrganizationID: &org}) // IR191
}

// ExecuteInput is restrictions.execute input.
type ExecuteInput struct {
	RestrictionID         uuid.UUID `json:"restrictionId"`
	ConfirmedRulesVersion string    `json:"confirmedRulesVersion"`
}

// Validate implements ops.Validator.
func (in *ExecuteInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.RestrictionID == uuid.Nil {
		fe["restrictionId"] = "error.required"
	}
	if strings.TrimSpace(in.ConfirmedRulesVersion) == "" {
		fe["confirmedRulesVersion"] = "error.required"
	}
	return fe
}

// lockCurrent loads a restriction for update and checks the expected version.
func lockCurrent(ctx context.Context, c *ops.Call, id uuid.UUID) (Restriction, error) {
	x, err := load(ctx, c, id, true)
	if err != nil {
		return x, err
	}
	if x.Version != *c.ExpectedVersion {
		return x, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	return x, nil
}

func causesPaid(ctx context.Context, c *ops.Call, id uuid.UUID) (bool, error) {
	var unpaid bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM restrictions.restriction_invoices ri JOIN billing.invoices i ON i.id = ri.invoice_id
		WHERE ri.restriction_id = $1 AND i.status <> 'paid')`, id).Scan(&unpaid)
	return !unpaid, err
}

// @Summary		restrictions.execute (write)
// @ID				restrictions.execute
// @Description	Authorization: admin:restriction.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR90 device operation running: not_sent intent
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A09
// @Tags			restrictions
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer			true	"all: required (target restrictions, read restrictions.get)"
// @Param			restrictionId		path		string			true	"input field restrictionId"
// @Param			request				body		ExecuteInput	true	"input; the path parameters come from the route"
// @Success		200					{object}	ops.Envelope{data=Restriction}
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/restrictions/{restrictionId}/execute [post]
func (m Restrictions) execute(ctx context.Context, c *ops.Call, in *ExecuteInput) (Restriction, error) {
	x, err := lockCurrent(ctx, c, in.RestrictionID)
	if err != nil {
		return x, err
	}
	if x.State != "scheduled" {
		return x, apperr.E(apperr.Conflict, "errors.restriction_state")
	}
	if in.ConfirmedRulesVersion != x.RulesVersion {
		return x, apperr.Fields(map[string]string{"confirmedRulesVersion": "errors.rules_version_mismatch"})
	}
	if c.Now.Before(x.ExecuteAfter) || c.Now.Before(x.NoticeAt.Add(24*time.Hour)) {
		return x, apperr.E(apperr.Conflict, "errors.restriction_not_due")
	}
	if (x.Exception != nil && c.Now.Before(x.Exception.Until)) || (x.GraceUntil != nil && c.Now.Before(*x.GraceUntil)) {
		return x, apperr.E(apperr.Conflict, "errors.restriction_exempted")
	}
	var contractVersion int
	if err := c.Tx.QueryRow(ctx, `SELECT COALESCE((SELECT version FROM billing.contracts WHERE id = $1 AND is_current), 0)`, x.ContractID).Scan(&contractVersion); err != nil {
		return x, err
	}
	evidence := 0 // the notices identity stored for the restriction at notice time (IR191)
	if len(x.NoticeNotificationIDs) > 0 {
		n, err := ops.Query[int](ctx, c, identity.QueryNotificationsStored, identity.NotificationsStoredInput{IDs: x.NoticeNotificationIDs, TargetID: x.ID, OccurredAt: x.NoticeAt})
		if err != nil {
			return x, err
		}
		evidence = n
	}
	if evidence == 0 || evidence != len(x.NoticeNotificationIDs) || contractVersion != x.ContractVersion {
		return x, apperr.E(apperr.Conflict, "errors.notice_invalid")
	}
	paid, err := causesPaid(ctx, c, x.ID)
	if err != nil {
		return x, err
	}
	next := x.Version + 1
	if paid { // all causes paid right before execution: cancel without apply requests (DD-A09)
		if _, err := c.Tx.Exec(ctx, `UPDATE restrictions.restrictions SET state = 'cancelled', version = version + 1, updated_at = $2 WHERE id = $1`, x.ID, c.Now); err != nil {
			return x, err
		}
		c.Emit(ops.Event{AggregateType: "restriction", AggregateID: x.ID, Type: "RestrictionCancelled", Payload: map[string]any{"source": "payment"}})
		c.Audit(ops.AuditEntry{Action: "restrictions.execute", TargetKind: "restriction", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &next, Reason: "causes paid"})
		return load(ctx, c, x.ID, false)
	}
	action, _ := json.Marshal(map[string]any{"kind": "apply_restriction", "restrictionId": x.ID, "rulesVersion": x.RulesVersion, "policy": x.Policy})
	for _, u := range x.PerUnit {
		cmd, delivered, reason, err := m.send(ctx, c, x.ID, u.UnitID, action)
		if err != nil {
			return x, err
		}
		state := "not_sent"
		if delivered {
			state = "sent_unknown"
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE restrictions.restriction_units SET apply_state = $3, apply_command_ids = apply_command_ids || $4::uuid, pending_reason = $5
			WHERE restriction_id = $1 AND unit_id = $2`, x.ID, u.UnitID, state, cmd, reason); err != nil {
			return x, err
		}
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE restrictions.restrictions SET state = 'requested', version = version + 1, updated_at = $2 WHERE id = $1`, x.ID, c.Now); err != nil {
		return x, err
	}
	c.Emit(ops.Event{AggregateType: "restriction", AggregateID: x.ID, Type: "RestrictionRequested", Payload: map[string]any{"unitIds": x.UnitIDs}})
	c.Audit(ops.AuditEntry{Action: "restrictions.execute", TargetKind: "restriction", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &next})
	return load(ctx, c, x.ID, false)
}

// send creates one restriction Command for a unit (D03): delivered when the bound device is online, powered and
// not running a device operation; otherwise an undelivered intent (status=requested, delivery=not_sent) with the
// pending reason. Restriction Commands bypass the normal-command mutual exclusion (IR90 REV19-018).
func (m Restrictions) send(ctx context.Context, c *ops.Call, restriction, unit uuid.UUID, action []byte) (uuid.UUID, bool, *string, error) {
	dev, conn, power, bound, err := m.Devices.BoundDevice(ctx, c, unit)
	if err != nil {
		return uuid.Nil, false, nil, err
	}
	opBusy, err := m.Devices.OperationBusy(ctx, c, unit)
	if err != nil {
		return uuid.Nil, false, nil, err
	}
	var reason *string
	switch {
	case !bound || conn != "online" || power == "off":
		r := "offline"
		reason = &r
	case opBusy:
		r := "device_operation_running"
		reason = &r
	}
	var device *uuid.UUID
	if bound {
		device = &dev
	}
	status, delivery, sentAt := "sent", "sent", &c.Now
	if reason != nil {
		status, delivery, sentAt = "requested", "not_sent", nil
	}
	// the command row is equipment's (IR185): billing assigns the ID and publishes the decided command
	id := uuid.Must(uuid.NewV7())
	err = events.Publish(ctx, c.Tx, c.Principal.TenantID, "command", id, events.RestrictionCommandRequested, events.RestrictionCommand{
		CommandID: id, UnitID: unit, DeviceID: device, ActorID: SystemActor, Action: action, RestrictionID: restriction, Status: status,
		Delivery: delivery, RequestedAt: c.Now, SentAt: sentAt, ExpiresAt: c.Now.Add(IntentTTL), CorrelationID: c.CorrelationID})
	if err == nil { // billing's own record of the command (IR194)
		var a struct{ Kind string }
		_ = json.Unmarshal(action, &a)
		_, err = c.Tx.Exec(ctx, `INSERT INTO restrictions.restriction_commands (tenant_id, command_id, restriction_id, unit_id, kind, delivered, status, requested_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'requested', $7, $7)`, c.Principal.TenantID, id, restriction, unit, a.Kind, reason == nil, c.Now)
	}
	if err == nil {
		c.Emit(ops.Event{AggregateType: "command", AggregateID: id, Type: "CommandRequested", Payload: map[string]any{"unitId": unit, "deviceId": device, "action": json.RawMessage(action), "delivery": delivery}})
	}
	return id, reason == nil, reason, err
}

// CancelInput is restrictions.cancel input.
type CancelInput struct {
	RestrictionID uuid.UUID `json:"restrictionId"`
	Reason        string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *CancelInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.RestrictionID == uuid.Nil {
		fe["restrictionId"] = "error.required"
	}
	trimReason(&in.Reason, "reason", fe)
	return fe
}

// @Summary		restrictions.cancel (write)
// @ID				restrictions.cancel
// @Description	Authorization: admin:restriction.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR87 reason 1-1000; IR96 state table; requested/applied to release_requested with releaseIntent=cancel
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A10
// @Tags			restrictions
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer		true	"all: required (target restrictions, read restrictions.get)"
// @Param			restrictionId		path		string		true	"input field restrictionId"
// @Param			request				body		CancelInput	true	"input; the path parameters come from the route"
// @Success		200					{object}	ops.Envelope{data=Restriction}
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/restrictions/{restrictionId}/cancel [post]
func (m Restrictions) cancel(ctx context.Context, c *ops.Call, in *CancelInput) (Restriction, error) {
	x, err := load(ctx, c, in.RestrictionID, true)
	if err != nil {
		return x, err
	}
	switch x.State {
	case "release_requested": // IR96: idempotent, no version or audit
		return x, nil
	case "released", "cancelled":
		return x, apperr.E(apperr.Conflict, "errors.restriction_state")
	}
	if x.Version != *c.ExpectedVersion {
		return x, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	next := x.Version + 1
	if x.State == "scheduled" {
		if _, err := c.Tx.Exec(ctx, `UPDATE restrictions.restrictions SET state = 'cancelled', version = version + 1, updated_at = $2 WHERE id = $1`, x.ID, c.Now); err != nil {
			return x, err
		}
		c.Emit(ops.Event{AggregateType: "restriction", AggregateID: x.ID, Type: "RestrictionCancelled", Payload: map[string]any{"source": "cancel"}})
	} else if err := m.RequestRelease(ctx, c, x.ID, "cancel"); err != nil {
		return x, err
	}
	c.Audit(ops.AuditEntry{Action: "restrictions.cancel", TargetKind: "restriction", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &next, Reason: in.Reason})
	return load(ctx, c, x.ID, false)
}
