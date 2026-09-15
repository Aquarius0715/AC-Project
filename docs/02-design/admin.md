---
document_id: DD-A
version: 0.3.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# 管理者・HQ 詳細設計書

## 入力・責務

入力: [役割別要件](../01-requirements/admin.md)、[共通要件](../01-requirements/common.md)。必読: [共通詳細設計](common.md)、[UIUX仕様書](../03-uiux/UIUXSpecification.md)。以下はフロントエンドの項目・表示・モック動作の設計。画面上の登録・割当・入金・制限・監査はすべて共有モックメモリの状態遷移で、サーバー実装やDB設計を依頼するものではない。

ルートパラメーターは未信頼入力として検証する。表のservice名は共通Repositoryの論理操作名。同じルートの行は同一画面内の機能を分担する。全行にloading/empty/error/forbidden/not-foundを実装する。再試行は回復可能なエラーだけに提供し、権限不足では許可された画面へ戻す。

## 画面・処理設計

| 設計ID / 要件 | ルート / 主コンポーネント | 取得・操作契約 | 入力・処理・検証 | 異常系と禁止事項 |
|---|---|---|---|---|
| DD-A01 / FR-A01 | `/admin` / `AdminOverview` | `admin.summary` | 組織・期間。稼働率は対象台数と不明台数を併記 | 未計測を非稼働/正常に自動分類しない |
| DD-A02 / FR-A02 | `/admin/units` / `AssetRegistry` | `organizations.list, organizations.save, customers.list, customers.save, properties.save, spaces.save, units.save, units.archive, units.list, units.get` | 必須名称1〜120文字、tenantId、親ID、メーカー/型番、形式split、設置日 | 使用中の場所削除は関連設備移動まで拒否。アーカイブを優先 |
| DD-A03 / FR-A03 | `/admin/settings/access` / `AccessManager` | `members.list, members.save` | identity.manage、membershipId、role、scope、validFrom/Until。HQ権限は能力別 | 無期限の外部割当を拒否（デモ方針）。自分の高権限付与を自動許可しない |
| DD-A04 / FR-A04 | `/admin/devices` / `DeviceRegistry` | `capabilities.list, capabilities.save, devices.list` | 温度範囲min<=max、step>0、許可mode/fan、ventilationを明示 | 既存コマンドと不整合な能力変更は影響を表示。勝手に対応機能を推定しない |
| DD-A05 / FR-A05 | `/admin/alerts` / `AlertPolicyEditor` | `alerts.list, policies.save, notifications.preview` | 単位整合、上下限、継続時間>0、通知先必須。しきい値はデモ | 通知既読でアラートを解消しない。外部送信はプレビューのみ |
| DD-A06 / FR-A06 | `/admin/jobs` / `MaintenanceCoordinator` | `jobs.list, jobs.create, jobs.offer, jobs.assign, jobs.review, jobs.saveCost, jobs.hold, jobs.resume, jobs.cancel, plans.save, plans.generateNext, jobs.get` | 種別・対象・期日、社内/外注、費用は金額>=0+通貨。外注は受諾後に自社割当 | 辞退なら再委託。作業完了と異常解消は別。実業者報酬送金なし |
| DD-A07 / FR-A07 | `/admin/billing/contracts` / `ContractEditor` | `contracts.list, contracts.save` | 種別、customerId、unitIds、期間、料金。制限可否は契約属性 | 一般保守にRTO制限を誤適用しない。確定請求に影響する変更は新しい版 |
| DD-A08 / FR-A08 | `/admin/billing` / `BillingManager` | `invoices.list, invoices.create, payments.confirm, notifications.preview, inquiries.list, inquiries.answer` | billing.manage、契約、金額、期限、入金参照ID。手動確認理由必須 | 同じ入金参照を二重計上しない。入金確認をブラウザ遷移だけで行わない |
| DD-A09 / FR-A09 | `/admin/restrictions` / `RestrictionManager` | `restrictions.schedule, restrictions.execute, restrictions.release, commands.get, restrictions.list, restrictions.get` | restriction.manage、契約、設備、理由、予告期限、制限内容。実行前に再照会し条件確認 | 入金済/猶予/例外/非対応なら実行拒否。失敗・期限切れは未反映を保持 |
| DD-A10 / FR-A10 | `/admin/restrictions/:id` / `RestrictionException` | `restrictions.defer, restrictions.exempt, restrictions.cancel, restrictions.override, audit.list, restrictions.get` | override権限、理由、期限。実行要求中は取消競合を照会し、必要なら解除要求へ | 支払い状態を手動解除に合わせて改変しない。履歴削除不可 |
| DD-A11 / FR-A11 | `/admin/settings/automation` / `ControlPolicy` | `policies.save, automations.simulate` | 適用設備、優先順位、イベント、動作、停止条件。契約制限/安全能力を優先 | データ元欠測時は自動発火を停止。外部電力設備への実要求なし |
| DD-A12 / FR-A12 | `/admin/settings/air-quality` / `AirPolicy` | `policies.save, automations.simulate` | ppm、µg/m³、°C、%の単位を対応指標に固定 | 健康安全保証を表示しない。送風能力で換気命令を許可しない |
| DD-A13 / FR-A13 | `/admin/energy` / `EnergyAnalysis` | `energy.summary, baselines.list, baselines.save` | 基準期間/境界/モデル版、設備集合。期間重複・欠測条件を検証 | 基準なしは算定不可。10〜20%以上を保証しない |
| DD-A14 / FR-A14 | `/admin/mrv` / `MRVWorkspace` | `mrv.preview, mrv.saveDraft, mrv.recordReview, factors.list, factors.save, mrv.list, mrv.get` | 対象期間、設備、基準版、係数版、境界を必須。根拠一覧を表示 | 欠測時に推定を注記。外部検証済みと表示せず「デモ確認」を使用 |
| DD-A15 / FR-A15 | `/admin/offsets` / `OffsetRegistry` | `offsets.preview, offsets.simulate, offsets.list` | 希望量>0、制度/プロバイダーは未選定ラベル、demoフラグ必須 | 排出量をクレジット残高へ転記しない。実取引/実証明作成なし |
| DD-A16 / FR-A16 | `/admin/audit` / `AuditExplorer` | `audit.list, devices.events` | audit.read、期間、主体、対象、イベント種別。機密値はマスク | 拒否と成功を別結果。画面から削除・改変不可。デモの耐改ざん性は保証しない |

## 実装の共通手順

1. セッションとスコープを確認し、ID・URLフィルターをschemaで検証する。
2. Query経由でモックサービスを呼び、画面モデルとして受け取る。
3. フォームはReact Hook Form＋共通schemaを利用し、能力・期間等の検証も適用する。
4. mutation直前に対象のversion・権限・現在状態を照合。高影響操作は対象と理由を確認する。
5. Repositoryで共有デモ状態を変更し、相関ID付きイベントを発行。該当Queryを無効化する。
6. 応答待ちは継続表示し、成功・拒否・失敗を分ける。フォーム失敗時は入力を保持する。

## テストへの引き渡し

設計ID DD-A番号ごとに同番号AT-Aの受入条件、上表の異常系、権限外の直接呼出しを検証する。テストデータと役割横断シナリオは[検証計画](../04-agentic-sdlc/verification.md)を正とする。設計の例示文字数等を変更する場合はschema、文書、境界値試験を同時更新する。

## 機能別詳細仕様（0.2.0）

表の入力はRHFで保持し、schema検証する。read-only値はQueryの単一sourceから表示する。共通の型・ページング・時間・エラーは[実装契約](implementation-contracts.md)を正とし、以下の個別条件を重ねる。視覚値は[UIUX](../03-uiux/UIUXSpecification.md) UX-04/08の参照準拠tokenとpatternを使用する。

### DD-A01 詳細

対象: FR-A01 / 主表示pattern: **UI-OVERVIEW**。画面サービス境界は`admin.summary`。

**初期表示と前提**: HQ Membershipに対象テナントの集計閲覧権がある。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| customerId / propertyId | ID/任意 | 管理テナント内 | 対象 |
| from / to | 日時/必須 | 最大366日 | 期間 |
| counts / rates | 読取 | 分子/分母/unknownCount/asOf | 稼働 |
| amountsByCurrency | 読取配列 | 通貨ごと | 未入金額 |
| energySummary | 読取 | 期間・品質付き | 省エネ概要 |

**処理手順**

1. 顧客/拠点/期間を絞る → 顧客数・設備・稼働・異常・保守・請求・電力を確認 → KPIから同じ条件の一覧へ進む。
2. 保存または操作直前に次の業務ガードを実行する: 稼働率は最新状態を把握できる有効設備のうちpowerOnの割合とし、全設備数・不明数を必ず併記。顧客数はactiveの顧客組織数、請求遅延は未入金額ベース。
3. 閲覧のみ。ダッシュボードと遷移先一覧で期間・スコープ・定義が一致する。
4. 更新対象Query: `admin summary（関連イベント時）`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 不明設備を稼働率の分母へ黙って含めない。異なる通貨を合算せず通貨別、データ0件の率は算定不可。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A01-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A02 詳細

対象: FR-A02 / 主表示pattern: **UI-LIST / UI-FORM**。画面サービス境界は`organizations.list, organizations.save, customers.list, customers.save, properties.save, spaces.save, units.save, units.archive, units.list, units.get`。

**初期表示と前提**: 管理対象組織の台帳編集権。顧客・物件・設備の関係を解決できる。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| organization.name / customer.name | 文字列/必須 | 1〜120文字 | 顧客台帳 |
| property.kind / name | enum・文字列/必須 | home/office、1〜120文字 | 物件 |
| space.parentId / kind | ID・enum | 同物件・循環なし | 階層 |
| unit.modelId / spaceId | ID/必須 | 有効な型番・顧客内場所 | 設備 |
| unit.type / installedAt | enum・日付/必須 | split、未来の設置完了日不可 | 形式・設置日 |
| serviceScope | enum配列/必須 | 対象点検グループ | 保守範囲 |
| changeReason | 文字列/移設等必須 | 1〜1000文字 | 変更根拠 |
| property.address / accessInstructions | 文字列/任意 | 0〜500 / 0〜1000文字、架空値のみ | 現場住所・入場案内。未受諾業者には非公開 |

**処理手順**

1. 顧客組織作成 → 物件・場所階層作成 → split設備の型番と場所を登録 → 検索/編集/移設/アーカイブを行う。
2. 保存または操作直前に次の業務ガードを実行する: 新規設備はcustomerOrgId・spaceId・modelIdの整合必須。契約/案件/IoT紐付けがある設備は物理削除不可。別テナントへの移管は1A対象外。
3. 不変IDと版、変更前後・理由を保持。移設は現行の所属場所を更新し履歴に旧場所を残す。
4. 更新対象Query: `organizations / customers / properties / spaces / units / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 他顧客の部屋、循環階層、存在しないmodelId、使用中設備の削除を拒否。inactive顧客に新規設備を登録しない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A02-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A03 詳細

対象: FR-A03 / 主表示pattern: **UI-LIST / UI-FORM**。画面サービス境界は`members.list, members.save`。

**初期表示と前提**: identity.manage。変更対象の現在role・scope・有効期間を取得済み。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| userId / organizationId | ID/必須 | 有効ユーザーと所属 | 対象 |
| role | enum/必須 | client/contractor/technician/admin | 役割 |
| employment | enum/技術者必須 | internal/external | 勤務区分 |
| permissions | enum配列/必須 | role許可集合の範囲 | 能力 |
| scopeIds | ID配列/必須 | 自テナント・委託範囲 | 対象 |
| validFrom / validUntil | 日時/条件必須 | 外部はuntil必須 | 有効期間 |
| reason | 文字列/必須 | 1〜1000文字 | 変更理由 |

**処理手順**

1. ユーザー/所属を選ぶ → 4役割と技術者の社内/外部を指定 → スコープ/期間/個別能力を設定 → 差分を確認して保存する。
2. 保存または操作直前に次の業務ガードを実行する: roleと能力を分ける。管理者でもrestriction.manage/overrideを自動付与しない。外部技術者に終了なしの設備アクセスを付与しない。自分への高権限付与は別HQ権限管理者の操作に限定。
3. Membership版とscopeVersionを更新。旧セッションの閲覧キャッシュを破棄し、次変更要求は新権限で判定。
4. 更新対象Query: `members / session scope / all affected query caches / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 別テナントscope、外部で期限なし、validFrom>=validUntil、自分へのoverride追加を拒否。失効操作後の古い画面からの保存を拒否。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A03-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A04 詳細

対象: FR-A04 / 主表示pattern: **UI-LIST / UI-FORM**。画面サービス境界は`capabilities.list, capabilities.save, devices.list`。

**初期表示と前提**: device.manageまたは型番管理能力を持つ。能力値はデモ台帳として管理。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| manufacturer / model | 文字列/必須 | 各1〜120文字、組合せ一意 | 型番 |
| control / ventilation | boolean/必須 | 初期false | 能力 |
| min / max / step | number/温度対応時必須 | min<=max、step>0、整合 | 温度°C |
| modes / fanLevels | enum配列/対応時必須 | 重複なし | 対応候補 |
| sensors | 配列/任意 | metric/unit/staleAfterSeconds | 測定能力 |
| firmwareCandidates | 版配列/任意 | 対応デモ版のみ | FW |
| changeReason | 文字列/更新時必須 | 1〜1000文字 | 版変更根拠 |

**処理手順**

1. 型番を登録 → 温度/モード/風量/センサー/換気/FW候補を設定 → 影響設備を確認 → 新しい能力版を保存する。
2. 保存または操作直前に次の業務ガードを実行する: 能力の未確認はfalse/unknown。対応modeを製品カテゴリから推定しない。既存automationが新能力に違反する場合は該当ルールを停止し理由を表示する。
3. Capability版を更新してUnit操作候補へ反映。未完了Commandの要求内容は履歴として保持し、勝手に新能力へ書換えない。
4. 更新対象Query: `capabilities / units / devices / automations / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: min>max、step<=0、空mode集合でmode制御trueを拒否。非対応FW版を候補へ紛れ込ませない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A04-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A05 詳細

対象: FR-A05 / 主表示pattern: **UI-LIST / UI-FORM**。画面サービス境界は`alerts.list, policies.save, notifications.preview`。

**初期表示と前提**: alert.policy.manageと通知先の閲覧権。指標単位・対象設備を取得済み。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| unitIds / metric | ID配列・enum/必須 | 能力内 | 対象 |
| operator / threshold | enum・number/必須 | gt/gte/lt/lte、有限値 | 発火条件 |
| durationSeconds | 整数/必須 | 1〜86400 | 継続 |
| recoveryThreshold | number/必須 | 方向に応じたヒステリシス | 解除候補 |
| severity | enum/必須 | warning/critical | 重要度 |
| recipientIds / channels | 配列/必須 | 各1件以上、inApp/email/whatsapp | 通知 |
| escalateAfterMinutes / cooldownMinutes | 整数/必須 | 1〜1440 / 1〜1440 | 未対応・重複抑制 |

**処理手順**

1. 対象・指標・比較条件・継続時間を設定 → 通知先・手段・エスカレーション時間を指定 → 保存 → 合成値で発火/解除条件を確認する。
2. 保存または操作直前に次の業務ガードを実行する: 単位はmetricに従い変更不可。missing/staleは測定閾値の正常判定に使わず通信/品質通知へ。繰返し通知はcooldownで抑制し、重大度変化は新しい通知理由として保持する。
3. Policy版を保存。発火でAlertとNotificationプレビューを作成。通知既読とAlert確認を分離。
4. 更新対象Query: `policies / alerts / notifications / admin summary / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 通知先0件、継続時間0、解除閾値と比較方向の不整合を拒否。閾値到達直前/一致/継続時間境界を検証する。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A05-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A06 詳細

対象: FR-A06 / 主表示pattern: **UI-LIST / UI-DETAIL / UI-FORM**。画面サービス境界は`jobs.list, jobs.create, jobs.offer, jobs.assign, jobs.review, jobs.saveCost, jobs.hold, jobs.resume, jobs.cancel, plans.save, plans.generateNext, jobs.get`。

**初期表示と前提**: job.manage。対象設備と社内/外注の選択肢を取得済み。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| unitId / type / dueAt | 必須 | split、periodic/reactive/preventive、期限 | 依頼 |
| deliveryMode | enum/必須 | internal/contractor | 実施区分 |
| assigneeId / contractorOrgId | ID/条件必須 | 区分に一致 | 委託・担当 |
| recurrence | 構造体/定期時任意 | monthly、interval1〜12、次回日、初回のみ生成 | 計画 |
| costLines | 配列/任意 | kind=estimate/actual、amountMinor>=0、currency、description | 費用 |
| reviewDecision / reason | 条件必須 | accept/return、中断/取消も理由必須 | 品質・例外 |

**処理手順**

1. 保守種別・設備・期限を登録 → 社内へ直接割当または業者へ委託 → 日程と進捗を追う → 品質確認と費用実績を記録する。
2. 保存または操作直前に次の業務ガードを実行する: 社内はHQが担当・日程を確定、外注はoffer受諾後に自社割当。定期計画の繰返し設定は次回生成予定を表示し、同じ計画/回の案件は1件。
3. Job/Assignment/Offer/費用明細・品質履歴を同じjobIdに集約。完了してもAlert解消は別判断。
4. 更新対象Query: `jobs / plans / offers / assignments / costs / notifications / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 辞退後の再委託、確定予定重複、期限超過、品質差戻しを検証。見積通貨と実績通貨が違う場合は変換せず別集計。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A06-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A07 詳細

対象: FR-A07 / 主表示pattern: **UI-LIST / UI-FORM**。画面サービス境界は`contracts.list, contracts.save`。

**初期表示と前提**: contract.manage。顧客と紐付け設備が同じテナント内。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| customerId / unitIds | 必須 | 顧客内の有効設備 | 契約対象 |
| planType | enum/必須 | rto/general/energy/environment | プラン |
| startAt / endAt | 日時/必須 | start<end | 期間 |
| priceMinor / currency | 整数・enum/必須 | >=0、デモMYR初期 | 料金 |
| restrictionEligible | boolean/必須 | 初期false、rtoのみtrue可 | 制限可否 |
| rulesVersion | ID/制限可なら必須 | 承認済み本番ルールではなくdemo版 | 適用条件 |

**処理手順**

1. プラン種別・期間・料金・設備を設定 → RTO制限可否とルール版を指定 → 内容確認 → 契約を保存/改版する。
2. 保存または操作直前に次の業務ガードを実行する: 一般保守はrestrictionEligible=false。RTOでも明示的に制限可能とした契約だけ対象。既に発行した請求は契約改版で遡及変更しない。
3. 契約版を保持し、新請求には新しい版、過去請求には元の版を参照させる。契約終了を実機停止として扱わない。
4. 更新対象Query: `contracts / customer payments / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 他顧客設備、期間逆転、負料金、一般保守の制限有効化を拒否。契約なし設備の監視・保守を妨げない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A07-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A08 詳細

対象: FR-A08 / 主表示pattern: **UI-LIST / UI-FORM**。画面サービス境界は`invoices.list, invoices.create, payments.confirm, notifications.preview, inquiries.list, inquiries.answer`。

**初期表示と前提**: billing.manage。契約・請求・模擬決済の対象を照合できる。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| contractId / period | 必須 | 契約期間内の請求期間 | 請求 |
| amountMinor / currency | 必須 | 正整数、契約通貨と一致 | 金額 |
| dueAt | 日時/必須 | 請求作成時より未来（seed遅延は専用） | 期限 |
| paymentReference | 文字列/確認必須 | 1〜128文字、テナント内一意 | 入金参照 |
| confirmedAmountMinor | 整数/確認必須 | 請求全額と一致 | 確認額 |
| reason | 文字列/手動確認必須 | 1〜1000文字 | 確認根拠 |
| channel | enum/督促必須 | email/whatsapp/inApp、preview | 案内 |
| inquiryId / reply | ID・文字列/問い合わせ回答時必須 | 管理範囲内、返信1〜2000文字 | 顧客へのin-app回答 |

**処理手順**

1. 契約版から請求作成 → 期限/状態で絞込 → 模擬決済または権限付き入金確認 → 督促プレビューと顧客表示を確認する。
2. 保存または操作直前に次の業務ガードを実行する: 1Aは全額入金のみ。invoiceId+paymentReferenceの二重確認は同じ結果を返す。督促対象は未入金・期限超過で、例外や係争の有無を表示する。
3. Payment confirmed、Invoice paid、監査を記録。関連Restrictionがある場合は解除要求を生成/案内し、実機応答待ちを維持。
4. 更新対象Query: `invoices / payments / restrictions / notifications / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 金額違い、通貨違い、同参照の別請求流用、paidの再請求を拒否。入金済みになった直後の督促実行を再確認して止める。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A08-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A09 詳細

対象: FR-A09 / 主表示pattern: **UI-LIST / UI-DETAIL / UI-FORM**。画面サービス境界は`restrictions.schedule, restrictions.execute, restrictions.release, commands.get, restrictions.list, restrictions.get`。

**初期表示と前提**: restriction.manage、制限可能RTO契約、未入金、対象設備の能力確認。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| contractId / unitIds | 必須 | 制限可能契約に含まれる設備 | 対象 |
| policy.kind | enum/必須 | temperature_limit/power_off | 制限方式 |
| policy.minimumCoolingSetpoint | number/温度制限時必須 | 機器min/max/step内、低い温度設定を禁止 | 冷房制限 |
| noticeAt / executeAfter | 日時/必須 | executeAfter>=noticeAt+demo通知期間 | 予告 |
| reason / rulesVersion | 文字列・ID/必須 | 1〜1000文字、demo版 | 根拠 |
| expectedVersion | 整数/実行必須 | 現在版 | 競合 |

**処理手順**

1. 予告の理由・対象・内容・時刻を入力 → 顧客プレビュー → 開始時に請求/猶予/例外を再照合 → 設備別適用要求 → 入金後解除要求と応答を追う。
2. 保存または操作直前に次の業務ガードを実行する: UIで期限到来しても自動実停止しない。1AはHQの明示確認で模擬要求。対象全台が応答するまでapplied/releasedにしない。停止と温度制限は別policy。
3. Restrictionと設備別Commandを作成。入金確認後はscheduledならcancelled、requested/appliedならrelease_requested。
4. 更新対象Query: `restrictions / commands / units / customer billing / notifications / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 開始直前の入金、猶予、例外、未通知、非対応機器を再検証。一部offline、遅い適用応答、解除失敗を表示し状態を逆行させない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A09-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A10 詳細

対象: FR-A10 / 主表示pattern: **UI-DETAIL / UI-FORM / UI-TIMELINE**。画面サービス境界は`restrictions.defer, restrictions.exempt, restrictions.cancel, restrictions.override, audit.list, restrictions.get`。

**初期表示と前提**: 猶予/例外にはrestriction.manage、手動解除にはrestriction.override。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| restrictionId | ID/必須 | 管理範囲内 | 対象 |
| action | enum/必須 | defer/exempt/cancel/override_release | 例外操作 |
| until | ISO日時/猶予・例外必須 | 現在より未来 | 有効期限 |
| reason | 文字列/必須 | 1〜2000文字 | 例外根拠 |
| expectedVersion | 整数/必須 | 再照会後の版 | 競合 |

**処理手順**

1. 対象の現在状態と機器反映を照会 → 猶予・例外・取消・手動解除を選ぶ → 理由/期限/影響を確認 → 保存 → 設備別結果を追う。
2. 保存または操作直前に次の業務ガードを実行する: 適用前の取消はcancelled。適用要求後は反映が不明でも解除フローへ。例外・猶予の期限が切れても自動再適用せず条件再確認を要する。
3. 例外・猶予の変更前後と期限、解除の理由・主体を記録。手動解除でInvoiceの未入金を解消しない。
4. 更新対象Query: `restrictions / commands / audit / notifications`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: overrideなしHQ、空理由、過去の猶予期限を拒否。requested中の取消でも実際の適用可能性をゼロと推定しない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A10-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A11 詳細

対象: FR-A11 / 主表示pattern: **UI-FORM**。画面サービス境界は`policies.save, automations.simulate`。

**初期表示と前提**: automation.policy.manage。対象設備と制御能力を取得済み。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| name / unitIds | 必須 | 1〜120文字、管理対象 | 方針 |
| condition.type | enum/必須 | occupancy/tariff/peak/solar/battery | 条件種別 |
| condition.params | 判別union/必須 | 料金閾値・時間帯・出力等、単位付き | 判定値 |
| action | Action/必須 | 能力・制限内 | 動作 |
| priority | 整数/必須 | 0〜100、大きい値優先 | 優先 |
| enabled | boolean/必須 | 初期false | 有効化 |

**処理手順**

1. 在室・料金・ピーク・太陽光/蓄電池の条件を選ぶ → 動作と優先順位を設定 → 競合プレビュー → 合成イベントで評価する。
2. 保存または操作直前に次の業務ガードを実行する: 能力・有効制限→HQ方針→顧客ルールの優先。各層で数値priorityの大きい方、同値はID昇順。データ欠測や失効時は実行を見送り理由を表示。
3. 方針版を保存。simulationは採用/抑止ルールと理由を返し、発火する場合も共通Commandを経る。
4. 更新対象Query: `policies / automations / simulation results / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 顧客ルールよりHQ方針を優先、同順位の結果が毎回同じ、太陽光データ欠測で勝手に実行しない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A11-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A12 詳細

対象: FR-A12 / 主表示pattern: **UI-FORM / UI-ANALYSIS**。画面サービス境界は`policies.save, automations.simulate`。

**初期表示と前提**: 環境policy管理権限。機器に対象metricと換気能力の定義がある。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| unitIds / metric | 必須 | 対応センサーあり | 対象 |
| threshold / recoveryThreshold | number/必須 | 指標固定単位 | 発火・回復 |
| durationSeconds | 整数/必須 | 1〜86400 | 継続 |
| responseMode | enum/必須 | notify_only/notify_and_ventilate | 対応 |
| recipientIds | ID配列/必須 | 有効宛先1件以上 | 通知 |

**処理手順**

1. 指標・しきい値・継続/回復条件を設定 → 通知と換気要求の有無を選ぶ → 対象の能力確認 → 模擬評価する。
2. 保存または操作直前に次の業務ガードを実行する: 換気自動要求はventilation=trueの対象だけ。それ以外は通知onlyにし、保存前に対象別の実行内容を表示する。
3. 環境policy版・通知プレビューを保存。要求を作る場合は機器応答を追い、室内環境の改善は後続測定で確認する。
4. 更新対象Query: `policies / alerts / commands / notifications / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: ppmとµg/m³の閾値を混用しない。未計測は正常/回復判定に使わない。換気非対応機器へ送風コマンドを代用しない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A12-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A13 詳細

対象: FR-A13 / 主表示pattern: **UI-ANALYSIS / UI-FORM**。画面サービス境界は`energy.summary, baselines.list, baselines.save`。

**初期表示と前提**: energy.manage、管理範囲の期間データ。基準モデルの根拠を入力可能。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| unitIds / period | 必須 | 同じ比較集合、最大366日 | 対象 |
| method | enum/必須 | demo_fixed/demo_period_comparison | 基準方法 |
| baselineKWh | number/必須 | >=0、仮データ表記 | 基準値 |
| boundary / assumptions | 文字列/必須 | 1〜2000文字 | 算定境界・条件 |
| version / source | 必須 | 不変版・demo出典 | 根拠 |

**処理手順**

1. 基準対象・期間・方法を指定 → 比較条件を確認 → 実績と差を表示 → 品質・根拠を展開する。
2. 保存または操作直前に次の業務ガードを実行する: 基準は設備集合・境界・期間条件・モデル版を保持。気象等の補正モデル未実装なら補正済みと表示しない。負の削減を0へ丸めない。
3. 基準保存時に新しい版。既存MRVレポートの基準版を遡及変更しない。
4. 更新対象Query: `baselines / energy / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 基準0、実績欠測、異なる設備集合、異なる算定境界を検証。100/80なら20kWh・20%、100/120なら-20kWh・-20%。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A13-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A14 詳細

対象: FR-A14 / 主表示pattern: **UI-ANALYSIS / UI-FORM / UI-DETAIL**。画面サービス境界は`mrv.preview, mrv.saveDraft, mrv.recordReview, factors.list, factors.save, mrv.list, mrv.get`。

**初期表示と前提**: mrv.manage。対象期間、設備、基準版、係数版、境界が選択済み。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| unitIds / from / to | 必須 | 管理範囲内、from<to | 対象 |
| baselineId / factorId / boundary | 必須 | 版付き、不変参照 | 算定条件 |
| factor.region / year / kgCO2ePerKWh / source | 必須 | 年度整数、係数>=0、出典demo明示 | 係数 |
| reviewComment | 文字列/確認時必須 | 1〜2000文字 | デモ確認 |
| reportVersion | 整数/更新必須 | 現在版 | 競合 |

**処理手順**

1. 算定条件を指定 → 測定/品質/計算結果をプレビュー → 根拠を確認 → ドラフト保存 → デモ確認履歴・報告プレビューを表示する。
2. 保存または操作直前に次の業務ガードを実行する: 係数の地域・年度・単位・出典を必須。確認履歴はdemo_reviewedで外部認証と区別。入力版が変われば新しい報告版を作り、旧結果は保持する。
3. MRVReportに係数/基準のスナップショット参照と算定結果・品質・reviewHistoryを保存。プレビューのみでは確定記録を作らない。
4. 更新対象Query: `mrv / review history / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 係数欠落、単位不整合、coverage0、同一報告版への重複確認を検証。未検証値が「認証済み」にならない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A14-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A15 詳細

対象: FR-A15 / 主表示pattern: **UI-LIST / UI-FORM / UI-DETAIL**。画面サービス境界は`offsets.preview, offsets.simulate, offsets.list`。

**初期表示と前提**: mrv.manageまたはoffset.manage。模擬取引のみであることを表示。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| customerId / purpose | 必須 | 管理範囲、1〜1000文字 | 申込者・目的 |
| amountKg | number/必須 | 0より大、<=100000、小数3桁 | 希望量 |
| quoteId / version | 見積後必須 | 有効なdemo見積 | 確認対象 |
| provider / scheme | 読取 | unselected/demo | 未選定 |
| demoConfirmed | boolean/必須 | 初期false | 実取引なし |
| demoCertificateRef | 読取 | DEMO-接頭辞、償却後のみ | 模擬証明 |

**処理手順**

1. 希望量と目的を指定 → 模擬見積 → 模擬申込 → デモ購入確認 → デモ償却 → 証明情報プレビューを閲覧する。
2. 保存または操作直前に次の業務ガードを実行する: 購入申込と購入確認と償却を別イベントにする。1Aは1記録の全量償却のみ。自社排出削減計算を購入済み残高に加算しない。
3. quoted→demo_requested→demo_purchased→demo_retiredを履歴に保存。証明参照はDEMO-接頭辞で実証明として出力しない。
4. 更新対象Query: `offsets / offset events / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 未購入の償却、二重償却、希望量0、見積失効、別テナントの記録操作を拒否。失敗はfailedと直前段階を保持。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A15-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-A16 詳細

対象: FR-A16 / 主表示pattern: **UI-TIMELINE / UI-DETAIL**。画面サービス境界は`audit.list, devices.events`。

**初期表示と前提**: audit.read。管理テナントを超えず、機微情報を含まない監査projection。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| from / to | ISO日時/必須 | 最大366日 | 検索期間 |
| actorId / targetId / correlationId | ID/任意 | 管理スコープ内 | 絞込 |
| result | enum/任意 | success/denied/failed/pending | 結果 |
| cursor / limit | 文字列・整数 | 25初期、最大100 | ページ |
| before / after / reason | 読取 | マスク済み | 差分・根拠 |

**処理手順**

1. 期間・主体・対象・結果・相関IDを検索 → 履歴詳細を開く → 関連Command/Job/Restrictionの状態と照合する。
2. 保存または操作直前に次の業務ガードを実行する: 監査は成功/拒否/失敗を分離し、before/afterは秘密・連絡先をマスク。フロント上は追加のみで編集/削除を提供しない。ブラウザ内デモに耐改ざん保証はしない。
3. 閲覧のみ。条件をURLへ保持するが機密本文をURLへ入れない。
4. 更新対象Query: `なし（監査参照）`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 他テナント相関ID、削除API相当の呼出し、期間逆転を拒否。実行主体が役割切替後の別人に書換わらない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-A16-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

