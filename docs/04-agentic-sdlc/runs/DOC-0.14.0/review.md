# DOC-0.14.0 レビュー・修正記録

## 1. Executive Review Summary

1Aモック文書に対する修正と自己再レビュー。6件を修正し、修正箇所と依存する型・画面・条件の再照合で未解決0件。全仕様の独立レビュー完了とは判定しない。G1はpending。ユーザーが承認した住所取得元と期限後の報告表示をDEC-16へ記録した。実装は未着手、アプリ試験はnot_run。

baseline: `f8e6cdd8387f1e623f69616518f41108e55ae9cd5f7ec4b223be5776238f4096`

Round 1: 下記6件を検出して修正。Round 2: 住所禁止の旧記述、報告有無と受理の組合せ、最新可視版、期限後不変、kind別filter、停止理由の保持/解除、型とカタログの一致を再確認。追加指摘なし。

検証: 64要件・136操作・47画面の静的検証、TypeScript strict合格。不整合を注入する検証器テスト20/20合格。これはアプリの動作試験ではない。

## 2. BLOCKER Issues

今回のレビュー範囲で検出なし。

## 3. CRITICAL Issues

- Issue ID: REV-002
- Severity: CRITICAL
- Document: 02-design/review-resolution-contracts.md
- Location: IR23 / JobHistorySnapshot
- Problem: redactedReportSummaryが自由文で伏せ字規則がない
- Why it matters: 期限後の公開範囲を一意に実装できない
- Example Failure: 報告本文中の住所や電話番号が残る
- Required Fix: 承認された報告有無・受理状態のみにする
- Suggested Revision: IR25: ReportHistorySummaryの3状態、最新可視版を終了時に凍結
- 状態: 修正済み・自己再確認済み

## 4. MAJOR Issues

- Issue ID: REV-001
- Severity: MAJOR
- Document: 02-design/service-contracts.ts
- Location: JobOfferSummary / FR-P01
- Problem: regionLabelの入力元が存在しない
- Why it matters: 設置場所との不一致と二重管理を招く
- Example Failure: HQが案件ごとに地域を推測入力する
- Required Fix: 設置物件を唯一の取得元にする
- Suggested Revision: IR25: siteAddress=Property.address、未登録null
- 状態: 修正済み・自己再確認済み

- Issue ID: REV-003
- Severity: MAJOR
- Document: 02-design/query-catalog.csv
- Location: summaries.get / DD-P01・T01
- Problem: 案件画面の状態・重大度条件を集計APIが受け取れない
- Why it matters: 一覧とKPIの同条件要件が満たせない
- Example Failure: warning一覧にcriticalの件数も表示する
- Required Fix: 案件集計のfilterと投影規則を一致させる
- Suggested Revision: IR26: partner/technicianに案件条件を追加、customerは拒否
- 状態: 修正済み・自己再確認済み

- Issue ID: REV-004
- Severity: MAJOR
- Document: 02-design/service-contracts.ts
- Location: RuleBase / AT-A04-B
- Problem: 能力変更時に停止理由を表示する要件に出力項目がない
- Why it matters: 顧客画面は監査権限なしで理由を取得できない
- Example Failure: enabled=falseだけを表示し停止理由が消える
- Required Fix: Repository生成の停止理由と消去条件を定義する
- Suggested Revision: IR27: disabledReason、明示再有効化時の検証と解除
- 状態: 修正済み・自己再確認済み

- Issue ID: REV-005
- Severity: MAJOR
- Document: 02-design/service-contracts.ts
- Location: capabilities.save / DD-A04
- Problem: DDは理由が更新時のみ必須、型は新規にも必須
- Why it matters: 新規入力の実装が分岐する
- Example Failure: 新規登録で未定義のダミー理由を送る
- Required Fix: 新規任意・更新必須を型と検証に反映する
- Suggested Revision: IR28: changeReason optional、既存IDでは非空必須
- 状態: 修正済み・自己再確認済み

## 5. MINOR Issues

- Issue ID: REV-006
- Severity: MINOR
- Document: 02-design/common.md
- Location: §4
- Problem: 操作件数が135だがカタログは136
- Why it matters: 引継ぎ時に欠落と誤認する
- Example Failure: 136件を実装して余剰操作と判断される
- Required Fix: 実際のカタログ件数に一致させる
- Suggested Revision: 136へ訂正、検証器で件数照合
- 状態: 修正済み・自己再確認済み

## 6. Open Questions

今回の住所・報告表示の業務判断は解決済み。別AIによる独立レビューの委任可否は未回答のため未実施。自己再レビューを独立合格へ置換しない。

## 7. Cross-document Inconsistencies

REV-003〜006の集計条件・DTO・必須性・件数を修正。受諾前住所を禁止した旧BR-P01/AT-P01-B/DTO説明もユーザー判断に合わせて修正。

## 8. Missing Requirements

新規機能は追加しない。以前からの長時間メモリ容量候補REV-018はIR18どおりdeferred。100設備/1000サンプルの既存デモ範囲を変更しない。

## 9. Edge Cases Not Defined

今回の修正範囲では未定義なし。住所null・住所変更・別社Offer・失効・報告なし・旧版受理と新版未受理・期限後受理・言語変更・filter不正・再有効化・理由欠落を追加受入計画へ記載。既存の通信切断、timeout、権限失効、二重操作、並行更新はD01/D04/IR17/IR24を継続。HTTP 400/401/403/404/409/429/500は本番契約対象であり、1AはDomainErrorを模擬する。アプリ実行結果を示すものではない。

## 10. Traceability Matrix

[64要件の追跡表](traceability-matrix.csv)。Status=OKは文書間の参照・受入条件の存在を確認した構造上の判定。全64件の独立した意味レビューやアプリ試験の合格を意味しない。修正5項目の振る舞いは[追加受入計画](../../acceptance-loop.csv)、件数訂正は静的検証で扱う。

## 11. Undefined Decisions

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| G1 | 別担当による独立レビュー | agents/review.md、DEC-12 | 自己修正の独立承認は禁止 | 独立Review Agent |
| OPEN-03〜06/11 | 本番接続・機器・認証・外部取引・終了責任 | Prepare、D11 | 1A対象外。本番契約はNOT DEFINED | Backend / IoT / Security / Business |
| REV-018 | 継続運用の容量上限 | IR18 | 今回のデモ規模を超える候補 | Product Owner / Frontend |

## 12. Implementation Readiness

| 観点 | 判定 | 根拠 |
|---|---|---|
| Requirements completeness | CONDITIONALLY READY | 修正済み、全体の独立G1待ち |
| Cross-document consistency | CONDITIONALLY READY | 静的・変更箇所照合合格、独立G1待ち |
| UI/UX completeness | CONDITIONALLY READY | 型付き表示契約を反映、独立G1待ち |
| Frontend architecture | CONDITIONALLY READY | 既存設計維持、独立G1待ち |
| API contract readiness | CONDITIONALLY READY | 1A Repositoryの型検証合格。本番HTTPはNOT READY |
| Error handling | CONDITIONALLY READY | 既存DomainErrorと追加境界を追跡、独立G1待ち |
| Authentication / Authorization | CONDITIONALLY READY | モック認可。公開住所の判断を反映、独立G1待ち |
| IoT state handling | CONDITIONALLY READY | 模擬状態のみ、独立G1待ち |
| Testability | CONDITIONALLY READY | 受入計画と静的チェックあり、独立G1待ち |
| Agentic SDLC handoff readiness | NOT READY | 独立レビュー未実施、G1未合格 |

## 13. Required Actions Before Implementation

同一baselineを別Review Agentへ渡し、残る指摘があれば修正→再レビューを継続する。G1を全てOKとする作業はまだ完了していない。実装・受入試験・本番接続・デプロイの承認はこの文書レビューから推定しない。
