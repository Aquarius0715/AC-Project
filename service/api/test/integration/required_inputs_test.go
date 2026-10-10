package integration

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
)

// TestRequiredInputs covers the input rules each operation applies before any permission or state check (the
// pipeline decodes, validates, then authorizes): a missing ID, an unknown choice, a reversed period or an over-long
// text is VALIDATION naming the field, whoever asks. Path parameters are sent as the nil UUID, so the route matches
// and the rule for a missing ID fires.
func TestRequiredInputs(t *testing.T) {
	s := server(t)
	long := func(n int) string { return `"` + strings.Repeat("x", n) + `"` }
	req, inv, length, rng := "error.required", "error.invalid", "error.length", "error.range"
	for _, c := range []struct {
		op   string
		body string            // without the path parameters
		want map[string]string // field → message key
	}{
		// maintenance: jobs, proposals, offers, on-site work, reports, follow-ups
		{"jobs.create", `{}`, map[string]string{"unitId": req}},
		{"jobs.cancel", `{}`, map[string]string{"jobId": req}},
		{"jobs.hold", `{}`, map[string]string{"jobId": req}},
		{"jobs.resumeHold", `{}`, map[string]string{"jobId": req}},
		{"jobs.addNote", `{}`, map[string]string{"jobId": req}},
		{"jobs.proposeSlot", `{}`, map[string]string{"jobId": req, "replyBy": req}},
		{"jobs.withdrawProposal", `{}`, map[string]string{"jobId": req, "proposalId": req}},
		{"jobs.withdrawPartnerSlot", `{}`, map[string]string{"jobId": req, "proposalId": req}},
		{"jobs.respondProposal", `{"decision":"maybe","comment":` + long(2001) + `}`, map[string]string{"jobId": req, "proposalId": req, "decision": inv, "comment": length}},
		{"jobs.requestReschedule", `{"comment":` + long(2001) + `}`, map[string]string{"jobId": req, "comment": length}},
		{"jobs.proposePartnerSlot", `{}`, map[string]string{"jobId": req, "technicianMembershipId": req}},
		{"jobs.resolvePartnerSlot", `{"decision":"maybe"}`, map[string]string{"proposalId": req, "decision": inv}},
		{"jobs.offer", `{}`, map[string]string{"jobId": req, "contractorOrgId": req, "visitSlot": rng, "accessValidUntil": rng}},
		{"jobs.accept", `{"termsVersion":` + long(65) + `}`, map[string]string{"termsVersion": length}},
		{"jobs.assign", `{}`, map[string]string{"jobId": req, "technicianMembershipId": req}},
		{"members.eligible", `{}`, map[string]string{"jobId": req}},
		{"members.setUnavailability", `{"note":` + long(1001) + `}`, map[string]string{"note": length}},
		{"jobs.acknowledgeAssignment", `{}`, map[string]string{"jobId": req}},
		{"jobs.start", `{}`, map[string]string{"jobId": req}},
		{"jobs.checkIn", `{}`, map[string]string{"jobId": req}},
		{"jobs.pauseWork", `{}`, map[string]string{"jobId": req}},
		{"jobs.saveDraft", `{}`, map[string]string{"jobId": req, "draft": req}},
		{"jobs.saveDraft", `{"items":[{"reason":` + long(1001) + `}],"measurements":[],"parts":[],"refrigerant":[],"attachmentIds":[],"nextAction":{}}`, map[string]string{"items": length, "nextAction": inv}},
		{"jobs.submit", `{}`, map[string]string{"jobId": req, "reportVersion": req}},
		{"jobs.review", `{"decision":"maybe","reviewMode":"loud"}`, map[string]string{"jobId": req, "reportVersion": req, "decision": inv, "reviewMode": inv}},
		{"reports.get", `{}`, map[string]string{"jobId": req, "reportId": req}},
		{"reports.signOff", `{}`, map[string]string{"reportId": req}},
		{"attachments.add", `{}`, map[string]string{"reportId": req}},
		{"jobs.rate", `{}`, map[string]string{"jobId": req}},
		{"jobs.reportProblem", `{}`, map[string]string{"jobId": req}},
		{"jobs.classifyFollowUp", `{}`, map[string]string{"jobId": req}},
		{"jobs.saveCost", `{}`, map[string]string{"jobId": req}},
		{"jobs.extendAccess", `{}`, map[string]string{"jobId": req, "accessValidUntil": req}},
		{"jobs.recordWarrantyClaim", `{}`, map[string]string{"jobId": req}},
		// maintenance: plans, contractors, rate cards, SLA targets, certificates
		{"plans.save", `{}`, map[string]string{"unitId": req, "nextDueAt": req}},
		{"plans.generateNext", `{}`, map[string]string{"id": req, "occurrenceDate": req}},
		{"contractors.save", `{}`, map[string]string{"organizationId": req}},
		{"contractors.setOfferStatus", `{"status":"paused"}`, map[string]string{"contractorOrgId": req, "status": inv}},
		{"rateCards.save", `{"currency":"XYZ","lines":[]}`, map[string]string{"contractorOrgId": req, "effectiveFrom": req, "currency": inv, "lines": "error.count"}},
		{"rateCards.save", `{"currency":"MYR","lines":[{"workType":"emergency","amountMinor":100,"note":` + long(201) + `}]}`, map[string]string{"lines": length}},
		{"sla.saveTargets", `{"firstTimeFixPercent":150}`, map[string]string{"firstTimeFixPercent": rng, "effectiveFrom": req}},
		{"certificates.submit", `{}`, map[string]string{"membershipId": req}},
		{"certificates.verify", `{"decision":"maybe"}`, map[string]string{"certificateId": req, "decision": inv}},
		{"certificates.requestTraining", `{}`, map[string]string{"certificateId": req}},
		// billing
		{"contracts.save", `{"planType":"gold"}`, map[string]string{"customerId": req, "planType": inv}},
		{"invoices.create", `{}`, map[string]string{"contractId": req}},
		{"invoices.remind", `{"reason":` + long(1001) + `}`, map[string]string{"invoiceId": req, "reason": length}},
		{"inquiries.answer", `{}`, map[string]string{"inquiryId": req}},
		{"payments.confirm", `{}`, map[string]string{"paymentId": req}},
		{"payments.simulate", `{}`, map[string]string{"event": inv}},
		{"payments.simulate", `{"event":"confirm","paymentReference":` + long(129) + `}`, map[string]string{"paymentId": req, "paymentReference": length}},
		{"payments.simulate", `{"event":"instructions","method":"demo_credit_card"}`, map[string]string{"invoiceId": req, "method": "error.notAllowed"}},
		{"payouts.transition", `{"reason":` + long(1001) + `}`, map[string]string{"statementId": req, "reason": length}}, // the route fixes the action
		{"payouts.query", `{"message":` + long(2001) + `}`, map[string]string{"lineId": req, "message": length}},
		{"payouts.resolveQuery", `{"reply":` + long(2001) + `}`, map[string]string{"queryId": req, "reply": length}},
		// control, monitoring, restrictions
		{"automations.nextRuns", `{}`, map[string]string{"automationId": "error.exactlyOneTarget"}},
		{"automations.nextRuns", `{"automationId":"00000000-0000-0000-0000-000000000000","count":8}`, map[string]string{"automationId": req}},
		{"diagnosticRuns.create", `{}`, map[string]string{"jobId": req, "expectedUnitVersion": req}},
		{"alerts.resolve", `{}`, map[string]string{"alertId": req}},
		{"policies.setDefaultRule", `{}`, map[string]string{"policyId": req, "customerId": req, "enabled": req}},
		{"restrictions.cancel", `{}`, map[string]string{"restrictionId": req}},
		{"restrictions.execute", `{}`, map[string]string{"restrictionId": req, "confirmedRulesVersion": req}},
		{"restrictions.schedule", `{}`, map[string]string{"contractId": req, "expectedContractVersion": req, "executeAfter": req, "rulesVersion": req}},
		// assets
		{"customers.save", `{}`, map[string]string{"organizationId": req}},
		{"units.archive", `{}`, map[string]string{"id": req}},
		{"units.save", `{}`, map[string]string{"customerOrgId": req, "propertyId": req, "modelId": req}},
		{"spaces.save", `{"kind":"attic"}`, map[string]string{"propertyId": req, "kind": inv, "name": length}},
		{"units.setAlertPolicies", `{}`, map[string]string{"unitId": req}},
		{"units.importPreview", `{"fileName":` + long(256) + `}`, map[string]string{"customerId": req, "fileName": length, "csvText": req}},
		{"units.importCommit", `{}`, map[string]string{"previewId": req, "customerId": req}},
		{"units.importUndo", `{"reason":` + long(1001) + `}`, map[string]string{"importId": req, "reason": length}},
		// devices
		{"devices.register", `{}`, map[string]string{"unitId": req}},
		{"devices.bind", `{}`, map[string]string{"deviceId": req, "unitId": req}},
		{"devices.calibrate", `{}`, map[string]string{"deviceId": req, "sensorId": req, "calibratedAt": req}},
		{"devices.updateFirmware", `{}`, map[string]string{"deviceId": req, "firmwareVersion": req}},
		{"devices.addResponseNote", `{}`, map[string]string{"deviceId": req, "eventId": req}},
		{"firmwareCampaigns.schedule", `{}`, map[string]string{"modelId": req, "targetVersion": req, "deviceIds": req}},
		{"firmwareCampaigns.control", `{}`, map[string]string{"campaignId": req}}, // the route fixes the action
		// energy, identity, notifications, writes
		{"mrv.recordReview", `{"reviewComment":` + long(4001) + `}`, map[string]string{"reportId": req, "reportVersion": req, "reviewComment": length}},
		{"energy.exportReport", `{"propertyIds":[],"sections":[]}`, map[string]string{"propertyIds": inv, "sections": req, "month": inv, "format": inv}},
		{"energy.exportReport", `{"propertyIds":["00000000-0000-0000-0000-000000000001","00000000-0000-0000-0000-000000000001"],"sections":["weather"]}`, map[string]string{"propertyIds": inv, "sections": inv}},
		{"members.save", `{}`, map[string]string{"userId": req, "organizationId": req, "role": inv, "validFrom": req}},
		{"clientUsers.remove", `{"reason":` + long(1001) + `}`, map[string]string{"id": req, "reason": length}},
		{"notifications.preview", `{}`, map[string]string{"recipientMembershipId": req}},
		{"writes.getResult", `{}`, map[string]string{"operation": req}},
	} {
		body := map[string]any{}
		if err := json.Unmarshal([]byte(c.body), &body); err != nil {
			t.Fatalf("%s: body %v", c.op, err)
		}
		spec, ok := ops.SpecByName()[c.op]
		if !ok || len(spec.Routes) == 0 {
			t.Errorf("%s: not in the catalog", c.op)
			continue
		}
		for _, p := range ops.PathParams(spec.Routes[0].Path) {
			if _, set := body[p]; !set && !strings.Contains(p, ".") {
				body[p] = "00000000-0000-0000-0000-000000000000"
			}
		}
		b, _ := json.Marshal(body)
		code, m := write(s, &hq, c.op, string(b), 0)
		fe, _ := m["fieldErrors"].(map[string]any)
		for field, key := range c.want {
			if code != 422 || fe[field] != key {
				t.Errorf("%s: %s want %s, got %d %v", c.op, field, key, code, fe)
				break
			}
		}
	}
}
