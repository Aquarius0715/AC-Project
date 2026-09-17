# 厳格レビュー指摘一覧 STRICT-DOC-0.17.0-REV18（2026-09-17、中断記録）

対象: DOC-0.17.0 baseline `654be5e4…d8df`。レビュー主体: 以前のセッションのAI（Fable 5.1）。修正（IR45〜74、DEC-25〜41、受入計画AT-REV18-001〜048）は作業ツリーへ反映されたが、実行記録の作成・自己再レビュー・baseline作成の前にセッションが中断した。本書は中断したセッションの記録から回収した指摘データを、変更せずに保存したものである。gate-G1はnot_evaluated、spec-manifestは作成していない（IR75）。修正内容の検証と引継ぎは独立レビューREV19（[DOC-0.19.0](../DOC-0.19.0/review.md)）で行った。アプリ試験はnot_run。

初回判定: NOT READY（BLOCKER 2、CRITICAL 4、MAJOR 24、MINOR 16、QUESTION 2）。

## BLOCKER（2件）

### REV18-001
- Severity: BLOCKER
- Document: 確定契約D07/D09、IR36、SR27、fixture.powerClassification
- Location: デモ時計の進行モデル、稼働分類、stale判定
- Problem: デモ時計は実時間で進むが、合成測定値や生存信号を時間経過で生成する仕組みがどこにも定義されていない。seedの最終観測は00:59:30Zで、SR27の120秒条件により起動から約90秒で全設備がstale／稼働unknownになる。
- Why it matters: 1Aの主目的はクリック可能な監視デモで、C01/A01/T03のダッシュボードが起動直後に機能しなくなる。実装者ごとに乱数生成・定期生成なし・自動offline化など別の挙動を作る。
- Example Failure: デモ開始から2分後、顧客ダッシュボードが「稼働0 / 停止0 / 不明4」になり、関係者向けデモが成立しない。別の実装では乱数値で受入の期待値が毎回変わる。
- Required Fix: 生成の有無・周期・値の決め方・ジャンプ時の補完・停止手段・受入試験での固定方法を確定する。
- Suggested Revision: IR45: UTC分境界ごとにonline Deviceの各Sensorの最新値を複写生成（乱数なし）、ジャンプは補完しない、DemoTrigger simulatorでON/OFF、生存フィールドはversionを変えない。
- Status: corrected_unverified（IR45。検証はDOC-0.19.0）

### REV18-002
- Severity: BLOCKER
- Document: 確定契約D03、SR05、DDC-08 §4、FR-C03/C12/A09/T10
- Location: 制限中（effectiveControlPolicy=restricted）の操作可否
- Problem: temperature_limitとpower_offの適用中に、どのUnitAction（set_power/set_temperature/set_mode/set_fan/ventilate）を許可・拒否するかの表が無い。AT-C03-E④が温度下限だけを示す。
- Why it matters: BIZ-21の未払い制限の中核ルールで、許可範囲の解釈違いはそのまま制限の迂回（セキュリティ上の欠陥）になる。
- Example Failure: power_off適用中に顧客がset_power trueを送ると、ある実装は受付けて冷房が再開し、制限が無効になる。音声・自動運転・試運転の各経路で結果が食い違う。
- Required Fix: policy種別×UnitActionの可否、適用する全経路、エラーコード、UIの候補表示を1表で確定する。
- Suggested Revision: IR46の表（temperature_limitは下限未満の温度だけFORBIDDEN、power_offはset_power false以外FORBIDDEN）を全経路に適用。
- Status: corrected_unverified（IR46。検証はDOC-0.19.0）

## CRITICAL（4件）

### REV18-003
- Severity: CRITICAL
- Document: 共通詳細設計§3、正規型ACUnit/Device、D01/D05
- Location: ACUnit.connection/lastSeenAtとDevice状態の関係
- Problem: ACUnitとDeviceの両方にconnection/lastSeenAtがあるが、どちらが正でどう導出するか未定義。powerSignal=offやtamper=detectedが制御可否に影響するかも未定義。
- Why it matters: AT-C03-E②のOFFLINEはUnitの値、T12の通信断はDeviceの値を変えるため、機器の通信断で制御が止まるかが実装者で分かれる。
- Example Failure: device-online-rtoにcommunication_lostを送ってもUnit.connectionがonlineのままで、顧客の操作が成功扱いになる。
- Required Fix: Unit側の派生規則、未bind時の値、OFFLINEの判定条件とmessageKey、tamperの扱いを確定する。
- Suggested Revision: IR47: Unitは現binding Deviceから派生、未bindはunknown、online以外またはpowerSignal=offはOFFLINE、tamperは制御を止めず注意表示。
- Status: corrected_unverified（IR47。検証はDOC-0.19.0）

### REV18-004
- Severity: CRITICAL
- Document: 共通詳細設計§5、DDC-04、IR23、FR-P02/A06
- Location: 未応答Offerの期限到来
- Problem: offerExpiresAtを過ぎた未応答Offerについて、業者一覧から消えることだけが定義され、Job.statusの遷移が無い。requested→offeredしか委託遷移が無いため、HQは再委託できない。
- Why it matters: S08の外注フローで、業者が応答しないだけで案件が取消以外に進めなくなる。
- Example Failure: Offer期限後、案件はofferedのまま残り、HQのjobs.offerはCONFLICT、業者のacceptもCONFLICTで、取消するしかない。
- Required Fix: 期限到来時のJob遷移、Offerの保持方法、再委託の可否、通知の有無を確定する。
- Suggested Revision: IR48: 期限到来でJobをrequestedへ戻しcontractorOrgId=null、Offerは保持、通知なし、新offerIdで再委託可。
- Status: corrected_unverified（IR48。検証はDOC-0.19.0）

### REV18-005
- Severity: CRITICAL
- Document: 確定契約D06、IR23/IR24、FR-T01、AT-T01-N
- Location: Assignmentの有効期間と技術者の閲覧範囲
- Problem: D06はAssignmentのvalidFrom/Untilを予定枠と同一にしており、予定開始前に担当技術者が案件を閲覧できるかが未定義。FR-T01は「予定」の表示を要求している。
- Why it matters: 技術者ダッシュボードの予定表示、job.assigned通知からのリンク、事前準備がいずれも成立しない。
- Example Failure: 10:00開始の割当を08:00に受け取った技術者が通知を開くとNOT_FOUNDになり、T01の予定件数も0になる。
- Required Fix: 閲覧と操作を分けた期間定義、開始前に拒否する操作、社内と外部の違いを確定する。
- Suggested Revision: IR49: 閲覧窓=[割当作成, 予定終了)、作業窓=[予定開始, 予定終了)。開始前は案件起点の操作と外部技術者の設備読取をFORBIDDEN。
- Status: corrected_unverified（IR49。検証はDOC-0.19.0）

### REV18-006
- Severity: CRITICAL
- Document: 画面カタログurl_selection、SR11、AT-C01-N③、AT-A01-N④
- Location: KPIからの一覧遷移とURL許可キー
- Problem: 顧客の設備一覧routeが存在せず、AT-C01-Nの「稼働カードから一覧へ遷移」の遷移先が無い。SR11の「共通filter/sort/期間キー」が列挙されておらず、url_selectionに無いpropertyId/powerState/unreadOnly等が「未知キーは除去」で消える。
- Why it matters: ダッシュボードの主要導線と、戻る操作での条件復元（複数AT）が実装者の推測に依存し、テスト不能。
- Example Failure: /admin/units?powerState=onがallowlistに無いため除去され、KPIと一覧の件数が一致しない。
- Required Fix: 遷移先route、共通キーの列挙、画面ごとの追加キー、複数値とbooleanの正規化を確定する。
- Suggested Revision: IR50と画面カタログのurl_selection追加（C01/C02/C06/C07/C08/C10/T01/T02/P01/P06/A01/A02/A16）。
- Status: corrected_unverified（IR50。検証はDOC-0.19.0）

## MAJOR（24件）

### REV18-007
- Severity: MAJOR
- Document: D07、AT-C08-B、IR30
- Location: 未対応アラート件数の母集団
- Problem: D07はopen/acknowledgedのAlert数を返すとし、AT-C08-Bはseverity=normalを「未対応件数に計上しない」とする。「未対応件数」がどの値かも未定義。
- Why it matters: KPIの件数と受入の期待値が矛盾し、どちらかのテストが必ず失敗する。
- Example Failure: normalのopen Alertが1件あると、C01では異常1件、AT-C08-Bの期待値では0件になる。
- Required Fix: 件数の母集団（severity・status・archived）と、通知未読件数との区別を確定する。
- Suggested Revision: IR51: open/acknowledgedかつcritical/warningだけを数える。
- Status: corrected_unverified（IR51。検証はDOC-0.19.0）

### REV18-008
- Severity: MAJOR
- Document: FR-C05、DD-C05、正規型Condition、IR16
- Location: 生活パターン条件（pattern）の評価
- Problem: Condition型にpattern{localTime}があるが、Factの種類にもTTL表にも無く、いつ成立するかが未定義。
- Why it matters: FR-C05の選択肢として実装しても、動く・動かないの判断を実装者が作ることになる。
- Example Failure: ある実装は保存直後に即時発火し、別の実装は一度も発火しない。
- Required Fix: 成立条件（時刻・timezone・頻度）、ジャンプ時の扱い、DST検証、表示ラベルを確定する。
- Suggested Revision: IR52: ルールtimezoneで毎日1回localTimeのtickに成立する合成条件。
- Status: corrected_unverified（IR52。検証はDOC-0.19.0）

### REV18-009
- Severity: MAJOR
- Document: FR-C05、IR27、正規型RuleBase.disabledReason
- Location: 同意撤回時のルール状態
- Problem: 同意を取り消すとルールが「disabled」になるとあるが、enabledを変えるのか評価時に抑止するだけか、disabledReasonの値、版の増分、再同意時の復帰が未定義。disabledReasonの型にも該当値が無い。
- Why it matters: AT-C05-N④の「ルールはdisabled」の観測方法が一意に決まらない。
- Example Failure: 再同意した瞬間にルールが自動で有効に戻り、利用者が意図しない位置連動運転が再開する。
- Required Fix: 撤回時の状態遷移、理由値、再同意時の扱いを確定する。
- Suggested Revision: IR53: 撤回と同じ遷移でenabled=false・consent_revoked・version+1、再同意では自動復帰しない。
- Status: corrected_unverified（IR53。検証はDOC-0.19.0）

### REV18-010
- Severity: MAJOR
- Document: DD-C04手順3、確定契約D02
- Location: 予定の発火経路
- Problem: DD-C04は「デモ時計のイベントをautomations.fireに渡す」とし、D02は「時計による内部評価は閲覧セッションを使わず、内部関数をUIへ公開しない」とする。
- Why it matters: UI側で発火を呼ぶ実装では、顧客画面を開いていないと予定が動かない。
- Example Failure: HQでログイン中に顧客の18:00の予定が発火せず、AT-C04-N③が失敗する。
- Required Fix: 時計発火の担い手とUIのfireの位置づけを1つに決める。
- Suggested Revision: IR54: 時計発火は内部評価だけ、UIのfireは任意のデモ発火で同一tick結果を共有。DD-C04を修正。
- Status: corrected_unverified（IR54。検証はDOC-0.19.0）

### REV18-011
- Severity: MAJOR
- Document: D09、NFR-01（WCAG 2.2 AA）
- Location: セッション期限と延長
- Problem: 30分の固定寿命で延長も事前予告も無く、期限で未保存draftを破棄する。WCAG 2.2 SC 2.2.1（時間制限の延長手段と20秒以上の猶予）を目標とするNFR-01と衝突する。
- Why it matters: キーボード・支援技術の利用者が報告入力中に予告なく入力を失う。a11y検収（D10）で不合格になる。
- Example Failure: スクリーンリーダー利用の技術者が29分かけた点検下書きを保存直前に失う。
- Required Fix: 予告の時点、延長の操作と回数、監査の有無、期限到来時の扱いを確定する。
- Suggested Revision: IR55: 期限120秒前にalertdialog、demoSession.extendで30分延長（回数制限なし）。
- Status: corrected_unverified（IR55。検証はDOC-0.19.0）

### REV18-012
- Severity: MAJOR
- Document: 共通詳細設計§5、D06、operation-catalog jobs.cancel
- Location: 案件取消の許可状態
- Problem: HQがrequested案件を取消せるか、in_progress/submittedを直接取消せるか、取消時の未決Offerの扱いが未定義。
- Why it matters: 状態遷移の実装が分かれ、取消後に業者が受諾できてしまう実装が生まれる。
- Example Failure: HQが提出済み案件を直接取消し、提出版の品質確認が行われないまま消える。
- Required Fix: 役割×状態の取消可否表と結果を確定する。
- Suggested Revision: IR56の表。
- Status: corrected_unverified（IR56。検証はDOC-0.19.0）

### REV18-013
- Severity: MAJOR
- Document: DDC-03、役割別DDの冒頭、D10、SCR-X-forbidden
- Location: FORBIDDEN/NOT_FOUNDの表示と遷移
- Problem: 「ホーム画面や一覧画面へ移動」「使える画面へ戻す」「その場でpermission-denied状態」「/forbidden route」の4通りが書かれ、どれを使うか決まらない。
- Why it matters: 書込みのFORBIDDEN（制限違反など）で画面遷移すると入力が失われ、NFR-05の「入力を保持」に反する。
- Example Failure: 温度23を送ってFORBIDDENになった顧客がホームへ飛ばされ、理由も入力値も見えない。
- Required Fix: route・primary query・secondary query・writeごとの表示と遷移を確定する。
- Suggested Revision: IR57。
- Status: corrected_unverified（IR57。検証はDOC-0.19.0）

### REV18-014
- Severity: MAJOR
- Document: operation-catalog notifications.list、IR19、FR-C08 BR
- Location: 通知一覧の公開範囲
- Problem: カタログは現在のtarget scopeで絞るとし、FR-C08は対象が使えない通知を「利用できません」と表示するとする。一覧から除外するのかマスク表示するのかが矛盾する。
- Why it matters: 件数と未読数が実装で変わり、除外しない実装では閲覧権を失った対象の存在を漏らす。
- Example Failure: 担当を外れた設備の通知が一覧に残り、件名から設備名が見えてしまう。
- Required Fix: 一覧・件数・リンク解決時の扱いを確定する。
- Suggested Revision: IR58: 一覧・total・未読から除外、表示済みリンクだけ「利用できません」。
- Status: corrected_unverified（IR58。検証はDOC-0.19.0）

### REV18-015
- Severity: MAJOR
- Document: D04、正規型DemoTrigger、payments.simulate、IR35
- Location: 決済確定の経路と操作主体
- Problem: 顧客試行の確定がpayments.simulate(confirm)とdemo.trigger(payment)の2経路で可能で、関係が未定義。public routeの/demoから確定した場合、IR35の監査主体（入金確認の実行者）が存在しない。
- Why it matters: 冪等性・版照合・監査の規則が経路ごとに異なり、同じ入金が2経路で扱われる。
- Example Failure: /demoから未ログインで確定すると、監査のactorが空になり制限解除の起動者を追跡できない。
- Required Fix: 経路を1つにし、時計・デモ由来の業務変更の監査主体を定義する。
- Suggested Revision: IR59: DemoTrigger paymentを廃止、システム主体はsystem-demo／actorRoleAtTime=system。
- Status: corrected_unverified（IR59。検証はDOC-0.19.0）

### REV18-016
- Severity: MAJOR
- Document: operation-catalog frontend_execution、NFR-06、D11
- Location: デモ専用操作の識別
- Problem: 顧客が自分の支払いを確定できるpayments.simulateなど、本番に持ち込んではならない操作が業務Repository interfaceと同じmock-serviceとして並び、区別が無い。
- Why it matters: 将来のadapter実装Agentが同じinterfaceを本番APIへ写すと、顧客による入金確定などの重大な権限欠陥になる。
- Example Failure: 本番adapterでpayments.simulate(confirm)をそのまま実装し、未払いの顧客が自分で制限を解除できる。
- Required Fix: デモ専用操作を機械可読に区別し、UI表示とD11の置換先を定める。
- Suggested Revision: IR60: frontend_execution=demo-onlyを11操作に付与、DEMOラベル表示。
- Status: corrected_unverified（IR60。検証はDOC-0.19.0）

### REV18-017
- Severity: MAJOR
- Document: FR-C10完了後の業務状態、DD-C10手順3
- Location: 非RTO契約の表示
- Problem: FR-C10は「RTO以外の契約は『契約なし・一般保守』と表示」、DD-C10は「契約0件のときだけ表示し、非RTO契約は種別・期間・請求を表示」とする。
- Why it matters: 要件と設計が逆の動作を求める。
- Example Failure: 一般保守契約がある顧客に「契約なし」と表示され、請求が見えない。
- Required Fix: 表示条件を一方に統一する。
- Suggested Revision: IR61: 契約0件だけ「契約なし」、FR-C10を修正。
- Status: corrected_unverified（IR61。検証はDOC-0.19.0）

### REV18-018
- Severity: MAJOR
- Document: FR-C02、正規型ACUnit.spaceId、units.save
- Location: 空間未割当の設備
- Problem: FR-C02は「どのスペースにも割り当てられていない設備も分かるようにする」とし、ACUnit.spaceIdはnull可だが、units.saveはspaceIdを必須にしている。未割当の設備を作れず、一覧で絞る条件も無い。
- Why it matters: 要件の表示対象が生成不能で、テストもできない。
- Example Failure: 未割当グループが常に空で、件数を現在ページから数える誤実装が生まれる。
- Required Fix: 保存時のnull許可と、未割当だけを取得する条件を確定する。
- Suggested Revision: IR62: units.save.spaceIdをID|null、units.listにunassignedOnly。
- Status: corrected_unverified（IR62。検証はDOC-0.19.0）

### REV18-019
- Severity: MAJOR
- Document: FR-A01、DD-A01、正規型AdminSummary.energySummary、admin.summary
- Location: 管理ダッシュボードの削減量
- Problem: FR-A01は「削減量の予想」を表示するが、admin.summaryの入力に基準の指定が無く、どの基準で計算するか未定義。
- Why it matters: 削減量が常にnullになる実装と、任意の基準を推測する実装に分かれる。
- Example Failure: ある実装は最初に見つかった基準を使い、別顧客・別境界の基準で削減量を表示する。
- Required Fix: 基準の選び方と、該当なしの表示を確定する。
- Suggested Revision: IR63: 設備集合・境界・分数が一致する最新の基準を自動選択、無ければbaseline_unavailable。
- Status: corrected_unverified（IR63。検証はDOC-0.19.0）

### REV18-020
- Severity: MAJOR
- Document: DD-C09 contactWindow、IR42、D06
- Location: 連絡可能時間の検証と公開範囲
- Problem: 「実連絡先なし」は検証方法が無くテスト不能。業者・技術者への公開可否も未定義で、D06の「顧客連絡先を業者へ返さない」と衝突しうる。
- Why it matters: 自由文に電話番号が入ると外部技術者へ個人情報が渡る。
- Example Failure: 「call 012-345-6789」が保存され、受諾前の業者画面に表示される。
- Required Fix: 入力検証の判定規則と、roleごとの公開範囲を確定する。
- Suggested Revision: IR64: '@'と連続7桁以上の数字を拒否、公開は顧客・HQ・閲覧窓内の担当・受諾済み業者だけ。
- Status: corrected_unverified（IR64。検証はDOC-0.19.0）

### REV18-021
- Severity: MAJOR
- Document: D09音声文法、FR-X02、AT-X02
- Location: 音声の部屋名照合と構文
- Problem: <room>をSpace名とUnit表示名のどちらと照合するか、部屋に設備が複数ある場合、部屋名に「to」を含む場合の構文解釈が未定義。
- Why it matters: AT-X02の「同名の部屋が2件」の期待値が決まらない。
- Example Failure: 「set Living room to 24 degrees」で、ある実装は設備名と照合して0件、別の実装は部屋の先頭設備を勝手に選ぶ。
- Required Fix: 照合対象、候補の作り方、正規表現、scope外の扱いを確定する。
- Suggested Revision: IR65。
- Status: corrected_unverified（IR65。検証はDOC-0.19.0）

### REV18-022
- Severity: MAJOR
- Document: FR-T07、AT-T07-N、D08
- Location: policyの無いAlertの解消と再発
- Problem: AT-T07-Nはevidence=inferredのAlertを「再測定で回復」させるが、D08の自動回復はpolicyのrecoveryThresholdに依存し、policyId=nullのAlertには回復条件が無い。再発時にpreviousAlertIdを付ける「同じ事象」の判定も未定義。
- Why it matters: 受入条件が実行不能で、再発の関連付けが実装ごとに変わる。
- Example Failure: seedのwindow_open Alertが永久に自動解消されずAT-T07-N③が失敗する。
- Required Fix: 自動回復の対象、同一事象キー、未解消中の重複生成を確定しATを修正する。
- Suggested Revision: IR66とAT-T07-Nの修正。
- Status: corrected_unverified（IR66。検証はDOC-0.19.0）

### REV18-023
- Severity: MAJOR
- Document: D05、共通詳細設計§5、DemoTrigger operation
- Location: 機器操作のqueued→running
- Problem: DeviceOperationはqueued→running→succeeded/failedと定義されるが、queuedからrunningへ進む契機が無い。checkのconnecting状態が再現できない。
- Why it matters: IoTのDevice connecting状態（レビュー必須項目）がテストできない。
- Example Failure: operationが永遠にqueuedのまま60秒でTIMEOUTになる実装と、作成時に即runningにする実装に分かれる。
- Required Fix: 開始の契機、再確認の内容、結果イベントを受け付ける状態を確定する。
- Suggested Revision: IR67: 作成1秒後のtickでrunning、checkはconnecting。
- Status: corrected_unverified（IR67。検証はDOC-0.19.0）

### REV18-024
- Severity: MAJOR
- Document: 共通詳細設計§7、FR-C06、FR-A13、DD-A13
- Location: 負の削減量の表示
- Problem: 共通設計は「マイナスは増加と表示」、FR-C06は「20%増加」、FR-A13とAT-A13は「-20kWh・-20%」と表示するとし、同じ値の表示が食い違う。
- Why it matters: 共通のEnergyChartで役割ごとに別の表示規則が必要になり、受入の期待文字列が衝突する。
- Example Failure: A13の画面が「増加 20.0%」を表示してAT-A13-Bの文字列比較が失敗する。
- Required Fix: DTO値と表示formatterを分けて一意にする。
- Suggested Revision: IR68: DTOは符号付き、表示は全roleで「増加 {絶対値}」。
- Status: corrected_unverified（IR68。検証はDOC-0.19.0）

### REV18-025
- Severity: MAJOR
- Document: AT-A06-N、共通詳細設計§5
- Location: 保守案件の状態列
- Problem: AT-A06-Nの期待状態列がassigned→submittedとなり、in_progressを飛ばしている。状態遷移表ではassignedから直接submittedへは進めない。
- Why it matters: 受入条件どおりに実装すると状態遷移の検査を外すことになる。
- Example Failure: テストAgentがin_progressを経ない提出を許可する実装を合格にする。
- Required Fix: 期待状態列を遷移表に合わせる。
- Suggested Revision: AT-A06-Nへin_progressを追加。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-026
- Severity: MAJOR
- Document: fixture-contract.json、検証計画§2
- Location: デモseedの業務データとテスト用の差分適用
- Problem: fixtureにはactorsと設備の所属だけがあり、請求・契約・制限・案件・Alert・能力・測定値などデモの初期業務データが無い。受入Givenを適用する手段（テスト専用の上書きinterface）も無い。
- Why it matters: 実装Agentがseedを自作し、デモの見え方と受入の前提が実装ごとに変わる。
- Example Failure: invoice-overdue-aの金額や期限を実装者が決め、AT-C10-Nと別の値でデモが作られる。
- Required Fix: 初期データの正と、テストの差分適用方法を定義する。
- Suggested Revision: IR69とfixture-contract.json demoSeed。
- Status: corrected_unverified（IR69。検証はDOC-0.19.0）

### REV18-027
- Severity: MAJOR
- Document: D07 members.capacity
- Location: 作業可能時間の「休日」
- Problem: 「休日は空」とあるが、休日が土日なのか祝日（例: 2026-09-16 マレーシアデー）を含むのか未定義。
- Why it matters: 稼働率の期待値が日付によって変わり、テストが実装者の暦に依存する。
- Example Failure: 2026-09-16を休日にした実装と作業日にした実装で稼働率が異なる。
- Required Fix: 休日の定義を確定する。
- Suggested Revision: IR70: 土日だけ、祝日カレンダーなし。
- Status: corrected_unverified（IR70。検証はDOC-0.19.0）

### REV18-028
- Severity: MAJOR
- Document: D07 events.subscribe、正規型ChangeEvent、共通詳細設計§6
- Location: 変更イベントの種別とQuery無効化
- Problem: ChangeEvent.entityTypeとsubscribeのresourcesがstringで、値の一覧もQuery無効化の対応も無い。共通設計は「jobs/units/invoices/restrictionsなど」と曖昧語で済ませている。
- Why it matters: Repository担当とUI担当のAgentが別の名前を使い、更新が画面に反映されない。
- Example Failure: Repositoryが'job'を送り、UIが'jobs'で購読して案件の状態変化が表示されない。
- Required Fix: entityTypeの列挙と、無効化する読取操作の対応表を定義する。
- Suggested Revision: IR71と正規型ChangeEntityType。
- Status: corrected_unverified（IR71。検証はDOC-0.19.0）

### REV18-029
- Severity: MAJOR
- Document: README、各文書末尾の「旧記述より優先」、Agentic SDLC §7
- Location: 規範の優先順位と旧記述の残存
- Problem: 優先順位は各文書末尾の散文だけで、どの論点が「同じ論点」かの判定が読み手任せ。実際にDD-A13の境界文字数（1〜2000）とIR11（1〜500）、baselineKWhの必須性とSR29など、上書き済みの旧記述が本文に残っている。
- Why it matters: 57ファイルを読む実装Agentが旧記述を採用し、Agent間で実装が食い違う。
- Example Failure: UI Agentは境界説明を2000文字まで許可し、Repository Agentは500文字でVALIDATIONにする。
- Required Fix: 優先順位表を定め、置換済みの旧記述を本文から除き、再混入を検出する。
- Suggested Revision: IR72、DD-A13等の修正、validatorの旧句検査。
- Status: corrected_unverified（IR72。検証はDOC-0.19.0）

### REV18-030
- Severity: MAJOR
- Document: tools/check_review_regressions.py、runs/DOC-0.17.0
- Location: 検証器の変異テスト
- Problem: 変異テストが旧baseline（DOC-0.16.0のmanifest文字列）を探して「Mutation target missing」で停止し、現行版で実行不能。0.17.0の引継ぎ記録は静的検証の成功だけを証跡にしている。
- Why it matters: 検証器自体の退行を検出できず、引継ぎ証跡が実態と合わない。
- Example Failure: 検査の一部を誤って削除しても、変異テストが走らないため気づけない。
- Required Fix: 変異対象を現行化し、両方の成功を引継ぎ条件にする。
- Suggested Revision: IR73とcheck_review_regressions.pyの修正。
- Status: corrected_unverified（IR73。検証はDOC-0.19.0）

## MINOR（16件）

### REV18-031
- Severity: MINOR
- Document: DD-C01 summary欄
- Location: 稼働カードの項目
- Problem: summary欄がtotal/online/offline/unknown/alertCountで、SR27のpowerOn/powerOff/powerUnknownが無い。
- Why it matters: 稼働カードを通信状態で作る誤実装が生まれる。
- Example Failure: 「稼働1」の代わりに「online 2」を表示する。
- Required Fix: 欄をSR27/IR51に合わせる。
- Suggested Revision: IR74とDD-C01の修正。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-032
- Severity: MINOR
- Document: DD-C08/DD-T01 severity
- Location: UI値allの送信
- Problem: UIのseverity=allをQueryへどう渡すか未定義で、そのまま送るとD12でVALIDATION。
- Why it matters: 一覧が常にエラーになる実装が生まれる。
- Example Failure: 初期表示でVALIDATION画面になる。
- Required Fix: allは省略と定める。
- Suggested Revision: IR74。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-033
- Severity: MINOR
- Document: DD-A03
- Location: validFromの必須性
- Problem: 表は「条件により必須」、正規型は常に必須。
- Why it matters: フォームの必須表示が型と一致しない。
- Example Failure: validFrom未入力で送信しVALIDATIONになる。
- Required Fix: 常に必須、validUntilは外部技術者だけ必須と定める。
- Suggested Revision: IR74とDD-A03の修正。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-034
- Severity: MINOR
- Document: D10対応環境、DEC-12
- Location: タブレット環境
- Problem: DEC-12はタブレット対応を採用しているが、対応環境にタブレットOSが無い。
- Why it matters: タブレットの検収環境が決まらない。
- Example Failure: iPadで未検証のまま完了扱いになる。
- Required Fix: タブレット環境を追加する。
- Suggested Revision: IR74とD10の修正（iPadOS 17 Safari）。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-035
- Severity: MINOR
- Document: AT-C09-N、AT-P03-N、AT-C06-N
- Location: 受入日時のタイムゾーン
- Problem: 「2026-09-15 10:00〜12:00」などZもoffsetも無い日時がある。
- Why it matters: UTCと現地時刻で期待値が9時間ずれる。
- Example Failure: 希望枠が過去/未来の境界テストが逆転する。
- Required Fix: 表記規則を定める。
- Suggested Revision: IR74と該当ATの修正。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-036
- Severity: MINOR
- Document: commands.create、IR09
- Location: clientのreason
- Problem: technician/adminのreasonは必須だが、clientがreasonを送った場合の扱いが無い。
- Why it matters: 入力検証が実装で分かれる。
- Example Failure: clientのreasonが監査に残る実装と拒否する実装がある。
- Required Fix: clientは送らないと定める。
- Suggested Revision: IR74。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-037
- Severity: MINOR
- Document: DDC-04
- Location: モック読取の待機
- Problem: 「即時〜300msの間で固定の設定」は値が一意でない。
- Why it matters: 性能計測の前提がぶれる。
- Example Failure: 0msで計測してNFR-04を過大評価する。
- Required Fix: 300ms固定と定める。
- Suggested Revision: IR74とDDC-04の修正。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-038
- Severity: MINOR
- Document: DD-A01/A13/P01/T01
- Location: 期間入力の型
- Problem: A01/A13のプリセット有無、P01/T01の日付→Instant変換が未定義。
- Why it matters: 期間の境界が実装で変わる。
- Example Failure: P01の「9月14日」を00:00Zからと解釈し、8時間ずれる。
- Required Fix: プリセットと変換規則を定める。
- Suggested Revision: IR74とDD表の修正。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-039
- Severity: MINOR
- Document: AT-T03-N/E
- Location: 順序判定のキー
- Problem: 「version=3の後に2」とあるが、Measurement.versionはRepository採番で入力できず、RawMeasurementはsequenceを持つ。
- Why it matters: テスト入力が型で表現できない。
- Example Failure: テストAgentが存在しない入力を作る。
- Required Fix: sequenceに修正する。
- Suggested Revision: AT-T03の修正。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-040
- Severity: MINOR
- Document: DemoTrigger device、DD-T12
- Location: evidenceSourceの決定
- Problem: DeviceEvent.evidenceSourceは必須だがDemoTriggerに入力が無く、DD-T12は必須入力としている。
- Why it matters: 組合せの矛盾（tamperをheartbeat根拠で作る等）が生じる。
- Example Failure: power_lostがheartbeat根拠で保存される。
- Required Fix: 種類から導出すると定める。
- Suggested Revision: IR74とDD-T12の修正。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-041
- Severity: MINOR
- Document: Space.kind、AT-C02-B①
- Location: 場所の入れ子規則
- Problem: area/floor/room/spaceの入れ子の順序制約の有無が未定義（ATは階の下に階を許容）。
- Why it matters: 実装者が順序制約を加えてATが失敗する。
- Example Failure: floorの下にfloorを作れずAT-C02-B①が失敗する。
- Required Fix: 制約なしと明記する。
- Suggested Revision: IR74。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-042
- Severity: MINOR
- Document: DD-A02概要
- Location: tenantIdの入力
- Problem: 「tenantIdを入力する」とあり、D14（Session由来、UI入力を信用しない）と矛盾する。
- Why it matters: 他テナントへの登録経路を作る誤実装が生まれる。
- Example Failure: フォームでtenant-bを選べる。
- Required Fix: 入力しないと修正する。
- Suggested Revision: DD-A02の修正。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-043
- Severity: MINOR
- Document: FR-A04、FR-A12
- Location: 権限名
- Problem: 「型番を管理する権限」「環境policyを管理する権限」は定義済みの権限名ではない。
- Why it matters: 権限判定が推測になる。
- Example Failure: 存在しない権限名をfixtureに追加する。
- Required Fix: device.manage／automation.policy.manageと明記する。
- Suggested Revision: FR-A04/A12の修正。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-044
- Severity: MINOR
- Document: NFR-03
- Location: 期限切れのときの再確認
- Problem: 何の期限か、何を再確認するかが書かれていない。
- Why it matters: テスト不能。
- Example Failure: 確認ダイアログを開いたまま版が変わっても送信される。
- Required Fix: 確認中の状態変化時の挙動を定める。
- Suggested Revision: IR74とNFR-03の修正。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-045
- Severity: MINOR
- Document: PrepareDocument §1.3
- Location: 1A/1Bの定義
- Problem: 1A/1Bという段階名が企業原文のPhase 1/2との対応なしに使われている。
- Why it matters: 範囲の議論で段階の意味がずれる。
- Example Failure: 1BをPhase 2（HVAC）と誤解する。
- Required Fix: 対応を明記する。
- Suggested Revision: PrepareDocument §1.3の追記。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-046
- Severity: MINOR
- Document: 画面カタログstates
- Location: 機器状態の列挙
- Problem: 契約・ログイン等、機器を扱わない画面にもconnecting/device-*が列挙されている。
- Why it matters: 不要な状態の実装・試験が発生する。
- Example Failure: 請求画面にdevice-offline表示を作る。
- Required Fix: IoT画面だけに限定する。
- Suggested Revision: IR74とstates列の修正。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

## QUESTION（2件）

### REV18-047
- Severity: QUESTION
- Document: UX-06、NFR-08
- Location: マレー語の文言の作成者・確認者
- Problem: ms辞書の文言を誰が作り誰が確認するか未定義。
- Why it matters: AIの機械翻訳が確認なしで企業デモに出る。
- Example Failure: 業務用語の誤訳が顧客向け画面に表示される。
- Required Fix: 作成と確認の責任を決める。
- Suggested Revision: DEC-40（PROPOSED）: 実装Agentが下書き、企業検収前にBusiness/UI/UXが確認。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

### REV18-048
- Severity: QUESTION
- Document: UX-04/UX-05、PrepareDocument §1.2
- Location: アプリ名・ヘッダー表記
- Problem: アプリ名が未定義で、参考サイト（Aconland）の名称・ロゴを流用するおそれがある。
- Why it matters: 第三者ブランドの誤用。
- Example Failure: ヘッダーに参考サイトの名称が表示される。
- Required Fix: 中立の名称キーを決める。
- Suggested Revision: DEC-41（PROPOSED）: app.name（AC Monitoring Demo / Demo Pemantauan AC）。
- Status: corrected_unverified（IR74。検証はDOC-0.19.0）

