package monitoring

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
	"github.com/pradita/ac-project/service/api/internal/platform/unitscope"
)

// TelemetryUnits is what the telemetry and ventilation operations need from Assets.
type TelemetryUnits interface {
	VisibleUnits(ctx context.Context, c *ops.Call, units []uuid.UUID) ([]uuid.UUID, error)
	UnitsInSpace(ctx context.Context, c *ops.Call, space uuid.UUID) ([]uuid.UUID, error)
	SpaceInfo(
		ctx context.Context,
		c *ops.Call,
		space uuid.UUID,
	) (uuid.UUID, []uuid.UUID, bool, error)
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

	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM monitoring.measurements WHERE unit_id = $1)`, unit).
		Scan(&b)

	return b, apperr.Wrap(err, "select monitoring.measurements")
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

const measurementCols = `m.event_id, m.tenant_id, m.received_at, m.boundary_id, m.quality_reason, m.raw_unit, ` +
	`m.unit_id, m.sensor_id, m.metric, m.value,
	m.unit, m.observed_at, m.origin, m.quality, m.sequence`

func scanMeasurement(row pgx.Row) (Measurement, error) {
	measurement := Measurement{
		Version:       1,
		IsDemo:        true,
		ID:            uuid.UUID{},
		TenantID:      uuid.UUID{},
		CreatedAt:     time.Time{},
		UpdatedAt:     time.Time{},
		BoundaryID:    nil,
		QualityReason: nil,
		RawUnit:       nil,
		UnitID:        uuid.UUID{},
		SensorID:      uuid.UUID{},
		Metric:        "",
		Value:         nil,
		Unit:          "",
		ObservedAt:    time.Time{},
		ReceivedAt:    time.Time{},
		Origin:        "",
		Quality:       "",
		Sequence:      0,
		EventID:       uuid.UUID{},
	}
	err := row.Scan(
		&measurement.ID,
		&measurement.TenantID,
		&measurement.ReceivedAt,
		&measurement.BoundaryID,
		&measurement.QualityReason,
		&measurement.RawUnit,
		&measurement.UnitID,
		&measurement.SensorID,
		&measurement.Metric,
		&measurement.Value,
		&measurement.Unit,
		&measurement.ObservedAt,
		&measurement.Origin,
		&measurement.Quality,
		&measurement.Sequence,
	)
	measurement.EventID = measurement.ID
	measurement.CreatedAt = measurement.ReceivedAt
	measurement.UpdatedAt = measurement.ReceivedAt

	return measurement, apperr.Wrap(err, "scan (scanMeasurement)")
}

// knownMetric reports whether metric is one the platform measures.
func knownMetric(metric string) bool { return slices.Contains(metricOrder(), metric) }

const maxSeriesRange = 35 * 24 * time.Hour

// ---- telemetry.series ----

// SeriesInput is telemetry.series input.
type SeriesInput struct {
	From     time.Time   `json:"from"`
	To       time.Time   `json:"to"`
	UnitIDs  []uuid.UUID `json:"unitIds,omitempty"`
	SpaceID  *uuid.UUID  `json:"spaceId,omitempty"`
	Metric   string      `json:"metric"`
	SensorID *uuid.UUID  `json:"sensorId,omitempty"`
	// BucketMinutes asks for the whole period in averages instead of a page of readings: 5 for a day, 60 for a week
	// (IR605, DEC-90).
	BucketMinutes *int         `json:"bucketMinutes,omitempty"`
	Query         paging.Query `json:"query"`
}

// maxBuckets bounds one bucketed series (a 35-day period in 60-minute buckets is 840).
const maxBuckets = 1000

func validRange(from, until time.Time, longest time.Duration, fieldErrs map[string]string) {
	switch {
	case from.IsZero():
		fieldErrs["from"] = apperr.KeyRequired
	case until.IsZero():
		fieldErrs["to"] = apperr.KeyRequired
	case !from.Before(until):
		fieldErrs["to"] = apperr.KeyRange
	case longest > 0 && until.Sub(from) > longest:
		fieldErrs["to"] = "error.rangeTooLong"
	}
}

// Validate implements ops.Validator (IR121 item 2).
func (in *SeriesInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	validRange(in.From, in.To, maxSeriesRange, fieldErrs)

	if (in.UnitIDs == nil) == (in.SpaceID == nil) {
		fieldErrs["unitIds"] = apperr.KeyExactlyOneTarget
	} else if in.UnitIDs != nil && (len(in.UnitIDs) < 1 || len(in.UnitIDs) > 20 || hasDup(in.UnitIDs)) {
		fieldErrs["unitIds"] = apperr.KeyInvalid
	}

	if !knownMetric(in.Metric) {
		fieldErrs["metric"] = apperr.KeyInvalid
	}

	if len(in.Query.Filters) > 0 && string(in.Query.Filters) != "{}" &&
		string(in.Query.Filters) != "null" {
		fieldErrs["query.filters"] = apperr.KeyInvalid
	}

	if b := in.BucketMinutes; b != nil {
		if *b != 5 && *b != 60 {
			fieldErrs["bucketMinutes"] = apperr.KeyInvalid
		} else if fieldErrs["to"] == "" && in.To.Sub(in.From) > time.Duration(maxBuckets)*time.Duration(*b)*time.Minute {
			fieldErrs["bucketMinutes"] = apperr.KeyRange
		}
	}

	return fieldErrs
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

// SeriesBucket is SeriesBucket of service-contracts.ts: one stretch of a bucketed series — the average of its valid
// readings (null without one) and how many there were.
type SeriesBucket struct {
	Start time.Time `json:"start"`
	Value *float64  `json:"value"`
	Count int       `json:"count"`
}

// AirSeries is Page<Measurement> & {allergenObservation, buckets} (the page fields spelled out so the API description
// can read them).
type AirSeries struct {
	Items               []Measurement        `json:"items"`
	NextCursor          *string              `json:"nextCursor"`
	Total               int                  `json:"total"`
	SnapshotVersion     int                  `json:"snapshotVersion"`
	AllergenObservation *AllergenObservation `json:"allergenObservation"`
	Buckets             []SeriesBucket       `json:"buckets"` // with bucketMinutes only; null otherwise (IR605)
}

// bucketed averages the valid readings of the series between from and to in stretches of size (IR605, DEC-90): a
// stretch without a valid reading stays in the list with no value and a count of 0, so a gap is never joined; stale,
// suspect and missing readings are left out, as the charts leave them out.
func bucketed(
	ctx context.Context,
	call *ops.Call,
	where string,
	args []any,
	from, until time.Time,
	size time.Duration,
) ([]SeriesBucket, error) {
	q := fmt.Sprintf(`SELECT date_bin(make_interval(secs => $%d), m.observed_at, $3) AS b,
		avg(m.value) FILTER (WHERE m.quality = 'valid' AND m.value IS NOT NULL), count(*) FILTER (WHERE m.quality = 'valid'
		AND m.value IS NOT NULL)
		FROM monitoring.measurements m WHERE %s GROUP BY b`, len(args)+1, where)

	rows, err := call.Tx.Query(ctx, q, append(append([]any{}, args...), size.Seconds())...)
	if err != nil {
		return nil, apperr.Wrap(err, "select monitoring.measurements")
	}

	got := map[int64]SeriesBucket{}

	for rows.Next() {
		var seriesBucket SeriesBucket

		err := rows.Scan(&seriesBucket.Start, &seriesBucket.Value, &seriesBucket.Count)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select monitoring.measurements")
		}

		got[seriesBucket.Start.UnixMilli()] = seriesBucket
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select monitoring.measurements")
	}

	out := []SeriesBucket{}

	for when := from; when.Before(until); when = when.Add(size) {
		seriesBucket, ok := got[when.UnixMilli()]
		if !ok {
			seriesBucket = SeriesBucket{Start: when, Value: nil, Count: 0}
		}

		seriesBucket.Start = when.UTC()
		if seriesBucket.Count == 0 {
			seriesBucket.Value = nil
		}

		out = append(out, seriesBucket)
	}

	return out, nil
}

// allergen returns the DEC-57 observation of one unit (not_measured without a row; unsupported clears all fields).
func allergen(ctx context.Context, c *ops.Call, unit uuid.UUID) (AllergenObservation, error) {
	var allergenObservation AllergenObservation

	err := c.Tx.QueryRow(ctx, `SELECT availability, substance, value, unit, source_label, observed_at, evidence_text FROM
		monitoring.allergen_observations
		WHERE unit_id = $1 ORDER BY observed_at DESC NULLS LAST, created_at DESC, id ASC LIMIT 1`, unit).
		Scan(&allergenObservation.Availability, &allergenObservation.Substance, &allergenObservation.Value,
			&allergenObservation.Unit, &allergenObservation.SourceLabel, &allergenObservation.ObservedAt,
			&allergenObservation.EvidenceText)
	if errors.Is(err, pgx.ErrNoRows) {
		return AllergenObservation{
			Availability: "not_measured",
			Substance:    nil,
			Value:        nil,
			Unit:         nil,
			SourceLabel:  nil,
			ObservedAt:   nil,
			EvidenceText: nil,
		}, nil
	}

	if allergenObservation.Availability == "unsupported" {
		allergenObservation = AllergenObservation{
			Availability: "unsupported",
			Substance:    nil,
			Value:        nil,
			Unit:         nil,
			SourceLabel:  nil,
			ObservedAt:   nil,
			EvidenceText: nil,
		}
	}

	return allergenObservation, apperr.Wrap(err, "select monitoring.allergen_observations")
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
	fieldErrs := map[string]string{}
	validRange(in.From, in.To, 0, fieldErrs)

	if len(in.UnitIDs) < 1 || len(in.UnitIDs) > 100 || hasDup(in.UnitIDs) {
		fieldErrs["unitIds"] = apperr.KeyInvalid
	}

	if in.Metric != nil && !knownMetric(*in.Metric) {
		fieldErrs["metric"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// TelemetrySummary is TelemetrySummary of service-contracts.ts.
type TelemetrySummary struct {
	Measurements []Measurement `json:"measurements"`
	AsOf         time.Time     `json:"asOf"`
	Energy       any           `json:"energy"`
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

const ventCols = `v.id, v.tenant_id, v.version, v.created_at, v.updated_at, v.space_id, v.unit_id, v.method, ` +
	`v.duration_minutes, v.co2_at_log,
	v.logged_by_membership_id, v.logged_at`

func scanVent(r pgx.Row) (VentilationLog, error) {
	var ventilationLog VentilationLog

	err := r.Scan(
		&ventilationLog.ID,
		&ventilationLog.TenantID,
		&ventilationLog.Version,
		&ventilationLog.CreatedAt,
		&ventilationLog.UpdatedAt,
		&ventilationLog.SpaceID,
		&ventilationLog.UnitID,
		&ventilationLog.Method,
		&ventilationLog.DurationMinutes,
		&ventilationLog.CO2AtLog,
		&ventilationLog.LoggedByMembershipID,
		&ventilationLog.LoggedAt,
	)

	return ventilationLog, apperr.Wrap(err, "scan (scanVent)")
}

// validVentMethod reports whether method is a way a room is ventilated.
func validVentMethod(method string) bool {
	switch method {
	case "window_opened", "door_opened", "ventilation_fan", "other":
		return true
	}

	return false
}

// VentLogInput is ventilation.log input.
type VentLogInput struct {
	SpaceID         uuid.UUID  `json:"spaceId"`
	UnitID          *uuid.UUID `json:"unitId,omitempty"`
	Method          string     `json:"method"`
	DurationMinutes int        `json:"durationMinutes"`
}

// Validate implements ops.Validator (IR110: 1–240 minutes).
func (in *VentLogInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.SpaceID == uuid.Nil {
		fieldErrs["spaceId"] = apperr.KeyRequired
	}

	if !validVentMethod(in.Method) {
		fieldErrs["method"] = apperr.KeyInvalid
	}

	if in.DurationMinutes < 1 || in.DurationMinutes > 240 {
		fieldErrs["durationMinutes"] = apperr.KeyRange
	}

	return fieldErrs
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
	StaleAfter(
		ctx context.Context,
		c *ops.Call,
		sensors []uuid.UUID,
	) (map[uuid.UUID]time.Duration, error)
}

// PowerMeasured implements assets.Monitoring (SR27): per unit, whether the latest power reading at or before now —
// chosen by observedAt desc, sequence desc, id asc, never falling back to older rows — is measured, valid, non-null and
// within its sensor's stale limit (D07). Units without a reading are absent; a reading without its sensor is not fresh.
func (m Telemetry) PowerMeasured(
	ctx context.Context,
	call *ops.Call,
	units []uuid.UUID,
) (map[uuid.UUID]bool, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT DISTINCT ON (m.unit_id) m.unit_id, m.sensor_id, m.origin, m.quality, m.value, m.observed_at
		FROM monitoring.measurements m WHERE m.unit_id = ANY($1) AND m.metric = 'power' AND m.observed_at <= $2
		ORDER BY m.unit_id, m.observed_at DESC, m.sequence DESC, m.event_id ASC`,
		units,
		call.Now,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select monitoring.measurements")
	}

	type reading struct {
		unit, sensor uuid.UUID
		ok           bool
		at           time.Time
	}

	var (
		readings []reading
		sensors  []uuid.UUID
	)

	for rows.Next() {
		var (
			latest          reading
			origin, quality string
			value           *float64
		)

		err := rows.Scan(&latest.unit, &latest.sensor, &origin, &quality, &value, &latest.at)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select monitoring.measurements")
		}

		latest.ok = origin == "measured" && quality == qualityValid && value != nil
		readings, sensors = append(readings, latest), append(sensors, latest.sensor)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select monitoring.measurements")
	}

	limits, err := m.Sensors.StaleAfter(ctx, call, sensors)
	if err != nil {
		return nil, apperr.Wrap(err, "monitoring.SensorLimits.StaleAfter")
	}

	out := map[uuid.UUID]bool{}

	for _, r := range readings {
		limit, known := limits[r.sensor]
		out[r.unit] = r.ok && known && call.Now.Sub(r.at) <= limit
	}

	return out, nil
}

// metricOrder lists every Metric for the latest-reading lookup (one index probe per unit and metric).
func metricOrder() []string {
	return []string{
		"airflow_drop",
		metricCO2,
		"compressor_cycles",
		"heartbeat_gap",
		metricHumidity,
		metricPM25,
		metricPower,
		metricRefrigerantPressure,
		metricTemperature,
		metricVibration,
	}
}

// LatestMeasurements implements assets.Monitoring (UnitSummary.latestMeasurements, IR213): per unit, the latest
// Measurement of each metric at or before now — observedAt desc, sequence desc, id asc, every origin and quality, null
// values kept — ordered by metric. A valid reading older than its sensor's stale limit, or whose sensor is unknown, is
// returned with quality=stale (D07: missing→suspect→stale→valid); history in telemetry.series keeps the stored quality.
func (m Telemetry) LatestMeasurements(
	ctx context.Context,
	call *ops.Call,
	units []uuid.UUID,
) (map[uuid.UUID][]Measurement, error) {
	out := map[uuid.UUID][]Measurement{}
	if len(units) == 0 {
		return out, nil
	}

	//nolint:unqueryvet // x.* is the row of the lateral subquery, which names measurementCols
	rows, err := call.Tx.Query(
		ctx,
		`SELECT x.* FROM unnest($1::uuid[]) AS u(id) CROSS JOIN unnest($3::text[]) AS k(metric)
		CROSS JOIN LATERAL (SELECT
		`+measurementCols+` FROM monitoring.measurements m WHERE m.unit_id = u.id AND m.metric =
			k.metric AND m.observed_at <= $2
		ORDER BY m.observed_at DESC, m.sequence DESC, m.event_id ASC LIMIT 1) x ORDER BY x.unit_id, x.metric`,
		units,
		call.Now,
		metricOrder(),
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select unnest")
	}

	var (
		all     []Measurement
		sensors []uuid.UUID
	)

	for rows.Next() {
		measurement, err := scanMeasurement(rows)
		if err != nil {
			rows.Close()

			return nil, err
		}

		all, sensors = append(all, measurement), append(sensors, measurement.SensorID)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select unnest")
	}

	limits, err := m.Sensors.StaleAfter(ctx, call, sensors)
	if err != nil {
		return nil, apperr.Wrap(err, "monitoring.SensorLimits.StaleAfter")
	}

	for _, measurement := range all {
		if limit, known := limits[measurement.SensorID]; measurement.Quality == qualityValid &&
			(!known || call.Now.Sub(measurement.ObservedAt) > limit) {
			measurement.Quality = "stale"
		}

		out[measurement.UnitID] = append(out[measurement.UnitID], measurement)
	}

	return out, nil
}

// hourTenths is the step the ventilation hours are rounded to: a tenth of an hour.
const hourTenths = 10

// RunHours implements IR134 item 1: hours with a valid measured power reading > 0 since `since` (nil = all), each
// reading counting until the next one capped at its sensor's stale limit; the reading in force at `since` counts from
// `since`, so a unit with power readings but none running since then has 0 h (IR238); nil without any power reading.
func (m Telemetry) RunHours(
	ctx context.Context,
	call *ops.Call,
	unit uuid.UUID,
	since *time.Time,
) (*float64, error) {
	const cols = `SELECT sensor_id, observed_at, sequence, value, origin, quality FROM monitoring.measurements WHERE
		unit_id = $1 AND metric = 'power' AND observed_at <= $2`

	stmt, args := cols, []any{unit, call.Now}
	if since != nil {
		stmt = `(` + cols + ` AND observed_at <= $3 ORDER BY observed_at DESC, sequence DESC LIMIT 1) UNION ALL
			(` + cols + ` AND observed_at > $3)`

		args = append(args, *since)
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT sensor_id, observed_at, value, origin, quality FROM (`+stmt+`) r ORDER BY observed_at, sequence`,
		args...)
	if err != nil {
		return nil, apperr.Wrap(err, "select sensor_id")
	}

	type r struct {
		sensor uuid.UUID
		at     time.Time
		on     bool
	}

	var (
		readings []r
		sensors  []uuid.UUID
	)

	for rows.Next() {
		var (
			reading         r
			value           *float64
			origin, quality string
		)

		err := rows.Scan(&reading.sensor, &reading.at, &value, &origin, &quality)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select sensor_id")
		}

		reading.on = origin == "measured" && quality == qualityValid && value != nil && *value > 0
		readings, sensors = append(readings, reading), append(sensors, reading.sensor)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil || len(readings) == 0 {
		return nil, apperr.Wrap(rowsErr, "select sensor_id")
	}

	limits, err := m.Sensors.StaleAfter(ctx, call, sensors)
	if err != nil {
		return nil, apperr.Wrap(err, "monitoring.SensorLimits.StaleAfter")
	}

	var total time.Duration

	for idx, reading := range readings {
		if !reading.on {
			continue
		}

		end := call.Now
		if idx+1 < len(readings) {
			end = readings[idx+1].at
		}

		if lim, ok := limits[reading.sensor]; ok && end.Sub(reading.at) > lim {
			end = reading.at.Add(lim)
		}

		start := reading.at
		if since != nil && start.Before(*since) {
			start = *since
		}

		if end.After(start) {
			total += end.Sub(start)
		}
	}

	h := math.Round(total.Hours()*hourTenths) / hourTenths

	return &h, nil
}

// readableUnits checks that every unit is readable by the caller (NOT_FOUND otherwise, D01).
func (m Telemetry) readableUnits(ctx context.Context, call *ops.Call, units []uuid.UUID) error {
	vis, err := m.Units.VisibleUnits(ctx, call, units)
	if err != nil {
		return apperr.Wrap(err, "monitoring.TelemetryUnits.VisibleUnits")
	}

	if len(vis) != len(units) {
		return apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return apperr.Wrap(
		unitscope.Gate(ctx, call, units),
		"unitscope.Gate",
	) // IR49(b): external technicians wait for the work window
}

// series serves telemetry.series.
//
//	@Summary		telemetry.series (read)
//	@ID				telemetry.series
//	@Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | \
//	@Description	admin:dashboard.read | admin:automation.policy.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot; IR98 \
//	@Description	allergenObservation source rows
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-C07, DD-T03, DD-A05, DD-A12, DD-C01 · Query: filters none · sort observedAt (default \
//	@Description	observedAt asc;sensorId asc;id asc)
//	@Tags			telemetry
//	@Accept			json
//	@Produce		json
//	@Param			from			query		string		false	"input field from"
//	@Param			to				query		string		false	"input field to"
//	@Param			unitIds			query		[]string	false	"input field unitIds (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
//	@Param			spaceId			query		string		false	"input field spaceId"
//	@Param			metric			query		string		false	"input field metric"
//	@Param			sensorId		query		string		false	"input field sensorId"
//	@Param			bucketMinutes	query		integer		false	"input field bucketMinutes"
//	@Param			cursor			query		string		false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit			query		integer		false	"page size 1–100, default 25"
//	@Param			sort			query		string		false	"field:direction — fields observedAt; default observedAt asc;sensorId asc;id asc"
//	@Success		200				{object}	ops.Envelope{data=AirSeries}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/telemetry/series [get]
func (m Telemetry) series(
	ctx context.Context,
	call *ops.Call,
	input *SeriesInput,
) (AirSeries, error) {
	units, err := m.seriesUnits(ctx, call, input)
	if err != nil {
		return AirSeries{}, err
	}

	if input.Query.Sort != nil && input.Query.Sort.Field != "observedAt" {
		return AirSeries{}, apperr.Fields(map[string]string{"query.sort.field": apperr.KeyInvalid})
	}

	dir := "ASC"
	if input.Query.Sort != nil && input.Query.Sort.Direction == "desc" {
		dir = "DESC"
	}

	window, err := paging.Resolve(input.Query, struct {
		From, To time.Time
		Units    []uuid.UUID
		Metric   string
		Sensor   *uuid.UUID
	}{input.From, input.To, units, input.Metric, input.SensorID}, call.Principal.ScopeVersion, 1)
	if err != nil {
		return AirSeries{}, apperr.Wrap(err, "paging.Resolve")
	}

	args := []any{units, input.Metric, input.From, input.To}
	where := "m.unit_id = ANY($1) AND m.metric = $2 AND m.observed_at >= $3 AND m.observed_at < $4"

	if input.SensorID != nil {
		args = append(args, *input.SensorID)
		where += " AND m.sensor_id = $5"
	}

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM monitoring.measurements m WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return AirSeries{}, apperr.Wrap(scanErr, "select monitoring.measurements")
	}

	if input.BucketMinutes != nil { // the whole period in averages, every bucket present (IR605)
		buckets, err := bucketed(
			ctx,
			call,
			where,
			args,
			input.From,
			input.To,
			time.Duration(*input.BucketMinutes)*time.Minute,
		)
		if err != nil {
			return AirSeries{}, err
		}

		out := AirSeries{
			Items:           []Measurement{},
			Total:           total,
			SnapshotVersion: window.Snapshot,
			Buckets:         buckets, NextCursor: nil, AllergenObservation: nil,
		}

		return withAllergen(ctx, call, out, units)
	}

	items, err := seriesPage(ctx, call, where, args, dir, window)
	if err != nil {
		return AirSeries{}, err
	}

	out := AirSeries{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot, AllergenObservation: nil, Buckets: nil,
	}

	return withAllergen(ctx, call, out, units)
}

// seriesPage is the window's page of the series' readings in time order (dir), then by sensor and event.
func seriesPage(
	ctx context.Context,
	call *ops.Call,
	where string,
	args []any,
	dir string,
	window paging.Window,
) ([]Measurement, error) {
	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM monitoring.measurements m WHERE %s ORDER BY m.observed_at %s, m.sensor_id ASC, m.event_id ASC "+
				"LIMIT %d OFFSET %d",
			measurementCols,
			where,
			dir,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return nil, apperr.Wrap(err, "select monitoring.measurements")
	}

	items := []Measurement{}

	for rows.Next() {
		measurement, err := scanMeasurement(rows)
		if err != nil {
			rows.Close()

			return nil, err
		}

		items = append(items, measurement)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select monitoring.measurements")
	}

	return items, nil
}

// seriesUnits are the units a series reads: the units in the space, or the units named, which the caller must be able
// to read.
func (m Telemetry) seriesUnits(
	ctx context.Context,
	call *ops.Call,
	input *SeriesInput,
) ([]uuid.UUID, error) {
	units := input.UnitIDs
	if input.SpaceID != nil {
		var err error

		units, err = m.Units.UnitsInSpace(ctx, call, *input.SpaceID)
		if err != nil {
			return nil, apperr.Wrap(err, "monitoring.TelemetryUnits.UnitsInSpace")
		}

		if len(units) == 0 {
			return nil, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}
	} else {
		err := m.readableUnits(ctx, call, units)
		if err != nil {
			return nil, err
		}
	}

	return units, nil
}

// withAllergen adds the unit's latest allergen observation to the series of one unit.
func withAllergen(
	ctx context.Context,
	call *ops.Call,
	out AirSeries,
	units []uuid.UUID,
) (AirSeries, error) {
	if len(units) != 1 {
		return out, nil
	}

	observation, err := allergen(ctx, call, units[0])
	if err != nil {
		return out, err
	}

	out.AllergenObservation = &observation

	return out, nil
}

// summary serves telemetry.summary.
//
//	@Summary		telemetry.summary (read)
//	@ID				telemetry.summary
//	@Description	Authorization: client:self | contractor:accepted-valid-offer | technician:assigned | \
//	@Description	admin:dashboard.read | admin:automation.policy.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-P04, DD-T03
//	@Tags			telemetry
//	@Accept			json
//	@Produce		json
//	@Param			from	query		string		false	"input field from"
//	@Param			to		query		string		false	"input field to"
//	@Param			unitIds	query		[]string	false	"input field unitIds (repeat the parameter or separate values with commas; an empty value is the empty list)"	collectionFormat(multi)
//	@Param			metric	query		string		false	"input field metric"
//	@Success		200		{object}	ops.Envelope{data=TelemetrySummary}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/telemetry/summary [get]
func (m Telemetry) summary(
	ctx context.Context,
	call *ops.Call,
	input *SummaryInput,
) (TelemetrySummary, error) {
	err := m.readableUnits(ctx, call, input.UnitIDs)
	if err != nil {
		return TelemetrySummary{}, err
	}

	args := []any{input.UnitIDs, input.From, input.To}
	where := "m.unit_id = ANY($1) AND m.observed_at >= $2 AND m.observed_at < $3"

	if input.Metric != nil {
		args = append(args, *input.Metric)
		where += " AND m.metric = $4"
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT `+measurementCols+` FROM (SELECT DISTINCT ON (m.unit_id, m.metric) m.* FROM monitoring.measurements m WHERE
			`+where+`
		ORDER BY m.unit_id, m.metric, m.observed_at DESC, m.sequence DESC) m ORDER BY m.unit_id, m.metric`,
		args...)
	if err != nil {
		return TelemetrySummary{}, apperr.Wrap(err, "select monitoring.measurements")
	}
	defer rows.Close()

	out := TelemetrySummary{Measurements: []Measurement{}, AsOf: call.Now, Energy: nil}

	for rows.Next() {
		x, err := scanMeasurement(rows)
		if err != nil {
			return out, err
		}

		out.Measurements = append(out.Measurements, x)
	}

	return out, apperr.Wrap(rows.Err(), "select monitoring.measurements")
}

// log serves ventilation.log.
//
//	@Summary		ventilation.log (write)
//	@ID				ventilation.log
//	@Description	Authorization: client:self
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR110 manual record only; never creates a Command; duration 1–240 minutes; IR213 co2AtLog: latest \
//	@Description	CO₂ reading when valid at logging time, never an older one
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C07
//	@Tags			ventilation
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		VentLogInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=VentilationLog}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/ventilation [post]
func (m Telemetry) log(
	ctx context.Context,
	call *ops.Call,
	input *VentLogInput,
) (VentilationLog, error) {
	org, spaceUnits, found, err := m.Units.SpaceInfo(ctx, call, input.SpaceID)
	if err != nil {
		return VentilationLog{}, apperr.Wrap(err, "monitoring.TelemetryUnits.SpaceInfo")
	}

	if !found || org != call.Principal.OrgID {
		return VentilationLog{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	src := spaceUnits

	if input.UnitID != nil {
		unitID := *input.UnitID

		ok := false
		for _, u := range spaceUnits {
			ok = ok || u == unitID
		}

		if !ok {
			return VentilationLog{}, apperr.Fields(
				map[string]string{keyUnitID: "error.unitNotInSpace"},
			)
		}

		src = []uuid.UUID{unitID}
	}

	co2, err := m.currentCO2(ctx, call, src)
	if err != nil {
		return VentilationLog{}, err
	}

	ventilationLog, err := scanVent(
		call.Tx.QueryRow(
			ctx,
			`INSERT INTO monitoring.ventilation_logs AS v (tenant_id, space_id, unit_id, method, duration_minutes, co2_at_log,
		logged_by_membership_id, logged_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $6, $7)
		RETURNING `+ventCols,
			input.SpaceID,
			input.UnitID,
			input.Method,
			input.DurationMinutes,
			co2,
			call.Principal.MembershipID,
			call.Now,
		),
	)
	if err != nil {
		return ventilationLog, err
	}

	call.Emit(
		ops.Event{
			AggregateType: "ventilation_log",
			AggregateID:   ventilationLog.ID,
			Type:          "VentilationLogged",
			Payload:       map[string]any{"spaceId": ventilationLog.SpaceID},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "ventilation.log",
			TargetKind:      "ventilation_log",
			TargetID:        ventilationLog.ID.String(),
			NextVersion:     &ventilationLog.Version,
			PreviousVersion: nil,
			Reason:          "",
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)

	return ventilationLog, nil
}

// currentCO2 is co2AtLog (IR110, IR213): among the units' latest CO₂ readings (LatestMeasurements, read-time
// staleness) the most recent one that is valid with a value; null when none is — an older valid reading is never used.
func (m Telemetry) currentCO2(
	ctx context.Context,
	c *ops.Call,
	units []uuid.UUID,
) (*CO2Reading, error) {
	latest, err := m.LatestMeasurements(ctx, c, units)
	if err != nil {
		return nil, err
	}

	var out *CO2Reading

	for _, u := range units { // units in a stable order: the first unit wins a tie
		for _, x := range latest[u] {
			if x.Metric == metricCO2 && x.Quality == qualityValid && x.Value != nil &&
				(out == nil || x.ObservedAt.After(out.ObservedAt)) {
				out = &CO2Reading{Value: *x.Value, Unit: x.Unit, ObservedAt: x.ObservedAt}
			}
		}
	}

	return out, nil
}

// list serves ventilation.list.
//
//	@Summary		ventilation.list (read)
//	@ID				ventilation.list
//	@Description	Authorization: client:self | technician:assigned | admin:alert.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-C07 · Query: filters spaceId,unitId,from,to · sort id,createdAt (default createdAt \
//	@Description	desc;id asc)
//	@Tags			ventilation
//	@Accept			json
//	@Produce		json
//	@Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer	false	"page size 1–100, default 25"
//	@Param			sort	query		string	false	"field:direction — fields id,createdAt; default createdAt desc;id asc"
//	@Param			spaceId	query		string	false	"filter → spaceId"
//	@Param			unitId	query		string	false	"filter → unitId"
//	@Param			from	query		string	false	"filter → [from,to) on loggedAt"
//	@Param			to		query		string	false	"filter → [from,to) on loggedAt"
//	@Success		200		{object}	ops.Envelope{data=VentilationLogPage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/ventilation [get]
func (m Telemetry) list(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[VentilationLog], error) {
	var filters ventFilters

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		err := dec.Decode(&filters)
		if err != nil {
			return paging.Page[VentilationLog]{}, apperr.Fields(
				map[string]string{keyFilters: apperr.KeyInvalid},
			)
		}
	}

	if filters.From != nil && filters.To != nil && !filters.From.Before(*filters.To) {
		return paging.Page[VentilationLog]{}, apperr.Fields(
			map[string]string{"filters.to": apperr.KeyRange},
		)
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{"id": "v.id", keyCreatedAt: "v.created_at"},
		"v.created_at DESC, v.id ASC",
	)
	if err != nil {
		return paging.Page[VentilationLog]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filters, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[VentilationLog]{}, apperr.Wrap(err, "paging.Resolve")
	}

	var args []any

	where, err := m.ventWhere(ctx, call, filters, &args)
	if err != nil {
		return paging.Page[VentilationLog]{}, err
	}

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM monitoring.ventilation_logs v WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[VentilationLog]{}, apperr.Wrap(
			scanErr,
			"select monitoring.ventilation_logs",
		)
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM monitoring.ventilation_logs v WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			ventCols,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[VentilationLog]{}, apperr.Wrap(err, "select monitoring.ventilation_logs")
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

	return paging.Page[VentilationLog]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, apperr.Wrap(rows.Err(), "select monitoring.ventilation_logs")
}

// ventFilters are the filters of ventilation.list.
type ventFilters struct {
	SpaceID *uuid.UUID `json:"spaceId,omitempty"`
	UnitID  *uuid.UUID `json:"unitId,omitempty"`
	From    *time.Time `json:"from,omitempty"`
	To      *time.Time `json:"to,omitempty"`
}

// ventWhere is the SQL condition of the filters over the ventilation logs the caller may read (alias v): HQ reads every
// log, a client the logs of its spaces, others the logs of their units and those units' spaces. The arguments it uses
// are added to args.
func (m Telemetry) ventWhere(
	ctx context.Context,
	call *ops.Call,
	filters ventFilters,
	args *[]any,
) (string, error) {
	add := func(v any) string {
		*args = append(*args, v)

		return fmt.Sprintf("$%d", len(*args))
	}

	var conds []string

	switch call.Principal.Role {
	case "admin":
	case authz.RoleClient:
		spaces, err := m.Units.SpacesOfOrg(ctx, call, call.Principal.OrgID)
		if err != nil {
			return "", apperr.Wrap(
				err,
				"monitoring.TelemetryUnits.SpacesOfOrg",
			)
		}

		conds = append(conds, "v.space_id = ANY("+add(spaces)+")")
	default:
		units, err := m.Units.ScopedUnits(ctx, call)
		if err != nil {
			return "", apperr.Wrap(
				err,
				"monitoring.TelemetryUnits.ScopedUnits",
			)
		}

		spaces, err := m.Units.SpacesOfUnits(ctx, call, units)
		if err != nil {
			return "", apperr.Wrap(
				err,
				"monitoring.TelemetryUnits.SpacesOfUnits",
			)
		}

		conds = append(
			conds,
			"(v.unit_id = ANY("+add(units)+") OR v.space_id = ANY("+add(spaces)+"))",
		)
	}

	if filters.SpaceID != nil {
		conds = append(conds, "v.space_id = "+add(*filters.SpaceID))
	}

	if filters.UnitID != nil {
		conds = append(conds, "v.unit_id = "+add(*filters.UnitID))
	}

	if filters.From != nil {
		conds = append(conds, "v.logged_at >= "+add(*filters.From))
	}

	if filters.To != nil {
		conds = append(conds, "v.logged_at < "+add(*filters.To))
	}

	if len(conds) == 0 {
		return "TRUE", nil
	}

	return strings.Join(conds, " AND "), nil
}

// LatestMeasurement returns the most recent measurement of a unit for a metric (highest observedAt, then sequence),
// and whether the unit has one (voice temperature reads, IR65).
func LatestMeasurement(
	ctx context.Context,
	c *ops.Call,
	unit uuid.UUID,
	metric string,
) (Measurement, bool, error) {
	measurement, err := scanMeasurement(
		c.Tx.QueryRow(
			ctx,
			`SELECT `+measurementCols+` FROM monitoring.measurements m WHERE m.unit_id = $1 AND m.metric = $2
		ORDER BY m.observed_at DESC, m.sequence DESC LIMIT 1`,
			unit,
			metric,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return measurement, false, nil
	}

	return measurement, err == nil, err
}
