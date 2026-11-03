package control

import (
	"context"
	"encoding/json"
	"errors"
	"maps"
	"slices"
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

	since, due, err := scheduleWindow(ctx, call, now)
	if err != nil || !due {
		return 0, err
	}

	order, err := dueTicks(ctx, call, since, now)
	if err != nil {
		return 0, err
	}

	last := lastTicks(order)
	created := 0

	for _, tick := range order {
		fired, err := m.fireTick(ctx, call, tick, last)
		created += fired

		if err != nil {
			return created, err
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

// scheduleWindow is where this tick starts, the tenant's watermark (locked for the tick), and whether any minute is
// due. The first tick only sets the watermark, so occurrences before it are never replayed; a long catch-up reaches
// back maxScheduleWindow at most.
func scheduleWindow(ctx context.Context, call *ops.Call, now time.Time) (time.Time, bool, error) {
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

		return since, false, apperr.Wrap(err, "insert control.schedule_watermarks")
	}

	if err != nil {
		return since, false, apperr.Wrap(err, "select control.schedule_watermarks")
	}

	if !now.After(since) {
		return since, false, nil
	}

	if now.Sub(since) > maxScheduleWindow {
		since = now.Add(-maxScheduleWindow)
	}

	return since, true, nil
}

// scheduleTick is one minute and phase that schedule occurrences fell on, with the units of their rules.
type scheduleTick struct {
	at    time.Time
	phase string
	units map[uuid.UUID]bool
}

// dueTicks are the ticks of the tenant's enabled schedule rules in (since, now], in time order, an end before a start
// of the same minute. A rule in a timezone this server does not know has none.
func dueTicks(ctx context.Context, call *ops.Call, since, now time.Time) ([]*scheduleTick, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT a.definition, a.timezone, array_agg(u.unit_id ORDER BY u.unit_id) FROM control.automations a
		JOIN control.automation_units u ON u.automation_id = a.id WHERE a.kind = 'schedule' AND a.enabled GROUP BY a.id ORDER
		BY a.id`,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select control.automations")
	}
	defer rows.Close()

	days := int(
		now.Sub(since).Hours()/hoursPerDay,
	) + tickDays // the days since the last tick, and one either side
	ticks := map[string]*scheduleTick{}

	for rows.Next() {
		var (
			raw      []byte
			timezone string
			units    []uuid.UUID
			def      definition
		)

		err := rows.Scan(&raw, &timezone, &units)
		if err != nil {
			return nil, apperr.Wrap(err, "select control.automations")
		}

		loc, err := time.LoadLocation(timezone)
		if err != nil {
			continue
		}

		err = json.Unmarshal(raw, &def)
		if err != nil {
			return nil, apperr.Wrap(err, "json.Unmarshal")
		}

		occ, _ := occurrences(def, loc, since.In(loc).AddDate(0, 0, -1), days)
		for _, occurrence := range occ {
			if !occurrence.At.After(since) || occurrence.At.After(now) {
				continue
			}

			key := scheduleRef(occurrence.Phase, occurrence.At)
			if ticks[key] == nil {
				ticks[key] = &scheduleTick{
					at:    occurrence.At.UTC(),
					phase: occurrence.Phase,
					units: map[uuid.UUID]bool{},
				}
			}

			for _, u := range units {
				ticks[key].units[u] = true
			}
		}
	}

	rowsErr := rows.Err()
	if rowsErr != nil {
		return nil, apperr.Wrap(rowsErr, "select control.automations")
	}

	order := slices.Collect(maps.Values(ticks))
	sort.Slice(order, func(left, right int) bool {
		if !order[left].at.Equal(order[right].at) {
			return order[left].at.Before(order[right].at)
		}

		return order[left].phase == phaseScheduleEnd &&
			order[right].phase != phaseScheduleEnd // end, then the next start
	})

	return order, nil
}

// lastTicks is the last minute the tick passed for each unit (IR473).
func lastTicks(order []*scheduleTick) map[uuid.UUID]time.Time {
	last := map[uuid.UUID]time.Time{}

	for _, tick := range order {
		for u := range tick.units {
			if tick.at.After(last[u]) {
				last[u] = tick.at
			}
		}
	}

	return last
}

// fireTick evaluates one tick for its units whose last minute it is (an earlier minute is passed over: the later one
// sets the unit's state), skipping archived or removed units, and audits each Command it created. It returns how many
// it created.
func (m Automations) fireTick(
	ctx context.Context,
	call *ops.Call,
	tick *scheduleTick,
	last map[uuid.UUID]time.Time,
) (int, error) {
	input := &EvaluationInput{
		EventID:    uuid.NewSHA1(call.Principal.TenantID, []byte(scheduleRef(tick.phase, tick.at))),
		OccurredAt: tick.at,
		Phase:      tick.phase,
		UnitIDs:    nil,
		Facts:      nil,
	}

	for unitID := range tick.units {
		if !tick.at.Equal(last[unitID]) {
			continue
		}

		_, found, err := m.Units.Target(ctx, call, unitID)
		if err != nil {
			return 0, apperr.Wrap(err, "control.Units.Target")
		}

		if found {
			input.UnitIDs = append(input.UnitIDs, unitID)
		}
	}

	if len(input.UnitIDs) == 0 {
		return 0, nil
	}

	result, err := m.evaluate(ctx, call, input, true, false)
	if err != nil {
		return 0, err
	}

	created := 0

	for _, decision := range result.Results {
		if decision.Decision != decisionRequested || decision.RuleID == nil ||
			decision.CommandID == nil ||
			!decision.fresh {
			continue
		}

		created++

		call.Audit(
			ops.AuditEntry{
				Action:          "automations.schedule",
				TargetKind:      kindAutomation,
				TargetID:        decision.RuleID.String(),
				Reason:          tick.phase,
				PreviousVersion: nil,
				NextVersion:     nil,
				Result:          "",
				Before:          nil,
				After:           nil,
			},
		)
	}

	return created, nil
}
