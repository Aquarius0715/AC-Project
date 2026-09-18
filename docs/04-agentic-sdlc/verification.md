---
document_id: TEST-PLAN
version: 0.21.0
status: planned-not-executed
owner: test-agent
scope: frontend-demo-1A
---

# Verification Plan and Acceptance Scenarios

This is a test plan for the future application. The app has not been implemented, and none of the tests below have been run. Verify each of the 64 requirements, AT-C/P/T/A/X, and AT-NFR entries in the [traceability matrix](../00-prepare/traceability.csv), then also check cross-role consistency through S01–S08. Cross-role scenarios alone do not prove coverage of all requirements.

This version gives each of the 49 requirements across four roles three acceptance categories: AT-number-N (normal), -E (boundary/exception), and -B (business conditions), for 147 cases. With 10 AT-*-SRC source-detail cases, 10 additional R01 cases, 7 common cases, and 8 nonfunctional cases, there are 182 requirement case bundles (from 0.7.0; every AT-FIX added in 0.8.0 is also required). UI-only cases and cross-role S scenarios are counted separately. The number 182 counts management bundles, not individual assertions. The ①② markers in acceptance rows identify observations within a cell, not subcase IDs. Separate independent input conditions into cases and check every assertion in each case. Use all chapters of the deterministic contracts and strict review correction contracts for valid shared fixtures, error precedence, and clock boundaries. Do not derive expected results from implementation. Treat each boundary-value variation as a separate subcase. Check the manifest together with the specification baseline at G1. Do not mark a case ready until design fixes missing expected results. Do not pass a parent AT until all subcases and applicable SRC/R01 cases pass.

## 1. Test levels and completion evidence

| Level | Main checks | Evidence |
|---|---|---|
| Unit | Tenant, period, and capability policies; state transitions; money/emission calculations; schema boundaries | Case name, result, target function |
| Component | Input validation, disabled reasons, requested/confirmed values, missing data, dialog focus movement | User interaction test results from RTL or similar tools |
| Contract | Frontend service input/output, combined errors, visibility, duplicate operations, versions; mocks only | Adapter name and redacted request/response records |
| E2E | S01–S08 with four roles switched in one tab, deep links, reload | Implementation revision, browser, video/trace/screenshots |
| Manual/automated accessibility | Keyboard, screen reader, contrast, 360/768/1280px widths, 200% zoom, English/Malay | Environment, actual interactions, and results. Automated checks alone do not establish compliance |

During implementation, define package scripts first, then record actual type-check, lint, build, and test commands. Do not claim that nonexistent commands passed. Acceptance requires coverage of every P0/P1 requirement, zero P0/P1 defects, and a clear list of remaining defects and open issues. Do not exclude failed cases as out of scope.

## 2. Fixed fixtures

| Data | Purpose |
|---|---|
| tenant-a / tenant-b | Check rejection of cross-tenant operations |
| customer-a / customer-b | Check customer boundaries within one tenant |
| contractor-a / contractor-b | Check contractor separation and rejection of assignments to another company |
| tech-internal-a / tech-external-a / tech-external-b | Check internal scope and external company/job/period boundaries |
| hq-operator / hq-restriction-manager | Compare normal HQ permissions with restriction/override permissions |
| property-home-a / property-office-a, floor-1 / room-1 | Check home/office properties, hierarchy, breadcrumbs |
| unit-online-rto / unit-offline-rto / unit-non-rto / unit-limited / unit-other-customer | Check controls, partial application, general maintenance, capability differences, unauthorized operations |
| device-no-sensor / device-tamper / ventilation-demo | Check missing measurements, dedicated removal events, ventilation |
| invoice-overdue-a, job-internal-a, job-contractor-a | Fixed IDs for S02/S03/S08 |
| clock=2026-09-14T01:00:00Z | Start time on reset/reload (advances in real time in the app; IR36). Freeze automated unit/component/acceptance tests through IR69 clock injection. Test exactly at expiry and ±1 millisecond |

Proposed boundaries: permissions are valid when `validFrom <= now < validUntil`; payments are overdue when `now > dueAt`; restrictions start when `now >= executeAfter`. Fix schedule and grace-period boundaries with the same rules.

Mock measurements: baseline 100kWh, actual 80kWh, sample factor 0.5kgCO₂e/kWh, sample rate 0.5MYR/kWh. Expected actual emissions are 40kgCO₂e, energy savings 20kWh (20%), estimated cost savings 10MYR, and estimated emission reductions 10kgCO₂e. These are not actual regional factors or guaranteed values. Also prepare negative reductions, a zero baseline, missing data, and zero expected samples.

## 3. S01 Control AC from a room

Given: customer-a, an online supported unit, no restrictions. Steps: Create/edit home or office properties, floors, and rooms in the customer's organization. Check rejection of parent IDs from other organizations and circular references. Open home → floor → room → unit. Confirm room temperature 28°C and setpoint 26°C, then request 24°C. Emit requested → sent → acknowledged events in order.

Expected: Before acknowledgment, keep the confirmed 26°C setpoint and show the pending 24°C request separately. Change the setpoint to 24°C only after acknowledgment. This action does not change the 28°C room temperature. History uses the same commandId. Do not show success on failure or timeout. The same control works for units under general maintenance.

Additional checks: double-clicks, the same idempotency key, late old responses, responses after expiry, actions during permission changes, and delayed responses during role switching. Reject access to another customer's data through direct URLs or Repository calls.

## 4. S02 From fault detection to maintenance completion

Given: A filter-inspection recommendation alert on unit-online-rto. Steps: Customer requests maintenance from the unit → HQ assigns an internal technician → technician records evidence, indoor/outdoor/electrical inspections, photos, measurements, replaced parts, and next actions → submits → HQ reviews.

Expected: The same jobId moves through assigned → in_progress → submitted → completed. The customer receives the result and HQ receives completion information. The Alert remains open or acknowledged. It becomes resolved only after remeasurement or a resolution action with an explicit reason.

Additional checks: reject submission with missing required fields; return → revised draft → resubmission; duplicate submission; preserve input on save failure; prevent starting cancelled or expired jobs. S08 covers outsourcing.

## 5. S03 Payments and operating restrictions

Given: An overdue invoice, a contract allowing restrictions, both online and offline target units, and an authorized HQ user. Steps: Create advance restriction notice → customer checks reason, deadline, release conditions, and notification preview → HQ attempts execution before the deadline → advance the demo clock and request application → only some units acknowledge → customer makes a simulated payment and it is confirmed (verify that the transition making all source invoices paid also starts the release request; IR35) → verify explicit restrictions.release returns the same state idempotently → devices acknowledge.

Expected: Reject execution before the deadline. Application is complete only when all devices succeed; release is complete only when all devices have released/not_required evidence. No real card input or external transmission occurs. The invoice remains unpaid while payment is processing and becomes paid only on confirmation. Keep offline devices pending if delivery occurred but application is unknown. Use D03 evidence to mark confirmed non-delivery not_required so release does not wait forever. Audit records contain actor, reason, time, target, and correlation ID.

Additional checks: grace periods, exceptions, cancellation before execution, cancellation/release after a request, manual release, insufficient permission, general maintenance contracts, duplicate payment confirmation, CONFLICT, late application acknowledgment after a release request, and failure retries. Manual release must not change Invoice to paid.

## 6. S04 Air quality and ventilation

Given: Rising CO₂ concentration; one unit with ventilation equipment and one without. Steps: Change synthetic sensor values and check customer guidance and HQ threshold/notification policies. Send simulated ventilation requests only to units with that capability.

Expected: Label ppm and thresholds as demo values. For units without ventilation equipment, show guidance only; do not call fan operation ventilation. Show missing data as unknown, not 0 or normal. Do not present a high-CO₂ display as a medical guarantee.

Additional checks: missing automation condition data, HQ policy precedence, rejection of firing under restrictions, and clear separation of CO₂ concentration (ppm) and emissions (kgCO₂e).

## 7. S05 Power, emissions, and MRV

Given: The baseline 100kWh/actual 80kWh mock data above (`acceptancePatches["AT-C06-N"]` in fixture-contract.json; window [2026-09-14T00:00Z, 01:00Z); factor-demo-2026; IR92). Steps: Customer and HQ compare the same period, units, and baseline → view MRV preview → simulate opting into offsets and record retirement.

Expected: Calculated values match the fixture. Show conditions, factor version, data quality, and estimated/demo labels. Missing data reduces coverage; a zero baseline gives no percentage. Actual usage of 120 gives negative savings. Do not label MRV externally verified or imply that offsets produced a real certificate or transaction.

Additional checks: reject or mark incalculable reversed periods, mismatched calculation boundaries, missing factors, and zero expected samples. Do not combine another customer's analysis values.

## 8. S06 IoT device lifecycle and faults

Given: An unregistered device, a calibratable sensor, and compatible firmware candidates. Steps: Technician registers device → links it to a unit → checks connection → records calibration history → firmware update succeeds/fails → network loss, power loss, and a dedicated removal event occur → recovery.

Expected: Reject duplicate serial registration and links to unauthorized units. Failed firmware updates leave the version unchanged; controls are unavailable during updates. Do not label a disconnected device stolen based only on disconnection. HQ and technician see the same detection/recovery times. Recovery alone does not erase the removal event.

Additional checks: wrong calibration units, unsupported firmware, expired access, and prevention of old asynchronous updates reappearing after reset.

## 9. S07 Language, voice, and automation

Given: English/Malay dictionaries and simulated voice. Steps: Switch language → main screens, notifications, and AI responses switch too → check temperature through the voice demo → confirm a settings change → continue through text if microphone access is denied or unsupported.

Expected: Repeat and confirm the target and value, then apply the same permissions and capabilities as a normal Command. Text permits completion even if microphone access is denied. The screen states that no real microphone is used. Both new and existing notifications appear in the selected language.

Additional checks: create/edit/stop cooling rules based on weekday, time, or pre-arrival conditions; show the next run time. Simulate occupancy, weather, lifestyle pattern, and location events. Location-dependent rules stop if location consent is denied or withdrawn. Test time-zone changes, crossing midnight, and exact expiry boundaries.

## 10. S08 Contractors and external technicians

Given: A job created by hq-operator for customer-b's unit-other-customer; contractor-a/contractor-b; tech-external-b with unit-other-customer in scope; a valid outsourcing period (IR94/IR102). Steps: HQ offers to contractor-a → contractor-a declines → HQ offers to contractor-b → contractor-b accepts → assigns tech-external-b → technician submits work → contractor returns it → technician resubmits → quality reviewer accepts → customer and HQ confirm completion.

Expected: offered/accepted/assigned/in_progress/submitted/rework_requested/completed states remain consistent under the same jobId. Only the contractor's own team candidates appear. The quality reviewer and worker are different people. Communication is a preview. Contractors cannot control invoices or restrictions.

Additional checks: reject candidates from another company, without required qualifications, or outside the outsourcing period. Reject actions from already-open screens after outsourcing expires. Reassignment revokes the previous assignee's access. Direct URLs and Repository calls must not expose other jobs or customers.

## 11. Common and nonfunctional AT observation points

| Case | Additional checks |
|---|---|
| AT-X01 / AT-X02 | Generic password-reset wording; clear old displays on sign-out; English/Malay; alternatives when voice is denied |
| AT-X03 / AT-X06 | Consistent quality/unit/capability displays across four roles; valid treatment of non-RTO (Rent to Own) units |
| AT-X04 | Route/service/Query boundaries, customer boundaries within one tenant, outsourcing period/company boundaries |
| AT-X05 | Shared data across role switches in one tab, seeds on reload/reset, no external communication |
| AT-X07 | Read status differs from business completion; correlation IDs; notification links; audit logs cannot be edited through UI |
| AT-NFR01 / AT-NFR02 | Automated accessibility plus manual keyboard/screen-reader checks; widths 360/768/1024/1279/1280/1440 CSS pixels; 200% zoom; error/empty states |
| AT-NFR03 / AT-NFR05 | No secrets or real card data; expiry, denial, and recovery from disconnection/conflict/failure |
| AT-NFR04 | Record production build, fixed data volume, and measurement conditions; compare with 200ms/2s targets |
| AT-NFR06 | UI uses interfaces instead of directly importing mock implementations. Check mock input/output and replaceable boundaries. API tests are out of scope |
| AT-NFR07 | Exit codes and evidence for actual type-check/lint/build/required-test scripts |
| AT-NFR08 | UTC storage, display time zone, English/Malay, long translations, amount/unit formats |

## 12. Document verification

For this documentation work, check relative links; company English source SRC-06 → BIZ-01–26 → FR-C/P/T/A/X and NFR mappings; labels for added contractor proposals; duplicate traceability entries and references; four-role boundaries; state transitions; and labels for unverified information. Report these separately from application test results.

## 13. Reference design checks

Reference values are in the [extraction evidence](../00-prepare/sources/reference-style-evidence.json); adopted values are in [UIUX](../03-uiux/UIUXSpecification.md) UX-04/08. The following are additional acceptance cases for future implementation, not completed rendering test results.

| Case | Check/expected result |
|---|---|
| AT-UX-REF01 | Compare tokens and computed styles: primary=#005BEA, page=#F8FBFF, surface=white, foreground=#0D2238 |
| AT-UX-REF02 | Latin body text uses Plus Jakarta Sans; IDs use Geist Mono. English/Malay both use Latin script, so no extra fallback font. Do not automatically apply Bricolage to h1 |
| AT-UX-REF03 | Card radius 14px, control radius 10px, navigation radius 16px, specified card shadow; no reversion to Tailwind defaults |
| AT-UX-REF04 | Desktop sidebar 240px, collapsed 56px; drawer at 1279px, fixed navigation at 1280px; maximum content width 1152px |
| AT-UX-REF05 | Page padding changes from 16/24px to 32/32px; KPIs from two to four columns; secondary column 320px on large screens; no clipping of long text on narrow screens |
| AT-UX-REF06 | Replace small/low-contrast reference text with ADAPT values. Check normal text contrast 4.5:1, visible input borders, and 44px touch areas |
| AT-UX-REF07 | Hover/press/focus appearance; reduced-motion support; drawer Esc handling, background scroll lock, and focus return after closing |
| AT-UX-REF08 | All four roles share the same Shell, fonts, tokens, and cards; role differences do not use inconsistent color systems |

Static color calculations: primary text on white, or white text on primary, has contrast of about 5.70:1. The 90%-opacity primary hover state gives about 4.80:1. The reference success text #059669 on #DCFCE7 gives only about 3.43:1; changing text to #166534 gives about 6.49:1. Reference subtle text at 50% opacity gives about 3.23:1 on white; the adopted 72% gives about 6.46:1. These are sRGB calculations for the specified colors only, not proof that the entire screen meets accessibility standards.

## 14. Additional boundary tests from detailed design

- Schedules: from/to dates, crossing midnight, no selected weekdays, from=to, nonexistent/ambiguous daylight-saving times, overlapping confirmed schedules, outsourcing period boundaries.
- Permissions: minimum information before offer acceptance; unit access after acceptance; minimum history after expiry; reject self-approval by switching Membership with the same userId.
- Saving: Unicode code-point counts, 5MiB image boundary, MIME/content agreement, different required fields for draft/submit, unsaved input retained on CONFLICT.
- Asynchronous processing: test demo boundaries of 30 seconds for Commands, 120 seconds for stale data, 15 minutes for quotes, and 24 hours for advance notice. Preserve release intent; do not send competing release and unresolved application requests together.
- Accounting/environment: full payment amount/currency/reference match, matching baseline/actual boundaries, fixed MRV version, reject retirement before confirmed purchase and duplicate retirement.
- Operation catalog: match input/output and role DD for every logical operation. All operations finish within mocks or local display preferences, with no external connections.

## Additional checks for details added to the company original

AT-*-SRC text in role requirements is authoritative for source-related acceptance criteria. Run these alongside normal N/E/B cases, covering missing, unverified, and processing states as well as success. This is a plan, not execution results.

| Requirement | Additional case | Detailed design | Status |
|---|---|---|---|
| FR-C07 | AT-C07-SRC | DD-C07 | not_run |
| FR-C08 | AT-C08-SRC | DD-C08 | not_run |
| FR-C11 | AT-C11-SRC | DD-C11 | not_run |
| FR-C13 | AT-C13-SRC | DD-C13 | not_run |
| FR-T07 | AT-T07-SRC | DD-T07 | not_run |
| FR-A05 | AT-A05-SRC | DD-A05 | not_run |
| FR-A08 | AT-A08-SRC | DD-A08 | not_run |
| FR-A12 | AT-A12-SRC | DD-A12 | not_run |
| FR-A14 | AT-A14-SRC | DD-A14 | not_run |
| FR-A15 | AT-A15-SRC | DD-A15 | not_run |

## Additional revisit, conflict, and cross-role conditions

The role requirement text is authoritative for AT-C12/P03/P05/P07/T10/T11/A08/A09/A11/A14-R01; register them in acceptance_case_ids in the traceability matrix. None has been run. Check identification of the two DDC-08 targets, state preservation after reassignment, photo redisplay, tracking through commissioning completion, manual payment entry with zero Payment records, two invoices under one contract, policy revisits, and report IDs. For S03, include two source invoices and verify that power/temperature do not return to their previous values after release.

## 0.8.0 Fixed conditions and additional acceptance criteria

Use permissions, clock, and numbers from [fixture-contract.json](fixture-contract.json) as shared preconditions. Strings such as customer-a are used in separate Organization and Membership namespaces in existing tests, but pass only the Membership ID to recipientMembershipIds. The fixture generator distinguishes Organization and Membership IDs through typed references. Start every case from a reset fixture; execute different invalid values in one row as independent cases.

Every AT-FIX-001–036 in [acceptance-fixes.csv](acceptance-fixes.csv) is required. Separate static specification-reference checks from future application acceptance tests. status=not_run does not mean success. If older ATs conflict with new contracts, stop G1; do not ignore either side when creating tests.

1A makes no real HTTP 400/401/403/404/409/429/500 calls. Test DomainError VALIDATION/UNAUTHENTICATED/FORBIDDEN/NOT_FOUND/CONFLICT/RATE_LIMITED/UNAVAILABLE with independent fixtures. The production HTTP mapping is a required D11 artifact; these are not settled HTTP contracts. Use separate fixtures for network disconnect, IoT offline, device response timeout, and sensor missing/suspect/stale.

## Additional acceptance criteria after independent review

AT-IND-001–007 in [acceptance-independent.csv](acceptance-independent.csv) are regression cases for independent AI review IND-001–007. Run all in addition to the existing 182 bundles and 36 AT-FIX cases. This document review checks only static contract consistency; application test results remain not_run.

## 0.9.0 Strict review tracking

The [additional acceptance plan](acceptance-strict-review.csv) contains 21 cases specifically for STRICT-DOC-0.8.0. All 21 are specified. The user approved SR17–19 on 2026-09-16. All application execution remains not_run. Do not confuse them with older REV-001 entries. Static checks of revised documents are separate from passing business-behavior tests.

## 0.10.0 Repeated review acceptance plan

Added [16 re-review and repeated-fix cases](acceptance-rereview.csv) to traceability. These are design-stage Given/When/Then definitions, not execution results. Document completion is judged by consistency of types, catalogs, authorization, state transitions, and expected acceptance results. An unimplemented application is not counted as a design defect.

## 0.11.0 Re-review fixes and repeated checks

[acceptance-resolution.csv](acceptance-resolution.csv) covers 18 INDEPENDENT-DOC-0.10.0 items (17 specified and one capacity-guarantee candidate marked deferred_candidate) plus six self-review items found after corrections. All 24 are post-implementation test plans with execution_status=not_run. Current checks cover static link/table/catalog/traceability/type consistency and desk checks of scenarios. Keep the capacity-guarantee candidate separate from unfixed defects in existing features and carry forward IR18 limits.

## 0.12.0 Additional repeated review

The [eight-case acceptance plan](acceptance-convergence.csv) tracks full restriction scope, retries after acceptance, limited-projection sorting, reminder previews, accumulation slots, inspection saving, Fact evaluation, and raw measurement times. Keep the previous 24 cases too. Run document consistency and type checks; application acceptance tests remain not_run.

0.13.0: Fixed five re-review findings about public job projections. The [acceptance plan](acceptance-projection.csv) tracks list search/counts, history after expiry, summaries, and access expiry during pagination. Application tests have not been run.

0.15.0: Add all [independent re-review regression cases](acceptance-independent-g1.csv). Split AT-P01-N before/after filtering; use concrete examples for completion times, severity, and self-approval of jointly edited reports. Application tests are not_run.

## 0.17.0 Additional acceptance checks from independent review (FRV)

Check AT-REV17-001–015 in the [additional acceptance plan](acceptance-review-017.csv) together with existing ATs. Expected results cover release-request entry paths, clock jumps/sessions, transport failure injection, job expiry, archived resources, customer counts, presets, role projections, Sensor creation, display formats, translation fallback, rendering exceptions, not-found, and installedAt=null. Use IR37 demo.trigger for transport/network injection; "make … UNAVAILABLE" in acceptance criteria means this injection. Under IR36, do not log in again after a 24-hour clock advance in acceptance steps. All application execution remains not_run.

## 0.16.0 Additional re-review acceptance checks

Check the [additional acceptance plan](acceptance-review-016.csv) with existing ATs. User answers settled DEC-17/18, defining expected results for four permission combinations, all ten states, other sorts, and URL restoration. All application execution remains not_run.

## 0.18.0 Additional strict review acceptance checks (REV18)

Check AT-REV18-001–048 in the [additional acceptance plan](acceptance-review-018.csv) with existing ATs. Initial business data is demoSeed in [fixture-contract.json](fixture-contract.json); apply each AT's Given as IR69 patches. For cases that fix measurement values/times, first set simulator enabled=false (IR45). Interpret acceptance timestamps without Z/offset as Asia/Kuala_Lumpur local time (IR74). All application execution remains not_run.

## 0.19.0 Additional independent review acceptance checks (REV19)

Check AT-REV19-001–042 in the [additional acceptance plan](acceptance-review-019.csv) with existing ATs. Expand demoSeed shorthand into canonical DTOs under IR91; use `acceptancePatches` in [fixture-contract.json](fixture-contract.json) for case-specific fixed changes (IR85). Coverage includes technician screens before/after work windows (IR76/IR89), simulator copy conditions (IR77), admin energy-saving forecasts (IR78), and refetch displays (IR83). Assess AT-REV18-013①'s "permission-denied display (start-time guidance)" using IR76 work-not-started, and AT-REV18-019 using IR78 energyForecast. All application execution remains not_run.

## 0.21.0 Additional acceptance checks for independent G1 findings

Check every [AT-G121-001–005](acceptance-review-021.csv) with existing acceptance cases. For A12 duration, distinguish 59 seconds from 60 seconds using ordinary IR103 ticks. Include notification type/severity, allergen Query refetch, and notification fixture scopeVersionAtCreation. Passing document validators and mutation tests is not an application acceptance result.
