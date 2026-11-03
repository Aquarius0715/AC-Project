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
)

// localZone is the devices' local time of the install window: Kuala Lumpur (no site has another zone).
func localZone() *time.Location {
	l, err := time.LoadLocation("Asia/Kuala_Lumpur")
	if err != nil {
		return time.FixedZone("MYT", 8*3600)
	}

	return l
}

// inWindow reports whether the local clock time of now lies in [from, to) (HH:MM); a window that ends earlier than
// it starts runs over midnight.
func inWindow(now time.Time, from, to string) bool {
	hm := now.In(localZone()).Format("15:04")
	if from < to {
		return hm >= from && hm < to
	}

	return hm >= from || hm < to
}

// outcome maps a finished firmware operation onto the device's campaign result. An operation that the unit's other
// work (CONFLICT) or a lost connection (OFFLINE) stopped at its start skips the device; no answer within the
// operation's lifetime (TIMEOUT) or a failure the device reports fails it, and it keeps its old version (FR-T11).
func outcome(status, code string) (result, key string) {
	switch {
	case status == "succeeded":
		return "succeeded", ""
	case code == string(apperr.Conflict):
		return "skipped", "errors.device_busy"
	case code == string(apperr.Offline):
		return "skipped", "errors.device_offline"
	case code == string(apperr.Timeout):
		return "failed", "errors.install_timeout"
	}

	return "failed", "errors.install_failed"
}

// waveCount is the number of a wave's devices in each result.
type waveCount map[string]int

func (w waveCount) total() int {
	return w["pending"] + w["installing"] + w["succeeded"] + w["failed"] + w["skipped"]
}

// passed reports whether the wave lets the next one start: 95 % of its devices succeeded (an empty wave passes).
func (w waveCount) passed() bool { return w["succeeded"]*100 >= waveSuccessPercent*w.total() }

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
		campaignChanged(call, s.id, "start", "running", s.v, "")
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
	c *ops.Call,
	campaignID uuid.UUID,
	action, state string,
	v int,
	reason string,
) {
	c.Emit(
		ops.Event{
			AggregateType: "firmware_campaign",
			AggregateID:   campaignID,
			Type:          "FirmwareCampaignChanged",
			Payload:       map[string]string{"action": action, "state": state},
		},
	)
	c.Audit(
		ops.AuditEntry{
			Action:      "firmwareCampaigns." + action,
			TargetKind:  "firmware_campaign",
			TargetID:    campaignID.String(),
			NextVersion: &v,
			Reason:      reason, PreviousVersion: nil, Result: "", Before: nil, After: nil,
		},
	)
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

	type due struct {
		id       uuid.UUID
		start    time.Time
		from, to string
	}

	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (due, error) {
		var d due

		return d, r.Scan(&d.id, &d.start, &d.from, &d.to)
	})
	if err != nil {
		return 0, apperr.Wrap(err, "select devices.firmware_campaigns")
	}

	for _, campaign := range list {
		rows, err := call.Tx.Query(
			ctx,
			`SELECT DISTINCT COALESCE(dv.unit_id, dv.target_unit_id) FROM devices.firmware_campaign_devices fd
			JOIN devices.devices dv ON dv.id = fd.device_id WHERE fd.campaign_id = $1`,
			campaign.id,
		)
		if err != nil {
			return 0, apperr.Wrap(err, "select devices.firmware_campaign_devices")
		}

		units, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return 0, apperr.Wrap(err, "select devices.firmware_campaign_devices")
		}

		message := fmt.Sprintf(
			"Firmware update from %s, %s–%s local time. The AC may restart for about 2 minutes.",
			campaign.start.In(localZone()).Format("2 Jan 2006"),
			campaign.from,
			campaign.to,
		)
		sent := 0

		for _, unitID := range units {
			info, ok, err := m.Units.UnitInfo(ctx, call, unitID)
			if err != nil {
				return 0, apperr.Wrap(err, "devices.Units.UnitInfo")
			}

			if !ok || info.OrgID == uuid.Nil {
				continue
			}

			recipients, err := identity.Members(
				ctx,
				call,
				identity.MembersInput{
					Role:           "client",
					OrganizationID: &info.OrgID,
					IDs:            nil,
					Permission:     "",
					ClientRole:     "",
				},
			)
			if err != nil {
				return 0, apperr.Wrap(err, "identity.Members")
			}

			for _, r := range recipients {
				_, err := (notify.Store{}).Create(ctx, call, notify.New{
					RecipientMembershipID: r,
					Type:                  "device_operation",
					TemplateKey:           "device_operation",
					TargetKind:            "unit",
					TargetID:              unitID,
					Severity:              "normal",
					Params: map[string]any{
						"at":      campaign.start,
						"status":  "scheduled",
						"message": message,
					}, Channel: "", SourceAlertID: nil,
				})
				if err != nil {
					return 0, apperr.Wrap(err, "notify.Store.Create")
				}

				sent++
			}
		}

		_, execErr := call.Tx.Exec(
			ctx,
			`UPDATE devices.firmware_campaigns SET notice_sent_at = $2 WHERE id = $1`,
			campaign.id,
			call.Now,
		)
		if execErr != nil {
			return 0, apperr.Wrap(execErr, "update devices.firmware_campaigns")
		}

		call.Audit(
			ops.AuditEntry{
				Action:     "firmwareCampaigns.notice",
				TargetKind: "firmware_campaign",
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
	}

	return len(list), nil
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
			wave, k int
			result  string
		)

		err := rows.Scan(&wave, &result, &k)
		if err != nil {
			return nil, apperr.Wrap(err, "select devices.firmware_campaign_devices")
		}

		if wave >= 0 && wave < waves {
			out[wave][result] += k
		}
	}

	return out, apperr.Wrap(rows.Err(), "select devices.firmware_campaign_devices")
}

// advanceCampaign moves one running campaign:
//   - the devices whose operation finished get their result (outcome);
//   - a wave that a new failure takes above autoPauseFailurePercent pauses the campaign;
//   - inside the install window, the pending devices of each wave whose earlier waves passed start installing;
//   - once every wave passed and no device is pending or installing, the campaign is completed.
func (m *Module) advanceCampaign(ctx context.Context, call *ops.Call, id uuid.UUID) (int, error) {
	var (
		raw              []byte
		model            uuid.UUID
		from, to, target string
		autoPause        int
	)

	err := call.Tx.QueryRow(ctx, `SELECT waves, model_id, to_char(window_start_local, 'HH24:MI'), to_char(window_end_local,
		'HH24:MI'), target_version, auto_pause_failure_percent
		FROM devices.firmware_campaigns WHERE id = $1`, id).
		Scan(&raw, &model, &from, &to, &target, &autoPause)
	if err != nil {
		return 0, apperr.Wrap(err, "select devices.firmware_campaigns")
	}

	var waves []Wave

	err = json.Unmarshal(raw, &waves)
	if err != nil {
		return 0, apperr.Wrap(err, "json.Unmarshal")
	}

	rows, err := call.Tx.Query(
		ctx,
		`SELECT d.device_id, d.wave, o.status, COALESCE(o.failure_code, ''), COALESCE(o.finished_at, o.updated_at)
		FROM devices.firmware_campaign_devices d JOIN devices.device_operations o ON o.id = d.operation_id
		WHERE d.campaign_id = $1 AND d.result = 'installing' AND o.status IN ('succeeded', 'failed')`,
		id,
	)
	if err != nil {
		return 0, apperr.Wrap(err, "select devices.firmware_campaign_devices")
	}

	type finished struct {
		device       uuid.UUID
		wave         int
		status, code string
		at           time.Time
	}

	done, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (finished, error) {
		var f finished

		return f, r.Scan(&f.device, &f.wave, &f.status, &f.code, &f.at)
	})
	if err != nil {
		return 0, apperr.Wrap(err, "select devices.firmware_campaign_devices")
	}

	failedNow := map[int]bool{}

	for _, install := range done {
		result, key := outcome(install.status, install.code)

		_, err := call.Tx.Exec(
			ctx,
			`UPDATE devices.firmware_campaign_devices SET result = $3, detail = NULLIF($4, ''), result_at = $5 WHERE campaign_id
				= $1 AND device_id = $2`,
			id,
			install.device,
			result,
			key,
			install.at,
		)
		if err != nil {
			return 0, apperr.Wrap(err, "update devices.firmware_campaign_devices")
		}

		if result == "failed" {
			failedNow[install.wave] = true
		}
	}

	count := len(done)

	counts, err := waveCounts(ctx, call, id, len(waves))
	if err != nil {
		return count, err
	}

	for wave := range waves {
		if failedNow[wave] && counts[wave]["failed"]*100 > autoPause*counts[wave].total() {
			reason := fmt.Sprintf(
				"Auto-paused: %d of %d devices failed in wave %d (more than %d %%).",
				counts[wave]["failed"],
				counts[wave].total(),
				wave+1,
				autoPause,
			)

			var v int

			err := call.Tx.QueryRow(ctx, `UPDATE devices.firmware_campaigns SET state = 'paused', reason = $2, version = version
				+
				1, updated_at = $3 WHERE id = $1 RETURNING version`,
				id, reason, call.Now).
				Scan(&v)
			if err != nil {
				return count, apperr.Wrap(err, "update devices.firmware_campaigns")
			}

			campaignChanged(call, id, "autoPause", "paused", v, reason)

			return count + 1, nil
		}
	}

	open := inWindow(call.Now, from, to)
	for wave := range waves {
		if open && counts[wave]["pending"] > 0 {
			k, err := m.dispatchWave(ctx, call, id, wave, model, target)
			if count += k; err != nil {
				return count, err
			}

			counts, err = waveCounts(ctx, call, id, len(waves))
			if err != nil {
				return count, err
			}
		}

		if !counts[wave].passed() {
			return count, nil // the later waves wait for this one
		}
	}

	for _, w := range counts {
		if w["pending"] > 0 || w["installing"] > 0 {
			return count, nil
		}
	}

	var version int

	scanErr := call.Tx.QueryRow(ctx, `UPDATE devices.firmware_campaigns SET state = 'completed', version = version + 1,
		updated_at = $2 WHERE id = $1 RETURNING version`,
		id, call.Now).
		Scan(&version)
	if scanErr != nil {
		return count, apperr.Wrap(scanErr, "update devices.firmware_campaigns")
	}

	campaignChanged(call, id, "complete", "completed", version, "")

	return count + 1, nil
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

	type pending struct {
		id               uuid.UUID
		unit             *uuid.UUID
		conn, tamper, fw string
	}

	list, err := pgx.CollectRows(rows, func(r pgx.CollectableRow) (pending, error) {
		var p pending

		return p, r.Scan(&p.id, &p.unit, &p.conn, &p.tamper, &p.fw)
	})
	if err != nil {
		return 0, apperr.Wrap(err, "select devices.firmware_campaign_devices")
	}

	set := func(device uuid.UUID, result, key string, operationID *uuid.UUID) error {
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
	for _, device := range list {
		key := ""

		switch {
		case device.unit == nil:
			key = "errors.device_unbound"
		case device.fw == target:
			err := set(device.id, "succeeded", "", nil)
			if err != nil {
				return 0, err
			}

			continue
		case device.tamper == "detected":
			key = "errors.tamper_unresolved"
		case device.conn != "online":
			key = "errors.device_offline"
		}

		if key == "" {
			u, found, err := m.Units.UnitInfo(ctx, call, *device.unit)
			if err != nil {
				return 0, apperr.Wrap(err, "devices.Units.UnitInfo")
			}

			var open bool // an operation of the device not finished yet, expired or not (one firmware operation per device)

			scanErr := call.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.device_operations WHERE device_id = $1 AND
				status
				IN ('queued','running'))`, device.id).
				Scan(&open)
			if scanErr != nil {
				return 0, apperr.Wrap(scanErr, "select devices.device_operations")
			}

			if !found || u.ModelID != model {
				key = "errors.other_model"
			} else if open {
				key = "errors.device_busy"
			} else {
				err := m.exclusive(ctx, call, *device.unit)
				if err != nil {
					var de *apperr.DomainError
					if !errors.As(err, &de) || de.Code != apperr.Conflict {
						return 0, err
					}

					key = "errors.device_busy"
				}
			}
		}

		if key != "" {
			err := set(device.id, "skipped", key, nil)
			if err != nil {
				return 0, err
			}

			continue
		}

		operationID := uuid.Must(uuid.NewV7())

		_, err := call.Tx.Exec(
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
			return 0, apperr.Wrap(err, "insert devices.device_operations")
		}

		err = set(device.id, "installing", "", &operationID)
		if err != nil {
			return 0, err
		}

		call.Emit(
			ops.Event{
				AggregateType: "device",
				AggregateID:   device.id,
				Type:          "DeviceOperationRequested",
				Payload: map[string]any{
					"operationId": operationID,
					"kind":        "firmware",
					"campaignId":  campaignID,
				},
			},
		)
		call.Audit(
			ops.AuditEntry{
				Action:          "firmwareCampaigns.install",
				TargetKind:      "device",
				TargetID:        device.id.String(),
				Reason:          "campaign " + campaignID.String(),
				PreviousVersion: nil,
				NextVersion:     nil,
				Result:          "",
				Before:          nil,
				After:           nil,
			},
		)
	}

	return len(list), nil
}
