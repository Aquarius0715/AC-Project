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

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
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
	in.Name = strings.TrimSpace(in.Name)
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

// uniqueName rejects a second organization of the same kind with the same trimmed, case-insensitive name in the
// tenant (a second customer with the same billing name is CONFLICT, IR208). It runs after the write in the same
// transaction, so NOT_FOUND and version conflicts come first and a duplicate rolls the write back.
func uniqueName(ctx context.Context, c *ops.Call, id uuid.UUID) error {
	var taken bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.organizations o, identity.organizations t WHERE t.id = $1
		AND o.kind = t.kind AND o.id <> t.id AND lower(btrim(o.name)) = lower(btrim(t.name)))`, id).Scan(&taken); err != nil {
		return err
	}
	if taken {
		return apperr.E(apperr.Conflict, "error.duplicateName")
	}
	return nil
}

func organizationsSave(ctx context.Context, c *ops.Call, in *OrganizationSave) (Organization, error) {
	if in.ID == nil {
		id := uuid.Must(uuid.NewV7())
		o, err := scanOrg(c.Tx.QueryRow(ctx, `INSERT INTO identity.organizations (id, tenant_id, name, kind, status)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4) RETURNING `+orgCols, id, in.Name, in.Kind, in.Status))
		if err != nil {
			return Organization{}, err
		}
		if err := uniqueName(ctx, c, id); err != nil {
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
	if err := uniqueName(ctx, c, o.ID); err != nil {
		return Organization{}, err
	}
	c.Audit(ops.AuditEntry{Action: "organizations.save", TargetKind: "organization", TargetID: o.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &o.Version})
	c.Emit(ops.Event{AggregateType: "organization", AggregateID: o.ID, Type: "OrganizationChanged"})
	return o, nil
}

// Directory answers membership questions for other modules. Every method is query-backed (IR192): callers in
// other domains ask identity-api; identity's own data is read only by the handlers in directory.go.
type Directory struct{}

// NonReaders returns the IDs that are not active memberships able to read the alerts of a customer organization:
// HQ memberships with alert.read or client memberships of that organization (IR120 item 3).
func (Directory) NonReaders(ctx context.Context, c *ops.Call, ids []uuid.UUID, customerOrg uuid.UUID) ([]uuid.UUID, error) {
	return ops.Delegate(ctx, c, QueryNonReaders, NonReadersInput{IDs: ids, CustomerOrgID: customerOrg}, nonReaders)
}

// OrgState returns the kind and status of an organization.
func (Directory) OrgState(ctx context.Context, c *ops.Call, org uuid.UUID) (kind, status string, found bool, err error) {
	st, err := ops.Delegate(ctx, c, QueryOrgState, OrgInput{OrgID: org}, orgState)
	return st.Kind, st.Status, st.Found, err
}

// Technician describes a role=technician membership for assignment checks.
type Technician struct {
	OrgID      uuid.UUID              `json:"orgId"`
	Employment string                 `json:"employment"`
	Active     bool                   `json:"active"`
	Scopes     map[string][]uuid.UUID `json:"scopes"`
}

// Technician loads a technician membership (found=false for unknown IDs or other roles).
func (Directory) Technician(ctx context.Context, c *ops.Call, membership uuid.UUID) (Technician, bool, error) {
	t, err := ops.Delegate(ctx, c, QueryTechnician, MembershipInput{MembershipID: membership}, technician)
	return t.Technician, t.Found, err
}

// Qualified reports whether the membership holds unrevoked grants for every code covering [start, end) (IR123 item 3).
func (Directory) Qualified(ctx context.Context, c *ops.Call, membership uuid.UUID, codes []string, start, end time.Time) (bool, error) {
	if len(codes) == 0 {
		return true, nil
	}
	return ops.Delegate(ctx, c, QueryQualified, QualifiedInput{MembershipID: membership, Codes: codes, Start: start, End: end}, qualified)
}

// OrgName returns an organization's name.
func (Directory) OrgName(ctx context.Context, c *ops.Call, org uuid.UUID) (string, error) {
	return ops.Delegate(ctx, c, QueryOrgName, OrgInput{OrgID: org}, orgName)
}

// SetGrant makes the membership's grant for code cover [from, until) without revocation (certificates.verify, IR133).
// The caller's domain publishes QualificationGranted; identity applies it (IR192).
func (Directory) SetGrant(ctx context.Context, c *ops.Call, membership uuid.UUID, code string, from, until time.Time) error {
	var tenant uuid.UUID
	if err := c.Tx.QueryRow(ctx, `SELECT current_setting('app.tenant_id')::uuid`).Scan(&tenant); err != nil {
		return err
	}
	return events.Publish(ctx, c.Tx, tenant, "membership", membership, events.QualificationGranted,
		events.Grant{MembershipID: membership, Code: code, ValidFrom: from, ValidUntil: until})
}

// ActiveClientOf reports whether a membership is an active client membership of the organization (identity.members,
// so callers in other domains ask identity-api, IR191).
func (Directory) ActiveClientOf(ctx context.Context, c *ops.Call, membership, org uuid.UUID) (bool, error) {
	ids, err := ops.Query[[]uuid.UUID](ctx, c, QueryMembers, MembersInput{IDs: []uuid.UUID{membership}, Role: "client", OrganizationID: &org})
	return len(ids) == 1, err
}
