# 0.12.0 再レビュー・修正結果

再レビューで見つけた8件を修正した。修正後の波及確認と最終検証を含む3回の確認を行い、今回確認した変更範囲に未解決の不整合は残っていない。

対象baseline: `254bd3088246362701d9816ab7cf4334a1d299e87cb93a347297363824bc36f0`。修正担当自身によるレビューであり、別担当による承認とは区別する。

| 指摘 | 再現条件と修正後の期待値 | 契約 |
|---|---|---|
| CV-001 | 一つの制限に2設備、一方のみ閲覧可能 → get/list/通知/同キー結果を読む → 個別NOT_FOUND・一覧件数に含めず、全対象閲覧宛先だけに予告 | [修正仕様](../../../02-design/review-resolution-contracts.md#ir19-複数設備制限の公開範囲--cv-001) |
| CV-002 | 受諾後開始前またはOffer期限後 → 同キー再送/getResult、別キーで再決定 → 元Membershipの受領記録のみ返す。新意思はCONFLICT | [修正仕様](../../../02-design/review-resolution-contracts.md#ir01-受諾と辞退の最小応答) |
| CV-003 | override専用の一覧 → createdAt/updatedAtでsortしページング → 限定DTOに対応値があり同順序・id tie-break | [修正仕様](../../../02-design/review-resolution-contracts.md#ir03-強制解除専用権限) |
| CV-004 | paid/processing/期限内または非HQ → reminder preview/recipients/remindを呼ぶ → 状態CONFLICTまたはFORBIDDEN、通知/版変更0 | [修正仕様](../../../02-design/review-resolution-contracts.md#ir20-督促previewの事前条件--cv-004) |
| CV-005 | 分始点の1kWと30秒後の9kW → 1分積算、同sequence候補を追加 → 始点値のみ1/60kWh、id順で決定し非始点で置換しない | [修正仕様](../../../02-design/review-resolution-contracts.md#ir08-電力量の由来) |
| CV-006 | raw未知単位と点検不一致単位 → demo.triggerとjobs.saveDraftで保存 → rawはsuspect、点検は全体VALIDATION、存在しない操作名なし | [修正仕様](../../../02-design/review-resolution-contracts.md#ir22-生測定と点検の保存経路--cv-006) |
| CV-007 | 保持factsと現binding Sensor → simulate→fire、bind、重複facts、再送 → simulate無副作用、binding切替破棄、重複拒否、再送は再評価なし | [修正仕様](../../../02-design/review-resolution-contracts.md#ir21-fact評価の時点とセンサー--cv-007) |
| CV-008 | RawMeasurementの入力時刻/eventID → 正常時刻、未来、外側eventID不一致、別scope sensor → 正常は入力時刻保持、未来suspect、不一致VALIDATION、scope外NOT_FOUND | [修正仕様](../../../02-design/review-resolution-contracts.md#ir12-生測定と正規測定) |

修正サイクル:

1. 0.11.0の権限・型・入力・状態遷移を照合し8件を修正。
2. 呼出元へ戻り、受諾再送の操作カタログ、督促の画面説明、予告確認の保存前/保存後を同期。
3. 追跡表・型・カタログの最終照合、および検証器に不整合を注入する12ケースを確認。

検証結果:

- [静的検証とTypeScript](static-check.json): 64要件・136操作・47画面・57 Component、エラー0。
- [検証器の異常検出](validator-negative-checks.json): 12/12ケースで意図した不整合を検出。アプリの振る舞いの試験ではない。
- [追加受入計画](../../acceptance-convergence.csv): 8件を追跡表へ接続、実行状態はnot_run。
- 一次資料4ファイルは0.11.0のhashと一致。旧版の記録は保持。

再実行:

```sh
python3 docs/tools/validate_documents.py
python3 docs/tools/validate_documents.py --tsc /private/tmp/ac-typescript-check/package/lib/tsc.js
python3 docs/tools/check_review_regressions.py
```

TypeScriptのパスはこの環境の既存コンパイラ。他環境ではインストール済みlib/tsc.jsを指定する。

アプリ実装と動作試験はこの作業の対象外で未実施。REV-018の長時間稼働容量保証は既存デモの範囲外として別管理を継続する。確認した文書不整合の解消は、未知の問題が一切存在しないという保証ではない。詳細は[機械可読の記録](design-review-result.json)。
