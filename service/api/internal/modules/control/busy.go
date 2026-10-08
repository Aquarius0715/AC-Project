// Package control implements the Control module (commands, diagnostic runs, automations, voice).
package control

import (
	"context"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// Busy reports in-flight control work on a unit (D05 mutual exclusion): Commands requested/sent and DiagnosticRuns
// awaiting_start/running/end_requested.
type Busy struct{}

// UnitBusy implements devices.Exclusion.
func (Busy) UnitBusy(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	var b bool
	err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM control.commands WHERE unit_id = $1 AND status IN ('requested','sent'))
		OR EXISTS (SELECT 1 FROM control.diagnostic_runs WHERE unit_id = $1 AND state IN ('awaiting_start','running','end_requested'))`, unit).Scan(&b)
	return b, err
}

// UnitActive implements assets.UnitActivity (same rule as UnitBusy).
func (b Busy) UnitActive(ctx context.Context, c *ops.Call, unit uuid.UUID) (bool, error) {
	return b.UnitBusy(ctx, c, unit)
}
