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
func (m OnSite) lockOwn(ctx context.Context, c *ops.Call, id uuid.UUID) (string, TimeOnSite, error) {
	var status string
	var unit uuid.UUID
	var v int
	var raw []byte
	err := c.Tx.QueryRow(ctx, `SELECT status, unit_id, version, time_on_site FROM maintenance.jobs WHERE id = $1 FOR UPDATE`, id).Scan(&status, &unit, &v, &raw)
	if errors.Is(err, pgx.ErrNoRows) {
		return "", TimeOnSite{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return "", TimeOnSite{}, err
	}
	if err := m.Access.TechnicianJob(ctx, c, id, unit); err != nil {
		return "", TimeOnSite{}, err
	}
	if v != *c.ExpectedVersion {
		return "", TimeOnSite{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	t := TimeOnSite{Pauses: []Pause{}}
	if len(raw) > 0 && string(raw) != "null" {
		if err := json.Unmarshal(raw, &t); err != nil {
			return "", t, err
		}
	}
	if t.Pauses == nil {
		t.Pauses = []Pause{}
	}
	return status, t, nil
}

func (m OnSite) save(ctx context.Context, c *ops.Call, id uuid.UUID, status string, t TimeOnSite, action, event, reason string) (Job, error) {
	raw, _ := json.Marshal(t)
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET status = $2, time_on_site = $3, started_at = COALESCE(started_at, $4), version = version + 1,
		updated_at = platform.app_now() WHERE id = $1`, id, status, raw, t.StartedAt); err != nil {
		return Job{}, err
	}
	if err := m.Jobs.event(ctx, c, id, action, nil); err != nil {
		return Job{}, err
	}
	j, err := m.Jobs.scoped(ctx, c, id)
	if err != nil {
		return j, err
	}
	c.Emit(ops.Event{AggregateType: "job", AggregateID: id, Type: event, Payload: map[string]any{"status": j.Status}})
	c.Audit(ops.AuditEntry{Action: "jobs." + action[4:], TargetKind: "job", TargetID: id.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &j.Version, Reason: reason})
	return j, nil
}

// StartInput is jobs.start input.
type StartInput struct {
	JobID          uuid.UUID `json:"jobId"`
	StartConfirmed *bool     `json:"startConfirmed"`
}

// Validate implements ops.Validator.
func (in *StartInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.StartConfirmed == nil || !*in.StartConfirmed {
		fe["startConfirmed"] = "error.required"
	}
	return fe
}

func (m OnSite) start(ctx context.Context, c *ops.Call, in *StartInput) (Job, error) {
	status, t, err := m.lockOwn(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	if status != "assigned" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	now := c.Now
	t.StartedAt = &now
	return m.save(ctx, c, in.JobID, "in_progress", t, "job.started", "JobStarted", "")
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
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	switch in.Method {
	case "location_qr":
		if in.DistanceMeters == nil || *in.DistanceMeters < 0 || *in.DistanceMeters > 200 {
			fe["distanceMeters"] = "errors.too_far"
		}
		if in.QRUnitID == nil {
			fe["qrUnitId"] = "error.required"
		}
	case "manual":
		if in.Reason == nil {
			fe["reason"] = "error.required"
		} else {
			trimmedReason(in.Reason, 1000, "reason", fe)
		}
		if in.DistanceMeters != nil && *in.DistanceMeters < 0 {
			fe["distanceMeters"] = "error.range"
		}
	default:
		fe["method"] = "error.invalid"
	}
	return fe
}

func (m OnSite) checkIn(ctx context.Context, c *ops.Call, in *CheckInInput) (Job, error) {
	status, t, err := m.lockOwn(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	if in.Method == "location_qr" {
		var unit uuid.UUID
		if err := c.Tx.QueryRow(ctx, `SELECT unit_id FROM maintenance.jobs WHERE id = $1`, in.JobID).Scan(&unit); err != nil {
			return Job{}, err
		}
		if *in.QRUnitID != unit {
			return Job{}, apperr.Fields(map[string]string{"qrUnitId": "errors.qr_unit_mismatch"})
		}
	}
	if status != "assigned" && !(status == "in_progress" && t.ArrivedAt == nil) {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	now := c.Now
	method := in.Method
	t.ArrivedAt, t.CheckInMethod, t.Distance = &now, &method, in.DistanceMeters
	if in.Method == "manual" {
		t.CheckInReason = in.Reason
	}
	if t.StartedAt == nil {
		t.StartedAt = &now
	}
	return m.save(ctx, c, in.JobID, "in_progress", t, "job.checked_in", "JobCheckedIn", deref(in.Reason))
}

// PauseInput is jobs.pauseWork input.
type PauseInput struct {
	JobID  uuid.UUID `json:"jobId"`
	Paused *bool     `json:"paused"`
}

// Validate implements ops.Validator.
func (in *PauseInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	if in.Paused == nil {
		fe["paused"] = "error.required"
	}
	return fe
}

func (m OnSite) pause(ctx context.Context, c *ops.Call, in *PauseInput) (Job, error) {
	status, t, err := m.lockOwn(ctx, c, in.JobID)
	if err != nil {
		return Job{}, err
	}
	if status != "in_progress" {
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	open := len(t.Pauses) > 0 && t.Pauses[len(t.Pauses)-1].To == nil
	now := c.Now
	switch {
	case *in.Paused && open, !*in.Paused && !open:
		return Job{}, apperr.E(apperr.Conflict, "error.invalidState")
	case *in.Paused:
		t.Pauses = append(t.Pauses, Pause{From: now})
	default:
		t.Pauses[len(t.Pauses)-1].To = &now
	}
	action := "job.resumed_work"
	if *in.Paused {
		action = "job.paused"
	}
	return m.save(ctx, c, in.JobID, status, t, action, "JobPauseChanged", "")
}

// AckInput is jobs.acknowledgeAssignment input.
type AckInput struct {
	JobID           uuid.UUID `json:"jobId"`
	Decision        string    `json:"decision"`
	Reason          *string   `json:"reason,omitempty"`
	AlternativeSlot *Slot     `json:"alternativeSlot,omitempty"`
}

// Validate implements ops.Validator (IR113 item 8).
func (in *AckInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.JobID == uuid.Nil {
		fe["jobId"] = "error.required"
	}
	switch in.Decision {
	case "accept":
		if in.Reason != nil || in.AlternativeSlot != nil {
			fe["decision"] = "error.fieldsNotAllowed"
		}
	case "cant_make":
		if in.Reason == nil {
			fe["reason"] = "error.required"
		} else {
			trimmedReason(in.Reason, 1000, "reason", fe)
		}
		if in.AlternativeSlot != nil && !in.AlternativeSlot.StartAt.Before(in.AlternativeSlot.EndAt) {
			fe["alternativeSlot"] = "error.range"
		}
	default:
		fe["decision"] = "error.invalid"
	}
	return fe
}

func (m OnSite) acknowledge(ctx context.Context, c *ops.Call, in *AckInput) (Assignment, error) {
	var aid uuid.UUID
	var ack string
	var end time.Time
	err := c.Tx.QueryRow(ctx, `SELECT id, acknowledgement, upper(scheduled) FROM maintenance.assignments WHERE job_id = $1 AND technician_membership_id = $2
		AND status = 'active' AND created_at <= $3 FOR UPDATE`, in.JobID, c.Principal.MembershipID, c.Now).Scan(&aid, &ack, &end)
	if errors.Is(err, pgx.ErrNoRows) {
		return Assignment{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return Assignment{}, err
	}
	if !c.Now.Before(end) {
		return Assignment{}, apperr.E(apperr.Forbidden, "errors.assignment_ended")
	}
	var v int
	if err := c.Tx.QueryRow(ctx, `SELECT version FROM maintenance.jobs WHERE id = $1 FOR UPDATE`, in.JobID).Scan(&v); err != nil {
		return Assignment{}, err
	}
	if v != *c.ExpectedVersion {
		return Assignment{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if ack != "pending" {
		return Assignment{}, apperr.E(apperr.Conflict, "error.invalidState")
	}
	value := "accepted"
	var altStart, altEnd *time.Time
	if in.Decision == "cant_make" {
		value = "cant_make"
		if in.AlternativeSlot != nil {
			altStart, altEnd = &in.AlternativeSlot.StartAt, &in.AlternativeSlot.EndAt
		}
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.assignments SET acknowledgement = $2, acknowledged_at = $3, cant_make_reason = $4,
		alternative_slot = CASE WHEN $5::timestamptz IS NULL THEN NULL ELSE tstzrange($5, $6) END, version = version + 1, updated_at = platform.app_now() WHERE id = $1`,
		aid, value, c.Now, in.Reason, altStart, altEnd); err != nil {
		return Assignment{}, err
	}
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.jobs SET version = version + 1, updated_at = platform.app_now() WHERE id = $1`, in.JobID); err != nil {
		return Assignment{}, err
	}
	if err := m.Jobs.event(ctx, c, in.JobID, "assignment."+value, nil); err != nil {
		return Assignment{}, err
	}
	j, err := m.Jobs.scoped(ctx, c, in.JobID)
	if err != nil {
		return Assignment{}, err
	}
	c.Emit(ops.Event{AggregateType: "job", AggregateID: in.JobID, Type: "AssignmentAcknowledged", Payload: map[string]any{"acknowledgement": value}})
	c.Audit(ops.AuditEntry{Action: "jobs.acknowledgeAssignment", TargetKind: "job", TargetID: in.JobID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &j.Version, Reason: deref(in.Reason)})
	a, err := m.Jobs.activeAssignment(ctx, c, in.JobID) // the result is the Assignment (service-contracts)
	if err != nil || a == nil {
		return Assignment{}, err
	}
	return *a, nil
}

// RegisterOnSite binds the technician on-site operations.
func RegisterOnSite(r *ops.Registry, m OnSite) {
	ops.Register(r, "jobs.start", m.start)
	ops.Register(r, "jobs.checkIn", m.checkIn)
	ops.Register(r, "jobs.pauseWork", m.pause)
	ops.Register(r, "jobs.acknowledgeAssignment", m.acknowledge)
}
