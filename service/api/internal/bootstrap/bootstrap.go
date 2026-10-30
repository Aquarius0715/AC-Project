// Package bootstrap creates the first HQ administrator of a tenant, so an empty environment starts from one admin
// (DEC-84, IR509): the tenant and its HQ (operator) organization when they are missing, the person, an admin
// membership with every HQ permission over the tenant, and the identity consumer's AccountRequested event, which opens
// the sign-in account with the two-step set-up (IR468) and e-mails the set-up link (IR461). A tenant that already has
// an active identity administrator is refused: from then on HQ adds people in Access & roles. It runs as the schema
// owner (cmd/migrate bootstrap-admin); the change capture triggers copy the new rows to the other domains (IR188).
package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
)

// Input names the tenant, its HQ organization and the first administrator.
type Input struct {
	Tenant       string // the tenant's name: an existing tenant of that name, or a new one
	Organization string // the HQ organization's name, used when the tenant has none
	Email        string
	Name         string
	At           time.Time // the membership's start; zero: the business time (the wall clock with a demo clock's offset)
}

// Result is what Admin created or reused.
type Result struct {
	TenantID, OrganizationID, UserID, MembershipID uuid.UUID
	NewTenant, NewOrganization                     bool
	At                                             time.Time // the membership's start: the business time (with a demo clock's offset)
}

// ErrAdminExists refuses a tenant that already has an active identity administrator.
var ErrAdminExists = errors.New("the tenant already has an active identity administrator: add people in Access & roles")

// ErrEmailTaken refuses an address another person already holds.
var ErrEmailTaken = errors.New("the address belongs to an existing person: use another address")

var email = regexp.MustCompile(`^[^\s@]+@[^\s@]+\.[^\s@]+$`)

// Validate checks the input as members.save checks a new person (IR461): a name of 1–120 characters and a plain
// address of up to 254.
func (in *Input) Validate() error {
	in.Tenant, in.Organization, in.Email, in.Name = strings.TrimSpace(in.Tenant), strings.TrimSpace(in.Organization), strings.TrimSpace(in.Email), strings.TrimSpace(in.Name)
	switch {
	case in.Tenant == "" || utf8.RuneCountInString(in.Tenant) > 120:
		return errors.New("tenant: a name of 1–120 characters")
	case in.Organization == "" || utf8.RuneCountInString(in.Organization) > 120:
		return errors.New("organization: a name of 1–120 characters")
	case len(in.Email) > 254 || !email.MatchString(in.Email):
		return errors.New("email: a valid address of up to 254 characters")
	case in.Name == "" || utf8.RuneCountInString(in.Name) > 120:
		return errors.New("name: a name of 1–120 characters")
	}
	return nil
}

// Admin creates the first administrator in tx. The tenant's identity administrators are locked as members.save locks
// them (IR472), so a save that adds one at the same time waits for this one.
func Admin(ctx context.Context, tx pgx.Tx, in Input) (Result, error) {
	var r Result
	if err := in.Validate(); err != nil {
		return r, err
	}
	// the business time: a demo environment's clock runs at an offset from the wall clock (IR168)
	r.At = in.At
	if r.At.IsZero() {
		if err := tx.QueryRow(ctx, `SELECT now() + COALESCE((SELECT offset_ms FROM platform.demo_clock WHERE id), 0) * interval '1 millisecond'`).Scan(&r.At); err != nil {
			return r, err
		}
	}
	var ids []uuid.UUID
	rows, err := tx.Query(ctx, `SELECT id FROM platform.tenants WHERE name = $1`, in.Tenant)
	if err != nil {
		return r, err
	}
	if ids, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID]); err != nil {
		return r, err
	}
	switch len(ids) {
	case 0:
		if err := tx.QueryRow(ctx, `INSERT INTO platform.tenants (name, created_at) VALUES ($1, $2) RETURNING id`, in.Tenant, r.At).Scan(&r.TenantID); err != nil {
			return r, err
		}
		r.NewTenant = true
	case 1:
		r.TenantID = ids[0]
	default:
		return r, fmt.Errorf("tenant: %d tenants are named %q", len(ids), in.Tenant)
	}
	if _, err := tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtext('identity.identity_admins:' || $1::text))`, r.TenantID); err != nil {
		return r, err
	}
	var exists bool
	if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.memberships m JOIN identity.membership_permissions p ON p.membership_id = m.id AND p.permission = 'identity.write'
		WHERE m.tenant_id = $1 AND m.valid_from <= $2 AND (m.valid_until IS NULL OR m.valid_until > $2))`, r.TenantID, r.At).Scan(&exists); err != nil {
		return r, err
	}
	if exists {
		return r, ErrAdminExists
	}
	err = tx.QueryRow(ctx, `SELECT id FROM identity.organizations WHERE tenant_id = $1 AND kind = 'operator' ORDER BY created_at, id LIMIT 1`, r.TenantID).Scan(&r.OrganizationID)
	if errors.Is(err, pgx.ErrNoRows) {
		err = tx.QueryRow(ctx, `INSERT INTO identity.organizations (tenant_id, name, kind, status, created_at, updated_at) VALUES ($1, $2, 'operator', 'active', $3, $3) RETURNING id`,
			r.TenantID, in.Organization, r.At).Scan(&r.OrganizationID)
		r.NewOrganization = true
	}
	if err != nil {
		return r, err
	}
	err = tx.QueryRow(ctx, `INSERT INTO identity.users (email, display_name, status, created_at, updated_at) VALUES ($1, $2, 'active', $3, $3)
		ON CONFLICT (email) DO NOTHING RETURNING id`, in.Email, in.Name, r.At).Scan(&r.UserID)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrEmailTaken
	}
	if err != nil {
		return r, err
	}
	if err := tx.QueryRow(ctx, `INSERT INTO identity.memberships (tenant_id, user_id, organization_id, role, valid_from, created_at, updated_at)
		VALUES ($1, $2, $3, 'admin', $4, $4, $4) RETURNING id`, r.TenantID, r.UserID, r.OrganizationID, r.At).Scan(&r.MembershipID); err != nil {
		return r, err
	}
	// every HQ permission (the contractor's partner.* are not HQ's), over the whole tenant
	if _, err := tx.Exec(ctx, `INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission)
		SELECT $1, $2, code FROM identity.permissions WHERE code NOT LIKE 'partner.%'`, r.TenantID, r.MembershipID); err != nil {
		return r, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES ($1, $2, 'tenant', $1)`, r.TenantID, r.MembershipID); err != nil {
		return r, err
	}
	if _, err := tx.Exec(ctx, `INSERT INTO audit.audit_log (tenant_id, actor_id, actor_role_at_time, action, target_kind, target_id, next_version, occurred_at, correlation_id, result, reason)
		VALUES ($1, 'system-bootstrap', 'system', 'bootstrap.admin', 'membership', $2::text, 1, $3, 'bootstrap-admin', 'success', 'first HQ administrator')`,
		r.TenantID, r.MembershipID, r.At); err != nil {
		return r, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform.outbox (tenant_id, aggregate_type, aggregate_id, event_type, payload, correlation_id, occurred_at)
		VALUES ($1, 'membership', $2::uuid, 'AccountRequested', jsonb_build_object('userId', $3::uuid::text, 'membershipId', $2::uuid::text, 'role', 'admin'), 'bootstrap-admin', $4)`,
		r.TenantID, r.MembershipID, r.UserID, r.At)
	return r, err
}
