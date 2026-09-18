# DOC-0.14.0 Independent Review Round 1

Reviewer: `/root/independent_review`. Correction author: `/root`. Supporting independent review of IoT/asynchronous behavior/restrictions and payments: `/root/state_review`.
Scope: Specification documents for the 1A frontend mock. baseline: `f8e6cdd8387f1e623f69616518f41108e55ae9cd5f7ec4b223be5776238f4096`. Implementation, application testing, and production connections are outside the assessment scope.

## 1. Executive Review Summary

G1: **changes_requested**. Six findings (CRITICAL 1 / MAJOR 3 / MINOR 2). Gaps remain in self-approval checks after reassignment, rules for generating job severity and completion timestamps, and acceptance expectations after filtering. The address source, three post-expiry report states, disabled reasons, and branches for model-save reasons are consistent across types, operations, and UI contracts, apart from the related inconsistencies below.

Existing self-review passes and static validation passes were not used in place of independent approval. Reviewed Prepare/DEC-12–16, common and all-role requirements and designs, D/SR/IR, canonical types, operation/version/Query/screen/Component catalogs, and traceability tables. The supporting reviewer separately checked partial restriction release, old-binding events, late acknowledgments, payment-attempt exclusivity, and related cases, finding zero new issues.

## 2. BLOCKER Issues

None found.

## 3. CRITICAL Issues

### IRV-006 — Editors of reassigned reports can be excluded from self-approval checks

- Document / Location: `02-design/contractor.md` DD-P05 reviewerId, `deterministic-contracts.md` D06, `strict-review-contracts.md` SR07, FR-P05.
- Problem: DD-P05 only requires the reviewer to differ from report.authorId. Reassignment preserves the original author and changes field authors only for fields updated by the new assignee. There is no version-level self-approval check covering additional editors of text, photos, and other content.
- Why it matters: This fails DEC-12's requirement for quality review by another person.
- Example Failure: T1 creates → reassigned to T2 → T2 edits and submits → T2 accepts as a quality reviewer/HQ through another Membership. It passes because T2 != the original report.authorId.
- Required Fix: The Repository must record the userIds that created or edited the submitted version and use them to reject self-approval. Preserve original-author and field-author history, and apply the same check across Memberships and for HQ.
- Suggested Revision: Inherit the version's internal contributor set and add only users who actually changed content, including text, measurements, and photos. Viewing or assignment without editing is not a contribution. Contributors' accept/return-for-correction actions must return FORBIDDEN. Add acceptance examples that allow only a third party.

## 4. MAJOR Issues

### IRV-001 — KPI acceptance criteria after status filtering conflict with IR26

- Document / Location: `01-requirements/contractor.md` AT-P01-N, IR26.
- Problem: The criterion requires activeCount=1/reviewCount=1 even after status=offered, but IR26 applies the same AND conditions to summaries.
- Why it matters: A correct implementation fails an existing required test.
- Example Failure: One offered, one accepted, and one submitted job. For the offered-only list, the summary has two possible results: 1/0/0 and 1/1/1.
- Required Fix: Clearly state before/after filter conditions and correct the filtered expectations.
- Suggested Revision: Without conditions, offer/active/review=1/1/1. After status=offered, use 1/0/0 and one list item. Also specify the acceptance period and fixture time.

### IRV-004 — The source of the historical completion timestamp is undefined

- Document / Location: `02-design/review-resolution-contracts.md` IR23, `service-contracts.ts` MaintenanceJob/JobHistorySnapshot.
- Problem: completedAt is frozen and used for date-range searches, but the source Job has no such field and no rule generates it from the completion operation.
- Why it matters: Historical date-range searches and displayed timestamps cannot be implemented unambiguously.
- Example Failure: A job has different submission, acceptance, and cost-update dates. Each implementer chooses a different date for history searches.
- Required Fix: Define storage of the completion timestamp or derive it from an immutable completion event.
- Suggested Revision: Save Repository now when jobs.review accept succeeds; use null before then. Later cost or note updates do not change it. Freeze that value when access ends.

### IRV-005 — Rules for calculating visible job severity are undefined

- Document / Location: `service-contracts.ts` JobSummary.severity, DD-P01/T01, IR23/26.
- Problem: The required severity has no defined source, multi-alert aggregation rule, handling of resolved alerts, or value when there are no alerts.
- Why it matters: Expected display, filter, sort, and KPI results cannot be fixed.
- Example Failure: A Unit has a resolved critical alert and an open warning alert, while Job.alertIds is empty. Implementers choose critical, warning, or normal.
- Required Fix: Specify the authorized source set and aggregation rule consistent with the existing equipment urgency concept. Preserve undisclosed null values for offer/history.
- Suggested Revision: State the chosen technical rule, such as deriving severity from visible unresolved alerts on the target unit. Add tests for zero alerts, mixed alerts, resolved alerts, and out-of-scope alerts. Do not describe normal as a guarantee of equipment safety.

## 5. MINOR Issues

### IRV-002 — Old wording says addresses are private

- Document / Location: `02-design/admin.md` DD-A02 property.address/accessInstructions table.
- Problem: The address and entry instructions are jointly described as private to contractors that have not yet accepted the job.
- Why it matters: Field guidance that conflicts with DEC-16/IR25 remains. IR25's priority resolves the final decision, but a local implementation can still be wrong.
- Example Failure: The address is also cleared in the response to the pre-acceptance screen.
- Required Fix / Suggested Revision: Separate the rules: project the registered address into the company's own Offer; show entry instructions only during the valid contracted period.

### IRV-003 — The SDLC entry points to an old baseline

- Document / Location: `04-agentic-sdlc/README.md` current-input guidance (independently confirmed after root found it).
- Problem: The document version is 0.14.0, but the current-input link points to DOC-0.13.0/spec-manifest.json.
- Why it matters: The next Agent pins the wrong version.
- Example Failure: Implementation evidence records a baseline without the address and post-expiry summary corrections.
- Required Fix / Suggested Revision: Align the current-manifest link and section name. Keep old runs as history.

## 6. Open Questions

No new commercial decisions are requested. Return the above as clarifications and consistency corrections for the approved 1A requirements. The potential continuous-operation capacity requirement in IR18 remains deferred.

## 7. Cross-document Inconsistencies

IRV-001/002/003/006. Even when a correction contract takes priority, do not pass contradictory acceptance expectations unchanged.

## 8. Missing Requirements

No new features are requested. IRV-004/005/006 concern missing implementation contracts for existing required outputs and the existing self-approval ban.

## 9. Edge Cases Not Defined

The multiple-editor, post-completion update, and multiple/resolved-alert cases above need correction. Existing contracts were checked for communication timeout/disconnection, session/assignment expiry, concurrent version updates, duplicate submissions, empty lists, expiry during pagination, missing/stale data, language switching, and reload/back navigation. HTTP 400/401/403/404/409/429/500 themselves are outside 1A scope; the corresponding DomainErrors are simulated.

## 10. Traceability Matrix

Independent assessments of 64 requirements are in [independent-round-1-traceability.csv](independent-round-1-traceability.csv). Reference information was carried over from the existing traceability table; Status reflects this review's semantic assessment. Existing structural OK results were not used as the basis for independent approval. Rows requiring correction at the time of this review are below. Requirements not listed are OK in the document review, not passed application tests.

| Requirement ID | Requirement | Prepare | Detailed Design | UI/UX | API | Error Handling | Testable | Status |
|---|---|---|---|---|---|---|---|---|
| FR-P01 | Contractor job summary | BIZ-04/12 | DD-P01/IR23/26 | P01 | jobs.list/summaries.get | D01/D04 | AT-P01-N conflict | CONFLICT |
| FR-P05 | Quality review | DEC-12/BIZ-12 | DD-P05/D06/SR07 | P05 | jobs.review | Incomplete self-approval checks after reassignment | Multi-editor cases needed | INCOMPLETE |
| FR-P08 | Post-expiry visibility | DEC-16/BIZ-12 | IR23/25 | P08 | jobs.get/list | D01/IR24 | completedAt undefined | INCOMPLETE |
| FR-T01 | Assignment summary | BIZ-04/08 | DD-T01/IR26 | T01 | jobs.list/summaries.get | D01/D04 | severity undefined | INCOMPLETE |
| FR-T09 | Report versions and editors | BIZ-12 | D06/SR07/DD-T09 | T04 | jobs.saveDraft/submit | D01 | Insufficient connection to self-approval checks | INCOMPLETE |
| FR-A02 | Address input | DEC-16/BIZ-07 | DD-A02/IR25 | A02 | properties.save | D01 | Conflicting old wording | CONFLICT |
| FR-A06 | HQ quality review | DEC-12/BIZ-12 | DD-A06 | A06 | jobs.review | Incomplete self-approval checks after reassignment | Multi-editor cases needed | INCOMPLETE |
| FR-X04 | Authorization | DEC-12 | D01/D06/SR07 | Common guard | jobs.review | Incomplete self-approval checks after reassignment | Additional cases needed | INCOMPLETE |

## 11. Undefined Decisions

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| IRV-004/005/006 | Technical details of existing outputs and authorization | Canonical types/D06/IR | Prevent implementers from filling gaps independently | Frontend design → independent Review |
| OPEN-03–06/11 | Production APIs/devices/authentication/external integration/termination responsibility | Prepare/D11 | Conditions for starting 1B | Backend / IoT / Security / Business |
| REV-018 | Long-running capacity guarantee | IR18 | Beyond 1A scale | Product Owner / Frontend |

## 12. Implementation Readiness

| Area | Assessment | Basis |
|---|---|---|
| Requirements completeness | CONDITIONALLY READY | Awaiting details for existing outputs and self-approval |
| Cross-document consistency | NOT READY | IRV-001/002/003/006 |
| UI/UX completeness | CONDITIONALLY READY | Awaiting severity and historical timestamp rules |
| Frontend architecture | READY | Responsibilities are separated for Repository/Query/RHF/URL/generations |
| API contract readiness | CONDITIONALLY READY | 1A Repository only. Awaiting IRV-004/005/006 corrections. Production HTTP is NOT READY |
| Error handling | READY | Rejection, recovery, and rendering rules in D01/D04/D10/IR17/24 |
| Authentication / Authorization | NOT READY | Self-approval gap in IRV-006 |
| IoT state handling | READY | No new findings in the independent supporting check. Simulated states only |
| Testability | CONDITIONALLY READY | Awaiting correction of IRV-001 and additional boundary examples |
| Agentic SDLC handoff readiness | NOT READY | G1 changes_requested; guidance points to an old baseline |

## 13. Required Actions Before Implementation

Correct the six findings, update the current manifest and static/type validation evidence, and resubmit to the same independent reviewers. Do not change G1 to passed before the corrections are rechecked. Application tests are not_run; implementation, production, and deployment approval belong to separate phases.
