# mods リポジトリ概観

このメモは、Go 製 CLI `mods` の全体構成、起動経路、設定読み込み、LLM provider 実装の置き場所を把握するための入口メモです。

## 全体構成

- `main.go`
  - Cobra の root command、flag 初期化、設定読み込み、DB 初期化、Bubble Tea program 起動を担当する CLI の中心。
- `mods.go`
  - Bubble Tea model `Mods` の状態遷移と、stdin 読み込み、LLM request 開始、stream 受信、出力描画を担当。
- `stream.go`
  - prompt/system role/cache から provider に渡す `proto.Message` 群を組み立てる。
- `config.go`
  - YAML 設定の型定義、デフォルト設定、設定ファイル生成/読み込み、環境変数反映。
- `config_template.yml`
  - 初回起動時に生成される設定ファイルのテンプレート。default API/model、roles、MCP、provider/model 定義が入る。
- `flag.go`
  - duration flag など flag parsing 周辺の補助。
- `db.go`
  - 保存会話のメタデータ用 SQLite DB。会話 ID/title/api/model/updated_at を管理する。
- `internal/cache/*`
  - 保存会話本体の gob cache。`[]proto.Message` を読み書きする。
- `mcp.go`
  - MCP server 設定の解決、tool 一覧取得、tool call 実行。
- `internal/proto/*`
  - provider 非依存の request/message/tool call 型。
- `internal/stream/*`
  - provider 共通の streaming client / stream interface。
- `internal/{openai,anthropic,google}/*`
  - 各 provider 実装と proto 変換処理。Ollama は専用実装を持たず `openai` 経由（PR#23）。

## エントリポイント

実行開始点は `main.go` の `main()`。

大まかな流れは以下。

1. `ensureConfig()` で設定ファイルを用意し、YAML と環境変数を `config` に読み込む。
2. `initFlags()` で Cobra flags を `config` の各フィールドに bind する。
3. completion/man/version/help 以外では `openDB(config.CachePath + "/conversations/mods.db")` で SQLite DB を開く。
4. completion/man 用の隠し command が必要な場合だけ追加する。
5. `rootCmd.Execute()` を実行する。
6. `rootCmd.RunE` 内で Bubble Tea の `Mods` model を作り、`tea.NewProgram(...).Run()` する。

`rootCmd.RunE` は CLI の実処理の分岐点。引数を `config.Prefix` に入れ、TTY 状態に応じて Bubble Tea の入出力 option を決め、必要なら editor や interactive prompt を開く。その後 `newMods(...)` を起動し、終了後に `--dirs`, `--settings`, `--reset-settings`, `--list`, `--show`, `--mcp-list` などの結果処理や保存を行う。

## 実行時の状態遷移

`mods.go` の `Mods` は Bubble Tea model で、主な state は以下。

- `startState`
- `configLoadedState`
- `requestState`
- `responseState`
- `doneState`
- `errorState`

起動時の `Init()` は `findCacheOpsDetails()` を返す。ここで continue/show/title 用の read/write ID、保存済み会話から引き継ぐ API/model などを決める。

その後の主な流れ。

1. `cacheDetailsMsg`
   - cache read/write 情報を `Config` に反映し、stdin 読み込みへ進む。
2. `completionInput`
   - stdin 内容と CLI 引数 prefix をもとに request を開始する。
3. `startCompletionCmd`
   - model/API 解決、provider client 作成、MCP tools 読み込み、`proto.Request` 組み立て、stream 開始。
4. `completionOutput`
   - stream chunk を受け取り、TTY なら Glamour で markdown 表示、非 TTY/raw なら逐次 stdout に出す。
5. stream 完了時
   - tool calls があれば MCP tool を実行して provider stream を継続する。
   - tool calls がなければ `m.messages = msg.stream.Messages()` に保存対象の会話を残して終了する。

## 設定ファイルの読み込み

設定読み込みの中心は `config.go` の `ensureConfig()`。

読み込み順は以下。

1. `xdg.ConfigFile(filepath.Join("mods", "mods.yml"))` で設定パスを決める。
2. 設定ディレクトリを `0700` で作る。
3. 設定ファイルがなければ `writeConfigFile()` -> `createConfigFile()` で `config_template.yml` から生成する。
4. `os.ReadFile()` で YAML を読む。
5. `yaml.Unmarshal()` で `Config` に読み込む。
6. `env.ParseWithOptions(&c, env.Options{Prefix: "MODS_"})` で `MODS_` prefix の環境変数を反映する。
7. `CachePath` が空なら `xdg.DataHome/mods` を使う。
8. `CachePath/conversations` を作る。
9. `WordWrap` が 0 なら 80 にする。

その後 `main()` の `initFlags()` で CLI flags が bind されるため、実効値の優先度はおおむね `config_template.yml` / YAML < `MODS_*` 環境変数 < CLI flags。

主な設定型は `Config`, `API`, `Model`。

- `Config.APIs` は YAML の `apis:` を保持する。
- `API` は `base-url`, `api-key`, `api-key-env`, `api-key-cmd`, `models`, `user` などを持つ。
- `Model` は model 名、alias、fallback、max input chars、Google 用 `thinking-budget` などを持つ。
- `APIs.UnmarshalYAML` は map 的な YAML を順序付き slice として読むための custom decoder。

API key 解決は `mods.go` の `ensureKey()`。

1. `api-key`
2. `api-key-env`
3. `api-key-cmd`
4. provider ごとの default env、例: `OPENAI_API_KEY`, `ANTHROPIC_API_KEY`, `GOOGLE_API_KEY`, `AZURE_OPENAI_KEY`

の順で探す。

## request 組み立て

`stream.go` の `setupStreamContext(content, mod)` が provider に渡す会話文脈を作る。

- `--format` が有効なら `format-text[format-as]` を system message に追加する。
- `role` が指定されていれば `roles[role]` の各要素を system message に追加する。
- role の各文字列は `loadMsg()` を通すため、通常文字列だけでなく `http://`, `https://`, `file://` から本文を読み込める。
- CLI 引数由来の `Prefix` と stdin content を結合する。
- `no-limit` が false なら `mod.MaxChars` で入力文字数を切る。
- continue 時など `cacheReadFromID` があれば既存会話を cache から読み込む。
- 最後に user message として今回の content を追加する。

`mods.go` の `startCompletionCmd()` はこれを呼んだ後、`proto.Request` を作る。`proto.Request` は provider 共通の request 型で、messages/model/user/sampling params/tools/tool caller などを持つ。

## LLM provider 実装

provider 共通 interface は `internal/stream/stream.go`。

- `Client.Request(context.Context, proto.Request) Stream`
- `Stream.Next()`, `Current()`, `Close()`, `Err()`, `Messages()`, `CallTools()`

provider 非依存の message/request 型は `internal/proto/proto.go`。

実装の置き場所は以下。

- `internal/openai/`
  - OpenAI 互換 API、Azure OpenAI、Perplexity などの default path。
  - SDK は `github.com/openai/openai-go`。
  - `openai.go` が client/stream 実装、`format.go` が `proto.Message` との変換。
  - MCP tool calls と JSON response format に対応。
- `internal/anthropic/`
  - Anthropic Messages API 用。
  - SDK は `github.com/anthropics/anthropic-sdk-go`。
  - system message と通常 messages を分ける変換を行う。
  - MCP tool calls に対応。
- `internal/google/`
  - Gemini REST SSE 用。
  - SDK ではなく HTTP request/response stream を直接扱う実装。
  - `ThinkingBudget` は Google provider の generation config に入る。
  - `CallTools()` は未対応（Google API は tool call 非対応のため）。
  - `Messages()` は PR#7 で修正済み。assistant content を `s.content` に蓄積し、会話履歴として返す。
provider 選択は `mods.go` の `startCompletionCmd()` にある `switch mod.API`。

- `anthropic` -> `anthropic.New(accfg)`
- `google` -> `google.New(gccfg)`
- その他 -> `openai.New(ccfg)`

`internal/ollama/`（`github.com/ollama/ollama/api` SDK 使用の専用実装）はPR#23で削除。Ollama もOpenAI互換の`/v1`エンドポイント経由で `default` ケースに統合された。動的な`num_ctx`指定という唯一の専用機能は失われたが、手書きgoroutine/channel実装（過去2回クラッシュ: PR#1-B/1-C）を丸ごと除去できた。詳細は `fix-roadmap.md` PR#23 参照。

`azure` と `azure-ad` は OpenAI client 用の config を変えて default path に流す。`perplexity` など OpenAI 互換 endpoint も、設定上の API 名は保持しつつ OpenAI client 実装を使う。

## model/API 解決

`mods.go` の `resolveModel(cfg)` が `cfg.APIs` を走査する。

- `--api` / `default-api` が指定されていれば、その API だけを見る。
- model 名そのもの、または `Model.Aliases` に一致すれば `cfg.Model` を canonical name に置き換える。
- 見つかった model に `Name` と `API` を埋めて返す。
- 見つからない場合は、API が指定されていればその API の available models を出す。API 未指定なら「settings に model がない」という error。

API error handling は `mods_errors.go`。

- 404 かつ `Model.Fallback` があれば fallback model にして retry。
- context length exceeded は `no-limit` が false なら prompt を短くして retry。
- rate limit や一部 server error は exponential backoff で retry。

## MCP と tool call

MCP 設定は `Config.MCPServers` / `MCPServerConfig` に入る。設定 YAML の `mcp-servers:` から読む。

`mcp.go` の役割。

- `enabledMCPs()` / `isMCPEnabled()` で有効 server を選ぶ。
- `mcpTools(ctx)` で enabled server から tool 一覧を並列取得する。
- `initMcpClient()` は `stdio`, `sse`, `http` の各 MCP client を作る。
- `toolCall(ctx, name, data)` は tool 名を `server_tool` として分解し、該当 server へ `CallTool` する。

`startCompletionCmd()` は `mcpTools(ctx)` の結果を `proto.Request.Tools` に入れ、`ToolCaller` に `toolCall` を渡す。provider の stream 実装は `CallTools()` 内で `internal/stream.CallTool()` を呼び、tool result を会話へ追加して stream を再開する。

## 保存会話

保存会話は metadata と本文が分かれている。

- metadata
  - `db.go`
  - SQLite DB: `config.CachePath/conversations/mods.db`
  - table: `conversations`
  - fields: `id`, `title`, `updated_at`, `model`, `api`
- 本文
  - `internal/cache/convo.go`
  - gob encoded `[]proto.Message`
  - `cache.Conversations` が `Read`, `Write`, `Delete` を提供する。

`Mods.findCacheOpsDetails()` が `--continue`, `--show`, `--title`, `--continue-last` などから read/write ID を決める。

## 外部ライブラリの利用状況

`go.mod` の direct dependencies を用途別に見ると、主な依存は以下。

### CLI / flag / help

- `github.com/spf13/cobra`
  - CLI command 本体、flag、help、completion の土台。
  - `main.go` の `rootCmd` と `initFlags()` で中心的に使われている。
- `github.com/spf13/pflag`
  - Cobra の flag 実装。独自 duration flag や flag error handling 周辺でも使う。
- `github.com/muesli/mango-cobra`, `github.com/muesli/roff`
  - `mods man` 用の manpage 生成。
  - CLI の主要実行経路ではなく、削減候補としては比較的切り出しやすい。

### TUI / terminal 表示

- `charm.land/bubbletea/v2`
  - `Mods` の event loop / state machine。
- `charm.land/bubbles/v2`
  - viewport など Bubble Tea 用 UI 部品。
- `charm.land/lipgloss/v2`
  - terminal styling。
- `charm.land/glamour/v2`
  - markdown rendering。
- `charm.land/huh/v2`
  - interactive prompt。
- `github.com/muesli/termenv`, `github.com/lucasb-eyer/go-colorful`
  - terminal color/profile や gradient 表示。
- `github.com/mattn/go-isatty`
  - stdin/stdout が TTY かどうかの判定。

この層は UX に深く関わるため、依存削減の難度は高め。

### LLM provider SDK / API client

- `github.com/openai/openai-go`
  - OpenAI 互換 API、Azure、Perplexity、Ollama（`/v1`互換エンドポイント、PR#23）などの request/streaming に使う。
- `github.com/anthropics/anthropic-sdk-go`
  - Anthropic Messages API に使う。

Google/Gemini は専用 SDK ではなく、`internal/google` で `net/http` による REST SSE を直接扱っている。

provider SDK は削れば依存削減効果が大きい一方、streaming、tool call、provider ごとの型差分を自前で持つことになるため、後回しが無難。

補足: Cohere provider は利用しない方針のため削除済み。

### MCP

- `github.com/mark3labs/mcp-go`
  - MCP client、tool schema、tool call に使う。
- `golang.org/x/sync`
  - `errgroup` による MCP tool 一覧取得の並列処理。

MCP support を残すなら中心依存。MCP 機能を optional にする設計なら削減対象になりうる。

### 設定 / ファイルパス / 環境変数

- `github.com/adrg/xdg`
  - XDG 準拠の config/data path 解決。
- `gopkg.in/yaml.v3`
  - `mods.yml` の読み書き。
- `github.com/caarlos0/env/v9`
  - `MODS_*` 環境変数を `Config` struct に反映。
- `github.com/caarlos0/duration`
  - `--delete-older-than` の `10d`, `1mo` などの duration parsing。
- `github.com/caarlos0/go-shellwords`
  - `api-key-cmd` の shell-like な引数分解。

この層は標準ライブラリで置き換えやすいものが多く、依存削減の最初の候補。

### DB / cache

- `github.com/jmoiron/sqlx`
  - SQLite query の helper。
- `modernc.org/sqlite`
  - pure Go SQLite driver。

`sqlx` は `database/sql` へ寄せられる可能性がある。`modernc.org/sqlite` を削る場合は SQLite 保存方式そのものの見直しが必要。

### その他

- `github.com/atotto/clipboard`
  - clipboard integration。
- `github.com/caarlos0/timea.go`
  - 相対時刻表示。
- `github.com/charmbracelet/x/editor`
  - `$EDITOR` 起動。
- `github.com/charmbracelet/x/exp/ordered`, `github.com/charmbracelet/x/exp/strings`
  - 小さな補助処理。
- `github.com/charmbracelet/x/exp/golden`, `github.com/stretchr/testify`
  - 主に test 用。

## 依存削減の着手順メモ

外部ライブラリ依存を減らすなら、リスクが低い順に以下から見るのがよさそう。

1. 小さな補助依存
   - `caarlos0/env`, `caarlos0/duration`, `caarlos0/go-shellwords`, `caarlos0/timea.go`, `adrg/xdg`
2. manpage 生成
   - `mango-cobra`, `roff`
3. DB helper
   - `sqlx` を `database/sql` に置き換える
4. provider SDK
   - OpenAI/Anthropic SDK を HTTP 直実装へ寄せる（Ollama 専用実装は PR#23 で削除済み。OpenAI互換パスに統合）
5. CLI/TUI の中核
   - Cobra や Bubble Tea 系。影響が大きいため最後に検討する

## 調査時の注意

`go list ./...` は package 構成確認のために試したが、この環境では依存 module が未取得で、さらに network が制限されているため完走しなかった。ただし出力上、package は以下であることは確認できた。

- `github.com/charmbracelet/mods`
- `github.com/charmbracelet/mods/internal/anthropic`
- `github.com/charmbracelet/mods/internal/cache`
- `github.com/charmbracelet/mods/internal/google`
- `github.com/charmbracelet/mods/internal/ollama`（当時の記録。PR#23で削除済み）
- `github.com/charmbracelet/mods/internal/openai`
- `github.com/charmbracelet/mods/internal/proto`
- `github.com/charmbracelet/mods/internal/stream`
