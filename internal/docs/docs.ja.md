henji はパイプライン向けのコマンドライン LLM クライアントです。このマニュアルは作業単位で
使い方と注意点を説明します。フラグの一行一覧は `henji -h` を実行してください。

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

出力の契約は次のとおりです。

- **stdout はモデル応答だけ**です。`--output json` 時は JSON envelope だけを出します。
- 進捗、保存通知、エラー詳細は **stderr** です。`-q` はエラー以外を抑制します。
- stdout が端末でない場合は ANSI なしのプレーンテキストです。
- 成功時の終了ステータスは `0`、失敗時は非 0 です。

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

`--json-schema` は失敗時に検証エラーを示して修正を依頼し、`--json-schema-retries`（既定 2）まで
再試行します。Google 用スキーマでは `additionalProperties` を使えません。OpenAI strict mode では
各 object に `"additionalProperties": false` が必要です。小型ローカルモデルがコードフェンスを
付ける場合は、`raw JSON only, no code fences` をプロンプトに加えてください。

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

## ロール、設定、API キー

`roles:` には名前付き system prompt を設定し、`-R <role>` で選べます。`--list-roles` で一覧を
出します。設定ファイルは `$XDG_CONFIG_HOME/henji/henji.yml`（既定 `~/.config/henji/henji.yml`）、
会話データは `$XDG_DATA_HOME/henji/`（既定 `~/.local/share/henji/`）です。

設定の scalar 値は、たとえば `HENJI_TEMP=0.2`、`HENJI_MAX_TOKENS=4000` のように `HENJI_` 環境
変数で上書きできます。`apis` と `roles` は YAML で設定します。`max-input-chars` は結合した
プロンプトを送信前に切り詰めます。`--no-limit` はこの切り詰めを無効にします。

API キーは `api-key-cmd`、`api-key-env`、`api-key`、プロバイダー既定環境変数の順で解決します。
`api-key-cmd` はシェルを通さないため、`$USER` や `$(...)` は展開されません。

## 安全なコマンド利用

henji はコマンドを提案できますが、自動で実行しません。`| sh` のように出力を直接実行せず、まず
確認してから必要なコマンドを自分で実行してください。

    henji -R shell "find the largest files here" | less
