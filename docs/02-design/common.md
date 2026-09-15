---
document_id: DD-COMMON
version: 0.5.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# 共通詳細設計書

入力: [共通要件](../01-requirements/common.md)、[PrepareDocument](../00-prepare/PrepareDocument.md)。実装粒度の入出力・エラー・デモ時間値は[フロントエンド入出力契約](implementation-contracts.md)、全論理操作は[操作カタログ](operation-catalog.csv)を併読する。本書が定義するのはブラウザ側の画面モデル・モックサービス・表示状態だけ。APIパス、DB、認証サーバー、バックエンド業務処理は設計対象外。

本書の業務目的は[企業要件原文](../00-prepare/sources/company-requirements-original.txt)から整理したBIZ項目と[共通要件](../01-requirements/common.md)に基づきます。型、Repository、キャッシュ、権限ガード、モック状態は、その目的をフロントエンドで実現する設計提案です。将来のAPI導入時は表示用モデルへの変換をadapterへ集約します。サーバーの認証・DB・通信契約は本書では定義しません。

## 1. 構成と責任

```text
src/
  app/                router, providers, composition-root, route-guards
  features/           units, jobs, billing, restrictions, energy, devices...
    <feature>/        pages, components, queries, forms, schemas
  domain/             entities, policies, transitions, repository-contracts
  infrastructure/
    mock/             repository, fixtures, scenario-clock, event-bus
    adapters/         外部データへの差替え口（今回はinterfaceのみ）
  shared/
    ui/               shadcnベースのUI primitives
    components/       StatusBadge, MetricCard等の共通業務表示
    styles/           tokens.css
    config/           demo-settings, locale-settings
    i18n/             en, ja
tests/                unit, component, contract, e2e
```

依存方向は`page → feature hook → Repository interface → injected adapter`。domainはReact・HTTP・モックに依存しない。ページからfetch、mock seed、localStorageを直接利用しない。composition-rootだけがadapterを選択する。

PROPOSED構成はTypeScript strict、React、Vite、React Router。SSR要件がないためSPAとする。ルーティングのディープリンクは将来ホストのSPAフォールバック設定が必要。依存版は実装開始時に互換性を検証しlockfileに固定する。アプリ起動コマンドは未作成であり、本書では実行済みとしない。

## 2. 共通ルートと画面状態

| ルート | 責務 |
|---|---|
| /login | 架空アカウント・4役割選択。実認証ではない旨を表示 |
| /forgot-password | メール形式検証→存在を漏らさない案内。送信プレビューのみ |
| /settings/preferences | 言語、表示タイムゾーン、デモ通貨、同意設定 |
| /notifications | スコープ内通知一覧と既読。業務状態とは別 |
| /demo | シナリオ切替、失敗イベント、デモ時計、リセット。デモ限定 |
| /forbidden、未定義ルート | アクセス不可、not-found。許可範囲のホームへ戻す |

共通ヘッダーに言語・通知・音声/テキスト・サインアウト。デモ役割切替は専用メニューで通常業務のユーザー変更と分離する。

loadingは骨格表示、emptyは説明と許可された次アクション、errorは再試行、offlineは更新時刻と操作不可理由、staleは古い値の注記。閲覧不可をemptyで誤魔化さない。404/403は機密存在を漏らさない文言に揃える。

## 3. フロントエンドの画面・モック用モデル

以下はDBのテーブル定義ではない。「保存」「履歴」はすべて同一タブ内の共有モックメモリへの保持を指す。

共通: `id: string`、`tenantId: string`、時刻はISO 8601 UTC、可変リソースは`version: number`。デモidも不変。nullと未登録はschemaで意味を分け、未知enumはunsupportedとして安全に扱う。

| エンティティ | 主なフィールドと関係 |
|---|---|
| User / Membership | userId, organizationId, role(client/contractor/technician/admin), employment(internal/external/null), permissions[], scopeIds[], validFrom, validUntil。Userに単一roleを直書きしない |
| Organization / Customer | name, kind(customer/contractor/operator), status / organizationId, serviceProfile。顧客IDとUser IDを分離し、作成時も管理テナント内に限定 |
| ContractorOrganization / Assignment | contractorOrgId, jobId, technicianMembershipId, delegatedScope, validFrom/Until, status。委託元tenantと受託会社IDを分離 |
| Property / Space | customerOrgId, kind(home/office), name / propertyId, parentSpaceId, kind(area/floor/room/space)。親階層の循環禁止 |
| ACUnit / Capability | spaceId, manufacturer, model, type(split), installedAt, serviceScope / temperature(min,max,step), modes[], fanLevels[], ventilation, control, sensors[] |
| Device / Sensor | unitId, serial, connection(online/offline/unknown), lastSeenAt, firmwareVersion / deviceId, metric, unit, calibrationAt, staleAfterSeconds |
| Telemetry | unitId, sensorId?, metric, value:number\|null, unit, observedAt, receivedAt, origin(measured/estimated/inspection), quality(valid/missing/stale/suspect), isDemo |
| Command | unitId, actorId, action:UnitAction, status, requestedAt, sentAt?, acknowledgedAt?, expiresAt, failureCode?, idempotencyKey, expectedVersion, correlationId |
| Alert | unitId, type, severity(critical/warning/normal), status, evidenceIds[], detectedAt, acknowledgedAt?, resolvedAt?, resolutionReason?。正常は健康サマリーにも使い、openアラートを正常へ自動変換しない |
| MaintenanceJob | unitId, alertIds[], type(periodic/reactive/preventive), status, contractorOrgId?, assignmentId?, requestedSlot, scheduledSlot?, dueAt, reportVersion?, costs[] |
| WorkReport / InspectionItem / Attachment | jobId, authorId, version, items[], measurements[], replacementParts[], workText, nextAction, submittedAt? / componentGroup, componentKey, result, reason?, evidenceIds[] / name, mime, size, previewUrl, status |
| Contract / Invoice / Payment | customerOrgId, unitIds[], planType, period, rulesVersion / contractId, amountMinor, currency, dueAt, status / invoiceId, amountMinor, status, externalRef?, confirmedAt? |
| Restriction | contractId, unitIds[], rulesVersion, noticeAt, executeAfter, reason, policy, state, applyCommandIds[], releaseCommandIds[], exception?, graceUntil? |
| Automation / Consent | unitIds[], condition(discriminated union), action, priority, timezone, enabled / purpose, granted, grantedAt?, revokedAt? |
| Notification / AuditEvent | recipientId, channel(inApp/email/whatsapp), templateKey, params, readAt?, deliveryState(preview/simulated/failed) / actorId, action, targetId, before?, after?, reason?, result, correlationId, occurredAt |
| EnergyBaseline / EmissionFactor | unitIds[], period, method, version, kWh, boundary / region, year, kgCO2ePerKWh, source, version, isDemo |
| MRVReport / OffsetRecord | period, baselineId, factorId, boundary, coverage, totals, evidenceIds[], reviewHistory[], status(draft/demo_reviewed) / amountKg, state(quoted/demo_requested/demo_purchased/demo_retired/failed), demoCertificateRef?, isDemo |

Telemetryの`isDemo`は測定区分と独立。デモの「実測」も合成値であると分かるよう表示する。連絡先・写真は架空のみ。ブラウザ画像object URLは削除・リセット・サインアウト時にrevokeする。

## 4. フロントエンドのデータサービス境界

`page → feature hook → Repository interface → mock adapter`で分離する。ここでいうRepositoryはフロントエンドから呼ぶ非同期サービスの抽象であり、DBアクセス層ではない。今回作成するのはinterfaceと共有メモリのmock adapter。

```ts
interface CommandRepository {
  create(context: DemoViewContext, input: {
    unitId: string;
    action: UnitAction;
    expectedUnitVersion: number;
  }, options: DemoWriteOptions): Promise<Command>;
  get(context: DemoViewContext, commandId: string,
      signal?: AbortSignal): Promise<Command>;
}
```

`DemoViewContext`は選択中の架空ユーザー・役割・閲覧範囲を表す。`DemoWriteOptions`は同じデモ操作の重複防止用キーと変更前の版を持つ。これらは本番認証やサーバーの認可方式を規定しない。具体的なフィールドは[フロントエンド入出力契約](implementation-contracts.md)を参照する。

[操作カタログ](operation-catalog.csv)にはUIが必要とする109のローカルサービス操作、その入力・戻り値・参照画面をまとめる。URL、HTTP method、DBテーブル、サーバートランザクションは定義しない。

モックの操作結果は成功・受付中・失敗・競合・閲覧不可として返す。画面はDomainErrorに応じて表示・入力保持・再読込を決める。遅延した古い応答は操作IDと表示世代で除外し、役割切替前の値を描画しない。

将来APIが用意された場合も、画面が使うinterfaceを維持し、新しいadapterで外部レスポンスを画面モデルへ変換する。実際のAPI仕様・認証方式・通信契約は今回決定しない。

## 5. 状態遷移と整合性

### コマンド

| 現状態 | イベント・ガード | 次状態 / 表示 |
|---|---|---|
| 未作成 | 権限・能力・契約制限・onlineを確認し送信 | requested / 要求受付。確認値は変更しない |
| requested | 配信イベント | sent / 機器応答待ち |
| requested / sent | 明示失敗 / 応答期限超過 | failed / expired。要求値を確認値へコピーしない |
| sent | 一致する成功応答、期限内 | acknowledged。応答由来の設定値のみ更新 |
| failed / expired | ユーザーが状態再照会後に再試行 | 新しい要求。旧Commandは履歴として残す |

期限後の応答は履歴に遅延イベントとして記録し、状態を黙って成功へ変更しない。実状態は再照会して別途表示する。通信断は新規要求を拒否、送信後通信断は期限まで応答待ち。

### 保守案件（施工業者分離を反映）

| 現状態 | 許可主体・イベント | 次状態 |
|---|---|---|
| requested | HQが社内担当へ割当 | assigned |
| requested | HQが外注委託 | offered |
| offered | 対象業者の期間内受諾 / 辞退 | accepted / requested（辞退履歴を保持） |
| accepted | 業者が自社の有効技術者へ割当 | assigned |
| assigned | 担当技術者が期間内に開始 | in_progress |
| assigned / in_progress | HQ（社内）または受託業者（外注）が理由付き再割当 | 状態維持。旧割当を失効、新割当を作成し、作業途中の記録は元作者を保持 |
| in_progress | 担当者が必須報告を提出 | submitted |
| submitted | 業者品質担当（外注）またはHQ（社内）が確認 | completed / rework_requested |
| rework_requested | 担当者が再作業開始 | in_progress（前報告版を保持） |
| requested | 依頼者の取消 | cancelled |
| offered / accepted / assigned | HQが理由付き取消 | cancelled。関連割当を失効 |
| in_progress / submitted | HQが理由付き中断 | on_hold。自動的な完了・取消にはしない |
| on_hold | HQが現在条件を確認して再開 / 終了 | in_progress / cancelled（理由と未完了記録必須） |

completed後の追加作業は関連する新jobIdを作る。外注報告の自己承認は不可。同一userIdをMembership切替で別担当として承認することも禁止。施工業者の品質担当不在はHQへエスカレーション。顧客による正式な最終承認はOPEN-01で未確定のため、1Aでは結果閲覧と問い合わせまで。

Alertはopen→acknowledged→resolved。resolvedの再発は新Alert（previousAlertIdで関連）にする。resolvedは再測定条件を満たすか、HQ/許可された診断者の理由付き確認を必要とし、Jobのcompletedから自動遷移させない。

### 支払い・制限

Paymentはinitiated→processing→confirmed/failed。Offsetはquoted→demo_requested→demo_purchased→demo_retired、失敗はfailedに直前状態を記録する。購入前の償却と二重償却は拒否。Invoiceはunpaid→processing→paid（入金確認時）。失敗時はunpaidへ戻し、遅延判定はdueAtと未入金額から算出する。1Aの分割払い・返金は対象外、1Bの未決契約とする。

| 現状態 | イベント・ガード | 次状態 / 注意 |
|---|---|---|
| 未作成 | 制限可能契約、未払い、権限、理由、予告 | scheduled |
| scheduled | 期限到来、猶予/例外なし、未払いを再確認 | requested、設備別適用Command作成 |
| scheduled | 入金確認または取消、猶予、例外 | cancelled / scheduled（期日変更または適用保留と理由） |
| requested | 対象設備に適用応答 | 全件成功ならapplied。一部未応答はrequestedで設備別状態を表示 |
| requested | オフライン・失敗・期限切れ | requestedのままpendingReasonとcommand結果。自動成功/再送しない |
| requested / applied | 入金確認または権限付き手動解除 | release_requested。適用要求との競合を照会し解除の意思を保持 |
| release_requested | 全対象の解除確認 | released |
| release_requested | オフライン・失敗 | 保留表示、再照会・明示再試行 |

requested以降の取消は「何も適用されていない」と推定せず、解除フローに進む。applied後の猶予・例外も必要なら解除要求を作る。遅い適用応答が解除要求後に届いてもappliedへ戻さない。各設備の観測値と解除要求を再照合する。overrideは支払い記録を変更しない。

CommandとRestriction、JobとAlert、InvoiceとPaymentは別レコード。イベント処理で関連更新するが単一状態に潰さない。UIは部分成功・保留・実際の反映状況を説明する。

### デバイス・自動運転

FW更新Operationはqueued→running→succeeded/failed。オンライン・能力・対象版を検証し、succeeded時だけfirmwareVersion更新。校正は参照値・単位・時刻・作業者付き履歴。制御と更新が競合する場合は更新中の制御を拒否するデモ方針。

自動運転の優先順は機器能力/制限・例外の適用結果→HQ方針→顧客ルール、同じ層ではpriority降順→ID昇順（DEC-09）。条件欠測・同意取消時は発火させない。最終的に同じCommand policyを経るため音声や自動運転で制限を迂回できない。

## 6. Queryとデモ状態

Query keyは`[tenantId, membershipId, scopeVersion, resource, id, normalizedFilters]`。役割切替/サインアウト時は旧リクエストと購読を中断し、キャッシュを消去してから次画面を描画する。権限変更でscopeVersionを更新する。

モックRepositoryは同一タブ内で1つ。正規化したMapとイベント列を持ち、画面のuseStateに複製しない。更新イベントでjobs/units/invoices/restrictions等の関連Queryを無効化する。S02の報告完了ならjobと履歴を更新し、Alert解消は別イベントまで待つ。

再読込時はseedに戻る（DEC-07）。ログアウトでは閲覧キャッシュ・未保存写真を破棄するが共有の架空業務レコードは残す。リセットはデモ時計・遅延処理・購読・画像URL・キャッシュ・業務データをまとめて初期化し、リセット前の遅延イベントを世代番号で破棄する。

時計・ID生成・成功失敗は注入可能にし、乱数と実時刻に依存する試験を避ける。失敗/通信断/取り外し/支払い/応答は/demoから明示発火する。架空テナント2、クライアント2、施工業者2、社内技術者1・外部2、HQ通常/制限権限ありのMembershipを用意する。

## 7. 算定・単位・データ品質

- 電力量[kWh]は電力[kW]の時間積分値。デモで時系列から算出する場合、サンプル間隔と欠測除外条件を固定する。Wからの変換は明示する。
- 推定料金=電力量×仮単価。時間帯料金なら各区間を合算。税・基本料金を含まないデモならその範囲を表示する。
- 排出量[kgCO₂e]=kWh×係数[kgCO₂e/kWh]。係数の地域・年度・版・出典と境界が必須。仮係数を公的実値として表示しない。
- 削減量=比較条件を揃えた基準−実績。削減率=差/基準×100、基準0は算定不可。負値は増加として表示。
- coverage=有効サンプル数/期待サンプル数。期待数0なら未算定。品質に問題があれば集計とレポートに注記する。欠測を0補完しない。
- 室内CO₂ ppmは上記排出量計算には使用しない。省エネ率の期待値は保証値ではない。

## 8. 将来APIへつなぐために残すもの

- 画面が必要とする入力・戻り値のTypeScript型と非同期interface。
- 外部データを画面モデルへ変換するadapterの差替え口。
- UIが扱うloading/error/empty/pending/confirmedと、役割切替時の表示破棄規則。
- 合成応答を使ったフロントエンド検証。

APIパス・HTTP方式・DB・サーバー認証認可・実決済・実通知・実機制御は本書の設計対象外。これらの未決定は今回のフロントエンド文書の完成を妨げない。
