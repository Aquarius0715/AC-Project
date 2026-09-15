# Agentic SDLC 成果物テンプレート

以下はコピー用。空欄の承認や実行結果を補完して作り上げない。状態は`planned / in_progress / passed / failed / blocked / not_run`を使い分ける。

## タスク入力（オーケストレーション→担当）

```yaml
task_id: AC-000
document_version: 0.3.0
status: planned
agent_role: implementation
objective: "対象要件に対する具体的な成果"
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

## 引き継ぎ（全エージェント）

```yaml
task_id: AC-000
agent_role: implementation
status: not_run
revision_or_diff_hash: null
requirements_addressed: []
files_changed: []
decisions_added: []
checks:
  - command: null
    environment: null
    result: not_run
    evidence_path: null
unresolved_findings: []
human_decisions_needed: []
next_agent: test
next_action: ""
```

## テストケース・実行記録

```yaml
case_id: AT-C03
requirement_ids: [FR-C03]
design_ids: [DD-C03]
revision_or_diff_hash: null
fixture: seed-A
clock: "2026-09-14T01:00:00Z"
role_and_scope: "client / customer-a"
preconditions: []
steps: []
expected: []
actual: null
result: not_run
command: null
browser_viewport: null
evidence_paths: []
defect_id: null
```

## レビュー指摘

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
status: open
retest_evidence: null
```

指摘優先度: P0=越権・実処理誤発火・重大なデータ破壊、P1=必須フロー/状態・重要a11yの欠陥、P2=代替可能な軽微不具合、P3=改善提案。これは要件の実装優先度とは別。P0/P1未解決で合格にしない。

## 人への判断依頼・決定記録

```yaml
decision_id: OPEN-00
task_id: AC-000
question: "判断が必要な一点"
source_of_required_decision: "依頼の矛盾、未確定の契約条件など具体的な根拠"
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
status: open
```

## 完了報告

対象・変更理由、実装した画面/操作、実施した検証と未実施、デモ/未実装/実接続済みの区別、仮定・未決、次の意思決定、API接続箇所を記載する。文書のみのタスクならビルド成功やデモ完成を主張しない。
