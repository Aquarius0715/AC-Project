---
document_id: DD-C
version: 0.3.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# クライアント 詳細設計書

## 入力・責務

入力: [役割別要件](../01-requirements/client.md)、[共通要件](../01-requirements/common.md)。必読: [共通詳細設計](common.md)、[UIUX仕様書](../03-uiux/UIUXSpecification.md)。以下はフロントエンドの項目・表示・モック動作の設計。画面上の登録・割当・入金・制限・監査はすべて共有モックメモリの状態遷移で、サーバー実装やDB設計を依頼するものではない。

ルートパラメーターは未信頼入力として検証する。表のservice名は共通Repositoryの論理操作名。同じルートの行は同一画面内の機能を分担する。全行にloading/empty/error/forbidden/not-foundを実装する。再試行は回復可能なエラーだけに提供し、権限不足では許可された画面へ戻す。

## 画面・処理設計

| 設計ID / 要件 | ルート / 主コンポーネント | 取得・操作契約 | 入力・処理・検証 | 異常系と禁止事項 |
|---|---|---|---|---|
| DD-C01 / FR-C01 | `/customer` / `Overview` | `units.list, telemetry.summary, alerts.list` | 物件・期間フィルター。URLに保持し、許可された設備のみ集計 | データなしと0を区別。古い値に更新時刻を併記 |
| DD-C02 / FR-C02 | `/customer/properties` / `PropertyExplorer` | `properties.list, properties.save, properties.archive, spaces.list, spaces.save, spaces.archive, units.list` | 自組織の物件種別・名称、階層を作成/編集。循環・他物件親IDを拒否。設備能力・メーカー台帳の編集はHQ | 関連設備がある場所の削除は拒否。対象削除後のURLはnot-found。パンくずで上位へ戻る |
| DD-C03 / FR-C03 | `/customer/units/:id` / `UnitControl` | `units.get, commands.create, commands.get` | 能力から温度min/max/step、mode、fan候補を生成。確認後mutation。現在室温・設定値は別欄 | 拒否・期限切れ・失敗理由を表示。再照会後に手動再試行 |
| DD-C04 / FR-C04 | `/customer/automations` / `AutomationEditor` | `automations.list, automations.save, automations.simulate` | 曜日1件以上、開始/終了、timezone、設備、動作を必須。日跨ぎは明示チェック | 重複条件は警告し優先順位を表示。制限中は発火しても拒否理由 |
| DD-C05 / FR-C05 | `/customer/automations` / `AutomationEditor` | `consents.get, consents.update, automations.save, automations.simulate` | 条件は判別union。位置同意の目的を提示。デモは座標取得せず帰宅/外出イベント入力 | 拒否・利用不可は手動/時刻方式へ。利用履歴推定はデモと明示 |
| DD-C06 / FR-C06 | `/customer/energy` / `EnergyExplorer` | `energy.summary, baselines.list` | 期間開始<=終了、最大366日（仮）。通貨、料金版、比較期間、データ品質を表示 | 欠測は集計対象率を併記し、推計補完を実測として扱わない |
| DD-C07 / FR-C07 | `/customer/air-quality` / `AirQuality` | `telemetry.series, units.get, commands.create` | 指標と期間。換気要求はventilation能力を別確認 | センサーなしは未対応。送風を外気導入として扱わない |
| DD-C08 / FR-C08 | `/customer/alerts` / `AlertInbox` | `alerts.list, notifications.markRead, notifications.list` | 重要度・未読フィルター。通知IDとalertIdを分離 | 取得失敗時に正常サマリーを表示しない |
| DD-C09 / FR-C09 | `/customer/maintenance` / `MaintenanceRequest` | `jobs.list, jobs.create, jobs.get, jobs.cancel, jobs.addNote` | unitId、種別、症状10〜2000文字、未来の希望枠を必須（仮）。日時は希望であり確定予約ではない | 二重送信を抑止。希望枠不可なら候補選び直し。顧客による取消は未割当のみ |
| DD-C10 / FR-C10 | `/customer/payments` / `BillingOverview` | `contracts.list, invoices.list` | 契約ID・状態フィルター。金額は通貨最小単位で扱う | 閲覧範囲外は拒否。請求なしを滞納表示しない |
| DD-C11 / FR-C11 | `/customer/payments/:id` / `PaymentDemo` | `invoices.get, payments.simulate, notifications.preview` | 請求ID、デモ決済方法、確認。生カード番号等の項目を設けない | 実送信しない。処理中に完了表示せず、再試行は同じ冪等キー |
| DD-C12 / FR-C12 | `/customer/payments/:id` / `RestrictionNotice` | `restrictions.forInvoice, commands.get, inquiries.create` | 閲覧のみ。支払い・問い合わせ導線。対象設備と適用条件の版を表示 | オフラインでは保留。顧客の強制解除操作は提供しない |
| DD-C13 / FR-C13 | `/customer/energy/offsets` / `OffsetPreview` | `energy.summary, offsets.preview, offsets.simulate, offsets.list` | 希望量>0、対象期間、デモ確認。取引や認証を示す実証明番号は生成しない | 排出削減と償却済みクレジットを別表示。失敗時は重複申込しない |

## 実装の共通手順

1. セッションとスコープを確認し、ID・URLフィルターをschemaで検証する。
2. Query経由でモックサービスを呼び、画面モデルとして受け取る。
3. フォームはReact Hook Form＋共通schemaを利用し、能力・期間等の検証も適用する。
4. mutation直前に対象のversion・権限・現在状態を照合。高影響操作は対象と理由を確認する。
5. Repositoryで共有デモ状態を変更し、相関ID付きイベントを発行。該当Queryを無効化する。
6. 応答待ちは継続表示し、成功・拒否・失敗を分ける。フォーム失敗時は入力を保持する。

## テストへの引き渡し

設計ID DD-C番号ごとに同番号AT-Cの受入条件、上表の異常系、権限外の直接呼出しを検証する。テストデータと役割横断シナリオは[検証計画](../04-agentic-sdlc/verification.md)を正とする。設計の例示文字数等を変更する場合はschema、文書、境界値試験を同時更新する。

## 機能別詳細仕様（0.2.0）

表の入力はRHFで保持し、schema検証する。read-only値はQueryの単一sourceから表示する。共通の型・ページング・時間・エラーは[実装契約](implementation-contracts.md)を正とし、以下の個別条件を重ねる。視覚値は[UIUX](../03-uiux/UIUXSpecification.md) UX-04/08の参照準拠tokenとpatternを使用する。

### DD-C01 詳細

対象: FR-C01 / 主表示pattern: **UI-OVERVIEW**。画面サービス境界は`units.list, telemetry.summary, alerts.list`。

**初期表示と前提**: 自組織に利用可能な設備が登録されている。設備0件でも画面へ入れる。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| propertyId | ID/任意 | 初期は自組織の全物件 | 対象物件 |
| period | enum/必須 | today/7d/30d、初期today | 電力の集計期間 |
| unitId | ID/任意 | 選択物件に属する設備のみ | 温湿度の対象 |
| summary | 読取 | total/online/offline/unknown/alertCountとasOf | カード・表示時点 |

**処理手順**

1. 対象物件と期間を選ぶ → 稼働・室温・湿度・空気環境・電力・異常件数を確認 → 状態カードから該当設備一覧または詳細を開く。
2. 保存または操作直前に次の業務ガードを実行する: 異なる部屋の室温を代表温度として平均しない。温湿度は選択設備または部屋の測定点別、電力量は期間内、現在電力は最新観測時刻を表示する。
3. 閲覧だけでは業務状態を更新しない。絞込条件をURLに保存し、戻る操作で復元する。
4. 更新対象Query: `units / telemetry / alerts（購読イベント時のみ）`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 1台だけ未計測にした場合、正常台数へ加えず不明台数に計上する。別顧客の設備を検索条件へ指定しても集計しない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-C01-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-C02 詳細

対象: FR-C02 / 主表示pattern: **UI-LIST / UI-FORM**。画面サービス境界は`properties.list, properties.save, properties.archive, spaces.list, spaces.save, spaces.archive, units.list`。

**初期表示と前提**: 自組織の物件編集が許可されている。設備台帳の能力編集はHQに限定。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| property.name | 文字列/必須 | trim後1〜120文字 | 物件名 |
| property.kind | enum/必須 | home/office、初期未選択 | 分類 |
| space.name | 文字列/必須 | 1〜120文字 | 階・部屋名 |
| space.kind | enum/必須 | area/floor/room/space | 階層種別 |
| parentSpaceId | ID/null | rootはnull、同一物件、循環不可 | 親 |
| expectedVersion | 整数/更新時必須 | 取得版 | 競合検出 |

**処理手順**

1. 自宅/オフィスの物件を作成 → 必要なエリア・階・部屋を追加 → 場所を選択して設備を閲覧 → 名称・分類を編集する。
2. 保存または操作直前に次の業務ガードを実行する: 場所は単一の親に所属。親は同一物件内、自己参照と子孫への移動を禁止。設備や子場所がある場所はアーカイブ/削除を拒否し、先に移動先を案内する。
3. 作成IDと版を取得しツリーとパンくずを更新。設備の所属変更はHQ台帳へ誘導し、名称変更で設備IDは変えない。
4. 更新対象Query: `properties / spaces / units / customer summary`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 空の名称、121文字、循環、他組織の親IDを拒否。CONFLICT時は現在版を提示して入力を保持し、自動上書きしない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-C02-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-C03 詳細

対象: FR-C03 / 主表示pattern: **UI-DETAIL**。画面サービス境界は`units.get, commands.create, commands.get`。

**初期表示と前提**: 有効Membership、対象設備のcontrol能力、オンライン、契約制限内の要求。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| power | boolean/変更時 | 初期は確認済み値 | 電源 |
| targetTemperature | number/変更時 | capability.min/max/stepに一致 | 設定温度°C |
| mode / fanLevel | enum/変更時 | 対応候補のみ | モード・風量 |
| unitVersion | 整数/必須 | 取得時version | 現在状態の照合 |
| idempotencyKey | UUID/必須 | 確認送信時作成、同じ意思の再送で保持 | 重複防止 |

**処理手順**

1. 室温と確認済み設定を確認 → 電源/温度/モード/風量を編集 → 対象と変更内容を確認 → 要求受付・送信・機器応答を順に確認する。
2. 保存または操作直前に次の業務ガードを実行する: 1回の確認送信は1Actionとし、電源・温度・モード・風量をそれぞれ変更する。温度は機種のmin/max/step、モード・風量は列挙能力から選択。電源OFF要求でも機器の温度測定値を0にしない。pending中の同一設備への競合要求は操作不可にする。
3. Commandを1件作成し要求値を別表示。acknowledged後のみ確認済み設定を更新。失敗時も要求・理由を履歴に残す。
4. 更新対象Query: `commands / unit detail / telemetry summary / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 16〜30°C/1°C刻みのデモ能力なら15、31、24.5を拒否。offline、他顧客、制限違反、遅延応答でも成功扱いしない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-C03-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-C04 詳細

対象: FR-C04 / 主表示pattern: **UI-LIST / UI-FORM**。画面サービス境界は`automations.list, automations.save, automations.simulate`。

**初期表示と前提**: 対象設備がスケジュール制御可能。表示と保存のtimezoneを確定できる。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| name | 文字列/必須 | 1〜120文字 | ルール名 |
| unitIds | ID配列/必須 | 1〜50、利用範囲内 | 対象 |
| weekdays | 整数配列/必須 | ISO1〜7、重複不可 | 曜日 |
| startLocal / endLocal | HH:mm/必須 | 同時刻不可 | 時間帯 |
| endsNextDay | boolean/必須 | 初期false | 日跨ぎ |
| timezone | IANA文字列/必須 | 初期表示設定、存在するzone | 実行基準 |
| startAction / endAction | Action/必須 | 能力に適合 | 開始・終了動作 |
| enabled | boolean/必須 | 新規false、保存後明示有効化 | 運転開始の意思 |

**処理手順**

1. 曜日・運転時間帯・設定を入力 → 次回実行予定を確認 → 保存/停止 → デモ時計で開始・終了イベントを発火する。
2. 保存または操作直前に次の業務ガードを実行する: 曜日は開始日の曜日。終了<=開始は日跨ぎフラグがある場合のみ翌日。開始=終了は24時間運転と推定せず拒否。終了時の動作も必須で、黙ってOFFにしない。
3. Automationにtimezone、開始/終了action、enabledを保存。作成だけで即時コマンドを送らない。発火時に再認可する。
4. 更新対象Query: `automations / next-run preview / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 曜日0件、終了動作なし、曖昧/存在しない夏時間のローカル時刻を拒否。停止後の予約イベントでは要求を作らない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-C04-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-C05 詳細

対象: FR-C05 / 主表示pattern: **UI-FORM**。画面サービス境界は`consents.get, consents.update, automations.save, automations.simulate`。

**初期表示と前提**: 自動運転対象と条件種別が選択可能。位置依存の場合は目的別の同意が必要。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| condition.type | enum/必須 | occupancy/location/pattern/weather | 条件種別 |
| condition.value | 判別union/必須 | 在室有無、arrival/departure、予定時刻、天候比較 | 条件内容 |
| consentPurpose | enum/位置時必須 | location_automation | 利用目的 |
| granted | boolean/位置時必須 | 初期false | 同意 |
| action | Action/必須 | 設備能力内 | 実行内容 |
| priority | 整数/必須 | 0〜100、初期50 | 同順位調整 |
| simulationEvent | 合成event/任意 | isDemo=true | 動作確認 |

**処理手順**

1. 在室・帰宅/外出・生活パターン・天候の条件を選択 → 必要な同意を確認 → 動作を保存 → 模擬イベントで一致/不一致を確認する。
2. 保存または操作直前に次の業務ガードを実行する: 位置同意と一般利用同意を分離する。位置情報の取得は1Aでは合成イベントのみ。生活パターンはデモ推定で、個人の実行履歴を収集しない。条件欠測を条件成立としない。
3. 同意取消で該当条件のルールをdisabledにし、既発行Commandを取消したとは表示しない。手動制御は権限内で継続可能。
4. 更新対象Query: `consents / automations / simulation results / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 同意なしでlocation条件を有効化できない。取消後の帰宅イベントはコマンド0件。天候データ欠測ではスキップ理由を表示する。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-C05-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-C06 詳細

対象: FR-C06 / 主表示pattern: **UI-ANALYSIS**。画面サービス境界は`energy.summary, baselines.list`。

**初期表示と前提**: 自組織設備の期間データ。基準・料金がない場合も実績だけは閲覧可能。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| from / to | ISO日時/必須 | from<to、最大366日 | 集計区間 |
| unitIds | ID配列/必須 | 自組織、重複除去 | 設備集合 |
| baselineId | ID/任意 | 同境界の有効版 | 基準 |
| tariffVersion | 読取 | 算定結果に付随 | 仮単価の版 |
| coverage / totals | 読取 | kWh/amountMinor/差分、null許可 | 比較表示 |

**処理手順**

1. 期間・設備を選ぶ → 電力量と推定料金を見る → 基準条件を展開 → 比較とデータ品質を確認する。
2. 保存または操作直前に次の業務ガードを実行する: 最大366日。集計は[from,to)で、料金・排出係数の版を表示。基準と実績の設備集合・算定境界が違う場合は差分を算定しない。金額は最後に通貨桁へ丸める。
3. 条件変更はURLとQuery keyを更新。表示のみで契約料金や排出係数は変更しない。
4. 更新対象Query: `energy（検索条件変更時のみ）`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 基準100/実績80kWh/単価0.5MYRなら節約10MYR。基準0は率なし、実績120は増加20%、欠測はcoverage付き。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-C06-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-C07 詳細

対象: FR-C07 / 主表示pattern: **UI-ANALYSIS**。画面サービス境界は`telemetry.series, units.get, commands.create`。

**初期表示と前提**: 空気環境センサー有無と換気能力を取得できる。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| spaceId / unitId | ID/必須 | 有効な対象 | 測定場所 |
| metric | enum/必須 | co2/pm25/temperature/humidity | 表示指標 |
| period | enum/必須 | 1h/24h/7d、初期24h | 時系列 |
| value / quality / observedAt | 読取 | nullと0を区別 | 測定根拠 |
| ventilationAction | Action/任意 | ventilation能力時のみ | 換気要求 |

**処理手順**

1. 部屋・指標を選ぶ → 値・単位・品質を確認 → 換気/清掃案内を見る → 換気能力がある場合のみ確認付き要求へ進む。
2. 保存または操作直前に次の業務ガードを実行する: CO₂ ppm、PM2.5 µg/m³、温度°C、湿度%を別系列で表示。湿度0は欠測ではなく測定値0、nullは欠測。換気能力がなければ手動案内のみ。
3. 閲覧は変更なし。換気要求時は通常Command履歴を作成し、その応答だけで室内CO₂低下を推定しない。
4. 更新対象Query: `telemetry / commands（換気時） / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: CO₂欠測でもPM2.5値は表示可能。fan能力だけの設備でventilationコマンドを生成しない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-C07-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-C08 詳細

対象: FR-C08 / 主表示pattern: **UI-LIST**。画面サービス境界は`alerts.list, notifications.markRead, notifications.list`。

**初期表示と前提**: 顧客スコープ内の通知とアラートが取得可能。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| severity | enum/任意 | critical/warning/all、初期all | 絞込 |
| unreadOnly | boolean/必須 | 初期false | 未読 |
| notificationId / alertId | 読取 | 別ID、関連なし通知も可 | 参照 |
| readAt | 日時/null | 未読null | 既読状態 |

**処理手順**

1. 重要度・未読で絞る → 通知本文を開く → 設備根拠または保守依頼へ遷移 → 通知だけを既読にする。
2. 保存または操作直前に次の業務ガードを実行する: critical、warningは対応順位。normalは健康サマリーであり未対応アラートではない。清掃・交換の予定通知と異常発生通知をtypeで区別する。
3. 既読時刻をNotificationへ保存。Alert.statusは変えない。遷移先に戻るとフィルター・ページを復元。
4. 更新対象Query: `notifications / unread count`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 取得失敗時に異常0件表示をしない。通知先の設備が失効した場合は詳細を漏らさず利用不可を表示。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-C08-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-C09 詳細

対象: FR-C09 / 主表示pattern: **UI-LIST / UI-FORM / UI-DETAIL**。画面サービス境界は`jobs.list, jobs.create, jobs.get, jobs.cancel, jobs.addNote`。

**初期表示と前提**: 依頼対象が自組織の有効設備。RTOの有無は依頼可否に使わない。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| unitId | ID/必須 | 自設備、archived不可 | 対象 |
| type | enum/必須 | periodic/reactive/preventive | 保守分類 |
| symptom | 文字列/必須 | 10〜2000文字 | 症状 |
| requestedStart / requestedEnd | ISO日時/必須 | 現在より未来、start<end | 希望枠 |
| contactWindow | 文字列/任意 | 0〜200文字、実連絡先なし | 連絡可能時間 |
| cancelReason / note | 文字列/操作時必須 | 1〜1000 / 1〜2000文字 | 取消・調整依頼 |

**処理手順**

1. 設備・保守種別・症状・希望枠を入力 → 内容確認 → 受付番号取得 → 一覧から確定日程・進捗・報告履歴を確認する。
2. 保存または操作直前に次の業務ガードを実行する: 希望枠は確定予約ではない。requestedだけ顧客取消可能。assigned以降は調整依頼メモを送れ、顧客が作業予定や担当を直接変更しない。
3. 同じjobIdをHQ・業者・技術者が参照。未割当取消はcancelledと理由を記録。提出済報告は品質確認後に顧客へ公開する。
4. 更新対象Query: `jobs / job events / admin summary / notifications`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 症状9文字/2001文字、過去の希望枠、設備なしを拒否。通信失敗で症状を保持。二重送信で案件が増えない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-C09-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-C10 詳細

対象: FR-C10 / 主表示pattern: **UI-LIST / UI-DETAIL**。画面サービス境界は`contracts.list, invoices.list`。

**初期表示と前提**: 自組織の契約が0件以上。閲覧のために支払い操作は不要。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| contractId | ID/任意 | 自組織のみ | 契約選択 |
| status | enum/任意 | all/unpaid/processing/paid/overdue | 請求絞込 |
| amountMinor / currency | 読取 | 整数、通貨セット | 金額 |
| dueAt / paidAt | 読取 | UTC→表示zone | 期限・入金日 |
| planType / unitIds | 読取 | RTO/general/energy/environment | 適用契約 |

**処理手順**

1. 契約を選択 → 対象設備・期間・プランを確認 → 請求金額・期限・入金状態を見る → 請求詳細へ進む。
2. 保存または操作直前に次の業務ガードを実行する: 契約終了と設備利用不可は同義ではない。Invoice遅延は期限と未入金残額から導出し、payment processing中もpaidとはしない。
3. 閲覧のみ。非RTOは契約なし/一般保守と表示し、監視画面への戻り先を提供。
4. 更新対象Query: `contracts / invoices（閲覧のみ）`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 顧客bのinvoiceIdを顧客aが指定しても取得しない。paid請求に支払い開始ボタンを表示しない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-C10-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-C11 詳細

対象: FR-C11 / 主表示pattern: **UI-DETAIL / UI-FORM**。画面サービス境界は`invoices.get, payments.simulate, notifications.preview`。

**初期表示と前提**: 未払い請求、現在金額、模擬決済であることを確認。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| invoiceId | ID/必須 | 未払い請求 | 対象 |
| method | enum/必須 | demo_card/demo_instructions | 模擬手段 |
| channel | enum/プレビュー時 | email/whatsapp | 案内 |
| demoConfirmed | boolean/必須 | 初期false | 模擬処理確認 |
| outcome | enum/デモ制御側 | processing/confirmed/failed | 試験用イベント。通常フォームと分離 |

**処理手順**

1. 通知プレビューを確認 → 模擬カード/支払い手順を選択 → デモ決済を開始 → processing/失敗/入金確認イベントを確認する。
2. 保存または操作直前に次の業務ガードを実行する: カード番号・CVV・有効期限を入力させない。画面遷移だけで入金確認しない。processing中は追加の決済意思を受け付けない。
3. Paymentを作成し、確認イベント後にInvoiceをpaidへ。結果・参照IDを履歴表示し、制限は解除要求へ進めても応答前は解除済みにしない。
4. 更新対象Query: `payments / invoices / restrictions / notifications / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 二重クリック/同一参照の確認で二重計上しない。決済失敗で元の未入金状態と支払い再試行導線を維持する。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-C11-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-C12 詳細

対象: FR-C12 / 主表示pattern: **UI-DETAIL**。画面サービス境界は`restrictions.forInvoice, commands.get, inquiries.create`。

**初期表示と前提**: 自分の契約に関連する制限情報。制限なしも正常な空状態。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| invoiceId | route ID/必須 | 請求閲覧範囲内 | 関連制限検索 |
| restrictionId / rulesVersion | 読取 | 請求との関連を照合 | 根拠 |
| noticeAt / executeAfter / graceUntil | 読取 | 表示timezone付き | 予定 |
| perUnitStates | 読取配列 | 適用/解除のCommand別 | 反映状況 |
| message | 文字列/問い合わせ時必須 | 1〜2000文字 | 調整依頼 |

**処理手順**

1. 予告・理由・対象設備・予定日を確認 → 猶予/例外と解除条件を見る → 支払いまたは調整問い合わせへ進む。
2. 保存または操作直前に次の業務ガードを実行する: 制限予定、適用要求、設備別反映、解除要求、解除済みを別表示。部分反映は対象設備ごとの状態を表示し、集約badgeだけで隠さない。
3. 制限を変更せず、支払い/問い合わせへ関連IDを渡す。問い合わせはin-appデモ受付で実外部送信しない。
4. 更新対象Query: `inquiries / inquiry events（送信時のみ）`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: offline設備が1台残れば全体を解除済みとしない。顧客はURL/サービス呼出しでもoverrideできない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-C12-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-C13 詳細

対象: FR-C13 / 主表示pattern: **UI-ANALYSIS / UI-FORM**。画面サービス境界は`energy.summary, offsets.preview, offsets.simulate, offsets.list`。

**初期表示と前提**: 自組織の算定対象期間があり、オフセットは任意。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| period / unitIds | 必須 | 算定条件と同一 | 対象 |
| amountKg | number/必須 | 0より大、最大100000、小数3桁（仮） | 希望量kgCO₂e |
| quoteId / quoteVersion | ID/申込時必須 | 取得済み模擬見積 | 確認対象 |
| demoConfirmed | boolean/必須 | 初期false | 実購入ではない確認 |

**処理手順**

1. 推定排出・削減と条件を確認 → 希望量を入力 → 模擬見積 → 確認後にデモ申込 → 模擬記録を見る。
2. 保存または操作直前に次の業務ガードを実行する: 電力の削減結果から取引可能残高を生成しない。希望量は正数、係数欠落時は排出量算定不可。申込自体は外部購入を意味しない。
3. OffsetRecordはdemo_requestedとして保存。見積のみでは申込記録を作らない。証明欄はdemo接頭辞付き参照か未発行。
4. 更新対象Query: `offsets / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 希望量0/負値、見積失効、別顧客の算定結果を拒否。模擬償却と外部認証を同一文言にしない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-C13-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

