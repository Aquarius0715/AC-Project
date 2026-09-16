# 独立レビュー報告 INDEPENDENT-DOC-0.16.0-FRV（2026-09-16）

対象: DOC-0.16.0 baseline `fe251845…e0b2`（PrepareDocument、要件定義書5、詳細設計書5＋契約4＋カタログ4、UIUX仕様書＋カタログ2）。レビュー主体: 本会話のAI（Fable 5.1）。修正も同じ主体が行ったため、修正後の再確認は**自己再レビュー**であり、独立G1判定ではない（gate-G1.yaml=pending）。アプリ実装・動作試験はnot_run。本番HTTP/認証/DB/実機はDEC-12・D11により1A対象外で、本報告の阻害項目に数えない。

# 1. Executive Review Summary

初回判定: **NOT READY（CRITICAL 2件、MAJOR 8件、MINOR 15件）**。修正後の自己再レビュー: 未解決0件、追跡表64要件OK、静的検証・TypeScript strict検証成功。独立G1は別主体の判定待ち。

重要な問題は次の2件。①制限解除要求（release_requested）の起動経路が入金確認による自動遷移（AT-A08-N/AT-C11-N）と、HQの明示`restrictions.release`（AT-A09-N）の両方で定義され、後者を先行状態から呼ぶとD01の状態不適合CONFLICTになる矛盾。②`demo.advanceClock`で24時間進める受入手順（S03/A09/C12/P02）と、デモ時計で30分のセッション寿命（D09）が衝突し、手順の途中でUNAUTHENTICATEDになる。いずれもIR35/IR36で確定。

MAJORは、受入条件が前提とするtransport障害注入とネットワーク断のRepository挙動、顧客起点案件のdueAt導出、archived資源の可視性、顧客数の母集団、1h/24hプリセット、役割別投影での内部情報（辞退理由・再割当理由・HQ理由・内部費用・監査actor）の非公開規則、顧客向けjobs.eventsのinternalメモ、Device登録時のSensor生成の8件。全てIR37〜IR43で確定した。

# 2. BLOCKER Issues

なし。要件矛盾・権限不明・主要フロー未定義に該当する2件はCRITICALとして扱った（設計の骨格・権限モデル・操作カタログは既に確定しており、局所的な契約追加で解消できるため）。

# 3. CRITICAL Issues

## FRV-001
- Issue ID: FRV-001
- Severity: CRITICAL
- Document: 共通詳細設計 §5、確定契約D03、入出力契約DDC-08 §3/§4、要件 FR-A08/A09/C11/C12
- Location: 支払い・制限の状態表、AT-A08-N③、AT-A09-N④、AT-C11-N④、操作カタログ restrictions.release
- Problem: release_requestedへの遷移が「入金確認で自動」（DDC-08 §3、AT-A08-N）と「HQがrestrictions.releaseで要求」（AT-A09-N）の両方で書かれ、`restrictions.release`の前提状態・冪等性・remove Commandの生成契機が未定義。
- Why it matters: S03の中核フロー。実装者によって「入金でremoveが飛ぶ」「HQが押すまで飛ばない」「release_requestedへのreleaseはCONFLICT」の3通りに分かれる。
- Example Failure: 顧客の入金でrelease_requestedになった後にHQがreleaseを押すとD01⑥でCONFLICT。AT-A09-N④が合格しない、またはAT-C11-N④が合格しない。
- Required Fix: 起動経路（入金/猶予・例外/強制解除/明示release）と冪等性、Command生成の遷移を1箇所で確定。
- Suggested Revision: IR35のとおり。入金確認の同一遷移でrelease_requested＋D03解除評価（applied+online→remove）。`restrictions.release`は未入金かつ猶予・例外なしでFORBIDDEN、release_requestedでは冪等。
- Status: resolved（IR35、common.md状態表、DDC-08 §3、BR-A09、AT-A09-N/AT-C11-N、AT-REV17-001）

## FRV-002
- Issue ID: FRV-002
- Severity: CRITICAL
- Document: 確定契約D04/D09、DDC-04、要件 FR-X01、AT-A09-N/AT-C12-N/AT-P02-E/AT-C04-E
- Location: 「寿命はデモ時計で30分、延長なし」、「実行時刻(executeAfter)に時計を進める」、AT-C04-E③（clockを2026-03-07に設定）
- Problem: 24時間以上の時計ジャンプでセッションが失効する。デモ時計が実時間で進むのか、後退できるのかも未定義（AT-C04-E③はseedより過去へ設定している）。
- Why it matters: 制限・委託期限・DSTの受入手順が全て途中で/loginへ戻り、期待値が成立しない。テストエージェントが「再ログインを挟む」か「advanceClockを別実装にする」かを推測する。
- Example Failure: schedule→advanceClock(+24h)→executeがUNAUTHENTICATED。
- Required Fix: 時計の進行モデル（実時間tick、前方ジャンプ、後退の可否）とジャンプ時のセッション扱いを確定。
- Suggested Revision: IR36のとおり。ジャンプはSession.expiresAtを同じ差分だけずらし寿命を消費しない。後退はreset直後のみ。失効試験はsession_expired trigger。
- Status: resolved（IR36、D09、common.md §6、AT-A09-N、AT-REV17-002）

# 4. MAJOR Issues

## FRV-003
- Severity: MAJOR / Document: 確定契約D01/D04/D10、DDC-03、正規型DemoTrigger、AT-C01-E③・AT-C08-E①・AT-C09-E④・AT-T08-E③・AT-T03-N③
- Problem: 受入条件が「units.listをUNAVAILABLEにする」等の障害注入と通信断を前提にするが、DemoTriggerに注入手段がなく、network(connected=false)時のRepository応答コードも未定義（OFFLINEは機器offline専用）。
- Why it matters: NFR-05・S01/S06の異常系が実装不能・テスト不能。
- Example Failure: テストがモック内部を直接改変する。ネットワーク断をOFFLINEで表示し機器offlineと混同する。
- Required Fix: transport注入イベントの契約と、network断の応答コード・購読・復帰規則。
- Suggested Revision: IR37（DemoTrigger 'transport'、UNAVAILABLE+errors.network_disconnected、購読解除→再snapshot）。
- Status: resolved（IR37、service-contracts.ts、DDC-03/07、AT-REV17-003）

## FRV-004
- Severity: MAJOR / Document: DD-C09、正規型jobs.create、D16、FR-C09
- Problem: 顧客がjobs.createするときのdueAt（省略可の入力）の導出規則が未定義。JobSummary.dueAt、overdue KPI、dueAt sortの基礎値が決まらない。
- Suggested Revision: IR38（client入力はVALIDATION、省略時dueAt=requestedEnd、HQ指定はrequestedEnd以上）。
- Status: resolved（IR38、BR-C09、DD-C09/DD-A06、AT-REV17-004）

## FRV-005
- Severity: MAJOR / Document: D05/D14、DD-A02/DD-C02、query-catalog（archivedフィルターなし）、SR27
- Problem: archived=trueのUnit/Space/Propertyが一覧・KPI分母・候補・個別取得でどう扱われるか未定義。AdminSummary.totalにarchivedを含めるとpowerUnknownが増える。
- Suggested Revision: IR39（無条件除外、HQのasset.manageだけ読取専用取得、他roleはNOT_FOUND）。
- Status: resolved（IR39、BR-A02、AT-A02-N、AT-REV17-005）

## FRV-006
- Severity: MAJOR / Document: FR-A01 BR-A01、DD-A01、D12、正規型Customer/Organization
- Problem: 「稼働中(active)の顧客組織の数」がCustomer.statusかOrganization.statusか、Customer:Organizationの多重度も未定義。
- Suggested Revision: IR40（1対1、両status=activeのCustomer件数）。
- Status: resolved（IR40、BR-A01、DD-A01、AT-REV17-006）

## FRV-007
- Severity: MAJOR / Document: DD-C07（1h/24h/7d）、DD-T03/AT-T03-N（24h）、SR17（today/7d/30dのみ）
- Problem: 1h/24hが移動窓か暦日か未定義。from/toの期待値を決められない。
- Suggested Revision: IR41（UTC分境界のtoから60分/1440分の移動窓、7dはSR17）。
- Status: resolved（IR41、BR-C07/BR-T03、DD-C07/DD-T03、AT-REV17-007）

## FRV-008
- Severity: MAJOR / Document: DDC-02（JobDetail/RestrictionDetail投影）、D06/D14、正規型Offer/Assignment/Restriction.events、members.list
- Problem: 顧客向けJobDetailにOffer（他社の辞退理由）とAssignment.reason（再割当理由）、RestrictionDetail.events（HQのactorId・理由・before/after）、業者向けMembership（permissions/scopes）が投影されるかが未定義。D14はcosts/reportRefsだけを扱う。
- Why it matters: 内部情報の漏えい。FR-X04「担当範囲外のデータは取得できない」と矛盾する実装が生まれる。
- Suggested Revision: IR42（offer=null、assignment.reason=null、events masked、exception.reason=null、technician/contractor costs=[]、contractor向けMembership permissions/scopes=[]）。型: Restriction.exception.reasonをstring|nullへ。
- Status: resolved（IR42、DD-A09入力欄注記、service-contracts.ts、AT-REV17-008）

## FRV-009
- Severity: MAJOR / Document: FR-X07、DD-P07、DDC-09、jobs.events
- Problem: 顧客向けjobs.eventsでvisibility=internalのメモを含むイベントを「除外」するか「note=null」で返すかが未定義。件数・cursorから内部メモの存在が漏れる。
- Suggested Revision: IR42（行ごと除外、totalも除外後）。
- Status: resolved（IR42、BR-P07、AT-REV17-009）

## FRV-010
- Severity: MAJOR / Document: DD-T11、D05、IR11補足、devices.register(sensorTypes)
- Problem: sensorTypesからSensor.unit/staleAfterSeconds/boundaryIdをどう生成するか、能力にないmetricの扱い、初期firmwareVersionが未定義。
- Suggested Revision: IR43（Capability.sensorsの同metric定義から複写、能力外はVALIDATION、空可）。
- Status: resolved（IR43、BR-T11、DD-T11、AT-REV17-010）

# 5. MINOR Issues

| ID | Document / Location | Problem | Fix | Status |
|---|---|---|---|---|
| FRV-011 | 役割別詳細設計 全DD詳細節「この画面が使うサービス境界は…」 | 概要表・操作カタログdesign_idsと不一致（DD-C06/C11/C12/T01/T04/T09/T10/T11/A02/A03/A04/A05/A07/A09/A10/A11/A12/A13/A14/A15/A16）。検証器は概要表しか照合しない | 概要表から48文を再生成し、検証器に「Detail service boundary drift」検査を追加 | resolved |
| FRV-012 | operation-catalog.csv ui_validation列 | telemetry.series/summary(read)に「expectedVersionは更新時必須」、offsets.preview(write)に「scope付きsnapshot」 | 列を修正、検証器で読取のexpectedVersion要求を禁止 | resolved |
| FRV-013 | UX-05 vs component-contracts.csv | AppShell/RoleNavigation/TelemetryValue/TimeSeriesChart/AsyncBoundary/EmptyState/AuditTimeline/NotificationPreviewがCSVに無く名称も不一致 | CSVへ9行追加、UX-05の名称をCSVに揃える | resolved |
| FRV-014 | 画面カタログ、D10「Screen IDはroute単位」 | 未定義route（not-found）の画面が無い | SCR-X-not-found（route `*`）を追加、common.md §2を分離 | resolved |
| FRV-015 | 共通要件 権限マトリクス「通常の空調操作: 施工業者 原則不可」 | 「原則」は曖昧語。例外の有無が不明 | 「不可（commands.create/voice.resolveIntentを付与しない）」へ | resolved |
| FRV-016 | FR-X02「問い合わせ」intent、「依頼」の二義 | 音声のhelpとInquiry（顧客問い合わせ）、Job（依頼）とOffer（HQの依頼）が同じ語 | FR-X02をヘルプへ、共通要件に用語の正規名節、DDC-08 §5に資源正規名を追加 | resolved |
| FRV-017 | AT-C04-N「start=cool 25°C」、AT-C05-N「action=cool 24°C」 | 単一UnitActionと矛盾する表現（mode+温度） | set_temperature 25/24へ | resolved |
| FRV-018 | units.save.installedAt必須 vs ACUnit.installedAt null可、AT-T02-E① | 未登録設備を登録・編集できない | Instant\|nullへ（IR44）、DD-A02欄修正 | resolved |
| FRV-019 | UX-06、D07、AT-C10-N「120.00 MYR」、AT-C01-B「26.0°C」 | Intl locale tag、通貨表記順、温度桁、丸めモードが未定義 | IR44で確定、AT-REV17-011 | resolved |
| FRV-020 | UX-06 i18n、VoicePanel | 翻訳キー欠落時のfallback、権限のないrole（施工業者）の音声パネル表示が未定義 | IR44、AT-REV17-012 | resolved |
| FRV-021 | D10画面優先順位（Missing Requirement Candidate） | 描画時の未捕捉例外の扱いが無い | ErrorBoundaryをUX-05/common §2/IR44に追加、AT-REV17-013 | resolved |
| FRV-022 | Summary.counts | kindに該当しないカウンタの値（0/null）が未定義 | IR44（0、UI非表示） | resolved |
| FRV-023 | DD-A04フィールド表 | 型にあるmodeControl/fanControl/ventilationLevelsの欄が無い | 欄を追加 | resolved |
| FRV-024 | screen-catalog.purpose、PrepareDocument章番号 | purposeが定型文でScreen目的が読めない。章が付録→10へ飛ぶ | FR題名から生成、9./10.へ | resolved |
| FRV-025 | DDC-07 demo.trigger.scenarioId | 意味・制約が未定義 | IR44（1〜64文字のラベル、挙動に影響しない） | resolved |

# 6. Open Questions

1Aの実装引渡しを止める未決事項はない。以下は可逆的なPROPOSED（DEC-19〜24）として実装を進め、企業検収前に確認する。

| ID | 質問 | 採用した仮の答え | 確認先 |
|---|---|---|---|
| Q-1 | 入金確認で解除要求とremove Commandを自動起動してよいか | はい（IR35） | Product Owner / Business |
| Q-2 | 顧客は保守依頼の期限を指定しない（希望枠終了＝期限）でよいか | はい（IR38） | Product Owner |
| Q-3 | 顧客数はCustomerとOrganizationの両方がactiveな件数でよいか | はい（IR40） | Product Owner / Business |
| Q-4 | 1h/24hは移動窓でよいか（今日/7d/30dは暦日のまま） | はい（IR41） | UI/UX |
| Q-5 | 顧客に他社の辞退理由・再割当理由・HQ操作者を見せない、でよいか | はい（IR42） | Security / Product Owner |
| Q-6 | デモ時計のジャンプでセッションを失効させない、でよいか | はい（IR36） | Frontend / Test |

# 7. Cross-document Inconsistencies

| 種別 | 内容 | 状態 |
|---|---|---|
| A. Requirement Missing | 要件にあり設計・UIに無い機能: なし。FR-X07「社内メモを顧客に表示しない」の読取投影規則が設計に無かった（FRV-009） | resolved |
| B. Design Without Requirement | 設計にのみ存在: `restrictions.release`の手動経路がFRの自動遷移と衝突（FRV-001）。SCR-X-not-found追加は既存FR-X04/D01の遂行経路 | resolved |
| C. UI Without Requirement | UX-05のAppShell/AsyncBoundary等は要件NFR-05/FR-X04の遂行部品。契約CSV欠落だけが問題（FRV-013） | resolved |
| D. UI Without API | 「units.listをUNAVAILABLEにする」等の障害注入操作に契約が無かった（FRV-003）。not-found画面に対応routeが無かった（FRV-014） | resolved |
| E. Data Model Gap | dueAt導出（FRV-004）、customerCount母集団（FRV-006）、Sensor生成（FRV-010）、installedAt null（FRV-018）、非該当カウンタ（FRV-022） | resolved |
| F. Terminology Conflict | 問い合わせ/help、依頼/委託、Component名（AuditTimeline vs Timeline等）、「原則不可」 | resolved |
| 契約優先順位 | IR>SR>D>DDC>DD>FRの優先はREADME/各文書の「同じ論点の旧記述より優先する」で表現され、単一の表は無いが矛盾は検出しなかった | 変更なし |

# 8. Missing Requirements

Missing Requirement Candidate（1Aへ追加せず、引継ぎに残す）:

| 候補 | 根拠 | 扱い |
|---|---|---|
| 描画時例外の回復（ErrorBoundary） | NFR-05の状態表示に含まれていなかった | 1Aへ追加した（FRV-021、既存NFR-05の遂行として） |
| 顧客側の通知チャネル設定（email/whatsapp/inAppの希望） | BIZ-22の案内はHQが手段を選ぶ設計。顧客の希望設定は無い | 候補として保留。DEC-12の範囲外 |
| 作業可能時間（members.capacityの分母）の編集 | D07はseed09:00–17:00固定。編集UIが無い | 候補として保留 |
| 完了案件の再開・期限変更 | IR29/IR38は1Aに無いと明記 | 候補として保留 |
| 長時間メモリ容量 | IR18のdeferred候補 | 変更なし |

# 9. Edge Cases Not Defined

| Edge case | 修正前 | 修正後の定義 |
|---|---|---|
| API timeout / HTTP 400〜500 | 1AはHTTP非接続。DomainError 9種で代替（D11で本番表を確定） | NOT DEFINED（1B）。変更なし |
| Network disconnected | Edge Case Undefined（Repository応答が未定義） | IR37: UNAVAILABLE+errors.network_disconnected、購読解除、再snapshot |
| IoT device offline / response timeout | D01 OFFLINE、Command 30秒expiry、D03未配送意図 | 変更なし |
| Invalid / Missing / Stale sensor data | IR12/D07 | 変更なし |
| Duplicate operation | D04冪等キー | 変更なし |
| Multiple browser tabs | D09で対象外・ヘッダー表示 | 変更なし |
| Session expiration | 30分・session_expired trigger。時計ジャンプとの衝突は未定義だった | IR36 |
| Permission changed during operation | D01/IR17/IR24 | 変更なし |
| Empty / Large device list | empty状態、25/100ページング、D10 100設備 | 変更なし |
| Slow network | D10 3000ms/12000ms fixtureの注入手段が無かった | IR37 DELAY |
| Language switch | D09（未確認intent破棄、UTC不変）。翻訳欠落fallbackは未定義だった | IR44 |
| Browser reload / Back button | DEC-07、D13 | 変更なし |
| Concurrent update | version CONFLICT | 変更なし |
| 描画例外 | 未定義 | IR44 ErrorBoundary |
| archived資源 | 未定義 | IR39 |

# 10. Traceability Matrix

[traceability-matrix.csv](traceability-matrix.csv)（64要件、status_before_fix/status_after_fix列）。修正前: CONFLICT 4（FR-A08/A09/C11/X01）、INCOMPLETE 21、OK 39。修正後: OK 64、INCOMPLETE 0、MISSING 0、CONFLICT 0。OKは文書上の対応（Prepare BIZ→FR→DD→Screen→操作→D01/IR37の異常系→AT-N/E/B＋追加AT）があるという意味であり、アプリ試験の合格ではない。

# 11. Undefined Decisions

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| DEC-19 | 入金確認による解除要求の自動起動とreleaseの冪等性（採用済みPROPOSED） | IR35 | S03フローの一意化 | Product Owner / Business |
| DEC-20 | 時計ジャンプでセッション寿命を消費しない | IR36 | 受入手順の成立 | Frontend / Test |
| DEC-21 | 顧客起点案件のdueAt=希望枠終了 | IR38 | 期限KPI・sortの基礎値 | Product Owner |
| DEC-22 | 顧客数の母集団（1対1、両active） | IR40 | KPI定義 | Product Owner / Business |
| DEC-23 | 1h/24h移動窓 | IR41 | 期間期待値 | UI/UX |
| DEC-24 | 役割別投影の非公開項目 | IR42 | 内部情報の保護 | Security / Product Owner |
| IR18 | 長時間メモリ容量 | IR18 | 保証範囲の拡張時 | Product Owner / Frontend |
| OPEN-03〜06/11 | 本番機器・API・認証・連携・終了責任 | D11/Prepare §10 | 本番設計開始前 | Backend / IoT / Security / Business |
| DEC-02/03 | ライブラリの版固定 | design-assumptions | 実装開始時にlockfile | Frontend |

# 12. Implementation Readiness

| 観点 | 判定 | 根拠 |
|---|---|---|
| Requirements completeness | READY | 64要件に出所・設計・画面・操作・AT。FRV-004/006の欠落を補完 |
| Cross-document consistency | CONDITIONALLY READY | FRV-001/002の矛盾を解消。修正後の再確認は自己再レビューであり、別主体のG1判定で確定する |
| UI/UX completeness | READY | 48画面・66 Component契約、状態7種＋IoT5種、not-found追加、Component名の統一 |
| Frontend architecture | READY | Repository/Query/RHF/Navigation/世代/ErrorBoundaryの責任分離 |
| API contract readiness | READY（1Aローカル契約） | 136操作の入出力・認可・版。本番HTTPはNOT READY（D11） |
| Error handling | READY | D01/D04/IR37の障害注入とネットワーク断、DomainError 9種 |
| Authentication / Authorization | READY（1A模擬） | 2権限・scope・投影の非公開規則（IR42）。実認証は対象外 |
| IoT state handling | READY（模擬） | 接続/電源/tamperの独立軸、D03/SR26の回復、IR43のSensor生成 |
| Testability | READY | AT-REV17-001〜015を含む全AT束にfixtureと期待値。時計ジャンプの矛盾を解消 |
| Agentic SDLC handoff readiness | CONDITIONALLY READY | manifest・検証器・受入計画は更新済み。独立G1（別主体）がpending |

# 13. Required Actions Before Implementation

1. 別主体（別エージェントまたは人）がDOC-0.17.0 baselineに対して独立G1判定を行い、gate-G1.yamlをpassed/failedへ更新する（本報告は自己再レビュー）。
2. Product Owner / Security / UI/UXがDEC-19〜24（PROPOSED）を企業検収前に確認する。実装は可逆的な提案として先行してよい。
3. 実装エージェントはmanifest全57ファイルを入力とし、IR35〜44を旧記述より優先して読む。アプリ試験はnot_runから開始し、AT-REV17-001〜015を既存ATに追加する。
4. 本番接続（HTTP/認証/DB/実機）はD11の成果物確定までNOT READYのまま。

検証コマンド: `python3 docs/tools/validate_documents.py`（errors=[]）、`--tsc /private/tmp/ac-typescript-check/package/lib/tsc.js`でTypeScript strict検証。
