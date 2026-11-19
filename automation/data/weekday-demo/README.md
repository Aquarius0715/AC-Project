# 曜日まとめ・部分承認の90日入力

既存100世帯のデータを変えず、4世帯で表示・承認・通知の動作を確認する入力です。全世帯で、曜日ごとに異なる終了予定と、その予定に整合する自動オフ・30分電力を持ちます。

| 顧客 | 入力の違い |
|---|---|
| G001 | 毎曜日に20:00の確認済み手動開始。平日すべて、土日まとめの表示 |
| G002 | 同じ操作履歴に、水曜の固定（8/25 12:05）と解除（9/15 12:05） |
| G003 | 8/24に月・火・木・金を承認、水曜は今回だけ対象外。8/25から承認した4曜日だけ20:00の予定を実行 |
| G004 | 火曜20:15は別候補。水曜20:00を見送り、木曜・水曜の使い方が途中で21:00へ変わる |

仮データ生成のケース識別は生成器だけに置き、期待する件数・最初の判定日・支持日・承認対象はテスト側で照合します。判定処理はG001などのIDで分岐しません。

```bash
python3 -B -m automation.src.cli generate-weekday-demo --seed 42 --output automation/data/weekday-demo
python3 -B -m automation.src.cli replay --data automation/data/weekday-demo --output automation/results/weekday-cards/demo
```

`automation/results/weekday-cards/demo/index.html`を直接開きます。再生成はこのフォルダの入力JSONを上書きします。操作や判断の編集方法は親の`data/README.md`を参照してください。
