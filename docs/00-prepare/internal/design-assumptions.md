---
document_id: PREP-INTERNAL
version: 0.5.0
status: working-notes
scope: frontend-only
updated: 2026-09-15
---

# 開発者向け補足：仮定・技術選択

企業への説明は[PrepareDocument](../PrepareDocument.md)を正とする。本ファイルは設計・実装エージェント向けの補足であり、企業原文の要求や企業による承認の記録ではない。最新の企業原文はSRC-06、開発範囲の指定はSRC-02として区別する。

## 技術選択とデモの仮定


| ID | 状態 | 内容・理由 | 変更時の影響 |
|---|---|---|---|
| DEC-01 | PROPOSED | 施工業者は受託と自社人員・品質を担当。HQの委託範囲内のみ再割当 | FR-P全件、S02/S08、MaintenanceJob |
| DEC-02 | PROPOSED | 新規SPAはReact + TypeScript + Vite + React Router。SSRが必要なら再検討 | 共通設計・ビルド。依存版は実装開始時に互換性を確認・固定 |
| DEC-03 | PROPOSED | 「reactForms」をReact Hook Formと解釈。Zod、shadcn/ui、Lucide、TanStack Queryを利用 | UIUX・フォーム・データ境界 |
| DEC-04 | PROPOSED | 英語・日本語を初期デモ言語、英語を初期選択。追加言語・提供順序は未確定 | 翻訳キーとAT-X01/X02 |
| DEC-05 | PROPOSED | デモ通貨MYR、表示時間帯Asia/Kuala_Lumpur。設定から変更可能 | 市場決定ではない。金額・時刻整形 |
| DEC-06 | 制作方針（ドキュメント作成者指定） | 顧客LoyaltyのHTML/CSSのデザインに合わせる。主色#005BEA、Plus Jakarta Sans、14pxカード。旧独立配色案を廃止 | [デザイン分析](../reference-design-analysis.md)とUIUXに根拠を記録。a11y補正はADAPTとして明示 |
| DEC-07 | PROPOSED | 共有メモリのデモRepository。再読込でseedに戻る。同一タブの役割切替で保持 | デモ操作説明、テスト。永続化・複数タブ同期は対象外 |
| DEC-08 | PROPOSED | 音声は文字起こし・応答のシミュレーションが標準。実マイク不要 | FR-X02、同意・拒否デモ |
| DEC-09 | PROPOSED | 49機能の入力・業務細則・出力・失敗を具体化。デモの時間/上限/受諾前projection/確定予定重複拒否等は[実装契約](../../02-design/implementation-contracts.md)と各DDを正とする | 本番の承認を意味しない。変更時は同じIDの要件・設計・AT-N/E/Bを更新 |

PROPOSEDの項目は人による業務承認済みではない。DEC-06の参考準拠方針は最新の明示指示に基づく。可逆的な1Aは提案基準で進行できる。


## 確認範囲の記録

参考モックのLoyaltyページは2026-09-14にHTML・参照CSS・公開コードの一部を取得した。他の画面は既存引き継ぎ資料の調査記録が根拠で、同じ深さの再確認を行っていない。初回取得時の環境制約は調査上の事情であり、参考アプリの機能不足を示すものではない。参照値は[デザイン分析](../reference-design-analysis.md)と[抽出証跡](../sources/reference-style-evidence.json)を参照。

## 原文との対応の読み方

FRは企業要件原文（SRC-06）から再整理した管理番号です。[要件別の出所表](../requirement-origins.csv)で原文の記述位置と設計補完を確認します。例えば担当割当、報告承認、入金確認、FW校正・登録の詳しい手順は、企業の監視・保守という目的を実現するための設計補完を含む。

画面での赤/オレンジ/緑の意味づけ、同意取得、値の品質区分、状態遷移の名称・時間値は、企業目的を支えるフロントエンド提案として扱う。企業原文中の検討質問や期待値を、検証済み能力や達成保証へ変換しない。
