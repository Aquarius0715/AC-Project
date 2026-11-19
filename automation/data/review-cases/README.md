# レビューf258d3fの90日再現入力

| 顧客 | 再現内容 |
|---|---|
| G003 | 8/24 12:05に月・火・木・金を承認、水曜は今回だけ対象外。予定版は8/24登録、8/27から有効。8/25・8/26はまだ18:30に自動オンし、19:00手動オフ、20:00手動オンする |
| H011 | 8/10（月）の予冷を8/9に見送る。8/17（月）は同じ設定の別の実行日。8/12分で週の通知上限を使い、8/17分は8/14から翌週へ持ち越す。8/15分は期限切れで除外 |

既存の仮データから再現可能なコピーを生成します。元の100世帯・曜日まとめ用4世帯は変更しません。H011の見送りには実行日の明示項目を入れず、既存の予報参照・提案ID・根拠署名から対象日を特定します。

```bash
python3 -B -m automation.src.generate_review_cases --output automation/data/review-cases
python3 -B -m automation.src.cli replay --data automation/data/review-cases --output automation/results/review-fixes/after
```

`automation/results/review-fixes/index.html`から修正前後を開けます。期待する判定日・承認状態・通知対象は`tests/test_review_fixes.py`側に置き、判定処理は顧客IDで分岐しません。
