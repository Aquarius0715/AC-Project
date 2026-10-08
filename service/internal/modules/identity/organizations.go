package identity

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/internal/ops"
	"github.com/pradita/ac-project/service/internal/platform/apperr"
	"github.com/pradita/ac-project/service/internal/platform/paging"
)

// Organization is Organization of service-contracts.ts.
type Organization struct {
	ID        uuid.UUID `json:"id"`
	TenantID  uuid.UUID `json:"tenantId"`
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`
	Name      string    `json:"name"`
	Kind      string    `json:"kind"`
	Status    string    `json:"status"`
}

const orgCols = `id, tenant_id, version, created_at, updated_at, name, kind, status`

func scanOrg(r pgx.Row) (Organization, error) {
	var o Organization
	err := r.Scan(&o.ID, &o.TenantID, &o.Version, &o.CreatedAt, &o.UpdatedAt, &o.Name, &o.Kind, &o.Status)
	return o, err
}

var orgKinds = map[string]bool{"customer": true, "contractor": true, "operator": true}

func organizationsList(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Organization], error) {
	var f struct {
		Kind   *string `json:"kind,omitempty"`
		Status *string `json:"status,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil {
			return paging.Page[Organization]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"TRUE"}
	if f.Kind != nil {
		if !orgKinds[*f.Kind] {
			return paging.Page[Organization]{}, apperr.Fields(map[string]string{"filters.kind": "error.invalid"})
		}
		conds = append(conds, "kind = "+add(*f.Kind))
	}
	if f.Status != nil {
		if *f.Status != "active" && *f.Status != "inactive" {
			return paging.Page[Organization]{}, apperr.Fields(map[string]string{"filters.status": "error.invalid"})
		}
		conds = append(conds, "status = "+add(*f.Status))
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "id", "createdAt": "created_at", "updatedAt": "updated_at"}, "id ASC")
	if err != nil {
		return paging.Page[Organization]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Organization]{}, err
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM identity.organizations WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Organization]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM identity.organizations WHERE %s ORDER BY %s LIMIT %d OFFSET %d", orgCols, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Organization]{}, err
	}
	defer rows.Close()
	items := []Organization{}
	for rows.Next() {
		o, err := scanOrg(rows)
		if err != nil {
			return paging.Page[Organization]{}, err
		}
		items = append(items, o)
	}
	return paging.Page[Organization]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, rows.Err()
}

// OrganizationSave is organizations.save input (Save<Organization>).
type OrganizationSave struct {
	ID     *uuid.UUID `json:"id,omitempty"`
	Name   string     `json:"name"`
	Kind   string     `json:"kind"`
	Status string     `json:"status"`
}

// Validate implements ops.Validator (DD-A02: name 1–120).
func (in *OrganizationSave) Validate() map[string]string {
	fe := map[string]string{}
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > 120 {
		fe["name"] = "error.length"
	}
	if !orgKinds[in.Kind] {
		fe["kind"] = "error.invalid"
	}
	if in.Status != "active" && in.Status != "inactive" {
		fe["status"] = "error.invalid"
	}
	return fe
}

func organizationsSave(ctx context.Context, c *ops.Call, in *OrganizationSave) (Organization, error) {
	if in.ID == nil {
		id := uuid.Must(uuid.NewV7())
		o, err := scanOrg(c.Tx.QueryRow(ctx, `INSERT INTO identity.organizations (id, tenant_id, name, kind, status)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4) RETURNING `+orgCols, id, in.Name, in.Kind, in.Status))
		if err != nil {
			return Organization{}, err
		}
		c.Audit(ops.AuditEntry{Action: "organizations.save", TargetKind: "organization", TargetID: id.String(), NextVersion: &o.Version})
		c.Emit(ops.Event{AggregateType: "organization", AggregateID: id, Type: "OrganizationChanged"})
		return o, nil
	}
	// kind is fixed after creation
	o, err := scanOrg(c.Tx.QueryRow(ctx, `UPDATE identity.organizations SET name=$3, status=$4, version = version + 1, updated_at = platform.app_now()
		WHERE id = $1 AND version = $2 AND kind = $5 RETURNING `+orgCols, *in.ID, *c.ExpectedVersion, in.Name, in.Status, in.Kind))
	if errors.Is(err, pgx.ErrNoRows) {
		var kind string
		if e := c.Tx.QueryRow(ctx, `SELECT kind FROM identity.organizations WHERE id = $1`, *in.ID).Scan(&kind); e != nil {
			return Organization{}, apperr.E(apperr.NotFound, "error.notFound")
		}
		if kind != in.Kind {
			return Organization{}, apperr.Fields(map[string]string{"kind": "error.immutable"})
		}
		return Organization{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if err != nil {
		return Organization{}, err
	}
	c.Audit(ops.AuditEntry{Action: "organizations.save", TargetKind: "organization", TargetID: o.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &o.Version})
	c.Emit(ops.Event{AggregateType: "organization", AggregateID: o.ID, Type: "OrganizationChanged"})
	return o, nil
}

// Directory answers membership questions for other modules.
type Directory struct{}

// NonReaders returns the IDs that are not active memberships able to read the alerts of a customer organization:
// HQ memberships with alert.read or client memberships of that organization (IR120 item 3).
func (Directory) NonReaders(ctx context.Context, c *ops.Call, ids []uuid.UUID, customerOrg uuid.UUID) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, `SELECT m.id FROM identity.memberships m
		WHERE m.id = ANY($1) AND m.valid_from <= $3 AND (m.valid_until IS NULL OR m.valid_until > $3)
		  AND ((m.role = 'admin' AND EXISTS (SELECT 1 FROM identity.membership_permissions p WHERE p.membership_id = m.id AND p.permission = 'alert.read'))
		    OR (m.role = 'client' AND m.organization_id = $2))`, ids, customerOrg, c.Now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	ok := map[uuid.UUID]bool{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		ok[id] = true
	}
	var bad []uuid.UUID
	for _, id := range ids {
		if !ok[id] {
			bad = append(bad, id)
		}
	}
	return bad, rows.Err()
}

// OrgState returns the kind and status of an organization.
func (Directory) OrgState(ctx context.Context, c *ops.Call, org uuid.UUID) (kind, status string, found bool, err error) {
	err = c.Tx.QueryRow(ctx, `SELECT kind, status FROM identity.organizations WHERE id = $1`, org).Scan(&kind, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", "", false, nil
	}
	return kind, status, err == nil, err
}

// Technician describes a role=technician membership for assignment checks.
type Technician struct {
	OrgID      uuid.UUID
	Employment string
	Active     bool
	Scopes     map[string][]uuid.UUID
}

// Technician loads a technician membership (found=false for unknown IDs or other roles).
func (Directory) Technician(ctx context.Context, c *ops.Call, membership uuid.UUID) (Technician, bool, error) {
	var t Technician
	var emp *string
	var from time.Time
	var until *time.Time
	err := c.Tx.QueryRow(ctx, `SELECT organization_id, employment, valid_from, valid_until FROM identity.memberships WHERE id = $1 AND role = 'technician'`, membership).
		Scan(&t.OrgID, &emp, &from, &until)
	if errors.Is(err, pgx.ErrNoRows) {
		return t, false, nil
	}
	if err != nil {
		return t, false, err
	}
	if emp != nil {
		t.Employment = *emp
	}
	t.Active = !from.After(c.Now) && (until == nil || until.After(c.Now))
	t.Scopes = map[string][]uuid.UUID{}
	rows, err := c.Tx.Query(ctx, `SELECT kind, ref_id FROM identity.membership_scopes WHERE membership_id = $1`, membership)
	if err != nil {
		return t, false, err
	}
	defer rows.Close()
	for rows.Next() {
		var k string
		var id uuid.UUID
		if err := rows.Scan(&k, &id); err != nil {
			return t, false, err
		}
		t.Scopes[k] = append(t.Scopes[k], id)
	}
	return t, true, rows.Err()
}

// Qualified reports whether the membership holds unrevoked grants for every code covering [start, end) (IR123 item 3).
func (Directory) Qualified(ctx context.Context, c *ops.Call, membership uuid.UUID, codes []string, start, end time.Time) (bool, error) {
	if len(codes) == 0 {
		return true, nil
	}
	var n int
	err := c.Tx.QueryRow(ctx, `SELECT count(DISTINCT code) FROM identity.qualification_grants WHERE membership_id = $1 AND code = ANY($2)
		AND valid_from <= $3 AND valid_until >= $4 AND revoked_at IS NULL`, membership, codes, start, end).Scan(&n)
	return n == len(codes), err
}

// OrgName returns an organization's name.
func (Directory) OrgName(ctx context.Context, c *ops.Call, org uuid.UUID) (string, error) {
	var name string
	err := c.Tx.QueryRow(ctx, `SELECT name FROM identity.organizations WHERE id = $1`, org).Scan(&name)
	return name, err
}

// SetGrant makes the membership's grant for code cover [from, until) without revocation (certificates.verify, IR133).
func (Directory) SetGrant(ctx context.Context, c *ops.Call, membership uuid.UUID, code string, from, until time.Time) error {
	tag, err := c.Tx.Exec(ctx, `UPDATE identity.qualification_grants SET valid_from = $3, valid_until = $4, revoked_at = NULL WHERE membership_id = $1 AND code = $2`,
		membership, code, from, until)
	if err != nil || tag.RowsAffected() > 0 {
		return err
	}
	_, err = c.Tx.Exec(ctx, `INSERT INTO identity.qualification_grants (tenant_id, membership_id, code, valid_from, valid_until) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4)`,
		membership, code, from, until)
	return err
}

// ActiveClientOf reports whether a membership is an active client membership of the organization.
func (Directory) ActiveClientOf(ctx context.Context, c *ops.Call, membership, org uuid.UUID) (bool, error) {
	var ok bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.memberships WHERE id = $1 AND role = 'client' AND organization_id = $2 AND valid_from <= $3
		AND (valid_until IS NULL OR valid_until > $3))`, membership, org, c.Now).Scan(&ok)
	return ok, err
}
