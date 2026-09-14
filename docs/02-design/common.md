---
document_id: DD-COMMON
version: 0.2.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# 共通詳細設計書

入力: [共通要件](../01-requirements/common.md)、[PrepareDocument](../00-prepare/PrepareDocument.md)。実装粒度の入出力・エラー・デモ時間値は[共通実装契約](implementation-contracts.md)、全論理操作は[操作カタログ](operation-catalog.csv)を併読する。以下の技術構成・APIパス・型はPROPOSED。サーバーの実装や契約が存在するという意味ではない。

## 1. 構成と責任

```text
src/
  app/                router, providers, composition-root, route-guards
  features/           units, jobs, billing, restrictions, energy, devices...
    <feature>/        pages, components, queries, forms, schemas
  domain/             entities, policies, transitions, repository-contracts
  infrastructure/
    mock/             repository, fixtures, scenario-clock, event-bus
    api/              http-client, dto-schemas, mappers（1Bで接続）
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

## 3. ドメインモデル

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

## 4. Repositoryと将来APIの契約

```ts
type RequestContext = {
  tenantId: string;
  membershipId: string;
  scopeVersion: number;
};
type Page<T> = { items: T[]; nextCursor: string | null; total?: number };
type WriteOptions = {
  idempotencyKey: string;
  expectedVersion?: number;
  signal?: AbortSignal;
};
type DomainError = {
  code: 'UNAUTHENTICATED' | 'FORBIDDEN' | 'NOT_FOUND' | 'VALIDATION'
    | 'CONFLICT' | 'OFFLINE' | 'TIMEOUT' | 'RATE_LIMITED' | 'UNAVAILABLE';
  messageKey: string;
  fieldErrors?: Record<string, string>;
  correlationId: string;
  retryAfterSeconds?: number;
};
interface CommandRepository {
  create(ctx: RequestContext, input: {
    unitId: string; action: UnitAction; expectedUnitVersion: number;
  }, options: WriteOptions): Promise<Command>;
  get(ctx: RequestContext, commandId: string, signal?: AbortSignal): Promise<Command>;
}
```

上記は型の説明用抜粋。実装ではUnitAction（[実装契約](implementation-contracts.md)）を判別unionにしてZodで検証する。全操作は`Promise<DomainEntity>`、一覧は`Promise<Page<T>>`、集計は型付きSummary、エラーはDomainErrorに統一する。ctxはUIの境界に必要だが、本番サーバーはブラウザの自己申告を認証・認可の根拠にしない。

| 論理操作（役割設計と一致） | 将来HTTP候補 / 入出力の要点 |
|---|---|
| session.get | GET /api/v1/session → 有効Membership・scopeVersion。認証方式はOPEN-04 |
| organizations/customers/units/properties/spaces/contracts/invoices/devices/alerts/jobs/automations/members等のlist/get | GET /api/v1/{resource}?cursor&limit&filter / {id} → Page<T> / T。フィルターはallowlist、limit初期25・最大100（仮） |
| organizations/customers/properties/spaces/units/contracts/members/capabilities/policies/baselines/automationsのsave | POST collectionで作成、PATCH /{id}で版付き更新 → 更新後entity。ID付替えでスコープ変更不可 |
| members.eligible | GET /api/v1/jobs/{id}/eligible-technicians → スコープ・所属・資格・期間内の候補 |
| telemetry.summary / series、energy.summary、admin.summary | GET /api/v1/{resource}/summary または /telemetry/series、metric/from/to/unitIds指定 → 品質・単位・期間付き結果 |
| commands.create / get | POST /api/v1/units/{id}/commands → 202 Command(requested)、GET /api/v1/commands/{id} → 現在の状態 |
| jobs.create / offer / accept / decline / assign / start / submit / review / saveCost | POST /api/v1/jobs または /jobs/{id}/{action} → Job。action payloadは委託先/担当/日程/報告版/判断/金額。状態遷移の前提を検証 |
| jobs.saveDraft / events / addNote、attachments.add | PUT /jobs/{id}/draft、GET /jobs/{id}/events、POST /jobs/{id}/notes、POST /jobs/{id}/attachments → Draft/イベント/案件メモ/Attachment。実アップロード方式は1Bで確定 |
| alerts.acknowledge、notifications.markRead / preview | POST /alerts/{id}/acknowledge、POST /notifications/{id}/read、POST /notification-previews → Alert/Notification/プレビュー |
| invoices.create / payments.confirm | POST /invoices、POST /payments/{id}/confirm → Invoice/Payment。入金確認は1Bでは信頼できるサーバー処理または権限付き操作 |
| restrictions.get / schedule / execute / release / defer / exempt / cancel / override | GET /restrictions/{id}、POST /restrictionsまたは /{id}/{action} → Restriction。理由・ルール版・expectedVersion必須 |
| devices.register / bind / check / calibrate / updateFirmware / events | POST /devices、POST /devices/{id}/{action}、GET /devices/{id}/events → Deviceまたは進捗Operation。実FW転送なし |
| consents.get / update | GET /consents、PUT /consents/{purpose} → Consent。取消で条件依存automationを停止 |
| mrv.preview / saveDraft / recordReview、offsets.preview / list、audit.list | POST /mrv/preview、PUT /mrv/{id}/draft、POST /mrv/{id}/demo-review、POST /offsets/preview、GET /offsets、GET /audit-events → プレビュー/報告/記録/Page |
| payments.simulate / offsets.simulate / automations.simulate | モック専用操作。実APIへ転送しない。将来は別の正式コマンド契約へ置換 |

パスはサーバーチームへの提案。実装時はoperation registryにmethod/path/request schema/response schema/error/retryを登録し、全論理操作を網羅する。DTOに余分な内部情報を含めず、金額や単位はmapperで明示変換する。

HTTP候補: 401=サインインへ、403/404=安全な不可表示、409=再取得し競合を説明、422=フィールドエラー、429=Retry-Afterを尊重、5xx=再試行案内。GETは最大2回のバックオフ再試行（仮）、変更要求は自動再送しない。タイムアウト既定10秒（仮）。ブラウザの中断はサーバー処理の取消ではない。

同じ意思の再送は同じ冪等キーを保持し、新しい意思のみ新キー。時刻変更や重複応答で完了状態を逆行させない。CommandはunitId＋commandId、他更新はversionで照合する。1BのストリームはeventIdで重複除去、versionで順序判定し、切断/欠番時は再取得する。

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

## 8. API接続へ移行する条件

1. OPEN-02〜06の依存項目を担当責任者が確定し、API担当とDTO・認可・エラー・冪等性・応答状態を合意する。
2. HTTP adapterに認証方式を実装。サーバーでテナント/期間/能力を検証。デモ役割選択を本番認証として流用しない。
3. モックとHTTP adapterへ同じcontract testsを適用する。ページ・フォーム・domainの直接API依存を増やさない。
4. 単なるHTTP 202を機器操作成功とみなさない。通知・決済・FW・MRVの実処理はそれぞれ検証可能な応答に接続する。
5. mock-onlyのsimulate操作とデモUIは本番構成で到達不能にする。実接続は今回の作業に含まれず、別途明示の実行指示が必要。
