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
	"errors"
	"fmt"
	"slices"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/control"
	"github.com/pradita/ac-project/service/api/internal/modules/devices"
	"github.com/pradita/ac-project/service/api/internal/modules/maintenance"
	"github.com/pradita/ac-project/service/api/internal/modules/restrictions"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/db"
	"github.com/pradita/ac-project/service/api/internal/platform/logging"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// Domains are the domains that have clock-driven transitions, in the order a tick takes their locks (IR356).
func Domains() []string {
	return []string{ops.DomainEquipment, ops.DomainMaintenance, ops.DomainBilling}
}

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
func TickDomains(
	ctx context.Context,
	m *db.TxManager,
	now time.Time,
	domains []string,
	reg *ops.Registry,
) (Result, error) {
	return tick(ctx, m, now, domains, reg, true)
}

// TryTickDomains is the background tick: a domain and tenant that another instance is ticking is skipped this time
// (Result.Skipped), never waited for (IR356).
func TryTickDomains(
	ctx context.Context,
	m *db.TxManager,
	now time.Time,
	domains []string,
	reg *ops.Registry,
) (Result, error) {
	return tick(ctx, m, now, domains, reg, false)
}

// lockSQL takes the advisory lock of one domain's scheduler for one tenant until the transaction ends: keyed by the
// hashes of "scheduler:<domain>" and the tenant ID. The try form answers whether the lock was free.
const (
	lockSQL    = `SELECT pg_advisory_xact_lock(hashtext('scheduler:' || $1), hashtext($2::text))`
	tryLockSQL = `SELECT pg_try_advisory_xact_lock(hashtext('scheduler:' || $1), hashtext($2::text))`
)

func tick(
	ctx context.Context,
	txm *db.TxManager,
	now time.Time,
	domains []string,
	reg *ops.Registry,
	wait bool,
) (Result, error) {
	tenants, err := tenantIDs(ctx, txm)
	if err != nil {
		return Result{}, err
	}

	var (
		result Result
		failed []error
	)

	// each tenant in its own transaction, and one that fails does not hold up the others: its failure is reported and
	// its transaction rolled back, and the next tick tries it again
	for _, tenant := range tenants {
		err := txm.Run(ctx, false, ops.TenantPrincipal(tenant), func(txn pgx.Tx) error {
			_, err := txn.Exec(
				ctx,
				`SELECT set_config('app.now', $1, true)`,
				now.UTC().Format(time.RFC3339Nano),
			)
			if err != nil {
				return apperr.Wrap(err, "select set_config")
			}

			held, err := lockDomains(ctx, txn, tenant, domains, wait, &result)
			if err != nil {
				return err
			}

			if held[ops.DomainEquipment] {
				err := tickEquipment(ctx, txn, tenant, now, reg, &result)
				if err != nil {
					return err
				}
			}

			if reg != nil {
				err := runJobs(ctx, txn, tenant, now, reg, held, &result)
				if err != nil {
					return err
				}
			}

			if held[ops.DomainMaintenance] {
				return tickMaintenance(ctx, txn, now, &result)
			}

			return nil
		})
		if err != nil {
			failed = append(failed, fmt.Errorf("tenant %s: %w", tenant, err))
		}
	}

	return result, apperr.Wrap(errors.Join(failed...), "db.TxManager.Run")
}

// tenantIDs are the tenants a tick visits, in ID order.
func tenantIDs(ctx context.Context, txm *db.TxManager) ([]uuid.UUID, error) {
	rows, err := txm.Writer.Query(ctx, `SELECT id FROM platform.tenants ORDER BY id`)
	if err != nil {
		return nil, apperr.Wrap(err, "select platform.tenants")
	}

	tenants, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return nil, apperr.Wrap(err, "select platform.tenants")
	}

	return tenants, nil
}

// lockDomains takes the tenant's lock of each served domain, in the order of Domains (IR356), and returns the
// domains whose lock the transaction holds. Waiting takes every lock; otherwise a domain that another instance is
// ticking is skipped (Result.Skipped).
func lockDomains(
	ctx context.Context,
	txn pgx.Tx,
	tenant uuid.UUID,
	domains []string,
	wait bool,
	result *Result,
) (map[string]bool, error) {
	held := map[string]bool{}

	for _, domain := range Domains() {
		if domains != nil && !slices.Contains(domains, domain) {
			continue
		}

		if wait {
			_, err := txn.Exec(ctx, lockSQL, domain, tenant)
			if err != nil {
				return nil, apperr.Wrap(err, "select pg_advisory_xact_lock")
			}

			held[domain] = true

			continue
		}

		var ok bool

		err := txn.QueryRow(ctx, tryLockSQL, domain, tenant).Scan(&ok)
		if err != nil {
			return nil, apperr.Wrap(err, "select pg_try_advisory_xact_lock")
		}

		held[domain] = ok
		if !ok {
			result.Skipped++
		}
	}

	return held, nil
}

// tickEquipment expires the commands whose time ran out and advances the diagnostic runs; the restriction policies
// the runs check come from billing through the registry's queries.
func tickEquipment(
	ctx context.Context,
	txn pgx.Tx,
	tenant uuid.UUID,
	now time.Time,
	reg *ops.Registry,
	result *Result,
) error {
	expired, err := control.ExpireCommands(ctx, txn, now)
	if err != nil {
		return apperr.Wrap(err, "control.ExpireCommands")
	}

	result.ExpiredCommands += expired

	call := &ops.Call{
		Tx:              txn,
		Now:             now,
		Principal:       ops.TenantPrincipal(tenant),
		Queries:         reg,
		CorrelationID:   "",
		Bearer:          "",
		ExpectedVersion: nil,
		Candidates:      nil,
		Events:          nil,
		Audits:          nil,
	}

	runs, err := control.AdvanceRuns(
		ctx,
		call,
		control.Commands{
			Devices:      devices.Models{},
			Restrictions: restrictions.Busy{},
			Units:        nil,
			Access:       nil,
		},
	)
	if err != nil {
		return apperr.Wrap(err, "control.AdvanceRuns")
	}

	result.Runs += runs

	return nil
}

// runJobs runs the modules' jobs of the domains whose lock the transaction holds, each recorded like a write (its
// events and audits) as the scheduler's system user.
func runJobs(
	ctx context.Context,
	txn pgx.Tx,
	tenant uuid.UUID,
	now time.Time,
	reg *ops.Registry,
	held map[string]bool,
	result *Result,
) error {
	for _, domain := range Domains() {
		if !held[domain] {
			continue
		}

		for _, job := range reg.Jobs[domain] {
			call := &ops.Call{
				Tx:              txn,
				Now:             now,
				CorrelationID:   uuid.NewString(),
				Queries:         reg,
				Principal:       ops.SystemPrincipal(tenant, seed.ID("system-scheduler")),
				Bearer:          "",
				ExpectedVersion: nil,
				Candidates:      nil,
				Events:          nil,
				Audits:          nil,
			}

			changed, err := job(ctx, call)
			if err != nil {
				return err
			}

			result.Jobs += changed

			if reg.Rec != nil {
				err := reg.Rec.Record(ctx, txn, call, "scheduler")
				if err != nil {
					return apperr.Wrap(err, "ops.Recorder.Record")
				}
			}
		}
	}

	return nil
}

// tickMaintenance expires the offers and proposals whose time ran out, confirms the jobs left unrated, then freezes
// the histories that ended, after the expiries, so that they keep the tick's final state.
func tickMaintenance(ctx context.Context, txn pgx.Tx, now time.Time, result *Result) error {
	expiredOffers, err := maintenance.ExpireOffers(ctx, txn, now, seed.ID("system-demo"))
	if err != nil {
		return apperr.Wrap(err, "maintenance.ExpireOffers")
	}

	result.ExpiredOffers += expiredOffers

	expiredProposals, err := maintenance.ExpireProposals(ctx, txn, now)
	if err != nil {
		return apperr.Wrap(err, "maintenance.ExpireProposals")
	}

	result.ExpiredProposals += expiredProposals

	confirmed, err := maintenance.ConfirmUnrated(ctx, txn, now)
	if err != nil {
		return apperr.Wrap(err, "maintenance.ConfirmUnrated")
	}

	result.Confirmed += confirmed

	frozen, err := maintenance.FreezeEnded(ctx, txn, now)
	result.FrozenHistories += frozen

	return apperr.Wrap(err, "maintenance.FreezeEnded")
}

// RunDomains ticks the given domains (every domain when domains is nil) every interval until ctx is cancelled.
func RunDomains(
	ctx context.Context,
	txm *db.TxManager,
	interval time.Duration,
	clock func() time.Time,
	logf func(string, ...any),
	domains []string,
	reg *ops.Registry,
) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	rep := &logging.Repeats{
		Logf: logf, Every: 0, Now: nil,
	} // a failure that repeats every second is logged once a minute (IR429)

	for {
		result, err := TryTickDomains(ctx, txm, clock(), domains, reg)
		if err != nil {
			rep.Failed(fmt.Sprintf("worker tick failed: %v", err))
		} else {
			rep.OK("worker tick")

			if result.changes() > 0 {
				logf("worker tick: %+v", result)
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
		}
	}
}
