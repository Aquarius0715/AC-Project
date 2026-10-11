package integration

import (
	"context"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/seed"
)

// BR-A20 / IR339: the equipment scheduler runs firmware campaigns. The customers hear of the update 24 h before
// the start, once. The campaign starts at startAt. Each wave installs inside the window once the waves before it
// reached 95 % success, and skips devices that are offline, tampered or busy. A new failure above the auto-pause
// share pauses the campaign; after Retry and Resume it goes on. An unanswered install fails with TIMEOUT. Outside the
// window nothing starts. The campaign completes when every wave passed and nothing is pending or installing.
func TestFirmwareCampaignRunner(t *testing.T) {
	s := serverWith(t, true)
	ctx := context.Background()
	var units, devs []string
	for range 6 {
		u := boundUnit(t, s, "online")
		var d string
		ownerScan(t, `SELECT id::text FROM devices.devices WHERE unit_id = $1`, []any{u}, &d)
		units, devs = append(units, u), append(devs, d)
	}
	code, m := write(s, &hq, "firmwareCampaigns.schedule", `{"modelId":"`+seed.ID("ventilation-demo").String()+`","targetVersion":"v2","deviceIds":["`+strings.Join(devs, `","`)+
		`"],"waves":[{"label":"Pilot","percent":34},{"label":"All","percent":100}],"window":{"startLocal":"01:00","endLocal":"05:00"},"autoPauseFailurePercent":20,"startAt":"`+
		clock.Add(30*24*time.Hour).Format(time.RFC3339)+`"}`, 0)
	if code != 200 {
		t.Fatalf("schedule: %d %v", code, m)
	}
	camp := data(m)["id"].(string)
	t.Cleanup(func() {
		owner(t, `UPDATE devices.firmware_campaigns SET state = 'aborted' WHERE id = $1 AND state <> 'completed'`, camp)
	})
	base, _ := time.Parse(time.RFC3339Nano, data(m)["createdAt"].(string)) // the server's business time
	kl := time.FixedZone("MYT", 8*3600)
	window := func(from, to int) { // the install window in whole Kuala Lumpur hours around base
		h := base.In(kl).Hour()
		owner(t, `UPDATE devices.firmware_campaigns SET window_start_local = $2::time, window_end_local = $3::time WHERE id = $1`, camp,
			fmt.Sprintf("%02d:00", (h+from+24)%24), fmt.Sprintf("%02d:00", (h+to+24)%24))
	}
	window(-1, 2)
	at := time.Duration(0)
	tick := func(d time.Duration) { // the next scheduler tick, d after the previous one
		t.Helper()
		at += d
		if _, err := schedTick(ctx, s, base.Add(at)); err != nil {
			t.Fatal(err)
		}
	}
	type result struct{ status, key, op string }
	results := func() []result {
		t.Helper()
		out := make([]result, len(devs))
		for i, d := range devs {
			ownerScan(t, `SELECT result, COALESCE(detail, ''), COALESCE(operation_id::text, '') FROM devices.firmware_campaign_devices WHERE campaign_id = $1 AND device_id = $2`,
				[]any{camp, d}, &out[i].status, &out[i].key, &out[i].op)
		}
		return out
	}
	statuses := func() string {
		r := results()
		out := []string{}
		for _, x := range r {
			out = append(out, strings.TrimSuffix(x.status+":"+x.key, ":"))
		}
		return strings.Join(out, " ")
	}
	state := func() (string, string) {
		var st, reason string
		ownerScan(t, `SELECT state, COALESCE(reason, '') FROM devices.firmware_campaigns WHERE id = $1`, []any{camp}, &st, &reason)
		return st, reason
	}
	finish := func(i int, r string) { // the device's answer to its running install (demo.trigger operation)
		t.Helper()
		op := results()[i].op
		if code, m := write(s, &customerB, "demo.trigger", `{"scenarioId":"t","eventId":"`+uuid.NewString()+`","occurredAt":"`+base.Add(at).Format(time.RFC3339Nano)+
			`","eventType":"operation","operationId":"`+op+`","result":"`+r+`"}`, 0); code != 200 {
			t.Fatalf("finish %d: %d %v", i, code, m)
		}
	}
	audits := func(action string) int {
		var n int
		ownerScan(t, `SELECT count(*) FROM audit.audit_log WHERE action = $1 AND target_id = $2`, []any{action, camp}, &n)
		return n
	}
	control := func(action, extra string) {
		t.Helper()
		var v int
		ownerScan(t, `SELECT version FROM devices.firmware_campaigns WHERE id = $1`, []any{camp}, &v)
		if code, m := write(s, &hq, "firmwareCampaigns.control", `{"campaignId":"`+camp+`","action":"`+action+`"`+extra+`}`, v); code != 200 {
			t.Fatalf("%s: %d %v", action, code, m)
		}
	}

	// 24 h before the start: one notice to each client member of each unit's customer, once
	owner(t, `UPDATE devices.firmware_campaigns SET start_at = $2 WHERE id = $1`, camp, base.Add(23*time.Hour))
	tick(time.Minute)
	tick(time.Minute)
	var notices, clients int
	ownerScan(t, `SELECT count(*) FROM notify.notifications WHERE type = 'device_operation' AND template_key = 'device_operation' AND target->>'kind' = 'unit'
		AND target->>'id' = ANY($1::text[]) AND params->>'message' LIKE 'Firmware update from %'`, []any{units}, &notices)
	ownerScan(t, `SELECT count(*) FROM identity.memberships WHERE organization_id = $1 AND role = 'client' AND valid_from <= $2 AND (valid_until IS NULL OR valid_until > $2)`,
		[]any{seed.ID("org-customer-b"), base.Add(at)}, &clients)
	if st, _ := state(); clients == 0 || notices != len(units)*clients || st != "scheduled" || audits("firmwareCampaigns.notice") != 1 {
		t.Fatalf("notice: %d notices for %d clients, %s", notices, clients, st)
	}
	_, m = post(s, &customerB, "notifications.list", `{"filters":{"type":"device_operation"},"limit":100}`)
	seen := false
	for _, it := range items(m) {
		seen = seen || it["target"].(map[string]any)["id"] == units[0]
	}
	if !seen {
		t.Error("the customer does not read the notice")
	}

	// at startAt the campaign runs and the pilot wave (two devices) installs; the second wave waits. startAt is the
	// server's own time, which the ticks do not move, so a later resume is after the start
	owner(t, `UPDATE devices.firmware_campaigns SET start_at = $2 WHERE id = $1`, camp, base)
	tick(2 * time.Minute)
	if st, _ := state(); st != "running" || audits("firmwareCampaigns.start") != 1 ||
		statuses() != "installing installing pending pending pending pending" {
		t.Fatalf("start: %s %s", st, statuses())
	}
	tick(2 * time.Second) // IR67: the operations start a second after their creation
	finish(0, "succeeded")
	finish(1, "succeeded")
	// the pilot passed: the second wave skips the offline, tampered and busy devices and installs the last one
	owner(t, `UPDATE devices.devices SET connection = 'offline' WHERE id = $1`, devs[2])
	owner(t, `UPDATE devices.devices SET tamper = 'detected' WHERE id = $1`, devs[3])
	busy := uuid.NewString()
	owner(t, `INSERT INTO devices.device_operations (id, tenant_id, device_id, unit_id, kind, status, expires_at) VALUES ($1,$2,$3,$4,'check','running',$5)`,
		busy, seed.ID("tenant-a"), devs[4], units[4], base.Add(time.Hour))
	tick(time.Minute)
	if got := statuses(); got != "succeeded succeeded skipped:errors.device_offline skipped:errors.tamper_unresolved skipped:errors.device_busy installing" {
		t.Fatalf("second wave: %s", got)
	}
	var fw string
	ownerScan(t, `SELECT firmware_version FROM devices.devices WHERE id = $1`, []any{devs[0]}, &fw)
	if fw != "v2" {
		t.Errorf("installed version %s", fw)
	}
	// a reported failure: 1 of 4 is above 20 %, so the campaign pauses; a paused campaign installs nothing
	tick(2 * time.Second)
	finish(5, "failed")
	tick(time.Minute)
	if st, reason := state(); st != "paused" || reason != "Auto-paused: 1 of 4 devices failed in wave 2 (more than 20 %)." || audits("firmwareCampaigns.autoPause") != 1 ||
		results()[5].status != "failed" || results()[5].key != "errors.install_failed" {
		t.Fatalf("auto-pause: %s %q %s", st, reason, statuses())
	}
	owner(t, `UPDATE devices.devices SET connection = 'online', tamper = 'clear' WHERE id = ANY($1::uuid[])`, []string{devs[2], devs[3]})
	owner(t, `UPDATE devices.device_operations SET status = 'failed', failure_code = 'TIMEOUT', finished_at = $2 WHERE id = $1`, busy, base.Add(at))
	for _, d := range devs[2:] {
		control("retry_device", `,"deviceId":"`+d+`"`)
	}
	tick(time.Minute)
	if got := statuses(); got != "succeeded succeeded pending pending pending pending" {
		t.Fatalf("paused campaign installs nothing: %s", got)
	}
	// Retry and Resume: the four install again; one does not answer and fails with TIMEOUT, a new failure pauses again
	control("resume", "")
	if st, reason := state(); st != "running" || reason != "" {
		t.Fatalf("resume clears the pause reason: %s %q", st, reason)
	}
	tick(time.Minute)
	if got := statuses(); got != "succeeded succeeded installing installing installing installing" {
		t.Fatalf("resumed: %s", got)
	}
	tick(2 * time.Second)
	for _, i := range []int{2, 3, 4} {
		finish(i, "succeeded")
	}
	tick(61 * time.Second) // the unanswered install times out (IR67)
	tick(time.Second)
	if st, _ := state(); st != "paused" || results()[5].key != "errors.install_timeout" || statuses() != "succeeded succeeded succeeded succeeded succeeded failed:errors.install_timeout" {
		t.Fatalf("timeout: %s %s", st, statuses())
	}
	// resumed without a retry, no new failure pauses it, and the wave waits below 95 %
	control("resume", "")
	tick(time.Minute)
	if st, _ := state(); st != "running" {
		t.Fatalf("waiting: %s", st)
	}
	// outside the window a retried device waits; inside, a device already on the target version succeeds
	window(3, 5)
	control("retry_device", `,"deviceId":"`+devs[5]+`"`)
	tick(time.Minute)
	if got := results()[5].status; got != "pending" {
		t.Fatalf("outside the window: %s", got)
	}
	window(-1, 2)
	owner(t, `UPDATE devices.devices SET firmware_version = 'v2' WHERE id = $1`, devs[5])
	tick(time.Minute)
	if st, _ := state(); st != "completed" || audits("firmwareCampaigns.complete") != 1 || results()[5].op != "" {
		t.Fatalf("complete: %s %s", st, statuses())
	}
	// the campaign as HQ reads it: every result has its time
	_, m = post(s, &hq, "firmwareCampaigns.get", `{"id":"`+camp+`"}`)
	for _, r := range data(m)["results"].([]any) {
		if r.(map[string]any)["at"] == nil {
			t.Errorf("result without a time: %v", r)
		}
	}
	if p := data(m)["progress"].(map[string]any); p["succeeded"].(float64) != 6 {
		t.Errorf("progress: %v", p)
	}
}

// IR339: the dispatch skips a device that left its unit, one now on a unit of another model, and one whose unit has a
// command in flight (D05, control.Busy). A wave passes at exactly 95 % success, but the campaign completes only once
// its last installing device finished.
func TestFirmwareCampaignSkipsAndCompletion(t *testing.T) {
	s := serverWith(t, true)
	ctx := context.Background()
	tenant := seed.ID("tenant-a")
	var units, devs []string
	for range 4 {
		u := boundUnit(t, s, "online")
		var d string
		ownerScan(t, `SELECT id::text FROM devices.devices WHERE unit_id = $1`, []any{u}, &d)
		units, devs = append(units, u), append(devs, d)
	}
	code, m := write(s, &hq, "firmwareCampaigns.schedule", `{"modelId":"`+seed.ID("ventilation-demo").String()+`","targetVersion":"v2","deviceIds":["`+strings.Join(devs, `","`)+
		`"],"waves":[{"label":"All","percent":100}],"window":{"startLocal":"00:00","endLocal":"23:59"},"autoPauseFailurePercent":50,"startAt":"`+
		clock.Add(30*24*time.Hour).Format(time.RFC3339)+`"}`, 0)
	if code != 200 {
		t.Fatalf("schedule: %d %v", code, m)
	}
	camp := data(m)["id"].(string)
	t.Cleanup(func() {
		owner(t, `UPDATE devices.firmware_campaigns SET state = 'aborted' WHERE id = ANY($1::uuid[]) AND state <> 'completed'`, []string{camp})
	})
	base, _ := time.Parse(time.RFC3339Nano, data(m)["createdAt"].(string))
	owner(t, `UPDATE devices.firmware_campaigns SET start_at = $2, notice_sent_at = $2 WHERE id = $1`, camp, base)
	// after scheduling: one device unbound, one moved to a split unit (another model), one unit with a command in flight
	owner(t, `UPDATE devices.devices SET unit_id = NULL WHERE id = $1`, devs[0])
	_, om := write(s, &hq, "units.save", `{"customerOrgId":"`+seed.ID("org-customer-b").String()+`","propertyId":"`+seed.ID("property-home-b").String()+`","spaceId":null,"displayName":"Split `+
		uuid.NewString()[:8]+`","modelId":"`+seed.ID("cap-split-std").String()+`","type":"split","installedAt":null,"serviceScope":["indoor"]}`, 0)
	owner(t, `UPDATE devices.devices SET unit_id = $2 WHERE id = $1`, devs[1], data(om)["id"].(string))
	cmd := uuid.NewString()
	owner(t, `INSERT INTO control.commands (id, tenant_id, unit_id, device_id, actor_membership_id, source, action, status, delivery, requested_at, expires_at, correlation_id)
		VALUES ($1,$2,$3,$4,$5,'ui','{"kind":"set_power","power":false}','requested','sent',$6,$7,'t')`, cmd, tenant, units[2], devs[2], seed.ID("hq-operator"), base, base.Add(time.Hour))
	t.Cleanup(func() { owner(t, `UPDATE control.commands SET status = 'cancelled' WHERE id = $1`, cmd) })
	if _, err := schedTick(ctx, s, base.Add(time.Minute)); err != nil {
		t.Fatal(err)
	}
	got := []string{}
	for _, d := range devs {
		var result, key string
		ownerScan(t, `SELECT result, COALESCE(detail, '') FROM devices.firmware_campaign_devices WHERE campaign_id = $1 AND device_id = $2`, []any{camp, d}, &result, &key)
		got = append(got, strings.TrimSuffix(result+":"+key, ":"))
	}
	if strings.Join(got, " ") != "skipped:errors.device_unbound skipped:errors.other_model skipped:errors.device_busy installing" {
		t.Fatalf("skips: %v", got)
	}

	// a wave of 20 with 19 succeeded passes 95 %; the campaign waits for the twentieth install, then completes
	unit, model := newUnit(t, s, "Wave AC"), seed.ID("ventilation-demo")
	big := uuid.NewString()
	owner(t, `INSERT INTO devices.firmware_campaigns (id, tenant_id, model_id, from_versions, target_version, checksum, waves, window_start_local, window_end_local,
		auto_pause_failure_percent, start_at, state) VALUES ($1,$2,$3,'{1.0}','v2','sha256:x','[{"label":"All","percent":100}]','00:00','23:59',5,$4,'running')`, big, tenant, model, base.Add(-time.Hour))
	t.Cleanup(func() {
		owner(t, `UPDATE devices.firmware_campaigns SET state = 'aborted' WHERE id = $1 AND state <> 'completed'`, big)
	})
	op := uuid.NewString()
	for i := range 20 {
		dev := uuid.NewString()
		owner(t, `INSERT INTO devices.devices (id, tenant_id, serial, target_unit_id, created_by_membership_id, firmware_version) VALUES ($1,$2,$3,$4,$5,'1.0')`,
			dev, tenant, "W-"+dev[:8], unit, seed.ID("hq-operator"))
		if i < 19 {
			owner(t, `INSERT INTO devices.firmware_campaign_devices (campaign_id, device_id, tenant_id, wave, result, result_at) VALUES ($1,$2,$3,0,'succeeded',$4)`, big, dev, tenant, base)
			continue
		}
		owner(t, `INSERT INTO devices.device_operations (id, tenant_id, device_id, kind, status, target_version, campaign_id, expires_at, started_at) VALUES ($1,$2,$3,'firmware','running','v2',$4,$5,$6)`,
			op, tenant, dev, big, base.Add(24*time.Hour), base)
		owner(t, `INSERT INTO devices.firmware_campaign_devices (campaign_id, device_id, tenant_id, wave, result, operation_id, result_at) VALUES ($1,$2,$3,0,'installing',$4,$5)`, big, dev, tenant, op, base)
	}
	state := func() string {
		var st string
		ownerScan(t, `SELECT state FROM devices.firmware_campaigns WHERE id = $1`, []any{big}, &st)
		return st
	}
	if _, err := schedTick(ctx, s, base.Add(2*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if st := state(); st != "running" {
		t.Fatalf("a device still installing: %s", st)
	}
	owner(t, `UPDATE devices.device_operations SET status = 'succeeded', finished_at = $2 WHERE id = $1`, op, base.Add(2*time.Minute))
	if _, err := schedTick(ctx, s, base.Add(3*time.Minute)); err != nil {
		t.Fatal(err)
	}
	if st := state(); st != "completed" {
		t.Fatalf("completed after the last install: %s", st)
	}
}
