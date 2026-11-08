package control

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

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// JobVersions reads job versions (Maintenance).
type JobVersions interface {
	JobVersion(ctx context.Context, c *ops.Call, job uuid.UUID) (int, uuid.UUID, bool, error)
}

// Diagnostics is the diagnostic test-run operation set (IR139).
type Diagnostics struct {
	Commands Commands
	Jobs     JobVersions
}

// Run is DiagnosticRun of service-contracts.ts.
type Run struct {
	ID                uuid.UUID       `json:"id"`
	TenantID          uuid.UUID       `json:"tenantId"`
	Version           int             `json:"version"`
	CreatedAt         time.Time       `json:"createdAt"`
	UpdatedAt         time.Time       `json:"updatedAt"`
	JobID             uuid.UUID       `json:"jobId"`
	UnitID            uuid.UUID       `json:"unitId"`
	ActorMembershipID uuid.UUID       `json:"actorMembershipId"`
	StartAction       json.RawMessage `json:"startAction"       swaggertype:"object"`
	EndAction         json.RawMessage `json:"endAction"         swaggertype:"object"`
	DurationMinutes   int             `json:"durationMinutes"`
	Reason            string          `json:"reason"`
	State             string          `json:"state"`
	StartCommandID    uuid.UUID       `json:"startCommandId"`
	EndCommandID      *uuid.UUID      `json:"endCommandId"`
	StartedAt         *time.Time      `json:"startedAt"`
	EndAt             *time.Time      `json:"endAt"`
	FailureCode       *string         `json:"failureCode"`
}

const runCols = `r.id, r.tenant_id, r.version, r.created_at, r.updated_at, r.job_id, r.unit_id, ` +
	`r.actor_membership_id, r.start_action, r.end_action, r.duration_minutes,
	r.reason, r.state, r.start_command_id, r.end_command_id, r.started_at, r.end_at, r.failure_code`

func scanRun(row pgx.Row) (Run, error) {
	var run Run

	err := row.Scan(
		&run.ID,
		&run.TenantID,
		&run.Version,
		&run.CreatedAt,
		&run.UpdatedAt,
		&run.JobID,
		&run.UnitID,
		&run.ActorMembershipID,
		&run.StartAction,
		&run.EndAction,
		&run.DurationMinutes,
		&run.Reason,
		&run.State,
		&run.StartCommandID,
		&run.EndCommandID,
		&run.StartedAt,
		&run.EndAt,
		&run.FailureCode,
	)

	return run, apperr.Wrap(err, "scan (scanRun)")
}

// RunInput is diagnosticRuns.create input.
type RunInput struct {
	JobID               uuid.UUID       `json:"jobId"`
	UnitID              uuid.UUID       `json:"unitId"`
	StartAction         json.RawMessage `json:"startAction"         swaggertype:"object"`
	EndAction           json.RawMessage `json:"endAction"           swaggertype:"object"`
	DurationMinutes     int             `json:"durationMinutes"`
	Reason              string          `json:"reason"`
	ExpectedUnitVersion int             `json:"expectedUnitVersion"`
	ExpectedJobVersion  int             `json:"expectedJobVersion"`
	start, end          UnitAction
}

// Validate implements ops.Validator.
func (in *RunInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil || in.UnitID == uuid.Nil {
		fieldErrs["jobId"] = apperr.KeyRequired
	}

	var ok bool
	if in.start, ok = ParseAction(in.StartAction); !ok {
		fieldErrs["startAction"] = apperr.KeyInvalid
	}

	if in.end, ok = ParseAction(in.EndAction); !ok {
		fieldErrs["endAction"] = apperr.KeyInvalid
	}

	if in.DurationMinutes < 1 || in.DurationMinutes > 15 {
		fieldErrs["durationMinutes"] = apperr.KeyRange
	}

	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fieldErrs[keyReason] = apperr.KeyLength
	}

	if in.ExpectedUnitVersion < 1 || in.ExpectedJobVersion < 1 {
		fieldErrs["expectedUnitVersion"] = apperr.KeyRequired
	}

	return fieldErrs
}

// checkRun checks a diagnostic run before it starts and returns the unit's device: the technician's job covers the
// unit, the unit and the job are at the versions shown, the unit can take both actions and no restriction forbids
// them, and the unit is ready for a command.
func (m Diagnostics) checkRun(
	ctx context.Context,
	call *ops.Call,
	input *RunInput,
) (uuid.UUID, error) {
	err := m.Commands.Access.TechnicianJob(ctx, call, input.JobID, input.UnitID)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "control.TechAccess.TechnicianJob")
	}

	target, found, err := m.Commands.Units.Target(ctx, call, input.UnitID)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "control.Units.Target")
	}

	if !found {
		return uuid.Nil, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	jv, _, _, err := m.Jobs.JobVersion(ctx, call, input.JobID)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "control.JobVersions.JobVersion")
	}

	if target.Version != input.ExpectedUnitVersion || jv != input.ExpectedJobVersion {
		return uuid.Nil, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if !input.start.Supports(target.Caps) {
		return uuid.Nil, apperr.Fields(
			map[string]string{"startAction": apperr.KeyUnsupportedAction},
		)
	}

	if !input.end.Supports(target.Caps) {
		return uuid.Nil, apperr.Fields(map[string]string{"endAction": apperr.KeyUnsupportedAction})
	}

	policy, err := m.Commands.Restrictions.UnitPolicy(ctx, call, input.UnitID)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "control.Restrictions.UnitPolicy")
	}

	if !Allowed(input.start, policy) || !Allowed(input.end, policy) {
		e := apperr.E(apperr.Forbidden, "errors.restriction_active")
		e.FieldErrors = map[string]string{"startAction": "errors.restriction_active"}

		return uuid.Nil, e
	}

	return m.Commands.ready(ctx, call, input.UnitID)
}

// create serves diagnosticRuns.create.
//
//	@Summary		diagnosticRuns.create (write)
//	@ID				diagnosticRuns.create
//	@Description	Authorization: technician:control.diagnose:assigned-valid-job
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR94 technician write table (assignment and work window, jobId required when typed)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T10 · Input versions: \
//	@Description	expectedUnitVersion=UnitDetail.version;expectedJobVersion=JobDetail.version
//	@Tags			diagnosticRuns
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		RunInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Run}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/diagnostic-runs [post]
func (m Diagnostics) create(ctx context.Context, call *ops.Call, input *RunInput) (Run, error) {
	dev, err := m.checkRun(ctx, call, input)
	if err != nil {
		return Run{}, err
	}

	runID := uuid.New()

	var cmd uuid.UUID

	scanErr := call.Tx.QueryRow(ctx, `INSERT INTO control.commands (tenant_id, unit_id, device_id, actor_membership_id,
		source, action, diagnostic_run_id, job_id, reason, status, delivery,
		requested_at, sent_at, expires_at, correlation_id, created_at, updated_at) VALUES
		(current_setting('app.tenant_id')::uuid, $1, $2, $3, 'diagnostic', $4, $5, $6, $7, 'requested', 'not_sent',
		$8, NULL, $9, $10, $8, $8) RETURNING
		id`,
		input.UnitID, dev, call.Principal.MembershipID, []byte(input.StartAction), runID, input.JobID, input.Reason,
		call.Now, call.Now.Add(CommandTTL), call.CorrelationID).
		Scan(&cmd)
	if scanErr != nil {
		return Run{}, apperr.Wrap(scanErr, "insert control.commands")
	}

	run, err := scanRun(
		call.Tx.QueryRow(
			ctx,
			`INSERT INTO control.diagnostic_runs AS r (id, tenant_id, job_id, unit_id, actor_membership_id, start_action,
				end_action, duration_minutes, reason, state,
		start_command_id, created_at, updated_at) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7,
		$8, 'awaiting_start', $9, $10, $10) RETURNING `+runCols,
			runID,
			input.JobID,
			input.UnitID,
			call.Principal.MembershipID,
			[]byte(input.StartAction),
			[]byte(input.EndAction),
			input.DurationMinutes,
			input.Reason,
			cmd,
			call.Now,
		),
	)
	if err != nil {
		return run, err
	}

	call.Emit(
		ops.Event{
			AggregateType: kindCommand,
			AggregateID:   cmd,
			Type:          eventCommandRequested,
			Payload: map[string]any{
				keyUnitID:         input.UnitID,
				"deviceId":        dev,
				"diagnosticRunId": runID,
			},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:      "diagnosticRuns.create",
			TargetKind:  "diagnostic_run",
			TargetID:    runID.String(),
			NextVersion: &run.Version,
			Reason:      input.Reason, PreviousVersion: nil, Result: "", Before: nil, After: nil,
		},
	)

	return run, nil
}

// visible applies the IR139 read scope to a run: technicians see runs of jobs they hold or held an Assignment of,
// read from equipment's projection of maintenance assignments (IR187).
func visible(ctx context.Context, call *ops.Call, run Run) (bool, error) {
	if call.Principal.Role != authz.RoleTechnician {
		return true, nil
	}

	var assigned bool

	err := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.unit_assignment_access WHERE job_id = $1 AND
		technician_membership_id = $2)`, run.JobID, call.Principal.MembershipID).
		Scan(&assigned)

	return assigned, apperr.Wrap(err, "select assets.unit_assignment_access")
}

// RunIDInput is diagnosticRuns.get input.
type RunIDInput struct {
	DiagnosticRunID uuid.UUID `json:"diagnosticRunId"`
}

// Validate implements ops.Validator.
func (in *RunIDInput) Validate() map[string]string {
	if in.DiagnosticRunID == uuid.Nil {
		return map[string]string{"diagnosticRunId": apperr.KeyRequired}
	}

	return nil
}

// get serves diagnosticRuns.get.
//
//	@Summary		diagnosticRuns.get (read)
//	@ID				diagnosticRuns.get
//	@Description	Authorization: technician:control.diagnose:assigned-valid-job | admin:job.read | \
//	@Description	admin:control.execute
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-T10, DD-A02
//	@Tags			diagnosticRuns
//	@Accept			json
//	@Produce		json
//	@Param			diagnosticRunId	path		string	true	"input field diagnosticRunId"
//	@Success		200				{object}	ops.Envelope{data=Run}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/diagnostic-runs/{diagnosticRunId} [get]
func (Diagnostics) get(ctx context.Context, call *ops.Call, in *RunIDInput) (Run, error) {
	run, err := scanRun(
		call.Tx.QueryRow(
			ctx,
			"SELECT "+runCols+" FROM control.diagnostic_runs r WHERE r.id = $1",
			in.DiagnosticRunID,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return run, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return run, err
	}

	ok, visibleErr := visible(ctx, call, run)
	if visibleErr != nil || !ok {
		if visibleErr != nil {
			return run, visibleErr
		}

		return Run{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	return run, nil
}

// RunListInput is diagnosticRuns.list input.
type RunListInput struct {
	UnitID uuid.UUID    `json:"unitId"`
	JobID  *uuid.UUID   `json:"jobId,omitempty"`
	Query  paging.Query `json:"query"`
}

// Validate implements ops.Validator.
func (in *RunListInput) Validate() map[string]string {
	if in.UnitID == uuid.Nil {
		return map[string]string{keyUnitID: apperr.KeyRequired}
	}

	return nil
}

// list serves diagnosticRuns.list.
//
//	@Summary		diagnosticRuns.list (read)
//	@ID				diagnosticRuns.list
//	@Description	Authorization: technician:control.diagnose:assigned-valid-job | admin:job.read | \
//	@Description	admin:control.execute
//	@Description	Validation: D01; SR08/SR09
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-T10, DD-A02 · Query: filters none · sort id,createdAt (default createdAt desc;id asc)
//	@Tags			diagnosticRuns
//	@Accept			json
//	@Produce		json
//	@Param			unitId	query		string	false	"input field unitId"
//	@Param			jobId	query		string	false	"input field jobId"
//	@Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer	false	"page size 1–100, default 25"
//	@Param			sort	query		string	false	"field:direction — fields id,createdAt; default createdAt desc;id asc"
//	@Success		200		{object}	ops.Envelope{data=RunPage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/diagnostic-runs [get]
func (Diagnostics) list(
	ctx context.Context,
	call *ops.Call,
	input *RunListInput,
) (paging.Page[Run], error) {
	err := paging.NoFilters(input.Query, "query.filters")
	if err != nil {
		return paging.Page[Run]{}, apperr.Wrap(err, "paging.NoFilters")
	}

	order, err := paging.OrderBy(
		input.Query.Sort,
		map[string]string{"id": "r.id", "createdAt": "r.created_at"},
		"r.created_at DESC, r.id",
	)
	if err != nil {
		return paging.Page[Run]{}, apperr.Wrap(err, "paging.OrderBy")
	}
	// the cursor binds the unit and job, not the paging fields themselves (the next page carries the cursor)
	window, err := paging.Resolve(input.Query, struct {
		UnitID uuid.UUID  `json:"unitId"`
		JobID  *uuid.UUID `json:"jobId"`
	}{input.UnitID, input.JobID}, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Run]{}, apperr.Wrap(err, "paging.Resolve")
	}

	args := []any{input.UnitID}
	where := "r.unit_id = $1"

	if input.JobID != nil {
		args = append(args, *input.JobID)
		where += " AND r.job_id = $2"
	}

	if call.Principal.Role == authz.RoleTechnician {
		args = append(args, call.Principal.MembershipID)
		where += fmt.Sprintf(
			" AND EXISTS (SELECT 1 FROM assets.unit_assignment_access a WHERE a.job_id = r.job_id AND "+
				"a.technician_membership_id = $%d)",
			len(args),
		)
	}

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM control.diagnostic_runs r WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Run]{}, apperr.Wrap(scanErr, "select control.diagnostic_runs")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM control.diagnostic_runs r WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			runCols,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[Run]{}, apperr.Wrap(err, "select control.diagnostic_runs")
	}
	defer rows.Close()

	items := []Run{}

	for rows.Next() {
		x, err := scanRun(rows)
		if err != nil {
			return paging.Page[Run]{}, err
		}

		items = append(items, x)
	}

	return paging.Page[Run]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, apperr.Wrap(rows.Err(), "select control.diagnostic_runs")
}

// runAcknowledged moves a run when its start or end command is acknowledged (IR139 item 2).
func runAcknowledged(ctx context.Context, txn pgx.Tx, command uuid.UUID, when time.Time) error {
	_, execErr := txn.Exec(
		ctx,
		`UPDATE control.diagnostic_runs SET state = 'running', started_at = $2::timestamptz, end_at = $2::timestamptz +
			make_interval(mins => duration_minutes),
		version = version + 1, updated_at = $2 WHERE start_command_id = $1 AND state = 'awaiting_start'`,
		command,
		when,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update control.diagnostic_runs")
	}

	_, err := txn.Exec(
		ctx,
		`UPDATE control.diagnostic_runs SET state = 'completed', version = version + 1, updated_at = $2 WHERE end_command_id =
			$1 AND state = 'end_requested'`,
		command,
		when,
	)

	return apperr.Wrap(err, "update control.diagnostic_runs")
}

// RunCommandFailed moves a run when the device rejects its start or end command: the start becomes start_failed and
// the end end_failed, both with the device's code, and the unit is free again (implementation-contracts.md, the
// DiagnosticRun table: "Start failure" and "Failure or expiry").
func RunCommandFailed(
	ctx context.Context,
	txn pgx.Tx,
	command uuid.UUID,
	code string,
	when time.Time,
) error {
	_, err := txn.Exec(
		ctx,
		`UPDATE control.diagnostic_runs SET state = 'start_failed', failure_code = $2, version = version + 1,
			updated_at = $3 WHERE start_command_id = $1 AND state = 'awaiting_start'`,
		command,
		code,
		when,
	)
	if err != nil {
		return apperr.Wrap(err, "update control.diagnostic_runs")
	}

	_, err = txn.Exec(
		ctx,
		`UPDATE control.diagnostic_runs SET state = 'end_failed', failure_code = $2, version = version + 1,
			updated_at = $3 WHERE end_command_id = $1 AND state = 'end_requested'`,
		command,
		code,
		when,
	)

	return apperr.Wrap(err, "update control.diagnostic_runs")
}

// AdvanceRuns is the worker step of IR139 item 2: failed starts / ends from expired commands, and end commands at
// endAt.
func AdvanceRuns(ctx context.Context, call *ops.Call, commands Commands) (int, error) {
	advanced, err := expireRunCommands(ctx, call)
	if err != nil {
		return advanced, err
	}

	due, err := dueRuns(ctx, call)
	if err != nil {
		return advanced, err
	}

	for _, run := range due {
		end, err := endOutcome(ctx, call, commands, run)
		if err != nil {
			return advanced, err
		}

		_, err = call.Tx.Exec(
			ctx,
			`UPDATE control.diagnostic_runs SET state = $2, failure_code = NULLIF($3, ''), end_command_id = COALESCE($4,
				end_command_id), version = version + 1, updated_at = $5
			WHERE id = $1`,
			run.ID,
			end.state,
			end.code,
			end.command,
			call.Now,
		)
		if err != nil {
			return advanced, apperr.Wrap(err, "update control.diagnostic_runs")
		}

		advanced++
	}

	return advanced, nil
}

// expireRunCommands fails the runs whose start or end command expired (TIMEOUT) and returns how many it changed.
func expireRunCommands(ctx context.Context, call *ops.Call) (int, error) {
	tag, err := call.Tx.Exec(
		ctx,
		`UPDATE control.diagnostic_runs r SET state = 'start_failed', failure_code = 'TIMEOUT', version = r.version + 1,
			updated_at = $1
		FROM control.commands k WHERE k.id = r.start_command_id AND k.status = 'expired' AND r.state = 'awaiting_start'`,
		call.Now,
	)
	if err != nil {
		return 0, apperr.Wrap(err, "update control.diagnostic_runs")
	}

	changed := int(tag.RowsAffected())

	tag, err = call.Tx.Exec(
		ctx,
		`UPDATE control.diagnostic_runs r SET state = 'end_failed', failure_code = 'TIMEOUT', version = r.version + 1,
			updated_at = $1
		FROM control.commands k WHERE k.id = r.end_command_id AND k.status = 'expired' AND r.state = 'end_requested'`,
		call.Now,
	)
	if err != nil {
		return changed, apperr.Wrap(err, "update control.diagnostic_runs")
	}

	return changed + int(tag.RowsAffected()), nil
}

// dueRuns are the running runs whose end time has come, locked for the tick.
func dueRuns(ctx context.Context, call *ops.Call) ([]Run, error) {
	rows, err := call.Tx.Query(
		ctx,
		"SELECT "+runCols+" FROM control.diagnostic_runs r WHERE r.state = 'running' AND r.end_at <= $1 FOR UPDATE",
		call.Now,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select control.diagnostic_runs")
	}
	defer rows.Close()

	var due []Run

	for rows.Next() {
		run, err := scanRun(rows)
		if err != nil {
			return nil, err
		}

		due = append(due, run)
	}

	return due, apperr.Wrap(rows.Err(), "select control.diagnostic_runs")
}

// runEnd is how a due diagnostic run ends: its state, its failure code and its end command.
type runEnd struct {
	state, code string
	command     *uuid.UUID
}

// endOutcome is how a due run ends: blocked when a restriction forbids its end action, failed while its device is
// offline, or requested with its end command.
func endOutcome(ctx context.Context, call *ops.Call, commands Commands, run Run) (runEnd, error) {
	end, _ := ParseAction(run.EndAction)

	policy, err := commands.Restrictions.UnitPolicy(ctx, call, run.UnitID)
	if err != nil {
		return runEnd{}, apperr.Wrap(err, "control.Restrictions.UnitPolicy")
	}

	if !Allowed(end, policy) {
		return runEnd{state: "end_blocked", code: "FORBIDDEN", command: nil}, nil
	}

	dev, conn, power, bound, err := commands.Devices.BoundDevice(ctx, call, run.UnitID)
	if err != nil {
		return runEnd{}, apperr.Wrap(err, "control.Devices.BoundDevice")
	}

	if deliverable := Deliverable(bound, conn, power) == nil; !deliverable {
		return runEnd{state: "end_failed", code: "OFFLINE", command: nil}, nil
	}

	var endCommandID uuid.UUID

	err = call.Tx.QueryRow(ctx, `INSERT INTO control.commands (tenant_id, unit_id, device_id, actor_membership_id,
		source,
		action, diagnostic_run_id, job_id, reason, status, delivery,
	requested_at, sent_at, expires_at, correlation_id, created_at, updated_at) VALUES ($1, $2, $3, $4, 'diagnostic', $5,
	$6, $7, $8, 'requested', 'not_sent', $9, NULL, $10, $11, $9, $9) RETURNING id`,
		run.TenantID, run.UnitID, dev, run.ActorMembershipID, []byte(run.EndAction), run.ID, run.JobID, run.Reason,
		call.Now,
		call.Now.Add(CommandTTL), "tick-"+call.Now.Format(time.RFC3339)).
		Scan(&endCommandID)
	if err != nil {
		return runEnd{}, apperr.Wrap(err, "insert control.commands")
	}

	return runEnd{state: "end_requested", code: "", command: &endCommandID}, nil
}

// RegisterDiagnostics binds the diagnostic run operations.
func RegisterDiagnostics(r *ops.Registry, m Diagnostics) {
	ops.Register(r, "diagnosticRuns.create", m.create)
	ops.Register(r, "diagnosticRuns.get", m.get)
	ops.Register(r, "diagnosticRuns.list", m.list)
}
