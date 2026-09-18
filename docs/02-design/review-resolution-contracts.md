---
document_id: DD-REVIEW-RESOLUTION
version: 0.21.0
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

Allow HQ users with only restriction.override to call restrictions.list/get within their management scope. Return RestrictionReleaseView (projection=release), containing only id/version/createdAt/updatedAt/unitIds/rulesVersion/policy/state/perUnit/recoveryCases and restriction times needed for the decision. Exclude invoice IDs, contract IDs, reminder text, and the full audit. Users with restriction.manage receive the existing Restriction. Open the A09 list entry to override-only users, with the path list → A10 get → override → get. Require the A09 create/apply sections and supporting contract/invoice/equipment reads only for manage holders. For override-only users, list filters support only status (invoiceId/contractId return FORBIDDEN), and counts/order are limited to the scope of this projection. Fetch audit.list only with audit.read.

Allow override-only users to reconcile and retry(phase=release) only for release_requested with an override release intent, or an unresolved recovery case on a terminal record. Reconcile during normal application, retry(apply), and schedule/execute/defer/exempt/cancel/release require manage. override requires a reason, the latest Restriction.version, and a new key. Read the version for reconcile/retry through get as well. Release Command status can be tracked through get.perUnit/recoveryCases, so no extra commands.get permission is needed. override always returns RestrictionReleaseView. Project reconcile/retry write responses and writes.getResult to RestrictionRead using current permissions, so a saved full response is not exposed after manage permission is lost. Also remove resourceIds outside the projection.

## IR04 Shared simulated reminder records

notifications.preview remains a read and saves no records. HQ A08 fetches an overdue unpaid Invoice, selects a customer recipient through notifications.recipients(target=invoice,templateKey=payment_reminder,channel), shows the preview, then calls invoices.remind after the explicit action "Record simulated reminder". Inputs are invoiceId/recipientMembershipId/channel/reason. expectedVersion is Invoice.version, and idempotencyKey is required. reason must be 1–1000 characters. Do not send externally.

In the same transition, the Repository checks billing.manage, invoice scope, latest version, now>dueAt, status=unpaid, and that the recipient is an active Membership of that customer eligible for the channel. An expired recipient returns VALIDATION; paid/processing/not yet overdue returns CONFLICT. On success, save one Notification and one audit record, and increment Invoice.version by 1. Notification has templateKey/type=payment_reminder; inApp is simulated and email/whatsapp are preview. All channels are saved in notifications.list. Return InvoiceReminderReceipt with only invoiceId/notificationId/invoiceVersion. Do not return the notification body to HQ users other than the recipient. The customer sees the same notification on C08 and the same Invoice on C11. A same-key retry returns the same receipt without creating duplicates. A reminder with a new key requires the latest Invoice version after checking it again. A read-only preview does not increment the Invoice version.

## IR05 Advance notice time and evidence

Remove noticeAt from restrictions.schedule input. The Repository uses the request acceptance time now as noticeAt and checks executeAfter>=now+24 hours. Generate an inApp restriction notice for every active client Membership of the target customer that can read the Contract and all target Units of the Restriction. Save their IDs in noticeNotificationIds. If there are no recipients, return VALIDATION with zero restriction, notification, and successful audit records. Save the Restriction and notifications atomically. The customer can see the notice in the notification list in the same tab. This is simulated notice, not proof of real delivery.

execute/retry(apply) rechecks that the notice IDs refer to the same Restriction, customer, and notice time; that both 24 hours after notice and executeAfter have passed; and that the original policy/rulesVersion/targets/cause-invoice conditions still hold. A change to the application details after notice requires a new notice. Reading the notification is not a required execution condition. Past fixtures are for initial seeds only and must generate notice evidence together. Normal operations/demo.trigger cannot insert a past noticeAt. Use clock advance to pass the 24-hour wait.

## IR06 Only one active payment attempt

Each Invoice may have at most one Payment in initiated or processing in total. Within the same transition, payments.simulate(event=initiate) checks that the Invoice is unpaid and has no nonterminal Payment, creates Payment=initiated, sets Invoice.status=processing/paymentStatus=initiated, and increments Invoice.version. The initial expectedVersion is the Invoice version. A same-key retry returns the original result. A different key returns CONFLICT even with the latest Invoice version and creates no additional attempt.

A processing event updates Payment and Invoice.paymentStatus to processing and increments both versions. confirm updates them to confirmed/paid; fail updates them to failed/unpaid. Keep Invoice.paymentMethod after failure. Retries after finalization do not increment versions. recordManual returns CONFLICT if either initiated or processing exists. A retry after failure requires the latest Invoice version and a new key. demo_instructions creates no Payment and does not affect this exclusion rule or version. Deliver state updates and subscription notifications after the same transition commits.

## IR07 Shared Policy form

All kinds on A05/A11/A12 require name (1–120 characters after trim), unitIds (nonempty, no duplicates), timezone (supported IANA name), enabled (boolean), and priority (integer 0–100). For new records, clearly show an empty name, no selected equipment, timezone=Preferences.timezone, enabled=false, and priority=50 in the UI. Editing uses fetched values; the Repository does not fill in missing values. Keep these defaults consistent with Automation. Kind-specific required fields follow the existing DD/SR28; saving is blocked when they are missing.

A05 obtains target equipment/capabilities through units.list → selected units.get, regardless of whether alerts exist. A12 also uses units.list → units.get. Candidate reads on A05 are within alert.policy.manage scope; on A12, within automation.policy.manage scope. Notification recipients use the eligible set for all selected targets/channels. Do not show the policy save form to read-only staff.

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

payment_reminder notifications.preview/recipients is limited to HQ with billing.manage. Also check target.kind=invoice, now>dueAt, and Invoice.status=unpaid on reads. An ineligible state returns CONFLICT, missing permission returns FORBIDDEN, and out-of-scope access returns NOT_FOUND. preview checks the same customer-recipient/channel conditions as IR04. If payment starts after preview, remind rechecks and rejects it. The preview Notification is unsaved; do not reuse its temporary ID in markRead or notification lists. On successful remind, the Repository issues an ID for the saved record.

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

A16 requires audit.list as a Query; do not show a failure as a normal empty result. Device events have a separate supporting panel that obtains authorized candidates from devices.list. Call devices.events({id:deviceId,query}) only after deviceId selection. With no selection, show guidance; with no candidates, show empty; on list failure, show retry within the panel. An invalid or invisible selected ID stops with not-found; do not substitute another device. Keep selected deviceId in the URL and restore it with the same steps on Back/Forward. audit.read candidate/history read permissions follow the existing operation catalog; do not additionally grant device.manage.

When navigating from an audit targetRef to an existing detail screen, use /admin/jobs?jobId=:id for kind=job, /admin/restrictions/:id for restriction, and /admin/devices?deviceId=:id for device, each requiring its existing read permission. For command, fetch commands.get separately only with its existing permission, then use its unitId to navigate to /admin/units?unitId=:unitId. A16 audit.read alone does not expand access to other resources. Resolve links through shared Navigation/feature hooks. Without permission or for an unknown kind, show only masked audit details. Deleted/expired resources show not-found at the destination; retain the original audit.

audit.list applies every filter to the authorized set. Both another tenant's correlationId and a nonexistent correlationId return items=[]/total=0/nextCursor=null. Do not search all tenants first and branch to NOT_FOUND. This differs from D01 NOT_FOUND for individual resource get operations. Only internal Repository business events add audit records; A16 has no write operation.


## IR34 User-confirmed permissions and job sorting

DEC-17: Restriction operations have two permission types. defer/exempt/cancel require restriction.manage; override requires restriction.override. Holding both allows both sets of operations, but neither implicitly grants the other. Keep existing manage conditions for schedule/execute/release and others, and IR03 conditions permitting only release tracking after override. Check both menus/buttons and the Repository; do not skip reason, scope, state, or version validation. A direct write without permission returns FORBIDDEN, makes zero business changes, and creates only the D01 denial audit. This fixes the FR-A10 inconsistency between list and detail; it adds no new permission.

DEC-18: Job lists offer sorting by status (business order), severity, and deadline, with ascending/descending directions. The default is status asc, with ties always broken by id/jobId ASCII asc. Fix status comparison order to these 10 states.

requested → offered → accepted → assigned → in_progress → on_hold → submitted → rework_requested → completed → cancelled

This is display order, not a table of allowed state transitions. status desc reverses that order. IDs within the same status remain asc even for desc. severity is normal < warning < critical; compare dueAt as UTC time, with null last in either direction. Do not sort by translated labels or only within the current page. Use authorization → IR23 public projection → filter → sort the whole snapshot → pagination. Use the public offer status and frozen history status, without reading private Job status. Reject unknown status with D01 UNAVAILABLE.

Expose JOB_STATUS_ORDER in the canonical types. This is a demo contract constant, not application implementation. The Repository default when sort is omitted is also status asc;id asc. Keep existing id sorting as an internal contract, but offer users only status/severity/dueAt. Apply this to every job list using jobs.list. Job candidate reads also use this default when sort is omitted.

The UI URL key sort uses field:direction (for example, sort=status:asc or sort=dueAt:desc). Omission means status:asc. Invalid fields/directions, duplicate sort keys, or empty values return VALIDATION and request condition correction. Reject id in the URL because it is outside UI choices. A selection change keeps other filters, updates the URL, removes the cursor, and fetches the first page under the new conditions. Back/Forward restores that URL's sort/filter; do not reuse an old cursor with new conditions. Include normalized sort in the Query key so a late response for old conditions cannot overwrite the new list. Subscription refetches keep the current sort selection. Language changes do not change ranks, IDs, or UTC values.

Label sort controls and make them selectable with a keyboard and on mobile. After selection, show loading for the new conditions; failures show error/retry and zero results show empty. Do not present rows in the old order as fetched in the new order. Reflect aria-sort in the corresponding column header. "Reset to default" resets only sort to status:asc and refetches from the first page, keeping other filters. KPIs do not depend on sort; do not pass sort to summaries.get.

## IR35 Release-request triggers and restrictions.release preconditions — FRV-001

Release requests (state=release_requested) may start only through routes ①–③ below and route ④ cancellation (restrictions.cancel) in IR96. All use the same internal transition function. ① Payment confirmation: in the same transition where payments.confirm/payments.recordManual/payments.simulate(confirm) makes all cause invoices (causeInvoiceIds) paid, move scheduled to cancelled and requested/applied to release_requested, saving releaseIntent={source:'payment',at:now,actorMembershipId}. ② Grace/exception: in the same transition where restrictions.defer/exempt runs on requested/applied, move to release_requested with source='exception'. ③ Forced release: restrictions.override uses source='override'.

In the same transition to release_requested, perform the D03 release evaluation once per perUnit. For online equipment with applyState=applied, create a remove_restriction Command and set releaseState=requested. not_sent/not_applied becomes not_required; sent_unknown becomes waiting_reconcile. Offline applied equipment uses releaseState=none/pendingReason=offline, not failed, and waits for explicit retry(phase=release). Create release Commands even when the caller is a client (Command.actorMembershipId is the Repository's internal actor 'system-restriction'; the audit actor is the person performing payment confirmation).

`restrictions.release` is an explicit release-request operation by a restriction.manage holder. Preconditions are state∈{requested,applied,release_requested} and either "all cause invoices are paid" or "exception/grace is active". Unpaid with no grace/exception returns FORBIDDEN (contract restriction violation). From requested/applied, perform the same transition as ② above with source='manual'. If already release_requested, idempotently return the current Restriction without adding Commands, audits (except denial audits), or versions. Only restrictions.retry(phase=release) retries or requests failed equipment again. "release" in AT-A09-N④ is this idempotent response; the transition to release_requested itself occurs during payment confirmation. AT-C11-N④ checks that client payment confirmation alone starts ①.

## IR36 Demo clock progression and session lifetime — FRV-002

On reset/reload, the demo clock starts from fixture.clock (2026-09-14T01:00:00.000Z) and advances monotonically at real-time speed. If to is at or after the current clock, demo.advanceClock({to}) jumps forward and processes deadlines reached during the jump (Command expiry, Offer/Assignment/qualification expiry, staleAfterSeconds, Fact TTL, predicted occurrence, DiagnosticRun endAt, cooldown) in time order using D04 event priority. If to is before the current clock, allow it as "initial time setting" only immediately after reset while business events have not advanced beyond the seed eventCursor; otherwise return VALIDATION (AT-C04-E③ performs this setting immediately after reset).

The 30-minute Session lifetime is measured with the demo clock, but advanceClock jumps do not consume it. When a jump commits, shift issuedAt/expiresAt of active Sessions by the same offset (also for initial time setting). User-triggered extension is only through IR55 demoSession.extend (IR79). Test session expiry through demo.trigger(session_expired) or elapsed time without jumps. Membership.validUntil, Offers, Assignments, qualifications, contracts, and invoice deadlines expire normally through jumps. Queries displaying resources expired by a jump increment viewEpoch and are discarded under IR24.

## IR37 Transport failure injection and network disconnection — FRV-003

Add eventType='transport' to DemoTrigger. Input is `{operation:OperationName, outcome:'UNAVAILABLE'|'TIMEOUT'|'RATE_LIMITED'|'DELAY', retryAfterSeconds:number|null, delayMs:number|null, remainingCalls:number}`. remainingCalls is an integer from 1–100; apply the outcome to the next remainingCalls calls of that operation and decrease it on each application. For outcome=UNAVAILABLE/TIMEOUT, the Repository rejects with DomainError{code} without running business processing (writes have zero side effects and do not record an idempotency key). RATE_LIMITED requires retryAfterSeconds (1–3600) and stops acceptance under D04. DELAY waits delayMs (1–60000) before normal processing. Use this path to inject D10 intentional slow responses (3000 ms) and the 10-second timeout display (12000 ms). Acceptance conditions saying "make units.list UNAVAILABLE", "jobs.create UNAVAILABLE", or "submit UNAVAILABLE" refer to this injection. Automatic UNAVAILABLE retries on reads consume remainingCalls, so use remainingCalls>=3 when checking the final error display, as in AT-C01-E③.

DemoTrigger network(connected=false) simulates network disconnection. Reject all Repository operations (except demoSession/preferences/demo.*) with DomainError{code:'UNAVAILABLE', messageKey:'errors.network_disconnected', retryAfterSeconds:null}, without running business processing. events.subscribe delivers no events while disconnected. The UI unsubscribes and shows "Updates stopped" and the last successful time (screen state offline). On reconnection with connected=true, the UI recovers through the D07 new snapshot → resubscribe sequence. Repository events committed during disconnection are reflected through replay. Separate network disconnection and device offline (code OFFLINE) in display areas and wording. Commands/deadlines that expire while disconnected still expire according to the clock.

## IR38 Deadline derivation for customer-created jobs — FRV-004

Only job.manage holders (HQ) may specify jobs.create dueAt. A client sending dueAt receives VALIDATION (fieldErrors.dueAt). If omitted, the Repository saves dueAt=requestedEnd (the same rule as D16 generated jobs). When HQ specifies it, check dueAt>=requestedEnd; a lower value returns VALIDATION. 1A has no later dueAt change operation (use a new job). JobSummary.dueAt, overdueOnly, dueAt sorting, and technician/partner overdueCount use only this saved value.

## IR39 Visibility of archived resources — FRV-005

Exclude Property/Space/ACUnit with archived=true from all lists (properties/spaces/units.list), summaries.get/admin.summary counts/denominators/KPIs, candidate selection (through units.list), notification recipient resolution, and scope checks for new business operations. 1A offers no archived filter; exclusion is unconditional. Individual reads (units.get and space/property selection resolution) return NOT_FOUND to clients/contractors/technicians. Only HQ asset.manage holders receive a read-only DTO with archived:true. Control, assignment, contract, restriction, and Device operations return CONFLICT (reason archived). Unit references from existing Job/Report/Audit/Command history permit history access under D05, and history screens show an archived label. In the same archive transition, invalidate units/properties/spaces/summaries/automatic-operation Queries. Archived IDs in a unitIds filter do not match and count as zero results.

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

For each active Assignment, the viewing window is [Assignment.createdAt, scheduledEnd), and the work window is [scheduledStart, scheduledEnd). D06's statement that "Assignment validFrom/Until equals scheduledStart/End" refers to the work window.

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

| Current Job.status | Client (own job) | HQ job.manage | Result on success |
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

contactWindow is 0–200 code points after trim. If it contains '@', or if removing whitespace, hyphens, parentheses, and '+' leaves at least seven consecutive digits, return VALIDATION (fieldErrors.contactWindow, messageKey=errors.contact_details_forbidden). Return JobDetail.contactWindow only to the client (own job), HQ job.manage, the assigned technician within the viewing window (IR49), and the accepted contractor within the access window. Other projections use null. Do not include it in JobOfferSummary, JobHistorySnapshot, or notification params. Complete detection in free text is not guaranteed (DDC-09). Input guidance and handling of time notation follow IR90 (REV19-017).

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
- REV18-043: FR-A04 requires device.manage; FR-A12 requires automation.policy.manage.
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

The A01 energy-saving card displays actual results (kWh, cost, coverage) beside the forecast. For forecast values >0, show “Expected reduction {absolute value}”; for values <0, “Expected increase {absolute value}”; for 0, “No change 0.0”. Always show “Forecast (prorated assumed baseline, demo)” and validUnitMinutes/expectedUnitMinutes. For null, show “No target equipment” if qualityWarnings contains no_units, “Baseline not set” if it contains baseline_unavailable, or “Cannot calculate” otherwise (this overrides IR68's null display). Show a link to /admin/energy only for energy.manage holders. Do not call admin.summary during SR17's first minute of the day (from=to).

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

- REV19-016: DD-A01's customer count follows IR40. DD-A04 requires device.manage; DD-A12 requires automation.policy.manage (REV18-043 in IR74). D09 `<room>` matches Space.name under IR65. In verification.md S03, payment confirmation triggers a release request (IR35); explicit release checks the idempotent response. /forbidden and undefined routes in common.md §2 display screens linking to role home; they do not redirect automatically (IR57).
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
| admin:job.manage:internal-job / internal-or-escalation | Jobs with contractorOrgId=null / for outsourced jobs, jobs.review reviewMode=hq_escalation (D06) |
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
| job.requested (jobs.create, plans.generateNext) | job_update | job | Customer client Memberships that can read the job Unit; HQ job.manage holders |
| job.offered | job_update | job | partner.accept holders at the offered contractor; customer clients (display "Being arranged") |
| job.accepted | job_update | job | HQ job.manage holders; customer clients |
| job.declined | job_update | job | HQ job.manage holders |
| job.assigned (initial assignment, reassignment, extension) | schedule_change | job | Technician of the new Assignment; customer clients; HQ job.manage holders; for outsourced work, partner.assign holders at the accepted contractor |
| report.submitted | job_update | job | For outsourced work, partner.review holders at the accepted contractor; for internal work, HQ job.manage holders; customer clients (progress only) |
| report.returned | report_return | job | Assigned technician; HQ job.manage holders |
| job.completed | completion | job | Customer clients; HQ job.manage holders; assigned technician; for outsourced work, partner.review holders at the accepted contractor |
| job.cancelled / on_hold / resumed | job_update | job | Customer clients; assigned technician; for outsourced work, partner.assign holders at the accepted contractor; HQ job.manage holders |
| restriction.requested / applied / release_requested / released / cancelled | restriction | restriction | Customer clients who can read all target Units under IR19; HQ restriction.manage holders |
| payment.confirmed (payment confirmation) | payment | invoice | Customer clients who can read the invoice; HQ billing.manage holders |
| inquiry.received / answered | inquiry | inquiry | For received, HQ billing.manage holders; for answered, clients of the customer who made the inquiry |
| Opening an Alert without a policy (device events, IR98 load_alert, records generated outside the seed) | alert | unit | Customer clients who can read the Unit; HQ alert.resolve holders; assigned technicians within their viewing window |
| device_operation.failed | device_operation | device | Membership that created the operation; HQ device.manage holders |

Add job_update and device_operation to canonical Notification.templateKey and NotificationType. For the seed's four actors, for example, if contractor-a performs job.assigned, tech-external-a, customer-a, hq-operator, and hq-restriction-manager receive one each (four total); actor contractor-a receives none.

## IR96 Restriction cancellation — G1-003 and G1-013

Determine the result of restrictions.cancel (restriction.manage, reason required) only from this table (DEC-56).

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
- Create AT-X04-E⑤ self-approval through `acceptancePatches["AT-X04-E.5"]` (hq-self-approver, a second Membership for user-tech-internal-a, with role=admin and job.manage).

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

After this duration condition is met, evaluate air_quality notifications and notify_and_ventilate candidates independently under SR25. Before it is met, notifications are suppressed/not_due and this Policy creates no control candidate (Units with no other candidates have results=suppressed/no_match). After it is met, ventilation capability, busy state, or restrictions do not suppress notifications. This is a reversible demo detail under DEC-60.

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

Policy-based alert/air_quality notifications inherit SR28 input severity from the source Alert; do not overwrite it with this table's business-notification defaults. The quality template is the D08/SR21 quality notification and uses sourcePolicy.severity. restrictions.schedule notices and notifications.preview use the same table with the target's current state. device_operation.failed is an asynchronous system event (IR59), so actor=system-demo. The creating Membership is not excluded as the actor and is included as a recipient if it currently has read access. Deduplicate recipients to one notification per Membership under IR95.

## IR105 Allergen observation change notifications — G120-004

Adding an IR98 allergen observation emits one ChangeEvent.entityType='allergen_observation' in the same save transition. entityId is the new allergenObservations row id, version=1, occurredAt is the demo clock now at save, and changedFields=['allergenObservation']. Assign the row id through normal Repository ID generation and set createdAt=now. A retry with the same DemoTrigger.eventId adds neither a row nor a change event. Distinguish observedAt from creation time.

Allow allergen_observation in events.subscribe resources and invalidate only telemetry.series under the IR71 table. Determine visibility from current read scope for the Unit and subscription unitIds. Do not expose observation IDs outside scope (D15). Follow existing rules for data-free cursor_only events. C07/A12 subscribes to this resource and fetches the latest observation again. Adding an observation with an old observedAt also triggers refetch, but does not change the displayed value unless it is the latest under IR98 order. Keep IR83 same-tick batching and SR14 page-snapshot rules.

## IR106 Notification fixture scope version at creation — G120-005

When IR91 normalizes notification rows in demoSeed and acceptancePatches, if scopeVersionAtCreation is omitted, set it from Membership.scopeVersion found through recipientMembershipId after all patches are applied. This is the fixture's creation-time snapshot. Later Membership.scopeVersion changes do not alter existing notification values. Validate an explicit scopeVersionAtCreation as a nonnegative integer and keep it; do not rewrite it to match current scopeVersion.

If the recipient Membership is absent, the source scopeVersion is not a nonnegative integer, or an explicit value is not a nonnegative integer, fail fixture generation. AT-C08-SRC notif-alert-insulation-a / notif-alert-unknown-a fills in customer-a scopeVersion=1. Runtime notification generation saves the current Membership scopeVersion at notification creation under IR95/D08; later authorization uses current scope. validate_documents.py checks explicit values or their source for the seed and every notification patch.
