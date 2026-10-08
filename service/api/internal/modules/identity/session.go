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
	ops.Register(r, "session.get", func(ctx context.Context, c *ops.Call, _ *struct{}) (Session, error) {
		p := c.Principal
		perms := make([]string, 0, len(p.Permissions))
		for k, ok := range p.Permissions {
			if ok {
				perms = append(perms, k)
			}
		}
		sort.Strings(perms)
		return Session{TenantID: p.TenantID, MembershipID: p.MembershipID, ScopeVersion: p.ScopeVersion, UserID: p.UserID,
			Role: p.Role, Permissions: perms, Generation: 1, ViewEpoch: 0, IssuedAt: c.Now, ExpiresAt: c.Now.Add(SessionTTL)}, nil
	})
}
