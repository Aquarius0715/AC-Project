# 1. Executive Review Summary

The user answers DEC-17/18 are reflected, and all seven findings have been corrected and rechecked by the author. Zero specification decisions are pending. Independent G1 by another reviewer after the corrections has not been performed, so the phase is CONDITIONALLY READY.

Restriction operations use two permissions: restriction.manage/override. Job lists allow ascending/descending sorting by status (business order), severity, and due date. The default is status in business order. These deliverables are 1A design documents; the application is not implemented and runtime tests are not_run.

Document/Location/Problem/Why it matters/Example Failure/Required Fix/Suggested Revision for all findings are saved in the [detailed report before the answers](before-decisions/review.md). After the answers, use the resolution assessments in this report. Decisions: [DEC-17/18](../../../00-prepare/internal/review-decisions-016.json). Corrected specification: [IR34](../../../02-design/review-resolution-contracts.md#ir34-user-confirmed-permissions-and-job-sorting). Expected results: [additional acceptance plan](../../acceptance-review-016.csv).

# 2. BLOCKER Issues

Zero unresolved. REV-001 is settled on two permissions. FR-A10's list and details, IR34, UI, and AT-REV16-001 are aligned while the existing Permission type and operation catalog are retained. Expected results are defined for manage only, override only, both, and neither. IR03's limited release tracking is also retained.

# 3. CRITICAL Issues

Zero unresolved. The audit-search correction for REV-004 is retained. Search the authorized set; both another tenant's correlation ID and a nonexistent correlation ID return a successful empty set.

# 4. MAJOR Issues

Zero unresolved.

| Issue | Basis for resolution |
|---|---|
| REV-002 | IR32 aligns list/summary unitIds, empty arrays, and matching after public projection |
| REV-003 | IR33 defines required audit Queries, candidate selection, URLs, and separate authorization for related links |
| REV-005 | DEC-18/IR34 defines the order of all 10 states, sort UI, defaults, ascending/descending order, stable IDs, and pagination |

Business order: requested → offered → accepted → assigned → in_progress → on_hold → submitted → rework_requested → completed → cancelled.

This is display order and does not change permitted state transitions. IDs within the same state stay ascending even with desc. offer/history uses public/frozen states. The recheck also added sort/cursor/limit and IR17 generations to the Query key summary.

# 5. MINOR Issues

Zero unresolved. REV-006 defines Page.total as an exact nonnegative count. REV-007 uses the current manifest's gate for phase status. The report, manifest, and gate from before the answers are saved in before-decisions.

# 6. Open Questions

No additional questions about these 1A findings. DEC-17/18 are accepted based on the user answers.

# 7. Cross-document Inconsistencies

FR-A10→Permission→operation catalog→UI→AT agree on two permissions. DEC-18→JOB_STATUS_ORDER→Query.default_sort/sort_mapping→screens/Components→AT agree on business order as the default. sort is passed only to lists and does not change KPI counts. These changes add no real HTTP/DB/device features or new permissions.

# 8. Missing Requirements

No unresolved gaps remain for these seven findings. Long-running memory limits remain deferred as the existing Missing Requirement Candidate in IR18. Production APIs/authentication/DB/device recovery are later deliverables under D11 and are NOT READY. Do not include them in the 1A defect count.

# 9. Edge Cases Not Defined

The [checks of communication, IoT, sensors, sessions, and related cases before the answers](before-decisions/review.md#9-edge-cases-not-defined) remain valid. Their Edge Case Undefined items for permission granularity and state order are resolved.

Additional checks: all 10 states, ID tie-breaks, public offer/history, asc/desc, null due dates, multiple pages, discarding cursors while preserving filters on sort changes, Back/Forward, invalid/empty/duplicate sort, language switches, late responses for old conditions, loading/error/empty, keyboard/mobile/aria-sort, and four permission combinations. AT-REV16-001/005 specifies expected results. Application tests are not_run.

# 10. Traceability Matrix

[Assessment table for all 64 requirements](traceability-matrix.csv): 64 document mappings OK. The previous one CONFLICT and six INCOMPLETE rows are resolved through DEC-17/18 and additional ATs. OK means document cross-checks, not passed application behavior tests or independent G1.

# 11. Undefined Decisions

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| DEC-17 | Resolved: two permissions | FR-A10/IR34 | User has answered | Product Owner / Security |
| DEC-18 | Resolved: sorting feature, business order by default | Query/UI/IR34 | User has answered | Product Owner / UI/UX |
| OPEN-03–06/11 | Production devices/APIs/authentication/external integration/termination responsibility | Prepare/D11 | Required deliverables before production design | Backend / IoT / Security / Business |
| IR18 | Long-running capacity guarantee | IR18 | Candidate when expanding the supported scope | Product Owner / Frontend |

# 12. Implementation Readiness

| Area | Assessment | Basis |
|---|---|---|
| Requirements completeness | READY | Both pending decisions resolved |
| Cross-document consistency | READY | Types/Queries/UI/AT agree |
| UI/UX completeness | READY | Selection, defaults, URLs, states, and usability defined |
| Frontend architecture | READY | Repository/Query/RHF/Navigation/generations and sort keys separated |
| API contract readiness | READY | 1A local contracts only. Production HTTP is NOT READY |
| Error handling | READY | D01/D04 plus invalid sort and rejection conditions |
| Authentication / Authorization | READY | Two 1A permissions. Real authentication is outside scope |
| IoT state handling | READY | Existing simulated-state contracts retained |
| Testability | READY | Expected results for permission matrix, all state ranks, and boundaries |
| Agentic SDLC handoff readiness | CONDITIONALLY READY | Independent G1 by another reviewer has not been performed |

# 13. Required Actions Before Implementation

No specification decisions or corrections remain for this work. An independent reviewer will assess G1 using the manifest after the answers. Do not record self-rechecks as approval by another reviewer.

Iterations: initial seven findings → five corrected → user decides the remaining two → requirements/design/types/UI/AT updated → Query key summary aligned during re-review → static/type validation and validator mutation checks. Evidence: static-check.json, validator-negative-checks.json, baseline-integrity-check.json. Post-implementation runtime/performance/a11y tests and human/external review before deployment are separate phases.
