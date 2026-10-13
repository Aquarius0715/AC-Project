package integration

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/authz"
	"github.com/pradita/ac-project/service/api/internal/seed"
)

// TestForeignTargets checks authorization by target (IDOR, IR366). Every operation a client, a contractor or a
// technician may call is called by someone the target does not belong to:
//   - customer B on customer A's records;
//   - contractor B on contractor A's job, offer, technician, certificate and payout;
//   - technician B on a job assigned to the internal technician;
//   - HQ and customer A on a unit of another tenant.
//
// The request is valid in every other way, so the scope check is what refuses it: FORBIDDEN or NOT_FOUND, never the
// record and never a change. Lists are asked with filters naming the foreign records and must not return them; a
// filter never widens the caller's scope. The catalog check keeps the table complete: each operation with a role
// other than admin is either called here or named with the reason it takes no foreign target.
func TestForeignTargets(t *testing.T) {
	s := server(t)
	id := func(k string) string { return seed.ID(k).String() }
	at := func(h int) string { return clock.Add(time.Duration(h) * time.Hour).Format(time.RFC3339) }
	img := tinyPNG(t)
	png := blobJSON("photo.png", "image/png", img, len(img))
	pdf := blobJSON("cert.pdf", "application/pdf", []byte("%PDF-1.4 demo"), 13)
	period := `"from":"2026-09-01T00:00:00Z","to":"2026-09-14T00:00:00Z"`

	// customer A's records
	custA, orgA, unitA, devA := id("cust-a"), id("org-customer-a"), id("unit-online-rto"), id("device-online-rto")
	alertA, cmdA, invA, jobA := id("alert-window-a"), id("cmd-limited-apply"), id("invoice-overdue-a"), id("job-internal-a")
	propA, roomA, notifA := id("property-home-a"), id("room-1"), id("notif-alert-window-a")
	var sensorA, clientUserA, partnerUserA string
	ownerScan(t, `SELECT id::text FROM devices.sensors WHERE device_id = $1 ORDER BY id LIMIT 1`, []any{devA}, &sensorA)
	ownerScan(t, `SELECT id::text FROM identity.client_users WHERE customer_id = $1 ORDER BY id LIMIT 1`, []any{custA}, &clientUserA)
	ownerScan(t, `SELECT id::text FROM identity.partner_users WHERE organization_id = $1 ORDER BY id LIMIT 1`, []any{seed.ID("org-contractor-a")}, &partnerUserA)
	policyA := newPolicy(t, "cust-a", "alert")
	code, m := write(s, &customerA, "automations.save", scheduleRule(unitA, nil), 0)
	if code != 200 {
		t.Fatalf("customer A's automation: %d %v", code, m)
	}
	autoA := data(m)["id"].(string)
	code, m = write(s, &customerA, "offsets.preview", `{"purpose":"Offset September usage","amountKg":12.5,"period":{`+period+`},"unitIds":["`+unitA+`"]}`, 0)
	if code != 200 {
		t.Fatalf("customer A's quote: %d %v", code, m)
	}
	quoteA := data(m)["id"].(string)
	done := completedJob(t, s) // customer A's completed job

	// work in progress on customer A's unit: assigned to the internal technician, started, with a draft report
	jobW, unitW := assignedNow(t, s), id("unit-non-rto")
	write(s, &techInt, "jobs.start", `{"jobId":"`+jobW+`","startConfirmed":true}`, 2)
	code, m = write(s, &techInt, "jobs.saveDraft", draftBody(jobW, "normal", nil), 0)
	if code != 200 {
		t.Fatalf("draft report: %d %v", code, m)
	}
	repW := data(m)["id"].(string)
	runW := uuid.NewString()
	owner(t, `INSERT INTO control.diagnostic_runs (id, tenant_id, job_id, unit_id, actor_membership_id, start_action, end_action, duration_minutes, reason, state, start_command_id)
		VALUES ($1,$2,$3,$4,$5,'{"kind":"set_power","power":true}','{"kind":"set_power","power":false}',10,'test','completed',$6)`,
		runW, seed.ID("tenant-a"), jobW, unitW, seed.ID("tech-internal-a"), cmdA)
	t.Cleanup(func() { owner(t, `DELETE FROM control.diagnostic_runs WHERE id = $1`, runW) })

	// contractor A's job, offer, technician, certificate and payout statement
	jobC, offerC, techExtA := id("job-contractor-a"), id("offer-contractor-a"), id("tech-external-a")
	code, m = write(s, &contrA, "certificates.submit", `{"membershipId":"`+techExtA+`","code":"demo_outdoor","name":"Outdoor AC","number":"N-IDOR","issuedAt":"`+
		at(-24*30)+`","expiresAt":"`+at(24*300)+`","file":`+pdf+`}`, 0)
	if code != 200 {
		t.Fatalf("contractor A's certificate: %d %v", code, m)
	}
	certA := data(m)["id"].(string)
	stmtA, lineA := uuid.NewString(), uuid.NewString()
	owner(t, `INSERT INTO billing.payout_statements (id, tenant_id, contractor_org_id, period, status, currency, gross_minor, deductions_minor, net_minor, pay_date)
		VALUES ($1,$2,$3,'2001-01','approved','MYR',30000,0,30000,'2001-02-15')`, stmtA, seed.ID("tenant-a"), seed.ID("org-contractor-a"))
	owner(t, `INSERT INTO billing.payout_lines (id, statement_id, tenant_id, job_id, work_type, amount_minor, kind) VALUES ($1,$2,$3,$4,'repair_base',30000,'charge')`,
		lineA, stmtA, seed.ID("tenant-a"), jobC)
	t.Cleanup(func() { owner(t, `DELETE FROM billing.payout_statements WHERE id = $1`, stmtA) })

	other := uuid.NewString() // an ID that names nothing
	cases := []struct {
		who     *actor
		op      string
		body    string
		version int // as write sends it; reads ignore it
	}{
		// customer B → customer A
		{&customerB, "units.get", `{"id":"` + unitA + `"}`, 0},
		{&customerB, "devices.get", `{"id":"` + devA + `"}`, 0},
		{&customerB, "devices.events", `{"id":"` + devA + `","query":{"limit":5}}`, 0},
		{&customerB, "alerts.get", `{"id":"` + alertA + `"}`, 0},
		{&customerB, "commands.get", `{"id":"` + cmdA + `"}`, 0},
		{&customerB, "commands.list", `{"unitId":"` + unitA + `","query":{"limit":5}}`, 0},
		{&customerB, "commands.create", `{"unitId":"` + unitA + `","action":{"kind":"set_power","power":true},"expectedUnitVersion":1}`, 0},
		{&customerB, "invoices.get", `{"id":"` + invA + `"}`, 0},
		{&customerB, "restrictions.forInvoice", `{"invoiceId":"` + invA + `","query":{"limit":5}}`, 0},
		{&customerB, "inquiries.create", `{"subjectType":"payment","invoiceId":"` + invA + `","message":"Why this amount?"}`, 0},
		{&customerB, "payments.simulate", `{"event":"initiate","invoiceId":"` + invA + `","method":"demo_credit_card","demoConfirmed":true}`, 1},
		{&customerB, "jobs.get", `{"jobId":"` + jobA + `"}`, 0},
		{&customerB, "jobs.events", `{"jobId":"` + jobA + `","query":{"limit":5}}`, 0},
		{&customerB, "jobs.create", jobBody(unitA, nil), 0},
		{&customerB, "jobs.cancel", `{"jobId":"` + jobA + `","cancelReason":"Not needed"}`, 1},
		{&customerB, "jobs.addNote", `{"jobId":"` + jobA + `","message":"Hello","visibility":"customer"}`, 1},
		{&customerB, "jobs.requestReschedule", `{"jobId":"` + jobA + `","preferredSlots":[` + slotJSON(49, 2) + `,` + slotJSON(73, 1) + `,` + slotJSON(97, 1) + `]}`, 1},
		{&customerB, "jobs.respondProposal", `{"jobId":"` + jobA + `","proposalId":"` + other + `","decision":"accept"}`, 1},
		{&customerB, "jobs.rate", `{"jobId":"` + done + `","stars":5,"tags":[]}`, 1},
		{&customerB, "jobs.reportProblem", `{"jobId":"` + done + `","reasonCode":"other","details":"Still dripping","photos":[],"preferredSlot":null}`, 1},
		{&customerB, "reports.get", `{"jobId":"` + jobW + `","reportId":"` + repW + `","reportVersion":1}`, 0},
		{&customerB, "attachments.getContent", `{"jobId":"` + jobW + `","reportId":"` + repW + `","reportVersion":1,"attachmentId":"` + other + `"}`, 0},
		{&customerB, "filterCare.markCleaned", `{"unitId":"` + unitA + `"}`, 0},
		{&customerB, "units.setAlertPolicies", `{"unitId":"` + unitA + `","alertPolicyIds":[]}`, 1},
		{&customerB, "ventilation.log", `{"spaceId":"` + roomA + `","unitId":"` + unitA + `","method":"window_opened","durationMinutes":15}`, 0},
		{&customerB, "locations.rename", `{"target":{"kind":"property","id":"` + propA + `"},"name":"Renamed"}`, 1},
		{&customerB, "locations.rename", `{"target":{"kind":"space","id":"` + roomA + `"},"name":"Renamed"}`, 1},
		{&customerB, "locations.rename", `{"target":{"kind":"unit","id":"` + unitA + `"},"name":"Renamed"}`, 1},
		{&customerB, "policies.get", `{"policyId":"` + policyA + `"}`, 0},
		{&customerB, "policies.save", alertPolicyBody(map[string]string{"id": `"` + policyA + `"`, "recipientMembershipIds": `["` + id("customer-b") + `"]`}), 1},
		{&customerB, "policies.save", alertPolicyBody(map[string]string{"recipientMembershipIds": `["` + id("customer-b") + `"]`}), 0},
		{&customerB, "policies.delete", `{"policyId":"` + policyA + `"}`, 1},
		{&customerB, "policies.setDefaultRule", `{"policyId":"` + defaultPolicyID(t) + `","ruleKey":"ventilation_co2","customerId":"` + custA + `","enabled":false,"reason":"quiet"}`, 1},
		{&customerB, "clientUsers.resendInvite", `{"id":"` + clientUserA + `"}`, 0},
		{&customerB, "clientUsers.save", `{"customerId":"` + custA + `","email":"new.member@example.com","clientRole":"member"}`, 0},
		{&customerB, "clientUsers.save", `{"id":"` + clientUserA + `","customerId":"` + custA + `","email":"new.member@example.com","clientRole":"member","status":"disabled","reason":"left"}`, 1},
		{&customerB, "automations.save", scheduleRule(unitA, nil), 0},
		{&customerB, "automations.save", scheduleRule(unitA, map[string]string{"id": `"` + autoA + `"`}), 1},
		{&customerB, "automations.delete", `{"id":"` + autoA + `"}`, 1},
		{&customerB, "automations.nextRuns", `{"automationId":"` + autoA + `","count":8}`, 0},
		{&customerB, "automations.simulate", `{"eventId":"` + other + `","occurredAt":"` + at(0) + `","unitIds":["` + unitA + `"],"facts":[],"phase":"condition"}`, 0},
		{&customerB, "automations.fire", `{"eventId":"` + uuid.NewString() + `","occurredAt":"` + at(0) + `","unitIds":["` + unitA + `"],"facts":[],"phase":"condition"}`, 0},
		{&customerB, "energy.summary", `{` + period + `,"unitIds":["` + unitA + `"]}`, 0},
		{&customerB, "energy.exportReport", `{"month":"2026-08","propertyIds":["` + propA + `"],"sections":["energy_cost"],"format":"csv"}`, 0},
		{&customerB, "telemetry.series", `{` + period + `,"unitIds":["` + unitA + `"],"metric":"temperature","query":{"limit":5}}`, 0},
		{&customerB, "telemetry.summary", `{` + period + `,"unitIds":["` + unitA + `"]}`, 0},
		{&customerB, "offsets.preview", `{"purpose":"Offset","amountKg":1,"period":{` + period + `},"unitIds":["` + unitA + `"]}`, 0},
		{&customerB, "offsets.preview", `{"customerId":"` + custA + `","purpose":"Offset","amountKg":1,"period":{` + period + `},"unitIds":["` + id("unit-other-customer") + `"]}`, 0},
		{&customerB, "offsets.simulate", `{"event":"request","quoteId":"` + quoteA + `","quoteVersion":1,"demoConfirmed":true}`, 0},
		{&customerB, "notifications.markRead", `{"id":"` + notifA + `"}`, 1},
		{&customerB, "notifications.preview", `{"target":{"kind":"unit","id":"` + unitA + `"},"templateKey":"alert","channel":"inApp","recipientMembershipId":"` + id("customer-b") + `"}`, 0},
		{&customerB, "notifications.recipients", `{"target":{"kind":"unit","id":"` + unitA + `"},"templateKey":"alert","channel":"inApp","query":{}}`, 0},
		{&customerB, "voice.resolveIntent", `{"text":"temperature Bedroom","locale":"en","selectedUnitId":"` + unitA + `"}`, 0},
		// contractor B → contractor A
		{&contrB, "partnerUsers.resendInvite", `{"id":"` + partnerUserA + `"}`, 0},
		{&contrB, "partnerUsers.save", `{"id":"` + partnerUserA + `","email":"moved@example.com","partnerRole":"technician","status":"disabled","reason":"left"}`, 1},
		{&contrB, "partnerUsers.remove", `{"id":"` + partnerUserA + `","reason":"left"}`, 1},
		{&contrB, "jobs.get", `{"jobId":"` + jobC + `"}`, 0},
		{&contrB, "jobs.events", `{"jobId":"` + jobC + `","query":{"limit":5}}`, 0},
		{&contrB, "jobs.accept", `{"jobId":"` + jobC + `","offerId":"` + offerC + `","termsVersion":"v1"}`, 1},
		{&contrB, "jobs.decline", `{"jobId":"` + jobC + `","offerId":"` + offerC + `","reason":"Busy"}`, 1},
		{&contrB, "jobs.assign", `{"jobId":"` + jobC + `","technicianMembershipId":"` + id("tech-external-b") + `","startAt":"` + at(25) + `","endAt":"` + at(27) + `"}`, 1},
		{&contrB, "jobs.proposePartnerSlot", `{"jobId":"` + jobC + `","offerId":"` + offerC + `","slot":` + slotJSON(49, 2) + `,"technicianMembershipId":"` + id("tech-external-b") + `","reason":"Later"}`, 1},
		{&contrB, "jobs.withdrawPartnerSlot", `{"jobId":"` + jobC + `","proposalId":"` + other + `"}`, 1},
		{&contrB, "jobs.review", `{"jobId":"` + jobC + `","reportVersion":1,"decision":"accept","reviewMode":"normal"}`, 1},
		{&contrB, "jobs.addNote", `{"jobId":"` + jobC + `","message":"Hello","visibility":"internal"}`, 1},
		{&contrB, "members.eligible", `{"jobId":"` + jobC + `","startAt":"` + at(25) + `","endAt":"` + at(27) + `","query":{"limit":5}}`, 0},
		{&contrB, "members.setUnavailability", `{"membershipId":"` + techExtA + `","from":"2026-09-20","to":"2026-09-21","type":"training"}`, 0},
		{&contrB, "certificates.submit", `{"membershipId":"` + techExtA + `","code":"demo_outdoor","name":"Outdoor AC","number":"N-B","issuedAt":"` + at(-24*30) + `","expiresAt":"` + at(24*300) + `","file":` + pdf + `}`, 0},
		{&contrB, "certificates.requestTraining", `{"certificateId":"` + certA + `","note":"Please renew"}`, 1},
		{&contrB, "payouts.get", `{"id":"` + stmtA + `"}`, 0},
		{&contrB, "payouts.query", `{"statementId":"` + stmtA + `","lineId":"` + lineA + `","topic":"amount","message":"Why?"}`, 1},
		{&contrB, "reports.get", `{"jobId":"` + jobW + `","reportId":"` + repW + `","reportVersion":1}`, 0},
		{&contrB, "attachments.getContent", `{"jobId":"` + jobW + `","reportId":"` + repW + `","reportVersion":1,"attachmentId":"` + other + `"}`, 0},
		{&contrB, "units.get", `{"id":"` + unitA + `"}`, 0},
		{&contrB, "devices.get", `{"id":"` + devA + `"}`, 0},
		{&contrB, "devices.events", `{"id":"` + devA + `","query":{"limit":5}}`, 0},
		{&contrB, "alerts.get", `{"id":"` + alertA + `"}`, 0},
		{&contrB, "telemetry.summary", `{` + period + `,"unitIds":["` + unitA + `"]}`, 0},
		{&contrB, "telemetry.series", `{` + period + `,"unitIds":["` + unitA + `"],"metric":"temperature","query":{"limit":5}}`, 0},
		// technician B → the internal technician's job on customer A's unit
		{&techB, "jobs.get", `{"jobId":"` + jobW + `"}`, 0},
		{&techB, "jobs.events", `{"jobId":"` + jobW + `","query":{"limit":5}}`, 0},
		{&techB, "jobs.acknowledgeAssignment", `{"jobId":"` + jobW + `","decision":"accept"}`, 2},
		{&techB, "jobs.start", `{"jobId":"` + jobW + `","startConfirmed":true}`, 3},
		{&techB, "jobs.checkIn", `{"jobId":"` + jobW + `","method":"manual","distanceMeters":null,"qrUnitId":null,"reason":"No QR label"}`, 3},
		{&techB, "jobs.pauseWork", `{"jobId":"` + jobW + `","paused":true}`, 3},
		{&techB, "jobs.saveDraft", draftBody(jobW, "normal", map[string]string{"reportId": `"` + repW + `"`}), 1},
		{&techB, "jobs.submit", `{"jobId":"` + jobW + `","reportVersion":1}`, 3},
		{&techB, "jobs.resumeRework", `{"jobId":"` + jobW + `"}`, 3},
		{&techB, "reports.get", `{"jobId":"` + jobW + `","reportId":"` + repW + `","reportVersion":1}`, 0},
		{&techB, "attachments.add", `{"jobId":"` + jobW + `","reportId":"` + repW + `","file":` + png + `}`, 1},
		{&techB, "attachments.getContent", `{"jobId":"` + jobW + `","reportId":"` + repW + `","reportVersion":1,"attachmentId":"` + other + `"}`, 0},
		{&techB, "reports.signOff", `{"jobId":"` + jobW + `","reportId":"` + repW + `","reportVersion":1,"signerName":"Aisha","signature":` + png + `}`, 1},
		{&techB, "units.get", `{"id":"` + unitW + `","jobId":"` + jobW + `"}`, 0},
		{&techB, "units.resolveQr", `{"code":"` + unitW + `"}`, 0},
		{&techB, "devices.get", `{"id":"` + devA + `"}`, 0},
		{&techB, "devices.events", `{"id":"` + devA + `","query":{"limit":5}}`, 0},
		{&techB, "devices.check", `{"id":"` + devA + `","jobId":"` + jobW + `"}`, 1},
		{&techB, "devices.bind", `{"deviceId":"` + devA + `","unitId":"` + unitW + `","jobId":"` + jobW + `","reason":"Swap"}`, 1},
		{&techB, "devices.calibrate", `{"deviceId":"` + devA + `","sensorId":"` + sensorA + `","metric":"temperature","unit":"°C","referenceValue":25,"measuredValue":25.2,"calibratedAt":"2026-09-14T00:10:00Z","jobId":"` + jobW + `"}`, 1},
		{&techB, "devices.calibrations", `{"deviceId":"` + devA + `","query":{"limit":5}}`, 0},
		{&techB, "devices.operations", `{"deviceId":"` + devA + `","query":{"limit":5}}`, 0},
		{&techB, "devices.register", `{"serial":"AX-` + uuid.NewString()[:8] + `","sensorTypes":["temperature"],"unitId":"` + unitW + `","jobId":"` + jobW + `"}`, 0},
		{&techB, "devices.updateFirmware", `{"deviceId":"` + devA + `","firmwareVersion":"v9","jobId":"` + jobW + `"}`, 1},
		{&techB, "devices.addResponseNote", `{"deviceId":"` + devA + `","eventId":"` + other + `","responseNote":"Checked"}`, 1},
		{&techB, "diagnosticRuns.create", `{"jobId":"` + jobW + `","unitId":"` + unitW + `","startAction":{"kind":"set_power","power":true},"endAction":{"kind":"set_power","power":false},"durationMinutes":10,"reason":"Check","expectedUnitVersion":1,"expectedJobVersion":3}`, 0},
		{&techB, "diagnosticRuns.get", `{"diagnosticRunId":"` + runW + `"}`, 0},
		{&techB, "commands.create", `{"unitId":"` + unitW + `","action":{"kind":"set_power","power":true},"jobId":"` + jobW + `","expectedUnitVersion":1}`, 0},
		{&techB, "commands.get", `{"id":"` + cmdA + `"}`, 0},
		{&techB, "commands.list", `{"unitId":"` + unitW + `","jobId":"` + jobW + `","query":{"limit":5}}`, 0},
		{&techB, "alerts.get", `{"id":"` + alertA + `"}`, 0},
		{&techB, "alerts.acknowledge", `{"alertId":"` + alertA + `"}`, 1},
		{&techB, "alerts.resolve", `{"alertId":"` + alertA + `","resolutionReason":"Window closed","resolutionEvidenceIds":[]}`, 1},
		{&techB, "alerts.evidence", `{"alertId":"` + alertA + `","query":{"limit":5}}`, 0},
		{&techB, "telemetry.summary", `{` + period + `,"unitIds":["` + unitW + `"]}`, 0},
		{&techB, "telemetry.series", `{` + period + `,"unitIds":["` + unitW + `"],"metric":"temperature","query":{"limit":5}}`, 0},
		// another tenant's unit
		{&hq, "units.get", `{"id":"` + id("unit-tenant-b") + `"}`, 0},
		{&customerA, "units.get", `{"id":"` + id("unit-tenant-b") + `"}`, 0},
	}
	called := map[string]bool{}
	for _, c := range cases {
		called[c.op] = true
		spec := ops.SpecByName()[c.op]
		var code int
		var m map[string]any
		if spec.Mode == ops.Write {
			code, m = write(s, c.who, c.op, c.body, c.version)
		} else {
			code, m = post(s, c.who, c.op, c.body)
		}
		if code != 403 && code != 404 {
			t.Errorf("%s as %s: %d %v", c.op, c.who.membership, code, m)
		}
		// the scope is checked before the expected version: a stale version gets the same refusal, so the answer
		// never tells that the record exists
		if spec.Mode == ops.Write && c.version > 0 {
			if code, m := write(s, c.who, c.op, c.body, 97); code != 403 && code != 404 {
				t.Errorf("%s as %s with a stale version: %d %v", c.op, c.who.membership, code, m["messageKey"])
			}
		}
	}

	// lists: a filter naming foreign records returns none of them
	lists := []struct {
		who     *actor
		op      string
		body    string
		foreign []string // IDs no returned row may carry
	}{
		{&customerB, "units.list", `{"limit":100,"filters":{"unitIds":["` + unitA + `"]}}`, []string{unitA}},
		{&customerB, "units.list", `{"limit":100,"filters":{"customerId":"` + custA + `"}}`, []string{orgA, custA}},
		{&customerB, "units.list", `{"limit":100,"filters":{"propertyId":"` + propA + `"}}`, []string{propA}},
		{&customerB, "devices.list", `{"limit":100,"filters":{"unitId":"` + unitA + `"}}`, []string{unitA}},
		{&customerB, "alerts.list", `{"limit":100,"filters":{"unitId":"` + unitA + `"}}`, []string{unitA}},
		{&customerB, "alerts.list", `{"limit":100,"filters":{"customerId":"` + custA + `"}}`, []string{orgA, custA}},
		{&customerB, "automations.list", `{"limit":100,"filters":{"unitId":"` + unitA + `"}}`, []string{unitA, autoA}},
		{&customerB, "baselines.list", `{"limit":100,"filters":{"customerId":"` + custA + `"}}`, []string{unitA, custA}},
		{&customerB, "contracts.list", `{"limit":100,"filters":{"customerId":"` + custA + `"}}`, []string{orgA, custA}},
		{&customerB, "invoices.list", `{"limit":100,"filters":{"customerId":"` + custA + `"}}`, []string{custA, invA}},
		{&customerB, "inquiries.list", `{"limit":100,"filters":{"invoiceId":"` + invA + `"}}`, []string{invA}},
		{&customerB, "inquiries.list", `{"limit":100,"filters":{"customerId":"` + custA + `"}}`, []string{custA}},
		{&customerB, "jobs.list", `{"limit":100,"filters":{"unitId":"` + unitA + `"}}`, []string{unitA}},
		{&customerB, "jobs.list", `{"limit":100,"filters":{"customerId":"` + custA + `"}}`, []string{orgA, custA}},
		{&customerB, "offsets.list", `{"limit":100,"filters":{"customerId":"` + custA + `"}}`, []string{custA}},
		{&customerB, "policies.list", `{"limit":100,"filters":{"customerId":"` + custA + `"}}`, []string{custA, policyA}},
		{&customerB, "properties.list", `{"limit":100,"filters":{"customerId":"` + custA + `"}}`, []string{orgA, custA, propA}},
		{&customerB, "spaces.list", `{"limit":100,"filters":{"propertyId":"` + propA + `"}}`, []string{propA, roomA}},
		{&customerB, "ventilation.list", `{"limit":100,"filters":{"unitId":"` + unitA + `"}}`, []string{unitA}},
		{&customerB, "clientUsers.list", `{"limit":100,"filters":{"customerId":"` + custA + `"}}`, []string{custA, clientUserA}},
		{&contrB, "partnerUsers.list", `{"limit":100}`, []string{partnerUserA}},
		{&customerB, "filterCare.list", `{"limit":100,"filters":{"customerId":"` + custA + `"}}`, []string{unitA, custA}},
		{&customerB, "filterCare.list", `{"limit":100,"filters":{"unitId":"` + unitA + `"}}`, []string{unitA}},
		{&customerB, "capabilities.list", `{"limit":100}`, []string{unitA, unitW}},
		{&contrB, "jobs.list", `{"limit":100,"filters":{"organizationId":"` + id("org-contractor-a") + `"}}`, []string{jobC}},
		{&contrB, "members.list", `{"limit":100,"filters":{"organizationId":"` + id("org-contractor-a") + `"}}`, []string{techExtA}},
		{&contrB, "members.capacity", `{"date":"2026-09-15","query":{"limit":100,"filters":{"organizationId":"` + id("org-contractor-a") + `"}}}`, []string{techExtA}},
		{&contrB, "rateCards.list", `{"limit":100,"filters":{"contractorOrgId":"` + id("org-contractor-a") + `"}}`, []string{id("org-contractor-a")}},
		{&contrB, "certificates.list", `{"limit":100,"filters":{"membershipId":"` + techExtA + `"}}`, []string{techExtA, certA}},
		{&contrB, "payouts.list", `{"limit":100,"filters":{"contractorOrgId":"` + id("org-contractor-a") + `"}}`, []string{stmtA}},
		{&contrB, "units.list", `{"limit":100,"filters":{"unitIds":["` + unitA + `"]}}`, []string{unitA}},
		{&contrB, "devices.list", `{"limit":100,"filters":{"unitId":"` + unitA + `"}}`, []string{unitA}},
		{&contrB, "alerts.list", `{"limit":100,"filters":{"unitId":"` + unitA + `"}}`, []string{unitA}},
		{&techB, "jobs.list", `{"limit":100,"filters":{"unitId":"` + unitW + `"}}`, []string{jobW}},
		{&techB, "units.list", `{"limit":100,"filters":{"unitIds":["` + unitW + `"]}}`, []string{unitW}},
		{&techB, "parts.list", `{"limit":100,"filters":{"membershipId":"` + id("tech-internal-a") + `"}}`, []string{jobW}},
		{&techB, "filterCare.list", `{"limit":100,"filters":{"unitId":"` + unitW + `"}}`, []string{unitW}},
		{&techB, "ventilation.list", `{"limit":100,"filters":{"unitId":"` + unitW + `"}}`, []string{unitW}},
		{&techB, "diagnosticRuns.list", `{"unitId":"` + unitW + `","jobId":"` + jobW + `","query":{"limit":100}}`, []string{runW}},
		{&hq, "units.list", `{"limit":100,"filters":{"unitIds":["` + id("unit-tenant-b") + `"]}}`, []string{id("unit-tenant-b")}},
	}
	for _, l := range lists {
		called[l.op] = true
		code, m := post(s, l.who, l.op, l.body)
		if code == 403 || code == 404 {
			continue
		}
		if code != 200 {
			t.Errorf("%s as %s: %d %v", l.op, l.who.membership, code, m)
			continue
		}
		for _, row := range items(m) {
			b, _ := json.Marshal(row)
			for _, f := range l.foreign {
				if strings.Contains(string(b), f) {
					t.Errorf("%s as %s returned a foreign row: %s", l.op, l.who.membership, b)
				}
			}
		}
	}
	// a summary filtered to foreign units counts nothing
	called["summaries.get"] = true
	if code, m := post(s, &customerB, "summaries.get", `{"kind":"customer","filters":{"unitIds":["`+unitA+`"]}}`); code == 200 {
		for k, v := range data(m) {
			if n, ok := v.(float64); ok && n != 0 {
				t.Errorf("summaries.get as customer-b counts %s=%v of customer A's unit", k, n)
			}
		}
	} else if code != 403 && code != 404 {
		t.Errorf("summaries.get as customer-b: %d %v", code, m)
	}

	// marking all read skips another member's notification, the same as an unknown ID, and leaves it as it is (IR379)
	called["notifications.markAllRead"] = true
	var before, after *time.Time
	ownerScan(t, `SELECT read_at FROM notify.notifications WHERE id = $1`, []any{notifA}, &before)
	if code, m := write(s, &customerB, "notifications.markAllRead", `{"ids":["`+notifA+`"]}`, 0); code != 200 || len(data(m)["markedIds"].([]any)) != 0 || len(data(m)["skippedIds"].([]any)) != 1 {
		t.Errorf("notifications.markAllRead as customer-b: %d %v", code, m)
	}
	ownerScan(t, `SELECT read_at FROM notify.notifications WHERE id = $1`, []any{notifA}, &after)
	if (before == nil) != (after == nil) {
		t.Error("customer-b changed customer A's notification")
	}

	// every operation a role other than admin may call is above, or takes no foreign target
	exempt := map[string]string{
		"auth.previewPasswordReset":    "public demo read of the demo e-mail's reset preview",
		"demo.advanceClock":            "demo panel; no target",
		"demo.reset":                   "demo panel; no target",
		"demo.trigger":                 "demo panel; scenarios, no target",
		"demoSession.extend":           "own session",
		"demoSession.signIn":           "public demo sign-in",
		"demoSession.signOut":          "own session",
		"demoSession.switchMembership": "own demo account's memberships (TestPrincipalFromIdentityService)",
		"session.get":                  "own session",
		"preferences.get":              "own session",
		"preferences.update":           "own session",
		"twoFactor.get":                "own session",
		"twoFactor.enable":             "own session",
		"twoFactor.disable":            "own session",
		"consents.get":                 "own membership",
		"consents.update":              "own membership",
		"filterCare.getSettings":       "own customer; no target",
		"filterCare.saveSettings":      "own customer; no target",
		"notifications.list":           "recipient only; no filter names a recipient",
		"writes.getResult":             "another member's key is NOT_FOUND (TestWritesGetResult)",
	}
	for _, spec := range ops.Catalog {
		e, err := authz.Parse(spec.Authorization)
		if err != nil {
			t.Fatal(err)
		}
		nonAdmin := false
		for _, a := range e.Alternatives {
			nonAdmin = nonAdmin || a.Role != "admin"
		}
		switch {
		case !nonAdmin:
		case called[spec.Name] && exempt[spec.Name] != "":
			t.Errorf("%s is called and exempt", spec.Name)
		case !called[spec.Name] && exempt[spec.Name] == "":
			t.Errorf("%s: a role other than admin may call it, and no foreign target is tried", spec.Name)
		}
	}
	for op := range exempt {
		if _, ok := ops.SpecByName()[op]; !ok {
			t.Errorf("exempt %s is not in the catalog", op)
		}
	}
}

// TestTechnicianRebind: a technician binds a device to the unit of their job only when the unit the device leaves is
// in their scope too, through an assignment whose work window contains now (DD-T11 rejects an unauthorized rebinding,
// IR366). An unbound device needs no second unit.
func TestTechnicianRebind(t *testing.T) {
	s := server(t)
	device := func(unit string) string {
		_, m := write(s, &hq, "devices.register", `{"serial":"AX-`+uuid.NewString()[:8]+`","sensorTypes":["temperature"],"unitId":"`+unit+`"}`, 0)
		return data(m)["id"].(string)
	}
	unitOf := func(dev string) string {
		var u *string
		ownerScan(t, `SELECT unit_id::text FROM devices.devices WHERE id = $1`, []any{dev}, &u)
		if u == nil {
			return ""
		}
		return *u
	}
	bind := func(dev, unit, job string, v int) (int, map[string]any) {
		return write(s, &techB, "devices.bind", `{"deviceId":"`+dev+`","unitId":"`+unit+`","jobId":"`+job+`","reason":"Swap"}`, v)
	}
	target := newUnit(t, s, "Rebind target")
	job := assign(t, target, "tech-external-b", clock.Add(-time.Hour), clock.Add(time.Hour), "active")
	t.Cleanup(func() {
		owner(t, `UPDATE maintenance.assignments SET status = 'revoked' WHERE technician_membership_id = $1 AND status = 'active'`, seed.ID("tech-external-b"))
	})

	// a device on a unit outside the technician's assignments stays where it is
	away := newUnit(t, s, "Rebind away")
	foreign := device(away)
	write(s, &hq, "devices.bind", `{"deviceId":"`+foreign+`","unitId":"`+away+`","reason":"install"}`, 1)
	for _, v := range []int{2, 97} {
		if code, m := bind(foreign, target, job, v); code != 404 {
			t.Errorf("rebind from a unit outside the assignments (version %d): %d %v", v, code, m)
		}
	}
	if u := unitOf(foreign); u != away {
		t.Fatalf("the device moved to %s", u)
	}

	// a device on a unit of another assignment of theirs, inside its work window, moves
	source := newUnit(t, s, "Rebind source")
	ours := device(source)
	write(s, &hq, "devices.bind", `{"deviceId":"`+ours+`","unitId":"`+source+`","reason":"install"}`, 1)
	second := uuid.NewString()
	owner(t, `INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at)
		VALUES ($1,$2,$3,$4,'reactive','assigned','client_request',tstzrange($5,$6),$6)`, second, seed.ID("tenant-a"), source, seed.ID("org-customer-b"), clock.Add(2*time.Hour), clock.Add(3*time.Hour))
	owner(t, `INSERT INTO maintenance.assignments (tenant_id, job_id, technician_membership_id, valid_from, valid_until, scheduled, status)
		VALUES ($1,$2,$3,$4,$5,tstzrange($6,$7),'active')`, seed.ID("tenant-a"), second, seed.ID("tech-external-b"), clock.Add(-time.Hour), clock.Add(4*time.Hour), clock.Add(2*time.Hour), clock.Add(3*time.Hour))
	if code, m := bind(ours, target, job, 2); code != 200 {
		t.Fatalf("rebind between two assigned units: %d %v", code, m)
	}
	if u := unitOf(ours); u != target {
		t.Errorf("the device is on %s", u)
	}

	// an unbound device needs only the target unit
	fresh := newUnit(t, s, "Rebind fresh")
	freshJob := uuid.NewString()
	owner(t, `INSERT INTO maintenance.jobs (id, tenant_id, unit_id, customer_org_id, type, status, origin, requested_slot, due_at)
		VALUES ($1,$2,$3,$4,'reactive','assigned','client_request',tstzrange($5,$6),$6)`, freshJob, seed.ID("tenant-a"), fresh, seed.ID("org-customer-b"), clock.Add(5*time.Hour), clock.Add(6*time.Hour))
	owner(t, `INSERT INTO maintenance.assignments (tenant_id, job_id, technician_membership_id, valid_from, valid_until, scheduled, status)
		VALUES ($1,$2,$3,$4,$5,tstzrange($6,$7),'active')`, seed.ID("tenant-a"), freshJob, seed.ID("tech-external-b"), clock.Add(-time.Hour), clock.Add(7*time.Hour), clock.Add(5*time.Hour), clock.Add(6*time.Hour))
	unbound := device(fresh)
	if code, m := bind(unbound, fresh, freshJob, 1); code != 200 {
		t.Errorf("bind an unbound device: %d %v", code, m)
	}
}

// TestFireAuthorizesFirst: automations.fire checks the target units before it returns a stored result or a tick
// CONFLICT (IR21, IR366). Customer B repeating customer A's evaluation gets NOT_FOUND, never A's result: the same
// event, the same input under another event, or other facts in the same tick.
func TestFireAuthorizesFirst(t *testing.T) {
	s := server(t)
	owner(t, `DELETE FROM control.evaluation_events`) // the test clock stands still: a fresh tick
	unitA, now := seed.ID("unit-online-rto").String(), clock.Format(time.RFC3339)
	body := func(event, facts string) string {
		return `{"eventId":"` + event + `","occurredAt":"` + now + `","unitIds":["` + unitA + `"],"facts":[` + facts + `],"phase":"condition"}`
	}
	event := uuid.NewString()
	if code, m := write(s, &customerA, "automations.fire", body(event, ""), 0); code != 200 {
		t.Fatalf("customer A's evaluation: %d %v", code, m)
	}
	occupied := `{"unitId":"` + unitA + `","metric":"occupied","value":true,"unit":"boolean","observedAt":"` + now + `","quality":"valid"}`
	for name, b := range map[string]string{
		"the same event":                 body(event, ""),
		"the same input, another event":  body(uuid.NewString(), ""),
		"other facts in the same minute": body(uuid.NewString(), occupied),
	} {
		if code, m := write(s, &customerB, "automations.fire", b, 0); code != 404 {
			t.Errorf("%s: %d %v", name, code, m)
		}
	}
	// customer A's own retry still gets the stored result
	if code, m := write(s, &customerA, "automations.fire", body(event, ""), 0); code != 200 || data(m)["eventId"] != event {
		t.Errorf("customer A's retry: %d %v", code, m)
	}
}
