# Independent Review Report INDEPENDENT-DOC-0.16.0-FRV (2026-09-16)

Scope: DOC-0.16.0 baseline `fe251845…e0b2` (PrepareDocument, five requirements documents, five detailed designs plus four contracts and four catalogs, UIUX specification plus two catalogs). Reviewer: the AI in this conversation (Fable 5.1). The same reviewer made the corrections, so the follow-up is a **self-review**, not an independent G1 decision (gate-G1.yaml=pending). Application implementation/runtime tests are not_run. Production HTTP/authentication/DB/real devices are outside 1A under DEC-12 and D11 and are not counted as blockers in this report.

# 1. Executive Review Summary

Initial assessment: **NOT READY (2 CRITICAL, 8 MAJOR, 15 MINOR)**. Self-review after corrections: zero unresolved findings, 64 requirements OK in the traceability table, static validation and TypeScript strict successful. Independent G1 awaits another reviewer.

The two main issues were: (1) release_requested is defined as both an automatic transition on payment confirmation (AT-A08-N/AT-C11-N) and an explicit HQ `restrictions.release` action (AT-A09-N). Calling the latter from the preceding state causes D01 state-mismatch CONFLICT. (2) Acceptance procedures that advance `demo.advanceClock` by 24 hours (S03/A09/C12/P02) conflict with D09's 30-minute session lifetime on the demo clock, causing UNAUTHENTICATED midway. IR35/IR36 settle both.

The eight MAJOR findings concern transport-failure injection assumed by acceptance criteria and Repository behavior on network disconnection, dueAt derivation for customer-created jobs, visibility of archived resources, the customer-count population, 1h/24h presets, role-projection rules that hide internal information (decline/reassignment/HQ reasons, internal costs, audit actors), internal notes in customer-facing jobs.events, and Sensor creation during Device registration. IR37–IR43 settle all eight.

# 2. BLOCKER Issues

None. The two issues involving requirement conflicts, unclear permissions, or undefined main flows were classified as CRITICAL because the design structure, permission model, and operation catalog were already fixed and local contract additions could resolve them.

# 3. CRITICAL Issues

## FRV-001
- Issue ID: FRV-001
- Severity: CRITICAL
- Document: Common detailed design §5, deterministic contract D03, input/output contract DDC-08 §3/§4, requirements FR-A08/A09/C11/C12
- Location: Payment/restriction state table, AT-A08-N③, AT-A09-N④, AT-C11-N④, operation catalog restrictions.release
- Problem: Transition to release_requested is described as both automatic on payment confirmation (DDC-08 §3, AT-A08-N) and requested by HQ through restrictions.release (AT-A09-N). Preconditions, idempotency, and the trigger for creating remove Commands are undefined.
- Why it matters: This is central to S03. Implementers may choose automatic remove on payment, no remove until HQ presses the button, or CONFLICT when release is called in release_requested.
- Example Failure: After customer payment sets release_requested, HQ presses release and gets CONFLICT under D01⑥. AT-A09-N④ or AT-C11-N④ fails.
- Required Fix: Define the start paths (payment/deferral/exemption/forced release/explicit release), idempotency, and Command-generating transition in one place.
- Suggested Revision: Follow IR35. In the same payment-confirmation transition, set release_requested and perform D03 release evaluation (applied+online→remove). restrictions.release is FORBIDDEN when unpaid without deferral/exemption and is idempotent in release_requested.
- Status: resolved (IR35, common.md state table, DDC-08 §3, BR-A09, AT-A09-N/AT-C11-N, AT-REV17-001)

## FRV-002
- Issue ID: FRV-002
- Severity: CRITICAL
- Document: Deterministic contracts D04/D09, DDC-04, requirement FR-X01, AT-A09-N/AT-C12-N/AT-P02-E/AT-C04-E
- Location: “Lifetime is 30 minutes on the demo clock, with no extension,” “advance the clock to executeAfter,” AT-C04-E③ (set clock to 2026-03-07)
- Problem: Clock jumps of 24 hours or more expire the session. It is also undefined whether the demo clock advances in real time or can move backward (AT-C04-E③ sets it before the seed time).
- Why it matters: Restriction, contracted-access-expiry, and DST acceptance procedures return to /login midway, so expected results cannot hold. The Test Agent must guess whether to log in again or implement advanceClock differently.
- Example Failure: schedule→advanceClock(+24h)→execute returns UNAUTHENTICATED.
- Required Fix: Define clock progression (real-time ticks, forward jumps, backward movement) and session handling during jumps.
- Suggested Revision: Follow IR36. A jump shifts Session.expiresAt by the same amount without consuming session lifetime. Backward movement is allowed only just after reset. Expiry tests use the session_expired trigger.
- Status: resolved (IR36, D09, common.md §6, AT-A09-N, AT-REV17-002)

# 4. MAJOR Issues

## FRV-003
- Severity: MAJOR / Document: Deterministic contracts D01/D04/D10, DDC-03, canonical DemoTrigger type, AT-C01-E③/AT-C08-E①/AT-C09-E④/AT-T08-E③/AT-T03-N③
- Problem: Acceptance criteria assume failure injection such as making units.list return UNAVAILABLE and disconnecting the network, but DemoTrigger has no injection mechanism. Repository response codes for network(connected=false) are also undefined (OFFLINE is reserved for devices).
- Why it matters: NFR-05 and error paths in S01/S06 cannot be implemented or tested.
- Example Failure: Tests directly alter mock internals. Network disconnection is shown as OFFLINE and confused with an offline device.
- Required Fix: Define transport injection events and response codes, subscriptions, and recovery rules for network disconnection.
- Suggested Revision: IR37 (DemoTrigger 'transport', UNAVAILABLE+errors.network_disconnected, unsubscribe→new snapshot).
- Status: resolved (IR37, service-contracts.ts, DDC-03/07, AT-REV17-003)

## FRV-004
- Severity: MAJOR / Document: DD-C09, canonical jobs.create type, D16, FR-C09
- Problem: Derivation of optional dueAt when customers call jobs.create is undefined. The base value for JobSummary.dueAt, overdue KPIs, and dueAt sorting is unsettled.
- Suggested Revision: IR38 (client input returns VALIDATION; omitted dueAt=requestedEnd; HQ-specified value must be at least requestedEnd).
- Status: resolved (IR38, BR-C09, DD-C09/DD-A06, AT-REV17-004)

## FRV-005
- Severity: MAJOR / Document: D05/D14, DD-A02/DD-C02, query-catalog (no archived filter), SR27
- Problem: Handling of archived=true Unit/Space/Property in lists, KPI denominators, candidates, and individual retrieval is undefined. Including archived units in AdminSummary.total increases powerUnknown.
- Suggested Revision: IR39 (always exclude; only HQ with asset.manage may retrieve read-only; other roles get NOT_FOUND).
- Status: resolved (IR39, BR-A02, AT-A02-N, AT-REV17-005)

## FRV-006
- Severity: MAJOR / Document: FR-A01 BR-A01, DD-A01, D12, canonical Customer/Organization types
- Problem: It is unclear whether “number of active customer organizations” uses Customer.status or Organization.status. Customer:Organization cardinality is also undefined.
- Suggested Revision: IR40 (one-to-one; count Customers with both statuses=active).
- Status: resolved (IR40, BR-A01, DD-A01, AT-REV17-006)

## FRV-007
- Severity: MAJOR / Document: DD-C07 (1h/24h/7d), DD-T03/AT-T03-N (24h), SR17 (only today/7d/30d)
- Problem: It is undefined whether 1h/24h are rolling windows or calendar periods. Expected from/to values cannot be fixed.
- Suggested Revision: IR41 (rolling windows of 60/1440 minutes ending at a UTC minute boundary; 7d follows SR17).
- Status: resolved (IR41, BR-C07/BR-T03, DD-C07/DD-T03, AT-REV17-007)

## FRV-008
- Severity: MAJOR / Document: DDC-02 (JobDetail/RestrictionDetail projections), D06/D14, canonical Offer/Assignment/Restriction.events, members.list
- Problem: It is undefined whether customer JobDetail includes Offer (another company's decline reason), Assignment.reason (reassignment reason), and RestrictionDetail.events (HQ actorId/reasons/before/after), or whether contractor Membership includes permissions/scopes. D14 covers only costs/reportRefs.
- Why it matters: Internal information can leak, creating implementations that conflict with FR-X04's rule that data outside assigned scope cannot be retrieved.
- Suggested Revision: IR42 (offer=null, assignment.reason=null, masked events, exception.reason=null, technician/contractor costs=[], contractor Membership permissions/scopes=[]). Type: Restriction.exception.reason becomes string|null.
- Status: resolved (IR42, DD-A09 field notes, service-contracts.ts, AT-REV17-008)

## FRV-009
- Severity: MAJOR / Document: FR-X07, DD-P07, DDC-09, jobs.events
- Problem: It is undefined whether customer-facing jobs.events excludes events containing visibility=internal notes or returns them with note=null. Counts/cursors can reveal internal-note existence.
- Suggested Revision: IR42 (exclude entire rows; calculate total after exclusion).
- Status: resolved (IR42, BR-P07, AT-REV17-009)

## FRV-010
- Severity: MAJOR / Document: DD-T11, D05, IR11 supplement, devices.register(sensorTypes)
- Problem: Generation of Sensor.unit/staleAfterSeconds/boundaryId from sensorTypes, handling of metrics outside capabilities, and initial firmwareVersion are undefined.
- Suggested Revision: IR43 (copy the matching metric definition from Capability.sensors; unsupported metrics return VALIDATION; empty is allowed).
- Status: resolved (IR43, BR-T11, DD-T11, AT-REV17-010)

# 5. MINOR Issues

| ID | Document / Location | Problem | Fix | Status |
|---|---|---|---|---|
| FRV-011 | All role-specific DD detail sections, “Service boundaries used by this screen...” | Disagree with summary tables and operation-catalog design_ids (DD-C06/C11/C12/T01/T04/T09/T10/T11/A02/A03/A04/A05/A07/A09/A10/A11/A12/A13/A14/A15/A16). Validator checks only summary tables | Regenerated 48 sentences from summary tables; added “Detail service boundary drift” validator check | resolved |
| FRV-012 | operation-catalog.csv ui_validation column | telemetry.series/summary(read) says expectedVersion is required on updates; offsets.preview(write) says scoped snapshot | Corrected column; validator prohibits expectedVersion requirements on reads | resolved |
| FRV-013 | UX-05 versus component-contracts.csv | AppShell/RoleNavigation/TelemetryValue/TimeSeriesChart/AsyncBoundary/EmptyState/AuditTimeline/NotificationPreview missing from CSV; names also disagree | Added nine CSV rows and aligned UX-05 names with CSV | resolved |
| FRV-014 | Screen catalog; D10 “Screen ID is per route” | No screen for undefined routes (not-found) | Added SCR-X-not-found (route `*`); separated common.md §2 | resolved |
| FRV-015 | Common requirement permission matrix, “Normal AC operations: contractor generally not allowed” | “Generally” is ambiguous; exceptions unclear | Changed to “Not allowed (commands.create/voice.resolveIntent not granted)” | resolved |
| FRV-016 | FR-X02 inquiry intent; two meanings of request | Voice help and Inquiry (customer inquiry), and Job (request) and Offer (HQ offer), use the same terms | Changed FR-X02 to help; added canonical terminology section to common requirements and canonical resource names to DDC-08 §5 | resolved |
| FRV-017 | AT-C04-N “start=cool 25°C”, AT-C05-N “action=cool 24°C” | Mode+temperature wording conflicts with a single UnitAction | Changed to set_temperature 25/24 | resolved |
| FRV-018 | Required units.save.installedAt versus nullable ACUnit.installedAt; AT-T02-E① | Cannot register/edit equipment with no recorded installation date | Changed to Instant\|null (IR44); corrected DD-A02 field | resolved |
| FRV-019 | UX-06, D07, AT-C10-N “120.00 MYR”, AT-C01-B “26.0°C” | Intl locale tags, currency order, temperature digits, and rounding mode undefined | Defined in IR44; AT-REV17-011 | resolved |
| FRV-020 | UX-06 i18n, VoicePanel | Missing-key fallback and voice-panel visibility for an unauthorized role (contractor) undefined | IR44; AT-REV17-012 | resolved |
| FRV-021 | D10 screen priority (Missing Requirement Candidate) | No handling of uncaught rendering exceptions | Added ErrorBoundary to UX-05/common §2/IR44; AT-REV17-013 | resolved |
| FRV-022 | Summary.counts | Values (0/null) for counters not applicable to kind undefined | IR44 (0, hidden in UI) | resolved |
| FRV-023 | DD-A04 field table | Missing modeControl/fanControl/ventilationLevels fields present in types | Added fields | resolved |
| FRV-024 | screen-catalog.purpose; PrepareDocument section numbers | Boilerplate purpose does not explain each screen; numbering jumps from appendix to 10 | Generated from FR titles; changed to 9./10. | resolved |
| FRV-025 | DDC-07 demo.trigger.scenarioId | Meaning and limits undefined | IR44 (1–64-character label; no effect on behavior) | resolved |

# 6. Open Questions

No pending decisions block 1A implementation handover. Proceed with the following as reversible PROPOSED decisions (DEC-19–24), and confirm them before company acceptance.

| ID | Question | Provisional answer adopted | Confirm with |
|---|---|---|---|
| Q-1 | May payment confirmation automatically start a release request and remove Command? | Yes (IR35) | Product Owner / Business |
| Q-2 | May customers omit a maintenance deadline, using the requested window's end as the deadline? | Yes (IR38) | Product Owner |
| Q-3 | May customer count include only records where both Customer and Organization are active? | Yes (IR40) | Product Owner / Business |
| Q-4 | May 1h/24h be rolling windows, while today/7d/30d remain calendar periods? | Yes (IR41) | UI/UX |
| Q-5 | May other companies' decline reasons, reassignment reasons, and HQ actors be hidden from customers? | Yes (IR42) | Security / Product Owner |
| Q-6 | May demo-clock jumps avoid expiring sessions? | Yes (IR36) | Frontend / Test |

# 7. Cross-document Inconsistencies

| Type | Details | Status |
|---|---|---|
| A. Requirement Missing | No features appear in requirements but lack design/UI. The design lacked read-projection rules for FR-X07's “do not show internal notes to customers” (FRV-009) | resolved |
| B. Design Without Requirement | Design-only manual restrictions.release path conflicted with the automatic FR transition (FRV-001). Added SCR-X-not-found implements existing FR-X04/D01 | resolved |
| C. UI Without Requirement | AppShell/AsyncBoundary and other UX-05 components implement NFR-05/FR-X04. Only the missing contract CSV entries were a problem (FRV-013) | resolved |
| D. UI Without API | No contract for failure injection such as making units.list UNAVAILABLE (FRV-003). No route for not-found screen (FRV-014) | resolved |
| E. Data Model Gap | dueAt derivation (FRV-004), customerCount population (FRV-006), Sensor creation (FRV-010), installedAt null (FRV-018), non-applicable counters (FRV-022) | resolved |
| F. Terminology Conflict | Inquiry/help, request/contracting, Component names (AuditTimeline versus Timeline, etc.), “generally not allowed” | resolved |
| Contract priority | IR>SR>D>DDC>DD>FR is expressed by “takes priority over old text on the same topic” in README/individual documents. No single table exists, but no conflict was found | Unchanged |

# 8. Missing Requirements

Missing Requirement Candidates (do not add to 1A; retain for handover):

| Candidate | Basis | Handling |
|---|---|---|
| Recovery from rendering exceptions (ErrorBoundary) | Not included in NFR-05 state display | Added to 1A (FRV-021, to implement existing NFR-05) |
| Customer notification-channel preferences (email/whatsapp/inApp) | BIZ-22 guidance lets HQ choose the channel; no customer preferences | Deferred as a candidate; outside DEC-12 scope |
| Editing available work hours (members.capacity denominator) | D07 fixes seed hours at 09:00–17:00; no edit UI | Deferred as a candidate |
| Reopening completed jobs/changing deadlines | IR29/IR38 explicitly excludes these from 1A | Deferred as a candidate |
| Long-running memory capacity | Deferred IR18 candidate | Unchanged |

# 9. Edge Cases Not Defined

| Edge case | Before correction | Definition after correction |
|---|---|---|
| API timeout / HTTP 400–500 | 1A has no HTTP connection; nine DomainError types substitute (D11 finalizes the production table) | NOT DEFINED (1B). Unchanged |
| Network disconnected | Edge Case Undefined (Repository response undefined) | IR37: UNAVAILABLE+errors.network_disconnected, unsubscribe, new snapshot |
| IoT device offline / response timeout | D01 OFFLINE, Command 30-second expiry, D03 undelivered intent | Unchanged |
| Invalid / Missing / Stale sensor data | IR12/D07 | Unchanged |
| Duplicate operation | D04 idempotency key | Unchanged |
| Multiple browser tabs | Outside scope under D09; header notice | Unchanged |
| Session expiration | 30 minutes; session_expired trigger. Conflict with clock jumps was undefined | IR36 |
| Permission changed during operation | D01/IR17/IR24 | Unchanged |
| Empty / Large device list | empty state, 25/100 pagination, D10 100 units | Unchanged |
| Slow network | No injection mechanism for D10 3000ms/12000ms fixtures | IR37 DELAY |
| Language switch | D09 (discard unconfirmed intents; UTC unchanged). Missing-translation fallback was undefined | IR44 |
| Browser reload / Back button | DEC-07, D13 | Unchanged |
| Concurrent update | version CONFLICT | Unchanged |
| Rendering exception | Undefined | IR44 ErrorBoundary |
| Archived resources | Undefined | IR39 |

# 10. Traceability Matrix

[traceability-matrix.csv](traceability-matrix.csv) (64 requirements; status_before_fix/status_after_fix columns). Before correction: CONFLICT 4 (FR-A08/A09/C11/X01), INCOMPLETE 21, OK 39. After correction: OK 64, INCOMPLETE 0, MISSING 0, CONFLICT 0. OK means document mappings exist (Prepare BIZ→FR→DD→Screen→operation→D01/IR37 error paths→AT-N/E/B plus additional ATs), not that application tests passed.

# 11. Undefined Decisions

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| DEC-19 | Automatic release requests on payment confirmation and idempotent release (adopted as PROPOSED) | IR35 | Make S03 flow unambiguous | Product Owner / Business |
| DEC-20 | Clock jumps do not consume session lifetime | IR36 | Make acceptance procedures work | Frontend / Test |
| DEC-21 | Customer-created job dueAt=requested window end | IR38 | Base value for deadline KPIs/sort | Product Owner |
| DEC-22 | Customer-count population (one-to-one, both active) | IR40 | KPI definition | Product Owner / Business |
| DEC-23 | 1h/24h rolling windows | IR41 | Expected periods | UI/UX |
| DEC-24 | Hidden fields in role projections | IR42 | Protect internal information | Security / Product Owner |
| IR18 | Long-running memory capacity | IR18 | When expanding the supported scope | Product Owner / Frontend |
| OPEN-03–06/11 | Production devices/APIs/authentication/integrations/termination responsibility | D11/Prepare §10 | Before production design starts | Backend / IoT / Security / Business |
| DEC-02/03 | Pin library versions | design-assumptions | Lockfile at implementation start | Frontend |

# 12. Implementation Readiness

| Area | Assessment | Basis |
|---|---|---|
| Requirements completeness | READY | Sources/design/screens/operations/AT for 64 requirements. FRV-004/006 gaps filled |
| Cross-document consistency | CONDITIONALLY READY | FRV-001/002 conflicts resolved. Follow-up is self-review; another reviewer finalizes G1 |
| UI/UX completeness | READY | 48 screens, 66 Component contracts, seven states plus five IoT states, not-found added, Component names aligned |
| Frontend architecture | READY | Responsibilities separated for Repository/Query/RHF/Navigation/generations/ErrorBoundary |
| API contract readiness | READY (1A local contracts) | Inputs/outputs/authorization/versions for 136 operations. Production HTTP is NOT READY (D11) |
| Error handling | READY | D01/D04/IR37 failure injection and network disconnection; nine DomainError types |
| Authentication / Authorization | READY (1A mock) | Two permissions, scope, hidden-field projection rules (IR42). Real authentication outside scope |
| IoT state handling | READY (simulated) | Independent connection/power/tamper axes; D03/SR26 recovery; IR43 Sensor creation |
| Testability | READY | Fixtures and expectations in all AT bundles, including AT-REV17-001–015. Clock-jump conflict resolved |
| Agentic SDLC handoff readiness | CONDITIONALLY READY | Manifest/validator/acceptance plan updated. Independent G1 by another reviewer pending |

# 13. Required Actions Before Implementation

1. Another reviewer (another Agent or a human) independently assesses G1 for the DOC-0.17.0 baseline and updates gate-G1.yaml to passed/failed. This report is self-review.
2. Product Owner / Security / UI/UX confirms DEC-19–24 (PROPOSED) before company acceptance. Implementation may proceed as a reversible proposal.
3. The implementation Agent uses all 57 manifest files, reading IR35–44 with priority over old text. Application tests start as not_run; add AT-REV17-001–015 to existing ATs.
4. Production connections (HTTP/authentication/DB/real devices) remain NOT READY until D11 deliverables are finalized.

Validation command: `python3 docs/tools/validate_documents.py` (errors=[]); TypeScript strict validation with `--tsc /private/tmp/ac-typescript-check/package/lib/tsc.js`.
