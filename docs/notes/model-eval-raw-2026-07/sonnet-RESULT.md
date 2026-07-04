# henji 利用能力テスト結果
- ドライバーモデル: Claude Sonnet 4.6 (claude-sonnet-4-6)
- 日時: 2026-07-04

## 情報源の利用
- `./henji -h` を最初に実行。オプション一覧（-m/-a/-f/--format-as/--json-schema/--output/-c/-C/-l/-t/-s/-d/-q/--list-models 等）が一目で分かり、この時点で「要約を取る」「モデル一覧を見る」コマンドの骨格は書けた（`./henji --list-models`、`cat file | ./henji -q "..."` など）。
- `--list-models` を実行し、設定済みモデルを確認: `gateway`(default) の `mlx-community/gemma-4-E2B-it-qat-4bit`（ローカル）と `openai` の `gpt-5.5`（クラウド）。`-h` の情報だけでどちらがローカル/クラウドかは `[local]` 表記から判断できた。
- `henji docs` は、タスク2で構造化JSON出力と「同じ会話の続きで質問する」方法の詳細を確認するために読んだ。具体的には、`--json-schema` の使い方・検証の挙動、`-c`/`-C` による会話継続、`--output json` のエンベロープ形式（`conversation_id` を取り出して `-c` に渡す一連の流れ）が `-h` の一行説明だけでは分からなかったため。また、ローカル小型モデルが JSON をコードフェンスで包む既知の落とし穴と対処法（"raw JSON only, no code fences" をプロンプトに追加）も docs 記載の情報で、実際にこの通りの問題に遭遇し解決した。

## タスク1
### ローカルモデル
- 成否: 成功
- 試行コマンド一覧（時系列）:
  1. `cat access.log | ./henji -q -a gateway -m mlx-community/gemma-4-E2B-it-qat-4bit "この access log を要約してください" > task1-summary.local.txt 2> task1-local.stderr` → 成功（exit 0、stderrも空、ANSIコード無し）
- つまずいた点と解決方法: 特になし。`-q` と stdout リダイレクトのみでスピナーやノイズは混入しなかった。

### クラウドモデル
- 成否: 成功
- 試行コマンド一覧（時系列）:
  1. `cat access.log | ./henji -q -a openai -m gpt-5.5 "この access log を要約してください" > task1-summary.cloud.txt 2> task1-cloud.stderr` → 成功（exit 0）
- つまずいた点と解決方法: 特になし。ただしクラウドモデルの回答はMarkdownの見出し・表を多用する構成だった（装飾指定はしていないのでモデルの自発的な出力スタイル）。ツール由来のノイズ（スピナー/JSON外皮）は混入していないため要件は満たしていると判断。

## タスク2
### ローカルモデル
- 成否: 成功
- 試行コマンド一覧（時系列）:
  1. `cat sample.go | ./henji -q --output json -a gateway -m mlx-community/gemma-4-E2B-it-qat-4bit --json-schema findings-schema.json "このGoコードをレビューし、指摘事項をfindings形式で報告してください" > ...` → **失敗**（exit 1）。エラー: `Response did not match --json-schema after 3 attempt(s).: response is not valid JSON: invalid character '`' looking for beginning of value`（モデルが ```json フェンスでJSONを包んでいた）
  2. 同コマンドにプロンプトを `「...raw JSON only, no code fences.」` に変更して再実行 → 成功（exit 0）。`jq -r '.content[0].text'` で envelope から取り出し `task2-findings.local.json` として保存、`python3 -c "json.load(...)"` で有効なJSONであることを確認。
  3. `./henji -q -a gateway -m mlx-community/gemma-4-E2B-it-qat-4bit -c <conversation_id> "先ほど指摘したSQLインジェクションの脆弱性について、具体的な修正方法をコード例つきで詳しく説明してください"` → 成功。ステップ1の指摘（fmt.Sprintfによる文字列連結）を踏まえた、プリペアドステートメントへの書き換え例を含む回答を `task2-detail.local.txt` に保存。
- つまずいた点と解決方法: docsに記載の通り、軽量ローカルモデルはMarkdownコードフェンスでJSONを包み、schema検証に3回とも失敗した。プロンプトに「raw JSON only, no code fences」を追加することで解消（docsの既知の落とし穴セクションの記載通り）。

### クラウドモデル
- 成否: 成功
- 試行コマンド一覧（時系列）:
  1. `cat sample.go | ./henji -q --output json -a openai -m gpt-5.5 --json-schema findings-schema.json "このGoコードをレビューし、指摘事項をfindings形式で報告してください"` → 成功（exit 0、1回目から有効なJSON。8件のfindingsを検出、ローカルより網羅的でnilポインタパニックやXSSも指摘）
  2. `jq -r '.content[0].text']` で抽出→`task2-findings.cloud.json`として保存、JSONとして有効性確認。
  3. `./henji -q -a openai -m gpt-5.5 -c <conversation_id> "先ほど指摘したXSS脆弱性（58行目）について、具体的な修正方法をコード例つきで詳しく説明してください"` → 成功。ステップ1で指摘した58行目のXSSを踏まえ、`html/template`への置き換えを中心とした詳細な修正例を `task2-detail.cloud.txt` に保存。
- つまずいた点と解決方法: 特になし。1回目の試行でコードフェンス無しの生JSONを返し、schema検証を一発通過した。

## ローカル/クラウドの違いで気づいたこと
- **JSON Schema遵守**: クラウド(gpt-5.5)は1回目からフェンス無しの正しいJSONを返したが、ローカル(gemma)は毎回```jsonフェンスで包み、プロンプトに明示的な指示（"raw JSON only, no code fences"）を追加するまで3回リトライしても失敗した。docsに書かれていた「小型ローカルモデルの既知の弱点」がそのまま再現した。
- **指摘の網羅性**: 同じsample.goに対し、ローカルは4件（SQLi/データ競合/エラー握りつぶし/goroutine乱発）、クラウドは8件（上記に加えdb未初期化のnilパニック、XSS、fdリーク、部分読み込み、エラー内容のレスポンス漏えいなど）を検出。クラウドの方が指摘範囲が広く、severityの重み付けも妥当だった。
- **要約の文体**: タスク1で、ローカルは見出し+箇条書き中心のプレーンな構成、クラウドは表（Markdownテーブル）を積極的に使い定量的な集計（総リクエスト数、ステータス別件数、IP別件数）まで自発的にまとめていた。
- **速度/安定性**: ローカルはリトライ機構がありつつも最終的に失敗パターンに1回落ちたのに対し、クラウドは全コマンドが一発で成功した。

## henji への改善提案
- -h に足りなかった情報: `--continue`(`-c`) が「タイトルまたはID」を受け取れることや、`--output json` の `conversation_id` をどう使うか（エージェントループの型）が -h だけでは分からない。一行説明だけでは「保存済み会話の再開」と「直前レスポンスからのID取り出し→継続」という組み合わせのユースケースに気づきにくい。
- docs に足りなかった情報・分かりにくかった箇所: `--json-schema` のリトライがモデルにどう「訂正指示」を送るか（プロンプトにどう追記されるか）の具体例が無く、ローカルモデルがコードフェンス問題を起こした際に「なぜ3回とも失敗したのか」の内部動作が見えなかった。実際に体感するまで、"raw JSON only" 的な自衛プロンプトが必要になることが予測しづらかった。
- その他: `--list-models` の出力で `[local]` や `[5.5]` のような注記が何を意味するか（`[local]`はエイリアス？API種別のヒント？）が `-h`/`docs` に説明が無く、モデルがローカルかクラウドかの判断は名前の見た目（`mlx-community/...` vs `gpt-5.5`）に頼らざるを得なかった。`--list-models --output json` のスキーマにエンドポイント種別やbase-urlの有無などが出れば、より確実にローカル/クラウドを機械的に判別できる。
