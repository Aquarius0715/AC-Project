# 独立G1レビュー報告 DOC-0.19.0（2026-09-17）

- レビュー主体: 独立G1レビュー担当AI（別エージェント。DOC-0.19.0の修正担当・自己再レビュー担当とは別の実行）
- 対象: リポジトリ `/Users/ji1wxs/PraditaProjects/AC_Project`、ブランチ `docs/0.19.0-independent-review`、コミット `0505446`
- spec_baseline_id: `048ffcef25f8463bca33346f5bb1dd8dbb1fdad03b61132d1cfc5d17ebeb1ac2`（[spec-manifest](../spec-manifest.json)、61ファイル）
- 範囲: 1A（クリック可能なフロントエンドのモックデモ）。API/HTTP・DB・サーバー・実機・実決済はDEC-12/D11により本番未決事項として扱い、本判定の阻害項目に数えていない。
- 判定根拠は現行baselineの文書本文だけとし、修正担当の自己レビュー記録（../review.md）は判定後にヘッダーだけ参照した。過去版（runs/DOC-0.17.0以前、05-document-review）の指摘は参照・再掲していない。
- アプリは未実装であり、アプリ試験は実行していない（not_run）。

## 1. 判定サマリー

**G1判定: FAIL**

| Severity | 件数 |
|---|---|
| BLOCKER | 0 |
| CRITICAL | 0 |
| MAJOR | 12 |
| MINOR | 17 |
| QUESTION | 2 |

両検証器（`validate_documents.py`、`check_review_regressions.py`）は成功し、manifestのSHA-256とbaseline IDも独立に再計算して一致した。カタログ間の機械的な整合（正規型137操作と操作カタログ、write 76操作と版カタログ、Query入力を持つread 35操作とQueryカタログ、画面カタログの操作参照、IR71の無効化表の操作名）にも不一致はなかった。

一方で、文書本文の突き合わせにより、実装Agent・テストAgentが推測なしに進められない欠陥をMAJORとして12件検出した。要点は次のとおり。

1. **G1-001** 社内技術者が案件（jobId）を持たずに行う書込み（alerts.acknowledge、devices.addResponseNote/calibrate/updateFirmware等）の認可が、操作カタログの`assigned-valid-job`・SR03と、AT-T07-N/AT-T12-N/AT-T11-E③/AT-REV19-021で食い違う。カタログ認可列の修飾語（assigned、assigned-valid-job、job-required等）の定義も無い。
2. **G1-002** 業務イベント（案件の依頼・委託・受諾・割当、報告の提出・差戻し・完了、入金確認、機器障害等）で生成する通知のtemplateKey/type/channel/deliveryState/宛先Membershipが未定義で、templateKeyのenumにも対応値が無い。AT-P03-N④が依存する。
3. **G1-003** requested/appliedの制限に対する`restrictions.cancel`の結果が、IR35（解除要求の起動経路は3つだけ）と、DD-A10・common.md §5・AT-A10-B②/E④（取消で解除要求へ進む）で矛盾する。
4. **G1-004** AT-C06-E.3（およびAT-A13-E/B、AT-REV18-024、AT-REV19-041）のfixtureが120 kWのpower測定をquality=validで置いており、D07の合成値範囲power/kW:[0,100]とIR12の範囲外→suspect規則に反する。
5. **G1-005** AT-A12-N/Bの「換気非対応設備には通知のみ」は、seedのunit-non-rto（device-tamper）にCO₂センサーが無いため、IR21/SR25の規則上成立しない。
6. **G1-006** allergenObservation（BIZ-18）の取得元データ・導出規則・fixture投入経路が無く、AT-C07-SRC/AT-A12-SRCを作れない。
7. **G1-007** 顧客の空気環境画面の「換気を推奨」「清掃の案内」を出す条件が未定義で、AT-C07-N②を判定できない。
8. **G1-008** 報告提出時に必須となる点検項目集合（UnitDetail.componentsの導出）が未定義で、AT-T04/T05/T06/T08/T09-Nの「提出成功」の前提入力（他グループの点検結果、workText、nextAction）が欠けている。
9. **G1-009** AT-A05-Nの保存入力に必須項目（name、recoveryThreshold、cooldownMinutes、escalateAfterMinutes）とenabledが無く、IR07の新規既定enabled=falseでは期待結果（Alert/通知の生成）に到達しない。
10. **G1-010** AT-C04-E③とAT-FIX-019が時計を2026-03-07／2026-10-31に置くが、seedの全actorのMembership有効期間は[2026-09-01, 2026-10-01)で、期待値VALIDATIONが一意に決まらない。
11. **G1-011** AT-REV19-015はAssignment.scheduledEndだけをpatchするが、IR89の「作業窓終了・再割当が必要」表示はJob.scheduledSlot.endAtで判定するため、期待結果が出ない。
12. **G1-012** verification.mdが実施を求める旧受入計画（AT-REV17-004/005/014、AT-REV18-003）の期待値が、現行のIR57/IR74/D05/IR47とseedに矛盾する。

## 2. 検証器の実行結果

リポジトリ直下で実行した。どちらも終了コード0。

### 2.1 `python3 docs/tools/validate_documents.py`

- 終了コード: 0
- `errors`: `[]`
- `baseline`: `048ffcef25f8463bca33346f5bb1dd8dbb1fdad03b61132d1cfc5d17ebeb1ac2`、`spec_files`: 61
- 件数: requirements 64、acceptance_bundles 182、role_details 49、operations 137、screens 48、components 69、query_contracts 35、write_version_branches 94、review_019_cases 42、proposed_decisions_019 10、pending_business_decisions 0
- `application_tests`: not_run、`typescript_semantic_check`: not_run（検証器自身はTypeScript検査を実行しない）

### 2.2 `python3 docs/tools/check_review_regressions.py`

- 終了コード: 0
- 変異ケース 62件すべてで `detected: true`、`passed: true`

### 2.3 本レビューで追加した独立確認

- manifestの61ファイルのSHA-256を再計算し全件一致。manifest記載の正規化規則で`spec_files`をJSON化したSHA-256は`048ffcef…1ac2`で一致。manifest外のdocsファイルは`.gitkeep`だけ（runs・05-document-reviewを除く）。
- `service-contracts.ts`をscratchpadへ複製し、TypeScript 5.8.3の`tsc --strict --noEmit --target es2022 --lib es2022,dom`で検査し終了コード0（仕様ファイルは変更していない）。
- 観察（仕様の指摘には数えない）: `runs/DOC-0.19.0/static-check.json`は`typescript_semantic_check: "passed"`と記録しているが、`validate_documents.py`の実際の出力は`not_run`である。自己レビュー側で別途tscを実行した結果を手で記入したと推測されるが、証跡ファイルと検証器出力の出所が一致していない。
- 検証器の実行前後で`git status`は変化なし。

## 3. 指摘一覧

各指摘の引用行番号はコミット`0505446`のファイルに対するもの。`01-requirements/*.md`を「要件」、`02-design/<role>.md`を「設計」と略記する場合がある。

### 3.1 MAJOR

#### G1-001

- Severity: MAJOR
- Issue ID: G1-001
- Document: 02-design/operation-catalog.csv、02-design/strict-review-contracts.md（SR03）、02-design/review-resolution-contracts.md（IR49/IR67）、02-design/implementation-contracts.md（DDC-07/DDC-08）、01-requirements/technician.md、04-agentic-sdlc/acceptance-review-019.csv
- Location: operation-catalog.csv:3（alerts.acknowledge）、:20（commands.create）、:35（devices.addResponseNote）、:37（devices.calibrate）、:45（devices.updateFirmware）／strict-review-contracts.md:24／review-resolution-contracts.md:343, 345, 430／implementation-contracts.md:240, 288／deterministic-contracts.md:21, 25, 83／technician.md:201（AT-T07-N）, :284（AT-T11-E③）, :308（AT-T12-N）／acceptance-review-019.csv:22（AT-REV19-021）
- Problem: 操作カタログの認可列は、技術者について`technician:alert.resolve:assigned`（alerts.acknowledge/resolve）、`technician:device.maintain:assigned-valid-job`（devices.addResponseNote/bind/calibrate/check/updateFirmware/register）、`technician:control.diagnose:job-required`（commands.create）を定めるが、`assigned`／`assigned-valid-job`／`job-required`等の修飾語の定義はカタログ以外のどの文書にも無い（全文検索で該当なし）。SR03は「内部技術者の書込みにも必要なAssignment条件を適用する」とし、DDC-08はaddResponseNoteに「有効な担当であること」を要求する。一方IR49は「社内技術者のunit scopeによる設備読取はSR03どおりだが、案件に紐づく書込みは作業窓内でなければFORBIDDEN」と、jobIdを伴う書込みだけを明示する。これに対し、AT-T12-N（tech-internal-aがdevice-tamperにresponseNote）、AT-T07-N（tech-internal-aがalert-temp-aをacknowledge。acceptancePatchesにJob/Assignmentなし）、AT-REV19-021（tech-internal-aがdevice-online-rtoをcalibrate）は、担当案件の無い社内技術者の書込み成功を期待する。AT-T11-E③（tech-internal-aがdevice-offline-rtoのFW更新、unit-offline-rtoの担当案件なし）は`OFFLINE`を期待するが、案件必須ならD01順位4のFORBIDDENが先になり、案件不要でもIR67ではupdateFirmwareはqueuedで作成され1秒後に`failed/failureCode=OFFLINE`になるため、DomainError OFFLINEか操作記録のfailureCodeかも決まらない。
- Why it matters: Repository側の認可行列（Role別アクセス範囲）が決まらない。IR72の順位ではSR03/カタログがATより上位だが、`assigned`の意味自体が未定義のため上位規範でも解消できない。P0のFR-T07を含む技術者フローの受入が実装方針によって合否が反転する。
- Example Failure: 実装Aはカタログの`assigned-valid-job`に従い、案件の無いtech-internal-aのaddResponseNoteをFORBIDDENにしてAT-T12-N②が失敗する。実装Bはunit scopeだけで許可し、AT-T12-Nは通るが、カタログ契約テスト（案件必須）とSR03に反する。AT-T11-E③はAではFORBIDDEN、BではwriteがDeviceOperationを返して1秒後にfailed/OFFLINEとなり、どちらも「OFFLINE」という期待値と照合方法が一致しない。
- Required Fix: 操作カタログ認可列の修飾語を凡例として定義する。技術者の書込みについて「社内/外部 × jobIdあり/なし × 作業窓内/外」の可否表を1か所に置き、alerts.acknowledge/resolve、devices.*、commands.create、diagnosticRuns.createに適用する。AT-T07-N/T12-N/T11-E③/AT-REV19-021をその表に合わせ、必要ならacceptancePatchesにJob/Assignmentを追加する。AT-T11-E③の観測対象（DomainErrorかDeviceOperation.failureCodeか）を明記する。
- Suggested Revision: IR94として「`client:self`=自己customer組織scope、`contractor:accepted-valid-offer`=IR23のsummary投影期間、`technician:assigned`=社内はMembership.scopes内、外部は自己Assignmentの閲覧窓、`technician:*:assigned-valid-job`=入力jobIdが有効Assignmentの作業窓内（社内技術者がjobIdを省略した場合はunit scope内で許可／不可のいずれかを明記）」を定め、AT-T11-E③のThenを「DeviceOperation.status=failed、failureCode=OFFLINE、firmwareVersion不変（IR67）」へ変更する。

#### G1-002

- Severity: MAJOR
- Issue ID: G1-002
- Document: 02-design/implementation-contracts.md（通知と公開範囲）、02-design/service-contracts.ts、02-design/deterministic-contracts.md（D08/D12）、02-design/review-resolution-contracts.md（IR10）、01-requirements/contractor.md
- Location: implementation-contracts.md:190-205／service-contracts.ts:108（Notification.templateKey）、:166（NotificationType）／review-resolution-contracts.md:64（IR10）、:32（IR04）、:36（IR05）、:337（IR48）／deterministic-contracts.md:121-129, 169／strict-review-contracts.md:183（SR28）／contractor.md:115（AT-P03-N④）／fixture-contract.json actors（admin 3件）
- Problem: 通知が生成される条件と内容が定義されているのは、Policy由来のAlert（SR21/SR28）、制限予告（IR05）、督促（IR04）だけである。implementation-contracts.mdの「通知と公開範囲」表はjob.requested/offered/accepted/declined/assigned/schedule_changed、report.submitted/returned、job.completed、payment.confirmed、device.fault/operation_failed、inquiry.received/answeredで「顧客・HQ・担当技術者…に伝えます」と宛先の種類を述べるだけで、templateKey、Notification.type、channel、deliveryState（preview/simulated）、宛先となるMembershipの選び方（例:「HQ」はfixtureのadmin 3件のうちどれか）を定めない。さらにtemplateKeyのenum（alert/quality/schedule_change/report_return/completion/payment/payment_reminder/restriction/inquiry）には依頼・委託・受諾・割当・機器障害に対応する値が無く、表どおりの通知を正規型で表現できない。D08の通知リンク表はjob/device/inquiry宛の通知を前提にしている。
- Why it matters: FR-X07（P0）の通知の仕組みのうち業務イベント由来の部分が「UIはあるがデータ定義が無い」状態で、実装Agentが件数・宛先・文面キーを推測することになる。受入の期待件数も決まらない。
- Example Failure: AT-P03-N④「担当技術者へ通知プレビュー1件」について、実装Aはschedule_change/inApp/simulatedのNotificationを顧客・admin 3件・技術者・業者に計6件保存し、実装Bは技術者宛のpreviewだけを返す。テストAgentはどちらが正しいか判定できない。
- Required Fix: 業務イベントごとに、生成の有無、templateKey、type、channel、deliveryState、target、宛先Membershipの選定規則（permission・scope条件）、件数を表で定める。enumに不足する値を追加するか、既存値への写像を明記する。AT-P03-N④をその表に合わせる。
- Suggested Revision: 「job.assigned: templateKey=schedule_change、type=schedule_change、channel=inApp、deliveryState=simulated、target={kind:job}、宛先=当該Assignmentの技術者Membership 1件＋顧客のclient Membership（全対象Unit閲覧可）＋job.manageを持つadmin」のような行をイベントごとに追加し、生成しないイベント（例: IR48の期限到来）も列挙する。

#### G1-003

- Severity: MAJOR
- Issue ID: G1-003
- Document: 02-design/review-resolution-contracts.md（IR35）、02-design/admin.md（DD-A10）、02-design/common.md §5、01-requirements/admin.md（FR-A10）、02-design/service-contracts.ts
- Location: review-resolution-contracts.md:236, 238, 240／設計admin.md:55, 395／02-design/common.md:181, 185／要件admin.md:268（BR-A10）, :275（AT-A10-E④）, :276（AT-A10-B②）／strict-review-contracts.md:127（SR19）／service-contracts.ts:84, 281
- Problem: IR35は「解除要求（state=release_requested）の起動経路は次の3つだけ」（入金確認、defer/exempt、override）とし、別途`restrictions.release`（source='manual'）を定める。`restrictions.cancel`は含まれない。一方、DD-A10は「適用の要求を出したあとの取消は、反映されたかどうかわからなくても解除の流れに進めます」、common.md §5本文は「requested以降に取り消したときは…解除の流れに進めます」とし、AT-A10-B②は「requestedの状態でcancelする→release_requestedになる」、AT-A10-E④は「requested中にcancelする→解除の流れに進み」を期待する。同じcommon.md §5の状態表（:181）のrequested/applied→release_requestedの起動条件にはcancelが無い。cancelによる遷移のreleaseIntent.source、D03の設備別解除評価の実行有無、applied/release_requestedでのcancelの結果（CONFLICTか）はどこにも無い。SR19は「予告はcancel、適用済み/結果不明はreconcile/release」とcancelを予告段階に限る読み方をしている。
- Why it matters: IR72の順位ではIR35（順位2）がDD（順位7）・AT（順位8）に優先し、requestedへのcancelはrelease_requestedを起こさないことになるが、その場合の結果が未定義である。制限の解除はFR-A09/A10（P0）の中核で、Command生成の有無に直結する。
- Example Failure: 実装AはIR35に従いrequestedへのcancelをCONFLICTにしてAT-A10-B②が失敗する。実装BはDD-A10に従いrelease_requestedへ遷移させ、remove Commandを作るが、releaseIntent.sourceに定義外の値を入れる。
- Required Fix: `restrictions.cancel`の状態別の遷移表（scheduled/requested/applied/release_requested/released/cancelled）を定め、IR35の起動経路一覧にcancelを加えるか、requested/appliedでのcancelを拒否してreleaseへ誘導するかを一意にする。DD-A10、common.md §5、FR-A10、AT-A10-B/Eを同じ内容に揃える。
- Suggested Revision: IR35に「④取消: restrictions.cancelがrequested/appliedに対して行われた同一遷移でrelease_requestedへ遷移し、releaseIntent.source='cancel'。scheduledはcancelled、release_requestedは冪等に現在状態、released/cancelledはCONFLICT」を追加する。

#### G1-004

- Severity: MAJOR
- Issue ID: G1-004
- Document: 04-agentic-sdlc/fixture-contract.json、02-design/deterministic-contracts.md（D07）、02-design/review-resolution-contracts.md（IR12/IR69）、01-requirements/client.md、01-requirements/admin.md、acceptance-review-018.csv、acceptance-review-019.csv、acceptance-fixes.csv
- Location: fixture-contract.json:1816-1879（`acceptancePatches["AT-C06-E.3"]`、series value=120、expected kWh=120.0）／deterministic-contracts.md:101／review-resolution-contracts.md:74, 438／client.md:178（AT-C06-E .3）／要件admin.md:344-345（AT-A13-E/B）／acceptance-review-018.csv:25（AT-REV18-024）／acceptance-review-019.csv:42（AT-REV19-041）／acceptance-fixes.csv:18（AT-FIX-017）
- Problem: D07は「1A合成値の範囲は…power/kW:[0,100]」「範囲外…はIR12のRawMeasurement正規化でsuspectとして値を集計・制御から除く」と定める。AT-C06-E.3のpatchは1時間窓・1台で「実績120 kWh」を作るため、各分のpower値が120 kWとなり範囲外である。それにもかかわらずquality=valid・qualityReason=nullで置き、期待値kWh=120.0、savingPercentage=-20とする。AT-FIX-017は範囲外値を「suspectで除外」と集計除外まで期待している。patch経由の行に範囲検査を適用するか（IR69「正規DTO schemaで検証して不正なら例外」）、集計時に再判定するかは定義されていない。
- Why it matters: 負の削減量表示（IR68/IR80）を検証する受入の前提データが、上位規範の不変条件に反する状態でしか作れない。/demoからの通常投入（demo.trigger telemetry）ではこの状態を再現できない。
- Example Failure: 実装Aは正規DTO検証にD07の範囲を含めてfixture生成時に例外となりAT-C06-E.3が実行不能。実装Bは集計時に範囲外を除外してkWh=null・coverage=0を返しAT-REV19-041が失敗。実装Cは保存済みqualityだけを信じて120.0を返し、AT-FIX-017の「集計から除外」と整合しない。
- Required Fix: E.3の前提をD07の範囲内で作るか、D07の範囲を変更する。どちらの場合もfixture・AT本文・AT-REV19-041の期待値を同時に更新し、patch行に範囲検査を適用するかを明記する。
- Suggested Revision: 案1: D07のpower範囲を[0,200] kWへ変更する。案2: E.3を`unitIds=[unit-online-rto, unit-limited]`・各60 kW・基準100 kWh（同じ2台）に変更し、DTO savedKWh=-20、savingPercentage=-20を維持する。あわせてIR69に「patchで作る測定もD07/IR12の正規化規則を満たすこと（違反はfixture欠陥）」を追記する。

#### G1-005

- Severity: MAJOR
- Issue ID: G1-005
- Document: 01-requirements/admin.md（FR-A12）、04-agentic-sdlc/fixture-contract.json、02-design/review-resolution-contracts.md（IR21/IR43/IR92）、02-design/strict-review-contracts.md（SR25）、02-design/admin.md（DD-A12）
- Location: 要件admin.md:324（AT-A12-N）, :326（AT-A12-B）／fixture-contract.json:1006-1045（device-tamperのsensorsはtemperature/humidity/powerのみ）, :1158-1162（unit-online-rtoのCO₂=800）／review-resolution-contracts.md:131（IR21「Sensor不在はmissing_data」）, :282（IR43「sensor不存在=NOT_FOUND」）, :679（IR92規則2）／strict-review-contracts.md:159（SR25「通知の成立条件は…指標の有効品質・閾値/継続時間」）／設計admin.md:452（DD-A12「unitIds/metric…対応するセンサーがあること」）
- Problem: AT-A12-Nは「換気対応のunit-online-rto・非対応のunit-non-rto」を対象にmetric=co2・threshold=1000ppmで保存・評価し、「対応していない設備には通知のみ」を期待する（AT-A12-Bも同じ）。しかしunit-non-rtoの現bindingであるdevice-tamperにはCO₂センサーが無く、IR21によりそのUnitのco2 Factはmissing_data、IR43によりdemo.trigger telemetryもNOT_FOUNDとなるため、SR25の通知成立条件（有効品質）を満たせない。DD-A12は保存時点で「対応するセンサーがあること」を要求しており、保存自体がVALIDATIONになる可能性もある。さらにAT-A12-Nは評価に使うCO₂値・観測時刻・継続時間（durationSeconds）・recoveryThresholdを指定しておらず、seedのunit-online-rtoのCO₂は800ppmで閾値未満である。
- Why it matters: FR-A12の「換気対応設備は換気要求、非対応設備は通知のみ」という要件の中心的な分岐を、現行seedでは受入として作れない。IR92は「規則で決まらない前提は文書欠陥」としている。
- Example Failure: テストAgentがIR92の既定どおりunit-non-rtoを使うと、通知はsuppressed/qualityとなり「通知のみ」の期待が失敗する。保存時にDD-A12の検査でVALIDATIONになる実装もある。
- Required Fix: CO₂センサーを持ち換気能力を持たない設備を受入前提として用意し（acceptancePatchesでdevice-tamperにsensor-tamper-co2を追加するか、別Unitを追加）、評価入力（facts/telemetryの値・時刻・継続秒数）と期待するFireResult/NotificationOutcomeを明記する。DD-A12の「対応するセンサー」がCapability.sensorsか現binding Deviceのsensorsかも明記する。
- Suggested Revision: `acceptancePatches["AT-A12-N"]`を追加し、device-tamperのsensorsにco2（ppm、stale 120秒）を加え、unit-online-rto・unit-non-rtoにCO₂=1100ppmのmeasured/valid測定を00:59:00〜01:00:00に置く。Givenにfixture.airPolicyNotificationInput・recoveryThreshold=900・durationSeconds=60・enabled=true・recipient=hq-operatorを明記し、Thenを「unit-online-rto: results=requested（ventilate low）・notifications=created、unit-non-rto: results=suppressed/invalid_capability・notifications=created」とする。

#### G1-006

- Severity: MAJOR
- Issue ID: G1-006
- Document: 02-design/service-contracts.ts、02-design/deterministic-contracts.md（D12）、02-design/client.md（DD-C07）、02-design/admin.md（DD-A12）、01-requirements/client.md、01-requirements/admin.md、04-agentic-sdlc/fixture-contract.json、02-design/review-resolution-contracts.md（IR69/IR92）
- Location: service-contracts.ts:316-317（AllergenObservation、AirSeries）、:142（DemoTrigger）／deterministic-contracts.md:173／設計client.md:252／設計admin.md:434-440／要件client.md:189（AT-C07-SRC）／要件admin.md:311（AT-A12-SRC）／fixture-contract.json demoSeed（アレルゲンの節なし）／review-resolution-contracts.md:438, 682, 687
- Problem: allergenObservationはtelemetry.seriesの付帯情報として出力型とavailability（available/not_measured/unsupported）の表示規則だけが定義されている。Repositoryがこの値を何から作るか（保存資源、Capabilityの属性、Measurement以外のイベント）、not_measuredとunsupportedを分ける条件は定義されていない。demoSeedにアレルゲン観測の節は無く、DemoTriggerにもアレルゲン投入のイベントが無い。IR69は「そこに無い業務記録をRepositoryが補わない」、IR92はpatchのentityをdemoSeedの節に限るため、AT-C07-SRC/AT-A12-SRCが求める「作り込んだ観測データ」のfixtureを規則どおりに作る方法が無い。
- Why it matters: 企業原文BIZ-18（アレルゲン）の対応として明記された表示が、データ定義の欠落により実装も受入もできない（「UIはあるがデータ定義が無い機能」）。
- Example Failure: 実装Agentは常にnot_measuredを返す実装しか作れず、AT-C07-SRCの「available」fixtureを用意できない。別の実装Agentは独自の保存資源を追加し、IR69/D12に反する。
- Required Fix: allergenObservationの取得元（例: demoSeedの新しい節、またはCapability/Deviceの属性とDemoTriggerの新イベント）、availabilityの導出規則、最新1件の選び方、複数Unit時のnullの扱い、acceptancePatchesのキーを定義する。
- Suggested Revision: demoSeedに`allergenObservations: [{id, unitId, substance, value, unit, sourceLabel, observedAt, evidenceText}]`を追加し、「Capabilityに`allergenSupported:boolean`を持ち、falseならunsupported、trueで観測0件ならnot_measured、観測ありならavailable（observedAt降順先頭）」とする。AT-C07-SRCとAT-A12-SRCに3件のacceptancePatchesキーを付ける。

#### G1-007

- Severity: MAJOR
- Issue ID: G1-007
- Document: 01-requirements/client.md（FR-C07）、02-design/client.md（DD-C07）、02-design/deterministic-contracts.md（D08）
- Location: 要件client.md:195（基本フロー）, :196（BR-C07）, :202（AT-C07-N②「案内『換気を推奨』」）／設計client.md:272-277／deterministic-contracts.md:119
- Problem: FR-C07は「換気や清掃の案内を見る」と定め、AT-C07-Nは「room-1: CO₂=1000ppm…→②案内『換気を推奨』」を期待する。しかし、どの指標がどの値（または品質）のときに「換気を推奨」を表示するか、清掃の案内をどの条件（PM2.5等）で出すか、条件を満たさないときに何を表示するかは、FR-C07・DD-C07・D08のどこにも無い（全文検索でも該当規則なし）。定義されているのは換気非対応設備の手動換気案内（D08）だけである。
- Why it matters: 企業原文BIZ-18「清掃・換気の案内」の表示条件を実装Agentが数値で推測することになり、「数値…を実装時に推測しない」という各文書冒頭の実装基準に反する。AT-C07-N②の合否も判定できない。
- Example Failure: 実装AはCO₂≥1000ppmで推奨を出し、実装BはA12のPolicy閾値があるときだけ出す。seedのCO₂=800ppmの表示で両者の画面が異なり、どちらもテストで否定できない。
- Required Fix: 案内の表示規則（指標、比較演算子、閾値、必要な品質、換気能力の有無ごとの文言キー、清掃案内の条件）を定義し、AT-C07-N/E/Bに境界値のsubcaseを追加する。
- Suggested Revision: DD-C07に「co2 value≥1000 ppmかつquality=validなら`air.guidance.ventilate`、pm25≥35 µg/m³かつvalidなら`air.guidance.clean`、欠測・stale・suspectでは案内を出さず品質表示のみ（デモ閾値、DEC-09）」を追加し、AT-C07-Bに999/1000ppmの境界を加える。

#### G1-008

- Severity: MAJOR
- Issue ID: G1-008
- Document: 02-design/technician.md（DD-T02/DD-T09）、02-design/implementation-contracts.md（DDC-02）、02-design/service-contracts.ts、01-requirements/technician.md
- Location: 設計technician.md:94（components/serviceScope）, :288-293（workText「提出時必須 10〜4000文字」、inspectionItems「提出時必須 対象全部に結果/理由」、nextAction.kind「必須」）／implementation-contracts.md:119／service-contracts.ts:43（UnitDetail.components）／fixture-contract.json units（unit-online-rtoのserviceScope=indoor/outdoor/electrical）／要件technician.md:138（AT-T04-N）, :157（AT-T05-N）, :176（AT-T06-N）, :220（AT-T08-N）, :239（AT-T09-N）
- Problem: 提出時の検証は「対象全部に結果/理由」を要求するが、「対象」を決めるUnitDetail.componentsをserviceScope等からどう導出するかは定義されていない。unit-online-rtoのserviceScopeは3グループ（計18部品）である。AT-T04-Nはindoor 8部品だけを入力して「②提出成功」を期待し、AT-T05-N（outdoor 5部品）、AT-T06-N（electrical 5部品）も同様である。これらのATにはworkText（提出時必須）とnextAction（必須）の入力も無い。逆にAT-T09-Nは本文・写真・部品・nextActionだけで点検結果を入力せずに提出成功を期待し、AT-T08-Nは「編集」とだけ書く。
- Why it matters: 技術者の点検・報告（FR-T04〜T06/T08/T09、P0）の受入で、書かれた入力どおりに操作すると提出がVALIDATIONになり、期待結果に到達しない。必須集合の導出が未定義のため、実装Agentも検証範囲を決められない。
- Example Failure: テストAgentがAT-T04-Nの記載どおりindoor 8部品だけを保存して提出すると、outdoor/electricalのresult=null・workText欠落でVALIDATIONとなる。別の実装はindoorだけで提出を通し、AT-T04-E①（result=null 1部品でVALIDATION）の判定範囲がグループ単位に縮む。
- Required Fix: UnitDetail.componentsの導出規則（serviceScopeのグループ→ComponentKey一覧）と、jobs.submitの必須検証集合を定義する。AT-T04/T05/T06/T08/T09-Nの前提に、対象外グループを含む全点検結果・workText・nextActionの入力値を明記するか、共通の「提出可能な下書き」fixtureを定義する。
- Suggested Revision: DD-T02に「components=serviceScopeの各グループのComponentKey全件（indoor 8、outdoor 5、electrical 5）」を追加し、AT-T04-NのWhenを「filter=attention（理由・写真1枚）、他の17部品=normal、workText=50文字、nextAction=noneで保存→提出」とする。T05/T06/T09-Nも同様に全入力を明記する。

#### G1-009

- Severity: MAJOR
- Issue ID: G1-009
- Document: 01-requirements/admin.md（FR-A05/A11/A12）、02-design/admin.md（DD-A05）、02-design/review-resolution-contracts.md（IR07）、02-design/strict-review-contracts.md（SR21/SR28）
- Location: 要件admin.md:161（AT-A05-N）, :293（AT-A11-N）, :324（AT-A12-N）／設計admin.md:219-229（DD-A05の必須欄）, :235／review-resolution-contracts.md:48（IR07「新規は…enabled=false、priority=50」）／strict-review-contracts.md:137-139（SR21「無効disabled…はsuppressed」）, :181（SR28「必須値をRepositoryやUIが勝手な数値で補完しない」）, :183（「inAppはsimulated」）
- Problem: AT-A05-Nの保存入力はunitIds、metric、gte 30°C、duration、severity、recipient、channelだけで、DD-A05/SR28/正規型で必須のname、recoveryThreshold、cooldownMinutes、escalateAfterMinutesが無く、enabledも指定していない。IR07の新規既定はenabled=falseで、SR21により無効なPolicyは評価でsuppressedとなるため、期待結果「②Alertが1件、Notificationのプレビューが1件」に到達しない。AT-A12-N（fixture.airPolicyNotificationInputは指定するがenabled・name・recoveryThreshold・durationSecondsなし）とAT-A11-N（enabled・name・timezoneなし）も同じ欠落を持つ。また「Notificationのプレビュー」はinAppではSR28によりdeliveryState=simulatedの保存Notificationであり、previewとの区別が本文で曖昧である。
- Why it matters: 必須値をUI/Repositoryが補完してはならない（SR28）ため、テストAgentが値を推測しない限り保存できず、既定値を使うと期待結果に達しない。FR-A05はP0である。
- Example Failure: テストAgentがAT-A05-Nの記載値だけで保存するとVALIDATION（name等の欠落）。欠落値を補ってもenabledを既定のfalseのまま保存するとAlert 0件で失敗する。
- Required Fix: AT-A05-N/A11-N/A12-NのWhenに全必須入力とenabled=trueを明記するか、fixture-contract.jsonにPolicy入力の固定オブジェクトを定義して参照させる。「プレビュー」をdeliveryState（preview/simulated）で書き分ける。
- Suggested Revision: AT-A05-NのWhenを「name=Demo high temperature、unitIds=[unit-online-rto]、metric=temperature、operator=gte、threshold=30、recoveryThreshold=28、durationSeconds=60、severity=warning、recipientMembershipIds=[customer-a]、channels=[inApp]、cooldownMinutes=5、escalateAfterMinutes=60、timezone=Asia/Kuala_Lumpur、enabled=true、priority=50で保存」とし、Thenを「Notification（deliveryState=simulated）1件」とする。

#### G1-010

- Severity: MAJOR
- Issue ID: G1-010
- Document: 01-requirements/client.md（FR-C04）、04-agentic-sdlc/acceptance-fixes.csv、04-agentic-sdlc/fixture-contract.json、02-design/review-resolution-contracts.md（IR36/IR92）、02-design/deterministic-contracts.md（D01/D09）、04-agentic-sdlc/verification.md
- Location: 要件client.md:140（AT-C04-E③）／acceptance-fixes.csv:20（AT-FIX-019）／fixture-contract.json:14-15（customer-aのvalidFrom=2026-09-01、validUntil=2026-10-01。全actor同じ）, :371（note）／review-resolution-contracts.md:246（IR36「Membership.validUntil…はジャンプで通常どおり失効」）, :678（IR92規則1）／deterministic-contracts.md:18-21, 141／verification.md:42（`validFrom <= now < validUntil`）
- Problem: AT-C04-E③は時計を2026-03-07／2026-10-31に設定して週次ルールを保存しVALIDATIONを期待し、AT-FIX-019も2026-03-07を使う。しかしseedの全actorのMembership有効期間は[2026-09-01, 2026-10-01)で、両日付とも範囲外である。Givenは有効期間の上書きを指定しておらず、IR92規則1では書かれていない値はseedのままとなる。範囲外のMembershipによる保存はD01順位2/4（UNAUTHENTICATEDまたは担当期限外FORBIDDEN）に当たり得るが、DST検証をD01のどの順位で行うか（順位1の構造検証か順位7の関連値か）は定義されていない。なおD09は保存時nowから366日以内の全occurrenceを検証するため、seedの時計（2026-09-14）のままでも2026-11-01（曖昧）と2027-03-14（不存在）が検出され、時計を動かす必要自体が無い。
- Why it matters: 受入の前提データと期待するエラーコードが、seed・IR92・D01から一意に決まらない。
- Example Failure: 実装Aは2026-03-07ではcustomer-aのsignInまたは保存をFORBIDDENとしAT-C04-E③が失敗する。実装BはDST検証を先に行いVALIDATIONを返す。どちらもD01の文言に反するとは言えない。
- Required Fix: Givenに有効期間の上書きを明記するか、時計を有効期間内に置いたまま366日検証で検出するケースに書き換える。DST検証がD01のどの順位に当たるかを明記する。
- Suggested Revision: AT-C04-E③を「時計はseedのまま（2026-09-14T01:00Z）、timezone=America/New_York、日曜02:30（2027-03-14が不存在）／日曜01:30（2026-11-01が曖昧）の週次ルールを保存→VALIDATION（D01順位1）」へ変更し、AT-FIX-019も同じ前提に揃える。

#### G1-011

- Severity: MAJOR
- Issue ID: G1-011
- Document: 04-agentic-sdlc/acceptance-review-019.csv、02-design/review-resolution-contracts.md（IR69/IR89）、02-design/deterministic-contracts.md（D06）、04-agentic-sdlc/fixture-contract.json
- Location: acceptance-review-019.csv:16（AT-REV19-015）, :3（AT-REV19-002）, :38（AT-REV19-037）／review-resolution-contracts.md:438, 629, 631／deterministic-contracts.md:93／fixture-contract.json:1501-1504（job-contractor-a.scheduledSlot.endAt=2026-09-20T00:00Z）, :1533-1536（assignment-contractor-aのscheduled/validFrom/validUntil）
- Problem: AT-REV19-015は「assignment-contractor-aをscheduledEnd=01:20Zへpatch」するだけで、01:20Zに「業者/HQの一覧に『作業窓終了・再割当が必要』」を期待する。IR89はこの表示を「JobSummary/JobDetailでstatus∈{assigned,in_progress}かつscheduledSlot.endAt<=now」で判定するが、IR69/IR92により書かれていないJob.scheduledSlotはseed（endAt=2026-09-20T00:00Z）のままで、01:20Zには条件を満たさない。IR89末尾はjobs.assignでJob.scheduledSlotとAssignmentを同期させる不変条件を定めているが、patchはこれを破る。さらにD06は「AssignmentのvalidFrom/UntilはscheduledStart/Endと同じ」とするのに、patch後はvalidUntil=2026-09-20のまま食い違い、作業窓をどちらで判定するかで技術者側の結果も変わり得る。AT-REV19-002・AT-REV19-037も同じくscheduledStart/Endだけをpatchしており、AT-REV19-037の「02:00Zを過ぎても表示されない」はpatch前から表示されない状態のため、同期の検証として機能しない。
- Why it matters: ユーザー確定のDEC-50（IR72順位1）に対応する受入が、規則どおりに前提を作ると期待結果を出せない。
- Example Failure: テストAgentがGivenどおりにpatchすると、01:20Zに技術者画面は破棄・history表示になるが、HQ/業者の一覧にはIR89の表示が出ず、AT-REV19-015が失敗する。
- Required Fix: AT-REV19-002/015/037のGivenでJob.scheduledSlotとAssignment.validFrom/validUntilも同じ値にpatchする（またはjobs.assignの通常操作で作る）。patchが守るべき関連フィールドの不変条件をIR92に追記する。
- Suggested Revision: AT-REV19-015のGivenを「assignment-contractor-aのscheduledEnd=validUntil=01:20Z、job-contractor-aのscheduledSlot.endAt=01:20Z・status=in_progress」とし、AT-REV19-037は旧枠が過ぎた02:00Zの時点で再割当前に表示が出ることも確認する手順に改める。

#### G1-012

- Severity: MAJOR
- Issue ID: G1-012
- Document: 04-agentic-sdlc/acceptance-review-017.csv、04-agentic-sdlc/acceptance-review-018.csv、04-agentic-sdlc/verification.md、02-design/review-resolution-contracts.md（IR47/IR57/IR74）、02-design/deterministic-contracts.md（D05/D14）、04-agentic-sdlc/fixture-contract.json
- Location: acceptance-review-017.csv:5（AT-REV17-004）, :6（AT-REV17-005）, :15（AT-REV17-014）／acceptance-review-018.csv:4（AT-REV18-003）, :14（AT-REV18-013④）／verification.md:209-219／review-resolution-contracts.md:329（IR47）, :390（IR57）, :513（IR74 REV18-035）／deterministic-contracts.md:87（D05）, :192（D14）／strict-review-contracts.md:133（SR20）／client.md:246（AT-C09-N）／fixture-contract.json:1006-1007（device-tamper.unitId=unit-non-rto）, :1318-1324（contract-general-a、endAt=2027-01-01）
- Problem: verification.mdはAT-REV17-001〜015・AT-REV18-001〜048を既存ATと合わせて検証するよう求めるが、次の期待値が現行の上位規範・seedと矛盾する。(a) AT-REV17-014は`/customer/units/unit-other-customer`で「SCR-X-not-foundを表示…認証済みならrole home」とするが、IR57は既知routeのprimary queryがNOT_FOUNDなら「URLを維持したままその場でnot-found状態（親一覧へのリンク）」とし、AT-REV18-013④も「URL維持でnot-found表示と親一覧リンク」を期待する。(b) AT-REV17-004はZの無い「希望枠2026-09-15 10:00〜12:00」から「省略時dueAt=2026-09-15 12:00Z」を期待するが、IR74 REV18-035ではZ無しはAsia/Kuala_Lumpurのローカル時刻で、AT-C09-Nも同じ枠を02:00Z〜04:00Zとしているため、dueAtは04:00Zになる。(c) AT-REV17-005はunit-non-rtoのarchive成功を前提にするが、seedではdevice-tamperがunit-non-rtoに現bindingを持ち、contract-general-a（endAt=2027-01-01）がunit-non-rtoを対象にするため、D05/D14によりarchiveはCONFLICTになる。(d) AT-REV18-003はcommunication_lost→restored→power_lost→tamperの後のcommands.createを「tamperだけではCommandを受付」とするが、restoredは接続軸だけを復旧し（SR20）、power_lostの後に電源の復旧が無いためIR47によりpowerSignal=offが最優先でOFFLINEになる。
- Why it matters: IR72は「見つけた食い違いは実装で選ばず文書欠陥として報告し、G1を停止する」と定める。旧受入計画は現行manifestに含まれ、テストAgentへの入力である。IR81の旧記述検査はMarkdownとverification.mdが対象で、受入CSVの期待値は検査されない。
- Example Failure: IR57どおりに実装するとAT-REV17-014が失敗し、AT-REV17-014に合わせるとAT-REV18-013④が失敗する。AT-REV17-005はseedのままではarchiveがCONFLICTとなり以降の手順が実行できない。
- Required Fix: 上記4件の期待値・前提を現行規範に合わせて修正する（またはverification.mdで置換済みのcaseを明示して実施対象から外す）。受入CSVの期待値も旧記述検査または整合テストの対象に加える。
- Suggested Revision: (a) AT-REV17-014の後半を「customer-aの/customer/units/unit-other-customerはSCR-C03上でURL維持のnot-found状態と親一覧リンク（IR57）。未定義routeだけSCR-X-not-found」、(b)「省略時dueAt=2026-09-15T04:00Z」、(c) Givenに「device-tamperのbindingを解除し、contract-general-aのendAtを過去へpatch」を追加するか、依存の無い新規Unitを対象にする、(d) 手順を「…→power_lost→restored(power)→tamper→commands.create」とする。

### 3.2 MINOR

#### G1-013

- Severity: MINOR
- Issue ID: G1-013
- Document: 02-design/service-contracts.ts、02-design/review-resolution-contracts.md（IR03/IR35）、02-design/operation-catalog.csv
- Location: service-contracts.ts:84（Restriction）, :86（RestrictionReleaseView）／review-resolution-contracts.md:26, 236／operation-catalog.csv restrictions.reconcile/retryの認可列（`release-intent-or-terminal-recovery-only`）
- Problem: IR35は解除要求時に`releaseIntent={source,at,actorMembershipId}`を保存するとし、IR03とカタログはoverride専用者のreconcile/retry可否をこの解除意思（override由来か）で決める。しかし正規型のRestriction/RestrictionReleaseViewにreleaseIntentは無く、内部専用値であるという宣言（IR31のcontributorUserIdsのような記述）も無い。
- Why it matters: override専用者のA10画面でreconcile/retryボタンの表示可否を、UIが取得した値から決められない。
- Example Failure: 入金確認でrelease_requestedになった制限に対し、override専用者の画面にretryボタンが出て、押すとFORBIDDENになる。
- Required Fix: releaseIntentをRepository内部値と明記するか、RestrictionReleaseViewに`releaseIntentSource`を追加する。
- Suggested Revision: 正規型に`releaseIntent:{source:'payment'|'exception'|'override'|'manual';at:Instant}|null`を追加し、RestrictionReleaseViewにも含める。

#### G1-014

- Severity: MINOR
- Issue ID: G1-014
- Document: 03-uiux/screen-catalog.csv、02-design/review-resolution-contracts.md（IR51）、02-design/client.md（DD-C08）
- Location: screen-catalog.csv:8（SCR-C08のoperations=alerts.list;notifications.markRead;notifications.list）／review-resolution-contracts.md:355／設計client.md:298-303／要件client.md:229（AT-C08-B①〜③）
- Problem: IR51は「FR-C08/AT-C08-Bの『未対応件数』はalertCountを指し、通知の未読件数とは別の値として別に表示する」とするが、SCR-C08の操作にsummaries.getが無く、DD-C08のフィールド表にもalertCountが無い。D07によりKPIはsummaries.getで集計し、一覧ページから計算してはならない。
- Why it matters: AT-C08-Bの観測画面（C08かC01か）が決まらない。
- Example Failure: 実装AはC08でalerts.listの結果から件数を数え、D07に反する。実装BはC08に件数を出さず、AT-C08-BをC01で観測する。
- Required Fix: 表示画面を明記し、C08で表示するならSCR-C08にsummaries.getを追加する。
- Suggested Revision: SCR-C08のoperations/secondary_queriesにsummaries.get(kind=customer)を追加し、DD-C08に「alertCount（IR51）」の読取欄を追加する。

#### G1-015

- Severity: MINOR
- Issue ID: G1-015
- Document: 01-requirements/client.md、02-design/service-contracts.ts
- Location: 要件client.md:310（AT-C12-E①「offline台はpending」）／service-contracts.ts:83（releaseStateのenum）
- Problem: 「pending」はRestrictionUnit.releaseState（none/waiting_reconcile/requested/released/not_required/failed）に無い。unit-limitedはremove Command作成後に通信断となるため、30秒経過前はrequested、後はfailedとなり、どの値・表示を「pending」とするか決まらない。
- Why it matters: 観測値が一意でない。
- Example Failure: テストAgentがreleaseState=requestedとfailedのどちらを合格とするか判断できない。
- Required Fix: 観測時刻と期待するreleaseState/pendingReason/表示ラベルを明記する。
- Suggested Revision: 「①30秒未満ではperUnit.releaseState=requested・表示『解除応答待ち（通信断）』、30秒経過後はfailed・集約release_requestedのまま」とする。

#### G1-016

- Severity: MINOR
- Issue ID: G1-016
- Document: 03-uiux/UIUXSpecification.md（UX-05）、03-uiux/component-contracts.csv
- Location: UIUXSpecification.md:142（StatusBadge: domain/status/labelKey）, :146（CommandPanel: capability/observedState/pendingCommand/permission）, :147（ConfirmActionDialog: target/action/impact/reason/onConfirm）／component-contracts.csv:2（StatusBadge: status,severity,labelKey,quality）, :4（CommandPanel: unit,pending,permission,loading,error）, :5（ConfirmActionDialog: open,target,before,after,reasonRequired,submitting,error）
- Problem: 同じ共通Componentのprops名・構成がUX-05とComponent契約で異なる。IR72では契約CSV（順位5）が優先するが、UX-05側が置換されていない。
- Why it matters: 実装Agentが2種類のprops名を見て迷い、IR72の「文書欠陥として報告」に当たる。
- Example Failure: StatusBadgeに`domain`を渡す実装と`severity`/`quality`を渡す実装が混在する。
- Required Fix: UX-05のprops記述をcomponent-contracts.csvへの参照に置き換えるか、名称を揃える。
- Suggested Revision: UX-05の「受け取る情報」列を「component-contracts.csvのprops（IR72順位5）」へ変更する。

#### G1-017

- Severity: MINOR
- Issue ID: G1-017
- Document: 03-uiux/screen-catalog.csv、02-design/contractor.md（DD-P03）、02-design/deterministic-contracts.md（D10）、02-design/strict-review-contracts.md（SR11）
- Location: screen-catalog.csv:15（SCR-P03のurl_selection=tab,sort）／設計contractor.md:122（jobId必須）／deterministic-contracts.md:145／strict-review-contracts.md:80
- Problem: DD-P03は割当対象のjobIdを必須入力とするが、SCR-P03のURL許可キーにjobIdが無い。D10は「URLの選択ID・tabはcatalogに固定」、SR11は未知キーを除去するため、P02から対象案件を指定して遷移・Back復元することができない。
- Why it matters: AT-P03-Nの操作導線（受諾した案件の割当画面を開く）が定義されない。
- Example Failure: /partner/schedule?jobId=job-internal-aでjobIdが除去され、別の案件を選ぶ手間が発生する。
- Required Fix: url_selectionにjobIdを追加するか、URLに保持しない理由と選択方法を明記する。
- Suggested Revision: SCR-P03のurl_selectionを`tab,sort,jobId`とする。

#### G1-018

- Severity: MINOR
- Issue ID: G1-018
- Document: 02-design/deterministic-contracts.md（D06）、02-design/review-resolution-contracts.md（IR49/IR89）、04-agentic-sdlc/acceptance-review-019.csv
- Location: deterministic-contracts.md:93／review-resolution-contracts.md:345, 631／acceptance-review-019.csv:38（AT-REV19-037）
- Problem: D06の重複判定（既存start<newEnd AND newStart<existingEnd）は、延長・再割当で置き換える旧Assignmentを除外するかを定めていない。AT-REV19-037は同じ技術者の旧枠[00:00Z,02:00Z)と重なる新枠[01:30Z,04:00Z)の成功を期待する。
- Why it matters: 除外しない実装では延長がCONFLICTになる。
- Example Failure: jobs.assignが旧Assignmentとの重複でCONFLICTを返し、AT-REV19-037が失敗する。
- Required Fix: 重複判定から同じJobの置換対象Assignmentを除外することを明記する。
- Suggested Revision: D06に「同じjobIdの現在active Assignment（置換対象）は重複判定から除外する」を追加する。

#### G1-019

- Severity: MINOR
- Issue ID: G1-019
- Document: 02-design/review-resolution-contracts.md（IR91/IR92/IR31）、01-requirements/admin.md、01-requirements/client.md、04-agentic-sdlc/fixture-contract.json
- Location: review-resolution-contracts.md:681（IR92規則4「該当フィールドだけを…patch」）, :669（IR91規則10）, :195（IR31）／要件admin.md:276（AT-A10-B①）／要件client.md:311（AT-C12-B）／fixture-contract.json `acceptancePatches["AT-P01-N"]`（job-contractor-aをstatus=submittedだけ変更）
- Problem: 状態フィールドだけのpatchは、通常操作では到達しない組合せを作る。restriction-limited-aをstate=scheduledにしてもperUnit.applyState=applied、apply Command=acknowledged、unit-limited.observedRestrictionは残り、そこへのcancelの副作用（D03の解除評価、SR26の回復case）が定義されない。AT-P01-Nのjob-contractor-aはsubmittedなのに報告版が無く（reportRefs=[]）、IR31の寄与者集合も無い。
- Why it matters: 期待値として書かれていない副作用が実装ごとに異なり、同じfixtureを使う他の画面（P05等）で例外が出る。
- Example Failure: AT-A10-B①のcancel後、unit-limitedが観測上は制限中なのにeffectiveControlPolicy=unrestrictedとなり、別テストで不整合が表面化する。
- Required Fix: 状態patchで同時に揃える関連フィールドを規則化するか、これらのcaseを通常操作で作る。
- Suggested Revision: IR92規則4に「Restriction.stateのpatchではperUnit/Command/observedRestrictionを状態に整合する値（scheduled: perUnit=not_sent・Commandなし・observedRestriction=null等）へ同時に置き換える。submitted Jobには提出済み報告版を同時に作る」を追加する。

#### G1-020

- Severity: MINOR
- Issue ID: G1-020
- Document: 01-requirements/admin.md（FR-A14）、02-design/service-contracts.ts、02-design/review-resolution-contracts.md（IR88）
- Location: 要件admin.md:362（BR-A14「係数には、地域・年度・単位・出典が必須」）, :369（AT-A14-E「係数の単位が合っていない」）／service-contracts.ts:112（EmissionFactorに単位欄なし）／review-resolution-contracts.md:617（factorUnitは固定表示）
- Problem: EmissionFactorは単位を持たずkgCO2ePerKWhに固定されているため、「係数の単位が合っていない」状態を作れず、BR-A14の「単位必須」も検証対象が無い。
- Why it matters: AT-A14-Eのsubcaseが実行できない。
- Example Failure: テストAgentが単位不一致のfixtureを作れない。
- Required Fix: subcaseを削除するか、係数に単位欄を追加する。
- Suggested Revision: AT-A14-Eから「係数の単位が合っていない」を削除し、BR-A14を「単位はkgCO₂e/kWh固定（IR88）」とする。

#### G1-021

- Severity: MINOR
- Issue ID: G1-021
- Document: 01-requirements/contractor.md（FR-P05）、02-design/technician.md（DD-T04/T09）、02-design/service-contracts.ts
- Location: 要件contractor.md:161（AT-P05-B③）／設計technician.md:148, 289／service-contracts.ts:70（ReviewAvailabilityの理由）
- Problem: 「未点検あり理由なし」の報告は提出時にVALIDATIONとなるため、submitted状態にならず品質確認の対象にならない。demoSeedに報告の節も無い。ReviewAvailabilityにも「記録不備」の理由値が無い。
- Why it matters: 到達不能な前提と、存在しない無効化理由を期待している。
- Example Failure: テストAgentがAT-P05-B③の前提を作れない。
- Required Fix: subcaseを削除するか、提出後に不備が判明する別の条件へ置き換える。
- Suggested Revision: AT-P05-B③を「提出直前に理由なしnot_inspectedで提出→VALIDATION（T04-E③と同じ）」としてFR-T側へ移す。

#### G1-022

- Severity: MINOR
- Issue ID: G1-022
- Document: 01-requirements/client.md（FR-C05）、02-design/service-contracts.ts
- Location: 要件client.md:152（BR-C05「一般的な利用の同意」）, :160（AT-C05-B①「一般同意のみ」）／service-contracts.ts:99（Consent.purposeは`location_automation`のみ）
- Problem: 「一般同意」に当たるデータ・操作が無い。
- Why it matters: AT-C05-B①の前提を規則どおり作れない。
- Example Failure: テストAgentが独自のpurpose値を作る。
- Required Fix: 一般同意をモデル化するか、表現を「位置同意なし」に改める。
- Suggested Revision: AT-C05-B①を「位置同意granted=false（一般利用は同意記録なし）」とする。

#### G1-023

- Severity: MINOR
- Issue ID: G1-023
- Document: 01-requirements/technician.md（FR-T12）、02-design/service-contracts.ts、02-design/strict-review-contracts.md（SR20）
- Location: 要件technician.md:309（AT-T12-E②「新しい通信断の後に古いheartbeat」）／service-contracts.ts:142（DemoTriggerのdevice kindはcommunication_lost/power_lost/tamper/restored）／strict-review-contracts.md:133
- Problem: heartbeatを投入するDemoTriggerが無く、「古いheartbeat」を何で再現するか（古いsequenceのrestored等）が書かれていない。
- Why it matters: 手順を一意に実行できない。
- Example Failure: テストAgentが存在しないeventTypeを送りVALIDATIONになる。
- Required Fix: 使用するDemoTriggerと値を明記する。
- Suggested Revision: 「communication_lost(sequence=5)の後にrestored(axis=connection、sequence=4)→CONFLICTまたは反映なし、offlineのまま」とする。

#### G1-024

- Severity: MINOR
- Issue ID: G1-024
- Document: 01-requirements/client.md、01-requirements/admin.md、01-requirements/technician.md、02-design/deterministic-contracts.md（D05/D13）、02-design/service-contracts.ts
- Location: 要件client.md:340（AT-C13-N: purpose未指定）, :246（AT-C09-N: 割当先・枠・時計進行が未指定）／要件admin.md:399（AT-A15-N: customerId/purpose/period/unitIds未指定）／要件technician.md:258（AT-T10-N: 試運転のstartAction未指定、先行Commandのack待ちが未記載）, :283（AT-T11-N: checkの成功イベント・sensorTypes未指定）／deterministic-contracts.md:81, 177／service-contracts.ts:217
- Problem: 期待値には影響しないが必須の入力・手順がWhenに無い。
- Why it matters: IR92が禁じる推測補完が必要になる。
- Example Failure: AT-T10-Nで先行set_modeのack前に試運転を開始するとD05によりCONFLICTになる。
- Required Fix: 必須入力と手順を明記する。
- Suggested Revision: 例: AT-T10-Nに「set_modeをacknowledgedにした後、startAction=set_power true・endAction=set_power false」、AT-C13-Nに「purpose=Demo offset、period/unitIdsはIR92規則7」を追記する。

#### G1-025

- Severity: MINOR
- Issue ID: G1-025
- Document: 01-requirements/common.md（AT-X01〜X07）、04-agentic-sdlc/fixture-contract.json
- Location: 01-requirements/common.md:88, 97, 104, 113, 120, 127, 134／fixture-contract.json demoSeed.capabilities（2件とも温度対応・同じmode候補）
- Problem: 共通ATは文章で観点を列挙するだけで、Given/When/Thenの具体値やacceptancePatchesのキーが無い。例えばAT-X06の「温度に対応していない」「モードの候補が異なる」「送風のみに対応」の機種はseedに無く、IR92規則6の一覧にも無い。
- Why it matters: 共通AT（7束）のfixtureをテストAgentが自作することになる。
- Example Failure: AT-X06の非対応機種の定義がテストごとに異なる。
- Required Fix: AT-X01〜X07をN/E/B形式の表にし、必要なpatchをacceptancePatchesへ追加する。
- Suggested Revision: `acceptancePatches["AT-X06-B"]`にtemperature=null、modes=[cool]、control=false等のcapabilityを追加する。

#### G1-026

- Severity: MINOR
- Issue ID: G1-026
- Document: 04-agentic-sdlc/verification.md（S08）、02-design/strict-review-contracts.md（SR03/SR13）、04-agentic-sdlc/fixture-contract.json
- Location: verification.md:104／strict-review-contracts.md:24, 88／fixture-contract.json actors（tech-external-bのscopes=[unit-other-customer]）
- Problem: S08は業者aの辞退後に「別の業者」（contractor-b）へ再委託し自社技術者を割り当てるが、対象案件・設備が未指定で、IR92の既定（unit-online-rto）ではcontractor-bの技術者tech-external-bのscopeに設備が無い。jobs.assign/members.eligibleで技術者のunit scopeを検査するかも定義されていない。
- Why it matters: S08の前提が一意に作れない。
- Example Failure: 割当は成功するが、tech-external-bが設備を読めず作業開始できない。
- Required Fix: S08の案件・業者・技術者のIDを明記し、割当時のscope検査を定義する。
- Suggested Revision: S08の前提を「job: customer-bのunit-other-customerの案件、辞退: contractor-a、再委託: contractor-b、技術者: tech-external-b」とする。

#### G1-027

- Severity: MINOR
- Issue ID: G1-027
- Document: 01-requirements/client.md、01-requirements/technician.md、01-requirements/admin.md、04-agentic-sdlc/fixture-contract.json、02-design/service-contracts.ts
- Location: 要件client.md:214（AT-C08-SRC）／要件technician.md:188（AT-T07-SRC）／要件admin.md:148（AT-A05-SRC）／fixture-contract.json demoSeed.alerts（window_openの1件のみ）／service-contracts.ts:142（inferred/inspectionのAlertを作るDemoTriggerなし）
- Problem: 「断熱不足の点検記録」「根拠なし」のfixtureにID・値・acceptancePatchesキーが無い。クリック可能なデモでも、insulation_loss/inspection根拠のAlertを作る経路が無い。
- Why it matters: SRC受入の前提値がテストごとに異なり、デモで原文要件（BIZ-17）を示せない。
- Example Failure: 2つのテストが異なるevidenceTextで同じcaseを合格とする。
- Required Fix: acceptancePatchesにSRC 3件を定義し、デモ用の生成経路を定める。
- Suggested Revision: `acceptancePatches["AT-C08-SRC"]`にalert-insulation-a（causeCode=insulation_loss、evidenceKind=inspection）とalert-unknown-a（causeCode=unknown、evidenceKind=demo_observation）を追加する。

#### G1-028

- Severity: MINOR
- Issue ID: G1-028
- Document: 02-design/common.md §2、03-uiux/screen-catalog.csv、03-uiux/component-contracts.csv、01-requirements/common.md（FR-X01）
- Location: 02-design/common.md:58／component-contracts.csv:59（AppShellのevent: switchLocale;signOut;openNotifications;toggleVoice;extendSession）／screen-catalog.csv（demoSession.switchMembershipを持つScreenなし）／01-requirements/common.md:21
- Problem: 役割切替（FR-X01、P0）の専用メニューを担うComponent/Screenの契約にdemoSession.switchMembershipが無い。
- Why it matters: D13「Pageのread/write操作はscreen-catalogのoperationsを使い」により、呼び出し箇所を推測することになる。
- Example Failure: 役割切替を/loginへの再遷移で実装する案と、ヘッダーメニューで実装する案が並立する。
- Required Fix: AppShell/ShellContainerの契約にswitchMembershipを追加する。
- Suggested Revision: AppShellのeventに`switchMembership(demoMembershipId)`、api_dependencyに「ShellContainerがdemoSession.switchMembershipを実行」を追加する。

#### G1-029

- Severity: MINOR
- Issue ID: G1-029
- Document: 01-requirements/admin.md（FR-A09）、02-design/review-resolution-contracts.md（IR05）
- Location: 要件admin.md:250（AT-A09-E③「未通知」）／review-resolution-contracts.md:36
- Problem: IR05ではscheduleの同一遷移で予告通知を必ず作り、宛先0ならscheduleがVALIDATIONとなるため、「未通知」の制限は存在しない。
- Why it matters: subcaseの前提に到達できない。
- Example Failure: テストAgentが未通知の制限を作れない。
- Required Fix: 意図する条件（予告後24時間未経過等）に書き換える。
- Suggested Revision: AT-A09-E③を「予告後24時間未満でexecute→D01順位6のCONFLICT」とし、B①との重複を整理する。

### 3.3 QUESTION

#### G1-030

- Severity: QUESTION
- Issue ID: G1-030
- Document: 01-requirements/contractor.md（FR-P06）、02-design/query-catalog.csv、02-design/contractor.md（DD-P06）
- Location: 要件contractor.md:184（AT-P06-N「技術者2名」）／query-catalog.csv members.capacity/members.list（filters: organizationId,qualification,activeOnly）／設計contractor.md:206-221／fixture-contract.json actors（contractor-aはorg-contractor-aのrole=contractor）
- Problem: members.list/members.capacityが組織内のrole=contractorのMembership（contractor-a自身）を含むかが書かれていない。AT-P06-Nは技術者2名だけを期待する。
- Why it matters: 件数の期待値が変わる。
- Example Failure: 実装がcontractor-aを含めて3名を返す。
- Required Fix: 返却対象をrole=technicianに限定するかを明記する。
- Suggested Revision: IR42のcontractor向け投影に「members.list/eligible/capacityはrole=technicianのMembershipだけ」を追記する。

#### G1-031

- Severity: QUESTION
- Issue ID: G1-031
- Document: 02-design/review-resolution-contracts.md（IR45/IR85）、01-requirements/technician.md、01-requirements/client.md、04-agentic-sdlc/verification.md
- Location: review-resolution-contracts.md:308, 595／要件technician.md:119-121（AT-T03-N/E/B）／要件client.md:120, 202-204（AT-C03-N、AT-C07-N/B）／verification.md:40
- Problem: IR45は「Givenで測定値・観測時刻を固定するcase」ではsimulator=falseを与えるとし、IR85はKPI・件数のATだけを明示する。acceptancePatchesを持たないAT（例: AT-T03-Bの120秒/120秒+1ms、AT-T03-Nのsequence=4投入、AT-C07-BのCO₂/湿度値）がこれに該当するかをcaseごとに示した一覧が無い。
- Why it matters: 時計を分境界の先へ進めるcaseでは、シミュレーターの複写によりstale判定やsequenceの期待値が変わる。
- Example Failure: AT-T03-B①②で01:01:00Zの複写が作られ、120秒+1msでもvalidのままになる。
- Required Fix: simulatorの状態をcaseごとに明記するか、「acceptancePatchesを持たないATも、測定値・時刻・sequenceを観測するものはsimulator=false」と一般規則化する。
- Suggested Revision: IR92に規則9「測定値・観測時刻・stale・sequenceを観測するATはsimulator=falseで開始し、自動生成を検証するcase（AT-REV18-001、AT-REV19-003/009）だけenabled=true」を追加する。

## 4. 確認した範囲と確認しなかった範囲

### 4.1 確認した範囲

- 通読: docs/README.md、04-agentic-sdlc/README.md、00-prepare/PrepareDocument.md、01-requirements/{common,client,contractor,technician,admin}.md、02-design/{common,deterministic-contracts,strict-review-contracts,review-resolution-contracts,implementation-contracts,client,contractor,technician,admin}.md、02-design/service-contracts.ts、03-uiux/UIUXSpecification.md、04-agentic-sdlc/verification.md。
- fixture-contract.json: actors、energy、reviewResolution、demoSeedの全節、acceptancePatchesの全20キーをスクリプトで展開して確認。主要ATの数値（AT-A01-N、AT-C06-N/E.2〜E.4、AT-REV19-004/011/041、AT-P01-N、AT-P06-Bの曜日、AT-C04-Nの曜日）を再計算。
- 受入計画CSV: acceptance-review-019.csv（全42件）を精査。acceptance-review-018/017/016、acceptance-fixes、acceptance-independent、acceptance-strict-review、acceptance-rereview、acceptance-resolution、acceptance-convergence、acceptance-loop、acceptance-projection、acceptance-independent-g1の全行を出力し、現行IR・seedとの矛盾を走査。全CSVの列数とexecution_status値をスクリプトで確認。
- スクリプトによる横断照合: 正規型OperationContracts（137）と操作カタログのinput/result/mode、write操作（76）と版カタログ、Query入力を持つread（35）とQueryカタログ、画面カタログのoperations/primary/secondaryの操作名と包含関係、Component名、IR71の無効化表の操作名、操作カタログの認可列（全行出力）、Queryカタログのfilter/sort/mapping（全行出力）、版カタログ（全行出力）、画面カタログのurl_selection/states/tabs（全行出力）、component-contracts.csv（全行出力）。
- 決定台帳: review-decisions-016〜019.jsonの各DECのstatus（DEC-44/50がaccepted）。
- 検証: 両検証器の実行、manifestのSHA-256とbaseline IDの再計算、service-contracts.tsのTypeScript strict検査（scratchpadの複製に対して実行）。

### 4.2 確認しなかった範囲・限界

- 00-prepare/sources/*（original-handover.md、company-requirements-original.txt、production-instructions.md、reference-style-evidence.json）、reference-design-analysis.md、internal/design-assumptions.md、decision-record-2026-09-16.mdの本文は通読していない（見出しと引用箇所のみ）。企業原文→BIZ→FRの対応の妥当性は再検証していない。
- company-requirement-map.csv、requirement-origins.csv、traceability.csvは行単位で照合しておらず、件数は検証器の出力に依拠した。
- agents/*.md、templates/artifacts.md は読んでいない。
- validate_documents.py・check_review_regressions.pyのソースコードはレビューしておらず、実行結果だけを確認した。検証器が何を検査していないかは、IR81の記載範囲から推定した。
- UIトークン値と参考サイトの抽出証跡の照合、コントラスト比の再計算は行っていない。
- 画面カタログのentry/exit/interaction/data_contract/query_triggerの各列、Component契約のpropsとDTOの型の対応は全Screen・全Componentでは照合していない（抜き取り）。
- 受入条件の期待値の再計算は上記の抜き取りに限る。182束・旧受入計画の全subcaseを1件ずつ机上実行してはいない。
- アプリは未実装のため、アプリ試験・E2E・アクセシビリティ試験は実行していない（not_run）。
- 過去版の記録（runs/DOC-0.17.0以前、05-document-review）は参照していない。修正担当の自己レビュー記録（runs/DOC-0.19.0/review.md、gate-G1.yaml等）は判定後にヘッダーだけ参照し、判定根拠にしていない。

## 5. G1判定

**FAIL**

理由: G1の合格条件は「BLOCKER・CRITICAL・MAJORが0件、両検証器が成功」である。両検証器は成功した（終了コード0、変異62件すべて検出）が、MAJORが12件ある（G1-001〜G1-012）。これらは、Role別の書込み認可の矛盾（G1-001）、業務イベント通知・アレルゲン観測・空気環境案内のデータ定義または表示規則の欠落（G1-002、G1-006、G1-007）、制限取消の遷移の矛盾（G1-003）、報告提出の必須集合の未定義（G1-008）、受入条件の前提データが上位規範・seedと矛盾するかIR92の規則から一意に作れない問題（G1-004、G1-005、G1-009〜G1-012）であり、実装Agent・テストAgentが追加質問や推測なしに進められない。

BLOCKER/CRITICALは0件で、baseline・manifest・カタログ間の機械的整合は保たれている。MAJOR 12件を修正して新しいbaselineを作成した後、別主体による再判定を行うこと。MINOR 17件とQUESTION 2件は記録として引き継ぐ。
