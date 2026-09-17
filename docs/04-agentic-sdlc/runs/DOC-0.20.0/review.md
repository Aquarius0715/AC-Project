# 独立G1指摘の修正報告 DOC-0.20.0（2026-09-17）

修正後baseline: `4f45da02ef91f0cf93075848d6ded3373878521740aa2f4047673eee709e4acc`（[spec-manifest](spec-manifest.json)）。

対象: DOC-0.19.0（branch docs/0.19.0-independent-review、commit 0505446）に対して別エージェントが行った独立G1（[判定記録](../DOC-0.19.0/independent-g1/review.md)、判定FAIL、MAJOR 12・MINOR 17・QUESTION 2）の指摘G1-001〜031。修正主体は本会話のAI（Opus 5）で、G1のレビュー主体とは別。修正後の確認は修正担当による**自己再レビュー**であり、DOC-0.20.0の独立G1判定ではない（[gate-G1.yaml](gate-G1.yaml)=pending、別エージェントへ依頼する）。アプリ実装・動作試験はnot_run。本番HTTP/認証/DB/実機はDEC-12・D11により1A対象外。

## 1. Executive Review Summary

修正前（DOC-0.19.0の独立G1）: **NOT READY（BLOCKER 0件、CRITICAL 0件、MAJOR 12件、MINOR 17件、QUESTION 2件）**。

修正後の自己再レビュー: 31件すべてに修正または回答を反映（resolved 29、answered 2）。追跡表64要件はすべてOK（修正前 CONFLICT 9, INCOMPLETE 32, OK 23）。`validate_documents.py`（`--tsc`付きでTypeScript strict検査も成功）と`check_review_regressions.py`（変異84件をすべて検出）はともに成功。新しい設計提案DEC-54〜59はPROPOSED（可逆）で、企業検収前に担当者が確認する。独立G1の再判定は未実施（pending）。

主な修正は次のとおり。

1. **技術者の書込み認可（G1-001）**: 認可列の修飾語をIR94で定義し、社内・外部とも自己のactive Assignmentと作業窓で判定する表を追加した。受入のfixtureに担当案件を加えた。
2. **業務イベントの通知（G1-002）**: IR95の表でtemplateKey・宛先・件数を一意にした。
3. **制限の取消（G1-003・G1-013）**: IR96の状態表と正規型のreleaseIntentを追加した。
4. **受入前提データ（G1-004・005・009・010・011・012・019）**: 範囲内の測定、Assignmentと案件枠の同期、保存入力の完全なオブジェクト、状態を操作で作る規則をIR97で定め、検証器で全patchを検査するようにした。旧受入計画の期待値も現行規則に合わせた。
5. **空気環境・アレルゲン・点検範囲（G1-006・007・008・027）**: IR98〜IR100でseed・閾値・部品集合・提出検証を定義した。
6. **共通受入（G1-025）**: AT-X01〜X07のN/E/Bの具体値表とfixtureを追加し、必要な失敗の扱いをDEC-59で定めた。

## 2. BLOCKER Issues

なし（独立G1でも0件）。

## 3. CRITICAL Issues

なし（独立G1でも0件）。

## 4. MAJOR Issues

各項目の Problem〜Suggested Revision は独立G1の記載を引用し、Applied Revision に修正内容を記す。

### G1-001

- Issue ID: G1-001
- Severity: MAJOR
- Document: 02-design/operation-catalog.csv、02-design/strict-review-contracts.md（SR03）、02-design/review-resolution-contracts.md（IR49/IR67）、02-design/implementation-contracts.md（DDC-07/DDC-08）、01-requirements/technician.md、04-agentic-sdlc/acceptance-review-019.csv
- Location: operation-catalog.csv:3（alerts.acknowledge）、:20（commands.create）、:35（devices.addResponseNote）、:37（devices.calibrate）、:45（devices.updateFirmware）／strict-review-contracts.md:24／review-resolution-contracts.md:343, 345, 430／implementation-contracts.md:240, 288／deterministic-contracts.md:21, 25, 83／technician.md:201（AT-T07-N）, :284（AT-T11-E③）, :308（AT-T12-N）／acceptance-review-019.csv:22（AT-REV19-021）（DOC-0.19.0、commit 0505446の行番号）
- Problem: 操作カタログの認可列は、技術者について`technician:alert.resolve:assigned`（alerts.acknowledge/resolve）、`technician:device.maintain:assigned-valid-job`（devices.addResponseNote/bind/calibrate/check/updateFirmware/register）、`technician:control.diagnose:job-required`（commands.create）を定めるが、`assigned`／`assigned-valid-job`／`job-required`等の修飾語の定義はカタログ以外のどの文書にも無い（全文検索で該当なし）。SR03は「内部技術者の書込みにも必要なAssignment条件を適用する」とし、DDC-08はaddResponseNoteに「有効な担当であること」を要求する。一方IR49は「社内技術者のunit scopeによる設備読取はSR03どおりだが、案件に紐づく書込みは作業窓内でなければFORBIDDEN」と、jobIdを伴う書込みだけを明示する。これに対し、AT-T12-N（tech-internal-aがdevice-tamperにresponseNote）、AT-T07-N（tech-internal-aがalert-temp-aをacknowledge。acceptancePatchesにJob/Assignmentなし）、AT-REV19-021（tech-internal-aがdevice-online-rtoをcalibrate）は、担当案件の無い社内技術者の書込み成功を期待する。AT-T11-E③（tech-internal-aがdevice-offline-rtoのFW更新、unit-offline-rtoの担当案件なし）は`OFFLINE`を期待するが、案件必須ならD01順位4のFORBIDDENが先になり、案件不要でもIR67ではupdateFirmwareはqueuedで作成され1秒後に`failed/failureCode=OFFLINE`になるため、DomainError OFFLINEか操作記録のfailureCodeかも決まらない。
- Why it matters: Repository側の認可行列（Role別アクセス範囲）が決まらない。IR72の順位ではSR03/カタログがATより上位だが、`assigned`の意味自体が未定義のため上位規範でも解消できない。P0のFR-T07を含む技術者フローの受入が実装方針によって合否が反転する。
- Example Failure: 実装Aはカタログの`assigned-valid-job`に従い、案件の無いtech-internal-aのaddResponseNoteをFORBIDDENにしてAT-T12-N②が失敗する。実装Bはunit scopeだけで許可し、AT-T12-Nは通るが、カタログ契約テスト（案件必須）とSR03に反する。AT-T11-E③はAではFORBIDDEN、BではwriteがDeviceOperationを返して1秒後にfailed/OFFLINEとなり、どちらも「OFFLINE」という期待値と照合方法が一致しない。
- Required Fix: 操作カタログ認可列の修飾語を凡例として定義する。技術者の書込みについて「社内/外部 × jobIdあり/なし × 作業窓内/外」の可否表を1か所に置き、alerts.acknowledge/resolve、devices.*、commands.create、diagnosticRuns.createに適用する。AT-T07-N/T12-N/T11-E③/AT-REV19-021をその表に合わせ、必要ならacceptancePatchesにJob/Assignmentを追加する。AT-T11-E③の観測対象（DomainErrorかDeviceOperation.failureCodeか）を明記する。
- Suggested Revision: IR94として「`client:self`=自己customer組織scope、`contractor:accepted-valid-offer`=IR23のsummary投影期間、`technician:assigned`=社内はMembership.scopes内、外部は自己Assignmentの閲覧窓、`technician:*:assigned-valid-job`=入力jobIdが有効Assignmentの作業窓内（社内技術者がjobIdを省略した場合はunit scope内で許可／不可のいずれかを明記）」を定め、AT-T11-E③のThenを「DeviceOperation.status=failed、failureCode=OFFLINE、firmwareVersion不変（IR67）」へ変更する。
- Applied Revision: 認可列の修飾語を表で定義し、技術者の書込み（社内・外部とも）を自己のactive Assignmentと作業窓で判定する表を追加。入力型にjobIdが無い操作で担当が無い社内技術者はFORBIDDEN（errors.assignment_required）、jobId付きで担当が無い場合はIR93のNOT_FOUND。updateFirmwareは要求時にonline以外ならOFFLINE（DeviceOperationなし）。AT-T07-N/AT-T07-SRCに担当案件job-t07（shared:tech-internal-a-job-online）、AT-T12-Nにjob-t12を追加。AT-T11-E③とAT-REV19-021を修正。操作カタログの注記をIR94へ向けた。
- Contract: [IR94](../../../02-design/review-resolution-contracts.md#ir94-認可列の修飾語と技術者の書込み条件--g1-001g1-026g1-030)、DEC-54（PROPOSED、可逆）
- Acceptance: AT-G120-001（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-002

- Issue ID: G1-002
- Severity: MAJOR
- Document: 02-design/implementation-contracts.md（通知と公開範囲）、02-design/service-contracts.ts、02-design/deterministic-contracts.md（D08/D12）、02-design/review-resolution-contracts.md（IR10）、01-requirements/contractor.md
- Location: implementation-contracts.md:190-205／service-contracts.ts:108（Notification.templateKey）、:166（NotificationType）／review-resolution-contracts.md:64（IR10）、:32（IR04）、:36（IR05）、:337（IR48）／deterministic-contracts.md:121-129, 169／strict-review-contracts.md:183（SR28）／contractor.md:115（AT-P03-N④）／fixture-contract.json actors（admin 3件）（DOC-0.19.0、commit 0505446の行番号）
- Problem: 通知が生成される条件と内容が定義されているのは、Policy由来のAlert（SR21/SR28）、制限予告（IR05）、督促（IR04）だけである。implementation-contracts.mdの「通知と公開範囲」表はjob.requested/offered/accepted/declined/assigned/schedule_changed、report.submitted/returned、job.completed、payment.confirmed、device.fault/operation_failed、inquiry.received/answeredで「顧客・HQ・担当技術者…に伝えます」と宛先の種類を述べるだけで、templateKey、Notification.type、channel、deliveryState（preview/simulated）、宛先となるMembershipの選び方（例:「HQ」はfixtureのadmin 3件のうちどれか）を定めない。さらにtemplateKeyのenum（alert/quality/schedule_change/report_return/completion/payment/payment_reminder/restriction/inquiry）には依頼・委託・受諾・割当・機器障害に対応する値が無く、表どおりの通知を正規型で表現できない。D08の通知リンク表はjob/device/inquiry宛の通知を前提にしている。
- Why it matters: FR-X07（P0）の通知の仕組みのうち業務イベント由来の部分が「UIはあるがデータ定義が無い」状態で、実装Agentが件数・宛先・文面キーを推測することになる。受入の期待件数も決まらない。
- Example Failure: AT-P03-N④「担当技術者へ通知プレビュー1件」について、実装Aはschedule_change/inApp/simulatedのNotificationを顧客・admin 3件・技術者・業者に計6件保存し、実装Bは技術者宛のpreviewだけを返す。テストAgentはどちらが正しいか判定できない。
- Required Fix: 業務イベントごとに、生成の有無、templateKey、type、channel、deliveryState、target、宛先Membershipの選定規則（permission・scope条件）、件数を表で定める。enumに不足する値を追加するか、既存値への写像を明記する。AT-P03-N④をその表に合わせる。
- Suggested Revision: 「job.assigned: templateKey=schedule_change、type=schedule_change、channel=inApp、deliveryState=simulated、target={kind:job}、宛先=当該Assignmentの技術者Membership 1件＋顧客のclient Membership（全対象Unit閲覧可）＋job.manageを持つadmin」のような行をイベントごとに追加し、生成しないイベント（例: IR48の期限到来）も列挙する。
- Applied Revision: 業務イベントごとのtemplateKey・target・宛先の表と共通規則（inApp・simulated・宛先ごと1件・actor除外・再判定・同一イベントで増やさない）を追加。正規型のtemplateKey/NotificationTypeにjob_update・device_operationを追加。implementation-contracts.mdの通知表をIR95へ委譲。AT-P03-N④を具体件数（4件、actor 0件）に修正。
- Contract: [IR95](../../../02-design/review-resolution-contracts.md#ir95-業務イベントの通知--g1-002)、DEC-55（PROPOSED、可逆）
- Acceptance: AT-G120-002（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-003

- Issue ID: G1-003
- Severity: MAJOR
- Document: 02-design/review-resolution-contracts.md（IR35）、02-design/admin.md（DD-A10）、02-design/common.md §5、01-requirements/admin.md（FR-A10）、02-design/service-contracts.ts
- Location: review-resolution-contracts.md:236, 238, 240／設計admin.md:55, 395／02-design/common.md:181, 185／要件admin.md:268（BR-A10）, :275（AT-A10-E④）, :276（AT-A10-B②）／strict-review-contracts.md:127（SR19）／service-contracts.ts:84, 281（DOC-0.19.0、commit 0505446の行番号）
- Problem: IR35は「解除要求（state=release_requested）の起動経路は次の3つだけ」（入金確認、defer/exempt、override）とし、別途`restrictions.release`（source='manual'）を定める。`restrictions.cancel`は含まれない。一方、DD-A10は「適用の要求を出したあとの取消は、反映されたかどうかわからなくても解除の流れに進めます」、common.md §5本文は「requested以降に取り消したときは…解除の流れに進めます」とし、AT-A10-B②は「requestedの状態でcancelする→release_requestedになる」、AT-A10-E④は「requested中にcancelする→解除の流れに進み」を期待する。同じcommon.md §5の状態表（:181）のrequested/applied→release_requestedの起動条件にはcancelが無い。cancelによる遷移のreleaseIntent.source、D03の設備別解除評価の実行有無、applied/release_requestedでのcancelの結果（CONFLICTか）はどこにも無い。SR19は「予告はcancel、適用済み/結果不明はreconcile/release」とcancelを予告段階に限る読み方をしている。
- Why it matters: IR72の順位ではIR35（順位2）がDD（順位7）・AT（順位8）に優先し、requestedへのcancelはrelease_requestedを起こさないことになるが、その場合の結果が未定義である。制限の解除はFR-A09/A10（P0）の中核で、Command生成の有無に直結する。
- Example Failure: 実装AはIR35に従いrequestedへのcancelをCONFLICTにしてAT-A10-B②が失敗する。実装BはDD-A10に従いrelease_requestedへ遷移させ、remove Commandを作るが、releaseIntent.sourceに定義外の値を入れる。
- Required Fix: `restrictions.cancel`の状態別の遷移表（scheduled/requested/applied/release_requested/released/cancelled）を定め、IR35の起動経路一覧にcancelを加えるか、requested/appliedでのcancelを拒否してreleaseへ誘導するかを一意にする。DD-A10、common.md §5、FR-A10、AT-A10-B/Eを同じ内容に揃える。
- Suggested Revision: IR35に「④取消: restrictions.cancelがrequested/appliedに対して行われた同一遷移でrelease_requestedへ遷移し、releaseIntent.source='cancel'。scheduledはcancelled、release_requestedは冪等に現在状態、released/cancelledはCONFLICT」を追加する。
- Applied Revision: restrictions.cancelの状態表（scheduled→cancelled、requested/applied→release_requested（source=cancel）、release_requestedは冪等、released/cancelledはCONFLICT）を追加。IR35の起動経路に取消を加え、DD-A10・common.md §5・AT-A10-B/E④を同じ表へ揃えた。
- Contract: [IR96](../../../02-design/review-resolution-contracts.md#ir96-制限の取消--g1-003g1-013)、DEC-56（PROPOSED、可逆）
- Acceptance: AT-G120-003（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-004

- Issue ID: G1-004
- Severity: MAJOR
- Document: 04-agentic-sdlc/fixture-contract.json、02-design/deterministic-contracts.md（D07）、02-design/review-resolution-contracts.md（IR12/IR69）、01-requirements/client.md、01-requirements/admin.md、acceptance-review-018.csv、acceptance-review-019.csv、acceptance-fixes.csv
- Location: fixture-contract.json:1816-1879（`acceptancePatches["AT-C06-E.3"]`、series value=120、expected kWh=120.0）／deterministic-contracts.md:101／review-resolution-contracts.md:74, 438／client.md:178（AT-C06-E .3）／要件admin.md:344-345（AT-A13-E/B）／acceptance-review-018.csv:25（AT-REV18-024）／acceptance-review-019.csv:42（AT-REV19-041）／acceptance-fixes.csv:18（AT-FIX-017）（DOC-0.19.0、commit 0505446の行番号）
- Problem: D07は「1A合成値の範囲は…power/kW:[0,100]」「範囲外…はIR12のRawMeasurement正規化でsuspectとして値を集計・制御から除く」と定める。AT-C06-E.3のpatchは1時間窓・1台で「実績120 kWh」を作るため、各分のpower値が120 kWとなり範囲外である。それにもかかわらずquality=valid・qualityReason=nullで置き、期待値kWh=120.0、savingPercentage=-20とする。AT-FIX-017は範囲外値を「suspectで除外」と集計除外まで期待している。patch経由の行に範囲検査を適用するか（IR69「正規DTO schemaで検証して不正なら例外」）、集計時に再判定するかは定義されていない。
- Why it matters: 負の削減量表示（IR68/IR80）を検証する受入の前提データが、上位規範の不変条件に反する状態でしか作れない。/demoからの通常投入（demo.trigger telemetry）ではこの状態を再現できない。
- Example Failure: 実装Aは正規DTO検証にD07の範囲を含めてfixture生成時に例外となりAT-C06-E.3が実行不能。実装Bは集計時に範囲外を除外してkWh=null・coverage=0を返しAT-REV19-041が失敗。実装Cは保存済みqualityだけを信じて120.0を返し、AT-FIX-017の「集計から除外」と整合しない。
- Required Fix: E.3の前提をD07の範囲内で作るか、D07の範囲を変更する。どちらの場合もfixture・AT本文・AT-REV19-041の期待値を同時に更新し、patch行に範囲検査を適用するかを明記する。
- Suggested Revision: 案1: D07のpower範囲を[0,200] kWへ変更する。案2: E.3を`unitIds=[unit-online-rto, unit-limited]`・各60 kW・基準100 kWh（同じ2台）に変更し、DTO savedKWh=-20、savingPercentage=-20を維持する。あわせてIR69に「patchで作る測定もD07/IR12の正規化規則を満たすこと（違反はfixture欠陥）」を追記する。
- Applied Revision: AT-C06-E.3を2台×60 kWと同じ2台の基準100 kWhに変更（D07のpower範囲内）。検証器が全acceptancePatchesでmeasured/validの値の範囲を検査する。AT-A13-B②・AT-REV18-024・AT-REV19-006・AT-REV19-041の参照も更新。
- Contract: [IR97](../../../02-design/review-resolution-contracts.md#ir97-受入fixtureの不変条件と入力オブジェクト--g1-004g1-005g1-009g1-011g1-019g1-031)
- Acceptance: AT-G120-004（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-005

- Issue ID: G1-005
- Severity: MAJOR
- Document: 01-requirements/admin.md（FR-A12）、04-agentic-sdlc/fixture-contract.json、02-design/review-resolution-contracts.md（IR21/IR43/IR92）、02-design/strict-review-contracts.md（SR25）、02-design/admin.md（DD-A12）
- Location: 要件admin.md:324（AT-A12-N）, :326（AT-A12-B）／fixture-contract.json:1006-1045（device-tamperのsensorsはtemperature/humidity/powerのみ）, :1158-1162（unit-online-rtoのCO₂=800）／review-resolution-contracts.md:131（IR21「Sensor不在はmissing_data」）, :282（IR43「sensor不存在=NOT_FOUND」）, :679（IR92規則2）／strict-review-contracts.md:159（SR25「通知の成立条件は…指標の有効品質・閾値/継続時間」）／設計admin.md:452（DD-A12「unitIds/metric…対応するセンサーがあること」）（DOC-0.19.0、commit 0505446の行番号）
- Problem: AT-A12-Nは「換気対応のunit-online-rto・非対応のunit-non-rto」を対象にmetric=co2・threshold=1000ppmで保存・評価し、「対応していない設備には通知のみ」を期待する（AT-A12-Bも同じ）。しかしunit-non-rtoの現bindingであるdevice-tamperにはCO₂センサーが無く、IR21によりそのUnitのco2 Factはmissing_data、IR43によりdemo.trigger telemetryもNOT_FOUNDとなるため、SR25の通知成立条件（有効品質）を満たせない。DD-A12は保存時点で「対応するセンサーがあること」を要求しており、保存自体がVALIDATIONになる可能性もある。さらにAT-A12-Nは評価に使うCO₂値・観測時刻・継続時間（durationSeconds）・recoveryThresholdを指定しておらず、seedのunit-online-rtoのCO₂は800ppmで閾値未満である。
- Why it matters: FR-A12の「換気対応設備は換気要求、非対応設備は通知のみ」という要件の中心的な分岐を、現行seedでは受入として作れない。IR92は「規則で決まらない前提は文書欠陥」としている。
- Example Failure: テストAgentがIR92の既定どおりunit-non-rtoを使うと、通知はsuppressed/qualityとなり「通知のみ」の期待が失敗する。保存時にDD-A12の検査でVALIDATIONになる実装もある。
- Required Fix: CO₂センサーを持ち換気能力を持たない設備を受入前提として用意し（acceptancePatchesでdevice-tamperにsensor-tamper-co2を追加するか、別Unitを追加）、評価入力（facts/telemetryの値・時刻・継続秒数）と期待するFireResult/NotificationOutcomeを明記する。DD-A12の「対応するセンサー」がCapability.sensorsか現binding Deviceのsensorsかも明記する。
- Suggested Revision: `acceptancePatches["AT-A12-N"]`を追加し、device-tamperのsensorsにco2（ppm、stale 120秒）を加え、unit-online-rto・unit-non-rtoにCO₂=1100ppmのmeasured/valid測定を00:59:00〜01:00:00に置く。Givenにfixture.airPolicyNotificationInput・recoveryThreshold=900・durationSeconds=60・enabled=true・recipient=hq-operatorを明記し、Thenを「unit-online-rto: results=requested（ventilate low）・notifications=created、unit-non-rto: results=suppressed/invalid_capability・notifications=created」とする。
- Applied Revision: acceptancePatches["AT-A12-N"]でdevice-tamperにsensor-tamper-co2を追加し、同じキーにPolicy入力（co2 gte 1000、recoveryThreshold 900、durationSeconds 60等）と評価入力（両設備1100 ppm、observedAt 00:59:00Z）を置いた。AT-A12-N/Bの期待結果をresults・Command・通知の単位で明記。
- Contract: [IR97](../../../02-design/review-resolution-contracts.md#ir97-受入fixtureの不変条件と入力オブジェクト--g1-004g1-005g1-009g1-011g1-019g1-031)
- Acceptance: AT-G120-005（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-006

- Issue ID: G1-006
- Severity: MAJOR
- Document: 02-design/service-contracts.ts、02-design/deterministic-contracts.md（D12）、02-design/client.md（DD-C07）、02-design/admin.md（DD-A12）、01-requirements/client.md、01-requirements/admin.md、04-agentic-sdlc/fixture-contract.json、02-design/review-resolution-contracts.md（IR69/IR92）
- Location: service-contracts.ts:316-317（AllergenObservation、AirSeries）、:142（DemoTrigger）／deterministic-contracts.md:173／設計client.md:252／設計admin.md:434-440／要件client.md:189（AT-C07-SRC）／要件admin.md:311（AT-A12-SRC）／fixture-contract.json demoSeed（アレルゲンの節なし）／review-resolution-contracts.md:438, 682, 687（DOC-0.19.0、commit 0505446の行番号）
- Problem: allergenObservationはtelemetry.seriesの付帯情報として出力型とavailability（available/not_measured/unsupported）の表示規則だけが定義されている。Repositoryがこの値を何から作るか（保存資源、Capabilityの属性、Measurement以外のイベント）、not_measuredとunsupportedを分ける条件は定義されていない。demoSeedにアレルゲン観測の節は無く、DemoTriggerにもアレルゲン投入のイベントが無い。IR69は「そこに無い業務記録をRepositoryが補わない」、IR92はpatchのentityをdemoSeedの節に限るため、AT-C07-SRC/AT-A12-SRCが求める「作り込んだ観測データ」のfixtureを規則どおりに作る方法が無い。
- Why it matters: 企業原文BIZ-18（アレルゲン）の対応として明記された表示が、データ定義の欠落により実装も受入もできない（「UIはあるがデータ定義が無い機能」）。
- Example Failure: 実装Agentは常にnot_measuredを返す実装しか作れず、AT-C07-SRCの「available」fixtureを用意できない。別の実装Agentは独自の保存資源を追加し、IR69/D12に反する。
- Required Fix: allergenObservationの取得元（例: demoSeedの新しい節、またはCapability/Deviceの属性とDemoTriggerの新イベント）、availabilityの導出規則、最新1件の選び方、複数Unit時のnullの扱い、acceptancePatchesのキーを定義する。
- Suggested Revision: demoSeedに`allergenObservations: [{id, unitId, substance, value, unit, sourceLabel, observedAt, evidenceText}]`を追加し、「Capabilityに`allergenSupported:boolean`を持ち、falseならunsupported、trueで観測0件ならnot_measured、観測ありならavailable（observedAt降順先頭）」とする。AT-C07-SRCとAT-A12-SRCに3件のacceptancePatchesキーを付ける。
- Applied Revision: demoSeed.allergenObservations（unit-online-rtoにavailable、unit-non-rtoにunsupported）と選択規則、DemoTriggerのallergen/load_alertを追加。AT-C07-SRC/AT-A12-SRCの4 fixture（④はacceptancePatches["AT-C07-SRC.4"]／["AT-A12-SRC.4"]）を定義。
- Contract: [IR98](../../../02-design/review-resolution-contracts.md#ir98-アレルゲン観測と原因候補alertのデモデータ--g1-006g1-027)、DEC-57（PROPOSED、可逆）
- Acceptance: AT-G120-006（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-007

- Issue ID: G1-007
- Severity: MAJOR
- Document: 01-requirements/client.md（FR-C07）、02-design/client.md（DD-C07）、02-design/deterministic-contracts.md（D08）
- Location: 要件client.md:195（基本フロー）, :196（BR-C07）, :202（AT-C07-N②「案内『換気を推奨』」）／設計client.md:272-277／deterministic-contracts.md:119（DOC-0.19.0、commit 0505446の行番号）
- Problem: FR-C07は「換気や清掃の案内を見る」と定め、AT-C07-Nは「room-1: CO₂=1000ppm…→②案内『換気を推奨』」を期待する。しかし、どの指標がどの値（または品質）のときに「換気を推奨」を表示するか、清掃の案内をどの条件（PM2.5等）で出すか、条件を満たさないときに何を表示するかは、FR-C07・DD-C07・D08のどこにも無い（全文検索でも該当規則なし）。定義されているのは換気非対応設備の手動換気案内（D08）だけである。
- Why it matters: 企業原文BIZ-18「清掃・換気の案内」の表示条件を実装Agentが数値で推測することになり、「数値…を実装時に推測しない」という各文書冒頭の実装基準に反する。AT-C07-N②の合否も判定できない。
- Example Failure: 実装AはCO₂≥1000ppmで推奨を出し、実装BはA12のPolicy閾値があるときだけ出す。seedのCO₂=800ppmの表示で両者の画面が異なり、どちらもテストで否定できない。
- Required Fix: 案内の表示規則（指標、比較演算子、閾値、必要な品質、換気能力の有無ごとの文言キー、清掃案内の条件）を定義し、AT-C07-N/E/Bに境界値のsubcaseを追加する。
- Suggested Revision: DD-C07に「co2 value≥1000 ppmかつquality=validなら`air.guidance.ventilate`、pm25≥35 µg/m³かつvalidなら`air.guidance.clean`、欠測・stale・suspectでは案内を出さず品質表示のみ（デモ閾値、DEC-09）」を追加し、AT-C07-Bに999/1000ppmの境界を加える。
- Applied Revision: 空気環境の案内表（co2≥1000 ppm→換気推奨／手動換気、pm25≥35 µg/m³→清掃点検、該当なし、データ不足）を追加。DD-C07、AT-C07-N/E/Bに反映（AT-C07-B④〜⑦を追加）。
- Contract: [IR99](../../../02-design/review-resolution-contracts.md#ir99-空気環境の案内表示--g1-007)、DEC-57（PROPOSED、可逆）
- Acceptance: AT-G120-007（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-008

- Issue ID: G1-008
- Severity: MAJOR
- Document: 02-design/technician.md（DD-T02/DD-T09）、02-design/implementation-contracts.md（DDC-02）、02-design/service-contracts.ts、01-requirements/technician.md
- Location: 設計technician.md:94（components/serviceScope）, :288-293（workText「提出時必須 10〜4000文字」、inspectionItems「提出時必須 対象全部に結果/理由」、nextAction.kind「必須」）／implementation-contracts.md:119／service-contracts.ts:43（UnitDetail.components）／fixture-contract.json units（unit-online-rtoのserviceScope=indoor/outdoor/electrical）／要件technician.md:138（AT-T04-N）, :157（AT-T05-N）, :176（AT-T06-N）, :220（AT-T08-N）, :239（AT-T09-N）（DOC-0.19.0、commit 0505446の行番号）
- Problem: 提出時の検証は「対象全部に結果/理由」を要求するが、「対象」を決めるUnitDetail.componentsをserviceScope等からどう導出するかは定義されていない。unit-online-rtoのserviceScopeは3グループ（計18部品）である。AT-T04-Nはindoor 8部品だけを入力して「②提出成功」を期待し、AT-T05-N（outdoor 5部品）、AT-T06-N（electrical 5部品）も同様である。これらのATにはworkText（提出時必須）とnextAction（必須）の入力も無い。逆にAT-T09-Nは本文・写真・部品・nextActionだけで点検結果を入力せずに提出成功を期待し、AT-T08-Nは「編集」とだけ書く。
- Why it matters: 技術者の点検・報告（FR-T04〜T06/T08/T09、P0）の受入で、書かれた入力どおりに操作すると提出がVALIDATIONになり、期待結果に到達しない。必須集合の導出が未定義のため、実装Agentも検証範囲を決められない。
- Example Failure: テストAgentがAT-T04-Nの記載どおりindoor 8部品だけを保存して提出すると、outdoor/electricalのresult=null・workText欠落でVALIDATIONとなる。別の実装はindoorだけで提出を通し、AT-T04-E①（result=null 1部品でVALIDATION）の判定範囲がグループ単位に縮む。
- Required Fix: UnitDetail.componentsの導出規則（serviceScopeのグループ→ComponentKey一覧）と、jobs.submitの必須検証集合を定義する。AT-T04/T05/T06/T08/T09-Nの前提に、対象外グループを含む全点検結果・workText・nextActionの入力値を明記するか、共通の「提出可能な下書き」fixtureを定義する。
- Suggested Revision: DD-T02に「components=serviceScopeの各グループのComponentKey全件（indoor 8、outdoor 5、electrical 5）」を追加し、AT-T04-NのWhenを「filter=attention（理由・写真1枚）、他の17部品=normal、workText=50文字、nextAction=noneで保存→提出」とする。T05/T06/T09-Nも同様に全入力を明記する。
- Applied Revision: UnitDetail.components＝serviceScopeの各グループの全部品（8/5/5）と提出時の8つの検証規則を追加。acceptancePatches["shared:report-draft-all-normal"]に18部品normalの完全な入力を置き、AT-T04/T05/T06/T08/T09-N・AT-C09-N・AT-P01-Nから参照。DD-T02/T09の項目表を更新。
- Contract: [IR100](../../../02-design/review-resolution-contracts.md#ir100-点検対象の部品集合と提出の検証--g1-008)、DEC-58（PROPOSED、可逆）
- Acceptance: AT-G120-008（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-009

- Issue ID: G1-009
- Severity: MAJOR
- Document: 01-requirements/admin.md（FR-A05/A11/A12）、02-design/admin.md（DD-A05）、02-design/review-resolution-contracts.md（IR07）、02-design/strict-review-contracts.md（SR21/SR28）
- Location: 要件admin.md:161（AT-A05-N）, :293（AT-A11-N）, :324（AT-A12-N）／設計admin.md:219-229（DD-A05の必須欄）, :235／review-resolution-contracts.md:48（IR07「新規は…enabled=false、priority=50」）／strict-review-contracts.md:137-139（SR21「無効disabled…はsuppressed」）, :181（SR28「必須値をRepositoryやUIが勝手な数値で補完しない」）, :183（「inAppはsimulated」）（DOC-0.19.0、commit 0505446の行番号）
- Problem: AT-A05-Nの保存入力はunitIds、metric、gte 30°C、duration、severity、recipient、channelだけで、DD-A05/SR28/正規型で必須のname、recoveryThreshold、cooldownMinutes、escalateAfterMinutesが無く、enabledも指定していない。IR07の新規既定はenabled=falseで、SR21により無効なPolicyは評価でsuppressedとなるため、期待結果「②Alertが1件、Notificationのプレビューが1件」に到達しない。AT-A12-N（fixture.airPolicyNotificationInputは指定するがenabled・name・recoveryThreshold・durationSecondsなし）とAT-A11-N（enabled・name・timezoneなし）も同じ欠落を持つ。また「Notificationのプレビュー」はinAppではSR28によりdeliveryState=simulatedの保存Notificationであり、previewとの区別が本文で曖昧である。
- Why it matters: 必須値をUI/Repositoryが補完してはならない（SR28）ため、テストAgentが値を推測しない限り保存できず、既定値を使うと期待結果に達しない。FR-A05はP0である。
- Example Failure: テストAgentがAT-A05-Nの記載値だけで保存するとVALIDATION（name等の欠落）。欠落値を補ってもenabledを既定のfalseのまま保存するとAlert 0件で失敗する。
- Required Fix: AT-A05-N/A11-N/A12-NのWhenに全必須入力とenabled=trueを明記するか、fixture-contract.jsonにPolicy入力の固定オブジェクトを定義して参照させる。「プレビュー」をdeliveryState（preview/simulated）で書き分ける。
- Suggested Revision: AT-A05-NのWhenを「name=Demo high temperature、unitIds=[unit-online-rto]、metric=temperature、operator=gte、threshold=30、recoveryThreshold=28、durationSeconds=60、severity=warning、recipientMembershipIds=[customer-a]、channels=[inApp]、cooldownMinutes=5、escalateAfterMinutes=60、timezone=Asia/Kuala_Lumpur、enabled=true、priority=50で保存」とし、Thenを「Notification（deliveryState=simulated）1件」とする。
- Applied Revision: AT-A05-N/AT-A11-N/AT-A12-Nのacceptancepatchesに完全な`input`（enabled=true等）とtrigger/evaluationを置き、本文から参照。「通知」「通知プレビュー」は保存されるinApp Notification（simulated）を指すと規則化（IR97の6）。
- Contract: [IR97](../../../02-design/review-resolution-contracts.md#ir97-受入fixtureの不変条件と入力オブジェクト--g1-004g1-005g1-009g1-011g1-019g1-031)
- Acceptance: AT-G120-009（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-010

- Issue ID: G1-010
- Severity: MAJOR
- Document: 01-requirements/client.md（FR-C04）、04-agentic-sdlc/acceptance-fixes.csv、04-agentic-sdlc/fixture-contract.json、02-design/review-resolution-contracts.md（IR36/IR92）、02-design/deterministic-contracts.md（D01/D09）、04-agentic-sdlc/verification.md
- Location: 要件client.md:140（AT-C04-E③）／acceptance-fixes.csv:20（AT-FIX-019）／fixture-contract.json:14-15（customer-aのvalidFrom=2026-09-01、validUntil=2026-10-01。全actor同じ）, :371（note）／review-resolution-contracts.md:246（IR36「Membership.validUntil…はジャンプで通常どおり失効」）, :678（IR92規則1）／deterministic-contracts.md:18-21, 141／verification.md:42（`validFrom <= now < validUntil`）（DOC-0.19.0、commit 0505446の行番号）
- Problem: AT-C04-E③は時計を2026-03-07／2026-10-31に設定して週次ルールを保存しVALIDATIONを期待し、AT-FIX-019も2026-03-07を使う。しかしseedの全actorのMembership有効期間は[2026-09-01, 2026-10-01)で、両日付とも範囲外である。Givenは有効期間の上書きを指定しておらず、IR92規則1では書かれていない値はseedのままとなる。範囲外のMembershipによる保存はD01順位2/4（UNAUTHENTICATEDまたは担当期限外FORBIDDEN）に当たり得るが、DST検証をD01のどの順位で行うか（順位1の構造検証か順位7の関連値か）は定義されていない。なおD09は保存時nowから366日以内の全occurrenceを検証するため、seedの時計（2026-09-14）のままでも2026-11-01（曖昧）と2027-03-14（不存在）が検出され、時計を動かす必要自体が無い。
- Why it matters: 受入の前提データと期待するエラーコードが、seed・IR92・D01から一意に決まらない。
- Example Failure: 実装Aは2026-03-07ではcustomer-aのsignInまたは保存をFORBIDDENとしAT-C04-E③が失敗する。実装BはDST検証を先に行いVALIDATIONを返す。どちらもD01の文言に反するとは言えない。
- Required Fix: Givenに有効期間の上書きを明記するか、時計を有効期間内に置いたまま366日検証で検出するケースに書き換える。DST検証がD01のどの順位に当たるかを明記する。
- Suggested Revision: AT-C04-E③を「時計はseedのまま（2026-09-14T01:00Z）、timezone=America/New_York、日曜02:30（2027-03-14が不存在）／日曜01:30（2026-11-01が曖昧）の週次ルールを保存→VALIDATION（D01順位1）」へ変更し、AT-FIX-019も同じ前提に揃える。
- Applied Revision: AT-C04-E③とAT-FIX-019を、seedの時計（Membership有効期間内）のまま日曜02:30（2027-03-14）と日曜01:30（2026-11-01）を保存してD09の366日検証でVALIDATION（D01順位7）とする形に修正。
- Contract: [IR102](../../../02-design/review-resolution-contracts.md#ir102-軽微な明確化--g1-010g1-012g1-014g1-024g1-026g1-029)
- Acceptance: AT-G120-010（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-011

- Issue ID: G1-011
- Severity: MAJOR
- Document: 04-agentic-sdlc/acceptance-review-019.csv、02-design/review-resolution-contracts.md（IR69/IR89）、02-design/deterministic-contracts.md（D06）、04-agentic-sdlc/fixture-contract.json
- Location: acceptance-review-019.csv:16（AT-REV19-015）, :3（AT-REV19-002）, :38（AT-REV19-037）／review-resolution-contracts.md:438, 629, 631／deterministic-contracts.md:93／fixture-contract.json:1501-1504（job-contractor-a.scheduledSlot.endAt=2026-09-20T00:00Z）, :1533-1536（assignment-contractor-aのscheduled/validFrom/validUntil）（DOC-0.19.0、commit 0505446の行番号）
- Problem: AT-REV19-015は「assignment-contractor-aをscheduledEnd=01:20Zへpatch」するだけで、01:20Zに「業者/HQの一覧に『作業窓終了・再割当が必要』」を期待する。IR89はこの表示を「JobSummary/JobDetailでstatus∈{assigned,in_progress}かつscheduledSlot.endAt<=now」で判定するが、IR69/IR92により書かれていないJob.scheduledSlotはseed（endAt=2026-09-20T00:00Z）のままで、01:20Zには条件を満たさない。IR89末尾はjobs.assignでJob.scheduledSlotとAssignmentを同期させる不変条件を定めているが、patchはこれを破る。さらにD06は「AssignmentのvalidFrom/UntilはscheduledStart/Endと同じ」とするのに、patch後はvalidUntil=2026-09-20のまま食い違い、作業窓をどちらで判定するかで技術者側の結果も変わり得る。AT-REV19-002・AT-REV19-037も同じくscheduledStart/Endだけをpatchしており、AT-REV19-037の「02:00Zを過ぎても表示されない」はpatch前から表示されない状態のため、同期の検証として機能しない。
- Why it matters: ユーザー確定のDEC-50（IR72順位1）に対応する受入が、規則どおりに前提を作ると期待結果を出せない。
- Example Failure: テストAgentがGivenどおりにpatchすると、01:20Zに技術者画面は破棄・history表示になるが、HQ/業者の一覧にはIR89の表示が出ず、AT-REV19-015が失敗する。
- Required Fix: AT-REV19-002/015/037のGivenでJob.scheduledSlotとAssignment.validFrom/validUntilも同じ値にpatchする（またはjobs.assignの通常操作で作る）。patchが守るべき関連フィールドの不変条件をIR92に追記する。
- Suggested Revision: AT-REV19-015のGivenを「assignment-contractor-aのscheduledEnd=validUntil=01:20Z、job-contractor-aのscheduledSlot.endAt=01:20Z・status=in_progress」とし、AT-REV19-037は旧枠が過ぎた02:00Zの時点で再割当前に表示が出ることも確認する手順に改める。
- Applied Revision: AT-REV19-002/015/037のGivenでAssignmentのscheduled*とvalid*、Job.scheduledSlotを同値にpatchし、jobs.startは通常操作で行う形に修正。AT-REV19-037は再割当前の02:00Zで表示が出ることも確認する手順にした。検証器が全patchで同期を検査する（IR97の2）。
- Contract: [IR97](../../../02-design/review-resolution-contracts.md#ir97-受入fixtureの不変条件と入力オブジェクト--g1-004g1-005g1-009g1-011g1-019g1-031)
- Acceptance: AT-G120-011（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-012

- Issue ID: G1-012
- Severity: MAJOR
- Document: 04-agentic-sdlc/acceptance-review-017.csv、04-agentic-sdlc/acceptance-review-018.csv、04-agentic-sdlc/verification.md、02-design/review-resolution-contracts.md（IR47/IR57/IR74）、02-design/deterministic-contracts.md（D05/D14）、04-agentic-sdlc/fixture-contract.json
- Location: acceptance-review-017.csv:5（AT-REV17-004）, :6（AT-REV17-005）, :15（AT-REV17-014）／acceptance-review-018.csv:4（AT-REV18-003）, :14（AT-REV18-013④）／verification.md:209-219／review-resolution-contracts.md:329（IR47）, :390（IR57）, :513（IR74 REV18-035）／deterministic-contracts.md:87（D05）, :192（D14）／strict-review-contracts.md:133（SR20）／client.md:246（AT-C09-N）／fixture-contract.json:1006-1007（device-tamper.unitId=unit-non-rto）, :1318-1324（contract-general-a、endAt=2027-01-01）（DOC-0.19.0、commit 0505446の行番号）
- Problem: verification.mdはAT-REV17-001〜015・AT-REV18-001〜048を既存ATと合わせて検証するよう求めるが、次の期待値が現行の上位規範・seedと矛盾する。(a) AT-REV17-014は`/customer/units/unit-other-customer`で「SCR-X-not-foundを表示…認証済みならrole home」とするが、IR57は既知routeのprimary queryがNOT_FOUNDなら「URLを維持したままその場でnot-found状態（親一覧へのリンク）」とし、AT-REV18-013④も「URL維持でnot-found表示と親一覧リンク」を期待する。(b) AT-REV17-004はZの無い「希望枠2026-09-15 10:00〜12:00」から「省略時dueAt=2026-09-15 12:00Z」を期待するが、IR74 REV18-035ではZ無しはAsia/Kuala_Lumpurのローカル時刻で、AT-C09-Nも同じ枠を02:00Z〜04:00Zとしているため、dueAtは04:00Zになる。(c) AT-REV17-005はunit-non-rtoのarchive成功を前提にするが、seedではdevice-tamperがunit-non-rtoに現bindingを持ち、contract-general-a（endAt=2027-01-01）がunit-non-rtoを対象にするため、D05/D14によりarchiveはCONFLICTになる。(d) AT-REV18-003はcommunication_lost→restored→power_lost→tamperの後のcommands.createを「tamperだけではCommandを受付」とするが、restoredは接続軸だけを復旧し（SR20）、power_lostの後に電源の復旧が無いためIR47によりpowerSignal=offが最優先でOFFLINEになる。
- Why it matters: IR72は「見つけた食い違いは実装で選ばず文書欠陥として報告し、G1を停止する」と定める。旧受入計画は現行manifestに含まれ、テストAgentへの入力である。IR81の旧記述検査はMarkdownとverification.mdが対象で、受入CSVの期待値は検査されない。
- Example Failure: IR57どおりに実装するとAT-REV17-014が失敗し、AT-REV17-014に合わせるとAT-REV18-013④が失敗する。AT-REV17-005はseedのままではarchiveがCONFLICTとなり以降の手順が実行できない。
- Required Fix: 上記4件の期待値・前提を現行規範に合わせて修正する（またはverification.mdで置換済みのcaseを明示して実施対象から外す）。受入CSVの期待値も旧記述検査または整合テストの対象に加える。
- Suggested Revision: (a) AT-REV17-014の後半を「customer-aの/customer/units/unit-other-customerはSCR-C03上でURL維持のnot-found状態と親一覧リンク（IR57）。未定義routeだけSCR-X-not-found」、(b)「省略時dueAt=2026-09-15T04:00Z」、(c) Givenに「device-tamperのbindingを解除し、contract-general-aのendAtを過去へpatch」を追加するか、依存の無い新規Unitを対象にする、(d) 手順を「…→power_lost→restored(power)→tamper→commands.create」とする。
- Applied Revision: AT-REV17-004（dueAt=04:00Z）、AT-REV17-005（依存の無いunit-archive-demo、acceptancePatches["AT-REV17-005"]）、AT-REV17-014（既知routeはURL維持のnot-found）、AT-REV18-003（restored(power)の後にtamper）を修正。受入計画CSVを旧記述検査とpatchキー参照検査の対象に追加。
- Contract: [IR102](../../../02-design/review-resolution-contracts.md#ir102-軽微な明確化--g1-010g1-012g1-014g1-024g1-026g1-029)
- Acceptance: AT-G120-012（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

## 5. MINOR Issues

### G1-013

- Issue ID: G1-013
- Severity: MINOR
- Document: 02-design/service-contracts.ts、02-design/review-resolution-contracts.md（IR03/IR35）、02-design/operation-catalog.csv
- Location: service-contracts.ts:84（Restriction）, :86（RestrictionReleaseView）／review-resolution-contracts.md:26, 236／operation-catalog.csv restrictions.reconcile/retryの認可列（`release-intent-or-terminal-recovery-only`）（DOC-0.19.0、commit 0505446の行番号）
- Problem: IR35は解除要求時に`releaseIntent={source,at,actorMembershipId}`を保存するとし、IR03とカタログはoverride専用者のreconcile/retry可否をこの解除意思（override由来か）で決める。しかし正規型のRestriction/RestrictionReleaseViewにreleaseIntentは無く、内部専用値であるという宣言（IR31のcontributorUserIdsのような記述）も無い。
- Why it matters: override専用者のA10画面でreconcile/retryボタンの表示可否を、UIが取得した値から決められない。
- Example Failure: 入金確認でrelease_requestedになった制限に対し、override専用者の画面にretryボタンが出て、押すとFORBIDDENになる。
- Required Fix: releaseIntentをRepository内部値と明記するか、RestrictionReleaseViewに`releaseIntentSource`を追加する。
- Suggested Revision: 正規型に`releaseIntent:{source:'payment'|'exception'|'override'|'manual';at:Instant}|null`を追加し、RestrictionReleaseViewにも含める。
- Applied Revision: 正規型RestrictionにreleaseIntent（source・at・actorMembershipId）、RestrictionReleaseViewにreleaseIntent{source,at}を追加し、override専用者のreconcile/retry表示条件を定義。
- Contract: [IR96](../../../02-design/review-resolution-contracts.md#ir96-制限の取消--g1-003g1-013)、DEC-56（PROPOSED、可逆）
- Acceptance: AT-G120-013（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-014

- Issue ID: G1-014
- Severity: MINOR
- Document: 03-uiux/screen-catalog.csv、02-design/review-resolution-contracts.md（IR51）、02-design/client.md（DD-C08）
- Location: screen-catalog.csv:8（SCR-C08のoperations=alerts.list;notifications.markRead;notifications.list）／review-resolution-contracts.md:355／設計client.md:298-303／要件client.md:229（AT-C08-B①〜③）（DOC-0.19.0、commit 0505446の行番号）
- Problem: IR51は「FR-C08/AT-C08-Bの『未対応件数』はalertCountを指し、通知の未読件数とは別の値として別に表示する」とするが、SCR-C08の操作にsummaries.getが無く、DD-C08のフィールド表にもalertCountが無い。D07によりKPIはsummaries.getで集計し、一覧ページから計算してはならない。
- Why it matters: AT-C08-Bの観測画面（C08かC01か）が決まらない。
- Example Failure: 実装AはC08でalerts.listの結果から件数を数え、D07に反する。実装BはC08に件数を出さず、AT-C08-BをC01で観測する。
- Required Fix: 表示画面を明記し、C08で表示するならSCR-C08にsummaries.getを追加する。
- Suggested Revision: SCR-C08のoperations/secondary_queriesにsummaries.get(kind=customer)を追加し、DD-C08に「alertCount（IR51）」の読取欄を追加する。
- Applied Revision: SCR-C08と同Pageのapi_dependency、DD-C08のサービス境界と項目表にsummaries.get（alertCount）を追加。
- Contract: [IR102](../../../02-design/review-resolution-contracts.md#ir102-軽微な明確化--g1-010g1-012g1-014g1-024g1-026g1-029)
- Acceptance: AT-G120-014（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-015

- Issue ID: G1-015
- Severity: MINOR
- Document: 01-requirements/client.md、02-design/service-contracts.ts
- Location: 要件client.md:310（AT-C12-E①「offline台はpending」）／service-contracts.ts:83（releaseStateのenum）（DOC-0.19.0、commit 0505446の行番号）
- Problem: 「pending」はRestrictionUnit.releaseState（none/waiting_reconcile/requested/released/not_required/failed）に無い。unit-limitedはremove Command作成後に通信断となるため、30秒経過前はrequested、後はfailedとなり、どの値・表示を「pending」とするか決まらない。
- Why it matters: 観測値が一意でない。
- Example Failure: テストAgentがreleaseState=requestedとfailedのどちらを合格とするか判断できない。
- Required Fix: 観測時刻と期待するreleaseState/pendingReason/表示ラベルを明記する。
- Suggested Revision: 「①30秒未満ではperUnit.releaseState=requested・表示『解除応答待ち（通信断）』、30秒経過後はfailed・集約release_requestedのまま」とする。
- Applied Revision: AT-C12-E①の保留を、remove Command作成から30秒未満はreleaseState=requested、30秒以降はfailed（集約はrelease_requested）と定義。
- Contract: [IR102](../../../02-design/review-resolution-contracts.md#ir102-軽微な明確化--g1-010g1-012g1-014g1-024g1-026g1-029)
- Acceptance: AT-G120-015（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-016

- Issue ID: G1-016
- Severity: MINOR
- Document: 03-uiux/UIUXSpecification.md（UX-05）、03-uiux/component-contracts.csv
- Location: UIUXSpecification.md:142（StatusBadge: domain/status/labelKey）, :146（CommandPanel: capability/observedState/pendingCommand/permission）, :147（ConfirmActionDialog: target/action/impact/reason/onConfirm）／component-contracts.csv:2（StatusBadge: status,severity,labelKey,quality）, :4（CommandPanel: unit,pending,permission,loading,error）, :5（ConfirmActionDialog: open,target,before,after,reasonRequired,submitting,error）（DOC-0.19.0、commit 0505446の行番号）
- Problem: 同じ共通Componentのprops名・構成がUX-05とComponent契約で異なる。IR72では契約CSV（順位5）が優先するが、UX-05側が置換されていない。
- Why it matters: 実装Agentが2種類のprops名を見て迷い、IR72の「文書欠陥として報告」に当たる。
- Example Failure: StatusBadgeに`domain`を渡す実装と`severity`/`quality`を渡す実装が混在する。
- Required Fix: UX-05のprops記述をcomponent-contracts.csvへの参照に置き換えるか、名称を揃える。
- Suggested Revision: UX-05の「受け取る情報」列を「component-contracts.csvのprops（IR72順位5）」へ変更する。
- Applied Revision: UX-05の受け取る情報は概要で、propsの正はcomponent-contracts.csv（IR72の順位5）と明記。
- Contract: [IR102](../../../02-design/review-resolution-contracts.md#ir102-軽微な明確化--g1-010g1-012g1-014g1-024g1-026g1-029)
- Acceptance: AT-G120-016（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-017

- Issue ID: G1-017
- Severity: MINOR
- Document: 03-uiux/screen-catalog.csv、02-design/contractor.md（DD-P03）、02-design/deterministic-contracts.md（D10）、02-design/strict-review-contracts.md（SR11）
- Location: screen-catalog.csv:15（SCR-P03のurl_selection=tab,sort）／設計contractor.md:122（jobId必須）／deterministic-contracts.md:145／strict-review-contracts.md:80（DOC-0.19.0、commit 0505446の行番号）
- Problem: DD-P03は割当対象のjobIdを必須入力とするが、SCR-P03のURL許可キーにjobIdが無い。D10は「URLの選択ID・tabはcatalogに固定」、SR11は未知キーを除去するため、P02から対象案件を指定して遷移・Back復元することができない。
- Why it matters: AT-P03-Nの操作導線（受諾した案件の割当画面を開く）が定義されない。
- Example Failure: /partner/schedule?jobId=job-internal-aでjobIdが除去され、別の案件を選ぶ手間が発生する。
- Required Fix: url_selectionにjobIdを追加するか、URLに保持しない理由と選択方法を明記する。
- Suggested Revision: SCR-P03のurl_selectionを`tab,sort,jobId`とする。
- Applied Revision: SCR-P03のurl_selectionにjobIdを追加。
- Contract: [IR102](../../../02-design/review-resolution-contracts.md#ir102-軽微な明確化--g1-010g1-012g1-014g1-024g1-026g1-029)
- Acceptance: AT-G120-017（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-018

- Issue ID: G1-018
- Severity: MINOR
- Document: 02-design/deterministic-contracts.md（D06）、02-design/review-resolution-contracts.md（IR49/IR89）、04-agentic-sdlc/acceptance-review-019.csv
- Location: deterministic-contracts.md:93／review-resolution-contracts.md:345, 631／acceptance-review-019.csv:38（AT-REV19-037）（DOC-0.19.0、commit 0505446の行番号）
- Problem: D06の重複判定（既存start<newEnd AND newStart<existingEnd）は、延長・再割当で置き換える旧Assignmentを除外するかを定めていない。AT-REV19-037は同じ技術者の旧枠[00:00Z,02:00Z)と重なる新枠[01:30Z,04:00Z)の成功を期待する。
- Why it matters: 除外しない実装では延長がCONFLICTになる。
- Example Failure: jobs.assignが旧Assignmentとの重複でCONFLICTを返し、AT-REV19-037が失敗する。
- Required Fix: 重複判定から同じJobの置換対象Assignmentを除外することを明記する。
- Suggested Revision: D06に「同じjobIdの現在active Assignment（置換対象）は重複判定から除外する」を追加する。
- Applied Revision: D06の確定重複判定から、同じjobIdの置き換え対象のactive Assignmentを除外。
- Contract: [IR102](../../../02-design/review-resolution-contracts.md#ir102-軽微な明確化--g1-010g1-012g1-014g1-024g1-026g1-029)
- Acceptance: AT-G120-018（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-019

- Issue ID: G1-019
- Severity: MINOR
- Document: 02-design/review-resolution-contracts.md（IR91/IR92/IR31）、01-requirements/admin.md、01-requirements/client.md、04-agentic-sdlc/fixture-contract.json
- Location: review-resolution-contracts.md:681（IR92規則4「該当フィールドだけを…patch」）, :669（IR91規則10）, :195（IR31）／要件admin.md:276（AT-A10-B①）／要件client.md:311（AT-C12-B）／fixture-contract.json `acceptancePatches["AT-P01-N"]`（job-contractor-aをstatus=submittedだけ変更）（DOC-0.19.0、commit 0505446の行番号）
- Problem: 状態フィールドだけのpatchは、通常操作では到達しない組合せを作る。restriction-limited-aをstate=scheduledにしてもperUnit.applyState=applied、apply Command=acknowledged、unit-limited.observedRestrictionは残り、そこへのcancelの副作用（D03の解除評価、SR26の回復case）が定義されない。AT-P01-Nのjob-contractor-aはsubmittedなのに報告版が無く（reportRefs=[]）、IR31の寄与者集合も無い。
- Why it matters: 期待値として書かれていない副作用が実装ごとに異なり、同じfixtureを使う他の画面（P05等）で例外が出る。
- Example Failure: AT-A10-B①のcancel後、unit-limitedが観測上は制限中なのにeffectiveControlPolicy=unrestrictedとなり、別テストで不整合が表面化する。
- Required Fix: 状態patchで同時に揃える関連フィールドを規則化するか、これらのcaseを通常操作で作る。
- Suggested Revision: IR92規則4に「Restriction.stateのpatchではperUnit/Command/observedRestrictionを状態に整合する値（scheduled: perUnit=not_sent・Commandなし・observedRestriction=null等）へ同時に置き換える。submitted Jobには提出済み報告版を同時に作る」を追加する。
- Applied Revision: IR92規則4に、Restriction.state・報告版を伴うJob.status・Payment/Invoiceの状態はpatchせず通常操作で作ると追記。AT-A10-B・AT-C12-B・AT-P01-Nを操作手順に変更し、AT-P01-Nのfixtureからjob-contractor-aのstatus patchを削除。検証器が状態だけのpatchを拒否する。
- Contract: [IR97](../../../02-design/review-resolution-contracts.md#ir97-受入fixtureの不変条件と入力オブジェクト--g1-004g1-005g1-009g1-011g1-019g1-031)
- Acceptance: AT-G120-019（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-020

- Issue ID: G1-020
- Severity: MINOR
- Document: 01-requirements/admin.md（FR-A14）、02-design/service-contracts.ts、02-design/review-resolution-contracts.md（IR88）
- Location: 要件admin.md:362（BR-A14「係数には、地域・年度・単位・出典が必須」）, :369（AT-A14-E「係数の単位が合っていない」）／service-contracts.ts:112（EmissionFactorに単位欄なし）／review-resolution-contracts.md:617（factorUnitは固定表示）（DOC-0.19.0、commit 0505446の行番号）
- Problem: EmissionFactorは単位を持たずkgCO2ePerKWhに固定されているため、「係数の単位が合っていない」状態を作れず、BR-A14の「単位必須」も検証対象が無い。
- Why it matters: AT-A14-Eのsubcaseが実行できない。
- Example Failure: テストAgentが単位不一致のfixtureを作れない。
- Required Fix: subcaseを削除するか、係数に単位欄を追加する。
- Suggested Revision: AT-A14-Eから「係数の単位が合っていない」を削除し、BR-A14を「単位はkgCO₂e/kWh固定（IR88）」とする。
- Applied Revision: EmissionFactorの単位はkgCO₂e/kWh固定（IR88）とし、BR-A14・DD-A14の必須項目を地域・年度・出典に、AT-A14-E/Bから単位不一致を削除。
- Contract: [IR102](../../../02-design/review-resolution-contracts.md#ir102-軽微な明確化--g1-010g1-012g1-014g1-024g1-026g1-029)
- Acceptance: AT-G120-020（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-021

- Issue ID: G1-021
- Severity: MINOR
- Document: 01-requirements/contractor.md（FR-P05）、02-design/technician.md（DD-T04/T09）、02-design/service-contracts.ts
- Location: 要件contractor.md:161（AT-P05-B③）／設計technician.md:148, 289／service-contracts.ts:70（ReviewAvailabilityの理由）（DOC-0.19.0、commit 0505446の行番号）
- Problem: 「未点検あり理由なし」の報告は提出時にVALIDATIONとなるため、submitted状態にならず品質確認の対象にならない。demoSeedに報告の節も無い。ReviewAvailabilityにも「記録不備」の理由値が無い。
- Why it matters: 到達不能な前提と、存在しない無効化理由を期待している。
- Example Failure: テストAgentがAT-P05-B③の前提を作れない。
- Required Fix: subcaseを削除するか、提出後に不備が判明する別の条件へ置き換える。
- Suggested Revision: AT-P05-B③を「提出直前に理由なしnot_inspectedで提出→VALIDATION（T04-E③と同じ）」としてFR-T側へ移す。
- Applied Revision: AT-P05-B③を「理由の無いnot_inspectedを含む提出がVALIDATIONとなり、品質確認の対象に出ない」に修正。
- Contract: [IR100](../../../02-design/review-resolution-contracts.md#ir100-点検対象の部品集合と提出の検証--g1-008)、DEC-58（PROPOSED、可逆）
- Acceptance: AT-G120-021（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-022

- Issue ID: G1-022
- Severity: MINOR
- Document: 01-requirements/client.md（FR-C05）、02-design/service-contracts.ts
- Location: 要件client.md:152（BR-C05「一般的な利用の同意」）, :160（AT-C05-B①「一般同意のみ」）／service-contracts.ts:99（Consent.purposeは`location_automation`のみ）（DOC-0.19.0、commit 0505446の行番号）
- Problem: 「一般同意」に当たるデータ・操作が無い。
- Why it matters: AT-C05-B①の前提を規則どおり作れない。
- Example Failure: テストAgentが独自のpurpose値を作る。
- Required Fix: 一般同意をモデル化するか、表現を「位置同意なし」に改める。
- Suggested Revision: AT-C05-B①を「位置同意granted=false（一般利用は同意記録なし）」とする。
- Applied Revision: 位置情報の同意はpurpose=location_automationだけとし、BR-C05・DD-C05から一般同意を削除。AT-C05-B①を「位置同意granted=false」に修正。
- Contract: [IR102](../../../02-design/review-resolution-contracts.md#ir102-軽微な明確化--g1-010g1-012g1-014g1-024g1-026g1-029)
- Acceptance: AT-G120-022（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-023

- Issue ID: G1-023
- Severity: MINOR
- Document: 01-requirements/technician.md（FR-T12）、02-design/service-contracts.ts、02-design/strict-review-contracts.md（SR20）
- Location: 要件technician.md:309（AT-T12-E②「新しい通信断の後に古いheartbeat」）／service-contracts.ts:142（DemoTriggerのdevice kindはcommunication_lost/power_lost/tamper/restored）／strict-review-contracts.md:133（DOC-0.19.0、commit 0505446の行番号）
- Problem: heartbeatを投入するDemoTriggerが無く、「古いheartbeat」を何で再現するか（古いsequenceのrestored等）が書かれていない。
- Why it matters: 手順を一意に実行できない。
- Example Failure: テストAgentが存在しないeventTypeを送りVALIDATIONになる。
- Required Fix: 使用するDemoTriggerと値を明記する。
- Suggested Revision: 「communication_lost(sequence=5)の後にrestored(axis=connection、sequence=4)→CONFLICTまたは反映なし、offlineのまま」とする。
- Applied Revision: AT-T12-E②をcommunication_lost(sequence=5)の後のrestored(axis=connection、sequence=4)と定義。
- Contract: [IR102](../../../02-design/review-resolution-contracts.md#ir102-軽微な明確化--g1-010g1-012g1-014g1-024g1-026g1-029)
- Acceptance: AT-G120-023（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-024

- Issue ID: G1-024
- Severity: MINOR
- Document: 01-requirements/client.md、01-requirements/admin.md、01-requirements/technician.md、02-design/deterministic-contracts.md（D05/D13）、02-design/service-contracts.ts
- Location: 要件client.md:340（AT-C13-N: purpose未指定）, :246（AT-C09-N: 割当先・枠・時計進行が未指定）／要件admin.md:399（AT-A15-N: customerId/purpose/period/unitIds未指定）／要件technician.md:258（AT-T10-N: 試運転のstartAction未指定、先行Commandのack待ちが未記載）, :283（AT-T11-N: checkの成功イベント・sensorTypes未指定）／deterministic-contracts.md:81, 177／service-contracts.ts:217（DOC-0.19.0、commit 0505446の行番号）
- Problem: 期待値には影響しないが必須の入力・手順がWhenに無い。
- Why it matters: IR92が禁じる推測補完が必要になる。
- Example Failure: AT-T10-Nで先行set_modeのack前に試運転を開始するとD05によりCONFLICTになる。
- Required Fix: 必須入力と手順を明記する。
- Suggested Revision: 例: AT-T10-Nに「set_modeをacknowledgedにした後、startAction=set_power true・endAction=set_power false」、AT-C13-Nに「purpose=Demo offset、period/unitIdsはIR92規則7」を追記する。
- Applied Revision: AT-C13-N（purpose・period・unitIds）、AT-C09-N（割当枠・時計・提出入力）、AT-A15-N（offsets.simulateの各イベントの入力）、AT-T10-N（jobId・Commandの応答・startAction/endAction）、AT-T11-N（unitId・checkの成功イベント）の手順と入力を明記。
- Contract: [IR102](../../../02-design/review-resolution-contracts.md#ir102-軽微な明確化--g1-010g1-012g1-014g1-024g1-026g1-029)
- Acceptance: AT-G120-024（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-025

- Issue ID: G1-025
- Severity: MINOR
- Document: 01-requirements/common.md（AT-X01〜X07）、04-agentic-sdlc/fixture-contract.json
- Location: 01-requirements/common.md:88, 97, 104, 113, 120, 127, 134／fixture-contract.json demoSeed.capabilities（2件とも温度対応・同じmode候補）（DOC-0.19.0、commit 0505446の行番号）
- Problem: 共通ATは文章で観点を列挙するだけで、Given/When/Thenの具体値やacceptancePatchesのキーが無い。例えばAT-X06の「温度に対応していない」「モードの候補が異なる」「送風のみに対応」の機種はseedに無く、IR92規則6の一覧にも無い。
- Why it matters: 共通AT（7束）のfixtureをテストAgentが自作することになる。
- Example Failure: AT-X06の非対応機種の定義がテストごとに異なる。
- Required Fix: AT-X01〜X07をN/E/B形式の表にし、必要なpatchをacceptancePatchesへ追加する。
- Suggested Revision: `acceptancePatches["AT-X06-B"]`にtemperature=null、modes=[cool]、control=false等のcapabilityを追加する。
- Applied Revision: 共通要件に「共通受入条件の具体値」表（AT-X01〜X07のN/E/B、21行）を追加。必要なfixture（AT-X02-B、AT-X04-E.5、AT-X06-B.1〜.3・.5）を追加し、capabilitiesにtenantId、unit-tenant-bにcap-split-std-tbを設定。旧Context・Membership期間外・制限不可契約・同じpathLabelの扱いをDEC-59として定義。
- Contract: [IR101](../../../02-design/review-resolution-contracts.md#ir101-共通受入条件の具体値と機種台帳のtenant--g1-025)、DEC-59（PROPOSED、可逆）
- Acceptance: AT-G120-025（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-026

- Issue ID: G1-026
- Severity: MINOR
- Document: 04-agentic-sdlc/verification.md（S08）、02-design/strict-review-contracts.md（SR03/SR13）、04-agentic-sdlc/fixture-contract.json
- Location: verification.md:104／strict-review-contracts.md:24, 88／fixture-contract.json actors（tech-external-bのscopes=[unit-other-customer]）（DOC-0.19.0、commit 0505446の行番号）
- Problem: S08は業者aの辞退後に「別の業者」（contractor-b）へ再委託し自社技術者を割り当てるが、対象案件・設備が未指定で、IR92の既定（unit-online-rto）ではcontractor-bの技術者tech-external-bのscopeに設備が無い。jobs.assign/members.eligibleで技術者のunit scopeを検査するかも定義されていない。
- Why it matters: S08の前提が一意に作れない。
- Example Failure: 割当は成功するが、tech-external-bが設備を読めず作業開始できない。
- Required Fix: S08の案件・業者・技術者のIDを明記し、割当時のscope検査を定義する。
- Suggested Revision: S08の前提を「job: customer-bのunit-other-customerの案件、辞退: contractor-a、再委託: contractor-b、技術者: tech-external-b」とする。
- Applied Revision: jobs.assign/members.eligibleの候補条件にscopeのJob.unitId包含を追加（外れればFORBIDDEN errors.technician_out_of_scope）。S08をcustomer-bのunit-other-customer・contractor-b・tech-external-bで構成。
- Contract: [IR94](../../../02-design/review-resolution-contracts.md#ir94-認可列の修飾語と技術者の書込み条件--g1-001g1-026g1-030)、DEC-54（PROPOSED、可逆）
- Acceptance: AT-G120-026（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-027

- Issue ID: G1-027
- Severity: MINOR
- Document: 01-requirements/client.md、01-requirements/technician.md、01-requirements/admin.md、04-agentic-sdlc/fixture-contract.json、02-design/service-contracts.ts
- Location: 要件client.md:214（AT-C08-SRC）／要件technician.md:188（AT-T07-SRC）／要件admin.md:148（AT-A05-SRC）／fixture-contract.json demoSeed.alerts（window_openの1件のみ）／service-contracts.ts:142（inferred/inspectionのAlertを作るDemoTriggerなし）（DOC-0.19.0、commit 0505446の行番号）
- Problem: 「断熱不足の点検記録」「根拠なし」のfixtureにID・値・acceptancePatchesキーが無い。クリック可能なデモでも、insulation_loss/inspection根拠のAlertを作る経路が無い。
- Why it matters: SRC受入の前提値がテストごとに異なり、デモで原文要件（BIZ-17）を示せない。
- Example Failure: 2つのテストが異なるevidenceTextで同じcaseを合格とする。
- Required Fix: acceptancePatchesにSRC 3件を定義し、デモ用の生成経路を定める。
- Suggested Revision: `acceptancePatches["AT-C08-SRC"]`にalert-insulation-a（causeCode=insulation_loss、evidenceKind=inspection）とalert-unknown-a（causeCode=unknown、evidenceKind=demo_observation）を追加する。
- Applied Revision: shared:load-cause-alerts（alert-window-a、alert-insulation-a、alert-unknown-a）とAT-C08-SRC/AT-T07-SRC/AT-A05-SRCのキーを追加。DemoTriggerのload_alertでデモ中にも作れる。
- Contract: [IR98](../../../02-design/review-resolution-contracts.md#ir98-アレルゲン観測と原因候補alertのデモデータ--g1-006g1-027)、DEC-57（PROPOSED、可逆）
- Acceptance: AT-G120-027（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-028

- Issue ID: G1-028
- Severity: MINOR
- Document: 02-design/common.md §2、03-uiux/screen-catalog.csv、03-uiux/component-contracts.csv、01-requirements/common.md（FR-X01）
- Location: 02-design/common.md:58／component-contracts.csv:59（AppShellのevent: switchLocale;signOut;openNotifications;toggleVoice;extendSession）／screen-catalog.csv（demoSession.switchMembershipを持つScreenなし）／01-requirements/common.md:21（DOC-0.19.0、commit 0505446の行番号）
- Problem: 役割切替（FR-X01、P0）の専用メニューを担うComponent/Screenの契約にdemoSession.switchMembershipが無い。
- Why it matters: D13「Pageのread/write操作はscreen-catalogのoperationsを使い」により、呼び出し箇所を推測することになる。
- Example Failure: 役割切替を/loginへの再遷移で実装する案と、ヘッダーメニューで実装する案が並立する。
- Required Fix: AppShell/ShellContainerの契約にswitchMembershipを追加する。
- Suggested Revision: AppShellのeventに`switchMembership(demoMembershipId)`、api_dependencyに「ShellContainerがdemoSession.switchMembershipを実行」を追加する。
- Applied Revision: AppShellのeventにswitchMembership(demoMembershipId)を追加し、ShellContainerがdemoSession.switchMembershipを実行すると定義。
- Contract: [IR102](../../../02-design/review-resolution-contracts.md#ir102-軽微な明確化--g1-010g1-012g1-014g1-024g1-026g1-029)
- Acceptance: AT-G120-028（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

### G1-029

- Issue ID: G1-029
- Severity: MINOR
- Document: 01-requirements/admin.md（FR-A09）、02-design/review-resolution-contracts.md（IR05）
- Location: 要件admin.md:250（AT-A09-E③「未通知」）／review-resolution-contracts.md:36（DOC-0.19.0、commit 0505446の行番号）
- Problem: IR05ではscheduleの同一遷移で予告通知を必ず作り、宛先0ならscheduleがVALIDATIONとなるため、「未通知」の制限は存在しない。
- Why it matters: subcaseの前提に到達できない。
- Example Failure: テストAgentが未通知の制限を作れない。
- Required Fix: 意図する条件（予告後24時間未経過等）に書き換える。
- Suggested Revision: AT-A09-E③を「予告後24時間未満でexecute→D01順位6のCONFLICT」とし、B①との重複を整理する。
- Applied Revision: AT-A09-E③を「予告の宛先となる顧客Membershipが0件でschedule→VALIDATION（IR05）、Restriction 0件」に修正。
- Contract: [IR102](../../../02-design/review-resolution-contracts.md#ir102-軽微な明確化--g1-010g1-012g1-014g1-024g1-026g1-029)
- Acceptance: AT-G120-029（[受入計画](../../acceptance-review-020.csv)）
- Status: resolved

## 6. Open Questions

### G1-030

- Issue ID: G1-030
- Severity: QUESTION
- Document: 01-requirements/contractor.md（FR-P06）、02-design/query-catalog.csv、02-design/contractor.md（DD-P06）
- Location: 要件contractor.md:184（AT-P06-N「技術者2名」）／query-catalog.csv members.capacity/members.list（filters: organizationId,qualification,activeOnly）／設計contractor.md:206-221／fixture-contract.json actors（contractor-aはorg-contractor-aのrole=contractor）（DOC-0.19.0、commit 0505446の行番号）
- Problem: members.list/members.capacityが組織内のrole=contractorのMembership（contractor-a自身）を含むかが書かれていない。AT-P06-Nは技術者2名だけを期待する。
- Why it matters: 件数の期待値が変わる。
- Example Failure: 実装がcontractor-aを含めて3名を返す。
- Required Fix: 返却対象をrole=technicianに限定するかを明記する。
- Suggested Revision: IR42のcontractor向け投影に「members.list/eligible/capacityはrole=technicianのMembershipだけ」を追記する。
- Applied Revision: 回答: members.eligible/members.capacityとcontractorのmembers.listはrole=technicianのMembershipだけを返す。contractor-a自身は含まない（HQのmembers.listは全role）。
- Contract: [IR94](../../../02-design/review-resolution-contracts.md#ir94-認可列の修飾語と技術者の書込み条件--g1-001g1-026g1-030)、DEC-54（PROPOSED、可逆）
- Acceptance: AT-G120-030（[受入計画](../../acceptance-review-020.csv)）
- Status: answered

### G1-031

- Issue ID: G1-031
- Severity: QUESTION
- Document: 02-design/review-resolution-contracts.md（IR45/IR85）、01-requirements/technician.md、01-requirements/client.md、04-agentic-sdlc/verification.md
- Location: review-resolution-contracts.md:308, 595／要件technician.md:119-121（AT-T03-N/E/B）／要件client.md:120, 202-204（AT-C03-N、AT-C07-N/B）／verification.md:40（DOC-0.19.0、commit 0505446の行番号）
- Problem: IR45は「Givenで測定値・観測時刻を固定するcase」ではsimulator=falseを与えるとし、IR85はKPI・件数のATだけを明示する。acceptancePatchesを持たないAT（例: AT-T03-Bの120秒/120秒+1ms、AT-T03-Nのsequence=4投入、AT-C07-BのCO₂/湿度値）がこれに該当するかをcaseごとに示した一覧が無い。
- Why it matters: 時計を分境界の先へ進めるcaseでは、シミュレーターの複写によりstale判定やsequenceの期待値が変わる。
- Example Failure: AT-T03-B①②で01:01:00Zの複写が作られ、120秒+1msでもvalidのままになる。
- Required Fix: simulatorの状態をcaseごとに明記するか、「acceptancePatchesを持たないATも、測定値・時刻・sequenceを観測するものはsimulator=false」と一般規則化する。
- Suggested Revision: IR92に規則9「測定値・観測時刻・stale・sequenceを観測するATはsimulator=falseで開始し、自動生成を検証するcase（AT-REV18-001、AT-REV19-003/009）だけenabled=true」を追加する。
- Applied Revision: 回答: 受入試験は既定でsimulator=falseで開始し、自動生成を検証するAT-REV18-001、AT-REV19-003、AT-REV19-009だけsimulator=true。
- Contract: [IR97](../../../02-design/review-resolution-contracts.md#ir97-受入fixtureの不変条件と入力オブジェクト--g1-004g1-005g1-009g1-011g1-019g1-031)、DEC-58（PROPOSED、可逆）
- Acceptance: AT-G120-031（[受入計画](../../acceptance-review-020.csv)）
- Status: answered

未回答の質問は無い。

## 7. Cross-document Inconsistencies

### A. Requirement Missing

- AT-X01〜X07の具体値（G1-025）→ 共通要件に表を追加。
- 業務イベント通知の要件（G1-002）→ IR95。受入はAT-P03-N④とAT-G120-002。

### B. Design Without Requirement

- 旧DD-C05・BR-C05の「一般的な利用の同意」（G1-022）→ 削除し、位置情報の同意だけに統一。
- 旧BR-A14の係数の単位（G1-020）→ 単位固定のため必須項目から削除。

### C. UI Without Requirement

- 該当なし（本修正で新しく見つかったものは無い）。

### D. UI Without API

- SCR-C08のalertCount（G1-014）→ summaries.getを画面・Component・DD-C08・カタログに追加。
- 役割切替（G1-028）→ AppShellのswitchMembershipイベントとShellContainerの実行経路を定義。
- SCR-P03のjobId（G1-017）→ url_selectionに追加。

### E. Data Model Gap

- releaseIntent（G1-013）、Notification.templateKeyのjob_update/device_operation（G1-002）、DemoTriggerのallergen/load_alert（G1-006・027）、demoSeed.allergenObservations（G1-006）、capabilitiesのtenantId（G1-025）を追加。

### F. Terminology Conflict

- 「通知」「通知プレビュー」（G1-009）→ 保存されるinApp Notification（simulated）と、保存しないnotifications.previewを区別（IR97の6）。
- 「offline台はpending」（G1-015）→ releaseState=requested/failedの正規値へ置換。

## 8. Missing Requirements

修正で追加した要件・規則: IR94（技術者の書込み表、候補のscope条件、名簿のrole）、IR95（業務イベント通知）、IR96（制限の取消）、IR97（受入fixtureの不変条件）、IR98（アレルゲン観測・原因候補Alert）、IR99（空気環境の案内）、IR100（点検部品と提出検証）、IR101（共通受入の具体値、DEC-59）。本報告の時点で、新たに見つかった未記載の要件は無い。

## 9. Edge Cases Not Defined

本修正で定義したもの: 担当の無い社内技術者の書込み、要求時offlineのFW更新、scope外技術者の割当、release_requested中の取消の再送、released後の取消、co2=1000 ppm・pm25=35 µg/m³の境界、CO₂とPM2.5がともに欠測、単位の無いアレルゲン値、部品の不足・余分な提出、旧scopeVersionのContext、Membership有効期間外、restrictionEligible=falseの契約、同じpathLabelの候補、作業窓終了時刻ちょうどの保存、通信断中の解除Commandの30秒境界。未定義として残す1A範囲外の事項は§11のとおり。

## 10. Traceability Matrix

全64要件の判定は[traceability-matrix.csv](traceability-matrix.csv)（列: Requirement ID | Requirement | Prepare | Detailed Design | UI/UX | API | Error Handling | Testable | 修正前 | 修正後 | G1 ID）。修正後はOK 64、INCOMPLETE 0、MISSING 0、CONFLICT 0。

## 11. Undefined Decisions

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| DEC-54 | 技術者の書込み条件と候補・名簿の範囲（提案採用、可逆） | IR94 | 1Aの認可を一意にするため採用。企業の業務ルールとしての確認が必要 | Security / Product Owner |
| DEC-55 | 業務イベント通知の宛先と種類（提案採用、可逆） | IR95 | 通知件数とテンプレートを一意にするため | Product Owner / UI/UX |
| DEC-56 | 制限の取消の状態表（提案採用、可逆） | IR96 | 取消後の機器状態の扱いを一意にするため | Business / Security |
| DEC-57 | アレルゲン観測の取得元と空気環境のデモ閾値（提案採用、可逆） | IR98・IR99 | 表示条件を一意にするため。健康基準ではない | Product Owner / IoT |
| DEC-58 | 点検部品の集合・提出検証・受入のsimulator既定（提案採用、可逆） | IR97・IR100 | 提出成功の条件と試験の再現性のため | Product Owner / QA |
| DEC-59 | 旧Context・Membership期間外・制限不可契約・同名候補の扱い（提案採用、可逆） | IR101 | 共通受入の期待値を一意にするため | Security / UI/UX |
| OPEN-03〜06/11 | 本番API・認証・DB・IoT接続（既存） | PrepareDocument §10、D11 | 1B着手前に必要。1Aの阻害項目には数えない | Backend / IoT / Security |

## 12. Implementation Readiness

| Aspect | Judgment |
|---|---|
| Requirements | CONDITIONALLY READY（独立G1の再判定待ち） |
| Detailed Design | CONDITIONALLY READY（独立G1の再判定待ち） |
| UI/UX | CONDITIONALLY READY（独立G1の再判定待ち） |
| API（1Aのローカルサービス契約） | CONDITIONALLY READY（独立G1の再判定待ち） |
| Error Handling | CONDITIONALLY READY（独立G1の再判定待ち） |
| Security（1Aの表示・操作制御） | CONDITIONALLY READY（独立G1の再判定待ち、DEC-54/59は企業確認前） |
| IoT（1Aのデモ事象） | CONDITIONALLY READY（独立G1の再判定待ち） |
| Test / Acceptance | CONDITIONALLY READY（独立G1の再判定待ち、アプリ試験not_run） |
| Agentic SDLC handoff | CONDITIONALLY READY（baseline・両検証器成功、gate-G1 pending） |
| Production connection | NOT READY（D11、1A範囲外） |

## 13. Required Actions Before Implementation

1. 別エージェントによるDOC-0.20.0の独立G1を実施し、BLOCKER・CRITICAL・MAJORが0件であることを確認する（self-reviewはG1の代わりにしない）。
2. G1で指摘があれば修正し、新しいbaselineで再判定する。
3. 企業検収前に、DEC-42〜59のPROPOSEDを担当者（Product Owner／Business／Security／UI/UX／IoT／QA）が確認する。
4. 1B（本番接続）へ進む前に、D11の未決事項を決める。
