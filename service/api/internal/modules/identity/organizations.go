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

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
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
	var organization Organization

	err := r.Scan(
		&organization.ID,
		&organization.TenantID,
		&organization.Version,
		&organization.CreatedAt,
		&organization.UpdatedAt,
		&organization.Name,
		&organization.Kind,
		&organization.Status,
	)

	return organization, apperr.Wrap(err, "scan (scanOrg)")
}

// organizationKinds are the kinds of an organization.
func organizationKinds() []string {
	return []string{orgKindCustomer, orgKindContractor, orgKindOperator}
}

// organizationFilter is the filters of organizations.list.
type organizationFilter struct {
	Kind   *string `json:"kind,omitempty"`
	Status *string `json:"status,omitempty"`
}

// organizationFilterOf decodes and checks the filters of organizations.list: known fields only, a known kind and a
// known status.
func organizationFilterOf(raw json.RawMessage) (organizationFilter, error) {
	var filter organizationFilter

	if len(raw) == 0 {
		return filter, nil
	}

	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()

	if dec.Decode(&filter) != nil {
		return organizationFilter{}, apperr.Fields(map[string]string{keyFilters: apperr.KeyInvalid})
	}

	if filter.Kind != nil && !slices.Contains(organizationKinds(), *filter.Kind) {
		return organizationFilter{}, apperr.Fields(
			map[string]string{"filters.kind": apperr.KeyInvalid},
		)
	}

	if filter.Status != nil && *filter.Status != statusActive && *filter.Status != "inactive" {
		return organizationFilter{}, apperr.Fields(
			map[string]string{"filters.status": apperr.KeyInvalid},
		)
	}

	return filter, nil
}

// where is the filter's WHERE clause, with its arguments.
func (filter organizationFilter) where() (string, []any) {
	var args []any

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{everyRow}
	if filter.Kind != nil {
		conds = append(conds, "kind = "+add(*filter.Kind))
	}

	if filter.Status != nil {
		conds = append(conds, "status = "+add(*filter.Status))
	}

	return strings.Join(conds, " AND "), args
}

// organizationsList serves organizations.list.
//
//	@Summary		organizations.list (read)
//	@ID				organizations.list
//	@Description	Authorization: admin:asset.read | admin:job.read | admin:identity.read | admin:mrv.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A02, DD-A06, DD-A03, DD-A14 · Query: filters kind,status · sort id,createdAt,updatedAt \
//	@Description	(default id asc)
//	@Tags			organizations
//	@Accept			json
//	@Produce		json
//	@Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer	false	"page size 1–100, default 25"
//	@Param			sort	query		string	false	"field:direction — fields id,createdAt,updatedAt; default id asc"
//	@Param			kind	query		string	false	"filter → kind"
//	@Param			status	query		string	false	"filter → status"
//	@Success		200		{object}	ops.Envelope{data=OrganizationPage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/organizations [get]
func organizationsList(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Organization], error) {
	filters, err := organizationFilterOf(query.Filters)
	if err != nil {
		return paging.Page[Organization]{}, err
	}

	where, args := filters.where()

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{"id": "id", keyCreatedAt: "created_at", keyUpdatedAt: "updated_at"},
		"id ASC",
	)
	if err != nil {
		return paging.Page[Organization]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Organization]{}, apperr.Wrap(err, "paging.Resolve")
	}

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM identity.organizations WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Organization]{}, apperr.Wrap(scanErr, "select identity.organizations")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM identity.organizations WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			orgCols,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[Organization]{}, apperr.Wrap(err, "select identity.organizations")
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

	return paging.Page[Organization]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, apperr.Wrap(rows.Err(), "select identity.organizations")
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
	fieldErrs := map[string]string{}

	in.Name = strings.TrimSpace(in.Name)
	if n := utf8.RuneCountInString(in.Name); n < 1 || n > 120 {
		fieldErrs["name"] = apperr.KeyLength
	}

	if !slices.Contains(organizationKinds(), in.Kind) {
		fieldErrs[keyKind] = apperr.KeyInvalid
	}

	if in.Status != statusActive && in.Status != "inactive" {
		fieldErrs[keyStatus] = apperr.KeyInvalid
	}

	return fieldErrs
}

// uniqueName rejects a second organization of the same kind with the same trimmed, case-insensitive name in the
// tenant (a second customer with the same billing name is CONFLICT, IR208). It runs after the write in the same
// transaction, so NOT_FOUND and version conflicts come first and a duplicate rolls the write back.
func uniqueName(ctx context.Context, c *ops.Call, id uuid.UUID) error {
	var taken bool

	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM identity.organizations o, identity.organizations t WHERE t.id =
		$1
		AND o.kind = t.kind AND o.id <> t.id AND lower(btrim(o.name)) = lower(btrim(t.name)))`, id).
		Scan(&taken)
	if err != nil {
		return apperr.Wrap(err, "select identity.organizations")
	}

	if taken {
		return apperr.E(apperr.Conflict, apperr.KeyDuplicateName)
	}

	return nil
}

// organizationsSave serves organizations.save.
//
//	@Summary		organizations.save (write)
//	@ID				organizations.save
//	@Description	Authorization: admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A02, DD-A21
//	@Tags			organizations
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string				true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		OrganizationSave	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Organization}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/organizations [post]
func organizationsSave(
	ctx context.Context,
	call *ops.Call,
	input *OrganizationSave,
) (Organization, error) {
	var (
		org  Organization
		prev *int
		err  error
	)

	if input.ID == nil {
		org, err = insertOrganization(ctx, call, input)
	} else {
		org, err = updateOrganization(ctx, call, input)
		prev = call.ExpectedVersion
	}

	if err != nil {
		return Organization{}, err
	}

	err = uniqueName(ctx, call, org.ID)
	if err != nil {
		return Organization{}, err
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "organizations.save",
			TargetKind:      scopeOrganization,
			TargetID:        org.ID.String(),
			PreviousVersion: prev,
			NextVersion:     &org.Version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)
	call.Emit(
		ops.Event{
			AggregateType: scopeOrganization,
			AggregateID:   org.ID,
			Type:          "OrganizationChanged",
			Payload:       nil,
		},
	)

	return org, nil
}

// insertOrganization stores a new organization.
func insertOrganization(
	ctx context.Context,
	call *ops.Call,
	input *OrganizationSave,
) (Organization, error) {
	return scanOrg(
		call.Tx.QueryRow(
			ctx,
			`INSERT INTO identity.organizations (id, tenant_id, name, kind, status)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4) RETURNING `+orgCols,
			uuid.Must(uuid.NewV7()),
			input.Name,
			input.Kind,
			input.Status,
		),
	)
}

// updateOrganization stores a new version of an organization at the version shown; its kind is fixed after creation.
// An update that matches no row tells why: an unknown organization, another kind, or a stale version.
func updateOrganization(
	ctx context.Context,
	call *ops.Call,
	input *OrganizationSave,
) (Organization, error) {
	org, err := scanOrg(
		call.Tx.QueryRow(
			ctx,
			`UPDATE identity.organizations SET name=$3, status=$4, version = version + 1, updated_at = platform.app_now()
		WHERE id = $1 AND version = $2 AND kind = $5 RETURNING `+orgCols,
			*input.ID,
			*call.ExpectedVersion,
			input.Name,
			input.Status,
			input.Kind,
		),
	)
	if !errors.Is(err, pgx.ErrNoRows) {
		return org, err
	}

	var kind string

	err = call.Tx.QueryRow(ctx, `SELECT kind FROM identity.organizations WHERE id = $1`, *input.ID).
		Scan(&kind)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return Organization{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	case err != nil:
		return Organization{}, apperr.Wrap(err, "select identity.organizations")
	case kind != input.Kind:
		return Organization{}, apperr.Fields(map[string]string{keyKind: apperr.KeyImmutable})
	}

	return Organization{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
}

// Directory answers membership questions for other modules. Every method is query-backed (IR192): callers in
// other domains ask identity-api; identity's own data is read only by the handlers in directory.go.
type Directory struct{}

// NonReaders returns the IDs that are not active memberships able to read the alerts of a customer organization:
// HQ memberships with alert.read or client memberships of that organization (IR120 item 3).
func (Directory) NonReaders(
	ctx context.Context,
	call *ops.Call,
	ids []uuid.UUID,
	customerOrg uuid.UUID,
) ([]uuid.UUID, error) {
	out, err := ops.Delegate(
		ctx,
		call,
		QueryNonReaders,
		NonReadersInput{IDs: ids, CustomerOrgID: customerOrg},
		nonReaders,
	)

	return out, apperr.Wrap(err, "delegate identity.nonReaders")
}

// OrgState returns the kind and status of an organization, and whether it exists.
func (Directory) OrgState(ctx context.Context, c *ops.Call, org uuid.UUID) (OrgStateResult, error) {
	st, err := ops.Delegate(ctx, c, QueryOrgState, OrgInput{OrgID: org}, orgState)

	return st, apperr.Wrap(err, "delegate identity.orgState")
}

// Technician describes a role=technician membership for assignment checks.
type Technician struct {
	OrgID      uuid.UUID              `json:"orgId"`
	Employment string                 `json:"employment"`
	Active     bool                   `json:"active"`
	ValidUntil *time.Time             `json:"validUntil"` // the end of the membership's access, nil for none (IR433)
	Scopes     map[string][]uuid.UUID `json:"scopes"`
}

// Technician loads a technician membership (found=false for unknown IDs or other roles).
func (Directory) Technician(
	ctx context.Context,
	c *ops.Call,
	membership uuid.UUID,
) (Technician, bool, error) {
	technicianResult, err := ops.Delegate(
		ctx,
		c,
		QueryTechnician,
		MembershipInput{MembershipID: membership},
		technician,
	)

	return technicianResult.Technician, technicianResult.Found, apperr.Wrap(
		err,
		"delegate identity.technician",
	)
}

// Qualified reports whether the membership holds unrevoked grants for every code covering [start, end) (IR123 item 3).
func (Directory) Qualified(
	ctx context.Context,
	call *ops.Call,
	membership uuid.UUID,
	codes []string,
	start, end time.Time,
) (bool, error) {
	if len(codes) == 0 {
		return true, nil
	}

	out, err := ops.Delegate(
		ctx,
		call,
		QueryQualified,
		QualifiedInput{MembershipID: membership, Codes: codes, Start: start, End: end},
		qualified,
	)

	return out, apperr.Wrap(err, "delegate identity.qualified")
}

// OrgName returns an organization's name.
func (Directory) OrgName(ctx context.Context, c *ops.Call, org uuid.UUID) (string, error) {
	out, err := ops.Delegate(ctx, c, QueryOrgName, OrgInput{OrgID: org}, orgName)

	return out, apperr.Wrap(err, "delegate identity.orgName")
}

// SetGrant makes the membership's grant for code cover [from, until) without revocation (certificates.verify, IR133).
// The caller's domain publishes QualificationGranted; identity applies it (IR192).
func (Directory) SetGrant(
	ctx context.Context,
	call *ops.Call,
	membership uuid.UUID,
	code string,
	from, until time.Time,
) error {
	var tenant uuid.UUID

	err := call.Tx.QueryRow(ctx, `SELECT current_setting('app.tenant_id')::uuid`).
		Scan(&tenant)
	if err != nil {
		return apperr.Wrap(err, "select current_setting")
	}

	return apperr.Wrap(
		events.Publish(
			ctx,
			call.Tx,
			tenant,
			kindMembership,
			membership,
			events.QualificationGranted,
			events.Grant{
				MembershipID: membership,
				Code:         code,
				ValidFrom:    from,
				ValidUntil:   until,
			},
		),
		"events.Publish",
	)
}

// ActiveClientOf reports whether a membership is an active client membership of the organization (identity.members,
// so callers in other domains ask identity-api, IR191) that a notice can reach on the channel ("" asks for none, IR779).
func (Directory) ActiveClientOf(
	ctx context.Context,
	c *ops.Call,
	membership, org uuid.UUID,
	channel string,
) (bool, error) {
	ids, err := ops.Query[[]uuid.UUID](
		ctx,
		c,
		QueryMembers,
		MembersInput{
			IDs:            []uuid.UUID{membership},
			Role:           authz.RoleClient,
			OrganizationID: &org,
			Permission:     "",
			ClientRole:     "",
			Channel:        channel,
		},
	)

	return len(ids) == 1, err
}
