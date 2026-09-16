# 1. Executive Review Summary

ユーザー回答DEC-17/18を反映し、今回の7指摘はすべて修正・自己再確認済み。仕様判断待ちは0件。別主体による修正後の独立G1は未実施のため、工程としてはCONDITIONALLY READY。

制限操作の権限はrestriction.manage/overrideの2種類。案件一覧は状態（業務順）・重大度・期限の昇順/降順を選択可能とし、デフォルトは状態の業務順とした。今回の成果物は1A設計文書であり、アプリ未実装・動作試験not_run。

全指摘のDocument/Location/Problem/Why it matters/Example Failure/Required Fix/Suggested Revisionは[回答前の詳細報告](before-decisions/review.md)に保存。回答後は本報告の解消判定を適用する。決定は[DEC-17/18](../../../00-prepare/internal/review-decisions-016.json)、修正仕様は[IR34](../../../02-design/review-resolution-contracts.md#ir34-ユーザー確定の権限と案件ソート)、期待値は[追加受入計画](../../acceptance-review-016.csv)。

# 2. BLOCKER Issues

未解決0件。REV-001は2権限に確定。FR-A10一覧・詳細・IR34・UI・AT-REV16-001を統一し、既存Permission型と操作カタログを維持。manageのみ／overrideのみ／両方／なしの期待結果を定義した。IR03の限定解除追跡も維持する。

# 3. CRITICAL Issues

未解決0件。REV-004の監査検索修正を維持。認可済み集合を検索し、他テナントと不存在の相関IDはいずれも成功空集合。

# 4. MAJOR Issues

未解決0件。

| Issue | 解消根拠 |
|---|---|
| REV-002 | IR32の一覧/集計unitIds整合、空配列、公開投影後の照合 |
| REV-003 | IR33の監査必須Query、候補選択、URL、関連リンクの個別認可 |
| REV-005 | DEC-18/IR34の全10状態順位、ソートUI、既定、昇降順、安定ID、ページング |

業務順: requested → offered → accepted → assigned → in_progress → on_hold → submitted → rework_requested → completed → cancelled。

これは表示順位であり、状態遷移の許可を変更しない。descでも同状態内はID昇順。offer/historyには公開・凍結状態を使う。再確認でQuery key要約にもsort/cursor/limitとIR17の世代を反映した。

# 5. MINOR Issues

未解決0件。REV-006のPage.totalは正確な非負件数、REV-007の工程状態は現行manifestのgateを参照。回答前の報告・manifest・gateはbefore-decisionsへ保存した。

# 6. Open Questions

今回の1A指摘に関する追加質問なし。DEC-17/18はaccepted。ユーザー回答を根拠に確定した。

# 7. Cross-document Inconsistencies

FR-A10→Permission→操作カタログ→UI→ATは2権限で一致。DEC-18→JOB_STATUS_ORDER→Query.default_sort/sort_mapping→画面・Component→ATは業務順を既定にして一致。sortは一覧だけへ渡しKPI件数を変えない。今回の変更で実HTTP/DB/実機機能や新permissionを追加していない。

# 8. Missing Requirements

今回の7指摘について未解決の欠落なし。長時間運用のメモリ上限は既存IR18のMissing Requirement Candidateとしてdeferred。本番API/認証/DB/機器回復はD11の後続成果物でNOT READY。これらを1Aの欠陥件数へ混入させない。

# 9. Edge Cases Not Defined

[回答前の通信・IoT・センサー・セッション等の照合](before-decisions/review.md#9-edge-cases-not-defined)を維持。その中の権限粒度・状態順のEdge Case Undefinedは解消。

追加確認: 全10状態、同値ID、公開offer/history、asc/desc、期限null、複数ページ、sort変更時cursor破棄・filter保持、Back/Forward、不正/空/重複sort、言語切替、遅延した旧条件応答、loading/error/empty、キーボード・モバイル・aria-sort、権限4組合せ。具体的期待値はAT-REV16-001/005。アプリ試験はnot_run。

# 10. Traceability Matrix

[全64要件の判定表](traceability-matrix.csv): 文書対応64件OK。前回CONFLICT 1件・INCOMPLETE 6件はDEC-17/18と追加ATにより解消。OKは文書照合であり、アプリ動作や独立G1の合格ではない。

# 11. Undefined Decisions

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| DEC-17 | 解消: 2権限 | FR-A10/IR34 | ユーザー回答済み | Product Owner / Security |
| DEC-18 | 解消: ソート機能、業務順が既定 | Query/UI/IR34 | ユーザー回答済み | Product Owner / UI/UX |
| OPEN-03〜06/11 | 本番機器/API/認証/外部連携/終了責任 | Prepare/D11 | 本番設計前の必須成果物 | Backend / IoT / Security / Business |
| IR18 | 長時間容量保証 | IR18 | 保証範囲拡張時の候補 | Product Owner / Frontend |

# 12. Implementation Readiness

| 観点 | 判定 | 根拠 |
|---|---|---|
| Requirements completeness | READY | 今回の未決2件解消 |
| Cross-document consistency | READY | 型・Query・UI・AT一致 |
| UI/UX completeness | READY | 選択・既定・URL・状態・操作性を定義 |
| Frontend architecture | READY | Repository/Query/RHF/Navigation/世代とsort keyを分離 |
| API contract readiness | READY | 1Aローカル契約のみ。本番HTTPはNOT READY |
| Error handling | READY | D01/D04と不正sort・拒否条件 |
| Authentication / Authorization | READY | 1Aの2権限。実認証は対象外 |
| IoT state handling | READY | 模擬状態の既存契約を維持 |
| Testability | READY | 権限行列・全状態順位・境界の期待値 |
| Agentic SDLC handoff readiness | CONDITIONALLY READY | 別主体の独立G1未実施 |

# 13. Required Actions Before Implementation

今回の仕様判断・修正の残作業なし。独立レビュアーが回答後manifestを入力にG1を判定する。自己再確認を別主体の合格とは記録しない。

反復: 初回7件→5件修正→ユーザーが残る2件を決定→要件/設計/型/UI/AT更新→再レビューでQuery key要約も統一→静的・型検証と検証器変異検査。証跡はstatic-check.json、validator-negative-checks.json、baseline-integrity-check.json。実装後の動作・性能・a11yとデプロイ前の人・外部レビューは別工程。
