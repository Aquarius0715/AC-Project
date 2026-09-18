---
document_id: REQ-P
version: 0.21.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# Contractor requirements

**0.21.0 implementation baseline**: Read all chapters of [deterministic contracts](../02-design/deterministic-contracts.md) and strict-review-contracts.md, the authorization column in the operation catalog, and the screen catalog. Do not guess numbers, permissions, asynchronous behavior, or recovery during implementation. These are demo design proposals, not production business approval.

## Purpose and assumptions

This document is based on the [original company requests in English (SRC-06)](../00-prepare/sources/company-requirements-original.txt). Company requests are first grouped into BIZ items, then into this document's FR (functional requirements), and finally into detailed designs and acceptance criteria. This provides end-to-end traceability.

Each feature separates company requests from added design details. Screen fields, allowed inputs, state transitions, and priorities are frontend implementation proposals, including details the company has not yet reviewed. Reference mock screens guide appearance only. Requirements and acceptance criteria make the original goals more concrete through production policies and design additions.

This document covers only what users can view, enter, and do in the frontend. Registration, billing, receipts, device actions, and notifications are simulated by mocks. Real server processing, storage, and authentication are out of scope.

The goal is to let contractors accept jobs within their delegated scope and manage their own technicians, schedules, and work quality. These detailed workflows are proposals under DEC-01. A separate contractor role is a production policy absent from the company original. The original instruction behind it is not archived ([verification status](../00-prepare/sources/production-instructions.md)). Company confirmation is tracked as OPEN-10.

Required reading: [PrepareDocument](../00-prepare/PrepareDocument.md) and [common requirements](common.md). All common authentication, language, voice, permission, notification, and non-functional requirements apply.

P0 means the core foundational flow. P1 is also required for completion in phase 1A. Each row is verified under its `AT-P` number. This document separates coverage of company requests from proposed screen design and provisional values.

## Functional requirements and acceptance criteria

| Requirement ID | Priority | Status/basis | Requirement | Acceptance criteria |
|---|---|---|---|---|
| FR-P01 | P0 | Production instructions SRC-02 + added design details / BIZ-04, BIZ-12 | Accepted-work dashboard | Show own company's offers, pending acceptance count, deadlines, and progress. Exclude other companies' jobs even from totals. |
| FR-P02 | P0 | Production instructions SRC-02 + added design details / BIZ-12 | Accept/decline jobs | Accept or decline HQ offers; reflect results in HQ screens and record decline reasons. |
| FR-P03 | P0 | Production instructions SRC-02 + added design details / BIZ-12 | Schedules and own technician assignments | Assign active own-company technicians to accepted jobs with limited work periods. Reject out-of-period or other-company assignments. |
| FR-P04 | P0 | Production instructions SRC-02 + added design details / BIZ-12 | View target units and alert evidence | View only locations, models, connectivity, alerts, and history needed for accepted jobs. Hide customer billing. |
| FR-P05 | P0 | Production instructions SRC-02 + added design details / BIZ-12 | Report quality review/rework | Review submitted technician reports. Return missing required records with a reason, or accept and add to completion history. |
| FR-P06 | P1 | Production instructions SRC-02 + added design details / BIZ-12 | View own workers and availability | View own technicians' qualifications, assignments, and schedules. Cannot grant permissions or view other companies' data. |
| FR-P07 | P1 | Production instructions SRC-02 + added design details / BIZ-12, BIZ-20 | Job communication/history | View schedule-change, rework, and completion notification previews and job history. No real sending. |
| FR-P08 | P0 | Production instructions SRC-02 + added design details / BIZ-12 | Delegation/work-period boundaries | Reject other-company jobs, out-of-period units, billing changes, and restriction changes even through direct URLs/service calls. |

## Business boundaries and dependencies

The [common permission matrix](common.md) is the sole authority for who can do what. Separate viewing from changing permissions. Recheck immediately before service calls. If capabilities, work periods, or contract conditions change, do not reuse old screen permissions.

Real device control, external notifications, payments, and API authentication belong to phase 1B. Phase 1A simulates actions, including rejection, failure, and missing data as well as success.

## Completion criteria

- Meet all FR-P and applicable FR-X/NFR requirements. Do not relabel unfinished work as out of scope to claim completion.
- Map acceptance criteria to screens, services, and error handling in the [detailed design](../02-design/contractor.md).
- Verify every AT-P under the [verification plan](../04-agentic-sdlc/verification.md) and retain applicable scenario evidence.
- Return undecided business questions to PrepareDocument's OPEN list and report provisional mock decisions.

## Feature use cases and business rules (0.6.0)

The table above is an index. The following sections explain entry conditions, steps, results, and acceptance criteria for each requirement.

Numbers ①②… in acceptance cells identify observations within that cell. Match independent Given conditions to Then results by meaning; do not confuse multiple assertions with case IDs. D01's cause-based priority table determines one failure code.

Use the fixed fixtures named in the [verification plan](../04-agentic-sdlc/verification.md).

Detailed thresholds and operating rules absent from the company original are phase 1A proposals under DEC-09, not confirmed production rules.

### FR-P01 Accepted-work dashboard

- **Company request basis**: SRC-06 BIZ-04, BIZ-12 — Clear dashboards for customers, internal/external technicians, and administrators/HQ; scheduled, reactive, and preventive maintenance, including general non-RTO maintenance.
- **Added design details**: Dashboard for accepted jobs. The separate contractor role comes from production instruction SRC-02, not an independent role in the company original.

- **Entry conditions**: Active contractor Membership. Before accepting an HQ offer, the job summary is visible.
- **Main flow**: Count own pending acceptance, scheduled, active, and awaiting-quality-review jobs → select a status to view the list → open the job action screen.
- **Business rule BR-P01**: Before acceptance, show only job type, the registered address of the AC's installation property, required qualifications, and candidate schedules. Detailed unit readings and entry instructions require acceptance and a valid work period.
- **Resulting business state**: Viewing does not accept a job. KPI counts and listed targets use the same filters.
- **Boundaries/prohibitions**: Exclude other companies' offers from counts. After delegation expires and unit access ends, retain minimal own acceptance/decline history.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-P01-N | `acceptancePatches["AT-P01-N"]`: contractor-a has one unanswered unexpired offered job, one accepted job within its access period, and one submitted job (tech-external-a starts job-contractor-a → saveDraft with `acceptancePatches["shared:report-draft-all-normal"]` → submit, IR97 item 3); contractor-b has one offered job. No period filter. When: ① Get `/partner` without status filter ② Change to status=offered | ① Three rows, offerCount=1/activeCount=1/reviewCount=1 ② One row, offerCount=1/activeCount=0/reviewCount=0; no b offer ③ Unanswered Offer decision remains null after viewing |
| AT-P01-E | ① Directly enter jobId of an offer to contractor-b ② Open own history after delegation expiry | ① NOT_FOUND ② Only JobHistorySnapshot (own decision/completion date), no live values |
| AT-P01-B | Open the same offer ① Before acceptance ② After acceptance within the period ③ After delegation expiry | ① JobOfferSummary (registered installation-property address only; no entry instructions/telemetry/billing) ② JobDetail ③ JobHistorySnapshot |

Design: [DD-P01](../02-design/contractor.md#dd-p01-details). Assess parent AT-P01 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-P02 Accept/decline jobs

- **Company request basis**: SRC-06 BIZ-12 — Scheduled, reactive, and preventive maintenance, including general non-RTO maintenance.
- **Added design details**: Acceptance/decline steps. The separate contractor role comes from production instruction SRC-02, not the company original.

- **Entry conditions**: An offered job addressed to own company, before offerExpiresAt, not cancelled by HQ. An unanswered expired offer returns the job to requested and removes it from the contractor list (IR48). Acceptance/decline after expiry returns CONFLICT(errors.offer_expired); individual job reads return NOT_FOUND (IR86).
- **Main flow**: Check minimal job details and delegation terms → accept or decline with a reason → update HQ and contractor displays.
- **Business rule BR-P02**: Acceptance does not assign a technician or confirm a booking. Decline returns the job to requested and records actor, reason, and offerId. A new offer receives a new offerId.
- **Resulting business state**: Acceptance sets `accepted` and grants needed unit access only during the delegated period. Decline grants no detailed access.
- **Boundaries/prohibitions**: Reject acceptance/decline exactly at expiry and require refetch. If HQ cancelled just before acceptance (CONFLICT), do not overwrite automatically.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-P02-N | hq-operator offers job-internal-a to contractor-a (offerExpiresAt=2026-09-15T01:00Z, accessValidFrom=2026-09-14T01:00Z, accessValidUntil=2026-09-22T00:00Z). When: contractor-a accepts | ① Job accepted, Offer decision=accept, decidedBy saved ② HQ list shows accepted ③ Target unit viewable within period |
| AT-P02-E | ① Accept/decline at now=offerExpiresAt ② HQ cancels after display → accept using old version | ① CONFLICT, refetch guidance, unchanged state ② CONFLICT, remains cancelled |
| AT-P02-B | ① accept ② decline with reason ③ Re-offer after decline | ① accepted ② requested, declineReason/offerId retained, no unit viewing permission ③ New offerId |

Design: [DD-P02](../02-design/contractor.md#dd-p02-details). Assess parent AT-P02 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-P03 Schedules and own technician assignments

- **Company request basis**: SRC-06 BIZ-12 — Scheduled, reactive, and preventive maintenance, including general non-RTO maintenance.
- **Added design details**: Own-staff assignment and qualification checks. The separate contractor role comes from production instruction SRC-02, not the company original.

- **Entry conditions**: Accepted job and permission to manage own assignments.
- **Main flow**: Check requested slot/delegation period → find active, qualified, available own-company technicians → set work start/end → confirm assignment → share schedule/assignee.
- **Business rule BR-P03**: Recheck membership, qualifications, and period just before saving, not only during candidate search. Warn on overlapping schedules. Phase 1A rejects saving when a confirmed schedule already occupies the same time.
- **Resulting business state**: First assignment creates Assignment, confirms scheduledSlot, and sets Job to `assigned`. Reassignment preserves `assigned` or `in_progress`, disables the old assignment, and updates the job's scheduledSlot/assignment ID to the new assignment (IR89). Preserve original report authorship and record the change reason.
- **Boundaries/prohibitions**: Reject other-company, unqualified, and out-of-delegation-period assignments. Reassignment during work requires a reason. The previous technician loses action access immediately.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-P03-N | job-internal-a accepted in AT-P02-N; tech-external-a (qualified, contractor-a; seed assignment-contractor-a lasts until 2026-09-20T00:00Z). When: Assign 2026-09-21 10:00–12:00 (Asia/Kuala_Lumpur) | ① Assignment created ② scheduledSlot confirmed ③ Job assigned ④ One templateKey=schedule_change notification to tech-external-a (inApp, simulated); one each to customer-a, hq-operator, hq-restriction-manager; zero to actor contractor-a (IR95) |
| AT-P03-E | ① contractor-b technician ② Unqualified ③ Outside delegation period ④ Reassign in_progress without reason ⑤ Reassign with reason | ① NOT_FOUND ② FORBIDDEN ③ VALIDATION (slot outside delegation period), zero assignments ④ VALIDATION ⑤ Success, Job remains in_progress |
| AT-P03-B | ① Non-overlapping schedule ② Overlap with confirmed schedule ③ Candidate qualification revoked just before save | ① Success ② CONFLICT ③ FORBIDDEN |

**Additional acceptance AT-P03-R01 (revisit, conflict, cross-role)**

| Acceptance ID | Given / When | Then |
|---|---|---|
| AT-P03-R01 | `acceptancePatches["AT-P03-R01"]` adds second contractor-a technician tech-external-a2. After tech-external-a starts job-contractor-a (in_progress), reassign from tech-external-a to tech-external-a2 with a reason | Job stays in_progress; reject changes by old assignee a. New assignee b continues in a new draft version; old versions/original author stay unchanged. |

Design: [DD-P03](../02-design/contractor.md#dd-p03-details). Assess parent AT-P03 using all N/E/B/R01 and applicable SRC cases in traceability.

### FR-P04 View target units and alert evidence

- **Company request basis**: SRC-06 BIZ-12 — Scheduled, reactive, and preventive maintenance, including general non-RTO maintenance.
- **Added design details**: Access only needed target units/alert evidence. The separate contractor role comes from production instruction SRC-02, not the company original.

- **Entry conditions**: Target unit belongs to an accepted, valid delegation.
- **Main flow**: Open unit from job → view location/model/maintenance scope/connectivity/alert evidence → return to job.
- **Business rule BR-P04**: Show only readings needed for diagnosis, read-only. Do not retrieve billing, payments, other contracts, or all customer history. Limit serial/location details to what is needed.
- **Resulting business state**: No business update. After delegation ends, remove live customer-unit values and return to minimal own-job history references.
- **Boundaries/prohibitions**: If the screen stays open past expiry, reject the next read and clear cache. Viewing monitoring values does not grant control permission.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-P04-N | unit-online-rto in an accepted job. When: Open unit from job | ① Location/model/maintenance scope/connection/alert evidence ② No billing/payment/contact fields ③ No control buttons ④ Zero writes |
| AT-P04-E | Keep screen open, set now=validUntil, read again | FORBIDDEN, clear cache, show JobHistorySnapshot |
| AT-P04-B | ① Within period ② After period | ① Live values/observation time ② Minimal references only |

Design: [DD-P04](../02-design/contractor.md#dd-p04-details). Assess parent AT-P04 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-P05 Report quality review and rework

- **Company request basis**: SRC-06 BIZ-12 — Scheduled, reactive, and preventive maintenance, including general non-RTO maintenance.
- **Added design details**: Report quality review/rework. The separate contractor role comes from production instruction SRC-02, not the company original.

- **Entry conditions**: Own delegated job in submitted state. Quality reviewer must be a different person from the report author.
- **Main flow**: Review submitted inspections/photos/readings/work/next actions → accept or return with reason → inform technician/customer/HQ.
- **Business rule BR-P05**: Acceptance requires all mandatory inspection records/evidence, including valid reasons for not inspected/not applicable. Reviewers do not rewrite original technician records.
- **Resulting business state**: Accept → `completed`; return → `rework_requested`. Link review history to report version. Completion does not automatically resolve alerts.
- **Boundaries/prohibitions**: Reject self-approval, actions on old report versions, and returns without reasons. A stale acceptance arriving after a return must not complete the job.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-P05-N | tech-external-a starts job-contractor-a → fills every inspection item and saveDraft (v1) → submits. Reviewer contractor-a has a different user. When: contractor-a accepts | ① completed ② reviewHistory linked to v1 ③ Customer can retrieve report body ④ Alert remains open |
| AT-P05-E | ① Accept through another Membership with author's userId ② Accept reportVersion=0 ③ Return without reason ④ Accept v1 after return | ① FORBIDDEN ② VALIDATION (version must be positive integer) ③ VALIDATION ④ CONFLICT, remains rework_requested |
| AT-P05-B | ① All inspection records present ② Not-inspected items with reasons ③ Technician submits not-inspected items without reasons | ①② Can accept ③ Submission returns VALIDATION, never becomes submitted, not listed for quality review (IR100) |

**Additional acceptance AT-P05-R01 (revisit, conflict, cross-role)**

| Acceptance ID | Given / When | Then |
|---|---|---|
| AT-P05-R01 | Technician submits report with photos → signs out → contractor quality reviewer signs in | reports.get and attachments.getContent redisplay the same photos/report version. Customer cannot retrieve body before acceptance but can afterward. |

Design: [DD-P05](../02-design/contractor.md#dd-p05-details). Assess parent AT-P05 using all N/E/B/R01 and applicable SRC cases in traceability.

### FR-P06 View own workers and availability

- **Company request basis**: SRC-06 BIZ-12 — Scheduled, reactive, and preventive maintenance, including general non-RTO maintenance.
- **Added design details**: Own workers, qualifications, and availability views. The separate contractor role comes from production instruction SRC-02, not the company original.

- **Entry conditions**: Permission to read own-company roster; no user creation permission included.
- **Main flow**: Select date/qualification/active or expired → view own technicians' assignments/free slots → proceed to job assignment.
- **Business rule BR-P06**: Utilization = assigned hours / configured available hours for the period. For example, 4 assigned hours out of 8 available = 50%. If available hours are undefined, show no percentage. Do not show personal location tracking or other-company schedules.
- **Resulting business state**: Viewing does not change membership/qualifications. Ask HQ to coordinate membership changes.
- **Boundaries/prohibitions**: Changing company ID in the URL cannot retrieve another company's roster. Expired technicians cannot be selected for assignment.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-P06-N | `acceptancePatches["AT-P06-N"]` gives contractor-a two technicians (tech-external-a, tech-external-a2). When: date=2026-09-15, activeOnly=true | ① Assignments/free slots for two people ② Zero other-company people ③ Zero writes |
| AT-P06-E | ① Change URL company ID to contractor-b ② Select expired technician | ① NOT_FOUND ② Not listed; direct selection FORBIDDEN |
| AT-P06-B | `acceptancePatches["AT-P06-B"]`: ① tech-external-a2, date=2026-09-15 (8h available/4h assigned) ② date=2026-09-19 (Saturday, no available intervals) ③ contractor-b's tech-external-b | ① 50% ② “—” ③ Hidden |

Design: [DD-P06](../02-design/contractor.md#dd-p06-details). Assess parent AT-P06 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-P07 Job communication and history

- **Company request basis**: SRC-06 BIZ-12, BIZ-20 — Scheduled, reactive, preventive, and general non-RTO maintenance; firmware updates and removal/theft protection/notifications for small low-cost in-unit devices.
- **Added design details**: Job communication and alert sharing. The separate contractor role comes from production instruction SRC-02, not the company original.

- **Entry conditions**: Permission to view communication/history for own jobs.
- **Main flow**: Open job history → choose schedule/quality template → confirm note and recipient role → create in-app record and external-send preview.
- **Business rule BR-P07**: No real sending. Exclude entire events containing visibility=internal notes from customer jobs.events (IR42). Recipients are limited to involved HQ, assigned technician, and customer contact. Internal quality notes are hidden from customers by default.
- **Resulting business state**: Save Note with author and visibility. Creating a preview does not set deliveryState=sent.
- **Boundaries/prohibitions**: Reject free-form email recipients, recipients from other jobs, and copied customer billing details. Preserve note text on save failure.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-P07-N | job-contractor-a. When: Preview template=schedule_change, recipient=hq, visibility=internal, channel=email | ① JobNote saved with authorId ② NotificationPreview deliveryState=preview ③ Note hidden from customer |
| AT-P07-E | ① Arbitrary email recipient ② Other-job recipient ③ Billing-record reference params ④ Save failure | ①②③ One error per D01 ④ Preserve note body |
| AT-P07-B | ① internal ② customer ③ Allowed recipient ④ Free-form external recipient | ① Hidden from customer ② Visible to customer ③ Preview created ④ Rejected |

**Additional acceptance AT-P07-R01 (revisit, conflict, cross-role)**

| Acceptance ID | Given / When | Then |
|---|---|---|
| AT-P07-R01 | Jobs a/b exist; save note to jobId=a and revisit | Note with author/visibility appears only in a's history. Zero additions to b. Missing jobId returns VALIDATION. |

Design: [DD-P07](../02-design/contractor.md#dd-p07-details). Assess parent AT-P07 using all N/E/B/R01 and applicable SRC cases in traceability.

### FR-P08 Delegation and period boundaries

- **Company request basis**: SRC-06 BIZ-12 — Scheduled, reactive, and preventive maintenance, including general non-RTO maintenance.
- **Added design details**: Access limits by contractor and assignment period. The separate contractor role comes from production instruction SRC-02, not the company original.

- **Entry conditions**: Applies to all contractor routes and Repository operations.
- **Main flow**: Check Membership → own offer/delegation/period → return minimum required data → recheck just before action.
- **Business rule BR-P08**: Live unit access requires acceptance and a valid work period. After delegation ends, history contains only minimal own acceptance/decline/work records. Previous access never justifies returning all customer data.
- **Resulting business state**: Rejection makes no business changes. Audit records only a reason code, without secret data. Session switching clears old Queries.
- **Boundaries/prohibitions**: Reject other-company jobId, exact expiry time, unaccepted units, billing changes, and restriction actions. Responses must not contain forbidden data.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-P08-N | contractor-a session. When: All routes/Repository operations | ① Minimal projection ② Recheck before action ③ Zero business changes on rejection |
| AT-P08-E | ① Other-company jobId ② now=validUntil ③ Unaccepted unit ④ invoices.create ⑤ restrictions.schedule | One error each under D01, audit reason code, no forbidden response data |
| AT-P08-B | ① Unaccepted ② Accepted within period ③ Expired period | ① JobOfferSummary ② JobDetail + live unit ③ JobHistorySnapshot only |

Design: [DD-P08](../02-design/contractor.md#dd-p08-details). Assess parent AT-P08 using all N/E/B and applicable SRC/R01 cases in traceability.


0.9.0 correction contracts: Read [strict review correction contracts](../02-design/strict-review-contracts.md) and [operation version contracts](../02-design/write-version-catalog.csv) together.

Additional current 0.21.0 contracts: Read [re-review correction contracts](../02-design/review-resolution-contracts.md) IR01–106. They override older text on the same issues; use IR72 for conflict priority.

0.14.0: Under IR25, the pre-acceptance address comes from the unit's installation property. After expiry, freeze only report existence/acceptance status.

0.15.0: FR-P01 lists/counts follow IR26/IR30. FR-P05 self-approval checks all contributors to the submitted version under IR31.

Job lists support ascending/descending sorting by status (business order), severity, and deadline. Default: status in business order (IR34). Sort all results before pagination; language changes do not change order. Also use AT-REV16-005 for acceptance.
