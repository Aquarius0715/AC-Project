# DOC-0.11.0 Correction and Re-review Results

Corrected the original 17 findings and six additional findings from the re-review. One memory-limit item for long-running sessions remains deferred as a possible additional requirement. Self-review of the design documents is complete; independent G1 remains pending.

Target baseline: `43d3c2b9c31307109808c42d41a7c71b6cac91c12af81dceca7ed29c5d475648`. The old 0.10.0 review/manifest is kept as history and is not reused for this approval.

## Correction and Re-review Progress

1. Round 1: Assessed the 18 original findings and reflected 17 in types, permissions, state transitions, and screen/operation/version catalogs. Clarified the supported scope for one potential capacity requirement.
2. Round 2: Corrected six items: list entry and filter permissions for forced release, source IDs for notification categories, payment operation names, Policy form dependencies, required values when creating measurements, and view generations and cursors.
3. Round 3: Checked the effects of the changes again. Kept the existing 120-character Policy name limit, merged the form tables, and synchronized the operation lists in the detailed DDs. Verified static checks, type checks, and validator error detection.

## Responses to Original Findings

| ID | Finding | Response |
|---|---|---|
| REV-001 | Write responses for acceptance/decline conflict with disclosure limits | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir01-minimum-responses-for-acceptance-and-decline) |
| REV-002 | No contract preserves historical data ownership boundaries after a customer change | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir02-customer-ownership-is-immutable) |
| REV-003 | A separate forced-release permission cannot reach the required reads | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir03-permission-limited-to-forced-release) |
| REV-004 | The requirement to show HQ reminder previews on customer screens has no save path | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir04-shared-simulated-reminder-records) |
| REV-005 | Backdating a notice can satisfy the 24-hour notice requirement | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir05-advance-notice-time-and-evidence) |
| REV-006 | Restarting with another key while Payment is initiated is undefined | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir06-only-one-active-payment-attempt) |
| REV-007 | Screens cannot build the required inputs for alert and air policies | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir07-shared-policy-form) |
| REV-008 | A baseline created from estimated power can be labeled measured | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir08-energy-data-origin) |
| REV-009 | The shared voice panel cannot complete a technician diagnostic command | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir09-operation-context-for-voice-changes) |
| REV-010 | The DTO has no identifier to distinguish cleaning schedules from fault notifications | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir10-notification-business-categories) |
| REV-011 | Actual-result calculation boundaries are unclear, so matching boundaries cannot be checked | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir11-calculation-boundary-identity) |
| REV-012 | The contract to display unknown measurement units as suspect conflicts with DTO rejection | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir12-raw-and-normalized-measurements) |
| REV-013 | The latest design and old acceptance criteria require different results | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir13-align-current-acceptance-conditions) |
| REV-014 | The version guidance at the start of documents does not match the current baseline | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir14-identify-the-current-version) |
| REV-015 | Is MRV export a 1A feature? | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir15-mrv-output-scope) |
| REV-016 | Freshness and retention limits for non-sensor Facts are undefined | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir16-fact-freshness-and-reuse) |
| REV-017 | When to increment the session generation is undefined | [Specification corrected](../../../02-design/review-resolution-contracts.md#ir17-repository-generation-and-view-generation) |
| REV-018 | Missing Requirement Candidate: memory limit for long-running demos | [Deferred as a possible additional requirement](../../../02-design/review-resolution-contracts.md#ir18-long-term-memory-retention) |

## Validation

- [Static validation results](static-check.json): 64 requirements, 136 operations, 47 screens, 57 Components, and 93 write branches; zero errors in traceability, links, or catalog consistency. TypeScript strict/noEmit also passed.
- [Validator error-detection results](validator-negative-checks.json): All eight intentionally broken cases produced the expected diagnostics. Checks used temporary copies without changing the original specifications.
- [Acceptance plan](../../acceptance-resolution.csv): 18 original cases plus six additions. All have execution_status=not_run. This is not evidence that the application ran.
- Verified that hashes of primary source materials in the old baseline match. See the [re-review record](design-review-result.json).

Commands to rerun:

```sh
python3 docs/tools/validate_documents.py
python3 docs/tools/validate_documents.py --tsc /private/tmp/ac-typescript-check/package/lib/tsc.js
python3 docs/tools/check_review_regressions.py
```

`--tsc` used the TypeScript compiler already available in this environment. In another environment, specify the installed lib/tsc.js.

## Handover

The changes cover frontend 1A design documents. Application implementation and runtime testing have not been performed. REV-018 concerns capacity guarantees for continuous long-running use beyond 100 units/1000 samples. No automatic deletion was added that would break the existing contract to retain data until reset. Limits, rejection, and offloading will be designed if that usage scope is adopted in the future.

This record is a re-review by the correction author, not independent G1 approval. The [gate](gate-G1.yaml) remains pending.
