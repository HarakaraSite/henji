# henji CLI（mods フォーク）

パイプラインのために作られた、コマンドラインの AI。

*henji CLI* は日本語の「返事」に由来する名前です。これは 2026 年 3 月 9 日にアーカイブされた
[charmbracelet/mods](https://github.com/charmbracelet/mods) の、現在も保守しているフォークです。
ローカル LLM の利用と、henji CLI を Unix フィルターとして扱うことを重視します。すなわち
`stdin → LLM → stdout` として、シェルの他のパイプラインと組み合わせます。henji CLI 自身は
入力フォームやモデル選択 UI を開きません。プロンプトは引数、`--text`、stdin で渡し、必要に
応じて `--model` / `--api` で設定済みモデルを選択します。

[English README](README.md)

## upstream からの主な変更

### バグ修正

| 対象 | 修正内容 |
|---|---|
| OpenAI | choices のない応答での panic |
| Ollama | ストリーム取消時のチャネルリークとデッドロック |
| Google | リクエスト失敗時の nil panic と response body リーク |
| 全プロバイダー | `cancelRequest` goroutine リークを `defer cancel` に置換 |
| OpenAI 互換 API | o1 系で `max-completion-tokens` を API へ正しく渡すよう修正 |
| 全プロバイダー | API キーを `api-key-cmd`、`api-key-env`、`api-key`、プロバイダー既定環境変数の順で解決 |

### セキュリティ

- `henji.yml` は `0600` で作成します。既存ファイルの権限が緩い場合は読む前に `0600` へ制限し、別ユーザー所有なら拒否します（Unix のみ。Windows は未対応）。
- Google API キーを URL クエリではなく `x-goog-api-key` ヘッダーで渡します。
- 設定は既定でリポジトリ外（macOS/Linux では `~/.config/henji/henji.yml`）に保存します。`*.bak` は Git の対象外ですが、リポジトリ内にコピーした `henji.yml` は自動では除外されません。

### 依存関係と削除した機能

依存バージョンは [`go.mod`](go.mod) と `go.sum` に固定し、セキュリティ更新を含めて保守します。
ネイティブ Ollama クライアントは削除し、OpenAI 互換の経路（`base-url: http://localhost:11434/v1`）を使います。
Azure OpenAI と Azure AD の対応も削除済みで、`azure` / `azure-ad` の API 設定はエラーになります。
未使用の UI・プロンプト表示・スピナー調整フラグは削除しました。`--temp`、`--topp`、
`--topk`、`--stop`、`--max-retries`、`--word-wrap`、`--http-proxy` は `henji.yml` または
`HENJI_*` 環境変数で引き続き設定できます。

通常の端末表示では、生成完了後に `PATH` 上の外部 `glow` でMarkdownを整形します。
Glowは任意で、未インストール・実行失敗時は元のMarkdownを表示します。`word-wrap` は
整形幅、`GLAMOUR_STYLE` はスタイル（既定 `auto`）を指定します。ページャとTUIは起動しません。
パイプ・`--raw` の逐次出力、およびJSON・JSON Schema・`henji decision` の出力はGlowを使いません。
内蔵Markdownレンダラーを外し、繰り返し起動時の初期化処理を減らしています。

MCP（Model Context Protocol）対応も完全に削除しました。信頼できない文章を処理した際に、
モデルが外部ツールを承認・読み書きの区別なく呼び出せる設計には実害のあるリスクがありました。
henji CLI は通常の Unix フィルターに戻し、ファイルやネットワークへのアクセスは周囲の `cat`、
`curl`、`find` などのシェルコマンドが担います。

## インストール

### リリースバイナリ

[Forgejo Releases](https://forge.harakara.site/littleisland/henji/releases) から、環境に合う
バイナリをダウンロードできます。タグリリースでは Linux・macOS の amd64/arm64 と、Windows の
amd64 を配布します。`henji`（Windows は `henji.exe`）へ名前を変更し、`PATH` の通った
ディレクトリに配置してください。macOS/Linux では `chmod +x henji` で実行権限を付けます。

v2.1.10 以降は `SHA256SUMS` も配布します。バイナリと一緒にダウンロードし、名前を変更する前に
チェックサムを確認してください。Linux では次を実行します。

```sh
sha256sum --ignore-missing --check SHA256SUMS
```

macOS では `shasum -a 256 <binary>`、Windows では PowerShell の
`Get-FileHash <binary> -Algorithm SHA256` でハッシュを求め、`SHA256SUMS` の該当ファイル名の
値と比較してください。

### ソースからビルド

[`go.mod`](go.mod) の指定に従い、Go 1.26 以降を使います。

```sh
git clone https://forge.harakara.site/littleisland/henji.git
cd henji
go build -o henji .
```

### シェル補完

```sh
henji completion bash -h
henji completion zsh -h
henji completion fish -h
henji completion powershell -h
```

## 推奨設定

設定例、Keychain の API キー、スクリプト用の `--output json`、推論モデルの注意点は
[Cookbook](docs/cookbook.ja.md) を参照してください（[English](docs/cookbook.md)）。

### ローカル LLM（Ollama / mlx-lm）

ローカルの OpenAI 互換エンドポイントを指定します。henji CLI は常に API キーを送るため、サーバーが
検証しない場合も任意のプレースホルダーを設定してください。

```yaml
# ~/.config/henji/henji.yml
apis:
  local:
    base-url: http://localhost:11434/v1
    api-key: local
    models:
      llama3.2:
        aliases: ["llama"]
        max-input-chars: 32000

default-api: local
default-model: llama3.2
```

```sh
echo "explain this error" | henji
ls -la | henji summarize these files
```

### API キー管理

最初の空でないキーを、`api-key-cmd`、`api-key-env`、`api-key`、プロバイダー既定の環境変数の
順で採用します。`api-key-cmd` はシェルを介さず直接実行するため、`$USER` や `$(whoami)` は
展開されません。stdout をキーとして取得し、stderr はキーに含めません。
コマンドが失敗した場合はエラー終了します。
macOS では Keychain を使えます。

```sh
security add-generic-password -a "$(whoami)" -s OPENAI_API -w "your-key"
```

```yaml
apis:
  openai:
    api-key-cmd: security find-generic-password -a masat -s OPENAI_API -w
```

## 使い方

```sh
# タスク指向の内蔵マニュアルを日本語で読む
henji docs --lang ja

# コマンド出力を渡す
git diff | henji "suggest a commit message"

# プロンプトだけを渡す
henji "explain this error"

# API とモデルを明示する
henji --api local --model llama "summarize this"

# stdin をパイプライン用に残したまま、テキストを 1 ファイル添付する
git diff | henji --text requirements.txt "check this diff"

# 会話を継続する
henji --continue <id-or-title> "now propose the smallest fix"
```

用例は [examples.ja.md](examples.ja.md)（[English](examples.md)）、機能一覧は
[features.ja.md](features.ja.md)（[English](features.md)）を参照してください。

### Decisions APIによるテキスト・画像判断

独立した `henji decision` でOpenRouter・OpenAIのDecisions APIを呼び出します。

```sh
henji decision -a openrouter -m typesafe/jev-1.13 \
  --questions docs/examples/decision-openrouter-questions.json < input.txt
henji decision -a openai -m gpt-6-luna \
  --questions docs/examples/decision-openai-questions.json < input.txt
```

stdinは判断対象の自然文です。`--questions` はOpenRouterなら名前付きオブジェクト、OpenAIなら
配列で指定します。同じ対象への複数質問を一回の要求で送ります。`-m` は必須で生成用の既定モデルを
継承せず、テキスト用モデルIDは設定への事前登録が不要です。設定済みの別名も利用できます。

成功時はAPI応答全体のJSONをstdoutへ出し、正常応答に含まれるrefusalも終了コード0です。
失敗時は終了コード1、stdoutは空、詳細はstderrです。Henjiのラッパーは付けず、`output` 設定にも
左右されません。会話履歴・キャッシュ・DBは作成せず、連続処理はshell側で繰り返し起動します。

認証は既存のキー取得設定を使い、既定環境変数は `OPENROUTER_API_KEY` / `OPENAI_API_KEY` です。
生成用 `base-url` とは独立した `decision-base-url` と `decision-protocol` を使い、標準2社は省略できます。
`http-proxy` と `max-retries` は設定・環境変数から利用します。decisionの `max-retries` は初回後の
追加試行回数で、0なら要求は一回だけです。

`--image` はJPEG/PNG/WebPを1枚、3MiBまで受け付け、モデル設定の `vision: true` が必要です。
この値はHenjiの読込み上限であり、モデルの受理上限は別です。自動縮小・再圧縮・文章切り詰めは
行いません。Clef系の画像サイズや文章切り詰めなど、[モデル固有の制約](https://openrouter.ai/docs/guides/community/multimodal-decisions)を確認してください。

専用フラグは `henji decision --help`、設定・画像・ループの例は[cookbook](docs/cookbook.ja.md#decisions-apiによる判断)を参照してください。

### 通常の生成で使う重要なフラグ

`henji -h` が完全な一覧です。代表的なものは、`--api` / `--model`（プロバイダーとモデルの
選択）、`--text` / `--image`（現在のリクエストだけに添付）、`--continue`、`--list`、
`--show`、`--delete`、`--no-cache`、`--output json`、`--json-schema` です。

`max-input-chars` はバイト数の上限です。結合したプロンプトと区切りを数え、UTF-8 の文字境界で
切り詰めます。画像はこの上限に含めません。
`--no-limit` は `max-input-chars` による入力の切り詰めを無効にします。応答トークン数の上限は
変わりません。OpenAI 互換の推論モデル向け `max-completion-tokens` は YAML（全体または
モデルごと）、または `HENJI_MAX_COMPLETION_TOKENS` で設定し、専用 CLI フラグはありません。

OpenAI 互換 API では、`max-tokens` と `max-completion-tokens` を両方設定すると両方を送信します。
CLI の `--max-tokens` は、継承した `max-completion-tokens` の値を解除しません。
両フィールドの扱いはプロバイダーによるため、`--max-tokens` だけで実際の上限は確定しません。

`--text` は UTF-8 テキストを 1 つ、`--image` は JPEG/PNG/WebP を 1 つ受け取り、どちらも
上限は 3 MiB です。添付内容は会話履歴に保存されないため、継続時に必要なら再度指定します。
画像を使うモデルには設定で `vision: true` が必要です。

### 会話

成功した会話は、`--no-cache` を指定しない限り保存されます。`henji --list` は対話選択ではなく
一覧を標準出力へ出します。ID またはタイトルを次のコマンドで使ってください。

```sh
henji --list
henji --show <id-or-title>
henji --continue <id-or-title> "follow-up prompt"
henji --delete <id-or-title>
```

`--continue` の ID・タイトルが存在しない、または複数件に一致する場合はエラーになります。
最新会話へは自動で切り替わりません。最新会話には明示的に `--continue-last` を使います。
継続時には、保存されている API とモデルを復元します。

同じ会話の操作は、履歴読込から応答の保存まで順番に実行します。後続の要求は先の操作が
終わるまで待機します。別会話は並行実行でき、排他の待機中も Ctrl-C で取り消せます。

長い会話の継続は履歴全体を再送するため、入力量と料金が増えます。過去の文脈が不要なら新規の
会話を始めてください。

## クラウドプロバイダーと構造化出力

`openai`、`anthropic`、`google`、`groq`、および任意の OpenAI 互換 API を設定できます。
Anthropic と Google はネイティブプロトコルを使い、それ以外は OpenAI 互換プロトコルを使います。

`--output json` は成功時・失敗時を一行 JSON で包むため、スクリプトで安全に扱えます。
準備・生成・保存の失敗は `error` に入り、保存失敗時も生成済みの `content` を保持します。
`--json-schema <file>` はプロバイダーの構造化出力を使い、クライアント側でも検証します。
小さなローカルモデルでは JSON をコードフェンスで囲むことがあるため、プロンプトに
`raw JSON only, no code fences` と添えると役立ちます。Google 向けスキーマでは
`additionalProperties` を使わないでください。

アプリ連携では、`-q --no-cache --json-schema <file>` を使い、`--output json` を指定しなければ、
成功時の stdout は検証済みのモデル JSON と末尾改行だけで、`content` や `error` のラッパーは
付きません。終了コードは 0 です。準備・API 呼び出し・応答検証が最終的に失敗した場合は、
終了コード 1、stdout は空、stderr に人間向けのエラー詳細を出します。
`-q` は進捗表示を抑制しますが、エラーは抑制しません。

`--json-schema-retries` は検証失敗後の再生成回数で、既定値は 2、`0` で再生成を無効にします。
レート制限などに対する HTTP/API 再試行は別で、`max-retries` とプロバイダークライアントが
処理するため、`--json-schema-retries 0` でも無効にはなりません。

```sh
git diff | henji --json-schema review.json "review this diff" | jq '.findings[]'
henji --output json "explain this error" < error.log | jq -r '.content[0].text'
```

## 生成、確認、実行

henji CLI にコマンドを提案させることはできますが、出力をそのまま `sh` に接続しないでください。
人が内容を確認してから自分で実行するのが、想定する安全な使い方です。

```sh
henji -R shell "find large files under the current directory" | less
# 確認後、必要なコマンドだけを自分で実行する
```

## 検証とライセンス

CI のリリースゲートは `go test ./...` と `go vet ./...` です。
`go test -race ./...` と `scripts/e2e-openrouter-test.sh` はタグ付け前の参照チェックで、
C ツールチェーンと課金済みの `OPENROUTER_API_KEY` が必要なためリリース workflow では
実行しません。参照環境で実行し、結果は
[release checkpoints](docs/release-checkpoints.md) に記録します。
ライセンスは MIT License です。
