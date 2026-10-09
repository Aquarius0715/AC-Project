// Package identity implements the Identity & access module.
package identity

import (
	"context"
	"sort"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// Session is Session of service-contracts.ts (generation and viewEpoch are owned by the BFF session; the Core API
// reports the membership-derived part and starts both at their initial values).
type Session struct {
	TenantID     uuid.UUID `json:"tenantId"`
	MembershipID uuid.UUID `json:"membershipId"`
	ScopeVersion int       `json:"scopeVersion"`
	UserID       uuid.UUID `json:"userId"`
	Role         string    `json:"role"`
	ClientRole   *string   `json:"clientRole"` // owner / member for client sessions, otherwise null (the owner-only Users page, IR210)
	Permissions  []string  `json:"permissions"`
	Generation   int       `json:"generation"`
	ViewEpoch    int       `json:"viewEpoch"`
	IssuedAt     time.Time `json:"issuedAt"`
	ExpiresAt    time.Time `json:"expiresAt"`
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
//	@Router			/v1/ops/session.get [post]
func sessionGet(ctx context.Context, c *ops.Call, _ *struct{}) (Session, error) {
	p := c.Principal
	perms := make([]string, 0, len(p.Permissions))
	for k, ok := range p.Permissions {
		if ok {
			perms = append(perms, k)
		}
	}
	sort.Strings(perms)
	var clientRole *string
	if p.Role == "client" && p.ClientRole != "" {
		clientRole = &p.ClientRole
	}
	return Session{TenantID: p.TenantID, MembershipID: p.MembershipID, ScopeVersion: p.ScopeVersion, UserID: p.UserID,
		Role: p.Role, ClientRole: clientRole, Permissions: perms, Generation: 1, ViewEpoch: 0, IssuedAt: c.Now, ExpiresAt: c.Now.Add(SessionTTL)}, nil
}
