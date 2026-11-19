# 提案型エアコンオートメーションのWeb接続引き継ぎ

エアコンの操作・予定・世帯の在宅履歴を使う承認型デモと、既存 Web への接続範囲をまとめます。デモの詳しい操作と判定ルールは [README](README.md) を参照してください。

## 既存 Web に接続した範囲

`/customer/automations` の API モードで、Go API が PostgreSQL の現行ルール、操作履歴、独立した電源観測または検証済みの実測電力を読みます。機器・曜日ごとに、直近4回の完了した対象曜日を同じタイムゾーンで集計します。各日の最初の確認済み手動オンを1票とし、±15分に3日以上入る候補を、支持数、合計の時刻差、早い時刻の順に選びます。候補が現在の開始と一致する場合や終了以降になる場合は見送ります。この集計順はデモの `start_pattern` と同じです。

| 提案 | 既存の取得元と粒度 | 蓄積状況・実装に必要なデータ |
|---|---|---|
| 開始時刻 | `control.automations` は現行のルールと版。`control.commands` は機器ごとの操作、発生・送信・応答時刻、操作元、実行者を保持。 | 既存の `assets.units.observed_state` は最新値で、ACK からも更新されます。実機の MQTT state も IR で送信した設定値です。`control.power_observations` は現行バインドごとの独立したオン／オフ／不明を保持します。`monitoring.measurements` のエアコン入力電力は kW の1分スロットで、同じバインドとセンサーに検証済みの待機・運転範囲が揃った場合に使います。実機の範囲と履歴の蓄積状況は未確認です。 |
| 暑い日の温度 | `control.commands` の `set_temperature` に要求時刻と設定値。設備の `monitoring.measurements` はセンサー別・時刻別の室内計測。`weather_temperature` は評価時に渡せる事実。 | 設置地点と操作日時に対応する過去の実測外気温が必要です。外気温の本番提供元は未決定で、操作時の外気温系列と設定変更前の値を揃えられません。暑い日だけ有効な温度変更を再現できる履歴と実行時の気象入力が必要です。 |
| 電気代を抑える予冷 | Energy は実測の分スロット電力を集計します。既存の集計額は固定のデモ単価 (`tariff-demo-1`)。`tariff` は評価時に渡せる値。 | 契約・時間帯別の有効単価と、同一期間の現行・予冷それぞれのスロット別 kWh、必要時刻の室温と予測時点が必要です。観測済みの kWh やデモ単価だけでは算出できないため、見送ります。 |
| 在宅による見直し | DEC-109 の `control.presence_states` は物件・メンバーごとに最新の内外、最終変更・報告時刻を保持。 | 予定開始時点の世帯判定を遡れません。同意の有効期間、観測・受信時刻付きの履歴、世帯全員を判定できる範囲と観測の期限、および当日の予定版が必要です。未同意・未取得・失敗や当時の記録がない在宅状態は不明で、手動オンからの開始提案には影響しません。 |

開始時刻はルール所有者の `ui` のオンを対象とします。検証済みの電力範囲がない期間は、`control.power_observations` に現行バインドの独立したオフ→オンが、要求前と送信後の5分窓内で揃った場合に数えます。どちらの取得元でも、前後の観測間に別の電源操作の要求または送信が入った操作は除外します。

実測電力の利用は `control.start_power_bands` で検証済みの期間に限ります。実測値はテナント・ユニット・現行バインド・その機器上のセンサーが一致し、`measured / valid`、`kW`、`ac_input_electricity` であることを確認します。要求時刻を含む分の直前と、送信時刻を含む分の直後のスロットを各1件使います。それぞれの分内に受信済みで、前は検証済み待機上限以下、後は検証済み運転下限以上のときに支持にします。欠測・同分内の不確かな前後関係・重複・遅延到着・範囲の間の電力値は除外します。ACK が `sent_at` を補う場合も独立した前後計測が必要です。

範囲の既定値はありません。バインドごとに対象電力計の配線を確かめ、待機、低負荷、送風、温度到達後の間欠運転、起動直後の読み取りを実機で比較し、区別できる期間と範囲を確認してから使います。制限違反の50 Wは継続運転の検知条件です。`node_1` と `node_2` の電力を相互に補完しません。

デモの旅行・「今回だけ」の除外に相当する区分は現行のコマンドにありません。根拠は手動開始の時刻であり、過去の予定との差やその変更の動機は推定しません。本番 DB での履歴の蓄積状況は未確認です。

承認は、表示された提案 ID・曜日と `X-Expected-Version` をサーバーで照合し、既存と同じ顧客の操作権限で行います。承認結果と承認者・時刻は監査ログに残します。単一曜日なら元のルールを更新します。複数曜日なら元の ID に残りの曜日を置き、承認済み曜日だけを同じ所有者の別ルールにします。終了時刻・開始と終了のアクション（設定温度を含む）・条件・優先度・タイムゾーンは引き継ぎます。既存の同一機器に競合し得る予定がある場合、複数機器を対象とする場合、日をまたぐ場合は提案の対象外です。承認時に対象枠が実行中かスケジューラに未処理の過去枠があれば、次の確認まで更新を見送ります。保存から機器へのコマンドは発行されません。

### PostgreSQL での再現

標準の PostgreSQL と、固定の開発時計（2026-09-14）を使います。API 結合テストは専用 DB の標準 seed にテスト用の予定、コマンド、電源観測、または検証済みの電力範囲と分単位の計測を追加します。リポジトリ直下から実行します。

```sh
docker compose --profile infra up -d postgres
DB=ac_test_start_proposals sh service/api/scripts/resetdb.sh
(cd service/api && AC_TEST_DB=ac_test_start_proposals go test ./test/integration -count=1 -run TestStartProposals)
(cd service/api && AC_TEST_DB=ac_test_start_proposals AC_TEST_CLUSTER=1 go test ./test/integration -count=1 -run TestStartProposals)
```

ブラウザの再現は、新規の標準 seed を使うローカル開発スタックで行います。既定の Compose 構成は DB 名 `ac` を使います。別名の検証 DB を使う構成では、その DB 名に読み替えます。fixture は初回だけ投入します。

```sh
docker compose --env-file .env.local --profile full up -d --build
docker compose exec -T postgres psql -U postgres -d ac -v ON_ERROR_STOP=1 < service/api/test/fixtures/start_proposals.sql
(cd service/web && E2E_START_PROPOSALS=1 npm run e2e -- --project=customer start-proposals.e2e.ts)
```

`customer-a` で、水曜18:30→20:00と金曜18:30→21:15のカードにそれぞれ3日分の確認日が表示されます。履歴に ACK しかない機器のカードは出ません。水曜を承認すると水曜20:00、月・木18:30、どちらも23:30終了・24℃で保存されます。金曜の予定を別タブで編集した後で古いカードを承認すると、変更後の開始と新しい提案が表示されます。完全オートは Coming Soon です。


## これまでの経緯

1. 個別の仮ケースで、操作履歴を使った開始時刻の変更、暑い日の温度変更、料金と快適性を比較した予冷を検証しました。入力を世帯別のJSONに整理し、標準100世帯×90日を日付順に再生できるようにしました。
2. スマホ由来を想定した在宅観測を追加しました。GPSを持たないエアコンは固定の設置地点に対応付け、仮のWi-Fi接続やジオフェンスのイベントから世帯の在宅・不在・不明を判定します。
3. 開始変更は機器・曜日ごとに判定します。表示上の開始時刻が同じ平日または土日の案はカードにまとめ、曜日を選ぶ部分承認、今回だけの対象外、固定・解除を再現できるようにしました。通知は世帯あたり週1回までの予定をシミュレーションします。
4. 承認を予定版の有効期間と照合し、反映待ち、反映済み、後の承認で置き換え済みを表示します。一度反映した承認は、後から予定を編集しても最初の反映日時を保持します。未反映のまま置き換えられた承認は、後の編集で予定が偶然一致しても反映済みに変わりません。

## 現在の処理と最初に読むコード

| 段階 | 入口と処理 |
|---|---|
| 仮データの読み込み | [`load_dataset`](src/dataset.py#L292) が `dataset.json` と世帯・天気・料金の各JSONを読み、[`Dataset.prepare`](src/dataset.py#L113) が1世帯分を検証・整形します。CLIの入口は [`cli.main`](src/cli.py#L13) です。 |
| 判定日時点の入力 | [`Prepared.snapshot`](src/dataset.py#L218) で、判定日時までに発生・受信した操作、利用可能になった日次記録、発行済みの天気データ、登録・有効化済みの現行予定を切り出します。在宅は [`PresenceIndex`](src/presence.py#L12) が調査する時刻までに観測・受信した情報から求めます。 |
| 提案と根拠の生成 | [`evaluate_household`](src/presence_proposals.py#L222) が実際の電源遷移を確認し、[`evaluate_customer`](src/monthly_engine.py#L828) が機器ごとの候補と根拠を返します。不在を繰り返した曜日には見直しを提案でき、具体的な開始時刻には確認済み操作の根拠が必要です。 |
| 承認履歴の再現 | [`replay_customer`](src/replay.py#L351) が日次判定、候補の継続、新規提案ID、カード、通知予定を組み立てます。[`apply_feedback`](src/replay.py#L113) が判定日時までの反応を使い、[`card_approval_states`](src/approval_state.py#L34) が承認値と有効だった予定版を照合します。[`validate_response_references`](src/weekday_cards.py#L254) が、承認対象を当時表示した機器・曜日・提案ID・値・根拠と突き合わせます。 |
| デモ結果の確認 | [`write_report`](src/html_report.py#L82) がHTMLと評価JSONを出力します。HTMLの曜日チェックは承認範囲のプレビューです。結果の `days[].cards` と `days[].devices[].decisions` に提案と根拠、`days[].devices[].approval_states` に承認履歴が入ります。通知予定は `days[].notification` で確認できます。 |

判定は**履歴集計と固定ルール**で行います。開始時刻は、直近4回の完了した対象曜日のうち、候補の前後15分に確認済みの手動オンが3日以上あることが基本条件です。判定ロジックの起点は [`start_pattern`](src/engine.py#L42) と [`confirmed_starts`](src/presence_proposals.py#L17) です。

入力形式は [data/README.md](data/README.md) から読むと追いやすくなります。[標準データの一覧](data/dataset.json) が各世帯と共有ファイルを参照します。`customers/` の1ファイルに機器・操作・在宅・予定版・反応が入り、`weather/` と `tariffs/` をIDで参照します。[曜日まとめ用データ](data/weekday-demo/README.md) と [レビュー再現用データ](data/review-cases/README.md) は独立したデータセットです。

テストは、[入力と時間の前後関係](tests/test_dataset.py)、[在宅観測](tests/test_signals.py)、[曜日別・部分承認と通知予定](tests/test_weekday_cards.py)、[HTMLとCLI](tests/test_report.py) に分かれています。承認の状態遷移は [反映待ちと反映](tests/test_review_fixes.py)、[反映後の別編集](tests/test_approval_history.py)、[未反映の承認の置き換え](tests/test_approval_replacement.py) が入口です。

### 代表ケースを再現する

Python 3.12以上で、リポジトリ直下から実行します。コミット済みの仮データを使うため、生成は不要です。

```bash
python3 -B -m automation.src.cli replay --data automation/data --customer H008 \
  --output automation/results/web-integration/start
python3 -B -m automation.src.cli replay --data automation/data/review-cases --customer G003 \
  --output automation/results/web-integration/approval
python3 -B -m automation.src.cli snapshot --data automation/data/review-cases --customer G003 \
  --output automation/results/web-integration/snapshot
python3 -B -m unittest discover -s automation/tests -p test_approval_replacement.py -v
```

各出力先の `index.html` はブラウザで直接開けます。H008は判定日8/20に水曜18:30→20:00を提案し、10/1には水曜21:00の案が成立します。G003は8/24に平日カードを表示し、入力履歴の月・火・木・金の承認を8/25の判定で反映待ち、予定版が有効になった8/27に反映済みと表示します。水曜は今回だけ対象外です。`snapshot` は10/31の最終判定を表示します。置き換えの回帰テストは別のG003入力を組み立て、20:00→21:00の承認順序と、日次再生と各基準日時点の結果の一致を確認します。

## デモ時点のWeb接続例　水曜18:30開始を20:00に変える

### 既存Webにある入口

| 入口 | コードで確認できる機能 |
|---|---|
| [`/customer/automations` のページ](../service/web/customer/app/customer/automations/page.tsx#L18) | APIモードでは認証済み顧客の `automations.list`、機器・物件・部屋・設定・位置情報同意を読み、次回の開始を `automations.nextRuns` で取得します。デモモードでは既存の画面用仮データを表示します。 |
| [ルール一覧の `List`](../service/web/customer/app/customer/automations/_components/automations-view.tsx#L82) と [編集画面の `Editor`](../service/web/customer/app/customer/automations/_components/automations-view.tsx#L172) | ルールの表示・有効化・編集があり、編集時は未保存の予定を `automations.nextRuns` の `draft` でプレビューします。 |
| [`draftOf`](../service/web/shared/lib/clientAutomations.ts#L287)、[`saveInput`](../service/web/shared/lib/clientAutomations.ts#L313)、[`saveAutomation`](../service/web/customer/app/customer/automations/actions.ts#L24) | 取得した既存ルールから編集値と版を作り、Server Actionが `automations.save` を呼びます。保存はルールの更新で、保存時にエアコンへのコマンドは発行されません。 |
| [`coreCall`](../service/web/shared/lib/dal.ts#L164) と [Core APIの `save` / `updateAutomation`](../service/api/internal/modules/control/automations.go#L816) | DALが認証・テナント・メンバーシップのヘッダー、書き込みの `Idempotency-Key`、更新の `X-Expected-Version` を設定します。APIが機器や操作の可否を検証し、[`updateAutomation`](../service/api/internal/modules/control/automations.go#L1019) が現在のルール版と照合してから版を進めます。 |

### デモ時点の接続案

1. **実データから判定入力を作る。** 認証済み顧客が参照できる機器を基準に、機器ID、タイムゾーン、有効な予定と予定履歴、操作と前後の電源観測を対応付けます。スマホ在宅を使う場合は同意と観測・受信時刻も渡します。現在のルールを曜日別へ展開するとき、Web/APIの曜日は月1〜日7、Pythonは月0〜日6です。水曜はAPIで3、Pythonで2です。開始変更に必要な実測根拠が取得できることを確認してから評価します。
2. **提案と理由を表示する。** 既存ルール一覧の近くに提案欄を設ける案です。対象エアコン、水曜18:30→20:00、維持する終了時刻と操作設定、根拠となった観測日・件数を表示します。たとえば「直近4回の水曜のうち3日で、20:00前後に手動で電源を入れたことを確認」です。表示値は `cards[].members[].values`、理由と根拠は判定の `reason`・`evidence`、支持日はカードの `support_dates` を参照できます。候補の内部計算値 `candidate_values` と、承認する表示値を区別します。
3. **水曜の承認を受ける。** サーバーで対象顧客・機器、曜日別の提案ID、表示時の値・根拠、元になったルールIDと版を照合し、承認者・日時・選んだ曜日を記録します。Webの書き込みでは、デモの連番 `proposal_id` をそのまま永続IDとせず、再評価後も承認対象を特定できるIDを決めます。カードのまとめ単位と承認の単位は分け、チェックを外した曜日の変更は作りません。
4. **承認した変更を既存予定に反映する。** 対象が水曜単独のルールなら、終了時刻・開始/終了アクション・条件などを保ったまま `startLocal` を20:00にして既存の保存経路を使えます。更新には、提案の前提として記録し承認時に再確認した **Automationの `version`** を `expectedVersion` として渡します。これは機器の版やデモの `schedule_versions` の番号とは異なります。Core APIで版が不一致なら `409 CONFLICT`（`error.versionConflict`）です。最新予定と差分を取り直して再提示し、承認の前提を確認してから保存します。成功後は保存されたルールとその有効時点を承認記録に対応付け、承認受付から予定反映までを追えるようにします。 [更新ルートの定義](../service/api/internal/modules/control/swagger_routes.go#L21)

## デモ時点の確認事項

| 判断事項 | 具体的な確認例 |
|---|---|
| 操作履歴と電源観測 | [`commands.list`](../service/api/internal/modules/control/commands.go#L574) には機器ごとのコマンド履歴があり、`source`・要求/応答日時・状態を取得できます。[`units.get` の `observedState`](../service/api/internal/modules/assets/units.go#L20) には電源値と観測時刻があります。コマンドの応答や最新の観測値だけでは、操作前後の電源遷移は確定できません。実環境で、必要な期間の操作履歴と、操作前後それぞれ5分以内の独立した電源観測・受信時刻を取得できるか確認します。リモコン由来の操作、手動と自動の区別、遅延・欠損時の扱いも確認します。 |
| Pythonの呼び出し方 | サーバー側の定期ジョブ・ワーカーで呼ぶか、内部API経由で再利用するかを決めます。現行CLIはJSONファイルとローカルレポートを使う検証入口です。1世帯・1判定日時の入出力、スキーマの版、再実行とタイムアウト、権限で絞ったデータの受け渡しを設計します。 |
| 承認・予定版の保存 | 提案の表示内容、承認者、機器・対象曜日、元のルール版、承認順序、適用結果と最初の反映日時をどこに保存するか決めます。予定版を `recorded_at`・`valid_from` 付きで遡れるよう、既存履歴で復元できる範囲も確認します。承認受付とルール更新を別処理にするなら、失敗後の再開と同じ承認の二重反映防止も要件に含めます。既存の `saveAutomation` はDALの既定で呼び出しごとにキーを生成するため、同じ承認を再試行する場合の安定した冪等キーの渡し方を決めます。 |
| 月〜金18:30のうち水曜だけ承認 | APIの1ルールには共通の `startLocal` が1つあります。元ルールを月・火・木・金 `[1, 2, 4, 5]` の18:30に更新し、水曜 `[3]` の20:00を別ルールとして作成する案があります。元ルールの更新には `id` と版を指定し、新ルールの作成では両方を省きます。終了時刻・アクション・タイムゾーン・条件を引き継ぎ、既存の他ルールや複数台対象を確認します。現行の `automations.save` は1ルールずつ保存するため、分割を一体で更新するAPIを用意するか、途中失敗・重複・欠落に対処する手順を定めるかを決めます。 [ルールの入力型](../service/api/internal/modules/control/automations.go#L351) |
| 承認前に予定が編集された | 表示時に版7の水曜18:30を前提にしていて、別編集で版8になった場合は、古い変更をそのまま適用せず再読込・再判定します。承認受付後の競合もAPIの版チェックで検知し、再提示時に保持する承認記録の扱いを決めます。 |
| 同時刻に複数の承認が記録された | 同じ機器・水曜の20:00と21:00が同一タイムスタンプなら、どちらを後とするかは時刻だけでは決まりません。現行デモは厳密に後の応答時刻を置き換え条件にしています。Webではサーバー採番や版を使って同一機器・曜日の順序を決めるか、競合として再確認を求めるかを決めます。別機器・別曜日の承認と、既に反映済みの履歴は区別して扱います。 [承認状態の判定](src/approval_state.py#L34) |
