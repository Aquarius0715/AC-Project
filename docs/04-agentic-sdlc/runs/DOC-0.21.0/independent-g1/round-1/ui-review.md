# DOC-0.21.0 independent UI / canonical-contract review

- Reviewer: independent `ui_requirements_review` agent; did not edit the reviewed specifications or fixtures.
- Target: working tree based on commit `717c545`; DOC-0.21.0.
- Baseline: `660c7855fdc3aaaf0bb3152d25788132ff838dd35067c87f7d9a28bf6e0ca802`.
- Independently recalculated SHA-256 of all 65 manifest files: all match. Recalculated canonical `spec_files` SHA-256: matches the baseline above.
- Scope: G120-001–005 correction and UI/DTO consequences, IR103–106, relevant older requirements and contracts, added acceptance cases, traceability and DEC-60/61. Frontend demo 1A only.
- Result for this bounded review: **PASS with one MINOR finding**. No BLOCKER, CRITICAL or MAJOR findings. Final consolidated G1 decision belongs to the parent review.

## Corrections verified by reading the contracts

1. G120-001: IR103, AT-A12-N/B and the fixture `AT-A12-N` consistently start duration at the first fresh current-tick evaluation. The fixture explicitly supplies observedAt=01:00, 120-second sensor TTL, ordinary 59+1 second ticks, separate finalEvaluation with unchanged facts, and counts limited to the newly saved Policy. The 60-second boundary remains inside TTL. IR103 distinguishes ordinary ticks from the jump behavior. AT-G121-001 records the boundary and repeat behavior. This is a document-level cross-check, not an executed policy test.
2. G120-002: IR95 now explicitly exempts templateKey=alert from same-name type mapping. IR104 retains IR10's sensor→fault mapping and sourceAlertId. The trigger in AT-G121-002 creates a policy-free sensor Alert with a new insulation_loss cause, avoiding the seed window_open duplicate key. Expected fault and warning values fit NotificationType and Severity.
3. G120-003: IR104 covers the mandatory notification severity for business events, preserves source-Alert severity, defines policy quality severity, and explicitly treats asynchronous device failure as a system actor so the creating Membership is not accidentally excluded. AT-P03-N recipient counts remain unchanged; schedule_change normal does not conflict with its original acceptance values. AT-C08-B continues to distinguish notification type icons from severity colors and alert counts. DEC-61 is proposed and reversible, not represented as company approval.
4. G120-004: IR105, the added IR71 allergen_observation row and ChangeEntityType agree. Row/event ID, version=1, creation/occurrence time, changedFields and eventId deduplication are specified. C07/A12 subscribe and invalidate telemetry.series; public delivery is bounded by current Unit scope and subscription unitIds. Older observedAt entries are re-read but do not replace the IR98 latest observation. AT-G121-004 supplies the complete trigger, duplicate and old-observation checks. No extra business DTO property is needed: AirSeries still carries the existing AllergenObservation.
5. G120-005: IR91 now delegates missing scopeVersionAtCreation to IR106. It uses the recipient Membership after all patches, preserves explicit nonnegative values and freezes the creation snapshot across later Membership changes. AT-C08-SRC's two abbreviated rows therefore acquire customer-a's scopeVersion=1. AT-G121-005 covers omission, explicit zero, invalid value, missing recipient and subsequent changes.

All five AT-G121 cases are included in verification.md and corresponding traceability rows. DEC-60/61 remain explicitly proposed, reversible demo choices. Other corrections merely make existing contracts representable and testable.

## MINOR UI21-001: new acceptance case asks for an undefined fault filter

- Location: `docs/04-agentic-sdlc/acceptance-review-021.csv:3` (AT-G121-002, Then).
- Related contracts: `docs/02-design/query-catalog.csv:2` and `:20`; `docs/02-design/client.md:309`; `docs/01-requirements/client.md:229` (AT-C08-B).
- Evidence: AT-G121-002 requires 「故障フィルターで表示」. Neither notifications.list nor alerts.list permits a type/fault filter. C08 defines severity and unread filtering; the existing type acceptance requires distinct icons and labels.
- Reproduction: create the specified load_alert and attempt to fulfill the new Then by sending notifications.list filters.type=fault. D12 rejects the unknown filter as VALIDATION; implementing a local filter would invent an unspecified feature and potentially filter only the current page.
- Impact: the final presentation assertion cannot be performed as written, although the underlying type=fault DTO correction is valid. This is a limited acceptance wording issue, not a need to expand the UI.
- Suggested correction: replace 「故障フィルターで表示」 with 「故障アイコン・ラベルで表示」, matching IR10 and AT-C08-B. Alternatively, explicitly specify a new filter across all affected catalogs if that is intended.
- Status: open at the pinned baseline.

## Limits

No specification edits, application execution, E2E tests, or production review were performed. No whole-document claim covering every historic acceptance subcase is made. Validation-program success is not treated as proof of semantic correctness; this review independently read the affected requirements, contracts and fixture values. The added report is outside the spec manifest and does not change the reviewed baseline.
