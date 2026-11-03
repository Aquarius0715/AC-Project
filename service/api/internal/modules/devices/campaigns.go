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
	CreatedByMembershipID   *uuid.UUID       `json:"createdByMembershipId"` // who scheduled it (IR344)
}

const campCols = `id, tenant_id, version, created_at, updated_at, model_id, from_versions, target_version, checksum, ` +
	`waves,
	to_char(window_start_local, 'HH24:MI'), to_char(window_end_local, 'HH24:MI'), auto_pause_failure_percent, start_at, ` +
	`state, reason, created_by_membership_id`

func scanCampaign(row pgx.Row) (Campaign, error) {
	var (
		campaign Campaign
		waves    []byte
	)

	err := row.Scan(
		&campaign.ID,
		&campaign.TenantID,
		&campaign.Version,
		&campaign.CreatedAt,
		&campaign.UpdatedAt,
		&campaign.ModelID,
		&campaign.FromVersions,
		&campaign.TargetVersion,
		&campaign.Checksum,
		&waves,
		&campaign.Window.StartLocal,
		&campaign.Window.EndLocal,
		&campaign.AutoPauseFailurePercent,
		&campaign.StartAt,
		&campaign.State,
		&campaign.Reason,
		&campaign.CreatedByMembershipID,
	)
	if err == nil {
		err = json.Unmarshal(waves, &campaign.Waves)
	}

	return campaign, apperr.Wrap(err, "json.Unmarshal")
}

func campaignDetail(ctx context.Context, call *ops.Call, campaignID uuid.UUID) (Campaign, error) {
	campaign, err := scanCampaign(
		call.Tx.QueryRow(
			ctx,
			`SELECT `+campCols+` FROM devices.firmware_campaigns WHERE id = $1`,
			campaignID,
		),
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return campaign, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return campaign, err
	}

	campaign.Progress = map[string]int{
		"succeeded":  0,
		"installing": 0,
		"pending":    0,
		"failed":     0,
		"skipped":    0,
	}
	campaign.Results, campaign.DeviceIDs = []CampaignResult{}, []uuid.UUID{}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT device_id, wave, result, detail, operation_id, result_at FROM devices.firmware_campaign_devices WHERE
			campaign_id = $1 ORDER BY wave, device_id`,
		campaignID,
	)
	if err != nil {
		return campaign, apperr.Wrap(err, "select devices.firmware_campaign_devices")
	}
	defer rows.Close()

	for rows.Next() {
		var campaignResult CampaignResult

		err := rows.Scan(
			&campaignResult.DeviceID,
			&campaignResult.WaveIndex,
			&campaignResult.Status,
			&campaignResult.ReasonKey,
			&campaignResult.OperationID,
			&campaignResult.At,
		)
		if err != nil {
			return campaign, apperr.Wrap(err, "select devices.firmware_campaign_devices")
		}

		campaign.Results = append(campaign.Results, campaignResult)
		campaign.DeviceIDs = append(campaign.DeviceIDs, campaignResult.DeviceID)
		campaign.Progress[campaignResult.Status]++
	}

	return campaign, apperr.Wrap(rows.Err(), "select devices.firmware_campaign_devices")
}

// campaignsList serves firmwareCampaigns.list.
//
//	@Summary		firmwareCampaigns.list (read)
//	@ID				firmwareCampaigns.list
//	@Description	Authorization: admin:device.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A20 · Query: filters modelId,status · sort id,createdAt,updatedAt,status (default \
//	@Description	createdAt desc;id asc)
//	@Tags			firmwareCampaigns
//	@Accept			json
//	@Produce		json
//	@Param			cursor	query		string	false	"page cursor: nextCursor of the previous page (D12)"
//	@Param			limit	query		integer	false	"page size 1–100, default 25"
//	@Param			sort	query		string	false	"field:direction — fields id,createdAt,updatedAt,status; default createdAt desc;id asc"
//	@Param			modelId	query		string	false	"filter → modelId"
//	@Param			status	query		string	false	"filter → state"
//	@Success		200		{object}	ops.Envelope{data=CampaignPage}
//	@Failure		401		{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403		{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404		{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409		{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422		{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429		{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503		{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504		{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/firmware-campaigns [get]
func campaignsList(
	ctx context.Context,
	call *ops.Call,
	query *paging.Query,
) (paging.Page[Campaign], error) {
	var filter struct {
		ModelID *uuid.UUID `json:"modelId,omitempty"`
		Status  *string    `json:"status,omitempty"`
	}

	if len(query.Filters) > 0 {
		dec := json.NewDecoder(strings.NewReader(string(query.Filters)))
		dec.DisallowUnknownFields()

		if dec.Decode(&filter) != nil {
			return paging.Page[Campaign]{}, apperr.Fields(
				map[string]string{"filters": apperr.KeyInvalid},
			)
		}
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":        "id",
			"createdAt": "created_at",
			"updatedAt": "updated_at",
			"status":    "state",
		},
		"created_at DESC, id ASC",
	)
	if err != nil {
		return paging.Page[Campaign]{}, apperr.Wrap(err, "paging.OrderBy")
	}

	window, err := paging.Resolve(*query, filter, call.Principal.ScopeVersion, 1)
	if err != nil {
		return paging.Page[Campaign]{}, apperr.Wrap(err, "paging.Resolve")
	}

	conds, args := []string{"TRUE"}, []any{}
	if filter.ModelID != nil {
		args = append(args, *filter.ModelID)
		conds = append(conds, fmt.Sprintf("model_id = $%d", len(args)))
	}

	if filter.Status != nil {
		if !slices.Contains(
			[]string{"scheduled", "running", "paused", "aborted", "completed"},
			*filter.Status,
		) {
			return paging.Page[Campaign]{}, apperr.Fields(
				map[string]string{"filters.status": apperr.KeyInvalid},
			)
		}

		args = append(args, *filter.Status)
		conds = append(conds, fmt.Sprintf("state = $%d", len(args)))
	}

	where := strings.Join(conds, " AND ")

	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM devices.firmware_campaigns WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return paging.Page[Campaign]{}, apperr.Wrap(scanErr, "select devices.firmware_campaigns")
	}

	rows, err := call.Tx.Query(
		ctx,
		fmt.Sprintf(
			"SELECT id FROM devices.firmware_campaigns WHERE %s ORDER BY %s LIMIT %d OFFSET %d",
			where,
			order,
			window.Limit,
			window.Offset,
		),
		args...)
	if err != nil {
		return paging.Page[Campaign]{}, apperr.Wrap(err, "select devices.firmware_campaigns")
	}

	var ids []uuid.UUID

	for rows.Next() {
		var campaignID uuid.UUID

		err := rows.Scan(&campaignID)
		if err != nil {
			rows.Close()

			return paging.Page[Campaign]{}, apperr.Wrap(err, "select devices.firmware_campaigns")
		}

		ids = append(ids, campaignID)
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return paging.Page[Campaign]{}, apperr.Wrap(rowsErr, "select devices.firmware_campaigns")
	}

	items := []Campaign{}

	for _, id := range ids {
		x, err := campaignDetail(ctx, call, id)
		if err != nil {
			return paging.Page[Campaign]{}, err
		}

		items = append(items, x)
	}

	return paging.Page[Campaign]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// campaignsGet serves firmwareCampaigns.get.
//
//	@Summary		firmwareCampaigns.get (read)
//	@ID				firmwareCampaigns.get
//	@Description	Authorization: admin:device.read
//	@Description	Validation: D01; input constraints in the corresponding DD; scope-bound snapshot
//	@Description	Recovery: D04: retry only UNAVAILABLE, at most twice
//	@Description	Design: DD-A20
//	@Tags			firmwareCampaigns
//	@Accept			json
//	@Produce		json
//	@Param			id	path		string	true	"input field id"
//	@Success		200	{object}	ops.Envelope{data=Campaign}
//	@Failure		401	{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403	{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404	{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409	{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422	{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429	{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503	{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504	{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/firmware-campaigns/{id} [get]
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
	fieldErrs := map[string]string{}
	if in.ModelID == uuid.Nil {
		fieldErrs["modelId"] = apperr.KeyRequired
	}

	if in.TargetVersion == "" {
		fieldErrs["targetVersion"] = apperr.KeyRequired
	}

	if len(in.DeviceIDs) == 0 {
		fieldErrs["deviceIds"] = apperr.KeyRequired
	}

	seen := map[uuid.UUID]bool{}
	for _, d := range in.DeviceIDs {
		if seen[d] {
			fieldErrs["deviceIds"] = apperr.KeyDuplicate
		}

		seen[d] = true
	}

	prev := 0
	for i, wave := range in.Waves {
		if wave.Percent <= prev || wave.Percent > 100 || utf8.RuneCountInString(wave.Label) < 1 ||
			utf8.RuneCountInString(wave.Label) > 40 {
			fieldErrs[fmt.Sprintf("waves[%d]", i)] = "error.ascending"
		}

		prev = wave.Percent
	}

	if len(in.Waves) == 0 || prev != 100 {
		fieldErrs["waves"] = "error.mustEndAt100"
	}

	if !hhmm.MatchString(in.Window.StartLocal) || !hhmm.MatchString(in.Window.EndLocal) ||
		in.Window.StartLocal == in.Window.EndLocal {
		fieldErrs["window"] = apperr.KeyInvalid
	}

	if in.AutoPauseFailurePercent < 1 || in.AutoPauseFailurePercent > 50 {
		fieldErrs["autoPauseFailurePercent"] = apperr.KeyRange
	}

	return fieldErrs
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

// campaignsSchedule serves firmwareCampaigns.schedule.
//
//	@Summary		firmwareCampaigns.schedule (write)
//	@ID				firmwareCampaigns.schedule
//	@Description	Authorization: admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 signed target version from Capability.firmwareCandidates; startAt at least 24 h ahead; waves \
//	@Description	ascending ending at 100; autoPause 1–50 %
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A20
//	@Tags			firmwareCampaigns
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key	header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			request			body		ScheduleInput	true	"input"
//	@Success		200				{object}	ops.Envelope{data=Campaign}
//	@Failure		401				{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403				{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404				{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409				{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422				{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429				{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503				{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504				{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/firmware-campaigns [post]
func (m *Module) campaignsSchedule(
	ctx context.Context,
	call *ops.Call,
	input *ScheduleInput,
) (Campaign, error) {
	if input.StartAt.Before(call.Now.Add(24 * time.Hour)) {
		return Campaign{}, apperr.Fields(map[string]string{"startAt": "error.atLeast24h"})
	}

	var candidates []string

	err := call.Tx.QueryRow(ctx, `SELECT firmware_candidates FROM devices.capabilities WHERE id = $1 AND
		is_current`, input.ModelID).
		Scan(&candidates)
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return Campaign{}, apperr.Wrap(err, "select devices.capabilities")
	}

	if !slices.Contains(candidates, input.TargetVersion) {
		return Campaign{}, apperr.Fields(
			map[string]string{"targetVersion": "error.unsupportedFirmware"},
		)
	}
	// devices of the model (via their current unit's model), excluding open tamper
	rows, err := call.Tx.Query(
		ctx,
		`SELECT d.id, d.firmware_version, d.tamper, COALESCE(d.unit_id, d.target_unit_id) FROM devices.devices d WHERE d.id =
			ANY($1)`,
		input.DeviceIDs,
	)
	if err != nil {
		return Campaign{}, apperr.Wrap(err, "select devices.devices")
	}

	type dv struct {
		fw, tamper string
		unit       uuid.UUID
	}

	found := map[uuid.UUID]dv{}

	for rows.Next() {
		var (
			deviceID uuid.UUID
			device   dv
		)

		err := rows.Scan(&deviceID, &device.fw, &device.tamper, &device.unit)
		if err != nil {
			rows.Close()

			return Campaign{}, apperr.Wrap(err, "select devices.devices")
		}

		found[deviceID] = device
	}

	rows.Close()

	rowsErr := rows.Err()
	if rowsErr != nil {
		return Campaign{}, apperr.Wrap(rowsErr, "select devices.devices")
	}

	from := []string{}

	for _, id := range input.DeviceIDs {
		device, ok := found[id]
		if !ok {
			return Campaign{}, apperr.Fields(map[string]string{"deviceIds": "error.unknownDevice"})
		}

		if device.tamper == "detected" {
			return Campaign{}, apperr.Fields(
				map[string]string{"deviceIds": apperr.KeyTamperUnresolved},
			)
		}

		if device.fw == input.TargetVersion {
			return Campaign{}, apperr.Fields(
				map[string]string{"deviceIds": "error.alreadyOnTarget"},
			)
		}

		u, ok, err := m.Units.UnitInfo(ctx, call, device.unit)
		if err != nil {
			return Campaign{}, apperr.Wrap(err, "devices.Units.UnitInfo")
		}

		if !ok || u.ModelID != input.ModelID {
			return Campaign{}, apperr.Fields(map[string]string{"deviceIds": "error.otherModel"})
		}

		if !slices.Contains(from, device.fw) {
			from = append(from, device.fw)
		}
	}

	slices.Sort(from)

	sum := sha256.Sum256([]byte(input.ModelID.String() + ":" + input.TargetVersion))
	campaignID := uuid.Must(uuid.NewV7())

	waves, _ := json.Marshal(input.Waves)

	_, execErr := call.Tx.Exec(
		ctx,
		`INSERT INTO devices.firmware_campaigns (id, tenant_id, model_id, from_versions, target_version, checksum, waves,
			window_start_local,
		window_end_local, auto_pause_failure_percent, start_at, state, created_by_membership_id) VALUES ($1,
		current_setting('app.tenant_id')::uuid, $2, $3, $4, $5, $6, $7::time, $8::time, $9, $10, 'scheduled', $11)`,
		campaignID,
		input.ModelID,
		from,
		input.TargetVersion,
		"sha256:"+hex.EncodeToString(sum[:]),
		waves,
		input.Window.StartLocal,
		input.Window.EndLocal,
		input.AutoPauseFailurePercent,
		input.StartAt,
		call.Principal.MembershipID,
	)
	if execErr != nil {
		return Campaign{}, apperr.Wrap(execErr, "insert devices.firmware_campaigns")
	}

	for idx, deviceID := range input.DeviceIDs {
		_, err := call.Tx.Exec(
			ctx,
			`INSERT INTO devices.firmware_campaign_devices (campaign_id, device_id, tenant_id, wave) VALUES ($1, $2,
				current_setting('app.tenant_id')::uuid, $3)`,
			campaignID,
			deviceID,
			waveOf(idx, len(input.DeviceIDs), input.Waves),
		)
		if err != nil {
			return Campaign{}, apperr.Wrap(err, "insert devices.firmware_campaign_devices")
		}
	}

	one := 1

	call.Emit(
		ops.Event{
			AggregateType: "firmware_campaign",
			AggregateID:   campaignID,
			Type:          "FirmwareCampaignScheduled", Payload: nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "firmwareCampaigns.schedule",
			TargetKind:      "firmware_campaign",
			TargetID:        campaignID.String(),
			NextVersion:     &one,
			PreviousVersion: nil,
			Reason:          "",
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)

	return campaignDetail(ctx, call, campaignID)
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
	fieldErrs := map[string]string{}
	if in.CampaignID == uuid.Nil {
		fieldErrs["campaignId"] = apperr.KeyRequired
	}

	switch in.Action {
	case "pause", "resume":
	case "abort":
		if in.Reason == nil || utf8.RuneCountInString(strings.TrimSpace(*in.Reason)) < 1 ||
			utf8.RuneCountInString(strings.TrimSpace(*in.Reason)) > 1000 {
			fieldErrs["reason"] = apperr.KeyLength
		}
	case "retry_device":
		if in.DeviceID == nil {
			fieldErrs["deviceId"] = apperr.KeyRequired
		}
	default:
		fieldErrs["action"] = apperr.KeyInvalid
	}

	return fieldErrs
}

var transitions = map[string]map[string]string{
	"pause":  {"running": "paused", "scheduled": "paused"},
	"resume": {"paused": "running"},
	"abort":  {"scheduled": "aborted", "running": "aborted", "paused": "aborted"},
}

// campaignsControl serves firmwareCampaigns.control.
//
//	@Summary		firmwareCampaigns.control (write)
//	@ID				firmwareCampaigns.control
//	@Description	Authorization: admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR111 abort requires reason 1–1000; retry_device only for failed or skipped devices
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A20 · This route sets action=pause
//	@Tags			firmwareCampaigns
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string			true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer			true	"all: required (target firmware_campaign, read firmwareCampaigns.get)"
//	@Param			campaignId			path		string			true	"input field campaignId"
//	@Param			request				body		ControlInput	true	"input; the path parameters and the fixed field come from the route"
//	@Success		200					{object}	ops.Envelope{data=Campaign}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/firmware-campaigns/{campaignId}/pause [post]
func campaignsControl(ctx context.Context, c *ops.Call, input *ControlInput) (Campaign, error) {
	var (
		state   string
		version int
		start   time.Time
	)

	err := c.Tx.QueryRow(ctx, `SELECT state, version, start_at FROM devices.firmware_campaigns WHERE id = $1 FOR
		UPDATE`, input.CampaignID).
		Scan(&state, &version, &start)
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return Campaign{}, apperr.Wrap(err, "select devices.firmware_campaigns")
	}

	if version != *c.ExpectedVersion {
		return Campaign{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	next := state

	var reason *string

	switch input.Action {
	case "retry_device":
		if state != "running" && state != "paused" {
			return Campaign{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
		}

		tag, err := c.Tx.Exec(
			ctx,
			`UPDATE devices.firmware_campaign_devices SET result = 'pending', detail = NULL, operation_id = NULL, result_at =
				NULL
			WHERE campaign_id = $1 AND device_id = $2 AND result IN ('failed','skipped')`,
			input.CampaignID,
			*input.DeviceID,
		)
		if err != nil {
			return Campaign{}, apperr.Wrap(err, "update devices.firmware_campaign_devices")
		}

		if tag.RowsAffected() == 0 {
			return Campaign{}, apperr.E(apperr.Conflict, "error.retryOnlyFailedOrSkipped")
		}
	default:
		target, ok := transitions[input.Action][state]
		if !ok {
			return Campaign{}, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
		}

		if input.Action == "resume" && c.Now.Before(start) {
			target = "scheduled" // resuming before the start time returns to scheduled
		}

		if input.Action == "resume" {
			clear := ""
			reason = &clear // the runner's pause reason ends with the pause (IR339)
		}

		next = target

		if input.Action == "abort" {
			r := strings.TrimSpace(*input.Reason)
			reason = &r
		}
	}

	scanErr := c.Tx.QueryRow(ctx, `UPDATE devices.firmware_campaigns SET state = $2, reason = NULLIF(COALESCE($3, reason),
		''), version = version + 1, updated_at = platform.app_now()
		WHERE id = $1 RETURNING version`, input.CampaignID, next, reason).
		Scan(&version)
	if scanErr != nil {
		return Campaign{}, apperr.Wrap(scanErr, "update devices.firmware_campaigns")
	}

	auditReason := ""
	if reason != nil {
		auditReason = *reason
	}

	c.Emit(
		ops.Event{
			AggregateType: "firmware_campaign",
			AggregateID:   input.CampaignID,
			Type:          "FirmwareCampaignChanged",
			Payload:       map[string]string{"action": input.Action, "state": next},
		},
	)
	c.Audit(
		ops.AuditEntry{
			Action:          "firmwareCampaigns.control",
			TargetKind:      "firmware_campaign",
			TargetID:        input.CampaignID.String(),
			PreviousVersion: c.ExpectedVersion,
			NextVersion:     &version,
			Reason:          auditReason, Result: "", Before: nil, After: nil,
		},
	)

	return campaignDetail(ctx, c, input.CampaignID)
}

// RegisterCampaigns binds the firmware campaign operations.
func RegisterCampaigns(r *ops.Registry, m *Module) {
	ops.Register(r, "firmwareCampaigns.list", campaignsList)
	ops.Register(r, "firmwareCampaigns.get", campaignsGet)
	ops.Register(r, "firmwareCampaigns.schedule", m.campaignsSchedule)
	ops.Register(r, "firmwareCampaigns.control", campaignsControl)
}
