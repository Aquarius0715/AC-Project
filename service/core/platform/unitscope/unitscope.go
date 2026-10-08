// Package unitscope is the one D01 unit read scope used by every module (IR152 table, IR49, IR124, IR169):
//
//   - admin: the tenant
//   - client: units of the caller's organization
//   - contractor: units of jobs with the company's accepted Offer inside its access window
//   - internal technician: units within Membership.scopes (organization, property, unit)
//   - external technician: Membership.scopes ∩ the caller's own active Assignment ∩ the accepted Offer's access window;
//     List mode uses the IR49 viewing window [createdAt, scheduledEnd), Equipment mode (telemetry.*, alerts.*,
//     devices.*, units.get — IR49(b)) the work window [scheduledStart, scheduledEnd)
package unitscope

import (
	"context"
	"fmt"
	"strings"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/core/ops"
	"github.com/pradita/ac-project/service/core/platform/apperr"
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
		return "EXISTS (SELECT 1 FROM maintenance.jobs sj JOIN maintenance.offers so ON so.job_id = sj.id WHERE sj.unit_id = " + unitExpr +
			" AND so.contractor_org_id = " + add(p.OrgID) + " AND so.decision = 'accept' AND so.access_valid_from <= " + now + " AND " + now + " < so.access_valid_until)"
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
	return "EXISTS (SELECT 1 FROM maintenance.jobs sj JOIN maintenance.assignments sa ON sa.job_id = sj.id" +
		" JOIN maintenance.offers so ON so.job_id = sj.id AND so.decision = 'accept' AND so.access_valid_from <= " + now + " AND " + now + " < so.access_valid_until" +
		" WHERE sj.unit_id = " + unitExpr + " AND sa.status = 'active' AND sa.technician_membership_id = " + add(c.Principal.MembershipID) +
		" AND " + from + " <= " + now + " AND " + now + " < upper(sa.scheduled))"
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
