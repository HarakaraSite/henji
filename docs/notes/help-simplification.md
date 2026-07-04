# --help オプション説明の簡素化案

作成日: 2026-07-04（Fable 5 レビューセッション）  
ステータス: 提案 → 実装待ち

## 背景と方針

`henji docs`（埋め込みマニュアル）が実装されたため、`--help` は「1行クイックリファレンス」に徹する。
詳細・注意点・出力例は docs に委譲する（`--help` フッターに `Full manual: henji docs` のポインタが既にある）。

現状の問題: 一部フラグの説明が2〜3文の長文になっており（`--api`、`--output`、`--json-schema`、
`--max-tool-calls`）、折り返しで一覧性が損なわれている。

**方針:**

1. 各フラグの説明は1行・おおむね60文字以内（英語）に収める
2. 削る詳細は `internal/docs/docs.md` がカバーしていることを確認済み（下の対応表参照）。
   カバー漏れがあれば docs.md に追記してから削る
3. `config.go` の `help` マップは `config_template.yml` のコメント（`{{ index .Help "..." }}`）
   と共用。短くなった説明はテンプレートにもそのまま反映されるが、テンプレートは周辺の
   構造・例で文脈が読めるので許容する
4. AI-discoverability 実験（examples.go のコメント参照）で意図的に足した
   「max-tool-calls は mcp-servers 設定が前提」の警告は、**完全には消さない**。
   短縮形でヒントを残す（下記参照）。Examples フッターの4例と `Full manual:` 行は変更しない

## 変更対象（help マップの書き換え案）

| キー | 現状 | 変更案 |
|---|---|---|
| `model` | `Default model (gpt-3.5-turbo, gpt-4, ggml-gpt4all-j...)` | `Default model; see --list-models for configured models` |
| `api` | `OpenAI compatible REST API (openai, localai, anthropic, ...). default-model belongs to one API, so pair this with -m/--model or it will likely fail to find the model` | `API endpoint to use; pair with -m/--model` |
| `format` | `Ask for the response to be formatted as markdown unless otherwise set` | `Ask for a formatted response (default: markdown; see --format-as)` |
| `format-as` | `Format to request from the model when -f/--format is set; valid values are keys in format-text (default: markdown, json)` | `Format to request when -f is set (markdown, json, or a format-text key)` |
| `json-schema` | `Path to a JSON Schema file; the response is constrained to it (Anthropic/OpenAI-compatible/Google) and validated against it client-side. Google rejects some keywords (e.g. additionalProperties)` | `Constrain and validate the response with a JSON Schema file` |
| `json-schema-retries` | `Maximum number of times to ask the model to correct a response that failed JSON Schema validation` | `Max correction attempts when the response fails schema validation` |
| `output` | `Output format: text or json. json prints one line: {"version":1,...}...`（エンベロープ全文） | `Output format: text or json (single-line JSON envelope)` |
| `max-tool-calls` | `Maximum number of agentic tool call rounds (default: 0, unlimited). Tools only exist if mcp-servers are configured; check what's available with --mcp-list-tools before relying on this` | `Max MCP tool call rounds (0 = unlimited; needs mcp-servers configured)` |
| `no-limit` | `Turn off the client-side limit on the size of the input into the model` | `Turn off the client-side input size limit` |
| `quiet` | `Quiet mode (hide the spinner while loading and stderr messages for success)` | `Hide the spinner and success messages on stderr` |
| `editor` | `Edit the prompt in your $EDITOR; only taken into account if no other args and if STDIN is a TTY` | `Compose the prompt in your $EDITOR` |
| `list-models` | `List configured APIs and their models (respects --output json)` | 変更なし（十分短く、`--output json` 対応のヒントは残す価値あり） |
| `mcp-list` | `List all available MCP servers` | `List configured MCP servers` |
| `mcp-list-tools` | `List all available tools from enabled MCP servers` | `List tools from enabled MCP servers` |

上記以外のキー（`continue`、`show`、`delete`、`raw` 等）は既に1行で簡潔なので変更しない。

**テンプレート専用キーは触らない**: `temp`、`topp`、`topk`、`stop`、`http-proxy`、
`max-retries`、`word-wrap`、`max-input-chars`、`max-completion-tokens`、`apis`、`roles`、
`format-text`、`mcp-servers`、`mcp-timeout` はフラグが存在せず `config_template.yml` の
コメント専用。設定ファイル側では詳しい説明に価値があるため現状維持。

## 削る詳細と docs.md の対応（確認済み）

| --help から削る内容 | docs.md での記載箇所 |
|---|---|
| `-a` と `-m` のペアリング必須の説明 | "Choosing a provider and model"（43行目付近） |
| `--output json` のエンベロープ全文 | "Getting machine-readable output"（66行目付近に実例） |
| Google が `additionalProperties` 等を拒否する注意 | 86行目付近 |
| max-tool-calls の「MCPサーバ設定が前提」詳細 | "MCP tools" セクション（短縮形のヒントは help にも残す） |

実装時に上記4点が docs.md に実際にあることを再確認し、欠けていれば docs.md に追記すること。

## 受け入れ条件

1. `./henji --help` の各オプションが端末幅100桁で折り返さない（長くても1行）
2. `go test ./...`・`go vet ./...`・`go test -race ./...` 全パス
   （`TestManualLongFlagsExist` は docs.md 側のフラグ参照をチェックするので影響しないはず）
3. `config_template.yml` を新規生成した場合（`XDG_CONFIG_HOME` を一時ディレクトリにして起動）に
   コメントが崩れていないこと
4. Examples フッター・`Full manual: henji docs` 行・`docs` サブコマンドの説明は変更されていないこと
5. 変更は1コミット（`Simplify --help flag descriptions, defer details to henji docs` 等）
