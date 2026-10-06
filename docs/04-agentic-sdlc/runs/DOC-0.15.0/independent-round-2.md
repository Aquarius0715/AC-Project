# DOC-0.15.0 Independent Re-review Round 2

Reviewer: `/root/independent_review`. Specification correction author: `/root`. The reviewer did not edit the specification text.

Scope: Prepare, requirements, detailed design, UIUX, and supporting contracts for the 1A frontend mock. baseline: `f11600bc8735d2a10797b418bb13da76601b8021184a7d4cef6c2a70d355dd5f`.

## 1. Executive Review Summary

**G1: passed. Zero unresolved findings.** Confirmed resolution of all six previous findings and rechecked the effects on public projections, retries, versioning, lists/summaries, prohibited inputs, UI availability, and acceptance criteria. Zero additional findings. The documents may be handed over to the 1A implementation Agent.

Independently compared the actual SHA-256 values for all 53 files with the manifest; zero mismatches. Recalculating the baseline using canonicalization also matched. Static and type-check passes were not used in place of semantic review. Confirmed that the corrected contracts resolve the counterexamples below. The application remains unimplemented and runtime tests remain not_run.

| Issue ID | Specific conditions rechecked | Basis for resolution | Assessment |
|---|---|---|---|
| IRV-001 | Filter one offered, one accepted, and one submitted job by status=offered | AT-P01-N separates unfiltered 1/1/1 from filtered 1/0/0. IR26 and AT-G1-001 agree | resolved |
| IRV-002 | Show an address for an unaccepted Offer without showing entry instructions | DD-A02 separates address and entry-instruction descriptions, matching IR25/DEC-16. AT-G1-002 | resolved |
| IRV-003 | Follow current-input links from the index and SDLC | Both point to DOC-0.15.0; all 53 file hashes match. AT-G1-003 | resolved |
| IRV-004 | Accept → retry with the same key → change cost/notes → access ends | IR29/canonical MaintenanceJob.completedAt: null on creation, now once on acceptance, unchanged thereafter, frozen in history. Plan generation in D16 also uses null. AT-G1-004 | resolved |
| IRV-005 | resolved critical/open warning, then acknowledged critical, finally all resolved | IR30 gives warning→critical→normal using the same-Unit/visible/unresolved set, separate from Job.alertIds. offer/history retains null. AT-G1-005 | resolved |
| IRV-006 | T1 is original author → T2 changes only text/measurements/photos → T2 reviews through another Membership/HQ | IR31 inherits version contributors and adds actual editors; both accept and return are FORBIDDEN. T3 is allowed under normal conditions. Per-actor reviewAvailability and disabled states in P05/A06 agree. AT-G1-006 | resolved |

## 2. BLOCKER Issues

None.

## 3. CRITICAL Issues

None unresolved. IRV-006 is resolved. The internal contributor set is not added to UI inputs or external DTOs; only the current actor's availability/reason is exposed. Read/write retries after a permission switch reproject availability while internal history stays unchanged. A missing contributor set returns UNAVAILABLE, without falling back to permission.

## 4. MAJOR Issues

None unresolved. IRV-001/004/005 are resolved. History whose access ended before completion retains completedAt=null and does not reflect later completion or acceptance. Severity normal means “no unresolved Alert”; it is not a guarantee that communication or measurements are normal.

## 5. MINOR Issues

None unresolved. IRV-002/003 are resolved.

## 6. Open Questions

No unanswered questions block 1A implementation. The approved scope of DEC-12–16 is retained; no new commercial approval has been created.

## 7. Cross-document Inconsistencies

Conflicts involving the six findings across requirements, DD, IR, canonical types, UI, traceability, and additional acceptance criteria are resolved. Existing priority rules apply IR→SR→D/old DD to the same topic. Decisions from past runs apply only to their original baselines and are not reused for this version.

## 8. Missing Requirements

No unresolved required features are missing from the current 1A scope. The long-running continuous-operation/memory-capacity guarantee in IR18 is an existing deferred candidate and must not be added to the completion criteria for the 100-unit/1000-sample demo.

## 9. Edge Cases Not Defined

No unresolved implementation blockers remain within current 1A scope. In addition to the D/SR/IR checks in Round 1, the following were rechecked for these changes.

- Null addresses, address changes during a page snapshot, no address retention after expiry, and fixed availability/acceptance states for the latest visible report version.
- An unaccepted latest version, acceptance after expiry, null completion timestamps/date boundaries, and no duplicate completion timestamps or contributors on retries.
- Visible/non-visible/resolved Alerts, zero alerts, matching summary filters and snapshots, and undisclosed severity for offer/history.
- Original authors and co-editors, photo-only/measurement-only edits, assignment/viewing/unchanged saves, another Membership, HQ escalation, missing contributors, and permission/version changes during UI confirmation.
- Retaining disabled reasons after capability changes, explicit re-enabling, and optional reasons for model creation but required reasons for updates.

D01/D04/D05/D07/SR14/IR17/IR24 applies to communication, timeouts, idempotency, expiry, old callbacks, and IoT states. The supporting independent review in Round 1 found no new issues in restriction control, binding, asynchronous behavior, or payments; these contracts have not changed in this round. Production HTTP connection specifications are outside scope.

## 10. Traceability Matrix

[All 64 requirements in the independent re-review](independent-round-2-traceability.csv): **64 OK**. This assessment confirms document mappings from Prepare→FR→DD→UIUX→Repository→Error Handling→AT and resolution of the semantic inconsistencies found here. It does not mean application tests passed. AT-G1-001–006 have execution_status=not_run.

## 11. Undefined Decisions

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| OPEN-03–06/11 | Production APIs/devices/authentication/external integration/termination responsibility | Prepare/D11 | Conditions for starting 1B; outside 1A G1 scope | Backend / IoT / Security / Business |
| REV-018 | Long-running capacity guarantee | IR18 | Expansion beyond demo scale | Product Owner / Frontend |
| deploy-final-review | Final review before deployment | DEC-12 | Separate AI document G1 from publication approval | Humans and external reviewers |

## 12. Implementation Readiness

All assessments are limited to document readiness for the 1A mock.

| Area | Assessment | Basis |
|---|---|---|
| Requirements completeness | READY | Mappings for 64 requirements and existing responsibilities checked; missing-item findings resolved |
| Cross-document consistency | READY | Six findings resolved; types/text/catalogs/AT/current inputs agree |
| UI/UX completeness | READY | Retrieval paths exist for states, confirmations, forms, display quality, and availability |
| Frontend architecture | READY | Responsibilities separated for Repository/Query/RHF/URL/generations |
| API contract readiness | READY | 1A Repository inputs, results, authorization, versions, and errors defined. Production HTTP is NOT READY |
| Error handling | READY | D01/D04/D10 plus expiry, retry, and partial-failure contracts |
| Authentication / Authorization | READY | Mock scope, expiry, public projections, and rejection of self-approval for co-edited versions |
| IoT state handling | READY | Simulated request/response/deadline/restriction recovery/old-binding separation |
| Testability | READY | Existing ATs and six additions map to specific inputs and expected results |
| Agentic SDLC handoff readiness | READY | Independent re-review, pinned current baseline, and separation of unexecuted phases |

## 13. Required Actions Before Implementation

No specification corrections remain. The orchestration owner can update the G1 record based on this independent decision. Pin this baseline when implementation starts. From G2 onward, perform application type checks, lint, build, unit/Component/E2E/a11y tests. Library compatibility/lockfile checks and pinning measured browser versions at implementation start follow the existing plan. Do not infer completion or approval of implementation, production connections, or deployment from this report.

Validation evidence: `static-check.json` (64 requirements, 136 operations, 47 screens, TypeScript strict, errors=[]), `validator-negative-checks.json` (27/27 detected), and `contract-type-examples-result.json` (positive and negative type examples, exit 0). These are document/type checks; application tests are not_run.
