package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
)

// LiveSensor is one sensor of the live site's module, by its fixture-style name.
type LiveSensor struct {
	Name     string // fixture-style name; the API ID is ID(Name)
	Metric   string
	Unit     string
	Boundary string // energy boundary ("" for none)
}

// LiveSite is where the readings of real sensor nodes land (DEC-86, IR524): one monitoring-only air conditioner of
// customer-b with one module whose sensors take the nodes' room temperature, humidity and power draw. The IoT bridge
// subscribes to the nodes' broker (REAL_MQTT_URL) and sends their readings to these sensors. Development only.
var LiveSite = struct {
	CustomerOrgID, Capability, Property, Floor, Room, Unit, Device, Binding string
	Sensors                                                                 []LiveSensor
}{
	CustomerOrgID: "org-customer-b",
	Capability:    "cap-live-sensor-kit",
	Property:      "property-live-sensors",
	Floor:         "space-live-1f",
	Room:          "space-live-room",
	Unit:          "unit-live-ac",
	Device:        "device-live-ac",
	Binding:       "binding-live-ac",
	Sensors: []LiveSensor{
		{Name: "sensor-live-temperature", Metric: "temperature", Unit: "°C"},
		{Name: "sensor-live-humidity", Metric: "humidity", Unit: "%"},
		{Name: "sensor-live-power", Metric: "power", Unit: "kW", Boundary: "ac_input_electricity"},
	},
}

// ApplyLive adds the live site to a seeded database: a monitoring-only model (no remote control: the nodes only
// measure), a property with one room, the air conditioner and its module, online since at. Records that exist are
// kept (ON CONFLICT DO NOTHING), so it can run on every start. Development only: tests and production never load it.
func ApplyLive(ctx context.Context, tx pgx.Tx, at time.Time) error {
	s := LiveSite
	var tenant string
	if err := tx.QueryRow(ctx, `SELECT tenant_id::text FROM assets.customers WHERE organization_id = $1`, ID(s.CustomerOrgID)).Scan(&tenant); err != nil {
		return fmt.Errorf("live site customer %s: %w", s.CustomerOrgID, err)
	}
	ex := func(what, sql string, args ...any) error {
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			return fmt.Errorf("live site %s: %w", what, err)
		}
		return nil
	}
	type capSensor struct {
		Metric            string  `json:"metric"`
		Unit              string  `json:"unit"`
		StaleAfterSeconds int     `json:"staleAfterSeconds"`
		BoundaryID        *string `json:"boundaryId"`
	}
	var sensors []capSensor
	for _, x := range s.Sensors {
		cs := capSensor{Metric: x.Metric, Unit: x.Unit, StaleAfterSeconds: 120}
		if x.Boundary != "" {
			cs.BoundaryID = &x.Boundary
		}
		sensors = append(sensors, cs)
	}
	capSensors, err := json.Marshal(sensors)
	if err != nil {
		return err
	}
	if err := ex("model", `INSERT INTO devices.capabilities (id, version, tenant_id, manufacturer, model, control, mode_control, fan_control, temperature,
		modes, fan_levels, ventilation, ventilation_levels, sensors, firmware_candidates, is_current)
		VALUES ($1,1,$2,'Live','Sensor kit',false,false,false,NULL,'{}','{}',false,'{}',$3,'{}',true) ON CONFLICT DO NOTHING`,
		ID(s.Capability), tenant, capSensors); err != nil {
		return err
	}
	if err := ex("property", `INSERT INTO assets.properties (id, tenant_id, customer_org_id, kind, name, address, access_instructions, archived)
		VALUES ($1,$2,$3,'office','Live sensors',NULL,NULL,false) ON CONFLICT DO NOTHING`, ID(s.Property), tenant, ID(s.CustomerOrgID)); err != nil {
		return err
	}
	if err := ex("floor", `INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name) VALUES ($1,$2,$3,NULL,'floor','1F') ON CONFLICT DO NOTHING`,
		ID(s.Floor), tenant, ID(s.Property)); err != nil {
		return err
	}
	if err := ex("room", `INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name) VALUES ($1,$2,$3,$4,'room','Sensor room') ON CONFLICT DO NOTHING`,
		ID(s.Room), tenant, ID(s.Property), ID(s.Floor)); err != nil {
		return err
	}
	// the nodes report no settings of the air conditioner: its observed state stays unknown
	state, _ := json.Marshal(map[string]any{"power": nil, "celsius": nil, "mode": nil, "fanLevel": nil, "observedAt": at.UTC()})
	unit := ID(s.Unit)
	if err := ex("unit", `INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id, installed_at,
		service_scope, archived, capability_version, observed_state, observed_restriction, version, warranty_ends_at)
		VALUES ($1,$2,$3,$4,$5,'Live · AC sensors',$6,NULL,'{indoor,outdoor,electrical}',false,1,$7,NULL,1,NULL) ON CONFLICT DO NOTHING`,
		unit, tenant, ID(s.CustomerOrgID), ID(s.Property), ID(s.Room), ID(s.Capability), state); err != nil {
		return err
	}
	device, binding := ID(s.Device), ID(s.Binding)
	added, err := tx.Exec(ctx, `INSERT INTO devices.devices (id, tenant_id, serial, binding_id, unit_id, target_unit_id, created_by_membership_id, connection, last_seen_at,
		firmware_version, power_signal, tamper) VALUES ($1,$2,'AC-LIVE-0001',$3,$4,$4,$5,'online',$6,'external','unknown','clear') ON CONFLICT DO NOTHING`,
		device, tenant, binding, unit, ID("system-demo"), at)
	if err != nil {
		return fmt.Errorf("live site module: %w", err)
	}
	for _, x := range s.Sensors {
		var boundary *string
		if x.Boundary != "" {
			boundary = &x.Boundary
		}
		if err := ex("sensor "+x.Metric, `INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, boundary_id, stale_after_seconds, calibrated_at)
			VALUES ($1,$2,$3,$4,$5,$6,120,NULL) ON CONFLICT DO NOTHING`, ID(x.Name), tenant, device, x.Metric, x.Unit, boundary); err != nil {
			return err
		}
	}
	if err := ex("binding", `INSERT INTO devices.device_bindings (id, tenant_id, device_id, unit_id, customer_org_id, bound_at, reason, actor_membership_id)
		VALUES ($1,$2,$3,$4,$5,$6,'live sensors',$7) ON CONFLICT DO NOTHING`, binding, tenant, device, unit, ID(s.CustomerOrgID), at, ID("system-demo")); err != nil {
		return err
	}
	if added.RowsAffected() == 0 { // a module from an earlier start keeps its connection and last seen
		return nil
	}
	// the unit mirrors its new module's connection and last seen (IR47)
	return ex("unit connection", `UPDATE assets.units SET connection = 'online', last_seen_at = $2 WHERE id = $1`, unit, at)
}
