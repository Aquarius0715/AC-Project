---
document_id: DD-T
version: 0.21.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# Technician Detailed Design

This design defines features, screen fields, states, and exceptions based on the original company document and its linked requirements. It defines the processing and acceptance criteria for each FR (functional requirement). Reference mock screens guide the shared UI appearance.

**Implementation baseline for 0.21.0**: Read all chapters of the [Deterministic Contracts](deterministic-contracts.md) and strict-review-contracts.md, the authorization columns of the operation catalog, and the screen catalog together. Do not guess values, permissions, asynchronous behavior, or recovery during implementation. These are demo design proposals, not approval for production business use.

## Inputs and Responsibilities

The primary source is the [Original Company Requirements (SRC-06)](../00-prepare/sources/company-requirements-original.txt). Screens, inputs, states, and acceptance criteria follow the requirements reorganized from this source. Inputs are the [Role Requirements](../01-requirements/technician.md) and [Common Requirements](../01-requirements/common.md). Required reading is the [Common Detailed Design](common.md) and [UIUX Specification](../03-uiux/UIUXSpecification.md). The following describes frontend fields, displays, and mock behavior. Registration, assignment, payment receipt, restrictions, and audit on the screen are state changes in shared mock memory. This does not request server implementation or database design.

Treat route parameters as untrusted input and always validate them. Service names in the table are logical operations in the shared Repository. Rows with the same route describe different functions on one screen. Every row supports loading, empty, error, forbidden, and not-found displays. Show retry only for recoverable errors. For forbidden and not-found, follow IR57 and do not show retry.

## Screen and Process Design

| Design ID / requirement | Route / main component | Read and action contracts | Input, processing, validation | Errors and prohibited actions |
|---|---|---|---|---|
| DD-T01 / FR-T01 | `/technician` / `TechnicianOverview` | `jobs.list`, `alerts.list`, `summaries.get` | Filter by period and severity. Check assignments and deadlines in the service | Exclude jobs assigned to others from results and summaries |
| DD-T02 / FR-T02 | `/technician/units/:id` / `DiagnosticUnit` | `units.get`, `devices.list` | View unit ID and supported features. Clearly identify items outside maintenance scope | Distinguish unregistered units from disconnected units |
| DD-T03 / FR-T03 | `/technician/units/:id` / `TelemetryPanel` | `telemetry.series`, `telemetry.summary` | Select metric and period. Subscribe through the Repository. Phase 1A uses only predefined events | Do not show old values as live. Refetch when the stream disconnects |
| DD-T04 / FR-T04 | `/technician/jobs/:id` / `InspectionForm` | `jobs.get`, `jobs.saveDraft`, `jobs.submit`, `reports.get`, `units.get` | Select normal/needs attention/not inspected/not applicable for each component. Record findings and measurement evidence. Do not default to normal | Require reasons for not applicable or not inspected. Do not replace actual measurements with demo diagnosis results |
| DD-T05 / FR-T05 | `/technician/jobs/:id` / `InspectionForm` | `jobs.get`, `jobs.saveDraft`, `jobs.submit`, `reports.get`, `units.get` | Outdoor units use the shared schema with a separate component group | Do not claim detection of tiny leaks without a selected sensor |
| DD-T06 / FR-T06 | `/technician/jobs/:id` / `InspectionForm` | `jobs.get`, `jobs.saveDraft`, `jobs.submit`, `reports.get`, `units.get` | Record each measurement's value, unit, observation time, and inspector. Operating procedures and installation instructions are outside this design | Do not save an unmeasured value as 0 or normal |
| DD-T07 / FR-T07 | `/technician/units/:id/alerts` / `DiagnosticEvidence` | `alerts.list`, `alerts.get`, `alerts.acknowledge`, `alerts.resolve` | Acknowledgement only marks the alert as acknowledged. Resolution requires remeasurement or a reason recorded by an authorized person | Do not infer theft from connection loss alone. Treat removal detection as a separate event |
| DD-T08 / FR-T08 | `/technician/jobs/:id` / `JobWorkspace` | `jobs.get`, `jobs.submit`, `reports.get`, `jobs.start`, `jobs.resumeRework`, `units.get` | Check active assignment and start conditions. Submission enters quality-review waiting state | Reject submission after assignment expiry or cancellation. Keep the draft if sending fails |
| DD-T09 / FR-T09 | `/technician/jobs/:id` / `ReportEditor` | `jobs.get`, `jobs.saveDraft`, `jobs.submit`, `reports.get`, `attachments.add`, `attachments.getContent`, `units.get` | Report body: 10–4000 characters. Photos: JPEG/PNG, at most 5MiB each, at most 10 (provisional). Part quantities must be greater than 0 | Drafts may be incomplete. Validate the schema on submission. Reselect images after processing failure, while keeping text |
| DD-T10 / FR-T10 | `/technician/units/:id/control` / `DiagnosticControl` | `commands.create`, `commands.get`, `diagnosticRuns.create`, `diagnosticRuns.get`, `units.get`, `jobs.get`, `diagnosticRuns.list` | Check `control.diagnose` permission, unit capabilities, reason, and test-run duration (1–15 minutes, provisional) | Do not use test runs to bypass contract restrictions. Do not automatically resend after expiry |
| DD-T11 / FR-T11 | `/technician/devices` / `DeviceMaintenance` | `devices.list`, `devices.register`, `devices.bind`, `devices.check`, `devices.calibrate`, `devices.updateFirmware`, `devices.get`, `units.list`, `units.get`, `jobs.list`, `devices.calibrations`, `devices.operations` | Serial numbers must be unique. Set unitId and sensor types. Enter unit, reference value, and date/time for calibration. Choose supported firmware versions | Do not start updates offline. Do not show the new version as applied after update failure |
| DD-T12 / FR-T12 | `/technician/devices/:id` / `DeviceEvents` | `devices.get`, `devices.events`, `alerts.get`, `alerts.acknowledge`, `devices.addResponseNote` | Show eventType and detection evidence. Use a dedicated simulated event for removal | Restored communication does not automatically clear removal alerts |

## Shared Implementation Steps

1. Check the session and permission scope. Validate IDs and URL filters against schemas.
2. Call mock services through Queries and receive screen models.
3. Build forms with React Hook Form and shared schemas. Also validate capabilities and periods.
4. Immediately before a mutation, check the target version, permissions, and current state. For major actions, also confirm the target and reason.
5. Change shared demo state through the Repository. Emit events with a correlation ID for tracking. Invalidate related Queries so they can be refetched.
6. Keep a waiting indicator visible while awaiting a response. Show success, denial, and failure separately. Keep form inputs when submission fails.

## Handoff to Testing

For each DD-T number, verify the matching AT-T acceptance criteria, the error cases above, and unauthorized calls. The [Verification Plan](../04-agentic-sdlc/verification.md) is the source of truth for test data and cross-role scenarios. If example limits such as character counts change, update schemas, documents, and boundary tests together.

## Detailed Feature Specifications (0.6.0)

Keep form values in RHF (React Hook Form) and validate them with schemas. Read-only screens do not require form validation. Follow input/output contracts DDC-03 and DDC-09 for audit, notifications, and shared errors. Display read-only values from a single Query source. The [Implementation Contracts](implementation-contracts.md) define shared types, paging, time, and error handling; the following adds individual conditions. Use the tokens and patterns in [UIUX](../03-uiux/UIUXSpecification.md) UX-04 and UX-08 for appearance.

### DD-T01 Details

**Source mapping**: SRC-06 BIZ-04, BIZ-08 → FR-T01 → DD-T01. Source category: original company requirements SRC-06 + design additions. Design addition: view scope based on assignment period. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-T01 / Main display pattern: **UI-OVERVIEW**. Service boundary: `jobs.list, alerts.list, summaries.get`.

**Initial view and prerequisites**: Internal technicians can view their assigned scope. External technicians can fetch jobs assigned individually within their company and their work periods. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| from / to | date/required | Default: today; maximum 366 days. Convert YYYY-MM-DD calendar dates in the display timezone to UTC Instants (IR74) | Schedule |
| status | enum/optional | assigned/in_progress/on_hold/submitted/rework_requested/completed/all (all means omit status, IR90) | Progress |
| severity | enum/optional | critical/warning/all (all means omit filters.severity, IR74) | Priority |
| summary | Read-only | Assigned unit count, not started, overdue | Assigned-scope summary |

**Steps**

1. Open assigned jobs for today or the selected period. Sort by alert severity, deadline, and progress. Open unit details or the work screen.
2. Apply these business conditions to reads and actions. “Not addressed” means jobs assigned to the user that have not started, not all requested jobs. Internal users viewing multiple jobs must still stay within their tenant and assigned scope.
3. This screen is read-only. For zero scheduled jobs, show an empty state and a way to view past history.
4. Queries to update: `jobs / assignments / alerts`.

**Boundary cases and failures**: Hide live unit information when an external technician's assignment expires. Before work starts, show assigned jobs as read-only within the viewing window; actions before the work window return FORBIDDEN (IR49). Directly entering another technician's job ID must not allow work to start.

**Verification**: Check the traceability entries under AT-T01 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-T02 Details

**Source mapping**: SRC-06 BIZ-06, BIZ-07, BIZ-10 → FR-T02 → DD-T02. Source category: original company requirements SRC-06 + design additions. Design addition: unit register fields and viewing steps. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-T02 / Main display pattern: **UI-DETAIL**. Service boundary: `units.get, devices.list`.

**Initial view and prerequisites**: An assignment relationship grants read access to the unit. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| unitId | ID/required | Within assignment | Unit |
| manufacturer / model / installedAt | Read-only | Show null for unregistered values | Register |
| components / serviceScope | Read-only | components=all components in each serviceScope group (indoor 8, outdoor 5, electrical 5, IR100) | Inspection scope |
| capabilityVersion | Read-only | Basis for available actions | Capability version |

**Steps**

1. Open the unit register from the job. Check installation location, model, configuration, installation date, and maintenance scope. Continue to diagnosis or work.
2. Show model capabilities with their capability version. Show unregistered or unknown fields as “Not registered”; do not fill them with common model values.
3. This screen is read-only. Technicians do not change manufacturer register data, customer memberships, or billing data.
4. No Queries are updated (read-only).

**Boundary cases and failures**: Do not fill an absent installation date with today's date. Return no information when an external technician requests an unassigned unit.

**Verification**: Check the traceability entries under AT-T02 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-T03 Details

**Source mapping**: SRC-06 BIZ-08, BIZ-11 → FR-T03 → DD-T03. Source category: original company requirements SRC-06 + design additions. Design addition: choosing data series and displaying data quality. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-T03 / Main display pattern: **UI-DETAIL** and **UI-ANALYSIS**. Service boundary: `telemetry.series, telemetry.summary`.

**Initial view and prerequisites**: The user may view the target unit's telemetry. Connection information can be shown even without sensors. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| unitId / metric | Required | Within assignment; supported sensor | Target series |
| period / from / to | enum and ISO datetime/required | 1h/24h/7d/custom; default: 24h. 1h/24h are rolling windows, 7d uses calendar days, custom is at most 366 days (IR41) | Period |
| observedAt / receivedAt | Read-only | UTC | Freshness |
| staleAfterSeconds | Read-only | From sensor policy; demo: 120 seconds | Stale check |
| eventId / version | Read-only | Duplicate and ordering control | Update basis |

**Steps**

1. Select metric and period. Check sensor, power, operation, and connection values and their times. Demo update events change values. Make stopped updates clear when communication is lost.
2. Keep observation time (measured on site) separate from receipt time (received by the server). Mark data stale after staleAfterSeconds. Do not overwrite current values with older events. Keep series units fixed and leave gaps where data is missing.
3. Keep latest values and time-series charts consistent using the same eventId and version. Unsubscribe when leaving the screen or changing scope.
4. Queries to update: `telemetry / unit summary`.

**Boundary cases and failures**: Reordered or duplicate events must not roll values back. While offline, the last received value may remain with its timestamp, but must not be shown as live.

**Verification**: Check the traceability entries under AT-T03 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-T04 Details

**Source mapping**: SRC-06 BIZ-10 → FR-T04 → DD-T04. Source category: original company requirements SRC-06 + design additions. Design addition: inspection forms and reasons for skipped inspections. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-T04 / Main display pattern: **UI-FORM**. Service boundary: `jobs.get, jobs.saveDraft, jobs.submit, reports.get, units.get`.

**Initial view and prerequisites**: The user has an active assigned job in `in_progress`. Maintenance scope and target components have already been fetched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| componentGroup | enum/required | Fixed: indoor | Group |
| componentKey | enum/required | Component keys above | Target |
| result | enum/null | Default: null; normal/attention/not_inspected/not_applicable | Inspection result |
| reason | string/conditionally required | 1–1000 characters for attention and similar results | Evidence / reason for no inspection |
| measurements | Measurement array/optional | value+unit+observedAt+origin=inspection | On-site measurements |
| attachmentIds | ID array/optional | Only images from the same job | Evidence |

**Steps**

1. Select an inspection result for each target component. Link findings, measurements, and photos as needed. Enter a reason for not inspected or not applicable. Save as a draft or report.
2. The target group is `indoor`. Components are `filter / evaporator_coil / blower_motor / blower_fan / drain_pipe / drain_pan / outlet / louver`. The initial result is `null` (not entered). To submit without inspection, explicitly choose `not_inspected` and enter a reason. If the unit has no such component, choose `not_applicable` and record a reason.
3. Link inspection results to the report version, author, and observation time. Keep sensor estimates as separate evidence. Do not overwrite original sensor data with on-site inspection results.
4. Queries to update: `report draft / inspection items`.

**Boundary cases and failures**: Reject missing entries, measurements without units, not-inspected results without reasons, and photos from other jobs. Never default to normal and treat uninspected work as complete.

**Verification**: Check the traceability entries under AT-T04 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-T05 Details

**Source mapping**: SRC-06 BIZ-10 → FR-T05 → DD-T05. Source category: original company requirements SRC-06 + design additions. Design addition: inspection forms and reasons for skipped inspections. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-T05 / Main display pattern: **UI-FORM**. Service boundary: `jobs.get, jobs.saveDraft, jobs.submit, reports.get, units.get`.

**Initial view and prerequisites**: The user has an active assigned job in `in_progress`. Maintenance scope and target components have already been fetched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| componentGroup | enum/required | Fixed: outdoor | Group |
| componentKey | enum/required | Component keys above | Target |
| result | enum/null | Default: null; normal/attention/not_inspected/not_applicable | Inspection result |
| reason | string/conditionally required | 1–1000 characters for attention and similar results | Evidence / reason for no inspection |
| measurements | Measurement array/optional | value+unit+observedAt+origin=inspection | On-site measurements |
| attachmentIds | ID array/optional | Only images from the same job | Evidence |

**Steps**

1. Select an inspection result for each target component. Link findings, measurements, and photos as needed. Enter a reason for not inspected or not applicable. Save as a draft or report.
2. The target group is `outdoor`. Components are `condenser_coil / compressor / fan / blade / refrigerant_pipe`. The initial result is `null` (not entered). To submit without inspection, explicitly choose `not_inspected` and enter a reason. If the unit has no such component, choose `not_applicable` and record a reason.
3. Link inspection results to the report version, author, and observation time. Keep sensor estimates as separate evidence. Do not overwrite original sensor data with on-site inspection results.
4. Queries to update: `report draft / inspection items`.

**Boundary cases and failures**: Reject missing entries, measurements without units, not-inspected results without reasons, and photos from other jobs. Never default to normal and treat uninspected work as complete.

**Verification**: Check the traceability entries under AT-T05 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-T06 Details

**Source mapping**: SRC-06 BIZ-10 → FR-T06 → DD-T06. Source category: original company requirements SRC-06 + design additions. Design addition: inspection forms and reasons for skipped inspections. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-T06 / Main display pattern: **UI-FORM**. Service boundary: `jobs.get, jobs.saveDraft, jobs.submit, reports.get, units.get`.

**Initial view and prerequisites**: The user has an active assigned job in `in_progress`. Maintenance scope and target components have already been fetched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| componentGroup | enum/required | Fixed: electrical | Group |
| componentKey | enum/required | Component keys above | Target |
| result | enum/null | Default: null; normal/attention/not_inspected/not_applicable | Inspection result |
| reason | string/conditionally required | 1–1000 characters for attention and similar results | Evidence / reason for no inspection |
| measurements | Measurement array/optional | value+unit+observedAt+origin=inspection | On-site measurements |
| attachmentIds | ID array/optional | Only images from the same job | Evidence |

**Steps**

1. Select an inspection result for each target component. Link findings, measurements, and photos as needed. Enter a reason for not inspected or not applicable. Save as a draft or report.
2. The target group is `electrical`. Components are `thermostat / sensor / capacitor / contactor / wiring`. The initial result is `null` (not entered). To submit without inspection, explicitly choose `not_inspected` and enter a reason. If the unit has no such component, choose `not_applicable` and record a reason.
3. Link inspection results to the report version, author, and observation time. Keep sensor estimates as separate evidence. Do not overwrite original sensor data with on-site inspection results.
4. Queries to update: `report draft / inspection items`.

**Boundary cases and failures**: Reject missing entries, measurements without units, not-inspected results without reasons, and photos from other jobs. Never default to normal and treat uninspected work as complete.

**Verification**: Check the traceability entries under AT-T06 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-T07 Details

**Load Alerts Caused by Open Windows or Poor Insulation (BIZ-17)**

Add `causeCode` (`window_open` / `insulation_loss` / `unknown`), `evidenceKind` (`demo_observation` / `inferred` / `inspection`), `evidenceText` (evidence description), and `observedAt` (observation time) to the `Alert` returned by `alerts.list`. `causeCode` and `evidenceKind` are required. If evidence is unavailable, use `unknown`; do not hard-code claims such as “doubled.” Label inferred results “Suspected” and inspection results “Inspection record.” From notification details, users can open the unit with the same unitId or the maintenance request screen.

Verification: AT-T07-SRC uses three fixtures: suspected open window, inspection record of poor insulation, and no evidence. Each has different wording, evidence, and time. Marking a notification read alone does not resolve the alert.

**Source mapping**: SRC-06 BIZ-08, BIZ-11, BIZ-17 → FR-T07 → DD-T07. Source category: original company requirements SRC-06 + design additions. Design addition: possible causes, evidence, and resolution steps. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-T07 / Main display pattern: **UI-DETAIL**. Service boundary: `alerts.list, alerts.get, alerts.acknowledge, alerts.resolve`.

**Initial view and prerequisites**: An assigned unit has an alert or a suspected issue to diagnose. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| alertId | ID/required | Event on an assigned unit | Target |
| evidenceIds | Read-only array | Identify telemetry/inspection | Evidence |
| resolutionReason | string/required on resolution | 1–1000 characters (IR87) | Resolution decision |
| resolutionEvidenceIds | ID array/on resolution | Remeasurement IDs or confirmation record | Verification |
| expectedVersion | integer/required | Latest Alert version | Conflict |

**Steps**

1. Open evidence from the alert list. Check measured values, estimates, on-site inspection results, and their history. Acknowledge the alert. If needed, proceed to remeasurement or resolution with a reason.
2. Do not invent a numerical probability if an estimate has no confidence information. Acknowledgement sets `acknowledged`. Resolution requires a remeasurement that meets conditions, or a person with `alert.resolve` permission to resolve it with a reason.
3. Record detection, acknowledgement, and resolution times and actors. For recurrence, create a new alertId linked to the previous event.
4. Queries to update: `alerts / alert events / customer summary / admin summary / audit`.

**Boundary cases and failures**: Job completion alone does not set `resolved`. Automatic resolution through remeasurement applies only to Alerts with a policy; others require manual resolution with a reason (IR66). Connection loss alone does not prove theft. Treat removal as its own distinct event.

**Verification**: Check the traceability entries under AT-T07 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-T08 Details

**Source mapping**: SRC-06 BIZ-12 → FR-T08 → DD-T08. Source category: original company requirements SRC-06 + design additions. Design addition: start, submit, and resubmit states. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-T08 / Main display pattern: **UI-DETAIL**. Service boundary: `jobs.get, jobs.submit, reports.get, jobs.start, jobs.resumeRework, units.get`.

**Initial view and prerequisites**: The user is assigned an `assigned` job within the valid period. Periodic, reactive, and preventive maintenance use the same work-state model. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| jobId | ID/required | Active assignment | Target |
| startConfirmed | boolean/required at start | Default: false | Target confirmation |
| reportVersion | integer/required on submission | Current draft version | Submission |
| expectedVersion | integer/required | Current job version | State conflict |

**Steps**

1. Check assignment and schedule. Start work, edit the report, submit, and wait for quality review. If returned, perform rework and resubmit.
2. Work can start from `assigned`; rework can resume from `rework_requested`. After `submitted`, the submitted version is read-only. Technicians cannot approve on behalf of customers.
3. Save start time, submission time, and `reportVersion`. The quality review determines completion.
4. Queries to update: `jobs / reports / job events / notifications / audit`.

**Boundary cases and failures**: Reject start and submission if unassigned, cancelled, `on_hold`, or outside the valid period. Before the work window, show the job read-only in work-not-started state (IR76). Follow IR89 for the 15-minute end warning and discarding unsaved inputs at the end. If submission fails, keep `in_progress` and the draft.

**Verification**: Check the traceability entries under AT-T08 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-T09 Details

**Source mapping**: SRC-06 BIZ-12 → FR-T09 → DD-T09. Source category: design additions supporting company goals. Design addition: managing photos, replacement parts, and report versions. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-T09 / Main display pattern: **UI-FORM**. Service boundary: `jobs.get, jobs.saveDraft, jobs.submit, reports.get, attachments.add, attachments.getContent, units.get`.

**Initial view and prerequisites**: Work is `in_progress` or rework is underway. Inspection items and the draft have been fetched. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| workText | string/required on submission | 10–4000 characters | Work performed |
| inspectionItems | array/required on submission | Same component set as Unit.components at submission, result for every item, reasons for attention and similar results (IR100) | Checklist |
| photos | Attachment array/optional | JPEG/PNG, <=5MiB×10; submit only ready items | Photos |
| parts | array/optional | name: 1–120 characters; quantity: positive integer <=999 | Replacement parts |
| nextAction.kind | enum/required | none/follow_up | Next action |
| nextAction.date / note | datetime and string/conditionally required | Future date/time and 1–1000 characters for follow_up | Follow-up plan |

**Steps**

1. Enter inspection results, measurements, photos, replacement parts, work text, and next actions. Save the draft, run pre-submission checks, and finalize the report version.
2. Record the author and version. Images are JPEG/PNG, at most 5MiB each, up to 10. Quantities are positive integers. Next action is either `none` or an explicit date/time and description. Refetching service data during the work window must not erase unsaved (dirty) changes. At window end, discard and notify under IR89.
3. On successful save, update the draft version. On submission, link the fixed `reportVersion` to the Job. Revoke object URLs when deleting photos.
4. Queries to update: `draft / attachments / reports`; also `jobs` on submission.

**Boundary cases and failures**: Reject submission for text of 9 or fewer or 4001 or more characters, or part quantity 0. Reject spoofed MIME types, an 11th image, or images over 5MiB. Show `failed` for image processing failures; submission is blocked while failed photos remain. Keep saved text and images.

**Verification**: Check the traceability entries under AT-T09 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-T10 Details

**Source mapping**: SRC-06 BIZ-13 → FR-T10 → DD-T10. Source category: design additions supporting company goals. Design addition: technician control permissions and test-run steps. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-T10 / Main display pattern: **UI-DETAIL** and **UI-FORM**. Service boundary: `commands.create, commands.get, diagnosticRuns.create, diagnosticRuns.get, units.get, jobs.get, diagnosticRuns.list`.

**Initial view and prerequisites**: Within the assignment period, with `control.diagnose` capability, the device online, and no contract restriction exceeded. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| jobId / unitId | ID/required | Matching assignment | Basis |
| action | UnitAction/required | Within capabilities and restrictions | Diagnostic action |
| durationMinutes | integer/required for test run | 1–15; default: 5. Count from start acknowledgement | Test-run duration |
| reason | string/required | 1–1000 characters | Action reason |
| endAction | UnitAction/required for test run | Within capabilities; show in confirmation dialog | End action |

**Steps**

1. Check current state and assigned job. Set diagnostic action, test-run duration, and reason. Confirm the details, then view Command acknowledgements and history.
2. Do not start a new action during firmware (FW) updates or while a Command is unfinished. Ending a test run also requires an end-Command acknowledgement. A browser timer ending alone does not prove the device has stopped.
3. Use `commands.create` for normal diagnosis. Use `diagnosticRuns.create` for test runs and save jobId, reason, durationMinutes, and endAction in shared memory. Link start and end Commands by `runId`. Show scheduled end time separately from actual end acknowledgement. Record failed endings as warnings.
4. Queries to update: `commands / unit detail / audit`.

**Boundary cases and failures**: Reject attempts to bypass temperature limits, actions outside the assignment period, test runs of 16 minutes or more, and missing reasons. Follow IR46 for actions under restrictions and IR47 for rejection based on connection/power signals. Do not show stopped without an end acknowledgement.

**Verification**: Check the traceability entries under AT-T10 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-T11 Details

**Source mapping**: SRC-06 BIZ-20 → FR-T11 → DD-T11. Source category: design additions supporting company goals. Design addition: simulated registration, calibration, and update steps. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-T11 / Main display pattern: **UI-LIST**, **UI-FORM**, **UI-DETAIL**. Service boundary: `devices.list, devices.register, devices.bind, devices.check, devices.calibrate, devices.updateFirmware, devices.get, units.list, units.get, jobs.list, devices.calibrations, devices.operations`.

**Initial view and prerequisites**: `device.maintain` permission and an active assignment for the unit are required. Registration, calibration, and updates are mock operations. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| serial | string/required for registration | 3–64 letters, digits, or hyphens; unique after normalization | Identifier |
| sensorTypes | Metric array/required for registration | No duplicates; may be empty. Only metrics in the unit's Capability.sensors. Copy units, stale seconds, and bounds from capability (IR43) | Sensors to create |
| unitId | ID/required for binding | Assigned and registered | Target |
| metric / unit | enum/required for calibration | Sensor capability combination | Measurement type |
| referenceValue / measuredValue | number/required for calibration | Finite values, same unit | Calibration basis |
| calibratedAt | ISO datetime/required | Cannot be in the future | Calibration time |
| firmwareVersion | enum/required for update | Supported candidate other than current version | Target version |

**Steps**

1. Register the serial number. Bind it to a unit, check the connection, and record calibration and reference values. Select supported firmware and update it. Check progress and result.
2. Trim serial numbers and convert to uppercase before checking uniqueness. Calibration only adds history; it does not rewrite existing measurements. Firmware can only be selected from supported versions. Do not allow free-entry URLs or binary data.
3. Save `Device`, `CalibrationRecord`, and `DeviceOperation`. Update `firmwareVersion` only when the result is `succeeded`. Follow IR67 for queued → running and the connecting display. Reject control requests during updates as CONFLICT. Follow D05 for mutual exclusion.
4. Queries to update: `devices / operations / calibrations / capabilities / audit`.

**Boundary cases and failures**: Reject duplicate serial numbers, unauthorized rebinding to another unit, and unit mismatches, without changing the register. Do not start firmware updates offline. Show `failed` on update failure and keep the old version.

**Verification**: Check the traceability entries under AT-T11 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

### DD-T12 Details

**Source mapping**: SRC-06 BIZ-20 → FR-T12 → DD-T12. Source category: original company requirements SRC-06 + design additions. Design addition: distinguishing communication loss, power loss, and removal. Field types, required status, defaults, and action order are implementation proposals.

Scope: FR-T12 / Main display pattern: **UI-DETAIL** and **UI-TIMELINE**. Service boundary: `devices.get, devices.events, alerts.get, alerts.acknowledge, devices.addResponseNote`.

**Initial view and prerequisites**: The user may view events for the assigned Device. Display in this order: validate route/conditions → check session scope → fetch the required Queries. Distinguish “not yet loaded” from “zero results.”

| Field | Type / required | Default / constraints | Purpose |
|---|---|---|---|
| deviceId | ID/required | Within assignment | Target |
| eventType | enum/required for demo controls | communication_lost/power_lost/tamper/restored | Event |
| evidenceSource | enum/read-only | heartbeat/power_signal/tamper_signal. Derived from eventType (IR74) | Evidence |
| responseNote | string/required for response | 1–1000 characters | Confirmation details |
| occurredAt / restoredAt | Read-only | From demo clock | Detection / recovery |

**Steps**

1. Simulate communication loss, power loss, and removal detection separately. Check notifications, enter response notes, and record recovery or confirmation.
2. Manage connection and tamper states separately. Identify power loss only when the demo provides a dedicated power signal. Missing heartbeats alone do not prove power loss.
3. Save detection time, observation evidence, response details, and recovery time as separate events. Acknowledging a notification does not change the device's physical state.
4. Queries to update: `devices / alerts / device events / notifications / audit`.

**Boundary cases and failures**: Reconnection does not clear unacknowledged tamper alerts. An old heartbeat received out of order must not restore online state.

**Verification**: Check the traceability entries under AT-T12 (N/E/B and applicable SRC/R01) and the relevant S scenarios.

0.9.0 correction contracts: Read the [Strict Review Correction Contracts](strict-review-contracts.md) and [Per-Operation Version Contract](write-version-catalog.csv) together.

0.10.0: T12 fetches alerts.get using DeviceEvent.alertIds and acknowledges using Alert.version (SR23). Filter device history by scope at event time (SR24).

Additional contracts for current version 0.21.0: Read IR01–106 in the [Re-review Correction Contracts](review-resolution-contracts.md). They take priority over older text on the same topic; follow IR72 for conflict precedence.

Apply IR34 to job-list and jobs.list sorting. When URL sort is absent, use status:asc. Changing the selection discards cursor, keeps filters, and fetches page one of a new snapshot. Allow ascending/descending sorting by state, severity, or deadline.
