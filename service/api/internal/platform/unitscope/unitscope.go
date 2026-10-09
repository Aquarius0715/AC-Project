// Package unitscope is the one D01 unit read scope used by every module (IR152 table, IR49, IR124, IR169):
//
//   - admin: the tenant
//   - client: units of the caller's organization
//   - contractor: units of jobs with the company's accepted Offer inside its access window
//   - internal technician: units within Membership.scopes (organization, property, unit)
//   - external technician: Membership.scopes ∩ the caller's own active Assignment ∩ the accepted Offer's access window;
//     List mode uses the IR49 viewing window [createdAt, scheduledEnd), Equipment mode (telemetry.*, alerts.*,
//     devices.*, units.get — IR49(b)) the work window [scheduledStart, scheduledEnd)
//
// Offers and Assignments belong to maintenance; the scope reads equipment's own projections of them
// (assets.unit_offer_access, assets.unit_assignment_access), fed by change-capture events (IR186).
package unitscope

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// Mode selects the external technician's window.
type Mode int

const (
	List      Mode = iota // unit lists and pickers: viewing window
	Equipment             // equipment reads: work window
)

// SQL returns a predicate that is true when the unit with id unitExpr (a column or placeholder of the outer query)
// is readable by the caller. Arguments are appended to args.
func SQL(c *ops.Call, args *[]any, unitExpr string, mode Mode) string {
	p := c.Principal
	add := func(v any) string { *args = append(*args, v); return fmt.Sprintf("$%d", len(*args)) }
	unit := func(cond string) string {
		return "EXISTS (SELECT 1 FROM assets.units su WHERE su.id = " + unitExpr + " AND " + cond + ")"
	}
	switch p.Role {
	case "admin":
		return "TRUE"
	case "client":
		return unit("su.customer_org_id = " + add(p.OrgID))
	case "contractor":
		now := add(c.Now)
		return "EXISTS (SELECT 1 FROM assets.unit_offer_access so WHERE so.unit_id = " + unitExpr +
			" AND so.contractor_org_id = " + add(p.OrgID) + " AND so.access_valid_from <= " + now + " AND " + now + " < so.access_valid_until)"
	case "technician":
	default:
		return "FALSE"
	}
	var parts []string
	if ids := p.Scopes["organization"]; len(ids) > 0 {
		parts = append(parts, "su.customer_org_id = ANY("+add(ids)+")")
	}
	if ids := p.Scopes["property"]; len(ids) > 0 {
		parts = append(parts, "su.property_id = ANY("+add(ids)+")")
	}
	if ids := p.Scopes["unit"]; len(ids) > 0 {
		parts = append(parts, "su.id = ANY("+add(ids)+")")
	}
	if len(parts) == 0 {
		return "FALSE"
	}
	scope := unit("(" + strings.Join(parts, " OR ") + ")")
	if p.Employment == "internal" {
		return scope
	}
	return scope + " AND " + assignmentSQL(c, add, unitExpr, mode)
}

func assignmentSQL(c *ops.Call, add func(any) string, unitExpr string, mode Mode) string {
	now := add(c.Now)
	from := "sa.created_at"
	if mode == Equipment {
		from = "lower(sa.scheduled)"
	}
	return "EXISTS (SELECT 1 FROM assets.unit_assignment_access sa" +
		" JOIN assets.unit_offer_access so ON so.job_id = sa.job_id AND so.access_valid_from <= " + now + " AND " + now + " < so.access_valid_until" +
		" WHERE sa.unit_id = " + unitExpr + " AND sa.active AND sa.technician_membership_id = " + add(c.Principal.MembershipID) +
		" AND " + from + " <= " + now + " AND " + now + " < upper(sa.scheduled))"
}

// EquipmentReadable returns the units (already visible in List mode) whose equipment data the caller may read now: all of
// them except an external technician's units whose work window has not started (IR49(b)).
func EquipmentReadable(ctx context.Context, c *ops.Call, units []uuid.UUID) ([]uuid.UUID, error) {
	p := c.Principal
	if p.Role != "technician" || p.Employment == "internal" || len(units) == 0 {
		return units, nil
	}
	args := []any{units}
	rows, err := c.Tx.Query(ctx, "SELECT t.id FROM unnest($1::uuid[]) AS t(id) WHERE "+SQL(c, &args, "t.id", Equipment), args...)
	if err != nil {
		return nil, err
	}
	return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
}

// Gate applies IR49(b) to equipment reads of units the caller can already see in List mode: an external technician
// whose work window has not started gets FORBIDDEN errors.assignment_not_started. Everyone else passes.
func Gate(ctx context.Context, c *ops.Call, units []uuid.UUID) error {
	p := c.Principal
	if p.Role != "technician" || p.Employment == "internal" || len(units) == 0 {
		return nil
	}
	args := []any{units}
	q := "SELECT count(*) FROM unnest($1::uuid[]) AS t(id) WHERE NOT (" + SQL(c, &args, "t.id", Equipment) + ")"
	var waiting int
	if err := c.Tx.QueryRow(ctx, q, args...).Scan(&waiting); err != nil {
		return err
	}
	if waiting > 0 {
		return apperr.E(apperr.Forbidden, "errors.assignment_not_started")
	}
	return nil
}
