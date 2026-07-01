# JSON 出力モード 実装計画

作成日: 2026-07-01  
更新日: 2026-07-01（Codex レビューを一部反映: `--output` 命名、content ブロック配列化、conversation_id 追加）  
実装日: 2026-07-01（Phase 1 実装完了、PR#17）  
対象ブランチ: main（PR#16 `649e925` 以降）

---

## Phase 1 実装済み内容（2026-07-01）

- `config.go`: `Config.Output string`（デフォルト `"text"`。YAML `output:` / env `HENJI_OUTPUT` 対応）
- `main.go`: `--output` フラグ追加。`text`/`json` を許可、`jsonl` は未実装エラー、その他は不正値エラー
- `main.go`: `--output json` 時は bubbletea の入力キャプチャ・レンダラーを無効化（raw モードと同様の扱い）
- `main.go`: 出力分岐に `printJSONOutput(mods)` を追加。エラー時は `printJSONError(*mods.Error)` を stdout に出し、exit code は既存どおり 1（種別ごとの分岐は Phase 2）
- `mods.go`: `appendToOutput()` と `View()` の非TTYストリーミング分岐を `Config.Output == "json"` でガードし、JSON モード時は生テキストの直接 stdout 出力を抑制（バッファのみ）
- `json_output.go`（新規）: `JSONOutput` / `ContentBlock` / `ErrorInfo` 構造体 + `buildJSONOutput` / `printJSONOutput` / `printJSONError`
- `json_output_test.go`（新規）: `buildJSONOutput` の単体テスト
- 動作確認: モック SSE ストリーミングサーバーで実際に `--output json` の成功パス（`content`/`conversation_id`/`model` を含む単一 JSON 行）とエラーパス（`error` フィールド + exit 1）を確認済み。既存の `--output` 未指定（text）モードの挙動に変化がないことも確認済み
- `usage` フィールドはプロバイダ側に使用量取得の配線が一切ないため Phase 1 では実装せず（Phase 2 に据え置き）

---

## 1. 概要

`--output json` を追加し、AI→AI 連携・スクリプト向けに構造化 JSON で応答を出力するモードを設ける。

### 動機

- `mods` はパイプライン用途（`git diff | mods "explain"`）が中心
- downstream の LLM や処理スクリプトがテキストを parse する手間を省く
- MCPツール呼び出しの情報・使用トークン数をプログラムから取得できるようにする

### スコープ外（Codex 提案からの取捨選択）

Codex からエージェント間通信プロトコル（sender/recipient、delegate、policy validation、idempotency-key、capability negotiation 等）の提案があったが、**mods は「1リクエスト→1応答」の CLI ラッパーであり、複数エージェントがターンを取り合うオーケストレーターではない**ため、以下は対象外とする。これらはツール呼び出し元（MCP サーバーや呼び出し側のagentic framework）の責務であり、CLI 側が持つべき層ではない。

- sender / recipient フィールド、エージェント間の委任(delegate)フロー
- idempotency_key、ポリシー検証層、capability check
- schema/payload の名前空間分離（`schema.name` / `sha256` 等の契約管理）
- `--schema` によるプロバイダ guided generation 対応（別機能・別トラックで検討）

---

## 2. 設計方針（Opus レビュー採用 + Codex 一部反映）

### 2-1. 封筒スキーマを最初に確定・フィールドは段階的に充足

```json
{
  "version": 1,
  "conversation_id": "...",
  "content": [
    {"type": "text", "text": "..."}
  ],
  "model": "...",
  "finish_reason": "stop",
  "usage": {
    "prompt_tokens": 100,
    "completion_tokens": 200,
    "total_tokens": 300
  },
  "tool_calls": [...]
}
```

- `version` は整数型。将来の破壊的変更でインクリメント
- `content` は単一文字列でなく**型付きブロックの配列**にする（Codex提案採用）。Phase 1 は `{"type":"text","text":"..."}` のみ生成するが、将来 `tool_call` / `tool_result` / `artifact` 等のブロック種別を追加しても既存フィールドを壊さない
- `conversation_id` は mods 既存の会話継続キャッシュ ID をそのまま転用する（新規の会話管理機構は作らない）
- `tool_calls` は Phase 2 で充足（Phase 1 は `null` または省略）
- `usage` は各プロバイダの SDK から取得できる範囲で充足（未取得フィールドは省略）

### 2-2. ストリーミング: `--output json` は一括バッファ、`--output jsonl` は ndjson

| フラグ | 挙動 | 用途 |
|---|---|---|
| `--output json` | 完了後に JSON 1行を stdout へ | AI→AI 連携（完全オブジェクト要求） |
| `--output jsonl` | 各チャンクを ndjson で逐次出力 | ストリーミング処理・ログ |

`--output` は `text`（デフォルト） / `json` / `jsonl` の3値。Phase 1 は `text` と `json` のみ実装。`jsonl` は Phase 2。

既存の `--format` / `--format-as` とは別軸のフラグであり、`--json` という単一フラグ名は採用しない（Codex 指摘: 出力形式・構造化生成・エージェント間プロトコルを同じフラグ名に混ぜると後で苦しくなる）。

### 2-3. エラーは stdout の JSON に統合（エラー種別ごとに exit code を分ける）

```json
{
  "version": 1,
  "error": {
    "code": "model_not_found",
    "message": "model 'foo' is not available"
  }
}
```

- JSON モード時は、これまで stderr に書いていたエラーを stdout の `error` フィールドで出す
- stderr への診断ログは残す（`--quiet` で抑制可能）
- 正常終了: exit 0。エラー時は種別ごとに exit code を分ける（Codex 提案採用。呼び出し側スクリプトが `$?` だけで分岐できる）

| exit code | 意味 |
|---|---|
| 0 | 正常終了 |
| 1 | 一般エラー |
| 2 | CLI 引数エラー |
| 3 | 認証エラー |
| 4 | provider 利用不能 |
| 6 | rate limit |
| 7 | timeout |

Phase 1 では 0/1/2 を実装し、3/4/6/7 は既存エラーハンドリングと対応付けながら段階的に割り当てる。

### 2-4. TTY 自動判定は行わない

- `--output json` は **明示フラグ必須**（非 TTY 時の自動有効化は事故の元）
- ただし YAML 設定（`output: json`）でデフォルト化は可能にする

### 2-5. glamour レンダリングは `--output json` 時に無効化

JSON モードで glamour ANSI エスケープが混入しないよう、`appendToOutput()` の TUI 更新パスをスキップする。

---

## 3. 実装スコープ

### Phase 1（PR#17）: `--output json` 基本実装

#### 変更ファイル

| ファイル | 変更内容 |
|---|---|
| `config.go` | `Output string \`yaml:"output"\``フィールド追加（`text`/`json`/`jsonl`、デフォルト `text`） |
| `flag.go` | `--output` フラグ追加（`config.Output` に紐付け） |
| `mods.go` | `appendToOutput()` で JSON モード時は TUI 更新をスキップ |
| `mods.go` | `completionOutput` に usage フィールド追加（phase 1 は取得できるもの） |
| `main.go` | 出力分岐に JSON モードを追加（`json.Marshal` して stdout へ） |
| `mods_errors.go` | JSON モード時のエラー出力ヘルパー追加 |

#### 出力分岐（main.go の変更イメージ）

```go
switch {
case config.Output == "json":
    out := buildJSONOutput(mods)  // 新関数
    fmt.Println(string(out))
case isOutputTTY() && !config.Raw:
    // 既存パス: glamour レンダリング済み出力
    if mods.glamOutput != "" {
        fmt.Print(mods.glamOutput)
    } else if mods.Output != "" {
        fmt.Print(mods.Output)
    }
}
```

#### JSON 組み立て関数（新規: `json_output.go`）

```go
type JSONOutput struct {
    Version        int             `json:"version"`
    ConversationID string          `json:"conversation_id,omitempty"`
    Content        []ContentBlock  `json:"content,omitempty"`
    Model          string          `json:"model,omitempty"`
    FinishReason   string          `json:"finish_reason,omitempty"`
    Usage          *UsageInfo      `json:"usage,omitempty"`
    Error          *ErrorInfo      `json:"error,omitempty"`
}

type ContentBlock struct {
    Type string `json:"type"` // "text" のみ Phase 1。将来 "tool_call" / "tool_result" / "artifact" 等を追加
    Text string `json:"text,omitempty"`
}

type UsageInfo struct {
    PromptTokens     int `json:"prompt_tokens,omitempty"`
    CompletionTokens int `json:"completion_tokens,omitempty"`
    TotalTokens      int `json:"total_tokens,omitempty"`
}

type ErrorInfo struct {
    Code    string `json:"code"`
    Message string `json:"message"`
}
```

#### エラーパス（main.go / mods_errors.go の変更イメージ）

```go
// JSON モード時はエラーも stdout に JSON で出す
if config.Output == "json" {
    out, _ := json.Marshal(JSONOutput{Version: 1, Error: &ErrorInfo{...}})
    fmt.Println(string(out))
    os.Exit(exitCodeForError(err)) // 2/3/4/6/7 等、種別に応じて分岐
}
```

### Phase 2（後続 PR）: 拡張

- `tool_calls` フィールドの充足（MCP ツール呼び出し履歴）。`content` ブロックに `type: "tool_call"` / `type: "tool_result"` を追加する形で拡張
- `--output jsonl`（ndjson、`type` 判別子付き）
  - 各行: `{"type":"content_delta","delta":"..."}` / `{"type":"tool_call",...}` / `{"type":"response_end","finish_reason":"stop"}`
- `usage` フィールドの各プロバイダ対応（OpenAI / Ollama / Google / Anthropic）
- exit code 3/4/6/7 の割り当て（認証エラー・provider不可・rate limit・timeout）

---

## 4. 実装ポイント・注意事項

### appendToOutput() の JSON モード挙動

```go
func (m *Mods) appendToOutput(s string) {
    m.Output += s
    if !isOutputTTY() || m.Config.Raw || m.Config.Output == "json" {
        // JSON モード: バッファのみ（ストリームしない）
        if m.Config.Output != "json" {
            m.contentMutex.Lock()
            m.content = append(m.content, s)
            m.contentMutex.Unlock()
        }
        return
    }
    // ... 既存 TUI 更新パス
}
```

### proto.Request への usage 追加

各プロバイダ SDK から usage を取得するには `proto.Response`（または `completionOutput`）に usage フィールドが必要。Phase 1 では OpenAI 互換の usage だけ取得し、他は nil 許容。

### --format / --raw との組み合わせ

- `--output json` + `--format` → JSON モード優先（glamour は無効）
- `--output json` + `--raw` → JSON モード優先（raw テキストが `content[0].text` に入る動作は同一）
- `--output json` + `--quiet` → stderr 診断ログを抑制（stdout JSON は通常出力）

---

## 5. 実装順（PR#17）

```
Step 1: config.go に Output string フィールド追加（デフォルト "text"）
Step 2: flag.go に --output フラグ追加（text/json/jsonl のバリデーション付き）
Step 3: json_output.go 新規作成（JSONOutput / ContentBlock / UsageInfo / ErrorInfo 構造体 + buildJSONOutput 関数）
Step 4: mods.go appendToOutput() に JSON モードの早期 return 追加
Step 5: main.go 出力分岐に JSON ケース追加 + エラーパスの JSON 対応（exit code 分岐含む）
Step 6: 動作確認（echo "test" | ./henji --output json "summarize"）
Step 7: テスト追加（main_test.go に --output json のスモークテスト）
```

---

## 6. 将来検討（スキーマ v2 以降）

- `tool_calls[].duration_ms`: ツール呼び出し所要時間
- `metadata.mcp_servers`: 使用した MCP サーバ一覧
- `--output jsonl` の `content_delta` イベント設計の詳細化

スキーマ変更時は `version` をインクリメントし、消費側が `version` を見て切り替えられるようにする。

---

## 7. ロードマップへの位置づけ

`docs/notes/fix-roadmap.md` の PR#17 として追記予定（PR#16 `649e925` 完了後のフェーズ）。
