# DOC-0.14.0 独立レビュー Round 1

Reviewer: `/root/independent_review`。修正担当: `/root`。IoT/非同期/制限・決済の補助独立レビュー: `/root/state_review`。
対象: 1Aフロントエンドモックの仕様文書。baseline: `f8e6cdd8387f1e623f69616518f41108e55ae9cd5f7ec4b223be5776238f4096`。実装・アプリ試験・本番接続は評価対象外。

## 1. Executive Review Summary

G1: **changes_requested**。6件（CRITICAL 1 / MAJOR 3 / MINOR 2）。再割当後の自己承認判定、案件重大度・完了日時の生成規則、絞込後の受入期待値に不足がある。今回の住所取得元、期限後の報告3状態、停止理由、機種保存理由の分岐は、下記の関連不整合を除き型・操作・UI契約で整合している。

既存の自己レビュー合格・静的検証合格を独立合格の代用にしていない。Prepare/DEC-12〜16、共通・全役割要件と設計、D/SR/IR、正規型、操作・版・Query・画面・Componentのカタログ、追跡表を照合した。補助担当は制限の部分解除、旧bindingイベント、遅延ack、決済試行排他等を別途照合し、新規指摘0件。

## 2. BLOCKER Issues

検出なし。

## 3. CRITICAL Issues

### IRV-006 — 再割当された報告の編集者を自己承認判定から除外できる

- Document / Location: `02-design/contractor.md` DD-P05 reviewerId、`deterministic-contracts.md` D06、`strict-review-contracts.md` SR07、FR-P05。
- Problem: DD-P05はreport.authorIdとの不一致だけを要求する。一方、再割当は元作者を保存し、新担当が更新した項目だけ作者を更新する。本文・写真等の追加編集者も含めた版単位の自己承認判定がない。
- Why it matters: DEC-12の別人による品質確認を満たせない。
- Example Failure: T1作成→T2へ再割当→T2が編集・提出→T2が別Membershipの品質担当/HQで受理。T2 != 元のreport.authorIdなので通過する。
- Required Fix: 提出版の作成・編集寄与userIdをRepositoryが記録し、自己承認拒否へ使う。元作者・項目作者履歴を維持し、別Membership/HQでも同じ判定にする。
- Suggested Revision: 版の内部寄与者集合を継承し、実内容変更者だけを追加。本文・測定・写真も対象。無編集の閲覧/割当を寄与としない。寄与者の受理・差戻しはFORBIDDEN、第三者だけ許可する受入例を追加する。

## 4. MAJOR Issues

### IRV-001 — 状態絞込後のKPI受入条件がIR26と矛盾

- Document / Location: `01-requirements/contractor.md` AT-P01-N、IR26。
- Problem: status=offered後もactiveCount=1/reviewCount=1を要求するが、IR26は集計にも同じAND条件を適用する。
- Why it matters: 正しい実装が既存必須テストで失敗する。
- Example Failure: offered/accepted/submittedが各1件。offeredだけの一覧に対し、集計は1/0/0と1/1/1の二通りになる。
- Required Fix: 条件変更前後を明記し、絞込後の期待値を修正。
- Suggested Revision: 無条件はoffer/active/review=1/1/1、status=offered後は1/0/0、一覧1件。受諾期間・fixture時刻も明示する。

### IRV-004 — 履歴の完了日時の生成元が未定義

- Document / Location: `02-design/review-resolution-contracts.md` IR23、`service-contracts.ts` MaintenanceJob/JobHistorySnapshot。
- Problem: completedAtを凍結し期間検索へ使うが、元Jobに項目がなく、完了操作から生成する規則もない。
- Why it matters: 履歴の期間検索と表示日時を一意に実装できない。
- Example Failure: 提出日・受理日・費用更新日が違う案件で、実装者ごとに履歴の検索対象日が変わる。
- Required Fix: 完了日時の保存または不変完了イベントからの導出を定義する。
- Suggested Revision: jobs.review accept成功時のRepository nowを保存し、それ以前はnull。後続の費用・メモ更新で変えず、終了時にその値を凍結する。

### IRV-005 — 公開中の案件重大度の計算規則が未定義

- Document / Location: `service-contracts.ts` JobSummary.severity、DD-P01/T01、IR23/26。
- Problem: 必須severityの取得元・複数アラート集約・resolvedの扱い・0件時の値が定義されていない。
- Why it matters: 表示、filter、sort、KPIの期待値を固定できない。
- Example Failure: 同Unitにresolved criticalとopen warningがありJob.alertIdsが空の場合、critical/warning/normalが実装者により分かれる。
- Required Fix: 既存の「設備の緊急度」と整合する認可済み取得集合・集約規則を明示する。offer/historyの未公開nullは維持する。
- Suggested Revision: 対象設備の可視な未解消アラートから決定する等、採用した技術規則を明記し、0件・混在・解消・scope外のテストを追加する。normalを設備の安全保証と呼ばない。

## 5. MINOR Issues

### IRV-002 — 住所の非公開という旧記述

- Document / Location: `02-design/admin.md` DD-A02 property.address/accessInstructions表。
- Problem: 住所と入場案内を合わせて「まだ依頼を受けていない業者には非公開」と記載。
- Why it matters: DEC-16/IR25に反する入力欄説明が残る。IR25の優先により最終判断は解決できるが、局所実装で誤る。
- Example Failure: 住所も空にして受諾前画面へ返す。
- Required Fix / Suggested Revision: 登録住所は自社Offerに投影、入場案内は有効受託期間内のみ、と分離する。

### IRV-003 — SDLC入口が旧baselineを指す

- Document / Location: `04-agentic-sdlc/README.md` 現行入力の案内（rootによる発見を独立確認）。
- Problem: 文書版0.14.0なのに現行入力リンクがDOC-0.13.0/spec-manifest.json。
- Why it matters: 次のAgentが誤った版を固定する。
- Example Failure: 住所・期限後要約修正を含まないbaselineを実装証跡へ記録する。
- Required Fix / Suggested Revision: 現行manifestへのリンクと節名を統一。旧runsは履歴として維持。

## 6. Open Questions

新規の商用判断を要求しない。上記は承認済み1A要件の具体化・整合修正として差し戻す。IR18の継続運用容量候補はdeferredを維持する。

## 7. Cross-document Inconsistencies

IRV-001/002/003/006。優先する修正契約があっても、矛盾した受入期待値をそのまま合格にしない。

## 8. Missing Requirements

新機能の追加要求なし。IRV-004/005/006は既存必須出力・既存自己承認禁止の実装契約不足。

## 9. Edge Cases Not Defined

上記の複数編集者、完了後の別更新、複数/解消済みアラートを要修正。通信timeout/切断、セッション・担当失効、並行版更新、二重送信、空一覧、ページング中失効、欠測/stale、言語切替、reload/戻るの既存契約は確認済み。HTTP 400/401/403/404/409/429/500そのものは1A対象外で、対応するDomainErrorを模擬する。

## 10. Traceability Matrix

64要件の独立判定は [independent-round-1-traceability.csv](independent-round-1-traceability.csv)。既存追跡表の参照情報を引き継ぎ、本レビューの意味判定でStatusを付けた。既存の構造上のOKを独立合格の根拠にはしていない。本レビュー時点の要修正行は下表。記載外の要件は文書レビュー上のOKであり、アプリ試験合格ではない。

| Requirement ID | Requirement | Prepare | Detailed Design | UI/UX | API | Error Handling | Testable | Status |
|---|---|---|---|---|---|---|---|---|
| FR-P01 | 受託集計 | BIZ-04/12 | DD-P01/IR23/26 | P01 | jobs.list/summaries.get | D01/D04 | AT-P01-N矛盾 | CONFLICT |
| FR-P05 | 品質確認 | DEC-12/BIZ-12 | DD-P05/D06/SR07 | P05 | jobs.review | 自己承認再割当不足 | 複数編集者追加必要 | INCOMPLETE |
| FR-P08 | 期限後公開 | DEC-16/BIZ-12 | IR23/25 | P08 | jobs.get/list | D01/IR24 | completedAt未定義 | INCOMPLETE |
| FR-T01 | 担当集計 | BIZ-04/08 | DD-T01/IR26 | T01 | jobs.list/summaries.get | D01/D04 | severity未定義 | INCOMPLETE |
| FR-T09 | 報告版・編集者 | BIZ-12 | D06/SR07/DD-T09 | T04 | jobs.saveDraft/submit | D01 | 自己承認との結合不足 | INCOMPLETE |
| FR-A02 | 住所入力 | DEC-16/BIZ-07 | DD-A02/IR25 | A02 | properties.save | D01 | 旧文言矛盾 | CONFLICT |
| FR-A06 | HQ品質確認 | DEC-12/BIZ-12 | DD-A06 | A06 | jobs.review | 自己承認再割当不足 | 複数編集者追加必要 | INCOMPLETE |
| FR-X04 | 認可 | DEC-12 | D01/D06/SR07 | 共通guard | jobs.review | 自己承認再割当不足 | 追加必要 | INCOMPLETE |

## 11. Undefined Decisions

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| IRV-004/005/006 | 既存出力・認可の技術具体化 | 正規型/D06/IR | 実装者による補完防止 | Frontend設計→独立Review |
| OPEN-03〜06/11 | 本番API/機器/認証/外部連携/終了責任 | Prepare/D11 | 1B着手条件 | Backend / IoT / Security / Business |
| REV-018 | 長時間容量保証 | IR18 | 1A規模外 | Product Owner / Frontend |

## 12. Implementation Readiness

| 観点 | 判定 | 根拠 |
|---|---|---|
| Requirements completeness | CONDITIONALLY READY | 既存出力・自己承認の具体化待ち |
| Cross-document consistency | NOT READY | IRV-001/002/003/006 |
| UI/UX completeness | CONDITIONALLY READY | 重大度・履歴日時の規則待ち |
| Frontend architecture | READY | Repository/Query/RHF/URL/世代の責任分離あり |
| API contract readiness | CONDITIONALLY READY | 1A Repositoryのみ。IRV-004/005/006修正待ち。本番HTTPはNOT READY |
| Error handling | READY | D01/D04/D10/IR17/24の拒否・復旧・描画規則 |
| Authentication / Authorization | NOT READY | IRV-006の自己承認抜け |
| IoT state handling | READY | 補助独立照合で新規指摘なし。模擬状態のみ |
| Testability | CONDITIONALLY READY | IRV-001の矛盾と追加境界例待ち |
| Agentic SDLC handoff readiness | NOT READY | G1 changes_requested、旧baseline案内 |

## 13. Required Actions Before Implementation

6件を修正し、現行manifestと静的・型検証の証跡を更新して同じ独立担当へ再提出する。修正後の再確認前にG1をpassedへ変更しない。アプリ試験はnot_run、実装・本番・デプロイ承認は別工程。
