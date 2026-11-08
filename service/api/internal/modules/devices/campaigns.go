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

// FirmwareCampaign states, the control actions that move them and the results of a campaign's devices.
const (
	entityCampaign = "firmware_campaign"

	stateScheduled = "scheduled"
	stateRunning   = "running"
	statePaused    = "paused"
	stateAborted   = "aborted"
	stateCompleted = "completed"

	actionPause       = "pause"
	actionResume      = "resume"
	actionAbort       = "abort"
	actionRetryDevice = "retry_device"

	resultPending    = "pending"
	resultInstalling = "installing"
	resultSucceeded  = "succeeded"
	resultFailed     = "failed"
	resultSkipped    = "skipped"
)

// minScheduleLead is how far ahead a campaign must start (DD-A20: startAt at least 24 h ahead).
const minScheduleLead = 24 * time.Hour

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
		resultSucceeded:  0,
		resultInstalling: 0,
		resultPending:    0,
		resultFailed:     0,
		resultSkipped:    0,
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
	var filter campaignFilter

	err := decodeFilters(query, &filter)
	if err != nil {
		return paging.Page[Campaign]{}, err
	}

	order, err := paging.OrderBy(
		query.Sort,
		map[string]string{
			"id":           "id",
			fieldCreatedAt: colCreatedAt,
			fieldUpdatedAt: "updated_at",
			fieldStatus:    "state",
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

	where, args, err := campaignConditions(filter)
	if err != nil {
		return paging.Page[Campaign]{}, err
	}

	ids, total, err := campaignPage(ctx, call, where, order, window, args)
	if err != nil {
		return paging.Page[Campaign]{}, err
	}

	items := []Campaign{}

	for _, id := range ids {
		campaign, err := campaignDetail(ctx, call, id)
		if err != nil {
			return paging.Page[Campaign]{}, err
		}

		items = append(items, campaign)
	}

	return paging.Page[Campaign]{
		Items:           items,
		NextCursor:      window.Next(total),
		Total:           total,
		SnapshotVersion: window.Snapshot,
	}, nil
}

// campaignFilter is the firmwareCampaigns.list filters of the query catalog.
type campaignFilter struct {
	ModelID *uuid.UUID `json:"modelId,omitempty"`
	Status  *string    `json:"status,omitempty"`
}

// campaignConditions is the firmwareCampaigns.list WHERE clause and its arguments (a status that is no campaign state
// is VALIDATION).
func campaignConditions(filter campaignFilter) (string, []any, error) {
	conds, args := []string{"TRUE"}, []any{}
	if filter.ModelID != nil {
		args = append(args, *filter.ModelID)
		conds = append(conds, fmt.Sprintf("model_id = $%d", len(args)))
	}

	if filter.Status != nil {
		if !slices.Contains(
			[]string{stateScheduled, stateRunning, statePaused, stateAborted, stateCompleted},
			*filter.Status,
		) {
			return "", nil, apperr.Fields(
				map[string]string{"filters.status": apperr.KeyInvalid},
			)
		}

		args = append(args, *filter.Status)
		conds = append(conds, fmt.Sprintf("state = $%d", len(args)))
	}

	return strings.Join(conds, " AND "), args, nil
}

// campaignPage counts the campaigns that match and reads the IDs of the page.
func campaignPage(
	ctx context.Context,
	call *ops.Call,
	where, order string,
	window paging.Window,
	args []any,
) ([]uuid.UUID, int, error) {
	var total int

	scanErr := call.Tx.QueryRow(ctx, "SELECT count(*) FROM devices.firmware_campaigns WHERE "+where, args...).
		Scan(&total)
	if scanErr != nil {
		return nil, 0, apperr.Wrap(scanErr, "select devices.firmware_campaigns")
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
		return nil, 0, apperr.Wrap(err, "select devices.firmware_campaigns")
	}

	var ids []uuid.UUID

	for rows.Next() {
		var campaignID uuid.UUID

		err := rows.Scan(&campaignID)
		if err != nil {
			rows.Close()

			return nil, 0, apperr.Wrap(err, "select devices.firmware_campaigns")
		}

		ids = append(ids, campaignID)
	}

	rows.Close()

	return ids, total, apperr.Wrap(rows.Err(), "select devices.firmware_campaigns")
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

	validateDeviceIDs(fieldErrs, in.DeviceIDs)
	validateWaves(fieldErrs, in.Waves)

	if !hhmm.MatchString(in.Window.StartLocal) || !hhmm.MatchString(in.Window.EndLocal) ||
		in.Window.StartLocal == in.Window.EndLocal {
		fieldErrs["window"] = apperr.KeyInvalid
	}

	if in.AutoPauseFailurePercent < 1 || in.AutoPauseFailurePercent > 50 {
		fieldErrs["autoPauseFailurePercent"] = apperr.KeyRange
	}

	return fieldErrs
}

// validateDeviceIDs requires at least one device and no device twice.
func validateDeviceIDs(fieldErrs map[string]string, deviceIDs []uuid.UUID) {
	if len(deviceIDs) == 0 {
		fieldErrs[fieldDeviceIDs] = apperr.KeyRequired
	}

	seen := map[uuid.UUID]bool{}
	for _, deviceID := range deviceIDs {
		if seen[deviceID] {
			fieldErrs[fieldDeviceIDs] = apperr.KeyDuplicate
		}

		seen[deviceID] = true
	}
}

// validateWaves requires waves of 1–40 character labels whose cumulative percents ascend and end at 100.
func validateWaves(fieldErrs map[string]string, waves []Wave) {
	prev := 0
	for i, wave := range waves {
		if wave.Percent <= prev || wave.Percent > 100 || utf8.RuneCountInString(wave.Label) < 1 ||
			utf8.RuneCountInString(wave.Label) > 40 {
			fieldErrs[fmt.Sprintf("waves[%d]", i)] = "error.ascending"
		}

		prev = wave.Percent
	}

	if len(waves) == 0 || prev != 100 {
		fieldErrs["waves"] = "error.mustEndAt100"
	}
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
	if input.StartAt.Before(call.Now.Add(minScheduleLead)) {
		return Campaign{}, apperr.Fields(map[string]string{"startAt": "error.atLeast24h"})
	}

	err := targetSupported(ctx, call, input)
	if err != nil {
		return Campaign{}, err
	}

	from, err := m.fromVersions(ctx, call, input)
	if err != nil {
		return Campaign{}, err
	}

	campaignID, err := insertCampaign(ctx, call, input, from)
	if err != nil {
		return Campaign{}, err
	}

	one := 1

	call.Emit(
		ops.Event{
			AggregateType: entityCampaign,
			AggregateID:   campaignID,
			Type:          "FirmwareCampaignScheduled", Payload: nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "firmwareCampaigns.schedule",
			TargetKind:      entityCampaign,
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

// targetSupported requires a current model whose firmware candidates include the target version (IR111).
func targetSupported(ctx context.Context, call *ops.Call, input *ScheduleInput) error {
	var candidates []string

	err := call.Tx.QueryRow(ctx, `SELECT firmware_candidates FROM devices.capabilities WHERE id = $1 AND
		is_current`, input.ModelID).
		Scan(&candidates)
	if errors.Is(err, pgx.ErrNoRows) {
		return apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return apperr.Wrap(err, "select devices.capabilities")
	}

	if !slices.Contains(candidates, input.TargetVersion) {
		return apperr.Fields(
			map[string]string{"targetVersion": "error.unsupportedFirmware"},
		)
	}

	return nil
}

// scheduledDevice is a device a campaign is scheduled for: its firmware version, tamper state and unit.
type scheduledDevice struct {
	fw, tamper string
	unit       uuid.UUID
}

// fromVersions checks the campaign's devices, the devices of the model (via their current unit's model) without an
// open tamper and not on the target version yet, and returns their firmware versions, sorted.
func (m *Module) fromVersions(
	ctx context.Context,
	call *ops.Call,
	input *ScheduleInput,
) ([]string, error) {
	devicesByID, err := scheduledDevices(ctx, call, input.DeviceIDs)
	if err != nil {
		return nil, err
	}

	from := []string{}

	for _, deviceID := range input.DeviceIDs {
		device, known := devicesByID[deviceID]
		if !known {
			return nil, apperr.Fields(map[string]string{fieldDeviceIDs: "error.unknownDevice"})
		}

		if device.tamper == tamperDetected {
			return nil, apperr.Fields(
				map[string]string{fieldDeviceIDs: apperr.KeyTamperUnresolved},
			)
		}

		if device.fw == input.TargetVersion {
			return nil, apperr.Fields(
				map[string]string{fieldDeviceIDs: "error.alreadyOnTarget"},
			)
		}

		unitInfo, unitFound, err := m.Units.UnitInfo(ctx, call, device.unit)
		if err != nil {
			return nil, apperr.Wrap(err, "devices.Units.UnitInfo")
		}

		if !unitFound || unitInfo.ModelID != input.ModelID {
			return nil, apperr.Fields(map[string]string{fieldDeviceIDs: "error.otherModel"})
		}

		if !slices.Contains(from, device.fw) {
			from = append(from, device.fw)
		}
	}

	slices.Sort(from)

	return from, nil
}

// scheduledDevices reads the campaign's devices by ID.
func scheduledDevices(
	ctx context.Context,
	call *ops.Call,
	deviceIDs []uuid.UUID,
) (map[uuid.UUID]scheduledDevice, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT d.id, d.firmware_version, d.tamper, COALESCE(d.unit_id, d.target_unit_id) FROM devices.devices d WHERE d.id =
			ANY($1)`,
		deviceIDs,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select devices.devices")
	}

	devicesByID := map[uuid.UUID]scheduledDevice{}

	for rows.Next() {
		var (
			deviceID uuid.UUID
			device   scheduledDevice
		)

		err := rows.Scan(&deviceID, &device.fw, &device.tamper, &device.unit)
		if err != nil {
			rows.Close()

			return nil, apperr.Wrap(err, "select devices.devices")
		}

		devicesByID[deviceID] = device
	}

	rows.Close()

	return devicesByID, apperr.Wrap(rows.Err(), "select devices.devices")
}

// insertCampaign stores the scheduled campaign and each of its devices in its wave, and returns the campaign ID.
func insertCampaign(
	ctx context.Context,
	call *ops.Call,
	input *ScheduleInput,
	from []string,
) (uuid.UUID, error) {
	sum := sha256.Sum256([]byte(input.ModelID.String() + ":" + input.TargetVersion))
	campaignID := uuid.Must(uuid.NewV7())

	waves, err := json.Marshal(input.Waves)
	if err != nil {
		return uuid.Nil, apperr.Wrap(err, "json.Marshal")
	}

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
		return uuid.Nil, apperr.Wrap(execErr, "insert devices.firmware_campaigns")
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
			return uuid.Nil, apperr.Wrap(err, "insert devices.firmware_campaign_devices")
		}
	}

	return campaignID, nil
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
	case actionPause, actionResume:
	case actionAbort:
		if in.Reason == nil || utf8.RuneCountInString(strings.TrimSpace(*in.Reason)) < 1 ||
			utf8.RuneCountInString(strings.TrimSpace(*in.Reason)) > 1000 {
			fieldErrs["reason"] = apperr.KeyLength
		}
	case actionRetryDevice:
		if in.DeviceID == nil {
			fieldErrs["deviceId"] = apperr.KeyRequired
		}
	default:
		fieldErrs[fieldAction] = apperr.KeyInvalid
	}

	return fieldErrs
}

// transitions is the state each control action leads to from each state it is allowed in.
func transitions() map[string]map[string]string {
	return map[string]map[string]string{
		actionPause:  {stateRunning: statePaused, stateScheduled: statePaused},
		actionResume: {statePaused: stateRunning},
		actionAbort: {
			stateScheduled: stateAborted,
			stateRunning:   stateAborted,
			statePaused:    stateAborted,
		},
	}
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
func campaignsControl(ctx context.Context, call *ops.Call, input *ControlInput) (Campaign, error) {
	var (
		state   string
		version int
		start   time.Time
	)

	err := call.Tx.QueryRow(ctx, `SELECT state, version, start_at FROM devices.firmware_campaigns WHERE id = $1 FOR
		UPDATE`, input.CampaignID).
		Scan(&state, &version, &start)
	if errors.Is(err, pgx.ErrNoRows) {
		return Campaign{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return Campaign{}, apperr.Wrap(err, "select devices.firmware_campaigns")
	}

	if version != *call.ExpectedVersion {
		return Campaign{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	next, reason, err := campaignTransition(ctx, call, input, state, start)
	if err != nil {
		return Campaign{}, err
	}

	scanErr := call.Tx.QueryRow(ctx, `UPDATE devices.firmware_campaigns SET state = $2, reason = NULLIF(COALESCE($3,
		reason),
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

	call.Emit(
		ops.Event{
			AggregateType: entityCampaign,
			AggregateID:   input.CampaignID,
			Type:          "FirmwareCampaignChanged",
			Payload:       map[string]string{fieldAction: input.Action, fieldState: next},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "firmwareCampaigns.control",
			TargetKind:      entityCampaign,
			TargetID:        input.CampaignID.String(),
			PreviousVersion: call.ExpectedVersion,
			NextVersion:     &version,
			Reason:          auditReason, Result: "", Before: nil, After: nil,
		},
	)

	return campaignDetail(ctx, call, input.CampaignID)
}

// campaignTransition applies a control action to a campaign in state. retry_device returns one failed or skipped
// device of a running or paused campaign to pending and keeps the state; the other actions move the state
// (transitions). It returns the next state and the reason to store (nil keeps the stored one).
func campaignTransition(
	ctx context.Context,
	call *ops.Call,
	input *ControlInput,
	state string,
	start time.Time,
) (string, *string, error) {
	if input.Action == actionRetryDevice {
		return state, nil, retryDevice(ctx, call, input, state)
	}

	next, allowed := transitions()[input.Action][state]
	if !allowed {
		return "", nil, apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	var reason *string

	if input.Action == actionResume && call.Now.Before(start) {
		next = stateScheduled // resuming before the start time returns to scheduled
	}

	if input.Action == actionResume {
		cleared := ""
		reason = &cleared // the runner's pause reason ends with the pause (IR339)
	}

	if input.Action == actionAbort {
		trimmed := strings.TrimSpace(*input.Reason)
		reason = &trimmed
	}

	return next, reason, nil
}

// retryDevice returns a failed or skipped device of a running or paused campaign to pending.
func retryDevice(ctx context.Context, call *ops.Call, input *ControlInput, state string) error {
	if state != stateRunning && state != statePaused {
		return apperr.E(apperr.Conflict, apperr.KeyInvalidState)
	}

	tag, err := call.Tx.Exec(
		ctx,
		`UPDATE devices.firmware_campaign_devices SET result = 'pending', detail = NULL, operation_id = NULL, result_at =
			NULL
		WHERE campaign_id = $1 AND device_id = $2 AND result IN ('failed','skipped')`,
		input.CampaignID,
		*input.DeviceID,
	)
	if err != nil {
		return apperr.Wrap(err, "update devices.firmware_campaign_devices")
	}

	if tag.RowsAffected() == 0 {
		return apperr.E(apperr.Conflict, "error.retryOnlyFailedOrSkipped")
	}

	return nil
}

// CampaignDeleteInput is firmwareCampaigns.delete input.
type CampaignDeleteInput struct {
	ID     uuid.UUID `json:"id"`
	Reason string    `json:"reason"`
}

// Validate implements ops.Validator.
func (in *CampaignDeleteInput) Validate() map[string]string {
	fieldErrs := map[string]string{}
	if in.ID == uuid.Nil {
		fieldErrs["id"] = apperr.KeyRequired
	}

	reasonErrors(&in.Reason, fieldErrs)

	return fieldErrs
}

// campaignsDelete deletes a campaign that has ended, aborted or completed, with its device rows (DEC-103); one that is
// scheduled, running or paused is CONFLICT error.campaignActive. The devices stay and so do the install operations in
// their history.
//
//	@Summary		firmwareCampaigns.delete (write)
//	@ID				firmwareCampaigns.delete
//	@Description	Authorization: admin:device.write
//	@Description	Validation: D01; input constraints in the corresponding DD; expectedVersion required for updates; \
//	@Description	IR653 only a campaign that has ended (aborted or completed; CONFLICT error.campaignActive); deletes \
//	@Description	its device rows
//	@Description	Recovery: D04: call writes.getResult with the key, then retry the same intent
//	@Description	Design: DD-A20
//	@Tags			firmwareCampaigns
//	@Accept			json
//	@Produce		json
//	@Param			Idempotency-Key		header		string	true	"D04: the same key replays the stored response; another body for the same key is CONFLICT"
//	@Param			X-Expected-Version	header		integer	true	"all: required (target firmware_campaign, read firmwareCampaigns.get)"
//	@Param			id					path		string	true	"input field id"
//	@Param			reason				query		string	false	"input field reason"
//	@Success		200					{object}	ops.Envelope{data=DeletedResource}
//	@Failure		401					{object}	apperr.DomainError	"UNAUTHENTICATED"
//	@Failure		403					{object}	apperr.DomainError	"FORBIDDEN"
//	@Failure		404					{object}	apperr.DomainError	"NOT_FOUND"
//	@Failure		409					{object}	apperr.DomainError	"CONFLICT / OFFLINE"
//	@Failure		422					{object}	apperr.DomainError	"VALIDATION (fieldErrors)"
//	@Failure		429					{object}	apperr.DomainError	"RATE_LIMITED (retryAfterSeconds)"
//	@Failure		503					{object}	apperr.DomainError	"UNAVAILABLE"
//	@Failure		504					{object}	apperr.DomainError	"TIMEOUT"
//	@Security		BearerAuth
//	@Router			/v1/firmware-campaigns/{id} [delete]
func campaignsDelete(
	ctx context.Context,
	call *ops.Call,
	input *CampaignDeleteInput,
) (DeletedResource, error) {
	var (
		state   string
		version int
	)

	err := call.Tx.QueryRow(
		ctx,
		`SELECT state, version FROM devices.firmware_campaigns WHERE id = $1 FOR UPDATE`,
		input.ID,
	).Scan(&state, &version)
	if errors.Is(err, pgx.ErrNoRows) {
		return DeletedResource{}, apperr.E(apperr.NotFound, apperr.KeyNotFound)
	}

	if err != nil {
		return DeletedResource{}, apperr.Wrap(err, "select devices.firmware_campaigns")
	}

	if call.ExpectedVersion != nil && version != *call.ExpectedVersion {
		return DeletedResource{}, apperr.E(apperr.Conflict, apperr.KeyVersionConflict)
	}

	if state != stateAborted && state != stateCompleted {
		return DeletedResource{}, apperr.E(apperr.Conflict, "error.campaignActive")
	}

	for _, stmt := range []string{
		`DELETE FROM devices.firmware_campaign_devices WHERE campaign_id = $1`,
		`DELETE FROM devices.firmware_campaigns WHERE id = $1`,
	} {
		_, execErr := call.Tx.Exec(ctx, stmt, input.ID)
		if execErr != nil {
			return DeletedResource{}, apperr.Wrap(execErr, "delete devices.firmware_campaigns")
		}
	}

	call.Emit(
		ops.Event{
			AggregateType: entityCampaign,
			AggregateID:   input.ID,
			Type:          "FirmwareCampaignDeleted",
			Payload:       nil,
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "firmwareCampaigns.delete",
			TargetKind:      entityCampaign,
			TargetID:        input.ID.String(),
			PreviousVersion: call.ExpectedVersion,
			Reason:          input.Reason, NextVersion: nil, Result: "", Before: nil, After: nil,
		},
	)

	return DeletedResource{ID: input.ID, Deleted: true}, nil
}

// RegisterCampaigns binds the firmware campaign operations.
func RegisterCampaigns(r *ops.Registry, m *Module) {
	ops.Register(r, "firmwareCampaigns.list", campaignsList)
	ops.Register(r, "firmwareCampaigns.get", campaignsGet)
	ops.Register(r, "firmwareCampaigns.schedule", m.campaignsSchedule)
	ops.Register(r, "firmwareCampaigns.control", campaignsControl)
	ops.Register(r, "firmwareCampaigns.delete", campaignsDelete)
}
