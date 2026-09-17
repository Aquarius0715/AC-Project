# DOC-0.21.0 修正報告

DOC-0.20.0の独立G1で検出したG120-001〜005をIR103〜106と受入計画へ反映した。仕様修正担当はCodex root、検証器修正担当はfixtures_review。修正担当による自己確認であり、独立G1承認ではない。最新の独立判定は[gate-G1.yaml](gate-G1.yaml)を参照。

| 指摘 | 修正 | 受入 |
|---|---|---|
| G120-001 | IR103: 新規方針の現在tickから継続時間を測り59秒/60秒を分離。A12入力と実行順を更新 | AT-G121-001 |
| G120-002 | IR104: alertテンプレートはIR10の分類を適用 | AT-G121-002 |
| G120-003 | IR104: 業務通知のseverity表を追加 | AT-G121-003 |
| G120-004 | IR105: allergen_observation変更イベントとtelemetry.series無効化を追加 | AT-G121-004 |
| G120-005 | IR106: 通知fixtureのscopeVersionAtCreationの明示値検証と省略時補完 | AT-G121-005 |

DEC-60〜61はPROPOSEDの可逆的なデモ具体化。企業承認や本番仕様の確定ではない。仕様変更前の独立G1証跡は旧baselineの記録として保存し、流用しない。アプリ実装・動作試験は未実施。コミット・pushは行っていない。

## 修正担当による検証

[静的検証](static-check.json)はerrors=[]、TypeScript strictはpassed。[変異テスト](validator-negative-checks.json)は既存84件＋追加18件の102/102を検出。[追加入力の型確認](contract-type-examples-result.json)で方針保存・開始/最終評価・load_alert/allergenの完全な入力を正規DTOへ照合した。[baseline照合](baseline-integrity-check.json)は全65ファイル一致。これらは文書・型の検査であり、アプリ動作試験は未実行。

## 独立レビュー後の追加修正

初回の独立レビューで、AT-G121-002が未定義の「故障フィルター」を要求する軽微な受入文言の欠陥を検出した（UI21-001）。既存C08の「故障アイコン・ラベル」に修正した。機能・DTO・権限の変更はない。初回baselineと検証証跡は`independent-g1/round-1/`へ保存し、最終baselineで再判定する。

## 最終結果

最終baseline `02a1e39793df35f760afe9188e86141c8790cd7e92366efbddb7f62aff62e908` に対し、修正未担当のcontracts_reviewとui_requirements_reviewが独立にPASS、未解決指摘0件と判定。[最終統合報告](independent-g1/review.md)と[2回のサイクル記録](review-cycle.json)を参照。
