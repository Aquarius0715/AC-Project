# 0.13.0 再レビュー・修正結果

案件の公開範囲に関する5件を修正。1回目で一覧・履歴・集計・ページングの4件、修正後の再レビューで社内技術者の履歴型1件を発見して修正し、最終照合を行った。今回確認した変更と依存経路に未解決の指摘はない。

baseline: `941c5a3152e4fa6057bcb33792a09fdfa64955f14059f951bb8071eb39282da5`

| 指摘 | 確認条件 | 修正後の期待値 |
|---|---|---|
| [PV-001](../../../02-design/review-resolution-contracts.md#ir23-案件一覧の投影と検索--pv-001003) | 未受諾Offerの背後のAlert/Job状態を変更 / status/severity/unitIdで検索・sort | 非公開変更で件数/順序が変わらない |
| [PV-002](../../../02-design/review-resolution-contracts.md#ir23-案件一覧の投影と検索--pv-001003) | 受諾していたOfferのアクセスが終了 / jobs.list/getからhistoryを開く | 型付き凍結履歴だけ、設備IDと編集導線なし |
| [PV-003](../../../02-design/review-resolution-contracts.md#ir23-案件一覧の投影と検索--pv-001003) | 将来開始accepted Offerと期限後history / 一覧とpartner summaryを同条件で取得 | 同じ投影集合、業務稼働件数は現有効summaryのみ |
| [PV-004](../../../02-design/review-resolution-contracts.md#ir24-ページング中の公開範囲縮小--pv-004) | 一覧1ページ後にOffer期限、scopeVersion不変 / 2ページと旧callbackを取得 | snapshot CONFLICT、viewEpoch更新、初頁からhistoryを取得 |
| [PV-005](../../../02-design/review-resolution-contracts.md#ir23-案件一覧の投影と検索--pv-001003) | 施工業者ID=nullの社内担当Assignmentが終了 / 本人と別担当で一覧/詳細/集計を取得 | 本人に凍結history、contractorOrgId=null、別担当へ履歴移管なし、稼働件数に加えない |

検証結果:

- [静的検証・TypeScript](static-check.json): 64要件、136操作、47画面、追跡・型・カタログの整合チェック合格。
- [検証器の異常検出](validator-negative-checks.json): 15/15ケースで意図した不整合を検出。
- [受入計画](../../acceptance-projection.csv): 5件を追加し追跡表へ接続。実装後のアプリ試験はnot_run。
- 一次資料4ファイルは前版のhashと一致。旧版の証跡は保持。

再実行:

```sh
python3 docs/tools/validate_documents.py
python3 docs/tools/validate_documents.py --tsc /private/tmp/ac-typescript-check/package/lib/tsc.js
python3 docs/tools/check_review_regressions.py
```

別環境では既存TypeScriptのlib/tsc.jsのパスを指定する。

これは修正担当自身の文書レビューであり、別担当による承認でもアプリ動作試験でもない。未知の不具合まで存在しないことを保証するものではない。長時間稼働の容量保証候補REV-018は、既存デモ範囲外として別管理を継続する。
