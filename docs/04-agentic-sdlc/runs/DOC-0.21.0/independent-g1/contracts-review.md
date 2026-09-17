# DOC-0.21.0 独立G1契約再レビュー

- レビュー担当: contracts_review（仕様修正に参加していない別エージェント）
- 対象: 親commit `717c545` の未コミット作業ツリー、DOC-0.21.0
- spec_baseline_id: `02a1e39793df35f760afe9188e86141c8790cd7e92366efbddb7f62aff62e908`
- 判定: **PASS（下記の担当範囲）**。新規指摘0件。
- 仕様ファイルは変更していない。本ファイルのみレビュー証跡として追加。

## 独立確認

manifestのspec_filesを指定のJSON正規化でSHA-256再計算し、上記baselineと一致した。全対象ファイルのバイト列からSHA-256を再計算し、不一致0件を確認した。

最終再確認ではround-1のmanifestと現manifestを比較し、変更対象がacceptance-review-021.csvだけであることを確認した。保存済みacceptance-review-021-before.csvとのdiffは、AT-G121-002の「故障フィルターで表示」→「故障アイコン・ラベルで表示」の1箇所だけだった。修正文言はIR10のfault分類表示と一致し、UI21-001を解消する。型・時間評価・fixture・通知生成規則に変更はなく、以下の契約判定を最終baselineでも維持する。

修正対象G120-001〜005と変更の影響を、IR95・IR103〜106、正規DTO、D02/D07/D08/D12/D15、SR21/SR25/SR28、IR10/IR21/IR45/IR54/IR71/IR83/IR91/IR98、A12本文、fixture、acceptance-review-020/021と照合した。validate_documents.pyの追加検証箇所も読んだ。修正担当による静的検査成功の報告は、ここでの独立実行結果としては数えていない。

| 指摘 | 判定 | 根拠 |
|---|---|---|
| G120-001 | 解消 | IR103とAT-A12-NのflowはPolicy保存後に現在時刻のFactを投入し、通常1秒tickを59回と1回進める。elapsed=0/59では対象Policy由来の副作用0、60で初めて成立する。observedAtは01:00Zで固定、01:01ZでもTTL120秒以内。simulator=falseなので自動測定で上書きされない。接続状態はIR45により時間経過だけではofflineにならない。seedに競合Policy/Automationはなく、対象Unitに進行中Commandもない。 |
| G120-002 | 解消 | IR95の同名規則にalertの例外が入り、IR104でsensor/tamper/reconciliation_required→fault、maintenance→cleaning_due、quality→qualityを明示。sourceAlertId保持と正規NotificationTypeが整合する。 |
| G120-003 | 解消 | IR104が必須severityを全templateに定義。Policy由来は設定severityを保持し、業務通知既定値で上書きしない。restrictionの段階別値、preview、非同期device_operation.failedのactor除外も具体化した。 |
| G120-004 | 解消 | IR105、ChangeEntityType、Subscribe、IR71の対応表にallergen_observationがそろう。追加保存と同一遷移のイベント、scope、再送、最新観測選択、C07/A12の再取得を規定。 |
| G120-005 | 解消 | IR106とIR91で欠落scopeVersionAtCreationを全patch適用後の宛先Membershipから補完。明示0を保持し、不正値/不存在宛先を拒否。生成後のMembership更新では過去snapshotを書き換えない。 |

## 時間評価と同tickの重点確認

IR103のfinalEvaluationはoccurredAtだけを01:01Zとし、保持Factと同じobservedAt/value/quality、phase=conditionを送る。D02の「異なるfactsまたは追加phaseはCONFLICT」に該当せず、IR54の内部評価済み同tick結果を参照する。ここで再評価してbusy/cooldownへ変える要求ではない。SR25の換気非対応と通知成立の独立性も維持され、対象Policyのcreated通知は両Unit、Commandは対応Unitだけとなる。

IR104のseverityは業務デモの設計補完（DEC-61）であり企業承認とはされていない。IR103の時間境界具体化もDEC-60の提案として記録され、本番判断を追加していない。

## 限界

これは文書とfixture契約の独立レビューであり、アプリ実装・ブラウザ・Repository実行試験は未実行。全既存要件を今回全面再レビューしたという判定ではない。静的検査・TypeScript・変異検査の実行証跡および他担当範囲を統合した最終G1判定は親レビューで行う。企業検収、本番Security/IoT/APIの承認を意味しない。
