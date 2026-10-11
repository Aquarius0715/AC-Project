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
func (m *Module) AdvanceCampaigns(ctx context.Context, c *ops.Call) (int, error) {
	n, err := m.campaignNotices(ctx, c)
	if err != nil {
		return n, err
	}
	rows, err := c.Tx.Query(ctx, `UPDATE devices.firmware_campaigns SET state = 'running', version = version + 1, updated_at = $1
		WHERE state = 'scheduled' AND start_at <= $1 RETURNING id, version`, c.Now)
	if err != nil {
		return n, err
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
		return n, err
	}
	for _, s := range list {
		campaignChanged(c, s.id, "start", "running", s.v, "")
	}
	n += len(list)
	rows, err = c.Tx.Query(ctx, `SELECT id FROM devices.firmware_campaigns WHERE state = 'running' ORDER BY start_at, id FOR UPDATE`)
	if err != nil {
		return n, err
	}
	running, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
	if err != nil {
		return n, err
	}
	for _, id := range running {
		k, err := m.advanceCampaign(ctx, c, id)
		if n += k; err != nil {
			return n, err
		}
	}
	return n, nil
}

// campaignChanged records a transition the runner made (FirmwareCampaignChanged, audited as the system).
func campaignChanged(c *ops.Call, id uuid.UUID, action, state string, v int, reason string) {
	c.Emit(ops.Event{AggregateType: "firmware_campaign", AggregateID: id, Type: "FirmwareCampaignChanged", Payload: map[string]string{"action": action, "state": state}})
	c.Audit(ops.AuditEntry{Action: "firmwareCampaigns." + action, TargetKind: "firmware_campaign", TargetID: id.String(), NextVersion: &v, Reason: reason})
}

// campaignNotices tells the customers of the affected units 24 h before a campaign starts (BR-A20, DD-A20 step 3):
// one device_operation notification to each client member of each unit's customer, once per campaign. A campaign
// paused before its start sends it when resumed.
func (m *Module) campaignNotices(ctx context.Context, c *ops.Call) (int, error) {
	rows, err := c.Tx.Query(ctx, `SELECT id, start_at, to_char(window_start_local, 'HH24:MI'), to_char(window_end_local, 'HH24:MI') FROM devices.firmware_campaigns
		WHERE state = 'scheduled' AND notice_sent_at IS NULL AND start_at <= $1 ORDER BY start_at, id FOR UPDATE`, c.Now.Add(noticeLead))
	if err != nil {
		return 0, err
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
		return 0, err
	}
	for _, d := range list {
		rows, err := c.Tx.Query(ctx, `SELECT DISTINCT COALESCE(dv.unit_id, dv.target_unit_id) FROM devices.firmware_campaign_devices fd
			JOIN devices.devices dv ON dv.id = fd.device_id WHERE fd.campaign_id = $1`, d.id)
		if err != nil {
			return 0, err
		}
		units, err := pgx.CollectRows(rows, pgx.RowTo[uuid.UUID])
		if err != nil {
			return 0, err
		}
		message := fmt.Sprintf("Firmware update from %s, %s–%s local time. The AC may restart for about 2 minutes.", d.start.In(localZone()).Format("2 Jan 2006"), d.from, d.to)
		sent := 0
		for _, u := range units {
			info, ok, err := m.Units.UnitInfo(ctx, c, u)
			if err != nil {
				return 0, err
			}
			if !ok || info.OrgID == uuid.Nil {
				continue
			}
			recipients, err := identity.Members(ctx, c, identity.MembersInput{Role: "client", OrganizationID: &info.OrgID})
			if err != nil {
				return 0, err
			}
			for _, r := range recipients {
				if _, err := (notify.Store{}).Create(ctx, c, notify.New{RecipientMembershipID: r, Type: "device_operation", TemplateKey: "device_operation", TargetKind: "unit",
					TargetID: u, Severity: "normal", Params: map[string]any{"at": d.start, "status": "scheduled", "message": message}}); err != nil {
					return 0, err
				}
				sent++
			}
		}
		if _, err := c.Tx.Exec(ctx, `UPDATE devices.firmware_campaigns SET notice_sent_at = $2 WHERE id = $1`, d.id, c.Now); err != nil {
			return 0, err
		}
		c.Audit(ops.AuditEntry{Action: "firmwareCampaigns.notice", TargetKind: "firmware_campaign", TargetID: d.id.String(), Reason: fmt.Sprintf("%d customer notices", sent)})
	}
	return len(list), nil
}

// waveCounts counts the campaign's devices per wave and result.
func waveCounts(ctx context.Context, c *ops.Call, id uuid.UUID, waves int) ([]waveCount, error) {
	out := make([]waveCount, waves)
	for i := range out {
		out[i] = waveCount{}
	}
	rows, err := c.Tx.Query(ctx, `SELECT wave, result, count(*) FROM devices.firmware_campaign_devices WHERE campaign_id = $1 GROUP BY wave, result`, id)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var w, k int
		var result string
		if err := rows.Scan(&w, &result, &k); err != nil {
			return nil, err
		}
		if w >= 0 && w < waves {
			out[w][result] += k
		}
	}
	return out, rows.Err()
}

// advanceCampaign moves one running campaign:
//   - the devices whose operation finished get their result (outcome);
//   - a wave that a new failure takes above autoPauseFailurePercent pauses the campaign;
//   - inside the install window, the pending devices of each wave whose earlier waves passed start installing;
//   - once every wave passed and no device is pending or installing, the campaign is completed.
func (m *Module) advanceCampaign(ctx context.Context, c *ops.Call, id uuid.UUID) (int, error) {
	var raw []byte
	var model uuid.UUID
	var from, to, target string
	var autoPause int
	if err := c.Tx.QueryRow(ctx, `SELECT waves, model_id, to_char(window_start_local, 'HH24:MI'), to_char(window_end_local, 'HH24:MI'), target_version, auto_pause_failure_percent
		FROM devices.firmware_campaigns WHERE id = $1`, id).Scan(&raw, &model, &from, &to, &target, &autoPause); err != nil {
		return 0, err
	}
	var waves []Wave
	if err := json.Unmarshal(raw, &waves); err != nil {
		return 0, err
	}
	rows, err := c.Tx.Query(ctx, `SELECT d.device_id, d.wave, o.status, COALESCE(o.failure_code, ''), COALESCE(o.finished_at, o.updated_at)
		FROM devices.firmware_campaign_devices d JOIN devices.device_operations o ON o.id = d.operation_id
		WHERE d.campaign_id = $1 AND d.result = 'installing' AND o.status IN ('succeeded', 'failed')`, id)
	if err != nil {
		return 0, err
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
		return 0, err
	}
	failedNow := map[int]bool{}
	for _, f := range done {
		result, key := outcome(f.status, f.code)
		if _, err := c.Tx.Exec(ctx, `UPDATE devices.firmware_campaign_devices SET result = $3, detail = NULLIF($4, ''), result_at = $5 WHERE campaign_id = $1 AND device_id = $2`,
			id, f.device, result, key, f.at); err != nil {
			return 0, err
		}
		if result == "failed" {
			failedNow[f.wave] = true
		}
	}
	n := len(done)
	counts, err := waveCounts(ctx, c, id, len(waves))
	if err != nil {
		return n, err
	}
	for w := range waves {
		if failedNow[w] && counts[w]["failed"]*100 > autoPause*counts[w].total() {
			reason := fmt.Sprintf("Auto-paused: %d of %d devices failed in wave %d (more than %d %%).", counts[w]["failed"], counts[w].total(), w+1, autoPause)
			var v int
			if err := c.Tx.QueryRow(ctx, `UPDATE devices.firmware_campaigns SET state = 'paused', reason = $2, version = version + 1, updated_at = $3 WHERE id = $1 RETURNING version`,
				id, reason, c.Now).Scan(&v); err != nil {
				return n, err
			}
			campaignChanged(c, id, "autoPause", "paused", v, reason)
			return n + 1, nil
		}
	}
	open := inWindow(c.Now, from, to)
	for w := range waves {
		if open && counts[w]["pending"] > 0 {
			k, err := m.dispatchWave(ctx, c, id, w, model, target)
			if n += k; err != nil {
				return n, err
			}
			if counts, err = waveCounts(ctx, c, id, len(waves)); err != nil {
				return n, err
			}
		}
		if !counts[w].passed() {
			return n, nil // the later waves wait for this one
		}
	}
	for _, w := range counts {
		if w["pending"] > 0 || w["installing"] > 0 {
			return n, nil
		}
	}
	var v int
	if err := c.Tx.QueryRow(ctx, `UPDATE devices.firmware_campaigns SET state = 'completed', version = version + 1, updated_at = $2 WHERE id = $1 RETURNING version`,
		id, c.Now).Scan(&v); err != nil {
		return n, err
	}
	campaignChanged(c, id, "complete", "completed", v, "")
	return n + 1, nil
}

// dispatchWave starts the firmware update of the wave's pending devices. A device that left its unit or the
// campaign's model, has an open tamper event, is not online, still has an unfinished operation, or whose unit is busy
// (D05: an open operation, a command in flight, a diagnostic run or a restriction) is skipped with the reason; a device
// already on the target version succeeded. The others get a queued firmware operation of the campaign, which IR67
// starts a second later.
func (m *Module) dispatchWave(ctx context.Context, c *ops.Call, id uuid.UUID, wave int, model uuid.UUID, target string) (int, error) {
	rows, err := c.Tx.Query(ctx, `SELECT d.device_id, dv.unit_id, dv.connection, dv.tamper, dv.firmware_version FROM devices.firmware_campaign_devices d
		JOIN devices.devices dv ON dv.id = d.device_id WHERE d.campaign_id = $1 AND d.wave = $2 AND d.result = 'pending' ORDER BY d.device_id FOR UPDATE OF dv`, id, wave)
	if err != nil {
		return 0, err
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
		return 0, err
	}
	set := func(device uuid.UUID, result, key string, op *uuid.UUID) error {
		_, err := c.Tx.Exec(ctx, `UPDATE devices.firmware_campaign_devices SET result = $3, detail = NULLIF($4, ''), operation_id = $5, result_at = $6
			WHERE campaign_id = $1 AND device_id = $2`, id, device, result, key, op, c.Now)
		return err
	}
	for _, p := range list {
		key := ""
		switch {
		case p.unit == nil:
			key = "errors.device_unbound"
		case p.fw == target:
			if err := set(p.id, "succeeded", "", nil); err != nil {
				return 0, err
			}
			continue
		case p.tamper == "detected":
			key = "errors.tamper_unresolved"
		case p.conn != "online":
			key = "errors.device_offline"
		}
		if key == "" {
			u, ok, err := m.Units.UnitInfo(ctx, c, *p.unit)
			if err != nil {
				return 0, err
			}
			var open bool // an operation of the device not finished yet, expired or not (one firmware operation per device)
			if err := c.Tx.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM devices.device_operations WHERE device_id = $1 AND status IN ('queued','running'))`, p.id).Scan(&open); err != nil {
				return 0, err
			}
			if !ok || u.ModelID != model {
				key = "errors.other_model"
			} else if open {
				key = "errors.device_busy"
			} else if err := m.exclusive(ctx, c, *p.unit); err != nil {
				var de *apperr.DomainError
				if !errors.As(err, &de) || de.Code != apperr.Conflict {
					return 0, err
				}
				key = "errors.device_busy"
			}
		}
		if key != "" {
			if err := set(p.id, "skipped", key, nil); err != nil {
				return 0, err
			}
			continue
		}
		op := uuid.Must(uuid.NewV7())
		if _, err := c.Tx.Exec(ctx, `INSERT INTO devices.device_operations (id, tenant_id, device_id, unit_id, kind, status, target_version, campaign_id, expires_at)
			VALUES ($1, current_setting('app.tenant_id')::uuid, $2, $3, 'firmware', 'queued', $4, $5, $6)`, op, p.id, *p.unit, target, id, c.Now.Add(operationTTL)); err != nil {
			return 0, err
		}
		if err := set(p.id, "installing", "", &op); err != nil {
			return 0, err
		}
		c.Emit(ops.Event{AggregateType: "device", AggregateID: p.id, Type: "DeviceOperationRequested", Payload: map[string]any{"operationId": op, "kind": "firmware", "campaignId": id}})
		c.Audit(ops.AuditEntry{Action: "firmwareCampaigns.install", TargetKind: "device", TargetID: p.id.String(), Reason: "campaign " + id.String()})
	}
	return len(list), nil
}
