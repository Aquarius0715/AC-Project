# DOC-0.21.0 Correction Report

Applied G120-001–005 from the independent G1 review of DOC-0.20.0 to IR103–106 and the acceptance plan. Codex root edited the specifications; fixtures_review edited the validator. Their checks are self-checks, not independent G1 approval. See [gate-G1.yaml](gate-G1.yaml) for the latest independent decision.

| Finding | Correction | Acceptance |
|---|---|---|
| G120-001 | IR103: Measure duration from the new policy's current tick; distinguish 59 from 60 seconds. Update A12 input and execution order | AT-G121-001 |
| G120-002 | IR104: Apply IR10 classification to the alert template | AT-G121-002 |
| G120-003 | IR104: Add severity table for business notifications | AT-G121-003 |
| G120-004 | IR105: Add allergen_observation change event and telemetry.series invalidation | AT-G121-004 |
| G120-005 | IR106: Validate explicit scopeVersionAtCreation in notification fixtures and fill omitted values | AT-G121-005 |

DEC-60–61 are PROPOSED, reversible demo details, not company approvals or final production specifications. Preserve earlier independent G1 evidence under its old baseline; do not reuse it. Application implementation and behavior tests have not been performed. No commit or push was made.

## Checks by the editors

[Static checks](static-check.json) returned errors=[]; TypeScript strict passed. [Mutation tests](validator-negative-checks.json) detected 102/102 cases: 84 existing plus 18 new. [Additional input type checks](contract-type-examples-result.json) matched complete policy-save, initial/final evaluation, and load_alert/allergen inputs against canonical DTOs. [Baseline verification](baseline-integrity-check.json) matched all 65 files. These are document/type checks; application behavior tests have not been run.

## Additional correction after independent review

The first independent review found a minor acceptance wording defect: AT-G121-002 required an undefined "fault filter" (UI21-001). Changed it to C08's existing "fault icon and label." No feature, DTO, or permission changed. Saved the first baseline and evidence under `independent-g1/round-1/`; reassess against the final baseline.

## Final result

For final baseline `02a1e39793df35f760afe9188e86141c8790cd7e92366efbddb7f62aff62e908`, contracts_review and ui_requirements_review, who did not make the corrections, independently decided PASS with zero unresolved findings. See the [final combined report](independent-g1/review.md) and [two-round cycle record](review-cycle.json).
