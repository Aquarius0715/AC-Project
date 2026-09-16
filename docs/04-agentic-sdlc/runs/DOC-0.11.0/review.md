# DOC-0.11.0 修正・再レビュー結果

元の17件を修正し、再レビューで見つけた追加6件も修正した。長時間稼働のメモリ上限1件は追加要件候補として保留。設計文書の自己再レビューは完了、独立G1はpending。

対象baseline: `43d3c2b9c31307109808c42d41a7c71b6cac91c12af81dceca7ed29c5d475648`。旧0.10.0のレビュー/manifestは履歴として保持し、今回の承認には流用しない。

## 修正と再レビューの経過

1. 第1回: 元レビュー18件を判定し、17件を型・権限・状態遷移・画面/操作/版カタログへ反映。容量候補1件は保証範囲を明記。
2. 第2回: 強制解除の一覧入口とfilter権限、通知分類の起点ID、決済操作名、Policyフォーム依存、測定生成時の必須値、閲覧世代とcursorの6件を修正。
3. 第3回: 変更の波及を再照合。既存Policy名の上限120文字を維持しフォーム表を統合、詳細DDの操作一覧を同期。静的・型・検証器の異常検出を確認。

## 元指摘の対応

| ID | 指摘 | 対応 |
|---|---|---|
| REV-001 | 受諾・辞退の書込み応答と公開範囲が両立しない | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir01-受諾と辞退の最小応答) |
| REV-002 | 顧客変更後に過去データの所有境界を保持する契約がない | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir02-所有顧客の不変性) |
| REV-003 | 独立した強制解除権限から必要な読取へ到達できない | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir03-強制解除専用権限) |
| REV-004 | HQ督促プレビューを顧客画面へ反映する要件に保存経路がない | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir04-督促の共有された模擬記録) |
| REV-005 | 予告日時の遡及入力で24時間の予告条件を満たせてしまう | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir05-予告の時刻と証跡) |
| REV-006 | Payment initiated中の別キー再開始が未定義 | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir06-決済試行の排他) |
| REV-007 | アラート・空気方針の必須入力を画面から組み立てられない | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir07-policy共通フォーム) |
| REV-008 | 推定電力から作った基準をmeasuredと表示し得る | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir08-電力量の由来) |
| REV-009 | 共通音声パネルから技術者の診断コマンドを完成できない | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir09-音声変更の操作文脈) |
| REV-010 | 清掃予定と故障通知を区別する識別子がDTOにない | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir10-通知の業務分類) |
| REV-011 | 実績の算定境界が不明で境界一致を検証できない | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir11-算定境界の識別) |
| REV-012 | 未知の測定単位をsuspectで表示する契約とDTO拒否が衝突する | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir12-生測定と正規測定) |
| REV-013 | 最新設計と旧受入条件で正解が異なる | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir13-現行受入条件の統一) |
| REV-014 | 文書先頭の版案内が現行baselineと一致しない | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir14-現行版の識別) |
| REV-015 | MRVの書き出しは1A機能なのか | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir15-mrv出力の範囲) |
| REV-016 | 非センサーFactの鮮度と保持期限が決まっていない | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir16-factの鮮度と再利用) |
| REV-017 | セッション世代をいつ増分するかが未確定 | [仕様修正済み](../../../02-design/review-resolution-contracts.md#ir17-repository世代と閲覧世代) |
| REV-018 | Missing Requirement Candidate：長時間デモのメモリ上限 | [追加要件候補として保留](../../../02-design/review-resolution-contracts.md#ir18-長時間メモリ保持の扱い) |

## 検証

- [静的検証結果](static-check.json): 64要件、136操作、47画面、57 Component、93 write分岐、追跡・リンク・カタログ整合のエラー0。TypeScript strict/noEmitも合格。
- [検証器の異常検出結果](validator-negative-checks.json): 意図的に壊した8ケース全件を想定の診断で検出。元仕様は変更せず一時コピーで実施。
- [受入計画](../../acceptance-resolution.csv): 元18件＋追加6件。全件execution_status=not_run。アプリが動いたという証跡ではない。
- 旧baseline収録の一次資料はハッシュ一致を確認。詳細は[再レビュー記録](design-review-result.json)。

再実行コマンド:

```sh
python3 docs/tools/validate_documents.py
python3 docs/tools/validate_documents.py --tsc /private/tmp/ac-typescript-check/package/lib/tsc.js
python3 docs/tools/check_review_regressions.py
```

`--tsc`はこの環境に既存のTypeScriptコンパイラを使った。別環境ではインストール済みのlib/tsc.jsを指定する。

## 引継ぎ

変更はフロントエンド1Aの設計文書。アプリ実装・動作試験は未実施。REV-018は100設備/1000サンプル規模を超える長時間連続稼働の容量保証であり、既存のresetまで保持する契約を壊す自動削除は加えていない。将来その利用範囲を採用するときに上限/拒否/退避を設計する。

本記録は修正担当自身の再レビューであり、独立G1の承認ではない。[ゲート](gate-G1.yaml)はpendingを維持する。
