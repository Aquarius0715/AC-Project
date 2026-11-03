package restrictions

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// MaxException is the longest grace / exception period (IR141).
const MaxException = 90 * 24 * time.Hour

// ExceptionInput is restrictions.defer / restrictions.exempt input.
type ExceptionInput struct {
	RestrictionID uuid.UUID `json:"restrictionId"`
	Until         time.Time `json:"until"`
	Reason        string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *ExceptionInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.RestrictionID == uuid.Nil {
		fieldErrs[keyRestrictionID] = apperr.KeyRequired
	}

	if in.Until.IsZero() {
		fieldErrs["until"] = apperr.KeyRequired
	}

	trimReason(&in.Reason, fieldErrs)

	return fieldErrs
}

// deferRestriction serves restrictions.defer.
//
//	@Summary		restrictions.defer (write)
//	@ID				restrictions.defer
//	@Description	Authorization: admin:restriction.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR87 reason 1-1000
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A10
//	@Tags			restrictions
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target restrictions, read restrictions.get)"
//	@Param			restrictionId		path		string			true	"input field restrictionId"
//	@Param			request				body		ExceptionInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/restrictions/{restrictionId}/defer [post]
func (m Restrictions) deferRestriction(
	ctx context.Context,
	c *ops.Call,
	in *ExceptionInput,
) (Restriction, error) {
	return m.except(ctx, c, in, "restrictions.defer", `grace_until = $2, updated_at = $3`, in.Until)
}

// exempt serves restrictions.exempt.
//
//	@Summary		restrictions.exempt (write)
//	@ID				restrictions.exempt
//	@Description	Authorization: admin:restriction.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR87 reason 1-1000
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A10
//	@Tags			restrictions
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target restrictions, read restrictions.get)"
//	@Param			restrictionId		path		string			true	"input field restrictionId"
//	@Param			request				body		ExceptionInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/restrictions/{restrictionId}/exempt [post]
func (m Restrictions) exempt(
	ctx context.Context,
	c *ops.Call,
	input *ExceptionInput,
) (Restriction, error) {
	return m.except(
		ctx,
		c,
		input,
		"restrictions.exempt",
		`exception_until = $2, updated_at = $3, exception_reason = $4`,
		input.Until,
		input.Reason,
	)
}

// except records a grace period or exception (DD-A10): scheduled restrictions keep their state (execute is blocked
// until expiry); requested/applied move to release_requested with source=exception (IR35 ②); release_requested only
// records the period. Expiry never reapplies automatically (D03).
func (m Restrictions) except(
	ctx context.Context,
	call *ops.Call,
	input *ExceptionInput,
	action, set string,
	args ...any,
) (Restriction, error) {
	if !input.Until.After(call.Now) || input.Until.After(call.Now.Add(MaxException)) {
		return Restriction{}, apperr.Fields(map[string]string{"until": "errors.until_range"})
	}

	restriction, err := lockCurrent(ctx, call, input.RestrictionID)
	if err != nil {
		return restriction, err
	}

	if restriction.State == stateReleased || restriction.State == stateCancelled {
		return restriction, apperr.E(apperr.Conflict, "errors.restriction_state")
	}

	args = append([]any{restriction.ID, args[0], call.Now}, args[1:]...)

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE restrictions.restrictions SET `+set+`, version = version + 1 WHERE id = $1`,
		args...)
	if execErr != nil {
		return restriction, apperr.Wrap(execErr, "update restrictions.restrictions")
	}

	next := restriction.Version + 1
	if restriction.State == stateRequested || restriction.State == stateApplied {
		err := m.RequestRelease(ctx, call, restriction.ID, "exception")
		if err != nil {
			return restriction, err
		}

		next++
	}

	call.Emit(
		ops.Event{
			AggregateType: kindRestriction,
			AggregateID:   restriction.ID,
			Type:          "RestrictionExceptionRecorded",
			Payload:       map[string]any{"action": action, "until": input.Until},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          action,
			TargetKind:      kindRestriction,
			TargetID:        restriction.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &next,
			Reason:          input.Reason, Result: "", Before: nil, After: nil,
		},
	)

	return load(ctx, call, restriction.ID, false)
}

// ReleaseInput is restrictions.release input.
type ReleaseInput struct {
	RestrictionID uuid.UUID `json:"restrictionId"`
}

// Validate implements ops.Validator.
func (in *ReleaseInput) Validate() map[string]string {
	if in.RestrictionID == uuid.Nil {
		return map[string]string{keyRestrictionID: apperr.KeyRequired}
	}

	return nil
}

func exempted(x Restriction, now time.Time) bool {
	return (x.Exception != nil && now.Before(x.Exception.Until)) ||
		(x.GraceUntil != nil && now.Before(*x.GraceUntil))
}

// release serves restrictions.release.
//
//	@Summary		restrictions.release (write)
//	@ID				restrictions.release
//	@Description	Authorization: admin:restriction.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR35 idempotent when release_requested
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A09
//	@Tags			restrictions
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target restrictions, read restrictions.get)"
//	@Param			restrictionId		path		string			true	"input field restrictionId"
//	@Param			request				body		ReleaseInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/restrictions/{restrictionId}/release [post]
func (m Restrictions) release(
	ctx context.Context,
	call *ops.Call,
	in *ReleaseInput,
) (Restriction, error) {
	restriction, err := load(ctx, call, in.RestrictionID, true)
	if err != nil {
		return restriction, err
	}

	switch restriction.State {
	case stateReleaseRequested: // IR35: idempotent
		return restriction, nil
	case stateRequested, stateApplied:
	default:
		return restriction, apperr.E(apperr.Conflict, "errors.restriction_state")
	}

	if restriction.Version != *call.ExpectedVersion {
		return restriction, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	paid, err := causesPaid(ctx, call, restriction.ID)
	if err != nil {
		return restriction, err
	}

	if !paid && !exempted(restriction, call.Now) {
		return restriction, apperr.E(apperr.Forbidden, "errors.restriction_release_unpaid")
	}

	requestReleaseErr := m.RequestRelease(ctx, call, restriction.ID, "manual")
	if requestReleaseErr != nil {
		return restriction, requestReleaseErr
	}

	next := restriction.Version + 1
	call.Audit(
		ops.AuditEntry{
			Action:          "restrictions.release",
			TargetKind:      kindRestriction,
			TargetID:        restriction.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &next, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return load(ctx, call, restriction.ID, false)
}

// OverrideInput is restrictions.override input.
type OverrideInput struct {
	RestrictionID uuid.UUID `json:"restrictionId"`
	Reason        string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *OverrideInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.RestrictionID == uuid.Nil {
		fieldErrs[keyRestrictionID] = apperr.KeyRequired
	}

	trimReason(&in.Reason, fieldErrs)

	return fieldErrs
}

// override is the forced release (IR35 ③): requested/applied → release_requested with source=override; invoices
// stay unpaid. scheduled and terminal restrictions return CONFLICT (cancel a scheduled one); release_requested is
// idempotent. The response is always RestrictionReleaseView (IR03).
//
//	@Summary		restrictions.override (write)
//	@ID				restrictions.override
//	@Description	Authorization: admin:restriction.override
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR87 reason 1-1000
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A10
//	@Tags			restrictions
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target restrictions, read restrictions.get)"
//	@Param			restrictionId		path		string			true	"input field restrictionId"
//	@Param			request				body		OverrideInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=ReleaseView}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/restrictions/{restrictionId}/override [post]
func (m Restrictions) override(
	ctx context.Context,
	call *ops.Call,
	input *OverrideInput,
) (ReleaseView, error) {
	restriction, err := load(ctx, call, input.RestrictionID, true)
	if err != nil {
		return ReleaseView{}, err
	}

	switch restriction.State {
	case stateReleaseRequested:
		return releaseView(restriction), nil
	case stateRequested, stateApplied:
	default:
		return ReleaseView{}, apperr.E(apperr.Conflict, "errors.restriction_state")
	}

	if restriction.Version != *call.ExpectedVersion {
		return ReleaseView{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	requestReleaseErr := m.RequestRelease(ctx, call, restriction.ID, "override")
	if requestReleaseErr != nil {
		return ReleaseView{}, requestReleaseErr
	}

	next := restriction.Version + 1
	call.Audit(
		ops.AuditEntry{
			Action:          "restrictions.override",
			TargetKind:      kindRestriction,
			TargetID:        restriction.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &next,
			Reason:          input.Reason, Result: "", Before: nil, After: nil,
		},
	)
	restriction, err = load(ctx, call, restriction.ID, false)

	return releaseView(restriction), err
}

// overrideOnlyAllowed applies IR03: callers without restriction.write may reconcile / retry(release) only a
// release_requested restriction whose release intent is an override.
func overrideOnlyAllowed(c *ops.Call, restriction Restriction) bool {
	if c.Principal.Permissions["restriction.write"] {
		return true
	}

	var intent struct {
		Source string `json:"source"`
	}

	_ = json.Unmarshal(restriction.ReleaseIntent, &intent)
	if (restriction.State == stateReleased || restriction.State == stateCancelled) &&
		unresolvedCase(restriction) { // IR03: terminal recovery
		return true
	}

	return restriction.State == stateReleaseRequested && intent.Source == "override"
}

func unresolvedCase(x Restriction) bool {
	var cases []RecoveryCase

	_ = json.Unmarshal(x.RecoveryCases, &cases)
	for _, k := range cases {
		if k.State != caseResolved {
			return true
		}
	}

	return false
}

func unitsOf(restriction Restriction, ids []uuid.UUID) ([]Unit, error) {
	out := []Unit{}

	for _, id := range ids {
		idx := slices.IndexFunc(restriction.PerUnit, func(u Unit) bool { return u.UnitID == id })
		if idx < 0 {
			return nil, apperr.Fields(
				map[string]string{keyUnitIDs: "errors.unit_not_in_restriction"},
			)
		}

		out = append(out, restriction.PerUnit[idx])
	}

	return out, nil
}

// ReconcileInput is restrictions.reconcile input.
type ReconcileInput struct {
	RestrictionID uuid.UUID   `json:"restrictionId"`
	UnitIDs       []uuid.UUID `json:"unitIds"`
}

// Validate implements ops.Validator.
func (in *ReconcileInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.RestrictionID == uuid.Nil {
		fieldErrs[keyRestrictionID] = apperr.KeyRequired
	}

	if !uniqueIDs(in.UnitIDs) {
		fieldErrs[keyUnitIDs] = apperr.KeyInvalid
	}

	return fieldErrs
}

// observation is the unit's current restriction observation (ACUnit.observedRestriction).
type observation struct {
	RestrictionID *uuid.UUID `json:"restrictionId"`
	RulesVersion  string     `json:"rulesVersion"`
}

// reconcile stores fresh observation evidence for sent_unknown units (D03): the observation must be at most 30 s old
// (TIMEOUT) and the device online (OFFLINE). This restriction observed → applied (and, while releasing, a remove
// Command); no restriction observed → not_applied (and not_required); another restriction → CONFLICT.
//
//	@Summary		restrictions.reconcile (write)
//	@ID				restrictions.reconcile
//	@Description	Authorization: admin:restriction.write | \
//	@Description	admin:restriction.override:release-intent-or-terminal-recovery-only
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	SR26 terminal recovery-case handling
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A09, DD-A10
//	@Tags			restrictions
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target restrictions, read restrictions.get)"
//	@Param			restrictionId		path		string			true	"input field restrictionId"
//	@Param			request				body		ReconcileInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/restrictions/{restrictionId}/reconcile [post]
func (m Restrictions) reconcile(
	ctx context.Context,
	call *ops.Call,
	input *ReconcileInput,
) (any, error) {
	restriction, err := lockCurrent(ctx, call, input.RestrictionID)
	if err != nil {
		return nil, err
	}

	if !overrideOnlyAllowed(call, restriction) {
		return nil, apperr.E(apperr.Forbidden, apperr.KeyForbidden)
	}

	if restriction.State == stateReleased ||
		restriction.State == stateCancelled { // SR26 recovery of a terminal restriction
		return m.reconcileTerminal(ctx, call, restriction, input.UnitIDs)
	}

	if restriction.State != stateRequested && restriction.State != stateApplied &&
		restriction.State != stateReleaseRequested {
		return nil, apperr.E(apperr.Conflict, "errors.restriction_state")
	}

	targets, err := unitsOf(restriction, input.UnitIDs)
	if err != nil {
		return nil, err
	}

	remove, marshalErr := json.Marshal(
		map[string]any{
			keyKind:          actionRemoveRestriction,
			keyRestrictionID: restriction.ID,
			keyRulesVersion:  restriction.RulesVersion,
		},
	)
	if marshalErr != nil {
		return nil, apperr.Wrap(marshalErr, "json.Marshal")
	}

	for _, unit := range targets {
		raw, observed, err := m.freshObservation(ctx, call, restriction, unit)
		if err != nil {
			return nil, err
		}

		err = m.reconcileUnit(ctx, call, restriction, unit, raw, observed, remove)
		if err != nil {
			return nil, err
		}
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE restrictions.restrictions r SET version = version + 1, updated_at = $2,
		state = CASE WHEN r.state = 'requested' AND NOT EXISTS (SELECT 1 FROM restrictions.restriction_units ru WHERE
		ru.restriction_id = r.id AND ru.apply_state <> 'applied')
			THEN 'applied' ELSE r.state END WHERE r.id = $1`,
		restriction.ID,
		call.Now,
	)
	if execErr != nil {
		return nil, apperr.Wrap(execErr, "update restrictions.restrictions")
	}

	finishReleaseErr := finishRelease(ctx, call.Tx, restriction.ID, call.Now)
	if finishReleaseErr != nil {
		return nil, finishReleaseErr
	}

	next := restriction.Version + 1
	call.Audit(
		ops.AuditEntry{
			Action:          "restrictions.reconcile",
			TargetKind:      kindRestriction,
			TargetID:        restriction.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &next, Reason: "", Result: "", Before: nil, After: nil,
		},
	)
	restriction, err = load(ctx, call, restriction.ID, false)

	return project(call, restriction), err
}

// commandPending is whether a command of the restriction for the unit is still requested.
func commandPending(
	ctx context.Context,
	call *ops.Call,
	restriction, unit uuid.UUID,
) (bool, error) {
	var open bool

	err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM restrictions.restriction_commands WHERE restriction_id = $1
		AND unit_id = $2 AND status = 'requested')`, restriction, unit).
		Scan(&open)
	if err != nil {
		return false, apperr.Wrap(err, "select restrictions.restriction_commands")
	}

	return open, nil
}

// freshObservation is the current observation of a sent_unknown unit that can be reconciled now (D03): no command of it
// is still requested.
func (m Restrictions) freshObservation(
	ctx context.Context,
	call *ops.Call,
	restriction Restriction,
	unit Unit,
) ([]byte, observation, error) {
	if unit.ApplyState != applySentUnknown {
		return nil, observation{}, apperr.Fields(
			map[string]string{keyUnitIDs: "errors.reconcile_not_needed"},
		)
	}

	open, err := commandPending(ctx, call, restriction.ID, unit.UnitID)
	if err != nil {
		return nil, observation{}, err
	}

	if open {
		return nil, observation{}, apperr.E(apperr.Conflict, "errors.command_pending")
	}

	raw, err := m.currentObservation(ctx, call, unit.UnitID)
	if err != nil {
		return nil, observation{}, err
	}

	var observed observation
	if len(raw) > 0 {
		err := json.Unmarshal(raw, &observed)
		if err != nil {
			return nil, observation{}, apperr.Wrap(err, "json.Unmarshal")
		}
	}

	return raw, observed, nil
}

// currentObservation is the equipment's observation of a unit (IR194) whose device is bound, online and powered
// (OFFLINE), taken at most IntentTTL ago (TIMEOUT, D03).
func (m Restrictions) currentObservation(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
) ([]byte, error) {
	_, conn, power, bound, err := m.Devices.BoundDevice(ctx, call, unit)
	if err != nil {
		return nil, apperr.Wrap(err, "restrictions.Devices.BoundDevice")
	}

	if !bound || conn != connectionOnline || power == powerOff {
		return nil, apperr.E(apperr.Offline, "errors.device_offline")
	}

	raw, observedAt, err := m.Units.Observation(ctx, call, unit)
	if err != nil {
		return nil, apperr.Wrap(err, "restrictions.Units.Observation")
	}

	if observedAt == nil || call.Now.Sub(*observedAt) > IntentTTL {
		return nil, apperr.E(apperr.Timeout, "errors.observation_stale")
	}

	return raw, nil
}

// reconcileUnit records a unit's fresh observation. Nothing observed means not applied; this restriction at its rules
// version means applied and, while the restriction is being released, the unit's remove command is sent; anything
// else is a mismatch.
func (m Restrictions) reconcileUnit(
	ctx context.Context,
	call *ops.Call,
	restriction Restriction,
	unit Unit,
	raw []byte,
	observed observation,
	remove []byte,
) error {
	releasing := unit.ReleaseState == releaseWaitingReconcile
	set := func(sql string, args ...any) error {
		_, err := call.Tx.Exec(
			ctx,
			`UPDATE restrictions.restriction_units SET
				`+sql+`, observed_at = $3, pending_reason = NULL WHERE restriction_id `+
				`= $1 AND unit_id = $2`,
			append([]any{restriction.ID, unit.UnitID, call.Now}, args...)...)

		return apperr.Wrap(err, "update restrictions.restriction_units")
	}

	switch {
	case observed.RestrictionID == nil && releasing:
		return set(
			`apply_state = 'not_applied', release_state = 'not_required', observed_restriction = NULL`,
		)
	case observed.RestrictionID == nil:
		return set(`apply_state = 'not_applied', observed_restriction = NULL`)
	case *observed.RestrictionID != restriction.ID || observed.RulesVersion != restriction.RulesVersion:
		return apperr.E(apperr.Conflict, "errors.observation_mismatch")
	}

	err := set(`apply_state = 'applied', observed_restriction = $4`, raw)
	if err != nil || !releasing {
		return err
	}

	cmd, delivered, _, err := m.send(ctx, call, restriction.ID, unit.UnitID, remove)
	if err != nil {
		return err
	}

	if !delivered {
		return apperr.E(apperr.Conflict, "errors.unit_busy")
	}

	return set(
		`release_state = 'requested', release_command_ids = release_command_ids || $4::uuid`,
		cmd,
	)
}

// RetryInput is restrictions.retry input.
type RetryInput struct {
	RestrictionID         uuid.UUID   `json:"restrictionId"`
	UnitIDs               []uuid.UUID `json:"unitIds"`
	Phase                 string      `json:"phase"`
	ConfirmedRulesVersion string      `json:"confirmedRulesVersion"`
	Reason                string      `json:"reason"`
}

// Validate implements ops.Validator.
func (in *RetryInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.RestrictionID == uuid.Nil {
		fieldErrs[keyRestrictionID] = apperr.KeyRequired
	}

	if !uniqueIDs(in.UnitIDs) {
		fieldErrs[keyUnitIDs] = apperr.KeyInvalid
	}

	if in.Phase != phaseApply && in.Phase != phaseRelease {
		fieldErrs["phase"] = apperr.KeyInvalid
	}

	if strings.TrimSpace(in.ConfirmedRulesVersion) == "" {
		fieldErrs["confirmedRulesVersion"] = apperr.KeyRequired
	}

	trimReason(&in.Reason, fieldErrs)

	return fieldErrs
}

// retry re-sends apply or remove Commands for selected units (D03). Units that already succeeded are returned
// unchanged; sent_unknown / waiting_reconcile units need reconcile first and an unfinished Command blocks a retry
// (CONFLICT). apply requires state requested with unpaid causes, no grace/exception and valid notice evidence; it
// creates delivered Commands or not_sent intents. release requires release_requested; an offline unit returns
// OFFLINE with no side effects.
//
//	@Summary		restrictions.retry (write)
//	@ID				restrictions.retry
//	@Description	Authorization: admin:restriction.write | \
//	@Description	admin:restriction.override:phase=release:release-intent-or-terminal-recovery-only
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A09, DD-A10
//	@Tags			restrictions
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target restrictions, read restrictions.get)"
//	@Param			restrictionId		path		string		true	"input field restrictionId"
//	@Param			request				body		RetryInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/restrictions/{restrictionId}/retry [post]
func (m Restrictions) retry(ctx context.Context, call *ops.Call, input *RetryInput) (any, error) {
	restriction, err := lockCurrent(ctx, call, input.RestrictionID)
	if err != nil {
		return nil, err
	}

	if input.Phase == phaseApply && !call.Principal.Permissions["restriction.write"] ||
		input.Phase == phaseRelease && !overrideOnlyAllowed(call, restriction) {
		return nil, apperr.E(apperr.Forbidden, apperr.KeyForbidden)
	}

	if input.ConfirmedRulesVersion != restriction.RulesVersion {
		return nil, apperr.Fields(
			map[string]string{"confirmedRulesVersion": keyRulesVersionMismatch},
		)
	}

	targets, err := unitsOf(restriction, input.UnitIDs)
	if err != nil {
		return nil, err
	}

	err = retryAllowed(ctx, call, restriction, input.Phase)
	if err != nil {
		return nil, err
	}

	action, err := commandAction(restriction, input.Phase)
	if err != nil {
		return nil, err
	}

	retryUnit := m.retryRelease
	if input.Phase == phaseApply {
		retryUnit = m.retryApply
	}

	changed := false

	for _, unit := range targets {
		retried, err := retryUnit(ctx, call, restriction, unit, action)
		if err != nil {
			return nil, err
		}

		changed = changed || retried
	}

	if !changed { // every target already succeeded: existing result, no resend
		return project(call, restriction), nil
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE restrictions.restrictions SET version = version + 1, updated_at = $2 WHERE id = $1`,
		restriction.ID,
		call.Now,
	)
	if execErr != nil {
		return nil, apperr.Wrap(execErr, "update restrictions.restrictions")
	}

	next := restriction.Version + 1
	call.Audit(
		ops.AuditEntry{
			Action:          "restrictions.retry",
			TargetKind:      kindRestriction,
			TargetID:        restriction.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &next,
			Reason:          input.Reason, Result: "", Before: nil, After: nil,
		},
	)
	restriction, err = load(ctx, call, restriction.ID, false)

	return project(call, restriction), err
}

// retryAllowed checks the restriction's state for a retry: an apply retry needs a requested restriction whose causes
// are unpaid and that exempts no one now, a release retry a restriction being released.
func retryAllowed(
	ctx context.Context,
	call *ops.Call,
	restriction Restriction,
	phase string,
) error {
	if phase != phaseApply {
		if restriction.State != stateReleaseRequested {
			return apperr.E(apperr.Conflict, "errors.restriction_state")
		}

		return nil
	}

	if restriction.State != stateRequested {
		return apperr.E(apperr.Conflict, "errors.restriction_state")
	}

	paid, err := causesPaid(ctx, call, restriction.ID)
	if err != nil {
		return err
	}

	if paid || exempted(restriction, call.Now) {
		return apperr.E(apperr.Conflict, "errors.restriction_not_applicable")
	}

	return nil
}

// commandAction is the command of a phase: the restriction to apply with its policy, or its removal.
func commandAction(restriction Restriction, phase string) ([]byte, error) {
	action := map[string]any{
		keyKind:          actionApplyRestriction,
		keyRestrictionID: restriction.ID,
		keyRulesVersion:  restriction.RulesVersion,
		keyPolicy:        restriction.Policy,
	}
	if phase == phaseRelease {
		action = map[string]any{
			keyKind:          actionRemoveRestriction,
			keyRestrictionID: restriction.ID,
			keyRulesVersion:  restriction.RulesVersion,
		}
	}

	encoded, err := json.Marshal(action)

	return encoded, apperr.Wrap(err, "json.Marshal")
}

// retryApply sends the apply command again to a unit that has not applied the restriction and reports whether it did.
// A unit that applied it is left as it is, one whose result is unknown must be reconciled first, and one with a
// command still requested waits for it.
func (m Restrictions) retryApply(
	ctx context.Context,
	call *ops.Call,
	restriction Restriction,
	unit Unit,
	action []byte,
) (bool, error) {
	open, err := commandPending(ctx, call, restriction.ID, unit.UnitID)
	if err != nil {
		return false, err
	}

	switch {
	case unit.ApplyState == stateApplied:
		return false, nil
	case unit.ApplyState == applySentUnknown:
		return false, apperr.E(apperr.Conflict, "errors.reconcile_required")
	case open:
		return false, apperr.E(apperr.Conflict, "errors.command_pending")
	}

	err = m.requestApply(ctx, call, restriction.ID, unit.UnitID, action)
	if err != nil {
		return false, err
	}

	return true, nil
}

// retryRelease sends the remove command again to a unit that still applies the restriction and reports whether it
// did. A released unit, or one that needs no release, is left as it is.
func (m Restrictions) retryRelease(
	ctx context.Context,
	call *ops.Call,
	restriction Restriction,
	unit Unit,
	action []byte,
) (bool, error) {
	open, err := commandPending(ctx, call, restriction.ID, unit.UnitID)
	if err != nil {
		return false, err
	}

	switch {
	case unit.ReleaseState == stateReleased || unit.ReleaseState == "not_required":
		return false, nil
	case unit.ReleaseState == releaseWaitingReconcile:
		return false, apperr.E(apperr.Conflict, "errors.reconcile_required")
	case unit.ReleaseState == stateRequested || open:
		return false, apperr.E(apperr.Conflict, "errors.command_pending")
	case unit.ApplyState != stateApplied:
		return false, apperr.E(apperr.Conflict, "errors.reconcile_required")
	}

	cmd, delivered, _, err := m.send(ctx, call, restriction.ID, unit.UnitID, action)
	if err != nil {
		return false, err
	}

	if !delivered {
		return false, apperr.E(apperr.Offline, "errors.device_offline")
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE restrictions.restriction_units SET release_state = 'requested', release_command_ids = release_command_ids ||
			$3::uuid, pending_reason = NULL
		WHERE restriction_id = $1 AND unit_id = $2`,
		restriction.ID,
		unit.UnitID,
		cmd,
	)
	if err != nil {
		return false, apperr.Wrap(err, "update restrictions.restriction_units")
	}

	return true, nil
}

// RegisterActions binds the exception, release and recovery operations.
func RegisterActions(reg *ops.Registry, module Restrictions) {
	ops.Register(reg, "restrictions.defer", module.deferRestriction)
	ops.Register(reg, "restrictions.exempt", module.exempt)
	ops.Register(reg, "restrictions.release", module.release)
	ops.Register(reg, "restrictions.override", module.override)
	ops.Register(reg, "restrictions.reconcile", module.reconcile)
	ops.Register(reg, "restrictions.retry", module.retry)
}
