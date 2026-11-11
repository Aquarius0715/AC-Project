package server

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"sync"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/control"
	"github.com/pradita/ac-project/service/api/internal/modules/devices"
	"github.com/pradita/ac-project/service/api/internal/modules/monitoring"
	"github.com/pradita/ac-project/service/api/internal/modules/restrictions"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/db"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
	"github.com/pradita/ac-project/service/api/internal/scheduler"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// ScenarioClock is the demo scenario clock: democlock.Clock (shared through platform.demo_clock, IR168) when the
// scenario has a start, else the process-local LocalClock (tests with a fixed base clock).
type ScenarioClock interface {
	Now() time.Time
	Advance(ctx context.Context, d time.Duration) error
}

// LocalClock is the base clock plus a forward offset set by demo.advanceClock (IR36), local to this process.
type LocalClock struct {
	mu     sync.Mutex
	base   func() time.Time
	offset time.Duration
}

// NewScenarioClock is a process-local scenario clock over base; services of one binary can share it
// (Config.DemoClock).
func NewScenarioClock(base func() time.Time) *LocalClock {
	return &LocalClock{base: base, mu: sync.Mutex{}, offset: 0}
}

// Now is the base clock plus the offset.
func (d *LocalClock) Now() time.Time {
	d.mu.Lock()
	defer d.mu.Unlock()

	return d.base().Add(d.offset)
}

// Advance moves the clock forward by by.
func (d *LocalClock) Advance(_ context.Context, by time.Duration) error {
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
	// notify evaluates a gateway's facts against the alert policies (control.Automations.Notify, IR529)
	notify func(context.Context, *ops.Call, *control.EvaluationInput) ([]json.RawMessage, error)
}

func demoOnly() error { return apperr.E(apperr.Unavailable, "errors.demo_only") }

// demoCaller admits a demo.* call from a signed-in session (the demo panel, through the BFF) or from the service
// token's caller (the IoT bridge); an anonymous one is UNAUTHENTICATED (IR768, B-30).
func demoCaller(ctx context.Context, call *ops.Call) error {
	if call.Principal == nil && !ops.InternalCaller(ctx) {
		return apperr.E(apperr.Unauthenticated, apperr.KeyUnauthenticated)
	}

	return nil
}

// AdvanceInput is demo.advanceClock input.
type AdvanceInput struct {
	To time.Time `json:"to"`
}

// Validate implements ops.Validator.
func (in *AdvanceInput) Validate() map[string]string {
	if in.To.IsZero() {
		return map[string]string{"to": apperr.KeyRequired}
	}

	return nil
}

// Generation is DemoGeneration (the API demo never resets data, so the generation stays 1).
type Generation struct {
	Generation int `json:"generation"`
}

// advance serves demo.advanceClock.
//
//	@Summary		demo.advanceClock (write)
//	@ID				demo.advanceClock
//	@Description	Authorization: public:demo-panel-only
//	@Description	Validation: D01; IR36 forward jumps; set initial time only immediately after reset; shift Session \
//	@Description	expiry; IR768 a signed-in session or the service token only (anonymous UNAUTHENTICATED)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DDC-07
//	@Tags			demo
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		AdvanceInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Generation}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/demo/advance-clock [post]
func (d *demoOps) advance(
	ctx context.Context,
	call *ops.Call,
	input *AdvanceInput,
) (Generation, error) {
	callerErr := demoCaller(ctx, call)
	if callerErr != nil {
		return Generation{}, callerErr
	}

	if !d.enabled {
		return Generation{}, demoOnly()
	}

	if input.To.Before(call.Now) { // only forward jumps in the API demo (IR36)
		return Generation{}, apperr.Fields(map[string]string{"to": "errors.clock_backwards"})
	}

	err := d.clock.Advance(ctx, input.To.Sub(call.Now))
	if err != nil {
		return Generation{}, apperr.From(err)
	}
	// process deadlines reached by the jump: in-process when one process serves every domain; split services leave
	// it to the scheduler worker, which runs on the same shared clock (IR168, IR185)
	if !d.inline {
		return Generation{Generation: 1}, nil
	}

	_, err = scheduler.Tick(ctx, d.m, input.To, d.reg)
	if err != nil {
		return Generation{}, apperr.Wrap(err, "scheduler.Tick")
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
		return map[string]string{"confirmation": apperr.KeyRequired}
	}

	return nil
}

// reset serves demo.reset.
//
//	@Summary		demo.reset (write)
//	@ID				demo.reset
//	@Description	Authorization: public:demo-panel-only
//	@Description	Validation: D01; input constraints in the corresponding DD; IR768 a signed-in session or the service \
//	@Description	token only (anonymous UNAUTHENTICATED)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DDC-07
//	@Tags			demo
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		ResetInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Generation}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/demo/reset [post]
func (d *demoOps) reset(ctx context.Context, call *ops.Call, _ *ResetInput) (Generation, error) {
	callerErr := demoCaller(ctx, call)
	if callerErr != nil {
		return Generation{}, callerErr
	}

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
	// state: the device reports its air conditioner's settings with each heartbeat (deviceId, state)
	State *ReportedState `json:"state,omitempty"`
	// automation: the facts a device gateway reports, evaluated against the alert policies only (IR529)
	Event *control.EvaluationInput `json:"event,omitempty"`
	// operation (IR67): the device's result for a running check / firmware operation
	OperationID *uuid.UUID `json:"operationId,omitempty"`
	Result      string     `json:"result,omitempty"`
	// The other event types of DemoTrigger (qualification_revoked, network, simulator, allergen, load_alert,
	// session_expired, microphone_denied, transport) are the browser demo's. Their fields are read here only so that a
	// trigger shaped by the contract is refused as errors.trigger_unsupported, not as malformed input (IR391).
	MembershipID      *uuid.UUID `json:"membershipId,omitempty"`
	Qualification     *string    `json:"qualification,omitempty"`
	Connected         *bool      `json:"connected,omitempty"`
	Enabled           *bool      `json:"enabled,omitempty"`
	Observation       any        `json:"observation,omitempty"`
	CauseCode         *string    `json:"causeCode,omitempty"`
	EvidenceKind      *string    `json:"evidenceKind,omitempty"`
	EvidenceText      *string    `json:"evidenceText,omitempty"`
	Severity          *string    `json:"severity,omitempty"`
	Operation         *string    `json:"operation,omitempty"`
	Outcome           *string    `json:"outcome,omitempty"`
	RetryAfterSeconds *int       `json:"retryAfterSeconds,omitempty"`
	DelayMs           *int       `json:"delayMs,omitempty"`
	RemainingCalls    *int       `json:"remainingCalls,omitempty"`
}

// ReportedState is the state a device reports for its air conditioner (DemoTrigger state): ObservedState without
// observedAt, which is the trigger's occurredAt.
type ReportedState struct {
	Power    *bool    `json:"power"`
	Celsius  *float64 `json:"celsius"`
	Mode     *string  `json:"mode"`
	FanLevel *string  `json:"fanLevel"`
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
	fieldErrs := map[string]string{}
	if in.EventID == uuid.Nil {
		fieldErrs["eventId"] = apperr.KeyRequired
	}

	if in.OccurredAt.IsZero() {
		fieldErrs["occurredAt"] = apperr.KeyRequired
	}

	command, measurement := in.CommandID != nil, in.Measurement != nil

	switch in.EventType {
	case eventCommandSent, eventCommandAck, eventCommandFail:
		if !command || measurement {
			fieldErrs["commandId"] = apperr.KeyRequired
		}
	case "telemetry":
		if !measurement || command {
			fieldErrs["measurement"] = apperr.KeyRequired
		}
	case eventRestrictionObservation:
		if in.RestrictionID == nil || in.UnitID == nil || command || measurement {
			fieldErrs["unitId"] = apperr.KeyRequired
		}
	case "device":
		in.validateDevice(fieldErrs)
	case keyState:
		in.validateState(fieldErrs)
	case "automation":
		in.validateAutomation(fieldErrs)
	case "operation":
		if in.OperationID == nil {
			fieldErrs["operationId"] = apperr.KeyRequired
		}

		if in.Result != "succeeded" && in.Result != "failed" {
			fieldErrs["result"] = apperr.KeyInvalid
		}
	default:
		fieldErrs["eventType"] = "errors.trigger_unsupported"
	}

	return fieldErrs
}

// validateDevice checks a device fault or recovery (SR20): a fault names its axis by its kind and carries no recovery;
// a recovery (DeviceRecovery: connection → online, power → on, tamper → clear) names its source event and the value
// its axis recovers to.
func (in *TriggerInput) validateDevice(fieldErrs map[string]string) {
	if in.DeviceID == nil {
		fieldErrs["deviceId"] = apperr.KeyRequired
	}

	if in.Sequence == nil || *in.Sequence < 0 {
		fieldErrs["sequence"] = apperr.KeyRequired
	}

	switch {
	case in.Kind == kindRestored:
		recovery := in.Recovery
		if recovery == nil || recovery.SourceEventID == uuid.Nil ||
			devices.RecoveredValue(recovery.Axis) == "" ||
			devices.RecoveredValue(recovery.Axis) != recovery.Value {
			fieldErrs["recovery"] = apperr.KeyInvalid
		}
	case devices.FaultAxis(in.Kind) == "":
		fieldErrs["kind"] = apperr.KeyInvalid
	case in.Recovery != nil:
		fieldErrs["recovery"] = apperr.KeyInvalid
	}
}

// reportedCelsiusMin and reportedCelsiusMax bound the temperature setting a device may report.
const (
	reportedCelsiusMin = 10
	reportedCelsiusMax = 40
)

// validateState checks a device's state report: its device, and settings of the air conditioner's modes, fan levels
// and temperature range.
func (in *TriggerInput) validateState(fieldErrs map[string]string) {
	if in.DeviceID == nil {
		fieldErrs["deviceId"] = apperr.KeyRequired
	}

	reported := in.State

	switch {
	case reported == nil:
		fieldErrs[keyState] = apperr.KeyRequired
	case reported.Mode != nil && !slices.Contains([]string{"cool", "dry", "fan"}, *reported.Mode):
		fieldErrs["state.mode"] = apperr.KeyInvalid
	case reported.FanLevel != nil && !slices.Contains([]string{"low", "mid", "high"}, *reported.FanLevel):
		fieldErrs["state.fanLevel"] = apperr.KeyInvalid
	case reported.Celsius != nil && (*reported.Celsius < reportedCelsiusMin || *reported.Celsius > reportedCelsiusMax):
		fieldErrs["state.celsius"] = apperr.KeyRange
	}
}

// validateAutomation checks a gateway's facts: a condition evaluation and nothing else. Schedules come from the clock
// (IR54), never from a gateway.
func (in *TriggerInput) validateAutomation(fieldErrs map[string]string) {
	switch evaluationInput := in.Event; {
	case evaluationInput == nil || in.CommandID != nil || in.Measurement != nil:
		fieldErrs["event"] = apperr.KeyRequired
	case evaluationInput.Phase != "condition":
		fieldErrs["event.phase"] = apperr.KeyInvalid
	default:
		for k, v := range evaluationInput.Validate() {
			fieldErrs["event."+k] = v
		}
	}
}

// Event is DemoEvent.
type Event struct {
	EventID    uuid.UUID `json:"eventId"`
	Generation int       `json:"generation"`
	OccurredAt time.Time `json:"occurredAt"`
	Type       string    `json:"type"`
}

// metricRange is the D07 unit and value range of a metric.
type metricRange struct {
	unit     string
	min, max float64
}

// metricUnits is the D07 metric / unit / range table: unit, minimum, maximum.
func metricUnits() map[string]metricRange {
	return map[string]metricRange{
		"temperature":          {"°C", -50, 100},
		"humidity":             {"%", 0, 100},
		"co2":                  {"ppm", 0, 10000},
		"pm25":                 {"µg/m³", 0, 1000},
		"power":                {"kW", 0, 100},
		"vibration":            {"mm/s", 0, 100},
		"refrigerant_pressure": {"kPa", 0, 5000},
	}
}

// inTenant runs fn in the tenant that owns the row found by probe (RLS needs a tenant). The service (the IoT bridge,
// IR768) probes every tenant; a signed-in caller only its own, so a session acts on its own tenant's rows alone.
func (d *demoOps) inTenant(
	ctx context.Context,
	call *ops.Call,
	probe string,
	probeID uuid.UUID,
	callback func(pgx.Tx, uuid.UUID) error,
) error {
	now := call.Now

	tenants := []uuid.UUID{}
	if call.Principal != nil {
		tenants = append(tenants, call.Principal.TenantID)
	} else {
		rows, err := d.m.Writer.Query(ctx, `SELECT id FROM platform.tenants ORDER BY id`)
		if err != nil {
			return apperr.Wrap(err, "select platform.tenants")
		}

		tenants, err = pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return apperr.Wrap(err, "select platform.tenants")
		}
	}

	for _, tenant := range tenants {
		found := false

		err := d.m.Run(ctx, false, ops.TenantPrincipal(tenant), func(txn pgx.Tx) error {
			var ok bool

			err := txn.QueryRow(ctx, probe, probeID).Scan(&ok)
			if err != nil || !ok {
				return apperr.Wrap(err, "queryrow (inTenant)")
			}

			found = true

			_, err = txn.Exec(
				ctx,
				`SELECT set_config('app.now', $1, true)`,
				now.UTC().Format(time.RFC3339Nano),
			)
			if err != nil { // column defaults on the demo clock
				return apperr.Wrap(err, "select set_config")
			}

			return callback(txn, tenant)
		})
		if err != nil || found {
			return apperr.Wrap(err, "db.TxManager.Run")
		}
	}

	return apperr.E(apperr.NotFound, apperr.KeyNotFound)
}

// trigger serves demo.trigger.
//
//	@Summary		demo.trigger (write)
//	@ID				demo.trigger
//	@Description	Authorization: public:demo-panel-only
//	@Description	Validation: D01; IR37 includes transport/network injection; IR45 simulator on/off; IR59 payment \
//	@Description	branch removed; IR77 telemetry sequence optional (latest+1); IR98 allergen/load_alert events; IR213 \
//	@Description	telemetry causes unit_mismatch → non_finite → out_of_range → invalid_time; IR217 device \
//	@Description	faults/recoveries (sequence per device/binding/axis; eventId repeat; current binding; tamper Alert) \
//	@Description	and operation results (running only); IR528 a state report with no settings moves last seen only; \
//	@Description	IR529 automation evaluates the event's facts (phase condition) against the alert policies only, \
//	@Description	deciding no automation
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DDC-07
//	@Tags			demo
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		TriggerInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Event}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/demo/trigger [post]
func (d *demoOps) trigger(ctx context.Context, call *ops.Call, input *TriggerInput) (Event, error) {
	callerErr := demoCaller(ctx, call)
	if callerErr != nil {
		return Event{}, callerErr
	}

	if !d.enabled {
		return Event{}, demoOnly()
	}

	out := Event{
		EventID:    input.EventID,
		Generation: 1,
		OccurredAt: input.OccurredAt,
		Type:       input.EventType,
	}

	var err error

	switch input.EventType {
	case eventCommandSent:
		err = d.commandSent(ctx, call, input)
	case eventCommandAck:
		err = d.commandAcknowledged(ctx, call, input)
	case eventCommandFail:
		err = d.commandFailed(ctx, call, input)
	case "device": // SR20 faults and recoveries, SR23 tamper Alert
		err = d.deviceSignal(ctx, call, input)
	case "operation": // IR67: the result of a running check / firmware operation
		err = d.operationResult(ctx, call, input)
	case "automation": // a gateway's facts: the alert policies of the units, never an automation (IR529)
		err = d.gatewayFacts(ctx, call, input)
	case keyState: // the device's state report: the unit's observed settings and when the device was last heard
		err = d.stateReport(ctx, call, input)
	case eventRestrictionObservation: // SR26: a device reports which restriction it enforces
		err = d.restrictionObserved(ctx, call, input)
	default: // telemetry: D07 / IR12 normalization, IR77 sequence latest+1, IR11 boundary from the sensor
		err = d.telemetry(ctx, call, input)
	}

	return out, err
}

// commandProbe finds the tenant of a command.
const commandProbe = `SELECT EXISTS (SELECT 1 FROM control.commands WHERE id = $1)`

// commandOfDevice refuses a command result that names another device than the command's (IR768, B-30): a caller
// holding one command's ID cannot settle it in another device's name. Only the service (the bridge) may leave the
// device out and have the result taken as the command's own; a signed-in caller must name it.
func commandOfDevice(ctx context.Context, txn pgx.Tx, input *TriggerInput, internal bool) error {
	if input.DeviceID == nil {
		if internal {
			return nil
		}

		return apperr.Fields(map[string]string{"deviceId": apperr.KeyRequired})
	}

	var device *uuid.UUID

	err := txn.QueryRow(ctx, `SELECT device_id FROM control.commands WHERE id = $1`, *input.CommandID).
		Scan(&device)
	if err != nil {
		return apperr.Wrap(err, "select control.commands")
	}

	if device == nil || *device != *input.DeviceID {
		return apperr.Fields(map[string]string{"deviceId": "errors.command_of_other_device"})
	}

	return nil
}

// commandSent marks a requested command sent.
func (d *demoOps) commandSent(ctx context.Context, call *ops.Call, input *TriggerInput) error {
	return d.inTenant(
		ctx,
		call,
		commandProbe,
		*input.CommandID,
		func(txn pgx.Tx, _ uuid.UUID) error {
			_, err := txn.Exec(
				ctx,
				`UPDATE control.commands SET status = 'sent', delivery = 'sent', sent_at = $2, version = version + 1, updated_at =
				$2 WHERE id = $1 AND status = 'requested'`,
				*input.CommandID,
				input.OccurredAt,
			)

			return apperr.Wrap(err, "update control.commands")
		},
	)
}

// commandAcknowledged records the device's acknowledgement of a command.
func (d *demoOps) commandAcknowledged(
	ctx context.Context,
	call *ops.Call,
	input *TriggerInput,
) error {
	return d.inTenant(
		ctx,
		call,
		commandProbe,
		*input.CommandID,
		func(txn pgx.Tx, _ uuid.UUID) error {
			deviceErr := commandOfDevice(ctx, txn, input, ops.InternalCaller(ctx))
			if deviceErr != nil {
				return deviceErr
			}

			_, err := control.Acknowledge(ctx, txn, *input.CommandID, input.OccurredAt)

			return apperr.Wrap(err, "control.Acknowledge")
		},
	)
}

// commandFailed fails a requested or sent command as the device rejected it, audits the result, fails the test run it
// started or ended and ends the restriction commands it belonged to.
func (d *demoOps) commandFailed(ctx context.Context, call *ops.Call, input *TriggerInput) error {
	return d.inTenant(
		ctx,
		call,
		commandProbe,
		*input.CommandID,
		func(txn pgx.Tx, _ uuid.UUID) error {
			deviceErr := commandOfDevice(ctx, txn, input, ops.InternalCaller(ctx))
			if deviceErr != nil {
				return deviceErr
			}

			tag, err := txn.Exec(
				ctx,
				`UPDATE control.commands SET status = 'failed', failure_code = 'DEVICE_REJECTED', version = version + 1, updated_at
				= $2
		WHERE id = $1 AND status IN ('requested','sent')`,
				*input.CommandID,
				input.OccurredAt,
			)
			if err != nil || tag.RowsAffected() == 0 {
				return apperr.Wrap(err, "update control.commands")
			}

			auditResultErr := control.AuditResult(
				ctx,
				txn,
				*input.CommandID,
				"system-iot",
				"failed",
				"DEVICE_REJECTED",
				input.OccurredAt,
			)
			if auditResultErr != nil {
				return apperr.Wrap(auditResultErr, "control.AuditResult")
			}

			runErr := control.RunCommandFailed(
				ctx,
				txn,
				*input.CommandID,
				"DEVICE_REJECTED",
				input.OccurredAt,
			)
			if runErr != nil {
				return apperr.Wrap(runErr, "control.RunCommandFailed")
			}

			return control.EndRestrictionCommands(
				ctx,
				txn,
				[]uuid.UUID{*input.CommandID},
				input.OccurredAt,
			)
		},
	)
}

// deviceSignal records a device fault or recovery (SR20); a tamper raises an Alert (SR23).
func (d *demoOps) deviceSignal(ctx context.Context, call *ops.Call, input *TriggerInput) error {
	signal := devices.Signal{
		EventID:    input.EventID,
		DeviceID:   *input.DeviceID,
		BindingID:  input.BindingID,
		Sequence:   int64(*input.Sequence),
		Kind:       input.Kind,
		Recovery:   input.Recovery,
		OccurredAt: input.OccurredAt,
	}

	return d.inTenant(
		ctx,
		call,
		`SELECT EXISTS (SELECT 1 FROM devices.devices WHERE id = $1)`,
		*input.DeviceID,
		func(txn pgx.Tx, _ uuid.UUID) error {
			_, err := devices.RecordSignal(
				ctx,
				txn,
				signal,
				call.Now,
				monitoring.Alerts{Units: nil, Access: nil, Devices: nil},
			)

			return apperr.Wrap(err, "devices.RecordSignal")
		},
	)
}

// operationResult finishes a running device operation (IR67) with the device's result.
func (d *demoOps) operationResult(ctx context.Context, call *ops.Call, input *TriggerInput) error {
	return d.inTenant(
		ctx,
		call,
		`SELECT EXISTS (SELECT 1 FROM devices.device_operations WHERE id = $1)`,
		*input.OperationID,
		func(txn pgx.Tx, tenant uuid.UUID) error {
			succeeded := input.Result == "succeeded"

			err := devices.FinishOperation(
				ctx,
				txn,
				*input.OperationID,
				succeeded,
				input.OccurredAt,
				call.Now,
			)
			if err != nil || succeeded {
				return apperr.Wrap(err, "devices.FinishOperation")
			}
			// the device reported the failure: the operation's creator and HQ hear of it (IR95, IR753)
			return devices.NotifyOperationFailed(ctx, &ops.Call{
				Tx:              txn,
				Now:             call.Now,
				CorrelationID:   uuid.NewString(),
				Queries:         d.reg,
				Principal:       ops.SystemPrincipal(tenant, seed.ID("system-demo")),
				Bearer:          "",
				ExpectedVersion: nil,
				Candidates:      nil,
				Events:          nil,
				Audits:          nil,
			}, *input.OperationID)
		},
	)
}

// gatewayFacts evaluates a gateway's facts against the alert policies of its units, never an automation (IR529), and
// records the alerts' and notifications' events and audits as a scheduler job does.
func (d *demoOps) gatewayFacts(ctx context.Context, call *ops.Call, input *TriggerInput) error {
	evaluationInput := input.Event
	if evaluationInput.OccurredAt.After(call.Now) {
		return apperr.Fields(map[string]string{"event.occurredAt": apperr.KeyRange})
	}

	if d.notify == nil {
		return apperr.E(apperr.Unavailable, "errors.demo_only")
	}

	return d.inTenant(
		ctx,
		call,
		`SELECT EXISTS (SELECT 1 FROM assets.units WHERE id = $1)`,
		evaluationInput.UnitIDs[0],
		func(txn pgx.Tx, tenant uuid.UUID) error {
			systemCall := &ops.Call{
				Tx:              txn,
				Now:             call.Now,
				CorrelationID:   uuid.NewString(),
				Queries:         d.reg,
				Principal:       ops.SystemPrincipal(tenant, seed.ID("system-demo")),
				Bearer:          "",
				ExpectedVersion: nil,
				Candidates:      nil,
				Events:          nil,
				Audits:          nil,
			}

			_, err := d.notify(ctx, systemCall, evaluationInput)
			if err != nil {
				return err
			}

			if d.reg != nil && d.reg.Rec != nil {
				return d.reg.Rec.Record(ctx, txn, systemCall, "demo.trigger")
			}

			return nil
		},
	)
}

// stateReport records a device's state report: the unit's observed settings and when the device was last heard.
func (d *demoOps) stateReport(ctx context.Context, call *ops.Call, input *TriggerInput) error {
	if input.OccurredAt.After(call.Now) {
		return apperr.Fields(map[string]string{"occurredAt": apperr.KeyRange})
	}

	return d.inTenant(
		ctx,
		call,
		`SELECT EXISTS (SELECT 1 FROM devices.devices WHERE id = $1 AND unit_id IS NOT NULL)`,
		*input.DeviceID,
		func(txn pgx.Tx, _ uuid.UUID) error {
			err := observeState(ctx, txn, *input.DeviceID, input.State, input.OccurredAt)
			if err != nil {
				return err
			}

			_, err = txn.Exec(
				ctx,
				`UPDATE devices.devices SET last_seen_at = GREATEST(COALESCE(last_seen_at, $2), $2) WHERE id = $1`,
				*input.DeviceID,
				input.OccurredAt,
			)

			return apperr.Wrap(err, "update devices.devices")
		},
	)
}

// observeState updates the observed settings and the last seen time of the device's unit. A report with no settings
// (a module that reports none, IR528) is heard only: the settings that acknowledged commands set stay.
func observeState(
	ctx context.Context,
	txn pgx.Tx,
	device uuid.UUID,
	reported *ReportedState,
	observedAt time.Time,
) error {
	if reported.Power == nil && reported.Celsius == nil && reported.Mode == nil &&
		reported.FanLevel == nil {
		_, err := txn.Exec(
			ctx,
			`UPDATE assets.units u SET last_seen_at = GREATEST(COALESCE(u.last_seen_at, $2), $2)
			FROM devices.devices d WHERE d.id = $1 AND u.id = d.unit_id`,
			device,
			observedAt,
		)

		return apperr.Wrap(err, "update assets.units")
	}

	state, err := json.Marshal(
		map[string]any{
			"power":      reported.Power,
			"celsius":    reported.Celsius,
			"mode":       reported.Mode,
			"fanLevel":   reported.FanLevel,
			"observedAt": observedAt.UTC(),
		},
	)
	if err != nil {
		return apperr.Wrap(err, "json.Marshal")
	}

	_, err = txn.Exec(
		ctx,
		`UPDATE assets.units u SET observed_state = $2, last_seen_at = GREATEST(COALESCE(u.last_seen_at, $3), $3)
		FROM devices.devices d WHERE d.id = $1 AND u.id = d.unit_id`,
		device,
		state,
		observedAt,
	)

	return apperr.Wrap(err, "update assets.units")
}

// restrictionObserved records which restriction a device reports it enforces (SR26) on its unit, and publishes the
// observation, from which billing updates the recovery cases (IR185).
func (d *demoOps) restrictionObserved(
	ctx context.Context,
	call *ops.Call,
	input *TriggerInput,
) error {
	observed, err := json.Marshal(input.Observed) // "null" when the device enforces none
	if err != nil {
		return apperr.Wrap(err, "json.Marshal")
	}

	var column any // the device observation itself (equipment data), SQL NULL when it enforces none
	if input.Observed != nil {
		column = observed
	}

	return d.inTenant(
		ctx,
		call,
		`SELECT EXISTS (SELECT 1 FROM assets.units WHERE id = $1)`,
		*input.UnitID,
		func(txn pgx.Tx, _ uuid.UUID) error {
			_, err := txn.Exec(
				ctx,
				`UPDATE assets.units SET observed_restriction = $2, last_seen_at = $3 WHERE id = $1`,
				*input.UnitID,
				column,
				call.Now,
			)
			if err != nil {
				return apperr.Wrap(err, "update assets.units")
			}

			return events.Publish(
				ctx,
				txn,
				call.Principal.TenantID,
				"unit",
				*input.UnitID,
				events.UnitRestrictionObserved,
				events.Observation{
					UnitID:   *input.UnitID,
					Observed: observed,
					EventID:  input.EventID,
					At:       call.Now,
				},
			)
		},
	)
}

// telemetry stores one measurement of a sensor: D07 / IR12 normalization, IR77 sequence latest+1, IR11 boundary from
// the sensor.
func (d *demoOps) telemetry(ctx context.Context, call *ops.Call, input *TriggerInput) error {
	measurement := input.Measurement

	return d.inTenant(
		ctx,
		call,
		`SELECT EXISTS (SELECT 1 FROM devices.sensors s JOIN devices.devices d ON d.id = s.device_id WHERE s.id = $1)`,
		measurement.SensorID,
		func(txn pgx.Tx, tenant uuid.UUID) error {
			return storeMeasurement(ctx, txn, tenant, measurement, call.Now)
		},
	)
}

// storeMeasurement checks a measurement against its sensor, classifies it (IR12) and stores it once: a repeated event
// ID is not stored again.
func storeMeasurement(
	ctx context.Context,
	txn pgx.Tx,
	tenant uuid.UUID,
	measurement *RawMeasurement,
	now time.Time,
) error {
	var (
		metric, unit string
		boundary     *string
		bound        *uuid.UUID
	)

	scanErr := txn.QueryRow(ctx, `SELECT s.metric, s.unit, s.boundary_id, d.unit_id FROM devices.sensors s JOIN
		devices.devices d ON d.id = s.device_id WHERE s.id = $1`, measurement.SensorID).
		Scan(&metric, &unit, &boundary, &bound)
	if scanErr != nil {
		return apperr.Wrap(scanErr, "select devices.sensors")
	}

	if metric != measurement.Metric || bound == nil || *bound != measurement.UnitID {
		return apperr.Fields(map[string]string{"measurement": "errors.sensor_mismatch"})
	}

	quality, value, reason := classify(measurement, now)

	seq, err := sequenceOf(ctx, txn, measurement)
	if err != nil {
		return err
	}

	stored, err := alreadyStored(ctx, txn, measurement.EventID)
	if err != nil || stored {
		return err
	}

	_, err = txn.Exec(
		ctx,
		`INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit,
			raw_unit, origin, quality, quality_reason,
	boundary_id, received_at, event_id) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,$15)`,
		tenant,
		measurement.UnitID,
		measurement.SensorID,
		measurement.Metric,
		measurement.ObservedAt,
		seq,
		value,
		unit,
		rawUnit(measurement.Unit, unit),
		measurement.Origin,
		quality,
		reason,
		boundary,
		measurement.ReceivedAt,
		measurement.EventID,
	)

	return apperr.Wrap(err, "insert monitoring.measurements")
}

// alreadyStored is whether the measurement of eventID is stored already.
func alreadyStored(ctx context.Context, txn pgx.Tx, eventID uuid.UUID) (bool, error) {
	var stored bool

	err := txn.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM monitoring.measurements WHERE event_id =
		$1)`, eventID).
		Scan(&stored)
	if err != nil {
		return false, apperr.Wrap(err, "select monitoring.measurements")
	}

	return stored, nil
}

// classify is the IR12 quality of a measurement, the value to store and the reason. A cause makes it suspect and the
// first three causes drop the value (D07); otherwise a null value is missing.
func classify(measurement *RawMeasurement, now time.Time) (string, *float64, *string) {
	cause, drop := suspectCause(measurement, now)

	switch {
	case cause != "" && drop:
		return qualitySuspect, nil, &cause
	case cause != "":
		return qualitySuspect, measurement.Value, &cause
	case measurement.Value == nil:
		return "missing", nil, nil
	}

	return measurement.Quality, measurement.Value, nil
}

// suspectCause is the first IR12 cause that makes a measurement suspect, in priority order unit_mismatch →
// non_finite → out_of_range → invalid_time, and whether it drops the value; "" when none applies.
func suspectCause(measurement *RawMeasurement, now time.Time) (string, bool) {
	spec, known := metricUnits()[measurement.Metric]
	if !known || measurement.Unit != spec.unit {
		return "unit_mismatch", true
	}

	if value := measurement.Value; value != nil {
		if math.IsNaN(*value) || math.IsInf(*value, 0) {
			return "non_finite", true
		}

		if *value < spec.min || *value > spec.max {
			return "out_of_range", true
		}
	}

	if measurement.ObservedAt.After(measurement.ReceivedAt) || measurement.ReceivedAt.After(now) {
		return "invalid_time", false
	}

	return "", false
}

// sequenceOf is the measurement's sequence, or the sensor's latest + 1 when the measurement has none (IR77).
func sequenceOf(ctx context.Context, txn pgx.Tx, measurement *RawMeasurement) (int64, error) {
	if measurement.Sequence != nil {
		return *measurement.Sequence, nil
	}

	var seq int64

	err := txn.QueryRow(ctx, `SELECT COALESCE(max(sequence), 0) + 1 FROM monitoring.measurements WHERE sensor_id =
		$1`, measurement.SensorID).
		Scan(&seq)
	if err != nil {
		return 0, apperr.Wrap(err, "select monitoring.measurements")
	}

	return seq, nil
}

// rawUnitMax is how many characters of a received unit IR12 keeps as evidence.
const rawUnitMax = 32

// rawUnit keeps a received unit that differs from the sensor's (IR12: at most 32 characters as evidence).
func rawUnit(got, want string) *string {
	if got == want {
		return nil
	}

	if r := []rune(got); len(r) > rawUnitMax {
		got = string(r[:rawUnitMax])
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
	// each trigger runs in the transaction of its tenant (inTenant)
	ops.RegisterOwnTx(reg, "demo.trigger", d.trigger)
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
//	@Description	Validation: D01; input constraints in the corresponding DD
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
//	@Router			/v1/demo-session [post]
func demoSessionSignIn(ctx context.Context, c *ops.Call, in *demoSignIn) (struct{}, error) {
	return demoSessionViaBFF(ctx, c, in)
}

// demoSessionSwitch serves demoSession.switchMembership.
//
//	@Summary		demoSession.switchMembership (write)
//	@ID				demoSession.switchMembership
//	@Description	Authorization: authenticated:demo-account-switch; D09
//	@Description	Validation: D01; input constraints in the corresponding DD
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DDC-07
//	@Tags			demoSession
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		demoSwitch	true	"input"
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
//	@Router			/v1/demo-session/switch-membership [post]
func demoSessionSwitch(ctx context.Context, c *ops.Call, in *demoSwitch) (struct{}, error) {
	return demoSessionViaBFF(ctx, c, in)
}

// demoSessionSignOut serves demoSession.signOut.
//
//	@Summary		demoSession.signOut (write)
//	@ID				demoSession.signOut
//	@Description	Authorization: authenticated:own-session
//	@Description	Validation: D01; input constraints in the corresponding DD
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DDC-07
//	@Tags			demoSession
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
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
//	@Router			/v1/demo-session [delete]
func demoSessionSignOut(ctx context.Context, c *ops.Call, in *struct{}) (struct{}, error) {
	return demoSessionViaBFF(ctx, c, in)
}

// demoSessionExtend serves demoSession.extend.
//
//	@Summary		demoSession.extend (write)
//	@ID				demoSession.extend
//	@Description	Authorization: authenticated:own-session
//	@Description	Validation: IR55 valid session only; expiresAt=now+30min; no audit
//	@Description	Recovery: UNAUTHENTICATED after expiry: purge and /login
//	@Description	Design: DDC-07
//	@Tags			demoSession
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
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
//	@Router			/v1/demo-session/extend [post]
func demoSessionExtend(ctx context.Context, c *ops.Call, in *struct{}) (struct{}, error) {
	return demoSessionViaBFF(ctx, c, in)
}
