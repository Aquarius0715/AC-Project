# AC Project フロントエンド開発ドキュメント

版: 0.6.0 / 作成日: 2026-09-14 / 更新日: 2026-09-15 / 状態: レビュー用ドラフト / 言語: 日本語

対象はSplit Unit ACの監視・操作・保守・契約に関するクリック可能なフロントエンドデモ（1A）。今回作成する文書はフロントエンド設計のみ。API仕様・HTTP契約・DB・サーバー処理・本番運用は設計対象外。将来のAPI接続はフロント側interfaceの差替え口だけを定義する。アプリ実装も今回の作業には含まない。

## 読む順序

| 順序 | 文書 | 用途 |
|---|---|---|
| 1 | [PrepareDocument（企業向け）](00-prepare/PrepareDocument.md) | 企業のオリジナル要件の整理・分析、フロントエンドへの展開方針 |
| 2 | [共通要件](01-requirements/common.md) | 全役割の業務境界・非機能要件・権限 |
| 3 | 役割別要件: [クライアント](01-requirements/client.md) / [施工業者](01-requirements/contractor.md) / [技術者](01-requirements/technician.md) / [管理者](01-requirements/admin.md) | 要件ID、優先度、受入条件 |
| 4 | [共通詳細設計](02-design/common.md) | 表示データ、差替えinterface、状態遷移、モック設計 |
| 5 | 役割別詳細設計: [クライアント](02-design/client.md) / [施工業者](02-design/contractor.md) / [技術者](02-design/technician.md) / [管理者](02-design/admin.md) | 画面、入力、処理、権限、異常系 |
| 6 | [共通UIUX仕様書](03-uiux/UIUXSpecification.md) | ライブラリ、状態管理、トークン、アクセシビリティ |
| 7 | [Agentic SDLC](04-agentic-sdlc/README.md) | エージェント分担、ゲート、引き継ぎ契約 |
| 8 | [参考デザイン分析](00-prepare/reference-design-analysis.md) / [フロントエンド入出力契約](02-design/implementation-contracts.md) / [操作カタログ](02-design/operation-catalog.csv) | ソース由来の視覚値・詳細入出力 |
| 9 | [検証計画](04-agentic-sdlc/verification.md) / [追跡表](00-prepare/traceability.csv) | 要件→設計→テストの照合 |

4種類の本体文書はPrepareDocument、要件定義書、詳細設計書、UIUX仕様書。要件・設計は4役割に分割し、重複を防ぐため共通事項を別ファイルに置く。Agentic SDLC文書は実行時の付属資料であり、第5の製品仕様ではない。

## 文書の規約

- 出所は「企業原文 SRC-06」「制作方針 SRC-02」「参考モックの観察 SRC-05/04」「設計補完」に分ける。企業原文由来でも詳細承認済みとは扱わない。
- `PROPOSED`: 本書で選んだ可逆的な設計案。通常のデモ実装は進行可能。変更時は決定記録を残す。
- `OPEN`: 人による業務判断または外部仕様が未確定。依存する本番処理は進めない。モックは仮定を明示して進める。
- `MUST` / `SHOULD` / `MAY`: 必須 / 推奨 / 任意。P0・P1はいずれも1A完了対象で、P0を先に実装する。P2は本番・将来拡張。
- ID接頭辞: `FR-C/P/T/A`=4役割、`FR-X`=共通、`NFR`=非機能、`DD`=設計、`UX`=共通UI、`AT`=受入テスト、`DEC/OPEN`=意思決定・未決。
- 各役割の要件行は1つの検証単位。表の受入条件に行頭のIDから作る`AT-*`を付与する（例: FR-C01 → AT-C01）。S01〜S08は役割横断の追加検証。
- 優先順位: 最新の制作指示 → 企業原文 → 原文から整理した要件 → 共通契約・役割別設計。参考モックは外観設計の参考資料。衝突は黙って上書きせず、影響IDを記録する。
- 承認者欄が空欄のものは未承認。SRC-02の過去原指示は未収録であり、[確認状態](00-prepare/sources/production-instructions.md)を参照する。実装／テスト／レビューエージェントは自分で業務承認を作らない。

## 変更管理

変更は元の要件IDを維持し、PrepareDocumentの決定表、影響する要件・設計、追跡表、受入条件を同じ変更単位で更新する。廃止IDは再利用しない。証跡は[成果物テンプレート](04-agentic-sdlc/templates/artifacts.md)に記録する。

現在の承認者: 未指定 / 実装状況: 未着手 / アプリ動作検証: 未実施。文書の整合確認とアプリの受入試験は別に扱う。

0.5.0（2026-09-15）: [企業原文](00-prepare/sources/company-requirements-original.txt)から26項目を整理し、共通・4役割の要件と詳細設計、UIUX仕様書を改訂しました。[企業要望対応表](00-prepare/company-requirement-map.csv)・[要件別の出所表](00-prepare/requirement-origins.csv)・[追跡表](00-prepare/traceability.csv)で原文、設計補完、受入条件を確認できます。窓開放・断熱不足、アレルゲン、カード区分、Scope 2、市場連携構想の表示を具体化しました。参考HTML/CSSの外観値は保持しています。

各エージェントは原文・出所表を先に読み、設計補完を企業の承認事項として扱わないでください。詳細本文・入力契約・受入条件を合わせて使用します。文書の構造検証とアプリ動作検証は区別し、アプリ試験は未実行です。

0.6.0（2026-09-15）: 入出力・読取・試運転・手動入金・制限解除の契約を具体化し、117操作に更新。受入条件を具体的な入力と期待結果へ整理し、追加R01 10件を含む182ケース束を追跡します。実行状態schema、仕様baselineと変更時の再判定を統一しました。[制作指示の確認状態](00-prepare/sources/production-instructions.md)で未収録の原指示と設計解釈を区別します。アプリ試験は未実行です。
