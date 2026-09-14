---
document_id: DD-CONTRACTS
version: 0.2.0
status: proposed-frontend-contract
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# 共通実装契約・データ入出力

本書は[共通詳細設計](common.md)を実装可能な入出力へ具体化する。各役割詳細のフィールド表・業務規則と合わせて実装する。ここで定める値は1Aのデモ仕様（DEC-09）。本番APIの稼働・業務承認を意味しない。

## DDC-01 共通型と不変条件

| 型 | 値・制約 | 保存・表示 |
|---|---|---|
| EntityId | 1〜128文字、英数字・ハイフン・アンダースコア、opaque | 作成時に生成後不変。URLで受けても認可の証拠にしない |
| Version | 1以上の整数 | 更新ごとに+1。フォーム表示時の版をexpectedVersionへ |
| Instant | timezone付きISO 8601、保存UTC | ローカル文字列をそのまま保存しない |
| DateRange | from < to、最大366日、[from,to) | 366日はデモUI上限、将来APIの実装制限とは別 |
| Money | amountMinor:非負整数、currency:ISO通貨コード | JS安全整数範囲。デモMYRは2桁、異なる通貨は合算不可 |
| Percentage | number/null | 分母0はnull、表示丸め小数1桁。負の削減率も保持 |
| Measurement | metric, value:number/null, unit, observedAt, receivedAt, origin, quality, isDemo | NaN/Infinity拒否。nullはmissing、0は測定値。実測/推定/点検とdemoは独立 |
| ResourceRef | tenantId, entityId, version | 参照先が同テナント・許可スコープにあることをRepositoryで確認 |
| ListQuery | cursor?, limit=25, sort, filters | limit1〜100。sort/filtersはoperationごとのallowlist。未知キーはvalidationエラー |
| Page<T> | items:T[], nextCursor:string/null, total?:number | total不明は「全件数未取得」。既知の0と区別 |

表示用丸め値を次の計算へ再利用しない。金額計算は最終段階で通貨のminor unitへ四捨五入するデモ方針。浮動小数の中間丸め誤差を避ける共通算定関数を用い、デモ期待値で試験する。単位変換はmapper/計算関数に集約する。

### 共通操作型

```ts
type UnitAction =
  | { kind: 'set_power'; power: boolean }
  | { kind: 'set_temperature'; celsius: number }
  | { kind: 'set_mode'; mode: string }
  | { kind: 'set_fan'; fanLevel: string }
  | { kind: 'ventilate'; level: string };

type CreateCommandInput = {
  unitId: string;
  action: UnitAction;
  jobId?: string;       // 技術者による診断は必須
  reason?: string;      // 診断/HQ変更は必須
  expectedUnitVersion: number;
};
type WriteMeta = {
  idempotencyKey: string;
  expectedVersion?: number;
};
type ApiEnvelope<T> = {
  data: T;
  meta: { correlationId: string; serverTime: string };
};
type ApiErrorEnvelope = {
  error: {
    code: string;
    messageKey: string;
    fieldErrors?: Record<string, string>;
    correlationId: string;
    retryAfterSeconds?: number;
  };
};
```

型名は契約上の名前。TypeScript/Zodの実ファイルは実装工程で作る。mode/fan/levelの値はCapabilityの候補と一致し、単なるstringで任意入力を受け入れない。1Aは1回の確認送信につき1Action。複数Actionの一括操作・部分成功UIは今回の必須にしない。複数設備への制限は別のRestrictionユースケースで設備別Commandとして管理する。

### 要求・応答の具体例

```json
{
  "unitId": "unit-online-rto",
  "action": { "kind": "set_temperature", "celsius": 24 },
  "expectedUnitVersion": 7
}
```

```json
{
  "data": {
    "id": "command-demo-001",
    "tenantId": "tenant-a",
    "version": 1,
    "unitId": "unit-online-rto",
    "status": "requested",
    "action": { "kind": "set_temperature", "celsius": 24 },
    "requestedAt": "2026-09-14T01:00:00Z",
    "expiresAt": "2026-09-14T01:00:30Z",
    "isDemo": true
  },
  "meta": { "correlationId": "corr-demo-001", "serverTime": "2026-09-14T01:00:00Z" }
}
```

HTTP候補はPOST `/api/v1/units/unit-online-rto/commands`、202。`Idempotency-Key`をheaderで送り、認証はサーバーの信頼できる主体へ紐付ける。要求時の旧Unitは室温28・設定26のまま、成功応答後に設定24へ更新する。

```json
{
  "error": {
    "code": "CONFLICT",
    "messageKey": "errors.resource_changed",
    "correlationId": "corr-demo-002"
  }
}
```

CONFLICTは最新データを取得して利用者の確認後に新しい意思として送る。機器応答待ちの同じ意思を通信理由で再送する場合は元の冪等キーを保持する。

## DDC-02 読取projection・関連取得

同じエンティティでも役割別に必要なフィールドだけ返す。UIで隠すために全顧客データを一度ブラウザへ送らない。以下のprojectionはmockでも適用する。

| projection | 必須応答フィールド | 除外・関連の解決 |
|---|---|---|
| UnitSummary | id, version, customerOrgId, propertyId, spaceId, displayName, modelId, capabilityVersion, connection, observedPower, latestMeasurements, activeAlertCount | 契約金額・顧客連絡先を含めない |
| UnitDetail | UnitSummary＋installedAt, components, serviceScope, capabilities, lastSeenAt, pendingCommandIds | capabilityと観測設定・要求設定を別プロパティ |
| JobOfferSummary | jobId, offerId, type, regionLabel, requiredQualifications, requestedSlot, dueAt, offerExpiresAt, termsVersion | 未受諾業者には正確な住所・live telemetry・顧客請求を返さない |
| JobSummary | id, version, unitRef, type, status, dueAt, requestedSlot, scheduledSlot, assignmentSummary, severity, isDemo | 内部Note・顧客請求なし。未受諾外注はJobOfferSummaryへ |
| JobDetail | id, version, unitRef, type, status, requestedSlot, scheduledSlot, assignment, offer, reportRefs, costSummary, eventCursor | 顧客には内部Note・未受理報告を含めず、費用は顧客向け表示が許可されたものだけ |
| JobHistorySnapshot | jobId, type, status, contractorOrgId, completedAt, ownDecisionEvents, redactedReportSummary | 外部失効後のlive設備参照・顧客個人情報・新規制御は不可 |
| InvoiceDetail | id, version, contractId, contractVersion, amountMinor, currency, dueAt, paymentState, paymentRefs, restrictionIds | 戻り値にカード情報なし。制限はrestrictionIdsまたはforInvoiceで取得 |
| RestrictionDetail | id, version, contractId, rulesVersion, state, reason, executeAfter, graceUntil, exception, perUnitApply, perUnitRelease, events | 設備ごとにCommand ID・状態・最後の確認時刻 |
| DeviceDetail | id, version, unitId, serial, connection, lastSeenAt, sensors, calibrationRefs, firmwareVersion, activeOperation | offline、power断、tamperを別軸 |
| EnergySummary | period, unitIds, totals, baselineRef, factorRef, tariffRef, boundary, coverage, qualityWarnings | totalsの未算定値はnull。丸め前の計算値と表示桁を分離 |
| AuditView | id, actorId, actorRoleAtTime, action, targetRef, occurredAt, correlationId, result, maskedBefore, maskedAfter, reason | 現在のMembership名で過去主体を上書きしない |

### 追加モデル

| モデル | 項目・責務 |
|---|---|
| Offer | id, jobId, contractorOrgId, termsVersion, offeredAt, offerExpiresAt, accessValidFrom, accessValidUntil, decision, decidedBy, decidedAt, declineReason |
| Assignment | id, jobId, technicianMembershipId, validFrom, validUntil, scheduledStart, scheduledEnd, status, reason。期間終了でlive権限失効 |
| MaintenancePlan | id, unitId, recurrence, nextDueAt, generatedOccurrenceIds。1Aは月単位・次回1回を明示生成、計画id＋予定日で重複防止 |
| JobNote | id, jobId, authorId, visibility(internal/customer), message, createdAt。内部初期、通知本文も同じ公開範囲 |
| Inquiry | id, customerId, invoiceId?, restrictionId?, subjectType, message, state(received/answered/closed), reply?, createdAt。デモin-app受付 |
| CalibrationRecord | id, deviceId, sensorId, metric, unit, referenceValue, measuredValue, calibratedAt, actorId, isDemo。過去履歴は追記 |
| DeviceOperation | id, deviceId, kind(check/calibrate/firmware), status(queued/running/succeeded/failed), targetVersion?, failureCode?, startedAt?, finishedAt? |
| OffsetQuote | id, version, amountKg, estimatedAmountMinor?, currency?, expiresAt, provider=unselected, scheme=demo, isDemo。料金未定なら金額null |
| OffsetRecord | quoteId, amountKg, state, previousState?, purchaseRef?, retirementRef?, demoCertificateRef?, eventHistory。全量デモ償却のみ |

## DDC-03 入力検証・表示・失敗

| 検証時点 | 処理 |
|---|---|
| URL読込 | ID/enum/dateを検証。入力不正は安全な初期条件またはnot-foundへ。自動で別顧客に解決しない |
| フィールドblur/submit | 役割別表の必須・形式・文字数・大小関係。文字数はtrim後のUnicode code pointで数える |
| mutation直前 | actor/tenant/role/scope/期間/現在status/capability/契約/expectedVersionを再照合 |
| adapter応答 | DTO schema検証、未知enumはunsupported/error。生の内部例外は画面へ出さない |
| 更新通知 | entityId＋version＋eventIdで重複/古いイベント除外。欠番や切断は再照会 |

| DomainError / HTTP候補 | UI動作 | 入力・再試行 |
|---|---|---|
| VALIDATION / 422 | error summaryとfieldErrors、最初の欄へfocus | 値を保持して修正。無条件再送なし |
| UNAUTHENTICATED / 401 | セッション終了とlogin | 旧スコープのQuery・機微な下書き/写真を破棄 |
| FORBIDDEN / 403 | 操作不可と許可されたホーム/一覧へ | 権限変更時は対象データを破棄。繰返し再試行しない |
| NOT_FOUND / 404 | 対象が利用できない旨と一覧へ | 存在を漏らす詳細を出さない |
| CONFLICT / 409 | 変更発生の説明＋最新取得＋差分確認 | 自動上書きなし。変更意図を再確認して新規要求 |
| OFFLINE | 最後の通信時刻と保留/操作不可 | 新規制御は不可。送信済は状態照会、自動成功なし |
| TIMEOUT / 408相当 | 処理未確定と相関ID | writeは状態照会→同じ冪等キーで再試行。元処理未実行と断定しない |
| RATE_LIMITED / 429 | 待機時間と再試行案内 | Retry-Afterを守る。通常フォーム値保持 |
| UNAVAILABLE / 5xx | インラインエラーと再試行 | GET最大2回バックオフ、writeは明示再試行のみ |

画面error時にKPIを0件や正常へ置換しない。前回値を残す場合はstaleと前回成功時刻を明示。対象やroleが変わった場合は前回値を残さない。

## DDC-04 非同期・モック実行の基準

| 設定 | 1A既定値・挙動 |
|---|---|
| HTTP timeout | 10秒。mock読取成功は即時〜300msの固定設定、試験では時計を注入 |
| Command expiry | 要求時から30秒（デモ仮値）。送信/応答はシナリオイベントで制御 |
| Telemetry stale | sensorに設定、seedは120秒。境界now-observedAt > staleAfterSeconds |
| Offer期限 | seedで24時間後。受諾はnow < offerExpiresAt |
| 予告期間 | seedルールで24時間。executeAfter >= noticeAt+24h。商用ルールではない |
| OffsetQuote有効期間 | デモ15分、申込時点でnow < expiresAt |
| write処理 | validate→version check→state mutation→audit/eventの記録をRepository内で原子的に実行。途中失敗で半端な複数レコード更新を残さない |
| invalidate | operationが触ったentityと関連集計をscope付きQuery keyで無効化。UIは手動コピーしない |
| role変更 | pending requestをabort、旧Query除去、次scopeのsession取得後に描画。遅い旧応答はsession generationで破棄 |
| reload/reset | DEC-07。reloadはseed、role切替は同タブ業務データ保持。resetは時計・世代・object URLも初期化 |

通常の顧客/診断コマンドは同一設備に未完了要求がある間は競合要求を拒否する。入金後の解除は別の調整処理として解除意思を先に保持し、適用要求の反映を照会してから解除Commandを送る。未確定のまま適用と解除を同時送信しない。

`AbortController`は通信や表示への反映を中断するためのもの。既に受理された機器・決済処理の取消と混同しない。1Bへ移行する際にはサーバー状態照会を必須とする。

## 通知と公開範囲

| 起点イベント | 宛先と公開内容 | 発生させない変更 |
|---|---|---|
| alert.opened / severity_changed | 顧客・HQ、担当がある場合は有効技術者。業者は受諾内設備の必要概要 | 既読でAlert解消しない |
| job.requested | 顧客に受付、HQへ新規依頼 | 希望枠を予約確定にしない |
| job.offered / accepted / declined | offer先業者とHQ。顧客は「手配中」まで | 受諾前に詳細顧客情報を公開しない |
| job.assigned / schedule_changed | 顧客・HQ・担当技術者・受託業者に確定日程と必要情報 | 別会社技術者への通知なし |
| report.submitted / returned | 品質担当/作成技術者/HQ。顧客には進捗のみ | 内部報告・内部Noteを顧客に公開しない |
| job.completed | 顧客・HQ・受託業者・担当に受理済報告 | Alertを自動解消しない |
| restriction.scheduled / changed | 対象顧客、権限付きHQへ理由・予定・対象・解除条件 | 業者/技術者へ請求詳細を渡さない |
| payment.confirmed | 顧客・billing権限HQ、関連解除イベント | 応答なしで解除済みにしない |
| device.fault / operation_failed | 担当技術者・HQ、必要な概要だけ顧客 | 故障原因・盗難を未確認で断定しない |
| inquiry.received / answered | 顧客と対応権限HQ | メール/WhatsAppを実送信しない |

in-app通知は翻訳キーとparamsを保存し選択言語で描画。メール/WhatsAppはpreview/simulatedで、送信済み実績として扱わない。通知には対象参照IDとsnapshot scopeを持ち、遷移時にも現在の認可を検証する。

## DDC-05 API移行の受入条件

- [操作カタログ](operation-catalog.csv)全行にrequest schema、response schema、認可、状態ガードを実装する。GET共通規約は本書、業務入力は各DDのフィールド表を参照する。
- mock-onlyの操作はHTTP adapterへ流さず、実接続用操作とは分離する。公開API候補は未合意で、サーバー側仕様が決まった時点で版を更新する。
- entity/DTOのmapper、DomainError変換、scope/period/capability/冪等性/versionのcontract testsを同じケース集合で実行する。
- ボタンや画面をAPI接続のために作り直さず、composition-rootのadapterと認証・購読層を置き換える。ブラウザ側のrole制御だけを本番の認可にしない。

## DDC-06 画面構成とローカル状態

各role pageは共通ShellとUI patternを使う。pageはURL・Query hook・フォームを組み立て、primitiveはRepositoryをimportしない。RHFフォームごとに対象IDをkeyとして分離し、query再取得でdirty値を自動resetしない。

dialog open、選択中tabなど短命stateは最も近いcomponentへ。対象・期間・一覧pageはURL、観測値・履歴はQuery、業務データはRepositoryに各1箇所だけ保持する。意味のあるフィルターは戻る操作で復元し、画面を跨いで不要なContextを追加しない。

一括取得失敗時はページ全体error、独立した補助パネルだけの失敗はパネル内errorにする。例えば設備基本情報が成功して履歴取得だけ失敗した場合、設備参照は継続し履歴のみ再試行する。ただし制御の可否判定に必要なcapability/制限情報が取得できない場合は操作を有効にしない。

## DDC-07 共通画面の項目・処理

| 画面/操作 | 入力・初期値 | 処理と完了 | 失敗・取消 |
|---|---|---|---|
| /login | demoActorId:未選択、returnTo:任意相対route | mock sessionへ既知actorのMembershipを設定しrole homeへ。実メール/パスワード不要 | 未選択・未知actorを拒否。外部URLのreturnToは破棄 |
| /forgot-password | demoEmail:空、形式・最大254文字 | 既存/非既存どちらも「該当する場合の案内を表示します」の同一demo完了 | 実送信なし。形式不正はfield error |
| /settings/preferences | locale=en、zone=Asia/Kuala_Lumpur、currency=MYR | locale即時反映、zone/通貨は表示設定。データや契約通貨は不変 | 未対応locale/不正IANA zoneを拒否 |
| 共通通知 | unreadOnly=false、cursor=null、limit25 | scope内通知Page、既読mutation、参照IDから関連routeへ | scope失効したリンクは安全な不可表示 |
| 音声/テキストパネル | text:空、intent:未解決、target:未選択 | intent解決→対象候補→変更は確認→既存Command。照会は読取のみ | 認識不可/同名複数/確認取消ならCommand0件 |
| /demo | scenarioId、eventType、clockAdvance、reset | 許可された合成イベントのみ、reset時にgeneration更新 | 任意URL/スクリプト/実機宛先を受け入れない |

共通操作の論理契約は`demoSession.signIn/signOut/switchMembership`、`auth.previewPasswordReset`、`preferences.get/update`、`voice.resolveIntent`、`notifications.list/markRead`、`demo.trigger/reset`。demo/auth/voiceは1Aではモック専用、API版は別途認証方式を合意する。preferencesは低頻度の表示設定Providerに置き、業務Repositoryのデータや請求通貨とは分離する。

### 能力権限の名称と付与先

| 能力 | 付与可能な役割・制約 |
|---|---|
| control.execute | 顧客/HQ、利用/管理範囲と契約・機器能力内 |
| control.diagnose / device.maintain / alert.resolve | 技術者の有効担当内。外部は作業期間内、解消は理由・根拠必須 |
| job.manage / contract.manage / billing.manage | HQ、対象テナント内 |
| partner.accept / partner.assign / partner.review | 施工業者、自社委託のみ。reviewは同一userIdの自己承認を拒否 |
| identity.manage / device.manage | HQ、同テナント。自己の高権限付与は別主体による変更が必要 |
| alert.policy.manage / automation.policy.manage / energy.manage | HQ、管理対象の方針・分析 |
| restriction.manage / restriction.override | HQ内でも独立能力。通常HQに自動付与しない |
| mrv.manage / offset.manage / audit.read | HQの個別能力。顧客向け分析/記録閲覧とは分離 |

roleによる候補allowlistとpermissionの両方を判定する。既知でない能力名は許可しない。付与可能であることと初期付与することは別で、seedの各actorへ明示した能力集合だけ与える。

### 補助操作の入力

Inquiry作成は`invoiceIdまたはrestrictionId、subjectType、message(1〜2000文字)`。HQ回答は`inquiryId、reply(1〜2000文字)、expectedVersion`でreceived→answered、顧客は返信を読取。閉じる操作は1Aでは任意で未実装なら表示しない。

MRV draftの初回保存はPOST `/api/v1/mrv/drafts`で新ID、既存版保存はPUT `/api/v1/mrv/{id}/draft`。reportIdなしの画面から存在しないIDへPUTしない。

監視・制御に必要なデータ取得と操作カタログには、補助的なget/listも含める。UIに編集ボタンを置く場合は対象detailの読取と保存契約を対にする。
