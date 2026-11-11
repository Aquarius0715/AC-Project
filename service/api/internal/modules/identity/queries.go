package identity

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// QueryActiveOrganizations returns which of the given organizations are active (IR190; IR40 customer counts).
const QueryActiveOrganizations = "identity.activeOrganizations"

// OrganizationsInput is QueryActiveOrganizations input.
type OrganizationsInput struct {
	IDs []uuid.UUID `json:"ids"`
}

// RegisterQueries binds identity's internal queries.
func RegisterQueries(reg *ops.Registry) {
	ops.RegisterQuery(
		reg,
		ops.DomainIdentity,
		QueryActiveOrganizations,
		func(ctx context.Context, c *ops.Call, in *OrganizationsInput) ([]uuid.UUID, error) {
			rows, err := c.Tx.Query(
				ctx,
				`SELECT id FROM identity.organizations WHERE id = ANY($1) AND status = 'active' ORDER BY id`,
				in.IDs,
			)
			if err != nil {
				return nil, apperr.Wrap(err, "select identity.organizations")
			}

			return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		},
	)
	ops.RegisterQuery(reg, ops.DomainIdentity, QueryMembers, members)
	ops.RegisterQuery(reg, ops.DomainIdentity, QueryReachable, reachable)
	ops.RegisterQuery(reg, ops.DomainIdentity, QueryNotificationsStored, notificationsStored)
	ops.RegisterQuery(reg, ops.DomainIdentity, QueryConsent, consent)
	ops.RegisterQuery(reg, ops.DomainIdentity, QueryLoadMembers, loadMembers)
	ops.RegisterQuery(reg, ops.DomainIdentity, ops.QueryOrgRecordsIdentity, orgUsers)
	registerDirectory(reg)
}

// orgUsers reports any account of the organization: a membership, a contractor company user, or a customer user. They
// are removed first, so the organization is not deleted with people in it (DEC-103).
func orgUsers(ctx context.Context, c *ops.Call, input *ops.OrgRecordsInput) (bool, error) {
	var held bool

	err := c.Tx.QueryRow(
		ctx,
		`SELECT EXISTS (SELECT 1 FROM identity.memberships WHERE organization_id = $1)
		OR EXISTS (SELECT 1 FROM identity.partner_users WHERE organization_id = $1)
		OR EXISTS (SELECT 1 FROM identity.client_users WHERE customer_id = $2)`,
		input.OrgID,
		input.CustomerID,
	).Scan(&held)

	return held, apperr.Wrap(err, "select identity.memberships")
}

// QueryMembers returns the memberships active at the caller's business time that match every given filter, sorted
// (IR191: recipients and owner checks of other domains).
const QueryMembers = "identity.members"

// MembersInput is QueryMembers input; empty fields do not filter.
type MembersInput struct {
	IDs            []uuid.UUID `json:"ids,omitempty"`
	Role           string      `json:"role,omitempty"`
	OrganizationID *uuid.UUID  `json:"organizationId,omitempty"`
	Permission     string      `json:"permission,omitempty"`
	// owner / member (filter cleaning reminders to the owners, IR239)
	ClientRole string `json:"clientRole,omitempty"`
}

// QueryConsent reports whether a membership granted a consent purpose (IR194: SR02 location automations).
const QueryConsent = "identity.consent"

// ConsentInput is QueryConsent input.
type ConsentInput struct {
	MembershipID uuid.UUID `json:"membershipId"`
	Purpose      string    `json:"purpose"`
}

// Members asks identity for the active memberships matching f (any domain; local for the worker).
func Members(ctx context.Context, c *ops.Call, f MembersInput) ([]uuid.UUID, error) {
	out, err := ops.Delegate(ctx, c, QueryMembers, f, members)

	return out, apperr.Wrap(err, "delegate identity.members")
}

// ConsentGranted asks identity whether the membership granted the purpose (false when never recorded).
func ConsentGranted(
	ctx context.Context,
	call *ops.Call,
	membership uuid.UUID,
	purpose string,
) (bool, error) {
	out, err := ops.Delegate(
		ctx,
		call,
		QueryConsent,
		ConsentInput{MembershipID: membership, Purpose: purpose},
		consent,
	)

	return out, apperr.Wrap(err, "delegate identity.consent")
}

func consent(ctx context.Context, c *ops.Call, in *ConsentInput) (bool, error) {
	var granted bool

	err := c.Tx.QueryRow(ctx, `SELECT COALESCE((SELECT granted FROM identity.consents WHERE membership_id = $1 AND purpose
		= $2), false)`, in.MembershipID, in.Purpose).
		Scan(&granted)

	return granted, apperr.Wrap(err, "select identity.consents")
}

// QueryNotificationsStored counts stored notifications among ids for the target at occurredAt (IR191: the
// restriction notice evidence of restrictions.execute).
const QueryNotificationsStored = "identity.notificationsStored"

// NotificationsStoredInput is QueryNotificationsStored input.
type NotificationsStoredInput struct {
	IDs        []uuid.UUID `json:"ids"`
	TargetID   uuid.UUID   `json:"targetId"`
	OccurredAt time.Time   `json:"occurredAt"`
}

func members(ctx context.Context, call *ops.Call, input *MembersInput) ([]uuid.UUID, error) {
	stmt := `SELECT m.id FROM identity.memberships m WHERE m.valid_from <= $1 AND (m.valid_until IS NULL OR m.valid_until >
		$1)
		AND ($2::uuid[] IS NULL OR m.id = ANY($2)) AND ($3 = '' OR m.role = $3) AND ($4::uuid IS NULL OR m.organization_id =
		$4)
		AND ($5 = '' OR EXISTS (SELECT 1 FROM identity.membership_permissions p WHERE p.membership_id = m.id AND p.permission
		= $5))
		AND ($6 = '' OR m.client_role = $6) ORDER BY m.id`

	var ids []uuid.UUID
	if input.IDs != nil {
		ids = input.IDs
	}

	rows, err := call.Tx.Query(
		ctx,
		stmt,
		call.Now,
		ids,
		input.Role,
		input.OrganizationID,
		input.Permission,
		input.ClientRole,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select identity.memberships")
	}

	out, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if out == nil {
		out = []uuid.UUID{}
	}

	return out, apperr.Wrap(err, "select identity.memberships")
}

func notificationsStored(
	ctx context.Context,
	c *ops.Call,
	input *NotificationsStoredInput,
) (int, error) {
	var count int

	err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM notify.notifications n WHERE n.id = ANY($1) AND n.target->>'id' =
		$2::text AND n.occurred_at = $3`,
		input.IDs, input.TargetID, input.OccurredAt).
		Scan(&count)

	return count, apperr.Wrap(err, "select notify.notifications")
}
