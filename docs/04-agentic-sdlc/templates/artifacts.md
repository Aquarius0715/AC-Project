# Agentic SDLC 成果物テンプレート

以下はコピー用。空欄の承認や実行結果を補完して作り上げない。状態は以下の対象別schemaを使用する。

## 状態schema

| 対象 | フィールドと許容値 | 完了・遷移規則 |
|---|---|---|
| タスク | task_status: planned/ready/in_progress/in_review/done/blocked | planned→readyは依存・受入subcase・仕様baseline確認後。doneは必要ゲートpassedと証跡が必要 |
| 試験 | test_result: not_run/passed/failed/blocked | 実行なしはnot_run、期待値不明・環境不足はblocked。実結果と全subcaseから判定 |
| ゲート | gate_result: pending/passed/failed/blocked/stale | 未判定pending。関連仕様変更で過去passedはstaleとして再判定対象にする |
| 証跡有効性 | evidence_status: current/stale | 過去試験の実結果を改変せず、現行仕様への適用可否を分離 |
| 指摘 | finding_status: open/in_progress/resolved | 修正と再検証を確認してresolved |
| 決定 | decision_status: open/proposed/accepted/rejected/superseded | acceptedには実在するdecided_by/at/answerが必要。提案の実装許可と業務承認を混同しない |

文書front matterのdraft/proposed等は文書状態であり、この実行schemaの対象外。異なる状態種別を単一status列で集計しない。

## タスク入力（オーケストレーション→担当）

```yaml
task_id: AC-000
document_version: 0.7.0
spec_baseline_id: null
spec_files: [] # pathとsha256。要件・設計・AT・共通契約・出所を含む
acceptance_manifest_hash: null
task_status: planned
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

## テストケース・実行記録

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
finding_status: open
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
decision_status: open
```

## 仕様baseline・変更影響とゲート記録

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

spec_baseline_idはspec_files（path/sha256をpath順で固定したmanifest）のハッシュ。document_versionだけで同一仕様と判断しない。未コミット差分を含む実ファイルをハッシュ化し、manifestそのものも成果物に保存する。指示原記録が未収録ならその事実をassumptionsへ残す。存在しない原記録のハッシュを作らない。

仕様を変更したら、オーケストレーションが変更FR/DD/AT/DECから追跡表・操作カタログ・task dependenciesを逆引きし、影響タスク・試験・ゲートを列挙する。共通schema・状態遷移・権限の変更は全参照タスクが対象。過去の実行結果は保持し、証跡をstale、影響ゲートをstale、完了タスクをready（依存未解決ならblocked）、現行baselineの該当試験をnot_runへ戻す。無関係なタスクまで再実行しない。

例: 実装差分なしでFR-A09の解除条件を変更した場合も、AT-A09とS03、関連する請求・制限タスクのG1/G3/G4を再判定する。旧結果は旧spec_baseline_idに残し、同じ実装revisionの新試験でも新しい期待値とbaselineを記録する。再開時はowned_files、現在差分、spec_baseline_idを再確認し、途中の担当変更を引き継ぎへ残す。

## 完了報告

対象・変更理由、実装した画面/操作、実施した検証と未実施、デモ/未実装/実接続済みの区別、仮定・未決、次の意思決定、将来のAPI接続に備えたadapter差替え箇所を記載する。文書のみのタスクならビルド成功やデモ完成を主張しない。
