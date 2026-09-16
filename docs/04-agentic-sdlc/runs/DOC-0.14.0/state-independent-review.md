# 状態遷移・認可の独立補助レビュー

- 対象: DOC-0.14.0 / frontend-demo-1A
- レビュー対象baseline: `f8e6cdd8387f1e623f69616518f41108e55ae9cd5f7ec4b223be5776238f4096`
- レビュー日: 2026-09-16
- レビュー担当: 独立補助AI `/root/state_review`
- 方法: 仕様をread-onlyで照合。既存の自己合格記録を判定根拠に使用しない。
- 判定: 下記担当範囲の新規指摘 **0件**。未解決指摘 **0件**。
- アプリ動作検証: **not_run**。文書上の実装可能性・整合性の判定であり、実装、実機、外部API、決済、本番適合の合格ではない。

## 確認範囲

`docs/02-design/` の `deterministic-contracts.md`、`strict-review-contracts.md`、`review-resolution-contracts.md`、`service-contracts.ts`、`implementation-contracts.md`、`operation-catalog.csv`、`write-version-catalog.csv` を中心に、関連するadmin/client/technicianの要件・詳細設計と、`acceptance-fixes.csv`、`acceptance-strict-review.csv`、`acceptance-rereview.csv`、`acceptance-resolution.csv`、`acceptance-convergence.csv` の該当条件を照合した。

中心は時限制御、Device binding、Restriction、支払確定と制限評価の同期、および設備稼働の集計分類である。案件投影・住所公開・報告要約・成果物全体のG1判定は主レビューの担当とし、本補助レビューの指摘0件を全仕様への単独合格判定に流用しない。ユーザー承認済みの1Aデモ範囲、住所は設置物件から取得する方針、期限後報告は有無・受理状態だけという方針を前提とした。

## 具体シナリオと判定根拠

| 確認シナリオ | 文書上の期待結果と根拠 | 判定 |
| --- | --- | --- |
| Command応答が期限直前、期限同値、期限後に到着 | D04は `receivedAt < expiresAt` のみ成功とし、同値は失効優先、遅延応答でexpiredをacknowledgedへ戻さない。AT-FIX-028が境界を指定する。 | 整合 |
| 診断run中に画面離脱、役割切替、担当失効、通信断が発生 | implementation-contractsの診断run契約はRepository時計で継続し、元Membershipを終了時に再認可。終了阻害はend_blocked/end_failedで記録し、応答なしに停止済みとしない。D05の設備排他と診断run用Command予約枠も整合する。 | 整合 |
| FW/checkの60秒期限後に成功が到着 | D05は境界同値を失敗優先とし、遅い成功でfirmwareVersionを更新しない。再試行は新operationIdと新キー。 | 整合 |
| Deviceを別Unitへ付替え、旧bindingイベントが後着 | D05/SR24は旧bindingを終了して保持し、新sensorId、観測unknown、lastSeenAt=nullへ更新。旧bindingイベントは履歴のみで現状態・Alert・通知を変更しない。旧bindingの復旧はCONFLICT。IR21は保持factsの破棄を指定する。 | 整合 |
| Device付替え後に現在顧客が旧顧客の履歴を参照 | SR24は現在Device読取権と発生時Unit/customerへの現在読取権の積集合を要求し、認可後に件数・ページングを算定する。IR02は既存Unitの顧客変更を禁止する。 | 整合 |
| 制限適用が未配送、送信済み不明、確認済みの各状態で全額入金 | D03は未配送証跡によるnot_required、送信済み不明のreconcile、適用済みのremoveを区別。SR05は論理制限と観測値を分離し、全設備の証跡が揃うまで集約releasedにしない。 | 整合 |
| 終端Restrictionの旧IDを、新しいsequenceで再観測。後継制限も存在 | SR26は終端を戻さず回復caseで制御をblock。旧IDだけをremoveし、後継の論理policyは維持。複数caseは全解決までblockを維持し、AT-REREV-005/AT-CYCLE-007で確認条件が定義される。 | 整合 |
| 制限中または未解決回復caseがある契約の編集とscheduleが競合 | SR19/SR26は同一遷移の再検証を要求し、active制限・回復case中の編集を拒否。先行編集が成功した場合は旧expectedContractVersionのscheduleを拒否する。 | 整合 |
| override専用HQが解除を開始し、結果不明から回復 | IR03は限定投影によるlist/get/overrideと解除意思後のreconcile/retry releaseを定義。正規型・操作認可・版カタログが対応し、請求詳細を取得せず実行経路が成立する。 | 整合 |
| 同じInvoiceに別キーで決済を同時開始、手動入金も競合 | IR06はinitiated/processingを最大1件に限定。Invoice版を照合し、手動入金も非終端試行があれば拒否。D04とwrite-version-catalogは初回Invoice版と後続Payment版を区別する。 | 整合 |
| 複数原因請求のうち1件だけ支払、続いて全件支払 | implementation-contractsの手動入金・複数請求契約はPayment/Invoice/監査/通知/制限評価を同一遷移にまとめる。1件だけでは解除せず、全件paidでscheduledはcancelled、requested/appliedはrelease_requested。processingをpaid扱いしない。 | 整合 |
| 古い観測、欠測、通信不明で稼働KPIと一覧を取得 | SR27はeffectivePowerStateを共通分類として用い、freshな電源観測と最新measured/validのpowerを要求する。SR06で同じ分類のfilterへ遷移する。通信unknownは稼働unknownと分ける。 | 整合 |
| 古い閲覧世代の非同期応答、権限失効後のページ取得 | D07/SR14/IR17/IR24はgeneration/viewEpoch/scopeと現在権限の再検証、旧callback破棄、公開範囲縮小時のsnapshot無効化を定義する。業務の受理済み処理と閲覧の中止を分離する。 | 整合 |

## 引継ぎと再レビュー条件

上記は固定baselineに対する初回の補助レビュー結果である。rootから共有されたIRV-001〜003（集計受入条件、住所文言、引渡し旧リンク）の修正はこの記録では検証済みとして扱わない。新baselineへの合格や全体G1は、主レビューが修正差分を確認して判断する。時限制御、binding、制限、決済、認可の契約または対応型・操作・版条件に変更が入った場合は本担当範囲を再レビューする。
