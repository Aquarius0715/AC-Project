---
document_id: DD-A
version: 0.21.0
status: draft
owner: design-agent
consumers: [implementation-agent, test-agent, review-agent]
scope: frontend-demo-1A
---

# 管理者・HQ 詳細設計書

この設計書は、企業の元の要望と対応する要件をもとに作っています。対象は次の3つです。

- どんな機能があるか
- 画面にどんな項目を出すか
- どんな状態・エラーが起こりうるか

各FR(機能要件)を満たすための処理と、テストで確認すべき条件(受入条件)を、この文書で決めます。参考にしているモック画面は、共通UIの見た目を考えるためだけに使います。

**0.21.0の実装基準**: [確定契約](deterministic-contracts.md) 全章およびstrict-review-contracts.md全章、操作カタログの認可列、画面カタログを併読する。数値・権限・非同期・復旧を実装時に推測しない。デモの設計提案であり本番の業務承認ではない。

## 入力・責務

一次資料(いちばんもとになる資料)は[企業要件原文（SRC-06）](../00-prepare/sources/company-requirements-original.txt)です。この原文を整理し直した要件から、画面・入力・状態・受入条件を設計します。

入力として使うもの: [役割別要件](../01-requirements/admin.md)、[共通要件](../01-requirements/common.md)。

必ず読むもの: [共通詳細設計](common.md)、[UIUX仕様書](../03-uiux/UIUXSpecification.md)。

以下に書くのは、フロントエンド(画面側)の項目・表示・モック動作の設計です。画面上でおこなう登録・割当・入金・制限・監査は、すべて「共有モックメモリ」というデモ用の仮データの状態が変わるだけです。サーバーの実装やデータベース設計をお願いするものではありません。

ルートパラメーター(URLの一部として渡される値)は、信頼できない入力として扱い、必ず検証します。表の中の「service名」は、共通Repository(データを取り出す仕組み)の中の処理の名前です。同じルート(画面URL)の行は、同じ画面の中で役割分担している機能です。すべての行で、次の5つの状態を実装します。

- loading(読み込み中)
- empty(データなし)
- error(エラー)
- forbidden(権限なし)
- not-found(見つからない)

再試行のボタンは、回復できるエラーのときだけ用意します。権限が足りないときと対象が見つからないときは、再試行ボタンを出さず、IR57に従って表示します。

## 画面・処理設計

| 設計ID / 要件 | ルート / 主コンポーネント | 取得・操作契約 | 入力・処理・検証 | 異常系と禁止事項 |
|---|---|---|---|---|
| DD-A01 / FR-A01 | `/admin` / `AdminOverview` | `admin.summary` | 組織・期間を指定する。稼働率は「対象台数」と「状態が不明な台数」を両方表示する | 状態が測れていない設備を、自動的に「非稼働」や「正常」に分類してはいけない |
| DD-A02 / FR-A02 | `/admin/units` / `AssetRegistry` | `organizations.list`、`organizations.save`、`customers.list`、`customers.save`、`properties.save`、`spaces.save`、`units.save`、`units.archive`、`units.list`、`units.get`、`properties.list`、`spaces.list`、`capabilities.list`、`units.delete`、`commands.create`、`commands.get`、`diagnosticRuns.list`、`diagnosticRuns.get` | 名称は1〜120文字で必須。親ID、型番(modelId)、形式(split)、設置日を入力する。tenantIdは入力せずSessionから決める(IR74) | 使用中の場所は削除できない。関連する設備を先に移動する必要がある。削除よりアーカイブ(使用停止扱い)を優先する |
| DD-A03 / FR-A03 | `/admin/settings/access` / `AccessManager` | `members.list`、`members.save`、`organizations.list` | identity.manage(権限管理)の権限、membershipId(所属ID)、role(役割)、scope(担当範囲)、有効期間(validFrom/Until)を扱う。HQの権限は機能ごとに分ける | 外部の人に「期限なし」の割当をしてはいけない(デモ用の方針)。自分自身に強い権限を勝手に付与できないようにする |
| DD-A04 / FR-A04 | `/admin/devices` / `DeviceRegistry` | `capabilities.list`、`capabilities.save`、`devices.list`、`devices.get`、`devices.register`、`devices.bind`、`devices.check`、`devices.calibrate`、`devices.updateFirmware`、`units.list`、`units.get`、`devices.calibrations`、`devices.operations` | 温度範囲はmin(最小)<=max(最大)、step(刻み幅)>0にする。許可するmode(運転モード)/fan(風量)を指定し、ventilation(換気機能があるか)も明示する | 今あるコマンドと合わない能力変更をするときは、影響を必ず表示する。対応している機能を勝手に推測してはいけない |
| DD-A05 / FR-A05 | `/admin/alerts` / `AlertPolicyEditor` | `alerts.list`、`policies.save`、`notifications.preview`、`policies.list`、`policies.get`、`alerts.get`、`alerts.acknowledge`、`alerts.resolve`、`notifications.recipients`, units.list, units.get | 単位を揃える。上限・下限、継続時間(0より大きい数)、通知先を必須にする。しきい値の数値はデモ用の仮の値 | 通知を既読にしただけでは、異常(アラート)は解消したことにならない。外部への送信は「プレビュー(確認用の見た目)」だけにする |
| DD-A06 / FR-A06 | `/admin/jobs` / `MaintenanceCoordinator` | `jobs.list`、`jobs.create`、`jobs.offer`、`jobs.assign`、`jobs.review`、`jobs.saveCost`、`jobs.hold`、`jobs.resumeHold`、`jobs.cancel`、`plans.save`、`plans.generateNext`、`jobs.get`、`reports.get`、`attachments.getContent`、`members.eligible`、`organizations.list`、`jobs.extendAccess`、`plans.list`、`plans.get`、`units.list` | 保守の種別・対象・期日を入力する。社内担当か外注かを選ぶ。費用は0以上の金額と通貨をセットで入力する。外注は、業者が依頼を受けてから自社の担当者に割り当てる | 業者が依頼を辞退したときは、別の業者に依頼し直す。「作業完了」と「異常の解消」は別々に判断する。実際の業者への報酬送金はおこなわない |
| DD-A07 / FR-A07 | `/admin/billing/contracts` / `ContractEditor` | `contracts.list`、`contracts.save`、`customers.list`、`units.list` | 契約の種別、customerId(顧客ID)、unitIds(対象設備ID)、期間、料金を入力する。「制限できるかどうか」は契約ごとの属性として持つ | 一般保守の契約にRTO制限(遠隔で止める制限)を誤って適用してはいけない。確定した請求に影響する変更は、新しいバージョンとして作る |
| DD-A08 / FR-A08 | `/admin/billing` / `BillingManager` | `invoices.list`、`invoices.create`、`payments.confirm`、`notifications.preview`、`inquiries.list`、`inquiries.answer`、`payments.recordManual`、`contracts.list`、`invoices.get`、`notifications.recipients`, invoices.remind | billing.manage(請求管理)の権限、契約、金額、期限、入金の参照IDを扱う。手動で入金確認するときは理由を必須にする | 同じ入金の参照番号を二重に計上してはいけない。入金確認は、画面遷移しただけで自動的に完了させてはいけない |
| DD-A09 / FR-A09 | `/admin/restrictions` / `RestrictionManager` | `restrictions.schedule`、`restrictions.execute`、`restrictions.release`、`commands.get`、`restrictions.list`、`restrictions.get`、`restrictions.retry`、`restrictions.reconcile`、`contracts.list`、`invoices.list`、`units.list`、`units.get` | restriction.manage(制限管理)の権限、契約、設備、理由、予告期限、制限の内容を入力する。実行の直前にもう一度確認して、条件が変わっていないかチェックする | 入金済み・猶予中・例外扱い・非対応設備のときは実行を拒否する。適用中に許可される設備操作はIR46の表に従う。失敗したときや期限切れのときは、「まだ反映されていない」状態のままにしておく |
| DD-A10 / FR-A10 | `/admin/restrictions/:id` / `RestrictionException` | `restrictions.defer`、`restrictions.exempt`、`restrictions.cancel`、`restrictions.override`、`audit.list`、`restrictions.get`、`restrictions.retry`、`restrictions.reconcile`, restrictions.list | override(強制的な変更)の権限、理由、期限を扱う。実行の要求中に取消がぶつかったときは状況を確認し、必要なら「解除」の要求に切り替える | 支払い状態を、手動解除に合わせて勝手に書き換えてはいけない。履歴は削除できない |
| DD-A11 / FR-A11 | `/admin/settings/automation` / `ControlPolicy` | `policies.save`、`automations.simulate`、`automations.fire`、`policies.list`、`policies.get`、`units.list`、`units.get` | 対象設備、優先順位、イベント(きっかけ)、動作、止める条件を入力する。契約上の制限や安全に関する能力を優先する | データが取得できていないときは、自動での実行を止める。外部の電力設備に対して、実際の指令は送らない |
| DD-A12 / FR-A12 | `/admin/settings/air-quality` / `AirPolicy` | `policies.save`、`automations.simulate`、`automations.fire`、`telemetry.series`、`policies.list`、`policies.get`、`units.get`、`commands.get`、`notifications.recipients`、`units.list` | ppm(気体濃度の単位)、µg/m³(微粒子濃度の単位)、°C(温度)、%(湿度など)の単位を、それぞれ対応する指標に固定する | 「健康・安全を保証する」という表示はしない。送風の機能だけで、換気の指令を出してはいけない |
| DD-A13 / FR-A13 | `/admin/energy` / `EnergyAnalysis` | `energy.summary`、`baselines.list`、`baselines.save`、`units.list` | 基準期間・基準の範囲・モデルのバージョン、対象設備の集合を入力する。期間が重なっていないか、データが欠けていないかを検証する | 基準値がなければ計算できない。「10〜20%以上削減できる」といった保証はしない |
| DD-A14 / FR-A14 | `/admin/mrv` / `MRVWorkspace` | `mrv.preview`、`mrv.saveDraft`、`mrv.recordReview`、`factors.list`、`factors.save`、`mrv.list`、`mrv.get`、`baselines.list`、`organizations.list`、`units.list`、`mrv.versions` | 対象期間、対象設備、基準のバージョン、係数のバージョン、範囲を必須にする。根拠の一覧を表示する | データが欠けているときは、その旨(推定であること)を注記する。「外部で検証済み」とは表示せず、「デモ確認」という表示にする |
| DD-A15 / FR-A15 | `/admin/offsets` / `OffsetRegistry` | `offsets.preview`、`offsets.simulate`、`offsets.list`、`customers.list`、`units.list` | 希望する量は0より大きい数にする。制度やプロバイダーは「未選定」というラベルを表示し、demoフラグ(デモであることを示す印)を必須にする | 排出量の数値を、そのままクレジットの残高に転記してはいけない。実際の取引や、実際の証明書の発行はしない |
| DD-A16 / FR-A16 | `/admin/audit` / `AuditExplorer` | `audit.list`、`devices.events` | audit.read(監査閲覧)の権限、期間、実行した人、対象、イベントの種類を入力する。機密な値は隠す(マスクする) | 「拒否された操作」と「成功した操作」は別の結果として表示する。画面からの削除・改変はできない。デモなので、記録が改ざんされないことまでは保証しない |

## 実装の共通手順

1. セッション(ログイン状態)と担当範囲(スコープ)を確認し、IDやURLの絞り込み条件をschema(データの形式ルール)で検証する。
2. Query(データ取得の仕組み)を通してモックサービスを呼び出し、その結果を画面用のデータとして受け取る。
3. フォームはReact Hook Formと共通のschemaを使う。能力や期間などの検証も、このschemaで行う。
4. データを変更する直前に、対象のversion(バージョン)・権限・今の状態が合っているか照合する。影響が大きい操作は、対象と理由を確認してから実行する。
5. Repository(データ管理の仕組み)で共有デモ状態を変更し、相関ID付きのイベントを発行する。関係するQueryは無効化して、最新の状態を取り直す。
6. 応答を待っている間も画面表示は続ける。「成功」「拒否」「失敗」をそれぞれ分けて表示する。フォームの送信に失敗したときは、入力した内容を消さずに残す。

## テストへの引き渡し

設計ID「DD-A」の番号ごとに、同じ番号の「AT-A」の受入条件を確認します。あわせて、上の表に書いた異常系と、権限のない人が直接呼び出した場合の挙動も確認します。テストデータと、役割をまたぐシナリオは[検証計画](../04-agentic-sdlc/verification.md)を正式なものとします。この設計書に書いた例(文字数の上限など)を変更する場合は、schema・文書・境界値のテストを同時に更新してください。

## 機能別詳細仕様（0.6.0）

入力フォームの値はRHF(React Hook Form)で保持し、schemaで検証します。読み取り専用の画面には、フォームの検証は不要です。監査・通知・共通エラーの扱いは、入出力契約DDC-03/09に従います。読み取り専用(read-only)の値は、Query(データ取得の仕組み)のひとつの情報源だけから表示します。共通の型・ページング(ページ分け)・時間・エラーの扱いは[実装契約](implementation-contracts.md)を正式なものとし、以下ではそこに加える個別の条件だけを書きます。見た目の値は、[UIUX](../03-uiux/UIUXSpecification.md)のUX-04/08にあるtoken(色や余白などの基本部品)とpattern(画面パターン)に従います。

### DD-A01 詳細

**一次資料との対応**: SRC-06 BIZ-04, BIZ-08 → FR-A01 → DD-A01。出所区分: 企業原文 SRC-06＋設計での補足。この節で具体的にしている設計補足の内容: 全体の集計と、担当者への画面遷移(導線)です。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A01 / 主な表示パターン: **UI-OVERVIEW**。画面のサービス境界は`admin.summary`です。

**初期表示と前提**: HQのMembership(所属情報)に、対象テナントの集計を見る権限があること。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| customerId / propertyId | ID/任意 | 管理しているテナント内のもの | 対象を絞る |
| customerCount | 読取 | Customer.statusとOrganization.statusがともにactiveな件数(IR40) | 顧客数 |
| from / to | 日時/必須 | today/7d/30d/customのプリセット(SR17)から決める。最大366日(IR74) | 期間 |
| counts / rates | 読取 | 分子/分母/不明な台数(unknownCount)/取得時刻(asOf) | 稼働状況 |
| amountsByCurrency | 読取配列 | 通貨ごとに分ける | 未入金額 |
| energySummary | 読取 | 期間の実績と品質。削減系はnull（比較はA13） | 省エネの実績 |
| energyForecast | 読取 | IR78: 対象設備集合と一致するdemo_fixed基準を按分した予想。null表示は「対象設備なし／基準未設定／算定不可」 | 削減量の予想 |

**処理手順**

1. 顧客・拠点・期間を絞り込みます。次に、顧客数・設備・稼働・異常・保守・請求・電力の状況を確認します。KPI(指標)の数値から、同じ条件の一覧画面に進めます。
2. 読み取りや操作にあたって、次の業務ルールを適用します。稼働率は、「今の状態がわかる設備」のうち、電源が入っている(powerOn)割合とします。全設備の数と、状態が不明な設備の数は必ず一緒に表示します。顧客数はIR40の定義（CustomerとOrganizationがともにactiveなCustomerの件数）です。請求の遅延は、未入金の金額をもとに判断します。
3. この画面は閲覧のみです。ダッシュボード画面と、そこから遷移した一覧画面とで、期間・範囲・定義がずれないようにします。
4. 更新する対象のQuery: `admin summary（関連イベントが起きたとき）`。

**境界条件・失敗時**: 状態が不明な設備を、こっそり稼働率の分母に含めてはいけません。違う通貨を合算せず、通貨ごとに分けて表示します。データが0件のときの割合は「計算できない」として扱います。

**検証**: 追跡表のAT-A01の項目(N=正常系/E=異常系/B=境界値・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A02 詳細

**一次資料との対応**: SRC-06 BIZ-07 → FR-A02 → DD-A02。出所区分: 企業原文 SRC-06＋設計での補足。この節で具体的にしている設計補足の内容: 登録・編集・アーカイブ(使用停止扱い)です。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A02 / 主な表示パターン: **UI-LIST / UI-FORM**。画面のサービス境界は`organizations.list, organizations.save, customers.list, customers.save, properties.save, spaces.save, units.save, units.archive, units.list, units.get, properties.list, spaces.list, capabilities.list, units.delete, commands.create, commands.get, diagnosticRuns.list, diagnosticRuns.get`です。

**初期表示と前提**: 管理対象の組織台帳を編集できる権限があること。顧客・物件・設備の関係を正しくたどれること。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| organization.name / customer.name | 文字列/必須 | 1〜120文字 | 顧客台帳 |
| property.kind / name | enum・文字列/必須 | home(住宅)/office(オフィス)、1〜120文字 | 物件 |
| space.parentSpaceId / kind | ID・enum | 同じ物件内、親子関係が循環しないこと | 階層 |
| unit.modelId / spaceId | modelIdはID/必須、spaceIdはID/null | 有効な型番。spaceIdは同じ物件の場所、nullは空間未割当(IR62) | 設備 |
| unit.type / installedAt | enum・日付/type必須・installedAtはnull可 | split(セパレート形式)、未来の設置完了日は不可、nullは「未登録」(IR44) | 形式・設置日 |
| serviceScope | enum配列/必須 | 対象になる点検グループ | 保守の範囲 |
| changeReason | 文字列/移設などのとき必須 | 1〜1000文字 | 変更した理由 |
| property.address / accessInstructions | 文字列/任意 | 0〜500文字 / 0〜1000文字。架空の値のみ使用 | 住所は自社Offerの受諾前もIR25のsiteAddressとして公開。入場案内は受諾後の有効期間内だけ公開 |

**処理手順**

1. 顧客組織を作成します。次に、物件と場所の階層を作ります。split形式の設備の型番と場所を登録します。その後、検索・編集・移設・アーカイブをおこないます。
2. 読み取りや操作にあたって、次の業務ルールを適用します。新しい設備を登録するときは、customerOrgId(顧客組織ID)・spaceId(場所ID)・modelId(型番ID)が矛盾していないことを確認します。契約・案件・IoTと紐づいている設備は、物理的には削除できません。別のテナントへ設備を移すことは、今回の対象(1A)には含みません。
3. 不変のID、バージョン、変更の前後の内容、変更理由を保持します。移設のときは、今どこにあるかの情報を更新し、履歴には元の場所を残します。
4. 更新する対象のQuery: `organizations / customers / properties / spaces / units / audit`。

**境界条件・失敗時**: 他の顧客の部屋を指定すること、階層が循環すること、存在しないmodelIdを指定すること、使用中の設備を削除しようとすることは、すべて拒否します。稼働していない(inactive)顧客に、新しい設備を登録することもできません。

**検証**: 追跡表のAT-A02の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A03 詳細

**一次資料との対応**: SRC-06 BIZ-04 → FR-A03 → DD-A03。出所区分: 設計での補足(企業の目的に沿ったもの)。この節で具体的にしている設計補足の内容: 権限・所属・担当期間の管理です。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A03 / 主な表示パターン: **UI-LIST / UI-FORM**。画面のサービス境界は`members.list, members.save, organizations.list`です。

**初期表示と前提**: identity.manage(権限管理)の権限があること。変更する対象の、今のrole(役割)・scope(担当範囲)・有効期間をすでに取得していること。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| userId / organizationId | ID/必須 | 有効なユーザーとその所属 | 対象を指定 |
| role | enum/必須 | client(顧客)/contractor(業者)/technician(技術者)/admin(管理者) | 役割 |
| employment | enum/技術者のとき必須 | internal(社内)/external(社外) | 勤務区分 |
| permissions | enum配列/必須 | roleごとに許可された範囲の中から選ぶ | 能力(できること) |
| scopes | ScopeRef配列/必須 | SR03のrole別種別。空は業務対象0件 | 対象範囲 |
| validFrom / validUntil | validFromは日時/必須、validUntilは日時/null | 社外の技術者だけvalidUntilが必須(IR74) | 有効期間 |
| reason | 文字列/必須 | 1〜1000文字 | 変更理由 |

**処理手順**

1. ユーザーと所属を選びます。4つの役割のうち、技術者なら社内か社外かも指定します。担当範囲・期間・個別の権限を設定します。変更内容を確認してから保存します。
2. 読み取りや操作にあたって、次の業務ルールを適用します。role(役割)とpermissions(できること)は別々に扱います。管理者だからといって、restriction.manage(制限管理)やoverride(強制変更)の権限を自動で持たせてはいけません。社外の技術者に、終了日のない設備アクセス権を与えてはいけません。自分自身に強い権限を付与できるのは、別のHQ権限管理者の操作に限ります。
3. Membership(所属情報)のバージョンとscopeVersion(担当範囲のバージョン)を更新します。古いセッションで見ていたキャッシュ(一時保存データ)は破棄し、次の変更要求からは新しい権限で判定します。
4. 更新する対象のQuery: `members / session scope / all affected query caches / audit`。

**境界条件・失敗時**: 別テナントの範囲を指定すること、社外なのに終了日がないこと、validFrom(開始日)がvalidUntil(終了日)以降になっていること、自分自身にoverride(強制変更)権限を追加しようとすることは拒否します。権限が失効した後に、古い画面から保存しようとした場合も拒否します。

**検証**: 追跡表のAT-A03の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A04 詳細

**一次資料との対応**: SRC-06 BIZ-06, BIZ-20 → FR-A04 → DD-A04。出所区分: 企業原文 SRC-06＋設計での補足。この節で具体的にしている設計補足の内容: 機種の能力とIoT台帳の編集です。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A04 / 主な表示パターン: **UI-LIST / UI-FORM**。画面のサービス境界は`capabilities.list, capabilities.save, devices.list, devices.get, devices.register, devices.bind, devices.check, devices.calibrate, devices.updateFirmware, units.list, units.get, devices.calibrations, devices.operations`です。

**初期表示と前提**: device.manage(設備管理)の権限があること(IR74)。能力の値はデモ用の台帳として管理します。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| manufacturer / model | 文字列/必須 | それぞれ1〜120文字。組み合わせは重複不可 | 型番 |
| control / ventilation | boolean(はい/いいえ)/必須 | 初期値はfalse(いいえ) | 能力の有無 |
| modeControl / fanControl | boolean/必須 | 初期false。trueならmodes/fanLevels非空、falseなら空(D14) | モード・風量制御の有無 |
| ventilationLevels | Fan配列/ventilation=trueのとき必須 | lowを含む非空(D08)。falseなら空 | 換気段階 |
| min / max / step | number/温度に対応するとき必須 | min<=max、step(刻み幅)>0、矛盾がないこと | 温度(°C) |
| modes / fanLevels | enum配列/対応するとき必須 | 重複なし | 対応できる候補 |
| sensors | 配列/任意 | metric(測る対象)/unit(単位)/staleAfterSeconds(古くなるまでの秒数) | 測定できる能力 |
| firmwareCandidates | 版の配列/任意 | 対応しているデモ版のみ | ファームウェア |
| changeReason | 文字列/更新のとき必須 | 1〜1000文字 | バージョン変更の理由 |

**処理手順**

1. 型番を登録します。温度・モード・風量・センサー・換気・ファームウェア候補を設定します。影響を受ける設備を確認してから、新しい能力バージョンを保存します。
2. 読み取りや操作にあたって、次の業務ルールを適用します。能力が未確認のときはfalse(いいえ)またはunknown(不明)にします。対応するmode(運転モード)を、製品カテゴリだけから推測してはいけません。既存の自動運転(automation)が新しい能力と矛盾する場合は、そのルールを止めて理由を表示します。
3. Capability(能力)のバージョンを更新し、設備の操作候補に反映します。まだ完了していないCommand(命令)の内容は履歴として残し、勝手に新しい能力の内容へ書き換えません。
4. 更新する対象のQuery: `capabilities / units / devices / automations / audit`。

**境界条件・失敗時**: min(最小)がmax(最大)より大きいこと、step(刻み幅)が0以下であること、mode(運転モード)の候補が空なのにmode制御をtrue(有効)にすることは拒否します。対応していないファームウェア版を、候補に紛れ込ませてはいけません。

**検証**: 追跡表のAT-A04の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A05 詳細

**窓開放・断熱不足の負荷通知の表示・処理設計（BIZ-17）**

`alerts.list`が返すAlert(異常通知)に、次の3つの項目を追加します。

- causeCode(原因コード): window_open(窓が開いている)/insulation_loss(断熱不足)/unknown(不明)
- evidenceKind(根拠の種類): demo_observation(デモの観測)/inferred(推定)/inspection(点検結果)
- evidenceText(根拠の説明文)、observedAt(観測した時刻)

causeCodeとevidenceKindは必須です。根拠が取得できていないときはunknown(不明)とし、「電気代が倍になる」といった具体的な数値は固定表示しません。推定した内容は「疑い」、点検で確認した内容は「点検記録」と表示します。通知の詳細画面から、同じunitId(設備ID)の設備や保守依頼の画面へ移動できます。

検証: AT-A05-SRC。窓開放の疑い・断熱不足の点検記録・根拠なし、という3種類のテストデータ(fixture)で確認します。文言・根拠・時刻がそれぞれ違うこと、既読にしても異常が解消しないことを確認します。

**一次資料との対応**: SRC-06 BIZ-08, BIZ-11, BIZ-17 → FR-A05 → DD-A05。出所区分: 企業原文 SRC-06＋設計での補足。この節で具体的にしている設計補足の内容: しきい値の設定と異常時の処理です。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A05 / 主な表示パターン: **UI-LIST / UI-FORM**。画面のサービス境界は`alerts.list, policies.save, notifications.preview, policies.list, policies.get, alerts.get, alerts.acknowledge, alerts.resolve, notifications.recipients, units.list, units.get`です。

**初期表示と前提**: alert.policy.manage(異常通知の設定管理)の権限、通知先を見る権限があること。指標の単位・対象設備をすでに取得していること。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| unitIds / metric | ID配列・enum/必須 | 対応している能力の範囲内 | 対象を指定 |
| operator / threshold | enum・number/必須 | gt/gte/lt/lte(それぞれ「より大きい」などの比較)、有限の数値 | 発火する条件 |
| durationSeconds | 整数/必須 | 1〜86400 | 続く時間(秒) |
| recoveryThreshold | number/必須 | 方向に応じたヒステリシス(戻すときのゆとり幅) | 解除の候補値 |
| severity | enum/必須 | warning(警告)/critical(重大) | 重要度 |
| recipientMembershipIds / channels | 配列/必須 | それぞれ1件以上。inApp(アプリ内)/email(メール)/whatsapp | 通知先・手段 |
| escalateAfterMinutes / cooldownMinutes | 整数/必須 | 1〜1440 / 1〜1440 | 未対応時のエスカレーション・重複を防ぐ時間 |
| name / unitIds | 必須 | trim後1〜120文字 / scope内・重複なし非空 | IR07共通入力 |
| timezone / enabled / priority | 必須 | IANA名 / boolean / 整数0〜100。新規UIはPreferences.timezone / false / 50を表示 | IR07共通入力 |

**処理手順**

1. 対象設備・指標・比較条件・継続時間を設定します。通知先・手段・エスカレーションまでの時間を指定します。保存後、条件を組み合わせて発火・解除の条件を確認します。
2. 読み取りや操作にあたって、次の業務ルールを適用します。単位はmetric(指標)ごとに決まっていて、変更できません。missing(欠測)やstale(古いデータ)は、しきい値の正常判定には使わず、通信・データ品質の通知として扱います。同じ通知の繰り返しはcooldown(冷却時間)で抑えます。重要度が変わったときは、新しい通知理由として記録します。
3. Policy(方針)のバージョンを保存します。発火したときはAlert(異常通知)とNotification(通知)のプレビューを作成します。通知を既読にすることと、Alertを確認済みにすることは別々に扱います。
4. 更新する対象のQuery: `policies / alerts / notifications / admin summary / audit`。

**境界条件・失敗時**: 通知先が0件のとき、継続時間が0のとき、解除のしきい値と比較の方向が矛盾しているときは拒否します。しきい値に届く直前・ちょうど届いたとき・継続時間の境界を確認します。

**検証**: 追跡表のAT-A05の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A06 詳細

**一次資料との対応**: SRC-06 BIZ-12 → FR-A06 → DD-A06。出所区分: 企業原文 SRC-06＋設計での補足。この節で具体的にしている設計補足の内容: 受付・委託・品質確認です。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A06 / 主な表示パターン: **UI-LIST / UI-DETAIL / UI-FORM**。画面のサービス境界は`jobs.list, jobs.create, jobs.offer, jobs.assign, jobs.review, jobs.saveCost, jobs.hold, jobs.resumeHold, jobs.cancel, plans.save, plans.generateNext, jobs.get, reports.get, attachments.getContent, members.eligible, organizations.list, jobs.extendAccess, plans.list, plans.get, units.list`です。

**初期表示と前提**: job.manage(保守依頼の管理)の権限があること。対象設備、社内・外注の選択肢をすでに取得していること。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| unitId / type | 必須 | 対象設備（split形式）、periodic(定期)/reactive(都度)/preventive(予防) | 依頼内容 |
| dueAt | 日時/任意 | requestedEnd以上。HQだけが指定でき、省略時はrequestedEnd(IR38) | 期限 |
| deliveryMode | enum/必須 | internal(社内)/contractor(外注) | 実施区分 |
| assigneeId / contractorOrgId | ID/条件により必須 | 区分に合ったものを指定 | 委託先・担当者 |
| recurrence | 構造体/定期のとき任意 | monthly(毎月)、間隔1〜12か月、次回日、明示操作ごとに次の1回だけ生成し次回日を更新（D16） | 定期計画 |
| costLines | 配列/任意 | kind=estimate(見積)/actual(実績)、金額(amountMinor)>=0、通貨、説明 | 費用 |
| reviewDecision / reason | 条件により必須 | accept(承認)/return(差し戻し)、中断・取消のときも理由が必須。取消できる状態はIR56 | 品質確認・例外の理由 |

**処理手順**

1. 保守の種別・設備・期限を登録します。社内に直接割り当てるか、業者に委託します。日程と進み具合を確認します。品質確認をおこない、実際の費用を記録します。
2. 読み取りや操作にあたって、次の業務ルールを適用します。社内の場合はHQ(本部)が担当と日程を確定します。外注の場合は、業者が依頼を受けてから自社の担当者に割り当てます。定期計画の繰り返し設定は、次回の生成予定を表示し、同じ計画・同じ回については依頼は1件だけにします。
3. Job(依頼)・Assignment(割当)・Offer(委託の申し出)・費用明細・品質確認の履歴は、すべて同じjobId(依頼ID)にまとめます。作業が完了しても、Alert(異常通知)の解消とは別に判断します。
4. 更新する対象のQuery: `jobs / plans / offers / assignments / costs / notifications / audit`。

**境界条件・失敗時**: 辞退したあとの再委託、確定した予定の重複、期限超過、品質不合格による差し戻しを確認します。見積の通貨と実績の通貨が違う場合は、換算せずに別々に集計します。

**検証**: 追跡表のAT-A06の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A07 詳細

**一次資料との対応**: SRC-06 BIZ-21 → FR-A07 → DD-A07。出所区分: 企業原文 SRC-06＋設計での補足。この節で具体的にしている設計補足の内容: プランと契約の編集です。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A07 / 主な表示パターン: **UI-LIST / UI-FORM**。画面のサービス境界は`contracts.list, contracts.save, customers.list, units.list`です。

**初期表示と前提**: contract.manage(契約管理)の権限があること。顧客と紐づく設備が同じテナント内にあること。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| customerId / unitIds | 必須 | その顧客の有効な設備 | 契約の対象 |
| planType | enum/必須 | rto(Rent to Own契約)/general(一般)/energy(省エネ)/environment(環境) | プランの種類 |
| startAt / endAt | 日時/必須 | 開始日は終了日より前 | 期間 |
| priceMinor / currency | 整数・enum/必須 | 0以上。デモではMYR(通貨)を初期値にする | 料金 |
| restrictionEligible | boolean(はい/いいえ)/必須 | 初期値はfalse。rto(Rent to Own契約)のときだけtrueにできる | 制限できるかどうか |
| rulesVersion | ID/制限できる場合は必須 | 承認済みの本番ルールではなく、デモ版 | 適用する条件 |

**処理手順**

1. プランの種類・期間・料金・対象設備を設定します。RTO制限ができるかどうかとルールのバージョンを指定します。内容を確認してから、契約を保存または改版します。
2. 読み取りや操作にあたって、次の業務ルールを適用します。一般保守の契約はrestrictionEligible(制限できるか)をfalse(いいえ)にします。RTOであっても、はっきり「制限できる」と決めた契約だけが対象です。すでに発行した請求は、契約を改版しても遡ってさかのぼって変更しません。
3. 契約のバージョンを保持します。新しい請求には新しいバージョンを、過去の請求には元のバージョンを参照させます。契約の終了は、実機の停止とは扱いません。
4. 更新する対象のQuery: `contracts / customer payments / audit`。

**境界条件・失敗時**: 他の顧客の設備を指定すること、期間が逆転していること、料金がマイナスであること、一般保守の契約で制限を有効にすることは拒否します。契約のない設備の監視・保守を妨げてはいけません。

**検証**: 追跡表のAT-A07の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A08 詳細

**カード種別と支払い案内の確認（BIZ-22）**

クライアントがDD-C11で選んだ、模擬の支払い方法を、HQは`invoices.list`が返す請求の表示データで確認します。paymentMethod(支払い方法)は次のどれかです。

- demo_credit_card(デモのクレジットカード)
- demo_debit_card(デモのデビットカード)
- null(未選択または手動入金)

paymentStatus(支払い状況)は、最新の模擬Payment(支払い記録)の状態を表示します。未操作または手動入金のときはmethod=nullです。案内表示だけでは既存のInvoice.paymentMethodを変更しません。処理中や失敗したときでも、すでに選ばれている種別は残します。手動で入金確認だけをした場合は、methodを勝手に補って書き換えず、paymentReference(入金参照番号)と確認した理由を表示します。

`notifications.preview`で、請求・案内のチャネル(手段)・すでに選ばれている支払い方法を確認します。HQは、次の操作をおこないます。

- 既存のPayment(支払い記録)の入金確認: `payments.confirm`
- Paymentがない請求の手動入金確認: `payments.recordManual`

カードを選ぶフォームや、`payments.simulate`のような顧客側の決済操作は、HQの画面には置きません。案内を見るだけ・プレビューを見るだけでは、支払い済み(paid)の状態にはなりません。

検証: AT-A08-SRC。クライアント側で各支払い方法の「処理中」「成功」「失敗」を再現したあと、HQ側の同じ請求で方法と状況が一致するか確認します。未操作なら未選択、失敗したときは選んだ種別を保持していること、案内のプレビューだけでは未入金のままであることを確認します。

**一次資料との対応**: SRC-06 BIZ-21, BIZ-22 → FR-A08 → DD-A08。出所区分: 企業原文 SRC-06＋設計での補足。この節で具体的にしている設計補足の内容: 請求と模擬入金確認です。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A08 / 主な表示パターン: **UI-LIST / UI-FORM**。画面のサービス境界は`invoices.list, invoices.create, payments.confirm, notifications.preview, inquiries.list, inquiries.answer, payments.recordManual, contracts.list, invoices.get, notifications.recipients, invoices.remind`です。

**初期表示と前提**: billing.manage(請求管理)の権限があること。契約・請求・模擬決済の対象を照合できること。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| contractId / period | 必須 | 契約期間内の請求期間 | 請求内容 |
| amountMinor / currency | 必須 | 正の整数。契約の通貨と一致すること | 金額 |
| dueAt | 日時/必須 | nowより後だけ。期限超過の請求はdemoSeedまたはdemo.advanceClockで作る(IR90) | 期限 |
| paymentReference | 文字列/確認時必須 | 1〜128文字。テナント内で重複しないこと | 入金の参照番号 |
| confirmedAmountMinor | 整数/確認時必須 | 請求の全額と一致すること | 確認した金額 |
| reason | 文字列/手動確認時必須 | 1〜1000文字 | 確認の根拠 |
| channel | enum/督促のとき必須 | email/whatsapp/inApp。preview後の明示操作で模擬記録、外部送信なし | 案内の手段 |
| inquiryId / reply | ID・文字列/問い合わせ回答時必須 | 管理できる範囲内。返信は1〜2000文字 | 顧客へのアプリ内での回答 |

**処理手順**

1. 契約のバージョンから請求を作成します。期限や状態で絞り込みます。模擬決済の結果を確認するか、権限のある人が入金を確認します。督促をpreviewし、明示確認でinvoices.remindを実行して、顧客に見える保存済み通知を確認します。
2. 読み取りや操作にあたって、次の業務ルールを適用します。今回の対象(1A)は全額入金のみを扱います。invoiceId(請求ID)とpaymentReference(入金参照番号)の組み合わせで二重に確認しても、同じ結果を返します。督促の対象は、未入金かつ期限超過のものです。例外や係争があるかどうかも表示します。
3. Payment(支払い)がconfirmed(確認済み)、Invoice(請求)がpaid(支払い済み)になったことと、監査の記録を残します。関連するRestriction(制限)のcauseInvoiceIds(原因になった請求)がすべてpaid(支払い済み)になったら、scheduled(予定)状態はcancelled(取消)に、requested/applied(要求中/適用済み)の状態はrelease_requested(解除要求)に進めます。未入金が1件でも残っていれば解除せず、機器からの応答待ちと残りの件数を表示します。
4. 更新する対象のQuery: `invoices / payments / restrictions / notifications / audit`。

**境界条件・失敗時**: 金額の違い、通貨の違い、同じ参照番号を別の請求に流用すること、支払い済み(paid)の請求を再請求することは拒否します。入金済みになった直後に督促を実行しようとしたときは、もう一度確認してから止めます。

**検証**: 追跡表のAT-A08の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A09 詳細

**一次資料との対応**: SRC-06 BIZ-21 → FR-A09 → DD-A09。出所区分: 企業原文 SRC-06＋設計での補足。この節で具体的にしている設計補足の内容: 予告・実行確認・機器からの応答です。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A09 / 主な表示パターン: **UI-LIST / UI-DETAIL / UI-FORM**。画面のサービス境界は`restrictions.schedule, restrictions.execute, restrictions.release, commands.get, restrictions.list, restrictions.get, restrictions.retry, restrictions.reconcile, contracts.list, invoices.list, units.list, units.get`です。

**初期表示と前提**: restriction.manage(制限管理)の権限、制限できるRTO契約、未入金の状態、対象設備の能力の確認がそろっていること。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| contractId / causeInvoiceIds / unitIds | 必須 | 同じ契約の、期限を過ぎた未入金請求すべてを原因として固定する。設備ごとに進行中の制限は1件まで | 対象 |
| policy.kind | enum/必須 | temperature_limit(温度制限)/power_off(電源オフ) | 制限の方式 |
| policy.minimumCoolingSetpoint | number/温度制限のとき必須 | 機器のmin/max/stepの範囲内。これより低い温度設定は禁止 | 冷房の制限内容 |
| executeAfter | 日時/必須 | 受付now+24時間以降。noticeAtはRepositoryがnowで採番し読取表示のみ（IR05） | 予告の日程 |
| reason / rulesVersion | 文字列・ID/必須 | 1〜1000文字、デモ版。reasonは顧客の制限説明画面にそのまま表示される(IR42) | 根拠 |
| expectedVersion | 整数/実行時必須 | 今のバージョン | 競合の確認用 |

**処理手順**

1. 予告の理由・対象・内容・executeAfterを入力します。保存前はフォームの確認表示、schedule成功後は採番されたnoticeAtと保存済み予告通知を確認します。開始のタイミングで、請求・猶予・例外をもう一度照合します。設備ごとに適用の要求を出します。入金後は、解除の要求とその応答を追います。
2. 読み取りや操作にあたって、次の業務ルールを適用します。画面上で期限が来ても、自動で実際に止めることはしません。今回の対象(1A)は、HQがはっきり確認した操作だけを模擬で実行します。適用は全台の成功証跡、解除は全台のreleased/not_required証跡を必要とします（D03）。電源オフと温度制限は別々のpolicy(方針)として扱います。
3. Restriction(制限)と、設備ごとのCommand(命令)を作成します。原因になった請求(causeInvoiceIds)がすべて入金確認できたら、scheduled(予定)ならcancelled(取消)に、requested/applied(要求中/適用済み)ならrelease_requested(解除要求)にします。1件でも未入金があれば、状態はそのまま維持します。
4. 更新する対象のQuery: `restrictions / commands / units / customer billing / notifications / audit`。

**境界条件・失敗時**: 実行の直前に、原因になった請求がすべて入金済みになっていれば、cancelled(取消)とし、適用の要求は作りません。猶予中・例外中、まだ通知していない場合、非対応の機器の場合も適用できません。一部の設備がオフラインのときは、全体をapplied(適用済み)にせず、設備ごとの保留状況を表示します。解除に失敗したときはrelease_requested(解除要求)のまま保持し、遅れて届いた適用の応答でapplied(適用済み)に戻してはいけません。

**検証**: 追跡表のAT-A09の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A10 詳細

**一次資料との対応**: SRC-06 BIZ-21 → FR-A10 → DD-A10。出所区分: 設計での補足(企業の目的に沿ったもの)。この節で具体的にしている設計補足の内容: 猶予・例外・監査の手順です。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A10 / 主な表示パターン: **UI-DETAIL / UI-FORM / UI-TIMELINE**。画面のサービス境界は`restrictions.defer, restrictions.exempt, restrictions.cancel, restrictions.override, audit.list, restrictions.get, restrictions.retry, restrictions.reconcile, restrictions.list`です。

**初期表示と前提**: 猶予や例外の操作にはrestriction.manage(制限管理)の権限、手動解除にはrestriction.override(強制解除)の権限が必要です。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| restrictionId | ID/必須 | 管理できる範囲内 | 対象 |
| action | enum/必須 | defer(猶予)/exempt(例外)/cancel(取消)/override_release(強制解除)。対応する操作は次の通りです。defer→`restrictions.defer`(until(期限)が必須)、exempt→`restrictions.exempt`(until(期限)が必須)、cancel→`restrictions.cancel`、override_release→`restrictions.override`(restriction.override(強制解除)の権限が必須)。理由(reason)はどの操作でも必須です | 例外の操作 |
| until | ISO日時/猶予・例外のとき必須 | 今より未来の日時 | 有効期限 |
| reason | 文字列/必須 | 1〜1000文字(IR87) | 例外の根拠 |
| expectedVersion | 整数/必須 | もう一度取得したあとのバージョン | 競合の確認用 |

**処理手順**

1. 対象の今の状態と、機器への反映状況を照会します。猶予・例外・取消・手動解除のどれかを選びます。理由・期限・影響を確認してから保存します。設備ごとの結果を追います。
2. 読み取りや操作にあたって、次の業務ルールを適用します。取消の結果はIR96の状態表に従います（scheduledはcancelled、requested/appliedはreleaseIntent.source=cancelでrelease_requested、release_requestedは冪等、released/cancelledはCONFLICT）。例外や猶予の期限が切れても、自動で再適用はせず、もう一度条件を確認する必要があります。
3. 例外・猶予の変更前後の内容と期限、解除の理由・実行した人を記録します。手動解除をしても、Invoice(請求)の未入金は解消されません。
4. 更新する対象のQuery: `restrictions / commands / audit / notifications`。

**境界条件・失敗時**: override(強制解除)の権限がないHQのユーザー、理由が空、過去の日付を猶予期限にすることは拒否します。requested(要求中)の状態を取り消したときも、実際に適用されている可能性がゼロだとは決めつけません。

**検証**: 追跡表のAT-A10の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A11 詳細

**一次資料との対応**: SRC-06 BIZ-14, BIZ-16, BIZ-17 → FR-A11 → DD-A11。出所区分: 企業原文 SRC-06＋設計での補足。この節で具体的にしている設計補足の内容: 条件の設定とシミュレーションです。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A11 / 主な表示パターン: **UI-FORM**。画面のサービス境界は`policies.save, automations.simulate, automations.fire, policies.list, policies.get, units.list, units.get`です。

**初期表示と前提**: automation.policy.manage(自動運転の設定管理)の権限があること。対象設備と制御できる能力をすでに取得していること。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| name / unitIds | 必須 | trim後1〜120文字、scope内・重複なし非空 | 方針の名前・対象設備 |
| condition.type | enum/必須 | occupancy(在室)/tariff(料金)/peak(ピーク)/solar(太陽光)/battery(蓄電池) | 条件の種類 |
| condition（Condition型） | 判別union(条件により形が変わる値)/必須 | 料金のしきい値・時間帯・出力など、単位つき | 判定する値 |
| action | UnitAction(設備への動作)/必須 | 能力・制限の範囲内 | 動作の内容 |
| timezone / enabled / priority | 必須 | IANA名 / boolean / 整数0〜100（大きい値ほど優先）。新規UIはPreferences.timezone / false / 50を表示 | IR07共通入力 |

**処理手順**

1. 在室・料金・ピーク・太陽光や蓄電池の条件を選びます。動作と優先順位を設定します。競合がないかプレビューで確認します。組み合わせたイベントで評価します。
2. 読み取りや操作にあたって、次の業務ルールを適用します。優先順位は「能力・有効な制限」→「HQの方針」→「顧客のルール」の順です。同じ層の中では、priority(優先度)の数値が大きいほうを優先し、同じ値ならID順(小さい方から)にします。データが取得できていないときや、期限切れのときは実行を見送り、理由を表示します。
3. 方針のバージョンを保存します。`automations.simulate`は、採用または抑止するルールとその理由だけを返し、Command(命令)は作りません。実際の発火は`automations.fire`(DemoWriteOptionsという重複防止の情報つき)でおこない、共通のCommand policy(命令のルール)を経て、commandIds(命令のID)を返します(DDC-08§6を参照)。
4. 更新する対象のQuery: `policies / automations / simulation results / audit`。

**境界条件・失敗時**: 顧客のルールよりHQの方針を優先すること、同じ優先順位のときは毎回同じ結果になること、太陽光のデータが取得できていないときは勝手に実行しないことを確認します。

**検証**: 追跡表のAT-A11の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A12 詳細

**アレルゲンを含む空気環境の表示・処理設計（BIZ-18）**

`telemetry.series`が返す空気環境の表示データに、allergenObservation(アレルゲンの観測結果)を追加します。availability(取得できているか)は次のどれかです。

- available(取得できている)
- not_measured(測定していない)
- unsupported(対応していない)

substance(物質名)・value(数値)・unit(単位)・observedAt(観測時刻)・sourceLabel(出典の表示)は、取得できたときだけ表示します。available(取得できている)のときは、根拠と時刻が必須です。数値があるときは単位も必須です。情報が不完全なときは「不明」として扱います。PM2.5の値からアレルゲンの量を計算してはいけません。CO₂はppm(単位)で表示し、電力の排出量は別画面でkgCO₂e(単位)として表示します。換気の能力がない設備では、案内だけを表示します。

検証: AT-A12-SRC。「未計測」「非対応」「合成した観測データ」というテストデータ(fixture)で表示を切り替え、未計測のときに0や「安全」と表示しないことを確認します。数値があるのに単位がないときは「不明」になることも確認します。

**一次資料との対応**: SRC-06 BIZ-18, BIZ-19 → FR-A12 → DD-A12。出所区分: 企業原文 SRC-06＋設計での補足。この節で具体的にしている設計補足の内容: 換気ルールと、データが欠けているときの扱いです。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A12 / 主な表示パターン: **UI-FORM / UI-ANALYSIS**。画面のサービス境界は`policies.save, automations.simulate, automations.fire, telemetry.series, policies.list, policies.get, units.get, commands.get, notifications.recipients, units.list`です。

**初期表示と前提**: automation.policy.manageの権限があること(IR74)。機器に、対象のmetric(指標)と換気能力の定義があること。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| unitIds / metric | 必須 | 各UnitのCapability.sensorsと現bindingのDevice.sensorsの両方にmetricがあること（IR21/IR43） | 対象 |
| threshold / recoveryThreshold | number/必須 | 指標ごとに単位が固定 | 発火・回復の条件 |
| durationSeconds | 整数/必須 | 1〜86400 | 続く時間(秒) |
| responseMode | enum/必須 | notify_only(通知のみ)/notify_and_ventilate(通知と換気) | 対応方法 |
| severity / channels | 必須 | warning/critical、channel非空。新規は未選択 | 通知設定SR28 |
| cooldownMinutes / escalateAfterMinutes | 整数/必須 | 1〜1440。新規は未入力 | 再通知/未確認時の通知SR28 |
| recipientMembershipIds | ID配列/必須 | 有効な宛先が1件以上 | 通知先 |
| name / unitIds | 必須 | trim後1〜120文字 / scope内・重複なし非空 | IR07共通入力 |
| timezone / enabled / priority | 必須 | IANA名 / boolean / 整数0〜100。新規UIはPreferences.timezone / false / 50を表示 | IR07共通入力 |

**処理手順**

1. 指標・しきい値・継続時間や回復条件を設定します。通知だけにするか、換気の要求も出すかを選びます。対象の能力を確認してから、模擬で評価します。
2. 読み取りや操作にあたって、次の業務ルールを適用します。換気の自動要求は、ventilation(換気機能)がtrue(あり)の対象だけに出します。それ以外は通知のみとし、保存する前に対象ごとの実行内容を表示します。
3. 環境に関するpolicy(方針)のバージョンと通知のプレビューを保存します。換気の要求は、`automations.fire`がventilation=true(換気機能あり)の対象だけにventilate(換気の)Commandを作り(DDC-08§6を参照)、機器からの応答を追います。室内環境が改善したかどうかは、あとの測定で確認します。
4. 更新する対象のQuery: `policies / alerts / commands / notifications / audit`。

**境界条件・失敗時**: ppmとµg/m³のしきい値を混ぜて使ってはいけません。未計測のデータを、正常・回復の判定には使いません。換気に対応していない機器に、送風のコマンドで代用してはいけません。

**検証**: 追跡表のAT-A12の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A13 詳細

**一次資料との対応**: SRC-06 BIZ-23, BIZ-25 → FR-A13 → DD-A13。出所区分: 企業原文 SRC-06＋設計での補足。この節で具体的にしている設計補足の内容: 基準バージョンと算定条件の管理です。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A13 / 主な表示パターン: **UI-ANALYSIS / UI-FORM**。画面のサービス境界は`energy.summary, baselines.list, baselines.save, units.list`です。

**初期表示と前提**: energy.manage(省エネ管理)の権限、管理できる範囲の期間データがあること。基準モデルの根拠を入力できること。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| unitIds / period | 必須 | 同じ比較対象の集合、最大366日 | 対象 |
| method | enum/必須 | demo_fixed(デモの固定値)/demo_period_comparison(デモの期間比較) | 基準の方法 |
| baselineKWh | number/demo_fixedのとき必須 | 0以上。仮のデータであることを表示。demo_period_comparisonでは入力しない(SR29) | 基準値 |
| boundaryId | enum/必須 | ac_input_electricity/whole_building_electricity(IR11) | 算定境界ID |
| boundary | 文字列/必須 | 1〜500文字(IR11) | 算定の範囲の説明 |
| assumptions | 文字列/必須 | 1〜2000文字 | 前提条件 |
| source | 文字列/必須 | デモの出典。versionは入力せずRepositoryが採番 | 根拠 |

**処理手順**

1. 基準にする対象・期間・方法を指定します。比較の条件を確認します。実績との差を表示します。品質と根拠を詳しく表示します。
2. 読み取りや操作にあたって、次の業務ルールを適用します。基準は、設備の集合・範囲・期間の条件・モデルのバージョンとセットで保持します。気象などの補正モデルがまだできていないときは、「補正済み」とは表示しません。マイナスの削減量(=増加)を0に丸めてはいけません。
3. 基準を保存するときは、新しいバージョンを作ります。すでにあるMRVレポートの基準バージョンは、あとから変更しません。
4. 更新する対象のQuery: `baselines / energy / audit`。

**境界条件・失敗時**: 基準値が0のとき、実績が欠けているとき、設備の集合が違うとき、算定範囲が違うときの動きを確認します。例: 基準100・実績80なら「削減 20.0 kWh・削減 20.0%」、基準100・実績120ならDTO値は-20kWh・-20%で、表示は「増加 20.0 kWh」「増加 20.0%」です(IR68)。

**検証**: 追跡表のAT-A13の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A14 詳細

**Scope 2報告プレビューの表示・処理設計（BIZ-25）**

レポート画面のデータに、次の項目を追加します。

- reportCategory(Scope 2の電力デモ集計であることの表示)、organizationId、period(期間)、siteIds(対象拠点)
- gridRegion(電力系統の地域)、factorValue(係数の値)、factorUnit(係数の単位)、factorYear(係数の年度)、factorVersion(係数のバージョン)
- boundaryDescription(算定範囲の説明)、coverageRatio(カバー率)

これらは表示ラベルであり、DTOにフィールドを追加しない。各項目の取得元はIR88の対応表に従う。

係数・地域・対象範囲のどれかが足りない場合は「算定未完了」として扱い、0で埋めてはいけません。省エネの量と、電力にともなう排出量は、別々の欄に分けます。画面と保存済み版のプレビューには、「デモ・未検証」という表示を残します。

検証: AT-A14-SRC。同じ使用量でも係数のバージョンを変えると、換算結果とバージョンが変わることを確認します。係数が欠けているときは「算定未完了」と表示し、期間外・対象外の拠点を混ぜないことを確認します。

**一次資料との対応**: SRC-06 BIZ-25 → FR-A14 → DD-A14。出所区分: 企業原文 SRC-06＋設計での補足。この節で具体的にしている設計補足の内容: レポートの項目・証跡・プレビューです。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A14 / 主な表示パターン: **UI-ANALYSIS / UI-FORM / UI-DETAIL**。画面のサービス境界は`mrv.preview, mrv.saveDraft, mrv.recordReview, factors.list, factors.save, mrv.list, mrv.get, baselines.list, organizations.list, units.list, mrv.versions`です。

**初期表示と前提**: mrv.manage(MRV管理)の権限があること。対象期間、設備、基準バージョン、係数バージョン、範囲がすでに選択されていること。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| unitIds / from / to | 必須 | 管理できる範囲内。開始日は終了日より前 | 対象 |
| baselineId / factorId / boundary | 必須 | バージョンつき、変更されない参照 | 算定条件 |
| factor.region / year / kgCO2ePerKWh / source | 必須 | 年度は整数、係数は0以上、出典にデモであることを明示 | 係数 |
| reviewComment | 文字列/確認時必須 | 1〜1000文字(IR87) | デモ確認のコメント |
| reportVersion | 整数/更新時必須 | 今のバージョン | 競合の確認用 |

**処理手順**

1. 算定条件を指定します。測定・品質・計算結果をプレビューします。根拠を確認します。ドラフト(下書き)として保存します。デモ確認の履歴と報告のプレビューを表示します。
2. 読み取りや操作にあたって、次の業務ルールを適用します。係数の地域・年度・出典は必須です（単位はkgCO₂e/kWh固定、IR102）。確認した履歴はdemo_reviewed(デモ確認済み)として扱い、外部の認証とは区別します。入力のバージョンが変わったら、新しい報告バージョンを作り、古い結果はそのまま残します。
3. MRVReport(MRVの報告)に、係数・基準の当時のスナップショット参照、算定結果・品質・reviewHistory(確認履歴)を保存します。プレビューを見ただけでは、確定した記録は作りません。
4. 更新する対象のQuery: `mrv / review history / audit`。

**境界条件・失敗時**: 係数が欠けているとき、coverage(カバー率)が0のとき、同じ報告バージョンに重複して確認したときの動きを確認します。未検証の値が「認証済み」に見えないようにします。

**検証**: 追跡表のAT-A14の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A15 詳細

**炭素市場への将来連携の表示・処理設計（BIZ-26）**

`offsets.preview`が返す表示結果に、marketConcept(市場構想)を追加します。次の項目を必須にします。

- stage=future_concept(将来の構想段階であること)
- providerLabel=未選定
- verificationStatus=unverified(未検証)
- ledgerStatus=not_connected(台帳と未接続)

省エネの量、推定した削減排出量、模擬の購入・償却の記録は、それぞれ分けて表示します。市場価格・実際のトークン残高・売買を実行するボタンは設けません。将来の連携先と検証条件がまだ決まっていないことを説明します。UIはRepository(データを取り出す仕組み)から返る非同期の結果を表示するだけで、将来データを取得する仕組み(adapter)をあとから追加できる作りにします。

検証: AT-A15-SRC。オフセット(排出量の相殺)を選ばなければ、申込は作られないことを確認します。市場構想の画面を開いても、残高・証明書・取引結果は生成されず、模擬の償却とは違う状態が表示されることを確認します。

**一次資料との対応**: SRC-06 BIZ-24, BIZ-26 → FR-A15 → DD-A15。出所区分: 企業原文 SRC-06＋設計での補足。この節で具体的にしている設計補足の内容: 模擬償却と市場構想のプレビューです。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A15 / 主な表示パターン: **UI-LIST / UI-FORM / UI-DETAIL**。画面のサービス境界は`offsets.preview, offsets.simulate, offsets.list, customers.list, units.list`です。

**初期表示と前提**: offset.manage(オフセット管理)の権限があること。模擬の取引であることを画面に表示すること。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| customerId / purpose | 必須 | 管理できる範囲内、1〜1000文字 | 申込者・目的 |
| amountKg | number/必須 | 0より大きく、100000以下。小数点以下3桁まで | 希望する量 |
| quoteId / version | 見積後は必須 | 有効なデモの見積 | 確認する対象 |
| provider / scheme | 読取 | unselected(未選定)/demo | 未選定の表示 |
| demoConfirmed | boolean(はい/いいえ)/必須 | 初期値はfalse(いいえ) | 実際の取引ではないことの確認 |
| demoCertificateRef | 読取 | DEMO-という接頭辞。償却後にのみ表示 | 模擬の証明書 |

**処理手順**

1. 希望する量と目的を指定します。模擬の見積を出します。模擬の申込をします。デモの購入確認をします。デモの償却をします。証明情報のプレビューを見ます。
2. 読み取りや操作にあたって、次の業務ルールを適用します。「購入の申込」「購入の確認」「償却」は、それぞれ別々のイベントとして扱います。今回の対象(1A)は、1つの記録を全量まとめて償却する場合だけを扱います。自社の排出削減の計算値を、購入済みの残高に足してはいけません。
3. quoted(見積済み)→demo_requested(申込済み)→demo_purchased(購入済み)→demo_retired(償却済み)、という流れを履歴として保存します。証明の参照番号はDEMO-という接頭辞をつけ、実際の証明書としては出力しません。
4. 更新する対象のQuery: `offsets / offset events / audit`。

**境界条件・失敗時**: 購入していないのに償却しようとすること、二重に償却しようとすること、希望する量が0であること、見積の期限が切れていること、別のテナントの記録を操作しようとすることは拒否します。失敗したときはfailed(失敗)の状態と、直前の段階を保持します。

**検証**: 追跡表のAT-A15の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

### DD-A16 詳細

**一次資料との対応**: SRC-06 BIZ-20 → FR-A16 → DD-A16。出所区分: 企業原文 SRC-06＋設計での補足。この節で具体的にしている設計補足の内容: 監査の絞り込み条件と相関IDです。フィールドの型・必須かどうか・初期値・操作の順番は、こちらからの実装案です。

対象: FR-A16 / 主な表示パターン: **UI-TIMELINE / UI-DETAIL**。画面のサービス境界は`audit.list, devices.events`です。

**初期表示と前提**: audit.read(監査閲覧)の権限があること。管理しているテナントの範囲を超えず、機密な情報を含まない監査データ(projection)であること。表示の順番は、ルート・条件の検証 → session scope(セッションの担当範囲)の確認 → 必要なQueryの取得、です。「まだ取得していない状態」と「0件だった状態」は分けて扱います。

| フィールド | 型・必須性 | 初期値・制約 | 用途 |
|---|---|---|---|
| from / to | ISO日時/必須 | 最大366日 | 検索する期間 |
| actorId / targetId / correlationId | ID/任意 | 管理できる範囲内 | 絞り込み |
| result | enum/任意 | success(成功)/denied(拒否)/failed(失敗)/pending(保留、結果待ちの受付。確定時に同じcorrelationIdで追記、IR90) | 結果 |
| cursor / limit | 文字列・整数 | 初期値25件、最大100件 | ページ送り |
| before / after / reason | 読取 | 機密な部分はマスク済み | 変更前後の内容・理由 |

**処理手順**

1. 期間・実行した人・対象・結果・相関IDで検索します。履歴の詳細を開きます。関連するCommand(命令)・Job(依頼)・Restriction(制限)の状態と照合します。
2. 読み取りや操作にあたって、次の業務ルールを適用します。監査は「成功」「拒否」「失敗」「保留」を分けて表示します。before/after(変更前後の内容)は、秘密の情報や連絡先をマスクします。記録の追加はRepositoryの業務イベントだけが行います。この画面からの追加・編集・削除はできません。ブラウザの中だけのデモなので、記録が改ざんされないことまでは保証しません。
3. この画面は閲覧のみです。検索条件はURLに保持しますが、機密な本文はURLに入れません。
4. 更新する対象のQuery: `なし（監査参照）`。

**境界条件・失敗時**: 相関IDは認可済み集合内で検索し、他テナントの相関IDと存在しない相関IDは同じ成功空集合を返します。削除APIに相当する呼び出しと逆転期間は拒否します。役割を切り替えたあとに、実行した人の情報が別人に書き換わらないようにします。

**検証**: 追跡表のAT-A16の項目(N/E/B・該当するSRC/R01)と、対応するSシナリオで確認します。

## 共通操作のHQ導線（FR-X04/X06）

/admin/units?unitId=:idにCommandPanelを表示し、control.execute保持者がcommands.create/getを使用する。理由1〜1000文字必須。/admin/alerts?alertId=:idの確認・解消はalert.resolve、/admin/devices?deviceId=:idの登録・紐付け・接続確認・校正・FW更新はdevice.manage。入力・状態はDD-C03/DD-T11と確定契約D01/D05を共用する。権限なしは対象閲覧だけとし操作disabled理由を表示する。

DD-A08: 請求を選択したらinvoices.getを必須取得し、InvoiceDetail.paymentRefsから確認対象のpaymentId/versionを選ぶ。payments.confirmはPayment版、recordManualはInvoice版を使う。PaymentなしをIDの推測で補わない。

条件フォームはCondition型のtype判別unionへ変換する。occupancyは{type,occupied}、locationは{type,event}、patternは{type,localTime}、weatherは{type,metric:"temperature",operator,value}、tariffは{type,operator,value,unit:"MYR_per_kWh"}、peakは{type,active}、solar/batteryは{type,operator,value,unit:"kW"}。paramsという追加wrapperは送らない。天候の評価Fact.metricはweather_temperatureとし、室温temperatureのFactとは混同しない。

0.9.0修正契約: [厳格レビュー修正契約](strict-review-contracts.md)と[操作別版契約](write-version-catalog.csv)を併読する。

2026-09-16承認反映: A07はactive制限がある契約の保存を無効化し理由を表示、RepositoryもSR19を再検証する。A15はfailed時にoffsets.simulate(event=retry,recordId,attemptId,demoConfirmed=true)を新キー・現在版で呼び、最新attemptを再取得する。

0.10.0: A09の矛盾観測回復はSR26のrecoveryCasesを使う。A13の入力はBaselineInputで、demo_fixedだけbaselineKWhを入力し、demo_period_comparisonはRepositoryで値と品質を計算する。品質と仮定基準ラベルを同時表示する（SR29）。A14はunits.list(filters.organizationId)で候補を絞る。

A07の保存可否はContract.activeRestrictionIdsが空かつhasUnresolvedRecovery=false。A09の回復case解決で後継制限を解除しない。DeviceデモイベントのbindingIdはDeviceの取得値を使う（SR24/SR26）。

現行0.21.0の追加契約: [再レビュー修正契約](review-resolution-contracts.md) IR01〜106を併読する。同じ論点の旧記述より優先し、衝突時の順位はIR72に従う。

A13/A14の境界はIR11のboundaryId（固定候補）とboundary（説明）を区別する。MRVは保存済み版の画面プレビューまででファイルexportは対象外（IR15）。

0.15.0: DD-A06の受理はIR29/IR31に従い、完了日時をRepositoryで保存し、全寄与者の自己承認を拒否する。

案件一覧とjobs.listのソートはIR34を適用する。URL sort未指定はstatus:asc。選択変更でcursorを破棄し、filterを保持して新snapshotの初頁から取得する。状態/重大度/期限の昇降順を選べる。
