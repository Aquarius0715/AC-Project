package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
)

// ApplyFleet adds the mock fleet (internal/devicesim/fleet.json) to a seeded database: a property of the fleet's
// customer and, per state pattern, an air conditioner and its IoT module with the pattern's connection, power signal,
// tamper, firmware and observed state. Records that exist are kept (ON CONFLICT DO NOTHING), so it can run on every
// start. Development only: tests and production never load it.
func ApplyFleet(ctx context.Context, tx pgx.Tx, f devicesim.Fleet, at time.Time) (int, error) {
	var tenant string
	if err := tx.QueryRow(ctx, `SELECT tenant_id::text FROM assets.customers WHERE organization_id = $1`, ID(f.CustomerOrgID)).Scan(&tenant); err != nil {
		return 0, fmt.Errorf("fleet customer %s: %w", f.CustomerOrgID, err)
	}
	ex := func(sql string, args ...any) error { _, err := tx.Exec(ctx, sql, args...); return err }
	if err := ex(`INSERT INTO assets.properties (id, tenant_id, customer_org_id, kind, name, address, access_instructions, archived)
		VALUES ($1,$2,$3,$4,$5,NULL,NULL,false) ON CONFLICT DO NOTHING`, ID(f.Property.ID), tenant, ID(f.CustomerOrgID), f.Property.Kind, f.Property.Name); err != nil {
		return 0, fmt.Errorf("fleet property: %w", err)
	}
	scope := []string{"indoor", "outdoor", "electrical"}
	n := 0
	for i, p := range f.Patterns {
		state, _ := json.Marshal(map[string]any{"power": p.State.Power, "celsius": p.State.Celsius, "mode": p.State.Mode, "fanLevel": p.State.FanLevel, "observedAt": at.UTC()})
		unit := ID(devicesim.UnitName(p))
		if err := ex(`INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id, installed_at,
			service_scope, archived, capability_version, observed_state, observed_restriction, version, warranty_ends_at)
			VALUES ($1,$2,$3,$4,NULL,$5,$6,NULL,$7,$8,$9,$10,NULL,1,NULL) ON CONFLICT DO NOTHING`,
			unit, tenant, ID(f.CustomerOrgID), ID(f.Property.ID), p.Name, ID(f.ModelID), scope, p.Archived, f.CapabilityVersion, state); err != nil {
			return n, fmt.Errorf("fleet unit %s: %w", p.Key, err)
		}
		n++
		if p.NoDevice {
			continue
		}
		device, binding := ID(devicesim.DeviceName(p)), ID(devicesim.BindingName(p))
		if err := ex(`INSERT INTO devices.devices (id, tenant_id, serial, binding_id, unit_id, target_unit_id, created_by_membership_id, connection, last_seen_at,
			firmware_version, power_signal, tamper) VALUES ($1,$2,$3,$4,$5,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT DO NOTHING`,
			device, tenant, fmt.Sprintf("AC-MOCK-%04d", i+1), binding, unit, ID("system-demo"), p.Connection, at, f.FirmwareOf(p), p.PowerSignal, p.Tamper); err != nil {
			return n, fmt.Errorf("fleet device %s: %w", p.Key, err)
		}
		for _, s := range devicesim.Sensors {
			if err := ex(`INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, boundary_id, stale_after_seconds, calibrated_at)
				VALUES ($1,$2,$3,$4,$5,NULL,120,NULL) ON CONFLICT DO NOTHING`, ID(devicesim.SensorName(p, s)), tenant, device, s.Metric, s.Unit); err != nil {
				return n, fmt.Errorf("fleet sensor %s/%s: %w", p.Key, s.Metric, err)
			}
		}
		if err := ex(`INSERT INTO devices.device_bindings (id, tenant_id, device_id, unit_id, customer_org_id, bound_at, reason, actor_membership_id)
			VALUES ($1,$2,$3,$4,$5,$6,'mock fleet',$7) ON CONFLICT DO NOTHING`, binding, tenant, device, unit, ID(f.CustomerOrgID), at, ID("system-demo")); err != nil {
			return n, fmt.Errorf("fleet binding %s: %w", p.Key, err)
		}
		if err := ex(`UPDATE assets.units SET connection = $2, last_seen_at = $3 WHERE id = $1`, unit, p.Connection, at); err != nil {
			return n, fmt.Errorf("fleet unit connection %s: %w", p.Key, err)
		}
	}
	return n, nil
}
