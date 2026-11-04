package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// LiveSensor is one sensor of the live site's module, by its fixture-style name.
type LiveSensor struct {
	Name     string // fixture-style name; the API ID is ID(Name)
	Metric   string
	Unit     string
	Boundary string // energy boundary ("" for none)
}

// LiveModule is a unit of the live site with its model and its module, by fixture-style names: the air conditioner,
// or a node's own sensor unit. A module has one sensor of a metric, so the readings of a node that repeat another
// node's metrics go to a unit of their own (DEC-91).
type LiveModule struct {
	Unit, UnitName, Device, Serial, Binding string
	Capability, Manufacturer, Model         string
	Control                                 bool // the model takes power commands
	Sensors                                 []LiveSensor
}

// Site is a site the seed adds outside the fixture, by the fixture-style names of its records: the fixture customer it
// belongs to and that customer's client, its place, the air conditioner (the embedded module) and the nodes' own
// sensor units.
type Site struct {
	LiveModule

	Tenant, CustomerOrgID, Customer, CustomerName string
	Client                                        string // the customer's client membership in the fixture
	Property, Floor, Room                         string
	SensorUnits                                   []LiveModule
}

// Modules are the site's units with their modules: the air conditioner first.
func (site Site) Modules() []LiveModule {
	return append([]LiveModule{site.LiveModule}, site.SensorUnits...)
}

// UnitNames are the names of the site's units, in the order of Modules.
func (site Site) UnitNames() []string {
	names := make([]string, 0, len(site.SensorUnits)+1)
	for _, module := range site.Modules() {
		names = append(names, module.UnitName)
	}

	return names
}

// liveStaleAfterSeconds is when a live reading goes stale: two missed minutes (the bridge sends one reading a minute).
const liveStaleAfterSeconds = 120

// liveExec runs one statement of the live seed; what names the record in its error.
type liveExec func(what, sql string, args ...any) error

// LiveSite is where the readings of real sensor nodes land (DEC-86, IR524): a property of the fixture's Demo Customer
// B, so that its client customer-b sees the real devices while customer-a keeps the demo data (DEC-92, IR628), with one
// Daikin air conditioner (DEC-89, IR528). The two nodes in front of it make one module: node_1 measures the
// room, node_2 the air conditioner's supply, and node_1's infrared LED switches it on and off. The model offers the
// power only, the one setting the nodes' firmware takes (ON, OFF and a cooling preset). node_2 measures the room too
// since November 2026; its temperature and humidity go to a sensor unit of its own in the same room (DEC-91, IR627).
// The IoT bridge subscribes to the nodes' broker (REAL_MQTT_URL), sends their readings to these sensors and the power
// commands to node_1. Development only.
func LiveSite() Site {
	return Site{
		Tenant:        demoTenant,
		CustomerOrgID: "org-customer-b",
		Customer:      "cust-b",
		CustomerName:  "Demo Customer B",
		Client:        "customer-b",
		Property:      "property-live-sensors",
		Floor:         "space-live-1f",
		Room:          "space-live-room",
		LiveModule: LiveModule{
			Capability:   "cap-live-daikin",
			Manufacturer: "Daikin",
			Model:        "IR via node_1",
			Control:      true,
			UnitName:     "Live · Daikin AC",
			Serial:       "node_1+node_2",
			Unit:         "unit-live-ac",
			Device:       "device-live-ac",
			Binding:      "binding-live-ac",
			Sensors: []LiveSensor{
				{Name: "sensor-live-temperature", Metric: keyTemperature, Unit: "°C", Boundary: ""},
				{Name: "sensor-live-humidity", Metric: keyHumidity, Unit: "%", Boundary: ""},
				{
					Name:     "sensor-live-power",
					Metric:   keyPower,
					Unit:     "kW",
					Boundary: "ac_input_electricity",
				},
			},
		},
		SensorUnits: []LiveModule{
			{
				Capability:   "cap-live-node-2-sensors",
				Manufacturer: "ESP32 node",
				Model:        "BME680 on node_2",
				Control:      false,
				UnitName:     "Live · node_2 sensors",
				Serial:       "node_2",
				Unit:         "unit-live-node-2-sensors",
				Device:       "device-live-node-2-sensors",
				Binding:      "binding-live-node-2-sensors",
				Sensors: []LiveSensor{
					{
						Name:     "sensor-live-node-2-temperature",
						Metric:   keyTemperature,
						Unit:     "°C",
						Boundary: "",
					},
					{
						Name:     "sensor-live-node-2-humidity",
						Metric:   keyHumidity,
						Unit:     "%",
						Boundary: "",
					},
				},
			},
		},
	}
}

// ApplyLive adds the live site to a database with the fixture: a property of the fixture customer with one room, and in
// it the air conditioner (remote power only) and the nodes' sensor units, each with its model and its module, online
// as of now. The customer's client keeps the access without an end date, so the real devices stay in view after the
// fixture's access ends (DEC-92); an end date set since is cleared at the next start. Records that exist are kept (ON
// CONFLICT DO NOTHING), so it can run on every start, and a unit added to the site comes in at the next start.
// Development only: tests and production never load it.
func ApplyLive(ctx context.Context, txn pgx.Tx, now time.Time) error {
	site := LiveSite()
	tenant := ID(site.Tenant)
	exec := func(what, sql string, args ...any) error {
		_, err := txn.Exec(ctx, sql, args...)
		if err != nil {
			return fmt.Errorf("live site %s: %w", what, err)
		}

		return nil
	}
	// the client sees the real devices beyond the fixture's end of access (DEC-92)
	err := exec(
		"client access",
		`UPDATE identity.memberships SET valid_until = NULL, version = version + 1, updated_at = platform.app_now()
		WHERE id = $1 AND valid_until IS NOT NULL`,
		ID(site.Client),
	)
	if err != nil {
		return err
	}

	err = livePlace(exec, site, tenant)
	if err != nil {
		return err
	}

	for _, module := range site.Modules() {
		err := liveModel(exec, module, tenant)
		if err != nil {
			return err
		}

		err = liveUnit(exec, site, module, tenant, now)
		if err != nil {
			return err
		}

		err = liveModuleOf(ctx, txn, exec, site, module, tenant, now)
		if err != nil {
			return err
		}
	}

	return nil
}

// liveModel adds a live unit's model with the module's sensors: the air conditioner's offers the remote power only, a
// sensor unit's no control; neither has a mode, fan or temperature setting (the deterministic contracts take empty
// lists and a null temperature).
func liveModel(exec liveExec, module LiveModule, tenant uuid.UUID) error {
	type capSensor struct {
		Metric            string  `json:"metric"`
		Unit              string  `json:"unit"`
		StaleAfterSeconds int     `json:"staleAfterSeconds"`
		BoundaryID        *string `json:"boundaryId"`
	}

	sensors := make([]capSensor, 0, len(module.Sensors))
	for _, sensor := range module.Sensors {
		capSens := capSensor{
			Metric:            sensor.Metric,
			Unit:              sensor.Unit,
			StaleAfterSeconds: liveStaleAfterSeconds,
			BoundaryID:        nil,
		}
		if sensor.Boundary != "" {
			capSens.BoundaryID = &sensor.Boundary
		}

		sensors = append(sensors, capSens)
	}

	capSensors, err := json.Marshal(sensors)
	if err != nil {
		return apperr.Wrap(err, "json.Marshal")
	}

	return exec(
		"model",
		`INSERT INTO devices.capabilities (id, version, tenant_id, manufacturer, model, control, mode_control, fan_control,
			temperature,
		modes, fan_levels, ventilation, ventilation_levels, sensors, firmware_candidates, is_current)
		VALUES ($1,1,$2,$3,$4,$6,false,false,NULL,'{}','{}',false,'{}',$5,'{}',true) ON CONFLICT DO NOTHING`,
		ID(module.Capability),
		tenant,
		module.Manufacturer,
		module.Model,
		capSensors,
		module.Control,
	)
}

// livePlace adds the live site's property with one floor and its room.
func livePlace(exec liveExec, site Site, tenant uuid.UUID) error {
	err := exec(
		"property",
		`INSERT INTO assets.properties (id, tenant_id, customer_org_id, kind, name, address, access_instructions, archived)
		VALUES ($1,$2,$3,'office','Live sensors',NULL,NULL,false) ON CONFLICT DO NOTHING`,
		ID(site.Property),
		tenant,
		ID(site.CustomerOrgID),
	)
	if err != nil {
		return err
	}

	err = exec(
		"floor",
		`INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name) VALUES ($1,$2,$3,NULL,'floor',
			'1F') ON CONFLICT DO NOTHING`,
		ID(site.Floor),
		tenant,
		ID(site.Property),
	)
	if err != nil {
		return err
	}

	return exec(
		"room",
		`INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name) VALUES ($1,$2,$3,$4,'room',
			'Sensor room') ON CONFLICT DO NOTHING`,
		ID(site.Room),
		tenant,
		ID(site.Property),
		ID(site.Floor),
	)
}

// liveUnit adds a live unit in the room as of now; the air conditioner's settings are unknown until a command sets
// them (the nodes report none).
func liveUnit(exec liveExec, site Site, module LiveModule, tenant uuid.UUID, now time.Time) error {
	state, marshalErr := json.Marshal(
		map[string]any{
			keyPower:      nil,
			"celsius":     nil,
			"mode":        nil,
			"fanLevel":    nil,
			keyObservedAt: now.UTC(),
		},
	)
	if marshalErr != nil {
		return apperr.Wrap(marshalErr, "json.Marshal")
	}

	return exec(
		"unit",
		`INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id,
			installed_at,
		service_scope, archived, capability_version, observed_state, observed_restriction, version, warranty_ends_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULL,'{indoor,outdoor,electrical}',false,1,$8,NULL,1,NULL) ON CONFLICT DO NOTHING`,
		ID(module.Unit),
		tenant,
		ID(site.CustomerOrgID),
		ID(site.Property),
		ID(site.Room),
		module.UnitName,
		ID(module.Capability),
		state,
	)
}

// liveModuleOf adds a live unit's module, online as of now, with its sensors and its binding to the unit; the unit
// mirrors a new module's connection and last seen (IR47), while a module from an earlier start keeps its own.
func liveModuleOf(
	ctx context.Context,
	txn pgx.Tx,
	exec liveExec,
	site Site,
	module LiveModule,
	tenant uuid.UUID,
	now time.Time,
) error {
	unit, device, binding := ID(module.Unit), ID(module.Device), ID(module.Binding)

	added, err := txn.Exec(
		ctx,
		`INSERT INTO devices.devices (id, tenant_id, serial, binding_id, unit_id, target_unit_id, created_by_membership_id,
			connection, last_seen_at,
		firmware_version, power_signal, tamper) VALUES ($1,$2,$3,$4,$5,$5,$6,'online',$7,'external','unknown','clear') ON
		CONFLICT DO NOTHING`,
		device,
		tenant,
		module.Serial,
		binding,
		unit,
		ID("system-demo"),
		now,
	)
	if err != nil {
		return fmt.Errorf("live site module: %w", err)
	}

	for _, liveSensor := range module.Sensors {
		var boundary *string
		if liveSensor.Boundary != "" {
			boundary = &liveSensor.Boundary
		}

		err := exec(
			"sensor "+liveSensor.Metric,
			`INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, boundary_id, stale_after_seconds,
				calibrated_at)
			VALUES ($1,$2,$3,$4,$5,$6,$7,NULL) ON CONFLICT DO NOTHING`,
			ID(liveSensor.Name),
			tenant,
			device,
			liveSensor.Metric,
			liveSensor.Unit,
			boundary,
			liveStaleAfterSeconds,
		)
		if err != nil {
			return err
		}
	}

	err = exec(
		"binding",
		`INSERT INTO devices.device_bindings (id, tenant_id, device_id, unit_id, customer_org_id, bound_at, reason,
			actor_membership_id)
		VALUES ($1,$2,$3,$4,$5,$6,'live sensors',$7) ON CONFLICT DO NOTHING`,
		binding,
		tenant,
		device,
		unit,
		ID(site.CustomerOrgID),
		now,
		ID("system-demo"),
	)
	if err != nil {
		return err
	}

	if added.RowsAffected() == 0 { // a module from an earlier start keeps its connection and last seen
		return nil
	}
	// the unit mirrors its new module's connection and last seen (IR47)
	return exec(
		"unit connection",
		`UPDATE assets.units SET connection = 'online', last_seen_at = $2 WHERE id = $1`,
		unit,
		now,
	)
}
