package devices

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/pradita/ac-project/service/api/internal/modules/identity"
	"github.com/pradita/ac-project/service/api/internal/modules/notify"
	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

// The firmware campaign runner (BR-A20, IR339).
const (
	waveSuccessPercent = 95             // the next wave starts once the one before it reaches 95 % success
	noticeLead         = 24 * time.Hour // the customers of the affected units hear of the update 24 h before the start
	mytOffsetSeconds   = 8 * 60 * 60    // Malaysia Time is UTC+8 all year
)

// The reason keys of a device's campaign result (FirmwareCampaignResult.reasonKey).
const (
	keyDeviceBusy       = "errors.device_busy"
	keyDeviceOffline    = "errors.device_offline"
	keyDeviceUnbound    = "errors.device_unbound"
	keyInstallFailed    = "errors.install_failed"
	keyInstallTimeout   = "errors.install_timeout"
	keyOtherModel       = "errors.other_model"
	keyTamperUnresolved = "errors.tamper_unresolved"
)

// localZone is the devices' local time of the install window: Kuala Lumpur (no site has another zone).
func localZone() *time.Location {
	l, err := time.LoadLocation("Asia/Kuala_Lumpur")
	if err != nil {
		return time.FixedZone("MYT", mytOffsetSeconds)
	}

	return l
}

// inWindow reports whether the local clock time of now lies in [start, end) (HH:MM); a window that ends earlier than
// it starts runs over midnight.
func inWindow(now time.Time, start, end string) bool {
	hm := now.In(localZone()).Format("15:04")
	if start < end {
		return hm >= start && hm < end
	}

	return hm >= start || hm < end
}

// outcome maps a finished firmware operation onto the device's campaign result and its reason key. An operation that
// the unit's other work (CONFLICT) or a lost connection (OFFLINE) stopped at its start skips the device; no answer
// within the operation's lifetime (TIMEOUT) or a failure the device reports fails it, and it keeps its old version
// (FR-T11).
func outcome(status, code string) (string, string) {
	switch {
	case status == statusSucceeded:
		return resultSucceeded, ""
	case code == string(apperr.Conflict):
		return resultSkipped, keyDeviceBusy
	case code == string(apperr.Offline):
		return resultSkipped, keyDeviceOffline
	case code == string(apperr.Timeout):
		return resultFailed, keyInstallTimeout
	}

	return resultFailed, keyInstallFailed
}

// waveCount is the number of a wave's devices in each result.
type waveCount map[string]int

func (w waveCount) total() int {
	return w[resultPending] + w[resultInstalling] + w[resultSucceeded] + w[resultFailed] + w[resultSkipped]
}

// passed reports whether the wave lets the next one start: 95 % of its devices succeeded (an empty wave passes).
func (w waveCount) passed() bool { return w[resultSucceeded]*100 >= waveSuccessPercent*w.total() }

// AdvanceCampaigns is the equipment scheduler job of firmware campaigns (BR-A20, IR339). Each tick it
//   - sends the customers' notice of a scheduled campaign 24 h before its start, once;
//   - starts a scheduled campaign at startAt;
//   - moves every running campaign: the results of its finished operations, the auto-pause, the waves that may install
//     now, and the completion.
func (m *Module) AdvanceCampaigns(ctx context.Context, call *ops.Call) (int, error) {
	count, err := m.campaignNotices(ctx, call)
	if err != nil {
		return count, err
	}

	rows, err := call.Tx.Query(
		ctx,
		`UPDATE devices.firmware_campaigns SET state = 'running', version = version + 1, updated_at = $1
		WHERE state = 'scheduled' AND start_at <= $1 RETURNING id, version`,
		call.Now,
	)
	if err != nil {
		return count, apperr.Wrap(err, "update devices.firmware_campaigns")
	}

	type started struct {
		id uuid.UUID
		v  int
	}

	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (started, error) {
		var s started

		return s, r.Scan(&s.id, &s.v)
	})
	if err != nil {
		return count, apperr.Wrap(err, "update devices.firmware_campaigns")
	}

	for _, s := range list {
		campaignChanged(call, s.id, "start", stateRunning, s.v, "")
	}

	count += len(list)

	rows, err = call.Tx.Query(
		ctx,
		`SELECT id FROM devices.firmware_campaigns WHERE state = 'running' ORDER BY start_at, id FOR UPDATE`,
	)
	if err != nil {
		return count, apperr.Wrap(err, "select devices.firmware_campaigns")
	}

	running, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return count, apperr.Wrap(err, "select devices.firmware_campaigns")
	}

	for _, id := range running {
		k, err := m.advanceCampaign(ctx, call, id)
		if count += k; err != nil {
			return count, err
		}
	}

	return count, nil
}

// campaignChanged records a transition the runner made (FirmwareCampaignChanged, audited as the system).
func campaignChanged(
	call *ops.Call,
	campaignID uuid.UUID,
	action, state string,
	version int,
	reason string,
) {
	call.Emit(
		ops.Event{
			AggregateType: entityCampaign,
			AggregateID:   campaignID,
			Type:          "FirmwareCampaignChanged",
			Payload:       map[string]string{fieldAction: action, fieldState: state},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:      "firmwareCampaigns." + action,
			TargetKind:  entityCampaign,
			TargetID:    campaignID.String(),
			NextVersion: &version,
			Reason:      reason, PreviousVersion: nil, Result: "", Before: nil, After: nil,
		},
	)
}

// dueNotice is a scheduled campaign whose customer notice is due, with its start and install window.
type dueNotice struct {
	id                     uuid.UUID
	start                  time.Time
	windowStart, windowEnd string
}

// campaignNotices tells the customers of the affected units 24 h before a campaign starts (BR-A20, DD-A20 step 3):
// one device_operation notification to each client member of each unit's customer, once per campaign. A campaign
// paused before its start sends it when resumed.
func (m *Module) campaignNotices(ctx context.Context, call *ops.Call) (int, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT id, start_at, to_char(window_start_local, 'HH24:MI'), to_char(window_end_local, 'HH24:MI') FROM
			devices.firmware_campaigns
		WHERE state = 'scheduled' AND notice_sent_at IS NULL AND start_at <= $1 ORDER BY start_at, id FOR UPDATE`,
		call.Now.Add(noticeLead),
	)
	if err != nil {
		return 0, apperr.Wrap(err, "select devices.firmware_campaigns")
	}

	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (dueNotice, error) {
		var notice dueNotice

		return notice, row.Scan(&notice.id, &notice.start, &notice.windowStart, &notice.windowEnd)
	})
	if err != nil {
		return 0, apperr.Wrap(err, "select devices.firmware_campaigns")
	}

	for _, campaign := range list {
		err := m.sendNotice(ctx, call, campaign)
		if err != nil {
			return 0, err
		}
	}

	return len(list), nil
}

// sendNotice notifies the customers of the units a campaign affects, marks its notice sent and audits how many
// notifications it sent.
func (m *Module) sendNotice(ctx context.Context, call *ops.Call, campaign dueNotice) error {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT DISTINCT COALESCE(dv.unit_id, dv.target_unit_id) FROM devices.firmware_campaign_devices fd
		JOIN devices.devices dv ON dv.id = fd.device_id WHERE fd.campaign_id = $1`,
		campaign.id,
	)
	if err != nil {
		return apperr.Wrap(err, "select devices.firmware_campaign_devices")
	}

	units, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return apperr.Wrap(err, "select devices.firmware_campaign_devices")
	}

	message := fmt.Sprintf(
		"Firmware update from %s, %s–%s local time. The AC may restart for about 2 minutes.",
		campaign.start.In(localZone()).Format("2 Jan 2006"),
		campaign.windowStart,
		campaign.windowEnd,
	)
	sent := 0

	for _, unitID := range units {
		count, err := m.notifyUnitCustomer(ctx, call, unitID, campaign.start, message)
		if err != nil {
			return err
		}

		sent += count
	}

	_, execErr := call.Tx.Exec(
		ctx,
		`UPDATE devices.firmware_campaigns SET notice_sent_at = $2 WHERE id = $1`,
		campaign.id,
		call.Now,
	)
	if execErr != nil {
		return apperr.Wrap(execErr, "update devices.firmware_campaigns")
	}

	call.Audit(
		ops.AuditEntry{
			Action:     "firmwareCampaigns.notice",
			TargetKind: entityCampaign,
			TargetID:   campaign.id.String(),
			Reason: fmt.Sprintf(
				"%d customer notices",
				sent,
			),
			PreviousVersion: nil,
			NextVersion:     nil,
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)

	return nil
}

// notifyUnitCustomer sends the campaign notice to each client member of the unit's customer (none for a unit
// without one) and returns how many notifications it sent.
func (m *Module) notifyUnitCustomer(
	ctx context.Context,
	call *ops.Call,
	unitID uuid.UUID,
	start time.Time,
	message string,
) (int, error) {
	info, found, err := m.Units.UnitInfo(ctx, call, unitID)
	if err != nil {
		return 0, apperr.Wrap(err, "devices.Units.UnitInfo")
	}

	if !found || info.OrgID == uuid.Nil {
		return 0, nil
	}

	recipients, err := identity.Members(
		ctx,
		call,
		identity.MembersInput{
			Role:           roleClient,
			OrganizationID: &info.OrgID,
			IDs:            nil,
			Permission:     "",
			ClientRole:     "",
		},
	)
	if err != nil {
		return 0, apperr.Wrap(err, "identity.Members")
	}

	for _, recipient := range recipients {
		_, err := (notify.Store{}).Create(ctx, call, notify.New{
			RecipientMembershipID: recipient,
			Type:                  "device_operation",
			TemplateKey:           "device_operation",
			TargetKind:            "unit",
			TargetID:              unitID,
			Severity:              "normal",
			Params: map[string]any{
				"at":        start,
				fieldStatus: stateScheduled,
				"message":   message,
			}, Channel: "", SourceAlertID: nil,
		})
		if err != nil {
			return 0, apperr.Wrap(err, "notify.Store.Create")
		}
	}

	return len(recipients), nil
}

// waveCounts counts the campaign's devices per wave and result.
func waveCounts(
	ctx context.Context,
	call *ops.Call,
	campaignID uuid.UUID,
	waves int,
) ([]waveCount, error) {
	out := make([]waveCount, waves)
	for i := range out {
		out[i] = waveCount{}
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT wave, result, count(*) FROM devices.firmware_campaign_devices WHERE campaign_id = $1 GROUP BY wave, result`,
		campaignID,
	)
	if err != nil {
		return nil, apperr.Wrap(err, "select devices.firmware_campaign_devices")
	}
	defer rows.Close()

	for rows.Next() {
		var (
			wave, count int
			result      string
		)

		err := rows.Scan(&wave, &result, &count)
		if err != nil {
			return nil, apperr.Wrap(err, "select devices.firmware_campaign_devices")
		}

		if wave >= 0 && wave < waves {
			out[wave][result] += count
		}
	}

	return out, apperr.Wrap(rows.Err(), "select devices.firmware_campaign_devices")
}

// runningCampaign is what the runner needs of a running campaign.
type runningCampaign struct {
	waves                  []Wave
	model                  uuid.UUID
	windowStart, windowEnd string
	target                 string
	autoPausePercent       int
}

// loadRunningCampaign reads the waves, model, install window, target version and auto-pause threshold of a campaign.
func loadRunningCampaign(
	ctx context.Context,
	call *ops.Call,
	campaignID uuid.UUID,
) (runningCampaign, error) {
	var (
		campaign runningCampaign
		raw      []byte
	)

	err := call.Tx.QueryRow(ctx, `SELECT waves, model_id, to_char(window_start_local, 'HH24:MI'), to_char(window_end_local,
		'HH24:MI'), target_version, auto_pause_failure_percent
		FROM devices.firmware_campaigns WHERE id = $1`, campaignID).
		Scan(
			&raw,
			&campaign.model,
			&campaign.windowStart,
			&campaign.windowEnd,
			&campaign.target,
			&campaign.autoPausePercent,
		)
	if err != nil {
		return campaign, apperr.Wrap(err, "select devices.firmware_campaigns")
	}

	err = json.Unmarshal(raw, &campaign.waves)
	if err != nil {
		return campaign, apperr.Wrap(err, "json.Unmarshal")
	}

	return campaign, nil
}

// advanceCampaign moves one running campaign:
//   - the devices whose operation finished get their result (outcome);
//   - a wave that a new failure takes above autoPauseFailurePercent pauses the campaign;
//   - inside the install window, the pending devices of each wave whose earlier waves passed start installing;
//   - once every wave passed and no device is pending or installing, the campaign is completed.
func (m *Module) advanceCampaign(
	ctx context.Context,
	call *ops.Call,
	campaignID uuid.UUID,
) (int, error) {
	campaign, err := loadRunningCampaign(ctx, call, campaignID)
	if err != nil {
		return 0, err
	}

	failedNow, count, err := recordInstallResults(ctx, call, campaignID)
	if err != nil {
		return 0, err
	}

	counts, err := waveCounts(ctx, call, campaignID, len(campaign.waves))
	if err != nil {
		return count, err
	}

	paused, err := autoPauseCampaign(
		ctx,
		call,
		campaignID,
		counts,
		failedNow,
		campaign.autoPausePercent,
	)
	if err != nil {
		return count, err
	}

	if paused {
		return count + 1, nil
	}

	dispatched, done, err := m.installWaves(ctx, call, campaignID, campaign, counts)
	if count += dispatched; err != nil || !done {
		return count, err
	}

	err = completeCampaign(ctx, call, campaignID)
	if err != nil {
		return count, err
	}

	return count + 1, nil
}

// finishedInstall is a campaign device whose firmware operation finished.
type finishedInstall struct {
	device       uuid.UUID
	wave         int
	status, code string
	at           time.Time
}

// recordInstallResults gives the installing devices whose operation finished their result (outcome). It returns the
// waves that got a new failure and how many devices it moved.
func recordInstallResults(
	ctx context.Context,
	call *ops.Call,
	campaignID uuid.UUID,
) (map[int]bool, int, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT d.device_id, d.wave, o.status, COALESCE(o.failure_code, ''), COALESCE(o.finished_at, o.updated_at)
		FROM devices.firmware_campaign_devices d JOIN devices.device_operations o ON o.id = d.operation_id
		WHERE d.campaign_id = $1 AND d.result = 'installing' AND o.status IN ('succeeded', 'failed')`,
		campaignID,
	)
	if err != nil {
		return nil, 0, apperr.Wrap(err, "select devices.firmware_campaign_devices")
	}

	done, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (finishedInstall, error) {
		var install finishedInstall

		return install, row.Scan(
			&install.device,
			&install.wave,
			&install.status,
			&install.code,
			&install.at,
		)
	})
	if err != nil {
		return nil, 0, apperr.Wrap(err, "select devices.firmware_campaign_devices")
	}

	failedNow := map[int]bool{}

	for _, install := range done {
		result, key := outcome(install.status, install.code)

		_, err := call.Tx.Exec(
			ctx,
			`UPDATE devices.firmware_campaign_devices SET result = $3, detail = NULLIF($4, ''), result_at = $5 WHERE campaign_id
				= $1 AND device_id = $2`,
			campaignID,
			install.device,
			result,
			key,
			install.at,
		)
		if err != nil {
			return nil, 0, apperr.Wrap(err, "update devices.firmware_campaign_devices")
		}

		if result == resultFailed {
			failedNow[install.wave] = true
		}
	}

	return failedNow, len(done), nil
}

// autoPauseCampaign pauses the campaign when a wave that got a new failure has more than autoPausePercent of its
// devices failed, and reports whether it did.
func autoPauseCampaign(
	ctx context.Context,
	call *ops.Call,
	campaignID uuid.UUID,
	counts []waveCount,
	failedNow map[int]bool,
	autoPausePercent int,
) (bool, error) {
	for wave := range counts {
		if !failedNow[wave] ||
			counts[wave][resultFailed]*100 <= autoPausePercent*counts[wave].total() {
			continue
		}

		reason := fmt.Sprintf(
			"Auto-paused: %d of %d devices failed in wave %d (more than %d %%).",
			counts[wave][resultFailed],
			counts[wave].total(),
			wave+1,
			autoPausePercent,
		)

		var version int

		err := call.Tx.QueryRow(ctx, `UPDATE devices.firmware_campaigns SET state = 'paused', reason = $2, version = version
				+
				1, updated_at = $3 WHERE id = $1 RETURNING version`,
			campaignID, reason, call.Now).
			Scan(&version)
		if err != nil {
			return false, apperr.Wrap(err, "update devices.firmware_campaigns")
		}

		campaignChanged(call, campaignID, "autoPause", statePaused, version, reason)

		return true, nil
	}

	return false, nil
}

// installWaves starts, inside the install window, the pending devices of each wave whose earlier waves passed. It
// returns how many devices it moved and whether every wave passed with no device pending or installing.
func (m *Module) installWaves(
	ctx context.Context,
	call *ops.Call,
	campaignID uuid.UUID,
	campaign runningCampaign,
	counts []waveCount,
) (int, bool, error) {
	count := 0

	open := inWindow(call.Now, campaign.windowStart, campaign.windowEnd)
	for wave := range campaign.waves {
		if open && counts[wave][resultPending] > 0 {
			dispatched, err := m.dispatchWave(
				ctx,
				call,
				campaignID,
				wave,
				campaign.model,
				campaign.target,
			)
			if count += dispatched; err != nil {
				return count, false, err
			}

			counts, err = waveCounts(ctx, call, campaignID, len(campaign.waves))
			if err != nil {
				return count, false, err
			}
		}

		if !counts[wave].passed() {
			return count, false, nil // the later waves wait for this one
		}
	}

	for _, tally := range counts {
		if tally[resultPending] > 0 || tally[resultInstalling] > 0 {
			return count, false, nil
		}
	}

	return count, true, nil
}

// completeCampaign completes a campaign whose waves all passed.
func completeCampaign(ctx context.Context, call *ops.Call, campaignID uuid.UUID) error {
	var version int

	scanErr := call.Tx.QueryRow(ctx, `UPDATE devices.firmware_campaigns SET state = 'completed', version = version + 1,
		updated_at = $2 WHERE id = $1 RETURNING version`,
		campaignID, call.Now).
		Scan(&version)
	if scanErr != nil {
		return apperr.Wrap(scanErr, "update devices.firmware_campaigns")
	}

	campaignChanged(call, campaignID, "complete", stateCompleted, version, "")

	return nil
}

// pendingDevice is a pending device of a wave with what decides its dispatch.
type pendingDevice struct {
	id               uuid.UUID
	unit             *uuid.UUID
	conn, tamper, fw string
}

// dispatchWave starts the firmware update of the wave's pending devices. A device that left its unit or the
// campaign's model, has an open tamper event, is not online, still has an unfinished operation, or whose unit is busy
// (D05: an open operation, a command in flight, a diagnostic run or a restriction) is skipped with the reason; a device
// already on the target version succeeded. The others get a queued firmware operation of the campaign, which IR67
// starts a second later.
func (m *Module) dispatchWave(
	ctx context.Context,
	call *ops.Call,
	campaignID uuid.UUID,
	wave int,
	model uuid.UUID,
	target string,
) (int, error) {
	rows, err := call.Tx.Query(
		ctx,
		`SELECT d.device_id, dv.unit_id, dv.connection, dv.tamper, dv.firmware_version FROM devices.firmware_campaign_devices
			d
		JOIN devices.devices dv ON dv.id = d.device_id WHERE d.campaign_id = $1 AND d.wave = $2 AND d.result = 'pending' ORDER
		BY d.device_id FOR UPDATE OF dv`,
		campaignID,
		wave,
	)
	if err != nil {
		return 0, apperr.Wrap(err, "select devices.firmware_campaign_devices")
	}

	list, err := pgx.CollectRows(rows, func(row pgx.CollectableRow) (pendingDevice, error) {
		var device pendingDevice

		return device, row.Scan(&device.id, &device.unit, &device.conn, &device.tamper, &device.fw)
	})
	if err != nil {
		return 0, apperr.Wrap(err, "select devices.firmware_campaign_devices")
	}

	for _, device := range list {
		err := m.dispatchDevice(ctx, call, campaignID, device, model, target)
		if err != nil {
			return 0, err
		}
	}

	return len(list), nil
}

// dispatchDevice settles one pending device of a wave: on the target version already it succeeded, with a skip reason
// it is skipped, otherwise it gets a queued firmware operation of the campaign.
func (m *Module) dispatchDevice(
	ctx context.Context,
	call *ops.Call,
	campaignID uuid.UUID,
	device pendingDevice,
	model uuid.UUID,
	target string,
) error {
	if device.unit != nil && device.fw == target {
		return setCampaignDevice(ctx, call, campaignID, device.id, resultSucceeded, "", nil)
	}

	key, err := m.skipReason(ctx, call, device, model)
	if err != nil {
		return err
	}

	if key != "" {
		return setCampaignDevice(ctx, call, campaignID, device.id, resultSkipped, key, nil)
	}

	operationID := uuid.Must(uuid.NewV7())

	_, err = call.Tx.Exec(
		ctx,
		`INSERT INTO devices.device_operations (id, tenant_id, device_id, unit_id, kind, status, target_version, campaign_id,
			expires_at)
		VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, 'firmware', 'queued', $4, $5, $6)`,
		operationID,
		device.id,
		*device.unit,
		target,
		campaignID,
		call.Now.Add(operationTTL),
	)
	if err != nil {
		return apperr.Wrap(err, "insert devices.device_operations")
	}

	err = setCampaignDevice(ctx, call, campaignID, device.id, resultInstalling, "", &operationID)
	if err != nil {
		return err
	}

	installRequested(call, campaignID, device.id, operationID)

	return nil
}

// installRequested emits DeviceOperationRequested of a campaign's firmware operation and audits the install.
func installRequested(call *ops.Call, campaignID, device, operationID uuid.UUID) {
	call.Emit(
		ops.Event{
			AggregateType: entityDevice,
			AggregateID:   device,
			Type:          "DeviceOperationRequested",
			Payload: map[string]any{
				fieldOperationID: operationID,
				"kind":           kindFirmware,
				"campaignId":     campaignID,
			},
		},
	)
	call.Audit(
		ops.AuditEntry{
			Action:          "firmwareCampaigns.install",
			TargetKind:      entityDevice,
			TargetID:        device.String(),
			Reason:          "campaign " + campaignID.String(),
			PreviousVersion: nil,
			NextVersion:     nil,
			Result:          "",
			Before:          nil,
			After:           nil,
		},
	)
}

// skipReason is the reason key that skips a pending device not yet on the target version ("" when it may install):
// it left its unit, has an open tamper event or is not online, else the reason its unit gives (unitSkipReason).
func (m *Module) skipReason(
	ctx context.Context,
	call *ops.Call,
	device pendingDevice,
	model uuid.UUID,
) (string, error) {
	switch {
	case device.unit == nil:
		return keyDeviceUnbound, nil
	case device.tamper == tamperDetected:
		return keyTamperUnresolved, nil
	case device.conn != connOnline:
		return keyDeviceOffline, nil
	}

	return m.unitSkipReason(ctx, call, device, model)
}

// unitSkipReason is the skip reason of a bound, online device without tamper: its unit has another model, it still has
// an unfinished operation, or its unit is busy (D05).
func (m *Module) unitSkipReason(
	ctx context.Context,
	call *ops.Call,
	device pendingDevice,
	model uuid.UUID,
) (string, error) {
	unitInfo, found, err := m.Units.UnitInfo(ctx, call, *device.unit)
	if err != nil {
		return "", apperr.Wrap(err, "devices.Units.UnitInfo")
	}

	var open bool // an operation of the device not finished yet, expired or not (one firmware operation per device)

	scanErr := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.device_operations WHERE device_id = $1 AND
		status
		IN ('queued','running'))`, device.id).
		Scan(&open)
	if scanErr != nil {
		return "", apperr.Wrap(scanErr, "select devices.device_operations")
	}

	switch {
	case !found || unitInfo.ModelID != model:
		return keyOtherModel, nil
	case open:
		return keyDeviceBusy, nil
	}

	err = m.exclusive(ctx, call, *device.unit)
	if err == nil {
		return "", nil
	}

	var de *apperr.DomainError
	if !errors.As(err, &de) || de.Code != apperr.Conflict {
		return "", err
	}

	return keyDeviceBusy, nil
}

// setCampaignDevice records the result of a campaign's device, with its reason key and operation.
func setCampaignDevice(
	ctx context.Context,
	call *ops.Call,
	campaignID, device uuid.UUID,
	result, key string,
	operationID *uuid.UUID,
) error {
	_, err := call.Tx.Exec(
		ctx,
		`UPDATE devices.firmware_campaign_devices SET result = $3, detail = NULLIF($4, ''), operation_id = $5, result_at = $6
		WHERE campaign_id = $1 AND device_id = $2`,
		campaignID,
		device,
		result,
		key,
		operationID,
		call.Now,
	)

	return apperr.Wrap(err, "update devices.firmware_campaign_devices")
}
