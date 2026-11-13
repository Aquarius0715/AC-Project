# 入力JSONを開く・編集する

- `dataset.json`: 100世帯のID一覧、90日の対象期間、判定時刻、共有ファイルの参照。
- `customers/H001.json`など: 1世帯1ファイル。圧縮されていない整形済みJSONです。
- `weather/tokyo-east.json`, `weather/kanagawa-west.json`: 地点別の実測相当・予報相当の天気。
- `tariffs/time-of-use.json`, `tariffs/flat.json`: 30分ごとの共有料金単価。

顧客は`installation.weather_site_id`で天気、`devices[].tariff_id`で料金を参照します。住居の`installation.id`は天気地点IDと別物です。複数台でも住人と`presence`は世帯で共有します。エアコンにGPSはありません。

## 顧客ファイルの主な項目

| フィールド | 内容 |
|---|---|
| `household_id`, `installation`, `residents` | 世帯ID、固定の設置地点、住人ID一覧 |
| `presence` | 住人・設置地点、`observed_at`, `received_at`、情報源・イベント・home/away/unknown、利用可否 |
| `devices[].schedule_versions` | `recorded_at`までに登録し、`valid_from`から使う曜日別予定。日時順で置く |
| `devices[].days` | 日付順の連続90日。操作記録の状態、利用有無、操作、48区間のkWh、`available_at` |
| `days[].operations` | `at`, `received_at`, 操作元、on/off/temperature、今回だけ、設定温度、電源の前後観測 |
| `operations[].power` | 前後の時刻と`before`/`after`、受信日時。実行確認が不明なら`null`。温度操作には不要 |
| `devices[].trips` | 旅行などの停止期間。開始以上・終了未満 |
| `devices[].predictions` | 将来の通常運転・快適性・予冷候補の区間kWh、必要時刻の室温、生成日時、予報の根拠署名 |
| `feedback` | 過去の提案・反応日時、機器・種類・曜日・内容・根拠署名、反応、修正後の`selected` |

`response`は`approved`（承認）、`edited`（修正承認）、`declined`（見送り）、`no_response`（無反応）です。`proposed_at`と`responded_at`は別で、反応より前の評価へ先取りしません。予定に実際に反映された過去の変更は`devices[].schedule_versions`へ記録します。CLIが入力を自動更新することはありません。

## 編集の例

1. `customers/H001.json`をVS Codeなどで開きます。
2. 例えば18:00の`phone_geofence/heartbeat/away`を、同じ設置地点の`home_wifi/connected/home`へ変えます。位置情報が利用不可でも、`sharing_enabled=true`で自宅Wi-Fi接続があれば在宅を確認できます。
3. 下記を実行し、最初の提案日と不在支持日が変わるか確認します。

```bash
python3 -B -m automation.src.cli replay --data automation/data --customer H001 \
  --output automation/results/edited-H001
```

`results/edited-H001/index.html`を開いてください。元の評価結果と比較する場合は、編集前後で出力先を変えます。入力を元に戻すときは、メインREADMEの固定seed生成コマンドを実行します。**再生成は編集したデータを上書きします。**

## 単位・日時・欠損

- 日時はUTCオフセット付きISO 8601です。住居のタイムゾーンを使い、標準データは`Asia/Tokyo`です。
- 48要素の配列は現地00:00〜00:30を添字0、23:30〜24:00を添字47とする30分区間です。kWhは区間全体の電力量なので、料金計算で時間をもう一度掛けません。
- `0`は既知のゼロ、`null`は欠損。区間料金は`kWh × 円/kWh`です。不明をゼロや無料として補いません。
- `operation_status=complete/missing/not_enrolled`と`usage=used/unused/unknown`を区別します。未利用確認日は操作なし、電力は全区間既知のゼロが必要です。
- `available_at`はその日の記録が利用可能になった時刻です。スマホは各観測の受信時刻、天気は発行時刻、予測は生成時刻も評価に使います。
- IDは識別子です。時刻や操作の意味はID文字列から推測せず、`at`・`source`・`action`などのフィールドを参照します。

操作の時刻・稼働時間を自分で変更した場合、電源の前後観測とkWhも対応させてください。評価器は入力の電力を勝手に再計算しません。生成器は操作・天気・設定温度から整合した電力を作ります。

過去の観測kWhと将来の候補予測は別です。天気予報を変更した場合は、その前提による候補の予測も作り直す必要があり、前提の署名が不一致なら予冷を見送ります。過去の見送りにある根拠署名は、その時点の根拠です。履歴を変えて署名が変わった場合は、新しい根拠として再評価されます。

生成する曜日別予定の終了は22:00・22:30・23:00・23:30に分散しています。自動オフはその日に有効な予定の終了時刻に対応し、電力は実際の操作から再積算しています。開始だけの提案は、この入力の終了時刻と設定温度を維持します。終了時刻を編集する場合は、過去の自動オフ・電源の前後観測・kWhと、今後の通常運転/予冷候補の比較終了・配列の区間数もそろえてください。

評価JSONの開始提案は、`values`が表示・承認対象、`candidate_values`が毎日の計算結果です。継続中は初回表示から±15分以内の候補変化でも`values`と提案IDを維持します。仮の反応を追加するときは、候補ではなく**その日に表示した`values`と根拠署名**を参照してください。`selected`は修正して選んだ値です。出力上の提案IDを記録する場合は`feedback.proposal_id`に残せます。

## 曜日まとめ・部分承認の追加データ

`weekday-demo/`は、既存100世帯とは別に開ける4世帯×90日の入力です。`weekday-demo/dataset.json`から参照し、顧客JSONは`weekday-demo/customers/G001.json`〜`G004.json`です。共有する天気・料金は、この独立したデータセットの`weather/`・`tariffs/`を使います。根拠判定は既存と同じです。

| 追加の顧客フィールド | 内容 |
|---|---|
| `start_controls` | 明示的な曜日固定・解除の記録。`id`, `device_id`, `weekday`（月0〜日6）, `action=lock/unlock`, `at`, `received_at` |
| `card_responses` | カードを見て選んだ曜日別の反応。`id`, `card_id`, `proposed_at`, `responded_at`, `members` |
| `card_responses[].members[]` | その時の`device_id`, `weekday`, `proposal_id`, 表示した`values`, `evidence_ids`, `evidence_signature`, `decision` |

`decision`は`approved`（その曜日を承認）、`declined`（その曜日・候補を見送り）、`excluded_once`（今回の承認だけから外す）です。固定は別の`start_controls`へ記録します。カード全体の承認を、選ばなかった曜日へ拡大しません。見送りの対象をカードIDだけで判定せず、曜日と表示した候補を保持します。

反応を編集するときは、先に再生して表示された日時・カード・各曜日のIDと内容・根拠をコピーしてください。`responded_at`はその表示判定時刻より後にします。別の曜日のIDや候補に取り替えると入力エラーです。実際に予定変更した過去の事実は、承認した曜日だけの`devices[].schedule_versions`にも記録し、自動操作・電源・電力を対応させます。HTMLの曜日チェックは保存用ではなくプレビューです。

G003では8/24のカードを見て水曜だけ`excluded_once`、他4曜日を`approved`としています。G004は水曜の20:00を`declined`とし、10/1には新しい操作の根拠で21:00を評価できます。支持日が増えただけで20:00を再提示することはありません。詳細な再生コマンドは`automation/README.md`を参照してください。
