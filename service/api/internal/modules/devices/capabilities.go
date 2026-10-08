package devices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// TempRange is Capability.temperature.
type TempRange struct {
	Min  float64 `json:"min"`
	Max  float64 `json:"max"`
	Step float64 `json:"step"`
}

// SensorSpec is a Capability sensor (Sensor without id/deviceId/calibratedAt).
type SensorSpec struct {
	BoundaryID        *string `json:"boundaryId"`
	Metric            string  `json:"metric"`
	Unit              string  `json:"unit"`
	StaleAfterSeconds int     `json:"staleAfterSeconds"`
}

// Capability is Capability of service-contracts.ts (one row per version; the current row is listed).
type Capability struct {
	ID                 uuid.UUID    `json:"id"`
	TenantID           uuid.UUID    `json:"tenantId"`
	Version            int          `json:"version"`
	CreatedAt          time.Time    `json:"createdAt"`
	UpdatedAt          time.Time    `json:"updatedAt"`
	Manufacturer       string       `json:"manufacturer"`
	Model              string       `json:"model"`
	Control            bool         `json:"control"`
	ModeControl        bool         `json:"modeControl"`
	FanControl         bool         `json:"fanControl"`
	Temperature        *TempRange   `json:"temperature"`
	Modes              []string     `json:"modes"`
	FanLevels          []string     `json:"fanLevels"`
	Ventilation        bool         `json:"ventilation"`
	VentilationLevels  []string     `json:"ventilationLevels"`
	Sensors            []SensorSpec `json:"sensors"`
	FirmwareCandidates []string     `json:"firmwareCandidates"`
}

const capCols = `id, tenant_id, version, created_at, created_at, manufacturer, model, control, mode_control, fan_control, temperature, modes, fan_levels,
	ventilation, ventilation_levels, sensors, firmware_candidates`

func scanCap(r pgx.Row) (Capability, error) {
	var c Capability
	var temp, sensors []byte
	if err := r.Scan(&c.ID, &c.TenantID, &c.Version, &c.CreatedAt, &c.UpdatedAt, &c.Manufacturer, &c.Model, &c.Control, &c.ModeControl, &c.FanControl,
		&temp, &c.Modes, &c.FanLevels, &c.Ventilation, &c.VentilationLevels, &sensors, &c.FirmwareCandidates); err != nil {
		return c, err
	}
	if len(temp) > 0 {
		c.Temperature = &TempRange{}
		if err := json.Unmarshal(temp, c.Temperature); err != nil {
			return c, err
		}
	}
	c.Sensors = []SensorSpec{}
	if len(sensors) > 0 {
		if err := json.Unmarshal(sensors, &c.Sensors); err != nil {
			return c, err
		}
	}
	return c, nil
}

func capabilitiesList(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Capability], error) {
	if len(in.Filters) > 0 && string(in.Filters) != "{}" && string(in.Filters) != "null" {
		return paging.Page[Capability]{}, apperr.Fields(map[string]string{"filters": "error.invalid"}) // query catalog: no filters
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "id", "createdAt": "created_at", "updatedAt": "created_at"}, "id ASC")
	if err != nil {
		return paging.Page[Capability]{}, err
	}
	w, err := paging.Resolve(*in, nil, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Capability]{}, err
	}
	var total int
	if err := c.Tx.QueryRow(ctx, `SELECT count(*) FROM devices.capabilities WHERE is_current`).Scan(&total); err != nil {
		return paging.Page[Capability]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf(`SELECT %s FROM devices.capabilities WHERE is_current ORDER BY %s LIMIT %d OFFSET %d`, capCols, order, w.Limit, w.Offset))
	if err != nil {
		return paging.Page[Capability]{}, err
	}
	defer rows.Close()
	items := []Capability{}
	for rows.Next() {
		x, err := scanCap(rows)
		if err != nil {
			return paging.Page[Capability]{}, err
		}
		items = append(items, x)
	}
	return paging.Page[Capability]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, rows.Err()
}

// CapabilitySave is capabilities.save input.
type CapabilitySave struct {
	ID                 *uuid.UUID   `json:"id,omitempty"`
	Manufacturer       string       `json:"manufacturer"`
	Model              string       `json:"model"`
	Control            bool         `json:"control"`
	ModeControl        bool         `json:"modeControl"`
	FanControl         bool         `json:"fanControl"`
	Temperature        *TempRange   `json:"temperature"`
	Modes              []string     `json:"modes"`
	FanLevels          []string     `json:"fanLevels"`
	Ventilation        bool         `json:"ventilation"`
	VentilationLevels  []string     `json:"ventilationLevels"`
	Sensors            []SensorSpec `json:"sensors"`
	FirmwareCandidates []string     `json:"firmwareCandidates"`
	ChangeReason       *string      `json:"changeReason,omitempty"`
}

var (
	modes      = map[string]bool{"cool": true, "dry": true, "fan": true}
	fans       = map[string]bool{"low": true, "mid": true, "high": true}
	metricUnit = map[string]string{"temperature": "°C", "humidity": "%", "co2": "ppm", "pm25": "µg/m³", "power": "kW", "vibration": "mm/s",
		"refrigerant_pressure": "kPa", "compressor_cycles": "cycles/h", "airflow_drop": "%", "heartbeat_gap": "min"}
	boundaries = map[string]bool{"ac_input_electricity": true, "whole_building_electricity": true}
)

func uniqueIn(vals []string, allowed map[string]bool) bool {
	seen := map[string]bool{}
	for _, v := range vals {
		if (allowed != nil && !allowed[v]) || seen[v] || v == "" {
			return false
		}
		seen[v] = true
	}
	return true
}

// Validate implements ops.Validator (DD-A04).
func (in *CapabilitySave) Validate() map[string]string {
	fe := map[string]string{}
	if n := utf8.RuneCountInString(in.Manufacturer); n < 1 || n > 120 {
		fe["manufacturer"] = "error.length"
	}
	if n := utf8.RuneCountInString(in.Model); n < 1 || n > 120 {
		fe["model"] = "error.length"
	}
	if t := in.Temperature; t != nil && (t.Min >= t.Max || t.Step <= 0 || t.Step > t.Max-t.Min) {
		fe["temperature"] = "error.range"
	}
	if !in.Control && (in.ModeControl || in.FanControl || in.Temperature != nil) {
		fe["control"] = "error.controlRequired" // mode/fan/temperature control needs control=true
	}
	if !uniqueIn(in.Modes, modes) || (in.ModeControl && len(in.Modes) == 0) {
		fe["modes"] = "error.invalid"
	}
	if !uniqueIn(in.FanLevels, fans) || (in.FanControl && len(in.FanLevels) == 0) {
		fe["fanLevels"] = "error.invalid"
	}
	if !uniqueIn(in.VentilationLevels, fans) || (!in.Ventilation && len(in.VentilationLevels) > 0) {
		fe["ventilationLevels"] = "error.invalid"
	}
	seen := map[string]bool{}
	for i, s := range in.Sensors {
		k := fmt.Sprintf("sensors[%d]", i)
		switch {
		case metricUnit[s.Metric] == "" || seen[s.Metric]:
			fe[k+".metric"] = "error.invalid"
		case metricUnit[s.Metric] != s.Unit:
			fe[k+".unit"] = "error.unitMismatch"
		case s.StaleAfterSeconds < 10 || s.StaleAfterSeconds > 86400:
			fe[k+".staleAfterSeconds"] = "error.range"
		case s.BoundaryID != nil && (!boundaries[*s.BoundaryID] || s.Metric != "power"):
			fe[k+".boundaryId"] = "error.invalid"
		}
		seen[s.Metric] = true
	}
	if !uniqueIn(in.FirmwareCandidates, nil) {
		fe["firmwareCandidates"] = "error.invalid"
	}
	if in.ChangeReason != nil && (utf8.RuneCountInString(*in.ChangeReason) < 1 || utf8.RuneCountInString(*in.ChangeReason) > 1000) {
		fe["changeReason"] = "error.length"
	}
	return fe
}

// UnitCapabilities is what Devices calls on Assets when a model gets a new capability version.
type UnitCapabilities interface {
	MoveToCapabilityVersion(ctx context.Context, c *ops.Call, modelID uuid.UUID, version int) (int, error)
}

// Capabilities is the operation set for the model register.
type Capabilities struct{ Units UnitCapabilities }

func nz(a []string) []string {
	if a == nil {
		return []string{}
	}
	return a
}

func (m Capabilities) save(ctx context.Context, c *ops.Call, in *CapabilitySave) (Capability, error) {
	var temp any
	if in.Temperature != nil {
		temp, _ = json.Marshal(in.Temperature)
	}
	sensors, _ := json.Marshal(append([]SensorSpec{}, in.Sensors...))
	var dup bool
	if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.capabilities WHERE is_current AND lower(manufacturer) = lower($1) AND lower(model) = lower($2)
		AND ($3::uuid IS NULL OR id <> $3))`, in.Manufacturer, in.Model, in.ID).Scan(&dup); err != nil {
		return Capability{}, err
	}
	if dup {
		return Capability{}, apperr.E(apperr.Conflict, "error.duplicateModel")
	}
	id, version := uuid.Must(uuid.NewV7()), 1
	reason := ""
	if in.ID != nil {
		var cur int
		err := c.Tx.QueryRow(ctx, `SELECT version FROM devices.capabilities WHERE id = $1 AND is_current FOR UPDATE`, *in.ID).Scan(&cur)
		if errors.Is(err, pgx.ErrNoRows) {
			return Capability{}, apperr.E(apperr.NotFound, "error.notFound")
		}
		if err != nil {
			return Capability{}, err
		}
		if cur != *c.ExpectedVersion {
			return Capability{}, apperr.E(apperr.Conflict, "error.versionConflict")
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE devices.capabilities SET is_current = false WHERE id = $1 AND version = $2`, *in.ID, cur); err != nil {
			return Capability{}, err
		}
		id, version = *in.ID, cur+1
		if in.ChangeReason != nil {
			reason = *in.ChangeReason
		}
	}
	x, err := scanCap(c.Tx.QueryRow(ctx, `INSERT INTO devices.capabilities (id, version, tenant_id, manufacturer, model, control, mode_control, fan_control, temperature,
		modes, fan_levels, ventilation, ventilation_levels, sensors, firmware_candidates, is_current)
		VALUES ($1,$2,current_setting('app.tenant_id')::uuid,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13,$14,true) RETURNING `+capCols,
		id, version, in.Manufacturer, in.Model, in.Control, in.ModeControl, in.FanControl, temp, nz(in.Modes), nz(in.FanLevels), in.Ventilation,
		nz(in.VentilationLevels), sensors, nz(in.FirmwareCandidates)))
	if err != nil {
		return Capability{}, err
	}
	if version > 1 && m.Units != nil {
		if _, err := m.Units.MoveToCapabilityVersion(ctx, c, id, version); err != nil {
			return Capability{}, err
		}
	}
	var prev *int
	if version > 1 {
		p := version - 1
		prev = &p
	}
	c.Emit(ops.Event{AggregateType: "capability", AggregateID: id, Type: "CapabilityChanged", Payload: map[string]int{"version": version}})
	c.Audit(ops.AuditEntry{Action: "capabilities.save", TargetKind: "capability", TargetID: id.String(), PreviousVersion: prev, NextVersion: &version, Reason: reason})
	return x, nil
}

// Register binds the Devices operations implemented so far.
func Register(r *ops.Registry, m Capabilities) {
	ops.Register(r, "capabilities.list", capabilitiesList)
	ops.Register(r, "capabilities.save", m.save)
}

// LoadCapability returns a capability version (UnitDetail.capabilities).
func LoadCapability(ctx context.Context, c *ops.Call, id uuid.UUID, version int) (Capability, bool, error) {
	x, err := scanCap(c.Tx.QueryRow(ctx, `SELECT `+capCols+` FROM devices.capabilities WHERE id = $1 AND version = $2`, id, version))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, false, nil
	}
	return x, err == nil, err
}
