# mods 機能要件メモ

このメモは、README だけでなく現在のコードから読み取れる mods の機能要件を整理したもの。今後、自分用に機能を削る/直す/作り替える際の基準にする。

前提: このメモは Cohere provider 削除後の状態を基準にしている。

## 1. 基本コンセプト

mods は「CLI の引数 prompt」と「stdin から渡された入力」を LLM に送り、結果を terminal / stdout に返す CLI。

主な利用形態:

- `mods "質問"` のように引数だけで prompt を送る。
- `cat file | mods "要約して"` のように stdin と引数 prompt を組み合わせる。
- `command | mods -f "JSON にして" | jq` のように pipeline の途中で使う。
- 会話を保存し、あとから `--continue`, `--show`, `--list`, `--delete` で扱う。

## 2. 入力要件

### 引数 prompt

- root command の positional args は空白で join され、`Config.Prefix` になる。
- join 後、空白だけなら空文字に正規化される。
- 引数 prompt は stdin 入力の前に付与される。

### stdin

- stdin が TTY でない場合、全内容を読み込む。
- stdin 内容は `increaseIndent()` により各行の先頭に tab が付く。
- prompt 組み立て時には `Prefix + "\n\n" + stdin content` の形で結合される。

### editor 入力

- `--editor` / `-e` は、引数なし・stdin が TTY の場合だけ有効。
- 一時ファイルを `$EDITOR` で開き、その内容を prompt として使う。

### interactive prompt

- 引数なし、または `--ask-model` 指定時、かつ stdin が TTY の場合は `huh` の form を表示する。
- model/API が config から解決できていて `--ask-model` が false の場合、API/model 選択は隠れる。
- prompt が既にある場合、prompt 入力欄は隠れる。

### 入力なしエラー

- 通常実行で prompt も stdin もなく、管理系 flag でもない場合は「prompt input がない」エラーにする。

## 3. 出力要件

### TTY 出力

- stdout が TTY かつ `--raw` でない場合、LLM 応答は Glamour で markdown render される。
- 生成中 animation は stderr に出る。
- viewport が必要な高さの場合、Bubble Tea viewport で表示する。

### pipeline 出力

- stdout が TTY でない、または `--raw` の場合、rendering は無効化され、stream chunk を stdout に逐次出す。
- raw mode では「raw mode already prints the output」として最後の再出力はしない。

### quiet mode

- `--quiet` / `-q` は spinner や保存成功メッセージなどを抑制する。
- `VIMRUNTIME` がある場合も quiet mode になる。

### prompt echo

- `--prompt-args` / `-p` は引数 prompt を応答前に出力へ含める。
- `--prompt` / `-P` は stdin prompt の先頭 N 行を応答前に出力へ含める。
- `-P` は値なし指定時に `-1` になる。
- `-P` 値なし、つまり `IncludePrompt == -1` は stdin prompt 全体を応答前に出力へ含める。

## 4. formatting 要件

### `--format`

- `--format` / `-f` が有効な場合、`format-text[format-as]` を system message として先頭に追加する。
- `--format-as` 未指定で `--format` が true の場合、`markdown` になる。
- `format-text` は YAML 上で旧形式の文字列、または format 名ごとの map を受け付ける。
- OpenAI provider で `--format --format-as json` の場合は `ResponseFormat=json` も request に設定する。

### role

- `--role` / `-R` で `roles` に定義された system prompt 群を追加する。
- role が存在しない場合はエラー。
- role の各文字列は `loadMsg()` を通す。
  - 通常文字列はそのまま。
  - `file://` はファイル内容を読む。
  - `http://` / `https://` は URL body を読む。

## 5. provider / model 要件

### provider

現在の専用 provider:

- OpenAI / OpenAI compatible endpoint
- Azure OpenAI / Azure AD
- Anthropic
- Google/Gemini
- Ollama

OpenAI compatible endpoint は default branch で `internal/openai` を使う。設定上は Groq, Perplexity, LocalAI, DeepSeek, GitHub Models などを `apis` に定義できる。

Cohere provider は使わない方針のため削除済み。

方針:

- OpenAI compatible endpoint と専用 provider の境界は現状維持。
- Ollama は将来的に OpenAI compatible に寄せる可能性があるが、現時点では専用 provider を維持する。

### model 解決

- `Config.APIs` を順に走査する。
- `--api` / `default-api` が指定されている場合、その API だけを見る。
- model 名そのもの、または model の `aliases` に一致すれば、その canonical model 名に置き換える。
- 見つかった model には `Name` と `API` が埋められる。
- model が見つからない場合:
  - API 指定ありなら、その API の available models を示すエラー。
  - API 指定なしなら、settings に model がない旨のエラー。

### API key 解決

認証 key は次の順に探す。

1. `api-key`
2. `api-key-env`
3. `api-key-cmd`
4. provider ごとの default env

default env:

- OpenAI compatible: `OPENAI_API_KEY`
- Anthropic: `ANTHROPIC_API_KEY`
- Google: `GOOGLE_API_KEY`
- Azure: `AZURE_OPENAI_KEY`

`api-key-cmd` は shell-like に parse され、command output を key として使う。

### generation params

request に渡す主な値:

- `temperature`
- `top_p`
- `top_k`
- `stop`
- `max_tokens`
- `user`
- provider tools

`temperature`, `top_p`, `top_k` は負値なら nil として送らない。

`o1` prefix の model は `max_tokens` を送らない。

`MaxCompletionTokens` は Config にあるが、現状 request 組み立てでは使われていない。

方針: 削除影響がまだ判断しづらいため、現時点では保留。設定項目として存在するが効いていない可能性が高いので、後で互換性と provider API の扱いを確認してから削除/実装を決める。

### HTTP proxy

- `--http-proxy` / `MODS_HTTP_PROXY` があれば proxy URL を parse し、OpenAI / Anthropic / Ollama / Google 用 HTTP client に設定する。

## 6. conversation / cache 要件

### 保存の単位

保存会話は metadata と本文が分かれる。

- metadata: SQLite `conversations` table
- 本文: gob encoded `[]proto.Message` cache

DB fields:

- `id`
- `title`
- `updated_at`
- `model`
- `api`

`created_at` はない。古さ判定も一覧表示も `updated_at` 基準。

### 保存タイミング

- 通常 request 完了後、`cacheWriteToID` があれば会話を保存する。
- `--no-cache` / `NO_CACHE` が true の場合は保存しない。
- 保存時は先に cache 本文を書き、その後 DB metadata を保存する。
- DB 保存に失敗した場合、cache 本文は削除する。

### title 生成

- `--title` 指定があればそれを title とする。
- title が空、または SHA-1 形式なら、最後の user prompt の先頭行を title にする。
- それでも空なら `Untitled conversation` に fallback する。

### ID

- 新規会話 ID は SHA-1 文字列。
- show/delete/continue は title または ID prefix で検索できる。
- ID prefix は短すぎる場合、title exact match のみになる。
- ID prefix が複数 match した場合はエラー。

## 7. conversation 操作要件

### list

- `--list` / `-l` は保存済み会話を `updated_at DESC` で一覧する。
- stdin/stdout が TTY かつ raw でなければ interactive select を出す。
- interactive select で選んだ conversation ID は clipboard / terminal clipboard にコピーされる。
- raw または非 TTY では tab-separated 形式で出力する。

### show

- `--show` / `-s` は保存会話を cache から読み、`proto.Conversation.String()` で表示する。
- `--show-last` / `-S` は最新会話を対象にする。
- show は新しい LLM request を行わない。

### continue

- `--continue` / `-c` は指定 title/ID、または title なしの場合は最新会話を読み込んで続ける。
- `--continue-last` / `-C` は最新会話を読み込んで続ける。
- continue 時は保存済み会話の `api` / `model` が DB にあれば引き継ぐ。
- `--continue` 単独は既存 conversation を上書き保存する。
- `--continue` + `--title` は branch として新しい conversation ID に保存する。

### delete

- `--delete` / `-d` は title/ID で会話を探し、DB と cache から削除する。
- flag は複数回指定できる。
- 削除は不可逆。

### delete older than

- `--delete-older-than` は duration parser が認識する期間を受け取り、`updated_at` がそれより古い会話を削除対象にする。
- quiet でない場合:
  - 対象を一覧表示する。
  - stdin/stdout が TTY でなければ削除せず、`--quiet` を付けて再実行する案内を出す。
  - TTY なら confirm prompt を出す。
- quiet の場合は confirm なしで削除する。

## 8. 設定管理要件

### 設定ファイル

- 設定ファイルは XDG config path の `mods/mods.yml`。
- 初回起動時に `config_template.yml` から生成する。
- 設定ディレクトリは `0700` で作る。

### 読み込み優先度

実効値の優先度はおおむね以下。

1. default / config file
2. `MODS_` prefix の環境変数
3. CLI flags

### settings

- `--settings` は `$EDITOR` で settings file を開く。
- `--reset-settings` は現在の settings を `.bak` に copy し、元ファイルを消して default を再生成する。
- `--dirs` は設定/cache path を表示する。
  - `mods --dirs config` は config dir だけ。
  - `mods --dirs cache` は cache path だけ。

## 9. MCP 要件

### 設定

`mcp-servers` は server 名ごとの map。

server type:

- `stdio` または空
- `sse`
- `http`

stdio は `command`, `env`, `args` を使う。sse/http は `url` を使う。

### list

- `--mcp-list` は configured server 名を表示し、有効なものに `(enabled)` を付ける。
- `--mcp-list-tools` は有効 server から tools を取得し、`server > tool` 形式で表示する。
- `--mcp-disable` は server 名、または `*` で無効化できる。

### request integration

- LLM request 前に有効 MCP server の tools を取得する。
- tools は provider request に渡される。
- tool 名は `server_tool` 形式に変換される。
- provider stream から tool call が返った場合、MCP tool を実行し、その結果を tool message として会話に追加して request を継続する。
- timeout は `mcp-timeout`。ただし tool call 実行本体には timeout が効いていない潜在バグがある。

方針:

- MCP は残す。
- ローカル LLM 使用時に MCP で機能を補う構想があるため、中核機能寄りとして扱う。
- 次の改善候補は tool call 実行本体への `mcp-timeout` 適用。

## 10. CLI 補助機能

### help/version/completion/man

- `--help` / `-h` は custom usage を表示する。
- `--version` / `-v` は version を表示する。
- shell completion は Cobra default completion command を隠し command 追加で有効化する。
- `mods man` は manpage を生成する隠し command。

### theme

interactive form theme:

- `charm`
- `catppuccin`
- `dracula`
- `base16`

未知の theme は `charm` 扱い。

### memprofile

- hidden flag `--memprofile` があり、CWD に `mods_heap.profile` と `mods_allocs.profile` を書く。

## 11. エラー / retry 要件

### API error handling

OpenAI SDK の error は status code ごとに扱う。

- 404:
  - model fallback があれば fallback model にして retry。
  - fallback がなければ model missing error。
- 400 `context_length_exceeded`:
  - `--no-limit` が false なら prompt を短くして retry。
  - `--no-limit` が true なら error。
- 401:
  - invalid API key error。
- 429:
  - exponential backoff で retry。
- 500:
  - OpenAI API なら retry。
  - その他 API なら model loading error。
- その他:
  - retry。

### retry

- retry 回数は `max-retries`。
- wait は `100ms * 2^retries`。

## 12. 今後の確認ポイントと方針

仕様として明確化したい点と、現時点の判断:

- `MaxCompletionTokens`
  - 保留。
  - `Config` と `config_template.yml` にはあるが、request では使われていない。
  - 削除影響が文面だけでは判断しづらいため、互換性確認後に削除/実装を決める。
- Google/Gemini の会話保存
  - 修正済み。
  - `Stream.Messages()` は request messages と stream で受け取った assistant response を返す。
- `--prompt` の no-arg `-1`
  - 仕様として活かす。修正済み。
  - `-P` 値なしは stdin prompt 全体を応答前に出力へ含める。
- `system` config field
  - 削除方向。
  - 使われていないため、参照確認後に `Config.System` を削る。
- OpenAI compatible endpoint と専用 provider の境界
  - 現状維持。
  - Ollama は将来的に OpenAI compatible に寄せる可能性あり。
- MCP
  - 残す。
  - ローカル LLM を MCP で補強する構想があるため、optional 削除ではなく改善対象にする。
