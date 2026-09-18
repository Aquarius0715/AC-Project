# 1. Executive Review Summary

**A complete handover to the implementation Agent is NOT READY. Not all items are OK.**

Scope: Prepare, common and four-role requirements and detailed designs, UI/UX, and canonical contracts. Review input: DOC-0.15.0 (baseline `f11600bc8735d2a10797b418bb13da76601b8021184a7d4cef6c2a70d355dd5f`); corrected version: DOC-0.16.0. Existing uncommitted changes were preserved, and a new decision was recorded without changing old runs.

Seven findings were raised; five were corrected and rechecked by the author. Two remain unresolved: one permission-granularity conflict (BLOCKER) and one undefined state sort order (MAJOR). Existing specifications do not rule out either interpretation, so the user was asked to choose for both. No option has been adopted without an answer.

The documents cover the **1A frontend mock**. Real HTTP/API, DB, authentication, and device-connection contracts are outside scope under D11; production is NOT READY. These undefined items are disclosed but are not added to the agreed 1A scope without permission. Actual device safety, commercial payment restrictions, and medical/legal compliance were not assessed.

Static validation checked 64 requirements, 136 operations, 47 screens, and 57 Components; TypeScript strict also passed. Validator mutation checks detected 30/30 cases. However, the application is not implemented and runtime/performance/a11y tests are not_run. Reference consistency checks do not prove all behavior is correct. The correction author and rechecker were both `/root`; **independent G1 by another reviewer after the corrections has not been performed**. Independent approval of an older version is not carried forward.

Review approach: Requirement Actors/start conditions/inputs/outputs/success and failure conditions are in each FR detail; operation permissions and validation are in operation/version catalogs and D01; UI states are in screen/Component catalogs and D10/D13. A role-specific DD alone is not enough for implementation; the full manifest, including common contracts, is required. Distributed documentation is not itself treated as missing information. The following inconsistencies remained after cross-checking.

# 2. BLOCKER Issues

## REV-001 — Conflicting requirements for two versus four permissions

- **Severity:** BLOCKER
- **Document:** Admin requirements, admin detailed design, canonical DTOs, operation catalog
- **Location:** FR-A10 feature-list row / FR-A10 start conditions / DD-A10 / Permission / restrictions.*.authorization
- **Problem:** The requirement list assigns separate permissions to defer, exempt, cancel, and manual release. The details of the same requirement and the design group the first three under restriction.manage and forced release under restriction.override. The type cannot express independent control of all four operations.
- **Why it matters:** Permissions define allowed actions, not display preferences. Choosing either the requirement or design without approval changes authorization rules.
- **Example Failure:** Giving restriction.manage to HQ staff responsible only for deferral also lets them exempt and cancel. An Agent that implements four permissions conflicts with current fixtures, types, and screen guards.
- **Required Fix:** The final decision-maker must choose two or four permissions, then update the FR list/details/types/operations/UI/rejection tests together.
- **Suggested Revision:** If two permissions are chosen: “Authorize deferral, exemption, and cancellation with restriction.manage, and forced release with restriction.override.” If four are chosen, also define individual permission names, their relation to existing manage, and fixture assignments. **No option is adopted yet.**
- **Correction/recheck:** Unresolved. DEC-17 is pending_user.

# 3. CRITICAL Issues

## REV-004 — Cross-tenant handling in audit search can reveal existence

- **Severity:** CRITICAL
- **Document:** Admin requirements, common contracts, admin detailed design
- **Location:** AT-A16-E① / FR-A16 boundaries / DD-A16 / SR03 list authorization
- **Problem:** An acceptance criterion requires NOT_FOUND when searching another tenant's correlationId, but the list contract filters the authorized set. A correlation ID is a list search condition, not an individual resource get.
- **Why it matters:** Returning an error only for IDs that exist in another tenant, while nonexistent IDs return zero results, reveals existence outside the authorized scope. The Test Agent may wrongly fail a safe empty-set implementation.
- **Example Failure:** Switching between IDs with the same syntax returns NOT_FOUND only for a known external correlation ID, revealing audit-data existence to the searcher.
- **Required Fix:** Always search after authorization. Expect the same successful empty set for another tenant's ID and a nonexistent ID. Keep this separate from NOT_FOUND on individual get.
- **Suggested Revision:** “audit.list searches the authorized set and returns items=[], total=0, nextCursor=null for both another tenant's and a nonexistent correlationId.”
- **Correction/recheck:** Corrected. FR-A16, AT-A16-E, DD-A16, and IR33 agree. Application tests for nondisclosure have not been run.

# 4. MAJOR Issues

## REV-002 — unitIds differs between job lists and summaries

- **Severity:** MAJOR
- **Document:** Detailed design, Query catalog, UI/UX
- **Location:** IR26 / query-catalog jobs.list and summaries.get / P01 and T01
- **Problem:** IR26 retains unitIds and passes the same conditions to lists and summaries, but jobs.list's allowlist lacks unitIds while summaries.get has it. Unknown filters return VALIDATION under D12.
- **Why it matters:** The list and KPIs cannot use the same unit set. Removing the filter broadens the summary scope.
- **Example Failure:** unitIds=[U1] gives a KPI count of one, but the list returns VALIDATION or shows U2 when the implementation drops the condition.
- **Required Fix:** Define the same condition for lists and align comparison after public projection, empty arrays, and AND with unitId.
- **Suggested Revision:** “jobs.list unitIds matches public summary.unitId. An empty array gives zero results; offer/history does not match. Do not evaluate against the non-public source Job.”
- **Correction/recheck:** Corrected. Updated IR32, Query catalog, and common UIUX. The validator compares the shared list/summary filter sets.

## REV-003 — Missing retrieval paths for device history and related references on the audit screen

- **Severity:** MAJOR
- **Document:** Admin detailed design, screen/Component/operation catalogs
- **Location:** DD-A16 / SCR-A16 / devices.events({id,query}) / commands.get.authorization
- **Problem:** A16 includes devices.events but lacks candidate retrieval, selection, and URL restoration for the required deviceId. It also requires comparison with Command/Job/Restriction while assuming only audit.read; individual resource read permissions and link resolution are unclear. audit.list itself is secondary, with primary=none.
- **Why it matters:** Implementations may guess target IDs, require unauthorized reads, or treat audit retrieval failure as a normal screen.
- **Example Failure:** The initial audit page passes undefined to devices.events. For audit-only HQ, Command comparison returns FORBIDDEN and makes the entire audit unreadable.
- **Required Fix:** Define the existing devices.list→explicit selection→devices.events path. Separate the required audit Query from supporting panels. Enable related links using existing individual read permissions.
- **Suggested Revision:** In IR33, define retrieval order, zero results/failure/invalid URL/Back, individual resource links, and permissions. Do not make audit.read a blanket permission for other resources.
- **Correction/recheck:** Corrected. Updated DD, screen, Component, and operation catalogs. The recheck also added the commands.get dependency for Command link resolution to the catalog. No new business operations or permissions were added.

## REV-005 — Job status comparison order is NOT DEFINED

- **Severity:** MAJOR
- **Document:** Detailed design, Query catalog, UI/UX
- **Location:** D07 / D12 / D15 / IR23 / jobs.list.allowed_sort=status
- **Problem:** asc/desc, nulls last, and ID tie-breaks are defined, but the order for comparing different JobStatus values is missing. TypeScript union declaration order is not a sort contract.
- **Why it matters:** Business order, English-code order, and translated-label order are all possible implementations, so expected language-switch and pagination results cannot be specified uniquely.
- **Example Failure:** Different Agents order accepted, requested, and completed differently. Only an implementation that sorts labels changes order when switching to Malay.
- **Required Fix:** Choose a fixed comparison order and apply it to descending order, stable IDs, and public offer/history status.
- **Suggested Revision:** “Compare status using [the chosen order of all 10 states]. desc reverses the order; ties use jobId asc. Do not compare translated labels.” **No order has been adopted.**
- **Correction/recheck:** Unresolved. DEC-18 is pending_user.

# 5. MINOR Issues

## REV-006 — Unknown Page count display conflicts with the required number type

- **Severity:** MINOR
- **Document:** Frontend input/output contracts, canonical DTOs
- **Location:** DDC-01 Page<T> / Page.total:number / D07 and SR14
- **Problem:** The text describes display “when total is unknown,” but the DTO requires a number and the mock computes the full authorized snapshot count. No unknown-value representation exists, which can confuse not-yet-fetched with a fetched zero result.
- **Why it matters:** An Agent may add undefined values such as -1, null, or undefined.
- **Example Failure:** Returning total=-1 breaks zero-result display or page-count calculations.
- **Required Fix:** Align the text with the existing exact snapshot count.
- **Suggested Revision:** “total is a nonnegative integer after authorization, projection, and filtering. Not-yet-fetched is initial/loading, not an unknown-count Page state.”
- **Correction/recheck:** Corrected without changing types. Zero results and not-yet-fetched are distinct.

## REV-007 — Current-version independent review status differs between text and evidence

- **Severity:** MINOR
- **Document:** strict-review-contracts, index, G1 evidence
- **Location:** Start of strict-review-contracts / DOC-0.15.0 gate-G1
- **Problem:** Text labeled with the current version says “G1 is pending because independent re-review has not been performed,” while that older baseline's actual gate is passed. History and current status are not distinguished.
- **Why it matters:** The Orchestration Agent must choose between the text and gate. It may also use the old passed result after corrections.
- **Example Failure:** Reuse an old pass as approval of the corrected version, or unnecessarily restart review of an old version.
- **Required Fix:** Explicitly label the historical text and use only the gate bound to the manifest for the current decision.
- **Suggested Revision:** “This paragraph describes events at version 0.9.0. Determine current G1 from the gate record for the baseline linked by the index.”
- **Correction/recheck:** Corrected. Old runs are retained. 0.16.0 has a separate pending gate; self-checks of corrections are not replaced with independent approval.

# 6. Open Questions

Do not create duplicate findings; link questions to unresolved issue IDs.

| Decision | Issue | Question | Status |
|---|---|---|---|
| DEC-17 | REV-001 | Should restriction operations have two or four permissions? | pending_user / BLOCKER |
| DEC-18 | REV-005 | Should job states use an explicit business order or ASCII order? | pending_user / MAJOR |

Awaiting an answer has not been changed to implied approval or adoption of the recommended option. The affected implementation and acceptance expectations cannot be finalized until the decisions are made.

# 7. Cross-document Inconsistencies

| Category | Path/specific problem | Assessment |
|---|---|---|
| A Requirement Missing | Types/design do not express FR-A10's independent permission for each operation | REV-001 unresolved |
| B Design Without Requirement | No new business feature additions found. Supporting reads provide paths for existing FRs | IR32/33 does not add scope |
| C UI Without Requirement | No new user-facing feature additions found. Real User CRUD, voice, and transactions remain outside scope | Scope retained |
| D UI Without API | No path to obtain the required ID for A16 device history | REV-003 corrected. API here means local Repository |
| E Data Model Gap | No type represents an unknown Page count | REV-006 corrected |
| F Terminology Conflict | Meaning and comparison target of state order undefined | REV-005 unresolved |
| Condition mismatch | unitIds in lists and KPIs | REV-002 corrected |
| Expected-result mismatch | Authorized list versus correlation-ID NOT_FOUND | REV-004 corrected |
| Phase-status mismatch | pending in text versus passed for an older baseline | REV-007 corrected |

ACUnit (equipment) and Device (IoT device) are separate models and must not be merged as synonyms. requested/acknowledged belongs to Command, requested/completed to Job, and paid to Invoice; do not combine them into one UI state. The existing rule uses canonical DTOs over D14 conceptual-model abbreviations.

# 8. Missing Requirements

See REV-001/003/005 for gaps in existing requirements. The feature list's temperature/humidity, CO₂, power, notifications, home/office and location hierarchy, four roles, languages, and voice demo have mappings in the reviewed documents. This does not mean they are implemented.

**Missing Requirement Candidates (do not add to current required 1A scope):**

| ID | Candidate | Reason/handling | Decision-maker |
|---|---|---|---|
| MC-01 | Memory limits and retention periods for continuous long-running use | Already deferred under IR18. Acceptance at 100 units/1000 samples does not imply unlimited operation | Product Owner / Frontend |
| MC-02 | Production APIs/authentication/authorization/DB/device consistency/failure recovery | Later D11/OPEN-03–06/11 deliverables. Required if proceeding to production | Backend / IoT / Security |

Do not add User registration, real email delivery, real GPS/microphone, real payments, final customer acceptance, or file export merely for review convenience.

# 9. Edge Cases Not Defined

“Defined” means the document contract was checked, not that tests passed.

| Edge case | Assessment/basis |
|---|---|
| API timeout | 1A uses D04's 10 seconds, no automatic retry of read TIMEOUT, and writes.getResult for writes. Production HTTP is NOT DEFINED |
| HTTP 400/401/403/404/409/429/500 | Real HTTP is outside scope and NOT DEFINED. Corresponding 1A VALIDATION/UNAUTHENTICATED/FORBIDDEN/NOT_FOUND/CONFLICT/RATE_LIMITED/UNAVAILABLE is in D01/D04/DDC-03. Do not independently finalize HTTP mappings |
| Network disconnected / Backend stopped | 1A defines OFFLINE/UNAVAILABLE and last-time display. Production Backend recovery remains pending under D11 |
| IoT device offline / response timeout | D03/D04/D05. Separate normal-control rejection, undelivered restriction intent, 30-second expiry, and late ack |
| Invalid / missing / stale sensor data | D07/IR08/IR11/IR12/IR16. Check non-finite values, units, boundaries, origin, and TTL; do not fill with zero |
| Duplicate operation | D04. Distinguish same key/content, different keys, old/new attempts, and authorization reevaluation |
| Multiple browser tabs | D09. Each tab is independent; synchronization/persistence is outside scope; always show a notice |
| Session expiration | D09/IR17. 30 minutes; discard the view; continue accepted business processing |
| Permission changed during operation | D01/SR03/IR17/IR24. Reauthorization, generation invalidation, and snapshot expiry |
| Empty device list / large device list | D07/D10/D13. Zero results versus not-found; default 25/max 100; performance scope 100 units/1000 samples |
| Slow network | D10. 3000 ms delay and 12000 ms timeout fixtures. Not a real-network SLA |
| Language switch | D09/UX06. Keep data values; discard unconfirmed voice intents. **State comparison order is Edge Case Undefined: REV-005** |
| Browser reload / Back button | D09/D13/IR17. Reload returns to seed; dirty confirmation; Back/Forward restores URLs. IR33 supplements A16 |
| Concurrent update | D04/SR02/SR14. Version mismatch, same-intent retry, immutable snapshots, and authorization expiry |
| Partial failure | D02/D03/D10/SR28. Per-unit restrictions, separate notification results, and local supporting-Query failures |
| Foreign correlationId | REV-004 corrected. Same zero results as nonexistent IDs; distinct from individual get |
| unitIds=[] / unitIds specified for offer/history | REV-002 corrected. Zero results or no match because there is no public ID. Do not search non-public values |
| HQ with permission only to defer | **Edge Case Undefined: REV-001. The current type cannot represent that permission** |

# 10. Traceability Matrix

The [traceability table for all 64 requirements](traceability-matrix.csv) is part of this report. Columns are the specified Requirement ID / Requirement / Prepare / Detailed Design / UI/UX / API / Error Handling / Testable / Status. Status is limited to OK, INCOMPLETE, MISSING, and CONFLICT.

The API column maps to the 1A Repository and does not show production HTTP approval. OK means no unresolved findings in this document cross-check. Instead of adopting old OK results, requirement sources and operation/screen/AT mappings were regenerated; rows depending on REV-001/005 were downgraded to CONFLICT/INCOMPLETE. Corrected topics also reference additional acceptance criteria. All application tests are not_run.

# 11. Undefined Decisions

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| DEC-17 | Formally choose two or four permissions | FR-A10/DD-A10/Permission | Authorization and rejection tests cannot be finalized | Product Owner / Security |
| DEC-18 | Comparison order for all Job.status states | D07/query-catalog/IR23 | sort/language-switch/pagination expectations are ambiguous | Product Owner / UI/UX |
| OPEN-03 | Real-device capabilities/events/detection accuracy | Prepare/D11 | Avoid mistaking mock success for real-device capability | IoT |
| OPEN-04 | Production APIs/DB/authentication/session/token/CSRF/authorization | Prepare/D11 | Hiding UI is not a substitute for server authorization | Backend / Security |
| OPEN-05/06 | Real payments/notifications/calculation methods/external transactions | Prepare/D11 | Responsibility for real delivery/billing/certification is undefined | Business / Backend |
| OPEN-11 | Responsibility for ending timed control after the browser stops | Prepare/D11 | Production termination/recovery owner undefined | Backend / IoT |
| MC-01 | Long-running capacity and retention limits | IR18 | Conditions for expansion to unlimited operation | Product Owner / Frontend |

Production OPEN items are not added as new unresolved 1A defects. Final 1A decision-makers are Masaki Kitano and Yuma Wakai, recorded in DEC-12. Production owners' names are unspecified.

# 12. Implementation Readiness

| Area | Assessment | Reason |
|---|---|---|
| Requirements completeness | NOT READY | Internal FR-A10 conflict remains |
| Cross-document consistency | NOT READY | REV-001/005 remain |
| UI/UX completeness | NOT READY | Permission-specific operations and state-sort expectations unsettled |
| Frontend architecture | READY | Responsibilities defined for Repository, Query, forms, Navigation, and generation management. Not implementation approval |
| API contract readiness | CONDITIONALLY READY | 1A input/output/error/version type checks passed. Authorization granularity and sort contracts unsettled. Production HTTP is NOT READY |
| Error handling | CONDITIONALLY READY | D01/D04 and audit-search corrections exist. Finalize rejection checks after permission-granularity decision |
| Authentication / Authorization | NOT READY | REV-001. Real authentication is outside scope and is not production ready |
| IoT state handling | READY | 1A request/response/expiry/reconciliation/restriction recovery/old binding defined. No real-device tests |
| Testability | NOT READY | Correct expectations for state sort and permission granularity are undecided |
| Agentic SDLC handoff readiness | NOT READY | Two unresolved findings; independent G1 after corrections not performed |

Agentic SDLC risks: Ambiguity/Non-deterministic Specification=REV-001/005, Missing Context=REV-003, Dangerous Assumption=REV-004/006, Agent Handoff Failure=REV-001/002/007. Static passes do not override these.

# 13. Required Actions Before Implementation

1. Record answers to DEC-17/18 and update related documents/types/UI/acceptance criteria to match the choices. Do not answer on the user's behalf.
2. Specify the two undecided rows in the [additional acceptance plan](../../acceptance-review-016.csv) and recheck conflicts with existing cases.
3. Pin the updated manifest and rerun static/type validation and validator mutation checks. A G1 reviewer separate from the document author must confirm zero unresolved findings.
4. After approval, start 1A implementation. Application type/lint/build/unit/Component/E2E/a11y/performance checks and human/external review before deployment are separate phases.

Iterations: Round 1 found seven issues → corrected five with established evidence → Round 2 rechecked changed operations/screens/Components/AT and permissions, and added missing Command link dependencies. Two undecided items and independent re-review remain. **The completion condition “until everything is OK” has not yet been met.**
