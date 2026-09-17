# AC Project フロントエンド開発ドキュメント

版: 0.19.0 / 作成日: 2026-09-14 / 更新日: 2026-09-17 / 状態: レビュー用ドラフト / 言語: 日本語

対象はSplit Unit ACの監視・操作・保守・契約に関するクリック可能なフロントエンドデモ（1A）。今回作成する文書はフロントエンド設計のみ。API仕様・HTTP契約・DB・サーバー処理・本番運用は設計対象外。将来のAPI接続はフロント側interfaceの差替え口だけを定義する。アプリ実装も今回の作業には含まない。

ドキュメント作成者・1A仕様の最終判断者: **北野正樹（Masaki Kitano）、若井悠馬（Yuma Wakai）**。モックデモ範囲と業務ルールの採用判断は[DEC-12](00-prepare/internal/decision-record-2026-09-16.md)を参照。デプロイ前の最終確認は人および外部レビュアーが行う。

## 読む順序

| 順序 | 文書 | 用途 |
|---|---|---|
| 1 | [PrepareDocument（企業向け）](00-prepare/PrepareDocument.md) | 企業のオリジナル要件の整理・分析、フロントエンドへの展開方針 |
| 2 | [共通要件](01-requirements/common.md) | 全役割の業務境界・非機能要件・権限 |
| 3 | 役割別要件: [クライアント](01-requirements/client.md) / [施工業者](01-requirements/contractor.md) / [技術者](01-requirements/technician.md) / [管理者](01-requirements/admin.md) | 要件ID、優先度、受入条件 |
| 4 | [共通詳細設計](02-design/common.md) | 表示データ、差替えinterface、状態遷移、モック設計 |
| 5 | 役割別詳細設計: [クライアント](02-design/client.md) / [施工業者](02-design/contractor.md) / [技術者](02-design/technician.md) / [管理者](02-design/admin.md) | 画面、入力、処理、権限、異常系 |
| 6 | [共通UIUX仕様書](03-uiux/UIUXSpecification.md) | ライブラリ、状態管理、トークン、アクセシビリティ |
| 7 | [Agentic SDLC](04-agentic-sdlc/README.md) | エージェント分担、ゲート、引き継ぎ契約 |
| 8 | [参考デザイン分析](00-prepare/reference-design-analysis.md) / [フロントエンド入出力契約](02-design/implementation-contracts.md) / [操作カタログ](02-design/operation-catalog.csv) | ソース由来の視覚値・詳細入出力 |
| 9 | [検証計画](04-agentic-sdlc/verification.md) / [追跡表](00-prepare/traceability.csv) | 要件→設計→テストの照合 |

4種類の本体文書はPrepareDocument、要件定義書、詳細設計書、UIUX仕様書。要件・設計は4役割に分割し、重複を防ぐため共通事項を別ファイルに置く。Agentic SDLC文書は実行時の付属資料であり、第5の製品仕様ではない。

## 文書の規約

- 出所は「企業原文 SRC-06」「制作方針 SRC-02」「参考モックの観察 SRC-05/04」「設計補完」に分ける。企業原文由来でも詳細承認済みとは扱わない。
- `PROPOSED`: 本書で選んだ可逆的な設計案。通常のデモ実装は進行可能。変更時は決定記録を残す。
- `OPEN`: 人による業務判断または外部仕様が未確定。依存する本番処理は進めない。モックは仮定を明示して進める。
- `MUST` / `SHOULD` / `MAY`: 必須 / 推奨 / 任意。P0・P1はいずれも1A完了対象で、P0を先に実装する。P2は本番・将来拡張。
- ID接頭辞: `FR-C/P/T/A`=4役割、`FR-X`=共通、`NFR`=非機能、`DD`=設計、`UX`=共通UI、`AT`=受入テスト、`DEC/OPEN`=意思決定・未決。
- 各役割の要件行は1つの検証単位。表の受入条件に行頭のIDから作る`AT-*`を付与する（例: FR-C01 → AT-C01）。S01〜S08は役割横断の追加検証。
- 同じ論点で仕様の記述が食い違う場合の規範の優先順位は[IR72](02-design/review-resolution-contracts.md#ir72-規範の優先順位--rev18-029)の表に従う。実装で選ばず文書欠陥として報告する。
- 優先順位: 原記録で確認できる最新の制作指示 → 企業原文 SRC-06 → 原文から整理した要件 → 原記録未収録の制作方針（4役割等。可逆的な範囲で現行基準として維持、OPEN-10で企業確認） → 共通契約・役割別設計。参考モックは外観設計の参考資料。衝突は黙って上書きせず、影響IDを記録する。
- 承認者欄が空欄のものは未承認。SRC-02の過去原指示は未収録であり、[確認状態](00-prepare/sources/production-instructions.md)を参照する。実装／テスト／レビューエージェントは自分で業務承認を作らない。

## 変更管理

変更は元の要件IDを維持し、PrepareDocumentの決定表、影響する要件・設計、追跡表、受入条件を同じ変更単位で更新する。廃止IDは再利用しない。証跡は[成果物テンプレート](04-agentic-sdlc/templates/artifacts.md)に記録する。

現在の承認者: 未指定 / 実装状況: 未着手 / アプリ動作検証: 未実施。文書の整合確認とアプリの受入試験は別に扱う。

0.5.0（2026-09-15）: [企業原文](00-prepare/sources/company-requirements-original.txt)から26項目を整理し、共通・4役割の要件と詳細設計、UIUX仕様書を改訂しました。[企業要望対応表](00-prepare/company-requirement-map.csv)・[要件別の出所表](00-prepare/requirement-origins.csv)・[追跡表](00-prepare/traceability.csv)で原文、設計補完、受入条件を確認できます。窓開放・断熱不足、アレルゲン、カード区分、Scope 2、市場連携構想の表示を具体化しました。参考HTML/CSSの外観値は保持しています。

各エージェントは原文・出所表を先に読み、設計補完を企業の承認事項として扱わないでください。詳細本文・入力契約・受入条件を合わせて使用します。文書の構造検証とアプリ動作検証は区別し、アプリ試験は未実行です。

0.6.0（2026-09-15）: 入出力・読取・試運転・手動入金・制限解除の契約を具体化し、117操作に更新。受入条件を具体的な入力と期待結果へ整理し、追加R01 10件を含む182ケース束を追跡します。実行状態schema、仕様baselineと変更時の再判定を統一しました。[制作指示の確認状態](00-prepare/sources/production-instructions.md)で未収録の原指示と設計解釈を区別します。アプリ試験は未実行です。

0.7.0（2026-09-16）: 4役割147行の受入条件（AT-N/E/B）をfixture値と観測可能な期待値の対へ書き直し、①②…のsubcase番号を導入しました。`automations.fire`（発火の書込み契約）と`jobs.resumeHold/resumeRework`を追加し119操作に更新。出所ラベルの判定規則をPrepareDocument §2に定め、FR-T02・FR-A07を再判定。優先順位を原記録の有無で分け、施工業者の独立役割をOPEN-10へ登録。役割識別子の正規名表（DDC-08§5）、エージェント定義の共通規則の一元化（SDLC README §7）、文書工程のgate記録（`04-agentic-sdlc/runs/`）を追加しました。アプリ試験は未実行です。


0.8.0（2026-09-16）: 厳格レビュー36件への修正。1Aの認可・発火仲裁・制限復旧・支払試行・数値・状態・表示安全性を[確定契約](02-design/deterministic-contracts.md)で具体化し、[正規DTO](02-design/service-contracts.ts)、131操作、47画面、Component/Query契約を追加。受入条件と元文書の矛盾を修正。AT-FIX-001〜036は追加試験計画でありアプリ実行結果ではない。

現行baselineは[DOC-0.19.0](04-agentic-sdlc/runs/DOC-0.19.0/spec-manifest.json)。DOC-0.7.0〜0.18.0は旧版の記録であり、現在の実装入力として使用しない（DOC-0.18.0はbaseline未作成、IR75）。検証は `python3 docs/tools/validate_documents.py` と `python3 docs/tools/check_review_regressions.py` の両方（IR73）。文書修正者による自己再レビューと、別主体によるG1承認は区別する。本番接続はD11の成果物確定までNOT READY。

0.9.0（2026-09-16）: STRICT-DOC-0.8.0の指摘21件の修正仕様を反映。期間・offset失敗・制限中契約編集の3件は2026-09-16ユーザー承認済み。G1はpendingで、旧版の承認は流用しない。詳細は[修正契約](02-design/strict-review-contracts.md)。

0.10.0（2026-09-16）: 設計再レビュー8件をSR22〜29へ反映し、反復確認の追加8件も修正。承認済みSR17〜19は維持。設計書の静的検証とシナリオ照合を行い、アプリの実装/動作試験は今回の完了条件としない。独立G1承認は別記録。

0.11.0（2026-09-16）: [再レビュー修正契約](02-design/review-resolution-contracts.md)で17件を仕様化。長時間メモリ容量1件は追加要件候補として明示保留。修正後の自己再レビューで追加6件も修正。[24件の受入計画](04-agentic-sdlc/acceptance-resolution.csv)を追跡表へ反映。独立G1はpending。

0.12.0（2026-09-16）: 再レビューで見つけた8件を修正。制限の全対象scope・予告宛先、受諾再送、限定投影sort、督促preview、分始点の積算、測定/点検保存、Fact評価を明確化。[受入計画](04-agentic-sdlc/acceptance-convergence.csv)を追加。

0.13.0: 案件の公開投影に関する再レビュー5件を修正。[受入計画](04-agentic-sdlc/acceptance-projection.csv)で一覧の検索/件数、期限後履歴、集計、ページング中の失効を追跡する。アプリ試験は未実施。

0.14.0: 設置物件住所と期限後報告状態のユーザー判断を反映。案件集計の条件、停止理由の取得、機種保存の理由入力と操作件数を修正。アプリ試験は未実施。

0.15.0: 独立レビューIRV-001〜006を修正。集計AT、住所説明、現行引渡し参照を統一し、完了日時・設備アラート重大度・共同編集報告の自己承認禁止を具体化。判定証跡は現行runsを参照。アプリ試験は未実施。

0.16.0: 再レビューで案件filter・監査取得経路・相関ID検索・Page件数説明を修正。権限は2種類、ソート機能のデフォルトは業務順というユーザー回答を反映済み。現行G1の判定は[gate記録](04-agentic-sdlc/runs/DOC-0.16.0/gate-G1.yaml)を参照。旧DOC-0.15.0の合格は本版へ流用しない。

0.16.0の判定記録は[runs/DOC-0.16.0](04-agentic-sdlc/runs/DOC-0.16.0/review.md)に当時版として保持する。

0.17.0（2026-09-16）: 別AIによる独立レビュー（FRV-001〜025、CRITICAL 2・MAJOR 8・MINOR 15）を反映。解除要求の起動経路とreleaseの冪等性、デモ時計ジャンプとセッション寿命、transport障害・ネットワーク断の注入、顧客起点案件の期限、archived資源の可視性、顧客数の母集団、1h/24hプリセット、役割別投影の非公開項目、Device登録時のSensor生成、表示書式・翻訳fallback・描画例外を[IR35〜44](02-design/review-resolution-contracts.md)で確定。SCR-X-not-foundとComponent契約9件を追加（48画面・66 Component）。採用した設計提案は[DEC-19〜24](00-prepare/internal/review-decisions-017.json)（PROPOSED、可逆）。[現行レビュー報告（13項目）](04-agentic-sdlc/runs/DOC-0.17.0/review.md)・[判定表](04-agentic-sdlc/runs/DOC-0.17.0/traceability-matrix.csv)・[受入計画15件](04-agentic-sdlc/acceptance-review-017.csv)を参照。修正後の再確認は修正担当と同一エージェントによる自己再レビューであり、独立G1は[gate記録](04-agentic-sdlc/runs/DOC-0.17.0/gate-G1.yaml)のとおりpending。アプリ試験は未実施。

0.18.0（2026-09-17）: 厳格レビューREV18-001〜048（BLOCKER 2・CRITICAL 4・MAJOR 24・MINOR 16・QUESTION 2）を反映。生存シミュレーター、制限中の操作表、設備接続の導出、Offer期限、技術者の閲覧窓/作業窓、KPI遷移とURL許可キー、件数定義、生活パターン、同意撤回、発火経路、セッション延長、取消表、FORBIDDEN/NOT_FOUND表示、通知一覧、決済確定の単一経路、demo-only操作、非RTO表示、空間未割当、省エネ基準、連絡可能時間、音声照合、Alert回復、機器操作の開始、負値表示、demoSeed、休日、変更イベント、規範の優先順位、検証器の変異テストを[IR45〜74](02-design/review-resolution-contracts.md)で確定した。137操作・48画面・67 Component。採用した設計提案は[DEC-25〜41](00-prepare/internal/review-decisions-018.json)（PROPOSED、可逆）。[指摘一覧](04-agentic-sdlc/runs/DOC-0.18.0/review.md)・[受入計画48件](04-agentic-sdlc/acceptance-review-018.csv)。修正の途中で作業が中断し、自己再レビューとbaseline作成は行われなかった（[gate記録](04-agentic-sdlc/runs/DOC-0.18.0/gate-G1.yaml)はnot_evaluated、IR75）。未完了分は0.19.0で引き継いだ。アプリ試験は未実施。

0.19.0（2026-09-17）: 中断したDOC-0.18.0の作業ツリーに対する独立レビューREV19-001〜042（BLOCKER 1・CRITICAL 3・MAJOR 16・MINOR 20・QUESTION 2。037は修正後の自己再レビュー、038〜042はユーザー指示による受入前提の全件点検で検出）を反映。引継ぎbaselineの作成と検証器の修復、作業窓開始前の技術者画面（work-not-started）、シミュレーターの複写条件、管理ダッシュボードの省エネ予想（energyForecast）、セッション延長の記述統一、C06の負値表示、旧記述検出の正規表現化、ログイン後の復帰先、再取得中の表示、同意の初期記録、KPI受入のseed差分、期限切れOfferへの応答、理由系の文字数、MRV表示項目の対応、作業窓終了の予告、demoSeedの正規化規則、再割当時の日程更新、受入Givenの解釈規則とseed差分、技術者の作業開始の失敗コードほかを[IR75〜93](02-design/review-resolution-contracts.md)で確定した。137操作・48画面・69 Component。採用した設計提案は[DEC-42〜53](00-prepare/internal/review-decisions-019.json)（PROPOSED、可逆）。省エネ予想の算出方法（DEC-44）と作業窓終了時の扱い（DEC-50）は2026-09-17のユーザー回答で確定。[レビュー報告](04-agentic-sdlc/runs/DOC-0.19.0/review.md)・[判定表](04-agentic-sdlc/runs/DOC-0.19.0/traceability-matrix.csv)・[受入計画42件](04-agentic-sdlc/acceptance-review-019.csv)。修正後の再確認は修正担当と同一セッションの自己再レビューであり、独立G1は[gate記録](04-agentic-sdlc/runs/DOC-0.19.0/gate-G1.yaml)のとおりpending。アプリ試験は未実施。
