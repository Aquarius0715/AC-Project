package devices

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"slices"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
	"github.com/pradita/ac-project/service/api/internal/platform/paging"
)

// Wave is one rollout wave.
type Wave struct {
	Label   string `json:"label"`
	Percent int    `json:"percent"`
}

// Window is the install window in device local time.
type Window struct {
	StartLocal string `json:"startLocal"`
	EndLocal   string `json:"endLocal"`
}

// CampaignResult is FirmwareCampaignResult.
type CampaignResult struct {
	DeviceID    uuid.UUID  `json:"deviceId"`
	WaveIndex   int        `json:"waveIndex"`
	Status      string     `json:"status"`
	ReasonKey   *string    `json:"reasonKey"`
	OperationID *uuid.UUID `json:"operationId"`
	At          *time.Time `json:"at"`
}

// Campaign is FirmwareCampaign of service-contracts.ts.
type Campaign struct {
	ID                      uuid.UUID        `json:"id"`
	TenantID                uuid.UUID        `json:"tenantId"`
	Version                 int              `json:"version"`
	CreatedAt               time.Time        `json:"createdAt"`
	UpdatedAt               time.Time        `json:"updatedAt"`
	ModelID                 uuid.UUID        `json:"modelId"`
	FromVersions            []string         `json:"fromVersions"`
	TargetVersion           string           `json:"targetVersion"`
	Checksum                string           `json:"checksum"`
	DeviceIDs               []uuid.UUID      `json:"deviceIds"`
	Waves                   []Wave           `json:"waves"`
	Window                  Window           `json:"window"`
	AutoPauseFailurePercent int              `json:"autoPauseFailurePercent"`
	StartAt                 time.Time        `json:"startAt"`
	State                   string           `json:"state"`
	Progress                map[string]int   `json:"progress"`
	Results                 []CampaignResult `json:"results"`
	Reason                  *string          `json:"reason"`
}

const campCols = `id, tenant_id, version, created_at, updated_at, model_id, from_versions, target_version, checksum, waves,
	to_char(window_start_local, 'HH24:MI'), to_char(window_end_local, 'HH24:MI'), auto_pause_failure_percent, start_at, state, reason`

func scanCampaign(r pgx.Row) (Campaign, error) {
	var x Campaign
	var waves []byte
	err := r.Scan(&x.ID, &x.TenantID, &x.Version, &x.CreatedAt, &x.UpdatedAt, &x.ModelID, &x.FromVersions, &x.TargetVersion, &x.Checksum, &waves,
		&x.Window.StartLocal, &x.Window.EndLocal, &x.AutoPauseFailurePercent, &x.StartAt, &x.State, &x.Reason)
	if err == nil {
		err = json.Unmarshal(waves, &x.Waves)
	}
	return x, err
}

func campaignDetail(ctx context.Context, c *ops.Call, id uuid.UUID) (Campaign, error) {
	x, err := scanCampaign(c.Tx.QueryRow(ctx, `SELECT `+campCols+` FROM devices.firmware_campaigns WHERE id = $1`, id))
	if errors.Is(err, pgx.ErrNoRows) {
		return x, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return x, err
	}
	x.Progress = map[string]int{"succeeded": 0, "installing": 0, "pending": 0, "failed": 0, "skipped": 0}
	x.Results, x.DeviceIDs = []CampaignResult{}, []uuid.UUID{}
	rows, err := c.Tx.Query(ctx, `SELECT device_id, wave, result, detail, operation_id FROM devices.firmware_campaign_devices WHERE campaign_id = $1 ORDER BY wave, device_id`, id)
	if err != nil {
		return x, err
	}
	defer rows.Close()
	for rows.Next() {
		var r CampaignResult
		if err := rows.Scan(&r.DeviceID, &r.WaveIndex, &r.Status, &r.ReasonKey, &r.OperationID); err != nil {
			return x, err
		}
		x.Results = append(x.Results, r)
		x.DeviceIDs = append(x.DeviceIDs, r.DeviceID)
		x.Progress[r.Status]++
	}
	return x, rows.Err()
}

func campaignsList(ctx context.Context, c *ops.Call, in *paging.Query) (paging.Page[Campaign], error) {
	var f struct {
		ModelID *uuid.UUID `json:"modelId,omitempty"`
		Status  *string    `json:"status,omitempty"`
	}
	if len(in.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(in.Filters)))
		dec.DisallowUnknownFields()
		if dec.Decode(&f) != nil {
			return paging.Page[Campaign]{}, apperr.Fields(map[string]string{"filters": "error.invalid"})
		}
	}
	order, err := paging.OrderBy(in.Sort, map[string]string{"id": "id", "createdAt": "created_at", "updatedAt": "updated_at", "status": "state"}, "created_at DESC, id ASC")
	if err != nil {
		return paging.Page[Campaign]{}, err
	}
	w, err := paging.Resolve(*in, f, c.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Campaign]{}, err
	}
	conds, args := []string{"TRUE"}, []any{}
	if f.ModelID != nil {
		args = append(args, *f.ModelID)
		conds = append(conds, fmt.Sprintf("model_id = $%d", len(args)))
	}
	if f.Status != nil {
		if !slices.Contains([]string{"scheduled", "running", "paused", "aborted", "completed"}, *f.Status) {
			return paging.Page[Campaign]{}, apperr.Fields(map[string]string{"filters.status": "error.invalid"})
		}
		args = append(args, *f.Status)
		conds = append(conds, fmt.Sprintf("state = $%d", len(args)))
	}
	where := strings.Join(conds, " AND ")
	var total int
	if err := c.Tx.QueryRow(ctx, "SELECT count(*) FROM devices.firmware_campaigns WHERE "+where, args...).Scan(&total); err != nil {
		return paging.Page[Campaign]{}, err
	}
	rows, err := c.Tx.Query(ctx, fmt.Sprintf("SELECT id FROM devices.firmware_campaigns WHERE %s ORDER BY %s LIMIT %d OFFSET %d", where, order, w.Limit, w.Offset), args...)
	if err != nil {
		return paging.Page[Campaign]{}, err
	}
	var ids []uuid.UUID
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			rows.Close()
			return paging.Page[Campaign]{}, err
		}
		ids = append(ids, id)
	}
	rows.Close()
	items := []Campaign{}
	for _, id := range ids {
		x, err := campaignDetail(ctx, c, id)
		if err != nil {
			return paging.Page[Campaign]{}, err
		}
		items = append(items, x)
	}
	return paging.Page[Campaign]{Items: items, NextCursor: w.Next(total), Total: total, SnapshotVersion: w.Snapshot}, nil
}

func campaignsGet(ctx context.Context, c *ops.Call, in *IDInput) (Campaign, error) {
	return campaignDetail(ctx, c, in.ID)
}

// ScheduleInput is firmwareCampaigns.schedule input.
type ScheduleInput struct {
	ModelID                 uuid.UUID   `json:"modelId"`
	TargetVersion           string      `json:"targetVersion"`
	DeviceIDs               []uuid.UUID `json:"deviceIds"`
	Waves                   []Wave      `json:"waves"`
	Window                  Window      `json:"window"`
	AutoPauseFailurePercent int         `json:"autoPauseFailurePercent"`
	StartAt                 time.Time   `json:"startAt"`
}

var hhmm = regexp.MustCompile(`^([01]\d|2[0-3]):[0-5]\d$`)

// Validate implements ops.Validator (DD-A20 / IR111).
func (in *ScheduleInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.ModelID == uuid.Nil {
		fe["modelId"] = "error.required"
	}
	if in.TargetVersion == "" {
		fe["targetVersion"] = "error.required"
	}
	if len(in.DeviceIDs) == 0 {
		fe["deviceIds"] = "error.required"
	}
	seen := map[uuid.UUID]bool{}
	for _, d := range in.DeviceIDs {
		if seen[d] {
			fe["deviceIds"] = "error.duplicate"
		}
		seen[d] = true
	}
	prev := 0
	for i, w := range in.Waves {
		if w.Percent <= prev || w.Percent > 100 || utf8.RuneCountInString(w.Label) < 1 || utf8.RuneCountInString(w.Label) > 40 {
			fe[fmt.Sprintf("waves[%d]", i)] = "error.ascending"
		}
		prev = w.Percent
	}
	if len(in.Waves) == 0 || prev != 100 {
		fe["waves"] = "error.mustEndAt100"
	}
	if !hhmm.MatchString(in.Window.StartLocal) || !hhmm.MatchString(in.Window.EndLocal) || in.Window.StartLocal == in.Window.EndLocal {
		fe["window"] = "error.invalid"
	}
	if in.AutoPauseFailurePercent < 1 || in.AutoPauseFailurePercent > 50 {
		fe["autoPauseFailurePercent"] = "error.range"
	}
	return fe
}

// waveOf assigns the i-th of n devices to the first wave whose cumulative percent covers it.
func waveOf(i, n int, waves []Wave) int {
	for w, wave := range waves {
		if (i+1)*100 <= wave.Percent*n {
			return w
		}
	}
	return len(waves) - 1
}

func (m *Module) campaignsSchedule(ctx context.Context, c *ops.Call, in *ScheduleInput) (Campaign, error) {
	if in.StartAt.Before(c.Now.Add(24 * time.Hour)) {
		return Campaign{}, apperr.Fields(map[string]string{"startAt": "error.atLeast24h"})
	}
	var candidates []string
	err := c.Tx.QueryRow(ctx, `SELECT firmware_candidates FROM devices.capabilities WHERE id = $1 AND is_current`, in.ModelID).Scan(&candidates)
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return Campaign{}, err
	}
	if !slices.Contains(candidates, in.TargetVersion) {
		return Campaign{}, apperr.Fields(map[string]string{"targetVersion": "error.unsupportedFirmware"})
	}
	// devices of the model (via their current unit's model), excluding open tamper
	rows, err := c.Tx.Query(ctx, `SELECT d.id, d.firmware_version, d.tamper, COALESCE(d.unit_id, d.target_unit_id) FROM devices.devices d WHERE d.id = ANY($1)`, in.DeviceIDs)
	if err != nil {
		return Campaign{}, err
	}
	type dv struct {
		fw, tamper string
		unit       uuid.UUID
	}
	found := map[uuid.UUID]dv{}
	for rows.Next() {
		var id uuid.UUID
		var x dv
		if err := rows.Scan(&id, &x.fw, &x.tamper, &x.unit); err != nil {
			rows.Close()
			return Campaign{}, err
		}
		found[id] = x
	}
	rows.Close()
	from := []string{}
	for _, id := range in.DeviceIDs {
		x, ok := found[id]
		if !ok {
			return Campaign{}, apperr.Fields(map[string]string{"deviceIds": "error.unknownDevice"})
		}
		if x.tamper == "detected" {
			return Campaign{}, apperr.Fields(map[string]string{"deviceIds": "error.tamperUnresolved"})
		}
		if x.fw == in.TargetVersion {
			return Campaign{}, apperr.Fields(map[string]string{"deviceIds": "error.alreadyOnTarget"})
		}
		u, ok, err := m.Units.UnitInfo(ctx, c, x.unit)
		if err != nil {
			return Campaign{}, err
		}
		if !ok || u.ModelID != in.ModelID {
			return Campaign{}, apperr.Fields(map[string]string{"deviceIds": "error.otherModel"})
		}
		if !slices.Contains(from, x.fw) {
			from = append(from, x.fw)
		}
	}
	slices.Sort(from)
	sum := sha256.Sum256([]byte(in.ModelID.String() + ":" + in.TargetVersion))
	id := uuid.Must(uuid.NewV7())
	waves, _ := json.Marshal(in.Waves)
	if _, err := c.Tx.Exec(ctx, `INSERT INTO devices.firmware_campaigns (id, tenant_id, model_id, from_versions, target_version, checksum, waves, window_start_local,
		window_end_local, auto_pause_failure_percent, start_at, state) VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7::time, $8::time, $9, $10, 'scheduled')`,
		id, in.ModelID, from, in.TargetVersion, "sha256:"+hex.EncodeToString(sum[:]), waves, in.Window.StartLocal, in.Window.EndLocal, in.AutoPauseFailurePercent, in.StartAt); err != nil {
		return Campaign{}, err
	}
	for i, d := range in.DeviceIDs {
		if _, err := c.Tx.Exec(ctx, `INSERT INTO devices.firmware_campaign_devices (campaign_id, device_id, tenant_id, wave) VALUES ($1, $2, current_setting('app.tenant_id')::uuid, $3)`,
			id, d, waveOf(i, len(in.DeviceIDs), in.Waves)); err != nil {
			return Campaign{}, err
		}
	}
	one := 1
	c.Emit(ops.Event{AggregateType: "firmware_campaign", AggregateID: id, Type: "FirmwareCampaignScheduled"})
	c.Audit(ops.AuditEntry{Action: "firmwareCampaigns.schedule", TargetKind: "firmware_campaign", TargetID: id.String(), NextVersion: &one})
	return campaignDetail(ctx, c, id)
}

// ControlInput is firmwareCampaigns.control input.
type ControlInput struct {
	CampaignID uuid.UUID  `json:"campaignId"`
	Action     string     `json:"action"`
	DeviceID   *uuid.UUID `json:"deviceId,omitempty"`
	Reason     *string    `json:"reason,omitempty"`
}

// Validate implements ops.Validator (IR111: abort reason 1–1000; retry_device needs deviceId).
func (in *ControlInput) Validate() map[string]string {
	fe := map[string]string{}
	if in.CampaignID == uuid.Nil {
		fe["campaignId"] = "error.required"
	}
	switch in.Action {
	case "pause", "resume":
	case "abort":
		if in.Reason == nil || utf8.RuneCountInString(strings.TrimSpace(*in.Reason)) < 1 || utf8.RuneCountInString(strings.TrimSpace(*in.Reason)) > 1000 {
			fe["reason"] = "error.length"
		}
	case "retry_device":
		if in.DeviceID == nil {
			fe["deviceId"] = "error.required"
		}
	default:
		fe["action"] = "error.invalid"
	}
	return fe
}

var transitions = map[string]map[string]string{
	"pause":  {"running": "paused", "scheduled": "paused"},
	"resume": {"paused": "running"},
	"abort":  {"scheduled": "aborted", "running": "aborted", "paused": "aborted"},
}

func campaignsControl(ctx context.Context, c *ops.Call, in *ControlInput) (Campaign, error) {
	var state string
	var v int
	var start time.Time
	err := c.Tx.QueryRow(ctx, `SELECT state, version, start_at FROM devices.firmware_campaigns WHERE id = $1 FOR UPDATE`, in.CampaignID).Scan(&state, &v, &start)
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, apperr.E(apperr.NotFound, "error.notFound")
	}
	if err != nil {
		return Campaign{}, err
	}
	if v != *c.ExpectedVersion {
		return Campaign{}, apperr.E(apperr.Conflict, "error.versionConflict")
	}
	next := state
	var reason *string
	switch in.Action {
	case "retry_device":
		if state != "running" && state != "paused" {
			return Campaign{}, apperr.E(apperr.Conflict, "error.invalidState")
		}
		tag, err := c.Tx.Exec(ctx, `UPDATE devices.firmware_campaign_devices SET result = 'pending', detail = NULL, operation_id = NULL
			WHERE campaign_id = $1 AND device_id = $2 AND result IN ('failed','skipped')`, in.CampaignID, *in.DeviceID)
		if err != nil {
			return Campaign{}, err
		}
		if tag.RowsAffected() == 0 {
			return Campaign{}, apperr.E(apperr.Conflict, "error.retryOnlyFailedOrSkipped")
		}
	default:
		to, ok := transitions[in.Action][state]
		if !ok {
			return Campaign{}, apperr.E(apperr.Conflict, "error.invalidState")
		}
		if in.Action == "resume" && c.Now.Before(start) {
			to = "scheduled" // resuming before the start time returns to scheduled
		}
		next = to
		if in.Action == "abort" {
			r := strings.TrimSpace(*in.Reason)
			reason = &r
		}
	}
	if err := c.Tx.QueryRow(ctx, `UPDATE devices.firmware_campaigns SET state = $2, reason = COALESCE($3, reason), version = version + 1, updated_at = platform.app_now()
		WHERE id = $1 RETURNING version`, in.CampaignID, next, reason).Scan(&v); err != nil {
		return Campaign{}, err
	}
	r := ""
	if reason != nil {
		r = *reason
	}
	c.Emit(ops.Event{AggregateType: "firmware_campaign", AggregateID: in.CampaignID, Type: "FirmwareCampaignChanged", Payload: map[string]string{"action": in.Action, "state": next}})
	c.Audit(ops.AuditEntry{Action: "firmwareCampaigns.control", TargetKind: "firmware_campaign", TargetID: in.CampaignID.String(), PreviousVersion: c.ExpectedVersion, NextVersion: &v, Reason: r})
	return campaignDetail(ctx, c, in.CampaignID)
}

// RegisterCampaigns binds the firmware campaign operations.
func RegisterCampaigns(r *ops.Registry, m *Module) {
	ops.Register(r, "firmwareCampaigns.list", campaignsList)
	ops.Register(r, "firmwareCampaigns.get", campaignsGet)
	ops.Register(r, "firmwareCampaigns.schedule", m.campaignsSchedule)
	ops.Register(r, "firmwareCampaigns.control", campaignsControl)
}
