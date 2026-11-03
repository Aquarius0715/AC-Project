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

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// Policy is RestrictionPolicy.
type Policy struct {
	Kind                   string   `json:"kind"`
	MinimumCoolingSetpoint *float64 `json:"minimumCoolingSetpoint,omitempty"`
}

func parsePolicy(raw json.RawMessage) (Policy, bool) {
	var policy Policy

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	if len(raw) == 0 || dec.Decode(&policy) != nil {
		return policy, false
	}

	switch policy.Kind {
	case "power_off":
		return policy, policy.MinimumCoolingSetpoint == nil
	case policyTemperatureLimit:
		return policy, policy.MinimumCoolingSetpoint != nil &&
			!math.IsNaN(*policy.MinimumCoolingSetpoint) &&
			!math.IsInf(*policy.MinimumCoolingSetpoint, 0)
	}

	return policy, false
}

func trimReason(s *string, key string, fe map[string]string) {
	*s = strings.TrimSpace(*s)
	if n := utf8.RuneCountInString(*s); n < 1 || n > 1000 {
		fe[key] = apperr.KeyLength
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
	Policy                  json.RawMessage `json:"policy"                  swaggertype:"object"`
	ExecuteAfter            time.Time       `json:"executeAfter"`
	Reason                  string          `json:"reason"`
	RulesVersion            string          `json:"rulesVersion"`
	policy                  Policy
}

// Validate implements ops.Validator.
func (in *ScheduleInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ContractID == uuid.Nil {
		fieldErrs["contractId"] = apperr.KeyRequired
	}

	if in.ExpectedContractVersion < 1 {
		fieldErrs["expectedContractVersion"] = apperr.KeyRequired
	}

	if !uniqueIDs(in.CauseInvoiceIDs) {
		fieldErrs["causeInvoiceIds"] = apperr.KeyInvalid
	}

	if !uniqueIDs(in.UnitIDs) || len(in.UnitIDs) > 100 {
		fieldErrs[keyUnitIDs] = apperr.KeyInvalid
	}

	p, ok := parsePolicy(in.Policy)
	if !ok {
		fieldErrs[keyPolicy] = apperr.KeyInvalid
	}

	in.policy = p
	if in.ExecuteAfter.IsZero() {
		fieldErrs[keyExecuteAfter] = apperr.KeyRequired
	}

	trimReason(&in.Reason, "reason", fieldErrs)

	if in.RulesVersion = strings.TrimSpace(
		in.RulesVersion,
	); in.RulesVersion == "" ||
		len(in.RulesVersion) > 64 {
		fieldErrs[keyRulesVersion] = apperr.KeyRequired
	}

	return fieldErrs
}

// overdueInvoices returns the contract's overdue unpaid invoices (DD-A09: all of them are the causes).
func overdueInvoices(ctx context.Context, c *ops.Call, contract uuid.UUID) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT id FROM billing.invoices WHERE contract_id = $1 AND status = 'unpaid' AND due_at < $2 ORDER BY id`,
		contract,
		c.Now,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select billing.invoices")
	}
	defer rows.Close()

	out, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])

	return out, apperr.Wrap(err, "select billing.invoices")
}

func sameSet(a, b []uuid.UUID) bool {
	x, y := slices.Clone(a), slices.Clone(b)
	slices.SortFunc(x, func(p, q uuid.UUID) int { return strings.Compare(p.String(), q.String()) })
	slices.SortFunc(y, func(p, q uuid.UUID) int { return strings.Compare(p.String(), q.String()) })

	return slices.Equal(x, y)
}

// schedule serves restrictions.schedule.
//
//	@Summary		restrictions.schedule (write)
//	@ID				restrictions.schedule
//	@Description	Authorization: admin:restriction.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A09 · Input versions: expectedContractVersion=Contract.version
//	@Tags			restrictions
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		ScheduleInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Restriction}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/restrictions [post]
func (m Restrictions) schedule(
	ctx context.Context,
	call *ops.Call,
	input *ScheduleInput,
) (Restriction, error) {
	var (
		version              int
		rules                *string
		planType             string
		eligible             bool
		customer, org        uuid.UUID
		contractUnits        []uuid.UUID
		termStart, termUntil time.Time
	)

	err := call.Tx.QueryRow(ctx, `SELECT k.version, k.plan_type, k.restriction_eligible, k.rules_version, k.customer_id,
		k.customer_org_id, lower(k.term), upper(k.term),
		COALESCE((SELECT array_agg(cu.unit_id) FROM billing.contract_units cu WHERE cu.contract_id = k.id AND
		cu.contract_version = k.version), '{}')
		FROM billing.contracts k WHERE k.id = $1 AND k.is_current FOR UPDATE`, input.ContractID).
		Scan(&version, &planType, &eligible, &rules, &customer, &org, &termStart, &termUntil, &contractUnits)
	if errors.Is(err, pgx.ErrNoRows) {
		return Restriction{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return Restriction{}, apperr.Wrap(err, "select billing.contracts")
	}

	if version != input.ExpectedContractVersion {
		return Restriction{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if planType != "rto" || !eligible || rules == nil || call.Now.Before(termStart) ||
		!call.Now.Before(termUntil) {
		return Restriction{}, apperr.Fields(
			map[string]string{"contractId": "errors.restriction_ineligible"},
		)
	}

	if *rules != input.RulesVersion {
		return Restriction{}, apperr.Fields(
			map[string]string{keyRulesVersion: keyRulesVersionMismatch},
		)
	}

	if input.ExecuteAfter.Before(call.Now.Add(24 * time.Hour)) {
		return Restriction{}, apperr.Fields(
			map[string]string{keyExecuteAfter: "errors.notice_24h"},
		)
	}

	overdue, err := overdueInvoices(ctx, call, input.ContractID)
	if err != nil {
		return Restriction{}, err
	}

	if len(overdue) == 0 {
		return Restriction{}, apperr.E(apperr.Conflict, "errors.no_overdue_invoice")
	}

	if !sameSet(overdue, input.CauseInvoiceIDs) {
		return Restriction{}, apperr.Fields(
			map[string]string{"causeInvoiceIds": "errors.cause_invoices_mismatch"},
		)
	}

	for _, unitID := range input.UnitIDs {
		if !slices.Contains(contractUnits, unitID) {
			return Restriction{}, apperr.Fields(
				map[string]string{keyUnitIDs: "errors.unit_not_in_contract"},
			)
		}

		unitCaps, found, err := m.Units.RestrictionTarget(ctx, call, unitID)
		if err != nil {
			return Restriction{}, apperr.Wrap(err, "restrictions.Units.RestrictionTarget")
		}

		if !found || unitCaps.Archived {
			return Restriction{}, apperr.E(apperr.Conflict, "errors.unit_archived")
		}

		if !unitCaps.Control ||
			(input.policy.Kind == policyTemperatureLimit && !unitCaps.HasTemperature) {
			return Restriction{}, apperr.Fields(
				map[string]string{keyUnitIDs: "errors.unsupported_capability"},
			)
		}

		if input.policy.Kind == policyTemperatureLimit {
			v := *input.policy.MinimumCoolingSetpoint
			if v < unitCaps.TempMin || v > unitCaps.TempMax ||
				(unitCaps.Step > 0 && math.Abs(math.Remainder(v-unitCaps.TempMin, unitCaps.Step)) > 1e-9) {
				return Restriction{}, apperr.Fields(
					map[string]string{keyPolicy: "errors.setpoint_out_of_range"},
				)
			}
		}
	}
	// one active restriction per unit (DDC-08 §3): schedules touching the same unit wait for each other here, in unit
	// order, so two contracts sharing a unit cannot both pass the check below (the contract row lock only orders
	// schedules of one contract)
	_, execErr := call.Tx.Exec(
		ctx,
		`SELECT pg_advisory_xact_lock(hashtext('restrictions.unit:' || u::text)) FROM unnest($1::uuid[]) AS u ORDER BY u`,
		input.UnitIDs,
	)
	if execErr != nil {
		return Restriction{}, apperr.Wrap(execErr, "select unnest")
	}

	var busy bool

	scanErr := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM restrictions.restriction_units ru JOIN
		restrictions.restrictions r ON r.id = ru.restriction_id
			WHERE ru.unit_id = ANY($1) AND (r.state NOT IN ('released','cancelled')
			  OR jsonb_path_exists(r.recovery_cases, '$[*] ? (@.state != "resolved" && @.unitId == $u)', jsonb_build_object('u',
			  ru.unit_id::text))))`, input.UnitIDs).
		Scan(&busy)
	if scanErr != nil {
		return Restriction{}, apperr.Wrap(scanErr, "select restrictions.restriction_units")
	}

	if busy {
		return Restriction{}, apperr.E(apperr.Conflict, "errors.restriction_exists")
	}

	recipients, err := noticeRecipients(ctx, call, org)
	if err != nil {
		return Restriction{}, err
	}

	if len(recipients) == 0 {
		return Restriction{}, apperr.Fields(
			map[string]string{keyUnitIDs: "errors.no_notice_recipient"},
		)
	}

	restrictionID := uuid.New()

	policy, marshalErr := json.Marshal(input.policy)
	if marshalErr != nil {
		return Restriction{}, apperr.Wrap(marshalErr, "json.Marshal")
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`INSERT INTO restrictions.restrictions (id, tenant_id, contract_id, contract_version, customer_id, rules_version,
			notice_at, execute_after, reason,
		policy, state, created_at, updated_at) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8,
		$9, 'scheduled', $6, $6)`,
		restrictionID,
		input.ContractID,
		version,
		customer,
		input.RulesVersion,
		call.Now,
		input.ExecuteAfter,
		input.Reason,
		policy,
	)
	if execErr != nil {
		return Restriction{}, apperr.Wrap(execErr, "insert restrictions.restrictions")
	}

	for _, inv := range input.CauseInvoiceIDs {
		_, err := call.Tx.Exec(
			ctx,
			`INSERT INTO restrictions.restriction_invoices (tenant_id, restriction_id, invoice_id) VALUES
				(current_setting('app.tenant_id')::uuid, $1, $2)`,
			restrictionID,
			inv,
		)
		if err != nil {
			return Restriction{}, apperr.Wrap(err, "insert restrictions.restriction_invoices")
		}
	}

	for _, unitID := range input.UnitIDs {
		_, err := call.Tx.Exec(
			ctx,
			`INSERT INTO restrictions.restriction_units (tenant_id, restriction_id, unit_id, apply_state, release_state)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, 'not_sent', 'none')`,
			restrictionID,
			unitID,
		)
		if err != nil {
			return Restriction{}, apperr.Wrap(err, "insert restrictions.restriction_units")
		}
	}

	notices := []uuid.UUID{}

	for _, r := range recipients {
		notificationID, err := m.Notify.Create(ctx, call, notify.New{
			RecipientMembershipID: r,
			Type:                  kindRestriction,
			TemplateKey:           kindRestriction,
			TargetKind:            kindRestriction,
			TargetID:              restrictionID,
			Severity:              "warning",
			Params: map[string]any{
				keyExecuteAfter: input.ExecuteAfter,
				"policyKind":    input.policy.Kind,
			}, Channel: "", SourceAlertID: nil,
		})
		if err != nil {
			return Restriction{}, apperr.Wrap(err, "notify.Store.Create")
		}

		notices = append(notices, notificationID)
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE restrictions.restrictions SET notice_notification_ids = $2 WHERE id = $1`,
		restrictionID,
		notices,
	)
	if execErr != nil {
		return Restriction{}, apperr.Wrap(execErr, "update restrictions.restrictions")
	}

	call.Emit(
		ops.Event{
			AggregateType: kindRestriction,
			AggregateID:   restrictionID,
			Type:          "RestrictionScheduled",
			Payload: map[string]any{
				"contractId": input.ContractID,
				keyUnitIDs:   input.UnitIDs,
			},
		},
	)

	nextVersion := 1
	call.Audit(
		ops.AuditEntry{
			Action:      "restrictions.schedule",
			TargetKind:  kindRestriction,
			TargetID:    restrictionID.String(),
			NextVersion: &nextVersion,
			Reason:      input.Reason, PreviousVersion: nil, Result: "", Before: nil, After: nil,
		},
	)

	return load(ctx, call, restrictionID, false)
}

// noticeRecipients returns the active client memberships of the customer organization (IR05 / IR19: clients read
// every unit and contract of their organization).
func noticeRecipients(ctx context.Context, c *ops.Call, org uuid.UUID) ([]uuid.UUID, error) {
	return ops.Query[[]uuid.UUID](
		ctx,
		c,
		identity.QueryMembers,
		identity.MembersInput{
			Role:           authz.RoleClient,
			OrganizationID: &org,
			IDs:            nil,
			Permission:     "",
			ClientRole:     "",
		},
	) // IR191
}

// ExecuteInput is restrictions.execute input.
type ExecuteInput struct {
	RestrictionID         uuid.UUID `json:"restrictionId"`
	ConfirmedRulesVersion string    `json:"confirmedRulesVersion"`
}

// Validate implements ops.Validator.
func (in *ExecuteInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.RestrictionID == uuid.Nil {
		fieldErrs[keyRestrictionID] = apperr.KeyRequired
	}

	if strings.TrimSpace(in.ConfirmedRulesVersion) == "" {
		fieldErrs["confirmedRulesVersion"] = apperr.KeyRequired
	}

	return fieldErrs
}

// lockCurrent loads a restriction for update and checks the expected version.
func lockCurrent(ctx context.Context, call *ops.Call, id uuid.UUID) (Restriction, error) {
	restriction, err := load(ctx, call, id, true)
	if err != nil {
		return restriction, err
	}

	if restriction.Version != *call.ExpectedVersion {
		return restriction, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	return restriction, nil
}

func causesPaid(ctx context.Context, c *ops.Call, id uuid.UUID) (bool, error) {
	var unpaid bool

	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM restrictions.restriction_invoices ri JOIN billing.invoices i ON
		i.id = ri.invoice_id
		WHERE ri.restriction_id = $1 AND i.status <> 'paid')`, id).
		Scan(&unpaid)

	return !unpaid, apperr.Wrap(err, "select restrictions.restriction_invoices")
}

// execute serves restrictions.execute.
//
//	@Summary		restrictions.execute (write)
//	@ID				restrictions.execute
//	@Description	Authorization: admin:restriction.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR90 device operation running: not_sent intent
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A09
//	@Tags			restrictions
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target restrictions, read restrictions.get)"
//	@Param			restrictionId		path		string			true	"input field restrictionId"
//	@Param			request				body		ExecuteInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Restriction}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/restrictions/{restrictionId}/execute [post]
func (m Restrictions) execute(
	ctx context.Context,
	call *ops.Call,
	input *ExecuteInput,
) (Restriction, error) {
	restriction, err := lockCurrent(ctx, call, input.RestrictionID)
	if err != nil {
		return restriction, err
	}

	if restriction.State != stateScheduled {
		return restriction, apperr.E(apperr.Conflict, "errors.restriction_state")
	}

	if input.ConfirmedRulesVersion != restriction.RulesVersion {
		return restriction, apperr.Fields(
			map[string]string{"confirmedRulesVersion": keyRulesVersionMismatch},
		)
	}

	if call.Now.Before(restriction.ExecuteAfter) ||
		call.Now.Before(restriction.NoticeAt.Add(24*time.Hour)) {
		return restriction, apperr.E(apperr.Conflict, "errors.restriction_not_due")
	}

	if (restriction.Exception != nil && call.Now.Before(restriction.Exception.Until)) ||
		(restriction.GraceUntil != nil && call.Now.Before(*restriction.GraceUntil)) {
		return restriction, apperr.E(apperr.Conflict, "errors.restriction_exempted")
	}

	var contractVersion int

	scanErr := call.Tx.QueryRow(ctx, `SELECT COALESCE((SELECT version FROM billing.contracts WHERE id = $1 AND is_current),
		0)`, restriction.ContractID).
		Scan(&contractVersion)
	if scanErr != nil {
		return restriction, apperr.Wrap(scanErr, "select billing.contracts")
	}

	evidence := 0 // the notices identity stored for the restriction at notice time (IR191)

	if len(restriction.NoticeNotificationIDs) > 0 {
		stored, err := ops.Query[int](
			ctx,
			call,
			identity.QueryNotificationsStored,
			identity.NotificationsStoredInput{
				IDs:        restriction.NoticeNotificationIDs,
				TargetID:   restriction.ID,
				OccurredAt: restriction.NoticeAt,
			},
		)
		if err != nil {
			return restriction, err
		}

		evidence = stored
	}

	if evidence == 0 || evidence != len(restriction.NoticeNotificationIDs) ||
		contractVersion != restriction.ContractVersion {
		return restriction, apperr.E(apperr.Conflict, "errors.notice_invalid")
	}

	paid, err := causesPaid(ctx, call, restriction.ID)
	if err != nil {
		return restriction, err
	}

	next := restriction.Version + 1
	if paid { // all causes paid right before execution: cancel without apply requests (DD-A09)
		_, err := call.Tx.Exec(
			ctx,
			`UPDATE restrictions.restrictions SET state = 'cancelled', version = version + 1, updated_at = $2 WHERE id = $1`,
			restriction.ID,
			call.Now,
		)
		if err != nil {
			return restriction, apperr.Wrap(err, "update restrictions.restrictions")
		}

		call.Emit(
			ops.Event{
				AggregateType: kindRestriction,
				AggregateID:   restriction.ID,
				Type:          "RestrictionCancelled",
				Payload:       map[string]any{keySource: "payment"},
			},
		)
		call.Audit(
			ops.AuditEntry{
				Action:          opExecute,
				TargetKind:      kindRestriction,
				TargetID:        restriction.ID.String(),
				PreviousVersion: call.ExpectedVersion,
				NextVersion:     &next,
				Reason:          "causes paid", Result: "", Before: nil, After: nil,
			},
		)

		return load(ctx, call, restriction.ID, false)
	}

	action, marshalErr := json.Marshal(
		map[string]any{
			keyKind:          actionApplyRestriction,
			keyRestrictionID: restriction.ID,
			keyRulesVersion:  restriction.RulesVersion,
			keyPolicy:        restriction.Policy,
		},
	)
	if marshalErr != nil {
		return Restriction{}, apperr.Wrap(marshalErr, "json.Marshal")
	}

	for _, unit := range restriction.PerUnit {
		cmd, delivered, reason, err := m.send(ctx, call, restriction.ID, unit.UnitID, action)
		if err != nil {
			return restriction, err
		}

		state := applyNotSent
		if delivered {
			state = applySentUnknown
		}

		_, execErr := call.Tx.Exec(
			ctx,
			`UPDATE restrictions.restriction_units SET apply_state = $3, apply_command_ids = apply_command_ids || $4::uuid,
				pending_reason = $5
			WHERE restriction_id = $1 AND unit_id = $2`,
			restriction.ID,
			unit.UnitID,
			state,
			cmd,
			reason,
		)
		if execErr != nil {
			return restriction, apperr.Wrap(execErr, "update restrictions.restriction_units")
		}
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE restrictions.restrictions SET state = 'requested', version = version + 1, updated_at = $2 WHERE id = $1`,
		restriction.ID,
		call.Now,
	)
	if execErr != nil {
		return restriction, apperr.Wrap(execErr, "update restrictions.restrictions")
	}

	call.Emit(
		ops.Event{
			AggregateType: kindRestriction,
			AggregateID:   restriction.ID,
			Type:          "RestrictionRequested",
			Payload:       map[string]any{keyUnitIDs: restriction.UnitIDs},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          opExecute,
			TargetKind:      kindRestriction,
			TargetID:        restriction.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &next, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return load(ctx, call, restriction.ID, false)
}

// send creates one restriction Command for a unit (D03): delivered when the bound device is online, powered and
// not running a device operation; otherwise an undelivered intent (status=requested, delivery=not_sent) with the
// pending reason. Restriction Commands bypass the normal-command mutual exclusion (IR90 REV19-018).
func (m Restrictions) send(
	ctx context.Context,
	call *ops.Call,
	restriction, unit uuid.UUID,
	action []byte,
) (uuid.UUID, bool, *string, error) {
	dev, conn, power, bound, err := m.Devices.BoundDevice(ctx, call, unit)
	if err != nil {
		return uuid.Nil, false, nil, apperr.Wrap(err, "restrictions.Devices.BoundDevice")
	}

	opBusy, err := m.Devices.OperationBusy(ctx, call, unit)
	if err != nil {
		return uuid.Nil, false, nil, apperr.Wrap(err, "restrictions.Devices.OperationBusy")
	}

	var reason *string

	switch {
	case !bound || conn != connectionOnline || power == powerOff:
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

	status, delivery, sentAt := "sent", "sent", &call.Now
	if reason != nil {
		status, delivery, sentAt = stateRequested, applyNotSent, nil
	}
	// the command row is equipment's (IR185): billing assigns the ID and publishes the decided command
	commandID := uuid.Must(uuid.NewV7())

	err = events.Publish(
		ctx,
		call.Tx,
		call.Principal.TenantID,
		"command",
		commandID,
		events.RestrictionCommandRequested,
		events.RestrictionCommand{
			CommandID:     commandID,
			UnitID:        unit,
			DeviceID:      device,
			ActorID:       SystemActor,
			Action:        action,
			RestrictionID: restriction,
			Status:        status,
			Delivery:      delivery,
			RequestedAt:   call.Now,
			SentAt:        sentAt,
			ExpiresAt:     call.Now.Add(IntentTTL),
			CorrelationID: call.CorrelationID,
		},
	)
	if err == nil { // billing's own record of the command (IR194)
		var parsedAction struct{ Kind string }

		_ = json.Unmarshal(action, &parsedAction)
		_, err = call.Tx.Exec(
			ctx,
			`INSERT INTO restrictions.restriction_commands (tenant_id, command_id, restriction_id, unit_id, kind, delivered,
				status, requested_at, updated_at)
			VALUES ($1, $2, $3, $4, $5, $6, 'requested', $7, $7)`,
			call.Principal.TenantID,
			commandID,
			restriction,
			unit,
			parsedAction.Kind,
			reason == nil,
			call.Now,
		)
	}

	if err == nil {
		call.Emit(
			ops.Event{
				AggregateType: "command",
				AggregateID:   commandID,
				Type:          "CommandRequested",
				Payload: map[string]any{
					"unitId":   unit,
					"deviceId": device,
					"action":   json.RawMessage(action),
					"delivery": delivery,
				},
			},
		)
	}

	return commandID, reason == nil, reason, apperr.Wrap(
		err,
		"insert restrictions.restriction_commands",
	)
}

// CancelInput is restrictions.cancel input.
type CancelInput struct {
	RestrictionID uuid.UUID `json:"restrictionId"`
	Reason        string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *CancelInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.RestrictionID == uuid.Nil {
		fieldErrs[keyRestrictionID] = apperr.KeyRequired
	}

	trimReason(&in.Reason, "reason", fieldErrs)

	return fieldErrs
}

// cancel serves restrictions.cancel.
//
//	@Summary		restrictions.cancel (write)
//	@ID				restrictions.cancel
//	@Description	Authorization: admin:restriction.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR87 reason 1-1000; IR96 state table; requested/applied to release_requested with \
//	@Description	releaseIntent=cancel
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A10
//	@Tags			restrictions
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target restrictions, read restrictions.get)"
//	@Param			restrictionId		path		string		true	"input field restrictionId"
//	@Param			request				body		CancelInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Restriction}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/restrictions/{restrictionId}/cancel [post]
func (m Restrictions) cancel(
	ctx context.Context,
	call *ops.Call,
	input *CancelInput,
) (Restriction, error) {
	restriction, err := load(ctx, call, input.RestrictionID, true)
	if err != nil {
		return restriction, err
	}

	switch restriction.State {
	case stateReleaseRequested: // IR96: idempotent, no version or audit
		return restriction, nil
	case stateReleased, stateCancelled:
		return restriction, apperr.E(apperr.Conflict, "errors.restriction_state")
	}

	if restriction.Version != *call.ExpectedVersion {
		return restriction, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	next := restriction.Version + 1
	if restriction.State == stateScheduled {
		_, err := call.Tx.Exec(
			ctx,
			`UPDATE restrictions.restrictions SET state = 'cancelled', version = version + 1, updated_at = $2 WHERE id = $1`,
			restriction.ID,
			call.Now,
		)
		if err != nil {
			return restriction, apperr.Wrap(err, "update restrictions.restrictions")
		}

		call.Emit(
			ops.Event{
				AggregateType: kindRestriction,
				AggregateID:   restriction.ID,
				Type:          "RestrictionCancelled",
				Payload:       map[string]any{keySource: "cancel"},
			},
		)
	} else {
		err := m.RequestRelease(ctx, call, restriction.ID, "cancel")
		if err != nil {
			return restriction, err
		}
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "restrictions.cancel",
			TargetKind:      kindRestriction,
			TargetID:        restriction.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &next,
			Reason:          input.Reason, Result: "", Before: nil, After: nil,
		},
	)

	return load(ctx, call, restriction.ID, false)
}
