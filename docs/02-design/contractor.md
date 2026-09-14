---
document_id: DD-P
version: 0.2.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# 施工業者 詳細設計書

## 入力・責務

入力: [役割別要件](../01-requirements/contractor.md)、[共通要件](../01-requirements/common.md)。必読: [共通詳細設計](common.md)、[UIUX仕様書](../03-uiux/UIUXSpecification.md)。以下はPROPOSEDのフロントエンド契約であり、稼働中のAPI仕様ではない。

ルートパラメーターは未信頼入力として検証する。表のservice名は共通Repositoryの論理操作名。同じルートの行は同一画面内の機能を分担する。全行にloading/empty/error/forbidden/not-foundを実装する。再試行は回復可能なエラーだけに提供し、権限不足では許可された画面へ戻す。

## 画面・処理設計

| 設計ID / 要件 | ルート / 主コンポーネント | 取得・操作契約 | 入力・処理・検証 | 異常系と禁止事項 |
|---|---|---|---|---|
| DD-P01 / FR-P01 | `/partner` / `PartnerOverview` | `jobs.list, jobs.get` | 委託先組織IDはセッション由来。期限超過と設備緊急度は別表示 | 案件0は空状態。アクセス失効時に前の集計を消す |
| DD-P02 / FR-P02 | `/partner/jobs/:id` / `PartnerJob` | `jobs.get, jobs.accept, jobs.decline` | 受諾は有効期間内の自社offerのみ。辞退理由1〜1000文字（仮） | 期限切れ・HQ取消・他更新は409扱いで再取得。辞退で案件を消さない |
| DD-P03 / FR-P03 | `/partner/schedule` / `AssignmentEditor` | `jobs.list, members.eligible, jobs.assign` | 技術者ID、開始/終了、必要資格。作業期間が委託期間内か検証 | 確定した重複予定は保存拒否し再調整。作業開始後の再割当は理由必須で旧アクセス取消 |
| DD-P04 / FR-P04 | `/partner/units/:id` / `PartnerUnit` | `units.get, alerts.list, telemetry.summary` | 設備IDから有効な受託案件を照合。顧客連絡先は必要最小限 | 受託期限終了後の直接URLも拒否。遠隔操作ボタンなし |
| DD-P05 / FR-P05 | `/partner/jobs/:id/review` / `QualityReview` | `jobs.get, jobs.review` | 提出済みのみ。受理/差戻し、理由（差戻し必須）。改版前レポートも保持 | 技術者の原報告を上書き不可。作業者自身の品質承認不可 |
| DD-P06 / FR-P06 | `/partner/team` / `TeamCapacity` | `members.list, jobs.list` | 自社スコープに限定した日付・資格フィルター。資格は架空のデモ属性 | 所属失効の候補は割当不可。ユーザー登録はHQへ引き継ぐ |
| DD-P07 / FR-P07 | `/partner/history` / `PartnerHistory` | `jobs.events, jobs.addNote, notifications.preview` | jobIdとテンプレート。メモ1〜2000文字（仮）、宛先は権限内から選ぶ | 外部宛先自由入力不可。機密の顧客請求情報をテンプレートに含めない |
| DD-P08 / FR-P08 | `/partner/*` / `PartnerAccessGuard` | `session.get, jobs.get` | 各操作直前にも受託・期間を検証。期限はデモ時計基準 | 期限境界で画面表示中でも操作を拒否しキャッシュ破棄 |

## 実装の共通手順

1. セッションとスコープを確認し、ID・URLフィルターをschemaで検証する。
2. Query経由でRepositoryを呼び、DTOを共通domainモデルへ変換する。
3. フォームはReact Hook Form＋共通schemaを利用し、能力・期間等の検証も適用する。
4. mutation直前に対象のversion・権限・現在状態を照合。高影響操作は対象と理由を確認する。
5. Repositoryで共有デモ状態を変更し、相関ID付きイベントを発行。該当Queryを無効化する。
6. 応答待ちは継続表示し、成功・拒否・失敗を分ける。フォーム失敗時は入力を保持する。

## テストへの引き渡し

設計ID DD-P番号ごとに同番号AT-Pの受入条件、上表の異常系、権限外の直接呼出しを検証する。テストデータと役割横断シナリオは[検証計画](../04-agentic-sdlc/verification.md)を正とする。設計の例示文字数等を変更する場合はschema、文書、境界値試験を同時更新する。

## 機能別詳細仕様（0.2.0）

表の入力はRHFで保持し、schema検証する。read-only値はQueryの単一sourceから表示する。共通の型・ページング・時間・エラーは[実装契約](implementation-contracts.md)を正とし、以下の個別条件を重ねる。視覚値は[UIUX](../03-uiux/UIUXSpecification.md) UX-04/08の参照準拠tokenとpatternを使用する。

### DD-P01 詳細

対象: FR-P01 / 主表示pattern: **UI-OVERVIEW**。API境界は`jobs.list, jobs.get`。

**初期表示と前提**: 有効な施工業者Membershipがある。HQからのofferは受諾前でも案件概要を閲覧可能。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| status | enum/任意 | offered/accepted/assigned/in_progress/submitted/all | 絞込 |
| from / to | 日付/任意 | 最大366日 | 日程範囲 |
| contractorOrgId | セッション由来 | 入力不可 | 自社境界 |
| summary | 読取 | offerCount/activeCount/reviewCount/overdueCount/asOf | 概要 |

**処理手順**

1. 自社の受諾待ち・予定・進行・品質確認待ちを集計 → 状態を選んで案件一覧へ → 対象案件の窓口操作へ進む。
2. 保存または操作直前に次の業務ガードを実行する: 受諾前は案件種別・地域・必要資格・日程候補までの最小情報。詳細設備値や顧客連絡情報は受諾後かつ期間内に限定する。
3. 閲覧による受諾は発生しない。KPIと一覧の対象集合を同じ検索条件で揃える。
4. 更新対象Query: `jobs / partner summary（イベント時）`。読取だけのケースは業務mutationを発行しない。複数更新はRepository内の1コマンドでまとめる。

**競合・失敗時**: 他社のofferを件数に含めない。委託期間終了で設備閲覧を閉じても、自社の受諾/辞退履歴の最小記録は確認できる。 共通HTTP/DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。409後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-P01-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-P02 詳細

対象: FR-P02 / 主表示pattern: **UI-DETAIL**。API境界は`jobs.get, jobs.accept, jobs.decline`。

**初期表示と前提**: 自社宛offered、offerExpiresAtより前、HQ未取消。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| jobId / offerId | ID/必須 | 最新の自社宛offer | 判断対象 |
| decision | enum/必須 | accept/decline | 判断 |
| reason | 文字列/辞退時必須 | 1〜1000文字 | 辞退理由 |
| expectedVersion | 整数/必須 | 表示した案件版 | 競合検出 |
| termsVersion | 読取・送信時必須 | 委託条件版 | 確認根拠 |

**処理手順**

1. 案件の最小情報・委託条件を確認 → 受諾または理由付き辞退 → HQと自社の状態表示を更新する。
2. 保存または操作直前に次の業務ガードを実行する: 受諾は技術者割当や確定予約とは別。辞退時はrequestedへ戻し、辞退者・理由・offerIdを保持。再委託は新offerId。
3. 受諾でaccepted、必要な設備閲覧権を委託期間内だけ開く。辞退は詳細閲覧権を付与しない。
4. 更新対象Query: `jobs / offers / partner summary / admin summary / notifications / audit`。読取だけのケースは業務mutationを発行しない。複数更新はRepository内の1コマンドでまとめる。

**競合・失敗時**: 締切一致時は辞退/受諾を拒否して再取得。受諾直前のHQ取消との409競合は自動上書きしない。 共通HTTP/DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。409後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-P02-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-P03 詳細

対象: FR-P03 / 主表示pattern: **UI-LIST / UI-FORM**。API境界は`jobs.list, members.eligible, jobs.assign`。

**初期表示と前提**: 受諾済み案件で、自社の割当管理権限を持つ。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| jobId | ID/必須 | acceptedまたは再割当可能状態 | 対象 |
| technicianMembershipId | ID/必須 | 自社かつ有効・資格一致 | 担当 |
| startAt / endAt | ISO日時/必須 | start<end、委託期間内、確定重複なし | 作業枠 |
| reason | 文字列/再割当時必須 | 1〜1000文字 | 変更理由 |
| expectedVersion | 整数/必須 | 最新取得版 | 競合 |

**処理手順**

1. 希望枠と委託期間を確認 → 自社の有効技術者・資格・空き時間を検索 → 作業開始/終了を指定 → 割当確認 → 日程と担当を共有する。
2. 保存または操作直前に次の業務ガードを実行する: 候補検索だけで権限判定を済ませず保存直前に所属・資格・期間を再照合。重複予定は警告し、同時間帯の確定重複は1Aでは保存拒否する。
3. Assignment作成、scheduledSlot確定、Job assigned。再割当は旧割当を失効、元報告の著者を維持し変更理由を保存。
4. 更新対象Query: `jobs / assignments / eligible members / schedule / notifications / audit`。読取だけのケースは業務mutationを発行しない。複数更新はRepository内の1コマンドでまとめる。

**競合・失敗時**: 他社/無資格/委託外期間の割当を拒否。作業中の再割当は理由なしでは保存できず、旧技術者は直ちに変更操作不可。 共通HTTP/DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。409後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-P03-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-P04 詳細

対象: FR-P04 / 主表示pattern: **UI-DETAIL**。API境界は`units.get, alerts.list, telemetry.summary`。

**初期表示と前提**: 受諾済みの有効委託に対象設備が含まれる。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| unitId | route ID/必須 | 有効受託案件から解決 | 設備 |
| jobId | query ID/必須 | 該当委託を識別 | 閲覧根拠 |
| unit / capability | 読取 | 型番・保守範囲 | 台帳 |
| telemetry / evidence | 読取 | 時刻・品質付き | 異常根拠 |

**処理手順**

1. 案件から設備を開く → 場所・型番・保守範囲・通信状態・異常根拠を閲覧 → 案件へ戻る。
2. 保存または操作直前に次の業務ガードを実行する: 診断に必要な値を読取専用表示。請求、支払い、他契約、全顧客履歴は取得しない。serialや場所は必要範囲に限定する。
3. 業務更新なし。委託終了後は顧客設備のlive値を表示せず、自社案件履歴の最小参照へ戻す。
4. 更新対象Query: `なし（読取）`。読取だけのケースは業務mutationを発行しない。複数更新はRepository内の1コマンドでまとめる。

**競合・失敗時**: 画面を開いたまま期限を越えても次要求で拒否・キャッシュ破棄。監視値を見られることを制御権限と扱わない。 共通HTTP/DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。409後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-P04-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-P05 詳細

対象: FR-P05 / 主表示pattern: **UI-DETAIL / UI-FORM**。API境界は`jobs.get, jobs.review`。

**初期表示と前提**: submittedの自社委託案件。レビュー担当は報告作成者と異なる。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| jobId / reportVersion | ID・整数/必須 | submittedの現在版 | 対象 |
| decision | enum/必須 | accept/return | 品質判断 |
| reason | 文字列/差戻し時必須 | 1〜2000文字 | 指摘 |
| reviewerId | セッション由来 | report.authorIdと異なる | 責任者 |

**処理手順**

1. 提出版の点検・写真・測定・作業・次回対応を確認 → 受理または差戻し理由を記入 → 技術者/顧客/HQへ結果を共有する。
2. 保存または操作直前に次の業務ガードを実行する: 受理は必須点検の記録と根拠が揃うこと。未点検/対象外の理由が妥当か確認。レビュー担当は技術者の原記録を編集しない。
3. 受理でcompleted、差戻しでrework_requested。レビュー履歴を報告版へ紐付ける。完了時もAlertを自動解消しない。
4. 更新対象Query: `jobs / reports / job events / notifications / audit`。読取だけのケースは業務mutationを発行しない。複数更新はRepository内の1コマンドでまとめる。

**競合・失敗時**: 自己承認、古いreportVersion、空の差戻し理由を拒否。差戻し後に古い受理操作が届いてもcompletedにしない。 共通HTTP/DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。409後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-P05-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-P06 詳細

対象: FR-P06 / 主表示pattern: **UI-LIST**。API境界は`members.list, jobs.list`。

**初期表示と前提**: 自社の作業者一覧を読む権限。ユーザー作成権限は含まない。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| date | 日付/必須 | 初期デモ当日 | 予定対象日 |
| qualification | 文字列/任意 | 資格台帳コード | 資格絞込 |
| activeOnly | boolean/必須 | 初期true | 有効所属 |
| members / slots | 読取 | 自社の必要な氏名・資格・期間・割当 | 予定 |

**処理手順**

1. 日付・資格・有効/失効を選ぶ → 自社技術者の割当と空き枠を確認 → 対象案件の割当へ進む。
2. 保存または操作直前に次の業務ガードを実行する: 稼働率は割当時間/表示期間の設定作業可能時間で、分母未設定なら割合なし。個人の位置追跡・他社予定を表示しない。
3. 閲覧だけで所属や資格を更新しない。所属変更はHQへ調整依頼。
4. 更新対象Query: `members / assignments（閲覧）`。読取だけのケースは業務mutationを発行しない。複数更新はRepository内の1コマンドでまとめる。

**競合・失敗時**: 会社IDをURLから書換えて他社名簿を取得できない。失効した技術者を候補として選べない。 共通HTTP/DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。409後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-P06-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-P07 詳細

対象: FR-P07 / 主表示pattern: **UI-TIMELINE / UI-FORM**。API境界は`jobs.events, jobs.addNote, notifications.preview`。

**初期表示と前提**: 自社の案件に連絡・履歴閲覧権がある。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| jobId | ID/必須 | 自社案件 | 対象 |
| templateKey | enum/必須 | schedule_change/report_return/completion | 文面 |
| message | 文字列/必須 | 1〜2000文字 | 連絡内容 |
| visibility | enum/必須 | internal/customer、初期internal | 公開範囲 |
| recipientRole | enum/必須 | hq/assigned_technician/customer_contact | 宛先 |
| channel | enum/必須 | inApp/email_preview/whatsapp_preview | 連絡手段 |

**処理手順**

1. 案件履歴を開く → 日程調整/品質連絡テンプレートを選ぶ → メモ・宛先役割を確認 → in-app記録と外部送信プレビューを作成する。
2. 保存または操作直前に次の業務ガードを実行する: 実送信はしない。宛先は該当案件のHQ/担当技術者/顧客窓口のみ。内部品質メモは顧客非公開を初期値とする。
3. Noteを作者・公開範囲付きで保存。プレビュー操作だけではdeliveryStateをsentへ変更しない。
4. 更新対象Query: `job notes / job events / notification previews / audit`。読取だけのケースは業務mutationを発行しない。複数更新はRepository内の1コマンドでまとめる。

**競合・失敗時**: 任意メール宛先・他案件の宛先・顧客請求の転記を拒否。保存失敗でメモを保持する。 共通HTTP/DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。409後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-P07-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

### DD-P08 詳細

対象: FR-P08 / 主表示pattern: **全patternのGuard**。API境界は`session.get, jobs.get`。

**初期表示と前提**: 施工業者としてアクセスするすべてのrouteとRepository操作。 ルート/条件検証→session scope→必要Queryの順に取得し、未取得状態と0件を分ける。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| membershipId / scopeVersion | セッション由来/必須 | 有効role=contractor | 主体 |
| jobId / tenantId | 入力・サーバー照合 | 組織/委託が一致 | 対象 |
| validFrom / validUntil | 読取 | from<=now<until | 期間 |
| action | operation enum/必須 | roleのallowlist | 操作 |

**処理手順**

1. Membership確認 → 自社offer/委託/期間を確認 → 最小データを返す → 操作直前に再確認する。
2. 保存または操作直前に次の業務ガードを実行する: live設備は受諾済みかつ期間内だけ。失効後の履歴は自社受諾/辞退/作業の最小記録で、過去アクセスを理由に顧客データ全件を返さない。
3. 拒否では業務変更0件。監査には秘密を含めず拒否理由コードを記録。session切替で旧Queryを破棄。
4. 更新対象Query: `scope変更時に全旧cache`。読取だけのケースは業務mutationを発行しない。複数更新はRepository内の1コマンドでまとめる。

**競合・失敗時**: 他社jobId、期限一致、未受諾の設備、請求変更、制限操作をそれぞれ拒否し、禁止対象のデータを応答へ含めない。 共通HTTP/DomainErrorは[実装契約](implementation-contracts.md)の表示・再試行表へ変換する。409後の再送は更新された対象を利用者が確認してから行う。

**監査と通知**: 業務変更は`action / actorMembershipId / targetId / previousVersion / nextVersion / correlationId / result / occurredAt`を記録。取得/検索は業務履歴へ成功イベントを乱造せず、アクセス拒否のみ監査対象とする。機能の通知先・公開タイミングは[通知・公開契約](implementation-contracts.md#通知と公開範囲)に従う。

**検証**: AT-P08-N/E/Bと該当Sシナリオ。フォームの必須/最小/最大/境界直外を検証し、能力/担当期間の変化があるケースは保存直前にも検証する。

