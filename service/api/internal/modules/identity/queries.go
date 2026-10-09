package identity

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// QueryActiveOrganizations returns which of the given organizations are active (IR190; IR40 customer counts).
const QueryActiveOrganizations = "identity.activeOrganizations"

// OrganizationsInput is QueryActiveOrganizations input.
type OrganizationsInput struct {
	IDs []uuid.UUID `json:"ids"`
}

// RegisterQueries binds identity's internal queries.
func RegisterQueries(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainIdentity, QueryActiveOrganizations, func(ctx context.Context, c *ops.Call, in *OrganizationsInput) ([]uuid.UUID, error) {
		rows, err := c.Tx.Query(ctx, `SELECT id FROM identity.organizations WHERE id = ANY($1) AND status = 'active' ORDER BY id`, in.IDs)
		if err != nil {
			return nil, err
		}
		return pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	})
	ops.RegisterQuery(r, ops.DomainIdentity, QueryMembers, members)
	ops.RegisterQuery(r, ops.DomainIdentity, QueryNotificationsStored, notificationsStored)
	ops.RegisterQuery(r, ops.DomainIdentity, QueryConsent, consent)
	ops.RegisterQuery(r, ops.DomainIdentity, QueryLoadMembers, loadMembers)
	registerDirectory(r)
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
	ClientRole     string      `json:"clientRole,omitempty"` // owner / member (filter cleaning reminders to the owners, IR239)
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
	return ops.Delegate(ctx, c, QueryMembers, f, members)
}

// ConsentGranted asks identity whether the membership granted the purpose (false when never recorded).
func ConsentGranted(ctx context.Context, c *ops.Call, membership uuid.UUID, purpose string) (bool, error) {
	return ops.Delegate(ctx, c, QueryConsent, ConsentInput{MembershipID: membership, Purpose: purpose}, consent)
}

func consent(ctx context.Context, c *ops.Call, in *ConsentInput) (bool, error) {
	var granted bool
	err := c.Tx.QueryRow(ctx, `SELECT COALESCE((SELECT granted FROM identity.consents WHERE membership_id = $1 AND purpose = $2), false)`, in.MembershipID, in.Purpose).Scan(&granted)
	return granted, err
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

func members(ctx context.Context, c *ops.Call, in *MembersInput) ([]uuid.UUID, error) {
	q := `SELECT m.id FROM identity.memberships m WHERE m.valid_from <= $1 AND (m.valid_until IS NULL OR m.valid_until > $1)
		AND ($2::uuid[] IS NULL OR m.id = ANY($2)) AND ($3 = '' OR m.role = $3) AND ($4::uuid IS NULL OR m.organization_id = $4)
		AND ($5 = '' OR EXISTS (SELECT 1 FROM identity.membership_permissions p WHERE p.membership_id = m.id AND p.permission = $5))
		AND ($6 = '' OR m.client_role = $6) ORDER BY m.id`
	var ids []uuid.UUID
	if in.IDs != nil {
		ids = in.IDs
	}
	rows, err := c.Tx.Query(ctx, q, c.Now, ids, in.Role, in.OrganizationID, in.Permission, in.ClientRole)
	if err != nil {
		return nil, err
	}
	out, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if out == nil {
		out = []uuid.UUID{}
	}
	return out, err
}

func notificationsStored(ctx context.Context, c *ops.Call, in *NotificationsStoredInput) (int, error) {
	var n int
	err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM notify.notifications n WHERE n.id = ANY($1) AND n.target->>'id' = $2::text AND n.occurred_at = $3`,
		in.IDs, in.TargetID, in.OccurredAt).Scan(&n)
	return n, err
}
