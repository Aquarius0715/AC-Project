# 0.13.0 Re-review and Correction Results

Corrected five findings about job visibility. The first round corrected four findings involving lists, history, summaries, and pagination. The review after those corrections found and corrected one history-type issue for internal technicians, followed by a final cross-check. No unresolved findings remain in the reviewed changes and their dependencies.

baseline: `941c5a3152e4fa6057bcb33792a09fdfa64955f14059f951bb8071eb39282da5`

| Finding | Check conditions | Expected result after correction |
|---|---|---|
| [PV-001](../../../02-design/review-resolution-contracts.md#ir23-job-list-projections-and-search--pv-001003) | Change the Alert/Job state behind an unaccepted Offer / search and sort by status/severity/unitId | Non-public changes do not change counts or order |
| [PV-002](../../../02-design/review-resolution-contracts.md#ir23-job-list-projections-and-search--pv-001003) | Access to a previously accepted Offer ends / open history through jobs.list/get | Only typed frozen history is shown; no unit ID or editing links |
| [PV-003](../../../02-design/review-resolution-contracts.md#ir23-job-list-projections-and-search--pv-001003) | An accepted Offer starts in the future, plus history after expiry / retrieve the list and partner summary with the same conditions | Use the same projected set; operational counts include only currently valid summary entries |
| [PV-004](../../../02-design/review-resolution-contracts.md#ir24-reduced-visibility-during-paging--pv-004) | An Offer expires after list page 1, with scopeVersion unchanged / fetch page 2 and an old callback | snapshot CONFLICT; update viewEpoch; retrieve history from the first page |
| [PV-005](../../../02-design/review-resolution-contracts.md#ir23-job-list-projections-and-search--pv-001003) | An internal Assignment with contractor ID=null ends / the assignee and another assignee retrieve lists/details/summaries | The original assignee gets frozen history with contractorOrgId=null; history is not transferred to another assignee or included in operational counts |

Validation results:

- [Static validation and TypeScript](static-check.json): 64 requirements, 136 operations, and 47 screens; traceability, type, and catalog consistency checks passed.
- [Validator error detection](validator-negative-checks.json): Detected the intended inconsistencies in 15/15 cases.
- [Acceptance plan](../../acceptance-projection.csv): Added five cases and linked them to the traceability table. Post-implementation application tests remain not_run.
- The four primary source files match the previous version's hashes. Evidence from earlier versions is retained.

Rerun:

```sh
python3 docs/tools/validate_documents.py
python3 docs/tools/validate_documents.py --tsc /private/tmp/ac-typescript-check/package/lib/tsc.js
python3 docs/tools/check_review_regressions.py
```

In another environment, specify the path to the existing TypeScript lib/tsc.js.

This is a document review by the correction author, not approval by another reviewer or application runtime testing. It does not guarantee that no unknown defects exist. REV-018, a potential requirement for a long-running capacity guarantee, remains tracked separately outside the existing demo scope.
