# `henji docs` 内容案（ドラフト）

作成日: 2026-07-04（同日改訂: エージェント視点のギャップを実機検証の上で補強）
状態: 内容案のみ、未実装（`docs/notes/ai-docs-plan.md` の設計議論に基づく)

## このドラフトの前提（実装時の約束事）

`ai-docs-plan.md` で決めた/推奨された制約を再掲する:

- **役割分担**: `--help` は「フラグ一覧 + 1行説明」のクイックリファレンス。
  `henji docs` は「タスク指向の説明と落とし穴」の詳細マニュアル。
  同じ情報を二箇所に書かない（フラグの1行説明はこのドラフトに含めない）
- **出力はプレーンMarkdown**: glamour レンダリングや ANSI 装飾はしない
  （AIエージェントにとってノイズ）
- **バージョン行はコードが自動挿入**: 下記ドラフト先頭の `henji <version> — manual`
  行は `ldflags` の Version から実行時に生成する（ドラフトにはプレースホルダで記載）
- **配置**: `internal/docs/docs.md` + 同居する埋め込み用 Go ファイル
  （`//go:embed` はパッケージ外を参照できないため。`config_template.yml` と同じパターン）
- **検証テスト**: ドキュメント中に登場するフラグ名が実在することを機械検証するテストを
  併設する（陳腐化の自動検出）
- **発見可能性**: `--help` 末尾（Examples の後）に誘導行を追加する作業とセット。
  さらに `usageFunc` はサブコマンド一覧を表示しないため、`docs` サブコマンドを
  出す「Commands:」セクションの追加も前提タスク（ai-docs-plan.md 4.5）
- **トピック分割**（`henji docs mcp` 等）は初版では作らない。単一ページで様子見し、
  下記の `##` 見出しをそのまま将来のトピック境界とする
- **サイズ予算**: 本文は現在 約8.6KB ≈ 2,200トークン（`--help` の約2倍、一般的な
  エージェントのコンテキストウィンドウの約1%）。読むのは opt-in で、誤コマンド
  1回の失敗ラウンドトリップ（1〜3kトークン）を防げば元が取れるため、この規模なら
  コストは問題にならないと評価済み（2026-07-04）。ただし追記で肥大化しやすいので
  **予算上限を12KB（≈3kトークン）とし、超えたらトピック分割を実施**する。
  上限はフラグ実在検証テストと同じテストファイルでサイズ検査として機械化する

内容の情報源: README / docs/cookbook.md / fix-roadmap.md の検証メモ /
--help 改善時のエージェント検証（opencode・codex）で判明した誤解ポイント。
PR#26 のフラグ削減（43→29項目、temp 等は YAML/env 専用化）を反映済み。

**実機検証（2026-07-04、ローカル mlx-lm ゲートウェイ）で確認済みの記述**:
引数+stdin の結合規則（`stream.go`: `TrimSpace(args + "\n\n" + stdin)`）、
stdout/stderr の分離、`--json-schema` と `--output json` の併用（エンベロープの
`content[0].text` に検証済みJSONが文字列で入る）、`--list-models --output json` の
出力形、非力なローカルモデルが schema を無視した際の失敗モード（リトライ枯渇で
exit 1）、temp/topp 未設定の最小構成でゲートウェイが 422 を返すケース。

---

以下、`henji docs` が出力する本文の案。

```markdown
# henji <version> — manual

henji is an LLM client for the command line, built for pipelines. This
manual is task-oriented and lists the pitfalls behind each task. For the
one-line flag reference, run `henji -h`.

## Invocation basics

The prompt is assembled from arguments and stdin, in that order:

    henji "explain this error"                # args only
    cat error.log | henji                     # stdin only
    cat error.log | henji "what went wrong?"  # args first, then stdin,
                                              # joined by a blank line

Output contract — what lands where:

- **stdout carries only the model's response** (or the JSON envelope with
  `--output json`). It is always safe to pipe or capture.
- Progress spinners, "Conversation saved" notices, and error details go to
  **stderr**. `-q` silences the non-error stderr chatter.
- When stdout is not a terminal, the response is plain text with no ANSI
  codes; markdown rendering only happens on a TTY (`-r` disables it there
  too).
- Exit status is `0` on success, non-zero on any failure. With
  `--output json`, failures additionally print an error envelope on
  stdout (see below).
- Running with no prompt at all (no args, empty stdin) is an error;
  scripts never hang waiting for input.

## Choosing a provider and model

Models and providers ("APIs") are defined in the config file
(`henji --settings` to edit). To see what's configured:

    henji --list-models
    henji --list-models --output json   # {"version":1,"apis":[{"name":...,
                                        #  "models":[{"id":...,"aliases":[...]}]}]}

- `-m <model>` accepts the model name or any alias from the config.
- `-a <api>` selects the provider. The configured `default-model` belongs
  to one specific API, so **always pair `-a` with `-m`**; `-a` alone will
  usually fail with "missing model".
- A model entry may declare `fallback:`; henji retries with the fallback
  when the API returns 404 for the model.
- `anthropic` and `google` API entries use those providers' native
  protocols; every other entry speaks the OpenAI-compatible protocol.

Local gateways (Ollama, mlx-lm, LM Studio, llama.cpp server, ...):

- Use their OpenAI-compatible endpoint and **include `/v1` in `base-url`**
  (e.g. `http://localhost:11434/v1` for Ollama). Without `/v1`, requests
  404.
- henji always sends an API key on the OpenAI-compatible path. Local
  servers don't check it, so set `api-key` to any placeholder.
- If a gateway rejects requests with `422 Unprocessable Entity`, set
  `temp:` and `topp:` explicitly in the config file; with a minimal config
  that omits them, henji sends zero values, which some gateways refuse.

## Getting machine-readable output (scripts and agents)

Three levels, from loosest to strictest:

1. `--format` / `--format-as json` only *asks* the model for JSON. No
   guarantee the output parses. Avoid for scripting.
2. `--output json` wraps whatever the model said in a reliable one-line
   envelope on stdout:

       # success
       {"version":1,"conversation_id":"<sha1>","content":[{"type":"text","text":"..."}],"model":"..."}
       # failure (exit code 1; partial content is included if any arrived)
       {"version":1,"error":{"code":"...","message":"..."}}

       git diff | henji --output json "suggest a commit message" | jq -r '.content[0].text'

3. `--json-schema <file>` constrains the *answer itself* to a JSON Schema
   via the provider's native structured output, then validates the
   response client-side before printing. On validation failure henji
   tells the model what was wrong and retries (`--json-schema-retries`,
   default 2).

       henji --json-schema review.json "review this diff" < diff.patch | jq '.findings[]'

   `--json-schema` and `--output json` compose: the validated JSON is
   delivered as a string inside the envelope's `content[0].text`.

Pitfalls for `--json-schema`:

- Google's dialect is an OpenAPI 3.0 subset: `additionalProperties` is
  rejected outright (400), and `$ref`/`oneOf`-heavy schemas are likely to
  fail. Keep schemas flat and simple when targeting Google.
- Only real OpenAI enforces `strict: true`. Other OpenAI-compatible
  backends (Groq, Azure, local gateways) receive the schema without
  `strict`; client-side validation still catches violations.
- Small local models may ignore the schema and wrap JSON in markdown code
  fences. Each retry explains the failure to the model, but a model that
  keeps doing it exhausts the retries and henji exits 1 with
  "Response did not match --json-schema". Adding "raw JSON only, no code
  fences" to the prompt helps weak models comply.
- Live streaming is suppressed: output appears only once, after a
  response has passed validation.

## A typical agent loop

    # 1. discover what's available
    henji --list-models --output json
    # 2. run a task, capture the conversation id
    id=$(git diff | henji --output json "review this diff" | jq -r .conversation_id)
    # 3. follow up in the same conversation
    henji --output json -c "$id" "now suggest fixes for finding 1"
    # 4. before relying on tools, check MCP is actually configured
    henji --mcp-list-tools

## Conversations

Every run is saved automatically (metadata in SQLite, body on disk)
unless `--no-cache` is set.

- `-l` lists saved conversations; `-t <title>` names one at save time.
- `-C` continues the most recent conversation; `-c <id-or-title>`
  continues a specific one. IDs can be abbreviated (unique SHA-1 prefix).
- `-s <id-or-title>` prints a saved conversation without calling the API.
- `-d <id-or-title>` deletes.

Pitfalls:

- **Continuing a conversation resends the entire history every turn.**
  All supported APIs are stateless, so cost grows roughly quadratically
  with the number of turns. Start fresh when prior context isn't needed.
- **`--max-tokens` set too low cuts the answer off mid-sentence.** There
  is no warning; the response just ends. You can recover the rest with
  `-C`, but if you find yourself doing that, raise `--max-tokens`
  instead.
- Reasoning models (o1-style, and several OpenRouter-hosted models) spend
  hidden "thinking" tokens first. If the per-model `max-completion-tokens`
  in the config is too small, the visible answer comes back **empty**.
  Raise it for that model in the config file.

## MCP tools (agentic tool calls)

henji can let the model call tools from MCP servers defined under
`mcp-servers:` in the config file.

- **Tools exist only if MCP servers are configured.** `--max-tool-calls`
  does nothing on its own. Check first: `henji --mcp-list` (servers),
  `henji --mcp-list-tools` (tools).
- Set `--max-tool-calls` (or `max-tool-calls:` in the config) when using
  local MCP servers; `0` means unlimited rounds and a confused model can
  loop. `10`–`50` is a reasonable range.

      henji --max-tool-calls 10 "list the largest files in this project"

- `--mcp-disable <name>` turns a server off for one run.

## Roles (system prompts)

Define named system prompts under `roles:` in the config file:

    roles:
      shell:
        - you are a shell expert
        - you only output one-liners, no explanation

Use with `-R shell`. List with `--list-roles`. Each role line may also be
a `http(s)://` or `file://` URL; the fetched content becomes the system
prompt text.

## Configuration and tuning knobs

- Config file: `$XDG_CONFIG_HOME/henji/henji.yml`, default
  `~/.config/henji/henji.yml` (`henji --settings` opens it in `$EDITOR`).
- Data (conversations): `$XDG_DATA_HOME/henji/`, default
  `~/.local/share/henji/`.
- Every config key can be overridden per run with a `HENJI_`-prefixed
  environment variable: `HENJI_TEMP=0.2`, `HENJI_MAX_TOKENS=4000`, etc.

Some knobs are config/env-only and have **no flag** on purpose (they
rarely change between runs): sampling parameters (`temp`, `topp`, `topk`,
`stop`), `max-retries`, `word-wrap`, `http-proxy`.

Input size: `max-input-chars` (global or per-model) truncates the
combined prompt (args + stdin) before sending; the tail is dropped
silently. `--no-limit` disables the truncation. If a long prompt seems to
lose its ending, check these.

API keys, in priority order (highest wins):

1. `api-key-cmd` — a command whose stdout is the key (recommended; keys
   never touch disk). It is executed directly, **not through a shell**:
   `$USER` or `$(...)` will not expand.
2. `api-key-env` — a named environment variable.
3. `api-key` — plaintext in the config file (kept at mode 0600).
4. Provider default env var (`OPENAI_API_KEY`, `ANTHROPIC_API_KEY`,
   `GOOGLE_API_KEY`, ...).
```

---

## 実装時の残タスク（このドラフトの外側）

1. `docs` サブコマンド追加 + `usageFunc` に Commands セクション追加（ai-docs-plan.md 4.5）
2. `--help` 末尾に誘導行（例: `Full manual: henji docs`）
3. `internal/docs/` に本文を移して `//go:embed`、バージョン行の動的挿入
4. ドキュメント内フラグ名の実在検証テスト + サイズ予算検査（12KB上限）
5. opencode/codex による2段階検証（誘導に気づくか / 内容でタスク完遂できるか、
   ai-docs-plan.md 4.2）

## 検証で発見した、docs とは別軸の課題（要トリアージ）

- **temp/topp 未設定（=0）の最小構成で、henji が 0 値をそのまま送る問題**:
  vmlx-engine ゲートウェイは 422 Unprocessable Entity を返した。旧 2-D（`ptrOrNil`
  の 0 値扱い）は「config_template.yml が正値を設定しているから実害なし」として
  廃止したが、テンプレートを使わない最小構成では前提が成り立たない。docs には
  回避策（temp/topp を明示設定）を記載したが、根本対応（0 を「未指定」として
  省略する）を再検討する価値がある → fix-roadmap.md の再開候補
