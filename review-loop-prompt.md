/loop プロジェクトのコードを1領域ずつ厳しくコードレビューし、見つかった問題を修正する。

【各回の手順】
0. 【最初に】開いている PR を確認する（`gh pr list --state open --author @me`）。1 件でもあれば、この回は新しい PR を出さない：
   - 例外：下書きの `release/ios` → main の PR（#621、マージの時期はユーザーが決める）は数えない。触らない・更新しない。
   - main ブランチ保護が「最新の main を取り込んだブランチのみマージ可」なので、複数の PR を同時に開くと 1 件マージされるたびに残りが BEHIND になり、
     CI が無駄に回り直す。PR は 1 件ずつ、マージされてから次を出す。
   - その PR が `BEHIND` なら `gh pr update-branch <番号>`、CI が落ちていれば原因を直して push する（直せなければ記録して終了）。
   - CI が通って自動マージされるまで待ち（マージされたら次へ）、待つ間は読むだけのレビュー（手順 1〜2、A は直さずメモのみ）にとどめる。
     読んだ結果が A 相当でも、開いている PR がある間は記録に「次の回で直す」と書いてコードは変えない。
   - 開いている PR が 0 件なら、通常どおり手順 1 へ進む。
0b. 前の回が途中で打ち切られた跡を確認する（再起動、kill、予算切れで起こる）：メインツリーの `git status` と `git branch --no-merged origin/main`、
   `git worktree list`、review-log.md の末尾（記録が書かれていない回の番号）。
     iOS の回のブランチ（`review/r<番号>-ios`）は `git branch --no-merged origin/release/ios` で見る（main には入らないので、main との比較では残って見える）。
   - 未コミットの変更・PR のないブランチがあれば、その続きから再開する（領域を選び直さない）。壊れていて続けられなければ理由を記録に書いて捨てる。
   - 記録のない回があれば、その回が何をしていたか（ブランチ名 r<番号>、`.review-loop-logs/` の最後のログ）を 1 行で記録する。
   - 開始時の `review/r<番号>` ブランチが残っていても、変更がなければそのまま使ってよい。
1. 領域を1つ選ぶ：次の順に回し、前回までの記録（docs/05-document-review/review-log.md）を見て、
   未レビューのもの・最後のレビューから変更が多いもの・前回問題があったものを優先する。
   - service/api：ドメインサービスを1つずつ（maintenance / equipment / billing / energy / identity）、
     gateway、worker、iot-bridge、platform 配下の共通パッケージ、schema.sql とマイグレーション
   - service/web：shared（lib・components）、customer / partner / technician / admin の各アプリ（BFF・DAL・画面）
   - scripts、CI（.github/workflows）、docker / compose / deploy
   - service/ios（2026-11-22 ユーザー指示で対象に追加）：ACKit の ACKit（Auth・Keychain・CoreClient）、ACShared（共通部品・Debug のスタブ）、
     CustomerFeature、TechnicianFeature（モデルと画面）、両アプリ（CustomerApp / TechnicianApp）、UI テスト（*UITests、UITestsShared）、
     service/ios/scripts と project.yml、生成器（service/web/codegen/swift*.ts、operations.json、strings.json）。下の【iOS の回】に従う。
   - 【必須】まず下の未レビュー一覧を上から見て、review-log.md に「新たに読んだ」と記録のない最初の項目を、この回の領域にする。
     一覧に残りがある間は、上の領域リストや「前回問題があったもの」より優先する（他の領域を選ぶなら、その理由を記録に 1 行書く）。
     ⓪ service/ios（まだ一度も読んでいない。①〜⑤と交互に 1 回ずつ：前の回が iOS でなければ iOS を選ぶ）を次の順に：
        ACKit（Auth、CoreClient、Keychain）→ ACShared（Debug を除く）→ CustomerFeature → TechnicianFeature の Jobs・CheckIn・Report・Unit・
        Alerts・Diagnostics・Devices・Overview → 両アプリと UI テスト → scripts と生成器（1 回 5〜9 ファイル・千行以内）
     ① service/api/internal/ops：changes, client, domains, forcedelete, internalcaller, query, readcheck, rest（1 回 3〜4 ファイル）
     ② server/crossdomain.go, server/unitdetail.go
     ③ modules/devices の iot_models.go, model_register.go（DEC-105）、modules/monitoring/escalation.go、iotbridge/dispatch.go
     ④ web の shared/screens、shared/bff
     ⑤ web 各アプリの _components と _lib（*-demo.tsx / *-view.tsx）、アプリごとに 1 回
     読んだら記録に「新たに読んだファイル: …」の行を書く（これで残りが分かる）。
     一覧が空になったら、ファイル名が review-log.md に一度も出ない git 管理ファイルを自分で探して続ける。
   - 1回の範囲は読み切れる大きさ（目安：数ファイル〜1パッケージ）に絞る。
   - 間引き：前回のレビュー以降に差分がほぼない領域（`git diff --stat` で数行以下、前回の問題もなし）は、
     連続 2 回までスキップして他の領域を優先し、3 回目に回す。スキップしたことも review-log.md に 1 行残す。
2. コードを実際に読んでレビューする。観点：
   - 正しさ：ロジックの誤り、境界値（0件・上限・空文字・null・タイムゾーン・日付またぎ）、状態遷移の抜け
   - エラー処理：握りつぶし、wrap 漏れ、間違ったエラーコード、ユーザーに届くメッセージ
   - データ整合性：トランザクション境界、楽観ロック（version）、冪等性、イベント（outbox）と ref_* コピーのずれ
   - 並行性：競合、二重実行、goroutine / 接続 / rows / body のリーク
   - 認可：スコープ・テナント・ロールの確認順（行を読む→スコープ確認→version）、IDOR
   - 入力検証：境界での Zod / Validate()、型の抜け道
   - Web：React hooks の依存、サーバー／クライアント境界、キャッシュと再検証、a11y
   - テスト：重要な分岐にテストがあるか、テストが本当に失敗を検出できるか（常に通るテストはないか）
   - 【必須・2026-11-17 ユーザー指示、2026-11-18 目標を 100 % に変更】テストカバレッジを毎回測り、すべてのパッケージ・ファイルで 100 % を目指して、足りない分を A として直す（テストを足す）：
     - Go：領域のパッケージを `go test -count=1 -coverpkg=./internal/<領域>/... -coverprofile=<worktree>/.review-loop-logs/r<番号>.cover ./internal/<領域>/... ./test/integration/...`
       （service/api で実行、統合テストは新しく作った DB が要る：`DB=ac_test_s1 sh scripts/resetdb.sh` のあと `AC_TEST_DB=ac_test_s1` を付けて流す。
       同じ DB にパッケージのテストを先に流すと統合テストが残った行とぶつかって落ちる）で測り、`go tool cover -func=…` で 100 % でない関数と count 0 のブロックを一覧にする。
       ブロックは行の途中から始まるので列も見て判定する。全体は `make cover`（約 5 分、dev stack の API を止めてから）で測る。
     - Web：`npx vitest run --coverage --coverage.include=<領域のファイル>` で測る（`@vitest/coverage-v8` は導入済み、#453）。
       対象は _lib・actions・BFF・DAL・mapper・hooks に加え、画面の部品（*-view.tsx / *-demo.tsx / _components）も testing-library + jsdom で測る。
       vitest の設定がないアプリ（partner など）は、admin / customer と同じ vitest.config.mts と `test` スクリプトを足す。
     - 足す順番：まず重要な分岐（Validate のフィールド規則、状態遷移、認可の拒否、エラーコードの対応づけ、境界値）、次にそれ以外のすべての分岐。
       `if err != nil { return … }` のようなエラーの経路も 100 % の対象にする：取り消した context、閉じたプール、権限のない DB ロール、
       既存のインターフェースの fake などで実際に起こし、「どのエラーコードが返るか・ロールバックされるか・副作用が残らないか」を確かめる。
       エラーを起こすためだけに本番コードを大きく変えない（関数の分割や差し替え口の追加は、小さく理由を説明できる場合だけ）。
     - テストは行を通すだけでなく結果を確かめる（返り値・エラーコード・DB の状態）。修正や分岐を一時的に壊して、そのテストが落ちることを確かめる。
     - どうしても到達できないコード（起こりえない防御分岐）は、本当に不要なら消す（A）。残す理由があるものは記録に「到達不能: file:line / 理由」と書く。
       これだけが 100 % に届かない理由として認められる。
     - 判定結果は記録に必ず 1 行書く：「カバレッジ: <値>（測定コマンド）/ 100 % でない箇所: … / 足したテスト: … / 到達不能: …」。測れなかったら理由を書く。
     - テストを足すのは 1 回の PR に収まる範囲（目安 10 件まで）。残りは「次の回で足す」と記録し、その領域が 100 %（または到達不能だけ）になるまで同じ領域を続ける。
     - 未判定のモジュールがなくなったら、最後に測った一覧でカバレッジの低いパッケージから順に選ぶ
       （2026-11-17 の全体測定 88.9 %：bootstrap 78.2、seed 82.9、platform/idp 83.9、devices 86.2、restrictions 87.0、server 87.2、
       maintenance 87.4、billing 87.7、monitoring 88.5、identity 89.4；web は admin / customer / technician の _lib・actions が 4〜20 %、partner は vitest なし）。
   - 仕様との一致：docs（要件・API・DB・画面カタログ）と Figma の記述がコードと食い違っていないか
   - CLAUDE.md の lint 憲法（理由のない suppression、キャスト、any、名前・分割による lint 回避）
3. 分類して対応する：
   - A（その場で直す）：バグ、漏れ、テスト不足、docs のずれ。まず失敗するテストを書き、修正して通す。
     docs / Figma と食い違うなら同じ回でそろえる（IR 番号を振る）。
   - B（提案だけ）：仕様・権限・フローの変更、大きな設計変更、判断が必要なもの。実装はしない。
     「何が問題か・影響・選択肢・推奨案」を docs/05-document-review/review-proposals.md に追記し、ユーザーの判断を待つ。
4. 完了の確認（CLAUDE.md §5）：
   - Go：`golangci-lint run ./...` が 0 issues、`make test-all` が通る
     （dev stack の API コンテナが動いていると接続枠不足で落ちるので、止めてから実行し、終わったら戻す）
   - Web：`npm run lint`、`npm run typecheck`、`npm test` が 0 errors
   - `sh scripts/check.sh` の終了コードを取得して 0 を確認する（パイプしない）
   - これらの生の出力を記録に残す
   - 【PR を出す前に必須】Go のコードを 1 行でも変えたら、PR の前に `make -C <worktree>/service/api lint` が `0 issues.` になるまで直す
     （2026-11-12 に #242・#244 が lint 未実行のまま出て CI で落ちた。整形は `go run github.com/golangci/golangci-lint/v2/cmd/golangci-lint@v2.15.0 fmt ./...`）。
     テストコードも対象：構造体リテラルは全フィールド（exhaustruct）→ `new(T)` に代入、`if err := …; err != nil` は使わない、短い変数名を避ける。
     走らせられなかったら PR を出さず、ブランチをローカルに残して記録に理由を書く（CI に任せない）。
   - 非対話の実行では複合コマンド（`cd … && …`、`;`）とプロジェクト外へのリダイレクトが拒否される。`make -C <dir> …` を使い、
     check.sh のログは `.review-loop-logs/` 内の絶対パスに書く。
   - 非対話の実行では `$?` が拒否される。check.sh は `sh scripts/check.sh > $LOG 2>&1` で実行し、ログ末尾の「all checks passed」の有無で成否を判定して、その旨を記録に書く。
5. PR を作る：
   - origin/main から作業ブランチを切る（PR を積み重ねない）。iOS の回は origin/release/ios から切り、release/ios に出す（【iOS の回】）
   - ブランチ全体の差分をレビューし、指摘がなくなるまで修正→再チェック→再レビューを繰り返す
   - 指摘ゼロになってから push し、`gh pr create`（本文にレビューループが指摘なしで終わったことを書く）
   - push は `git push origin <ブランチ名>` の形で行う（`-u` は付けない。非対話の実行では `git push -u origin …` が許可に一致せず止まる）。
   - CI の結果を確認する
6. 結果を記録する（review-log.md）：回の番号、領域、読んだファイル、見つけた問題、A で直したもの（IR 番号・PR）、
   B に回したもの。問題がなければ「レビュー済み・問題なし」と正直に書く。
   - 書き方：review-log.md と PR 本文は、ツール呼び出しやファイルを見ていない読み手が分かるように、要点（何が壊れていて、どう直したか）を
     最初に書き、その回だけで作った略語や呼び名を使わない。
   - 最後のメッセージに、その回の要約（領域・結果・PR・次の回にやること）を自己完結で書く。

【iOS の回】（service/ios は release/ios にだけある。main へのマージ時期はユーザーが決める）
- ブランチは origin/release/ios から `review/r<番号>-ios` を切り（`git switch -c review/r<番号>-ios origin/release/ios`）、PR は
  `gh pr create --base release/ios` で出す。release/ios も必須チェックが通ると自動マージされる。main 向けの修正（Go・Web）を見つけたら、
  同じ PR に混ぜず、記録に書いて main 向けの回で直す。
- 観点は手順 2 に加えて：force unwrap / force cast / force try、`Any` の使用、理由のない `swiftlint:disable`、生成物（Generated/、
  Localizable.xcstrings）の手編集、`tr("…")` を通らない表示テキスト、Codable の厳密さ（必須フィールド・リテラル）、Keychain と
  Info.plist の型付きの読み取り、@MainActor と Sendable、Task の取り消しとリーク、サインイン（PKCE・トークンの更新と破棄）、
  送信失敗時に下書きを端末に残すこと（自動再送しない）、Web 版（service/web/technician・customer）や docs/02-design との食い違い、a11y。
- 確認：直している間は `sh service/ios/scripts/check.sh lint` と `sh service/ios/scripts/check.sh unit`（単体テストとカバレッジ、数分）を回す。
  カバレッジは scripts/coverage.sh が手書きコードの行と関数で 100 % を要求する（足りなければ落ちる）。記録の「カバレッジ:」行には
  モジュールごとの TOTAL 行を書く。行を通すだけで結果を確かめていないテストも A として直す。
  最後に `sh scripts/check.sh`（release/ios のブランチでは iOS の段＝lint・単体・両アプリの UI テストも含む、約 15 分）を通す。
- UI テストのビルドで Xcode が Shared/Localizable.xcstrings を書き換えることがある：check.sh が「not current: Shared/Localizable.xcstrings」で
  落ちたら `npm --prefix service/web run gen:swift` で作り直す（手で直さない）。表示テキストを足したら strings.json に 4 言語をそろえる。
- 整形は `swift format -i -r <ファイル>`、Simulator が固まったら `xcrun simctl shutdown all`。非対話の実行で許可されていなければ、
  ブランチをローカルに残して記録に理由を書く。
- 実機（iPhone）への配布、TestFlight、署名（DEVELOPMENT_TEAM）は扱わない（2026-11-22 ユーザー：TestFlight はやらない）。
- 仕様（DD-T01〜T16、FR-C・FR-T のモバイル行、IR860〜）は docs/02-design、Web 版は service/web/technician/app/technician/**。

【作業を最後まで終える】
- 普通の障害は原因を調べ、再試行するか別の方法で進める：CI のタイムアウト、不安定なテスト、lint の指摘、`gh` の一時エラー、
  DB の接続枠不足（dev stack の API を止めてから流し直す）、新しい worktree の node_modules 不足（`npm ci`）。
- 意図的な制約は越えない：手順 0 の「開いている PR があれば新しい PR を出さない」、実機・実 MQTT への書き込み禁止、
  main への push 禁止、B に分類したもの。見つけたことを記録に書き、安全に進められる別の作業（読むだけのレビュー）をする。
- 「1 回（1 PR）で止まる」のは意図した終わり方。それより前に「残りは次の回」と止めない（テストの 10 件上限など、手順にある区切りは除く）。

【確認なしで進めてよい手順】
- 同じ結果になる再実行：テスト、lint、`sh scripts/check.sh`、`gh pr checks`、`gh pr view`、`gh pr update-branch`。
- `npm ci`、`ac_test_*` のテスト用 DB の作り直し（`scripts/resetdb.sh`）、`make test-all` のための dev stack の API コンテナの停止と再起動。
- ローカルコミット、`review/r<番号>` ブランチの `git push origin <ブランチ名>` と `gh pr create`。

【ルール】
- 推測で直さない。コードを読み、テストか再現で問題を確かめてから直す。
- 好みだけのリファクタリングはしない。誰の何が壊れる／危ないかを説明できるものだけ直す。
- 1回の変更は小さく保つ。大きな作り直しは B に回す。
- lint の指摘は原因を直す。3回試しても消えなければ止めて、ログを示して相談する。
- 【サブエージェント（ユーザー許可済み 2026-11-11、実測と理由はメモ review-loop-agent-setup）】親は軽く保ち、重い読み込みだけを子に出す。
  - 親が自分でやる：手順 1（領域選び。review-log.md は末尾 40 行と `git diff --stat <前回のレビュー時点>..HEAD` だけ読む）、
    3A（テストを書いて直す）、4（検証。`cmd > $LOG 2>&1; echo rc=$?` で終了コードを見て、失敗時だけログを読む）、6（記録）。
  - 子に出す（読むだけ。1 回につき最大 1 体、並列にしない、sonnet の Explore）：
    1. 手順 2 のレビューは、領域が 600 行を超えるときだけ（以下は親が読む）。対象は 1 回 5〜9 ファイル・千行以内に絞り、
       ヘルパーを辿るのは指摘の確認に必要なときだけと指示する。返却は「file:line / 問題 / 根拠 / A か B / 重大度」で最大 10 件、
       問題なしなら 1 行「問題なし（読んだファイル一覧）」。ファイルの中身や長い引用は返させない。
    2. 手順 5 の差分レビューは、差分が 100 行以上のときだけ。親の結論や修正理由は渡さず、`git diff origin/main...HEAD` だけを見て独立に指摘させる。
  - 自分の作業をレビューさせるためだけに子を起動しない（上の 2 は除く）。
  - 親は子の指摘を鵜呑みにせず、A にする前に該当箇所を読んで再現かテストで確かめる。重大度 low の指摘は B に回してよい。
  - 子に渡すプロンプトは自己完結にする（対象パス・観点・返却形式・「推測で書かない・長文を返さない」）。
  - advisor：lint の指摘が 2 回直しても消えない、または A か B か判断が割れるときだけ、親より強いモデルの子（読み取り専用）に
    1 回の loop につき最大 1 回聞く。渡すのは失敗ログの要点（20 行以内）と対象のファイル・行、返却は「原因・推奨する修正・根拠」10 行以内。
    親が Fable 5.2 なら advisor は使わない。3 回目も失敗したら CLAUDE.md §4 どおり止めて相談する（advisor で覆さない）。
  - 子が 3 回連続で空振り（指摘ゼロで、後で親が問題を見つけた）なら、その領域は親が直接読む。
- 1 回（1 PR）で終わる。状態は review-log.md だけに残し、次の回は scripts/review-loop.sh が新しいセッションで始める。
- worktree で作業する回は、git 管理外の review-log.md と review-proposals.md をメインツリーの絶対パスで読み書きする。
- 新しい worktree では check.sh の前に `(cd service/web && npm ci)` が要る（node_modules が無いと web の 7 ジョブが exit 127）。check.sh は再生成後に未コミットの変更があると rc=1 になるので、先にローカルコミットしてから実行する。
- main へ直接 push しない、force-push しない、マージしない：main はブランチ保護されていて、CI（必須チェック）が全部通ると
  .github/workflows/auto-merge.yml が自動でマージする（2026-11-08 のユーザー指示）。落ちた PR は直す。
- 実機（Live · Daikin AC）と実 MQTT ブローカーには書き込みをしない（実機のエアコンが動くため）。実機のデータを読むのはよい（2026-11-02 ユーザー許可）。
  実機に書き込む経路の問題は A で直さず B に回す。
- 加えて各回でセキュリティ脆弱性がないかも確認する。
