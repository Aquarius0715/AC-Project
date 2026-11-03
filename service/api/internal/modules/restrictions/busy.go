// Package restrictions implements the Restrictions module.
package restrictions

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Busy reports an active restriction (requested / applied / release_requested) on a unit, which blocks rebinding
// its device (D05, SR24).
type Busy struct{}

// Internal queries behind Busy for equipment (IR193, IR194).
const (
	QueryUnitBusy        = "restrictions.unitBusy"
	QueryUnitPolicy      = "restrictions.unitPolicy"
	QueryUnitRestriction = "restrictions.unitRestriction"
	QueryUnitRecovering  = "restrictions.unitRecovering"
)

// UnitRestriction is the restriction that currently restricts a unit (IR46): its phase (state) and policy.
type UnitRestriction struct {
	Phase  string          `json:"phase"`
	Policy json.RawMessage `json:"policy" swaggertype:"object"`
	Found  bool            `json:"found"`
}

// UnitInput names a unit.
type UnitInput struct {
	UnitID uuid.UUID `json:"unitId"`
}

// RegisterQueries binds the restriction queries other domains ask.
func RegisterQueries(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainBilling, QueryUnitBusy, unitBusy)
	ops.RegisterQuery(r, ops.DomainBilling, QueryUnitPolicy, unitPolicy)
	ops.RegisterQuery(r, ops.DomainBilling, QueryUnitRestriction, unitRestriction)
	ops.RegisterQuery(r, ops.DomainBilling, QueryUnitRecovering, unitRecovering)
}

// UnitRestriction returns the newest restriction that currently restricts the unit (unit detail, IR46).
func (Busy) UnitRestriction(
	ctx context.Context,
	c *ops.Call,
	unit uuid.UUID,
) (UnitRestriction, error) {
	out, err := ops.Delegate(ctx, c, QueryUnitRestriction, UnitInput{UnitID: unit}, unitRestriction)

	return out, apperr.Wrap(err, "delegate restrictions.unitRestriction")
}

func unitRestriction(ctx context.Context, c *ops.Call, input *UnitInput) (UnitRestriction, error) {
	var restriction UnitRestriction

	err := c.Tx.QueryRow(ctx, `SELECT r.state, r.policy FROM restrictions.restriction_units ru JOIN
		restrictions.restrictions r ON r.id = ru.restriction_id
		WHERE ru.unit_id = $1 AND r.state IN ('requested','applied','release_requested') ORDER BY r.created_at DESC LIMIT
		1`, input.UnitID).
		Scan(&restriction.Phase, &restriction.Policy)
	if errors.Is(err, pgx.ErrNoRows) {
		return UnitRestriction{Phase: "", Policy: nil, Found: false}, nil
	}

	restriction.Found = err == nil

	return restriction, apperr.Wrap(err, "select restrictions.restriction_units")
}

// UnitRecovering reports an unresolved terminal-restriction recovery case of the unit, which blocks ordinary control
// (SR26).
func (Busy) UnitRecovering(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	out, err := ops.Delegate(ctx, c, QueryUnitRecovering, UnitInput{UnitID: unit}, unitRecovering)

	return out, apperr.Wrap(err, "delegate restrictions.unitRecovering")
}

func unitRecovering(ctx context.Context, c *ops.Call, in *UnitInput) (bool, error) {
	var blocked bool

	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM restrictions.restrictions r,
		jsonb_array_elements(r.recovery_cases) k
		WHERE k->>'unitId' = $1::text AND k->>'state' <> 'resolved')`, in.UnitID).
		Scan(&blocked)

	return blocked, apperr.Wrap(err, "select restrictions.restrictions")
}

// UnitBusy implements devices.Exclusion (asks billing, IR193).
func (Busy) UnitBusy(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	out, err := ops.Delegate(ctx, c, QueryUnitBusy, UnitInput{UnitID: unit}, unitBusy)

	return out, apperr.Wrap(err, "delegate restrictions.unitBusy")
}

func unitBusy(ctx context.Context, c *ops.Call, in *UnitInput) (bool, error) {
	var busy bool

	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM restrictions.restriction_units ru JOIN
		restrictions.restrictions r ON r.id = ru.restriction_id
		WHERE ru.unit_id = $1 AND r.state IN ('requested','applied','release_requested'))`, in.UnitID).
		Scan(&busy)

	return busy, apperr.Wrap(err, "select restrictions.restriction_units")
}

// UnitActive implements assets.UnitActivity (same rule as UnitBusy).
func (b Busy) UnitActive(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	return b.UnitBusy(ctx, c, unit)
}

// ContractState returns the contract's active restrictions and whether a recovery case is unresolved (IR135 item 1).
func (Busy) ContractState(
	ctx context.Context,
	c *ops.Call,
	contract uuid.UUID,
) ([]uuid.UUID, bool, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT id, state IN ('scheduled','requested','applied','release_requested'),
		jsonb_path_exists(recovery_cases, '$[*] ? (@.status != "resolved")') FROM restrictions.restrictions WHERE contract_id
		= $1 ORDER BY id`,
		contract,
	)
	if err != nil {
		return nil, false, apperr.Wrap(err, "select restrictions.restrictions")
	}
	defer rows.Close()

	active, unresolved := []uuid.UUID{}, false

	for rows.Next() {
		var (
			restrictionID  uuid.UUID
			isActive, open bool
		)

		err := rows.Scan(&restrictionID, &isActive, &open)
		if err != nil {
			return nil, false, apperr.Wrap(err, "select restrictions.restrictions")
		}

		if isActive {
			active = append(active, restrictionID)
		}

		unresolved = unresolved || open
	}

	return active, unresolved, apperr.Wrap(rows.Err(), "select restrictions.restrictions")
}

// ForInvoice returns the restrictions citing an invoice.
func (Busy) ForInvoice(ctx context.Context, c *ops.Call, invoice uuid.UUID) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT DISTINCT restriction_id FROM restrictions.restriction_invoices WHERE invoice_id = $1 ORDER BY restriction_id`,
		invoice,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select restrictions.restriction_invoices")
	}
	defer rows.Close()

	out := []uuid.UUID{}

	for rows.Next() {
		var restrictionID uuid.UUID

		err := rows.Scan(&restrictionID)
		if err != nil {
			return nil, apperr.Wrap(err, "select restrictions.restriction_invoices")
		}

		out = append(out, restrictionID)
	}

	return out, apperr.Wrap(rows.Err(), "select restrictions.restriction_invoices")
}

// OnInvoicePaid applies IR35 route ①: restrictions citing the invoice whose cause invoices are all paid move scheduled
// → cancelled and requested/applied → release_requested with a payment release intent. It returns the changed IDs.
func (m Restrictions) OnInvoicePaid(
	ctx context.Context,
	call *ops.Call,
	invoice uuid.UUID,
) ([]uuid.UUID, error) {
	// the restrictions citing the invoice are locked first, in ID order, so the payments of their other cause invoices
	// wait for each other: two at once each saw the other invoice still unpaid and both left the restriction (IR476)
	_, err := call.Tx.Exec(
		ctx,
		`SELECT 1 FROM restrictions.restrictions r WHERE r.state IN ('scheduled','requested','applied')
		AND EXISTS (SELECT 1 FROM restrictions.restriction_invoices ri WHERE ri.restriction_id = r.id AND ri.invoice_id = $1)
		ORDER BY r.id FOR UPDATE OF r`,
		invoice,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select restrictions.restrictions")
	}

	rows, err := call.Tx.Query(ctx, `UPDATE restrictions.restrictions r SET
			state = CASE WHEN r.state = 'scheduled' THEN 'cancelled' ELSE 'release_requested' END,
			release_intent = CASE WHEN r.state = 'scheduled' THEN r.release_intent ELSE jsonb_build_object('source', 'payment',
			'at', $2::timestamptz, 'actorMembershipId', $3::uuid) END,
			version = r.version + 1, updated_at = platform.app_now()
		WHERE r.state IN ('scheduled','requested','applied')
		  AND EXISTS (SELECT 1 FROM restrictions.restriction_invoices ri WHERE ri.restriction_id = r.id AND ri.invoice_id =
		  $1)
		  AND NOT EXISTS (SELECT 1 FROM restrictions.restriction_invoices ri JOIN billing.invoices i ON i.id = ri.invoice_id
			WHERE ri.restriction_id = r.id AND i.status <> 'paid')
		RETURNING r.id`, invoice, call.Now, call.Principal.MembershipID)
	if err != nil {
		return nil, apperr.Wrap(err, "update restrictions.restrictions")
	}
	defer rows.Close()

	out := []uuid.UUID{}

	for rows.Next() {
		var restrictionID uuid.UUID

		err := rows.Scan(&restrictionID)
		if err != nil {
			return nil, apperr.Wrap(err, "update restrictions.restrictions")
		}

		out = append(out, restrictionID)
	}

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "update restrictions.restrictions")
	}

	for _, restrictionID := range out {
		call.Emit(
			ops.Event{
				AggregateType: "restriction",
				AggregateID:   restrictionID,
				Type:          "RestrictionReleaseRequested",
				Payload:       map[string]any{"source": "payment", "invoiceId": invoice},
			},
		)

		err := m.evaluateRelease(
			ctx,
			call,
			restrictionID,
		)
		if err != nil { // D03 release evaluation in the same transition (IR35 ①)
			return nil, err
		}

		call.Audit(
			ops.AuditEntry{
				Action:          "payments.release",
				TargetKind:      "restriction",
				TargetID:        restrictionID.String(),
				Reason:          "cause invoices paid",
				PreviousVersion: nil,
				NextVersion:     nil,
				Result:          "",
				Before:          nil,
				After:           nil,
			},
		)
	}

	return out, nil
}

// UnitPolicy returns the policy of the restriction that currently restricts a unit (requested / applied /
// release_requested), as raw RestrictionPolicy JSON; nil when the unit is unrestricted (IR46). Asks billing (IR193).
func (Busy) UnitPolicy(ctx context.Context, c *ops.Call, unit uuid.UUID) ([]byte, error) {
	out, err := ops.Delegate(ctx, c, QueryUnitPolicy, UnitInput{UnitID: unit}, unitPolicy)

	return out, apperr.Wrap(err, "delegate restrictions.unitPolicy")
}

func unitPolicy(ctx context.Context, c *ops.Call, input *UnitInput) ([]byte, error) {
	var policy []byte

	err := c.Tx.QueryRow(ctx, `SELECT r.policy FROM restrictions.restriction_units ru JOIN restrictions.restrictions r ON
		r.id = ru.restriction_id
		WHERE ru.unit_id = $1 AND r.state IN ('requested','applied','release_requested') ORDER BY r.created_at DESC LIMIT
		1`, input.UnitID).
		Scan(&policy)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}

	return policy, apperr.Wrap(err, "select restrictions.restriction_units")
}
