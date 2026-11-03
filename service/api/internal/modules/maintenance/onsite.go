package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// TimeOnSite is TimeOnSite of service-contracts.ts.
type TimeOnSite struct {
	ArrivedAt     *time.Time `json:"arrivedAt"`
	CheckInMethod *string    `json:"checkInMethod"`
	CheckInReason *string    `json:"checkInReason"`
	Distance      *float64   `json:"distanceMeters"`
	StartedAt     *time.Time `json:"startedAt"`
	Pauses        []Pause    `json:"pauses"`
	FinishedAt    *time.Time `json:"finishedAt"`
	OnSiteMinutes *int       `json:"onSiteMinutes"`
}

// Pause is one TimeOnSite.pauses entry.
type Pause struct {
	From time.Time  `json:"from"`
	To   *time.Time `json:"to"`
}

// OnSite is the technician on-site operation set (IR125).
type OnSite struct {
	Jobs   Jobs
	Access Access
}

// lockOwn loads the caller's job for update, applies the IR94 work-window rule and the version.
func (m OnSite) lockOwn(
	ctx context.Context,
	call *ops.Call,
	jobID uuid.UUID,
) (string, TimeOnSite, error) {
	var (
		status  string
		unit    uuid.UUID
		version int
		raw     []byte
	)

	err := call.Tx.QueryRow(ctx, `SELECT status, unit_id, version, time_on_site FROM maintenance.jobs WHERE id = $1 FOR
		UPDATE`, jobID).
		Scan(&status, &unit, &version, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", TimeOnSite{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return "", TimeOnSite{}, apperr.Wrap(err, "select maintenance.jobs")
	}

	technicianJobErr := m.Access.TechnicianJob(ctx, call, jobID, unit)
	if technicianJobErr != nil {
		return "", TimeOnSite{}, technicianJobErr
	}

	if version != *call.ExpectedVersion {
		return "", TimeOnSite{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	timeOnSite := TimeOnSite{
		Pauses:        []Pause{},
		ArrivedAt:     nil,
		CheckInMethod: nil,
		CheckInReason: nil,
		Distance:      nil,
		StartedAt:     nil,
		FinishedAt:    nil,
		OnSiteMinutes: nil,
	}
	if len(raw) > 0 && string(raw) != jsonNull {
		err := json.Unmarshal(raw, &timeOnSite)
		if err != nil {
			return "", timeOnSite, apperr.Wrap(err, "json.Unmarshal")
		}
	}

	if timeOnSite.Pauses == nil {
		timeOnSite.Pauses = []Pause{}
	}

	return status, timeOnSite, nil
}

func (m OnSite) save(
	ctx context.Context,
	call *ops.Call,
	jobID uuid.UUID,
	status string,
	timeOnSite TimeOnSite,
	action, event, reason string,
) (Job, error) {
	raw, _ := json.Marshal(timeOnSite)

	_, err := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET status = $2, time_on_site = $3, started_at = COALESCE(started_at, $4), version = version
			+ 1,
		updated_at = platform.app_now() WHERE id = $1`,
		jobID,
		status,
		raw,
		timeOnSite.StartedAt,
	)
	if err != nil {
		return Job{}, apperr.Wrap(err, "update maintenance.jobs")
	}

	err = m.Jobs.event(ctx, call, jobID, action, nil)
	if err != nil {
		return Job{}, err
	}

	job, err := m.Jobs.scoped(ctx, call, jobID)
	if err != nil {
		return job, err
	}

	call.Emit(outboxEvent("job", jobID, event, map[string]any{fieldStatus: job.Status}))
	call.Audit(
		auditEntry("jobs."+action[4:], "job", jobID, call.ExpectedVersion, &job.Version, reason),
	)

	return job, nil
}

// StartInput is jobs.start input.
type StartInput struct {
	JobID          uuid.UUID `json:"jobId"`
	StartConfirmed *bool     `json:"startConfirmed"`
}

// Validate implements ops.Validator.
func (in *StartInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.StartConfirmed == nil || !*in.StartConfirmed {
		fieldErrs["startConfirmed"] = apperr.KeyRequired
	}

	return fieldErrs
}

// start serves jobs.start.
//
//	@Summary		jobs.start (write)
//	@ID				jobs.start
//	@Description	Authorization: technician:assigned-valid-job
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR94 technician write table (assignment and work window, jobId required when typed)
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T08, DD-T13
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		StartInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=MaintenanceJob}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/start [post]
func (m OnSite) start(ctx context.Context, c *ops.Call, in *StartInput) (MaintenanceJob, error) {
	return written(m.startJob(ctx, c, in))
}

// startJob is start answering the job with its detail extras.
func (m OnSite) startJob(ctx context.Context, call *ops.Call, input *StartInput) (Job, error) {
	status, timeOnSite, err := m.lockOwn(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if status != statusAssigned {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	now := call.Now
	timeOnSite.StartedAt = &now

	return m.save(
		ctx,
		call,
		input.JobID,
		statusInProgress,
		timeOnSite,
		"job.started",
		"JobStarted",
		"",
	)
}

// CheckInInput is jobs.checkIn input.
type CheckInInput struct {
	JobID          uuid.UUID  `json:"jobId"`
	Method         string     `json:"method"`
	DistanceMeters *float64   `json:"distanceMeters"`
	QRUnitID       *uuid.UUID `json:"qrUnitId"`
	Reason         *string    `json:"reason,omitempty"`
}

// Validate implements ops.Validator (IR111: location_qr within 200 m, manual needs a reason).
func (in *CheckInInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	switch in.Method {
	case "location_qr":
		if in.DistanceMeters == nil || *in.DistanceMeters < 0 || *in.DistanceMeters > 200 {
			fieldErrs["distanceMeters"] = "errors.too_far"
		}

		if in.QRUnitID == nil {
			fieldErrs["qrUnitId"] = apperr.KeyRequired
		}
	case "manual":
		if in.Reason == nil {
			fieldErrs[fieldReason] = apperr.KeyRequired
		} else {
			trimmedReason(in.Reason, 1000, fieldReason, fieldErrs)
		}

		if in.DistanceMeters != nil && *in.DistanceMeters < 0 {
			fieldErrs["distanceMeters"] = apperr.KeyRange
		}
	default:
		fieldErrs["method"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// checkIn serves jobs.checkIn.
//
//	@Summary		jobs.checkIn (write)
//	@ID				jobs.checkIn
//	@Description	Authorization: technician:assigned-valid-job
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 within the work window; manual method requires reason 1–1000; location within 200 m for \
//	@Description	location_qr
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T13
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target job, read jobs.get)"
//	@Param			jobId				path		string			true	"input field jobId"
//	@Param			request				body		CheckInInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=MaintenanceJob}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/check-in [post]
func (m OnSite) checkIn(
	ctx context.Context,
	c *ops.Call,
	in *CheckInInput,
) (MaintenanceJob, error) {
	return written(m.checkInJob(ctx, c, in))
}

// checkInJob is checkIn answering the job with its detail extras.
func (m OnSite) checkInJob(ctx context.Context, call *ops.Call, input *CheckInInput) (Job, error) {
	status, timeOnSite, err := m.lockOwn(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if input.Method == "location_qr" {
		var unit uuid.UUID

		err := call.Tx.QueryRow(ctx, `SELECT unit_id FROM maintenance.jobs WHERE id = $1`, input.JobID).
			Scan(&unit)
		if err != nil {
			return Job{}, apperr.Wrap(err, "select maintenance.jobs")
		}

		if *input.QRUnitID != unit {
			return Job{}, apperr.Fields(map[string]string{"qrUnitId": "errors.qr_unit_mismatch"})
		}
	}

	if status != statusAssigned && (status != statusInProgress || timeOnSite.ArrivedAt != nil) {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	now := call.Now
	method := input.Method

	timeOnSite.ArrivedAt, timeOnSite.CheckInMethod, timeOnSite.Distance = &now, &method, input.DistanceMeters
	if input.Method == "manual" {
		timeOnSite.CheckInReason = input.Reason
	}

	if timeOnSite.StartedAt == nil {
		timeOnSite.StartedAt = &now
	}

	return m.save(
		ctx,
		call,
		input.JobID,
		statusInProgress,
		timeOnSite,
		"job.checked_in",
		"JobCheckedIn",
		deref(input.Reason),
	)
}

// PauseInput is jobs.pauseWork input.
type PauseInput struct {
	JobID  uuid.UUID `json:"jobId"`
	Paused *bool     `json:"paused"`
}

// Validate implements ops.Validator.
func (in *PauseInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	if in.Paused == nil {
		fieldErrs["paused"] = apperr.KeyRequired
	}

	return fieldErrs
}

// pause serves jobs.pauseWork.
//
//	@Summary		jobs.pauseWork (write)
//	@ID				jobs.pauseWork
//	@Description	Authorization: technician:assigned-valid-job
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T14
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		PauseInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=MaintenanceJob}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/pause-work [post]
func (m OnSite) pause(ctx context.Context, c *ops.Call, in *PauseInput) (MaintenanceJob, error) {
	return written(m.pauseJob(ctx, c, in))
}

// pauseJob is pause answering the job with its detail extras.
func (m OnSite) pauseJob(ctx context.Context, call *ops.Call, input *PauseInput) (Job, error) {
	status, timeOnSite, err := m.lockOwn(ctx, call, input.JobID)
	if err != nil {
		return Job{}, err
	}

	if status != statusInProgress {
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	open := len(timeOnSite.Pauses) > 0 && timeOnSite.Pauses[len(timeOnSite.Pauses)-1].To == nil
	now := call.Now

	switch {
	case *input.Paused && open, !*input.Paused && !open:
		return Job{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	case *input.Paused:
		timeOnSite.Pauses = append(timeOnSite.Pauses, Pause{From: now, To: nil})
	default:
		timeOnSite.Pauses[len(timeOnSite.Pauses)-1].To = &now
	}

	action := "job.resumed_work"
	if *input.Paused {
		action = "job.paused"
	}

	return m.save(ctx, call, input.JobID, status, timeOnSite, action, "JobPauseChanged", "")
}

// The acknowledgements of an assignment (Assignment.acknowledgement): pending until the technician answers, then
// accepted or cant_make, which is also the decision that answers it so.
const (
	ackPending  = "pending"
	ackAccepted = "accepted"
	ackCantMake = "cant_make"
)

// AckInput is jobs.acknowledgeAssignment input.
type AckInput struct {
	JobID           uuid.UUID `json:"jobId"`
	Decision        string    `json:"decision"`
	Reason          *string   `json:"reason,omitempty"`
	AlternativeSlot *Slot     `json:"alternativeSlot,omitempty"`
}

// Validate implements ops.Validator (IR113 item 8).
func (in *AckInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.JobID == uuid.Nil {
		fieldErrs[fieldJobID] = apperr.KeyRequired
	}

	switch in.Decision {
	case decisionAccept:
		if in.Reason != nil || in.AlternativeSlot != nil {
			fieldErrs["decision"] = "error.fieldsNotAllowed"
		}
	case ackCantMake:
		if in.Reason == nil {
			fieldErrs[fieldReason] = apperr.KeyRequired
		} else {
			trimmedReason(in.Reason, 1000, fieldReason, fieldErrs)
		}

		if in.AlternativeSlot != nil &&
			!in.AlternativeSlot.StartAt.Before(in.AlternativeSlot.EndAt) {
			fieldErrs["alternativeSlot"] = apperr.KeyRange
		}
	default:
		fieldErrs["decision"] = apperr.KeyInvalid
	}

	return fieldErrs
}

// acknowledge serves jobs.acknowledgeAssignment.
//
//	@Summary		jobs.acknowledgeAssignment (write)
//	@ID				jobs.acknowledgeAssignment
//	@Description	Authorization: technician:assigned-valid-job
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR113 own active Assignment with acknowledgement=pending; cant_make needs reason 1–1000; \
//	@Description	alternativeSlot optional
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-T08
//	@Tags			jobs
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string		true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer		true	"all: required (target job, read jobs.get;jobs.list)"
//	@Param			jobId				path		string		true	"input field jobId"
//	@Param			request				body		AckInput	true	"input; the path parameters come from the route"
//	@Success		200					{object}	ops.Envelope{data=Assignment}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/jobs/{jobId}/assignment/acknowledge [post]
func (m OnSite) acknowledge(
	ctx context.Context,
	call *ops.Call,
	input *AckInput,
) (Assignment, error) {
	var (
		aid uuid.UUID
		ack string
		end time.Time
	)

	err := call.Tx.QueryRow(ctx, `SELECT id, acknowledgement, upper(scheduled) FROM maintenance.assignments WHERE job_id = $1
		AND technician_membership_id = $2
		AND status = 'active' AND created_at <= $3 FOR UPDATE`, input.JobID, call.Principal.MembershipID, call.Now).
		Scan(&aid, &ack, &end)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return Assignment{}, apperr.Wrap(err, "select maintenance.assignments")
	}

	if !call.Now.Before(end) {
		return Assignment{}, apperr.E(apperr.Forbidden, "errors.assignment_ended")
	}

	var version int

	scanErr := call.Tx.QueryRow(ctx, `SELECT version FROM maintenance.jobs WHERE id = $1 FOR UPDATE`, input.JobID).
		Scan(&version)
	if scanErr != nil {
		return Assignment{}, apperr.Wrap(scanErr, "select maintenance.jobs")
	}

	if version != *call.ExpectedVersion {
		return Assignment{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if ack != ackPending {
		return Assignment{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	value := ackAccepted

	var altStart, altEnd *time.Time

	if input.Decision == ackCantMake {
		value = ackCantMake

		if input.AlternativeSlot != nil {
			altStart, altEnd = &input.AlternativeSlot.StartAt, &input.AlternativeSlot.EndAt
		}
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE maintenance.assignments SET acknowledgement = $2, acknowledged_at = $3, cant_make_reason = $4,
		alternative_slot = CASE WHEN $5::timestamptz IS NULL THEN NULL ELSE tstzrange($5, $6) END, version = version + 1,
		updated_at = platform.app_now() WHERE id = $1`,
		aid,
		value,
		call.Now,
		input.Reason,
		altStart,
		altEnd,
	)
	if execErr != nil {
		return Assignment{}, apperr.Wrap(execErr, "update maintenance.assignments")
	}

	_, execErr = call.Tx.Exec(
		ctx,
		`UPDATE maintenance.jobs SET version = version + 1, updated_at = platform.app_now() WHERE id = $1`,
		input.JobID,
	)
	if execErr != nil {
		return Assignment{}, apperr.Wrap(execErr, "update maintenance.jobs")
	}

	eventErr := m.Jobs.event(ctx, call, input.JobID, "assignment."+value, nil)
	if eventErr != nil {
		return Assignment{}, eventErr
	}

	job, err := m.Jobs.scoped(ctx, call, input.JobID)
	if err != nil {
		return Assignment{}, err
	}

	call.Emit(
		outboxEvent(
			"job",
			input.JobID,
			"AssignmentAcknowledged",
			map[string]any{"acknowledgement": value},
		),
	)
	call.Audit(
		auditEntry(
			"jobs.acknowledgeAssignment",
			"job",
			input.JobID,
			call.ExpectedVersion,
			&job.Version,
			deref(input.Reason),
		),
	)

	active, err := m.Jobs.activeAssignment(
		ctx,
		call,
		input.JobID,
	) // the result is the Assignment (service-contracts)
	if err != nil || active == nil {
		return Assignment{}, err
	}

	return *active, nil
}

// RegisterOnSite binds the technician on-site operations.
func RegisterOnSite(r *ops.Registry, m OnSite) {
	ops.Register(r, "jobs.start", m.start)
	ops.Register(r, "jobs.checkIn", m.checkIn)
	ops.Register(r, "jobs.pauseWork", m.pause)
	ops.Register(r, "jobs.acknowledgeAssignment", m.acknowledge)
}
