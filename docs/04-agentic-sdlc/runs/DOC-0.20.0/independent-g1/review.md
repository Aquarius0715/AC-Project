# DOC-0.20.0 independent G1 review

**Result: FAIL (1 MAJOR, 4 MINOR).** Passing static checks alone does not guarantee consistency between acceptance criteria and generated DTOs.

- Target commit: `717c545cccff8ab11932fab016c854ff8526897f`. Specification editor: Claude/Opus 5; reviewer: Codex.
- Target baseline: `4f45da02ef91f0cf93075848d6ded3373878521740aa2f4047673eee709e4acc`. All 63 files and baseline ID independently recalculated and matched.
- Scope: G1-001–031 correction contracts/acceptance plans and related authorization, state transitions, canonical DTOs, notifications, fixtures, and screen/Query contracts. Separate reviewers checked contracts, fixtures, and UI/requirements; the lead confirmed findings.
- Document review only, not app execution of every acceptance subcase or company approval. App implementation/runtime tests were not run. DEC-54–59 remain PROPOSED.

## Verification evidence

- [Static checks](static-check.json): `python3 docs/tools/validate_documents.py --tsc /private/tmp/claude-501/-Users-ji1wxs-PraditaProjects-AC-Project/ee33c3d9-cdb9-460b-a78b-8810c5cf9104/scratchpad/ts/package/lib/tsc.js`, exit 0, errors=[], TypeScript strict passed.
- [Mutation tests](validator-negative-checks.json): `python3 docs/tools/check_review_regressions.py`, exit 0, 84/84 detected.
- [Baseline check](baseline-integrity-check.json): All 63 SHA-256 values matched. Existing self-review results were not reused as independent judgments.

## Findings

### G120-001 / MAJOR — Air-quality policy acceptance does not establish the 60-second continuous condition

- Location: `docs/01-requirements/admin.md:324`, `docs/04-agentic-sdlc/fixture-contract.json:3590`, `docs/04-agentic-sdlc/fixture-contract.json:3603`, `docs/02-design/review-resolution-contracts.md:795`, `docs/02-design/deterministic-contracts.md:117`
- Related: FR-A12 / earlier finding G1-005, G1-009
- Reproduction: Apply the AT-A12-N / AT-G120-005 patch at 01:00Z and save a new Policy with durationSeconds=60. In the same tick, fire a Fact with observedAt=00:59Z for the first time.
- Problem: The test expects a created notification without 60 seconds of normal ticks, a duration counter, or prior policy evaluation. D08 requires a continuously true condition and does not create past Alerts from late past observations. A past observedAt alone cannot prove continuous satisfaction for a new policy. IR97 sensor additions fix the original missing_data but leave this mismatch.
- Impact: A correct duration implementation fails acceptance, or an implementation matching acceptance bypasses duration. At least the expected created notification cannot be achieved unambiguously.
- Suggested revision: After saving, inject valid current-time Facts for both units and specify 60 seconds of evaluation. Align notification expectations at 59/60 seconds and ventilation Command timing with D08/SR25. Update fixtures, AT-A12-N/B, and AT-G120-005 together.

### G120-002 / MINOR — Alert notification type conflicts between IR95 and the canonical DTO

- Location: `docs/02-design/review-resolution-contracts.md:753`, `docs/02-design/review-resolution-contracts.md:769`, `docs/02-design/review-resolution-contracts.md:64`, `docs/02-design/service-contracts.ts:168`
- Related: FR-X07, FR-C08, FR-T07, FR-A05 / earlier finding G1-002
- Reproduction: Use demo.trigger(load_alert) to create an Alert with policyId=null and type=sensor, then generate its notification under IR95.
- Problem: IR95 requires templateKey and type to share a name, implying type=alert, but NotificationType has no alert. IR10 maps the same notification to type=fault. IR72 does not allow implementation to choose between conflicting rules.
- Impact: Notification DTO validation disagrees with fault classification/filters. Rated MINOR because the existing mapping to fix is clear.
- Suggested revision: Exclude the alert template from the same-name rule and apply IR10 classification/sourceAlertId rules. Add acceptance for actual generated type=fault.

### G120-003 / MINOR — Business notification severity is undefined

- Location: `docs/02-design/review-resolution-contracts.md:753`, `docs/02-design/service-contracts.ts:110`
- Related: FR-X07, FR-P03, FR-C08 / earlier finding G1-002
- Reproduction: In AT-P03-N / AT-G120-002, contractor-a assigns work and generates schedule_change notifications for four recipients.
- Problem: IR95 defines recipients, template, channel, and other fields, but not required Notification.severity. Referenced D08/D12 also provide no default for non-Alert business notifications.
- Impact: Choosing normal/warning/critical depends on implementation, so colors, severity filters, and default order are not deterministic.
- Suggested revision: Define severity by business event, or a default with exceptions. State that Alerts inherit source severity and verify it in acceptance.

### G120-004 / MINOR — Allergen observation updates lack change events and Query invalidation rules

- Location: `docs/02-design/review-resolution-contracts.md:804`, `docs/02-design/review-resolution-contracts.md:446`, `docs/02-design/service-contracts.ts:315`
- Related: FR-C07, FR-A12, NFR-05 / earlier finding G1-006
- Reproduction: Load C07/A12 telemetry.series, then add a new observation to the same Unit using demo.trigger(allergen).
- Problem: IR98 defines a new store, but the closed IR71 change-event table and ChangeEntityType have no matching entry. No mapping to measurement is defined, so telemetry.series refetch behavior is undecided.
- Impact: The stored latest observation may disagree with displayed availability and values.
- Suggested revision: Add a dedicated entityType or define a mapping to existing events, including entityId/version and target Queries. Add acceptance proving cached screens update.

### G120-005 / MINOR — No rule fills required scopeVersionAtCreation in notification fixtures

- Location: `docs/04-agentic-sdlc/fixture-contract.json:3095`, `docs/04-agentic-sdlc/fixture-contract.json:3114`, `docs/02-design/review-resolution-contracts.md:658`, `docs/02-design/review-resolution-contracts.md:668`, `docs/02-design/service-contracts.ts:110`
- Related: FR-C08, FR-X07 / earlier finding G1-027
- Reproduction: Normalize AT-C08-SRC notif-alert-insulation-a / notif-alert-unknown-a using only IR91, then validate as Notification. Seed notifications have the same omission.
- Problem: The required numeric scopeVersionAtCreation is absent and is not generated by common or Notification-specific IR91 rules. It is neither nullable nor an array covered by default filling.
- Impact: Fixture generation following IR69/91 fails type validation. Implicitly filling the value requires an assumption absent from the rules.
- Suggested revision: Specify creation-time scopeVersion in each row or derive it from recipient Membership during fixture generation. Add required-field checks for seed and all notification patches.

## Verified fixes and limitations

Verified fixes: IR94 technician write conditions/candidate scope; IR96 cancel/release state table; IR100 component sets/submission validation; IR99 advice conditions; C08 aggregation Query; P03 jobId; AppShell role switching. Also checked in-range readings, Assignment/schedule synchronization, and older acceptance dates, DST, archive, connection/power states. G1-002/005/006/027 retain the issues above, so not all 31 findings are fully resolved.

These findings come from document-based reproduction using rules within the same baseline, not app execution. Reviewers differed on MAJOR/MINOR for notification classification; the lead rated MINOR because IR10 mapping is clear and the fix is local. Pending company approval/production integration decisions are not counted as 1A defects.

## Handover

Next owner: design correction agent. Apply G120-001–005 consistently to contracts, types, fixtures, and acceptance criteria; add needed regression checks. After specification changes, create a new baseline, run both validators and TypeScript strict, then have someone other than the editor reassess G1. Keep DOC-0.20.0 self-review records as historical results. Baseline text such as README was not changed in this review; its pending label is a pre-review snapshot. See this record and parent gate-G1.yaml for the current result.

Review materials created here were not committed or pushed.
