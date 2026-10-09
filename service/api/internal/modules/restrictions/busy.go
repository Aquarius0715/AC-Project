// Package restrictions implements the Restrictions module.
package restrictions

import (
	"context"
	"encoding/json"
	"errors"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
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
func (Busy) UnitRestriction(ctx context.Context, c *ops.Call, unit uuid.UUID) (UnitRestriction, error) {
	return ops.Delegate(ctx, c, QueryUnitRestriction, UnitInput{UnitID: unit}, unitRestriction)
}

func unitRestriction(ctx context.Context, c *ops.Call, in *UnitInput) (UnitRestriction, error) {
	var r UnitRestriction
	err := c.Tx.QueryRow(ctx, `SELECT r.state, r.policy FROM restrictions.restriction_units ru JOIN restrictions.restrictions r ON r.id = ru.restriction_id
		WHERE ru.unit_id = $1 AND r.state IN ('requested','applied','release_requested') ORDER BY r.created_at DESC LIMIT 1`, in.UnitID).Scan(&r.Phase, &r.Policy)
	if errors.Is(err, pgx.ErrNoRows) {
		return UnitRestriction{}, nil
	}
	r.Found = err == nil
	return r, err
}

// UnitRecovering reports an unresolved terminal-restriction recovery case of the unit, which blocks ordinary control
// (SR26).
func (Busy) UnitRecovering(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	return ops.Delegate(ctx, c, QueryUnitRecovering, UnitInput{UnitID: unit}, unitRecovering)
}

func unitRecovering(ctx context.Context, c *ops.Call, in *UnitInput) (bool, error) {
	var blocked bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM restrictions.restrictions r, jsonb_array_elements(r.recovery_cases) k
		WHERE k->>'unitId' = $1::text AND k->>'state' <> 'resolved')`, in.UnitID).Scan(&blocked)
	return blocked, err
}

// UnitBusy implements devices.Exclusion (asks billing, IR193).
func (Busy) UnitBusy(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	return ops.Delegate(ctx, c, QueryUnitBusy, UnitInput{UnitID: unit}, unitBusy)
}

func unitBusy(ctx context.Context, c *ops.Call, in *UnitInput) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM restrictions.restriction_units ru JOIN restrictions.restrictions r ON r.id = ru.restriction_id
		WHERE ru.unit_id = $1 AND r.state IN ('requested','applied','release_requested'))`, in.UnitID).Scan(&b)
	return b, err
}

// UnitActive implements assets.UnitActivity (same rule as UnitBusy).
func (b Busy) UnitActive(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	return b.UnitBusy(ctx, c, unit)
}

// ContractState returns the contract's active restrictions and whether a recovery case is unresolved (IR135 item 1).
func (Busy) ContractState(ctx context.Context, c *ops.Call, contract uuid.UUID) ([]uuid.UUID, bool, error) {
	rows, err := c.Tx.Query(ctx, `SELECT id, state IN ('scheduled','requested','applied','release_requested'),
		jsonb_path_exists(recovery_cases, '$[*] ? (@.status != "resolved")') FROM restrictions.restrictions WHERE contract_id = $1 ORDER BY id`, contract)
	if err != nil {
		return nil, false, err
	}
	defer rows.Close()
	active, unresolved := []uuid.UUID{}, false
	for rows.Next() {
		var id uuid.UUID
		var isActive, open bool
		if err := rows.Scan(&id, &isActive, &open); err != nil {
			return nil, false, err
		}
		if isActive {
			active = append(active, id)
		}
		unresolved = unresolved || open
	}
	return active, unresolved, rows.Err()
}

// ForInvoice returns the restrictions citing an invoice.
func (Busy) ForInvoice(ctx context.Context, c *ops.Call, invoice uuid.UUID) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, `SELECT DISTINCT restriction_id FROM restrictions.restriction_invoices WHERE invoice_id = $1 ORDER BY restriction_id`, invoice)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// OnInvoicePaid applies IR35 route ①: restrictions citing the invoice whose cause invoices are all paid move scheduled →
// cancelled and requested/applied → release_requested with a payment release intent. It returns the changed IDs.
func (m Restrictions) OnInvoicePaid(ctx context.Context, c *ops.Call, invoice uuid.UUID) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, `UPDATE restrictions.restrictions r SET
			state = CASE WHEN r.state = 'scheduled' THEN 'cancelled' ELSE 'release_requested' END,
			release_intent = CASE WHEN r.state = 'scheduled' THEN r.release_intent ELSE jsonb_build_object('source', 'payment', 'at', $2::timestamptz, 'actorMembershipId', $3::uuid) END,
			version = r.version + 1, updated_at = platform.app_now()
		WHERE r.state IN ('scheduled','requested','applied')
		  AND EXISTS (SELECT 1 FROM restrictions.restriction_invoices ri WHERE ri.restriction_id = r.id AND ri.invoice_id = $1)
		  AND NOT EXISTS (SELECT 1 FROM restrictions.restriction_invoices ri JOIN billing.invoices i ON i.id = ri.invoice_id
			WHERE ri.restriction_id = r.id AND i.status <> 'paid')
		RETURNING r.id`, invoice, c.Now, c.Principal.MembershipID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for _, id := range out {
		c.Emit(ops.Event{AggregateType: "restriction", AggregateID: id, Type: "RestrictionReleaseRequested", Payload: map[string]any{"source": "payment", "invoiceId": invoice}})
		if err := m.evaluateRelease(ctx, c, id); err != nil { // D03 release evaluation in the same transition (IR35 ①)
			return nil, err
		}
		c.Audit(ops.AuditEntry{Action: "payments.release", TargetKind: "restriction", TargetID: id.String(), Reason: "cause invoices paid"})
	}
	return out, nil
}

// UnitPolicy returns the policy of the restriction that currently restricts a unit (requested / applied /
// release_requested), as raw RestrictionPolicy JSON; nil when the unit is unrestricted (IR46). Asks billing (IR193).
func (Busy) UnitPolicy(ctx context.Context, c *ops.Call, unit uuid.UUID) ([]byte, error) {
	return ops.Delegate(ctx, c, QueryUnitPolicy, UnitInput{UnitID: unit}, unitPolicy)
}

func unitPolicy(ctx context.Context, c *ops.Call, in *UnitInput) ([]byte, error) {
	var p []byte
	err := c.Tx.QueryRow(ctx, `SELECT r.policy FROM restrictions.restriction_units ru JOIN restrictions.restrictions r ON r.id = ru.restriction_id
		WHERE ru.unit_id = $1 AND r.state IN ('requested','applied','release_requested') ORDER BY r.created_at DESC LIMIT 1`, in.UnitID).Scan(&p)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return p, err
}
