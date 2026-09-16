# 1. Executive Review Summary

**実装Agentへの一括引渡しはNOT READY。全件OKではない。**

対象はPrepare、共通・4役割の要件と詳細設計、UI/UXおよび正規契約。レビュー入力はDOC-0.15.0（baseline `f11600bc8735d2a10797b418bb13da76601b8021184a7d4cef6c2a70d355dd5f`）、修正後はDOC-0.16.0。既存の未コミット変更を保持し、旧runを変更せず新しい判定を記録した。

7件を指摘し、5件を修正・自己再確認した。未解決は権限粒度の矛盾1件（BLOCKER）と状態sort順の未定義1件（MAJOR）。既存仕様だけではどちらの解釈も排除できないため、ユーザーに2件の選択を依頼した。回答を受けていない選択肢は採用していない。

文書の対象は**1Aフロントエンドモック**。実HTTP/API・DB・実認証・実機接続の契約はD11により対象外であり、本番はNOT READY。これらの未定義を隠さず、同時に合意済み1Aへ無断で追加しない。機器の実際の安全性、商用支払い制限、医学的・法的な適合性は評価していない。

静的検証は64要件・136操作・47画面・57Componentを確認し、TypeScript strict検証も成功。検証器の変異検査は30/30を検出。ただしアプリ未実装で、動作・性能・a11y試験はnot_run。参照整合検査は全動作の正しさを証明しない。今回の修正者と再確認者は同じ`/root`であり、**修正後の別主体による独立G1は未実施**。旧版の独立合格を本版へ引き継がない。

レビュー観点: 要件のActor/開始条件/入力/出力/成功・失敗条件は各FR詳細、操作の権限・検証は操作/版カタログとD01、UI状態は画面・ComponentカタログとD10/D13へ分散している。単独の役割別DDだけでは実装できず、manifest内の共通契約を含む一式が必須。分散そのものを欠落と誤認せず、以下は照合後にも残った不整合である。

# 2. BLOCKER Issues

## REV-001 — 権限を2種類にするか4種類にするかが矛盾

- **Severity:** BLOCKER
- **Document:** 要件定義admin、詳細設計admin、正規DTO、操作カタログ
- **Location:** FR-A10機能一覧行／FR-A10利用開始条件／DD-A10／Permission／restrictions.*.authorization
- **Problem:** 要件一覧は猶予・例外・取消・手動解除を「それぞれ独立した権限」とする。同じ要件の詳細と設計は前3操作をrestriction.manage、強制解除をrestriction.overrideにまとめる。4操作の独立制御を型で表現できない。
- **Why it matters:** 権限は表示上の好みではなく操作可能範囲であり、要件か設計のどちらかを無断で選ぶと認可仕様を変更する。
- **Example Failure:** 猶予だけを担当するHQへrestriction.manageを与えた結果、例外・取消も実行可能になる。4権限を実装したAgentの結果は現行fixture・型・画面ガードと不一致になる。
- **Required Fix:** 最終判断者が2権限／4権限を選び、FR一覧・詳細・型・操作・UI・拒否試験を同時更新する。
- **Suggested Revision:** 2権限採用時は「猶予・例外・取消はrestriction.manage、強制解除はrestriction.overrideで認可する」。4権限の場合は個別permission名・既存manageとの関係・fixture割当も確定する。**現時点では未採用。**
- **修正・再確認:** 未解決。DEC-17はpending_user。

# 3. CRITICAL Issues

## REV-004 — 監査検索の他テナント判定が存在確認につながる

- **Severity:** CRITICAL
- **Document:** 要件定義admin、共通契約、詳細設計admin
- **Location:** AT-A16-E①／FR-A16境界条件／DD-A16／SR03一覧認可
- **Problem:** 他テナントのcorrelationIdを検索するとNOT_FOUNDという受入条件があるが、一覧は認可済み集合へfilterして返す契約。相関IDは個別資源のgetではなく一覧検索条件である。
- **Why it matters:** 別テナントに存在する相関IDだけエラー、不存在なら正常0件という実装は、認可範囲外の存在を判別する分岐になる。Test Agentが安全な空集合実装を誤って失敗とする。
- **Example Failure:** 同じ構文のIDを切り替えると、外部の既知相関IDだけNOT_FOUNDになる。監査データの存在を検索者へ漏らす。
- **Required Fix:** 認可後検索を徹底し、別テナントIDと不存在IDで同じ成功空集合を期待する。個別getのNOT_FOUNDと分離する。
- **Suggested Revision:** 「audit.listは認可済み集合を検索し、別テナント／不存在のcorrelationIdともitems=[]、total=0、nextCursor=nullを返す」。
- **修正・再確認:** 修正済み。FR-A16、AT-A16-E、DD-A16、IR33を一致させた。アプリでの情報非開示試験は未実行。

# 4. MAJOR Issues

## REV-002 — 案件一覧と集計のunitIdsが一致しない

- **Severity:** MAJOR
- **Document:** 詳細設計・Queryカタログ・UI/UX
- **Location:** IR26／query-catalog jobs.listとsummaries.get／P01・T01
- **Problem:** IR26はunitIdsを保持し一覧と集計へ同じ条件を渡すが、jobs.listのallowlistにunitIdsがない。summaries.getにはある。未知filterはD12によりVALIDATIONになる。
- **Why it matters:** 一覧とKPIを同じ設備集合で取得できない。Agentがfilterを削除すると集計範囲が広がる。
- **Example Failure:** unitIds=[U1]でKPIは1件になるが、一覧はVALIDATION、または条件を捨てた実装でU2も表示する。
- **Required Fix:** 一覧に同じ条件を定義し、公開projection後の比較・空配列・unitIdとのANDをそろえる。
- **Suggested Revision:** 「jobs.listのunitIdsは公開summary.unitIdに照合し、空配列は0件、offer/historyは不一致。非公開の元Jobで判定しない」。
- **修正・再確認:** 修正済み。IR32・Queryカタログ・共通UIUXを更新。検証器で一覧／集計の共通filter集合を照合。

## REV-003 — 監査画面の機器履歴と関連参照の取得経路が欠落

- **Severity:** MAJOR
- **Document:** 詳細設計admin、画面・Component・操作カタログ
- **Location:** DD-A16／SCR-A16／devices.events({id,query})／commands.get.authorization
- **Problem:** A16にはdevices.eventsがあるが必須deviceIdの候補取得・選択・URL復元がない。またaudit.readだけを前提にCommand/Job/Restrictionとの照合を求め、各資源の読取権限とリンク解決の扱いが不明。audit.list自体もsecondary扱いでprimaryがnone。
- **Why it matters:** 対象IDを推測して呼び出す、無権限readを要求する、監査取得失敗を正常画面扱いする実装が生まれる。
- **Example Failure:** 監査ページ初回からdevices.eventsへundefinedを渡す。audit-only HQでCommand照合がFORBIDDENとなり監査全体が閲覧不能になる。
- **Required Fix:** 既存devices.list→明示選択→devices.eventsを定義し、監査必須Queryと補助パネルを分離。関連リンクは既存の個別read権限で有効化する。
- **Suggested Revision:** IR33に取得順、0件／失敗／無効URL／Back、個別資源リンクと権限を定義。audit.readを他資源への包括権限にしない。
- **修正・再確認:** 修正済み。DD、画面、Component、操作カタログを更新。再確認でCommandリンク解決のcommands.get依存もカタログへ追加した。新しい業務操作・permissionは追加していない。

## REV-005 — 案件statusの比較順がNOT DEFINED

- **Severity:** MAJOR
- **Document:** 詳細設計・Queryカタログ・UI/UX
- **Location:** D07／D12／D15／IR23／jobs.list.allowed_sort=status
- **Problem:** asc/desc・null末尾・同値ID順はあるが、異なるJobStatus同士を比較する順序がない。TypeScript unionの記載順はsort契約ではない。
- **Why it matters:** 業務順、英語コード順、翻訳ラベル順のいずれも実装でき、言語切替とページングの期待結果を一意に書けない。
- **Example Failure:** accepted・requested・completedの3件がAgentごとに異なる順番になる。マレー語へ切替時にラベルsortを使う実装だけ並び順が変わる。
- **Required Fix:** 比較する固定順序を選び、降順・安定ID・offer/historyの公開statusにも適用することを確定する。
- **Suggested Revision:** 「statusは［決定した全10状態の順序］で比較する。descは逆順、同順位はjobId asc。翻訳ラベルでは比較しない」。**順序は未採用。**
- **修正・再確認:** 未解決。DEC-18はpending_user。

# 5. MINOR Issues

## REV-006 — Pageの件数不明表示と必須number型が矛盾

- **Severity:** MINOR
- **Document:** フロントエンド入出力契約、正規DTO
- **Location:** DDC-01 Page<T>／Page.total:number／D07・SR14
- **Problem:** 「totalが不明なとき」の表示を説明する一方、DTOは必須number、モックはsnapshotの認可済み全件数を計算する。未知値を表す値がなく、未取得と取得済み0件を混同する余地がある。
- **Why it matters:** -1、null、undefinedなど未定義の値をAgentが追加する。
- **Example Failure:** total=-1を返して0件表示やページ数計算を壊す。
- **Required Fix:** 既存の正確なsnapshot件数へ本文を一致させる。
- **Suggested Revision:** 「totalは認可・投影・filter適用後の非負整数。未取得はinitial/loadingでありPageの件数不明状態ではない」。
- **修正・再確認:** 修正済み。型変更なし。0件と未取得を区別する。

## REV-007 — 現行版の独立レビュー状態が本文と証跡で異なる

- **Severity:** MINOR
- **Document:** strict-review-contracts、索引、G1証跡
- **Location:** strict-review-contracts冒頭／DOC-0.15.0 gate-G1
- **Problem:** 現行バージョンを付けた本文に「独立再レビューは未実施のためG1はpending」とあり、当該旧baselineの実際のgateはpassed。歴史説明と現在状態の区別がない。
- **Why it matters:** Orchestration Agentが本文かgateのどちらかを選ぶことになる。修正後にも旧passedを使う危険がある。
- **Example Failure:** 旧合格を修正後の承認として流用する、または旧版レビューを無駄に再開する。
- **Required Fix:** 歴史記述であることを明示し、現在の判定先をmanifestに束縛したgateへ一本化する。
- **Suggested Revision:** 「この段落は0.9.0時点の経緯。現行G1は索引が示すbaselineのgate記録で判断する」。
- **修正・再確認:** 修正済み。旧runは保存。0.16.0は別gateでpendingとし、自己修正確認を独立合格に置換していない。

# 6. Open Questions

重複指摘は作らず、未解決の問題IDと結び付ける。

| Decision | Issue | 質問 | 状態 |
|---|---|---|---|
| DEC-17 | REV-001 | 制限の操作権限は2種類か4種類か | pending_user／BLOCKER |
| DEC-18 | REV-005 | 案件状態順は明示した業務順かASCII順か | pending_user／MAJOR |

回答待ちを「黙示承認」「推奨案採用」に変更していない。決定まで該当実装・受入期待値を確定できない。

# 7. Cross-document Inconsistencies

| 分類 | 経路・具体的問題 | 判定 |
|---|---|---|
| A Requirement Missing | FR-A10の「各操作独立権限」を型・設計が表現しない | REV-001未解決 |
| B Design Without Requirement | 新規の業務機能追加は今回検出せず。補助readは既存FRの遂行経路 | IR32/33は範囲追加でない |
| C UI Without Requirement | 新規の利用者機能追加は今回検出せず。実User CRUD・実音声・実取引は対象外のまま | 範囲維持 |
| D UI Without API | A16機器履歴の必須ID取得経路なし | REV-003修正済み。ここでAPIはローカルRepository |
| E Data Model Gap | Page不明件数を表す型なし | REV-006修正済み |
| F Terminology Conflict | 「状態順」の意味と比較対象が未定義 | REV-005未解決 |
| 条件不一致 | 一覧とKPIのunitIds | REV-002修正済み |
| 期待結果不一致 | 認可済み一覧と相関IDのNOT_FOUND | REV-004修正済み |
| 工程状態不一致 | 文書のpendingと旧baselineのpassed | REV-007修正済み |

ACUnit（設備）とDevice（IoT機器）は別モデルであり同義語に統合しない。requested/acknowledgedはCommand、requested/completedはJob、paidはInvoiceで、同じUI状態へまとめない。D14の概念モデル略称より正規DTOを用いる規則は既存。

# 8. Missing Requirements

既存要件の不足はREV-001/003/005を参照。機能一覧に列挙された温湿度・CO₂・電力、通知、home/officeと場所階層、4役割、言語、音声デモは対象文書に対応先がある。実装済みとは判定しない。

**Missing Requirement Candidate（現行1Aの必須要件へ追加しない）:**

| ID | 候補 | 理由・扱い | 決定者 |
|---|---|---|---|
| MC-01 | 長時間連続運用のメモリ上限・保持期限 | IR18で既にdeferred。100設備/1000サンプルの検収から無制限稼働を推定できない | Product Owner / Frontend |
| MC-02 | 本番API・認証・認可・DB・機器整合性・障害回復 | D11/OPEN-03〜06/11の後続成果物。本番へ進む場合の必須要件 | Backend / IoT / Security |

User新規登録、実メール送信、実GPS/マイク、実決済、顧客最終検収、ファイルexportをレビュー都合で追加しない。

# 9. Edge Cases Not Defined

「定義あり」は文書契約を確認した意味で、試験合格ではない。

| Edge case | 判定・根拠 |
|---|---|
| API timeout | 1AはD04の10秒、readのTIMEOUT自動再試行なし、writeはwrites.getResult。本番HTTPはNOT DEFINED |
| HTTP 400/401/403/404/409/429/500 | 実HTTPは対象外・NOT DEFINED。対応する1AのVALIDATION/UNAUTHENTICATED/FORBIDDEN/NOT_FOUND/CONFLICT/RATE_LIMITED/UNAVAILABLEはD01/D04/DDC-03。HTTPとの写像を勝手に確定しない |
| Network disconnected / Backend停止 | 1AのOFFLINE/UNAVAILABLEと最終時刻表示あり。本番Backend復旧はD11の未決 |
| IoT device offline / response timeout | D03/D04/D05。通常制御拒否と制限の未配送意図、30秒失効、遅延ackを分離 |
| Invalid / missing / stale sensor data | D07/IR08/IR11/IR12/IR16。非有限値・単位・境界・由来・TTLを判定し0補完しない |
| Duplicate operation | D04。同一キー/内容と別キー、新旧試行、認可再評価を区別 |
| Multiple browser tabs | D09。タブごと独立、同期・永続化対象外、常時注意表示 |
| Session expiration | D09/IR17。30分、閲覧破棄、受理済み業務は継続 |
| Permission changed during operation | D01/SR03/IR17/IR24。再認可、世代無効化、snapshot失効 |
| Empty device list / large device list | D07/D10/D13。0件とnot-found、25件既定/100上限、性能100設備・1000サンプル |
| Slow network | D10。3000ms遅延と12000ms timeout fixture。実通信のSLAではない |
| Language switch | D09/UX06。データ値維持、未確認音声intent破棄。**状態sortの比較順はEdge Case Undefined: REV-005** |
| Browser reload / Back button | D09/D13/IR17。reloadでseedへ、dirty確認、戻る/進むでURL復元。A16はIR33で補完 |
| Concurrent update | D04/SR02/SR14。版不一致、同一意図再送、不変snapshotと認可失効 |
| Partial failure | D02/D03/D10/SR28。設備別制限・通知別結果、補助Query局所失敗 |
| Foreign correlationId | REV-004修正。不存在と同じ0件、個別getと区別 |
| unitIds=[] / offer/historyでunitIds指定 | REV-002修正。0件または公開IDなしで不一致。非公開値で検索しない |
| 猶予のみ権限を持つHQ | **Edge Case Undefined: REV-001。現行型にその権限を表現できない** |

# 10. Traceability Matrix

[全64要件の追跡表](traceability-matrix.csv)を本報告の一部とする。列は指定どおりRequirement ID / Requirement / Prepare / Detailed Design / UI/UX / API / Error Handling / Testable / Status。StatusはOK・INCOMPLETE・MISSING・CONFLICTのみ。

API列は1A Repositoryの対応であり本番HTTPの合格を示さない。OKは今回の文書照合で未解決指摘がないもの。旧表のOKをそのまま結論にせず、要件出所・操作・画面・AT対応を再生成し、REV-001/005に依存する行をCONFLICT/INCOMPLETEへ下げた。修正済みの論点も追加受入条件を参照し、アプリ試験は全件not_run。

# 11. Undefined Decisions

| ID | Decision Needed | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| DEC-17 | 2権限と4権限のどちらを正式採用するか | FR-A10/DD-A10/Permission | 認可と拒否試験を確定できない | Product Owner / Security |
| DEC-18 | Job.statusの全状態比較順 | D07/query-catalog/IR23 | sort/言語切替/ページングの期待値が一意でない | Product Owner / UI/UX |
| OPEN-03 | 実機能力・イベント・検知精度 | Prepare/D11 | モック成功を実機能力と誤認しない | IoT |
| OPEN-04 | 本番API/DB/認証/session/token/CSRF/認可 | Prepare/D11 | UI非表示をサーバー認可の代替にしない | Backend / Security |
| OPEN-05/06 | 実決済・通知・算定方法・外部取引 | Prepare/D11 | 実配送/課金/証明の責任が未定義 | Business / Backend |
| OPEN-11 | ブラウザ停止後の時間付き制御の終了責任 | Prepare/D11 | 本番の終了・復旧主体が未定義 | Backend / IoT |
| MC-01 | 長時間容量と保持上限 | IR18 | 無制限運用へ拡張する場合の条件 | Product Owner / Frontend |

OPENの本番項目は1Aの新たな未解決欠陥として件数に足さない。1Aの最終判断者はDEC-12に記録された北野正樹・若井悠馬。本番担当者の実名は未指定。

# 12. Implementation Readiness

| 観点 | 判定 | 理由 |
|---|---|---|
| Requirements completeness | NOT READY | FR-A10の要件内矛盾が未解決 |
| Cross-document consistency | NOT READY | REV-001/005が残存 |
| UI/UX completeness | NOT READY | 権限別操作と状態sort期待値が未確定 |
| Frontend architecture | READY | Repository・Query・フォーム・Navigation・世代管理の責任定義あり。実装合格ではない |
| API contract readiness | CONDITIONALLY READY | 1A入出力・エラー・版の型検証は通過。認可粒度とsortの契約は未確定。本番HTTPはNOT READY |
| Error handling | CONDITIONALLY READY | D01/D04の定義と監査検索修正はある。権限粒度決定後に拒否条件を最終照合する |
| Authentication / Authorization | NOT READY | REV-001。実認証は対象外であり本番readyとは判定しない |
| IoT state handling | READY | 1Aの要求/応答/失効/再照合/制限回復/旧bindingを定義。実機試験なし |
| Testability | NOT READY | 状態sortと権限粒度の正しい期待値が未決 |
| Agentic SDLC handoff readiness | NOT READY | 未解決2件、修正後独立G1未実施 |

Agentic SDLC上のリスクは、Ambiguity/Non-deterministic Specification=REV-001/005、Missing Context=REV-003、Dangerous Assumption=REV-004/006、Agent Handoff Failure=REV-001/002/007。静的合格でこれらを上書きしない。

# 13. Required Actions Before Implementation

1. DEC-17/18の回答を記録し、選択結果に沿って関連文書・型・UI・受入条件を修正する。現時点で回答を代行しない。
2. [追加受入計画](../../acceptance-review-016.csv)の未確定2行を具体化し、既存ケースとの競合を再確認する。
3. 更新後のmanifestを固定し、静的・型検証と検証器変異検査を再実行する。文書修正者とは別主体によるG1で未解決0件を確認する。
4. 合格後、1A実装を開始する。アプリの型/lint/build/単体/Component/E2E/a11y/性能検査とデプロイ前の人・外部レビューは別工程。

今回の反復: Round 1で7件発見→根拠が確定している5件を修正→Round 2で変更の操作/画面/Component/ATと権限を再照合し、Commandリンクの依存記載も補完。未確定2件と独立再レビューを残す。**「全てOKになるまで」の完了条件はまだ満たしていない。**
