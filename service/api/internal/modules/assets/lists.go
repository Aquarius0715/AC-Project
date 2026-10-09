package assets

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

var listSort = map[string]string{"id": "id", "createdAt": "created_at", "updatedAt": "updated_at"}

func decodeFilters(raw json.RawMessage, dst any) error {
	if len(raw) == 0 {
		return nil
	}
	dec := json.NewDecoder(strings.NewReader(string(raw)))
	dec.DisallowUnknownFields()
	if dec.Decode(dst) != nil {
		return apperr.Fields(map[string]string{"filters": "error.invalid"})
	}
	return nil
}

// list runs a scoped, filtered, paged query; scan reads one row of cols.
func list[T any](ctx context.Context, c *ops.Call, in *paging.Query, f any, table, cols string, conds []string, args []any, scan func(pgx.Row) (T, error)) (paging.Page[T], error) {
	order, err := paging.OrderBy(in.Sort, listSort, "id ASC")
	if err != nil {
		return paging.Page[T]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[T]{}, err
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[T]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d", cols, table, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[T]{}, err
	}
	defer rows.Close()
	items := []T{}
	for rows.Next() {
		v, err := scan(rows)
		if err != nil {
			return paging.Page[T]{}, err
		}
		items = append(items, v)
	}
	if err := rows.Err(); err != nil {
		return paging.Page[T]{}, err
	}
	return paging.Page[T]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

// clientOrg restricts client callers to their own customer organization (client:self-customer).
func clientOrg(c *ops.Call, column string, args *[]any) string {
	if c.Principal.Role != "client" {
		return "TRUE"
	}
	*args = append(*args, c.Principal.OrgID)
	return fmt.Sprintf("%s = $%d", column, len(*args))
}

// ---- customers ----

// Customer is Customer of service-contracts.ts.
type Customer struct {
	ID             uuid.UUID `json:"id"`
	TenantID       uuid.UUID `json:"tenantId"`
	Version        int       `json:"version"`
	CreatedAt      time.Time `json:"createdAt"`
	UpdatedAt      time.Time `json:"updatedAt"`
	Name           string    `json:"name"`
	Status         string    `json:"status"`
	OrganizationID uuid.UUID `json:"organizationId"`
	ServiceProfile string    `json:"serviceProfile"`
}

const customerCols = `id, tenant_id, version, created_at, updated_at, name, status, organization_id, service_profile`

func scanCustomer(r pgx.Row) (Customer, error) {
	var x Customer
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.Name, &x.Status, &x.OrganizationID, &x.ServiceProfile)
	return x, err
}

var profiles = map[string]bool{"rto": true, "general": true, "energy": true, "environment": true}

// @Summary		customers.list (read)
// @ID				customers.list
// @Description	Authorization: admin:asset.read | admin:contract.read:scope-candidate-read-only | admin:offset.read:scope-candidate-read-only
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A02, DD-A07, DD-A15, DD-A05, DD-A18, DD-A06 · Query: filters kind,status,organizationId · sort id,createdAt,updatedAt (default id asc)
// @Tags			customers
// @Accept			json
// @Produce		json
// @Param			cursor			query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit			query		integer	false	"page size 1–100, default 25"
// @Param			sort			query		string	false	"field:direction — fields id,createdAt,updatedAt; default id asc"
// @Param			kind			query		string	false	"filter → serviceProfile"
// @Param			status			query		string	false	"filter → status"
// @Param			organizationId	query		string	false	"filter → organizationId"
// @Success		200				{object}	ops.Envelope{data=CustomerPage}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/customers [get]
func (m *Module) customersList(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Customer], error) {
	var f struct {
		Kind           *string    `json:"kind,omitempty"`
		Status         *string    `json:"status,omitempty"`
		OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
	}
	if err := decodeFilters(in.Filters, &f); err != nil {
		return paging.Page[Customer]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"TRUE"}
	if f.Kind != nil {
		if !profiles[*f.Kind] {
			return paging.Page[Customer]{}, apperr.Fields(map[string]string{"filters.kind": "error.invalid"})
		}
		conds = append(conds, "service_profile = "+add(*f.Kind))
	}
	if f.Status != nil {
		if *f.Status != "active" && *f.Status != "inactive" {
			return paging.Page[Customer]{}, apperr.Fields(map[string]string{"filters.status": "error.invalid"})
		}
		conds = append(conds, "status = "+add(*f.Status))
	} else {
		conds = append(conds, "status = 'active'") // inactive customers only with Status: All / inactive (IR40)
	}
	if f.OrganizationID != nil {
		conds = append(conds, "organization_id = "+add(*f.OrganizationID))
	}
	return list(ctx, c, in, f, "assets.customers", customerCols, conds, args, scanCustomer)
}

// CustomerSave is customers.save input.
type CustomerSave struct {
	ID             *uuid.UUID `json:"id,omitempty"`
	Name           string     `json:"name"`
	OrganizationID uuid.UUID  `json:"organizationId"`
	ServiceProfile string     `json:"serviceProfile"`
	Status         string     `json:"status"`
}

// Validate implements ops.Validator.
func (in *CustomerSave) Validate() map[string]string {
	fe := map[string]string{}
	in.Name = strings.TrimSpace(in.Name)
	if !strLen(in.Name, 1, 120) {
		fe["name"] = "error.length"
	}
	if in.OrganizationID == uuid.Nil {
		fe["organizationId"] = "error.required"
	}
	if !profiles[in.ServiceProfile] {
		fe["serviceProfile"] = "error.invalid"
	}
	if in.Status != "active" && in.Status != "inactive" {
		fe["status"] = "error.invalid"
	}
	return fe
}

// @Summary		customers.save (write)
// @ID				customers.save
// @Description	Authorization: admin:asset.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-A02
// @Tags			customers
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		CustomerSave	true	"input"
// @Success		200				{object}	ops.Envelope{data=Customer}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/customers [post]
func (m *Module) customersSave(ctx context.Context, c *ops.Call, in *CustomerSave) (Customer, error) {
	if in.ID == nil {
		id := uuid.Must(uuid.NewV7())
		x, err := scanCustomer(c.Tx.QueryRow(ctx, `INSERT INTO assets.customers (id, tenant_id, organization_id, name, status, service_profile)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5) RETURNING `+customerCols, id, in.OrganizationID, in.Name, in.Status, in.ServiceProfile))
		if err != nil {
			return Customer{}, err // 23505: organization already has a customer → CONFLICT
		}
		c.Audit(ops.AuditEntry{Action: "customers.save", TargetKind: "customer", TargetID: id.String(), NextVersion: &x.Version})
		c.Emit(ops.Event{AggregateType: "customer", AggregateID: id, Type: "CustomerChanged"})
		return x, nil
	}
	if in.Status == "inactive" { // D05: deactivation conflicts with active units, contracts or jobs
		var units bool
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.units WHERE customer_org_id = $1 AND NOT archived)`, in.OrganizationID).Scan(&units); err != nil {
			return Customer{}, err
		}
		if units {
			return Customer{}, conflict("error.customerActive")
		}
		for _, a := range m.OrgActive {
			busy, err := a.OrgActive(ctx, c, in.OrganizationID)
			if err != nil {
				return Customer{}, err
			}
			if busy {
				return Customer{}, conflict("error.customerActive")
			}
		}
	}
	x, err := scanCustomer(c.Tx.QueryRow(ctx, `UPDATE assets.customers SET name=$3, status=$4, service_profile=$5, version = version + 1, updated_at = platform.app_now()
		WHERE id = $1 AND version = $2 AND organization_id = $6 RETURNING `+customerCols, *in.ID, *c.ExpectedVersion, in.Name, in.Status, in.ServiceProfile, in.OrganizationID))
	if err != nil {
		return Customer{}, versionedUpdate(ctx, c, "assets.customers", *in.ID, err)
	}
	c.Audit(ops.AuditEntry{Action: "customers.save", TargetKind: "customer", TargetID: x.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &x.Version})
	c.Emit(ops.Event{AggregateType: "customer", AggregateID: x.ID, Type: "CustomerChanged"})
	return x, nil
}

// ---- properties / spaces lists ----

// @Summary		properties.list (read)
// @ID				properties.list
// @Description	Authorization: client:self-customer | admin:asset.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A02, DD-C02, DD-C07, DD-C04, DD-A06, DD-C09 · Query: filters customerId,kind · sort id,createdAt,updatedAt (default id asc)
// @Tags			properties
// @Accept			json
// @Produce		json
// @Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit		query		integer	false	"page size 1–100, default 25"
// @Param			sort		query		string	false	"field:direction — fields id,createdAt,updatedAt; default id asc"
// @Param			customerId	query		string	false	"filter → Customer.organizationId = Property.customerOrgId"
// @Param			kind		query		string	false	"filter → kind"
// @Success		200			{object}	ops.Envelope{data=PropertyPage}
// @Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504			{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/properties [get]
func (m *Module) propertiesList(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Property], error) {
	var f struct {
		CustomerID *uuid.UUID `json:"customerId,omitempty"`
		Kind       *string    `json:"kind,omitempty"`
	}
	if err := decodeFilters(in.Filters, &f); err != nil {
		return paging.Page[Property]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"NOT archived", clientOrg(c, "customer_org_id", &args)}
	if f.CustomerID != nil {
		conds = append(conds, "customer_org_id = (SELECT organization_id FROM assets.customers WHERE id = "+add(*f.CustomerID)+")")
	}
	if f.Kind != nil {
		if *f.Kind != "home" && *f.Kind != "office" {
			return paging.Page[Property]{}, apperr.Fields(map[string]string{"filters.kind": "error.invalid"})
		}
		conds = append(conds, "kind = "+add(*f.Kind))
	}
	return list(ctx, c, in, f, "assets.properties", propertyCols, conds, args, scanProperty)
}

// @Summary		spaces.list (read)
// @ID				spaces.list
// @Description	Authorization: client:self-customer | admin:asset.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-A02, DD-C02, DD-C07, DD-C04, DD-C09 · Query: filters propertyId,kind · sort id,createdAt,updatedAt (default id asc)
// @Tags			spaces
// @Accept			json
// @Produce		json
// @Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit		query		integer	false	"page size 1–100, default 25"
// @Param			sort		query		string	false	"field:direction — fields id,createdAt,updatedAt; default id asc"
// @Param			propertyId	query		string	false	"filter → propertyId"
// @Param			kind		query		string	false	"filter → kind"
// @Success		200			{object}	ops.Envelope{data=SpacePage}
// @Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504			{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/spaces [get]
func (m *Module) spacesList(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Space], error) {
	var f struct {
		PropertyID *uuid.UUID `json:"propertyId,omitempty"`
		Kind       *string    `json:"kind,omitempty"`
	}
	if err := decodeFilters(in.Filters, &f); err != nil {
		return paging.Page[Space]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"NOT archived"}
	if c.Principal.Role == "client" {
		conds = append(conds, "property_id IN (SELECT id FROM assets.properties WHERE customer_org_id = "+add(c.Principal.OrgID)+")")
	}
	if f.PropertyID != nil {
		conds = append(conds, "property_id = "+add(*f.PropertyID))
	}
	if f.Kind != nil {
		if !spaceKinds[*f.Kind] {
			return paging.Page[Space]{}, apperr.Fields(map[string]string{"filters.kind": "error.invalid"})
		}
		conds = append(conds, "kind = "+add(*f.Kind))
	}
	return list(ctx, c, in, f, "assets.spaces", spaceCols, conds, args, scanSpace)
}

// ---- locations.rename (IR109) ----

// RenameInput is locations.rename input.
type RenameInput struct {
	Target struct {
		Kind string    `json:"kind"`
		ID   uuid.UUID `json:"id"`
	} `json:"target"`
	Name string `json:"name"`
}

// Validate implements ops.Validator.
func (in *RenameInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.Target.Kind != "property" && in.Target.Kind != "space" && in.Target.Kind != "unit" {
		fe["target.kind"] = "error.invalid"
	}
	if in.Target.ID == uuid.Nil {
		fe["target.id"] = "error.required"
	}
	if !strLen(strings.TrimSpace(in.Name), 1, 120) {
		fe["name"] = "error.length"
	}
	return fe
}

// @Summary		locations.rename (write)
// @ID				locations.rename
// @Description	Authorization: client:self-customer | admin:asset.write
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR109 name 1–120 characters, unique among siblings; structure fields are not accepted
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-C02, DD-C03, DD-A02
// @Tags			locations
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			X-Expected-Version	header		integer		true	"all: required (target property|space|unit, read properties.list;spaces.list;units.get)"
// @Param			target.kind			path		string		true	"input field target.kind"
// @Param			target.id			path		string		true	"input field target.id"
// @Param			request				body		RenameInput	true	"input; the path parameters come from the route"
// @Success		200					{object}	ops.Envelope
// @Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504					{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/locations/{target.kind}/{target.id}/rename [post]
func (m *Module) locationsRename(ctx context.Context, c *ops.Call, in *RenameInput) (any, error) {
	name := strings.TrimSpace(in.Name)
	var table, col, org, siblings string
	switch in.Target.Kind {
	case "property":
		table, col, org = "assets.properties", "name", "customer_org_id"
		siblings = `SELECT 1 FROM assets.properties o, assets.properties t WHERE t.id = $1 AND o.customer_org_id = t.customer_org_id AND o.id <> t.id AND NOT o.archived AND lower(o.name) = lower($2)`
	case "space":
		table, col, org = "assets.spaces", "name", "(SELECT customer_org_id FROM assets.properties p WHERE p.id = property_id)"
		siblings = `SELECT 1 FROM assets.spaces o, assets.spaces t WHERE t.id = $1 AND o.property_id = t.property_id AND o.parent_space_id IS NOT DISTINCT FROM t.parent_space_id AND o.id <> t.id AND NOT o.archived AND lower(o.name) = lower($2)`
	default:
		table, col, org = "assets.units", "display_name", "customer_org_id"
		siblings = `SELECT 1 FROM assets.units o, assets.units t WHERE t.id = $1 AND o.property_id = t.property_id AND o.space_id IS NOT DISTINCT FROM t.space_id AND o.id <> t.id AND NOT o.archived AND lower(o.display_name) = lower($2)`
	}
	visArgs, updArgs := []any{in.Target.ID}, []any{in.Target.ID, *c.ExpectedVersion, name}
	visScope, updScope := "TRUE", "TRUE"
	if c.Principal.Role == "client" { // client:self-customer
		visArgs, updArgs = append(visArgs, c.Principal.OrgID), append(updArgs, c.Principal.OrgID)
		visScope, updScope = org+" = $2", org+" = $4"
	}
	var visible bool
	if err := c.Tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM "+table+" WHERE id = $1 AND NOT archived AND "+visScope+")", visArgs...).Scan(&visible); err != nil {
		return nil, err
	}
	if !visible {
		return nil, notFound()
	}
	var dup bool
	if err := c.Tx.QueryRow(ctx, "SELECT EXISTS ("+siblings+")", in.Target.ID, name).Scan(&dup); err != nil {
		return nil, err
	}
	if dup {
		return nil, conflict("error.duplicateSiblingName")
	}
	var v int
	err := c.Tx.QueryRow(ctx, "UPDATE "+table+" SET "+col+" = $3, version = version + 1, updated_at = platform.app_now() WHERE id = $1 AND version = $2 AND NOT archived AND "+updScope+" RETURNING version", updArgs...).Scan(&v)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, conflict("error.versionConflict")
	}
	if err != nil {
		return nil, err
	}
	typ := "LocationChanged"
	if in.Target.Kind == "unit" {
		typ = "UnitChanged"
	}
	c.Emit(ops.Event{AggregateType: in.Target.Kind, AggregateID: in.Target.ID, Type: typ})
	c.Audit(ops.AuditEntry{Action: "locations.rename", TargetKind: in.Target.Kind, TargetID: in.Target.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &v})
	switch in.Target.Kind {
	case "property":
		return scanProperty(c.Tx.QueryRow(ctx, "SELECT "+propertyCols+" FROM assets.properties WHERE id = $1", in.Target.ID))
	case "space":
		return scanSpace(c.Tx.QueryRow(ctx, "SELECT "+spaceCols+" FROM assets.spaces WHERE id = $1", in.Target.ID))
	}
	return m.load(ctx, c, in.Target.ID)
}
