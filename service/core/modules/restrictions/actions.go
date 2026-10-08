package restrictions

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/core/ops"
	"github.com/pradita/ac-project/service/core/platform/apperr"
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
	fe := map[string]string{}
	if in.RestrictionID == uuid.Nil {
		fe["restrictionId"] = "error.required"
	}
	if in.Until.IsZero() {
		fe["until"] = "error.required"
	}
	trimReason(&in.Reason, "reason", fe)
	return fe
}

func (m Restrictions) deferRestriction(ctx context.Context, c *ops.Call, in *ExceptionInput) (Restriction, error) {
	return m.except(ctx, c, in, "restrictions.defer", `grace_until = $2, updated_at = $3`, in.Until)
}

func (m Restrictions) exempt(ctx context.Context, c *ops.Call, in *ExceptionInput) (Restriction, error) {
	return m.except(ctx, c, in, "restrictions.exempt", `exception_until = $2, updated_at = $3, exception_reason = $4`, in.Until, in.Reason)
}

// except records a grace period or exception (DD-A10): scheduled restrictions keep their state (execute is blocked
// until expiry); requested/applied move to release_requested with source=exception (IR35 ②); release_requested only
// records the period. Expiry never reapplies automatically (D03).
func (m Restrictions) except(ctx context.Context, c *ops.Call, in *ExceptionInput, action, set string, args ...any) (Restriction, error) {
	if !in.Until.After(c.Now) || in.Until.After(c.Now.Add(MaxException)) {
		return Restriction{}, apperr.Fields(map[string]string{"until": "errors.until_range"})
	}
	x, err := lockCurrent(ctx, c, in.RestrictionID)
	if err != nil {
		return x, err
	}
	if x.State == "released" || x.State == "cancelled" {
		return x, apperr.E(apperr.Conflict, "errors.restriction_state")
	}
	args = append([]any{x.ID, args[0], c.Now}, args[1:]...)
	if _, err := c.Tx.Exec(ctx, `UPDATE restrictions.restrictions SET `+set+`, version = version + 1 WHERE id = $1`, args...); err != nil {
		return x, err
	}
	next := x.Version + 1
	if x.State == "requested" || x.State == "applied" {
		if err := m.RequestRelease(ctx, c, x.ID, "exception"); err != nil {
			return x, err
		}
		next++
	}
	c.Emit(ops.Event{AggregateType: "restriction", AggregateID: x.ID, Type: "RestrictionExceptionRecorded", Payload: map[string]any{"action": action, "until": in.Until}})
	c.Audit(ops.AuditEntry{Action: action, TargetKind: "restriction", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &next, Reason: in.Reason})
	return load(ctx, c, x.ID, false)
}

// ReleaseInput is restrictions.release input.
type ReleaseInput struct {
	RestrictionID uuid.UUID `json:"restrictionId"`
}

// Validate implements ops.Validator.
func (in *ReleaseInput) Validate() map[string]string {
	if in.RestrictionID == uuid.Nil {
		return map[string]string{"restrictionId": "error.required"}
	}
	return nil
}

func exempted(x Restriction, now time.Time) bool {
	return (x.Exception != nil && now.Before(x.Exception.Until)) || (x.GraceUntil != nil && now.Before(*x.GraceUntil))
}

func (m Restrictions) release(ctx context.Context, c *ops.Call, in *ReleaseInput) (Restriction, error) {
	x, err := load(ctx, c, in.RestrictionID, true)
	if err != nil {
		return x, err
	}
	switch x.State {
	case "release_requested": // IR35: idempotent
		return x, nil
	case "requested", "applied":
	default:
		return x, apperr.E(apperr.Conflict, "errors.restriction_state")
	}
	if x.Version != *c.ExpectedVersion {
		return x, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	paid, err := causesPaid(ctx, c, x.ID)
	if err != nil {
		return x, err
	}
	if !paid && !exempted(x, c.Now) {
		return x, apperr.E(apperr.Forbidden, "errors.restriction_release_unpaid")
	}
	if err := m.RequestRelease(ctx, c, x.ID, "manual"); err != nil {
		return x, err
	}
	next := x.Version + 1
	c.Audit(ops.AuditEntry{Action: "restrictions.release", TargetKind: "restriction", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &next})
	return load(ctx, c, x.ID, false)
}

// OverrideInput is restrictions.override input.
type OverrideInput struct {
	RestrictionID uuid.UUID `json:"restrictionId"`
	Reason        string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *OverrideInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.RestrictionID == uuid.Nil {
		fe["restrictionId"] = "error.required"
	}
	trimReason(&in.Reason, "reason", fe)
	return fe
}

// override is the forced release (IR35 ③): requested/applied → release_requested with source=override; invoices
// stay unpaid. scheduled and terminal restrictions return CONFLICT (cancel a scheduled one); release_requested is
// idempotent. The response is always RestrictionReleaseView (IR03).
func (m Restrictions) override(ctx context.Context, c *ops.Call, in *OverrideInput) (ReleaseView, error) {
	x, err := load(ctx, c, in.RestrictionID, true)
	if err != nil {
		return ReleaseView{}, err
	}
	switch x.State {
	case "release_requested":
		return releaseView(x), nil
	case "requested", "applied":
	default:
		return ReleaseView{}, apperr.E(apperr.Conflict, "errors.restriction_state")
	}
	if x.Version != *c.ExpectedVersion {
		return ReleaseView{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if err := m.RequestRelease(ctx, c, x.ID, "override"); err != nil {
		return ReleaseView{}, err
	}
	next := x.Version + 1
	c.Audit(ops.AuditEntry{Action: "restrictions.override", TargetKind: "restriction", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &next, Reason: in.Reason})
	x, err = load(ctx, c, x.ID, false)
	return releaseView(x), err
}

// overrideOnlyAllowed applies IR03: callers without restriction.write may reconcile / retry(release) only a
// release_requested restriction whose release intent is an override.
func overrideOnlyAllowed(c *ops.Call, x Restriction) bool {
	if c.Principal.Permissions["restriction.write"] {
		return true
	}
	var ri struct {
		Source string `json:"source"`
	}
	_ = json.Unmarshal(x.ReleaseIntent, &ri)
	if (x.State == "released" || x.State == "cancelled") && unresolvedCase(x) { // IR03: terminal recovery
		return true
	}
	return x.State == "release_requested" && ri.Source == "override"
}

func unresolvedCase(x Restriction) bool {
	var cases []RecoveryCase
	_ = json.Unmarshal(x.RecoveryCases, &cases)
	for _, k := range cases {
		if k.State != "resolved" {
			return true
		}
	}
	return false
}

func unitsOf(x Restriction, ids []uuid.UUID) ([]Unit, error) {
	out := []Unit{}
	for _, id := range ids {
		i := slices.IndexFunc(x.PerUnit, func(u Unit) bool { return u.UnitID == id })
		if i < 0 {
			return nil, apperr.Fields(map[string]string{"unitIds": "errors.unit_not_in_restriction"})
		}
		out = append(out, x.PerUnit[i])
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
	fe := map[string]string{}
	if in.RestrictionID == uuid.Nil {
		fe["restrictionId"] = "error.required"
	}
	if !uniqueIDs(in.UnitIDs) {
		fe["unitIds"] = "error.invalid"
	}
	return fe
}

// observation is the unit's current restriction observation (ACUnit.observedRestriction).
type observation struct {
	RestrictionID *uuid.UUID `json:"restrictionId"`
	RulesVersion  string     `json:"rulesVersion"`
}

// reconcile stores fresh observation evidence for sent_unknown units (D03): the observation must be at most 30 s old
// (TIMEOUT) and the device online (OFFLINE). This restriction observed → applied (and, while releasing, a remove
// Command); no restriction observed → not_applied (and not_required); another restriction → CONFLICT.
func (m Restrictions) reconcile(ctx context.Context, c *ops.Call, in *ReconcileInput) (any, error) {
	x, err := lockCurrent(ctx, c, in.RestrictionID)
	if err != nil {
		return nil, err
	}
	if !overrideOnlyAllowed(c, x) {
		return nil, apperr.E(apperr.Forbidden, "error.forbidden")
	}
	if x.State == "released" || x.State == "cancelled" { // SR26 recovery of a terminal restriction
		return m.reconcileTerminal(ctx, c, x, in.UnitIDs)
	}
	if x.State != "requested" && x.State != "applied" && x.State != "release_requested" {
		return nil, apperr.E(apperr.Conflict, "errors.restriction_state")
	}
	targets, err := unitsOf(x, in.UnitIDs)
	if err != nil {
		return nil, err
	}
	remove, _ := json.Marshal(map[string]any{"kind": "remove_restriction", "restrictionId": x.ID, "rulesVersion": x.RulesVersion})
	for _, u := range targets {
		if u.ApplyState != "sent_unknown" {
			return nil, apperr.Fields(map[string]string{"unitIds": "errors.reconcile_not_needed"})
		}
		var open bool
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM control.commands WHERE restriction_id = $1 AND unit_id = $2 AND status IN ('requested','sent'))`, x.ID, u.UnitID).Scan(&open); err != nil {
			return nil, err
		}
		if open {
			return nil, apperr.E(apperr.Conflict, "errors.command_pending")
		}
		_, conn, power, bound, err := m.Devices.BoundDevice(ctx, c, u.UnitID)
		if err != nil {
			return nil, err
		}
		if !bound || conn != "online" || power == "off" {
			return nil, apperr.E(apperr.Offline, "errors.device_offline")
		}
		var raw []byte
		var at *time.Time
		if err := c.Tx.QueryRow(ctx, `SELECT observed_restriction, last_seen_at FROM assets.units WHERE id = $1`, u.UnitID).Scan(&raw, &at); err != nil {
			return nil, err
		}
		if at == nil || c.Now.Sub(*at) > IntentTTL {
			return nil, apperr.E(apperr.Timeout, "errors.observation_stale")
		}
		var o observation
		if len(raw) > 0 {
			_ = json.Unmarshal(raw, &o)
		}
		releasing := u.ReleaseState == "waiting_reconcile"
		set := func(sql string, args ...any) error {
			_, err := c.Tx.Exec(ctx, `UPDATE restrictions.restriction_units SET `+sql+`, observed_at = $3, pending_reason = NULL WHERE restriction_id = $1 AND unit_id = $2`,
				append([]any{x.ID, u.UnitID, c.Now}, args...)...)
			return err
		}
		switch {
		case o.RestrictionID == nil:
			if releasing {
				err = set(`apply_state = 'not_applied', release_state = 'not_required', observed_restriction = NULL`)
			} else {
				err = set(`apply_state = 'not_applied', observed_restriction = NULL`)
			}
		case *o.RestrictionID == x.ID && o.RulesVersion == x.RulesVersion:
			if err = set(`apply_state = 'applied', observed_restriction = $4`, raw); err != nil {
				return nil, err
			}
			if releasing {
				cmd, delivered, _, err := m.send(ctx, c, x.ID, u.UnitID, remove)
				if err != nil {
					return nil, err
				}
				if !delivered {
					return nil, apperr.E(apperr.Conflict, "errors.unit_busy")
				}
				err = set(`release_state = 'requested', release_command_ids = release_command_ids || $4::uuid`, cmd)
			}
		default:
			return nil, apperr.E(apperr.Conflict, "errors.observation_mismatch")
		}
		if err != nil {
			return nil, err
		}
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE restrictions.restrictions r SET version = version + 1, updated_at = $2,
		state = CASE WHEN r.state = 'requested' AND NOT EXISTS (SELECT 1 FROM restrictions.restriction_units ru WHERE ru.restriction_id = r.id AND ru.apply_state <> 'applied')
			THEN 'applied' ELSE r.state END WHERE r.id = $1`, x.ID, c.Now); err != nil {
		return nil, err
	}
	if err := finishRelease(ctx, c.Tx, x.ID, c.Now); err != nil {
		return nil, err
	}
	next := x.Version + 1
	c.Audit(ops.AuditEntry{Action: "restrictions.reconcile", TargetKind: "restriction", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &next})
	x, err = load(ctx, c, x.ID, false)
	return project(c, x), err
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
	fe := map[string]string{}
	if in.RestrictionID == uuid.Nil {
		fe["restrictionId"] = "error.required"
	}
	if !uniqueIDs(in.UnitIDs) {
		fe["unitIds"] = "error.invalid"
	}
	if in.Phase != "apply" && in.Phase != "release" {
		fe["phase"] = "error.invalid"
	}
	if strings.TrimSpace(in.ConfirmedRulesVersion) == "" {
		fe["confirmedRulesVersion"] = "error.required"
	}
	trimReason(&in.Reason, "reason", fe)
	return fe
}

// retry re-sends apply or remove Commands for selected units (D03). Units that already succeeded are returned
// unchanged; sent_unknown / waiting_reconcile units need reconcile first and an unfinished Command blocks a retry
// (CONFLICT). apply requires state requested with unpaid causes, no grace/exception and valid notice evidence; it
// creates delivered Commands or not_sent intents. release requires release_requested; an offline unit returns
// OFFLINE with no side effects.
func (m Restrictions) retry(ctx context.Context, c *ops.Call, in *RetryInput) (any, error) {
	x, err := lockCurrent(ctx, c, in.RestrictionID)
	if err != nil {
		return nil, err
	}
	if in.Phase == "apply" && !c.Principal.Permissions["restriction.write"] || in.Phase == "release" && !overrideOnlyAllowed(c, x) {
		return nil, apperr.E(apperr.Forbidden, "error.forbidden")
	}
	if in.ConfirmedRulesVersion != x.RulesVersion {
		return nil, apperr.Fields(map[string]string{"confirmedRulesVersion": "errors.rules_version_mismatch"})
	}
	targets, err := unitsOf(x, in.UnitIDs)
	if err != nil {
		return nil, err
	}
	if in.Phase == "apply" {
		if x.State != "requested" {
			return nil, apperr.E(apperr.Conflict, "errors.restriction_state")
		}
		paid, err := causesPaid(ctx, c, x.ID)
		if err != nil {
			return nil, err
		}
		if paid || exempted(x, c.Now) {
			return nil, apperr.E(apperr.Conflict, "errors.restriction_not_applicable")
		}
	} else if x.State != "release_requested" {
		return nil, apperr.E(apperr.Conflict, "errors.restriction_state")
	}
	action, _ := json.Marshal(map[string]any{"kind": "apply_restriction", "restrictionId": x.ID, "rulesVersion": x.RulesVersion, "policy": x.Policy})
	if in.Phase == "release" {
		action, _ = json.Marshal(map[string]any{"kind": "remove_restriction", "restrictionId": x.ID, "rulesVersion": x.RulesVersion})
	}
	changed := false
	for _, u := range targets {
		var open bool
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM control.commands WHERE restriction_id = $1 AND unit_id = $2 AND status IN ('requested','sent'))`, x.ID, u.UnitID).Scan(&open); err != nil {
			return nil, err
		}
		if in.Phase == "apply" {
			switch {
			case u.ApplyState == "applied":
				continue
			case u.ApplyState == "sent_unknown":
				return nil, apperr.E(apperr.Conflict, "errors.reconcile_required")
			case open:
				return nil, apperr.E(apperr.Conflict, "errors.command_pending")
			}
			cmd, delivered, reason, err := m.send(ctx, c, x.ID, u.UnitID, action)
			if err != nil {
				return nil, err
			}
			state := "not_sent"
			if delivered {
				state = "sent_unknown"
			}
			if _, err := c.Tx.Exec(ctx, `UPDATE restrictions.restriction_units SET apply_state = $3, apply_command_ids = apply_command_ids || $4::uuid, pending_reason = $5
				WHERE restriction_id = $1 AND unit_id = $2`, x.ID, u.UnitID, state, cmd, reason); err != nil {
				return nil, err
			}
			changed = true
			continue
		}
		switch {
		case u.ReleaseState == "released" || u.ReleaseState == "not_required":
			continue
		case u.ReleaseState == "waiting_reconcile":
			return nil, apperr.E(apperr.Conflict, "errors.reconcile_required")
		case u.ReleaseState == "requested" || open:
			return nil, apperr.E(apperr.Conflict, "errors.command_pending")
		case u.ApplyState != "applied":
			return nil, apperr.E(apperr.Conflict, "errors.reconcile_required")
		}
		cmd, delivered, _, err := m.send(ctx, c, x.ID, u.UnitID, action)
		if err != nil {
			return nil, err
		}
		if !delivered {
			return nil, apperr.E(apperr.Offline, "errors.device_offline")
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE restrictions.restriction_units SET release_state = 'requested', release_command_ids = release_command_ids || $3::uuid, pending_reason = NULL
			WHERE restriction_id = $1 AND unit_id = $2`, x.ID, u.UnitID, cmd); err != nil {
			return nil, err
		}
		changed = true
	}
	if !changed { // every target already succeeded: existing result, no resend
		return project(c, x), nil
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE restrictions.restrictions SET version = version + 1, updated_at = $2 WHERE id = $1`, x.ID, c.Now); err != nil {
		return nil, err
	}
	next := x.Version + 1
	c.Audit(ops.AuditEntry{Action: "restrictions.retry", TargetKind: "restriction", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &next, Reason: in.Reason})
	x, err = load(ctx, c, x.ID, false)
	return project(c, x), err
}

// RegisterActions binds the exception, release and recovery operations.
func RegisterActions(r *ops.Registry, m Restrictions) {
	ops.Register(r, "restrictions.defer", m.deferRestriction)
	ops.Register(r, "restrictions.exempt", m.exempt)
	ops.Register(r, "restrictions.release", m.release)
	ops.Register(r, "restrictions.override", m.override)
	ops.Register(r, "restrictions.reconcile", m.reconcile)
	ops.Register(r, "restrictions.retry", m.retry)
}
