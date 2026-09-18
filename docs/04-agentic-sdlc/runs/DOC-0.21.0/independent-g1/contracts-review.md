# DOC-0.21.0 Independent G1 Contract Re-review

- Reviewer: contracts_review (a separate agent who did not edit the specifications)
- Target: Uncommitted working tree based on parent commit `717c545`, DOC-0.21.0
- spec_baseline_id: `02a1e39793df35f760afe9188e86141c8790cd7e92366efbddb7f62aff62e908`
- Decision: **PASS for the scope below**. Zero new findings.
- No specification files changed. Only this review evidence file was added.

## Independent checks

Recomputed SHA-256 for manifest spec_files using the specified JSON normalization; it matched the baseline above. Recomputed every target file's hash from its bytes and found zero mismatches.

In the final check, comparing the round-1 and current manifests showed that only acceptance-review-021.csv changed. The diff against saved acceptance-review-021-before.csv was a single AT-G121-002 phrase: "display with a fault filter" → "display with a fault icon and label." The corrected wording matches IR10 fault classification and resolves UI21-001. Types, time evaluation, fixtures, and notification rules did not change, so the following contract decisions also hold for the final baseline.

Checked findings G120-001–005 and the impact of their corrections against IR95/IR103–106, canonical DTOs, D02/D07/D08/D12/D15, SR21/SR25/SR28, IR10/IR21/IR45/IR54/IR71/IR83/IR91/IR98, A12 text, fixtures, and acceptance-review-020/021. Also read the added validate_documents.py checks. The editors' reported static-check success is not counted as an independent execution here.

| Finding | Result | Evidence |
|---|---|---|
| G120-001 | Resolved | IR103 and AT-A12-N flow save the Policy, supply current-time Facts, then advance ordinary one-second ticks 59 times and once more. At elapsed=0/59, the Policy causes no side effects; it first matches at 60. observedAt stays 01:00Z and remains within TTL120 seconds at 01:01Z. simulator=false prevents automatic measurements from overwriting it. Under IR45, elapsed time alone does not make the connection offline. The seed has no competing Policy/Automation or ongoing Command on the target Unit. |
| G120-002 | Resolved | IR95 adds the alert exception to the same-name rule. IR104 explicitly maps sensor/tamper/reconciliation_required→fault, maintenance→cleaning_due, quality→quality. Preserved sourceAlertId and canonical NotificationType agree. |
| G120-003 | Resolved | IR104 defines required severity for every template. Policy notifications keep configured severity, without replacement by business-notification defaults. It also defines restriction-stage values, previews, and actor exclusion for asynchronous device_operation.failed. |
| G120-004 | Resolved | IR105, ChangeEntityType, Subscribe, and the IR71 table all include allergen_observation. They define saving and an event in the same transition, scope, retries, latest-observation selection, and C07/A12 refetch. |
| G120-005 | Resolved | IR106/IR91 fill missing scopeVersionAtCreation from the recipient Membership after all patches. Keep explicit 0; reject invalid values/nonexistent recipients. Later Membership updates do not rewrite historical snapshots. |

## Focused checks of time evaluation and the same tick

IR103 finalEvaluation changes only occurredAt to 01:01Z, retaining the stored Fact's observedAt/value/quality and sending phase=condition. It does not violate D02's rule that different facts or an added phase cause CONFLICT; it retrieves the same-tick result already evaluated internally under IR54. It does not request reevaluation that would change the result to busy/cooldown. SR25's independence of unsupported ventilation and successful notification also remains: the Policy creates notifications for both Units, but a Command only for the supported Unit.

IR104 severity is an added business-demo design detail (DEC-61), not company approval. IR103 time boundaries are also recorded as the DEC-60 proposal, adding no production decisions.

## Limits

This is an independent review of documents and fixture contracts. Application implementation, browser, and Repository execution tests have not been run. It is not a full re-review of every existing requirement. The parent review combines static, TypeScript, and mutation execution evidence with other reviewers' scopes for the final G1 decision. It does not mean company acceptance or production Security/IoT/API approval.
