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
	StartAction       json.RawMessage `json:"startAction" swaggertype:"object"`
	EndAction         json.RawMessage `json:"endAction" swaggertype:"object"`
	DurationMinutes   int             `json:"durationMinutes"`
	Reason            string          `json:"reason"`
	State             string          `json:"state"`
	StartCommandID    uuid.UUID       `json:"startCommandId"`
	EndCommandID      *uuid.UUID      `json:"endCommandId"`
	StartedAt         *time.Time      `json:"startedAt"`
	EndAt             *time.Time      `json:"endAt"`
	FailureCode       *string         `json:"failureCode"`
}

const runCols = `r.id, r.tenant_id, r.version, r.created_at, r.updated_at, r.job_id, r.unit_id, r.actor_membership_id, r.start_action, r.end_action, r.duration_minutes,
	r.reason, r.state, r.start_command_id, r.end_command_id, r.started_at, r.end_at, r.failure_code`

func scanRun(row pgx.Row) (Run, error) {
	var x Run
	err := row.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.JobID, &x.UnitID, &x.ActorMembershipID, &x.StartAction, &x.EndAction, &x.DurationMinutes,
		&x.Reason, &x.State, &x.StartCommandID, &x.EndCommandID, &x.StartedAt, &x.EndAt, &x.FailureCode)
	return x, err
}

// RunInput is diagnosticRuns.create input.
type RunInput struct {
	JobID               uuid.UUID       `json:"jobId"`
	UnitID              uuid.UUID       `json:"unitId"`
	StartAction         json.RawMessage `json:"startAction" swaggertype:"object"`
	EndAction           json.RawMessage `json:"endAction" swaggertype:"object"`
	DurationMinutes     int             `json:"durationMinutes"`
	Reason              string          `json:"reason"`
	ExpectedUnitVersion int             `json:"expectedUnitVersion"`
	ExpectedJobVersion  int             `json:"expectedJobVersion"`
	start, end          UnitAction
}

// Validate implements ops.Validator.
func (in *RunInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil || in.UnitID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	var ok bool
	if in.start, ok = ParseAction(in.StartAction); !ok {
		fe["startAction"] = "error.invalid"
	}
	if in.end, ok = ParseAction(in.EndAction); !ok {
		fe["endAction"] = "error.invalid"
	}
	if in.DurationMinutes < 1 || in.DurationMinutes > 15 {
		fe["durationMinutes"] = "error.range"
	}
	in.Reason = strings.TrimSpace(in.Reason)
	if n := utf8.RuneCountInString(in.Reason); n < 1 || n > 1000 {
		fe["reason"] = "error.length"
	}
	if in.ExpectedUnitVersion < 1 || in.ExpectedJobVersion < 1 {
		fe["expectedUnitVersion"] = "error.required"
	}
	return fe
}

// @Summary		diagnosticRuns.create (write)
// @ID				diagnosticRuns.create
// @Description	Authorization: technician:control.diagnose:assigned-valid-job
// @Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; IR94 technician write table (assignment and work window, jobId required when typed)
// @Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
// @Description	Design: DD-T10 · Input versions: expectedUnitVersion=UnitDetail.version;expectedJobVersion=JobDetail.version
// @Tags			diagnosticRuns
// @Accept			json
// @Produce		json
// @Param			Idempotency-Key	header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
// @Param			request			body		RunInput	true	"input"
// @Success		200				{object}	ops.Envelope{data=Run}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/diagnostic-runs [post]
func (m Diagnostics) create(ctx context.Context, c *ops.Call, in *RunInput) (Run, error) {
	if err := m.Commands.Access.TechnicianJob(ctx, c, in.JobID, in.UnitID); err != nil {
		return Run{}, err
	}
	t, found, err := m.Commands.Units.Target(ctx, c, in.UnitID)
	if err != nil {
		return Run{}, err
	}
	if !found {
		return Run{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	jv, _, _, err := m.Jobs.JobVersion(ctx, c, in.JobID)
	if err != nil {
		return Run{}, err
	}
	if t.Version != in.ExpectedUnitVersion || jv != in.ExpectedJobVersion {
		return Run{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if !in.start.Supports(t.Caps) {
		return Run{}, apperr.Fields(map[string]string{"startAction": "error.unsupportedAction"})
	}
	if !in.end.Supports(t.Caps) {
		return Run{}, apperr.Fields(map[string]string{"endAction": "error.unsupportedAction"})
	}
	policy, err := m.Commands.Restrictions.UnitPolicy(ctx, c, in.UnitID)
	if err != nil {
		return Run{}, err
	}
	if !Allowed(in.start, policy) || !Allowed(in.end, policy) {
		e := apperr.E(apperr.Forbidden, "errors.restriction_active")
		e.FieldErrors = map[string]string{"startAction": "errors.restriction_active"}
		return Run{}, e
	}
	dev, err := m.Commands.ready(ctx, c, in.UnitID)
	if err != nil {
		return Run{}, err
	}
	runID := uuid.New()
	var cmd uuid.UUID
	if err := c.Tx.QueryRow(ctx, `INSERT INTO control.commands (tenant_id, unit_id, device_id, actor_membership_id, source, action, diagnostic_run_id, job_id, reason, status, delivery,
		requested_at, sent_at, expires_at, correlation_id, created_at, updated_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, 'diagnostic', $4, $5, $6, $7, 'sent', 'sent',
		$8, $8, $9, $10, $8, $8) RETURNING id`, in.UnitID, dev, c.Principal.MembershipID, []byte(in.StartAction), runID, in.JobID, in.Reason, c.Now, c.Now.Add(CommandTTL), c.CorrelationID).Scan(&cmd); err != nil {
		return Run{}, err
	}
	x, err := scanRun(c.Tx.QueryRow(ctx, `INSERT INTO control.diagnostic_runs AS r (id, tenant_id, job_id, unit_id, actor_membership_id, start_action, end_action, duration_minutes, reason, state,
		start_command_id, created_at, updated_at) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7, $8, 'awaiting_start', $9, $10, $10) RETURNING `+runCols,
		runID, in.JobID, in.UnitID, c.Principal.MembershipID, []byte(in.StartAction), []byte(in.EndAction), in.DurationMinutes, in.Reason, cmd, c.Now))
	if err != nil {
		return x, err
	}
	c.Emit(ops.Event{AggregateType: "command", AggregateID: cmd, Type: "CommandRequested", Payload: map[string]any{"unitId": in.UnitID, "deviceId": dev, "diagnosticRunId": runID}})
	c.Audit(ops.AuditEntry{Action: "diagnosticRuns.create", TargetKind: "diagnostic_run", TargetID: runID.String(), NextVersion: &x.Version, Reason: in.Reason})
	return x, nil
}

// visible applies the IR139 read scope to a run: technicians see runs of jobs they hold or held an Assignment of,
// read from equipment's projection of maintenance assignments (IR187).
func visible(ctx context.Context, c *ops.Call, run Run) (bool, error) {
	if c.Principal.Role != "technician" {
		return true, nil
	}
	var ok bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM assets.unit_assignment_access WHERE job_id = $1 AND technician_membership_id = $2)`, run.JobID, c.Principal.MembershipID).Scan(&ok)
	return ok, err
}

// RunIDInput is diagnosticRuns.get input.
type RunIDInput struct {
	DiagnosticRunID uuid.UUID `json:"diagnosticRunId"`
}

// Validate implements ops.Validator.
func (in *RunIDInput) Validate() map[string]string {
	if in.DiagnosticRunID == uuid.Nil {
		return map[string]string{"diagnosticRunId": "error.required"}
	}
	return nil
}

// @Summary		diagnosticRuns.get (read)
// @ID				diagnosticRuns.get
// @Description	Authorization: technician:control.diagnose:assigned-valid-job | admin:job.read | admin:control.execute
// @Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-T10, DD-A02
// @Tags			diagnosticRuns
// @Accept			json
// @Produce		json
// @Param			diagnosticRunId	path		string	true	"input field diagnosticRunId"
// @Success		200				{object}	ops.Envelope{data=Run}
// @Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504				{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/diagnostic-runs/{diagnosticRunId} [get]
func (m Diagnostics) get(ctx context.Context, c *ops.Call, in *RunIDInput) (Run, error) {
	x, err := scanRun(c.Tx.QueryRow(ctx, "SELECT "+runCols+" FROM control.diagnostic_runs r WHERE r.id = $1", in.DiagnosticRunID))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return x, err
	}
	if ok, err := visible(ctx, c, x); err != nil || !ok {
		if err != nil {
			return x, err
		}
		return Run{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	return x, nil
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
		return map[string]string{"unitId": "error.required"}
	}
	return nil
}

// @Summary		diagnosticRuns.list (read)
// @ID				diagnosticRuns.list
// @Description	Authorization: technician:control.diagnose:assigned-valid-job | admin:job.read | admin:control.execute
// @Description	Validation: D01; SR08/SR09
// @Description	Recovery: D04: retry only UNAVAILABLE, at most twice
// @Description	Design: DD-T10, DD-A02 · Query: filters none · sort id,createdAt (default createdAt desc;id asc)
// @Tags			diagnosticRuns
// @Accept			json
// @Produce		json
// @Param			unitId	query		string	false	"input field unitId"
// @Param			jobId	query		string	false	"input field jobId"
// @Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
// @Param			limit	query		integer	false	"page size 1–100, default 25"
// @Param			sort	query		string	false	"field:direction — fields id,createdAt; default createdAt desc;id asc"
// @Success		200		{object}	ops.Envelope{data=RunPage}
// @Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
// @Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
// @Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
// @Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
// @Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
// @Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
// @Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
// @Failure		504		{object}	apperr.DomainError	"TIMEOUT"
// @Security		BearerAuth
// @Router			/v1/diagnostic-runs [get]
func (m Diagnostics) list(ctx context.Context, c *ops.Call, in *RunListInput) (paging.Page[Run], error) {
	if err := paging.NoFilters(in.Query, "query.filters"); err != nil {
		return paging.Page[Run]{}, err
	}
	order, err := paging.OrderBy(in.Query.Sort, map[string]string{"id": "r.id", "createdAt": "r.created_at"}, "r.created_at DESC, r.id")
	if err != nil {
		return paging.Page[Run]{}, err
	}
	// the cursor binds the unit and job, not the paging fields themselves (the next page carries the cursor)
	w, err := paging.Resolve(in.Query, struct {
		UnitID uuid.UUID  `json:"unitId"`
		JobID  *uuid.UUID `json:"jobId"`
	}{in.UnitID, in.JobID}, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Run]{}, err
	}
	args := []any{in.UnitID}
	where := "r.unit_id = $1"
	if in.JobID != nil {
		args = append(args, *in.JobID)
		where += " AND r.job_id = $2"
	}
	if c.Principal.Role == "technician" {
		args = append(args, c.Principal.MembershipID)
		where += fmt.Sprintf(" AND EXISTS (SELECT 1 FROM assets.unit_assignment_access a WHERE a.job_id = r.job_id AND a.technician_membership_id = $%d)", len(args))
	}
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM control.diagnostic_runs r WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Run]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM control.diagnostic_runs r WHERE %s ORDER BY %s LIMIT %d OFFSET %d", runCols, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Run]{}, err
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
	return paging.Page[Run]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, rows.Err()
}

// runAcknowledged moves a run when its start or end command is acknowledged (IR139 item 2).
func runAcknowledged(ctx context.Context, tx pgx.Tx, command uuid.UUID, at time.Time) error {
	if _, err := tx.Exec(ctx, `UPDATE control.diagnostic_runs SET state = 'running', started_at = $2::timestamptz, end_at = $2::timestamptz + make_interval(mins => duration_minutes),
		version = version + 1, updated_at = $2 WHERE start_command_id = $1 AND state = 'awaiting_start'`, command, at); err != nil {
		return err
	}
	_, err := tx.Exec(ctx, `UPDATE control.diagnostic_runs SET state = 'completed', version = version + 1, updated_at = $2 WHERE end_command_id = $1 AND state = 'end_requested'`, command, at)
	return err
}

// AdvanceRuns is the worker step of IR139 item 2: failed starts / ends from expired commands, and end commands at endAt.
func AdvanceRuns(ctx context.Context, c *ops.Call, m Commands) (int, error) {
	n := 0
	tag, err := c.Tx.Exec(ctx, `UPDATE control.diagnostic_runs r SET state = 'start_failed', failure_code = 'TIMEOUT', version = r.version + 1, updated_at = $1
		FROM control.commands k WHERE k.id = r.start_command_id AND k.status = 'expired' AND r.state = 'awaiting_start'`, c.Now)
	if err != nil {
		return 0, err
	}
	n += int(tag.RowsAffected())
	tag, err = c.Tx.Exec(ctx, `UPDATE control.diagnostic_runs r SET state = 'end_failed', failure_code = 'TIMEOUT', version = r.version + 1, updated_at = $1
		FROM control.commands k WHERE k.id = r.end_command_id AND k.status = 'expired' AND r.state = 'end_requested'`, c.Now)
	if err != nil {
		return n, err
	}
	n += int(tag.RowsAffected())
	rows, err := c.Tx.Query(ctx, "SELECT "+runCols+" FROM control.diagnostic_runs r WHERE r.state = 'running' AND r.end_at <= $1 FOR UPDATE", c.Now)
	if err != nil {
		return n, err
	}
	var due []Run
	for rows.Next() {
		x, err := scanRun(rows)
		if err != nil {
			rows.Close()
			return n, err
		}
		due = append(due, x)
	}
	rows.Close()
	for _, x := range due {
		end, _ := ParseAction(x.EndAction)
		policy, err := m.Restrictions.UnitPolicy(ctx, c, x.UnitID)
		if err != nil {
			return n, err
		}
		state, code := "", ""
		var cmd *uuid.UUID
		if !Allowed(end, policy) {
			state, code = "end_blocked", "FORBIDDEN"
		} else if dev, conn, power, bound, err := m.Devices.BoundDevice(ctx, c, x.UnitID); err != nil {
			return n, err
		} else if Deliverable(bound, conn, power) != nil {
			state, code = "end_failed", "OFFLINE"
		} else {
			var id uuid.UUID
			if err := c.Tx.QueryRow(ctx, `INSERT INTO control.commands (tenant_id, unit_id, device_id, actor_membership_id, source, action, diagnostic_run_id, job_id, reason, status, delivery,
				requested_at, sent_at, expires_at, correlation_id, created_at, updated_at) VALUES ($1, $2, $3, $4, 'diagnostic', $5, $6, $7, $8, 'sent', 'sent', $9, $9, $10, $11, $9, $9) RETURNING id`,
				x.TenantID, x.UnitID, dev, x.ActorMembershipID, []byte(x.EndAction), x.ID, x.JobID, x.Reason, c.Now, c.Now.Add(CommandTTL), "tick-"+c.Now.Format(time.RFC3339)).Scan(&id); err != nil {
				return n, err
			}
			state, cmd = "end_requested", &id
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE control.diagnostic_runs SET state = $2, failure_code = NULLIF($3, ''), end_command_id = COALESCE($4, end_command_id), version = version + 1, updated_at = $5
			WHERE id = $1`, x.ID, state, code, cmd, c.Now); err != nil {
			return n, err
		}
		n++
	}
	return n, nil
}

// RegisterDiagnostics binds the diagnostic run operations.
func RegisterDiagnostics(r *ops.Registry, m Diagnostics) {
	ops.Register(r, "diagnosticRuns.create", m.create)
	ops.Register(r, "diagnosticRuns.get", m.get)
	ops.Register(r, "diagnosticRuns.list", m.list)
}
