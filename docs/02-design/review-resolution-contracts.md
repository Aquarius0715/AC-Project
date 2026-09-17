---
document_id: DD-REVIEW-RESOLUTION
version: 0.20.0
status: self-reviewed-pending-independent-G1
scope: frontend-demo-1A
---

# DOC-0.10.0再レビューの修正契約

2026-09-16のINDEPENDENT-DOC-0.10.0のREV-001〜018を扱う。ユーザーの修正・反復レビュー指示に基づく1Aの技術仕様。企業原文・DEC-12〜14の承認内容は変更しない。本書は同じ論点について旧DD/D/SRの記述より優先する。正規型・操作/画面/版カタログ・受入計画も本書と同時更新する。実装・実機・実決済の合格記録ではない。

## IR01 受諾と辞退の最小応答

jobs.accept/declineはJobDecisionReceiptだけを返す。jobId/jobVersion/offerId/decision以外の設備・住所・症状・費用・報告を含めない。受諾後もaccessValidFrom到来前は詳細を取得できない。成功後はP02の結果表示→案件一覧へ戻る。詳細リンクは現在有効な受諾期間内だけ表示し、jobs.getで別途認可する。受諾・辞退の同キー再送とwrites.getResultは、元のuserと当該業者Membershipの有効なpartner.acceptおよび自身のOfferに対する最小受領記録の閲覧権を照合する。受諾開始前・辞退後・Offer期限後の詳細閲覧権を再送条件にしない。この受領記録の認可はD01の冪等照合前に行い、初回操作に必要な未決定Offer/応答期限の判定は既存受領記録のない新規操作だけに適用する。別キーで決定済みOfferを操作するとCONFLICT。期限後も有効な当該Membershipには受領記録だけを返せる。Membership失効/別業者なら返さない。writes.getResultのresourceIdsもjobId/offerIdのみとし、保存済みの案件本体を返さない。

## IR02 所有顧客の不変性

1Aは既存Unit.customerOrgId、Property.customerOrgId、Customer.organizationId、Contract.customerId/customerOrgIdの変更を禁止する。idありsaveで差異があればCONFLICT、全副作用0。新規では認可済み親から所有顧客を確定し、Unit/Space/Property/Contractの親と設備集合を同一顧客に限定する。Space.propertyId変更も移動元/先の顧客が一致する場合だけ許可し、parentSpaceIdは移動先Property所属・非循環、既存子孫Space/UnitのpropertyIdとの整合が保てない移動はCONFLICTとする。別顧客への移管は別IDの新規登録であり、旧履歴の複製/付替えは行わない。organizations.saveで既存Organization.kindの変更も禁止する。

同一顧客内Unit移設は双方のscopeとchangeReasonを検証し、進行中Command/run/operation、active Job、active Restriction/未解決回復caseがある間はCONFLICT。成功でUnit.versionを増分し、旧/新Property・Space、Unit、一覧・集計・関連履歴Queryをinvalidateする。Property単位の閲覧者は移設後のscopeを再照合し、旧scopeのキャッシュを破棄する。請求/点検/Telemetry/報告の所有顧客は変わらない。SR24のDevice移設履歴の積集合認可も維持する。

## IR03 強制解除専用権限

restriction.overrideのみのHQにも管理scope内のrestrictions.list/getを許可する。結果はRestrictionReleaseView（projection=release）で、id/version/createdAt/updatedAt/unitIds/rulesVersion/policy/state/perUnit/recoveryCasesと操作判断に必要な制限時刻だけを返す。請求ID・契約ID・督促文・監査全体は含めない。restriction.manageには従来のRestrictionを返す。A09の一覧入口はoverride専用者にも開放し、list→A10のget→override→getの経路を持つ。A09の作成/適用sectionと契約/請求/設備補助readはmanage保持者だけに要求する。override専用者のlist filterはstatusだけ（invoiceId/contractIdはFORBIDDEN）、件数/並び順も限定投影のscope内のみ。audit.listはaudit.readがある場合だけ取得する。

override後の解除意思があるrelease_requested、または終端記録の未解決回復caseに限り、override専用者もreconcileとretry(phase=release)を許可する。通常適用中のreconcile、retry(apply)、schedule/execute/defer/exempt/cancel/releaseはmanageが必要。overrideは理由必須、最新Restriction.version、新キーで実行。reconcile/retryの版読取もget。解除Commandの状態はget.perUnit/recoveryCasesで追跡できるためcommands.getへの追加権限は不要。overrideの応答は常にRestrictionReleaseView。reconcile/retryのwrite応答とwrites.getResultも現在権限でRestrictionReadへ投影し、manageを失った場合に保存済み完全応答を漏らさない。投影外のresourceIdsも除く。

## IR04 督促の共有された模擬記録

notifications.previewはreadのまま、保存0件。HQ A08は期限超過unpaidのInvoiceを取得→notifications.recipients(target=invoice,templateKey=payment_reminder,channel)で顧客宛先を選択→preview→明示「模擬督促を記録」でinvoices.remindを呼ぶ。入力はinvoiceId/recipientMembershipId/channel/reason、expectedVersionはInvoice.version、idempotencyKey必須。reasonは1〜1000文字。外部送信は行わない。

Repositoryはbilling.manage・請求scope・最新version・now>dueAt・status=unpaid・受信者が有効な当該顧客Membershipでchannel適格なことを同じ遷移で検証する。失効宛先はVALIDATION、paid/processing/期限内はCONFLICT。成功でNotificationを1件と監査1件保存しInvoice.versionを1増分する。NotificationはtemplateKey/type=payment_reminder、inAppはsimulated、email/whatsappはpreview、全channelともnotifications.listに保存される。戻りはInvoiceReminderReceiptでinvoiceId/notificationId/invoiceVersionだけ。通知宛先本人以外のHQに通知本文を返さない。顧客はC08で同じ通知、C11で同じInvoiceを確認する。再送は同キーで同受領記録、二重生成なし。新キーで再督促する場合は再確認した最新Invoice版が必要。読取previewだけではInvoice版も増やさない。

## IR05 予告の時刻と証跡

restrictions.schedule入力からnoticeAtを除く。Repositoryが受付nowをnoticeAtとし、executeAfter>=now+24時間を検証する。対象顧客の有効client Membershipのうち当該ContractとRestrictionの全対象Unitを閲覧できる宛先全員へinApp制限予告を生成し、そのIDをnoticeNotificationIdsへ保存する。宛先0ならVALIDATIONで制限/通知/監査成功記録0件。Restrictionと通知の保存は不可分。予告は顧客の同一タブ通知一覧から確認できる。これは模擬予告であり実配送証明ではない。

execute/retry(apply)は予告IDが同じRestriction・顧客・予告時刻を指すこと、予告後24時間とexecuteAfterの両方が経過したこと、元のpolicy/rulesVersion/対象/原因請求条件を再検証する。予告後に適用内容を変更する場合は新規予告。通知既読は実行の必須条件にしない。過去のfixtureは初期seed専用で通知証跡も一緒に生成する。通常操作/demo.triggerで過去のnoticeAtを差し込めない。時計advanceによる24時間待機を使う。

## IR06 決済試行の排他

Invoiceにつきinitiated/processingのPaymentは合計最大1件。payments.simulate(event=initiate)の同一遷移内でunpaidかつ非終端Paymentなしを検証し、Payment=initiatedを作りInvoice.status=processing/paymentStatus=initiatedとしてInvoice.versionを増分する。初回expectedVersionはInvoice版。同キー再送は当時結果を返す。別キーは最新Invoice版を取得していてもCONFLICTとなり試行を増やさない。

processingイベントはPaymentとInvoice.paymentStatusをprocessingへ更新し各versionを増分する。confirmは両者をconfirmed/paidへ、failはfailed/unpaidへ更新する。Invoice.paymentMethodは失敗後も保持する。確定後の再送は版を増やさない。recordManualはinitiated/processingのいずれがあってもCONFLICT。失敗後の再試行は最新Invoice版・新キー。demo_instructionsはPaymentを作らずこの排他/版に影響しない。状態更新と購読通知は同じ遷移の確定後に配信する。

## IR07 Policy共通フォーム

A05/A11/A12の全kindはname（trim後1〜120文字）、unitIds（重複なし非空）、timezone（対応IANA名）、enabled（boolean）、priority（整数0〜100）を必須入力とする。新規はname空・設備未選択、timezone=Preferences.timezone、enabled=false、priority=50をUIに明示表示する。編集は取得値を使い、Repositoryで欠落値を補わない。Automationの初期値とも整合させる。kind固有必須欄は既存DD/SR28に従い未入力なら保存不可。

A05はalertsの有無に依存せずunits.list→選択units.getで対象設備/能力を得る。A12もunits.list→units.get。A05の候補readはalert.policy.manage、A12はautomation.policy.manageのscope内。通知先は全選択対象/channelの適格集合を用いる。readonly担当にpolicy保存フォームを表示しない。

## IR08 電力量の由来

D07の実績とSR29のdemo_period_comparison基準はorigin=measuredかつ品質validのpowerサンプルだけを積算する。isDemo=trueは全デモ値のラベルでありoriginとは別軸。estimated/inspectionは系列表示可能だが実測slot/coverageに加算せずqualityWarningsにnon_measured_inputを含める。slot始点に一致する候補だけから先にD07のsequence/id順で1件を選び、その後origin/品質/単位を判定する。最新がestimated/suspectの場合、古いmeasured/validへfallbackしない。除外slotがあればcoverage<1で比較値はnull。固定モデル基準は従来どおりmodeledを明示する。SR27の稼働判定も最新powerがmeasuredであることを要求する。

## IR09 音声変更の操作文脈

VoicePanelはresolveIntent成功だけでCommandを作らない。changeの場合は設備・要求温度・現観測を確認表示する。technicianはjobs.listの自己担当候補を対象unitIdで絞りjobs.getのdetailで選択jobIdと有効期間を確認する。候補0なら操作不可、複数なら明示選択。ページのjobIdを初期選択できるが再検証は省かない。technician/adminはreason（trim後1〜1000文字）を必須入力とする。clientはjobIdを送らず通常の自己設備制御条件を使う。

確認時にunits.get（technicianはjobIdも指定）と必要なjobs.getを再取得し、観測/版/担当が変化したら確認表示を更新し再確認を求める。commands.createへjobId・reason・expectedUnitVersionを通常フォームと同じ形で渡す。確認中失効は拒否、cancel/help/unsupported/温度照会ではwrite0件。ホームから開いた場合も同じ選択手順。VoicePanelの依存操作は共通component契約へ明記し、全音声対応ページで遅延取得する。

## IR10 通知の業務分類

Notification.typeをseverityと別に保存する。型はcleaning_due/fault/quality/schedule_change/report_return/completion/payment/payment_reminder/restriction/inquiry。Alert.type=maintenanceはcleaning_due、それ以外のsensor/tamper/reconciliation_requiredはfault、qualityはqualityへ写像する。alertテンプレートのpreview入力にはsourceAlertIdを必須とし、起点Alertの現在scopeとtarget.unit一致をRepositoryで照合して分類する。欠落/target不一致はVALIDATION、scope外はNOT_FOUND。保存Notification.sourceAlertIdにも起点IDを保存する。他テンプレートでは入力禁止・出力null。その他templateKeyは同名type。通知作成後は分類を保持する。cleaning_dueは清掃アイコン、faultは故障アイコンを表示しseverityは重大度の色/ラベルだけを担う。未知分類はD01どおりUNAVAILABLEで正常アイコンへfallbackしない。

## IR11 算定境界の識別

1Aの実績境界IDはac_input_electricity（AC設備入力電力、共用設備/太陽光/蓄電池を含めない）。Sensor.boundaryIdはpowerの場合このID、それ以外null。Measurement.boundaryIdは取り込み時のSensorから確定して不変保存し、現在のSensor移設で書き換えない。inspectionはnull。実績集計は当該境界の測定だけを採用し、欠落/不一致はslot無効としてboundary_mismatchを付ける。EnergySummary.boundaryIdは基準とは無関係にこのID、boundaryは固定説明文。

BaselineInput/EnergyBaselineのboundaryIdはac_input_electricityまたはwhole_building_electricity。後者は比較不一致のデモ用固定モデルのみ許可し、demo_period_comparisonではVALIDATION。境界説明boundaryは編集可能な1〜500文字で、同一性判定には使用しない。比較はID一致が必須で、不一致なら差分/率/削減関連値null、実績自体は維持。MRVConditionsもboundaryIdを必須にし、基準/実績との不一致ならincomplete=true・demo_reviewed不可。保存済みReportにはIDと当時説明をsnapshotとして保持する。UIは固定候補から選択し自由入力IDを受けない。

## IR12 生測定と正規測定

RawMeasurementはデモの入力境界専用でunit:string/value:number|nullを受ける。demo.trigger telemetryはRawMeasurementを受け、公開Measurementへ正規化してから保存する。未知metricは入力VALIDATION。sensor不存在またはscope外はNOT_FOUND。既知sensorの未知unitまたはmetricと異なるunitは、単位換算せずvalue=null/quality=suspect、unitにはsensorの期待単位、qualityReason=unit_mismatch、rawUnitに受信文字列（最大32文字）を保存する。UIは値を欠測として表示し、rawUnitをエスケープして根拠表示する。非有限値はnon_finite、範囲外はout_of_range。正常値はqualityReason/rawUnitともnull。suspect原因があるnullをmissingへ上書きしない。

取り込み時にboundaryIdはSensorから、id/version/tenantId/createdAt/updatedAt/isDemoはRepositoryから生成する。RawMeasurementにこれらの出力専用値を受け取らない。observedAt/receivedAtはRawMeasurementの入力であり、ISO UTC形式と時系列を検証する。外側DemoTrigger.eventIdとmeasurement.eventIdは一致必須、所属違いはscope外ならNOT_FOUND、同一scope内のsensor/unit対応不整合ならVALIDATION。日時の未来不整合はinvalid_time。原因が複数ならunit_mismatch→non_finite→out_of_range→invalid_timeの順。点検入力の正規化も同じ追加フィールドを生成する。公開DTOの未知enumは依然としてUNAVAILABLE。raw受信文字列の未知と公開enumの未知を混同しない。

## IR13 現行受入条件の統一

AT-C01-N/AT-A01-Nの現時点設備KPI遷移先はscopeとpowerStateだけ。periodは時系列/金額集計の条件で、設備一覧へ送らない。戻る操作では元ダッシュボードのperiodを復元する。AT-A08-Nは期限超過unpaid時の模擬督促→入金確認→paid後の督促拒否を順に検証する。previewと保存操作の件数を区別する。Restrictionの入金後遷移はscheduled→cancelled、requested/applied→release_requestedを区別する。

## IR14 現行版の識別

現行仕様はREADMEが示す現行baseline（0.20.0ではDOC-0.20.0、IR75）のmanifest収録ファイル。front matter、現行実装基準と案内をその版に統一する。過去レビュー/DEC/変更履歴/旧runsは当時版のまま保持し、旧合格記録を現版の承認として利用しない。

## IR15 MRV出力の範囲

FR-A14は画面プレビュー・ドラフト保存・版参照・デモ確認まで。CSV/PDF等のファイルexport/downloadは1A対象外。DDの「出力されたデータ」は「保存済み版のプレビュー」を指すよう修正し、実装で未定義のexport機能を追加しない。

## IR16 Factの鮮度と再利用

Factの評価時刻はRepositoryのデモ時計now。fireのoccurredAtは現在tickと一致必須、予約イベントはそのtickへ時計が到達したときに評価する。過去tickへの新規fireはVALIDATION。同じeventIdの既存結果取得は再評価しない。simulateは現在snapshot上の仮評価で保存しない。

| metric | 型/単位 | TTL秒 | 保持 |
|---|---|---|---|
| 通常Metric | D07の数値/単位 | Sensor.staleAfterSeconds | 最新観測 |
| weather_temperature | number / °C | 1800 | 最新観測 |
| tariff | number / MYR_per_kWh | 300 | 最新観測 |
| solar / battery | number / kW | 120 | 最新観測 |
| occupied / peak | boolean / boolean | 120 | 最新観測 |
| location | arrivalまたはdeparture / event | 0 | 同じtickのみ |

未来observedAtはquality不良としてmissing_data、now-observedAt>TTLはstale。TTL境界ちょうどはfresh。通常MetricはD07の範囲、weatherは-50〜100、tariffは0〜100、solar/batteryは0〜1000の有限デモ値。未知metric/型/単位は入力VALIDATION、value=null/quality不良は不成立。保持型は最新observedAtを選び、同時刻ならD02のeventId順を適用してから品質を判定する。古いvalidへfallbackしない。統合factsはfire/予約イベント確定時のみ更新し、simulateで変更しない。locationは次tickへ持ち越さず、同意撤回で未評価locationを破棄。時計tickでTTL失効を再評価し、過去成立を再発火しない。

## IR17 Repository世代と閲覧世代

Session.generation/ChangeEvent.generation/冪等キーのgenerationはRepository世代で、reset時だけ単調増加する。役割切替/ログアウトで共有業務記録を消さない。Session.viewEpochは閲覧世代でsignIn成功・switch成功・signOut・期限失効・resetごとに単調増加し、A→B→Aでも過去値を再利用しない。scopeVersion/権限の変更通知を受けたときも増加し、Sessionを再取得する。両世代は別のカウンタ。

UIのquery key/非同期callbackはRepository instance識別子・generation・viewEpoch・tenantId・membershipId・scopeVersionを捕捉し、描画/通知前に現在値と全て比較する。購読closureにもviewEpochを捕捉し、旧購読は解除済みでも配送済みcallbackを捨てる。reloadは別instance識別子で旧応答を拒否する。中止は業務の取消を意味せず、受理済み処理はRepositoryへ確定し、現在セッションが再認可の上で取得する。ContextにviewEpochは送らず、認可根拠にも使わない。

## IR18 長時間メモリ保持の扱い

REV-018は既存要件の不具合ではなく追加要件候補としてdeferredにする。1Aの保証対象はD10の100設備/1000サンプル規模のデモであり、無制限の長時間連続稼働・メモリ上限は保証しない。reset/reloadまでsnapshot/event/冪等結果を保持する現契約は維持する。自動削除や暗黙resetを加えない。容量上限・拒否/退避方式は継続運用を対象に加える段階の要件とし、未試験の性能合格を主張しない。利用者が選ぶresetの破棄確認は既存仕様に従う。この候補はG1の既存機能欠落件数には数えないが、引継ぎに残す。

## 補足 再レビューで確定した生成・参照経路

IR11/12: Capability.sensorsの境界は機種フォームがmetric=powerならac_input_electricity、それ以外nullを読取専用の派生欄として表示して送信し、機種保存時にその対応を検証する。devices.register/bindで同じ値のSensorを生成する。jobs.saveDraftのInspectionMeasurementInputはorigin=inspection・boundaryId=nullとし、品質原因/rawUnitを正規化時に埋める。正常入力は両者null、測定欠如もnull。Sensor境界を任意の値で偽装して実績に混入させない。

IR17: viewEpoch変更時は全一覧cursorとsnapshot Queryも破棄する。Query cursorは引き続きSR14のRepository世代/scopeに属し、UIは別viewEpochで使い回さない。新閲覧世代では初頁からsnapshotを作り直す。

## IR19 複数設備制限の公開範囲 — CV-001

Restrictionのget/list/forInvoice、write応答、通知、writes.getResultにはSR03の全対象Unit条件を適用する。一部だけ読めるMembershipにはRestriction全体を返さず、個別はNOT_FOUND、一覧は件数にも含めない。顧客はさらに当該Contract/Invoiceの自己顧客条件が必要。通知宛先の所属組織だけで全設備の閲覧を許可しない。schedule時に適格な全対象閲覧者が0ならIR05のVALIDATION。予告作成後に受信者が失効しても保存済み予告証跡は消さず、通知読取を現在scopeで拒否する。設備単位の利用者にはUnitDetail.effectiveControlPolicyの請求情報を含まない投影だけを返す。

## IR20 督促previewの事前条件 — CV-004

payment_reminderのnotifications.preview/recipientsはbilling.manageのHQだけ。target.kind=invoice、now>dueAt、Invoice.status=unpaidをread時にも照合し、不適格状態はCONFLICT、権限不足はFORBIDDEN、scope外はNOT_FOUND。previewはIR04と同じ顧客宛先/channel条件を検証する。preview後に決済が始まればremindで再検証して拒否する。previewのNotificationは未保存であり、その仮IDをmarkReadや通知一覧に流用しない。remind成功時はRepositoryが保存用IDを発行する。

## IR21 Fact評価の時点とセンサー — CV-007

simulate/fireとも新規入力occurredAtは現在のデモ時計tickと一致必須。simulateは現在の保持factsへ入力factsを一時的に重ねて同じ評価関数を使い、保持値・cooldown・継続カウンタを更新しない。通常MetricのTTLは当該Unitの現bindingの同metric Sensorから決める。Sensor不在はmissing_data。factの単位はそのSensor単位と一致必須。過去bindingの値は新bindingで再利用せず、bind時に保持factsを破棄する。solar等の外部デモFactにはIR16の固定TTLを使う。同一入力内のunitId/metric重複、および入力unitIdsに含まれないFactはVALIDATION。別イベントを同tickへ統合するときだけD02のeventId順を使う。同じeventIdの確定fire再送は認可後に既存結果を返し、現在tick一致検証より先に冪等照合する。

## IR22 生測定と点検の保存経路 — CV-006

RawMeasurementはdemo.trigger telemetry専用。jobs.saveDraftは引き続きInspectionMeasurementInputを使い、既知UnitSymbolだけを受ける。点検で不一致単位を指定した場合はVALIDATIONとして報告保存全体を拒否し、部分更新しない。Raw受信の不一致をsuspectとして保存するIR12と区別する。点検の非有限値/範囲外/未来時刻は同じ品質原因を付けてvalue=null、その他正常値はqualityReason/rawUnit=null、origin=inspection、boundaryId=null。公開Measurement生成時にはisDemo=trueを必ず設定する。

## IR23 案件一覧の投影と検索 — PV-001〜003

jobs.listはJobSummary/JobOfferSummary/JobHistorySnapshotのunion。業者について、受諾前と受諾済みだがaccessValidFrom前はoffer、受諾した有効期間内はsummary、終了後はhistory。辞退済み案件は一覧から除き、IR01の受領記録だけを取得できる。未応答のOfferがofferExpiresAtに達した場合も一覧から除く。自己受諾した案件だけに期限後履歴を用意する。

JobOfferSummary.statusは自身のOffer.decisionがnullならoffered、acceptならaccepted。現場のJob.statusを公開statusへ流用しない。severity=nullは未公開でありnormalを意味しない。JobHistorySnapshotはアクセス終了直前に読めた案件のstatus/type/contractorOrgId/completedAt、自己決定イベントとredactedReportSummaryを固定する。asOfは凍結時刻。未到来のアクセスが撤回された場合は受領記録だけで履歴を作らない。期限後の他社作業・報告更新を反映しない。historyのseverity/dueAtはnull、設備ID/住所/連絡情報/費用/Assignmentは含めない。ownDecisionEventsは自分の受諾/辞退操作だけでnote/reportRef=nullとし、他者イベントを混入させない。

フィルター・sort・totalは投影後に評価する。summaryは認可済み設備との結合を含む従来定義。offerはid=jobId、公開status/dueAt/requestedSlotを使い、organizationIdは自社との一致だけを判定する。severity/unitId/membershipId/customerId/propertyIdは非公開なので指定時そのofferは不一致とする。historyもid=jobId、固定status/contractorOrgIdを使い、期間はcompletedAt、nullなら期間条件に不一致。severity/unitId/membershipId/customerId/propertyId/overdueOnly=trueには不一致。offerの期間はrequestedSlot.startAt、overdueは公開dueAtから判断する。sortは公開フィールドのみ、nullはasc/descとも最後、同値はid/jobIdのASCII昇順。現在の非公開Job/Alertを変更してもoffer/historyの件数・順序・フィルター結果は変わらない。

P01/P03/P06はprojectionを判別し、offerでは重大度未公開、historyでは期限欄を「対象外」と表示する。historyクリックはjobs.getの履歴表示だけで設備リンクや編集操作を作らない。partner summaryは同じ投影・同じfilter集合から計算し、totalはその件数、offerCountはoffer.status=offered、業務active/scheduled/inProgress/review/overdueは現在有効なsummaryのみで従来status規則を適用する。将来開始のaccepted offerとhistoryは業務稼働件数に加えない。ゼロの重大度を正常台数として数えない。

## IR24 ページング中の公開範囲縮小 — PV-004

SR14の不変snapshotは業務値の更新に対する保証であり、過去の閲覧権を保持する権利ではない。各ページとwrites.getResult返却時に、現在の委託/担当/資格/資源所属をscopeVersionとは独立して再検証する。snapshot内のいずれかの行で公開projectionが狭くなる、または閲覧権を失う場合はsnapshot全体を無効化しCONFLICT、件数や部分行を返さず初頁から再取得する。新snapshotではIR23のhistory等へ投影する。session失効はUNAUTHENTICATEDを優先する。

Offer/Assignment期限、撤回、Unit移設によるアクセス変更は時計/変更イベントで現在viewEpochを増分し、表示中の該当Query・詳細・snapshot cursorを破棄する。期限直後の旧callbackはIR17で拒否する。Repository読取時にも同じ検証を行い、イベント通知待ちの間に旧データを再返却しない。history凍結はアクセス終了時の内部処理として一度だけ行い、後から期限前データを復元するために現在のJobを読まない。

IR23の技術者経路（PV-005）: 有効Assignmentと閲覧条件を満たす間はsummary/detail、過去に実際の閲覧期間を持ち担当期限が終了した本人にはhistoryだけを返す。社内案件のcontractorOrgIdはnull、自己受諾/辞退のない技術者のownDecisionEventsは空配列。historyはRepository内部で元userId/membershipIdへ束縛し、現在も有効な同Membershipかつ現在scope内である場合だけ返す。他者への再割当で履歴所有者を書き換えない。未開始Assignmentの取消ではhistoryを作らない。technicianの業務件数と担当設備数も現在有効なsummaryだけから計算し、historyを稼働に加えない。


## IR25 設置場所の住所と期限後の報告表示

ユーザー判断: 住所はエアコンに紐づく設置場所を使用する。期限後の報告要約は「報告あり／なし」「受理済み／未受理」のみ。HQが委託時に住所・地域を再入力する設計は採用しない。

JobOfferSummary.siteAddressはJob.unitId→ACUnit.propertyId→Property.addressの値を現在の読取snapshotで取得する。regionLabelは廃止する。受諾前の自社Offer投影に限りこの住所を公開し、Property全体の読取権やUnit ID・入場案内・顧客連絡先の公開を追加しない。住所は既存の設置場所フォームで管理し、委託フォームには読取専用で表示する。null/空白のみならnullへ正規化し「住所未登録」と表示する。物件名や報告本文から補完・地域推定をしない。住所更新はjobs Queryもinvalidateする。ページsnapshotではSR14を適用し、住所変更は新snapshotに反映する。期限後historyには住所を残さない。

redactedReportSummaryは自由文ではなくReportHistorySummary。アクセス終了直前に当該閲覧主体が取得可能な報告版のうち最大versionを対象とする（提出前draftを閲覧できる主体にはdraftも含む）。対象なしは{hasReport:false,acceptance:not_accepted}、対象ありはhasReport=true、当該版に受理イベントがあればaccepted、それ以外はnot_accepted。受理済み旧版があっても閲覧可能な最新改訂版が未受理なら未受理とする。Repository内部で凍結し、本文・写真・メモ・連絡先・報告ID・版番号を含めない。UIはen/ms辞書で「報告なし／未受理」「報告あり／未受理」「報告あり／受理済み」の3通りを表示する。翻訳された自由文をDTOに保存しない。後続の報告提出・受理・住所変更で期限後snapshotを書き換えない。報告書の原本・保存版は従来どおり保持する。

## IR26 案件集計の検索条件

summaries.getのkind=partner/technicianはjobs.listと共通のstatus/statuses/severity/overdueOnlyを追加で受け取る。statusとstatusesの併用、空statuses、不正enumはVALIDATION。kind=customerでこれらの案件専用条件はVALIDATION。既存のcustomerId/propertyId/unitId/unitIds/from/toは維持する。unitIdsはIR23で公開unitIdのあるsummaryにだけ照合し、空配列は0件。partner/technicianの案件数はIR23の投影・同じAND条件を適用した集合から算定し、history/offerの非公開値を絞込に使わない。technicianの担当設備数はその集合の現在有効summaryのunitIdを重複除去する。P01/T01の状態・重大度・期間変更は一覧と集計へ同じ条件を渡し、KPIと一覧の片側だけを絞らない。kind=customerの設備指標は案件条件と混在させない。

## IR27 自動運転の停止理由

既存FR-A04の停止理由表示をRuleBase.disabledReasonで返す。Repository生成のcapability_changed/unit_archived/nullで、フォーム入力に含めない。能力変更で既存Automationが不適合となればenabled=false、disabledReason=capability_changed、versionを増分し、同一遷移の後にautomations/units/capabilitiesへ変更通知する。設備archiveによる停止はunit_archived。新規および利用者によるenabled=true→falseの明示無効化ではnull。停止後の自動再有効化は行わない。明示save(enabled=true)では全対象設備の現能力・archive・scopeを再検証し、不適合なら副作用0、成功なら理由をnullへ戻す。disabledのまま編集すると既存理由を保持する。複数対象で1台でも不適合ならルール全体を停止する。C04/C05/A04は返された理由をen/ms辞書から表示し、停止の理由を知るために顧客へaudit.readを要求しない。Policyにも同じ出力型を用いるが、本節だけでPolicyの新たな自動停止処理を追加しない。

## IR28 機種保存の理由入力

capabilities.saveのchangeReasonは新規では省略可能、既存IDの更新ではtrim後1〜1000文字必須。新規でも指定された場合は同じ文字数で検証する。欠落または空の更新理由はVALIDATION。新規のUIは任意欄、編集のUIは必須欄。正規型のoptionalは入力分岐の表現であり、更新時の検証を省略できるという意味ではない。


## IR29 案件の完了時刻

MaintenanceJob.completedAtはRepositoryが保持するInstant|null。jobs.createおよびplans.generateNextの新規案件はnull。jobs.review(decision=accept)が認可・自己承認検査・版検査・状態検査に成功した同一遷移でstatus=completedとし、完了時刻にその遷移のRepositoryデモ時計nowを一度だけ保存する。対象報告版の受理イベントとjob.completedイベントも同じ時刻を使う。return・拒否・未完了/取消案件はnull。同キー再送、閲覧、メモ、費用更新はcompletedAtを変更しない。完了案件を再開する機能は1Aにない。UI入力にcompletedAtを受け取らず、updatedAt・提出時刻・予定終了から補完しない。

IR23のhistoryはアクセス終了直前のMaintenanceJob.completedAtをそのまま凍結する。終端前にアクセスが終了すればnullのまま、その後の他者の完了を反映しない。期間filterは凍結completedAtに[from,to)を適用し、nullなら不一致。初期seedにcompleted案件を含める場合は受理イベントと同一のcompletedAtを必ず用意する。

## IR30 案件に表示する設備のアラート重大度

JobSummary.severityは当該Job.unitIdの設備に属する、現在の閲覧主体へ公開可能なAlertのうちstatus=open/acknowledgedの最大重大度。critical>warning>normalの順。Job.alertIdsは案件発生の根拠リンクであり、この現在設備重大度の対象をその配列へ限定しない。resolvedは除外し、対象0件はnormal（未解消アラートなし）とする。通信や測定の正常性を保証する値ではなく、offline/stale/missingは別の品質表示を維持する。

一覧・重大度filter/sort・P01/T01集計は同じ認可済みsnapshotの関数を共有し、Job/Alertの片側だけ新しいsnapshotで再計算しない。Alertの作成・解消・重大度変更はjobsとpartner/technician summaryもinvalidateする。JobOfferSummaryとJobHistorySnapshotは従来どおりseverity=nullであり、非公開Alertの存在・重大度を公開しない。例: 同Unitのresolved criticalとopen warning→warning、acknowledged critical追加→critical、全てresolved→normal。これらの変更でoffer/historyのfilter結果は変わらない。

## IR31 報告の共同編集と自己承認

Repository内部に各reportId/reportVersionのcontributorUserIds（重複なしuserId集合）を保存する。公開WorkReport.authorIdは従来の原作者を保持し、項目authorIdもSR07を維持する。自己承認の判定を単一authorIdの比較で代用しない。UIから寄与者を受け取らず、外部向けDTOへこの内部集合を追加しない。

初回draft作成者を集合へ登録する。新しい内容版を作るときは直前版の集合を継承し、本文・点検項目・測定・部品・次回対応・写真参照のいずれかを実際に追加/変更/削除した操作の実行userIdを追加する。attachments.addで写真を追加した人も含む。再割当後、on_hold/reworkからのコピー版も元版の集合を継承する。閲覧、割当のみ、完全な無変更save、提出のみ、受理/差戻し操作のみでは新たな寄与者を追加しない。冪等再送で集合や内容版を増やさない。集合は版の提出時に固定し、過去版を後から変更しない。

jobs.reviewは対象の提出版の集合を読み、現在Session.userIdが含まれる場合、accept/returnともFORBIDDEN、副作用0（D01の拒否監査のみ）。別Membership、partner.review、HQの通常review、hq_escalationのすべてに同じ検査を適用する。別担当の承認を得るために原作者・項目作者・寄与者履歴を書き換えない。集合が欠けたseed/旧版を非寄与と推測せずUNAVAILABLEでレビューを拒否し、fixtureを修正する。seed報告にも生成履歴と整合した寄与者集合を必須とする。

再現例: T1のdraftをT2へ再割当し、T2が本文だけ、測定だけ、写真だけのいずれかを変更して提出した各ケースで集合はT1/T2を含む。T2が別Membershipの品質担当またはHQへ切り替えてもFORBIDDEN。作成・編集していない有効なT3は他の認可/状態条件を満たせば受理できる。T2が割当・閲覧・提出だけを行い内容を変更しない場合には寄与者に追加せず、自己承認条件では拒否しない（通常の品質担当権限は別途必須）。


IR31のUI投影: reports.getとWorkReportを返すwrite/再送応答には、現在の閲覧主体別reviewAvailabilityを合成する。元の報告本文・内容版・寄与者集合は変えない。判定順は、現在の対象案件でpartner.reviewまたはHQの通常/引継ぎreview資格がない→permission_denied、対象版の寄与者に現在userIdが含まれる→self_authored、最新報告版でない→not_current、Job.statusがsubmittedでないか対象版が未提出→not_submitted、すべて通過→allowed=true/reason=null。読取scope自体がない場合は従来どおりDTOを返さずNOT_FOUND。資格は現在の委託・所属・scope・自己承認以外の既存review条件も含む。引継ぎモード・理由・input/options版の送信時検証は別途維持する。

P05/A06は取得したreviewAvailability.allowed=falseで受理/差戻しボタンをdisabledにし、reasonを翻訳して表示する。元作者IDだけからボタン可否を計算しない。再割当・報告改版・権限変更は当該Queryをinvalidateし、確認中の変更は再取得して確認をやり直す。Repositoryは送られたUI可否を信用せずjobs.review実行時にIR31を再検証する。contributorUserIds欠落時はUNAVAILABLEとし、可否を許可へfallbackしない。


## IR32 案件一覧と集計の設備集合条件

jobs.listもsummaries.get(kind=partner/technician)と同じunitIdsを受ける。Queryの共通制約を適用し、認可・IR23投影後のsummary.unitIdが配列に含まれる行だけを対象とする。空配列は0件。unitIdも指定された場合はAND。offer/historyには公開unitIdがないため、非空配列でも不一致とする。非公開Jobの設備IDで照合しない。P01/T01は両読取へ同一の共通filter値を送信する。集計固有・一覧固有の未許可filterを暗黙に捨てない。

## IR33 監査画面の参照経路と検索境界

A16はaudit.listを必須Queryとし、失敗を正常空表示にしない。機器イベントは独立した補助パネルでdevices.listから認可済み候補を取得し、deviceId選択後だけdevices.events({id:deviceId,query})を呼ぶ。未選択は選択案内、候補0件はempty、一覧失敗はパネル内retry、無効・非可視の選択IDはnot-foundとして停止し別機器へ置換しない。選択deviceIdをURLへ保持し、Back/Forwardも同じ手順で復元する。audit.readの候補/履歴read許可は既存操作カタログどおりで、device.manageを追加で与えない。

監査のtargetRefから既存の詳細画面へ移動する場合、kind=jobは/admin/jobs?jobId=:id、restrictionは/admin/restrictions/:id、deviceは/admin/devices?deviceId=:idを使い、それぞれの既存read権限を必要とする。commandはcommands.getの既存権限を持つ場合だけ別途取得し、そのunitIdで/admin/units?unitId=:unitIdへ移動する。A16のaudit.readだけで他資源への権限を拡張しない。リンク解決は共有Navigation/feature hookで行い、権限なし・未知kindはマスク済み監査詳細だけを表示する。削除済み/失効はリンク先でnot-found、元の監査は保持する。

audit.listは全filterを認可済み集合へ適用する。別テナントのcorrelationIdも存在しないcorrelationIdもitems=[]/total=0/nextCursor=nullとし、全テナントを先に検索してNOT_FOUNDへ分岐しない。個別資源のgetに対するD01のNOT_FOUNDとは区別する。監査の追加はRepository内部の業務イベントだけでありA16にwrite操作はない。


## IR34 ユーザー確定の権限と案件ソート

DEC-17: 制限操作の権限は2種類。defer/exempt/cancelはrestriction.manage、overrideはrestriction.override。両方を持てば両方の操作を行えるが、片方からもう片方を暗黙付与しない。その他のschedule/execute/release等の既存manage条件、IR03のoverride後の解除追跡だけを許す条件は維持する。メニュー/ボタンとRepositoryの双方で判定し、理由・scope・状態・版の検証は省略しない。権限不足の直接writeはFORBIDDEN、業務変更0件、D01の拒否監査のみ。これはFR-A10の一覧と詳細の矛盾修正であり、新permissionは追加しない。

DEC-18: 案件一覧は状態（業務順）・重大度・期限のソート項目と昇順/降順を提供する。既定はstatus asc、同順位は常にid/jobId ASCII asc。状態の比較順位は次の全10状態で固定する。

requested → offered → accepted → assigned → in_progress → on_hold → submitted → rework_requested → completed → cancelled

これは表示順位であり状態遷移の許可表ではない。状態descは上記順位の逆順。同状態のIDはdescでもasc。severityはnormal < warning < critical、dueAtはUTC時刻で比較し、nullはどちらの方向でも最後。翻訳ラベルや現在ページ内だけでsortしない。認可→IR23の公開投影→filter→全snapshotのsort→ページ分割の順。offerの公開status、historyの凍結statusを使い、非公開のJob状態を参照しない。未知statusはD01のUNAVAILABLEで拒否する。

正規型にJOB_STATUS_ORDERを公開する。これはデモ契約定数でありアプリ実装ではない。Repositoryのsort省略もstatus asc;id asc。既存のid sortは内部契約として保持するが、利用者の選択肢はstatus/severity/dueAtの3種類。全jobs.listを使う案件一覧に適用する。案件候補の読取もsort省略時は同じ既定値を使う。

UIのURLキーsortはfield:direction（例sort=status:asc、sort=dueAt:desc）。省略はstatus:asc。不正なfield/方向、重複sort、空値はVALIDATIONで条件修正を求める。UI選択肢外のidをURLから指定することは拒否する。選択変更は他のfilterを保持し、URLを更新してcursorを除去、新条件の初頁を取得する。Back/ForwardはそのURLのsort/filterを復元し、古いcursorを新条件へ流用しない。Query keyにも正規化sortを含め、遅い旧条件応答で新一覧を上書きしない。購読による再取得では現在のsort選択を保持する。言語切替は順位・ID・UTC値を変えない。

ソートコントロールにはラベルを付け、キーボードとモバイルでも選択可能にする。選択後は新条件のloadingを表示し、失敗はerror/retry、0件はempty。旧順の行を新しい順序で取得済みと表示しない。対応する列見出しにはaria-sortを反映する。「既定に戻す」はsortだけをstatus:ascへ戻して初頁から再取得し、他のfilterは保持する。KPIはsortに依存せず、summaries.getへsortを渡さない。

## IR35 解除要求の起動経路とrestrictions.releaseの前提 — FRV-001

解除要求（state=release_requested）の起動経路は次の①〜③とIR96の④取消（restrictions.cancel）だけとし、いずれも同じ内部遷移関数を使う。①入金確認: payments.confirm／payments.recordManual／payments.simulate(confirm)が原因請求（causeInvoiceIds）を全件paidにした同一遷移で、scheduledはcancelled、requested/appliedはrelease_requestedへ遷移し、releaseIntent={source:'payment',at:now,actorMembershipId}を保存する。②猶予・例外: restrictions.defer/exemptがrequested/appliedに対して行われた同一遷移でrelease_requestedへ遷移し、source='exception'。③強制解除: restrictions.overrideでsource='override'。

release_requestedへ遷移した同一遷移で、perUnitごとにD03の解除評価を1回実行する。applyState=appliedかつonlineの設備にはremove_restriction Commandを作成しreleaseState=requested、not_sent/not_appliedはnot_required、sent_unknownはwaiting_reconcile、offlineのappliedはfailedではなくreleaseState=none/pendingReason=offlineとして明示retry(phase=release)を待つ。呼出者がclientでも解除Commandは作成される（Command.actorMembershipIdはRepositoryの内部主体'system-restriction'、監査のactorは入金確認の実行者）。

`restrictions.release`はrestriction.manage保持者による解除要求の明示操作であり、前提はstate∈{requested,applied,release_requested}かつ「原因請求が全件paid」または「exception/graceが有効」のいずれか。未入金かつ猶予・例外なしはFORBIDDEN（契約制限違反）。requested/appliedからは上記②と同じ遷移（source='manual'）を行う。既にrelease_requestedの場合は冪等に現在のRestrictionを返し、Command・監査（拒否監査を除く）・versionを増やさない。再送や失敗設備の再要求はrestrictions.retry(phase=release)だけが行う。AT-A09-N④の「release」はこの冪等応答であり、release_requestedへの遷移自体は入金確認の遷移で起きる。AT-C11-N④はclientの入金確認だけで①が起動することを検証する。

## IR36 デモ時計の進行モデルとセッション寿命 — FRV-002

デモ時計はreset/reload時にfixture.clock（2026-09-14T01:00:00.000Z）から始まり、実時間と同じ速さで単調に進む。demo.advanceClock({to})はtoが現在時計以上なら前方へジャンプし、ジャンプ中に到来する期限（Command expiry、Offer/Assignment/資格の期限、staleAfterSeconds、Fact TTL、predicted occurrence、DiagnosticRun endAt、cooldown）をD04のイベント優先順で時刻順に処理する。toが現在時計より前の場合、reset直後で業務イベントがseedのeventCursorから増えていないときだけ「初期時刻の設定」として許可し、それ以外はVALIDATION（AT-C04-E③はreset直後にこの設定を行う）。

Sessionの寿命30分はデモ時計で測るが、advanceClockのジャンプはセッション寿命を消費しない。ジャンプ確定時に有効なSessionのissuedAt/expiresAtを同じ差分だけ後ろへずらす（初期時刻の設定時も同様）。利用者操作による延長はIR55のdemoSession.extendだけで行う（IR79）。セッション失効の試験はdemo.trigger(session_expired)またはジャンプなしの経過で行う。Membership.validUntil、Offer、Assignment、資格、契約、請求期限はジャンプで通常どおり失効する。ジャンプで失効した資源を表示中のQueryはIR24のとおりviewEpochを増分して破棄する。

## IR37 transport障害の注入とネットワーク断 — FRV-003

DemoTriggerにeventType='transport'を追加する。入力は`{operation:OperationName, outcome:'UNAVAILABLE'|'TIMEOUT'|'RATE_LIMITED'|'DELAY', retryAfterSeconds:number|null, delayMs:number|null, remainingCalls:number}`。remainingCallsは1〜100の整数で、当該operationの次のremainingCalls回の呼出しに適用し、適用のたびに減らす。outcome=UNAVAILABLE/TIMEOUTはRepositoryが業務処理を実行せずDomainError{code}でrejectする（writeは副作用0、冪等キーも記録しない）。RATE_LIMITEDはretryAfterSecondsを必須（1〜3600）としD04どおり受理を止める。DELAYはdelayMs（1〜60000）だけ応答を遅らせてから通常処理し、D10の意図的slow（3000ms）と10秒timeout表示（12000ms）はこの経路で注入する。受入条件の「units.listをUNAVAILABLEにする」「jobs.createをUNAVAILABLE」「submitをUNAVAILABLE」はこの注入を指す。read側のUNAVAILABLE自動再試行はremainingCallsを消費するため、AT-C01-E③のように最終的なerror表示を検証する場合はremainingCalls>=3を与える。

DemoTrigger network(connected=false)は「ネットワーク断の模擬」で、全Repository操作（demoSession/preferences/demo.*を除く）をDomainError{code:'UNAVAILABLE', messageKey:'errors.network_disconnected', retryAfterSeconds:null}でrejectし、業務処理を実行しない。events.subscribeは断中にイベントを配送せず、UIは購読を解除して「更新停止」と最後の成功時刻を表示する（画面状態offline）。connected=trueで再接続すると、UIはD07の再snapshot→再購読で復帰し、断中に確定していたRepositoryイベントはリプレイで反映される。ネットワーク断と機器offline（code OFFLINE）は表示領域・文言を分ける。断中にexpireしたCommand/期限は時計どおり失効する。

## IR38 顧客起点案件の期限導出 — FRV-004

jobs.createのdueAtはjob.manage保持者（HQ）だけが指定できる。clientがdueAtを送るとVALIDATION（fieldErrors.dueAt）。省略時はRepositoryがdueAt=requestedEndを保存する（D16の生成案件と同じ規則）。HQが指定する場合はdueAt>=requestedEndを検証し、下回る場合はVALIDATION。dueAtの後からの変更操作は1Aにない（新規案件で対応する）。JobSummary.dueAt、overdueOnly、dueAt sort、technician/partnerのoverdueCountはこの保存値だけを使う。

## IR39 archived資源の可視性 — FRV-005

Property/Space/ACUnitのarchived=trueは、全一覧（properties/spaces/units.list）、summaries.get/admin.summaryの件数・分母・KPI、候補選択（units.list経由）、通知宛先解決、新規業務のscope判定から除外する。archivedフィルターは1Aで提供せず、除外は無条件。個別取得（units.get、およびspace/property選択の解決）はclient/contractor/technicianにはNOT_FOUND、HQのasset.manage保持者だけにarchived:trueの読取専用DTOを返し、制御・割当・契約・制限・Deviceの各操作はCONFLICT（理由archived）。既存のJob/Report/Audit/Command履歴からのUnit参照はD05のとおり履歴閲覧を許し、履歴画面はarchivedラベルを表示する。archive操作の同一遷移でunits/properties/spaces/summaries/自動運転のQueryをinvalidateする。unitIdsフィルターにarchived IDを含めた場合は不一致として0件に数える。

## IR40 顧客数の母集団 — FRV-006

Customer（サービス台帳）とOrganization(kind=customer)は1Aでは1対1。customers.saveは同じorganizationIdの2件目をCONFLICTとし、organizationIdはkind=customerかつ管理scope内であることを検証する。AdminSummary.customerCountはCustomer.status=activeかつ対応Organization.status=activeのCustomer件数（filters.customerId指定時はその1件の該当有無で0/1）。inactiveの顧客の設備・案件・請求はKPIから除外しないが、Customer/Organizationのどちらかがinactiveなら顧客数に数えない。AT-A01-N/Bの「active顧客」はこの定義で作る。

## IR41 時系列プリセット1h/24h — FRV-007

DD-C07およびDD-T03の期間プリセットは1h/24h/7d/custom。toはSR17と同じくデモ時計をUTC分境界へ切り捨てた値。1hは[to-60分,to)、24hは[to-1440分,to)の固定長・移動窓（暦日を使わない）。7dはSR17の暦日規則。customはfrom<to、最大366日。プリセットのラベルはIR17のviewEpochとURLに保存したfrom/toで再現し、明示更新時に選択中プリセットを再計算する。当日最初の1分でfrom=toとなるのはtodayだけで、1h/24hは常に非空。C01/C06/A01/A13のtoday/7d/30dはSR17を変更しない。

## IR42 役割別投影の非公開項目 — FRV-008/FRV-009

readとwriteの応答に次の投影規則を追加する。client向けJobDetail: offer=null、assignment={id,version,jobId,technicianMembershipId,scheduledStart,scheduledEnd,status,validFrom,validUntil,reason:null}、costsはvisibility=customerのみ、draftReportRef=null、reportRefsは受理済み版のみ、contactWindowは自己入力値。technician向けJobDetail: offer=null、costs=[]、assignment.reasonは自己の割当のものだけ。contractor向けJobDetail: 自社Offerのみ（他社のOffer/declineReasonは含めない）、costs=[]、assignment.reasonは自社割当のもの。HQは全項目。

jobs.events: client向けにはnote.visibility=internalのイベントを配列から除外し（note=nullにするのではなく行ごと除外）、totalも除外後の件数。contractor向けは自社案件のinternal/customer両方、technicianは有効担当案件の両方。除外したイベントの存在を件数・cursorで漏らさない。JobNoteの本文はテキストノード表示（D08）。

client向けRestrictionDetail（forInvoice/通知リンク）: events[]は state変更イベント（action∈{scheduled,requested,applied,release_requested,released,cancelled,defer,exempt}）に限定し、actorId='masked'、actorRoleAtTime=admin、reason=null、maskedBefore/After={}。exception.reasonはnull、graceUntil/exception.untilは返す。perUnitはそのまま。HQ向けは全項目。Restriction.reason（予告理由）は顧客向け文言としてHQが入力する項目であり、DD-A09の入力欄に「顧客に表示される」と明記する。

members.list/members.eligible/members.capacityのcontractor向けMembership投影: permissions=[]、scopes=[]、qualificationsは自社技術者分のみ、validFrom/validUntilは返す。HQ向けは全項目。

## IR43 Device登録時のSensor生成 — FRV-010

devices.registerのsensorTypesは重複なしのMetric配列で空を許可する（device-no-sensor）。各metricは対象UnitのCapability.sensorsに同じmetricの定義が存在しなければVALIDATION（fieldErrors.sensorTypes）。Sensorはregister時に生成し、unit/staleAfterSeconds/boundaryIdをCapability.sensorsの同metric定義から複写し、idをRepositoryが採番、calibratedAt=nullとする。bindで別UnitへつなぐときはSR24のとおり新sensorIdを発行し、同じ複写規則をbind先の能力に適用する。能力に定義のないmetricのTelemetryは受け取らない（demo.trigger telemetryのsensor不存在=NOT_FOUND）。firmwareVersionの初期値は対象能力のfirmwareCandidates先頭（ASCII昇順）、候補が空なら'unknown'。

## IR44 表示書式・翻訳・描画例外・補足 — FRV-019〜025

- Intl locale tagはen→'en-MY'、ms→'ms-MY'。金額はIntl.NumberFormat(tag,{minimumFractionDigits:2,maximumFractionDigits:2})の数値部と通貨コードを半角空白で連結し「120.00 MYR」の順で表示する（通貨記号・currencyDisplayは使わない）。温度は小数1桁（"24.0°C"）、湿度/割合は小数1桁、電力量1桁、kgCO₂e 1桁、ppm/µg/m³は整数。丸めは十進の四捨五入（half away from zero）で、Number.prototype.toFixedを丸めに使わない。受入条件の数値（24°C等）は値の指定であり、表示はこの桁数に従う。日時はIntl.DateTimeFormat(tag,{timeZone:Preferences.timezone, dateStyle:'medium', timeStyle:'short'})とタイムゾーン略称を併記する。
- 翻訳キーがms辞書に欠けていればen辞書へfallbackし、開発ビルドでconsole.warnを出す。両方欠ければキー文字列をそのまま表示する。en/msのキー集合の一致はNFR-07のlintで検査し、不一致はビルド失敗とする。
- 描画時の未捕捉例外は共通ErrorBoundaryで捕捉し、correlationId付きの全画面errorとして表示する（白画面にしない）。再試行は同一routeの再マウント、失敗が続けばrole homeへの導線。RepositoryのDomainErrorは従来どおりAsyncBoundaryが扱う。
- voice.resolveIntentの権限がないrole（contractor、権限を持たないHQ/技術者）にはVoicePanelを表示せず、ヘッダーの音声切替はdisabledで理由（voice.unavailable_for_role）を表示する。
- Summary.countsで当該kindに該当しないカウンタ（customerのoffer/active/review/scheduled/inProgress/overdue/assigned、partner/technicianのpowerOn/powerOff/powerUnknown/alertCount等）は0を返し、UIは表示しない。nullにしない。
- demo.trigger.scenarioIdは1〜64文字の任意ラベルで、DemoEvent.typeと監査に記録するだけで挙動を変えない。S01〜S08のシナリオ名を推奨する。
- units.save.installedAtはInstant|nullを受け、nullは「未登録」として保存し、未来日はVALIDATION。既存の未登録設備を編集する際にnullのまま保存できる。

## IR45 合成テレメトリーと生存信号の生成 — REV18-001

デモ時計は実時間で進む（IR36）。Repositoryは生存シミュレーターを持つ。有効な間（reset/reload直後はenabled=true）、デモ時計がUTC分境界（秒・ミリ秒=0）の時刻tに到達するたびに、非archivedで現binding（Device.unitId=Unit.id）のDeviceを持つUnitごとに、Device.connection=onlineかつDevice.powerSignal≠offの場合だけ、次を同一遷移で行う。

1. Device.lastSeenAt=t。IR47によりACUnit.lastSeenAtも同値になる。
2. Deviceの各Sensorについて、同じsensorIdの最新Measurement（observedAt降順、sequence降順、id昇順の先頭）がIR77の複写条件（origin=measured、quality=valid、value≠null）を満たす場合だけ、そのvalueとunitを複写した新Measurementを1件保存する。observedAt=receivedAt=t、origin=measured、quality=valid、qualityReason=null、rawUnit=null、sequence=複写元+1。条件を満たさないSensorと過去Measurementが無いSensorは生成しない。乱数を使わない。
3. ACUnit.observedState.observedAt=t。power/celsius/mode/fanLevelは変更しない（Command ackだけが変更する）。

生成で変わるDevice.lastSeenAt、ACUnit.lastSeenAt、observedState.observedAtは観測時刻フィールドであり、Device/ACUnitのversionとupdatedAtを変えない。したがって利用者が取得したexpectedUnitVersionは生成によって失効しない。生成したMeasurementはIR71のmeasurement/deviceイベントとして通知する。reset/reload時刻そのものでは生成せず、次の分境界から生成する。

connectionがonline以外、powerSignal=off、未bind、Sensor無しのUnitは生成しないため、D07/SR27どおりstale/unknownになる。接続断は明示のdevice event（communication_lost）だけで判定し、lastSeenAtの経過時間から自動でofflineにしない。

demo.advanceClockのジャンプは中間の分境界を補完生成しない。ジャンプ先が分境界ならその時刻で1回生成し、そうでなければ次の分境界まで生成しない。補完しない区間はD07の欠測slotとしてcoverageを下げる。

DemoTriggerに`{eventType:'simulator';enabled:boolean}`を追加する。enabled=falseで以後の生成を止め、trueで次の分境界から再開する。受入試験はGivenで測定値・観測時刻を固定するcaseの最初にsimulator enabled=falseを与える。自動更新そのものを検証するcaseだけenabled=trueのまま実行する。表示中の画面はデモ時計の1秒tick通知でstale・期限・残秒を再評価し、tickだけではQueryを再取得しない。

## IR46 制限中の操作可否 — REV18-002

UnitDetail.effectiveControlPolicy.state=restricted（phase=requested/applied/release_requested）の間、UnitActionの可否は次の表だけで決める。unrestricted（予定・猶予/例外中のscheduled、released、当該Unitのnot_required確定後）は制限による拒否をしない。

| UnitAction | temperature_limit（minimumCoolingSetpoint=m） | power_off |
|---|---|---|
| set_power power=true | 許可 | FORBIDDEN |
| set_power power=false | 許可 | 許可 |
| set_temperature celsius=c | c>=mなら許可、c<mならFORBIDDEN | FORBIDDEN |
| set_mode | 許可 | FORBIDDEN |
| set_fan | 許可 | FORBIDDEN |
| ventilate | 許可 | FORBIDDEN |

適用経路はclient/HQ/technicianのcommands.create、voiceのchange確認、automations（Command候補をsuppressed/restrictedにする）、diagnosticRuns.create（startActionとendActionのどちらかが禁止ならFORBIDDEN）、試運転終了時の再判定（禁止ならend_blocked）の全て。restriction.overrideを持つHQもcommands.createでは迂回できず、解除はrestrictions.overrideだけで行う。FORBIDDENはD01順位4、messageKey=errors.restriction_active、fieldErrors.action。CommandPanelは同じ表で候補をdisabledにし理由を表示し、温度入力の下限はmax(capability.temperature.min, m)とする。

## IR47 設備の接続・電源・取り外し状態の導出 — REV18-003

ACUnit.connectionとACUnit.lastSeenAtはRepositoryの派生値で、units.saveの入力に含めない。現bindingのDeviceがあればそのconnection/lastSeenAtを複写し、無ければconnection=unknown、lastSeenAt=null。Deviceのconnection/powerSignal/tamperが変わる遷移で、同時にACUnit.versionを増分しIR71どおり通知する。lastSeenAtだけの変化（IR45の生存信号を含む）はversionとupdatedAtを変えない。

制御の受付（commands.create、diagnosticRuns.create、voiceのchange確認、restrictions.execute/retry(apply)の配送判定）は、派生connection=onlineかつDevice.powerSignal≠offの場合だけ配送可能とする。それ以外はOFFLINEとし、messageKeyはerrors.device_offline／errors.device_unknown／errors.device_connecting／errors.device_error／errors.device_power_lostのいずれか（powerSignal=offを最優先、次にconnection値）。制限のapplyはD03の未配送意図（not_sent）として扱う。

Device.tamper=detectedは制御可否に影響しない。CommandPanelとUnitDetailに取り外し注意を表示する。ObservedState.powerはCommand ackとtelemetryだけで更新し、powerSignalから書き換えない。UIは「電源信号断」を接続状態とは別の表示領域に出す。summaries.get/admin.summaryのonline/offline/unknownはこの派生connectionで数え、connecting/errorはD14どおりunknownへ加算する。

## IR48 未応答Offerの期限到来 — REV18-004

jobs.offerの受付状態はJob.status=requestedだけで、それ以外はCONFLICT。入力はnow<offerExpiresAt、accessValidFrom<accessValidUntil、offerExpiresAt<=accessValidUntilを満たさなければVALIDATION。

デモ時計がdecision=nullのOffer.offerExpiresAtに到達した時点（D04の期限失効の順位）で、同一遷移によりJob.statusをofferedからrequestedへ戻し、contractorOrgId=null、Job.version+1、JobEvent（action=offer_expired、actorUserId=system-demo）を1件保存する。Offer.decisionはnullのまま保持し、期限切れはnow>=offerExpiresAtから導出する。通知は生成しない。業者一覧からの除外とaccept/declineのCONFLICTはIR23/AT-P02-Eのとおり。HQはrequestedに戻った案件へ、新しいofferIdでjobs.offerを再実行できる（同じ業者も可）。

## IR49 技術者の担当案件の閲覧窓と作業窓 — REV18-005

active Assignment 1件につき、閲覧窓=[Assignment.createdAt, scheduledEnd)、作業窓=[scheduledStart, scheduledEnd)とする。D06の「AssignmentのvalidFrom/UntilはscheduledStart/Endと同じ」は作業窓を指す。

閲覧窓内の担当技術者には、jobs.list/getでJobSummary/JobDetail（IR42投影）を返し、summaries.get(kind=technician)のassignedCountと担当設備数に含め、通知リンク/technician/jobs/:idを表示可能とする。作業窓の開始前にFORBIDDEN（messageKey=errors.assignment_not_started）とする操作は次の2群。(a) 社内・外部を問わず案件を起点とする操作: jobIdを伴うunits.get、jobs.start、jobs.saveDraft、jobs.submit、attachments.add、jobIdを伴うcommands.create、diagnosticRuns.create、jobIdを伴うdevices.*のwrite。(b) 外部技術者の設備関連の読取: units.get、telemetry.*、alerts.*、devices.*（SR03により外部技術者の閲覧は自己Assignmentとの積集合のため）。社内技術者がjobIdを伴わずunit scopeで行う読取は(b)に含めず、SR03のscopeどおり許可する。UIは作業開始時刻と「開始時刻から操作できます」を表示し、該当の設備リンクと操作をdisabledにする。作業窓の終了後はIR23/IR24のhistoryだけを返す。

外注技術者の閲覧窓・作業窓はさらにOfferのaccess窓との積集合とする。社内技術者のunit scopeによる設備読取はSR03どおりだが、案件に紐づく書込みは作業窓内でなければFORBIDDEN。作業窓の延長はjobs.assign（同じtechnicianMembershipIdも可）で新Assignmentを作り旧Assignmentをrevokedにする（in_progressでは理由必須）。AT-T01-NのvalidUntil=2026-09-20はscheduledEnd=2026-09-20T00:00:00.000Zを意味する。

## IR50 KPIからの一覧遷移とURL許可キー — REV18-006

各ScreenのURL許可キーは、screen-catalog.url_selectionと共通キー{tab, sort, period, from, to}の和集合とする。共通キーは当該Screenが対応する入力を持つ場合だけ解釈し、持たない場合はSR11どおり除去する。複数値（unitIds、connections）はカンマ区切りで、ASCII昇順・重複除去に正規化する。booleanはtrue/falseの文字列だけを受け付ける。

顧客の稼働KPIの遷移先は`/customer/properties?propertyId=:propertyId&powerState=on|off|unknown`とする（propertyId未選択時はpowerStateだけ）。SCR-C02は選択物件（未選択時は全物件）のunits.listをpowerState・connections条件で表示する設備一覧sectionを持つ。通信KPIの遷移は`connections=connecting,error,unknown`。HQの稼働KPIは`/admin/units?customerId=:customerId&propertyId=:propertyId&powerState=on|off|unknown`。どちらもperiodを送らない（IR13）。Backは遷移元URLを復元する（D13）。

## IR51 未対応アラート件数の母集団 — REV18-007

Summary.counts.alertCount（kind=customer）、AdminSummary.alertCount、UnitSummary.activeAlertCountは、投影・scope適用後でarchived Unitを除き、status∈{open, acknowledged}かつseverity∈{critical, warning}のAlert件数とする。severity=normalのAlertは件数に含めない（健康サマリー）。IR30のJobSummary.severityは従来どおりnormalを含む最大値とする。FR-C08/AT-C08-Bの「未対応件数」はこのalertCountを指し、通知の未読件数（notifications.listのunreadOnly=trueのtotal）とは別の値として別に表示する。

## IR52 生活パターン条件の評価 — REV18-008

Condition `{type:'pattern', localTime:'HH:mm'}`は「毎日、ルールのtimezoneでlocalTimeと一致する時刻に1回だけ成立する合成条件」とする。Factは使わない。デモ時計がそのローカル時刻（秒・ミリ秒=0）のtickに到達したとき、RepositoryがD02の内部評価（phase=condition）を行う。ジャンプで通過した時刻は追いつき発火しない（IR36）。ジャンプ先がちょうど一致時刻なら発火する。UIのautomations.simulate/fireではoccurredAtのローカル時刻が一致するときだけ成立し、それ以外はsuppressed/no_match。保存時にD09の366日DST検証を適用し、存在しない／曖昧な時刻はVALIDATION。画面ラベルは「デモ生活パターン（毎日の固定時刻）」とし、学習・推定済みとは表示しない。位置情報の同意は不要。

## IR53 同意撤回とルール状態 — REV18-009

consents.update(granted=false)の同一遷移で、同じownerMembershipIdのAutomationのうちkind=event・condition.type=location・enabled=trueのもの全てをenabled=false、disabledReason=consent_revokedとし、各versionを1増分してautomationsへ変更通知する。RuleBase.disabledReasonの値にconsent_revokedを追加する。再同意（granted=true）はルールを自動で有効化しない。利用者が明示save(enabled=true)して成功した時点でdisabledReason=nullに戻る（IR27）。評価ではenabled=falseのルールをsuppressed/disabledとし、DecisionReason=consent_revokedは評価時点でgranted=falseかつenabled=trueのlocationルールだけに使う。

## IR54 予定・生活パターンの発火経路 — REV18-010

デモ時計による予定（schedule_start/schedule_end）とpattern条件の発火は、D02のRepository内部評価だけが必須の経路であり、ログイン中のroleや画面の表示に依存しない。UIはこれらの発火のためにautomations.fireを呼ばない。automations.fireは、C04/C05のeventタブとA11/A12の「デモイベント発火」から、現在tickの合成factsを即時評価する任意の操作である。同じtickで内部評価とUIのfireが重なった場合はD02の同一tick結果を参照し、Commandを再生成しない。DD-C04の旧記述「デモ用の時計から発生したイベントはautomations.fireに渡し」は本節で置換する。

## IR55 セッション期限の予告と延長 — REV18-011

Session.expiresAtの120秒前（デモ時計）に、SessionExpiryDialog（role=alertdialog）を表示する。ボタンは「延長する」（初期focus）と「サインアウト」。延長は`demoSession.extend`（input={}、result=Session、write、demo-only）で、有効なSession（now<expiresAt）のexpiresAtをnow+30分にする。issuedAt・generation・viewEpochは変えず、回数制限はない。期限後の呼出しはUNAUTHENTICATED。業務監査・writes.getResultの対象外（D14のローカル境界）。ダイアログは業務データと入力値を変更しない。表示中に期限が到来した場合はD09どおり破棄して/loginへ移動し、未保存draftの破棄を通知する。advanceClockのジャンプはIR36でexpiresAtも同じ差分だけずれるため予告を越えない。これはWCAG 2.2 SC 2.2.1（延長手段と20秒以上の猶予）への対応であり、適合を主張しない。D09の「利用者操作による延長なし」は本節で置換する。

## IR56 案件の取消の許可状態 — REV18-012

jobs.cancelの許可は次の表だけで決める。cancelReasonはtrim後1〜1000文字で必須。業者と技術者はjobs.cancelを持たない（FORBIDDEN）。

| 現在のJob.status | client（自己の案件） | HQ job.manage | 成功時の結果 |
|---|---|---|---|
| requested | 可 | 可 | cancelled |
| offered | 不可 | 可 | cancelled。未決Offerは業者一覧から除外し、以後のaccept/declineはCONFLICT |
| accepted / assigned | 不可 | 可 | cancelled。関連Assignmentをrevoked |
| in_progress / submitted | 不可 | 不可（先にjobs.holdでon_hold） | CONFLICT |
| on_hold / rework_requested | 不可 | 可 | cancelled。未完了の記録を保持 |
| completed / cancelled | 不可 | 不可 | CONFLICT |

clientの「不可」はD01順位6の状態不適合としてCONFLICT、HQの「不可」も同じくCONFLICT。

## IR57 FORBIDDEN・NOT_FOUNDの表示と遷移 — REV18-013

routeガードでroleと異なるrole prefixのrouteを開いた場合だけ`/forbidden`へ置換遷移する。未認証はUNAUTHENTICATEDとして/loginへ移動する。Screenのprimary queryがFORBIDDENならURLを維持したままその場でpermission-denied状態（role homeへのリンク）、NOT_FOUNDならその場でnot-found状態（親一覧へのリンク）を表示し、自動で別画面へ移動しない。secondary queryは該当パネル内だけに同じ状態を表示する。writeがFORBIDDEN/NOT_FOUNDの場合は画面遷移せず、入力値を保持してフォーム上部にエラー要約・messageKey・相関IDを表示し、対象のprimary queryをinvalidateする。再取得の結果がFORBIDDEN/NOT_FOUNDなら前述のprimary queryの状態へ移る。ただし技術者のmessageKey=errors.assignment_not_startedはIR76のwork-not-started状態で表示する。DDC-03表と役割別DDにあった「使える画面へ戻す」「一覧画面へ移動」は本節で置換する。

## IR58 通知一覧の公開範囲 — REV18-014

notifications.listは、recipientMembershipIdが現在のMembershipで、かつtargetが現在scopeで閲覧可能な通知だけを返す。対象を閲覧できなくなった通知はitems・total・未読件数から除外し、マスクした行を返さない。通知自体は削除せず、再び閲覧可能になれば一覧に戻る。表示済みの一覧やURLから通知リンクを開いた時点でtargetを解決できない場合は、対象名・値を出さずに「利用できません」を表示する。FR-C08の「利用できません」表示はこのリンク解決時を指す。

## IR59 決済確定の単一経路とシステム主体 — REV18-015

顧客の決済試行のprocessing/confirm/failはpayments.simulateだけで起こす。DemoTriggerのpayment分岐は廃止し、/demo画面は決済イベントを発生させない。HQの入金確認はpayments.confirm/recordManualだけで行う。demo.trigger・demo.advanceClock・resetと、時計駆動の内部遷移（IR45/IR48/IR52/IR54、期限失効）が業務記録を変えた場合、監査はactorId=system-demo、actorRoleAtTime=system、reason=null、correlationId=起点のDemoEvent.eventIdまたは時計tick ID（`tick-<ISO時刻>`）とする。IR35の解除CommandはactorMembershipId=system-restriction、監査のactorRoleAtTime=systemとする。AuditView.actorRoleAtTimeの型はRole|'system'とし、UIは「システム（デモ）」と表示する。解除要求の起動時の監査の単位はIR90（REV19-033）。

## IR60 デモ専用操作の識別 — REV18-016

operation-catalog.frontend_executionは、mock-service（将来の本番adapterで同じ業務interfaceを実装する候補）、local-preference、demo-only（1A専用で本番adapterへ移植しない）の3値とする。demo-onlyはdemo.advanceClock、demo.reset、demo.trigger、demoSession.signIn、demoSession.signOut、demoSession.switchMembership、demoSession.extend、auth.previewPasswordReset、payments.simulate、offsets.simulate、automations.fireの11操作。demo-only操作を呼ぶUI領域には「DEMO」ラベルを常時表示する。D11の本番必須成果物に、これらの置換先（実認証・実パスワード再設定・決済事業者連携・機器イベント受信）を含める。

## IR61 非RTO契約の表示 — REV18-017

C10は契約が0件の顧客だけ「契約なし・一般保守」と監視画面への導線を表示する。general/energy/environment契約は種別・期間・請求を表示し、「契約なし」と表示しない。FR-C10の完了後の業務状態の記述を本節に合わせる。AT-C10-N④のcustomer-bはdemoSeed（IR69）で契約0件。

## IR62 空間未割当の設備 — REV18-018

units.saveのspaceIdはID|nullとし、nullは物件直下（空間未割当）を表す。propertyIdとの整合だけを検証する。spaces.archiveは直接所属のUnitがあれば従来どおりCONFLICTで、未割当へ自動移動しない。units.listのfiltersにunassignedOnly（boolean）を追加し、trueならspaceId=nullのUnitだけを返す（spaceIdとの併用はVALIDATION）。C02のツリーは物件直下に「空間未割当の設備」グループを表示し、その件数はunassignedOnly=trueのPage.totalを使う。

## IR63 管理ダッシュボードの省エネ概要 — REV18-019

AdminSummary.energySummaryはdashboard.read保持者に返す。対象はfilters（customerId/propertyId）に一致する非archived Unit、期間は[from,to)、料金・係数はD07/SR09の既定。0.19.0で、基準の自動選択と削減量の算出はIR78の`energyForecast`（按分した仮定基準による予想）へ移し、energySummaryの削減系はnullとした。表示文言と導線もIR78による。

## IR64 連絡可能時間の入力と公開 — REV18-020

contactWindowはtrim後0〜200 code point。'@'を含む場合、または空白・ハイフン・括弧・'+'を取り除いた文字列に連続7桁以上の数字を含む場合はVALIDATION（fieldErrors.contactWindow、messageKey=errors.contact_details_forbidden）。JobDetail.contactWindowは、client（自己の案件）、HQ job.manage、閲覧窓内の担当技術者（IR49）、受諾済みでaccess窓内の業者にだけ返し、それ以外の投影ではnullとする。JobOfferSummary・JobHistorySnapshot・通知paramsには含めない。自由文の完全な検出は保証しない（DDC-09）。入力案内と時刻表記の扱いはIR90（REV19-017）。

## IR65 音声の対象照合と構文 — REV18-021

文法は、trim後にASCII大文字小文字を無視して次の正規表現で判定する。en: `^temperature (.+)$`、`^set (.+) to (\d{1,3}) degrees$`、`^help$`。ms: `^suhu (.+)$`、`^tetapkan (.+) kepada (\d{1,3}) darjah$`、`^bantuan$`。`(.+)`は貪欲一致とする。`<room>`は現在scope内で非archivedのSpace.name（kindは問わない）とtrim・ASCII大小無視の完全一致で照合し、ACUnit.displayNameとは照合しない。scope外のSpace名は一致0件として扱う（D09の対象不存在）。一致したSpaceに直接所属する非archived Unitを候補とする。候補が1台ならtemperature/change、候補が2台以上または一致Spaceが複数ならcandidates（pathLabel=物件名 > 祖先Space名 > Space名 > Unit表示名）、候補0台ならunsupported。selectedUnitIdは直前の候補集合に含まれる場合だけ確定し、含まれなければNOT_FOUND。

## IR66 policyのないAlertの解消と再発の関連付け — REV18-022

D08の継続回復による自動resolvedはpolicyId≠nullのAlertだけに適用する。policyId=nullのAlert（seed、inferred/inspection根拠、tamper、maintenance、reconciliation_required、機器障害）は、alert.resolve保持者の手動resolve（理由・根拠ID必須）だけで解消する。新しいAlertを作るとき、同じunitIdで、policyId≠nullなら同じpolicyId、policyId=nullなら同じtypeとcauseCodeの組を同一事象キーとする。同一キーでopen/acknowledgedのAlertがあれば新しいAlertを作らず既存を維持し、重大度上昇はD08の通知だけを行う。同一キーの最新がresolvedならpreviousAlertIdにそのIDを設定する。AT-T07-NはpolicyIdを持つAlertで再測定回復を検証する。

## IR67 機器操作の開始と接続確認の状態 — REV18-023

devices.checkとdevices.updateFirmwareはstatus=queuedで作成する。作成時刻の1秒後のデモ時計tickで排他を再確認し、running・startedAtを設定する。checkはこのときDevice.connection=connectingとする。updateFirmwareはこのときconnection≠onlineならfailed・failureCode=OFFLINEとし、connectionを変えない。demo.trigger(operation)はrunningの操作だけを受理し、queuedへの結果はCONFLICT。checkのsucceededでconnection=online、failedでconnection=error。FWのsucceededでfirmwareVersion=targetVersion、failedは版とconnectionを変えない。作成から60秒でfailed・failureCode=TIMEOUT（D05）とし、checkのTIMEOUTはconnection=error。devices.calibrateは作成と同じ遷移でsucceededとし、queued/runningを経ない。再確認の失敗と制限Commandとの重なりはIR90（REV19-018）。

## IR68 負の削減量の表示 — REV18-024

DTOは符号付きの値を返す（savedKWh=baseline−actual、savingPercentage=savedKWh÷baseline×100、savedAmountMinor・savedEmissionsKgも同じ符号）。表示は全role共通のformatterで、値>0は「削減 {絶対値}」、値<0は「増加 {絶対値}」、値=0は「増減なし 0.0」、nullは「算定不可」。桁と丸めはIR44。例: 基準100kWh・実績120kWhのDTOはsavedKWh=-20、savingPercentage=-20で、表示は「増加 20.0 kWh」「増加 20.0%」。C06/C13/A13/A14で同じformatterを使い、A01の予想値はIR78の「削減予想／増加予想」表記とnull表示に従う。FR-A13/DD-A13の「-20kWh・-20%」はDTOの値を指す。

## IR69 デモseedとテスト用fixture上書き — REV18-026

初期業務データは[fixture-contract.json](../04-agentic-sdlc/fixture-contract.json)のdemoSeedを正とし、そこに無い業務記録をRepositoryが補わない。テスト専用のfactory `createDemoRepository({clock, seed:'demoSeed', patches, simulator})`を用意する。patchesは`{entity, id, set}`の配列（entityはdemoSeedのセクション名。例: customers、units、memberships。membershipsはactorsを対象にする）で、reset直後に一括適用し、正規DTO schemaで検証して不正なら例外（テスト失敗）とする。UIとcomposition-rootはpatchesを渡さない。simulatorはIR45のenabled初期値。受入条件のGivenはdemoSeedへの差分として記述し、記載の無い値はdemoSeedのままとする。省略形式の行から正規DTOへの展開はIR91、受入ごとの固定patchesはfixture-contract.jsonのacceptancePatches（IR85）。Membership失効・契約終了・Customer inactive等、テストでだけ作る業務状態はpatchesで作り、demo.triggerに新しい種類を追加しない。

## IR70 作業可能時間の休日 — REV18-027

members.capacityの作業可能区間は、Asia/Kuala_Lumpurの月曜〜金曜09:00–17:00とする。休日は同timezoneの土曜・日曜だけで、1Aは祝日カレンダーを持たない（例: 2026-09-16も作業日）。祝日への対応は本番要件の候補とする。

## IR71 変更イベントの種別とQuery無効化 — REV18-028

ChangeEvent.entityTypeは次の表の値とcursor_onlyだけを取る。events.subscribeのresourcesはcursor_onlyを除く値の配列で、空配列と未知値はVALIDATION。UIは受信したentityTypeに対応する操作のQueryだけをinvalidateし、表に無い操作はinvalidateしない。common.md §6の「jobs/units/invoices/restrictionsなど」は本表で置換する。

| entityType | invalidateする読取操作 |
|---|---|
| unit | units.list, units.get, summaries.get, admin.summary, telemetry.summary |
| device | devices.list, devices.get, devices.events, units.list, units.get, summaries.get, admin.summary |
| measurement | telemetry.series, telemetry.summary, units.list, units.get, summaries.get, admin.summary, energy.summary |
| command | commands.get, units.get, diagnosticRuns.get, restrictions.get, restrictions.forInvoice |
| diagnostic_run | diagnosticRuns.list, diagnosticRuns.get, units.get |
| device_operation | devices.operations, devices.calibrations, devices.get, units.get |
| alert | alerts.list, alerts.get, units.list, units.get, jobs.list, jobs.get, summaries.get, admin.summary |
| notification | notifications.list |
| job | jobs.list, jobs.get, jobs.events, summaries.get, admin.summary |
| report | reports.get, jobs.get |
| attachment | reports.get, attachments.getContent |
| offer | jobs.list, jobs.get, summaries.get |
| assignment | jobs.list, jobs.get, members.capacity, members.eligible, summaries.get |
| plan | plans.list, plans.get |
| contract | contracts.list |
| invoice | invoices.list, invoices.get, admin.summary |
| payment | invoices.list, invoices.get, admin.summary |
| restriction | restrictions.list, restrictions.get, restrictions.forInvoice, contracts.list, units.get |
| inquiry | inquiries.list |
| automation | automations.list, automations.nextRuns |
| policy | policies.list, policies.get |
| consent | consents.get, automations.list |
| membership | members.list, members.eligible, members.capacity, session.get |
| organization | organizations.list, admin.summary |
| customer | customers.list, admin.summary |
| property | properties.list, units.get, jobs.list, jobs.get |
| space | spaces.list, units.get |
| capability | capabilities.list, units.get, automations.list |
| baseline | baselines.list, energy.summary |
| factor | factors.list, energy.summary |
| mrv_report | mrv.list, mrv.get, mrv.versions |
| offset_record | offsets.list |
| session | 全Query（IR17のviewEpochを更新して破棄） |

## IR72 規範の優先順位 — REV18-029

同じ論点で記述が食い違う場合の優先順位は次の順で、上が優先する。実装Agentは下位の記述で上位を覆さない。見つけた食い違いは実装で選ばず文書欠陥として報告し、G1を停止する。

| 順位 | 規範 |
|---|---|
| 1 | ユーザー確定の決定（DEC-12/13/16/17/18/44/50） |
| 2 | 本書IR（同一論点では番号の大きいIRが優先） |
| 3 | strict-review-contracts.mdのSR |
| 4 | deterministic-contracts.mdのD01〜D16 |
| 5 | service-contracts.tsと各カタログCSV（IRと同時に更新し、不一致は文書欠陥） |
| 6 | implementation-contracts.mdのDDC |
| 7 | common.mdと役割別の詳細設計（DD） |
| 8 | 要件定義書のFR・BR・AT |
| 9 | UIUXSpecification.md |

IRが置換した旧記述は、同じ版で本文から削除するか、IRへの参照に書き換える。validate_documents.pyは置換済みの旧句を検出する。

## IR73 検証器の変異テスト — REV18-030

check_review_regressions.pyの変異対象は現行baselineと現行文言に一致させる。静的検証（validate_documents.py）と変異テスト（check_review_regressions.py）の両方の成功を文書引継ぎの条件とし、gate記録のevidence_pathsに両方の結果ファイルを含める。

## IR74 軽微な明確化 — REV18-025・REV18-031〜048

- REV18-025: AT-A06-Nの状態列はrequested→offered→accepted→assigned→in_progress→submitted→completed。技術者のjobs.startを経る（common.md §5）。
- REV18-031: DD-C01のsummary欄はpowerOn/powerOff/powerUnknown/online/offline/unknown/alertCount/asOf（SR27/IR51）。
- REV18-032: C08/T01のUI値severity=allはfilters.severityを省略して送る。
- REV18-033: DD-A03のvalidFromは常に必須、validUntilは外部技術者だけ必須。
- REV18-034: D10の対応環境にiPadOS 17 Safari（幅768/1024）を追加する。
- REV18-035: 受入条件の「YYYY-MM-DD HH:mm」でZもoffsetも無い表記はAsia/Kuala_Lumpurのローカル時刻、日付だけのfrom/toは同timezoneの暦日00:00を指す。
- REV18-036: commands.createのreasonはclientでは送らない（送ればVALIDATION）。technician/adminは必須（IR09）。
- REV18-037: モック読取の正常待機はfixture.defaultWaitMs=300ms固定。単体・部品テストは時計注入で0ms。
- REV18-038: A01/A13の期間はtoday/7d/30d/customのプリセット（SR17）。P01/T01のfrom/toはYYYY-MM-DDの表示timezone暦日で、[from日00:00, to日の翌日00:00)のUTC Instantへ変換し、最大366日。
- REV18-039: AT-T03-Eの順序と重複はRawMeasurement.sequenceで判定する（Measurement.versionはRepository採番）。
- REV18-040: DemoTrigger(device)のevidenceSourceは種類から導出する。communication_lost→heartbeat、power_lost→power_signal、tamper→tamper_signal、restoredは参照障害と同じ値。DD-T12のevidenceSourceは読取専用。
- REV18-041: Space.kindの入れ子に順序制約は無い（同一物件・非循環だけを検証）。
- REV18-042: units.save等はtenantIdを入力しない（D14のSession由来）。
- REV18-043: FR-A04の権限はdevice.manage、FR-A12の権限はautomation.policy.manage。
- REV18-044: NFR-03の「期限切れのときの再確認」は、確認ダイアログの表示中にSession期限・権限変更・対象versionの変更が起きたら、confirmで送信せず再取得して確認をやり直すことを指す。
- REV18-045: 1Aは企業原文Phase 1のクリック可能なフロントエンドデモ、1Bは同Phase 1の実機・ファームウェア・本番API接続、Phase 2はHVAC。
- REV18-046: 画面カタログのconnecting/device-error/device-online/device-offlineは、operationsに設備・機器・測定・制御・制限・集計の読取を含むScreenだけに列挙する。
- REV18-047: ms辞書の文言は実装Agentが下書きし、`i18n/ms`の各キーに未確認フラグを付けて企業検収前にBusiness/UI/UXが確認する。キー集合の一致だけを自動検査する（IR44）。
- REV18-048: アプリ名は翻訳キーapp.name（en: "AC Monitoring Demo"、ms: "Demo Pemantauan AC"）とし、参考サイトの名称・ロゴを使わない。

## IR75 現行baselineと0.18.0記録の扱い — REV19-001

0.18.0の修正（REV18）は、自己再レビューとbaseline作成の前に作業が中断した。0.18.0のspec-manifestは作成しない。runs/DOC-0.18.0にはREV18の指摘一覧（review.md、findings.json）と、gate-G1.yaml（gate_result=not_evaluated、spec_baseline_id=null、interrupted=true）だけを保存し、実装入力として使用しない。現行baselineはDOC-0.19.0で、runs/DOC-0.19.0にspec-manifest.json、review.md、findings.json、traceability-matrix.csv、static-check.json、validator-negative-checks.json、gate-G1.yaml、completion.jsonを保存する。受入計画CSVの値にカンマや引用符を含む場合はCSV規則どおり引用し、列数不一致は静的検証エラーとする。README・SDLC §8・IR14の現行版表記はDOC-0.19.0に揃える。

## IR76 作業窓開始前の技術者画面 — REV19-002

IR49の作業窓開始前（閲覧窓内かつnow < 自己のactive AssignmentのscheduledStart）の技術者画面は、画面カタログの状態`work-not-started`で表示する。IR57のpermission-deniedより本節が優先する。

- SCR-T04（/technician/jobs/:id）とSCR-T10（/technician/units/:id/control）: jobs.getの結果からscheduledStart>nowを判定した場合、units.get・reports.get・attachments.getContent・commands.get・diagnosticRuns.get/listのQueryを無効（呼ばない）にし、primary queryはjobs.getだけとする。案件の種類・設備ID・予定枠・状態を読取表示し、「{scheduledStart}から作業できます」と、開始・保存・提出・制御のボタンをdisabledで表示する。
- SCR-T02（/technician/units/:id?jobId=）とSCR-T07（/technician/units/:id/alerts?jobId=）: primary queryがFORBIDDEN（messageKey=errors.assignment_not_started）を返した場合、permission-deniedではなく`work-not-started`を表示する。開始時刻はDomainErrorに含めず、URLのjobIdでjobs.getを取得して表示し、`/technician/jobs/:jobId`へのリンクを出す。jobIdが無い、またはjobs.getが失敗した場合は開始時刻とリンクを出さず「担当案件の作業開始時刻から利用できます」だけを表示する。その他のFORBIDDENはIR57どおり。
- SCR-T11（/technician/devices?jobId=）: jobIdを伴うdevices.*のwriteボタンをdisabledにして開始時刻を表示する。読取はIR49(b)に従う。
- 表示中の画面はデモ時計の1秒tickでnow>=scheduledStartを判定し、到達した時点で無効にしていたQueryを有効化して取得する（通常のloadingへ移る）。viewEpochは変えない。
- D10の画面優先順位では`work-not-started`をforbidden/not-foundと同じ段に置く。AT-REV18-013①の「permission-denied表示（開始時刻の案内）」は`work-not-started`を指す。

## IR77 生存シミュレーターの複写条件とデモ測定のsequence — REV19-003

IR45の手順2は、Sensorごとに同じsensorIdの最新Measurement（observedAt降順、sequence降順、id昇順の先頭）がorigin=measured、quality=valid、value≠nullの場合だけ、そのvalue/unitを複写する。最新がestimated/inspection、suspect/missing、value=nullのSensorは生成しない。古い実測値へ遡って複写しない（IR08）。そのSensorはD07/SR27どおりstale・unknownへ移り、demo.trigger telemetryでmeasured/validの観測が投入されると次の分境界から再び複写対象になる。手順1（lastSeenAt）と手順3（observedState.observedAt）は、複写したSensorの有無にかかわらず、IR45のonline・powerSignal≠offの条件で行う。

RawMeasurement.sequenceは省略可能とする。省略時、Repositoryは同じsensorIdの既存Measurementの最大sequence+1を採番する（既存が無ければ1）。明示した場合は、最大sequence以下なら値を反映しない（重複・逆行、IR74のREV18-039）。/demo画面のtelemetryフォームはsequence欄を既定で空（自動採番）とし、origin=measured、quality=validを既定値として表示する。受入試験でsequenceの順序や重複を検証するcaseだけ明示する。

## IR78 管理ダッシュボードの省エネ予想 — REV19-004

IR63の「期間の分数が同じ基準を自動選択して削減量を返す」を本節で置換する。AdminSummaryに`energyForecast: EnergyForecast`を追加し、dashboard.read保持者に常に返す。AdminSummary.energySummaryは期間の実績（kWh・料金・coverage・品質）だけを返し、baselineRef/baselineSnapshot=null、削減系（savedKWh/savingPercentage/savedAmountMinor/savedEmissionsKg）はnullとする。基準との比較はA13（energy.summary）で行う。

EnergyForecastの算出は次のとおり。演算は十進有理数、表示の丸めはIR44による。

1. 対象設備集合U: filters（customerId/propertyId）に一致し現在scope内の非archived ACUnit。|U|=0ならbaselineRef/baselineSnapshot=null、数値はすべてnull、expectedUnitMinutes=0、validUnitMinutes=0、qualityWarnings=['no_units']。
2. 基準: method=demo_fixed、boundaryId=ac_input_electricity、baselineKWh≠null、unitIdsの集合がUと完全一致するEnergyBaselineのうち、createdAtが最新（同時刻はid昇順の先頭）のものの最新version。無ければbaselineRef/baselineSnapshot=null、predictedBaselineKWh/forecastSavedKWh/forecastSavingPercentage=null、qualityWarningsにbaseline_unavailableを含める。demo_period_comparison（実測基準）は自動選択しない。
3. periodMinutes=[from,to)の分数、baselineMinutes=基準periodの分数、expectedUnitMinutes=|U|×periodMinutes。
4. validUnitMinutes=D07/IR08/IR11の有効slot（origin=measured、valid、境界一致）の数。actualKWhOnValidSlots=その電力量の合計（validUnitMinutes=0ならnullとし、qualityWarningsにactual_unavailableを含める）。
5. predictedBaselineKWh = baselineKWh ÷ (|U|×baselineMinutes) × expectedUnitMinutes。
6. predictedActualKWh = actualKWhOnValidSlots ÷ validUnitMinutes × expectedUnitMinutes（validUnitMinutes≥1のとき、それ以外null）。
7. forecastSavedKWh = predictedBaselineKWh − predictedActualKWh（どちらかnullならnull）。forecastSavingPercentage = forecastSavedKWh ÷ predictedBaselineKWh × 100（predictedBaselineKWhが0またはnullならnull）。
8. 基準を選んだ場合はqualityWarningsにmodeled_baselineとprorated_forecastを含め、validUnitMinutes<expectedUnitMinutesならpartial_coverageも含める。qualityWarningsはASCII昇順で重複なし。

A01の省エネカードは、実績（kWh・料金・coverage）と予想を並べて表示する。予想の値>0は「削減予想 {絶対値}」、値<0は「増加予想 {絶対値}」、値=0は「増減なし 0.0」とし、常に「予想（按分した仮定基準・デモ）」のラベルとvalidUnitMinutes/expectedUnitMinutesを併記する。nullの表示は、qualityWarningsにno_unitsがあれば「対象設備なし」、baseline_unavailableがあれば「基準未設定」、それ以外は「算定不可」とする（IR68のnull表示より本節が優先）。energy.manage保持者にだけ/admin/energyへの導線を表示する。SR17の当日最初の1分（from=to）はadmin.summaryを呼ばない。

demoSeed.baselinesに`baseline-demo-tenant-a`（unitIds=tenant-aの非archived全5台、method=demo_fixed、boundaryId=ac_input_electricity、period=[2026-08-01T00:00:00.000Z,2026-08-31T00:00:00.000Z)、baselineKWh=2592、quality=modeled、createdAt=2026-09-01T00:00:00.000Z、version=1）を置く。filtersなしの/adminで予想が表示され、customerId/propertyIdで設備集合が変わると「基準未設定」になる。

## IR79 セッション延長の記述統一 — REV19-005

IR36にあった延長を否定する一文を「利用者操作による延長はIR55のdemoSession.extendだけで行う」へ置換した。延長の規範はIR55、時計ジャンプ時のexpiresAtのずらしはIR36とし、他文書の記述は両節への参照にする。validate_documents.pyは本書を含めて延長を否定する表現を検出する（IR81）。

## IR80 負の削減量の表示（C06） — REV19-006

IR68の共通formatterをFR-C06のBR・AT-C06-E・DD-C06の文字列にも適用する。基準100kWh・実績120kWhはDTOがsavedKWh=-20、savingPercentage=-20で、表示は「増加 20.0 kWh」「増加 20.0%」。基準0は削減率「算定不可」。旧表記の「20%増加」「増加20%」「増加率20%」は使わない。

## IR81 旧記述検出の範囲 — REV19-007

validate_documents.pyの旧記述検査は、01-requirements・02-design・03-uiuxの全Markdownと04-agentic-sdlc/verification.mdを対象とし、本書（review-resolution-contracts.md）も含める。本書では、置換対象を引用して説明する行（「置換」「旧記述」「旧表現」「旧文」のいずれかを含む行）だけを検査から除外する。検査語は正規表現で表記揺れを含めて登録する。対象は、延長を否定する表現、「型番を管理（する|できる）権限」、「環境（に関する）policy（(方針)）を管理する権限」、「roomは表示名」、「顧客組織の数」、負値表示の旧表記、S03の入金確認後に解除要求を必須手順とする表現、および0.18.0以前に登録済みの旧句。check_review_regressions.pyは各検査語を1件ずつ文書へ戻す変異を持ち、全変異が検出されることを成功条件とする。

## IR82 ログイン後の復帰先の受け渡し — REV19-008

routeガードは、未認証で保護routeを開いたとき`/login?returnTo=<encodeURIComponent(pathname+search)>`へ置換遷移する（hashは含めない）。SCR-X-loginのurl_selectionはreturnToを持つ。ログイン画面はreturnToを1回だけdecodeし、D08の条件（同一origin相対path、screen-catalogの許可route、`//`・scheme・制御文字・バックスラッシュ・二重encodingの禁止）を満たす場合だけdemoSession.signIn.returnToへ渡す。条件を満たさない値はVALIDATION画面にせず、URLから除去して無視する。signIn成功後、returnToが選択したroleで許可されたrouteならそこへ、それ以外はrole homeへ置換遷移する。/login・/forgot-password・/demoはreturnToにしない。

## IR83 再取得中・再取得失敗の表示と無効化の集約 — REV19-009

同じquery keyに成功済みデータがある状態で、購読イベント・書込み成功・明示更新により再取得している間は、表示データを保持し、対象領域にaria-busy=trueと非モーダルの「更新中」表示を出す。skeleton（loading）へ戻さず、live regionで読み上げない。再取得が失敗した場合もデータを保持し、DDC-03どおりstale注記・最終成功時刻・再試行を表示する（読取の自動再試行はD04）。query keyが変わった場合（URL条件・sort・対象ID・role・viewEpochの変更）はinitial/loadingから表示し、前のkeyのデータを新条件の結果として表示しない（IR34）。再取得の結果がFORBIDDEN/NOT_FOUND/UNAUTHENTICATEDなら保持データを破棄してIR57/D09に従う。

購読イベントによる無効化は、同じデモ時計1秒tick内に届いたイベントをquery keyごとに1回へ集約し、そのtickの確定後に実行する。SR14の複数ページ取得中の保留は維持する。

## IR84 位置情報同意の初期記録 — REV19-010

demoSeed.consentsに、client Membership（customer-a、customer-b）ごとにpurpose=location_automation、granted=false、grantedAt=null、revokedAt=null、version=1のConsentを置く。members.saveでrole=clientのMembershipを新規作成した同一遷移でも、同じ初期Consentを1件作成し監査する。consents.getは自己Membershipの記録を返し、記録が無い場合はNOT_FOUND（fixture欠陥として扱う）。SR02の「初回からgranted=falseの版付きConsent」はこの記録を指し、IR69の「seedに無い業務記録を補わない」と矛盾しない。

## IR85 KPI受入Givenのseed差分 — REV19-011

AT-A01-N/AT-A01-B①のGivenは、fixture-contract.jsonの`acceptancePatches["AT-A01-N"]`を正とする。内容はsimulator=false、clock=2026-09-14T01:00:00.000Z、device-offline-rtoをconnection=online・lastSeenAt=2026-09-14T00:59:30.000Z・powerSignal=on、unit-offline-rtoのobservedStateを{power:false,celsius:25,mode:'cool',fanLevel:'mid',observedAt:'2026-09-14T00:59:30.000Z'}、sensor-offline-powerの測定（value=0.0、unit=kW、origin=measured、quality=valid、observedAt=receivedAt=00:59:30Z、sequence=2）の追加。結果はtenant-aでON2（unit-online-rto・unit-limited）、OFF2（unit-non-rto・unit-offline-rto）、不明1（unit-other-customer）、稼働率50.0%。KPIや件数を観測するATでは、Givenに差分として書かれていない値はdemoSeedのままとし、観測の最初にsimulator=falseを与える（IR45）。

## IR86 期限切れの未応答Offerへの応答 — REV19-012

Offer.decision=nullかつnow>=offerExpiresAtのOfferについて、当該業者（Offer.contractorOrgIdが自社で、partner.acceptを持つ有効Membership）のjobs.accept/declineは、scope内の状態不適合としてD01順位6のCONFLICT（messageKey=errors.offer_expired、業務変更0件）を返す。他社のOffer IDや存在しないIDはNOT_FOUND（順位3）。D01順位4の「担当期限外」はaccessValidFrom/Until、Assignmentの閲覧窓/作業窓、資格の有効期間に限り、Offerの応答期限には使わない。期限後、当該業者のjobs.list/jobs.eventsは当該案件を返さず、jobs.getはNOT_FOUND（IR23）。同キー再送とwrites.getResultはIR01に従う。AT-P02-E①（now=offerExpiresAt）とAT-REV18-004の「期限後のacceptはCONFLICT」は本節による。

## IR87 理由系入力の文字数 — REV19-013

reason、cancelReason、declineReason、resolutionReason、reviewComment、changeReason、purposeは、DDの表記にかかわらずtrim後1〜1000 Unicode code point（D12）。本文・メモ系（symptom、workText、message、reply、note、responseNote、assumptions、boundary）は各DD/IRの文字数を使う。DD-T07のresolutionReason、DD-A14のreviewComment、DD-A10のreason、DD-P05のreasonは1〜1000へ修正した。D12の「各DDに規定のない名前は1〜120文字」は名称欄だけに掛かる。

## IR88 MRV画面の表示項目と正規型の対応 — REV19-014

DD-A14の表示項目は正規型の次の値から表示し、DTOにフィールドを追加しない（D12）。

| 表示項目 | 取得元 |
|---|---|
| reportCategory | MRVPreview.scope（'scope_2'）を「Scope 2（電力）」と表示 |
| organizationId | conditions.organizationId |
| period | conditions.from / conditions.to |
| siteIds（対象拠点） | conditions.unitIdsの各ACUnit.propertyIdを重複除去しASCII昇順（units.list(filters.organizationId)の結果から導出） |
| gridRegion | summary.factorSnapshot.region |
| factorValue | summary.factorSnapshot.kgCO2ePerKWh |
| factorUnit | 固定表示「kgCO₂e/kWh」 |
| factorYear | summary.factorSnapshot.year |
| factorVersion | summary.factorRef.version |
| boundaryDescription | conditions.boundary（IDはconditions.boundaryId） |
| coverageRatio | summary.coverage（nullは「未算定」） |

summary.factorSnapshotがnullの場合、係数関連の欄は「算定未完了」を表示する。

## IR89 作業窓の終了予告と終了時の扱い — REV19-015・REV19-037

- 予告: 技術者が当該案件のjobIdを持つ画面（SCR-T02/T04/T07/T10/T11）を表示している間に、デモ時計が自己のactive AssignmentのscheduledEnd−15分に到達したら、role=statusのバナー「作業窓は{scheduledEnd}に終了します。未保存の入力を保存してください」を1回表示する。閉じた後は同じAssignmentについて再表示しない。時計ジャンプで予告時刻だけを通過した場合も到達を検知した時点で1回表示する。ジャンプでscheduledEndも通過した場合は予告を出さず、次項の終了の扱いだけを行う。
- 終了: now>=scheduledEndになると、IR24どおりviewEpochを増分し、Query・snapshot・未保存のフォーム値・object URLを破棄してhistory表示へ移り、「作業窓の終了により未保存の入力を破棄しました」と通知する。FR-T09の「再取得しても未保存の編集内容を消さない」は作業窓内の再取得だけに適用する。
- HQ/業者の表示: JobSummary/JobDetailでstatus∈{assigned,in_progress}かつscheduledSlot.endAt<=nowの案件に「作業窓終了・再割当が必要」を表示する。既存DTOからの導出であり新フィールドは追加しない。表示中の一覧は1秒tickで再評価し、再取得しない。
- 作業窓の延長はIR49のjobs.assignで行い、新Assignmentができるまで技術者の書込みはFORBIDDEN（D06）。
- jobs.assignが成功した同一遷移（初回・再割当・延長のすべて）で、Job.assignmentId=新AssignmentのID、Job.scheduledSlot=[新AssignmentのscheduledStart, scheduledEnd)に更新し、Job.versionを1増分する。旧Assignmentはstatus=revokedで保持する（REV19-037）。

## IR90 軽微な明確化 — REV19-016〜035

- REV19-016: DD-A01の顧客数はIR40の定義を参照する。DD-A04の前提はdevice.manage、DD-A12の前提はautomation.policy.manage（IR74のREV18-043）。D09の`<room>`はIR65のSpace.name照合。verification.md S03は入金確認の遷移で解除要求が起動し（IR35）、明示releaseは冪等応答の確認手順とする。common.md §2の/forbiddenと未定義routeは、role homeへのリンクを表示する画面であり自動遷移しない（IR57）。
- REV19-017: contactWindowの判定規則（IR64）は変えない。入力欄に「時刻はHH:mm形式（例: Weekdays 09:00-18:00）」の案内を常時表示し、errors.contact_details_forbiddenの文言にも同じ例を含める。「0900-1800」は連続8桁としてVALIDATION、「09:00-18:00」は受け付ける。
- REV19-018: IR67の1秒後の排他再確認で、同じ設備にrequested/sentの通常Command（UnitAction）、進行中のDiagnosticRun、または別のqueued/running DeviceOperationがあれば、当該operationをfailed・failureCode=CONFLICT・finishedAt=nowとし、connectionと版を変えない。D05により通常の操作経路ではこの競合は生じないため、この規則は不変条件の防御として働き、受入ではIR69のpatchesで競合状態を作って確認する。delivery=not_sentの制限Commandは再確認の対象に含めない。check/firmwareがqueued/runningの設備に対する制限のapply/remove Commandはdelivery=not_sent・pendingReason=device_operation_runningの未配送意図としてD03に従い、operation終了後にrestrictions.retryで送る。
- REV19-019: invoices.createのdueAtはnowより後だけを受け付ける。期限超過の請求はdemoSeedまたはdemo.advanceClock（IR36）で作る。
- REV19-020: DD-T01のstatus候補はassigned/in_progress/on_hold/submitted/rework_requested/completed/all、DD-P01はoffered/accepted/assigned/in_progress/on_hold/submitted/rework_requested/completed/all。allはstatus省略。
- REV19-021: IR71のdevice_operation行にdevices.calibrationsを追加する。IR71は購読イベントを起点とする無効化の規則であり、書込み成功後の無効化はD10と各DDの「更新の対象になるQuery」も適用する。
- REV19-022: AuditView.result=pendingは、非同期の結果待ち記録を作る操作（commands.create、diagnosticRuns.create、devices.check、devices.updateFirmware、restrictions.execute、restrictions.retry、restrictions.override、payments.simulate(event=initiate)）の受付監査に使う。結果が確定した遷移で、同じcorrelationIdのsuccessまたはfailedの監査を追加し、pending行は書き換えない。FR-A16とDD-A16の結果は成功・拒否・失敗・保留の4分類。
- REV19-023: severityの並べ替えと比較は、全操作でnormal < warning < criticalの順位を使う。文字列比較をしない（alerts.list/notifications.listの`severity desc`はcritical→warning→normal）。
- REV19-024: VoiceContainer（feature hook）がIR09のvoice.resolveIntent・units.get・jobs.list・jobs.get・commands.create・commands.getを実行し、VoicePanelはpropsとeventだけを持つ表示Componentとする（D10/D13）。
- REV19-025: 件数KPIカードはKpiCard（label、value:number|null、denominator:number|null、unknownCount:number|null、asOf、href:string|null、loading、error）を使う。valueのnullは「算定不可」、0は「0」と表示する。
- REV19-026: NotificationPanelの空表示は、unreadOnly=trueなら「未読の通知はありません」、falseなら「通知はありません」。
- REV19-027: データを取得しない公開画面の状態は、SCR-X-login/SCR-X-forgot-passwordがinitial・loading・success・error・offline、SCR-X-demoがinitial・loading・success・error、SCR-X-forbidden/SCR-X-not-foundがsuccessだけとする。認証後の画面の必須状態は従来どおり。
- REV19-028: UIUX仕様書の「テレメトリー」はセンサー測定値（Measurement）を指し、利用状況の記録ではない。
- REV19-029: SCR-A13のurl_selectionにunitIdsとbaselineId、SCR-A11/SCR-A12にpolicyIdを追加し、Back/Forwardで選択を復元する（D13）。
- REV19-030: EnergyBaseline、OffsetQuote、OffsetRecordをclientに返すのは、unitIdsの全件が現在scope内の場合だけ。一部だけなら一覧から除外し、個別取得はNOT_FOUND（SR03のinvoice/contractと同じ）。
- REV19-031: DD-A06のunitIdとtypeは必須、dueAtは任意（HQだけが入力、IR38）。DD-A11のenabled・priorityは共通入力行だけで定義する。
- REV19-032: DD-P07のrecipientRoleはnotifications.recipientsのroleへ、hq→admin、assigned_technician→technician（当該案件のactive Assignmentの技術者）、customer_contact→client（案件設備の顧客Membership）として渡す。
- REV19-033: IR35の入金確認などで解除要求が起動した場合、Restrictionの状態遷移の監査は起動した利用者（actorId・当時のrole）で記録し、remove_restriction Commandの作成監査はactorId=system-restriction・actorRoleAtTime=systemで記録する。両者は同じcorrelationIdを持つ。IR59の記述はCommand作成監査を指す。
- REV19-034: 省エネ予想の算出方法（IR78、案A）と作業窓終了時の扱い（IR89）は、2026-09-17のユーザー回答でDEC-44・DEC-50として確定した。企業の商用承認とは区別する。
- REV19-035: 1B接続では、HTTP状態と通信例外をDomainErrorへ写す表（400/401/403/404/409/429/5xx/timeout/offline）をD11の本番必須成果物に含める。1Aでは写像を定めない。

## IR91 demoSeedの正規化規則 — REV19-036

demoSeedとacceptancePatchesの行は省略形式で記述する。Repositoryは生成時に次の規則だけで正規DTOへ展開し、service-contracts.tsのschemaで検証する。規則で埋まらない必須値や型不一致はfixture欠陥として例外にする（IR69）。これは業務記録の追加ではなく、記述済みの行の共通項目の補完である。

1. tenantId: 行に値があればそれを使う。無ければ親から導出する（Property←customerOrgIdのOrganization、Space←Property、ACUnit←customerOrgIdのOrganization、Device/Measurement/Command/Alert/MaintenanceJob←unitIdのACUnit、Offer/Assignment←jobIdのMaintenanceJob、Contract←customerIdのCustomer、Invoice/Restriction←contractIdのContract、Notification←recipientMembershipIdのactor）。
2. version: 行の値、無ければ1。createdAt: 行の値、無ければfixture.seedCreatedAt（2026-09-01T00:00:00.000Z）。updatedAt: 行の値、無ければcreatedAt。
3. 行に無いnull可能フィールドはnull、配列フィールドは[]。Space.archived=false、ACUnit.type='split'。
4. 派生値は保存せず読取時に計算する: ACUnit.connection/lastSeenAt（IR47）、Contract.activeRestrictionIds/hasUnresolvedRecovery（SR19/SR26）、Invoice.paymentMethod/paymentStatus（DDC-08 §3）。
5. Device: targetUnitId=unitId、createdByMembershipId='system-demo'。sensorsはseedに明示したSensor（id、metric、unit、staleAfterSeconds、boundaryId、calibratedAt=null）を使い、IR43の能力定義と一致しなければfixture欠陥。
6. Measurement: eventId=id、isDemo=true、qualityReason=null、rawUnit=null。
7. Command: correlationId=`seed-`＋id、diagnosticRunId/jobId/reason/failureCode=null。
8. Alert: evidenceIds=[]、deliveryFailures=[]、acknowledgedAt/resolvedAt/resolutionReason=null。
9. Notification: paramsは実行時と同じテンプレート生成関数で、正規化済みseedの対象資源から生成する（D12）。
10. MaintenanceJob: planId/occurrenceAt/startedAt/completedAt/draftReportRef=null、reportRefs/costs=[]。
11. Restriction: events=[]。perUnit[].observedRestrictionは同じUnitのobservedRestriction、perUnit[].evidenceIdはnull。
12. acceptancePatchesの`{entity,id,set}`で、idが存在すればsetの項目を上書きし、存在しなければsetを新しい行として本節の規則で正規化して追加する。
13. Policy: tenantIdはownerMembershipIdのactor、createdByUserIdは行に無ければ同actorのuserId、disabledReason=null。EmissionFactor: isDemo=true。

## IR92 受入Givenの解釈規則とseed差分 — REV19-038・REV19-040〜042

受入条件（AT-*-N/E/B/SRC/R01、S01〜S08）のGivenは次の規則だけで前提データに変換する。規則で決まらない前提は文書欠陥として報告し、テストAgentが推測で補わない。

1. Givenに書かれたID（unit-online-rto、job-contractor-a等）はIR91で正規化したdemoSeedの行を指し、Givenに書かれた値だけをその行へ上書きする。書かれていない値はdemoSeedのまま。
2. 対象が書かれていない場合の既定: 顧客の設備操作・監視・自動運転・方針の対象はunit-online-rto、換気非対応の設備はunit-non-rto、制限中の設備はunit-limited、オフライン設備はunit-offline-rto、顧客はcustomer-a、業者はcontractor-a、外注の技術者はtech-external-aとjob-contractor-a、社内の技術者はtech-internal-a、HQはhq-operator（制限・解除の操作はhq-restriction-manager）、請求はinvoice-overdue-a、制限はrestriction-limited-a。
3. 「in_progress案件」「submitted報告」「受諾済み案件」など業務状態を表すGivenは、acceptancePatchesが無い限り、seedから通常の操作（jobs.start、jobs.saveDraft、jobs.submit、jobs.offer、jobs.accept等）を書かれた順に実行して作る。
4. ①②…で状態を列挙するGivenは、規則2の対象の該当フィールドだけを各subcaseで独立にpatchする。ただしIR97の3で通常の操作が必要な状態（Restriction.state、報告版を伴うJob.status、Payment/Invoiceの状態）はpatchせず、受入本文に書いた操作で作る。
5. fixture-contract.jsonの`acceptancePatches`は、キーがcase ID（subcaseは`.番号`）または`shared:`名で、値は`{clock, simulator, include?, patches, query?, expected?, input?, evaluation?, trigger?, advanceSeconds?, flow?}`（`shared:`は`{description, patches, input?, bindAtUse?}`）。inputは保存操作の完全な入力、evaluationはEvaluationInput、triggerはDemoTrigger、flowは状態を作る通常操作の順序、bindAtUseは受入本文で決める値（IR97）。includeに挙げた共有patchを先に展開してから自身のpatchesを適用する。patchは`{entity,id,set}`（IR91の12）か、測定系列の`{entity:'measurements', series:{idPrefix, unitId, sensorId, metric, unit, boundaryId, from, to, stepSeconds, value, origin, quality, sequenceStart, skip}}`。seriesは[from,to)のfrom+k×stepSecondsの各時刻（skipの[from,to)に入る時刻を除く）に、id=`idPrefix-k`、observedAt=receivedAt=その時刻、sequence=sequenceStart+kの行を作る。entityはdemoSeedの節名（membershipsはactors）。
6. acceptancePatchesを持つcase: AT-A01-N、AT-C06-N、AT-C06-E.2、AT-C06-E.3、AT-C06-E.4、AT-C08-N、AT-P01-N、AT-P03-R01、AT-P06-N、AT-P06-B、AT-T07-N、AT-T10-E.1、AT-T11-N、AT-A09-R01、AT-A13-N、AT-A14-N、AT-C13-N、AT-C10-E.1、AT-C08-SRC、AT-T07-SRC、AT-A05-SRC、AT-C07-SRC.4、AT-A12-SRC.4、AT-T12-N、AT-A05-N、AT-A11-N、AT-A12-N、AT-X02-B、AT-X06-B.1、AT-X06-B.2、AT-X06-B.3、AT-X06-B.5、AT-REV17-005、AT-X04-E.5。共有patch: shared:energy-actual-80、shared:tech-internal-a-job-online、shared:load-cause-alerts、shared:report-draft-all-normal。受入本文はキーを明記する。
7. 電力量・排出量の受入（C06/C13/A13/A14、S05）は、fixture.energyの窓[2026-09-14T00:00Z, 01:00Z)とunitIds=[unit-online-rto]を使う。demoSeed.factorsのfactor-demo-2026（0.5 kgCO₂e/kWh）がfixture.defaultEmissionFactorIdの実体である。
8. 同じ設備を対象に含む契約の期間重複は1Aでは拒否しない（現行規則の明文化。制限は設備ごとに進行中1件、DDC-08 §3）。

validate_documents.pyは、受入本文が参照するacceptancePatchesのキーの存在、includeの解決、entityがdemoSeedの節であること、seriesの形式、既存行の上書き対象の存在を検査する。

## IR93 技術者の作業開始・提出の失敗コード — REV19-039

技術者のjobs.start/jobs.submit/jobs.saveDraftの失敗コードはD01の順位で次のとおり決める。

| 案件と担当の状態 | 結果 |
|---|---|
| 当該技術者にAssignmentが一度も無い（例: requestedで未割当のjob-internal-a） | NOT_FOUND（順位3） |
| 作業窓開始後にAssignmentが取消・再割当・期限でrevoked/終了し、historyだけが見える | FORBIDDEN（順位4、messageKey=errors.assignment_ended） |
| 作業窓開始前のAssignmentが取消でrevoked（IR24によりhistoryなし） | NOT_FOUND |
| active Assignmentで作業窓内だが状態が不適合（on_hold、submitted、completed等） | CONFLICT（順位6） |
| active Assignmentで作業窓開始前 | FORBIDDEN（errors.assignment_not_started、IR49/IR76） |

外注案件（contractorOrgId≠null）の報告の受理・差戻しは受託業者のpartner.review保持者が行い、HQはjobs.reviewのreviewMode=hq_escalationと理由がある場合だけ行う（D06）。

## IR94 認可列の修飾語と技術者の書込み条件 — G1-001・G1-026・G1-030

操作カタログのauthorization列の修飾語は次の意味に限る（DEC-54）。複数Unitを持つ資源にはSR03の全対象Unit条件を常に重ねる。

| 修飾語 | 意味 |
|---|---|
| public:demo-only / public:demo-panel-only | Session不要のデモ操作（IR60）。demo-panel-onlyは/demo画面からだけ呼ぶ |
| authenticated:own-session / demo-account-switch | 有効Sessionの本人（D09）／デモアカウント切替 |
| authenticated:recipient-only | Notification.recipientMembershipIdが現在Membership（IR58） |
| authenticated:current-target-party / recipient-or-current-target-party | D08/D15の通知対象資源を現在scopeで閲覧できる当事者 |
| authenticated:original-user-and-current-target-scope | D04のwrites.getResult条件 |
| client:self / client:self-customer / client:control.execute[:self] | 自己customer組織のMembership.scopes内の資源（control.executeは権限も必須） |
| client:own-membership | 自己Membershipの記録（Consent） |
| client:accepted-report-only | 受理済みの報告版だけ |
| client:self:customer-visibility / demo-event-only / event=request-or-retry / requested-only | JobNote visibility=customerだけ／IR59の顧客決済イベントだけ／SR22／IR56のrequestedだけ |
| contractor:accepted-valid-offer / delegated | 自社Offerがacceptで、now∈[accessValidFrom, accessValidUntil)（IR23のsummary投影期間） |
| contractor:offer-projection-or-delegated-history | IR23のoffer/summary/history投影 |
| contractor:partner.accept:own-valid-offer:first-attempt | IR01/IR86 |
| contractor:partner.assign:own-valid-offer / own-company | 自社の受諾済みOfferのaccess窓内／自社organizationのrole=technicianのMembershipだけ |
| contractor:partner.review:own-offer / submitted | 自社受託案件の提出版 |
| technician:assigned（読取） | 社内はMembership.scopes内のUnit（IR49(a)の案件起点読取を除く）、外部は自己Assignmentの閲覧窓∩scopes |
| technician:assigned-history | IR23/IR24の投影 |
| technician:*:assigned / assigned-valid-job / job-required（書込み） | 下表 |
| admin:<permission> | 当該permissionを持ち管理scope内 |
| admin:<permission>:scope-candidate-read-only | D12の補助読取 |
| admin:<permission>:kind=… | Policy.kindごとの権限 |
| admin:job.manage:internal-job / internal-or-escalation | contractorOrgId=nullの案件／外注ではjobs.reviewのreviewMode=hq_escalation（D06） |
| admin:restriction.override:release-projection / release-intent-or-terminal-recovery-only | IR03 |
| IR01:same-key-receipt… | IR01 |

技術者の書込み（alerts.acknowledge/resolve、devices.*、commands.create、diagnosticRuns.create、jobs.start/saveDraft/submit/resumeRework、attachments.add）は、社内・外部を問わず次の表で判定する（SR03「内部技術者の書込みにも必要なAssignment条件を適用する」の具体化）。

| 条件 | 結果 |
|---|---|
| 入力型にjobIdがある操作でjobIdを省略 | VALIDATION（fieldErrors.jobId、D01順位1） |
| 入力jobIdのactive Assignmentが自己で、Job.unitId＝対象Unit、now∈作業窓 | 許可 |
| 入力型にjobIdが無い操作（alerts.acknowledge/resolve、devices.addResponseNote） | 対象Unit（Alert.unitId、Deviceの現在unitId）に自己のactive Assignmentがあり、そのいずれかの作業窓内なら許可 |
| 作業窓の開始前 | FORBIDDEN（errors.assignment_not_started、IR49） |
| 作業窓の開始後に終了・revoked | FORBIDDEN（errors.assignment_ended、IR93） |
| 入力jobIdの案件に自己のAssignmentが一度も無い、または作業窓開始前に取消でrevoked | NOT_FOUND（IR93） |
| 入力型にjobIdが無い操作で、対象Unitに自己のAssignmentが一度も無い | 社内でunit scope内ならFORBIDDEN（errors.assignment_required、D01順位4）。scope外、または外部技術者はNOT_FOUND |

devices.updateFirmwareは、要求時にDevice.connection≠onlineならD01順位8のOFFLINE（DeviceOperationを作らない）。作成後1秒の開始tickでonline以外になった場合だけIR67のfailed・failureCode=OFFLINEとする。devices.checkは接続確認のためconnectionにかかわらず受け付ける。

jobs.assignとmembers.eligibleは、候補技術者のMembership.scopesがJob.unitIdを含む（unit/property/organization scopeの包含、SR03）ことを条件にする。含まない技術者は候補に出さず、直接指定はFORBIDDEN（errors.technician_out_of_scope）。members.eligibleとmembers.capacityはrole=technicianのMembershipだけを返す。contractorのmembers.listも自社organizationのrole=technicianだけを返し、HQのmembers.listは管理scope内の全roleを返す。

## IR95 業務イベントの通知 — G1-002

Policy由来のAlert・品質通知（SR21/SR28/D08）、制限予告（IR05）、督促（IR04）はそれぞれの節に従う。それ以外の業務イベントの通知は次の表だけで生成する（DEC-55）。表に無いイベント（IR48のOffer期限到来、閲覧、下書き保存、メモ、プレビュー等）は通知を作らない。

共通規則: channel=inApp、deliveryState=simulated、宛先Membershipごとに1件、target・paramsはD08/D12、occurredAt=遷移のnow、イベントを起こした操作のMembership（actor）には送らない、宛先は現在scopeと有効期間で再判定し閲覧できないMembershipには作らない、同じイベントIDの再送で増やさない。templateKeyとtypeは同名（IR10）。

| イベント | templateKey | target | 宛先 |
|---|---|---|---|
| job.requested（jobs.create、plans.generateNext） | job_update | job | 案件Unitを閲覧できる顧客のclient Membership、HQのjob.manage保持者 |
| job.offered | job_update | job | Offer先業者のpartner.accept保持者、顧客client（表示は「手配中」） |
| job.accepted | job_update | job | HQのjob.manage保持者、顧客client |
| job.declined | job_update | job | HQのjob.manage保持者 |
| job.assigned（初回・再割当・延長） | schedule_change | job | 新Assignmentの技術者、顧客client、HQのjob.manage保持者、外注なら受託業者のpartner.assign保持者 |
| report.submitted | job_update | job | 外注は受託業者のpartner.review保持者、社内はHQのjob.manage保持者、顧客client（進捗だけ） |
| report.returned | report_return | job | 担当技術者、HQのjob.manage保持者 |
| job.completed | completion | job | 顧客client、HQのjob.manage保持者、担当技術者、外注なら受託業者のpartner.review保持者 |
| job.cancelled / on_hold / resumed | job_update | job | 顧客client、担当技術者、外注なら受託業者のpartner.assign保持者、HQのjob.manage保持者 |
| restriction.requested / applied / release_requested / released / cancelled | restriction | restriction | IR19の全対象Unitを閲覧できる顧客client、HQのrestriction.manage保持者 |
| payment.confirmed（入金確認） | payment | invoice | 請求を閲覧できる顧客client、HQのbilling.manage保持者 |
| inquiry.received / answered | inquiry | inquiry | receivedはHQのbilling.manage保持者、answeredは問い合わせ元顧客のclient |
| policyの無いAlertのopen（device事象、IR98のload_alert、seed以外で生成されたもの） | alert | unit | 当該Unitを閲覧できる顧客client、HQのalert.resolve保持者、閲覧窓内の担当技術者 |
| device_operation.failed | device_operation | device | 操作を作成したMembership、HQのdevice.manage保持者 |

正規型のNotification.templateKeyとNotificationTypeにjob_updateとdevice_operationを追加する。seedの4 actorでは、例えばjob.assignedをcontractor-aが行うと、tech-external-a・customer-a・hq-operator・hq-restriction-managerに各1件（計4件）、actorのcontractor-aには0件となる。

## IR96 制限の取消 — G1-003・G1-013

restrictions.cancel（restriction.manage、reason必須）の結果は次の表だけで決める（DEC-56）。

| 現在のstate | 結果 |
|---|---|
| scheduled | cancelled。Commandは作らず、予告通知の証跡は保持 |
| requested / applied | release_requested。releaseIntent.source='cancel'で、IR35と同じ遷移内でD03の設備別解除評価を行う |
| release_requested | 冪等に現在のRestrictionを返し、versionと監査（拒否を除く）を増やさない |
| released / cancelled | CONFLICT（D01順位6） |

IR35の解除要求の起動経路は、入金確認・猶予/例外・強制解除・取消（本節）の4つと、`restrictions.release`の明示要求（source='manual'）である。正規型のRestrictionに`releaseIntent:ReleaseIntent|null`（source、at、actorMembershipId）を追加し、RestrictionReleaseViewには`releaseIntent:{source,at}|null`を含める。client向け投影ではactorMembershipId='masked'（IR42）。A10でoverride専用者にreconcile/retry(phase=release)を表示するのは、releaseIntent.source='override'または未解決recoveryCasesがあるときだけ（IR03）。

## IR97 受入fixtureの不変条件と入力オブジェクト — G1-004・G1-005・G1-009・G1-011・G1-019・G1-031

1. patchやseriesで作る測定もD07の範囲とIR12の正規化規則を満たす。origin=measured・quality=validの値は範囲内でなければfixture欠陥として生成時に例外にする。AT-C06-E.3の実績120 kWhは、unit-online-rtoとunit-non-rtoの2台×60 kW×60 slotと、同じ2台の基準100 kWh（baseline-energy-100-two-units）で作る。
2. AssignmentのpatchはscheduledStart=validFrom、scheduledEnd=validUntilを同値にする。そのAssignmentがJob.assignmentIdなら、Job.scheduledSlotも同じ枠にする。
3. 状態フィールドだけのpatchは、他の資源と不変条件を持たない値（Notification.readAt等）に限る。Restriction.state、報告版を伴うJob.status（submitted/completed/rework_requested）、Payment/Invoiceの状態は通常の操作で作る。
4. 受入試験は既定でsimulator=falseで開始する（DEC-58）。自動生成そのものを検証するAT-REV18-001、AT-REV19-003、AT-REV19-009だけsimulator=trueとする。
5. 保存入力を伴う受入は、acceptancePatchesの`input`に正規型の完全な入力オブジェクトを置き、本文から参照する（AT-A05-N、AT-A11-N、AT-A12-N等）。UIやRepositoryが欠けた必須値を補わない（SR28）。
6. 受入本文の「通知」「通知プレビュー」は、inAppの保存Notification（deliveryState=simulated）を指す。保存しないnotifications.previewは「プレビュー（保存0件）」と書く。
7. AT-A12-N/Bは`acceptancePatches["AT-A12-N"]`でdevice-tamperにCO₂センサー（sensor-tamper-co2）を加え、同じキーの`evaluation`（両設備のco2=1100 ppm、observedAt=00:59:00Z、valid、occurredAt=01:00:00Z）でautomations.fireを行う。unit-non-rtoは換気非対応のため制御results=suppressed/invalid_capability、通知はcreated（SR25）。

validate_documents.pyは1・2を全acceptancePatchesで検査し、受入計画CSV（acceptance-review-019/020）が参照するキーの存在も検査する。

## IR98 アレルゲン観測と原因候補Alertのデモデータ — G1-006・G1-027

アレルゲン観測（DEC-57）はdemoSeed.allergenObservationsの行（id、unitId、availability、substance、value、unit、sourceLabel、observedAt、evidenceText、createdAt）を取得元とする。telemetry.seriesの対象が1Unitのとき、そのUnitの行のうちobservedAt降順（nullは最後）、createdAt降順、id昇順の先頭をallergenObservationとして返す。行が無ければ{availability:'not_measured', 他はnull}、availability='unsupported'の行が先頭なら他の項目はnull。複数Unitの場合はnull（D12）。available行はsubstance・sourceLabel・observedAt・evidenceTextを必須とし、valueがあってunitがnullの行はそのまま返してUIは「不明」と表示する。

DemoTriggerに次の2つを追加する（demo-only、IR60）。
- `{eventType:'allergen', observation:{unitId, availability, substance, value, unit, sourceLabel, observedAt, evidenceText}}`: allergenObservationsに1行を追加する。
- `{eventType:'load_alert', unitId, causeCode, evidenceKind, evidenceText, severity}`: policyId=null、type=sensor、status=openのAlertをobservedAt=detectedAt=nowで作る。IR66の同一事象キーとIR95の通知規則を適用する。

demoSeedには、unit-online-rtoのavailable行（ダニ由来アレルゲンのデモ値）と、unit-non-rtoのunsupported行を置く。その他のUnitは行が無くnot_measuredとなる。

受入のfixtureは次のとおり。
- AT-C07-SRC/AT-A12-SRC: ①available＝unit-online-rto、②unsupported＝unit-non-rto、③not_measured＝unit-limited、④単位欠落＝`acceptancePatches["AT-C07-SRC.4"]`／`["AT-A12-SRC.4"]`。
- AT-C08-SRC/AT-T07-SRC/AT-A05-SRC: 同名の`acceptancePatches`を使う。いずれも`shared:load-cause-alerts`（窓開放の疑い＝seedのalert-window-a、断熱不足の点検記録＝alert-insulation-a、根拠なし＝alert-unknown-a）をincludeし、AT-C08-SRCはcustomer-a宛の通知2件、AT-T07-SRCはtech-internal-aの担当案件（`shared:tech-internal-a-job-online`のjob-t07）を加える。

## IR99 空気環境の案内表示 — G1-007

C07（とA12の表示）は、対象1Unitの最新Measurementから次の案内を表示する。閾値はデモ値（DEC-09、DEC-57）で、健康上の判断を示さない。案内はCommandを作らない。

| 指標と条件（quality=validの値だけを比較） | 案内キーと文言 |
|---|---|
| co2 ≥ 1000 ppmで、Capability.ventilation=trueかつventilationLevelsにlow | air.guidance.ventilate「換気を推奨」と換気要求ボタン |
| co2 ≥ 1000 ppmで、換気非対応 | air.guidance.ventilate_manual「窓を開けるなど手動で換気してください」（D08） |
| pm25 ≥ 35 µg/m³ | air.guidance.clean「フィルターの清掃・点検を推奨」 |
| co2・pm25のうち少なくとも1つがvalidで、上のどれにも該当しない | air.guidance.none「現在の案内はありません」 |
| co2とpm25のどちらもmissing/stale/suspect/センサーなし | air.guidance.unavailable「データが不足しているため案内できません」 |

複数の条件に該当する場合はすべての案内を表示する。temperatureとhumidityは案内の対象にしない。

## IR100 点検対象の部品集合と提出の検証 — G1-008

UnitDetail.componentsは、ACUnit.serviceScopeの各グループの部品全件である（DEC-58）（indoor 8件、outdoor 5件、electrical 5件）。並び順はグループがindoor→outdoor→electrical、グループ内はDD-T04〜T06の列挙順。初回のjobs.saveDraftでは、UIがcomponentsの全件をresult=nullのInspectionItemInputとして送る。saveDraftは一部の部品だけの保存も受け付ける。

jobs.submitは対象版を次のとおり検証し、違反があればVALIDATION（D01順位7）とし、違反した項目を全てfieldErrorsに入れる。
1. itemsのcomponentKey集合が、提出時点のUnit.componentsと一致する（不足・余分はfieldErrors.items）。
2. 全itemのresultがnullでない。
3. resultがattention・not_inspected・not_applicableのitemはreasonが1〜1000文字。
4. workTextは10〜4000文字。
5. nextActionがnullでない（follow_upは未来日時とnote 1〜1000文字）。
6. partsのquantityは1〜999。
7. attachmentRefsが全てstatus=ready。
8. measurementsのunitがmetricと一致する。

作業中にHQがserviceScopeを変えた場合は、提出時点のcomponentsで判定する。受入の全入力は`acceptancePatches["shared:report-draft-all-normal"]`の`input`（18部品normal、workText 50文字、nextAction=none）を基準にし、各受入は差分だけを本文に書く。

## IR101 共通受入条件の具体値と機種台帳のtenant — G1-025

AT-X01〜X07のN/E/Bの具体値は、共通要件定義書の「共通受入条件の具体値」表を正とする。AT-X06の非対応機種は`acceptancePatches["AT-X06-B.1"]`〜`["AT-X06-B.3"]`（温度非対応、coolだけ、送風だけ）と`["AT-X06-B.5"]`（制限できないRTO）で作る。同名Spaceの候補は`["AT-X02-B"]`で作る。

共通受入を一意にするため、次を定める（DEC-59）。
- Context.scopeVersionは表示世代の照合用であり、Repositoryは認可に現在のMembershipだけを使う。Context.scopeVersionが現在値と異なる要求は、D01順位3〜4の認可結果（NOT_FOUND/FORBIDDEN）を先に決め、認可を通る場合はCONFLICT（messageKey=errors.scope_changed、D01順位6、副作用0）を返す。UIはsession.getでContextを更新し、IR17どおりQueryを破棄して再取得する。
- Membershipがnow>=validUntilまたはnow<validFromになった後の要求はUNAUTHENTICATED（messageKey=errors.membership_inactive、D01順位2）とし、D09の期限時と同じく画面を破棄して/loginへ移る。有効期間外のMembershipへのdemoSession.signIn/switchMembershipはFORBIDDEN（errors.membership_inactive）。
- restrictions.scheduleでContract.restrictionEligible=falseの契約を指定した場合はVALIDATION（fieldErrors.contractId、messageKey=errors.restriction_ineligible、D01順位7）。
- voice.resolveIntentのcandidatesでpathLabelが同じ候補が複数ある場合、VoicePanelは各候補にunitIdを併記し、文字入力モードへ切り替えて選ばせる（FR-X02の「区別がつかない場合」）。自動で選ばない。
- AT-X04-E⑤の自己承認は`acceptancePatches["AT-X04-E.5"]`（user-tech-internal-aの2つ目のMembership hq-self-approver、role=admin、job.manage）で作る。

demoSeed.capabilitiesは所属tenantを明示する（tenantId）。tenant-bのunit-tenant-bはtenant-b用のcap-split-std-tbを参照する。別tenantの機種IDを参照する設備はfixture欠陥とする（IR91の1）。

## IR102 軽微な明確化 — G1-010・G1-012・G1-014〜G1-024・G1-026〜G1-029

- G1-010: AT-C04-E③とAT-FIX-019は時計をseedのまま（2026-09-14T01:00Z）とし、D09の366日検証でAmerica/New_Yorkの日曜02:30（2027-03-14は存在しない時刻）と日曜01:30（2026-11-01は曖昧な時刻）を検出してVALIDATION（D01順位7）とする。
- G1-012: 旧受入計画は現行規範に合わせて修正する。AT-REV17-004の省略時dueAtは2026-09-15T04:00Z（IR74）。AT-REV17-005は依存の無い設備（acceptancePatches AT-REV17-005）で確認する。AT-REV17-014の既知route上のscope外IDは、URLを維持したnot-found状態と親一覧リンク（IR57）とし、未定義routeだけSCR-X-not-foundとする。AT-REV18-003は電源断の後にrestored(power)を送ってからtamperを確認する。IR81の旧記述検査は受入計画CSVにも適用する。
- G1-014: SCR-C08はsummaries.get(kind=customer)を補助Queryに持ち、未対応アラート件数（alertCount、IR51）を未読件数と別に表示する。
- G1-015: AT-C12-E①の「保留」は、remove Commandの作成から30秒未満はperUnit.releaseState=requested（表示「解除応答待ち（通信断）」）、30秒以降はfailed（集約はrelease_requestedのまま）とする。
- G1-016: UX-05の「受け取る情報」は概要であり、propsの正はcomponent-contracts.csv（IR72の順位5）とする。
- G1-017: SCR-P03のurl_selectionにjobIdを追加する。
- G1-018: D06の確定重複の判定では、同じjobIdの置き換え対象のactive Assignmentを除外する。
- G1-019: 状態の列挙を前提にする受入（AT-A10-B、AT-C12-B、AT-P01-Nのsubmitted）は、IR97の3により通常の操作で状態を作る。
- G1-020: EmissionFactorの単位はkgCO₂e/kWhに固定（IR88）であり、BR-A14の必須項目は地域・年度・出典とする。AT-A14-Eから旧表記「係数の単位が合っていない」を削除する。
- G1-021: AT-P05-B③は、理由の無いnot_inspectedを含む報告の提出がVALIDATIONとなり、品質確認の対象にならないことを確認する（IR100）。
- G1-022: 位置情報の同意はpurpose=location_automationだけをモデル化する。旧表記の「一般的な利用の同意」は同意記録を持たない。AT-C05-B①は「位置同意granted=false」とする。
- G1-023: AT-T12-E②の「古いheartbeat」は、communication_lost(sequence=5)の後に送るrestored(axis=connection、sequence=4)とする。SR20により状態に反映せず、offlineのままとする。
- G1-024: AT-C13-N、AT-C09-N、AT-A15-N、AT-T10-N、AT-T11-Nの必須入力と手順を本文に明記する。
- G1-026: S08は、customer-bのunit-other-customerの案件を使う。contractor-aが辞退してcontractor-bへ再委託し、tech-external-bを割り当てる（IR94のscope条件）。
- G1-027: IR98のとおり。
- G1-028: AppShellのeventにswitchMembership(demoMembershipId)を追加し、ShellContainerがdemoSession.switchMembershipを実行する。
- G1-029: AT-A09-E③は「予告の宛先となる顧客Membershipが0件（全対象Unitを閲覧できるclientがいない）でschedule→VALIDATION（IR05）」とする。
