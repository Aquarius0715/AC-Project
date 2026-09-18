# 1. Executive Review Summary

**Independent G1: PASS (the 1A frontend mock design is READY for implementation handover). Zero unresolved findings.**

Final input: DOC-0.16.0, spec_baseline_id=`fe251845299c3b5d0a9d493683be3e70076111af3b290b8b01a8fa2f7450e0b2`. Reviewer `/root/independent_g1_016` ran separately from specification author `/root`. The reviewer did not edit specification files, returned findings to the author, and checked the actual corrected files. Previous-version passes and self-reviews were not reused as approval evidence.

One additional finding was raised: G1R-001 (MINOR, missing source metadata in decision records). It was corrected and independently rechecked. Existing REV-001–007 were confirmed resolved. The two permissions and the job sorting feature/default business order agree across requirements, contracts, types, Queries, URLs, screens, Components, and acceptance plans.

Independently calculated SHA-256 for all 55 files and the baseline from canonical JSON; all matched. Independently ran static validation and TypeScript strict successfully. Also checked authorization, projection, sort, pagination, state, retry, and deadline counterexamples across documents. Application implementation and runtime/performance/a11y tests are **not_run**. Production connections/deployment are **NOT READY** and outside G1 scope.

# 2. BLOCKER Issues

Zero unresolved. REV-001: FR-A10's list and details, Permission, operation catalog, IR03/34, A09/A10, and AT-REV16-001 agree on two permissions. Checked manage only, override only, both, and neither; neither permission implicitly grants the other. Override-only users have a limited read projection and release-tracking path without permission to apply restrictions.

# 3. CRITICAL Issues

Zero unresolved. REV-004: audit.list applies search conditions to the authorized set. Another tenant's correlationId and a nonexistent ID both return a successful empty set, distinct from NOT_FOUND on individual get. AT-A16-E and AT-REV16-004 expectations also agree.

# 4. MAJOR Issues

Zero unresolved.

| Item | Contracts and counterexamples independently checked | Assessment |
|---|---|---|
| REV-002 | IR32/26/23; unitIds in jobs.list and summaries.get. Empty arrays, AND with unitId, and no match for undisclosed offer/history | resolved |
| REV-003 | IR33, A16, screen/Component/operation catalogs. Required audit; devices candidates→explicit selection→events; unauthorized Command links cause no additional read | resolved |
| REV-005 | DEC-18, JOB_STATUS_ORDER, Query, IR34, all jobs.list screens, AT-REV16-005. All 10 states, reverse order, ID tie-breaks, public/frozen states, nulls last | resolved |

Verified that sort changes preserve filters and discard cursors, invalid/empty/duplicate URL sort values are rejected, Back/Forward restores state, and Query keys include sort and view generation. Apply sort to the full snapshot before pagination; do not pass it to summaries.get. Sorting does not change KPI meaning or state transitions.

# 5. MINOR Issues

Zero unresolved. REV-006 defines Page.total as a nonnegative integer after authorization/projection/filtering; data not yet retrieved is represented separately by initial/loading. REV-007 uses the current manifest's gate as the decision source. Fixed pending/not-performed statements in README were replaced with links to that source.

## G1R-001 — Source metadata for accepted decisions does not meet the execution schema

- **Issue ID:** G1R-001
- **Severity:** MINOR
- **Document:** docs/00-prepare/internal/review-decisions-016.json, docs/04-agentic-sdlc/templates/artifacts.md
- **Location:** Accepted DEC-17/18 records; template sections “State schema” and “Requests for human decisions and decision records”
- **Problem:** At the initial review, the records had status/selected/source but lacked decided_by, the decision date, and answer required for accepted records, as well as decision_status from the execution schema.
- **Why it matters:** A machine-based handover cannot distinguish a candidate owner from the actual decision-maker or a design-side selected value from the actual user answer.
- **Example Failure:** Product Owner / Security in owner is read as the actual approver, or the next phase's accepted-metadata check marks the record incomplete.
- **Required Fix:** Record the decision-maker, the decision date at the known precision, the original answer, and decision_status based on the actual conversation answer. Do not invent names or times that were not provided.
- **Suggested Revision:** decided_by=user in this conversation, decided_at=2026-09-16, decided_at_precision=day, answer=each original answer, decision_status=accepted. If the status alias is retained, verify that it agrees.
- **Finding status:** resolved
- **Recheck:** Confirmed the above in DEC-17/18 in the final baseline. The user decisions themselves are unchanged. Required provenance and status-agreement checks were added to the validator, and typed static validation also passed. Correction author: /root. Rechecker: /root/independent_g1_016.

# 6. Open Questions

No additional questions block this 1A design handover. DEC-17/18 are accepted based on the user answers. Specific business-order ranks are fixed in the IR34 implementation contract, which distinguishes display order from permitted state transitions.

# 7. Cross-document Inconsistencies

No additional inconsistencies were found after correction.

| Check axis | Basis |
|---|---|
| Requirement Missing | Statically checked source/design/UI/operation/acceptance mappings for 64 requirements. Focus: FR-A10/A16, P01/P03/P06/T01/C09/A06, FR-X04 |
| Design / UI Without Requirement | Two permissions resolve an existing FR-A10 conflict. Sorting follows the current user instruction. A16 candidate reads provide a path for the existing audit feature |
| UI Without API | API means the 1A Repository. Operations exist for audit device selection, Command link resolution, restriction-only reads, and jobs.list inputs |
| Data Model Gap | Compared Page.total, Query.unitIds/sort, the three Job projections, JOB_STATUS_ORDER, and RestrictionReleaseView with canonical types |
| Terminology Conflict | Distinguished Device from Unit, Command response from Job completion, view generation from Repository generation, and null severity from normal |
| Handoff consistency | Current manifest and decision evidence pinned to the baseline. An old passed result is not current approval |

Applied the existing rule that IR contracts take priority over old DD/D/SR text on the same topic. Hand over the full manifest; do not implement from a screen DD alone.

# 8. Missing Requirements

This review found no additional missing required 1A requirements. Checked company source SRC-06→BIZ mapping→FR source table and preserved the distinction between company requests and production guidance/design additions. A separate contractor role and detailed demo conditions are not treated as the company's original text.

Long-running memory capacity is a deferred candidate under IR18. Production APIs/authentication/DB/IoT/real payments and notifications/responsibility for termination after the browser stops are later D11/OPEN issues. Do not add them to 1A without authorization or treat them as production READY.

# 9. Edge Cases Not Defined

The following are document counterexample checks, not application test results.

| Boundary/counterexample | Definition and expected result |
|---|---|
| No manage; override only | IR03/34. Limited projection, forced release, and limited release tracking only. Reject defer/exempt/cancel |
| Another tenant's correlation ID / nonexistent ID | IR33. Both succeed with zero results. A failed required audit read is not treated as a successful empty result |
| No events selection / candidate failure / invisible URL | IR33. Zero calls / local retry / not-found. No substitution with another device |
| Equal states under desc / null dueAt and severity | IR34. ID asc for ties; nulls last in both directions |
| Offer/Assignment ends just before page 2 | IR24/SR14. Entire old snapshot returns CONFLICT; update viewEpoch; retrieve frozen projections from page 1 |
| Non-public Job update with the same state | IR23/34. No count/order changes if the public offer/history values are unchanged |
| Late response after sort change / A→B→A | IR17/34. Reject rendering from an old key/generation. An old response does not overwrite the new list |
| Normal control while offline versus undelivered restriction | D03. The former creates zero records; the latter records undelivered intent and is not shown as applied |
| sent_unknown/late ack during restriction release | D03/SR26. Keep pending until reconciliation; do not restore a terminal state to active; track with a recovery case |
| Write timeout / retry / permission expiry | D01/D04/IR03. Check result → retry the same intent; reapply current authorization and projection |
| Session expiry / reload / tabs | D09/IR17. Discard the view; continue accepted business processing; reload returns to seed; tabs are not synchronized |
| Unknown units / missing or stale sensors | D07/IR08/12/16/22. suspect/null and excluded from integration; no fallback to older valid data |
| Test-run end by clock alone / late FW success | D04/05 and AT-T10/T11. Do not invent real stopping or update success |
| Report co-editor approves through another Membership | IR31. Contributor userId set gives FORBIDDEN; UI is also disabled through reviewAvailability |
| HTTP codes / real-device recovery | Outside 1A. D11 production contracts are NOT DEFINED/NOT READY |

No additional undefined 1A boundaries were found. Sampled existing ATs for remote control, schedules/DST, power comparisons, requests, contracted-access expiry, self-approval, test runs, and FW updates, applying priority definitions in IR.

# 10. Traceability Matrix

See the [existing mapping table for 64 requirements](traceability-matrix.csv) and [specification traceability table](../../../00-prepare/traceability.csv). Independently run static validation confirmed 64 requirements, 182 acceptance bundles, 49 role details, 136 operations, 47 screens, 57 Components, 35 Query contracts, and 93 version branches. Changes in IR32–34 were checked semantically from requirements through types/Queries/UI/AT.

OK in the table means a document mapping exists. It does not mean every subcase of all 64 requirements was executed or prove implementation correctness for each feature. The semantic review covered the changes, related risks, and sampled existing ATs listed in this report.

# 11. Undefined Decisions

| ID | Decision Needed / Status | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| DEC-17/18 | Resolved; user answers recorded | Decision JSON/IR34 | Fix two permissions and default business order | User in this conversation (answered) |
| IR18 | Long-term retention capacity, deferred | IR18 | Needed when expanding the supported scope | Product Owner / Frontend |
| OPEN-03–06/11 | Production devices/APIs/authentication/integrations/termination responsibility | D11/Prepare | Later deliverables required to proceed to production | Backend / IoT / Security / Business |

See DEC-12 for 1A decision-makers. G1 does not replace business or deployment approval.

# 12. Implementation Readiness

| Area | 1A Design Assessment | Basis |
|---|---|---|
| Requirements completeness | READY | Zero pending decisions in this work; acceptance criteria cover the changes |
| Cross-document consistency | READY | Corrections confirmed for the existing seven findings and the additional metadata finding |
| UI/UX completeness | READY | sort/URL/loading/error/empty/permission-specific navigation and accessibility conditions |
| Frontend architecture | READY | Responsibilities separated for Repository, Query, RHF, Navigation, and generations |
| API contract readiness | READY | 1A local contracts. Real HTTP is NOT READY |
| Error handling | READY | D01/D04, correlation IDs, invalid URLs, snapshots, retries |
| Authentication / Authorization | READY | 1A mock authentication, two permissions, scope, and projections. Real authentication is outside scope |
| IoT state handling | READY | Simulated requests/observations/recovery and exclusivity. Does not guarantee real-device compatibility |
| Testability | READY | Specific expected orders, rejections, counts, and boundaries defined |
| Agentic SDLC handoff readiness | READY | G1 decision by another reviewer for this baseline. Orchestrator updates the gate record |

# 13. Required Actions Before Implementation

No unresolved specification corrections remain. The orchestrator updates gate-G1 for the same baseline to passed based on this report and the result JSON. If the manifest changes, reassess instead of automatically reusing this decision.

At implementation, hand over all manifest inputs and existing/additional ATs; application tests start as not_run. Performance, a11y, browser behavior, and human/external review before deployment are later phases. This report is not evidence that they passed.

Independently executed command: `python3 docs/tools/validate_documents.py --tsc /private/tmp/ac-typescript-check/package/lib/tsc.js` (exit 0, errors=[], typescript_semantic_check=passed). Hash checks used a separate Python process from the validator to calculate all 55 files and canonical JSON; zero mismatches, with unique sorted paths confirmed.
