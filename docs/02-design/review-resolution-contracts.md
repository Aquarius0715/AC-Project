---
document_id: DD-REVIEW-RESOLUTION
version: 0.30.0
status: self-reviewed-pending-independent-G1
scope: frontend-demo-1A
---

# Correction contracts for the DOC-0.10.0 re-review

This document covers REV-001–018 of INDEPENDENT-DOC-0.10.0 dated 2026-09-16. It defines 1A technical specifications based on the user's instructions to correct and repeat the review. It does not change the original company text or the approvals in DEC-12–14. For the same topic, this document takes priority over older DD/D/SR descriptions. Update the canonical types, operation/screen/version catalogs, and acceptance plan together with this document. This is not a pass record for implementation, real equipment, or real payments.

## IR01 Minimum responses for acceptance and decline

jobs.accept/decline returns only JobDecisionReceipt. It includes no equipment, address, symptoms, costs, or reports beyond jobId/jobVersion/offerId/decision. Even after acceptance, details cannot be read before accessValidFrom. After success, show the result on P02 and return to the job list. Show the detail link only during the currently valid acceptance period, and authorize jobs.get separately. Same-key retries of acceptance/decline and writes.getResult check the original user, valid partner.accept permission of that contractor Membership, and permission to read the minimum receipt for its own Offer. Detail access before the acceptance period, after decline, or after Offer expiry is not a retry condition. Check receipt authorization before the D01 idempotency check. Check the undecided Offer and response deadline required for the first operation only for new operations without an existing receipt. An operation on a decided Offer with a different key returns CONFLICT. After expiry, the same valid Membership may receive only the receipt. Do not return it to an expired Membership or another contractor. writes.getResult resourceIds also contain only jobId/offerId; do not return the saved Job object.

## IR02 Customer ownership is immutable

1A prohibits changes to existing Unit.customerOrgId, Property.customerOrgId, Customer.organizationId, and Contract.customerId/customerOrgId. A save with an id and a different value returns CONFLICT with zero side effects. For new records, determine the owning customer from the authorized parent. Require the parents and equipment sets of Unit/Space/Property/Contract to belong to the same customer. A Space.propertyId change is allowed only when the source and destination customers match. parentSpaceId must belong to the destination Property and form no cycle. A move that cannot keep propertyId consistent with existing descendant Spaces/Units returns CONFLICT. Transfer to another customer requires a new registration with a new ID; do not copy or reassign old history. organizations.save also prohibits changes to an existing Organization.kind.

For a Unit move within the same customer, check both scopes and changeReason. Return CONFLICT while there is an ongoing Command/run/operation, active Job, active Restriction, or unresolved recovery case. On success, increment Unit.version and invalidate the old/new Property and Space, Unit, list, summary, and related history Queries. Readers with Property-level access must be checked against the scope after the move, and caches for the old scope must be discarded. The owning customer of invoices, inspections, Telemetry, and reports does not change. Keep the intersection-based authorization for Device move history in SR24.

## IR03 Permission limited to forced release

Allow HQ users with only restriction.override to call restrictions.list/get within their management scope. Return RestrictionReleaseView (projection=release), containing only id/version/createdAt/updatedAt/unitIds/rulesVersion/policy/state/perUnit/recoveryCases and restriction times needed for the decision. Exclude invoice IDs, contract IDs, reminder text, and the full audit. Users with restriction.write receive the existing Restriction. Open the A09 list entry to override-only users, with the path list → A10 get → override → get. Require the A09 create/apply sections and supporting contract/invoice/equipment reads only for manage holders. For override-only users, list filters support only status (invoiceId/contractId return FORBIDDEN), and counts/order are limited to the scope of this projection. Fetch audit.list only with audit.read.

Allow override-only users to reconcile and retry(phase=release) only for release_requested with an override release intent, or an unresolved recovery case on a terminal record. Reconcile during normal application, retry(apply), and schedule/execute/defer/exempt/cancel/release require manage. override requires a reason, the latest Restriction.version, and a new key. Read the version for reconcile/retry through get as well. Release Command status can be tracked through get.perUnit/recoveryCases, so no extra commands.get permission is needed. override always returns RestrictionReleaseView. Project reconcile/retry write responses and writes.getResult to RestrictionRead using current permissions, so a saved full response is not exposed after manage permission is lost. Also remove resourceIds outside the projection.

## IR04 Shared simulated reminder records

notifications.preview remains a read and saves no records. HQ A08 fetches an overdue unpaid Invoice, selects a customer recipient through notifications.recipients(target=invoice,templateKey=payment_reminder,channel), shows the preview, then calls invoices.remind after the explicit action "Record simulated reminder". Inputs are invoiceId/recipientMembershipId/channel/reason. expectedVersion is Invoice.version, and idempotencyKey is required. reason must be 1–1000 characters. Do not send externally.

In the same transition, the Repository checks billing.write, invoice scope, latest version, now>dueAt, status=unpaid, and that the recipient is an active Membership of that customer eligible for the channel. An expired recipient returns VALIDATION; paid/processing/not yet overdue returns CONFLICT. On success, save one Notification and one audit record, and increment Invoice.version by 1. Notification has templateKey/type=payment_reminder; inApp is simulated and email/whatsapp are preview. All channels are saved in notifications.list. Return InvoiceReminderReceipt with only invoiceId/notificationId/invoiceVersion. Do not return the notification body to HQ users other than the recipient. The customer sees the same notification on C08 and the same Invoice on C11. A same-key retry returns the same receipt without creating duplicates. A reminder with a new key requires the latest Invoice version after checking it again. A read-only preview does not increment the Invoice version.

## IR05 Advance notice time and evidence

Remove noticeAt from restrictions.schedule input. The Repository uses the request acceptance time now as noticeAt and checks executeAfter>=now+24 hours. Generate an inApp restriction notice for every active client Membership of the target customer that can read the Contract and all target Units of the Restriction. Save their IDs in noticeNotificationIds. If there are no recipients, return VALIDATION with zero restriction, notification, and successful audit records. Save the Restriction and notifications atomically. The customer can see the notice in the notification list in the same tab. This is simulated notice, not proof of real delivery.

execute/retry(apply) rechecks that the notice IDs refer to the same Restriction, customer, and notice time; that both 24 hours after notice and executeAfter have passed; and that the original policy/rulesVersion/targets/cause-invoice conditions still hold. A change to the application details after notice requires a new notice. Reading the notification is not a required execution condition. Past fixtures are for initial seeds only and must generate notice evidence together. Normal operations/demo.trigger cannot insert a past noticeAt. Use clock advance to pass the 24-hour wait.

## IR06 Only one active payment attempt

Each Invoice may have at most one Payment in initiated or processing in total. Within the same transition, payments.simulate(event=initiate) checks that the Invoice is unpaid and has no nonterminal Payment, creates Payment=initiated, sets Invoice.status=processing/paymentStatus=initiated, and increments Invoice.version. The initial expectedVersion is the Invoice version. A same-key retry returns the original result. A different key returns CONFLICT even with the latest Invoice version and creates no additional attempt.

A processing event updates Payment and Invoice.paymentStatus to processing and increments both versions. confirm updates them to confirmed/paid; fail updates them to failed/unpaid. Keep Invoice.paymentMethod after failure. Retries after finalization do not increment versions. recordManual returns CONFLICT if either initiated or processing exists. A retry after failure requires the latest Invoice version and a new key. demo_instructions creates no Payment and does not affect this exclusion rule or version. Deliver state updates and subscription notifications after the same transition commits.

## IR07 Shared Policy form

All kinds on A05/A11/A12 require name (1–120 characters after trim), unitIds (nonempty, no duplicates), timezone (supported IANA name), enabled (boolean), and priority (integer 0–100). For new records, clearly show an empty name, no selected equipment, timezone=Preferences.timezone, enabled=false, and priority=50 in the UI. Editing uses fetched values; the Repository does not fill in missing values. Keep these defaults consistent with Automation. Kind-specific required fields follow the existing DD/SR28; saving is blocked when they are missing.

A05 obtains target equipment/capabilities through units.list → selected units.get, regardless of whether alerts exist. A12 also uses units.list → units.get. Candidate reads on A05 are within alert.policy.write scope; on A12, within automation.policy.write scope. Notification recipients use the eligible set for all selected targets/channels. Do not show the policy save form to read-only staff.

## IR08 Energy data origin

D07 actual energy and the SR29 demo_period_comparison baseline integrate only power samples with origin=measured and quality=valid. isDemo=true labels all demo values and is independent of origin. estimated/inspection values may appear in series but do not count toward measured slots/coverage; include non_measured_input in qualityWarnings. First select one candidate that exactly matches the slot start using D07 sequence/id order, then check origin, quality, and unit. If the latest is estimated/suspect, do not fall back to an older measured/valid sample. If any slot is excluded, coverage<1 and comparison values are null. Continue to label fixed-model baselines as modeled. SR27 operating-state detection also requires the latest power to be measured.

## IR09 Operation context for voice changes

VoicePanel does not create a Command merely because resolveIntent succeeds. For change, show the equipment, requested temperature, and current observation for confirmation. A technician filters their assigned candidates from jobs.list by target unitId and confirms the selected jobId and valid period through jobs.get detail. With no candidates, the operation is unavailable; with multiple candidates, require an explicit selection. The page jobId may be the initial selection, but still recheck it. technician/admin must enter reason (1–1000 characters after trim). A client sends no jobId and uses the normal conditions for controlling their own equipment.

On confirmation, fetch units.get again (also specify jobId for a technician) and any required jobs.get. If the observation, version, or assignment changed, update the confirmation display and request confirmation again. Pass jobId, reason, and expectedUnitVersion to commands.create in the same form as the normal form. Reject access that expires during confirmation. cancel/help/unsupported/temperature inquiries perform zero writes. Use the same selection steps when opened from the home screen. State VoicePanel dependencies in the shared component contract and fetch them lazily on every voice-enabled page.

## IR10 Notification business categories

Save Notification.type separately from severity. Types are cleaning_due/fault/quality/schedule_change/report_return/completion/payment/payment_reminder/restriction/inquiry. Map Alert.type=maintenance to cleaning_due, sensor/tamper/reconciliation_required to fault, and quality to quality. Preview input for the alert template requires sourceAlertId. The Repository checks the source Alert's current scope and that target.unit matches before classifying it. Missing input or a target mismatch returns VALIDATION; an out-of-scope source returns NOT_FOUND. Also save the source ID in Notification.sourceAlertId. Other templates prohibit this input and output null. Other templateKey values use the type of the same name. Keep the classification after creating the notification. Show a cleaning icon for cleaning_due and a fault icon for fault; severity controls only the severity color/label. Unknown classifications return UNAVAILABLE under D01, with no fallback to a healthy icon.

## IR11 Calculation boundary identity

The actual-data boundary ID in 1A is ac_input_electricity (AC equipment input electricity, excluding shared equipment, solar generation, and batteries). Sensor.boundaryId is this ID for power and null otherwise. Determine Measurement.boundaryId from the Sensor at ingestion and save it immutably; do not rewrite it when the current Sensor moves. For inspection, it is null. Actual aggregation uses only measurements from this boundary. A missing or mismatched boundary makes the slot invalid and adds boundary_mismatch. EnergySummary.boundaryId is this ID regardless of the baseline; boundary is fixed explanatory text.

BaselineInput/EnergyBaseline boundaryId is ac_input_electricity or whole_building_electricity. The latter is allowed only for a fixed model demonstrating comparison mismatch; demo_period_comparison returns VALIDATION. The boundary description is editable text of 1–500 characters and is not used to determine identity. Comparison requires matching IDs. On mismatch, differences, rates, and reduction-related values are null, while actual values remain. MRVConditions also requires boundaryId. A mismatch with the baseline/actual boundary sets incomplete=true and prevents demo_reviewed. Saved Reports retain the ID and the description at that time as a snapshot. The UI offers fixed choices and does not accept free-form IDs.

## IR12 Raw and normalized measurements

RawMeasurement is only for the demo input boundary and accepts unit:string/value:number|null. demo.trigger telemetry accepts RawMeasurement, normalizes it to the public Measurement, then saves it. An unknown metric returns input VALIDATION. A missing or out-of-scope sensor returns NOT_FOUND. For a known sensor, an unknown unit or a unit that differs from the metric is not converted: save value=null/quality=suspect, the sensor's expected unit in unit, qualityReason=unit_mismatch, and the received string (at most 32 characters) in rawUnit. The UI shows the value as missing and displays escaped rawUnit as evidence. Nonfinite values use non_finite; out-of-range values use out_of_range. Normal values have both qualityReason and rawUnit null. Do not overwrite null with a suspect cause as missing.

At ingestion, generate boundaryId from the Sensor and id/version/tenantId/createdAt/updatedAt/isDemo from the Repository. Do not accept these output-only values in RawMeasurement. observedAt/receivedAt are RawMeasurement inputs; validate ISO UTC format and time order. The outer DemoTrigger.eventId and measurement.eventId must match. A membership mismatch outside scope returns NOT_FOUND; an inconsistent sensor/unit pair within the same scope returns VALIDATION. An invalid future time uses invalid_time. If multiple causes apply, use this priority: unit_mismatch → non_finite → out_of_range → invalid_time. Inspection input normalization generates the same additional fields. Unknown public DTO enum values still return UNAVAILABLE. Do not confuse an unknown raw received string with an unknown public enum.

## IR13 Align current acceptance conditions

Current equipment KPI links in AT-C01-N/AT-A01-N pass only scope and powerState. period is a condition for time-series/amount aggregation and is not passed to the equipment list. Back navigation restores the original dashboard period. AT-A08-N checks, in order, a simulated reminder for an overdue unpaid invoice, payment confirmation, and reminder rejection after paid. Distinguish preview counts from saved-operation counts. For Restriction transitions after payment, distinguish scheduled → cancelled from requested/applied → release_requested.

## IR14 Identify the current version

The current specification consists of the manifest files in the current baseline shown by README (specification 0.21.0; English translation baseline TRANSLATION-EN-2026-09-17; see IR75). Align front matter, the current implementation baseline, and guidance with that version. Keep past reviews, DEC records, change history, and old runs at their historical versions. Do not use old pass records as approval of the current version.

## IR15 MRV output scope

FR-A14 covers screen preview, draft saving, version references, and demo review only. File export/download, such as CSV/PDF, is outside 1A. Change DD's "output data" to mean "preview of a saved version". Do not add an undefined export feature during implementation.

## IR16 Fact freshness and reuse

The Fact evaluation time is the Repository demo clock now. fire occurredAt must match the current tick; scheduled events are evaluated when the clock reaches that tick. A new fire for a past tick returns VALIDATION. Fetching an existing result with the same eventId does not reevaluate it. simulate is a temporary evaluation on the current snapshot and is not saved.

| metric | Type/unit | TTL seconds | Retention |
|---|---|---|---|
| Normal Metric | D07 number/unit | Sensor.staleAfterSeconds | Latest observation |
| weather_temperature | number / °C | 1800 | Latest observation |
| tariff | number / MYR_per_kWh | 300 | Latest observation |
| solar / battery | number / kW | 120 | Latest observation |
| occupied / peak | boolean / boolean | 120 | Latest observation |
| location | arrival or departure / event | 0 | Same tick only |

A future observedAt is bad quality and gives missing_data; now-observedAt>TTL gives stale. Exactly at the TTL boundary is fresh. Normal Metrics use D07 ranges; finite demo values are -50–100 for weather, 0–100 for tariff, and 0–1000 for solar/battery. Unknown metrics/types/units return input VALIDATION. value=null or bad quality does not match. For retained types, choose the latest observedAt; for equal times, apply D02 eventId order before checking quality. Do not fall back to an older valid value. Update combined facts only when fire/scheduled events commit; simulate does not change them. Do not carry location into the next tick. Discard unevaluated location on consent withdrawal. Reevaluate TTL expiry on clock ticks, without firing past matches again.

## IR17 Repository generation and view generation

Session.generation, ChangeEvent.generation, and the generation in idempotency keys are the Repository generation, which increases monotonically only on reset. Role switching/logout does not delete shared business records. Session.viewEpoch is the view generation. It increases monotonically on successful signIn, successful switch, signOut, expiry, and reset. Even A → B → A does not reuse an old value. It also increases on scopeVersion/permission change notifications, followed by a Session refetch. These generations use separate counters.

UI query keys/async callbacks capture the Repository instance ID, generation, viewEpoch, tenantId, membershipId, and scopeVersion, and compare all of them with current values before rendering/notifying. Subscription closures also capture viewEpoch. Discard callbacks already delivered from old subscriptions even after unsubscribe. A reload uses a different instance ID to reject old responses. Aborting does not cancel the business operation: accepted work commits to the Repository and is fetched after the current session is authorized again. Do not send viewEpoch in Context or use it as authorization evidence.

## IR18 Long-term memory retention

Defer REV-018 as a possible additional requirement, not a defect in existing requirements. The 1A guarantee covers the D10 demo scale of 100 equipment units/1000 samples; it does not guarantee unlimited continuous operation or a memory ceiling. Keep the current contract that retains snapshots/events/idempotent results until reset/reload. Do not add automatic deletion or implicit reset. Capacity limits and rejection/offloading methods are requirements for a stage that includes continuous operation. Do not claim untested performance passes. Discard confirmation for user-selected reset follows the existing specification. Do not count this candidate as a missing existing G1 feature, but retain it in the handoff.

## Supplement: Creation and reference paths confirmed by re-review

IR11/12: The model form shows and sends the Capability.sensors boundary as a read-only derived field: ac_input_electricity for metric=power, otherwise null. Validate that mapping when saving the model. devices.register/bind creates Sensors with the same values. jobs.saveDraft InspectionMeasurementInput uses origin=inspection and boundaryId=null, and fills in the quality cause/rawUnit during normalization. Both are null for normal input and for missing measurements. Do not allow arbitrary forged Sensor boundaries to enter actual values.

IR17: When viewEpoch changes, discard all list cursors and snapshot Queries as well. Query cursors still belong to the Repository generation/scope under SR14; the UI must not reuse them across viewEpoch values. In a new view generation, rebuild the snapshot from the first page.

## IR19 Visibility of restrictions on multiple equipment units — CV-001

Apply the SR03 all-target-Units condition to Restriction get/list/forInvoice, write responses, notifications, and writes.getResult. Do not return the whole Restriction to a Membership that can read only some targets. Individual reads return NOT_FOUND; lists exclude it from counts as well. Customers must also meet the own-customer condition for the Contract/Invoice. The notification recipient's organization alone does not grant access to all equipment. If schedule has no eligible reader of all targets, return IR05 VALIDATION. If a recipient expires after notice creation, keep the saved notice evidence and deny notification reads using current scope. Equipment-level users receive only the UnitDetail.effectiveControlPolicy projection without billing information.

## IR20 Reminder preview preconditions — CV-004

payment_reminder notifications.preview/recipients is limited to HQ with billing.write. Also check target.kind=invoice, now>dueAt, and Invoice.status=unpaid on reads. An ineligible state returns CONFLICT, missing permission returns FORBIDDEN, and out-of-scope access returns NOT_FOUND. preview checks the same customer-recipient/channel conditions as IR04. If payment starts after preview, remind rechecks and rejects it. The preview Notification is unsaved; do not reuse its temporary ID in markRead or notification lists. On successful remind, the Repository issues an ID for the saved record.

## IR21 Fact evaluation time and sensors — CV-007

For both simulate/fire, new input occurredAt must match the current demo clock tick. simulate temporarily overlays input facts on the current retained facts and uses the same evaluation function, without updating retained values, cooldown, or duration counters. Determine the TTL of a normal Metric from the Sensor for the same metric in the Unit's current binding. No Sensor gives missing_data. The fact unit must match that Sensor's unit. Do not reuse values from a past binding in a new binding; discard retained facts on bind. External demo Facts such as solar use the fixed IR16 TTL. Duplicate unitId/metric pairs within one input, or Facts outside input unitIds, return VALIDATION. Use D02 eventId order only when merging separate events into the same tick. A retry of a committed fire with the same eventId returns the existing result after authorization. Check idempotency before checking whether the current tick matches.

## IR22 Saving raw measurements and inspections — CV-006

RawMeasurement is only for demo.trigger telemetry. jobs.saveDraft continues to use InspectionMeasurementInput and accepts only known UnitSymbol values. A mismatched unit in inspection input returns VALIDATION and rejects the entire report save, with no partial update. This differs from IR12, which saves mismatched raw input as suspect. Nonfinite values, out-of-range values, and future times in inspections use the same quality causes and value=null. Other normal values use qualityReason/rawUnit=null, origin=inspection, and boundaryId=null. Always set isDemo=true when creating a public Measurement.

## IR23 Job list projections and search — PV-001–003

jobs.list is a union of JobSummary/JobOfferSummary/JobHistorySnapshot. For contractors, use offer before acceptance and after acceptance but before accessValidFrom, summary during the accepted access period, and history afterward. Exclude declined jobs from the list; only the IR01 receipt can be fetched. Also exclude unanswered Offers when offerExpiresAt is reached. Provide history after expiry only for jobs accepted by that contractor.

JobOfferSummary.status is offered when its own Offer.decision is null and accepted when it is accept. Do not reuse the site's Job.status as the public status. severity=null means undisclosed, not normal. JobHistorySnapshot freezes the status/type/contractorOrgId/completedAt, own decision events, and redactedReportSummary that were readable just before access ended. asOf is the freeze time. If access is withdrawn before it starts, provide only the receipt and create no history. Do not reflect another company's work or report updates after expiry. history has severity/dueAt=null and contains no equipment ID, address, contact information, costs, or Assignment. ownDecisionEvents contains only the user's own acceptance/decline operations, with note/reportRef=null; do not mix in others' events.

Evaluate filters, sort, and total after projection. summary follows the existing definition, including joins with authorized equipment. offer uses id=jobId and public status/dueAt/requestedSlot; organizationId checks only a match with the user's own company. severity/unitId/membershipId/customerId/propertyId are private, so specifying one makes that offer fail the filter. history also uses id=jobId and frozen status/contractorOrgId. Period filters use completedAt; null fails a period filter. It does not match severity/unitId/membershipId/customerId/propertyId/overdueOnly=true filters. offer period filters use requestedSlot.startAt; overdue uses the public dueAt. Sort only by public fields, with null last for both asc/desc and ties broken by ASCII ascending id/jobId. Changes to the current private Job/Alert do not change offer/history counts, order, or filter results.

P01/P03/P06 distinguish projections. For offer, show severity as undisclosed; for history, show the deadline as "Not applicable". Clicking history only opens jobs.get history; do not add equipment links or editing actions. Calculate partner summary from the same projections and filter set. total is that count; offerCount counts offer.status=offered. Business active/scheduled/inProgress/review/overdue uses only currently valid summary records and the existing status rules. Do not include accepted offers with a future start or history in active business counts. Do not count zero severity as normal equipment.

## IR24 Reduced visibility during paging — PV-004

The immutable snapshot in SR14 guarantees stability against business-value updates; it does not preserve old access rights. Before returning each page or writes.getResult, recheck current delegation, assignment, qualification, and resource ownership independently of scopeVersion. If any row in the snapshot has a narrower public projection or loses read access, invalidate the whole snapshot and return CONFLICT without counts or partial rows. Fetch again from the first page. The new snapshot uses projections such as IR23 history. Session expiry takes priority as UNAUTHENTICATED.

Access changes from Offer/Assignment expiry, withdrawal, or Unit relocation increment the current viewEpoch through clock/change events. Discard the affected displayed Queries, details, and snapshot cursors. IR17 rejects old callbacks immediately after expiry. Repository reads perform the same checks, so old data is not returned again while event notification is pending. Freeze history only once as internal processing when access ends. Do not later read the current Job to reconstruct data from before expiry.

IR23 technician path (PV-005): Return summary/detail while the Assignment is valid and read conditions hold. Return only history to the same person whose assignment has ended if they previously had an actual access period. Internal jobs have contractorOrgId=null. A technician with no own acceptance/decline has an empty ownDecisionEvents array. Bind history internally in the Repository to the original userId/membershipId. Return it only if the same Membership remains active and it is within current scope. Reassignment to someone else does not change the history owner. Cancelling an Assignment before it starts creates no history. Calculate technician business counts and assigned equipment counts only from currently valid summary records; do not count history as active work.


## IR25 Installation address and report display after expiry

User decision: Use the address of the installation site linked to the AC unit. After expiry, the report summary shows only "Report present/absent" and "Accepted/not accepted". Do not require HQ to enter the address/region again when delegating.

JobOfferSummary.siteAddress reads Job.unitId → ACUnit.propertyId → Property.address in the current read snapshot. Remove regionLabel. Expose this address only in the pre-acceptance projection of the company's own Offer. This does not add access to the whole Property or expose Unit ID, entry instructions, or customer contacts. Manage the address in the existing installation-site form and show it read-only in the delegation form. Normalize null/whitespace-only values to null and show "Address not registered". Do not fill it in or infer a region from the property name or report body. Address updates also invalidate jobs Queries. Apply SR14 to page snapshots; address changes appear in a new snapshot. Do not retain the address in history after expiry.

redactedReportSummary is ReportHistorySummary, not free text. Use the highest version among report versions readable by that viewer just before access ended (including drafts for viewers who can read drafts before submission). With no target, return {hasReport:false,acceptance:not_accepted}. With a target, hasReport=true; use accepted if that version has an acceptance event, otherwise not_accepted. Even if an older version was accepted, show not accepted when the latest readable revision is unaccepted. Freeze this internally in the Repository, without body text, photos, notes, contacts, report ID, or version number. The UI uses en/ms dictionaries to show three states: "No report / Not accepted", "Report present / Not accepted", and "Report present / Accepted". Do not save translated free text in the DTO. Later report submission, acceptance, or address changes do not rewrite the expired-access snapshot. Keep original reports and saved versions as before.

## IR26 Job summary filters

summaries.get with kind=partner/technician also accepts status/statuses/severity/overdueOnly shared with jobs.list. Using both status and statuses, empty statuses, or invalid enums returns VALIDATION. These job-only conditions return VALIDATION for kind=customer. Keep existing customerId/propertyId/unitId/unitIds/from/to. Match unitIds only against summary records with a public unitId under IR23; an empty array gives zero results. Calculate partner/technician job counts from the set after IR23 projection and the same AND conditions. Do not filter using private history/offer values. The technician assigned-equipment count deduplicates unitId from currently valid summary records in that set. Status, severity, and period changes on P01/T01 pass the same conditions to the list and summary; do not filter only one side. Do not mix kind=customer equipment metrics with job conditions.

## IR27 Reasons for stopping automatic operation

Return the stop reason required by FR-A04 in RuleBase.disabledReason. It is Repository-generated capability_changed/unit_archived/null, not form input. If a capability change makes an existing Automation incompatible, set enabled=false and disabledReason=capability_changed, increment version, and notify automations/units/capabilities after the same transition. A stop caused by equipment archive uses unit_archived. New records and explicit user disabling from enabled=true to false use null. Do not reenable automatically after a stop. Explicit save(enabled=true) rechecks current capabilities, archive state, and scope for all target equipment. Incompatibility causes zero side effects; success resets the reason to null. Editing while still disabled keeps the existing reason. If even one of multiple targets is incompatible, stop the whole rule. C04/C05/A04 display the returned reason through en/ms dictionaries; do not require customer audit.read to learn why it stopped. Use the same output type for Policy, but this section alone does not add new automatic Policy stopping behavior.

## IR28 Reason input for model saving

capabilities.save changeReason is optional for a new record and required as 1–1000 characters after trim when updating an existing ID. If supplied for a new record, validate the same length. A missing or empty update reason returns VALIDATION. The new-record UI has an optional field; the edit UI has a required field. Optional in the canonical type represents the input branch, not permission to skip validation on update.


## IR29 Job completion time

MaintenanceJob.completedAt is Instant|null held by the Repository. New jobs from jobs.create and plans.generateNext have null. In the same transition where jobs.review(decision=accept) passes authorization, self-approval, version, and state checks, set status=completed and save that transition's Repository demo clock now as the completion time once. The report-version acceptance event and job.completed event use the same time. Returned, rejected, incomplete, and cancelled jobs have null. Same-key retries, reads, notes, and cost updates do not change completedAt. 1A has no feature to reopen completed jobs. Do not accept completedAt as UI input or derive it from updatedAt, submission time, or scheduled end.

IR23 history freezes MaintenanceJob.completedAt exactly as it was just before access ended. If access ends before completion, it stays null and does not reflect another person's later completion. Period filters apply [from,to) to frozen completedAt; null does not match. Initial seeds containing completed jobs must provide completedAt matching the acceptance event.

## IR30 Equipment alert severity shown on jobs

JobSummary.severity is the highest severity among Alerts belonging to Job.unitId that are visible to the current viewer and have status=open/acknowledged, ordered critical>warning>normal. Job.alertIds links the reasons for job creation; it does not limit the Alerts used for current equipment severity. Exclude resolved Alerts. With no eligible Alerts, use normal (no unresolved alerts). This value does not guarantee healthy communication or measurements; keep separate offline/stale/missing quality displays.

Lists, severity filters/sorts, and P01/T01 summaries share the same function over the authorized snapshot. Do not recalculate only Job or Alert from a newer snapshot. Alert creation, resolution, and severity changes also invalidate jobs and partner/technician summaries. JobOfferSummary and JobHistorySnapshot keep severity=null and do not expose the existence or severity of private Alerts. Example: resolved critical plus open warning on the same Unit gives warning; adding acknowledged critical gives critical; resolving all gives normal. These changes do not change offer/history filter results.

## IR31 Report collaboration and self-approval

Internally, the Repository saves contributorUserIds (a set of unique userIds) for each reportId/reportVersion. Public WorkReport.authorId keeps the original author, and item authorId continues to follow SR07. Do not replace self-approval checks with a comparison of a single authorId. Do not accept contributors from the UI or add this internal set to external DTOs.

Add the initial draft creator to the set. When creating a new content version, inherit the previous version's set and add the userId executing any actual addition/change/deletion of body text, inspection items, measurements, parts, next actions, or photo references. Include people who add photos through attachments.add. Copies after reassignment or from on_hold/rework also inherit the source version's set. Reads, assignment only, saves with no changes at all, submission only, and acceptance/return only add no new contributor. Idempotent retries do not grow the set or content version. Freeze the set when the version is submitted; do not change past versions later.

jobs.review reads the set for the submitted target version. If it includes current Session.userId, both accept and return yield FORBIDDEN with zero side effects (only the D01 denial audit). Apply the same check to another Membership, partner.review, normal HQ review, and hq_escalation. Do not rewrite original authors, item authors, or contributor history to obtain another person's approval. If a seed/old version lacks the set, do not assume no contribution: reject review with UNAVAILABLE and fix the fixture. Seed reports also require a contributor set consistent with their creation history.

Reproduction: Reassign T1's draft to T2. In each case where T2 changes only the body, only measurements, or only photos and submits, the set contains T1/T2. Switching T2 to a quality role in another Membership or to HQ still returns FORBIDDEN. An active T3 who did not create/edit the report may accept it if other authorization/state conditions hold. If T2 only assigns, reads, and submits without changing content, do not add T2 as a contributor or reject under self-approval rules (normal quality-role permission is still separately required).


IR31 UI projection: reports.get and write/retry responses returning WorkReport compose reviewAvailability for the current viewer. Do not change the original report body, content version, or contributor set. Check in this order: no partner.review or normal/handoff HQ review eligibility for the current job → permission_denied; current userId is a contributor to the target version → self_authored; target is not the latest report version → not_current; Job.status is not submitted or the target version is unsubmitted → not_submitted; all pass → allowed=true/reason=null. If read scope itself is absent, return NOT_FOUND without a DTO as before. Eligibility also includes current delegation, affiliation, scope, and existing review conditions other than self-approval. Keep separate submission-time validation of handoff mode, reason, and input/options versions.

On P05/A06, disable accept/return buttons when the fetched reviewAvailability.allowed=false and display the translated reason. Do not determine button availability from original author ID alone. Reassignment, report revisions, and permission changes invalidate the Query. Changes during confirmation require refetching and repeating confirmation. The Repository does not trust UI availability sent by the client; it rechecks IR31 when jobs.review runs. Missing contributorUserIds returns UNAVAILABLE; do not fall back to permission granted.


## IR32 Equipment-set filters for job lists and summaries

jobs.list also accepts the same unitIds as summaries.get(kind=partner/technician). Apply shared Query constraints and include only rows whose summary.unitId after authorization and IR23 projection is in the array. An empty array gives zero results. If unitId is also specified, combine with AND. offer/history have no public unitId, so they do not match even a nonempty array. Do not match against the private Job equipment ID. P01/T01 sends identical shared filter values to both reads. Do not silently discard unallowed filters specific to a summary or list.

## IR33 Audit screen read paths and search boundary

A16 requires audit.list as a Query; do not show a failure as a normal empty result. Device events have a separate supporting panel that obtains authorized candidates from devices.list. Call devices.events({id:deviceId,query}) only after deviceId selection. With no selection, show guidance; with no candidates, show empty; on list failure, show retry within the panel. An invalid or invisible selected ID stops with not-found; do not substitute another device. Keep selected deviceId in the URL and restore it with the same steps on Back/Forward. audit.read candidate/history read permissions follow the existing operation catalog; do not additionally grant device.write.

When navigating from an audit targetRef to an existing detail screen, use /admin/jobs?jobId=:id for kind=job, /admin/restrictions/:id for restriction, and /admin/devices?deviceId=:id for device, each requiring its existing read permission. For command, fetch commands.get separately only with its existing permission, then use its unitId to navigate to /admin/units?unitId=:unitId. A16 audit.read alone does not expand access to other resources. Resolve links through shared Navigation/feature hooks. Without permission or for an unknown kind, show only masked audit details. Deleted/expired resources show not-found at the destination; retain the original audit.

audit.list applies every filter to the authorized set. Both another tenant's correlationId and a nonexistent correlationId return items=[]/total=0/nextCursor=null. Do not search all tenants first and branch to NOT_FOUND. This differs from D01 NOT_FOUND for individual resource get operations. Only internal Repository business events add audit records; A16 has no write operation.


## IR34 User-confirmed permissions and job sorting

DEC-17: Restriction operations have two permission types. defer/exempt/cancel require restriction.write; override requires restriction.override. Holding both allows both sets of operations, but neither implicitly grants the other. Keep existing manage conditions for schedule/execute/release and others, and IR03 conditions permitting only release tracking after override. Check both menus/buttons and the Repository; do not skip reason, scope, state, or version validation. A direct write without permission returns FORBIDDEN, makes zero business changes, and creates only the D01 denial audit. This fixes the FR-A10 inconsistency between list and detail; it adds no new permission.

DEC-18: Job lists offer sorting by status (business order), severity, and deadline, with ascending/descending directions. The default is status asc, with ties always broken by id/jobId ASCII asc. Fix status comparison order to these 10 states.

requested → offered → accepted → assigned → in_progress → on_hold → submitted → rework_requested → completed → cancelled

This is display order, not a table of allowed state transitions. status desc reverses that order. IDs within the same status remain asc even for desc. severity is normal < warning < critical; compare dueAt as UTC time, with null last in either direction. Do not sort by translated labels or only within the current page. Use authorization → IR23 public projection → filter → sort the whole snapshot → pagination. Use the public offer status and frozen history status, without reading private Job status. Reject unknown status with D01 UNAVAILABLE.

Expose JOB_STATUS_ORDER in the canonical types. This is a demo contract constant, not application implementation. The Repository default when sort is omitted is also status asc;id asc. Keep existing id sorting as an internal contract, but offer users only status/severity/dueAt. Apply this to every job list using jobs.list. Job candidate reads also use this default when sort is omitted.

The UI URL key sort uses field:direction (for example, sort=status:asc or sort=dueAt:desc). Omission means status:asc. Invalid fields/directions, duplicate sort keys, or empty values return VALIDATION and request condition correction. Reject id in the URL because it is outside UI choices. A selection change keeps other filters, updates the URL, removes the cursor, and fetches the first page under the new conditions. Back/Forward restores that URL's sort/filter; do not reuse an old cursor with new conditions. Include normalized sort in the Query key so a late response for old conditions cannot overwrite the new list. Subscription refetches keep the current sort selection. Language changes do not change ranks, IDs, or UTC values.

Label sort controls and make them selectable with a keyboard and on mobile. After selection, show loading for the new conditions; failures show error/retry and zero results show empty. Do not present rows in the old order as fetched in the new order. Reflect aria-sort in the corresponding column header. "Reset to default" resets only sort to status:asc and refetches from the first page, keeping other filters. KPIs do not depend on sort; do not pass sort to summaries.get.

## IR35 Release-request triggers and restrictions.release preconditions — FRV-001

Release requests (state=release_requested) may start only through routes ①–③ below and route ④ cancellation (restrictions.cancel) in IR96. All use the same internal transition function. ① Payment confirmation: in the same transition where payments.confirm/payments.recordManual/payments.simulate(confirm) makes all cause invoices (causeInvoiceIds) paid, move scheduled to cancelled and requested/applied to release_requested, saving releaseIntent={source:'payment',at:now,actorMembershipId}. ② Grace/exception: in the same transition where restrictions.defer/exempt runs on requested/applied, move to release_requested with source='exception'. ③ Forced release: restrictions.override uses source='override'.

In the same transition to release_requested, perform the D03 release evaluation once per perUnit. For online equipment with applyState=applied, create a remove_restriction Command and set releaseState=requested. not_sent/not_applied becomes not_required; sent_unknown becomes waiting_reconcile. Offline applied equipment uses releaseState=none/pendingReason=offline, not failed, and waits for explicit retry(phase=release). Create release Commands even when the caller is a client (Command.actorMembershipId is the Repository's internal actor 'system-restriction'; the audit actor is the person performing payment confirmation).

`restrictions.release` is an explicit release-request operation by a restriction.write holder. Preconditions are state∈{requested,applied,release_requested} and either "all cause invoices are paid" or "exception/grace is active". Unpaid with no grace/exception returns FORBIDDEN (contract restriction violation). From requested/applied, perform the same transition as ② above with source='manual'. If already release_requested, idempotently return the current Restriction without adding Commands, audits (except denial audits), or versions. Only restrictions.retry(phase=release) retries or requests failed equipment again. "release" in AT-A09-N④ is this idempotent response; the transition to release_requested itself occurs during payment confirmation. AT-C11-N④ checks that client payment confirmation alone starts ①.

## IR36 Demo clock progression and session lifetime — FRV-002

On reset/reload, the demo clock starts from fixture.clock (2026-09-14T01:00:00.000Z) and advances monotonically at real-time speed. If to is at or after the current clock, demo.advanceClock({to}) jumps forward and processes deadlines reached during the jump (Command expiry, Offer/Assignment/qualification expiry, staleAfterSeconds, Fact TTL, predicted occurrence, DiagnosticRun endAt, cooldown) in time order using D04 event priority. If to is before the current clock, allow it as "initial time setting" only immediately after reset while business events have not advanced beyond the seed eventCursor; otherwise return VALIDATION (AT-C04-E③ performs this setting immediately after reset).

The 30-minute Session lifetime is measured with the demo clock, but advanceClock jumps do not consume it. When a jump commits, shift issuedAt/expiresAt of active Sessions by the same offset (also for initial time setting). User-triggered extension is only through IR55 demoSession.extend (IR79). Test session expiry through demo.trigger(session_expired) or elapsed time without jumps. Membership.validUntil, Offers, Assignments, qualifications, contracts, and invoice deadlines expire normally through jumps. Queries displaying resources expired by a jump increment viewEpoch and are discarded under IR24.

## IR37 Transport failure injection and network disconnection — FRV-003

Add eventType='transport' to DemoTrigger. Input is `{operation:OperationName, outcome:'UNAVAILABLE'|'TIMEOUT'|'RATE_LIMITED'|'DELAY', retryAfterSeconds:number|null, delayMs:number|null, remainingCalls:number}`. remainingCalls is an integer from 1–100; apply the outcome to the next remainingCalls calls of that operation and decrease it on each application. For outcome=UNAVAILABLE/TIMEOUT, the Repository rejects with DomainError{code} without running business processing (writes have zero side effects and do not record an idempotency key). RATE_LIMITED requires retryAfterSeconds (1–3600) and stops acceptance under D04. DELAY waits delayMs (1–60000) before normal processing. Use this path to inject D10 intentional slow responses (3000 ms) and the 10-second timeout display (12000 ms). Acceptance conditions saying "make units.list UNAVAILABLE", "jobs.create UNAVAILABLE", or "submit UNAVAILABLE" refer to this injection. Automatic UNAVAILABLE retries on reads consume remainingCalls, so use remainingCalls>=3 when checking the final error display, as in AT-C01-E③.

DemoTrigger network(connected=false) simulates network disconnection. Reject all Repository operations (except demoSession/preferences/demo.*) with DomainError{code:'UNAVAILABLE', messageKey:'errors.network_disconnected', retryAfterSeconds:null}, without running business processing. events.subscribe delivers no events while disconnected. The UI unsubscribes and shows "Updates stopped" and the last successful time (screen state offline). On reconnection with connected=true, the UI recovers through the D07 new snapshot → resubscribe sequence. Repository events committed during disconnection are reflected through replay. Separate network disconnection and device offline (code OFFLINE) in display areas and wording. Commands/deadlines that expire while disconnected still expire according to the clock.

## IR38 Deadline derivation for customer-created jobs — FRV-004

Only job.write holders (HQ) may specify jobs.create dueAt. A client sending dueAt receives VALIDATION (fieldErrors.dueAt). If omitted, the Repository saves dueAt=requestedEnd (the same rule as D16 generated jobs). When HQ specifies it, check dueAt>=requestedEnd; a lower value returns VALIDATION. 1A has no later dueAt change operation (use a new job). JobSummary.dueAt, overdueOnly, dueAt sorting, and technician/partner overdueCount use only this saved value.

## IR39 Visibility of archived resources — FRV-005

Exclude Property/Space/ACUnit with archived=true from all lists (properties/spaces/units.list), summaries.get/admin.summary counts/denominators/KPIs, candidate selection (through units.list), notification recipient resolution, and scope checks for new business operations. 1A offers no archived filter; exclusion is unconditional. Individual reads (units.get and space/property selection resolution) return NOT_FOUND to clients/contractors/technicians. Only HQ asset.write holders receive a read-only DTO with archived:true. Control, assignment, contract, restriction, and Device operations return CONFLICT (reason archived). Unit references from existing Job/Report/Audit/Command history permit history access under D05, and history screens show an archived label. In the same archive transition, invalidate units/properties/spaces/summaries/automatic-operation Queries. Archived IDs in a unitIds filter do not match and count as zero results.

## IR40 Population for customer counts — FRV-006

Customer (service registry) and Organization(kind=customer) are one-to-one in 1A. customers.save returns CONFLICT for a second record with the same organizationId, and checks that organizationId has kind=customer and is within management scope. AdminSummary.customerCount counts Customers with Customer.status=active and corresponding Organization.status=active (with filters.customerId, return 0/1 depending on whether that one record qualifies). Do not exclude inactive customers' equipment, jobs, or invoices from KPIs, but do not count a customer if either Customer or Organization is inactive. Use this definition for "active customers" in AT-A01-N/B.

## IR41 Time-series presets 1h/24h — FRV-007

DD-C07 and DD-T03 period presets are 1h/24h/7d/custom. As in SR17, to is the demo clock rounded down to the UTC minute boundary. 1h is [to-60 minutes,to); 24h is [to-1440 minutes,to). Both are fixed-length moving windows, not calendar days. 7d follows SR17 calendar-day rules. custom requires from<to and at most 366 days. Reproduce preset labels using IR17 viewEpoch and from/to saved in the URL; recalculate the selected preset on explicit refresh. Only today has from=to in the first minute of a day; 1h/24h are always nonempty. today/7d/30d on C01/C06/A01/A13 do not change SR17.

## IR42 Private fields in role-specific projections — FRV-008/FRV-009

Add these projection rules to read and write responses. Client JobDetail: offer=null; assignment={id,version,jobId,technicianMembershipId,scheduledStart,scheduledEnd,status,validFrom,validUntil,reason:null}; costs include only visibility=customer; draftReportRef=null; reportRefs include only accepted versions; contactWindow is the client's own input. Technician JobDetail: offer=null, costs=[], and assignment.reason only for their own assignment. Contractor JobDetail: only their own company's Offer (no other company's Offer/declineReason), costs=[], and assignment.reason for their own company's assignments. HQ receives all fields.

jobs.events: For clients, remove events with note.visibility=internal from the array (remove the whole row, rather than setting note=null). total is also the count after exclusion. Contractors receive both internal/customer events for their own company's jobs; technicians receive both for currently assigned jobs. Do not reveal excluded events through counts or cursors. Display JobNote body as text nodes (D08).

Client RestrictionDetail (forInvoice/notification links): Limit events[] to state-change events (action∈{scheduled,requested,applied,release_requested,released,cancelled,defer,exempt}), with actorId='masked', actorRoleAtTime=admin, reason=null, and maskedBefore/After={}. exception.reason is null; return graceUntil/exception.until. Keep perUnit unchanged. HQ receives all fields. Restriction.reason (advance notice reason) is customer-facing text entered by HQ; clearly label the DD-A09 input "Shown to the customer".

Contractor Membership projection for members.list/members.eligible/members.capacity: permissions=[], scopes=[], qualifications only for their own company's technicians, and validFrom/validUntil are returned. HQ receives all fields.

## IR43 Sensor creation on Device registration — FRV-010

devices.register sensorTypes is a Metric array without duplicates and may be empty (device-no-sensor). Each metric must have a definition with the same metric in the target Unit's Capability.sensors; otherwise return VALIDATION (fieldErrors.sensorTypes). Create Sensors on register, copying unit/staleAfterSeconds/boundaryId from the matching Capability.sensors definition. The Repository assigns the id; calibratedAt=null. When bind connects to another Unit, issue new sensorIds under SR24 and apply the same copying rules to the destination capabilities. Do not accept Telemetry for metrics absent from the capabilities (a missing sensor in demo.trigger telemetry returns NOT_FOUND). Initial firmwareVersion is the first target-capability firmwareCandidates value in ASCII ascending order, or 'unknown' if there are no candidates.

## IR44 Formatting, translation, rendering exceptions, and notes — FRV-019–025

- Intl locale tags are en → 'en-MY' and ms → 'ms-MY'. Format amounts with the numeric part from Intl.NumberFormat(tag,{minimumFractionDigits:2,maximumFractionDigits:2}), followed by one ASCII space and the currency code, as "120.00 MYR" (do not use currency symbols or currencyDisplay). Temperature has one decimal place ("24.0°C"); humidity/percentages, energy, and kgCO₂e have one decimal place; ppm/µg/m³ use integers. Round decimal values half away from zero; do not use Number.prototype.toFixed for rounding. Numbers in acceptance conditions (such as 24°C) specify values; displays follow these decimal-place rules. Show dates/times with Intl.DateTimeFormat(tag,{timeZone:Preferences.timezone, dateStyle:'medium', timeStyle:'short'}) and the timezone abbreviation.
- If a translation key is missing from the ms dictionary, fall back to en and emit console.warn in development builds. If missing from both, display the key string. NFR-07 lint checks that en/ms key sets match; a mismatch fails the build.
- Catch uncaught rendering exceptions with the shared ErrorBoundary and show a full-screen error with correlationId (never a blank screen). Retry remounts the same route; continued failure offers navigation to the role home. AsyncBoundary continues to handle Repository DomainError.
- Do not show VoicePanel to roles without voice.resolveIntent permission (contractors and HQ/technicians without permission). Disable the header voice toggle and show the reason (voice.unavailable_for_role).
- Summary.counts returns 0 for counters not applicable to the kind (offer/active/review/scheduled/inProgress/overdue/assigned for customers, powerOn/powerOff/powerUnknown/alertCount and others for partners/technicians), and the UI does not display them. Do not use null.
- demo.trigger.scenarioId is an arbitrary label of 1–64 characters. It is recorded only in DemoEvent.type and the audit and does not change behavior. Scenario names S01–S08 are recommended.
- units.save.installedAt accepts Instant|null. Save null as "Not registered"; future dates return VALIDATION. Existing equipment with no registered date may be edited and saved with null unchanged.

## IR45 Synthetic telemetry and heartbeat generation — REV18-001

The demo clock advances in real time (IR36). The Repository has a heartbeat simulator. While enabled (enabled=true immediately after reset/reload), whenever the demo clock reaches time t at a UTC minute boundary (seconds and milliseconds both 0), perform the following in one transition for each nonarchived Unit with a currently bound Device (Device.unitId=Unit.id), only if Device.connection=online and Device.powerSignal≠off.

1. Device.lastSeenAt=t. Under IR47, ACUnit.lastSeenAt becomes the same value.
2. For each Device Sensor, only if the latest Measurement with the same sensorId (first by observedAt descending, sequence descending, id ascending) meets the IR77 copying conditions (origin=measured, quality=valid, value≠null), save one new Measurement copying its value and unit. Set observedAt=receivedAt=t, origin=measured, quality=valid, qualityReason=null, rawUnit=null, and sequence=source+1. Generate nothing for Sensors that fail the conditions or have no past Measurement. Do not use random numbers.
3. ACUnit.observedState.observedAt=t. Do not change power/celsius/mode/fanLevel (only Command ack changes them).

Device.lastSeenAt, ACUnit.lastSeenAt, and observedState.observedAt changed by generation are observation-time fields; they do not change Device/ACUnit version or updatedAt. Thus generation does not invalidate a user's fetched expectedUnitVersion. Notify generated Measurements as IR71 measurement/device events. Generate nothing at the reset/reload time itself; start at the next minute boundary.

Units with a connection other than online, powerSignal=off, no binding, or no Sensors generate nothing, and become stale/unknown under D07/SR27. Determine disconnection only from an explicit device event (communication_lost); do not automatically set offline based on elapsed time since lastSeenAt.

A demo.advanceClock jump does not fill in intermediate minute boundaries. If the destination is a minute boundary, generate once at that time; otherwise wait until the next minute boundary. Unfilled intervals lower coverage as missing D07 slots.

Add `{eventType:'simulator';enabled:boolean}` to DemoTrigger. enabled=false stops future generation; true resumes at the next minute boundary. Acceptance tests that fix measurements and observation times in Given must start with simulator enabled=false. Leave enabled=true only for cases testing automatic updates themselves. Displayed screens reevaluate stale state, deadlines, and remaining seconds on demo clock 1-second ticks; ticks alone do not refetch Queries.

## IR46 Allowed operations during restrictions — REV18-002

While UnitDetail.effectiveControlPolicy.state=restricted (phase=requested/applied/release_requested), determine UnitAction availability only from the table below. unrestricted (scheduled while planned or under grace/exception, released, or after not_required is confirmed for that Unit) does not reject actions because of restrictions.

| UnitAction | temperature_limit (minimumCoolingSetpoint=m) | power_off |
|---|---|---|
| set_power power=true | Allowed | FORBIDDEN |
| set_power power=false | Allowed | Allowed |
| set_temperature celsius=c | Allowed if c>=m; FORBIDDEN if c<m | FORBIDDEN |
| set_mode | Allowed | FORBIDDEN |
| set_fan | Allowed | FORBIDDEN |
| ventilate | Allowed | FORBIDDEN |

Apply this to all paths: client/HQ/technician commands.create, voice change confirmation, automations (mark Command candidates suppressed/restricted), diagnosticRuns.create (FORBIDDEN if either startAction or endAction is prohibited), and rechecking at trial-run end (end_blocked if prohibited). Even HQ with restriction.override cannot bypass this through commands.create; release only through restrictions.override. FORBIDDEN has D01 priority 4, messageKey=errors.restriction_active, and fieldErrors.action. CommandPanel disables candidates and shows reasons using the same table. The temperature input minimum is max(capability.temperature.min, m).

## IR47 Derive equipment connection, power, and removal states — REV18-003

ACUnit.connection and ACUnit.lastSeenAt are Repository-derived values, not units.save inputs. If a currently bound Device exists, copy its connection/lastSeenAt; otherwise use connection=unknown and lastSeenAt=null. In a transition changing Device connection/powerSignal/tamper, also increment ACUnit.version and notify under IR71. Changes only to lastSeenAt (including IR45 heartbeats) do not change version or updatedAt.

Control acceptance (commands.create, diagnosticRuns.create, voice change confirmation, and delivery checks for restrictions.execute/retry(apply)) permits delivery only when derived connection=online and Device.powerSignal≠off. Otherwise return OFFLINE, with messageKey one of errors.device_offline/errors.device_unknown/errors.device_connecting/errors.device_error/errors.device_power_lost (powerSignal=off takes priority, then the connection value). Treat restriction apply as an undelivered intent (not_sent) under D03.

Device.tamper=detected does not affect control availability. Show a removal warning in CommandPanel and UnitDetail. Update ObservedState.power only through Command ack and telemetry; do not rewrite it from powerSignal. The UI shows "Power signal lost" separately from connection state. summaries.get/admin.summary counts online/offline/unknown from this derived connection; connecting/error count as unknown under D14.

## IR48 Unanswered Offer expiry — REV18-004

jobs.offer accepts only Job.status=requested; other states return CONFLICT. Inputs must satisfy now<offerExpiresAt, accessValidFrom<accessValidUntil, and offerExpiresAt<=accessValidUntil; otherwise return VALIDATION.

When the demo clock reaches Offer.offerExpiresAt with decision=null (at D04 deadline-expiry priority), in one transition return Job.status from offered to requested, set contractorOrgId=null, increment Job.version by 1, and save one JobEvent (action=offer_expired, actorUserId=system-demo). Keep Offer.decision=null and derive expiry from now>=offerExpiresAt. Generate no notification. Exclusion from contractor lists and CONFLICT for accept/decline follow IR23/AT-P02-E. HQ may run jobs.offer again with a new offerId on a job returned to requested (the same contractor is allowed).

## IR49 Technician job viewing and work windows — REV18-005

For each active Assignment, the viewing window is [Assignment.createdAt, scheduledEnd), and the work window is [scheduledStart, scheduledEnd). Completion releases the Assignment and ends both windows at that moment (IR234, DEC-73). D06's statement that "Assignment validFrom/Until equals scheduledStart/End" refers to the work window.

For the assigned technician within the viewing window, jobs.list/get returns JobSummary/JobDetail (IR42 projection), summaries.get(kind=technician) includes it in assignedCount and assigned-equipment counts, and notification links and /technician/jobs/:id may be shown. Before the work window starts, these two groups of operations return FORBIDDEN (messageKey=errors.assignment_not_started). (a) Job-based operations for both internal and external technicians: units.get with jobId, jobs.start, jobs.saveDraft, jobs.submit, attachments.add, commands.create with jobId, diagnosticRuns.create, and devices.* writes with jobId. (b) External technicians' equipment-related reads: units.get, telemetry.*, alerts.*, devices.* (SR03 limits external technician access to the intersection with their own Assignment). Internal technicians' reads using unit scope without jobId are outside (b) and are allowed under SR03 scope. The UI shows the work start time and "You can operate from the start time", disabling the relevant equipment links and operations. After the work window ends, return only IR23/IR24 history.

External technicians' viewing and work windows are further intersected with the Offer access window. Internal technicians' equipment reads through unit scope follow SR03, but job-linked writes outside the work window return FORBIDDEN. Extend a work window by creating a new Assignment through jobs.assign (the same technicianMembershipId is allowed) and marking the old Assignment revoked (a reason is required during in_progress). validUntil=2026-09-20 in AT-T01-N means scheduledEnd=2026-09-20T00:00:00.000Z.

## IR50 KPI list links and allowed URL keys — REV18-006

Each Screen's allowed URL keys are the union of screen-catalog.url_selection and shared keys {tab, sort, period, from, to}. Interpret a shared key only if the Screen has a corresponding input; otherwise remove it under SR11. Multiple values (unitIds, connections) are comma-separated and normalized by deduplication and ASCII ascending order. Booleans accept only the strings true/false.

Customer operation KPIs link to `/customer/properties?propertyId=:propertyId&powerState=on|off|unknown` (only powerState if no propertyId is selected). SCR-C02 has an equipment-list section showing units.list for the selected property (all properties if none is selected), filtered by powerState and connections. The connection KPI link uses `connections=connecting,error,unknown`. HQ operation KPIs link to `/admin/units?customerId=:customerId&propertyId=:propertyId&powerState=on|off|unknown`. Neither passes period (IR13). Back restores the source URL (D13).

## IR51 Population for unresolved alert counts — REV18-007

Summary.counts.alertCount (kind=customer), AdminSummary.alertCount, and UnitSummary.activeAlertCount count Alerts after projection and scope checks, excluding archived Units, with status∈{open, acknowledged} and severity∈{critical, warning}. Do not count severity=normal Alerts (health summaries). IR30 JobSummary.severity still uses the maximum including normal. "Unresolved count" in FR-C08/AT-C08-B means this alertCount. Display it separately from unread notifications (the total from notifications.list with unreadOnly=true).

## IR52 Lifestyle pattern condition evaluation — REV18-008

Condition `{type:'pattern', localTime:'HH:mm'}` is a synthetic condition that matches once daily when the time in the rule's timezone equals localTime. It uses no Fact. When the demo clock reaches that local-time tick (seconds and milliseconds 0), the Repository performs D02 internal evaluation (phase=condition). Do not fire missed occurrences passed by a jump (IR36). Fire if the jump destination exactly matches the time. UI automations.simulate/fire matches only when occurredAt local time matches; otherwise return suppressed/no_match. Apply D09 366-day DST validation on save; nonexistent/ambiguous times return VALIDATION. Label the screen "Demo lifestyle pattern (fixed daily time)"; do not describe it as learned or inferred. Location consent is not required.

## IR53 Consent withdrawal and rule state — REV18-009

In the same consents.update(granted=false) transition, set enabled=false and disabledReason=consent_revoked on all Automations with the same ownerMembershipId, kind=event, condition.type=location, and enabled=true. Increment each version by 1 and notify automations. Add consent_revoked to RuleBase.disabledReason. Renewed consent (granted=true) does not automatically enable rules. Reset disabledReason=null only when the user explicitly saves enabled=true successfully (IR27). Evaluation marks enabled=false rules as suppressed/disabled. Use DecisionReason=consent_revoked only for location rules that have granted=false and enabled=true at evaluation time.

## IR54 Schedule and lifestyle pattern firing paths — REV18-010

The only required path for demo-clock schedule (schedule_start/schedule_end) and pattern-condition firing is D02 internal Repository evaluation, independent of the logged-in role or displayed screen. The UI does not call automations.fire for these triggers. automations.fire is an optional operation that immediately evaluates synthetic facts for the current tick from the C04/C05 event tab and A11/A12 "Fire demo event". If internal evaluation and UI fire overlap at the same tick, reference the D02 same-tick result and do not create Commands again. This replaces the old DD-C04 statement "Pass events from the demo clock to automations.fire".

## IR55 Session expiry warning and extension — REV18-011

Show SessionExpiryDialog (role=alertdialog) 120 seconds before Session.expiresAt (demo clock). Buttons are "Extend" (initial focus) and "Sign out". Extension uses `demoSession.extend` (input={}, result=Session, write, demo-only), setting expiresAt=now+30 minutes for an active Session (now<expiresAt). Do not change issuedAt/generation/viewEpoch; there is no limit on extensions. Calls after expiry return UNAUTHENTICATED. This is outside business audits and writes.getResult (D14 local boundary). The dialog changes no business data or input values. If expiry occurs while it is open, discard under D09, navigate to /login, and notify that the unsaved draft was discarded. advanceClock jumps also shift expiresAt by the same offset under IR36, so they do not skip the warning. This addresses WCAG 2.2 SC 2.2.1 (extension mechanism and at least 20 seconds of warning), without claiming conformance. This replaces D09's "No extension through user action".

## IR56 States allowing job cancellation — REV18-012

Determine jobs.cancel availability only from this table. cancelReason is required as 1–1000 characters after trim. Contractors and technicians do not have jobs.cancel (FORBIDDEN).

| Current Job.status | Client (own job) | HQ job.write | Result on success |
|---|---|---|---|
| requested | Allowed | Allowed | cancelled |
| offered | Not allowed | Allowed | cancelled. Remove undecided Offers from contractor lists; later accept/decline returns CONFLICT |
| accepted / assigned | Not allowed | Allowed | cancelled. Mark related Assignments revoked |
| in_progress / submitted | Not allowed | Not allowed (first use jobs.hold to set on_hold) | CONFLICT |
| on_hold / rework_requested | Not allowed | Allowed | cancelled. Keep incomplete records |
| completed / cancelled | Not allowed | Not allowed | CONFLICT |

"Not allowed" for clients returns CONFLICT as a state mismatch at D01 priority 6. "Not allowed" for HQ also returns CONFLICT.

## IR57 FORBIDDEN/NOT_FOUND display and navigation — REV18-013

Replace the route with `/forbidden` only when the route guard detects a role prefix different from the user's role. Unauthenticated access is UNAUTHENTICATED and navigates to /login. If a Screen's primary query returns FORBIDDEN, keep the URL and show permission-denied in place (with a role-home link). For NOT_FOUND, show not-found in place (with a parent-list link). Do not navigate automatically to another screen. Secondary queries show the same state only in their panel. If a write returns FORBIDDEN/NOT_FOUND, do not navigate; keep inputs, show an error summary, messageKey, and correlation ID above the form, and invalidate the target primary query. If refetch returns FORBIDDEN/NOT_FOUND, enter the primary-query state described above. However, technician messageKey=errors.assignment_not_started uses the IR76 work-not-started state. This replaces "Return to an available screen" and "Go to the list screen" in the DDC-03 table and role-specific DD.

## IR58 Notification list visibility — REV18-014

notifications.list returns only notifications whose recipientMembershipId is the current Membership and whose target is readable in current scope. Exclude notifications for unreadable targets from items, total, and unread counts; do not return masked rows. Keep the notifications saved; they reappear if access is restored. If the target cannot be resolved when opening a notification link from a displayed list or URL, show "Unavailable" without the target name or values. The "Unavailable" display in FR-C08 refers to this link-resolution step.

## IR59 One payment-finalization path and system actors — REV18-015

Customer payment-attempt processing/confirm/fail occurs only through payments.simulate. Remove the payment branch from DemoTrigger; /demo does not generate payment events. HQ payment confirmation uses only payments.confirm/recordManual. When demo.trigger, demo.advanceClock, reset, or clock-driven internal transitions (IR45/IR48/IR52/IR54 and expiry) change business records, use audit actorId=system-demo, actorRoleAtTime=system, reason=null, and correlationId=source DemoEvent.eventId or clock tick ID (`tick-<ISO time>`). IR35 release Commands use actorMembershipId=system-restriction and audit actorRoleAtTime=system. AuditView.actorRoleAtTime has type Role|'system'; the UI displays "System (demo)". Audit granularity when starting release requests follows IR90 (REV19-033).

## IR60 Identify demo-only operations — REV18-016

operation-catalog.frontend_execution has three values: mock-service (candidate for the same business interface in a future production adapter), local-preference, and demo-only (1A-only, not ported to the production adapter). The 11 demo-only operations are demo.advanceClock, demo.reset, demo.trigger, demoSession.signIn, demoSession.signOut, demoSession.switchMembership, demoSession.extend, auth.previewPasswordReset, payments.simulate, offsets.simulate, and automations.fire. UI areas calling demo-only operations always show a "DEMO" label. Include their replacements (real authentication, real password reset, payment-provider integration, and device-event reception) in D11 required production deliverables.

## IR61 Non-RTO contract display — REV18-017

C10 shows "No contract / General maintenance" and a monitoring-screen link only for customers with zero contracts. general/energy/environment contracts show their type, period, and invoices; do not label them "No contract". Align the FR-C10 post-completion business-state description with this section. customer-b in AT-C10-N④ has zero contracts in demoSeed (IR69).

## IR62 Equipment without a Space assignment — REV18-018

units.save spaceId is ID|null; null means directly under the Property (no Space assignment). Validate consistency with propertyId only. spaces.archive still returns CONFLICT if directly assigned Units exist; do not automatically move them to unassigned. Add unassignedOnly (boolean) to units.list filters. true returns only Units with spaceId=null (combining it with spaceId returns VALIDATION). The C02 tree shows an "Equipment without a Space" group directly under the Property; its count uses Page.total with unassignedOnly=true.

## IR63 Admin dashboard energy-saving summary — REV18-019

Return AdminSummary.energySummary to dashboard.read holders. Targets are nonarchived Units matching filters (customerId/propertyId), the period is [from,to), and rates/factors use D07/SR09 defaults. In 0.19.0, automatic baseline selection and reduction calculation moved to IR78 `energyForecast` (forecast using a prorated assumed baseline), and energySummary reduction fields became null. Display wording and links also follow IR78.

## IR64 Contact-window input and visibility — REV18-020

contactWindow is 0–200 code points after trim. If it contains '@', or if removing whitespace, hyphens, parentheses, and '+' leaves at least seven consecutive digits, return VALIDATION (fieldErrors.contactWindow, messageKey=errors.contact_details_forbidden). Return JobDetail.contactWindow only to the client (own job), HQ job.write, the assigned technician within the viewing window (IR49), and the accepted contractor within the access window. Other projections use null. Do not include it in JobOfferSummary, JobHistorySnapshot, or notification params. Complete detection in free text is not guaranteed (DDC-09). Input guidance and handling of time notation follow IR90 (REV19-017).

## IR65 Voice target matching and syntax — REV18-021

After trim, match grammar case-insensitively for ASCII with these regular expressions. en: `^temperature (.+)$`, `^set (.+) to (\d{1,3}) degrees$`, `^help$`. ms: `^suhu (.+)$`, `^tetapkan (.+) kepada (\d{1,3}) darjah$`, `^bantuan$`. `(.+)` is greedy. Match `<room>` exactly against nonarchived Space.name in current scope, after trim and ignoring ASCII case, regardless of kind. Do not match ACUnit.displayName. Out-of-scope Space names count as zero matches (D09 target absent). Candidates are nonarchived Units directly assigned to the matched Space. One candidate returns temperature/change; two or more candidates or multiple matching Spaces return candidates (pathLabel=property name > ancestor Space names > Space name > Unit display name); zero candidates returns unsupported. Confirm selectedUnitId only if it belongs to the immediately preceding candidate set; otherwise return NOT_FOUND.

## IR66 Resolve policy-free Alerts and link recurrences — REV18-022

D08 automatic resolution after sustained recovery applies only to Alerts with policyId≠null. Alerts with policyId=null (seed, inferred/inspection evidence, tamper, maintenance, reconciliation_required, device faults) resolve only through manual resolve by an alert.resolve holder (reason and evidence ID required). When creating an Alert, use an incident key with the same unitId and, for policyId≠null, the same policyId; for policyId=null, the same type/causeCode pair. If an open/acknowledged Alert exists with that key, keep it and create no new Alert; severity escalation produces only the D08 notification. If the latest Alert with that key is resolved, set previousAlertId to its ID. AT-T07-N tests recovery after remeasurement using an Alert with policyId.

## IR67 Device operation start and connection-check states — REV18-023

Create devices.check and devices.updateFirmware with status=queued. At the demo clock tick one second after creation, recheck mutual exclusion and set running/startedAt. check sets Device.connection=connecting at this point. updateFirmware sets failed/failureCode=OFFLINE if connection≠online, without changing connection. demo.trigger(operation) accepts only running operations; results for queued operations return CONFLICT. Successful check sets connection=online; failed check sets connection=error. Successful FW update sets firmwareVersion=targetVersion; failure changes neither version nor connection. At 60 seconds after creation, set failed/failureCode=TIMEOUT (D05); check TIMEOUT sets connection=error. devices.calibrate succeeds in the creation transition without queued/running. Recheck failures and overlap with restriction Commands follow IR90 (REV19-018).

## IR68 Display negative savings — REV18-024

DTOs return signed values (savedKWh=baseline−actual, savingPercentage=savedKWh÷baseline×100; savedAmountMinor and savedEmissionsKg use the same sign). Use one formatter for all roles: values>0 show "Reduction {absolute value}", values<0 show "Increase {absolute value}", values=0 show "No change 0.0", and null shows "Cannot calculate". Decimal places and rounding follow IR44. Example: baseline 100 kWh and actual 120 kWh gives DTO savedKWh=-20 and savingPercentage=-20, displayed as "Increase 20.0 kWh" and "Increase 20.0%". C06/C13/A13/A14 use the same formatter. A01 forecasts follow IR78 "Expected reduction / Expected increase" wording and null display. "-20 kWh / -20%" in FR-A13/DD-A13 refers to DTO values.

## IR69 Demo seed and test fixture overrides — REV18-026

Initial business data uses demoSeed in [fixture-contract.json](../04-agentic-sdlc/fixture-contract.json) as the source of truth. The Repository does not fill in business records absent there. Provide the test-only factory `createDemoRepository({clock, seed:'demoSeed', patches, simulator})`. patches is an array of `{entity, id, set}` (entity is a demoSeed section name, such as customers, units, or memberships; memberships targets actors). Apply them together immediately after reset and validate with the canonical DTO schema; invalid data throws an exception (test failure). The UI and composition-root do not pass patches. simulator is the initial IR45 enabled value. Acceptance Given conditions describe differences from demoSeed; unspecified values stay as in demoSeed. IR91 expands abbreviated rows into canonical DTOs. Per-acceptance fixed patches are in fixture-contract.json acceptancePatches (IR85). Create test-only states such as Membership expiry, contract end, or Customer inactive through patches; do not add new demo.trigger types.

## IR70 Nonworking days for capacity — REV18-027

members.capacity working intervals are Monday–Friday 09:00–17:00 in Asia/Kuala_Lumpur. Nonworking days are only Saturday/Sunday in that timezone; 1A has no public-holiday calendar (for example, 2026-09-16 is also a workday). Public-holiday support is a possible production requirement.

## IR71 Change event types and Query invalidation — REV18-028

ChangeEvent.entityType accepts only the values in the following table plus cursor_only. events.subscribe resources is an array of these values excluding cursor_only; empty arrays and unknown values return VALIDATION. The UI invalidates only Queries for operations mapped to the received entityType, not operations absent from the table. This replaces "jobs/units/invoices/restrictions, etc." in common.md §6.

| entityType | Read operations to invalidate |
|---|---|
| unit | units.list, units.get, summaries.get, admin.summary, telemetry.summary |
| device | devices.list, devices.get, devices.events, units.list, units.get, summaries.get, admin.summary |
| allergen_observation | telemetry.series |
| measurement | telemetry.series, telemetry.summary, units.list, units.get, summaries.get, admin.summary, energy.summary |
| command | commands.get, units.get, diagnosticRuns.get, restrictions.get, restrictions.forInvoice |
| diagnostic_run | diagnosticRuns.list, diagnosticRuns.get, units.get |
| device_operation | devices.operations, devices.calibrations, devices.get, units.get |
| alert | alerts.list, alerts.get, units.list, units.get, jobs.list, jobs.get, summaries.get, admin.summary |
| notification | notifications.list |
| job | jobs.list, jobs.get, jobs.events, summaries.get, admin.summary |
| report | reports.get, jobs.get |
| attachment | reports.get, attachments.getContent |
| offer | jobs.list, jobs.get, summaries.get |
| assignment | jobs.list, jobs.get, members.capacity, members.eligible, summaries.get |
| plan | plans.list, plans.get |
| contract | contracts.list |
| invoice | invoices.list, invoices.get, admin.summary |
| payment | invoices.list, invoices.get, admin.summary |
| restriction | restrictions.list, restrictions.get, restrictions.forInvoice, contracts.list, units.get |
| inquiry | inquiries.list |
| automation | automations.list, automations.nextRuns |
| policy | policies.list, policies.get |
| consent | consents.get, automations.list |
| membership | members.list, members.eligible, members.capacity, session.get |
| organization | organizations.list, admin.summary |
| customer | customers.list, admin.summary |
| property | properties.list, units.get, jobs.list, jobs.get |
| space | spaces.list, units.get |
| capability | capabilities.list, units.get, automations.list |
| baseline | baselines.list, energy.summary |
| factor | factors.list, energy.summary |
| mrv_report | mrv.list, mrv.get, mrv.versions |
| offset_record | offsets.list |
| client_user | clientUsers.list |
| ventilation_log | ventilation.list |
| unit_import | units.list, properties.list, spaces.list |
| firmware_campaign | firmwareCampaigns.list, firmwareCampaigns.get, devices.get, devices.operations |
| contractor | contractors.list |
| rate_card | rateCards.list, payouts.get |
| certificate | certificates.list, members.eligible, members.capacity |
| unavailability | members.capacity, members.eligible |
| sla_target | sla.scorecard |
| payout_statement | payouts.list, payouts.get |
| filter_care | filterCare.list |
| default_rule_setting | policies.list, policies.get |
| session | All Queries (update IR17 viewEpoch and discard) |

## IR72 Specification priority — REV18-029

When descriptions conflict on the same topic, use the following priority, highest first. Implementation Agents must not override a higher-priority description with a lower-priority one. Report conflicts as document defects and stop G1; do not choose between them during implementation.

| Priority | Specification |
|---|---|
| 1 | User-confirmed decisions (DEC-12/13/16/17/18/44/50) |
| 2 | IR in this document (higher-numbered IR takes priority on the same topic) |
| 3 | SR in strict-review-contracts.md |
| 4 | D01–D16 in deterministic-contracts.md |
| 5 | service-contracts.ts and all catalog CSVs (update together with IR; mismatches are document defects) |
| 6 | DDC in implementation-contracts.md |
| 7 | common.md and role-specific detailed design (DD) |
| 8 | FR/BR/AT in the requirements specification |
| 9 | UIUXSpecification.md |

Remove old text replaced by an IR from the body in the same version, or replace it with a reference to the IR. validate_documents.py detects superseded phrases.

## IR73 Validator mutation tests — REV18-030

Align check_review_regressions.py mutation targets with the current baseline and wording. Successful static validation (validate_documents.py) and mutation tests (check_review_regressions.py) are both conditions for document handoff. Include both result files in gate-record evidence_paths.

## IR74 Minor clarifications — REV18-025 and REV18-031–048

- REV18-025: The AT-A06-N state sequence is requested → offered → accepted → assigned → in_progress → submitted → completed. It includes technician jobs.start (common.md §5).
- REV18-031: DD-C01 summary fields are powerOn/powerOff/powerUnknown/online/offline/unknown/alertCount/asOf (SR27/IR51).
- REV18-032: C08/T01 UI severity=all omits filters.severity from the request.
- REV18-033: DD-A03 validFrom is always required; validUntil is required only for external technicians.
- REV18-034: Add iPadOS 17 Safari (widths 768/1024) to D10 supported environments.
- REV18-035: Acceptance-condition times written as "YYYY-MM-DD HH:mm" without Z or an offset are local Asia/Kuala_Lumpur times. Date-only from/to values mean calendar-day 00:00 in that timezone.
- REV18-036: Clients do not send commands.create reason (sending it returns VALIDATION). It is required for technician/admin (IR09).
- REV18-037: Normal mock-read wait is fixed at fixture.defaultWaitMs=300 ms. Unit/component tests use clock injection for 0 ms.
- REV18-038: A01/A13 periods use today/7d/30d/custom presets (SR17). P01/T01 from/to are YYYY-MM-DD calendar dates in the display timezone, converted to UTC Instants [from date 00:00, day after to date 00:00), with a maximum of 366 days.
- REV18-039: AT-T03-E ordering and duplicates use RawMeasurement.sequence (Measurement.version is assigned by the Repository).
- REV18-040: Derive DemoTrigger(device) evidenceSource from its type: communication_lost → heartbeat, power_lost → power_signal, tamper → tamper_signal; restored uses the same value as the referenced fault. DD-T12 evidenceSource is read-only.
- REV18-041: Space.kind nesting has no order constraint (validate only same Property and no cycles).
- REV18-042: units.save and similar operations do not accept tenantId input (it comes from the D14 Session).
- REV18-043: FR-A04 requires device.write; FR-A12 requires automation.policy.write.
- REV18-044: NFR-03 "Reconfirm on expiry" means that if Session expiry, a permission change, or a target-version change occurs while the confirmation dialog is open, confirm must not submit. Fetch again and repeat confirmation.
- REV18-045: 1A is the clickable frontend demo of Phase 1 in the original company text. 1B connects real equipment, firmware, and production APIs in the same Phase 1. Phase 2 is HVAC.
- REV18-046: List connecting/device-error/device-online/device-offline in the screen catalog only for Screens whose operations include equipment, device, measurement, control, restriction, or summary reads.
- REV18-047: The implementation Agent drafts ms dictionary wording and marks each `i18n/ms` key as unreviewed. Business/UI/UX reviews it before company acceptance. Only key-set equality is checked automatically (IR44).
- REV18-048: Use translation key app.name for the app name (en: "AC Monitoring Demo", ms: "Demo Pemantauan AC"). Do not use the reference site's name or logo.

## IR75 Current baseline and treatment of the 0.18.0 records — REV19-001

Work on the 0.18.0 fixes (REV18) stopped before self-review and baseline creation. Do not create a spec-manifest for 0.18.0. Store only the REV18 findings (review.md, findings.json) and gate-G1.yaml (gate_result=not_evaluated, spec_baseline_id=null, interrupted=true) in runs/DOC-0.18.0, and do not use them as implementation input. The source-language baseline for specification 0.21.0 is DOC-0.21.0. Store its original spec-manifest.json, review.md, findings.json, traceability-matrix.csv, static-check.json, validator-negative-checks.json, gate-G1.yaml, and completion.json in runs/DOC-0.21.0. Quote acceptance-plan CSV values containing commas or quotation marks according to CSV rules; mismatched column counts are static validation errors. The English translation uses the separate TRANSLATION-EN-2026-09-17 manifest; README, SDLC §8, and IR14 point to that manifest. The original DOC-0.21.0 approval applies only to its original hashes; it does not approve translated contents. Keep DOC-0.19.0 and DOC-0.20.0 as historical versions that failed independent G1 review.

## IR76 Technician screens before the work window starts — REV19-002

Before the IR49 work window starts (within the viewing window and now < scheduledStart of the technician's own active Assignment), show the technician screen in the screen-catalog state `work-not-started`. This section takes priority over IR57 permission-denied.

- SCR-T04 (/technician/jobs/:id) and SCR-T10 (/technician/units/:id/control): when jobs.get shows scheduledStart>now, disable Queries for units.get, reports.get, attachments.getContent, commands.get, and diagnosticRuns.get/list (do not call them). Use only jobs.get as the primary query. Show the job type, equipment ID, scheduled slot, and status as read-only, with “Work can start at {scheduledStart}” and disabled start, save, submit, and control buttons.
- SCR-T02 (/technician/units/:id?jobId=) and SCR-T07 (/technician/units/:id/alerts?jobId=): if the primary query returns FORBIDDEN (messageKey=errors.assignment_not_started), show `work-not-started` rather than permission-denied. Do not include the start time in DomainError. Fetch it with jobs.get using the URL jobId, display it, and link to `/technician/jobs/:jobId`. If jobId is absent or jobs.get fails, omit the start time and link, and show only “Available from the work start time of your assigned job.” Handle other FORBIDDEN results according to IR57.
- SCR-T11 (/technician/devices?jobId=): disable devices.* write buttons that use jobId and show the start time. Reads follow IR49(b).
- On the displayed screen, check now>=scheduledStart at each one-second demo-clock tick. Once reached, enable and fetch the disabled Queries (move to normal loading). Do not change viewEpoch.
- In D10 screen-state priority, place `work-not-started` at the same level as forbidden/not-found. “permission-denied display (start-time guidance)” in AT-REV18-013① means `work-not-started`.

## IR77 Liveness simulator copying conditions and demo measurement sequence — REV19-003

In IR45 step 2, for each Sensor, copy value/unit only when the latest Measurement with the same sensorId (first by observedAt descending, sequence descending, then id ascending) has origin=measured, quality=valid, and value≠null. Do not generate measurements for Sensors whose latest value is estimated/inspection, suspect/missing, or value=null. Do not go back to copy an older measured value (IR08). The Sensor becomes stale/unknown under D07/SR27. Once demo.trigger telemetry supplies a measured/valid observation, copying resumes at the next minute boundary. Perform step 1 (lastSeenAt) and step 3 (observedState.observedAt) under IR45's online and powerSignal≠off conditions, whether or not any Sensor was copied.

RawMeasurement.sequence is optional. If omitted, Repository assigns the highest existing Measurement sequence for that sensorId plus 1 (or 1 if none exists). If explicitly supplied, do not apply the value when it is at or below the highest sequence (duplicate or backward sequence; REV18-039 in IR74). The /demo telemetry form leaves sequence blank by default (automatic numbering), and displays origin=measured and quality=valid as defaults. Specify sequence only in acceptance cases testing its order or duplication.

## IR78 Energy-saving forecast on the admin dashboard — REV19-004

This section replaces IR63's “automatically select a baseline with the same duration in minutes and return savings.” Add `energyForecast: EnergyForecast` to AdminSummary and always return it to dashboard.read holders. AdminSummary.energySummary returns only actual results for the period (kWh, cost, coverage, and quality), with baselineRef/baselineSnapshot=null and all savings fields (savedKWh/savingPercentage/savedAmountMinor/savedEmissionsKg) null. Compare with baselines in A13 (energy.summary).

Calculate EnergyForecast as follows. Use decimal rational arithmetic and round displays according to IR44.

1. Target equipment set U: nonarchived ACUnits matching filters (customerId/propertyId) and within current scope. If |U|=0, set baselineRef/baselineSnapshot=null, all numerical values=null, expectedUnitMinutes=0, validUnitMinutes=0, and qualityWarnings=['no_units'].
2. Baseline: among EnergyBaselines with method=demo_fixed, boundaryId=ac_input_electricity, baselineKWh≠null, and a unitIds set exactly equal to U, use the latest version of the one with the latest createdAt (first by id ascending for ties). If none exists, set baselineRef/baselineSnapshot=null and predictedBaselineKWh/forecastSavedKWh/forecastSavingPercentage=null, and include baseline_unavailable in qualityWarnings. Do not automatically select demo_period_comparison (measured baselines).
3. periodMinutes=minutes in [from,to); baselineMinutes=minutes in the baseline period; expectedUnitMinutes=|U|×periodMinutes.
4. validUnitMinutes=number of valid slots under D07/IR08/IR11 (origin=measured, valid, matching boundary). actualKWhOnValidSlots=sum of their energy values (null when validUnitMinutes=0; include actual_unavailable in qualityWarnings).
5. predictedBaselineKWh = baselineKWh ÷ (|U|×baselineMinutes) × expectedUnitMinutes.
6. predictedActualKWh = actualKWhOnValidSlots ÷ validUnitMinutes × expectedUnitMinutes (when validUnitMinutes≥1; otherwise null).
7. forecastSavedKWh = predictedBaselineKWh − predictedActualKWh (null if either is null). forecastSavingPercentage = forecastSavedKWh ÷ predictedBaselineKWh × 100 (null if predictedBaselineKWh is 0 or null).
8. If a baseline was selected, include modeled_baseline and prorated_forecast in qualityWarnings. Also include partial_coverage if validUnitMinutes<expectedUnitMinutes. Sort qualityWarnings in ascending ASCII order with no duplicates.

The A01 energy-saving card displays actual results (kWh, cost, coverage) beside the forecast. For forecast values >0, show “Expected reduction {absolute value}”; for values <0, “Expected increase {absolute value}”; for 0, “No change 0.0”. Always show “Forecast (prorated assumed baseline, demo)” and validUnitMinutes/expectedUnitMinutes. For null, show “No target equipment” if qualityWarnings contains no_units, “Baseline not set” if it contains baseline_unavailable, or “Cannot calculate” otherwise (this overrides IR68's null display). Show a link to /admin/energy only for energy.write holders. Do not call admin.summary during SR17's first minute of the day (from=to).

Place `baseline-demo-tenant-a` in demoSeed.baselines (unitIds=all five nonarchived units in tenant-a, method=demo_fixed, boundaryId=ac_input_electricity, period=[2026-08-01T00:00:00.000Z,2026-08-31T00:00:00.000Z), baselineKWh=2592, quality=modeled, createdAt=2026-09-01T00:00:00.000Z, version=1). /admin without filters displays a forecast; changing the equipment set with customerId/propertyId displays “Baseline not set”.

## IR79 Consistent session-extension wording — REV19-005

The sentence in IR36 denying extension was replaced with “User-initiated extension uses only demoSession.extend in IR55.” IR55 defines extension; IR36 defines shifting expiresAt during clock jumps. Other documents reference both sections. validate_documents.py detects wording that denies extension, including in this document (IR81).

## IR80 Displaying negative savings (C06) — REV19-006

Apply IR68's common formatter to the strings in FR-C06 BR, AT-C06-E, and DD-C06 too. With a 100kWh baseline and 120kWh actual use, the DTO has savedKWh=-20 and savingPercentage=-20, displayed as “Increase 20.0 kWh” and “Increase 20.0%”. For a zero baseline, display “Cannot calculate” for the savings percentage. Do not use the old forms “20% increase”, “Increase20%”, or “Increase rate 20%”.

## IR81 Scope of obsolete-wording detection — REV19-007

validate_documents.py checks obsolete wording in all Markdown under 01-requirements, 02-design, and 03-uiux, plus 04-agentic-sdlc/verification.md, including this document (review-resolution-contracts.md). In this document, exclude only lines that quote and explain text being replaced (lines containing “replace”, “obsolete wording”, “old expression”, or “old text”). Register checks as regular expressions covering wording variants. Targets include expressions denying extension, “permission to (manage|be able to manage) model numbers”, “permission to manage environmental policy (policies)”, “room is a display name”, “number of customer organizations”, obsolete negative-value displays, expressions making a release request mandatory after payment confirmation in S03, and obsolete phrases registered in 0.18.0 or earlier. check_review_regressions.py has mutations that restore each checked phrase individually; success requires detecting every mutation.

## IR82 Passing the return destination after login — REV19-008

When an unauthenticated user opens a protected route, the route guard replaces it with `/login?returnTo=<encodeURIComponent(pathname+search)>` (without hash). SCR-X-login url_selection includes returnTo. The login screen decodes returnTo only once and passes it to demoSession.signIn.returnTo only if it meets D08 conditions (same-origin relative path, allowed screen-catalog route, no `//`, scheme, control characters, backslashes, or double encoding). Remove invalid values from the URL and ignore them without showing a VALIDATION screen. After signIn succeeds, replace the route with returnTo if the selected role allows it; otherwise use role home. Do not use /login, /forgot-password, or /demo as returnTo.

## IR83 Refetching and refetch-failure displays; grouping invalidations — REV19-009

When successful data exists for the same query key, retain it during refetching triggered by subscription events, successful writes, or explicit refresh. Set aria-busy=true on the affected region and show a nonmodal “Updating” indicator. Do not return to skeleton (loading) or announce it in a live region. If refetching fails, retain data and show a stale note, last success time, and retry under DDC-03 (automatic read retries follow D04). If the query key changes (URL conditions, sort, target ID, role, or viewEpoch), start at initial/loading; do not display the previous key's data as results for the new conditions (IR34). If refetch returns FORBIDDEN/NOT_FOUND/UNAUTHENTICATED, discard retained data and follow IR57/D09.

For subscription-event invalidation, group events received within the same one-second demo-clock tick into one invalidation per query key, and run it after that tick is finalized. Keep SR14's deferral during multipage fetching.

## IR84 Initial location-consent records — REV19-010

In demoSeed.consents, place one Consent per client Membership (customer-a, customer-b), with purpose=location_automation, granted=false, grantedAt=null, revokedAt=null, version=1. In the same transition where members.save creates a role=client Membership, create and audit one identical initial Consent. consents.get returns the current Membership's record; if absent, return NOT_FOUND (a fixture defect). SR02's “versioned Consent with granted=false from the first request” means this record and does not conflict with IR69's “do not add business records absent from the seed”.

## IR85 Seed changes in KPI acceptance Given — REV19-011

For AT-A01-N/AT-A01-B① Given, `acceptancePatches["AT-A01-N"]` in fixture-contract.json is authoritative. It sets simulator=false and clock=2026-09-14T01:00:00.000Z; sets device-offline-rto to connection=online, lastSeenAt=2026-09-14T00:59:30.000Z, powerSignal=on; sets unit-offline-rto observedState to {power:false,celsius:25,mode:'cool',fanLevel:'mid',observedAt:'2026-09-14T00:59:30.000Z'}; and adds a sensor-offline-power measurement (value=0.0, unit=kW, origin=measured, quality=valid, observedAt=receivedAt=00:59:30Z, sequence=2). Results for tenant-a are ON 2 (unit-online-rto, unit-limited), OFF 2 (unit-non-rto, unit-offline-rto), unknown 1 (unit-other-customer), and operating rate 50.0%. In ATs that observe KPIs or counts, retain demoSeed values unless Given specifies a change, and set simulator=false before observation (IR45).

## IR86 Responding to expired unanswered Offers — REV19-012

For Offers with Offer.decision=null and now>=offerExpiresAt, jobs.accept/decline by the relevant contractor (an active Membership with partner.accept whose company matches Offer.contractorOrgId) returns D01 priority-6 CONFLICT as an invalid state within scope (messageKey=errors.offer_expired, zero business changes). Another company's Offer ID or a nonexistent ID returns NOT_FOUND (priority 3). D01 priority-4 “outside assignment validity” applies only to accessValidFrom/Until, Assignment viewing/work windows, and qualification validity periods, not Offer response deadlines. After expiry, that contractor's jobs.list/jobs.events omit the job and jobs.get returns NOT_FOUND (IR23). Same-key resends and writes.getResult follow IR01. This section governs AT-P02-E① (now=offerExpiresAt) and AT-REV18-004's “accept after expiry returns CONFLICT”.

## IR87 Character limits for reason inputs — REV19-013

reason, cancelReason, declineReason, resolutionReason, reviewComment, changeReason, and purpose require 1–1000 Unicode code points after trimming, regardless of DD wording (D12). Body/note fields (symptom, workText, message, reply, note, responseNote, assumptions, boundary) use their DD/IR limits. DD-T07 resolutionReason, DD-A14 reviewComment, DD-A10 reason, and DD-P05 reason were corrected to 1–1000. D12's “names not specified in each DD use 1–120 characters” applies only to name fields.

## IR88 Mapping MRV screen fields to canonical types — REV19-014

Display DD-A14 fields from the following canonical values; do not add DTO fields (D12).

| Display field | Source |
|---|---|
| reportCategory | Display MRVPreview.scope ('scope_2') as “Scope 2 (electricity)” |
| organizationId | conditions.organizationId |
| period | conditions.from / conditions.to |
| siteIds (target sites) | Deduplicate ACUnit.propertyId for each conditions.unitIds entry and sort in ascending ASCII order (derive from units.list(filters.organizationId) results) |
| gridRegion | summary.factorSnapshot.region |
| factorValue | summary.factorSnapshot.kgCO2ePerKWh |
| factorUnit | Fixed display “kgCO₂e/kWh” |
| factorYear | summary.factorSnapshot.year |
| factorVersion | summary.factorRef.version |
| boundaryDescription | conditions.boundary (ID is conditions.boundaryId) |
| coverageRatio | summary.coverage (display null as “Not calculated”) |

If summary.factorSnapshot is null, show “Calculation incomplete” in factor-related fields.

## IR89 Work-window ending notice and end handling — REV19-015 / REV19-037

- Notice: while a technician displays a screen with the jobId (SCR-T02/T04/T07/T10/T11), when the demo clock reaches scheduledEnd−15 minutes for their active Assignment, show a role=status banner once: “The work window ends at {scheduledEnd}. Please save unsaved input.” After dismissal, do not show it again for that Assignment. If a clock jump passes only the notice time, show it once when detected. If the jump also passes scheduledEnd, skip the notice and perform only the end handling below.
- End: when now>=scheduledEnd, increment viewEpoch under IR24, discard Queries, snapshots, unsaved form values, and object URLs, switch to history display, and notify “Unsaved input was discarded because the work window ended.” FR-T09's “do not erase unsaved edits on refetch” applies only to refetches within the work window.
- HQ/contractor display: in JobSummary/JobDetail, show “Work window ended; reassignment required” for jobs with status∈{assigned,in_progress} and scheduledSlot.endAt<=now. Derive this from existing DTOs without new fields. Reevaluate visible lists on each one-second tick without refetching.
- Extend the work window through jobs.assign in IR49. Technician writes return FORBIDDEN until a new Assignment exists (D06).
- In the same transition where jobs.assign succeeds (initial assignment, reassignment, or extension), set Job.assignmentId to the new Assignment ID and Job.scheduledSlot to [new Assignment.scheduledStart, scheduledEnd), and increment Job.version by 1. Retain the old Assignment with status=revoked (REV19-037).

## IR90 Minor clarifications — REV19-016–035

- REV19-016: DD-A01's customer count follows IR40. DD-A04 requires device.write; DD-A12 requires automation.policy.write (REV18-043 in IR74). D09 `<room>` matches Space.name under IR65. In verification.md S03, payment confirmation triggers a release request (IR35); explicit release checks the idempotent response. /forbidden and undefined routes in common.md §2 display screens linking to role home; they do not redirect automatically (IR57).
- REV19-017: Keep contactWindow validation rules (IR64). Always show “Use HH:mm for times (example: Weekdays 09:00-18:00)” beside the input and include the same example in errors.contact_details_forbidden. “0900-1800” returns VALIDATION as eight consecutive digits; “09:00-18:00” is accepted.
- REV19-018: At IR67's mutual-exclusion recheck after one second, if the same equipment has a normal Command (UnitAction) in requested/sent, an active DiagnosticRun, or another queued/running DeviceOperation, set the operation to failed, failureCode=CONFLICT, finishedAt=now, without changing connection or version. D05 prevents this conflict in normal operation paths, so this rule protects an invariant; acceptance tests create the conflicting state using IR69 patches. Exclude restriction Commands with delivery=not_sent from this recheck. Restriction apply/remove Commands for equipment with check/firmware queued/running follow D03 as undelivered intent with delivery=not_sent and pendingReason=device_operation_running. Send them through restrictions.retry after the operation ends.
- REV19-019: invoices.create accepts dueAt only when later than now. Create overdue invoices with demoSeed or demo.advanceClock (IR36).
- REV19-020: DD-T01 status options are assigned/in_progress/on_hold/submitted/rework_requested/completed/all. DD-P01 options are offered/accepted/assigned/in_progress/on_hold/submitted/rework_requested/completed/all. all means omit status.
- REV19-021: Add devices.calibrations to IR71's device_operation row. IR71 defines invalidation from subscription events. After successful writes, also apply D10 and each DD's “Queries to refresh”.
- REV19-022: Use AuditView.result=pending for receipt audits of operations that create records awaiting asynchronous results (commands.create, diagnosticRuns.create, devices.check, devices.updateFirmware, restrictions.execute, restrictions.retry, restrictions.override, payments.simulate(event=initiate)). In the transition that finalizes the result, append a success or failed audit with the same correlationId; do not rewrite the pending row. FR-A16 and DD-A16 have four result categories: success, rejected, failed, and pending.
- REV19-023: All operations sort and compare severity using normal < warning < critical, not string comparison (`severity desc` in alerts.list/notifications.list means critical→warning→normal).
- REV19-024: VoiceContainer (feature hook) runs IR09's voice.resolveIntent, units.get, jobs.list, jobs.get, commands.create, and commands.get. VoicePanel is a display Component with only props and events (D10/D13).
- REV19-025: Count KPI cards use KpiCard (label, value:number|null, denominator:number|null, unknownCount:number|null, asOf, href:string|null, loading, error). Display null value as “Cannot calculate” and 0 as “0”.
- REV19-026: NotificationPanel's empty display is “No unread notifications” when unreadOnly=true, and “No notifications” when false.
- REV19-027: Public screens that do not fetch data use these states: SCR-X-login/SCR-X-forgot-password: initial, loading, success, error, offline; SCR-X-demo: initial, loading, success, error; SCR-X-forbidden/SCR-X-not-found: success only. Required states for authenticated screens remain unchanged.
- REV19-028: “Telemetry” in the UIUX specification means sensor measurements (Measurement), not usage tracking.
- REV19-029: Add unitIds and baselineId to SCR-A13 url_selection, and policyId to SCR-A11/SCR-A12, so Back/Forward restores selections (D13).
- REV19-030: Return EnergyBaseline, OffsetQuote, and OffsetRecord to a client only if every unitIds entry is in current scope. If only some are, omit the record from lists and return NOT_FOUND for individual reads (as with invoice/contract in SR03).
- REV19-031: DD-A06 unitId and type are required; dueAt is optional (HQ alone enters it; IR38). Define DD-A11 enabled and priority only in the common input row.
- REV19-032: Pass DD-P07 recipientRole to notifications.recipients role as hq→admin, assigned_technician→technician (the technician in the job's active Assignment), and customer_contact→client (customer Membership for the job equipment).
- REV19-033: When payment confirmation in IR35 or another event triggers release, audit the Restriction state transition under the initiating user (actorId and role at that time), and audit remove_restriction Command creation under actorId=system-restriction and actorRoleAtTime=system. Both share correlationId. IR59 refers to the Command creation audit.
- REV19-034: The energy-saving forecast calculation (IR78, option A) and work-window end handling (IR89) were finalized as DEC-44 and DEC-50 through user answers on 2026-09-17. Distinguish this from company commercial approval.
- REV19-035: For 1B connections, include an HTTP status/communication exception to DomainError mapping table (400/401/403/404/409/429/5xx/timeout/offline) among D11's required production artifacts. No mapping is defined in 1A.

## IR91 demoSeed normalization rules — REV19-036

demoSeed and acceptancePatches rows use abbreviated form. At creation, Repository expands them into canonical DTOs using only the following rules and validates them with the service-contracts.ts schema. Missing required values not supplied by these rules, or type mismatches, throw exceptions as fixture defects (IR69). This fills common fields of existing rows; it does not add business records.

1. tenantId: use the row value if present; otherwise derive from the parent (Property←Organization identified by customerOrgId; Space←Property; ACUnit←Organization identified by customerOrgId; Device/Measurement/Command/Alert/MaintenanceJob←ACUnit identified by unitId; Offer/Assignment←MaintenanceJob identified by jobId; Contract←Customer identified by customerId; Invoice/Restriction←Contract identified by contractId; Notification←actor identified by recipientMembershipId).
2. version: row value, or 1 if absent. createdAt: row value, or fixture.seedCreatedAt (2026-09-01T00:00:00.000Z) if absent. updatedAt: row value, or createdAt if absent.
3. Nullable fields absent from the row are null; array fields are []. Space.archived=false; ACUnit.type='split'.
4. Do not store derived values; calculate them on read: ACUnit.connection/lastSeenAt (IR47), Contract.activeRestrictionIds/hasUnresolvedRecovery (SR19/SR26), Invoice.paymentMethod/paymentStatus (DDC-08 §3).
5. Device: targetUnitId=unitId, createdByMembershipId='system-demo'. For sensors, use Sensors explicitly defined in the seed (id, metric, unit, staleAfterSeconds, boundaryId, calibratedAt=null). A mismatch with IR43 capability definitions is a fixture defect.
6. Measurement: eventId=id, isDemo=true, qualityReason=null, rawUnit=null.
7. Command: correlationId=`seed-`+id, diagnosticRunId/jobId/reason/failureCode=null.
8. Alert: evidenceIds=[], deliveryFailures=[], acknowledgedAt/resolvedAt/resolutionReason=null.
9. Notification: generate params from target resources in the normalized seed, using the same template-generation function as at runtime (D12). If scopeVersionAtCreation is omitted, fill it from the receiving Membership under IR106.
10. MaintenanceJob: planId/occurrenceAt/startedAt/completedAt/draftReportRef=null, reportRefs/costs=[].
11. Restriction: events=[]. perUnit[].observedRestriction is observedRestriction of the same Unit; perUnit[].evidenceId is null.
12. For `{entity,id,set}` in acceptancePatches, if id exists, overwrite fields in set. Otherwise, normalize set as a new row under this section's rules and add it.
13. Policy: tenantId comes from the ownerMembershipId actor; if createdByUserId is absent, use that actor's userId; disabledReason=null. EmissionFactor: isDemo=true.

## IR92 Interpreting acceptance Given and seed changes — REV19-038 / REV19-040–042

Convert Given in acceptance conditions (AT-*-N/E/B/SRC/R01, S01–S08) to prerequisite data using only the following rules. Report prerequisites not determined by these rules as document defects; test agents must not invent them.

1. IDs in Given (unit-online-rto, job-contractor-a, etc.) refer to demoSeed rows normalized under IR91. Overwrite only values stated in Given; leave all other demoSeed values unchanged.
2. Defaults when no target is stated: customer equipment control, monitoring, automation, and policy target unit-online-rto; equipment without ventilation support is unit-non-rto; restricted equipment is unit-limited; offline equipment is unit-offline-rto; customer is customer-a; contractor is contractor-a; external technician is tech-external-a with job-contractor-a; internal technician is tech-internal-a; HQ is hq-operator (hq-restriction-manager for restriction/release operations); invoice is invoice-overdue-a; restriction is restriction-limited-a.
3. Given business states such as “in_progress job”, “submitted report”, or “accepted job” are created from the seed by running normal operations (jobs.start, jobs.saveDraft, jobs.submit, jobs.offer, jobs.accept, etc.) in the stated order, unless acceptancePatches exist.
4. For Given that lists states as ①②…, independently patch only the relevant fields of rule 2's target in each subcase. However, do not patch states that require normal operations under IR97 rule 3 (Restriction.state, Job.status with report versions, and Payment/Invoice states); create them using the operations stated in the acceptance text.
5. In fixture-contract.json, `acceptancePatches` keys are case IDs (subcases use `.number`) or `shared:` names. Values are `{clock, simulator, include?, patches, query?, expected?, input?, evaluation?, finalEvaluation?, trigger?, advanceSeconds?, flow?}` (`shared:` uses `{description, patches, input?, bindAtUse?}`). input is the complete save input; evaluation is EvaluationInput; trigger is DemoTrigger; flow is the sequence of normal operations that creates a state; bindAtUse is a value determined in the acceptance text (IR97). Expand shared patches in include first, then apply the case's own patches. A patch is `{entity,id,set}` (IR91 rule 12) or a measurement series: `{entity:'measurements', series:{idPrefix, unitId, sensorId, metric, unit, boundaryId, from, to, stepSeconds, value, origin, quality, sequenceStart, skip}}`. For each from+k×stepSeconds in [from,to), excluding timestamps within skip's [from,to), series creates a row with id=`idPrefix-k`, observedAt=receivedAt=that timestamp, and sequence=sequenceStart+k. entity is a demoSeed section name (memberships uses actors).
6. Cases with acceptancePatches: AT-A01-N, AT-C06-N, AT-C06-E.2, AT-C06-E.3, AT-C06-E.4, AT-C08-N, AT-P01-N, AT-P03-R01, AT-P06-N, AT-P06-B, AT-T07-N, AT-T10-E.1, AT-T11-N, AT-A09-R01, AT-A13-N, AT-A14-N, AT-C13-N, AT-C10-E.1, AT-C08-SRC, AT-T07-SRC, AT-A05-SRC, AT-C07-SRC.4, AT-A12-SRC.4, AT-T12-N, AT-A05-N, AT-A11-N, AT-A12-N, AT-X02-B, AT-X06-B.1, AT-X06-B.2, AT-X06-B.3, AT-X06-B.5, AT-REV17-005, AT-X04-E.5, AT-G121-002, AT-G121-004. Shared patches: shared:energy-actual-80, shared:tech-internal-a-job-online, shared:load-cause-alerts, shared:report-draft-all-normal. Acceptance text explicitly states the key.
7. Energy/emissions acceptance (C06/C13/A13/A14, S05) uses fixture.energy's [2026-09-14T00:00Z, 01:00Z) window and unitIds=[unit-online-rto]. factor-demo-2026 (0.5 kgCO₂e/kWh) in demoSeed.factors is the record identified by fixture.defaultEmissionFactorId.
8. 1A does not reject overlapping contract periods that include the same equipment (an explicit statement of the current rule; only one active restriction per equipment under DDC-08 §3).

validate_documents.py checks that acceptancePatches keys referenced by acceptance text exist, include resolves, entity names are demoSeed sections, series formats are valid, and existing-row overwrite targets exist.

## IR93 Failure codes for technician work start and submission — REV19-039

Determine failure codes for technician jobs.start/jobs.submit/jobs.saveDraft according to D01 priority as follows.

| Job and assignment state | Result |
|---|---|
| The technician has never had an Assignment (for example, unassigned job-internal-a in requested) | NOT_FOUND (priority 3) |
| After the work window starts, Assignment becomes revoked/ended through cancellation, reassignment, or expiry; only history is visible | FORBIDDEN (priority 4, messageKey=errors.assignment_ended) |
| Assignment is revoked through cancellation before the work window starts (no history under IR24) | NOT_FOUND |
| Active Assignment within its work window, but incompatible state (on_hold, submitted, completed, etc.) | CONFLICT (priority 6) |
| Active Assignment before its work window starts | FORBIDDEN (errors.assignment_not_started, IR49/IR76) |

The contracted company's partner.review holders accept or return reports for outsourced jobs (contractorOrgId≠null). HQ may do so only with jobs.review reviewMode=hq_escalation and a reason (D06).

## IR94 Authorization-column qualifiers and technician write conditions — G1-001 / G1-026 / G1-030

Qualifiers in the operation catalog's authorization column have only the meanings below (DEC-54). Always also apply SR03's condition covering all target Units to resources with multiple Units.

| Qualifier | Meaning |
|---|---|
| public:demo-only / public:demo-panel-only | Demo operations requiring no Session (IR60). Call demo-panel-only only from /demo |
| authenticated:own-session / demo-account-switch | The user of a valid Session (D09) / demo account switching |
| authenticated:recipient-only | Notification.recipientMembershipId is the current Membership (IR58) |
| authenticated:current-target-party / recipient-or-current-target-party | A party who can view the D08/D15 notification target resource in current scope |
| authenticated:original-user-and-current-target-scope | D04 writes.getResult conditions |
| client:self / client:self-customer / client:control.execute[:self] | Resources of the user's own customer organization within Membership.scopes (control.execute also requires that permission) |
| client:own-membership | Records of the user's own Membership (Consent) |
| client:accepted-report-only | Accepted report versions only |
| client:self:customer-visibility / demo-event-only / event=request-or-retry / requested-only | Only JobNote visibility=customer / only IR59 customer payment events / SR22 / only requested under IR56 |
| contractor:accepted-valid-offer / delegated | Own-company Offer is accept and now∈[accessValidFrom, accessValidUntil) (IR23 summary projection period) |
| contractor:offer-projection-or-delegated-history | IR23 offer/summary/history projections |
| contractor:partner.accept:own-valid-offer:first-attempt | IR01/IR86 |
| contractor:partner.assign:own-valid-offer / own-company | Within the access window of the company's accepted Offer / only role=technician Memberships in the user's own organization |
| contractor:partner.review:own-offer / submitted | Submitted versions for jobs contracted by the user's own company |
| technician:assigned (read) | Internal: Units within Membership.scopes (except job-based reads under IR49(a)); external: own Assignment's viewing window ∩ scopes |
| technician:assigned-history | IR23/IR24 projections |
| technician:*:assigned / assigned-valid-job / job-required (write) | Table below |
| admin:<permission> | Holds the permission and is within managed scope |
| admin:<permission>:scope-candidate-read-only | D12 supporting reads |
| admin:<permission>:kind=… | Permission for each Policy.kind |
| admin:job.write:internal-job / internal-or-escalation | Jobs with contractorOrgId=null / for outsourced jobs, jobs.review reviewMode=hq_escalation (D06) |
| admin:restriction.override:release-projection / release-intent-or-terminal-recovery-only | IR03 |
| IR01:same-key-receipt… | IR01 |

For both internal and external technicians, determine write access (alerts.acknowledge/resolve, devices.*, commands.create, diagnosticRuns.create, jobs.start/saveDraft/submit/resumeRework, attachments.add) using this table (making SR03's “apply required Assignment conditions to internal technician writes too” concrete).

| Condition | Result |
|---|---|
| jobId omitted for an operation whose input type includes jobId | VALIDATION (fieldErrors.jobId, D01 priority 1) |
| The active Assignment for input jobId belongs to the user, Job.unitId=target Unit, and now is within the work window | Allowed |
| Operation whose input type has no jobId (alerts.acknowledge/resolve, devices.addResponseNote) | Allowed if the user has an active Assignment for the target Unit (Alert.unitId or Device's current unitId), and now is within at least one such work window |
| Before the work window starts | FORBIDDEN (errors.assignment_not_started, IR49) |
| Ended/revoked after the work window starts | FORBIDDEN (errors.assignment_ended, IR93) |
| The user has never had an Assignment for input jobId, or it was revoked through cancellation before the work window starts | NOT_FOUND (IR93) |
| Operation without jobId in its input type; the user has never had an Assignment for the target Unit | For internal technicians within unit scope: FORBIDDEN (errors.assignment_required, D01 priority 4). Outside scope, or for external technicians: NOT_FOUND |

If Device.connection≠online when devices.updateFirmware is requested, return D01 priority-8 OFFLINE (do not create a DeviceOperation). Use IR67's failed with failureCode=OFFLINE only if connection becomes non-online at the start tick one second after creation. devices.check checks connectivity, so accept it regardless of connection.

jobs.assign and members.eligible require the candidate technician's Membership.scopes to include Job.unitId (unit/property/organization scope containment; SR03). Omit technicians who fail this condition from candidates; direct selection returns FORBIDDEN (errors.technician_out_of_scope). members.eligible and members.capacity return only role=technician Memberships. Contractor members.list also returns only role=technician in the user's own organization; HQ members.list returns all roles within managed scope.

## IR95 Business event notifications — G1-002

Policy-based Alerts/quality notifications (SR21/SR28/D08), restriction notices (IR05), and reminders (IR04) follow their respective sections. Generate notifications for other business events only from this table (DEC-55). Events absent from the table (IR48 Offer expiry, reads, draft saves, notes, previews, and others) create no notifications.

Shared rules: channel=inApp, deliveryState=simulated, one notification per recipient Membership, target/params under D08/D12, and occurredAt=transition now. Do not send to the Membership (actor) that performed the triggering operation. Recheck recipients against current scope and validity; create none for Memberships that cannot read the target. Retries with the same event ID add none. templateKey=alert follows IR10 Alert classification; all other templates use the same name for templateKey and type. Determine severity from the IR104 table.

| Event | templateKey | target | Recipients |
|---|---|---|---|
| job.requested (jobs.create, plans.generateNext) | job_update | job | Customer client Memberships that can read the job Unit; HQ job.write holders |
| job.offered | job_update | job | partner.accept holders at the offered contractor; customer clients (display "Being arranged") |
| job.accepted | job_update | job | HQ job.write holders; customer clients |
| job.declined | job_update | job | HQ job.write holders |
| job.assigned (initial assignment, reassignment, extension) | schedule_change | job | Technician of the new Assignment; customer clients; HQ job.write holders; for outsourced work, partner.assign holders at the accepted contractor |
| report.submitted | job_update | job | For outsourced work, partner.review holders at the accepted contractor; for internal work, HQ job.write holders; customer clients (progress only) |
| report.returned | report_return | job | Assigned technician; HQ job.write holders |
| job.completed | completion | job | Customer clients; HQ job.write holders; assigned technician; for outsourced work, partner.review holders at the accepted contractor |
| job.cancelled / on_hold / resumed | job_update | job | Customer clients; assigned technician; for outsourced work, partner.assign holders at the accepted contractor; HQ job.write holders |
| restriction.requested / applied / release_requested / released / cancelled | restriction | restriction | Customer clients who can read all target Units under IR19; HQ restriction.write holders |
| payment.confirmed (payment confirmation) | payment | invoice | Customer clients who can read the invoice; HQ billing.write holders |
| inquiry.received / answered | inquiry | inquiry | For received, HQ billing.write holders; for answered, clients of the customer who made the inquiry |
| Opening an Alert without a policy (device events, IR98 load_alert, records generated outside the seed) | alert | unit | Customer clients who can read the Unit; HQ alert.resolve holders; assigned technicians within their viewing window |
| device_operation.failed | device_operation | device | Membership that created the operation; HQ device.write holders |

Add job_update and device_operation to canonical Notification.templateKey and NotificationType. For the seed's four actors, for example, if contractor-a performs job.assigned, tech-external-a, customer-a, hq-operator, and hq-restriction-manager receive one each (four total); actor contractor-a receives none.

## IR96 Restriction cancellation — G1-003 and G1-013

Determine the result of restrictions.cancel (restriction.write, reason required) only from this table (DEC-56).

| Current state | Result |
|---|---|
| scheduled | cancelled. Create no Commands; keep advance-notice evidence |
| requested / applied | release_requested. Set releaseIntent.source='cancel' and perform D03 per-equipment release evaluation within the same transition as IR35 |
| release_requested | Idempotently return the current Restriction without increasing version or audits (except denials) |
| released / cancelled | CONFLICT (D01 priority 6) |

IR35 release-request triggers are payment confirmation, grace/exception, forced release, cancellation (this section), and an explicit `restrictions.release` request (source='manual'). Add `releaseIntent:ReleaseIntent|null` (source, at, actorMembershipId) to the canonical Restriction. Include `releaseIntent:{source,at}|null` in RestrictionReleaseView. Client projections use actorMembershipId='masked' (IR42). A10 shows reconcile/retry(phase=release) to override-only users only when releaseIntent.source='override' or unresolved recoveryCases exist (IR03).

## IR97 Acceptance fixture invariants and input objects — G1-004/G1-005/G1-009/G1-011/G1-019/G1-031

1. Measurements created through patches or series must also meet D07 ranges and IR12 normalization rules. Values with origin=measured and quality=valid must be in range; otherwise throw a fixture-defect exception during generation. Create the AT-C06-E.3 actual value of 120 kWh with two Units, unit-online-rto and unit-non-rto, × 60 kW × 60 slots, and the same two Units' 100 kWh baseline (baseline-energy-100-two-units).
2. Assignment patches must set scheduledStart=validFrom and scheduledEnd=validUntil to matching values. If the Assignment is Job.assignmentId, Job.scheduledSlot must use the same interval.
3. Patches changing only state fields are limited to values without invariants involving other resources (such as Notification.readAt). Create Restriction.state, Job.status involving report versions (submitted/completed/rework_requested), and Payment/Invoice states through normal operations.
4. Acceptance tests start with simulator=false by default (DEC-58). Use simulator=true only for AT-REV18-001, AT-REV19-003, and AT-REV19-009, which test automatic generation itself.
5. For acceptance tests involving save inputs, place the full canonical input object in acceptancePatches `input` and reference it from the text (AT-A05-N, AT-A11-N, AT-A12-N, and others). Neither UI nor Repository fills in missing required values (SR28).
6. "Notification" and "notification preview" in acceptance text mean a saved inApp Notification (deliveryState=simulated). Describe unsaved notifications.preview as "Preview (zero saved records)".
7. AT-A12-N/B uses the CO₂ sensor addition, input, evaluation, and flow in `acceptancePatches["AT-A12-N"]`, checking the 59-second and 60-second duration boundaries in IR103 order. Do not expect notification created from a single fire immediately after saving.

validate_documents.py checks 1 and 2 across all acceptancePatches, and checks that keys referenced by the acceptance-plan CSVs (acceptance-review-019/020) exist.

## IR98 Demo data for allergen observations and possible-cause Alerts — G1-006 and G1-027

Allergen observations (DEC-57) come from demoSeed.allergenObservations rows (id, unitId, availability, substance, value, unit, sourceLabel, observedAt, evidenceText, createdAt). When telemetry.series targets one Unit, return that Unit's first row ordered by observedAt descending (null last), createdAt descending, id ascending as allergenObservation. With no row, return {availability:'not_measured', all other fields:null}. If the first row has availability='unsupported', all other fields are null. Multiple Units return null (D12). available rows require substance/sourceLabel/observedAt/evidenceText. Return rows with a value but unit=null unchanged; the UI displays "Unknown".

Add these two DemoTrigger branches (demo-only, IR60).
- `{eventType:'allergen', observation:{unitId, availability, substance, value, unit, sourceLabel, observedAt, evidenceText}}`: Add one allergenObservations row.
- `{eventType:'load_alert', unitId, causeCode, evidenceKind, evidenceText, severity}`: Create an Alert with policyId=null, type=sensor, status=open, and observedAt=detectedAt=now. Apply IR66 incident keys and IR95 notification rules.

Place an available row for unit-online-rto (demo dust-mite allergen value) and an unsupported row for unit-non-rto in demoSeed. Other Units have no row and return not_measured.

Acceptance fixtures are as follows.
- AT-C07-SRC/AT-A12-SRC: ① available = unit-online-rto; ② unsupported = unit-non-rto; ③ not_measured = unit-limited; ④ missing unit = `acceptancePatches["AT-C07-SRC.4"]` / `["AT-A12-SRC.4"]`.
- AT-C08-SRC/AT-T07-SRC/AT-A05-SRC: Use the same-named `acceptancePatches`. All include `shared:load-cause-alerts` (suspected open window = seed alert-window-a; inspection record of poor insulation = alert-insulation-a; no evidence = alert-unknown-a). AT-C08-SRC adds two notifications to customer-a; AT-T07-SRC adds tech-internal-a's assigned job (job-t07 in `shared:tech-internal-a-job-online`).

## IR99 Air-quality guidance display — G1-007

C07 (and the A12 display) shows the following guidance from the latest Measurements for one target Unit. Thresholds are demo values (DEC-09, DEC-57), not health judgments. Guidance creates no Commands.

| Metric and condition (compare only quality=valid values) | Guidance key and text |
|---|---|
| co2 ≥ 1000 ppm, Capability.ventilation=true, and ventilationLevels includes low | air.guidance.ventilate "Ventilation recommended" and a ventilation-request button |
| co2 ≥ 1000 ppm, ventilation unsupported | air.guidance.ventilate_manual "Ventilate manually, for example by opening a window" (D08) |
| pm25 ≥ 35 µg/m³ | air.guidance.clean "Filter cleaning and inspection recommended" |
| At least one of co2/pm25 is valid, and none of the above applies | air.guidance.none "No current guidance" |
| Both co2 and pm25 are missing/stale/suspect or have no sensor | air.guidance.unavailable "Not enough data to provide guidance" |

Show all guidance when multiple conditions apply. Do not provide guidance based on temperature or humidity.

## IR100 Inspection component sets and submission validation — G1-008

UnitDetail.components includes every component in each ACUnit.serviceScope group (DEC-58): 8 indoor, 5 outdoor, and 5 electrical. Group order is indoor → outdoor → electrical; within each group, use the order listed in DD-T04–T06. On the first jobs.saveDraft, the UI sends all components as InspectionItemInput with result=null. saveDraft also accepts saves containing only some components.

jobs.submit validates the target version as follows. Any violation returns VALIDATION (D01 priority 7), with every invalid field included in fieldErrors.
1. The items componentKey set matches Unit.components at submission time (missing/extra keys use fieldErrors.items).
2. Every item's result is nonnull.
3. Items with result=attention/not_inspected/not_applicable have a reason of 1–1000 characters.
4. workText is 10–4000 characters.
5. nextAction is nonnull (follow_up requires a future time and a note of 1–1000 characters).
6. parts quantity is 1–999.
7. Every attachmentRef has status=ready.
8. Each measurement's unit matches its metric.

If HQ changes serviceScope during work, validate against components at submission time. Full acceptance inputs use `input` in `acceptancePatches["shared:report-draft-all-normal"]` as the baseline (18 normal components, 50-character workText, nextAction=none); each acceptance description states only its differences.

## IR101 Concrete shared acceptance values and model-registry tenants — G1-025

The "Concrete Shared Acceptance Values" table in the common requirements specification is authoritative for AT-X01–X07 N/E/B values. Create AT-X06 unsupported models through `acceptancePatches["AT-X06-B.1"]`–`["AT-X06-B.3"]` (no temperature support, cool only, fan only) and `["AT-X06-B.5"]` (RTO without restriction support). Create same-named Space candidates through `["AT-X02-B"]`.

Define the following to make shared acceptance unambiguous (DEC-59).
- Context.scopeVersion checks the view generation; the Repository uses only the current Membership for authorization. For requests whose Context.scopeVersion differs from the current value, first determine the D01 priority 3–4 authorization result (NOT_FOUND/FORBIDDEN). If authorized, return CONFLICT (messageKey=errors.scope_changed, D01 priority 6, zero side effects). The UI updates Context through session.get and discards/refetches Queries under IR17.
- Requests after a Membership reaches now>=validUntil or now<validFrom return UNAUTHENTICATED (messageKey=errors.membership_inactive, D01 priority 2). Discard the screen and navigate to /login as for D09 expiry. demoSession.signIn/switchMembership to a Membership outside its valid period returns FORBIDDEN (errors.membership_inactive).
- restrictions.schedule with a Contract whose restrictionEligible=false returns VALIDATION (fieldErrors.contractId, messageKey=errors.restriction_ineligible, D01 priority 7).
- If voice.resolveIntent candidates have multiple identical pathLabels, VoicePanel also shows unitId for each and switches to text-input mode for selection (FR-X02 "When they cannot be distinguished"). Do not select automatically.
- Create AT-X04-E⑤ self-approval through `acceptancePatches["AT-X04-E.5"]` (hq-self-approver, a second Membership for user-tech-internal-a, with role=admin and job.write).

demoSeed.capabilities explicitly identifies the owning tenant (tenantId). tenant-b's unit-tenant-b references cap-split-std-tb for tenant-b. Equipment referencing another tenant's model ID is a fixture defect (IR91 item 1).

## IR102 Minor clarifications — G1-010/G1-012/G1-014–G1-024/G1-026–G1-029

- G1-010: AT-C04-E③ and AT-FIX-019 keep the seed clock (2026-09-14T01:00Z). D09 366-day validation detects Sunday 02:30 in America/New_York (nonexistent on 2027-03-14) and Sunday 01:30 (ambiguous on 2026-11-01), returning VALIDATION (D01 priority 7).
- G1-012: Update old acceptance plans to the current specification. Omitted dueAt in AT-REV17-004 is 2026-09-15T04:00Z (IR74). Test AT-REV17-005 with equipment without dependencies (acceptancePatches AT-REV17-005). An out-of-scope ID on a known route in AT-REV17-014 keeps the URL and shows not-found with a parent-list link (IR57); only undefined routes use SCR-X-not-found. AT-REV18-003 sends restored(power) after power loss before checking tamper. IR81 checks for obsolete text also apply to acceptance-plan CSVs.
- G1-014: SCR-C08 has summaries.get(kind=customer) as a supporting Query and displays unresolved alert count (alertCount, IR51) separately from unread count.
- G1-015: "Pending" in AT-C12-E① means perUnit.releaseState=requested for less than 30 seconds after remove Command creation (display "Waiting for release response (connection lost)"). At 30 seconds or later it is failed (aggregate remains release_requested).
- G1-016: "Received information" in UX-05 is a summary; component-contracts.csv is authoritative for props (IR72 priority 5).
- G1-017: Add jobId to SCR-P03 url_selection.
- G1-018: D06 confirmed-overlap checks exclude the active Assignment being replaced for the same jobId.
- G1-019: Acceptance tests that assume listed states (AT-A10-B, AT-C12-B, and submitted in AT-P01-N) create those states through normal operations under IR97 item 3.
- G1-020: EmissionFactor units are fixed to kgCO₂e/kWh (IR88). BR-A14 required fields are region, year, and source. Remove the old "Factor units do not match" wording from AT-A14-E.
- G1-021: AT-P05-B③ checks that submitting a report containing not_inspected without a reason returns VALIDATION and cannot proceed to quality review (IR100).
- G1-022: Model location consent only with purpose=location_automation. The old "General usage consent" has no consent record. AT-C05-B① uses "Location consent granted=false".
- G1-023: The "old heartbeat" in AT-T12-E② is restored(axis=connection, sequence=4) sent after communication_lost(sequence=5). Under SR20 it does not change state; offline remains.
- G1-024: State required inputs and steps explicitly in AT-C13-N, AT-C09-N, AT-A15-N, AT-T10-N, and AT-T11-N.
- G1-026: S08 uses a job for customer-b's unit-other-customer. contractor-a declines, the job is offered again to contractor-b, and tech-external-b is assigned (IR94 scope conditions).
- G1-027: Follow IR98.
- G1-028: Add switchMembership(demoMembershipId) to AppShell events; ShellContainer runs demoSession.switchMembership.
- G1-029: AT-A09-E③ is "No customer Membership can receive the notice (no client can read all target Units); schedule → VALIDATION (IR05)".


## IR103 Policy duration and A12 acceptance evaluation order — G120-001

D08 duration starts when a saved, enabled Policy first evaluates a fresh/valid Fact meeting the threshold at the current tick. Do not use pre-save history or a late Fact's observedAt as the start time. elapsedSeconds=now−condition-start tick. With durationSeconds=60, elapsed time is 0 at the start, the condition does not qualify at 59 seconds, and it first qualifies at 60 seconds. Discard the start tick on bad quality, stale data, disconnection, or a nonmatching condition (D08). A new save starts the counter at 0. Internal evaluation and fire at the same tick must not count time twice.

After this duration condition is met, evaluate the air-quality alert policy’s notifications under SR25 (since IR108 an air-quality policy is an alert policy and has no ventilation candidate). Before it is met, notifications are suppressed/not_due and this Policy creates no control candidate (Units with no other candidates have results=suppressed/no_match). After it is met, ventilation capability, busy state, or restrictions do not suppress notifications. This is a reversible demo detail under DEC-60.

AT-A12-N/B and AT-G120-005 use fixture `acceptancePatches["AT-A12-N"]` in this order.

1. Set clock=2026-09-14T01:00:00.000Z and simulator=false. hq-operator saves the Policy with input and observes its id as policyId in subsequent notification evaluation.
2. Send evaluation to automations.fire with co2=1100 ppm for both Units, observedAt=occurredAt=01:00:00Z, and quality=valid. At the start, target Policy notifications are suppressed/not_due and it creates zero Commands.
3. Advance the test clock 59 times in 1-second ticks. This is normal elapsed time on the injected clock, not one demo.advanceClock jump. Retained Facts are fresh because they remain within the Sensor TTL of 120 seconds. At 01:00:59Z, the target Policy has created zero Alerts, Notifications, and Commands.
4. Advance one more normal 1-second tick. In internal evaluation at 01:01:00Z, the target Policy creates one Alert per Unit and one Notification to hq-operator per Unit (severity=warning, inApp, simulated). unit-online-rto gets one ventilate low Command; unit-non-rto gets none. Do not include seed notifications from other Policies in these counts.
5. Passing finalEvaluation to automations.fire at the same tick returns that tick's committed result (D02/IR54). results is requested for supported equipment and suppressed/invalid_capability for unsupported equipment; notifications is created for both. Same-tick references and same-eventId retries add no Alerts, notifications, or Commands. Do not reevaluate with a new observedAt.

Object steps in fixture.flow are the machine-readable form of this procedure. operation=policies.save/automations.fire uses inputRef input; clock.tick means seconds normal ticks (stepSeconds=1); assert checks the elapsedSeconds row in expected.boundaries. Boundary alertCount/notificationCount/commandCount are totals across all Units from this new Policy. Also use expected.results for the final FireResult and counts. Application tests have not run.

## IR104 Business notification categories and severity — G120-002 and G120-003

When generating notifications under IR95, determine the required Notification type/severity from this table. Only the alert template derives its type from the source Alert rather than using the template name, and retains sourceAlertId (IR10). Other templates have sourceAlertId=null. Business severity defaults are reversible demo proposals under DEC-61, not company approval.

| templateKey | type | severity |
|---|---|---|
| alert | maintenance→cleaning_due; sensor/tamper/reconciliation_required→fault; quality→quality | sourceAlert.severity |
| quality | quality | sourcePolicy.severity |
| schedule_change | schedule_change | normal |
| report_return | report_return | normal |
| completion | completion | normal |
| payment | payment | normal |
| payment_reminder | payment_reminder | warning |
| restriction | restriction | scheduled/requested/applied/release_requested→warning; released/cancelled→normal |
| inquiry | inquiry | normal |
| job_update | job_update | normal |
| device_operation | device_operation | warning |

Policy-based alert notifications (including air-quality alert policies, IR108) inherit SR28 input severity from the source Alert; do not overwrite it with this table's business-notification defaults. The quality template is the D08/SR21 quality notification and uses sourcePolicy.severity. restrictions.schedule notices and notifications.preview use the same table with the target's current state. device_operation.failed is an asynchronous system event (IR59), so actor=system-demo. The creating Membership is not excluded as the actor and is included as a recipient if it currently has read access. Deduplicate recipients to one notification per Membership under IR95.

## IR105 Allergen observation change notifications — G120-004

Adding an IR98 allergen observation emits one ChangeEvent.entityType='allergen_observation' in the same save transition. entityId is the new allergenObservations row id, version=1, occurredAt is the demo clock now at save, and changedFields=['allergenObservation']. Assign the row id through normal Repository ID generation and set createdAt=now. A retry with the same DemoTrigger.eventId adds neither a row nor a change event. Distinguish observedAt from creation time.

Allow allergen_observation in events.subscribe resources and invalidate only telemetry.series under the IR71 table. Determine visibility from current read scope for the Unit and subscription unitIds. Do not expose observation IDs outside scope (D15). Follow existing rules for data-free cursor_only events. C07/A12 subscribes to this resource and fetches the latest observation again. Adding an observation with an old observedAt also triggers refetch, but does not change the displayed value unless it is the latest under IR98 order. Keep IR83 same-tick batching and SR14 page-snapshot rules.

## IR106 Notification fixture scope version at creation — G120-005

When IR91 normalizes notification rows in demoSeed and acceptancePatches, if scopeVersionAtCreation is omitted, set it from Membership.scopeVersion found through recipientMembershipId after all patches are applied. This is the fixture's creation-time snapshot. Later Membership.scopeVersion changes do not alter existing notification values. Validate an explicit scopeVersionAtCreation as a nonnegative integer and keep it; do not rewrite it to match current scopeVersion.

If the recipient Membership is absent, the source scopeVersion is not a nonnegative integer, or an explicit value is not a nonnegative integer, fail fixture generation. AT-C08-SRC notif-alert-insulation-a / notif-alert-unknown-a fills in customer-a scopeVersion=1. Runtime notification generation saves the current Membership scopeVersion at notification creation under IR95/D08; later authorization uses current scope. validate_documents.py checks explicit values or their source for the seed and every notification patch.

## IR107 Permission model of 38 values — Figma 2026-10-01

User decision 2026-10-01 (Figma Admin 03-1…03-7, confirmed as final). `Permission` has 38 values: Read/Write per resource (asset, identity, device, alert.policy, job, contract, billing, restriction, automation.policy, energy, mrv, offset), `dashboard.read`, `alert.read`, `audit.read`, and independent actions `device.maintain`, `control.execute`, `control.diagnose`, `alert.resolve`, `billing.payment`, `restriction.override`, `mrv.review`, `mrv.factors`, `partner.accept`, `partner.assign`, `partner.review`. This replaces the 22 `.manage`-style values: every former `X.manage` maps to `X.read` for read operations and `X.write` for write operations in the operation catalog; `payments.confirm`, `payments.recordManual`, and payouts writes need `billing.payment`; `mrv.recordReview` needs `mrv.review`; `factors.save` needs `mrv.factors`. Write implies Read: saving a Membership with a Write permission and without its Read adds the Read (the UI locks Read on). Only another HQ identity administrator may grant `identity.write` or `restriction.override`; self-grant is FORBIDDEN. Fixture actors keep their previous capabilities by expanding each former value (billing.manage → billing.read/write/payment, mrv.manage → mrv.read/write/review/factors). Access & roles lists HQ/contractor/technician memberships only; client accounts are managed through `clientUsers.*` (IR111).

## IR108 Customer-owned alert policies attached by units — Figma 2026-10-01

Alert policies (`kind=alert`) belong to one customer (`customerId`, fixed after creation) and have one AlertCondition (metric, operator, threshold, recoveryThreshold, durationSeconds, activeWindow, severity). Units carry policies: `ACUnit.alertPolicyIds` is the only attachment source and is changed only by `units.setAlertPolicies` with the full list; every ID must be an alert policy of the unit's customer (otherwise NOT_FOUND). `Policy.unitIds` of an alert policy is the derived list of units carrying it and is not accepted as input (`AlertPolicyInput` has no unitIds). The HQ default policy (`kind=default_alert`, id policy-default, customerId=null) applies to every unit implicitly, cannot be attached, detached, or deleted (VALIDATION), and holds six DefaultAlertRules; its limits are edited only by HQ alert.policy.write and affect all units. `policies.setDefaultRule` stores one DefaultRuleSetting per (policy, rule, customer); a disabled rule is not evaluated for that customer's units. `policies.delete` first removes the policy from every unit's alertPolicyIds in the same change set, then deletes it. Separate air-quality policies (`kind=air_quality`) are removed: CO₂/PM2.5 limits are alert policies, which create Alerts and notifications only and never Commands; AT-A12-N therefore expects zero Commands. Acceptance inputs for alert policies carry `attachUnitIds` at the case level, applied with `units.setAlertPolicies` right after `policies.save` at the same tick.

## IR109 Client structure is read-only; rename and group control — Figma 2026-10-01

Client sessions cannot call `properties.save`, `properties.archive`, `spaces.save`, `spaces.archive`, or `units.save` (FORBIDDEN). `locations.rename` renames a property, space, or unit of the session's customer: name trimmed 1–120 characters, unique among siblings (same parent, case-insensitive; a duplicate is CONFLICT `error.duplicateSiblingName`), expectedVersion required; it changes only the name and version. HQ uses `locations.rename` or the existing save operations. Group control (FR-C14) is owner-only (`Membership.clientRole=owner`) and limited to units of one space; the UI sends one `commands.create` per unit with its own idempotency key and expectedUnitVersion; there is no batch operation and no rollback across units. The earlier rule “no room-wide bulk control” is replaced by this rule.

## IR110 Client feedback, ventilation log, filter care, and export — Figma 2026-10-01

`ventilation.log` records a manual ventilation (method, 1–240 minutes, co2AtLog from the latest valid CO₂ reading of the room or null) and never creates a Command or a notification. `jobs.rate` is allowed only when the job is completed and the caller is a client of its customer; the first call sets customerConfirmedAt; updates are allowed until rating.editableUntil (ratedAt of the first call + 7 days); afterwards CONFLICT. Completed jobs without a rating are confirmed automatically 7 days after completedAt (rating stays null). `jobs.reportProblem` creates a new job (status requested, same unit, type reactive, followUpOfJobId, followUpClass=pending); `jobs.classifyFollowUp` sets rework or new_request once. Filter care: runHoursSinceCleaning sums intervals with effectivePowerState=on since lastCleanedAt; unknown connection makes it null and status unknown; due_soon at ≥ 80 % and overdue at ≥ 100 % of thresholdHours; crossing 100 % creates one Alert type=maintenance (cleaning_due) per cycle. `energy.exportReport` is a read that returns a demo ReportFile; `Preferences.monthlyReportEmail` defaults to false.

## IR111 HQ and partner operations added from Figma — 2026-10-01

Client users: email unique per customer (case-insensitive); the last active owner cannot be demoted, disabled, or removed (CONFLICT); `clientUsers.resendInvite` returns a preview only. CSV import: `units.importPreview` writes nothing and expires after 30 minutes; `units.importCommit` with an expired or changed preview is CONFLICT; error rows are skipped; `units.importUndo` within undoUntil archives created entities only when none has telemetry or jobs. Warranty: coverage status = contract when an active contract covers the unit, else under_warranty when now < warrantyEndsAt − 90 days, expiring when within 90 days, otherwise no_coverage. Firmware campaigns: waves ascend and end at 100; a wave starts only when the previous wave reached 95 % success; failures above autoPauseFailurePercent within a wave pause the campaign; devices with an active diagnostic run, firmware operation, or open tamper are skipped with reasonKey. Contractors: suspension blocks `jobs.offer` (CONFLICT) but not existing offers or jobs; rate cards are versioned by effectiveFrom (future only). Certificates: `members.eligible` uses only valid verified certificates; a pending renewal does not extend eligibility; `certificates.verify` approve sets the matching QualificationGrant.validUntil. Unavailability sets available minutes to 0 for the dates and is reported in Capacity.unavailability. SLA metrics follow FR-A22 definitions; targets apply to jobs created at or after effectiveFrom. Payouts: `payouts.generate` creates or replaces draft statements for an ended month from jobs accepted in that month priced by the rate card effective at acceptance; approved/paid statements are untouched; approve → paid only on or after payDate; contractors see approved/paid statements only; `payouts.query` adds a job note; `payouts.resolveQuery` with adjustmentMinor adds an adjustment line to the next draft. Technician: `units.resolveQr` returns NOT_FOUND for units outside the technician's assignments; `jobs.checkIn` follows the `jobs.start` rules (IR76/IR89/IR94) and additionally records arrival; location_qr requires distanceMeters ≤ 200 and qrUnitId = job unit, otherwise only manual with reason; `jobs.pauseWork` only in in_progress; `reports.signOff` binds to reportVersion and is cleared by the next `jobs.saveDraft`.

## IR112 Shared UI conventions and two-step verification — Figma 2026-10-01

Desktop screens are 1920×1080 responsive layouts (sidebar + main; columns before scrolling); modals and dialogs are centered on the whole viewport, not the content area. Customer/Property/Unit/Device filters are search-selects that show the first 20 options and search the rest on the server as the user types (GitHub branch-picker style). Status chips use the shared StatusBadge vocabulary; power uses Running/Stopped/Unknown and connection Online/Offline. Forbidden (`/forbidden`) and not found (`*`) render one shared “Page unavailable” view (403/404 wording does not reveal existence, IR57). Two-step verification (FR-X08) is demo only: `twoFactor.enable` accepts any 6 digits and returns 8 recovery codes once; `twoFactor.disable` requires a 6-digit code; demo sign-in is unchanged. New DTO fields absent from demoSeed default to null or [] (ACUnit.alertPolicyIds=[], warrantyEndsAt=null; MaintenanceJob.followUpOfJobId/followUpClass/timeOnSite/rating/customerConfirmedAt=null, warrantyClaims=[]; WorkReport.refrigerant=[], signOff=null; Membership.clientRole=owner for seed client actors, null otherwise).

## IR113 Maintenance scheduling: preferred times, time proposals, partner acknowledgement, and job origin — 2026-10-02

User request 2026-10-02 (Figma Client 07d/07k–07n, Admin 06-11–06-16, Contractor 02-22–02-24, Technician 02-33/02-34 and the “Maintenance scheduling — cross-role flow” board). The client chooses the visit time; nobody else may book a time the client did not choose or accept.

1. **Origin.** `MaintenanceJob.origin` is `periodic_plan` when `planId≠null` and `client_request` otherwise (including jobs HQ enters on a client's behalf with `jobs.create`). Every job list and detail of every role shows the origin badge (Client request / Periodic plan); `jobs.list` accepts `filters.origin`.
2. **Preferred times.** A client `jobs.create` sends the 1st preferred time as `requestedStart/requestedEnd` and exactly 2 `alternativeSlots` (2nd, 3rd). The three slots must be distinct, in the future and start on a later calendar day than now (Asia/Kuala_Lumpur), each 1–4 hours long; otherwise VALIDATION (`fieldErrors.alternativeSlots`). HQ on a client's behalf may send 0–2 alternatives. The Repository stores `preferredSlots=[requestedSlot, ...alternativeSlots]`, `preferenceRound=1`; `requestedSlot` and the IR38 `dueAt` default are unchanged. Plan-generated jobs have `preferredSlots=[]`.
3. **Agreed slot.** For a job of either origin, the *agreed slots* are: a slot in the current `preferredSlots`, the slot of a `SlotProposal` with status `accepted`, or (periodic_plan without an accepted reschedule) the plan occurrence slot `[occurrenceAt, occurrenceAt+requested duration)`. `jobs.assign` (start/end) and `jobs.offer` (`visitSlot`) must use an agreed slot exactly; otherwise VALIDATION `errors.slot_not_agreed` with zero side effects. `jobs.offer` stores `Offer.visitSlot`; the contractor's later `jobs.assign` must use `Offer.visitSlot`. `JobOfferSummary.visitSlot` replaces candidate date ranges on partner screens.
4. **HQ proposal.** `jobs.proposeSlot` (job.write) is allowed while the job is `requested` (or `offered`/`accepted`/`assigned` after the partner or technician reported it cannot make the agreed slot) and no SlotProposal is pending. The slot must not be one of the current `preferredSlots` (VALIDATION `errors.slot_is_preferred`; book it directly instead). `hold` names the HQ technician or contractor whose capacity is reserved; `message` 1–1000 characters; `replyBy` in the future and at most 7 days ahead. The job status does not change; `slotProposal.status=pending` makes the UI status `time_proposed` (`JobDisplayStatus`), counted separately in pipelines but not in `AdminSummary.jobCounts`. `jobs.withdrawProposal` sets `withdrawn` and releases the hold.
5. **Client answer.** `jobs.respondProposal` (client of the job's customer, pending proposal only). *accept*: proposal `accepted`; if `hold.kind=internal`, the same change set performs `jobs.assign` for the held technician at the proposal slot (status `assigned`); if `contractor` and no Offer is open, it performs `jobs.offer` with `visitSlot`=proposal slot and the default offer/access windows of DD-A06 (status `offered`); if `source=contractor`, it updates the open Offer's `visitSlot` (status stays `offered`, the partner proposal becomes `approved`). *decline*: requires `declineReason` (`not_home`/`too_late`/`other`), optional comment 0–1000; with 3 `preferredSlots` (same rules as item 2) the job gets the new slots and `preferenceRound+1`; status stays `requested` (or `offered` for a partner proposal, whose Offer keeps the old `visitSlot`). At `replyBy` an unanswered proposal becomes `expired` (same as decline without new slots) and HQ receives a notification to call the client. Declining never cancels; `jobs.cancel` stays allowed for `requested` jobs including those with a pending proposal.
6. **Periodic visits.** `jobs.requestReschedule` (client, `periodic_plan` job in `offered`/`accepted`/`assigned`, at least 48 hours before the scheduled start) revokes the Offer/Assignment, stores 3 `preferredSlots`, increments `preferenceRound` and returns the job to `requested`; HQ then follows items 3–5.
7. **Contractor time change.** `jobs.proposePartnerSlot` (receiving contractor, own open Offer, status `offered`, no pending partner proposal, qualified own technician) stores `PartnerSlotProposal` (pending) and keeps the Offer reserved; `jobs.accept` is CONFLICT (`errors.partner_proposal_pending`) until it is resolved; `jobs.withdrawPartnerSlot` withdraws it. `jobs.resolvePartnerSlot` (job.write): `send_to_client` creates a SlotProposal (`source=contractor`, hold = that contractor/technician, `replyBy` required) and marks the partner proposal `sent_to_client`; `keep` marks it `kept` and the agreed `visitSlot` stays. An agreed time never changes without the client's acceptance.
8. **Technician acknowledgement (受領).** `jobs.assign` creates the Assignment with `acknowledgement=pending`. `jobs.acknowledgeAssignment` by the assigned technician sets `accepted` (`acknowledgedAt`) or `cant_make` (reason 1–1000, optional `alternativeSlot`). `jobs.start` is unaffected by the acknowledgement (IR76/IR89 still decide); `cant_make` notifies the coordinator (HQ for internal jobs, the receiving contractor otherwise), which reassigns (new Assignment, acknowledgement pending) or asks HQ to propose a new time (item 4). Reassignment keeps the agreed slot.
9. **Notifications.** One in-app notification per event to the role that has to act, excluding the actor (IR95): new client request → HQ; proposal / partner proposal sent to client → client; accept / decline / expiry → HQ; offer created or visit slot updated → contractor; partner proposal → HQ; assignment created → technician; acknowledgement accepted/cant_make → coordinator, and accepted → client (technician name visible from then on). templateKey `schedule_change`, type `job` (IR104).
10. **Fixtures.** demoSeed jobs, Offers and Assignments carry these fields (normalized in 0.23.0). Jobs, Offers and Assignments created by acceptance patches without them default to: `origin` from `planId`, `preferredSlots=[requestedSlot]` for `client_request` and `[]` otherwise, `preferenceRound=1`, `slotProposal=null`, `partnerSlotProposal=null`; seed Offers default `visitSlot` to the job's `scheduledSlot` or else `requestedSlot`; seed Assignments default `acknowledgement=accepted` with `acknowledgedAt=createdAt` (they predate IR113), `cantMakeReason=null`, `alternativeSlot=null`. Cases written before IR113 that book job-internal-a at 2026-09-21 10:00–12:00 use `acceptancePatches["shared:ir113-agreed-slots"]`, which makes that slot one of the client's preferred times.

## IR114 Customer users (owner), coordination notes, and follow-up classification screens — 2026-10-06

User decision 2026-10-06 (DEC-65): the three use-case gaps found in the use case diagrams get screens (Figma Client 11a–11c and 07o–07p, Admin 06-17–06-18) and the rules below. Documents follow Figma; nothing else changes.

1. **Customer users (FR-C19, SCR-C19 `/customer/users`).** The sidebar item Users and the page exist only for clientRole=owner; a member opening the route gets the shared Page unavailable view (FORBIDDEN, IR112). Owners call `clientUsers.list` for their own customer, `clientUsers.save` without id and with clientRole=member (clientRole=owner or a save with id → FORBIDDEN), and `clientUsers.resendInvite` for status=invited only (otherwise VALIDATION, zero previews). E-mail uniqueness is case-insensitive (IR111). Role changes, disabling, password resets, and removal stay HQ-only (DD-A17); `clientUsers.remove` from a client session is FORBIDDEN.
2. **Coordination notes (FR-C09).** Clients call `jobs.addNote` only with visibility=customer (internal → FORBIDDEN), only on their own jobs whose status is not completed or cancelled (otherwise CONFLICT), with a message of 1–2000 characters (VALIDATION, input kept). A note never changes slots, assignee, or status. Notifications (IR95): one job_update notification to HQ and, for a delegated job, to the contractor; the assigned technician sees the note in job events.
3. **Follow-up classification (FR-C17, FR-A06).** Jobs with followUpClass=pending show “↩ Follow-up of <followUpOfJobId>” and a classify-by time (createdAt + 1 business day, display only) in the Admin Jobs list. `jobs.classifyFollowUp` requires job.write, followUpClass=pending (otherwise CONFLICT — classification is set once), and a reason of 1–1000 characters (VALIDATION). rework links to followUpOfJobId and is not billable; new_request is a normal billable request. Either way the job stays requested and is booked under IR113. The client sees “Under HQ review” until classification, then “Rework (free)” or “New request”; one job_update notification goes to the client.
4. **Figma IDs.** Client 07o (Add note dialog), 07p (note sent), 11a (Users list), 11b (Invite member dialog), 11c (duplicate e-mail VALIDATION); Admin 06-17 (follow-up detail) and 06-18 (Classify dialog). The Client sidebar component gains Users (variant Active=users), shown for owners only.

## IR115 Consistency check across use cases, user flows, screens, and documents — 2026-10-07

User decision 2026-10-07 (DEC-66): the consistency check between the Figma Use Cases / User Flows / wireframe pages and these documents is resolved as follows. Figma and documents now say the same thing.

1. **Admin dashboard permissions (FR-A01).** Billing figures and the billing link need `billing.read` (otherwise billingVisibility=forbidden, amountsByCurrency=null, “Not permitted”); the Energy analysis link needs `energy.read`, the permission of the target screen. `billing.write` and `energy.write` are not required to see them. The pre-IR107 names `billing.manage`, `energy.manage`, `restriction.manage`, `device.manage` and `job.manage` are not permissions and must not appear in screens or contracts (use billing.read, energy.read, restriction.read/write, device.write and job.write).
2. **Default-policy rules are owner-only for clients (FR-C15).** `policies.setDefaultRule` from the customer app requires clientRole=owner; a member sees the six rule toggles read-only (“Only the account owner can change this”) and a direct call is FORBIDDEN with zero side effects. HQ keeps switching rules per customer with alert.policy.write. Own customer policies (`policies.save` / `policies.delete` / `units.setAlertPolicies`) stay available to owners and members.
3. **Follow-up classification belongs to FR-A06.** Admin 06-17/06-18 and use case UC-A25 trace to FR-A06 (DD-A06, job.write) with FR-C17 as the client-side origin (IR114 item 3).
4. **Tabs.** SCR-T01 `tab=today` (default) / `tab=all`; SCR-T04 `tab=indoor` (default) / `outdoor` / `electrical` (the Readings / Parts & refrigerant / Time on site switch is local state); SCR-A04 adds `tab=firmware`; SCR-A06 `jobs/plans/contractors/sla`; SCR-A08 `invoices/inquiries/payouts`; SCR-A15 `records` (default) / `market`; SCR-A16 `log` (default) / `devices`; SCR-C08 `overview/policies`; SCR-C09 `overview/filter-care`; SCR-A02 `overview/users/policies/warranty`; SCR-P06 `overview/certifications` and the URL key `orgId` (own company only; any other value NOT_FOUND, AT-P06-E①).
5. **Location operations.** Adding a location in Admin 02-9 calls `properties.save` (property) or `spaces.save` (floor/area/room); there is no `locations.save`. `locations.rename` is the shared rename.
6. **Technician device URL.** Technician device screens use the device ID in `deviceId` (`device-ac-001` for the device registered with serial `ac-001` in AT-T11-N); serials are shown as labels only.
7. **Illustrative IDs.** Figma examples that are not in the fixture keep their role-page meaning and are not seed data: job-c07 (Client Bedroom AC #2 request used for the IR113 scheduling walkthrough), job-c09 (follow-up of job-c02, IR114), mrv-a14-a / mrv-aug-a, policy-peak-a, restriction-x06, stmt-2026-09 and similar. Acceptance cases use only fixture-contract IDs.
8. **Cross-cutting requirements.** FR-X03 (status and quality labels) and FR-X06 (capabilities and contract types) have no use case of their own; every use case that shows telemetry or capability-gated actions applies them, and the use-case catalogue notes this.
9. **Voice assistant scope.** FR-X02 is offered only in the Client app header (Figma Client 09a–09g, web/ AppShell); HQ, Contractor, and Technician screens do not carry FR-X02.

## IR116 Frontend stack, production backend, and network design — 2026-10-07

User decision 2026-10-07 (DEC-67): the frontend stack is Next.js (App Router), as already implemented in `web/`, and the production backend and network are designed at a logical, cloud-agnostic level. Phase 1A scope, screens, operations, and acceptance criteria do not change.

1. **Frontend stack.** TypeScript strict + React + Next.js App Router replaces the Vite + React Router SPA proposal ([common design §1](common.md#1-structure-and-responsibilities)). The layered rules stay: pages call feature hooks, hooks call the Repository interface, only the composition root selects the adapter and the router (Navigation interface). In production the Next.js app is also the BFF; the browser never calls the Core API directly.
2. **Backend.** A modular monolith Core API implements the 197 operations exactly as catalogued (`POST /v1/ops/<operation>`, ServiceResult / DomainError with fixed HTTP status mapping, Idempotency-Key, expected versions), with separately deployed IoT, scheduler, notification, import / export, webhook, and outbox workers ([backend architecture](backend-architecture.md)).
3. **Network.** Only the edge is public (`app`, `hooks`, `mqtt`, `files`); application and data zones are private; devices connect outbound only over MQTT with mutual TLS; egress uses an allowlist ([network architecture](network-architecture.md)).
4. **Status.** Both documents are PROPOSED; hosting, region, AC device interface, payment and messaging providers, retention, and fleet size remain OPEN (OPEN-BE-01…06, OPEN-NW-01…04). They are not Phase 1A acceptance inputs.

## IR117 AWS, Stripe, HQ network restriction, retention, and capacity — 2026-10-07

User decision 2026-10-07 (DEC-68) for the production target in the [backend architecture](backend-architecture.md) and [network architecture](network-architecture.md). Phase 1A is unchanged.

1. **Hosting.** AWS; primary region ap-southeast-5 (Malaysia), DR and services not yet available there in ap-southeast-1 (Singapore). Component mapping: backend architecture §3a, network architecture §2a.
2. **Payments.** Stripe Checkout (MYR; cards and FPX as enabled). `payments.simulate` becomes a Checkout Session; PaymentConfirmed is set only after a verified webhook and a PaymentIntent retrieval; restriction release keeps IR35. Contractor payouts stay outside Stripe until OPEN-BE-07 is decided.
3. **HQ network restriction.** The HQ admin app is served only on `admin.<domain>`, allowed by WAF only from the company network (office and company VPN egress addresses). Admin-role sessions are issued only there, and the Core API rejects admin-role operations without that session mark (FORBIDDEN → shared Page unavailable). Other roles are not restricted.
4. **Retention and capacity.** Set by the design (backend architecture §7 and §13): billing records 7 years, audit 7 years in S3 Object Lock, raw telemetry 35 days hot / 13 months in S3, hourly aggregates 7 years; launch 3,000 units, design capacity 20,000 units. Confirm retention with legal counsel before go-live.
5. **Customer office firewall requirements.** Outbound TCP 8883 (or 443 WSS) to `mqtt.<domain>` by name, no TLS inspection, DNS and NTP, no inbound (network architecture §5a).
6. **Still OPEN.** AC manufacturer interface (OPEN-BE-02), WhatsApp provider (OPEN-BE-04), payout rail (OPEN-BE-07), cellular SIM provider (OPEN-NW-02).

## IR118 Backend implementation in Go + Echo and database design — 2026-10-07

User decision 2026-10-07 (DEC-69): the backend is implemented in Go with the Echo framework. The implementation design is the [backend Go design](backend-go-design.md); the database is the [database design](database-design.md) with the executable [db/schema.sql](db/schema.sql). Phase 1A is unchanged.

1. **API.** One Echo route `POST /v1/ops/:operation` dispatches the 197 catalogued operations; the registry is generated from the operation catalog and every operation has exactly one handler. Inputs are decoded strictly; DomainError uses the nine `ErrorCode` values with a fixed HTTP mapping (VALIDATION 422, UNAUTHENTICATED 401, FORBIDDEN 403, NOT_FOUND 404, CONFLICT 409, OFFLINE 409, TIMEOUT 504, RATE_LIMITED 429, UNAVAILABLE 503).
2. **Transactions.** Each operation runs in one PostgreSQL transaction that sets `app.tenant_id` for row-level security, writes audit and outbox rows before commit, and uses optimistic versions; scoped zero-row writes are NOT_FOUND (D01), version mismatches CONFLICT.
3. **Database.** Aurora PostgreSQL 16, one schema per module, no cross-schema foreign keys, tenant-scoped foreign keys, business rules encoded as constraints where they need no clock or other module (database design §5), partitioned telemetry and audit with the IR117 retention.
4. **Verification.** db/schema.sql was applied to PostgreSQL 16 and the core Go pipeline was compiled and tested against it on 2026-10-07 (backend Go design §1). These are design checks, not Phase 1A acceptance.

## IR119 Everything runs in Docker — 2026-10-07

User decision 2026-10-07 (DEC-70): all application components run as Docker images in every environment; production data and messaging stay AWS managed services. Design: [container design](container-design.md); files: repository-root `compose.yaml`, `web/Dockerfile`, `docker/`.

1. **Images.** `ac-web` (Next.js standalone; demo mock mode and production BFF) and `ac-backend` (Go binaries api, webhook, worker, migrate; dev target adds iotbridge and devicesim). Distroless, non-root, read-only root filesystem, linux/arm64 (+ amd64 for developers), secrets only at run time.
2. **Local and CI.** Docker Compose profiles `demo`, `infra`, `schema`, `backend`, `full`, `obs`, `stripe`; containers stand in for Aurora (PostgreSQL 16), ElastiCache (Valkey), SQS/SNS/Kinesis/S3/SES/Secrets Manager (LocalStack), IoT Core (Mosquitto + iot-bridge), Cognito (Keycloak), Stripe (stripe-mock / Stripe CLI), weather (WireMock).
3. **Staging and production.** The same image digests run on ECS Fargate (container design §5); Compose is never used there.
4. **Toolchain.** Go 1.25 or later (current pgx v5 requires it); Node 22 LTS for the web image. The Phase 1A demo can be started with `docker compose --profile demo up`; Phase 1A scope is unchanged.


## IR120 Alert-policy details fixed for the backend — 2026-10-08

Gaps found while implementing `policies.*` (production backend, IR118). Phase 1A screens keep their behavior; the client default-rule label “Refrigerant leak” becomes “Refrigerant low pressure” (BR-A05 wording; no leak-detection claim).

1. **Default rules.** `policy-default` holds exactly these six DefaultAlertRules (order fixed). Their limits are fictional demo values:

| ruleKey | name | category | metric | operator | threshold | recoveryThreshold | durationSeconds | severity |
|---|---|---|---|---|---|---|---|---|
| ventilation_co2 | Ventilation | air_quality | co2 | gte | 1000 | 900 | 600 | warning |
| dust_pm25 | Dust / filter | air_quality | pm25 | gte | 35 | 30 | 1800 | warning |
| refrigerant_low_pressure | Refrigerant low pressure | fault | refrigerant_pressure | lte | 350 | 380 | 120 | critical |
| compressor_short_cycling | Compressor short-cycling | fault | compressor_cycles | gte | 4 | 3 | 10800 | warning |
| clogged_filter | Clogged filter | maintenance | airflow_drop | gte | 30 | 20 | 3600 | normal |
| ac_offline | AC offline | connection | heartbeat_gap | gte | 15 | 1 | 1 | warning |

2. **Editing default limits.** `policies.save` also accepts `DefaultPolicyInput` (`{id, kind:'default_alert', rules}`) from HQ alert.policy.write only (expectedVersion required). `rules` must list the six ruleKeys in the order above with unchanged name and category; each condition follows item 3. Saving changes the limits for every unit; per-customer settings are kept. A kind the caller is not granted (for example a client sending `default_alert` or `automation`) is FORBIDDEN.
3. **Alert condition and policy fields.** name trimmed 1–120; metric for HQ temperature/humidity/co2/pm25/refrigerant_pressure/vibration/power (default rules also use their own metric), for clients temperature/humidity/co2/pm25/power; operator gt/gte/lt/lte; threshold and recoveryThreshold finite numbers with recovery strictly below the threshold for gt/gte and strictly above for lt/lte; durationSeconds integer 1–86400; activeWindow null or `{weekdays: ISO 1–7, 1–7 entries, no duplicates; startLocal, endLocal: HH:mm, different}` (an end before the start runs past midnight; weekdays are start days); severity critical/warning/normal; channels 1–3 distinct values containing inApp; recipientMembershipIds 1–20 distinct IDs, each an active membership that can read the customer's alerts (HQ with alert.read or a client of the customer's organization), otherwise VALIDATION; escalateAfterMinutes and cooldownMinutes integers 1–1440; timezone an IANA name; priority integer 0–100. customerId must be an active customer (unknown NOT_FOUND); a client may use only its own customer (otherwise NOT_FOUND); customerId cannot change on update (VALIDATION `error.ownerFixed`).
4. **Client editor values.** The DD-C15 editor does not show recipients, escalation, cooldown, timezone, or priority; it sends `recipientMembershipIds=[session membership]`, `escalateAfterMinutes=60`, `cooldownMinutes=5`, `timezone=Preferences.timezone`, `priority=50`, and on edit keeps the stored values.
5. **Reads.** `policies.list` filters: kind (alert/default_alert/automation), customerId, propertyId, unitId, enabled. The visible kinds are those of the caller's granted alternatives; clients see the default policy and their own customer's policies. `ruleSettings` of the default policy contain only the caller's customer for clients and only the `customerId` filter's customer when given. unitId/propertyId list the policies attached to those units (the implicit default policy is not listed); an automation policy matches customerId when one of its units belongs to that customer. Default order: default_alert, alert, automation, then name (case-insensitive) and id; sort fields name, priority, updatedAt (id breaks ties).
6. **Delete.** `policies.delete` of an alert policy detaches it from every unit in the same change set (IR108); of an automation policy removes it with its unit targets; the default policy is VALIDATION `error.defaultPolicyFixed`. Result `{id, deleted:true}`.
7. **Default-rule switch.** `policies.setDefaultRule`: policyId must be the default policy and ruleKey one of item 1 (VALIDATION); reason trimmed 0–500. expectedVersion is the version of that customer's DefaultRuleSetting from `policies.get`; a rule without a stored setting is enabled with version 0. Clients: own customer only (other NOT_FOUND), owner only (IR115).
8. **Attachment errors.** `units.setAlertPolicies` follows IR108: an unknown policy or a policy of another customer is NOT_FOUND; the default policy or a duplicate ID is VALIDATION.
9. **Automation policies (HQ, DD-A11).** Fields follow IR120 item 3 for name, timezone, enabled, priority; `unitIds` 1–500 distinct active units of the tenant (unknown or archived NOT_FOUND); `condition` is exactly one of occupancy `{occupied}`, peak `{active}`, tariff `{operator, value ≥ 0, unit:'MYR_per_kWh'}`, solar/battery `{operator, value ≥ 0, unit:'kW'}` with no other fields; `action` is one UnitAction with exactly its own field and must be supported by every target unit's capability (power needs control; temperature on min/max/step; mode and fan level from the model lists; ventilate only on ventilation models), otherwise VALIDATION `error.unsupportedAction`. Alert-only fields on an automation policy, or condition/action on an alert policy, are VALIDATION. An update replaces the unit targets and clears `disabledReason`. Restrictions and busy units are evaluated when the policy fires, not when it is saved.

## IR121 Telemetry and ventilation reads fixed for the backend — 2026-10-08

Gaps found while implementing `telemetry.*` and `ventilation.*` (IR118). Phase 1A behavior is unchanged.

1. **Measurement DTO.** `Measurement.id` equals `eventId` (one ingest event per reading; seeded readings use the fixture ID); `version` is 1 and `createdAt`/`updatedAt` equal `receivedAt`; `isDemo` stays the contract literal `true`. Readings live in the 35-day hot table; older ranges return no rows from `telemetry.series`.
2. **telemetry.series.** Exactly one of `unitIds` (1–20 distinct) or `spaceId`; `metric` required; `sensorId` optional; `from < to` and `to − from ≤ 35 days` (VALIDATION otherwise). Every unitId must be readable by the caller (otherwise NOT_FOUND); `spaceId` targets the caller-readable units whose `spaceId` is that space (an unknown space or one with no readable unit is NOT_FOUND). Rows of every quality are returned (missing/stale stay visible as such); order `observedAt asc, sensorId asc, id asc`; Page limits per D12. `allergenObservation` follows DEC-57 when the target resolves to exactly one unit (rows stored in `monitoring.allergen_observations`), otherwise null.
3. **telemetry.summary.** `unitIds` 1–100 distinct, all readable (otherwise NOT_FOUND); optional metric; `from < to`. `measurements` holds the latest reading in `[from, to)` per unit and metric (any quality), ordered unitId, metric; `asOf` is the request time; `energy` is null (energy figures come from `energy.summary`).
4. **ventilation.log (client).** `spaceId` must be a space of the caller's customer (otherwise NOT_FOUND); `unitId` optional and must be in that space (VALIDATION `error.unitNotInSpace`); method enum; `durationMinutes` integer 1–240. `co2AtLog` is the latest `valid` co2 reading at or before the request time of `unitId`, or of any unit in the space when unitId is absent; null without one. `loggedAt` is the request time; no Command and no notification.
5. **ventilation.list.** Filters spaceId, unitId, from/to (half-open on `loggedAt`); default order `createdAt desc, id asc`. Scope: clients their customer's spaces; technicians logs whose unit or space contains a unit in their scope; HQ alert.read the tenant.

## IR122 Job intake, notes, hold and history details for the backend — 2026-10-08

Gaps found while implementing the Maintenance job operations (IR118). Phase 1A behavior is unchanged.

1. **jobs.create.** `unitId` must be readable by the caller (client: own customer; otherwise NOT_FOUND) and not archived (VALIDATION `error.unitArchived`); `symptom` is trimmed and 10–2000 characters; slots follow IR113 item 2 (calendar days in Asia/Kuala_Lumpur); `contactWindow` follows IR64; `dueAt` follows IR38. The job starts `requested` with `origin=client_request`, `customerOrgId` of the unit, `preferredSlots=[requested, ...alternatives]`, `preferenceRound=1`, and one `job.created` event.
2. **jobs.addNote.** message trimmed 1–2000; clients may only send `visibility=customer` (an internal note from a client is FORBIDDEN); notes are refused on completed or cancelled jobs (CONFLICT). The note bumps the job version and adds a `note.added` event that references the note.
3. **jobs.hold / jobs.resumeHold.** reason trimmed 1–1000. `jobs.hold` moves `in_progress` or `submitted` to `on_hold` and stores the reason; `jobs.resumeHold` moves `on_hold` back to `in_progress` (deterministic contracts: a submitted report stays as is and a new draft starts) and clears the reason; other states are CONFLICT.
4. **jobs.cancel.** IR56 table; cancelReason trimmed 1–1000; undecided Offers are closed (decision decline, reason `job_cancelled`) and active Assignments revoked in the same change set.
5. **JobEvent DTO.** `version` 1, `createdAt`/`updatedAt` = `occurredAt`; actions used: `job.created`, `job.cancelled`, `job.held`, `job.resumed`, `note.added`. Event IDs are UUIDv7 so events of the same instant keep their write order under `occurredAt asc, id asc`. Clients do not receive events whose note is internal.
6. **jobs.list filters.** `from/to` apply to `requestedSlot.startAt`; `organizationId` matches the delivering contractor (`contractorOrgId`), and the HQ organization matches internal jobs (`contractorOrgId=null`); `membershipId` matches the technician of the active Assignment; `overdueOnly=true` keeps jobs with `now > dueAt` that are not completed or cancelled; `status` and `statuses` together, an empty `statuses`, or unknown enums are VALIDATION. `severity` and the sort rank follow IR23 (JobSummary.severity from the unit's open/acknowledged alerts).

## IR123 Offer, decision and assignment details for the backend — 2026-10-08

Gaps found while implementing `jobs.offer`, `jobs.accept`, `jobs.decline` and `jobs.assign` (IR118). Phase 1A behavior is unchanged.

1. **jobs.offer.** IR48 inputs plus: `contractorOrgId` must be an active organization of kind contractor (otherwise NOT_FOUND); a suspended ContractorProfile is CONFLICT `errors.contractor_suspended` (IR111); `termsVersion` 1–64 characters; the access window must cover `visitSlot` (`accessValidFrom ≤ startAt`, `endAt ≤ accessValidUntil`); `visitSlot` is an agreed slot (IR113 item 3). Success: Job `offered`, `contractorOrgId` set, one Offer (decision null), event `job.offered`; result JobDetail.
2. **jobs.accept / jobs.decline.** The Offer must be the job's undecided Offer of the caller's organization (otherwise NOT_FOUND); expired (now ≥ offerExpiresAt) is CONFLICT `errors.offer_expired` (IR86); accept requires `termsVersion` equal to the Offer's (otherwise CONFLICT `errors.terms_changed`); decline reason trimmed 1–1000 (IR87). Accept: Offer `accept`, Job `accepted`; decline: Offer `decline`, Job back to `requested` with `contractorOrgId=null`. Both add an event (`offer.accepted` / `offer.declined`) and return JobDecisionReceipt with the new job version.
3. **Required qualifications.** A job requires the codes of `qualificationRequirements` for every entry of the unit's `serviceScope`; a technician qualifies when each code has a QualificationGrant with `validFrom ≤ startAt`, `endAt ≤ validUntil` and no `revokedAt` (otherwise FORBIDDEN `errors.qualification_missing`).
4. **jobs.assign.** HQ (job.write) assigns internal jobs only (`contractorOrgId=null`, internal technicians); the contractor (partner.assign) assigns jobs of its accepted Offer within the access window and only technicians of its own organization (otherwise FORBIDDEN `errors.technician_not_allowed`). The technician must be an active role=technician membership whose scopes contain the unit (IR94 `errors.technician_out_of_scope`). Allowed states and slots: from `requested` (HQ) or `accepted` (contractor) the slot must equal an agreed slot (contractor: `Offer.visitSlot`, IR113); from `assigned` the slot must equal the current `scheduledSlot` (reassignment keeps the agreed slot); from `in_progress`, `rework_requested` or `on_hold` (D06) an extension or reassignment keeps `startAt`, needs `endAt ≥` the current end and a reason (IR49); other states are CONFLICT. reason trimmed 0–1000. Overlapping active Assignments of the technician are CONFLICT. Success revokes the previous active Assignment, creates one with `acknowledgement=pending`, sets `assignmentId`/`scheduledSlot`, moves `requested`/`accepted` to `assigned` (other states stay), and adds event `job.assigned`.

## IR124 Contractor and technician job projections in the backend — 2026-10-08

Gaps found while implementing IR23/IR24/IR49 projections (IR118). Phase 1A behavior is unchanged.

1. **Storage.** A JobHistorySnapshot is stored once in `maintenance.job_history_snapshots` per job and owner — the contractor organization (accepted Offer whose access window ended) or the technician membership (Assignment whose viewing window ended, or revoked after it started). The worker tick freezes ended windows at most one tick after the end from the job values at that moment; it never rewrites a stored snapshot. `redactedReportSummary` is `{hasReport, acceptance}` from the report state at freeze time; `ownDecisionEvents` are the owner's own `offer.accepted` / `offer.declined` events (note and reportRef null). Assignments revoked before their work window started create no snapshot.
2. **Contractor projection.** Offer projection: own Offer with decision null and `now < offerExpiresAt`, or decision accept and `now < accessValidFrom`; summary/detail: accepted Offer with `accessValidFrom ≤ now < accessValidUntil`; history: own organization's snapshots. `JobOfferSummary.requiredQualifications` are the IR123 item 3 codes; `siteAddress` follows IR25.
3. **Technician projection.** Summary/detail while the user's active Assignment is in its viewing window `[createdAt, scheduledEnd)`, for external technicians also inside the accepted Offer's access window; history: the user's own snapshots while the membership is active.
4. **jobs.list union.** Offers and histories are filtered with the IR23 rules (private filters fail; period filters use requestedSlot.startAt for offers and completedAt for histories), merged with summaries, then sorted and paged together (status rank on the public status, severity and dueAt null last, ties by id).
5. **jobs.get / jobs.events.** `jobs.get` returns the projection valid now (detail → offer → history, otherwise NOT_FOUND). `jobs.events` returns all events during the detail projection (contractors and technicians see internal notes of delegated jobs) and only the caller's own decision events under the offer and history projections.

## IR125 Technician on-site operations in the backend — 2026-10-08

Details fixed while implementing `jobs.start`, `jobs.checkIn`, `jobs.pauseWork` and `jobs.acknowledgeAssignment` (IR118). Phase 1A behavior is unchanged.

1. **Access.** start, checkIn and pauseWork follow the IR94 technician write table for the job (own active Assignment, work window); expectedVersion is the job version and every success bumps it and adds one event (`job.started`, `job.checked_in`, `job.paused`, `job.resumed_work`).
2. **jobs.start.** `startConfirmed` must be true; only `assigned` moves to `in_progress`, setting `startedAt` and `timeOnSite.startedAt`; other states are CONFLICT.
3. **jobs.checkIn.** `location_qr` needs `distanceMeters` 0–200 and `qrUnitId` equal to the job's unit (VALIDATION `errors.too_far` / `errors.qr_unit_mismatch`); `manual` needs a reason trimmed 1–1000. Allowed from `assigned` (it also starts the job, IR111) or from `in_progress` before an arrival was recorded; it records `arrivedAt`, method, distance and reason.
4. **jobs.pauseWork.** Only `in_progress`; `paused=true` opens a pause and `paused=false` closes the open one; pausing twice or resuming without an open pause is CONFLICT. The job status does not change.
5. **jobs.acknowledgeAssignment.** The caller's active Assignment within its viewing window (otherwise NOT_FOUND, or FORBIDDEN `errors.assignment_ended` after it); only `pending` can be answered (CONFLICT otherwise); `accept` takes no other fields; `cant_make` needs a reason trimmed 1–1000 and an optional valid alternative slot. The job version is bumped (`assignment.accepted` / `assignment.cant_make` events).
6. **Offer expiry (IR48).** The worker tick returns expired jobs to `requested`, sets `Offer.expiredAt` (decision stays null) and adds one `offer_expired` event by the system actor; only Offers without `expiredAt` count as the job's open Offer, so HQ can offer the job again.

## IR126 Work reports in the backend — 2026-10-08

Details fixed while implementing `jobs.saveDraft`, `jobs.submit`, `jobs.review`, `jobs.resumeRework` and `reports.get` (IR118). Phase 1A behavior is unchanged.

1. **Versions.** A report keeps one ID per job; each row `(id, version)` is one version. `jobs.saveDraft` without `reportId` creates version 1 when the job (in_progress or after a resume) has no report yet (otherwise CONFLICT `errors.draft_exists`); with `reportId` it updates the job's current draft and bumps its version (expectedVersion = report version, branch `reportId present`). `jobs.submit` freezes that version (`reportVersion` must equal the draft's version, otherwise CONFLICT); `jobs.resumeRework` and `jobs.resumeHold` after a submission copy the latest version into a new draft with version + 1. `MaintenanceJob.draftReportRef` is the current draft, `reportRefs` the frozen versions in ascending order.
2. **Draft inputs (SR07).** Items: no duplicate componentKey, componentGroup must be the key's group (indoor: filter, evaporator_coil, blower_motor, blower_fan, drain_pipe, drain_pan, outlet, louver; outdoor: condenser_coil, compressor, fan, blade, refrigerant_pipe; electrical: thermostat, sensor, capacitor, contactor, wiring), reason trimmed 0–1000, evidenceIds and attachmentIds must be Attachments of the same report. Measurements: componentKey present among the draft's items, unknown IDs NOT_FOUND; the expected unit per metric is temperature °C, humidity %, co2 ppm, pm25 µg/m³, power kW, vibration mm/s, refrigerant_pressure kPa, compressor_cycles cycles/h, airflow_drop %, heartbeat_gap min — a different unit is stored with quality `suspect` / `unit_mismatch`, a null or non-finite value with `missing`. Parts: name 1–120, quantity integer 0–999 while drafting; refrigerant: cylinderId 1–64, kilograms ≥ 0; workText 0–4000; nextAction `none` or `follow_up` with note 0–1000.
3. **Submit.** IR100 checks (items equal the unit's components, results non-null, reasons for non-normal results, workText 10–4000, nextAction non-null with a future follow-up date and a 1–1000 note, parts quantity 1–999, attachments ready, measurement units match) return VALIDATION with all field errors. Success: report `submitted`, job `submitted`, `timeOnSite.finishedAt` = now and `onSiteMinutes` = minutes from arrival (or start) to now minus closed pauses, event `job.submitted` with the reportRef.
4. **Review.** Contractors (partner.review) review jobs delivered by their organization with `reviewMode=normal`; HQ (job.write) reviews internal jobs with `normal` and outsourced jobs only with `hq_escalation` and a reason. The reviewer's user must differ from the report author (FORBIDDEN `errors.self_review`). The job must be `submitted` and `reportVersion` the submitted version (CONFLICT otherwise). accept → report `accepted`, job `completed` (`completedAt`); return → reason trimmed 1–1000, report `returned`, job `rework_requested`. One `report_reviews` row and event `job.reviewed`.
5. **reports.get.** Clients read accepted versions of their own jobs; contractors read non-draft versions of jobs their organization delivers; technicians read the jobs they may write (IR94 work window); HQ job.read reads every version. `reviewAvailability` is computed for the caller (`permission_denied`, `self_authored`, `not_current`, `not_submitted`).

## IR127 Follow-up, cost, access and warranty operations in the backend — 2026-10-08

Details fixed while implementing `jobs.rate`, `jobs.reportProblem`, `jobs.classifyFollowUp`, `jobs.saveCost`, `jobs.extendAccess` and `jobs.recordWarrantyClaim` (IR118). Phase 1A behavior is unchanged.

1. **jobs.rate (IR110).** Client of the job's customer, job `completed`; stars integer 1–5; tags distinct values of On time, Clean work, Explained clearly, Polite, Fixed the problem; comment trimmed 0–1000. The first call sets `customerConfirmedAt` and `rating.editableUntil = ratedAt + 7 days`; later calls replace stars/tags/comment until then (CONFLICT afterwards). The worker confirms completed jobs without a rating 7 days after `completedAt` (`customerConfirmedAt` set, rating stays null).
2. **jobs.reportProblem.** Client of the job's customer, job `completed`, within 7 days of `completedAt` (CONFLICT otherwise); details trimmed 10–2000; photos 0–5 JPEG/PNG of 1 byte–5 MiB (`BlobInput.bytes` travels as base64 in JSON); `preferredSlot` null or an IR113 slot. It creates a `requested` reactive job on the same unit with `followUpOfJobId`, `followUpClass=pending`, `symptom=details`, `origin=client_request`; with a preferred slot it is the requested slot and the only preferred slot, without one the requested slot is the next Kuala Lumpur day 09:00–12:00 and `preferredSlots=[]` (HQ contacts the client). Photos become ready Attachments of the new job. The source job's version is bumped.
3. **jobs.classifyFollowUp.** HQ job.write; only a job with `followUpClass=pending` (CONFLICT otherwise); classification rework or new_request; reason trimmed 1–1000 (stored as `followUpReason`).
4. **jobs.saveCost.** HQ job.write; not for cancelled jobs (CONFLICT); 0–50 lines replace the costs; each line kind estimate/actual, amountMinor integer ≥ 0, currency MYR or USD, description trimmed 1–200, visibility internal/customer.
5. **jobs.extendAccess.** HQ job.write; the job's accepted Offer (otherwise CONFLICT); new `accessValidUntil` later than both now and the current end; reason trimmed 1–1000. The Offer's access end moves; the job version is bumped.
6. **jobs.recordWarrantyClaim.** HQ job.write; job `completed`; the unit's `warrantyEndsAt` must exist and not be before `completedAt` (VALIDATION `errors.outside_warranty`); partLabel trimmed 1–120; amountMinor integer > 0; reason trimmed 1–1000. Appends a WarrantyClaim with state `filed` and `filedAt` = now.
7. **IR42 projections.** JobDetail applies IR42 for every role, and `offer` is the job's latest Offer that was not declined or expired (contractors only their own company's, without declineReason).

## IR128 Time proposals and reschedules in the backend — 2026-10-08

Details fixed while implementing the IR113 proposal operations (IR118). Phase 1A behavior is unchanged.

1. **jobs.proposeSlot.** Allowed for `requested`, for `offered`/`accepted` when the job's Offer has a partner slot proposal, and for `assigned` when the active Assignment is `cant_make` (otherwise CONFLICT); no pending SlotProposal (CONFLICT). The slot follows IR113 item 2 and must not be a current preferred slot (VALIDATION `errors.slot_is_preferred`); message trimmed 1–1000; `replyBy` after now and at most 7 days ahead. The hold is checked like an assignment or offer: `internal` needs an internal technician in scope and qualified for the slot; `contractor` needs an active, not suspended contractor organization and, when a technician is named, one of its technicians in scope and qualified. Job status and version: status unchanged, version + 1, event `proposal.sent`.
2. **jobs.respondProposal accept.** Proposal `accepted`. Internal hold: the held technician is assigned at the proposal slot (IR123 assignment effects). Contractor hold without an open or accepted Offer: an Offer is created with `visitSlot` = slot, `offerExpiresAt = min(now + 24 h, slot start)`, `accessValidFrom = now`, `accessValidUntil = slot end + 24 h` and the contractor's latest `termsVersion` (`terms-demo-v1` when none). With an open Offer of that contractor (including `source=contractor`): its `visitSlot` becomes the slot and a pending/sent partner proposal becomes `approved`. With an accepted Offer: its `visitSlot` moves to the slot, an active Assignment is revoked and the job returns to `accepted` for the contractor to assign again.
3. **jobs.respondProposal decline.** declineReason required; comment trimmed 0–1000; preferredSlots empty or exactly 3 slots following IR113 item 2 (they replace the preferred slots and increment `preferenceRound`). A partner proposal sent to the client becomes `declined`. Status unchanged.
4. **Expiry.** The worker tick marks pending proposals with `replyBy ≤ now` as `expired` (same effect as a decline without new slots).
5. **jobs.withdrawProposal / jobs.withdrawPartnerSlot.** Only pending proposals (CONFLICT otherwise); HQ withdraws SlotProposals, the proposing contractor its own partner proposal.
6. **jobs.requestReschedule.** Client of the job's customer; `periodic_plan` job in offered/accepted/assigned; at least 48 hours before the scheduled start (the scheduled slot, else the Offer visit slot); exactly 3 preferred slots (IR113 item 2); comment trimmed 0–1000. The undecided Offer is closed (decline `rescheduled`), an accepted Offer's access ends now, an active Assignment is revoked, and the job returns to `requested` with `contractorOrgId`, `assignmentId` and `scheduledSlot` cleared, the new preferred slots and `preferenceRound + 1`.
7. **Partner proposals.** `jobs.proposePartnerSlot`: the caller's open Offer (`offered`, not expired), no pending or sent partner proposal, technician of the caller's organization in scope and qualified, slot following IR113 item 2, reason trimmed 1–1000. While one is pending or sent, `jobs.accept` is CONFLICT `errors.partner_proposal_pending`. `jobs.resolvePartnerSlot` (HQ): `send_to_client` requires no pending SlotProposal and `replyBy` (item 1 rules) and creates a SlotProposal with `source=contractor` and hold `{contractor, contractorOrgId, technicianMembershipId}`; `keep` marks it `kept`.

## IR129 Attachments and sign-off in the backend — 2026-10-08

Details fixed while implementing `attachments.add`, `attachments.getContent` and `reports.signOff` (IR118). Phase 1A behavior is unchanged.

1. **Transport.** `BlobInput.bytes` travels as base64 in JSON. The `Blob` result of `attachments.getContent` is `{name, mime, size, bytes}` with base64 bytes; the web adapter turns it into a browser Blob. Files are stored in the blob store (S3 in staging/production, a local directory in development).
2. **attachments.add.** IR94 technician write table; the job is `in_progress` and `reportId` is its current draft (otherwise CONFLICT); expectedVersion is the draft version. The file is JPEG or PNG, 1 byte–5 MiB, size equal to the payload, name trimmed 1–200; at most 10 attachments per report (CONFLICT `errors.too_many_attachments`). The Attachment is `ready` at once (the production malware scan keeps it `processing` until clean). The draft version increases and its sign-off is cleared.
3. **reports.signOff (IR111).** IR94 technician write table; `reportVersion` must be the current draft's version (= expectedVersion, otherwise CONFLICT); signerName trimmed 1–120; exactly one of `signature` (PNG, 1 byte–5 MiB) or `absentReason` (trimmed 1–1000) with `sitePhoto` (JPEG/PNG, 1 byte–5 MiB). The files become Attachments of the report; SignOff `{signerName, signedAt, signatureAttachmentId, absentReason, sitePhotoAttachmentId, reportVersion}` replaces any earlier one; the report version does not change. The next `jobs.saveDraft` or `attachments.add` clears it.
4. **attachments.getContent.** The attachment must belong to the report and job, and the caller must be able to read that report version under IR126 item 5 (otherwise NOT_FOUND).

## IR130 Maintenance plans in the backend — 2026-10-08

Details fixed while implementing `plans.*` (IR118); D16 stays authoritative. The schema follows D16 (intervalMonths 1–12, anchorDay 1–31).

1. **plans.save.** unitId must be an existing, not archived unit of the tenant (unknown NOT_FOUND, archived VALIDATION); `recurrence.kind=monthly`, intervalMonths integer 1–12; nextDueAt after now. Create stores `anchorDay` = UTC day of nextDueAt; an update with a changed nextDueAt stores the new day; unitId cannot change after creation (VALIDATION `error.unitFixed`). expectedVersion follows the `id present` branch.
2. **plans.generateNext.** `occurrenceDate` must equal the saved nextDueAt (otherwise CONFLICT `errors.occurrence_mismatch`); `occurrenceDate ≤ now` is CONFLICT `errors.next_date_past`; an occurrence that already has a job is CONFLICT. The job follows D16; the plan's nextDueAt advances by intervalMonths to the anchorDay (clamped to the month's last day) at the same UTC time, and the plan version increases.
3. **Reads.** `generatedOccurrences` lists the plan's jobs (occurrenceAt ascending). plans.list filters unitId / customerId / propertyId; order id asc; sort fields id, createdAt, updatedAt.

## IR131 Contractor register, rate cards and SLA in the backend — 2026-10-08

Gaps found while implementing `contractors.*`, `rateCards.*` and `sla.*` (IR118). Phase 1A behavior is unchanged.

1. **contractors.save.** organizationId must be an existing organization (NOT_FOUND) of kind contractor (VALIDATION); one profile per organization (a second create is CONFLICT); registrationNo trimmed 1–64; serviceAreas 1–20 distinct values trimmed 1–80; contactEmail a valid address ≤ 254; insuranceValidUntil null or any instant. `name` is the organization name. `delegation` is `[createdAt, insuranceValidUntil)` when insurance is set and later than createdAt, otherwise `[createdAt, createdAt + 365 days)`. `rateCardId` is the rate card effective now (latest effectiveFrom ≤ now) or null.
2. **contractors.setOfferStatus.** reason trimmed 1–1000; setting the current status again is CONFLICT; `suspendedReason` holds the reason while suspended and is cleared on reactivation; open offers and jobs are unchanged (IR111).
3. **Rate cards.** `rateCards.save` needs an existing contractor profile (NOT_FOUND), effectiveFrom after now (VALIDATION), currency MYR or USD, 1–4 lines with distinct workType, amountMinor integer ≥ 0 and note trimmed 0–200; each save is a new row whose `version` is the contractor's previous highest + 1. `rateCards.list` filters contractorOrgId (contractors see only their own) and orders effectiveFrom desc.
4. **SLA targets.** `sla.saveTargets` stores a new row per plan type (version = previous highest + 1 for that plan type) with effectiveFrom ≥ now; responseHours integer 1–168; percentages 0–100. A job's plan type is its customer's `serviceProfile`; its target is the latest row with effectiveFrom ≤ job createdAt, otherwise the default 4 h / 90 % / 85 %.
5. **Scorecard metrics** (jobs created in the period; `contractorOrgId` limits to jobs delivered by that contractor). Response: hours from job creation to the accepted Offer (contractor) or first Assignment (internal); a job without one counts as a miss once the target time has passed. Arrival in window: jobs with an arrival time — arrival inside the scheduled slot. First-time fix: completed jobs whose report was accepted without a return and whose unit got no follow-up job within 30 days of completion. Rating: average stars and count of rated jobs. Open overdue: open jobs past dueAt. Percentages are rounded to one decimal; no data is null. Customer status: `breached` when a metric misses its target by more than 10 points (response: share within target) or the customer has open overdue jobs; `at_risk` when a metric misses by up to 10 points; otherwise `on_track`. Breaches list every response, arrival, first-time-fix and overdue miss (newest first, at most 50).
6. **Contractor KPIs** (90 days ending now). offerAcceptance: accepted ÷ (accepted + declined + expired) Offers offered in the window; arrivalInWindow as item 5; firstTimeAccepted: share of jobs whose first submitted report version was accepted; averageRating / ratingCount; reworkRate: share of completed jobs that got a follow-up classified `rework`.

## IR132 Memberships, eligibility, capacity and unavailability in the backend — 2026-10-08

Details fixed while implementing `members.*` (IR118). Phase 1A behavior is unchanged.

1. **members.save.** userId an existing user, organizationId an existing organization (NOT_FOUND otherwise). Organization kind per role: admin → operator, client → customer, contractor → contractor, technician → operator (`employment=internal`) or contractor (`external`); employment is required for technicians and null for other roles (VALIDATION). Permissions are distinct values allowed for the role — admin: all 38; contractor: partner.accept, partner.assign, partner.review; technician: alert.read, alert.resolve, control.diagnose, device.maintain; client: none — and every `<resource>.write` needs its `<resource>.read` (VALIDATION). Scopes per role: admin `tenant` (at most one); client and contractor `organization` = their own organization; technician `unit`, `property` or `organization` (a customer organization); every reference must exist in the tenant (VALIDATION). validFrom < validUntil; external technicians need validUntil; reason trimmed 1–1000. userId, organizationId and role are fixed after creation. Granting identity.write or restriction.override to a membership of one's own user is FORBIDDEN; removing identity.write from, or ending, the last active identity.write holder is CONFLICT. A change of permissions or scopes increments `scopeVersion`; a new client membership gets `clientRole=member` and the IR84 initial Consent.
2. **members.list.** Filters role, organizationId, active (true/false at now); client memberships appear only with `role=client`. Contractors see the role=technician memberships of their own organization with the IR42 projection (permissions and scopes empty). Order id asc.
3. **members.eligible.** The job must be readable by the caller (HQ any job; contractor jobs of its accepted Offer); `startAt < endAt`. Candidates are active technicians — internal ones for HQ, the contractor's own for contractors — whose scopes contain the unit (IR94), qualified for the slot (IR123 item 3), without unavailability on the slot's Kuala Lumpur dates and without an overlapping active Assignment on another job.
4. **members.capacity.** `date` is a Kuala Lumpur calendar date (YYYY-MM-DD). Work time is 09:00–17:00 Asia/Kuala_Lumpur on Monday–Friday (IR70); weekends and unavailability days have no available time (`availableMinutes = 0`, `utilization = null`). `assignedSlots` are the technician's active Assignments clipped to the work time; `assignedMinutes` is their union; `utilization` = assigned ÷ available × 100 rounded to one decimal. HQ sees every technician, contractors their own.
5. **members.setUnavailability.** from ≤ to, both YYYY-MM-DD, at most 31 days; type enum; note trimmed 0–500. `membershipId` names a technician of the caller's organization (contractor) or any technician (HQ); null applies to the caller's whole organization. `conflictingAssignmentIds` lists active Assignments overlapping the dates; they are not changed.

## IR133 Certificates and parts catalog in the backend — 2026-10-08

Details fixed while implementing `certificates.*` and `parts.list` (IR118). Phase 1A behavior is unchanged.

1. **certificates.submit.** Contractor (partner.assign) for an active technician of its own organization (otherwise NOT_FOUND); code a QualificationCode or `other`; name trimmed 1–120; number trimmed 1–64; issuedAt < expiresAt; file PDF/JPEG/PNG of 1 byte–10 MB (base64 bytes, IR129); `renewalOf` optional and must be a certificate of the same membership and code (NOT_FOUND otherwise). The certificate starts `pending_verification`; the file is kept in the blob store and its name in `fileName`.
2. **Status.** Stored states are `pending_verification`, `rejected` and `valid` (approved). A valid certificate reads as `expired` when `expiresAt ≤ now` and `expiring` when it expires within 30 days.
3. **certificates.verify.** HQ job.write; only `pending_verification` (CONFLICT otherwise). approve: status valid, `verifiedByMembershipId`/`verifiedAt` set; for a QualificationCode the membership's QualificationGrant of that code becomes `[issuedAt, expiresAt)` without `revokedAt` (created when missing). reject: reason trimmed 1–1000 (stored as `rejectionReason`, audited); a pending renewal never changes the grant.
4. **certificates.requestTraining.** Contractor for a certificate of its own technician; note trimmed 1–1000; sets `trainingRequestedAt`.
5. **certificates.list.** Filters membershipId, code, status (the read status of item 2); HQ sees all, contractors their own organization; order expiresAt asc, id asc.
6. **parts.list.** Active catalog items ordered by name, then code; filter `search` (case-insensitive code or name contains) and `membershipId` (that technician's van stock first, IR200). Van stock is not tracked in the backend yet: `vanStockQuantity` is null.

## IR134 Filter care in the backend — 2026-10-08

Details fixed while implementing `filterCare.*` (IR118); this resolves the DD-C18 / IR110 wording for unknown run time.

1. **Run time.** `runHoursSinceCleaning` adds, for every valid measured power reading with a value > 0 since the last cleaning, the time to the next reading capped at the sensor's stale limit (D07; the last reading is counted up to now with the same cap), in hours rounded to one decimal. It is null while the unit's connection is not online or when the unit has no power reading since the last cleaning.
2. **Last cleaning.** The latest of a customer `filterCare.markCleaned` (`lastCleanedBy=customer`) and a completed job whose accepted report has the filter item with result `normal` (`technician`, `lastCleaningJobId`).
3. **Status.** With run time: `overdue` at ≥ 100 % and `due_soon` at ≥ 80 % of `thresholdHours` (setting, or the model default 250 h), else `ok`. Without run time, `fallbackDays` decides from the last cleaning date with the same percentages; with neither run time nor a cleaning date the status is `unknown` (IR110).
4. **Settings.** One FilterCareSettings per customer (defaults: thresholdHours null, fallbackDays 30, recipients owners, channels [inApp]). `filterCare.saveSettings`: client owner only (FORBIDDEN for members, IR115); thresholdHours null or integer 50–2000; fallbackDays integer 7–180; recipients owners/all_users; channels 1–2 distinct of inApp/email containing inApp; each save increments the version (no expectedVersion, write version catalog).
5. **Operations.** `filterCare.list`: clients their customer's units, technicians their scoped units; filters customerId, propertyId, spaceId, unitId, status; order status desc (overdue, due_soon, ok, unknown) then unitId. `filterCare.markCleaned`: a not archived unit of the caller's customer (NOT_FOUND / VALIDATION); records a cleaning at now and returns the unit's status. The cleaning_due Alert on crossing 100 % is raised by the worker together with notifications.

## IR135 Contracts, invoices and reminders in the backend — 2026-10-08

Details fixed while implementing `contracts.*`, `invoices.*` (IR118). Phase 1A behavior is unchanged.

1. **contracts.save.** customerId must be an active customer (NOT_FOUND / VALIDATION); unitIds 1–500 distinct, not archived units of that customer (VALIDATION `error.otherCustomerUnit`; a unit on another contract is allowed); planType enum; startAt < endAt; priceMinor integer ≥ 0; currency MYR or USD; `restrictionEligible=true` only for `rto` and then `rulesVersion` 1–64 characters is required (otherwise null). Each save writes a new version row (`is_current` moves); customerId is fixed after creation. A contract with active restrictions or an unresolved recovery case cannot be saved (CONFLICT `errors.contract_restricted`). `activeRestrictionIds` are its restrictions in scheduled/requested/applied/release_requested; `hasUnresolvedRecovery` is true when a recovery case is not resolved.
2. **contracts.list.** Filters customerId, unitId, kind (the plan type, IR200); clients see their own customer's current contracts; order id asc.
3. **invoices.create.** contractVersion must be the contract's current version (CONFLICT); period from < to and inside the contract term; amountMinor integer > 0 in the contract currency; dueAt after now (IR90) and not before the period start; one invoice per contract and period (CONFLICT). The number is `INV-<YYYYMM of the period start in Asia/Kuala_Lumpur>-<4-digit tenant sequence>`, drawn under a per-tenant lock (IR199). The invoice starts `unpaid`.
4. **Reads.** invoices.list filters customerId, contractId, status, overdueOnly (now > dueAt and not paid); order dueAt desc, id; clients see their customer's invoices. invoices.get returns InvoiceDetail with its payments (newest first) and the restrictions citing it.
5. **invoices.remind (IR04).** Recipient: an active client membership of the invoice's customer; channel inApp/email/whatsapp; reason trimmed 1–1000. The notification row records its `channel` (new column: inApp simulated, email/whatsapp preview).

## IR136 Payments in the backend — 2026-10-08

Details fixed while implementing `payments.*` (IR118). The schema follows the contract: `Payment.method` is a PaymentMethod or null (manual), the payment channel is demo / stripe_checkout / manual, `paymentReference` is unique within the tenant, and simulated event IDs are stored.

1. **payments.simulate (client, IR59/IR60).** `initiate`: own invoice in `unpaid` (CONFLICT otherwise), method demo_credit_card/demo_debit_card, `demoConfirmed=true`; creates an `initiated` Payment for the full amount; expectedVersion and version bump on the invoice. `instructions`: returns a NotificationPreview (payment template, channel inApp, deliveryState preview) and changes nothing except the version check. `processing` (initiated → processing; invoice processing), `confirm` (processing → confirmed with paymentReference 1–128 unique in the tenant; invoice paid) and `fail` (initiated/processing → failed; invoice back to unpaid) use the payment version. A repeated eventId returns the payment unchanged; confirm after failed and fail after confirmed are CONFLICT.
2. **payments.confirm (HQ billing.payment).** An existing `processing` payment; paymentReference 1–128 unique; confirmedAmountMinor equal to the invoice amount and currency equal (VALIDATION); reason trimmed 1–1000; the payment becomes confirmed (method unchanged) and the invoice paid.
3. **payments.recordManual (HQ billing.payment).** Only an invoice without any Payment (CONFLICT otherwise; the same invoice/reference/amount again returns the existing confirmed payment); same field rules; creates a confirmed manual Payment with method null; the invoice becomes paid.
4. **Release trigger (IR35 ①).** In the same change set as an invoice becomes paid, restrictions citing it whose cause invoices are all paid move scheduled → cancelled and requested/applied → release_requested with `releaseIntent {source: payment, at, actorMembershipId}`. Release Commands follow the restriction release evaluation (Restrictions module).

## IR137 Contractor payouts in the backend — 2026-10-08

Details fixed while implementing `payouts.*` (IR118); BR-A23 decides the source jobs. The schema follows PayoutQuestion (topic, states open/answered/adjusted, message 1–2000, signed adjustment and the statement that carries it).

1. **payouts.generate.** `period` YYYY-MM whose month has ended in Asia/Kuala_Lumpur (VALIDATION otherwise). For every contractor, the jobs it delivered whose report was accepted in that month make one draft: a `charge` line per job priced by the rate card effective at the acceptance time — workType `periodic_inspection` for periodic jobs, otherwise `repair_base` — plus a `deduction` line (`rework_deduction`) when the report had been returned; a job without a rate card gets a charge of 0 with the note `no rate card`. Adjustments of answered questions not yet applied are added as `adjustment` lines (positive amounts add to gross, negative ones to deductions). `payDate` is the 15th of the following month; the currency is the rate card's (MYR without one). Existing drafts of the period are replaced; approved and paid statements are untouched; contractors without lines get no statement. Returns the drafts of the period.
2. **payouts.transition.** `approve`: draft → approved (approvedByMembershipId); `mark_paid`: approved → paid, only when today (Kuala Lumpur) is on or after payDate; otherwise CONFLICT; reason trimmed 0–1000.
3. **payouts.query (contractor).** Own approved statement; lineId must be a line of it; topic enum; message trimmed 1–2000. Creates an open question, adds an internal note to the line's job history, bumps the statement version.
4. **payouts.resolveQuery (HQ).** An open question of the statement; reply trimmed 1–2000; adjustmentMinor an optional non-zero integer (state `adjusted`, otherwise `answered`). The reply is added to the job history; the adjustment waits for the contractor's next generated draft.
5. **Reads.** payouts.list filters period, contractorOrgId, status; order period desc, contractor; contractors see their own approved/paid statements only (NOT_FOUND for drafts in payouts.get).

## IR138 Unit commands in the backend — 2026-10-08

Details fixed while implementing `commands.create` and `commands.get` (IR118). Phase 1A behavior is unchanged.

1. **Order of checks.** Unit readable by the caller (client: own customer with control.execute; technician: IR94 table with jobId required; HQ control.execute) else NOT_FOUND/FORBIDDEN; reason absent for clients (VALIDATION when sent) and trimmed 1–1000 for technicians and HQ; `expectedUnitVersion` equal to the unit version (CONFLICT); the action supported by the unit's capability (VALIDATION `error.unsupportedAction`); the IR46 restriction table (FORBIDDEN `errors.restriction_active`, `fieldErrors.action`); a pending Command (requested/sent), an active diagnostic run or a queued/running device operation on the unit (CONFLICT `errors.unit_busy`, D04 mutual exclusion); IR47 delivery (OFFLINE with `errors.device_power_lost` when the bound device's powerSignal is off, else `errors.device_<connection>`; no bound device is `errors.device_unknown`).
2. **Lifecycle.** An accepted command is published at once: `status=sent`, `delivery=sent`, `sentAt=requestedAt=now`, `expiresAt = now + 30 s` (the D03 intent window), `correlationId` = the request's correlation ID, source `ui` (technician and HQ) or `group` is not distinguished in the API. The device acknowledgement (IoT bridge) moves it to `acknowledged` when received before expiresAt and updates the unit's observed state; the worker tick marks `sent` commands past expiresAt as `expired` with failureCode TIMEOUT. Late acknowledgements are recorded only (D03/D04 timing).
3. **commands.get.** Clients read commands of their customer's units, technicians of their scoped units, HQ any.

## IR139 Diagnostic test runs in the backend — 2026-10-08

Details fixed while implementing `diagnosticRuns.*` (IR118); D04 mutual exclusion and IR46/IR47 apply. The schema allows reasons of 1–1000 characters (DD-T10).

1. **diagnosticRuns.create.** Technician with control.diagnose under the IR94 table for jobId and unitId; `expectedUnitVersion` and `expectedJobVersion` must match (CONFLICT); startAction and endAction supported by the capability (VALIDATION) and allowed by IR46 (FORBIDDEN if either is prohibited); durationMinutes integer 1–15; reason trimmed 1–1000; the unit must not be busy (CONFLICT) and must be deliverable (OFFLINE). The start Command is sent with the run (`source=diagnostic`); the run is `awaiting_start`.
2. **Progress.** The start Command's acknowledgement makes the run `running` with `startedAt` = acknowledgement time and `endAt = startedAt + durationMinutes`. Its expiry makes the run `start_failed` (TIMEOUT). When the worker tick reaches endAt, IR46 is checked again: a prohibited endAction gives `end_blocked`; an undeliverable device gives `end_failed` (OFFLINE); otherwise the end Command is sent and the run is `end_requested`, then `completed` on its acknowledgement or `end_failed` (TIMEOUT) on its expiry.
3. **Reads.** `diagnosticRuns.get` / `diagnosticRuns.list` (unitId required, jobId optional, newest first): technicians see runs of jobs they hold or held an Assignment for; HQ job.read or control.execute see all.

## IR140 Restriction schedule, execution, cancellation and reads in the backend — 2026-10-08

Details fixed while implementing `restrictions.schedule/execute/cancel/get/list/forInvoice` (IR118); D03, IR03, IR05, IR19, IR35, IR42 and IR96 apply.

1. **schedule.** The contract must be current with `expectedContractVersion` (CONFLICT), plan `rto`, restrictionEligible, with a rulesVersion and a term containing now (VALIDATION `contractId`); `rulesVersion` must equal the contract's (VALIDATION). `causeInvoiceIds` must equal exactly the contract's overdue unpaid invoices (status=unpaid and dueAt<now; none → CONFLICT, mismatch → VALIDATION). `unitIds` (1–100, no duplicates) must belong to the contract version, not be archived (CONFLICT), support control and, for temperature_limit, temperature with a setpoint inside min/max on the step grid (VALIDATION). A unit with an active restriction or an unresolved recovery case returns CONFLICT. executeAfter ≥ now+24h (VALIDATION). noticeAt=now. One inApp `restriction` notice (severity warning) goes to every active client membership of the customer organization; none returns VALIDATION with nothing saved (IR05). Each unit starts at applyState=not_sent / releaseState=none.
2. **execute.** Requires state scheduled, latest version, `confirmedRulesVersion` = rulesVersion (VALIDATION), now ≥ executeAfter and ≥ noticeAt+24h, no active grace/exception, the current contract version unchanged and every saved notice still pointing at this restriction with occurredAt = noticeAt (otherwise CONFLICT). If every cause invoice is already paid, the restriction becomes cancelled with no Commands. Otherwise one `apply_restriction` Command per unit is created by the internal actor system-restriction (source=restriction, expiresAt = now+30 s): online, powered units without a queued/running device operation get status=sent and applyState=sent_unknown; others get an undelivered intent (status=requested, delivery=not_sent) with applyState=not_sent and pendingReason `offline` or `device_operation_running`. Aggregate state becomes requested.
3. **Command results.** A timely acknowledgement of an apply sets applyState=applied, observedRestriction={restrictionId, rulesVersion, policy, observedAt} on the unit row and the ACUnit; the aggregate becomes applied only from requested once every unit is applied (late acknowledgements never restore applied). A timely remove acknowledgement sets releaseState=released and clears the observation. A failed apply sets not_applied; an expired or failed remove sets releaseState=failed and keeps release_requested; expired apply intents keep not_sent / sent_unknown. Every result increments Restriction.version.
4. **Release evaluation.** cancel (requested/applied), payment confirmation (IR35 ①) and later release routes run the D03 table once per unit at releaseState=none: not_sent → the undelivered apply is cancelled, not_applied/not_required; not_applied → not_required; sent_unknown → waiting_reconcile; applied and deliverable → a remove Command with releaseState=requested; applied but offline → releaseState stays none with pendingReason=offline. The aggregate becomes released when every unit is released or not_required.
5. **Reads.** get/list return the canonical Restriction to admins with restriction.read or restriction.write and RestrictionReleaseView to override-only admins (contractId/invoiceId filters FORBIDDEN). List filters contractId, invoiceId, state; sort id/createdAt/executeAfter/noticeAt, default newest first. `events` is the restriction's audit history in time order. forInvoice requires the invoice to belong to the client's organization (NOT_FOUND) and applies the IR42 client mask; its query filters may only use state.

## IR141 Restriction exceptions, release, override, reconcile and retry in the backend — 2026-10-08

Details fixed while implementing the remaining `restrictions.*` writes (IR118); IR140's Command results and release evaluation apply. All require the latest Restriction version.

1. **defer / exempt.** restriction.write; `until` must be after now and at most 90 days ahead (VALIDATION); reason 1–1000. defer stores graceUntil, exempt stores exception {until, reason}. scheduled keeps its state (execute is CONFLICT while the period is active); requested/applied move to release_requested with source=exception and the D03 evaluation (IR35 ②); release_requested only records the period; released/cancelled CONFLICT. Expiry never reapplies (D03).
2. **release.** requested/applied with all causes paid or an active grace/exception → release_requested with source=manual; unpaid without one → FORBIDDEN; release_requested idempotent (no version or audit); other states CONFLICT.
3. **override.** restriction.override with reason; requested/applied → release_requested with source=override; release_requested idempotent; scheduled and terminal states CONFLICT (a scheduled restriction is cancelled with restrictions.cancel). Invoices stay unpaid. The response is always RestrictionReleaseView.
4. **reconcile.** For requested/applied/release_requested restrictions; released/cancelled restrictions follow IR164 (SR26 recovery cases). Every target unit must be at applyState=sent_unknown (VALIDATION) with no unfinished restriction Command (CONFLICT), an online powered device (OFFLINE) and a unit observation (ACUnit.observedRestriction, lastSeenAt) at most 30 s old (TIMEOUT). Observation of this restriction and rulesVersion → applied (while waiting_reconcile, a remove Command and releaseState=requested); no observation → not_applied (and not_required); any other restriction → CONFLICT. The aggregate then becomes applied (from requested, when every unit is applied) or released (per D03).
5. **retry.** `confirmedRulesVersion` must match (VALIDATION) and target units must belong to the restriction (VALIDATION). apply: restriction.write, state requested, causes still unpaid and no active grace/exception (CONFLICT); applied units are skipped, sent_unknown needs reconcile and an unfinished Command blocks (CONFLICT); others get a new delivered Command or not_sent intent. release: state release_requested; released/not_required units are skipped; waiting_reconcile or not yet applied needs reconcile, a requested remove or unfinished Command blocks (CONFLICT); an undeliverable unit returns OFFLINE with no side effects. When every target already succeeded the current Restriction is returned without a new version. Override-only callers (IR03) may reconcile and retry(phase=release) only for an override release intent; responses are projected to RestrictionRead by current permissions.

## IR142 Notification inbox, recipients, preview, preferences and consents in the backend — 2026-10-08

Details fixed while implementing `notifications.*`, `preferences.*` and `consents.*` (IR118); IR04, IR20, IR58, IR84 and IR112 apply. Business-event notification generation (IR95) is a separate step.

1. **Target readability.** A target is readable when it exists and: HQ holds a permission for its kind (unit: asset.read/alert.read/device.read/control.execute; job: job.read/job.write; invoice and inquiry: billing.read/billing.write; restriction: restriction.read/write/override; device: device.read/device.write); a client's organization is the target's customer organization; a contractor's organization has an Offer on the job; a technician holds an Assignment on the job or a unit/device target is inside the membership scopes. targetName is the unit display name (unit, job, the first restriction unit), the invoice number, the device serial or the inquiry subject type.
2. **notifications.list.** Only the caller's own notifications with readable targets count toward items and total. Filters severity (critical/warning/normal), unreadOnly, type; sort occurredAt (default newest first), severity (normal < warning < critical) or id. params is projected to the canonical shape (targetName, at = stored at or occurredAt, status = stored status or type, the other fields or null); deliveryState is simulated for inApp and preview for email/whatsapp.
3. **notifications.markRead.** Recipient only, readable target (NOT_FOUND), latest version; sets readAt=now and version+1. An already-read notification is returned unchanged. Alerts are not changed (DD-C08).
4. **notifications.recipients / preview.** The caller must be able to read the target (NOT_FOUND). payment_reminder needs HQ billing.write (FORBIDDEN), an invoice target (VALIDATION) and an overdue unpaid invoice (CONFLICT). Recipients are active memberships that can read the target: clients of its customer organization, and, except for payment_reminder, HQ members with a permission for the kind and, for jobs, contractor members of offered organizations and active assigned technicians. allowedChannels is inApp+email, plus whatsapp for clients with a phone number; the channel input keeps only recipients allowing it, role filters, the query accepts limit/cursor only, order is role, label, id. preview requires an eligible recipient (VALIDATION) and trimmed message/reason of 1–1000; it returns an unsaved Notification with deliveryState=preview and a temporary ID.
5. **preferences.** Per user with defaults en / Asia/Kuala_Lumpur / MYR / false when no row exists. update requires locale en|ms and an IANA zone name (UTC or Area/City; VALIDATION); monthlyReportEmail only from client sessions (VALIDATION otherwise) and an omitted value keeps the saved one. twoFactorEnabled is false until FR-X08 is implemented.
6. **consents.** consents.get returns the caller's location_automation record (NOT_FOUND when absent, IR84); the seed loads demoSeed.consents. update needs the latest version: granting sets grantedAt=now and clears revokedAt; revoking keeps grantedAt and sets revokedAt=now; the same value returns the record unchanged.

## IR143 Inquiries and audit search in the backend — 2026-10-08

Details fixed while implementing `inquiries.*` and `audit.list` (IR118); DD-C12, DD-A08, DD-A16 and IR95 apply.

1. **inquiries.create.** Client of the customer; subjectType payment requires invoiceId and forbids restrictionId; restriction requires restrictionId and allows an invoiceId that is one of its cause invoices (VALIDATION otherwise); message trimmed 1–2000. Invoices and restrictions of other customers or unknown IDs return NOT_FOUND. The inquiry is saved as received and HQ billing.write holders receive one inquiry notification each (IR95 inquiry.received).
2. **inquiries.list.** Clients see their own customer's inquiries; HQ billing.read sees all. Filters state, subjectType, invoiceId, restrictionId, customerId; sort createdAt (default newest first), updatedAt or id.
3. **inquiries.answer.** billing.write, latest version, state received (answered → CONFLICT), reply trimmed 1–2000; sets answered with the reply, answeredBy and answeredAt, and notifies the customer's active clients (IR95 inquiry.answered). The acting membership never receives its own notification.
4. **audit.list.** audit.read; filters.from and filters.to are required with from < to and at most 366 days (VALIDATION); optional actorId, targetKind, targetId, correlationId, result (success/denied/failed/pending) and action filter by equality. Unknown or other-tenant correlation IDs return an empty page. Sort occurredAt, default newest first. maskedBefore/After are returned as stored (masked at write time).

## IR144 Client users and two-step verification in the backend — 2026-10-08

Details fixed while implementing `clientUsers.*` and `twoFactor.*` (IR118); IR111, IR112 and IR114 apply.

1. **Client user records.** `identity.client_users` lists a customer's users. The seed creates one active owner row per seed client membership; `members.save` creating a client membership adds an active member row (unless the email is already listed). `Target.kind` gains `client_user` and `Notification.templateKey` gains `invite` for the invite preview.
2. **clientUsers.list.** Client owners (members FORBIDDEN) see their own customer; HQ asset.read sees all. Filters customerId, status, clientRole; sort email (default), invitedAt or id. lastSignInAt comes from the linked user and is recorded at sign-in (IR268).
3. **clientUsers.save.** Email must be a plain valid address up to 254 characters and unique per customer, case-insensitive (VALIDATION). Without id it invites: status=invited, invitedAt=now, invitedBy=caller; status is not allowed; a client owner may invite only members without status or reason (FORBIDDEN otherwise) and only into their own customer (NOT_FOUND). With id it is HQ asset.write only, latest version, reason 1–1000 required, customerId immutable; an invite cannot be set active before first sign-in (CONFLICT); demoting or disabling the last active owner is CONFLICT. Changes are mirrored on the linked membership (clientRole; disabled ends validUntil=now, active clears it; scopeVersion+1).
4. **clientUsers.remove.** HQ asset.write, latest version, reason 1–1000; the last active owner is CONFLICT. The row is deleted, the linked membership ends at now, and DeletedResource {id, deleted:true} is returned.
5. **clientUsers.resendInvite.** Read: client owner of the customer or HQ asset.write; only status=invited (CONFLICT otherwise); returns an email NotificationPreview (templateKey/type invite, target client_user, deliveryState preview) and saves or sends nothing.
6. **twoFactor.** Per user in `identity.two_factor`. get returns enabled=false with a stable demo setupKey, or enabled=true with enabledAt and recoveryCodesLeft. enable accepts any 6 digits (VALIDATION otherwise), CONFLICT when already enabled, and returns 8 recovery codes once (stored hashed). disable accepts any 6 digits and is CONFLICT when not enabled. Preferences.twoFactorEnabled reflects the record.

## IR145 QR resolution, write results and password-reset preview in the backend — 2026-10-08

Details fixed while implementing `units.resolveQr`, `writes.getResult` and `auth.previewPasswordReset` (IR118); D04, DD-T13 and IR111 apply.

1. **units.resolveQr.** The code (trimmed, 1–200) is a unit ID, `ac-unit:<unit ID>` (prefix case-insensitive) or the serial of the bound device (case-insensitive). The unit must have an active Assignment of the technician (NOT_FOUND otherwise, also for unknown codes). jobId is the technician's open job on the unit (assigned, in_progress, on_hold or rework_requested) whose scheduled window ends first after now, or null.
2. **writes.getResult.** operation must be a catalog operation and idempotencyKey 8–128 characters (VALIDATION). Only the caller membership's own key for the same operation is visible: absent, expired or another operation → not_received; in progress → pending; completed → succeeded with the stored result and resourceIds = [result.id] when present. Rejected writes leave no record (zero side effects), so they read as not_received and the same intent may be retried with the same key.
3. **auth.previewPasswordReset.** Public; demoEmail must contain “@” (up to 254). Every address returns the same `{messageKey:'auth.reset_generic', deliveryState:'preview'}` so accounts cannot be enumerated; nothing is sent or stored.

## IR146 Role summaries in the backend — 2026-10-08

Details fixed while implementing `summaries.get` (IR118); IR26, IR44, IR49 and IR51 apply. admin.summary is a separate step.

1. **Kind and caller.** kind must match the caller role: client → customer, contractor → partner, technician → technician (FORBIDDEN otherwise). asOf = now.
2. **customer.** Equipment metrics over the caller's not archived units, filtered by customerId, propertyId, unitId and unitIds (an empty array gives zeros); from/to are accepted (from < to) and do not change current counts; job-only filters are VALIDATION. total = online + offline + unknown, where every connection other than online/offline counts as unknown; power uses the effective power state (on/off/unknown, SR27); alertCount sums unresolved (open/acknowledged) critical/warning alerts. Job counts are 0.
3. **partner / technician.** The jobs.list filters, validation and projections (IR23/IR26/IR49) are reused, so the summary and the list always agree. offerCount counts open Offer projections (contractors only); among current rows (not completed/cancelled) scheduledCount counts accepted+assigned, inProgressCount in_progress, activeCount accepted+assigned+in_progress, reviewCount submitted (D07), overdueCount dueAt < now, assignedCount rows with an active Assignment. Technicians also get total/online/offline/unknown over the distinct units of current rows; other equipment metrics and alertCount are 0 (IR44).
4. **Unresolved alert count.** UnitSummary.activeAlertCount uses the same IR51 population (open/acknowledged, critical/warning); normal alerts are not counted.

## IR147 Emission factors and energy baselines in the backend — 2026-10-08

Details fixed while implementing `factors.*` and `baselines.*` (IR118); D07, IR08, IR11, IR90 and SR29 apply. The seed loads demoSeed.factors and demoSeed.baselines.

1. **factors.save.** region trimmed 1–120, integer year 2000–2100, kgCO2ePerKWh > 0 and ≤ 10 (unit fixed to kgCO₂e/kWh), source trimmed 1–500, isDemo must be true (VALIDATION). One current factor per region and year (VALIDATION `errors.factor_exists`). With id and the latest version a new version is stored and becomes current; earlier versions stay readable for the reports that reference them. factors.list (mrv.read) returns current versions, filters region and year, sort year desc, region, id.
2. **Energy integration (shared by baselines and energy.summary).** For each unit and UTC minute in [from, to) only the power sample exactly at the slot start is considered (highest sequence, then lowest ID); it counts when origin=measured, quality=valid, unit kW and boundaryId=ac_input_electricity, contributing value/60 kWh. Otherwise the slot is invalid with warnings non_measured_input / boundary_mismatch; any invalid slot adds partial_coverage. Ranges must be minute-aligned, from < to, at most 366 days.
3. **baselines.save.** unitIds 1–100 unique, existing and not archived (VALIDATION); boundary 1–500, assumptions 1–2000, source 1–500. demo_fixed requires a finite non-negative baselineKWh and stores quality kind=modeled. demo_period_comparison forbids baselineKWh and whole_building_electricity, integrates the period at save time and stores kind=measured with expectedSlots, validSlots, coverage and sourceSnapshot {generation 1, eventCursor = number of outbox events, snapshotAt = now}; zero valid slots gives baselineKWh=null. With id and the latest version a new version becomes current; saved versions never recalculate.
4. **baselines.list.** HQ energy.read or mrv.read sees all current baselines; clients see those whose units all belong to their organization. Filters unitId (contained), method, boundaryId; sort createdAt (default newest first), periodFrom or id.

## IR148 Energy summary and admin dashboard in the backend — 2026-10-08

Details fixed while implementing `energy.summary` and `admin.summary` (IR118); D07, SR29, IR40, IR51, IR68, IR78, IR115 and IR147 apply.

1. **energy.summary.** from/to form a D07 energy range; unitIds is required (may be empty, at most 100, unique) and every unit must exist, not be archived and, for clients, belong to their organization (NOT_FOUND). tariffVersion, when given, must be `tariff-demo-1` (0.5 MYR/kWh). Totals: kWh from the IR147 integration (null with no valid slot), amountMinor = kWh × tariff rounded half away from zero to minor units, emissionsKg = kWh × the current version of fixture.defaultEmissionFactorId (SR09; factorRef/factorSnapshot, or factor_missing with null emissions). coverage = valid/expected slots, null with no units. With baselineId (readable current version, else NOT_FOUND) baselineRef/snapshot are returned; the comparison is made only when the baseline has a value, the same unit set, boundary ac_input_electricity, the same minute count, actual coverage 1 and either a modeled value or measured coverage 1: savedKWh = baseline − actual, deltaKWh = −savedKWh, savingPercentage = savedKWh ÷ baseline × 100 (null for 0), savedAmountMinor and savedEmissionsKg with the same sign (IR68). Otherwise savings stay null with baseline_not_comparable. Modeled baselines add modeled_baseline; warnings are sorted and unique.
2. **admin.summary.** dashboard.read; from/to form a D07 energy range. U = not archived units matching customerId/propertyId. customerCount counts customers whose Customer and Organization are active (IR40), narrowed by the filters. Unit counts use the IR146 classification; operatingRate = powerOn ÷ (powerOn + powerOff) × 100, null when no unit has a known power state. alertCount follows IR51. jobCounts has all ten statuses for jobs of U whose requested slot starts in the period. With billing.read: billingVisibility=allowed, overdueInvoiceCount and amountsByCurrency (sorted by currency) over unpaid invoices past due on contracts covering U; without it forbidden with nulls (IR115). energySummary returns actuals only (no baseline, savings null). energyForecast follows IR78 using the latest-created current demo_fixed baseline on ac_input_electricity whose unit set equals U.
3. **Partner and technician KPIs.** IR146 item 3 is aligned with D07: scheduledCount = accepted+assigned, activeCount = accepted+assigned+in_progress.

## IR149 MRV reports in the backend — 2026-10-08

Details fixed while implementing `mrv.*` (IR118); SR09, SR29, IR11, IR88 and IR148 apply.

1. **Conditions.** from/to form a D07 energy range; unitIds 1–100 unique; baselineId+baselineVersion and factorId+factorVersion reference stored versions (NOT_FOUND otherwise); boundaryId is one of the two IDs and boundary is 1–500 characters; organizationId must be a customer organization and every unit a not archived unit of it (VALIDATION).
2. **mrv.preview.** Computes the IR148 summary with exactly the referenced baseline and factor versions. incomplete is true when the conditions boundary is not ac_input_electricity or differs from the baseline, when there is no expected slot or coverage < 1, or when the baseline is not comparable. scope=scope_2, isDemo=true; nothing is stored.
3. **mrv.saveDraft.** evidenceIds 0–20 unique existing attachments (VALIDATION). Without id it stores version 1; with id the expected version must be the latest and the result is a new version. Each version stores the conditions and the result computed at save time with status draft; old versions stay readable and unchanged.
4. **mrv.recordReview.** reportVersion and the expected version must both be the latest (CONFLICT); incomplete versions and already demo_reviewed versions are CONFLICT; reviewComment trimmed 1–1000. It records the review on the reviewed version and stores a new demo_reviewed version with the same conditions and results. reviewHistory of a version lists the reviews of earlier versions.
5. **Reads.** mrv.get returns the latest version or the requested reportVersion; mrv.versions pages all versions by version ascending (query accepts limit/cursor only); mrv.list lists latest versions, filters organizationId, unitId, status (draft/demo_reviewed) and from/to (period overlap), sort updatedAt (default newest first), periodFrom or id.

## IR150 Offset quotes and simulated records in the backend — 2026-10-08

Details fixed while implementing `offsets.*` (IR118); SR18, SR22 and REV19-030 apply.

1. **offsets.preview.** purpose trimmed 1–1000; amountKg > 0, at most 100000 with up to 3 decimals; period from < to, at most 366 days; unitIds 1–100 unique. Clients quote for their own customer (another customerId or unit is NOT_FOUND); HQ offset.write must give customerId (VALIDATION) and units of that customer (VALIDATION). The quote is saved with version 1, expiresAt = now+15 minutes, provider unselected, estimatedAmountMinor/currency null, scheme demo and the fixed future-concept marketConcept.
2. **offsets.simulate request.** demoConfirmed must be true for every branch, and only the branch's fields are accepted (VALIDATION). quoteVersion must match (CONFLICT); an expired quote or one already requested is CONFLICT. The record starts demo_requested with one pending purchase attempt.
3. **Events and retry.** Clients may only request and retry (FORBIDDEN otherwise); other customers' records are NOT_FOUND. purchase_confirm/retire/fail need the latest record version and the current pending attempt of the right stage (CONFLICT otherwise, including late success while failed). An eventId already used returns the stored record when attempt and event match and CONFLICT otherwise. purchase_confirm creates purchaseRef once and a pending retirement attempt (demo_purchased); retire creates retirementRef and demoCertificateRef (demo_retired, currentAttemptId null); fail keeps the attempt current (failed, previousState = the state before). retry (latest version, failed current attempt) starts a new attempt of the failed stage and returns to demo_requested or demo_purchased. Attempt IDs are UUIDv7 so same-instant attempts keep creation order; eventHistory is the audit history ordered by time and correlation ID.
4. **offsets.list.** HQ offset.read sees all; clients see their own customer's records whose quote units are all theirs. Filters customerId, status (the record state, IR200) and from / to on createdAt; sort createdAt (default newest first), updatedAt or id.
5. **Schema.** `energy.offset_attempts` gains completed_event_id (unique) and completed_event for the eventId replay rule.

## IR151 Customer automation rules in the backend — 2026-10-08

Details fixed while implementing `automations.save`, `automations.list` and `automations.nextRuns` (IR118); DD-C04, DD-C05, D09, IR27 and SR02 apply. simulate/fire are a separate step.

1. **Common fields.** name trimmed 1–120 (schema CHECK aligned from 80 to 120); unitIds 1–50 unique, own, not archived units (another customer's unit NOT_FOUND, archived CONFLICT) whose capabilities support every action (VALIDATION); timezone UTC or an Area/City IANA zone; priority 0–100; enabled as given (new forms default false in the UI). kind is fixed after creation. Owner and creator are the caller.
2. **schedule.** weekdays ISO 1–7, at least one, no duplicates; startLocal/endLocal HH:mm; endsNextDay required. start = end is VALIDATION; end < start requires endsNextDay; endsNextDay with end > start is VALIDATION (over 24 hours). startAction and endAction are both required UnitActions. Every start and end occurrence in the next 367 days is checked in the rule timezone; a nonexistent or ambiguous local time is VALIDATION (`errors.local_time_invalid`). Condition fields are not allowed.
3. **event.** condition is one of occupancy {occupied}, location {event: arrival|departure}, pattern {localTime HH:mm}, weather {metric: temperature, operator gt|gte|lt|lte, value −50–100}, with exactly its fields; tariff/peak/solar/battery belong to HQ automation policies. Location conditions need the owner's granted location_automation consent (VALIDATION `errors.consent_required`). action is required; schedule fields are not allowed.
4. **Updates.** Latest version required; editing while disabled keeps disabledReason; saving with enabled=true rechecks the targets and clears disabledReason (IR27).
5. **automations.list / nextRuns.** Clients with control.execute see their organization's rules; filters kind, enabled, unitId; sort createdAt (default newest first), name, priority, id. nextRuns requires count = 8 and a schedule rule (VALIDATION otherwise) and returns the next 8 start/end occurrences after now in time order, skipping invalid local times.

## IR152 Automation evaluation (simulate / fire) in the backend — 2026-10-08

Details fixed while implementing `automations.simulate` and `automations.fire` (IR118); D02, IR16, IR21, IR27, IR52, IR54 and SR21/SR25 apply. Notification evaluation (the `notifications` array) follows IR155.

1. **Input.** eventId, occurredAt, phase (schedule_start/schedule_end/condition), unitIds 1–100 unique and facts. Each fact uses the IR16 metric/unit table (normal metrics with their D07 unit; weather_temperature °C; tariff MYR_per_kWh; solar/battery kW; occupied/peak boolean; location event with arrival/departure), belongs to a listed unit, appears once per unit/metric and has a value of the right type (VALIDATION). occurredAt must be in the current demo-clock minute (VALIDATION); for fire a stored replay is checked first. Every target unit must exist and, for clients, be theirs (NOT_FOUND).
2. **Candidates.** condition phase: HQ automation policies attached to the unit and the customer's event rules; schedule phases: customer schedule rules with an occurrence of that phase at the tick. A condition matches from merged facts: missing/null/future/bad-quality facts give missing_data, stale quality or now−observedAt > TTL (location only within the same minute) gives stale; pattern matches the rule-timezone local time of occurredAt (IR52).
3. **Arbitration.** HQ policies before customer rules, priority descending, ID ascending; the first eligible candidate wins. A candidate is excluded as disabled, the condition reason, owner_forbidden (owner not active or without control.execute in the unit's organization / automation.policy.write), consent_revoked (location without consent), invalid_capability, restricted (IR46), busy (D04 mutual exclusion) or offline (IR47). Without a winner the unit is suppressed with the first exclusion reason in candidate-ID order, or no_match without candidates. Results are sorted by unitId.
4. **fire.** The winner gets one Command per unit (source automation, actor = rule owner, sent with the 30-second window) recorded in `control.automation_runs`; decision requested with commandId. The result is stored in `control.evaluation_events` per tenant/eventId/phase with an input hash: the same eventId and input returns the stored result; the same eventId with other input, or another input or phase for an already evaluated tick, is CONFLICT; the same input with a new eventId reuses the tick result without new Commands. simulate stores nothing.

## IR153 Voice intents and the monthly energy export in the backend — 2026-10-08

Details fixed while implementing `voice.resolveIntent` and `energy.exportReport` (IR118); D09, IR65, IR110 and DD-C16 apply.

1. **voice.resolveIntent.** text trimmed 1–200 code points, locale en|ms. The IR65 grammar of the requested locale is matched ASCII case-insensitively (help/bantuan → help with messageKey voice.help; no match → unsupported). The room matches not archived Space names (trimmed, case-insensitive, any kind); candidates are the caller-readable not archived units placed directly in the matched spaces, labelled property > ancestor spaces > space > unit and sorted by label. None → unsupported; several → candidates; one, or a selectedUnitId among this request's candidates (otherwise NOT_FOUND) → temperature with the latest temperature measurement (null when none) or change with celsius, the unit's observedState as before and the unit version as expectedVersion.
2. **energy.exportReport.** Client read. month YYYY-MM must have ended in the caller's preferences timezone (VALIDATION); propertyIds 1–100 unique, all not archived properties of the caller's organization (NOT_FOUND); sections 1–4 unique values; format pdf|csv. The demo file covers the not archived units of the properties: energy_cost (kWh and cost), month_comparison (previous and current month kWh), co2_offsets (emissions and demo-retired kg in the month), alerts_maintenance (alerts detected and jobs completed in the month). Only ReportFile metadata is returned (fileName energy-report-<month>.<format>, mime, size of the rendered content, generatedAt, isDemo).

## IR154 Demo operations and demo sessions in the Core API — 2026-10-08

Details fixed while registering `demo.*` and `demoSession.*` in the Core API (IR118, backend architecture §15: production never includes demo operations).

1. **demoSession.\*.** Sign-in, membership switching, extension and sign-out are handled by the BFF session (OIDC sign-in with the demo identities); the Core API answers them with UNAVAILABLE `errors.session_via_bff` and stores nothing. The browser-only Phase 1A mock keeps implementing them as before.
2. **demo.\* switch.** The Core API serves demo operations only when started with `DEMO_OPS=1` (demo environment); otherwise they return UNAVAILABLE `errors.demo_only` with zero side effects.
3. **demo.advanceClock.** `to` must not be before now (VALIDATION `errors.clock_backwards`; initial-time setting after reset is a mock-only feature). The scenario clock starts at fixture.clock when the API starts (`DEMO_CLOCK_START` overrides it) and then runs in real time; demo.advanceClock moves it forward by the difference (all requests and the session checks use it) and one worker tick processes the deadlines reached (command expiry, diagnostic runs, offer/proposal expiry, auto-confirmation, history freezing). The generation stays 1.
4. **demo.reset.** Requires confirmation=true; the Core API does not drop shared data while running and returns UNAVAILABLE `errors.demo_reset_offline` (rebuild with `make resetdb`).
5. **demo.trigger.** Supported eventTypes: restriction_observation (IR164), command_sent (requested → sent), command_ack (control acknowledgement, D04 timing and restriction results), command_fail (failed with DEVICE_REJECTED and restriction release results), telemetry, and device faults / recoveries and device operation results (IR217). Telemetry needs a sensor bound to the unit with the same metric (VALIDATION), applies the D07 unit/range and IR12 time rules (unit mismatch, out-of-range values with value=null, or observedAt > receivedAt / receivedAt > now give suspect; null is missing), takes the boundary from the sensor (IR11), uses sequence latest+1 when omitted (IR77) and ignores a repeated measurement eventId. Unknown commands or sensors are NOT_FOUND; other eventTypes are VALIDATION `errors.trigger_unsupported` in the Core API. The result is DemoEvent {eventId, generation 1, occurredAt, type}.

## IR155 Alert-policy notification evaluation in simulate / fire — 2026-10-08

Details fixed while adding notification results to `automations.simulate` and `automations.fire` (IR152); SR12, SR21, SR25, SR28 and IR108 apply.

1. **Candidates.** For each target unit: every alert policy attached to it plus the tenant default policy restricted to the rules not switched off for the unit's customer. Results are one per policy, in policy-ID order, independent of the control decision. A unit whose control result would be no_match but whose notification policies match (selected/created/failed) reports no_control_action.
2. **Matching.** A policy matches when one of its conditions holds for the unit's fact of that metric at the tick and occurredAt is inside its activeWindow (rule timezone; null means always). The demo evaluation takes a fact as sustained for durationSeconds. No fact gives not_due; a fact that is not valid, null, in the future or older than 120 s gives quality. Disabled policies give disabled; an owner whose membership is no longer active gives owner_forbidden; a notification created for the same policy and unit within cooldownMinutes gives cooldown.
3. **Recipients.** Alert policies notify their recipientMembershipIds that are still active and may read the unit's customer (clients of that organization, HQ with alert.read) on each configured channel; the default policy notifies the customer's active clients in-app. None gives failed/no_recipient.
4. **fire.** A matching policy reuses its open source Alert (one per unit/policy/rule) or opens a sensor Alert with the condition severity, then creates one Notification per recipient and channel (templateKey alert, type fault for the sensor source Alert per IR104, target unit). Zero recipients append one DeliveryFailure {id, policyId, eventId, occurredAt, reason no_recipient, deliveryState failed} to the Alert (deduplicated per event and policy) and do not advance the cooldown. NotificationOutcome always carries notificationIds and failureIds; simulate returns NotificationDecision and stores nothing.

## IR156 Business-event notifications in the backend — 2026-10-08

Details fixed while implementing the IR95 table (IR118); IR04, IR05, IR104 and IR143 keep their own notifications (reminders, restriction advance notices, inquiries).

1. **Where.** Notifications are created in the same transaction as the triggering write, from its committed transition, before the audit and outbox rows. Each recipient membership receives one notification; the acting membership and memberships that are not currently active receive none. Retries replayed by the idempotency store create nothing.
2. **Jobs.** jobs.create and jobs.reportProblem → job.requested; jobs.offer → job.offered (the contractor of the latest offer); jobs.accept / jobs.decline → job.accepted / job.declined; jobs.assign → job.assigned (schedule_change); jobs.submit → report.submitted; jobs.review → report.returned (report_return) when rework is requested and job.completed (completion) when accepted; jobs.cancel / jobs.hold / jobs.resumeHold → job_update with the resulting status. Recipients follow the IR95 table: customer clients of the job's organization, HQ job.write holders, the assigned technician (including the assignment revoked by the same cancel/hold), and for outsourced jobs partner.accept / partner.assign / partner.review holders of the contractor. Target job, severity normal, params.status the event.
3. **Restrictions.** Every audited restriction transition except schedule (execute, cancel, defer/exempt, release, override, reconcile, retry and the payment release) notifies the customer's active clients and HQ restriction.write holders with the resulting state; severity warning for scheduled/requested/applied/release_requested and normal for released/cancelled (IR104).
4. **Payments.** A payment that becomes confirmed notifies the invoice customer's clients and HQ billing.write holders (template payment, target invoice, severity normal).
5. **Alert template type.** Policy notifications created by automations.fire use type fault for the sensor source Alert (IR104), not quality.
6. **Clock.** Revoking assignments on jobs.cancel stamps updated_at with the demo clock.

## IR157 One business clock in the database — 2026-10-08

Every API transaction (reads and writes) and every worker tenant transaction sets `app.now` to the request clock (the demo scenario clock in the demo environment) before running. `platform.app_now()` returns it, falling back to the database time when it is not set (seed, migrations, the idempotency store). All column defaults and SQL timestamps that used `now()` use `platform.app_now()`, so created_at/updated_at and other database-side times agree with the times the operations compute (D04, IR36). The idempotency store keeps the real time for its 24-hour expiry.

## IR158 Web BFF and the first API-backed screen — 2026-10-08

Details fixed while connecting `web/` to the Core API (backend architecture §5 and §15).

1. **Sign-in.** With `DATA_SOURCE=api` the login screen sends the chosen demo identity to `/bff/auth/login` (OIDC authorization code with PKCE, `login_hint` = demo user). `/bff/auth/callback` exchanges the code at the internal issuer, reads the `tenant_id`, `membership_id` and `role` claims (Keycloak user attributes mapped into the tokens; realm users carry the seed user IDs so the token subject is the seed user) and stores an httpOnly, HMAC-signed session cookie (30 minutes); missing claims return to `/login?error=membership`. `/bff/auth/logout` drops the session. Browsers never receive tokens.
2. **Operation relay.** `POST /bff/ops/<operation>` forwards the body, Idempotency-Key and X-Expected-Version with the session's Bearer token, X-Tenant-Id and X-Membership-Id to `CORE_API_URL/v1/ops/<operation>`; status codes and DomainError bodies pass through unchanged; an unreachable API is UNAVAILABLE; with `DATA_SOURCE=mock` the relay answers UNAVAILABLE `errors.api_data_source_disabled`. `GET /bff/session` reports the data source and the signed-in role without tokens.
3. **Screens.** `lib/ops.ts` (`callOp`, OpError) and `lib/useOp.ts` (data-source switch) let a screen read the Core API in api mode and keep its fixture data in the demo. Screens wired so far: customer overview (`units.list`, `summaries.get(kind=customer)`), customer alerts (`alerts.list`; unresolved critical/warning alerts under “Needs attention”, everything else under information) the admin overview KPIs (`admin.summary` for the current Asia/Kuala_Lumpur day of the server clock; billing shows “Not permitted” when billingVisibility is forbidden) and every screen reading the shared job store (`jobs.list` caller projection for admin, partner, technician and customer job screens; job actions cancel, partner accept/decline, technician acknowledgement and notes call jobs.cancel / jobs.accept / jobs.decline / jobs.acknowledgeAssignment / jobs.addNote with the version and offer IDs read from jobs.get, then invalidate every mounted read; the remaining job actions still use the demo store). In mock mode `useOp` always returns the live fixture value. Other screens follow screen by screen.

## IR159 Full-stack check of the BFF path — 2026-10-08

Verified locally with Keycloak (realm import), the Core API (`DEMO_OPS=1`) and the web app (`DATA_SOURCE=api`): authorization-code sign-in with PKCE for a client, a technician and a contractor; `summaries.get`, `units.list` and a write (`preferences.update`) through `/bff/ops`; role checks (a contractor calling `admin.summary` is FORBIDDEN). HQ demo identities must set up TOTP on first sign-in (Keycloak required action, matching the HQ MFA requirement), so automated HQ checks use the API test suite. In the demo environment the scenario clock must start at fixture.clock, otherwise fixture memberships whose validUntil has passed in real time are rejected.

## IR160 Result meta and the server clock for screens — 2026-10-08

Every Core API result carries `meta = {correlationId, snapshotAt, eventCursor}` (service-contracts Meta) plus `operation`: snapshotAt is the transaction's business time (the demo scenario clock in the demo environment) and eventCursor the tenant's outbox position of that snapshot (D07). `GET /bff/session` returns `serverNow` from `session.get` meta.snapshotAt, and screens derive date ranges (today, 7d, 30d) from it (`useNow`), so they agree with the demo clock.

## IR161 Seeding the demo business records — 2026-10-08

The Core API seed loads the demoSeed business collections with deterministic IDs (seed namespace UUIDv5 of the fixture ID), so API-mode screens show the fixture scenario: contracts with their unit sets, invoices (numbered by the invoices.create rule, IR199), restrictions with cause invoices and per-unit state (an applied unit also gets its ACUnit.observedRestriction), commands, alerts, notifications (targets mapped to database IDs), jobs (requested/preferred/scheduled slots, alert links), offers and assignments. Inserts are idempotent (ON CONFLICT DO NOTHING); fixture IDs inside JSON values (for example RestrictionAction.restrictionId) are converted to database IDs. Tests that need a clean window isolate themselves from these records and restore them afterwards.

## IR162 BFF action check and the jobs.addNote result — 2026-10-08

With the seeded scenario the BFF path was exercised end to end: the technician lists the seeded assigned job and starts it with the version read from jobs.get; the contractor sees it in progress; the customer reads the seeded alert and restriction notifications, adds a note and cancels the requested job, and a write with a stale version returns CONFLICT. A technician adding a note is FORBIDDEN (jobs.addNote allows clients, delegated contractors and HQ job.write only), and the technician screens do not offer it. `jobs.addNote` returns the created JobNote (service contract) instead of the job; the job version still increases.

## IR163 Result-type conformance sweep — 2026-10-08

Every registered handler's result type was compared with the `result` of its service contract, and the JSON fields of 40 canonical DTOs with their Go structs. Differences were naming only (Go type names such as FilterStatus for FilterCareStatus, unions returned as `any`, intersections such as Money & …) except `jobs.acknowledgeAssignment`, which now returns the Assignment (it returned the job). Together with IR162 (jobs.addNote → JobNote) all write results match their contracts; the demoSession handlers intentionally answer UNAVAILABLE (IR154).

## IR164 Terminal-restriction recovery cases in the backend — 2026-10-08

Details fixed while implementing SR26 (IR118).

1. **Observation.** `demo.trigger(restriction_observation)` {restrictionId, unitId, observed} sets the unit's observedRestriction and lastSeenAt. If observed names a released or cancelled restriction, its state stays terminal and a pending recovery case {id, unitId, observedRestrictionId, observedRulesVersion, successorRestrictionId (the unit's newest active restriction), state, observationEventId, commandIds, createdAt, resolvedAt} is added, or the open case for the same unit / restriction / rules version takes the new observation. One open reconciliation_required Alert is kept per unit. Case changes increment the restriction and the target units' versions.
2. **Blocking.** A unit with an unresolved case cannot get a new restriction (schedule CONFLICT, IR140).
3. **reconcile.** For a released/cancelled restriction, restriction.write or an override-only caller (IR03) reconciles units with an unresolved case (otherwise VALIDATION; a removing case CONFLICT). The observation must be at most 30 s old (TIMEOUT) and the device online (OFFLINE). Observation of the old restriction and rules version creates a remove_restriction Command (case removing); no observation or the successor resolves the case without Commands; any other restriction is CONFLICT.
4. **Results.** A timely acknowledgement of the remove resolves the case, clears the unit's observedRestriction and, when no unresolved case remains for the unit, resolves the reconciliation_required Alert; a failed or expired remove returns the case to pending. Terminal per-unit states never change.

## IR165 UnitDetail parts in the backend — 2026-10-08

`units.get` returns UnitDetail: the unit summary plus capabilities (the unit's capability version), effectiveControlPolicy ({state:'unrestricted'} or {state:'restricted', phase, policy, reasonKey:'control.restriction_active'} from the newest requested/applied/release_requested restriction, IR46), controlAvailability ({state:'blocked', reasonKey:'control.reconciliation_required'} while any recovery case of the unit is unresolved, else available; SR26), components (the inspection components of the serviceScope groups in DD-T04–T06 order), pendingCommands / pendingCommandIds (requested or sent Commands, oldest first) and location {pathLabels: property > ancestor spaces > space, address, accessInstructions}. While a unit is blocked, commands.create, diagnostic runs and automation control return CONFLICT `errors.reconciliation_required` (automation reason reconciliation_required); restriction Commands are unaffected.

## IR166 Service directory layout — 2026-10-08

User decision (microservices): every deployable service lives under `service/` at the repository root, one directory per service, each with its own build (Dockerfile) and dependencies. Go code is a `service/go.work` workspace of three modules: `service/core` (`github.com/pradita/ac-project/service/core`: domain modules, ops registry, platform, seed, scheduler; tools cmd/gen and cmd/seed), `service/api` (Core API: cmd/api and the HTTP assembly internal/app) and `service/worker` (cmd/worker background roles, independent service per user decision). Each has its own Dockerfile built with context `service/`. `service/web` is the Next.js web app and BFF (formerly `web/`). Further services (webhook receiver, migrate, IoT bridge, device simulator) are added as new `service/<name>` directories. compose.yaml builds from these directories; documents name the new paths, while earlier changelog entries keep the paths of their time.

## IR167 Migration service — 2026-10-08

`service/migrate` is the fourth Go module in `service/go.work` and its own image `ac-migrate`. It embeds `migrations/NNNNNN_name.up.sql` (golang-migrate naming; `000001_init.up.sql` is [db/schema.sql](db/schema.sql) verbatim, enforced by a test until the first release) and applies newer versions in order under a PostgreSQL advisory lock, each in one transaction with the version row, or outside a transaction with a dirty flag when the file starts with `-- migrate:no-transaction`. Bookkeeping is the golang-migrate compatible `public.schema_migrations(version, dirty)`. Commands: `up`, `version`, `force N`. `up` refuses a database that has the `platform` schema but no recorded version and stops on a dirty version. Local only: `APP_LOGIN_PASSWORD` creates or updates `ac_app_login IN ROLE ac_app`, and `SEED_FIXTURE` loads the demo fixture when `identity.users` is empty. Compose `migrate` (profiles schema, backend, full) replaces the former `db-schema` psql job; `service/api/tools/resetdb.sh` uses the same runner. Checked: `docker compose up migrate` on the seeded database applied 0 migrations, and `api` then started healthy behind it.

## IR168 Shared demo clock, acknowledged settings, BFF token refresh — 2026-10-08

1. Demo scenario clock shared by every process: `platform.demo_clock` holds one row with `offset_ms` (scenario time minus wall-clock time). With `DEMO_OPS=1` the Core API and the workers (compose `x-worker` now sets `DEMO_OPS`) open it through `service/core/platform/democlock`: the first process inserts the offset that starts the scenario at `DEMO_CLOCK_START` (default fixture.clock 2026-09-14T01:00:00Z), later processes keep the stored offset, `demo.advanceClock` adds to it, and each process re-reads it at most once per second. A restarted API therefore continues the scenario instead of jumping back to fixture.clock, and the scheduler expires Commands, Offers and Proposals and freezes histories on the scenario clock rather than on wall-clock time (before this change a worker using wall-clock 2026-10-08 froze fixture histories that were still open at the scenario time). Tests that set a fixed base clock without a scenario start keep a process-local offset. Production never sets `DEMO_OPS` and never creates the row.
2. A timely Command acknowledgement copies the acknowledged unit setting (`set_power` → power, `set_temperature` → celsius, `set_mode` → mode, `set_fan` → fanLevel) into `ACUnit.observedState` with `observedAt` = acknowledgement time, as IR45/IR50 require ("only Command ack changes them"). observedState is an observation field: the unit version and `updatedAt` do not change, so a fetched `expectedUnitVersion` stays valid. Late acknowledgements change nothing.
3. BFF session (backend architecture §5): the signed cookie outlives the access token; 30 seconds before the access token expires the BFF uses the refresh token, keeps the session only when the refreshed token carries the same tenant and membership, and rewrites the cookie (30-minute sliding lifetime). A failed refresh clears the cookie (UNAUTHENTICATED). Cookies with any shape other than `body.mac` are rejected.
4. Web screens on the API (DATA_SOURCE=api): `/notifications` (notifications.list newest first, markRead with the row version, role-specific links) and `/customer/units/[id]` (units.get header, observed setting, restriction and reconciliation banners, capability-bounded temperature, commands.create per changed setting with commands.get polling until acknowledged / failed / expired, pending commands in the history, room siblings by spaceId, alert policies attach / detach with units.setAlertPolicies).

## IR169 Unit read scope for external technicians and contractors — 2026-10-08

Found in the technician screen wiring: `units.list` / `units.get` and every read built on the same unit scope (counts, telemetry, ventilation, devices, alerts through the scoped unit set) gave external technicians all units of their Membership.scopes and contractors none (the contractor's own organization was compared with the unit's customer organization). The unit scope now follows the IR152 table and IR49:

1. Internal technicians: units within Membership.scopes (unchanged).
2. External technicians: units within Membership.scopes that have the caller's own `active` Assignment with `createdAt ≤ now < scheduledEnd` (IR49 viewing window) on a job whose Offer is `accept` with `accessValidFrom ≤ now < accessValidUntil` (IR124). Without such an Assignment the unit is hidden (lists omit it, single reads return NOT_FOUND).
3. External technicians' `units.get` without jobId inside the viewing window but before the work window `[scheduledStart, scheduledEnd)` returns FORBIDDEN `errors.assignment_not_started` (IR49(b)); the technician unit screen shows "Not started yet". Other equipment reads (telemetry.*, alerts.*, devices.*) filter by the viewing window; their FORBIDDEN-before-start distinction is still open.
4. Contractors (`contractor:accepted-valid-offer`): units of jobs with the company's own accepted Offer inside its access window.

Tests: `TestUnitScopeExternalTechnicianAndContractor` covers no Assignment, Assignment without an accepted Offer, viewing window before the work window (403), inside the work window (200), revoked Assignment, other contractor, and an ended access window; the scope tests that previously used an external technician's Membership.scopes now use the internal technician. Test acknowledgements restore the unit's observedState afterwards, because IR168 copies acknowledged settings with a later observedAt. `democlock` keeps the last known offset when the row disappears (database rebuilt while running). Web: `/technician/units/[id]` reads units.get, open alerts (alerts.list unitId), job history (jobs.list unitId) and telemetry.series (latest 100 temperature measurements, bucketed) in API mode.

## IR170 One unit scope for every module — 2026-10-08

The open point of IR169 item 3 is closed. `service/core/platform/unitscope` is the single D01 unit read scope; assets (units), monitoring (alerts, telemetry, ventilation through the scoped unit set) and devices (list, get, history) use it instead of their own copies, which had the IR169 defect as well (external technicians saw every unit in Membership.scopes; contractors matched their own organization against the customer organization).

- List mode (units.list and pickers): the IR169 rule with the IR49 viewing window for external technicians.
- Equipment mode (alerts.list, devices.list, telemetry list scopes): external technicians only inside the work window `[scheduledStart, scheduledEnd)`.
- Single equipment reads (units.get, alerts.get, devices.get, telemetry.series / latest by unit) resolve in List mode and then apply `unitscope.Gate`: an external technician whose work window has not started gets FORBIDDEN `errors.assignment_not_started` (IR49(b)).
- Devices keep the D05 rule that a technician or contractor sees their own unbound registrations; clients keep the binding-organization rule.

Tests: the IR169 scope test also covers alerts.list (hidden before the work window, listed inside it), alerts.get and telemetry.series (403 before, 200 inside) and the contractor reading the alert of its accepted job. Go coverage 84.0 % of 12 343 statements.

## IR171 Test database isolation — 2026-10-08

Go tests used the development database `ac`. A local worker running on the shared demo clock (IR168) processed test rows mid-test and broke `TestRestrictionLifecycle`, and tests left rows that changed what the local BFF showed. Tests now connect to `ac_test` (`service/api/tools/resetdb.sh` takes `DB=ac_test`; `make testdb` rebuilds it, `make test` creates it when missing and runs with `-count=1` so a cached skip cannot hide a missing database). The seed also derives the job history implied by the fixture (job.created, job.offered, offer decision with decidedBy, job.assigned; deterministic IDs).

## IR172 Membership display name — 2026-10-08

`Membership.displayName` (string) is the member user's `identity.users.display_name`, returned by members.list / members.eligible / members.capacity projections and every Membership result. Contractors need it to list their technicians on the team screen (FR-P team, IR42 projection otherwise unchanged: no permissions or scopes for contractors). Web: `/partner/team` reads members.list, a week of members.capacity and saves members.setUnavailability in API mode.

## IR173 Echo v5 and the official Echo guide — 2026-10-08

User request: build the backend following the official Echo documentation (and the web side following the official Next.js documentation, IR174+). The current Echo documentation is for v5 (v5.4.0), so the Core API moved from `echo/v4` 4.16 to `echo/v5` 5.4.0 (DEC-71):

- Handlers and middleware take `*echo.Context` and return errors; `ops.HTTPErrorHandler(c, err)` is the single place that writes error bodies (Echo guide › Error Handling). `apperr.DomainError` implements `echo.HTTPStatusCoder`; Echo's own errors are recognised through that interface.
- Middleware from `echo/v5/middleware`: Recover, RequestID (UUIDv7), RequestLogger (slog JSON, `HandleError` so the logged status is the final one), ContextTimeout.
- Routes: `/healthz`, `/readyz`, group `/v1` with the authentication middleware, `POST /v1/ops/:operation`.
- `cmd/api`: `slog` JSON logger as `e.Logger`, `echo.StartConfig{GracefulTimeout: 25s}.Start(ctx, e)` (Cookbook › Graceful Shutdown).
- Tests keep `httptest` + `e.ServeHTTP` (Guide › Testing); `TestHTTPErrorHandler` covers DomainError, plain error (no leak), 413, 503, unknown route, wrong method, HEAD and an already committed response. All test files are gofmt-formatted.

Verified: go test (all modules) green; images ac-api 35.6 MB, ac-worker 25.8 MB, ac-migrate 25.7 MB build; the running API answers unknown routes and methods with the DomainError body and correlation ID and logs one JSON line per request.

## IR174 Go module layout per the official Go guide — 2026-10-08

User decision (DEC-71, layout chosen 2026-10-08: single module): the Go code follows "Organizing a Go module" (go.dev/doc/modules/layout), section Server projects — one module `github.com/pradita/ac-project/service` at `service/go.mod` replaces the four-module `go.work` workspace (core, api, worker, migrate); commands live in `service/cmd/{api,worker,migrate,seed,gen}`; every package lives in `service/internal/` (`server` = Echo layer and composition root, formerly `api/internal/app`; `ops`, `modules/*`, `platform/*`, `seed`, `scheduler`, `migrate`, `migrations`), so nothing is importable from outside the module; there is no `pkg/`. Unit tests stay next to the code as the guide says; the HTTP integration tests that need PostgreSQL moved to `service/test/integration` (package `integration`, black box through `internal/server`) at the user's request. Dockerfiles are `service/build/{api,worker,migrate}.Dockerfile` (build context `service/`), scripts `service/scripts/`, and `service/Makefile` adds `test-unit` and `test-integration`. Microservices stay separate deployables: one command, one image and one compose service each. Verified: `go vet ./...`, 129 tests (0 skipped), coverage 84.0 % of 12 483 statements, `make gen` reproduces the committed catalog, the three images build, and compose `migrate` → `api` starts healthy.

## IR175 Web on Next.js 16 per the official docs (step 1) — 2026-10-08

DEC-71 for the web: `service/web` moved from Next.js 15.5 to 16.4 (React 19.3) following "How to upgrade to version 16":

- `next lint` is gone: ESLint 9 flat config `eslint.config.mjs` with `eslint-config-next` core-web-vitals + typescript, script `npm run lint`; Node ≥ 20.9 in `engines`. The first run found 10 React errors and 62 warnings; all fixed: URL and localStorage state are read through `useSyncExternalStore` (`lib/urlState.ts`; `useUrlTab`, admin units customer/tab state, login returnTo) instead of effects, state that follows a prop is adjusted during render (AppShell menu, customer alerts list, unit controls), `useOp` derives `loading` from the request it holds, impure `Date.now()` moved to a lazy state initializer, the nested `NavLink` component moved to module level, and unused imports and locals were removed. `tsc`, `eslint .` and `next build` (Turbopack) pass.
- `proxy.ts` (the Next.js 16 name for middleware) performs the optimistic checks of the authentication guide in API mode only: it verifies the signed session cookie without network calls, sends signed-out users of a role area to `/login?returnTo=…` and users of another role to their own home. The Core API still authorizes every operation. `lib/session.ts` imports `server-only`.
- Security fix: the sign-in `returnTo` accepted protocol-relative paths (`//host`), an open redirect; `safeReturnTo` now accepts only same-origin paths (no `//`, backslash or control characters).

Next steps (IR176+): Server Components that read through a server-only data access layer instead of the browser calling `/bff/ops`, Server Actions for writes, `loading` / `error` files per segment, and Playwright E2E in `service/web/e2e`.

## IR176 Server-side data access in the web (Next.js DAL pattern, pilot) — 2026-10-08

Following the Next.js authentication and data guides, the web reads and writes through a server-only Data Access Layer instead of the browser calling `/bff/ops`:

- `service/web/lib/dal.ts` (`server-only`): `getSession` / `verifySession` (React `cache`, redirect to `/login`) and `coreOp(operation, input, {write, expectedVersion})`, which calls the Core API with the session's access token, tenant and membership and throws `CoreError` with the DomainError.
- Token refresh moved into `proxy.ts`: Server Components cannot write cookies, so an access token within 30 s of expiry is refreshed there (the only identity-provider call proxy makes) and the new cookie is forwarded both to the page render and to the browser; a failed refresh clears the cookie. Route Handlers keep refreshing through `readSession`. Proxy now also requires a session for the shared pages `/notifications` and `/settings` (307 to `/login?returnTo=…`).
- Pilot `/notifications`: `page.tsx` is a Server Component (`await connection()` because `DATA_SOURCE` is a runtime setting; otherwise the page was prerendered at build time) that reads `notifications.list` and passes plain rows to `_components/notifications-view.tsx`; marking read is the Server Action `actions.ts › markRead` (verifies the session through the DAL, sends the row version, `refresh()` re-renders the route, returns a message for version conflicts); `loading.tsx` and `error.tsx` (`retry`) follow the file conventions. The pure row mapping lives in `lib/notifications.ts`. The Phase 1A demo (mock) renders the same view with its in-browser rows.

Verified with the running stack: rows rendered on the server for customer-a, the Server Action marked a notification read, a repeat with the stale version returned the conflict message, and anonymous page or action requests got 307 to sign-in. `tsc`, `eslint .` and `next build` pass. The other screens follow the same pattern in later rounds.

## IR177 Server-rendered screens and local HQ sign-in — 2026-10-08

1. More screens follow the IR176 pattern (Server Component reads through `lib/dal.ts`, client view in `_components/`, Server Actions for writes, `loading.tsx` / `error.tsx`): customer overview (`app/customer/(overview)`, route group scopes its loading/error to the page), customer alerts, customer unit detail (unknown, invalid or other-customer IDs render not-found; `createCommand` and `setUnitAlertPolicies` are Server Actions; command status polling stays on the BFF Route Handler because Server Functions are for mutations) and the admin dashboard (`admin.summary` for today up to the Core API business clock, `dal.coreNow` = Meta.snapshotAt of `session.get`).
2. Local HQ sign-in: HQ users must use TOTP (IR117). The local demo realm `docker/keycloak/realm-ac.json` now pre-provisions one TOTP credential per HQ user (HmacSHA1, 6 digits, 30 s, the local test secret in the file) instead of forcing `CONFIGURE_TOTP`, so local and CI end-to-end runs can complete the second factor; Keycloak still asks for the code. The secret exists only in the local realm file and must never be used outside the local stack.

Verified: hq-operator signs in with password + TOTP, the admin dashboard renders the Core API KPIs on the server (2 customers, 5 units, 120.00 MYR billed); customer screens render server-side in API mode and keep their fixture data in the Phase 1A demo.

## IR178 One web app per entry point — 2026-10-08

User request: the four roles have different entry points, so each is its own web service (like `admin-web`). The single Next.js app `service/web` is split into four Next.js apps in one npm workspace (`service/package.json`):

| App | Routes | Local port | OIDC client |
|---|---|---|---|
| `service/customer-web` | `/customer/*` | 3000 | `ac-web` |
| `service/partner-web` | `/partner/*` | 3001 | `ac-web` |
| `service/technician-web` | `/technician/*` | 3002 | `ac-web` |
| `service/admin-web` | `/admin/*` | 3003 | `ac-admin-web` (HQ network rule, IR117) |

Shared code is the workspace package `@ac/web` (`service/web-shared`: components, lib with the DAL and session, shared screens — sign-in, notifications, preferences, demo panel, forbidden, not-found — the BFF Route Handlers and the proxy logic), consumed with `transpilePackages` as the Next.js monorepo guidance describes; each app re-exports the shared screens and Route Handlers in thin route files and keeps its own role routes, root layout, `proxy.ts` (fixed `matcher`, calls `authProxy(req, role)`) and `next.config.ts` (`output: "standalone"`, `outputFileTracingRoot` / `turbopack.root` at the workspace root, `env.AC_APP_ROLE`). Paths stay as before inside each app (`/customer/...` on the customer app), so links within a role are unchanged; links between apps are plain `<a>` links to `CUSTOMER_WEB_URL` / `PARTNER_WEB_URL` / `TECHNICIAN_WEB_URL` / `ADMIN_WEB_URL` (hard navigations). Each app has its own session cookie `ac_session_<role>` (local apps share the host `localhost`), its sign-in shows only its role, and the OIDC callback refuses an identity of another role (`/login?error=role`). Images: `build/web.Dockerfile --build-arg APP=<name>-web` (≈ 310 MB each); compose `<name>-web` (demo) and `<name>-web-api` (full). The Keycloak clients list the four local ports.

Verified: the four apps typecheck, lint and build; the demo profile serves each role on its port (the customer app answers 404 for `/admin`); in API mode customer-a, contractor-a, tech-external-a and hq-operator (with TOTP) sign in to their own apps and their pages render on the server; contractor-a signing in to the customer app is refused.

## IR179 service/api and service/web — 2026-10-08

User request: the web must not live inside the API ("APIの中にWebがあるのはおかしい") and the API is to be split into microservices by business domain (IR180+). Step 1 separates the trees: all Go code (module `github.com/pradita/ac-project/service/api`: `cmd/`, `internal/`, `test/integration/`, `build/*.Dockerfile`, `scripts/`, `Makefile`) moved to `service/api/`, and the npm workspace moved to `service/web/` with the apps `service/web/{customer,partner,technician,admin}` and the shared package `service/web/shared` (`@ac/web`); the web image is `service/web/Dockerfile --build-arg APP=<role>`. Compose build contexts are `./service/api` and `./service/web`. Verified: go vet, all Go tests (0 skipped), `make gen` unchanged, four web apps typecheck/lint/build, all images build, compose demo apps answer on 3000–3003 and `migrate` → `api` starts healthy.

## IR180 Core API as business-domain microservices (phase A) — 2026-10-08

User decision: split the API into microservices by business domain. Phase A turns the single Core API process into five domain services and a gateway; every service is its own command (`service/api/cmd/<name>`), image (`build/service.Dockerfile --build-arg SERVICE=<name>`) and compose / ECS service. The ownership map is code (`internal/ops/domains.go`) and covers all 197 operations (test `TestEveryOperationHasOneDomain`):

| Service | Catalog modules | Operations |
|---|---|---|
| `identity-api` | Identity & access, Notifications, Audit (Demo moved to equipment-api by IR185) | 27 |
| `equipment-api` | Assets, Devices, Control, Monitoring & alerts, Read models, Demo (IR185) | 66 |
| `maintenance-api` | Maintenance | 59 |
| `billing-api` | Billing, Restrictions | 30 |
| `energy-api` | Energy & carbon | 15 |

- A service dispatches only its domain's operations (`Registry.ServeDomains`); any other operation answers NOT_FOUND `error.unknownOperation`, so a misrouted request never runs (integration test `TestDomainServiceServesOnlyItsOperations`).
- `gateway` is the single entry for the web apps (`CORE_API_URL`): it forwards `POST /v1/ops/:operation` unchanged (headers incl. Authorization, tenant, membership, Idempotency-Key, X-Expected-Version, body, status) to `<DOMAIN>_API_URL`, keeps the request ID, answers NOT_FOUND for unknown operations and UNAVAILABLE when the service is down. It does not authenticate; each service verifies the token (IR173 pipeline). In production the ALB can route `/v1/ops/<prefix>.*` to the services directly.
- Workers, migrate and the shared demo clock (IR168) are unchanged; demo operations belong to `identity-api`.

Phase A keeps one PostgreSQL cluster. Each domain owns its schemas (identity, notify, audit / assets, devices, control, monitoring / maintenance / billing, restrictions / energy); modules still read — and in a few places write — other domains' schemas in SQL (corrected inventory in IR181) — for example billing and restrictions read assets and identity, energy reads assets, maintenance and monitoring, and the read models aggregate assets, billing, maintenance and energy. Phase B replaces these reads with service calls or events and then separates the databases; until then the services must be released together with schema-compatible migrations.

Verified: go vet, all Go tests (0 skipped) plus gateway tests (routing per domain, header and body forwarding, unknown operation, upstream down, env parsing); six images build; in compose the five services and the gateway start healthy; the four web apps in API mode, through the gateway, render the customer overview, alerts and notifications, the partner team, the technician unit and the admin dashboard.

## IR181 Phase B inventory and order — 2026-10-08

A scan of every SQL statement in `service/api/internal` (tests excluded) against the IR180 ownership map corrects IR180: besides cross-domain reads there are cross-domain **writes**, and every service reads identity tables to authenticate.

| From (service) | Writes to another domain | Reads from other domains |
|---|---|---|
| every service (`platform/auth`) | — | identity.memberships, users, membership_permissions, membership_scopes (principal per request) |
| billing-api (restrictions) | control.commands, assets.units (observed restriction), monitoring.alerts (reconciliation alerts) | assets.customers/units, control.commands, monitoring.alerts, identity memberships/permissions, notify.notifications, audit.audit_log |
| identity-api (demo operations in `internal/server`) | control.commands (acks), monitoring.measurements (telemetry trigger) | equipment, billing, maintenance tables used by demo triggers |
| identity-api (notifications, consents) | — | billing contracts/invoices/inquiries, restrictions, assets, devices, maintenance jobs/assignments/offers (recipient and target-scope checks) |
| equipment-api (unit scope, unit detail, read models) | — | maintenance jobs (read models; the unit scope reads its own projections since IR186), billing contract_units/invoices, restrictions, identity memberships/organizations/consents, notify |
| maintenance-api | — | devices.devices, identity.memberships |
| billing-api (billing) | — | assets, identity, notify (see first billing row) |
| energy-api | — | assets customers/properties/units, monitoring alerts/measurements, maintenance jobs/attachments, identity organizations/preferences, audit |

`migrate` (seed) writes every schema by design (fixture loader of the demo environment) and is not a business service.

Phase B order (each step keeps all tests green and is released on its own):

1. **Principal from identity-api.** Services stop reading identity tables in the auth middleware: identity-api exposes the membership principal (permissions, scopes, client role, employment, validity) on an internal endpoint; services cache it for 30 s keyed by membership and scope version (backend Go design §4).
2. **No cross-domain writes.** Restrictions send device commands and raise reconciliation alerts through equipment-api operations (or domain events) instead of inserting into control and monitoring tables; demo triggers move to the owning services.
3. **Reads replaced by events and local projections** (event-carried state transfer through the outbox, backend architecture §8): unit access windows (maintenance → equipment), unit / customer labels (equipment → billing, energy, identity), invoice and contract state (billing → equipment, identity), membership directory (identity → maintenance, equipment).
4. **Database per service:** once a service reads only its own schemas, its schemas move to its own database (or Aurora cluster) with its own migrations.

## IR182 Phase B step 1 done: principals from identity-api — 2026-10-08

Only identity-api reads the identity tables to authenticate. It serves `GET /internal/v1/principal?subject=&tenant=&membership=` (Bearer `INTERNAL_API_TOKEN`, constant-time compare; 200 with the principal — role, client role, employment, permissions, scopes, scope version —, 404 for an unknown, foreign, inactive or out-of-window membership, 401 for a wrong internal token). The gateway never routes `/internal/*`. The other four services resolve principals with `auth.RemoteSource` (`IDENTITY_INTERNAL_URL`): cached 30 s per subject, tenant and membership (backend Go design §4), outages are never cached and answer UNAVAILABLE 503 instead of UNAUTHENTICATED. Tests: `TestRemoteSource` (cache hit and expiry, 404, outage not cached, server down) and `TestPrincipalFromIdentityService` (identity-only service endpoint, wrong token, another user's membership, billing-only service authenticating through it, identity down → 503). Compose sets `INTERNAL_API_TOKEN` and `IDENTITY_INTERNAL_URL` for every Core API service. Verified in compose: identity-api answered the other services' principal requests and the customer, partner and admin pages rendered through the gateway.

## IR183 Event delivery between services (phase B step 2a) — 2026-10-08

User decision: cross-service writes are replaced by events (backend architecture §8), not by synchronous calls. Step 2a adds the delivery mechanism; step 2b moves the restriction ↔ equipment interactions onto it.

- Producers keep writing events into `platform.outbox` in the same transaction as the state change (`ops.Call.Emit`). The outbox gains `seq bigint GENERATED ALWAYS AS IDENTITY UNIQUE` and the index `(event_type, seq)`.
- `service/api/internal/platform/events.Consumer` (one per subscribing service, stable name) polls its event types in `seq` order for events it has not processed (anti-join on `platform.processed_events`), so a transaction that commits late is never skipped. Each event runs in its own transaction in the event's tenant with `app.now` = the event time (IR157); the `processed_events(consumer, event_id)` row is inserted in the same transaction, so delivery is at-least-once and applied exactly once. A failing handler rolls back, stops the pass (no reordering within the consumer) and is retried on the next poll (250 ms in the services).
- Inline mode: when one process serves every domain (tests, single-process runs) the dispatcher drains all consumers after each committed write (`Registry.AfterCommit`), so callers observe the same results as before; deployed domain services run their consumers as background loops.
- In production the outbox relay publishes the same rows to SNS/SQS; the handler contract (`Handler(ctx, tx, Event)`) and idempotency key stay the same.

Tests: `TestConsumerOrderDedupAndRetry` (order, tenant context, a failing event retried without reordering, independent consumers, replay ignored). All Go tests green; the dev and test databases were rebuilt for the new column (pre-release: `000001_init` is still `schema.sql`).

## IR184 Restrictions no longer write equipment tables (phase B step 2b, part 1) — 2026-10-08

The restrictions module (billing-api) stops writing `assets.units` and `monitoring.alerts`; it publishes events and equipment-api applies them (contracts in `service/api/internal/platform/events/contracts.go`):

| Event (billing → equipment) | Published when | Applied by equipment |
|---|---|---|
| `UnitRestrictionApplied {unitId, restrictionId, observed}` | an apply command is acknowledged | `assets.units.observed_restriction` = observed |
| `UnitRestrictionCleared {unitId, restrictionId}` | a release / recovery remove is acknowledged | observed restriction cleared when it still names the restriction |
| `RecoveryCasesChanged {restrictionId, unitIds}` | recovery cases change (SR29) | unit versions + 1 |
| `ReconciliationRequired {restrictionId, unitIds}` | a device still reports an ended restriction (SR26) | one open `reconciliation_required` alert per unit |
| `ReconciliationResolved {unitIds}` | units have no unresolved recovery case left | their reconciliation alerts resolved |

`events.Publish(ctx, tx, tenant, …)` writes the outbox row in the producer's transaction with an explicit tenant (device acknowledgements and worker ticks run without a tenant context); the equipment consumer runs every 250 ms in equipment-api, inline in the all-domain test server (repeated until no event is pending, since events can chain), and test helpers that call the device path directly drain the events afterwards. The device observation itself (`assets.units.observed_restriction` from `demo.trigger restriction_observation`) is now written by the trigger, before `restrictions.Observe` updates recovery cases. Remaining for part 2: device commands created and cancelled by restrictions (`control.commands`) and the reverse calls from control into restrictions on acknowledgement and expiry.

Verified: all Go tests green (restriction lifecycle, recovery, cancel / payment, exception release, override reconcile); in compose the deployed equipment-api consumer applied an injected `RecoveryCasesChanged` event within 2 s, recorded once in `platform.processed_events`.

## IR185 No cross-domain writes left (phase B step 2b, part 2) — 2026-10-09

Device commands of restrictions and the callbacks between control and restrictions now travel as events, and the demo operations belong to equipment-api. A scan of every INSERT / UPDATE / DELETE in the business modules and demo operations against the IR180 ownership map finds no statement on another domain's schema.

| Event | From → to | Effect |
|---|---|---|
| `RestrictionCommandRequested {commandId, unitId, deviceId, action, restrictionId, status, delivery, requestedAt, sentAt, expiresAt, correlationId}` | billing → equipment | equipment stores the command billing decided; billing assigns the UUIDv7 command ID, so restriction records reference it at once and the API still returns the command IDs and delivery state immediately |
| `RestrictionCommandsCancelled {commandIds}` or `{restrictionId, unitId}` | billing → equipment | cancel the given commands, or every undelivered intent of the restriction on the unit; applied after the request event in outbox order |
| `CommandAcknowledged {commandIds, at}` | equipment → billing | `restrictions.CommandAcknowledged` (apply / release / recovery progress); control no longer imports the restrictions module |
| `CommandsEnded {commandIds, at}` | equipment → billing | failed or expired restriction commands (`ExpireCommands`, device rejection) |
| `UnitRestrictionObserved {unitId, observed, eventId, at}` | equipment → billing | `restrictions.Observe` updates recovery cases (SR26) after equipment recorded the device observation |

- The Demo catalog module moves from identity-api to equipment-api (`demo.trigger` simulates device events). `demo.advanceClock` moves the shared scenario clock (IR168); it runs the cross-domain scheduler tick only when one process serves every domain; the split services leave the reached deadlines to the scheduler worker on the same clock.
- Equipment consumes restriction events and billing consumes command events (consumer names `equipment` and `billing`); in the all-domain process the chains are applied inline.
- Remaining phase B work (IR181 steps 3–4): cross-domain reads (for example restrictions read device connection and in-flight commands to decide delivery, the unit scope reads maintenance assignments) and the database split.

Verified: all Go tests green, none skipped (restriction lifecycle, cancel / payment, exception release / retry, override / reconcile, recovery cases, demo operations, commands, diagnostics, worker); the write scan reports 0 cross-domain writes; in compose the six services start healthy and `demo.advanceClock` is routed to equipment-api. The dev database was rebuilt afterwards because the smoke test had moved the shared demo clock to 2030.


## IR186 Unit scope without maintenance reads (phase B step 3, part 1) — 2026-10-09

The D01 unit scope (IR152, IR169, IR170) no longer joins `maintenance.jobs / offers / assignments`. Equipment keeps its own projection of the maintenance access it needs and the scope reads only that.

| Projection (schema assets, RLS) | Row | Kept while |
|---|---|---|
| `assets.unit_offer_access` | `(tenant_id, offer_id)` → job_id, unit_id, contractor_org_id, access_valid_from, access_valid_until | the Offer is accepted |
| `assets.unit_assignment_access` | `(tenant_id, assignment_id)` → job_id, unit_id, technician_membership_id, created_at, scheduled | the Assignment is active |

| Event | From → to | Effect |
|---|---|---|
| `OfferAccessChanged {offerId, jobId, unitId, contractorOrgId, accepted, accessValidFrom, accessValidUntil}` | maintenance → equipment | upsert the row while `accepted`, otherwise delete it |
| `AssignmentAccessChanged {assignmentId, jobId, unitId, technicianMembershipId, active, createdAt, scheduledFrom, scheduledUntil}` | maintenance → equipment | upsert the row while `active`, otherwise delete it |

- Change capture: row triggers `maintenance.publish_access()` on INSERT, DELETE and UPDATE of the access columns of `maintenance.offers` / `maintenance.assignments` write the full snapshot into `platform.outbox` in the same transaction. Every write path (operations, worker, fixtures, support SQL) is covered without instrumenting each one, and snapshots make replays and late deliveries idempotent (last event in outbox order wins).
- The predicate is unchanged: contractor = accepted Offer inside its access window; external technician = Membership.scopes ∩ own active Assignment (List: `[createdAt, scheduledEnd)`, Equipment: `[scheduledStart, scheduledEnd)`) ∩ an accepted Offer of the same job inside its access window.
- The split services see access changes after the consumer poll (≤ 250 ms). In the all-domain process `Registry.BeforeDispatch` drains pending events before each operation (as `AfterCommit` does after writes), so reads after direct SQL changes stay exact.
- The scope still reads `assets.units` and the projections from the other services that use it (maintenance, billing, energy); these move behind equipment with the database split (IR181 step 4).

Verified: all Go tests green, none skipped, including `TestUnitAccessProjection` (insert, accept, decline, revoke, delete) and the existing scope tests; in compose the projections match the seeded accepted Offers and active Assignments with no pending access events, and the contractor and external technician apps list their units through the gateway.

## IR187 Diagnostic-run scope from the assignment projection (phase B step 3, part 2) — 2026-10-09

`diagnosticRuns.get / list` (IR139) let a technician read the runs of jobs they hold or held an Assignment of. Control (equipment) read `maintenance.assignments` for this; it now reads `assets.unit_assignment_access`.

- The projection keeps revoked Assignments: `AssignmentAccessChanged` gains `deleted` (row deleted) and the row gains `active` (`status = 'active'`). Only `deleted` removes the row; the unit scope (IR186) filters `active`.
- Verified: all Go tests green, none skipped; `TestDiagnosticRuns` checks that runs stay visible after the Assignment is revoked, `TestUnitAccessProjection` that a revoked Assignment stays inactive in the projection.

## IR188 Generic change capture and notify reference copies (phase B step 3, part 3) — 2026-10-09

Notifications (identity-api) resolved targets and recipients (IR142, IR58) by joining assets, maintenance, billing, restrictions and devices. They now read reference copies in their own schema.

- `platform.capture_row(col, …)`: one AFTER row trigger function for every captured table. It publishes `RowChanged:<schema>.<table>` with `{op: INSERT|UPDATE|DELETE, row: {only the named columns}}` into `platform.outbox` in the writer's transaction; UPDATE triggers fire only for the captured columns.
- Consumer side: `events.Replica{Source, Table, Keys, Cols}` keeps `<schema>.ref_<table>` with the same column names (upsert via `jsonb_populate_record`, delete by key). Consumer `identity` applies `notify.Replicas`.

| Copy (notify, RLS) | Source columns |
|---|---|
| `ref_units` | assets.units id, tenant_id, customer_org_id, property_id, display_name |
| `ref_customers` | assets.customers id, tenant_id, organization_id |
| `ref_jobs` | maintenance.jobs id, tenant_id, unit_id, customer_org_id |
| `ref_offers` | maintenance.offers id, tenant_id, job_id, contractor_org_id |
| `ref_assignments` | maintenance.assignments id, tenant_id, job_id, technician_membership_id, status, scheduled (IR253) |
| `ref_invoices` | billing.invoices id, tenant_id, number, customer_id, status, due_at |
| `ref_inquiries` | billing.inquiries id, tenant_id, customer_id, subject_type |
| `ref_restrictions` | restrictions.restrictions id, tenant_id, customer_id |
| `ref_restriction_units` | restrictions.restriction_units restriction_id, unit_id, tenant_id |
| `ref_devices` | devices.devices id, tenant_id, serial, unit_id |

- Invoice targets take the customer organization from `ref_customers` via the invoice's customer (previously via the contract version; both name the same organization).
- The business-event recorder (`eventnotify`) runs inside maintenance-api's write transaction and reads maintenance rows of the same transaction; it is unchanged here (its identity reads are listed in IR181).
- The IR186 projections stay as they are (they derive windows); new cross-domain reads use the generic copies.

Verified: all Go tests green, none skipped; `TestNotifyReplicas` compares every copy with its source (EXCEPT both ways) after updates, inserts and deletes; the notification and inbox tests pass on the copies.

## IR189 Energy reference copies and the principal timezone (phase B step 3, part 4) — 2026-10-09

Energy (energy-api) read assets, identity, maintenance and monitoring tables. It now reads its own copies (IR188 mechanism, consumer `energy`).

| Copy (energy, RLS) | Source columns | Used by |
|---|---|---|
| `ref_units` | assets.units id, tenant_id, customer_org_id, property_id, archived | summary, baselines, quotes, MRV, report scope checks |
| `ref_customers` | assets.customers id, tenant_id, organization_id | offset customer and records |
| `ref_properties` | assets.properties id, tenant_id, customer_org_id, archived | monthly report |
| `ref_organizations` | identity.organizations id, tenant_id, kind | MRV preview (customer organizations only) |
| `ref_attachments` | maintenance.attachments id, tenant_id | MRV evidence check |
| `ref_alerts` | monitoring.alerts id, tenant_id, unit_id, detected_at | monthly report counts |
| `ref_jobs` | maintenance.jobs id, tenant_id, unit_id, status, updated_at | monthly report counts |
| `ref_power_samples` | monitoring.measurements (minute-slot `power` rows only, trigger `WHEN`) sensor_id, observed_at, sequence, tenant_id, unit_id, value, unit, origin, quality, boundary_id, event_id | D07 integration (`Integrate`) |

- One capture trigger per source table: its column list is the union of what the consumers copy (units add `archived`, jobs add `status`, `updated_at`); each replica copies its subset.
- `platform.capture_row` names the partition root as the source, so rows of `monitoring.measurements_*` partitions publish `RowChanged:monitoring.measurements`.
- The report timezone comes with the principal: identity resolves `preferences.timezone` with the membership (`Principal.Timezone`, wire field `timezone`; default Asia/Kuala_Lumpur). In the split services a preference change applies after the principal cache (30 s).
- Production note: in the demo the power samples travel through the outbox; at telemetry volume the same rows come from the ingestion stream (Kinesis) into energy's store.
- Still shared: energy writes and reads `audit.audit_log` for offset event history (audit is shared infrastructure until the database split, IR181 step 4).

Verified: all Go tests green, none skipped; `TestReplicas` compares all notify and energy copies with their sources, including power samples; the energy summary, baseline, offsets, MRV and report tests pass on the copies.

## IR190 Read models by API composition (phase B step 3, part 5) — 2026-10-09

`summaries.get` and `admin.summary` (equipment-api, catalog module Read models) combine logic owned by other domains: job counts with the IR26 / IR49 projections, overdue invoices, energy actuals and the IR78 forecast, organization status (IR40). Copying those tables would duplicate the owners' rules, so the read models call the owners instead.

| Internal query | Owner | Input → output |
|---|---|---|
| `maintenance.countJobs` | maintenance-api | `{filters}` (jobs.list filters) → JobCounts (offer, active, review, scheduled, inProgress, overdue, assigned, units) |
| `maintenance.statusCounts` | maintenance-api | `{unitIds, from, to}` → `{status: count}` of jobs whose requested slot starts in the period |
| `billing.overdue` | billing-api | `{unitIds}` → `[{currency, count, amountMinor}]` unpaid past due on contracts covering the units |
| `energy.actuals` | energy-api | `{unitIds, from, to}` → `{summary, forecast}` (default factor, no baseline) |
| `identity.activeOrganizations` | identity-api | `{ids}` → active organization IDs |

- `ops.RegisterQuery(r, domain, name, h)` binds a query; `ops.Ask` (generic over the output type; arguments ctx, registry, call, name, input) runs it in the caller's transaction when the process serves the owner's domain, otherwise through `ops.HTTPQueries`.
- Wire: `POST /internal/v1/queries/<name>` on each service (not routed by the gateway), behind the normal authentication middleware with the caller's `Authorization`, `X-Tenant-Id`, `X-Membership-Id`, plus `X-Internal-Token: INTERNAL_API_TOKEN`, `X-Request-Id` (correlation) and `X-Business-Now` (the caller's business time, IR157). The owner runs the query in a read transaction with the caller's principal and applies its own scope rules; errors come back as DomainError. Missing token → 401; a query of another domain → 404.
- Configuration: `<DOMAIN>_API_URL` per service (compose: identity, maintenance, billing, energy); without it a remote query is UNAVAILABLE.
- In the split services the composed read is not one snapshot across domains (each owner reads its own committed state); dashboards accept this.
- Remaining in-process cross-domain module calls (for example control → maintenance access, restrictions → equipment device models, voice → assets) are the next candidates for the same mechanism (IR181).

Verified: all Go tests green, none skipped; `TestReadModelsAcrossServices` runs equipment-api alone against identity, maintenance, billing and energy servers over HTTP and gets the same `admin.summary` and `summaries.get` (customer, partner, technician) data as the single process, and checks the token and domain guards; in compose the admin and partner apps load the summaries through the gateway with the split services.

## IR191 Notifications by events and identity membership queries (phase B step 3, part 6) — 2026-10-09

Equipment (alert policies), maintenance (job events, IR95), billing (inquiries, payment reminders) and restrictions (notices) stored notifications in `notify.notifications` and chose recipients from `identity.memberships` in their own transactions. Both now go through identity-api.

- **Create**: `notify.Store.Create` assigns the notification ID (UUIDv7) and publishes `NotificationRequested {notificationId, recipientMembershipId, channel, type, templateKey, target {kind, id}, params, severity, sourceAlertId, occurredAt}` in the producer's transaction. The `identity` consumer inserts the notification with that ID and the recipient's scope version (`ON CONFLICT (id) DO NOTHING`; a recipient without a membership row is skipped). API results that return notification IDs (alert outcomes, `invoices.remind`, restriction notices) keep returning them at once.
- **Recipients**: internal query `identity.members {ids?, role?, organizationId?, permission?}` returns the memberships active at the caller's business time (sorted). It replaces the membership SQL of the alert notifier (default policy clients; custom recipients that are clients of the unit's organization or admins with `alert.read`; the policy owner check), restriction notices, inquiry notifications (excluding the caller), `Directory.ActiveClientOf` (payment reminders) and the job-event recorder (clients, HQ with a permission, partner members with a permission, the assigned technician).
- **Restriction notice evidence** (`restrictions.execute`): internal query `identity.notificationsStored {ids, targetId, occurredAt}` counts the stored notices.
- **Alert cooldown**: `monitoring.alert_notifications (notification_id, alert_id, policy_id, unit_id, occurred_at)` records the notifications equipment requested; the cooldown reads it instead of `notify.notifications`.
- The job-event recorder reads restriction organizations from `billing.contracts` (same domain as restrictions) instead of `assets.customers`.
- Delivery is asynchronous between the split services (consumer poll ≤ 250 ms); the all-domain process applies the events before the response.

Verified: all Go tests green, none skipped; `TestNotificationsAcrossServices` runs billing-api alone against identity-api over HTTP: the inquiry notification is not stored by billing and appears after identity's consumer runs. The existing alert, inquiry, reminder, restriction and job-event notification tests pass unchanged.

## IR192 Query-backed identity Directory, qualification grants by event, maintenance copies (phase B step 3, part 7) — 2026-10-09

Other domains called `identity.Directory` in-process, so its SQL ran in their transactions; `SetGrant` even wrote `identity.qualification_grants` from maintenance-api (`certificates.verify`). The provider methods now delegate to identity.

- `ops.Delegate(ctx, call, query, input, local)`: the body of a provider method. With a request registry it asks the owner (same transaction when served locally, `/internal/v1/queries` otherwise); calls without a registry (the scheduler worker) run `local` directly. Consumers and the wiring in `app.go` stay unchanged.
- Directory queries: `identity.nonReaders {ids, customerOrgId}` (alert policy recipients, IR120), `identity.orgState {orgId}` → `{kind, status, found}`, `identity.orgName {orgId}`, `identity.technician {membershipId}` → `{orgId, employment, active, scopes, found}`, `identity.qualified {membershipId, codes, start, end}` (IR123); `ActiveClientOf` uses `identity.members` (IR191).
- `SetGrant` publishes `QualificationGranted {membershipId, code, validFrom, validUntil}`; the `identity` consumer updates or inserts the grant. Between the split services the grant applies after the consumer poll; assignment checks that follow use it from then on.
- Maintenance reference copies (consumer `maintenance`): `ref_memberships` (all membership columns; capture trigger on `identity.memberships`) for technician planning (`members.*`), unavailability conflicts and the worker's history snapshots (`FreezeEnded`); `ref_devices` (id, tenant_id, serial, unit_id) for QR serial lookup (IR145).

Verified: all Go tests green twice on the same database, none skipped; `TestDirectoryAcrossServices` runs maintenance-api alone against identity-api: certificate submission checks the technician over HTTP (another contractor's technician is NOT_FOUND) and the approved certificate's grant appears after identity's consumer runs; `TestReplicas` covers the maintenance copies.

## IR193 Cross-domain adapters through owner queries; job notes by event (phase B step 3, part 8) — 2026-10-09

The composition root (`internal/server/app.go`) wired modules of one domain directly into another domain's operations. An audit of every adapter found these cross-domain providers; each now delegates (`ops.Delegate`, IR192) to an internal query of its owner.

| Consumer → provider | Methods → internal queries |
|---|---|
| equipment (assets units / customers) → maintenance | `maintenance.unitInUse`, `maintenance.claimableJobs`, `maintenance.unitActive`, `maintenance.orgActive` (`maintenance.Usage`) |
| equipment (assets) → billing | `billing.unitInUse`, `billing.activeContracts`, `billing.unitActive`, `billing.orgActive` (`billing.Usage`) |
| equipment (assets, devices, control) → billing (restrictions) | `restrictions.unitBusy`, `restrictions.unitPolicy` (`restrictions.Busy`) |
| equipment (monitoring, control) → maintenance | `maintenance.technicianJob`, `maintenance.technicianUnit`, `maintenance.jobVersion` (`maintenance.Access`, IR94) |
| maintenance, billing, restrictions → equipment | `assets.unitState`, `assets.unitsOfProperty`, `assets.orgOfCustomer`, `assets.unitsOfOrg`, `assets.unitService`, `assets.customerProfiles`, `assets.siteAddress`, `assets.warrantyEnd`, `assets.briefUnits`, `assets.customerOfOrg`, `assets.customerState`, `assets.restrictionTarget`, `monitoring.unitSeverities`, `monitoring.runHours`, `devices.boundDevice`, `devices.operationBusy` (adapter `equipmentView`) |
| billing (payouts) → maintenance | `maintenance.acceptedJobs`, `maintenance.rateAt` (adapter `maintenanceView`) |

- `PayoutSource.AddJobNote` wrote `maintenance.job_notes` / `job_events` from billing-api; billing now publishes `JobNoteRequested {jobId, authorUserId, message, at}` and the `maintenance` consumer adds the note and its `note.added` event with that author and time.
- Equipment's own modules still call assets, monitoring and devices directly; only other domains go through `equipmentView`.
- Configuration: every service gets `<DOMAIN>_API_URL` of the others (compose adds `EQUIPMENT_API_URL`).
- Tests: `newCluster` starts one server per domain wired over HTTP like compose; the split tests (domain dispatch, remote principals, read models, notifications, directory) run on it.
- Still direct: the scheduler worker runs every domain's clock transitions in one transaction (its calls run locally), and `audit.audit_log` is written by every service.

Verified: all Go tests green, none skipped; in compose the six services are healthy, the client and partner apps list invoices and jobs (billing and maintenance ask equipment), filter care shows run hours through equipment, and the services log no errors.

## IR194 Least-privilege database role per service; the suite runs on the split services (phase B step 3, part 9) — 2026-10-09

Each business-domain service now connects with a role that reaches only its own schemas, so any remaining cross-domain table access fails instead of silently working on the shared cluster.

- **Roles** (`schema.sql`): `ac_svc_identity` (identity, notify), `ac_svc_equipment` (assets, devices, control, monitoring), `ac_svc_maintenance` (maintenance), `ac_svc_billing` (billing, restrictions), `ac_svc_energy` (energy); all with DML on `platform` and `SELECT`/`INSERT` on `audit` (still shared). `ac_app` stays for the scheduler worker and tools. `cmd/migrate` creates the local logins `ac_<domain>_login`; compose gives every service its own login.
- **Errors**: SQLSTATE 42501 "permission denied …" is a defect and maps to UNAVAILABLE (row-level security violations stay NOT_FOUND); DomainErrors keep the converted cause, which the error handler logs for 5xx.
- **Cluster mode of the integration suite** (`make test-cluster`, `AC_TEST_CLUSTER=1`): an all-domain test server becomes a facade — the gateway in front of five single-domain services on their own roles, principals from identity-api (`Config.PrincipalTTL` negative: no cache, since the suite edits memberships directly), internal queries over HTTP, one shared scenario clock (`Config.DemoClock`, like `platform.demo_clock` in compose) and the worker's tick after `demo.advanceClock`; pending events are applied before and after every request. All 94 integration tests pass in both modes.

The first cluster run found these accesses, now removed:

| Operation (service) | Was reading | Now |
|---|---|---|
| `units.get` unit detail (equipment) | `restrictions.restriction_units`, `restrictions.restrictions` | billing queries `restrictions.unitRestriction`, `restrictions.unitRecovering` |
| commands, diagnostics (equipment) | `restrictions.restrictions` recovery cases | `restrictions.unitRecovering` (`control.Restrictions` interface) |
| `automations.save / simulate / evaluate` (equipment) | `identity.consents`, `identity.memberships` / permissions | `identity.consent`, `identity.members` (`identity.ConsentGranted`, `identity.Members`) |
| `restrictions.*` (billing) | `assets.customers`, `assets.units` (observation), `control.commands` | `billing.ref_customers` copy; `assets.unitObservation` query; billing's own `restrictions.restriction_commands` |
| `inquiries.*` (billing) | `assets.customers` | `billing.ref_customers` |
| `members.save`, `clientUsers.*` (identity) | `assets.customers / properties / units` | `notify.ref_customers / ref_properties / ref_units` |
| `members.eligible / capacity` (maintenance) | `identity.*` through `identity.LoadMembers` | `identity.loadMembers` query (also fills the missing display name, IR172) |
| `units.resolveQr` (equipment; Assets in the catalog) | `maintenance.assignments / jobs` (implemented in maintenance) | implemented in assets; `maintenance.qrAssignment` query; `maintenance.ref_devices` dropped |
| `demo.trigger` restriction observation (equipment) | `restrictions.restriction_units` (tenant probe) | `assets.units` |

- `restrictions.restriction_commands (command_id, restriction_id, unit_id, kind, delivered, status)`: billing records every command it requests (`requested` = open), mirrors its cancellations with equipment's rule (undelivered intents of the restriction on the unit, or explicit IDs), and applies `CommandAcknowledged` / `CommandsEnded` (which now carries `statuses` per command: failed / expired / cancelled). Open-command checks, acknowledgement lookups and end handling read it instead of `control.commands`; the fixture seeds it with its restriction commands.

Verified: `make test` and `make test-cluster` green (94/94 integration tests in cluster mode), none skipped; in compose every service runs on its own login, the four web apps' main reads work through the gateway (client, partner, technician, admin), `units.resolveQr` answers the assigned technician and is NOT_FOUND for another, and the services log no errors.

## IR195 Scheduler per business domain; system internal queries (phase B step 3, part 10) — 2026-10-09

The scheduler worker ran every domain's clock-driven transitions in one transaction on the all-domain role. It now runs per domain.

| Scheduler | Transitions | Database login |
|---|---|---|
| `worker --role=scheduler --domain=equipment` | command expiry (IR138), diagnostic run advance (start / end actions; restriction policy asked from billing) | `ac_equipment_login` |
| `worker --role=scheduler --domain=maintenance` | offer expiry (IR48), slot proposal expiry, unrated job confirmation, history snapshots (IR124) | `ac_maintenance_login` |

- A domain scheduler is built from the same wiring as its service (`server.ConfigFromEnv`, `server.New` without HTTP): the domain's role, the registry with the internal queries of other domains, and the shared scenario clock (IR168). `scheduler.TickDomains(ctx, db, now, domains, registry)`; `scheduler.Tick` (all domains, local) stays for tests and single-process development (`worker --role=scheduler` without `--domain`).
- **System internal queries**: calls without a user (workers) go to `POST /internal/v1/system/queries/<name>` with `X-Internal-Token` and `X-Tenant-Id` only; the query runs with a tenant-only principal. `ops.HTTPQueries` picks this route when the call has no bearer token. Missing or wrong token, or no tenant → 401.
- Compose: `worker-scheduler-equipment` and `worker-scheduler-maintenance` replace `worker-scheduler`, each with its domain login and the services' environment (internal token, `<DOMAIN>_API_URL`).
- Cluster mode of the suite runs both domain schedulers with their roles and registries wherever a test ticks the worker (`schedTick`) and after `demo.advanceClock`.

Verified: `make test` and `make test-cluster` green (96 integration tests), none skipped; in the cluster run the equipment scheduler asks billing `restrictions.unitPolicy` over the system route; `TestSystemQueries` checks the token and tenant guards; in compose both schedulers run on their logins and a client's command expires after `demo.advanceClock` (`worker tick: … ExpiredCommands:1`).

## IR196 The audit log belongs to identity (phase B step 3, part 11) — 2026-10-09

Every service inserted its audit rows into `audit.audit_log` in its own write transaction, and billing (restrictions) and energy (offset records) read the history of their records from it.

- **Write**: the recorder publishes one `AuditRecorded {id, actorId, actorRole, membershipId, action, targetKind, targetId, previousVersion, nextVersion, occurredAt, correlationId, result, reason}` per audit entry in the writer's transaction (the writer assigns the UUIDv7 entry ID); the `identity` consumer inserts the row (`ON CONFLICT (id, occurred_at) DO NOTHING`). The business effect stays exactly-once and its audit entry at-least-once, as before for events. A handler never saw its own entry (the recorder runs after the handler), so responses are unchanged.
- **Read**: `audit.history {targetKind, targetId}` (owner identity; `audit.History`) returns a target's entries in log order; restriction details (`events`) and offset records (`eventHistory`) use it. `audit.list` (FR-A16) is identity's own read.
- **Roles**: only `ac_svc_identity` keeps `INSERT`/`SELECT` on audit; the other domain roles have no audit access, so the cluster suite fails on any direct audit write or read elsewhere.
- With this, a domain service touches only its own schemas and `platform` (outbox, idempotency keys, processed events, tenants, demo clock); the database split (IR181 step 4) can follow.

Verified: `make test` and `make test-cluster` green, none skipped; the recorder unit test checks the published entry and row-level security on the log; in compose a client's `inquiries.create` on billing-api appears in the HQ audit view from identity-api with no audit events pending, and the services log no errors.

## IR197 System Architecture diagrams follow the domain services — 2026-10-09

The Figma page “System Architecture” (file VOeKPrid46kOf24ktEfe8r) still showed the modular-monolith Core API, a single `web/` app and one all-domain scheduler.

- **Board 03** (production backend): the Core API is the gateway plus five business-domain services; the notes map catalog modules to services and state the domain boundaries (events, reference copies, internal queries; enforced by database roles and the split-service tests); the scheduler worker runs per domain.
- **Board 06** (Go implementation and database): Echo v5; `service/api` layout (`cmd/<service>`, `internal/server`, `internal/gateway`, `internal/ops` with domains and internal queries, `internal/migrations`, `test/integration`); principals from identity-api; outbox with domain events and `AuditRecorded`; schemas labelled with their owning service, the reference copies and projections, `restriction_commands`, `alert_notifications`, `demo_clock`; one database role per domain; the 42501 mapping of IR194; enforced dependency rules; workers and consumers per domain.
- **Board 07** (containers): four web images from `service/web/Dockerfile`, `ac-gateway` and `ac-<domain>-api` from `service/api/build/service.Dockerfile`, `ac-worker` / `ac-migrate`; compose `gateway + 5 services`, `worker-scheduler × 2`, web apps on 3000–3003, the `planned` profile.
- **Board 08** (new): request path (web apps → gateway → services), one card per service (operations, catalog modules, schemas and role, copies, consumed events, answered queries, schedulers), the three cross-domain contracts (events, change capture, internal queries), workers, enforcement and the open database split.
- Compose: `iot-bridge` and `device-sim` move to the `planned` profile with `webhook` (their commands are not built yet, so `--profile backend` no longer starts failing containers); the profiles table of container-design.md lists the current services.

## IR198 Frontend board as built; data source of every page — 2026-10-09

Board 02 of the Figma page “System Architecture” still showed the single `web/` app on Next.js 15 with page-level stores. It now shows the frontend as built (IR175–IR179): the four apps and `@ac/web`, the request and data flow in API mode (`proxy.ts` optimistic check and token refresh → Server Component reading through `lib/dal.ts` → client view → Server Actions → gateway), the data source of every page, and the differences from the design (board 01, common.md §1 / §8). Boards 03–08 moved down to keep the spacing.

Data source of the 72 pages in API mode (measured from the imports of every `page.tsx`):

| App | Server (DAL + Server Actions) | Browser through `/bff/ops` | Fixture rows only | No data |
|---|---|---|---|---|
| customer | overview, alerts, units/[id], notifications | maintenance, preferences, demo, login | air-quality, automations, energy, energy/offsets, payments, payments/[id], properties, users (8) | /, forgot-password, forbidden |
| partner | team, units/[id], notifications | overview, jobs, jobs/[id], schedule, history, preferences, demo, login | jobs/[id]/review, payouts (2) | same three |
| technician | units/[id], units/[id]/alerts, notifications | overview, jobs/[id], preferences, demo, login | jobs, devices, devices/[id], units/[id]/control (4) | same three |
| admin | overview, notifications | jobs, preferences, demo, login | alerts, audit, billing, billing/contracts, devices, energy, mrv, offsets, restrictions, restrictions/[id], settings/access, settings/automation, units (13) | same three |

- 27 data screens still render fixture rows in API mode; the Core API implements all 197 catalogued operations (registry check), so the gap is the web wiring. Order of work: admin (13), customer (8), technician (4), partner (2), each to the IR176 pattern.
- Design deviations recorded for a decision: the data-access layering of common.md (page → feature hook → Repository → adapter) is replaced by the Next.js DAL / Server Components pattern the user asked for (to be written into common.md §1 / §8); the Navigation interface, shadcn/ui, React Hook Form + Zod, i18next and the Vitest / Playwright / axe test stack are not implemented yet.

## IR199 Admin audit, alerts and billing on the server pattern; invoice scope filters and numbering — 2026-10-09

Three admin data screens of the IR198 inventory now follow IR176 (Server Component reading through `lib/dal.ts`, a client view, Server Actions for writes, `loading.tsx` / `error.tsx`); the admin fixture-only count drops from 13 to 10.

- **Audit** (`/admin/audit`, FR-A16): `audit.list` for the URL filters (`from`, `to`, `corr`, `result`, `limit` up to 100), the device events tab (`tab=devices`, `deviceId`) with `devices.list` and `devices.events`.
- **Alerts** (`/admin/alerts`, FR-A05): `alerts.list`, `policies.list`, `units.list`, `customers.list`, `members.list`; Server Actions `alerts.acknowledge`, `alerts.resolve` (reason and evidence IDs) and `policies.save` (recipients, channels, recovery threshold checked before the call).
- **Billing** (`/admin/billing`, FR-A08 / FR-A23): the SCR-A08 URL keys (`tab`, `invoiceId`, `inquiryId`, `statementId`, `customerId` → `propertyId` → `contractId`, `overdueOnly`); `invoices.get` of the selected invoice, `notifications.recipients` for an overdue one; Server Actions `invoices.create`, `payments.recordManual` (only when the invoice has no Payment), `payments.confirm` (the processing Payment and its version), `notifications.preview` then `invoices.remind`, `inquiries.answer`, `payouts.generate`, `payouts.transition`, `payouts.resolveQuery`. A link with only `inquiryId` opens the inquiries tab (D08).

Backend and contract fixes found while wiring:

1. **invoices.list filters.** The catalogued `propertyId` (SR06: at least one unit of the invoice's contract version belongs to the property) and `from` / `to` (`[from, to)` on the period start; `from ≥ to` is VALIDATION `filters.to`) were rejected as unknown keys. billing keeps `billing.ref_units (id, tenant_id, property_id)`, a reference copy of `assets.units` fed by the existing `units_capture` change events (IR194 pattern).
2. **Invoice number.** `INV-<YYYYMM>` used the UTC month of the period start, so a period starting at local midnight on 1 September was numbered August. The month is now taken in Asia/Kuala_Lumpur, the tenant sequence is drawn under a transaction advisory lock (two concurrent creates no longer collide on `(tenant_id, number)` and report a false `errors.invoice_period_exists`), and the seed uses the same rule instead of the due month. `Invoice.number` is added to service-contracts.ts; the API already returned it.
3. **Deep links (D08).** Admin notification links carry the target: `/admin/units?unitId=`, `/admin/jobs?jobId=`, `/admin/billing?invoiceId=` / `?inquiryId=`, `/admin/devices?deviceId=`; the client job link is `/customer/maintenance?jobId=`. The admin dashboard billing links open `/admin/billing?overdueOnly=true` (DD-A01 step 7). Links that need a related ID (an inquiry's or restriction's invoice for clients, a device's unit, the contractor or technician `jobId`) still open the list and are resolved when those screens are wired.
4. **Confirmation reason.** DD-A08 shows the payment reference and the confirmation reason of a confirmed payment, but Payment carried no reason. `Payment.confirmationReason` returns the reason of `payments.confirm` / `payments.recordManual` to HQ callers and null to everyone else. The HQ invoice detail shows the method as “Not selected” when it is null (manual payments never invent one) with each payment's status, reference and reason.
5. **Action errors.** Failed Server Actions show readable text from the ServiceError (`@ac/web/lib/actionMessage`: field errors, version conflicts, domain conflict keys, codes) instead of message keys.

## IR200 The query catalog and the list handlers agree; a test reads every row — 2026-10-09

query-catalog.csv is the filter and sort allowlist of every Query (SR06), but the handlers had drifted: some catalogued keys were rejected as unknown, some were named differently, a few handlers silently ignored filters or sorts, and the catalog's sort columns were placeholders. Both sides now agree, and `TestQueryCatalogFilters` (test/integration) reads the CSV and checks, for each of the 45 rows, that an unknown filter key is VALIDATION, each catalogued key is accepted on its own, each allowed sort field sorts both ways and an unknown sort field is VALIDATION.

1. **Keys renamed to the catalog.** `contracts.list` `kind` (was `planType`); `inquiries.list`, `offsets.list`, `restrictions.list` and `restrictions.forInvoice` `status` mapped to the stored state (was `state`); `parts.list` `search` (was `q`) plus `membershipId`; `members.list` `activeOnly` (was `active`, whose false meant inactive only; false now adds no validity filter).
2. **Catalogued keys implemented.** `baselines.list` unitIds, from/to on the period start, customerId and propertyId through the units; `certificates.list` organizationId and expiringWithinDays (status values checked); `clientUsers.list` and `contractors.list` search; `members.list` qualification; `members.eligible` and `members.capacity` organizationId, qualification and activeOnly (they ignored filters); `notifications.list` and `offsets.list` from/to; `devices.events` from/to (rejected before); `restrictions.forInvoice` contractId and a second invoiceId that must also be cited (rejected before).
3. **No filters where the catalog lists none.** `diagnosticRuns.list`, `devices.calibrations` and `devices.operations` return VALIDATION on `query.filters` (diagnostic runs ignored them); `mrv.versions` checks its query before the report lookup.
4. **Implemented keys added to the catalog.** `audit.list` targetKind and action, `baselines.list` method and boundaryId, `certificates.list` code, `clientUsers.list` clientRole, `factors.list` region and year, `inquiries.list` subjectType and customerId, `members.list` role, `notifications.list` type.
5. **Sorts.** allowed_sort and default_sort of 23 rows follow the implementation and the IRs: for example audit newest first, invoices by due date descending, notifications by occurredAt descending (IR notifications.list item 2), certificates by expiresAt (the row said dueAt), `notifications.recipients` without a sort (role, label, id). Handlers that ignored the sort now apply it and reject unknown fields (certificates, parts, eligible members, capacity, payouts, rate cards, diagnostic runs, MRV versions by version); client users and parts default to name.

## IR201 Admin contracts and devices on the server pattern; audit before/after values — 2026-10-09

Two more admin data screens follow IR176, and writes now record what they changed.

- **Contracts** (`/admin/billing/contracts`, FR-A07): `contracts.list` for the URL scope (`customerId` → `unitId`, the plan-type chip `kind`), every current contract to label units already on one (DD-A07 step 6), customers, properties and units; the editor saves with `contracts.save` and the version it was read at. Eligibility and the rules version are enabled only for RTO; save is disabled with the SR19 reason while a restriction or recovery is open; an unchanged date keeps its stored instant.
- **Devices** (`/admin/devices`, FR-A04 / FR-A20): `tab=models` reads `capabilities.list` with unit and device counts and saves `capabilities.save` (the next version needs the version read and a change reason); the version history is `audit.list` of the capability over the last 12 months, read on explicit open and only offered with audit.read. `tab=devices` reads `devices.list`, and for `deviceId` `devices.get`, `devices.operations`, `devices.calibrations` (newest first) and `devices.events`; register, bind (reason), connection check, calibration (at the business clock) and firmware update (a candidate of the unit's model) are Server Actions with the device version; check, calibration and firmware stay disabled while an operation is active. `tab=firmware` reads `firmwareCampaigns.list` and the `campaignId` detail (waves, per-device results), schedules campaigns (waves ascending to 100 %, auto-pause 1–50 %, window, start at least 24 hours ahead in Kuala Lumpur time) and pauses, resumes, aborts (reason) or retries a device.

Backend fixes found while wiring:

1. **Audit before/after values.** audit_log had `masked_before` / `masked_after`, and AuditView returned them, but no write filled them. `ops.AuditEntry` now carries Before / After; `ops.Changes` keeps only the fields whose value changed (strings as they are, other values as JSON), the recorder masks sensitive fields (names containing password, secret, token, phone, email, contact, totp, iban or card become `***masked***`), and the AuditRecorded consumer stores them. `capabilities.save` and `contracts.save` record their changed fields; other writes keep empty maps until they are added.
2. **Capability change reason.** `capabilities.save` with an id now requires `changeReason` (VALIDATION `error.required`; DD-A04 says required on update).
3. **Shared web pieces.** `corePermissions()` in the DAL reads the membership's permissions once per render (session.get) to show permission-scoped sections; `useUrlPatch` and `useAction` are the URL-state and Server Action helpers of the client views; `ConnBadge` shows unknown and error connections.

## IR202 Admin energy analysis on the server pattern; the IR68 / IR44 formatter in the web — 2026-10-09

`/admin/energy` (FR-A13) follows IR176. The analysis tab reads `energy.summary` for the URL scope: `customerId`, `unitIds` (comma separated), `from` / `to` (Kuala Lumpur datetime-local values, minute aligned as D07 requires) and `baselineId`; a selected baseline supplies the default unit set and period, otherwise the default is the customer's units from local midnight to the current minute. It shows actual, baseline, difference in kWh and %, cost and saved cost, emissions and saved emissions, the not-adjusted label, the calculation conditions (period, boundary, baseline snapshot and assumptions, factor snapshot, tariff, coverage) and each quality warning; a VALIDATION result of the period or unit set shows next to the form. The baselines tab lists `baselines.list` for `customerId` → `propertyId` → `unitId` and the period start window, and saves a new baseline or the next version with `baselines.save` (baselineKWh only for demo_fixed; period comparison measures at save time and refuses whole_building_electricity).

- `@ac/web/lib/energy` holds the IR68 formatter (`saving`: Reduction / Increase / No change 0.0 / Cannot calculate) and the IR44 number rules (one decimal for kWh, % and kgCO₂e, amounts with two decimals and the currency code, rounding half away from zero without `toFixed`), for the customer energy screen as well.
- The screen catalog lists `from` and `to` among the SCR-A13 URL keys: DD-A13 step 6 names the period as a URL key and the analysis needs one too.

## IR203 Admin MRV demo workspace on the server pattern; MRV evidence selection is open — 2026-10-09

`/admin/mrv` (FR-A14) follows IR176. The reports tab reads `mrv.list` for the URL filters (`organizationId`, `unitId`, `status`, `from` / `to` on the conditions start) and, for `reportId`, `mrv.get` of `reportVersion` (latest when omitted) with `mrv.versions` for the version selector; earlier versions are read only. The detail shows electricity, Scope 2 emissions, energy vs baseline and emissions vs baseline as separate figures with the IR68 wording, or “Calculation incomplete” without zero filling; the stored condition snapshot (organization, units, period, boundary, baseline and factor versions with region, year, value and source, coverage), the quality warnings, evidence and the demo review history, with the review action on the latest complete draft only (mrv.review). A new report or a new version (mrv.write) takes customer organization → units, period, a baseline version, a factor version and the boundary, previews with `mrv.preview` (“Not saved”, “Demo — unverified”) and saves with `mrv.saveDraft`. The factors tab lists `factors.list` and saves a new factor or the next version with `factors.save` (mrv.factors).

- The screen catalog lists `from` and `to` among the SCR-A14 URL keys (DD-A14 step 5 names the period as a URL filter).
- **Open: evidence selection.** `mrv.saveDraft` takes 0–20 attachment IDs, but no read lists attachments outside a job's work report (`attachments.add` / `attachments.getContent` are report scoped), so the screen cannot offer a choice. Until a decision adds such a read (proposal: `attachments.list` filtered by unit set and period, returning ready attachments of accepted reports), a new version carries the previous version's evidence and a new report starts without evidence.

## IR204 Admin offset demo registry on the server pattern — 2026-10-09

`/admin/offsets` (FR-A15) follows IR176. The records tab reads `offsets.list` with customers and units; the `recordId` detail comes from the list item (stages, current attempt, purchase reference, the DEMO- certificate after retirement, the event history from the audit log). A new demo quote takes customer → units, period, purpose and amountKg (above 0, at most 100000, three decimals) and calls `offsets.preview`; the quote (valid 15 minutes, provider not selected, no price, the fixed market concept) is held by the view, since no read returns a quote, and `offsets.simulate request` with demoConfirmed turns it into a record. The next step offers purchase_confirm (or a simulated failure), then the retirement after an explicit acknowledgement; a failed record offers retry of the failed stage. Each event sends a fresh eventId and the record version. The market tab is read only: stage future_concept, provider not selected, verification unverified, ledger not connected, no prices or orders. The IR rule for offsets.list now names the `status` filter and the period (IR200).

## IR205 Admin restriction manager and exception screen on the server pattern — 2026-10-09

`/admin/restrictions` (FR-A09) and `/admin/restrictions/[id]` (FR-A10) follow IR176.

- **Manager.** `restrictions.list` for the URL scope (`contractId`, `invoiceId`, `status`) with per-state counts of the scoped result set; the `restrictionId` detail reads `restrictions.get`, the cause invoices with their paid state (`invoices.list` of the contract), the latest apply and remove command per unit (`commands.get`) and unit names. Execute is offered only for scheduled restrictions at or after executeAfter and 24 hours after the notice without an active grace or exception, after confirming the rules version; Request release only for requested/applied restrictions whose cause invoices are all paid or that have an active grace or exception (otherwise disabled with the reason, IR35); Retry apply / Retry release / Reconcile only for the units IR141 allows. Schedule (restriction.write) lists eligible RTO contracts (current term, restrictionEligible, rules version), fixes the cause invoices to the contract's overdue unpaid invoices, disables units under an active restriction or recovery case or without power control, keeps a temperature setpoint inside the chosen units' range, requires executeAfter at least 24 hours ahead and shows how many active clients receive the notice (none blocks, IR05).
- **Exception screen.** Four action cards (grace, exception, cancel, override) state the result for the current state before saving (IR96, IR35, IR141); defer and exempt need a future `until` within 90 days, every action a reason; override is offered only to restriction.override holders; units needing reconcile or a release retry are listed, for override-only callers only after an override release or with unresolved recovery cases; the audit timeline (`audit.list` of the restriction) appears only with audit.read.
- **Permissions.** Callers without restriction.read, restriction.write or restriction.override see a no-access state. Override-only callers get the release projection: no contract, invoice, customer or reason fields, no contract or invoice filters (IR03), and unit names only with asset.read.

## IR206 Admin access and roles on the server pattern — 2026-10-09

`/admin/settings/access` (FR-A03) follows IR176. A Server Component reads `members.list` (HQ, contractor and technician memberships; clients stay in Customers & units › Users) with the URL filters `role` and `status` (active = `activeOnly`, inactive = the complement of the result), `organizations.list` and the scope targets (customer organizations, properties, units). The editor shows the user, organization (kind per role and employment), role (fixed after creation, the choice only pre-checks defaults), employment for technicians, the role's scope rule (tenant for admins, the own organization for contractors, unit / property / customer-organization targets for technicians), the valid period and the 38-permission matrix (only the role's allowed permissions are enabled, Write keeps Read on, identity.write and restriction.override cannot be granted to oneself); every save needs a reason. Revoking is `members.save` with validUntil at the business clock, never a delete.

- There is no operation that lists or creates HQ, contractor or technician users (they come from the identity provider), so `membershipId=new` adds another membership for a user who already has one.
- `corePrincipal()` in the DAL reads the signed-in user and membership from session.get so the view can disable self-grants before the API refuses them (`errors.self_promotion`).

## IR207 Admin automation policies on the server pattern — 2026-10-09

`/admin/settings/automation` (FR-A11) follows IR176. A Server Component reads `policies.list` (kind automation) for the URL scope `customerId` → `propertyId` → `unitId`, grouped by the customer of the target units (“Across customers” when they span several), the `policyId` editor from `policies.get` (AT-A11-R01; `policyId=new` for a blank form), units with their model's temperature range, customers and properties. The editor sets name, priority 0–100, timezone, enabled (new policies start disabled), target units, a When sentence (occupancy, tariff in MYR/kWh, peak, solar or battery in kW) and a Then sentence (power, temperature within every target's range, mode, fan, ventilation), and explains the tier order; saving is `policies.save` with the version read. Simulate builds one fact per target unit for the condition's metric (IR16 units) at the current business-clock minute, with value or no reading and a quality, and shows per unit the selected rule or the suppression reason; Fire (demo) sends the same evaluation to `automations.fire` with the event ID as the one-time key, so a repeat returns the stored result and its command IDs, and a changed fact starts a new event.

## IR208 Admin customers and units on the server pattern (part 1) — 2026-10-09

`/admin/units` (FR-A02, SCR-A02) follows IR176. A Server Component reads the register: `customers.list` for both statuses (inactive rows only with Status: All, IR40), `organizations.list` (kind customer, the billing name), `properties.list` and `units.list` (archived records never appear, IR39), and, where the session may read them, `contracts.list`, `invoices.list` with `overdueOnly` and `restrictions.list` for the Contract badges (Overdue, the restriction state, Good standing, No contract). Lists are read page by page until `nextCursor` is null (`coreAll`). For `customerId` it adds the customer's `spaces.list` per property, `capabilities.list` (models), `policies.list` for the customer, open and acknowledged `alerts.list` (severity breakdown) and the last `audit.list` entry of the selected location; for `unitId` `units.get` (archived units open read only) with the bound device (`devices.list`), the attached policies and the D05 blockers that stop taking the unit out of use: an unexpired contract, a job that is not completed or cancelled, an active device binding, a requested / applied / release-requested restriction and pending commands, each linked to its screen. An opened unit selects its own location in the tree unless the URL names one.

- **Writes.** New customer = `organizations.save` (kind customer) then `customers.save`; an organization left without its customer by an earlier half-finished attempt is reused. Properties and spaces are added, renamed and deleted (`properties.archive` / `spaces.archive` with a reason, disabled while they still hold locations or units). The unit form saves name, location, model, installation date (Kuala Lumpur date, never in the future) and service scope; a relocation asks for a change reason in a modal. “Delete unit” is `units.archive` (the ID and history stay), “Remove permanently” is `units.delete` for a unit registered by mistake. Policies are attached and detached with `units.setAlertPolicies` (the customer's alert policies only), and the customer's default-rule switches are `policies.setDefaultRule` with the setting version (0 creates it).
- **Backend gaps closed.** (1) `units.save` records before/after values (property, space, name, model, installation date, scope) so a relocation keeps the original location in the history; `ops.Changes` compares times as instants and shows string values without JSON quotes. (2) `properties.save`, `spaces.save` and `units.save` apply the `locations.rename` sibling rule: a trimmed, case-insensitive duplicate among non-archived siblings is CONFLICT `error.duplicateSiblingName` (the `properties_name` / `spaces_name` unique indexes map to the same key); names are trimmed before they are stored. (3) `organizations.save` rejects a second organization of the same kind with the same trimmed, case-insensitive name (CONFLICT `error.duplicateName`: a second customer with the same billing name). (4) Alert active windows whose end is before the start run past midnight, the weekdays being start days, as IR120 item 3 states; the evaluator previously never matched such a window. (5) `heartbeat_gap` is measured in minutes (UnitSymbol `min`), so the IR120 default rule `ac_offline` is 15 with recovery 1 (15 minutes without a heartbeat, backend architecture §9), not 900 / 60.
- **Web platform.** `@ac/web/globals.css` declares `@source "."`: each app builds in its own directory and Tailwind skips `node_modules`, where the shared package is linked, so classes used only in shared components (the app shell grid among them) were never generated. The single-column `.split-rev` / `.split-even` rules now come before the container query that widens them, and a width class on `Input`, `Select` or `Textarea` replaces their `w-full`.
- **Next (part 2).** The Users tab (`clientUsers.*`, DD-A17), Warranty & coverage (`units.coverage`, `jobs.recordWarrantyClaim`, DD-A19), CSV import (`units.importPreview` / `importCommit` / `importUndo`, DD-A18) and the command panel and diagnostic runs of the unit edit.

## IR209 Admin customers and units on the server pattern (part 2) — 2026-10-09

`/admin/units` completes FR-A17–A19 on the IR176 pattern. The customer's **Users** tab reads `clientUsers.list` for the customer: invite (email unique per customer, owner or member), change role, disable or enable sign-in and remove from the customer each ask for a reason (IR144); resend invite and reset password only build previews (`clientUsers.resendInvite`, `auth.previewPasswordReset`); the last active owner offers no demote, disable or remove. The register's **Warranty & coverage** tab reads `units.coverage` with the KPI tiles, local customer / coverage / ends-within filters, Renewal offer and Open contract links to SCR-A07 and a client-side CSV export; “Warranty on jobs” lists the claimable jobs with the parts of their accepted report (`jobs.get`, `reports.get`) and files a claim with `jobs.recordWarrantyClaim`. **Import CSV** is the two-step wizard: the file is read in the browser (header, rows, automatic column mapping by name, template download), `units.importPreview` returns the row results (Download error report), `units.importCommit` imports ready and warning rows and the result offers undo with a reason (`units.importUndo`). The unit form gains the warranty end.

- **Contract.** `units.save` takes an optional `warrantyEndsAt` (omitted keeps the stored end, null clears it; not before `installedAt`, VALIDATION `error.range`); the end is recorded in the before/after audit values.
- **Import gaps closed.** (1) A property that does not exist yet is created at import (kind office, no address; `warning.importCreatesProperty`), as the import step 1 states and `createdPropertyIds` / `created_property_ids` foresaw — rows for an unknown property used to fail. (2) Floors and rooms match an existing location of any kind with the same name under the parent (the `spaces_name` unique index ignores the kind as well), so a commit no longer fails on the index. (3) `installed_on` and `warranty_end` are Kuala Lumpur calendar dates (a UTC reading moved them and rejected today's date before 08:00). (4) The serial column binds IoT: the preview rejects an unknown serial (`error.unknownSerial`), a bound device (`error.serialBound`), an unresolved tamper (`error.tamperUnresolved`) and a serial repeated in the file (`error.duplicateSerial`); commit binds the device to the created unit like `devices.bind` (CONFLICT `error.previewExpired` when it changed since the preview), and undo ends those bindings before archiving.
- **Claimable jobs.** No operation ever stored a claim in state `claimable`, so `claimableJobIds` was always empty. A job is now claimable when it is completed, its accepted work report lists parts, it has no filed claim and the unit's warranty end is not before its completion (the `jobs.recordWarrantyClaim` rule); `maintenance.claimableJobs` returns the candidates with their completion time and Assets applies the warranty end. Filing the claim ends the claimable state.
- **Demo data.** The fixture contract gives the seeded units the warranty ends of Figma 02 warranty (Living room AC 2026-10-28, Bedroom AC 2026-11-30, Lobby AC 2027-03-31, Meeting room AC 2027-06-30, Bedroom AC B ended 2026-08-31, Kuala Lumpur dates).
- **Open.** There is no read of past imports, so undo is offered right after the import in the wizard only. The unit edit's command panel and diagnostic runs links have no admin screen in Figma and stay out until one is designed.

## IR210 Customer contracts, payments and users on the server pattern — 2026-10-09

`/customer/payments`, `/customer/payments/[id]` and `/customer/users` (FR-C10–C12, FR-C19) follow IR176.

- **Contracts & payments.** A Server Component reads the customer's own `contracts.list`, the selected contract's invoices (`invoices.list` with the URL `status` filter: overdue = `overdueOnly`, all = no filter), the unit names and paths (`units.list`, `properties.list`, `spaces.list`) and, for a contract with `activeRestrictionIds`, the restriction notice through `restrictions.forInvoice` of one of its invoices (the customer has no restriction list). URL keys `contractId` and `status`; contracts show Active / Upcoming / Expired, and with no contract the page says “No contract — general maintenance”.
- **Invoice.** `invoices.get` with its payments, the restriction (`restrictions.forInvoice`, `commands.get` for each unit's latest command on the restriction tab), the inquiries (`inquiries.list` by invoice and by restriction) and the payment reminders the customer received (`notifications.list`, type payment_reminder). Tabs payment / restriction / inquiry with `restrictionId` and `inquiryId`. Paying is `payments.simulate` initiate with the chosen demo card and the invoice version, followed by the demo provider's processing event; confirm and decline are demo provider events in a separate DEMO box (DD-C11 outcome controls), so the invoice is paid only after a confirmation and a second payment intent during processing is CONFLICT. Payment instructions use the instructions event and create no payment. Email / WhatsApp previews call `notifications.preview` (payment template) and render the message from the invoice. The restriction tab lists the IR42 state events, the per-unit apply / release state with the last command and never shows Released before every unit confirms; inquiries are `inquiries.create` with subject payment or restriction.
- **Users.** `session.get` returns `clientRole` (owner / member for client sessions, null otherwise); the BFF session exposes it so the customer sidebar shows Users to owners only, and members opening the route get the Page unavailable view. Owners read `clientUsers.list`, invite members (`clientUsers.save` without id, clientRole member) and resend invites; resending for a user who is not invited is CONFLICT `errors.invite_not_pending` (IR144 item 5 replaces the VALIDATION wording of IR114 item 1).

## IR211 Customer units & locations and group control on the server pattern — 2026-10-09

`/customer/properties` (FR-C02, FR-C14) follows IR176. A Server Component reads the customer's `properties.list`, `spaces.list` and `units.list` into the read-only tree (IR109, counts of direct units per room, unassigned units “placed by HQ”) and, for the selected room or area (descendants included), the property's unassigned units or the KPI filter (`powerState`, `connections`), each unit's `units.get` for the setting, room temperature, power draw, capability and restriction. URL keys `propertyId`, `spaceId`, `unassignedOnly`, `mode`, `unitIds`, `powerState`, `connections`. Rename is `locations.rename` with the location version for properties and spaces.

- **Group control.** Offered to owners (`clientRole`, IR210) for the ACs directly in one room or area — a floor's child rooms are other spaces, so they never mix (DD-C14 mixed spaces). The plan per AC lists the commands that move its observed setting to the change: one `commands.create` per changed setting (power, setpoint, mode, fan; the fan may be kept), clamped to the model's range and an active temperature restriction (IR46); offline ACs, models without remote control, blocked control and power-off restrictions are skipped with the reason, and offline ACs can be selected so the review shows them as Skipped. Each AC's commands run in turn with `commands.get` polling through the BFF and stop at the first failure; ACs never roll each other back, and Retry failed ACs plans the failed ones again.
- **DD-C02 wording.** Duplicate sibling names are CONFLICT `error.duplicateSiblingName` (the backend rule of IR208), not VALIDATION.

## IR212 Customer energy & cost, monthly report and carbon offsets on the server pattern — 2026-10-09

`/customer/energy` and `/customer/energy/offsets` (FR-C06, FR-C13, FR-C16) follow IR176.

- **Energy & cost.** A Server Component reads the customer's `units.list`, `baselines.list` (IR90: only baselines whose units are all the customer's), `properties.list` and `preferences.get`, and for the URL selection `energy.summary` for the period with the chosen baseline, one `energy.summary` per Kuala Lumpur day for the daily chart and, when comparing units, one per unit (with a baseline covering exactly that unit) and per unit and day for periods of at most 7 days. Periods are Today, the last 7 or 30 Kuala Lumpur days up to the current minute of the business clock, or a custom range of at most 366 days; up to 4 units. Only baselines over exactly the selected units are offered (D07), “No baseline” keeps actual energy viewable, and the backend still decides comparability (same minutes and full coverage); a comparable baseline is drawn as an even daily share of its value, while every saving comes from the API (IR68 wording). The chart is replaced by a note when the period has no valid minute readings. URL keys `unitIds`, `period` (today, 7d, 30d, custom), `from`, `to`, `baselineId` (`none` for no baseline); the screen catalog gains `period`, `from` and `to` for SCR-C06.
- **Monthly report.** Export calls `energy.exportReport` (completed months, at least one section, the customer's own properties) and shows the demo file's name, size and format; changing “Email this report to me every month” calls `preferences.update` with the stored locale and timezone.
- **Offsets.** The page reads `energy.summary` for the selection carried from Energy & cost (`unitIds`, `period`, `from`, `to`, `baselineId`, added to SCR-C13) and the customer's `offsets.list`. A demo quote is `offsets.preview` (no record), kept in the view; Submit demo request is `offsets.simulate` request with the quote version and demoConfirmed, and a used or expired quote (CONFLICT `errors.quote_unavailable`) sends the customer back to step 1. The market concept shows the quote's `marketConcept` values; nothing real is bought, retired or tradable.
- **Open.** energy.summary has no hourly or peak-window breakdown, so the Figma peak-window note (share of energy in 17:00–21:00, highest draw) is not shown. The seeded demo data has almost no minute-slot power samples, so coverage is near 0 % until devices report.


## IR213 Customer air quality on the server pattern, latest readings and ventilation CO₂ — 2026-10-09

`/customer/air-quality` (FR-C07) follows IR176; the backend gains the latest readings the screen needs.

- **UnitSummary.latestMeasurements.** `units.list` and `units.get` return, per unit, the latest Measurement of each metric at or before now (observedAt desc, sequence desc, id asc; every origin and quality, null values kept), ordered by metric; `[]` without readings. Quality is read-time (D07): a valid reading older than its sensor's `staleAfterSeconds`, or whose sensor is unknown, is returned with `quality=stale`; `telemetry.series` history keeps the stored quality. The list is an equipment read (IR49(b)): an external technician's unit whose work window has not started is listed with `latestMeasurements=[]`. Screens show a stale or suspect value only with its quality and never fall back to an older reading.
- **co2AtLog.** `ventilation.log` records the most recent of the target units' latest CO₂ readings (the named unit, or every unit of the room) that is valid at logging time with a value; otherwise null. An older valid reading is never used (refines IR110).
- **Telemetry normalization.** `demo.trigger` telemetry applies the IR12 causes in the order unit_mismatch → non_finite → out_of_range → invalid_time: the first three store `value=null`, invalid_time keeps the value, a null input with a cause stays suspect and otherwise is missing; `rawUnit` keeps at most 32 characters.
- **Screen.** A Server Component reads `properties.list`, `spaces.list` and `units.list` for the room choices (each space with units directly in it, plus each property's units outside a room — readings only; Log ventilation needs a room), `units.get` of the selected unit (latest readings, capability sensors, fresh-air function = `ventilation` with level `low`), `telemetry.series` of the metric and period newest first — at most 1000 readings in 10 pages of 100 (D07), a cut-off series is labelled — with the IR98 allergen observation, and `ventilation.list` of the room (newest 10). Cards: Live (current valid), Unavailable (no sensor, no reading, stale), Unknown (null; humidity 0 is a measured 0 %), Suspect. IR99 guidance uses only valid CO₂/PM2.5; Log ventilation is always offered in a room and calls `ventilation.log` (no Command, nothing sent to HQ); VALIDATION keeps the modal and its input. The chart draws 5-minute averages of valid readings for 1h/24h and hourly averages for 7d; empty slots are shaded gaps, never joined; the table lists the same slots newest first. URL keys `spaceId`, `unitId`, `metric`, `period` (chart/table is local); SCR-C07 gains `period` and the operations `properties.list` and `spaces.list`.
- **Periods.** C07 offers 1h/24h/7d (Figma Client 05a), no custom range. `telemetry.series` keeps `to − from ≤ 35 days` (IR121 item 2, the raw hot window), so a custom series range (DD-T03) is at most 35 days, not the 366 days of IR41.
- **Open.** The development environment has no device simulator (IR45), so seeded readings turn stale a few minutes after reset; `demo.trigger` telemetry supplies current readings.

## IR214 Customer automations on the server pattern, schedule firing, consent withdrawal and delete — 2026-10-09

`/customer/automations` (FR-C04, FR-C05) follows IR176; the backend gains the parts the Figma screens (Client 03a–03i) and the acceptance criteria need.

- **Schedule firing (IR54).** Schedule occurrences fire from an equipment scheduler job (registry jobs run by the domain scheduler in each tenant's transaction, events and audits recorded as for a write). Every start / end occurrence that fell in (watermark, now] is evaluated at its minute with the D02 arbitration of the unit's rules, in time order (an end before a start of the same minute); the winner gets its Command (source automation, actor = rule owner re-authorized now) and an `automations.schedule` audit entry by `system`. `control.schedule_watermarks` (per tenant, locked for the tick) starts at the first tick, so occurrences before it are never replayed; a demo clock jump processes the occurrences it passed (at most 400 days). `control.automation_runs` keeps one outcome per rule, unit and occurrence (`trigger_ref` `schedule:<phase>:<minute>`, the same for `automations.fire`), so a repeated tick, a replayed window or an overlapping fire creates no second Command. Disabled rules, missing owner permissions, restrictions and offline units suppress as in D02; schedule ticks evaluate no alert-policy notifications.
- **Consent withdrawal (IR53).** `consents.update` with `granted=false` publishes `ConsentRevoked`; the equipment service sets `enabled=false`, `disabledReason=consent_revoked` and version + 1 on the membership's enabled location rules (consent and rules live in different domains, so the event replaces the same transition; evaluation still checks consent, so no Command runs in between). Granting again enables nothing; an explicit save with `enabled=true` does and clears the reason (IR27).
- **Delete.** New `automations.delete` `{id}` with the rule version (client:control.execute:self): the rule and its unit links are removed and it never runs again; its Commands and run log stay as history; other rules and manual control are untouched. Another customer's rule is NOT_FOUND, a stale version CONFLICT.
- **Draft preview.** `automations.nextRuns` also accepts `{draft:ScheduleDraft;count:8}` — the unsaved schedule of the editor, validated like `automations.save` (weekdays, equal times, overnight, actions, timezone; a nonexistent or ambiguous local time is VALIDATION) — and returns its next 8 occurrences with `automationId=null`. The editor's Test shows the next start and end ("would send 1 command") and the first following day that is not selected ("no match"); a saved, unchanged event rule is tested with `automations.simulate` at the current tick (the AC's saved rules arbitrate; nothing is stored or sent); a routine is tried with Demo controls.
- **Screen.** A Server Component reads `automations.list`, `automations.nextRuns` of each enabled schedule (next run), `units.list` with `properties.list` and `spaces.list` (AC names and places — a location event happens at the AC's property), `consents.get` (absent record: location rules unavailable) and `preferences.get` (timezone of new rules); the editor adds `units.get` of the ACs for their action options. Rules control one AC from this screen (extra target ACs of rules saved elsewhere are kept). Cards show On / Off / Disabled · reason with the IR27 stop note; the switch saves `enabled` with the version (location rules need consent); ⋯ › Delete confirms first. Save, switch, delete and consent are Server Actions; times are 24-hour HH:mm. URL keys `automationId` (an id or `new`) and `tab`; SCR-C04 gains `automations.delete`, `properties.list`, `spaces.list` and `preferences.get`.
- **Open.** Figma 03b/03e show “Only if … (optional)” extra conditions and 03a a “last evaluation skipped” note; the Automation contract has neither (no extra conditions, no persisted skip reason), so the screen shows neither yet — closed by IR215.

## IR215 Automation “Only if” conditions and the last-run note — 2026-10-09

Figma Client 03a/03b/03e show “Only if …” extra conditions and a “last evaluation skipped” note; the contract gains both (closes the IR214 Open item).

- **onlyIf.** `Automation.onlyIf: ExtraCondition[]` (`automations.save` input `onlyIf?`, default `[]`): at most 3, each type once — `weekday {weekdays}` (ISO days of the evaluation time in the rule's timezone), `occupancy {occupied}`, `weather {metric:'temperature', operator, value −50…100}`. A schedule cannot use `weekday` (its own weekdays decide) and an event rule cannot repeat its condition's type; otherwise VALIDATION `fieldErrors.onlyIf`. Evaluation (simulate, fire, the schedule job) checks them after the rule's own trigger, in order: a failed one is `no_match`, absent or null data `missing_data`, old data `stale` (D02, IR16 TTLs) — missing data never matches. Schedule ticks carry no facts, so a schedule with an occupancy or weather condition skips with `missing_data` until such facts exist (demo).
- **Run log.** Firing records, besides the winner's `command_created` row, a `skipped` row with the reason for each rule whose own trigger held (a due schedule, a matching condition) or whose own data arrived null or stale — not for `disabled`, not for events that are not the rule's (absent data for its condition) — in `control.automation_runs` (one row per rule, unit and trigger; a later Command for the same trigger replaces a skip). `automations.list` returns `lastRun: AutomationRun|null` — the latest row by occurrence time and recording order (`seq`).
- **Screen.** The editor's step 2 “Only if … (optional)” adds up to three conditions (Someone is home / No one is home, Outdoor temperature, Weekdays for event rules) and the summary names them; cards add “· Only if: …” to When and, when the last run was a skip, “Last evaluation skipped <time> — <reason>”.

## IR216 Command history and technician diagnostic control on the server pattern — 2026-10-09

`/technician/units/:id/control` (FR-T10) follows IR176, and the stored command history replaces the per-screen list of C03.

- **commands.list.** New read `commands.list` `{unitId, jobId?, query}` → `Page<Command>`, newest first (`requestedAt desc, id asc`; sort `id` or `requestedAt`; no filters): a client sees the commands of its organization's unit — the `reason` of a command another actor sent (technician, HQ) is withheld; a technician must pass `jobId` and sees that job's commands when it holds or held an Assignment of it (equipment's projection, IR187); HQ (`control.execute` or `job.read`) sees the unit's commands. Another customer's unit, or a job the technician never held, is NOT_FOUND. 199 operations.
- **Command.source.** Commands return `source` (`ui`, `voice`, `automation`, `restriction`, `diagnostic`, `group`) so histories label automation, restriction and test-run commands.
- **Technician screen.** A Server Component reads `units.get` (observed state, capabilities, restriction, pending commands, control availability), the job of the URL `jobId` (the unit page links its in-progress, else assigned, job) with `jobs.get` (assignment window, version), `diagnosticRuns.list` of the job (latest run) and `commands.list` of the job. A diagnostic command (`commands.create` with jobId and reason) and a test run (`diagnosticRuns.create`, 1–15 minutes, start and end actions, reason) are Server Actions, each confirmed first; the page refreshes every 4 seconds while a command or run is open. The run banner follows the DiagnosticRun state — Stopped only after the end acknowledgement (completed); end_requested, end_failed, start_failed and end_blocked say what the device did not confirm. Refusals read as Figma: FORBIDDEN (restriction minimum, window), CONFLICT (firmware update or unfinished command), OFFLINE, VALIDATION (duration, reason). Without `control.diagnose` the page explains it; without `jobId` it links back to the unit and its jobs.
- **Customer screen.** C03 reads `commands.list` of the unit and shows it under this screen's own sends, newest first, with “by an automation / by a restriction / technician test run” labels.

## IR217 Device operation lifecycle, device events and technician devices on the server pattern — 2026-10-09

`/technician/devices` (FR-T11) and `/technician/devices/:id` (FR-T12) follow IR176; the clock-driven part of IR67 and the device branch of DemoTrigger were not run by the Core API before.

- **Operation lifecycle (IR67, D05).** The equipment scheduler runs the device operations as a domain job. An operation still `queued` or `running` at `expiresAt` (60 s after creation) becomes `failed`/`TIMEOUT` with `finishedAt = expiresAt`; a timed-out check sets the device `error`. A `queued` operation starts at the first tick one second or more after its creation: the mutual exclusion is checked again — a Command `requested`/`sent` other than an undelivered restriction Command (`source = restriction`, `delivery = not_sent`), an active DiagnosticRun, or another open operation of the unit makes it `failed`/`CONFLICT` (REV19-018); a firmware update whose device is no longer `online` becomes `failed`/`OFFLINE` (IR94); otherwise it is `running` with `startedAt`, and a check sets the device `connecting`. Failures at the start change neither the device nor its version; the device version changes only with the device fields (connection, lastSeenAt, power signal, tamper, firmware). The unit's `connection` and `lastSeenAt` (units.get, units.list) mirror the bound device whenever the device connection changes.
- **demo.trigger operation.** `{eventType: 'operation', operationId, result}` applies to a `running` operation before its `expiresAt` only; a queued, finished or expired one is CONFLICT `errors.operation_not_running`, an unknown one NOT_FOUND. `finishedAt` is the event's `occurredAt`. A succeeded check sets the device `online` with `lastSeenAt = occurredAt`, a failed check `error`; a succeeded firmware update installs `targetVersion`; a failed result is `failureCode = UNAVAILABLE` and keeps the version and the connection.
- **demo.trigger device (SR20, SR23, REV18-040).** `kind` communication_lost / power_lost / tamper act on the axes connection / power / tamper with the evidence heartbeat / power_signal / tamper_signal; `restored` carries the DeviceRecovery of one axis (connection → online, power → on, tamper → clear; anything else VALIDATION `recovery`) and faults carry none. `bindingId` must be the device's current binding (null while unbound; otherwise VALIDATION `bindingId` `errors.binding_mismatch`). Each DeviceEvent stores the reported `eventId`: a repeat with the same content changes nothing, other content is CONFLICT `errors.event_id_reused`. `sequence` counts per device, binding and axis (unique there); a sequence not newer than the latest event of its axis is an out-of-order report that is accepted but leaves history and state unchanged, so an old heartbeat after a newer communication loss keeps the device offline (IR102 G1-023). communication_lost sets the connection `offline`, power_lost the power signal `off`, tamper the tamper state `detected` and links the unit's tamper Alert in `alertIds` in the same transition — the IR66 key (unit, type tamper, cause unknown, no policy) keeps an open or acknowledged Alert, otherwise a new `critical` Alert (evidence demo_observation) links the latest resolved one as `previousAlertId`; a device without a binding gets no Alert. `restored` must name the newest fault of its axis while it has no `restoredAt` (otherwise CONFLICT `errors.recovery_source_not_current`): the fault gets `restoredAt = occurredAt`, only that axis recovers (a connection recovery also sets `lastSeenAt`), and the event inherits the fault's evidence source and `alertIds` without resolving the Alerts. Notifications for device faults (implementation contracts, device.fault) are not generated yet.
- **Technician screens.** One loader serves both routes: `devices.list`, `units.list` and `jobs.list` of the technician (the job of each unit for the IR94 `jobId`: in progress, else assigned, else rework — a lookup, no job list is shown, so neither route has a job sort), the register candidates (assigned units without a device, read with `units.get`: place, capability sensors, firmware candidates), the selected device (`devices.get`) and its unit. T11 adds `devices.operations` and `devices.calibrations` (with `device.maintain` and a job); T12 adds `devices.events` and `alerts.get` for each `alertIds` entry. Register (serial trimmed and upper-cased, unit, sensor types from the capability, job), rebind (reason 1–1000), check, calibrate (now on the demo clock), firmware update, response note (DeviceEvent version) and alert acknowledgement (Alert version, SR23) are Server Actions; the page refreshes every 4 seconds while an operation is open. Only firmware candidates above the installed version are offered (“no newer candidate” otherwise; HQ campaigns handle other versions). T12 tiles show when the open fault of each axis was detected; event rows show the evidence, the alert count, the recovery source of a restored event and the response notes; the acknowledge links of the open Alerts sit beside “Add response note…”.

## IR218 Contractor quality review and payouts on the server pattern — 2026-10-09

`/partner/jobs/:id/review` (FR-P05) and `/partner/payouts` (FR-P10) follow IR176; they were the contractor app's last fixture screens.

- **Quality review reads.** jobs.get must return the detail projection of a delegated job: the review opens on the last `reportRefs` entry (the submitted version; after a decision the accepted or returned version stays there and a rework draft appears as `draftReportRef`); offer and history projections, or a job without a report, show "nothing submitted yet". reports.get gives the WorkReport; members.list (the company's technicians) names the author and the contributors of the version; alerts.get resolves `alertIds` (absent outside the access window); units.get labels the unit while the window is open; attachments.getContent returns the ready photos (at most 6 of ≤ 2 MB) rendered as data URLs. The reviewer is the signed-in user (session.get), shown as "you"; IR31 `reviewAvailability` disables both decisions with its reason (self_authored, not_current, not_submitted, permission_denied).
- **Evidence check (DD-P05 step 2).** Required inspection items recorded n / N, a reason for every item that is not OK, photos attached, readings recorded and the contributors of the version. Accept is enabled only when the first two hold; the API stays the authority (jobs.review).
- **Decisions.** jobs.review with reviewMode normal, the job version as expectedVersion and the report version: accept → completed (receipt banner; the linked alert stays open), return with a reason of 1–1000 characters → rework_requested (banner with the reason; the technician resumes as v+1). A stale job or report version is CONFLICT and the page shows the latest state; an empty reason is VALIDATION before the call.
- **Payouts reads.** payouts.list (the contractor's approved and paid statements; the URL keys period and status filter the list, statementId selects one, default the newest period), rateCards.list for the rate card version, jobs.get per line job (the history snapshot after the delegation gives the type and "customer details closed with the delegation"; the detail with units.get while the window is open) and jobs.list status submitted for the "Not included · next statement" rows (AT-P10-B; a lookup, no job list sort). KPIs: jobs paid (distinct charge lines), gross, deductions, net payable with the pay date or the paid date. The bank account has no DTO field and reads "set by HQ".
- **Ask HQ.** payouts.query on a line of the approved statement (topic, message 1–2000) through a Server Action with the statement version; a line with a question shows its state (open / answered / adjusted) and the questions list shows HQ's reply or adjustment. Download PDF renders the statement as a one-page PDF in the browser (demo).
- **Demo seed.** fixture-contract.json gains demoSeed.rateCards with rc-contractor-a v3 (periodic inspection 380.00, repair 450.00, emergency 520.00, rework deduction 120.00 MYR, effective 2026-06-01) and the seed inserts it. Statements need a closed month of accepted reports (payouts.generate), so a fresh demo database shows the payouts empty state.

## IR219 Test coverage: web unit tests and the uncovered backend branches — 2026-10-09

- **Web unit tests.** Vitest in `service/web/shared` (config as in the Next.js docs: React plugin, jsdom, `@ac/web` resolved to the package), `npm run test` at the workspace root; five suites (41 tests) for the pure mappers the Server Components and client views share — device tiles, firmware card, operation and event rows (techDevices), command history and run banners (techControl), the quality-review evidence check, versions and alerts (partnerReview), payout statements, KPIs, lines, questions and the PDF (partnerPayouts), and the Server Action failure text (actionMessage). The shared package gets its own `tsconfig.json`, so `npm run typecheck` now covers it and its tests.
- **Backend coverage.** `make cover`: 84.7 % → 85.2 % of 14,278 statements. Unit tests beside the code: automation evaluation (`compare`, `conditionMetric`, `match` for peak / tariff / solar / battery / location / pattern / weather with stale and missing facts, `extrasMatch` with Sunday as ISO weekday 7 and unknown time zones), the scheduler result counter and the event consumer without handlers / with a failing poll (logged, loop continues). Integration tests: automations.simulate input rules (ids, tick, unit list, every fact shape); automations.fire idempotency (a repeated eventId returns the stored result, the eventId with other input is CONFLICT `errors.event_reused`, another eventId with the same input in the same tick reuses the result, other input is `errors.tick_already_evaluated`, a past tick is VALIDATION); the IR94 reasons of a technician history read without jobId (`errors.assignment_required`, `errors.assignment_not_started`, `errors.assignment_ended`, NOT_FOUND for an external technician without a delegation); commands.create input rules, the technician's reason and job checks and `pendingCommands` of units.get; the input rules of the restriction actions.
- **Remaining gaps.** The uncovered statements are mostly database error returns; the weakest packages are maintenance (86.7 % per function) and restrictions (87.0 %). Next: restriction recovery paths (`recoveryEnded`, `reconcileTerminal` refusals) and the decide reasons owner_forbidden / consent_revoked / invalid_capability / restricted.

## IR220 API description built from handler annotations (Swagger) and further coverage tests — 2026-10-09

- **Swagger from the code.** Every registered operation handler carries swag annotations (`@Summary`, `@ID`, `@Description` with the authorization, validation, recovery and design IDs of the operation catalog plus the query and input-version rules, `@Tags`, the `Idempotency-Key` and `X-Expected-Version` headers of the write-version catalog, the body input type, `@Success 200 ops.Envelope{data=<result>}`, the ServiceError failures 401/403/404/409/422/429/503/504, `@Security BearerAuth`, `@Router /v1/ops/<operation> [post]`); the general API information sits on `cmd/gateway/main.go`. `make swagger` (service/api) checks that no registered operation lacks an annotation (`scripts/swag_annotate.py --check`), formats the annotations (`swag fmt`) and builds Swagger 2.0 JSON with swag into `internal/gateway/swagger/swagger.json` and the copy `docs/02-design/api/swagger.json`; the document is never edited by hand. `scripts/swag_annotate.py` (without `--check`) refreshes the annotation blocks from the catalogs and writes `swagger_pages.go` per package: a named page type for each list result, because swag cannot instantiate `paging.Page` with a type of the annotated package (each page converts to `paging.Page[T]`, so the build fails if the page shape changes).
- **Serving.** The gateway embeds the document: `GET /swagger.json` and Swagger UI at `GET /docs` (locally http://localhost:8080/docs). A gateway test checks that all 199 catalog operations are documented with their write headers and responses.
- **Code changes for the annotations.** Handlers registered as closures, generic instantiations or factories became named functions (`session.get`, `preferences.get`, `twoFactor.get`, `units.resolveQr`, `demoSession.*`, `properties.archive`, `spaces.archive`); `paging.Sort` is now `paging.SortSpec` (swag confused it with the `sort` package); monitoring `Policy` names its wire fields in json tags (its MarshalJSON is unchanged); `AirSeries` spells out the page fields instead of embedding the generic page; raw JSON fields carry `swaggertype:"object"`; `.swaggo` maps `uuid.UUID` to string.
- **Coverage tests.** automations.simulate skip reasons decided after the own trigger (invalid_capability, owner_forbidden, consent_revoked); restriction recovery refusals (device offline, observation older than 30 s, another restriction observed, remove command blocked by a device operation), a failed remove command returning the case to pending, and the restricted / reconciliation_required skip reasons.

## IR221 Technician job workspace on the server pattern and per-operation request body limits — 2026-10-09

`/technician/jobs/:id` (FR-T04–T06, T08, T09, T13–T15) follows IR176; it was the technician app's last fixture screen.

- **Reads.** jobs.get must return the detail projection (offer and history projections, or a refused read, show not found); units.get while the window allows it gives the name, place, model and service scope, and the scope decides the checklist components (a refused read shows its reason without unit data); reports.get reads the open draft (`draftReportRef`) or the last submitted version; parts.list feeds the add-part dialog while the job is in progress. Photos and the signature are streamed by the route `/technician/jobs/:id/photos/:attachmentId?reportId=&reportVersion=` (attachments.getContent; FORBIDDEN → 403, NOT_FOUND → 404), so the page payload carries no files.
- **Assignment and check-in.** Accept or “can’t make this time” (reason, optional other slot) is jobs.acknowledgeAssignment with the job version. Start job opens Check in: location ≤ 200 m with the scanned label, resolved by units.resolveQr (its unit must be the job’s unit), or manual with a reason of 1–1000 characters; jobs.checkIn starts the work (IR111). Before the window the job is read-only with Start disabled (IR76); after the window the workspace shows the ended state (IR89).
- **Draft.** The checklist (tabs indoor / outdoor / electrical of the service scope; the initial result is empty; attention, not inspected and not applicable need a reason), readings (metric, value, the metric’s unit, component, observed at; an unmeasured value stays empty, never 0), parts (catalog or custom, source, quantity 1–999, lot / serial), refrigerant (cylinder, recovered and charged kg, leak check), the work text (10–4000) and the next action (none, or a follow-up with its reason) stay on the page until saved. jobs.saveDraft creates the draft on the first save (no expected version) and carries the draft version afterwards; autosave runs 20 s after the last edit while the window is open. Saves, photos and the sign-off return the new report state instead of reloading, so unsaved edits survive; the job transitions reload the page.
- **Submit.** The client check (every component recorded, reasons, numeric readings, work text, the follow-up reason, part quantities, ready photos) runs on the first submit attempt and is re-evaluated as the draft changes, so fixed rows and counts clear at once. jobs.submit carries the job version and the draft version; unsaved changes are saved first. The API stays the authority: VALIDATION, CONFLICT (on hold, submitted, or a version changed elsewhere), FORBIDDEN (outside the window or assignment) and UNAVAILABLE are shown as refusals and the draft is kept.
- **Photos and sign-off.** Up to 10 JPEG / PNG photos of at most 5 MiB are checked in the browser before the upload (HEIC or larger files are refused with the reason; saved text and photos stay). attachments.add moves the draft version on and clears the sign-off. reports.signOff binds the customer’s signature (a PNG drawn on the device) or an absence reason with a site photo to the draft version. The Versions card shows the open draft as the page holds it (version, recorded results, ready photos, the time of the last save on this page) above the submitted versions with their reviews.
- **Request body limits.** The dispatcher read every request body with a 1 MiB limit, which refused photos (base64 adds a third to the file). The limit is now per operation: attachments.add and jobs.reportProblem 8 MiB, reports.signOff 16 MiB (signature and site photo), every other operation 1 MiB; a larger body is VALIDATION `error.bodyTooLarge`. The technician app raises the Server Action body size to 12 MB (`experimental.serverActions.bodySizeLimit`, Next.js docs). Production uploads go to S3 (IR117) and are unaffected.
- **Tests.** Vitest for the workspace mappers (components per service scope, draft ↔ jobs.saveDraft input, submit issues, progress, IR89 window states, time on site, version rows) and a dispatcher test for the body limits; the whole flow was driven through the UI: check in → refused submit (19 issues) → checklist → reading → part and refrigerant → save → HEIC refused → two PNG photos (one 1.5 MiB) → sign-off → submit v3.

## IR222 REST routes for every operation of the Core API — 2026-10-09

Decision of the product owner after reviewing the Swagger description (every operation was `POST /v1/ops/<operation>`): the Core API uses proper HTTP methods and resource URLs. The operation stays the unit of the contract — its name, input and result are unchanged and remain the ID in authorization, audit, idempotency and `meta.operation`; the routes are a second, catalogued addressing of the same operations.

- **Catalog.** `operation-catalog.csv` gains `rest_routes`: one or more `METHOD /v1/path` entries joined by `;`, path parameters `{field}` named after the input contract fields they fill (`{target.kind}` fills a nested field), optionally ` [field=value]` for a fixed input field. 199 operations, 219 routes: reads `GET /v1/<resource>[/{id}]`, creates `POST /v1/<resource>`, saves with an `id` branch (16) `POST` on the collection and `PUT` on the item, deletes `DELETE /v1/<resource>/{id}` (input in the path and the query string), business commands `POST /v1/<resource>/{id}/<verb>`; `payouts.transition` is `/approve` and `/mark-paid`, `firmwareCampaigns.control` `/pause`, `/resume`, `/abort` and `/devices/{deviceId}/retry`. Reads whose input is a document, personal data or free text use POST (`automations.simulate`, `automations.nextRuns`, `notifications.preview`, `units.importPreview`, `voice.resolveIntent`, `auth.previewPasswordReset`). Reports and attachments live under their job (`/v1/jobs/{jobId}/reports/{reportId}`), `restrictions.forInvoice` under its invoice. The validator checks the syntax, unique method and path shapes, GET only for reads and DELETE only for writes, path parameters and fixed fields inside the input contract, and both routes of every save.
- **Query catalog.** `restrictions.forInvoice` no longer lists the filter `invoiceId`: the invoice is the path parameter.
- **Binding (service/api/internal/ops/rest.go).** A route builds the JSON input the operation reads and runs the same pipeline as `POST /v1/ops` (decode with unknown fields rejected, validation, authorization, Idempotency-Key, X-Expected-Version, transaction, ServiceResult): path parameters first; then GET and DELETE read the query string — top-level fields by name, nested fields as `object.field`, lists as repeated or comma-separated values with an empty value as the empty list, `cursor`, `limit`, `sort=field:direction` (ascending without a direction) and the operation's catalogued filters by their own names, typed by `Query.filters` of service-contracts.ts (`FilterKinds`, generated); POST, PUT and PATCH read the JSON object body and accept no query parameters; then the fixed field. Unknown or misplaced parameters are VALIDATION `error.notAllowed`, unreadable values `error.invalid`, a body that repeats a path parameter or the fixed field with another value `error.pathMismatch`; numbers keep their text. At start-up `NewBinding` rejects a path parameter that is not a scalar input field, GET on a write, DELETE on a read, and a query-string route whose input has lists of objects, maps or raw JSON.
- **Serving.** Each domain service mounts the routes of its operations on `/v1`; the gateway registers all 219 routes and forwards them unchanged to the owning service. `POST /v1/ops/<operation>` stays until the web apps' data access layer uses the routes (next round), then it is removed.
- **API description.** Each handler's annotations describe its first route (path parameters, query parameters with their roles — field, cursor, limit, sort with the catalog's sort fields, filter with its mapping — the body, the write headers per route: a save's POST omits X-Expected-Version, its PUT requires it); the further routes are annotated on generated stubs (`swagger_routes.go`, operation IDs `<operation>.update` and `<operation>.<fixed value>`). `cmd/gen/restdoc` prints the mounted routes and parameters for `scripts/swag_annotate.py`, whose `--check` in `make swagger` fails on a missing or stale block or stub. Swagger: 181 paths, 219 operations, no `/v1/ops` path.
- **Tests.** Binding unit tests (parameter discovery, typing, empty lists, errors, path and body merge, fixed fields, rules) and an end-to-end route test through the registry (create, update, delete, verbs, replay, unknown routes); every route resolves to its operation in the assembled router and is forwarded by the gateway to its domain; the gateway's document test checks every route. The integration suite (111 tests, also on the split services) now calls the Core API through the routes: a client helper picks the route the input fills, puts the remaining input in the query string or the body. REST changed three expectations: a read without the id in the path has no route (NOT_FOUND `error.unknownOperation`), an unknown filter is the parameter `error.notAllowed`, and empty lists travel as an empty parameter.

## IR223 The web apps call the REST routes; POST /v1/ops retired — 2026-10-09

- **Route table for the web.** `make gen` (service/api/cmd/gen/ops) writes `service/web/shared/lib/routes.gen.ts` from `rest_routes` next to `catalog_gen.go`; a test regenerates both from the design documents and fails when either is stale.
- **Requests.** `@ac/web/lib/rest.ts` `coreRequest(operation, input)` turns an operation call into the request of one of its routes by the rules of the Core API's bindings (IR222): the route whose path parameters the input holds (an item route before a collection route, so a save with an `id` is `PUT /…/{id}` and without one `POST /…`) and whose fixed field it matches; path parameters taken out of the input and percent-encoded; GET and DELETE with the rest as the query string (the paging query and the filters by name, `sort=field:direction`, `object.field`, lists repeated, an empty list as one empty value, null left out); POST, PUT and PATCH with the rest as the JSON body. The DAL (`coreOp`, Server Components and Server Actions), the BFF relay (`POST /bff/ops/<operation>`: the browser keeps calling operations by name; a body that is not JSON is VALIDATION `error.malformedInput`, an operation outside the catalog NOT_FOUND `error.unknownOperation`) and `/bff/session` (`GET /v1/session`) use it. Vitest covers the mapping; the Go counterpart `ops.ClientRequest` drives the Core API tests.
- **Retired.** `POST /v1/ops/<operation>` is no longer routed by the services or the gateway (NOT_FOUND `error.unknownOperation`); the Core API tests call the routes.
- **IDs in paths.** An ID path parameter that is not a UUID names no resource: NOT_FOUND `error.notFound` before the input is decoded (it was VALIDATION `error.malformedInput`), so a page opened with a malformed ID shows "This page isn’t available" instead of failing. Every ServiceError carries a correlation ID, also for unknown routes.
- **Checks.** A signed-in crawl of each app (every own link from the home page, navigations only) reports document status, page and console errors, BFF 5xx and error or not-available text: customer 15, partner 14, technician 6, admin 15 pages. It found two pages that failed in API mode — the customer unit page linked a fixture invoice (`/customer/payments/invoice-overdue-a`; now `/customer/payments`, whose restriction notice links the real invoice) and the HQ restriction detail failed for a caller without restriction permissions (now the list's "No access" state; an unknown restriction is not found). Writes through the routes were driven through the UI (technician workspace: check-in, draft, two photos, sign-off, submit) and the BFF relay (contractor review accept; customer automation create POST, update PUT, stale DELETE CONFLICT, DELETE).
- **History pages.** commands.list and diagnosticRuns.list bound their cursor to the whole input, the cursor itself included, so a second page was always VALIDATION `error.queryChanged`; the cursor now binds the unit and the job (tests read page two). The split-service suite found it once the shared test database held more than one page of a unit's commands.
- **Open: fixture content on browser-read screens.** The crawl and a source scan show that screens reading through the BFF still mix fixture rows into API mode: the partner overview (KPIs, actions, today, capacity, activity), jobs, job detail (capacity rows), schedule and history, the technician overview and job list, HQ jobs (plans, default selection), customer maintenance, and the Scan QR dialog. Their fixture links (`/partner/jobs/job-p05/review`, `/technician/jobs/job-contractor-a`, `/technician/jobs/job-t07`) end on "This page isn’t available". These screens move to the server pattern with live reads next.

## IR224 Contractor overview on the server pattern; offer decline events — 2026-10-09

`/partner` (FR-P01, DD-P01, SCR-P01) follows IR176; it was the first of the browser-read screens that mixed fixture rows into API mode (IR223 list).

- **Period.** URL `from` / `to` (Kuala Lumpur dates, at most 366 days) or this week (Monday–Sunday) by default; the select offers last, this and next week. One period applies to every section, as on the job list: `summaries.get` (kind partner) and `jobs.list` filter on the requested slot `[from, to)`.
- **KPI tiles.** Offers to answer (offerCount; the soonest expiry), Awaiting assignment (the list's accepted jobs without an assignment), In progress / scheduled (activeCount without those), Reports to review (reviewCount; one report opens its review, several the filtered list), Overdue (overdueCount). Each tile opens the job list or the schedule.
- **Progress and actions.** Jobs per bucket — offered, accepted · unassigned, assigned / in progress, overdue (assigned or started with the work window ended, IR89), submitted · in review, completed — with the total and the completed share. Needs your action lists offers (Respond → the offer), ended work windows (Reassign → schedule), submitted reports (Review) and accepted jobs without a technician (Assign → schedule), the soonest offer first.
- **Today and capacity.** `members.capacity` of today draws each technician's assigned slots on 08:00–18:00 with a red line at now (unavailability shown instead of “Free all day”); the period's capacity sums each day's assigned and available minutes (undefined availability shows “—”). A banner counts the accepted jobs without a technician.
- **Names and activity.** `members.list` names technicians and actors, `units.list` names the delegated units inside their access window (an offer shows the site address), `jobs.get` gives an assigned job's technician and scheduled slot, and the latest `jobs.events` of the recent jobs (open jobs first) fill Recent activity with links to the job history. These reads are optional: a refused one leaves its section empty; the summary and the list are required (error boundary with retry otherwise).
- **States.** Loading and error boundaries per segment (route group `(overview)`); the empty period keeps the layout with zero counts, “Nothing needs your action” and “No job activity in this period” (Figma 01-2). The Phase 1A demo keeps the fixture dashboard.
- **Offer decline event.** jobs.decline recorded the job event `offer.declineed` (and the outbox type `OfferDeclineed`), so the company's decline record was missing from its history snapshot, which reads `offer.declined`; the past tense is now explicit (`offer.accepted` / `offer.declined`, `OfferAccepted` / `OfferDeclined`) and a test checks the decline event.
- **Observed, open.** A completed job keeps its Assignment active until its scheduled end, so the seed's week-long demo assignment books tech-external-a for the whole week (capacity 100 %, other assignments that week are `errors.assignment_overlap`); whether completion should release the Assignment is open. Same-day work cannot be requested (the slot rules reject preferred slots that start within the lead time), so today's timeline shows only the seeded assignment in a fresh demo.

## IR225 Contractor job list on the server pattern; the summary names the technician — 2026-10-09

`/partner/jobs` (FR-P01, DD-P01 job list, SCR-P09) follows IR176; it mixed fixture rows into API mode (IR223 list).

- **JobSummary.technicianMembershipId.** The summary projection of `jobs.list` carries the technician of the job's active Assignment (null without one), next to `assignmentAcknowledgement`, so a list shows who works on each job without a `jobs.get` per row. The contractor overview reads it too and no longer calls `jobs.get` (SCR-P01 operations drop it).
- **Tabs and counts.** All, Offered (offered), Active (accepted, assigned, in progress, on hold, rework), Review (submitted), Completed (completed); each tab's count is the total of its own `jobs.list` (limit 1) within the period, so the counts match the rows. The KPI tiles of the overview open the list with `tab`, `from` and `to` (the old `status` keys still map to tabs).
- **Sort, period and paging.** Sort status (default, ascending), deadline or unit urgency (IR34, IR30); the overview's period (this week by default, last / this / next week); 25 rows per page with the cursor and the page number in the URL; a cursor that no longer fits the list (changed conditions or snapshot) restarts at page 1 with a notice.
- **Rows.** Job · unit (an offer shows the site address) with the origin badge; the detail line per status (offered with the fixed visit slot or a pending time change; accepted, assigned, in progress, overdue when the work window has ended (IR89), on hold, rework, submitted, completed, cancelled; after the delegation only the history); the technician with the acknowledgement (awaiting, accepted, can’t make it); the slot or deadline (answer-by with the time left, due date, ended window); the status badge. Rows open the offer, the schedule (accepted without a technician), the job detail, the quality review (submitted) or the job history (history projection).
- **States.** Loading and error boundaries per segment (route group `(list)`); empty company (“No jobs yet”) and empty tab. The Phase 1A demo keeps the fixture list.
- **Test runs.** The suites leave their rows in the test database; after many rounds `ac_test` held about 2,000 jobs and 2,800 units, `make test` took 12 minutes and `make test-cluster` 20, and tests that read only the first page of a list failed. Each suite now rebuilds its own test database first (`ac_test`, `ac_test_cluster`, selected by `AC_TEST_DB`), so both take about a minute, and `make test-all` runs them at the same time (the databases are rebuilt one after the other because their migrations update the cluster-wide login roles).

## IR226 Contractor offer and job detail on the server pattern; offer projection fields; decision inputs per operation — 2026-10-09

`/partner/jobs/:id` (FR-P02, FR-P08, DD-P02, SCR-P02) follows IR176; in API mode it mixed fixture offers into the page (IR223 list).

- **Offer projection.** `JobOfferSummary` carries `offeredAt`, `accessValidFrom` and `accessValidUntil`, so the offer states when HQ offered it and the access period the company gets if it accepts (Figma 02-7). Its `partnerSlotProposal` was always null: the projection now reads the offer's latest partner proposal that is not withdrawn (pending, sent to the client, approved, declined or kept), so a pending proposal disables Accept and shows Withdraw after a reload (test in the partner proposal scenario).
- **Decision inputs.** `jobs.accept` and `jobs.decline` shared one input with both `termsVersion` and `reason` and required exactly one of them, reported as `termsVersion: error.required` even when it was sent. Each operation now takes only its contract input — accept `{jobId, offerId, termsVersion}`, decline `{jobId, offerId, reason}` — so the other decision's field is `error.malformedInput` like any unknown field, a missing reason is `reason: error.length` and a missing terms version `termsVersion: error.required`; Swagger shows the two request bodies separately (tests).
- **Views.** Offer (before the delegation period), detail (inside it) and history (after it) as DD-P02 describes; another company's job, a declined offer and an unknown ID are not found. Reads: `jobs.get`; `members.list` with `members.capacity` of the visit day and the next four days (who can take it); `jobs.events` (status timeline, the offer's ordinal); inside the delegation period `units.get` and `alerts.get` of the linked alerts. The required qualifications of a delegated job follow the unit's maintenance scope (indoor, outdoor, electrical).
- **Actions.** Accept, decline with a reason (confirmation; the page then says it was declined and links back, since the job is no longer the company's), propose another time (date and times in Kuala Lumpur, an own technician qualified at that time, reason; IR113 item 2 slot rules) and withdraw it — Server Actions with the job version as the expected version; CONFLICT (expired, cancelled, changed elsewhere, proposal pending) refreshes the page and states why; field errors are named.
- **Page width.** `Page` takes `narrow` for the 640 px single column of offers and snapshots (Figma), because a `max-w-*` class next to the default `max-w-[1280px]` won or lost by stylesheet order.
- **Observed, open (IR224).** The seed's completed job keeps its week-long Assignment active, so tech-external-a shows 0 h free and “Busy at the visit” for every new offer that week.

## IR227 Contractor schedule & assignments on the server pattern; job summaries carry the access window — 2026-10-09

`/partner/schedule` (FR-P03, DD-P03, SCR-P03) follows IR176; in API mode it showed fixture jobs and a fixed team grid (IR223 list).

- **JobSummary.accessValidFrom / accessValidUntil.** The summary carries the access window of the job's accepted Offer (null for internal jobs and before an acceptance), so the delegation windows of a list need no `jobs.get` per row (contract, Swagger, test). The offer projection already carries its window (IR226).
- **Form per state (IR113, IR123 item 4).** Accepted: the agreed visit time, locked. Assigned: the same slot with another technician; the reason is optional. In progress, on hold, rework: the start is kept, the end can only move later and the reason is required; the previous technician loses action access. An accepted offer before its delegation period and an assigned job whose agreed time has passed cannot be booked on this page — the second needs a new time agreed with the client through HQ (the contractor cannot pick one). Figma row 03 showed editable work start and end from before IR113; it now shows the locked visit time (03-1…03-5), the kept start (03-7, 03-8), 03-4 as the delegation-period refusal (FORBIDDEN `errors.access_window` — a locked visit time inside the access window cannot fall outside it) and 03-9 as the extension of an in-progress job whose window ended.
- **Candidates.** `members.eligible` for the slot gives who can be booked; the other technicians of the company are listed with the reason from `members.list` and `members.capacity` (missing qualification, unavailable, busy at this time) and every row shows the free time of the slot's day. Saving rechecks everything (`jobs.assign`), and the refusals are named.
- **Week.** Monday–Sunday of the slot: `members.capacity` per day and `jobs.list` per technician (filter `membershipId`) to name the job of each booked block; free, leave and no-hours days; the proposed slot of the chosen technician.
- **States.** Loading and error boundaries for the segment; “No accepted jobs” with a link to the offers; a job no longer delegated reads as gone. The receipt stays after the refresh (the form is keyed by the job, not its version). The Phase 1A demo keeps the fixture page.
- **Not exercised live.** The extension path needs a job in progress, which needs the demo clock inside its work window; it is covered by the mapper tests and the jobs.assign integration tests, not by a browser run.

## IR228 Contractor job history on the server pattern; previews take a note-length message — 2026-10-09

`/partner/history` (FR-P07, FR-P08, DD-P07, SCR-P07) follows IR176; in API mode it listed the jobs through the BFF and showed fixture previews (IR223 list).

- **Pages.** `/partner/history` lists the company's jobs after acceptance with their events (tabs, counts, latest event, technician, status); `?jobId=` opens one job's timeline and the communication form, as DD-P07 describes. Reads: `jobs.list` (statuses after acceptance; the 90-day period uses the requested slot, histories their completion — a job outside it is looked up without the period), `jobs.events` per job, `units.list` and `members.list` for names, `notifications.recipients` of the open job.
- **Customer-visible.** The filter follows IR42: the customer reads every job event except internal notes, so only internal notes are “internal”. Figma 05-2…05-6 marked report and work events internal; they now follow IR42.
- **Notes and previews.** “Save note & create preview” is `jobs.addNote` (job version as the expected version) followed by `notifications.preview` for the chosen recipient (nothing is stored or sent; the previews of the session are listed). Completed and cancelled jobs take no notes (IR123 item 2), so the form offers only the preview there; after the delegation the snapshot shows only the company's own decisions (IR124 item 5) and the form is closed. Figma showed a note saved on a completed job whose delegation had ended; 05-2…05-6 now show job-p03 inside its delegation with the preview only, and the list's ended example is job-p04.
- **Preview message.** `notifications.preview` limited `message` to 1–1000 characters while the job note it carries allows 1–2000 (DD-P07); the message now takes 1–2000 and the reason stays 1–1000 (test).
- **Names.** Timeline actors are “you”, a member of the company or the system; HQ and customer users are not named to the contractor.
- **States.** Loading and error boundaries; empty list and empty tab; a job that is not the company's reads as absent; a refused note keeps the message, a refused preview after a saved note says so. The Phase 1A demo keeps the fixture page.

## IR229 Technician overview on the server pattern; uploaded files survive restarts — 2026-10-09

`/technician` (FR-T01, DD-T01, SCR-T01) follows IR176; in API mode it showed fixture jobs, tiles, alerts and reports (IR223 list).

- **Overview.** As DD-T01 describes: the tiles of `summaries.get`, the new assignments to accept, Today / All assigned with the sort, the job rows with what can be done now, today's timeline, the alerts on the assigned units and the reports by state; `units.list` names the units. A row opens the job workspace (IR220).
- **Sidebar.** Sidebar › Assigned jobs links to `/technician?tab=all` and is now shown as active there (Figma 01-2): an item whose link carries a query is active when the path and the query match; the sidebar reads the search parameters inside a Suspense boundary so static pages keep rendering.
- **Uploaded files.** The API containers run read-only with `/tmp` on tmpfs, and the maintenance service kept its local blob store there, so every container restart lost the report photos while their rows stayed; the photos then failed with UNAVAILABLE (and the contractor's quality review, which treats UNAVAILABLE as retryable, failed as a whole). The store now lives on the named volume `blobs` (`BLOB_DIR=/var/lib/ac/blobs`, the directory created in the image for the non-root user), and a stored row whose content is missing reads as NOT_FOUND (`blob.ErrNotFound`, test) — the workspace shows “Photo unavailable” and the review leaves the photo out. Photos lost before this change stay unavailable.
- **Open.** Staging and production need the S3 store (bucket `ac-files`; container design §4, OPEN-CT-02); the Go blob package has only the local directory store.

## IR230 Technician unit QR scan from the Core API — 2026-10-09

The Scan QR dialog in the shell header (FR-T13, DD-T13, Figma Technician 01-6) showed a fixed match in API mode (IR223 list); it now scans through the BFF as DD-T13 describes.

- **Reads.** `units.resolveQr` (unit and the user's next open job on it — the soonest job of the unit in assigned, in progress, on hold or rework whose window has not ended; NOT_FOUND for unknown labels and units outside the user's assignments), then `units.get` and `jobs.get` for the matched card, and `units.list` for the one-tap scans and the unit name.
- **Before the work window.** `units.get` answers FORBIDDEN `errors.assignment_not_started` until the work window opens (IR94); the card then names the unit from the user's unit list and says its details open with the window, and Open job still leads to the read-only workspace.
- **Demo.** The camera is simulated: a field takes the label code (`ac-unit:<unitId>`), a device serial bound to the unit or the unit ID, and each of the user's units can be scanned with one tap. The Phase 1A demo keeps its fixture dialog.

## IR231 HQ maintenance jobs (Jobs tab) on the server pattern; labelled choice groups — 2026-10-09

`/admin/jobs` (FR-A06, DD-A06, SCR-A06) read only the in-browser job store in both modes (IR223 list). The Jobs tab now follows IR176 as DD-A06 describes (scope, stage counts, filters, list, detail, booking, proposals, partner time changes, hold / resume / cancel, follow-up classification); Server Actions carry the job version.

- **Availability.** The preferred-times table shows, per time, the HQ technicians `members.eligible` returns (qualified, in scope, free, not on leave); a time without one can still be offered to a contractor, who confirms its own capacity. Times in the past cannot be booked.
- **Offers.** The booking dialog defaults the offer to IR141 item 2 (answer within 24 h but not after the visit starts, access from now until a day after the visit) with an editable terms version (`terms-demo-v1`).
- **Proposals.** Reply within 24, 48 or 72 hours, never after the proposed time; a contractor's time change sent to the client gets 48 hours (or until the time).
- **Accessibility.** `Field` wrapped every control in a `<label>`; around a `Choice` (a set of buttons) the label named the first button instead of the group. A `Choice` child now renders a labelled `role="group"`.
- **Still to connect.** Report review (`reports.get`, `jobs.review`), costs (`jobs.saveCost`), New job (`jobs.create`), access extension (`jobs.extendAccess`), and the Plans, Contractors and SLA tabs, which show illustrative data in API mode until they are connected.

## IR232 HQ job report review, costs, New job and access extension from the Core API — 2026-10-09

The Jobs tab detail of `/admin/jobs` (FR-A06, DD-A06 items 6 and 8, SCR-A06) now connects what IR231 left to connect. Every write carries the job version as the expected version; CONFLICT reloads the detail.

- **Report.** For a job with report refs the detail reads `reports.get` of the latest ref and up to four ready photos (`attachments.getContent`, ≤ 2 MB each). The card shows the version, who submitted it when (“Resubmitted” after a rework), the items that are not OK first with the OK ones folded, the readings, the photos and the review history; **Open report →** opens the whole report (every result, readings, parts and refrigerant, work performed, next action, sign-off, photos, reviews). The version shown is `WorkReport.version`, which every draft change increments (deterministic contracts), so a first submission may read “version 4”.
- **Review.** `reviewAvailability` decides whether the buttons appear (self-authored, not current, permission). An internal job is reviewed with `reviewMode=normal`: **Accept report** needs no reason; **Return for rework…** lists the items needing attention (ticked), the readings and the photos (Figma 06-1 “Items that need rework”), and the ticked items prefix the reason (“Rework: …. <reason>”, 1–1000 together). A contractor's job is reviewed by the contractor first; HQ accepts or returns it only with `hq_escalation` and a reason (IR31, D06).
- **Costs.** The cost card lists the lines with the estimate and actual totals per currency, never converted. **+ Add cost line** (kind, description 1–200, amount ≥ 0 with at most two decimals, MYR or USD, customer or internal) and × on a line each save the whole list with `jobs.saveCost`; a cancelled job's costs are read-only.
- **New job.** **+ New job** opens step 1: the unit (grouped by customer), type, symptom (10–2000), the customer's preferred times asked by phone (the 1st is the requested window, up to two more; each 1–4 h, from tomorrow, distinct — IR113 item 2), due (default: the 1st time's end, IR38) and contact window (IR64). **Save as requested** creates the job (`jobs.create`) and opens it. **Next: choose delivery →** creates it and opens step 2, the booking dialog with any of its preferred times still ahead (IR113: no free start / end). The job exists before step 2 because `members.eligible` needs it, so step 2 has no way back to edit step 1; **Leave it requested** closes it.
- **Access extension.** A contractor's job with an accepted offer that is not completed or cancelled has **Extend access…**: a later end (after the current end and after now, otherwise `error.mustExtend`) with a reason (`jobs.extendAccess`).
- **Figma.** Admin 06-1 New job step 2 (281:2, 360:632): one “Visit time · one of the customer’s preferred times” select replaces Scheduled start / end, “Created · requested” replaces Edit, the footer is Leave it requested / Assign (Send offer), and the note says step 1 created the job. Return for rework (360:2): “the technician submits a new report version” replaces “(v2) is expected”.
- **Still to connect.** The Plans, Contractors and SLA tabs, which show illustrative data in API mode.

## IR233 HQ maintenance plans (Plans tab) on the server pattern; plan occurrences book as agreed times; base-layer element styles — 2026-10-09

`/admin/jobs?tab=plans` (FR-A06, DD-A06 item 9, SCR-A06) showed fixture plans in both modes (IR231 list). It now follows IR176 with D16 / IR130.

- **Reads.** `plans.list` in the shared customer → property → unit scope (the tab links keep the scope, DD-A06 item 5), next due first (plans.list sorts by id only); `plans.get` for `planId` (default the first); `units.get` for the place; the status of each generated occurrence from the unit's periodic jobs (`jobs.list` with unitId and origin=periodic_plan; jobs.list has no planId filter). The tab counts are jobs that are not cancelled, plans and contractor organizations in the scope.
- **Plan form.** Repeat is Monthly (read-only), Every 1–12 months, Next due as a date and time in the display zone stored in UTC; the hint shows the anchor day a save stores (the UTC day of a changed next date, otherwise the plan's) and the occurrence after it (D16 arithmetic: the same UTC time, the day clamped to the month). The time of day matters: a generated job's requested window is [occurrence, +1 h). Save plan is enabled only with a change; Discard restores the saved values.
- **Generate job.** Enabled for a saved next date in the future with nothing unsaved; it calls `plans.generateNext` with that date and the plan version. The box then names the new job and the advanced next due. A refused call (another tab, a double submit, a next date saved back onto a generated date) reloads the plan and shows Figma 360:414's banner: “CONFLICT · <date> already has a job … Still one job: <job>” when the reloaded plan has that occurrence, otherwise that the plan changed meanwhile. Occurrences list newest first with their status and Open job →.
- **New plan.** **+ New plan** (`plans.save` without an id): the unit (grouped by customer; a hint names its existing plans), Every and the first visit; the new plan opens selected.
- **Plan occurrences in the Jobs tab.** A plan's job without preferred times now offers its occurrence (the requested window) as the time to book, labelled Plan, with HQ technicians from `members.eligible` — the backend accepts it as an agreed slot (IR113). The list row reads “plan occurrence <time> · book it”. Fixture qualifications end 2026-10-01, so later occurrences have no eligible HQ technician and are offered to a contractor.
- **Styles.** `@ac/web/globals.css` declared element defaults (`a { color: inherit }`, button, focus) outside any layer; unlayered rules beat every Tailwind utility, so `text-primary` / `text-white` on links never applied (selected tabs, TextLink, LinkBtn primary rendered in the body colour). The element defaults are now in `@layer base`.
- **Technician device events.** `/technician/devices/:id` 404'd for a device listed in scope whose unit the technician has no assignment on (`devices.events` FORBIDDEN `errors.assignment_required`); it now shows the device with a notice that events open while there is an assignment (IR94).
- **Still to connect.** The Contractors and SLA tabs.

## IR234 Completion releases the assignment — 2026-10-10

DEC-73. A completed job kept its Assignment active, so its scheduled window went on booking the technician (the seed's week-long job-contractor-a blocked tech-external-a 09:00–17:00 Mon–Fri after it was accepted).

1. **Transition.** When `jobs.review` accepts the report (the contractor with `normal`, HQ with `normal` or `hq_escalation`) and the job becomes completed, the same transition sets the job's active Assignment to `completed` (reason `job_completed`, version + 1, updatedAt = the transition time). A return keeps the Assignment (the rework continues on it); cancellation and reassignment still revoke it.
2. **Capacity and access.** A completed Assignment is not active: it leaves the overlap constraint, `members.eligible` (busy), `members.capacity` (assigned slots) and the contractor week grid; change capture publishes active=false, so unit and device access through it ends (IR186, IR187).
3. **Technician.** The IR49 viewing and work windows end at completion. The technician's history snapshot (IR124) is stored in the same transition (asOf = completion), so `jobs.list` / `jobs.get` return the history projection at once; job-linked writes return FORBIDDEN `errors.assignment_ended`. The completion notification still goes to the technician whose Assignment the transition released, and its link `/technician/jobs/:id` shows the snapshot (completed, report accepted, when the assignment ended) instead of not found (DD-T04).
4. **Projections.** `JobDetail.assignment` is the active Assignment, or for a completed job the one its completion released (status completed), so HQ, the client and the contractor still see who did the work; `JobSummary.technicianMembershipId` / `assignmentAcknowledgement` follow it, and the `membershipId` filter of `jobs.list` matches completed Assignments.
5. **Schema and contracts.** `maintenance.assignments.status` adds `completed`; `Assignment.status` is `active | revoked | completed`. Before the first release the schema is the first migration, so existing local databases are rebuilt (database design §9).
6. **Web.** The HQ delivery card says the assignment ended at completion and that the technician's time is free; the contractor's completed banner says the same; the contractor week grid labels booked cells only with jobs that are not completed or cancelled.

## IR235 HQ contractor register (Contractors tab) on the server pattern — 2026-10-10

`/admin/jobs?tab=contractors` (FR-A21, DD-A21, SCR-A06, IR131, IR133) showed fixture contractors in both modes. It now follows IR176; URL key `contractorId` (the contractor organization).

- **Reads.** Every organization of kind contractor (`organizations.list`) with its profile from `contractors.list` (status, areas, delegation, the rate card in effect, the 90-day KPIs) and its technicians (`members.list`); a contractor without a profile is listed as “No profile” (offers to it still work). For the selected contractor: `rateCards.list` (newest first; without a profile the card in effect is the latest started one) and `certificates.list` of the organization.
- **KPI tiles.** Offer acceptance, arrival in window, report accepted first time, customer rating with count, rework rate (IR131 item 6). There is no read of SLA targets outside the SLA tab and targets are per plan type, so the tiles name the measure instead of a target (Figma 06-9 updated).
- **Profile.** **+ Add contractor** / **+ Add profile** (`contractors.save` without id) for an organization without a profile — registration 1–64, 1–20 distinct service areas (comma-separated), contact e-mail, insurance date (empty: the delegation runs a year); **Edit** saves with the profile version (the organization is fixed).
- **Offers.** **Suspend offers** / **Resume offers** (`contractors.setOfferStatus`) need a reason; while suspended the list says so, the profile shows the reason, and the Jobs tab no longer proposes the contractor for offers or held capacity (`jobs.offer` refuses it with CONFLICT `errors.contractor_suspended`).
- **Rate cards.** **Edit rate card** saves a new version (`rateCards.save`) from a future date (default the first day of next month in the display zone), prefilled from the card in effect: 1–4 lines of different work types, amounts ≥ 0 with two decimals, notes ≤ 200; the rework deduction is entered positive and shown as “−”. Scheduled versions are listed under the card in effect.
- **Certificates.** Each technician shows the qualifications (or that the membership ended) and the soonest expiry; **Verify uploads (n)** lists the `pending_verification` uploads with Approve (the certificate becomes the qualification grant) and Reject (reason required) (`certificates.verify` with the certificate version).
- **New job from other tabs.** The header's **+ New job** on the Contractors tab opens the same dialog; the created job is handed to the Jobs tab in browser module state, so step 2 still opens there.
- **Still to connect.** The SLA tab.

## IR236 HQ SLA by customer (SLA tab) on the server pattern; the scorecard carries its targets — 2026-10-10

`/admin/jobs?tab=sla` (FR-A22, DD-A22, SCR-A06) showed fixture rows in both modes, and DD-A22's “KPI tiles with targets” and the edit dialog had no way to read the targets: `sla.scorecard` judged customers against them but did not return them.

1. **Contract.** `SlaScorecard.targets: SlaTargetView[]` lists, per plan type (rto, general, energy, environment), the targets in effect now — a saved row (`state=in_effect`, version, effectiveFrom) or the IR131 default 4 h / 90 % / 85 % (`state=default`, version 0, effectiveFrom null) — followed by the saved rows that start later (`state=scheduled`, earliest first). Each customer row adds `planType` (the customer's service profile, whose targets decide its status). The response share's target is 100 % of jobs within the plan's response hours (IR131 item 5).
2. **Reads.** URL keys `period` (30 / 90 / 365 days ending now; default 90, IR50 shared key) and `contractorId` (`sla.scorecard` contractorOrgId). Customers are named from `customers.list` with their properties and unit count.
3. **Tiles.** Response ≤ N h (the hours of the customers' plans; “within target” with the hours by plan when they differ), arrival in window and first-time fix with “target X %” (or the range by plan), average rating with its count, open & overdue with the number of breaches in the period; a value below its target is warned (critical when more than 10 points below); no data is “—”, never 0 %.
4. **Customers and breaches.** One row per customer (plan, jobs, response, arrival, first-time fix, rating, overdue, status); **Export CSV** builds the file in the browser from these rows with the period. Recent breaches (newest first, at most 50) link to the job (`/admin/jobs?jobId=`).
5. **Edit SLA targets.** Lists every plan type's targets in effect and scheduled; saving (`sla.saveTargets`) needs response 1–168 whole hours, percentages 0–100 and a start now or later, and creates the plan type's next version; jobs created from then on use it.
6. **Figma.** Admin 06-10 (699:21404): the response tile's target is 100 % of jobs, the overdue tile counts the period's breaches, the customers table has a Plan column, the third breach is an overdue job. With this the Jobs, Plans, Contractors and SLA tabs all read the Core API; customer maintenance is the last screen that mixes fixture rows.

## IR237 Customer maintenance requests (My requests) on the server pattern; the client sees the technician's name — 2026-10-10

`/customer/maintenance` (FR-C09, FR-C17, DD-C09, DD-C17, SCR-C09) read the in-browser job store in API mode: rows were filtered by fixture IDs and most actions wrote to the demo store. The My requests tab now follows IR176.

1. **Reads.** `jobs.list` (client projection, business status order) with each unit's place (`units.list`, `properties.list`, `spaces.list`); status tabs All / Needs your reply / Requested / Scheduled / In progress / Completed / Cancelled and the Origin filter. For `jobId`: `jobs.get`, `jobs.events` (the customer-visible notes — “You” for the client's own — and the history in the client's words) and the accepted report (`reports.get`, up to four photos through `attachments.getContent`). A completed request still waiting for Confirm & rate gets the banner.
2. **Actions.** New request (unit, type, symptom 10–2000, 3 preferred times — each 1–4 h, from tomorrow, all different — and the contact window without phone numbers or e-mail addresses, IR64/IR90) → the new request opens; **Accept this time** / **Decline…** with a reason, comment and 3 new times (`jobs.respondProposal`); **Request another time…** for a plan visit at least 48 h ahead (`jobs.requestReschedule`); **Cancel request** while requested (`jobs.cancel`); **+ Add note** (`jobs.addNote`, customer visibility); **Confirm & rate** / **Edit rating** (`jobs.rate`, 7 days); **Report a problem** with up to 5 JPEG/PNG photos (5.5 MB together, the 8 MiB body of `jobs.reportProblem`) and an optional visit → the follow-up opens “Under HQ review”. Each carries the job version.
3. **Contract.** `Assignment.technicianName`: the technician's display name in the client's projection once the technician accepted the assignment (DD-C09 “once the technician has accepted, the technician's name”); null before, and for the other roles, which read names from `members.list`.
4. **Form fields.** `Field` renders a `<label>` only around a single form control (Input, Select, Textarea or a native one); any other child — choice buttons, a set of checkboxes or buttons, a read-only box — is a labelled group, because inside a `<label>` a button takes the label as its name (the rating stars and tags here; IR230 had fixed it for Choice only).
5. **Times.** A slot over several days shows both ends (“09-14 08:00 → 09-20 08:00”).
6. **Still to connect.** The Filter care tab (`filterCare.*`, DD-C18), which shows illustrative data in API mode.


## IR238 Customer Filter care on the server pattern; the settings read back; run time restarts at 0 h — 2026-10-10

The Filter care tab of `/customer/maintenance` (FR-C18, DD-C18, SCR-C09, Figma Client 07j) showed illustrative data in API mode (IR237 item 6). It now follows IR176, and the gaps found on the way are closed.

1. **filterCare.getSettings** (operation 200, `GET /v1/filter-care/settings`, `client:self`): the caller's customer's FilterCareSettings, or the defaults with version 0 before the first save (thresholdHours null, fallbackDays 30, recipients owners, channels [inApp]; IR134 item 4). The Reminders card and the edit dialog read it; before, a saved setting could only be seen through the thresholds in `filterCare.list`.
2. **Run time since a cleaning** (amends IR134 item 1). The power reading in force at the cleaning counts from the cleaning to the next reading (same stale cap), and a unit that has power readings but none running since the cleaning has 0 h — so Mark cleaned restarts the counter at 0 h (AT-C18-N) instead of turning the run time unknown. Null stays for a unit that is not online or has no power reading at all; then the fallback days decide (IR134 item 3).
3. **The tab.** The tab is in the URL (`tab=filter-care`, `allowed_tabs`), and the server reads only the shown tab: `filterCare.list` and `filterCare.getSettings` on Filter care, the requests on My requests. One row per AC in the list order (overdue, due soon, OK, unknown): run time (“268 h since cleaning”; “… so far” before the first recorded cleaning; “—” with “offline” or “no power readings”), progress (% of the threshold, capped at 100 %; without run time the % of the fallback days since the last cleaning), status, and the last cleaning (“last cleaned Aug 30”, “cleaned by technician Sep 8” with the job link, “no cleaning recorded yet”).
4. **Areas.** A space with 4 or more ACs is one summary row — “Open office · 8 ACs”, “2 due soon · 1 offline”, the average progress of the ACs with run time and “n due” — whose **View n** lists its ACs below it (DD-C18 step 1).
5. **Actions.** **Mark cleaned** on every row, overdue rows included (a customer who cleans an overdue filter must be able to restart its counter; Figma 07j updated); **Request cleaning** on overdue rows opens New request with the unit, type Preventive and a symptom line (“Filter cleaning: 268 h of running since the last cleaning (reminder at 250 h).”), and the created request opens on My requests. **Edit reminders** (owners): model default or 50–2000 h, 7–180 days, owners or all users, the app always and e-mail optionally; members see the settings read-only with “Only the account owner can change the reminders.” (FORBIDDEN otherwise, IR115).
6. **Still to build.** The worker that raises the cleaning_due maintenance Alert and its notifications when an AC crosses 100 % (IR134 item 5).

## IR239 Filter cleaning reminders from the maintenance scheduler — 2026-10-10

IR134 item 5 (and IR238 item 6) left the reminder to a worker. It is now a maintenance scheduler job (registry job, IR54 pattern).

1. **Cadence.** The job evaluates the filter status of the tenant's units (IR134 items 1–3, the customer's settings) every 15 minutes of business time (`maintenance.filter_reminder_watermarks`), not on every tick.
2. **Once per cleaning cycle.** A cycle starts at the unit's last cleaning (or with its readings when none is recorded; `maintenance.filter_reminders.cycle_from`). A unit that is overdue (≥ 100 % of the threshold, or of the fallback days without run time) gets one reminder per cycle; it is never repeated in the same cycle, also when the customer resolves the Alert by hand.
3. **The Alert.** Maintenance assigns the Alert ID and publishes `FilterCleaningDue`; equipment opens the Alert: type maintenance, ruleKey `filter_cleaning`, severity normal, causeCode unknown, evidenceKind inferred, evidence “Cleaning due (268 h of run time since the last cleaning). Not a fault.” (“… of run time, no cleaning recorded” / “… days since the last cleaning, run time unknown”; Figma Client 06a). A later cleaning ends the cycle: `FilterCleaningCleared` resolves the Alert with the reason “filter cleaned” — at once on Mark cleaned, at the next evaluation for a technician cleaning (an accepted report with the filter item normal).
4. **Notifications.** One per recipient and channel of the settings: the customer's active client memberships (`identity.members` with clientRole owner, or all client users), channels inApp (simulated) and email (preview); templateKey alert, type cleaning_due (IR10/IR104), severity normal, target the unit, sourceAlertId, params status open and the evidence as the message. Without a recipient the Alert records a no_recipient DeliveryFailure (SR12). The job audits `filterCare.remind` on the unit.
5. **Internal contracts.** `identity.members` takes `clientRole`; the system query path runs with the system principal, and the system principal reads the tenant's units (unit scope), so a domain scheduler can ask another domain for them.
6. **Web.** A maintenance Alert is titled “Filter cleaning reminder” under Reminders & information; a cleaning_due notification reads “Filter cleaning due on <unit>” and opens Filter care; the inbox no longer prints the in-app channel (it compared with `in_app`).
7. **Found while checking.** In API mode the customer Alerts page still renders its Alert policies tab and the detail's status from fixed demo values, and the customer overview its header (property picker, update time) and attention count; the fixture-ID scan cannot catch literals that equal the seed's names. Next: rename the development data, crawl every app and wire what still shows fixed values.

## IR240 Customer overview on the server pattern; the unit table shows measured values — 2026-10-10

In API mode `/customer` (FR-C01, DD-C01, SCR-C01, Figma Client 01a) read only the power tiles and the unit list from the Core API. The property picker, the update time, Needs attention, the automations, energy, emissions and air quality were fixed demo values. The table's “Room temp” showed the AC's setting (`observedState.celsius`) and humidity and power stayed empty (IR239 item 7). The page now follows IR176.

1. **Scope and period in the URL.** `propertyId` (default: all properties), `unitId` (a unit of that property) and `period` today / 7d / 30d (default today: 00:00 Kuala Lumpur to the business-clock minute; 7d and 30d are whole days up to now, SR17). The tiles count the selection (`summaries.get` with `propertyId` / `unitId`). “Updated” is `Summary.asOf` with a Live badge that turns Stale two minutes after the values were read, and Refresh reads them again.
2. **Unit table.** The latest valid temperature, humidity and power (W) per unit from `UnitSummary.latestMeasurements` (IR213). A stale, suspect or missing reading shows “— no data” (D07), never the AC's setpoint.
3. **Energy used.** `energy.summary` of the period and of the previous period of the same length (“vs yesterday at this time”, “the 7 days before”, “the 30 days before”). The bars are the last 7 days (dark) against the same days a week earlier (light). A day without readings has no bar and is never 0 kWh.
4. **Estimated emissions.** `emissionsKg` of the period with the factor snapshot. With a baseline covering exactly these units (D07), the baseline's emissions and the estimated saving; otherwise “no saving is estimated”. An estimate only.
5. **Air quality.** The selected unit, else the first unit shown with a CO2 reading: the latest CO2 and PM2.5 as the FR-C07 metric cards (a stale or suspect reading is never current), the ventilation advice at ≥ 1000 ppm, and the hourly average CO2 of the last 24 h (up to 300 readings).
6. **Needs attention.** The unresolved critical and warning alerts of the units shown, most severe and newest first (at most 3, then “+n more”). Unresolved normal-severity alerts are listed as “Reminders & info (n) — …” and never counted (IR51); the tile shows `Summary.alertCount`.
7. **Automations.** The rules of the units shown, enabled first: “n on · m off” and at most 3 rules, each with its next run (`automations.nextRuns`) or its trigger sentence (location rules with the consent state).
8. **Boundary.** `telemetry.summary` is no longer read here: the latest readings come with `units.list`, and its energy part is not filled. DD-C01 lists the reads above.

## IR241 The shell names the signed-in user and counts its badges from the Core API — 2026-10-10

In API mode the shell still showed the demo persona: the scope label (“CUSTOMER-A”), the chip (“Client — customer-a”) and fixed sidebar badges (customer Alerts 2, partner Jobs 1, HQ Alert policies 1, Notifications “3 + unread”; IR239 item 7).

1. **Session.** `session.get` returns `displayName` (the user's) and `organizationName` (the membership's organization). The scope label is the organization in capitals and the chip is “<Role> — <display name>”.
2. **Badges.** Customer Alerts is `summaries.get(kind=customer).counts.alertCount` (unresolved critical / warning, IR51). Partner Jobs is `summaries.get(kind=partner).counts.offerCount` (offers waiting for an answer). HQ Alert policies is the unresolved critical / warning alerts (`alerts.list` totals). Notifications in every app is the unread count (`notifications.list` with `unreadOnly`, IR102). A count of 0, or one that cannot be read, shows no badge.
3. **Freshness.** The role layouts (Server Components) read the shell on every render, so a write's `refresh()` updates the badges (opening an alert, reading a notification). Changes made by others show on the next refresh or load.
4. **Demo.** Phase 1A keeps the fixed persona and badges.

## IR242 Customer alert inbox on the server pattern; read state on the notifications — 2026-10-10

In API mode `/customer/alerts` (FR-C08, DD-C08, SCR-C08) listed `alerts.list` rows, but the read flag was a browser-only toggle tied to the alert's status. The detail always said “Unresolved”, and the update time and a “show load error” control were demo values.

1. **Filters in the URL.** `severity` (critical, warning; all omits it, IR74) and `unreadOnly`. The bar shows the unresolved count (`summaries.get` alertCount, IR51) and the number of unread alerts.
2. **Rows.** Unit and place, severity, kind (fault, maintenance reminder, air quality, inspection record), status, evidence and detection time. Unresolved critical and warning alerts are under Needs attention; normal-severity and resolved alerts are under Reminders & information.
3. **Read state (DD-C08).** It is the signed-in membership's notifications about the alert (`sourceAlertId`): Unread while one is unread, ✓ Read when all are read, nothing when the membership got none. Opening an alert marks its unread notifications read (`notifications.markRead` with each version) and never changes the Alert; the Notifications badge drops at once (IR241).
4. **Detail.** The real status: “Open — not resolved yet”, “Acknowledged <time> · still unresolved” or “Resolved <time> · <reason>”. The hint about reading not resolving shows only while the alert is unresolved.
5. **Still to connect.** The Alert policies tab (FR-C15, DD-C15) shows illustrative data in API mode, with a banner.

## IR243 Customer alert policies on the server pattern; the client session names its customer — 2026-10-10

The Alert policies tab of `/customer/alerts` (FR-C15, DD-C15, SCR-C08, Figma Client 06e/06f) showed illustrative data in API mode (IR242 item 5). It now follows IR176, and the inbox gets the rest of Figma 06g.

1. **Session.customerId.** A client session's customer (the membership's organization's, from identity's reference copy, IR188); null for the other roles. A member could not read the customer otherwise, and policy saves and default rule settings name it.
2. **Default policy.** The rules with this customer's settings (a rule without a setting is on, version 0) and “n of 6 rules on”. The owner switches a rule with `policies.setDefaultRule`, sending the setting's version as the expected version (IR115); members see the switches disabled with “Only the account owner can change this”. A rule that is off says who turned it off and when.
3. **Own policies.** Each card has its When sentence (condition, recovery, “only … on weekdays”), its Then sentence (severity and channels), the attached ACs (or “Not attached to any AC yet”), an on / off switch (`policies.save` of the saved policy with only `enabled` changed, at its version), Edit and Delete. Delete names the ACs it detaches from (`policies.delete` at the version, IR108).
4. **Editor.** `policyId=new` or an own policy's id in the URL opens the editor: name 1–120; Temperature / Humidity / Air quality (CO₂, PM2.5) / Power; operator, threshold, duration (s or min, 1 s–24 h) and a recovery on the safe side; optional days and times in the policy's time zone; Info / Warning / Critical; in-app always plus optional e-mail. WhatsApp stays disabled because there is no opt-in yet (OPEN-BE-04). The summary sentence follows the inputs. The hidden fields are the session membership as recipient, escalation 60, cooldown 5, the Preferences time zone and priority 50 on create; on edit the policy keeps its own (IR120). Attaching stays on each AC (DD-C03).
5. **Inbox (Figma 06g).** A unit filter (`unitId`, `alerts.list` filters.unitId), “Updated hh:mm · n unread”, and per alert: View unit, Book cleaning for maintenance reminders (Filter care), Request repair for unresolved faults. Request repair opens New request for the unit (`/customer/maintenance?new=<unitId>`, type repair).

## IR244 HQ overview sections from admin.summary — 2026-10-10

In API mode `/admin` (FR-A01, DD-A01, SCR-A01, Figma Admin 01) read `admin.summary` only for the KPI row and only for today. The filter bar, the as-of line, the forecast card, the power and connection cards, jobs by status and billing were fixed demo values, and so were the skeleton / error / zero-units toggles (IR239 item 7).

1. **Scope and period in the URL.** `customerId`, `propertyId` (options from `customers.list` and `properties.list` of the customer, hidden without asset.read) and `period`: today, 7d, 30d or custom with `from` / `to` in Kuala Lumpur time, at most 366 days (SR17, IR74). A refused custom range is explained and reads nothing. The as-of line gives the time and period range, with Refresh.
2. **Sections, all from the one result.** The eight KPIs link to their lists with the scope but not the period (IR50). The forecast card shows Expected reduction / Expected increase / No change 0.0 with absolute values, the predicted baseline (reference, method, kWh), the predicted actual (kWh on valid slots ÷ valid × expected unit-minutes), the coverage and the quality warnings. Without a forecast it says No target equipment / Baseline not set / Cannot calculate (IR78). The power and connection cards show a stacked bar and one row per class with its list link (SR27). All ten job statuses are listed, zeros included. Billing has one row per currency, never summed, and is hidden without billing.read.
3. **Jobs link.** `jobCounts` counts the jobs whose requested slot starts in the period. A status row opens the Jobs tab for that stage and scope, but the Jobs tab has no period filter, so it lists every job of the stage. Cancelled has no stage and opens the tab unfiltered.
4. **Demo.** Phase 1A keeps the fixture KPIs and the state toggles.

## IR245 HQ Jobs tab period from the overview — 2026-10-10

IR244 item 3 left a gap: a job-status row of the HQ overview opened the Jobs tab with the stage and scope but without the period, so the tab listed every job of the stage, not the jobs counted on the overview. This supersedes IR244 item 3.

1. **Period in the URL.** `/admin/jobs` takes `from` / `to` (ISO instants). They apply only when both parse and from < to; otherwise they are ignored. `jobs.list` filters.from / to select jobs whose requested slot starts in [from, to), the rule of `admin.summary` jobCounts (query catalog). The period narrows both the list and every stage total.
2. **Chip.** “Requested time MM-DD hh:mm – MM-DD hh:mm” (Kuala Lumpur time) under the filter bar; ✕ clears the period and the selection. Filter changes replace the history entry, so Back returns to the overview with its period (D13). The scope line reads “n jobs in scope · in the period”.
3. **Overview links.** The status rows and the Jobs KPI carry the scope and the period. Current-state KPIs (units, alerts, unpaid) still carry no period (IR50). The tab also lists jobs of archived units, which the overview does not count, because its target units exclude archived ones (DD-A01).

## IR246 Preferences on the Core API — 2026-10-10

In API mode `/settings/preferences` (FR-X01, FR-X08, DDC-07, SCR-X-settings-preferences, Figma Client 10d / 10g) was still the Phase 1A page. Choices only showed a toast, the consent and two-step switches saved nothing, and a client could not turn the monthly report e-mail off there, although BR-C16 says it can.

1. **Reads.** A Server Component reads `preferences.get`, `twoFactor.get` and, for a client, `consents.get` for `location_automation`; NOT_FOUND means not granted yet. The time zone options are the usual six, plus the saved zone if it is another one. Each shows its UTC offset at the demo clock, worked out on the server from the numeric wall-clock parts. Node and the browser name offsets differently (“UTC” and “UTC+0”), which broke hydration.
2. **Save.** One Save sends `preferences.update` with the language and time zone. A client's save also carries `monthlyReportEmail` (IR142 item 5). When the location consent changed, it also sends `consents.update` with the consent's version as the expected version (0 when none is recorded). Cancel restores the saved values, and a refusal shows in a banner.
3. **Consent line.** It reads “Granted <date>”, “Withdrawn <date>” or “Not granted yet”. The microphone switch stays on the device: it is a browser permission, not an account setting.
4. **Two-step verification.** Turn on shows the setup key from `twoFactor.get` and asks for a 6-digit code (`twoFactor.enable`), then shows the recovery codes once. Turn off asks for a code (`twoFactor.disable`). While it is on, the screen shows how many recovery codes are left. Any 6 digits are accepted in this build (IR144 item 6).
5. **Monthly report e-mail (client).** “Email me the monthly energy report (1st of each month)” is under Reports. It is the same flag as the Energy export dialog's option (BR-C16, AT-C16-B). Figma 10d and 10g gain the row.
6. **Demo.** Phase 1A keeps the browser-only page.

## IR247 Two-step verification dialog as in Figma 10g — 2026-10-10

The Turn on dialog of `/settings/preferences` (FR-X08, IR144, IR246) showed the setup key as text with one input, while Figma Client 10g and the other roles' Preferences frames show a QR code beside the steps and six code boxes.

1. **Setup QR.** While two-step verification is off, the page turns the `twoFactor.get` setupKey into the authenticator key URI of the signed-in user: `otpauth://totp/AC%20Project:<display name>?secret=<key>&issuer=AC%20Project&algorithm=SHA1&digits=6&period=30`. It draws the URI as a QR code on the server (error correction M, one SVG path), dark on white in a 150 px frame. A unit test decodes the drawn code back to the URI with an independent decoder.
2. **Steps.** “1  Scan with an authenticator app”, “or enter key” with the key in groups of four, “2  Enter the 6-digit code” with six boxes, and “3  Save your 8 recovery codes (shown after verifying)”. The note: no real authenticator is called, any 6 digits are accepted and sign-in is unchanged; turning off later asks for a current code.
3. **Code boxes (CodeInput).** One transparent input over six boxes, so typing, pasting and the browser's one-time-code autofill all work. Only digits are kept, at most six; the next box is outlined while the input has focus. Verify stays disabled until six digits are entered. Turn off uses the same boxes.
4. **Libraries.** The QR encoder is `uqr` 0.1.3 (MIT, no dependencies) in `@ac/web`; the test decoder `jsqr` 1.4.0 (Apache-2.0, no dependencies) is a development dependency only.
5. **Demo.** The Phase 1A dialog shows the same layout with the fixed demo key and Figma's note.

## IR248 The shared screens' frame renders per request — 2026-10-10

After a Server Action on `/settings/preferences` or `/notifications`, the frame of the shared screens switched to the demo persona (scope CUSTOMER-A, chip Client — customer-a, fixed badges), even in the HQ app. A reload restored it.

1. **Cause.** The shared layout covers `/demo`, which had no request-time API and was prerendered at build time. There is no session or Core API at build time, so its frame was the demo one. The sidebar link prefetched `/demo` in production, and the client router reused that cached layout segment when an action called `refresh()`. Development servers do not prefetch, which is why only `next start` showed it.
2. **Fix.** The shared layout calls `connection()` and so renders per request. With no session in API mode it takes the app's own role instead of the browser's last demo role. `loadShell` (every role layout) calls `connection()` too, so no frame that reads the session is prerendered. `/demo`, `/forbidden` and `/technician/jobs` are now dynamic; `/`, `/_not-found` and `/forgot-password` stay static and have no session-dependent frame.
3. **Check.** In all four apps the frame keeps the signed-in organization, user and badges after the enable and disable actions.

## IR249 Demo controls on the Core API — 2026-10-10

In API mode `/demo` (FR-X05, DDC-07, SCR-X-demo, Figma Client 10e) was still the Phase 1A page: a fixed clock (“2026-09-14 09:41 UTC”), a simulator switch and reset that changed only browser stores, and failure triggers that only showed a toast. The renamed-data audit of IR244 missed it, because a time is not a renamed name.

1. **Layout (Figma 10e).** An orange header with “✦ Demo controls” and the Always labelled demo badge, and the dashed notice that nothing reaches real devices, payments, notifications or IoT. Demo clock, Scenario and Reset sit in one row, above the Trigger failures tiles. The browser demo uses the same layout.
2. **Demo clock.** The Core API scenario clock (session.get meta) in UTC. +1 min / +1 hour call `demo.advanceClock` with the clock now plus the jump (forward only, IR36). Every service reads the shared offset at most a second late (IR168), so the action waits until session.get shows the jump before the page renders again.
3. **Device offline / Restore connection.** The device list holds the devices the signed-in role can see (`devices.list`, unit names from `units.list`) whose history it can read (`devices.events`; a technician needs a current assignment, SR24). Devices left out are counted. Device offline sends `demo.trigger` with eventType device, kind communication_lost, the device's current binding, and a sequence above every event of the device. While that fault is open, the tile turns into Restore connection: kind restored with recovery {axis connection, value online, sourceEventId: the open fault} (SR20). Refusals are explained: demo operations off, the clock moving backwards, the device rebound, the fault already restored, the device no longer visible.
4. **Expire session.** Drops this app's session cookie and opens sign-in, as when the session lifetime ends (D09). The identity provider's own session is untouched.
5. **What stays in the browser demo.** Reset is disabled with its reason: the running Core API keeps shared data, and the reset is offline with `make resetdb` (IR154 item 4). Delay transport is browser-demo only (IR37). The Scenario select shows the one Core API seed (generation 1).
6. **Catalog.** SCR-X-demo reads `devices.list` (primary), `units.list`, `devices.events` and `devices.get`, so it carries the IoT device states. The IR90 state set for SCR-X-demo is extended to match.

## IR250 Sign out ends the session in API mode — 2026-10-10

The shell's Sign out was a link to `/login`. In API mode the BFF session cookie stayed, so the role area opened again without signing in (FR-X01: sign-out clears screen content and cache).

1. **Fix.** In API mode Sign out is a form that posts to `/bff/auth/logout`. The route drops the BFF session and redirects to `/login` with 303, and the full navigation clears the screen and its client cache. The browser demo keeps the link to the role picker.
2. **Check.** In all four apps, after Sign out (or Expire session, IR249) the role home redirects to `/login?returnTo=…`.
3. **Unchanged.** The local Keycloak session ends with its own lifetime, so signing in again within it needs no password (local runs, as before).

## IR251 Sign-in page on the server, with the reason a sign-in came back — 2026-10-10

`/login` (FR-X01, DDC-07, SCR-X-login, Figma Login frames of the four roles) was a Client Component. It read `/bff/session` in the browser to learn the data source, so Continue pointed at the demo home until that read came back. The BFF callback's `?error=` was never shown: a refused sign-in just landed on the page again.

1. **Server-rendered.** Each app's page passes the data source and the URL parameters. In API mode Continue is a full navigation to `/bff/auth/login` with the demo identity as login hint and the kept returnTo (only inside the role's area, never `//`; the BFF validates it again). The browser demo opens the role home and remembers the role for the shared screens' frame.
2. **Reason (error state).** `state`: the sign-in expired or was opened in another tab. `token`: the identity service did not accept it. `membership`: no active AC Project membership. `role`: the account belongs to another service. `expired`: the session ended (Expire session, IR249). Any other value: the sign-in did not complete.
3. **Figma texts.** “AC Project — Client / Partner / Technician / Admin”, each role's line (for example, “Sign in to manage customers, contracts, and access”), the Not real authentication badge, the demo account tile (the technician app's is tech-internal-a, “Internal · inspections & work reports”), “Continue as … →”, the other three services in Figma's order (links when their URLs are configured), Forgot password? and the demo note, with the English chip at the top right (the only language in this build).
4. **Check.** In the customer, partner and technician apps the full UI sign-in works: a protected path redirects to `/login?returnTo=…`, Continue opens the local identity provider with the username filled in, and after sign-in the browser is back on the requested path. The HQ app has the same page; its sign-in also asks for the one-time code.

## IR252 End-to-end tests of the four web apps — 2026-10-10

The UI flows had been driven only by scripts outside the repository. `service/web/e2e` is now a Playwright suite (Playwright Test 1.62, the version of the cached Chromium) that runs against the local stack in API mode: the compose backend with Keycloak, and the four apps on ports 3000–3003 (`E2E_<APP>_URL` points a project at another host).

1. **Projects.** One project per app (customer, partner, technician, admin) runs the shared specs and its own. A setup project per app signs in once through the UI, as a person would: the sign-in page, Continue, the identity provider with the account filled in, the password and, for HQ, the one-time code. It then stores the browser state for the app's specs.
2. **Credentials.** The local demo realm's test values are read from its import file (`docker/keycloak/realm-ac.json`), so they live in one place; `E2E_PASSWORD` and `E2E_TOTP_SECRET` replace them for another identity provider. A one-time code is accepted once per 30-second window, so a sign-in that comes too soon after the last one waits for the next window.
3. **Specs.**
   - smoke: every page the app links to renders, with no error boundary, not-found text, page error or failed document.
   - session: the shell names the signed-in user; Sign out ends the session; a protected page keeps its returnTo; a sign-in that came back says why.
   - preferences: time zone and language; a client's consent and monthly report e-mail; two-step verification on and off.
   - demo: the labelled panel, the clock and the disabled browser-only parts, a device fault and its recovery, Expire session.
   - admin: an overview job-status row opens the Jobs tab for its period.
4. **Rules.** Specs put back what they change and run one at a time, because they share the demo users. A save is done when the Server Action has answered and its toast has appeared, not when the button is disabled (it is also disabled while pending). Moving the demo clock is opt-in (`E2E_ADVANCE_CLOCK=1`), because the clock never goes back. Specs are named `*.e2e.ts`, so the shared package's Vitest never picks them up.
5. **Commands.** From `service/web`: `npm run e2e` (`-- --project=admin` for one app). `npm run typecheck` also checks the suite (`e2e/tsconfig.json`). Browser state and run output stay out of git.
6. **First run.** 45 passed and 8 skipped (the clock jump in every app, the client-only consent in the other three, the technician's device fault while no assignment is current). The technician smoke spec found the gap closed in IR253.

## IR253 A technician's job notifications follow the assignment — 2026-10-10

The technician inbox listed a schedule_change notification for a job whose assignment HQ had revoked. Its link opened “This page isn't available”, because `jobs.get` refuses a job without a current assignment (IR49), while IR58 says such notifications must not be listed at all. notify checked only that an assignment row existed, whatever its status or window.

1. **Rule.** A technician reads a job target while an assignment of theirs is active and its scheduled window has not ended, the condition of `jobs.get` (IR49). Otherwise the notification is left out of items, total and unread counts, and cannot be marked read (NOT_FOUND). It comes back if access does (IR58).
2. **Data.** notify's copy of the assignments (`notify.ref_assignments`, IR188) also keeps `scheduled`. The change-capture trigger sends it, and existing rows are backfilled from `maintenance.assignments`.
3. **Remaining difference.** For an external technician, `jobs.get` also needs the accepted offer's access window. notify has no copy of that window, so the scheduled window, which lies inside it, stands in.
4. **Check.** TestTechnicianJobNotificationScope: listed while active and inside the window; hidden when revoked and after the window; mark-read 404 outside the window and 200 inside. make test-all passes on both suites. In the dev stack the technician's revoked-job notification is gone, and the E2E technician smoke spec passes.

## IR254 Technician QR one-tap labels are the units of the open jobs — 2026-10-10

The Scan unit QR dialog (FR-T13, DD-T13, IR230) offered a one-tap label for every unit in `units.list`. For an internal technician that is the whole SR03 unit scope. `units.resolveQr` resolves only units with an active assignment of the technician (DD-T13: assigned units only; IR111), so most of the offered labels answered “Page unavailable”.

1. **Labels.** The one-tap labels are the units of the technician's open jobs: `jobs.list` with statuses assigned, in progress, on hold and rework, the jobs `resolveQr` opens, with the names from `units.list`. The heading reads “Labels on the units of your open jobs”. Typing a code or unit ID still reaches any unit, and an unassigned one still answers Page unavailable.
2. **Catalog.** DD-T13 lists `jobs.list` in its row and service boundary, and `jobs.list` carries DD-T13 among its design ids (swagger regenerated).
3. **Check.** The E2E technician spec (IR255) scans an unknown code (refused, no unit data). It then taps the one offered label, which matches with the next job, and Open unit opens the unit register.

## IR255 More end-to-end specs — 2026-10-10

The IR252 suite gains specs for the customer, partner and technician apps. Each restores what it changes.

1. **Customer alert policies.** An owner switches the AC offline default rule and back. A policy (named `E2E …`) is created with a recovery on the wrong side refused first, edited (threshold, weekdays only), switched off and deleted. A cleanup after each test removes any `E2E …` policy that is left.
2. **Customer filter care.** The tab is in the URL and every AC has a row. Reminder settings refuse 20 hours, save 120 hours for all users, and are put back to the model default, 30 days and owners. Request cleaning opens New request prefilled for an overdue AC and is cancelled. Mark cleaned is left out, because it moves the cleaning date for good.
3. **Partner jobs.** Every status tab keeps the URL and is selected. It lists jobs, or nothing when its count is 0, and the first job opens its detail.
4. **Technician QR scan.** See IR254.
5. **Result.** 51 passed and 9 skipped. The skips: the opt-in clock jump in each app, the client-only consent in the other three apps, the technician's device fault without a current assignment, and Request cleaning while no AC is overdue.

## IR256 An end-to-end scenario across the four apps — 2026-10-10

The IR252 suite now also checks one job's way through all four apps. A `scenarios` project runs after every app's sign-in and opens its own browser context per app.

1. **Scenario.**
   - The customer sends a maintenance request with the default three preferred times.
   - HQ opens it in the Jobs tab, uses the first preferred time and offers it to contractor-a with the default offer and access windows.
   - The partner accepts the offer and, in Schedule & assignments, gives the job to tech-external-a.
   - tech-external-a signs in through the technician app (another account of the same role on the identity provider's form). They have the job to accept on the overview, and its page opens.
2. **Clean-up.** HQ cancels the job with a reason at the end, whatever happened before.
3. **Customer request spec.** New request refuses short symptoms and a phone number in the contact window. It is sent with three preferred times, takes a note to the coordinator and is cancelled with a reason. The spec cancels the request it sent whatever happened before, after a failed run had left one requested (it was cancelled by hand).
4. **Writing specs.** A success that shows both as a toast and as a banner matches twice, so specs take the first match. A clean-up error must not hide the step that failed.
5. **Result.** 53 passed and 9 skipped. Each run leaves two cancelled jobs whose symptoms start with “E2E”.

## IR257 The frontend design follows the build; demo data per tab; two open items — 2026-10-10

Board 02 had listed common.md §1 / §8 and board 01 as not aligned with the build. The design text now describes what runs.

1. **Structure (common.md §1).** The `service/web` workspace: one Next.js 16 App Router app per entry point (IR178) and `@ac/web` (components, lib, screens, bff), plus the e2e suite.
2. **Data (§1, §4).** API mode reads in Server Components through the server-only DAL, at the operations' Core API REST routes (IR222). Writes are Server Actions with the row version and an Idempotency-Key, then `refresh()`. The few interactive reads go through `/bff/ops`. This follows the user's instruction to follow the Next.js documentation (2026-10-08). §4 marks the Repository interface as the Phase 1A proposal; its contract (ServiceResult, DomainError, late responses) still holds. The catalog's `rest_routes` column now appears in §4.
3. **Navigation.** `next/link` and `next/navigation` directly; filters, tabs and the selection in the URL (`lib/urlState`, `useUrlPatch`). The Navigation-interface proposal is not used.
4. **Demo data per tab (FR-X05).** The Phase 1A job and client-user stores kept their state in localStorage, so a reload did not bring back the seed. They now keep it in sessionStorage through `lib/demoStore` and drop it when the document was opened by a reload. The four apps of a tab on one origin see the same demo data, a role switch keeps it, and a reload or `/demo` reset restores the seed. In demo mode, 7 requests became 8 after a new request, stayed 8 across a fresh navigation and in-app navigation, and were 7 again after a reload; unit tests cover keep, reload and blocked storage.
5. **Open — UI and forms (DEC-03).** The SRC-02 condition “shared libraries, reactForms” has no archived original. DEC-03 (PROPOSED) reads it as shadcn/ui, Lucide, React Hook Form, Zod and TanStack Query, and the role designs and the UIUX specification still describe that. The build uses in-house Tailwind components and controlled forms with shared validators. This is a product-owner decision: adopt the libraries, or record the build's choice and update the role designs and the UIUX specification.
6. **Open — language (FR-X01).** A change to Bahasa Melayu is stored but has no effect: the build shows English only.
7. **Scenario.** The IR256 scenario continues. tech-external-a accepts the assignment (受領); the partner then sees it accepted and the customer sees the technician's name. Starting the work needs the visit window, which only a jump of the never-returning demo clock would reach.

## IR258 Bahasa Melayu on the shell, Preferences, Demo controls and the inbox — 2026-10-10

IR257 item 6 left FR-X01's language open: a change to Bahasa Melayu was stored but had no effect. The build now shows the saved language on part of the web apps. AT-X01-N ③ holds there: stored UTC times, units and IDs do not change.

1. **Where the language comes from.** API mode: the role layouts read `preferences.get` on the server (`coreDisplay` in the DAL, once per render). The locale reaches the client components through `I18nProvider`. After a save, `refresh()` renders the shell again in the new language. Browser demo: Preferences keeps the choice in this browser (`ac-locale`), and the shell reads it from there.
2. **Dictionary.** As in the Next.js internationalization guide, the Malay dictionary is a plain object (`shared/lib/i18n-ms.ts`). The English text is the key, so English needs no dictionary. A text without a Malay entry stays English, with a warning in development (IR44). `{name}` placeholders keep their names in both languages.
3. **Key check (IR44, NFR-07).** IR44 asks the lint to compare the en and ms key sets. With the English text as the key, a unit test does the same job:
   - every literal the code translates has a Malay entry;
   - every Malay entry is still shown by the code.
   A mismatch fails `npm test`.
4. **Covered.** Translated now:
   - the shell: sidebar items, app names, the role chip, Users / Notifications / Preferences / Demo controls, Sign out, the header buttons;
   - Preferences, both the API view and the browser demo, with the two-step dialogs and the consent line;
   - Demo controls (API and browser demo, with the refusals);
   - the notifications inbox, whose titles come from the templates.
   Inbox times follow the IR44 rule: `Intl.DateTimeFormat(en-MY | ms-MY, { timeZone: Preferences.timezone, dateStyle: medium, timeStyle: short })` with the zone's abbreviation, for example “14 Sept 2026, 9:00 am MYT”. A notification's reason, message and status are shown as the Core API recorded them.
5. **Still English (open).**
   - The other business screens, the sign-in page (no user is known before sign-in) and the error pages.
   - Dates on the other screens: they still use en-MY in Asia/Kuala_Lumpur without the abbreviation, not the user's display time zone.
   These move to the IR44 rule screen by screen.
6. **Draft wording.** The Malay texts are a draft by the implementation agent. Business, UI and UX have not reviewed them. The Preferences hint says so: "Malay text is a draft under review; text not translated yet stays in English."
7. **Checked.**
   - Vitest: 158 passed, including the key check, placeholder parity, the nav labels and the time format.
   - E2E: 53 passed, 9 skipped. Each app switches to Malay and sees “Log keluar”, “Keutamaan”, “Bahasa paparan” and the inbox tab “Semua”, then switches back to English in a `finally`.
   - The dev data is back to en / Asia/Kuala_Lumpur for all four users. customer-a had been left on Asia/Tokyo by an earlier debugging run and was reset.

## IR259 The customer's unit screen in the display language and time zone (AT-X01-N) — 2026-10-10

AT-X01-N ③ opens `/customer/units/unit-online-rto` and expects the Malay display after `preferences.update(locale=ms)`, with stored UTC times, units and IDs unchanged. IR258 had left that screen in English.

1. **Display context.** The shell now passes the user's display language and time zone to the client (`ShellLive.display` from `coreDisplay`; `useDisplay()` beside `useT()`). The browser demo uses the language chosen in this browser and Asia/Kuala_Lumpur.
2. **Unit screen (FR-C03).** Every label, banner, dialog and toast of `/customer/units/[id]` is translated:
   - devices in the room;
   - the readings, remote control and the confirmation;
   - live telemetry and device information;
   - alert policies on the AC;
   - command history.
   The mode and fan words (Cool / Dry / Fan, Low / Mid / High) are translated; the stored values stay cool / dry / fan and low / mid / high. The server page words its policy rules in the user's language.
3. **Times (IR44).**
   - Recent readings and the reported setting show the time of day, for example “9:59 am GMT+9”.
   - Last seen and the command history show the date and time.
   Both use the user's display time zone and the zone's abbreviation. `showClock` / `showTime` in `lib/i18n` do this; a zone the runtime does not know falls back to Asia/Kuala_Lumpur.
4. **Shared parts.** These now speak the display language on every screen: the state badges (power, connection, severity, on/off), the dialog's Close button and the empty table text. The unit helpers take the translator and the display: `actionText`, `historyRow` and `latest`. The technician's unit monitoring shows its readings in the display time zone; its text is still English.
5. **Translation library — recorded with DEC-03.** The UIUX specification lists i18next + react-i18next as the translation candidate. The build follows the Next.js internationalization guide instead: plain dictionaries, the locale chosen on the server. That is the user's 2026-10-08 instruction to follow the Next.js documentation. Like the UI and form libraries, the UIUX table is left for the product owner's DEC-03 decision.
6. **Checked.**
   - Vitest: 161 passed, including units.test.ts — command and history wording in both languages, IR44 times in another zone, and the quality of a stale reading. The key check of IR258 covers the new texts.
   - E2E: `customer/unit-language.e2e.ts` saves Malay with Asia/Tokyo, opens an AC from the overview and checks the following, then puts English and the zone back in a `finally`:
     - the Malay headings;
     - the same path (ID);
     - °C;
     - GMT+9 and no MYT.
   - The whole suite: 54 passed, 9 skipped. All four users are back to en / Asia/Kuala_Lumpur afterwards.
   - A screenshot of the unit screen in Malay with Asia/Tokyo shows no overflow.
7. **Still open.** The other business screens and their dates (klTime in Asia/Kuala_Lumpur), and the voice demo's answers.

## IR260 The customer overview in the display language and time zone — 2026-10-10

After the unit screen (IR259), the customer's home screen `/customer` (FR-C01, DD-C01, Figma Client 01a) is the next key screen of FR-X01.

1. **Texts.** The overview is translated: the scope bar, the four counts, the energy, emissions, air-quality, units, Needs attention and automation cards. So are the texts its server reads build: the scope ("all 4 units", "2 units in Home A"), the energy change against the previous period, the emission factor, the automation lines and the day labels of the energy chart.
2. **Times (IR44).**
   - A unit's latest reading and the read time ("Updated …") show the time of day in the user's display time zone with the zone's abbreviation. Last seen shows the date as well.
   - Times on the days next to now, such as Needs attention and next runs, show "today", "yesterday" or "tomorrow" instead of the date ("today 9:12 am MYT"). Other days show the IR44 date and time. The day is the calendar day in the user's display time zone (`relativeTime` in `lib/i18n`).
3. **The period note stays in Kuala Lumpur time.** The overview's periods (today, the last 7 and 30 days) are Kuala Lumpur days (REV18-035). The note names the zone ("Today = 00:00–16:10 · Asia/Kuala_Lumpur"); only its words and month names follow the language.
4. **Shared helpers.** These take the translator and the display (`I18n`, `i18nOf`, `useI18n`):
   - `lib/customerOverview` — every function;
   - `alertTitle`;
   - `metricCard`;
   - the automation sentences and the rule card: `ruleCard`, `whenText`, `weekdaysText`, `extraText`, `onlyIfText`, `thenText`, `actionLabel`;
   - `dayRanges`.
   A rule's runs keep the rule's own time zone, now with its abbreviation ("Mon, 14 Sept, 18:00 MYT"), because a schedule is set in local time. Screens that do not pass a display yet get English, with IR44 times in Asia/Kuala_Lumpur: air quality, automations and energy for the customer, and the alert titles of the other roles.
5. **Checked.**
   - Vitest: 33 files, 168 tests.
     - New: `air.test.ts` (IR44 rounding, live, stale, suspect, null and unsupported readings, Malay with Asia/Tokyo) and `clientAutomations.test.ts` (days, actions, only-if sentences, run times in the rule's zone, card states in both languages).
     - `customerOverview.test.ts` now expects the IR44 times and adds Malay / Asia/Tokyo cases.
   - The typecheck caught a duplicate dictionary key (TS1117), so the key check relies on it as well.
   - E2E: `customer/unit-language.e2e.ts` also opens the overview in Malay with Asia/Tokyo. It checks the card headings, the read time in GMT+9 and the period note in Asia/Kuala_Lumpur. The whole suite: 54 passed, 9 skipped; the users are back to en / Asia/Kuala_Lumpur.
   - A screenshot of the overview in Malay with Asia/Tokyo shows no overflow.
6. **Still open.** The customer's alerts (inbox and policies), maintenance and the other screens, and the voice demo's answers.

## IR261 The customer's alerts in the display language and time zone — 2026-10-10

`/customer/alerts` (FR-C08, FR-C15, DD-C08, DD-C15, Figma Client 06a–06g) follows the overview (IR260).

1. **Inbox.** These are translated:
   - the filters and the summary line;
   - Needs attention and Reminders & information;
   - each alert's title, kind, status, read state and links;
   - the alert dialog.
   Times follow IR44 in the user's display time zone with the zone's abbreviation: detected, acknowledged, resolved and the read time. The evidence text and the resolution reason are shown as the Core API recorded them.
2. **Status by state, not by words.** An inbox row now carries `status.state` (open / acknowledged / resolved) beside its text. The view used to compare the English text ("Resolved") to hide Book cleaning and to word the dialog; it now uses the state, so the screen behaves the same in Malay.
3. **Alert policies.** These are translated:
   - the default policy: its conditions, types, the turned-off note with its day (`showDate`, IR44 date in the display time zone) and the owner-only notice;
   - the customer's own policies: When / Then, attached ACs and notes;
   - the delete dialog;
   - the editor: its steps, checks, live summary sentence and refusals.
   The rule names of the default policy are HQ's data and stay as HQ wrote them. The rule type badge uses the rule category, not its translated word.
4. **Shared helpers.** These take the translator and the display:
   - `inboxAlerts`;
   - in `lib/customerPolicies`: `conditionText`, `windowText`, `defaultRuleRows`, `policyCards`, `policyErrors`, `summaryText`, `policyRefusal`;
   - `recoveryError` in `lib/adminAlerts`.
   HQ's alert screens still call them without a display, so they stay English.
5. **Checked.**
   - Vitest: 33 files, 168 tests.
     - `notifications.test.ts` checks the inbox in Malay with Asia/Tokyo and the status states.
     - `customerPolicies.test.ts` checks the default rules, the editor's summary and checks, the cards and a refusal in Malay.
     - The IR44 times replace the old ones in both.
   - E2E: `customer/unit-language.e2e.ts` also opens the alerts in Malay with Asia/Tokyo: the inbox headings, the read time in GMT+9, the Alert policies tab and its table. `customer/alert-policies.e2e.ts` still passes in English. The whole suite: 54 passed, 9 skipped; the users are back to en / Asia/Kuala_Lumpur.
   - Screenshots of the inbox and the policies tab in Malay with Asia/Tokyo show no overflow.
6. **Still open.** Maintenance and the other customer screens, the browser demo's alerts screen (its seed rows and labels), the other roles' screens and the voice demo's answers.

## IR262 The customer's maintenance in the display language and time zone; preferred times typed in that zone — 2026-10-10

`/customer/maintenance` (FR-C09, FR-C17, FR-C18, DD-C09, DD-C17, DD-C18, Figma Client 07a–07p) follows the alerts (IR261).

1. **Texts.** These are translated:
   - My requests: the tabs, the Origin filter, each row's next step and both banners;
   - the request detail: facts, preferred times, the proposal to answer, the plan visit, the completed report, the rating, notes and history;
   - every dialog: new request, decline, another time, rate, report a problem, note, cancel;
   - Filter care: the table, area rows, the reminders card and its dialog;
   - the job status and origin badges on every screen.
   A request's symptom, notes, the proposal's message, part names and the work text stay as written. Rating tags are stored in English (the API's values); only their labels follow the language. The default rule names (IR261) and other HQ data stay as recorded. The history uses the client's titles and, for the rest, the shared job-event titles, also translated.
2. **Times (IR44, NFR-08).** These follow the user's display time zone:
   - Booked, preferred and proposed times show as one span with the zone's abbreviation, for example “Tue, 22 Sept, 10:00 am – 12:00 pm MYT” (`showSpan`, Intl `formatRange`).
   - Reply-by, editable-until, notes, history and the report's acceptance show the IR44 date and time; the rate banner and the plan visit show the date.
   NFR-08 asks that a time zone change keep the meaning of booked times. Times are stored in UTC, so the same visit reads 11:00 am GMT+9 in Asia/Tokyo.
3. **Preferred times typed in the display time zone.** The request, decline, another-time and problem dialogs used to read the typed date and time as Kuala Lumpur time (+08:00), while the screen now shows times in the user's zone. They now read them in the user's display time zone (`zonedInstant`), prefill in it (`zonedParts`) and say so ("times in Asia/Tokyo"). The "from tomorrow" check stays the backend's rule: a later day than today in Kuala Lumpur (`PreferredSlotsOK`), and its message says so.
   Checked: with Asia/Tokyo saved, the default first time (16 Sept, 10:00–12:00) was stored as 01:00–03:00 UTC. The request was cancelled and the zone put back.
4. **Fixed: an empty date crashed the request form.** `new Date("T10:00:00+08:00").toISOString()` threw during render when a date field was cleared, so the error boundary replaced the screen. An incomplete date or time is now "" and reads as "Enter the date and both times of each preferred time."
5. **Plain spaces in times.** ICU puts narrow and thin spaces (U+202F, U+2009) around times and ranges, and versions differ between Node and browsers. A time a client component renders on the server and again on hydration could then differ. `lib/i18n` now returns plain spaces in every time it formats.
6. **Shared helpers.** These take the translator and the display:
   - `lib/customerMaintenance` — every function;
   - `lib/customerFilterCare`;
   - `inspectionRows` and `readingRows` (also used by the partner's review);
   - `JobStatusBadge`, `OriginBadge` and `Rank`.
   The partner's screens still call the report helpers without a translator, so they stay English.
7. **Checked.**
   - Vitest: 33 files, 172 tests.
     - `i18n.test.ts`: zone conversions both ways (Kuala Lumpur, Tokyo, UTC, incomplete input, an unknown zone) and spans with plain spaces.
     - `customerMaintenance.test.ts` and `customerFilterCare.test.ts`: the IR44 spans and dates, plus Malay / Asia/Tokyo cases.
   - E2E: `customer/unit-language.e2e.ts` also opens maintenance in Malay with Asia/Tokyo — the tabs and the New request dialog with "masa dalam Asia/Tokyo". The maintenance-request and filter-care specs pass in English. The whole suite: 54 passed, 9 skipped; the users are back to en / Asia/Kuala_Lumpur, and all 27 E2E jobs are cancelled.
8. **Still open.** The other customer screens (units & locations, automations, energy, air quality, contracts & payments, users), the browser demo's screens, the other roles, and the voice demo's answers.

## IR263 Units & locations in the display language and time zone; language specs restore in afterEach — 2026-10-10

`/customer/properties` (FR-C02, FR-C14, DD-C02, DD-C14, Figma Client 02a–02d) follows the maintenance screen (IR262).

1. **Texts.** These are translated:
   - the location tree: kinds, unit counts, unassigned units, the read-only note;
   - the property summary;
   - the unit rows;
   - the filtered list from the overview's counts (power and connection);
   - the rename dialog and its refusals;
   - group control: selection, change, review table, results, retry.
   Location and unit names are the customer's data and stay as written. The plan result of a group change (Will send / Clamped / Skipped / No change) stays the plan's code; only its badge is translated.
2. **Times.** These use the user's display time zone with the zone's abbreviation:
   - a unit's last-seen time and an offline AC's note in the group plan (time of day);
   - the property's last edit (`showDate`).
3. **Shared helpers.** `roomUnits`, `groupPlan` and `changeText` (`lib/clientProperties`) take the translator and the display. `nameError` and `reasonError` (`lib/assets`) take the translator; HQ's unit register still calls them in English. `clientProperties.test.ts` is new: room rows in both languages, the group plan's commands, clamping, restriction and skips, and the Malay plan.
4. **Language specs restore in afterEach.** The language E2E spec timed out on a wrong selector, and its `finally` block could not run: Playwright stops a timed-out test and closes the page. customer-a was left in Malay with Asia/Tokyo, which would have broken the English specs that ran after it. The data was reset by hand.
   - The display language and time zone are now set through `e2e/fixtures/display.ts`.
   - The two specs that change them (`customer/unit-language`, `shared/preferences`) put English and the earlier zone back at the end of the test, or in `test.afterEach` when the test fails. afterEach also runs after a timeout.
   Checked by forcing a 1.8 s timeout: the spec failed after switching to Malay, and the user came back as en / Asia/Kuala_Lumpur.
5. **Checked.**
   - Vitest: 34 files, 175 tests.
   - E2E: the language spec also opens units & locations in Malay. The whole suite: 54 passed, 9 skipped; the users are en / Asia/Kuala_Lumpur, and all 29 E2E jobs are cancelled.
   - A screenshot of the tree and a property in Malay shows no overflow.
6. **Still open.** Automations, energy, air quality, contracts & payments and users for the customer; the browser demo's screens; the other roles; the voice demo's answers.

## IR264 The customer's automations in the display language; the schedule preview's end time fixed — 2026-10-10

`/customer/automations` (FR-C04, FR-C05, DD-C04, DD-C05, Figma Client 03a–03i) follows units & locations (IR263).

1. **Texts.** These are translated:
   - the rule list: cards, status, the toggle and menu, the delete dialog;
   - the location consent card, full and compact;
   - the editor: the triggers, weekday buttons, times, presence / location / routine / weather fields, "Only if" rows, target AC and actions with their options;
   - the summary sentence, the next runs, the checks and the API's field errors;
   - the test dialog: the schedule test and the event test.
   Rule names are the customer's data and stay as written.
2. **Times.** A rule's runs stay in the rule's own time zone with its abbreviation: next run, last skip, schedule test and event test. A new rule takes its zone from Preferences, so with Asia/Tokyo saved its runs read "18:00 GMT+9". The consent card's grant and withdrawal show the IR44 date and time in the user's display time zone; the consent-withdrawn note on a rule shows the date.
3. **Fixed: the schedule preview's end time.** "Next runs" cut the end time from `runText(…).slice(-5)`. IR260 added the zone abbreviation to `runText`, so the end read "· 0 MYT". The end now comes from `runClock` ("22:00 MYT").
   `customer/automations.e2e.ts` is new and would have caught it. It creates a schedule, checks its preview "… 18:00 MYT · 22:00 MYT", saves it switched off (no command), checks the card and deletes it; a leftover is deleted in afterEach.
4. **Shared helpers.** These take the translator or the display: `actionOptions`, `draftErrors`, `apiErrors`, `summaryText`, `scheduleTest`, `eventTest` and `consentCard`, plus `runClock` (new). `dayShort` is exported for the weekday buttons. `scheduleTest` still finds the weekday index from the en-US short name; the labels follow the language.
5. **Checked.**
   - Vitest: 34 files, 176 tests. `clientAutomations.test.ts` checks the editor in both languages: summary, checks, field errors, options, the schedule test (start, end and the first day not selected), the event test, the consent card and `runClock`.
   - E2E: 55 passed, 9 skipped (+1 automations). The language spec also opens automations and the editor's next runs in Malay with Asia/Tokyo. The users are en / Asia/Kuala_Lumpur, all 31 E2E jobs are cancelled and no E2E automation is left.
   - A screenshot of the editor in Malay shows no overflow.
6. **Still open.** Energy, air quality, contracts & payments and users for the customer; the browser demo's screens; the other roles; the voice demo's answers.

## IR265 Energy & cost and carbon offsets in the display language; one default unit; rounding as written — 2026-10-10

`/customer/energy` and `/customer/energy/offsets` (FR-C06, FR-C13, FR-C16, DD-C06, DD-C13, DD-C16, Figma Client 04) follow automations (IR264).

1. **Texts.** These are translated:
   - the selection bar: periods, baseline, units and the comparison picker;
   - the four figures, the daily chart and table, the per-unit comparison, the warnings and the calculation conditions;
   - the carbon impact card;
   - the export dialog, its checks and the monthly e-mail notice;
   - the offsets page: the demo banner, the three steps, the quote, the confirmation, the records and the market concept.
   The IR68 wording follows the language: "Pengurangan 12.3 kWh", "Peningkatan 3.3%", "Tiada perubahan 0.0 kWh", "Tidak dapat dikira". Unit, location, baseline, tariff and factor names are data and stay as written. The suggested offset purpose is in the user's language ("Ofset demo") and is kept as typed.
2. **Times.** The business periods stay Kuala Lumpur days (REV18-035) and say so: the period label, the calculation conditions and the quote's period end with "Asia/Kuala_Lumpur", and the custom From/To fields are typed in Kuala Lumpur. Their dates and the daily chart's day names follow the language ("Isn 14"). A quote's expiry shows the IR44 date and time in the user's display time zone; a record shows its date (`showDate`).
3. **One default unit.** Without `unitIds`, Energy & cost took the first unit by name but Offsets took the API's first unit, so the two pages could open on different ACs (Bedroom AC and Meeting room AC for customer-a). Both now use `pickUnits`.
4. **Rounding as written.** `roundAway` multiplied before rounding, so 1.005 became 1.00 and 0.285 became 0.28. A value just below zero became −0, which Intl shows as "-0.0". It now rounds the decimal as written and never returns −0. One-decimal and whole-number results do not change: a scan of every value ending in 5 up to 20 000 found no difference.
5. **Shared helpers.** These take the translator or the display: `saving`, `warning` and `summaryView` (`lib/energy`), and `periodRange`, `dayRanges` and `compareRow` (`lib/clientEnergy`). HQ's energy, MRV and offsets screens still call them in English. `energy.test.ts` is new. It covers:
   - IR44 rounding and number formats;
   - the IR68 wording in both languages;
   - a summary and its conditions in both languages, and a summary without data;
   - the periods and their errors, and the days;
   - the comparison rows, `pickUnits` and the offsets query.
6. **Checked.**
   - Vitest: 35 files, 185 tests.
   - E2E: 55 passed, 9 skipped. The language spec also opens Energy & cost and Offsets in Malay with Asia/Tokyo and checks that the period stays "(n hari) · Asia/Kuala_Lumpur". After the default-unit fix the customer and scenario projects passed again (23 passed, 2 skipped). The users are en / Asia/Kuala_Lumpur, and all 37 E2E jobs are cancelled.
   - Screenshots of the energy page, the export dialog and Offsets in Malay show no overflow.
7. **Still open.** Air quality, contracts & payments and users for the customer; the browser demo's screens; the other roles; the voice demo's answers.

## IR266 Air quality in the display language and time zone; a hint the customer could not follow — 2026-10-10

`/customer/air-quality` (FR-C07, DD-C07, Figma Client 05a–05c) follows energy & cost (IR265).

1. **Texts.** These are translated:
   - the viewing bar: room and sensor choices, the update time, Refresh;
   - the metric cards (`metricCard`, IR260) and the metric, period and view tabs;
   - the allergen strip (IR98), the cleaning banner and the ventilation strip (IR99), also without CO2 and PM2.5 readings and without a fresh-air function;
   - the chart: title, sub line, axis, legend, the no-sensor and no-valid-reading states and the cut-off note (D07), and the table view;
   - the ventilation history and the Log ventilation dialog with its field errors and toasts.
   Room, property and unit names and the allergen data (substance, source, evidence) stay as written.
2. **Times.** Every instant is in the user's display time zone (NFR-08):
   - the update time and the readings' times show the zone's abbreviation (IR44);
   - the chart axis leaves it out, because the sub line names it;
   - each table row shows its slot as a span (`showSpan`);
   - the current CO2 and the history use `relativeTime` ("today …").
   The 7-day window still starts at 00:00 Kuala Lumpur six days earlier (IR41, SR17), and the sub line says so: "calendar days in Asia/Kuala_Lumpur from 8 Sept 2026 to 9:12 am MYT now".
3. **Fixed: a hint the customer could not follow.** For a unit outside any room, the screen said "assign this unit to a room in Units & locations". Client locations are read-only: HQ sets up properties, floors and rooms (2026-09-30). The hint now says "HQ sets up the rooms, so ask HQ to place this unit in one."
4. **Shared helpers.**
   - These take the translator or the display: `airRooms`, `ventStrip`, `cleanNote` (new, moved from the page), `allergenView`, `windowTitle`, `windowSub`, `axisLabels`, `airRows`, `co2Now` and `ventRow`. `showClock` replaces `updatedAt`.
   - `i18n.ts` adds `showDay` ("Mon 14"), which the energy chart's days also use, and lets `showClock` leave out the abbreviation.
   - `air.test.ts` adds five tests:
     - rooms, guidance and the cleaning note in both languages;
     - the allergen cases;
     - the 1-hour, 24-hour and 7-day labels and sub lines in Kuala Lumpur and Tokyo;
     - the table's spans and gaps;
     - the ventilation rows and the current CO2.
5. **Checked.**
   - Vitest: 35 files, 190 tests.
   - E2E: 55 passed, 9 skipped. The language spec also opens air quality in Malay with Asia/Tokyo:
     - the 24-hour window ends in GMT+9;
     - the 7-day window names Kuala Lumpur;
     - the Log ventilation dialog opens and is cancelled, because a log cannot be removed.
     The users are en / Asia/Kuala_Lumpur, and all 39 E2E jobs are cancelled.
   - Screenshots in Malay (24 hours, 7 days, the dialog) and a scrolled view show no overflow.
6. **Still open.**
   - For the customer: contracts & payments and users.
   - The browser demo's screens, the other roles and the voice demo's answers.
   - Shared by all four apps and still English: the failure toast of a Server Action (`actionMessage`) and each route's loading and error states (`RouteLoading`, `RouteError`).

## IR267 Contracts & payments and users in the display language; due dates stay Kuala Lumpur days — 2026-10-10

`/customer/payments`, `/customer/payments/[id]` (FR-C10–C12, DD-C10–C12) and `/customer/users` (FR-C19, DD-C19) follow air quality (IR266). With them, every customer screen shows the display language.

1. **Texts.** These are translated:
   - contracts: the list, plan names, period, scope and status badges;
   - invoices: the status filter, the rows, the empty states and the notes;
   - the invoice page:
     - the demo payment: methods; the processing, confirmed and failed states; the demo provider's events;
     - the payment history and the reminders;
     - the instructions and the message previews (email, WhatsApp);
   - the cooling-restriction notice and tab: reason, notice, start, units, per-unit apply and release states, timeline (IR42);
   - inquiries: the list, the form and the toasts;
   - users:
     - the table: role, status, last sign-in, channels;
     - the invite dialog with its checks and toasts;
     - the page shown to members.
   Unit and location names, contract IDs, invoice numbers and inquiry messages stay as written. The payment message preview is written in the recipient's language.
2. **Dates and times.**
   - Contract periods, invoice due dates and billing months are Kuala Lumpur business days (REV18-035), written in the user's language.
     - A due date at 23:59 Kuala Lumpur stays that day for a user in Tokyo, where the instant is already the next day.
     - When the display time zone is not Kuala Lumpur, the pages say so. The list shows "Due dates and contract periods are Kuala Lumpur dates (Asia/Kuala_Lumpur)."; the invoice shows "Kuala Lumpur date (Asia/Kuala_Lumpur)".
   - These are instants in the user's display time zone (IR44):
     - payments, inquiries, reminders and the restriction's notice day;
     - the restriction's events, its start and its release request;
     - invitations and last sign-ins.
3. **Status codes stay codes.** The per-unit release badge took its tone from its English text ("Released", "Release failed"); `noticeUnits` now returns `releaseTone`. Contract and invoice statuses stay the codes that the badges key on, and the view translates them.
4. **Shared helpers.** These take the translator or the display:
   - in `lib/clientBilling`: `contractCards`, `clientInvoiceRows`, `paymentLines`, `restrictionNotice`, `noticeUnits`, `noticeTimeline`, `inquiryLines` and `previewText`. `businessDay` and `billingMonth` are new; `longDay` is removed;
   - `policyText` (`lib/restrictions`), `clientUserRows` and `inviteError` (`lib/assets`).
   HQ's billing, restriction and users screens still call them in English with Kuala Lumpur times.
   `clientBilling.test.ts` is new (6 tests):
   - contract cards and invoice rows in both languages, with the Kuala Lumpur due day;
   - payment and event times in Tokyo;
   - the restriction notice per unit;
   - the message previews;
   - the users table for HQ and for the owner, and the invite checks.
5. **Fixed in the E2E suite.** `customer/filter-care.e2e.ts` read the reminder lines right after navigating, before the settings card had rendered. One run compared an empty snapshot and failed, although the settings were put back. The spec now waits for the card and fails on an empty snapshot.
6. **Checked.**
   - Vitest: 36 files, 196 tests.
   - E2E: 55 passed, 9 skipped. The language spec also opens contracts & payments, the newest invoice and users in Malay with Asia/Tokyo:
     - it checks the Kuala Lumpur date notes;
     - it opens the invite dialog and cancels it.
     The users are en / Asia/Kuala_Lumpur, all 44 E2E jobs are cancelled, and no invitation was left.
   - Screenshots of the list, the invoice and the restriction tab in Malay show no overflow.
7. **Found: the last sign-in is never recorded.** `clientUsers.list` reads `identity.users.last_sign_in_at`, but nothing writes it. "Last sign-in" therefore shows "—" for everyone, both on the customer's Users page and in HQ's Users tab. The access token carries `auth_time` (checked on the local Keycloak; Cognito sends it too). Recording it is planned for the next round.
8. **Still open.**
   - The browser demo's screens.
   - The partner, technician and HQ screens.
   - The voice demo's answers.
   - The shared failure toast and route states (IR266).

## IR268 The last sign-in is recorded from the token's sign-in time — 2026-10-10

IR267 found that `identity.users.last_sign_in_at` was read by `clientUsers.list` but never written. "Last sign-in" therefore showed "—" for everyone, on the customer's Users page (FR-C19) and in HQ's Users tab (FR-A17). The BFF signs users in (backend architecture §6) and never tells the Core API. The access token does say when its holder signed in: OIDC `auth_time`, sent by Cognito and checked on the local Keycloak.

1. **Rule.** When an authenticated request carries an `auth_time` later than the sign-in recorded for the user, identity sets:
   - `last_sign_in_at` to the business time now — in the demo that is the demo clock, so the time reads like the rest of the scenario;
   - `sign_in_auth_time` (new column) to that `auth_time`.
   These requests leave both columns alone:
   - more requests of the same sign-in;
   - the token of an older session on another device;
   - a token without `auth_time` (the static test tokens).
   A failure to record is logged and never fails the request. The time is informational and has no version or audit entry.
2. **Where.** The verifier returns the subject with the sign-in time (`auth.Identity`).
   - identity-api records it when it loads the principal.
   - The other services pass it on in their principal call (`signedIn`, RFC 3339), so identity-api records it there.
   - A principal served from another service's 30-second cache is not passed on; the next fetch passes it on.
   - Each process remembers the latest sign-in time it has recorded per user, so further requests of that sign-in cost no query. The database condition (`sign_in_auth_time < auth_time`) keeps replicas and restarts correct.
3. **Schema.** `identity.users.sign_in_auth_time timestamptz` (schema.sql = `000001_init.up.sql`, before the first release). The dev database `ac` got the column with `ALTER TABLE … ADD COLUMN IF NOT EXISTS` instead of a reset, so its data and demo clock stay. The test databases are rebuilt by `make test-all`.
4. **Checked.**
   - Go unit tests:
     - TestOIDCVerifier reads `auth_time` and ignores a value that is not a number;
     - TestRemoteSource passes the time on;
     - TestRecordSignIn covers a missing time, the first sign-in on the business clock, the same and an older sign-in in a process with nothing cached, a later one, and the middleware path.
   - Integration TestLastSignIn, in monolith and cluster mode:
     - the first sign-in shows in the Users list;
     - repeats, an older session and a token without a time do not move it;
     - a later sign-in moves it, directly on identity and through equipment-api's principal call.
   - `make test-all`: both suites pass (114 integration tests, 71 unit tests).
   - E2E: `customer/users.e2e.ts` is new (2 tests):
     - the owner's row shows the last sign-in in the IR44 format instead of "—";
     - the invite dialog refuses a bad address and an existing user in another letter case, and Cancel invites nobody.
     The suite: 57 passed, 9 skipped. After the run every user that signed in has a last sign-in in demo time; the preferences are en / Asia/Kuala_Lumpur, and all 47 E2E jobs are cancelled.

## IR269 The failed-action toast and the route states in the display language — 2026-10-10

These two parts are shared by the four apps; IR266 item 6 left them open.

1. **Failure toast.** `actionMessage` builds its line from sentence templates in the display language:
   - the field errors: "{field} is required", "{field} is out of range" and the others;
   - the version conflict;
   - the codes FORBIDDEN, NOT_FOUND, UNAVAILABLE, UNAUTHENTICATED and VALIDATION.
   `useAction` passes the user's translator, so on a translated screen every failed Server Action reads in Malay. Preferences, two-step verification and the energy export pass it too. HQ's billing, contracts and alerts screens and HQ's energy page call it without one and stay English.
   A field name ("Reason", from `reason`) and a domain message key ("Operation not running") stay humanized English: the API sends keys, not texts, and they have no fixed list.
2. **Route states.** `RouteLoading` and `RouteError` (each segment's `loading.tsx` and `error.tsx`) translate:
   - "Loading {what}…" and "{what} could not be loaded";
   - the reference line, "Please try again." and "Try again".
   `what` is a dictionary key. The 70 names of the four apps have Malay entries. The i18n key check now collects them from `<RouteLoading what="…">` and `<RouteError what="…">`, so a new route cannot miss its entry. On the partner, technician and HQ screens, these states show Malay before the screens themselves do.
3. **Checked.**
   - Vitest: 36 files, 197 tests. `actionMessage.test.ts` adds the Malay lines, and the key check covers the route names.
   - E2E: 57 passed, 9 skipped. The users are en / Asia/Kuala_Lumpur, and all 49 E2E jobs are cancelled.
4. **Still open.**
   - The partner, technician and HQ screens.
   - The browser demo's screens.
   - The assistant panel and the voice demo's answers.
   - The technician's QR dialog and device screens.

## IR270 The contractor overview in the display language — 2026-10-10

`/partner` (FR-P01, FR-P08, DD-P01, Figma Contractor 01-1…01-4) is the first partner screen.

1. **Texts.** These are translated:
   - the update line and the period choice;
   - the five KPI tiles;
   - job progress with its buckets;
   - Needs your action: offers, overdue windows, reports and unassigned jobs;
   - team capacity, today's timeline and the recent activity;
   - the empty states and notes.
   The job event titles share their entries with the customer's maintenance timeline. Unit, technician and qualification names stay as written.
2. **Times.**
   - The period (this week, last week, next week or the URL's dates) and today's 08:00–18:00 timeline are Kuala Lumpur days and hours. Their dates are written in the user's language. When the display time zone is another one, the timeline says "The timeline is in Kuala Lumpur time (Asia/Kuala_Lumpur)."
   - Instants use the display time zone with the abbreviation (IR44):
     - the update time;
     - an offer's answer-by time, an ended work window and a due time;
     - the next start ("starts today 10:00 am MYT");
     - the activity ("today …", `relativeTime`).
   - The English screen changes too: "09-20 17:00" now reads "20 Sept 2026, 5:00 pm MYT".
   - The loader formats the dates on the server. A browser's ICU can write a month differently ("Sept" / "Sep"), and the page would then not hydrate.
3. **Shared helpers.** These take the translator or the display:
   - `until`, which the job list, schedule, history and HQ also use; they keep English;
   - `kpis`, `actions`, `timeline` and `capacity`;
   - `activity`, which now also takes the business clock.
   The period words of the section titles are explicit dictionary keys, so the key check sees them.
4. **Checked.**
   - Vitest: 36 files, 198 tests. `partnerOverview.test.ts` adds the Malay / Tokyo case and the new English times.
   - E2E: `partner/overview-language.e2e.ts` is new. The suite: 58 passed, 9 skipped. The users are en / Asia/Kuala_Lumpur, and all 51 E2E jobs are cancelled.
   - A screenshot in Malay shows no overflow.
5. **Still open.**
   - The other partner screens: jobs, job detail, review, schedule, team, history, payouts and unit.
   - The technician and HQ screens.
   - The browser demo's screens.
   - The assistant panel and the voice demo's answers.

## IR271 The contractor job list in the display language — 2026-10-10

`/partner/jobs` (FR-P01, DD-P01 job list, Figma Contractor 02-1…02-6) follows the overview (IR270).

1. **Texts.** These are translated:
   - the status tabs;
   - the sort choices and the footer's sort text;
   - the period choice ("Period: This week (14 Sept – 20 Sept)");
   - the heading and the column heads;
   - every row: offer, accepted before the window, unassigned, assigned with the technician's acknowledgement, in progress, overdue, submitted, completed, on hold, rework, cancelled, history only;
   - the empty states, the restart banner and the paging.
2. **Times.**
   - A visit slot and an assigned slot show as one span in the display time zone ("22 Sept, 9:00 – 11:00 am MYT").
   - The answer-by, due, ended and completed times use IR44.
   - The period stays in Kuala Lumpur days. The overview and the list share `periodChoice`, which formats on the server.
3. **No English text as a key.** The line under a technician's name was coloured warn when the name was not "Unassigned". That compared English text, which Malay would break. Rows now carry `tech.subTone`, which is set for a technician who can't make the slot.
4. **Shared helpers.**
   - `jobRow` takes the display.
   - `slotText` keeps the Kuala Lumpur form for the schedule and the job detail until they are translated.
   - `periodChoice` and `klDay` move to `lib/partnerOverview`.
5. **Checked.**
   - Vitest: 36 files, 200 tests. `partnerJobs.test.ts` covers the new English times, the Malay rows and the period choice.
   - E2E: the partner language spec is now `partner/language.e2e.ts` and also opens the job list in Malay. The suite: 58 passed, 9 skipped. The users are en / Asia/Kuala_Lumpur, and all 53 E2E jobs are cancelled.
6. **Still open.**
   - The other partner screens: job detail, review, schedule, team, history, payouts and unit.
   - The technician and HQ screens.
   - The browser demo's screens.
   - The assistant panel and the voice demo's answers.

## IR272 The contractor's offer and job page in the display language — 2026-10-10

`/partner/jobs/[id]` (FR-P02, FR-P08, DD-P02, Figma Contractor 02-7…02-14) follows the job list (IR271). It covers the offer, the delegated job and the history after the delegation.

1. **Texts.** These are translated:
   - the offer: banners for a pending, approved or declined time change and for an accepted offer; the projection note; the facts; the decline reason and its checks; the decision buttons and toasts; the "Can your team take it?" card; the offer facts strip;
   - the decline and propose dialogs;
   - the delegated job: the state banner, the facts, the target unit (connection, alerts, scope), the status steps, the delegation countdown and "Who can take it";
   - the history snapshot;
   - the refusals of the decisions (`decisionRefusal`).
   A field in a refusal keeps its API key ("slot: …"). Unit names, addresses, entry instructions, models and the customer's request stay as written. The offer's ordinal reads "2nd" in English and "ke-2" in Malay.
2. **Times.** The loader formats every time on the server, and the view only translates fixed texts. A client view could otherwise write a month differently from the server, and the page would not hydrate (IR270).
   - Instants use the display time zone (IR44): the visit, access and scheduled spans (`showSpan`); the expiry, offered, access-start, decided, ended and completed times; each status step; the history rows; an alert's time; the unit's last-seen time.
   - "Can your team take it?" and "Who can take it" keep the capacity days as Kuala Lumpur dates in the user's language ("Sel 8 j", "8 j lapang 22 Sep – 23 Sep").
   - Propose another time is typed in the display time zone and says so ("times in Asia/Tokyo", NFR-08).
3. **Shared helpers.**
   - These take the translator or the display: `fits`, `timeline` (now with `atText`), `detailBanner`, `delegationLeft`, `decisionRefusal`, `eventRows`, `qualificationLabel`, `typeLabel` and `originLabel`.
   - `when` and `range` keep the Kuala Lumpur form for the schedule. HQ, the technician and the history call the labels in English.
   - `partnerSchedule` passed `qualificationLabel` straight to `map`, which would have handed it the index as the translator. It now calls it with the code only.
4. **Checked.**
   - Vitest: 36 files, 200 tests. `partnerJobDetail.test.ts` covers the new English times, the Malay fits, the banner, the countdown, the refusal and the timeline in Tokyo.
   - E2E: `partner/language.e2e.ts` also opens a job from the list in Malay. The suite: 58 passed, 9 skipped. The users are en / Asia/Kuala_Lumpur, and all 55 E2E jobs are cancelled.
   - The completed job's page in Malay with Asia/Tokyo shows no English except stored data.
5. **Still open.**
   - The other partner screens: review, schedule, team, history, payouts and unit.
   - The technician and HQ screens.
   - The browser demo's screens.
   - The assistant panel and the voice demo's answers.

## IR273 DEC-03 decided: the build's components, forms and dictionaries stand — 2026-10-10

DEC-03 was open since IR257. The SRC-02 phrase "shared libraries, reactForms" has no archived original. DEC-03 had proposed reading it as React Hook Form, with Zod, shadcn/ui, Lucide, TanStack Query and i18next. The build follows the Next.js guides instead, by the user's instruction of 2026-10-08.

1. **Decision.** On 2026-10-10 the product owner chose to keep the build's approach:
   - UI: in-house components on Tailwind CSS 4 (`shared/components/ui.tsx`, Figma UI Guideline), with glyph icons;
   - forms: controlled inputs in the shared Field, one pure validator per form in `shared/lib`, and the Core API's field errors from Server Actions;
   - translation: plain dictionaries chosen on the server (`shared/lib/i18n-ms.ts`, the Next.js internationalization guide);
   - data: Server Components through the DAL. There is no client query cache in API mode, and the Phase 1A demo keeps its in-browser Repository.
   No UI kit, icon set, or form, schema, query or translation library is added. An exception records the feature, why the in-house approach is insufficient, the cost and impact, and whether replacement is possible.
2. **Documents.** These now describe the build:
   - the UIUX specification: UX-01 libraries, UX-02 state ownership and the example, UX-03 forms, and the token mapping;
   - the four role designs (form rules);
   - common.md §1;
   - the implementation agent's library rule;
   - the design assumptions (DEC-03 DECIDED);
   - the production-instructions row and the reference analysis' icon row.
   Earlier IRs and review records keep their history.
3. **Code.** Nothing changes; the build already works this way. Vitest (200), E2E (58 passed, 9 skipped) and `make test-all` stay as they were in IR272.

## IR274 The contractor's quality review in the display language — 2026-10-10

`/partner/jobs/[id]/review` (FR-P05, DD-P05, Figma Contractor 03) follows the offer and job page (IR272).

1. **Texts.** These are translated:
   - the report: the inspection rows with their result badges and reasons, photos, work performed and the next action, parts and refrigerant, readings, time on site and the customer sign-off;
   - the quality review card with the IR31 note and the decision buttons;
   - the evidence check (rows, missing items, verdict);
   - the versions and linked alerts;
   - the return dialog, the banners, the refusals (CONFLICT, FORBIDDEN, NOT_FOUND) and the toasts.
   Part sources (van stock, HQ warehouse, bought locally) and leak checks (passed, failed, not done) get words instead of codes (`partLines`). Reasons, work text, notes and names stay as written.
2. **Times.** The loader formats every time on the server in the display time zone (IR44):
   - the submission and the sign-off;
   - the time on site as a span with its duration;
   - each version's review, an alert's time, the work window and the due time.
   A follow-up date stays the calendar date the technician picked.
3. **Shared helpers.** These take the translator or the display: `timeOnSiteText`, `evidenceCheck`, `availabilityText`, `versionRows`, `alertRows` and `dueRow`; `partLines` is new. Only the review uses them, so their English output moves to IR44 too ("v1 · returned 14 Sept 2026, 8:30 am MYT").
4. **Checked.**
   - Vitest: 36 files, 201 tests. `partnerReview.test.ts` covers the new English times and the Malay evidence check, availability, versions, alerts, time on site and parts.
   - E2E: `partner/language.e2e.ts` also opens a completed job's review in Malay. The suite: 58 passed, 9 skipped. The users are en / Asia/Kuala_Lumpur, and all 57 E2E jobs are cancelled.
5. **Still open.**
   - The other partner screens: schedule, team, history, payouts and unit.
   - The technician and HQ screens.
   - The browser demo's screens.
   - The assistant panel and the voice demo's answers.

## IR275 The contractor's schedule in the display language — 2026-10-10

`/partner/schedule` (FR-P03, DD-P03, Figma Contractor 03-1…03-10) follows the quality review (IR274).

1. **Texts.** These are translated:
   - the accepted jobs with their state lines and the sort, and the delegation windows;
   - the assign form: titles, banners (cannot make it, work window ended, not yet delegated, nothing to schedule), the technician choices with their badges, the fixed visit time or the kept start and the new end, the reason, the warnings and the buttons;
   - the team's week with its legend and notes;
   - the refusals of `jobs.assign`, the toasts and the confirmation.
   Validation refusals name the field ("Reason: required (1–1000 characters)"). Qualifications, unavailability types (annual leave, sick leave, training, public holiday, other) and job states are words (`shortQualification`, `unavailabilityLabel`, `statusWord`); the overview's timeline uses the first two as well, so it no longer shows codes. Names, IDs and reasons stay as written.
2. **Times.** The loader formats the job's instants on the server in the display time zone (IR44):
   - the delegation windows and when an accepted offer's delegation starts;
   - the agreed visit time, the current slot and the kept start;
   - the technician's alternative time.
   The Server Action returns the saved slot formatted the same way for the confirmation. While the work is under way, the new end is typed in the display time zone (`zonedInstant`, NFR-08). The team's week and the candidates' free hours stay on Kuala Lumpur working days and hours, and the screen says so. The week marks today from the demo clock instead of the browser's clock.
3. **Shared helpers.** These take the translator or the display: `scheduleRow`, `formMode`, `freeText`, `candidates`, `weekGrid` and `assignRefusal`. Their English output moves to IR44 too ("Accepted · delegation starts 22 Sept 2026, 8:00 am MYT"). `freeText` reads "free 09:00–13:00", "no hours" or "fully booked". A row says whether its window ended (`ended`) instead of the view comparing the English "Ended".
4. **Checked.**
   - Vitest: 36 files, 202 tests. `partnerSchedule.test.ts` covers the new English times and the Malay rows, form modes, candidates, week cells and refusals.
   - E2E: `partner/language.e2e.ts` also opens the schedule in Malay. The suite: 58 passed, 9 skipped. The users are en / Asia/Kuala_Lumpur, and all 62 E2E jobs are cancelled.
   - A visual check on an accepted job in Malay with Asia/Tokyo: the windows and the visit time in GMT+9, the week in Kuala Lumpur hours with "Cadangan ini" on the visit day.
5. **Still open.**
   - The other partner screens: team, history, payouts and unit.
   - The technician and HQ screens.
   - The browser demo's screens.
   - The assistant panel and the voice demo's answers.

## IR276 The contractor's team & capacity as in Figma, in the display language — 2026-10-10

`/partner/team` (FR-P06, DD-P06, SCR-P06) followed an early layout in API mode. It now follows Figma Contractor 04-1…04-5 and 04-8.

1. **Conditions in the URL.** `date` is a Kuala Lumpur day; the default is today on the demo clock. `qualification` is a register code. `activeOnly` defaults to true, and its button reads "Include expired" when off. `orgId` names the own company; any other value shows "Roster not found" (AT-P06-E①).
2. **Technicians.** Each row shows:
   - the qualifications and the membership ("active since Apr 2025", "expired 1 Aug 2026");
   - the week's assigned and available hours with a bar;
   - the chosen day: the booked blocks with their job ("10:00–12:00 assigned (b5f000c1)"), the free time and the day's utilization.
   A day without configured hours shows "No available hours set" and "—". A day off shows the unavailability. An expired membership shows "Expired — cannot be assigned".
   A qualification nobody holds on that day shows the empty state with "Clear filter".
3. **The week and the grants.**
   - The week grid shows "2 / 8 h" per technician and day, marks the chosen day and names a chosen Saturday or Sunday in its title.
   - Its totals are assigned hours, available hours, team utilization and the chosen day's free hours.
   - The Qualifications card lists each grant: valid, expiring within 30 days (IR133 item 2), expired or revoked. A register qualification that no listed technician holds shows "not held". The note counts the expired members that Active only hides.
4. **Unavailable days (04-8).**
   - The form takes a technician or all technicians (public holiday), the first and last Kuala Lumpur day, the type and a note.
   - Before saving, it lists the confirmed assignments those days overlap, from the technicians' booked jobs.
   - Its own checks are the dates, at most 31 days and a note of at most 500 characters (`unavailabilityErrors`). The Core API's field errors become readable text (`unavailabilityRefusal`).
   - The confirmation counts the assignments the Core API kept (`conflictingAssignmentIds`).
   - "Open schedule →" opens the first overlapping job.
5. **The own company.** `session.get` returns `organizationId` (Session in service-contracts.ts, Swagger regenerated), so the screen can tell its own company from another one. `corePrincipal` exposes it.
6. **Language and time.**
   - Every text is translated (77 entries).
   - Capacity is cut on Kuala Lumpur working days, and so are unavailable days; the screen says so in another display time zone.
   - An overlapping assignment's time is an instant, shown in the display time zone.
   - Grant and membership dates are Kuala Lumpur days in the user's language.
7. **Shared helpers.** `shared/lib/partnerTeam.ts` is new: `teamQuery`, `listed`, `memberRows`, `weekRows`, `teamStats`, `grantRows`, `unavailabilityErrors`, `unavailabilityConflicts` and `unavailabilityRefusal`. `partnerSchedule.freeSlots` is split out of `freeText`.
8. **Still to do.**
   - The Certifications tab still lists the qualification grants. Figma 04-6/04-7 (certificates.list, renewal upload, training request, assignment impact) comes next.
   - The browser demo keeps its fixture screen.
9. **Checked.**
   - Vitest: 37 files, 210 tests. `partnerTeam.test.ts` is new, with Malay cases.
   - Go: `make test-all` passes. The session tests check `organizationId`.
   - E2E: `partner/team.e2e.ts` is new and covers the technicians, week and grants, a Saturday, another company's roster and the form's day checks. Nothing is saved, because unavailable days have no undo. `partner/language.e2e.ts` also opens the team in Malay. The suite: 62 passed, 9 skipped. The users are en / Asia/Kuala_Lumpur, all 65 E2E jobs are cancelled and no unavailable days exist.
   - A visual check with an accepted and assigned job showed the booked block, the week and the overlap warning. Nothing was saved, and the job was cancelled.

## IR277 The contractor's certifications as in Figma, in the display language — 2026-10-10

The Certifications tab of `/partner/team` (FR-P09, DD-P09, Figma Contractor 04-6/04-7) listed the technicians' qualification grants (IR276). It now reads `certificates.list`.

1. **Conditions in the URL.** `membershipId`, `status` (valid, expiring, expired, pending, rejected, not held) and `expiringWithinDays` (30 / 60 / 90, default 60) join the SCR-P06 URL keys (screen catalog).
2. **Rows.** There is one row per technician and qualification:
   - the certificate on file: its latest one, with a pending renewal noted;
   - else the HQ grant without a certificate on file — also while a first upload waits for HQ, or after HQ rejected it, because the grant still decides eligibility (BR-P09);
   - else "not held", when a booked job's unit scope needs the qualification (IR123 item 3).
   Within the window a certificate is "Expiring · N d". It blocks the technician's booked jobs that end after it ("job-p07 (20 Oct)").
   A certificate keeps the name it was uploaded with; grants and missing qualifications use the qualification's name.
3. **KPIs and the assignment impact.**
   - The KPIs count Valid (in date), Expiring ≤ N days with the soonest named, Expired, and Pending HQ verification.
   - The impact card names the first job that an expiring, expired or missing qualification blocks, with "Open assignment →".
4. **Forms.**
   - **Upload renewal** sends `renewalOf`. A grant or a missing qualification gets **Add certificate** (technician, qualification, name).
   - Both forms take Issued on and Expires on as Kuala Lumpur days (the start of those days), a number of 1–64 characters, and a PDF, JPG or PNG of at most 10 MB (`certificateErrors`).
   - The file goes as form data to a Server Action, which sends it base64. The partner app's Server Action body limit is 11 MB.
   - **Request training** (note 1–1000, the certificate's version as the expected version) needs a certificate on file.
   - Only an active technician can get a new certificate. The Core API's refusals become readable text (`certificateRefusal`).
5. **Fix: certificate files above 1 MiB.** `certificates.submit` had the ordinary 1 MiB body limit, so a file larger than about 750 KB was refused with `error.bodyTooLarge` before the IR133 10 MB check. It now has a 16 MiB limit, like `reports.signOff`. A file over 10 MB is a field error (`file: error.invalidFile`).
6. **Language.** Every text is translated; certificate dates are Kuala Lumpur days in the user's language.
7. **Shared helpers.** `shared/lib/partnerCertificates.ts` is new: `certQuery`, `certRows`, `certKpis`, `impact`, `certificateErrors`, `certDates`, `certificateRefusal` and `fileSize`. The shared `ApiCertificate` type gains `verifiedAt` and `trainingRequestedAt`.
8. **Checked.**
   - Vitest: 38 files, 217 tests. `partnerCertificates.test.ts` is new, with Malay cases.
   - Go: `make test-all` passes. `TestBodyLimits` covers a 10 MB certificate in JSON. The integration test sends a 3 MiB scan and a file over 10 MB.
   - E2E: `partner/team.e2e.ts` adds the Certifications tab (KPIs, the window in the URL, the form's checks); nothing is uploaded. `partner/language.e2e.ts` also opens the tab in Malay.
   - The first E2E run failed both new checks: they matched a hidden status `<option>` with the same text. Each KPI tile is now a named region. The suite then passed 63 tests and skipped 9. The users are en / Asia/Kuala_Lumpur, all 69 E2E jobs are cancelled and no unavailable days exist.
   - A visual flow uploaded a 1.5 MB PDF from the tab. The row turned "Pending HQ verification", and HQ rejected it in Verify uploads. The dev database keeps that one rejected certificate (EB-2026-0931).

## IR278 The contractor's job history in the display language — 2026-10-10

`/partner/history` (FR-P07, FR-P08, DD-P07, Figma Contractor 05-1…05-6) already matched Figma (IR228). It now follows the display language and time zone, like the other contractor screens (IR270–IR277).

1. **Texts.** These are translated:
   - the list: tabs, sort, period, table headings and rows, empty states, footer;
   - the detail: summary, event filters, timeline, visibility badges, previews card;
   - the communication form: labels, hints, errors, buttons, results and refusals.
   Template, role and channel codes stay as Figma shows them (`schedule_change`, `hq`, `inApp`). Notes, names and IDs stay as written. The refusal names the field ("Recipient: …").
2. **Times.** These are formatted on the server in the display time zone (IR44):
   - the latest event reads relative to now ("today 9:05 am MYT", `relativeTime`);
   - the timeline's events and the delegation's end use `showTime`;
   - the Server Action returns the preview's time formatted the same way.
   The list and the form no longer format times in the browser.
3. **Shared helpers.** These take the translator or the display: `historyRow`, `eventItem`, `byUser`, `channelText`, `noteMode` and `communicationRefusal`. Their English output moves to IR44. A row says whether it is overdue (`overdue`) instead of the view reading the badge's tone.
4. **Checked.**
   - Vitest: 38 files, 218 tests. `partnerHistory.test.ts` covers the new English times and adds a Malay case.
   - E2E: `partner/language.e2e.ts` also opens the history list and a job's history in Malay. The suite: 63 passed, 9 skipped. The users are en / Asia/Kuala_Lumpur, and all 71 E2E jobs are cancelled.
5. **Still open.**
   - The other partner screens: payouts and the unit.
   - The technician and HQ screens.
   - The browser demo's screens.
   - The assistant panel and the voice demo's answers.

## IR279 The contractor's payouts in the display language — 2026-10-10

`/partner/payouts` (FR-P10, DD-P10, Figma Contractor 06-1/06-2) already matched Figma. It now follows the display language and time zone.

1. **Texts.** These are translated:
   - the filters, the KPIs and the statements list with its empty states;
   - the lines table: work, status, actions and "Not included · next statement";
   - the totals, notes and questions to HQ;
   - the Ask HQ dialog with its refusals and toasts;
   - the one-page statement PDF.
   Line statuses are codes (`accepted`, `deduction`, `adjustment`, `not_included`) with a translated `statusText`, so the view never compares translated text.
2. **Dates.**
   - A period reads as a month in the user's language ("Sept 2026", "Sep 2026").
   - The pay date is a Kuala Lumpur business day: HQ pays on the 15th (`businessDay`).
   - When a report was accepted and when a statement was paid are moments, shown as dates in the user's display time zone (`showDayMonth`, a new helper — "15 Sept" — and `showDate`).
   - The loader formats the totals, the statement title and its subtitle; the view formats nothing.
3. **Shared helpers.** These take the translator or the display: `periodLabel`, `statementRows`, `kpis`, `lineRows`, `questionRows` and `payoutPdf`. Amounts keep the Malaysian number format with the currency code.
4. **Checked.**
   - Vitest: 38 files, 219 tests. `partnerPayouts.test.ts` covers the new English dates and adds a Malay case, the PDF included.
   - E2E: `partner/language.e2e.ts` also opens the payouts in Malay. The dev database has no approved statement for Contractor A, so the E2E sees the empty state; the unit tests cover the statement view.
   - The suite: 63 passed, 9 skipped. The users are en / Asia/Kuala_Lumpur, and the E2E jobs are cancelled.
5. **Still open.**
   - The partner unit screen.
   - The technician and HQ screens.
   - The browser demo's screens.
   - The assistant panel and the voice demo's answers.

## IR280 The contractor's unit view as in Figma, in the display language — 2026-10-10

`/partner/units/[id]` (FR-P04, FR-P08, DD-P04, Figma Contractor Unit View and "delegation ended while open") showed a plain read of the unit in API mode. It now follows Figma.

1. **The view.**
   - A banner names the job the view is for.
   - The left column has the unit register (location, model, maintenance scope, connection, last seen), the job context and the company's past work on the unit. The job context shows the job and type, the technician or "Not assigned yet", and the access window with its elapsed share and when access ends.
   - The right column has the alert evidence, the latest readings with their quality in words ("good", "stale", "suspect") and the power state, and the last 24 hours of two diagnosis metrics in 12 two-hour buckets.
   - Nothing on the page controls the unit.
2. **After the delegation.**
   - When `units.get` refuses the unit and the URL's `jobId` names one of the company's history snapshots, the page shows the snapshot: the company's decision, the work, the report, and "Live unit values: Not available after the period".
   - Otherwise the unit is not found, as before (IR169).
3. **Language and time.**
   - Every text is translated (44 entries).
   - Every time is formatted on the server in the display time zone, the chart labels included.
4. **Shared helpers.** `shared/lib/partnerUnit.ts` is new: `registerRows`, `contextJob`, `jobContext`, `pastWork`, `evidenceRows`, `readingRows`, `chartMetrics`, `chart` and `snapshotRows`.
5. **Checked.**
   - Vitest: 39 files, 223 tests. `partnerUnit.test.ts` is new, with Malay cases.
   - E2E: the cross-app scenario now opens the unit from the partner's job page and checks the diagnosis-only view with no control buttons. The suite: 63 passed, 9 skipped. The users are en / Asia/Kuala_Lumpur, and all 75 E2E jobs are cancelled.
   - A visual check in English and Malay on a delegated unit showed the register, the job context, the alert evidence and the 24-hour temperature and power charts.
6. **Contractor screens.** Every contractor screen now follows the display language: overview, jobs, offer / job, quality review, schedule, team & capacity with certifications, job history, payouts and the unit.
7. **Still open.**
   - The technician and HQ screens.
   - The browser demo's screens.
   - The assistant panel and the voice demo's answers.

## IR281 The technician overview in the display language — 2026-10-10

`/technician` (FR-T01, DD-T01, Figma Technician 01-1…01-5) is the first technician screen to follow the display language and time zone.

1. **Texts.** These are translated:
   - the tiles, the tabs and the sort, and the job rows (badge, what to do now, the alert on the unit);
   - the assignment banners and the read-only banners;
   - the cards for today, alerts on my units and my reports;
   - the empty states.
2. **Times.**
   - A job's work window is an instant span in the display time zone (`showSpan`).
   - "Opens …" reads relative to now ("opens today 2:00 pm MYT", `relativeTime`).
   - An alert's time uses `showTime`.
   - Today and its 08:00–18:00 timeline stay Kuala Lumpur, where the work happens. The card says so in another display time zone, and the now line shows Kuala Lumpur time.
   - The loader formats the day title, so the view no longer formats times itself.
3. **Shared helpers.** These take the translator or the display: `techRow`, `kpis`, `alertRows` and `reportRows`. Their English moves to IR44. `windowText` keeps the Kuala Lumpur form for the timeline and the QR scan screen (next).
4. **Checked.**
   - Vitest: 39 files, 224 tests. `techOverview.test.ts` covers the new English times and adds a Malay case.
   - E2E: `technician/language.e2e.ts` is new. It sets Malay and Asia/Tokyo, opens the overview (tiles, tabs, sort, cards, the Kuala Lumpur note and the All assigned tab) and puts English back. The suite: 64 passed, 9 skipped, and the users are en / Asia/Kuala_Lumpur.
5. **Still open.**
   - The other technician screens: the job workspace, the unit with its alerts and control, the devices and the QR scan.
   - The HQ screens.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

## IR282 The technician job workspace in the display language — 2026-10-10

`/technician/jobs/[id]` (SCR-T04, FR-T04–T06, T08, T09, T13–T15, Figma Technician 02) follows the display language and time zone.

1. **Texts.** These are translated:
   - the header, the assignment, window, hold, submitted, completed, rework and refusal banners;
   - the checklist (groups, components, results, reasons and the submit checks), the checklist progress and the job & unit card;
   - the readings, parts & refrigerant and time-on-site tabs, the work report with its photos, next action and sign-off, and the versions & autosave card;
   - the check-in, can't-make, sign-off, part, refrigerant and submit dialogs, and the toasts;
   - a refused write in words (FORBIDDEN: the window has not started or has ended, not assigned, a QR label of another unit; CONFLICT; UNAVAILABLE), the codes kept in front;
   - the history card of a job whose viewing window ended (IR49 / IR124).
   The dictionary gains 241 entries.
2. **Times.**
   - The work window is an instant span in the display time zone (`showSpan`); "starts" and "ended" use `showTime`, the IR89 warning, the times on site, a reading's time and the sign-off use `showClock`.
   - A follow-up visit date stays the Kuala Lumpur calendar day the technician picked (`businessDay`); a new time typed in the can't-make dialog is read in the display time zone (`zonedInstant`).
   - The loader formats every instant of the first render on the server (`knownOf`: clocks, stamps and spans keyed by the ISO string). The view's formatter (`fmtOf`) reads them, so the first render matches the server's ("Sept" in Node and the browser alike), and formats in the browser only what appears after the page loaded — a new reading, a save, a sign-off.
3. **Shared helpers.** These take the translator or the formatter: `submitIssues`, `progress`, `windowState`, `timeRows` and `versionRows`; `followUpText` and `historyCard` are new. Their English moves to IR44.
4. **Checked.**
   - Vitest: 39 files, 226 tests. `techJob.test.ts` covers the new English times and adds a case for the server's formatting and Malay.
   - E2E: `technician/language.e2e.ts` also opens an assigned job from the All assigned tab and checks the Malay headings, the back link and the window in GMT+9. The suite: 64 passed, 9 skipped, and the users are en / Asia/Kuala_Lumpur.
5. **Still open.**
   - The other technician screens: the unit with its alerts and control, the devices and the QR scan.
   - The HQ screens.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

## IR283 The technician's unit screen as in Figma, in the display language — 2026-10-10

`/technician/units/[id]` (SCR-T02, FR-T02, FR-T03, DD-T02, DD-T03, Figma Technician 02-10…02-13) is rebuilt as in Figma and follows the display language and time zone.

1. **Register tab.**
   - The unit register: location, manufacturer / model, configuration (the unit type), installed, capability version and maintenance scope, with the access instructions when the unit has them. A missing value says "Not registered" and the card says it is never filled from a similar model or today's date (DD-T02).
   - The note "18 components across 3 groups (8 indoor, 5 outdoor, 5 electrical)", and the buttons to the alert evidence and to diagnostic control. Control opens only for an active job of the technician on the unit: in progress first, then assigned or returned for rework (IR94).
   - The components of each group in the service scope (IR100). Each gets its result in the job workspace.
   - Time-series monitoring: temperature, power and connection with their times, and the chart of the period.
   - Maintenance history: the technician's own jobs on the unit. Current visits come first with their window, then ended jobs with their completion day.
   - Open alerts: worst and newest first, with the evidence, the detection time and who acknowledged them. Finishing the job does not resolve an alert.
2. **Monitoring tab.**
   - The metric's chart with four tiles: temperature, power, operation and connection.
   - Up to three other metrics, each with its latest value and the period's slots. One click charts that metric.
   - The events of the period: the unit's alert raises, acknowledgements and resolutions, and its device's connection, power and tamper events (`devices.list` → `devices.events` with from / to).
3. **URL and periods (DD-T03).**
   - `metric` opens Monitoring, `period` is 1h / 24h / 7d (24h by default), and `jobId` names the job the page was opened for: the back link, and the job the alert evidence opens with. The job workspace's "Unit →" now passes it.
   - 1 h and 24 h roll back from now. 7 days are the Kuala Lumpur calendar days up to today, and the title says so.
   - The chart has 12 slots, each with the latest valid reading. Stale, suspect and missing readings are not plotted, and the empty slots between readings are named as a gap that stays unconnected.
4. **Offline.** While the unit is not online, the values say "last known · observed …". The banner says when updates stopped and that nothing on the page is real-time (DD-T03).
5. **Before the work window (IR76).** `units.get` answers `errors.assignment_not_started`. The page then reads the URL's job (`jobs.get`) and says when the AC can be seen, with a link to the job. Without the job it gives only the general sentence.
6. **Texts and times.**
   - Every text is translated; the dictionary gains 78 entries.
   - Instants use the display time zone with "today / yesterday" (`relativeTime`). Visit windows are spans (`showSpan`); the start of the 7-day period is a Kuala Lumpur day.
   - The loader formats everything, so the view only renders.
   - New pure code is `shared/lib/techUnit.ts`. The fixture view moves to `unit-demo.tsx` and stays English (Phase 1A).
7. **Catalog.**
   - SCR-T02 now lists the reads the screen makes: units.get, alerts.list, jobs.list, jobs.get, devices.list, devices.events, telemetry.series and telemetry.summary. Its URL keys are jobId, metric and period.
   - `jobs.list` is a lookup there (no sort; the validator's job lookups gain SCR-T02).
   - DD-T02 gains alerts.list, jobs.list and jobs.get; DD-T03 gains devices.events. The operation catalog, the Go catalog and the API description carry the new design ids.
8. **Checked.**
   - Vitest: 40 files, 232 tests. `techUnit.test.ts` is new: the register with missing values, the tiles online and offline, the 1 h / 24 h / 7 d periods with a gap and a suspect reading, the events, the jobs, the control job, the open alerts, and Malay with Asia/Tokyo.
   - E2E: `technician/language.e2e.ts` opens the job's unit in Malay. It checks the register cards, Monitoring with the 24-hour title and the events, and the 7-day period in the URL. The suite: 64 passed, 9 skipped, and the users are en / Asia/Kuala_Lumpur.
9. **Still open.**
   - Figma's "last inspection" per component group (the results of the latest accepted report). No read gives it to the technician yet.
   - Figma's alert threshold line on the chart. The technician cannot read alert policies; the alert evidence screen raises the same gap (next).
   - The other technician screens: the alert evidence, diagnostic control, the devices and the QR scan.
   - The HQ screens.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

## IR284 The technician's alert evidence as in Figma; the condition on every alert; acknowledging with alert.read — 2026-10-10

`/technician/units/[id]/alerts` (SCR-T07, FR-T07, DD-T07, Figma Technician 02-14…02-17) is rebuilt as in Figma and follows the display language and time zone. Two Core API gaps behind Figma close with it.

1. **`Alert.rule` (API).**
   - Every Alert now carries `rule: AlertRule | null`: name, metric, operator, threshold, recovery threshold and duration of the condition that raised it.
   - It is read with the alert: the policy's own condition for kind alert, or the rule of `ruleKey` in the default policy. It is null for an alert without a policy (device, maintenance, reconciliation).
   - Why: Figma shows the policy line and the recovery rule to technicians. Policy reads stay with policy permission holders (SCR-T02 query rule), so the alert itself carries what raised it.
   - Every reader of alerts gets it: clients, contractors inside the access window, technicians and HQ. There is no schema change; it is a read-time join inside the monitoring domain.
2. **Acknowledge with `alert.read` (API).**
   - `alerts.acknowledge` for technicians is now `technician:alert.read:assigned` (it was `technician:alert.resolve:assigned`). `alerts.resolve` still needs `alert.resolve`.
   - Both still follow the IR94 write table: an active assignment on the unit, inside the work window.
   - Why: DD-T07 says acknowledgement only marks the alert as seen, and resolution needs alert.resolve. Figma 02-15 / 02-16 shows a technician without alert.resolve acknowledging and then refused when resolving. Figma is the final spec (2026-10-01) and later than the catalog row.
3. **The screen.**
   - The banner: the alert and its rule's name, or its title. Below it the policy line ("Policy: Temperature ≥ 30.0 °C, recovery < 28.0 °C, duration 60 s"), or that it has no policy and resolves only with a reason (IR66). On the right, Acknowledge while open, else the state and its time.
   - The evidence: the evidence at detection (kind, text, observed), then the unit's latest reading of the rule's metric. That reading gives its origin and quality, and "confidence unknown" for an estimate (BR-T07).
   - Resolution:
     - What resolves the alert: a sustained remeasurement for a policy alert (D08), else only a reason (IR66). Then the reason field and Resolve alert.
     - A resolved alert says when and why, and when it was acknowledged.
     - A refused write is shown in words. Without alert.resolve it says how the alert can still resolve.
   - Alert history: resolved, acknowledged and raised, newest first.
   - Related: the URL's job, the unit with its place, the policy, the recovery rule, the previous alert, and "You can resolve" (with a reason; with a remeasurement only; or not without alert.resolve).
   - The unit's other alerts to switch to. `alertId` in the URL; by default the worst unresolved one.
   - Links now name the alert: the overview's alert rows and each open alert on the unit screen. The back link keeps the job.
   - Before the work window the page says when it opens (IR76, `jobs.get`).
4. **Texts and times.**
   - Every text is translated; the dictionary gains 53 entries.
   - Times use the display time zone with "today / yesterday".
   - The loader formats everything. New pure code is `shared/lib/techAlerts.ts`.
5. **Catalog.**
   - SCR-T07 adds units.get and jobs.get and the URL key alertId.
   - DD-T07 adds units.get and jobs.get, and an as-built note.
   - The operation catalog (design ids, the acknowledge authorization), `service-contracts.ts` (AlertRule), the Go catalog and the API description are regenerated.
6. **Checked.**
   - Go: `TestAlertRuleAndTechnicianAcknowledge`. It covers the rule of a policy alert (HQ and the client), of a default rule, and none without a policy, in alerts.get and alerts.list. A technician on the unit without alert.resolve acknowledges, is refused when resolving (403), and resolves once the permission is back. Both suites pass.
   - Vitest: 41 files, 237 tests. `techAlerts.test.ts` is new: rule words and durations, an open policy alert without alert.resolve, a resolved alert without a policy, the selection, refusals and Malay. The overview's alert links are updated.
   - E2E: `technician/language.e2e.ts` opens the unit's alert evidence in Malay without acknowledging or resolving. The suite: 64 passed, 9 skipped; the users are en / Asia/Kuala_Lumpur, and the dev alerts are unchanged.
7. **Still open.**
   - D08 threshold evaluation on telemetry. The Core API opens policy alerts from demo events, but it does not yet resolve them after a sustained recovery, so Figma's "resolved after remeasurement" (02-17) needs it. That is next.
   - Who acknowledged or resolved an alert. The columns are kept, but Alert does not carry the actor.
   - Figma's measured / estimated / inspection evidence trio. It needs the evidence records behind `evidenceIds`; the screen shows the alert's evidence and the latest reading.
   - The other technician screens: diagnostic control, the devices and the QR scan.
   - The HQ screens.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

## IR285 Sustained recovery resolves policy alerts; recurrences link to the alert before — 2026-10-10

IR284 left one D08 rule unbuilt: the Core API opened policy alerts from the demo evaluation, but nothing resolved them when the reading recovered. Figma 02-17, "resolved after remeasurement", needs it. IR66's recurrence link was missing too.

1. **Recovery (D08).**
   - In `automations.fire`, the notifier checks every condition of the unit's alert policies, the attached ones and the default rules enabled for the customer.
   - When the condition's metric has a fresh, valid reading past the recovery threshold, the policy's open or acknowledged alert on the unit is resolved. Past means below it for a high limit and above it for a low one. Fresh means valid quality, observed within 120 s, and not after the tick.
   - As with a breach, the demo evaluation takes the reading as sustained for `durationSeconds`.
   - It applies whether the policy is enabled or not and outside its active window: recovery is a fact about the unit, not about notifying.
   - An alert detected after the tick is left alone.
2. **Evidence.**
   - `resolutionReason` names the reading, for example "Recovered: co2 850 < 900 for 60 s (D08)".
   - `resolutionEvidenceIds` holds the evaluation event id: the fired facts that recovered. A replay of that event returns the stored result (D02) and resolves nothing again.
   - Each resolution emits `AlertResolved` with `recovered: true`.
   - `automations.simulate` saves nothing.
3. **Limits (IR66).** Only alerts with a policy resolve this way. Device, maintenance, reconciliation and other alerts without a policy still need a reason from an alert.resolve holder. A reading between the two thresholds, or of suspect, missing or stale quality, resolves nothing.
4. **Recurrence (IR66).** A breach after the alert resolved opens a new source alert. Its `previousAlertId` is the latest earlier alert of the same policy and rule on the unit. The alert evidence shows it under Related as the previous alert.
5. **Checked.**
   - Go: `TestAlertRecoveryResolvesPolicyAlerts`. A breach opens the policy's and the default rule's alerts. These resolve nothing: a reading between the thresholds, a suspect one, and a simulation. A valid recovery resolves both alerts with the reason and the event. A policy-free tamper alert stays open. The next breach opens a new alert linked to the first, also seen in alerts.get. Both suites pass, and the dev containers run it.
   - No web change. The technician's alert evidence shows the resolved alert, its reason and the link.
6. **Still open.**
   - The continuous D08 evaluation of stored telemetry at one-second ticks, where the counter resets on poor quality. The demo evaluation still decides on fired facts.
   - The actor of an acknowledgement or resolution on Alert.
   - The diagnostic control, devices and QR-scan screens in Malay.
   - The HQ screens.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

## IR286 The technician's diagnostic control in the display language — 2026-10-10

`/technician/units/[id]/control` (SCR-T10, FR-T10, DD-T10, Figma Technician 02-18…02-28) already had Figma's layout: normal diagnostic action, current unit state, the job's command history, test run, authorization and the confirm dialog. It now follows the display language and time zone.

1. **Texts.** These are translated:
   - the cards and fields, the banners (restriction, blocked control, outside the work window), the empty states (no control.diagnose, no job) and the confirm dialog;
   - the toasts and the inline validation;
   - the test-run banner of every DiagnosticRun state (awaiting start, running, end requested, completed, start failed, end failed, end blocked);
   - the command history (test-run start and end, the device outcome, the badges);
   - the state tiles, and the capability, restriction and connection text;
   - a refused write in words (restriction, busy, reconciliation, outside the window, OFFLINE, FORBIDDEN, CONFLICT, UNAVAILABLE). A VALIDATION only marks the fields, and the field errors are in words.
   The dictionary gains 85 entries.
2. **What stays a code.** A command as the device gets it ("set_mode = cool", "set_temperature = 24 °C"), as in Figma, and the DiagnosticRun state names in the banner text (end_requested, end_blocked …). The optgroup names of the action list and "Power ON / OFF" are words.
3. **Times.**
   - A command's request and acknowledgement times, the run's start and end, and the observation time are clock times in the display time zone (`showClock`, with the zone).
   - The work window is a span (`showSpan`). Before, these were fixed Kuala Lumpur times, and the window was "09/14 08:00 – 09/20 08:00".
4. **Before the work window (IR76).** units.get answers `errors.assignment_not_started`. The page then reads the URL's job (`jobs.get`) and says when diagnostic control opens, with a link back to the unit.
5. **Code.** The loader moves to `_lib/load.ts` and formats everything; the view only renders. `shared/lib/techControl.ts` gains capabilityText, restrictionText, blockedText, connectionText, refusal and fieldText. A history row says whether it is a test-run command, so the card's last command no longer depends on the English title.
6. **Checked.**
   - Vitest: 41 files, 240 tests. `techControl.test.ts` moves to the display-zone times and adds the capability, restriction, blocked, connection, refusal and field words and a Malay case.
   - E2E: `technician/language.e2e.ts` opens the job's diagnostic control in Malay from the unit screen, with its five cards and the send button; nothing is sent. The suite: 64 passed, 9 skipped; the users are en / Asia/Kuala_Lumpur, and no diagnostic command was created.
7. **Still open.**
   - The technician devices (list, detail, events) and the QR scan in Malay.
   - The HQ screens.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

## IR287 The technician's devices in the display language; dictionary keys with a context — 2026-10-10

`/technician/devices` and `/technician/devices/[id]` (SCR-T11, SCR-T12, FR-T11, FR-T12, Figma Technician 03-x) already had Figma's layout: the list with filters, then the device with its tiles, binding, actions and history, or its events. They now follow the display language and time zone.

1. **Texts.** These are translated:
   - the filter chips, the search, the list and its empty states;
   - the four tiles: connection, power signal, tamper and firmware, with their evidence lines;
   - the binding card, the three actions, the sensor table, and the operation and calibration history;
   - the firmware card in each state, and the events with their recovery, alert and note lines;
   - the five dialogs (register, rebind, calibrate, firmware, response note), the toasts, the inline validation, and the refusals in words.
   The dictionary gains 151 entries.
2. **What stays a code** (as in Figma): the event types (tamper, power_lost, communication_lost …), the evidence sources (heartbeat, power_signal, tamper_signal), firmware versions, serials and sensor metrics.
3. **Times.** These are in the display time zone:
   - fault times on the tiles, and the firmware start and deadline (`showClock`);
   - operation starts, calibrations and a finished update (`showTime`);
   - event, recovery and note times, with "today / yesterday" (`relativeTime`).
   "… ago" lines stay relative. Before, all of these were fixed Kuala Lumpur times. The loader formats everything, including the sensor table, which now comes from `sensorRows`.
4. **Acknowledging a device's alert** needs `alert.read` on the unit's assignment, as IR284 made it. The page offered the link only to alert.resolve holders before.
5. **Context keys.**
   - "Clear" was already the Malay button "Kosongkan", but the tamper tile uses it as a state.
   - A key can now name its context before `::`: `t("tamper::Clear")` shows "Clear" in English and "Tiada gangguan" in Malay, while "Clear" stays the button.
   - `translate` shows the text after the context when the dictionary has no entry. `i18n.test.ts` covers it.
6. **Checked.**
   - Vitest: 41 files, 243 tests. `techDevices.test.ts` moves to the display-zone times and adds Malay tiles, history, events, failure text, the sensor rows and refusals. The i18n test adds the context keys.
   - E2E: `technician/language.e2e.ts` opens the devices and one device's events in Malay. The suite: 64 passed, 9 skipped; the users are en / Asia/Kuala_Lumpur.
7. **Still open.**
   - The technician QR scan in Malay; the technician screens are complete after it.
   - The HQ screens.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

## IR288 The technician's QR scan in the display language — every technician screen done — 2026-10-10

The "Scan unit QR" dialog of the technician shell (FR-T13, DD-T13, Figma Technician 01-6) follows the display language and time zone. Every technician screen does now.

1. **Texts.** These are translated:
   - the dialog, the simulated camera, the input and its label, and the one-tap labels of the user's open jobs;
   - the idle, busy, matched and refused states, and the footer buttons;
   - the matched card: the unit, its model and place, the job of today or the next one, its type, status and linked alerts;
   - the refusals: "Page unavailable" for a label outside the assignments or unknown, "Not a label", "Scan failed".
   The dictionary gains 25 entries.
2. **Times.**
   - The matched job's window is a span in the display time zone (`showSpan`). Before, it was a fixed Kuala Lumpur "10:00–12:00".
   - "Your job today" still means today's Kuala Lumpur day, the business day the work happens on, as on the overview (IR281).
   - The scan time is a display-zone clock time.
   - The dialog formats in the browser, after the user scans, so there is no first render to match the server.
3. **Code.** `qrMatch` and `qrRefusal` take the display. The refusal says whether the label is absent (`absent`), and the dialog shows that as a warning, without comparing translated text. `techOverview.windowText` now only serves the timeline.
4. **Checked.**
   - Vitest: 41 files, 244 tests. `techQr.test.ts` moves to the display-zone span and adds a Malay case.
   - E2E: `technician/language.e2e.ts` opens the dialog in Malay and scans an unknown label ("Halaman tidak tersedia"). The suite: 64 passed, 9 skipped; the users are en / Asia/Kuala_Lumpur.
5. **Progress.** Every customer, contractor and technician screen follows the display language: 29 of the 44 business screens.
6. **Still open.**
   - The HQ screens (15).
   - The browser demo.
   - The assistant panel and the voice demo's answers.

## IR289 The HQ overview in the display language — 2026-10-10

`/admin` (SCR-A01, FR-A01, DD-A01, Figma Admin 01) is the first HQ screen to follow the display language and time zone.

1. **Texts.** These are translated:
   - the customer, property and period filters and the refresh button;
   - the eight KPI tiles with their sub lines and links;
   - the energy-saving forecast in each direction, or why there is none (IR78);
   - the power-state and connection axes with the operation-rate note;
   - the job statuses (`statusWord`, which gains "requested") and the billing card, still per currency (IR50);
   - the footnotes, and the empty and error banners.
   The dictionary gains 63 entries. Field names that name the API (forecastSavedKWh, amountsByCurrency, powerState=on …) stay as they are.
2. **Times.**
   - The as-of time is a clock time in the display time zone, with its zone.
   - The period stays Kuala Lumpur days. `periodRange` labels it in the user's language with "Asia/Kuala_Lumpur", as on the customer energy screens.
   - The page formats both on the server and hands the line to the view ("As of 1:18 am GMT+9 · 15 Sept, 00:00 – 15 Sept, 00:18 (1 day) · Asia/Kuala_Lumpur"). Before, it was a fixed "As of 09:00 MYT · 2026-09-14 00:00–09:00 (Asia/Kuala_Lumpur)".
3. **Shared helpers.** These take the translator: `kpisFrom`, `forecastView`, `axisRows` and `jobRows` (now with a label beside the status code). `asOfText` takes the period label and the display. The overdue sub line is singular or plural.
4. **Checked.**
   - Vitest: 41 files, 245 tests. `adminSummary.test.ts` moves to the new as-of line and adds a Malay case.
   - E2E: `admin/language.e2e.ts` is new. It sets Malay and Asia/Tokyo and checks the filters, KPIs and cards, and the as-of line in GMT+9 with the Kuala Lumpur period. Then it restores English. The suite: 65 passed, 9 skipped; the users are en / Asia/Kuala_Lumpur.
5. **Still open.**
   - The other 14 HQ screens: jobs with SLA, alerts, units, devices, energy, MRV, offsets, billing, contracts, restrictions with exceptions, access, automation and audit.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

## IR290 The HQ Jobs tab in the display language; jobs.list filters by type — 2026-10-10

The Jobs tab of `/admin/jobs` (SCR-A06, FR-A06, DD-A06, Figma Admin 06-1, 06-11…06-18) follows the display language and time zone. The Plans, Contractors and SLA tabs come next.

1. **Texts.** These are translated:
   - the tabs and the customer → property → unit scope, which the other tabs share;
   - the ten stage tiles, the filters (type, origin, delivery, assignee, overdue only, sort) and the period chip;
   - the list rows: type and what is next;
   - the detail: the stepper, the four facts, the follow-up card, the preferred times with the HQ technicians free for each, the proposal, delivery and partner-time cards, the submitted report with its review, the cost lines with their totals, and the history;
   - the eight dialogs (book, propose another time, hold / resume / cancel / reassign, classify, report, review, cost line, extend access) and New job, with their validation, toasts and refusals.
   The dictionary gains 290 entries. These stay as they are: status codes in technical lines (submitted → rework_requested), operation names (members.eligible, jobs.saveCost) and 受領.
2. **Times.**
   - Instants are in the display time zone:
     - the requested window, the preferred times and proposals, as one span (`showSpan`, with the weekday in the table and the dialogs);
     - the due, classify-by, reply-by, offer and access times;
     - the report's submitted, signed and reviewed times, and the history (`showTime`).
   - The loader formats every time the detail shows on its first render (`texts`). The dialogs format after the user acts.
   - Business days stay Kuala Lumpur days:
     - the follow-up's classify-by business day (its time shows in the display zone);
     - the plan occurrence's date;
     - "from tomorrow" for new preferred times. Its message now says "(Kuala Lumpur)", as on the customer's form.
   - The times HQ types are read in the display time zone and sent as instants (NFR-08). Before, they were fixed +08:00. They are:
     - the proposed time;
     - New job's preferred times and due time;
     - an offer's expiry and access window;
     - the new end of an access extension.
     The dialogs name the zone ("times in Asia/Tokyo") and start on tomorrow in that zone.
3. **jobs.list filters by type** (query catalog, DD-A06 item 6).
   - DD-A06 has a type filter, but jobs.list had none. The tab filtered the first 100 rows in the browser, and dropped cancelled jobs there too.
   - As a result, "Showing 4 of 100" counted 96 cancelled jobs. With the deadline sort, cancelled jobs could also push open ones off the first page.
   - jobs.list now takes `type` (MaintenanceJob.type). Offer and history rows match on their public type (IR23). An unknown type is VALIDATION.
   - Without a stage tile, the tab asks for every status except cancelled (`statuses`). No tile shows cancelled jobs, and the "in scope" count already left them out. The total now counts what the list shows.
   - Swagger is regenerated. `jobs_test.go`, `projections_test.go` and the query-catalog test cover the filter.
4. **Code.**
   - These adminJobs helpers take the display or the translator: `hqRow`, `stepper`, `facts`, `delivery`, `preferredRows`, `controls`, `classifyBy`, `hqRefusal`, `costLineOf`, `newJobErrors`, `reportCard`, `returnReason`, `slotText`, `longSlot` and `periodChip`.
   - New: `zonedInput` / `fromZonedInput` (a datetime-local value in the display zone), `tomorrowIn` and `LISTED`.
   - The fixed Kuala Lumpur formatters (`klTime`, `md`, `hm`) are gone from adminJobs.
5. **Checked.**
   - Go: both suites pass.
   - Vitest: 41 files, 247 tests. `adminJobs.test.ts` moves to the display-zone spans, and adds a Malay / Tokyo case, the typed-time helpers and the filters.
   - E2E: `admin/language.e2e.ts` opens the Jobs tab and New job in Malay ("masa dalam Asia/Tokyo") and closes it without saving. `jobs-period.e2e.ts` reads the new chip.
   - The suite: 65 passed, 9 skipped; the users are en / Asia/Kuala_Lumpur.
6. **Still open.**
   - The Plans, Contractors and SLA tabs of `/admin/jobs`.
   - The other 13 HQ screens: alerts, units, devices, energy, MRV, offsets, billing, contracts, restrictions with exceptions, access, automation and audit.
   - The browser demo (its New job keeps its own English form).
   - The assistant panel and the voice demo's answers.

## IR291 The HQ Plans, Contractors and SLA tabs in the display language; a breach carries its minutes — 2026-10-10

The other three tabs of `/admin/jobs` (FR-A06, FR-A21, FR-A22, DD-A06 item 9, DD-A21, DD-A22, Figma Admin 06 283:2, 06-9, 06-10) follow the display language and time zone. With IR290, the whole maintenance jobs screen does.

1. **Texts.** These are translated:
   - Plans: the list, the plan card with its form and the next-occurrence box, the generated occurrences, the CONFLICT banners, New plan and the refusals;
   - Contractors: the list and its badges, the 90-day KPI tiles, the profile facts, the rate card in effect and the scheduled ones, technicians and certificates, and the four dialogs (profile, offer suspension, rate card, certificate verification);
   - SLA: the period and contractor filters, the KPI tiles with their targets, the customers table, the recent breaches, the targets dialog, the toasts and refusals;
   - the CSV export, whose header and plan and status words follow the language.
   The dictionary gains 264 entries.
2. **Dates.**
   - A plan's dates are its instants in the display time zone, the zone its next date is typed in (D16). Before, they were fixed Kuala Lumpur dates. The loader formats the dates of the first render: the list, the next occurrence, the date after it and the generated occurrences.
   - The Jobs tab's "plan occurrence" date follows the same rule. IR290 named it a Kuala Lumpur day; this replaces that.
   - The contractor register's dates are Kuala Lumpur days, as on the contractor's own certifications (IR277). These are the delegation period, the insurance end, the rate card starts, membership ends and certificate dates. The insurance end and a rate card's start are typed as Kuala Lumpur days and say so. In another display time zone the tab says "The days are Kuala Lumpur days."
   - An SLA target's start is typed and shown in the display time zone (NFR-08). The CSV's period ends are dates in that zone.
3. **A breach carries its minutes** (`SlaScorecard.breaches`, service contracts, Swagger).
   - `detail` was the only text, and it was English ("response 6 h 10 min vs 4 h").
   - Now each breach also has `tookMinutes` (request → response; null while unanswered) and `limitMinutes` (the target in effect when the job was created). Both are null for kinds other than response.
   - The tab words a breach from its kind and minutes. `detail` stays English for logs.
4. **A job cancelled before its response was due is no response miss** (IR131 item 5 said nothing about cancellation).
   - Every cancelled job without a response counted as a miss once its target time had passed. On the dev data, the E2E suite's jobs, which are cancelled within seconds, made 40 "no response within 4 h" breaches and a 56 % response rate.
   - Now a job cancelled (the `job.cancelled` event) within its response target counts neither as within the target nor as a miss. A job cancelled later without a response is still a miss. On the same data: 4 breaches, 94 %.
   - `partners_test.go` covers both cases and the breach minutes.
5. **Code.**
   - The adminPlans, adminContractors and adminSla helpers take the display or the translator.
   - `nextBox` takes the server's next date, so the first render matches it (IR282). `breachText` is new.
   - Rate card lines keep their work type, so the form finds them in any language.
   - The fixed Kuala Lumpur input helpers `klInput` / `fromKlInput` and `klDate` are gone. Plans and SLA use the display-zone `zonedInput` / `fromZonedInput`; Contractors uses the Kuala Lumpur day helpers of the contractor team (`klDate`, `klStart`).
6. **Checked.**
   - Go: both suites pass.
   - Vitest: 41 files, 250 tests. The three tab test files move to the new dates and add Malay / Tokyo cases.
   - E2E: `admin/language.e2e.ts` also opens the Plans, Contractors and SLA tabs in Malay, and the targets dialog ("masa dalam Asia/Tokyo"); nothing is saved.
   - The suite: 65 passed, 9 skipped; the users are en / Asia/Kuala_Lumpur.
7. **Progress.** 31 of the 44 business screens follow the display language: every customer, contractor and technician screen, and the HQ overview and maintenance jobs.
8. **Still open.**
   - The other 13 HQ screens: alerts, units, devices, energy, MRV, offsets, billing, contracts, restrictions with exceptions, access, automation and audit.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

## IR292 The HQ alerts and alert policies in the display language; the HQ metrics as DD-A05 lists them — 2026-10-10

`/admin/alerts` (SCR-A05, FR-A05, DD-A05, Figma Admin 05) follows the display language and time zone.

1. **Texts.** These are translated:
   - the tabs and the open / acknowledged / resolved tiles;
   - the list of open alerts with the severity and state badges;
   - the alert card (unit, context, evidence, cause, observed, evidence records), its note and the timeline;
   - the policy list with the default policy first, and the policy editor: basics, owner and units, condition with the recovery check, severity, recipients and channels, cooldown and escalation;
   - the resolve dialog, the toasts, the validation and the refusals.
   The dictionary gains 68 entries. Cause codes (window_open …) stay codes, as in Figma, with "(suspected)" translated. Evidence texts stay as the Core API records them.
2. **Times.**
   - The list's and the timeline's times are in the display time zone, with "today / yesterday" for the days next to now (`relativeTime`).
   - The observed and detected times are dates and times in that zone (`showTime`).
   - The page formats all of them on the server. Before, they were fixed Kuala Lumpur "08:55" times without a date.
   - The card's "detected" time showed the observed time. It now shows `detectedAt`.
3. **State codes.** A row carries its state (open, acknowledged, resolved) beside its translated label. The tiles, the list filter and the buttons read the code, so the screen works in any language. The Phase 1A demo rows keep their English data.
4. **HQ metrics.** The editor offered all ten metric labels, but the Core API takes seven for an HQ alert policy. Compressor cycles, airflow drop and heartbeat gap were refused as VALIDATION. The editor now offers the seven that DD-A05 item 10 and the API list (`HQ_METRICS`).
5. **Code.**
   - `adminAlertRows` takes the clock and the display; `policyRows` takes the translator.
   - `policyErrors` is new: the form's checks in one place.
   - `adminAlerts.test.ts` is new: order and rows, the timeline, the policies and their condition, the form checks and the HQ metrics, and Malay / Tokyo.
6. **Checked.**
   - Vitest: 42 files, 254 tests.
   - E2E: `admin/language.e2e.ts` also opens the alerts and the policies in Malay; nothing is acknowledged, resolved or saved.
   - The suite: 65 passed, 9 skipped; the users are en / Asia/Kuala_Lumpur, and the dev alerts are unchanged.
7. **Progress.** 32 of the 44 business screens follow the display language.
8. **Still open.**
   - Resolving with evidence. IR66 and DD-A05 item 6 ask for an evidence ID when an alert without a policy is resolved. Like the technician's screen (IR284), the HQ dialog sends a reason only, and the Core API takes an empty list. It needs the evidence records behind `evidenceIds` (IR284, still open).
   - The other 12 HQ screens: units, devices, energy, MRV, offsets, billing, contracts, restrictions with exceptions, access, automation and audit.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

## IR293 HQ customers & units in the display language — 2026-10-10

`/admin/units` (SCR-A02, FR-A02, FR-A17, FR-A18, FR-A19, DD-A02, Figma Admin 02) follows the display language and time zone.

1. **Texts.** These are translated:
   - the register: its tabs, KPI tiles, filters, the customer table and the contract badges;
   - one customer: its header and KPIs, and the location tree with its menus;
   - the property, space and unassigned cards; the units table with its power and connection filters;
   - the unit edit, with its alert policies, blockers and archive note;
   - the Alert policies tab with the default rules, and the Users tab;
   - Warranty & coverage with the claims, and the CSV import wizard;
   - the thirteen dialogs (customer, property, location, rename, delete location, new unit, relocate, delete unit, attach policies, invite, user change, warranty claim, CSV import), the toasts, the validation and the import messages.
   The dictionary gains 401 entries. Unit types (split), plan codes (RTO …), permission names and API field names stay as they are.
2. **Times and dates.**
   - Instants are in the display time zone:
     - the last edit of a location;
     - when a unit was last seen ("today …", `relativeTime`);
     - the day a default rule was switched;
     - a job's completion on a warranty claim;
     - the time an import can be undone until. Before, this was a fixed "… MYT".
   - "Customer since" is the month in the user's language.
   - These stay Kuala Lumpur days, in the user's language:
     - a unit's installation date and warranty end, which are also typed as Kuala Lumpur days and say so;
     - a contract's end ("until …").
   - Weekday names in a policy condition come from the user's language ("Isn, Rab").
   - The coverage CSV keeps its ISO warranty date (`endsDate`). The CSV column names stay machine names; status and message texts follow the language.
   - HQ's client-user rows use the display too. IR267 had kept them English.
3. **Code.**
   - These assets helpers take the translator or the display: `standingMarks`, `customerRows`, `registerKpis`, `customerErrors`, `propertyErrors`, `placeOptions`, `unitRows`, `unitErrors`, `changedFields`, `conditionText`, `policyLines`, `defaultRules`, `coverageRows`, `coverageKpis`, `claimCandidate`, `importMessage` and `errorReportCsv`.
   - English stays the default, so the customer screens that share them are unchanged.
   - Counts have singular and plural keys ("1 rule", "{n} rules").
4. **Checked.**
   - Vitest: 43 files, 259 tests. `assets.test.ts` is new: standing marks and the register, units and the unit form, conditions and default rules, coverage with Kuala Lumpur warranty days and the CSV, import messages and the error report, a claim, and Malay / Tokyo.
   - E2E: `admin/language.e2e.ts` also opens the register, one customer's locations, users and policies, and warranty & coverage in Malay; nothing is saved.
   - The suite: 65 passed, 9 skipped; the users are en / Asia/Kuala_Lumpur.
5. **Progress.** 33 of the 44 business screens follow the display language.
6. **Still open.**
   - Table rows that open a record (`DataTable` with `onRowClick`) take a mouse click only, with no keyboard focus or Enter. This is a shared-component gap across HQ screens.
   - The other 11 HQ screens: devices, energy, MRV, offsets, billing, contracts, restrictions with exceptions, access, automation and audit.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

## IR294 Table rows that open a record work by keyboard — 2026-10-10

IR293 found that `DataTable` rows with `onRowClick` opened a record only on a mouse click. Those rows are the HQ register's customers, a customer's units and its alert policies.

1. **Rows.**
   - Such a row now takes the keyboard focus (`tabIndex` 0) and opens with Enter or Space. Both keys act only when the row itself has the focus, so a control inside the row keeps its own keys.
   - A focused row shows an outline. The selected row carries `aria-current`.
   - Rows without `onRowClick` are unchanged.
2. **Checked.** E2E: `admin/table-keyboard.e2e.ts` is new. It focuses the register's customer row and presses Enter, then a unit row and presses Space; each opens its record. This closes the IR293 open item.

## IR295 The HQ device registry in the display language; a campaign's start in the display time zone — 2026-10-10

`/admin/devices` (SCR-A04, FR-A04, FR-A20, DD-A04, DD-A20, Figma Admin 04) follows the display language and time zone.

1. **Texts.** These are translated:
   - the tabs;
   - the models list and the capability editor: identity, controls, temperature, modes and fan, ventilation, sensors, firmware candidates, the impact of a change and the change reason;
   - the version history;
   - the IoT devices list and a device's tiles, summary, sensors, operations, calibrations and device events;
   - the register, rebind, calibrate and firmware dialogs;
   - firmware campaigns with their waves and devices, and the new-campaign and abort dialogs;
   - the toasts and the validation.
   The dictionary gains 159 entries. These stay as they are: metric codes, evidence sources, firmware versions, serials and result reason keys.
   - "Schedule" was already the noun "Jadual". The campaign button uses the context key `campaign::Schedule` ("Jadualkan", IR287).
2. **Times.**
   - These are in the display time zone (`showTime`): a capability's update, an operation and its finish, a calibration, a sensor's calibration, a campaign's start, and the version history entries.
   - A device's last seen time and its events show "today / yesterday" (`relativeTime`).
   - The page formats the times of the first render (IR282). Before, they were fixed Kuala Lumpur "YYYY-MM-DD HH:MM".
3. **A campaign's start.**
   - It is now typed in the display time zone (`campaignStart`), and the Server Action takes the instant. Before, the form added +08:00. The 24-hour check reads the same instant.
   - The install window stays the device's local time, as the Core API defines it, and the dialog says so.
   - Wave labels are stored with the campaign, so they stay English data. The screen words each wave from its number and percentage.
4. **Device events on the audit screen** keep English and Kuala Lumpur until that screen is translated.
   - Its call is now `map((e) => deviceEventItem(e))`. A direct `map(deviceEventItem)` would have passed the array index as the display.
5. **Checked.**
   - Vitest: 44 files, 263 tests. `devices.test.ts` is new. It covers models and devices, operations, calibrations and device events, waves, the campaign start and its checks, and Malay / Tokyo.
   - E2E: `admin/language.e2e.ts` also opens the three device tabs and the new-campaign dialog ("masa dalam Asia/Tokyo", "Waktu tempatan peranti"); nothing is saved.
   - The suite: 66 passed, 9 skipped; the users are en / Asia/Kuala_Lumpur.
6. **Progress.** 34 of the 44 business screens follow the display language.
7. **Still open.**
   - The other 10 HQ screens: energy, MRV, offsets, billing, contracts, restrictions with exceptions, access, automation and audit.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

## IR296 The HQ energy analysis in the display language; its periods stay Kuala Lumpur time — 2026-10-10

`/admin/energy` (SCR-A13, FR-A13, DD-A13, Figma Admin 13) follows the display language.

1. **Texts.** These are translated:
   - the Analysis and Baselines tabs;
   - the scope card: customer, baseline, period, units and the comparison notes;
   - the figure tiles, the cost card with the IR68 note, and the calculation conditions with their quality warnings;
   - the baselines filters, the list, the editor and its checks (`baselineErrors`), the toasts, and a failed summary's message.
   The dictionary gains 49 entries. These stay as they are: method and boundary codes, baseline IDs, and the stored boundary, assumption, factor and source texts.
2. **Periods.**
   - The analysis and baseline periods are Kuala Lumpur business time, like the customer energy periods (REV18-035). They are typed in Kuala Lumpur time, and the labels say so: "From (Kuala Lumpur)", "Period start (Kuala Lumpur)".
   - They are shown as Kuala Lumpur "YYYY-MM-DD HH:MM" with "(Asia/Kuala_Lumpur)", as in the conditions of the customer screens (IR265).
   - The baselines filter reads `[from, to)` on a baseline's period start (query catalog). Its labels are now "Period starts from (Kuala Lumpur)" and "Period starts before"; before, they were "Period start from" and "to".
3. **The baseline list** words its value and unit count in the display language (`baselineRows(bs, t)`). The MRV screen still calls it without `t`, so it stays English until that screen is translated.
4. **Checked.**
   - Vitest: 44 files, 264 tests. `energy.test.ts` gains the baseline rows and the form checks in Malay.
   - E2E: `admin/language.e2e.ts` also opens both energy tabs. It checks that the period is named Kuala Lumpur time in another zone. It also checks the new-baseline form errors, which stop before any call. Nothing is saved.
5. **Progress.** 35 of the 44 business screens follow the display language.
6. **Still open.**
   - The other 9 HQ screens: MRV, offsets, billing, contracts, restrictions with exceptions, access, automation and audit.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR297 The HQ MRV workspace in the display language; report periods stay Kuala Lumpur time — 2026-10-10

`/admin/mrv` (SCR-A14, FR-A14, DD-A14, Figma Admin 14) follows the display language.

1. **Texts.** These are translated:
   - the Reports and Emission factors tabs;
   - the report filters, the list, a report's figures, its version selector, the condition snapshot, the evidence and the demo review;
   - the new-report dialog with its checks (`mrvDraftErrors`) and the "not saved" preview;
   - the factors list, the factor form and its checks (`factorErrors`), and the toasts.
   The dictionary gains 73 entries. "Demo — unverified" stays on screen as "Demo — belum disahkan".
   These stay as they are: method and boundary codes, report and baseline IDs, and the stored boundary, region, source and review texts.
2. **Times.**
   - Report periods are Kuala Lumpur business time, as on the energy screens (IR296). They are typed in that time ("Start (Kuala Lumpur)"), and the list and the condition snapshot show them as Kuala Lumpur "YYYY-MM-DD HH:MM" with "(Asia/Kuala_Lumpur)".
   - The list's period filter reads `[from, to)` on a report's period start (query catalog). It is labelled like the baselines filter: "Period starts from (Kuala Lumpur)" / "Period starts before".
   - Demo review times are instants. The page formats them in the display time zone (`reviewItems`, `showTime`, IR282). Before, they were fixed Kuala Lumpur stamps.
3. **New helpers.**
   - `statusWord` words a version's status ("draft", "demo reviewed"). Before, the version selector showed the raw code.
   - The evidence line words one attachment and several attachments separately.
   - The new factor's default source is in the display language ("Demo (fictional)"). It is the user's to change, and it is stored as typed.
4. **Checked.**
   - Vitest: 45 files, 269 tests. `mrv.test.ts` is new. It covers the report rows, the figures and conditions with the IR68 wording, the incomplete case, the review history, the report and factor checks, the Kuala Lumpur conditions, and Malay / Tokyo.
   - E2E: `admin/language.e2e.ts` also opens the MRV reports. It checks the new-report errors, then runs a preview, which is a read (`mrv.preview`): "Pratonton · tidak disimpan" with "Demo — belum disahkan", and the figures or why they are missing. It also checks the new-factor errors, which stop before any call. Nothing is saved.
5. **Progress.** 36 of the 44 business screens follow the display language.
6. **Still open.**
   - The other 8 HQ screens: offsets, billing, contracts, restrictions with exceptions, access, automation and audit.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR298 The HQ offset demo in the display language; attempt and event times in the display time zone — 2026-10-10

`/admin/offsets` (SCR-A15, FR-A15, DD-A15, Figma Admin 13-1…13-5) follows the display language and time zone.

1. **Texts.** These are translated:
   - the demo banner and the Demo records and Market concept tabs;
   - the records list with each record's state and next step;
   - a record's steps, failure banner, summary, next-step cards and event history;
   - the market concept card;
   - the new-quote dialog with its checks (`quoteErrors`), the quote, and the toasts.
   The dictionary gains 69 entries. They use the customer offsets page's words: "sebut harga" (quote), "pembatalan" (retirement), "penyedia" (provider), "pengesahan" (verification), "lejar" (ledger).
   These stay as they are: IDs, purchase and DEMO- certificate references, the scheme code, and stored purposes and reasons.
2. **States and events.**
   - A record's state shows as a word (`stateWord`, the same words as the customer page). Before, the badge showed the raw state code.
   - A failed record names the failed stage and the previous state in words.
   - The event history comes from the audit log. Its actions (`offsets.request`, `purchase_confirm`, `retire`, `fail`, `retry`) and results (`success`, `denied`, `failed`, `pending`) are worded. An unknown action or result stays as its code.
   - The market concept's codes (`future_concept`, `unverified`, `not_connected`) are worded (`conceptText`).
3. **Times.**
   - The current attempt's start and the event times are instants. The page formats them in the display time zone (`attemptText`, `eventItem`, IR282). Before, they were fixed Kuala Lumpur stamps.
   - A quote's expiry is shown in the display time zone. The quote comes from a Server Action, so the browser formats it.
   - A quote's period is Kuala Lumpur business time, like the energy and MRV periods (IR296, IR297). It is typed in that time ("Period start (Kuala Lumpur)").
4. **The note about energy savings and MRV estimates** is one translated sentence, followed by the two links. Before, the links sat inside the English sentence.
5. **Checked.**
   - Vitest: 46 files, 272 tests. `offsets.test.ts` is new. It covers the states and next steps, attempts and events in the display zone, unknown event codes, the quote checks with the Kuala Lumpur period sent as instants, the market concept, and Malay / Tokyo.
   - E2E: `admin/language.e2e.ts` also opens the offset demo. It checks the banner, the records, the new-quote checks (which stop before any call, since a quote is a write) and the market concept. Nothing is saved.
6. **Progress.** 37 of the 44 business screens follow the display language.
7. **Still open.**
   - The other 7 HQ screens: billing, contracts, restrictions with exceptions, access, automation and audit.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR299 HQ billing in the display language; billing dates stay Kuala Lumpur days — 2026-10-10

`/admin/billing` (SCR-A08, FR-A08, FR-A23, DD-A08, DD-A23, Figma Admin 08) follows the display language. The Invoices, Inquiries and Contractor payouts tabs are covered.

1. **Texts.** These are translated:
   - the tabs, the scope filters, the totals and the status filter;
   - the invoice list and an invoice's summary, payments, card confirmation and manual-payment notes;
   - the conflict banner and the reminder card;
   - the related restriction and the linked inquiries;
   - the inquiries list and an inquiry's reply form;
   - the statements list and a statement's summary, lines and contractor questions;
   - the manual-payment, card-confirmation and new-invoice dialogs with their checks, and the toasts.
   The dictionary gains 116 entries. These stay as they are: invoice numbers, plan type and recipient role codes, payment references, `release_requested`, and stored messages and replies.
2. **Dates.**
   - Billing periods, due dates, billing months and payout pay dates are Kuala Lumpur business days, in the user's language ("1 Aug 2026 → 1 Sept 2026", "10 Sept 2026", "August 2026"). They use `businessDay` / `billingMonth`, as on the customer payments screen (IR267).
   - When the display time zone is another one, the screen says so. HQ types new invoice dates as Kuala Lumpur days, and the dialog says so too.
   - Inquiries are instants. The page formats them in the display time zone (IR282). Before, they were Kuala Lumpur days.
   - `businessDay`, `billingMonth` and `KL` now live in `lib/billing`, and `lib/clientBilling` re-exports them, so no module imports in a circle.
3. **Words for codes.**
   - Payment methods and payment statuses are worded (`paymentWord`). An unknown status stays as its code.
   - Statement statuses (`statementWord`), line work types and kinds, and question topics and states are worded, with the partner payouts' words.
   - A row carries the payment method and the payment status as separate fields. Before, the view split one joined string on " · ".
   - Deduction lines use the minus sign, as the adjustments already did. Before, they used "-".
4. **The reminder preview** is now the message the customer gets: `previewText` of the customer screen, with the subject for e-mail, in this screen's language. Before, it was a separate English sentence.
5. **A fix.** The Contractor payouts tab showed a count of 1 on the other tabs in API mode, which was the demo's count. Statements are read on that tab only, so the count now shows only there.
6. **Checked.**
   - Vitest: 47 files, 275 tests. `billing.test.ts` is new. It covers the invoice statuses, rows, totals and payments, inquiries, contracts, statements and their lines and questions, Kuala Lumpur business days against another display zone, and Malay / Tokyo.
   - E2E: `admin/language.e2e.ts` also opens billing. It checks the named Kuala Lumpur dates and the totals. It runs a reminder preview, which is a read (`notifications.preview`): "belum ada apa-apa dihantar" and the customer's message. It checks the manual-payment and new-invoice errors, which stop before any call, and opens the inquiries and payouts tabs. Nothing is saved or sent.
7. **Progress.** 38 of the 44 business screens follow the display language.
8. **Still open.**
   - The other 6 HQ screens: contracts, restrictions with exceptions, access, automation and audit.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR300 HQ contracts in the display language; contract periods stay Kuala Lumpur days — 2026-10-10

`/admin/billing/contracts` (SCR-A07, FR-A07, DD-A07, Figma Admin 07) follows the display language.

1. **Texts.** These are translated:
   - the scope filters and the plan-type tabs;
   - the list with each contract's plan, unit count, period and restriction eligibility;
   - the editor: customer, plan type, units, period, price and currency, restriction eligibility and rules version;
   - the SR19 block, the "what saving does" summary, the toasts and the checks (`draftErrors`).
   The dictionary gains 39 entries. These stay as they are: contract IDs, the rules version, unit and property names, and the "id ↑" sort marker.
2. **Plan types** are worded (`planWord`: RTO, General, Energy, Environment). Before, the view showed the English labels.
3. **Dates.**
   - Contract periods are Kuala Lumpur business days. The list shows them in the user's language ("1 Jan 2026 → 1 Jan 2027", `businessDay`).
   - The date inputs keep their `yyyy-mm-dd` Kuala Lumpur values. Under Start, a hint says that the dates are Kuala Lumpur days.
   - An unchanged day still keeps the stored instant (`dayInstant`).
4. **Checked.**
   - Vitest: 48 files, 278 tests. `contracts.test.ts` is new. It covers the rows with the input days and the shown period, and the unit options (archived units and other customers' units are left out). It also covers the draft checks, `dayInstant`, `priceMinor`, and Malay / Tokyo (the days stay Kuala Lumpur's).
   - E2E: `admin/language.e2e.ts` also opens contracts. It checks the plan tabs, the Kuala Lumpur day hint, and the new-contract errors, which stop before any call. Nothing is saved.
5. **Progress.** 39 of the 44 business screens follow the display language.
6. **Still open.**
   - The other 5 HQ screens: restrictions with exceptions, access, automation and audit.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR301 HQ restrictions and the exception screen in the display language; HQ types their times in the display time zone — 2026-10-10

`/admin/restrictions` and `/admin/restrictions/[id]` (SCR-A09, SCR-A10, FR-A09, FR-A10, DD-A09, DD-A10, Figma Admin 09–10) follow the display language and time zone.

1. **Texts.** These are translated:
   - the state chips and the contract filter;
   - the list with each restriction's progress;
   - the detail: lifecycle, summary, execute and release with their reasons, cause invoices, units with their apply, release, observed and command states, and recovery cases;
   - the retry dialog, and the schedule dialog with its checks;
   - the exception screen: the four action cards with the transition for the current state, the form, "what happens", the units needing follow-up, the summary and the audit timeline;
   - the no-access state.
   The dictionary gains 142 entries. They use the customer notice's state words (IR267: "Dikenakan", "Pelepasan diminta").
   - The units table's "Apply" column means applying the restriction, so it uses the context key `restriction::Apply` ("Pengenaan").
   - The schedule button uses the context key `restriction::Schedule` ("Jadualkan").
   These stay as they are: IDs, rules versions, operation and permission codes, failure codes, and stored reasons.
2. **Words for codes.** These are worded: states (`stateWord`), release-intent sources (`sourceWord`), per-unit apply and release states, command statuses, pending reasons and recovery states. Before, the screens showed the raw codes.
3. **Times.**
   - Notice, execute-after, grace and exception ends, release intent, observed and audit times are instants, shown in the display time zone. Before, they were Kuala Lumpur stamps.
   - The page formats the first render's times, and the execute and release blockers that contain them (IR282).
   - HQ types the schedule's "execute after" and the exception's "until" in the display time zone (`zonedInstant`, NFR-08), as with a firmware campaign's start (IR295). They are deadlines, not business days. Before, they were typed in Kuala Lumpur time. The 24-hour and 90-day checks read the same instant.
4. **Button wording.**
   - The exception buttons name their action: "Apply grace period", "Apply exception", "Cancel restriction", "Override release". Before, they were built as "Apply cancel" and "Apply override release".
   - The toasts name what was recorded.
5. **Checked.**
   - Vitest: 49 files, 282 tests. `restrictions.test.ts` is new. It covers states and progress, unit rows with their actions (SR26), the execute and release blockers, active periods and release intents, the outcome matrix (IR35, IR96), and Malay / Tokyo.
   - E2E: the new `admin/restrictions-language.e2e.ts` signs in as `hq-restriction-manager` in its own browser context. It sets Malay and Asia/Tokyo and opens the list and the selected restriction (execute-after in GMT+9, the release blocker). It runs the schedule dialog's checks, which stop before any call, and opens the exception screen (the end typed in Asia/Tokyo, the reason check). It restores the manager's English and zone in afterEach, and nothing is saved.
   - `admin/language.e2e.ts` checks hq-operator's no-access state in Malay.
   - The suite: 67 passed, 9 skipped. The manager now has a saved preference row, so five users are en / Asia/Kuala_Lumpur.
6. **Progress.** 41 of the 44 business screens follow the display language; this round covers two of them.
7. **Still open.**
   - The other 3 HQ screens: access, automation and audit.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR302 HQ access & roles in the display language; the valid period in the display time zone; a fixture name fix — 2026-10-10

`/admin/settings/access` (SCR-A03, FR-A03, DD-A03, Figma Admin 03) follows the display language and time zone.

1. **Texts.** These are translated:
   - the role and status filters;
   - the memberships list with each one's role and organization;
   - the editor: identity and role, employment, scope with its targets, valid period, the permission matrix with its resource names, and the change reason;
   - the revoke dialog, the toasts and the checks (`memberErrors`).
   The dictionary gains 61 entries. Roles (`roleWord`) and matrix resources (`resourceWord`) are worded.
   - The matrix's "Read" column uses the context key `permission::Read` ("Baca"). "Read" alone is already the notification state "Dibaca".
   These stay as they are: permission codes, `scopeVersion`, user and organization names, and the stored reasons.
2. **The valid period** is an access window, so per NFR-08 it is typed and shown in the display time zone, like an offer's access window (IR290).
   - `memberDraft` and `memberInput` take the zone (`zonedLocal` / `zonedInstant`). A revoke ends the access at now in that zone.
   - The list and the editor show the period with `showTime`, formatted by the page (IR282). Before, it was typed and shown in Kuala Lumpur time.
3. **Fixture fix.**
   - In `fixture-contract.json`, the override-only actor (`user-hq-override-only`) had the display name "hq-operator". The access screen therefore listed two "hq-operator" memberships, and the user picker offered two "hq-operator" users.
   - It is now "hq-override-only", its realm username. A seed test (`TestActorNamesUnique`) keeps every actor's display name unique.
   - The local dev database row was corrected the same way, because the seed does not overwrite existing rows.
4. **A spec race.**
   - `customer/filter-care.e2e.ts` waited only for a "Reminder settings saved." toast before reloading. The first save's toast could satisfy that wait, so the reload could read before the restoring save committed. This happened once in this round. The data was restored, but the check compared the stale page.
   - Each save now waits for its Server Action's answer and for the dialog to close.
   - The spec passed three repeated runs.
5. **Checked.**
   - Vitest: 50 files, 287 tests. `members.test.ts` is new. It covers the 38 permissions and the role limits, the rows without clients and their validity in the display zone, a Tokyo-typed period saved as the same instants, the members.save checks (including the self-grant rule), and Malay / Tokyo.
   - Go: `make test-all`, with the new seed test.
   - E2E: `admin/language.e2e.ts` also opens access & roles. It checks the role filters, the valid period in GMT+9, the "Masa dalam Asia/Tokyo" hint, the matrix's "Baca" column, and the new-membership errors, which stop before any call. Nothing is saved.
6. **Progress.** 42 of the 44 business screens follow the display language.
7. **Still open.**
   - The other 2 HQ screens: automation and audit.
   - The browser demo.
   - The assistant panel and the voice demo's answers.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR303 HQ automation policies in the display language — 2026-10-10

`/admin/settings/automation` (SCR-A11, FR-A11, DD-A11, Figma Admin 11) follows the display language.

1. **Texts.** These are translated:
   - the scope filters, and the policy list grouped by customer with each policy's When → Then sentence and priority;
   - the editor: name, priority, timezone, enabled, units, When / Then with the sentence preview, and the conflict order;
   - the simulator: facts, quality, results per unit with the decision and the rule or reason, and fire (demo);
   - the toasts and the checks (`autoErrors`).
   The dictionary gains 84 entries. These stay as they are: metric codes, units (MYR/kWh, kW), comparison signs, policy and command IDs, and policy names.
2. **Words for codes.**
   - These are worded: conditions and actions (`conditionText`, `actionText`, `subjectText`, `modeWord`, `levelWord`), decisions (`decisionWord`: selected, suppressed, requested), suppression reasons (`reasonWord`) and automatic disable reasons (`disabledReasonWord`). An unknown code stays as it is.
   - Before, the decision badge and the disabled reason showed the raw code.
3. **The "Across customers" group.** It is now keyed apart from its label, so it still sorts first in any language. Before, the code compared the English label.
4. **No times change.** The evaluation runs at the business clock's current minute, as before.
5. **Checked.**
   - Vitest: 51 files, 292 tests. `automation.test.ts` is new. It covers every condition and action sentence, the grouping and its order, the checks and the policies.save input, the evaluation input with null facts, the words for unknown codes, and Malay.
   - E2E: `admin/language.e2e.ts` also opens automation policies. It checks the condition choices, the Malay When → Then sentence, the conflict order, and the new-policy errors, which stop before any call. Nothing is saved.
   - The dev data has no HQ automation policy, so the simulator card, which needs a saved policy, was checked only by the unit tests.
6. **Progress.** 43 of the 44 business screens follow the display language.
7. **Still open.**
   - The audit screen (the last HQ screen).
   - The browser demo.
   - The assistant panel and the voice demo's answers.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR304 The HQ audit log in the display language; every business screen now follows the display language — 2026-10-10

`/admin/audit` (SCR-A16, FR-A16, DD-A16, Figma Admin 14-1…14-5) follows the display language and time zone. It is the last of the 44 business screens.

1. **Texts.** These are translated:
   - the Audit log and Device events tabs, the filter card and the result choices;
   - the list and an entry's detail: actor, role at the time, occurred, correlation ID, reason, and the masked before / after table;
   - the device events tab, the empty states and "Load more".
   The dictionary gains 27 entries. These stay as they are: actions, target kinds and IDs, correlation IDs, field names and the masked values.
2. **Words for codes.** Results (`resultWord`) and the role recorded at the time (`roleAtTimeWord`) are worded. Before, the badge showed the English code, and the role was printed raw. An unknown role stays as recorded.
3. **Times.**
   - An entry's time is an instant. The list shows it relative to now ("today 9:30"), and the detail shows the full time, both in the display time zone and formatted by the page (IR282). Before, they were Kuala Lumpur stamps.
   - Device events use `relativeTime` too (IR295).
   - The period filter's days are now the display time zone's days. `auditFilters` and `auditQuery` take the zone, and the form names it ("Days in Asia/Tokyo"). An entry shown at a time on a day is then inside a filter that starts on that day. Before, the days were Kuala Lumpur days while the times showed another zone.
4. **One timeline helper.** The restriction exception screen's audit card now uses `auditItem` from `lib/audit` (IR301).
5. **Checked.**
   - Vitest: 52 files, 296 tests. `audit.test.ts` is new. It covers the URL filters and their default period in the display zone, the period check, the audit.list input in two zones, the rows with relative and full times, the masked changes, the recorded role and unknown codes, and Malay / Tokyo.
   - E2E: `admin/language.e2e.ts` also opens the audit log. It checks "Hari dalam Asia/Tokyo", the result choices, an entry's time in GMT+9, and the device events tab.
6. **Progress.** All 44 business screens follow the display language (IR258–IR304).
7. **Still open.**
   - The Phase 1A browser demo's fixture screens, and the assistant panel and voice demo's answers.
   - Audit actors show as user IDs. `audit.list` should return the actor's display name (identity holds both), so this is next.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR305 Audit entries carry the actor's name; customers never see it — 2026-10-10

The audit log showed actors as user IDs (IR304 left this open). `AuditView` gains `actorName: string | null` (service-contracts.ts).

1. **The contract.**
   - `actorName` is the actor user's current display name (`identity.users.display_name`).
   - It is null for a system actor (`system-demo`, `system-restriction`, `system-scheduler`), which matches no user.
   - It is not recorded with the entry. The role stays as recorded at the time (AT-A16-B ④), while the name follows the user's current name.
   - The same field reaches every `AuditView` list: audit.list, `Restriction.events` and `OffsetRecord.eventHistory`.
2. **Customers never see a name.**
   - The client projection of restriction events (IR42) already masked the actor as "masked". It now also sets `actorName` to null.
   - Offset records read by a client return `actorName` null in their event history. The actor ID there is unchanged.
3. **The backend.**
   - The audit log and the users both belong to the identity domain (IR196), so audit.list and the history query (`audit.History`, used by restrictions and offsets) add `LEFT JOIN identity.users u ON u.id::text = a.actor_id`.
   - The two `Event` copies in restrictions and offsets gain the field.
   - Swagger is regenerated from the annotations.
4. **The web.**
   - The audit list, the entry detail and the restriction exception screen's audit card show the name, and the ID when no name exists.
   - The entry detail adds an "Actor ID" row when a name is shown.
   - HQ offset events show who acted.
5. **Checked.**
   - Go: `TestAuditList` checks the name of an HQ entry and a system actor's null name. The restrictions test checks that the client events carry no name. `TestOffsets` checks that a customer's event history carries no names while HQ's does. `make test-all` passes.
   - Vitest: 52 files, 296 tests (audit rows with name and ID, a system actor, offset events with a name).
   - E2E: 67 passed, 9 skipped.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR306 The customer assistant reads voice.resolveIntent and speaks its chosen language — 2026-10-10

In API mode the header assistant (FR-X02, D09, IR65, Figma Client 09a–09g) was still a browser mock. It never called `voice.resolveIntent`, and it always answered in English (IR304 left this open). It now uses the Core API, speaks the language chosen in its header, and labels its commands.

1. **Reads and writes.**
   - Each question goes through the BFF to `voice.resolveIntent` with the panel's language as `locale`. The results are help, unsupported, candidates, temperature and change.
   - A temperature answer shows the latest reading with its time in the display time zone, and its quality when it is not valid. Without a reading it says so; it never shows a zero.
   - A change first shows the confirmation card (Figma 09c): the target with its place, the current setting, the requested setting and the allowed range. Nothing is sent before Confirm, and Cancel sends nothing (AT-X02-E ④).
   - Confirm is a Server Action: one `commands.create` whose `expectedUnitVersion` is the `expectedVersion` the intent was resolved at (AT-X02-N ③, IR09). It uses the same checks as Unit Control.
   - The command is followed until the device answers (`waitForCommand`):
     - while sending, the card shows the command, its state and the confirmed setting, which stays until the device acknowledges (09e);
     - an acknowledged command shows the acknowledgement time (09f);
     - a failed, expired or cancelled command says why and that nothing changed (09g). An expired one says "within 30 s", taken from the command's expiry.
2. **The fixed grammar.**
   - The suggestions are sentences of the grammar (D09): "temperature Bedroom", "set Bedroom to 24 degrees" and "help". In Malay they are "suhu Bedroom", "tetapkan Bedroom kepada 24 darjah" and "bantuan".
   - Figma's natural sentences ("What's the temperature in Bedroom?", "Set Bedroom AC to 24 degrees", "Help: how do schedules work?") are refused by the grammar, which matches room names, not AC names (IR65). Figma 09a–09g now use the grammar's sentences.
   - The help answer names the grammar of the panel's language.
   - No microphone is connected. The voice button shows a scripted transcript, which goes through the same grammar.
3. **Ambiguous rooms (Figma 09d, IR101).**
   - Candidates are grouped by room. The panel asks "Which Bedroom did you mean?" and lists each room with its ACs, then asks "Then: which AC in …?".
   - Nothing is chosen for the user. A room with one AC needs no second choice.
   - Continue asks again with `selectedUnitId`. The Core API accepts only a candidate of that request (IR65, AT-X02-B ③).
   - ACs with the same name in one room also show their ID.
4. **Language (D09, AT-X01-B ③).**
   - The panel starts in the display language, and its header switches between English and Bahasa Melayu.
   - A switch discards an unconfirmed change or room choice and keeps the typed text.
   - Times stay in the user's display time zone.
5. **Microphone consent (FR-X02, AT-X02-E ③).**
   - The Preferences switch is kept on this device (`ac-voice-microphone`), not on the account.
   - When it is off, the panel is text only: the voice button says so and moves the focus to the text box.
   - Turning it off while listening discards the voice request. Nothing is sent.
6. **The command source.**
   - `commands.create` takes `source?: 'ui' | 'voice'`, with ui as the default.
   - Only a client may send voice, because the assistant exists only in the Client app (IR115). HQ and technicians get VALIDATION `source` `error.notAllowed`. Any other value is `error.invalid`.
   - Unit histories label these commands "by voice".
   - `Command.source` already listed voice (IR216), but nothing created it.
   - service-contracts.ts, the operation catalog and the swag annotation are updated, and Swagger is regenerated.
7. **Fix.** In Chromium, `Element.scrollIntoView()` returns a Promise. The panel's arrow-function effects returned it, React called it as the effect's cleanup, and the page crashed ("i is not a function"). The effects now use a block body.
8. **Checked.**
   - Go: `TestCommands` creates the client's change with source voice. It checks that HQ and a technician may not send voice, that an unknown source is VALIDATION, and that a Unit Control command stays ui. `make test-all` passes.
   - Vitest: 53 files, 302 tests. `assistant.test.ts` is new. It covers:
     - the suggestions, which parse with the grammar in both languages, and the help in each language;
     - the temperature answers and the confirmation card;
     - the room → AC grouping, with IDs for equal names;
     - the progress in every state, and Malay / Tokyo.
     `units.test.ts` covers the "by voice" label.
   - E2E: `customer/assistant.e2e.ts` has three tests:
     - help, a temperature question, a cancelled change, an unknown intent, and the switch to Malay, which keeps the typed text;
     - the scripted voice transcript, and the microphone turned off in Preferences in another tab while listening, which leaves the panel text only;
     - an ambiguous room. The dev seed has no customer with two rooms of the same name (AT-X02-B uses an acceptance patch), so the spec rewrites the first answer into candidates; the choice goes to the Core API.
     No command is sent. 70 passed, 9 skipped.
9. **Progress.** Every business screen and the assistant follow the display language (IR258–IR306). Still English: the Phase 1A browser demo's fixture screens, including its simulated assistant.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR307 Test coverage: the BFF session and the DAL, every operation's input rules, and payment, restriction, policy and plan refusals — 2026-10-10

1. **Two fixes found by the new tests.**
   - `sessionFromTokens` accepted inherited names as a role, because `role in roleHome` follows the prototype ("constructor", "toString", "__proto__"). It now accepts only the object's own keys (`Object.hasOwn`). Tokens come from the identity provider's token endpoint, so this is defense in depth.
   - `coreAll` reads 100 items per page and could return up to 99 more than `max`. It now stops at `max`.
2. **Web unit tests.** Vitest: 56 files, 330 tests. These are new:
   - `session.test.ts` (15 tests):
     - the HMAC-signed cookie: every altered form, the server's secret, and a signed body that is not JSON;
     - the cookie options;
     - the session from the token's claims: missing claims, unknown and inherited roles, and a broken token;
     - the refresh grant: the request, the kept tenant and membership, a refused grant, a network failure, and no refresh token;
     - reading the cookie in a route handler: valid, refreshed and rewritten, cleared after a failed refresh, and missing or forged;
     - where sign-in returns to: absolute, protocol-relative, backslash and control-character forms are refused;
     - the identity provider settings.
   - `dal.test.ts` (9 tests):
     - the REST route with the token, tenant and membership headers;
     - the write headers, with a fresh Idempotency-Key when none is given;
     - a DomainError as CoreError, and an unreadable answer as UNAVAILABLE;
     - an operation outside the catalog, which calls nothing;
     - sign-in for a missing or expiring session;
     - whole lists: the cursor, filters and sort, 100 per page, never past `max`;
     - the principal, permissions, business clock and display settings, with their defaults.
   - `unitCommands.test.ts` (4 tests): `waitForCommand` polls every 2 s, stops at the first final state, gives up after 40 s, and passes a failed read on.
3. **Backend tests.**
   - `TestRequiredInputs` checks the input rules of 79 operations in 85 cases. The rules run before authorization: a missing ID, an unknown choice, a reversed period or an over-long text is VALIDATION with the field named. Path parameters are sent as the nil UUID, so the route matches.
   - Payments:
     - payments.simulate refuses a stale invoice or payment version and a second processing event;
     - a confirmed payment's reference cannot confirm another invoice's payment (`errors.reference_used`);
     - payments.recordManual and payments.confirm refuse a stale version, and payments.confirm refuses the manual payment's reference.
   - restrictions.schedule refuses an ineligible contract, a contract with no overdue invoice and an archived unit. The same request schedules once the causes are gone.
   - policies.save refuses an inactive customer, and the kind of a saved automation policy is fixed.
   - plans.generateNext:
     - asking for the old occurrence again is now asserted as `errors.occurrence_mismatch`;
     - an occurrence is generated once (`errors.occurrence_generated`), even when the next date is moved back to it;
     - an archived unit gets no new job.
   - Every demoSession operation answers UNAVAILABLE `errors.session_via_bff` in API mode, and `DomainError.StatusCode` (read by Echo's error handler) matches the status table.
   - `make cover`: 85.2 % → 86.4 % of 14,835 statements. `make test-all` passes.
4. **Remaining gaps.**
   - About 500 logic blocks are uncovered. They are mostly:
     - database error returns;
     - maintenance proposal and report rules (an assignment revoked on reschedule, an expired offer, report checks);
     - restriction reconcile paths;
     - the cluster's internal query errors;
     - process start-up.
   - Two kinds of branch cannot be reached through the API:
     - the NaN / Infinity threshold check, because JSON has no NaN;
     - the action check of payouts.transition and firmwareCampaigns.control, because their routes fix the action.
5. **No UI change.** E2E: 70 passed, 9 skipped.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR308 Uploads are checked by their bytes; the remaining input rules, reschedule, partner proposal, report and reconcile paths tested — 2026-10-10

1. **Uploads are what their bytes are.**
   - The Core API checked an upload's declared type, name and size, but not its bytes. A file declared `image/png` could hold anything and would be stored and served as a PNG.
   - Now the leading bytes must match the declared type: the PNG signature, the JPEG start-of-image marker or the PDF header (`ContentMatches`). This applies to report photos (attachments.add), the sign-off's signature and site photo, the photos of jobs.reportProblem, and certificate files. A mismatch is VALIDATION `error.invalidFile`, as before.
   - The browser's `File.type` follows the file name, so the four Server Actions that upload now send the type of the bytes (`uploadType`, `lib/files.ts`). A JPEG photo renamed to `.png` is therefore still accepted, as a JPEG. A file that is none of these keeps the browser's type, and the API refuses it.
   - The refusal reads "Site photo is not an accepted file — check its type and size" (`error.invalidFile` in `actionMessage`, with Malay). Before, it showed the raw key.
   - The frontend contract already asked attachments.add to validate the content (DDC attachments.add row). The API now does, for every upload.
2. **Backend tests.**
   - `TestRequiredInputs` gains 29 cases: wrong shapes, references inside lists and kind-specific fields. They include locations.rename, properties.save, units.save, automations.save, capabilities.save, baselines, factors, offsets.preview and offsets.simulate, mrv.saveDraft, energy.summary, clientUsers.save, members.save, the acknowledgement's other time, a manual check-in, a follow-up without a date, an invalid site photo, the three policy kinds, and telemetry.series (the REST binding refuses an unknown query parameter first).
   - `TestRescheduleAfterContractorAssignment`:
     - HQ may propose another time for an assigned job only after the technician answered "can't make it";
     - when the customer accepts it, the contractor's accepted offer moves to the new visit, the assignment is revoked as rescheduled, and the job goes back to accepted.
   - `TestPartnerProposalRefusals`: a time that breaks the slot rules, an expired offer, a reply-by after 7 days, and sending the contractor's time while HQ's own proposal is pending.
   - `TestSubmitErrors` (unit): every IR100 submit check of a draft.
   - `TestRestrictionReconcilePaths`:
     - a scheduled restriction has nothing to reconcile;
     - a unit whose device carries no restriction becomes not applied, and is not reconciled twice;
     - a releasing unit whose device still carries the restriction gets its remove command at once.
   - Uploads: a non-PNG declared as PNG, a PNG declared as JPEG, and a non-PDF certificate are refused.
3. **Web unit tests.** `files.test.ts`: the type of PNG, JPEG and PDF bytes whatever the name says; anything else keeps the browser's type.
4. **Cannot be reached through the API.** These branches stay untested: `retry_device` without a device (the device ID is in the route), the query filters of telemetry.series (an unknown parameter is refused first), the NaN threshold, and the fixed actions of payouts.transition and firmwareCampaigns.control (IR307).
5. **Checked.**
   - `make cover`: 86.4 % → 87.0 %. `make test-all` passes. Vitest: 57 files, 332 tests.
   - E2E: 70 passed, 9 skipped. The suite uploads no files; the upload rule is covered by the Go tests and `files.test.ts`.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR309 Contractor quality figures, import, payout and alert evaluation tests; the Phase 1A job actions stay local — 2026-10-10

1. **The Phase 1A job actions stay local.**
   - Six actions of the demo job store (cancel, partner accept and decline, the technician's accept and "can't make it", and the customer's note) asked `/bff/session` whether the app ran on the Core API, then either called it or the local store. This was left over from before the job screens were wired.
   - That detour made them asynchronous and always return nothing. A refused note ("CONFLICT — notes are only possible on open jobs") never reached the note dialog, which closed as if the note had been sent.
   - With DATA_SOURCE=api every job screen writes through Server Actions and none of these actions is called. The detour is removed, so the demo actions run in the tab and return their refusals at once.
2. **Backend tests.**
   - `TestContractorQualityMetrics` uses two completed jobs of contractor B on two units: one arrived in its window and was accepted at once, the other arrived late, was returned once and got a rework visit. It checks:
     - the arrival and first-time-fix breaches of the second job only, 50 % in the totals, and the customer status "breached";
     - the contractor's 90-day KPIs (arrival in window, first-time accepted, rework rate between 0 and 100 %).
     A later job on the same unit within 30 days also counts as not fixed the first time, which is why the two jobs use different units.
   - `TestUnitsImportRefusals`: an inactive customer at preview and at commit, a header that is not CSV, a unit name over 120 characters, a device with an unresolved tamper, and a preview that went stale because its new property was created meanwhile.
   - Payouts:
     - approving on a stale version, approving twice, and a question or answer on a stale version;
     - a question on a paid statement;
     - a contractor without a rate card gets lines noted "no rate card" with no amount (`TestPayoutWithoutRateCard`).
   - Unit tests:
     - the four threshold operators at the boundary, and the recovery side of a high and a low limit (`TestCompareAndRecover`);
     - the busy time of a day from overlapping, contained, touching, separate and unsorted slots (`TestUnionMinutes`).
   - `make cover`: 87.0 % → 87.3 %. Integration tests: 125; unit tests: 75. `make test-all` passes.
3. **Web unit tests.** Vitest: 59 files, 345 tests.
   - `jobs.test.ts` (10 tests) covers the demo maintenance flow:
     - a request with three preferred times, and booking or offering;
     - HQ's proposal, accepted, declined with new times or withdrawn;
     - a contractor's time forwarded and approved, or the agreed time kept;
     - another time for a periodic visit;
     - notes, refused at once on a closed job;
     - follow-ups classified once;
     - the contractor's and technician's answers, the customer's cancellation, read notices and the reset;
     - availability and slot texts.
   - `clientUsers.test.ts` (3 tests) covers the invitation e-mail checks, re-sending to invited users only, and keeping one active owner.
4. **No API-mode change.** E2E: 70 passed, 9 skipped. The suite runs on the Core API; the demo screens are covered by the unit tests.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR310 HQ can change a signed-in client user again; release, forecast, campaign and internal query tests — 2026-10-10

1. **A signed-in client user could not be changed.**
   - When HQ changes a client user who has signed in (role or status, clientUsers.save, FR-A17), the membership is kept in step. Its `valid_until` was set with `CASE WHEN $3 = 'disabled' THEN $4 ELSE NULL END`.
   - Inside that CASE, PostgreSQL took the parameter's type as text, and the update failed: "column valid_until is of type timestamp with time zone but expression is of type text". Every such change answered 503.
   - Only invited users, who have no membership yet, could be changed. The tests changed only those, so the fault went unnoticed.
   - The parameter is now cast (`$4::timestamptz`), as the other CASE updates of the code already were.
   - `TestClientUserMembershipSync` checks it:
     - disabling ends the membership now and raises its scope version;
     - activating opens the membership again;
     - removal ends it, and needs the current version;
     - a user cannot move to another customer.
2. **Backend tests.**
   - `TestRestrictionExplicitRelease`: restrictions.release requests the release itself once every cause invoice is paid (IR35), on the current version only, and the audit log records it.
   - `TestForecastWithoutReadings`: a unit set without valid readings gets the forecast warning `actual_unavailable` and no predicted actual (IR78).
   - Firmware campaigns:
     - a device with an unresolved tamper is refused (`error.tamperUnresolved`);
     - a device on a unit of another model is refused (`error.otherModel`). The old "other model" case named a model that does not exist and got 404, so it never reached this check. It is now named "unknown model".
   - Unit tests:
     - the internal queries between domain services (IR192): run in-process for a served domain, UNAVAILABLE without a transport, local without a registry, and the errors of a missing registry, an unknown name and an undecodable input;
     - their HTTP transport: the caller's tenant, membership and token, the internal token, correlation ID and business time on the request, the system path for a worker without a user, a DomainError passed through, and anything else UNAVAILABLE;
     - the four wordings of a filter cleaning reminder's evidence.
   - `make cover`: 87.3 % → 87.4 %. Integration tests: 128; unit tests: 78. `make test-all` passes.
3. **No web change.** E2E: 70 passed, 9 skipped, against the rebuilt backend.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR311 Every SQL statement of the Core API runs in a test — 2026-10-10

IR310's fault was a statement that no test had ever run. This round looked for more of that kind.

1. **Parameters without a type.** Every parameter in a CASE arm, COALESCE, NULLIF, GREATEST / LEAST, `IS NULL` or a select list was checked. None other than IR310's was at risk:
   - the NULLIFs feed text columns;
   - each COALESCE takes its type from a column;
   - the other CASE arms are cast.
2. **Statements no test ran.** A scan of the coverage profile lists the SQL statements (UPDATE, INSERT, DELETE, SELECT, WITH) that sit in blocks no test executed. Four of them were in the modules, and they are tested now:
   - the QualificationGranted consumer inserts a grant when the technician holds none for the code (`TestCertificateCreatesMissingGrant`; before, only the extension of an existing grant ran);
   - demo.trigger `command_sent` marks a requested command sent with its time;
   - members.save checks a property scope against identity's copy of the properties, both a known and an unknown property;
   - jobs.requestReschedule for a periodic visit that is offered but not yet assigned reads the open offer's visit for the 48-hour rule (`TestRescheduleOfferedPeriodicVisit`).
   Only process start-up, the seed and the migration runner remain outside the tests.
3. **The standing rule.** Backend design §9 now requires every SQL statement of a module to run in at least one test. The coverage scan checks it after `make cover`.
4. **Checked.**
   - `make cover`: 87.4 % → 87.5 %. Integration tests: 130; unit tests: 78. `make test-all` passes.
   - No production code changed, so the web build and E2E are unchanged (70 passed, 9 skipped, IR310).

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR312 The customer overview shows the next run again; the web's calls are checked against the contract — 2026-10-10

1. **The overview never showed a schedule's next run.**
   - In API mode the Automations card of `/customer` (Figma Client 01a, "Next run: today 18:00 · Bedroom AC") asked automations.nextRuns with `count: 4`. The API fixes the count at 8 and answered VALIDATION.
   - The loader swallowed the refusal, so the card never named the next run. The automations screen asked for 8 and worked.
   - The overview now asks for 8.
   - `customer/automations.e2e.ts` switches its new schedule on and checks the overview's "Next run: …" before deleting the schedule. With the old loader the spec fails.
2. **The web's calls against the contract.** `contract.test.ts` (Vitest) checks every coreOp / callOp / coreAll call that has a literal operation and an object-literal input — 368 calls. It reads the contract with the TypeScript checker (`OperationContracts` of service-contracts.ts) and the filters from the API's generated catalog. A call may use only:
   - operations of the contract;
   - their input fields (the API refuses unknown fields);
   - the filters of the operation's catalog row (the REST binding refuses others);
   - the literal values the contract fixes, such as `count: 8`.
   Before the fix the test fails on the overview's `count 4`.
3. **Contract corrections.**
   - The audit filter `targetKind` is a string, like `AuditView.targetRef.kind`. The audit log records more kinds than notifications target, such as property, space, automation and certificate.
   - `BlobInput.bytes` and `DocumentInput.bytes` are base64 strings in JSON.
4. **Findings for the next rounds.**
   - 76 of the web's 111 write operations never reach the API in the E2E suite, which changes no data. Their API side is covered by the Go tests, and their mappers by the unit tests.
   - Typing `coreOp`'s input by `OperationContracts` was tried as an experiment and reverted. About 50 call sites need tighter types, for example plain strings where the contract has a union of literals, or mappers that return `Record<string, unknown>`.
5. **Checked.** Vitest: 60 files, 346 tests. Typecheck and lint pass. E2E: 70 passed, 9 skipped. The dev data is unchanged; the spec deletes its schedule.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR313 Every Core API call of the web is typed by the contract — 2026-10-10

IR312 found a call the API refused (`count: 4`) with a check that sees only literal inputs. Now tsc checks every call.

1. **The contract types in the web.** `make gen` (service/api `cmd/gen/ops`) copies service-contracts.ts into `shared/lib/contracts.gen.ts`, as it writes routes.gen.ts from the catalog. Its test fails when a generated file is stale.
2. **Typed calls.**
   - `coreOp`, `coreAll`, `callOp` and `useOp` take `[operation, input, …]` as one of the tuples of `OpArgs` (`shared/lib/opTypes.ts`), one tuple per operation.
   - The literal operation name picks `OperationContracts[op].input`. Callers still name only the result type, as in `coreOp<ApiCommand>("commands.create", …)`.
   - `coreAll` takes the operation's `filters` and `sort`. The job writes of `jobsApi` take the operation's input without its `jobId`.
3. **Typed mappers and actions.**
   - Twelve shared mappers return the contract's input type:
     - alert policies of HQ and the customer;
     - automation policies and their evaluation input;
     - the audit query;
     - the customer's automation save input and test facts;
     - baselines;
     - device models;
     - memberships;
     - the contractor's job list filters and sort;
     - the technician's report draft.
   - The Server Actions take the contract's input or field types. A form's or an API row's value is narrowed where it enters a call, because the API validates it.
   - The device units are typed by `UnitSymbol`; the web's unit table already matches it.
4. **What the typing found.**
   - A unit test sorted units.list by `displayName`, which the catalog does not allow. It now sorts by `createdAt`.
   - The contractor's accept and decline in `jobsApi` sent an undefined offer ID and terms version when the job had no open offer. They now stop with an explicit error first.
   - The technician's acknowledgement and manual check-in sent `reason: null`. They now leave the reason out when there is none.
   - A baseline is built as the contract's two cases: a fixed baseline with its kWh, or a period comparison without it.
5. **Checked.**
   - The typecheck of the four apps, the shared package and the E2E passes. Lint passes.
   - Vitest: 60 files, 346 tests. The cmd/gen/ops test passes.
   - All four apps build. E2E: 70 passed, 9 skipped. The dev data is unchanged.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR314 The web's result types are checked against the contract — 2026-10-10

IR313 typed every call's input. The results were still typed by hand in the web, and nothing compared them with the contract. A new check does, and it found faults in the contract and in the web.

1. **The check.** `shared/lib/__tests__/results.test.ts` (Vitest) runs on every coreOp / coreAll / callOp / useOp call with a literal operation. The result type the caller names, or TypeScript infers, must accept `OperationContracts[op].result`; for coreAll, an item of its page. The rules:
   - Every field the web reads is in the contract, with a type the web accepts. A field the contract allows to be null must allow null in the web too.
   - Below the top of a result, every variant the contract allows must fit, for example the kinds of a command's action.
   - At the top, and for the items of a page, the request or the role picks the variant. The screen names the projection or kind it reads, and each variant it names must fit. This applies to a union with a field that tells every variant apart. A union without such a field needs one variant that fits (payments.simulate, where the event decides).
   - The check covers 384 typed calls. It fails if the earlier contract or the earlier web types are put back.
2. **Contract fixes (service-contracts.ts).**
   - `attachments.getContent` answered `Blob`, which the contract never declared, so it meant the browser's Blob. It now answers `BlobContent` `{name, mime, size, bytes}`, with the bytes in base64, as IR129 item 1 says.
   - The document validator now compiles the contract without the DOM library. Only the repository call's `AbortSignal` may be missing.
   - Narrowed to what the API accepts:
     - customer automations take `ClientCondition` (occupancy, location, pattern, weather; DD-C05);
     - HQ automation policies take `PolicyCondition` (occupancy, tariff, peak, solar, battery; DD-A11);
     - filter-care reminders take `FilterCareChannel` (in-app and e-mail; IR134 item 4).
     The API already refused the other values.
3. **Web faults found.**
   - **Restriction commands had no words.** The unit's command history showed a restriction's own commands as “undefined”. The seed's Lobby AC shows one: the restriction command that limits cooling to 24 °C. Now every command kind of the contract has words:
     - “Set ventilation {level}”;
     - “Apply restriction: {policy}”;
     - “Remove restriction”.
     The technician's command codes add `apply_restriction = min 24 °C` and `remove_restriction`.
   - **Ventilating rules could not be edited.** The customer automation editor turned a ventilate action into `fan:undefined`, so such a rule could not be saved again. The editor now keeps it. It offers ventilation on a model with a fresh-air function, as DD-C05 and DD-T10 allow (UnitAction within the unit's capabilities), and the technician's diagnostic control offers it too.
   - **Ended jobs could crash two technician screens.** Diagnostic control and alert evidence read the URL's job with `jobs.get`. After the job's window ends, that read answers the history snapshot, which has no ID, and both screens then failed on `job.id`. Now only the detail counts as the technician's job:
     - diagnostic control shows a job that is no longer the technician's as not found;
     - alert evidence shows no job;
     - the start time before the window is read only from the detail.
     The job page and the QR scan name the projections they read, and the scan's list of the user's open jobs skips history rows, which name no unit.
   - Smaller fixes:
     - HQ's job reads name the summary and detail projections they get;
     - the partner's preview no longer claims a field the API does not send;
     - the HQ invite resend no longer names an unused result.
4. **Fewer casts.** The web's sensor and reading metrics and units, and the currencies of invoices, payments, contracts and warranty claims, are typed by the contract's `Metric`, `UnitSymbol` and `Currency`. That removes the casts IR313 left at the screens. 318 other fields are still typed wider than the contract, such as `string` where it has a literal union. They are safe, because the check only requires that the web accepts every value the contract allows.
5. **Checked.**
   - The typecheck of the four apps, the shared package and the E2E passes. Lint passes.
   - Vitest: 61 files, 347 tests, with new tests for the words of every command kind, the ventilation option and its round trip, and the snapshot in the QR card.
   - All four apps build.
   - E2E: 71 passed, 9 skipped. The new `customer/command-history` spec reads the Lobby AC history. The dev data is unchanged.
   - The cmd/gen/ops test passes.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR315 The customer screens checked against Figma — 2026-10-11

The visible text of sixteen customer screens in API mode was compared with their default Figma states (Client 01a, 02a, 02e, 03a, 04a, 04c, 05a, 06a, 06e, 07a, 07j, 08a, 08b, 10c, 10d, 11a). Most differences come from the data: Figma shows its own demo rows, while the API seed is the fixture contract with fewer units. The differences that are not data are fixed here.

1. **The AC can be renamed on its screen (Figma 02e, DD-C03).** The unit screen now has ✎ Rename, as DD-C03 already described. It opens the rename dialog of Units & locations (Figma 02b / 02c), which now lives in the customer app's shared components. Saving calls `locations.rename` with kind `unit` and the unit's version. A duplicate name among the AC's siblings keeps the input, and so does a version conflict, which says the page now shows the current name. The new `customer/unit-rename` E2E spec renames Bedroom AC and gives it back its seed name, also in afterEach.
2. **A load cause is not a fault (Figma 06a, DD-C08).** The customer's alert inbox showed “✕ Fault” for every alert that was not an inspection record, a maintenance reminder or an air-quality alert. That included the seed's “Possible open window”. An open window or poor insulation (`causeCode` window_open / insulation_loss) is now “⌂ Load cause (possible)”, with Malay; an inspection record still says so first.
3. **Repair requests are for faults (IR243 item 5).** “Request repair” showed on every unresolved alert that was not a maintenance reminder, the load causes included. It now shows only for faults, as IR243 says and Figma 06a draws. IR243 named the button after Figma, so the Figma texts “Request maintenance” become “Request repair” in 02h, 06a, 06b, 06c and 06g.
4. **An offline AC offers a repair request (Figma 02h).** Below the disabled controls, the unit screen now says “Controls disabled while offline — try again when the unit reconnects, or request a repair.” It links to a new repair request for the unit, the same link as IR243's.
5. **The users page names its customer (Figma 11a).** The heading reads “Users of {customer}” and the invite dialog “Invite a member to {customer}”, from the session's organization name, in both languages. The users E2E spec checks both.
6. **Not changed, by design or by data.**
   - The unit's device information shows only what the contract has: the model, remote control and capabilities. Wi-Fi strength, firmware, protocol and uptime stay in the Phase 1A demo.
   - Figma 02e's Single AC / Group control switch is replaced by a sentence that points to group control in Units & locations (02m).
   - Wording that later IRs changed stays as built: the preferences hint (IR258), the consent text (IR84) and the notifications footnote (IR113).
7. **The dev data drifts with the E2E runs.** The specs restore what they change, but they leave records behind: every run cancels its scenario jobs (151 so far) and leaves their notifications (more than 1,400 for the customer). Filter cleanings that a spec marks cannot be undone, and each consent toggle leaves a revoke time. The customer's lists are full of these records. A reset of the dev DB to the seed, with the suite run on that fresh seed, is the next step.
8. **Checked.**
   - The typecheck of the four apps, the shared package and the E2E passes. This also fixes the type error that IR314's ventilation test left in the shared tests (a readonly fixture). Lint passes.
   - Vitest: 61 files, 349 tests.
   - All four apps build.
   - E2E: 72 passed, 9 skipped. The dev data is unchanged apart from the scenario's two cancelled jobs.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR316 The E2E suite passes on a fresh seed; the seed has an overdue filter — 2026-10-11

IR315 found the dev data full of E2E leftovers. The dev DB was reset to the seed (`scripts/resetdb.sh`) and the whole suite was run on it. Four specs had passed only on the drifted data, and two of them had never tested anything.

1. **Filter care settings.** The spec compared the card's text before and after a save and its put-back. On a fresh seed the first save turns “Default settings” into “Set by the account owner” for good, because a settings row cannot be removed. The spec now compares only the reminder values, from “Remind at” to “Edit reminders”. Its old 200-character window had also cut the card's note at different points.
2. **Request cleaning had never run.** It skipped when no AC was overdue, for two reasons:
   - it counted the buttons before the rows had streamed in;
   - the seed had no overdue AC.
   It now waits for the rows and finds the overdue row relative to its button; the old inner locator started at `main tr` again and matched nothing. It passes.
3. **The seed's overdue filter (Figma Client 07j, 06a).** `demoSeed.filterCleanings` in the fixture contract holds one cleaning by customer-a of Meeting room AC (unit-offline-rto) on 21 July. The AC is offline, so its run time is unknown, and the 30-day fallback makes it overdue. The reminder follows, as Figma shows. The seed applies the section (`internal/seed`), its test counts the row, and a consistency line records it. The fixture contract remains the only source of initial business data (IR69).
4. **The cross-app scenario.** The seed books tech-external-a for job-contractor-a from 14 to 20 Sept, so the customer's first preferred time (the next day) found them busy and the scenario timed out. Its job was left accepted and is now cancelled. The customer's preferred times now move to 21–23 Sept while the default date falls in that week. Before, a completion done by hand in R233 had freed the technician.
5. **The QR scan.** The one-tap labels need an open job, and on the seed only tech-external-a has one. The spec signs in as that technician and no longer skips.
6. **Seed lifetime.** The demo clock moves with real time: the clock table stores an offset, and the clock only moves further forward. Two seed dates bound it:
   - After 20 Sept (demo), the seed booking ends and the QR label part skips.
   - From 1 Oct, the technicians' fixture qualifications end and assignments fail.
   A reset to the seed brings the clock back to 14 Sept 09:00 (Kuala Lumpur). Each run leaves two cancelled jobs and their notifications.
7. **compose.yaml.** The gateway's comment still said it routes `POST /v1/ops/<operation>`, which IR223 retired. It now says the gateway forwards each operation's REST route (IR222). The migrate image is rebuilt with the new seed section.
8. **Checked.**
   - On a fresh seed, E2E: 73 passed, 8 skipped by design:
     - four clock moves, which are opt-in;
     - three client-only consent tests, in the other apps;
     - the technician's device fault, because tech-internal-a manages no device.
   - Go: make test-all passes on fresh test databases, the seed test included.
   - The dev DB holds the seed plus one run.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR317 The contractor and technician screens checked against Figma — 2026-10-11

The method of IR315 was applied to the default states of eleven contractor screens and nine technician screens. The technician screens were read as tech-external-a, the seed's technician with a job.
- **Contractor:** overview, jobs, job detail, schedule, unit, quality review, team, certifications, job history and one job's history, payouts.
- **Technician:** today, all assigned, unit register and monitoring, job workspace, alert evidence, diagnostic control, devices and device events.

Figma's demo shows ten contractor jobs; the seed has one (job-contractor-a). Most differences come from that.

1. **Contractor screens: no fix needed.** The cards, KPI tiles, columns, timelines, banners and dialogs match Figma. Two kinds of text differ:
   - wording that later IRs set: the history footnote (IR42) and Review & send before a diagnostic command (Figma 594:23802);
   - small labels such as “in date” for “certificates in date”.
2. **The technician's job workspace shows the site's access (Figma 73:5 / 423:604).**
   - The Job & unit card now has an **Access** row with the unit's `location.accessInstructions`, as written. Before, the card showed no way into the site, while the partner's job page and the unit register did.
   - The card also offers **Alert evidence →** and **Diagnostics →** for an active job inside its window: in progress, assigned or returned for rework. That is the rule of IR94 and IR283; before, the links appeared only once work had started.
   - The demo's link label follows Figma (“Diagnostics →”).
3. **The technician language spec walks a job.** It ran as tech-internal-a, who has no job on a fresh seed, so the walk through the workspace, unit, alert evidence and diagnostic control was skipped without a sign. It now signs in as tech-external-a, puts that user's language and zone back in afterEach, and checks the Access row and the unit links.
4. **Not changed, by design or by data.**
   - Readings the seed lacks: supply air, refrigerant pressure, vibration.
   - The evidence kind chips of alerts the seed lacks: measured, estimated, inspection.
   - The devices list's search field: its placeholder is not part of the visible text.
5. **Checked.**
   - The typecheck and lint pass.
   - The technician app builds.
   - E2E: 73 passed, 8 skipped by design. The dev data holds the seed and the runs' cancelled scenario jobs.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR318 HQ alerts as DD-A05 asks; the seed's timestamps — 2026-10-11

The method of IR315 was applied to the first nine HQ screens (Figma Admin): overview, customers, a customer's locations, unit edit, users, warranty & coverage, models, devices and alerts. hq-operator was the user. The overview, customers, locations, unit edit, users, warranty and models match Figma apart from data and the editor's layout.

1. **The Alerts tab as DD-A05 items 5 and 11 and Figma 256:2 / 534:16158 describe it.** It had kept the selection and the status in local state, had no filters, and offered only Acknowledge and Resolve.
   - The customer → property → unit scope, the severity, the status (the three tiles) and the selected alert are in the URL (`customerId`, `propertyId`, `unitId`, `severity`, `status`, `alertId`). The page passes the scope and the severity to `alerts.list`; the tiles count every status in the scope. A changed filter clears the selection. An empty list says so.
   - The detail has three cards:
     - the alert, with Acknowledge, Resolve…, **Request maintenance** and **Open unit →**;
     - **Why we think this**: evidence, cause, observation time and evidence records, with the inference note only for inferred evidence;
     - **Activity**: the timeline, that reading a notification does not acknowledge or resolve an alert and, for an alert without a policy, that no policy escalates it.
   - **Request maintenance** opens `/admin/jobs?new=<unitId>&alertId=<id>`. The Jobs tab then opens New job for that unit, its symptom taken from the alert: “Alert {id}: {title} — {evidence}”. Closing the dialog drops the parameters.
   - **Open unit →** opens the unit edit of the unit's customer.
   - `adminAlertRows` adds the unit, the customer (`customers.id`), whether a policy raised the alert, and whether its evidence is inferred.
   - The new `admin/alerts` E2E spec covers the filters, the empty status, the detail, both links and the prefilled New job; nothing is created.
2. **The seed's timestamps (IR91 item 2).** Rows the fixture gives no createdAt or updatedAt must carry `fixture.seedCreatedAt`, 1 Sept 2026. The seed left them to the column default, `platform.app_now()`, which outside the API is the wall clock. So HQ read “customer since Oct 2026” and a capability “updated 11 Oct 2026” while the demo clock stood at 14 Sept. Eleven tables were affected:
   - identity: organizations, users, memberships;
   - assets: customers, properties, spaces, units;
   - devices: capabilities, devices;
   - monitoring.alert_policies and billing.invoices.
   `seed.Apply` now sets the transaction's `app.now` to seedCreatedAt while it applies the fixture and clears it at the end. Rows with an explicit time keep theirs. The seed test checks those tables and that `app.now` is cleared; it fails without the change.
3. **A test the earlier round broke.** IR317 renamed the workspace link to “Diagnostics →” and left its Malay entry “Diagnostic control →”. The i18n test, which refuses entries the code no longer shows, failed, and that round had not run Vitest. The rebuilt alerts tab retired five more entries. All six are gone. Every round now runs Vitest with the typecheck before committing.
4. **Next.**
   - **Devices tab.** It still has its older layout: no state chips, search or sort, and no tiles, bound-unit card, per-sensor Calibrate → or operation history table (Figma 246:2). The technician's devices screen already follows that layout.
   - **Customer list.** For users without `restriction.read` it cannot show “Restriction applied”, although contracts list their `activeRestrictionIds`.
   - **Resolving with evidence.** It still needs the evidence records behind `evidenceIds` (IR292, open).
   - **The other HQ screens:** jobs and its tabs, contracts, billing, restrictions and their exceptions, automation policies, energy, MRV, offsets, audit, and access & roles.
5. **Checked.**
   - The typecheck and lint pass.
   - Vitest: 61 files, 349 tests.
   - All four apps build.
   - E2E on a fresh seed: 74 passed, 8 skipped by design.
   - make test-all passes, the seed test included. The migrate image is rebuilt.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR319 HQ devices as Figma draws them; a restriction without restriction.read — 2026-10-11

Two items IR318 left open.

1. **The Devices tab (Figma Admin 246:2, DD-A04 item 5).** It now draws what the technician's device screen draws, from the same mappers (`techDevices`):
   - **The list.** It is sorted by serial, with the state chips All, Online, Offline, Tamper and No sensors (HQ only), counts and the search “Search serial, device or unit…”. Each row shows the serial, the connection, how long the device has been silent, tamper, the unit, the firmware, and “no sensors” where there are none.
   - **The detail.** It shows:
     - the four tiles with their evidence lines: connection with last seen, the power signal, tamper and firmware with a newer version;
     - the **Bound to unit** card with the place and the model's capability version, and Rebind…;
     - the three actions with their notes and the firmware card while an update runs;
     - **Sensors**, each with **Calibrate →**;
     - the **Operation history** and the **Calibration history**;
     - the **Device events** with their recovery and response.
     The section titles are headings.
   - **What stays.** The register, rebind, calibration and firmware dialogs and their Server Actions are unchanged.
   - **What is removed.** `operationItem` and `calibrationItem` of `lib/devices` have no other user and go, with 15 Malay entries of the old layout.
   - **Tests.** `TechDeviceRow` gains its sensor count, and `filterOf` the `nosensors` state. The new `admin/devices` E2E spec checks the order, the Offline and No sensors chips, the search and the detail. The HQ language spec checks the new headings.
2. **An active restriction without restriction.read (Figma Admin 223:2).** The customer list and the customer header read restrictions only with restriction.read. hq-operator holds none, so Demo Customer A showed “‼ Overdue” without its applied restriction. Contracts list their `activeRestrictionIds`, which this user may read. With them, the standing now marks **Restriction active**, its state unknown. The header's billing tile says so too, with no link to the restrictions screen that the user cannot open. The new `admin/customers` E2E spec checks the row and the header.
3. **Checked.**
   - The typecheck and lint pass.
   - Vitest: 61 files, 349 tests, with the No sensors filter and the restriction fallback.
   - All four apps build.
   - E2E: 76 passed, 8 skipped by design.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR320 The HQ audit explorer as SCR-A16 and Figma describe it — 2026-10-11

The audit screen (FR-A16, DD-A16, SCR-A16, Figma Admin 337:2, 338:2, 338:332, 338:602 and 340:2) had three gaps. It had no actor or target filter. It showed related records only in the demo. Its device tab fell back to the first device, which IR33 forbids.

1. **URL keys.** The log tab keeps `from`, `to`, `actorId`, `targetKind`, `targetId`, `correlationId`, `result`, `limit` and the open entry `entryId` in the URL. `correlationId` replaces the former key `corr`. The device tab keeps `deviceId` and `eventId`, and uses the same period.
   - **Actor filter.** It lists the users of the memberships the session may read (`members.list`, where readable), the actors of the shown entries and the URL's actor.
   - **Target filter.** It offers the business kinds alert, automation, command, contract, device, device_event, invoice, job, membership, payment, policy, restriction, unit and user, with the kinds of the shown entries.
   - **Filter by ….** In the open entry, **Filter by actor →**, **Filter by ID →** (the correlation) and **Filter by target →** keep that entry open, because it matches the new search. A target ID shows as a chip that clears it.
   - **New searches.** Any other filter change starts from the newest entry.
2. **The open entry.** It shows:
   - the actor's name and ID;
   - the recorded role, "kept as recorded, not rewritten by the current membership";
   - the time, the correlation ID, the target with the versions it moved between ("version 3 → 4") and the reason;
   - the masked before/after values.
   The page then reads `audit.list` once more for the entry's correlation ID, a day either side, oldest first. That read gives two things:
   - **Correlation trace.** When there are several entries, the trace lists them all. A pending entry and its final result are separate entries; earlier entries are never edited.
   - **Related records.** These are the restriction, command, job and unit of the entry and its correlation, the entry's own target first. Restriction, job and unit open their own screens, and each screen checks its own permission. The command is read with `commands.get` only when the user clicks **Load command**. A restriction the entry changed shows the state and exception end that the entry recorded, so the screen reads no restriction.
   `/admin/units?unitId=` without a customer now redirects to the unit under its customer.
3. **Why nothing shows.**
   - A reversed or too long period searches nothing and says so.
   - A correlation ID that matches nothing says that an unknown ID and an out-of-scope ID look the same.
   - Otherwise the list says that no entry matches the filters.
   - An `entryId` outside the results says so, instead of opening another entry.
   - A read-only banner states that entries are appended only by business events: `audit.audit_log` rejects updates and deletes. Before/after values are masked, and filters, but never reasons or values, go to the URL.
4. **Device events (Figma 340:2).**
   - **No fallback.** Without a `deviceId` the tab asks the user to pick a device. An unknown or out-of-scope `deviceId` says it is not in the list.
   - **Device list.** It comes from `devices.list`, with the unit names of `units.list` where readable.
   - **Events.** `devices.events` runs only for the picked device and the period.
   - **Event list.** Each event shows its title, its type code, the evidence source, the sequence and the time, and its recovery.
   - **Event detail.** It shows the type, the evidence, the sequence with any gap ("42 (previous 40 — 1 missing)"), the unit at the time, the times and the recovery. It also shows:
     - the event's alerts, through `alerts.get` where readable, with **Open alert →**;
     - **Filter log →**, for the response notes' audit entries (`targetKind=device_event`);
     - the response notes, read-only here. The screen does not claim to mask the text of notes, which device.write users write; Figma 340:2's footnote no longer says so either.
5. **Code.**
   - `lib/audit` gains `correlationQuery`, `correlationTrace`, `relatedRecords`, `actorOptions`, `targetKinds`, `auditDeviceEvents` and `periodInstants`.
   - `deviceEventItem` and its five Malay words go.
   - `lib/techDevices` exports `eventCause`.
   - SCR-A16 adds `members.list`, `units.list` and `alerts.get` and the new URL keys. The review regression check's audit mutation follows the new read list.
6. **Checked.**
   - The typecheck and lint pass.
   - Vitest covers the filters, the query, the related records, the trace, the actor and kind options and the device event words, in both languages.
   - The new `admin/audit` E2E spec checks the URL state, the Filter by links, the chip, the empty states and the device tab. The HQ language spec checks the banner and the device tab in Malay.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR321 HQ access, billing and restrictions: the filters and links Figma and the DDs name — 2026-10-11

R311 compared the HQ frames 10–25 with the screens. Most differences came from data the fixture does not seed (IR69). Three screens also missed filters or links that their DDs and Figma name.

1. **Access & roles (DD-A03, SCR-A03, Figma Admin 539:16956).**
   - **Organization filter.** The organization filter "Organization: All" now sits beside the role and status filters. Its URL key is `organizationId`, and it is a `members.list` filter.
   - **Search-selects.** The filter and the "+ Add scope" target are search-selects, as DD-A03 names them. Typing narrows the list to the first 20 matches; the arrow keys and Enter choose, and Escape closes. `SearchSelect` of `components/ui` is an ARIA combobox with a listbox, built in house (DEC-03).
   - **Lists read in full.** The list and the users of a new membership now read every membership (`coreAll`). Before, the role filter also narrowed the users that a new membership could be for.
   - **A membership outside the list.** A `membershipId` that the list does not hold now says "That membership is not in this list", with Clear the filters when a filter is set. Before, the screen opened the first membership in its place, so an edit could land on the wrong person.
   - **Notes.** The role and permission notes use Figma's fuller wording.
2. **Billing (DD-A08 steps 5–6, SCR-A08, Figma Admin 38:7).**
   - **Billing months.** The billing months `from` / `to` are URL keys: YYYY-MM months in Kuala Lumpur, with the first not after the last. They go to `invoices.list` as from / to on the period start. With a period, the paid tile reads "Paid in period" with the months, such as "Aug – Sept 2026"; without one, it counts the whole scope. A reversed range is not sent and says why.
   - **Related restrictions.** Each restriction the invoice causes shows its short ID and **Open restriction →**, which opens it on the Restrictions screen (`/admin/restrictions?restrictionId=`). Before, the full UUID linked the exception screen. The inquiry detail links its restriction the same way.
3. **Restrictions (DD-A09 items 5–6, SCR-A09, Figma Admin 38:8).**
   - **Cause invoice filter.** The `invoiceId` filter was reachable only from links, and showed as a chip. It is now the "Cause invoice" select, which lists the invoices that restrictions cite, with their customers. An invoice that no restriction cites, opened from a link, still shows and says so.
   - **Details →.** Each unit of the selected restriction links its unit screen with **Details →** for asset.read holders.
4. **Code.**
   - `lib/billing` gains `billingMonths` and `monthsText`.
   - `components/ui` gains `SearchSelect`.
   - The Malay dictionary gains the new texts and loses the four replaced ones.
5. **Checked.**
   - The typecheck and lint pass.
   - Vitest covers the billing months and their words in both languages.
   - New E2E specs:
     - `admin/access` checks the search-select by mouse and keyboard, the narrowed list and a membership outside the list;
     - `admin/billing` checks the months, the empty month, the reversed range and the restriction link;
     - `admin/restrictions` runs as the restriction manager and checks the cause invoice filter, Details → and an uncited invoice.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR322 HQ contractor payouts and contracts as Figma draws them — 2026-10-11

Two more of the HQ frames that R311 compared.

1. **Contractor payouts (FR-A23, DD-A23, Figma Admin 699:22119).**
   - **Filters.** The statement period, contractor and status are URL keys (`period`, `contractorOrgId`, `status`) and `payouts.list` filters. The default is every period: DD-A23 named the current month, but that month has not ended, so its statements can neither be generated nor paid. A `statementId` the filtered list does not hold says so.
   - **Statements table.** The table has the columns Contractor / statement (with the period and job count), Queries (with the open ones), Net, Status and Review / Open. The status carries its date, such as "Approved · pays 15 Oct 2026" or "Paid 15 Sept 2026". The footnote explains how drafts are made and approved.
   - **Generate drafts.** Generate drafts opens a dialog. It offers the last month that has ended in Kuala Lumpur, and refuses a running month before any call; the backend refuses it too (`errors.period_not_ended`). The dialog says that generating again rebuilds the drafts, while approved and paid statements stay. This replaces the free-text month field.
   - **Detail.** The detail names who approved the statement (`approvedByMembershipId`, through `members.list` where readable). While Mark paid is locked, it notes that it unlocks on the pay date.
2. **Contracts (FR-A07, DD-A07, Figma Admin 106:4).**
   - **Scope and plans.** The scope names its contract count, and each plan tab shows its count in the scope.
   - **Rows.** Each row marks its eligibility: "Eligible · demo-v1", plus "· restriction active" while one is, or "Not restriction eligible". The screen cannot tell an applied restriction from a scheduled one without restriction.read, so it says "active" (IR319).
   - **Editor.** The editor has the sections Customer & units, Plan (RTO — Rent to Own, General — maintenance, Energy, Environment) and Restriction eligibility, with the hints Figma shows. The heading names the customer and the active restrictions.
   - **Save.** Save reads "Save as version N". The SR19 banner opens the restriction on the Restrictions screen.
3. **Code.** `lib/billing`:
   - statement rows gain `statusText`, `queryCount` and `openQueries`;
   - the detail gains `approvedBy` and `paidAt`;
   - `lastClosedMonth` is new;
   - `ApiStatement` follows PayoutStatement (`approvedByMembershipId`, `paidAt`).
   SCR-A08 adds the URL keys `period`, `contractorOrgId` and `status`.
4. **Checked.**
   - The typecheck and lint pass.
   - Vitest covers the statement texts, the approver and the last ended month.
   - New E2E specs: `admin/billing` gains the payouts filters, the dialog's refusal of a running month and a statement outside the list; `admin/contracts` checks the counts, the eligibility marks, the restricted editor and an empty scope. The HQ language spec reads the plan tabs with their counts.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR323 HQ baselines and automation policies as Figma draws them — 2026-10-11

Two more of the HQ frames that R311 compared.

1. **Baselines (FR-A13, DD-A13 item 6, Figma Admin 316:2).**
   - **Scope and list.** The scope names its baseline count. The list reads "Baselines · all customers", or the chosen customer.
   - **Rows.** Each row shows its version with its quality: "v1 · modeled" for demo_fixed and "measured" for demo_period_comparison (SR29). Below come the units and period, then the method, value and boundary.
   - **Editor.** The heading carries Version N and "Editing → vN+1", and says that the version number is assigned automatically. The units are "the same set required for comparison", and the period ends "Up to 366 days". Method and boundary are segmented choices; the boundary shows as "AC input electricity" or "Whole-building electricity". The baseline energy and the texts have the hints Figma shows.
   - **Warning.** The note says that MRV reports keep the version they reference, and that nothing is labelled adjusted without an adjustment model.
   - **Save.** Discard and "Save as version N" replace "Save as vN"; "+ New baseline" replaces "+ New".
2. **Automation policies (FR-A11, DD-A11 items 5–6, Figma Admin 106:5 / 361:7456).**
   - **Editor.** The editor groups its Basics. Its footer says "No unsaved changes · form loaded from policy vN", or that saving creates the next version, beside Discard and Save policy.
   - **List.** Show disabled hides disabled policies; the open one stays listed.
   - **Simulate.** Every fact row now chooses its unit and its kind. "+ Add fact" adds another fact, for example a missing solar reading beside the tariff (AT-A11-E/B), so other policies and customer rules can match. `evaluationInput` gives each fact its own metric and lists each unit once. "Run simulation" replaces "Simulate"; it still creates no command.
3. **Checked.**
   - The typecheck and lint pass.
   - Vitest covers a unit with two facts of different kinds.
   - New E2E spec `admin/automation`: it creates a disabled policy and checks the footer, Discard, Show disabled and a simulation with an added missing solar fact. Its afterEach deletes the policy again through the app's operation relay (`policies.delete` with the version).
   - The HQ language spec reads the baselines' count and their new button.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR324 HQ offset record filters; the HQ frames 10–25 are aligned — 2026-10-11

1. **Offsets (FR-A15, DD-A15 item 5, SCR-A15, Figma Admin 106:9).**
   - **Filters.** The records tab gains the Figma filter bar: customer, status and created — the last 7, 30 or 90 days of the business clock, or all, with 30 days by default. They are URL keys (`customerId`, `status`, `created`) and `offsets.list` filters; created becomes `from` on createdAt.
   - **A record outside the list.** A `recordId` the list does not hold now says so. Before, the screen opened the first record instead.
   - **Retire.** The button reads "Retire whole amount (demo)", because 1A retires a record only as a whole (DD-A15 step 2).
2. **The HQ comparison of R311 is done.** IR311–IR324 compared HQ frames 10–25 with the screens and closed the gaps in their code. The differences that remain come from data the fixture does not seed (IR69): plans, contractor profiles, payout statements, automation policies, MRV reports and offset records. Each of those screens draws the Figma states once such data exists. MRV keeps its period inputs; Figma's "Period: All" select is the same filter.
3. **Checked.**
   - The typecheck and lint pass.
   - New E2E spec `admin/offsets`: the filters in the URL, the default that leaves the URL, and a record outside the list.

No UI kit, icon set, or form, schema, query or translation library is added. Reads are Server Components, by the user's instruction to follow the Next.js documentation (2026-10-08).

## IR325 Coverage: business rules the Core API tests had not reached — 2026-10-11

`make cover` measured 87.5 % of 14,848 statements. The gap scanner lists the zero-count blocks, without the plain error returns. Among them were rules that no test exercised. They are tested now, and coverage is 87.7 %.

1. **SLA (DD-A22).**
   - **customerStatus.** A customer's standing is now a function of its own, `customerStatus`, with unit tests. The response share is held to 100 %. A share up to 10 points below its target is at risk; further below, or any open overdue job, means breached. A share without jobs says nothing.
   - **Target in effect.** `targetFor` and `targetViews` have unit tests too: the target in effect at a job's creation, the default without a version, and the later rows earliest first.
   - **Scorecard.** The scorecard test gains a job answered within its hours. Before, no test counted a response within the target.
2. **Slot proposals (IR128 item 1).** The proposal test gains these cases:
   - a hold that names the wrong side: internal without a technician, internal naming a contractor, or a contractor naming a member;
   - another company's technician (FORBIDDEN);
   - a suspended contractor (CONFLICT `errors.contractor_suspended`);
   - a contractor hold naming its own qualified technician, which succeeds and is withdrawn again.
3. **Report review (IR31).** The same user acting through another membership is still the report's author, so review availability says `self_authored`.
4. **Restriction retry (SR26).**
   - An apply retry is not applicable once every cause invoice is paid (`errors.restriction_not_applicable`), and it is refused once the restriction is applied (`errors.restriction_state`).
   - A release retry needs reconciliation for a unit waiting for reconciliation, or one never confirmed applied (`errors.reconcile_required`).
5. **Energy (SR09).** Without a current default emission factor, the emissions are null and `factor_missing` is reported. They are never shown as zero.
6. **Alert policies (D02, IR115).**
   - A policy whose owner membership is gone notifies nobody (`owner_forbidden`), and that check comes before the cooldown.
   - A default rule that the customer switched off no longer fires for their units.

Both Go suites pass (`make test-all`): 130 integration tests and 80 unit tests.

## IR326 Coverage: inbox scopes, schedule boundaries, diagnostic ends; two open questions — 2026-10-11

The second coverage round (IR325) adds Core API tests where business rules had none.

1. **Contractor inbox (IR58).**
   - A contractor reads the notifications of jobs offered to its company, and nothing else: not another job, and not a unit. Another contractor does not see them. This is the new test `TestContractorInbox`.
   - An alert message is previewed as a fault notification of warning severity.
2. **Schedule boundaries (IR54).**
   - A schedule rule whose unit was archived, or whose time zone cannot be loaded, is skipped, and the tick goes on.
   - When one schedule ends and another starts on the same unit in the same minute, the end is evaluated first. Its Command makes the unit busy (D04), so D02 excludes the next schedule's start as `busy`. The new test `TestScheduleBoundaries` records this current rule.
3. **Diagnostic runs (IR123).** When the device is offline at a run's end, the end action fails with OFFLINE and sends nothing.
4. **Open question — back-to-back schedules (for the product owner).**
   - **Today.** With the rule in item 2, a schedule that starts as another ends on the same unit does not run its start action: the unit keeps the earlier schedule's end state. IR54 names the order "an end before a start of the same minute", which suggests that the start was meant to take effect.
   - **Proposal.** When another schedule starts on the same unit in the same minute, its start wins: the earlier schedule's end is not sent for that unit. The run would be recorded as superseded, a new suppression reason.
   - **Status.** Nothing changes until the product owner decides.
5. **Open question — plan recurrence in UTC (for production).** D16 computes the monthly recurrence in UTC for Phase 1A, and the plan form says so.
   - **Effect.** A Kuala Lumpur plan due at 00:00 on the 1st is anchored to the UTC day, the 30th or 31st. Some of its later occurrences therefore fall on the last day of the previous month in Kuala Lumpur. The form shows the next occurrence before saving.
   - **Decision.** Computing the recurrence in the property's time zone is a production decision; nothing changes in 1A.

Both Go suites pass (`make test-all`): 132 integration tests and 80 unit tests. Coverage is 87.7 %.

## IR327 Resolution evidence: `alerts.evidence` and the evidence picker — 2026-10-11

IR66 lets an alert without a policy resolve only by hand, with a reason and evidence (DD-A05 item 6). Until now no read listed the records that `resolutionEvidenceIds` may cite. The web sent an empty list, and the Core API accepted any list, an empty one too.

1. **New read `alerts.evidence` (201 operations).** `GET /v1/alerts/{alertId}/evidence` returns `Page<EvidenceCandidate>` to a user who may resolve the alert: a technician with `alert.resolve` on an assigned unit, or an admin with `alert.resolve`. It takes no filters and sorts `observedAt desc; id asc`. The candidates are:
   - `detection` — each of the Alert's own `evidenceIds`, at its `observedAt`;
   - `remeasurement` — the unit's valid readings observed after `detectedAt`, measured or recorded on site and never estimated. They are of the rule's metric when the Alert has a rule, of any metric otherwise, at most 100, with metric, value, unit, origin and quality;
   - `device_event` — the `restored` events of the unit's device that carry the Alert, since `detectedAt` (Devices answers them).
2. **`alerts.resolve` checks the evidence.**
   - An Alert without a policy needs at least one ID (`resolutionEvidenceIds: errors.evidence_required`).
   - Every ID must be a candidate (`errors.evidence_unknown`). At most 20 IDs, each once (`error.invalid`).
   - A policy Alert may still be resolved by hand without evidence. The automatic resolution (IR285) is unchanged.
3. **HQ Resolve dialog (Figma Admin 259:2).** The dialog reads the candidates only while it is open (SCR-A05; this replaces the earlier `telemetry.series` read). It lists them as checkboxes under "Resolution evidence · required" ("· optional" for a policy Alert), with the evidence attached at detection last. The reason shows its count of 1000 characters. Resolve alert checks the reason and the evidence before anything is sent. A refused evidence ID is said in words and re-reads the list. The frame's rows and note now use the screen's words ("Remeasurement · Temperature 26.4 °C · today 9:20 am MYT", "measured · valid", "none attached").
4. **Technician resolution (DD-T07).** For an unresolved alert, a user with `alert.resolve` reads the candidates with the page (SCR-T07). The resolution card lists them above the reason in the same way, and a refusal of the evidence is said in words instead of the reason message. The Technician frames 02-14…02-17 show tech-internal-a without `alert.resolve` (02-16 is its FORBIDDEN), so they rightly show no picker; for a holder the card follows Figma Admin 259:2.
5. **Persistence.** `alerts.evidence` reads `monitoring.alerts`, `monitoring.measurements` and `devices.device_events`. `alerts.resolve` reads the same and writes `monitoring.alerts`.
6. **Tests.**
   - Go: `TestAlertEvidence` covers the candidates, their order and paging, the refused filter, the scope, each refusal of the resolution, a rule Alert's metric and a policy Alert resolved by hand. `TestDeviceLifecycle` checks a tamper recovery as a candidate, and `TestQueryCatalogFilters` checks the new row.
   - Web: `alertEvidence.test.ts` (Vitest) covers the rows, the heading, the checks and the refusals. The E2E test "the Resolve dialog requires evidence for an alert without a policy" opens the dialog on the seed's alert, finds its five readings and cancels.

Both Go suites pass (`make test-all`): 133 integration tests and 80 unit tests. Coverage is 87.7 %. Vitest passes 358 tests, and the E2E suite passes 86.

## IR328 Coverage round 3: paths no test reached — 2026-10-11

The third coverage round (after IR325 and IR326) adds Core API tests for paths that no test reached. No rule changes.

1. **Alert evidence (IR327).**
   - Candidates observed at the same time are ordered by ID, and both allowed sorts work.
   - A nil alert ID is refused.
   - A tamper alert is resolved through the API, citing the device's recovery.
2. **Work reports (IR100, SR07).** The new test `TestWorkReportEdges` covers:
   - a draft or a submission of a job that does not exist;
   - an item that cites an attachment of another report;
   - an empty reason, saved as none;
   - a measurement corrected in place (same ID, version 2);
   - a submission of a job not in progress, or of a stale job version;
   - the time on site without an ended pause (60 − 10 = 50 minutes);
   - a report read through another job, and a review of a job that does not exist.
3. **Partners (DD-A06).**
   - A contractor profile keeps its organization.
   - A stale profile or offer-status version is refused, and so is an unknown profile or contractor.
   - A blank rate-card note is none.
   - SLA targets of the same plan type and start are refused (`errors.targets_exist`).
4. **Offsets (DD-A15).**
   - A failed retirement retries the retirement: the record goes back to demo_purchased with a new retirement attempt, then is retired.
   - An unknown quote or record is NOT_FOUND, and so is a client's quote for another customer.
   - A reversed created period is refused.
5. **Ratings (IR110).** Re-rating a stale version is refused. A blank comment is none. A job that is not completed cannot be rated.
6. **Commands, campaigns, devices.**
   - A restriction policy that cannot be read allows nothing (IR46).
   - A command or a history of an unknown unit is NOT_FOUND, and so is control of an unknown campaign.
   - A unit holds one device (`error.unitHasDevice`), and an unbound device is not calibrated (`error.deviceNotBound`).
7. **Payouts (IR321).** A negative adjustment is a deduction of the next statement.
8. **Restrictions (IR42, IR46).**
   - The client's history leaves out the reconcile, which stays HQ's record.
   - A temperature limit on a model without a setpoint is refused (`errors.unsupported_capability`).
9. **Alert notifications (SR12).** A later event finds the policy's open alert again and records a failure of its own.
10. **Proposals (IR113).** The offer that an accepted contractor hold creates is open for 24 hours, or until the slot starts when that is sooner.
11. **Policies, members, jobs.**
    - A default rule switched without a reason records none.
    - A blank unavailability note is none.
    - A unit filter combines with a property filter.
    - jobs.get, jobs.events, commands.get, commands.list and policies.get require their IDs.
    - Holding a job that does not exist is NOT_FOUND.
12. **Unit tests.** `readStatus` (IR133 item 2) and `ContentMatches` (IR308).

The REST routes already refuse two paths, so they stay untested: a filter that does not decode, and the deviceId check of a device retry (the route carries the ID).

Both Go suites pass (`make test-all`): 135 integration tests and 83 unit tests. Coverage is 88.1 %.
