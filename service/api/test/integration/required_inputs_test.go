package integration_test

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/pradita/ac-project/service/api/internal/ops"
	"github.com/pradita/ac-project/service/api/internal/platform/apperr"
)

const (
	fieldDecision      = "decision"
	fieldInvoiceID     = "invoiceId"
	fieldProposalID    = "proposalId"
	fieldReportVersion = "reportVersion"
)

// TestRequiredInputs covers the input rules each operation applies before any permission or state check (the
// pipeline decodes, validates, then authorizes): a missing ID, an unknown choice, a reversed period or an over-long
// text is VALIDATION naming the field, whoever asks. Path parameters are sent as the nil UUID, so the route matches
// and the rule for a missing ID fires.
func TestRequiredInputs(t *testing.T) {
	srv := server(t)
	long := func(n int) string { return `"` + strings.Repeat("x", n) + `"` }

	req, inv, length, rng := apperr.KeyRequired, apperr.KeyInvalid, apperr.KeyLength, apperr.KeyRange
	for _, c := range []struct {
		op   string
		body string            // without the path parameters
		want map[string]string // field → message key
	}{
		// maintenance: jobs, proposals, offers, on-site work, reports, follow-ups
		{"jobs.create", `{}`, map[string]string{fieldUnitID: req}},
		{"jobs.cancel", `{}`, map[string]string{fieldJobID: req}},
		{"jobs.hold", `{}`, map[string]string{fieldJobID: req}},
		{opJobsGet, `{}`, map[string]string{fieldJobID: req}},
		{opCommandsGet, `{}`, map[string]string{"id": req}},
		{opPoliciesGet, `{}`, map[string]string{"policyId": req}},
		{opCommandsList, bodyEmptyQuery, map[string]string{fieldUnitID: req}},
		{opJobsEvents, `{}`, map[string]string{fieldJobID: req}},
		{"jobs.resumeHold", `{}`, map[string]string{fieldJobID: req}},
		{opJobsAddNote, `{}`, map[string]string{fieldJobID: req}},
		{"jobs.proposeSlot", `{}`, map[string]string{fieldJobID: req, "replyBy": req}},
		{"jobs.withdrawProposal", `{}`, map[string]string{fieldJobID: req, fieldProposalID: req}},
		{"jobs.withdrawPartnerSlot", `{}`, map[string]string{fieldJobID: req, fieldProposalID: req}},
		{
			"jobs.respondProposal", `{"decision":"maybe",` +
				`"comment":` + long(2001) + `}`,
			map[string]string{fieldJobID: req, fieldProposalID: req, fieldDecision: inv, "comment": length},
		},
		{"jobs.requestReschedule", `{"comment":` + long(2001) + `}`, map[string]string{fieldJobID: req, "comment": length}},
		{"jobs.proposePartnerSlot", `{}`, map[string]string{fieldJobID: req, "technicianMembershipId": req}},
		{"jobs.resolvePartnerSlot", `{"decision":"maybe"}`, map[string]string{fieldProposalID: req, fieldDecision: inv}},
		{
			"jobs.offer", `{}`,
			map[string]string{fieldJobID: req, fieldContractorOrgID: req, fieldVisitSlot: rng, fieldAccessValidUntil: rng},
		},
		{opJobsAccept, `{"termsVersion":` + long(65) + `}`, map[string]string{fieldTermsVersion: length}},
		{opJobsAssign, `{}`, map[string]string{fieldJobID: req, "technicianMembershipId": req}},
		{opMembersEligible, `{}`, map[string]string{fieldJobID: req}},
		{"members.setUnavailability", `{"note":` + long(1001) + `}`, map[string]string{"note": length}},
		{opJobsAcknowledgeAssignment, `{}`, map[string]string{fieldJobID: req}},
		{"jobs.start", `{}`, map[string]string{fieldJobID: req}},
		{opJobsCheckIn, `{}`, map[string]string{fieldJobID: req}},
		{"jobs.pauseWork", `{}`, map[string]string{fieldJobID: req}},
		{opJobsSaveDraft, `{}`, map[string]string{fieldJobID: req, statusDraft: req}},
		{opJobsSaveDraft, `{"items":[{"reason":` + long(1001) + `}],"measurements":[],"parts":[],"refrigerant":[],` +
			`"attachmentIds":[],"nextAction":{}}`, map[string]string{fieldItems: length, fieldNextAction: inv}},
		{"jobs.submit", `{}`, map[string]string{fieldJobID: req, fieldReportVersion: req}},
		{
			"jobs.review", `{"decision":"maybe",` +
				`"reviewMode":"loud"}`,
			map[string]string{fieldJobID: req, fieldReportVersion: req, fieldDecision: inv, "reviewMode": inv},
		},
		{opReportsGet, `{}`, map[string]string{fieldJobID: req, fieldReportID: req}},
		{opReportsSignOff, `{}`, map[string]string{fieldReportID: req}},
		{"attachments.add", `{}`, map[string]string{fieldReportID: req}},
		{"jobs.rate", `{}`, map[string]string{fieldJobID: req}},
		{"jobs.reportProblem", `{}`, map[string]string{fieldJobID: req}},
		{"jobs.classifyFollowUp", `{}`, map[string]string{fieldJobID: req}},
		{"jobs.saveCost", `{}`, map[string]string{fieldJobID: req}},
		{"jobs.extendAccess", `{}`, map[string]string{fieldJobID: req, fieldAccessValidUntil: req}},
		{"jobs.recordWarrantyClaim", `{}`, map[string]string{fieldJobID: req}},
		// maintenance: plans, contractors, rate cards, SLA targets, certificates
		{"plans.save", `{}`, map[string]string{fieldUnitID: req, "nextDueAt": req}},
		{"plans.generateNext", `{}`, map[string]string{"id": req, "occurrenceDate": req}},
		{"contractors.save", `{}`, map[string]string{fieldOrganizationID: req}},
		{"contractors.setOfferStatus", `{"status":"paused"}`, map[string]string{fieldContractorOrgID: req, fieldStatus: inv}},
		{
			"rateCards.save", `{"currency":"XYZ",` +
				`"lines":[]}`,
			map[string]string{fieldContractorOrgID: req, "effectiveFrom": req, fieldCurrency: inv, "lines": apperr.KeyCount},
		},
		{"rateCards.save", `{"currency":"MYR","lines":[{"workType":"emergency","amountMinor":100,` +
			`"note":` + long(201) + `}]}`, map[string]string{"lines": length}},
		{
			"sla.saveTargets", `{"firstTimeFixPercent":150}`,
			map[string]string{"firstTimeFixPercent": rng, "effectiveFrom": req},
		},
		{"certificates.submit", `{}`, map[string]string{"membershipId": req}},
		{"certificates.verify", `{"decision":"maybe"}`, map[string]string{"certificateId": req, fieldDecision: inv}},
		{"certificates.requestTraining", `{}`, map[string]string{"certificateId": req}},
		// billing
		{"contracts.save", `{"planType":"gold"}`, map[string]string{fieldCustomerID: req, fieldPlanType: inv}},
		{"invoices.create", `{}`, map[string]string{fieldContractID: req}},
		{"invoices.remind", `{"reason":` + long(1001) + `}`, map[string]string{fieldInvoiceID: req, fieldReason: length}},
		{"inquiries.answer", `{}`, map[string]string{"inquiryId": req}},
		{"payments.confirm", `{}`, map[string]string{"paymentId": req}},
		{opPaymentsRecordManual, `{}`, map[string]string{fieldInvoiceID: req}},
		{opPaymentsSimulate, `{}`, map[string]string{"event": inv}},
		{opPaymentsSimulate, `{"event":"confirm",` +
			`"paymentReference":` + long(129) + `}`, map[string]string{"paymentId": req, "paymentReference": length}},
		{opPaymentsSimulate, `{"event":"instructions",` +
			`"method":"demo_credit_card"}`, map[string]string{fieldInvoiceID: req, "method": apperr.KeyNotAllowed}},
		// the route fixes the action
		{"payouts.transition", `{"reason":` + long(1001) + `}`, map[string]string{"statementId": req, fieldReason: length}},
		{"payouts.query", `{"message":` + long(2001) + `}`, map[string]string{"lineId": req, "message": length}},
		{"payouts.resolveQuery", `{"reply":` + long(2001) + `}`, map[string]string{"queryId": req, "reply": length}},
		// control, monitoring, restrictions
		{opAutomationsNextRuns, `{}`, map[string]string{"automationId": apperr.KeyExactlyOneTarget}},
		{opAutomationsNextRuns, `{"automationId":"00000000-0000-0000-0000-000000000000",` +
			`"count":8}`, map[string]string{"automationId": req}},
		{"diagnosticRuns.create", `{}`, map[string]string{fieldJobID: req, "expectedUnitVersion": req}},
		{"alerts.resolve", `{}`, map[string]string{"alertId": req}},
		{opPoliciesSetDefaultRule, `{}`, map[string]string{"policyId": req, fieldCustomerID: req, fieldEnabled: req}},
		{"restrictions.cancel", `{}`, map[string]string{fieldRestrictionID: req}},
		{"restrictions.execute", `{}`, map[string]string{fieldRestrictionID: req, "confirmedRulesVersion": req}},
		{
			"restrictions.schedule", `{}`,
			map[string]string{
				fieldContractID: req, fieldExpectedContractVersion: req, fieldExecuteAfter: req,
				fieldRulesVersion: req,
			},
		},
		// assets
		{"customers.save", `{}`, map[string]string{fieldOrganizationID: req}},
		{"units.archive", `{}`, map[string]string{"id": req}},
		{"units.save", `{}`, map[string]string{"customerOrgId": req, "propertyId": req, fieldModelID: req}},
		{"spaces.save", `{"kind":"attic"}`, map[string]string{"propertyId": req, fieldKind: inv, fieldName: length}},
		{"units.setAlertPolicies", `{}`, map[string]string{fieldUnitID: req}},
		{
			"units.importPreview", `{"fileName":` + long(256) + `}`,
			map[string]string{fieldCustomerID: req, "fileName": length, "csvText": req},
		},
		{"units.importCommit", `{}`, map[string]string{"previewId": req, fieldCustomerID: req}},
		{"units.importUndo", `{"reason":` + long(1001) + `}`, map[string]string{"importId": req, fieldReason: length}},
		// devices
		{"devices.register", `{}`, map[string]string{fieldUnitID: req}},
		{"devices.bind", `{}`, map[string]string{fieldDeviceID: req, fieldUnitID: req}},
		{"devices.calibrate", `{}`, map[string]string{fieldDeviceID: req, "sensorId": req, "calibratedAt": req}},
		{"devices.updateFirmware", `{}`, map[string]string{fieldDeviceID: req, "firmwareVersion": req}},
		{"devices.addResponseNote", `{}`, map[string]string{fieldDeviceID: req, fieldEventID: req}},
		{
			"firmwareCampaigns.schedule", `{}`,
			map[string]string{fieldModelID: req, fieldTargetVersion: req, fieldDeviceIDs: req},
		},
		{"firmwareCampaigns.control", `{}`, map[string]string{"campaignId": req}}, // the route fixes the action
		// energy, identity, notifications, writes
		{
			"mrv.recordReview", `{"reviewComment":` + long(4001) + `}`,
			map[string]string{fieldReportID: req, fieldReportVersion: req, "reviewComment": length},
		},
		{opEnergyExportReport, `{"propertyIds":[],` +
			`"sections":[]}`, map[string]string{"propertyIds": inv, "sections": req, "month": inv, "format": inv}},
		{opEnergyExportReport, `{"propertyIds":["00000000-0000-0000-0000-000000000001",` +
			`"00000000-0000-0000-0000-000000000001"],` +
			`"sections":["weather"]}`, map[string]string{"propertyIds": inv, "sections": inv}},
		{
			"members.save", `{}`,
			map[string]string{fieldUserID: req, fieldOrganizationID: req, fieldRole: inv, fieldValidFrom: req},
		},
		{opDevicesEvents, `{}`, map[string]string{"id": req}}, // its own ID field (IR391)
		{opDevicesOperations, `{}`, map[string]string{fieldDeviceID: req}},
		{opDevicesCalibrations, `{}`, map[string]string{fieldDeviceID: req}},
		{opClientUsersRemove, `{"reason":` + long(1001) + `}`, map[string]string{"id": req, fieldReason: length}},
		{"notifications.preview", `{}`, map[string]string{"recipientMembershipId": req}},
		{opWritesGetResult, `{}`, map[string]string{"operation": req}},
		// the other branches of the same rules: wrong shapes, references inside lists, kind-specific fields
		{opLocationsRename, `{"target":{"kind":"space","id":"00000000-0000-0000-0000-000000000000"},"name":" ` +
			`"}`, map[string]string{"target.id": req, fieldName: length}},
		{"properties.save", `{"accessInstructions":` + long(1001) + `}`, map[string]string{"accessInstructions": length}},
		{"units.save", `{"warrantyEndsAt":"not a date",` +
			`"changeReason":""}`, map[string]string{"warrantyEndsAt": inv, "changeReason": length}},
		{opAutomationsSave, `{"unitIds":["00000000-0000-0000-0000-000000000000"],` +
			`"kind":"weekly"}`, map[string]string{fieldUnitIDs: inv, fieldKind: inv}},
		{opAutomationsSave, `{"unitIds":[],"kind":"schedule",` +
			`"condition":{}}`, map[string]string{fieldUnitIDs: inv, fieldKind: inv}},
		{opAutomationsSave, `{"kind":"event","weekdays":[1]}`, map[string]string{fieldKind: inv}},
		{"capabilities.save", `{"manufacturer":"","changeReason":" ` +
			`"}`, map[string]string{"manufacturer": length, "changeReason": length}},
		{
			"baselines.save", `{"unitIds":["00000000-0000-0000-0000-000000000000"]}`,
			map[string]string{fieldUnitIDs: inv, fieldBoundaryID: inv},
		},
		{"factors.save", `{}`, map[string]string{"region": length}},
		{opOffsetsPreview, `{"unitIds":["00000000-0000-0000-0000-000000000000"],"period":{"from":"2026-09-02T00:00:00Z",` +
			`"to":"2026-09-01T00:00:00Z"}}`, map[string]string{"purpose": length, fieldPeriod: rng, fieldUnitIDs: inv}},
		{opOffsetsPreview, `{"unitIds":[]}`, map[string]string{fieldUnitIDs: inv}},
		{opOffsetsSimulate, `{}`, map[string]string{"event": inv, "demoConfirmed": req}},
		{opOffsetsSimulate, `{"event":"request","demoConfirmed":true}`, map[string]string{"quoteId": inv}},
		{opOffsetsSimulate, `{"event":"retire","demoConfirmed":true}`, map[string]string{"recordId": inv}},
		{opOffsetsSimulate, `{"event":"retry","demoConfirmed":true}`, map[string]string{"recordId": inv}},
		{"mrv.saveDraft", `{"evidenceIds":["00000000-0000-0000-0000-000000000000"]}`, map[string]string{"evidenceIds": inv}},
		{"mrv.saveDraft", `{}`, map[string]string{"evidenceIds": inv}},
		{"energy.summary", `{"unitIds":["00000000-0000-0000-0000-000000000000"]}`, map[string]string{fieldUnitIDs: inv}},
		{opClientUsersSave, `{}`, map[string]string{fieldCustomerID: req}},
		{"members.save", `{"role":"technician","scopes":[{"kind":"tenant",` +
			`"id":"00000000-0000-0000-0000-000000000001"}]}`, map[string]string{fieldScopes: inv}},
		{opJobsAcknowledgeAssignment, `{"decision":"cant_make","reason":"busy",` +
			`"alternativeSlot":{"startAt":"2026-09-20T03:00:00Z",` +
			`"endAt":"2026-09-20T01:00:00Z"}}`, map[string]string{"alternativeSlot": rng}},
		{opJobsCheckIn, `{"method":"manual",` +
			`"distanceMeters":-1}`, map[string]string{fieldReason: req, "distanceMeters": rng}},
		{opJobsSaveDraft, `{"items":[],"measurements":[],"parts":[],"refrigerant":[],"attachmentIds":[],` +
			`"nextAction":{"kind":"follow_up"}}`, map[string]string{fieldNextAction: inv}},
		{opReportsSignOff, `{"signerName":"Tan","absentReason":"not at home","sitePhoto":{"name":"p.png",` +
			`"mime":"image/png","size":3,"bytes":"AAAA"}}`, map[string]string{"sitePhoto": apperr.KeyInvalidFile}},
		{opPoliciesSave, `{}`, map[string]string{fieldKind: inv}},
		{opPoliciesSave, `{"kind":"default_alert"}`, map[string]string{"id": req, "rules": apperr.KeyFixedRules}},
		{
			opPoliciesSave, `{"kind":"alert"}`,
			map[string]string{fieldName: req, fieldTimezone: req, fieldEnabled: req, fieldPriority: rng, fieldCustomerID: req},
		},
		// the REST binding refuses an unknown query parameter first
		{opTelemetrySeries, `{"query":{"filters":{"x":"1"}}}`, map[string]string{"x": apperr.KeyNotAllowed}},
	} {
		body := map[string]any{}

		err := json.Unmarshal([]byte(c.body), &body)
		if err != nil {
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

		b := jsonBytes(t, body)
		code, m := write(srv, hq(), c.op, string(b), 0)

		fe, _ := m["fieldErrors"].(map[string]any)
		for field, key := range c.want {
			if code != 422 || fe[field] != key {
				t.Errorf("%s: %s want %s, got %d %v", c.op, field, key, code, fe)

				break
			}
		}
	}
}
