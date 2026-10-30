package seed

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
)

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

// LiveNode is one real sensor node of the live site: its own monitoring-only model, air conditioner (unit) and
// module, by fixture-style names, with the sensors it reports. A unit has at most one active module, so each node is
// a unit of its own (DEC-88).
type LiveNode struct {
	Node                    string // the node's name on its broker (the topic level, its device_id)
	UnitName, Serial, Model string // the unit's display name, the module's serial and the model's name
	Capability              string
	Unit, Device, Binding   string
	Sensors                 []LiveSensor
}

// LiveSite is where the readings of real sensor nodes land (DEC-86, IR524): a customer of its own (DEC-87, IR525)
// with one monitoring-only air conditioner per node (DEC-88, IR526), whose module's sensors take that node's
// readings. The IoT bridge subscribes to the nodes' broker (REAL_MQTT_URL) and sends their readings to these sensors.
// Development only.
var LiveSite = struct {
	Tenant, CustomerOrgID, Customer, CustomerName string
	Owner                                         LiveOwner
	Property, Floor, Room                         string
	Nodes                                         []LiveNode
}{
	Tenant:        "tenant-a",
	CustomerOrgID: "org-customer-live",
	Customer:      "cust-live",
	CustomerName:  "Live Sensor Customer",
	Owner:         LiveOwner{User: "user-live-owner", Membership: "live-owner", ClientUser: "client-user-live-owner", Email: "live-sensors@example.com", Name: "Live Sensor Owner"},
	Property:      "property-live-sensors",
	Floor:         "space-live-1f",
	Room:          "space-live-room",
	Nodes: []LiveNode{
		{Node: "node_1", UnitName: "Live · node_1 room sensor", Serial: "node_1", Model: "Room sensor", Capability: "cap-live-room-sensor",
			Unit: "unit-live-ac", Device: "device-live-ac", Binding: "binding-live-ac",
			Sensors: []LiveSensor{{Name: "sensor-live-temperature", Metric: "temperature", Unit: "°C"}, {Name: "sensor-live-humidity", Metric: "humidity", Unit: "%"}}},
		{Node: "node_2", UnitName: "Live · node_2 power meter", Serial: "node_2", Model: "Power meter", Capability: "cap-live-power-meter",
			Unit: "unit-live-node-2", Device: "device-live-node-2", Binding: "binding-live-node-2",
			Sensors: []LiveSensor{{Name: "sensor-live-power", Metric: "power", Unit: "kW", Boundary: "ac_input_electricity"}}},
	},
}

// LiveNodeNamed is the live site's node of that broker name.
func LiveNodeNamed(node string) (LiveNode, bool) {
	for _, n := range LiveSite.Nodes {
		if n.Node == node {
			return n, true
		}
	}
	return LiveNode{}, false
}

// ApplyLive adds the live site to a seeded database: the live customer and its owner (liveOwner), a property with one
// room and, per node, a monitoring-only model (no remote control: the nodes only measure), the air conditioner and its
// module, online since at. Records that exist are kept (ON CONFLICT DO NOTHING), so it can run on every start.
// Development only: tests and production never load it.
func ApplyLive(ctx context.Context, tx pgx.Tx, at time.Time) error {
	s := LiveSite
	tenant := ID(s.Tenant)
	ex := func(what, sql string, args ...any) error {
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			return fmt.Errorf("live site %s: %w", what, err)
		}
		return nil
	}
	// the customer as HQ adds one: its organization, then the customer (organizations.save, customers.save)
	if err := ex("customer organization", `INSERT INTO identity.organizations (id, tenant_id, name, kind, status, created_at, updated_at)
		VALUES ($1,$2,$3,'customer','active',$4,$4) ON CONFLICT DO NOTHING`, ID(s.CustomerOrgID), tenant, s.CustomerName, at); err != nil {
		return err
	}
	if err := ex("customer", `INSERT INTO assets.customers (id, tenant_id, organization_id, name, status, service_profile, created_at, updated_at)
		VALUES ($1,$2,$3,$4,'active','environment',$5,$5) ON CONFLICT DO NOTHING`, ID(s.Customer), tenant, ID(s.CustomerOrgID), s.CustomerName, at); err != nil {
		return err
	}
	if err := liveOwner(ctx, tx, tenant, at); err != nil {
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
	for _, n := range s.Nodes {
		if err := liveNode(ctx, tx, tenant, n, at); err != nil {
			return fmt.Errorf("live site %s: %w", n.Node, err)
		}
	}
	return nil
}

// liveNode adds one node: its model with the node's sensors, the unit in the site's room and its online module, bound.
func liveNode(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, n LiveNode, at time.Time) error {
	s := LiveSite
	ex := func(what, sql string, args ...any) error {
		if _, err := tx.Exec(ctx, sql, args...); err != nil {
			return fmt.Errorf("%s: %w", what, err)
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
	for _, x := range n.Sensors {
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
		VALUES ($1,1,$2,'Live',$3,false,false,false,NULL,'{}','{}',false,'{}',$4,'{}',true) ON CONFLICT DO NOTHING`,
		ID(n.Capability), tenant, n.Model, capSensors); err != nil {
		return err
	}
	// the nodes report no settings of the air conditioner: its observed state stays unknown
	state, _ := json.Marshal(map[string]any{"power": nil, "celsius": nil, "mode": nil, "fanLevel": nil, "observedAt": at.UTC()})
	unit := ID(n.Unit)
	if err := ex("unit", `INSERT INTO assets.units (id, tenant_id, customer_org_id, property_id, space_id, display_name, model_id, installed_at,
		service_scope, archived, capability_version, observed_state, observed_restriction, version, warranty_ends_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,NULL,'{indoor,outdoor,electrical}',false,1,$8,NULL,1,NULL) ON CONFLICT DO NOTHING`,
		unit, tenant, ID(s.CustomerOrgID), ID(s.Property), ID(s.Room), n.UnitName, ID(n.Capability), state); err != nil {
		return err
	}
	device, binding := ID(n.Device), ID(n.Binding)
	added, err := tx.Exec(ctx, `INSERT INTO devices.devices (id, tenant_id, serial, binding_id, unit_id, target_unit_id, created_by_membership_id, connection, last_seen_at,
		firmware_version, power_signal, tamper) VALUES ($1,$2,$3,$4,$5,$5,$6,'online',$7,'external','unknown','clear') ON CONFLICT DO NOTHING`,
		device, tenant, n.Serial, binding, unit, ID("system-demo"), at)
	if err != nil {
		return fmt.Errorf("module: %w", err)
	}
	for _, x := range n.Sensors {
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

// liveOwner adds the live customer's owner as clientUsers.save adds one for HQ (IR461): the person, a client owner
// membership with a client's permissions over the organization, the initial location consent (IR84) and the invited
// entry in the customer's user list. A new membership asks for the sign-in account as bootstrap-admin does (IR509): on
// AccountRequested the identity-accounts consumer opens it, with the address as its username, and e-mails the set-up
// link (in development to Mailpit), where the owner chooses a password. A membership that exists asks for nothing.
func liveOwner(ctx context.Context, tx pgx.Tx, tenant uuid.UUID, at time.Time) error {
	o, org, customer := LiveSite.Owner, ID(LiveSite.CustomerOrgID), ID(LiveSite.Customer)
	user, membership := ID(o.User), ID(o.Membership)
	if _, err := tx.Exec(ctx, `INSERT INTO identity.users (id, email, display_name, status, created_at, updated_at) VALUES ($1,$2,$3,'active',$4,$4)
		ON CONFLICT DO NOTHING`, user, o.Email, o.Name, at); err != nil {
		return fmt.Errorf("live owner: %w", err)
	}
	var holder uuid.UUID
	if err := tx.QueryRow(ctx, `SELECT id FROM identity.users WHERE email = $1`, o.Email).Scan(&holder); err != nil {
		return fmt.Errorf("live owner: %w", err)
	}
	if holder != user {
		return fmt.Errorf("live owner: %s belongs to another person", o.Email)
	}
	added, err := tx.Exec(ctx, `INSERT INTO identity.memberships (id, tenant_id, user_id, organization_id, role, client_role, valid_from, created_at, updated_at)
		VALUES ($1,$2,$3,$4,'client','owner',$5,$5,$5) ON CONFLICT DO NOTHING`, membership, tenant, user, org, at)
	if err != nil {
		return fmt.Errorf("live owner membership: %w", err)
	}
	for _, p := range identity.DefaultPermissions("client") {
		if _, err := tx.Exec(ctx, `INSERT INTO identity.membership_permissions (tenant_id, membership_id, permission) VALUES ($1,$2,$3) ON CONFLICT DO NOTHING`, tenant, membership, p); err != nil {
			return fmt.Errorf("live owner permission %s: %w", p, err)
		}
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity.membership_scopes (tenant_id, membership_id, kind, ref_id) VALUES ($1,$2,'organization',$3) ON CONFLICT DO NOTHING`,
		tenant, membership, org); err != nil {
		return fmt.Errorf("live owner scope: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity.consents (tenant_id, membership_id, purpose, granted, created_at, updated_at) VALUES ($1,$2,'location_automation',false,$3,$3)
		ON CONFLICT DO NOTHING`, tenant, membership, at); err != nil {
		return fmt.Errorf("live owner consent: %w", err)
	}
	if _, err := tx.Exec(ctx, `INSERT INTO identity.client_users (id, tenant_id, customer_id, membership_id, email, display_name, client_role, status, invited_at,
		invited_by_membership_id, last_invite_sent_at, created_at, updated_at) VALUES ($1,$2,$3,$4,$5,$6,'owner','invited',$7,$4,$7,$7,$7) ON CONFLICT DO NOTHING`,
		ID(o.ClientUser), tenant, customer, membership, o.Email, o.Name, at); err != nil {
		return fmt.Errorf("live owner user list: %w", err)
	}
	if added.RowsAffected() == 0 {
		return nil
	}
	_, err = tx.Exec(ctx, `INSERT INTO platform.outbox (tenant_id, aggregate_type, aggregate_id, event_type, payload, correlation_id, occurred_at)
		VALUES ($1, 'membership', $2, $3, jsonb_build_object('userId', $4::uuid::text, 'membershipId', $2::uuid::text, 'role', 'client'), 'seed-live-site', $5)`,
		tenant, membership, identity.EventAccountRequested, user, at)
	if err != nil {
		return fmt.Errorf("live owner account: %w", err)
	}
	return nil
}
