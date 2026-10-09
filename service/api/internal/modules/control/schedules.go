package control

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// scheduleRef is the automation_runs trigger of a schedule occurrence: the same for the scheduler and a UI fire of
// the same tick, so one occurrence creates at most one Command per rule and unit (IR54).
func scheduleRef(phase string, at time.Time) string {
	return "schedule:" + phase + ":" + at.UTC().Truncate(time.Minute).Format(time.RFC3339)
}

// maxScheduleWindow bounds one catch-up (a long demo clock jump is processed up to this far back).
const maxScheduleWindow = 400 * 24 * time.Hour

// FireSchedules is the IR54 internal path of schedule automations, a scheduler job of the equipment domain: every
// start / end occurrence of the tenant's schedule rules that fell in (watermark, now] is evaluated at its minute with
// the D02 arbitration of the unit's rules, in time order (an end before a start of the same minute). The winner per
// unit gets its Command (source automation, actor = rule owner re-authorized now) and an audit entry; a disabled
// rule, a missing owner permission, a restriction or an offline unit suppresses it. The watermark
// (control.schedule_watermarks, locked for the tick) starts at the first tick, so occurrences before it are never
// replayed; automation_runs keeps one outcome per rule, unit and occurrence. Returns the Commands created.
func (m Automations) FireSchedules(ctx context.Context, c *ops.Call) (int, error) {
	now := c.Now.UTC().Truncate(time.Minute)
	var since time.Time
	err := c.Tx.QueryRow(ctx, `SELECT evaluated_until FROM control.schedule_watermarks WHERE tenant_id = $1 FOR UPDATE`, c.Principal.TenantID).Scan(&since)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = c.Tx.Exec(ctx, `INSERT INTO control.schedule_watermarks (tenant_id, evaluated_until) VALUES ($1, $2) ON CONFLICT (tenant_id) DO NOTHING`, c.Principal.TenantID, now)
		return 0, err
	}
	if err != nil || !now.After(since) {
		return 0, err
	}
	if now.Sub(since) > maxScheduleWindow {
		since = now.Add(-maxScheduleWindow)
	}
	type tick struct {
		at    time.Time
		phase string
		units map[uuid.UUID]bool
	}
	ticks := map[string]*tick{}
	rows, err := c.Tx.Query(ctx, `SELECT a.definition, a.timezone, array_agg(u.unit_id ORDER BY u.unit_id) FROM control.automations a
		JOIN control.automation_units u ON u.automation_id = a.id WHERE a.kind = 'schedule' GROUP BY a.id ORDER BY a.id`)
	if err != nil {
		return 0, err
	}
	days := int(now.Sub(since).Hours()/24) + 3
	for rows.Next() {
		var raw []byte
		var tz string
		var units []uuid.UUID
		if err := rows.Scan(&raw, &tz, &units); err != nil {
			rows.Close()
			return 0, err
		}
		loc, err := time.LoadLocation(tz)
		if err != nil {
			continue
		}
		var d definition
		_ = json.Unmarshal(raw, &d)
		occ, _ := occurrences(d, loc, since.In(loc).AddDate(0, 0, -1), days)
		for _, o := range occ {
			if !o.At.After(since) || o.At.After(now) {
				continue
			}
			key := scheduleRef(o.Phase, o.At)
			t := ticks[key]
			if t == nil {
				t = &tick{at: o.At.UTC(), phase: o.Phase, units: map[uuid.UUID]bool{}}
				ticks[key] = t
			}
			for _, u := range units {
				t.units[u] = true
			}
		}
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return 0, err
	}
	order := make([]*tick, 0, len(ticks))
	for _, t := range ticks {
		order = append(order, t)
	}
	sort.Slice(order, func(i, j int) bool {
		if !order[i].at.Equal(order[j].at) {
			return order[i].at.Before(order[j].at)
		}
		return order[i].phase == "schedule_end" && order[j].phase != "schedule_end" // end, then the next start
	})
	created := 0
	for _, t := range order {
		in := &EvaluationInput{EventID: uuid.NewSHA1(c.Principal.TenantID, []byte(scheduleRef(t.phase, t.at))), OccurredAt: t.at, Phase: t.phase}
		for u := range t.units {
			if _, found, err := m.Units.Target(ctx, c, u); err != nil {
				return created, err
			} else if found { // an archived or removed unit is skipped, not an error of the tick
				in.UnitIDs = append(in.UnitIDs, u)
			}
		}
		if len(in.UnitIDs) == 0 {
			continue
		}
		r, err := m.evaluate(ctx, c, in, true, false)
		if err != nil {
			return created, err
		}
		for _, d := range r.Results {
			if d.Decision == "requested" && d.RuleID != nil && d.CommandID != nil && d.fresh {
				created++
				c.Audit(ops.AuditEntry{Action: "automations.schedule", TargetKind: "automation", TargetID: d.RuleID.String(), Reason: t.phase})
			}
		}
	}
	_, err = c.Tx.Exec(ctx, `UPDATE control.schedule_watermarks SET evaluated_until = $2 WHERE tenant_id = $1`, c.Principal.TenantID, now)
	return created, err
}
