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

func nonReaders(ctx context.Context, c *ops.Call, input *NonReadersInput) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, `SELECT m.id FROM identity.memberships m
		WHERE m.id = ANY($1) AND m.valid_from <= $3 AND (m.valid_until IS NULL OR m.valid_until > $3)
		  AND ((m.role = 'admin' AND EXISTS (SELECT 1 FROM identity.membership_permissions p WHERE p.membership_id = m.id AND
		  p.permission = 'alert.read'))
		    OR (m.role = 'client' AND m.organization_id = $2))`, input.IDs, input.CustomerOrgID, c.Now)
	if err != nil {
		return nil, apperr.Wrap(err, "select identity.memberships")
	}

	ok := map[uuid.UUID]bool{}

	readers, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, apperr.Wrap(err, "select identity.memberships")
	}

	for _, id := range readers {
		ok[id] = true
	}

	bad := []uuid.UUID{}

	for _, id := range input.IDs {
		if !ok[id] {
			bad = append(bad, id)
		}
	}

	return bad, nil
}

func orgState(ctx context.Context, c *ops.Call, in *OrgInput) (OrgStateResult, error) {
	var orgStateResult OrgStateResult

	err := c.Tx.QueryRow(ctx, `SELECT kind, status FROM identity.organizations WHERE id = $1`, in.OrgID).
		Scan(&orgStateResult.Kind, &orgStateResult.Status)
	if errors.Is(err, pgx.ErrNoRows) {
		return OrgStateResult{Kind: "", Status: "", Found: false}, nil
	}

	orgStateResult.Found = err == nil

	return orgStateResult, apperr.Wrap(err, "select identity.organizations")
}

func orgName(ctx context.Context, c *ops.Call, in *OrgInput) (string, error) {
	var name string

	err := c.Tx.QueryRow(ctx, `SELECT name FROM identity.organizations WHERE id = $1`, in.OrgID).
		Scan(&name)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return name, apperr.Wrap(err, "select identity.organizations")
}

func technician(
	ctx context.Context,
	call *ops.Call,
	in *MembershipInput,
) (TechnicianResult, error) {
	var (
		technicianResult TechnicianResult
		emp              *string
		from             time.Time
		until            *time.Time
	)

	err := call.Tx.QueryRow(ctx, `SELECT organization_id, employment, valid_from, valid_until FROM identity.memberships
		WHERE
		id = $1 AND role = 'technician'`, in.MembershipID).
		Scan(&technicianResult.OrgID, &emp, &from, &until)
	if errors.Is(err, pgx.ErrNoRows) {
		return TechnicianResult{
			Technician: Technician{
				OrgID:      uuid.UUID{},
				Employment: "",
				Active:     false,
				ValidUntil: nil,
				Scopes:     nil,
			},
			Found: false,
		}, nil
	}

	if err != nil {
		return technicianResult, apperr.Wrap(err, "select identity.memberships")
	}

	if emp != nil {
		technicianResult.Employment = *emp
	}

	technicianResult.Active = !from.After(call.Now) && (until == nil || until.After(call.Now))
	technicianResult.ValidUntil = until
	technicianResult.Scopes = map[string][]uuid.UUID{}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT kind, ref_id FROM identity.membership_scopes WHERE membership_id = $1`,
		in.MembershipID,
	)
	if err != nil {
		return technicianResult, apperr.Wrap(err, "select identity.membership_scopes")
	}
	defer rows.Close()

	for rows.Next() {
		var (
			key string
			id  uuid.UUID
		)

		err := rows.Scan(&key, &id)
		if err != nil {
			return technicianResult, apperr.Wrap(err, "select identity.membership_scopes")
		}

		technicianResult.Scopes[key] = append(technicianResult.Scopes[key], id)
	}

	technicianResult.Found = true

	return technicianResult, apperr.Wrap(rows.Err(), "select identity.membership_scopes")
}

func qualified(ctx context.Context, c *ops.Call, input *QualifiedInput) (bool, error) {
	var n int

	err := c.Tx.QueryRow(ctx, `SELECT count(DISTINCT code) FROM identity.qualification_grants WHERE membership_id = $1 AND
		code = ANY($2)
		AND valid_from <= $3 AND valid_until >= $4 AND revoked_at IS
		NULL`, input.MembershipID, input.Codes, input.Start, input.End).
		Scan(&n)

	return n == len(input.Codes), apperr.Wrap(err, "select identity.qualification_grants")
}

// GrantHandlers apply QualificationGranted (consumer identity, IR192): the grant for the code covers
// [validFrom, validUntil) without revocation.
func GrantHandlers() map[string]events.Handler {
	return map[string]events.Handler{
		events.QualificationGranted: func(ctx context.Context, tx pgx.Tx, event events.Event) error {
			var grant events.Grant

			err := event.Decode(&grant)
			if err != nil {
				return apperr.Wrap(err, "events.Event.Decode")
			}

			tag, err := tx.Exec(
				ctx,
				`UPDATE identity.qualification_grants SET valid_from = $3, valid_until = $4, revoked_at = NULL WHERE membership_id =
					$1 AND code = $2`,
				grant.MembershipID,
				grant.Code,
				grant.ValidFrom,
				grant.ValidUntil,
			)
			if err != nil || tag.RowsAffected() > 0 {
				return apperr.Wrap(err, "update identity.qualification_grants")
			}

			_, err = tx.Exec(
				ctx,
				`INSERT INTO identity.qualification_grants (tenant_id, membership_id, code, valid_from, valid_until) VALUES ($1, $2,
					$3, $4, $5)`,
				event.TenantID,
				grant.MembershipID,
				grant.Code,
				grant.ValidFrom,
				grant.ValidUntil,
			)

			return apperr.Wrap(err, "insert identity.qualification_grants")
		},
	}
}
