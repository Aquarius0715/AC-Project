package server

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/modules/assets"
	"github.com/pradita/ac-project/service/api/internal/modules/devices"
	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/modules/monitoring"
	"github.com/pradita/ac-project/service/api/internal/modules/restrictions"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
)

// Cross-domain adapters (IR193): what maintenance, billing and restrictions need from equipment, and what billing
// needs from maintenance. Each method delegates to an internal query of the owner — in the caller's transaction when
// this process serves the owner's domain, over HTTP otherwise, directly for the worker — and registerCrossDomain
// binds the owner-side handlers, which call the owning module. Equipment's own modules keep calling assets directly.

// equipmentView is the equipment domain as other domains see it.
type equipmentView struct {
	am     *assets.Module
	alerts monitoring.Alerts
	tel    monitoring.Telemetry
	models devices.Models
}

type unitIn struct {
	UnitID uuid.UUID `json:"unitId"`
}

type unitsIn struct {
	UnitIDs []uuid.UUID `json:"unitIds"`
}

type orgIn struct {
	OrgID uuid.UUID `json:"orgId"`
}

type orgsIn struct {
	OrgIDs []uuid.UUID `json:"orgIds"`
}

type customerIn struct {
	CustomerID uuid.UUID `json:"customerId"`
}

type propertyIn struct {
	PropertyID uuid.UUID `json:"propertyId"`
}

type briefIn struct {
	CustomerID *uuid.UUID `json:"customerId"`
	PropertyID *uuid.UUID `json:"propertyId"`
	SpaceID    *uuid.UUID `json:"spaceId"`
	UnitID     *uuid.UUID `json:"unitId"`
}

type runHoursIn struct {
	UnitID uuid.UUID  `json:"unitId"`
	Since  *time.Time `json:"since"`
}

type unitStateOut struct {
	OrgID    uuid.UUID `json:"orgId"`
	Archived bool      `json:"archived"`
	Found    bool      `json:"found"`
}

type idFound struct {
	ID    uuid.UUID `json:"id"`
	Found bool      `json:"found"`
}

type customerStateOut struct {
	OrgID  uuid.UUID `json:"orgId"`
	Active bool      `json:"active"`
	Found  bool      `json:"found"`
}

type unitServiceOut struct {
	OrgID      uuid.UUID `json:"orgId"`
	PropertyID uuid.UUID `json:"propertyId"`
	Scope      []string  `json:"scope"`
}

type siteOut struct {
	Address *string  `json:"address"`
	Scope   []string `json:"scope"`
}

type targetOut struct {
	Caps  restrictions.UnitCaps `json:"caps"`
	Found bool                  `json:"found"`
}

type deviceOut struct {
	ID          uuid.UUID `json:"id"`
	Connection  string    `json:"connection"`
	PowerSignal string    `json:"powerSignal"`
	Found       bool      `json:"found"`
}

// Internal query names of the equipment domain.
const (
	qUnitState         = "assets.unitState"
	qUnitsOfProperty   = "assets.unitsOfProperty"
	qOrgOfCustomer     = "assets.orgOfCustomer"
	qUnitsOfOrg        = "assets.unitsOfOrg"
	qUnitService       = "assets.unitService"
	qCustomerProfiles  = "assets.customerProfiles"
	qSiteAddress       = "assets.siteAddress"
	qWarrantyEnd       = "assets.warrantyEnd"
	qBriefUnits        = "assets.briefUnits"
	qCustomerOfOrg     = "assets.customerOfOrg"
	qCustomerState     = "assets.customerState"
	qRestrictionTarget = "assets.restrictionTarget"
	qUnitSeverities    = "monitoring.unitSeverities"
	qRunHours          = "monitoring.runHours"
	qBoundDevice       = "devices.boundDevice"
	qOperationBusy     = "devices.operationBusy"
)

func (v equipmentView) UnitState(ctx context.Context, c *ops.Call, unit uuid.UUID) (uuid.UUID, bool, bool, error) {
	r, err := ops.Delegate(ctx, c, qUnitState, unitIn{unit}, v.unitState)
	return r.OrgID, r.Archived, r.Found, err
}

func (v equipmentView) unitState(ctx context.Context, c *ops.Call, in *unitIn) (unitStateOut, error) {
	org, archived, found, err := v.am.UnitState(ctx, c, in.UnitID)
	return unitStateOut{org, archived, found}, err
}

func (v equipmentView) UnitsOfProperty(ctx context.Context, c *ops.Call, property uuid.UUID) ([]uuid.UUID, error) {
	return ops.Delegate(ctx, c, qUnitsOfProperty, propertyIn{property}, v.unitsOfProperty)
}

func (v equipmentView) unitsOfProperty(ctx context.Context, c *ops.Call, in *propertyIn) ([]uuid.UUID, error) {
	return v.am.UnitsOfProperty(ctx, c, in.PropertyID)
}

func (v equipmentView) OrgOfCustomer(ctx context.Context, c *ops.Call, customer uuid.UUID) (uuid.UUID, bool, error) {
	r, err := ops.Delegate(ctx, c, qOrgOfCustomer, customerIn{customer}, v.orgOfCustomer)
	return r.ID, r.Found, err
}

func (v equipmentView) orgOfCustomer(ctx context.Context, c *ops.Call, in *customerIn) (idFound, error) {
	id, found, err := v.am.OrgOfCustomer(ctx, c, in.CustomerID)
	return idFound{id, found}, err
}

func (v equipmentView) UnitsOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) ([]uuid.UUID, error) {
	return ops.Delegate(ctx, c, qUnitsOfOrg, orgIn{org}, v.unitsOfOrg)
}

func (v equipmentView) unitsOfOrg(ctx context.Context, c *ops.Call, in *orgIn) ([]uuid.UUID, error) {
	return v.am.UnitsOfOrg(ctx, c, in.OrgID)
}

func (v equipmentView) UnitService(ctx context.Context, c *ops.Call, unit uuid.UUID) (uuid.UUID, uuid.UUID, []string, error) {
	r, err := ops.Delegate(ctx, c, qUnitService, unitIn{unit}, v.unitService)
	return r.OrgID, r.PropertyID, r.Scope, err
}

func (v equipmentView) unitService(ctx context.Context, c *ops.Call, in *unitIn) (unitServiceOut, error) {
	org, prop, scope, err := v.am.UnitService(ctx, c, in.UnitID)
	return unitServiceOut{org, prop, scope}, err
}

func (v equipmentView) CustomerProfiles(ctx context.Context, c *ops.Call, orgs []uuid.UUID) (map[uuid.UUID][2]string, error) {
	return ops.Delegate(ctx, c, qCustomerProfiles, orgsIn{orgs}, v.customerProfiles)
}

func (v equipmentView) customerProfiles(ctx context.Context, c *ops.Call, in *orgsIn) (map[uuid.UUID][2]string, error) {
	return v.am.CustomerProfiles(ctx, c, in.OrgIDs)
}

func (v equipmentView) SiteAddress(ctx context.Context, c *ops.Call, unit uuid.UUID) (*string, []string, error) {
	r, err := ops.Delegate(ctx, c, qSiteAddress, unitIn{unit}, v.siteAddress)
	return r.Address, r.Scope, err
}

func (v equipmentView) siteAddress(ctx context.Context, c *ops.Call, in *unitIn) (siteOut, error) {
	addr, scope, err := v.am.SiteAddress(ctx, c, in.UnitID)
	return siteOut{addr, scope}, err
}

func (v equipmentView) WarrantyEnd(ctx context.Context, c *ops.Call, unit uuid.UUID) (*time.Time, error) {
	return ops.Delegate(ctx, c, qWarrantyEnd, unitIn{unit}, v.warrantyEnd)
}

func (v equipmentView) warrantyEnd(ctx context.Context, c *ops.Call, in *unitIn) (*time.Time, error) {
	return v.am.WarrantyEnd(ctx, c, in.UnitID)
}

func (v equipmentView) BriefUnits(ctx context.Context, c *ops.Call, customer, property, space, unit *uuid.UUID) ([]ops.UnitBrief, error) {
	return ops.Delegate(ctx, c, qBriefUnits, briefIn{customer, property, space, unit}, v.briefUnits)
}

func (v equipmentView) briefUnits(ctx context.Context, c *ops.Call, in *briefIn) ([]ops.UnitBrief, error) {
	return v.am.BriefUnits(ctx, c, in.CustomerID, in.PropertyID, in.SpaceID, in.UnitID)
}

func (v equipmentView) CustomerOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) (uuid.UUID, bool, error) {
	r, err := ops.Delegate(ctx, c, qCustomerOfOrg, orgIn{org}, v.customerOfOrg)
	return r.ID, r.Found, err
}

func (v equipmentView) customerOfOrg(ctx context.Context, c *ops.Call, in *orgIn) (idFound, error) {
	id, found, err := v.am.CustomerOfOrg(ctx, c, in.OrgID)
	return idFound{id, found}, err
}

func (v equipmentView) CustomerState(ctx context.Context, c *ops.Call, customer uuid.UUID) (uuid.UUID, bool, bool, error) {
	r, err := ops.Delegate(ctx, c, qCustomerState, customerIn{customer}, v.customerState)
	return r.OrgID, r.Active, r.Found, err
}

func (v equipmentView) customerState(ctx context.Context, c *ops.Call, in *customerIn) (customerStateOut, error) {
	org, active, found, err := v.am.CustomerState(ctx, c, in.CustomerID)
	return customerStateOut{org, active, found}, err
}

func (v equipmentView) RestrictionTarget(ctx context.Context, c *ops.Call, unit uuid.UUID) (restrictions.UnitCaps, bool, error) {
	r, err := ops.Delegate(ctx, c, qRestrictionTarget, unitIn{unit}, v.restrictionTarget)
	return r.Caps, r.Found, err
}

func (v equipmentView) restrictionTarget(ctx context.Context, c *ops.Call, in *unitIn) (targetOut, error) {
	caps, found, err := restrictionTargets{v.am}.RestrictionTarget(ctx, c, in.UnitID)
	return targetOut{caps, found}, err
}

func (v equipmentView) UnitSeverities(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID]string, error) {
	return ops.Delegate(ctx, c, qUnitSeverities, unitsIn{units}, v.unitSeverities)
}

func (v equipmentView) unitSeverities(ctx context.Context, c *ops.Call, in *unitsIn) (map[uuid.UUID]string, error) {
	return v.alerts.UnitSeverities(ctx, c, in.UnitIDs)
}

func (v equipmentView) RunHours(ctx context.Context, c *ops.Call, unit uuid.UUID, since *time.Time) (*float64, error) {
	return ops.Delegate(ctx, c, qRunHours, runHoursIn{unit, since}, v.runHours)
}

func (v equipmentView) runHours(ctx context.Context, c *ops.Call, in *runHoursIn) (*float64, error) {
	return v.tel.RunHours(ctx, c, in.UnitID, in.Since)
}

func (v equipmentView) BoundDevice(ctx context.Context, c *ops.Call, unit uuid.UUID) (uuid.UUID, string, string, bool, error) {
	r, err := ops.Delegate(ctx, c, qBoundDevice, unitIn{unit}, v.boundDevice)
	return r.ID, r.Connection, r.PowerSignal, r.Found, err
}

func (v equipmentView) boundDevice(ctx context.Context, c *ops.Call, in *unitIn) (deviceOut, error) {
	id, conn, signal, found, err := v.models.BoundDevice(ctx, c, in.UnitID)
	return deviceOut{id, conn, signal, found}, err
}

func (v equipmentView) OperationBusy(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	return ops.Delegate(ctx, c, qOperationBusy, unitIn{unit}, v.operationBusy)
}

func (v equipmentView) operationBusy(ctx context.Context, c *ops.Call, in *unitIn) (bool, error) {
	return v.models.OperationBusy(ctx, c, in.UnitID)
}

// maintenanceView is the maintenance domain as billing sees it (payouts).
type maintenanceView struct {
	src maintenance.PayoutSource
}

type acceptedIn struct {
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
}

type rateIn struct {
	ContractorOrgID uuid.UUID `json:"contractorOrgId"`
	At              time.Time `json:"at"`
}

type rateOut struct {
	Lines    map[string]int64 `json:"lines"`
	Currency string           `json:"currency"`
	Found    bool             `json:"found"`
}

const (
	qAcceptedJobs = "maintenance.acceptedJobs"
	qRateAt       = "maintenance.rateAt"
)

func (v maintenanceView) AcceptedJobs(ctx context.Context, c *ops.Call, from, to time.Time) ([]maintenance.AcceptedJob, error) {
	return ops.Delegate(ctx, c, qAcceptedJobs, acceptedIn{from, to}, v.acceptedJobs)
}

func (v maintenanceView) acceptedJobs(ctx context.Context, c *ops.Call, in *acceptedIn) ([]maintenance.AcceptedJob, error) {
	return v.src.AcceptedJobs(ctx, c, in.From, in.To)
}

func (v maintenanceView) RateAt(ctx context.Context, c *ops.Call, contractor uuid.UUID, at time.Time) (map[string]int64, string, bool, error) {
	r, err := ops.Delegate(ctx, c, qRateAt, rateIn{contractor, at}, v.rateAt)
	return r.Lines, r.Currency, r.Found, err
}

func (v maintenanceView) rateAt(ctx context.Context, c *ops.Call, in *rateIn) (rateOut, error) {
	lines, currency, found, err := v.src.RateAt(ctx, c, in.ContractorOrgID, in.At)
	return rateOut{lines, currency, found}, err
}

// AddJobNote is a write into maintenance: billing publishes JobNoteRequested and maintenance's consumer adds the note
// and its note.added event (IR193).
func (v maintenanceView) AddJobNote(ctx context.Context, c *ops.Call, job uuid.UUID, message string) error {
	var tenant uuid.UUID
	if err := c.Tx.QueryRow(ctx, `SELECT current_setting('app.tenant_id')::uuid`).Scan(&tenant); err != nil {
		return err
	}
	return events.Publish(ctx, c.Tx, tenant, "job", job, events.JobNoteRequested,
		events.JobNote{JobID: job, AuthorUserID: c.Principal.UserID, Message: message, At: c.Now})
}

// registerCrossDomain binds the owner-side handlers of the adapters above.
func registerCrossDomain(r *ops.Registry, eq equipmentView, mv maintenanceView) {
	e := ops.DomainEquipment
	ops.RegisterQuery(r, e, qUnitState, eq.unitState)
	ops.RegisterQuery(r, e, qUnitsOfProperty, eq.unitsOfProperty)
	ops.RegisterQuery(r, e, qOrgOfCustomer, eq.orgOfCustomer)
	ops.RegisterQuery(r, e, qUnitsOfOrg, eq.unitsOfOrg)
	ops.RegisterQuery(r, e, qUnitService, eq.unitService)
	ops.RegisterQuery(r, e, qCustomerProfiles, eq.customerProfiles)
	ops.RegisterQuery(r, e, qSiteAddress, eq.siteAddress)
	ops.RegisterQuery(r, e, qWarrantyEnd, eq.warrantyEnd)
	ops.RegisterQuery(r, e, qBriefUnits, eq.briefUnits)
	ops.RegisterQuery(r, e, qCustomerOfOrg, eq.customerOfOrg)
	ops.RegisterQuery(r, e, qCustomerState, eq.customerState)
	ops.RegisterQuery(r, e, qRestrictionTarget, eq.restrictionTarget)
	ops.RegisterQuery(r, e, qUnitSeverities, eq.unitSeverities)
	ops.RegisterQuery(r, e, qRunHours, eq.runHours)
	ops.RegisterQuery(r, e, qBoundDevice, eq.boundDevice)
	ops.RegisterQuery(r, e, qOperationBusy, eq.operationBusy)
	ops.RegisterQuery(r, ops.DomainMaintenance, qAcceptedJobs, mv.acceptedJobs)
	ops.RegisterQuery(r, ops.DomainMaintenance, qRateAt, mv.rateAt)
}
