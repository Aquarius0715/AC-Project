# AC Project フロントエンド開発ドキュメント

版: 0.2.0 / 作成日: 2026-09-14 / 状態: レビュー用ドラフト / 言語: 日本語

対象はSplit Unit ACの監視・操作・保守・契約に関するクリック可能なフロントエンドデモ（1A）。アプリケーションの実装、本番API、機器接続は今回の成果物に含まない。

## 読む順序

| 順序 | 文書 | 用途 |
|---|---|---|
| 1 | [PrepareDocument](00-prepare/PrepareDocument.md) | 要件整理、参考サイトとの差分、仮定と未決事項 |
| 2 | [共通要件](01-requirements/common.md) | 全役割の業務境界・非機能要件・権限 |
| 3 | 役割別要件: [クライアント](01-requirements/client.md) / [施工業者](01-requirements/contractor.md) / [技術者](01-requirements/technician.md) / [管理者](01-requirements/admin.md) | 要件ID、優先度、受入条件 |
| 4 | [共通詳細設計](02-design/common.md) | データ、API境界、状態遷移、モック設計 |
| 5 | 役割別詳細設計: [クライアント](02-design/client.md) / [施工業者](02-design/contractor.md) / [技術者](02-design/technician.md) / [管理者](02-design/admin.md) | 画面、入力、処理、権限、異常系 |
| 6 | [共通UIUX仕様書](03-uiux/UIUXSpecification.md) | ライブラリ、状態管理、トークン、アクセシビリティ |
| 7 | [Agentic SDLC](04-agentic-sdlc/README.md) | エージェント分担、ゲート、引き継ぎ契約 |
| 8 | [参考デザイン分析](00-prepare/reference-design-analysis.md) / [共通実装契約](02-design/implementation-contracts.md) / [操作カタログ](02-design/operation-catalog.csv) | ソース由来の視覚値・詳細入出力 |
| 9 | [検証計画](04-agentic-sdlc/verification.md) / [追跡表](00-prepare/traceability.csv) | 要件→設計→テストの照合 |

4種類の本体文書はPrepareDocument、要件定義書、詳細設計書、UIUX仕様書。要件・設計は4役割に分割し、重複を防ぐため共通事項を別ファイルに置く。Agentic SDLC文書は実行時の付属資料であり、第5の製品仕様ではない。

## 文書の規約

- `CONFIRMED`: 提供資料または今回の明示依頼に由来する要件。製品として実装済みという意味ではない。
- `PROPOSED`: 本書で選んだ可逆的な設計案。通常のデモ実装は進行可能。変更時は決定記録を残す。
- `OPEN`: 人による業務判断または外部仕様が未確定。依存する本番処理は進めない。モックは仮定を明示して進める。
- `MUST` / `SHOULD` / `MAY`: 必須 / 推奨 / 任意。P0・P1はいずれも1A完了対象で、P0を先に実装する。P2は本番・将来拡張。
- ID接頭辞: `FR-C/P/T/A`=4役割、`FR-X`=共通、`NFR`=非機能、`DD`=設計、`UX`=共通UI、`AT`=受入テスト、`DEC/OPEN/GAP`=意思決定・未決・差分。
- 各役割の要件行は1つの検証単位。表の受入条件に行頭のIDから作る`AT-*`を付与する（例: FR-C01 → AT-C01）。S01〜S08は役割横断の追加検証。
- 優先順位: 最新ユーザー指示 → 確定要件 → 共通契約 → 役割別設計 → 提案。衝突は黙って上書きせず、影響IDを記録する。
- 承認者欄が空欄のものは未承認。実装／テスト／レビューエージェントは自分で業務承認を作らない。

## 変更管理

変更は元の要件IDを維持し、PrepareDocumentの決定表、影響する要件・設計、追跡表、受入条件を同じ変更単位で更新する。廃止IDは再利用しない。証跡は[成果物テンプレート](04-agentic-sdlc/templates/artifacts.md)に記録する。

現在の承認者: 未指定 / 実装状況: 未着手 / アプリ動作検証: 未実施。文書の整合確認とアプリの受入試験は別に扱う。

文書検証（0.2.0 / 2026-09-14）: 相対リンクと見出し参照、表の列数、64要件の網羅、49機能の詳細本文、147受入ケース、109操作の参照整合、原文の不変性を確認し、検出エラー0件。アプリの受入試験・ブラウザ描画検証は未実行。

追加確認（2026-09-14）: 指定された顧客Loyaltyページは制限環境外のHTTP取得で確認できました。[PrepareDocument](00-prepare/PrepareDocument.md)のSRC-05・GAP-11に追記しています。クリック・描画・本番処理は未検証です。

0.2.0: 参考HTML/CSSに合わせたtokenへ改訂。4役割49機能それぞれに業務規則・項目定義・例外・AT-N/E/Bを追加しました。概要表だけで実装せず、機能別本文と共通実装契約を合わせて使用してください。未確定の商用判断は引き続きPrepareのOPEN台帳を参照します。
