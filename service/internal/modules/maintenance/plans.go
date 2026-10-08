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

	"github.com/pradita/ac-project/service/internal/ops"
	"github.com/pradita/ac-project/service/internal/platform/apperr"
	"github.com/pradita/ac-project/service/internal/platform/paging"
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

const planCols = `p.id, p.tenant_id, p.version, p.created_at, p.updated_at, p.unit_id, p.interval_months, p.anchor_day, p.next_due_at`

func scanPlan(r pgx.Row) (Plan, error) {
	p := Plan{Timezone: "UTC", Recurrence: Recurrence{Kind: "monthly"}}
	err := r.Scan(&p.ID, &p.TenantID, &p.Version, &p.CreatedAt, &p.UpdatedAt, &p.UnitID, &p.Recurrence.IntervalMonths, &p.AnchorDay, &p.NextDueAt)
	return p, err
}

func (m Plans) occurrences(ctx context.Context, c *ops.Call, ps []Plan) error {
	for i := range ps {
		rows, err := c.Tx.Query(ctx, `SELECT occurrence_at, id FROM maintenance.jobs WHERE plan_id = $1 ORDER BY occurrence_at`, ps[i].ID)
		if err != nil {
			return err
		}
		ps[i].GeneratedOccurrences = []Occurrence{}
		for rows.Next() {
			var o Occurrence
			if err := rows.Scan(&o.OccurrenceAt, &o.JobID); err != nil {
				rows.Close()
				return err
			}
			ps[i].GeneratedOccurrences = append(ps[i].GeneratedOccurrences, o)
		}
		rows.Close()
		if err := rows.Err(); err != nil {
			return err
		}
	}
	return nil
}

func (m Plans) load(ctx context.Context, c *ops.Call, id uuid.UUID, lock bool) (Plan, error) {
	q := "SELECT " + planCols + " FROM maintenance.plans p WHERE p.id = $1"
	if lock {
		q += " FOR UPDATE"
	}
	p, err := scanPlan(c.Tx.QueryRow(ctx, q, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return p, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return p, err
	}
	ps := []Plan{p}
	err = m.occurrences(ctx, c, ps)
	return ps[0], err
}

// NextOccurrence adds months to t and moves to the anchor day (clamped to the month's last day) at the same UTC time.
func NextOccurrence(t time.Time, months, anchor int) time.Time {
	t = t.UTC()
	first := time.Date(t.Year(), t.Month()+time.Month(months), 1, t.Hour(), t.Minute(), t.Second(), t.Nanosecond(), time.UTC)
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

func (m Plans) get(ctx context.Context, c *ops.Call, in *IDInput) (Plan, error) {
	return m.load(ctx, c, in.ID, false)
}

func (m Plans) list(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Plan], error) {
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
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "p.id", "createdAt": "p.created_at", "updatedAt": "p.updated_at"}, "p.id ASC")
	if err != nil {
		return paging.Page[Plan]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Plan]{}, err
	}
	var args []any
	add := func(v any) string { args = append(args, v); return fmt.Sprintf("$%d", len(args)) }
	conds := []string{"TRUE"}
	if f.UnitID != nil {
		conds = append(conds, "p.unit_id = "+add(*f.UnitID))
	}
	if f.CustomerID != nil {
		org, ok, err := m.Jobs.Units.OrgOfCustomer(ctx, c, *f.CustomerID)
		if err != nil {
			return paging.Page[Plan]{}, err
		}
		if !ok {
			org = uuid.Nil
		}
		units, err := m.Jobs.Units.UnitsOfOrg(ctx, c, org)
		if err != nil {
			return paging.Page[Plan]{}, err
		}
		conds = append(conds, "p.unit_id = ANY("+add(units)+")")
	}
	if f.PropertyID != nil {
		units, err := m.Jobs.Units.UnitsOfProperty(ctx, c, *f.PropertyID)
		if err != nil {
			return paging.Page[Plan]{}, err
		}
		conds = append(conds, "p.unit_id = ANY("+add(units)+")")
	}
	where := "(" + strings.Join(conds, ") AND (") + ")"
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM maintenance.plans p WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Plan]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT %s FROM maintenance.plans p WHERE %s ORDER BY %s LIMIT %d OFFSET %d", planCols, where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Plan]{}, err
	}
	items := []Plan{}
	for rows.Next() {
		p, err := scanPlan(rows)
		if err != nil {
			rows.Close()
			return paging.Page[Plan]{}, err
		}
		items = append(items, p)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return paging.Page[Plan]{}, err
	}
	if err := m.occurrences(ctx, c, items); err != nil {
		return paging.Page[Plan]{}, err
	}
	return paging.Page[Plan]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
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
	fe := map[string]string{}
	if in.UnitID == uuid.Nil {
		fe["unitId"] = "error.required"
	}
	if in.Recurrence.Kind != "monthly" || in.Recurrence.IntervalMonths < 1 || in.Recurrence.IntervalMonths > 12 {
		fe["recurrence"] = "error.invalid"
	}
	if in.NextDueAt.IsZero() {
		fe["nextDueAt"] = "error.required"
	}
	return fe
}

func (m Plans) save(ctx context.Context, c *ops.Call, in *PlanSaveInput) (Plan, error) {
	if !in.NextDueAt.After(c.Now) {
		return Plan{}, apperr.Fields(map[string]string{"nextDueAt": "error.past"})
	}
	_, archived, found, err := m.Jobs.Units.UnitState(ctx, c, in.UnitID)
	if err != nil {
		return Plan{}, err
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
		if err := c.Tx.QueryRow(ctx, `INSERT INTO maintenance.plans (tenant_id, unit_id, interval_months, anchor_day, next_due_at, created_at, updated_at)
			VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, $5, $5) RETURNING id`, in.UnitID, in.Recurrence.IntervalMonths, next.Day(), next, c.Now).Scan(&id); err != nil {
			return Plan{}, err
		}
	} else {
		p, err := m.load(ctx, c, *in.ID, true)
		if err != nil {
			return p, err
		}
		if p.UnitID != in.UnitID {
			return p, apperr.Fields(map[string]string{"unitId": "error.unitFixed"})
		}
		if p.Version != *c.ExpectedVersion {
			return p, apperr.E(apperr.Conflict, "error.versionConflict")
		}
		anchor := p.AnchorDay
		if !p.NextDueAt.Equal(next) {
			anchor = next.Day()
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.plans SET interval_months = $2, anchor_day = $3, next_due_at = $4, version = version + 1, updated_at = $5 WHERE id = $1`,
			p.ID, in.Recurrence.IntervalMonths, anchor, next, c.Now); err != nil {
			return p, err
		}
		id, event = p.ID, "PlanChanged"
	}
	p, err := m.load(ctx, c, id, false)
	if err != nil {
		return p, err
	}
	c.Emit(ops.Event{AggregateType: "plan", AggregateID: id, Type: event, Payload: map[string]any{"unitId": p.UnitID}})
	c.Audit(ops.AuditEntry{Action: "plans.save", TargetKind: "plan", TargetID: id.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &p.Version})
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
	fe := map[string]string{}
	if in.ID == uuid.Nil {
		fe["id"] = "error.required"
	}
	if in.OccurrenceDate.IsZero() {
		fe["occurrenceDate"] = "error.required"
	}
	return fe
}

func (m Plans) generateNext(ctx context.Context, c *ops.Call, in *GenerateInput) (Job, error) {
	p, err := m.load(ctx, c, in.ID, true)
	if err != nil {
		return Job{}, err
	}
	if p.Version != *c.ExpectedVersion {
		return Job{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	if !in.OccurrenceDate.Equal(p.NextDueAt) {
		return Job{}, apperr.E(apperr.Conflict, "errors.occurrence_mismatch")
	}
	if !in.OccurrenceDate.After(c.Now) {
		return Job{}, apperr.E(apperr.Conflict, "errors.next_date_past")
	}
	for _, o := range p.GeneratedOccurrences {
		if o.OccurrenceAt.Equal(in.OccurrenceDate) {
			return Job{}, apperr.E(apperr.Conflict, "errors.occurrence_generated")
		}
	}
	org, archived, found, err := m.Jobs.Units.UnitState(ctx, c, p.UnitID)
	if err != nil {
		return Job{}, err
	}
	if !found || archived {
		return Job{}, apperr.E(apperr.Conflict, "error.unitArchived")
	}
	start := in.OccurrenceDate.UTC()
	var id uuid.UUID
	if err := c.Tx.QueryRow(ctx, `INSERT INTO maintenance.jobs (tenant_id, unit_id, customer_org_id, plan_id, occurrence_at, type, status, origin, symptom, requested_slot,
		preferred_slots, due_at) VALUES (current_setting('app.tenant_id')::uuid, $1, $2, $3, $4, 'periodic', 'requested', 'periodic_plan', 'Scheduled periodic maintenance',
		tstzrange($4, $5), '[]', $5) RETURNING id`, p.UnitID, org, p.ID, start, start.Add(time.Hour)).Scan(&id); err != nil {
		return Job{}, err
	}
	next := NextOccurrence(start, p.Recurrence.IntervalMonths, p.AnchorDay)
	if _, err := c.Tx.Exec(ctx, `UPDATE maintenance.plans SET next_due_at = $2, version = version + 1, updated_at = $3 WHERE id = $1`, p.ID, next, c.Now); err != nil {
		return Job{}, err
	}
	if err := m.Jobs.event(ctx, c, id, "job.created", nil); err != nil {
		return Job{}, err
	}
	j, err := m.Jobs.detail(ctx, c, id)
	if err != nil {
		return j, err
	}
	nv := p.Version + 1
	c.Emit(ops.Event{AggregateType: "plan", AggregateID: p.ID, Type: "PlanOccurrenceGenerated", Payload: map[string]any{"jobId": id, "occurrenceAt": start}})
	c.Audit(ops.AuditEntry{Action: "plans.generateNext", TargetKind: "plan", TargetID: p.ID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &nv})
	return j, nil
}

// RegisterPlans binds the plan operations.
func RegisterPlans(r *ops.Registry, m Plans) {
	ops.Register(r, "plans.get", m.get)
	ops.Register(r, "plans.list", m.list)
	ops.Register(r, "plans.save", m.save)
	ops.Register(r, "plans.generateNext", m.generateNext)
}
