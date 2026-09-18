# Strict Review Findings STRICT-DOC-0.17.0-REV18 (2026-09-17, Interrupted Record)

Scope: DOC-0.17.0 baseline `654be5e4…d8df`. Reviewer: the AI in the previous session (Fable 5.1). Corrections (IR45–74, DEC-25–41, acceptance plan AT-REV18-001–048) were applied to the working tree, but the session was interrupted before execution records, self-review, or a baseline were created. This document preserves the finding data recovered from that interrupted session without changes. gate-G1 is not_evaluated; no spec-manifest was created (IR75). Correction validation and handover were performed in independent review REV19 ([DOC-0.19.0](../DOC-0.19.0/review.md)). Application tests are not_run.

Initial assessment: NOT READY (2 BLOCKER, 4 CRITICAL, 24 MAJOR, 16 MINOR, 2 QUESTION).

## BLOCKER (2)

### REV18-001
- Severity: BLOCKER
- Document: Deterministic contracts D07/D09, IR36, SR27, fixture.powerClassification
- Location: Demo-clock progression, operation classification, stale checks
- Problem: The demo clock advances in real time, but no mechanism is defined to generate synthetic measurements or heartbeat signals over time. The last seed observation is 00:59:30Z. Under SR27's 120-second rule, all units become stale/operation unknown about 90 seconds after startup.
- Why it matters: 1A primarily provides a clickable monitoring demo. C01/A01/T03 dashboards stop working soon after startup. Implementers may choose random generation, no periodic generation, or automatic offline transitions.
- Example Failure: Two minutes after demo startup, the customer dashboard shows “Running 0 / Stopped 0 / Unknown 4,” making the stakeholder demo unusable. Another implementation uses random values that change acceptance expectations each time.
- Required Fix: Define whether generation occurs, its interval and values, gap filling on jumps, how to stop it, and how to fix it during acceptance tests.
- Suggested Revision: IR45: At each UTC minute boundary, copy the latest value for each Sensor of online Devices (no randomness). Do not fill gaps on jumps. Use DemoTrigger simulator for ON/OFF. Heartbeat fields do not change version.
- Status: corrected_unverified (IR45; validation in DOC-0.19.0)

### REV18-002
- Severity: BLOCKER
- Document: Deterministic contract D03, SR05, DDC-08 §4, FR-C03/C12/A09/T10
- Location: Allowed operations while restricted (effectiveControlPolicy=restricted)
- Problem: No table defines which UnitActions (set_power/set_temperature/set_mode/set_fan/ventilate) are allowed or rejected while temperature_limit or power_off applies. AT-C03-E④ covers only the minimum temperature.
- Why it matters: This is a core unpaid-restriction rule in BIZ-21. Different interpretations of allowed actions can bypass restrictions, creating a security defect.
- Example Failure: While power_off applies, a customer sends set_power true. One implementation accepts it and cooling resumes, defeating the restriction. Voice, automation, and test-run paths give different results.
- Required Fix: Define policy type × UnitAction permissions, all applicable paths, error codes, and UI candidate display in one table.
- Suggested Revision: Apply the IR46 table to every path: temperature_limit returns FORBIDDEN only for temperatures below the lower limit; power_off returns FORBIDDEN for everything except set_power false.
- Status: corrected_unverified (IR46; validation in DOC-0.19.0)

## CRITICAL (4)

### REV18-003
- Severity: CRITICAL
- Document: Common detailed design §3, canonical ACUnit/Device types, D01/D05
- Location: Relationship between ACUnit.connection/lastSeenAt and Device state
- Problem: Both ACUnit and Device have connection/lastSeenAt, but the source of truth and derivation are undefined. It is also undefined whether powerSignal=off or tamper=detected affects control availability.
- Why it matters: OFFLINE in AT-C03-E② changes the Unit value, while communication loss in T12 changes Device values. Implementers may disagree on whether device communication loss stops control.
- Example Failure: Sending communication_lost to device-online-rto leaves Unit.connection=online, so customer control is treated as successful.
- Required Fix: Define Unit derivation, unbound values, OFFLINE conditions and messageKey, and tamper handling.
- Suggested Revision: IR47: Derive Unit state from the currently bound Device; unbound is unknown. Anything other than online, or powerSignal=off, gives OFFLINE. Tamper shows a warning without blocking control.
- Status: corrected_unverified (IR47; validation in DOC-0.19.0)

### REV18-004
- Severity: CRITICAL
- Document: Common detailed design §5, DDC-04, IR23, FR-P02/A06
- Location: Expiry of an unanswered Offer
- Problem: For an unanswered Offer past offerExpiresAt, only disappearance from the contractor list is defined; there is no Job.status transition. Since the only offer transition is requested→offered, HQ cannot offer the job again.
- Why it matters: In the S08 outsourcing flow, a contractor failing to respond leaves cancellation as the only way forward.
- Example Failure: After the Offer expires, the job stays offered. HQ jobs.offer returns CONFLICT and contractor accept also returns CONFLICT, leaving only cancellation.
- Required Fix: Define the Job transition at expiry, Offer retention, whether reoffering is allowed, and whether a notification is sent.
- Suggested Revision: IR48: At expiry, return Job to requested with contractorOrgId=null. Retain the Offer, send no notification, and allow reoffering with a new offerId.
- Status: corrected_unverified (IR48; validation in DOC-0.19.0)

### REV18-005
- Severity: CRITICAL
- Document: Deterministic contract D06, IR23/IR24, FR-T01, AT-T01-N
- Location: Assignment validity period and technician visibility
- Problem: D06 makes Assignment validFrom/Until equal to the scheduled window, but visibility before scheduled start is undefined. FR-T01 requires planned-work display.
- Why it matters: The technician dashboard schedule, job.assigned notification links, and advance preparation cannot work.
- Example Failure: A technician receives a 10:00 assignment at 08:00 and opens its notification, which returns NOT_FOUND. T01's planned-job count is also zero.
- Required Fix: Define separate viewing and working periods, operations rejected before start, and differences between internal and external technicians.
- Suggested Revision: IR49: Viewing window=[assignment creation, scheduled end); work window=[scheduled start, scheduled end). Before start, job-based operations and external technicians' equipment reads return FORBIDDEN.
- Status: corrected_unverified (IR49; validation in DOC-0.19.0)

### REV18-006
- Severity: CRITICAL
- Document: Screen catalog url_selection, SR11, AT-C01-N③, AT-A01-N④
- Location: Navigation from KPIs to lists and allowed URL keys
- Problem: No customer equipment-list route exists for AT-C01-N's running-card navigation. SR11 does not enumerate common filter/sort/period keys. Keys absent from url_selection, such as propertyId/powerState/unreadOnly, are removed by the unknown-key rule.
- Why it matters: Main dashboard navigation and filter restoration with Back (multiple ATs) depend on implementer guesses and cannot be tested.
- Example Failure: /admin/units?powerState=on loses powerState because it is not allowlisted, so KPI and list counts differ.
- Required Fix: Define destination routes, common keys, screen-specific extra keys, and normalization of multiple values and booleans.
- Suggested Revision: IR50 and additional screen-catalog url_selection entries (C01/C02/C06/C07/C08/C10/T01/T02/P01/P06/A01/A02/A16).
- Status: corrected_unverified (IR50; validation in DOC-0.19.0)

## MAJOR (24)

### REV18-007
- Severity: MAJOR
- Document: D07, AT-C08-B, IR30
- Location: Population for unresolved-alert counts
- Problem: D07 counts open/acknowledged Alerts, while AT-C08-B excludes severity=normal from the unresolved count. Which value “unresolved count” refers to is also undefined.
- Why it matters: KPI counts conflict with acceptance expectations, so one test must fail.
- Example Failure: One open normal Alert gives C01 an alert count of one, but AT-C08-B expects zero.
- Required Fix: Define the counted population (severity/status/archived) and distinguish it from unread notification counts.
- Suggested Revision: IR51: Count only open/acknowledged Alerts with critical/warning severity.
- Status: corrected_unverified (IR51; validation in DOC-0.19.0)

### REV18-008
- Severity: MAJOR
- Document: FR-C05, DD-C05, canonical Condition type, IR16
- Location: Evaluation of lifestyle-pattern conditions (pattern)
- Problem: Condition includes pattern{localTime}, but it is absent from Fact types and the TTL table, and its match timing is undefined.
- Why it matters: Even if implemented as an FR-C05 option, the implementer must invent when it runs.
- Example Failure: One implementation fires immediately after saving; another never fires.
- Required Fix: Define matching conditions (time/timezone/frequency), jump handling, DST validation, and display labels.
- Suggested Revision: IR52: A synthetic condition that matches once daily at the localTime tick in the rule's timezone.
- Status: corrected_unverified (IR52; validation in DOC-0.19.0)

### REV18-009
- Severity: MAJOR
- Document: FR-C05, IR27, canonical RuleBase.disabledReason type
- Location: Rule state when consent is withdrawn
- Problem: Withdrawing consent is said to disable the rule, but it is undefined whether enabled changes or evaluation is only suppressed. disabledReason, version increments, and restoration on renewed consent are undefined. The disabledReason type lacks a matching value.
- Why it matters: AT-C05-N④ has no single observable definition of “the rule is disabled.”
- Example Failure: Renewing consent automatically re-enables the rule and restarts location-based operation the user did not intend.
- Required Fix: Define the withdrawal transition, reason value, and handling of renewed consent.
- Suggested Revision: IR53: In the same transition as withdrawal, set enabled=false, consent_revoked, and version+1. Renewed consent does not restore it automatically.
- Status: corrected_unverified (IR53; validation in DOC-0.19.0)

### REV18-010
- Severity: MAJOR
- Document: DD-C04 step 3, deterministic contract D02
- Location: Schedule firing path
- Problem: DD-C04 says to pass demo-clock events to automations.fire, while D02 says internal clock evaluation does not use the viewing session and internal functions are not exposed to UI.
- Why it matters: If the UI calls fire, schedules do not run unless the customer screen is open.
- Example Failure: While logged in as HQ, a customer's 18:00 schedule does not fire, failing AT-C04-N③.
- Required Fix: Choose one owner for clock-based firing and define the role of UI fire.
- Suggested Revision: IR54: Clock firing uses internal evaluation only. UI fire is an optional demo action sharing the same tick result. Correct DD-C04.
- Status: corrected_unverified (IR54; validation in DOC-0.19.0)

### REV18-011
- Severity: MAJOR
- Document: D09, NFR-01 (WCAG 2.2 AA)
- Location: Session expiry and extension
- Problem: A fixed 30-minute lifetime has no extension or advance warning, and expiry discards unsaved drafts. This conflicts with NFR-01's WCAG 2.2 SC 2.2.1 target (a way to extend the time limit and at least 20 seconds to act).
- Why it matters: Keyboard and assistive-technology users lose report input without warning, failing a11y acceptance (D10).
- Example Failure: A screen-reader user loses a 29-minute inspection draft just before saving.
- Required Fix: Define warning timing, extension operation and count, whether auditing occurs, and behavior at expiry.
- Suggested Revision: IR55: Show an alertdialog 120 seconds before expiry; demoSession.extend adds 30 minutes without a limit on extensions.
- Status: corrected_unverified (IR55; validation in DOC-0.19.0)

### REV18-012
- Severity: MAJOR
- Document: Common detailed design §5, D06, operation-catalog jobs.cancel
- Location: States in which job cancellation is allowed
- Problem: It is undefined whether HQ may cancel requested jobs, directly cancel in_progress/submitted jobs, or what happens to pending Offers on cancellation.
- Why it matters: State-transition implementations differ, and some may let contractors accept after cancellation.
- Example Failure: HQ directly cancels a submitted job, losing it before the submitted version receives quality review.
- Required Fix: Define a role × state cancellation table and results.
- Suggested Revision: The IR56 table.
- Status: corrected_unverified (IR56; validation in DOC-0.19.0)

### REV18-013
- Severity: MAJOR
- Document: DDC-03, introductions to role-specific DDs, D10, SCR-X-forbidden
- Location: FORBIDDEN/NOT_FOUND display and navigation
- Problem: Four behaviors are described: navigate to home/list, return to an available screen, show permission-denied in place, and use /forbidden. The choice is undefined.
- Why it matters: Navigating after write FORBIDDEN (such as a restriction violation) loses input, violating NFR-05 input retention.
- Example Failure: A customer submits temperature 23, gets FORBIDDEN, and is sent home without seeing the reason or entered value.
- Required Fix: Define display and navigation separately for routes, primary queries, secondary queries, and writes.
- Suggested Revision: IR57.
- Status: corrected_unverified (IR57; validation in DOC-0.19.0)

### REV18-014
- Severity: MAJOR
- Document: operation-catalog notifications.list, IR19, FR-C08 BR
- Location: Notification list visibility
- Problem: The catalog filters by current target scope, while FR-C08 displays unavailable targets as “Unavailable.” Exclusion versus masked display conflicts.
- Why it matters: Counts and unread totals vary by implementation. Keeping such notifications can reveal targets whose read access was lost.
- Example Failure: Notifications for equipment removed from the assignment remain in the list and reveal equipment names in subjects.
- Required Fix: Define list, count, and link-resolution handling.
- Suggested Revision: IR58: Exclude from list/total/unread count; show “Unavailable” only for already displayed links.
- Status: corrected_unverified (IR58; validation in DOC-0.19.0)

### REV18-015
- Severity: MAJOR
- Document: D04, canonical DemoTrigger type, payments.simulate, IR35
- Location: Payment confirmation paths and actors
- Problem: Customer attempts can be confirmed through both payments.simulate(confirm) and demo.trigger(payment), with no defined relationship. Confirmation through public /demo has no IR35 audit actor (the payment-confirming actor).
- Why it matters: Idempotency, version checking, and audit rules differ across paths; the same payment is handled twice.
- Example Failure: Confirming without login through /demo leaves the audit actor empty, so the restriction-release initiator cannot be traced.
- Required Fix: Use a single path and define the audit actor for clock/demo-origin business changes.
- Suggested Revision: IR59: Remove DemoTrigger payment. The system actor is system-demo with actorRoleAtTime=system.
- Status: corrected_unverified (IR59; validation in DOC-0.19.0)

### REV18-016
- Severity: MAJOR
- Document: operation-catalog frontend_execution, NFR-06, D11
- Location: Identifying demo-only operations
- Problem: Operations that must not reach production, such as payments.simulate allowing customers to confirm their own payments, are listed as mock-service alongside business Repository interfaces without distinction.
- Why it matters: A future adapter Agent may map the same interface to production APIs, creating serious permission defects such as customer-confirmed payments.
- Example Failure: A production adapter implements payments.simulate(confirm) unchanged, letting unpaid customers release their own restrictions.
- Required Fix: Mark demo-only operations in machine-readable form and define UI display and D11 replacements.
- Suggested Revision: IR60: Assign frontend_execution=demo-only to 11 operations and display DEMO labels.
- Status: corrected_unverified (IR60; validation in DOC-0.19.0)

### REV18-017
- Severity: MAJOR
- Document: FR-C10 post-completion business state, DD-C10 step 3
- Location: Non-RTO contract display
- Problem: FR-C10 says to show non-RTO contracts as “No contract / general maintenance.” DD-C10 says to show that only with zero contracts, and show type/period/invoices for non-RTO contracts.
- Why it matters: Requirements and design demand opposite behavior.
- Example Failure: A customer with a general-maintenance contract sees “No contract” and cannot see invoices.
- Required Fix: Use one display condition.
- Suggested Revision: IR61: “No contract” only when there are zero contracts; correct FR-C10.
- Status: corrected_unverified (IR61; validation in DOC-0.19.0)

### REV18-018
- Severity: MAJOR
- Document: FR-C02, canonical ACUnit.spaceId type, units.save
- Location: Equipment with no assigned space
- Problem: FR-C02 requires unassigned equipment to be identifiable, and ACUnit.spaceId is nullable, but units.save requires spaceId. Unassigned equipment cannot be created and there is no list filter for it.
- Why it matters: Required display targets cannot be generated or tested.
- Example Failure: The unassigned group is always empty, or implementations wrongly count only the current page.
- Required Fix: Allow null on save and define a condition to retrieve only unassigned equipment.
- Suggested Revision: IR62: units.save.spaceId becomes ID|null; add unassignedOnly to units.list.
- Status: corrected_unverified (IR62; validation in DOC-0.19.0)

### REV18-019
- Severity: MAJOR
- Document: FR-A01, DD-A01, canonical AdminSummary.energySummary type, admin.summary
- Location: Savings on the admin dashboard
- Problem: FR-A01 displays estimated savings, but admin.summary input does not select a baseline and the calculation baseline is undefined.
- Why it matters: Implementations either always return null savings or guess a baseline.
- Example Failure: An implementation chooses the first baseline found and displays savings against another customer or boundary.
- Required Fix: Define baseline selection and display when none matches.
- Suggested Revision: IR63: Automatically select the latest baseline matching the unit set, boundary, and minute count; otherwise baseline_unavailable.
- Status: corrected_unverified (IR63; validation in DOC-0.19.0)

### REV18-020
- Severity: MAJOR
- Document: DD-C09 contactWindow, IR42, D06
- Location: Validation and visibility of contact windows
- Problem: “No real contact details” has no validation method and cannot be tested. Visibility to contractors/technicians is also undefined and may conflict with D06's rule against returning customer contact details to contractors.
- Why it matters: A phone number in free text passes personal information to external technicians.
- Example Failure: “call 012-345-6789” is saved and displayed to a contractor before acceptance.
- Required Fix: Define input-validation rules and visibility by role.
- Suggested Revision: IR64: Reject @ and runs of seven or more digits. Show only to the customer, HQ, assignees within the viewing window, and accepted contractors.
- Status: corrected_unverified (IR64; validation in DOC-0.19.0)

### REV18-021
- Severity: MAJOR
- Document: D09 voice grammar, FR-X02, AT-X02
- Location: Voice room-name matching and syntax
- Problem: It is undefined whether <room> matches Space names or Unit display names, how multiple units in a room are handled, and how room names containing “to” are parsed.
- Why it matters: AT-X02's expected result for two rooms with the same name cannot be fixed.
- Example Failure: For “set Living room to 24 degrees,” one implementation matches equipment names and finds none, while another silently selects the room's first unit.
- Required Fix: Define matching targets, candidate generation, regular expressions, and out-of-scope handling.
- Suggested Revision: IR65.
- Status: corrected_unverified (IR65; validation in DOC-0.19.0)

### REV18-022
- Severity: MAJOR
- Document: FR-T07, AT-T07-N, D08
- Location: Resolution and recurrence of Alerts without a policy
- Problem: AT-T07-N recovers an evidence=inferred Alert by remeasurement, but D08 automatic recovery depends on policy recoveryThreshold. Alerts with policyId=null have no recovery condition. The “same event” rule for previousAlertId on recurrence is also undefined.
- Why it matters: Acceptance criteria cannot be executed, and recurrence linking varies by implementation.
- Example Failure: The seed window_open Alert never resolves automatically, failing AT-T07-N③.
- Required Fix: Define automatic-recovery targets, same-event keys, and duplicate creation while unresolved; correct ATs.
- Suggested Revision: IR66 and correction of AT-T07-N.
- Status: corrected_unverified (IR66; validation in DOC-0.19.0)

### REV18-023
- Severity: MAJOR
- Document: D05, common detailed design §5, DemoTrigger operation
- Location: Device operation queued→running
- Problem: DeviceOperation is defined as queued→running→succeeded/failed, but no trigger moves queued to running. The check connecting state cannot be reproduced.
- Why it matters: The IoT Device connecting state, a required review item, cannot be tested.
- Example Failure: One implementation leaves operations queued until TIMEOUT at 60 seconds; another sets running immediately on creation.
- Required Fix: Define the start trigger, rechecks, and states that accept result events.
- Suggested Revision: IR67: Set running at the tick one second after creation; check becomes connecting.
- Status: corrected_unverified (IR67; validation in DOC-0.19.0)

### REV18-024
- Severity: MAJOR
- Document: Common detailed design §7, FR-C06, FR-A13, DD-A13
- Location: Display of negative savings
- Problem: Common design displays negative values as increases, FR-C06 says “20% increase,” while FR-A13 and AT-A13 show “-20kWh/-20%.” The same value has conflicting displays.
- Why it matters: A shared EnergyChart would need separate role rules, and expected acceptance strings conflict.
- Example Failure: A13 displays “Increase 20.0%,” failing AT-A13-B's string comparison.
- Required Fix: Separate DTO values and display formatting and define one rule.
- Suggested Revision: IR68: DTO values are signed; all roles display “Increase {absolute value}.”
- Status: corrected_unverified (IR68; validation in DOC-0.19.0)

### REV18-025
- Severity: MAJOR
- Document: AT-A06-N, common detailed design §5
- Location: Maintenance job state sequence
- Problem: AT-A06-N expects assigned→submitted, skipping in_progress. The transition table does not allow assigned directly to submitted.
- Why it matters: Implementing the acceptance criterion removes state-transition checks.
- Example Failure: The Test Agent passes an implementation that allows submission without in_progress.
- Required Fix: Align the expected state sequence with the transition table.
- Suggested Revision: Add in_progress to AT-A06-N.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-026
- Severity: MAJOR
- Document: fixture-contract.json, validation plan §2
- Location: Demo seed business data and test-specific overrides
- Problem: Fixtures contain only actors and equipment ownership, with no initial invoice/contract/restriction/job/Alert/capability/measurement business data. There is also no test-only override interface to apply acceptance Given conditions.
- Why it matters: The implementation Agent invents seed data, changing demo appearance and acceptance premises between implementations.
- Example Failure: An implementer chooses invoice-overdue-a amounts and dates, creating a demo with values different from AT-C10-N.
- Required Fix: Define the authoritative initial data and how tests apply overrides.
- Suggested Revision: IR69 and fixture-contract.json demoSeed.
- Status: corrected_unverified (IR69; validation in DOC-0.19.0)

### REV18-027
- Severity: MAJOR
- Document: D07 members.capacity
- Location: Days off in available work hours
- Problem: “Days off are empty” does not define whether days off are weekends only or include public holidays (for example Malaysia Day on 2026-09-16).
- Why it matters: Expected utilization changes by date and depends on the implementer's calendar.
- Example Failure: Implementations treating 2026-09-16 as a holiday versus a workday produce different utilization.
- Required Fix: Define days off.
- Suggested Revision: IR70: Saturdays and Sundays only; no public-holiday calendar.
- Status: corrected_unverified (IR70; validation in DOC-0.19.0)

### REV18-028
- Severity: MAJOR
- Document: D07 events.subscribe, canonical ChangeEvent type, common detailed design §6
- Location: Change-event types and Query invalidation
- Problem: ChangeEvent.entityType and subscribe resources are strings with no value list or Query invalidation mapping. Common design vaguely says “jobs/units/invoices/restrictions, etc.”
- Why it matters: Repository and UI Agents may use different names, preventing updates from appearing.
- Example Failure: Repository sends job while UI subscribes to jobs, so job state changes are not displayed.
- Required Fix: Define entityType values and a mapping to read operations to invalidate.
- Suggested Revision: IR71 and canonical ChangeEntityType.
- Status: corrected_unverified (IR71; validation in DOC-0.19.0)

### REV18-029
- Severity: MAJOR
- Document: README, end-of-document “takes priority over old text,” Agentic SDLC §7
- Location: Normative priority and remaining old text
- Problem: Priority exists only in prose at document ends, leaving readers to decide what counts as the same topic. Superseded text remains, such as DD-A13 boundary length 1–2000 versus IR11 1–500, and required baselineKWh versus SR29.
- Why it matters: An implementation Agent reading 57 files may follow old text, producing differences between Agents.
- Example Failure: UI allows 2000 characters for boundary descriptions, but Repository returns VALIDATION above 500.
- Required Fix: Define a priority table, remove superseded text, and detect reintroduction.
- Suggested Revision: IR72, DD-A13 and other corrections, and validator checks for old phrases.
- Status: corrected_unverified (IR72; validation in DOC-0.19.0)

### REV18-030
- Severity: MAJOR
- Document: tools/check_review_regressions.py, runs/DOC-0.17.0
- Location: Validator mutation tests
- Problem: Mutation tests look for an old baseline (DOC-0.16.0 manifest text), stop with “Mutation target missing,” and cannot run on the current version. The 0.17.0 handover records only static-validation success.
- Why it matters: Validator regressions cannot be detected, and handover evidence does not match reality.
- Example Failure: Accidentally removing part of a check goes unnoticed because mutation tests cannot run.
- Required Fix: Update mutation targets and require both checks to pass for handover.
- Suggested Revision: IR73 and correction of check_review_regressions.py.
- Status: corrected_unverified (IR73; validation in DOC-0.19.0)

## MINOR (16)

### REV18-031
- Severity: MINOR
- Document: DD-C01 summary field
- Location: Operation-status card fields
- Problem: The summary field lists total/online/offline/unknown/alertCount but lacks SR27's powerOn/powerOff/powerUnknown.
- Why it matters: Implementations may build operation-status cards from communication state.
- Example Failure: Displays “online 2” instead of “Running 1.”
- Required Fix: Align fields with SR27/IR51.
- Suggested Revision: IR74 and correction of DD-C01.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-032
- Severity: MINOR
- Document: DD-C08/DD-T01 severity
- Location: Sending the UI value all
- Problem: How UI severity=all maps to Query is undefined; sending it unchanged returns VALIDATION under D12.
- Why it matters: Implementations may always fail to load the list.
- Example Failure: Initial display is a VALIDATION error screen.
- Required Fix: Define all as omitted.
- Suggested Revision: IR74.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-033
- Severity: MINOR
- Document: DD-A03
- Location: Whether validFrom is required
- Problem: The table says conditionally required, but the canonical type always requires it.
- Why it matters: Form required-field indicators disagree with the type.
- Example Failure: Submitting without validFrom returns VALIDATION.
- Required Fix: Always require validFrom; require validUntil only for external technicians.
- Suggested Revision: IR74 and correction of DD-A03.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-034
- Severity: MINOR
- Document: D10 supported environments, DEC-12
- Location: Tablet environment
- Problem: DEC-12 adopts tablet support, but the supported environments include no tablet OS.
- Why it matters: Tablet acceptance environment is unsettled.
- Example Failure: Work is marked complete without iPad verification.
- Required Fix: Add a tablet environment.
- Suggested Revision: IR74 and correction of D10 (iPadOS 17 Safari).
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-035
- Severity: MINOR
- Document: AT-C09-N, AT-P03-N, AT-C06-N
- Location: Timezones in acceptance timestamps
- Problem: Some timestamps, such as “2026-09-15 10:00–12:00,” have neither Z nor an offset.
- Why it matters: Expected values differ by nine hours between UTC and local time.
- Example Failure: Past/future boundary expectations for the requested window are reversed.
- Required Fix: Define timestamp notation.
- Suggested Revision: IR74 and corrections to the relevant ATs.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-036
- Severity: MINOR
- Document: commands.create, IR09
- Location: Client reason
- Problem: Technician/admin reason is required, but handling of a reason sent by a client is missing.
- Why it matters: Input validation differs by implementation.
- Example Failure: One implementation audits the client reason; another rejects it.
- Required Fix: Specify that clients do not send it.
- Suggested Revision: IR74.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-037
- Severity: MINOR
- Document: DDC-04
- Location: Mock read delay
- Problem: “A fixed setting between immediate and 300ms” does not define a single value.
- Why it matters: Performance measurement premises vary.
- Example Failure: Measuring at 0ms overstates NFR-04 performance.
- Required Fix: Set it to a fixed 300ms.
- Suggested Revision: IR74 and correction of DDC-04.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-038
- Severity: MINOR
- Document: DD-A01/A13/P01/T01
- Location: Period input types
- Problem: Whether A01/A13 have presets and how P01/T01 dates convert to Instant are undefined.
- Why it matters: Period boundaries differ by implementation.
- Example Failure: P01 interprets “September 14” as starting at 00:00Z, shifting it by eight hours.
- Required Fix: Define presets and conversion rules.
- Suggested Revision: IR74 and corrections to DD tables.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-039
- Severity: MINOR
- Document: AT-T03-N/E
- Location: Ordering key
- Problem: The text says version=3 followed by 2, but Measurement.version is assigned by the Repository and cannot be input. RawMeasurement has sequence.
- Why it matters: Test input cannot be expressed by the type.
- Example Failure: The Test Agent creates a nonexistent input.
- Required Fix: Change it to sequence.
- Suggested Revision: Correction of AT-T03.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-040
- Severity: MINOR
- Document: DemoTrigger device, DD-T12
- Location: Deriving evidenceSource
- Problem: DeviceEvent.evidenceSource is required, but DemoTrigger has no input for it, while DD-T12 describes it as required input.
- Why it matters: Inconsistent combinations can be created, such as tamper with heartbeat evidence.
- Example Failure: power_lost is saved with heartbeat evidence.
- Required Fix: Define derivation from event type.
- Suggested Revision: IR74 and correction of DD-T12.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-041
- Severity: MINOR
- Document: Space.kind, AT-C02-B①
- Location: Location nesting rules
- Problem: Whether area/floor/room/space have nesting-order constraints is undefined (the AT allows a floor under a floor).
- Why it matters: Implementers may add an order constraint that fails the AT.
- Example Failure: Creating a floor under a floor is rejected, failing AT-C02-B①.
- Required Fix: Explicitly state that there is no order constraint.
- Suggested Revision: IR74.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-042
- Severity: MINOR
- Document: DD-A02 overview
- Location: tenantId input
- Problem: The text says to enter tenantId, conflicting with D14 (derived from Session; UI input is untrusted).
- Why it matters: A wrong implementation can create a cross-tenant registration path.
- Example Failure: The form allows selecting tenant-b.
- Required Fix: State that it is not entered.
- Suggested Revision: Correction of DD-A02.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-043
- Severity: MINOR
- Document: FR-A04, FR-A12
- Location: Permission names
- Problem: “Permission to manage models” and “permission to manage environment policies” are not defined permission names.
- Why it matters: Permission checks require guesses.
- Example Failure: Nonexistent permission names are added to fixtures.
- Required Fix: Specify device.manage / automation.policy.manage.
- Suggested Revision: Correction of FR-A04/A12.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-044
- Severity: MINOR
- Document: NFR-03
- Location: Reconfirmation at expiry
- Problem: It does not say what expires or what must be rechecked.
- Why it matters: It cannot be tested.
- Example Failure: Submission proceeds after the version changes while a confirmation dialog remains open.
- Required Fix: Define behavior when state changes during confirmation.
- Suggested Revision: IR74 and correction of NFR-03.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-045
- Severity: MINOR
- Document: PrepareDocument §1.3
- Location: Definitions of 1A/1B
- Problem: The 1A/1B phase names are used without mapping them to Phase 1/2 in the company source.
- Why it matters: Phase meanings differ in scope discussions.
- Example Failure: 1B is mistaken for Phase 2 (HVAC).
- Required Fix: State the mapping.
- Suggested Revision: Addition to PrepareDocument §1.3.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-046
- Severity: MINOR
- Document: Screen catalog states
- Location: Device state list
- Problem: connecting/device-* states are listed even for screens without devices, such as contracts/login.
- Why it matters: Unnecessary state implementation and testing result.
- Example Failure: A device-offline display is built on an invoice screen.
- Required Fix: Limit them to IoT screens.
- Suggested Revision: IR74 and correction of the states column.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

## QUESTION (2)

### REV18-047
- Severity: QUESTION
- Document: UX-06, NFR-08
- Location: Author and reviewer of Malay wording
- Problem: It is undefined who writes and reviews the ms dictionary.
- Why it matters: AI machine translation can reach the company demo without review.
- Example Failure: Mistranslated business terms appear on customer-facing screens.
- Required Fix: Assign writing and review responsibilities.
- Suggested Revision: DEC-40 (PROPOSED): Implementation Agent drafts; Business/UI/UX reviews before company acceptance.
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

### REV18-048
- Severity: QUESTION
- Document: UX-04/UX-05, PrepareDocument §1.2
- Location: Application name and header text
- Problem: The application name is undefined, risking reuse of the reference site's name/logo (Aconland).
- Why it matters: Misuse of a third-party brand.
- Example Failure: The reference site's name appears in the header.
- Required Fix: Choose a neutral name key.
- Suggested Revision: DEC-41 (PROPOSED): app.name (AC Monitoring Demo / Demo Pemantauan AC).
- Status: corrected_unverified (IR74; validation in DOC-0.19.0)

