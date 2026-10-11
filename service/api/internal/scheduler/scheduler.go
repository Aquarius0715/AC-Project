// Package scheduler runs the clock-driven transitions (worker tick, backend Go design §7). Each business domain with
// clock-driven work has its own scheduler (IR195): equipment expires commands and advances diagnostic runs,
// maintenance expires offers and proposals, confirms unrated jobs and freezes histories, billing generates the
// monthly payout drafts (IR341). A domain scheduler uses the domain's database role and asks other domains through
// the registry's internal queries; Tick (all domains, local) serves tests and single-process runs.
//
// Several instances of a domain's scheduler may run (IR356): each tenant transaction takes a transaction-scoped
// advisory lock per domain and tenant before it changes anything. The background loop skips a domain and tenant whose
// lock another instance holds, and its next tick catches up; Tick and TickDomains wait for the lock, so a demo clock
// jump or a test sees its transitions applied.
package scheduler

import (
	"context"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/control"
	"github.com/pradita/ac-project/service/api/internal/modules/devices"
	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/modules/restrictions"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
	"github.com/pradita/ac-project/service/api/internal/platform/db"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// Domains that have clock-driven transitions.
var Domains = []string{ops.DomainEquipment, ops.DomainMaintenance, ops.DomainBilling}

// Result counts the changes of one tick.
type Result struct {
	ExpiredOffers    int
	ExpiredCommands  int
	Runs             int
	Confirmed        int
	ExpiredProposals int
	FrozenHistories  int
	Jobs             int // changes of the registry's domain jobs (IR54 schedule automations)
	Skipped          int // domains × tenants left to the instance holding their lock (IR356)
}

func (r Result) changes() int {
	return r.ExpiredOffers + r.ExpiredCommands + r.Runs + r.ExpiredProposals + r.Confirmed + r.FrozenHistories + r.Jobs
}

// Tick runs every domain's clock-driven transitions once per tenant, each tenant in its own transaction (RLS
// context set), with local access to every domain; reg (may be nil) adds the modules' jobs (IR54).
func Tick(ctx context.Context, m *db.TxManager, now time.Time, reg *ops.Registry) (Result, error) {
	return TickDomains(ctx, m, now, nil, reg)
}

// TickDomains runs the transitions of the given domains (nil: all) once per tenant, waiting while another instance
// ticks the same domain and tenant. reg (may be nil) answers the internal queries of other domains, so a domain
// scheduler needs only its own role.
func TickDomains(ctx context.Context, m *db.TxManager, now time.Time, domains []string, reg *ops.Registry) (Result, error) {
	return tick(ctx, m, now, domains, reg, true)
}

// TryTickDomains is the background tick: a domain and tenant that another instance is ticking is skipped this time
// (Result.Skipped), never waited for (IR356).
func TryTickDomains(ctx context.Context, m *db.TxManager, now time.Time, domains []string, reg *ops.Registry) (Result, error) {
	return tick(ctx, m, now, domains, reg, false)
}

// lockSQL takes the advisory lock of one domain's scheduler for one tenant until the transaction ends: keyed by the
// hashes of "scheduler:<domain>" and the tenant ID. The try form answers whether the lock was free.
const (
	lockSQL    = `SELECT pg_advisory_xact_lock(hashtext('scheduler:' || $1), hashtext($2::text))`
	tryLockSQL = `SELECT pg_try_advisory_xact_lock(hashtext('scheduler:' || $1), hashtext($2::text))`
)

func tick(ctx context.Context, m *db.TxManager, now time.Time, domains []string, reg *ops.Registry, wait bool) (Result, error) {
	serves := func(d string) bool { return domains == nil || slices.Contains(domains, d) }
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
			// the domains this transaction ticks: those whose lock it holds, taken in the order of Domains (IR356)
			held := map[string]bool{}
			for _, d := range Domains {
				if !serves(d) {
					continue
				}
				if wait {
					if _, err := tx.Exec(ctx, lockSQL, d, t); err != nil {
						return err
					}
					held[d] = true
					continue
				}
				var ok bool
				if err := tx.QueryRow(ctx, tryLockSQL, d, t).Scan(&ok); err != nil {
					return err
				}
				held[d] = ok
				if !ok {
					r.Skipped++
				}
			}
			if held[ops.DomainEquipment] {
				n, err := control.ExpireCommands(ctx, tx, now)
				if err != nil {
					return err
				}
				r.ExpiredCommands += n
				c := &ops.Call{Tx: tx, Now: now, Principal: &ops.Principal{TenantID: t}, Queries: reg} // restriction policies come from billing
				if n, err = control.AdvanceRuns(ctx, c, control.Commands{Devices: devices.Models{}, Restrictions: restrictions.Busy{}}); err != nil {
					return err
				}
				r.Runs += n
			}
			if reg != nil { // the modules' jobs of the served domains, recorded like a write (events, audits)
				for _, d := range Domains {
					if !held[d] {
						continue
					}
					for _, job := range reg.Jobs[d] {
						c := &ops.Call{Tx: tx, Now: now, CorrelationID: uuid.NewString(), Queries: reg,
							Principal: &ops.Principal{Principal: authz.Principal{Role: "system"}, TenantID: t, UserID: seed.ID("system-scheduler")}}
						n, err := job(ctx, c)
						if err != nil {
							return err
						}
						r.Jobs += n
						if reg.Rec != nil {
							if err := reg.Rec.Record(ctx, tx, c, "scheduler"); err != nil {
								return err
							}
						}
					}
				}
			}
			if held[ops.DomainMaintenance] {
				n, err := maintenance.ExpireOffers(ctx, tx, now, actor)
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
				if err != nil {
					return err
				}
			}
			return nil
		})
		if err != nil {
			return r, err
		}
	}
	return r, nil
}

// Run ticks every domain every interval until ctx is cancelled.
func Run(ctx context.Context, m *db.TxManager, interval time.Duration, clock func() time.Time, logf func(string, ...any), reg *ops.Registry) {
	RunDomains(ctx, m, interval, clock, logf, nil, reg)
}

// RunDomains ticks the given domains every interval until ctx is cancelled.
func RunDomains(ctx context.Context, m *db.TxManager, interval time.Duration, clock func() time.Time, logf func(string, ...any), domains []string, reg *ops.Registry) {
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		if r, err := TryTickDomains(ctx, m, clock(), domains, reg); err != nil {
			logf("worker tick failed: %v", err)
		} else if r.changes() > 0 {
			logf("worker tick: %+v", r)
		}
		select {
		case <-ctx.Done():
			return
		case <-t.C:
		}
	}
}
