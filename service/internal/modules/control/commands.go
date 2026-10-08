package control

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/internal/modules/restrictions"
	"github.com/pradita/ac-project/service/internal/ops"
	"github.com/pradita/ac-project/service/internal/platform/apperr"
)

// Target is what Control needs to know about a unit.
type Target struct {
	OrgID, PropertyID uuid.UUID
	Version           int
	Caps              Caps
}

// Units resolves control targets (Assets + Devices capability).
type Units interface {
	Target(ctx context.Context, c *ops.Call, unit uuid.UUID) (Target, bool, error)
}

// Devices reports the device bound to a unit (Devices).
type Devices interface {
	BoundDevice(ctx context.Context, c *ops.Call, unit uuid.UUID) (id uuid.UUID, connection, powerSignal string, found bool, err error)
	OperationBusy(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error)
}

// Restrictions reports a unit's active restriction policy (Restrictions).
type Restrictions interface {
	UnitPolicy(ctx context.Context, c *ops.Call, unit uuid.UUID) ([]byte, error)
}

// TechAccess is the IR94 technician write check (Maintenance).
type TechAccess interface {
	TechnicianJob(ctx context.Context, c *ops.Call, jobID uuid.UUID, unit uuid.UUID) error
}

// Commands is the unit command operation set (IR138).
type Commands struct {
	Units        Units
	Devices      Devices
	Restrictions Restrictions
	Access       TechAccess
}

// CommandTTL is the acknowledgement window of a sent command.
const CommandTTL = 30 * time.Second

// Command is Command of service-contracts.ts.
type Command struct {
	ID                uuid.UUID       `json:"id"`
	TenantID          uuid.UUID       `json:"tenantId"`
	Version           int             `json:"version"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	UnitID            uuid.UUID       `json:"unitId"`
	ActorMembershipID uuid.UUID       `json:"actorMembershipId"`
	Action            json.RawMessage `json:"action"`
	DiagnosticRunID   *uuid.UUID      `json:"diagnosticRunId"`
	JobID             *uuid.UUID      `json:"jobId"`
	Reason            *string         `json:"reason"`
	Status            string          `json:"status"`
	Delivery          string          `json:"delivery"`
	RequestedAt       time.Time       `json:"requestedAt"`
	SentAt            *time.Time      `json:"sentAt"`
	AcknowledgedAt    *time.Time      `json:"acknowledgedAt"`
	ExpiresAt         time.Time       `json:"expiresAt"`
	FailureCode       *string         `json:"failureCode"`
	CorrelationID     string          `json:"correlationId"`
	orgID             uuid.UUID
}

const commandCols = `id, tenant_id, version, created_at, updated_at, unit_id, actor_membership_id, action, diagnostic_run_id, job_id, reason, status, delivery,
	requested_at, sent_at, acknowledged_at, expires_at, failure_code, correlation_id`

func scanCommand(r pgx.Row) (Command, error) {
	var x Command
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.UnitID, &x.ActorMembershipID, &x.Action, &x.DiagnosticRunID, &x.JobID, &x.Reason,
		&x.Status, &x.Delivery, &x.RequestedAt, &x.SentAt, &x.AcknowledgedAt, &x.ExpiresAt, &x.FailureCode, &x.CorrelationID)
	return x, err
}

// Allowed applies the IR46 restriction table to an action (policy is RestrictionPolicy JSON or nil).
func Allowed(a UnitAction, policy []byte) bool {
	if len(policy) == 0 {
		return true
	}
	var p struct {
		Kind                   string   `json:"kind"`
		MinimumCoolingSetpoint *float64 `json:"minimumCoolingSetpoint"`
	}
	if json.Unmarshal(policy, &p) != nil {
		return false
	}
	if p.Kind == "power_off" {
		return a.Kind == "set_power" && a.Power != nil && !*a.Power
	}
	if a.Kind == "set_temperature" && p.MinimumCoolingSetpoint != nil {
		return *a.Celsius >= *p.MinimumCoolingSetpoint
	}
	return true
}

// Deliverable applies IR47 to the bound device (nil when control may be delivered).
func Deliverable(found bool, connection, powerSignal string) error {
	switch {
	case !found:
		return apperr.E(apperr.Offline, "errors.device_unknown")
	case powerSignal == "off":
		return apperr.E(apperr.Offline, "errors.device_power_lost")
	case connection != "online":
		return apperr.E(apperr.Offline, "errors.device_"+connection)
	}
	return nil
}

// CreateInput is commands.create input.
type CreateInput struct {
	UnitID              uuid.UUID       `json:"unitId"`
	Action              json.RawMessage `json:"action"`
	JobID               *uuid.UUID      `json:"jobId,omitempty"`
	Reason              *string         `json:"reason,omitempty"`
	ExpectedUnitVersion int             `json:"expectedUnitVersion"`
	action              UnitAction
}

// Validate implements ops.Validator.
func (in *CreateInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.UnitID == uuid.Nil {
		fe["unitId"] = "error.required"
	}
	a, ok := ParseAction(in.Action)
	if !ok {
		fe["action"] = "error.invalid"
	}
	in.action = a
	if in.ExpectedUnitVersion < 1 {
		fe["expectedUnitVersion"] = "error.required"
	}
	if in.Reason != nil {
		*in.Reason = strings.TrimSpace(*in.Reason)
		if n := utf8.RuneCountInString(*in.Reason); n < 1 || n > 1000 {
			fe["reason"] = "error.length"
		}
	}
	return fe
}

// inScope reports whether a technician's membership scopes contain the unit.
func inScope(p *ops.Principal, unit, property, org uuid.UUID) bool {
	has := func(k string, id uuid.UUID) bool {
		for _, x := range p.Scopes[k] {
			if x == id {
				return true
			}
		}
		return false
	}
	return has("unit", unit) || has("property", property) || has("organization", org)
}

func (m Commands) create(ctx context.Context, c *ops.Call, in *CreateInput) (Command, error) {
	t, found, err := m.Units.Target(ctx, c, in.UnitID)
	if err != nil {
		return Command{}, err
	}
	nf := apperr.E(apperr.NotFound, "error.notFound")
	switch c.Principal.Role {
	case "client":
		if !found || t.OrgID != c.Principal.OrgID {
			return Command{}, nf
		}
		if in.Reason != nil {
			return Command{}, apperr.Fields(map[string]string{"reason": "error.notAllowed"})
		}
	case "technician":
		if !found {
			return Command{}, nf
		}
		if in.JobID == nil {
			return Command{}, apperr.Fields(map[string]string{"jobId": "error.required"})
		}
		if err := m.Access.TechnicianJob(ctx, c, *in.JobID, in.UnitID); err != nil {
			return Command{}, err
		}
		if in.Reason == nil {
			return Command{}, apperr.Fields(map[string]string{"reason": "error.required"})
		}
	default:
		if !found {
			return Command{}, nf
		}
		if in.Reason == nil {
			return Command{}, apperr.Fields(map[string]string{"reason": "error.required"})
		}
	}
	if t.Version != in.ExpectedUnitVersion {
		return Command{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if !in.action.Supports(t.Caps) {
		return Command{}, apperr.Fields(map[string]string{"action": "error.unsupportedAction"})
	}
	policy, err := m.Restrictions.UnitPolicy(ctx, c, in.UnitID)
	if err != nil {
		return Command{}, err
	}
	if !Allowed(in.action, policy) {
		e := apperr.E(apperr.Forbidden, "errors.restriction_active")
		e.FieldErrors = map[string]string{"action": "errors.restriction_active"}
		return Command{}, e
	}
	dev, err := m.ready(ctx, c, in.UnitID)
	if err != nil {
		return Command{}, err
	}
	x, err := scanCommand(c.Tx.QueryRow(ctx, `INSERT INTO control.commands (tenant_id, unit_id, device_id, actor_membership_id, source, action, job_id, reason, status, delivery,
		requested_at, sent_at, expires_at, correlation_id, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, 'ui', $4, $5, $6, 'sent', 'sent', $7, $7, $8, $9, $7, $7) RETURNING `+commandCols,
		in.UnitID, dev, c.Principal.MembershipID, []byte(in.Action), in.JobID, in.Reason, c.Now, c.Now.Add(CommandTTL), c.CorrelationID))
	if err != nil {
		return x, err
	}
	c.Emit(ops.Event{AggregateType: "command", AggregateID: x.ID, Type: "CommandRequested", Payload: map[string]any{"unitId": in.UnitID, "deviceId": dev, "action": json.RawMessage(in.Action)}})
	c.Audit(ops.AuditEntry{Action: "commands.create", TargetKind: "command", TargetID: x.ID.String(), NextVersion: &x.Version, Reason: deref(in.Reason)})
	return x, nil
}

// ready applies the D04 mutual exclusion and IR47 delivery checks and returns the bound device.
func (m Commands) ready(ctx context.Context, c *ops.Call, unit uuid.UUID) (uuid.UUID, error) {
	var blocked bool // SR26: an unresolved terminal-restriction recovery case blocks ordinary control
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM restrictions.restrictions r, jsonb_array_elements(r.recovery_cases) k
		WHERE k->>'unitId' = $1::text AND k->>'state' <> 'resolved')`, unit).Scan(&blocked); err != nil {
		return uuid.Nil, err
	}
	if blocked {
		return uuid.Nil, apperr.E(apperr.Conflict, "errors.reconciliation_required")
	}
	busy, err := Busy{}.UnitBusy(ctx, c, unit)
	if err != nil {
		return uuid.Nil, err
	}
	opBusy, err := m.Devices.OperationBusy(ctx, c, unit)
	if err != nil {
		return uuid.Nil, err
	}
	if busy || opBusy {
		return uuid.Nil, apperr.E(apperr.Conflict, "errors.unit_busy")
	}
	dev, conn, power, bound, err := m.Devices.BoundDevice(ctx, c, unit)
	if err != nil {
		return uuid.Nil, err
	}
	return dev, Deliverable(bound, conn, power)
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// GetInput is commands.get input.
type GetInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *GetInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": "error.required"}
	}
	return nil
}

func (m Commands) get(ctx context.Context, c *ops.Call, in *GetInput) (Command, error) {
	x, err := scanCommand(c.Tx.QueryRow(ctx, "SELECT "+commandCols+" FROM control.commands WHERE id = $1", in.ID))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return x, err
	}
	if c.Principal.Role == "client" || c.Principal.Role == "technician" {
		t, found, err := m.Units.Target(ctx, c, x.UnitID)
		if err != nil {
			return x, err
		}
		ok := found && ((c.Principal.Role == "client" && t.OrgID == c.Principal.OrgID) || (c.Principal.Role == "technician" && inScope(c.Principal, x.UnitID, t.PropertyID, t.OrgID)))
		if !ok {
			return Command{}, apperr.E(apperr.NotFound, "error.notFound")
		}
	}
	return x, nil
}

// ExpireCommands marks sent commands past their window as expired (worker tick, IR138 item 2).
func ExpireCommands(ctx context.Context, tx pgx.Tx, now time.Time) (int, error) {
	rows, err := tx.Query(ctx, `UPDATE control.commands SET status = 'expired', failure_code = 'TIMEOUT', version = version + 1, updated_at = $1
		WHERE status IN ('requested','sent') AND expires_at <= $1 RETURNING id`, now)
	if err != nil {
		return 0, err
	}
	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, err
	}
	return len(ids), restrictions.CommandsEnded(ctx, tx, ids, now)
}

// Acknowledge applies a device acknowledgement (IoT bridge / device simulator): before expiresAt the command becomes
// acknowledged; later acknowledgements are only recorded (D04 timing). It returns whether the command was acknowledged.
func Acknowledge(ctx context.Context, tx pgx.Tx, command uuid.UUID, receivedAt time.Time) (bool, error) {
	tag, err := tx.Exec(ctx, `UPDATE control.commands SET status = 'acknowledged', acknowledged_at = $2, version = version + 1, updated_at = $2
		WHERE id = $1 AND status = 'sent' AND $2 < expires_at`, command, receivedAt)
	if err != nil {
		return false, err
	}
	if tag.RowsAffected() == 1 {
		if err := restrictions.CommandAcknowledged(ctx, tx, command, receivedAt); err != nil {
			return true, err
		}
		if err := applyObserved(ctx, tx, command, receivedAt); err != nil {
			return true, err
		}
		return true, runAcknowledged(ctx, tx, command, receivedAt)
	}
	_, err = tx.Exec(ctx, `UPDATE control.commands SET late_ack_at = $2 WHERE id = $1 AND late_ack_at IS NULL`, command, receivedAt)
	return false, err
}

// applyObserved copies an acknowledged unit setting into ACUnit.observedState (power / celsius / mode / fanLevel
// change only through a Command ack or telemetry, IR45/IR50). observedState is an observation field, so the unit
// version is unchanged and a fetched expectedUnitVersion stays valid.
func applyObserved(ctx context.Context, tx pgx.Tx, command uuid.UUID, at time.Time) error {
	_, err := tx.Exec(ctx, `UPDATE assets.units u SET observed_state = u.observed_state || CASE c.action->>'kind'
			WHEN 'set_power' THEN jsonb_build_object('power', c.action->'power')
			WHEN 'set_temperature' THEN jsonb_build_object('celsius', c.action->'celsius')
			WHEN 'set_mode' THEN jsonb_build_object('mode', c.action->'mode')
			WHEN 'set_fan' THEN jsonb_build_object('fanLevel', c.action->'fanLevel')
		END || jsonb_build_object('observedAt', $2::text)
		FROM control.commands c
		WHERE c.id = $1 AND u.id = c.unit_id AND c.action->>'kind' IN ('set_power', 'set_temperature', 'set_mode', 'set_fan')`,
		command, at.UTC().Format(time.RFC3339Nano))
	return err
}

// Register binds commands.create / commands.get.
func Register(r *ops.Registry, m Commands) {
	ops.Register(r, "commands.create", m.create)
	ops.Register(r, "commands.get", m.get)
}

// PendingCommands returns the unit's requested / sent Commands, oldest first (UnitDetail.pendingCommands).
func PendingCommands(ctx context.Context, c *ops.Call, unit uuid.UUID) ([]Command, error) {
	rows, err := c.Tx.Query(ctx, "SELECT "+commandCols+" FROM control.commands WHERE unit_id = $1 AND status IN ('requested','sent') ORDER BY requested_at, id", unit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Command{}
	for rows.Next() {
		x, err := scanCommand(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, x)
	}
	return out, rows.Err()
}
