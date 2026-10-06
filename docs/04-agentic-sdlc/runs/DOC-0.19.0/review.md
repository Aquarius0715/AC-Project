# Independent Review Report INDEPENDENT-DOC-0.18.0-REV19 (2026-09-17)

Corrected baseline: `048ffcef25f8463bca33346f5bb1dd8dbb1fdad03b61132d1cfc5d17ebeb1ac2` ([spec-manifest](spec-manifest.json)).

Scope: The DOC-0.18.0 working tree, interrupted after REV18 corrections, uncommitted and without a baseline. Documents: PrepareDocument; five requirements documents; five detailed designs plus four contracts, four catalogs, and canonical types; UIUX specification plus two catalogs; and Agentic SDLC supporting materials (fixtures, validation plan, validators). Reviewer: the AI in this conversation (Opus 5), in a separate session from the REV18 correction author, so its DOC-0.18.0 assessment is an independent review. The same reviewer also made corrections (IR75–93, DEC-42–53), so the follow-up is **self-review**, not independent G1 for DOC-0.19.0 (gate-G1.yaml=pending; another Agent has been asked). Application implementation/runtime tests are not_run. Production HTTP/authentication/DB/real devices are outside 1A under DEC-12/D11, so they are not counted as blockers here; §11 lists them as decisions required before 1B.

## 1. Executive Review Summary

Initial assessment: **NOT READY (1 BLOCKER, 3 CRITICAL, 16 MAJOR, 20 MINOR, 2 QUESTION)**. These comprise 36 initial findings, REV19-037 found in the first post-correction self-review, and REV19-038–042 found in the user-requested full acceptance-premise check (round 3). Final self-review: zero unresolved findings, all 64 requirements OK, both validate_documents.py and check_review_regressions.py successful, and service-contracts.ts TypeScript strict passed. DEC-44 (estimated savings, option A) and DEC-50 (work-window end handling) were confirmed by the user on 2026-09-17. Independent G1 awaits another reviewer.

The four most important findings were:

1. **REV19-001 (BLOCKER): No handover baseline.** The indexed runs/DOC-0.18.0 did not exist, one acceptance-plan CSV row was broken, and both validators failed.
2. **REV19-002 (CRITICAL): Technician screens before work-window start.** IR49 read-only display conflicted with IR57 full-screen permission-denied on primary-query FORBIDDEN and the units.get primary query in SCR-T04/T10.
3. **REV19-003 (CRITICAL): The heartbeat simulator converts estimated/bad values to measured values.** This breaks FR-X03 (P0) distinctions and SR27 operation classification.
4. **REV19-004 (CRITICAL): Admin dashboard estimated savings are effectively never shown.** IR63 conditions conflict with variable-length SR17 presets, and display wording conflicts with IR68.

The 16 MAJOR findings cover conflicting extension rules, C06 negative-value display, gaps in the old-text detector, post-login return destinations, refetch display, initial consent records, KPI Given/seed mismatch, expired-Offer responses, reason-field lengths, MRV display fields, unsaved input at work-window end, demoSeed normalization, and four full-check findings: Given/seed conflicts (038), expectation/authorization/state conflicts (039), missing emission-factor seed (040), and energy acceptance data (041). All were settled in IR75–93.

## 2. BLOCKER Issues

### REV19-001

- Issue ID: REV19-001
- Severity: BLOCKER
- Document: docs/README.md, 04-agentic-sdlc/README.md §8, acceptance-review-018.csv, tools/validate_documents.py, runs/
- Location: Current baseline and handover records
- Problem: runs/DOC-0.18.0, referenced as current input by README and SDLC §8, does not exist (spec-manifest.json/review.md/traceability-matrix.csv/gate-G1.yaml). Unquoted commas break the columns on line 15 of acceptance-review-018.csv (AT-REV18-014). validate_documents.py reports 10 failures; check_review_regressions.py fails its prerequisite validation. The 0.18.0 corrections were interrupted before self-review.
- Why it matters: IR73 requires both validators to pass and a manifest for handover. Without a manifest, the implementation Agent has no fixed input file set or spec_baseline_id, so test results cannot be tied to a specification version.
- Example Failure: The implementation Agent reads an unsettled working tree and implements text changed by later corrections. Test evidence cannot be traced to a baseline.
- Required Fix: Fix the CSV and record 0.18.0 truthfully as interrupted, unverified, and without a baseline. Pass both validators on the corrected version, create manifest and gate records, and point the index to the current version.
- Suggested Revision: IR75: Make DOC-0.19.0 the current baseline. Store review/findings/traceability/static-check/validator-negative-checks/gate-G1(pending)/completion/spec-manifest in runs/DOC-0.19.0. Store only REV18 findings and the interrupted/no-baseline record in runs/DOC-0.18.0.
- Status: resolved (IR75, AT-REV19-001)

## 3. CRITICAL Issues

### REV19-002

- Issue ID: REV19-002
- Severity: CRITICAL
- Document: review-resolution-contracts.md IR49/IR57, screen-catalog.csv SCR-T04/T10/T02/T07, SR04
- Location: Technician screens before the work window starts
- Problem: IR49 returns FORBIDDEN for units.get with jobId before the work window starts, while requiring read-only job display during the viewing window. SCR-T04/T10 has units.get as a primary query, and IR57 makes the whole screen permission-denied when a primary query is FORBIDDEN.
- Why it matters: In the core technician flow (notification→open job→prepare until start), the specification supports both a read-only job screen and full denial.
- Example Failure: T04 jobs.get succeeds but units.get returns FORBIDDEN, making the entire screen permission-denied and hiding the schedule/start time. Another implementation displays only the job, splitting test results.
- Required Fix: Define queries to call before work starts, the display state and content, and re-enabling at start time in one place.
- Suggested Revision: IR76: While scheduledStart>now, T04/T10 does not call units.get (query disabled); display the job read-only in work-not-started. On T02/T07, units.get FORBIDDEN(errors.assignment_not_started) shows work-not-started with start time and a job link, not permission-denied. Enable Queries when the one-second tick reaches start time.
- Status: resolved (IR76, DEC-42 (PROPOSED), AT-REV19-002)

### REV19-003

- Issue ID: REV19-003
- Severity: CRITICAL
- Document: review-resolution-contracts.md IR45, IR08, SR27, FR-X03, service-contracts.ts RawMeasurement
- Location: Copying by the heartbeat simulator
- Problem: IR45 copies the latest Measurement value/unit for each Sensor and saves it with origin=measured and quality=valid. Even estimated or suspect values become measured/valid after one minute. RawMeasurement.sequence is required for /demo input, but users cannot learn the sequence that increases every minute, so manual values can be ignored as old sequences.
- Why it matters: FR-X03 (P0) requires measured/estimated/quality distinctions; IR08 prohibits fallback from estimated or suspect values to old measured values. SR27 requires measured/valid for operation classification, so estimates can turn into running status.
- Example Failure: Injecting power=1.0kW with origin=estimated through /demo creates a measured/valid copy at the next minute boundary, changing the operation KPI from unknown to running. Manual sequence=2 is ignored because the simulator is at sequence=5.
- Required Fix: Define eligible source data, handling of Sensors that are not copied, and sequence assignment for demo input.
- Suggested Revision: IR77: Copy only Sensors whose latest Measurement is measured/valid with a non-null value. Generate nothing for others, allowing natural stale/unknown transitions. Make sequence optional for demo.trigger telemetry; when omitted, Repository assigns latest+1. /demo omits it by default.
- Status: resolved (IR77, DEC-43 (PROPOSED), AT-REV19-003)

### REV19-004

- Issue ID: REV19-004
- Severity: CRITICAL
- Document: FR-A01, DD-A01, IR63, SR17, IR68, AdminSummary, demoSeed
- Location: Estimated savings on the admin dashboard
- Problem: IR63 requires equal minute counts for automatic baseline selection. A01 today/7d/30d presets (SR17) end at the last completed minute and change length every minute, so matching baselines rarely exist. Seed has no baseline either. IR63 displays no match as “Baseline not set,” while IR68 displays the same null as “Cannot calculate.” IR72 number priority favors IR68, conflicting with AT-REV18-019.
- Why it matters: Estimated savings in FR-A01 (P0) are unavailable for almost the entire demo. Expected acceptance strings are also ambiguous.
- Example Failure: HQ opens /admin with today and always sees “Baseline not set.” AT-REV18-019 expects that wording, but an IR68 implementation shows “Cannot calculate” and fails.
- Required Fix: Define A01 forecast calculation, baseline selection, missing-data handling, and text for each null reason. Provide a default baseline in seed.
- Suggested Revision: IR78: Add AdminSummary.energyForecast. Select the latest demo_fixed baseline matching the exact target unit set and boundary ac_input_electricity. Return the difference between a forecast baseline prorated using baseline energy per unit-minute and a forecast actual value extrapolated from valid-slot averages. baseline_unavailable→“Baseline not set”; other null→“Cannot calculate.” A01 energySummary savings fields are null. Add an assumed baseline for all tenant-a units to seed.
- Status: resolved (IR78, DEC-44 (confirmed by user), AT-REV19-004)

## 4. MAJOR Issues

### REV19-005

- Issue ID: REV19-005
- Severity: MAJOR
- Document: review-resolution-contracts.md IR36
- Location: Session lifetime and extension
- Problem: IR36 still says users cannot extend the session, directly conflicting with the 30-minute demoSession.extend in IR55/D09/FR-X01. IR72 requires removing superseded text, but the validator excludes the correction-contract document.
- Why it matters: The session Agent may skip the extension dialog or reject extension based on IR36.
- Example Failure: SessionExpiryDialog is not implemented, failing AT-REV18-011.
- Required Fix: Replace the IR36 sentence with a reference to current IR55 and detect reintroduction.
- Suggested Revision: IR79: Replace the sentence with “User extension is available only through demoSession.extend in IR55.” Include the correction-contract document in validator checks.
- Status: resolved (IR79, AT-REV19-005)

### REV19-006

- Issue ID: REV19-006
- Severity: MAJOR
- Document: FR-C06 BR-C06/AT-C06-E, DD-C06 boundary conditions, IR68
- Location: Negative-savings display strings (C06)
- Problem: IR68 defines a common formatter such as “Increase 20.0%” for every role and explicitly includes C06. FR-C06 BR still says “20% increase,” AT-C06-E says “Increase20%,” and DD-C06 says “Increase rate20%.” REV18-024 corrected only A13.
- Why it matters: The acceptance oracle differs from the norm in three ways, leaving the Test Agent without a clear expected string.
- Example Failure: A C06 implementation showing “Increase 20.0%” fails the AT-C06-E string comparison.
- Required Fix: Align C06 text with IR68 display strings.
- Suggested Revision: IR80: Change FR-C06 BR, AT-C06-E, and DD-C06 to “Increase 20.0 kWh / Increase 20.0%” (DTO=-20); register old expressions in the validator.
- Status: resolved (IR80, AT-REV19-006)

### REV19-007

- Issue ID: REV19-007
- Severity: MAJOR
- Document: tools/validate_documents.py, tools/check_review_regressions.py, IR72
- Location: Detection of superseded text
- Problem: Old-text checks use exact strings only and exclude review-resolution-contracts.md. They miss the old IR36 sentence, DD-A04 permission wording (“can manage models” versus registered “manage models”), DD-A12 environment-policy permission wording, D09 exact display-name matching for room, DD-A01 customer-organization count, and C06 negative-value wording.
- Why it matters: IR72 relies on validate_documents.py detecting superseded phrases as a handover safeguard, but wording variants pass through. Static success is not evidence of consistency.
- Example Failure: A later change restores a slightly reworded old phrase and validation passes, so the implementation Agent follows old specifications.
- Required Fix: Include the correction-contract document; exclude only quoted-context lines containing replacement/old-text explanations. Register the found variants as regular expressions and add mutation tests.
- Suggested Revision: IR81: Use regular expressions for old-phrase checks and include review-resolution-contracts.md, excluding lines that explain replacement. Register these six cases and add mutations to check_review_regressions.py.
- Status: resolved (IR81, AT-REV19-007)

### REV19-008

- Issue ID: REV19-008
- Severity: MAJOR
- Document: screen-catalog.csv SCR-X-login, DDC-07 /login, D08 returnTo, SR11/IR50
- Location: Passing the destination for return after login
- Problem: FR-X01 returns unauthenticated users to the original allowed route after login. demoSession.signIn accepts returnTo, but passing it from the route guard to the login screen is undefined. SCR-X-login url_selection contains only tab. SR11/IR50 removes unknown URL keys.
- Why it matters: The guard Agent may pass a URL query that the login Agent removes as unknown. Deep links, including notification links, fail.
- Example Failure: Opening /customer/units/unit-online-rto while logged out redirects to /login?returnTo=…, but returnTo is removed and login ends at /customer.
- Required Fix: Define transfer method (URL key, encoding, invalid-value handling) and register it in the catalog.
- Suggested Revision: IR82: Guard replace-navigates to /login?returnTo=<encodeURIComponent(path+search)>. Add returnTo to SCR-X-login url_selection. Ignore and remove values failing D08 validation and go to role home, without a VALIDATION screen.
- Status: resolved (IR82, DEC-45 (PROPOSED), AT-REV19-008)

### REV19-009

- Issue ID: REV19-009
- Severity: MAJOR
- Document: D10/D13 screen states, DDC-03, IR71, IR45, UX-06
- Location: Display during refetch after invalidation
- Problem: States initial/loading/empty/success exist, but display during subscription-driven refetch of visible data (retain data or return to skeleton), refetch failure, and announcements are undefined. IR45 creates measurement/device events every minute, invalidating dashboard and unit-detail Queries.
- Why it matters: Screens may return to skeleton every minute, disrupting focus and announcements. UX-06's rule not to announce every telemetry update cannot be tested.
- Example Failure: The customer dashboard flashes and the screen reader announces “Loading” every minute.
- Required Fix: Define display during same-key refetch and failure, and how simultaneous invalidations are combined.
- Suggested Revision: IR83: Retain displayed data during same-key refetch with refreshing (aria-busy, no live region). On failure retain data with stale note, last-success time, and retry. Use loading only when the key changes. Combine event invalidations within one one-second tick into one per key.
- Status: resolved (IR83, DEC-46 (PROPOSED), AT-REV19-009)

### REV19-010

- Issue ID: REV19-010
- Severity: MAJOR
- Document: SR02, IR69, fixture-contract.json demoSeed.consents, FR-C05
- Location: Initial location-consent records
- Problem: SR02 says consents.get returns a versioned granted=false Consent from the first call. IR69 prohibits adding business records absent from demoSeed, whose consents array is empty. IR>SR priority prevents Repository from creating consent records, leaving consents.get undefined.
- Why it matters: Initial FR-C05 consent display and expectedVersion for consents.update are unsettled.
- Example Failure: One implementation returns NOT_FOUND and an error screen. Another silently creates version 1, shifting test version numbers.
- Required Fix: Define the source of initial consent records (seed/Membership creation).
- Suggested Revision: IR84: Put granted=false/version=1 records for every client Membership in demoSeed.consents. Create the same record in the same members.save transition when creating a client Membership. Missing consents.get records return NOT_FOUND (fixture defect).
- Status: resolved (IR84, DEC-47 (PROPOSED), AT-REV19-010)

### REV19-011

- Issue ID: REV19-011
- Severity: MAJOR
- Document: AT-A01-N/AT-A01-B, IR69, demoSeed
- Location: Mismatch between acceptance Given and demo seed
- Problem: AT-A01-N Given requires tenant-a with two power-ON, two OFF, and one unknown unit. The five-unit seed has ON2 (unit-online-rto/unit-limited), OFF1 (unit-non-rto), unknown2 (unit-offline-rto/unit-other-customer). IR69 says Given is a seed diff but does not specify which units to change or how.
- Why it matters: The Test Agent guesses the patch; selected units change utilization denominators and destination-list counts.
- Example Failure: Agent A turns unit-other-customer OFF for AT-A01-N; Agent B makes unit-offline-rto online. Destination lists differ.
- Required Fix: Fix Given as concrete patches and validate consistency with seed.
- Suggested Revision: IR85: Define acceptancePatches[AT-A01-N] in fixture-contract.json (device-offline-rto online, power measurement 0.0kW, observedState.power=false), referenced by AT-A01-N/B. KPI ATs start with simulator=false.
- Status: resolved (IR85, AT-REV19-011)

### REV19-012

- Issue ID: REV19-012
- Severity: MAJOR
- Document: D01, IR23, IR48, SR01, AT-P02-E①, AT-REV18-004
- Location: Responses to expired unanswered Offers
- Problem: After offerExpiresAt, jobs.accept/decline has three possible error codes: D01 assignment expiry gives FORBIDDEN (priority 4), out-of-scope gives NOT_FOUND (priority 3), and AT-P02-E①/AT-REV18-004 expect CONFLICT. jobs.get after expiry is also undefined, though lists exclude it.
- Why it matters: Error display and acceptance decisions vary for the same operation.
- Example Failure: One implementation returns NOT_FOUND and “Target not found,” failing AT-P02-E①, which expects CONFLICT and refetch guidance.
- Required Fix: Decide whether response expiry is a state condition, whether the company's own expired Offer remains referenceable, and what jobs.get returns.
- Suggested Revision: IR86: accept/decline of the company's own Offer remains in scope after expiry and returns D01 priority-6 CONFLICT with messageKey=errors.offer_expired. jobs.get returns NOT_FOUND after expiry (receipts only under IR01). Expiry FORBIDDEN applies only to viewing/work windows such as accessValidFrom/Until.
- Status: resolved (IR86, DEC-48 (PROPOSED), AT-REV19-012)

### REV19-013

- Issue ID: REV19-013
- Severity: MAJOR
- Document: D12, DD-T07 resolutionReason, DD-A14 reviewComment, DD-A10 reason, DD-P05 reason
- Location: Character limits for reason fields
- Problem: D12 sets reason/resolutionReason/reviewComment and similar fields to 1–1000 characters after trim, but DD-T07/DD-A14/DD-A10/DD-P05 return-for-correction reason use 1–2000. The scope of D12's “not specified in each DD” is also unclear.
- Why it matters: UI and Repository validation boundaries differ: UI accepts 1001–2000 characters but the service rejects them. Boundary-test expectations are unsettled.
- Example Failure: A 1500-character resolution reason passes the form but Repository returns VALIDATION.
- Required Fix: Set one limit for reason fields and clarify D12 scope.
- Suggested Revision: IR87: reason/cancelReason/declineReason/resolutionReason/reviewComment/changeReason/purpose are 1–1000 after trim regardless of DD. Text/notes/replies (symptom/workText/message/reply/note) follow each DD. Correct the four DDs to 1–1000.
- Status: resolved (IR87, DEC-49 (PROPOSED), AT-REV19-013)

### REV19-014

- Issue ID: REV19-014
- Severity: MAJOR
- Document: DD-A14 Scope 2 report preview, service-contracts.ts MRVPreview/MRVReport, D12
- Location: MRV display field names
- Problem: DD-A14 says to add reportCategory/siteIds/gridRegion/factorValue/factorUnit/factorYear/factorVersion/boundaryDescription/coverageRatio to report-screen data, but canonical types lack them and no mapping to existing fields is given. D12 prohibits turning conceptual names into separate DTO fields.
- Why it matters: UI expects new fields while Repository returns canonical types, leaving blank screen values (Data Model Gap).
- Example Failure: Reading gridRegion from the DTO gives undefined and leaves the Scope 2 factor region blank.
- Required Fix: Fix the source of each display value (canonical path or derivation) and state that no DTO fields are added.
- Suggested Revision: IR88 mapping: reportCategory→display scope=scope_2; siteIds→deduplicated ACUnit.propertyId for conditions.unitIds; gridRegion→factorSnapshot.region; factorValue→factorSnapshot.kgCO2ePerKWh; factorUnit→fixed kgCO₂e/kWh; factorYear→factorSnapshot.year; factorVersion→factorRef.version; boundaryDescription→conditions.boundary; coverageRatio→summary.coverage.
- Status: resolved (IR88, AT-REV19-014)

### REV19-015

- Issue ID: REV19-015
- Severity: MAJOR
- Document: IR49/IR24, FR-T09 BR-T09, D09, DD-T08/T09
- Location: Work-window end and unsaved drafts
- Problem: After scheduledEnd, technicians see only job history and IR24 discards visible Queries/details. FR-T09 says refetch must not erase unsaved edits. Handling dirty values at window end, advance warning, and ways for HQ/contractors to notice expired windows on in_progress jobs are undefined.
- Why it matters: Inspection input can be lost without warning at window end, unlike session expiry with IR55 warning. Screens also lack information needed to decide extension (jobs.assign).
- Example Failure: A technician starts a report at 11:55; at 12:00 the screen becomes history and unsaved text disappears. The contractor list still says in_progress without showing the need for reassignment.
- Required Fix: Define advance warning, handling/notification of unsaved values at end, and expired-window display conditions.
- Suggested Revision: IR89: Show one role=status warning 15 demo-clock minutes before end. At end discard unsaved values and notify, like D09 expiry; FR-T09 retention applies only to refetch within the window. HQ/contractor lists/details show “Work window ended; reassignment needed” when status is assigned/in_progress and scheduledSlot.endAt<=now, derived from existing DTOs.
- Status: resolved (IR89, DEC-50 (confirmed by user), AT-REV19-015)

### REV19-036

- Issue ID: REV19-036
- Severity: MAJOR
- Document: fixture-contract.json demoSeed, IR69, service-contracts.ts
- Location: Expansion of demoSeed into canonical DTOs
- Problem: IR69 makes demoSeed authoritative and requires canonical DTO schema validation at generation, but rows omit shared Entity fields (tenantId/version/createdAt/updatedAt) and required fields (Device.sensors/targetUnitId/createdByMembershipId, Measurement.eventId/isDemo, Command.correlationId, Alert.evidenceIds, Notification.params, MaintenanceJob.reportRefs/costs, etc.). Device has noncanonical sensorMetrics. Expansion rules and Sensor IDs are undefined.
- Why it matters: Agents fill seed gaps differently, changing Sensor IDs and versions. Acceptance patches referencing IDs such as sensor-offline-power may not match.
- Example Failure: An implementation generates sensor-online-rto-power, but AT-A01-N targets missing sensor-offline-power and causes a fixture exception.
- Required Fix: Define shorthand-to-canonical DTO completion rules in one place. Put guessed values such as Sensor IDs explicitly in seed.
- Suggested Revision: IR91: Define normalization of tenantId, version, timestamps, nulls/arrays, derived values, and resource defaults. Explicitly include Device sensors with IDs in seed and remove sensorMetrics. Normalize nonexistent acceptancePatches IDs as new rows.
- Status: resolved (IR91, AT-REV19-036)

### REV19-038

- Issue ID: REV19-038
- Severity: MAJOR
- Document: 01-requirements (AT-C02-N, AT-C08-N, AT-P01-N, AT-P03-N, AT-P03-R01, AT-P06-N, AT-T11-N, AT-A06-N), fixture-contract.json demoSeed, IR69
- Location: Acceptance Given versus demoSeed conflicts, found in the user-requested full check
- Problem: Some Given conditions conflict with demoSeed. AT-C02-N assumes zero properties but customer-a has two. AT-C08-N assumes three unread notifications but seed has two, one a restriction notice. AT-P01-N counts seeded assigned job-contractor-a, breaking expectations. AT-P03-N/AT-A06-N windows overlap assignment-contractor-a (2026-09-14–20), causing CONFLICT. AT-P03-R01/AT-P06-N assumes two contractor-a technicians but seed has one. AT-T11-N equipment already has a bound device, so bind returns CONFLICT.
- Why it matters: IR69 defines Given as seed diffs, but some expectations cannot be reached or their setup is ambiguous. Test Agents invent data and disagree.
- Example Failure: AT-P03-N executed as written conflicts with the seeded assignment and fails to create Assignment.
- Required Fix: Make every Given uniquely constructible from seed; define fixed patches if needed.
- Suggested Revision: IR92: Define Given interpretation rules and acceptancePatches (AT-C08-N, AT-P01-N, AT-P03-R01, AT-P06-N, AT-T11-N, etc.). Change AT-C02-N/P03-N/A06-N text to avoid overlap with existing data.
- Status: resolved (IR92, AT-REV19-038)

### REV19-039

- Issue ID: REV19-039
- Severity: MAJOR
- Document: AT-T08-N, AT-T08-E①, AT-T10-E①, D01, D06, IR24
- Location: Expected-result conflicts with authorization/state rules, found in the full check
- Problem: AT-T08-N has HQ return an outsourced report, but quality review belongs to the contracted partner.review actor; HQ is allowed only under hq_escalation (D06). AT-T08-E① expects CONFLICT for start in requested/cancelled/on_hold, but D01 first gives NOT_FOUND for unassigned jobs (priority 3) and FORBIDDEN for jobs whose assignment ended on cancellation (priority 4). AT-T10-E① tests below-restriction temperatures on unit-online-rto, which has no restriction.
- Why it matters: Acceptance oracles conflict with higher-priority rules, failing correct implementations.
- Example Failure: jobs.start on unassigned job-internal-a returns NOT_FOUND and fails AT-T08-E①'s CONFLICT expectation.
- Required Fix: Clarify error-code selection and the Given for assignment to a restricted unit.
- Suggested Revision: IR93: Define technician start/submission error-code tables and outsourced quality reviewers. Correct AT-T08-N/E and AT-T10-E① (acceptancePatches AT-T10-E.1).
- Status: resolved (IR93, AT-REV19-039)

### REV19-040

- Issue ID: REV19-040
- Severity: MAJOR
- Document: fixture-contract.json demoSeed, fixture.defaultEmissionFactorId, IR69, SR09, AT-C13-N, AT-A14-N, S05
- Location: Missing emission factor in demo seed, found in the full check
- Problem: D07/SR09 uses fixture.defaultEmissionFactorId (factor-demo-2026) for ordinary energy.summary, but demoSeed lacks factors and IR69 prohibits adding records absent from seed. The factor cannot be found, so emissions are null with factor_missing in qualityWarnings.
- Why it matters: AT-C13-N's actual 40kgCO₂e and MRV preview cannot work.
- Example Failure: C13 shows emissions as “Cannot calculate” and disables the offset application button.
- Required Fix: Put the default factor in seed as a business record.
- Suggested Revision: IR92: Add factor-demo-2026 (0.5 kgCO₂e/kWh) to demoSeed.factors and identify it as the default factor record.
- Status: resolved (IR92, AT-REV19-040)

### REV19-041

- Issue ID: REV19-041
- Severity: MAJOR
- Document: AT-C06-N/E, AT-C13-N, AT-A13-N/E/B, AT-A14-N, fixture.energy, D07
- Location: Energy acceptance data, found in the full check
- Problem: AT-C06-N expects actual 80kWh and 100% coverage over 2026-09-01–08 (seven calendar days), but fixture.energy defines [2026-09-14T00:00Z,01:00Z) with 80kW×60 slots. They do not match. No mechanism inserts seven days of valid minute slots, so 100% coverage cannot be built. Baseline registration is unspecified.
- Why it matters: Core energy-comparison/emission/MRV acceptance cases cannot run; Test Agents invent measurements.
- Example Failure: One test creates seven days of custom measurements for AT-C06-N while another tests one hour, producing different expectations.
- Required Fix: Align the test window with fixture.energy and uniquely create measurement series/baselines through patches.
- Suggested Revision: IR92: Define series-based measurement generation and shared:energy-actual-80 in acceptancePatches. Use the same window/units/baseline for C06/C13/A13/A14. Recalculate energy and coverage in the validator.
- Status: resolved (IR92, AT-REV19-041)

## 5. MINOR Issues

### REV19-016

- Issue ID: REV19-016
- Severity: MINOR
- Document: DD-A01 step 2, DD-A04 prerequisites, DD-A12 prerequisites, D09 voice grammar, verification.md S03, common.md §2
- Location: Remaining superseded text
- Problem: Remaining old text includes DD-A01 counting active customer organizations (IR40); DD-A04 alternative model-management permission and DD-A12 environment-policy permission (REV18-043); D09 exact display-name matching for room (IR65); S03 payment confirmation→request release (automatic under IR35); and common.md §2 returning home for /forbidden/undefined routes (IR57 shows links).
- Why it matters: This invites readings that conflict with higher-priority rules and violates IR72's requirement to remove superseded text.
- Example Failure: S03 E2E requires explicit release after payment confirmation and mistakes an idempotent response for the main path.
- Required Fix: Rewrite these passages as references to current contracts.
- Suggested Revision: IR90-1: Correct each sentence to IR40/device.manage/automation.policy.manage/Space.name/IR35 automatic transition/link display.
- Status: resolved (IR90, AT-REV19-016)

### REV19-017

- Issue ID: REV19-017
- Severity: MINOR
- Document: IR64, DD-C09 contactWindow
- Location: Contact-window input guidance
- Problem: Rejecting seven or more consecutive digits after removing spaces, hyphens, parentheses, and + also rejects times such as 0900-1800 as phone numbers. Input examples and guidance are undefined.
- Why it matters: Valid user input produces an unexplained error.
- Example Failure: A customer enters Mon-Fri 0900-1800 and sees only a prohibited-contact-details error.
- Required Fix: Retain the validation rule and define examples and rejection wording.
- Suggested Revision: IR90-2: Show an HH:mm example such as Weekdays 09:00-18:00. Include “Enter times in HH:mm format” in errors.contact_details_forbidden.
- Status: resolved (IR90, DEC-53 (PROPOSED), AT-REV19-017)

### REV19-018

- Issue ID: REV19-018
- Severity: MINOR
- Document: IR67, D03, D05
- Location: Device-operation start recheck and restriction Commands
- Problem: IR67 does not define failure of its exclusivity recheck at the tick one second after creation. There is also no handling for restriction apply/remove Commands (not normal Commands) overlapping a running check/firmware operation.
- Why it matters: Simultaneous IoT operations produce different states across implementations.
- Example Failure: Calling restrictions.execute during running FW update gives CONFLICT in one implementation but sends the command in another.
- Required Fix: Define failed-recheck state and delivery handling for restriction Commands during device operations.
- Suggested Revision: IR90-3: Recheck failure sets failed/failureCode=CONFLICT with connection unchanged. Restriction Commands to units with queued/running check/firmware use delivery=not_sent and pendingReason=device_operation_running under D03 undelivered intent; explicitly retry after completion.
- Status: resolved (IR90, DEC-51 (PROPOSED), AT-REV19-018)

### REV19-019

- Issue ID: REV19-019
- Severity: MINOR
- Document: DD-A08 dueAt
- Location: Invoice deadline input limits
- Problem: “After invoice creation time (special handling for overdue seed)” leaves special handling undefined.
- Why it matters: It is unclear whether HQ may create overdue invoices.
- Example Failure: An implementer permits past dates, changing reminder/restriction test premises.
- Required Fix: Specify how overdue invoices are created.
- Suggested Revision: IR90-4: invoices.create dueAt must be after now. Create overdue cases through seed or demo.advanceClock.
- Status: resolved (IR90, AT-REV19-019)

### REV19-020

- Issue ID: REV19-020
- Severity: MINOR
- Document: DD-T01 status, DD-P01 status
- Location: Job-status filter options
- Problem: Technician options lack rework_requested/on_hold; contractor options lack rework_requested/on_hold/completed. Returned jobs cannot be filtered.
- Why it matters: There is no path to find rework targets for FR-T08.
- Example Failure: A technician cannot find returned jobs, delaying resubmission.
- Required Fix: Align options with business-relevant JobStatus values.
- Suggested Revision: IR90-5: T01 uses assigned/in_progress/on_hold/submitted/rework_requested/completed/all. P01 uses offered/accepted/assigned/in_progress/on_hold/submitted/rework_requested/completed/all.
- Status: resolved (IR90, AT-REV19-020)

### REV19-021

- Issue ID: REV19-021
- Severity: MINOR
- Document: IR71, D10
- Location: Invalidation targets for change events
- Problem: No entityType invalidates devices.calibrations. It is unclear whether IR71's prohibition on invalidating unlisted operations also prohibits D10 related-Query invalidation after successful writes.
- Why it matters: Calibration history does not update after calibration.
- Example Failure: Saving calibration in T11 does not show it in history, prompting a duplicate save.
- Required Fix: Add the missing mapping and distinguish subscription-driven from write-driven invalidation.
- Suggested Revision: IR90-6: Add devices.calibrations to device_operation. IR71 governs subscription events; successful writes also invalidate D10 and each DD's related Queries.
- Status: resolved (IR90, AT-REV19-021)

### REV19-022

- Issue ID: REV19-022
- Severity: MINOR
- Document: AuditView.result, DD-A16 result, FR-A16
- Location: Meaning of pending audit results
- Problem: Canonical types and DD-A16 include pending, but FR-A16 has only success/rejected/failed and no rule for recording pending.
- Why it matters: Audit filter options and display vary.
- Example Failure: A Command request is audited as success, contradicting later failure.
- Required Fix: Define operations recorded as pending and how final results are appended.
- Suggested Revision: IR90-7: Receipt audits for asynchronous-result operations (commands.create, diagnosticRuns.create, devices.check/updateFirmware, restrictions.execute/retry/release/override, payments.simulate initiate) are pending. Append success/failed with the same correlationId at completion. Give FR-A16 four categories.
- Status: resolved (IR90, DEC-52 (PROPOSED), AT-REV19-022)

### REV19-023

- Issue ID: REV19-023
- Severity: MINOR
- Document: query-catalog alerts.list/notifications.list default_sort, IR34
- Location: Severity order
- Problem: Severity ranks normal<warning<critical are stated only for jobs.list (IR34) and notifications (D07). alerts.list severity desc may mean lexical order or rank.
- Why it matters: Lexical order gives warning>normal>critical, putting urgent items last.
- Example Failure: C08 shows critical alerts last.
- Required Fix: Define rank-based severity comparison for every operation.
- Suggested Revision: IR90-8: All severity sorting/comparison uses normal<warning<critical. Prohibit lexical comparison.
- Status: resolved (IR90, AT-REV19-023)

### REV19-024

- Issue ID: REV19-024
- Severity: MINOR
- Document: component-contracts.csv VoicePanel, D10, D13
- Location: Voice-panel Repository dependencies
- Problem: D10/D13 says shared Components are display-only and do not call Repository, but VoicePanel api_dependency directly lists voice.resolveIntent/commands.create and others. AppShell is already separated from ShellContainer.
- Why it matters: The UI Agent may embed retrieval in display components, shifting mock-injection boundaries in tests.
- Example Failure: VoicePanel unit tests need Repository and behavior differs by page.
- Required Fix: Define a container responsible for reads and submissions.
- Suggested Revision: IR90-9: VoiceContainer (feature hook) performs IR09 reads/submissions. VoicePanel uses only props/events. List both in component-contracts.
- Status: resolved (IR90, AT-REV19-024)

### REV19-025

- Issue ID: REV19-025
- Severity: MINOR
- Document: component-contracts.csv, UX-05, DD-C01/A01/P01/T01
- Location: Count KPI card component contract
- Problem: No component contract displays Summary/AdminSummary counts such as running/stopped/unknown or unresolved counts. MetricCard is only for Measurement.
- Why it matters: Four-role dashboards build separate cards with inconsistent denominators, unknown counts, asOf, and links.
- Example Failure: Only A01 shows a denominator while C01 omits unknown counts.
- Required Fix: Add a count KPI card contract.
- Suggested Revision: IR90-10: Add KpiCard(label, value:number|null, denominator:number|null, unknownCount:number|null, asOf, href:string|null, loading, error).
- Status: resolved (IR90, AT-REV19-025)

### REV19-026

- Issue ID: REV19-026
- Severity: MINOR
- Document: component-contracts.csv NotificationPanel
- Location: Empty notification-list display
- Problem: empty_state is fixed to “Zero unread,” even when unreadOnly=false and the entire list is empty.
- Why it matters: Zero total and zero unread cannot be distinguished.
- Example Failure: A customer with no notifications sees “Zero unread.”
- Required Fix: Use different wording for each filter.
- Suggested Revision: IR90-11: unreadOnly=true shows “No unread notifications”; false shows “No notifications.”
- Status: resolved (IR90, AT-REV19-026)

### REV19-027

- Issue ID: REV19-027
- Severity: MINOR
- Document: screen-catalog.csv SCR-X-login/forgot-password/demo/forbidden/not-found, validate_documents.py required_states
- Location: State lists for public screens
- Problem: Public screens without data retrieval uniformly list permission-denied/not-found/stale/empty and similar states, leading the Test Agent to test nonexistent states.
- Why it matters: Unnecessary implementation/testing makes state-coverage assessment meaningless.
- Example Failure: A permission-denied state branch is implemented inside /forbidden.
- Required Fix: Define state sets for public/non-data screens and vary validator requirements by screen type.
- Suggested Revision: IR90-12: login/forgot-password: initial;loading;success;error;offline. demo: initial;loading;success;error. forbidden/not-found: success only.
- Status: resolved (IR90, AT-REV19-027)

### REV19-028

- Issue ID: REV19-028
- Severity: MINOR
- Document: UIUXSpecification.md UX-02/UX-06
- Location: Incorrect definition of telemetry
- Problem: UX-02/UX-06 defines telemetry as usage records/data, but Telemetry here means sensor measurements (Measurement).
- Why it matters: The UI Agent may confuse it with usage analytics and suppress the wrong announcements.
- Example Failure: Measurement-update announcement suppression is omitted, causing announcements every minute.
- Required Fix: Use the canonical meaning.
- Suggested Revision: IR90-13: Change to “Telemetry (sensor measurements, Measurement).”
- Status: resolved (IR90, AT-REV19-028)

### REV19-029

- Issue ID: REV19-029
- Severity: MINOR
- Document: screen-catalog.csv SCR-A13/A11/A12 url_selection
- Location: Restoring selection from URLs
- Problem: C06 retains unitIds/baselineId in URL, but A13 does not; A11/A12 do not retain selected policyId. Back navigation loses selections.
- Why it matters: This conflicts with UX-02's rule that selected targets belong in URLs.
- Example Failure: Open a policy in A11, go to a unit screen, then return; nothing is selected.
- Required Fix: Add the keys to url_selection.
- Suggested Revision: IR90-14: Add unitIds,baselineId to SCR-A13 and policyId to SCR-A11/A12.
- Status: resolved (IR90, AT-REV19-029)

### REV19-030

- Issue ID: REV19-030
- Severity: MINOR
- Document: SR03, operation-catalog baselines.list/energy.summary
- Location: Customer visibility of baselines and analysis results
- Problem: SR03 returns invoices/contracts only if all target Units are readable, but lacks customer visibility rules for multi-Unit EnergyBaseline/OffsetQuote/OffsetRecord.
- Why it matters: It is unclear whether users who can view only some units receive baselines covering other units.
- Example Failure: A customer with unit-level scope retrieves a baseline containing other units' energy use.
- Required Fix: Apply the all-target-Units condition.
- Suggested Revision: IR90-15: Return EnergyBaseline/OffsetQuote/OffsetRecord only when every unitId is currently in scope. Partial access excludes them from lists and returns NOT_FOUND individually.
- Status: resolved (IR90, AT-REV19-030)

### REV19-031

- Issue ID: REV19-031
- Severity: MINOR
- Document: DD-A06 field table, DD-A11 field table
- Location: Required markers in field tables
- Problem: DD-A06 marks unitId/type/dueAt required, but dueAt is optional for HQ only (IR38). DD-A11 duplicates enabled/priority rows.
- Why it matters: Form required markers conflict with types/IR.
- Example Failure: The form cannot save without dueAt.
- Required Fix: Correct the tables.
- Suggested Revision: IR90-16: Split DD-A06 into required unitId/type and optional dueAt. Remove duplicate DD-A11 rows.
- Status: resolved (IR90, AT-REV19-031)

### REV19-032

- Issue ID: REV19-032
- Severity: MINOR
- Document: DD-P07 recipientRole, DDC-09, notifications.recipients
- Location: Mapping recipient roles to canonical Role
- Problem: The mapping from DD-P07 recipientRole (hq/assigned_technician/customer_contact) to notifications.recipients role (Role enum) is not stated.
- Why it matters: Implementers guess candidate-filter values.
- Example Failure: Sending customer_contact as contractor gives zero candidates and prevents sending.
- Required Fix: State the mapping.
- Suggested Revision: IR90-17: hq→admin; assigned_technician→technician (technician of the job's valid Assignment); customer_contact→client (customer Membership for the job unit).
- Status: resolved (IR90, AT-REV19-032)

### REV19-033

- Issue ID: REV19-033
- Severity: MINOR
- Document: IR35, IR59
- Location: Audit actors when release starts
- Problem: IR35 says the audit actor is the payment-confirming actor; IR59 says IR35 release Commands have actorRoleAtTime=system. It is unclear whether the transition creates one or two records and who owns each.
- Why it matters: Audit actor display and AT-REV18-015 checks vary.
- Example Failure: The restriction release_requested audit after customer payment confirmation displays system as actor.
- Required Fix: Separate record units and actors.
- Suggested Revision: IR90-18: Restriction transition (release_requested) audit uses the payment-confirming or other initiating actor and role at that time. remove Command creation audit uses actorId=system-restriction/actorRoleAtTime=system. Use the same correlationId.
- Status: resolved (IR90, AT-REV19-033)

### REV19-037

- Issue ID: REV19-037
- Severity: MINOR
- Document: D06, DD-P03, FR-P03, IR49, JobSummary.scheduledSlot/assignmentId
- Location: Job schedule and Assignment reference on reassignment
- Problem: Initial assignment fixes Job.scheduledSlot, but it is undefined whether reassignment/work-window extension (jobs.assign) updates Job.scheduledSlot and assignmentId to the new Assignment. Customer confirmed schedules, JobSummary.assignmentId, and IR89 window-ended display depend on these values.
- Why it matters: After extension the old window may remain, leaving the reassignment-needed badge or showing customers an old schedule.
- Example Failure: A contractor extends a window from 12:00 to 14:00, but scheduledSlot.endAt stays 12:00 and the badge remains.
- Required Fix: Define Job fields updated when jobs.assign succeeds. Found in the first self-review.
- Suggested Revision: IR89: In the same successful jobs.assign transition, update Job.assignmentId and scheduledSlot to the new Assignment and version+1. Retain the old Assignment as revoked.
- Status: resolved (IR89, AT-REV19-037)

### REV19-042

- Issue ID: REV19-042
- Severity: MINOR
- Document: AT-C01-E①, AT-C03-B③, AT-C05, AT-C07-E/B, AT-C08-E②/B, AT-C09-B, AT-C10-E/B, AT-C12, AT-P02-N, AT-P05-N, AT-T02-E, AT-T04–06/09-N, AT-T12-N, AT-A05-N, AT-A07, AT-A08-N, AT-A09-R01, AT-A10-B, AT-A11-N, AT-A12-N
- Location: Unspecified acceptance Given targets/procedures, found in the full check
- Problem: Some cases omit target units, actors, or setup procedures: which telemetry becomes null, which unit lacks ventilation, how to create an in_progress job, or who confirms a simulated payment.
- Why it matters: The Test Agent chooses targets, changing results; null temperature, for example, does not change operation classification.
- Example Failure: AT-C01-E① nulls temperature, fails to produce Unknown 1, and is judged failed.
- Required Fix: Define default targets and state setup rules; state targets explicitly in the text.
- Suggested Revision: Define IR92 defaults/procedure rules and add target IDs, actors, and steps to each acceptance case.
- Status: resolved (IR92, AT-REV19-042)

## 6. Open Questions

No pending decisions block 1A implementation handover. DEC-44 and DEC-50 were confirmed by user answers. The others are specified as reversible PROPOSED decisions to be confirmed before company acceptance.

### REV19-034

- Issue ID: REV19-034
- Severity: QUESTION
- Document: FR-A01, IR78, DEC-44
- Location: Business definition of estimated savings
- Problem: Company source BIZ-23 describes comparison with normal operation and expected reductions, but the basis for dashboard forecasts is a business decision. IR78 adopts a prorated assumed baseline and extrapolation from valid-slot averages (reversible).
- Why it matters: If the company misunderstands forecasts, the demo may appear to guarantee savings.
- Example Failure: HQ treats “Forecast 50.0% reduction” as an actual measured reduction rate.
- Required Fix: Product Owner must confirm calculation and wording.
- Suggested Revision: DEC-44 (PROPOSED): Always label the value as a forecast using an assumed prorated baseline. Confirm before company acceptance.
- Status: answered (IR78, DEC-44; DEC-44 is confirmed by the user; HTTP mapping will be decided in 1B)

### REV19-035

- Issue ID: REV19-035
- Severity: QUESTION
- Document: D11, DDC-03, service-contracts.ts ErrorCode
- Location: HTTP-to-DomainError mapping for 1B connections
- Problem: 1A excludes HTTP contracts; D11 lists status/error tables as required production deliverables. Mapping HTTP 400/401/403/404/409/429/5xx and timeout to DomainError is undefined.
- Why it matters: If the 1B adapter owner guesses mappings, UI recovery (retry/input retention/relogin) changes.
- Example Failure: HTTP 503 maps to TIMEOUT, preventing automatic read retry.
- Required Fix: Backend/Frontend must decide a mapping table before 1B design starts. Do not decide it in 1A.
- Suggested Revision: Add an explicit HTTP-status/communication-exception→DomainError mapping table to D11 required deliverables under OPEN-04.
- Status: answered (IR90; DEC-44 is confirmed by the user; HTTP mapping will be decided in 1B)

| ID | Question | Answer | Confirm with |
|---|---|---|---|
| DEC-42 (REV19-002) | Before the work window starts, should technician screens skip units.get and show read-only jobs as work-not-started? | Before start, use only jobs.get for read-only display (work-not-started) | Security / UI/UX |
| DEC-43 (REV19-003) | Should the heartbeat simulator copy only latest measured/valid/non-null values and auto-assign omitted demo sequences? | Copy only measured/valid/non-null; auto-assign sequence when omitted | IoT / Product Owner |
| DEC-44 (REV19-004) | Should admin estimated savings use a prorated assumed baseline matching the unit set, extrapolation from valid-slot averages, and a default assumed baseline in seed? | **User confirmed (option A):** Forecast using the prorated assumed baseline and extrapolated valid-slot average; label “Forecast (prorated assumed baseline, demo)” | Product Owner / Business |
| DEC-45 (REV19-008) | Pass the post-login destination through URL returnTo and ignore invalid values? | Use /login?returnTo=…; ignore invalid values | Frontend / Security |
| DEC-46 (REV19-009) | Keep displayed data with aria-busy during refetch and combine same-tick subscription invalidations once per key? | Keep data with aria-busy; combine within a one-second tick | UI/UX / Frontend |
| DEC-47 (REV19-010) | Create initial location-consent records in seed and on customer Membership creation? | granted=false/version 1 in seed and on client Membership creation | Security / Product Owner |
| DEC-48 (REV19-012) | Return CONFLICT(errors.offer_expired) for accepting/declining expired unanswered Offers, and NOT_FOUND for individual job retrieval? | CONFLICT(errors.offer_expired); jobs.get returns NOT_FOUND | Product Owner / Security |
| DEC-49 (REV19-013) | Standardize reason/resolutionReason/reviewComment and similar fields at 1000 characters maximum? | Reason fields use 1–1000 characters | UI/UX / Product Owner |
| DEC-50 (REV19-015) | Warn 15 minutes before work-window end, discard unsaved input and notify at end, and show reassignment needed to HQ/contractors? | **User confirmed (current proposal):** Warn 15 minutes before end; discard unsaved input and notify at end; show reassignment to HQ/contractors | Product Owner / UI/UX |
| DEC-51 (REV19-018) | Hold restriction Commands as undelivered intent during check/firmware operations? | Hold them as undelivered intent during device operations | IoT / Business |
| DEC-52 (REV19-022) | Audit asynchronous receipt as pending and append the final result with the same correlationId? | Receipt audit is pending; append final result | Security / Product Owner |
| DEC-53 (REV19-017) | Keep contact-window validation and show HH:mm input guidance? | Keep validation; show HH:mm guidance | UI/UX / Security |
| REV19-035 | How should 1B map HTTP statuses and communication exceptions to DomainError? | Not decided in 1A; add to D11 required production deliverables | Backend / Frontend |

## 7. Cross-document Inconsistencies

| Type | Details | Status |
|---|---|---|
| A. Requirement Missing | Design cannot provide FR-A01 estimated savings (REV19-004). No transfer mechanism for FR-X01 return destination (008). FR-T09 unsaved-input retention fails at work-window end (015) | resolved |
| B. Design Without Requirement | Audit pending is missing from FR-A16's three categories (022). DD-A14 says to add display fields absent from canonical types (014) | resolved |
| C. UI Without Requirement | No new cases. KpiCard/VoiceContainer (024/025) contracts implement existing requirements | Unchanged |
| D. UI Without API | SCR-T04/T10 uses units.get as primary before it is accessible (002). /demo cannot know required telemetry sequence (003). /login return destination is absent from URL (008) | resolved |
| E. Data Model Gap | No AdminSummary forecast type (004); DD-A14 display fields (014); shorthand demoSeed (036); initial Consent (010); Job.scheduledSlot on reassignment (037); missing default factor seed (040); energy acceptance data (041) | resolved |
| F. Terminology Conflict | Wrong telemetry definition (028); recipientRole versus Role (032); customer-organization versus Customer count and room display-name versus Space.name (016) | resolved |
| Acceptance oracle | Given/seed conflicts (038), expectation/authorization/state conflicts (039), unspecified targets (042), C06 expected strings (006), KPI Given (011) | resolved (IR80/IR85/IR92/IR93) |
| Normative priority | IR63/IR68 display conflict (004); superseded text remains in IR (005/007) | resolved |

## 8. Missing Requirements

Missing Requirement Candidates (do not add to 1A; retain for handover):

| Candidate | Basis | Handling |
|---|---|---|
| Technician requests to contractor/HQ for work-window extension | IR89 warns before end, but only actors with jobs.assign can extend | Deferred as a candidate |
| Customer notification-channel preferences | Carried over from 0.17.0 | Deferred as a candidate |
| Editing available work hours; holiday calendar | D07/IR70 fixes Monday–Friday 09:00–17:00 with no holidays | Deferred as a candidate |
| Memory limits for continuous long-running operation | Deferred under IR18 | Deferred as a candidate |
| Business rules for overlapping contract periods on the same unit | IR92 explicitly does not reject them in 1A | Confirm commercial rules before company acceptance |
| Production HTTP→DomainError mapping, real authentication, cross-tab synchronization | Outside 1A under D11/OPEN-04 and D09 | Decide before 1B design starts (§11) |

## 9. Edge Cases Not Defined

“Initial” means the start of this review; “after correction” means DOC-0.19.0.

| Edge case | Definition | Initial | After correction |
|---|---|---|---|
| API timeout (10 seconds per attempt) | D04, IR37, DDC-03 | Defined | Defined |
| HTTP 400 equivalent (VALIDATION) | D01 priority 1/7, DDC-03 | Defined | Defined |
| HTTP 401 equivalent (UNAUTHENTICATED/expiry) | D01 priority 2, D09, IR55 | Conflict (IR36/IR55) | Defined (IR79) |
| HTTP 403 equivalent (FORBIDDEN) | D01 priority 4, IR57, IR76, IR93 | Conflict (before work start/start after cancellation) | Defined (IR76/IR93) |
| HTTP 404 equivalent (NOT_FOUND) | D01 priority 3, IR57, IR86, IR93 | Undefined (expired Offer/start unassigned job) | Defined (IR86/IR93) |
| HTTP 409 equivalent (CONFLICT) | D01 priority 5/6, D04 | Defined | Defined |
| HTTP 429 equivalent (RATE_LIMITED) | D04, IR37 | Defined | Defined |
| HTTP 500 equivalent (UNAVAILABLE/unknown exception) | D01, DDC-03, IR44 | 1A defined / 1B mapping undefined | 1A defined / 1B mapping under D11 (REV19-035) |
| Network disconnected | IR37, DDC-03 | Defined | Defined |
| IoT device offline | IR47, D03 | Defined | Defined |
| IoT device response timeout | D04/D05 | Defined | Defined |
| Restriction Commands during device operations | IR90 (REV19-018) | Edge Case Undefined | Defined |
| Invalid sensor data | IR12, D07 | Defined | Defined |
| Missing / estimated sensor data | D07, IR08, IR77 | Conflict | Defined (IR77) |
| Stale sensor data | D07, SR27, IR45 | Defined | Defined |
| Duplicate operation | D04, D02 | Defined | Defined |
| Multiple browser tabs | D09 (always show out-of-scope notice) | Defined (outside scope) | Defined (outside scope) |
| Session expiration | D09, IR36, IR55 | Conflict | Defined (IR79) |
| Permission changed during operation | IR17, IR24, NFR-03 | Defined | Defined |
| Work window expires during work | IR89 | Edge Case Undefined | Defined (DEC-50 confirmed by user) |
| Empty device list | DD-C01, FR-A01, IR78 | Defined | Defined |
| Large device list | D07, D10, IR18 | Defined (up to 100 units) | Defined (up to 100 units) |
| Slow network | IR37, D10 | Defined | Defined |
| Background refetch / periodic invalidation | IR83 | Edge Case Undefined | Defined |
| Language switch | D09, IR44 | Defined | Defined |
| Browser reload | DEC-07, D09 | Defined | Defined |
| Back button | D13, IR34, IR50 | Defined | Defined |
| Deep link before login | IR82 | Edge Case Undefined | Defined |
| Concurrent update | D04, write-version-catalog | Defined | Defined |
| Reassignment / extension | D06, IR49, IR89 | Undefined (Job.scheduledSlot update) | Defined |
| Overlapping assignment in acceptance data | IR92, acceptancePatches | Conflict (overlaps seed assignment) | Defined |

## 10. Traceability Matrix

Status has two columns: before correction (at review start) and after correction (DOC-0.19.0). Machine-readable version: [traceability-matrix.csv](traceability-matrix.csv).

| Requirement ID | Requirement | Prepare | Detailed Design | UI/UX | API | Error Handling | Testable | Status (before correction) | Status (after correction) |
|---|---|---|---|---|---|---|---|---|---|
| FR-C01 | Monitoring dashboard (status at a glance) | BIZ-04;BIZ-08 | DD-C01 | SCR-C01 | operation-catalog (Repository) | D01/D04/IR37/IR77/IR83/IR90/IR92 | AT-C01-N;AT-C01-E;AT-C01-B | CONFLICT | OK |
| FR-C02 | Location hierarchy management | BIZ-07 | DD-C02 | SCR-C02 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-C02-N;AT-C02-E;AT-C02-B | CONFLICT | OK |
| FR-C03 | Remote control (operation from another location) | BIZ-13 | DD-C03 | SCR-C03 | operation-catalog (Repository) | D01/D04/IR37 | AT-C03-N;AT-C03-E;AT-C03-B | OK | OK |
| FR-C04 | Scheduled operation and cooling before returning home | BIZ-14 | DD-C04 | SCR-C04 | operation-catalog (Repository) | D01/D04/IR37 | AT-C04-N;AT-C04-E;AT-C04-B | OK | OK |
| FR-C05 | Automatic operation with consent | BIZ-14;BIZ-15;BIZ-17 | DD-C05 | SCR-C04 | operation-catalog (Repository) | D01/D04/IR37/IR84/IR92 | AT-C05-N;AT-C05-E;AT-C05-B | CONFLICT | OK |
| FR-C06 | Compare power use and electricity costs | BIZ-16;BIZ-23 | DD-C06 | SCR-C06 | operation-catalog (Repository) | D01/D04/IR37/IR80/IR90/IR92 | AT-C06-N;AT-C06-E;AT-C06-B | CONFLICT | OK |
| FR-C07 | Air conditions (air environment) | BIZ-18;BIZ-19 | DD-C07 | SCR-C07 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-C07-N;AT-C07-E;AT-C07-B | INCOMPLETE | OK |
| FR-C08 | Alert and inspection notifications | BIZ-08;BIZ-09;BIZ-17 | DD-C08 | SCR-C08 | operation-catalog (Repository) | D01/D04/IR37/IR90/IR92 | AT-C08-N;AT-C08-E;AT-C08-B | CONFLICT | OK |
| FR-C09 | Maintenance requests, bookings, and history | BIZ-12 | DD-C09 | SCR-C09 | operation-catalog (Repository) | D01/D04/IR37/IR89/IR90/IR92 | AT-C09-N;AT-C09-E;AT-C09-B | INCOMPLETE | OK |
| FR-C10 | View contracts and invoices | BIZ-21 | DD-C10 | SCR-C10 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-C10-N;AT-C10-E;AT-C10-B | INCOMPLETE | OK |
| FR-C11 | Payment guidance and payment demo | BIZ-22 | DD-C11 | SCR-C11 | operation-catalog (Repository) | D01/D04/IR37 | AT-C11-N;AT-C11-E;AT-C11-B | OK | OK |
| FR-C12 | Explain operation restrictions | BIZ-21 | DD-C12 | SCR-C11 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-C12-N;AT-C12-E;AT-C12-B;AT-C12-R01 | INCOMPLETE | OK |
| FR-C13 | Emissions and offset guidance | BIZ-23;BIZ-24;BIZ-25;BIZ-26 | DD-C13 | SCR-C13 | operation-catalog (Repository) | D01/D04/IR37/IR90/IR92 | AT-C13-N;AT-C13-E;AT-C13-B | CONFLICT | OK |
| FR-P01 | Contracted-work status dashboard | BIZ-04;BIZ-12 | DD-P01 | SCR-P01 | operation-catalog (Repository) | D01/D04/IR37/IR90/IR92 | AT-P01-N;AT-P01-E;AT-P01-B | CONFLICT | OK |
| FR-P02 | Accept or decline jobs | BIZ-12 | DD-P02 | SCR-P02 | operation-catalog (Repository) | D01/D04/IR37/IR86/IR92 | AT-P02-N;AT-P02-E;AT-P02-B | CONFLICT | OK |
| FR-P03 | Schedule and assign company technicians | BIZ-12 | DD-P03 | SCR-P03 | operation-catalog (Repository) | D01/D04/IR37/IR89/IR92 | AT-P03-N;AT-P03-E;AT-P03-B;AT-P03-R01 | CONFLICT | OK |
| FR-P04 | View target equipment and alert evidence | BIZ-12 | DD-P04 | SCR-P04 | operation-catalog (Repository) | D01/D04/IR37 | AT-P04-N;AT-P04-E;AT-P04-B | OK | OK |
| FR-P05 | Report quality review and return for correction | BIZ-12 | DD-P05 | SCR-P05 | operation-catalog (Repository) | D01/D04/IR37/IR87/IR92/IR93 | AT-P05-N;AT-P05-E;AT-P05-B;AT-P05-R01 | CONFLICT | OK |
| FR-P06 | View company workers and availability | BIZ-12 | DD-P06 | SCR-P06 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-P06-N;AT-P06-E;AT-P06-B | CONFLICT | OK |
| FR-P07 | Job communication and history | BIZ-12;BIZ-20 | DD-P07 | SCR-P07 | operation-catalog (Repository) | D01/D04/IR37/IR90 | AT-P07-N;AT-P07-E;AT-P07-B;AT-P07-R01 | INCOMPLETE | OK |
| FR-P08 | Boundaries of contracted access and work periods | BIZ-12 | DD-P08 | SCR-P01;SCR-P02;SCR-P03;SCR-P04;SCR-P05;SCR-P06;SCR-P07 | operation-catalog (Repository) | D01/D04/IR37/IR86 | AT-P08-N;AT-P08-E;AT-P08-B | CONFLICT | OK |
| FR-T01 | Assigned-work status dashboard | BIZ-04;BIZ-08 | DD-T01 | SCR-T01 | operation-catalog (Repository) | D01/D04/IR37/IR76/IR90 | AT-T01-N;AT-T01-E;AT-T01-B | CONFLICT | OK |
| FR-T02 | Equipment register (list of equipment information) | BIZ-06;BIZ-07;BIZ-10 | DD-T02 | SCR-T02 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-T02-N;AT-T02-E;AT-T02-B | INCOMPLETE | OK |
| FR-T03 | Time-series monitoring (monitoring over time) | BIZ-08;BIZ-11 | DD-T03 | SCR-T02 | operation-catalog (Repository) | D01/D04/IR37/IR77/IR83 | AT-T03-N;AT-T03-E;AT-T03-B | CONFLICT | OK |
| FR-T04 | Indoor unit inspection | BIZ-10 | DD-T04 | SCR-T04 | operation-catalog (Repository) | D01/D04/IR37/IR76/IR92 | AT-T04-N;AT-T04-E;AT-T04-B | CONFLICT | OK |
| FR-T05 | Outdoor unit inspection | BIZ-10 | DD-T05 | SCR-T04 | operation-catalog (Repository) | D01/D04/IR37 | AT-T05-N;AT-T05-E;AT-T05-B | OK | OK |
| FR-T06 | Electrical and control component inspection | BIZ-10 | DD-T06 | SCR-T04 | operation-catalog (Repository) | D01/D04/IR37 | AT-T06-N;AT-T06-E;AT-T06-B | OK | OK |
| FR-T07 | Alert evidence | BIZ-08;BIZ-11;BIZ-17 | DD-T07 | SCR-T07 | operation-catalog (Repository) | D01/D04/IR37/IR87 | AT-T07-N;AT-T07-E;AT-T07-B | CONFLICT | OK |
| FR-T08 | Regular inspections, fault response, and preventive maintenance | BIZ-12 | DD-T08 | SCR-T04 | operation-catalog (Repository) | D01/D04/IR37/IR76/IR89/IR93 | AT-T08-N;AT-T08-E;AT-T08-B | CONFLICT | OK |
| FR-T09 | Work reports | BIZ-12 | DD-T09 | SCR-T04 | operation-catalog (Repository) | D01/D04/IR37/IR89 | AT-T09-N;AT-T09-E;AT-T09-B | INCOMPLETE | OK |
| FR-T10 | Remote diagnostics and test runs | BIZ-13 | DD-T10 | SCR-T10 | operation-catalog (Repository) | D01/D04/IR37/IR76/IR93 | AT-T10-N;AT-T10-E;AT-T10-B;AT-T10-R01 | CONFLICT | OK |
| FR-T11 | IoT device lifecycle management | BIZ-20 | DD-T11 | SCR-T11 | operation-catalog (Repository) | D01/D04/IR37/IR90/IR92 | AT-T11-N;AT-T11-E;AT-T11-B;AT-T11-R01 | CONFLICT | OK |
| FR-T12 | IoT device alerts | BIZ-20 | DD-T12 | SCR-T12 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-T12-N;AT-T12-E;AT-T12-B | INCOMPLETE | OK |
| FR-A01 | Overall dashboard | BIZ-04;BIZ-08 | DD-A01 | SCR-A01 | operation-catalog (Repository) | D01/D04/IR37/IR77/IR78/IR83/IR85/IR90 | AT-A01-N;AT-A01-E;AT-A01-B | CONFLICT | OK |
| FR-A02 | Organization, location, and equipment registers | BIZ-07 | DD-A02 | SCR-A02 | operation-catalog (Repository) | D01/D04/IR37 | AT-A02-N;AT-A02-E;AT-A02-B | OK | OK |
| FR-A03 | Four roles and scope management | BIZ-04 | DD-A03 | SCR-A03 | operation-catalog (Repository) | D01/D04/IR37 | AT-A03-N;AT-A03-E;AT-A03-B | OK | OK |
| FR-A04 | Model and IoT capability register | BIZ-06;BIZ-20 | DD-A04 | SCR-A04 | operation-catalog (Repository) | D01/D04/IR37/IR90 | AT-A04-N;AT-A04-E;AT-A04-B | CONFLICT | OK |
| FR-A05 | Alert and notification policies | BIZ-08;BIZ-11;BIZ-17 | DD-A05 | SCR-A05 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-A05-N;AT-A05-E;AT-A05-B | INCOMPLETE | OK |
| FR-A06 | Maintenance plans, quality, and costs | BIZ-12 | DD-A06 | SCR-A06 | operation-catalog (Repository) | D01/D04/IR37/IR89/IR90/IR92 | AT-A06-N;AT-A06-E;AT-A06-B | CONFLICT | OK |
| FR-A07 | Contract plans | BIZ-21 | DD-A07 | SCR-A07 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-A07-N;AT-A07-E;AT-A07-B | INCOMPLETE | OK |
| FR-A08 | Billing, payment receipts, and reminders | BIZ-21;BIZ-22 | DD-A08 | SCR-A08 | operation-catalog (Repository) | D01/D04/IR37/IR90/IR92 | AT-A08-N;AT-A08-E;AT-A08-B;AT-A08-R01 | INCOMPLETE | OK |
| FR-A09 | Advance notice, restrictions, and release after payment | BIZ-21 | DD-A09 | SCR-A09 | operation-catalog (Repository) | D01/D04/IR37/IR90 | AT-A09-N;AT-A09-E;AT-A09-B;AT-A09-R01 | CONFLICT | OK |
| FR-A10 | Deferrals, exemptions, manual release, and audit | BIZ-21 | DD-A10 | SCR-A10 | operation-catalog (Repository) | D01/D04/IR37/IR87 | AT-A10-N;AT-A10-E;AT-A10-B | CONFLICT | OK |
| FR-A11 | Automatic operation policies | BIZ-14;BIZ-16;BIZ-17 | DD-A11 | SCR-A11 | operation-catalog (Repository) | D01/D04/IR37/IR90/IR92 | AT-A11-N;AT-A11-E;AT-A11-B;AT-A11-R01 | INCOMPLETE | OK |
| FR-A12 | Air environment policies | BIZ-18;BIZ-19 | DD-A12 | SCR-A12 | operation-catalog (Repository) | D01/D04/IR37/IR90/IR92 | AT-A12-N;AT-A12-E;AT-A12-B | CONFLICT | OK |
| FR-A13 | Energy-saving analysis | BIZ-23;BIZ-25 | DD-A13 | SCR-A13 | operation-catalog (Repository) | D01/D04/IR37/IR78/IR90/IR92 | AT-A13-N;AT-A13-E;AT-A13-B | CONFLICT | OK |
| FR-A14 | Digital MRV demo | BIZ-25 | DD-A14 | SCR-A14 | operation-catalog (Repository) | D01/D04/IR37/IR87/IR88/IR92 | AT-A14-N;AT-A14-E;AT-A14-B;AT-A14-R01 | CONFLICT | OK |
| FR-A15 | Offset demo | BIZ-24;BIZ-26 | DD-A15 | SCR-A15 | operation-catalog (Repository) | D01/D04/IR37 | AT-A15-N;AT-A15-E;AT-A15-B | OK | OK |
| FR-A16 | Abnormal operations and audit | BIZ-20 | DD-A16 | SCR-A16 | operation-catalog (Repository) | D01/D04/IR37/IR90 | AT-A16-N;AT-A16-E;AT-A16-B | INCOMPLETE | OK |
| FR-X01 | Sign in, sign out, reset password, select demo role, and select display language | BIZ-01;BIZ-02 | DD-COMMON | SCR-X-login;SCR-X-forgot-password;SCR-X-settings-preferences | operation-catalog (Repository) | D01/D04/IR37/IR79/IR82/IR90 | AT-X01 | CONFLICT | OK |
| FR-X02 | Use the selected language for voice status checks, operations, and help (operating instructions). The same actions are available through text input. | BIZ-03 | DD-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37/IR90 | AT-X02 | CONFLICT | OK |
| FR-X03 | Clearly distinguish severity, measured and estimated values, inspection results, data quality, and units. | BIZ-09;BIZ-18 | DD-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37/IR77/IR90 | AT-X03 | CONFLICT | OK |
| FR-X04 | Limit what users can view and operate based on tenant, role, assigned scope, period, and capability. | BIZ-04 | DD-COMMON | SCR-X-forbidden;SCR-X-not-found | operation-catalog (Repository) | D01/D04/IR37/IR76/IR90 | AT-X04 | CONFLICT | OK |
| FR-X05 | Clearly distinguish shared demo data, data reset, and real processing. | BIZ-05 | DD-COMMON | SCR-X-demo | operation-catalog (Repository) | D01/D04/IR37/IR77/IR91 | AT-X05 | CONFLICT | OK |
| FR-X06 | Distinguish supported features and handling of non-RTO contracts (contracts other than RTO). | BIZ-05;BIZ-06;BIZ-12;BIZ-19 | DD-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37 | AT-X06 | OK | OK |
| FR-X07 | Provide notification, history, and audit mechanisms. | BIZ-08;BIZ-20;BIZ-22 | DD-COMMON | SCR-X-notifications | operation-catalog (Repository) | D01/D04/IR37/IR82/IR90 | AT-X07 | INCOMPLETE | OK |
| NFR-01 | Accessibility (keyboard; WCAG 2.2 AA target) | BIZ-05 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37/IR79/IR83/IR90 | AT-NFR01 | CONFLICT | OK |
| NFR-02 | Responsive layout (360–1440px; 200% zoom) | BIZ-05 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37 | AT-NFR02 | OK | OK |
| NFR-03 | Fictional data, no sensitive output, and confirmation of risky actions | BIZ-05 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37 | AT-NFR03 | OK | OK |
| NFR-04 | Performance targets (200ms/2 seconds) | BIZ-05 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37 | AT-NFR04 | OK | OK |
| NFR-05 | State display, duplicate-submission prevention, and input retention | BIZ-05 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37/IR76/IR83/IR90 | AT-NFR05 | CONFLICT | OK |
| NFR-06 | Asynchronous interfaces and mock replacement | BIZ-05 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37/IR90 | AT-NFR06 | INCOMPLETE | OK |
| NFR-07 | Type checks, lint, build, unit/E2E tests, and evidence | BIZ-05 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37/IR75/IR81/IR91 | AT-NFR07 | INCOMPLETE | OK |
| NFR-08 | Translation keys, Intl, and UTC storage | BIZ-02 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37 | AT-NFR08 | OK | OK |

Before correction: OK 14, INCOMPLETE 16, MISSING 0, CONFLICT 34. After correction: OK 64.

## 11. Undefined Decisions

AI must not finalize these as commercial or production decisions. In 1A they are reversible PROPOSED choices (DEC-42–53); the user confirmed DEC-44/50 as 1A specifications. The owners below finalize commercial/production decisions.

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| UD-01 (DEC-44) | Commercial treatment of forecast calculation/display (option A confirmed for 1A) | FR-A01, IR78 | Forecasts may be mistaken for guaranteed savings | 1A: user confirmed / Commercial: Product Owner / Business |
| UD-02 (DEC-50) | Unsaved input at work-window end (current proposal confirmed for 1A) and extension-request process | FR-T09, IR89 | Field-work input loss and responsibility boundaries | 1A: user confirmed / Commercial: Product Owner / UI/UX |
| UD-03 (DEC-48) | Responses to expired work offers | FR-P02, IR86 | Contracted-work operating rules | Product Owner / Security |
| UD-04 (DEC-49) | Maximum reason/comment lengths | D12, IR87 | Audit/report text volume | UI/UX / Product Owner |
| UD-05 (DEC-51) | Priority of restriction application during device operations | IR90, D03 | Restriction delays and real-device safety | IoT / Business |
| UD-06 (DEC-52) | Recording pending audit results | FR-A16, IR90 | Audit evidence requirements | Security / Product Owner |
| UD-07 (DEC-43) | Value-generation policy for the demo heartbeat simulator | IR45/IR77 | Demo appearance and quality display | IoT / Product Owner |
| UD-08 (DEC-42/45/46/47/53) | Pre-work display, login return, refetch display, initial consent, contact-window guidance | IR76/82/83/84/90 | Screen behavior and personal-information handling | UI/UX / Security |
| UD-09 (REV19-035) | HTTP-status/communication-exception→DomainError mapping | D11, DDC-03 | 1B adapter recovery behavior | Backend / Frontend |
| UD-10 (OPEN-04) | Production APIs/authentication/DB/server authorization | D11 | Production connection prerequisites | Backend / Security |
| UD-11 (OPEN-03/11) | Sensors/device capabilities/timed-operation execution owner/failure recovery | D05/D11 | Real-device control safety | IoT / Backend |
| UD-12 (OPEN-05/06) | Payment/notification/location/tariff integrations, emission factors, MRV schemes | D11, FR-C11/A14 | Selecting external services and schemes | Business / Backend |
| UD-13 (OPEN-10) | Contractor as a separate fourth role | PrepareDocument §7 | Roles and responsibility boundaries | Product Owner / Business |
| UD-14 (IR70/IR92) | Holiday calendars and commercial overlap rules for contracts on the same unit | IR70, IR92 | Utilization and contract management | Business |

## 12. Implementation Readiness

| Area | Initial assessment | After correction (self-review) | Basis |
|---|---|---|---|
| Requirements completeness | CONDITIONALLY READY | READY | Added FR-A01 forecasts (user confirmed), FR-X01 return destination, FR-A16 result categories |
| Cross-document consistency | NOT READY | READY | Resolved IR36/IR55, IR63/IR68, C06 display, and remaining old text; expanded detector |
| UI/UX completeness | CONDITIONALLY READY | READY | work-not-started, refreshing, public-screen state sets, KpiCard, VoiceContainer |
| Frontend architecture | CONDITIONALLY READY | READY | Display Component/Container separation; grouped invalidation |
| API contract readiness | CONDITIONALLY READY | READY (1A Repository contracts) / production HTTP NOT READY (D11, outside scope) | EnergyForecast type, optional sequence, TypeScript strict passed |
| Error handling | CONDITIONALLY READY | READY | Expired Offers, pre-work windows, start after cancellation, refetch failures, device-operation conflicts (IR76/83/86/90/93) |
| Authentication / Authorization | CONDITIONALLY READY | READY | returnTo validation, work-not-started versus permission denial, outsourced quality reviewers, customer baseline visibility |
| IoT state handling | NOT READY | READY | Simulator copying conditions; restriction Commands during device operations |
| Testability | NOT READY | READY | Given interpretation and acceptancePatches, seed normalization/default factor, aligned strings, AT-REV19-001–042 (IR80/85/91/92/93) |
| Agentic SDLC handoff readiness | NOT READY | READY (document/baseline/validator conditions) / independent G1 pending | spec-manifest, both validators passed, run records. G1 requested from another reviewer |

## 13. Required Actions Before Implementation

1. Another Agent must independently assess and record G1 for the DOC-0.19.0 spec-manifest. This report's post-correction assessment is self-review and cannot replace it.
2. The implementation Agent uses only all files in [spec-manifest.json](spec-manifest.json), not records from DOC-0.18.0 or earlier. If specifications change, regenerate the manifest and pass both validators (IR73/IR75).
3. Build acceptance premises using IR92 interpretation rules, acceptancePatches, and IR69 patches. Do not guess values missing from Given. Normalize demoSeed under IR91.
4. DEC-44/50 are confirmed 1A specifications. Implement other DEC-42–53 as PROPOSED; owners confirm before company acceptance. Do not describe DEC-44 forecasts as guaranteed savings.
5. Before 1B design (production APIs, real devices, payments), decide UD-09–UD-12 in §11 and create D11 required deliverables.

## Appendix: Self-review Record

- Round 1 (immediately after corrections): Found missing Job.scheduledSlot/assignmentId update rules for reassignment/extension (REV19-037, IR89). Clarified the IR76 start-time source and IR90 (REV19-018) recheck target. Changed AT-REV19-018/021/022 into executable procedures.
- Round 2: Compared corrected contracts, catalogs, fixtures, acceptance plan, traceability tables, index, and validation plan (§1–§11). Clarified validation-plan §2 clock wording and S08 state sequence without normative changes. Zero unresolved findings.
- Round 3 (user-requested full acceptance-premise check): Compared requirement acceptance cases (N/E/B, SRC, R01) and S01–S08 premises with demoSeed. Found seed conflicts (REV19-038), expectation/rule conflicts (039), missing default factor (040), energy-data generation gaps (041), and unspecified targets (042). Added IR92/IR93, acceptancePatches (19 keys), and seed factors, and corrected text. Added patch-reference/application checks and energy/coverage recalculation to the validator. Expanded mutation tests to 62 cases; the first run found one missed detection caused by duplicate check terms, which was fixed, after which all cases were detected. Zero unresolved findings.
