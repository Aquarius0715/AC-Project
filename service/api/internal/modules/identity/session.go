// Package identity implements the Identity & access module.
package identity

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

// Session is Session of service-contracts.ts (generation and viewEpoch are owned by the BFF session; the Core API
// reports the membership-derived part and starts both at their initial values).
type Session struct {
	TenantID     uuid.UUID `json:"tenantId"`
	MembershipID uuid.UUID `json:"membershipId"`
	ScopeVersion int       `json:"scopeVersion"`
	UserID       uuid.UUID `json:"userId"`
	Role         string    `json:"role"`
	// owner / member for client sessions, otherwise null (the owner-only Users page, IR210)
	ClientRole *string `json:"clientRole"`
	// DisplayName and OrganizationName name the signed-in user and the membership's organization in the shell (IR241);
	// OrganizationID is that organization, so a screen can tell its own company from another one in a URL (IR276)
	DisplayName      string    `json:"displayName"`
	OrganizationName string    `json:"organizationName"`
	OrganizationID   uuid.UUID `json:"organizationId"`
	// CustomerID is a client session's customer (its organization's), for the writes that name it (alert policies,
	// default rule settings, IR243); null for the other roles
	CustomerID  *uuid.UUID `json:"customerId"`
	Permissions []string   `json:"permissions"`
	Generation  int        `json:"generation"`
	ViewEpoch   int        `json:"viewEpoch"`
	IssuedAt    time.Time  `json:"issuedAt"`
	ExpiresAt   time.Time  `json:"expiresAt"`
}

// SessionTTL is the idle lifetime reported to the client.
const SessionTTL = 30 * time.Minute

// Register binds the Identity operations implemented so far.
func Register(r *ops.Registry) {
	ops.Register(r, "organizations.list", organizationsList)
	ops.Register(r, "organizations.save", organizationsSave)
	ops.Register(r, "session.get", sessionGet)
}

// sessionGet answers session.get: the signed-in principal as the Session of service-contracts.ts.
//
//	@Summary		session.get (read)
//	@ID				session.get
//	@Description	Authorization: authenticated:own-session
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-P08
//	@Tags			session
//	@Accept			json
//	@Produce		json
//	@Success		200	{object}	ops.Envelope{data=Session}
//	@Failure		401	{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403	{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404	{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409	{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422	{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429	{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503	{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504	{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/session [get]
func sessionGet(ctx context.Context, call *ops.Call, _ *struct{}) (Session, error) {
	principal := call.Principal

	perms := make([]string, 0, len(principal.Permissions))
	for k, ok := range principal.Permissions {
		if ok {
			perms = append(perms, k)
		}
	}

	sort.Strings(perms)

	var clientRole *string
	if principal.Role == authz.RoleClient && principal.ClientRole != "" {
		clientRole = &principal.ClientRole
	}

	out := Session{
		TenantID:       principal.TenantID,
		MembershipID:   principal.MembershipID,
		ScopeVersion:   principal.ScopeVersion,
		UserID:         principal.UserID,
		OrganizationID: principal.OrgID,
		Role:           principal.Role,
		ClientRole:     clientRole,
		Permissions:    perms,
		Generation:     1,
		ViewEpoch:      0,
		IssuedAt:       call.Now,
		ExpiresAt: call.Now.Add(
			SessionTTL,
		),
		DisplayName:      "",
		OrganizationName: "",
		CustomerID:       nil,
	}
	if call.Tx == nil { // a registry without a database (unit tests): no names
		return out, nil
	}

	err := call.Tx.QueryRow(ctx, `SELECT u.display_name, o.name FROM identity.memberships m JOIN identity.users u ON u.id =
		m.user_id
		JOIN identity.organizations o ON o.id = m.organization_id WHERE m.id = $1`, principal.MembershipID).
		Scan(&out.DisplayName, &out.OrganizationName)
	if errors.Is(err, pgx.ErrNoRows) {
		err = nil
	}

	if err == nil &&
		principal.Role == authz.RoleClient { // identity's reference copy of the customers (IR188)
		var id uuid.UUID
		switch scanErr := call.Tx.QueryRow(ctx, `SELECT c.id FROM notify.ref_customers c JOIN identity.memberships m ON
			m.organization_id = c.organization_id WHERE m.id = $1`,
			principal.MembershipID).Scan(&id); {
		case scanErr == nil:
			out.CustomerID = &id
		case !errors.Is(scanErr, pgx.ErrNoRows):
			err = scanErr
		}
	}

	return out, err
}
