package iotbridge

import (
	"slices"
	"sync"

	"github.com/google/uuid"
)

// NodeSensor is a sensor of a node's device, as devices.mqttNodes answers it.
type NodeSensor struct {
	ID     uuid.UUID `json:"id"`
	Metric string    `json:"metric"`
	Unit   string    `json:"unit"`
}

// NodeDevice is a bound device that has an MQTT node_id (IR635): the <node> of smarthome/ac/<node>/…, the device and
// unit its readings belong to, whether its model takes commands, and the sensors the readings are recorded on.
type NodeDevice struct {
	Node     string       `json:"node"`
	DeviceID uuid.UUID    `json:"deviceId"`
	UnitID   uuid.UUID    `json:"unitId"`
	Control  bool         `json:"control"`
	Sensors  []NodeSensor `json:"sensors"`
}

// NodeField is how one metric is read from a node's JSON message (device-firmware.md §5.2): all nodes run the same
// firmware, so the table is the same for every node.
type NodeField struct {
	Field string  // the field of the node's message
	Scale float64 // to the sensor's unit (W → kW: 0.001)
	// Gate is the field that must be non-zero for the value to be a reading: a node sends 0 for what it does not
	// measure, so a zero temperature is no reading, while a zero power is one when the meter sees a voltage.
	Gate string
}

// kWPerWatt scales the meter's power in W to the sensor's kW.
const kWPerWatt = 0.001

// NodeFields are the metrics the bridge records from a node's message, by the sensor's metric. Pressure, gas
// resistance and altitude have no metric in the platform; voltage, current and cumulative energy are derived from
// the power instead.
func NodeFields() map[string]NodeField {
	return map[string]NodeField{
		sensorTemperature: {Field: fieldTemp, Scale: 1, Gate: fieldTemp},
		sensorHumidity:    {Field: sensorHumidity, Scale: 1, Gate: sensorHumidity},
		sensorPower:       {Field: sensorPower, Scale: kWPerWatt, Gate: "voltage"},
	}
}

// NodeRegistry is the bridge's view of the registered nodes: the devices of the tenant that have a node_id and are
// bound. It is replaced as a whole every time the API is read; between reads, and while the API is out of reach, the
// last reading stands.
type NodeRegistry struct {
	mu       sync.RWMutex
	loaded   bool
	byNode   map[string]NodeDevice
	byDevice map[uuid.UUID]NodeDevice
}

// NewNodeRegistry is a registry that has not been loaded yet.
func NewNodeRegistry() *NodeRegistry {
	return &NodeRegistry{
		mu:       sync.RWMutex{},
		loaded:   false,
		byNode:   map[string]NodeDevice{},
		byDevice: map[uuid.UUID]NodeDevice{},
	}
}

// Set replaces the registered nodes.
func (r *NodeRegistry) Set(devices []NodeDevice) {
	byNode := make(map[string]NodeDevice, len(devices))
	byDevice := make(map[uuid.UUID]NodeDevice, len(devices))

	for _, dev := range devices {
		byNode[dev.Node] = dev
		byDevice[dev.DeviceID] = dev
	}

	r.mu.Lock()
	defer r.mu.Unlock()

	r.loaded, r.byNode, r.byDevice = true, byNode, byDevice
}

// Loaded reports whether the API has been read at least once.
func (r *NodeRegistry) Loaded() bool {
	r.mu.RLock()
	defer r.mu.RUnlock()

	return r.loaded
}

// Node is the device registered for a node_id (exact match).
func (r *NodeRegistry) Node(node string) (NodeDevice, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dev, ok := r.byNode[node]

	return dev, ok
}

// Device is the node of a device.
func (r *NodeRegistry) Device(id uuid.UUID) (NodeDevice, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()

	dev, ok := r.byDevice[id]

	return dev, ok
}

// Devices are the registered nodes in the order of their node_id.
func (r *NodeRegistry) Devices() []NodeDevice {
	r.mu.RLock()
	defer r.mu.RUnlock()

	out := make([]NodeDevice, 0, len(r.byNode))
	for _, dev := range r.byNode {
		out = append(out, dev)
	}

	slices.SortFunc(out, func(a, b NodeDevice) int {
		switch {
		case a.Node < b.Node:
			return -1
		case a.Node > b.Node:
			return 1
		default:
			return 0
		}
	})

	return out
}
