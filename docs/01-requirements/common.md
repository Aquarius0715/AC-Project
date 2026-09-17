---
document_id: REQ-COMMON
version: 0.21.0
status: draft
owner: design-agent
scope: frontend-demo-1A
---

# 共通要件定義書

上位の文書: [PrepareDocument](../00-prepare/PrepareDocument.md)。この文書は、4つの役割それぞれの要件定義書とあわせて読んでください。内容の出どころは、企業原文・制作方針・設計チームの補足に区分します。

この文書は、[企業要件の英語原文（SRC-06）](../00-prepare/sources/company-requirements-original.txt)をもとに、内容を整理し直したものです。この英語原文が、いちばん元になる資料(一次資料)です。情報は次の順番でたどれます。企業原文 → BIZ整理項目(企業の要望を分類した項目) → この文書のFR(機能要件) → 詳細設計・受入条件。各機能について、「企業からの要望」と「設計チームが補った部分」を分けて書きます。画面の項目、入力の制約、状態の変化、優先度は、フロントエンド(画面側)の実装案として示すものです。これらは、企業から詳しく承認をもらった内容ではありません。参考として示すモック画面は、見た目のデザインを考えるための参考資料です。機能要件と受入条件は、企業原文の目的に、制作方針と設計チームの補足を加えて、具体的にしたものです。

**0.21.0の実装基準**: [確定契約](../02-design/deterministic-contracts.md) 全章およびstrict-review-contracts.md全章、操作カタログの認可列、画面カタログを併読する。数値・権限・非同期・復旧を実装時に推測しない。デモの設計提案であり本番の業務承認ではない。

## 共通機能

| ID | 優先 | 状態 | 要件 | 受入条件（AT-X番号） |
|---|---|---|---|---|
| FR-X01 | P0 | 企業原文 SRC-06＋設計補完 / BIZ-01, BIZ-02 | サインイン・サインアウト、パスワードの再設定、デモ役割の選択、表示言語の選択 | 4つの役割へ切り替えられる。サインアウトすると、画面の表示とキャッシュ(一時保存データ)が消える。パスワード再設定は、宛先が実在するかどうかを漏らさない共通の文言を表示する。言語を切り替えると、主な画面・通知・日時の表示が変わる。認証はデモ用で、見た目だけを再現する |
| FR-X02 | P1 | 企業原文 SRC-06＋設計補完 / BIZ-03 | 選んだ言語で、音声による状態確認・操作・ヘルプ(操作説明)ができる。文字入力でも同じことができる | 温度を確認・変更するデモが動く。変更する前に、対象と内容を確認し、通常の操作と同じ権限チェックを行う。音声が使えない場合や拒否された場合も、文字入力で操作を完了できる |
| FR-X03 | P0 | 企業原文 SRC-06＋設計補完 / BIZ-09, BIZ-18 | 重要度、実測値・推定値・点検結果、データの品質、単位を区別して表示する | 緊急は赤、注意はオレンジ、正常は緑で表示し、色だけでなく文字やアイコンもあわせて使う。未計測・古いデータ・通信が切れている場合は「不明」と表示する。ppm(体積比の単位)とkgCO₂e(二酸化炭素換算量)を混同しない |
| FR-X04 | P0 | 設計補完(企業目的に対応) / BIZ-04 | テナント、役割、担当範囲、期間、能力に応じて、見られる・操作できる内容を制限する | メニュー、URLの直接入力、サービス呼び出しのどれを使っても、担当範囲の外にあるデータは取得できない。役割を切り替えたときに、前の役割で見ていた画面のキャッシュが表示されない。API側でも、あらためて権限を確認する必要があることを明記する |
| FR-X05 | P0 | 企業原文 SRC-06＋設計補完 / BIZ-05 | 共有のデモデータ、データのリセット、実際の処理との違いをはっきりさせる | 同じタブで役割を切り替えても、同じ案件・請求・設備が表示される。リセットすると同じseed(初期データのもと)に戻る。実際のカード情報は入力させず、外部への送信・実際の機器の制御・実際の取引は行わない |
| FR-X06 | P0 | 企業原文 SRC-06＋設計補完 / BIZ-05, BIZ-06, BIZ-12, BIZ-19 | 対応している機能と、非RTO(RTO契約以外)の扱いを区別する | 型番が対応していない操作は、理由とともに選べなくする。一般保守の設備は、契約による請求がなくても監視・操作できる。送風を「換気」として表示しない |
| FR-X07 | P0 | 企業原文 SRC-06＋設計補完 / BIZ-08, BIZ-20, BIZ-22 | 通知・履歴・監査の仕組みを用意する | 異常・案件・制限に関連する画面へ移動できる。通知を既読にすることと、異常が解消したことは別に扱う。変更した記録には、実行した人、時刻、対象、理由、結果、相関ID(関連する記録をまとめる番号)を残す |

## 非機能要件

NFR-01〜05・07は、企業が求める「クリックして操作できる画面(UI)」という目的(BIZ-05)を支えるための品質の提案です。NFR-06は、将来API(外部システムとのつなぎ込み)に接続することを見据えた制作方針(SRC-02)です。NFR-08は、企業からの多言語対応の要望(BIZ-02)を具体化した設計提案です。以下に出てくる数値、テストの方法、最初に表示する言語は、企業原文で指定された値ではありません。

| 要件ID / 受入ID | 優先 | 要件と測定方法 | 根拠・扱い |
|---|---|---|---|
| NFR-01 / AT-NFR01 | P0 | キーボードだけで主な操作を最後まで行える。今どこを選んでいるか(フォーカス)が見える。ラベルとエラーメッセージが結びついている。色だけに頼らず区別できる。WCAG 2.2 AA(アクセシビリティの国際基準)を目標にして、自動チェックと手動での操作確認を行う | アクセシビリティの基準はPROPOSED(提案段階)であり、基準に適合済みとは主張しない |
| NFR-02 / AT-NFR02 | P0 | 画面の幅が360/768/1024/1279/1280/1440 CSSピクセルのとき、主な操作とナビゲーションの見た目を確認する。本文の文字を200%に拡大しても操作ができなくならない。データを表で見せる画面以外は、ページが横にスクロールしない | レスポンシブ(画面幅に応じた表示)の目標はPROPOSED(提案段階) |
| NFR-03 / AT-NFR03 | P0 | 架空のデータだけを使う。機密情報をログ・URL・保存領域に出さない。危険な操作の前には確認を行い、拒否されるケースと、期限切れのときの再確認(確認中にセッション期限・権限・対象versionが変わったら送信せず再確認する、IR74)も用意する | 本番向けのセキュリティ実装の代わりにはならない |
| NFR-04 / AT-NFR04 | P1 | 本番向けビルド(production build)、100台の設備・1000件のサンプル(選んだ系列)、同じ端末・同じブラウザの条件で計測する。主な操作へのすぐの反応は200ミリ秒以内、デモの一覧表示は2秒以内を目標にする | あくまで暫定の、開発環境での目標。わざと遅らせている部分は別に計測する。ネットワークの品質保証(SLA)ではない |
| NFR-05 / AT-NFR05 | P0 | 読み込み中・空・エラー・オフライン・権限なし・見つからない・古いデータ、それぞれの状態を表示する。表示中のデータを取り直している間は、表示を消さずに「更新中」と示す(IR83)。もう一度試したときに二重に登録されない。まだ保存していない入力内容は保持する | 画面ごとに用意した異常時のデータ(fixture)で確認する |
| NFR-06 / AT-NFR06 | P0 | 画面(UI)は、非同期のインターフェース(仕組み)を通して、モックのサービスを呼び出す。画面の見た目とデータ取得の仕組みを分けておき、将来、実際のシステムに差し替えられるようにする | 今回はフロントエンドのみを扱う。API仕様、HTTPでのつなぎ込み、データベース、サーバーの設計は対象外 |
| NFR-07 / AT-NFR07 | P0 | 型のチェック、コードの静的チェック(lint)、ビルド、画面遷移や権限判定の単体テスト、フォーム部品のテスト、S01〜S08のE2Eテスト(画面を通した一連の動作テスト)を、実装したあとに実行する。実行しない場合は理由を明記する | 実行するコマンドは、実装時のpackage scripts(実行設定)を正式なものとする |
| NFR-08 / AT-NFR08 | P1 | 翻訳キーとIntl(国際化の仕組み)を使って、言語・時刻・通貨・単位を扱う。時刻はUTC(世界標準時)で保存し、表示するタイムゾーンをわかるようにする。タイムゾーンを切り替えても、予約した時刻の意味が変わらないようにする | デモで対応する言語は英語(初期選択)・マレー語。実際にどの市場に出すかはまだ決まっていない |

各AT-NFRの判定は、上の表の測定方法と、[検証計画の観測点](../04-agentic-sdlc/verification.md#11-共通at非機能atの観測ポイント)をあわせて使います。

## 権限マトリクス

「対象内」とは、tenantId(テナントID)が一致していることに加えて、顧客としての所属、委託案件、担当範囲・有効期間のいずれも一致していることを指します。HQであっても、すべてのテナントを無制限に見られるわけではありません。

| 操作 | クライアント | 施工業者 | 技術者（社内/外部） | HQ |
|---|---|---|---|---|
| 設備・場所の閲覧 | 自分が使う範囲 | 受託した案件に必要な情報 | 社内: 担当範囲 / 外部: 割り当てられた設備・期間 | 管理する範囲 |
| 物件・場所の作成編集 | 自分の組織内 | 不可 | 不可 | 管理する範囲 |
| 通常の空調操作 | 対象範囲内・能力の範囲内 | 不可（commands.create／voice.resolveIntentを施工業者に付与しない） | 診断の権限があり、対象・期間内であること | control.execute権限を持つ人 |
| 保守依頼 | 自分の設備 | 受託した案件の調整 | 担当する案件への対応 | 管理する範囲で作成・割り当て |
| 業者の受諾・自社割り当て | 不可 | 受託した案件で、自社かつ資格・期間内であること | 不可 | 委託・社内への割り当て |
| 点検・作業報告 | 結果を閲覧 | 閲覧・品質確認・差し戻し | 担当する案件で作成 | 閲覧・品質確認 |
| 請求の閲覧・支払いデモ | 自分の契約分 | 不可 | 不可 | billing.manage権限を持つ人が管理 |
| 制限・猶予・解除 | 理由の閲覧 | 不可 | 不可 | restriction.manage権限。override(強制解除)は別の権限が必要 |
| IoTの登録・校正・FW更新 | 状態の閲覧 | 受託した設備の状態の閲覧 | device.maintain権限を持ち、担当範囲内であること | device.manage権限を持つ人 |
| MRV・監査 | 自分向けの推定値・履歴 | 案件に必要な履歴のみ | 担当する操作の履歴 | mrv.manage権限・audit.read権限を持つ人 |
| ユーザー・役割の変更 | 不可 | 自社の割り当て候補を閲覧するのみ | 不可 | identity.manage権限を持つ人 |

制限中に許可される空調操作はIR46の表、機器の接続・電源信号による拒否はIR47で判定します。閲覧できないデータについては、そのデータが存在するかどうかも含めて詳しい情報を漏らしません。操作ができない理由は、そのデータを閲覧すること自体が許されている場合にだけ表示します。施工業者に、技術者の操作権限を自動では与えません。1人の業務責任者が複数の役割を持つ場合も、どの役割で操作しているかがはっきりわかるように、Membership(所属)を切り替えます。

## 用語の正規名

本文の略称は次の正規名の別名です。設備＝ACUnit(エアコン本体、`unitId`)、機器＝Device(空調内に設置するIoT機器、`deviceId`)、案件＝MaintenanceJob(顧客の保守依頼、`jobId`)、委託の申し出＝Offer(HQから施工業者への依頼、`offerId`)、割当＝Assignment、問い合わせ＝Inquiry(顧客からHQへの請求・制限に関する問い合わせ)、通知＝Notification、異常＝Alert。「依頼」はJobを指し、HQから業者への依頼はOfferと書き分けます。役割識別子はDDC-08 §5を正とします。

## 業務不変条件

- 室温と設定温度、要求した値と機器が確認した値は、それぞれ別のものとして扱う。完了したと判定するには、デモであっても機器からの応答イベントが必要。
- 作業が完了したことと、異常が解消したことは別のことである。異常の解消には、再測定するか、記録を残した確認操作が必要。
- 支払いの操作を行ったことと、入金が確認できたことは別のことである。制限を解除するには、入金の確認のあとに解除を要求し、適用済み機器の解除応答または未配送/未適用確定の証跡を経る必要がある（D03）。
- 制限、診断、FW(基本ソフト)の更新、音声設定の変更は、能力・権限・担当範囲の確認を、共通の仕組みで行う。
- 室内のCO₂、粉じん、電力の排出量について、データが欠けている場合に0として扱わない。省エネの割合を保証せず、排出量をクレジットとして発行することもしない。
- デモで使うしきい値や契約のルールは、いずれも仮の値であり、医学的・法的・電気工事上の正式な判断を示すものではない。

## 共通機能の具体的な業務条件（0.6.0）

### FR-X01 セッション・言語

- **企業要望の根拠**: SRC-06 BIZ-01, BIZ-02 — サインイン、サインアウト、パスワードの再設定。好きな言語を選んで表示できること。
- **設計補完の範囲**: 4役割のデモ、最初の表示言語(英語を初期選択・マレー語も選択可能)、再設定時の文言。

デモのログインは、あらかじめ用意された架空のアカウント一覧から選びます。セッションの30分寿命はデモ時計で測りますが、`demo.advanceClock`の早送りは寿命を消費しません(IR36)。期限の120秒前に延長の確認を表示し、延長を選ぶとその時点から30分延びます(IR55)。これにより、4つの役割と、社内・外部の技術者のMembership(所属)に入ることができます。本人の実際のメールアドレスやパスワードを入力する必要はありません。ログインしていない状態で、保護されたroute(画面)を開こうとした場合は、`/login?returnTo=…`へ移動します。ログイン後に戻る先は、許可された相対route(画面)だけを保持し、不正な値は無視します(IR82)。サインアウトしたときや役割を切り替えたときは、古い画面の内容、検索条件(Query)、画像のプレビュー、権限に依存する下書き(draft)を消します。

パスワード再設定のデモでは、メールアドレスの形式だけを確認します。実際に登録されているかどうかに関わらず、同じ完了メッセージと送信プレビューを返します。言語を切り替えると、英語・マレー語の表示に反映されますが、保存されているデータのUTC(世界標準時)・単位・IDは変わりません。AT-X01では、`demoSession.signIn/signOut/switchMembership`と`auth.previewPasswordReset`、`preferences.update`(locale、表示言語の設定)を対象に、次のことを確認します。保護されたrouteを直接開いた場合の動き、ログアウト後に「戻る」を押した場合の動き、役割を切り替えている途中に届いた古い応答の扱い、登録されていない形式のデモメールでも実在するかどうかを漏らさないこと、の4つです。

### FR-X02 音声・問い合わせ

- **企業要望の根拠**: SRC-06 BIZ-03 — 音声AIで応答できること。
- **設計補完の範囲**: 音声での応答を、温度の確認・機器の操作・問い合わせに広げる案、音声のデモと文字入力による代わりの手段、変更前の確認。音声による機器の変更そのものは、企業の原文にははっきり書かれていません。

対応するintent(操作の意図)は、温度の確認、設定温度の変更、ヘルプ(操作説明の読取。FR-C12の問い合わせInquiryは生成しない)の3つです。音声・テキストの操作権限がない役割(施工業者)にはパネルを表示しません(IR44)。部屋の名前が複数一致する場合は、コンテナ名や階などの付加情報をあわせて候補に示し、対象を絞り込みます。それでも区別がつかない場合は、文字入力に切り替えて対象を指定してもらいます。いずれの場合も、システムが推測して実行することはありません。認識できない意図に対しては、「操作できません」と表示し、文字入力に戻します。音声による変更の要求は、画面で対象・変更前の値・変更後の値を示してから、Command(操作要求)として送ります。確認する前は、業務上の変更は0件です。

AT-X02では、同じ名前の部屋が2件ある場合(コンテナ名や階を示しても区別できず、文字入力に切り替える場合を含む)、権限のない部屋を指定した場合、対応していない温度を指定した場合、マイクの使用を拒否した場合、確認を取り消した場合、をそれぞれ確認します。位置情報やマイクの利用への同意を取り消した場合は、今出そうとしていた要求を破棄しますが、過去にすでに行われた機器への応答を取り消したことにはしません。実際の音声サービスへは接続しません。

### FR-X03 状態と品質

- **企業要望の根拠**: SRC-06 BIZ-09, BIZ-18 — 赤・オレンジ・緑の色で知らせることと、通知。CO₂濃度、粉じん、湿度、アレルゲンなどを把握し、清掃・換気の案内を行う。
- **設計補完の範囲**: 色の意味、単位、データの品質、色以外の表現方法。

重要度、接続の状態、運転の状態、業務の進み具合、データの品質は、それぞれ別の軸として表示します。機器がオフラインになっていても、最後にわかっていた電源の状態(last-known)を表示できますが、それを「今も稼働中」と断定はしません。測定値が欠けている場合に0へ置き換えることはせず、値・更新した時刻・データの品質をセットで表示します。AT-X03では、同じ設備について「通信が切れている」「以前は電源ONだった」「まだ解消していないwarning(警告)がある」という3つを同時に表示できること、そしてppm(体積比の単位)とkgCO₂e(二酸化炭素換算量)が別の指標(metric)として扱われることを確認します。

### FR-X04 スコープと操作権限

- **企業要望の根拠**: SRC-06 BIZ-04 — 顧客、社内・外部の技術者、管理者・HQ向けに、見てわかるダッシュボードを用意する。
- **設計補完の範囲**: 役割・担当・期間・能力による表示の制限。

操作を許可する条件は、次のすべてを満たすことです。有効なセッションであること、テナントが一致していること、role(役割)やpermission(権限)で許可されていること、対象データのscope(担当範囲)が一致していること、有効期間内であること、そして今の状態・能力の条件を満たしていること。閲覧できる範囲(projection)も、これらの条件で決めます。クライアント・業者・外部技術者は、それぞれ違う境界で判定します。権限の確認は、メニューを表示するときだけでなく、そのつど行います。

AT-X04では、次のケースをそれぞれ「拒否されること」として確認します。別のテナント、同じテナントの別の顧客、別の業者、資格のない技術者、期限がちょうど一致するタイミング、権限を変更したあとに開いたままの古い画面、同じ人がMembership(所属)を切り替えて自分自身を承認しようとする場合、の7つです。これはフロントエンドでのデモ表示・操作制御を確認するものであり、本番の認可の仕組みの設計・実装は対象外です。

### FR-X05 デモと共有データ

- **企業要望の根拠**: SRC-06 BIZ-05 — まず分離型エアコンを先に扱い、最初にクリックして操作できるUI/UXを作り、そのあとで全体のソリューションへ広げる。
- **設計補完の範囲**: 共有するモックデータ、リセット、実際の処理との違い。

seed(初期データのもと)は固定し、同じタブで4つの役割が、同じIDの業務データを読み込みます。役割を切り替えても業務データはリセットされません。画面のリロードや、明示的なリセット操作を行ったときだけ、初期値に戻ります。リセットする前に発生していた遅延イベントが、新しいseedに紛れ込むことはありません。状態を操作するためのパネルは、業務用の画面とは見た目を分け、常に「デモ」であることを表示します。AT-X05では、`demo.reset`・`demo.trigger`・`demoSession.switchMembership`を対象に、seedが再現されること、役割の間でデータの整合性が保たれること、古い遅延イベントが破棄されること、決済・通知・IoT・取引への実際の接続が0件であることを確認します。

### FR-X06 能力と契約種別

- **企業要望の根拠**: SRC-06 BIZ-05, BIZ-06, BIZ-12, BIZ-19 — まず分離型エアコンを先に扱い、最初にクリックして操作できるUI/UXを作り、そのあとで全体のソリューションへ広げる。HVAC(空調設備全般)を第2段階の対象とし、メーカーや分離型・中央空調・カセット型などへ対応を広げる。定期・事後・予防の保全を行い、RTO以外の一般保守にも利用する。CO₂が上昇したときに外気を取り入れるなどして、空気環境を改善する。
- **設計補完の範囲**: 機種の能力と、画面をいつ有効にするかの判定。

Capability(能力)は型番のバージョンに結びつけ、確認できていない能力は「対応している」とは扱いません。一般保守の設備は、RTOによる請求がなくても操作できます。制限できるかどうかは、契約が「RTO」という名前かどうかではなく、契約のrestrictionEligible(制限できるかどうかの項目)で確認します。AT-X06では、温度に対応していない場合、モードの候補が異なる場合、送風のみに対応している場合、一般保守の場合、制限できないRTOの場合、という5種類を用意します。

### FR-X07 通知・監査

- **企業要望の根拠**: SRC-06 BIZ-08, BIZ-20, BIZ-22 — 異常を事前に、または発生した時点で把握し、すぐに通知する。小型で低価格な、空調内に設置する機器やFW(基本ソフト)、取り外しや盗難への対策と通知。WhatsAppやメールと、カード決済・支払い手順への案内を用意する。
- **設計補完の範囲**: 既読の扱い、関連画面への導線、操作履歴。

通知の送り先は、発生したイベントと、今の業務上の担当範囲(scope)から選びます。顧客向けには、品質の確認が済んだ報告だけを公開し、社内での差し戻しのメモは表示しません。監査記録では、当時の実行主体・役割・対象・理由・結果・バージョン・相関ID(関連する記録をまとめる番号)を保持します。AT-X07では、通知を既読にしたあとも異常が残っていること、報告が受理される前と後で公開される内容が違うこと、サインアウトしたあとも過去の実行主体(actor)の記録が変わらないことを確認します。

### 共通受入条件の具体値

AT-X01〜X07のGiven・When・Thenは次の表を正とする（IR101）。書かれていない前提はIR92のとおりdemoSeedのままで、時計は2026-09-14T01:00:00.000Z、simulator=false（IR97の4）。

| ID | Given / When | Then |
|---|---|---|
| AT-X01-N | 未認証で`/customer/units/unit-online-rto`を開く→customer-aでdemoSession.signIn→preferences.update(locale=ms、timezone=Asia/Kuala_Lumpur) | ①`/login?returnTo=%2Fcustomer%2Funits%2Funit-online-rto`へ移る ②signIn後にreturnTo先を表示（IR82） ③表示がマレー語になり、保存値のUTC時刻・単位・IDは変わらない |
| AT-X01-E | ①customer-aでsignOut後にブラウザの「戻る」 ②demo.trigger(transport、operation=units.list、outcome=DELAY、delayMs=3000、retryAfterSeconds=null、remainingCalls=1)の遅延中にswitchMembership(hq-operator) ③auth.previewPasswordResetにa@example.comとb@example.com ④demoEmail=not-an-email ⑤returnTo=https://example.com/x でsignIn | ①/loginを表示し、旧画面の値を表示しない ②遅延した応答は旧viewEpochのため破棄し、HQ画面に顧客の一覧を表示しない（IR17） ③2件とも同じ`{messageKey:auth.reset_generic, deliveryState:preview}` ④VALIDATION ⑤returnToを無視してrole homeへ（IR82） |
| AT-X01-B | customer-aのSessionで ①expiresAtの120秒前に到達 ②延長せずdemo.trigger(session_expired) ③未確認の音声changeを表示中にlocaleをmsへ変更 | ①延長の確認を表示（IR55） ②画面・Query・未保存draftを破棄して/loginへ（D09） ③未確認intentを破棄し、入力文は保持（D09） |
| AT-X02-N | customer-aで「set Bedroom to 24 degrees」→確認 | ①voice.resolveIntentはchange（unitId=unit-online-rto、celsius=24、before=26）。property-home-bの同名Spaceはscope外で一致0件（IR65） ②確認前のCommand 0件 ③確認でcommands.createが1件（jobIdなし、expectedUnitVersion=7、IR09） |
| AT-X02-E | ①customer-aで「set Bedroom to 31 degrees」→確認 ②customer-bで「set Living room to 24 degrees」 ③demo.trigger(microphone_denied) ④changeの確認を取り消す | ①commands.createがVALIDATION（Capability.temperatureは16〜30、D01順位7の機種候補）、Command 0件 ②scope外のSpaceは一致0件でunsupported、Command 0件 ③文字入力へ切り替え、Command 0件（D09） ④write 0件（IR09） |
| AT-X02-B | `acceptancePatches["AT-X02-B"]`（unit-non-rtoをroom-1と同じ「Home A > 1F > Bedroom」に移し、表示名も「Bedroom AC」にする）。customer-aで ①「temperature Bedroom」 ②候補からunit-non-rtoを選ぶ ③selectedUnitId=unit-limitedを送る | ①candidates 2件。pathLabelが同じため各候補にunitIdを併記し、文字入力で選ばせる（IR101） ②temperature（unitId=unit-non-rto） ③NOT_FOUND（IR65） |
| AT-X03-N | unit-online-rtoにdemo.trigger(device、deviceId=device-online-rto、bindingId=binding-online-rto、kind=communication_lost、sequence=2) | ①同じ設備に「オフライン」「最後に確認した電源: ON（2026-09-14T00:59:30Z）」「warning未解消（alert-window-a）」を別々の表示で同時に出し、稼働中と断定しない ②CO₂（ppm）と排出量（kgCO₂e）を別の指標・単位として表示する |
| AT-X03-E | unit-online-rtoへdemo.trigger(telemetry)で ①humidity value=null ②power unit=W、value=1000 ③時計を2026-09-14T01:02:31Zへ進める（最新temperatureのobservedAtから181秒） | ①「未計測」と表示し0にしない ②suspect（unit_mismatch）で集計・制御から除く（IR12） ③staleとして最後の値・時刻・品質を表示（D07） |
| AT-X03-B | unit-online-rtoへdemo.trigger(telemetry)で ①humidity=0 ②co2=10000 ③co2=10001 | ①「0%」 ②valid ③suspect（D07の範囲外） |
| AT-X04-N | ①customer-aでunit-online-rto ②contractor-aでjob-contractor-a ③tech-external-aでjob-contractor-a ④hq-operatorで/admin | 各画面をsuccessで表示する |
| AT-X04-E | ①customer-aで`/customer/units/unit-tenant-b`（別テナント） ②customer-aで`/customer/units/unit-other-customer`（同じテナントの別顧客） ③contractor-bでjobs.get(job-contractor-a)（別業者） ④tech-external-aでjob-contractor-aを表示中にdemo.trigger(qualification_revoked、demo_indoor)→表示中の画面から開始 ⑤`acceptancePatches["AT-X04-E.5"]`でtech-internal-aがjob-t07をstart→`shared:report-draft-all-normal`の入力でsaveDraft→submit→同じuserのhq-self-approverへswitchMembership→jobs.reviewで受理 ⑥tech-external-aがjob-contractor-aをstartした後、時計をassignment-contractor-aのvalidUntilと同じ2026-09-20T00:00:00.000Zへ進めてsaveDraft | ①②URLを維持したnot-found状態（IR57） ③NOT_FOUND ④開いたままの画面からのjobs.startはFORBIDDEN（資格失効は実行の都度再評価、SR03・D01順位4）、業務変更0件 ⑤FORBIDDEN（自己承認、D01順位4）、Jobはsubmittedのまま ⑥FORBIDDEN（errors.assignment_ended、作業窓は半開区間、IR94） |
| AT-X04-B | ①hq-operatorがmembers.saveでtech-external-aのvalidUntilを2026-10-15へ変更（scopeVersion=2）した後、Repository試験で旧Context（scopeVersion=1）のcommands.create(jobId=job-contractor-a) ②①の代わりにpermissionsからcontrol.diagnoseを外した後（scopeVersion=2）、旧Contextのcommands.create ③時計をcustomer-aのMembership.validUntil（2026-10-01T00:00:00.000Z）へ進めた後のunits.list | ①CONFLICT（errors.scope_changed）、Command 0件 ②FORBIDDEN（認可を先に判定、D01順位4） ③UNAUTHENTICATED（errors.membership_inactive）、/loginへ（IR101） |
| AT-X05-N | customer-aでunit-online-rtoにset_temperature 25→switchMembership(hq-operator)で同じ設備を開く | 同じcommandIdとUnit.versionを表示し、役割の切替で業務データをリセットしない |
| AT-X05-E | ①demo.trigger(transport、operation=commands.create、outcome=DELAY、delayMs=3000、retryAfterSeconds=null、remainingCalls=1)で応答を遅らせている間にdemo.reset ②画面を再読み込み | ①遅れて届いた応答は旧generationのため破棄して表示しない（IR17）。新しいgenerationのCommandは0件 ②seedに戻り、Sessionはnullで/login、locale=en（D09） |
| AT-X05-B | ①demo.resetを2回続けて実行 ②AT-X05-Nの操作中の外部ネットワーク要求を記録 | ①2回目もseedと同じIDとversion ②決済・通知・IoT・取引の外部オリジンへの要求0件 |
| AT-X06-N | customer-aでunit-online-rto（ventilation-demo v3）の操作パネルを開く | 温度16〜30、mode cool/dry/fan、風量low/mid/high、換気を有効として表示 |
| AT-X06-E | `acceptancePatches["AT-X06-B.1"]`でcustomer-aがUIを経由せずunit-non-rtoへcommands.create(set_temperature 25) | VALIDATION（D01順位7の機種候補）、Command 0件 |
| AT-X06-B | ①`acceptancePatches["AT-X06-B.1"]`（温度非対応） ②`["AT-X06-B.2"]`（coolだけ） ③`["AT-X06-B.3"]`（送風だけ） ④seedのunit-non-rto（contract-general-aの一般保守） ⑤`["AT-X06-B.5"]`（restrictionEligible=falseのRTO契約contract-rto-x06）でhq-restriction-managerがrestrictions.schedule | ①温度操作をdisabledにして理由を表示 ②mode候補はcoolだけ ③mode候補はfanだけで温度操作はdisabled ④RTOの請求がなくても操作できる ⑤VALIDATION（fieldErrors.contractId、errors.restriction_ineligible、IR101）、Restriction 0件 |
| AT-X07-N | customer-aでnotif-alert-window-aを既読にする | readAtを保存し、alert-window-aはopenのまま、summaries.getのalertCount=1のまま（IR51） |
| AT-X07-E | AT-C09-Nの社内案件で、tech-internal-aの提出後にcustomer-aがjobs.get→hq-operatorがjobs.reviewで理由付きの差し戻し→tech-internal-aが再提出→hq-operatorが受理→customer-aがjobs.get | 受理前はreportRefsが空で本文を表示しない。受理後は受理版だけを表示し、差し戻しの理由は表示しない（IR42） |
| AT-X07-B | customer-aでcommands.create→signOut→hq-operatorでaudit.list | 監査のactorId、actorRoleAtTime=client、correlationId、result=successがサインアウト後も変わらない |

具体的なやり取りの内容については、[フロントエンド入出力契約](../02-design/implementation-contracts.md)を参照してください。非機能要件は、数値の目標と測定した環境をあわせて示して判定するものであり、ライブラリを導入しただけで達成したとは扱いません。

## 0.8.0で具体化した実装条件

本版は既存FRの曖昧さをDEC-11として解消する。機能範囲の追加ではない。Actorとscopeは本書の権限表、操作ごとの必要permissionは操作カタログ、Trigger/Input/Processing/Output/成功条件は各DDと正規DTO、Failure/Validation/Error handlingは確定契約D01〜D13を合わせて参照する。

FR-X01/X02はD09の30分セッション・en/ms固定文法、FR-X03/X05はD05/D07の接続/品質/購読、FR-X04はD01/D06/D12の認可と投影、FR-X07はD08の宛先・通知リンクを必須条件とする。NFR01/02/04はD10の測定環境・回数・最大値・a11y検収、NFR03/05はD04/D08の再送・安全な表示、NFR06はD11の本番分離、NFR07は検証計画と証跡、NFR08はD09の表示通貨固定を適用する。新規User CRUD、実決済、実音声、実IoT制御は追加しない。

0.9.0修正契約: [厳格レビュー修正契約](../02-design/strict-review-contracts.md)と[操作別版契約](../02-design/write-version-catalog.csv)を併読する。

現行0.21.0の追加契約: [再レビュー修正契約](../02-design/review-resolution-contracts.md) IR01〜106を併読する。同じ論点の旧記述より優先し、衝突時の順位はIR72に従う。
