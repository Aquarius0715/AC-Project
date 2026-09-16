---
document_id: REQ-T
version: 0.7.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# 技術者 要件定義書

## 目的と前提

本書は[企業要件の英語原文（SRC-06）](../00-prepare/sources/company-requirements-original.txt)を一次資料として再整理した文書です。企業原文 → BIZ整理項目 → 本書のFR → 詳細設計・受入条件の順に追跡します。各機能の「企業要望」と「設計補完」を分け、画面項目・入力制約・状態遷移・優先度はフロントエンドの実装案として提示します。企業の詳細承認済みを意味しません。参考モックは外観設計の参考資料です。機能要件と受入条件は、企業原文の目的に制作方針・設計補完を加えて具体化します。

本書の対象は利用者がフロントエンドで確認・入力・操作できること。登録・請求・入金・機器操作・通知はモックで再現する。バックエンドの実処理・永続化・実認証は要件に含めない。

社内/外部の担当範囲で監視、診断、点検、IoT保守を実行する。

必読: [PrepareDocument](../00-prepare/PrepareDocument.md)、[共通要件](common.md)。共通の認証、言語、音声、権限、通知、非機能要件をすべて適用する。

優先度P0は基盤・中核フロー、P1も1Aの完成対象。各行の受入条件は`AT-T番号`として検証する。企業の要望に対応する範囲と、具体的なUI・仮値の提案を区別する。

## 機能要件と受入条件

| 要件ID | 優先 | 状態・根拠 | 要件 | 受入条件 |
|---|---|---|---|---|
| FR-T01 | P0 | 企業原文 SRC-06＋設計補完 / BIZ-04, BIZ-08 | 担当ダッシュボード | 担当設備、重要度、未対応、予定、進捗を表示。社内/外部のアクセス差を確認。 |
| FR-T02 | P0 | 企業原文 SRC-06＋設計補完 / BIZ-06, BIZ-07, BIZ-10 | 設備台帳 | 場所、メーカー、型番、構成、設置日、保守範囲を確認。 |
| FR-T03 | P0 | 企業原文 SRC-06＋設計補完 / BIZ-08, BIZ-11 | 時系列監視 | センサー、電力、運転、通信、取得時刻を表示。デモイベント更新で同一設備値が変わる。 |
| FR-T04 | P0 | 企業原文 SRC-06＋設計補完 / BIZ-10 | 室内機点検 | フィルター、蒸発器、送風モーター/ファン、ドレン配管/パン、吹出口/ルーバー全項目に結果または未点検理由を記録。 |
| FR-T05 | P0 | 企業原文 SRC-06＋設計補完 / BIZ-10 | 室外機点検 | 凝縮器、コンプレッサー、ファン/羽根、冷媒配管全項目に点検記録を残す。 |
| FR-T06 | P0 | 企業原文 SRC-06＋設計補完 / BIZ-10 | 電気・制御点検 | サーモスタット、センサー、コンデンサー、接触器、配線全項目に記録を残す。 |
| FR-T07 | P0 | 企業原文 SRC-06＋設計補完 / BIZ-08, BIZ-11, BIZ-17 | 異常根拠 | 疑い・測定・現場点検の区分、根拠値・履歴を提示。作業完了だけではresolvedに変わらない。 |
| FR-T08 | P0 | 企業原文 SRC-06＋設計補完 / BIZ-12 | 定期・事後・予防保全 | 担当案件を開始、報告提出、差戻しから再提出でき、顧客・HQの進捗と一致する。 |
| FR-T09 | P0 | 設計補完（企業目的に対応） / BIZ-12 | 作業報告 | チェックリスト、写真、測定値、交換部品、作業内容、次回対応を保存・提出。必須欠落を項目に表示。 |
| FR-T10 | P0 | 設計補完（企業目的に対応） / BIZ-13 | 遠隔診断・試運転 | 担当期間・能力・権限を満たす場合だけ設定変更/試運転。確認→応答まで区別。 |
| FR-T11 | P1 | 設計補完（企業目的に対応） / BIZ-20 | IoTライフサイクル | 登録、設備紐付け、接続確認、校正、FW更新をシミュレーション。進行・失敗・履歴が見える。 |
| FR-T12 | P1 | 企業原文 SRC-06＋設計補完 / BIZ-20 | IoT異常 | 通信断、電源断、取り外し検知を別々に再現・確認し、復旧時刻と対応履歴を保持。 |

## 業務境界・依存関係

権限は[共通要件の権限マトリクス](common.md)を唯一の基準とする。画面表示の許可と変更操作の許可を分離し、サービス呼出し直前にも検証する。設備能力・作業期間・契約条件が変わった場合は古い画面の許可を使わない。

実機操作、外部通知、実決済、API認証は1B。1Aでは操作できるシミュレーションを用意し、成功だけでなく拒否・失敗・欠測を再現する。

## 完了条件

- 全FR-T要件とFR-X/NFRの適用分を満たす。未実装を「対象外」に変更して完成扱いにしない。
- [詳細設計](../02-design/technician.md)の画面・サービス・異常系と受入条件を対応させる。
- [検証計画](../04-agentic-sdlc/verification.md)に従い、AT-T全件と該当シナリオの証跡を残す。
- 不確定な業務判断はPrepareDocumentのOPEN台帳へ戻し、モックの仮定を報告する。

## 機能別ユースケース・業務規則（0.6.0）

上の表は索引。以下が各要件の業務上の開始条件・手順・結果・受入条件の本文。受入行の①②…は記載順のsubcase（例: AT-C01-E.01）で、Given側とThen側の同じ番号が対になる。fixture名は[検証計画](../04-agentic-sdlc/verification.md)の固定fixtureを使う。原文に記載のない閾値・運用細則はDEC-09の1A提案として扱う。本番の確定ルールと混同しない。

### FR-T01 担当ダッシュボード

- **企業要望の根拠**: SRC-06 BIZ-04, BIZ-08 — 顧客、社内/外部技術者、管理者/HQの視覚的なダッシュボード / 異常を事前または発生時に把握し、リアルタイム通知する。
- **設計補完の範囲**: 担当期間による表示範囲。

- **利用開始条件**: 社内は担当範囲、外部は自社かつ個別割当・作業期間を取得できる。
- **基本フロー**: 今日/期間の担当案件を開く → 異常重要度・期限・進捗で並替え → 設備詳細または作業画面へ進む。
- **業務規則 BR-T01**: 未対応はrequested全件ではなく、自分が担当して未着手の案件。社内の横断閲覧も所属テナントと担当範囲を超えない。
- **完了後の業務状態**: 参照のみ。予定0件の際は空状態と利用可能な履歴導線。
- **境界条件・禁止事項**: 外部担当の期限終了をまたぐと当該live設備を隠す。別技術者の案件ID直打ちでも作業開始不可。

| 受入ID | Given / When | Then（観測可能な結果） |
|---|---|---|
| AT-T01-N | tech-external-a: job-contractor-a assigned（validUntil=2026-09-20）、他技術者のrequested1件。When: `/technician` today | ①担当1件、未着手1 ②他人のrequestedは非表示 ③書込み0件 |
| AT-T01-E | ①now=validUntilで外部担当画面 ②別技術者のjobIdでstart | ①当該live設備を非表示 ②FORBIDDEN |
| AT-T01-B | ①自分のassigned ②他人のrequested ③社内scope外 ④外部割当期間外 | ①表示 ②③④非表示 |

設計: [DD-T01](../02-design/technician.md#dd-t01-詳細)。親ケースAT-T01は追跡表に登録したN/E/Bおよび該当SRC/R01の全件で判定する。

### FR-T02 設備台帳

- **企業要望の根拠**: SRC-06 BIZ-06, BIZ-07, BIZ-10 — HVACを第2段階とし、メーカーや分離型・中央空調・カセット型等へ対応拡大 / 自宅/オフィス、エリア・階・部屋・スペース別の管理 / 室内機、室外機、電気・制御部品の状態把握。
- **設計補完の範囲**: 設備台帳の項目と閲覧手順。

- **利用開始条件**: 対象設備に閲覧可能な担当関係がある。
- **基本フロー**: 案件から台帳を開く → 設置場所・型番・構成・設置日・保守範囲を確認 → 診断/作業へ進む。
- **業務規則 BR-T02**: 機種能力はcapability版を表示。未登録/不明項目は未登録と表示し、典型機種の値で補完しない。
- **完了後の業務状態**: 参照のみ。技術者はメーカー台帳・顧客所属・請求を変更しない。
- **境界条件・禁止事項**: 台帳に設置日なしなら現在日を補わない。外部技術者が非割当設備を参照しても情報を返さない。

| 受入ID | Given / When | Then（観測可能な結果） |
|---|---|---|
| AT-T02-N | 担当内unit-online-rto（メーカー・型番・installedAt登録済み、capabilityVersion=3）。When: 台帳を開く | ①場所・型番・構成・設置日・保守範囲 ②capabilityVersion=3 ③書込み0件 |
| AT-T02-E | ①installedAt=nullの台帳 ②tech-external-aで非割当unitId | ①「未登録」表示、現在日を補わない ②NOT_FOUND |
| AT-T02-B | ①能力登録済み ②未登録 | ①候補表示 ②「未登録」、典型値なし |

設計: [DD-T02](../02-design/technician.md#dd-t02-詳細)。親ケースAT-T02は追跡表に登録したN/E/Bおよび該当SRC/R01の全件で判定する。

### FR-T03 時系列監視

- **企業要望の根拠**: SRC-06 BIZ-08, BIZ-11 — 異常を事前または発生時に把握し、リアルタイム通知する / 振動・高温・冷媒低下、微小漏れ、フィルター詰まり等の早期把握。
- **設計補完の範囲**: 系列選択と品質表示。

- **利用開始条件**: 対象設備のtelemetry閲覧権。センサーなしでも通信情報は表示可能。
- **基本フロー**: 指標と期間を選ぶ → センサー/電力/運転/通信の値と時刻を確認 → デモ更新イベントで変更 → 切断時は更新停止を認識する。
- **業務規則 BR-T03**: 観測時刻と受信時刻を分け、staleAfterSeconds超過はstale。古いイベントを最新値へ上書きしない。系列の単位を固定し欠測の間を連結しない。
- **完了後の業務状態**: 最新値と時系列を同じeventId/版で整合させる。画面離脱・scope変更で購読解除。
- **境界条件・禁止事項**: 順序逆転・重複イベントで値が逆行しない。通信断時も最後の値は時刻付きで残せるがリアルタイムと表示しない。

| 受入ID | Given / When | Then（観測可能な結果） |
|---|---|---|
| AT-T03-N | unit-online-rto、staleAfterSeconds=120。When: metric=temperature、24hで表示→/demoでevent version=4を発火→通信断 | ①系列と最新値が同じeventId／版 ②発火後に最新値更新 ③通信断で「更新停止」表示、購読解除 |
| AT-T03-E | ①version=3の後に2と重複3を送る ②通信断にする | ①値が逆行しない、重複は無視 ②最後の値と時刻を残し「リアルタイム」表示なし |
| AT-T03-B | ①観測から120秒 ②120秒+1ms ③系列途中にnull | ①valid ②stale ③欠測区間を線で連結しない |

設計: [DD-T03](../02-design/technician.md#dd-t03-詳細)。親ケースAT-T03は追跡表に登録したN/E/Bおよび該当SRC/R01の全件で判定する。

### FR-T04 室内機点検

- **企業要望の根拠**: SRC-06 BIZ-10 — 室内機、室外機、電気・制御部品の状態把握。
- **設計補完の範囲**: 点検フォーム・未点検理由。

- **利用開始条件**: 有効な担当案件がin_progressである。保守範囲と部品別の点検対象が取得済み。
- **基本フロー**: 対象部品ごとに点検結果を選ぶ → 必要な所見・測定・写真を関連付け → 未点検/対象外は理由を記入 → ドラフトまたは報告へ保存する。
- **業務規則 BR-T04**: 対象グループはindoor、項目はfilter / evaporator_coil / blower_motor / blower_fan / drain_pipe / drain_pan / outlet / louver。初期結果はnullで、未点検のまま提出するなら明示的なnot_inspectedと理由が必要。設備に存在しない部品はnot_applicableと理由を記録する。
- **完了後の業務状態**: 点検結果を報告版・作者・観測時刻へ紐付ける。センサー推定は別の根拠として残し、現地点検結果で元データを上書きしない。
- **境界条件・禁止事項**: 未入力、単位なし測定、未点検理由なし、他案件の写真参照を拒否。正常を初期選択して空点検を完了扱いしない。

| 受入ID | Given / When | Then（観測可能な結果） |
|---|---|---|
| AT-T04-N | in_progress案件、indoor 8部品。When: filter=attention（理由あり、写真1枚）、他7部品normalでドラフト保存→提出 | ①ドラフト保存でreportVersion=1 ②提出成功、8部品の結果・作者・観測時刻が版に紐付く ③センサー推定は別根拠として残る |
| AT-T04-E | 提出時に ①result=null 1部品 ②測定に単位なし ③not_inspected理由なし ④他案件のattachmentId | 各VALIDATION、提出0件 |
| AT-T04-B | indoor 8部品を ①normal ②attention（理由あり） ③not_inspected（理由あり） ④not_applicable（理由あり） ⑤null で保存／提出 | ①〜④ドラフト保存可・提出可 ⑤ドラフト保存可・提出はVALIDATION。初期値はnullで「normal」が既定選択されない |

設計: [DD-T04](../02-design/technician.md#dd-t04-詳細)。親ケースAT-T04は追跡表に登録したN/E/Bおよび該当SRC/R01の全件で判定する。

### FR-T05 室外機点検

- **企業要望の根拠**: SRC-06 BIZ-10 — 室内機、室外機、電気・制御部品の状態把握。
- **設計補完の範囲**: 点検フォーム・未点検理由。

- **利用開始条件**: 有効な担当案件がin_progressである。保守範囲と部品別の点検対象が取得済み。
- **基本フロー**: 対象部品ごとに点検結果を選ぶ → 必要な所見・測定・写真を関連付け → 未点検/対象外は理由を記入 → ドラフトまたは報告へ保存する。
- **業務規則 BR-T05**: 対象グループはoutdoor、項目はcondenser_coil / compressor / fan / blade / refrigerant_pipe。初期結果はnullで、未点検のまま提出するなら明示的なnot_inspectedと理由が必要。設備に存在しない部品はnot_applicableと理由を記録する。
- **完了後の業務状態**: 点検結果を報告版・作者・観測時刻へ紐付ける。センサー推定は別の根拠として残し、現地点検結果で元データを上書きしない。
- **境界条件・禁止事項**: 未入力、単位なし測定、未点検理由なし、他案件の写真参照を拒否。正常を初期選択して空点検を完了扱いしない。

| 受入ID | Given / When | Then（観測可能な結果） |
|---|---|---|
| AT-T05-N | in_progress案件、outdoor 5部品。When: compressor=attention（理由あり）、他4部品normalでドラフト保存→提出 | ①ドラフト保存でreportVersion=1 ②提出成功、5部品の結果・作者・観測時刻が版に紐付く ③センサー推定は別根拠として残る |
| AT-T05-E | 提出時に ①result=null 1部品 ②測定に単位なし ③not_inspected理由なし ④他案件のattachmentId | 各VALIDATION、提出0件 |
| AT-T05-B | outdoor 5部品を ①normal ②attention（理由あり） ③not_inspected（理由あり） ④not_applicable（理由あり） ⑤null で保存／提出 | ①〜④ドラフト保存可・提出可 ⑤ドラフト保存可・提出はVALIDATION。初期値はnull |

設計: [DD-T05](../02-design/technician.md#dd-t05-詳細)。親ケースAT-T05は追跡表に登録したN/E/Bおよび該当SRC/R01の全件で判定する。

### FR-T06 電気・制御点検

- **企業要望の根拠**: SRC-06 BIZ-10 — 室内機、室外機、電気・制御部品の状態把握。
- **設計補完の範囲**: 点検フォーム・未点検理由。

- **利用開始条件**: 有効な担当案件がin_progressである。保守範囲と部品別の点検対象が取得済み。
- **基本フロー**: 対象部品ごとに点検結果を選ぶ → 必要な所見・測定・写真を関連付け → 未点検/対象外は理由を記入 → ドラフトまたは報告へ保存する。
- **業務規則 BR-T06**: 対象グループはelectrical、項目はthermostat / sensor / capacitor / contactor / wiring。初期結果はnullで、未点検のまま提出するなら明示的なnot_inspectedと理由が必要。設備に存在しない部品はnot_applicableと理由を記録する。
- **完了後の業務状態**: 点検結果を報告版・作者・観測時刻へ紐付ける。センサー推定は別の根拠として残し、現地点検結果で元データを上書きしない。
- **境界条件・禁止事項**: 未入力、単位なし測定、未点検理由なし、他案件の写真参照を拒否。正常を初期選択して空点検を完了扱いしない。

| 受入ID | Given / When | Then（観測可能な結果） |
|---|---|---|
| AT-T06-N | in_progress案件、electrical 5部品。When: capacitor=attention（理由あり、測定値付き）、他4部品normalでドラフト保存→提出 | ①ドラフト保存でreportVersion=1 ②提出成功、5部品の結果・作者・観測時刻が版に紐付く ③測定値にunit・observedAt・origin=inspection |
| AT-T06-E | 提出時に ①result=null 1部品 ②測定に単位なし ③not_inspected理由なし ④他案件のattachmentId | 各VALIDATION、提出0件 |
| AT-T06-B | electrical 5部品を ①normal ②attention（理由あり） ③not_inspected（理由あり） ④not_applicable（理由あり） ⑤null で保存／提出 | ①〜④ドラフト保存可・提出可 ⑤ドラフト保存可・提出はVALIDATION。初期値はnull |

設計: [DD-T06](../02-design/technician.md#dd-t06-詳細)。親ケースAT-T06は追跡表に登録したN/E/Bおよび該当SRC/R01の全件で判定する。

### FR-T07 異常根拠

**原文から具体化する要件 — 窓開放・断熱不足の負荷通知（BIZ-17）**

窓開放・断熱不足による負荷増加を原因候補として通知し、対象設備、時刻、根拠、確認・保守導線を表示する。

**追加受入条件 AT-T07-SRC**: 窓開放の疑い・断熱不足の点検記録・根拠なしの3 fixtureで、文言・根拠・時刻が異なり、既読にしても異常が解消しない。

- **企業要望の根拠**: SRC-06 BIZ-08, BIZ-11, BIZ-17 — 異常を事前または発生時に把握し、リアルタイム通知する / 振動・高温・冷媒低下、微小漏れ、フィルター詰まり等の早期把握 / 生活パターンや天候に応じた運転、開いた窓や断熱不足に起因する負荷への通知。
- **設計補完の範囲**: 原因候補と根拠・解消手順。

- **利用開始条件**: 担当設備に異常または診断の疑いがある。
- **基本フロー**: 異常一覧から根拠を開く → 計測/推定/現地点検と履歴を確認 → 確認済みにする → 必要なら再測定・理由付き解消へ進む。
- **業務規則 BR-T07**: 推定に確信度がない場合は数値の確率を生成しない。確認操作はacknowledged。解消には再測定で設定条件を満たすかalert.resolve権限と理由が必要。
- **完了後の業務状態**: 検知、確認、解消の各時刻と主体を保持。再発は新alertIdで旧事象と関連付ける。
- **境界条件・禁止事項**: Job完了だけではresolvedにしない。通信断だけの根拠で盗難と断定せず、取り外し専用事象と分離する。

| 受入ID | Given / When | Then（観測可能な結果） |
|---|---|---|
| AT-T07-N | unit-online-rtoにopen Alert（evidenceKind=inferred）。When: 根拠を開く→acknowledge→再測定で回復条件成立 | ①計測／推定／点検が区別表示 ②acknowledged、acknowledgedAt・主体 ③resolved、resolvedAt ④再発は新alertIdでpreviousAlertId |
| AT-T07-E | ①未解消AlertのJobだけcompletedにする ②heartbeat途絶だけを与える | ①Alert.status不変 ②盗難表示なし、connection=offlineのみ |
| AT-T07-B | ①確信度なしの推定 ②alert.resolveあり理由付き ③権限なし ④再測定で回復 | ①確率数値なし ②resolved ③FORBIDDEN ④resolved |

設計: [DD-T07](../02-design/technician.md#dd-t07-詳細)。親ケースAT-T07は追跡表に登録したN/E/Bおよび該当SRC/R01の全件で判定する。

### FR-T08 定期・事後・予防保全

- **企業要望の根拠**: SRC-06 BIZ-12 — 定期・事後・予防保全。RTO以外の一般保守にも利用。
- **設計補完の範囲**: 開始・提出・再提出の状態。

- **利用開始条件**: assigned案件を担当し、有効期間内。定期/事後/予防は同じ作業状態モデル。
- **基本フロー**: 担当と予定を確認 → 作業開始 → 報告を編集 → 提出 → 品質確認待ち → 差戻しなら再作業と再提出。
- **業務規則 BR-T08**: 開始可能はassigned、差戻し再開はrework_requested。submittedでは提出版を読取専用。顧客承認を技術者が代行しない。
- **完了後の業務状態**: 開始時刻・提出時刻・reportVersionを保存。完了は品質担当のreviewで決定。
- **境界条件・禁止事項**: 未割当、取消、on_hold、期限外のstart/submitを拒否。提出失敗ではin_progressとドラフトを保持する。

| 受入ID | Given / When | Then（観測可能な結果） |
|---|---|---|
| AT-T08-N | assigned案件、期間内。When: start→編集→submit→HQがreturn→resumeRework→submit | ①in_progress、startedAt ②submitted、reportVersion=1 ③rework_requested ④in_progressへ戻り新版v2で提出 |
| AT-T08-E | ①requested／cancelled／on_holdでstart ②担当期限外でsubmit ③in_progressでsubmitをUNAVAILABLE | ①CONFLICT ②FORBIDDEN ③in_progressとドラフト保持 |
| AT-T08-B | ①assignedでstart ②submittedで編集 ③rework_requestedでresumeRework | ①in_progress ②読取専用、保存不可 ③in_progress |

設計: [DD-T08](../02-design/technician.md#dd-t08-詳細)。親ケースAT-T08は追跡表に登録したN/E/Bおよび該当SRC/R01の全件で判定する。

### FR-T09 作業報告

- **企業要望の根拠**: SRC-06 BIZ-12 — 定期・事後・予防保全。RTO以外の一般保守にも利用。
- **設計補完の範囲**: 写真・交換部品・報告版の管理。

- **利用開始条件**: in_progressまたは再作業中。点検項目とドラフトを取得済み。
- **基本フロー**: 点検・測定・写真・交換部品・作業本文・次回対応を入力 → ドラフト保存 → 提出前チェック → 報告版を確定する。
- **業務規則 BR-T09**: 報告は作者・版を保持。画像はJPEG/PNG、1枚5MiB以下、最大10枚。数量は正整数、次回対応はnoneまたは日時/内容を明示。サービス再取得でdirty内容を消さない。
- **完了後の業務状態**: 保存成功時にdraft版を更新し、提出時は固定したreportVersionをJobへ紐付ける。写真削除時にobject URLを解放する。
- **境界条件・禁止事項**: 本文9/4001文字・部品数量0は提出を拒否。偽MIME・11枚目・5MiB超過は画像追加を拒否。画像処理失敗はfailedと表示し、失敗写真が残るまま提出できない。保存済み本文・画像は保持する。

| 受入ID | Given / When | Then（観測可能な結果） |
|---|---|---|
| AT-T09-N | in_progress案件。When: 本文50文字、写真JPEG 1枚、部品1点、nextAction=noneでドラフト保存→提出 | ①draft version=1、Attachment status=ready ②提出でreportVersion固定、JobにreportRefs ③写真削除でobject URL解放 |
| AT-T09-E | ①本文9／4001文字 ②偽MIME ③11枚目 ④5MiB+1byte ⑤部品数量0 ⑥failed写真が残る | ①⑤⑥提出VALIDATION ②③④追加拒否 ⑦保存済み本文・画像は保持 |
| AT-T09-B | ①5MiBちょうどのJPEG／PNG各10枚 ②数量1 ③nextAction=follow_up（未来日時・内容） ④dirty中にreports.get再取得 | ①追加可 ②提出可 ③提出可 ④dirty値が保持される |

設計: [DD-T09](../02-design/technician.md#dd-t09-詳細)。親ケースAT-T09は追跡表に登録したN/E/Bおよび該当SRC/R01の全件で判定する。

### FR-T10 遠隔診断・試運転

- **企業要望の根拠**: SRC-06 BIZ-13 — スマートサーモスタット等で温度を確認し、離れた場所から設定を変更。
- **設計補完の範囲**: 技術者の操作権限と試運転手順。

- **利用開始条件**: 担当期間内、control.diagnose能力、機器online。契約制限を超えない。
- **基本フロー**: 現在状態・担当案件を確認 → 診断操作・試運転時間・理由を指定 → 確認 → Commandの応答と履歴を見る。
- **業務規則 BR-T10**: 同時FW更新や未完了Command中は開始不可。試運転終了も終了Commandの応答が必要で、ブラウザタイマー終了を実停止扱いしない。
- **完了後の業務状態**: 通常診断は理由とjobId付きCommand、試運転は時間・終了動作を保持するDiagnosticRunと開始/終了Commandを作成。終了予定と終了応答を別表示し、終了失敗は注意として残す。
- **境界条件・禁止事項**: 制限温度の迂回、担当期間外、16分の試運転、理由なしを拒否。終了応答がなければ停止済みにしない。

| 受入ID | Given / When | Then（観測可能な結果） |
|---|---|---|
| AT-T10-N | 担当期間内、control.diagnose、unit-online-rto online。When: 通常診断set_mode=cool（理由付き）→試運転duration=5・endAction=power OFF開始→開始応答→終了時刻→終了応答 | ①Command1件（jobId・reason付き） ②DiagnosticRun awaiting_start→running、endAt=startedAt+5分 ③end_requested、終了Command1件 ④completed、「停止済み」表示 |
| AT-T10-E | ①制限下限未満の温度 ②担当期間外 ③durationMinutes=16 ④reason空 ⑤終了応答なし | ①FORBIDDEN ②FORBIDDEN ③④VALIDATION ⑤end_failed、「停止済み」表示なし |
| AT-T10-B | ①未完了Commandあり ②FW更新中 ③終了時計到来のみ ④終了応答到来 | ①②CONFLICT ③end_requested（停止済みではない） ④completed |

**追加受入条件 AT-T10-R01（再訪・競合・役割横断）**

| 受入ID | Given / When | Then |
|---|---|---|
| AT-T10-R01 | 開始応答時刻01:00:05、duration=5、endAction=OFF。画面離脱・役割切替後に01:05:05へ進める | 終了Commandは1件、元Membershipで再認可。終了応答までは停止済みでない。応答後completed。途中失効ならend_blockedで要求0件。reset後の旧イベントは反映0件。 |

設計: [DD-T10](../02-design/technician.md#dd-t10-詳細)。親ケースAT-T10は追跡表に登録したN/E/B・R01および該当SRCの全件で判定する。

### FR-T11 IoTライフサイクル

- **企業要望の根拠**: SRC-06 BIZ-20 — 小型・低価格で空調内に設置する機器、FW、取り外し・盗難への対策と通知。
- **設計補完の範囲**: 登録・校正・更新の模擬手順。

- **利用開始条件**: device.maintainと対象設備の有効担当。登録/校正/更新はモック。
- **基本フロー**: serial登録 → 設備紐付け → 接続確認 → 校正値と参照を記録 → 対応FW候補を選び更新 → 進行/結果を確認する。
- **業務規則 BR-T11**: serialはtrim/大文字化して一意判定。校正は履歴追加、既存測定値は書き換えない。FWは対応版リストからのみ選択。URLやバイナリを自由入力させない。
- **完了後の業務状態**: Device、CalibrationRecord、DeviceOperationを保持。succeededだけfirmwareVersion更新。更新中は競合制御不可。
- **境界条件・禁止事項**: 重複serial・別設備への無断再紐付け・単位不一致を拒否し台帳を変更しない。offlineではFW更新を開始しない。FW更新失敗はfailedと表示し、旧FW版を保持する。

| 受入ID | Given / When | Then（観測可能な結果） |
|---|---|---|
| AT-T11-N | device.maintain、担当unit。When: serial ac-001登録→紐付け→check→校正（同単位）→FW v2更新→成功イベント | ①Device作成、serial=AC-001 ②bind成功 ③DeviceOperation succeeded ④CalibrationRecord追加、既存測定値不変 ⑤firmwareVersion=v2 |
| AT-T11-E | ①「ac-001 」を再登録 ②他設備へ理由なし再紐付け ③offlineで更新 ④校正単位不一致 ⑤FW失敗イベント | ①CONFLICT ②VALIDATION ③OFFLINE ④VALIDATION ⑤failed、旧版保持 |
| AT-T11-B | ①小文字／大文字serial ②校正前後の測定履歴 ③対応／非対応FW | ①同一判定 ②履歴追記、既存値不変 ③対応のみ選択可 |

**追加受入条件 AT-T11-R01（再訪・競合・役割横断）**

| 受入ID | Given / When | Then |
|---|---|---|
| AT-T11-R01 | 機器a/bが同じFW候補v2を持つ。deviceId=aで更新し、成功イベントを送る | aだけv2、bは旧版。deviceId省略はVALIDATION、対象外IDは拒否。 |

設計: [DD-T11](../02-design/technician.md#dd-t11-詳細)。親ケースAT-T11は追跡表に登録したN/E/B・R01および該当SRCの全件で判定する。

### FR-T12 IoT異常

- **企業要望の根拠**: SRC-06 BIZ-20 — 小型・低価格で空調内に設置する機器、FW、取り外し・盗難への対策と通知。
- **設計補完の範囲**: 通信断・電源断と取り外しの区別。

- **利用開始条件**: 担当Deviceにイベント閲覧権がある。
- **基本フロー**: 通信断・電源断・取り外し検知を別々に模擬発火 → 通知を確認 → 対応メモ → 復旧/確認を記録する。
- **業務規則 BR-T12**: connectionとtamperを独立管理。電源断判定は電源専用信号のデモがある時のみ。heartbeatなしから電源断と断定しない。
- **完了後の業務状態**: 検知時刻・観測根拠・対応・復旧時刻を別イベントで保持。通知の確認は物理状態を変更しない。
- **境界条件・禁止事項**: 通信再接続で未確認のtamperアラートが消えない。out-of-orderの古いheartbeatでonlineへ戻さない。

| 受入ID | Given / When | Then（観測可能な結果） |
|---|---|---|
| AT-T12-N | device-tamper。When: communication_lost→power_lost→tamper→responseNote→restored | ①3事象を別イベント、occurredAt ②responseNoteで物理状態不変 ③restoredAt保存、tamperアラートは残る |
| AT-T12-E | ①tamper未確認のまま通信復旧 ②新しい通信断の後に古いheartbeat | ①tamperアラート残存 ②offlineのまま |
| AT-T12-B | ①heartbeat途絶 ②power_signal断 ③tamper_signal | ①offlineのみ ②電源断 ③tamper、それぞれ独立 |

設計: [DD-T12](../02-design/technician.md#dd-t12-詳細)。親ケースAT-T12は追跡表に登録したN/E/Bおよび該当SRC/R01の全件で判定する。

