henji は文章生成と意思決定モデルに対応するコマンドライン LLM クライアントです。
このマニュアルは使い方と注意点を説明します。フラグは `henji -h` / `henji decision -h` で確認できます。

## 基本の実行方法

プロンプトは「引数、任意のテキスト・画像添付、stdin」の順で組み立てます。

    henji "explain this error"                # 引数だけ
    cat error.log | henji                      # stdin だけ
    cat error.log | henji "what went wrong?"  # 引数、空行、stdin
    henji --text report.txt "summarize this"  # 引数、添付テキスト

パイプ入力は指示と視覚的に区別できるようインデントして追加されます。プロンプトがなければ
対話入力を待たずエラー終了します。`Ctrl-C` はモデル要求、または stdin を閉じない上流コマンドを
取消します。

`--text` は UTF-8 テキストを 1 ファイル、`--image` は JPEG/PNG/WebP を 1 ファイル受け取り、
どちらも 3 MiB までです。`--image` を使うモデルには設定で `vision: true` が必要です。添付は
会話に保存されないため、継続で必要なら再添付してください。

通常の生成の出力契約は次のとおりです。

- **stdout はモデル応答だけ**です。`--output json` 時は JSON envelope だけを出します。
- 進捗、保存通知、エラー詳細は **stderr** です。`-q` はエラー以外を抑制します。
- stdout が端末でない場合は ANSI なしのプレーンテキストです。
- 端末のMarkdown整形は、別途インストールした `PATH` 上の外部 `glow` に依存します。
  生成完了後に整形し、Glowがない場合や失敗時は原文を表示します。パイプ・`--raw` は逐次出力し、
  JSON・Schema・decisionはGlowを使いません。導入・表示設定はREADMEとcookbookを参照してください。
- 成功時の終了ステータスは `0`、失敗時は非 0 です。
- JSON 出力を選択した後の準備・生成・保存の失敗は、一行の `error` envelope に反映します。
  保存失敗時も生成済みの `content` を保持します。出力形式の選択前に設定読込が失敗した場合は
  stderr に診断を出します。

## プロバイダーとモデルを選ぶ

プロバイダー（API）とモデルは設定ファイルに定義します。確認には次を使います。

    henji --list-models
    henji --list-models --output json

- `-m <model>` はモデル ID または alias を指定します。
- `-a <api>` はプロバイダーを選びます。`default-model` は特定 API に属するため、通常は `-a` と
  `-m` を組にしてください。
- 404 時の代替として、モデルに `fallback:` を設定できます。
- `anthropic` と `google` はネイティブプロトコル、それ以外は OpenAI 互換プロトコルです。

Ollama、mlx-lm、LM Studio などのローカルゲートウェイでは、`base-url` に `/v1` を含めます。
henji はこの経路でも API キーを送るため、検証しないサーバーにはプレースホルダーを設定します。

## 機械可読な出力

用途に応じて三段階あります。

1. `--format --format-as json` は JSON を依頼するだけで、検証はしません。
2. `--output json` は応答を一行の安定した envelope に包みます。
3. `--json-schema <file>` はプロバイダーの構造化出力を使い、クライアント側でも検証します。

    git diff | henji --output json "suggest a commit message" | jq -r '.content[0].text'
    henji --json-schema review.json "review this diff" < diff.patch | jq '.findings[]'

アプリ連携では、`-q --no-cache --json-schema <file>` を使い、`--output json` を指定しなければ、
成功時の stdout は検証済みのモデル JSON と末尾改行だけで、`content` や `error` のラッパーは
付きません。終了コードは 0 です。準備・API 呼び出し・応答検証が最終的に失敗した場合は、
終了コード 1、stdout は空、stderr に人間向けのエラー詳細を出します。
`-q` は進捗表示を抑制しますが、エラーは抑制しません。
`--json-schema` と `--output json` を併用すると、検証済みの JSON は envelope の
`content[0].text` に文字列として入ります。

`--json-schema` は失敗時に検証エラーを示して修正を依頼し、`--json-schema-retries`（既定 2）まで
再生成します。`--json-schema-retries 0` はこの再生成だけを無効にします。レート制限などに対する
HTTP/API 再試行は別で、`max-retries` とプロバイダークライアントが処理します。

Google 用スキーマでは `additionalProperties` を使えません。OpenAI strict mode では
各 object に `"additionalProperties": false` が必要です。小型ローカルモデルがコードフェンスを
付ける場合は、`raw JSON only, no code fences` をプロンプトに加えてください。

## Decisions APIによる判断

    henji decision -a openrouter -m typesafe/jev-1.13 --questions q.json < input.txt

`-m` は必須で生成用の既定モデルを継承しません。モデルIDの事前登録は不要で、設定済み別名も
使えます。`-a` は省略時に設定のAPIを使います。質問は各APIのJSON形式を指定します。

- OpenRouter: `{"urgent":{"type":"noul","instructions":"Is it urgent?"}}`
- OpenAI: `[{"name":"urgent","type":"predicate","instructions":"Is it urgent?"}]`

OpenAIでは `-a openai -m gpt-6-luna` を使います。一回の起動で一つの対象を判断する一問一答です。
同じ対象への複数質問は一緒に送れます。会話の継続はなく、連続実行はシェルループを想定します。
成功時はAPI応答全体のJSONをstdoutへ出し、正常refusalも終了コード0です。失敗時は終了コード1、
stdoutは空、詳細はstderrです。`output` 設定に左右されず、履歴・DB・キャッシュは作りません。
前の判断に依存する処理では呼び出し元が次の入力を作ります。stdinの文章は加工せず送ります。

`--image` は1枚・JPEG/PNG/WebP・3MiB以内で、モデル設定の `vision: true` が必要です。
3MiBは読込み上限であり、モデルの受理上限は別です。自動縮小・再圧縮は行いません。
Clef系は画像約300KB未満を推奨し、文章は先頭約2,000トークン以降を黙って切り捨てます。

認証・proxy・再試行は共通設定を使います。再試行回数は初回後の追加試行回数で、0なら一回です。
独自gatewayには `decision-protocol` と `decision-base-url`（末尾の `/decisions` は除く）を設定します。
生成用 `base-url` はこの接続先に使いません。専用フラグは `henji decision -h` で確認できます。

## 典型的なエージェントの流れ

    # 1. プロバイダーとモデルを確認
    henji --list-models --output json

    # 2. 会話 ID を取得
    id=$(git diff | henji --output json "review this diff" | jq -r .conversation_id)

    # 3. 同じ会話を継続
    henji --output json -c "$id" "now suggest fixes for finding 1"

## 会話

成功したモデル会話は、`--no-cache` を指定しない限り保存されます。`-l` は保存時刻を含むプレーンな
一覧を出し、対話選択を開きません。`-t` で保存時のタイトルを指定できます。`-C` は最新会話、
`-c <id-or-title>` は特定会話、`-s` は表示、`-d` は削除です。

    henji --list
    henji --show <id-or-title>
    henji --continue <id-or-title> "follow-up prompt"

継続では履歴全体を再送するため、入力と費用が増えます。`-c` / `-C` で `-a` / `-m` を省略すると、
保存時の API とモデルを復元します。

同じ会話の操作は履歴読込から保存まで排他し、後続の要求は先の操作が終わるまで待機します。
別会話は並行実行でき、排他の待機中も Ctrl-C で取り消せます。

## ロール、設定、API キー

`roles:` には名前付き system prompt を設定し、`-R <role>` で選べます。`--list-roles` で一覧を
出します。設定ファイルは `$XDG_CONFIG_HOME/henji/henji.yml`（既定 `~/.config/henji/henji.yml`）、
会話データは `$XDG_DATA_HOME/henji/`（既定 `~/.local/share/henji/`）です。

設定の scalar 値は、たとえば `HENJI_TEMP=0.2`、`HENJI_MAX_TOKENS=4000` のように `HENJI_` 環境
変数で上書きできます。`apis` と `roles` は YAML で設定します。`max-input-chars` はバイト数の
上限です。結合したプロンプトとテキスト間の区切りを数え、UTF-8 の文字境界で切り詰めます。
画像はこの上限に含めません。`--no-limit` はこの切り詰めを無効にします。

OpenAI 互換 API 向けの `max-completion-tokens` は YAML（全体またはモデルごと）、または
`HENJI_MAX_COMPLETION_TOKENS` で設定し、専用 CLI フラグはありません。
`max-tokens` と両方設定すると両方を送信します。CLI の `--max-tokens` は、継承した
`max-completion-tokens` の値を解除しません。両フィールドの扱いはプロバイダーによるため、
`--max-tokens` だけで実際の上限は確定しません。

API キーは `api-key-cmd`、`api-key-env`、`api-key`、プロバイダー既定環境変数の順で解決します。
`api-key-cmd` はシェルを通さないため、`$USER` や `$(...)` は展開されません。stdout だけを
キーとして取得し、stderr は含めません。コマンドが非 0 で終了した場合はエラーとし、下位の
取得方法にはフォールバックしません。

## 安全なコマンド利用

henji はコマンドを提案できますが、自動で実行しません。`| sh` のように出力を直接実行せず、まず
確認してから必要なコマンドを自分で実行してください。

    henji -R shell "find the largest files here" | less
