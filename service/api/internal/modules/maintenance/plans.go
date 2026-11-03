package maintenance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// Plans is the maintenance plan operation set (D16 / IR130).
type Plans struct {
	Jobs Jobs
}

// Recurrence is MaintenancePlan.recurrence.
type Recurrence struct {
	Kind           string `json:"kind"`
	IntervalMonths int    `json:"intervalMonths"`
}

// Occurrence is one generated occurrence.
type Occurrence struct {
	OccurrenceAt time.Time `json:"occurrenceAt"`
	JobID        uuid.UUID `json:"jobId"`
}

// Plan is MaintenancePlan.
type Plan struct {
	ID                   uuid.UUID    `json:"id"`
	TenantID             uuid.UUID    `json:"tenantId"`
	Version              int          `json:"version"`
	CreatedAt            time.Time    `json:"createdAt"`
	UpdatedAt            time.Time    `json:"updatedAt"`
	UnitID               uuid.UUID    `json:"unitId"`
	Recurrence           Recurrence   `json:"recurrence"`
	Timezone             string       `json:"timezone"`
	AnchorDay            int          `json:"anchorDay"`
	NextDueAt            time.Time    `json:"nextDueAt"`
	GeneratedOccurrences []Occurrence `json:"generatedOccurrences"`
}

const planCols = `p.id, p.tenant_id, p.version, p.created_at, p.updated_at, p.unit_id, p.interval_months, ` +
	`p.anchor_day, p.next_due_at`

func scanPlan(row pgx.Row) (Plan, error) {
	plan := Plan{
		Timezone:             "UTC",
		Recurrence:           Recurrence{Kind: "monthly", IntervalMonths: 0},
		ID:                   uuid.UUID{},
		TenantID:             uuid.UUID{},
		Version:              0,
		CreatedAt:            time.Time{},
		UpdatedAt:            time.Time{},
		UnitID:               uuid.UUID{},
		AnchorDay:            0,
		NextDueAt:            time.Time{},
		GeneratedOccurrences: nil,
	}
	err := row.Scan(
		&plan.ID,
		&plan.TenantID,
		&plan.Version,
		&plan.CreatedAt,
		&plan.UpdatedAt,
		&plan.UnitID,
		&plan.Recurrence.IntervalMonths,
		&plan.AnchorDay,
		&plan.NextDueAt,
	)

	return plan, apperr.Wrap(err, "scan (scanPlan)")
}

// occurrences reads the jobs generated for the plans in one query, not one per plan (IR497).
func (m Plans) occurrences(ctx context.Context, call *ops.Call, ps []Plan) error {
	if len(ps) == 0 {
		return nil
	}

	ids, at := make([]uuid.UUID, len(ps)), make(map[uuid.UUID]int, len(ps))
	for i := range ps {
		ps[i].GeneratedOccurrences = []Occurrence{}
		ids[i], at[ps[i].ID] = ps[i].ID, i
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT plan_id, occurrence_at, id FROM maintenance.jobs WHERE plan_id = ANY($1) ORDER BY plan_id, occurrence_at`,
		ids,
	)
	if err != nil {
		return apperr.Wrap(err, "select maintenance.jobs")
	}

	for rows.Next() {
		var (
			plan       uuid.UUID
			occurrence Occurrence
		)

		err := rows.Scan(&plan, &occurrence.OccurrenceAt, &occurrence.JobID)
		if err != nil {
			rows.Close()

			return apperr.Wrap(err, "select maintenance.jobs")
		}

		if i, ok := at[plan]; ok {
			ps[i].GeneratedOccurrences = append(ps[i].GeneratedOccurrences, occurrence)
		}
	}

	rows.Close()

	return apperr.Wrap(rows.Err(), "select maintenance.jobs")
}

func (m Plans) load(ctx context.Context, c *ops.Call, id uuid.UUID, lock bool) (Plan, error) {
	q := "SELECT " + planCols + " FROM maintenance.plans p WHERE p.id = $1"
	if lock {
		q += " FOR UPDATE"
	}

	plan, err := scanPlan(c.Tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return plan, apperr.E(apperr.NotFound, "error.notFound")
	}

	if err != nil {
		return plan, err
	}

	ps := []Plan{plan}
	err = m.occurrences(ctx, c, ps)

	return ps[0], err
}

// NextOccurrence adds months to t and moves to the anchor day (clamped to the month's last day) at the same UTC time.
func NextOccurrence(t time.Time, months, anchor int) time.Time {
	t = t.UTC()
	first := time.Date(
		t.Year(),
		t.Month()+time.Month(months),
		1,
		t.Hour(),
		t.Minute(),
		t.Second(),
		t.Nanosecond(),
		time.UTC,
	)
	last := first.AddDate(0, 1, -1).Day()

	return first.AddDate(0, 0, min(anchor, last)-1)
}

// ---- plans.get / plans.list ----

// IDInput is the {id} input.
type IDInput struct {
	ID uuid.UUID `json:"id"`
}

// Validate implements ops.Validator.
func (in *IDInput) Validate() map[string]string {
	if in.ID == uuid.Nil {
		return map[string]string{"id": "error.required"}
	}

	return nil
}

// get serves plans.get.
//
//	@Summary		plans.get (read)
//	@ID				plans.get
//	@Description	Authorization: admin:job.read
//	@Description	Validation: D01/D16
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A06
//	@Tags			plans
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"input field id"
//	@Success		200	{object}	ops.Envelope{data=Plan}
//	@Failure		401	{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403	{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404	{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409	{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422	{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429	{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503	{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504	{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/plans/{id} [get]
func (m Plans) get(ctx context.Context, c *ops.Call, in *IDInput) (Plan, error) {
	return m.load(ctx, c, in.ID, false)
}

// list serves plans.list.
//
//	@Summary		plans.list (read)
//	@ID				plans.list
//	@Description	Authorization: admin:job.read
//	@Description	Validation: D01/D16
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A06 · Query: filters unitId,customerId,propertyId · sort id,createdAt,updatedAt (default \
//	@Description	id asc)
//	@Tags			plans
//	@Accept			json
//	@Produce		json
//	@Param			cursor		query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit		query		integer	false	"page size 1–100, default 25"
//	@Param			sort		query		string	false	"field:direction — fields id,createdAt,updatedAt; default id asc"
//	@Param			unitId		query		string	false	"filter → unitId"
//	@Param			customerId	query		string	false	"filter → unit belongs to the customer"
//	@Param			propertyId	query		string	false	"filter → unit is in the property"
//	@Success		200			{object}	ops.Envelope{data=PlanPage}
//	@Failure		401			{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403			{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404			{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409			{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422			{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429			{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503			{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504			{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/plans [get]
func (m Plans) list(
	ctx context.Context,
	call *ops.Call,
	in *paging.Query,
) (paging.Page[Plan], error) {
	var f struct {
		UnitID     *uuid.UUID `json:"unitId,omitempty"`
		CustomerID *uuid.UUID `json:"customerId,omitempty"`
		PropertyID *uuid.UUID `json:"propertyId,omitempty"`
	}

	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&f) != nil {
			return paging.Page[Plan]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}

	order, err := paging.OrderBy(
		in.Sort,
		map[string]string{"id": "p.id", "createdAt": "p.created_at", "updatedAt": "p.updated_at"},
		"p.id ASC",
	)
	if err != nil {
		return paging.Page[Plan]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*in, f, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Plan]{}, apperr.Wrap(err, "paging.Resolve")
	}

	var args []any

	add := func(v any) string {
		args = append(args, v)

		return fmt.Sprintf("$%d", len(args))
	}

	conds := []string{"TRUE"}
	if f.UnitID != nil {
		conds = append(conds, "p.unit_id = "+add(*f.UnitID))
	}

	if f.CustomerID != nil {
		org, ok, err := m.Jobs.Units.OrgOfCustomer(ctx, call, *f.CustomerID)
		if err != nil {
			return paging.Page[Plan]{}, apperr.Wrap(err, "maintenance.JobUnits.OrgOfCustomer")
		}

		if !ok {
			org = uuid.Nil
		}

		units, err := m.Jobs.Units.UnitsOfOrg(ctx, call, org)
		if err != nil {
			return paging.Page[Plan]{}, apperr.Wrap(err, "maintenance.JobUnits.UnitsOfOrg")
		}

		conds = append(conds, "p.unit_id = ANY("+add(units)+")")
	}

	if f.PropertyID != nil {
		units, err := m.Jobs.Units.UnitsOfProperty(ctx, call, *f.PropertyID)
		if err != nil {
			return paging.Page[Plan]{}, apperr.Wrap(err, "maintenance.JobUnits.UnitsOfProperty")
		}

		conds = append(conds, "p.unit_id = ANY("+add(units)+")")
	}

	where := "(" + strings.Join(conds, ") AND (") + ")"

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM maintenance.plans p WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Plan]{}, apperr.Wrap(scanErr, "select maintenance.plans")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT %s FROM maintenance.plans p WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			planCols,
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[Plan]{}, apperr.Wrap(err, "select maintenance.plans")
	}

	items := []Plan{}

	for rows.Next() {
		plan, err := scanPlan(rows)
		if err != nil {
			rows.Close()

			return paging.Page[Plan]{}, err
		}

		items = append(items, plan)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[Plan]{}, apperr.Wrap(rowsErr, "select maintenance.plans")
	}

	occurrencesErr := m.occurrences(ctx, call, items)
	if occurrencesErr != nil {
		return paging.Page[Plan]{}, occurrencesErr
	}

	return paging.Page[Plan]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// ---- plans.save ----

// PlanSaveInput is plans.save input.
type PlanSaveInput struct {
	ID         *uuid.UUID `json:"id,omitempty"`
	UnitID     uuid.UUID  `json:"unitId"`
	Recurrence Recurrence `json:"recurrence"`
	NextDueAt  time.Time  `json:"nextDueAt"`
}

// Validate implements ops.Validator.
func (in *PlanSaveInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.UnitID == uuid.Nil {
		fieldErrs["unitId"] = "error.required"
	}

	if in.Recurrence.Kind != "monthly" || in.Recurrence.IntervalMonths < 1 ||
		in.Recurrence.IntervalMonths > 12 {
		fieldErrs["recurrence"] = "error.invalid"
	}

	if in.NextDueAt.IsZero() {
		fieldErrs["nextDueAt"] = "error.required"
	}

	return fieldErrs
}

// save serves plans.save.
//
//	@Summary		plans.save (write)
//	@ID				plans.save
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A06
//	@Tags			plans
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		PlanSaveInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Plan}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/plans [post]
func (m Plans) save(ctx context.Context, call *ops.Call, in *PlanSaveInput) (Plan, error) {
	if !in.NextDueAt.After(call.Now) {
		return Plan{}, apperr.Fields(map[string]string{"nextDueAt": "error.past"})
	}

	_, archived, found, err := m.Jobs.Units.UnitState(ctx, call, in.UnitID)
	if err != nil {
		return Plan{}, apperr.Wrap(err, "maintenance.JobUnits.UnitState")
	}

	if !found {
		return Plan{}, apperr.E(apperr.NotFound, "error.notFound")
	}

	if archived {
		return Plan{}, apperr.Fields(map[string]string{"unitId": "error.unitArchived"})
	}

	next := in.NextDueAt.UTC()

	var id uuid.UUID

	event := "PlanCreated"

	if in.ID == nil {
		err := call.Tx.QueryRow(ctx, `INSERT INTO maintenance.plans (tenant_id, unit_id, interval_months, anchor_day,
			next_due_at, created_at, updated_at)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $5) RETURNING
			id`, in.UnitID, in.Recurrence.IntervalMonths, next.Day(), next, call.Now).
			Scan(&id)
		if err != nil {
			return Plan{}, apperr.Wrap(err, "insert maintenance.plans")
		}
	} else {
		plan, err := m.load(ctx, call, *in.ID, true)
		if err != nil {
			return plan, err
		}

		if plan.UnitID != in.UnitID {
			return plan, apperr.Fields(map[string]string{"unitId": "error.unitFixed"})
		}

		if plan.Version != *call.ExpectedVersion {
			return plan, apperr.E(apperr.Conflict, "error.versionConflict")
		}

		anchor := plan.AnchorDay
		if !plan.NextDueAt.Equal(next) {
			anchor = next.Day()
		}

		_, execErr := call.Tx.Exec(
			ctx,
			`UPDATE maintenance.plans SET interval_months = $2, anchor_day = $3, next_due_at = $4, version = version + 1,
				updated_at = $5 WHERE id = $1`,
			plan.ID,
			in.Recurrence.IntervalMonths,
			anchor,
			next,
			call.Now,
		)
		if execErr != nil {
			return plan, apperr.Wrap(execErr, "update maintenance.plans")
		}

		id, event = plan.ID, "PlanChanged"
	}

	p, err := m.load(ctx, call, id, false)
	if err != nil {
		return p, err
	}

	call.Emit(
		ops.Event{
			AggregateType: "plan",
			AggregateID:   id,
			Type:          event,
			Payload:       map[string]any{"unitId": p.UnitID},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "plans.save",
			TargetKind:      "plan",
			TargetID:        id.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &p.Version, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return p, nil
}

// ---- plans.generateNext ----

// GenerateInput is plans.generateNext input.
type GenerateInput struct {
	ID             uuid.UUID `json:"id"`
	OccurrenceDate time.Time `json:"occurrenceDate"`
}

// Validate implements ops.Validator.
func (in *GenerateInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ID == uuid.Nil {
		fieldErrs["id"] = "error.required"
	}

	if in.OccurrenceDate.IsZero() {
		fieldErrs["occurrenceDate"] = "error.required"
	}

	return fieldErrs
}

// generateNext serves plans.generateNext.
//
//	@Summary		plans.generateNext (write)
//	@ID				plans.generateNext
//	@Description	Authorization: admin:job.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A06
//	@Tags			plans
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target plan, read plans.get)"
//	@Param			id					path		string			true	"input field id"
//	@Param			request				body		GenerateInput	true	"input; the path parameters come from the route"
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
//	@Router			/v1/plans/{id}/generate-next [post]
func (m Plans) generateNext(
	ctx context.Context,
	c *ops.Call,
	in *GenerateInput,
) (MaintenanceJob, error) {
	return written(m.generateNextJob(ctx, c, in))
}

// generateNextJob is generateNext answering the job with its detail extras.
func (m Plans) generateNextJob(
	ctx context.Context,
	c *ops.Call,
	input *GenerateInput,
) (Job, error) {
	plan, err := m.load(ctx, c, input.ID, true)
	if err != nil {
		return Job{}, err
	}

	if plan.Version != *c.ExpectedVersion {
		return Job{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}

	if !input.OccurrenceDate.Equal(plan.NextDueAt) {
		return Job{}, apperr.E(apperr.Conflict, "errors.occurrence_mismatch")
	}

	if !input.OccurrenceDate.After(c.Now) {
		return Job{}, apperr.E(apperr.Conflict, "errors.next_date_past")
	}

	for _, o := range plan.GeneratedOccurrences {
		if o.OccurrenceAt.Equal(input.OccurrenceDate) {
			return Job{}, apperr.E(apperr.Conflict, "errors.occurrence_generated")
		}
	}

	org, archived, found, err := m.Jobs.Units.UnitState(ctx, c, plan.UnitID)
	if err != nil {
		return Job{}, apperr.Wrap(err, "maintenance.JobUnits.UnitState")
	}

	if !found || archived {
		return Job{}, apperr.E(apperr.Conflict, "error.unitArchived")
	}

	start := input.OccurrenceDate.UTC()

	var id uuid.UUID

	scanErr := c.Tx.QueryRow(ctx, `INSERT INTO maintenance.jobs (tenant_id, unit_id, customer_org_id, plan_id,
		occurrence_at, type, status, origin, symptom, requested_slot,
		preferred_slots, due_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, 'periodic', 'requested',
		'periodic_plan', 'Scheduled periodic maintenance',
		tstzrange($4, $5), '[]', $5) RETURNING id`, plan.UnitID, org, plan.ID, start, start.Add(time.Hour)).
		Scan(&id)
	if scanErr != nil {
		return Job{}, apperr.Wrap(scanErr, "insert maintenance.jobs")
	}

	next := NextOccurrence(start, plan.Recurrence.IntervalMonths, plan.AnchorDay)

	_, execErr := c.Tx.Exec(
		ctx,
		`UPDATE maintenance.plans SET next_due_at = $2, version = version + 1, updated_at = $3 WHERE id = $1`,
		plan.ID,
		next,
		c.Now,
	)
	if execErr != nil {
		return Job{}, apperr.Wrap(execErr, "update maintenance.plans")
	}

	eventErr := m.Jobs.event(ctx, c, id, "job.created", nil)
	if eventErr != nil {
		return Job{}, eventErr
	}

	job, err := m.Jobs.detail(ctx, c, id)
	if err != nil {
		return job, err
	}

	nv := plan.Version + 1
	c.Emit(
		ops.Event{
			AggregateType: "plan",
			AggregateID:   plan.ID,
			Type:          "PlanOccurrenceGenerated",
			Payload:       map[string]any{"jobId": id, "occurrenceAt": start},
		},
	)
	c.Audit(
		ops.AuditEntry{
			Action:          "plans.generateNext",
			TargetKind:      "plan",
			TargetID:        plan.ID.String(),
			PreviousVersion: c.ExpectedVersion,
			NextVersion:     &nv, Reason: "", Result: "", Before: nil, After: nil,
		},
	)

	return job, nil
}

// RegisterPlans binds the plan operations.
func RegisterPlans(r *ops.Registry, m Plans) {
	ops.Register(r, "plans.get", m.get)
	ops.Register(r, "plans.list", m.list)
	ops.Register(r, "plans.save", m.save)
	ops.Register(r, "plans.generateNext", m.generateNext)
}
