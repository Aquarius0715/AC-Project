package auth

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

// PrincipalPath is identity-api's internal endpoint (never routed by the gateway).
const PrincipalPath = "/internal/v1/principal"

// PrincipalDTO is the wire form of ops.Principal between identity-api and the other services.
type PrincipalDTO struct {
	UserID       uuid.UUID              `json:"userId"`
	TenantID     uuid.UUID              `json:"tenantId"`
	MembershipID uuid.UUID              `json:"membershipId"`
	OrgID        uuid.UUID              `json:"orgId"`
	Role         string                 `json:"role"`
	ClientRole   string                 `json:"clientRole"`
	Employment   string                 `json:"employment"`
	Timezone     string                 `json:"timezone"`
	Permissions  []string               `json:"permissions"`
	Scopes       map[string][]uuid.UUID `json:"scopes"`
	ScopeVersion int                    `json:"scopeVersion"`
}

// ToDTO converts a principal for the wire.
func ToDTO(p *ops.Principal) PrincipalDTO {
	d := PrincipalDTO{UserID: p.UserID, TenantID: p.TenantID, MembershipID: p.MembershipID, OrgID: p.OrgID, Role: p.Role,
		ClientRole: p.ClientRole, Employment: p.Employment, Timezone: p.Timezone, Scopes: p.Scopes, ScopeVersion: p.ScopeVersion, Permissions: []string{}}
	for perm, ok := range p.Permissions {
		if ok {
			d.Permissions = append(d.Permissions, perm)
		}
	}
	return d
}

// FromDTO rebuilds the principal.
func FromDTO(d PrincipalDTO) *ops.Principal {
	p := &ops.Principal{Principal: authz.Principal{Role: d.Role, ClientRole: d.ClientRole, Permissions: map[string]bool{}},
		TenantID: d.TenantID, MembershipID: d.MembershipID, UserID: d.UserID, OrgID: d.OrgID, ScopeVersion: d.ScopeVersion,
		Scopes: d.Scopes, Employment: d.Employment, Timezone: d.Timezone}
	for _, perm := range d.Permissions {
		p.Permissions[perm] = true
	}
	if p.Scopes == nil {
		p.Scopes = map[string][]uuid.UUID{}
	}
	return p
}

// RemoteSource asks identity-api for principals and caches them for TTL (backend Go design §4: 30 s keyed by the
// membership; a scope change is visible after at most TTL).
type RemoteSource struct {
	BaseURL string // e.g. http://identity-api:8080
	Token   string // INTERNAL_API_TOKEN shared by the services
	Client  *http.Client
	TTL     time.Duration
	Now     func() time.Time

	mu    sync.Mutex
	cache map[string]cached
}

type cached struct {
	p   *ops.Principal
	err error
	at  time.Time
}

// ErrNoMembership is identity-api's answer for an unknown, foreign or inactive membership.
var ErrNoMembership = errors.New("auth: membership not valid")

// Principal implements PrincipalSource. A fetch passes the token's sign-in time on, so identity-api records it
// (IR268); a cached principal does not, and the next fetch does.
func (r *RemoteSource) Principal(ctx context.Context, subject string, tenant, membership uuid.UUID, signedIn time.Time) (*ops.Principal, error) {
	now := time.Now
	if r.Now != nil {
		now = r.Now
	}
	key := subject + "|" + tenant.String() + "|" + membership.String()
	r.mu.Lock()
	if c, ok := r.cache[key]; ok && now().Sub(c.at) < r.TTL {
		r.mu.Unlock()
		return c.p, c.err
	}
	r.mu.Unlock()
	p, err := r.fetch(ctx, subject, tenant, membership, signedIn)
	if errors.Is(err, ErrUnavailable) {
		return nil, err // never cache an outage
	}
	r.mu.Lock()
	if r.cache == nil {
		r.cache = map[string]cached{}
	}
	r.cache[key] = cached{p: p, err: err, at: now()}
	r.mu.Unlock()
	return p, err
}

func (r *RemoteSource) fetch(ctx context.Context, subject string, tenant, membership uuid.UUID, signedIn time.Time) (*ops.Principal, error) {
	q := url.Values{"subject": {subject}, "tenant": {tenant.String()}, "membership": {membership.String()}}
	if !signedIn.IsZero() {
		q.Set("signedIn", signedIn.UTC().Format(time.RFC3339))
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.BaseURL+PrincipalPath+"?"+q.Encode(), nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Bearer "+r.Token)
	client := r.Client
	if client == nil {
		client = &http.Client{Timeout: 5 * time.Second}
	}
	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	defer res.Body.Close()
	switch {
	case res.StatusCode == http.StatusOK:
		var d PrincipalDTO
		if err := json.NewDecoder(res.Body).Decode(&d); err != nil {
			return nil, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		return FromDTO(d), nil
	case res.StatusCode == http.StatusNotFound:
		return nil, ErrNoMembership
	default:
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, res.StatusCode)
	}
}
