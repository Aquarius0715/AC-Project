package devices

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// QueryConnectionFault reads a device's connection axis on its current binding: the open fault, if any, and the
// newest sequence. A demo.trigger restored must name the newest fault by its DeviceEvent ID (SR20), which a gateway
// cannot know from what it sent, so the IoT gateway (cmd/iotbridge) reads it here with the internal service token to
// take a live device offline and back online, also after its own restart (IR532).
const QueryConnectionFault = "devices.connectionFault"

// ConnectionFaultInput is QueryConnectionFault input.
type ConnectionFaultInput struct {
	DeviceID uuid.UUID `json:"deviceId"`
}

// Validate implements ops.Validator.
func (in *ConnectionFaultInput) Validate() map[string]string {
	if in.DeviceID == uuid.Nil {
		return map[string]string{"deviceId": apperr.KeyRequired}
	}

	return nil
}

// ConnectionFault is the connection axis of the device's current binding.
type ConnectionFault struct {
	BindingID *uuid.UUID `json:"bindingId"` // null while the device is unbound
	// FaultID is the DeviceEvent ID of the newest fault on the axis while it is not restored (a restored names it as
	// recovery.sourceEventId); null when the connection has no open fault.
	FaultID  *uuid.UUID `json:"faultId"`
	Sequence int64      `json:"sequence"` // the newest sequence on the axis (0 when none): a new event needs a greater one
}

// QueryMqttNodes lists the bound devices that have an MQTT node_id, with their unit, whether their model takes
// commands, and their sensors. The IoT gateway (cmd/iotbridge) reads it with the internal service token every 30
// seconds to know which node's readings go to which sensors and which node a unit's command goes to (IR635).
const QueryMqttNodes = "devices.mqttNodes"

// MqttNodesInput is QueryMqttNodes input (the tenant comes from the request header).
type MqttNodesInput struct{}

// Validate implements ops.Validator.
func (*MqttNodesInput) Validate() map[string]string { return nil }

// MqttNodeSensor is a sensor of a node's device.
type MqttNodeSensor struct {
	ID     uuid.UUID `json:"id"`
	Metric string    `json:"metric"`
	Unit   string    `json:"unit"`
}

// MqttNode is one node_id with the device and unit it feeds.
type MqttNode struct {
	Node     string           `json:"node"`
	DeviceID uuid.UUID        `json:"deviceId"`
	UnitID   uuid.UUID        `json:"unitId"`
	Control  bool             `json:"control"` // the unit's model takes power commands
	Sensors  []MqttNodeSensor `json:"sensors"`
}

// MqttNodes is QueryMqttNodes answer.
type MqttNodes struct {
	Nodes []MqttNode `json:"nodes"`
}

// RegisterQueries binds the Devices internal queries.
func RegisterQueries(r *ops.Registry) {
	ops.RegisterQuery(r, ops.DomainEquipment, QueryConnectionFault, connectionFault)
	ops.RegisterQuery(r, ops.DomainEquipment, QueryMqttNodes, mqttNodes)
}

func mqttNodes(ctx context.Context, call *ops.Call, _ *MqttNodesInput) (MqttNodes, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT d.mqtt_node, d.id, d.unit_id, COALESCE(c.control, false)
		FROM devices.devices d
		JOIN assets.units u ON u.id = d.unit_id
		LEFT JOIN devices.capabilities c ON c.id = u.model_id AND c.version = u.capability_version
		WHERE d.mqtt_node IS NOT NULL AND d.binding_id IS NOT NULL AND d.unit_id IS NOT NULL
		ORDER BY d.mqtt_node`,
	)
	if err != nil {
		return MqttNodes{}, apperr.Wrap(err, "select devices.devices")
	}

	out := MqttNodes{Nodes: []MqttNode{}}
	byDevice := map[uuid.UUID]int{}

	for rows.Next() {
		var node MqttNode

		node.Sensors = []MqttNodeSensor{}

		scanErr := rows.Scan(&node.Node, &node.DeviceID, &node.UnitID, &node.Control)
		if scanErr != nil {
			rows.Close()

			return MqttNodes{}, apperr.Wrap(scanErr, "select devices.devices")
		}

		byDevice[node.DeviceID] = len(out.Nodes)
		out.Nodes = append(out.Nodes, node)
	}

	rows.Close()

	if rows.Err() != nil {
		return MqttNodes{}, apperr.Wrap(rows.Err(), "select devices.devices")
	}

	if len(out.Nodes) == 0 {
		return out, nil
	}

	sensors, err := call.Tx.Query(
		ctx,
		`SELECT device_id, id, metric, unit FROM devices.sensors ORDER BY metric`,
	)
	if err != nil {
		return MqttNodes{}, apperr.Wrap(err, "select devices.sensors")
	}

	for sensors.Next() {
		var (
			device uuid.UUID
			sensor MqttNodeSensor
		)

		scanErr := sensors.Scan(&device, &sensor.ID, &sensor.Metric, &sensor.Unit)
		if scanErr != nil {
			sensors.Close()

			return MqttNodes{}, apperr.Wrap(scanErr, "select devices.sensors")
		}

		if i, ok := byDevice[device]; ok {
			out.Nodes[i].Sensors = append(out.Nodes[i].Sensors, sensor)
		}
	}

	sensors.Close()

	return out, apperr.Wrap(sensors.Err(), "select devices.sensors")
}

func connectionFault(
	ctx context.Context,
	call *ops.Call,
	input *ConnectionFaultInput,
) (ConnectionFault, error) {
	var out ConnectionFault

	err := call.Tx.QueryRow(ctx, `SELECT binding_id FROM devices.devices WHERE id = $1`, input.DeviceID).
		Scan(&out.BindingID)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return out, apperr.Wrap(err, "select devices.devices")
	}

	scanErr := call.Tx.QueryRow(ctx, `SELECT COALESCE(max(sequence), 0) FROM devices.device_events WHERE device_id = $1 AND
		binding_id IS NOT DISTINCT FROM $2
		AND axis = 'connection'`, input.DeviceID, out.BindingID).
		Scan(&out.Sequence)
	if scanErr != nil {
		return out, apperr.Wrap(scanErr, "select devices.device_events")
	}
	// the newest fault of the axis, as RecordSignal's restored looks it up
	var (
		faultID  uuid.UUID
		restored *time.Time
	)

	err = call.Tx.QueryRow(ctx, `SELECT id, restored_at FROM devices.device_events WHERE device_id = $1 AND binding_id IS
		NOT
		DISTINCT FROM $2
		AND axis = 'connection' AND event_type <> 'restored' ORDER BY sequence DESC LIMIT 1`, input.DeviceID, out.BindingID).
		Scan(&faultID, &restored)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return out, nil
	case err != nil:
		return out, apperr.Wrap(err, "select devices.device_events")
	case restored == nil:
		out.FaultID = &faultID
	}

	return out, nil
}
