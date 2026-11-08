// Package assets implements the Assets module (customers, properties, spaces, locations, units).
package assets

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
	"github.com/pradita/ac-project/service/api/internal/platform/unitscope"
)

// ObservedState is ObservedState of service-contracts.ts.
type ObservedState struct {
	Power      *bool      `json:"power"`
	Celsius    *float64   `json:"celsius"`
	Mode       *string    `json:"mode"`
	FanLevel   *string    `json:"fanLevel"`
	ObservedAt *time.Time `json:"observedAt"`
}

// ACUnit is ACUnit of service-contracts.ts: what a unit write answers (IR392).
type ACUnit struct {
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
	ObservedRestriction json.RawMessage `json:"observedRestriction" swaggertype:"object"`
	LastSeenAt          *time.Time      `json:"lastSeenAt"`
}

// Unit is UnitSummary: ACUnit plus the summary fields this module owns.
type Unit struct {
	ACUnit

	EffectivePowerState string `json:"effectivePowerState"`
	LatestMeasurements  []any  `json:"latestMeasurements"`
	ActiveAlertCount    int    `json:"activeAlertCount"`
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
	ActiveAlertCounts(
		ctx context.Context,
		c *ops.Call,
		units []uuid.UUID,
	) (map[uuid.UUID]int, error)
	// LatestMeasurements returns, per unit, the latest Measurement of each metric with read-time staleness (IR213).
	LatestMeasurements(
		ctx context.Context,
		c *ops.Call,
		units []uuid.UUID,
	) (map[uuid.UUID][]any, error)
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
func Register(reg *ops.Registry, module *Module) {
	ops.Register(reg, "units.list", module.unitsList)
	ops.Register(reg, "units.get", module.unitsGet)
	ops.Register(reg, "properties.save", module.propertiesSave)
	ops.Register(reg, "spaces.save", module.spacesSave)
	ops.Register(reg, "units.save", module.unitsSave)
	ops.Register(reg, "properties.archive", module.propertiesArchive)
	ops.Register(reg, "spaces.archive", module.spacesArchive)
	ops.Register(reg, "units.archive", module.unitsArchive)
	ops.Register(reg, "customers.list", module.customersList)
	ops.Register(reg, "customers.save", module.customersSave)
	ops.Register(reg, "customers.delete", module.customersDelete)
	ops.Register(reg, "properties.list", module.propertiesList)
	ops.Register(reg, "spaces.list", module.spacesList)
	ops.Register(reg, "locations.rename", module.locationsRename)
	ops.Register(reg, "units.delete", module.unitsDelete)
	ops.Register(reg, "units.setAlertPolicies", module.unitsSetAlertPolicies)
	ops.Register(reg, "units.coverage", module.unitsCoverage)
	ops.Register(reg, "units.importPreview", module.unitsImportPreview)
	ops.Register(reg, "units.importCommit", module.unitsImportCommit)
	ops.Register(reg, "units.importUndo", module.unitsImportUndo)
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

// validConnection reports whether state is a unit's connection.
func validConnection(state string) bool {
	switch state {
	case connectionOnline, "offline", stateUnknown, "connecting", stateError:
		return true
	}

	return false
}

// scopeSQL restricts unit rows (alias u) to the caller's List-mode scope (unitscope, IR169).
func scopeSQL(
	c *ops.Call,
	args *[]any,
) string {
	return unitscope.SQL(c, args, "u.id", unitscope.List)
}

const unitCols = `u.id, u.tenant_id, u.version, u.created_at, u.updated_at, u.customer_org_id, u.property_id, ` +
	`u.space_id, u.display_name,
	u.model_id, u.type, u.installed_at, u.service_scope, u.warranty_ends_at, u.archived, u.capability_version, ` +
	`u.connection,
	u.observed_state, u.observed_restriction, u.last_seen_at,
	COALESCE((SELECT array_agg(policy_id ORDER BY policy_id) FROM assets.unit_alert_policies ap WHERE ap.unit_id = ` +
	`u.id), '{}')`

func (m *Module) query(
	ctx context.Context,
	call *ops.Call,
	where string,
	args []any,
	order string,
	limit, offset int,
) ([]Unit, int, error) {
	var total int

	err := call.Tx.QueryRow(ctx, "SELECT count(*) FROM assets.units u WHERE "+where, args...).
		Scan(&total)
	if err != nil {
		return nil, 0, apperr.Wrap(err, "select assets.units")
	}

	q := "SELECT " + unitCols + " FROM assets.units u WHERE " + where + " ORDER BY " + order
	if limit > 0 {
		q += fmt.Sprintf(" LIMIT %d OFFSET %d", limit, offset)
	}

	rows, err := call.Tx.Query(ctx, q, args...)
	if err != nil {
		return nil, 0, apperr.Wrap(err, "query (query)")
	}
	defer rows.Close()

	var out []Unit

	for rows.Next() {
		var (
			unit  Unit
			state []byte
		)

		err := rows.Scan(
			&unit.ID,
			&unit.TenantID,
			&unit.Version,
			&unit.CreatedAt,
			&unit.UpdatedAt,
			&unit.CustomerOrgID,
			&unit.PropertyID,
			&unit.SpaceID,
			&unit.DisplayName,
			&unit.ModelID,
			&unit.Type,
			&unit.InstalledAt,
			&unit.ServiceScope,
			&unit.WarrantyEndsAt,
			&unit.Archived,
			&unit.CapabilityVersion,
			&unit.Connection,
			&state,
			&unit.ObservedRestriction,
			&unit.LastSeenAt,
			&unit.AlertPolicyIDs,
		)
		if err != nil {
			return nil, 0, apperr.Wrap(err, "query (query)")
		}

		err = json.Unmarshal(state, &unit.ObservedState)
		if err != nil {
			return nil, 0, apperr.Wrap(err, "json.Unmarshal")
		}

		if len(unit.ObservedRestriction) == 0 {
			unit.ObservedRestriction = json.RawMessage("null")
		}

		unit.LatestMeasurements = []any{}
		out = append(out, unit)
	}

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, 0, apperr.Wrap(rowsErr, "query (query)")
	}

	return out, total, m.enrich(ctx, call, out)
}

// EffectivePower implements SR27: on/off only when online, the observation is not in the future and at most
// 120 s old, and the latest valid measured power reading is fresh; otherwise unknown.
func EffectivePower(u *Unit, now time.Time, measuredFresh, ok bool) string {
	observedState := u.ObservedState
	if u.Connection != connectionOnline || observedState.Power == nil ||
		observedState.ObservedAt == nil ||
		observedState.ObservedAt.After(now) ||
		now.Sub(*observedState.ObservedAt) > 120*time.Second ||
		!ok ||
		!measuredFresh {
		return stateUnknown
	}

	if *observedState.Power {
		return "on"
	}

	return powerOff
}

func (m *Module) enrich(ctx context.Context, call *ops.Call, units []Unit) error {
	if len(units) == 0 {
		return nil
	}

	ids := make([]uuid.UUID, len(units))
	for i := range units {
		ids[i] = units[i].ID
	}

	power, err := m.Mon.PowerMeasured(ctx, call, ids)
	if err != nil {
		return apperr.Wrap(err, "assets.Monitoring.PowerMeasured")
	}

	counts, err := m.Mon.ActiveAlertCounts(ctx, call, ids)
	if err != nil {
		return apperr.Wrap(err, "assets.Monitoring.ActiveAlertCounts")
	}

	readable, err := unitscope.EquipmentReadable(
		ctx,
		call,
		ids,
	) // telemetry is an equipment read (IR49(b)): nothing before the work window
	if err != nil {
		return apperr.Wrap(err, "unitscope.EquipmentReadable")
	}

	latest, err := m.Mon.LatestMeasurements(ctx, call, readable)
	if err != nil {
		return apperr.Wrap(err, "assets.Monitoring.LatestMeasurements")
	}

	for idx := range units {
		fresh, ok := power[units[idx].ID]
		units[idx].EffectivePowerState = EffectivePower(&units[idx], call.Now, fresh, ok)

		units[idx].ActiveAlertCount = counts[units[idx].ID]
		if ms := latest[units[idx].ID]; ms != nil {
			units[idx].LatestMeasurements = ms
		}
	}

	return nil
}

// unitsList serves units.list.
//
//	@Summary		units.list (read)
//	@ID				units.list
//	@Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | \
//	@Description	admin:dashboard.read | admin:asset.read | admin:control.execute:scope-candidate-read-only | \
//	@Description	admin:device.read:scope-candidate-read-only | admin:job.read:scope-candidate-read-only | \
//	@Description	admin:contract.read:scope-candidate-read-only | admin:restriction.read:scope-candidate-read-only | \
//	@Description	admin:automation.policy.read:scope-candidate-read-only | \
//	@Description	admin:alert.policy.read:scope-candidate-read-only | admin:energy.read:scope-candidate-read-only | \
//	@Description	admin:mrv.read:scope-candidate-read-only | admin:offset.read:scope-candidate-read-only
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR62 \
//	@Description	unassignedOnly; IR213 latestMeasurements: latest reading per metric with read-time quality; [] \
//	@Description	before an external technician's work window
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A02, DD-C01, DD-C02, DD-C04, DD-C05, DD-C06, DD-C07, DD-C09, DD-C13, DD-T11, DD-A04, \
//	@Description	DD-A06, DD-A07, DD-A09, DD-A11, DD-A12, DD-A13, DD-A14, DD-A15, DD-A05, DD-C14, DD-C15, DD-C03, \
//	@Description	DD-T12, DD-P01, DD-P03, DD-P07, DD-T01, DD-T13, DD-C08 · Query: filters \
//	@Description	customerId,propertyId,status,spaceId,includeDescendants,connections,powerState,unitIds,\
//	@Description	organizationId,unassignedOnly · sort id,createdAt,updatedAt (default id asc)
//	@Tags			units
//	@Accept			json
//	@Produce		json
//	@Param			cursor				query		string		false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit				query		integer		false	"page size 1–100, default 25"
//	@Param			sort				query		string		false	"field:direction — fields id,createdAt,updatedAt; default id asc"
//	@Param			customerId			query		string		false	"filter → Customer.organizationId = ACUnit.customerOrgId"
//	@Param			propertyId			query		string		false	"filter → propertyId"
//	@Param			status				query		string		false	"filter → connection"
//	@Param			spaceId				query		string		false	"filter"
//	@Param			includeDescendants	query		boolean		false	"filter"
//	@Param			connections			query		[]string	false	"filter (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
//	@Param			powerState			query		string		false	"filter → SR27 effectivePowerState (not observedState.power)"
//	@Param			unitIds				query		[]string	false	"filter → id in unitIds (empty array returns zero) (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
//	@Param			organizationId		query		string		false	"filter → ACUnit.customerOrgId"
//	@Param			unassignedOnly		query		boolean		false	"filter → true => spaceId=null (combined with spaceId is VALIDATION)"
//	@Success		200					{object}	ops.Envelope{data=UnitPage}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/units [get]
func (m *Module) unitsList(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Unit], error) {
	var unitFilters UnitFilters

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		err := dec.Decode(&unitFilters)
		if err != nil {
			return paging.Page[Unit]{}, apperr.Fields(
				map[string]string{"filters": apperr.KeyInvalid},
			)
		}
	}

	err := unitFilters.check()
	if err != nil {
		return paging.Page[Unit]{}, err
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{"id": "u.id", "createdAt": "u.created_at", "updatedAt": "u.updated_at"},
		"u.id ASC",
	)
	if err != nil {
		return paging.Page[Unit]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, unitFilters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Unit]{}, apperr.Wrap(err, "paging.Resolve")
	}

	var args []any

	where := unitFilters.where(call, &args)
	if unitFilters.PowerState != nil {
		return m.unitsByPower(ctx, call, where, args, order, window, *unitFilters.PowerState)
	}

	items, total, err := m.query(ctx, call, where, args, order, window.Limit, window.Offset)
	if err != nil {
		return paging.Page[Unit]{}, err
	}

	if items == nil {
		items = []Unit{}
	}

	return paging.Page[Unit]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// check refuses filters that contradict each other (unassignedOnly with a space) or name a state that is not one.
func (filters UnitFilters) check() error {
	if filters.UnassignedOnly != nil && *filters.UnassignedOnly &&
		filters.SpaceID != nil {
		return apperr.Fields(
			map[string]string{"filters.unassignedOnly": "error.conflictsWithSpaceId"},
		)
	}

	if filters.Status != nil && !validConnection(*filters.Status) {
		return apperr.Fields(
			map[string]string{"filters.status": apperr.KeyInvalid},
		)
	}

	for _, s := range filters.Connections {
		if !validConnection(s) {
			return apperr.Fields(
				map[string]string{"filters.connections": apperr.KeyInvalid},
			)
		}
	}

	if filters.PowerState != nil && *filters.PowerState != "on" &&
		*filters.PowerState != powerOff &&
		*filters.PowerState != stateUnknown {
		return apperr.Fields(
			map[string]string{"filters.powerState": apperr.KeyInvalid},
		)
	}

	return nil
}

// where is the SQL condition of the filters over the units the caller may list (alias u); the arguments it uses are
// added to args.
func (filters UnitFilters) where(call *ops.Call, args *[]any) string {
	add := func(v any) string {
		*args = append(*args, v)

		return fmt.Sprintf("$%d", len(*args))
	}

	conds := []string{unarchivedUnits, scopeSQL(call, args)}
	if filters.CustomerID != nil {
		conds = append(
			conds,
			"u.customer_org_id = (SELECT organization_id FROM assets.customers WHERE id = "+add(
				*filters.CustomerID,
			)+")",
		)
	}

	if filters.OrganizationID != nil {
		conds = append(conds, "u.customer_org_id = "+add(*filters.OrganizationID))
	}

	if filters.PropertyID != nil {
		conds = append(conds, "u.property_id = "+add(*filters.PropertyID))
	}

	if filters.SpaceID != nil {
		if filters.IncludeDescendants != nil && *filters.IncludeDescendants {
			conds = append(
				conds,
				`u.space_id IN (WITH RECURSIVE t AS (SELECT id FROM assets.spaces WHERE id = `+add(
					*filters.SpaceID,
				)+
					` UNION ALL SELECT s.id FROM assets.spaces s JOIN t ON s.parent_space_id = t.id) SELECT id FROM t)`,
			)
		} else {
			conds = append(conds, "u.space_id = "+add(*filters.SpaceID))
		}
	}

	if filters.UnassignedOnly != nil && *filters.UnassignedOnly {
		conds = append(conds, "u.space_id IS NULL")
	}

	if filters.Status != nil {
		conds = append(conds, "u.connection = "+add(*filters.Status))
	}

	if len(filters.Connections) > 0 {
		conds = append(conds, "u.connection = ANY("+add(filters.Connections)+")")
	}

	if filters.UnitIDs != nil {
		conds = append(conds, "u.id = ANY("+add(*filters.UnitIDs)+")")
	}

	return strings.Join(conds, " AND ")
}

// unitsByPower is the page of the units whose derived power state is state: the filter is evaluated on the full
// scoped set, then paged in memory.
func (m *Module) unitsByPower(
	ctx context.Context,
	call *ops.Call,
	where string,
	args []any,
	order string,
	window paging.Window,
	state string,
) (paging.Page[Unit], error) {
	all, _, err := m.query(ctx, call, where, args, order, 0, 0)
	if err != nil {
		return paging.Page[Unit]{}, err
	}

	var keep []Unit

	for _, unit := range all {
		if unit.EffectivePowerState == state {
			keep = append(keep, unit)
		}
	}

	end := min(window.Offset+window.Limit, len(keep))

	items := []Unit{}
	if window.Offset < len(keep) {
		items = keep[window.Offset:end]
	}

	return paging.Page[Unit]{
		Items:           items,
		NextCursor:      window.Next(len(keep)),
		Total:           len(keep),
		SnapshotVersion: window.Snapshot,
	}, nil
}

// UnitGetInput is units.get input.
type UnitGetInput struct {
	ID    uuid.UUID  `json:"id"`
	JobID *uuid.UUID `json:"jobId,omitempty"`
}

// Validate implements ops.Validator.
func (in *UnitGetInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": apperr.KeyRequired}
	}

	return nil
}

// unitsGet serves units.get.
//
//	@Summary		units.get (read)
//	@ID				units.get
//	@Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | \
//	@Description	admin:dashboard.read | admin:asset.read | admin:control.execute:scope-candidate-read-only | \
//	@Description	admin:device.read:scope-candidate-read-only | admin:job.read:scope-candidate-read-only | \
//	@Description	admin:contract.read:scope-candidate-read-only | admin:restriction.read:scope-candidate-read-only | \
//	@Description	admin:automation.policy.read:scope-candidate-read-only | \
//	@Description	admin:alert.policy.read:scope-candidate-read-only | admin:energy.read:scope-candidate-read-only | \
//	@Description	admin:mrv.read:scope-candidate-read-only | admin:offset.read:scope-candidate-read-only
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR76 technician \
//	@Description	before work window: work-not-started state; IR213 latestMeasurements: latest reading per metric with \
//	@Description	read-time quality (stale past the sensor limit)
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A02, DD-A12, DD-C03, DD-C07, DD-P04, DD-T02, DD-T10, DD-C04, DD-C05, DD-T04, DD-T05, \
//	@Description	DD-T06, DD-T08, DD-T09, DD-T11, DD-A04, DD-A09, DD-A11, DD-A05, DD-C14, DD-T12, DD-P05, DD-P10, \
//	@Description	DD-P02, DD-P03, DD-T13, DD-A06, DD-T07
//	@Tags			units
//	@Accept			json
//	@Produce		json
//	@Param			id		path		string	true	"input field id"
//	@Param			jobId	query		string	false	"input field jobId"
//	@Success		200		{object}	ops.Envelope{data=UnitDetail}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/units/{id} [get]
func (m *Module) unitsGet(
	ctx context.Context,
	call *ops.Call,
	input *UnitGetInput,
) (UnitDetail, error) {
	args := []any{input.ID}
	where := "u.id = $1 AND " + scopeSQL(call, &args)

	units, _, err := m.query(ctx, call, where, args, "u.id", 1, 0)
	if err != nil {
		return UnitDetail{}, err
	}

	if len(units) == 0 {
		return UnitDetail{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	gateErr := unitscope.Gate(
		ctx,
		call,
		[]uuid.UUID{input.ID},
	)
	if gateErr != nil { // IR49(b): equipment read waits for the work window
		return UnitDetail{}, apperr.Wrap(gateErr, "unitscope.Gate")
	}

	unitDetail := UnitDetail{
		Unit: units[0],
		DetailExtras: DetailExtras{
			Capabilities:           nil,
			EffectiveControlPolicy: nil,
			ControlAvailability:    nil,
			Components:             nil,
			PendingCommands:        nil,
			PendingCommandIDs:      nil,
			Location: Location{
				PathLabels:         nil,
				Address:            nil,
				AccessInstructions: nil,
			},
		},
	}
	if m.Details != nil {
		unitDetail.DetailExtras, err = m.Details.Detail(ctx, call, units[0])
		if err != nil {
			return UnitDetail{}, apperr.Wrap(err, "assets.Details.Detail")
		}
	}

	return unitDetail, nil
}
