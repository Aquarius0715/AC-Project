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
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
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
		return apperr.Fields(map[string]string{"filters": apperr.KeyInvalid})
	}

	return nil
}

// list runs a scoped, filtered, paged query; scan reads one row of cols.
func list[T any](
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
	filters any,
	table, cols string,
	conds []string,
	args []any,
	scan func(pgx.Row) (T, error),
) (paging.Page[T], error) {
	order, err := paging.OrderBy(query.Sort, listSort, "id ASC")
	if err != nil {
		return paging.Page[T]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[T]{}, apperr.Wrap(err, "paging.Resolve")
	}

	where := strings.Join(conds, " AND ")

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM "+table+" WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[T]{}, apperr.Wrap(scanErr, "select count")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM %s WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			cols,
			table,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[T]{}, apperr.Wrap(err, "select (list)")
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

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[T]{}, apperr.Wrap(rowsErr, "select (list)")
	}

	return paging.Page[T]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// clientOrg restricts client callers to their own customer organization (client:self-customer).
func clientOrg(c *ops.Call, column string, args *[]any) string {
	if c.Principal.Role != authz.RoleClient {
		return everyRow
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
	var customer Customer

	err := r.Scan(
		&customer.ID,
		&customer.TenantID,
		&customer.Version,
		&customer.CreatedAt,
		&customer.UpdatedAt,
		&customer.Name,
		&customer.Status,
		&customer.OrganizationID,
		&customer.ServiceProfile,
	)

	return customer, apperr.Wrap(err, "scan (scanCustomer)")
}

var profiles = map[string]bool{"rto": true, "general": true, "energy": true, "environment": true}

// customersList serves customers.list.
//
//	@Summary		customers.list (read)
//	@ID				customers.list
//	@Description	Authorization: admin:asset.read | admin:contract.read:scope-candidate-read-only | \
//	@Description	admin:offset.read:scope-candidate-read-only
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A02, DD-A07, DD-A15, DD-A05, DD-A18, DD-A06, DD-A01 · Query: filters \
//	@Description	kind,status,organizationId · sort id,createdAt,updatedAt (default id asc)
//	@Tags			customers
//	@Accept			json
//	@Produce		json
//	@Param			cursor			query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit			query		integer	false	"page size 1–100, default 25"
//	@Param			sort			query		string	false	"field:direction — fields id,createdAt,updatedAt; default id asc"
//	@Param			kind			query		string	false	"filter → serviceProfile"
//	@Param			status			query		string	false	"filter → status"
//	@Param			organizationId	query		string	false	"filter → organizationId"
//	@Success		200				{object}	ops.Envelope{data=CustomerPage}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/customers [get]
func (m *Module) customersList(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Customer], error) {
	var filters struct {
		Kind           *string    `json:"kind,omitempty"`
		Status         *string    `json:"status,omitempty"`
		OrganizationID *uuid.UUID `json:"organizationId,omitempty"`
	}

	err := decodeFilters(query.Filters, &filters)
	if err != nil {
		return paging.Page[Customer]{}, err
	}

	var args []any

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}
	conds := []string{everyRow}

	if filters.Kind != nil {
		if !profiles[*filters.Kind] {
			return paging.Page[Customer]{}, apperr.Fields(
				map[string]string{keyFiltersKind: apperr.KeyInvalid},
			)
		}

		conds = append(conds, "service_profile = "+add(*filters.Kind))
	}

	if filters.Status != nil {
		if *filters.Status != statusActive && *filters.Status != statusInactive {
			return paging.Page[Customer]{}, apperr.Fields(
				map[string]string{"filters.status": apperr.KeyInvalid},
			)
		}

		conds = append(conds, "status = "+add(*filters.Status))
	} else {
		conds = append(
			conds,
			"status = 'active'",
		) // inactive customers only with Status: All / inactive (IR40)
	}

	if filters.OrganizationID != nil {
		conds = append(conds, "organization_id = "+add(*filters.OrganizationID))
	}

	return list(
		ctx,
		call,
		query,
		filters,
		"assets.customers",
		customerCols,
		conds,
		args,
		scanCustomer,
	)
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
	fieldErrs := map[string]string{}

	in.Name = strings.TrimSpace(in.Name)
	if !strLen(in.Name, 1, 120) {
		fieldErrs[keyName] = apperr.KeyLength
	}

	if in.OrganizationID == uuid.Nil {
		fieldErrs["organizationId"] = apperr.KeyRequired
	}

	if !profiles[in.ServiceProfile] {
		fieldErrs["serviceProfile"] = apperr.KeyInvalid
	}

	if in.Status != statusActive && in.Status != statusInactive {
		fieldErrs["status"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// customersSave serves customers.save.
//
//	@Summary		customers.save (write)
//	@ID				customers.save
//	@Description	Authorization: admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A02
//	@Tags			customers
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		CustomerSave	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Customer}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/customers [post]
func (m *Module) customersSave(
	ctx context.Context,
	call *ops.Call,
	input *CustomerSave,
) (Customer, error) {
	if input.ID == nil {
		customerID := uuid.Must(uuid.NewV7())

		customer, err := scanCustomer(
			call.Tx.QueryRow(
				ctx,
				`INSERT INTO assets.customers (id, tenant_id, organization_id, name, status, service_profile)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5) RETURNING `+customerCols,
				customerID,
				input.OrganizationID,
				input.Name,
				input.Status,
				input.ServiceProfile,
			),
		)
		if err != nil {
			return Customer{}, err // 23505: organization already has a customer → CONFLICT
		}

		call.Audit(
			ops.AuditEntry{
				Action:          "customers.save",
				TargetKind:      kindCustomer,
				TargetID:        customerID.String(),
				NextVersion:     &customer.Version,
				PreviousVersion: nil,
				Reason:          "",
				Result:          "",
				Before:          nil,
				After:           nil,
			},
		)
		call.Emit(
			ops.Event{
				AggregateType: kindCustomer,
				AggregateID:   customerID,
				Type:          "CustomerChanged",
				Payload:       nil,
			},
		)

		return customer, nil
	}

	if input.Status == statusInactive { // D05: deactivation conflicts with active units, contracts or jobs
		var units bool

		err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.units WHERE customer_org_id = $1 AND NOT
			archived)`, input.OrganizationID).
			Scan(&units)
		if err != nil {
			return Customer{}, apperr.Wrap(err, "select assets.units")
		}

		if units {
			return Customer{}, conflict(apperr.KeyCustomerActive)
		}

		for _, a := range m.OrgActive {
			busy, err := a.OrgActive(ctx, call, input.OrganizationID)
			if err != nil {
				return Customer{}, apperr.Wrap(err, "assets.OrgActivity.OrgActive")
			}

			if busy {
				return Customer{}, conflict(apperr.KeyCustomerActive)
			}
		}
	}

	updated, err := scanCustomer(
		call.Tx.QueryRow(
			ctx,
			`UPDATE assets.customers SET name=$3, status=$4, service_profile=$5, version = version + 1, updated_at =
				platform.app_now()
		WHERE id = $1 AND version = $2 AND organization_id = $6 RETURNING `+customerCols,
			*input.ID,
			*call.ExpectedVersion,
			input.Name,
			input.Status,
			input.ServiceProfile,
			input.OrganizationID,
		),
	)
	if err != nil {
		return Customer{}, versionedUpdate(ctx, call, "assets.customers", *input.ID, err)
	}

	call.Audit(
		ops.AuditEntry{
			Action:          "customers.save",
			TargetKind:      kindCustomer,
			TargetID:        updated.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &updated.Version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)
	call.Emit(
		ops.Event{
			AggregateType: kindCustomer,
			AggregateID:   updated.ID,
			Type:          "CustomerChanged",
			Payload:       nil,
		},
	)

	return updated, nil
}

// ---- properties / spaces lists ----

// propertiesList serves properties.list.
//
//	@Summary		properties.list (read)
//	@ID				properties.list
//	@Description	Authorization: client:self-customer | admin:asset.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A02, DD-C02, DD-C07, DD-C04, DD-A06, DD-C09, DD-C01, DD-C08, DD-A01 · Query: filters \
//	@Description	customerId,kind · sort id,createdAt,updatedAt (default id asc)
//	@Tags			properties
//	@Accept			json
//	@Produce		json
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,createdAt,updatedAt; default id asc"
//	@Param			customerId	query		string	false	"filter → Customer.organizationId = Property.customerOrgId"
//	@Param			kind		query		string	false	"filter → kind"
//	@Success		200			{object}	ops.Envelope{data=PropertyPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/properties [get]
func (m *Module) propertiesList(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Property], error) {
	var filters struct {
		CustomerID *uuid.UUID `json:"customerId,omitempty"`
		Kind       *string    `json:"kind,omitempty"`
	}

	err := decodeFilters(query.Filters, &filters)
	if err != nil {
		return paging.Page[Property]{}, err
	}

	var args []any

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{"NOT archived", clientOrg(call, "customer_org_id", &args)}
	if filters.CustomerID != nil {
		conds = append(
			conds,
			"customer_org_id = (SELECT organization_id FROM assets.customers WHERE id = "+add(
				*filters.CustomerID,
			)+")",
		)
	}

	if filters.Kind != nil {
		if *filters.Kind != "home" && *filters.Kind != "office" {
			return paging.Page[Property]{}, apperr.Fields(
				map[string]string{keyFiltersKind: apperr.KeyInvalid},
			)
		}

		conds = append(conds, "kind = "+add(*filters.Kind))
	}

	return list(
		ctx,
		call,
		query,
		filters,
		"assets.properties",
		propertyCols,
		conds,
		args,
		scanProperty,
	)
}

// spacesList serves spaces.list.
//
//	@Summary		spaces.list (read)
//	@ID				spaces.list
//	@Description	Authorization: client:self-customer | admin:asset.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A02, DD-C02, DD-C07, DD-C04, DD-C09, DD-C01, DD-C08 · Query: filters propertyId,kind · \
//	@Description	sort id,createdAt,updatedAt (default id asc)
//	@Tags			spaces
//	@Accept			json
//	@Produce		json
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,createdAt,updatedAt; default id asc"
//	@Param			propertyId	query		string	false	"filter → propertyId"
//	@Param			kind		query		string	false	"filter → kind"
//	@Success		200			{object}	ops.Envelope{data=SpacePage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/spaces [get]
func (m *Module) spacesList(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Space], error) {
	var filters struct {
		PropertyID *uuid.UUID `json:"propertyId,omitempty"`
		Kind       *string    `json:"kind,omitempty"`
	}

	err := decodeFilters(query.Filters, &filters)
	if err != nil {
		return paging.Page[Space]{}, err
	}

	var args []any

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{"NOT archived"}
	if call.Principal.Role == authz.RoleClient {
		conds = append(
			conds,
			"property_id IN (SELECT id FROM assets.properties WHERE customer_org_id = "+add(
				call.Principal.OrgID,
			)+")",
		)
	}

	if filters.PropertyID != nil {
		conds = append(conds, "property_id = "+add(*filters.PropertyID))
	}

	if filters.Kind != nil {
		if !spaceKinds[*filters.Kind] {
			return paging.Page[Space]{}, apperr.Fields(
				map[string]string{keyFiltersKind: apperr.KeyInvalid},
			)
		}

		conds = append(conds, "kind = "+add(*filters.Kind))
	}

	return list(ctx, call, query, filters, "assets.spaces", spaceCols, conds, args, scanSpace)
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
	fieldErrs := map[string]string{}
	if in.Target.Kind != kindProperty && in.Target.Kind != kindSpace && in.Target.Kind != kindUnit {
		fieldErrs["target.kind"] = apperr.KeyInvalid
	}

	if in.Target.ID == uuid.Nil {
		fieldErrs["target.id"] = apperr.KeyRequired
	}

	if !strLen(strings.TrimSpace(in.Name), 1, 120) {
		fieldErrs[keyName] = apperr.KeyLength
	}

	return fieldErrs
}

// locationsRename serves locations.rename.
//
//	@Summary		locations.rename (write)
//	@ID				locations.rename
//	@Description	Authorization: client:self-customer | admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR109 name 1–120 characters, unique among siblings; structure fields are not accepted
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C02, DD-C03, DD-A02
//	@Tags			locations
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target property|space|unit, read properties.list;spaces.list;units.get)"
//	@Param			target.kind			path		string		true	"input field target.kind"
//	@Param			target.id			path		string		true	"input field target.id"
//	@Param			request				body		RenameInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/locations/{target.kind}/{target.id}/rename [post]
func (m *Module) locationsRename(
	ctx context.Context,
	call *ops.Call,
	input *RenameInput,
) (any, error) {
	name := strings.TrimSpace(input.Name)

	var table, col, org, siblings string

	switch input.Target.Kind {
	case kindProperty:
		table, col, org = "assets.properties", keyName, "customer_org_id"
		siblings = `SELECT 1 FROM assets.properties o, assets.properties t WHERE t.id = $1 AND o.customer_org_id =
			t.customer_org_id AND o.id <> t.id AND NOT o.archived AND lower(o.name) = lower($2)`
	case kindSpace:
		table, col, org = "assets.spaces", keyName, "(SELECT customer_org_id FROM assets.properties p WHERE p.id = "+
			"property_id)"
		siblings = `SELECT 1 FROM assets.spaces o, assets.spaces t WHERE t.id = $1 AND o.property_id = t.property_id AND
			o.parent_space_id IS NOT DISTINCT FROM t.parent_space_id AND o.id <> t.id AND NOT o.archived AND lower(o.name) =
			lower($2)`
	default:
		table, col, org = "assets.units", "display_name", "customer_org_id"
		siblings = `SELECT 1 FROM assets.units o, assets.units t WHERE t.id = $1 AND o.property_id = t.property_id AND
			o.space_id IS NOT DISTINCT FROM t.space_id AND o.id <> t.id AND NOT o.archived AND lower(o.display_name) = lower($2)`
	}

	visArgs, updArgs := []any{input.Target.ID}, []any{input.Target.ID, *call.ExpectedVersion, name}
	visScope, updScope := everyRow, everyRow

	if call.Principal.Role == authz.RoleClient { // client:self-customer
		visArgs, updArgs = append(
			visArgs,
			call.Principal.OrgID,
		), append(
			updArgs,
			call.Principal.OrgID,
		)
		visScope, updScope = org+" = $2", org+" = $4"
	}

	var visible bool

	scanErr := call.Tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM "+table+" WHERE id = $1 AND NOT archived AND "+visScope+
		")", visArgs...).
		Scan(&visible)
	if scanErr != nil {
		return nil, apperr.Wrap(scanErr, "select exists")
	}

	if !visible {
		return nil, notFound()
	}

	var dup bool

	scanErr = call.Tx.QueryRow(ctx, "SELECT EXISTS ("+siblings+")", input.Target.ID, name).
		Scan(&dup)
	if scanErr != nil {
		return nil, apperr.Wrap(scanErr, "select exists")
	}

	if dup {
		return nil, conflict(apperr.KeyDuplicateSiblingName)
	}

	var version int

	err := call.Tx.QueryRow(ctx, "UPDATE "+table+" SET "+col+" = $3, version = version + 1, updated_at = "+
		"platform.app_now() WHERE id = $1 AND version = $2 AND NOT archived AND "+updScope+" RETURNING version", updArgs...).
		Scan(&version)
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, conflict(apperr.KeyVersionConflict)
	}

	if err != nil {
		return nil, apperr.Wrap(err, "update (locationsRename)")
	}

	typ := eventLocationChanged
	if input.Target.Kind == kindUnit {
		typ = eventUnitChanged
	}

	call.Emit(
		ops.Event{
			AggregateType: input.Target.Kind,
			AggregateID:   input.Target.ID,
			Type:          typ,
			Payload:       nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "locations.rename",
			TargetKind:      input.Target.Kind,
			TargetID:        input.Target.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	switch input.Target.Kind {
	case kindProperty:
		return scanProperty(
			call.Tx.QueryRow(
				ctx,
				"SELECT "+propertyCols+" FROM assets.properties WHERE id = $1",
				input.Target.ID,
			),
		)
	case kindSpace:
		return scanSpace(
			call.Tx.QueryRow(
				ctx,
				"SELECT "+spaceCols+" FROM assets.spaces WHERE id = $1",
				input.Target.ID,
			),
		)
	}

	return m.load(ctx, call, input.Target.ID)
}
