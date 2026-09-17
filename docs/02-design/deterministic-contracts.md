---
document_id: DD-DETERMINISTIC
version: 0.21.0
status: accepted-demo-policy-under-review
scope: frontend-demo-1A
---

# 1A実装・検証の確定契約

DEC-11で具体化したデモ設計。デモ範囲・業務方針は[DEC-12](../00-prepare/internal/decision-record-2026-09-16.md)により採用済み。技術的整合性は独立G1で確認する。企業の商用承認・本番API設計ではない。既存DDの要約を具体化する。各FRのActorは役割別文書、開始条件・入力・処理・成功結果は該当DD、操作権限は操作カタログのauthorization列、失敗結果は本書D01、UI状態は画面カタログを参照する。これらを合わせて一つの契約とする。実装者が未定義の業務条件を追加してはならない。将来外部APIへadapterだけで接続できるという保証はしない。

## D01 認可・失敗の順序

呼出形式はread=`operation(context,input,{signal?})`、write=`operation(context,input,writeOptions)`。成功はServiceResult、失敗はDomainErrorのreject。公開のlogin/reset-previewとデモ制御は明示された例外。業務操作にroleだけで全許可する経路を作らない。

判定順序は次の通り。同じ入力に複数の誤りがある場合は最初のエラーを返す。

1. 入力の構造・型・文字数・enumが不正: VALIDATION。未知フィールドは拒否。
2. セッションなし／expiresAt <= now: UNAUTHENTICATED。
3. 指定IDが不存在・別tenant・別顧客／業者scope: NOT_FOUND。存在の区別を漏らさない。
4. 対象の閲覧scopeはあるが操作permission不足、担当期限外、資格失効、自己承認、契約制限違反: FORBIDDEN。
5. 認可後に同じ冪等キーを検索。同じ内容なら既存結果、異なる内容ならCONFLICT。
6. expectedVersion不一致、状態不適合、確定予定重複、進行中操作との競合: CONFLICT。
7. 関連値の不整合（単位、同一顧客、委託期間への収容、必要な理由、機種候補）: VALIDATION。
8. 制御対象offline: OFFLINE。状態を変更しない。制限専用の未配送意図はD03による。

権限失効後は結果の再返却も禁止。FORBIDDEN時の業務変更0件、マスクした拒否監査のみ1件。同じ拒否リクエストの重複は相関IDで監査を1件にする。readの成功では監査を増やさない。

unknown enumのDTOは全体をUNAVAILABLEとして扱い、原文や内部例外を表示しない。URLは不正resource IDならnot-found、不正filter/日付/enumなら初期化せずVALIDATIONの条件修正画面、未知routeはnot-found。既知routeのscope外IDも同じnot-found文言。ネットワーク障害と機器offlineは異なる表示領域とする。

## D02 発火の仲裁と主体

Automation/PolicyはownerMembershipId、createdByUserId、versionを持つ。actorは作成時のownerであり、閲覧中のroleを使わない。編集でownerは変えない。ownerが失効すれば抑止する。

`automations.simulate/fire`の入力は`{eventId, occurredAt, unitIds, facts, phase}`。phaseはschedule_start/schedule_end/condition。個別rule ID指定で他の方針を無視する経路はない。factsはunitId別のmetric/value/unit/observedAt/qualityと、arrival/departure/occupied/peakの型付き値。1イベントを受けると、そのtenantで有効な全Policy/Automationを同一snapshotで照合する。各ruleは保存されたownerで認可する。通知評価とCommand候補仲裁はSR21により別結果を返す。phaseは起点イベントの分類であって評価対象ruleを絞る条件ではない。予定はoccurredAtに到来する全開始/終了Actionを列挙し、条件ruleは統合factsとその時点のsnapshotで評価する。複数phaseが同tickなら同じ仲裁結果を参照する。UIの呼出者も対象全unitの閲覧と発火権限を持つ必要があり、scope外はNOT_FOUND、権限なしはFORBIDDEN。正当な評価内の不成立はsuppressedとして返す。デモ時計による内部評価は閲覧セッションを使わず、各ownerを再認可する。内部評価関数をUIへ公開しない。

Command仲裁単位は設備（異なるActionでも同一unitでは一つ）。alertおよびnotify_only方針はCommand仲裁へ参加しない。能力・制限・owner権限・同意・freshな入力・enabledで候補を除外し、HQ policy、顧客automationの順、同層はpriority降順、IDのASCII昇順で一つを選ぶ。選ばれたActionだけをCommand policyへ渡す。評価とCommand受付は同じメモリ遷移内で実行し、呼出順で顧客がHQを追い越さない。異なるeventが同じoccurredAtを持つ場合は一つのtickバッチへ統合してから評価する。同じ指標の異なるfactsはeventId ASCII昇順の最後を採用する。シナリオで複数イベントを同時刻に与える場合はdemo.triggerで予約し、demo.advanceClockで一括確定する。UIのfireは現在tickの全factsを1要求にまとめて即時評価し、同じtenant/occurredAtの異なるfactsまたは異なる追加phaseを後からfireした場合はCONFLICT（再評価しない）。予約と即時発火が同じtickを使う場合も先に全予約を統合する。イベントID別結果は同じtick結果への参照でありCommandを再生成しない。

FireResult/SimulationResultは正規DTOに従う。結果はunitId昇順。制御のsimulated結果はdecision=selected/suppressedとしCommandを作らない。両結果は独立したnotifications配列も返す（正規DTO/SR21）。複数設備の一部失敗は他設備の要求を取り消さない。unitIdの重複はVALIDATION。1tick/1unitにCommandは最大1件。同じtenant/eventId/phaseの再送はキーが違っても既存結果を返す。異なるfactsで同じeventIdはCONFLICT。

予定・生活パターンの時計発火はIR54の内部評価、生活パターン条件の成立はIR52。予定終了も独立Action。停止／同意取消後の予定イベントは抑止し、過去のCommandを取消とは扱わない。手動操作がpendingなら自動発火はbusyとして抑止し後刻自動再送しない。確定済みの手動設定は次回正当な発火で変更可能。

## D03 制限の設備別回復

Restrictionのstateはscheduled/requested/applied/release_requested/released/cancelledのみ。exceptionは`{until,reason}`属性でありstateではない。猶予・例外の解除要求も請求を書き換えない。期限切れ後の自動再適用は行わず、新規予告で再判断する。

適用時に各unitへCommand記録を1件作る。offline機の制限Commandだけはstatus=requested、delivery=not_sentとして配送待ちの意図を保持する。通常のcommands.createはofflineで0件。適用確認がない記録をappliedと表示しない。意図作成から30秒でexpiredとなるが、未配送の確定証跡は残る。再接続だけで自動配送しない。

perUnit[].applyStateは`not_sent|sent_unknown|applied|not_applied`、perUnit[].releaseStateは`none|waiting_reconcile|requested|released|not_required|failed`。必須データはunitId、applyCommandIds、releaseCommandIds、observedRestriction、observedAt、evidenceId、pendingReason。観測値は現行generationかつ最新device sequenceの証跡のみを使う。

| 適用の状態 | 全額入金／override／適用後の猶予・例外 | 次の操作と完了条件 |
|---|---|---|
| not_sent | 未配送のapplyをcancelledとして確定。perUnit[].applyState=not_applied | 配送0件の証跡でrelease=not_required。offlineでもこの証跡は有効 |
| not_applied | apply拒否の証跡がある | release=not_required。removeは0件 |
| sent_unknown | release=waiting_reconcile、制限policyを保持 | restrictions.reconcileで現在観測を取得。offline／古い観測なら保留 |
| applied | onlineならremoveを作成しrelease=requested | 対応ID・rulesVersion・Command成功とobservedRestriction=nullでreleased |
| sent_unknown→観測null | reconcile成功、未適用の新鮮な証跡 | release=not_required |
| sent_unknown→観測あり | reconcile成功 | removeを送る。別Restriction IDならCONFLICTとして手動確認 |
| remove失敗／expired | release=failed、集約release_requested維持 | 明示retryで新Command。旧Commandは履歴のまま |

`restrictions.retry({restrictionId,unitIds,phase:apply|release,confirmedRulesVersion})`は最新version、新キー、reasonを必須とする。applyはrequestedかつnot_sent/確定失敗で、未払い・猶予なし・同じ予告版・onlineを再確認する。sent_unknownは先にreconcile必須。releaseは解除意思ありかつreconcile済みまたはapplied、未完了removeなしを確認する。既存成功設備は再送せず既存結果を返す。retryは対象設備ごとの結果を返す。未確定applyを新しいapplyで置き換えない。

`restrictions.reconcile({restrictionId,unitIds})`は模擬観測イベントを用いた照会であり、成功時に観測証跡を保存するwrite。観測が30秒より古ければTIMEOUT。all perUnit[].releaseStateがreleased/not_requiredなら集約released。適用要求後の遅延ackは集約appliedに戻さず、観測の再照合とremove必要判定を行う。released/cancelled後の矛盾観測はSR26の独立したrecoveryCasesに記録し、終端Restrictionをactiveへ戻さない。機器成功を捏造しない。

## D04 冪等性・決済・待機

解除要求の起動経路（入金確認／猶予・例外／強制解除／明示release）と`restrictions.release`の前提・冪等性はIR35を正とする。

全writeのキーはUUID、スコープはgeneration/tenantId/userId/operation。payloadは構造化値をキー順に正規化して比較し、expectedVersionも初回値を保持する。同一意図再送は初回payloadのまま送る。キーと結果はresetまで保持し、role切替やlogoutで消さない。認可は返却前に再評価する。実行中なら同じPromise、成功なら当時の結果、確定業務失敗なら同じ失敗を返す。transport TIMEOUT/UNAVAILABLEは業務結果を確定しない。

ID未取得でも`writes.getResult({operation,idempotencyKey})`で`not_received|pending|succeeded|failed`、資源IDと当時結果／エラーを取得する。作成時のuserIdと現在の対象scopeが一致する場合のみ返す。確認不能なら再送ボタンを出すが同じキー・payloadを使う。確定failed後の新しい意思は新キー。旧キーを新決済試行に使わない。

Paymentのinitiate入力はinvoiceId/method/demoConfirmed。戻りPayment.idが試行ID。後続`payments.simulate`は`{event:processing|confirm|fail,paymentId,eventId,paymentReference}`を必須とし、対象試行以外を更新しない。confirmはprocessingからのみ、failはinitiated/processingからのみ。同一eventIdは既存結果、終端failedに遅れたconfirmはCONFLICT。confirmed後のfailもCONFLICT。参照番号はtenant内一意。同じinvoice/参照/金額の確認は認可後に既存成功を返す。別invoice流用はCONFLICT。demo_instructionsはPaymentを作らずNotificationPreviewだけを返す別union分岐。固定カード番号の入力欄も作らない。

通信待機は1試行10秒。transport障害（UNAVAILABLE/TIMEOUT/RATE_LIMITED/DELAY）とネットワーク断の注入はIR37のdemo.triggerで行う。readのUNAVAILABLEのみ1秒、2秒待機して最大2回追加（全体最大33秒）。TIMEOUT/VALIDATION/認可エラーは自動retryなし。writeは自動再送なし。RATE_LIMITEDは正整数1〜3600秒を受理し、不正／省略は30秒とする。その間同じ操作のボタンとサービス受付を止め、残秒を表示する。reset/role変更は閲覧待機を中止するが業務受付を巻き戻さない。

Commandのack許可はreceivedAt < expiresAt。now==expiresAtではexpiryを先に処理する。イベント優先順はreset→権限変更→期限失効→機器応答→定期評価→画面購読通知。Command遅延ackは履歴と新観測のみ記録し、expiredをacknowledgedへ書き換えない。タイマー進行はdemo時計で制御する。

## D05 機器の排他・登録・利用終了

unit単位でCommand(requested/sent)、DiagnosticRun(awaiting_start/running/end_requested)、DeviceOperation(queued/running)を排他にする。runが作る開始・終了Commandだけは同じrunの予約枠を使う。check/calibrate/FWと通常Commandは相互に受付CONFLICT。FW/checkはqueued→running→succeeded/failed、作成から60秒でfailed/TIMEOUT。境界ちょうどは失敗優先、遅い成功は履歴だけ、firmwareVersionを更新しない。再試行は新operationId・新キー。校正は同じ排他判定で履歴を即時保存し既存測定を変更しない。

devices.registerはunitIdとjobId（技術者のみ必須）を受け、targetUnitIdとして予約する。unitIdはbindまでnull。未紐付を返せるのは作成Membershipとdevice.manageのHQのみ。登録時に対象の担当を検証する。Unitにactive Deviceは最大1件。SensorはDevice内metricごとに1件、複数同metricは1Aでは拒否。serialの正規化一意性はtenant内。bindは両unitのscope、理由、排他、未解決tamperなし、SR24のactive制限/回復caseなしを照合する。旧bindingは終了時刻を付けて保持し、Telemetry/Alert/CommandのunitIdは書換えない。新bindingで新sensorIdを発行する。履歴の発生時binding/Unit/customerを保持し、SR24で行単位に認可する。

接続はunknown/connecting/online/offline/error（ACUnitの接続はDeviceからの派生値でIR47、operationの開始はIR67）。checkのrunningでconnecting、成功でonline、明示接続失敗でerror、通信途絶でoffline。FW失敗はoperation errorであり接続状態を勝手に変えない。powerSignalはunknown/on/off、tamperはclear/detectedの別軸。画面はtamper注意、operation失敗、connection、last-known電源を併記する。未知値を正常色にしない。

archived資源の一覧・集計・候補からの除外と個別取得の投影はIR39。units.archiveはactive Job、active Device binding、進行中Restriction/Command/run/operation、未終了ContractがあればCONFLICT。関連の履歴だけなら可。有効Automationは同じ遷移内でdisabledにし理由unit_archivedを記録。アーカイブ後は新操作不可、監査・完了報告は元のscopeで履歴閲覧可。units.deleteは参照が一件もない未使用Unitだけ物理削除可能。履歴参照があればCONFLICT。Property/Spaceは配下Space/Unitが1件でも残ればarchive不可。顧客の無効化もactive Unit/Contract/JobがあればCONFLICT。同一顧客内の移設だけIDは不変、versionは+1。IR02により既存IDの顧客変更および別tenantへの移管は禁止。

## D06 案件・公開情報

業者の受諾前はIR25の設置物件住所を含むJobOfferSummaryだけ。受諾後の期間内は現場住所・入場案内と設備根拠だけを返す。顧客のメール・電話・担当者個人名は業者と外部技術者へ一切返さない。連絡はアプリのメモ／宛先roleによるpreviewのみ。期限後は一覧/詳細ともIR23の不変JobHistorySnapshotのみ。投影後のデータをUIへ返し、UIで隠すだけにしない。

AssignmentのvalidFrom/UntilはscheduledStart/Endと同じ半開区間（作業窓。閲覧窓はIR49）。重複は既存start < newEnd AND newStart < existingEnd（同じjobIdの置き換え対象のactive Assignmentは除外、IR102）。接する枠は可。assigned/in_progress/rework_requested/on_holdで再割当または延長可能。外注はpartner.assignと現在有効なOffer、社内はHQ job.manage。Offer期限の延長はHQのjobs.extendAccessで同じ業者・job・未来until・理由を保存する（過去を遡って許可しない）。旧Assignmentは即時無効。状態は維持。期限切れでも管理者側の割当操作はでき、失効技術者側の変更だけを拒否する。

submitted→on_hold→resumeHoldではin_progressに戻し、提出版を不変に保った新draftを作る。reworkも新draft。差戻し後の割当変更も元作者を保存する。新担当が更新した項目だけauthorId/observedAtをRepositoryが更新する（SR07）。入力に作者を受け取らない。期限切れ提出はFORBIDDEN。on_hold/reworkの取消はHQ job.manage、理由必須。品質担当不在の外注をHQへ引き継ぐ場合はjobs.reviewのreviewMode=hq_escalationと理由を必須にし、同一userの自己承認はHQにも禁止する。

ユーザー本体は固定デモアカウントのみ（FR-A03）。Userの作成／削除は1A対象外、Membershipの追加・更新・失効は対象。最後の有効なidentity.manage保持者を失効／降格させる変更はCONFLICT。自分の別Membershipへの昇格もuserId単位で禁止。第三者の業務責任としての承認とは区別する。

## D07 センサー・購読・数値

Telemetryの系列キーはtenantId/unitId/sensorId/metric。sensorIdは必須（点検はinspection-item IDを使用）。metric/unitの対応と1A合成値の範囲はtemperature/°C:[-50,100]、humidity/%:[0,100]、co2/ppm:[0,10000]、pm25/µg/m³:[0,1000]、power/kW:[0,100]、vibration/mm/s:[0,100]、refrigerant_pressure/kPa:[0,5000]。実機能力や健康基準を意味しない。非有限値はDTOに通さずvalue=null/quality=suspectとする。この場合suspectを保持しmissingへ変更しない。範囲外、未知unit、observedAt>receivedAt、receivedAt>nowはIR12のRawMeasurement正規化でsuspectとして値を集計・制御から除く。入力nullはmissing。ただしIR12の品質原因を持つnullはsuspectを保持する。その他のqualityの優先はmissing→suspect→stale→valid。staleはnow-observedAt>120秒（sensorの正整数staleAfterSecondsを使う）。suspect/missing/staleは最後の値と品質を表示できるが正常判定／自動発火に使用しない。過去区間の積算は観測時点でvalidだったサンプルを使い、現在staleになった理由だけで過去積算を失わない。

`events.subscribe(context,{afterCursor,resources,unitIds},listener)`は解除関数を返すRepository補助interface（Promise操作カタログとは別）。イベントは`{generation,cursor,eventId,entityType,entityId,version,occurredAt,changedFields}`。cursorはgeneration内で単調増加し全イベント共通、eventIdは一意。read metaにeventCursorを返す。snapshotを取得後そのcursorから購読し、間のイベントをリプレイする。履歴はresetまで全件保持。scope外イベントは配信せずcursor進行だけ通知する。欠番は再snapshot→再購読。重複cursor/同一entityの旧versionは無視。role変更で解除、generation変更で全旧イベント拒否。接続復帰は再snapshotで開始し、古いheartbeatでonlineに戻さない。時計tickはデータ到着なしでもstale・期限の再評価を通知する。

全Page読取はcursor/limitを持つ。既定25、最大100。cursorはqueryの正規化条件、scopeVersion、snapshotVersion、offsetを束縛し、条件変更はVALIDATION、業務更新ではsnapshotを変更せず取得開始時の不変snapshotを読み続ける（SR14）。失効cursorはCONFLICT。並び順の最後のキーは常にid asc。notificationsはseverity critical→warning→normal、occurredAt desc、id asc。jobsのUI選択sortはseverity/dueAt/statusのasc/desc、telemetryはobservedAt asc、sensorId asc、id asc。テレメトリー1000点は100点×10ページで取得し、欠けたページを正常グラフとしない。

KPIは専用summaries.getで全検索対象を集計し、現在ページから計算しない。kind=customer/partner/technicianをroleと一致させる。scope外は件数にも含めない。partner countsはoffered、accepted+assigned+in_progress、submitted、dueAt<nowかつ未完了でそれぞれoffer/active/review/overdue。technicianは担当Unit distinct数、assigned数、未完了かつ期限超過数。customer/adminの稼働分類はSR27のeffectivePowerStateを共通使用し、customerはopen/acknowledgedかつseverity=critical/warningのAlert数も返す（IR51）。必要な最新telemetry欠測時はunknown（C01条件）。admin.summaryのbilling値はbilling.manage保持者のみ、非保持者はamountsByCurrency=null、billingVisibility=forbidden（0ではない）。

members.capacityはdateの作業可能区間（seed09:00–17:00 Asia/Kuala_Lumpur、休日は空）と割当区間の和集合の交差分数を返す。分母0／未設定はnull。確定割当のみ分子に含める。4h/8h=50.0%。

電力量は60秒の固定slot。[from,to)はUTC分境界、最大366日、端の部分分はVALIDATION。slot始点のpowerサンプルだけを60秒分の長方形積算に使用。補間・前値保持なし。欠測／suspectは除外。期待数=対象設備数×分数、coverage=valid slot数/期待数。全欠測はkWh/料金/排出量null、coverage=0。0対象は期待数0、coverage=null。同じslot始点の複数サンプルは最大device sequence、同sequenceならid ASCII昇順の先頭を採用し、その後IR08/IR11のorigin/品質/境界を判定する。始点以外のサンプルをslot内最新として代用しない。

料金はslot kWh×そのslot始点のtariff単価、合計後にminor unitへ四捨五入。tariffは半開区間で重複なし、欠けるslotの料金はnullとして全体料金もnull（電力量は表示可）。仮単価0.5MYR/kWh固定、税・基本料金なし。演算は十進有理数で行い、表示丸めを再利用しない。電力量表示1桁、料金2桁、割合1桁。削減量=baseline−actual、増減量を別途出す場合deltaKWh=actual−baselineと名前を分ける。baseline0は削減率null。比較は同一設備集合・同一境界・同じ分数で、実績coverage100%かつ基準品質がSR29で比較可能な場合だけ。実測基準はcoverage100%、固定仮定基準はmodeledとして明示する。部分coverageでは実績を表示し比較差分／率はnull。MRVは係数版snapshotを保存し、coverage<100%ならincomplete=true、demo_reviewedにできずdraftのまま。100kWh/80kWh/0.5MYR/0.5kgCO₂eなら削減20kWh、20%、節約10MYR、排出40kgCO₂e。

## D08 アラート・通知・表示安全性

閾値評価は観測時刻で順序付けた1秒tick。quality validの値のみ、保持はsensor stale期限まで。threshold条件が連続durationSeconds成立して初回open。missing/suspect/stale/通信断で継続カウンタを0にし品質通知を1件作る（同じunit/metric/品質は回復まで追加しない）。遅着した過去観測では過去Alertを新規発生／解除しない。gt/gteのrecoveryThresholdはthreshold未満、lt/lteはthreshold超。回復条件も同じdurationSeconds連続で自動resolved（policyId≠nullのAlertだけ、IR66）、未確認openからの自動resolvedも許可しrecovery evidenceを保存する。手動resolveは権限・理由・根拠ID必須。cooldownは同じunit/policy/重大度の通知間隔に適用し、Alert履歴は抑制しない。重大度上昇はcooldownを無視して1通知。acknowledged以前にescalateAfterMinutesが経過したらHQのalert.policy.manage保持者へ1回。宛先が0ならAlert.deliveryFailuresへDeliveryFailureを保存する。Notificationは0件、失敗の公開範囲はSR12とし、ダミーの宛先を作らない。

AirPolicyはoperator=gte固定（高CO₂等へのデモ）、ventilationLevel=low固定、recoveryThreshold<threshold。他方向はalert policyで通知のみ。ventilation=trueかつventilationLevelsにlowを含む機器だけCommandを作る。非対応は換気ボタンdisabled、理由と手動案内を表示する。通知だけの機器もpolicy保存可能。通知先はrecipientMembershipIds（Customer IDは渡さない）。全air_qualityは通知評価し、換気候補だけを別仲裁する（SR25）。通知設定はSR28の必須入力を用いる。

Notificationは`id,tenantId,version,recipientMembershipId,scopeVersionAtCreation,target:{kind:unit|job|invoice|restriction|device|inquiry,id},templateKey,params,channel,deliveryState,severity,occurredAt,readAt`。paramsはテンプレートごとの固定項目（対象名、時刻、状態、reason、amountMinor/currency、method、message）だけ。billingテンプレートはclientまたはbilling.manage保持者だけ。internal noteはcustomer宛禁止。履歴へのリンクも現在scopeで再認可する。

| target | Client | Contractor | Technician | Admin |
|---|---|---|---|---|
| unit | /customer/units/:id | /partner/units/:id?jobId=:jobId | /technician/units/:id?jobId=:jobId | /admin/units?unitId=:id |
| job | /customer/maintenance?jobId=:id | /partner/jobs/:id | /technician/jobs/:id | /admin/jobs?jobId=:id |
| invoice/inquiry | /customer/payments/:invoiceId?inquiryId=:inquiryId | 禁止 | 禁止 | /admin/billing?invoiceId=:invoiceId&inquiryId=:inquiryId |
| restriction | /customer/payments/:invoiceId?restrictionId=:id | 禁止 | 禁止 | /admin/restrictions/:id |
| device | /customer/units/:unitId | /partner/units/:unitId?jobId=:jobId | /technician/devices/:id | /admin/devices?deviceId=:id |

invoiceId/jobId等の関連は権限付きRepositoryで解決し、対象が複数なら選択一覧、0なら「利用できません」。任意URLは保存しない。自由文・翻訳params・ファイル名・監査値はテキストノードで表示。HTML/Markdown実行は禁止。returnToはURL構文解析後、同一origin相対pathかつscreen-catalogの許可routeだけ。`//`、scheme、制御文字、バックスラッシュ、二重encodingは拒否。画像はJPEG/PNGのsignatureとdecode成功を確認し、SVG/HTMLは拒否。画像は最大幅/高さ8192px、合計画素16000000以下も必要。

## D09 セッション・言語・音声・予定

Sessionは`userId,membershipId,tenantId,role,permissions,scopeVersion,generation,viewEpoch,issuedAt,expiresAt`。寿命はデモ時計で30分。延長はIR55のdemoSession.extendだけで、期限120秒前に予告する。demo.advanceClockのジャンプは寿命を消費せずexpiresAtを同じ差分だけずらす(IR36)。期限時に画面／Query／未保存draft／object URLを破棄し/loginへ。Repositoryの受理済み業務イベントは継続。signIn時のreturnToが現在roleで許可ならそこへ、それ以外はrole home。switchMembershipは同じデモuserのMembershipか、デモ専用アカウント切替による別userを明示し、業務の自己承認はuserIdで判定する。

preferencesはlocale=en|ms、timezone（IANA）、currency=MYR固定read-only。通貨切替UIは作らない。契約がUSDならMoney.currency=USDで表示し換算しない。変更可能なのはlocale/zoneのみ。role変更とlogoutでpreferencesを保持、reload/resetではen/Asia/Kuala_Lumpurへ戻す。reload/resetはsessionもnullとしログインへ。タブ間同期／永続化は対象外、ヘッダーに「このタブのみ・再読み込みで初期化」を常時表示する。dirty値があれば通常離脱とreloadに破棄確認、role変更/logout/期限では保護のため破棄して通知する。

音声・テキストは実マイクを使用しない固定文法。文字数1〜200 Unicode code point、trimしASCII大文字小文字だけ無視。enは`temperature <room>`、`set <room> to <integer> degrees`、`help`、msは`suhu <room>`、`tetapkan <room> kepada <integer> darjah`、`bantuan`。整数は10進、roomの照合対象と候補の作り方はIR65（Space.nameの完全一致）、候補はscope内だけ。結果はunsupported、help(messageKey)、candidates([{unitId,pathLabel}])、temperature(unitId,measurement)、change(unitId,celsius,before,expectedVersion)の判別union。helpは操作説明の読取のみでInquiryを生成しない。同名は候補選択後resolveを再実行。権限なしは対象不存在として返す。言語変更は未確認intentを破棄し入力文保持、フォーム入力は保持、通知は新辞書で再描画。確認取消／マイク拒否の合成イベントはCommand0件。

週次予定の有効化はsave(enabled=true)による明示操作。新規フォーム既定false、確認画面でON選択後の一回のsaveも許可。start/endは1Actionずつ。次回previewは`automations.nextRuns`で次の8 occurrenceを返す。DST検証は保存時nowから366日以内の全occurrenceを列挙し、一件でも存在しない／二重解釈のlocal日時があればVALIDATION。検証済み期間外の実行時にも同じ検証をし、不正ならその回をskipして警告、他の時刻に移動しない。テストはclockを該当DST日の前日にして同じ曜日/HH:mmを保存する。zone表示変更は保存済みautomation timezoneを変更しない。end<=startはendsNextDay必須、start=endは常に拒否。endsNextDay=trueかつend>startは24時間超なので拒否。

## D10 UI・環境・検収

画面カタログのScreen IDはroute単位。DDが同routeを共有する場合は同Screenのsection。URLの選択ID・tabはcatalogに固定、初期tabはoverview（作業画面はinspection）。formはjobId/reportIdをkey、タブ移動では同フォームを維持しdirty値を破棄しない。対象ID変更／別routeはdirty確認。更新成功後はmutationのresult.versionを受けて関連Queryをinvalidate、pendingのCommandは成功toastにしない。

画面の優先順位: unauthenticated→forbidden/not-found/work-not-started（IR76）→必須Query error→initial/loading→empty/success。成功済みデータの再取得中はrefreshing、再取得失敗はstaleとして成功データへ重ねる（IR83）。offline/stale/device connecting/errorは成功データへ重ねる状態であり別画面へ遷移しない。必須Queryはscope、編集対象本体、能力、現制限。補助は履歴、添付、グラフ。補助失敗はパネル内retry。能力／制限不取得で制御disabled。通知・ダッシュボードは全対象のprimary Queryを必須とする。common componentsはpresentationだけでRepositoryを呼ばない。各pageのpropsは`{context,routeParams,searchParams,navigate}`、stateはURL/Query/RHF/local dialog、eventsはcatalogのread/write、error/loading/emptyはこの優先表。ConfirmActionDialogはcancel/confirm、初期focus=cancel、送信中confirm disabled。

1A対応環境はWindows 11 Chrome/Edge、macOS 14 Safari/Chrome、iPadOS 17 Safari（タブレット幅768/1024）、iOS 17 Safari、Android 14 Chrome。ブラウザ版は実装開始時に各OSで取得可能な安定版一つを採用して検証environment.jsonに実測版を固定する（現時点で架空の版を書かない）。性能の基準端末はApple M1 8GB以上、macOS、Chrome、production build、1440×900、他タブなし。100Unit/1000sample、cold初回5回・warm10回の最大値を測る。操作一覧は一覧filter、pagination、詳細表示、温度確認dialog、form submitの5種。入力から最初のloading/disabled/結果DOM描画まで200ms以内。正常待機300msを含む一覧全表示は2秒以内。意図的slow=3000msは初動200msと継続loadingを判定し、2秒条件からだけ除外。12000ms fixtureは10秒timeout表示を判定する。

a11y検収は全roleのS01〜S08に含まれる主要操作、キーボードのみ、NVDA+Windows ChromeとVoiceOver+Safari、英語・マレー語。自動axeのcritical/serious未解決0、手動でラベル／focus復帰／エラー関連／200%拡大／色以外の識別を全件確認。WCAG全適合を試験前に主張しない。幅360/768/1024/1279/1280/1440で全Screenを確認。コントラストと44px操作領域はUX-04/06の値を使用する。

## D11 本番移行と未決事項

1Aの範囲・業務方針はDEC-12で採用済み。詳細なデモ値は本番へ昇格させない。本番はNOT READY。Backend/IoT/Securityの実担当者未指名は公開・実接続のゲートで解決すべき事項であり、デモ値を商用値へ昇格させない。OPENには決定責任の役割、期限（本番設計開始前／企業検収前）、対象FR、デモ暫定方針を記録する。

本番着手の必要成果物: HTTP endpoint/method/request/response/status/error一覧、HTTP状態・通信例外（400/401/403/404/409/429/5xx/timeout/offline）からDomainErrorへの写像表（IR90）、認証/session/token/CSRF契約、サーバー認可表、DBの識別子・version・整合性責任、機器の受理／配送／適用／再照合イベント、再試行と冪等性、Backend停止／機器offlineの回復、ログとPII保持、SLAとテスト環境。demo-only操作（IR60）の置換先も含む。これらは本書で捏造しない。実機の時間付き操作の主体はブラウザから独立して設計し、ブラウザ終了・Backend停止時の終了責任をIoT/Backendが決定する。成果物未確定なら本番ゲートはblocked、1Aゲートと分離する。

## D12 フィールド・補助読取・編集版の共通規則

正規のDTO/入力/結果はservice-contracts.ts、操作固有の文字数・数値は各DDのフィールド表。IDはDDC-01、InstantはUTCのミリ秒精度へ正規化、Versionは正整数。入力の未知フィールドはVALIDATION。DTOのnullを省略しない。各DDに規定のない名称欄は1〜120文字。理由系（reason/cancelReason/declineReason/changeReason/resolutionReason/reviewComment/purpose）はDDの記載にかかわらずtrim後1〜1000 Unicode code point（IR87）。ID配列は重複なし1〜100件。空が意味を持つreport.items/parts/measurements/attachments、permissions/scopes、query unitIdsは空を許可。自由文は改行LFへ正規化しHTMLを実行しない。安全整数でない金額・非有限数値はVALIDATION。文字数は正規化後に検証する。

writeOptions.expectedVersionの完全な操作別指定はwrite-version-catalog.csvを正とする。以下は要約。新規restrictions.schedule/offsets.preview、offsets.simulate(event=request)はoptions版不要（requestはinput.quoteVersionを照合）。writeOptions.expectedVersionは更新する主資源の現在版。saveの新規(id省略)、create/register、auth/preferences/demo操作、automations.fireには不要。例外と分岐は操作別版契約を参照する。commands.create/diagnosticRuns.createはinputのexpectedUnitVersion（runはexpectedJobVersionも）を必須としoptions版は不要。jobs.saveDraftは既存report版（新規は不要）、jobs.submit/reviewはjob版に加えinputのreportVersion、restriction更新操作（schedule以外）はrestriction版、devices操作はdevice版（addResponseNoteだけDeviceEvent版）、attachments.addはreport版を使う。payments.simulate initiateはInvoice版、後続イベントはPayment版、instructionsはInvoice版。イベントによる内部更新は新しいsnapshot内でversionを照合し、外部UIの古いcontextを流用しない。updatedAtが同時刻でもversionは単調増加する。

補助読取の権限: 特定の操作権限を持つHQは、その操作に必要な対象Unit/Capability/Organization/Customer/Property/Space/Membershipの候補を管理scope内で読取できる。これはreadだけの許可で、当該台帳の編集やbillingフィールドを許可しない。用途はcatalogのdesign_idsに含まれる画面へ限定するのではなく、Repositoryが操作permissionと対象関係から判定する。client/contractor/technicianは各scopeを適用し、このHQ補助許可を使わない。

Queryの許可filters/sortはquery-catalog.csvに固定する。フィールド名が存在しても、その操作で未許可ならVALIDATION。kind/statusは返却モデルのenum、from/toは両方または両方省略、dateは実在YYYY-MM-DD。filterはAND、unitIdsはOR、空unitIdsは0件。未知enumはVALIDATION。文字列検索・全フィールド検索は対象外。sort未指定はcatalog default、nullは末尾、同順位はid asc。cursorとlimitは全Pageで使用可。afterCursorは購読の整数でありPageの不透明cursorとは別物。

offsets.previewはquoteのsnapshotと15分期限を保存するwrite。申込は保存されたquoteId/versionを照合する。notifications.previewは保存しないread。履歴に残るNotificationは業務イベントが生成する。通知paramsはRepositoryが対象から生成し、UIからparamsを入力するとVALIDATION。message/reasonだけは各DDの自由文を許可する。宛先は現在の当事者、billing/internal制限を通したMembershipだけ。関連当事者だからという理由で他人の通知一覧・既読状態を返さない。

Customer.name/statusはOrganizationとは独立したサービス台帳の名称/利用状態。organizationIdはcustomer kindのみ。members.saveの外部技術者は有限validUntil必須、資格は固定fixtureから読み取り（1Aの資格編集は対象外）。ServiceResultには必ずmeta.eventCursorがあり、JobSummary.unitId/assignmentId、Unit.observedState、Invoice.status/paymentStatus、Restriction.perUnitを正式名とする。DDの概念図のRef/Summary略称をDTOの別フィールドにしない。

allergenObservationはAirSeriesの付帯情報。availability=available/not_measured/unsupported、sourceLabel/evidenceTextを使う。availableはsubstance/sourceLabel/observedAt/evidenceText必須、valueがあるならunit必須。欠けた場合のUIは不明。PM2.5から導出しない。数値表示以外の定性的根拠も許可する。telemetry.seriesのmetricは通常指標のみで、allergenは独立metricではない。最新付帯情報は対象1Unit時だけ返し、複数Unitはnullとして対象選択を求める。

## D13 画面の不足入力とコンポーネント契約

DD-C13/A15の見積入力: purposeは必須1〜1000文字、初期値は空、period/unitIds必須。DD-A14のMRVはorganizationIdと対象unitIdsを必須選択し、全Unitが同じcustomer Organizationに属することを検証する。DD-C04/C05は同じScreen内のschedule/eventタブ、C11/C12は同じinvoice内payment/restriction/inquiryタブ。作業記録はjobId/reportIdの同じFormProviderを点検/測定/写真/報告の各sectionで共有する。

共通Component契約はcomponent-contracts.csv。表示ComponentはRepositoryへ依存せずPageが渡す。PropsのDTOを複製して業務stateにしない。Pageのread/write操作はscreen-catalogのoperationsを使い、primary_queries失敗では画面全体、secondary_queries失敗では局所を表示する。選択なしの一覧はlist、選択IDありでは対応getがprimaryとなる。初期値未取得はinitial、取得中loading、取得成功で0件empty、0以外success。単体getの不存在はemptyではなくnot-found。権限なしはpermission-denied。再試行の回数/ボタンはD04、成功後Query更新はD10、確認取消は変更0件。

URLのtabはcomponent-contractsのsection名だけ。無指定はscreen-catalogのdefault_tab、不正値はVALIDATION。ブラウザBack/Forwardもdirty確認を通す。拒否なら元URLとdraftを保持。確認なら破棄して新URLを再取得。reload確認はブラウザ標準文言を使用し、離脱を強制阻止できるとは保証しない。

## D14 再レビューで補足した遷移・取得の一意性

- 集計: partnerの業務件数はIR23の現在有効JobSummaryだけから計算し、scheduledCount=accepted+assigned、inProgressCount=in_progress、activeCountはその和。adminのjobCountsは各JobStatusの件数、overdueInvoiceCountはnow>dueAtかつ未入金の請求数、amountsByCurrencyはその未入金額。billing権限なしでは両方null。機器接続のconnecting/errorはonline/offlineのどちらにも加算せずunknownへ加算するが一覧には元enumを表示する。集計期間のjobはrequestedSlot.startAtが[from,to)、設備数と現在Alertはsnapshot時点、電力量は[from,to)。KPIの遷移先へ同じ各定義のfilterを渡す。
- Queryの一覧sortにidがないprojectionではJobOfferSummary.jobId、Capacity.membershipIdを安定IDとして用いる。summary.filtersはcustomerId/propertyId/unitId/unitIds/from/toを許可し、roleのscope内でAND評価する。telemetry.seriesはunitIdsかspaceIdのいずれか一つ必須、複数Unit結果はunitIdで分離して表示する。temperatureを平均値へ畳み込まない。スコープを広げる暗黙の全件fallbackは禁止。
- readとwriteのprojectionは同じ公開規則を使う。Customer向けのJobはcostsをvisibility=customerだけに絞り、draftReportRef=null、reportRefsは受理済みだけ。外注は顧客連絡情報とHQ内部費用を取得できない。完了後の資源の版を返す場合も公開規則を回避しない。
- ReportのIDはjob内で固定、内容版reportVersionはdraft保存時のWorkReport.version。初回saveは1、既存draftの内容変更は+1。submitは内容を固定し、同一版にsubmittedAtイベントを付ける。受理/差戻しの時刻・reviewHistoryは別イベントからprojectionに合成し、内容本文を書き換えない。resumeは最新内容を次の版へコピーし新draftとして保存。attachments.addはまず作成済みreportへ紐付け、内容版を+1し返却後reports.getで新版を取得する。入力中の写真はローカル一時値、初回draft→写真保存→最新draftという順で行う。saveDraft.attachmentIdsから外した写真は現draftだけから除き、提出済み版からは除かない。object URLは直ちに解放し、Blobは参照中の版が一つでもあれば保持する。
- AT-T09のdraft version=1は最初の本文保存時点。その後の写真追加/本文変更で版が増え、submitではその現在版を固定する。reviewに古い正整数版はCONFLICT、0は入力VALIDATION。jobs.startはstartedAt=nowを初回だけ設定する。
- DemoSession操作/preferences/demo制御は業務writeの冪等結果照会とは別のローカル境界であり、session未作成でもsignIn/reset/trigger/advanceClockをデモUIから呼べる。業務writeの冪等スコープに必要なtenant/userは現在Sessionから取り、UIが入力した値を信用しない。デモ専用イベントを本番認証/機器制御として公開してはならない。

- アーカイブのactive依存はJob.statusがcompleted/cancelled以外、Device.unitIdが対象ID、Restriction.stateがreleased/cancelled以外、Contract.endAt>now、非終端Command/Run/DeviceOperationを指す。完了したJobの履歴や失効済み契約だけでは拒否しない。1AではDevice廃棄/unbindの独立機能は追加しない。既存bindingのまま設備を廃止しようとしたらCONFLICTと理由を表示する。
- CapabilityはmodeControl=trueならmodes非空、falseならmodes空。fanControlも同じ。control=falseでは通常Action不可。temperature=nullは温度変更不可、ventilation=trueならventilationLevels非空。非対応値を保存することと、既存未登録値をnullで表示することを分ける。

- resetはgenerationを単調増加させる（0へ戻さない）。clockとseed/sessionは初期化する。reloadは別Repository instanceとなり、前instanceのcallbackを受理しない。scope外購読のcursor通知はentityId/version=null、changedFields=[]、entityType=cursor_onlyとし、資源情報を含めない。

## D15 独立レビューによる取得・投影の補足

notifications.recipients(context,{target,templateKey,channel,role?,query})は、指定対象・template・channelで現在通知可能な候補をPage<NotificationRecipient>で返す。候補はid(Membership ID)/role/displayLabel/allowedChannelsだけで、電話・メール・資格・他のscopeを返さない。呼出者は対象の閲覧と通知preview権限が必要。billing/internalの公開規則で絞り、複数なら利用者が選ぶ。0件は送信不可。P07/A05/A12/C11/A08の宛先選択で使用する。Policyの複数UnitはUnitごとに候補を取得し、全対象で許可されるMembershipの共通集合を選択肢にする。保存時にも再認可する。

notifications.previewの入力はtarget/templateKey/channel/recipientMembershipIdと任意message/reasonだけ。paramsの対象名・時刻・状態・金額/通貨・方法はRepositoryが現資源から生成する。任意paramsの入力を拒否する。instructionsイベントはinAppで本人宛の支払案内previewを返す。別channelの確認はrecipients→previewを用いる。いずれも実送信なし。

HQのalert.resolve保持者はalerts.get/listを許可する。複合画面の各sectionは個別のpermissionでqueryを有効化する。A05のPolicy一覧/編集はalert.policy.manage、Alert一覧/解消はalert.resolveまたは閲覧権限を判定。無権限sectionのqueryをprimaryとして要求せず、他の許可sectionを閲覧可能にする。A08の請求選択後はinvoices.getからpaymentRefsのID/versionを取得して入金確認する。

Queryのaliasと比較先はquery-catalog.filter_mapping/sort_mappingを正とする。返却DTOに存在しないdueAtでJobEventを並べ替えない。JobOfferSummaryのstatus sort/filterはIR23の公開statusを使う。投影前のJob.statusやAlert重大度を参照しない。全sortはscope/projectionを適用した集合内で行う。

## D16 定期保守計画の再訪と生成

plans.list/getはjob.manageのHQが管理scopeの既存計画を取得する。/admin/jobsの計画一覧とplanId選択で使い、保存直後のUI状態を失ってもID/version/nextDueAtを再取得できる。

1Aの月次計算timezoneはUTC固定。保存するnextDueAtはnowより未来、recurrence.intervalMonthsは整数1〜12。作成または次回日時を変更した保存時にanchorDay=nextDueAtのUTC日を記録する。UIは表示時間帯の入力をUTCに変換し、「繰返し計算はUTC」と表示する。存在しないローカル日時はD09の入力拒否を適用する。

plans.generateNextはid/occurrenceDate=保存済みnextDueAtとplanのexpectedVersionで明示生成する。作成時点でoccurrenceDate<=nowならCONFLICTとして次回日時の修正を求める。自動生成や過去分の一括追いつきは行わない。同一planId/occurrenceAtは1Jobだけ。同じキー再送は既存結果、別キーで既に生成した回はCONFLICT。

生成JobはplanId、occurrenceAt、unitIdを保存し、type=periodic/status=requested、symptom="Scheduled periodic maintenance"、requestedSlot=[occurrenceDate,occurrenceDate+1時間)、dueAt=その終了、contactWindow/startedAt/completedAt/contractorOrgId/assignmentId/draftReportRef=null、alertIds/reportRefs/costs=[]とする。これらの日時は希望枠でありAssignmentではない。通常のjobs.createで作るJobはplanId/occurrenceAt=null。

同じメモリ遷移でgeneratedOccurrencesに{occurrenceAt,jobId}を追記しplan.version+1。nextDueAtは元の年月へintervalMonthsを加算し、日=min(anchorDay,移動先月の日数)、UTC時分秒ミリ秒は維持する。例: 2027-01-31T10:00Z→2027-02-28T10:00Z→2027-03-31T10:00Z。anchorDayを2月の28へ縮めない。plan更新で生成済みJobを変更しない。返却Job後はplans.getを再取得して次回を表示する。

0.9.0修正契約: [厳格レビュー修正契約](strict-review-contracts.md)と[操作別版契約](write-version-catalog.csv)を併読する。

SR17〜19は2026-09-16ユーザー承認済み。期間は暦日・完了分、offsetは同一記録で失敗段階だけ再試行、active制限中の契約編集は拒否する。詳細・優先規則はstrict-review-contracts.mdを適用する。

現行0.21.0の追加契約: [再レビュー修正契約](review-resolution-contracts.md) IR01〜106を併読する。同じ論点の旧記述より優先し、衝突時の順位はIR72に従う。
