# Agentic SDLC Artifact Templates

These templates are for copying and reuse. Do not fill blank approval or execution results without facts and claim completion. Use the following schema for each type of status.

## Status schema

| Target | Field and allowed values | Completion/transition rules |
|---|---|---|
| Task | task_status: planned/ready/in_progress/in_review/done/blocked | Move from planned to ready only after checking dependencies, acceptance subcases, and specification baseline. done requires passing all required gates and supporting evidence |
| Test | test_result: not_run/passed/failed/blocked | Use not_run if not executed. Use blocked if the expected result is unknown or the environment is unavailable. Decide from actual results and all subcases |
| Gate | gate_result: pending/passed/failed/blocked/stale | Use pending before a decision. If relevant specifications change, mark earlier passes stale and reassess them |
| Evidence validity | evidence_status: current/stale | Keep past actual test results unchanged; track separately whether they apply to the current specification |
| Finding | finding_status: open/in_progress/resolved | Mark resolved only after confirming the fix and retest |
| Decision | decision_status: open/proposed/accepted/rejected/superseded | accepted requires actual decided_by/at/answer values. Permission to implement a proposal is separate from business approval |

The draft/proposed values in document front matter describe the document itself, separately from this execution schema. Do not combine different status types into one status column.

## Task input (orchestration agent → assigned agent)

```yaml
task_id: AC-000
document_version: 0.10.0
spec_baseline_id: null
spec_files: [] # path and sha256; include requirements, design, ATs, common contracts, and sources
acceptance_manifest_hash: null
task_status: planned
agent_role: implementation
objective: "Concrete outcome for the assigned requirements"
requirement_ids: [FR-C03, FR-X04]
design_ids: [DD-C03, DD-COMMON]
acceptance_ids: [AT-C03, AT-X04, S01]
read_first:
  - docs/README.md
  - docs/02-design/common.md
owned_files: []
dependencies: []
assumptions: [DEC-02]
out_of_scope: [production_api, physical_device_control]
allowed_actions: [read, frontend_edit, local_test]
deliverables: []
completion_criteria: []
```

## Handoff (all agents)

```yaml
task_id: AC-000
agent_role: implementation
task_status: planned
revision_or_diff_hash: null
spec_baseline_id: null
acceptance_manifest_hash: null
evidence_status: current
requirements_addressed: []
files_changed: []
decisions_added: []
checks:
  - command: null
    environment: null
    test_result: not_run
    evidence_path: null
unresolved_findings: []
human_decisions_needed: []
next_agent: test
next_action: ""
```

## Test cases and execution records

```yaml
case_id: AT-C03
requirement_ids: [FR-C03]
design_ids: [DD-C03]
revision_or_diff_hash: null
spec_baseline_id: null
acceptance_manifest_hash: null
evidence_status: current
fixture: seed-A
clock: "2026-09-14T01:00:00Z"
role_and_scope: "client / customer-a"
preconditions: []
steps: []
expected: []
actual: null
test_result: not_run
command: null
browser_viewport: null
evidence_paths: []
defect_id: null
```

## Review findings

```yaml
finding_id: RV-000
severity: P1
requirement_ids: []
file_and_line: null
reproduction: []
expected: ""
actual: ""
impact: ""
suggested_fix: ""
owner: implementation
finding_status: open
retest_evidence: null
```

Finding priority: P0 = unauthorized operations, accidental real processing, or major data corruption; P1 = defects in required flows/states or major accessibility defects; P2 = minor defects with a workaround; P3 = improvement proposals. These priorities are separate from requirement implementation priorities. Do not pass with unresolved P0/P1 findings.

## Human decision request and decision record

```yaml
decision_id: OPEN-00
task_id: AC-000
question: "One point requiring a decision"
source_of_required_decision: "Concrete basis, such as conflicting requests or unsettled contract terms"
affected_ids: []
facts: []
prepared_artifacts: []
options: []
recommendation: ""
blocked_work: []
work_that_can_continue: []
requested_owner: product-owner
answer: null
decided_by: null
decided_at: null
decision_status: open
```

## Specification baseline, change impact, and gate records

```yaml
gate_id: G1
task_id: AC-000
gate_result: pending
spec_baseline_id: null
revision_or_diff_hash: null
evidence_paths: []
affected_requirement_ids: []
invalidated_task_ids: []
invalidated_acceptance_ids: []
supersedes_evidence: []
```

spec_baseline_id is the hash calculated from spec_files (a fixed manifest of path/sha256 pairs sorted by path). The same document_version does not prove that specifications are identical. Hash the actual files, including uncommitted changes, and save the manifest as an artifact. If the original instruction record is missing, record that fact in assumptions. Do not invent hashes for nonexistent records.

When specifications change, orchestration traces backward from changed FR/DD/AT/DEC entries through the traceability matrix, operation catalog, and task dependencies to identify affected tasks, tests, and gates. Shared schema, state transition, or permission changes affect every task that references them. Preserve past execution results, but mark evidence and affected gates stale. Return completed tasks to ready (or blocked if dependencies are unresolved), and reset tests for the current baseline to not_run. Unrelated tasks do not need to be rerun.

Example: Even with no implementation diff, changing FR-A09 release conditions requires reassessing G1/G3/G4 for AT-A09, S03, and related billing/restriction tasks. Keep earlier results under their old spec_baseline_id. New tests record the new expected results and baseline even at the same implementation revision. When resuming work, recheck owned_files, current diff, and spec_baseline_id. Record any change of owner in the handoff.

## Completion report

Describe the scope and reasons for changes, implemented screens and operations, checks run and not run, which parts are demo-only/unimplemented/actually connected, assumptions and open issues, next required decisions, and the planned replacement points for future API connections. For document-only tasks, do not claim a successful build or a completed demo.
