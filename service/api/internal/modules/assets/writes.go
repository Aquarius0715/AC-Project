package assets

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
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/pradita/ac-project/service/api/internal/modules/devices"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/unitscope"
)

// Models is what Assets needs from the Devices module.
type Models interface {
	CurrentVersion(ctx context.Context, c *ops.Call, modelID uuid.UUID) (int, bool, error)
}

func strLen(s string, lo, hi int) bool {
	n := utf8.RuneCountInString(s)

	return n >= lo && n <= hi
}

func notFound() error           { return apperr.E(apperr.NotFound, apperr.KeyNotFound) }
func conflict(key string) error { return apperr.E(apperr.Conflict, key) }

// versionedUpdate turns "no row updated" into NOT_FOUND (target missing / out of scope) or CONFLICT (version moved).
func versionedUpdate(
	ctx context.Context,
	call *ops.Call,
	table string,
	rowID uuid.UUID,
	err error,
) error {
	if !errors.Is(err, pgx.ErrNoRows) {
		return dupSibling(err)
	}

	var exists bool

	scanErr := call.Tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM "+table+" WHERE id = $1)", rowID).
		Scan(&exists)
	if scanErr != nil {
		return apperr.Wrap(scanErr, "select exists")
	}

	if exists {
		return conflict(apperr.KeyVersionConflict)
	}

	return notFound()
}

// ---- properties ----

// Property is Property of service-contracts.ts.
type Property struct {
	ID                 uuid.UUID `json:"id"`
	TenantID           uuid.UUID `json:"tenantId"`
	Version            int       `json:"version"`
	CreatedAt          time.Time `json:"createdAt"`
	UpdatedAt          time.Time `json:"updatedAt"`
	CustomerOrgID      uuid.UUID `json:"customerOrgId"`
	Kind               string    `json:"kind"`
	Name               string    `json:"name"`
	Address            *string   `json:"address"`
	AccessInstructions *string   `json:"accessInstructions"`
	Archived           bool      `json:"archived"`
}

const propertyCols = `id, tenant_id, version, created_at, updated_at, customer_org_id, kind, name, address, ` +
	`access_instructions, archived`

func scanProperty(row pgx.Row) (Property, error) {
	var property Property

	err := row.Scan(
		&property.ID,
		&property.TenantID,
		&property.Version,
		&property.CreatedAt,
		&property.UpdatedAt,
		&property.CustomerOrgID,
		&property.Kind,
		&property.Name,
		&property.Address,
		&property.AccessInstructions,
		&property.Archived,
	)

	return property, apperr.Wrap(err, "scan (scanProperty)")
}

// PropertySave is properties.save input.
type PropertySave struct {
	ID                 *uuid.UUID `json:"id,omitempty"`
	CustomerOrgID      uuid.UUID  `json:"customerOrgId"`
	Kind               string     `json:"kind"`
	Name               string     `json:"name"`
	Address            *string    `json:"address"`
	AccessInstructions *string    `json:"accessInstructions"`
}

// Validate implements ops.Validator (DD-A02 field table).
func (in *PropertySave) Validate() map[string]string {
	fieldErrs := map[string]string{}

	in.Name = strings.TrimSpace(in.Name) // 1–120 characters after trim, as in locations.rename
	if in.CustomerOrgID == uuid.Nil {
		fieldErrs["customerOrgId"] = apperr.KeyRequired
	}

	if in.Kind != "home" && in.Kind != "office" {
		fieldErrs["kind"] = apperr.KeyInvalid
	}

	if !strLen(in.Name, 1, 120) {
		fieldErrs["name"] = apperr.KeyLength
	}

	if in.Address != nil && !strLen(*in.Address, 0, 500) {
		fieldErrs["address"] = apperr.KeyLength
	}

	if in.AccessInstructions != nil && !strLen(*in.AccessInstructions, 0, 1000) {
		fieldErrs["accessInstructions"] = apperr.KeyLength
	}

	return fieldErrs
}

// dupSibling maps the properties_name / spaces_name unique indexes (lower(name) per parent) to the sibling-name
// CONFLICT of siblingName (IR208).
func dupSibling(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" &&
		(pg.ConstraintName == "properties_name" || pg.ConstraintName == "spaces_name") {
		return conflict(apperr.KeyDuplicateSiblingName)
	}

	return err
}

// siblingName applies the locations.rename sibling rule to the HQ saves (DD-A02, IR208): another non-archived
// property of the customer, space under the same parent or unit in the same space with the same trimmed,
// case-insensitive name is CONFLICT. It runs after the write in the same transaction, so NOT_FOUND and version
// conflicts come first and a duplicate rolls the write back.
func siblingName(ctx context.Context, call *ops.Call, kind string, locationID uuid.UUID) error {
	stmt := `SELECT 1 FROM assets.properties o, assets.properties t WHERE t.id = $1 AND o.customer_org_id =
		t.customer_org_id
		AND o.id <> t.id AND NOT o.archived AND lower(btrim(o.name)) = lower(btrim(t.name))`

	switch kind {
	case "space":
		stmt = `SELECT 1 FROM assets.spaces o, assets.spaces t WHERE t.id = $1 AND o.property_id = t.property_id AND
			o.parent_space_id IS NOT DISTINCT FROM t.parent_space_id AND o.id <> t.id AND NOT o.archived AND lower(btrim(o.name))
			= lower(btrim(t.name))`
	case "unit":
		stmt = `SELECT 1 FROM assets.units o, assets.units t WHERE t.id = $1 AND o.property_id = t.property_id AND o.space_id
			IS
			NOT DISTINCT FROM t.space_id AND o.id <> t.id AND NOT o.archived AND lower(btrim(o.display_name)) =
			lower(btrim(t.display_name))`
	}

	var taken bool

	err := call.Tx.QueryRow(ctx, "SELECT EXISTS ("+stmt+")", locationID).Scan(&taken)
	if err != nil {
		return apperr.Wrap(err, "select exists")
	}

	if taken {
		return conflict(apperr.KeyDuplicateSiblingName)
	}

	return nil
}

// ---- spaces ----

// Space is Space of service-contracts.ts.
type Space struct {
	ID            uuid.UUID  `json:"id"`
	TenantID      uuid.UUID  `json:"tenantId"`
	Version       int        `json:"version"`
	CreatedAt     time.Time  `json:"createdAt"`
	UpdatedAt     time.Time  `json:"updatedAt"`
	PropertyID    uuid.UUID  `json:"propertyId"`
	ParentSpaceID *uuid.UUID `json:"parentSpaceId"`
	Kind          string     `json:"kind"`
	Name          string     `json:"name"`
	Archived      bool       `json:"archived"`
}

const spaceCols = `id, tenant_id, version, created_at, updated_at, property_id, parent_space_id, kind, name, archived`

func scanSpace(row pgx.Row) (Space, error) {
	var space Space

	err := row.Scan(
		&space.ID,
		&space.TenantID,
		&space.Version,
		&space.CreatedAt,
		&space.UpdatedAt,
		&space.PropertyID,
		&space.ParentSpaceID,
		&space.Kind,
		&space.Name,
		&space.Archived,
	)

	return space, apperr.Wrap(err, "scan (scanSpace)")
}

// SpaceSave is spaces.save input.
type SpaceSave struct {
	ID            *uuid.UUID `json:"id,omitempty"`
	PropertyID    uuid.UUID  `json:"propertyId"`
	ParentSpaceID *uuid.UUID `json:"parentSpaceId"`
	Kind          string     `json:"kind"`
	Name          string     `json:"name"`
}

var spaceKinds = map[string]bool{"area": true, "floor": true, "room": true, "space": true}

// Validate implements ops.Validator.
func (in *SpaceSave) Validate() map[string]string {
	fieldErrs := map[string]string{}

	in.Name = strings.TrimSpace(in.Name)
	if in.PropertyID == uuid.Nil {
		fieldErrs["propertyId"] = apperr.KeyRequired
	}

	if !spaceKinds[in.Kind] {
		fieldErrs["kind"] = apperr.KeyInvalid
	}

	if !strLen(in.Name, 1, 120) {
		fieldErrs["name"] = apperr.KeyLength
	}

	if in.ID != nil && in.ParentSpaceID != nil && *in.ID == *in.ParentSpaceID {
		fieldErrs["parentSpaceId"] = "error.cycle"
	}

	return fieldErrs
}

// ---- units ----

// UnitSave is units.save input.
type UnitSave struct {
	ID            *uuid.UUID `json:"id,omitempty"`
	CustomerOrgID uuid.UUID  `json:"customerOrgId"`
	PropertyID    uuid.UUID  `json:"propertyId"`
	SpaceID       *uuid.UUID `json:"spaceId"`
	DisplayName   string     `json:"displayName"`
	ModelID       uuid.UUID  `json:"modelId"`
	Type          string     `json:"type"`
	InstalledAt   *time.Time `json:"installedAt"`
	ServiceScope  []string   `json:"serviceScope"`
	ChangeReason  *string    `json:"changeReason,omitempty"`
	// WarrantyEndsAt is optional: omitted keeps the stored end (none for a new unit), null clears it (IR209).
	WarrantyEndsAt json.RawMessage `json:"warrantyEndsAt,omitempty" swaggertype:"object"`
	warranty       *time.Time
	warrantySet    bool
}

var scopes = map[string]bool{"indoor": true, "outdoor": true, "electrical": true}

// Validate implements ops.Validator.
func (in *UnitSave) Validate() map[string]string {
	fieldErrs := map[string]string{}

	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.CustomerOrgID == uuid.Nil {
		fieldErrs["customerOrgId"] = apperr.KeyRequired
	}

	if in.PropertyID == uuid.Nil {
		fieldErrs["propertyId"] = apperr.KeyRequired
	}

	if in.ModelID == uuid.Nil {
		fieldErrs["modelId"] = apperr.KeyRequired
	}

	if in.Type != "split" {
		fieldErrs["type"] = apperr.KeyInvalid
	}

	if !strLen(in.DisplayName, 1, 120) {
		fieldErrs["displayName"] = apperr.KeyLength
	}

	if len(in.WarrantyEndsAt) > 0 {
		in.warrantySet = true
		if string(in.WarrantyEndsAt) != "null" {
			var endsAt time.Time

			err := json.Unmarshal(in.WarrantyEndsAt, &endsAt)
			if err != nil {
				fieldErrs["warrantyEndsAt"] = apperr.KeyInvalid
			} else if in.InstalledAt != nil && endsAt.Before(*in.InstalledAt) {
				fieldErrs["warrantyEndsAt"] = apperr.KeyRange // a warranty cannot end before the installation
			} else {
				in.warranty = &endsAt
			}
		}
	}

	seen := map[string]bool{}
	for _, s := range in.ServiceScope {
		if !scopes[s] || seen[s] {
			fieldErrs["serviceScope"] = apperr.KeyInvalid
		}

		seen[s] = true
	}

	if len(in.ServiceScope) == 0 {
		fieldErrs["serviceScope"] = apperr.KeyRequired
	}

	if in.ChangeReason != nil && !strLen(*in.ChangeReason, 1, 1000) {
		fieldErrs["changeReason"] = apperr.KeyLength
	}

	return fieldErrs
}

// ---- archive ----

// ArchiveInput is the input of properties/spaces/units.archive.
type ArchiveInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *ArchiveInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ID == uuid.Nil {
		fieldErrs["id"] = apperr.KeyRequired
	}

	if !strLen(in.Reason, 1, 1000) {
		fieldErrs["reason"] = apperr.KeyLength
	}

	return fieldErrs
}

// ArchivedResource is ArchivedResource of service-contracts.ts.
type ArchivedResource struct {
	ID       uuid.UUID `json:"id"`
	Version  int       `json:"version"`
	Archived bool      `json:"archived"`
}

// MoveToCapabilityVersion points every unit of the model at its new capability version (called by Devices on
// capabilities.save, DD-A04 step 3); returns the number of units changed.
func (m *Module) MoveToCapabilityVersion(
	ctx context.Context,
	call *ops.Call,
	modelID uuid.UUID,
	version int,
) (int, error) {
	rows, err := call.Tx.Query(
		ctx,
		`UPDATE assets.units SET capability_version = $2, version = version + 1, updated_at = platform.app_now()
		WHERE model_id = $1 AND capability_version <> $2 RETURNING id`,
		modelID,
		version,
	)
	if err != nil {
		return 0, apperr.Wrap(err, "update assets.units")
	}
	defer rows.Close()

	moved := 0

	for rows.Next() {
		var unitID uuid.UUID

		err := rows.Scan(&unitID)
		if err != nil {
			return moved, apperr.Wrap(err, "update assets.units")
		}

		call.Emit(
			ops.Event{
				AggregateType: "unit",
				AggregateID:   unitID,
				Type:          "UnitChanged",
				Payload:       nil,
			},
		)

		moved++
	}

	return moved, apperr.Wrap(rows.Err(), "update assets.units")
}

// UnitInfo answers unit lookups for Devices (scope is applied by the calling operation's own rules).
func (m *Module) UnitInfo(
	ctx context.Context,
	c *ops.Call,
	unit uuid.UUID,
) (devices.UnitInfo, bool, error) {
	var unitInfo devices.UnitInfo

	err := c.Tx.QueryRow(ctx, `SELECT customer_org_id, property_id, model_id, capability_version FROM assets.units WHERE id
		= $1 AND NOT archived`, unit).
		Scan(&unitInfo.OrgID, &unitInfo.PropertyID, &unitInfo.ModelID, &unitInfo.CapabilityVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return unitInfo, false, nil
	}

	return unitInfo, err == nil, apperr.Wrap(err, "select assets.units")
}

// UnitsOfProperty lists the units of a property (Monitoring propertyId filters).
func (m *Module) UnitsOfProperty(
	ctx context.Context,
	c *ops.Call,
	property uuid.UUID,
) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, `SELECT id FROM assets.units WHERE property_id = $1`, property)
	if err != nil {
		return nil, apperr.Wrap(err, "select assets.units")
	}
	defer rows.Close()

	out := []uuid.UUID{}

	for rows.Next() {
		var unitID uuid.UUID

		err := rows.Scan(&unitID)
		if err != nil {
			return nil, apperr.Wrap(err, "select assets.units")
		}

		out = append(out, unitID)
	}

	return out, apperr.Wrap(rows.Err(), "select assets.units")
}

// OrgOfCustomer maps a customer ID to its organization.
func (m *Module) OrgOfCustomer(
	ctx context.Context,
	c *ops.Call,
	customer uuid.UUID,
) (uuid.UUID, bool, error) {
	var org uuid.UUID

	err := c.Tx.QueryRow(ctx, `SELECT organization_id FROM assets.customers WHERE id = $1`, customer).
		Scan(&org)
	if errors.Is(err, pgx.ErrNoRows) {
		return org, false, nil
	}

	return org, err == nil, apperr.Wrap(err, "select assets.customers")
}

// AttachedUnits returns the units carrying each alert policy (IR108: units are the attachment source).
func (m *Module) AttachedUnits(
	ctx context.Context,
	c *ops.Call,
	policies []uuid.UUID,
) (map[uuid.UUID][]uuid.UUID, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT policy_id, unit_id FROM assets.unit_alert_policies WHERE policy_id = ANY($1) ORDER BY policy_id, unit_id`,
		policies,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select assets.unit_alert_policies")
	}
	defer rows.Close()

	out := map[uuid.UUID][]uuid.UUID{}

	for rows.Next() {
		var policyID, unitID uuid.UUID

		err := rows.Scan(&policyID, &unitID)
		if err != nil {
			return nil, apperr.Wrap(err, "select assets.unit_alert_policies")
		}

		out[policyID] = append(out[policyID], unitID)
	}

	return out, apperr.Wrap(rows.Err(), "select assets.unit_alert_policies")
}

// PoliciesOfUnits returns the alert policies attached to any of the units.
func (m *Module) PoliciesOfUnits(
	ctx context.Context,
	c *ops.Call,
	units []uuid.UUID,
) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT DISTINCT policy_id FROM assets.unit_alert_policies WHERE unit_id = ANY($1)`,
		units,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select assets.unit_alert_policies")
	}
	defer rows.Close()

	out := []uuid.UUID{}

	for rows.Next() {
		var policyID uuid.UUID

		err := rows.Scan(&policyID)
		if err != nil {
			return nil, apperr.Wrap(err, "select assets.unit_alert_policies")
		}

		out = append(out, policyID)
	}

	return out, apperr.Wrap(rows.Err(), "select assets.unit_alert_policies")
}

// DetachPolicy removes an alert policy from every unit and bumps the version of each changed unit (policies.delete,
// IR108).
func (m *Module) DetachPolicy(ctx context.Context, call *ops.Call, policy uuid.UUID) error {
	rows, err := call.Tx.Query(
		ctx,
		`DELETE FROM assets.unit_alert_policies WHERE policy_id = $1 RETURNING unit_id`,
		policy,
	)
	if err != nil {
		return apperr.Wrap(err, "delete assets.unit_alert_policies")
	}

	var units []uuid.UUID

	for rows.Next() {
		var unitID uuid.UUID

		err := rows.Scan(&unitID)
		if err != nil {
			rows.Close()

			return apperr.Wrap(err, "delete assets.unit_alert_policies")
		}

		units = append(units, unitID)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return apperr.Wrap(rowsErr, "delete assets.unit_alert_policies")
	}

	if len(units) == 0 {
		return nil
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE assets.units SET version = version + 1, updated_at = platform.app_now() WHERE id = ANY($1)`,
		units,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update assets.units")
	}

	for _, u := range units {
		call.Emit(
			ops.Event{AggregateType: "unit", AggregateID: u, Type: "UnitChanged", Payload: nil},
		)
	}

	return nil
}

// OrgsOfUnits maps units to their customer organization.
func (m *Module) OrgsOfUnits(
	ctx context.Context,
	c *ops.Call,
	units []uuid.UUID,
) (map[uuid.UUID]uuid.UUID, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT id, customer_org_id FROM assets.units WHERE id = ANY($1)`,
		units,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select assets.units")
	}
	defer rows.Close()

	out := map[uuid.UUID]uuid.UUID{}

	for rows.Next() {
		var unitID, orgID uuid.UUID

		err := rows.Scan(&unitID, &orgID)
		if err != nil {
			return nil, apperr.Wrap(err, "select assets.units")
		}

		out[unitID] = orgID
	}

	return out, apperr.Wrap(rows.Err(), "select assets.units")
}

// CustomerOfOrg maps an organization to its customer.
func (m *Module) CustomerOfOrg(
	ctx context.Context,
	c *ops.Call,
	org uuid.UUID,
) (uuid.UUID, bool, error) {
	var customerID uuid.UUID

	err := c.Tx.QueryRow(ctx, `SELECT id FROM assets.customers WHERE organization_id = $1`, org).
		Scan(&customerID)
	if errors.Is(err, pgx.ErrNoRows) {
		return customerID, false, nil
	}

	return customerID, err == nil, apperr.Wrap(err, "select assets.customers")
}

// CustomerState returns the organization of a customer and whether it is active.
func (m *Module) CustomerState(
	ctx context.Context,
	call *ops.Call,
	customer uuid.UUID,
) (uuid.UUID, bool, bool, error) {
	var (
		org    uuid.UUID
		status string
	)

	err := call.Tx.QueryRow(ctx, `SELECT organization_id, status FROM assets.customers WHERE id = $1`, customer).
		Scan(&org, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return org, false, false, nil
	}

	return org, status == "active", err == nil, apperr.Wrap(err, "select assets.customers")
}

// VisibleUnits returns the given units that the caller may read (D01 scope; archived units included for history).
func (m *Module) VisibleUnits(
	ctx context.Context,
	c *ops.Call,
	units []uuid.UUID,
) ([]uuid.UUID, error) {
	args := []any{units}

	return m.unitIDs(ctx, c, "u.id = ANY($1) AND "+scopeSQL(c, &args), args)
}

// UnitsInSpace returns the caller-readable units placed directly in a space.
func (m *Module) UnitsInSpace(
	ctx context.Context,
	c *ops.Call,
	space uuid.UUID,
) ([]uuid.UUID, error) {
	args := []any{space}

	return m.unitIDs(ctx, c, "u.space_id = $1 AND "+scopeSQL(c, &args), args)
}

// SpaceInfo returns the customer organization of a space and the units placed directly in it.
func (m *Module) SpaceInfo(
	ctx context.Context,
	call *ops.Call,
	space uuid.UUID,
) (uuid.UUID, []uuid.UUID, bool, error) {
	var org uuid.UUID

	err := call.Tx.QueryRow(ctx, `SELECT p.customer_org_id FROM assets.spaces s JOIN assets.properties p ON p.id =
		s.property_id WHERE s.id = $1`, space).
		Scan(&org)
	if errors.Is(err, pgx.ErrNoRows) {
		return org, nil, false, nil
	}

	if err != nil {
		return org, nil, false, apperr.Wrap(err, "select assets.spaces")
	}

	units, err := m.unitIDs(ctx, call, "u.space_id = $1", []any{space})

	return org, units, true, err
}

// SpacesOfUnits returns the spaces of the given units (units without a space are skipped).
func (m *Module) SpacesOfUnits(
	ctx context.Context,
	c *ops.Call,
	units []uuid.UUID,
) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT DISTINCT space_id FROM assets.units WHERE id = ANY($1) AND space_id IS NOT NULL`,
		units,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select assets.units")
	}
	defer rows.Close()

	out := []uuid.UUID{}

	for rows.Next() {
		var spaceID uuid.UUID

		err := rows.Scan(&spaceID)
		if err != nil {
			return nil, apperr.Wrap(err, "select assets.units")
		}

		out = append(out, spaceID)
	}

	return out, apperr.Wrap(rows.Err(), "select assets.units")
}

// ScopedUnits returns every unit whose equipment data the caller may read (Equipment mode: external technicians
// only inside their work window, IR49(b)); used for telemetry list scopes.
func (m *Module) ScopedUnits(ctx context.Context, c *ops.Call) ([]uuid.UUID, error) {
	var args []any

	return m.unitIDs(ctx, c, unitscope.SQL(c, &args, "u.id", unitscope.Equipment), args)
}

// SpacesOfOrg returns every space in the properties of a customer organization.
func (m *Module) SpacesOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT s.id FROM assets.spaces s JOIN assets.properties p ON p.id = s.property_id WHERE p.customer_org_id = $1`,
		org,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select assets.spaces")
	}
	defer rows.Close()

	out := []uuid.UUID{}

	for rows.Next() {
		var spaceID uuid.UUID

		err := rows.Scan(&spaceID)
		if err != nil {
			return nil, apperr.Wrap(err, "select assets.spaces")
		}

		out = append(out, spaceID)
	}

	return out, apperr.Wrap(rows.Err(), "select assets.spaces")
}

// UnitState returns the customer organization of a unit and whether it is archived (Maintenance job intake).
func (m *Module) UnitState(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
) (uuid.UUID, bool, bool, error) {
	var (
		org      uuid.UUID
		archived bool
	)

	err := call.Tx.QueryRow(ctx, `SELECT customer_org_id, archived FROM assets.units WHERE id = $1`, unit).
		Scan(&org, &archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return org, false, false, nil
	}

	return org, archived, err == nil, apperr.Wrap(err, "select assets.units")
}

// UnitService returns the organization, property and service scope of a unit (Maintenance qualification checks).
func (m *Module) UnitService(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
) (uuid.UUID, uuid.UUID, []string, error) {
	var (
		org, prop uuid.UUID
		scope     []string
	)

	err := call.Tx.QueryRow(ctx, `SELECT customer_org_id, property_id, service_scope FROM assets.units WHERE id =
		$1`, unit).
		Scan(&org, &prop, &scope)

	return org, prop, scope, apperr.Wrap(err, "select assets.units")
}

// SiteAddress returns the IR25 installation address of a unit (null for blank) and the unit's service scope.
func (m *Module) SiteAddress(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
) (*string, []string, error) {
	var (
		addr  *string
		scope []string
	)

	err := call.Tx.QueryRow(ctx, `SELECT NULLIF(btrim(p.address), ''), u.service_scope FROM assets.units u JOIN
		assets.properties p ON p.id = u.property_id WHERE u.id = $1`, unit).
		Scan(&addr, &scope)

	return addr, scope, apperr.Wrap(err, "select assets.units")
}

// WarrantyEnd returns a unit's warrantyEndsAt (Maintenance warranty claims).
func (m *Module) WarrantyEnd(ctx context.Context, c *ops.Call, unit uuid.UUID) (*time.Time, error) {
	var t *time.Time

	err := c.Tx.QueryRow(ctx, `SELECT warranty_ends_at FROM assets.units WHERE id = $1`, unit).
		Scan(&t)

	return t, apperr.Wrap(err, "select assets.units")
}

// UnitsOfOrg returns the units of a customer organization.
func (m *Module) UnitsOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) ([]uuid.UUID, error) {
	return m.unitIDs(ctx, c, "u.customer_org_id = $1", []any{org})
}

// CustomerProfiles maps customer organizations to {customerId, serviceProfile} (Maintenance SLA, IR131).
func (m *Module) CustomerProfiles(
	ctx context.Context,
	c *ops.Call,
	orgs []uuid.UUID,
) (map[uuid.UUID][2]string, error) {
	rows, err := c.Tx.Query(
		ctx,
		`SELECT organization_id, id::text, service_profile FROM assets.customers WHERE organization_id = ANY($1)`,
		orgs,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select assets.customers")
	}
	defer rows.Close()

	out := map[uuid.UUID][2]string{}

	for rows.Next() {
		var (
			org              uuid.UUID
			customerID, prof string
		)

		err := rows.Scan(&org, &customerID, &prof)
		if err != nil {
			return nil, apperr.Wrap(err, "select assets.customers")
		}

		out[org] = [2]string{customerID, prof}
	}

	return out, apperr.Wrap(rows.Err(), "select assets.customers")
}

// BriefUnits lists the caller-readable, not archived units matching the optional customer / property / space / unit
// filters.
func (m *Module) BriefUnits(
	ctx context.Context,
	call *ops.Call,
	customer, property, space, unit *uuid.UUID,
) ([]ops.UnitBrief, error) {
	args := []any{}
	where := "NOT u.archived AND " + scopeSQL(call, &args)

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}
	if customer != nil {
		where += " AND u.customer_org_id = (SELECT organization_id FROM assets.customers WHERE id = " + add(
			*customer,
		) + ")"
	}

	if property != nil {
		where += " AND u.property_id = " + add(*property)
	}

	if space != nil {
		where += " AND u.space_id = " + add(*space)
	}

	if unit != nil {
		where += " AND u.id = " + add(*unit)
	}

	rows, err := call.Tx.Query(
		ctx,
		"SELECT u.id, u.customer_org_id, u.connection FROM assets.units u WHERE "+where+" ORDER BY u.id",
		args...)
	if err != nil {
		return nil, apperr.Wrap(err, "select assets.units")
	}
	defer rows.Close()

	var out []ops.UnitBrief

	for rows.Next() {
		var unitBrief ops.UnitBrief

		err := rows.Scan(&unitBrief.ID, &unitBrief.OrgID, &unitBrief.Connection)
		if err != nil {
			return nil, apperr.Wrap(err, "select assets.units")
		}

		out = append(out, unitBrief)
	}

	return out, apperr.Wrap(rows.Err(), "select assets.units")
}

// UnitVersion returns a unit's current version (Control expectedUnitVersion).
func (m *Module) UnitVersion(ctx context.Context, c *ops.Call, unit uuid.UUID) (int, error) {
	var v int

	err := c.Tx.QueryRow(ctx, `SELECT version FROM assets.units WHERE id = $1`, unit).Scan(&v)

	return v, apperr.Wrap(err, "select assets.units")
}

func (m *Module) customerActive(ctx context.Context, c *ops.Call, org uuid.UUID) error {
	var status string

	err := c.Tx.QueryRow(ctx, `SELECT status FROM assets.customers WHERE organization_id = $1`, org).
		Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound()
	}

	if err != nil {
		return apperr.Wrap(err, "select assets.customers")
	}

	if status != "active" {
		return conflict(apperr.KeyCustomerInactive)
	}

	return nil
}

// propertiesSave serves properties.save.
//
//	@Summary		properties.save (write)
//	@ID				properties.save
//	@Description	Authorization: admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A02
//	@Tags			properties
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		PropertySave	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Property}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/properties [post]
func (m *Module) propertiesSave(
	ctx context.Context,
	call *ops.Call,
	input *PropertySave,
) (Property, error) {
	if input.ID == nil {
		err := m.customerActive(ctx, call, input.CustomerOrgID)
		if err != nil {
			return Property{}, err
		}

		propertyID := uuid.Must(uuid.NewV7())

		property, err := scanProperty(
			call.Tx.QueryRow(
				ctx,
				`INSERT INTO assets.properties (id, tenant_id, customer_org_id, kind, name, address, access_instructions)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6) RETURNING `+propertyCols,
				propertyID,
				input.CustomerOrgID,
				input.Kind,
				input.Name,
				input.Address,
				input.AccessInstructions,
			),
		)
		if err != nil {
			return Property{}, dupSibling(err)
		}

		siblingNameErr := siblingName(ctx, call, "property", propertyID)
		if siblingNameErr != nil {
			return Property{}, siblingNameErr
		}

		call.Emit(
			ops.Event{
				AggregateType: "property",
				AggregateID:   propertyID,
				Type:          "LocationChanged",
				Payload:       nil,
			},
		)
		call.Audit(
			ops.AuditEntry{
				Action:          "properties.save",
				TargetKind:      "property",
				TargetID:        propertyID.String(),
				NextVersion:     &property.Version,
				PreviousVersion: nil,
				Reason:          "",
				Result:          "",
				Before:          nil,
				After:           nil,
			},
		)

		return property, nil
	}

	updated, err := scanProperty(
		call.Tx.QueryRow(
			ctx,
			`UPDATE assets.properties SET kind=$3, name=$4, address=$5, access_instructions=$6,
		version = version + 1, updated_at = platform.app_now()
		WHERE id = $1 AND version = $2 AND customer_org_id = $7 AND NOT archived RETURNING `+propertyCols,
			*input.ID,
			*call.ExpectedVersion,
			input.Kind,
			input.Name,
			input.Address,
			input.AccessInstructions,
			input.CustomerOrgID,
		),
	)
	if err != nil {
		return Property{}, versionedUpdate(ctx, call, "assets.properties", *input.ID, err)
	}

	siblingNameErr := siblingName(ctx, call, "property", updated.ID)
	if siblingNameErr != nil {
		return Property{}, siblingNameErr
	}

	call.Emit(
		ops.Event{
			AggregateType: "property",
			AggregateID:   updated.ID,
			Type:          "LocationChanged",
			Payload:       nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "properties.save",
			TargetKind:      "property",
			TargetID:        updated.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &updated.Version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return updated, nil
}

// activeProperty requires an active property, read with a share lock: an archive of it waits, or is waited for and seen
// (IR478).
func (m *Module) activeProperty(ctx context.Context, c *ops.Call, id uuid.UUID) error {
	err := c.Tx.QueryRow(ctx, `SELECT 1 FROM assets.properties WHERE id = $1 AND NOT archived FOR SHARE`, id).
		Scan(new(int))
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound()
	}

	return apperr.Wrap(err, "select assets.properties")
}

// checkParent requires the parent to be an active space of the same property and, for an existing space, not one
// of its own descendants (no hierarchy cycles).
func (m *Module) checkParent(
	ctx context.Context,
	call *ops.Call,
	self *uuid.UUID,
	property uuid.UUID,
	parent *uuid.UUID,
) error {
	if parent == nil {
		return nil
	}

	err := call.Tx.QueryRow(ctx, `SELECT 1 FROM assets.spaces WHERE id = $1 AND property_id = $2 AND NOT archived FOR
		SHARE`, *parent, property).
		Scan(new(int))

		// IR478
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.Fields(map[string]string{"parentSpaceId": "error.otherProperty"})
	}

	if err != nil {
		return apperr.Wrap(err, "select assets.spaces")
	}

	if self == nil {
		return nil
	}

	var cycle bool

	scanErr := call.Tx.QueryRow(ctx, `WITH RECURSIVE d AS (SELECT id FROM assets.spaces WHERE parent_space_id = $1
		UNION ALL SELECT s.id FROM assets.spaces s JOIN d ON s.parent_space_id = d.id) SELECT EXISTS (SELECT 1 FROM d WHERE id
		= $2)`, *self, *parent).
		Scan(&cycle)
	if scanErr != nil {
		return apperr.Wrap(scanErr, "select d")
	}

	if cycle {
		return apperr.Fields(map[string]string{"parentSpaceId": "error.cycle"})
	}

	return nil
}

// spacesSave serves spaces.save.
//
//	@Summary		spaces.save (write)
//	@ID				spaces.save
//	@Description	Authorization: admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A02
//	@Tags			spaces
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		SpaceSave	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Space}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/spaces [post]
func (m *Module) spacesSave(ctx context.Context, c *ops.Call, input *SpaceSave) (Space, error) {
	err := m.activeProperty(ctx, c, input.PropertyID)
	if err != nil {
		return Space{}, err
	}

	err = m.checkParent(ctx, c, input.ID, input.PropertyID, input.ParentSpaceID)
	if err != nil {
		return Space{}, err
	}

	if input.ID == nil {
		spaceID := uuid.Must(uuid.NewV7())

		space, err := scanSpace(
			c.Tx.QueryRow(
				ctx,
				`INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5) RETURNING `+spaceCols,
				spaceID,
				input.PropertyID,
				input.ParentSpaceID,
				input.Kind,
				input.Name,
			),
		)
		if err != nil {
			return Space{}, dupSibling(err)
		}

		siblingNameErr := siblingName(ctx, c, "space", spaceID)
		if siblingNameErr != nil {
			return Space{}, siblingNameErr
		}

		c.Emit(
			ops.Event{
				AggregateType: "space",
				AggregateID:   spaceID,
				Type:          "LocationChanged",
				Payload:       nil,
			},
		)
		c.Audit(
			ops.AuditEntry{
				Action:          "spaces.save",
				TargetKind:      "space",
				TargetID:        spaceID.String(),
				NextVersion:     &space.Version,
				PreviousVersion: nil,
				Reason:          "",
				Result:          "",
				Before:          nil,
				After:           nil,
			},
		)

		return space, nil
	}

	space, err := scanSpace(
		c.Tx.QueryRow(
			ctx,
			`UPDATE assets.spaces SET parent_space_id=$3, kind=$4, name=$5, version = version + 1, updated_at =
				platform.app_now()
		WHERE id = $1 AND version = $2 AND property_id = $6 AND NOT archived RETURNING `+spaceCols,
			*input.ID,
			*c.ExpectedVersion,
			input.ParentSpaceID,
			input.Kind,
			input.Name,
			input.PropertyID,
		),
	)
	if err != nil {
		return Space{}, versionedUpdate(ctx, c, "assets.spaces", *input.ID, err)
	}

	siblingNameErr := siblingName(ctx, c, "space", space.ID)
	if siblingNameErr != nil {
		return Space{}, siblingNameErr
	}

	c.Emit(
		ops.Event{
			AggregateType: "space",
			AggregateID:   space.ID,
			Type:          "LocationChanged",
			Payload:       nil,
		},
	)
	c.Audit(
		ops.AuditEntry{
			Action:          "spaces.save",
			TargetKind:      "space",
			TargetID:        space.ID.String(),
			PreviousVersion: c.ExpectedVersion,
			NextVersion:     &space.Version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return space, nil
}

// unitsSave serves units.save.
//
//	@Summary		units.save (write)
//	@ID				units.save
//	@Description	Authorization: admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR62 spaceId=null is property root; IR74 no tenantId input
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A02
//	@Tags			units
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		UnitSave	true	"input"
//	@Success		200				{object}	ops.Envelope{data=ACUnit}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/units [post]
func (m *Module) unitsSave(ctx context.Context, c *ops.Call, in *UnitSave) (ACUnit, error) {
	u, err := m.unitsSaveUnit(ctx, c, in)

	return u.ACUnit, err
}

// unitsSaveUnit is unitsSave answering the unit with its summary fields.
func (m *Module) unitsSaveUnit(ctx context.Context, c *ops.Call, input *UnitSave) (Unit, error) {
	if input.InstalledAt != nil && input.InstalledAt.After(c.Now) {
		return Unit{}, apperr.Fields(map[string]string{"installedAt": apperr.KeyFuture})
	}

	var propOrg uuid.UUID
	// the property and the space are read with a share lock: their archive waits, or is waited for and seen (IR478)
	err := c.Tx.QueryRow(ctx, `SELECT customer_org_id FROM assets.properties WHERE id = $1 AND NOT archived FOR
		SHARE`, input.PropertyID).
		Scan(&propOrg)
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, notFound()
	}

	if err != nil {
		return Unit{}, apperr.Wrap(err, "select assets.properties")
	}

	if propOrg != input.CustomerOrgID {
		return Unit{}, apperr.Fields(map[string]string{"propertyId": "error.otherCustomer"})
	}

	if input.SpaceID != nil {
		err := c.Tx.QueryRow(ctx, `SELECT 1 FROM assets.spaces WHERE id = $1 AND property_id = $2 AND NOT archived FOR
			SHARE`, *input.SpaceID, input.PropertyID).
			Scan(new(int))
		if errors.Is(err, pgx.ErrNoRows) {
			return Unit{}, apperr.Fields(map[string]string{"spaceId": "error.otherProperty"})
		}

		if err != nil {
			return Unit{}, apperr.Wrap(err, "select assets.spaces")
		}
	}

	capVer, ok, err := m.Models.CurrentVersion(ctx, c, input.ModelID)
	if err != nil {
		return Unit{}, apperr.Wrap(err, "assets.Models.CurrentVersion")
	}

	if !ok {
		return Unit{}, apperr.Fields(map[string]string{"modelId": "error.unknownModel"})
	}

	if input.ID == nil {
		err := m.customerActive(ctx, c, input.CustomerOrgID)
		if err != nil {
			return Unit{}, err
		}

		unitID := uuid.Must(uuid.NewV7())

		_, err = c.Tx.Exec(
			ctx,
			`INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id, type,
				installed_at, service_scope, capability_version, warranty_ends_at)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			unitID,
			input.CustomerOrgID,
			input.PropertyID,
			input.SpaceID,
			input.DisplayName,
			input.ModelID,
			input.Type,
			input.InstalledAt,
			input.ServiceScope,
			capVer,
			input.warranty,
		)
		if err != nil {
			return Unit{}, apperr.Wrap(err, "insert assets.units")
		}

		err = siblingName(ctx, c, "unit", unitID)
		if err != nil {
			return Unit{}, err
		}

		c.Emit(
			ops.Event{
				AggregateType: "unit",
				AggregateID:   unitID,
				Type:          "UnitChanged",
				Payload:       nil,
			},
		)

		one := 1
		c.Audit(
			ops.AuditEntry{
				Action:          "units.save",
				TargetKind:      "unit",
				TargetID:        unitID.String(),
				NextVersion:     &one,
				PreviousVersion: nil,
				Reason:          "",
				Result:          "",
				Before:          nil,
				After:           nil,
			},
		)

		return m.load(ctx, c, unitID)
	}
	// relocation (property or space change) requires a reason (DD-A02 changeReason)
	var (
		curProp      uuid.UUID
		curSpace     *uuid.UUID
		curName      string
		curModel     uuid.UUID
		curInstalled *time.Time
		curScope     []string
		curWarranty  *time.Time
	)

	err = c.Tx.QueryRow(ctx, `SELECT property_id, space_id, display_name, model_id, installed_at, service_scope,
		warranty_ends_at FROM assets.units WHERE id = $1 AND customer_org_id = $2 AND NOT archived`,
		*input.ID, input.CustomerOrgID).
		Scan(&curProp, &curSpace, &curName, &curModel, &curInstalled, &curScope, &curWarranty)
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, notFound()
	}

	if err != nil {
		return Unit{}, apperr.Wrap(err, "select assets.units")
	}

	moved := curProp != input.PropertyID || (curSpace == nil) != (input.SpaceID == nil) ||
		(curSpace != nil && input.SpaceID != nil && *curSpace != *input.SpaceID)
	reason := ""

	if moved {
		if input.ChangeReason == nil {
			return Unit{}, apperr.Fields(map[string]string{"changeReason": apperr.KeyRequired})
		}

		reason = *input.ChangeReason
	}

	warranty := curWarranty
	if input.warrantySet {
		warranty = input.warranty
	}

	if warranty != nil && input.InstalledAt != nil && warranty.Before(*input.InstalledAt) {
		return Unit{}, apperr.Fields(map[string]string{"warrantyEndsAt": apperr.KeyRange})
	}

	var version int

	err = c.Tx.QueryRow(ctx, `UPDATE assets.units SET property_id=$3, space_id=$4, display_name=$5, model_id=$6,
		installed_at=$7, service_scope=$8,
		capability_version=$9, warranty_ends_at=$10, version = version + 1, updated_at = platform.app_now()
		WHERE id = $1 AND version = $2 AND NOT archived RETURNING version`,
		*input.ID, *c.ExpectedVersion, input.PropertyID, input.SpaceID, input.DisplayName, input.ModelID, input.InstalledAt,
		input.ServiceScope,
		capVer, warranty).
		Scan(&version)
	if err != nil {
		return Unit{}, versionedUpdate(ctx, c, "assets.units", *input.ID, err)
	}

	siblingNameErr := siblingName(ctx, c, "unit", *input.ID)
	if siblingNameErr != nil {
		return Unit{}, siblingNameErr
	}

	c.Emit(
		ops.Event{AggregateType: "unit", AggregateID: *input.ID, Type: "UnitChanged", Payload: nil},
	)
	// before/after keep the original location and values in the history (DD-A02 step 3)
	before, after := ops.Changes(
		map[string]any{
			"propertyId":     curProp,
			"spaceId":        curSpace,
			"displayName":    curName,
			"modelId":        curModel,
			"installedAt":    curInstalled,
			"serviceScope":   curScope,
			"warrantyEndsAt": curWarranty,
		},
		map[string]any{
			"propertyId":     input.PropertyID,
			"spaceId":        input.SpaceID,
			"displayName":    input.DisplayName,
			"modelId":        input.ModelID,
			"installedAt":    input.InstalledAt,
			"serviceScope":   input.ServiceScope,
			"warrantyEndsAt": warranty,
		},
	)
	c.Audit(
		ops.AuditEntry{
			Action:          "units.save",
			TargetKind:      "unit",
			TargetID:        input.ID.String(),
			PreviousVersion: c.ExpectedVersion,
			NextVersion:     &version,
			Reason:          reason,
			Before:          before,
			After:           after, Result: "",
		},
	)

	return m.load(ctx, c, *input.ID)
}

func (m *Module) load(ctx context.Context, c *ops.Call, id uuid.UUID) (Unit, error) {
	units, _, err := m.query(ctx, c, "u.id = $1", []any{id}, "u.id", 1, 0)
	if err != nil {
		return Unit{}, err
	}

	if len(units) == 0 {
		return Unit{}, notFound()
	}

	return units[0], nil
}

// propertiesArchive answers properties.archive: refused while the property has units or spaces that are not archived.
//
//	@Summary		properties.archive (write)
//	@ID				properties.archive
//	@Description	Authorization: admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A02
//	@Tags			properties
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target properties, read properties.list)"
//	@Param			id					path		string			true	"input field id"
//	@Param			request				body		ArchiveInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=ArchivedResource}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/properties/{id}/archive [post]
func (m *Module) propertiesArchive(
	ctx context.Context,
	call *ops.Call,
	input *ArchiveInput,
) (ArchivedResource, error) {
	return m.archive(
		"property",
		"assets.properties",
		`SELECT EXISTS (SELECT 1 FROM assets.units WHERE property_id = $1 AND NOT archived) OR EXISTS (SELECT 1 FROM
			assets.spaces WHERE property_id = $1 AND NOT archived)`,
	)(
		ctx,
		call,
		input,
	)
}

// spacesArchive answers spaces.archive: refused while the space has units or child spaces that are not archived.
//
//	@Summary		spaces.archive (write)
//	@ID				spaces.archive
//	@Description	Authorization: admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A02
//	@Tags			spaces
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target spaces, read spaces.list)"
//	@Param			id					path		string			true	"input field id"
//	@Param			request				body		ArchiveInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=ArchivedResource}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/spaces/{id}/archive [post]
func (m *Module) spacesArchive(
	ctx context.Context,
	call *ops.Call,
	input *ArchiveInput,
) (ArchivedResource, error) {
	return m.archive(
		"space",
		"assets.spaces",
		`SELECT EXISTS (SELECT 1 FROM assets.units WHERE space_id = $1 AND NOT archived) OR EXISTS (SELECT 1 FROM
			assets.spaces WHERE parent_space_id = $1 AND NOT archived)`,
	)(
		ctx,
		call,
		input,
	)
}

func (m *Module) archive(
	kind, table, busySQL string,
) func(context.Context, *ops.Call, *ArchiveInput) (ArchivedResource, error) {
	return func(ctx context.Context, call *ops.Call, input *ArchiveInput) (ArchivedResource, error) {
		// the row is locked before the check for active children: a unit or space added at the same moment reads its
		// parent with a share lock, so one of the two waits and then sees the other (IR478)
		_, execErr := call.Tx.Exec(
			ctx,
			"SELECT 1 FROM "+table+" WHERE id = $1 FOR UPDATE",
			input.ID,
		)
		if execErr != nil {
			return ArchivedResource{}, apperr.Wrap(execErr, "select (archive)")
		}

		if busySQL != "" {
			var busy bool

			err := call.Tx.QueryRow(ctx, busySQL, input.ID).Scan(&busy)
			if err != nil {
				return ArchivedResource{}, apperr.Wrap(err, "queryrow (archive)")
			}

			if busy {
				return ArchivedResource{}, conflict("error.notEmpty")
			}
		}

		var version int

		err := call.Tx.QueryRow(ctx, "UPDATE "+table+` SET archived = true, version = version + 1, updated_at =
			platform.app_now()
			WHERE id = $1 AND version = $2 AND NOT archived RETURNING version`, input.ID, *call.ExpectedVersion).
			Scan(&version)
		if err != nil {
			return ArchivedResource{}, versionedUpdate(ctx, call, table, input.ID, err)
		}

		typ := "LocationChanged"
		if kind == "unit" {
			typ = "UnitChanged"
		}

		call.Emit(ops.Event{AggregateType: kind, AggregateID: input.ID, Type: typ, Payload: nil})
		call.Audit(
			ops.AuditEntry{
				Action:          kind + "s.archive",
				TargetKind:      kind,
				TargetID:        input.ID.String(),
				PreviousVersion: call.ExpectedVersion,
				NextVersion:     &version,
				Reason:          input.Reason, Result: "", Before: nil, After: nil,
			},
		)

		return ArchivedResource{ID: input.ID, Version: version, Archived: true}, nil
	}
}

// unitsArchive applies D05: an active job, device binding or operation, restriction, command / run or unexpired
// contract on the unit is CONFLICT; history alone is allowed.
//
//	@Summary		units.archive (write)
//	@ID				units.archive
//	@Description	Authorization: admin:asset.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A02
//	@Tags			units
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target units, read units.get)"
//	@Param			id					path		string			true	"input field id"
//	@Param			request				body		ArchiveInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=ArchivedResource}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/units/{id}/archive [post]
func (m *Module) unitsArchive(
	ctx context.Context,
	call *ops.Call,
	input *ArchiveInput,
) (ArchivedResource, error) {
	for _, a := range m.Active {
		busy, err := a.UnitActive(ctx, call, input.ID)
		if err != nil {
			return ArchivedResource{}, apperr.Wrap(err, "assets.UnitActivity.UnitActive")
		}

		if busy {
			return ArchivedResource{}, conflict("error.unitActive")
		}
	}

	return m.archive("unit", "assets.units", "")(ctx, call, input)
}

func (m *Module) unitIDs(
	ctx context.Context,
	c *ops.Call,
	where string,
	args []any,
) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(
		ctx,
		"SELECT u.id FROM assets.units u WHERE "+where+" ORDER BY u.id",
		args...)
	if err != nil {
		return nil, apperr.Wrap(err, "select assets.units")
	}
	defer rows.Close()

	out := []uuid.UUID{}

	for rows.Next() {
		var unitID uuid.UUID

		err := rows.Scan(&unitID)
		if err != nil {
			return nil, apperr.Wrap(err, "select assets.units")
		}

		out = append(out, unitID)
	}

	return out, apperr.Wrap(rows.Err(), "select assets.units")
}
