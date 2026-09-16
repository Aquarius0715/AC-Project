---
document_id: DD-CONTRACTS
version: 0.7.0
status: proposed-frontend-contract
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# フロントエンド入出力契約・モック動作

この文書は、[共通詳細設計](common.md)の内容を、実装できる形の入力と出力に具体化したものです。各役割の詳細設計にあるフィールド表・業務規則と合わせて実装します。ここで決める値は、1A(このフェーズ)のデモ仕様です(DEC-09)。対象は、ブラウザの中だけで動く画面モデル・フォーム・モックサービス(模擬のサービス)です。データベース、サーバー側の処理、APIのendpoint、認証の方式は、この文書では定めません。ここで使う「保存」「一意」「監査」という言葉は、架空のデータをブラウザの中だけで扱う動作を指します。本番環境でデータが残ることや、安全であることを保証するものではありません。

## DDC-01 フロントエンド共通型と表示整合

| 型 | 値・制約 | 保存・表示 |
|---|---|---|
| EntityId | 1〜128文字で、英数字・ハイフン・アンダースコアだけを使います。中身に意味を持たせない不透明な値(opaque)です | 作成時に生成され、その後は変わりません。URLから受け取った値だけでは、権限があるとは判断しません |
| Version | 1以上の整数 | 更新するたびに1ずつ増えます。フォームを表示したときのバージョンを`expectedVersion`として使います |
| Instant | タイムゾーン付きのISO 8601形式です。保存はUTC(協定世界時)で行います | ローカルの時刻文字列を、そのままは保存しません |
| DateRange | from < to、最大366日、[from,to) | 366日という上限は、デモ用UIの上限です。将来のAPIの実装で決める制限とは別のものです |
| Money | amountMinorは0以上の整数、currencyはISOの通貨コードです | JavaScriptの安全な整数の範囲内で扱います。デモで使うMYR(マレーシアリンギット)は小数点以下2桁です。通貨が異なる金額は合算できません |
| Percentage | number/null | 分母が0のときはnullにします。表示するときは、小数点以下1桁に丸めます。マイナスの削減率もそのまま保持します |
| Measurement | metric, value:number/null, unit, observedAt, receivedAt, origin, quality, isDemo | NaNやInfinityの値は拒否します。nullは「値がない」ことを表し、0は「測定値が0だった」ことを表します。実測・推定・点検の区別と、デモかどうかの区別は、それぞれ独立しています |
| ResourceRef | tenantId, entityId, version | 参照先が、同じテナント(組織の区画)にあり、許可された範囲(scope)にあることを、Repositoryで確認します |
| ListQuery | cursor?, limit=25, sort, filters | limit(件数)は1〜100の範囲です。sortとfiltersは、操作(operation)ごとに決めた許可リスト(allowlist)の中からだけ使えます。知らないキーを指定すると、検証エラーになります |
| Page<T> | items:T[], nextCursor:string/null, total?:number | totalが不明なときは「全件数はまだ取得していない」ことを表します。件数が0件だと分かっている場合とは区別します |

表示用に丸めた値を、次の計算にそのまま使い回してはいけません。金額の計算では、最後の段階で通貨のminor unit(最小単位)に四捨五入するのが、デモでの方針です。浮動小数点の計算で途中の丸め誤差が出ないように、共通の計算関数を使い、デモの期待値でテストします。単位の変換は、mapper(変換処理)や計算関数にまとめます。

### 共通操作型

```ts
type DemoViewContext = {
  tenantId: string; membershipId: string; scopeVersion: number;
};

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
type DemoWriteOptions = {
  idempotencyKey: string;
  expectedVersion?: number;
};
type ServiceResult<T> = {
  data: T;
  meta: { correlationId: string; snapshotAt: string };
};
// 失敗時はこの型を持つErrorをthrowし、Promiseをrejectする。
interface DomainError extends Error {
  code: string;
  messageKey: string;
  fieldErrors?: Record<string, string>;
  correlationId: string;
  retryAfterSeconds?: number;
}
```

型名は契約上決めた名前です。TypeScript/Zodの実際のファイルは、実装の工程で作成します。mode・fan・levelの値は、Capability(その設備が持つ能力)の候補と一致させます。任意の文字列を自由に受け入れることはしません。1A(このフェーズ)では、1回の確認送信につき1つのActionだけを扱います。複数のActionをまとめて操作したり、一部だけ成功したりするUIは、今回は必須にしません。複数の設備に対する制限は、別のRestriction(制限)のユースケースとして、設備ごとのCommandで管理します。

### 画面からモックサービスへ渡す値・戻り値の例

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
  "meta": { "correlationId": "corr-demo-001", "snapshotAt": "2026-09-14T01:00:00Z" }
}
```

画面は`commands.create`を呼び出します。モックは`requested`(依頼済み)という状態を返します。要求した時点の室温28度と、確認済みの設定温度26度を保持し、合成した成功イベントの後で、設定温度を24度へ更新します。HTTPの送信や、実際の機器への接続は行いません。

失敗したときは、成功したかのような値を返しません。次の属性を持つ`DomainError`でPromiseをreject(拒否)します。以下は、エラーが記録される例です。

```json
{
  "name": "DomainError",
  "message": "Resource changed",
  "code": "CONFLICT",
  "messageKey": "errors.resource_changed",
  "correlationId": "corr-demo-002"
}
```

すべてのサービスで、成功時の型は`Promise<ServiceResult<T>>`です。操作カタログにある`result_contract`は、この包み(ServiceResult)の中にあるTの部分を表します。戻り値がない操作(void操作)でも、`ServiceResult<void>`を返します。失敗した場合に、失敗オブジェクトをresolve(成功扱い)することはありません。Query hook(データ取得の仕組み)は、成功したときの`result.data`を表示用のデータとして使い、catchした`DomainError`はDDC-03の規則へ渡します。ローカルの設定を変更する操作も、同じ非同期の規約に合わせます。

```ts
try {
  const result = await commands.create(context, input, options);
  showRequestedCommand(result.data); // requestedを表示し、機器成功とはしない
} catch (error) {
  showDomainError(asDomainError(error)); // 未知例外はUNAVAILABLEへ正規化
}
```

`CONFLICT`(競合)が起きた場合は、最新のデータを取得し、利用者に確認してもらった上で、新しい意思として送り直します。機器の応答を待っている間、同じ意思を通信の都合で再送する場合は、元の冪等キー(idempotencyKey)をそのまま使います。

## DDC-02 読取projection・関連取得

同じエンティティ(データの対象)でも、役割によって必要なフィールドだけを返します。UI側で隠すために、いったん全顧客のデータをブラウザへ送るようなことはしません。以下のprojection(絞り込んだ表示形式)は、モックでも同じように適用します。

| projection | 必須応答フィールド | 除外・関連の解決 |
|---|---|---|
| UnitSummary | id, version, customerOrgId, propertyId, spaceId, displayName, modelId, capabilityVersion, connection, observedPower, latestMeasurements, activeAlertCount | 契約金額や顧客の連絡先は含めません |
| UnitDetail | UnitSummary＋installedAt, components, serviceScope, capabilities, lastSeenAt, pendingCommandIds | capability(能力)と、観測された設定・要求された設定は、別々のプロパティに分けます |
| JobOfferSummary | jobId, offerId, type, regionLabel, requiredQualifications, requestedSlot, dueAt, offerExpiresAt, termsVersion | まだ受諾していない業者には、正確な住所・ライブのtelemetry(計測データ)・顧客の請求情報は返しません |
| JobSummary | id, version, unitRef, type, status, dueAt, requestedSlot, scheduledSlot, assignmentSummary, severity, isDemo | 内部向けのメモや顧客の請求情報は含めません。まだ受諾していない外注については、JobOfferSummaryを使います |
| JobDetail | id, version, unitRef, type, status, requestedSlot, scheduledSlot, assignment, offer, draftReportRef, reportRefs（reportId/reportVersion）, costSummary, eventCursor | 顧客に対しては、内部向けメモや未受理の報告は含めません。費用は、顧客向けに表示してよいと許可されたものだけを含めます |
| JobHistorySnapshot | jobId, type, status, contractorOrgId, completedAt, ownDecisionEvents, redactedReportSummary | 外部の権限が失効した後は、ライブの設備参照・顧客の個人情報・新しい制御の操作はできません |
| InvoiceDetail | id, version, contractId, contractVersion, amountMinor, currency, dueAt, paymentState, paymentRefs, restrictionIds | 戻り値にカード情報は含めません。制限に関する情報は、restrictionIdsまたはforInvoiceを使って取得します |
| RestrictionDetail | id, version, contractId, causeInvoiceIds, rulesVersion, state, reason, executeAfter, graceUntil, exception, perUnitApply, perUnitRelease, events | 設備ごとに、Command(命令)のID・状態・最後に確認した時刻を持ちます |
| DeviceDetail | id, version, unitId, serial, connection, lastSeenAt, sensors, calibrationRefs, firmwareVersion, activeOperation | offline(オフライン)、電源断、tamper(不正な取り外し)は、それぞれ別の軸として扱います |
| EnergySummary | period, unitIds, totals, baselineRef, factorRef, tariffRef, boundary, coverage, qualityWarnings | totalsの中で、まだ計算できていない値はnullにします。丸める前の計算値と、表示する桁数は分けて扱います |
| AuditView | id, actorId, actorRoleAtTime, action, targetRef, occurredAt, correlationId, result, maskedBefore, maskedAfter, reason | 現在のMembership(所属)の名前で、過去の行為者の名前を上書きしてはいけません |

### 追加モデル

| モデル | 項目・責務 |
|---|---|
| Offer | id, jobId, contractorOrgId, termsVersion, offeredAt, offerExpiresAt, accessValidFrom, accessValidUntil, decision, decidedBy, decidedAt, declineReason |
| Assignment | id, jobId, technicianMembershipId, validFrom, validUntil, scheduledStart, scheduledEnd, status, reason。期間が終了すると、ライブの権限は失効します |
| MaintenancePlan | id, unitId, recurrence, nextDueAt, generatedOccurrenceIds。1A(このフェーズ)では、月単位で、次回の1回分だけをはっきり生成します。計画のIDと予定日の組み合わせで、重複を防ぎます |
| JobNote | id, jobId, authorId, visibility(internal/customer), message, createdAt。初期状態は内部向けです。通知の本文も、同じ公開範囲に従います |
| Inquiry | id, customerId, invoiceId?, restrictionId?, subjectType, message, state(received/answered/closed), reply?, createdAt。デモでは、アプリ内での受付のみです |
| CalibrationRecord | id, deviceId, sensorId, metric, unit, referenceValue, measuredValue, calibratedAt, actorId, isDemo。過去の履歴には追記するだけで、書き換えません |
| DeviceOperation | id, deviceId, kind(check/calibrate/firmware), status(queued/running/succeeded/failed), targetVersion?, failureCode?, startedAt?, finishedAt? |
| OffsetQuote | id, version, amountKg, estimatedAmountMinor?, currency?, expiresAt, provider=unselected, scheme=demo, isDemo。料金がまだ決まっていない場合、金額はnullにします |
| OffsetRecord | quoteId, amountKg, state, previousState?, purchaseRef?, retirementRef?, demoCertificateRef?, eventHistory。すべてデモ上の償却(retirement)としてのみ扱います |

## DDC-03 入力検証・表示・失敗

| 検証時点 | 処理 |
|---|---|
| URL読込 | ID・enum(列挙値)・日付を検証します。入力が不正な場合は、安全な初期条件にするか、not-found(見つからない)として扱います。自動的に別の顧客のデータに解決することはありません |
| フィールドblur/submit | 役割ごとの表にある、必須項目・形式・文字数・大小関係を検証します。文字数は、前後の空白を除いた後のUnicodeコードポイントで数えます |
| mutation直前 | actor(行為者)・tenant・role(役割)・scope・期間・現在のstatus・capability・契約・expectedVersionを、あらためて確認します |
| adapter応答 | DTOのschemaを検証します。知らないenum値はunsupportedまたはエラーとして扱います。内部の生の例外を、そのまま画面に表示することはありません |
| 更新通知 | entityId・version・eventIdの組み合わせで、重複したイベントや古いイベントを除きます。欠番や通信の切断があった場合は、あらためて取得し直します |

| モックが返すDomainError | UI動作 | 入力・再試行 |
|---|---|---|
| VALIDATION | エラーの概要と、項目ごとのエラー(fieldErrors)を表示し、最初のエラー欄にフォーカスを移します | 入力した値は保持したまま、修正できるようにします。無条件に再送はしません |
| UNAUTHENTICATED | セッションを終了し、ログイン画面へ移動します | 古いscopeのQuery、画面内の未保存の下書きや写真、一時的なURLは破棄します。保存済みの報告やBlob(画像データ)は、DDC-08の規則に従って共有Repositoryに保持します |
| FORBIDDEN | 操作できないことを示し、許可されているホーム画面や一覧画面へ移動します | 権限が変更されたときは、対象のデータを破棄します。繰り返し再試行することはしません |
| NOT_FOUND | 対象が利用できないことを示し、一覧画面へ移動します | その対象が存在するかどうかが分かってしまうような詳細は表示しません |
| CONFLICT | 変更が発生したことを説明し、最新のデータを取得して、差分を確認できるようにします | 自動的に上書きすることはありません。変更したい内容をあらためて確認してから、新しい要求として送ります |
| OFFLINE | 最後に通信できた時刻を表示し、保留中であることや、操作できないことを示します | 新しい制御操作はできません。すでに送信したものは、状態を照会します。自動的に成功したことにはしません |
| TIMEOUT | 処理がまだ確定していないことと、相関ID(処理を追跡するID)を表示します | 書き込み(write)は、まず状態を照会し、その後は同じ冪等キーで再試行します。元の処理が実行されなかったと決めつけません |
| RATE_LIMITED | 待つべき時間と、再試行の案内を表示します | モックの結果にあるretryAfterSeconds(再試行までの秒数)を表示します。通常どおり、フォームの入力値は保持します |
| UNAVAILABLE | 画面内にエラーを表示し、再試行できるようにします | 読み取り用のサービスは、最大2回まで自動で再試行します。書き込み(write)は、利用者が明示的に再試行したときだけ行います |

画面にエラーが出ているときに、KPI(指標)の値を0件や正常な値に置き換えてはいけません。前回の値を残す場合は、staleである(古い)ことと、前回成功した時刻をはっきり表示します。対象やroleが変わった場合は、前回の値を残しません。

## DDC-04 非同期・モック実行の基準

デモで使う仮の値(時間・上限・枚数・期間)は、この節とDDC-01で定義します。要件や各DDに同じ数値が書かれている場合は、この節を正式なものとします。値を変更するときは、まずこの節を更新し、同じ変更を他の箇所にも反映してそろえます。

| 設定 | 1A既定値・挙動 |
|---|---|
| モック待機タイムアウト | 10秒です。モックの読み取りが成功するまでの時間は、即時〜300msの間で固定の設定を使います。試験(テスト)では、時計を注入して制御します |
| Command expiry | 要求してから30秒です(デモの仮の値)。送信と応答は、シナリオのイベントで制御します |
| Telemetry stale | センサーごとに設定します。初期値(seed)は120秒です。境界の判定は「現在時刻 − 観測時刻 > staleAfterSeconds」です |
| Offer期限 | 初期値(seed)では24時間後です。受諾できるのは「現在時刻 < offerExpiresAt」のときです |
| 予告期間 | 初期値のルールでは24時間です。「executeAfter >= noticeAt + 24時間」となります。これは商用のルールではありません |
| OffsetQuote有効期間 | デモでは15分です。申し込み時点で「現在時刻 < expiresAt」である必要があります |
| write処理 | 共有メモリの遷移関数で、入力とバージョンを確認してから、状態と表示用の履歴をまとめて更新します。データベースのトランザクションは設計しません |
| invalidate | 操作(operation)が触れたentityと、関連する集計は、scope付きのQuery keyで無効化します。UI側で手動でデータをコピーすることはしません |
| role変更 | 進行中のリクエスト(pending request)は中止(abort)します。古いQueryは取り除きます。新しいscopeのセッションを取得してから、画面を描画します。遅れて届いた古い応答は、session generation(セッションの世代番号)によって破棄します |
| reload/reset | DEC-07に従います。reload(再読み込み)では初期値(seed)に戻ります。role(役割)の切り替えでは、同じタブの業務データは保持します。reset(リセット)では、時計・世代番号・object URLもすべて初期化します |

通常の顧客や診断からのコマンドは、同じ設備に対してまだ完了していない要求がある間は、競合する新しい要求を拒否します。入金後の解除は、別の調整処理として扱います。まず解除したいという意思を保持しておき、適用の要求が反映されたことを確認してから、解除のCommandを送ります。適用と解除を、未確定のまま同時に送ることはありません。

`AbortController`は、通信や画面への反映を途中で中断するためのものです。すでに受理された機器の処理や決済処理を取り消すこととは、混同しないでください。将来、外部処理を取り消す仕様が必要になった場合は、API側の仕様が決まった後で、adapter(接続部分)に対応させます。

## 通知と公開範囲

| 起点イベント | 宛先と公開内容 | 発生させない変更 |
|---|---|---|
| alert.opened / severity_changed | 顧客とHQ(本部)に通知します。担当がいる場合は、有効な技術者にも通知します。業者には、受諾済みの設備についての必要な概要だけを伝えます | 既読にしても、アラートは解消しません |
| job.requested | 顧客には受け付けたことを伝え、HQには新規の依頼として伝えます | 希望した時間枠を、そのまま予約確定にはしません |
| job.offered / accepted / declined | オファーを受けた業者とHQに伝えます。顧客には「手配中」という段階までしか伝えません | 受諾する前に、詳しい顧客情報を公開することはありません |
| job.assigned / schedule_changed | 顧客・HQ・担当技術者・受託業者に、確定した日程と必要な情報を伝えます | 別の会社の技術者には通知しません |
| report.submitted / returned | 品質担当・作成した技術者・HQに伝えます。顧客には進捗だけを伝えます | 内部向けの報告や内部メモは、顧客に公開しません |
| job.completed | 顧客・HQ・受託業者・担当技術者に、受理済みの報告を伝えます | アラートを自動では解消しません |
| restriction.scheduled / changed | 対象の顧客と、権限を持つHQに、理由・予定・対象・解除条件を伝えます | 業者や技術者には、請求の詳細を渡しません |
| payment.confirmed | 顧客と、billing(請求)の権限を持つHQに伝えます。関連する解除のイベントも伝えます | 応答がないまま、解除済みとして扱うことはありません |
| device.fault / operation_failed | 担当技術者とHQに伝えます。顧客には、必要な概要だけを伝えます | 故障の原因や盗難を、確認しないまま決めつけません |
| inquiry.received / answered | 顧客と、対応する権限を持つHQに伝えます | メールやWhatsAppを、実際には送信しません |

アプリ内通知は、翻訳キーとparams(パラメーター)を保存し、選んでいる言語で表示します。メールやWhatsAppは、preview(プレビュー)またはsimulated(模擬)として扱い、実際に送信した実績としては扱いません。通知には、対象を参照するIDと、snapshot scope(通知時点の権限範囲)を持たせます。通知から画面へ遷移するときも、現在の権限を検証します。

## DDC-05 フロントエンド境界の完了条件

- [操作カタログ](operation-catalog.csv)にある各操作を、対応する画面の入力の型・戻り値・モックの結果に対応させます。
- UIは、モックの実体に直接依存しません。サービスのinterface(窓口)を通して、データを取得したり変更したりします。
- 成功・受付中・失敗・競合・閲覧不可、それぞれの合成した結果を使って、画面が期待どおりに表示され、回復することを検証します。
- 将来、実際のAPIに接続するときは、adapter(接続部分)を差し替えるだけで対応できる構成にしておきます。HTTP adapterの実装や検証、本番用の認証、サーバー側の処理設計は、今回の完了条件には含めません。

## DDC-06 画面構成とローカル状態

それぞれのrole page(役割ごとの画面)は、共通のShell(枠組み)とUI patternを使います。pageは、URL・Query hook・フォームを組み立てる役割を持ち、primitive(部品)がRepositoryを直接importすることはありません。RHFのフォームは、対象のIDをkeyとして別々に分離します。queryを再取得しても、まだ保存していない入力(dirty値)を自動でresetすることはありません。

ダイアログが開いているかどうか、選んでいるタブなど、短い時間だけ使う状態(state)は、最も近いcomponent(部品)が持ちます。対象・期間・一覧のページに関する情報はURLに、観測値や履歴はQueryに、業務データはRepositoryに、それぞれ1か所だけで保持します。意味のあるフィルターは、戻る操作をしたときに復元できるようにします。画面をまたいで、不要なContextを追加することはしません。

一括で取得するデータの取得に失敗した場合は、ページ全体のエラーとして表示します。独立した補助的なパネルだけが失敗した場合は、そのパネルの中だけでエラーを表示します。例えば、設備の基本情報の取得には成功したが、履歴の取得だけ失敗した場合は、設備の参照は続けたまま、履歴だけを再試行します。ただし、操作できるかどうかを判定するために必要なcapability(能力)や制限の情報が取得できない場合は、操作を有効にしません。

## DDC-07 共通画面の項目・処理

| 画面/操作 | 入力・初期値 | 処理と完了 | 失敗・取消 |
|---|---|---|---|
| /login | demoActorId:未選択、returnTo:任意相対route | モックのセッションに、既知のactor(利用者)のMembership(所属)を設定し、その役割のホーム画面へ移動します。実際のメールアドレスやパスワードは不要です | 選択していない場合や、知らないactorの場合は拒否します。外部のURLが指定されたreturnToは破棄します |
| /forgot-password | demoEmail:空、形式・最大254文字 | 登録されているアドレスでも、されていないアドレスでも、「該当する場合は案内を表示します」という同じデモの完了メッセージを表示します | 実際にメールは送信しません。形式が不正な場合は、項目のエラーとして表示します |
| /settings/preferences | locale=en、zone=Asia/Kuala_Lumpur、currency=MYR | locale(言語設定)はすぐに反映されます。zone(タイムゾーン)や通貨は、表示のための設定です。データや契約上の通貨は変わりません | 対応していないlocaleや、不正なIANAタイムゾーンは拒否します |
| 共通通知 | unreadOnly=false、cursor=null、limit25 | scope内の通知を、Page(ページ単位)で表示します。既読にする操作(mutation)ができます。参照IDから、関連する画面へ移動できます | scopeが失効したリンクは、安全な「利用できません」という表示にします |
| 音声/テキストパネル | text:空、intent:未解決、target:未選択 | intent(意図)を解決し、対象の候補を出し、変更の場合は確認したうえで、既存のCommandの仕組みを使います。照会(問い合わせ)は読み取りのみです | 認識できない場合、同じ名前が複数ある場合、確認を取り消した場合は、いずれもCommandを0件のままにします |
| /demo | scenarioId、eventType、clockAdvance、reset | 許可された合成イベントだけを使います。resetのときに、generation(世代番号)が更新されます | 任意のURL、スクリプト、実機宛てのデータは受け入れません |

共通操作の論理的な契約は、`demoSession.signIn/signOut/switchMembership`、`auth.previewPasswordReset`、`preferences.get/update`、`voice.resolveIntent`、`notifications.list/markRead`、`demo.trigger/reset`です。demo・auth・voiceは、1A(このフェーズ)ではモック専用であり、実際の認証への接続仕様は対象外です。preferences(設定)は、あまり変わらない表示設定用のProviderに置き、業務Repositoryのデータや請求で使う通貨とは分けて扱います。

### 能力権限の名称と付与先

| 能力 | 付与可能な役割・制約 |
|---|---|
| control.execute | 顧客とHQに付与できます。利用・管理できる範囲と、契約・機器の能力の範囲内に限られます |
| control.diagnose / device.maintain / alert.resolve | 技術者が、有効に担当している範囲内で使えます。外部の技術者は、作業期間内に限られます。解消するには、理由と根拠が必須です |
| job.manage / contract.manage / billing.manage | HQに付与できます。対象のテナント内に限られます |
| partner.accept / partner.assign / partner.review | 施工業者に付与できます。自社が委託した分だけです。reviewでは、同じuserIdによる自己承認は拒否します |
| identity.manage / device.manage | HQに付与できます。同じテナント内に限られます。自分自身に高い権限を付与することはできず、別の人が変更する必要があります |
| alert.policy.manage / automation.policy.manage / energy.manage | HQに付与できます。管理している対象の方針や分析に使います |
| restriction.manage / restriction.override | HQの中でも独立した能力です。通常のHQには、自動では付与しません |
| mrv.manage / offset.manage / audit.read | HQが個別に持つ能力です。顧客向けの分析や記録の閲覧とは分けて扱います |

role(役割)による候補の許可リスト(allowlist)と、permission(権限)の両方を判定します。知らない能力名は許可しません。「付与できること」と「最初から付与されていること」は別です。初期値(seed)の各actorには、明示的に決めた能力の集合だけを与えます。

### 補助操作の入力

Inquiry(問い合わせ)を作成するには、`invoiceId`または`restrictionId`、`subjectType`、`message`(1〜2000文字)が必要です。HQが回答するには、`inquiryId`、`reply`(1〜2000文字)、`expectedVersion`を使い、状態を`received`から`answered`に変えます。顧客は、その返信を読み取ることができます。閉じる操作は、1A(このフェーズ)では任意であり、実装していない場合は表示しません。

MRVのdraft(下書き)は、`mrv.saveDraft`を呼び出します。初回は、モックが新しいIDを返します。編集するときは、保持しているIDとバージョンを使って更新します。通信先や、データを保存する方式は、この文書では定めません。

監視や制御に必要なデータ取得と、操作カタログには、補助的なgetやlistの操作も含めます。UIに編集ボタンを置く場合は、対象のdetail(詳細)を読み取る契約と、保存する契約を対にします。

## DDC-原文補完. 企業要望に対応する表示モデル

一次資料(最初の情報源)はSRC-06です。以下は、フロントエンドの表示のために拡張した内容であり、APIの通信仕様ではありません。詳細・必須かどうか・値が欠けているときの処理は、それぞれのDDを正式なものとします。

| 既存の論理操作・結果 | 表示用拡張 | 利用設計 |
|---|---|---|
| alerts.list / Alert | causeCode、evidenceKind、evidenceText、observedAt | DD-C08、DD-T07、DD-A05 |
| telemetry.series / 空気環境モデル | allergenObservationの取得状態・対象物質・値・単位・出典・観測時刻 | DD-C07、DD-A12 |
| payments.simulate / Payment、invoices.list / 請求表示モデル | `Payment.method`を、Invoice表示モデルの`paymentMethod`へ変換します。値は`demo_credit_card` / `demo_debit_card` / `demo_instructions`のいずれかです。未選択や手動確認のときはnullにします。処理中や失敗の場合でも、種別はそのまま保持します | DD-C11、DD-A08 |
| MRVレポートプレビュー | Scope 2分類、組織・期間・拠点・地域係数・算定境界・品質 | DD-A14 |
| offsets.preview / OffsetQuote | marketConcept: future_concept・未選定・未検証・未接続 | DD-C13、DD-A15 |

上で説明した表示の変換処理は、feature(機能)ごとの純粋関数にまとめます。mock adapterは、正常・欠落・失敗、それぞれの合成した結果を返します。実際のAPIを導入するときは、adapterがこの画面モデルへ変換します。画面から、外部のAPIへ直接接続することはありません。

## DDC-08 複数資源・再訪・役割横断の契約

以下は、DEC-10に基づく1A(このフェーズ)の設計提案です。企業の商用ルールを確定するものではありません。それぞれのDDの入出力と、事後条件に適用します。すべての操作は、はっきり指定した資源のIDと、現在の`DemoViewContext`で認可します。画面が「今選択しているID」を、暗黙のうちに参照することはしません。カタログにある入力に加えて、サービスの第1引数には`context`を渡します。最後の引数であるread options（`{signal?: AbortSignal}`）には、`AbortSignal`を渡すことができます。更新用の`DemoWriteOptions`は、カタログに記載されている位置で渡します。

### 1. 読取・履歴・添付

| 操作 | 読取・更新の意味 | 権限・再訪時の扱い |
|---|---|---|
| policies.list/get | `Policy`は、id・version・kind(alert/automation/air_quality)・unitIds・enabledと、対応するDDの入力項目を持ちます。`get`は、保存されている現在のバージョンを返します | `alert`の場合は`alert.policy.manage`、`automation`と`air_quality`の場合は`automation.policy.manage`という権限が必要です。対象の範囲内でだけ閲覧・更新できます。再び訪れたときは、`get`でフォームを初期化します |
| jobs.get | `draftReportRef`と`reportRefs`は、それぞれ`reportId`と`reportVersion`の組です。まだ作成していないdraftはnullになります | 技術者が最初にdraftを作るときは、`jobId`を付けて`saveDraft`を呼び出し、IDを生成します。既存のdraftを再開するときは、`reports.get`を使います |
| reports.get | `jobId`・`reportId`・`reportVersion`のすべてが一致する`WorkReport`を返します。items/measurements/parts/workText/nextAction/attachmentRefsと、作成者・バージョン・提出や受理の情報を含みます | 有効に担当している人は、自分の案件のdraftや提出版を見られます。品質担当は提出版を見られます。顧客は、受理済みのバージョンだけを見られます。過去のバージョンは変更されません。外部への委託が失効した後は、JobHistorySnapshotだけが見られ、本文の取得は拒否されます |
| attachments.add | 作成済みのdraftの`jobId`と`reportId`に、JPEGまたはPNGを追加します。MIMEタイプ・内容・サイズ・枚数を検証し、Blob(画像データ)と`Attachment`を、同じモックの中で保持します | 技術者が有効に担当していて、かつ編集できるdraftのときだけ追加できます。`Attachment`は、id/jobId/reportId/blobId/name/mime/size/statusを持ちます |
| attachments.getContent | 指定した`reportVersion`に結び付いている`attachmentId`のBlob(画像データ)を返します。URLは返しません | `reports.get`と同じ公開の判定基準を使います。画面側で`URL.createObjectURL`を使って一時的なURLを作り、離脱や切替でrevoke(解放)します。再び訪れたときは、あらためて取得・再生成します |
| inquiries.list | 顧客には、自分の`invoiceId`や`restrictionId`に結び付いている`Inquiry`を返します。HQには、`billing.manage`の権限で管理できる範囲のものを返します | 顧客は、received/answeredの状態とreply(返信)を見られます。回答の通知にあるtargetIdから、同じ請求の画面を開き、inquiryIdで対象を選びます。別の顧客の回答を返すことはありません |
| devices.addResponseNote | `deviceId`と`eventId`に、`responseNote`(1〜1000文字)を、作成者・時刻・相関IDを付けて追記します | `device.maintain`の権限と、有効な担当であることが必要です。確認のメモを追加しても、接続状態やtamper(不正な取り外し)の状態は変わりません。復旧は、`/demo`の独立したイベントとして扱います |

画像の本体は、共有Repositoryの`Map<blobId, Blob>`に保持します。サインアウトや役割の切り替えをしても、共有しているBlobを消すことはありません。まだ保存していない、ローカルで選んだだけの画像は、画面を離れるときに破棄できます。draftから写真を削除する操作は、`jobs.saveDraft`の`attachmentIds`の差分で表します。提出済みの旧バージョンから参照されているBlobは、残しておきます。どこからも参照されなくなったBlobは解放します。resetのときは、すべてのBlobとすべての一時URLを解放します。提出に失敗しても、本文や保存済みの画像が失われることはありません。

`WorkReportDraft`の入力は、jobId、reportId(初回はなし)、items、measurements、parts、workText、nextAction、attachmentIdsです。初回の応答には、reportIdとversionが返されます。更新するときは、`expectedVersion`が必須です。ドラフトの間は、まだ完了していない項目があっても構いません。提出(submit)のときに、それぞれのDDにある必須項目を検証します。提出済みのバージョンや、過去のバージョンを直接編集することはありません。再作業や再割当のときは、過去のバージョンを参照して、新しいdraftのバージョンを作ります。過去のバージョンと、それぞれの記録の作成者は、そのまま維持します。

### 2. 試運転

```ts
type CreateDiagnosticRunInput = {
  jobId: string;
  unitId: string;
  startAction: UnitAction;
  endAction: UnitAction;
  durationMinutes: number; // 整数1〜15
  reason: string;          // 1〜1000文字
  expectedUnitVersion: number;
  expectedJobVersion: number;
};
type DiagnosticRun = {
  id: string; tenantId: string; version: number;
  jobId: string; unitId: string; actorMembershipId: string;
  startAction: UnitAction; endAction: UnitAction;
  durationMinutes: number; reason: string;
  state: 'awaiting_start' | 'running' | 'end_requested' |
         'completed' | 'start_failed' | 'end_failed' | 'end_blocked';
  startCommandId: string; endCommandId: string | null;
  startedAt: string | null; endAt: string | null;
  failureCode: string | null;
};
```

通常の1回だけの診断操作は`commands.create`を使います。時間を指定した試運転は、`diagnosticRuns.create`がrunと開始のCommandを作り、`get`が同じrunを返します。`create`のときには、両方のactionについて、能力・制限・`control.diagnose`・有効な担当・online(通信可能)であること・ファームウェアが更新中でないことを検証します。同じ設備で、進行中のrun(`awaiting_start`/`running`/`end_requested`)があるか、まだ完了していないCommandがある場合は、`CONFLICT`になります。`start_failed`/`end_failed`/`end_blocked`は、履歴として残る終了状態であり、状態をあらためて確認した後に、明示的な復旧操作を行うことは妨げません。開始と終了のCommandは、どちらも`diagnosticRunId`を持ちます。

| 現状態 | イベント | 次状態・結果 |
|---|---|---|
| awaiting_start | 開始Commandが期限内acknowledged | `running`(実行中)になります。`startedAt`は応答した時刻です。`endAt`は`startedAt + durationMinutes`です |
| awaiting_start | 開始失敗・30秒期限超過 | `start_failed`(開始失敗)になります。終了のタイマーは動きません。開始済みとは表示しません |
| running | now>=endAt、元の担当・権限・現能力・制限・onlineを再確認して終了要求 | `end_requested`(終了要求中)になります。`endAction`のCommandを1件作成します |
| running | 終了時に担当失効・権限剥奪・能力/制限変更・FW競合 | `end_blocked`(終了ブロック)になります。要求は0件のままで、「未停止・要確認」という注意を、HQと技術者へ表示します |
| running | 終了時にoffline | `end_failed`(終了失敗)になります。「未停止・要確認」という表示と、再照会するための導線を出します。成功したという表示はしません |
| end_requested | 終了Commandの期限内acknowledged | `completed`(完了)になります。応答した`endAction`の観測値を表示します。「停止済み」と表示するのは、`endAction`が電源OFFの場合だけです |
| end_requested | 失敗・期限超過 | `end_failed`(終了失敗)になります。終了を確認できなかったことと、Commandの履歴を保持します |

`create`のときには、「現在時刻 + 30秒 + duration < 担当のvalidUntil」であることを求め、通常の実行中に担当期限が切れてしまう予定は拒否します。それでも、途中で権限が失効したり剥奪されたりした場合は、終了のときにあらためて判定します。終了できなくなった場合の復旧は、現在の権限を満たしている担当が、明示的な診断操作を行うことで行います。自動での再送はしません。過去のrunの失敗履歴を、後から`completed`に書き換えることはありません。

タイマーは、画面が持つのではなく、共有しているモック時計の購読として保持します。サインアウト・画面を離れる・役割の切り替えは、閲覧のための要求だけを中断します。すでに受理されたrunの業務イベントは、そのまま続きます。実行の主体は、作成時のMembership(所属)を使って現在の権限を再評価します。切り替え先の主体に置き換えることはしません。resetのときは、Repositoryのgeneration(世代番号)を変えるだけで、過去のrunと過去のイベントを破棄します。

### 3. 手動入金と複数請求

- `payments.confirm(paymentId, ...)`は、既存の`processing`(処理中)のPaymentを`confirmed`(確定済み)にします。`payments.recordManual(invoiceId, ...)`は、Paymentがないか、失敗の履歴だけがある`unpaid`(未入金)の請求に対して、`method=null`の`confirmed`なPaymentを新しく作成します。どちらの操作にも`billing.manage`の権限が必要です。
- `recordManual`の金額は、請求の全額でなければなりません。通貨は、請求の通貨と一致させます。`reason`は1〜1000文字、`paymentReference`は1〜128文字です。すでに`processing`のPaymentがある場合は`CONFLICT`とし、結果が確定した後にあらためて照会します。`paid`(支払い済み)の請求に対して、新しい入金を作ることはありません。
- 同じtenant・invoice・referenceで、同じ金額・通貨の確認が再び来た場合は、`expectedVersion`の判定より先に、既存の成功結果をそのまま返します。参照(reference)を別の請求に流用したり、同じ参照で金額を変えたりした場合は`CONFLICT`になります。新しい意思として別のキーを使っても、二重の入金は拒否します。
- Paymentの`confirmed`、Invoiceの`paid`、監査・通知・関連する制限の評価は、同じモックの遷移処理でまとめて更新します。顧客のカード決済が成功したイベントも、同じ確定用の関数を使います。決済のシミュレーションと手動での確認が競合した場合は、どちらか一方だけが確定します。
- Invoiceの`paymentMethod`と`paymentStatus`は、最新のPaymentの`method`と`status`から導き出します。カード決済が失敗した後に手動入金が確定した場合、現在の方法(method)はnullになりますが、過去のカードの種別は履歴に残ります。`paymentRefs`は、id・version・method・status・reference・confirmedAtを、表示に必要な範囲で返します。

制限を作成するときの`causeInvoiceIds`は、同じ契約にある、期限を超過していてかつ未入金の請求をすべて列挙します。空の集合は拒否します。作成時には、モック側で再計算し、入力した集合と一致することを確認したうえで、以後はその内容を固定します。契約・顧客・通貨を照合し、別の契約の請求を混ぜることはありません。`restrictions.forInvoice`は、この集合に含まれているかどうかで検索します。

原因となった請求のすべてが`paid`(支払い済み)になったときだけ、自動的に`scheduled`を`cancelled`に、`requested`/`applied`を`release_requested`に進めます。1件だけ入金があった場合は、適用の状態をそのまま維持し、原因請求のうち未入金の件数を表示します。`processing`(処理中)は`paid`とは扱いません。後から発行した請求を、既存の予告に自動で追加することはありません。

1A(このフェーズ)では、1つの設備につき、進行中の制限(scheduled/requested/applied/release_requested)を1件だけに限定します。重なる新しい`schedule`は`CONFLICT`として拒否します。既存の制限が`cancelled`または`released`になった後であれば、新しい原因請求に対して、別のIDで予告を作ることができます。手動での解除には`restriction.override`、適用後の猶予や例外にともなう解除には`restriction.manage`という権限で許可する、という提案です。どちらの場合も、Invoiceを変更することはありません。release(解除)の操作は、未入金が残っている場合は拒否します。例外操作の権限経路とは混同しないでください。

### 4. 制限Commandと解除の観測値

```ts
type RestrictionPolicy =
  | { kind: 'temperature_limit'; minimumCoolingSetpoint: number }
  | { kind: 'power_off' };
type RestrictionAction =
  | { kind: 'apply_restriction'; restrictionId: string;
      rulesVersion: string; policy: RestrictionPolicy }
  | { kind: 'remove_restriction'; restrictionId: string;
      rulesVersion: string };
```

`RestrictionAction`は、共有モックのrestriction(制限)の遷移処理だけが作る内部用のactionです。`commands.create`、音声操作、顧客のautomation(自動化)、診断からは受け付けません。`Command.action`は、`UnitAction`か`RestrictionAction`のどちらかです。機器の確認状態には、`observedRestriction: {restrictionId, rulesVersion, policy} | null`と、確認した時刻を持たせます。これは、通常の設定値とは分けて扱います。

| 応答したaction | 確認済みの状態変化 | 変化させない値 |
|---|---|---|
| temperature_limit適用 | `observedRestriction`へ保存します。設定温度が下限未満であれば、下限まで上げます。下限以上であれば、そのまま維持します | 電源の状態と室温は変更しません |
| power_off適用 | `observedRestriction`へ保存し、確認済みの電源をOFFにします | 設定温度と室温は変更しません |
| remove_restriction | 対象のIDとバージョンの`observedRestriction`をnullにします。通常の操作を、再び許可します | 自動でONにしたり、適用前の温度に戻したりすることはしません。解除した時点の電源と設定温度を、そのまま維持します |

応答が来る前は、観測値を変更しません。ただし、適用要求中や解除要求中であっても、通常操作のpolicy(方針)では制限を保持し、回避されないようにします。解除は、同じ`restrictionId`・バージョン・Command IDへの成功応答と、観測値がnullであることの両方を照合して、設備ごとの完了とします。すべての設備で完了したときだけ`released`になります。適用要求がまだ確定していない場合は、解除したいという意思だけを保持しておき、適用の結果を照会した後で`remove`を送ります。遅れて届いた適用のイベントによって、`release_requested`を`applied`に戻すことはありません。

### 5. 用語と入力型の共通規則

正式な名前は、`UnitAction`、`parentSpaceId`、`planType=rto`、`channel=inApp/email/whatsapp`、証明の参照に使う接頭辞`DEMO-`です。役割を表す識別子は、次の表を正式なものとし、それ以外はすべて翻訳のためのラベルです。

| 役割 | role enum | route prefix | permission prefix | 組織kind | 日本語正式名（略記） | 英語表示 |
|---|---|---|---|---|---|---|
| クライアント | client | /customer | control.* | customer | クライアント（顧客） | Client |
| 施工業者 | contractor | /partner | partner.* | contractor | 施工業者（業者） | Contractor |
| 技術者 | technician | /technician | control.diagnose, device.maintain, alert.resolve | contractor または operator | 技術者（社内/外部） | Technician |
| 管理者 | admin | /admin | job/contract/billing/identity/device/restriction/mrv/offset/audit/*.policy | operator | 管理者（HQ） | Admin / HQ |

企業向けの文書(PrepareDocument)では正式名を使います。要件や設計本文にある「顧客」「業者」「HQ」は、同じ役割の略した呼び方として扱います。routeやenumの値を、表示名から推測してはいけません。preview/simulated/failedは`deliveryState`(配信状態)として表し、channelの値には混ぜません。UIに出てくる「RTO」「顧客」「HQ」は翻訳のためのラベルであり、保存用のenumとは分けて考えます。

それぞれの`*Input`は、対応するDDの入力欄を、名前付きのプロパティとして持ちます。save(保存)操作では、新規の場合はidを省略し、既存の場合はidが必須です。既存のものを更新するときは`expectedVersion`が必須です。`PolicyInput`は`kind`を判別子にし、DD-A05・DD-A11・DD-A12を、それぞれ別のschemaにします。`AutomationInput`は、`schedule`(DD-C04)と`event`(DD-C05)を`kind`で区別し、共通して`id?`/`name`/`unitIds`/`timezone`/`enabled`/`priority`(既定値50)を持ちます。`event`の条件は、`occupancy={occupied:boolean}`、`location={event:arrival/departure}`、`pattern={localTime:HH:mm}`、`weather={metric:temperature_c,operator:gt/gte/lt/lte,value:number}`です。HQの条件は、`occupancy`は同じ形で、`tariff={operator,value,unit:MYR_per_kWh}`、`peak={active:boolean}`、`solar/battery={operator,value,unit:kW}`というデモに限ります。値が欠けている場合を、条件が成立したとは扱いません。

`ListQuery`の`filters`と`sort`は、それぞれのDDにある検索項目名に限定します。追加できる共通のfilterは、tenantに由来するscope、resource ID、kind、enabled、from/toです。sortは、そのモデルが`createdAt`/`updatedAt`/`name`/`status`のいずれかを持っている場合だけ、`asc`または`desc`で指定できます。既定はid昇順です。知らないキーを指定すると`VALIDATION`エラーになります。任意のsort式を自由に受け入れることはしません。


### 6. 自動運転・方針の発火

`automations.simulate`は読み取り専用です。対象のルールや方針と、合成したイベントから、採用するかどうか・抑止するかどうか・その理由(`SimulationResult`)を返します。Command・監査・通知は作りません。実際に発火させるのは`automations.fire`(`DemoWriteOptions`が必須)です。`fire`は同じ評価関数を使い、採用されたactionごとに、`commands.create`と同じpolicy(能力・制限・同意・担当・online・未完了のCommand)を通してCommandを作成し、`FireResult{decision, reasons, commandIds[]}`を返します。抑止された場合、値が欠けている場合、同意が取り消されている場合、制限中の場合は、`commandIds=[]`と理由を返しますが、`FORBIDDEN`にはしません。デモ時計による予定イベント、HQの方針の評価、換気の方針(`ventilation=true`の対象だけ`ventilate`にする)は、すべて`fire`を経由します。UIや`/demo`が、直接Commandを作ることはありません。同じAutomationや方針で、同じイベント時刻の再送は、冪等キーによって1回だけ発火します。

## DDC-09 共通の監査・試験規則

業務上の変更は、action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAtを記録します。データの取得に成功しただけでは、業務の履歴を増やしません。アクセスが拒否された場合は、内容をマスク(伏せた状態)にして記録します。通知先や公開のタイミングは、この文書の「通知と公開範囲」に従います。読み取り専用の画面では、mutation(データ変更)は行いません。入力フォームがある操作だけ、必須項目・文字数・境界値を検証します。保存する直前の認可の確認とバージョンの照合、失敗したときの回復方法は、DDC-03の規則を共通して適用します。

MRVの確認は、同じ`reportId`・`reportVersion`・同じコメントで再送された場合は、既存の結果をそのまま返し、履歴は追加しません。異なるコメントで、同じ確認済みのバージョンを変更しようとした場合は`CONFLICT`になります。バージョンが変わった後の報告は、あらためて確認します。

顧客のメモは`visibility=customer`だけを指定できます。業者のメモは`internal`か`customer`のどちらかを指定できます。`notifications.preview`の`recipientRole`(通知の受け取り手の役割)は、`jobId`の関係者から決めます。`internal`の場合は、`customer_contact`(顧客の連絡先)への公開を拒否します。任意のアドレスや、請求レコードの参照を書き写すことは許可しません。自由な文章の内容を完全に自動で検出できる、という保証はしません。デモでは、架空のデータだけを使用します。
