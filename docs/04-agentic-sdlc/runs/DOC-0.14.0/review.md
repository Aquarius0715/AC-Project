# DOC-0.14.0 Review and Correction Record

## 1. Executive Review Summary

Corrections and self-review of the 1A mock documents. Corrected six findings; cross-checking the corrections and their dependent types, screens, and conditions left zero unresolved findings. This does not mean the independent review of all specifications is complete. G1 is pending. DEC-16 records the address source and post-expiry report display approved by the user. Implementation has not started; application tests are not_run.

baseline: `f8e6cdd8387f1e623f69616518f41108e55ae9cd5f7ec4b223be5776238f4096`

Round 1: Found and corrected the six items below. Round 2: Rechecked the old prohibition on addresses, combinations of report availability and acceptance, the latest visible version, immutability after expiry, filters by kind, retaining/clearing disabled reasons, and type/catalog consistency. No additional findings.

Validation: Static validation of 64 requirements, 136 operations, and 47 screens, plus TypeScript strict, passed. All 20/20 validator tests that inject inconsistencies passed. These are not application runtime tests.

## 2. BLOCKER Issues

None found within this review scope.

## 3. CRITICAL Issues

- Issue ID: REV-002
- Severity: CRITICAL
- Document: 02-design/review-resolution-contracts.md
- Location: IR23 / JobHistorySnapshot
- Problem: redactedReportSummary is free text with no redaction rules
- Why it matters: Post-expiry visibility cannot be implemented unambiguously
- Example Failure: Addresses or phone numbers remain in the report body
- Required Fix: Limit it to the approved report-availability and acceptance states
- Suggested Revision: IR25: Three ReportHistorySummary states; freeze the latest visible version when access ends
- Status: Corrected and rechecked by the author

## 4. MAJOR Issues

- Issue ID: REV-001
- Severity: MAJOR
- Document: 02-design/service-contracts.ts
- Location: JobOfferSummary / FR-P01
- Problem: No input source exists for regionLabel
- Why it matters: This causes differences from the installation location and duplicate data management
- Example Failure: HQ guesses and enters the region for each job
- Required Fix: Use the installation property as the only source
- Suggested Revision: IR25: siteAddress=Property.address; null when not registered
- Status: Corrected and rechecked by the author

- Issue ID: REV-003
- Severity: MAJOR
- Document: 02-design/query-catalog.csv
- Location: summaries.get / DD-P01・T01
- Problem: The summary API cannot accept the job screen status and severity conditions
- Why it matters: The requirement to use the same conditions for the list and KPIs cannot be met
- Example Failure: The warning list also displays critical counts
- Required Fix: Align job summary filters and projection rules
- Suggested Revision: IR26: Add job conditions for partner/technician; reject them for customer
- Status: Corrected and rechecked by the author

- Issue ID: REV-004
- Severity: MAJOR
- Document: 02-design/service-contracts.ts
- Location: RuleBase / AT-A04-B
- Problem: The requirement to show a disabled reason after a capability change has no output field
- Why it matters: The customer screen cannot retrieve the reason without audit permission
- Example Failure: Only enabled=false is shown and the disabled reason is lost
- Required Fix: Define Repository-generated disabled reasons and when to clear them
- Suggested Revision: IR27: disabledReason; validation and clearing on explicit re-enabling
- Status: Corrected and rechecked by the author

- Issue ID: REV-005
- Severity: MAJOR
- Document: 02-design/service-contracts.ts
- Location: capabilities.save / DD-A04
- Problem: The DD requires a reason only for updates; the type also requires it for creation
- Why it matters: Implementations of new-entry input may differ
- Example Failure: Creation sends an undefined dummy reason
- Required Fix: Make it optional on creation and required on updates in types and validation
- Suggested Revision: IR28: changeReason is optional; an existing ID requires a nonempty value
- Status: Corrected and rechecked by the author

## 5. MINOR Issues

- Issue ID: REV-006
- Severity: MINOR
- Document: 02-design/common.md
- Location: §4
- Problem: The stated operation count is 135, but the catalog contains 136
- Why it matters: Handover may wrongly identify an operation as missing
- Example Failure: Implementing 136 operations may be judged to include an extra operation
- Required Fix: Match the actual catalog count
- Suggested Revision: Correct it to 136 and check the count with the validator
- Status: Corrected and rechecked by the author

## 6. Open Questions

The business decisions about address and report display are resolved. Independent review by another AI has not been performed because the delegation question has not been answered. Self-review does not replace independent approval.

## 7. Cross-document Inconsistencies

Corrected summary conditions, DTOs, required fields, and counts in REV-003–006. Also updated the old BR-P01/AT-P01-B/DTO descriptions that prohibited pre-acceptance addresses to reflect the user decision.

## 8. Missing Requirements

No new features are added. The existing potential long-running memory-capacity requirement REV-018 remains deferred under IR18. The existing demo scope of 100 units/1000 samples is unchanged.

## 9. Edge Cases Not Defined

No undefined cases remain in this correction scope. The additional acceptance plan covers null addresses, address changes, Offers from other companies, expiry, no report, an accepted old version with an unaccepted new version, acceptance after expiry, language changes, invalid filters, re-enabling, and missing reasons. Existing disconnection, timeout, permission expiry, duplicate operations, and concurrent update cases continue to follow D01/D04/IR17/IR24. HTTP 400/401/403/404/409/429/500 belong to production contracts; 1A simulates DomainError. This does not show application execution results.

## 10. Traceability Matrix

[Traceability table for 64 requirements](traceability-matrix.csv). Status=OK is a structural assessment confirming cross-document references and the presence of acceptance criteria. It does not mean all 64 requirements passed an independent semantic review or application tests. The [additional acceptance plan](../../acceptance-loop.csv) covers behavior for five corrected items; static validation covers the count correction.

## 11. Undefined Decisions

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| G1 | Independent review by another reviewer | agents/review.md, DEC-12 | Independent approval of one's own corrections is prohibited | Independent Review Agent |
| OPEN-03–06/11 | Production connections, devices, authentication, external transactions, and termination responsibility | Prepare, D11 | Outside 1A scope. Production contracts are NOT DEFINED | Backend / IoT / Security / Business |
| REV-018 | Capacity limits for continuous operation | IR18 | A potential requirement beyond the current demo scale | Product Owner / Frontend |

## 12. Implementation Readiness

| Area | Assessment | Basis |
|---|---|---|
| Requirements completeness | CONDITIONALLY READY | Corrected; awaiting independent G1 for the full scope |
| Cross-document consistency | CONDITIONALLY READY | Static and changed-section checks passed; awaiting independent G1 |
| UI/UX completeness | CONDITIONALLY READY | Typed display contracts included; awaiting independent G1 |
| Frontend architecture | CONDITIONALLY READY | Existing design retained; awaiting independent G1 |
| API contract readiness | CONDITIONALLY READY | 1A Repository type checks passed. Production HTTP is NOT READY |
| Error handling | CONDITIONALLY READY | Existing DomainError and added boundaries traced; awaiting independent G1 |
| Authentication / Authorization | CONDITIONALLY READY | Mock authorization. Address disclosure decision included; awaiting independent G1 |
| IoT state handling | CONDITIONALLY READY | Simulated states only; awaiting independent G1 |
| Testability | CONDITIONALLY READY | Acceptance plan and static checks available; awaiting independent G1 |
| Agentic SDLC handoff readiness | NOT READY | Independent review not performed; G1 not passed |

## 13. Required Actions Before Implementation

Send the same baseline to another Review Agent. If findings remain, continue correcting and reviewing again. The work needed for all G1 items to be OK is not complete. Do not infer approval for implementation, acceptance testing, production connections, or deployment from this document review.
