# DOC-0.20.0 独立G1レビュー

**判定: FAIL（MAJOR 1件、MINOR 4件）。** 静的検証の成功だけでは、受入条件と生成DTOの整合性を保証できない。

- 対象コミット: `717c545cccff8ab11932fab016c854ff8526897f`。仕様修正者はClaude/Opus 5、今回のレビュー担当はCodex。
- 対象baseline: `4f45da02ef91f0cf93075848d6ded3373878521740aa2f4047673eee709e4acc`。63ファイルとbaseline IDを独立に再計算し一致。
- レビュー範囲: G1-001〜031の修正契約・受入計画、関連する認可・状態遷移・正規DTO・通知・fixture・画面/Query契約。契約、fixture、UI/要件を別担当で照合し、主担当が指摘を確認した。
- 文書レビューであり、全受入subcaseのアプリ実行や企業承認ではない。アプリ実装/動作試験は未実施。DEC-54〜59はPROPOSEDのまま。

## 検証証跡

- [静的検証](static-check.json): `python3 docs/tools/validate_documents.py --tsc /private/tmp/claude-501/-Users-ji1wxs-PraditaProjects-AC-Project/ee33c3d9-cdb9-460b-a78b-8810c5cf9104/scratchpad/ts/package/lib/tsc.js`、exit 0、errors=[]、TypeScript strict passed。
- [変異テスト](validator-negative-checks.json): `python3 docs/tools/check_review_regressions.py`、exit 0、84/84検出。
- [baseline照合](baseline-integrity-check.json): SHA-256全63件一致。既存の自己レビュー結果を独立判定として流用していない。

## 指摘

### G120-001 / MAJOR — 空気環境方針の受入で60秒の継続条件が成立していない

- 場所: `docs/01-requirements/admin.md:324`, `docs/04-agentic-sdlc/fixture-contract.json:3590`, `docs/04-agentic-sdlc/fixture-contract.json:3603`, `docs/02-design/review-resolution-contracts.md:795`, `docs/02-design/deterministic-contracts.md:117`
- 関連: FR-A12 / 旧指摘 G1-005, G1-009
- 再現条件: AT-A12-N / AT-G120-005のpatchを01:00Zで適用し、durationSeconds=60の新しいPolicyを保存。同tickにobservedAt=00:59ZのFactを初めてfireする。
- 問題: 通常の60秒tick、継続カウンタ、事前の方針評価を作らないまま通知createdを期待している。D08は条件の連続成立を必要とし、遅着した過去観測から過去Alertを新規発生させない。過去のobservedAtだけでは新規方針の継続成立を証明できない。IR97のセンサー追加は元のmissing_dataを解消するが、この不整合は残る。
- 影響: 正しい継続時間判定の実装が受入に失敗するか、受入に合わせた実装が継続時間を迂回する。少なくとも通知createdの期待値を一意に実現できない。
- 修正案: 保存後の現在時刻で両設備へ有効なFactを投入し、60秒の評価経過を明記する。59秒と60秒の通知期待値、換気Commandの発生タイミングをD08/SR25と整合させ、fixture・AT-A12-N/B・AT-G120-005を同時更新する。

### G120-002 / MINOR — Alert通知のtypeがIR95と正規DTOで矛盾する

- 場所: `docs/02-design/review-resolution-contracts.md:753`, `docs/02-design/review-resolution-contracts.md:769`, `docs/02-design/review-resolution-contracts.md:64`, `docs/02-design/service-contracts.ts:168`
- 関連: FR-X07, FR-C08, FR-T07, FR-A05 / 旧指摘 G1-002
- 再現条件: demo.trigger(load_alert)でpolicyId=null、type=sensorのAlertを作り、IR95による通知を生成する。
- 問題: IR95のtemplateKeyとtypeは同名という規則はtype=alertを要求するが、NotificationTypeにalertはない。IR10は同じ通知をtype=faultに写像する。IR72では実装側に矛盾の選択を委ねられない。
- 影響: 通知のDTO検証と故障分類・フィルターが一致しない。修正すべき既存写像は明確なためMINORとする。
- 修正案: alertテンプレートを同名規則の例外とし、IR10の分類とsourceAlertIdの規則を適用する。実際の生成通知のtype=faultを受入に追加する。

### G120-003 / MINOR — 業務通知のseverityが未定義

- 場所: `docs/02-design/review-resolution-contracts.md:753`, `docs/02-design/service-contracts.ts:110`
- 関連: FR-X07, FR-P03, FR-C08 / 旧指摘 G1-002
- 再現条件: AT-P03-N / AT-G120-002でcontractor-aが割当を行い、4宛先へのschedule_change通知を生成する。
- 問題: IR95は宛先・template・channel等を定めるが、必須のNotification.severityを定めない。参照先D08/D12にも非Alert業務通知の既定値はない。
- 影響: normal/warning/criticalの選択が実装依存となり、表示色・重大度フィルター・既定の並び順が一意にならない。
- 修正案: 業務イベント別のseverity、または既定値と例外を定める。Alertについては起点からの継承を明記し、受入で確認する。

### G120-004 / MINOR — アレルゲン観測更新の変更イベントとQuery無効化が未定義

- 場所: `docs/02-design/review-resolution-contracts.md:804`, `docs/02-design/review-resolution-contracts.md:446`, `docs/02-design/service-contracts.ts:315`
- 関連: FR-C07, FR-A12, NFR-05 / 旧指摘 G1-006
- 再現条件: C07/A12のtelemetry.seriesを取得済みにし、同じUnitにdemo.trigger(allergen)で新しい観測を追加する。
- 問題: IR98が新たな保存先を定義したが、閉じたIR71の変更イベント表とChangeEntityTypeに対応項目がない。measurementへ写す規則もなく、telemetry.seriesの再取得を決められない。
- 影響: 保存済みの最新観測と、表示中の取得状態・値が食い違う可能性がある。
- 修正案: 専用entityTypeを追加するか既存イベントへの写像を明記し、entityId/versionと対象Queryを定義する。キャッシュ済み画面が更新される受入を追加する。

### G120-005 / MINOR — 通知fixtureの必須scopeVersionAtCreationを補完する規則がない

- 場所: `docs/04-agentic-sdlc/fixture-contract.json:3095`, `docs/04-agentic-sdlc/fixture-contract.json:3114`, `docs/02-design/review-resolution-contracts.md:658`, `docs/02-design/review-resolution-contracts.md:668`, `docs/02-design/service-contracts.ts:110`
- 関連: FR-C08, FR-X07 / 旧指摘 G1-027
- 再現条件: AT-C08-SRCのnotif-alert-insulation-a / notif-alert-unknown-aをIR91の規則だけで正規化してNotificationとして検証する。seed通知にも同じ省略がある。
- 問題: 必須numberのscopeVersionAtCreationが行になく、IR91の共通規則やNotification専用規則でも生成されない。null可能な項目や配列の補完対象でもない。
- 影響: IR69/91に従うfixture生成は型検証で失敗する。値を暗黙に補う実装は規範にない推測を必要とする。
- 修正案: 各行に作成時scopeVersionを明示するか、fixture生成時のrecipient Membershipから導出する規則を追加する。seedと全通知patchの必須項目検査を追加する。

## 修正確認と限界

IR94の技術者書込み条件・候補scope、IR96の取消/解除状態表、IR100の部品集合・提出検証、IR99の案内条件、C08集計Query、P03のjobId、AppShell役割切替は修正が確認できた。測定の範囲内への修正、Assignmentと予定枠の同期、旧受入の期日・DST・archive・通信/電源状態の修正も確認した。G1-002/005/006/027には上記の残件があり、31件すべての完全解消とは判定しない。

今回の指摘はアプリの実行結果ではなく、同一baseline内の規範を適用した文書上の再現条件に基づく。通知分類の指摘は担当間でMAJOR/MINORの評価差があったが、既存のIR10写像が明確で修正が局所的なため、主担当はMINORとして集約した。企業承認・本番接続の未確定事項を1Aの欠陥件数に加えていない。

## 引き継ぎ

次の担当は設計修正担当。G120-001〜005を契約・型・fixture・受入条件へ同じ変更単位で反映し、必要な回帰検査を追加する。仕様を変更したら新しいbaselineを作り、両検証器とTypeScript strictを実行したうえで、修正担当以外によるG1再判定を行う。DOC-0.20.0の自己レビュー記録は当時の結果として保持する。READMEなどbaseline対象本文は今回変更していないため、そのpending表記はレビュー実施前のsnapshotであり、現在判定は本記録と親gate-G1.yamlを参照する。

今回作成したレビュー資料はコミット・pushしていない。
