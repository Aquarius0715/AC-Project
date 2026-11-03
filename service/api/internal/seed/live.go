package seed

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// errAddressTaken stops the live seed when the owner's address is another person's.
var errAddressTaken = errors.New("belongs to another person")

// LiveSensor is one sensor of the live site's module, by its fixture-style name.
type LiveSensor struct {
	Name     string // fixture-style name; the API ID is ID(Name)
	Metric   string
	Unit     string
	Boundary string // energy boundary ("" for none)
}

// LiveOwner is the live customer's first owner by fixture-style names (the person, the client membership and the
// entry in the customer's user list), with the address the sign-in account takes as its username.
type LiveOwner struct {
	User, Membership, ClientUser string
	Email, Name                  string
}

// LiveSite is where the readings of real sensor nodes land (DEC-86, IR524): a customer of its own (DEC-87, IR525)
// with one Daikin air conditioner (DEC-89, IR528). The two nodes in front of it make one module: node_1 measures the
// room, node_2 the air conditioner's supply, and node_1's infrared LED switches it on and off. The model offers the
// power only, the one setting the nodes' firmware takes (ON, OFF and a cooling preset). The IoT bridge subscribes to
// the nodes' broker (REAL_MQTT_URL), sends their readings to these sensors and the power commands to node_1.
// Development only.
var LiveSite = struct {
	Tenant, CustomerOrgID, Customer, CustomerName     string
	Owner                                             LiveOwner
	Property, Floor, Room                             string
	Capability, Manufacturer, Model, UnitName, Serial string
	Unit, Device, Binding                             string
	Sensors                                           []LiveSensor
}{
	Tenant:        "tenant-a",
	CustomerOrgID: "org-customer-live",
	Customer:      "cust-live",
	CustomerName:  "Live Sensor Customer",
	Owner: LiveOwner{
		User:       "user-live-owner",
		Membership: "live-owner",
		ClientUser: "client-user-live-owner",
		Email:      "live-sensors@example.com",
		Name:       "Live Sensor Owner",
	},
	Property:     "property-live-sensors",
	Floor:        "space-live-1f",
	Room:         "space-live-room",
	Capability:   "cap-live-daikin",
	Manufacturer: "Daikin",
	Model:        "IR via node_1",
	UnitName:     "Live · Daikin AC",
	Serial:       "node_1+node_2",
	Unit:         "unit-live-ac",
	Device:       "device-live-ac",
	Binding:      "binding-live-ac",
	Sensors: []LiveSensor{
		{Name: "sensor-live-temperature", Metric: "temperature", Unit: "°C", Boundary: ""},
		{Name: "sensor-live-humidity", Metric: "humidity", Unit: "%", Boundary: ""},
		{Name: "sensor-live-power", Metric: "power", Unit: "kW", Boundary: "ac_input_electricity"},
	},
}

// ApplyLive adds the live site to a seeded database: the live customer and its owner (liveOwner), the model (remote
// power only), a property with one room, the air conditioner and its module, online since at. Records that exist are
// kept (ON CONFLICT DO NOTHING), so it can run on every start. Development only: tests and production never load it.
func ApplyLive(ctx context.Context, txn pgx.Tx, at time.Time) error {
	s := LiveSite
	tenant := ID(s.Tenant)
	ex := func(what, sql string, args ...any) error {
		_, err := txn.Exec(ctx, sql, args...)
		if err != nil {
			return fmt.Errorf("live site %s: %w", what, err)
		}

		return nil
	}
	// the customer as HQ adds one: its organization, then the customer (organizations.save, customers.save)
	err := ex(
		"customer organization",
		`INSERT INTO identity.organizations (id, tenant_id, name, kind, status, created_at, updated_at)
		VALUES ($1,$2,$3,'customer','active',$4,$4) ON CONFLICT DO NOTHING`,
		ID(s.CustomerOrgID),
		tenant,
		s.CustomerName,
		at,
	)
	if err != nil {
		return err
	}

	err = ex(
		"customer",
		`INSERT INTO assets.customers (id, tenant_id, organization_id, name, status, service_profile, created_at, updated_at)
		VALUES ($1,$2,$3,$4,'active','environment',$5,$5) ON CONFLICT DO NOTHING`,
		ID(s.Customer),
		tenant,
		ID(s.CustomerOrgID),
		s.CustomerName,
		at,
	)
	if err != nil {
		return err
	}

	err = liveOwner(ctx, txn, tenant, at)
	if err != nil {
		return err
	}

	type capSensor struct {
		Metric            string  `json:"metric"`
		Unit              string  `json:"unit"`
		StaleAfterSeconds int     `json:"staleAfterSeconds"`
		BoundaryID        *string `json:"boundaryId"`
	}

	var sensors []capSensor

	for _, x := range s.Sensors {
		cs := capSensor{Metric: x.Metric, Unit: x.Unit, StaleAfterSeconds: 120, BoundaryID: nil}
		if x.Boundary != "" {
			cs.BoundaryID = &x.Boundary
		}

		sensors = append(sensors, cs)
	}

	capSensors, err := json.Marshal(sensors)
	if err != nil {
		return apperr.Wrap(err, "json.Marshal")
	}
	// remote power only: no mode, fan or temperature setting (deterministic contracts: empty lists, null temperature)
	exErr := ex(
		"model",
		`INSERT INTO devices.capabilities (id, version, tenant_id, manufacturer, model, control, mode_control, fan_control,
			temperature,
		modes, fan_levels, ventilation, ventilation_levels, sensors, firmware_candidates, is_current)
		VALUES ($1,1,$2,$3,$4,true,false,false,NULL,'{}','{}',false,'{}',$5,'{}',true) ON CONFLICT DO NOTHING`,
		ID(s.Capability),
		tenant,
		s.Manufacturer,
		s.Model,
		capSensors,
	)
	if exErr != nil {
		return exErr
	}

	exErr = ex(
		"property",
		`INSERT INTO assets.properties (id, tenant_id, customer_org_id, kind, name, address, access_instructions, archived)
		VALUES ($1,$2,$3,'office','Live sensors',NULL,NULL,false) ON CONFLICT DO NOTHING`,
		ID(s.Property),
		tenant,
		ID(s.CustomerOrgID),
	)
	if exErr != nil {
		return exErr
	}

	exErr = ex(
		"floor",
		`INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name) VALUES ($1,$2,$3,NULL,'floor',
			'1F') ON CONFLICT DO NOTHING`,
		ID(s.Floor),
		tenant,
		ID(s.Property),
	)
	if exErr != nil {
		return exErr
	}

	exErr = ex(
		"room",
		`INSERT INTO assets.spaces (id, tenant_id, property_id, parent_space_id, kind, name) VALUES ($1,$2,$3,$4,'room',
			'Sensor room') ON CONFLICT DO NOTHING`,
		ID(s.Room),
		tenant,
		ID(s.Property),
		ID(s.Floor),
	)
	if exErr != nil {
		return exErr
	}
	// the air conditioner's settings are unknown until a command sets them (the nodes report none)
	state, _ := json.Marshal(
		map[string]any{
			"power":      nil,
			"celsius":    nil,
			"mode":       nil,
			"fanLevel":   nil,
			"observedAt": at.UTC(),
		},
	)

	unit := ID(s.Unit)

	exErr = ex(
		"unit",
		`INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id,
			installed_at,
		service_scope, archived, capability_version, observed_state, observed_restriction, version, warranty_ends_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULL,'{indoor,outdoor,electrical}',false,1,$8,NULL,1,NULL) ON CONFLICT DO NOTHING`,
		unit,
		tenant,
		ID(s.CustomerOrgID),
		ID(s.Property),
		ID(s.Room),
		s.UnitName,
		ID(s.Capability),
		state,
	)
	if exErr != nil {
		return exErr
	}

	device, binding := ID(s.Device), ID(s.Binding)

	added, err := txn.Exec(
		ctx,
		`INSERT INTO devices.devices (id, tenant_id, serial, binding_id, unit_id, target_unit_id, created_by_membership_id,
			connection, last_seen_at,
		firmware_version, power_signal, tamper) VALUES ($1,$2,$3,$4,$5,$5,$6,'online',$7,'external','unknown','clear') ON
		CONFLICT DO NOTHING`,
		device,
		tenant,
		s.Serial,
		binding,
		unit,
		ID("system-demo"),
		at,
	)
	if err != nil {
		return fmt.Errorf("live site module: %w", err)
	}

	for _, liveSensor := range s.Sensors {
		var boundary *string
		if liveSensor.Boundary != "" {
			boundary = &liveSensor.Boundary
		}

		err := ex(
			"sensor "+liveSensor.Metric,
			`INSERT INTO devices.sensors (id, tenant_id, device_id, metric, unit, boundary_id, stale_after_seconds,
				calibrated_at)
			VALUES ($1,$2,$3,$4,$5,$6,120,NULL) ON CONFLICT DO NOTHING`,
			ID(liveSensor.Name),
			tenant,
			device,
			liveSensor.Metric,
			liveSensor.Unit,
			boundary,
		)
		if err != nil {
			return err
		}
	}

	exErr = ex(
		"binding",
		`INSERT INTO devices.device_bindings (id, tenant_id, device_id, unit_id, customer_org_id, bound_at, reason,
			actor_membership_id)
		VALUES ($1,$2,$3,$4,$5,$6,'live sensors',$7) ON CONFLICT DO NOTHING`,
		binding,
		tenant,
		device,
		unit,
		ID(s.CustomerOrgID),
		at,
		ID("system-demo"),
	)
	if exErr != nil {
		return exErr
	}

	if added.RowsAffected() == 0 { // a module from an earlier start keeps its connection and last seen
		return nil
	}
	// the unit mirrors its new module's connection and last seen (IR47)
	return ex(
		"unit connection",
		`UPDATE assets.units SET connection = 'online', last_seen_at = $2 WHERE id = $1`,
		unit,
		at,
	)
}

// liveOwner adds the live customer's owner as clientUsers.save adds one for HQ (IR461): the person, a client owner
// membership with a client's permissions over the organization, the initial location consent (IR84) and the invited
// entry in the customer's user list. A new membership asks for the sign-in account as bootstrap-admin does (IR509): on
// AccountRequested the identity-accounts consumer opens it, with the address as its username, and e-mails the set-up
// link (in development to Mailpit), where the owner chooses a password. A membership that exists asks for nothing.
func liveOwner(ctx context.Context, txn pgx.Tx, tenant uuid.UUID, at time.Time) error {
	o, org, customer := LiveSite.Owner, ID(LiveSite.CustomerOrgID), ID(LiveSite.Customer)

	user, membership := ID(o.User), ID(o.Membership)

	_, err := txn.Exec(
		ctx,
		`INSERT INTO identity.users (id, email, display_name, status, created_at, updated_at) VALUES ($1,$2,$3,'active',$4,$4)
		ON CONFLICT DO NOTHING`,
		user,
		o.Email,
		o.Name,
		at,
	)
	if err != nil {
		return fmt.Errorf("live owner: %w", err)
	}

	var holder uuid.UUID

	err = txn.QueryRow(ctx, `SELECT id FROM identity.users WHERE email = $1`, o.Email).
		Scan(&holder)
	if err != nil {
		return fmt.Errorf("live owner: %w", err)
	}

	if holder != user {
		return fmt.Errorf("live owner: %s %w", o.Email, errAddressTaken)
	}

	added, err := txn.Exec(
		ctx,
		`INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from, created_at,
			updated_at)
		VALUES ($1,$2,$3,$4,'client','owner',$5,$5,$5) ON CONFLICT DO NOTHING`,
		membership,
		tenant,
		user,
		org,
		at,
	)
	if err != nil {
		return fmt.Errorf("live owner membership: %w", err)
	}

	for _, p := range identity.DefaultPermissions("client") {
		_, err := txn.Exec(
			ctx,
			`INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES ($1,$2,$3) ON CONFLICT DO
				NOTHING`,
			tenant,
			membership,
			p,
		)
		if err != nil {
			return fmt.Errorf("live owner permission %s: %w", p, err)
		}
	}

	_, execErr := txn.Exec(
		ctx,
		`INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES ($1,$2,'organization',$3) ON
			CONFLICT DO NOTHING`,
		tenant,
		membership,
		org,
	)
	if execErr != nil {
		return fmt.Errorf("live owner scope: %w", execErr)
	}

	_, execErr = txn.Exec(
		ctx,
		`INSERT INTO identity.consents (tenant_id, membership_id, purpose, granted, created_at, updated_at) VALUES ($1,$2,
			'location_automation',false,$3,$3)
		ON CONFLICT DO NOTHING`,
		tenant,
		membership,
		at,
	)
	if execErr != nil {
		return fmt.Errorf("live owner consent: %w", execErr)
	}

	_, execErr = txn.Exec(
		ctx,
		`INSERT INTO identity.client_users (id, tenant_id, customer_id, membership_id, email, display_name, client_role,
			status, invited_at,
		invited_by_membership_id, last_invite_sent_at, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,'owner','invited',$7,
		$4,$7,$7,$7) ON CONFLICT DO NOTHING`,
		ID(o.ClientUser),
		tenant,
		customer,
		membership,
		o.Email,
		o.Name,
		at,
	)
	if execErr != nil {
		return fmt.Errorf("live owner user list: %w", execErr)
	}

	if added.RowsAffected() == 0 {
		return nil
	}

	_, err = txn.Exec(
		ctx,
		`INSERT INTO platform.outbox (tenant_id, aggregate_type, aggregate_id, event_type, payload, correlation_id,
			occurred_at)
		VALUES ($1, 'membership', $2, $3, jsonb_build_object('userId', $4::uuid::text, 'membershipId', $2::uuid::text, 'role',
		'client'), 'seed-live-site', $5)`,
		tenant,
		membership,
		identity.EventAccountRequested,
		user,
		at,
	)
	if err != nil {
		return fmt.Errorf("live owner account: %w", err)
	}

	return nil
}
