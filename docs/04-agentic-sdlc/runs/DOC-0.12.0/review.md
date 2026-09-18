# 0.12.0 Re-review and Correction Results

Corrected eight findings from the re-review. Three rounds of checks included a review of the effects of the corrections and final validation. No unresolved inconsistencies remain in the changes reviewed here.

Target baseline: `254bd3088246362701d9816ab7cf4334a1d299e87cb93a347297363824bc36f0`. This review was performed by the correction author and is separate from approval by another reviewer.

| Finding | Reproduction conditions and expected result after correction | Contract |
|---|---|---|
| CV-001 | One restriction covers two units, but only one is readable → read get/list/notifications/same-key results → individual read returns NOT_FOUND; exclude it from list counts; send advance notice only to recipients who can read all targets | [Corrected specification](../../../02-design/review-resolution-contracts.md#ir19-visibility-of-restrictions-on-multiple-equipment-units--cv-001) |
| CV-002 | After acceptance but before access starts, or after the Offer expires → retry with the same key/getResult; decide again with another key → return only the original Membership receipt. A new decision returns CONFLICT | [Corrected specification](../../../02-design/review-resolution-contracts.md#ir01-minimum-responses-for-acceptance-and-decline) |
| CV-003 | Override-only list → sort by createdAt/updatedAt and paginate → the limited DTO includes these values; use the same order with id as tie-breaker | [Corrected specification](../../../02-design/review-resolution-contracts.md#ir03-permission-limited-to-forced-release) |
| CV-004 | paid/processing/not yet overdue, or non-HQ → call reminder preview/recipients/remind → state CONFLICT or FORBIDDEN; zero notifications/version changes | [Corrected specification](../../../02-design/review-resolution-contracts.md#ir20-reminder-preview-preconditions--cv-004) |
| CV-005 | 1 kW at the minute start and 9 kW 30 seconds later → integrate one minute; add a candidate with the same sequence → use only the start value, 1/60 kWh; decide by id order; do not replace it with a later sample | [Corrected specification](../../../02-design/review-resolution-contracts.md#ir08-energy-data-origin) |
| CV-006 | Unknown raw unit and mismatched inspection unit → save through demo.trigger and jobs.saveDraft → raw is suspect; reject the whole inspection with VALIDATION; use no nonexistent operation names | [Corrected specification](../../../02-design/review-resolution-contracts.md#ir22-saving-raw-measurements-and-inspections--cv-006) |
| CV-007 | Retained facts and a Sensor with the current binding → simulate→fire, bind, duplicate facts, retry → simulate has no side effects; discard on binding change; reject duplicates; do not reevaluate on retry | [Corrected specification](../../../02-design/review-resolution-contracts.md#ir21-fact-evaluation-time-and-sensors--cv-007) |
| CV-008 | RawMeasurement input time/eventID → valid time, future time, mismatched outer eventID, sensor from another scope → preserve valid input time; future time is suspect; mismatch returns VALIDATION; out-of-scope returns NOT_FOUND | [Corrected specification](../../../02-design/review-resolution-contracts.md#ir12-raw-and-normalized-measurements) |

Correction cycles:

1. Compared permissions, types, inputs, and state transitions in 0.11.0 and corrected eight findings.
2. Checked callers and synchronized the operation catalog for acceptance retries, reminder screen descriptions, and before-save/after-save notice confirmation.
3. Completed final checks of traceability tables, types, and catalogs, and checked 12 cases that inject inconsistencies into the validator.

Validation results:

- [Static validation and TypeScript](static-check.json): 64 requirements, 136 operations, 47 screens, and 57 Components; zero errors.
- [Validator error detection](validator-negative-checks.json): Detected the intended inconsistencies in 12/12 cases. These are not application behavior tests.
- [Additional acceptance plan](../../acceptance-convergence.csv): Linked eight cases to the traceability table; execution status is not_run.
- The four primary source files match the 0.11.0 hashes. Records from earlier versions are retained.

Rerun:

```sh
python3 docs/tools/validate_documents.py
python3 docs/tools/validate_documents.py --tsc /private/tmp/ac-typescript-check/package/lib/tsc.js
python3 docs/tools/check_review_regressions.py
```

The TypeScript path points to the compiler already available in this environment. In another environment, specify the installed lib/tsc.js.

Application implementation and runtime testing are outside this task and have not been performed. The long-running capacity guarantee in REV-018 remains tracked separately, outside the existing demo scope. Resolving the identified document inconsistencies does not guarantee that no unknown problems exist. See the [machine-readable record](design-review-result.json).
