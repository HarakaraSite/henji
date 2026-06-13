# 潜在バグ調査メモ

mods を自分用に育てていく前提で、Issue に上がっているもの以外の潜在バグをコードレビューしたメモ。

確認時点では `go test ./...` は通っている。ただし provider 実装や MCP 周りはテストが薄いため、実行時にだけ出る問題が残っている可能性が高い。

## Fixed: Ollama streams never complete

場所:

- `internal/ollama/ollama.go`
  - `Stream.Next()`
  - `Stream.Current()`
- `mods.go`
  - `receiveCompletionStreamCmd()`

状況:

Ollama の `Stream.Next()` は `s.err != nil` 以外では常に `true` を返す。`s.done` が true の場合も、終了を知らせずに `factory()` で新しい request を始めてから `true` を返している。

一方、共通側の `receiveCompletionStreamCmd()` は `Next() == false` になった時だけ stream 完了扱いにして、`Err()`, `CallTools()`, `Messages()` へ進む。

影響:

Ollama 利用時に応答が終わっても CLI が完了せず、同じ request を再実行し続ける可能性がある。tool call がない通常応答でも終了経路に到達しない。

修正候補:

Ollama stream も OpenAI/Anthropic と同じように、通常完了時は一度 `Next() == false` を返して共通側に完了を知らせる。tool call がある場合だけ、`CallTools()` 後に再 request できる状態にする。

対応:

`Stream` に `finalized` state を追加し、完了 response の後はまず assistant message を保存して `Next() == false` を返すようにした。tool call 後の再 request では、`finalized` 済みの stream だけを restart する。

また、Ollama の streamed response から蓄積する assistant message に `Role` が入っていなかったため、`resp.Message.Role` も保持するようにした。

## Removed: Cohere streams never finish after EOF

場所:

- `internal/cohere/cohere.go`
  - `Stream.Current()`
  - `Stream.Next()`
- `mods.go`
  - `receiveCompletionStreamCmd()`

状況:

Cohere の `Current()` は `io.EOF` を受けると `stream.ErrNoContent` を返すが、`s.done = true` にしていない。`Next()` は `s.err != nil` 以外では `!s.done` を返すため、EOF 後も `true` のままになる。

共通側では `ErrNoContent` は非致命扱いなので、再度 stream を読み続ける。

影響:

Cohere 利用時に応答完了後、CLI が完了しない可能性がある。

修正候補:

EOF 時に `s.done = true` を設定する。可能なら EOF 後の `Next()` が false になることを unit test で固定する。

判断:

Cohere provider は使わない方針のため削除済み。このバグは削除によって解消した。

## Fixed: Google/Gemini の会話保存が空になる

場所:

- `internal/google/google.go`
  - `Stream.Messages()`
- `mods.go`
  - `receiveCompletionStreamCmd()`
- `main.go`
  - `saveConversation()`

状況:

以前は Google/Gemini の `Stream.Messages()` が常に `nil` を返していた。共通側は stream 完了時に `m.messages = msg.stream.Messages()` とするため、Google 利用時は会話履歴が空になっていた。

Issue #604 の修正で title は `Untitled conversation` に fallback するが、この時点では会話本文は保存されなかった。

影響:

Google/Gemini で `--title`, continue, show など保存会話機能を使うと、保存済み会話の中身が空になる可能性があった。

修正:

Google stream 側で request messages と assistant response を保持し、`Messages()` が `[]proto.Message` を返すようにした。stream chunks を蓄積して assistant message を作る最小修正。

## Removed: Cohere の custom base-url が無視される

場所:

- `mods.go`
  - Cohere config 作成部分

状況:

Cohere branch で `api.BaseURL` がある場合、`cccfg.BaseURL` ではなく OpenAI 用の `ccfg.BaseURL` に代入している。

```go
cccfg = cohere.DefaultConfig(key)
if api.BaseURL != "" {
	ccfg.BaseURL = api.BaseURL
}
```

影響:

設定ファイルに Cohere 用 `base-url` を書いても `cohere.New(cccfg)` に反映されない。Cohere 互換 endpoint や proxy gateway を使えない。

修正候補:

代入先を `cccfg.BaseURL` に直す。小さな typo 修正で済む。

判断:

Cohere provider は使わない方針のため削除済み。この typo は削除によって解消した。

## Fixed: `--http-proxy` が Google/Gemini に効かない

場所:

- `mods.go`
  - HTTP proxy 設定部分

状況:

以前は `--http-proxy` 指定時に作った `httpClient` が OpenAI, Anthropic, Ollama config には入るが、Google config には入っていなかった。

影響:

proxy 必須環境で Google/Gemini provider だけ直通接続しようとして失敗する可能性があった。

修正:

proxy 設定時に `gccfg.HTTPClient = httpClient` も追加した。

## P2: MCP tool call の timeout が実 call に効かない

場所:

- `mcp.go`
  - `toolCall()`

状況:

`toolCall(ctx, name, data)` は引数で `ctx` を受け取り、MCP client の初期化には使っている。しかし実際の tool 実行は以下のように `context.Background()` を渡している。

```go
result, err := client.CallTool(context.Background(), request)
```

影響:

`mcp-timeout` が tool 実行本体に効かない。遅い/固まった tool があると CLI 全体が長時間待たされる可能性がある。

修正候補:

`client.CallTool(ctx, request)` に変更する。timeout/cancel の挙動を fake MCP client か integration-ish test で確認したい。

## Fixed: 会話削除が cache 欠損で中途半端になる

場所:

- `main.go`
  - `deleteConversationOlderThan()`
  - `deleteConversation()`
- `internal/cache/cache.go`
  - `Cache.Delete()`

状況:

以前の削除処理は DB を先に削除し、その後 cache file を削除していた。cache file が存在しない場合でも `Cache.Delete()` は `os.Remove` の error を返していた。

影響:

DB と cache が既に不整合な状態で削除すると、DB だけ消えた後に error になっていた。複数削除では途中で停止し、ユーザーには「削除失敗」と見えるが一部は削除済みになる可能性があった。

修正:

`Cache.Delete()` を idempotent にし、cache file missing を成功扱いにした。DB 側の delete も存在しない ID を成功扱いにしているため、削除操作の性質が揃う。

## P3: `loadMsg` の URL 読み込みに timeout/status check がない

場所:

- `load.go`
  - `loadMsg()`

状況:

`http.Get` を直接使っており、timeout がない。HTTP status も確認せず body を読む。

影響:

role に URL を指定したとき、接続先が応答しないと CLI が長時間止まる可能性がある。また 404/500 の HTML や error body を system prompt として取り込む可能性がある。

修正候補:

timeout 付き `http.Client` を使い、2xx 以外は error にする。既存仕様として redirect は Go default のままでよさそう。

## 着手順案

1. MCP tool call timeout
2. `loadMsg` の timeout/status check

まずは provider stream の終了契約を unit test で固定するとよい。`stream.Stream` interface の期待動作が provider ごとにずれているのが、いちばん大きな不安定要因に見える。
