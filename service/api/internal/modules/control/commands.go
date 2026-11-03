package control

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
	"github.com/pradita/ac-project/service/api/internal/platform/events"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
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
	BoundDevice(
		ctx context.Context,
		c *ops.Call,
		unit uuid.UUID,
	) (id uuid.UUID, connection, powerSignal string, found bool, err error)
	OperationBusy(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error)
}

// Restrictions reports a unit's active restriction policy and unresolved recovery cases (Restrictions, billing).
type Restrictions interface {
	UnitPolicy(ctx context.Context, c *ops.Call, unit uuid.UUID) ([]byte, error)
	UnitRecovering(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error)
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
	Action            json.RawMessage `json:"action"            swaggertype:"object"`
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
	Source            string          `json:"source"` // ui | voice | automation | restriction | diagnostic | group (IR216)
}

const commandCols = `id, tenant_id, version, created_at, updated_at, unit_id, actor_membership_id, action, ` +
	`diagnostic_run_id, job_id, reason, status, delivery,
	requested_at, sent_at, acknowledged_at, expires_at, failure_code, correlation_id, source`

func scanCommand(r pgx.Row) (Command, error) {
	var command Command

	err := r.Scan(
		&command.ID,
		&command.TenantID,
		&command.Version,
		&command.CreatedAt,
		&command.UpdatedAt,
		&command.UnitID,
		&command.ActorMembershipID,
		&command.Action,
		&command.DiagnosticRunID,
		&command.JobID,
		&command.Reason,
		&command.Status,
		&command.Delivery,
		&command.RequestedAt,
		&command.SentAt,
		&command.AcknowledgedAt,
		&command.ExpiresAt,
		&command.FailureCode,
		&command.CorrelationID,
		&command.Source,
	)

	return command, apperr.Wrap(err, "scan (scanCommand)")
}

// Allowed applies the IR46 restriction table to an action (policy is RestrictionPolicy JSON or nil).
func Allowed(unitAction UnitAction, policy []byte) bool {
	if len(policy) == 0 {
		return true
	}

	var restrictionPolicy struct {
		Kind                   string   `json:"kind"`
		MinimumCoolingSetpoint *float64 `json:"minimumCoolingSetpoint"`
	}
	if json.Unmarshal(policy, &restrictionPolicy) != nil {
		return false
	}

	if restrictionPolicy.Kind == "power_off" {
		return unitAction.Kind == "set_power" && unitAction.Power != nil && !*unitAction.Power
	}

	if unitAction.Kind == "set_temperature" && restrictionPolicy.MinimumCoolingSetpoint != nil {
		return *unitAction.Celsius >= *restrictionPolicy.MinimumCoolingSetpoint
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
	Action              json.RawMessage `json:"action"              swaggertype:"object"`
	JobID               *uuid.UUID      `json:"jobId,omitempty"`
	Reason              *string         `json:"reason,omitempty"`
	ExpectedUnitVersion int             `json:"expectedUnitVersion"`
	// the channel the user chose; ui when omitted (IR306)
	Source string `enums:"ui,voice" json:"source,omitempty"`
	action UnitAction
}

// Validate implements ops.Validator.
func (in *CreateInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.UnitID == uuid.Nil {
		fieldErrs[keyUnitID] = apperr.KeyRequired
	}

	a, ok := ParseAction(in.Action)
	if !ok {
		fieldErrs[keyAction] = apperr.KeyInvalid
	}

	in.action = a
	if in.ExpectedUnitVersion < 1 {
		fieldErrs["expectedUnitVersion"] = apperr.KeyRequired
	}

	if in.Source == "" {
		in.Source = "ui"
	} else if in.Source != "ui" && in.Source != sourceVoice {
		fieldErrs[keySource] = apperr.KeyInvalid
	}

	if in.Reason != nil {
		*in.Reason = strings.TrimSpace(*in.Reason)
		if n := utf8.RuneCountInString(*in.Reason); n < 1 || n > 1000 {
			fieldErrs[keyReason] = apperr.KeyLength
		}
	}

	return fieldErrs
}

// inScope reports whether a technician's membership scopes contain the unit.
func inScope(p *ops.Principal, unit, property, org uuid.UUID) bool {
	has := func(k string, id uuid.UUID) bool {
		return slices.Contains(p.Scopes[k], id)
	}

	return has("unit", unit) || has("property", property) || has("organization", org)
}

// create serves commands.create.
//
//	@Summary		commands.create (write)
//	@ID				commands.create
//	@Description	Authorization: client:control.execute | technician:control.diagnose:job-required | \
//	@Description	admin:control.execute
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR46 restriction action table; IR47 derived connection/powerSignal OFFLINE; IR74 client omits \
//	@Description	reason; IR94 technician write table (assignment and work window, jobId required when typed); IR109 \
//	@Description	group control sends one command per selected unit (owner only); IR306 source ui (default) or voice, \
//	@Description	voice only from a client (the assistant of the Client app)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-C03, DD-T10, DD-A11 through automations.fire, DD-C04, DD-C05, DD-A02, DD-C14 · Input \
//	@Description	versions: expectedUnitVersion=UnitDetail.version
//	@Tags			commands
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		CreateInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Command}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/commands [post]
func (m Commands) create(ctx context.Context, c *ops.Call, input *CreateInput) (Command, error) {
	target, found, err := m.Units.Target(ctx, c, input.UnitID)
	if err != nil {
		return Command{}, apperr.Wrap(err, "control.Units.Target")
	}

	notFound := apperr.E(apperr.NotFound, apperr.KeyNotFound)

	switch c.Principal.Role {
	case authz.RoleClient:
		if !found || target.OrgID != c.Principal.OrgID {
			return Command{}, notFound
		}

		if input.Reason != nil {
			return Command{}, apperr.Fields(map[string]string{keyReason: apperr.KeyNotAllowed})
		}
	case authz.RoleTechnician:
		if input.Source == sourceVoice {
			return Command{}, apperr.Fields(map[string]string{keySource: apperr.KeyNotAllowed})
		}

		if !found {
			return Command{}, notFound
		}

		if input.JobID == nil {
			return Command{}, apperr.Fields(map[string]string{"jobId": apperr.KeyRequired})
		}

		err := m.Access.TechnicianJob(ctx, c, *input.JobID, input.UnitID)
		if err != nil {
			return Command{}, apperr.Wrap(err, "control.TechAccess.TechnicianJob")
		}

		if input.Reason == nil {
			return Command{}, apperr.Fields(map[string]string{keyReason: apperr.KeyRequired})
		}
	default:
		if !found {
			return Command{}, notFound
		}

		if input.Source == sourceVoice { // the assistant is offered only in the Client app (IR115)
			return Command{}, apperr.Fields(map[string]string{keySource: apperr.KeyNotAllowed})
		}

		if input.Reason == nil {
			return Command{}, apperr.Fields(map[string]string{keyReason: apperr.KeyRequired})
		}
	}

	if target.Version != input.ExpectedUnitVersion {
		return Command{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if !input.action.Supports(target.Caps) {
		return Command{}, apperr.Fields(map[string]string{keyAction: apperr.KeyUnsupportedAction})
	}

	policy, err := m.Restrictions.UnitPolicy(ctx, c, input.UnitID)
	if err != nil {
		return Command{}, apperr.Wrap(err, "control.Restrictions.UnitPolicy")
	}

	if !Allowed(input.action, policy) {
		e := apperr.E(apperr.Forbidden, "errors.restriction_active")
		e.FieldErrors = map[string]string{keyAction: "errors.restriction_active"}

		return Command{}, e
	}

	dev, err := m.ready(ctx, c, input.UnitID)
	if err != nil {
		return Command{}, err
	}

	command, err := scanCommand(
		c.Tx.QueryRow(
			ctx,
			`INSERT INTO control.commands (tenant_id, unit_id, device_id, actor_membership_id, source, action, job_id, reason,
				status, delivery,
		requested_at, sent_at, expires_at, correlation_id, created_at, updated_at)
		VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $10, $4, $5, $6, 'requested', 'not_sent', $7, NULL, $8,
		$9, $7, $7) RETURNING `+commandCols,
			input.UnitID,
			dev,
			c.Principal.MembershipID,
			[]byte(input.Action),
			input.JobID,
			input.Reason,
			c.Now,
			c.Now.Add(CommandTTL),
			c.CorrelationID,
			input.Source,
		),
	)
	if err != nil {
		return command, err
	}
	// requested: the IoT gateway sends it to the device and reports it sent (demo trigger command_sent)
	c.Emit(
		ops.Event{
			AggregateType: kindCommand,
			AggregateID:   command.ID,
			Type:          eventCommandRequested,
			Payload: map[string]any{
				keyUnitID:  input.UnitID,
				"deviceId": dev,
				keyAction:  json.RawMessage(input.Action),
			},
		},
	)
	// the device answers later: the request is pending, and its result follows under this correlation ID (IR90, IR604)
	c.Audit(
		ops.AuditEntry{
			Action:      "commands.create",
			TargetKind:  kindCommand,
			TargetID:    command.ID.String(),
			NextVersion: &command.Version,
			Reason:      deref(input.Reason),
			Result:      "pending", PreviousVersion: nil, Before: nil, After: nil,
		},
	)

	return command, nil
}

// ready applies the D04 mutual exclusion and IR47 delivery checks and returns the bound device.
func (m Commands) ready(ctx context.Context, call *ops.Call, unit uuid.UUID) (uuid.UUID, error) {
	blocked, err := m.Restrictions.UnitRecovering(
		ctx,
		call,
		unit,
	) // SR26: an unresolved terminal-restriction recovery case blocks ordinary control
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "control.Restrictions.UnitRecovering")
	}

	if blocked {
		return uuid.Nil, apperr.E(apperr.Conflict, "errors.reconciliation_required")
	}

	busy, err := Busy{}.UnitBusy(ctx, call, unit)
	if err != nil {
		return uuid.Nil, err
	}

	opBusy, err := m.Devices.OperationBusy(ctx, call, unit)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "control.Devices.OperationBusy")
	}

	if busy || opBusy {
		return uuid.Nil, apperr.E(apperr.Conflict, "errors.unit_busy")
	}

	dev, conn, power, bound, err := m.Devices.BoundDevice(ctx, call, unit)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "control.Devices.BoundDevice")
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
		return map[string]string{"id": apperr.KeyRequired}
	}

	return nil
}

// get serves commands.get.
//
//	@Summary		commands.get (read)
//	@ID				commands.get
//	@Description	Authorization: client:self | technician:assigned | admin:control.execute | admin:restriction.read | \
//	@Description	admin:restriction.override | admin:automation.policy.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A09, DD-A12, DD-C03, DD-C12, DD-T10, DD-A02, DD-A16, DD-C14
//	@Tags			commands
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"input field id"
//	@Success		200	{object}	ops.Envelope{data=Command}
//	@Failure		401	{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403	{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404	{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409	{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422	{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429	{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503	{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504	{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/commands/{id} [get]
func (m Commands) get(ctx context.Context, call *ops.Call, in *GetInput) (Command, error) {
	command, err := scanCommand(
		call.Tx.QueryRow(ctx, "SELECT "+commandCols+" FROM control.commands WHERE id = $1", in.ID),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return command, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return command, err
	}

	if call.Principal.Role == authz.RoleClient || call.Principal.Role == authz.RoleTechnician {
		target, found, err := m.Units.Target(ctx, call, command.UnitID)
		if err != nil {
			return command, apperr.Wrap(err, "control.Units.Target")
		}

		ok := found &&
			((call.Principal.Role == authz.RoleClient && target.OrgID == call.Principal.OrgID) ||
				(call.Principal.Role == authz.RoleTechnician && inScope(call.Principal, command.UnitID, target.PropertyID, target.OrgID)))
		if !ok {
			return Command{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
		}
	}

	return command, nil
}

// ListInput is commands.list input (IR216).
type ListInput struct {
	UnitID uuid.UUID    `json:"unitId"`
	JobID  *uuid.UUID   `json:"jobId,omitempty"`
	Query  paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *ListInput) Validate() map[string]string {
	if in.UnitID == uuid.Nil {
		return map[string]string{keyUnitID: apperr.KeyRequired}
	}

	return nil
}

// list is the unit's Command history, newest first (IR216: C03 unit history, T10 job history): a client sees the
// commands of its organization's unit — the reason of a command someone else sent is withheld; a technician sees
// one job's commands (jobId required) of a job they hold or held an Assignment of (equipment's projection, IR187);
// HQ sees the unit's commands.
//
//	@Summary		commands.list (read)
//	@ID				commands.list
//	@Description	Authorization: client:self | technician:control.diagnose:assigned-valid-job | admin:control.execute \
//	@Description	| admin:job.read
//	@Description	Validation: D01; SR08/SR09; IR216 the unit's history newest first; a client sees others' reasons \
//	@Description	withheld; a technician sees one job it holds or held an Assignment of (jobId required)
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-C03, DD-T10 · Query: filters none · sort id,requestedAt (default requestedAt desc;id asc)
//	@Tags			commands
//	@Accept			json
//	@Produce		json
//	@Param			unitId	query		string	false	"input field unitId"
//	@Param			jobId	query		string	false	"input field jobId"
//	@Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer	false	"page size 1–100, default 25"
//	@Param			sort	query		string	false	"field:direction — fields id,requestedAt; default requestedAt desc;id asc"
//	@Success		200		{object}	ops.Envelope{data=CommandPage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/commands [get]
func (m Commands) list(
	ctx context.Context,
	call *ops.Call,
	input *ListInput,
) (paging.Page[Command], error) {
	err := paging.NoFilters(input.Query, "query.filters")
	if err != nil {
		return paging.Page[Command]{}, apperr.Wrap(err, "paging.NoFilters")
	}

	order, err := paging.OrderBy(
		input.Query.Sort,
		map[string]string{"id": "id", "requestedAt": "requested_at"},
		"requested_at DESC, id",
	)
	if err != nil {
		return paging.Page[Command]{}, apperr.Wrap(err, "paging.OrderBy")
	}
	// the cursor binds the unit and job, not the paging fields themselves (the next page carries the cursor)
	window, err := paging.Resolve(input.Query, struct {
		UnitID uuid.UUID  `json:"unitId"`
		JobID  *uuid.UUID `json:"jobId"`
	}{input.UnitID, input.JobID}, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Command]{}, apperr.Wrap(err, "paging.Resolve")
	}

	target, found, err := m.Units.Target(ctx, call, input.UnitID)
	if err != nil {
		return paging.Page[Command]{}, apperr.Wrap(err, "control.Units.Target")
	}

	notFound := apperr.E(apperr.NotFound, apperr.KeyNotFound)

	switch call.Principal.Role {
	case authz.RoleClient:
		if !found || target.OrgID != call.Principal.OrgID {
			return paging.Page[Command]{}, notFound
		}
	case authz.RoleTechnician:
		if input.JobID == nil {
			return paging.Page[Command]{}, apperr.Fields(
				map[string]string{"jobId": apperr.KeyRequired},
			)
		}

		var assigned bool

		err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.unit_assignment_access WHERE job_id = $1 AND unit_id
			=
			$2 AND technician_membership_id = $3)`,
			*input.JobID, input.UnitID, call.Principal.MembershipID).
			Scan(&assigned)
		if err != nil {
			return paging.Page[Command]{}, apperr.Wrap(err, "select assets.unit_assignment_access")
		}

		if !assigned {
			return paging.Page[Command]{}, notFound
		}
	default:
		if !found {
			return paging.Page[Command]{}, notFound
		}
	}

	args := []any{input.UnitID}
	where := "unit_id = $1"

	if input.JobID != nil {
		args = append(args, *input.JobID)
		where += " AND job_id = $2"
	}

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM control.commands WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Command]{}, apperr.Wrap(scanErr, "select control.commands")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM control.commands WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			commandCols,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[Command]{}, apperr.Wrap(err, "select control.commands")
	}
	defer rows.Close()

	items := []Command{}

	for rows.Next() {
		command, err := scanCommand(rows)
		if err != nil {
			return paging.Page[Command]{}, err
		}

		if call.Principal.Role == authz.RoleClient &&
			command.ActorMembershipID != call.Principal.MembershipID {
			command.Reason = nil // another actor's reason (a technician's or HQ's) is not the customer's
		}

		items = append(items, command)
	}

	return paging.Page[Command]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, apperr.Wrap(rows.Err(), "select control.commands")
}

// AuditResult appends the final result of a command that a person asked for (commands.create: source ui or voice) under
// the request's correlation ID, after its pending entry: success when the device acknowledged it, failed when the
// device refused it or its window passed, the failure code as the reason (IR90, IR604). actor is the system part that
// saw it. Commands of a restriction, a diagnosis or an automation are recorded under their own operations.
func AuditResult(
	ctx context.Context,
	txn pgx.Tx,
	command uuid.UUID,
	actor, result, reason string,
	when time.Time,
) error {
	var (
		tenant uuid.UUID
		corr   *string
		source string
	)

	err := txn.QueryRow(ctx, `SELECT tenant_id, correlation_id, source FROM control.commands WHERE id = $1`, command).
		Scan(&tenant, &corr, &source)
	if err != nil {
		return apperr.Wrap(err, "select control.commands")
	}

	if (source != "ui" && source != sourceVoice) || corr == nil || *corr == "" {
		return nil
	}

	id := uuid.Must(uuid.NewV7())

	return apperr.Wrap(
		events.Publish(ctx, txn, tenant, "audit", id, events.AuditRecorded, events.Audit{
			ID:              id,
			ActorID:         actor,
			ActorRole:       "system",
			Action:          "commands.create",
			TargetKind:      kindCommand,
			TargetID:        command.String(),
			OccurredAt:      when,
			CorrelationID:   *corr,
			Result:          result,
			Reason:          reason,
			MembershipID:    nil,
			PreviousVersion: nil,
			NextVersion:     nil,
			MaskedBefore:    nil,
			MaskedAfter:     nil,
		}),
		"events.Publish",
	)
}

// ExpireGrace is how long after expiresAt a command waits before it is expired (IR453): an acknowledgement the bridge
// received before expiresAt may still be on its way to the API, and the time it was received decides the result.
const ExpireGrace = 5 * time.Second

// ExpireCommands marks sent commands past their window as expired (worker tick, IR138 item 2), ExpireGrace after it.
func ExpireCommands(ctx context.Context, txn pgx.Tx, now time.Time) (int, error) {
	rows, err := txn.Query(
		ctx,
		`UPDATE control.commands SET status = 'expired', failure_code = 'TIMEOUT', version = version + 1, updated_at = $1
		WHERE status IN ('requested','sent') AND expires_at <= $2 RETURNING id`,
		now,
		now.Add(-ExpireGrace),
	)
	if err != nil {
		return 0, apperr.Wrap(err, "update control.commands")
	}

	ids, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return 0, apperr.Wrap(err, "update control.commands")
	}

	for _, id := range ids {
		err := AuditResult(
			ctx,
			txn,
			id,
			"system-scheduler",
			"failed",
			"TIMEOUT",
			now,
		)
		if err != nil {
			return 0, err
		}
	}

	return len(
			ids,
		), publishRestrictionCommands(
			ctx,
			txn,
			events.CommandsEnded,
			ids,
			now,
		) // billing reacts (IR185)
}

// Acknowledge applies a device acknowledgement (IoT bridge / device simulator): before expiresAt the command becomes
// acknowledged; later acknowledgements are only recorded (D04 timing). It returns whether the command was acknowledged.
func Acknowledge(
	ctx context.Context,
	txn pgx.Tx,
	command uuid.UUID,
	receivedAt time.Time,
) (bool, error) {
	tag, err := txn.Exec(
		ctx,
		`UPDATE control.commands SET status = 'acknowledged', acknowledged_at = $2, version = version + 1, updated_at = $2
		WHERE id = $1 AND status = 'sent' AND $2 < expires_at`,
		command,
		receivedAt,
	)
	if err != nil {
		return false, apperr.Wrap(err, "update control.commands")
	}

	if tag.RowsAffected() == 1 {
		err := publishRestrictionCommands(
			ctx,
			txn,
			events.CommandAcknowledged,
			[]uuid.UUID{command},
			receivedAt,
		)
		if err != nil { // billing reacts (IR185)
			return true, err
		}

		err = AuditResult(
			ctx,
			txn,
			command,
			"system-iot",
			"success",
			"",
			receivedAt,
		)
		if err != nil {
			return true, err
		}

		err = applyObserved(ctx, txn, command, receivedAt)
		if err != nil {
			return true, err
		}

		return true, runAcknowledged(ctx, txn, command, receivedAt)
	}

	_, err = txn.Exec(
		ctx,
		`UPDATE control.commands SET late_ack_at = $2 WHERE id = $1 AND late_ack_at IS NULL`,
		command,
		receivedAt,
	)

	return false, apperr.Wrap(err, "update control.commands")
}

// applyObserved copies an acknowledged unit setting into ACUnit.observedState (power / celsius / mode / fanLevel
// change only through a Command ack or telemetry, IR45/IR50). observedState is an observation field, so the unit
// version is unchanged and a fetched expectedUnitVersion stays valid.
func applyObserved(ctx context.Context, tx pgx.Tx, command uuid.UUID, when time.Time) error {
	_, err := tx.Exec(
		ctx,
		`UPDATE assets.units u SET observed_state = u.observed_state || CASE c.action->>'kind'
			WHEN 'set_power' THEN jsonb_build_object('power', c.action->'power')
			WHEN 'set_temperature' THEN jsonb_build_object('celsius', c.action->'celsius')
			WHEN 'set_mode' THEN jsonb_build_object('mode', c.action->'mode')
			WHEN 'set_fan' THEN jsonb_build_object('fanLevel', c.action->'fanLevel')
		END || jsonb_build_object('observedAt', $2::text)
		FROM control.commands c
		WHERE c.id = $1 AND u.id = c.unit_id AND c.action->>'kind' IN ('set_power', 'set_temperature', 'set_mode',
		'set_fan')`,
		command,
		when.UTC().Format(time.RFC3339Nano),
	)

	return apperr.Wrap(err, "update assets.units")
}

// Register binds commands.create / commands.get.
func Register(r *ops.Registry, m Commands) {
	ops.Register(r, "commands.create", m.create)
	ops.Register(r, "commands.get", m.get)
	ops.Register(r, "commands.list", m.list)
}

// PendingCommands returns the unit's requested / sent Commands, oldest first (UnitDetail.pendingCommands).
func PendingCommands(ctx context.Context, c *ops.Call, unit uuid.UUID) ([]Command, error) {
	rows, err := c.Tx.Query(
		ctx,
		"SELECT "+commandCols+" FROM control.commands WHERE unit_id = $1 AND status IN ('requested','sent') ORDER BY "+
			"requested_at, id",
		unit,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select control.commands")
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

	return out, apperr.Wrap(rows.Err(), "select control.commands")
}
