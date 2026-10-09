package server

import (
	"context"
	"encoding/json"
	"math"
	"sync"
	"time"

	"github.com/pradita/ac-project/service/api/internal/platform/events"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/control"
	"github.com/pradita/ac-project/service/api/internal/modules/devices"
	"github.com/pradita/ac-project/service/api/internal/modules/monitoring"
	"github.com/pradita/ac-project/service/api/internal/modules/restrictions"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/db"
	"github.com/pradita/ac-project/service/api/internal/scheduler"
)

// ScenarioClock is the demo scenario clock: democlock.Clock (shared through platform.demo_clock, IR168) when the
// scenario has a start, else the process-local demoClock (tests with a fixed base clock).
type ScenarioClock interface {
	Now() time.Time
	Advance(ctx context.Context, d time.Duration) error
}

// NewScenarioClock is a process-local scenario clock over base; services of one binary can share it
// (Config.DemoClock).
func NewScenarioClock(base func() time.Time) ScenarioClock { return &demoClock{base: base} }

// demoClock is the base clock plus a forward offset set by demo.advanceClock (IR36), local to this process.
type demoClock struct {
	mu     sync.Mutex
	base   func() time.Time
	offset time.Duration
}

func (d *demoClock) Now() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()
	return d.base().Add(d.offset)
}

func (d *demoClock) Advance(_ context.Context, by time.Duration) error {
	d.mu.Lock()
	d.offset += by
	d.mu.Unlock()
	return nil
}

// demoOps serves demo.* in the demo environment only (backend architecture §15, IR154); production returns
// UNAVAILABLE errors.demo_only. demoSession.* is always served by the BFF sign-in, never by the Core API.
type demoOps struct {
	inline  bool // serves every domain (tests, single process)
	enabled bool
	m       *db.TxManager
	clock   ScenarioClock
	reg     *ops.Registry // the modules' scheduler jobs (IR54 schedule automations)
}

func demoOnly() error { return apperr.E(apperr.Unavailable, "errors.demo_only") }

// AdvanceInput is demo.advanceClock input.
type AdvanceInput struct {
	To time.Time `json:"to"`
}

// Validate implements ops.Validator.
func (in *AdvanceInput) Validate() map[string]string {
	if in.To.IsZero() {
		return map[string]string{"to": "error.required"}
	}
	return nil
}

// Generation is DemoGeneration (the API demo never resets data, so the generation stays 1).
type Generation struct {
	Generation int `json:"generation"`
}

// @Summary		demo.advanceClock (write)
// @ID				demo.advanceClock
// @Description	Authorization: public:demo-panel-only
// @Description	Validation: D01; IR36 forward jumps; set initial time only immediately after reset; shift Session expiry
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DDC-07
// @Tags			demo
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		AdvanceInput	true	"input"
// @Success		200				{object}	ops.Envelope{data=Generation}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ops/demo.advanceClock [post]
func (d *demoOps) advance(ctx context.Context, c *ops.Call, in *AdvanceInput) (Generation, error) {
	if !d.enabled {
		return Generation{}, demoOnly()
	}
	if in.To.Before(c.Now) { // only forward jumps in the API demo (IR36)
		return Generation{}, apperr.Fields(map[string]string{"to": "errors.clock_backwards"})
	}
	if err := d.clock.Advance(ctx, in.To.Sub(c.Now)); err != nil {
		return Generation{}, apperr.From(err)
	}
	// process deadlines reached by the jump: in-process when one process serves every domain; split services leave
	// it to the scheduler worker, which runs on the same shared clock (IR168, IR185)
	if !d.inline {
		return Generation{Generation: 1}, nil
	}
	if _, err := scheduler.Tick(ctx, d.m, in.To, d.reg); err != nil {
		return Generation{}, err
	}
	return Generation{Generation: 1}, nil
}

// ResetInput is demo.reset input.
type ResetInput struct {
	Confirmation bool `json:"confirmation"`
}

// Validate implements ops.Validator.
func (in *ResetInput) Validate() map[string]string {
	if !in.Confirmation {
		return map[string]string{"confirmation": "error.required"}
	}
	return nil
}

// @Summary		demo.reset (write)
// @ID				demo.reset
// @Description	Authorization: public:demo-panel-only
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DDC-07
// @Tags			demo
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Success		200				{object}	ops.Envelope{data=Generation}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ops/demo.reset [post]
func (d *demoOps) reset(context.Context, *ops.Call, *ResetInput) (Generation, error) {
	if !d.enabled {
		return Generation{}, demoOnly()
	}
	// the database is rebuilt offline (`make resetdb`); the running API cannot drop shared data
	return Generation{}, apperr.E(apperr.Unavailable, "errors.demo_reset_offline")
}

// TriggerInput is the supported part of DemoTrigger: command_sent/ack/fail, telemetry, restriction_observation,
// device faults and recoveries, and device operation results.
type TriggerInput struct {
	ScenarioID  string          `json:"scenarioId"`
	EventID     uuid.UUID       `json:"eventId"`
	OccurredAt  time.Time       `json:"occurredAt"`
	EventType   string          `json:"eventType"`
	CommandID   *uuid.UUID      `json:"commandId,omitempty"`
	Sequence    *int            `json:"sequence,omitempty"`
	Measurement *RawMeasurement `json:"measurement,omitempty"`
	// restriction_observation (SR26)
	RestrictionID *uuid.UUID                `json:"restrictionId,omitempty"`
	UnitID        *uuid.UUID                `json:"unitId,omitempty"`
	Observed      *restrictions.Observation `json:"observed"`
	// device (SR20): kind communication_lost / power_lost / tamper / restored with recovery
	DeviceID  *uuid.UUID        `json:"deviceId,omitempty"`
	BindingID *uuid.UUID        `json:"bindingId,omitempty"`
	Kind      string            `json:"kind,omitempty"`
	Recovery  *devices.Recovery `json:"recovery,omitempty"`
	// operation (IR67): the device's result for a running check / firmware operation
	OperationID *uuid.UUID `json:"operationId,omitempty"`
	Result      string     `json:"result,omitempty"`
}

// RawMeasurement is RawMeasurement of service-contracts.ts.
type RawMeasurement struct {
	UnitID     uuid.UUID `json:"unitId"`
	SensorID   uuid.UUID `json:"sensorId"`
	Metric     string    `json:"metric"`
	Value      *float64  `json:"value"`
	Unit       string    `json:"unit"`
	ObservedAt time.Time `json:"observedAt"`
	ReceivedAt time.Time `json:"receivedAt"`
	Origin     string    `json:"origin"`
	Quality    string    `json:"quality"`
	Sequence   *int64    `json:"sequence,omitempty"`
	EventID    uuid.UUID `json:"eventId"`
}

// Validate implements ops.Validator.
func (in *TriggerInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.EventID == uuid.Nil {
		fe["eventId"] = "error.required"
	}
	if in.OccurredAt.IsZero() {
		fe["occurredAt"] = "error.required"
	}
	switch in.EventType {
	case "command_sent", "command_ack", "command_fail":
		if in.CommandID == nil || in.Measurement != nil {
			fe["commandId"] = "error.required"
		}
	case "telemetry":
		if in.Measurement == nil || in.CommandID != nil {
			fe["measurement"] = "error.required"
		}
	case "restriction_observation":
		if in.RestrictionID == nil || in.UnitID == nil || in.CommandID != nil || in.Measurement != nil {
			fe["unitId"] = "error.required"
		}
	case "device":
		if in.DeviceID == nil {
			fe["deviceId"] = "error.required"
		}
		if in.Sequence == nil || *in.Sequence < 0 {
			fe["sequence"] = "error.required"
		}
		switch r := in.Recovery; {
		case in.Kind == "restored" && (r == nil || r.SourceEventID == uuid.Nil || devices.RecoveredValue[r.Axis] == "" || devices.RecoveredValue[r.Axis] != r.Value):
			fe["recovery"] = "error.invalid" // DeviceRecovery: connection → online, power → on, tamper → clear
		case in.Kind != "restored" && devices.FaultAxis[in.Kind] == "":
			fe["kind"] = "error.invalid"
		case in.Kind != "restored" && r != nil:
			fe["recovery"] = "error.invalid" // faults carry no recovery (SR20)
		}
	case "operation":
		if in.OperationID == nil {
			fe["operationId"] = "error.required"
		}
		if in.Result != "succeeded" && in.Result != "failed" {
			fe["result"] = "error.invalid"
		}
	default:
		fe["eventType"] = "errors.trigger_unsupported"
	}
	return fe
}

// Event is DemoEvent.
type Event struct {
	EventID    uuid.UUID `json:"eventId"`
	Generation int       `json:"generation"`
	OccurredAt time.Time `json:"occurredAt"`
	Type       string    `json:"type"`
}

// metricUnits is the D07 metric / unit / range table.
var metricUnits = map[string]struct {
	unit     string
	min, max float64
}{"temperature": {"°C", -50, 100}, "humidity": {"%", 0, 100}, "co2": {"ppm", 0, 10000}, "pm25": {"µg/m³", 0, 1000}, "power": {"kW", 0, 100},
	"vibration": {"mm/s", 0, 100}, "refrigerant_pressure": {"kPa", 0, 5000}}

// inTenant runs fn in the tenant that owns the row found by probe (demo triggers are anonymous, RLS needs a tenant).
func (d *demoOps) inTenant(ctx context.Context, now time.Time, probe string, id uuid.UUID, fn func(pgx.Tx, uuid.UUID) error) error {
	rows, err := d.m.Writer.Query(ctx, `SELECT id FROM platform.tenants ORDER BY id`)
	if err != nil {
		return err
	}
	tenants, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return err
	}
	for _, t := range tenants {
		found := false
		err := d.m.Run(ctx, false, &ops.Principal{TenantID: t}, func(tx pgx.Tx) error {
			var ok bool
			if err := tx.QueryRow(ctx, probe, id).Scan(&ok); err != nil || !ok {
				return err
			}
			found = true
			if _, err := tx.Exec(ctx, `SELECT set_config('app.now', $1, true)`, now.UTC().Format(time.RFC3339Nano)); err != nil { // column defaults on the demo clock
				return err
			}
			return fn(tx, t)
		})
		if err != nil || found {
			return err
		}
	}
	return apperr.E(apperr.NotFound, "error.notFound")
}

// @Summary		demo.trigger (write)
// @ID				demo.trigger
// @Description	Authorization: public:demo-panel-only
// @Description	Validation: D01; IR37 includes transport/network injection; IR45 simulator on/off; IR59 payment branch removed; IR77 telemetry sequence optional (latest+1); IR98 allergen/load_alert events; IR213 telemetry causes unit_mismatch → non_finite → out_of_range → invalid_time; IR217 device faults/recoveries (sequence per device/binding/axis; eventId repeat; current binding; tamper Alert) and operation results (running only)
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DDC-07
// @Tags			demo
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		TriggerInput	true	"input"
// @Success		200				{object}	ops.Envelope{data=Event}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ops/demo.trigger [post]
func (d *demoOps) trigger(ctx context.Context, c *ops.Call, in *TriggerInput) (Event, error) {
	if !d.enabled {
		return Event{}, demoOnly()
	}
	out := Event{EventID: in.EventID, Generation: 1, OccurredAt: in.OccurredAt, Type: in.EventType}
	switch in.EventType {
	case "command_sent":
		return out, d.inTenant(ctx, c.Now, `SELECT EXISTS (SELECT 1 FROM control.commands WHERE id = $1)`, *in.CommandID, func(tx pgx.Tx, _ uuid.UUID) error {
			_, err := tx.Exec(ctx, `UPDATE control.commands SET status = 'sent', delivery = 'sent', sent_at = $2, version = version + 1, updated_at = $2 WHERE id = $1 AND status = 'requested'`,
				*in.CommandID, in.OccurredAt)
			return err
		})
	case "command_ack":
		return out, d.inTenant(ctx, c.Now, `SELECT EXISTS (SELECT 1 FROM control.commands WHERE id = $1)`, *in.CommandID, func(tx pgx.Tx, _ uuid.UUID) error {
			_, err := control.Acknowledge(ctx, tx, *in.CommandID, in.OccurredAt)
			return err
		})
	case "command_fail":
		return out, d.inTenant(ctx, c.Now, `SELECT EXISTS (SELECT 1 FROM control.commands WHERE id = $1)`, *in.CommandID, func(tx pgx.Tx, _ uuid.UUID) error {
			tag, err := tx.Exec(ctx, `UPDATE control.commands SET status = 'failed', failure_code = 'DEVICE_REJECTED', version = version + 1, updated_at = $2
				WHERE id = $1 AND status IN ('requested','sent')`, *in.CommandID, in.OccurredAt)
			if err != nil || tag.RowsAffected() == 0 {
				return err
			}
			return control.EndRestrictionCommands(ctx, tx, []uuid.UUID{*in.CommandID}, in.OccurredAt)
		})
	}
	switch in.EventType {
	case "device": // SR20 faults and recoveries, SR23 tamper Alert
		s := devices.Signal{EventID: in.EventID, DeviceID: *in.DeviceID, BindingID: in.BindingID, Sequence: int64(*in.Sequence), Kind: in.Kind, Recovery: in.Recovery, OccurredAt: in.OccurredAt}
		return out, d.inTenant(ctx, c.Now, `SELECT EXISTS (SELECT 1 FROM devices.devices WHERE id = $1)`, *in.DeviceID, func(tx pgx.Tx, _ uuid.UUID) error {
			_, err := devices.RecordSignal(ctx, tx, s, c.Now, monitoring.Alerts{})
			return err
		})
	case "operation": // IR67: the result of a running check / firmware operation
		return out, d.inTenant(ctx, c.Now, `SELECT EXISTS (SELECT 1 FROM devices.device_operations WHERE id = $1)`, *in.OperationID, func(tx pgx.Tx, _ uuid.UUID) error {
			return devices.FinishOperation(ctx, tx, *in.OperationID, in.Result == "succeeded", in.OccurredAt, c.Now)
		})
	}
	if in.EventType == "restriction_observation" { // SR26: a device reports which restriction it enforces
		return out, d.inTenant(ctx, c.Now, `SELECT EXISTS (SELECT 1 FROM assets.units WHERE id = $1)`, *in.UnitID, func(tx pgx.Tx, _ uuid.UUID) error {
			var obsRaw any // the device observation itself (equipment data); restrictions reacts to it
			if in.Observed != nil {
				obsRaw, _ = json.Marshal(in.Observed)
			}
			if _, err := tx.Exec(ctx, `UPDATE assets.units SET observed_restriction = $2, last_seen_at = $3 WHERE id = $1`, *in.UnitID, obsRaw, c.Now); err != nil {
				return err
			}
			raw, _ := json.Marshal(in.Observed) // "null" when the device enforces none
			return events.Publish(ctx, tx, c.Principal.TenantID, "unit", *in.UnitID, events.UnitRestrictionObserved,
				events.Observation{UnitID: *in.UnitID, Observed: raw, EventID: in.EventID, At: c.Now}) // billing updates recovery cases (IR185)
		})
	}
	r := in.Measurement // telemetry: D07 / IR12 normalization, IR77 sequence latest+1, IR11 boundary from the sensor
	return out, d.inTenant(ctx, c.Now, `SELECT EXISTS (SELECT 1 FROM devices.sensors s JOIN devices.devices d ON d.id = s.device_id WHERE s.id = $1)`, r.SensorID, func(tx pgx.Tx, t uuid.UUID) error {
		var metric, unit string
		var boundary *string
		var bound *uuid.UUID
		if err := tx.QueryRow(ctx, `SELECT s.metric, s.unit, s.boundary_id, d.unit_id FROM devices.sensors s JOIN devices.devices d ON d.id = s.device_id WHERE s.id = $1`, r.SensorID).
			Scan(&metric, &unit, &boundary, &bound); err != nil {
			return err
		}
		if metric != r.Metric || bound == nil || *bound != r.UnitID {
			return apperr.Fields(map[string]string{"measurement": "errors.sensor_mismatch"})
		}
		// IR12 causes in priority order unit_mismatch → non_finite → out_of_range → invalid_time; a cause keeps a null
		// input suspect (D07), the value itself is never stored for the first three; otherwise null is missing
		quality, value, reason := r.Quality, r.Value, (*string)(nil)
		spec, known := metricUnits[r.Metric]
		suspect := func(why string, drop bool) {
			quality, reason = "suspect", &why
			if drop {
				value = nil
			}
		}
		switch {
		case !known || r.Unit != spec.unit:
			suspect("unit_mismatch", true)
		case value != nil && (math.IsNaN(*value) || math.IsInf(*value, 0)):
			suspect("non_finite", true)
		case value != nil && (*value < spec.min || *value > spec.max):
			suspect("out_of_range", true)
		case r.ObservedAt.After(r.ReceivedAt) || r.ReceivedAt.After(c.Now):
			suspect("invalid_time", false)
		case value == nil:
			quality = "missing"
		}
		seq := int64(0)
		if r.Sequence != nil {
			seq = *r.Sequence
		} else if err := tx.QueryRow(ctx, `SELECT COALESCE(max(sequence), 0) + 1 FROM monitoring.measurements WHERE sensor_id = $1`, r.SensorID).Scan(&seq); err != nil {
			return err
		}
		var dup bool
		if err := tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM monitoring.measurements WHERE event_id = $1)`, r.EventID).Scan(&dup); err != nil || dup {
			return err
		}
		_, err := tx.Exec(ctx, `INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, raw_unit, origin, quality, quality_reason,
			boundary_id, received_at, event_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
			t, r.UnitID, r.SensorID, r.Metric, r.ObservedAt, seq, value, unit, rawUnit(r.Unit, unit), r.Origin, quality, reason, boundary, r.ReceivedAt, r.EventID)
		return err
	})
}

// rawUnit keeps a received unit that differs from the sensor's (IR12: at most 32 characters as evidence).
func rawUnit(got, want string) *string {
	if got == want {
		return nil
	}
	if r := []rune(got); len(r) > 32 {
		got = string(r[:32])
	}
	return &got
}

// demoSessionViaBFF answers demoSession.*: sign-in, switching and extension are handled by the BFF (OIDC session),
// not by the Core API (IR154).
func demoSessionViaBFF[I any](context.Context, *ops.Call, *I) (struct{}, error) {
	return struct{}{}, apperr.E(apperr.Unavailable, "errors.session_via_bff")
}

type demoSignIn struct {
	DemoActorID string  `json:"demoActorId"`
	ReturnTo    *string `json:"returnTo,omitempty"`
}
type demoSwitch struct {
	DemoMembershipID string `json:"demoMembershipId"`
}

func registerDemo(reg *ops.Registry, d *demoOps) {
	ops.Register(reg, "demo.advanceClock", d.advance)
	ops.Register(reg, "demo.reset", d.reset)
	ops.Register(reg, "demo.trigger", d.trigger)
	ops.Register(reg, "demoSession.signIn", demoSessionSignIn)
	ops.Register(reg, "demoSession.switchMembership", demoSessionSwitch)
	ops.Register(reg, "demoSession.signOut", demoSessionSignOut)
	ops.Register(reg, "demoSession.extend", demoSessionExtend)
}

// The demoSession.* operations (one named handler each for the API description).
//
//	@Summary		demoSession.signIn (write)
//	@ID				demoSession.signIn
//	@Description	Authorization: public:demo-only
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DDC-07
//	@Tags			demoSession
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		demoSignIn	true	"input"
//	@Success		200				{object}	ops.Envelope
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/ops/demoSession.signIn [post]
func demoSessionSignIn(ctx context.Context, c *ops.Call, in *demoSignIn) (struct{}, error) {
	return demoSessionViaBFF(ctx, c, in)
}

// @Summary		demoSession.switchMembership (write)
// @ID				demoSession.switchMembership
// @Description	Authorization: authenticated:demo-account-switch; D09
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DDC-07
// @Tags			demoSession
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		demoSwitch	true	"input"
// @Success		200				{object}	ops.Envelope
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ops/demoSession.switchMembership [post]
func demoSessionSwitch(ctx context.Context, c *ops.Call, in *demoSwitch) (struct{}, error) {
	return demoSessionViaBFF(ctx, c, in)
}

// @Summary		demoSession.signOut (write)
// @ID				demoSession.signOut
// @Description	Authorization: authenticated:own-session
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DDC-07
// @Tags			demoSession
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Success		200				{object}	ops.Envelope
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ops/demoSession.signOut [post]
func demoSessionSignOut(ctx context.Context, c *ops.Call, in *struct{}) (struct{}, error) {
	return demoSessionViaBFF(ctx, c, in)
}

// @Summary		demoSession.extend (write)
// @ID				demoSession.extend
// @Description	Authorization: authenticated:own-session
// @Description	Validation: IR55 valid session only; expiresAt=now+30min; no audit
// @Description	Recovery: UNAUTHENTICATED after expiry: purge and /login
// @Description	Design: DDC-07
// @Tags			demoSession
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Success		200				{object}	ops.Envelope
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/ops/demoSession.extend [post]
func demoSessionExtend(ctx context.Context, c *ops.Call, in *struct{}) (struct{}, error) {
	return demoSessionViaBFF(ctx, c, in)
}
