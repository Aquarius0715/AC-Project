# 独立レビュー報告 INDEPENDENT-DOC-0.18.0-REV19（2026-09-17）

修正後baseline: `048ffcef25f8463bca33346f5bb1dd8dbb1fdad03b61132d1cfc5d17ebeb1ac2`（[spec-manifest](spec-manifest.json)）。

対象: DOC-0.18.0の作業ツリー（REV18の修正を反映した後に中断し、未コミット・baseline未作成）。対象文書はPrepareDocument、要件定義書5、詳細設計書5＋契約4＋カタログ4＋正規型、UIUX仕様書＋カタログ2、Agentic SDLC付属資料（fixture・検証計画・検証器）。レビュー主体: 本会話のAI（Opus 5）。REV18の修正担当とは別のセッションであり、DOC-0.18.0に対する判定は独立レビューに当たる。修正（IR75〜93、DEC-42〜53）も同じ主体が行ったため、修正後の再確認は**自己再レビュー**であり、DOC-0.19.0の独立G1判定ではない（gate-G1.yaml=pending、別エージェントへ依頼）。アプリ実装・動作試験はnot_run。本番HTTP/認証/DB/実機はDEC-12・D11により1A対象外で、本報告の阻害項目に数えないが、1B着手前の未決事項として§11に列挙する。

## 1. Executive Review Summary

初回判定: **NOT READY（BLOCKER 1件、CRITICAL 3件、MAJOR 16件、MINOR 20件、QUESTION 2件）**。内訳は、初回レビューの36件、修正後の自己再レビュー1回目で見つけたREV19-037、ユーザー指示による受入前提の全件点検（3回目）で見つけたREV19-038〜042。最終の自己再レビュー: 未解決0件、追跡表64要件すべてOK、`validate_documents.py`と`check_review_regressions.py`はともに成功、service-contracts.tsのTypeScript strict検査成功。DEC-44（削減量の予想、案A）とDEC-50（作業窓終了時の扱い）は2026-09-17のユーザー回答で確定。独立G1は別主体の判定待ち。

最重要の問題は次の4件。

1. **REV19-001（BLOCKER）引継ぎbaselineが存在しない。** 索引が指すruns/DOC-0.18.0が無く、受入計画CSVが1行壊れ、両検証器が失敗していた。
2. **REV19-002（CRITICAL）作業窓開始前の技術者画面。** IR49（開始前は読取表示）とIR57（primary queryがFORBIDDENなら全面permission-denied）とSCR-T04/T10のprimary query（units.get）が衝突していた。
3. **REV19-003（CRITICAL）生存シミュレーターが推定値・不良値を実測値へ変える。** FR-X03（P0）の区別とSR27の稼働分類が崩れていた。
4. **REV19-004（CRITICAL）管理ダッシュボードの「削減量の予想」が実質表示されない。** IR63の条件がSR17の可変長プリセットと両立せず、表示文言もIR68と衝突していた。

MAJORは、延長規定の矛盾、C06の負値表示、旧記述検出器の抜け穴、ログイン後の復帰先、再取得中の表示、同意の初期記録、KPI受入Givenとseedの不一致、期限切れOfferへの応答、理由系の文字数、MRV表示項目、作業窓終了時の未保存入力、demoSeedの正規化規則に加え、全件点検で見つけた受入Givenとseedの矛盾（038）、期待値と権限・状態規則の矛盾（039）、排出係数のseed欠落（040）、電力量系の受入データ（041）の16件。すべてIR75〜93で確定した。

## 2. BLOCKER Issues

### REV19-001

- Issue ID: REV19-001
- Severity: BLOCKER
- Document: docs/README.md、04-agentic-sdlc/README.md §8、acceptance-review-018.csv、tools/validate_documents.py、runs/
- Location: 現行baseline・引継ぎ記録
- Problem: READMEとSDLC §8が現行入力として参照するruns/DOC-0.18.0（spec-manifest.json・review.md・traceability-matrix.csv・gate-G1.yaml）が存在しない。acceptance-review-018.csv 15行目（AT-REV18-014）は引用符のないカンマで列が崩れている。validate_documents.pyは10件、check_review_regressions.pyは前提検証で失敗する。0.18.0の修正は自己再レビューを経ないまま中断している。
- Why it matters: IR73は両検証器の成功とmanifestを引継ぎ条件にしている。manifestが無いと実装Agentの入力ファイル集合とspec_baseline_idが決まらず、テスト結果を仕様版へ紐付けられない。
- Example Failure: 実装Agentが未確定の作業ツリーを読み、後の修正で変わった記述を実装する。テスト証跡がどのbaselineに対するものか追跡できない。
- Required Fix: CSVを修正し、0.18.0の実行記録を事実どおり（中断・未検証・baselineなし）に保存する。修正後の版で両検証器を成功させ、manifestとgate記録を作り、索引を現行版へ向ける。
- Suggested Revision: IR75: DOC-0.19.0を現行baselineとし、runs/DOC-0.19.0に review/findings/traceability/static-check/validator-negative-checks/gate-G1(pending)/completion/spec-manifestを保存。runs/DOC-0.18.0はREV18の指摘一覧と『中断・baselineなし』の記録だけを保存する。
- Status: resolved（IR75、AT-REV19-001）

## 3. CRITICAL Issues

### REV19-002

- Issue ID: REV19-002
- Severity: CRITICAL
- Document: review-resolution-contracts.md IR49/IR57、screen-catalog.csv SCR-T04/T10/T02/T07、SR04
- Location: 作業窓開始前の技術者画面
- Problem: IR49はjobIdを伴うunits.getを作業窓の開始前にFORBIDDENとし、同時に『閲覧窓では案件を読取専用で表示』とする。SCR-T04/T10はunits.getをprimary queryに持ち、IR57は『primary queryがFORBIDDENなら画面全体をpermission-denied』とする。
- Why it matters: 技術者の中核フロー（通知→案件を開く→開始時刻まで準備）で、画面を案件の読取表示にするか全面拒否にするかが仕様上2通りに読める。
- Example Failure: T04を開くとjobs.getは成功、units.getがFORBIDDENでpermission-denied全画面になり、案件の予定も開始時刻も見えない。別の実装は案件だけ表示してテストが割れる。
- Required Fix: 作業窓開始前にどのQueryを呼ぶか、どの状態名で何を表示するか、開始時刻到達時の再有効化を1箇所で確定する。
- Suggested Revision: IR76: scheduledStart>nowの間、T04/T10はunits.getを呼ばず（query disabled）状態work-not-startedで案件を読取表示。T02/T07でunits.getがFORBIDDEN(errors.assignment_not_started)ならpermission-deniedではなくwork-not-startedで開始時刻と案件リンクを表示。1秒tickで到達を判定してQueryを有効化。
- Status: resolved（IR76、DEC-42（PROPOSED）、AT-REV19-002）

### REV19-003

- Issue ID: REV19-003
- Severity: CRITICAL
- Document: review-resolution-contracts.md IR45、IR08、SR27、FR-X03、service-contracts.ts RawMeasurement
- Location: 生存シミュレーターの複写
- Problem: IR45は各Sensorの最新Measurementのvalue/unitを複写し、origin=measured・quality=validで保存する。最新がestimated（推定）やsuspect（不良）でも、1分後に実測・正常値へ変わる。さらに/demoから投入するRawMeasurement.sequenceは必須で、毎分増えるsequenceを利用者が知る手段がなく、手入力値が『古いsequence』として無視される。
- Why it matters: FR-X03（P0）は実測・推定・品質の区別を要求し、IR08は推定値やsuspectから古い実測値へ戻ることを禁じる。SR27の稼働分類がmeasured/validを要件にしているため、推定値が稼働ONに化ける。
- Example Failure: /demoでpower=1.0kW・origin=estimatedを投入すると、次の分境界でmeasured/validの複写が作られ、稼働KPIが不明→稼働へ変わる。手動投入したsequence=2が、シミュレーターのsequence=5より古いため反映されない。
- Required Fix: 複写してよい元データの条件、複写しないSensorの扱い、デモ投入時のsequence採番を確定する。
- Suggested Revision: IR77: 最新Measurementがorigin=measured・quality=valid・value非nullのSensorだけ複写し、それ以外は生成しない（stale/unknownへ自然に移る）。demo.trigger telemetryのsequenceは省略可能とし、省略時はRepositoryが最新+1を採番。/demo画面は既定で省略する。
- Status: resolved（IR77、DEC-43（PROPOSED）、AT-REV19-003）

### REV19-004

- Issue ID: REV19-004
- Severity: CRITICAL
- Document: FR-A01、DD-A01、IR63、SR17、IR68、AdminSummary、demoSeed
- Location: 管理ダッシュボードの『削減量の予想』
- Problem: IR63は基準の自動選択条件に『期間の分数が同じ』を課すが、A01の期間プリセットtoday/7d/30d（SR17）は完了分までの可変長で毎分分数が変わるため、一致する基準はほぼ存在しない。seedにも基準が無い。さらにIR63は該当なしの表示を『基準未設定』、IR68は同じnullを『算定不可』とし、IR72の番号優先ではIR68が勝つためAT-REV18-019の期待文言と衝突する。
- Why it matters: FR-A01（P0）の削減量の予想が、デモのほぼ全時間で表示されない。受入の期待文字列も仕様どおりに決まらない。
- Example Failure: HQが/adminをtodayで開くと常に『基準未設定』。AT-REV18-019は『基準未設定』を期待し、IR68どおり実装すると『算定不可』が出て失敗する。
- Required Fix: A01で表示する『予想』の算出方法、基準の選び方、欠測時の扱い、null理由ごとの表示文言を確定し、seedに既定の基準を用意する。
- Suggested Revision: IR78: AdminSummary.energyForecastを追加。対象設備集合と完全一致するdemo_fixed基準（境界ac_input_electricity、最新）を選び、基準の設備・分あたり電力量を期間へ按分した予想基準と、有効slotの平均から外挿した予想実績の差を返す。baseline_unavailable→『基準未設定』、その他null→『算定不可』。energySummaryの削減系はA01ではnull。seedにtenant-a全設備の仮定基準を追加。
- Status: resolved（IR78、DEC-44（ユーザー確定）、AT-REV19-004）

## 4. MAJOR Issues

### REV19-005

- Issue ID: REV19-005
- Severity: MAJOR
- Document: review-resolution-contracts.md IR36
- Location: セッション寿命と延長
- Problem: IR36本文に『利用者操作による延長はない』が残り、IR55・D09・FR-X01の『demoSession.extendで30分延長』と正面から矛盾する。IR72は置換済みの旧記述を削除すると定めるが、検証器は修正契約書を検査対象から除外している。
- Why it matters: セッション処理の担当Agentが、IR36を根拠に延長ダイアログを実装しない、または延長を拒否する。
- Example Failure: SessionExpiryDialogが実装されず、AT-REV18-011が失敗する。
- Required Fix: IR36の文を現行契約（IR55）への参照に書き換え、再混入を検出する。
- Suggested Revision: IR79: IR36の該当文を『利用者操作による延長はIR55のdemoSession.extendだけ』へ置換し、検証器で修正契約書も検査する。
- Status: resolved（IR79、AT-REV19-005）

### REV19-006

- Issue ID: REV19-006
- Severity: MAJOR
- Document: FR-C06 BR-C06・AT-C06-E、DD-C06 境界条件、IR68
- Location: 負の削減量の表示文字列（C06）
- Problem: IR68は全roleで『増加 20.0%』等の共通formatterとし、C06にも適用すると明記する。一方FR-C06のBRは『20%増加』、AT-C06-Eは『増加20%』、DD-C06は『増加率20%』のまま。REV18-024はA13側だけを直した。
- Why it matters: 受入の期待文字列（テストオラクル）が規範と3通りに食い違い、テストAgentがどれで判定するか決まらない。
- Example Failure: C06で『増加 20.0%』を表示する実装がAT-C06-Eの文字列比較で失敗する。
- Required Fix: C06側の記述をIR68の表示規則と同じ文字列へ揃える。
- Suggested Revision: IR80: FR-C06 BR・AT-C06-E・DD-C06を『増加 20.0 kWh／増加 20.0%』（DTOは-20）へ修正し、旧表現を検証器に登録。
- Status: resolved（IR80、AT-REV19-006）

### REV19-007

- Issue ID: REV19-007
- Severity: MAJOR
- Document: tools/validate_documents.py、tools/check_review_regressions.py、IR72
- Location: 置換済み旧記述の検出
- Problem: 旧記述の検出は完全一致の文字列照合だけで、修正契約書review-resolution-contracts.mdを対象外にしている。そのためIR36の旧文、DD-A04『または型番を管理できる権限』（登録句は『管理する権限』）、DD-A12『環境に関するpolicy(方針)を管理する権限』、D09『roomは表示名の完全一致』、DD-A01『顧客組織の数』、C06の負値表記が検出されずに残っている。
- Why it matters: IR72は『validate_documents.pyは置換済みの旧句を検出する』ことを引継ぎの安全弁にしているが、表記揺れで素通りする。静的検証の成功が整合性の証拠にならない。
- Example Failure: 次の修正で旧句を少し言い換えて戻しても検証が成功し、実装Agentが旧仕様を採用する。
- Required Fix: 修正契約書を含めて検査し、引用文脈（『置換』『旧記述』を含む行）だけを除外する。今回見つかった表記揺れを正規表現で登録し、変異テストに加える。
- Suggested Revision: IR81: 旧句検査を正規表現化し、review-resolution-contracts.mdも対象（置換を説明する行は除外）。今回の6件を登録し、check_review_regressions.pyに変異を追加。
- Status: resolved（IR81、AT-REV19-007）

### REV19-008

- Issue ID: REV19-008
- Severity: MAJOR
- Document: screen-catalog.csv SCR-X-login、DDC-07 /login、D08 returnTo、SR11/IR50
- Location: ログイン後の復帰先の受け渡し
- Problem: FR-X01は未認証で保護routeを開いたとき、ログイン後に元の許可routeへ戻すとする。demoSession.signInはreturnToを受けるが、routeガードからログイン画面へreturnToを渡す手段が未定義で、SCR-X-loginのurl_selectionはtabだけ。SR11/IR50により未知キーはURLから除去される。
- Why it matters: ガード担当はURLクエリで渡し、ログイン画面担当は未知キーとして除去する、というAgent間の齟齬がそのまま起きる。ディープリンク（通知リンク等）が機能しない。
- Example Failure: /customer/units/unit-online-rtoを未ログインで開く→/login?returnTo=…へ移動→returnToが除去され、ログイン後は/customerへ移動する。
- Required Fix: 受け渡し方法（URLキー名・エンコード・検証失敗時の扱い）を確定しカタログに登録する。
- Suggested Revision: IR82: ガードは/login?returnTo=<encodeURIComponent(path+search)>へ置換遷移。SCR-X-loginのurl_selectionにreturnToを追加。D08の検証に失敗した値はVALIDATION画面にせず無視して除去し、role homeへ。
- Status: resolved（IR82、DEC-45（PROPOSED）、AT-REV19-008）

### REV19-009

- Issue ID: REV19-009
- Severity: MAJOR
- Document: D10/D13 画面状態、DDC-03、IR71、IR45、UX-06
- Location: 無効化後の再取得中の表示
- Problem: 状態はinitial/loading/empty/successで定義されるが、既に表示中のデータを購読イベントで再取得している間の表示（前データ保持か、skeletonへ戻すか）、再取得失敗時の表示、読み上げの扱いが未定義。IR45により毎分measurement/deviceイベントが発生し、ダッシュボード・設備詳細のQueryが毎分無効化される。
- Why it matters: 実装によって毎分画面がskeletonに戻り、フォーカス位置や読み上げが乱れる。UX-06の『テレメトリー更新のたびに読み上げない』もテストできない。
- Example Failure: 顧客ダッシュボードが1分ごとに点滅し、スクリーンリーダーが毎分『読み込み中』を読み上げる。
- Required Fix: 同一query keyの再取得中・失敗時の表示と、同時刻に届いたイベントの無効化の集約単位を確定する。
- Suggested Revision: IR83: 同一keyの再取得中は表示データを保持してrefreshing表示（aria-busy、live regionなし）。失敗時はデータを保持しstale注記と最終成功時刻・再試行。key変更時だけloading。同じ1秒tick内のイベントによる無効化はkeyごとに1回へ集約。
- Status: resolved（IR83、DEC-46（PROPOSED）、AT-REV19-009）

### REV19-010

- Issue ID: REV19-010
- Severity: MAJOR
- Document: SR02、IR69、fixture-contract.json demoSeed.consents、FR-C05
- Location: 位置情報同意の初期記録
- Problem: SR02は『consents.getは初回からgranted=falseの版付きConsentを返す』とするが、IR69は『demoSeedに無い業務記録をRepositoryが補わない』とし、demoSeed.consentsは空配列。優先順位（IR>SR）ではRepositoryが同意記録を作れず、consents.getの結果が決まらない。
- Why it matters: FR-C05の同意画面の初期表示と、consents.updateの版（expectedVersion）が決まらない。
- Example Failure: ある実装はNOT_FOUNDを返し同意画面がエラーになる。別の実装は暗黙に版1の記録を作り、テストの版番号がずれる。
- Required Fix: 初期同意記録の出所（seed／Membership作成時）を確定する。
- Suggested Revision: IR84: demoSeed.consentsにclient Membershipごとのgranted=false・version=1の記録を置く。members.saveでclient Membershipを新規作成する同一遷移でも同じ記録を作る。consents.getは存在しない場合NOT_FOUND（fixture欠陥）。
- Status: resolved（IR84、DEC-47（PROPOSED）、AT-REV19-010）

### REV19-011

- Issue ID: REV19-011
- Severity: MAJOR
- Document: AT-A01-N/AT-A01-B、IR69、demoSeed
- Location: 受入Givenとデモseedの不一致
- Problem: AT-A01-NのGivenは『tenant-aで電源ON2台・OFF2台・状態不明1台』だが、demoSeedのtenant-a（5台）はON2（unit-online-rto・unit-limited）・OFF1（unit-non-rto）・不明2（unit-offline-rto・unit-other-customer）。IR69は『Givenはseedへの差分として記述』とするが、どの設備をどう変えるかが書かれていない。
- Why it matters: テストAgentが差分を推測し、選んだ設備によって稼働率の分母や遷移先一覧の件数が変わる。
- Example Failure: Agent AはAT-A01-Nの前提を作るためunit-other-customerをOFFにし、Agent Bはunit-offline-rtoをonlineにする。遷移後の一覧の内容が一致しない。
- Required Fix: Givenを具体的なpatchesとして固定し、seedとの整合を検証器で確認する。
- Suggested Revision: IR85: fixture-contract.jsonにacceptancePatches['AT-A01-N']（device-offline-rtoをonline・power 0.0 kWの測定・observedState.power=false）を定義し、AT-A01-N/Bの本文から参照する。KPI系ATはsimulator=falseで開始。
- Status: resolved（IR85、AT-REV19-011）

### REV19-012

- Issue ID: REV19-012
- Severity: MAJOR
- Document: D01、IR23、IR48、SR01、AT-P02-E①、AT-REV18-004
- Location: 期限切れの未応答Offerへの応答
- Problem: offerExpiresAt到達後のjobs.accept/declineの失敗コードが3通りに読める。D01では『担当期限外』はFORBIDDEN（順位4）、scope外はNOT_FOUND（順位3）、AT-P02-E①とAT-REV18-004はCONFLICT。期限後のjobs.getの結果（一覧からは除外）も未定義。
- Why it matters: 同じ操作に対するエラー表示と受入判定が実装者で分かれる。
- Example Failure: ある実装はNOT_FOUNDで『対象が見つかりません』を出し、AT-P02-E①（CONFLICT・再取得案内）が失敗する。
- Required Fix: 応答期限を状態条件として扱うか、期限後の自社Offerへの参照可否、jobs.getの結果を確定する。
- Suggested Revision: IR86: 自社Offerへのaccept/declineは期限後もscope内とし、D01順位6のCONFLICT（messageKey=errors.offer_expired）。jobs.getは期限後NOT_FOUND（受領記録はIR01のみ）。FORBIDDEN（期限外）はaccessValidFrom/Until等の閲覧・作業窓に限る。
- Status: resolved（IR86、DEC-48（PROPOSED）、AT-REV19-012）

### REV19-013

- Issue ID: REV19-013
- Severity: MAJOR
- Document: D12、DD-T07 resolutionReason、DD-A14 reviewComment、DD-A10 reason、DD-P05 reason
- Location: 理由系入力の文字数
- Problem: D12は『reason/resolutionReason/reviewComment等はtrim後1〜1000』とするが、DD-T07（resolutionReason）・DD-A14（reviewComment）・DD-A10（reason）・DD-P05（差戻しreason）は1〜2000。D12の『各DDに規定のない』がどこまで掛かるかも曖昧。
- Why it matters: UI（DD）とRepository（D12）の検証境界が食い違い、1001〜2000文字の入力で画面は通すがサービスは拒否する。境界値テストの期待値が決まらない。
- Example Failure: 1500文字の解消理由を入力すると、フォームは送信を許可し、RepositoryがVALIDATIONを返す。
- Required Fix: 理由系フィールドの上限を一意にし、D12の適用範囲の文言を明確にする。
- Suggested Revision: IR87: 理由系（reason/cancelReason/declineReason/resolutionReason/reviewComment/changeReason/purpose）はDDの記載にかかわらずtrim後1〜1000。本文・メモ・返信（symptom/workText/message/reply/note）は各DD。4つのDDを1〜1000へ修正。
- Status: resolved（IR87、DEC-49（PROPOSED）、AT-REV19-013）

### REV19-014

- Issue ID: REV19-014
- Severity: MAJOR
- Document: DD-A14 Scope 2報告プレビュー、service-contracts.ts MRVPreview/MRVReport、D12
- Location: MRV画面の表示項目名
- Problem: DD-A14は『レポート画面のデータに次の項目を追加』としてreportCategory、siteIds、gridRegion、factorValue、factorUnit、factorYear、factorVersion、boundaryDescription、coverageRatioを列挙するが、正規型には存在せず、既存フィールドとの対応も書かれていない。D12は概念名をDTOの別フィールドにしないとする。
- Why it matters: UI Agentは新しいDTOフィールドを期待し、Repository Agentは正規型どおり返すため、画面に値が出ない（Data Model Gap）。
- Example Failure: gridRegionをDTOから読もうとしてundefinedになり、Scope 2プレビューの係数地域が空欄になる。
- Required Fix: 各表示項目の取得元（正規型のパス、または導出）を固定し、DTOを増やさないことを明記する。
- Suggested Revision: IR88: 対応表を定義（reportCategory→scope='scope_2'の表示、siteIds→conditions.unitIdsの各ACUnit.propertyIdの重複除去、gridRegion→factorSnapshot.region、factorValue→factorSnapshot.kgCO2ePerKWh、factorUnit→固定表示kgCO₂e/kWh、factorYear→factorSnapshot.year、factorVersion→factorRef.version、boundaryDescription→conditions.boundary、coverageRatio→summary.coverage）。
- Status: resolved（IR88、AT-REV19-014）

### REV19-015

- Issue ID: REV19-015
- Severity: MAJOR
- Document: IR49/IR24、FR-T09 BR-T09、D09、DD-T08/T09
- Location: 作業窓の終了と未保存の下書き
- Problem: 作業窓（scheduledEnd）を過ぎると技術者の案件はhistoryだけになり、IR24により表示中のQuery・詳細を破棄する。一方FR-T09は『再取得しても未保存の編集内容を消さない』とし、終了時のdirty値の扱い、事前の予告、in_progressのまま窓が切れた案件をHQ/業者が気付く手段が未定義。
- Why it matters: 点検入力の途中で窓が切れると、予告なく入力を失う（セッション期限にはIR55の予告があるのに、作業窓には無い）。延長（jobs.assign）の判断材料も画面に出ない。
- Example Failure: 技術者が11:55から報告を書き、12:00の窓終了で画面が履歴表示へ切り替わり、未保存の本文が消える。業者の一覧ではin_progressのままで、再割当が必要なことが分からない。
- Required Fix: 終了前の予告、終了時の未保存値の扱いと通知、窓超過の表示条件を確定する。
- Suggested Revision: IR89: 窓終了15分前（デモ時計）にrole=statusの予告を1回表示。終了時はD09の期限時と同じく未保存値を破棄して通知（FR-T09の保持は窓内の再取得に限定）。HQ/業者の一覧・詳細はstatus∈{assigned,in_progress}かつscheduledSlot.endAt<=nowで『作業窓終了・再割当が必要』を表示（既存DTOからの導出）。
- Status: resolved（IR89、DEC-50（ユーザー確定）、AT-REV19-015）

### REV19-036

- Issue ID: REV19-036
- Severity: MAJOR
- Document: fixture-contract.json demoSeed、IR69、service-contracts.ts
- Location: demoSeedから正規DTOへの展開
- Problem: IR69はdemoSeedを初期業務データの正とし、生成時に正規DTO schemaで検証するとするが、seedの行はEntityの共通項目（tenantId/version/createdAt/updatedAt）や必須項目（Device.sensors・targetUnitId・createdByMembershipId、Measurement.eventId/isDemo、Command.correlationId、Alert.evidenceIds、Notification.params、MaintenanceJob.reportRefs/costs等）を省略し、Deviceは正規型に無いsensorMetricsを持つ。展開規則とSensor IDの決め方が未定義。
- Why it matters: 実装Agentごとにseedの補完方法が異なり、Sensor IDや版番号が変わる。受入Givenのpatch（sensor-offline-power等のID参照）が実装によって一致しない。
- Example Failure: ある実装はSensor IDを'sensor-online-rto-power'と生成し、AT-A01-Nのpatch対象'sensor-offline-power'が存在せずfixture例外になる。
- Required Fix: 省略形式から正規DTOへの補完規則を1箇所で定め、推測が必要な値（Sensor ID等）はseedに明示する。
- Suggested Revision: IR91: tenantId・版・作成時刻・null/配列・派生値・各資源固有の既定値の正規化規則を定義。seedのDeviceにsensors（id付き）を明示しsensorMetricsを廃止。acceptancePatchesで存在しないidは新規行として正規化。
- Status: resolved（IR91、AT-REV19-036）

### REV19-038

- Issue ID: REV19-038
- Severity: MAJOR
- Document: 01-requirements（AT-C02-N、AT-C08-N、AT-P01-N、AT-P03-N、AT-P03-R01、AT-P06-N、AT-T11-N、AT-A06-N）、fixture-contract.json demoSeed、IR69
- Location: 受入Givenとdemoseedの矛盾（ユーザー指示の全件点検で検出）
- Problem: 受入Givenの一部がdemoSeedと両立しない。AT-C02-Nは『物件0件』だがcustomer-aは2物件を持つ。AT-C08-Nは未読通知3件を前提にするがseedは2件（1件は制限予告）。AT-P01-Nはseedのjob-contractor-a（assigned）が件数に入り期待値と合わない。AT-P03-N/AT-A06-Nの割当枠はseedのassignment-contractor-a（2026-09-14〜20）と重なり確定重複でCONFLICTになる。AT-P03-R01/AT-P06-Nはcontractor-aの技術者2名を前提にするがseedは1名。AT-T11-Nの担当設備には既に機器が紐付いておりbindがCONFLICTになる。
- Why it matters: IR69はGivenをseedへの差分とするが、差分をどう作っても期待値に届かない、または作り方が一意でない。テストAgentが独自にデータを作り、判定が割れる。
- Example Failure: AT-P03-Nを仕様どおり実行すると、seedの割当と重なるため割当がCONFLICTとなり『Assignment作成』が失敗する。
- Required Fix: 各ATのGivenをseedから一意に構成できる形にし、必要なら固定patchを定義する。
- Suggested Revision: IR92: Givenの解釈規則とacceptancePatches（AT-C08-N、AT-P01-N、AT-P03-R01、AT-P06-N、AT-T11-N等）を定義し、AT-C02-N/P03-N/A06-Nの本文を既存データと重ならない形へ修正。
- Status: resolved（IR92、AT-REV19-038）

### REV19-039

- Issue ID: REV19-039
- Severity: MAJOR
- Document: AT-T08-N、AT-T08-E①、AT-T10-E①、D01、D06、IR24
- Location: 期待値と権限・状態規則の矛盾（全件点検で検出）
- Problem: AT-T08-Nは外注案件の報告を『HQがreturn』するが、外注の品質確認は受託業者（partner.review）でありHQはhq_escalationのときだけ（D06）。AT-T08-E①はrequested・cancelled・on_holdのstartを一律CONFLICTとするが、D01では担当の無い案件はNOT_FOUND（順位3）、取消で担当が終わった案件はFORBIDDEN（順位4）が先に決まる。AT-T10-E①の『制限下限未満の温度』は、技術者の担当設備unit-online-rtoに制限が無く成立しない。
- Why it matters: 受入のオラクルが上位規範と食い違い、正しい実装が不合格になる。
- Example Failure: 未割当のjob-internal-aでjobs.startを呼ぶとNOT_FOUNDになり、AT-T08-E①のCONFLICT期待で不合格になる。
- Required Fix: 失敗コードの決まり方と、制限中の設備を担当するGivenを明確にする。
- Suggested Revision: IR93: 技術者の作業開始・提出の失敗コード表と外注案件の品質確認主体を定義し、AT-T08-N/E、AT-T10-E①（acceptancePatches AT-T10-E.1）を修正。
- Status: resolved（IR93、AT-REV19-039）

### REV19-040

- Issue ID: REV19-040
- Severity: MAJOR
- Document: fixture-contract.json demoSeed、fixture.defaultEmissionFactorId、IR69、SR09、AT-C13-N、AT-A14-N、S05
- Location: 排出係数がデモseedに無い（全件点検で検出）
- Problem: D07/SR09は通常のenergy.summaryでfixture.defaultEmissionFactorId（factor-demo-2026）を使うとするが、demoSeedにfactors節が無く、IR69は『seedに無い業務記録を補わない』とする。係数が見つからないため排出量はnull・qualityWarningsにfactor_missingとなる。
- Why it matters: AT-C13-Nの『実績40kgCO₂e』やMRVのプレビューが成立しない。
- Example Failure: C13画面で排出量が『算定不可』になり、オフセットの申込ボタンが無効になる。
- Required Fix: 既定係数をseedの業務記録として置く。
- Suggested Revision: IR92: demoSeed.factorsにfactor-demo-2026（0.5 kgCO₂e/kWh）を追加し、既定係数の実体と明記。
- Status: resolved（IR92、AT-REV19-040）

### REV19-041

- Issue ID: REV19-041
- Severity: MAJOR
- Document: AT-C06-N/E、AT-C13-N、AT-A13-N/E/B、AT-A14-N、fixture.energy、D07
- Location: 電力量系の受入データ（全件点検で検出）
- Problem: AT-C06-Nは期間2026-09-01〜08（7暦日）で実績80kWh・coverage 100%を期待するが、fixture.energyは[2026-09-14T00:00Z,01:00Z)・80kW×60slotの窓で定義され、両者が一致しない。さらに7日分×1分の有効slotを投入する手段が無く、coverage 100%の前提を作れない。基準値の登録方法も未指定。
- Why it matters: 省エネ比較・排出量・MRVの主要受入が実行不能で、テストAgentが独自の測定データを作る。
- Example Failure: AT-C06-Nのためにテストが7日分の測定を独自の値で作り、別の実装では1時間窓で試験して期待値が一致しない。
- Required Fix: 試験窓をfixture.energyに揃え、測定系列と基準をpatchで一意に作る。
- Suggested Revision: IR92: acceptancePatchesにseries形式の測定生成と共有patch（shared:energy-actual-80）を定義し、C06/C13/A13/A14を同じ窓・設備・基準で記述。検証器で電力量とcoverageを再計算。
- Status: resolved（IR92、AT-REV19-041）

## 5. MINOR Issues

### REV19-016

- Issue ID: REV19-016
- Severity: MINOR
- Document: DD-A01 処理手順2、DD-A04 前提、DD-A12 前提、D09 音声文法、verification.md S03、common.md §2
- Location: 置換済み記述の残存
- Problem: DD-A01『顧客数は、稼働中(active)の顧客組織の数』（IR40）、DD-A04『または型番を管理できる権限』とDD-A12『環境に関するpolicy(方針)を管理する権限』（REV18-043）、D09『roomは表示名の完全一致』（IR65）、S03の『入金を確認する→解除を要求する』（IR35の自動遷移）、common.md §2の/forbidden・未定義route『ホーム画面へ戻す』（IR57のリンク表示）が残る。
- Why it matters: 上位規範と異なる読み方を誘発し、IR72の『置換済みの旧記述を残さない』に違反する。
- Example Failure: S03のE2Eで入金確認後に明示releaseを必須手順として実装し、冪等応答を主経路と誤認する。
- Required Fix: 該当箇所を現行契約への参照へ書き換える。
- Suggested Revision: IR90-1: 各文を修正（IR40/device.manage/automation.policy.manage/Space.name/IR35の自動遷移/リンク表示）。
- Status: resolved（IR90、AT-REV19-016）

### REV19-017

- Issue ID: REV19-017
- Severity: MINOR
- Document: IR64、DD-C09 contactWindow
- Location: 連絡可能時間の入力案内
- Problem: 空白・ハイフン・括弧・'+'を除いて連続7桁以上の数字を拒否する規則は、『0900-1800』のような時刻表記も電話番号として拒否するが、利用者への入力例・案内が未定義。
- Why it matters: 正当な入力が理由の分からないエラーになる。
- Example Failure: 顧客が『Mon-Fri 0900-1800』を入力し、連絡先禁止エラーだけが表示される。
- Required Fix: 判定規則は維持し、入力例と拒否時の文言を定める。
- Suggested Revision: IR90-2: 入力欄にHH:mm形式の例（例: Weekdays 09:00-18:00）を表示し、errors.contact_details_forbiddenの文言に『時刻はHH:mm形式で入力』を含める。
- Status: resolved（IR90、DEC-53（PROPOSED）、AT-REV19-017）

### REV19-018

- Issue ID: REV19-018
- Severity: MINOR
- Document: IR67、D03、D05
- Location: 機器操作の開始時再確認と制限Command
- Problem: IR67の『作成1秒後のtickで排他を再確認』が失敗した場合の結果が未定義。また制限のapply/remove Command（通常Commandではない）と実行中のcheck/firmware操作が重なった場合の扱いが無い。
- Why it matters: IoT機器への同時操作で状態が実装ごとに変わる。
- Example Failure: FW更新running中にrestrictions.executeを行うと、ある実装はCONFLICT、別の実装はそのまま送信する。
- Required Fix: 再確認失敗の状態と、機器操作中の制限Commandの配送扱いを定める。
- Suggested Revision: IR90-3: 再確認失敗はfailed・failureCode=CONFLICT・connection不変。check/firmwareがqueued/runningの設備への制限Commandはdelivery=not_sent・pendingReason=device_operation_runningとしてD03の未配送意図に従い、完了後に明示retry。
- Status: resolved（IR90、DEC-51（PROPOSED）、AT-REV19-018）

### REV19-019

- Issue ID: REV19-019
- Severity: MINOR
- Document: DD-A08 dueAt
- Location: 請求期限の入力制約
- Problem: 『請求作成時より未来(seed遅延のときは専用の扱い)』の『専用の扱い』が未定義。
- Why it matters: 期限超過請求をHQ画面で作れるかが曖昧。
- Example Failure: 実装者が過去日付の作成を許可し、督促・制限のテスト前提が変わる。
- Required Fix: 期限超過の請求の作り方を明記する。
- Suggested Revision: IR90-4: invoices.createのdueAtはnowより後のみ。期限超過はseedまたはdemo.advanceClockで作る。
- Status: resolved（IR90、AT-REV19-019）

### REV19-020

- Issue ID: REV19-020
- Severity: MINOR
- Document: DD-T01 status、DD-P01 status
- Location: 案件状態の絞り込み候補
- Problem: 技術者のstatus候補にrework_requested/on_holdが無く、業者の候補にrework_requested/on_hold/completedが無い。差し戻し案件を一覧で絞り込めない。
- Why it matters: FR-T08の再作業対象を見つける導線が無い。
- Example Failure: 技術者が差し戻された案件を探せず、再提出が遅れる。
- Required Fix: 候補をJobStatusの業務上必要な値へ揃える。
- Suggested Revision: IR90-5: T01はassigned/in_progress/on_hold/submitted/rework_requested/completed/all、P01はoffered/accepted/assigned/in_progress/on_hold/submitted/rework_requested/completed/all。
- Status: resolved（IR90、AT-REV19-020）

### REV19-021

- Issue ID: REV19-021
- Severity: MINOR
- Document: IR71、D10
- Location: 変更イベントによる無効化の対象
- Problem: devices.calibrationsはどのentityTypeでも無効化されない。IR71の『表に無い操作はinvalidateしない』が、D10の書込み成功後の関連Query無効化まで禁じるのか曖昧。
- Why it matters: 校正後に履歴パネルが更新されない。
- Example Failure: T11で校正を保存しても履歴に表示されず、利用者が二重に保存する。
- Required Fix: 表に追記し、購読起点と書込み起点の無効化を区別する。
- Suggested Revision: IR90-6: device_operation行にdevices.calibrationsを追加。IR71は購読イベント起点の規則で、書込み成功後はD10と各DDの『更新の対象になるQuery』も無効化する。
- Status: resolved（IR90、AT-REV19-021）

### REV19-022

- Issue ID: REV19-022
- Severity: MINOR
- Document: AuditView.result、DD-A16 result、FR-A16
- Location: 監査結果pendingの意味
- Problem: 正規型とDD-A16はresultにpendingを持つが、FR-A16は成功・拒否・失敗の3分類で、pendingを記録する条件が未定義。
- Why it matters: 監査画面の絞り込み候補と表示が実装で分かれる。
- Example Failure: Command要求の監査をsuccessとして記録し、その後の失敗と矛盾する。
- Required Fix: pendingを記録する操作と、確定時の追記方法を定める。
- Suggested Revision: IR90-7: 非同期の結果待ち記録を作る操作（commands.create、diagnosticRuns.create、devices.check/updateFirmware、restrictions.execute/retry/release/override、payments.simulate initiate）の受付監査はpending。確定時に同じcorrelationIdでsuccess/failedを追記。FR-A16を4分類へ。
- Status: resolved（IR90、DEC-52（PROPOSED）、AT-REV19-022）

### REV19-023

- Issue ID: REV19-023
- Severity: MINOR
- Document: query-catalog alerts.list/notifications.list default_sort、IR34
- Location: 重大度の並び順
- Problem: 重大度の順位（normal<warning<critical）はjobs.list（IR34）とnotifications（D07）だけに書かれ、alerts.listの『severity desc』は文字列順か順位かが未定義。
- Why it matters: 文字列順ではwarning>normal>criticalになり、緊急が末尾に並ぶ。
- Example Failure: C08のアラート一覧でcriticalが最後に表示される。
- Required Fix: 全操作の重大度比較を順位で定義する。
- Suggested Revision: IR90-8: severityのsort・比較は全操作でnormal<warning<critical。文字列比較を禁止。
- Status: resolved（IR90、AT-REV19-023）

### REV19-024

- Issue ID: REV19-024
- Severity: MINOR
- Document: component-contracts.csv VoicePanel、D10、D13
- Location: 音声パネルのRepository依存
- Problem: D10/D13は共通Componentが表示専用でRepositoryを呼ばないとするが、VoicePanelのapi_dependencyはvoice.resolveIntent・commands.create等を直接持つ。AppShellはShellContainerに分離済み。
- Why it matters: UI Agentが表示部品にデータ取得を埋め込み、テストでモック注入の境界がずれる。
- Example Failure: VoicePanelの単体テストがRepositoryを必要とし、ページごとに挙動が変わる。
- Required Fix: 取得・送信を担うコンテナを定義する。
- Suggested Revision: IR90-9: VoiceContainer（feature hook）がIR09の読取・送信を行い、VoicePanelはpropsとeventだけ。component-contractsに両方を記載。
- Status: resolved（IR90、AT-REV19-024）

### REV19-025

- Issue ID: REV19-025
- Severity: MINOR
- Document: component-contracts.csv、UX-05、DD-C01/A01/P01/T01
- Location: 件数KPIカードの部品契約
- Problem: 稼働/停止/不明や未対応件数などSummary/AdminSummaryの件数を表示する部品の契約が無い。MetricCardはMeasurement専用。
- Why it matters: 4役割のダッシュボードで件数カードが個別に作られ、分母・不明件数・asOf・遷移リンクの表示が揃わない。
- Example Failure: A01だけ分母を表示し、C01は不明件数を省略する。
- Required Fix: 件数KPIカードの契約を追加する。
- Suggested Revision: IR90-10: KpiCard（label, value:number|null, denominator:number|null, unknownCount:number|null, asOf, href:string|null, loading, error）を追加。
- Status: resolved（IR90、AT-REV19-025）

### REV19-026

- Issue ID: REV19-026
- Severity: MINOR
- Document: component-contracts.csv NotificationPanel
- Location: 通知一覧の空表示
- Problem: empty_stateが『未読0件』固定で、unreadOnly=falseで0件のときも未読0件と表示する。
- Why it matters: 全件0件と未読0件を区別できない。
- Example Failure: 通知が1件も無い顧客に『未読0件』と表示される。
- Required Fix: 絞り込み条件で文言を分ける。
- Suggested Revision: IR90-11: unreadOnly=trueは『未読の通知はありません』、falseは『通知はありません』。
- Status: resolved（IR90、AT-REV19-026）

### REV19-027

- Issue ID: REV19-027
- Severity: MINOR
- Document: screen-catalog.csv SCR-X-login/forgot-password/demo/forbidden/not-found、validate_documents.py required_states
- Location: 公開画面の状態列挙
- Problem: データを取得しない公開画面にもpermission-denied/not-found/stale/empty等が一律に列挙され、テストAgentが存在しない状態の試験を作る。
- Why it matters: 不要な実装・試験が発生し、状態網羅の判定が形骸化する。
- Example Failure: /forbidden画面にpermission-denied状態の表示分岐を実装する。
- Required Fix: 公開・非データ画面の状態集合を定義し、検証器の必須状態を画面種別で分ける。
- Suggested Revision: IR90-12: login/forgot-passwordはinitial;loading;success;error;offline、demoはinitial;loading;success;error、forbidden/not-foundはsuccessだけ。
- Status: resolved（IR90、AT-REV19-027）

### REV19-028

- Issue ID: REV19-028
- Severity: MINOR
- Document: UIUXSpecification.md UX-02/UX-06
- Location: 用語『テレメトリー』の誤定義
- Problem: UX-02とUX-06で『テレメトリー(利用状況の記録／利用状況データ)』と説明しているが、本仕様のTelemetryはセンサー測定値（Measurement）。
- Why it matters: UI Agentが利用状況の分析ログと混同し、読み上げ抑止の対象を誤る。
- Example Failure: 測定値更新の読み上げ抑止が実装されず、毎分読み上げられる。
- Required Fix: 正規名に揃える。
- Suggested Revision: IR90-13: 『テレメトリー(センサーの測定値、Measurement)』へ修正。
- Status: resolved（IR90、AT-REV19-028）

### REV19-029

- Issue ID: REV19-029
- Severity: MINOR
- Document: screen-catalog.csv SCR-A13/A11/A12 url_selection
- Location: 選択状態のURL復元
- Problem: C06はunitIds/baselineIdをURLに保持するが、A13は保持せず、A11/A12は選択中のpolicyIdを保持しない。戻る操作で選択が失われる。
- Why it matters: UX-02（選択中の対象はURL）と一致しない。
- Example Failure: A11で方針を開いて設備画面へ移動し戻ると、未選択に戻る。
- Required Fix: url_selectionへ追加する。
- Suggested Revision: IR90-14: SCR-A13にunitIds,baselineId、SCR-A11/A12にpolicyIdを追加。
- Status: resolved（IR90、AT-REV19-029）

### REV19-030

- Issue ID: REV19-030
- Severity: MINOR
- Document: SR03、operation-catalog baselines.list/energy.summary
- Location: 顧客の基準・分析結果の閲覧範囲
- Problem: SR03はinvoice/contractに『全対象Unitを読める場合だけ返す』規則を置くが、複数UnitのEnergyBaseline・OffsetQuote・OffsetRecordの顧客閲覧条件が無い。
- Why it matters: 一部の設備だけ見られる利用者に、他の設備を含む基準値を返すかが決まらない。
- Example Failure: 設備単位scopeの顧客が、他設備の電力量を含む基準を取得できる。
- Required Fix: 全対象Unit条件を適用する。
- Suggested Revision: IR90-15: EnergyBaseline・OffsetQuote・OffsetRecordは全unitIdsが現在scope内の場合だけ返し、一部だけなら一覧から除外・個別NOT_FOUND。
- Status: resolved（IR90、AT-REV19-030）

### REV19-031

- Issue ID: REV19-031
- Severity: MINOR
- Document: DD-A06 フィールド表、DD-A11 フィールド表
- Location: フィールド表の必須表記
- Problem: DD-A06は『unitId / type / dueAt | 必須』だがdueAtはHQだけ任意入力（IR38）。DD-A11はenabled・priorityの行が重複する。
- Why it matters: フォームの必須表示が型・IRと一致しない。
- Example Failure: dueAt未入力で保存できないフォームが作られる。
- Required Fix: 表を修正する。
- Suggested Revision: IR90-16: DD-A06はunitId/typeを必須、dueAtを任意に分割。DD-A11の重複行を削除。
- Status: resolved（IR90、AT-REV19-031）

### REV19-032

- Issue ID: REV19-032
- Severity: MINOR
- Document: DD-P07 recipientRole、DDC-09、notifications.recipients
- Location: 宛先roleと正規Roleの対応
- Problem: DD-P07のrecipientRole（hq/assigned_technician/customer_contact）とnotifications.recipientsのrole（Role enum）の対応が明記されていない。
- Why it matters: 宛先候補の絞り込み値を実装者が推測する。
- Example Failure: customer_contactをcontractorとして送り、候補0件で送信不可になる。
- Required Fix: 対応を明記する。
- Suggested Revision: IR90-17: hq→admin、assigned_technician→technician（当該案件の有効Assignmentの技術者）、customer_contact→client（案件設備の顧客Membership）。
- Status: resolved（IR90、AT-REV19-032）

### REV19-033

- Issue ID: REV19-033
- Severity: MINOR
- Document: IR35、IR59
- Location: 解除起動時の監査主体
- Problem: IR35は『監査のactorは入金確認の実行者』、IR59は『IR35の解除Commandは監査のactorRoleAtTime=system』とし、同じ遷移の監査記録が1件か2件か、それぞれの主体が読み分けられない。
- Why it matters: 監査画面のactor表示とAT-REV18-015の検証が揺れる。
- Example Failure: 顧客の入金確認で制限が解除要求になった監査の実行者がsystemと表示される。
- Required Fix: 記録の単位と主体を分ける。
- Suggested Revision: IR90-18: Restrictionの状態遷移（release_requested）の監査は入金確認等の実行者と当時role、remove Command作成の監査はactorId=system-restriction・actorRoleAtTime=system。同じcorrelationId。
- Status: resolved（IR90、AT-REV19-033）

### REV19-037

- Issue ID: REV19-037
- Severity: MINOR
- Document: D06、DD-P03、FR-P03、IR49、JobSummary.scheduledSlot/assignmentId
- Location: 再割当時の案件の日程とAssignment参照
- Problem: 初回割当ではJob.scheduledSlotを確定すると書かれているが、再割当・作業窓の延長（jobs.assign）でJob.scheduledSlotとJob.assignmentIdを新しいAssignmentへ更新するかが未定義。顧客の確定日程表示、JobSummary.assignmentId、作業窓終了の表示（IR89）がこの値に依存する。
- Why it matters: 延長後も旧枠が表示され、『作業窓終了・再割当が必要』が消えない、または顧客に古い日程が表示される。
- Example Failure: 業者が12:00終了の枠を14:00まで延長しても、一覧のscheduledSlot.endAtが12:00のままで再割当バッジが残る。
- Required Fix: jobs.assign成功時のJobの更新項目を定める。（自己再レビュー1回目で検出）
- Suggested Revision: IR89: jobs.assign成功の同一遷移でJob.assignmentIdとscheduledSlotを新Assignmentへ更新しversion+1、旧Assignmentはrevokedで保持。
- Status: resolved（IR89、AT-REV19-037）

### REV19-042

- Issue ID: REV19-042
- Severity: MINOR
- Document: AT-C01-E①、AT-C03-B③、AT-C05、AT-C07-E/B、AT-C08-E②/B、AT-C09-B、AT-C10-E/B、AT-C12、AT-P02-N、AT-P05-N、AT-T02-E、AT-T04〜06/09-N、AT-T12-N、AT-A05-N、AT-A07、AT-A08-N、AT-A09-R01、AT-A10-B、AT-A11-N、AT-A12-N
- Location: 受入Givenの対象・手順の未指定（全件点検で検出）
- Problem: 対象設備・操作主体・前提状態の作り方が書かれていない受入がある（例: どのtelemetryをnullにするか、どの設備を換気非対応とするか、in_progress案件をどう作るか、どの主体で模擬決済を確定するか）。
- Why it matters: テストAgentが対象を選び、選び方で結果が変わる（例: 温度をnullにしても稼働分類は変わらない）。
- Example Failure: AT-C01-E①で温度の測定をnullにし、『不明1』にならず不合格と判断する。
- Required Fix: 既定の対象と、状態の作り方の規則を定め、本文に対象を明記する。
- Suggested Revision: IR92の既定表・手順規則を定義し、各受入の本文へ対象ID・主体・手順を明記。
- Status: resolved（IR92、AT-REV19-042）

## 6. Open Questions

1Aの実装引渡しを止める未決事項はない。DEC-44とDEC-50はユーザー回答で確定した。他は可逆的なPROPOSEDとして仕様化し、企業検収前に確認する。

### REV19-034

- Issue ID: REV19-034
- Severity: QUESTION
- Document: FR-A01、IR78、DEC-44
- Location: 『削減量の予想』の業務上の定義
- Problem: 企業原文BIZ-23は通常運転との比較と削減の期待を述べるが、ダッシュボードの『予想』を何に基づく推計とするかは業務判断。IR78は按分した仮定基準と有効slot平均からの外挿を採用した（可逆）。
- Why it matters: 予想値の意味を企業が誤解すると、デモで削減効果を保証したように見える。
- Example Failure: HQが『予想 50.0%削減』を実績の削減率と受け取る。
- Required Fix: Product Ownerが算出方法と表示文言を確認する。
- Suggested Revision: DEC-44（PROPOSED）: 予想であること・仮定基準であること・按分であることを常時表示。企業検収前に確認。
- Status: answered（IR78、DEC-44。DEC-44はユーザー確定、HTTP写像は1Bで決定）

### REV19-035

- Issue ID: REV19-035
- Severity: QUESTION
- Document: D11、DDC-03、service-contracts.ts ErrorCode
- Location: 1B接続時のHTTP状態とDomainErrorの対応
- Problem: 1AはHTTP契約を対象外とし、D11が本番必須成果物に『status/error一覧』を挙げる。HTTP 400/401/403/404/409/429/5xx・timeoutをDomainErrorへ写す規則は未定義。
- Why it matters: 1Bのadapter担当が推測で写像すると、UIの回復動作（再試行・入力保持・再ログイン）が変わる。
- Example Failure: HTTP 503をTIMEOUTへ写し、読取の自動再試行が働かない。
- Required Fix: Backend/Frontendが1B設計開始前に写像表を決める。1Aでは決めない。
- Suggested Revision: D11の必須成果物に『HTTP状態・通信例外→DomainError写像表』を明記し、OPEN-04の範囲とする。
- Status: answered（IR90。DEC-44はユーザー確定、HTTP写像は1Bで決定）

| ID | 質問 | 答え | 確認先 |
|---|---|---|---|
| DEC-42（REV19-002） | 作業窓の開始前は技術者画面でunits.get等を呼ばず、状態work-not-startedで案件を読取表示するか | 開始前はjobs.getだけで読取表示（work-not-started） | Security / UI/UX |
| DEC-43（REV19-003） | 生存シミュレーターはmeasured/valid/非nullの最新値だけを複写し、デモ投入のsequenceは省略時に自動採番するか | measured/valid/非nullだけ複写、sequenceは省略時に自動採番 | IoT / Product Owner |
| DEC-44（REV19-004） | 管理ダッシュボードの『削減量の予想』を、対象設備集合と一致する仮定基準の按分と有効slot平均からの外挿で算出し、seedに既定の仮定基準を置くか | **ユーザー確定（案A）**: 仮定基準の按分と有効slot平均の外挿で予想し「予想（按分した仮定基準・デモ）」と表示 | Product Owner / Business |
| DEC-45（REV19-008） | 未認証時のログイン後の復帰先をURLクエリreturnToで受け渡し、不正値は無視するか | /login?returnTo=…で受け渡し、不正値は無視 | Frontend / Security |
| DEC-46（REV19-009） | 再取得中は表示データを保持してaria-busyの更新中表示とし、同じ1秒tickの購読イベントによる無効化をkeyごとに1回へまとめるか | 再取得中はデータ保持＋aria-busy、1秒tickで集約 | UI/UX / Frontend |
| DEC-47（REV19-010） | 位置情報同意の初期記録をseedと顧客Membership作成時に作るか | seedとclient Membership作成時にgranted=false・版1 | Security / Product Owner |
| DEC-48（REV19-012） | 期限切れの未応答Offerへの受諾・辞退をCONFLICT(errors.offer_expired)、案件の個別取得をNOT_FOUNDとするか | CONFLICT（errors.offer_expired）、jobs.getはNOT_FOUND | Product Owner / Security |
| DEC-49（REV19-013） | 理由系入力（reason/resolutionReason/reviewComment等）の上限を1000文字に統一するか | 理由系は1〜1000文字 | UI/UX / Product Owner |
| DEC-50（REV19-015） | 作業窓の終了15分前に予告し、終了時は未保存入力を破棄して通知し、HQ/業者には再割当が必要な表示を出すか | **ユーザー確定（現在の案）**: 15分前に予告、終了時は未保存入力を破棄して通知、HQ/業者に再割当表示 | Product Owner / UI/UX |
| DEC-51（REV19-018） | 機器操作（check/firmware）中の設備への制限Commandを未配送意図として保留するか | 機器操作中の制限Commandは未配送意図として保留 | IoT / Business |
| DEC-52（REV19-022） | 非同期の受付監査をpendingで記録し、確定時に同じcorrelationIdで結果を追記するか | 非同期の受付監査はpending、確定時に追記 | Security / Product Owner |
| DEC-53（REV19-017） | 連絡可能時間の判定規則を維持し、HH:mm形式の入力案内を表示するか | 判定規則は維持しHH:mm形式の案内を表示 | UI/UX / Security |
| REV19-035 | 1B接続時のHTTP状態・通信例外をDomainErrorへどう写すか | 1Aでは決めない（D11の本番必須成果物に追加） | Backend / Frontend |

## 7. Cross-document Inconsistencies

| 種別 | 内容 | 状態 |
|---|---|---|
| A. Requirement Missing | FR-A01「削減量の予想」を設計が実現できない（REV19-004）。FR-X01の復帰先の受け渡し手段が無い（REV19-008）。FR-T09の未保存入力保持が作業窓の終了時に破綻する（REV19-015） | resolved |
| B. Design Without Requirement | 監査結果pendingが要件FR-A16の3分類に無い（REV19-022）。DD-A14が正規型に無い表示フィールドを「追加」と記載（REV19-014） | resolved |
| C. UI Without Requirement | 新たな該当なし。KpiCard・VoiceContainer（REV19-024/025）は既存要件の実現部品として契約を追加 | 変更なし |
| D. UI Without API | SCR-T04/T10が作業窓開始前に取得できないunits.getをprimary queryに持つ（REV19-002）。/demoのtelemetry投入に必要なsequenceを画面が知る手段が無い（REV19-003）。/login復帰先がURLに載らない（REV19-008） | resolved |
| E. Data Model Gap | AdminSummaryに予想値の型が無い（004）。DD-A14の表示項目（014）。demoSeedの省略形式（036）。初期Consent（010）。再割当時のJob.scheduledSlot（037）。既定排出係数がseedに無い（040）。電力量系の受入データ（041） | resolved |
| F. Terminology Conflict | 「テレメトリー」の誤った説明（028）。recipientRoleとRole（032）。「顧客組織の数」とCustomer件数、「roomは表示名」とSpace.name（016） | resolved |
| 受入オラクル | 受入Givenとseedの矛盾（038）、期待値と権限・状態規則の矛盾（039）、対象の未指定（042）、C06の期待文字列（006）、KPIのGiven（011） | resolved（IR80/IR85/IR92/IR93） |
| 規範の優先順位 | IR63とIR68の表示衝突（004）。置換済み旧記述がIR本文に残る（005/007） | resolved |

## 8. Missing Requirements

Missing Requirement Candidate（1Aへ追加せず、引継ぎに残す）:

| 候補 | 根拠 | 扱い |
|---|---|---|
| 技術者から業者/HQへの作業窓延長の依頼 | IR89は窓終了を予告するが、延長はjobs.assignを持つ側の操作だけ | 候補として保留 |
| 顧客側の通知チャネルの希望設定 | 0.17.0から継続 | 候補として保留 |
| 作業可能時間の編集、祝日カレンダー | D07/IR70は月〜金09:00–17:00固定、祝日なし | 候補として保留 |
| 長時間連続稼働のメモリ上限 | IR18でdeferred | 候補として保留 |
| 同一設備の契約期間重複の業務ルール | IR92で1Aは拒否しないと明文化 | 商用ルールは企業検収前に確認 |
| 本番のHTTP状態→DomainError写像、実認証、タブ間同期 | D11/OPEN-04、D09で1A対象外 | 1B設計開始前に決定（§11） |

## 9. Edge Cases Not Defined

「初回」は本レビュー開始時点、「修正後」はDOC-0.19.0。

| Edge case | 定義箇所 | 初回 | 修正後 |
|---|---|---|---|
| API timeout（1試行10秒） | D04、IR37、DDC-03 | 定義済み | 定義済み |
| HTTP 400相当（VALIDATION） | D01順位1/7、DDC-03 | 定義済み | 定義済み |
| HTTP 401相当（UNAUTHENTICATED・期限） | D01順位2、D09、IR55 | 矛盾（IR36とIR55） | 定義済み（IR79） |
| HTTP 403相当（FORBIDDEN） | D01順位4、IR57、IR76、IR93 | 矛盾（作業窓開始前・取消後の開始） | 定義済み（IR76/IR93） |
| HTTP 404相当（NOT_FOUND） | D01順位3、IR57、IR86、IR93 | 未定義（期限切れOffer、未割当案件の開始） | 定義済み（IR86/IR93） |
| HTTP 409相当（CONFLICT） | D01順位5/6、D04 | 定義済み | 定義済み |
| HTTP 429相当（RATE_LIMITED） | D04、IR37 | 定義済み | 定義済み |
| HTTP 500相当（UNAVAILABLE・未知例外） | D01、DDC-03、IR44 | 1A定義済み／1B写像未定義 | 1A定義済み／1B写像はD11（REV19-035） |
| Network disconnected | IR37、DDC-03 | 定義済み | 定義済み |
| IoT device offline | IR47、D03 | 定義済み | 定義済み |
| IoT device response timeout | D04/D05 | 定義済み | 定義済み |
| 機器操作中の制限Command | IR90（REV19-018） | Edge Case Undefined | 定義済み |
| Invalid sensor data | IR12、D07 | 定義済み | 定義済み |
| Missing / estimated sensor data | D07、IR08、IR77 | 矛盾 | 定義済み（IR77） |
| Stale sensor data | D07、SR27、IR45 | 定義済み | 定義済み |
| Duplicate operation | D04、D02 | 定義済み | 定義済み |
| Multiple browser tabs | D09（対象外を常時表示） | 定義済み（対象外） | 定義済み（対象外） |
| Session expiration | D09、IR36、IR55 | 矛盾 | 定義済み（IR79） |
| Permission changed during operation | IR17、IR24、NFR-03 | 定義済み | 定義済み |
| Work window expires during work | IR89 | Edge Case Undefined | 定義済み（DEC-50ユーザー確定） |
| Empty device list | DD-C01、FR-A01、IR78 | 定義済み | 定義済み |
| Large device list | D07、D10、IR18 | 定義済み（100台まで） | 定義済み（100台まで） |
| Slow network | IR37、D10 | 定義済み | 定義済み |
| Background refetch / 定期無効化 | IR83 | Edge Case Undefined | 定義済み |
| Language switch | D09、IR44 | 定義済み | 定義済み |
| Browser reload | DEC-07、D09 | 定義済み | 定義済み |
| Back button | D13、IR34、IR50 | 定義済み | 定義済み |
| Deep link before login | IR82 | Edge Case Undefined | 定義済み |
| Concurrent update | D04、write-version-catalog | 定義済み | 定義済み |
| Reassignment / extension | D06、IR49、IR89 | 未定義（Job.scheduledSlot更新） | 定義済み |
| Overlapping assignment in acceptance data | IR92、acceptancePatches | 矛盾（seedの割当と重複） | 定義済み |

## 10. Traceability Matrix

Statusは修正前（本レビュー開始時）と修正後（DOC-0.19.0）の2列。機械可読版は[traceability-matrix.csv](traceability-matrix.csv)。

| Requirement ID | Requirement | Prepare | Detailed Design | UI/UX | API | Error Handling | Testable | Status（修正前） | Status（修正後） |
|---|---|---|---|---|---|---|---|---|---|
| FR-C01 | 監視ダッシュボード(状況を一目で見る画面) | BIZ-04;BIZ-08 | DD-C01 | SCR-C01 | operation-catalog (Repository) | D01/D04/IR37/IR77/IR83/IR90/IR92 | AT-C01-N;AT-C01-E;AT-C01-B | CONFLICT | OK |
| FR-C02 | 場所の階層管理 | BIZ-07 | DD-C02 | SCR-C02 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-C02-N;AT-C02-E;AT-C02-B | CONFLICT | OK |
| FR-C03 | 遠隔操作(離れた場所からの操作) | BIZ-13 | DD-C03 | SCR-C03 | operation-catalog (Repository) | D01/D04/IR37 | AT-C03-N;AT-C03-E;AT-C03-B | OK | OK |
| FR-C04 | スケジュール運転・帰宅前冷房 | BIZ-14 | DD-C04 | SCR-C04 | operation-catalog (Repository) | D01/D04/IR37 | AT-C04-N;AT-C04-E;AT-C04-B | OK | OK |
| FR-C05 | 同意付きの自動運転 | BIZ-14;BIZ-15;BIZ-17 | DD-C05 | SCR-C04 | operation-catalog (Repository) | D01/D04/IR37/IR84/IR92 | AT-C05-N;AT-C05-E;AT-C05-B | CONFLICT | OK |
| FR-C06 | 電力・電気代の比較 | BIZ-16;BIZ-23 | DD-C06 | SCR-C06 | operation-catalog (Repository) | D01/D04/IR37/IR80/IR90/IR92 | AT-C06-N;AT-C06-E;AT-C06-B | CONFLICT | OK |
| FR-C07 | 空気の状態(空気環境) | BIZ-18;BIZ-19 | DD-C07 | SCR-C07 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-C07-N;AT-C07-E;AT-C07-B | INCOMPLETE | OK |
| FR-C08 | 異常・点検の通知 | BIZ-08;BIZ-09;BIZ-17 | DD-C08 | SCR-C08 | operation-catalog (Repository) | D01/D04/IR37/IR90/IR92 | AT-C08-N;AT-C08-E;AT-C08-B | CONFLICT | OK |
| FR-C09 | 保守の依頼・予約・履歴 | BIZ-12 | DD-C09 | SCR-C09 | operation-catalog (Repository) | D01/D04/IR37/IR89/IR90/IR92 | AT-C09-N;AT-C09-E;AT-C09-B | INCOMPLETE | OK |
| FR-C10 | 契約・請求の確認 | BIZ-21 | DD-C10 | SCR-C10 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-C10-N;AT-C10-E;AT-C10-B | INCOMPLETE | OK |
| FR-C11 | 支払い案内・決済のデモ | BIZ-22 | DD-C11 | SCR-C11 | operation-catalog (Repository) | D01/D04/IR37 | AT-C11-N;AT-C11-E;AT-C11-B | OK | OK |
| FR-C12 | 運転制限の説明 | BIZ-21 | DD-C12 | SCR-C11 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-C12-N;AT-C12-E;AT-C12-B;AT-C12-R01 | INCOMPLETE | OK |
| FR-C13 | 排出量・オフセットへの案内 | BIZ-23;BIZ-24;BIZ-25;BIZ-26 | DD-C13 | SCR-C13 | operation-catalog (Repository) | D01/D04/IR37/IR90/IR92 | AT-C13-N;AT-C13-E;AT-C13-B | CONFLICT | OK |
| FR-P01 | 受託の状況を見るダッシュボード | BIZ-04;BIZ-12 | DD-P01 | SCR-P01 | operation-catalog (Repository) | D01/D04/IR37/IR90/IR92 | AT-P01-N;AT-P01-E;AT-P01-B | CONFLICT | OK |
| FR-P02 | 案件の受諾・辞退 | BIZ-12 | DD-P02 | SCR-P02 | operation-catalog (Repository) | D01/D04/IR37/IR86/IR92 | AT-P02-N;AT-P02-E;AT-P02-B | CONFLICT | OK |
| FR-P03 | 日程と自社技術者の割り当て | BIZ-12 | DD-P03 | SCR-P03 | operation-catalog (Repository) | D01/D04/IR37/IR89/IR92 | AT-P03-N;AT-P03-E;AT-P03-B;AT-P03-R01 | CONFLICT | OK |
| FR-P04 | 対象設備と異常の根拠の閲覧 | BIZ-12 | DD-P04 | SCR-P04 | operation-catalog (Repository) | D01/D04/IR37 | AT-P04-N;AT-P04-E;AT-P04-B | OK | OK |
| FR-P05 | 報告の品質確認・差し戻し | BIZ-12 | DD-P05 | SCR-P05 | operation-catalog (Repository) | D01/D04/IR37/IR87/IR92/IR93 | AT-P05-N;AT-P05-E;AT-P05-B;AT-P05-R01 | CONFLICT | OK |
| FR-P06 | 自社の作業者・稼働状況の閲覧 | BIZ-12 | DD-P06 | SCR-P06 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-P06-N;AT-P06-E;AT-P06-B | CONFLICT | OK |
| FR-P07 | 案件の連絡・履歴 | BIZ-12;BIZ-20 | DD-P07 | SCR-P07 | operation-catalog (Repository) | D01/D04/IR37/IR90 | AT-P07-N;AT-P07-E;AT-P07-B;AT-P07-R01 | INCOMPLETE | OK |
| FR-P08 | 委託・作業期間の境界 | BIZ-12 | DD-P08 | SCR-P01;SCR-P02;SCR-P03;SCR-P04;SCR-P05;SCR-P06;SCR-P07 | operation-catalog (Repository) | D01/D04/IR37/IR86 | AT-P08-N;AT-P08-E;AT-P08-B | CONFLICT | OK |
| FR-T01 | 担当の状況を見るダッシュボード | BIZ-04;BIZ-08 | DD-T01 | SCR-T01 | operation-catalog (Repository) | D01/D04/IR37/IR76/IR90 | AT-T01-N;AT-T01-E;AT-T01-B | CONFLICT | OK |
| FR-T02 | 設備台帳(設備の情報の一覧) | BIZ-06;BIZ-07;BIZ-10 | DD-T02 | SCR-T02 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-T02-N;AT-T02-E;AT-T02-B | INCOMPLETE | OK |
| FR-T03 | 時系列監視(時間の流れで見る監視) | BIZ-08;BIZ-11 | DD-T03 | SCR-T02 | operation-catalog (Repository) | D01/D04/IR37/IR77/IR83 | AT-T03-N;AT-T03-E;AT-T03-B | CONFLICT | OK |
| FR-T04 | 室内機の点検 | BIZ-10 | DD-T04 | SCR-T04 | operation-catalog (Repository) | D01/D04/IR37/IR76/IR92 | AT-T04-N;AT-T04-E;AT-T04-B | CONFLICT | OK |
| FR-T05 | 室外機の点検 | BIZ-10 | DD-T05 | SCR-T04 | operation-catalog (Repository) | D01/D04/IR37 | AT-T05-N;AT-T05-E;AT-T05-B | OK | OK |
| FR-T06 | 電気・制御部分の点検 | BIZ-10 | DD-T06 | SCR-T04 | operation-catalog (Repository) | D01/D04/IR37 | AT-T06-N;AT-T06-E;AT-T06-B | OK | OK |
| FR-T07 | 異常の根拠 | BIZ-08;BIZ-11;BIZ-17 | DD-T07 | SCR-T07 | operation-catalog (Repository) | D01/D04/IR37/IR87 | AT-T07-N;AT-T07-E;AT-T07-B | CONFLICT | OK |
| FR-T08 | 定期点検・故障対応・予防保全 | BIZ-12 | DD-T08 | SCR-T04 | operation-catalog (Repository) | D01/D04/IR37/IR76/IR89/IR93 | AT-T08-N;AT-T08-E;AT-T08-B | CONFLICT | OK |
| FR-T09 | 作業報告 | BIZ-12 | DD-T09 | SCR-T04 | operation-catalog (Repository) | D01/D04/IR37/IR89 | AT-T09-N;AT-T09-E;AT-T09-B | INCOMPLETE | OK |
| FR-T10 | 遠隔診断・試運転 | BIZ-13 | DD-T10 | SCR-T10 | operation-catalog (Repository) | D01/D04/IR37/IR76/IR93 | AT-T10-N;AT-T10-E;AT-T10-B;AT-T10-R01 | CONFLICT | OK |
| FR-T11 | IoT機器のライフサイクル管理 | BIZ-20 | DD-T11 | SCR-T11 | operation-catalog (Repository) | D01/D04/IR37/IR90/IR92 | AT-T11-N;AT-T11-E;AT-T11-B;AT-T11-R01 | CONFLICT | OK |
| FR-T12 | IoT機器の異常 | BIZ-20 | DD-T12 | SCR-T12 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-T12-N;AT-T12-E;AT-T12-B | INCOMPLETE | OK |
| FR-A01 | 全体ダッシュボード | BIZ-04;BIZ-08 | DD-A01 | SCR-A01 | operation-catalog (Repository) | D01/D04/IR37/IR77/IR78/IR83/IR85/IR90 | AT-A01-N;AT-A01-E;AT-A01-B | CONFLICT | OK |
| FR-A02 | 組織・場所・設備台帳 | BIZ-07 | DD-A02 | SCR-A02 | operation-catalog (Repository) | D01/D04/IR37 | AT-A02-N;AT-A02-E;AT-A02-B | OK | OK |
| FR-A03 | 4役割とスコープ管理 | BIZ-04 | DD-A03 | SCR-A03 | operation-catalog (Repository) | D01/D04/IR37 | AT-A03-N;AT-A03-E;AT-A03-B | OK | OK |
| FR-A04 | 型番・IoT能力台帳 | BIZ-06;BIZ-20 | DD-A04 | SCR-A04 | operation-catalog (Repository) | D01/D04/IR37/IR90 | AT-A04-N;AT-A04-E;AT-A04-B | CONFLICT | OK |
| FR-A05 | アラート・通知方針 | BIZ-08;BIZ-11;BIZ-17 | DD-A05 | SCR-A05 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-A05-N;AT-A05-E;AT-A05-B | INCOMPLETE | OK |
| FR-A06 | 保守計画と品質・費用 | BIZ-12 | DD-A06 | SCR-A06 | operation-catalog (Repository) | D01/D04/IR37/IR89/IR90/IR92 | AT-A06-N;AT-A06-E;AT-A06-B | CONFLICT | OK |
| FR-A07 | 契約プラン | BIZ-21 | DD-A07 | SCR-A07 | operation-catalog (Repository) | D01/D04/IR37/IR92 | AT-A07-N;AT-A07-E;AT-A07-B | INCOMPLETE | OK |
| FR-A08 | 請求・入金・督促 | BIZ-21;BIZ-22 | DD-A08 | SCR-A08 | operation-catalog (Repository) | D01/D04/IR37/IR90/IR92 | AT-A08-N;AT-A08-E;AT-A08-B;AT-A08-R01 | INCOMPLETE | OK |
| FR-A09 | 予告・制限・入金後解除 | BIZ-21 | DD-A09 | SCR-A09 | operation-catalog (Repository) | D01/D04/IR37/IR90 | AT-A09-N;AT-A09-E;AT-A09-B;AT-A09-R01 | CONFLICT | OK |
| FR-A10 | 猶予・例外・手動解除・監査 | BIZ-21 | DD-A10 | SCR-A10 | operation-catalog (Repository) | D01/D04/IR37/IR87 | AT-A10-N;AT-A10-E;AT-A10-B | CONFLICT | OK |
| FR-A11 | 自動運転方針 | BIZ-14;BIZ-16;BIZ-17 | DD-A11 | SCR-A11 | operation-catalog (Repository) | D01/D04/IR37/IR90/IR92 | AT-A11-N;AT-A11-E;AT-A11-B;AT-A11-R01 | INCOMPLETE | OK |
| FR-A12 | 空気環境方針 | BIZ-18;BIZ-19 | DD-A12 | SCR-A12 | operation-catalog (Repository) | D01/D04/IR37/IR90/IR92 | AT-A12-N;AT-A12-E;AT-A12-B | CONFLICT | OK |
| FR-A13 | 省エネ分析 | BIZ-23;BIZ-25 | DD-A13 | SCR-A13 | operation-catalog (Repository) | D01/D04/IR37/IR78/IR90/IR92 | AT-A13-N;AT-A13-E;AT-A13-B | CONFLICT | OK |
| FR-A14 | Digital MRVデモ | BIZ-25 | DD-A14 | SCR-A14 | operation-catalog (Repository) | D01/D04/IR37/IR87/IR88/IR92 | AT-A14-N;AT-A14-E;AT-A14-B;AT-A14-R01 | CONFLICT | OK |
| FR-A15 | オフセットデモ | BIZ-24;BIZ-26 | DD-A15 | SCR-A15 | operation-catalog (Repository) | D01/D04/IR37 | AT-A15-N;AT-A15-E;AT-A15-B | OK | OK |
| FR-A16 | 異常操作・監査 | BIZ-20 | DD-A16 | SCR-A16 | operation-catalog (Repository) | D01/D04/IR37/IR90 | AT-A16-N;AT-A16-E;AT-A16-B | INCOMPLETE | OK |
| FR-X01 | サインイン・サインアウト、パスワードの再設定、デモ役割の選択、表示言語の選択 | BIZ-01;BIZ-02 | DD-COMMON | SCR-X-login;SCR-X-forgot-password;SCR-X-settings-preferences | operation-catalog (Repository) | D01/D04/IR37/IR79/IR82/IR90 | AT-X01 | CONFLICT | OK |
| FR-X02 | 選んだ言語で、音声による状態確認・操作・ヘルプ(操作説明)ができる。文字入力でも同じことができる | BIZ-03 | DD-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37/IR90 | AT-X02 | CONFLICT | OK |
| FR-X03 | 重要度、実測値・推定値・点検結果、データの品質、単位を区別して表示する | BIZ-09;BIZ-18 | DD-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37/IR77/IR90 | AT-X03 | CONFLICT | OK |
| FR-X04 | テナント、役割、担当範囲、期間、能力に応じて、見られる・操作できる内容を制限する | BIZ-04 | DD-COMMON | SCR-X-forbidden;SCR-X-not-found | operation-catalog (Repository) | D01/D04/IR37/IR76/IR90 | AT-X04 | CONFLICT | OK |
| FR-X05 | 共有のデモデータ、データのリセット、実際の処理との違いをはっきりさせる | BIZ-05 | DD-COMMON | SCR-X-demo | operation-catalog (Repository) | D01/D04/IR37/IR77/IR91 | AT-X05 | CONFLICT | OK |
| FR-X06 | 対応している機能と、非RTO(RTO契約以外)の扱いを区別する | BIZ-05;BIZ-06;BIZ-12;BIZ-19 | DD-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37 | AT-X06 | OK | OK |
| FR-X07 | 通知・履歴・監査の仕組みを用意する | BIZ-08;BIZ-20;BIZ-22 | DD-COMMON | SCR-X-notifications | operation-catalog (Repository) | D01/D04/IR37/IR82/IR90 | AT-X07 | INCOMPLETE | OK |
| NFR-01 | アクセシビリティ（キーボード・WCAG 2.2 AA目標） | BIZ-05 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37/IR79/IR83/IR90 | AT-NFR01 | CONFLICT | OK |
| NFR-02 | レスポンシブ（360〜1440px・200%拡大） | BIZ-05 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37 | AT-NFR02 | OK | OK |
| NFR-03 | 架空データ・機密非出力・危険操作の確認 | BIZ-05 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37 | AT-NFR03 | OK | OK |
| NFR-04 | 性能目標（200ms/2秒） | BIZ-05 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37 | AT-NFR04 | OK | OK |
| NFR-05 | 状態表示・二重登録防止・入力保持 | BIZ-05 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37/IR76/IR83/IR90 | AT-NFR05 | CONFLICT | OK |
| NFR-06 | 非同期interfaceとモック差替え | BIZ-05 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37/IR90 | AT-NFR06 | INCOMPLETE | OK |
| NFR-07 | 型・lint・build・単体/E2E試験と証跡 | BIZ-05 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37/IR75/IR81/IR91 | AT-NFR07 | INCOMPLETE | OK |
| NFR-08 | 翻訳キー・Intl・UTC保存 | BIZ-02 | DD-COMMON;UX-COMMON | UX-COMMON | operation-catalog (Repository) | D01/D04/IR37 | AT-NFR08 | OK | OK |

修正前: OK 14・INCOMPLETE 16・MISSING 0・CONFLICT 34。修正後: OK 64。

## 11. Undefined Decisions

AIが商用・本番の確定として決めてはいけない事項。1Aでは可逆的なPROPOSEDとして仮決め（DEC-42〜53）し、DEC-44/50はユーザーが1A仕様として確定した。商用・本番の確定は下表の担当が行う。

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| UD-01（DEC-44） | 削減量の予想の算出方法と表示（1Aは案Aで確定）の商用上の扱い | FR-A01、IR78 | 予想値が削減効果の保証と誤解されるおそれ | 1A: ユーザー確定済み / 商用: Product Owner / Business |
| UD-02（DEC-50） | 作業窓終了時の未保存入力の扱い（1Aは現在の案で確定）と延長依頼の運用 | FR-T09、IR89 | 現場作業の入力損失と責任分界 | 1A: ユーザー確定済み / 商用: Product Owner / UI/UX |
| UD-03（DEC-48） | 期限切れの委託依頼への応答の扱い | FR-P02、IR86 | 委託業務の運用ルール | Product Owner / Security |
| UD-04（DEC-49） | 理由・コメント欄の上限文字数 | D12、IR87 | 監査・報告の記載量 | UI/UX / Product Owner |
| UD-05（DEC-51） | 機器操作中の制限適用の優先順位 | IR90、D03 | 実機での制限の遅延と安全性 | IoT / Business |
| UD-06（DEC-52） | 監査記録に保留（pending）を残す運用 | FR-A16、IR90 | 監査の証跡要件 | Security / Product Owner |
| UD-07（DEC-43） | デモ用の生存シミュレーターの値の生成方針 | IR45/IR77 | デモの見え方と品質表示 | IoT / Product Owner |
| UD-08（DEC-42/45/46/47/53） | 作業開始前の表示、ログイン復帰、再取得表示、同意初期値、連絡時間の入力案内 | IR76/82/83/84/90 | 画面の挙動と個人情報の扱い | UI/UX / Security |
| UD-09（REV19-035） | HTTP状態・通信例外からDomainErrorへの写像 | D11、DDC-03 | 1Bのadapterで回復動作が変わる | Backend / Frontend |
| UD-10（OPEN-04） | 本番API・認証・DB・サーバー認可 | D11 | 本番接続の前提 | Backend / Security |
| UD-11（OPEN-03/11） | センサー・機器能力・時間付き操作の実行主体と障害復旧 | D05/D11 | 実機制御の安全性 | IoT / Backend |
| UD-12（OPEN-05/06） | 決済・通知・位置・料金連携、排出係数とMRV制度 | D11、FR-C11/A14 | 外部サービスと制度の選定 | Business / Backend |
| UD-13（OPEN-10） | 施工業者を独立した第4役割とすること | PrepareDocument §7 | 役割と責任分界 | Product Owner / Business |
| UD-14（IR70/IR92） | 祝日カレンダー、同一設備の契約期間重複の商用ルール | IR70、IR92 | 稼働率・契約管理 | Business |

## 12. Implementation Readiness

| 観点 | 初回判定 | 修正後（自己再レビュー） | 根拠 |
|---|---|---|---|
| Requirements completeness | CONDITIONALLY READY | READY | FR-A01の予想（ユーザー確定）・FR-X01の復帰先・FR-A16の結果分類を補完 |
| Cross-document consistency | NOT READY | READY | IR36/IR55、IR63/IR68、C06表示、旧記述残存を解消し検出器を拡張 |
| UI/UX completeness | CONDITIONALLY READY | READY | work-not-started・refreshing・公開画面の状態集合・KpiCard・VoiceContainer |
| Frontend architecture | CONDITIONALLY READY | READY | 表示ComponentとContainerの分離、無効化の集約 |
| API contract readiness | CONDITIONALLY READY | READY（1AのRepository契約）／本番HTTPはNOT READY（D11、対象外） | EnergyForecast型、sequence省略、TypeScript strict成功 |
| Error handling | CONDITIONALLY READY | READY | 期限切れOffer、作業窓開始前、取消後の開始、再取得失敗、機器操作との競合（IR76/83/86/90/93） |
| Authentication / Authorization | CONDITIONALLY READY | READY | returnToの検証、work-not-startedと権限拒否の区別、外注の品質確認主体、顧客の基準閲覧範囲 |
| IoT state handling | NOT READY | READY | シミュレーターの複写条件、機器操作中の制限Command |
| Testability | NOT READY | READY | 受入Givenの解釈規則とacceptancePatches、seed正規化と既定係数、期待文字列の統一、AT-REV19-001〜042（IR80/85/91/92/93） |
| Agentic SDLC handoff readiness | NOT READY | READY（文書・baseline・検証器の条件）／独立G1はpending | spec-manifest、両検証器成功、runs記録。G1の判定は別主体へ依頼 |

## 13. Required Actions Before Implementation

1. 別主体（別エージェント）がDOC-0.19.0のspec-manifestに対して独立G1を判定し記録する。本報告の修正後判定は自己再レビューであり代用しない。
2. 実装Agentは[spec-manifest.json](spec-manifest.json)の全ファイルだけを入力とし、DOC-0.18.0以前の記録を使わない。仕様を変更した場合はmanifestを再生成し、両検証器を成功させる（IR73/IR75）。
3. 受入の前提はIR92の解釈規則・acceptancePatches・IR69のpatchesで作り、Givenに書かれていない値を推測で作らない。demoSeedはIR91で正規化する。
4. DEC-44/50は1A仕様として確定済み。その他のDEC-42〜53はPROPOSEDとして実装を進め、企業検収前に担当が確認する。DEC-44の予想値を削減効果の保証として説明しない。
5. 1B（本番API・実機・決済）の設計開始前に§11のUD-09〜UD-12を決定し、D11の必須成果物を作成する。

## 付録 自己再レビューの記録

- 1回目（修正直後）: 再割当・延長時のJob.scheduledSlot/assignmentId更新規則の欠落を検出（REV19-037、IR89）。IR76の開始時刻の取得元とIR90（REV19-018）の再確認対象を明確化。AT-REV19-018/021/022を実行可能な手順へ修正。
- 2回目: 修正した契約・カタログ・fixture・受入計画・追跡表・索引と検証計画（§1〜§11）を照合。検証計画§2の時計の表現とS08の状態列を明確化（規範の変更なし）。未解決0件。
- 3回目（ユーザー指示による受入前提の全件点検）: 要件定義書の受入（N/E/B・SRC・R01）とS01〜S08の前提をdemoSeedと突き合わせ、seedとの矛盾（REV19-038）、期待値と規則の矛盾（039）、既定係数の欠落（040）、電力量系データの作成手段（041）、対象の未指定（042）を検出。IR92/IR93、acceptancePatches（19キー）、seedの係数を追加し、本文を修正。検証器にpatchの参照・適用・電力量とcoverageの再計算を追加し、変異テストを62件に拡張（初回実行で検査語の重複による検出漏れ1件を見つけ、検査語を修正して全件検出）。未解決0件。
