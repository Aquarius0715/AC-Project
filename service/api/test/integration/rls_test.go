package integration

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestTenantIsolationEverywhere: every business table with tenant_id enforces row-level security, partitions included
// (IR376). A partition is a table of its own; read directly, it does not apply its parent's policy. So a partition
// without one would show a service role every tenant's rows. platform.* stays outside: the relay and the scheduler
// read it across tenants and set the tenant per item.
func TestTenantIsolationEverywhere(t *testing.T) {
	ctx := context.Background()
	conn, err := pgx.Connect(ctx, testDB("postgres"))
	if err != nil {
		t.Skip(err)
	}
	defer conn.Close(ctx)
	rows, err := conn.Query(ctx, `SELECT n.nspname || '.' || c.relname, c.relispartition, c.relrowsecurity, c.relforcerowsecurity,
		EXISTS (SELECT 1 FROM pg_policies p WHERE p.schemaname = n.nspname AND p.tablename = c.relname AND p.policyname = 'tenant_isolation')
		FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
		WHERE c.relkind IN ('r','p') AND n.nspname NOT IN ('pg_catalog','information_schema','platform') AND n.nspname NOT LIKE 'pg_%'
		  AND EXISTS (SELECT 1 FROM pg_attribute a WHERE a.attrelid = c.oid AND a.attname = 'tenant_id' AND NOT a.attisdropped)`)
	if err != nil {
		t.Fatal(err)
	}
	tables, partitions := 0, 0
	for rows.Next() {
		var name string
		var partition, rls, force, policy bool
		if err := rows.Scan(&name, &partition, &rls, &force, &policy); err != nil {
			t.Fatal(err)
		}
		tables++
		if partition {
			partitions++
		}
		if !rls || !force || !policy {
			t.Errorf("%s (partition %v): rls %v, forced %v, tenant_isolation %v", name, partition, rls, force, policy)
		}
	}
	if rows.Err() != nil {
		t.Fatal(rows.Err())
	}
	if tables < 100 || partitions == 0 {
		t.Fatalf("checked %d tables, %d partitions", tables, partitions)
	}

	// read directly, the default partition of the measurements shows a service role its own tenant only
	other := seed.ID("tenant-b")
	event := uuid.NewString()
	owner(t, `INSERT INTO monitoring.measurements (tenant_id, unit_id, sensor_id, metric, observed_at, sequence, value, unit, origin, quality, received_at, event_id)
		VALUES ($1,$2,$3,'temperature',$4,1,21.5,'°C','measured','valid',$4,$5)`, other, seed.ID("unit-tenant-b"), uuid.NewString(), time.Date(2026, 9, 14, 0, 0, 0, 0, time.UTC), event)
	t.Cleanup(func() { owner(t, `DELETE FROM monitoring.measurements WHERE event_id = $1`, event) })
	app, err := pgx.Connect(ctx, testDB("ac_app_login"))
	if err != nil {
		t.Skip(err)
	}
	defer app.Close(ctx)
	tx, err := app.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `SELECT set_config('app.tenant_id', $1, true)`, seed.ID("tenant-a").String()); err != nil {
		t.Fatal(err)
	}
	for _, table := range []string{"monitoring.measurements", "monitoring.measurements_default"} {
		var n int
		if err := tx.QueryRow(ctx, `SELECT count(*) FROM `+table+` WHERE event_id = $1`, event).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n != 0 {
			t.Errorf("%s shows tenant A's service role %d row(s) of tenant B", table, n)
		}
	}
}
