---
document_id: DD-COMMON
version: 0.7.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# 共通詳細設計書

入力として使うもの: [共通要件](../01-requirements/common.md)、[PrepareDocument](../00-prepare/PrepareDocument.md)。実装で使う細かい入出力・エラー・デモ時間の値は[フロントエンド入出力契約](implementation-contracts.md)を、すべての論理操作(処理の名前の一覧)は[操作カタログ](operation-catalog.csv)を、あわせて読んでください。この文書が決めるのは、ブラウザ側の画面データ・モックサービス(仮のデータ処理)・表示状態だけです。APIのパス、データベース、認証サーバー、バックエンド(サーバー側)の業務処理は、この文書の対象ではありません。

この文書の業務目的は、[企業要件原文](../00-prepare/sources/company-requirements-original.txt)から整理したBIZ項目と、[共通要件](../01-requirements/common.md)にもとづきます。型・Repository(データを取り出す仕組み)・キャッシュ(一時保存)・権限ガード(アクセス制限)・モック状態は、その目的をフロントエンドで実現するための設計案です。将来、実際のAPIを導入するときは、表示用データへの変換をadapter(変換の仕組み)にまとめます。サーバー側の認証・データベース・通信の取り決めは、この文書では決めません。

設計の対象になる機能・画面項目・状態・例外は、企業原文と対応する要件をもとにします。各FR(機能要件)を満たすための処理と、受入条件(合格の基準)をこの文書で決めます。参考にしているモック画面は、共通UIの見た目を考えるためだけに使います。

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

データの依存する向きは、`page(画面) → feature hook(機能ごとの処理) → Repository interface(取り出し方の約束事) → injected adapter(差し込む変換の仕組み)`という順番です。domain(業務のルールを扱う層)は、React・HTTP通信・モックのどれにも依存しません。ページから、fetch(通信)・mock seed(初期データ)・localStorageを直接使ってはいけません。composition-root(組み立て役)だけが、どのadapterを使うかを選びます。

いま提案している構成(PROPOSED)は、TypeScript(strictモード)、React、Vite、React Routerです。サーバー側でHTMLを組み立てる必要(SSR)がないので、SPA(1つのページで動くアプリ)として作ります。URLを直接開く「ディープリンク」に対応するには、将来、ホスティング側でSPA用のフォールバック設定が必要です。使うライブラリのバージョンは、実装を始めるときに互換性を確認し、lockfile(バージョン固定ファイル)に固定します。アプリを起動するコマンドはまだ作っておらず、この文書では「動作確認済み」とは書きません。

## 2. 共通ルートと画面状態

| ルート | 責務 |
|---|---|
| /login | 架空のアカウントと4つの役割を選ぶ画面。「本物の認証ではない」ことを表示する |
| /forgot-password | メールの形式を確認したあと、「そのメールが存在するかどうか」は教えない案内を出す。送信はプレビューのみ |
| /settings/preferences | 言語、表示するタイムゾーン、デモ用の通貨、同意の設定 |
| /notifications | 自分の担当範囲内の通知一覧と既読の管理。業務の状態そのものとは別 |
| /demo | シナリオの切り替え、失敗イベントの発生、デモ用の時計、リセット。デモ専用の画面 |
| /forbidden、未定義のルート | アクセスできないとき、ページが見つからないときの表示。許可されている範囲のホーム画面へ戻す |

共通ヘッダーには、言語切り替え・通知・音声/テキストの切り替え・サインアウトを置きます。デモ用の役割切り替えは、専用のメニューにして、通常業務での「ユーザー変更」とは分けます。

画面の状態表示は次のようにします。

- loading(読み込み中): 骨組みだけの表示
- empty(データなし): 説明と、その人が次にできる操作
- error(エラー): 再試行のボタン
- offline(通信できない): 最後に更新した時刻と、操作できない理由
- stale(古いデータ): 「これは古い値です」という注記

見る権限がないだけなのに、empty(データなし)のふりをして誤魔化してはいけません。404(見つからない)と403(権限なし)は、「本当は存在するかどうか」を悟られない同じような文言にそろえます。

## 3. フロントエンドの画面・モック用モデル

以下はデータベースのテーブル定義ではありません。「保存」「履歴」と書いているものは、すべて同じブラウザのタブの中にある「共有モックメモリ」という仮データの置き場に保持することを指します。

共通の決まりは次のとおりです。

- `id: string`(識別番号)、`tenantId: string`(テナントの識別番号)
- 時刻はすべてISO 8601形式のUTC(協定世界時)
- 変わりうるデータには`version: number`(バージョン番号)をつける
- デモ用のIDも、あとから値を変えない

null(値がない)と「未登録」は、schema(データの形式ルール)で意味を分けます。知らないenum(選択肢の値)が来たときは、unsupported(対応していない)として安全に扱います。

| エンティティ | 主なフィールドと関係 |
|---|---|
| User / Membership | userId、organizationId、role(client/contractor/technician/admin)、employment(internal/external/null)、permissions[]、scopeIds[]、validFrom、validUntil。Userにひとつのroleだけを直接書き込まない |
| Organization / Customer | name、kind(customer/contractor/operator)、status / organizationId、serviceProfile。顧客のIDとUserのIDを分け、作成するときも管理しているテナントの中に限る |
| ContractorOrganization / Assignment | contractorOrgId、jobId、technicianMembershipId、delegatedScope、validFrom/Until、status。委託した側のtenantと、受託した会社のIDを分ける |
| Property / Space | customerOrgId、kind(home/office)、name / propertyId、parentSpaceId、kind(area/floor/room/space)。親子の階層が循環しないようにする |
| ACUnit / Capability | spaceId、manufacturer、model、type(split)、installedAt、serviceScope / temperature(min,max,step)、modes[]、fanLevels[]、ventilation、control、sensors[] |
| Device / Sensor | unitId、serial、connection(online/offline/unknown)、lastSeenAt、firmwareVersion / deviceId、metric、unit、calibrationAt、staleAfterSeconds |
| Telemetry | unitId、sensorId?、metric、value:number\|null、unit、observedAt、receivedAt、origin(measured/estimated/inspection)、quality(valid/missing/stale/suspect)、isDemo |
| Command | unitId、actorId、action:UnitActionまたは内部のRestrictionAction、diagnosticRunId?、status、requestedAt、sentAt?、acknowledgedAt?、expiresAt、failureCode?、idempotencyKey(重複防止のキー)、expectedVersion、correlationId |
| Alert | unitId、type、severity(critical/warning/normal)、status、evidenceIds[]、detectedAt、acknowledgedAt?、resolvedAt?、resolutionReason?。normal(正常)は健康サマリーにも使う。open(未対応)のアラートを、勝手に正常へ変えてはいけない |
| MaintenanceJob | unitId、alertIds[]、type(periodic/reactive/preventive)、status、contractorOrgId?、assignmentId?、requestedSlot、scheduledSlot?、dueAt、reportVersion?、draftReportRef?、costs[] |
| WorkReport / InspectionItem / Attachment | jobId、authorId、version、items[]、measurements[]、replacementParts[]、workText、nextAction、submittedAt? / componentGroup、componentKey、result、reason?、evidenceIds[] / jobId、reportId、blobId、name、mime、size、status(previewUrl(プレビュー用URL)は画面上でだけ作る) |
| Contract / Invoice / Payment | customerOrgId、unitIds[]、planType、period、rulesVersion / contractId、amountMinor、currency、dueAt、status / invoiceId、amountMinor、method?、status、externalRef?、confirmedAt? |
| Restriction | contractId、causeInvoiceIds[]、unitIds[]、rulesVersion、noticeAt、executeAfter、reason、policy、state、applyCommandIds[]、releaseCommandIds[]、exception?、graceUntil? |
| Automation / Consent | unitIds[]、condition(discriminated union)、action、priority、timezone、enabled / purpose、granted、grantedAt?、revokedAt? |
| Notification / AuditEvent | recipientId、channel(inApp/email/whatsapp)、templateKey、params、readAt?、deliveryState(preview/simulated/failed) / actorId、action、targetId、before?、after?、reason?、result、correlationId、occurredAt |
| EnergyBaseline / EmissionFactor | unitIds[]、period、method、version、kWh、boundary / region、year、kgCO2ePerKWh、source、version、isDemo |
| MRVReport / OffsetRecord | period、baselineId、factorId、boundary、coverage、totals、evidenceIds[]、reviewHistory[]、status(draft/demo_reviewed) / amountKg、state(quoted/demo_requested/demo_purchased/demo_retired/failed)、demoCertificateRef?、isDemo |

上の表は、共通データの要約です。企業原文を補ってできた「原因の候補」「アレルゲン」「支払い方法」「Scope 2」「市場構想」の項目は、[表示モデルの補完](implementation-contracts.md#ddc-原文補完-企業要望に対応する表示モデル)と、それぞれ対応するDD(詳細設計)を合わせて確認してください。Invoice(請求)の表示データは、Payment(支払い記録)からpaymentMethod(支払い方法)/paymentStatus(支払い状況)を計算して出すもので、同じ情報を二重に保存しません。

Telemetry(測定データ)の`isDemo`は、「測定した」か「推定した」かとは別の区分です。デモの「実測」も、実は作り物の値であることが伝わるように表示します。連絡先・写真は架空のものだけを使います。画像そのものは、共有モックの中にあるBlobストア(画像データの置き場)に保存し、Attachment(添付ファイル)はblobId(画像データの識別番号)で参照します。画面が作るobject URL(一時的な画像表示用のURL)は、画面から離れたとき・役割を切り替えたとき・サインアウトしたときに解放(revoke)し、もう一度表示するときは権限を確認したうえで作り直します。提出済みのBlob(画像データ)は、サインアウトでは消さず、reset(リセット)のときに解放します。詳しくは入出力契約DDC-08を参照してください。

## 4. フロントエンドのデータサービス境界

`page(画面) → feature hook(機能ごとの処理) → Repository interface(取り出し方の約束事) → mock adapter(仮データへの変換)`という順で分けます。ここでいうRepositoryは、フロントエンドから呼び出す非同期のサービスをまとめた考え方であり、データベースへのアクセス層ではありません。今回作るのは、このinterface(約束事)と、共有メモリを使うmock adapter(仮データの変換部分)だけです。

```ts
interface CommandRepository {
  create(context: DemoViewContext, input: CreateCommandInput,
      options: DemoWriteOptions): Promise<ServiceResult<Command>>;
  get(context: DemoViewContext, commandId: string,
      signal?: AbortSignal): Promise<ServiceResult<Command>>;
}
```

`DemoViewContext`は、今選んでいる架空のユーザー・役割・見られる範囲を表します。`DemoWriteOptions`は、同じデモ操作を二重に実行しないためのキーと、変更前のバージョンを持ちます。これらは、本番の認証方法やサーバー側の権限の仕組みを決めるものではありません。具体的な項目は、[フロントエンド入出力契約](implementation-contracts.md)を見てください。

[操作カタログ](operation-catalog.csv)には、画面が必要とする119個のローカルサービス操作と、それぞれの入力・戻り値・使われる画面をまとめています。URL、HTTPのメソッド、データベースのテーブル、サーバー側のトランザクションは、ここでは決めません。

モックの操作結果は、次のどれかとして返します。

- success(成功)
- pending(受付中)
- failed(失敗)
- conflict(競合)
- forbidden(閲覧・操作不可)

画面は、返ってきたDomainError(業務エラーの種類)に応じて、表示・入力内容の保持・再読み込みのどれをするか決めます。遅れて届いた古い応答は、操作IDと表示の世代(バージョン)で判定して無視します。役割を切り替える前の値を、あとから描画してはいけません。

将来、実際のAPIが用意されたときも、画面が使うinterface(約束事)はそのまま残し、新しいadapter(変換の仕組み)で外部からの応答を画面用のデータに変換します。実際のAPI仕様・認証方式・通信の取り決めは、今回は決めません。

## 5. 状態遷移と整合性

### コマンド

| 現状態 | イベント・ガード(条件) | 次状態 / 表示 |
|---|---|---|
| 未作成 | 権限・能力・契約上の制限・オンラインかどうかを確認してから送信する | requested(要求済み) / 「要求を受け付けました」と表示。この時点では確認済みの値は変えない |
| requested | 配信イベントが起きる | sent(送信済み) / 「機器の応答待ち」と表示 |
| requested / sent | はっきりした失敗、または応答が期限内に来ない | failed(失敗) / expired(期限切れ)。要求した値を、勝手に確認済みの値にコピーしない |
| sent | 一致する成功の応答が、期限内に届く | acknowledged(確認済み)。応答にもとづく設定値だけを更新する |
| failed / expired | ユーザーが状態をもう一度確認してから、再試行する | 新しい要求を作る。古いCommand(命令)は履歴として残す |

期限が過ぎたあとに届いた応答は、「遅れて届いたイベント」として履歴に記録します。状態を黙って成功に書き換えてはいけません。本当の状態は、もう一度確認してから別に表示します。通信が切れている間は新しい要求を拒否します。送信したあとに通信が切れた場合は、期限が来るまで応答を待ちます。

### 保守案件（施工業者を分けて扱う）

| 現状態 | 許可される人・イベント | 次状態 |
|---|---|---|
| requested(依頼済み) | HQが社内の担当者に割り当てる | assigned(割当済み) |
| requested | HQが外部の業者に委託する | offered(委託の申し出) |
| offered | 対象の業者が、期間内に受諾または辞退する | accepted(受諾) / requested(辞退の履歴を残す) |
| accepted | 業者が、自社の有効な技術者に割り当てる | assigned |
| assigned | 担当の技術者が、期間内に作業を開始する | in_progress(作業中) |
| assigned / in_progress | HQ(社内の場合)、または受託した業者(外注の場合)が、理由をつけて担当を変える | 状態はそのまま。古い割当は無効にし、新しい割当を作る。作業の途中経過の記録は、もとの作成者のまま残す |
| in_progress | 担当者が、必要な報告を提出する | submitted(提出済み) |
| submitted | 業者の品質担当(外注の場合)、またはHQ(社内の場合)が確認する | completed(完了) / rework_requested(やり直し依頼) |
| rework_requested | 担当者が、やり直し作業を開始する(`jobs.resumeRework`) | in_progress(前の報告バージョンは残す) |
| requested | 保守を依頼したクライアントが取り消す | cancelled(取消) |
| offered / accepted / assigned | HQが理由をつけて取り消す | cancelled。関連する割当も無効にする |
| in_progress / submitted | HQが理由をつけて中断する | on_hold(保留)。自動的に完了や取消にはしない |
| on_hold | HQが今の状況を確認して再開する(`jobs.resumeHold`、理由が必須)、または終了する | in_progress / cancelled(理由と未完了の記録が必須) |

completed(完了)になったあとの追加作業は、新しいjobId(依頼ID)を作って別の依頼として扱います。担当を変えたあとの新しい担当者は、今のdraft(下書き)から新しい報告バージョンを作って作業を続け、古いバージョンや各点検のもとの作成者は変更しません。外注の報告を、自分で自分の作業を承認することはできません。同じuserId(ユーザーID)の人が、Membership(所属)を切り替えて別の担当者として承認することも禁止します。施工業者の品質担当がいないときは、HQにエスカレーション(引き上げて対応)します。顧客が正式に最終承認する仕組みは、OPEN-01でまだ決まっていないため、今回(1A)では結果を見ることと問い合わせまでにとどめます。

Alert(異常通知)は、open(未対応)→acknowledged(確認済み)→resolved(解消)という順に進みます。resolved(解消)のあとにまた同じ異常が起きたときは、新しいAlertとして作り、previousAlertId(前のAlertとの関連)で紐づけます。resolved(解消)にするには、もう一度測定して条件を満たすか、HQまたは許可された診断者が理由をつけて確認する必要があります。Job(依頼)がcompleted(完了)になっただけで、自動的にresolved(解消)へは進めません。

### 支払い・制限

Payment(支払い)は、initiated(開始)→processing(処理中)→confirmed(確認済み)/failed(失敗)という順に進みます。Offset(排出量の相殺)は、quoted(見積済み)→demo_requested(申込済み)→demo_purchased(購入済み)→demo_retired(償却済み)という順に進み、失敗したときはfailed(失敗)として直前の状態を記録します。購入する前の償却や、二重の償却は拒否します。Invoice(請求)は、unpaid(未入金)→processing(処理中)→paid(支払い済み、入金確認時)という順に進みます。失敗したときはunpaid(未入金)に戻します。遅延しているかどうかは、dueAt(期限)と未入金額から計算します。今回(1A)は、分割払いと返金を対象にせず、1B(次の段階)で決めることとします。

| 現状態 | イベント・ガード(条件) | 次状態 / 注意点 |
|---|---|---|
| 未作成 | 制限できる契約であること、未払いであること、権限、理由、予告があること | scheduled(予定) |
| scheduled | 期限が来て、猶予や例外がなく、未払いであることをもう一度確認する | requested(要求済み)、設備ごとに適用のCommand(命令)を作る |
| scheduled | 原因になった請求がすべて入金確認された、または取消・猶予・例外になった | cancelled(取消) / scheduled(予定日を変える、または適用を保留し、理由をつける) |
| requested | 対象設備から適用の応答がある | 全台成功ならapplied(適用済み)。一部が未応答ならrequestedのまま、設備ごとの状態を表示する |
| requested | オフライン・失敗・期限切れ | requestedのまま、pendingReason(保留の理由)とcommandの結果を保持する。自動で成功にしたり、再送したりしない |
| requested / applied | 原因になった請求がすべて入金確認された、または権限のある人が手動で解除する | release_requested(解除要求)。適用要求との競合を確認し、解除する意思を記録する |
| release_requested | すべての対象で解除が確認できた | released(解除済み) |
| release_requested | オフライン・失敗 | 保留の表示。もう一度確認するか、はっきり再試行する |

requested(要求済み)以降に取り消したときは、「まだ何も適用されていない」と決めつけず、解除の流れに進めます。applied(適用済み)のあとに猶予・例外になった場合も、必要なら解除の要求を作ります。解除を要求したあとに、遅れて届いた「適用できた」という応答で、状態をapplied(適用済み)に戻してはいけません。各設備の観測値と、解除の要求内容をもう一度照合します。override(強制的な変更)は、支払いの記録そのものは変更しません。

Command(命令)とRestriction(制限)、Job(依頼)とAlert(異常通知)、Invoice(請求)とPayment(支払い)は、それぞれ別の記録として扱います。イベントが起きたときに関連する記録を更新しますが、1つの状態にまとめてしまってはいけません。画面では、一部だけ成功した状態、保留中の状態、実際の反映状況を、それぞれ分かるように説明します。

### デバイス・自動運転

ファームウェア更新の処理(Operation)は、queued(順番待ち)→running(実行中)→succeeded(成功)/failed(失敗)という順に進みます。オンラインであること・能力・対象のバージョンを確認し、succeeded(成功)したときだけfirmwareVersion(ファームウェアのバージョン)を更新します。校正(キャリブレーション)は、参照した値・単位・時刻・作業者をセットにした履歴として残します。制御の操作と更新が同時に起きたときは、デモの方針として、更新中の制御操作は拒否します。

自動運転が優先する順番は、次のとおりです。

1. 機器の能力・制限・例外の適用結果
2. HQの方針
3. 顧客のルール

同じ層の中では、priority(優先度)の数値が大きいほうを優先し、同じ値ならID順(小さいほうから)にします(DEC-09)。条件のデータが取得できていないとき、または同意が取り消されているときは、自動運転を発火させません。最終的にはどの経路でも同じCommand policy(命令のルール)を経由するため、音声操作や自動運転を使って制限を迂回することはできません。

## 6. Queryとデモ状態

Query(データ取得)のキーは、`[tenantId, membershipId, scopeVersion, resource, id, normalizedFilters]`という組み合わせです。役割を切り替えたとき、またはサインアウトしたときは、古いリクエストと購読を中断し、キャッシュ(一時保存データ)を消してから、次の画面を表示します。権限が変わったときは、scopeVersion(担当範囲のバージョン)を更新します。

モックのRepository(データ管理の仕組み)は、同じブラウザタブの中に1つだけ存在します。正規化されたMap(データの一覧)とイベントの列を持ち、画面のuseState(画面ごとの状態)には複製しません。更新のイベントが起きたら、jobs/units/invoices/restrictionsなど、関係するQueryを無効化して取り直します。例えば、S02という報告完了のシナリオでは、job(依頼)と履歴を更新しますが、Alert(異常通知)の解消は別のイベントが来るまで待ちます。

画面を再読み込みすると、最初のデモデータ(seed)に戻ります(DEC-07)。ログアウトすると、閲覧していたキャッシュや、まだ保存していない写真は消えますが、みんなで共有している架空の業務データは残ります。リセットの操作をすると、デモ用の時計・遅延の処理・購読・画像のURL・キャッシュ・業務データを、まとめて最初の状態に戻します。リセットする前に起きた「遅れて届くイベント」は、世代番号を使って無視します。

時計・IDの生成・成功や失敗の結果は、あとから差し替えられるようにします。乱数や実際の時刻に依存するテストは避けます。失敗・通信断・機器の取り外し・支払い・応答は、/demo画面からはっきり発生させられるようにします。テスト用に、次のような架空データを用意します。

- テナント: 2つ
- クライアント: 2つ
- 施工業者: 2つ
- 社内の技術者: 1人、社外の技術者: 2人
- HQ: 通常権限と、制限操作の権限がある人、それぞれのMembership(所属情報)

## 7. 算定・単位・データ品質

- 電力量[kWh]は、電力[kW]を時間で積み上げた値です。デモで時系列データから計算するときは、サンプルの間隔と、欠けているデータを除く条件を決めておきます。W(ワット)からの変換をするときは、その旨をはっきり示します。
- 推定の料金は「電力量×仮の単価」です。時間帯ごとに料金が違う場合は、それぞれの区間を合計します。税金や基本料金を含まないデモの場合は、その範囲を表示します。
- 排出量[kgCO₂e]は「kWh×係数[kgCO₂e/kWh]」です。係数の地域・年度・バージョン・出典と、算定の範囲は必須です。仮の係数を、公的な実際の値であるかのように表示してはいけません。
- 削減量は「比較条件をそろえた基準−実績」です。削減率は「差÷基準×100」で、基準が0のときは計算できません。マイナスの値は「増加」として表示します。
- coverage(カバー率)は「有効なサンプル数÷期待するサンプル数」です。期待する数が0のときは「未算定」とします。品質に問題があるときは、集計とレポートにその旨を注記します。欠けているデータを0として埋めてはいけません。
- 室内のCO₂濃度[ppm]は、上に書いた排出量の計算には使いません。省エネ率の期待値は、保証された値ではありません。

## 8. 将来APIへつなぐために残すもの

- 画面が必要とする入力・戻り値のTypeScriptの型と、非同期のinterface(約束事)。
- 外部から来たデータを画面用のデータに変換する、adapter(変換の仕組み)の差し替え口。
- UIが扱うloading(読み込み中)/error(エラー)/empty(データなし)/pending(保留)/confirmed(確認済み)の状態と、役割を切り替えたときに表示を破棄するルール。
- 作り物の応答を使った、フロントエンドの検証方法。

APIのパス・HTTPの方式・データベース・サーバー側の認証や認可・実際の決済・実際の通知・実際の機器制御は、この文書の設計対象ではありません。これらがまだ決まっていないことは、今回のフロントエンド文書を完成させる妨げにはなりません。
