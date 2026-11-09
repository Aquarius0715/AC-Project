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
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
)

// PrincipalPath is identity-api's internal endpoint (never routed by the gateway).
const PrincipalPath = "/internal/v1/principal"

// fetchTimeout bounds one request to identity-api when RemoteSource has no Client of its own.
const fetchTimeout = 5 * time.Second

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
	// SessionsRevokedAt travels with the principal, so every service refuses an earlier sign-in itself (IR384)
	SessionsRevokedAt *time.Time `json:"sessionsRevokedAt"`
}

// ToDTO converts a principal for the wire.
func ToDTO(principal *ops.Principal) PrincipalDTO {
	principalDTO := PrincipalDTO{
		UserID:            principal.UserID,
		TenantID:          principal.TenantID,
		MembershipID:      principal.MembershipID,
		OrgID:             principal.OrgID,
		Role:              principal.Role,
		ClientRole:        principal.ClientRole,
		Employment:        principal.Employment,
		Timezone:          principal.Timezone,
		Scopes:            principal.Scopes,
		ScopeVersion:      principal.ScopeVersion,
		Permissions:       []string{},
		SessionsRevokedAt: principal.SessionsRevokedAt,
	}
	for perm, ok := range principal.Permissions {
		if ok {
			principalDTO.Permissions = append(principalDTO.Permissions, perm)
		}
	}

	return principalDTO
}

// FromDTO rebuilds the principal.
func FromDTO(principalDTO PrincipalDTO) *ops.Principal {
	principal := &ops.Principal{
		Principal: authz.Principal{
			Role:        principalDTO.Role,
			ClientRole:  principalDTO.ClientRole,
			Permissions: map[string]bool{},
		},
		TenantID:          principalDTO.TenantID,
		MembershipID:      principalDTO.MembershipID,
		UserID:            principalDTO.UserID,
		OrgID:             principalDTO.OrgID,
		ScopeVersion:      principalDTO.ScopeVersion,
		Scopes:            principalDTO.Scopes,
		Employment:        principalDTO.Employment,
		Timezone:          principalDTO.Timezone,
		SessionsRevokedAt: principalDTO.SessionsRevokedAt,
		IssuerHost:        "",
	}
	for _, perm := range principalDTO.Permissions {
		principal.Permissions[perm] = true
	}

	if principal.Scopes == nil {
		principal.Scopes = map[string][]uuid.UUID{}
	}

	return principal
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
func (r *RemoteSource) Principal(
	ctx context.Context,
	subject string,
	tenant, membership uuid.UUID,
	signedIn time.Time,
) (*ops.Principal, error) {
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

	principal, err := r.fetch(ctx, subject, tenant, membership, signedIn)
	if errors.Is(err, ErrUnavailable) {
		return nil, err // never cache an outage
	}

	r.mu.Lock()
	if r.cache == nil {
		r.cache = map[string]cached{}
	}

	r.cache[key] = cached{p: principal, err: err, at: now()}
	r.mu.Unlock()

	return principal, err
}

func (r *RemoteSource) fetch(
	ctx context.Context,
	subject string,
	tenant, membership uuid.UUID,
	signedIn time.Time,
) (*ops.Principal, error) {
	params := url.Values{
		"subject":    {subject},
		"tenant":     {tenant.String()},
		"membership": {membership.String()},
	}
	if !signedIn.IsZero() {
		params.Set("signedIn", signedIn.UTC().Format(time.RFC3339))
	}

	req, err := http.NewRequestWithContext(
		ctx,
		http.MethodGet,
		r.BaseURL+PrincipalPath+"?"+params.Encode(),
		http.NoBody,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "http.NewRequestWithContext")
	}

	req.Header.Set("Authorization", "Bearer "+r.Token)

	client := r.Client
	if client == nil {
		client = &http.Client{
			Timeout:       fetchTimeout,
			Transport:     nil,
			CheckRedirect: nil,
			Jar:           nil,
		}
	}

	res, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
	}
	defer func() { _ = res.Body.Close() }()

	switch res.StatusCode {
	case http.StatusOK:
		var principalDTO PrincipalDTO

		err := json.NewDecoder(res.Body).Decode(&principalDTO)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrUnavailable, err)
		}

		return FromDTO(principalDTO), nil
	case http.StatusNotFound:
		return nil, ErrNoMembership
	default:
		return nil, fmt.Errorf("%w: status %d", ErrUnavailable, res.StatusCode)
	}
}
