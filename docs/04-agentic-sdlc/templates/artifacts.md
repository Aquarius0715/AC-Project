# Agentic SDLC 成果物テンプレート

以下はコピーして使うためのテンプレートである。空欄になっている承認結果や実行結果を、事実に基づかずに埋めて完成させたことにしない。状態を表す値は、対象ごとに決まった以下のschema(型の定義)を使う。

## 状態schema

| 対象 | フィールドと許容される値 | 完了・状態遷移のルール |
|---|---|---|
| タスク | task_status: planned(計画中)/ready(着手可能)/in_progress(作業中)/in_review(レビュー中)/done(完了)/blocked(保留) | plannedからreadyに進むのは、依存関係・受入subcase・仕様baselineを確認した後。doneにするには、必要なゲートがpassed(合格)になっていて、証跡があること |
| 試験 | test_result: not_run(未実行)/passed(合格)/failed(不合格)/blocked(保留) | 実行していなければnot_run。期待値が不明、または環境が足りない場合はblocked。実際の結果とすべてのsubcaseから判定する |
| ゲート | gate_result: pending(判定待ち)/passed(合格)/failed(不合格)/blocked(保留)/stale(古くなった) | 未判定はpending。関連する仕様が変わった場合、過去にpassedだったものもstaleとして扱い、再判定の対象にする |
| 証跡の有効性 | evidence_status: current(現行)/stale(古くなった) | 過去に行った試験の実際の結果は書き換えず、それが今の仕様にも当てはまるかどうかを別に管理する |
| 指摘 | finding_status: open(未対応)/in_progress(対応中)/resolved(解決済み) | 修正されたことと、再検証されたことを確認してからresolvedにする |
| 決定 | decision_status: open(未対応)/proposed(提案中)/accepted(承認済み)/rejected(却下)/superseded(別の決定に置き換え) | acceptedにするには、実際にdecided_by(決定した人)/at(日時)/answer(回答)が必要。提案の実装を許可することと、業務としての承認を混同しない |

文書のfront matter(先頭のメタデータ)にあるdraft/proposedなどは、文書自体の状態を表すものであり、この実行用のschemaとは別のものである。異なる種類の状態を、1つのstatus列にまとめて集計しない。

## タスク入力(オーケストレーションエージェント→担当エージェント)

```yaml
task_id: AC-000
document_version: 0.10.0
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

## 引き継ぎ(すべてのエージェント共通)

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

指摘の優先度: P0=権限を超えた操作・実際の処理の誤発火・重大なデータ破壊、P1=必須の操作の流れや状態に関わる欠陥・重要なアクセシビリティの欠陥、P2=代わりの方法がある軽い不具合、P3=改善の提案。この優先度は、要件そのものの実装優先度とは別のものである。P0/P1が未解決のままでは合格にしない。

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

## 仕様baseline・変更の影響とゲート記録

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

spec_baseline_idとは、spec_files(pathとsha256をpathの順番で並べて固定したmanifest、つまりファイル一覧)から計算したハッシュ値である。document_version(文書のバージョン番号)が同じというだけで、同じ仕様だと判断してはいけない。まだコミットしていない差分を含めて実際のファイルをハッシュ化し、そのmanifest自体も成果物として保存する。指示の元になった記録が収録されていない場合は、その事実をassumptions(仮定)として残す。存在しない元記録について、架空のハッシュを作らない。

仕様を変更したときは、オーケストレーションエージェントが、変更されたFR/DD/AT/DECから追跡表・操作カタログ・タスクの依存関係を逆にたどり、影響を受けるタスク・試験・ゲートを洗い出す。共通のスキーマ・状態遷移・権限に関する変更は、それを参照しているすべてのタスクが対象になる。過去の実行結果はそのまま保持したうえで、証跡をstale(古い)、影響するゲートをstale、完了していたタスクをready(依存関係が未解決ならblocked)に戻し、現在のbaselineに対応する試験をnot_run(未実行)に戻す。無関係なタスクまで再実行する必要はない。

例: 実装の差分がなくても、FR-A09の解除条件を変更した場合は、AT-A09とS03、および関連する請求・制限のタスクについて、G1/G3/G4を再判定する。以前の結果は古いspec_baseline_idのまま残しておき、同じ実装revision(版)であっても、新しい試験では新しい期待値とbaselineを記録する。作業を再開するときは、owned_files(担当ファイル)・現在の差分・spec_baseline_idを再確認し、途中で担当者が変わった場合はその旨を引き継ぎに残す。

## 完了報告

対象の内容と変更した理由、実装した画面や操作、実施した検証と実施していない検証、デモ用・未実装・実際に接続済みのものの区別、仮定や未決事項、次に必要な意思決定、将来API接続をする際に差し替える予定の箇所を記載する。文書だけを作るタスクの場合、ビルドが成功したことやデモが完成したことを主張しない。
