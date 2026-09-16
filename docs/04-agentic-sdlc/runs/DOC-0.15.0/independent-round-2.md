# DOC-0.15.0 独立再レビュー Round 2

Reviewer: `/root/independent_review`。仕様修正者: `/root`。レビュー担当は仕様本文を編集していない。

対象: 1AフロントエンドモックのPrepare・要件・詳細設計・UIUXと付属契約。baseline: `f11600bc8735d2a10797b418bb13da76601b8021184a7d4cef6c2a70d355dd5f`。

## 1. Executive Review Summary

**G1: passed。未解決指摘0件。** 前回の6件をすべて解消確認し、修正に伴う公開投影、再送、版管理、一覧・集計、入力禁止、UI可否、受入条件の影響を再照合した。追加指摘0件。1Aの実装Agentへ引き渡してよい。

manifest全53ファイルの実SHA-256とmanifestを独立照合し、不一致0件。canonicalizationによるbaseline再計算も一致。静的・型検証の合格を意味レビューの代用にせず、下記の反例を修正契約で解消できることを確認した。アプリ未実装・動作試験not_runは維持する。

| Issue ID | 再確認した具体的条件 | 解消根拠 | 判定 |
|---|---|---|---|
| IRV-001 | offered/accepted/submitted各1件をstatus=offeredで絞る | AT-P01-Nを無条件1/1/1→絞込後1/0/0へ分離。IR26とAT-G1-001一致 | resolved |
| IRV-002 | 未受諾Offerに住所を表示し入場案内を表示しない | DD-A02の住所・入場案内説明を分離、IR25/DEC-16と一致。AT-G1-002 | resolved |
| IRV-003 | 索引・SDLCの現行入力をたどる | 両方DOC-0.15.0、53ファイルhash一致。AT-G1-003 | resolved |
| IRV-004 | 受理→同キー再送→費用/メモ変更→アクセス終了 | IR29/正規MaintenanceJob.completedAt、作成null・受理now一度・後続不変・履歴凍結。D16の計画生成もnull。AT-G1-004 | resolved |
| IRV-005 | resolved critical/open warning、後からacknowledged critical、最後に全resolved | IR30でwarning→critical→normal、同Unit/可視/未解消集合、Job.alertIdsと分離。offer/historyのnullは維持。AT-G1-005 | resolved |
| IRV-006 | T1原作者→T2が本文/測定/写真だけを変更→T2の別Membership/HQ review | IR31の版寄与者継承と実変更者追加、accept/returnともFORBIDDEN。T3は通常条件下で可。主体別reviewAvailability、P05/A06 disabledも一致。AT-G1-006 | resolved |

## 2. BLOCKER Issues

なし。

## 3. CRITICAL Issues

未解決なし。IRV-006解消。内部寄与者集合をUI入力・外部向けDTOへ追加せず、公開するのは現在主体の可否/理由だけ。権限切替後のread/write再送で可否を再投影し、内部履歴は不変。集合欠落時に許可へfallbackせずUNAVAILABLE。

## 4. MAJOR Issues

未解決なし。IRV-001/004/005解消。完了前にアクセスが終了したhistoryはcompletedAt=nullを保持し、後日の完了・受理を反映しない。重大度のnormalは「未解消Alertなし」であり、通信/測定の正常保証と混同しない。

## 5. MINOR Issues

未解決なし。IRV-002/003解消。

## 6. Open Questions

1Aの実装を阻害する未決質問なし。DEC-12〜16の承認範囲を維持し、新規の商用承認を作っていない。

## 7. Cross-document Inconsistencies

今回の6件に関する要件・DD・IR・正規型・UI・追跡・追加受入条件の矛盾は解消。既存優先規則はIR→SR→D/旧DDの同論点に適用する。過去runの判定は当時baselineに限定され、本版へ流用しない。

## 8. Missing Requirements

現行1A範囲で未解決の必須機能欠落なし。IR18の長時間連続運用・メモリ容量保証は既存deferred候補であり、100設備/1000サンプルのデモ完了条件へ混入させない。

## 9. Edge Cases Not Defined

現行1A範囲で未解決の実装阻害なし。Round 1のD/SR/IR照合に加え、今回の変更について次を再確認した。

- 住所null、ページsnapshot中の住所変更、期限後の住所非保持、最新可視報告版の有無・受理状態の固定。
- 最新版が未受理、期限後の受理、完了日時null/期間境界、再送による完了時刻・寄与者重複の禁止。
- 可視/非可視・解消済みAlert、0件、集計filterとsnapshotの一致、offer/historyの非公開重大度。
- 元作者と共同編集者、写真のみ/測定のみ編集、割当・閲覧・無変更save、別Membership、HQ escalation、寄与者欠落、UI確認中の権限/版変更。
- 能力変更による停止理由保持と明示再有効化、機種新規の理由省略と更新時必須。

通信・timeout・冪等性・失効・古いcallback・IoTの状態についてはD01/D04/D05/D07/SR14/IR17/IR24を適用する。制限制御・binding・非同期・決済のRound 1補助独立レビューは新規指摘0件で、今回それらの契約を変更していない。HTTP自体の本番接続仕様は対象外。

## 10. Traceability Matrix

[独立再レビューの全64要件](independent-round-2-traceability.csv): **64件OK**。Prepare→FR→DD→UIUX→Repository→Error Handling→ATの文書対応と、今回の意味上の不整合の解消を確認した判定。アプリ試験合格を意味しない。AT-G1-001〜006のexecution_statusはnot_run。

## 11. Undefined Decisions

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| OPEN-03〜06/11 | 本番API/機器/認証/外部連携/終了責任 | Prepare/D11 | 1B着手の条件。1A G1対象外 | Backend / IoT / Security / Business |
| REV-018 | 長時間容量保証 | IR18 | デモ規模を超える拡張 | Product Owner / Frontend |
| deploy-final-review | デプロイ前の最終確認 | DEC-12 | AIの文書G1と公開承認を分離 | 人・外部レビュアー |

## 12. Implementation Readiness

判定はすべて1Aモックの文書準備に限定する。

| 観点 | 判定 | 根拠 |
|---|---|---|
| Requirements completeness | READY | 64要件の対応と既存責務を確認、欠落指摘解消 |
| Cross-document consistency | READY | 6指摘解消、型/本文/カタログ/AT/現行入力一致 |
| UI/UX completeness | READY | 状態・確認・フォーム・表示品質・可否の取得経路あり |
| Frontend architecture | READY | Repository/Query/RHF/URL/世代の責任分離 |
| API contract readiness | READY | 1A Repositoryの入力・結果・認可・版・エラーが定義済み。本番HTTPはNOT READY |
| Error handling | READY | D01/D04/D10と失効・再送・部分失敗契約 |
| Authentication / Authorization | READY | モックscope・期限・公開投影・共同編集版自己承認拒否 |
| IoT state handling | READY | 模擬要求/応答/期限/制限回復/旧binding分離 |
| Testability | READY | 既存ATと追加6件が具体的入力・期待結果へ対応 |
| Agentic SDLC handoff readiness | READY | 独立再レビュー、現行baseline固定、未実行工程の分離 |

## 13. Required Actions Before Implementation

仕様修正の残作業なし。この独立判定を根拠にオーケストレーション担当がG1記録を更新できる。実装は本baselineを固定して開始し、G2以降でアプリ型検証・lint・build・単体/Component/E2E/a11yを実施する。実装開始時のライブラリ互換性/lockfile・実測ブラウザ版固定は既存計画どおり。実装・本番接続・デプロイの完了や承認は本レポートから推定しない。

検証証跡: `static-check.json`（64要件・136操作・47画面・TypeScript strict、errors=[]）、`validator-negative-checks.json`（27/27検出）、`contract-type-examples-result.json`（正負型例、exit 0）。これらは文書・型検証であり、アプリ試験はnot_run。
