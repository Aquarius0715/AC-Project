package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/internal/ops"
	"github.com/pradita/ac-project/service/internal/platform/apperr"
	"github.com/pradita/ac-project/service/internal/platform/authz"
	"github.com/pradita/ac-project/service/internal/platform/paging"
)

// ScopeRef is ScopeRef of service-contracts.ts.
type ScopeRef struct {
	Kind string    `json:"kind"`
	ID   uuid.UUID `json:"id"`
}

// Grant is QualificationGrant.
type Grant struct {
	Code       string     `json:"code"`
	ValidFrom  time.Time  `json:"validFrom"`
	ValidUntil time.Time  `json:"validUntil"`
	RevokedAt  *time.Time `json:"revokedAt"`
}

// Member is Membership of service-contracts.ts.
type Member struct {
	ID             uuid.UUID  `json:"id"`
	TenantID       uuid.UUID  `json:"tenantId"`
	Version        int        `json:"version"`
	CreatedAt      time.Time  `json:"createdAt"`
	UpdatedAt      time.Time  `json:"updatedAt"`
	UserID         uuid.UUID  `json:"userId"`
	DisplayName    string     `json:"displayName"` // the user's display name (IR172)
	OrganizationID uuid.UUID  `json:"organizationId"`
	Role           string     `json:"role"`
	Employment     *string    `json:"employment"`
	Permissions    []string   `json:"permissions"`
	Scopes         []ScopeRef `json:"scopes"`
	ScopeVersion   int        `json:"scopeVersion"`
	Qualifications []Grant    `json:"qualifications"`
	ValidFrom      time.Time  `json:"validFrom"`
	ValidUntil     *time.Time `json:"validUntil"`
	ClientRole     *string    `json:"clientRole"`
}

const memberCols = `m.id, m.tenant_id, m.version, m.created_at, m.updated_at, m.user_id, m.organization_id, m.role, m.employment, m.scope_version, m.valid_from,
	m.valid_until, m.client_role, COALESCE((SELECT u.display_name FROM identity.users u WHERE u.id = m.user_id), '')`

func scanMember(r pgx.Row) (Member, error) {
	var x Member
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.UserID, &x.OrganizationID, &x.Role, &x.Employment, &x.ScopeVersion,
		&x.ValidFrom, &x.ValidUntil, &x.ClientRole, &x.DisplayName)
	return x, err
}

// LoadMembers fills permissions, scopes and qualifications (IR42: contractors get no permissions or scopes).
func LoadMembers(ctx context.Context, c *ops.Call, ms []Member) error {
	for i := range ms {
		x := &ms[i]
		x.Permissions, x.Scopes, x.Qualifications = []string{}, []ScopeRef{}, []Grant{}
		if c.Principal.Role != "contractor" {
			rows, err := c.Tx.Query(ctx, `SELECT permission FROM identity.membership_permissions WHERE membership_id = $1 ORDER BY permission`, x.ID)
			if err != nil {
				return err
			}
			for rows.Next() {
				var p string
				if err := rows.Scan(&p); err != nil {
					rows.Close()
					return err
				}
				x.Permissions = append(x.Permissions, p)
			}
			rows.Close()
			rows, err = c.Tx.Query(ctx, `SELECT kind, ref_id FROM identity.membership_scopes WHERE membership_id = $1 ORDER BY kind, ref_id`, x.ID)
			if err != nil {
				return err
			}
			for rows.Next() {
				var s ScopeRef
				if err := rows.Scan(&s.Kind, &s.ID); err != nil {
					rows.Close()
					return err
				}
				x.Scopes = append(x.Scopes, s)
			}
			rows.Close()
		}
		rows, err := c.Tx.Query(ctx, `SELECT code, valid_from, valid_until, revoked_at FROM identity.qualification_grants WHERE membership_id = $1 ORDER BY code`, x.ID)
		if err != nil {
			return err
		}
		for rows.Next() {
			var g Grant
			if err := rows.Scan(&g.Code, &g.ValidFrom, &g.ValidUntil, &g.RevokedAt); err != nil {
				rows.Close()
				return err
			}
			x.Qualifications = append(x.Qualifications, g)
		}
		rows.Close()
	}
	return nil
}

// ---- members.list ----

func membersList(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Member], error) {
	var f struct {
		Role           *string    `json:"role,omitempty"`
		OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
		Active         *bool      `json:"active,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil || (f.Role != nil && !slices.Contains(roles, *f.Role)) {
			return paging.Page[Member]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "m.id", "validFrom": "m.valid_from", "updatedAt": "m.updated_at"}, "m.id ASC")
	if err != nil {
		return paging.Page[Member]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Member]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"TRUE"}
	if c.Principal.Role == "contractor" {
		conds = append(conds, "m.role = 'technician' AND m.organization_id = "+add(c.Principal.OrgID))
	}
	if f.Role != nil {
		conds = append(conds, "m.role = "+add(*f.Role))
	} else {
		conds = append(conds, "m.role <> 'client'")
	}
	if f.OrganizationID != nil {
		conds = append(conds, "m.organization_id = "+add(*f.OrganizationID))
	}
	if f.Active != nil {
		now := add(c.Now)
		q := "(m.valid_from <= " + now + " AND (m.valid_until IS NULL OR m.valid_until > " + now + "))"
		if !*f.Active {
			q = "NOT " + q
		}
		conds = append(conds, q)
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM identity.memberships m WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Member]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM identity.memberships m WHERE %s ORDER BY %s LIMIT %d OFFSET %d", memberCols, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Member]{}, err
	}
	items := []Member{}
	for rows.Next() {
		x, err := scanMember(rows)
		if err != nil {
			rows.Close()
			return paging.Page[Member]{}, err
		}
		items = append(items, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return paging.Page[Member]{}, err
	}
	if err := LoadMembers(ctx, c, items); err != nil {
		return paging.Page[Member]{}, err
	}
	return paging.Page[Member]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

// ---- members.save ----

var roles = []string{"client", "contractor", "technician", "admin"}

// RolePermissions are the permissions each role may hold (IR132 item 1).
var RolePermissions = map[string][]string{
	"contractor": {"partner.accept", "partner.assign", "partner.review"},
	"technician": {"alert.read", "alert.resolve", "control.diagnose", "device.maintain"},
	"client":     {},
}

// MemberInput is members.save input.
type MemberInput struct {
	ID             *uuid.UUID `json:"id,omitempty"`
	UserID         uuid.UUID  `json:"userId"`
	DisplayName    string     `json:"displayName"` // the user's display name (IR172)
	OrganizationID uuid.UUID  `json:"organizationId"`
	Role           string     `json:"role"`
	Employment     *string    `json:"employment"`
	Permissions    []string   `json:"permissions"`
	Scopes         []ScopeRef `json:"scopes"`
	ValidFrom      time.Time  `json:"validFrom"`
	ValidUntil     *time.Time `json:"validUntil"`
	Reason         string     `json:"reason"`
}

// Validate implements ops.Validator for the lookup-free rules.
func (in *MemberInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.UserID == uuid.Nil {
		fe["userId"] = "error.required"
	}
	if in.OrganizationID == uuid.Nil {
		fe["organizationId"] = "error.required"
	}
	if !slices.Contains(roles, in.Role) {
		fe["role"] = "error.invalid"
	}
	switch {
	case in.Role == "technician" && (in.Employment == nil || (*in.Employment != "internal" && *in.Employment != "external")):
		fe["employment"] = "error.required"
	case in.Role != "technician" && in.Employment != nil:
		fe["employment"] = "error.notAllowed"
	}
	if in.Permissions == nil || in.Scopes == nil {
		fe["permissions"] = "error.required"
	}
	seen := map[string]bool{}
	for _, p := range in.Permissions {
		allowed := authz.Permissions[p]
		if list, ok := RolePermissions[in.Role]; ok {
			allowed = slices.Contains(list, p)
		}
		if !allowed || seen[p] {
			fe["permissions"] = "error.invalid"
		}
		seen[p] = true
	}
	for p := range seen {
		if r, ok := strings.CutSuffix(p, ".write"); ok && authz.Permissions[r+".read"] && !seen[r+".read"] {
			fe["permissions"] = "errors.write_needs_read"
		}
	}
	kinds := map[string][]string{"admin": {"tenant"}, "client": {"organization"}, "contractor": {"organization"}, "technician": {"unit", "property", "organization"}}[in.Role]
	for _, sc := range in.Scopes {
		if !slices.Contains(kinds, sc.Kind) {
			fe["scopes"] = "error.invalid"
		}
	}
	if in.Role == "admin" && len(in.Scopes) > 1 {
		fe["scopes"] = "error.invalid"
	}
	if in.ValidFrom.IsZero() {
		fe["validFrom"] = "error.required"
	}
	if in.ValidUntil != nil && !in.ValidFrom.Before(*in.ValidUntil) {
		fe["validUntil"] = "error.range"
	}
	if in.Role == "technician" && in.Employment != nil && *in.Employment == "external" && in.ValidUntil == nil {
		fe["validUntil"] = "errors.external_needs_end"
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fe["reason"] = "error.length"
	}
	return fe
}

func membersSave(ctx context.Context, c *ops.Call, in *MemberInput) (Member, error) {
	var exists bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.users WHERE id = $1)`, in.UserID).Scan(&exists); err != nil {
		return Member{}, err
	}
	var kind string
	err := c.Tx.QueryRow(ctx, `SELECT kind FROM identity.organizations WHERE id = $1`, in.OrganizationID).Scan(&kind)
	if !exists || errors.Is(err, pgx.ErrNoRows) {
		return Member{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return Member{}, err
	}
	want := map[string]string{"admin": "operator", "client": "customer", "contractor": "contractor"}[in.Role]
	if in.Role == "technician" {
		want = map[string]string{"internal": "operator", "external": "contractor"}[*in.Employment]
	}
	if kind != want {
		return Member{}, apperr.Fields(map[string]string{"organizationId": "errors.organization_kind"})
	}
	// scope references
	for _, sc := range in.Scopes {
		var ok bool
		switch sc.Kind {
		case "tenant":
			ok = sc.ID == c.Principal.TenantID
		case "organization":
			err = c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.organizations WHERE id = $1 AND (kind = 'customer' OR id = $2))`, sc.ID, in.OrganizationID).Scan(&ok)
			if in.Role != "technician" {
				ok = ok && sc.ID == in.OrganizationID
			}
		case "property":
			err = c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.properties WHERE id = $1)`, sc.ID).Scan(&ok)
		case "unit":
			err = c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.units WHERE id = $1)`, sc.ID).Scan(&ok)
		}
		if err != nil {
			return Member{}, err
		}
		if !ok {
			return Member{}, apperr.Fields(map[string]string{"scopes": "errors.unknown_scope"})
		}
	}
	// self promotion
	if in.UserID == c.Principal.UserID { // granting identity.write / restriction.override to oneself (DD-A03)
		for _, p := range []string{"identity.write", "restriction.override"} {
			if !slices.Contains(in.Permissions, p) {
				continue
			}
			had := false
			if in.ID != nil {
				if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.membership_permissions WHERE membership_id = $1 AND permission = $2)`, *in.ID, p).Scan(&had); err != nil {
					return Member{}, err
				}
			}
			if !had {
				return Member{}, apperr.E(apperr.Forbidden, "errors.self_promotion")
			}
		}
	}
	var id uuid.UUID
	var prev *int
	if in.ID == nil {
		var clientRole *string
		if in.Role == "client" {
			cr := "member"
			clientRole = &cr
		}
		if err := c.Tx.QueryRow(ctx, `INSERT INTO identity.memberships (tenant_id, user_id, organization_id, role, employment, client_role, valid_from, valid_until, created_at, updated_at)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, $7, $8, $8) RETURNING id`,
			in.UserID, in.OrganizationID, in.Role, in.Employment, clientRole, in.ValidFrom, in.ValidUntil, c.Now).Scan(&id); err != nil {
			return Member{}, err
		}
		if in.Role == "client" { // IR84 initial Consent
			if _, err := c.Tx.Exec(ctx, `INSERT INTO identity.consents (tenant_id, membership_id, purpose, granted) VALUES (current_setting('app.tenant_id')::uuid, $1, 'location_automation', false)`, id); err != nil {
				return Member{}, err
			}
			// IR144: the customer's user list includes the new active member (no-op when the email is already listed)
			if _, err := c.Tx.Exec(ctx, `INSERT INTO identity.client_users (tenant_id, customer_id, membership_id, email, display_name, client_role, status, invited_at, invited_by_membership_id, created_at, updated_at)
				SELECT current_setting('app.tenant_id')::uuid, cu.id, $1, u.email, u.display_name, 'member', 'active', $3, $4, $3, $3
				FROM assets.customers cu, identity.users u WHERE cu.organization_id = $2 AND u.id = $5 ON CONFLICT DO NOTHING`, id, in.OrganizationID, c.Now, c.Principal.MembershipID, in.UserID); err != nil {
				return Member{}, err
			}
		}
	} else {
		x, err := scanMember(c.Tx.QueryRow(ctx, "SELECT "+memberCols+" FROM identity.memberships m WHERE m.id = $1 FOR UPDATE", *in.ID))
		if errors.Is(err, pgx.ErrNoRows) {
			return Member{}, apperr.E(apperr.NotFound, "error.notFound")
		}
		if err != nil {
			return Member{}, err
		}
		if x.UserID != in.UserID || x.OrganizationID != in.OrganizationID || x.Role != in.Role {
			return Member{}, apperr.Fields(map[string]string{"role": "errors.membership_fixed"})
		}
		if x.Version != *c.ExpectedVersion {
			return Member{}, apperr.E(apperr.Conflict, "error.versionConflict")
		}
		// last active identity.write holder
		ending := in.ValidUntil != nil && !in.ValidUntil.After(c.Now)
		if !slices.Contains(in.Permissions, "identity.write") || ending {
			var holds bool
			var others int
			if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.membership_permissions WHERE membership_id = $1 AND permission = 'identity.write')`, x.ID).Scan(&holds); err != nil {
				return Member{}, err
			}
			if err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM identity.memberships m JOIN identity.membership_permissions p ON p.membership_id = m.id AND p.permission = 'identity.write'
				WHERE m.id <> $1 AND m.valid_from <= $2 AND (m.valid_until IS NULL OR m.valid_until > $2)`, x.ID, c.Now).Scan(&others); err != nil {
				return Member{}, err
			}
			if holds && others == 0 {
				return Member{}, apperr.E(apperr.Conflict, "errors.last_identity_admin")
			}
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE identity.memberships SET employment = $2, valid_from = $3, valid_until = $4, version = version + 1, updated_at = $5 WHERE id = $1`,
			x.ID, in.Employment, in.ValidFrom, in.ValidUntil, c.Now); err != nil {
			return Member{}, err
		}
		id, prev = x.ID, c.ExpectedVersion
	}
	before := Member{ID: id}
	if prev != nil {
		ms := []Member{before}
		if err := LoadMembers(ctx, c, ms); err != nil {
			return Member{}, err
		}
		before = ms[0]
	}
	if _, err := c.Tx.Exec(ctx, `DELETE FROM identity.membership_permissions WHERE membership_id = $1`, id); err != nil {
		return Member{}, err
	}
	for _, p := range in.Permissions {
		if _, err := c.Tx.Exec(ctx, `INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES (current_setting('app.tenant_id')::uuid, $1, $2)`, id, p); err != nil {
			return Member{}, err
		}
	}
	if _, err := c.Tx.Exec(ctx, `DELETE FROM identity.membership_scopes WHERE membership_id = $1`, id); err != nil {
		return Member{}, err
	}
	for _, sc := range in.Scopes {
		if _, err := c.Tx.Exec(ctx, `INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3)
			ON CONFLICT DO NOTHING`, id, sc.Kind, sc.ID); err != nil {
			return Member{}, err
		}
	}
	if prev != nil {
		perms := slices.Clone(in.Permissions)
		slices.Sort(perms)
		changed := !slices.Equal(perms, before.Permissions) || len(in.Scopes) != len(before.Scopes)
		if !changed {
			for _, sc := range in.Scopes {
				changed = changed || !slices.Contains(before.Scopes, sc)
			}
		}
		if changed {
			if _, err := c.Tx.Exec(ctx, `UPDATE identity.memberships SET scope_version = scope_version + 1 WHERE id = $1`, id); err != nil {
				return Member{}, err
			}
		}
	}
	x, err := scanMember(c.Tx.QueryRow(ctx, "SELECT "+memberCols+" FROM identity.memberships m WHERE m.id = $1", id))
	if err != nil {
		return x, err
	}
	ms := []Member{x}
	if err := LoadMembers(ctx, c, ms); err != nil {
		return x, err
	}
	c.Emit(ops.Event{AggregateType: "membership", AggregateID: id, Type: "MembershipSaved", Payload: map[string]any{"role": in.Role}})
	c.Audit(ops.AuditEntry{Action: "members.save", TargetKind: "membership", TargetID: id.String(), PreviousVersion: prev, NextVersion: &ms[0].Version, Reason: in.Reason})
	return ms[0], nil
}

// RegisterMembers binds members.list / members.save.
func RegisterMembers(r *ops.Registry) {
	ops.Register(r, "members.list", membersList)
	ops.Register(r, "members.save", membersSave)
}
