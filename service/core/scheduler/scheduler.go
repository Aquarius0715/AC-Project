// Package worker runs the clock-driven transitions (worker tick, backend Go design §7).
package scheduler

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/core/modules/control"
	"github.com/pradita/ac-project/service/core/modules/devices"
	"github.com/pradita/ac-project/service/core/modules/maintenance"
	"github.com/pradita/ac-project/service/core/modules/restrictions"
	"github.com/pradita/ac-project/service/core/ops"
	"github.com/pradita/ac-project/service/core/platform/db"
	"github.com/pradita/ac-project/service/core/seed"
)

// Result counts the changes of one tick.
type Result struct {
	ExpiredOffers    int
	ExpiredCommands  int
	Runs             int
	Confirmed        int
	ExpiredProposals int
	FrozenHistories  int
}

// Tick runs every clock-driven transition once per tenant, each tenant in its own transaction (RLS context set).
func Tick(ctx context.Context, m *db.TxManager, now time.Time) (Result, error) {
	var tenants []uuid.UUID
	rows, err := m.Writer.Query(ctx, `SELECT id FROM platform.tenants ORDER BY id`)
	if err != nil {
		return Result{}, err
	}
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return Result{}, err
		}
		tenants = append(tenants, id)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	var r Result
	actor := seed.ID("system-demo")
	for _, t := range tenants {
		err := m.Run(ctx, false, &ops.Principal{TenantID: t}, func(tx pgx.Tx) error {
			if _, err := tx.Exec(ctx, `SELECT set_config('app.now', $1, true)`, now.UTC().Format(time.RFC3339Nano)); err != nil {
				return err
			}
			n, err := control.ExpireCommands(ctx, tx, now)
			if err != nil {
				return err
			}
			r.ExpiredCommands += n
			if n, err = control.AdvanceRuns(ctx, &ops.Call{Tx: tx, Now: now, Principal: &ops.Principal{TenantID: t}}, control.Commands{Devices: devices.Models{}, Restrictions: restrictions.Busy{}}); err != nil {
				return err
			}
			r.Runs += n
			n, err = maintenance.ExpireOffers(ctx, tx, now, actor)
			if err != nil {
				return err
			}
			r.ExpiredOffers += n
			if n, err = maintenance.ExpireProposals(ctx, tx, now); err != nil {
				return err
			}
			r.ExpiredProposals += n
			if n, err = maintenance.ConfirmUnrated(ctx, tx, now); err != nil {
				return err
			}
			r.Confirmed += n
			// freeze after expiry so histories see the final state of the tick
			n, err = maintenance.FreezeEnded(ctx, tx, now)
			r.FrozenHistories += n
			return err
		})
		if err != nil {
			return r, err
		}
	}
	return r, nil
}

// Run ticks every interval until ctx is cancelled.
func Run(ctx context.Context, m *db.TxManager, interval time.Duration, clock func() time.Time, logf func(string, ...any)) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if r, err := Tick(ctx, m, clock()); err != nil {
			logf("worker tick failed: %v", err)
		} else if r.ExpiredOffers+r.ExpiredCommands+r.Runs+r.ExpiredProposals+r.Confirmed+r.FrozenHistories > 0 {
			logf("worker tick: %+v", r)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
