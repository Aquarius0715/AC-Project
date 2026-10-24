package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/monitoring"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestConcurrentAlertOpening (IR460): two tamper events of one unit opening its alert at the same time end with one
// open alert and both see it; the second one used to fail on alerts_one_open_per_rule.
func TestConcurrentAlertOpening(t *testing.T) {
	ctx := context.Background()
	connect := func() *pgx.Conn {
		conn, err := pgx.Connect(ctx, testDB("postgres"))
		if err != nil {
			t.Skip(err)
		}
		t.Cleanup(func() { conn.Close(ctx) })
		return conn
	}
	c1, c2 := connect(), connect()
	unit, org, tenant := seed.ID("unit-online-rto"), seed.ID("org-customer-a"), seed.ID("tenant-a")
	owner(t, `UPDATE monitoring.alerts SET status = 'resolved', resolved_at = now() WHERE unit_id = $1 AND type = 'tamper' AND status <> 'resolved'`, unit)
	t.Cleanup(func() {
		owner(t, `UPDATE monitoring.alerts SET status = 'resolved', resolved_at = now() WHERE unit_id = $1 AND type = 'tamper' AND status <> 'resolved'`, unit)
	})
	begin := func(conn *pgx.Conn) pgx.Tx {
		tx, err := conn.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, tenant.String()); err != nil {
			t.Fatal(err)
		}
		return tx
	}
	at := clock
	tx1 := begin(c1)
	first, err := monitoring.Alerts{}.TamperAlert(ctx, tx1, unit, org, "race 1", at)
	if err != nil {
		t.Fatal(err)
	}
	type result struct {
		id  uuid.UUID
		err error
	}
	done := make(chan result, 1)
	tx2 := begin(c2)
	go func() { // waits on the first transaction's row in the unique index
		id, err := monitoring.Alerts{}.TamperAlert(ctx, tx2, unit, org, "race 2", at)
		if err == nil {
			err = tx2.Commit(ctx)
		} else {
			_ = tx2.Rollback(ctx)
		}
		done <- result{id, err}
	}()
	time.Sleep(300 * time.Millisecond)
	if err := tx1.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	r := <-done
	if r.err != nil || r.id != first {
		t.Errorf("second opening: %v, id %s (first %s)", r.err, r.id, first)
	}
	var open int
	ownerScan(t, `SELECT count(*) FROM monitoring.alerts WHERE unit_id = $1 AND type = 'tamper' AND status <> 'resolved'`, []any{unit}, &open)
	if open != 1 {
		t.Errorf("open tamper alerts: %d", open)
	}
}
