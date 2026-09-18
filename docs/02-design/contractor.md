---
document_id: DD-P
version: 0.21.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# Contractor Detailed Design

This document defines contractor screen features, fields, states, and errors (exceptions). It follows the original company requirements and their linked requirements. It defines the processing and acceptance criteria (what tests check) needed for each FR (functional requirement). Reference mock screens are used only to guide the shared UI appearance.

**Implementation baseline for 0.21.0**: Read all chapters of the [Deterministic Contracts](deterministic-contracts.md) and strict-review-contracts.md, the authorization columns of the operation catalog, and the screen catalog together. Do not guess values, permissions, asynchronous behavior, or recovery during implementation. These are demo design proposals, not approval for production business use.

## Inputs and Responsibilities

The primary source is the [Original Company Requirements (SRC-06)](../00-prepare/sources/company-requirements-original.txt). Screens, inputs, states, and acceptance criteria follow the requirements reorganized from this source. Inputs are the [Role Requirements](../01-requirements/contractor.md) and [Common Requirements](../01-requirements/common.md). Read the [Common Detailed Design](common.md) and [UIUX Specification](../03-uiux/UIUXSpecification.md) before designing.

This document designs frontend fields, displays, and mock behavior. Registration, assignment, payment receipt, restrictions, and audit on the screen only change state in shared mock memory (temporary example data). This does not request server implementation or database design.

Always validate route parameters (values in URLs) as untrusted input. “Service name” in the table means an operation in the shared Repository (data service). Rows with the same route describe different functions on one screen. Every row supports loading, empty, error, forbidden, and not-found states. Show retry only for recoverable errors. For forbidden and not-found, follow IR57 and do not show retry.

## Screen and Process Design

| Design ID / requirement | Route / main component | Read and action contracts | Input, processing, validation | Errors and prohibited actions |
|---|---|---|---|---|
| DD-P01 / FR-P01 | `/partner` / `PartnerOverview` | `jobs.list`, `jobs.get`, `summaries.get` | Get contractorOrgId from the session. Show overdue jobs separately from unit urgency | Show an empty state for zero jobs. Clear previous summaries when access expires |
| DD-P02 / FR-P02 | `/partner/jobs/:id` / `PartnerJob` | `jobs.get`, `jobs.accept`, `jobs.decline` | Accept only an offer addressed to the user's company within its valid period. Declining requires a reason (1–1000 characters, provisional) | Treat expiry, HQ cancellation, or another person's update as CONFLICT and refetch. Declining does not delete the job |
| DD-P03 / FR-P03 | `/partner/schedule` / `AssignmentEditor` | `jobs.list`, `members.eligible`, `jobs.assign` | Enter technician ID, work start/end times, and required qualifications. Validate that the work period fits within the delegation period | Reject saving if a confirmed schedule overlaps, and reschedule. Reassignment after work starts requires a reason and revokes previous access |
| DD-P04 / FR-P04 | `/partner/units/:id` / `PartnerUnit` | `units.get`, `alerts.list`, `telemetry.summary` | Match the unit ID to a valid accepted job. Show only the necessary site address and entry instructions | Reject direct URLs after the delegation period ends. Provide no remote-control buttons |
| DD-P05 / FR-P05 | `/partner/jobs/:id/review` / `QualityReview` | `jobs.get`, `jobs.review`, `reports.get`, `attachments.getContent` | Review only submitted reports. Select “Accept” or “Return”; a return requires a reason. Keep earlier report revisions | Do not overwrite the technician's original report or allow people to approve their own work |
| DD-P06 / FR-P06 | `/partner/team` / `TeamCapacity` | `members.list`, `jobs.list`, `members.capacity` | Filter by date or qualification within the user's company. Qualifications are fictional demo attributes | Do not assign candidates with expired memberships. Refer new user registration to HQ |
| DD-P07 / FR-P07 | `/partner/history` / `PartnerHistory` | `jobs.events`, `jobs.addNote`, `notifications.preview`, `notifications.recipients` | Select jobId and a template. Notes are 1–2000 characters (provisional); select only authorized recipients | Do not allow free-entry external recipients or include confidential customer billing data in templates |
| DD-P08 / FR-P08 | `/partner/*` / `PartnerAccessGuard` | `jobs.get`, `session.get` | Recheck accepted delegation and its period immediately before every action. Use the demo clock for expiry | Once expired, reject the next action and discard cached data even if the screen remains open |

## Shared Implementation Steps

1. Check the session and permitted scope. Validate IDs and URL filters against schemas (data format rules).
2. Call mock services through Queries and receive display data.
3. Use React Hook Form and shared schemas for forms. The schemas also validate unit capabilities and periods.
4. Immediately before a mutation, check the target version, permissions, and current state. For major actions, show the target and reason in a confirmation view.
5. Change shared demo data through the Repository and emit an event with a correlation ID (tracking ID). Invalidate related Queries and fetch fresh data.
6. Keep a pending indicator visible while awaiting a response. Show success, denial, and failure separately. Keep form inputs when submission fails.

## Handoff to Testing

For each design ID (DD-P number), test the matching AT-P acceptance criteria, error cases in the table, and unauthorized direct calls. The [Verification Plan](../04-agentic-sdlc/verification.md) is the source of truth for test data and cross-role scenarios. If example limits such as character counts change, update the schema, document, and boundary tests together.

## Detailed Feature Specifications (0.6.0)

Keep form values in RHF (React Hook Form) and validate them with schemas. Read-only screens do not need form validation. Follow input/output contract DDC-03/09 for audit records, notifications, and shared error displays. Show read-only values from a single Query source. The [Implementation Contracts](implementation-contracts.md) define shared types, paging, time, and error handling; the following adds screen-specific conditions. Use the tokens (base colors, sizes, etc.) and patterns in [UIUX](../03-uiux/UIUXSpecification.md) UX-04/08 for appearance.

### DD-P01 Details

**Source mapping**: SRC-06 BIZ-04, BIZ-12 → FR-P01 → DD-P01. Source category: development policy SRC-02 + design additions. Design additions defined here: dashboard for delegated jobs. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-P01 / Main display pattern: **UI-OVERVIEW**. Service boundary: `jobs.list, jobs.get, summaries.get`.

**Initial view and prerequisites**: An active contractor Membership is required. HQ offer summaries can be viewed before acceptance. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| status | enum/optional | offered/accepted/assigned/in_progress/on_hold/submitted/rework_requested/completed/all (all means omit status, IR90) | Filter |
| from / to | date/optional | Maximum 366 days. Convert YYYY-MM-DD calendar dates in the display timezone to UTC Instants (IR74) | Schedule range |
| contractorOrgId | From session | Cannot be entered | Company boundary |
| summary | Read-only | offerCount/activeCount/reviewCount/overdueCount/asOf | Overview |

**Steps**

1. Count the company's jobs awaiting acceptance, scheduled, in progress, and awaiting quality review. Select a state to open its job list, then open the target job's management screen.
2. Apply the following business rules to both reads and actions.
   - Before acceptance, show only the job type, registered address of the property where the AC unit is installed, required qualifications, and possible dates.
   - Detailed unit values and entry instructions are visible only after acceptance and within the delegation period.
3. Viewing a screen does not accept an offer. Use the same search conditions for KPIs (counts and other indicators) and list entries.
4. Queries to update: `jobs / partner summary (on events)`.

**Boundary cases and failures**: Exclude offers addressed to other companies from counts. After delegation ends and unit access closes, keep minimum history of the company's acceptance and decline records visible.

**Verification**: Check the traceability entries under AT-P01 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-P02 Details

**Source mapping**: SRC-06 BIZ-12 → FR-P02 → DD-P02. Source category: development policy SRC-02 + design additions. Design additions defined here: acceptance and decline steps. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-P02 / Main display pattern: **UI-DETAIL**. Service boundary: `jobs.get, jobs.accept, jobs.decline`.

**Initial view and prerequisites**: The offer is addressed to the user's company, is before offerExpiresAt, and has not been cancelled by HQ. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| jobId / offerId | ID/required | Latest offer addressed to the user's company | Decision target |
| decision | enum/required | accept/decline | Decision |
| reason | string/required for decline | 1–1000 characters | Decline reason |
| expectedVersion | integer/required | Displayed job version | Conflict detection |
| termsVersion | Read-only/required on submission | Delegation terms version | Basis for confirmation |

**Steps**

1. Check minimum job information and delegation terms. Choose acceptance or decline with a reason. Update state displays for both HQ and the contractor.
2. Apply the following business rules to both reads and actions.
   - Acceptance is separate from assigning a technician or creating a confirmed booking.
   - On decline, return to requested and record who declined, the reason, and offerId.
   - Issue a new offerId when offering the job again.
3. Acceptance sets accepted and opens necessary unit read access only within the delegation period. Declining grants no detailed read access.
4. Queries to update: `jobs / offers / partner summary / admin summary / notifications / audit`.

**Boundary cases and failures**: At the exact expiry time, neither acceptance nor decline is allowed; refetch data. If HQ cancels immediately before acceptance, return CONFLICT and do not overwrite automatically. An unanswered expired offer returns the job to requested (IR48). Acceptance or decline after expiry returns CONFLICT(errors.offer_expired); fetching the individual job returns NOT_FOUND (IR86).

**Verification**: Check the traceability entries under AT-P02 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-P03 Details

**Source mapping**: SRC-06 BIZ-12 → FR-P03 → DD-P03. Source category: development policy SRC-02 + design additions. Design additions defined here: assigning company staff and checking qualifications. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-P03 / Main display pattern: **UI-LIST / UI-FORM**. Service boundary: `jobs.list, members.eligible, jobs.assign`.

**Initial view and prerequisites**: The job is accepted, and the user has permission to manage assignments for their company. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| jobId | ID/required | accepted or a state that allows reassignment | Target |
| technicianMembershipId | ID/required | Same company, active, matching qualifications | Assignee |
| startAt / endAt | ISO datetime/required | start<end, within delegation period, no confirmed overlap | Work slot |
| reason | string/required on reassignment | 1–1000 characters | Change reason |
| expectedVersion | integer/required | Latest fetched version | Conflict |

**Steps**

1. Check the requested dates and delegation period. Search active company technicians for matching qualifications and availability. Set work start/end times, confirm the assignment, and share the schedule and assignee.
2. Apply the following business rules to both reads and actions.
   - Candidate search does not finalize authorization. Recheck membership, qualifications, and period immediately before saving.
   - Warn about overlapping schedules. In phase 1A, reject saving if a confirmed schedule overlaps the same time slot.
3. For the first assignment, create an Assignment, confirm scheduledSlot, and set the Job to assigned. On reassignment, keep the original assigned/in_progress state, invalidate the old assignment, and update Job.scheduledSlot and assignmentId to the new assignment (IR89). Keep the original report author and save the change reason.
4. Queries to update: `jobs / assignments / eligible members / schedule / notifications / audit`.

**Boundary cases and failures**: Reject technicians from other companies, unqualified people, and assignments outside the delegation period. Reassignment during work requires a reason. The previous technician immediately loses write access. For jobs still assigned/in_progress after the scheduled slot ends, show “Work window ended; reassignment required” and extend through jobs.assign (IR89).

**Verification**: Check the traceability entries under AT-P03 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-P04 Details

**Source mapping**: SRC-06 BIZ-12 → FR-P04 → DD-P04. Source category: development policy SRC-02 + design additions. Design additions defined here: limited access to target units and alert evidence. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-P04 / Main display pattern: **UI-DETAIL**. Service boundary: `units.get, alerts.list, telemetry.summary`.

**Initial view and prerequisites**: The unit belongs to an accepted, active delegation. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| unitId | route ID/required | Resolve from an active accepted job | Unit |
| jobId | query ID/required | Identify the relevant delegation | Access basis |
| unit / capability | Read-only | Model and maintenance scope | Register |
| telemetry / evidence | Read-only | With timestamps and quality | Alert evidence |

**Steps**

1. Open the unit screen from the job. Check location, model, maintenance scope, connection state, and alert evidence. Return to the job screen afterward.
2. Apply the following business rules to both reads and actions.
   - Show only values needed for diagnosis, as read-only.
   - Do not fetch invoices, payments, other contracts, or the customer's full history.
   - Limit serial numbers and location information to what is needed.
3. This screen does not update business data. After delegation ends, show only minimum company job history, without the customer's live unit values.
4. Queries to update: `none (read-only)`.

**Boundary cases and failures**: If the period expires while the screen remains open, reject the next request and discard the cache. Permission to view monitoring data does not grant control permission.

**Verification**: Check the traceability entries under AT-P04 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-P05 Details

**Source mapping**: SRC-06 BIZ-12 → FR-P05 → DD-P05. Source category: development policy SRC-02 + design additions. Design additions defined here: report quality review and return for rework. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-P05 / Main display pattern: **UI-DETAIL / UI-FORM**. Service boundary: `jobs.get, jobs.review, reports.get, attachments.getContent`.

**Initial view and prerequisites**: The company's delegated job is submitted. The reviewer must differ from the report author. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| jobId / reportVersion | ID and integer/required | Current submitted version | Target |
| decision | enum/required | accept/return | Quality decision |
| reason | string/required on return | 1–1000 characters (IR87) | Findings |
| reviewerId | From session | Display reviewAvailability under IR31; Repository checks userId against contributors to the target version | Responsible reviewer |

**Steps**

1. Review the submitted inspections, photos, measurements, work details, and next actions. Enter acceptance or return with a reason. Share the result with the technician, customer, and HQ.
2. Apply the following business rules to both reads and actions.
   - Accept only when required inspection records and evidence are complete.
   - Check whether reasons for “Not inspected” or “Not applicable” items are valid.
   - Reviewers do not edit the technician's original records.
3. Set completed on acceptance and rework_requested on return. Link review history to the target report version. Completion does not automatically resolve Alerts.
4. Queries to update: `jobs / reports / job events / notifications / audit`.

**Boundary cases and failures**: Reject self-approval, actions on old reportVersion values, and empty return reasons. A late old acceptance action after a return must not set completed.

**Verification**: Check the traceability entries under AT-P05 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-P06 Details

**Source mapping**: SRC-06 BIZ-12 → FR-P06 → DD-P06. Source category: development policy SRC-02 + design additions. Design additions defined here: viewing company workers, qualifications, and capacity. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-P06 / Main display pattern: **UI-LIST**. Service boundary: `members.list, jobs.list, members.capacity`.

**Initial view and prerequisites**: The user has permission to read the company worker list. This does not include creating users. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| date | date/required | Default: current demo date | Schedule date |
| qualification | string/optional | Qualification register code. Lists, candidates, and capacity include only role=technician in the user's company (IR94) | Qualification filter |
| activeOnly | boolean/required | Default: true | Active membership |
| members / slots | Read-only | Necessary company names, qualifications, periods, and assignments | Schedule |

**Steps**

1. Select date, qualification, and membership validity. Check company technicians' assignments and free slots. Open the target job's assignment screen.
2. Apply the following business rules to both reads and actions.
   - Utilization is assigned time divided by configured available work time for the period. For example, 4 assigned hours out of 8 available hours is 50%. If available time (the denominator) is not configured, do not show a percentage.
   - Do not show personal location tracking or other companies' schedules.
3. This screen is read-only and does not update memberships or qualifications. Ask HQ to arrange membership changes.
4. Queries to update: `members / assignments (read-only)`.

**Boundary cases and failures**: Editing the company ID in the URL must not reveal another company's roster. Technicians with expired memberships cannot be selected for assignment.

**Verification**: Check the traceability entries under AT-P06 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-P07 Details

**Source mapping**: SRC-06 BIZ-12, BIZ-20 → FR-P07 → DD-P07. Source category: development policy SRC-02 + design additions. Design additions defined here: job communication and sharing alert information. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-P07 / Main display pattern: **UI-TIMELINE / UI-FORM**. Service boundary: `jobs.events, jobs.addNote, notifications.preview, notifications.recipients`.

**Initial view and prerequisites**: The user may view communication and history for company jobs. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| jobId | ID/required | Company job | Target |
| templateKey | enum/required | schedule_change/report_return/completion | Message template |
| message | string/required | 1–2000 characters | Message content |
| visibility | enum/required | internal/customer; default: internal | Visibility |
| recipientRole | enum/required | hq/assigned_technician/customer_contact. Map to notifications.recipients role as hq→admin, assigned_technician→technician, customer_contact→client (IR90) | Recipient |
| channel | enum/required | inApp/email/whatsapp(deliveryState=preview) | Contact channel |

**Steps**

1. Open job history. Select a schedule or quality communication template. Check the note and recipient role, then create an in-app record and an external-send preview.
2. Apply the following business rules to both reads and actions.
   - Do not send actual messages.
   - Recipients are limited to the job's HQ, assigned technician, or customer contact.
   - Internal quality notes are hidden from customers by default.
3. Save the Note with its author and visibility. Preview actions alone must not change deliveryState to sent.
4. Queries to update: `job notes / job events / notification previews / audit`.

**Boundary cases and failures**: Reject free-entry email addresses, recipients from other jobs, and copied customer billing data. Keep written notes if saving fails.

**Verification**: Check the traceability entries under AT-P07 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-P08 Details

**Source mapping**: SRC-06 BIZ-12 → FR-P08 → DD-P08. Source category: development policy SRC-02 + design additions. Design additions defined here: access limits based on contractor and assignment period. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-P08 / Main display pattern: **Guard shared by all patterns**. Service boundary: `jobs.get, session.get`.

**Initial view and prerequisites**: Applies to every route and Repository operation accessed as a contractor. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| membershipId / scopeVersion | From session/required | Active role=contractor | Actor |
| jobId / tenantId | Input and mock lookup | Organization/delegation match | Target |
| validFrom / validUntil | Read-only | from<=now<until | Period |
| action | operation enum/required | Role allowlist | Action |

**Steps**

1. Check Membership. Check the company's offer, delegation, and period, and return only the minimum needed data. Recheck immediately before an action.
2. Apply the following business rules to both reads and actions.
   - Live unit values are visible only after acceptance and within the delegation period.
   - After delegation expires, history contains only minimum records of the company's acceptance, decline, and work. Past access does not justify returning all customer data.
3. Access denial changes no business data. Audit records contain only a denial reason code, with no confidential information. Discard previous Query data when switching sessions.
4. Queries to update: `all previous caches on scope change`.

**Boundary cases and failures**: Reject other companies' jobIds, access at the exact expiry time, units not yet accepted, billing changes, and restriction actions. Never include forbidden data in responses.

**Verification**: Check the traceability entries under AT-P08 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

0.9.0 correction contracts: Read the [Strict Review Correction Contracts](strict-review-contracts.md) and [Per-Operation Version Contract](write-version-catalog.csv) together.

Additional contracts for current version 0.21.0: Read IR01–106 in the [Re-review Correction Contracts](review-resolution-contracts.md). They take priority over older text on the same topic; follow IR72 for conflict precedence.

0.14.0: Under IR25, get the pre-acceptance address from the unit's installation property. After expiry, freeze only report presence and acceptance state for report display.

0.15.0: Apply IR30 to DD-P01 unit severity, IR29 to DD-P05 completion time, and IR31 to the ban on self-approval of jointly edited versions.

Apply IR34 to job-list and jobs.list sorting. When URL sort is absent, use status:asc. Changing the selection discards cursor, keeps filters, and fetches page one of a new snapshot. Allow ascending/descending sorting by state, severity, or deadline.
