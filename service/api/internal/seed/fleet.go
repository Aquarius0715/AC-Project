package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/devicesim"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// ApplyFleet adds the mock fleet (internal/devicesim/fleet.json) to a seeded database: a property of the fleet's
// customer and, per state pattern, an air conditioner and its IoT module with the pattern's connection, power signal,
// tamper, firmware and observed state as of now. Records that exist are kept (ON CONFLICT DO NOTHING), so it can run
// on every start. Development only: tests and production never load it.
func ApplyFleet(
	ctx context.Context,
	tx pgx.Tx,
	fleet devicesim.Fleet,
	now time.Time,
) (int, error) {
	var tenant string

	err := tx.QueryRow(ctx, `SELECT tenant_id::text FROM assets.customers WHERE organization_id =
		$1`, ID(fleet.CustomerOrgID)).
		Scan(&tenant)
	if err != nil {
		return 0, fmt.Errorf("fleet customer %s: %w", fleet.CustomerOrgID, err)
	}

	exec := func(sql string, args ...any) error {
		_, err := tx.Exec(ctx, sql, args...)

		return apperr.Wrap(err, "exec (ApplyFleet)")
	}

	err = fleetPlace(exec, fleet, tenant)
	if err != nil {
		return 0, err
	}

	n := 0

	for i, pattern := range fleet.Patterns {
		err := fleetUnit(exec, fleet, tenant, pattern, now)
		if err != nil {
			return n, err
		}

		n++

		if pattern.NoDevice {
			continue
		}

		err = fleetModule(exec, fleet, tenant, pattern, fmt.Sprintf("AC-MOCK-%04d", i+1), now)
		if err != nil {
			return n, err
		}
	}

	for _, statement := range iotModelStatements() {
		err := exec(statement)
		if err != nil {
			return n, fmt.Errorf("fleet iot models: %w", err)
		}
	}

	return n, nil
}

// fleetPlace adds the fleet's property with its floors and their rooms (parents before children).
func fleetPlace(exec execSQL, fleet devicesim.Fleet, tenant string) error {
	err := exec(
		`INSERT INTO assets.properties (id, tenant_id, customer_org_id, kind, name, address, access_instructions, archived)
		VALUES ($1,$2,$3,$4,$5,NULL,NULL,false) ON CONFLICT DO NOTHING`,
		ID(fleet.Property.ID),
		tenant,
		ID(fleet.CustomerOrgID),
		fleet.Property.Kind,
		fleet.Property.Name,
	)
	if err != nil {
		return fmt.Errorf("fleet property: %w", err)
	}

	for _, floor := range fleet.Floors {
		err := exec(
			`INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name) VALUES ($1,$2,$3,NULL,'floor',
				$4) ON CONFLICT DO NOTHING`,
			ID(devicesim.SpaceName(floor.Key)),
			tenant,
			ID(fleet.Property.ID),
			floor.Name,
		)
		if err != nil {
			return fmt.Errorf("fleet floor %s: %w", floor.Key, err)
		}

		for _, room := range floor.Rooms {
			err := exec(
				`INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name) VALUES ($1,$2,$3,$4,'room',$5)
					ON CONFLICT DO NOTHING`,
				ID(
					devicesim.SpaceName(room.Key),
				),
				tenant,
				ID(fleet.Property.ID),
				ID(devicesim.SpaceName(floor.Key)),
				room.Name,
			)
			if err != nil {
				return fmt.Errorf("fleet room %s: %w", room.Key, err)
			}
		}
	}

	return nil
}

// fleetUnit adds the pattern's air conditioner in its room, with the pattern's observed state as of now.
func fleetUnit(
	exec execSQL,
	fleet devicesim.Fleet,
	tenant string,
	pattern devicesim.Pattern,
	now time.Time,
) error {
	scope := []string{"indoor", "outdoor", "electrical"}

	state, marshalErr := json.Marshal(
		map[string]any{
			keyPower:      pattern.State.Power,
			"celsius":     pattern.State.Celsius,
			"mode":        pattern.State.Mode,
			"fanLevel":    pattern.State.FanLevel,
			keyObservedAt: now.UTC(),
		},
	)
	if marshalErr != nil {
		return apperr.Wrap(marshalErr, "json.Marshal")
	}

	unit := ID(devicesim.UnitName(pattern))

	space := ID(devicesim.SpaceName(pattern.Room))

	err := exec(
		`INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id,
			installed_at,
		service_scope, archived, capability_version, observed_state, observed_restriction, version, warranty_ends_at)
		VALUES ($1,$2,$3,$4,$11,$5,$6,NULL,$7,$8,$9,$10,NULL,1,NULL) ON CONFLICT DO NOTHING`,
		unit,
		tenant,
		ID(fleet.CustomerOrgID),
		ID(fleet.Property.ID),
		pattern.Name,
		ID(fleet.ModelID),
		scope,
		pattern.Archived,
		fleet.CapabilityVersion,
		state,
		space,
	)
	if err != nil {
		return fmt.Errorf("fleet unit %s: %w", pattern.Key, err)
	}
	// a unit from before the fleet had rooms moves into its room (a room someone set now is kept)
	err = exec(
		`UPDATE assets.units SET space_id = $2, version = version + 1 WHERE id = $1 AND space_id IS NULL`,
		unit,
		space,
	)
	if err != nil {
		return fmt.Errorf("fleet unit room %s: %w", pattern.Key, err)
	}

	return nil
}

// fleetModule adds the pattern's IoT module with its serial, sensors and binding; the unit mirrors the pattern's
// connection, and its last seen never moves back.
func fleetModule(
	exec execSQL,
	fleet devicesim.Fleet,
	tenant string,
	pattern devicesim.Pattern,
	serial string,
	now time.Time,
) error {
	unit := ID(devicesim.UnitName(pattern))
	device, binding := ID(devicesim.DeviceName(pattern)), ID(devicesim.BindingName(pattern))

	err := exec(
		`INSERT INTO devices.devices (id, tenant_id, serial, binding_id, unit_id, target_unit_id, created_by_membership_id,
			connection, last_seen_at,
		firmware_version, power_signal, tamper) VALUES ($1,$2,$3,$4,$5,$5,$6,$7,$8,$9,$10,$11) ON CONFLICT DO NOTHING`,
		device,
		tenant,
		serial,
		binding,
		unit,
		ID("system-demo"),
		pattern.Connection,
		now,
		fleet.FirmwareOf(pattern),
		pattern.PowerSignal,
		pattern.Tamper,
	)
	if err != nil {
		return fmt.Errorf("fleet device %s: %w", pattern.Key, err)
	}

	for _, sensor := range devicesim.Sensors() {
		var boundary *string
		if sensor.Boundary != "" {
			boundary = &sensor.Boundary
		}

		err := exec(
			`INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, boundary_id, stale_after_seconds,
				calibrated_at)
			VALUES ($1,$2,$3,$4,$5,$6,120,NULL) ON CONFLICT (id) DO UPDATE SET boundary_id = EXCLUDED.boundary_id`,
			ID(devicesim.SensorName(pattern, sensor)),
			tenant,
			device,
			sensor.Metric,
			sensor.Unit,
			boundary,
		)
		if err != nil {
			return fmt.Errorf("fleet sensor %s/%s: %w", pattern.Key, sensor.Metric, err)
		}
	}

	err = exec(
		`INSERT INTO devices.device_bindings (id, tenant_id, device_id, unit_id, customer_org_id, bound_at, reason,
			actor_membership_id)
		VALUES ($1,$2,$3,$4,$5,$6,'mock fleet',$7) ON CONFLICT DO NOTHING`,
		binding,
		tenant,
		device,
		unit,
		ID(fleet.CustomerOrgID),
		now,
		ID("system-demo"),
	)
	if err != nil {
		return fmt.Errorf("fleet binding %s: %w", pattern.Key, err)
	}

	err = exec(
		`UPDATE assets.units SET connection = $2, last_seen_at = GREATEST(COALESCE(last_seen_at, $3), $3) WHERE id = $1`,
		unit,
		pattern.Connection,
		now,
	)
	if err != nil { // never moves last seen back
		return fmt.Errorf("fleet unit connection %s: %w", pattern.Key, err)
	}

	return nil
}
