package identity

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// Internal queries behind Directory (IR192).
const (
	QueryNonReaders = "identity.nonReaders"
	QueryOrgState   = "identity.orgState"
	QueryOrgName    = "identity.orgName"
	QueryTechnician = "identity.technician"
	QueryQualified  = "identity.qualified"
)

// NonReadersInput is QueryNonReaders input.
type NonReadersInput struct {
	IDs           []uuid.UUID `json:"ids"`
	CustomerOrgID uuid.UUID   `json:"customerOrgId"`
}

// OrgInput names an organization.
type OrgInput struct {
	OrgID uuid.UUID `json:"orgId"`
}

// OrgStateResult is QueryOrgState output.
type OrgStateResult struct {
	Kind   string `json:"kind"`
	Status string `json:"status"`
	Found  bool   `json:"found"`
}

// MembershipInput names a membership.
type MembershipInput struct {
	MembershipID uuid.UUID `json:"membershipId"`
}

// TechnicianResult is QueryTechnician output.
type TechnicianResult struct {
	Technician
	Found bool `json:"found"`
}

// QualifiedInput is QueryQualified input.
type QualifiedInput struct {
	MembershipID uuid.UUID `json:"membershipId"`
	Codes        []string  `json:"codes"`
	Start        time.Time `json:"start"`
	End          time.Time `json:"end"`
}

func registerDirectory(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainIdentity, QueryNonReaders, nonReaders)
	ops.RegisterQuery(r, ops.DomainIdentity, QueryOrgState, orgState)
	ops.RegisterQuery(r, ops.DomainIdentity, QueryOrgName, orgName)
	ops.RegisterQuery(r, ops.DomainIdentity, QueryTechnician, technician)
	ops.RegisterQuery(r, ops.DomainIdentity, QueryQualified, qualified)
}

func nonReaders(ctx context.Context, c *ops.Call, in *NonReadersInput) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, `SELECT m.id FROM identity.memberships m
		WHERE m.id = ANY($1) AND m.valid_from <= $3 AND (m.valid_until IS NULL OR m.valid_until > $3)
		  AND ((m.role = 'admin' AND EXISTS (SELECT 1 FROM identity.membership_permissions p WHERE p.membership_id = m.id AND p.permission = 'alert.read'))
		    OR (m.role = 'client' AND m.organization_id = $2))`, in.IDs, in.CustomerOrgID, c.Now)
	if err != nil {
		return nil, err
	}
	ok := map[uuid.UUID]bool{}
	readers, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, err
	}
	for _, id := range readers {
		ok[id] = true
	}
	bad := []uuid.UUID{}
	for _, id := range in.IDs {
		if !ok[id] {
			bad = append(bad, id)
		}
	}
	return bad, nil
}

func orgState(ctx context.Context, c *ops.Call, in *OrgInput) (OrgStateResult, error) {
	var r OrgStateResult
	err := c.Tx.QueryRow(ctx, `SELECT kind, status FROM identity.organizations WHERE id = $1`, in.OrgID).Scan(&r.Kind, &r.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgStateResult{}, nil
	}
	r.Found = err == nil
	return r, err
}

func orgName(ctx context.Context, c *ops.Call, in *OrgInput) (string, error) {
	var name string
	err := c.Tx.QueryRow(ctx, `SELECT name FROM identity.organizations WHERE id = $1`, in.OrgID).Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.E(apperr.NotFound, "error.notFound")
	}
	return name, err
}

func technician(ctx context.Context, c *ops.Call, in *MembershipInput) (TechnicianResult, error) {
	var t TechnicianResult
	var emp *string
	var from time.Time
	var until *time.Time
	err := c.Tx.QueryRow(ctx, `SELECT organization_id, employment, valid_from, valid_until FROM identity.memberships WHERE id = $1 AND role = 'technician'`, in.MembershipID).
		Scan(&t.OrgID, &emp, &from, &until)
	if errors.Is(err, pgx.ErrNoRows) {
		return TechnicianResult{}, nil
	}
	if err != nil {
		return t, err
	}
	if emp != nil {
		t.Employment = *emp
	}
	t.Active = !from.After(c.Now) && (until == nil || until.After(c.Now))
	t.Scopes = map[string][]uuid.UUID{}
	rows, err := c.Tx.Query(ctx, `SELECT kind, ref_id FROM identity.membership_scopes WHERE membership_id = $1`, in.MembershipID)
	if err != nil {
		return t, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var id uuid.UUID
		if err := rows.Scan(&k, &id); err != nil {
			return t, err
		}
		t.Scopes[k] = append(t.Scopes[k], id)
	}
	t.Found = true
	return t, rows.Err()
}

func qualified(ctx context.Context, c *ops.Call, in *QualifiedInput) (bool, error) {
	var n int
	err := c.Tx.QueryRow(ctx, `SELECT count(DISTINCT code) FROM identity.qualification_grants WHERE membership_id = $1 AND code = ANY($2)
		AND valid_from <= $3 AND valid_until >= $4 AND revoked_at IS NULL`, in.MembershipID, in.Codes, in.Start, in.End).Scan(&n)
	return n == len(in.Codes), err
}

// GrantHandlers apply QualificationGranted (consumer identity, IR192): the grant for the code covers
// [validFrom, validUntil) without revocation.
func GrantHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.QualificationGranted: func(ctx context.Context, tx pgx.Tx, e events.Event) error {
			var g events.Grant
			if err := e.Decode(&g); err != nil {
				return err
			}
			tag, err := tx.Exec(ctx, `UPDATE identity.qualification_grants SET valid_from = $3, valid_until = $4, revoked_at = NULL WHERE membership_id = $1 AND code = $2`,
				g.MembershipID, g.Code, g.ValidFrom, g.ValidUntil)
			if err != nil || tag.RowsAffected() > 0 {
				return err
			}
			_, err = tx.Exec(ctx, `INSERT INTO identity.qualification_grants (tenant_id, membership_id, code, valid_from, valid_until) VALUES ($1, $2, $3, $4, $5)`,
				e.TenantID, g.MembershipID, g.Code, g.ValidFrom, g.ValidUntil)
			return err
		},
	}
}
