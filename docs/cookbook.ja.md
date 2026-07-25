# henji Cookbook

日常的な henji の設定と利用のための、コピーして使える実践集です。
[English](cookbook.md)

フラグの一覧は `henji -h`、JSON 出力の設計意図は
[`docs/notes/json-output-plan.md`](notes/json-output-plan.md) を参照してください。

## 保存場所

- 設定: `$XDG_CONFIG_HOME/henji/henji.yml`。未設定なら `~/.config/henji/henji.yml`。
  macOS と Linux はこの場所を共有し、Windows は `%LOCALAPPDATA%` を使います。
- 会話履歴・キャッシュ: `$XDG_DATA_HOME/henji/`。未設定なら `~/.local/share/henji/`。

## 実行方法と端末での挙動

henji は非対話式です。指示は引数に、まとまった入力は stdin に渡します。どちらもなければ
プロンプト画面を開かずエラー終了します。上流コマンドが対話モードへ入りパイプを閉じない場合も、
`Ctrl-C` で EOF を待たずに取消できます。

```sh
henji "explain this error" < error.log
git diff | henji --api local --model llama "suggest a commit message"
henji --text report.txt "summarize this report"
```

モデルの応答は常に stdout です。進捗スピナーとキャッシュ状態は、stdout が TTY のときだけ
stderr に出ます。`--quiet` はエラー以外の stderr 出力を隠します。そのため次のようなパイプは
安全です。

```sh
henji --output json "..." | jq -r '.content[0].text'
```

## 新しいプロバイダーを設定する

すべてのプロバイダーは `henji.yml` の `apis:` に置きます。必要なのは `base-url`、API キーの
取得方法、`models:` マップです。

### クラウドプロバイダー

macOS では API キーを設定ファイルに平文で置かず、Keychain に保存します。

```sh
security add-generic-password -a "$(whoami)" -s "MY-PROVIDER-API" -w "the actual key"
```

```yaml
apis:
  anthropic:
    base-url: https://api.anthropic.com/v1
    api-key-cmd: security find-generic-password -a masat -s ANTHROPIC-API -w
    models:
      claude-sonnet-5:
        aliases: ["sonnet-5", "sonnet"]
        max-input-chars: 4000000
```

`api-key-cmd` はシェルを経由しないため、`$USER` と `$(whoami)` は展開されません。リテラルの
ユーザー名を指定するか、サービス名だけで Keychain を検索してください。

### ローカルゲートウェイ（Ollama、mlx-lm、LM Studio など）

ローカルサーバーは通常キーを検証しませんが、OpenAI 互換経路では henji が常に送信します。
任意のプレースホルダーを指定し、`base-url` には `/v1` を含めてください。

```yaml
apis:
  local:
    base-url: http://localhost:8080/v1
    api-key: local
    models:
      mlx-community/gemma-4-E2B-it-qat-4bit:
        aliases: ["local", "gemma"]
        max-input-chars: 50000
```

### 現在のモデル名を調べる

モデルカタログは変わります。古い名前を推測せず、プロバイダー自身の models エンドポイントを
照会してください。たとえば Anthropic では次の形です。

```sh
curl -s https://api.anthropic.com/v1/models \
  -H "x-api-key: $(security find-generic-password -a masat -s ANTHROPIC-API -w)" \
  -H "anthropic-version: 2023-06-01" | jq -r '.data[].id'
```

## 設定済みモデルを調べる

```sh
henji --list-models                 # 人間向け
henji --list-models --output json   # スクリプト・エージェント向け
```

`default-api` と `default-model` は、`--api` / `--model` を省略したときに使います。
既定値を一時的に変えたいときは `henji.yml` を直接更新します。

```yaml
default-api: openrouter
default-model: z-ai/glm-5.2  # alias ではなく実際のモデル ID
```

## 推論モデルの出力が空になるとき

DeepSeek、Kimi、GLM、o1/o3 などは、表示しない推論にも token を使います。
`max-completion-tokens` が小さすぎると、推論だけで予算を使い切り、エラーなしで空の応答に
なることがあります。モデル単位で十分な値を設定してください。

```yaml
apis:
  openrouter:
    models:
      moonshotai/kimi-latest:
        max-completion-tokens: 4096
```

## 大きなファイルを渡す

AI エージェントやスクリプトから使うなら、内容を自分のコンテキストに読まず、シェルで stdin へ
渡します。

```sh
cat large_file.txt | henji --output json "summarize this"
cat file1.txt file2.txt | henji --output json "diff these"
henji --output json "summarize" < large_file.txt
```

henji は「指示 + 空行 + stdin」をモデルに渡します。指示は引数、処理対象データは stdin と
分けるのが基本です。

## `--text` と `--image`

`--text` は、stdin が別のパイプライン入力を持つときに UTF-8 テキストを 1 ファイル添付する
ためのものです。上限は 3 MiB で、繰り返し指定とバイナリらしい内容は明示的に失敗します。

```sh
git diff | henji --text requirements.txt "check whether this diff satisfies the requirements"
cat docs/*.md | henji "find contradictions across these documents"
```

`--image` は JPEG、PNG、WebP を 1 つ、最大 3 MiB 添付します。使うモデルにだけ
`vision: true` を設定してください。テキスト・画像の添付は会話履歴に保存されないため、継続時に
必要ならもう一度添付します。

## 会話の一覧・再開

`--list` は選択 UI ではなくプレーンな一覧です。ID またはタイトルを次のコマンドへコピーします。

```sh
henji --list
henji --show a1b2c3d
henji --continue a1b2c3d "now propose the smallest fix"
```

## スクリプト向け `--output json`

```sh
henji --output json "explain this error" < error.log
```

成功時は内容・モデル・会話 ID を含む一行 JSON、失敗時は終了コード 1 と error envelope を返します。

```json
{"version":1,"content":[{"type":"text","text":"..."}],"model":"...","conversation_id":"..."}
```

```sh
henji --output json "..." | jq -r '.content[0].text'
henji --output json "..." | jq -e '.error == null'
```

## `--json-schema` による構造化出力

`--format --format-as json` は JSON を依頼するだけです。厳密な形が必要な自動処理には、
ネイティブの構造化出力とクライアント側検証を行う `--json-schema` を使ってください。

```sh
git diff main | henji --json-schema review-schema.json \
  "review this diff for security issues" | jq '.findings[] | select(.severity == "critical")'
```

最初の応答が検証に失敗すると、henji は検証エラーを示して修正を依頼し、
`--json-schema-retries`（既定 2）まで再試行します。`--output json` と組み合わせることも可能です。

Google を対象にするスキーマでは `additionalProperties` を外してください。Gemini はこれを拒否します。
小さなローカルモデルには `raw JSON only, no code fences` とプロンプトで指定すると有効です。
