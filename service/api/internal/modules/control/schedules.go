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
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// scheduleRef is the automation_runs trigger of a schedule occurrence: the same for the scheduler and a UI fire of
// the same tick, so one occurrence creates at most one Command per rule and unit (IR54).
func scheduleRef(phase string, at time.Time) string {
	return "schedule:" + phase + ":" + at.UTC().Truncate(time.Minute).Format(time.RFC3339)
}

// maxScheduleWindow bounds one catch-up (a long demo clock jump is processed up to this far back).
const maxScheduleWindow = 400 * 24 * time.Hour

// FireSchedules is the IR54 internal path of schedule automations, a scheduler job of the equipment domain: the start /
// end occurrences of the tenant's enabled schedule rules that fell in (watermark, now] are evaluated at their minute
// with the D02 arbitration of the unit's rules, in time order (an end before a start of the same minute; the end yields
// to a start of the same minute that would run, IR504). A unit takes
// only the last minute the tick passed for it (IR473): a tick that passes several — a demo clock jump, or the scheduler
// paused — passes over the earlier ones, whose Commands would still be pending when the later one is evaluated, so the
// unit ends in the state real time would have left. The winner per unit gets its Command (source automation, actor =
// rule owner re-authorized now) and an audit entry; a missing owner permission, a restriction or an offline unit
// suppresses it. The watermark (control.schedule_watermarks, locked for the tick) starts at the first tick, so
// occurrences before it are never replayed; automation_runs keeps one outcome per rule, unit and occurrence. Returns
// the Commands created.
func (m Automations) FireSchedules(ctx context.Context, call *ops.Call) (int, error) {
	now := call.Now.UTC().Truncate(time.Minute)

	var since time.Time

	err := call.Tx.QueryRow(ctx, `SELECT evaluated_until FROM control.schedule_watermarks WHERE tenant_id = $1 FOR
		UPDATE`, call.Principal.TenantID).
		Scan(&since)
	if errors.Is(err, pgx.ErrNoRows) {
		_, err = call.Tx.Exec(
			ctx,
			`INSERT INTO control.schedule_watermarks (tenant_id, evaluated_until) VALUES ($1, $2) ON CONFLICT (tenant_id) DO
				NOTHING`,
			call.Principal.TenantID,
			now,
		)

		return 0, apperr.Wrap(err, "insert control.schedule_watermarks")
	}

	if err != nil || !now.After(since) {
		return 0, apperr.Wrap(err, "insert control.schedule_watermarks")
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

	rows, err := call.Tx.Query(
		ctx,
		`SELECT a.definition, a.timezone, array_agg(u.unit_id ORDER BY u.unit_id) FROM control.automations a
		JOIN control.automation_units u ON u.automation_id = a.id WHERE a.kind = 'schedule' AND a.enabled GROUP BY a.id ORDER
		BY a.id`,
	)
	if err != nil {
		return 0, apperr.Wrap(err, "select control.automations")
	}

	days := int(now.Sub(since).Hours()/24) + 3

	for rows.Next() {
		var (
			raw      []byte
			timezone string
			units    []uuid.UUID
		)

		err := rows.Scan(&raw, &timezone, &units)
		if err != nil {
			rows.Close()

			return 0, apperr.Wrap(err, "select control.automations")
		}

		loc, err := time.LoadLocation(timezone)
		if err != nil {
			continue
		}

		var d definition

		_ = json.Unmarshal(raw, &d)

		occ, _ := occurrences(d, loc, since.In(loc).AddDate(0, 0, -1), days)
		for _, scheduledOccurrence := range occ {
			if !scheduledOccurrence.At.After(since) || scheduledOccurrence.At.After(now) {
				continue
			}

			key := scheduleRef(scheduledOccurrence.Phase, scheduledOccurrence.At)

			scheduleTick := ticks[key]
			if scheduleTick == nil {
				scheduleTick = &tick{
					at:    scheduledOccurrence.At.UTC(),
					phase: scheduledOccurrence.Phase,
					units: map[uuid.UUID]bool{},
				}
				ticks[key] = scheduleTick
			}

			for _, u := range units {
				scheduleTick.units[u] = true
			}
		}
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return 0, apperr.Wrap(rowsErr, "select control.automations")
	}

	order := make([]*tick, 0, len(ticks))
	for _, t := range ticks {
		order = append(order, t)
	}

	sort.Slice(order, func(left, right int) bool {
		if !order[left].at.Equal(order[right].at) {
			return order[left].at.Before(order[right].at)
		}

		return order[left].phase == "schedule_end" &&
			order[right].phase != "schedule_end" // end, then the next start
	})

	last := map[uuid.UUID]time.Time{} // the last minute the tick passed for each unit (IR473)

	for _, t := range order {
		for u := range t.units {
			if t.at.After(last[u]) {
				last[u] = t.at
			}
		}
	}

	created := 0

	for _, scheduleTick := range order {
		input := &EvaluationInput{
			EventID: uuid.NewSHA1(
				call.Principal.TenantID,
				[]byte(scheduleRef(scheduleTick.phase, scheduleTick.at)),
			),
			OccurredAt: scheduleTick.at,
			Phase:      scheduleTick.phase,
			UnitIDs:    nil,
			Facts:      nil,
		}
		for unitID := range scheduleTick.units {
			if !scheduleTick.at.Equal(last[unitID]) {
				continue // passed over: a later minute sets the unit's state
			}

			_, found, err := m.Units.Target(ctx, call, unitID)
			if err != nil {
				return created, apperr.Wrap(err, "control.Units.Target")
			} else if found { // an archived or removed unit is skipped, not an error of the tick
				input.UnitIDs = append(input.UnitIDs, unitID)
			}
		}

		if len(input.UnitIDs) == 0 {
			continue
		}

		r, err := m.evaluate(ctx, call, input, true, false)
		if err != nil {
			return created, err
		}

		for _, decision := range r.Results {
			if decision.Decision == "requested" && decision.RuleID != nil &&
				decision.CommandID != nil &&
				decision.fresh {
				created++

				call.Audit(
					ops.AuditEntry{
						Action:          "automations.schedule",
						TargetKind:      "automation",
						TargetID:        decision.RuleID.String(),
						Reason:          scheduleTick.phase,
						PreviousVersion: nil,
						NextVersion:     nil,
						Result:          "",
						Before:          nil,
						After:           nil,
					},
				)
			}
		}
	}

	_, err = call.Tx.Exec(
		ctx,
		`UPDATE control.schedule_watermarks SET evaluated_until = $2 WHERE tenant_id = $1`,
		call.Principal.TenantID,
		now,
	)

	return created, apperr.Wrap(err, "update control.schedule_watermarks")
}
