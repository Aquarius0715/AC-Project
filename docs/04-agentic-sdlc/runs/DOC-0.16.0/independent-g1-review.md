# 1. Executive Review Summary

**独立G1: PASS（1Aフロントエンドモック設計の実装引渡しはREADY）。未解決指摘0件。**

最終入力はDOC-0.16.0、spec_baseline_id=`fe251845299c3b5d0a9d493683be3e70076111af3b290b8b01a8fa2f7450e0b2`。レビュー担当は仕様修正者`/root`とは別実行の`/root/independent_g1_016`。レビュー担当は仕様ファイルを編集せず、指摘を修正者へ戻し、修正後の実物を再確認した。旧版の合格や自己レビューを承認根拠として流用していない。

今回の追加指摘はG1R-001（MINOR、決定記録の出所metadata不足）1件。修正・独立再確認済み。既存REV-001〜007は解消を確認した。権限2種類と、案件のソート機能・既定業務順が要件、契約、型、Query、URL、画面、Component、受入計画で一致する。

55ファイル全件のSHA-256とcanonical JSONからのbaselineを独立計算し一致。静的検証およびTypeScript strict検証を独立実行して成功。これに加えて認可・投影・sort・ページング・状態・再送・期限の反例を文書間で照合した。アプリ実装、動作・性能・a11y試験は**not_run**。本番接続・デプロイは**NOT READY**であり、G1の対象に含めない。

# 2. BLOCKER Issues

未解決0件。REV-001: FR-A10一覧・詳細、Permission、操作カタログ、IR03/34、A09/A10、AT-REV16-001で2権限に統一。manageのみ／overrideのみ／両方／なしを照合し、暗黙の相互付与はない。override専用者には限定したread投影と解除追跡経路があり、適用操作を付与しない。

# 3. CRITICAL Issues

未解決0件。REV-004: audit.listは認可済み集合へ検索条件を適用。他テナントcorrelationIdと不存在IDは同じ成功空集合となり、個別getのNOT_FOUNDと区別される。AT-A16-EとAT-REV16-004の期待値も一致する。

# 4. MAJOR Issues

未解決0件。

| 対象 | 独立照合した契約と反例 | 判定 |
|---|---|---|
| REV-002 | IR32/26/23、jobs.listとsummaries.getのunitIds。空配列、unitIdとのAND、不公開offer/historyの非一致 | resolved |
| REV-003 | IR33、A16、画面/Component/操作カタログ。audit必須、devices候補→明示選択→events、無権限Commandリンクは追加readなし | resolved |
| REV-005 | DEC-18、JOB_STATUS_ORDER、Query、IR34、全jobs.list画面、AT-REV16-005。全10状態・逆順・同値ID・公開/凍結状態・null末尾 | resolved |

sort変更時にfilterを維持しcursorを捨てる、URLの不正/空/重複sortを拒否する、Back/Forwardで復元する、Query keyにsortと閲覧世代を含める条件を確認した。sortは全snapshotに適用してから分割し、summaries.getへ渡さない。KPIの意味や状態遷移自体をソートにより変更しない。

# 5. MINOR Issues

未解決0件。REV-006のPage.totalは認可・投影・filter後の非負整数であり、未取得はinitial/loadingとして分離。REV-007は現行manifestのgateを判定先とし、READMEの固定pending/未実施記述も判定先へのリンクに改められた。

## G1R-001 — accepted決定の出所metadataが実行schemaを満たさない

- **Issue ID:** G1R-001
- **Severity:** MINOR
- **Document:** docs/00-prepare/internal/review-decisions-016.json、docs/04-agentic-sdlc/templates/artifacts.md
- **Location:** DEC-17/18のacceptedレコード、テンプレート「状態schema」「人への判断依頼・決定記録」
- **Problem:** 初回レビュー時のレコードにstatus/selected/sourceはあったが、acceptedに必要なdecided_by・決定日・answerと実行schemaのdecision_statusがなかった。
- **Why it matters:** 機械的な引渡しで、担当候補ownerと実際の決定者、設計側の選択値と実際のユーザー回答を区別できない。
- **Example Failure:** Product Owner / Securityというownerを実際の承認者として読み替えたり、acceptedの必須metadataを検査する次工程が記録を不完全と判定する。
- **Required Fix:** 会話の実回答に基づく決定者、判明する精度の決定日、回答原文、decision_statusを記録する。未提供の氏名や時刻を創作しない。
- **Suggested Revision:** decided_by=本会話のユーザー、decided_at=2026-09-16、decided_at_precision=day、answer=各回答原文、decision_status=accepted。status aliasを残す場合は一致を検証する。
- **Finding status:** resolved
- **再確認:** 最終baselineのDEC-17/18で上記を確認。ユーザーの決定内容自体は変更されていない。検証器にprovenance必須・status一致検査が追加され、型付き静的検証も成功した。修正者は/root、再確認者は/root/independent_g1_016。

# 6. Open Questions

今回の1A設計引渡しを阻害する追加質問なし。DEC-17/18はユーザー回答に基づくaccepted。業務順の具体的順位はIR34の実装契約として固定され、表示順位と状態遷移許可を区別している。

# 7. Cross-document Inconsistencies

修正後の追加不整合は検出しなかった。

| 照合軸 | 根拠 |
|---|---|
| Requirement Missing | 64要件の出所・設計・UI・操作・受入対応を静的照合。重点対象はFR-A10/A16、P01/P03/P06/T01/C09/A06、FR-X04 |
| Design / UI Without Requirement | 2権限は既存FR-A10の矛盾解消。sortは今回のユーザー指示。A16候補readは既存監査機能の遂行経路 |
| UI Without API | APIは1A Repository。監査機器選択、Commandリンク解決、制限専用read、jobs.listの入力に対応操作あり |
| Data Model Gap | Page.total、Query.unitIds/sort、Jobの3投影、JOB_STATUS_ORDER、RestrictionReleaseViewと正規型を照合 |
| Terminology Conflict | DeviceとUnit、Command応答とJob完了、閲覧世代とRepository世代、重大度nullとnormalを区別 |
| Handoff consistency | 現行manifestと判定証跡をbaselineで固定。旧passedを現行承認にしない |

同論点ではIR契約が旧DD/D/SR記述に優先する既定ルールを適用した。単独の画面DDだけで実装せず、manifest一式を引き渡す。

# 8. Missing Requirements

今回のレビューで追加の必須1A要件欠落は検出しなかった。企業原文SRC-06→BIZ対応表→FR出所表を確認し、企業要望と制作方針/設計補完の区別を維持した。独立した施工業者役割やデモの詳細条件を企業原文そのものとは扱わない。

長時間メモリ容量はIR18のdeferred候補。本番API・認証・DB・IoT・実決済/通知・ブラウザ停止後の終了責任はD11/OPENの後続課題。これらを1Aへ無断追加せず、本番READYとも扱わない。

# 9. Edge Cases Not Defined

以下は文書上の反例照合結果であり、アプリでの試験結果ではない。

| 境界・反例 | 定義と期待値 |
|---|---|
| manageなし・overrideのみ | IR03/34。限定投影、強制解除と限定した解除追跡のみ。defer/exempt/cancel拒否 |
| 他tenant相関ID／不存在 | IR33。同じ成功0件。監査必須read失敗は正常0件にしない |
| events未選択／候補失敗／不可視URL | IR33。呼出し0回／局所retry／not-found。別機器への置換なし |
| 状態descの同値／dueAtとseverity null | IR34。同値ID asc、nullは両方向末尾 |
| 2頁目直前にOffer/Assignment終了 | IR24/SR14。旧snapshot全体CONFLICT、viewEpoch更新、初頁から凍結投影 |
| 同一状態の非公開Job更新 | IR23/34。offer/historyの公開値が変わらなければ件数・順序に影響させない |
| ソート変更後の遅い応答／A→B→A | IR17/34。旧key/世代の描画を拒否。新一覧を旧応答で上書きしない |
| 通常制御offlineと制限の未配送 | D03。前者は0件、後者は未配送意図を記録し、適用済みと表示しない |
| 制限解除時sent_unknown・遅延ack | D03/SR26。再照合まで保留、終端をactiveへ戻さず回復caseで追跡 |
| write timeout・再送・権限失効 | D01/D04/IR03。結果確認→同じ意図で再送、現在認可と投影を再適用 |
| session期限／reload／タブ | D09/IR17。閲覧破棄、受理済み業務は継続、reloadはseedへ、タブ間非同期 |
| 不明単位・欠測・古いセンサー | D07/IR08/12/16/22。suspect/null・積算除外、旧validへのfallbackなし |
| 試運転終了時計のみ／FW遅延成功 | D04/05とAT-T10/T11。実停止や更新成功を捏造しない |
| 報告共同編集者の別Membership承認 | IR31。userIdの寄与者集合でFORBIDDEN、UIもreviewAvailabilityで無効化 |
| HTTPコード・実機回復 | 1A対象外。D11の本番契約はNOT DEFINED/NOT READY |

追加の未定義1A境界は検出しなかった。既存ATの遠隔制御・予定/DST・電力比較・依頼・委託期限・自己承認・試運転・FW更新を標本照合し、IRでの優先定義も適用した。

# 10. Traceability Matrix

[64要件の既存対応表](traceability-matrix.csv)と[仕様追跡表](../../../00-prepare/traceability.csv)を参照。独立実行の静的検証で64要件、182受入束、49役割詳細、136操作、47画面、57Component、35Query契約、93版分岐を確認した。IR32〜34の変更対象は要件から型/Query/UI/ATまで意味照合した。

表のOKは文書上の対応がある意味。全64要件の全subcaseを実行した意味でも、各機能の実装正しさを証明した意味でもない。意味レビューは本報告で列挙した変更箇所と関連リスク・既存AT標本を対象とした。

# 11. Undefined Decisions

| ID | Decision Needed / 状態 | Related Document | Why Needed | Who Should Decide |
|---|---|---|---|---|
| DEC-17/18 | 解消、ユーザー回答を記録済み | 決定JSON/IR34 | 2権限と既定業務順を固定 | 本会話のユーザー（回答済み） |
| IR18 | 長時間保持容量、deferred | IR18 | 保証範囲拡張時に必要 | Product Owner / Frontend |
| OPEN-03〜06/11 | 本番機器・API・認証・連携・終了責任 | D11/Prepare | 本番へ進むための後続成果物 | Backend / IoT / Security / Business |

1A判断者はDEC-12を参照。G1判定は業務承認やデプロイ承認を代行しない。

# 12. Implementation Readiness

| 観点 | 1A設計判定 | 根拠 |
|---|---|---|
| Requirements completeness | READY | 今回の判断待ち0、変更対象の受入条件あり |
| Cross-document consistency | READY | 既存7件と追加metadata指摘を修正確認 |
| UI/UX completeness | READY | sort/URL/loading/error/empty/権限別導線とアクセシビリティ条件 |
| Frontend architecture | READY | Repository、Query、RHF、Navigation、世代の責任分離 |
| API contract readiness | READY | 1Aローカル契約。実HTTPはNOT READY |
| Error handling | READY | D01/D04、相関ID、無効URL、snapshot、再送 |
| Authentication / Authorization | READY | 1A模擬認証と2権限・scope・投影。実認証は対象外 |
| IoT state handling | READY | 模擬要求/観測/回復と排他。実機適合を保証しない |
| Testability | READY | 具体的な順位・拒否・件数・境界の期待値を定義 |
| Agentic SDLC handoff readiness | READY | 本baselineへの別主体G1判定あり。gate記録はオーケストレーターが反映 |

# 13. Required Actions Before Implementation

仕様修正の未解決事項なし。オーケストレーターは本報告・結果JSONに基づき、同じbaselineのgate-G1をpassedへ更新する。manifestを変更した場合は本判定を自動流用せず再評価する。

実装時は全manifest入力と既存/追加ATを渡し、アプリ試験はnot_runから開始する。性能・a11y・ブラウザ動作とデプロイ前の人/外部レビューは後続工程。本報告はそれらの合格証跡ではない。

独立実行コマンド: `python3 docs/tools/validate_documents.py --tsc /private/tmp/ac-typescript-check/package/lib/tsc.js`（exit 0、errors=[]、typescript_semantic_check=passed）。hash検証は検証器と別のPython処理で全55ファイルとcanonical JSONを計算し、不一致0・pathの一意な整列を確認した。
