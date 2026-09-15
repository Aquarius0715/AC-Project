---
document_id: UX-COMMON
version: 0.3.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# 共通UIUX仕様書

4役割に共通する実装・操作・表示の規則を定義する。ワイヤフレームや画面配置図は対象外。各画面の業務処理は[詳細設計](../02-design/common.md)を参照する。ライブラリはDEC-02/03の提案標準。配色・書体・形状はユーザー指定のLoyaltyページの取得HTML/CSSに合わせる。根拠と補正箇所は[参考デザイン分析](../00-prepare/reference-design-analysis.md)を参照。

## UX-01. UIライブラリと取得元

| 用途 | 標準・取得元 | プロジェクト内の使用ルール |
|---|---|---|
| 基本UI | [shadcn/ui公式](https://ui.shadcn.com/docs) | 公式コンポーネントをshared/uiに取り込み管理する方式。ボタン、Dialog、Sheet、Tabs、Select、Popover等を共通化。独自の同等UIを役割別に作らない |
| アイコン | [Lucide React公式](https://lucide.dev/guide/react) | lucide-reactから名前付きimport。標準16px、主要操作20px、強調24px。線幅1.75を標準（参考sidebarのSVGに合わせる）。装飾はaria-hidden、アイコンのみのボタンは名前必須 |
| フォーム | [React Hook Form公式リポジトリ](https://github.com/react-hook-form/react-hook-form) | react-hook-formを標準とする。「reactForms」の解釈はDEC-03。入力状態はフォーム内で管理し画面stateへコピーしない |
| スキーマ検証 | Zod + @hookform/resolvers | 入力schemaとDTO schemaを区別。ユースケースの必須・形式・条件分岐をschemaへ集約 |
| データ取得・更新 | [TanStack Query公式](https://tanstack.com/query/latest/docs/framework/react/overview) | Repositoryへの非同期要求、キャッシュ、mutationを共通hookへ集約 |
| 一覧・グラフ | TanStack Table / Recharts | 複雑な並替え・ページングや時系列グラフに使用。単純一覧は共通Table。グラフを独自描画し直さない |
| 日付入力 | shadcn Calendar系 | 日付と時刻・タイムゾーンを別管理。表示書式はIntl、UTC変換を共通関数へ集約 |
| 翻訳 | i18next + react-i18next | en/jaのキーを同時更新。通知テンプレート・音声デモ応答も同じ辞書体系 |
| 検証 | Vitest、React Testing Library、Playwright、axe-core | ロジック・利用者操作・E2E・自動a11yで役割分担 |

公式資料の取得確認日: 2026-09-14。shadcn、Lucide、TanStack Queryは公式文書を確認。React Hook Formのガイド本文は取得できず公式リポジトリで確認。その他は本プロジェクトの採用候補であり、互換性・ライセンス・メンテナンス状況は実装開始時に確認する。バージョン番号を推測して固定しない。

既存実装はない。導入時に1つの互換セットとlockfileを作り、採用版・ライセンスを記録する。同じ目的のUIキットやアイコンセットを追加しない。例外は必要機能、標準で足りない理由、負担、影響、置換可能性を決定記録に残す。セマンティックHTMLで足りる部分にライブラリを強制しない。

## UX-02. State・Effect・Contextの責任

| 状態の種類 | 保管先 | 禁止する二重管理 |
|---|---|---|
| Repository由来の設備・案件・請求・履歴 | TanStack Query cache | 取得結果をuseState/Context/Zustand等へ再コピー |
| 検索・ソート・ページ・期間・選択物件 | URL search params（schema検証） | URLとローカルstateの双方向Effect同期 |
| 入力値・dirty・validation・送信状態 | React Hook Form | 各入力をuseStateで再管理、独自エラー辞書 |
| ダイアログ開閉など短命な局所状態 | コンポーネントのuseState、必要ならuseReducer | 全画面Contextへの格納 |
| 絞込結果・合計・ボタン有効判定 | render時の純粋な導出関数 | 導出値をEffectで別stateに格納 |
| テーマ・locale・セッション・Repository参照 | ライブラリProvider/小さなContext | テレメトリー等高頻度更新を巨大Contextに混在 |
| 共有デモ業務データ | mock Repository内部 | 画面ごとのseed変更、Contextを疑似DBに使用 |

Effectは外部システムへの同期が必要な場合に限定する。イベントに起因する保存はイベントハンドラー/mutationで実行し、依存配列を隠して動作を抑えない。React公式も不要なEffectを避ける考え方を説明している。[React公式: You Might Not Need an Effect](https://react.dev/learn/you-might-not-need-an-effect)

許可例は購読/解除、メディアAPI、DOM外部widgetとの同期。理由とcleanupをセットにし、Strict Modeの再実行で二重登録しない。取得は原則Query hookへ。`useMemo`/`useCallback`は測定や参照安定性の必要がある場合に限定する。

```tsx
// 方針例: 選択はURL、取得はQuery、表示用の変換は純粋な導出。
const filters = parseUnitFilters(searchParams);
const unitsQuery = useUnits(filters);
const visibleUnits = selectVisibleUnits(unitsQuery.data?.items ?? [], filters);
// visibleUnitsを別stateへコピーするEffectは作らない。
```

Contextはセッション等の配布に限定し、必要なProviderは許可する。「Context禁止」「useEffectゼロ」「useStateゼロ」を目的化しない。レビューでは保管先が1つか、外部同期が必要かを判断する。

## UX-03. フォーム標準

- `useForm`＋`zodResolver`を基本とし、ネイティブ入力はregister、制御型コンポーネントはControllerを必要箇所だけに使う。配列入力はuseFieldArray。
- ラベル、必須/任意、説明、単位、エラー、disabled理由を共通Fieldで統一。placeholderだけをラベルにしない。
- 初期値はschemaと一致させ、非同期読込後のresetは対象ID変更時または明示的再読込時に限る。バックグラウンド更新でdirty入力を上書きしない。
- 初回検証はblur/submit、エラー後は修正時に再検証する方針。送信時は最初のエラーへフォーカスし、上部要約から該当項目へ移動できる。
- 送信中は二重送信を抑制し、失敗時は値を保持。モックのVALIDATION結果はfieldErrorsへ、CONFLICT結果は競合案内へ変換する。
- 点検ドラフト保存は未完了入力を許容、提出schemaは必須を満たす。完了状態のフォームを直接書き換えず新報告版を作る。
- 温度・時間・金額は文字列から境界で変換。空文字を0にしない。許容範囲は能力・共通schemaから取得する。
- 未保存で離脱する場合のみ破棄確認。通常の閲覧遷移には不要な確認を追加しない。

## UX-04. 参考準拠のデザイントークン

唯一の値定義先は`src/shared/styles/tokens.css`。以下が0.2.0の正本で、旧Inter・ティール補助色・1440px幅案を置換する。`REF`は取得ソースにある値、`ADAPT`は今回の要件のための明示的補正。根拠は[抽出証跡](../00-prepare/sources/reference-style-evidence.json)。

| semantic token | 採用値 | 根拠・用途 |
|---|---|---|
| --color-primary / --color-primary-fg | #005BEA / #FFFFFF | REF 主ボタン、選択nav |
| --color-primary-soft | #E6F0FF | REF アイコン枠、選択候補の背景 |
| --color-primary-hover | rgb(0 91 234 / .90) | REF hover:bg-primary/90。背景と合成し固定の濃青へ勝手に変更しない |
| --color-background / --color-surface / --color-surface-2 | #F8FBFF / #FFFFFF / #EDF6FF | REF ページ・カード・補助領域 |
| --color-secondary / --color-secondary-fg | #EDF6FF / #0D2238 | REF secondary。ティール系を第2ブランド色にしない |
| --color-text / --color-text-muted | #0D2238 / rgb(13 34 56 / .72) | REF 本文・説明 |
| --color-text-subtle-reference / --color-text-subtle | rgb(13 34 56 / .50) / rgb(13 34 56 / .72) | REF値を残し、12px程度の意味あるラベルはADAPTで濃くする |
| --color-hero / --color-hero-fg | #0D2238 / #FFFFFF | REF 主要サマリー、補助文字は白72%へADAPT |
| --color-border / --color-input-border | #D6E4F5 / #71849A | 装飾REF、入力境界はADAPTで視認性を補強 |
| --color-success-accent / --color-success / --color-success-soft | #059669 / #166534 / #DCFCE7 | REF装飾・背景、文字はADAPT |
| --color-critical-accent / --color-critical / --color-critical-soft | #FF4757 / #B91C1C / #FFE5E9 | REF装飾・背景、文字はADAPT |
| --color-warning / --color-warning-soft | #9A3412 / #FFEDD5 | REF、オレンジ系の注意 |
| --color-unknown / --color-unknown-soft | #475569 / #EDF6FF | ADAPT、不明/欠測を正常色にしない |
| --color-focus | #005BEA | REF 2px outline＋2px offset。ボタンは補助ring3px/50% |
| --chart-series-1〜4 | #005BEA / #0D2238 / #7C3AED / #9A3412 | ADAPT、1/2はブランド色。色＋線種＋凡例 |

| 分類 | tokenと値 | 使用規則 |
|---|---|---|
| 本文font | --font-sans: "Plus Jakarta Sans", "Noto Sans JP", system-ui, sans-serif | REF英数字、ADAPT日本語fallback。font-display:swap |
| ID/コードfont | --font-mono: "Geist Mono", ui-monospace, monospace | REF。契約ID/機器serialに限定 |
| 任意display font | --font-display: "Bricolage Grotesque", var(--font-sans) | REF定義はあるが業務h1の標準には使わない |
| 文字サイズ | --text-xs:12px / sm:14px / base:16px / lg:18px / xl:20px / 2xl:24px / 3xl:30px / 5xl:48px / 6xl:60px | REF scale。本文14〜16px、重要説明は12px以上。5xl/6xlはhero数値だけ |
| 行高 | xs:16px / sm:20px / base:24px / 2xl:32px / 3xl:36px / hero:1 | REF。日本語長文はbody1.6へADAPT |
| 太さ・字間 | normal400 / medium500 / semibold600 / bold700、KPI数値-.01em | REF。英語短いlabelのみ.12em、和文へuppercase/広い字間を適用しない |
| 余白 | --space-1〜8:4/8/12/16/20/24/28/32px、--space-12:48px | REF 4px基準。カード内16/20px、hero24→28px |
| 角丸 | --radius-control:10px / --radius-card:14px / --radius-nav:16px / --radius-small:6px | REF。rounded-lg=16、rounded-xl=14の実値を保持 |
| 影 | --shadow-card:0 14px 34px rgba(13,34,56,.055) | REF。1px白70%ring、border90%と組合せ可能 |
| ボタン影 | --shadow-action:0 10px 22px rgba(0,91,234,.18) / hover:0 14px 30px rgba(0,91,234,.24) | REF primaryだけ。全カードをhover浮上させない |
| 動き | --duration-normal:200ms、--ease-out:cubic-bezier(0,0,.2,1) | REF hover y=-2px、press y=1px/scale=.985。reduced-motionでは移動なし |
| 内容幅 | --content-max:1152px | REF max-w-6xl=72rem。margin-inline:autoは参考HTMLにないため自動追加しない |
| sidebar | --sidebar-width:240px / --sidebar-width-icon:56px | REF画面inline。ライブラリ既定256/48pxを使わない |
| breakpoint | sm640 / md768 / lg1024 / xl1280px | REF。nav切替はxl、KPIはmd、補助領域はlg |
| 層 | --z-sidebar:10 / --z-header:30 / --z-modal:50 / --z-toast:60 | 前2つREF、後2つADAPT。重なりとfocusを共通管理 |

shadcnの`--primary`/`--background`/`--card`/`--muted`/`--border`を上記へマッピングする。参考の.bg-backgroundは白、ページ.bg-bgは薄青なので、両者を1つのtokenに統合しない。raw値はこの表と生成元だけに置き、各画面ではsemantic tokenを参照する。

fontは公式配布のライセンスを確認してローカル配信する。参考のハッシュ付きwoff2を恒久hotlinkしない。日本語fontは参照サイトの確認値ではなく追加設計。dark modeとブランドロゴの複製は今回の標準に含めない。

## UX-05. コンポーネントの共通契約

| 共通コンポーネント | 受け取る情報 | 規則 |
|---|---|---|
| AppShell / RoleNavigation | role、許可ルート、表示名 | ナビ表示とサービス認可は両方必要。現在位置をaria-currentで表現 |
| MetricCard / TelemetryValue | value/null、unit、origin、quality、observedAt、isDemo | 未計測は「— 未計測」。デモ・推定・更新時刻を隠さない |
| StatusBadge | domain、status、labelKey | 設備・通信・案件・請求の状態名を別辞書で管理。色だけで区別しない |
| DataTable | columns、rows、sort、pagination、rowAction | キーボードとモバイルで操作可能。大量表はページング。操作は行クリックのみに依存しない |
| TimeSeriesChart | series、unit、quality、period | 欠測を線で連結せず、数値表と凡例を併設。二軸は単位を明示 |
| CommandPanel | capability、observedState、pendingCommand、permission | 要求中を成功トーストで上書きしない。非対応理由を表示 |
| ConfirmActionDialog | target、action、impact、reason、onConfirm | 制限・解除・試運転・FW・音声変更に共通。初期フォーカスは安全な選択 |
| AsyncBoundary / EmptyState | status、messageKey、retryAction | 永続的なエラーを一時トーストだけにしない |
| AuditTimeline / NotificationPreview | actor、time、action、result、correlationId | 「プレビュー・未送信」を表示。既読と業務完了を区別 |

UI primitivesは業務Repositoryを呼ばない。業務コンポーネントはtyped propsとcallbackを受け、データ取得はfeature hookへ。役割の違いは権限や表示データで表し、コピーした同一コンポーネントを4つ保守しない。

## UX-06. 操作・言語・アクセシビリティ

グラフや状態カードは概要から根拠・詳細へ進める。期間・組織・単位は常に分かる位置に表示する。成功、保留、失敗を明確にし、エラーには次の行動を添える。破壊的操作の直後に「元に戻す」を出す場合も実際に取り消せるものだけにする。

英語・日本語のキー、複数形、長い翻訳を検証する。時刻はIntl.DateTimeFormat、金額はIntl.NumberFormatで整形し、予約にはタイムゾーンを表示する。言語変更で測定単位や保存UTCを勝手に変えない。音声デモは文字起こし・対象・操作内容を確認してから通常Commandへ渡す。

WCAG 2.2 AAを設計目標とする。通常文字のコントラスト4.5:1、大きな文字3:1、操作対象の識別・フォーカスの視認性を検証する。これは適合保証ではなく実装時の検査基準。[W3C公式クイックリファレンス](https://www.w3.org/WAI/WCAG22/quickref/)

プロジェクトの操作領域目標は44×44px。キーボードで主要シナリオを完遂し、Dialogを閉じると呼出元へフォーカスを戻す。重大エラーは適切なlive regionで通知し、テレメトリー更新を毎回読み上げない。ブラウザ拡大200%、360px幅、スクリーンリーダーでフォームと状態を確認する。

## UX-07. UIレビューの合格条件

- ライブラリ取得元・採用版と共通componentの場所を報告し、重複キットを追加していない。
- フォーム、Query、URL、局所stateの責務が分かれ、Effectごとに外部同期の理由がある。
- 色・フォント・サイズ・余白のtoken参照を確認し、状態色に文字とアイコンがある。
- loading/empty/error/forbidden/offline/staleと送信失敗時の入力保持を確認する。
- スマホ、キーボード、日英切替、欠測、長文、音声代替を含め検証し、未実施を明記する。
- 機器確認前の成功表示、実取引と誤認する表現、CO₂単位混同、既読で異常解消がない。

## UX-08. 参考準拠の画面パターンと検収

ワイヤフレームは作らず、再利用可能な構成規則を定義する。各役割の詳細設計にpatternを明記する。

| Pattern ID | 構成・寸法規則 | 主な用途 |
|---|---|---|
| UI-OVERVIEW | タイトル24→30px＋説明14px、必要なら濃色hero、KPI2→4列、下部カード。page padding x16/y24→x32/y32（xl） | 各役割dashboard |
| UI-LIST | 同じshell、見出し＋操作、検索/絞込、白カードの一覧、件数/ページング、詳細導線 | 設備・案件・請求・デバイス |
| UI-DETAIL | 主要情報カードと関連履歴。lg以上はminmax(0,1fr)+320px、gap20px。小画面は縦並び | 設備・案件・請求詳細 |
| UI-FORM | 白カード内の14px label/16px input、意味ごとのsection、欄間16px、主操作と取消。エラー要約 | 場所、報告、方針、契約 |
| UI-ANALYSIS | 共通KPI、単位・期間、chart＋数値table、根拠/品質card | 電力、空気環境、MRV |
| UI-TIMELINE | 白カード、行py12px、左に事象と時刻、右に結果badge。長いIDは折返し | 監査・案件履歴 |

PC navは1280px以上で固定左、未満は上部header＋Sheet。drawerは背景スクロールを止め、Escで閉じ、選択後に閉じて遷移先見出しへfocus。roleが違っても色やfont体系を変更せず、メニューと権限・主要指標で区別する。

参照との差分はADAPTとして記録する。360px、768px、1024px、1279px、1280px、1440pxで幅・折返し・nav境界を検証する。入力を含む業務画面では44px操作領域を優先し、参考の28px操作をそのままコピーしない。

検収はtoken値、font適用、card radius14px、sidebar240/56px、breakpoint、主要componentをDOM/computed styleで確認し、実装のスクリーンショットを記録する。参照ページとのpixel比較は参照側の描画基準を別途取得できた場合に実施し、未実施を合格としない。
