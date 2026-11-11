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
	// IoTCurrentVersion is CurrentVersion of an IoT model (DEC-105).
	IoTCurrentVersion(ctx context.Context, c *ops.Call, modelID uuid.UUID) (int, bool, error)
}

// The longest texts the forms take, in characters (the DDs' input constraints).
const (
	maxName     = 120  // a customer, property, location or unit name
	maxFileName = 200  // an import file's name
	maxAddress  = 500  // a property's address
	maxText     = 1000 // access instructions and reasons
)

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

	if !strLen(in.Name, 1, maxName) {
		fieldErrs[keyName] = apperr.KeyLength
	}

	if in.Address != nil && !strLen(*in.Address, 0, maxAddress) {
		fieldErrs["address"] = apperr.KeyLength
	}

	if in.AccessInstructions != nil && !strLen(*in.AccessInstructions, 0, maxText) {
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
	case kindSpace:
		stmt = `SELECT 1 FROM assets.spaces o, assets.spaces t WHERE t.id = $1 AND o.property_id = t.property_id AND
			o.parent_space_id IS NOT DISTINCT FROM t.parent_space_id AND o.id <> t.id AND NOT o.archived AND lower(btrim(o.name))
			= lower(btrim(t.name))`
	case kindUnit:
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

// validSpaceKind reports whether kind is a kind of location.
func validSpaceKind(kind string) bool {
	switch kind {
	case "area", levelFloor, levelRoom, kindSpace:
		return true
	}

	return false
}

// Validate implements ops.Validator.
func (in *SpaceSave) Validate() map[string]string {
	fieldErrs := map[string]string{}

	in.Name = strings.TrimSpace(in.Name)
	if in.PropertyID == uuid.Nil {
		fieldErrs[keyPropertyID] = apperr.KeyRequired
	}

	if !validSpaceKind(in.Kind) {
		fieldErrs["kind"] = apperr.KeyInvalid
	}

	if !strLen(in.Name, 1, maxName) {
		fieldErrs[keyName] = apperr.KeyLength
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
	// IoTModelID is the unit's planned IoT model (DEC-105); null or omitted is none.
	IoTModelID   *uuid.UUID `json:"iotModelId,omitempty"`
	Type         string     `json:"type"`
	InstalledAt  *time.Time `json:"installedAt"`
	ServiceScope []string   `json:"serviceScope"`
	ChangeReason *string    `json:"changeReason,omitempty"`
	// WarrantyEndsAt is optional: omitted keeps the stored end (none for a new unit), null clears it (IR209).
	WarrantyEndsAt json.RawMessage `json:"warrantyEndsAt,omitempty" swaggertype:"object"`
	warranty       *time.Time
	warrantySet    bool
}

// validScope reports whether scope is a part of a unit a service can cover.
func validScope(scope string) bool {
	switch scope {
	case "indoor", "outdoor", "electrical":
		return true
	}

	return false
}

// Validate implements ops.Validator.
func (in *UnitSave) Validate() map[string]string {
	fieldErrs := map[string]string{}

	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.CustomerOrgID == uuid.Nil {
		fieldErrs["customerOrgId"] = apperr.KeyRequired
	}

	if in.PropertyID == uuid.Nil {
		fieldErrs[keyPropertyID] = apperr.KeyRequired
	}

	if in.ModelID == uuid.Nil {
		fieldErrs[keyModelID] = apperr.KeyRequired
	}

	if in.Type != "split" {
		fieldErrs["type"] = apperr.KeyInvalid
	}

	if !strLen(in.DisplayName, 1, maxName) {
		fieldErrs["displayName"] = apperr.KeyLength
	}

	if key := in.readWarranty(); key != "" {
		fieldErrs[keyWarrantyEndsAt] = key
	}

	seen := map[string]bool{}
	for _, s := range in.ServiceScope {
		if !validScope(s) || seen[s] {
			fieldErrs["serviceScope"] = apperr.KeyInvalid
		}

		seen[s] = true
	}

	if len(in.ServiceScope) == 0 {
		fieldErrs["serviceScope"] = apperr.KeyRequired
	}

	if in.ChangeReason != nil && !strLen(*in.ChangeReason, 1, maxText) {
		fieldErrs["changeReason"] = apperr.KeyLength
	}

	return fieldErrs
}

// readWarranty reads the warranty end the form sent: none keeps the stored one, null clears it, and a time must not
// be before the installation. It returns the field error's key, or "" when the warranty end is fine.
func (in *UnitSave) readWarranty() string {
	if len(in.WarrantyEndsAt) == 0 {
		return ""
	}

	in.warrantySet = true
	if string(in.WarrantyEndsAt) == "null" {
		return ""
	}

	var endsAt time.Time

	err := json.Unmarshal(in.WarrantyEndsAt, &endsAt)

	switch {
	case err != nil:
		return apperr.KeyInvalid
	case in.InstalledAt != nil && endsAt.Before(*in.InstalledAt):
		return apperr.KeyRange // a warranty cannot end before the installation
	}

	in.warranty = &endsAt

	return ""
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

	if !strLen(in.Reason, 1, maxText) {
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
func (*Module) MoveToCapabilityVersion(
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
				AggregateType: kindUnit,
				AggregateID:   unitID,
				Type:          eventUnitChanged,
				Payload:       nil,
			},
		)

		moved++
	}

	return moved, apperr.Wrap(rows.Err(), "update assets.units")
}

// MoveToIoTModelVersion points every unit and device of the IoT model at its new version (called by Devices on
// iotModels.save); returns the number of units changed.
func (*Module) MoveToIoTModelVersion(
	ctx context.Context,
	call *ops.Call,
	modelID uuid.UUID,
	version int,
) (int, error) {
	rows, err := call.Tx.Query(
		ctx,
		`UPDATE assets.units SET iot_model_version = $2, version = version + 1, updated_at = platform.app_now()
		WHERE iot_model_id = $1 AND iot_model_version IS DISTINCT FROM $2 RETURNING id`,
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
				AggregateType: kindUnit,
				AggregateID:   unitID,
				Type:          eventUnitChanged,
				Payload:       nil,
			},
		)

		moved++
	}

	return moved, apperr.Wrap(rows.Err(), "update assets.units")
}

// UnitInfo answers unit lookups for Devices (scope is applied by the calling operation's own rules).
func (*Module) UnitInfo(
	ctx context.Context,
	c *ops.Call,
	unit uuid.UUID,
) (devices.UnitInfo, bool, error) {
	var unitInfo devices.UnitInfo

	err := c.Tx.QueryRow(ctx, `SELECT customer_org_id, property_id, model_id, capability_version, iot_model_id FROM
		assets.units WHERE id = $1 AND NOT archived`, unit).
		Scan(&unitInfo.OrgID, &unitInfo.PropertyID, &unitInfo.ModelID, &unitInfo.CapabilityVersion, &unitInfo.IoTModelID)
	if errors.Is(err, pgx.ErrNoRows) {
		return unitInfo, false, nil
	}

	return unitInfo, err == nil, apperr.Wrap(err, "select assets.units")
}

// UnitsOfProperty lists the units of a property (Monitoring propertyId filters).
func (*Module) UnitsOfProperty(
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
func (*Module) OrgOfCustomer(
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
func (*Module) AttachedUnits(
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
func (*Module) PoliciesOfUnits(
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
func (*Module) DetachPolicy(ctx context.Context, call *ops.Call, policy uuid.UUID) error {
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
			ops.Event{
				AggregateType: kindUnit,
				AggregateID:   u,
				Type:          eventUnitChanged,
				Payload:       nil,
			},
		)
	}

	return nil
}

// OrgsOfUnits maps units to their customer organization.
func (*Module) OrgsOfUnits(
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
func (*Module) CustomerOfOrg(
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
func (*Module) CustomerState(
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

	return org, status == statusActive, err == nil, apperr.Wrap(err, "select assets.customers")
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
func (*Module) SpacesOfUnits(
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
func (*Module) SpacesOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) ([]uuid.UUID, error) {
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
func (*Module) UnitState(
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
func (*Module) UnitService(
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
func (*Module) SiteAddress(
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
func (*Module) WarrantyEnd(ctx context.Context, c *ops.Call, unit uuid.UUID) (*time.Time, error) {
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
func (*Module) CustomerProfiles(
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
func (*Module) BriefUnits(
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
func (*Module) UnitVersion(ctx context.Context, c *ops.Call, unit uuid.UUID) (int, error) {
	var v int

	err := c.Tx.QueryRow(ctx, `SELECT version FROM assets.units WHERE id = $1`, unit).Scan(&v)

	return v, apperr.Wrap(err, "select assets.units")
}

// customerActive requires an active customer, read with a share lock: customers.delete waits for the save, or is waited
// for and seen, so a property or unit is never added to a customer that is being deleted.
func (*Module) customerActive(ctx context.Context, c *ops.Call, org uuid.UUID) error {
	var status string

	err := c.Tx.QueryRow(ctx, `SELECT status FROM assets.customers WHERE organization_id = $1 FOR SHARE`, org).
		Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound()
	}

	if err != nil {
		return apperr.Wrap(err, "select assets.customers")
	}

	if status != statusActive {
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

		err = locationSaved(
			ctx,
			call,
			"properties.save",
			kindProperty,
			propertyID,
			nil,
			property.Version,
		)
		if err != nil {
			return Property{}, err
		}

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
		return Property{}, versionedUpdate(ctx, call, tableProperties, *input.ID, err)
	}

	err = locationSaved(
		ctx,
		call,
		"properties.save",
		kindProperty,
		updated.ID,
		call.ExpectedVersion,
		updated.Version,
	)
	if err != nil {
		return Property{}, err
	}

	return updated, nil
}

// locationSaved finishes the save of a property or space: its name must stay unique among its siblings, and the
// change is announced (LocationChanged) and audited from the previous version (nil for a new one) to next.
func locationSaved(
	ctx context.Context,
	call *ops.Call,
	action, kind string,
	id uuid.UUID,
	previous *int,
	next int,
) error {
	err := siblingName(ctx, call, kind, id)
	if err != nil {
		return err
	}

	call.Emit(
		ops.Event{
			AggregateType: kind,
			AggregateID:   id,
			Type:          eventLocationChanged,
			Payload:       nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          action,
			TargetKind:      kind,
			TargetID:        id.String(),
			PreviousVersion: previous,
			NextVersion:     &next,
			Reason:          "",
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)

	return nil
}

// activeProperty requires an active property, read with a share lock: an archive of it waits, or is waited for and seen
// (IR478).
func (*Module) activeProperty(ctx context.Context, c *ops.Call, id uuid.UUID) error {
	err := c.Tx.QueryRow(ctx, `SELECT 1 FROM assets.properties WHERE id = $1 AND NOT archived FOR SHARE`, id).
		Scan(new(int))
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound()
	}

	return apperr.Wrap(err, "select assets.properties")
}

// checkParent requires the parent to be an active space of the same property and, for an existing space, not one
// of its own descendants (no hierarchy cycles).
func (*Module) checkParent(
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
func (m *Module) spacesSave(ctx context.Context, call *ops.Call, input *SpaceSave) (Space, error) {
	err := m.activeProperty(ctx, call, input.PropertyID)
	if err != nil {
		return Space{}, err
	}

	err = m.checkParent(ctx, call, input.ID, input.PropertyID, input.ParentSpaceID)
	if err != nil {
		return Space{}, err
	}

	if input.ID == nil {
		spaceID := uuid.Must(uuid.NewV7())

		space, err := scanSpace(
			call.Tx.QueryRow(
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

		err = locationSaved(ctx, call, "spaces.save", kindSpace, spaceID, nil, space.Version)
		if err != nil {
			return Space{}, err
		}

		return space, nil
	}

	space, err := scanSpace(
		call.Tx.QueryRow(
			ctx,
			`UPDATE assets.spaces SET parent_space_id=$3, kind=$4, name=$5, version = version + 1, updated_at =
				platform.app_now()
		WHERE id = $1 AND version = $2 AND property_id = $6 AND NOT archived RETURNING `+spaceCols,
			*input.ID,
			*call.ExpectedVersion,
			input.ParentSpaceID,
			input.Kind,
			input.Name,
			input.PropertyID,
		),
	)
	if err != nil {
		return Space{}, versionedUpdate(ctx, call, tableSpaces, *input.ID, err)
	}

	err = locationSaved(
		ctx,
		call,
		"spaces.save",
		kindSpace,
		space.ID,
		call.ExpectedVersion,
		space.Version,
	)
	if err != nil {
		return Space{}, err
	}

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
func (m *Module) unitsSaveUnit(ctx context.Context, call *ops.Call, input *UnitSave) (Unit, error) {
	if input.InstalledAt != nil && input.InstalledAt.After(call.Now) {
		return Unit{}, apperr.Fields(map[string]string{keyInstalledAt: apperr.KeyFuture})
	}

	models, err := m.unitPlace(ctx, call, input)
	if err != nil {
		return Unit{}, err
	}

	if input.ID == nil {
		return m.addUnit(ctx, call, input, models)
	}

	return m.updateUnit(ctx, call, input, models)
}

// unitValues are the values of a unit its form changes, as stored or as the form sends them.
type unitValues struct {
	property  uuid.UUID
	space     *uuid.UUID
	name      string
	model     uuid.UUID
	iotModel  *uuid.UUID
	installed *time.Time
	scope     []string
	warranty  *time.Time
}

// history is the values as the unit's history keeps them (DD-A02 step 3).
func (values unitValues) history() map[string]any {
	return map[string]any{
		keyPropertyID:     values.property,
		keySpaceID:        values.space,
		"displayName":     values.name,
		keyModelID:        values.model,
		keyIoTModelID:     values.iotModel,
		keyInstalledAt:    values.installed,
		"serviceScope":    values.scope,
		keyWarrantyEndsAt: values.warranty,
	}
}

// moved reports whether the unit goes to another property or space.
func (values unitValues) moved(property uuid.UUID, space *uuid.UUID) bool {
	return values.property != property || (values.space == nil) != (space == nil) ||
		(values.space != nil && space != nil && *values.space != *space)
}

// storedUnit is the stored values of the customer's unit.
func storedUnit(ctx context.Context, call *ops.Call, id, org uuid.UUID) (unitValues, error) {
	var values unitValues

	err := call.Tx.QueryRow(ctx, `SELECT property_id, space_id, display_name, model_id, iot_model_id, installed_at,
		service_scope, warranty_ends_at FROM assets.units WHERE id = $1 AND customer_org_id = $2 AND NOT archived`,
		id, org).
		Scan(&values.property, &values.space, &values.name, &values.model, &values.iotModel, &values.installed,
			&values.scope, &values.warranty)
	if errors.Is(err, pgx.ErrNoRows) {
		return values, notFound()
	}

	return values, apperr.Wrap(err, "select assets.units")
}

// updateUnit saves the unit form at the version shown: a relocation (another property or space) needs a reason
// (DD-A02 changeReason), and the warranty end stays as stored unless the form sends one. The history keeps the
// original location and values (DD-A02 step 3).
func (m *Module) updateUnit(
	ctx context.Context,
	call *ops.Call,
	input *UnitSave,
	models unitModels,
) (Unit, error) {
	current, err := storedUnit(ctx, call, *input.ID, input.CustomerOrgID)
	if err != nil {
		return Unit{}, err
	}

	reason := ""

	if current.moved(input.PropertyID, input.SpaceID) {
		if input.ChangeReason == nil {
			return Unit{}, apperr.Fields(map[string]string{"changeReason": apperr.KeyRequired})
		}

		reason = *input.ChangeReason
	}

	warranty := current.warranty
	if input.warrantySet {
		warranty = input.warranty
	}

	if warranty != nil && input.InstalledAt != nil && warranty.Before(*input.InstalledAt) {
		return Unit{}, apperr.Fields(map[string]string{keyWarrantyEndsAt: apperr.KeyRange})
	}

	var version int

	err = call.Tx.QueryRow(ctx, `UPDATE assets.units SET property_id=$3, space_id=$4, display_name=$5, model_id=$6,
		installed_at=$7, service_scope=$8,
		capability_version=$9, warranty_ends_at=$10, iot_model_id=$11, iot_model_version=$12, version = version + 1,
		updated_at = platform.app_now()
		WHERE id = $1 AND version = $2 AND NOT archived RETURNING version`,
		*input.ID, *call.ExpectedVersion, input.PropertyID, input.SpaceID, input.DisplayName, input.ModelID,
		input.InstalledAt, input.ServiceScope, models.capVer, warranty, input.IoTModelID, models.iotVer).
		Scan(&version)
	if err != nil {
		return Unit{}, versionedUpdate(ctx, call, "assets.units", *input.ID, err)
	}

	err = siblingName(ctx, call, kindUnit, *input.ID)
	if err != nil {
		return Unit{}, err
	}

	call.Emit(
		ops.Event{
			AggregateType: kindUnit,
			AggregateID:   *input.ID,
			Type:          eventUnitChanged,
			Payload:       nil,
		},
	)

	next := unitValues{
		property:  input.PropertyID,
		space:     input.SpaceID,
		name:      input.DisplayName,
		model:     input.ModelID,
		iotModel:  input.IoTModelID,
		installed: input.InstalledAt,
		scope:     input.ServiceScope,
		warranty:  warranty,
	}
	before, after := ops.Changes(current.history(), next.history())
	call.Audit(
		ops.AuditEntry{
			Action:          "units.save",
			TargetKind:      kindUnit,
			TargetID:        input.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &version,
			Reason:          reason,
			Before:          before,
			After:           after,
			Result:          "",
		},
	)

	return m.load(ctx, call, *input.ID)
}

// unitPlace checks the unit's place and model: the property is the customer's, the space is in the property, and the
// model is in the register. It returns the model's current capability version.
// unitModels are the versions a unit is saved with: the aircon model's capability version and the IoT model's version
// (nil without an IoT model).
type unitModels struct {
	capVer int
	iotVer *int
}

func (m *Module) unitPlace(
	ctx context.Context,
	call *ops.Call,
	input *UnitSave,
) (unitModels, error) {
	var propOrg uuid.UUID
	// the property and the space are read with a share lock: their archive waits, or is waited for and seen (IR478)
	err := call.Tx.QueryRow(ctx, `SELECT customer_org_id FROM assets.properties WHERE id = $1 AND NOT archived FOR
		SHARE`, input.PropertyID).
		Scan(&propOrg)
	if errors.Is(err, pgx.ErrNoRows) {
		return unitModels{}, notFound()
	}

	if err != nil {
		return unitModels{}, apperr.Wrap(err, "select assets.properties")
	}

	if propOrg != input.CustomerOrgID {
		return unitModels{}, apperr.Fields(map[string]string{keyPropertyID: "error.otherCustomer"})
	}

	if input.SpaceID != nil {
		err := call.Tx.QueryRow(ctx, `SELECT 1 FROM assets.spaces WHERE id = $1 AND property_id = $2 AND NOT archived FOR
			SHARE`, *input.SpaceID, input.PropertyID).
			Scan(new(int))
		if errors.Is(err, pgx.ErrNoRows) {
			return unitModels{}, apperr.Fields(map[string]string{keySpaceID: "error.otherProperty"})
		}

		if err != nil {
			return unitModels{}, apperr.Wrap(err, "select assets.spaces")
		}
	}

	capVer, ok, err := m.Models.CurrentVersion(ctx, call, input.ModelID)
	if err != nil {
		return unitModels{}, apperr.Wrap(err, "assets.Models.CurrentVersion")
	}

	if !ok {
		return unitModels{}, apperr.Fields(map[string]string{keyModelID: "error.unknownModel"})
	}

	models := unitModels{capVer: capVer, iotVer: nil}
	if input.IoTModelID == nil {
		return models, nil
	}

	iotVer, ok, err := m.Models.IoTCurrentVersion(ctx, call, *input.IoTModelID)
	if err != nil {
		return unitModels{}, apperr.Wrap(err, "assets.Models.IoTCurrentVersion")
	}

	if !ok {
		return unitModels{}, apperr.Fields(map[string]string{keyIoTModelID: "error.unknownModel"})
	}

	models.iotVer = &iotVer

	return models, nil
}

// addUnit creates the unit of an active customer with the model's current capability version.
func (m *Module) addUnit(
	ctx context.Context,
	call *ops.Call,
	input *UnitSave,
	models unitModels,
) (Unit, error) {
	err := m.customerActive(ctx, call, input.CustomerOrgID)
	if err != nil {
		return Unit{}, err
	}

	unitID := uuid.Must(uuid.NewV7())

	_, err = call.Tx.Exec(
		ctx,
		`INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id, type,
			installed_at, service_scope, capability_version, warranty_ends_at, iot_model_id, iot_model_version)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)`,
		unitID,
		input.CustomerOrgID,
		input.PropertyID,
		input.SpaceID,
		input.DisplayName,
		input.ModelID,
		input.Type,
		input.InstalledAt,
		input.ServiceScope,
		models.capVer,
		input.warranty,
		input.IoTModelID,
		models.iotVer,
	)
	if err != nil {
		return Unit{}, apperr.Wrap(err, "insert assets.units")
	}

	err = siblingName(ctx, call, kindUnit, unitID)
	if err != nil {
		return Unit{}, err
	}

	call.Emit(
		ops.Event{
			AggregateType: kindUnit,
			AggregateID:   unitID,
			Type:          eventUnitChanged,
			Payload:       nil,
		},
	)

	one := 1
	call.Audit(
		ops.AuditEntry{
			Action:          "units.save",
			TargetKind:      kindUnit,
			TargetID:        unitID.String(),
			NextVersion:     &one,
			PreviousVersion: nil,
			Reason:          "",
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)

	return m.load(ctx, call, unitID)
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
		kindProperty,
		tableProperties,
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
		kindSpace,
		tableSpaces,
		`SELECT EXISTS (SELECT 1 FROM assets.units WHERE space_id = $1 AND NOT archived) OR EXISTS (SELECT 1 FROM
			assets.spaces WHERE parent_space_id = $1 AND NOT archived)`,
	)(
		ctx,
		call,
		input,
	)
}

func (*Module) archive(
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

		typ := eventLocationChanged
		if kind == kindUnit {
			typ = eventUnitChanged
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

	return m.archive(kindUnit, "assets.units", "")(ctx, call, input)
}

func (*Module) unitIDs(
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
