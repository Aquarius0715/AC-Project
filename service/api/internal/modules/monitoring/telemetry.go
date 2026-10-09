package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
	"github.com/pradita/ac-project/service/api/internal/platform/unitscope"
)

// TelemetryUnits is what the telemetry and ventilation operations need from Assets.
type TelemetryUnits interface {
	VisibleUnits(ctx context.Context, c *ops.Call, units []uuid.UUID) ([]uuid.UUID, error)
	UnitsInSpace(ctx context.Context, c *ops.Call, space uuid.UUID) ([]uuid.UUID, error)
	SpaceInfo(ctx context.Context, c *ops.Call, space uuid.UUID) (uuid.UUID, []uuid.UUID, bool, error)
	SpacesOfUnits(ctx context.Context, c *ops.Call, units []uuid.UUID) ([]uuid.UUID, error)
	ScopedUnits(ctx context.Context, c *ops.Call) ([]uuid.UUID, error)
	SpacesOfOrg(ctx context.Context, c *ops.Call, org uuid.UUID) ([]uuid.UUID, error)
}

// Telemetry answers telemetry questions for other modules and serves telemetry.* / ventilation.*.
type Telemetry struct {
	Units   TelemetryUnits
	Sensors SensorLimits
}

// UnitInUse reports whether the unit has received any measurement (blocks CSV import undo, IR111).
func (Telemetry) UnitInUse(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM monitoring.measurements WHERE unit_id = $1)`, unit).Scan(&b)
	return b, err
}

// Measurement is Measurement of service-contracts.ts (IR121 item 1).
type Measurement struct {
	ID            uuid.UUID `json:"id"`
	TenantID      uuid.UUID `json:"tenantId"`
	Version       int       `json:"version"`
	CreatedAt     time.Time `json:"createdAt"`
	UpdatedAt     time.Time `json:"updatedAt"`
	BoundaryID    *string   `json:"boundaryId"`
	QualityReason *string   `json:"qualityReason"`
	RawUnit       *string   `json:"rawUnit"`
	UnitID        uuid.UUID `json:"unitId"`
	SensorID      uuid.UUID `json:"sensorId"`
	Metric        string    `json:"metric"`
	Value         *float64  `json:"value"`
	Unit          string    `json:"unit"`
	ObservedAt    time.Time `json:"observedAt"`
	ReceivedAt    time.Time `json:"receivedAt"`
	Origin        string    `json:"origin"`
	Quality       string    `json:"quality"`
	IsDemo        bool      `json:"isDemo"`
	Sequence      int64     `json:"sequence"`
	EventID       uuid.UUID `json:"eventId"`
}

const measurementCols = `m.event_id, m.tenant_id, m.received_at, m.boundary_id, m.quality_reason, m.raw_unit, m.unit_id, m.sensor_id, m.metric, m.value,
	m.unit, m.observed_at, m.origin, m.quality, m.sequence`

func scanMeasurement(r pgx.Row) (Measurement, error) {
	x := Measurement{Version: 1, IsDemo: true}
	err := r.Scan(&x.ID, &x.TenantID, &x.ReceivedAt, &x.BoundaryID, &x.QualityReason, &x.RawUnit, &x.UnitID, &x.SensorID, &x.Metric, &x.Value,
		&x.Unit, &x.ObservedAt, &x.Origin, &x.Quality, &x.Sequence)
	x.EventID, x.CreatedAt, x.UpdatedAt = x.ID, x.ReceivedAt, x.ReceivedAt
	return x, err
}

var metrics = map[string]bool{"temperature": true, "humidity": true, "co2": true, "pm25": true, "power": true, "vibration": true, "refrigerant_pressure": true,
	"compressor_cycles": true, "airflow_drop": true, "heartbeat_gap": true}

const maxSeriesRange = 35 * 24 * time.Hour

// readableUnits checks that every unit is readable by the caller (NOT_FOUND otherwise, D01).
func (m Telemetry) readableUnits(ctx context.Context, c *ops.Call, units []uuid.UUID) error {
	vis, err := m.Units.VisibleUnits(ctx, c, units)
	if err != nil {
		return err
	}
	if len(vis) != len(units) {
		return apperr.E(apperr.NotFound, "error.notFound")
	}
	return unitscope.Gate(ctx, c, units) // IR49(b): external technicians wait for the work window
}

// ---- telemetry.series ----

// SeriesInput is telemetry.series input.
type SeriesInput struct {
	From     time.Time    `json:"from"`
	To       time.Time    `json:"to"`
	UnitIDs  []uuid.UUID  `json:"unitIds,omitempty"`
	SpaceID  *uuid.UUID   `json:"spaceId,omitempty"`
	Metric   string       `json:"metric"`
	SensorID *uuid.UUID   `json:"sensorId,omitempty"`
	Query    paging.Query `json:"query"`
}

func validRange(from, to time.Time, max time.Duration, fe map[string]string) {
	switch {
	case from.IsZero():
		fe["from"] = "error.required"
	case to.IsZero():
		fe["to"] = "error.required"
	case !from.Before(to):
		fe["to"] = "error.range"
	case max > 0 && to.Sub(from) > max:
		fe["to"] = "error.rangeTooLong"
	}
}

// Validate implements ops.Validator (IR121 item 2).
func (in *SeriesInput) Validate() map[string]string {
	fe := map[string]string{}
	validRange(in.From, in.To, maxSeriesRange, fe)
	if (in.UnitIDs == nil) == (in.SpaceID == nil) {
		fe["unitIds"] = "error.exactlyOneTarget"
	} else if in.UnitIDs != nil && (len(in.UnitIDs) < 1 || len(in.UnitIDs) > 20 || hasDup(in.UnitIDs)) {
		fe["unitIds"] = "error.invalid"
	}
	if !metrics[in.Metric] {
		fe["metric"] = "error.invalid"
	}
	if len(in.Query.Filters) > 0 && string(in.Query.Filters) != "{}" && string(in.Query.Filters) != "null" {
		fe["query.filters"] = "error.invalid"
	}
	return fe
}

// AllergenObservation is AllergenObservation of service-contracts.ts.
type AllergenObservation struct {
	Availability string     `json:"availability"`
	Substance    *string    `json:"substance"`
	Value        *float64   `json:"value"`
	Unit         *string    `json:"unit"`
	SourceLabel  *string    `json:"sourceLabel"`
	ObservedAt   *time.Time `json:"observedAt"`
	EvidenceText *string    `json:"evidenceText"`
}

// AirSeries is Page<Measurement> & {allergenObservation} (the page fields spelled out so the API description can
// read them).
type AirSeries struct {
	Items               []Measurement        `json:"items"`
	NextCursor          *string              `json:"nextCursor"`
	Total               int                  `json:"total"`
	SnapshotVersion     int                  `json:"snapshotVersion"`
	AllergenObservation *AllergenObservation `json:"allergenObservation"`
}

// @Summary		telemetry.series (read)
// @ID				telemetry.series
// @Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | admin:dashboard.read | admin:automation.policy.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR98 allergenObservation source rows
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-C07, DD-T03, DD-A05, DD-A12 · Query: filters none · sort observedAt (default observedAt asc;sensorId asc;id asc)
// @Tags			telemetry
// @Accept			json
// @Produce		json
// @Param			from		query		string		false	"input field from"
// @Param			to			query		string		false	"input field to"
// @Param			unitIds		query		[]string	false	"input field unitIds (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
// @Param			spaceId		query		string		false	"input field spaceId"
// @Param			metric		query		string		false	"input field metric"
// @Param			sensorId	query		string		false	"input field sensorId"
// @Param			cursor		query		string		false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit		query		integer		false	"page size 1–100, default 25"
// @Param			sort		query		string		false	"field:direction — fields observedAt; default observedAt asc;sensorId asc;id asc"
// @Success		200			{object}	ops.Envelope{data=AirSeries}
// @Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504			{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/telemetry/series [get]
func (m Telemetry) series(ctx context.Context, c *ops.Call, in *SeriesInput) (AirSeries, error) {
	units := in.UnitIDs
	if in.SpaceID != nil {
		var err error
		if units, err = m.Units.UnitsInSpace(ctx, c, *in.SpaceID); err != nil {
			return AirSeries{}, err
		}
		if len(units) == 0 {
			return AirSeries{}, apperr.E(apperr.NotFound, "error.notFound")
		}
	} else if err := m.readableUnits(ctx, c, units); err != nil {
		return AirSeries{}, err
	}
	if in.Query.Sort != nil && in.Query.Sort.Field != "observedAt" {
		return AirSeries{}, apperr.Fields(map[string]string{"query.sort.field": "error.invalid"})
	}
	dir := "ASC"
	if in.Query.Sort != nil && in.Query.Sort.Direction == "desc" {
		dir = "DESC"
	}
	w, err := paging.Resolve(in.Query, struct {
		From, To time.Time
		Units    []uuid.UUID
		Metric   string
		Sensor   *uuid.UUID
	}{in.From, in.To, units, in.Metric, in.SensorID}, c.Principal.ScopeVersion, 1)
	if err != nil {
		return AirSeries{}, err
	}
	args := []any{units, in.Metric, in.From, in.To}
	where := "m.unit_id = ANY($1) AND m.metric = $2 AND m.observed_at >= $3 AND m.observed_at < $4"
	if in.SensorID != nil {
		args = append(args, *in.SensorID)
		where += " AND m.sensor_id = $5"
	}
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM monitoring.measurements m WHERE "+where, args...).Scan(&total); err != nil {
		return AirSeries{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM monitoring.measurements m WHERE %s ORDER BY m.observed_at %s, m.sensor_id ASC, m.event_id ASC LIMIT %d OFFSET %d",
		measurementCols, where, dir, w.Limit, w.Offset), args...)
	if err != nil {
		return AirSeries{}, err
	}
	items := []Measurement{}
	for rows.Next() {
		x, err := scanMeasurement(rows)
		if err != nil {
			rows.Close()
			return AirSeries{}, err
		}
		items = append(items, x)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return AirSeries{}, err
	}
	out := AirSeries{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}
	if len(units) == 1 {
		a, err := allergen(ctx, c, units[0])
		if err != nil {
			return out, err
		}
		out.AllergenObservation = &a
	}
	return out, nil
}

// allergen returns the DEC-57 observation of one unit (not_measured without a row; unsupported clears all fields).
func allergen(ctx context.Context, c *ops.Call, unit uuid.UUID) (AllergenObservation, error) {
	var a AllergenObservation
	err := c.Tx.QueryRow(ctx, `SELECT availability, substance, value, unit, source_label, observed_at, evidence_text FROM monitoring.allergen_observations
		WHERE unit_id = $1 ORDER BY observed_at DESC NULLS LAST, created_at DESC, id ASC LIMIT 1`, unit).
		Scan(&a.Availability, &a.Substance, &a.Value, &a.Unit, &a.SourceLabel, &a.ObservedAt, &a.EvidenceText)
	if errors.Is(err, pgx.ErrNoRows) {
		return AllergenObservation{Availability: "not_measured"}, nil
	}
	if a.Availability == "unsupported" {
		a = AllergenObservation{Availability: "unsupported"}
	}
	return a, err
}

// ---- telemetry.summary ----

// SummaryInput is telemetry.summary input.
type SummaryInput struct {
	From    time.Time   `json:"from"`
	To      time.Time   `json:"to"`
	UnitIDs []uuid.UUID `json:"unitIds"`
	Metric  *string     `json:"metric,omitempty"`
}

// Validate implements ops.Validator (IR121 item 3).
func (in *SummaryInput) Validate() map[string]string {
	fe := map[string]string{}
	validRange(in.From, in.To, 0, fe)
	if len(in.UnitIDs) < 1 || len(in.UnitIDs) > 100 || hasDup(in.UnitIDs) {
		fe["unitIds"] = "error.invalid"
	}
	if in.Metric != nil && !metrics[*in.Metric] {
		fe["metric"] = "error.invalid"
	}
	return fe
}

// TelemetrySummary is TelemetrySummary of service-contracts.ts.
type TelemetrySummary struct {
	Measurements []Measurement `json:"measurements"`
	AsOf         time.Time     `json:"asOf"`
	Energy       any           `json:"energy"`
}

// @Summary		telemetry.summary (read)
// @ID				telemetry.summary
// @Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | admin:dashboard.read | admin:automation.policy.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-C01, DD-P04, DD-T03
// @Tags			telemetry
// @Accept			json
// @Produce		json
// @Param			from	query		string		false	"input field from"
// @Param			to		query		string		false	"input field to"
// @Param			unitIds	query		[]string	false	"input field unitIds (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
// @Param			metric	query		string		false	"input field metric"
// @Success		200		{object}	ops.Envelope{data=TelemetrySummary}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/telemetry/summary [get]
func (m Telemetry) summary(ctx context.Context, c *ops.Call, in *SummaryInput) (TelemetrySummary, error) {
	if err := m.readableUnits(ctx, c, in.UnitIDs); err != nil {
		return TelemetrySummary{}, err
	}
	args := []any{in.UnitIDs, in.From, in.To}
	where := "m.unit_id = ANY($1) AND m.observed_at >= $2 AND m.observed_at < $3"
	if in.Metric != nil {
		args = append(args, *in.Metric)
		where += " AND m.metric = $4"
	}
	rows, err := c.Tx.Query(ctx, `SELECT `+measurementCols+` FROM (SELECT DISTINCT ON (m.unit_id, m.metric) m.* FROM monitoring.measurements m WHERE `+where+`
		ORDER BY m.unit_id, m.metric, m.observed_at DESC, m.sequence DESC) m ORDER BY m.unit_id, m.metric`, args...)
	if err != nil {
		return TelemetrySummary{}, err
	}
	defer rows.Close()
	out := TelemetrySummary{Measurements: []Measurement{}, AsOf: c.Now}
	for rows.Next() {
		x, err := scanMeasurement(rows)
		if err != nil {
			return out, err
		}
		out.Measurements = append(out.Measurements, x)
	}
	return out, rows.Err()
}

// ---- ventilation ----

// CO2Reading is VentilationLog.co2AtLog.
type CO2Reading struct {
	Value      float64   `json:"value"`
	Unit       string    `json:"unit"`
	ObservedAt time.Time `json:"observedAt"`
}

// VentilationLog is VentilationLog of service-contracts.ts.
type VentilationLog struct {
	ID                   uuid.UUID   `json:"id"`
	TenantID             uuid.UUID   `json:"tenantId"`
	Version              int         `json:"version"`
	CreatedAt            time.Time   `json:"createdAt"`
	UpdatedAt            time.Time   `json:"updatedAt"`
	SpaceID              uuid.UUID   `json:"spaceId"`
	UnitID               *uuid.UUID  `json:"unitId"`
	Method               string      `json:"method"`
	DurationMinutes      int         `json:"durationMinutes"`
	CO2AtLog             *CO2Reading `json:"co2AtLog"`
	LoggedByMembershipID uuid.UUID   `json:"loggedByMembershipId"`
	LoggedAt             time.Time   `json:"loggedAt"`
}

const ventCols = `v.id, v.tenant_id, v.version, v.created_at, v.updated_at, v.space_id, v.unit_id, v.method, v.duration_minutes, v.co2_at_log,
	v.logged_by_membership_id, v.logged_at`

func scanVent(r pgx.Row) (VentilationLog, error) {
	var v VentilationLog
	err := r.Scan(&v.ID, &v.TenantID, &v.Version, &v.CreatedAt, &v.UpdatedAt, &v.SpaceID, &v.UnitID, &v.Method, &v.DurationMinutes, &v.CO2AtLog,
		&v.LoggedByMembershipID, &v.LoggedAt)
	return v, err
}

var ventMethods = map[string]bool{"window_opened": true, "door_opened": true, "ventilation_fan": true, "other": true}

// VentLogInput is ventilation.log input.
type VentLogInput struct {
	SpaceID         uuid.UUID  `json:"spaceId"`
	UnitID          *uuid.UUID `json:"unitId,omitempty"`
	Method          string     `json:"method"`
	DurationMinutes int        `json:"durationMinutes"`
}

// Validate implements ops.Validator (IR110: 1–240 minutes).
func (in *VentLogInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.SpaceID == uuid.Nil {
		fe["spaceId"] = "error.required"
	}
	if !ventMethods[in.Method] {
		fe["method"] = "error.invalid"
	}
	if in.DurationMinutes < 1 || in.DurationMinutes > 240 {
		fe["durationMinutes"] = "error.range"
	}
	return fe
}

// @Summary		ventilation.log (write)
// @ID				ventilation.log
// @Description	Authorization: client:self
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR110 manual record only; never creates a Command; duration 1–240 minutes; IR213 co2AtLog: latest CO₂ reading when valid at logging time, never an older one
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-C07
// @Tags			ventilation
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		VentLogInput	true	"input"
// @Success		200				{object}	ops.Envelope{data=VentilationLog}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ventilation [post]
func (m Telemetry) log(ctx context.Context, c *ops.Call, in *VentLogInput) (VentilationLog, error) {
	org, spaceUnits, found, err := m.Units.SpaceInfo(ctx, c, in.SpaceID)
	if err != nil {
		return VentilationLog{}, err
	}
	if !found || org != c.Principal.OrgID {
		return VentilationLog{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	src := spaceUnits
	if in.UnitID != nil {
		in := *in.UnitID
		ok := false
		for _, u := range spaceUnits {
			ok = ok || u == in
		}
		if !ok {
			return VentilationLog{}, apperr.Fields(map[string]string{"unitId": "error.unitNotInSpace"})
		}
		src = []uuid.UUID{in}
	}
	co2, err := m.currentCO2(ctx, c, src)
	if err != nil {
		return VentilationLog{}, err
	}
	v, err := scanVent(c.Tx.QueryRow(ctx, `INSERT INTO monitoring.ventilation_logs AS v (tenant_id, space_id, unit_id, method, duration_minutes, co2_at_log,
		logged_by_membership_id, logged_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, $7) RETURNING `+ventCols,
		in.SpaceID, in.UnitID, in.Method, in.DurationMinutes, co2, c.Principal.MembershipID, c.Now))
	if err != nil {
		return v, err
	}
	c.Emit(ops.Event{AggregateType: "ventilation_log", AggregateID: v.ID, Type: "VentilationLogged", Payload: map[string]any{"spaceId": v.SpaceID}})
	c.Audit(ops.AuditEntry{Action: "ventilation.log", TargetKind: "ventilation_log", TargetID: v.ID.String(), NextVersion: &v.Version})
	return v, nil
}

// currentCO2 is co2AtLog (IR110, IR213): among the units' latest CO₂ readings (LatestMeasurements, read-time
// staleness) the most recent one that is valid with a value; null when none is — an older valid reading is never used.
func (m Telemetry) currentCO2(ctx context.Context, c *ops.Call, units []uuid.UUID) (*CO2Reading, error) {
	latest, err := m.LatestMeasurements(ctx, c, units)
	if err != nil {
		return nil, err
	}
	var out *CO2Reading
	for _, u := range units { // units in a stable order: the first unit wins a tie
		for _, x := range latest[u] {
			if x.Metric == "co2" && x.Quality == "valid" && x.Value != nil && (out == nil || x.ObservedAt.After(out.ObservedAt)) {
				out = &CO2Reading{Value: *x.Value, Unit: x.Unit, ObservedAt: x.ObservedAt}
			}
		}
	}
	return out, nil
}

// @Summary		ventilation.list (read)
// @ID				ventilation.list
// @Description	Authorization: client:self | technician:assigned | admin:alert.read
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-C07 · Query: filters spaceId,unitId,from,to · sort id,createdAt (default createdAt desc;id asc)
// @Tags			ventilation
// @Accept			json
// @Produce		json
// @Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit	query		integer	false	"page size 1–100, default 25"
// @Param			sort	query		string	false	"field:direction — fields id,createdAt; default createdAt desc;id asc"
// @Param			spaceId	query		string	false	"filter → spaceId"
// @Param			unitId	query		string	false	"filter → unitId"
// @Param			from	query		string	false	"filter → [from,to) on loggedAt"
// @Param			to		query		string	false	"filter → [from,to) on loggedAt"
// @Success		200		{object}	ops.Envelope{data=VentilationLogPage}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ventilation [get]
func (m Telemetry) list(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[VentilationLog], error) {
	var f struct {
		SpaceID *uuid.UUID `json:"spaceId,omitempty"`
		UnitID  *uuid.UUID `json:"unitId,omitempty"`
		From    *time.Time `json:"from,omitempty"`
		To      *time.Time `json:"to,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if err := dec.Decode(&f); err != nil {
			return paging.Page[VentilationLog]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	if f.From != nil && f.To != nil && !f.From.Before(*f.To) {
		return paging.Page[VentilationLog]{}, apperr.Fields(map[string]string{"filters.to": "error.range"})
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "v.id", "createdAt": "v.created_at"}, "v.created_at DESC, v.id ASC")
	if err != nil {
		return paging.Page[VentilationLog]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[VentilationLog]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	var conds []string
	switch c.Principal.Role {
	case "admin":
	case "client":
		spaces, err := m.Units.SpacesOfOrg(ctx, c, c.Principal.OrgID)
		if err != nil {
			return paging.Page[VentilationLog]{}, err
		}
		conds = append(conds, "v.space_id = ANY("+add(spaces)+")")
	default:
		units, err := m.Units.ScopedUnits(ctx, c)
		if err != nil {
			return paging.Page[VentilationLog]{}, err
		}
		spaces, err := m.Units.SpacesOfUnits(ctx, c, units)
		if err != nil {
			return paging.Page[VentilationLog]{}, err
		}
		conds = append(conds, "(v.unit_id = ANY("+add(units)+") OR v.space_id = ANY("+add(spaces)+"))")
	}
	if f.SpaceID != nil {
		conds = append(conds, "v.space_id = "+add(*f.SpaceID))
	}
	if f.UnitID != nil {
		conds = append(conds, "v.unit_id = "+add(*f.UnitID))
	}
	if f.From != nil {
		conds = append(conds, "v.logged_at >= "+add(*f.From))
	}
	if f.To != nil {
		conds = append(conds, "v.logged_at < "+add(*f.To))
	}
	where := "TRUE"
	if len(conds) > 0 {
		where = strings.Join(conds, " AND ")
	}
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM monitoring.ventilation_logs v WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[VentilationLog]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM monitoring.ventilation_logs v WHERE %s ORDER BY %s LIMIT %d OFFSET %d", ventCols, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[VentilationLog]{}, err
	}
	defer rows.Close()
	items := []VentilationLog{}
	for rows.Next() {
		v, err := scanVent(rows)
		if err != nil {
			return paging.Page[VentilationLog]{}, err
		}
		items = append(items, v)
	}
	return paging.Page[VentilationLog]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, rows.Err()
}

// RegisterTelemetry binds telemetry.* and ventilation.*.
func RegisterTelemetry(r *ops.Registry, m Telemetry) {
	ops.Register(r, "telemetry.series", m.series)
	ops.Register(r, "telemetry.summary", m.summary)
	ops.Register(r, "ventilation.log", m.log)
	ops.Register(r, "ventilation.list", m.list)
}

// SensorLimits returns the D07 stale limit of sensors (Devices); unknown sensors are absent.
type SensorLimits interface {
	StaleAfter(ctx context.Context, c *ops.Call, sensors []uuid.UUID) (map[uuid.UUID]time.Duration, error)
}

// PowerMeasured implements assets.Monitoring (SR27): per unit, whether the latest power reading at or before now —
// chosen by observedAt desc, sequence desc, id asc, never falling back to older rows — is measured, valid, non-null
// and within its sensor's stale limit (D07). Units without a reading are absent; a reading without its sensor is not fresh.
func (m Telemetry) PowerMeasured(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID]bool, error) {
	rows, err := c.Tx.Query(ctx, `SELECT DISTINCT ON (m.unit_id) m.unit_id, m.sensor_id, m.origin, m.quality, m.value, m.observed_at
		FROM monitoring.measurements m WHERE m.unit_id = ANY($1) AND m.metric = 'power' AND m.observed_at <= $2
		ORDER BY m.unit_id, m.observed_at DESC, m.sequence DESC, m.event_id ASC`, units, c.Now)
	if err != nil {
		return nil, err
	}
	type reading struct {
		unit, sensor uuid.UUID
		ok           bool
		at           time.Time
	}
	var rs []reading
	var sensors []uuid.UUID
	for rows.Next() {
		var r reading
		var origin, quality string
		var value *float64
		if err := rows.Scan(&r.unit, &r.sensor, &origin, &quality, &value, &r.at); err != nil {
			rows.Close()
			return nil, err
		}
		r.ok = origin == "measured" && quality == "valid" && value != nil
		rs, sensors = append(rs, r), append(sensors, r.sensor)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	limits, err := m.Sensors.StaleAfter(ctx, c, sensors)
	if err != nil {
		return nil, err
	}
	out := map[uuid.UUID]bool{}
	for _, r := range rs {
		limit, known := limits[r.sensor]
		out[r.unit] = r.ok && known && c.Now.Sub(r.at) <= limit
	}
	return out, nil
}

// metricOrder lists every Metric for the latest-reading lookup (one index probe per unit and metric).
var metricOrder = []string{"airflow_drop", "co2", "compressor_cycles", "heartbeat_gap", "humidity", "pm25", "power", "refrigerant_pressure", "temperature", "vibration"}

// LatestMeasurements implements assets.Monitoring (UnitSummary.latestMeasurements, IR213): per unit, the latest
// Measurement of each metric at or before now — observedAt desc, sequence desc, id asc, every origin and quality, null
// values kept — ordered by metric. A valid reading older than its sensor's stale limit, or whose sensor is unknown, is
// returned with quality=stale (D07: missing→suspect→stale→valid); history in telemetry.series keeps the stored quality.
func (m Telemetry) LatestMeasurements(ctx context.Context, c *ops.Call, units []uuid.UUID) (map[uuid.UUID][]Measurement, error) {
	out := map[uuid.UUID][]Measurement{}
	if len(units) == 0 {
		return out, nil
	}
	rows, err := c.Tx.Query(ctx, `SELECT x.* FROM unnest($1::uuid[]) AS u(id) CROSS JOIN unnest($3::text[]) AS k(metric)
		CROSS JOIN LATERAL (SELECT `+measurementCols+` FROM monitoring.measurements m WHERE m.unit_id = u.id AND m.metric = k.metric AND m.observed_at <= $2
		ORDER BY m.observed_at DESC, m.sequence DESC, m.event_id ASC LIMIT 1) x ORDER BY x.unit_id, x.metric`, units, c.Now, metricOrder)
	if err != nil {
		return nil, err
	}
	var all []Measurement
	var sensors []uuid.UUID
	for rows.Next() {
		x, err := scanMeasurement(rows)
		if err != nil {
			rows.Close()
			return nil, err
		}
		all, sensors = append(all, x), append(sensors, x.SensorID)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}
	limits, err := m.Sensors.StaleAfter(ctx, c, sensors)
	if err != nil {
		return nil, err
	}
	for _, x := range all {
		if limit, known := limits[x.SensorID]; x.Quality == "valid" && (!known || c.Now.Sub(x.ObservedAt) > limit) {
			x.Quality = "stale"
		}
		out[x.UnitID] = append(out[x.UnitID], x)
	}
	return out, nil
}

// RunHours implements IR134 item 1: hours with a valid measured power reading > 0 since `since` (nil = all), each reading
// counting until the next one capped at its sensor's stale limit; the reading in force at `since` counts from `since`, so a
// unit with power readings but none running since then has 0 h (IR238); nil without any power reading.
func (m Telemetry) RunHours(ctx context.Context, c *ops.Call, unit uuid.UUID, since *time.Time) (*float64, error) {
	const cols = `SELECT sensor_id, observed_at, sequence, value, origin, quality FROM monitoring.measurements WHERE unit_id = $1 AND metric = 'power' AND observed_at <= $2`
	q, args := cols, []any{unit, c.Now}
	if since != nil {
		q = `(` + cols + ` AND observed_at <= $3 ORDER BY observed_at DESC, sequence DESC LIMIT 1) UNION ALL (` + cols + ` AND observed_at > $3)`
		args = append(args, *since)
	}
	rows, err := c.Tx.Query(ctx, `SELECT sensor_id, observed_at, value, origin, quality FROM (`+q+`) r ORDER BY observed_at, sequence`, args...)
	if err != nil {
		return nil, err
	}
	type r struct {
		sensor uuid.UUID
		at     time.Time
		on     bool
	}
	var rs []r
	var sensors []uuid.UUID
	for rows.Next() {
		var x r
		var v *float64
		var origin, quality string
		if err := rows.Scan(&x.sensor, &x.at, &v, &origin, &quality); err != nil {
			rows.Close()
			return nil, err
		}
		x.on = origin == "measured" && quality == "valid" && v != nil && *v > 0
		rs, sensors = append(rs, x), append(sensors, x.sensor)
	}
	rows.Close()
	if err := rows.Err(); err != nil || len(rs) == 0 {
		return nil, err
	}
	limits, err := m.Sensors.StaleAfter(ctx, c, sensors)
	if err != nil {
		return nil, err
	}
	var total time.Duration
	for i, x := range rs {
		if !x.on {
			continue
		}
		end := c.Now
		if i+1 < len(rs) {
			end = rs[i+1].at
		}
		if lim, ok := limits[x.sensor]; ok && end.Sub(x.at) > lim {
			end = x.at.Add(lim)
		}
		start := x.at
		if since != nil && start.Before(*since) {
			start = *since
		}
		if end.After(start) {
			total += end.Sub(start)
		}
	}
	h := math.Round(total.Hours()*10) / 10
	return &h, nil
}

// LatestMeasurement returns the most recent measurement of a unit for a metric (highest observedAt, then sequence),
// or nil (voice temperature reads, IR65).
func LatestMeasurement(ctx context.Context, c *ops.Call, unit uuid.UUID, metric string) (*Measurement, error) {
	x, err := scanMeasurement(c.Tx.QueryRow(ctx, `SELECT `+measurementCols+` FROM monitoring.measurements m WHERE m.unit_id = $1 AND m.metric = $2
		ORDER BY m.observed_at DESC, m.sequence DESC LIMIT 1`, unit, metric))
	if errors.Is(err, pgx.ErrNoRows) {
		return nil, nil
	}
	return &x, err
}
