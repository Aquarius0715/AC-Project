---
document_id: PREP-STYLE
version: 0.2.0
status: source-inspected
owner: design-agent
updated: 2026-09-14
---

# 参考ページのデザイン分析・採用根拠

対象は[顧客Loyaltyページ](https://aconland-mudah-milik.vercel.app/customer/loyalty)。HTMLとそこから参照されるCSS 2件・公開JavaScriptの一部を読み取りで取得した。ブラウザ描画・computed style・特典交換処理は未検証。以下はソース上で確認できた宣言とクラスの対応であり、ピクセル一致の検証結果ではない。

## 根拠と確認方法

[抽出値・取得物ハッシュ](sources/reference-style-evidence.json)にURL、SHA-256、CSS宣言、HTMLクラスを保存した。個人名・メール等を含むHTML全体はdocsへコピーしていない。

| 根拠ID | 原本 | 確認内容 |
|---|---|---|
| STYLE-01 | [フォントCSS](https://aconland-mudah-milik.vercel.app/_next/static/chunks/0xh6wh_1u6m3d.css) | Plus Jakarta Sans、Geist Mono、Bricolage Grotesqueのfont-faceと変数 |
| STYLE-02 | [共通CSS](https://aconland-mudah-milik.vercel.app/_next/static/chunks/0xipyi-ld-nvh.css) | 色・角丸・影・文字サイズ・breakpoint・focus宣言 |
| STYLE-03 | 上記Loyalty HTML | 実際に付与されたclass、sidebar幅のinline変数、見出し/カード/ボタン構造 |
| STYLE-04 | [公開UI chunk](https://aconland-mudah-milik.vercel.app/_next/static/chunks/0_n04r.htcg3l.js) | mobile判定は1280px未満。sidebar既定幅は画面inline指定で上書きされる |

## 採用する視覚言語

| 対象 | 確認した値・構成 | 採用規則 |
|---|---|---|
| 主色 | #005BEA、白文字、淡青#E6F0FF | primary/primary-soft。以前の#1D4ED8案を置換 |
| ページとカード | 背景#F8FBFF、surface白、surface-2 #EDF6FF | 薄青の画面＋白カードを全役割で共通化 |
| 濃色 | #0D2238 | 本文・見出し・強調パネル。濃色パネルは1画面の主要サマリーに限定 |
| 補助文字 | #0D2238の72% / 50% | muted / subtle。重要ラベルは薄くしすぎず下記例外で補正 |
| 本文font | Plus Jakarta Sans | 全業務画面の英数字。h1にも別fontクラスがなく同じfontを継承 |
| 補助font | Geist Mono、Bricolage Grotesque | monoはID/コードに使用。Bricolageは定義があるだけでLoyaltyのh1へ自動適用しない |
| 見出し | h1は24→30px（768px以上）、700。h2は16px/600 | typography scaleで共通化 |
| カード | rounded-xl=14px、border #D6E4F5、影0 14px 34px rgba(13,34,56,.055) | クラス名から既定値を推定しない。tokenへ直接定義 |
| ボタン/その他角丸 | rounded-md=10px、rounded-lg=16px | cardとcontrolで別semantic token。大小の名前順に数値を並べ替えない |
| 内容幅・余白 | max-w-6xl=72rem、横16→32px、縦24→32px（1280px以上） | 基本ページ幅1152px。dense tableの拡幅は理由付き例外 |
| ナビ | 左sidebar 240px、折畳み56px。1280px未満は上部headerとdrawer | CSSとJSの境界値を共通化。640/768/1024/1280の各役割も分離 |
| 情報階層 | 見出し→濃色サマリー→KPI→詳細/履歴 | 共通page patternとして採用。ポイントを各役割の業務指標に置換 |
| KPI | 2列→4列（768px以上）、gap12→16px、値24→30px | 長い翻訳・360pxで崩れる場合は1列へ落とす明示的拡張 |
| 明細＋履歴 | 1024px以上で本文＋20remの補助領域、gap20px | 任意の共通detail pattern。小画面では履歴を下へ |
| アイコン | inline SVG主体、sidebarに16px/stroke1.75、toggleにLucideを確認 | Lucideへ統一してサイズ・線幅を合わせる。全SVGがLucide由来とは断定しない |

## 機械的にはコピーしない部分

- 参考にある10〜11pxの重要説明・高さ28pxの操作は、業務UIでは文字12px以上、主要本文14〜16px、タッチ領域44px以上へ補正する。見た目の系統を維持し、差分を隠さない。
- success文字#059669と淡緑背景の組合せなどは、算定したコントラストによってはAA目標に不足する。文字用semantic tokenは#166534へ補正し、参考値を装飾用として分ける。
- hover移動や押下scaleは参考値を採用できるが、reduced-motionでは停止する。監視中の値を装飾アニメーションで揺らさない。
- 背景grid/auroraは共通CSSに存在する。Loyalty本文の不透明背景に隠れる可能性があるため、業務画面へ新規追加しない。
- ポイント・ロイヤルティ機能、参考の人物情報、ロゴ素材を自動的に製品要件や再配布資産へ取り込まない。ブランド画像は提供があるまでAC Projectの文字表記とする。

最終採用値は[共通UIUX仕様書](../03-uiux/UIUXSpecification.md)のUX-04/08を正とする。HTML/CSSの値確認を、サイト全画面の操作検証や本番品質の確認と取り違えない。
