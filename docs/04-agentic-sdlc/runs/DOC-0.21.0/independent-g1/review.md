# DOC-0.21.0 Final Independent G1 Decision

**PASS — zero unresolved findings.**

The target is the uncommitted DOC-0.21.0 specification based on parent commit `717c545`. Final baseline: `02a1e39793df35f760afe9188e86141c8790cd7e92366efbddb7f62aff62e908`. Codex root edited the specifications; fixtures_review edited the validator. The [contract reviewer](contracts-review.md) and [UI/requirements reviewer](ui-review.md), neither involved in corrections, made independent decisions. This record combines their results.

## Corrections and re-review

1. Fixed DOC-0.20.0 findings G120-001–005. Applied A12 duration, notification classification/severity, allergen change events, and notification-fixture creation-time scope versions to contracts, inputs, acceptance criteria, and traceability.
2. The first independent re-review confirmed that the original five findings were resolved and found one new MINOR wording issue (UI21-001). Replaced "fault filter" with the existing "fault icon and label."
3. Both reviewers independently checked all 65 file hashes and changes against the new baseline, deciding PASS with zero unresolved findings. Only one acceptance CSV phrase differs from the previous candidate. First-round evidence remains in [round-1](round-1/ui-review.md).

## Validation

- [Static checks and TypeScript strict](../static-check.json): passed.
- [Mutation tests](../validator-negative-checks.json): 102/102 detected, comprising 84 existing and 18 new cases.
- [Acceptance input type checks](../contract-type-examples-result.json): complete Policy/Fire/Trigger inputs match canonical DTOs.
- [Baseline integrity](../baseline-integrity-check.json): all 65 files and the baseline SHA-256 match.
- [Cycle record](../review-cycle.json), [final gate](../gate-G1.yaml), [completion record](../completion.json).

## Scope and remaining checks

This decision covers remaining DOC-0.20.0 findings and consistency of the 1A specifications affected by their corrections. It does not claim that all older acceptance subcases were executed or that all existing specifications were fully re-reviewed. Application implementation/tests are not started/not run. Review of PROPOSED items before company acceptance (including DEC-60/61), production connection D11, and human/external review before deployment are separate stages. G1 pass is not company approval or deployment permission.

The specifications and review materials have not been committed or pushed.
