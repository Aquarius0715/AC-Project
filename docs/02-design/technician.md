---
document_id: DD-T
version: 0.5.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# 技術者 詳細設計書

設計対象の機能・画面項目・状態・例外は企業原文と対応要件を基準とし、各FRを満たす処理と受入条件を定義します。参考モックは共通UIの外観検討に使用します。

## 入力・責務

一次資料: [企業要件原文（SRC-06）](../00-prepare/sources/company-requirements-original.txt)。原文を再整理した要件から画面・入力・状態・受入条件を設計する。入力: [役割別要件](../01-requirements/technician.md)、[共通要件](../01-requirements/common.md)。必読: [共通詳細設計](common.md)、[UIUX仕様書](../03-uiux/UIUXSpecification.md)。以下はフロントエンドの項目・表示・モック動作の設計。画面上の登録・割当・入金・制限・監査はすべて共有モックメモリの状態遷移で、サーバー実装やDB設計を依頼するものではない。

ルートパラメーターは未信頼入力として検証する。表のservice名は共通Repositoryの論理操作名。同じルートの行は同一画面内の機能を分担する。全行にloading/empty/error/forbidden/not-foundを実装する。再試行は回復可能なエラーだけに提供し、権限不足では許可された画面へ戻す。

## 画面・処理設計

| 設計ID / 要件 | ルート / 主コンポーネント | 取得・操作契約 | 入力・処理・検証 | 異常系と禁止事項 |
|---|---|---|---|---|
| DD-T01 / FR-T01 | `/technician` / `TechnicianOverview` | `jobs.list, alerts.list` | 期間・重要度フィルター。割当と期限をサービスで照合 | 未割当の他案件が検索・集計に出ない |
| DD-T02 / FR-T02 | `/technician/units/:id` / `DiagnosticUnit` | `units.get, devices.list` | 設備ID、対応能力を閲覧。保守範囲外を明示 | 台帳未登録と通信断を区別 |
| DD-T03 / FR-T03 | `/technician/units/:id` / `TelemetryPanel` | `telemetry.series, telemetry.summary` | 指標・期間。購読はRepository経由。1Aは決定的イベント | 古い値をリアルタイムと表示しない。ストリーム切断は再照会へ |
| DD-T04 / FR-T04 | `/technician/jobs/:id` / `InspectionForm` | `jobs.get, jobs.saveDraft, jobs.submit` | 各部品に正常/要対応/未点検/対象外、所見、測定根拠。正常を初期値にしない | 対象外/未点検は理由必須。デモ診断を実測結果に置換しない |
| DD-T05 / FR-T05 | `/technician/jobs/:id` / `InspectionForm` | `jobs.get, jobs.saveDraft, jobs.submit` | 室内機と共通schema、部品グループを分離 | 未選定センサーで微小漏れ検知を保証する文言を拒否 |
| DD-T06 / FR-T06 | `/technician/jobs/:id` / `InspectionForm` | `jobs.get, jobs.saveDraft, jobs.submit` | 測定値は数値+単位+観測時刻+点検者。操作手順や施工指示は本書対象外 | 測定なしを0や正常として保存しない |
| DD-T07 / FR-T07 | `/technician/units/:id/alerts` / `DiagnosticEvidence` | `alerts.list, alerts.get, alerts.acknowledge, alerts.resolve` | 確認はacknowledgedまで。解消は再測定または権限付き理由記録 | 通信断だけで盗難と断定せず、取り外し検知を別事象 |
| DD-T08 / FR-T08 | `/technician/jobs/:id` / `JobWorkspace` | `jobs.get, jobs.start, jobs.resume, jobs.submit` | 有効割当と開始条件を検証。提出後は品質確認待ち | 失効・取消後の提出は拒否。送信失敗時はドラフトを保持 |
| DD-T09 / FR-T09 | `/technician/jobs/:id` / `ReportEditor` | `jobs.saveDraft, attachments.add, jobs.submit` | 報告本文10〜4000文字、写真JPEG/PNG各5MiB以下・最大10枚（仮）、部品は数量>0 | ドラフトは不完全保存可。提出はschema検証。画像失敗は再選択し本文を保持 |
| DD-T10 / FR-T10 | `/technician/units/:id/control` / `DiagnosticControl` | `commands.create, commands.get` | control.diagnose、設備能力、理由、試運転時間1〜15分（仮）を検証 | 契約制限を試運転で迂回しない。期限切れは自動再送しない |
| DD-T11 / FR-T11 | `/technician/devices` / `DeviceMaintenance` | `devices.list, devices.register, devices.bind, devices.check, devices.calibrate, devices.updateFirmware, devices.get` | serial一意、unitId、センサー種別。校正は単位/参照値/日時、FWは対応版から選択 | 通信断時の更新は開始不可。失敗を新バージョン反映済みと表示しない |
| DD-T12 / FR-T12 | `/technician/devices/:id` / `DeviceEvents` | `devices.get, devices.events, alerts.acknowledge` | eventType、検知根拠を表示。取り外しは専用模擬イベント | 通信復旧でも取り外しアラートを自動で消さない |

## 実装の共通手順

1. セッションとスコープを確認し、ID・URLフィルターをschemaで検証する。
2. Query経由でモックサービスを呼び、画面モデルとして受け取る。
3. フォームはReact Hook Form＋共通schemaを利用し、能力・期間等の検証も適用する。
4. mutation直前に対象のversion・権限・現在状態を照合。高影響操作は対象と理由を確認する。
5. Repositoryで共有デモ状態を変更し、相関ID付きイベントを発行。該当Queryを無効化する。
6. 応答待ちは継続表示し、成功・拒否・失敗を分ける。フォーム失敗時は入力を保持する。

## テストへの引き渡し

設計ID DD-T番号ごとに同番号AT-Tの受入条件、上表の異常系、権限外の直接呼出しを検証する。テストデータと役割横断シナリオは[検証計画](../04-agentic-sdlc/verification.md)を正とする。設計の例示文字数等を変更する場合はschema、文書、境界値試験を同時更新する。

## 機能別詳細仕様（0.5.0）

表の入力はRHFで保持し、schema検証する。read-only値はQueryの単一sourceから表示する。共通の型・ページング・時間・エラーは[実装契約](implementation-contracts.md)を正とし、以下の個別条件を重ねる。視覚値は[UIUX](../03-uiux/UIUXSpecification.md) UX-04/08の参照準拠tokenとpatternを使用する。

### DD-T01 詳細

**一次資料との対応**: SRC-06 BIZ-04, BIZ-08 → FR-T01 → DD-T01。出所区分: 企業原文 SRC-06＋設計補完。本節で具体化する設計補完: 担当期間による表示範囲。フィールド型・必須性・初期値・操作順序は実装提案。

対象: FR-T01 / 主表示pattern: **UI-OVERVIEW**。画面サービス境界は`jobs.list, alerts.list`。

**初期表示と前提**: 社内は担当範囲、外部は自社かつ個別割当・作業期間を取得できる。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| from / to | 日付/必須 | 初期今日、最大366日 | 予定 |
| status | enum/任意 | assigned/in_progress/submitted/completed | 進捗 |
| severity | enum/任意 | critical/warning/all | 優先 |
| summary | 読取 | 担当設備数・未着手・期限超過 | 担当範囲の集計 |

**処理手順**

1. 今日/期間の担当案件を開く → 異常重要度・期限・進捗で並替え → 設備詳細または作業画面へ進む。
2. 保存または操作直前に次の業務ガードを実行する: 未対応はrequested全件ではなく、自分が担当して未着手の案件。社内の横断閲覧も所属テナントと担当範囲を超えない。
3. 参照のみ。予定0件の際は空状態と利用可能な履歴導線。
4. 更新対象Query: `jobs / assignments / alerts`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 外部担当の期限終了をまたぐと当該live設備を隠す。別技術者の案件ID直打ちでも作業開始不可。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-T01-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-T02 詳細

**一次資料との対応**: SRC-06 BIZ-06, BIZ-07, BIZ-10 → FR-T02 → DD-T02。出所区分: 設計補完（企業目的に対応）。本節で具体化する設計補完: 設備台帳の項目と閲覧手順。フィールド型・必須性・初期値・操作順序は実装提案。

対象: FR-T02 / 主表示pattern: **UI-DETAIL**。画面サービス境界は`units.get, devices.list`。

**初期表示と前提**: 対象設備に閲覧可能な担当関係がある。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| unitId | ID/必須 | 担当内 | 設備 |
| manufacturer / model / installedAt | 読取 | 未登録はnull表示 | 台帳 |
| components / serviceScope | 読取 | 対象/対象外を明示 | 点検範囲 |
| capabilityVersion | 読取 | 操作候補の根拠 | 能力版 |

**処理手順**

1. 案件から台帳を開く → 設置場所・型番・構成・設置日・保守範囲を確認 → 診断/作業へ進む。
2. 保存または操作直前に次の業務ガードを実行する: 機種能力はcapability版を表示。未登録/不明項目は未登録と表示し、典型機種の値で補完しない。
3. 参照のみ。技術者はメーカー台帳・顧客所属・請求を変更しない。
4. 更新対象Query: `なし（読取）`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 台帳に設置日なしなら現在日を補わない。外部技術者が非割当設備を参照しても情報を返さない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-T02-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-T03 詳細

**一次資料との対応**: SRC-06 BIZ-08, BIZ-11 → FR-T03 → DD-T03。出所区分: 企業原文 SRC-06＋設計補完。本節で具体化する設計補完: 系列選択と品質表示。フィールド型・必須性・初期値・操作順序は実装提案。

対象: FR-T03 / 主表示pattern: **UI-DETAIL / UI-ANALYSIS**。画面サービス境界は`telemetry.series, telemetry.summary`。

**初期表示と前提**: 対象設備のtelemetry閲覧権。センサーなしでも通信情報は表示可能。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| unitId / metric | 必須 | 担当内・センサー対応 | 対象系列 |
| from / to | ISO日時/必須 | 最大366日 | 期間 |
| observedAt / receivedAt | 読取 | UTC | 鮮度 |
| staleAfterSeconds | 読取 | sensor policy由来、デモ120秒 | stale判定 |
| eventId / version | 読取 | 重複・順序制御 | 更新根拠 |

**処理手順**

1. 指標と期間を選ぶ → センサー/電力/運転/通信の値と時刻を確認 → デモ更新イベントで変更 → 切断時は更新停止を認識する。
2. 保存または操作直前に次の業務ガードを実行する: 観測時刻と受信時刻を分け、staleAfterSeconds超過はstale。古いイベントを最新値へ上書きしない。系列の単位を固定し欠測の間を連結しない。
3. 最新値と時系列を同じeventId/版で整合させる。画面離脱・scope変更で購読解除。
4. 更新対象Query: `telemetry / unit summary`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 順序逆転・重複イベントで値が逆行しない。通信断時も最後の値は時刻付きで残せるがリアルタイムと表示しない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-T03-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-T04 詳細

**一次資料との対応**: SRC-06 BIZ-10 → FR-T04 → DD-T04。出所区分: 企業原文 SRC-06＋設計補完。本節で具体化する設計補完: 点検フォーム・未点検理由。フィールド型・必須性・初期値・操作順序は実装提案。

対象: FR-T04 / 主表示pattern: **UI-FORM**。画面サービス境界は`jobs.get, jobs.saveDraft, jobs.submit`。

**初期表示と前提**: 有効な担当案件がin_progressである。保守範囲と部品別の点検対象が取得済み。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| componentGroup | enum/必須 | indoor固定 | 分類 |
| componentKey | enum/必須 | 上記部品キー | 対象 |
| result | enum/null | 初期null、normal/attention/not_inspected/not_applicable | 点検結果 |
| reason | 文字列/条件必須 | attention等は1〜1000文字 | 根拠/未点検理由 |
| measurements | Measurement配列/任意 | value+unit+observedAt+origin=inspection | 実地測定 |
| attachmentIds | ID配列/任意 | 同じjobに属する画像のみ | 証跡 |

**処理手順**

1. 対象部品ごとに点検結果を選ぶ → 必要な所見・測定・写真を関連付け → 未点検/対象外は理由を記入 → ドラフトまたは報告へ保存する。
2. 保存または操作直前に次の業務ガードを実行する: 対象グループはindoor、項目はfilter / evaporator_coil / blower_motor / blower_fan / drain_pipe / drain_pan / outlet / louver。初期結果はnullで、未点検のまま提出するなら明示的なnot_inspectedと理由が必要。設備に存在しない部品はnot_applicableと理由を記録する。
3. 点検結果を報告版・作者・観測時刻へ紐付ける。センサー推定は別の根拠として残し、現地点検結果で元データを上書きしない。
4. 更新対象Query: `report draft / inspection items`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 未入力、単位なし測定、未点検理由なし、他案件の写真参照を拒否。正常を初期選択して空点検を完了扱いしない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-T04-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-T05 詳細

**一次資料との対応**: SRC-06 BIZ-10 → FR-T05 → DD-T05。出所区分: 企業原文 SRC-06＋設計補完。本節で具体化する設計補完: 点検フォーム・未点検理由。フィールド型・必須性・初期値・操作順序は実装提案。

対象: FR-T05 / 主表示pattern: **UI-FORM**。画面サービス境界は`jobs.get, jobs.saveDraft, jobs.submit`。

**初期表示と前提**: 有効な担当案件がin_progressである。保守範囲と部品別の点検対象が取得済み。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| componentGroup | enum/必須 | outdoor固定 | 分類 |
| componentKey | enum/必須 | 上記部品キー | 対象 |
| result | enum/null | 初期null、normal/attention/not_inspected/not_applicable | 点検結果 |
| reason | 文字列/条件必須 | attention等は1〜1000文字 | 根拠/未点検理由 |
| measurements | Measurement配列/任意 | value+unit+observedAt+origin=inspection | 実地測定 |
| attachmentIds | ID配列/任意 | 同じjobに属する画像のみ | 証跡 |

**処理手順**

1. 対象部品ごとに点検結果を選ぶ → 必要な所見・測定・写真を関連付け → 未点検/対象外は理由を記入 → ドラフトまたは報告へ保存する。
2. 保存または操作直前に次の業務ガードを実行する: 対象グループはoutdoor、項目はcondenser_coil / compressor / fan / blade / refrigerant_pipe。初期結果はnullで、未点検のまま提出するなら明示的なnot_inspectedと理由が必要。設備に存在しない部品はnot_applicableと理由を記録する。
3. 点検結果を報告版・作者・観測時刻へ紐付ける。センサー推定は別の根拠として残し、現地点検結果で元データを上書きしない。
4. 更新対象Query: `report draft / inspection items`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 未入力、単位なし測定、未点検理由なし、他案件の写真参照を拒否。正常を初期選択して空点検を完了扱いしない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-T05-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-T06 詳細

**一次資料との対応**: SRC-06 BIZ-10 → FR-T06 → DD-T06。出所区分: 企業原文 SRC-06＋設計補完。本節で具体化する設計補完: 点検フォーム・未点検理由。フィールド型・必須性・初期値・操作順序は実装提案。

対象: FR-T06 / 主表示pattern: **UI-FORM**。画面サービス境界は`jobs.get, jobs.saveDraft, jobs.submit`。

**初期表示と前提**: 有効な担当案件がin_progressである。保守範囲と部品別の点検対象が取得済み。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| componentGroup | enum/必須 | electrical固定 | 分類 |
| componentKey | enum/必須 | 上記部品キー | 対象 |
| result | enum/null | 初期null、normal/attention/not_inspected/not_applicable | 点検結果 |
| reason | 文字列/条件必須 | attention等は1〜1000文字 | 根拠/未点検理由 |
| measurements | Measurement配列/任意 | value+unit+observedAt+origin=inspection | 実地測定 |
| attachmentIds | ID配列/任意 | 同じjobに属する画像のみ | 証跡 |

**処理手順**

1. 対象部品ごとに点検結果を選ぶ → 必要な所見・測定・写真を関連付け → 未点検/対象外は理由を記入 → ドラフトまたは報告へ保存する。
2. 保存または操作直前に次の業務ガードを実行する: 対象グループはelectrical、項目はthermostat / sensor / capacitor / contactor / wiring。初期結果はnullで、未点検のまま提出するなら明示的なnot_inspectedと理由が必要。設備に存在しない部品はnot_applicableと理由を記録する。
3. 点検結果を報告版・作者・観測時刻へ紐付ける。センサー推定は別の根拠として残し、現地点検結果で元データを上書きしない。
4. 更新対象Query: `report draft / inspection items`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 未入力、単位なし測定、未点検理由なし、他案件の写真参照を拒否。正常を初期選択して空点検を完了扱いしない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-T06-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-T07 詳細

**窓開放・断熱不足の負荷通知の表示・処理設計（BIZ-17）**

alerts.listのAlertにcauseCode（window_open / insulation_loss / unknown）、evidenceKind（demo_observation / inferred / inspection）、evidenceText、observedAtを追加する。causeCodeとevidenceKindは必須、根拠未取得はunknownとし、倍増などの数値を固定表示しない。推定は「疑い」、点検結果は「点検記録」と表示。通知詳細から同じunitIdの設備・保守依頼へ移動する。

検証: AT-T07-SRC — 窓開放の疑い・断熱不足の点検記録・根拠なしの3 fixtureで、文言・根拠・時刻が異なり、既読にしても異常が解消しない。

**一次資料との対応**: SRC-06 BIZ-08, BIZ-11, BIZ-17 → FR-T07 → DD-T07。出所区分: 企業原文 SRC-06＋設計補完。本節で具体化する設計補完: 原因候補と根拠・解消手順。フィールド型・必須性・初期値・操作順序は実装提案。

対象: FR-T07 / 主表示pattern: **UI-DETAIL**。画面サービス境界は`alerts.list, alerts.get, alerts.acknowledge, alerts.resolve`。

**初期表示と前提**: 担当設備に異常または診断の疑いがある。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| alertId | ID/必須 | 担当設備の事象 | 対象 |
| evidenceIds | 読取配列 | telemetry/inspectionを識別 | 根拠 |
| resolutionReason | 文字列/解消時必須 | 1〜2000文字 | 解消判断 |
| resolutionEvidenceIds | ID配列/解消時 | 再測定IDまたは確認記録 | 検証 |
| expectedVersion | 整数/必須 | 最新Alert版 | 競合 |

**処理手順**

1. 異常一覧から根拠を開く → 計測/推定/現地点検と履歴を確認 → 確認済みにする → 必要なら再測定・理由付き解消へ進む。
2. 保存または操作直前に次の業務ガードを実行する: 推定に確信度がない場合は数値の確率を生成しない。確認操作はacknowledged。解消には再測定で設定条件を満たすかalert.resolve権限と理由が必要。
3. 検知、確認、解消の各時刻と主体を保持。再発は新alertIdで旧事象と関連付ける。
4. 更新対象Query: `alerts / alert events / customer summary / admin summary / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: Job完了だけではresolvedにしない。通信断だけの根拠で盗難と断定せず、取り外し専用事象と分離する。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-T07-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-T08 詳細

**一次資料との対応**: SRC-06 BIZ-12 → FR-T08 → DD-T08。出所区分: 企業原文 SRC-06＋設計補完。本節で具体化する設計補完: 開始・提出・再提出の状態。フィールド型・必須性・初期値・操作順序は実装提案。

対象: FR-T08 / 主表示pattern: **UI-DETAIL**。画面サービス境界は`jobs.get, jobs.start, jobs.resume, jobs.submit`。

**初期表示と前提**: assigned案件を担当し、有効期間内。定期/事後/予防は同じ作業状態モデル。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| jobId | ID/必須 | 有効担当 | 対象 |
| startConfirmed | boolean/開始時必須 | 初期false | 対象確認 |
| reportVersion | 整数/提出時必須 | 現在draft版 | 提出物 |
| expectedVersion | 整数/必須 | 現在job版 | 状態競合 |

**処理手順**

1. 担当と予定を確認 → 作業開始 → 報告を編集 → 提出 → 品質確認待ち → 差戻しなら再作業と再提出。
2. 保存または操作直前に次の業務ガードを実行する: 開始可能はassigned、差戻し再開はrework_requested。submittedでは提出版を読取専用。顧客承認を技術者が代行しない。
3. 開始時刻・提出時刻・reportVersionを保存。完了は品質担当のreviewで決定。
4. 更新対象Query: `jobs / reports / job events / notifications / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 未割当、取消、on_hold、期限外のstart/submitを拒否。提出失敗ではin_progressとドラフトを保持する。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-T08-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-T09 詳細

**一次資料との対応**: SRC-06 BIZ-12 → FR-T09 → DD-T09。出所区分: 設計補完（企業目的に対応）。本節で具体化する設計補完: 写真・交換部品・報告版の管理。フィールド型・必須性・初期値・操作順序は実装提案。

対象: FR-T09 / 主表示pattern: **UI-FORM**。画面サービス境界は`jobs.saveDraft, attachments.add, jobs.submit`。

**初期表示と前提**: in_progressまたは再作業中。点検項目とドラフトを取得済み。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| workText | 文字列/提出時必須 | 10〜4000文字 | 実施内容 |
| inspectionItems | 配列/提出時必須 | 対象全部に結果/理由 | チェックリスト |
| photos | Attachment配列/任意 | JPEG/PNG、<=5MiB×10、readyのみ提出 | 写真 |
| parts | 配列/任意 | name1〜120文字、quantity正整数<=999 | 交換部品 |
| nextAction.kind | enum/必須 | none/follow_up | 次回対応 |
| nextAction.date / note | 日時・文字列/条件必須 | follow_up時に未来日時と1〜1000文字 | 次回計画 |

**処理手順**

1. 点検・測定・写真・交換部品・作業本文・次回対応を入力 → ドラフト保存 → 提出前チェック → 報告版を確定する。
2. 保存または操作直前に次の業務ガードを実行する: 報告は作者・版を保持。画像はJPEG/PNG、1枚5MiB以下、最大10枚。数量は正整数、次回対応はnoneまたは日時/内容を明示。サービス再取得でdirty内容を消さない。
3. 保存成功時にdraft版を更新し、提出時は固定したreportVersionをJobへ紐付ける。写真削除時にobject URLを解放する。
4. 更新対象Query: `draft / attachments / reports / jobs（提出時）`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 本文9/4001文字、偽MIME、11枚目、5MiB超過、部品数量0、写真アップロード失敗を検証。失敗写真が残るまま提出不可。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-T09-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-T10 詳細

**一次資料との対応**: SRC-06 BIZ-13 → FR-T10 → DD-T10。出所区分: 設計補完（企業目的に対応）。本節で具体化する設計補完: 技術者の操作権限と試運転手順。フィールド型・必須性・初期値・操作順序は実装提案。

対象: FR-T10 / 主表示pattern: **UI-DETAIL / UI-FORM**。画面サービス境界は`commands.create, commands.get`。

**初期表示と前提**: 担当期間内、control.diagnose能力、機器online。契約制限を超えない。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| jobId / unitId | ID/必須 | 一致する担当 | 根拠 |
| action | Action/必須 | 能力・制限内 | 診断動作 |
| durationMinutes | 整数/試運転必須 | 1〜15、初期5 | 試運転時間 |
| reason | 文字列/必須 | 1〜1000文字 | 操作理由 |
| endAction | Action/試運転必須 | 能力内、確認ダイアログに表示 | 終了動作 |

**処理手順**

1. 現在状態・担当案件を確認 → 診断操作・試運転時間・理由を指定 → 確認 → Commandの応答と履歴を見る。
2. 保存または操作直前に次の業務ガードを実行する: 同時FW更新や未完了Command中は開始不可。試運転終了も終了Commandの応答が必要で、ブラウザタイマー終了を実停止扱いしない。
3. 理由とjobId付きCommandを作成。終了予定と終了応答を別表示し、終了失敗は注意として残す。
4. 更新対象Query: `commands / unit detail / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 制限温度の迂回、担当期間外、16分の試運転、理由なしを拒否。終了応答がなければ停止済みにしない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-T10-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-T11 詳細

**一次資料との対応**: SRC-06 BIZ-20 → FR-T11 → DD-T11。出所区分: 設計補完（企業目的に対応）。本節で具体化する設計補完: 登録・校正・更新の模擬手順。フィールド型・必須性・初期値・操作順序は実装提案。

対象: FR-T11 / 主表示pattern: **UI-LIST / UI-FORM / UI-DETAIL**。画面サービス境界は`devices.list, devices.register, devices.bind, devices.check, devices.calibrate, devices.updateFirmware, devices.get`。

**初期表示と前提**: device.maintainと対象設備の有効担当。登録/校正/更新はモック。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| serial | 文字列/登録必須 | 英数ハイフン3〜64文字、正規化後一意 | 識別子 |
| unitId | ID/紐付け必須 | 担当・登録済み | 対象 |
| metric / unit | enum/校正必須 | センサー能力の組合せ | 測定種別 |
| referenceValue / measuredValue | number/校正必須 | 有限値、同じ単位 | 校正根拠 |
| calibratedAt | ISO日時/必須 | 未来不可 | 校正時刻 |
| firmwareVersion | enum/更新必須 | 対応候補・現行版以外 | 目標版 |

**処理手順**

1. serial登録 → 設備紐付け → 接続確認 → 校正値と参照を記録 → 対応FW候補を選び更新 → 進行/結果を確認する。
2. 保存または操作直前に次の業務ガードを実行する: serialはtrim/大文字化して一意判定。校正は履歴追加、既存測定値は書き換えない。FWは対応版リストからのみ選択。URLやバイナリを自由入力させない。
3. Device、CalibrationRecord、DeviceOperationを保持。succeededだけfirmwareVersion更新。更新中は競合制御不可。
4. 更新対象Query: `devices / operations / calibrations / capabilities / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 重複serial、別設備への無断再紐付け、offline更新、単位不一致、失敗更新を検証。失敗しても旧FW版を保持。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-T11-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-T12 詳細

**一次資料との対応**: SRC-06 BIZ-20 → FR-T12 → DD-T12。出所区分: 企業原文 SRC-06＋設計補完。本節で具体化する設計補完: 通信断・電源断と取り外しの区別。フィールド型・必須性・初期値・操作順序は実装提案。

対象: FR-T12 / 主表示pattern: **UI-DETAIL / UI-TIMELINE**。画面サービス境界は`devices.get, devices.events, alerts.acknowledge`。

**初期表示と前提**: 担当Deviceにイベント閲覧権がある。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| deviceId | ID/必須 | 担当内 | 対象 |
| eventType | enum/デモ制御必須 | communication_lost/power_lost/tamper/restored | 事象 |
| evidenceSource | enum/必須 | heartbeat/power_signal/tamper_signal | 根拠 |
| responseNote | 文字列/対応時必須 | 1〜1000文字 | 確認内容 |
| occurredAt / restoredAt | 読取 | デモ時計由来 | 検知・復旧 |

**処理手順**

1. 通信断・電源断・取り外し検知を別々に模擬発火 → 通知を確認 → 対応メモ → 復旧/確認を記録する。
2. 保存または操作直前に次の業務ガードを実行する: connectionとtamperを独立管理。電源断判定は電源専用信号のデモがある時のみ。heartbeatなしから電源断と断定しない。
3. 検知時刻・観測根拠・対応・復旧時刻を別イベントで保持。通知の確認は物理状態を変更しない。
4. 更新対象Query: `devices / alerts / device events / notifications / audit`。読取だけのケースは業務mutationを発行しない。複数の表示状態は共有モックの遷移関数でまとめて更新する。

**競合・失敗時**: 通信再接続で未確認のtamperアラートが消えない。out-of-orderの古いheartbeatでonlineへ戻さない。 共通DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。CONFLICT後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-T12-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

