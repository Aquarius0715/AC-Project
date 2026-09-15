# 全ドキュメント厳格レビュー

レビュー日: 2026-09-15 / 対象版: 0.5.0 / 基準コミット: `4c6001c608629a347606a2245520b0a60f48ed90`

対象: レビュー開始時の32文書・資料（Markdown 26、CSV 4、TXT 1、JSON 1）。`.DS_Store`は対象外。仕様本文は変更していない。本書の指摘はレビュー担当による評価・修正提案であり、企業の追加要件や承認ではない。

## 1. 総合判定

**要修正。現状のままG1（設計準備）を合格にして、一式を実装へ引き渡すことは推奨しない。**

出所の区分、4役割、フロントエンド限定、共有モック、独立した試験・レビュー、人への限定的な判断依頼という骨格は整っている。しかし、実装担当とテスト担当が同じ仕様から異なる結論に至る具体的な矛盾・契約不足がある。件数とリンクの一致をもって、意味上の整合性や実装可能性まで確認済みとはできない。

| 観点 | 判定 | 主な理由 |
|---|---|---|
| ファイル間の整合性 | 不合格 | 再割当の遷移、操作の対象ID、保存後の読取経路、決済・制限の関連が未整合 |
| 表現の統一 | 要修正 | 実行状態の語彙が衝突。定型文の反復が正常系と異常系の意味を崩している |
| 企業要件と提案の分離 | おおむね良好だが不十分 | SRC-06保存と出所表は有効。一方、旧整理番号を原文扱いし、音声操作の解釈を明示要求側に含めている |
| Agentic SDLC | 方針は適合、実行契約は未完成 | 役割分担・ゲートはあるが、矛盾した試験期待値、曖昧なinterface、仕様基準の固定不足が自律実行の障害 |

指摘: **P1 8件、P2 7件、P3 1件**。ここでP1は重要フロー・状態・検証を阻害する設計欠陥、P2は局所的欠陥・引き継ぎの曖昧さ、P3は改善提案。要件の実装順序P0/P1とは別。実アプリの不具合や本番事故を再現したという意味ではない。

## 2. 重要指摘（P1）

### RV-01 — 受入試験の期待結果が、正常な表示条件でも「拒否」になっている

- 根拠: [クライアント要件](../01-requirements/client.md) L168、L193、[管理者要件](../01-requirements/admin.md) AT-A11-E・AT-A13-E、[検証計画](../04-agentic-sdlc/verification.md) L13。
- AT-C06-Eの条件には「100/80kWhなら節約10MYR」、AT-C07-Eには「CO₂欠測でもPM2.5値は表示可能」とあるが、Thenは一律「不正な変更・閲覧を拒否」。正しい部分表示・算定と拒否を同じケースで要求している。
- 4役割49件すべてのEに同じThen、49件すべてのBに同じ抽象的Thenが反復されている。条件側に期待動作を書いているため、表を機械的に試験へ変換できない。172は管理上のケース数であり、確定した独立assertionの数ではない。
- 影響: テスト担当が実装を見て期待値を補うと、独立検証が崩れる。正しい欠測表示を失敗扱いする逆方向の誤判定も生じる。
- 修正: 各ケースを具体的な前提・操作・期待値へ分解し、正常な境界値、部分表示、入力拒否、権限拒否、通信失敗を分ける。例: CO₂=null、PM2.5=12なら「CO₂は未計測、PM2.5は12と単位を表示、Commandは0件」。葉ケースIDと親ATの集約条件を更新する。
- 再確認: 表示継続を要求するケースに一律拒否が残っていないこと。実装を読まなくても期待値を確定できること。

### RV-02 — 更新操作の入力に、更新対象のIDがない

- 根拠: [操作カタログ](../02-design/operation-catalog.csv) L37 `devices.updateFirmware`、L48 `jobs.addNote`、L70 `mrv.recordReview`、[技術者設計](../02-design/technician.md) DD-T11、[施工業者設計](../02-design/contractor.md) DD-P07。
- FW更新にはfirmwareVersionしかなくdeviceIdがない。案件メモにはmessage/visibilityしかなくjobIdがない。MRV確認にはreportVersionしかなくreportIdがない。共通DemoViewContextはテナント・Membership・scopeVersionであり、対象資源を特定しない。
- 影響: 同じ版の報告2件、同じFW候補を持つ機器2台を区別できない。画面の選択中IDをサービスが暗黙参照すると、画面とRepositoryの分離も崩れる。
- 修正: 各操作を対象ID付きの完全なシグネチャにする。資源に束縛したRepositoryを使うなら、その生成契約を明記する。attachments.addのjob/reportへの所属確定方法も同時に定義する。
- 再確認: 同種資源2件のうち1件だけを更新する契約試験と、対象外IDの拒否試験。

### RV-03 — 試運転の時間と終了動作を、共通Command契約に渡せない

- 根拠: [技術者設計](../02-design/technician.md) L326–346、[入出力契約](../02-design/implementation-contracts.md) L45–51、[共通設計](../02-design/common.md) L92–100。
- DD-T10はdurationMinutesとendActionを必須とするが、唯一の作成操作commands.createのCreateCommandInputには両方がない。共通設計のinterface例には、さらに診断で必須のjobId/reasonもない。
- 終了予定を共有メモリへ保持する型、開始応答を待って計時するか、画面離脱・役割切替・担当失効時に誰の権限で終了要求を生成するかも定義されていない。
- 影響: 一方の実装者はローカルタイマー、別の実装者はRepositoryのイベントとして実装し、役割切替で試運転の終了追跡が消える可能性がある。
- 修正: 試運転用ユースケースと保持モデル、開始/終了Commandの関連、計時起点、失敗・失効時の表示を定義する。通常の1Action契約と明確に接続する。
- 再確認: 試運転中の画面離脱、役割切替、開始失敗、終了応答なし、担当期限到来を固定時計で検証できる仕様にする。

### RV-04 — 保存・参照を要求する画面に、読み戻す操作が足りない

- 根拠: [管理者設計](../02-design/admin.md) DD-A05/A11/A12、[入出力契約](../02-design/implementation-contracts.md) L124・L251、[操作カタログ](../02-design/operation-catalog.csv) L43・L54・L84、[クライアント設計](../02-design/client.md) DD-C12。
- policies.saveはあるがpolicies.get/listがなく、保存済み方針を画面へ戻って編集する経路がない。
- JobDetailはreportRefsを返すが、報告本文・過去版を取得する操作または埋込projectionが定義されていない。技術者のドラフト再開、品質確認、顧客の受理済み報告閲覧に影響する。
- 顧客は問い合わせ返信を読むと規定されているが、inquiries.listの対応先はHQのDD-A08だけで、顧客側DD-C12にはcreateしかない。
- 影響: 作成直後だけ画面stateで表示できても、再訪・直接URL・役割横断では同じ情報を再構成できない。
- 修正: 読取操作を追加するか、既存戻り値への埋込を明記する。現在版/過去版、顧客公開前後、スコープ失効時のprojectionを揃える。
- 再確認: 各保存後に画面を離れて再訪し、別役割でも許可範囲の同じ情報を読めること。

### RV-05 — 作業中の再割当後のJob状態が一致しない

- 根拠: [共通設計](../02-design/common.md) L134、[施工業者要件](../01-requirements/contractor.md) FR-P03の完了後条件、[施工業者設計](../02-design/contractor.md) L127。
- 共通遷移表はassigned/in_progressの再割当で「状態維持」。個別要件・設計は再割当を含む処理の結果を「Job assigned」とする。
- 再現条件: in_progressの案件を別技術者へ再割当する。共通表ならin_progress、DD-P03をそのまま実装すればassignedになる。
- 影響: 新担当に「開始」を出すか「作業継続」を出すか、報告編集を許可するかが食い違う。
- 修正: 初回割当と再割当の事後条件を分離し、共通遷移を唯一の定義にする。元作者の保持と新担当の編集可能範囲も合わせる。
- 再確認: assigned/in_progressの各状態から再割当し、旧担当失効と新担当の操作・表示を確認する。

### RV-06 — 顧客の決済操作なしで行う手動入金確認が、契約上成立しない

- 根拠: [管理者設計](../02-design/admin.md) L282–284、L294–307、[操作カタログ](../02-design/operation-catalog.csv) L80、[入出力契約](../02-design/implementation-contracts.md) 支払い方法の補完。
- DD-A08は「手動入金確認だけの場合」を定義するが、payments.confirmは既存paymentIdを必須とする。Paymentの明示的な作成手段は顧客向けpayments.simulateで、HQでの利用は明確に禁止されている。
- 影響: Invoiceだけがある未操作請求を、HQが参照番号・全額・理由で入金済みにできない。未操作請求ごとにPaymentを自動作成する規則もない。
- 修正: 手動確認をinvoiceId起点にしてPaymentを作成する契約、または手動Paymentを作成する独立操作を定義する。カード決済の確認と二重計上防止を共有する。
- 再確認: Paymentが0件の請求への手動確認、同じ参照の再送、顧客決済との競合、方法nullの表示を検証する。

### RV-07 — 制限と原因請求の関連が未定義で、複数請求時の解除を決められない

- 根拠: [共通設計](../02-design/common.md) Restrictionモデル・支払い制限遷移、[操作カタログ](../02-design/operation-catalog.csv) restrictions.schedule/forInvoice/release、[管理者要件](../01-requirements/admin.md) FR-A09。
- scheduleの入力とRestrictionモデルはcontractIdを持つが、原因となるinvoiceId集合を持たない。一方、顧客はforInvoiceで検索し、入金で「関連Restriction」を解除する。
- 再現条件: 同じ契約に期限超過請求が2件あり、1件だけ入金する。制限がどの請求に依存するか、残債があれば継続するかが決まらない。複数請求を禁止する不変条件もない。
- 影響: 1件の入金で未払いが残る契約まで解除する実装と、全請求入金まで解除しない実装の両方が文面から成立する。
- 修正: 1A提案として、原因請求の集合と解除条件を保持するか、1契約1対象請求などの明示的制約を設ける。本番の契約条件を確定する必要はない。
- 再確認: 同契約2請求、別契約、請求処理中、全対象入金、手動解除を分ける。

### RV-08 — 制限解除で機器へ要求する内容と、解除応答の判定対象がない

- 根拠: [管理者設計](../02-design/admin.md) L329–330、[入出力契約](../02-design/implementation-contracts.md) UnitAction、[共通設計](../02-design/common.md) L151–163。
- temperature_limit/power_offの制限と、設備別の解除Command・応答待ちを定義しているが、UnitActionには制限設定/解除がない。通常のset_temperature/set_powerへ変換する場合の規則もない。
- power_off解除は再び電源ONにするのか、操作許可だけ戻すのか。温度制限解除は元温度へ戻すのか、下限だけ外すのか。解除直前の顧客操作を復元処理が上書きするかも未定義。
- 影響: 「全台応答でreleased」という状態名は揃っていても、何を確認したら解除成功なのかを実装・試験で一致させられない。
- 修正: モックの制限適用/解除actionと観測状態を定義する。通常運転設定の復元有無を提案として明記する。
- 再確認: 2種類の制限ごとに解除前後の許可・電源・設定温度・応答未到着時の期待値を固定する。

## 3. 局所的欠陥・引き継ぎ上の問題（P2）

### RV-09 — Repositoryの戻り値とエラーの方式が一意でない

- 根拠: [共通設計](../02-design/common.md) L92–100、[入出力契約](../02-design/implementation-contracts.md) L56–70およびJSON例。
- interfaceはPromise<Command>だが、入出力契約はServiceResult<T>のdata/meta包みを示す。失敗もServiceErrorResultを返すのかDomainErrorをthrowするのかが書かれていない。
- 影響: Query hookがresult.idを読む実装とresult.data.idを読む実装に分かれ、失敗結果をQueryの成功として扱うおそれがある。
- 修正・再確認: UIが呼ぶ最終interfaceで戻り値/throw方式を統一し、正常・失敗それぞれ1本の呼出例を示す。内部変換層があるなら位置を明記する。

### RV-10 — 写真URLの破棄と、共有された報告写真の再表示を両立する保存モデルがない

- 根拠: [共通設計](../02-design/common.md) L75・L85・第6節、[共通要件](../01-requirements/common.md) FR-X01、[技術者設計](../02-design/technician.md) DD-T09、[検証計画](../04-agentic-sdlc/verification.md) S02/S08。
- Attachmentの要約にpreviewUrlはあるが、元のBlob/Fileの保持先とURL再生成の責任がない。サインアウト等でURLをrevokeする一方、共有業務レコードは残す。
- 影響: 技術者が保存・提出後に役割を変えた際、品質担当が写真を開けない実装になり得る。未保存previewと提出済み写真を同じ寿命で扱ってはならない。
- 修正・再確認: 共有モック内の画像本体と、画面ごとの一時URLを分離する。提出→サインアウト→HQ再ログイン→写真閲覧、reset後の解放を仕様化する。

### RV-11 — 文書検証が旧引き継ぎ資料の整理番号を「原文」と呼んでいる

- 根拠: [検証計画](../04-agentic-sdlc/verification.md) L128、[企業英語原文](../00-prepare/sources/company-requirements-original.txt)、[旧引き継ぎ資料](../00-prepare/sources/original-handover.md) 第4–6節、[PrepareDocument](../00-prepare/PrepareDocument.md) 付録。
- 現行計画の「原文C01〜C13/T01〜T12/A01〜A16」はSRC-06に存在せず、二次資料SRC-01で付与された番号。Prepareの「企業要求の一次根拠には使用しない」と矛盾する。
- 影響: 旧資料にある「確定要件」を一次要求と誤認し、企業原文との差分を見逃す。旧資料自体を履歴として保存することは問題ではない。
- 修正・再確認: SRC-06→BIZ→FRの検証へ書き換える。旧番号は旧版対応として別管理し、現行規約で企業原文と呼ばない。

### RV-12 — 最優先の制作指示SRC-02に、照合できる原記録がない

- 根拠: [文書索引](../README.md) 文書の規約、[PrepareDocument](../00-prepare/PrepareDocument.md) L201、[開発者向け補足](../00-prepare/internal/design-assumptions.md) DEC-02/03/06。
- 優先順位の最上位に制作指示を置くが、SRC-02は名称と要約のみ。発言日時・メッセージ参照・原文抜粋などがなく、4役割化、特定ライブラリ指定、「reactForms」の解釈、ワイヤフレーム除外の根拠を次担当が独立に確認できない。
- 影響: 作成者の解釈と明示指示の境界を、記述者の自己申告だけで判定することになる。指示が存在しなかったと断定するものではない。
- 修正・再確認: 再確認可能な指示記録を追加し、明示指定/解釈/追加提案を分ける。記録がないものは根拠未収録と書き、承認を捏造しない。

### RV-13 — 音声による「応答」から「機器操作」への展開が、企業の明示要求側に入っている

- 根拠: [企業英語原文](../00-prepare/sources/company-requirements-original.txt) L5、[PrepareDocument](../00-prepare/PrepareDocument.md) L61、[企業要望対応表](../00-prepare/company-requirement-map.csv) BIZ-03、[共通要件](../01-requirements/common.md) FR-X02。
- 原文は音声AIへの応答を述べるが、音声で温度変更することまでは明記していない。BIZ-03の「企業からの要望」はすでに「応答・操作」となり、補完欄も操作機能の追加自体を明示していない。
- 影響: 合理的な製品提案が企業指定として扱われる。原文との追跡があっても、意味を追加した箇所の識別ができない。
- 修正・再確認: 原文の最小主張は音声応答、状態照会・機器変更・確認フローは設計解釈/提案として分離する。機能削除は不要。

### RV-14 — Agentic SDLCのタスク状態と試験結果の語彙が衝突する

- 根拠: [SDLC規約](../04-agentic-sdlc/README.md) L53、[成果物テンプレート](../04-agentic-sdlc/templates/artifacts.md) L3・L10、[テスト担当](../04-agentic-sdlc/agents/test.md) L63。
- 規約のtask状態はready/in_progress/in_review/done/blocked、共通テンプレートはplanned/in_progress/passed/failed/blocked/not_run。試験ではpassも使用する。状態の対象ごとのschemaや変換規則がない。
- 影響: task packetをテンプレートどおり作ると規約の状態遷移に乗らない。passedとdone、not_runとreadyの扱いをオーケストレーターが推測することになる。
- 修正・再確認: task_status、test_result、gate_result、decision_statusを分離し、許容値・遷移・完了条件を揃える。テンプレート例を規約に照合する。

### RV-15 — 仕様変更で失効する試験・ゲートを、成果物契約で特定できない

- 根拠: [成果物テンプレート](../04-agentic-sdlc/templates/artifacts.md) L9・L34・L55、[SDLC規約](../04-agentic-sdlc/README.md) L80、[文書索引](../README.md) 変更管理。
- 実装revision/差分ハッシュへの紐付けは良い。一方、テスト記録に仕様baselineや受入条件のハッシュがなく、task入力のdocument_versionだけに依存する。仕様更新時にどの既存pass/G1/G3を再判定へ戻すかも明記されていない。
- 影響: 同じ実装に対する旧期待値のpassを、新しい仕様の証跡として再利用できてしまう。共通仕様の変更が既完了タスクへ及ぶ範囲を追跡しづらい。
- 修正・再確認: 読み込んだ仕様のcommitまたはファイルハッシュを記録し、変更ID→依存タスク/AT/ゲートの失効規則を加える。仕様だけ変更した場合の再判定手順を例示する。

## 4. 表現と保守性（P3）

### RV-16 — 定型文の重複と命名のばらつきが、意味上の違いを隠している

- 根拠: 4役割の要件・設計全体、[クライアント設計](../02-design/client.md) DD-C06/C13、[管理者設計](../02-design/admin.md) DD-A15、[施工業者設計](../02-design/contractor.md) DD-P07。
- 例: 読取画面にも「保存または操作直前」「フォーム必須境界試験」が反復される。正常な省エネ算定例が「競合・失敗時」に置かれる。「表示の表示・処理設計」が残る。
- API用語としてもAction/UnitAction、space.parentId/parentSpaceId、RTO/rto、channelのemail/email_preview、demo/DEMO-が混在する。役割名称のクライアント/顧客、管理者/HQは同義定義があり、それ自体は欠陥に数えない。
- 修正: 共通の監査・失敗・検証定型文を共通契約へ寄せ、各DDには差分と参照IDを残す。状態値・入力型は正規名を1つにし、表示ラベルとの変換を記載する。
- 再確認: 文体の好みではなく、同じ単語が同じ型・状態・操作を指すかで判定する。

## 5. Agentic SDLCの設計思想について

評価の中心は、このリポジトリ自身が定義したG0–G5、独立検証、版付き引き継ぎ、限定的な人へのエスカレーションである。補助的に、明確な成功条件、環境からの実結果、評価と修正の反復を重視する[Anthropicの設計指針](https://www.anthropic.com/engineering/building-effective-agents)、仕様・計画・タスク・実装の整合確認を行う[GitHub Spec KitのAgentic SDD](https://github.github.com/spec-kit/reference/agentic-sdd.html)を参照した。特定フレームワークの導入を合格条件にはしていない。

**適合している点**

- オーケストレーション、設計、実装、テスト、レビューの責務が分かれている。
- 実装者による要件改変での試験通過を禁止し、テスト・レビューの独立性を明記している。
- 共有schemaの単一owner、固定seed・時計、相関ID、実装版付き証跡を要求している。
- 可逆的作業を都度人待ちにせず、業務判断や外部実行だけを限定して戻す。
- 未実行・未接続・未承認を明記し、架空の実行記録を作らない。

**不足している点**

- 自律実行に必要な「同じ入力なら同じ解釈になる契約」が未完成（RV-02〜10）。
- 独立評価の基準となるThenが一部矛盾し、評価者の推測に依存する（RV-01）。
- 入力資料の優先順位はあるが、その原記録と失効管理が弱い（RV-11〜15）。

したがって「Agentic SDLCに反する設計」と一括評価するのは不正確だが、**思想を説明する文書から、エージェントが再現可能に実行できる仕様へ進むための修正が必要**である。エージェント・アプリ・バックエンドが未実装であること自体は、今回の文書レビューの欠陥に数えていない。

## 6. ファイル別の確認記録

以下は32ファイルすべての確認先。指摘なしは、そのファイル単独で追加の具体的欠陥を採番しなかった意味で、全システムの完全性保証ではない。

| ファイル（docs/以下） | 評価・関連指摘 |
|---|---|
| README.md | 範囲・優先順位・未承認の明記は良い。RV-12/15 |
| 00-prepare/PrepareDocument.md | 企業向けと開発者向けを分離。音声要求と制作指示証拠にRV-12/13 |
| 00-prepare/internal/design-assumptions.md | PROPOSEDと制作方針の区別は良い。DEC-09が広範で、個別補完の追跡は各DDに依存。RV-12 |
| 00-prepare/reference-design-analysis.md | ソース確認と描画未検証を分離。追加の必須欠陥なし |
| 00-prepare/sources/reference-style-evidence.json | JSON構文は正常。抽出値・取得物ハッシュあり。元CSS/HTML不在のため、取得物ハッシュの再計算は今回未実施 |
| 00-prepare/sources/company-requirements-original.txt | 原文として保全。編集提案なし。BIZの意味追加を照合 |
| 00-prepare/sources/original-handover.md | 旧3役割・旧仮定を含む履歴資料。現行仕様との差は履歴として許容。RV-11は現行側の参照方法の問題 |
| 00-prepare/company-requirement-map.csv | 26項目、逆引き一致。音声の出所分類にRV-13 |
| 00-prepare/requirement-origins.csv | 64要件、追跡表と一致。RV-12/13の根拠粒度に注意 |
| 00-prepare/traceability.csv | ID・件数・ファイル対応は一致。RV-01/15。リンクの存在は意味上の充足を示さない |
| 01-requirements/common.md | 不変条件・認可境界は良い。RV-10/13、個別契約との照合が必要 |
| 01-requirements/client.md | RV-01/07。欠測・正常表示のATを具体化する |
| 01-requirements/contractor.md | 追加役割を制作方針と明記。RV-01/05 |
| 01-requirements/technician.md | 点検対象・推定区分は具体的。RV-01/03/10 |
| 01-requirements/admin.md | RV-01/06/07/08。複数請求・解除の期待値を補う |
| 02-design/common.md | 共通化の方針は良い。RV-03/05/07/08/09/10 |
| 02-design/client.md | RV-04/07/16。再訪時の取得と問い合わせ返信を補う |
| 02-design/contractor.md | RV-02/04/05/16。再割当とメモ対象を揃える |
| 02-design/technician.md | RV-02/03/04/10。試運転と報告・添付の完全な契約が必要 |
| 02-design/admin.md | RV-02/04/06/07/08/16。設定・報告の読取と入金・解除を補う |
| 02-design/implementation-contracts.md | RV-02/03/04/06/07/08/09。契約名だけでなく完全な引数・戻り値・遷移を定義する |
| 02-design/operation-catalog.csv | 109操作、キー重複なし。RV-02/03/04/06/07/08/09。参照名の網羅と契約の完備は別 |
| 03-uiux/UIUXSpecification.md | REF/ADAPT、State責務、a11y未検証の表記は良い。RV-16の型・ラベル整理と接続 |
| 04-agentic-sdlc/README.md | 自律進行・独立評価・限定的な人への判断依頼は良い。RV-14/15 |
| 04-agentic-sdlc/agents/orchestration.md | 依存・所有者・証跡管理は妥当。RV-14/15を共通契約から適用する |
| 04-agentic-sdlc/agents/design.md | 出所・変更影響の扱いは妥当。RV-01〜13の修正担当となる |
| 04-agentic-sdlc/agents/implementation.md | 実装範囲・責務は妥当。不足契約を実装者が独自補完しない運用が必要 |
| 04-agentic-sdlc/agents/test.md | 独立した期待値と未実行の表記は良い。RV-01/14/15 |
| 04-agentic-sdlc/agents/review.md | 根拠付き指摘と自己レビュー制限は妥当。今回の意味上の欠陥をG1で検出する必要がある |
| 04-agentic-sdlc/templates/artifacts.md | RV-14/15。状態schemaと仕様baselineの情報を追加する |
| 04-agentic-sdlc/verification.md | 172件の集計、横断試験の分離は一致。RV-01/11/15 |
| 05-document-review/consistency-review.md | 既存の修正履歴として保持。件数の整合は再確認できたが、契約・遷移の完全性の証拠としては不足。本書の指摘が未解決 |

## 7. 今回実施した確認と限界

実施済みの文書確認:

- 全32ファイルを対象に原文・出所・要件・設計・UI・SDLC・既存レビューを照合。
- Python標準csvによる4表の読取: 26/64/64/109行、キー重複・欠損列なし。
- BIZ→FRとFR→BIZの関連集合を双方向比較: 差分0。
- 出所表と追跡表のorigin/source、company_item_ids、design_supplementを比較: 差分0。
- 追跡表の受入葉ID数: 172。参照した受入IDの本文/検証計画内の存在を確認。
- Markdownの相対リンク先ファイルとCSVの参照ファイルの存在を確認: 欠落0。
- 49件のE、49件のBで定型Thenが反復されることを文字列集計で確認。

今回はアプリ未実装のため、型検査・ビルド・E2E・実画面・スクリーンリーダー試験は実施していない。CSS取得物のハッシュ再照合、参照サイトとのピクセル比較、企業の業務承認も行っていない。上記リンク確認はファイル到達を対象とし、全見出しアンカーの機械的検査まで実施したという主張ではない。

## 8. 修正の順序と再レビュー条件

1. RV-05/06/07/08の業務状態・対象の関係を、1Aの設計提案として決める。
2. RV-02/03/04/09/10の共通interface・モデル・読み戻しを具体化し、各DDと操作カタログを同じ変更単位で更新する。
3. RV-01のATを独立した期待値へ分解し、追跡表と件数を更新する。
4. RV-11/12/13の出所を補正し、企業原文・制作指示・解釈・追加提案を分ける。
5. RV-14/15の状態schema・仕様baseline・再判定規則を統一し、RV-16の重複表現を整理する。

再レビューでは「該当IDを追える」だけでなく、**対象資源2件・再訪・役割切替・失敗・境界条件を与えて、設計担当と試験担当が同じ期待結果を導けること**をG1合格の条件とする。無関係な文書整備や可逆的な設計作業まで停止する必要はない。
