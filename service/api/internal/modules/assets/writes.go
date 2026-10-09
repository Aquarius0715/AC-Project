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

func strLen(s string, lo, hi int) bool { n := utf8.RuneCountInString(s); return n >= lo && n <= hi }

func notFound() error           { return apperr.E(apperr.NotFound, "error.notFound") }
func conflict(key string) error { return apperr.E(apperr.Conflict, key) }

// versionedUpdate turns "no row updated" into NOT_FOUND (target missing / out of scope) or CONFLICT (version moved).
func versionedUpdate(ctx context.Context, c *ops.Call, table string, id uuid.UUID, err error) error {
	if !errors.Is(err, pgx.ErrNoRows) {
		return dupSibling(err)
	}
	var exists bool
	_ = c.Tx.QueryRow(ctx, "SELECT EXISTS (SELECT 1 FROM "+table+" WHERE id = $1)", id).Scan(&exists)
	if exists {
		return conflict("error.versionConflict")
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

const propertyCols = `id, tenant_id, version, created_at, updated_at, customer_org_id, kind, name, address, access_instructions, archived`

func scanProperty(row pgx.Row) (Property, error) {
	var p Property
	err := row.Scan(&p.ID, &p.TenantID, &p.Version, &p.CreatedAt, &p.UpdatedAt, &p.CustomerOrgID, &p.Kind, &p.Name, &p.Address, &p.AccessInstructions, &p.Archived)
	return p, err
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
	fe := map[string]string{}
	in.Name = strings.TrimSpace(in.Name) // 1–120 characters after trim, as in locations.rename
	if in.CustomerOrgID == uuid.Nil {
		fe["customerOrgId"] = "error.required"
	}
	if in.Kind != "home" && in.Kind != "office" {
		fe["kind"] = "error.invalid"
	}
	if !strLen(in.Name, 1, 120) {
		fe["name"] = "error.length"
	}
	if in.Address != nil && !strLen(*in.Address, 0, 500) {
		fe["address"] = "error.length"
	}
	if in.AccessInstructions != nil && !strLen(*in.AccessInstructions, 0, 1000) {
		fe["accessInstructions"] = "error.length"
	}
	return fe
}

func (m *Module) customerActive(ctx context.Context, c *ops.Call, org uuid.UUID) error {
	var status string
	err := c.Tx.QueryRow(ctx, `SELECT status FROM assets.customers WHERE organization_id = $1`, org).Scan(&status)
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound()
	}
	if err != nil {
		return err
	}
	if status != "active" {
		return conflict("error.customerInactive")
	}
	return nil
}

// dupSibling maps the properties_name / spaces_name unique indexes (lower(name) per parent) to the sibling-name
// CONFLICT of siblingName (IR208).
func dupSibling(err error) error {
	var pg *pgconn.PgError
	if errors.As(err, &pg) && pg.Code == "23505" && (pg.ConstraintName == "properties_name" || pg.ConstraintName == "spaces_name") {
		return conflict("error.duplicateSiblingName")
	}
	return err
}

// siblingName applies the locations.rename sibling rule to the HQ saves (DD-A02, IR208): another non-archived
// property of the customer, space under the same parent or unit in the same space with the same trimmed,
// case-insensitive name is CONFLICT. It runs after the write in the same transaction, so NOT_FOUND and version
// conflicts come first and a duplicate rolls the write back.
func siblingName(ctx context.Context, c *ops.Call, kind string, id uuid.UUID) error {
	q := `SELECT 1 FROM assets.properties o, assets.properties t WHERE t.id = $1 AND o.customer_org_id = t.customer_org_id AND o.id <> t.id AND NOT o.archived AND lower(btrim(o.name)) = lower(btrim(t.name))`
	switch kind {
	case "space":
		q = `SELECT 1 FROM assets.spaces o, assets.spaces t WHERE t.id = $1 AND o.property_id = t.property_id AND o.parent_space_id IS NOT DISTINCT FROM t.parent_space_id AND o.id <> t.id AND NOT o.archived AND lower(btrim(o.name)) = lower(btrim(t.name))`
	case "unit":
		q = `SELECT 1 FROM assets.units o, assets.units t WHERE t.id = $1 AND o.property_id = t.property_id AND o.space_id IS NOT DISTINCT FROM t.space_id AND o.id <> t.id AND NOT o.archived AND lower(btrim(o.display_name)) = lower(btrim(t.display_name))`
	}
	var taken bool
	if err := c.Tx.QueryRow(ctx, "SELECT EXISTS ("+q+")", id).Scan(&taken); err != nil {
		return err
	}
	if taken {
		return conflict("error.duplicateSiblingName")
	}
	return nil
}

func (m *Module) propertiesSave(ctx context.Context, c *ops.Call, in *PropertySave) (Property, error) {
	if in.ID == nil {
		if err := m.customerActive(ctx, c, in.CustomerOrgID); err != nil {
			return Property{}, err
		}
		id := uuid.Must(uuid.NewV7())
		p, err := scanProperty(c.Tx.QueryRow(ctx, `INSERT INTO assets.properties (id, tenant_id, customer_org_id, kind, name, address, access_instructions)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6) RETURNING `+propertyCols,
			id, in.CustomerOrgID, in.Kind, in.Name, in.Address, in.AccessInstructions))
		if err != nil {
			return Property{}, dupSibling(err)
		}
		if err := siblingName(ctx, c, "property", id); err != nil {
			return Property{}, err
		}
		c.Emit(ops.Event{AggregateType: "property", AggregateID: id, Type: "LocationChanged"})
		c.Audit(ops.AuditEntry{Action: "properties.save", TargetKind: "property", TargetID: id.String(), NextVersion: &p.Version})
		return p, nil
	}
	p, err := scanProperty(c.Tx.QueryRow(ctx, `UPDATE assets.properties SET kind=$3, name=$4, address=$5, access_instructions=$6,
		version = version + 1, updated_at = platform.app_now()
		WHERE id = $1 AND version = $2 AND customer_org_id = $7 AND NOT archived RETURNING `+propertyCols,
		*in.ID, *c.ExpectedVersion, in.Kind, in.Name, in.Address, in.AccessInstructions, in.CustomerOrgID))
	if err != nil {
		return Property{}, versionedUpdate(ctx, c, "assets.properties", *in.ID, err)
	}
	if err := siblingName(ctx, c, "property", p.ID); err != nil {
		return Property{}, err
	}
	c.Emit(ops.Event{AggregateType: "property", AggregateID: p.ID, Type: "LocationChanged"})
	c.Audit(ops.AuditEntry{Action: "properties.save", TargetKind: "property", TargetID: p.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &p.Version})
	return p, nil
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
	var s Space
	err := row.Scan(&s.ID, &s.TenantID, &s.Version, &s.CreatedAt, &s.UpdatedAt, &s.PropertyID, &s.ParentSpaceID, &s.Kind, &s.Name, &s.Archived)
	return s, err
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
	fe := map[string]string{}
	in.Name = strings.TrimSpace(in.Name)
	if in.PropertyID == uuid.Nil {
		fe["propertyId"] = "error.required"
	}
	if !spaceKinds[in.Kind] {
		fe["kind"] = "error.invalid"
	}
	if !strLen(in.Name, 1, 120) {
		fe["name"] = "error.length"
	}
	if in.ID != nil && in.ParentSpaceID != nil && *in.ID == *in.ParentSpaceID {
		fe["parentSpaceId"] = "error.cycle"
	}
	return fe
}

func (m *Module) activeProperty(ctx context.Context, c *ops.Call, id uuid.UUID) error {
	var ok bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.properties WHERE id = $1 AND NOT archived)`, id).Scan(&ok); err != nil {
		return err
	}
	if !ok {
		return notFound()
	}
	return nil
}

// checkParent requires the parent to be an active space of the same property and, for an existing space, not one
// of its own descendants (no hierarchy cycles).
func (m *Module) checkParent(ctx context.Context, c *ops.Call, self *uuid.UUID, property uuid.UUID, parent *uuid.UUID) error {
	if parent == nil {
		return nil
	}
	var same bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.spaces WHERE id = $1 AND property_id = $2 AND NOT archived)`, *parent, property).Scan(&same); err != nil {
		return err
	}
	if !same {
		return apperr.Fields(map[string]string{"parentSpaceId": "error.otherProperty"})
	}
	if self == nil {
		return nil
	}
	var cycle bool
	if err := c.Tx.QueryRow(ctx, `WITH RECURSIVE d AS (SELECT id FROM assets.spaces WHERE parent_space_id = $1
		UNION ALL SELECT s.id FROM assets.spaces s JOIN d ON s.parent_space_id = d.id) SELECT EXISTS (SELECT 1 FROM d WHERE id = $2)`, *self, *parent).Scan(&cycle); err != nil {
		return err
	}
	if cycle {
		return apperr.Fields(map[string]string{"parentSpaceId": "error.cycle"})
	}
	return nil
}

func (m *Module) spacesSave(ctx context.Context, c *ops.Call, in *SpaceSave) (Space, error) {
	if err := m.activeProperty(ctx, c, in.PropertyID); err != nil {
		return Space{}, err
	}
	if err := m.checkParent(ctx, c, in.ID, in.PropertyID, in.ParentSpaceID); err != nil {
		return Space{}, err
	}
	if in.ID == nil {
		id := uuid.Must(uuid.NewV7())
		s, err := scanSpace(c.Tx.QueryRow(ctx, `INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5) RETURNING `+spaceCols, id, in.PropertyID, in.ParentSpaceID, in.Kind, in.Name))
		if err != nil {
			return Space{}, dupSibling(err)
		}
		if err := siblingName(ctx, c, "space", id); err != nil {
			return Space{}, err
		}
		c.Emit(ops.Event{AggregateType: "space", AggregateID: id, Type: "LocationChanged"})
		c.Audit(ops.AuditEntry{Action: "spaces.save", TargetKind: "space", TargetID: id.String(), NextVersion: &s.Version})
		return s, nil
	}
	s, err := scanSpace(c.Tx.QueryRow(ctx, `UPDATE assets.spaces SET parent_space_id=$3, kind=$4, name=$5, version = version + 1, updated_at = platform.app_now()
		WHERE id = $1 AND version = $2 AND property_id = $6 AND NOT archived RETURNING `+spaceCols,
		*in.ID, *c.ExpectedVersion, in.ParentSpaceID, in.Kind, in.Name, in.PropertyID))
	if err != nil {
		return Space{}, versionedUpdate(ctx, c, "assets.spaces", *in.ID, err)
	}
	if err := siblingName(ctx, c, "space", s.ID); err != nil {
		return Space{}, err
	}
	c.Emit(ops.Event{AggregateType: "space", AggregateID: s.ID, Type: "LocationChanged"})
	c.Audit(ops.AuditEntry{Action: "spaces.save", TargetKind: "space", TargetID: s.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &s.Version})
	return s, nil
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
	WarrantyEndsAt json.RawMessage `json:"warrantyEndsAt,omitempty"`
	warranty       *time.Time
	warrantySet    bool
}

var scopes = map[string]bool{"indoor": true, "outdoor": true, "electrical": true}

// Validate implements ops.Validator.
func (in *UnitSave) Validate() map[string]string {
	fe := map[string]string{}
	in.DisplayName = strings.TrimSpace(in.DisplayName)
	if in.CustomerOrgID == uuid.Nil {
		fe["customerOrgId"] = "error.required"
	}
	if in.PropertyID == uuid.Nil {
		fe["propertyId"] = "error.required"
	}
	if in.ModelID == uuid.Nil {
		fe["modelId"] = "error.required"
	}
	if in.Type != "split" {
		fe["type"] = "error.invalid"
	}
	if !strLen(in.DisplayName, 1, 120) {
		fe["displayName"] = "error.length"
	}
	if len(in.WarrantyEndsAt) > 0 {
		in.warrantySet = true
		if string(in.WarrantyEndsAt) != "null" {
			var t time.Time
			if err := json.Unmarshal(in.WarrantyEndsAt, &t); err != nil {
				fe["warrantyEndsAt"] = "error.invalid"
			} else if in.InstalledAt != nil && t.Before(*in.InstalledAt) {
				fe["warrantyEndsAt"] = "error.range" // a warranty cannot end before the installation
			} else {
				in.warranty = &t
			}
		}
	}
	seen := map[string]bool{}
	for _, s := range in.ServiceScope {
		if !scopes[s] || seen[s] {
			fe["serviceScope"] = "error.invalid"
		}
		seen[s] = true
	}
	if len(in.ServiceScope) == 0 {
		fe["serviceScope"] = "error.required"
	}
	if in.ChangeReason != nil && !strLen(*in.ChangeReason, 1, 1000) {
		fe["changeReason"] = "error.length"
	}
	return fe
}

func (m *Module) unitsSave(ctx context.Context, c *ops.Call, in *UnitSave) (Unit, error) {
	if in.InstalledAt != nil && in.InstalledAt.After(c.Now) {
		return Unit{}, apperr.Fields(map[string]string{"installedAt": "error.future"})
	}
	var propOrg uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT customer_org_id FROM assets.properties WHERE id = $1 AND NOT archived`, in.PropertyID).Scan(&propOrg)
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, notFound()
	}
	if err != nil {
		return Unit{}, err
	}
	if propOrg != in.CustomerOrgID {
		return Unit{}, apperr.Fields(map[string]string{"propertyId": "error.otherCustomer"})
	}
	if in.SpaceID != nil {
		var ok bool
		if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.spaces WHERE id = $1 AND property_id = $2 AND NOT archived)`, *in.SpaceID, in.PropertyID).Scan(&ok); err != nil {
			return Unit{}, err
		}
		if !ok {
			return Unit{}, apperr.Fields(map[string]string{"spaceId": "error.otherProperty"})
		}
	}
	capVer, ok, err := m.Models.CurrentVersion(ctx, c, in.ModelID)
	if err != nil {
		return Unit{}, err
	}
	if !ok {
		return Unit{}, apperr.Fields(map[string]string{"modelId": "error.unknownModel"})
	}
	if in.ID == nil {
		if err := m.customerActive(ctx, c, in.CustomerOrgID); err != nil {
			return Unit{}, err
		}
		id := uuid.Must(uuid.NewV7())
		if _, err := c.Tx.Exec(ctx, `INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id, type, installed_at, service_scope, capability_version, warranty_ends_at)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)`,
			id, in.CustomerOrgID, in.PropertyID, in.SpaceID, in.DisplayName, in.ModelID, in.Type, in.InstalledAt, in.ServiceScope, capVer, in.warranty); err != nil {
			return Unit{}, err
		}
		if err := siblingName(ctx, c, "unit", id); err != nil {
			return Unit{}, err
		}
		c.Emit(ops.Event{AggregateType: "unit", AggregateID: id, Type: "UnitChanged"})
		one := 1
		c.Audit(ops.AuditEntry{Action: "units.save", TargetKind: "unit", TargetID: id.String(), NextVersion: &one})
		return m.load(ctx, c, id)
	}
	// relocation (property or space change) requires a reason (DD-A02 changeReason)
	var curProp uuid.UUID
	var curSpace *uuid.UUID
	var curName string
	var curModel uuid.UUID
	var curInstalled *time.Time
	var curScope []string
	var curWarranty *time.Time
	err = c.Tx.QueryRow(ctx, `SELECT property_id, space_id, display_name, model_id, installed_at, service_scope, warranty_ends_at FROM assets.units WHERE id = $1 AND customer_org_id = $2 AND NOT archived`,
		*in.ID, in.CustomerOrgID).Scan(&curProp, &curSpace, &curName, &curModel, &curInstalled, &curScope, &curWarranty)
	if errors.Is(err, pgx.ErrNoRows) {
		return Unit{}, notFound()
	}
	if err != nil {
		return Unit{}, err
	}
	moved := curProp != in.PropertyID || (curSpace == nil) != (in.SpaceID == nil) || (curSpace != nil && in.SpaceID != nil && *curSpace != *in.SpaceID)
	reason := ""
	if moved {
		if in.ChangeReason == nil {
			return Unit{}, apperr.Fields(map[string]string{"changeReason": "error.required"})
		}
		reason = *in.ChangeReason
	}
	warranty := curWarranty
	if in.warrantySet {
		warranty = in.warranty
	}
	if warranty != nil && in.InstalledAt != nil && warranty.Before(*in.InstalledAt) {
		return Unit{}, apperr.Fields(map[string]string{"warrantyEndsAt": "error.range"})
	}
	var v int
	err = c.Tx.QueryRow(ctx, `UPDATE assets.units SET property_id=$3, space_id=$4, display_name=$5, model_id=$6, installed_at=$7, service_scope=$8,
		capability_version=$9, warranty_ends_at=$10, version = version + 1, updated_at = platform.app_now()
		WHERE id = $1 AND version = $2 AND NOT archived RETURNING version`,
		*in.ID, *c.ExpectedVersion, in.PropertyID, in.SpaceID, in.DisplayName, in.ModelID, in.InstalledAt, in.ServiceScope, capVer, warranty).Scan(&v)
	if err != nil {
		return Unit{}, versionedUpdate(ctx, c, "assets.units", *in.ID, err)
	}
	if err := siblingName(ctx, c, "unit", *in.ID); err != nil {
		return Unit{}, err
	}
	c.Emit(ops.Event{AggregateType: "unit", AggregateID: *in.ID, Type: "UnitChanged"})
	// before/after keep the original location and values in the history (DD-A02 step 3)
	before, after := ops.Changes(
		map[string]any{"propertyId": curProp, "spaceId": curSpace, "displayName": curName, "modelId": curModel, "installedAt": curInstalled, "serviceScope": curScope, "warrantyEndsAt": curWarranty},
		map[string]any{"propertyId": in.PropertyID, "spaceId": in.SpaceID, "displayName": in.DisplayName, "modelId": in.ModelID, "installedAt": in.InstalledAt, "serviceScope": in.ServiceScope, "warrantyEndsAt": warranty})
	c.Audit(ops.AuditEntry{Action: "units.save", TargetKind: "unit", TargetID: in.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &v, Reason: reason, Before: before, After: after})
	return m.load(ctx, c, *in.ID)
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

// ---- archive ----

// ArchiveInput is the input of properties/spaces/units.archive.
type ArchiveInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *ArchiveInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.ID == uuid.Nil {
		fe["id"] = "error.required"
	}
	if !strLen(in.Reason, 1, 1000) {
		fe["reason"] = "error.length"
	}
	return fe
}

// ArchivedResource is ArchivedResource of service-contracts.ts.
type ArchivedResource struct {
	ID       uuid.UUID `json:"id"`
	Version  int       `json:"version"`
	Archived bool      `json:"archived"`
}

func (m *Module) archive(kind, table, busySQL string) func(context.Context, *ops.Call, *ArchiveInput) (ArchivedResource, error) {
	return func(ctx context.Context, c *ops.Call, in *ArchiveInput) (ArchivedResource, error) {
		if busySQL != "" {
			var busy bool
			if err := c.Tx.QueryRow(ctx, busySQL, in.ID).Scan(&busy); err != nil {
				return ArchivedResource{}, err
			}
			if busy {
				return ArchivedResource{}, conflict("error.notEmpty")
			}
		}
		var v int
		err := c.Tx.QueryRow(ctx, "UPDATE "+table+` SET archived = true, version = version + 1, updated_at = platform.app_now()
			WHERE id = $1 AND version = $2 AND NOT archived RETURNING version`, in.ID, *c.ExpectedVersion).Scan(&v)
		if err != nil {
			return ArchivedResource{}, versionedUpdate(ctx, c, table, in.ID, err)
		}
		typ := "LocationChanged"
		if kind == "unit" {
			typ = "UnitChanged"
		}
		c.Emit(ops.Event{AggregateType: kind, AggregateID: in.ID, Type: typ})
		c.Audit(ops.AuditEntry{Action: kind + "s.archive", TargetKind: kind, TargetID: in.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &v, Reason: in.Reason})
		return ArchivedResource{ID: in.ID, Version: v, Archived: true}, nil
	}
}

// MoveToCapabilityVersion points every unit of the model at its new capability version (called by Devices on
// capabilities.save, DD-A04 step 3); returns the number of units changed.
func (m *Module) MoveToCapabilityVersion(ctx context.Context, c *ops.Call, modelID uuid.UUID, version int) (int, error) {
	rows, err := c.Tx.Query(ctx, `UPDATE assets.units SET capability_version = $2, version = version + 1, updated_at = platform.app_now()
		WHERE model_id = $1 AND capability_version <> $2 RETURNING id`, modelID, version)
	if err != nil {
		return 0, err
	}
	defer rows.Close()
	n := 0
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return n, err
		}
		c.Emit(ops.Event{AggregateType: "unit", AggregateID: id, Type: "UnitChanged"})
		n++
	}
	return n, rows.Err()
}

// UnitInfo answers unit lookups for Devices (scope is applied by the calling operation's own rules).
func (m *Module) UnitInfo(ctx context.Context, c *ops.Call, unit uuid.UUID) (devices.UnitInfo, bool, error) {
	var u devices.UnitInfo
	err := c.Tx.QueryRow(ctx, `SELECT customer_org_id, property_id, model_id, capability_version FROM assets.units WHERE id = $1 AND NOT archived`, unit).
		Scan(&u.OrgID, &u.PropertyID, &u.ModelID, &u.CapabilityVersion)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, false, nil
	}
	return u, err == nil, err
}

// unitsArchive applies D05: an active job, device binding or operation, restriction, command / run or unexpired
// contract on the unit is CONFLICT; history alone is allowed.
func (m *Module) unitsArchive(ctx context.Context, c *ops.Call, in *ArchiveInput) (ArchivedResource, error) {
	for _, a := range m.Active {
		busy, err := a.UnitActive(ctx, c, in.ID)
		if err != nil {
			return ArchivedResource{}, err
		}
		if busy {
			return ArchivedResource{}, conflict("error.unitActive")
		}
	}
	return m.archive("unit", "assets.units", "")(ctx, c, in)
}

// UnitsOfProperty lists the units of a property (Monitoring propertyId filters).
func (m *Module) UnitsOfProperty(ctx context.Context, c *ops.Call, property uuid.UUID) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, `SELECT id FROM assets.units WHERE property_id = $1`, property)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// OrgOfCustomer maps a customer ID to its organization.
func (m *Module) OrgOfCustomer(ctx context.Context, c *ops.Call, customer uuid.UUID) (uuid.UUID, bool, error) {
	var org uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT organization_id FROM assets.customers WHERE id = $1`, customer).Scan(&org)
	if errors.Is(err, pgx.ErrNoRows) {
		return org, false, nil
	}
	return org, err == nil, err
}

// AttachedUnits returns the units carrying each alert policy (IR108: units are the attachment source).
func (m *Module) AttachedUnits(ctx context.Context, c *ops.Call, policies []uuid.UUID) (map[uuid.UUID][]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, `SELECT policy_id, unit_id FROM assets.unit_alert_policies WHERE policy_id = ANY($1) ORDER BY policy_id, unit_id`, policies)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID][]uuid.UUID{}
	for rows.Next() {
		var p, u uuid.UUID
		if err := rows.Scan(&p, &u); err != nil {
			return nil, err
		}
		out[p] = append(out[p], u)
	}
	return out, rows.Err()
}

// PoliciesOfUnits returns the alert policies attached to any of the units.
func (m *Module) PoliciesOfUnits(ctx context.Context, c *ops.Call, units []uuid.UUID) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, `SELECT DISTINCT policy_id FROM assets.unit_alert_policies WHERE unit_id = ANY($1)`, units)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DetachPolicy removes an alert policy from every unit and bumps the version of each changed unit (policies.delete, IR108).
func (m *Module) DetachPolicy(ctx context.Context, c *ops.Call, policy uuid.UUID) error {
	rows, err := c.Tx.Query(ctx, `DELETE FROM assets.unit_alert_policies WHERE policy_id = $1 RETURNING unit_id`, policy)
	if err != nil {
		return err
	}
	var units []uuid.UUID
	for rows.Next() {
		var u uuid.UUID
		if err := rows.Scan(&u); err != nil {
			rows.Close()
			return err
		}
		units = append(units, u)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return err
	}
	if len(units) == 0 {
		return nil
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE assets.units SET version = version + 1, updated_at = platform.app_now() WHERE id = ANY($1)`, units); err != nil {
		return err
	}
	for _, u := range units {
		c.Emit(ops.Event{AggregateType: "unit", AggregateID: u, Type: "UnitChanged"})
	}
	return nil
}

// OrgsOfUnits maps units to their customer organization.
func (m *Module) OrgsOfUnits(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, `SELECT id, customer_org_id FROM assets.units WHERE id = ANY($1)`, units)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID]uuid.UUID{}
	for rows.Next() {
		var u, o uuid.UUID
		if err := rows.Scan(&u, &o); err != nil {
			return nil, err
		}
		out[u] = o
	}
	return out, rows.Err()
}

// CustomerOfOrg maps an organization to its customer.
func (m *Module) CustomerOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) (uuid.UUID, bool, error) {
	var id uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT id FROM assets.customers WHERE organization_id = $1`, org).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) {
		return id, false, nil
	}
	return id, err == nil, err
}

// CustomerState returns the organization of a customer and whether it is active.
func (m *Module) CustomerState(ctx context.Context, c *ops.Call, customer uuid.UUID) (uuid.UUID, bool, bool, error) {
	var org uuid.UUID
	var status string
	err := c.Tx.QueryRow(ctx, `SELECT organization_id, status FROM assets.customers WHERE id = $1`, customer).Scan(&org, &status)
	if errors.Is(err, pgx.ErrNoRows) {
		return org, false, false, nil
	}
	return org, status == "active", err == nil, err
}

// VisibleUnits returns the given units that the caller may read (D01 scope; archived units included for history).
func (m *Module) VisibleUnits(ctx context.Context, c *ops.Call, units []uuid.UUID) ([]uuid.UUID, error) {
	args := []any{units}
	return m.unitIDs(ctx, c, "u.id = ANY($1) AND "+scopeSQL(c, &args), args)
}

// UnitsInSpace returns the caller-readable units placed directly in a space.
func (m *Module) UnitsInSpace(ctx context.Context, c *ops.Call, space uuid.UUID) ([]uuid.UUID, error) {
	args := []any{space}
	return m.unitIDs(ctx, c, "u.space_id = $1 AND "+scopeSQL(c, &args), args)
}

func (m *Module) unitIDs(ctx context.Context, c *ops.Call, where string, args []any) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, "SELECT u.id FROM assets.units u WHERE "+where+" ORDER BY u.id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// SpaceInfo returns the customer organization of a space and the units placed directly in it.
func (m *Module) SpaceInfo(ctx context.Context, c *ops.Call, space uuid.UUID) (uuid.UUID, []uuid.UUID, bool, error) {
	var org uuid.UUID
	err := c.Tx.QueryRow(ctx, `SELECT p.customer_org_id FROM assets.spaces s JOIN assets.properties p ON p.id = s.property_id WHERE s.id = $1`, space).Scan(&org)
	if errors.Is(err, pgx.ErrNoRows) {
		return org, nil, false, nil
	}
	if err != nil {
		return org, nil, false, err
	}
	units, err := m.unitIDs(ctx, c, "u.space_id = $1", []any{space})
	return org, units, true, err
}

// SpacesOfUnits returns the spaces of the given units (units without a space are skipped).
func (m *Module) SpacesOfUnits(ctx context.Context, c *ops.Call, units []uuid.UUID) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, `SELECT DISTINCT space_id FROM assets.units WHERE id = ANY($1) AND space_id IS NOT NULL`, units)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ScopedUnits returns every unit whose equipment data the caller may read (Equipment mode: external technicians
// only inside their work window, IR49(b)); used for telemetry list scopes.
func (m *Module) ScopedUnits(ctx context.Context, c *ops.Call) ([]uuid.UUID, error) {
	var args []any
	return m.unitIDs(ctx, c, unitscope.SQL(c, &args, "u.id", unitscope.Equipment), args)
}

// SpacesOfOrg returns every space in the properties of a customer organization.
func (m *Module) SpacesOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) ([]uuid.UUID, error) {
	rows, err := c.Tx.Query(ctx, `SELECT s.id FROM assets.spaces s JOIN assets.properties p ON p.id = s.property_id WHERE p.customer_org_id = $1`, org)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []uuid.UUID{}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// UnitState returns the customer organization of a unit and whether it is archived (Maintenance job intake).
func (m *Module) UnitState(ctx context.Context, c *ops.Call, unit uuid.UUID) (uuid.UUID, bool, bool, error) {
	var org uuid.UUID
	var archived bool
	err := c.Tx.QueryRow(ctx, `SELECT customer_org_id, archived FROM assets.units WHERE id = $1`, unit).Scan(&org, &archived)
	if errors.Is(err, pgx.ErrNoRows) {
		return org, false, false, nil
	}
	return org, archived, err == nil, err
}

// UnitService returns the organization, property and service scope of a unit (Maintenance qualification checks).
func (m *Module) UnitService(ctx context.Context, c *ops.Call, unit uuid.UUID) (uuid.UUID, uuid.UUID, []string, error) {
	var org, prop uuid.UUID
	var scope []string
	err := c.Tx.QueryRow(ctx, `SELECT customer_org_id, property_id, service_scope FROM assets.units WHERE id = $1`, unit).Scan(&org, &prop, &scope)
	return org, prop, scope, err
}

// SiteAddress returns the IR25 installation address of a unit (null for blank) and the unit's service scope.
func (m *Module) SiteAddress(ctx context.Context, c *ops.Call, unit uuid.UUID) (*string, []string, error) {
	var addr *string
	var scope []string
	err := c.Tx.QueryRow(ctx, `SELECT NULLIF(btrim(p.address), ''), u.service_scope FROM assets.units u JOIN assets.properties p ON p.id = u.property_id WHERE u.id = $1`, unit).
		Scan(&addr, &scope)
	return addr, scope, err
}

// WarrantyEnd returns a unit's warrantyEndsAt (Maintenance warranty claims).
func (m *Module) WarrantyEnd(ctx context.Context, c *ops.Call, unit uuid.UUID) (*time.Time, error) {
	var t *time.Time
	err := c.Tx.QueryRow(ctx, `SELECT warranty_ends_at FROM assets.units WHERE id = $1`, unit).Scan(&t)
	return t, err
}

// UnitsOfOrg returns the units of a customer organization.
func (m *Module) UnitsOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) ([]uuid.UUID, error) {
	return m.unitIDs(ctx, c, "u.customer_org_id = $1", []any{org})
}

// CustomerProfiles maps customer organizations to {customerId, serviceProfile} (Maintenance SLA, IR131).
func (m *Module) CustomerProfiles(ctx context.Context, c *ops.Call, orgs []uuid.UUID) (map[uuid.UUID][2]string, error) {
	rows, err := c.Tx.Query(ctx, `SELECT organization_id, id::text, service_profile FROM assets.customers WHERE organization_id = ANY($1)`, orgs)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := map[uuid.UUID][2]string{}
	for rows.Next() {
		var org uuid.UUID
		var id, prof string
		if err := rows.Scan(&org, &id, &prof); err != nil {
			return nil, err
		}
		out[org] = [2]string{id, prof}
	}
	return out, rows.Err()
}

// BriefUnits lists the caller-readable, not archived units matching the optional customer / property / space / unit filters.
func (m *Module) BriefUnits(ctx context.Context, c *ops.Call, customer, property, space, unit *uuid.UUID) ([]ops.UnitBrief, error) {
	args := []any{}
	where := "NOT u.archived AND " + scopeSQL(c, &args)
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	if customer != nil {
		where += " AND u.customer_org_id = (SELECT organization_id FROM assets.customers WHERE id = " + add(*customer) + ")"
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
	rows, err := c.Tx.Query(ctx, "SELECT u.id, u.customer_org_id, u.connection FROM assets.units u WHERE "+where+" ORDER BY u.id", args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []ops.UnitBrief
	for rows.Next() {
		var b ops.UnitBrief
		if err := rows.Scan(&b.ID, &b.OrgID, &b.Connection); err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

// UnitVersion returns a unit's current version (Control expectedUnitVersion).
func (m *Module) UnitVersion(ctx context.Context, c *ops.Call, unit uuid.UUID) (int, error) {
	var v int
	err := c.Tx.QueryRow(ctx, `SELECT version FROM assets.units WHERE id = $1`, unit).Scan(&v)
	return v, err
}
