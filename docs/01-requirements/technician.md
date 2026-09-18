---
document_id: REQ-T
version: 0.21.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# Technician requirements

**0.21.0 implementation baseline**: Read all chapters of [deterministic contracts](../02-design/deterministic-contracts.md) and strict-review-contracts.md, the authorization column in the operation catalog, and the screen catalog. Do not guess numbers, permissions, asynchronous behavior, or recovery during implementation. These are demo design proposals, not production business approval.

## Purpose and assumptions

This document is based on the [original company requests in English (SRC-06)](../00-prepare/sources/company-requirements-original.txt). Company requests are first grouped into BIZ items, then into this document's FR (functional requirements), and finally into detailed designs and acceptance criteria. This provides end-to-end traceability.

Each feature separates company requests from added design details. Screen fields, allowed inputs, state transitions, and priorities are frontend implementation proposals, including details the company has not yet reviewed. Reference mock screens guide appearance only. Requirements and acceptance criteria make the original goals more concrete through production policies and design additions.

This document covers only what users can view, enter, and do in the frontend. Registration, billing, receipts, device actions, and notifications are simulated by mocks. Real server processing, storage, and authentication are out of scope.

The goal is to let internal and external technicians monitor, diagnose, inspect, and maintain IoT devices within their assigned scope.

Required reading: [PrepareDocument](../00-prepare/PrepareDocument.md) and [common requirements](common.md). All common authentication, language, voice, permission, notification, and non-functional requirements apply.

P0 means the core foundational flow. P1 is also required for completion in phase 1A. Each row is verified under its `AT-T` number. This document separates coverage of company requests from proposed screen design and provisional values.

## Functional requirements and acceptance criteria

| Requirement ID | Priority | Status/basis | Requirement | Acceptance criteria |
|---|---|---|---|---|
| FR-T01 | P0 | Company original SRC-06 + added design details / BIZ-04, BIZ-08 | Assigned-work dashboard | Show assigned units, severity, unhandled counts, schedules, and progress. Verify different internal/external access scopes. |
| FR-T02 | P0 | Company original SRC-06 + added design details / BIZ-06, BIZ-07, BIZ-10 | Unit register | View location, brand, model, configuration, installation date, and maintenance scope. |
| FR-T03 | P0 | Company original SRC-06 + added design details / BIZ-08, BIZ-11 | Time-series monitoring | Show sensor readings, power, operation, connectivity, and data times. Demo updates change values for the same unit. |
| FR-T04 | P0 | Company original SRC-06 + added design details / BIZ-10 | Indoor inspection | Record a result or reason for not inspecting every filter, evaporator, blower motor/fan, drain pipe/pan, outlet, and louver. |
| FR-T05 | P0 | Company original SRC-06 + added design details / BIZ-10 | Outdoor inspection | Record inspections for every condenser, compressor, fan/blade, and refrigerant pipe. |
| FR-T06 | P0 | Company original SRC-06 + added design details / BIZ-10 | Electrical/control inspection | Record every thermostat, sensor, capacitor, contactor, and wiring item. |
| FR-T07 | P0 | Company original SRC-06 + added design details / BIZ-08, BIZ-11, BIZ-17 | Alert evidence | Distinguish suspected, measured, and on-site inspection findings with evidence/history. Completing work alone does not resolve alerts. |
| FR-T08 | P0 | Company original SRC-06 + added design details / BIZ-12 | Scheduled/reactive/preventive maintenance | Start assigned jobs, submit reports, and rework/resubmit after return. Match progress shown to customers/HQ. |
| FR-T09 | P0 | Added design details (supporting a company goal) / BIZ-12 | Work reports | Save/submit checklists, photos, readings, replaced parts, work details, and next actions. Clearly identify missing required fields. |
| FR-T10 | P0 | Added design details (supporting a company goal) / BIZ-13 | Remote diagnostics/test runs | Allow settings/test runs only within assignment period, capabilities, and permissions. Distinguish confirmation through response. |
| FR-T11 | P1 | Added design details (supporting a company goal) / BIZ-20 | IoT device lifecycle | Simulate registration, unit binding, connection checks, calibration, and firmware updates with visible progress, failure, and history. |
| FR-T12 | P1 | Company original SRC-06 + added design details / BIZ-20 | IoT faults | Separately reproduce/check communication loss, power loss, and removal. Keep recovery times and response history. |

## Business boundaries and dependencies

The [common permission matrix](common.md) is the sole authority for who can do what. Separate viewing from changing permissions. Recheck immediately before service calls. If capabilities, work periods, or contract conditions change, do not reuse old screen permissions.

Real device control, external notifications, payments, and API authentication belong to phase 1B. Phase 1A simulates actions, including rejection, failure, and missing data as well as success.

## Completion criteria

- Meet all FR-T and applicable FR-X/NFR requirements. Do not relabel unfinished work as out of scope to claim completion.
- Map acceptance criteria to screens, services, and error handling in the [detailed design](../02-design/technician.md).
- Verify every AT-T under the [verification plan](../04-agentic-sdlc/verification.md) and retain applicable scenario evidence.
- Return undecided business questions to PrepareDocument's OPEN list and report provisional mock decisions.

## Feature use cases and business rules (0.6.0)

The table above is an index. The following sections explain entry conditions, steps, results, and acceptance criteria for each requirement.

Numbers ①②… in acceptance cells identify observations within that cell. Match independent Given conditions to Then results by meaning; do not confuse multiple assertions with case IDs. D01's cause-based priority table determines one failure code.

Use the fixed fixtures named in the [verification plan](../04-agentic-sdlc/verification.md).

Detailed thresholds and operating rules absent from the company original are phase 1A proposals under DEC-09, not confirmed production rules.

### FR-T01 Assigned-work dashboard

- **Company request basis**: SRC-06 BIZ-04, BIZ-08 — Clear dashboards for customers, internal/external technicians, and administrators/HQ; real-time notifications of faults before or when they occur.
- **Added design details**: Visibility based on assignment period.

- **Entry conditions**: Internal technicians read their assigned scope; external technicians read only their company's individually assigned work periods. Before work starts, show assigned jobs read-only within the viewing window (work-not-started, IR76); actions require the work window (IR49).
- **Main flow**: Open assigned jobs for today/selected period → sort by severity/deadline/progress → open unit details or work screen.
- **Business rule BR-T01**: Unstarted count means own assigned jobs not yet started, not all requested jobs. Broad internal access still cannot exceed tenant and assigned scope.
- **Resulting business state**: Read-only; no changes. With zero jobs, show an empty state and link to history.
- **Boundaries/prohibitions**: External technicians lose live unit access at assignment expiry. Directly entering another technician's job URL cannot start work.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-T01-N | tech-external-a: assigned job-contractor-a (validUntil=2026-09-20); one requested job for another technician. When: `/technician` today | ① One assigned job, one unstarted ② Other person's requested job hidden ③ Zero writes |
| AT-T01-E | ① External technician screen at now=validUntil ② tech-external-a starts a jobId assigned only to another technician | ① Live unit hidden ② NOT_FOUND |
| AT-T01-B | ① Own assigned job ② Other person's requested job ③ Outside internal scope ④ Outside external assignment period | ① Visible ②③④ Hidden |

Design: [DD-T01](../02-design/technician.md#dd-t01-details). Assess parent AT-T01 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-T02 Unit register

- **Company request basis**: SRC-06 BIZ-06, BIZ-07, BIZ-10 — Split Unit AC in Phase 1, HVAC in Phase 2, and more brands/models; management by home/office/area/floor/room/space; indoor/outdoor/electrical/control component status.
- **Added design details**: Unit register fields and viewing steps.

- **Entry conditions**: An assignment relationship granting target-unit viewing permission.
- **Main flow**: Open register from job → check location/model/configuration/installation date/maintenance scope → proceed to diagnosis/work.
- **Business rule BR-T02**: Show the capability version. Label unknown/unregistered fields “Not registered”; do not fill them from similar models.
- **Resulting business state**: Read-only. Technicians cannot change manufacturer records, customer membership, or billing.
- **Boundaries/prohibitions**: Do not fill missing installation dates with today's date. Return no data when external technicians request unassigned units.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-T02-N | In-scope unit-online-rto (brand/model/installedAt registered, capabilityVersion=3). When: Open register | ① Location/model/configuration/installation date/maintenance scope ② capabilityVersion=3 ③ Zero writes |
| AT-T02-E | ① tech-internal-a opens unit-non-rto with installedAt=null ② tech-external-a opens unassigned unit-other-customer | ① “Not registered,” no current-date fallback ② NOT_FOUND |
| AT-T02-B | ① Registered capability ② Unregistered | ① Show choices ② “Not registered,” no assumed typical values |

Design: [DD-T02](../02-design/technician.md#dd-t02-details). Assess parent AT-T02 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-T03 Time-series monitoring

- **Company request basis**: SRC-06 BIZ-08, BIZ-11 — Real-time fault notifications before/at occurrence; early detection of vibration, high temperature, low refrigerant, tiny leaks, and clogged filters.
- **Added design details**: Series selection and data quality display.

- **Entry conditions**: Permission to view target telemetry. Connectivity can be shown even without sensors.
- **Main flow**: Choose metric/period → check sensor/power/operation/connectivity values and times → observe demo update events → make stopped updates clear after communication loss.
- **Business rule BR-T03**: Presets: 1h/24h/7d/custom. 1h/24h are fixed rolling windows; 7d uses calendar days (IR41). Separate observation and receipt times. Data older than staleAfterSeconds is stale. Old events cannot overwrite new values. Each series has fixed units; do not connect gaps in missing data.
- **Resulting business state**: Latest value and chart agree on event ID/version. Stop subscriptions on navigation away or scope change.
- **Boundaries/prohibitions**: Reordered/duplicate events must not move values backward. Offline last-known values may remain with timestamps, but must not be labeled real-time.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-T03-N | unit-online-rto, staleAfterSeconds=120. When: Show metric=temperature, 24h (IR41 rolling window [to−1440 minutes,to)) → trigger sequence=4 telemetry in /demo → communication loss | ① Series/latest value share eventId/version ② Latest value updates after event ③ “Updates stopped” on communication loss; unsubscribe |
| AT-T03-E | ① Send sequence=3, then sequence=2 and duplicate sequence=3 (IR74) ② Disconnect | ① No value rollback; ignore duplicate ② Retain last value/time without real-time label |
| AT-T03-B | ① 120 seconds after observation ② 120 seconds+1ms ③ null inside series | ① valid ② stale ③ No line across gap |

Design: [DD-T03](../02-design/technician.md#dd-t03-details). Assess parent AT-T03 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-T04 Indoor inspection

- **Company request basis**: SRC-06 BIZ-10 — Understand indoor, outdoor, electrical, and control component status.
- **Added design details**: Inspection forms and reasons for not inspecting.

- **Entry conditions**: Active assigned job in_progress, with maintenance scope and component targets loaded.
- **Main flow**: Select each component result → link required findings/readings/photos → give reasons for not inspected/not applicable → save draft or report.
- **Business rule BR-T04**: Group: indoor; components: filter, evaporator_coil, blower_motor, blower_fan, drain_pipe, drain_pan, outlet, louver. Initial results are null. To submit without inspection, explicitly select not_inspected and give a reason. If the unit lacks the component, select not_applicable with a reason.
- **Resulting business state**: Save results linked to report version, author, and observation time. Keep sensor estimates as separate evidence; on-site inspection does not overwrite original sensor data.
- **Boundaries/prohibitions**: Reject missing inputs, readings without units, not_inspected without reason, and photos from other jobs. Do not preselect normal to allow completion without inspection.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-T04-N | tech-external-a starts job-contractor-a (in_progress; unit-online-rto has 18 components, IR100). When: In `acceptancePatches["shared:report-draft-all-normal"]`, change filter to attention with a reason → saveDraft → add one photo with attachments.add → submit latest version | ① First save: reportVersion=1 ② Submission succeeds; results/author/observation times for 18 components including 8 indoor components are linked to the version ③ Sensor estimates remain separate evidence |
| AT-T04-E | On submission: ① One component result=null ② Reading without unit ③ not_inspected without reason ④ attachmentId visible within own scope but from another job | Each returns VALIDATION; zero submissions |
| AT-T04-B | Save/submit 8 indoor components as ① normal ② attention with reason ③ not_inspected with reason ④ not_applicable with reason ⑤ null | ①–④ Draft save/submission allowed ⑤ Draft save allowed, submission VALIDATION. Initial value is null; normal is not preselected |

Design: [DD-T04](../02-design/technician.md#dd-t04-details). Assess parent AT-T04 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-T05 Outdoor inspection

- **Company request basis**: SRC-06 BIZ-10 — Understand indoor, outdoor, electrical, and control component status.
- **Added design details**: Inspection forms and reasons for not inspecting.

- **Entry conditions**: Active assigned job in_progress, with maintenance scope and component targets loaded.
- **Main flow**: Select each component result → link required findings/readings/photos → give reasons for not inspected/not applicable → save draft or report.
- **Business rule BR-T05**: Group: outdoor; components: condenser_coil, compressor, fan, blade, refrigerant_pipe. Initial results are null. To submit without inspection, explicitly select not_inspected and give a reason. If the unit lacks the component, select not_applicable with a reason.
- **Resulting business state**: Save results linked to report version, author, and observation time. Keep sensor estimates as separate evidence; on-site inspection does not overwrite original sensor data.
- **Boundaries/prohibitions**: Reject missing inputs, readings without units, not_inspected without reason, and photos from other jobs. Do not preselect normal to allow completion without inspection.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-T05-N | tech-external-a starts job-contractor-a (in_progress). When: In `acceptancePatches["shared:report-draft-all-normal"]`, change compressor to attention with a reason → saveDraft → submit | ① First save: reportVersion=1 ② Submission succeeds; results/author/observation times for 18 components including 5 outdoor components are linked to the version ③ Sensor estimates remain separate evidence |
| AT-T05-E | On submission: ① One component result=null ② Reading without unit ③ not_inspected without reason ④ attachmentId visible within own scope but from another job | Each returns VALIDATION; zero submissions |
| AT-T05-B | Save/submit 5 outdoor components as ① normal ② attention with reason ③ not_inspected with reason ④ not_applicable with reason ⑤ null | ①–④ Draft save/submission allowed ⑤ Draft save allowed, submission VALIDATION. Initial value is null |

Design: [DD-T05](../02-design/technician.md#dd-t05-details). Assess parent AT-T05 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-T06 Electrical/control inspection

- **Company request basis**: SRC-06 BIZ-10 — Understand indoor, outdoor, electrical, and control component status.
- **Added design details**: Inspection forms and reasons for not inspecting.

- **Entry conditions**: Active assigned job in_progress, with maintenance scope and component targets loaded.
- **Main flow**: Select each component result → link required findings/readings/photos → give reasons for not inspected/not applicable → save draft or report.
- **Business rule BR-T06**: Group: electrical; components: thermostat, sensor, capacitor, contactor, wiring. Initial results are null. To submit without inspection, explicitly select not_inspected and give a reason. If the unit lacks the component, select not_applicable with a reason.
- **Resulting business state**: Save results linked to report version, author, and observation time. Keep sensor estimates as separate evidence; on-site inspection does not overwrite original sensor data.
- **Boundaries/prohibitions**: Reject missing inputs, readings without units, not_inspected without reason, and photos from other jobs. Do not preselect normal to allow completion without inspection.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-T06-N | tech-external-a starts job-contractor-a (in_progress). When: In `acceptancePatches["shared:report-draft-all-normal"]`, change capacitor to attention with a reason and add capacitor reading (metric=vibration, value=2.5, unit=mm/s) → saveDraft → submit | ① First save: reportVersion=1 ② Submission succeeds; results/author/observation times for 18 components including 5 electrical components are linked to the version ③ Reading has unit, observedAt, origin=inspection |
| AT-T06-E | On submission: ① One component result=null ② Reading without unit ③ not_inspected without reason ④ attachmentId visible within own scope but from another job | Each returns VALIDATION; zero submissions |
| AT-T06-B | Save/submit 5 electrical components as ① normal ② attention with reason ③ not_inspected with reason ④ not_applicable with reason ⑤ null | ①–④ Draft save/submission allowed ⑤ Draft save allowed, submission VALIDATION. Initial value is null |

Design: [DD-T06](../02-design/technician.md#dd-t06-details). Assess parent AT-T06 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-T07 Alert evidence

**Requirement made concrete from the original — Load notifications for open windows/poor insulation (BIZ-17)**

Notify about open windows or poor insulation as possible causes of increased load. Show target unit, time, supporting data, and links to checking/maintenance.

**Additional acceptance AT-T07-SRC**: Check three fixtures in `acceptancePatches["AT-T07-SRC"]` (IR98, with an assigned job for tech-internal-a). Each has different wording, evidence, and time. Reading the notification does not clear the alert.

- **Company request basis**: SRC-06 BIZ-08, BIZ-11, BIZ-17 — Real-time fault notifications before/at occurrence; early detection of vibration, high temperature, low refrigerant, tiny leaks, and clogged filters; adapt to routines/weather and identify load from open windows/poor insulation.
- **Added design details**: Possible causes, evidence, and resolution steps.

- **Entry conditions**: Assigned unit has an alert or suspected issue to diagnose.
- **Main flow**: Open evidence from alert list → check measured/estimated/inspection results and history → acknowledge → if needed, remeasure or resolve with a reason.
- **Business rule BR-T07**: Do not show numeric probability when estimate confidence is unknown. Confirmation is `acknowledged`. Resolution requires remeasurement meeting configured recovery conditions, or `alert.resolve` permission and a reason.
- **Resulting business state**: Record detection/acknowledgment/resolution times and actors. Recurrence gets a new alertId linked to the previous event.
- **Boundaries/prohibitions**: Completing a Job does not set resolved. Communication loss alone does not prove theft; keep it separate from removal events.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-T07-N | `acceptancePatches["AT-T07-N"]`: unit-online-rto has open alert-temp-a from policy-temp-a (temperature gte 30°C, recoveryThreshold 28°C, duration 60 seconds), with three evidence records: measured/estimated/inspection. Include `shared:tech-internal-a-job-online` to add assigned job-t07 within its work window; act as tech-internal-a (IR94). When: Open evidence → acknowledge → keep remeasurement below 28°C for 60 seconds → meet the same alert condition again for 60 seconds | ① Distinct measured/estimated/inspection display ② acknowledged with acknowledgedAt/actor ③ resolved with resolvedAt ④ Recurrence has new alertId, previousAlertId=original ID (IR66) |
| AT-T07-E | ① Complete only the Job of an unresolved Alert ② Supply only missing heartbeat | ① Alert.status unchanged ② No theft label, only connection=offline |
| AT-T07-B | ① Estimate without confidence ② alert.resolve permission and reason ③ No permission ④ Remeasurement shows recovery | ① No numeric probability ② resolved ③ FORBIDDEN ④ resolved |

Design: [DD-T07](../02-design/technician.md#dd-t07-details). Assess parent AT-T07 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-T08 Scheduled, reactive, and preventive maintenance

- **Company request basis**: SRC-06 BIZ-12 — Scheduled, reactive, and preventive maintenance, including general non-RTO maintenance.
- **Added design details**: Start, submit, and resubmit state management.

- **Entry conditions**: Own assigned job within the valid period. Scheduled/reactive/preventive maintenance share one work-state model.
- **Main flow**: Check assignment/schedule → start → edit report → submit → await quality review → rework/resubmit if returned.
- **Business rule BR-T08**: Start only from assigned; resume rework from rework_requested. Submitted versions are read-only. Technicians cannot approve on behalf of customers.
- **Resulting business state**: Save start/submission times and reportVersion. The quality reviewer decides completion.
- **Boundaries/prohibitions**: Reject start/submit when unassigned, cancelled, on_hold, or outside the valid period. Submission failure retains in_progress and draft content.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-T08-N | tech-external-a, assigned job-contractor-a within work window. When: start → saveDraft with `acceptancePatches["shared:report-draft-all-normal"]` → submit → contractor-a returns (outsourced quality review, IR93) → resumeRework → submit | ① in_progress, startedAt ② submitted, reportVersion=1 ③ rework_requested ④ Back to in_progress, submit new v2 |
| AT-T08-E | ① Start unassigned requested job-internal-a / start after HQ cancels job-contractor-a / start job-contractor-a after starting and HQ jobs.hold sets on_hold ② Submit outside assignment period ③ UNAVAILABLE on submit in_progress | ① NOT_FOUND / FORBIDDEN (errors.assignment_ended) / CONFLICT (IR93) ② FORBIDDEN ③ Retain in_progress/draft |
| AT-T08-B | ① Start assigned ② Edit submitted ③ resumeRework from rework_requested | ① in_progress ② Read-only, cannot save ③ in_progress |

Design: [DD-T08](../02-design/technician.md#dd-t08-details). Assess parent AT-T08 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-T09 Work reports

- **Company request basis**: SRC-06 BIZ-12 — Scheduled, reactive, and preventive maintenance, including general non-RTO maintenance.
- **Added design details**: Photos, replaced parts, and report versions.

- **Entry conditions**: in_progress or rework in progress; inspection fields and draft loaded.
- **Main flow**: Enter inspections/readings/photos/replaced parts/work/next actions → save draft → validate before submission → finalize report version.
- **Business rule BR-T09**: Keep author/version. Images: JPEG/PNG, at most 5MiB each and 10 images. Part quantities are positive integers. Next action is explicitly none or a date/time and description. Refetch during the work window preserves dirty input. Warn 15 minutes before the work window ends; at the end, discard unsaved input with a notice (IR89).
- **Resulting business state**: Successful save updates draft version. Submission links finalized reportVersion to Job. Deleting a photo releases its object URL.
- **Boundaries/prohibitions**: Reject submission with body ≤9 or ≥4001 characters or part quantity 0. Reject adding false file types, an 11th image, or images over 5MiB. Mark failed image processing as failed; submission is blocked while failed photos remain. Preserve already saved text/images.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-T09-N | tech-external-a starts job-contractor-a (in_progress). When: Add one part to `acceptancePatches["shared:report-draft-all-normal"]` (18 normal components, 50-character body, nextAction=none) → saveDraft → attachments.add one JPEG → submit latest version | ① First body save: draft version=1; photo addition increments version by 1, Attachment status=ready; submit version from latest reports.get ② Submission fixes reportVersion and Job.reportRefs ③ Deleting photo releases object URL |
| AT-T09-E | ① Body 9/4001 characters ② False MIME ③ 11th image ④ 5MiB+1byte ⑤ Part quantity 0 ⑥ Failed photo remains | ①⑤⑥ Submission VALIDATION ②③④ Addition rejected; preserve saved text/images in every case |
| AT-T09-B | ① Ten JPEG/PNG images exactly 5MiB each ② Quantity 1 ③ nextAction=follow_up with future date/time and content ④ reports.get refetch while dirty | ① Can add ② Can submit ③ Can submit ④ Dirty input retained |

Design: [DD-T09](../02-design/technician.md#dd-t09-details). Assess parent AT-T09 using all N/E/B and applicable SRC/R01 cases in traceability.

### FR-T10 Remote diagnostics and test runs

- **Company request basis**: SRC-06 BIZ-13 — Check temperature with smart thermostats and change settings remotely.
- **Added design details**: Technician control permissions and test-run steps.

- **Entry conditions**: Within assignment period, `control.diagnose` capability, device online, and no breach of contract restrictions.
- **Main flow**: Check current state/assigned job → specify diagnostic action, duration, reason → confirm → view Command response/history.
- **Business rule BR-T10**: Cannot start during firmware updates or while a Command awaits response. Ending a test run also needs an end-Command response; a browser timer ending does not prove physical shutdown.
- **Resulting business state**: Normal diagnostics create a Command with reason/jobId. A test run creates a DiagnosticRun holding duration/end action, plus start/end Commands. Show scheduled end separately from actual end response; record end failures as warnings.
- **Boundaries/prohibitions**: Reject actions beyond restricted temperatures, outside assignment periods, 16-minute test runs, or missing reasons. Do not show “Stopped” without an end response.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-T10-N | tech-external-a, job-contractor-a within work window, control.diagnose, unit-online-rto online. When: Normal diagnostic set_mode=cool with jobId=job-contractor-a and reason → Command sent→acknowledged → start test run with startAction=set_power true, endAction=set_power false, duration=5 → start response → end time → end response | ① One Command with jobId/reason ② DiagnosticRun awaiting_start→running, endAt=startedAt+5 minutes ③ end_requested, one end Command ④ completed, “Stopped” |
| AT-T10-E | ① With `acceptancePatches["AT-T10-E.1"]`, tech-internal-a requests 23°C for assigned job-t10-limited on unit-limited (minimum 24°C) ②–⑤ tech-external-a/job-contractor-a: ② Outside assignment period ③ durationMinutes=16 ④ Empty reason ⑤ No end response | ① FORBIDDEN ② FORBIDDEN ③④ VALIDATION ⑤ end_failed, no “Stopped” |
| AT-T10-B | ① Unfinished Command ② Firmware updating ③ End time reached only ④ End response received | ①② CONFLICT ③ end_requested, not stopped ④ completed |

**Additional acceptance AT-T10-R01 (revisit, conflict, cross-role)**

| Acceptance ID | Given / When | Then |
|---|---|---|
| AT-T10-R01 | Start response at 01:00:05, duration=5, endAction=OFF. Leave screen/switch roles, then advance to 01:05:05 | One end Command, reauthorize original Membership. Not stopped until end response; afterward completed. If authorization expired, end_blocked and zero requests. Old post-reset events apply zero changes. |

Design: [DD-T10](../02-design/technician.md#dd-t10-details). Assess parent AT-T10 using all N/E/B/R01 and applicable SRC cases in traceability.

### FR-T11 IoT lifecycle

- **Company request basis**: SRC-06 BIZ-20 — Firmware updates and removal/theft protection/notifications for small low-cost devices inside AC units.
- **Added design details**: Simulated registration, calibration, and update steps.

- **Entry conditions**: `device.maintain` permission and valid target-unit assignment. Registration/calibration/updates are all mocks.
- **Main flow**: Register serial → bind unit → check connection → record calibration values/reference → select supported firmware version → update → view progress/result.
- **Business rule BR-T11**: Trim serial and uppercase before comparing uniqueness. Registration sensorTypes allows only metrics in target Capability.sensors; copy units, stale seconds, and boundaries from capability (IR43). Calibration appends history without changing existing readings. Firmware must come from supported version choices; no free-form URL/file input.
- **Resulting business state**: Save Device, CalibrationRecord, and DeviceOperation history. Update firmwareVersion only on succeeded. Block concurrent actions during updates.
- **Boundaries/prohibitions**: Reject duplicate serials, unauthorized rebindings, and unit mismatches without changing the register. Do not start firmware updates offline. Failed updates show failed and retain the old version.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-T11-N | `acceptancePatches["AT-T11-N"]`: unbound unit-t11-new and assigned job-t11 for tech-internal-a with device.maintain. When: Register serial ac-001, sensorTypes=[temperature, power], jobId=job-t11, unitId=unit-t11-new → bind unit-t11-new → check → after start tick, demo.trigger(operation succeeded) → calibrate temperature in same unit → update FW v2 → success event after start tick | ① Device created, serial=AC-001 ② Bind succeeds ③ DeviceOperation succeeded ④ CalibrationRecord added, existing readings unchanged ⑤ firmwareVersion=v2 |
| AT-T11-E | tech-internal-a: ① After AT-T11-N, register “ac-001 ” again ② Rebind another unit without reason ③ After demo.trigger(communication_lost) on AT-T11-N's bound device, updateFirmware with jobId=job-t11 ④ Calibration unit mismatch ⑤ Firmware failure event | ① CONFLICT ② VALIDATION ③ DomainError OFFLINE, zero DeviceOperations (IR94) ④ VALIDATION ⑤ failed, old version retained |
| AT-T11-B | ① Lower/uppercase serial ② Reading history before/after calibration ③ Supported/unsupported firmware | ① Same identity ② Append history, existing values unchanged ③ Only supported versions selectable |

**Additional acceptance AT-T11-R01 (revisit, conflict, cross-role)**

| Acceptance ID | Given / When | Then |
|---|---|---|
| AT-T11-R01 | Devices a/b share FW candidate v2. Update deviceId=a and send success event | Only a becomes v2; b keeps old version. Missing deviceId returns VALIDATION; out-of-scope ID rejected. |

Design: [DD-T11](../02-design/technician.md#dd-t11-details). Assess parent AT-T11 using all N/E/B/R01 and applicable SRC cases in traceability.

### FR-T12 IoT faults

- **Company request basis**: SRC-06 BIZ-20 — Firmware updates and removal/theft protection/notifications for small low-cost devices inside AC units.
- **Added design details**: Distinguish communication loss, power loss, and removal.

- **Entry conditions**: Permission to view assigned Device events.
- **Main flow**: Simulate communication loss, power loss, and removal separately → check notifications → write response note → record recovery/confirmation.
- **Business rule BR-T12**: Manage connection and tamper separately. Event type determines evidenceSource (IR74). Classify power loss only with a dedicated demo power signal; missing heartbeat alone is not proof of power loss.
- **Resulting business state**: Save detection time, observed evidence, response, and recovery time as separate events. Acknowledging notifications does not change physical device state.
- **Boundaries/prohibitions**: Reconnection does not clear unacknowledged tamper alerts. Old out-of-order heartbeats cannot restore online state.

| Acceptance ID | Given / When | Then (observable result) |
|---|---|---|
| AT-T12-N | `acceptancePatches["AT-T12-N"]` assigns unit-non-rto job-t12 to tech-internal-a within work window (IR94). device-tamper. When: communication_lost→power_lost→tamper→responseNote→restored | ① Three separate events with occurredAt ② responseNote leaves physical state unchanged ③ restoredAt saved, tamper alert remains |
| AT-T12-E | With `acceptancePatches["AT-T12-N"]`: ① Restore communication while tamper is unacknowledged ② After communication_lost(sequence=5), send restored(axis=connection, sequence=4) (IR102) | ① Tamper alert remains ② Remains offline |
| AT-T12-B | ① Missing heartbeat ② Lost power_signal ③ tamper_signal | ① Offline only ② Power loss ③ Tamper; each independent |

Design: [DD-T12](../02-design/technician.md#dd-t12-details). Assess parent AT-T12 using all N/E/B and applicable SRC/R01 cases in traceability.


0.9.0 correction contracts: Read [strict review correction contracts](../02-design/strict-review-contracts.md) and [operation version contracts](../02-design/write-version-catalog.csv) together.

Additional current 0.21.0 contracts: Read [re-review correction contracts](../02-design/review-resolution-contracts.md) IR01–106. They override older text on the same issues; use IR72 for conflict priority.

Job lists support ascending/descending sorting by status (business order), severity, and deadline. Default: status in business order (IR34). Sort all results before pagination; language changes do not change order. Also use AT-REV16-005 for acceptance.
