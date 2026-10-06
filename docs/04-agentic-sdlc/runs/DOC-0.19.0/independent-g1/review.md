# Independent G1 Review Report DOC-0.19.0 (2026-09-17)

- Reviewer: Independent G1 review AI (another Agent, running separately from the DOC-0.19.0 correction author/self-reviewer)
- Target: Repository `/Users/ji1wxs/PraditaProjects/AC_Project`, branch `docs/0.19.0-independent-review`, commit `0505446`
- spec_baseline_id: `048ffcef25f8463bca33346f5bb1dd8dbb1fdad03b61132d1cfc5d17ebeb1ac2` ([spec-manifest](../spec-manifest.json), 61 files)
- Scope: 1A, a clickable frontend mock demo. APIs/HTTP, DB, servers, real devices, and real payments are pending production matters under DEC-12/D11 and are not counted as blockers for this decision.
- The decision uses only the current baseline's document text. Only the header of the correction author's self-review (../review.md) was read after the decision. Findings from past versions (runs/DOC-0.17.0 and earlier, 05-document-review) were neither consulted nor repeated.
- The application is not implemented; application tests were not run (not_run).

## 1. Decision Summary

**G1 decision: FAIL**

| Severity | Count |
|---|---|
| BLOCKER | 0 |
| CRITICAL | 0 |
| MAJOR | 12 |
| MINOR | 17 |
| QUESTION | 2 |

Both validators (validate_documents.py and check_review_regressions.py) passed. Independently recalculated manifest SHA-256 values and baseline ID also matched. Mechanical catalog consistency checks found no differences: 137 canonical operations versus operation catalog, 76 writes versus version catalog, 35 Query-input reads versus Query catalog, screen operation references, and operation names in the IR71 invalidation table.

However, cross-document review found 12 MAJOR defects that prevent implementation and test Agents from proceeding without guesses:

1. **G1-001:** Authorization for internal-technician writes without jobId (alerts.acknowledge, devices.addResponseNote/calibrate/updateFirmware, etc.) conflicts between assigned-valid-job/SR03 and AT-T07-N/AT-T12-N/AT-T11-E③/AT-REV19-021. Catalog qualifiers such as assigned, assigned-valid-job, and job-required are undefined.
2. **G1-002:** Business-event notifications (job requests/offers/acceptance/assignment, report submission/return/completion, payment confirmation, device faults, etc.) lack templateKey/type/channel/deliveryState/recipient Membership rules. The templateKey enum also lacks matching values. AT-P03-N④ depends on this.
3. **G1-003:** restrictions.cancel on requested/applied conflicts between IR35's three release-start paths and DD-A10/common.md §5/AT-A10-B②/E④, which start release on cancellation.
4. **G1-004:** AT-C06-E.3 fixtures (also AT-A13-E/B, AT-REV18-024, AT-REV19-041) mark 120kW as valid, violating D07 power/kW [0,100] and IR12 out-of-range→suspect.
5. **G1-005:** AT-A12-N/B notification-only behavior for unsupported ventilation cannot hold under IR21/SR25 because seeded unit-non-rto/device-tamper has no CO₂ Sensor.
6. **G1-006:** allergenObservation (BIZ-18) has no source data, derivation, or fixture input path, so AT-C07-SRC/AT-A12-SRC cannot be built.
7. **G1-007:** Conditions for customer air-screen ventilation/cleaning guidance are undefined; AT-C07-N② cannot be assessed.
8. **G1-008:** Required inspection sets on submission (UnitDetail.components derivation) are undefined. AT-T04/T05/T06/T08/T09-N lacks other-group inspections, workText, and nextAction needed for success.
9. **G1-009:** AT-A05-N omits name/recoveryThreshold/cooldownMinutes/escalateAfterMinutes and enabled. IR07 default enabled=false cannot produce the expected Alerts/notifications.
10. **G1-010:** AT-C04-E③/AT-FIX-019 use 2026-03-07/2026-10-31 outside all seeded Membership validity [2026-09-01,2026-10-01), leaving expected VALIDATION ambiguous.
11. **G1-011:** AT-REV19-015 patches only Assignment.scheduledEnd, while IR89's window-ended/reassignment display uses Job.scheduledSlot.endAt, so the result cannot appear.
12. **G1-012:** Older acceptance plans required by verification.md (AT-REV17-004/005/014, AT-REV18-003) conflict with current IR57/IR74/D05/IR47 and seed.

## 2. Validator Results

Executed from the repository root. Both exited with code 0.

### 2.1 `python3 docs/tools/validate_documents.py`

- Exit code: 0
- `errors`: `[]`
- `baseline`: `048ffcef25f8463bca33346f5bb1dd8dbb1fdad03b61132d1cfc5d17ebeb1ac2`, `spec_files`: 61
- Counts: requirements 64, acceptance_bundles 182, role_details 49, operations 137, screens 48, components 69, query_contracts 35, write_version_branches 94, review_019_cases 42, proposed_decisions_019 10, pending_business_decisions 0
- `application_tests`: not_run; `typescript_semantic_check`: not_run (the validator itself does not run TypeScript checks)

### 2.2 `python3 docs/tools/check_review_regressions.py`

- Exit code: 0
- All 62 mutation cases had `detected: true` and `passed: true`.

### 2.3 Additional Independent Checks

- Recalculated SHA-256 for all 61 manifest files; all matched. SHA-256 of spec_files JSON using the manifest's normalization rules matched `048ffcef…1ac2`. Only `.gitkeep` was outside the manifest among docs files, excluding runs and 05-document-review.
- Copied service-contracts.ts to a scratchpad and checked it with TypeScript 5.8.3 using `tsc --strict --noEmit --target es2022 --lib es2022,dom`; exit 0. Specification files were unchanged.
- Observation (not counted as a specification finding): runs/DOC-0.19.0/static-check.json records typescript_semantic_check=passed, but actual validate_documents.py output is not_run. The self-reviewer presumably inserted a separately run tsc result manually; the evidence file and validator output have different provenance.
- git status was unchanged before and after validator execution.

## 3. Findings

Quoted line numbers refer to files at commit `0505446`. “Requirements” may abbreviate 01-requirements/*.md and “Design” may abbreviate 02-design/<role>.md.

### 3.1 MAJOR

#### G1-001

- Severity: MAJOR
- Issue ID: G1-001
- Document: 02-design/operation-catalog.csv, 02-design/strict-review-contracts.md (SR03), 02-design/review-resolution-contracts.md (IR49/IR67), 02-design/implementation-contracts.md (DDC-07/DDC-08), 01-requirements/technician.md, 04-agentic-sdlc/acceptance-review-019.csv
- Location: operation-catalog.csv:3 (alerts.acknowledge), :20 (commands.create), :35 (devices.addResponseNote), :37 (devices.calibrate), :45 (devices.updateFirmware) / strict-review-contracts.md:24 / review-resolution-contracts.md:343, 345, 430 / implementation-contracts.md:240, 288 / deterministic-contracts.md:21, 25, 83 / technician.md:201 (AT-T07-N), :284 (AT-T11-E③), :308 (AT-T12-N) / acceptance-review-019.csv:22 (AT-REV19-021)
- Problem: The operation catalog defines technician:alert.resolve:assigned for alerts.acknowledge/resolve, technician:device.maintain:assigned-valid-job for devices.addResponseNote/bind/calibrate/check/updateFirmware/register, and technician:control.diagnose:job-required for commands.create. However, assigned/assigned-valid-job/job-required and other qualifiers are undefined outside the catalog (no matches in a full-text search). SR03 applies required Assignment conditions to internal technicians' writes, and DDC-08 requires a valid assignment for addResponseNote. IR49 explicitly discusses only writes with jobId: internal unit-scope reads follow SR03, while job-linked writes outside the work window are FORBIDDEN. In contrast, AT-T12-N (tech-internal-a notes on device-tamper), AT-T07-N (tech-internal-a acknowledges alert-temp-a, with no Job/Assignment patch), and AT-REV19-021 (tech-internal-a calibrates device-online-rto) expect success without assigned jobs. AT-T11-E③ expects OFFLINE for tech-internal-a updating device-offline-rto firmware with no job on unit-offline-rto. If a job is required, D01 priority-4 FORBIDDEN comes first. Even without a job requirement, IR67 creates a queued operation that becomes failed/failureCode=OFFLINE one second later, leaving DomainError versus operation failureCode unclear.
- Why it matters: The Repository role-access matrix is unsettled. IR72 ranks SR03/catalog above ATs, but even those rules cannot resolve undefined assigned semantics. Technician acceptance, including P0 FR-T07, passes or fails depending on implementation choices.
- Example Failure: Implementation A follows assigned-valid-job and returns FORBIDDEN for tech-internal-a addResponseNote without a job, failing AT-T12-N②. B allows unit scope alone and passes that case but violates job-required catalog tests and SR03. AT-T11-E③ gives FORBIDDEN in A, while B returns DeviceOperation then failed/OFFLINE after one second; neither clearly matches the unspecified OFFLINE observation.
- Required Fix: Define catalog authorization qualifiers in a legend. Put an internal/external × jobId present/absent × inside/outside work-window matrix in one place for alerts.acknowledge/resolve, devices.*, commands.create, and diagnosticRuns.create. Align AT-T07-N/T12-N/T11-E③/AT-REV19-021, adding Job/Assignment patches if needed. State whether AT-T11-E③ observes DomainError or DeviceOperation.failureCode.
- Suggested Revision: Define IR94: client:self means own-customer organization scope; contractor:accepted-valid-offer means the IR23 summary-projection period; technician:assigned means Membership.scopes for internal staff and own Assignment viewing window for external staff; technician:*:assigned-valid-job means input jobId is within a valid Assignment work window. Explicitly allow or reject internal technicians omitting jobId within unit scope. Change AT-T11-E③ Then to DeviceOperation.status=failed, failureCode=OFFLINE, firmwareVersion unchanged (IR67).

#### G1-002

- Severity: MAJOR
- Issue ID: G1-002
- Document: 02-design/implementation-contracts.md (Notifications and visibility), 02-design/service-contracts.ts, 02-design/deterministic-contracts.md (D08/D12), 02-design/review-resolution-contracts.md (IR10), 01-requirements/contractor.md
- Location: implementation-contracts.md:190-205 / service-contracts.ts:108 (Notification.templateKey), :166 (NotificationType) / review-resolution-contracts.md:64 (IR10), :32 (IR04), :36 (IR05), :337 (IR48) / deterministic-contracts.md:121-129, 169 / strict-review-contracts.md:183 (SR28) / contractor.md:115 (AT-P03-N④) / fixture-contract.json actors (three admins)
- Problem: Notification generation conditions and contents exist only for Policy Alerts (SR21/SR28), restriction notices (IR05), and reminders (IR04). The implementation-contracts notification/visibility table lists recipient categories for job.requested/offered/accepted/declined/assigned/schedule_changed, report.submitted/returned, job.completed, payment.confirmed, device.fault/operation_failed, and inquiry.received/answered, but does not define templateKey, Notification.type, channel, deliveryState (preview/simulated), or recipient Membership selection (for example, which of three fixture admins counts as HQ). The templateKey enum (alert/quality/schedule_change/report_return/completion/payment/payment_reminder/restriction/inquiry) has no values for requests, offers, acceptance, assignment, or device faults, so canonical types cannot express the table. D08 notification links assume job/device/inquiry targets.
- Why it matters: The business-event part of P0 FR-X07 has UI but no data definition. Implementation Agents must guess counts, recipients, and message keys; expected acceptance counts are also unsettled.
- Example Failure: For AT-P03-N④'s one notification preview to the assigned technician, implementation A saves six schedule_change/inApp/simulated Notifications for the customer, three admins, technician, and contractor. B returns only a technician preview. The Test Agent cannot choose.
- Required Fix: For each business event, define whether to generate, templateKey/type/channel/deliveryState/target, recipient Membership permission/scope selection, and counts. Add missing enum values or map existing ones. Align AT-P03-N④.
- Suggested Revision: Add rows such as job.assigned: templateKey=schedule_change, type=schedule_change, channel=inApp, deliveryState=simulated, target={kind:job}, recipients=one assigned technician Membership plus customer client Memberships able to view all target Units plus admins with job.manage. Also list events that generate nothing, such as IR48 expiry.

#### G1-003

- Severity: MAJOR
- Issue ID: G1-003
- Document: 02-design/review-resolution-contracts.md (IR35), 02-design/admin.md (DD-A10), 02-design/common.md §5, 01-requirements/admin.md (FR-A10), 02-design/service-contracts.ts
- Location: review-resolution-contracts.md:236, 238, 240 / Design admin.md:55, 395 / 02-design/common.md:181, 185 / Requirements admin.md:268 (BR-A10), :275 (AT-A10-E④), :276 (AT-A10-B②) / strict-review-contracts.md:127 (SR19) / service-contracts.ts:84, 281
- Problem: IR35 says release_requested has only three start paths (payment confirmation, defer/exempt, override), and separately defines restrictions.release with source=manual. restrictions.cancel is absent. DD-A10 says cancellation after application is requested proceeds to release even if application is unknown; common.md §5 says cancellation from requested onward proceeds to release. AT-A10-B② expects requested cancel→release_requested; AT-A10-E④ also expects release flow. Yet the same common.md §5 state table (:181) omits cancel from requested/applied→release_requested triggers. releaseIntent.source for cancel, whether D03 per-unit release evaluation runs, and cancel results in applied/release_requested (CONFLICT or otherwise) are missing. SR19 appears to limit cancel to notice stage: cancel notices, reconcile/release applied or unknown results.
- Why it matters: IR72 ranks IR35 (2) above DD (7) and AT (8), implying requested cancel does not trigger release_requested, but then its result is undefined. Release is central to P0 FR-A09/A10 and directly controls Command creation.
- Example Failure: A follows IR35 and returns CONFLICT for requested cancel, failing AT-A10-B②. B follows DD-A10 and creates release_requested/remove Command but uses an undefined releaseIntent.source.
- Required Fix: Define restrictions.cancel transitions for scheduled/requested/applied/release_requested/released/cancelled. Either add cancel to IR35 start paths or reject it in requested/applied and direct users to release. Align DD-A10, common.md §5, FR-A10, and AT-A10-B/E.
- Suggested Revision: Add IR35 path ④ cancellation: restrictions.cancel on requested/applied sets release_requested in the same transition with releaseIntent.source=cancel. scheduled becomes cancelled; release_requested returns current state idempotently; released/cancelled returns CONFLICT.

#### G1-004

- Severity: MAJOR
- Issue ID: G1-004
- Document: 04-agentic-sdlc/fixture-contract.json, 02-design/deterministic-contracts.md (D07), 02-design/review-resolution-contracts.md (IR12/IR69), 01-requirements/client.md, 01-requirements/admin.md, acceptance-review-018.csv, acceptance-review-019.csv, acceptance-fixes.csv
- Location: fixture-contract.json:1816-1879 (`acceptancePatches["AT-C06-E.3"]`, series value=120, expected kWh=120.0) / deterministic-contracts.md:101 / review-resolution-contracts.md:74, 438 / client.md:178 (AT-C06-E .3) / Requirements admin.md:344-345 (AT-A13-E/B) / acceptance-review-018.csv:25 (AT-REV18-024) / acceptance-review-019.csv:42 (AT-REV19-041) / acceptance-fixes.csv:18 (AT-FIX-017)
- Problem: D07 limits synthetic 1A power/kW to [0,100]; IR12 marks out-of-range raw values suspect and excludes them from summaries/control. AT-C06-E.3 patches one unit over one hour to actual 120kWh by setting every minute to 120kW, outside the range. It still sets quality=valid/qualityReason=null and expects kWh=120.0/savingPercentage=-20. AT-FIX-017 explicitly expects out-of-range values to be suspect and excluded from summaries. Whether patch rows receive range validation under IR69 canonical schema checks, or are rechecked during aggregation, is undefined.
- Why it matters: The negative-savings display acceptance premise (IR68/IR80) can only be built by violating higher-priority invariants. Normal demo.trigger telemetry input cannot reproduce it.
- Example Failure: A includes D07 ranges in DTO validation and throws during fixture creation. B excludes out-of-range values during aggregation, returning kWh=null/coverage=0 and failing AT-REV19-041. C trusts saved quality and returns 120.0, conflicting with AT-FIX-017 aggregation exclusion.
- Required Fix: Build E.3 within D07 ranges or change the range. Update fixtures, AT text, and AT-REV19-041 expectations together. State whether patch rows receive range checks.
- Suggested Revision: Option 1: Expand D07 power range to [0,200]kW. Option 2: Use unitIds=[unit-online-rto,unit-limited], 60kW each, baseline 100kWh for the same pair, retaining savedKWh=-20/savingPercentage=-20. Add to IR69 that patched measurements must meet D07/IR12 normalization rules; violations are fixture defects.

#### G1-005

- Severity: MAJOR
- Issue ID: G1-005
- Document: 01-requirements/admin.md (FR-A12), 04-agentic-sdlc/fixture-contract.json, 02-design/review-resolution-contracts.md (IR21/IR43/IR92), 02-design/strict-review-contracts.md (SR25), 02-design/admin.md (DD-A12)
- Location: Requirements admin.md:324 (AT-A12-N), :326 (AT-A12-B) / fixture-contract.json:1006-1045 (device-tamper sensors contain only temperature/humidity/power), :1158-1162 (unit-online-rto CO₂=800) / review-resolution-contracts.md:131 (IR21“No Sensor means missing_data”), :282 (IR43“Missing sensor=NOT_FOUND”), :679 (IR92 rule 2) / strict-review-contracts.md:159 (SR25“Notification conditions: valid metric quality and threshold/duration”) / Design admin.md:452 (DD-A12“unitIds/metric: a matching Sensor must exist”)
- Problem: AT-A12-N saves/evaluates metric=co2/threshold=1000ppm for ventilation-capable unit-online-rto and unsupported unit-non-rto, expecting notifications only for the unsupported unit; AT-A12-B is similar. But unit-non-rto's current device-tamper binding has no CO₂ Sensor. IR21 therefore gives missing_data and IR43 demo.trigger telemetry returns NOT_FOUND, so SR25 valid-quality notification conditions cannot hold. DD-A12 also requires a matching Sensor at save, potentially causing VALIDATION. AT-A12-N omits evaluation CO₂ values, observation times, durationSeconds, and recoveryThreshold; seeded unit-online-rto CO₂ is only 800ppm.
- Why it matters: The central FR-A12 branch (ventilation request on supported units, notification only otherwise) cannot be built from current seed. IR92 treats premises not fixed by rules as document defects.
- Example Failure: Using default unit-non-rto under IR92 gives suppressed/quality notifications instead of notification-only success. Another implementation fails DD-A12 save validation.
- Required Fix: Provide a unit with CO₂ sensing but no ventilation capability, by patching sensor-tamper-co2 into device-tamper or adding a Unit. Specify fact/telemetry values, times, durations, and expected FireResult/NotificationOutcome. Clarify whether DD-A12 matching Sensor means Capability.sensors or current-bound Device.sensors.
- Suggested Revision: Add acceptancePatches[AT-A12-N], with co2 (ppm, stale 120 seconds) in device-tamper sensors and measured/valid CO₂=1100ppm for unit-online-rto/unit-non-rto from 00:59:00–01:00:00. Given specifies fixture.airPolicyNotificationInput, recoveryThreshold=900, durationSeconds=60, enabled=true, recipient=hq-operator. Then: unit-online-rto results=requested (ventilate low), notifications=created; unit-non-rto results=suppressed/invalid_capability, notifications=created.

#### G1-006

- Severity: MAJOR
- Issue ID: G1-006
- Document: 02-design/service-contracts.ts, 02-design/deterministic-contracts.md (D12), 02-design/client.md (DD-C07), 02-design/admin.md (DD-A12), 01-requirements/client.md, 01-requirements/admin.md, 04-agentic-sdlc/fixture-contract.json, 02-design/review-resolution-contracts.md (IR69/IR92)
- Location: service-contracts.ts:316-317 (AllergenObservation, AirSeries), :142 (DemoTrigger) / deterministic-contracts.md:173 / Design client.md:252 / Design admin.md:434-440 / Requirements client.md:189 (AT-C07-SRC) / Requirements admin.md:311 (AT-A12-SRC) / fixture-contract.json demoSeed (no allergen section) / review-resolution-contracts.md:438, 682, 687
- Problem: allergenObservation has only an output type and availability display rules (available/not_measured/unsupported) as telemetry.series metadata. Its Repository source (stored resource, Capability attributes, or non-Measurement event) and the distinction between not_measured and unsupported are undefined. demoSeed has no allergen section and DemoTrigger has no allergen event. IR69 prohibits adding business records absent from seed and IR92 limits patch entities to seed sections, so no compliant fixture path creates the crafted observations required by AT-C07-SRC/AT-A12-SRC.
- Why it matters: The display explicitly mapped to company BIZ-18 allergens cannot be implemented or accepted because its data definition is missing: UI exists without data.
- Example Failure: One Agent can only return not_measured and cannot create the available fixture for AT-C07-SRC. Another invents a storage resource, violating IR69/D12.
- Required Fix: Define the source (for example, a new seed section or Capability/Device attributes plus a new DemoTrigger), availability derivation, latest-record selection, null behavior for multiple Units, and acceptancePatches keys.
- Suggested Revision: Add demoSeed.allergenObservations rows with id/unitId/substance/value/unit/sourceLabel/observedAt/evidenceText. Add Capability.allergenSupported:boolean: false→unsupported; true with no observations→not_measured; observations→available, selecting first by observedAt descending. Give AT-C07-SRC/AT-A12-SRC three acceptancePatches keys.

#### G1-007

- Severity: MAJOR
- Issue ID: G1-007
- Document: 01-requirements/client.md (FR-C07), 02-design/client.md (DD-C07), 02-design/deterministic-contracts.md (D08)
- Location: Requirements client.md:195 (Basic flow), :196 (BR-C07), :202 (AT-C07-N②“Guidance: Ventilation recommended”) / Design client.md:272-277 / deterministic-contracts.md:119
- Problem: FR-C07 requires ventilation/cleaning guidance. AT-C07-N expects “Ventilation recommended” for room-1 CO₂=1000ppm. Neither FR-C07, DD-C07, nor D08 defines metric/value/quality conditions for ventilation, conditions such as PM2.5 for cleaning, or display when conditions fail (full-text search found no rules). Only manual ventilation guidance for unsupported equipment is defined in D08.
- Why it matters: The implementation Agent must guess numerical conditions for company BIZ-18 cleaning/ventilation guidance, violating the documents' rule against guessing values during implementation. AT-C07-N② cannot be judged.
- Example Failure: A recommends ventilation at CO₂≥1000ppm; B only when A12 has a Policy threshold. Their displays for seeded 800ppm differ, and neither can be rejected by tests.
- Required Fix: Define metrics, operators, thresholds, required quality, wording keys by ventilation capability, and cleaning conditions. Add boundary subcases to AT-C07-N/E/B.
- Suggested Revision: Add to DD-C07: co2≥1000ppm and quality=valid→air.guidance.ventilate; pm25≥35µg/m³ and valid→air.guidance.clean; missing/stale/suspect shows only quality, no guidance (demo thresholds, DEC-09). Add 999/1000ppm boundaries to AT-C07-B.

#### G1-008

- Severity: MAJOR
- Issue ID: G1-008
- Document: 02-design/technician.md (DD-T02/DD-T09), 02-design/implementation-contracts.md (DDC-02), 02-design/service-contracts.ts, 01-requirements/technician.md
- Location: Design technician.md:94 (components/serviceScope), :288-293 (workText“Required on submission: 10–4000 characters”, inspectionItems“Required on submission: result/reason for all targets”, nextAction.kind“Required”) / implementation-contracts.md:119 / service-contracts.ts:43 (UnitDetail.components) / fixture-contract.json units (unit-online-rto serviceScope=indoor/outdoor/electrical) / Requirements technician.md:138 (AT-T04-N), :157 (AT-T05-N), :176 (AT-T06-N), :220 (AT-T08-N), :239 (AT-T09-N)
- Problem: Submission requires results/reasons for all target components, but derivation of UnitDetail.components from serviceScope is undefined. unit-online-rto covers three groups with 18 components. AT-T04-N enters only eight indoor components and expects submission success; AT-T05-N (five outdoor) and AT-T06-N (five electrical) do the same. These ATs also omit required workText and nextAction. AT-T09-N instead enters text/photos/parts/nextAction without inspections and expects success; AT-T08-N only says edit.
- Why it matters: Following the written input for P0 technician inspection/report acceptance (FR-T04–T06/T08/T09) gives VALIDATION instead of success. Undefined required-set derivation also leaves implementers unable to choose validation scope.
- Example Failure: Saving only eight indoor items under AT-T04-N then submitting fails because outdoor/electrical results are null and workText is missing. Another implementation accepts indoor alone, narrowing AT-T04-E①'s null-result validation to one group.
- Required Fix: Define serviceScope-group→ComponentKey derivation and the required jobs.submit validation set. Specify all inspection groups, workText, and nextAction in AT-T04/T05/T06/T08/T09-N, or define a shared submission-ready draft fixture.
- Suggested Revision: Add DD-T02 components=all ComponentKeys in each serviceScope group (indoor 8, outdoor 5, electrical 5). Change AT-T04-N to filter=attention with reason/one photo, other 17=normal, workText=50 characters, nextAction=none, save→submit. Specify complete inputs similarly for T05/T06/T09-N.

#### G1-009

- Severity: MAJOR
- Issue ID: G1-009
- Document: 01-requirements/admin.md (FR-A05/A11/A12), 02-design/admin.md (DD-A05), 02-design/review-resolution-contracts.md (IR07), 02-design/strict-review-contracts.md (SR21/SR28)
- Location: Requirements admin.md:161 (AT-A05-N), :293 (AT-A11-N), :324 (AT-A12-N) / Design admin.md:219-229 (DD-A05 required fields), :235 / review-resolution-contracts.md:48 (IR07“On creation: enabled=false, priority=50”) / strict-review-contracts.md:137-139 (SR21“disabled means suppressed”), :181 (SR28“Repository/UI must not invent numbers for required values”), :183 (“inApp is simulated”)
- Problem: AT-A05-N save input lists only unitIds/metric/gte 30°C/duration/severity/recipient/channel, omitting required name/recoveryThreshold/cooldownMinutes/escalateAfterMinutes and enabled. IR07 defaults enabled=false and SR21 suppresses disabled Policies, so one Alert and one Notification preview cannot result. AT-A12-N references fixture.airPolicyNotificationInput but omits enabled/name/recoveryThreshold/durationSeconds; AT-A11-N omits enabled/name/timezone. Also, inApp under SR28 saves deliveryState=simulated, while the text ambiguously calls it a Notification preview.
- Why it matters: SR28 prohibits UI/Repository from inventing required values. The Test Agent cannot save without guessing, and defaults cannot produce expected results. FR-A05 is P0.
- Example Failure: Saving only written AT-A05-N values returns VALIDATION for missing name and others. Even after filling them, default enabled=false gives zero Alerts.
- Required Fix: Specify all required inputs and enabled=true in AT-A05-N/A11-N/A12-N, or reference fixed Policy input objects in fixture-contract.json. Distinguish preview versus simulated by deliveryState.
- Suggested Revision: AT-A05-N saves name=Demo high temperature, unitIds=[unit-online-rto], metric=temperature, operator=gte, threshold=30, recoveryThreshold=28, durationSeconds=60, severity=warning, recipientMembershipIds=[customer-a], channels=[inApp], cooldownMinutes=5, escalateAfterMinutes=60, timezone=Asia/Kuala_Lumpur, enabled=true, priority=50. Then expects one Notification with deliveryState=simulated.

#### G1-010

- Severity: MAJOR
- Issue ID: G1-010
- Document: 01-requirements/client.md (FR-C04), 04-agentic-sdlc/acceptance-fixes.csv, 04-agentic-sdlc/fixture-contract.json, 02-design/review-resolution-contracts.md (IR36/IR92), 02-design/deterministic-contracts.md (D01/D09), 04-agentic-sdlc/verification.md
- Location: Requirements client.md:140 (AT-C04-E③) / acceptance-fixes.csv:20 (AT-FIX-019) / fixture-contract.json:14-15 (customer-a validFrom=2026-09-01, validUntil=2026-10-01; same for all actors), :371 (note) / review-resolution-contracts.md:246 (IR36“Membership.validUntil expires normally on clock jumps”), :678 (IR92 rule 1) / deterministic-contracts.md:18-21, 141 / verification.md:42 (`validFrom <= now < validUntil`)
- Problem: AT-C04-E③ sets the clock to 2026-03-07/2026-10-31, saves weekly rules, and expects VALIDATION; AT-FIX-019 also uses 2026-03-07. All seeded Memberships are valid only [2026-09-01,2026-10-01), so both dates are outside. Given does not override validity, and IR92 rule 1 retains unspecified seed values. Saving outside validity may give D01 priority-2/4 UNAUTHENTICATED or assignment-expired FORBIDDEN. DST validation priority (structural priority 1 or related-value priority 7) is undefined. D09 checks every occurrence within 366 days of save now, so the seed clock 2026-09-14 already detects ambiguous 2026-11-01 and nonexistent 2027-03-14; no clock change is needed.
- Why it matters: Seed/IR92/D01 do not uniquely determine acceptance premises and expected error codes.
- Example Failure: A returns FORBIDDEN on customer-a signIn/save at 2026-03-07, failing the case. B checks DST first and returns VALIDATION. Neither clearly violates D01 wording.
- Required Fix: Explicitly override Membership validity in Given or retain a valid clock and use 366-day checks. State DST validation's D01 priority.
- Suggested Revision: Use the seed clock 2026-09-14T01:00Z and timezone=America/New_York. Save weekly Sunday 02:30 (nonexistent 2027-03-14) or Sunday 01:30 (ambiguous 2026-11-01)→VALIDATION at D01 priority 1. Align AT-FIX-019.

#### G1-011

- Severity: MAJOR
- Issue ID: G1-011
- Document: 04-agentic-sdlc/acceptance-review-019.csv, 02-design/review-resolution-contracts.md (IR69/IR89), 02-design/deterministic-contracts.md (D06), 04-agentic-sdlc/fixture-contract.json
- Location: acceptance-review-019.csv:16 (AT-REV19-015), :3 (AT-REV19-002), :38 (AT-REV19-037) / review-resolution-contracts.md:438, 629, 631 / deterministic-contracts.md:93 / fixture-contract.json:1501-1504 (job-contractor-a.scheduledSlot.endAt=2026-09-20T00:00Z), :1533-1536 (assignment-contractor-a scheduled/validFrom/validUntil)
- Problem: AT-REV19-015 patches only assignment-contractor-a scheduledEnd=01:20Z, then expects a work-window-ended/reassignment-needed message for HQ/contractors at 01:20Z. IR89 derives this from JobSummary/JobDetail status assigned/in_progress and scheduledSlot.endAt<=now. Under IR69/IR92, unmentioned Job.scheduledSlot stays at seed endAt=2026-09-20T00:00Z, so the condition fails. The patch breaks IR89's jobs.assign synchronization invariant. D06 also equates Assignment validFrom/Until to scheduledStart/End, but validUntil remains 2026-09-20, making technician results depend on which field is used. AT-REV19-002/037 likewise patch only scheduledStart/End. AT-REV19-037's absence of the message after 02:00Z does not test synchronization because it was already absent before the patch.
- Why it matters: The acceptance case for user-confirmed DEC-50 (IR72 priority 1) cannot reach its expected result when premises follow the rules.
- Example Failure: Patching Given as written discards the technician screen and shows history at 01:20Z, but HQ/contractors see no IR89 message, failing AT-REV19-015.
- Required Fix: Patch Job.scheduledSlot and Assignment.validFrom/validUntil consistently in AT-REV19-002/015/037, or use normal jobs.assign. Add related-field patch invariants to IR92.
- Suggested Revision: AT-REV19-015 Given: Assignment scheduledEnd=validUntil=01:20Z; Job scheduledSlot.endAt=01:20Z and status=in_progress. AT-REV19-037 must also confirm the message appears at 02:00Z after the old window ends, before reassignment.

#### G1-012

- Severity: MAJOR
- Issue ID: G1-012
- Document: 04-agentic-sdlc/acceptance-review-017.csv, 04-agentic-sdlc/acceptance-review-018.csv, 04-agentic-sdlc/verification.md, 02-design/review-resolution-contracts.md (IR47/IR57/IR74), 02-design/deterministic-contracts.md (D05/D14), 04-agentic-sdlc/fixture-contract.json
- Location: acceptance-review-017.csv:5 (AT-REV17-004), :6 (AT-REV17-005), :15 (AT-REV17-014) / acceptance-review-018.csv:4 (AT-REV18-003), :14 (AT-REV18-013④) / verification.md:209-219 / review-resolution-contracts.md:329 (IR47), :390 (IR57), :513 (IR74 REV18-035) / deterministic-contracts.md:87 (D05), :192 (D14) / strict-review-contracts.md:133 (SR20) / client.md:246 (AT-C09-N) / fixture-contract.json:1006-1007 (device-tamper.unitId=unit-non-rto), :1318-1324 (contract-general-a, endAt=2027-01-01)
- Problem: verification.md requires AT-REV17-001–015 and AT-REV18-001–048 with existing ATs, but four expectations conflict with current rules/seed. (a) AT-REV17-014 uses SCR-X-not-found then role home for /customer/units/unit-other-customer; IR57 and AT-REV18-013④ require in-place not-found with URL retained and parent-list link for a known route's primary NOT_FOUND. (b) AT-REV17-004 expects dueAt=2026-09-15 12:00Z from a 10:00–12:00 window without Z, but IR74 treats it as Asia/Kuala_Lumpur and AT-C09-N maps it to 02:00–04:00Z, so dueAt is 04:00Z. (c) AT-REV17-005 assumes unit-non-rto archives successfully, but current device-tamper binding and contract-general-a ending 2027-01-01 make D05/D14 return CONFLICT. (d) AT-REV18-003 expects tamper alone to allow Command after communication_lost→restored→power_lost→tamper. SR20 restored recovers only connection; power is not restored after power_lost, so IR47 powerSignal=off gives OFFLINE first.
- Why it matters: IR72 requires reporting conflicts as document defects and stopping G1, not choosing during implementation. Old acceptance plans remain in the current manifest as Test Agent input. IR81 old-text checks cover Markdown/verification.md but not CSV expectations.
- Example Failure: Following IR57 fails AT-REV17-014; following that AT fails AT-REV18-013④. AT-REV17-005 archive fails with CONFLICT on unchanged seed, preventing later steps.
- Required Fix: Align these four premises/expectations with current rules, or explicitly mark replaced cases excluded in verification.md. Include acceptance CSV expectations in old-text or consistency checks.
- Suggested Revision: (a) customer-a at /customer/units/unit-other-customer stays on SCR-C03 with URL-preserving not-found and parent-list link; only undefined routes use SCR-X-not-found. (b) Omitted dueAt=2026-09-15T04:00Z. (c) Unbind device-tamper and patch contract-general-a endAt into the past, or use a new Unit without dependencies. (d) Use …→power_lost→restored(power)→tamper→commands.create.

### 3.2 MINOR

#### G1-013

- Severity: MINOR
- Issue ID: G1-013
- Document: 02-design/service-contracts.ts, 02-design/review-resolution-contracts.md (IR03/IR35), 02-design/operation-catalog.csv
- Location: service-contracts.ts:84 (Restriction), :86 (RestrictionReleaseView) / review-resolution-contracts.md:26, 236 / operation-catalog.csv authorization column for restrictions.reconcile/retry (`release-intent-or-terminal-recovery-only`)
- Problem: IR35 stores releaseIntent={source,at,actorMembershipId} on release request. IR03/catalog uses whether this intent came from override to allow override-only reconcile/retry. Canonical Restriction/RestrictionReleaseView lacks releaseIntent, with no declaration that it is internal-only like IR31 contributorUserIds.
- Why it matters: The override-only A10 UI cannot decide reconcile/retry button availability from retrieved values.
- Example Failure: An override-only user sees retry for a payment-origin release_requested restriction and gets FORBIDDEN on pressing it.
- Required Fix: Declare releaseIntent Repository-internal or add releaseIntentSource to RestrictionReleaseView.
- Suggested Revision: Add releaseIntent:{source:'payment'|'exception'|'override'|'manual';at:Instant}|null to canonical types and include it in RestrictionReleaseView.

#### G1-014

- Severity: MINOR
- Issue ID: G1-014
- Document: 03-uiux/screen-catalog.csv, 02-design/review-resolution-contracts.md (IR51), 02-design/client.md (DD-C08)
- Location: screen-catalog.csv:8 (SCR-C08 operations=alerts.list;notifications.markRead;notifications.list) / review-resolution-contracts.md:355 / Design client.md:298-303 / Requirements client.md:229 (AT-C08-B①–③)
- Problem: IR51 says FR-C08/AT-C08-B unresolved count means alertCount, displayed separately from unread notifications. SCR-C08 has no summaries.get operation and DD-C08 lacks alertCount. D07 requires KPI aggregation through summaries.get, not calculation from list pages.
- Why it matters: It is unclear whether AT-C08-B observes C08 or C01.
- Example Failure: A counts alerts.list items on C08, violating D07. B omits the count from C08 and observes C01 instead.
- Required Fix: Specify the display screen; add summaries.get to SCR-C08 if it displays the count.
- Suggested Revision: Add summaries.get(kind=customer) to SCR-C08 operations/secondary_queries and an alertCount (IR51) read field to DD-C08.

#### G1-015

- Severity: MINOR
- Issue ID: G1-015
- Document: 01-requirements/client.md, 02-design/service-contracts.ts
- Location: Requirements client.md:310 (AT-C12-E①“offline units are pending”) / service-contracts.ts:83 (releaseState enum)
- Problem: pending is not in RestrictionUnit.releaseState (none/waiting_reconcile/requested/released/not_required/failed). unit-limited loses communication after remove Command creation, so it is requested before 30 seconds and failed afterward. The value/display called pending is undefined.
- Why it matters: The observed value is ambiguous.
- Example Failure: The Test Agent cannot decide whether releaseState=requested or failed passes.
- Required Fix: Specify observation time and expected releaseState/pendingReason/display label.
- Suggested Revision: Before 30 seconds: perUnit.releaseState=requested, display “Waiting for release response (communication lost).” After 30 seconds: failed, with aggregate release_requested unchanged.

#### G1-016

- Severity: MINOR
- Issue ID: G1-016
- Document: 03-uiux/UIUXSpecification.md (UX-05), 03-uiux/component-contracts.csv
- Location: UIUXSpecification.md:142 (StatusBadge: domain/status/labelKey), :146 (CommandPanel: capability/observedState/pendingCommand/permission), :147 (ConfirmActionDialog: target/action/impact/reason/onConfirm) / component-contracts.csv:2 (StatusBadge: status,severity,labelKey,quality), :4 (CommandPanel: unit,pending,permission,loading,error), :5 (ConfirmActionDialog: open,target,before,after,reasonRequired,submitting,error)
- Problem: Shared Component prop names and shapes differ between UX-05 and Component contracts. IR72 gives contract CSV priority 5, but UX-05 has not been replaced.
- Why it matters: Agents see two prop sets and face a document defect under IR72.
- Example Failure: Implementations mix StatusBadge domain with severity/quality props.
- Required Fix: Replace UX-05 prop descriptions with references to component-contracts.csv or align the names.
- Suggested Revision: Change UX-05 “Information received” to “props in component-contracts.csv (IR72 priority 5).”

#### G1-017

- Severity: MINOR
- Issue ID: G1-017
- Document: 03-uiux/screen-catalog.csv, 02-design/contractor.md (DD-P03), 02-design/deterministic-contracts.md (D10), 02-design/strict-review-contracts.md (SR11)
- Location: screen-catalog.csv:15 (SCR-P03 url_selection=tab,sort) / Design contractor.md:122 (jobId required) / deterministic-contracts.md:145 / strict-review-contracts.md:80
- Problem: DD-P03 requires target jobId, but SCR-P03 URL keys omit it. D10 fixes selection IDs/tabs in the catalog and SR11 removes unknown keys, preventing targeted navigation from P02 and Back restoration.
- Why it matters: AT-P03-N navigation to assignment for an accepted job is undefined.
- Example Failure: /partner/schedule?jobId=job-internal-a loses jobId, forcing users to select again.
- Required Fix: Add jobId to url_selection, or explain why it is not retained and how selection works.
- Suggested Revision: Set SCR-P03 url_selection to tab,sort,jobId.

#### G1-018

- Severity: MINOR
- Issue ID: G1-018
- Document: 02-design/deterministic-contracts.md (D06), 02-design/review-resolution-contracts.md (IR49/IR89), 04-agentic-sdlc/acceptance-review-019.csv
- Location: deterministic-contracts.md:93 / review-resolution-contracts.md:345, 631 / acceptance-review-019.csv:38 (AT-REV19-037)
- Problem: D06 overlap checking (existingStart<newEnd AND newStart<existingEnd) does not say whether to exclude the old Assignment being replaced by extension/reassignment. AT-REV19-037 expects a new [01:30Z,04:00Z) window to succeed despite overlapping the same technician's old [00:00Z,02:00Z) window.
- Why it matters: Without exclusion, extension returns CONFLICT.
- Example Failure: jobs.assign conflicts with the old Assignment, failing AT-REV19-037.
- Required Fix: Explicitly exclude the replaced Assignment for the same Job from overlap checks.
- Suggested Revision: Add to D06: Exclude the current active Assignment with the same jobId (the replacement target) from overlap checks.

#### G1-019

- Severity: MINOR
- Issue ID: G1-019
- Document: 02-design/review-resolution-contracts.md (IR91/IR92/IR31), 01-requirements/admin.md, 01-requirements/client.md, 04-agentic-sdlc/fixture-contract.json
- Location: review-resolution-contracts.md:681 (IR92 rule 4“patch only the relevant fields”), :669 (IR91 rule 10), :195 (IR31) / Requirements admin.md:276 (AT-A10-B①) / Requirements client.md:311 (AT-C12-B) / fixture-contract.json `acceptancePatches["AT-P01-N"]` (change only job-contractor-a status=submitted)
- Problem: Patching only state creates combinations unreachable through normal operations. Setting restriction-limited-a to scheduled leaves perUnit.applyState=applied, an acknowledged apply Command, and unit-limited.observedRestriction. Cancel side effects (D03 release evaluation, SR26 recovery cases) are undefined. AT-P01-N job-contractor-a is submitted without a report version (reportRefs=[]) or IR31 contributor set.
- Why it matters: Unspecified side effects differ, and other screens sharing the fixture, such as P05, may throw errors.
- Example Failure: After AT-A10-B① cancellation, unit-limited still observes a restriction but effectiveControlPolicy becomes unrestricted; another test exposes the inconsistency.
- Required Fix: Define related fields that state patches must align, or build these cases through normal operations.
- Suggested Revision: Add IR92 rule 4: Restriction.state patches must simultaneously align perUnit/Command/observedRestriction (scheduled: perUnit=not_sent, no Command, observedRestriction=null, etc.). Submitted Jobs must also have a submitted report version.

#### G1-020

- Severity: MINOR
- Issue ID: G1-020
- Document: 01-requirements/admin.md (FR-A14), 02-design/service-contracts.ts, 02-design/review-resolution-contracts.md (IR88)
- Location: Requirements admin.md:362 (BR-A14“Factor requires region, year, unit, and source”), :369 (AT-A14-E“Factor unit does not match”) / service-contracts.ts:112 (EmissionFactor has no unit field) / review-resolution-contracts.md:617 (factorUnit has fixed display)
- Problem: EmissionFactor has no unit field and is fixed to kgCO2ePerKWh, so a mismatched-factor-unit state cannot be created and BR-A14 required unit has no validation target.
- Why it matters: The AT-A14-E subcase cannot run.
- Example Failure: The Test Agent cannot create a unit-mismatch fixture.
- Required Fix: Remove the subcase or add a factor unit field.
- Suggested Revision: Remove mismatched factor unit from AT-A14-E and state in BR-A14 that the unit is fixed to kgCO₂e/kWh (IR88).

#### G1-021

- Severity: MINOR
- Issue ID: G1-021
- Document: 01-requirements/contractor.md (FR-P05), 02-design/technician.md (DD-T04/T09), 02-design/service-contracts.ts
- Location: Requirements contractor.md:161 (AT-P05-B③) / Design technician.md:148, 289 / service-contracts.ts:70 (ReviewAvailability reasons)
- Problem: A report with not_inspected and no reason returns VALIDATION on submission, so it cannot become submitted or reach quality review. demoSeed also has no report section. ReviewAvailability has no incomplete-record reason.
- Why it matters: The premise is unreachable and the expected disabled reason does not exist.
- Example Failure: The Test Agent cannot build AT-P05-B③ Given.
- Required Fix: Remove the subcase or replace it with a defect discovered after submission.
- Suggested Revision: Move AT-P05-B③ to FR-T as submit with reasonless not_inspected→VALIDATION, matching T04-E③.

#### G1-022

- Severity: MINOR
- Issue ID: G1-022
- Document: 01-requirements/client.md (FR-C05), 02-design/service-contracts.ts
- Location: Requirements client.md:152 (BR-C05“Consent for general use”), :160 (AT-C05-B①“General consent only”) / service-contracts.ts:99 (Consent.purpose only allows `location_automation`)
- Problem: No data or operation represents general consent.
- Why it matters: AT-C05-B① Given cannot be built according to the rules.
- Example Failure: The Test Agent invents a purpose value.
- Required Fix: Model general consent or reword as no location consent.
- Suggested Revision: Use location consent granted=false; general use has no consent record in AT-C05-B①.

#### G1-023

- Severity: MINOR
- Issue ID: G1-023
- Document: 01-requirements/technician.md (FR-T12), 02-design/service-contracts.ts, 02-design/strict-review-contracts.md (SR20)
- Location: Requirements technician.md:309 (AT-T12-E②“Old heartbeat after new communication loss”) / service-contracts.ts:142 (DemoTrigger device kind: communication_lost/power_lost/tamper/restored) / strict-review-contracts.md:133
- Problem: There is no heartbeat DemoTrigger and no specified way to reproduce an old heartbeat, such as restored with an old sequence.
- Why it matters: The procedure is not deterministic.
- Example Failure: The Test Agent sends a nonexistent eventType and gets VALIDATION.
- Required Fix: Specify the DemoTrigger and values.
- Suggested Revision: Use communication_lost(sequence=5), then restored(axis=connection,sequence=4)→CONFLICT or no effect, remaining offline.

#### G1-024

- Severity: MINOR
- Issue ID: G1-024
- Document: 01-requirements/client.md, 01-requirements/admin.md, 01-requirements/technician.md, 02-design/deterministic-contracts.md (D05/D13), 02-design/service-contracts.ts
- Location: Requirements client.md:340 (AT-C13-N: purpose unspecified), :246 (AT-C09-N: assignee, window, and clock progression unspecified) / Requirements admin.md:399 (AT-A15-N: customerId/purpose/period/unitIds unspecified) / Requirements technician.md:258 (AT-T10-N: test-run startAction unspecified; no step to await preceding Command ack), :283 (AT-T11-N: check success event and sensorTypes unspecified) / deterministic-contracts.md:81, 177 / service-contracts.ts:217
- Problem: Required inputs/steps that do not affect expected values are absent from When.
- Why it matters: This requires guessed completion prohibited by IR92.
- Example Failure: Starting a test run before the preceding set_mode acknowledgment in AT-T10-N returns CONFLICT under D05.
- Required Fix: State required inputs and steps.
- Suggested Revision: For example, add to AT-T10-N: after set_mode is acknowledged, startAction=set_power true/endAction=set_power false. Add to AT-C13-N: purpose=Demo offset, period/unitIds follow IR92 rule 7.

#### G1-025

- Severity: MINOR
- Issue ID: G1-025
- Document: 01-requirements/common.md (AT-X01–X07), 04-agentic-sdlc/fixture-contract.json
- Location: 01-requirements/common.md:88, 97, 104, 113, 120, 127, 134 / fixture-contract.json demoSeed.capabilities (both support temperature and have the same mode options)
- Problem: Common ATs only list checks in prose, without concrete Given/When/Then values or acceptancePatches keys. For example, AT-X06 models without temperature support, with different mode options, or with fan-only support do not exist in seed or IR92 rule 6.
- Why it matters: The Test Agent must invent fixtures for seven common AT bundles.
- Example Failure: Unsupported models in AT-X06 differ between tests.
- Required Fix: Turn AT-X01–X07 into N/E/B tables and add necessary acceptancePatches.
- Suggested Revision: Add capabilities such as temperature=null, modes=[cool], control=false to acceptancePatches[AT-X06-B].

#### G1-026

- Severity: MINOR
- Issue ID: G1-026
- Document: 04-agentic-sdlc/verification.md (S08), 02-design/strict-review-contracts.md (SR03/SR13), 04-agentic-sdlc/fixture-contract.json
- Location: verification.md:104 / strict-review-contracts.md:24, 88 / fixture-contract.json actors (tech-external-b scopes=[unit-other-customer])
- Problem: S08 reoffers to another contractor (contractor-b) after contractor-a declines and assigns its technician, but no job/unit is specified. IR92 default unit-online-rto is outside contractor-b technician tech-external-b's scope. Whether jobs.assign/members.eligible checks technician unit scope is also undefined.
- Why it matters: S08 Given cannot be built uniquely.
- Example Failure: Assignment succeeds but tech-external-b cannot read the unit or start work.
- Required Fix: Specify job/contractor/technician IDs in S08 and define assignment scope checks.
- Suggested Revision: Use a customer-b job on unit-other-customer; decline by contractor-a; reoffer to contractor-b; technician tech-external-b.

#### G1-027

- Severity: MINOR
- Issue ID: G1-027
- Document: 01-requirements/client.md, 01-requirements/technician.md, 01-requirements/admin.md, 04-agentic-sdlc/fixture-contract.json, 02-design/service-contracts.ts
- Location: Requirements client.md:214 (AT-C08-SRC) / Requirements technician.md:188 (AT-T07-SRC) / Requirements admin.md:148 (AT-A05-SRC) / fixture-contract.json demoSeed.alerts (only one window_open record) / service-contracts.ts:142 (no DemoTrigger creates inferred/inspection Alerts)
- Problem: Fixtures for insulation-loss inspection records and no evidence have no IDs, values, or acceptancePatches keys. The clickable demo also has no path to create insulation_loss/inspection Alerts.
- Why it matters: SRC premises vary by test, and the demo cannot show company requirement BIZ-17.
- Example Failure: Two tests use different evidenceText and pass the same case.
- Required Fix: Define three SRC acceptancePatches and a demo generation path.
- Suggested Revision: Add alert-insulation-a with causeCode=insulation_loss/evidenceKind=inspection and alert-unknown-a with causeCode=unknown/evidenceKind=demo_observation to acceptancePatches[AT-C08-SRC].

#### G1-028

- Severity: MINOR
- Issue ID: G1-028
- Document: 02-design/common.md §2, 03-uiux/screen-catalog.csv, 03-uiux/component-contracts.csv, 01-requirements/common.md (FR-X01)
- Location: 02-design/common.md:58 / component-contracts.csv:59 (AppShell event: switchLocale;signOut;openNotifications;toggleVoice;extendSession) / screen-catalog.csv (no Screen includes demoSession.switchMembership) / 01-requirements/common.md:21
- Problem: The Component/Screen contract for the dedicated role-switching menu (FR-X01, P0) lacks demoSession.switchMembership.
- Why it matters: D13 says Page reads/writes use screen-catalog operations, so the call location must be guessed.
- Example Failure: One implementation switches roles by returning to /login; another uses the header menu.
- Required Fix: Add switchMembership to AppShell/ShellContainer contracts.
- Suggested Revision: Add switchMembership(demoMembershipId) to AppShell events and state in api_dependency that ShellContainer runs demoSession.switchMembership.

#### G1-029

- Severity: MINOR
- Issue ID: G1-029
- Document: 01-requirements/admin.md (FR-A09), 02-design/review-resolution-contracts.md (IR05)
- Location: Requirements admin.md:250 (AT-A09-E③“No notice sent”) / review-resolution-contracts.md:36
- Problem: IR05 always creates advance notices within the schedule transition, and zero recipients causes VALIDATION. A restriction with no notice therefore cannot exist.
- Why it matters: The subcase premise is unreachable.
- Example Failure: The Test Agent cannot create an unnotified restriction.
- Required Fix: Rewrite to the intended condition, such as less than 24 hours after notice.
- Suggested Revision: Use execute less than 24 hours after notice→D01 priority-6 CONFLICT in AT-A09-E③, and resolve overlap with B①.

### 3.3 QUESTION

#### G1-030

- Severity: QUESTION
- Issue ID: G1-030
- Document: 01-requirements/contractor.md (FR-P06), 02-design/query-catalog.csv, 02-design/contractor.md (DD-P06)
- Location: Requirements contractor.md:184 (AT-P06-N“Two technicians”) / query-catalog.csv members.capacity/members.list (filters: organizationId,qualification,activeOnly) / Design contractor.md:206-221 / fixture-contract.json actors (contractor-a has role=contractor in org-contractor-a)
- Problem: It is not stated whether members.list/members.capacity includes role=contractor Memberships in the organization, including contractor-a itself. AT-P06-N expects only two technicians.
- Why it matters: Expected counts differ.
- Example Failure: The implementation includes contractor-a and returns three members.
- Required Fix: State whether results are limited to role=technician.
- Suggested Revision: Add to the IR42 contractor projection: members.list/eligible/capacity returns only role=technician Memberships.

#### G1-031

- Severity: QUESTION
- Issue ID: G1-031
- Document: 02-design/review-resolution-contracts.md (IR45/IR85), 01-requirements/technician.md, 01-requirements/client.md, 04-agentic-sdlc/verification.md
- Location: review-resolution-contracts.md:308, 595 / Requirements technician.md:119-121 (AT-T03-N/E/B) / Requirements client.md:120, 202-204 (AT-C03-N, AT-C07-N/B) / verification.md:40
- Problem: IR45 requires simulator=false for cases fixing measurements/observation times in Given; IR85 explicitly lists only KPI/count ATs. There is no per-case list saying whether ATs without acceptancePatches qualify, such as AT-T03-B 120 seconds/120 seconds+1ms, AT-T03-N sequence=4 input, or AT-C07-B CO₂/humidity values.
- Why it matters: When a case advances past a minute boundary, simulator copies change stale/sequence expectations.
- Example Failure: AT-T03-B①② creates a copy at 01:01:00Z, keeping data valid even at 120 seconds+1ms.
- Required Fix: Specify simulator state per case or define a general rule: even ATs without acceptancePatches observing measurements/time/sequence start with simulator=false.
- Suggested Revision: Add IR92 rule 9: ATs observing measurements, observation times, stale, or sequence start with simulator=false. Only auto-generation cases (AT-REV18-001, AT-REV19-003/009) use enabled=true.

## 4. Reviewed and Unreviewed Scope

### 4.1 Reviewed Scope

- Read in full: docs/README.md, 04-agentic-sdlc/README.md, 00-prepare/PrepareDocument.md, 01-requirements/{common,client,contractor,technician,admin}.md, 02-design/{common,deterministic-contracts,strict-review-contracts,review-resolution-contracts,implementation-contracts,client,contractor,technician,admin}.md, 02-design/service-contracts.ts, 03-uiux/UIUXSpecification.md, 04-agentic-sdlc/verification.md.
- fixture-contract.json: Expanded and checked actors, energy, reviewResolution, all demoSeed sections, and all 20 acceptancePatches keys by script. Recalculated key AT numbers: AT-A01-N, AT-C06-N/E.2–E.4, AT-REV19-004/011/041, AT-P01-N, weekdays in AT-P06-B and AT-C04-N.
- Acceptance CSVs: Closely reviewed all 42 acceptance-review-019 cases. Printed all rows of acceptance-review-018/017/016, acceptance-fixes, acceptance-independent, acceptance-strict-review, acceptance-rereview, acceptance-resolution, acceptance-convergence, acceptance-loop, acceptance-projection, and acceptance-independent-g1; scanned for current IR/seed conflicts. Checked all CSV column counts and execution_status values by script.
- Scripted cross-checks: canonical OperationContracts (137) versus operation-catalog input/result/mode; writes (76) versus version catalog; Query-input reads (35) versus Query catalog; screen operations/primary/secondary names and containment; Component names; IR71 invalidation-table operation names; all authorization rows; all Query filter/sort/mapping rows; all version rows; all screen url_selection/states/tabs rows; all component-contracts.csv rows.
- Decision registers: Each DEC status in review-decisions-016–019.json (DEC-44/50 accepted).
- Validation: Ran both validators, recalculated manifest hashes/baseline ID, and TypeScript strict checked a scratchpad copy of service-contracts.ts.

### 4.2 Unreviewed Scope and Limits

- Did not read all body text of 00-prepare/sources/* (original-handover.md, company-requirements-original.txt, production-instructions.md, reference-style-evidence.json), reference-design-analysis.md, internal/design-assumptions.md, or decision-record-2026-09-16.md; only headings and cited passages. Did not revalidate company-source→BIZ→FR mappings.
- Did not compare company-requirement-map.csv, requirement-origins.csv, or traceability.csv row by row; relied on validator counts.
- Did not read agents/*.md or templates/artifacts.md.
- Did not review validator source code, only execution results. Inferred unchecked areas from the scope stated in IR81.
- Did not compare UI token values with reference-site extraction evidence or recalculate contrast ratios.
- Did not compare every screen's entry/exit/interaction/data_contract/query_trigger fields or every Component's props against DTO types; sampled only.
- Acceptance-value recalculation was limited to the samples above. Did not manually execute every subcase of all 182 bundles and older plans.
- Application implementation does not exist, so application/E2E/accessibility tests were not run (not_run).
- Did not consult old records (runs/DOC-0.17.0 and earlier, 05-document-review). Read only headers of the correction author's DOC-0.19.0 review/gate records after the decision, not as evidence for it.

## 5. G1 Decision

**FAIL**

G1 requires zero BLOCKER/CRITICAL/MAJOR findings and both validators passing. Both validators passed (exit 0; all 62 mutations detected), but 12 MAJOR findings remain (G1-001–012): role-based write-authorization conflict (001); missing business-notification/allergen/air-guidance data or display rules (002/006/007); restriction-cancellation transition conflict (003); undefined required report-submission set (008); and acceptance data conflicting with higher-priority rules/seed or not uniquely constructible under IR92 (004/005/009–012). Implementation and test Agents cannot proceed without questions or guesses.

There are zero BLOCKER/CRITICAL findings, and mechanical consistency across baseline, manifest, and catalogs is maintained. Correct the 12 MAJOR findings, create a new baseline, and have another reviewer reassess. Carry forward the 17 MINOR findings and two QUESTION items as records.
