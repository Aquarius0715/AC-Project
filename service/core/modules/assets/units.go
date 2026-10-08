// Package assets implements the Assets module (customers, properties, spaces, locations, units).
package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/core/ops"
	"github.com/pradita/ac-project/service/core/platform/apperr"
	"github.com/pradita/ac-project/service/core/platform/paging"
	"github.com/pradita/ac-project/service/core/platform/unitscope"
)

// ObservedState is ObservedState of service-contracts.ts.
type ObservedState struct {
	Power      *bool      `json:"power"`
	Celsius    *float64   `json:"celsius"`
	Mode       *string    `json:"mode"`
	FanLevel   *string    `json:"fanLevel"`
	ObservedAt *time.Time `json:"observedAt"`
}

// Unit is ACUnit plus the UnitSummary fields this module owns.
type Unit struct {
	ID                  uuid.UUID       `json:"id"`
	TenantID            uuid.UUID       `json:"tenantId"`
	Version             int             `json:"version"`
	CreatedAt           time.Time       `json:"createdAt"`
	UpdatedAt           time.Time       `json:"updatedAt"`
	CustomerOrgID       uuid.UUID       `json:"customerOrgId"`
	PropertyID          uuid.UUID       `json:"propertyId"`
	SpaceID             *uuid.UUID      `json:"spaceId"`
	DisplayName         string          `json:"displayName"`
	ModelID             uuid.UUID       `json:"modelId"`
	Type                string          `json:"type"`
	InstalledAt         *time.Time      `json:"installedAt"`
	ServiceScope        []string        `json:"serviceScope"`
	WarrantyEndsAt      *time.Time      `json:"warrantyEndsAt"`
	AlertPolicyIDs      []uuid.UUID     `json:"alertPolicyIds"`
	Archived            bool            `json:"archived"`
	CapabilityVersion   int             `json:"capabilityVersion"`
	Connection          string          `json:"connection"`
	ObservedState       ObservedState   `json:"observedState"`
	ObservedRestriction json.RawMessage `json:"observedRestriction"`
	LastSeenAt          *time.Time      `json:"lastSeenAt"`
	EffectivePowerState string          `json:"effectivePowerState"`
	LatestMeasurements  []any           `json:"latestMeasurements"`
	ActiveAlertCount    int             `json:"activeAlertCount"`
}

// UnitDetail is UnitDetail of service-contracts.ts: the unit summary plus the detail parts other modules own.
type UnitDetail struct {
	Unit
	DetailExtras
}

// DetailExtras are the UnitDetail fields owned by Devices, Control, Restrictions and Maintenance (assembled by app/).
type DetailExtras struct {
	Capabilities           any         `json:"capabilities"`
	EffectiveControlPolicy any         `json:"effectiveControlPolicy"`
	ControlAvailability    any         `json:"controlAvailability"`
	Components             []string    `json:"components"`
	PendingCommands        any         `json:"pendingCommands"`
	PendingCommandIDs      []uuid.UUID `json:"pendingCommandIds"`
	Location               Location    `json:"location"`
}

// Location is UnitDetail.location.
type Location struct {
	PathLabels         []string `json:"pathLabels"`
	Address            *string  `json:"address"`
	AccessInstructions *string  `json:"accessInstructions"`
}

// Details builds the DetailExtras of a unit (wired by app/).
type Details interface {
	Detail(ctx context.Context, c *ops.Call, u Unit) (DetailExtras, error)
}

// Monitoring is what the Assets read model needs from the Monitoring module (wired by app/).
type Monitoring interface {
	// PowerMeasured returns, per unit, whether the latest valid measured power reading is fresh (SR27) and its value.
	PowerMeasured(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID]bool, error)
	ActiveAlertCounts(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID]int, error)
}

// Module holds the dependencies.
type Module struct {
	Mon       Monitoring
	Models    Models
	Policies  PolicyReader
	Usage     []UnitUsage
	Contracts ContractReader
	Claims    ClaimReader
	Import    ImportModels
	Activity  []UnitUsage // telemetry and jobs block import undo
	Store     ops.Runner  // separate transactions for non-business state (import previews)
	Active    []UnitActivity
	OrgActive []OrgActivity
	Details   Details // UnitDetail parts owned by other modules
}

// Register binds the Assets operations implemented so far.
func Register(r *ops.Registry, m *Module) {
	ops.Register(r, "units.list", m.unitsList)
	ops.Register(r, "units.get", m.unitsGet)
	ops.Register(r, "properties.save", m.propertiesSave)
	ops.Register(r, "spaces.save", m.spacesSave)
	ops.Register(r, "units.save", m.unitsSave)
	ops.Register(r, "properties.archive", m.archive("property", "assets.properties",
		`SELECT EXISTS (SELECT 1 FROM assets.units WHERE property_id = $1 AND NOT archived) OR EXISTS (SELECT 1 FROM assets.spaces WHERE property_id = $1 AND NOT archived)`))
	ops.Register(r, "spaces.archive", m.archive("space", "assets.spaces",
		`SELECT EXISTS (SELECT 1 FROM assets.units WHERE space_id = $1 AND NOT archived) OR EXISTS (SELECT 1 FROM assets.spaces WHERE parent_space_id = $1 AND NOT archived)`))
	ops.Register(r, "units.archive", m.unitsArchive)
	ops.Register(r, "customers.list", m.customersList)
	ops.Register(r, "customers.save", m.customersSave)
	ops.Register(r, "properties.list", m.propertiesList)
	ops.Register(r, "spaces.list", m.spacesList)
	ops.Register(r, "locations.rename", m.locationsRename)
	ops.Register(r, "units.delete", m.unitsDelete)
	ops.Register(r, "units.setAlertPolicies", m.unitsSetAlertPolicies)
	ops.Register(r, "units.coverage", m.unitsCoverage)
	ops.Register(r, "units.importPreview", m.unitsImportPreview)
	ops.Register(r, "units.importCommit", m.unitsImportCommit)
	ops.Register(r, "units.importUndo", m.unitsImportUndo)
}

// UnitFilters are the units.list filters of query-catalog.csv.
type UnitFilters struct {
	CustomerID         *uuid.UUID   `json:"customerId,omitempty"`
	PropertyID         *uuid.UUID   `json:"propertyId,omitempty"`
	SpaceID            *uuid.UUID   `json:"spaceId,omitempty"`
	IncludeDescendants *bool        `json:"includeDescendants,omitempty"`
	Status             *string      `json:"status,omitempty"`
	Connections        []string     `json:"connections,omitempty"`
	PowerState         *string      `json:"powerState,omitempty"`
	UnitIDs            *[]uuid.UUID `json:"unitIds,omitempty"`
	OrganizationID     *uuid.UUID   `json:"organizationId,omitempty"`
	UnassignedOnly     *bool        `json:"unassignedOnly,omitempty"`
}

var connections = map[string]bool{"online": true, "offline": true, "unknown": true, "connecting": true, "error": true}

// scopeSQL restricts unit rows (alias u) to the caller's List-mode scope (unitscope, IR169).
func scopeSQL(c *ops.Call, args *[]any) string { return unitscope.SQL(c, args, "u.id", unitscope.List) }

const unitCols = `u.id, u.tenant_id, u.version, u.created_at, u.updated_at, u.customer_org_id, u.property_id, u.space_id, u.display_name,
	u.model_id, u.type, u.installed_at, u.service_scope, u.warranty_ends_at, u.archived, u.capability_version, u.connection,
	u.observed_state, u.observed_restriction, u.last_seen_at,
	COALESCE((SELECT array_agg(policy_id ORDER BY policy_id) FROM assets.unit_alert_policies ap WHERE ap.unit_id = u.id), '{}')`

func (m *Module) query(ctx context.Context, c *ops.Call, where string, args []any, order string, limit, offset int) ([]Unit, int, error) {
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM assets.units u WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}
	q := "SELECT " + unitCols + " FROM assets.units u WHERE " + where + " ORDER BY " + order
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}
	rows, err := c.Tx.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var out []Unit
	for rows.Next() {
		var u Unit
		var state []byte
		if err := rows.Scan(&u.ID, &u.TenantID, &u.Version, &u.CreatedAt, &u.UpdatedAt, &u.CustomerOrgID, &u.PropertyID, &u.SpaceID,
			&u.DisplayName, &u.ModelID, &u.Type, &u.InstalledAt, &u.ServiceScope, &u.WarrantyEndsAt, &u.Archived, &u.CapabilityVersion,
			&u.Connection, &state, &u.ObservedRestriction, &u.LastSeenAt, &u.AlertPolicyIDs); err != nil {
			return nil, 0, err
		}
		if err := json.Unmarshal(state, &u.ObservedState); err != nil {
			return nil, 0, err
		}
		if len(u.ObservedRestriction) == 0 {
			u.ObservedRestriction = json.RawMessage("null")
		}
		u.LatestMeasurements = []any{}
		out = append(out, u)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}
	return out, total, m.enrich(ctx, c, out)
}

// EffectivePower implements SR27: on/off only when online, the observation is not in the future and at most
// 120 s old, and the latest valid measured power reading is fresh; otherwise unknown.
func EffectivePower(u *Unit, now time.Time, measuredFresh, ok bool) string {
	o := u.ObservedState
	if u.Connection != "online" || o.Power == nil || o.ObservedAt == nil || o.ObservedAt.After(now) ||
		now.Sub(*o.ObservedAt) > 120*time.Second || !ok || !measuredFresh {
		return "unknown"
	}
	if *o.Power {
		return "on"
	}
	return "off"
}

func (m *Module) enrich(ctx context.Context, c *ops.Call, units []Unit) error {
	if len(units) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(units))
	for i := range units {
		ids[i] = units[i].ID
	}
	power, err := m.Mon.PowerMeasured(ctx, c, ids)
	if err != nil {
		return err
	}
	counts, err := m.Mon.ActiveAlertCounts(ctx, c, ids)
	if err != nil {
		return err
	}
	for i := range units {
		fresh, ok := power[units[i].ID]
		units[i].EffectivePowerState = EffectivePower(&units[i], c.Now, fresh, ok)
		units[i].ActiveAlertCount = counts[units[i].ID]
	}
	return nil
}

func (m *Module) unitsList(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Unit], error) {
	var f UnitFilters
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&f); err != nil {
			return paging.Page[Unit]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	if f.UnassignedOnly != nil && *f.UnassignedOnly && f.SpaceID != nil {
		return paging.Page[Unit]{}, apperr.Fields(map[string]string{"filters.unassignedOnly": "error.conflictsWithSpaceId"})
	}
	if f.Status != nil && !connections[*f.Status] {
		return paging.Page[Unit]{}, apperr.Fields(map[string]string{"filters.status": "error.invalid"})
	}
	for _, s := range f.Connections {
		if !connections[s] {
			return paging.Page[Unit]{}, apperr.Fields(map[string]string{"filters.connections": "error.invalid"})
		}
	}
	if f.PowerState != nil && *f.PowerState != "on" && *f.PowerState != "off" && *f.PowerState != "unknown" {
		return paging.Page[Unit]{}, apperr.Fields(map[string]string{"filters.powerState": "error.invalid"})
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "u.id", "createdAt": "u.created_at", "updatedAt": "u.updated_at"}, "u.id ASC")
	if err != nil {
		return paging.Page[Unit]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Unit]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"NOT u.archived", scopeSQL(c, &args)}
	if f.CustomerID != nil {
		conds = append(conds, "u.customer_org_id = (SELECT organization_id FROM assets.customers WHERE id = "+add(*f.CustomerID)+")")
	}
	if f.OrganizationID != nil {
		conds = append(conds, "u.customer_org_id = "+add(*f.OrganizationID))
	}
	if f.PropertyID != nil {
		conds = append(conds, "u.property_id = "+add(*f.PropertyID))
	}
	if f.SpaceID != nil {
		if f.IncludeDescendants != nil && *f.IncludeDescendants {
			conds = append(conds, `u.space_id IN (WITH RECURSIVE t AS (SELECT id FROM assets.spaces WHERE id = `+add(*f.SpaceID)+
				` UNION ALL SELECT s.id FROM assets.spaces s JOIN t ON s.parent_space_id = t.id) SELECT id FROM t)`)
		} else {
			conds = append(conds, "u.space_id = "+add(*f.SpaceID))
		}
	}
	if f.UnassignedOnly != nil && *f.UnassignedOnly {
		conds = append(conds, "u.space_id IS NULL")
	}
	if f.Status != nil {
		conds = append(conds, "u.connection = "+add(*f.Status))
	}
	if len(f.Connections) > 0 {
		conds = append(conds, "u.connection = ANY("+add(f.Connections)+")")
	}
	if f.UnitIDs != nil {
		conds = append(conds, "u.id = ANY("+add(*f.UnitIDs)+")")
	}
	where := strings.Join(conds, " AND ")
	if f.PowerState != nil { // derived filter: evaluate on the full scoped set, then page in memory
		all, _, err := m.query(ctx, c, where, args, order, 0, 0)
		if err != nil {
			return paging.Page[Unit]{}, err
		}
		var keep []Unit
		for _, u := range all {
			if u.EffectivePowerState == *f.PowerState {
				keep = append(keep, u)
			}
		}
		end := min(w.Offset+w.Limit, len(keep))
		items := []Unit{}
		if w.Offset < len(keep) {
			items = keep[w.Offset:end]
		}
		return paging.Page[Unit]{Items: items, NextCursor: w.Next(len(keep)), Total: len(keep), SnapshotVersion: w.Snapshot}, nil
	}
	items, total, err := m.query(ctx, c, where, args, order, w.Limit, w.Offset)
	if err != nil {
		return paging.Page[Unit]{}, err
	}
	if items == nil {
		items = []Unit{}
	}
	return paging.Page[Unit]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

// UnitGetInput is units.get input.
type UnitGetInput struct {
	ID    uuid.UUID  `json:"id"`
	JobID *uuid.UUID `json:"jobId,omitempty"`
}

// Validate implements ops.Validator.
func (in *UnitGetInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": "error.required"}
	}
	return nil
}

func (m *Module) unitsGet(ctx context.Context, c *ops.Call, in *UnitGetInput) (UnitDetail, error) {
	var args []any
	args = append(args, in.ID)
	where := "u.id = $1 AND " + scopeSQL(c, &args)
	units, _, err := m.query(ctx, c, where, args, "u.id", 1, 0)
	if err != nil {
		return UnitDetail{}, err
	}
	if len(units) == 0 {
		return UnitDetail{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err := unitscope.Gate(ctx, c, []uuid.UUID{in.ID}); err != nil { // IR49(b): equipment read waits for the work window
		return UnitDetail{}, err
	}
	d := UnitDetail{Unit: units[0]}
	if m.Details != nil {
		if d.DetailExtras, err = m.Details.Detail(ctx, c, units[0]); err != nil {
			return UnitDetail{}, err
		}
	}
	return d, nil
}
