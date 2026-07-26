# henji（mods フォーク）

パイプラインのために作られた、コマンドラインの AI。

*henji* は日本語の「返事」に由来する名前です。これは 2026 年 3 月 9 日にアーカイブされた
[charmbracelet/mods](https://github.com/charmbracelet/mods) の、現在も保守しているフォークです。
ローカル LLM の利用と、henji を Unix フィルターとして扱うことを重視します。すなわち
`stdin → LLM → stdout` として、シェルの他のパイプラインと組み合わせます。henji 自身は
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
| 全プロバイダー | o1 系で `max-completion-tokens` を API へ正しく渡すよう修正 |
| 全プロバイダー | 文書どおり `api-key-env` を `api-key-cmd` より優先 |

### セキュリティ

- `henji.yml` は `0600` で作成します。既存ファイルの権限が緩い場合は読む前に `0600` へ制限し、別ユーザー所有なら拒否します（Unix のみ。Windows は未対応）。
- Google API キーを URL クエリではなく `x-goog-api-key` ヘッダーで渡します。
- `henji.yml` と `*.bak` を `.gitignore` に追加しています。

### 依存関係と削除した機能

依存関係は `x/net`、`x/crypto` のセキュリティ更新を含めて更新しました。ネイティブ Ollama
クライアントは削除し、OpenAI 互換の経路（`base-url: http://localhost:11434/v1`）を使います。
未使用の UI・プロンプト表示・スピナー調整フラグは削除しました。`--temp`、`--topp`、
`--topk`、`--stop`、`--max-retries`、`--word-wrap`、`--http-proxy` は `henji.yml` または
`HENJI_*` 環境変数で引き続き設定できます。

MCP（Model Context Protocol）対応も完全に削除しました。信頼できない文章を処理した際に、
モデルが外部ツールを承認・読み書きの区別なく呼び出せる設計には実害のあるリスクがありました。
henji は通常の Unix フィルターに戻し、ファイルやネットワークへのアクセスは周囲の `cat`、
`curl`、`find` などのシェルコマンドが担います。

## インストール

### ソースからビルド

```sh
git clone https://forge.harakara.site/littleisland/henji.git
cd henji
go build -o henji .
```

このリポジトリはまだ公開されていないため、現在はローカル clone からのビルドだけをサポート
しています。

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

ローカルの OpenAI 互換エンドポイントを指定します。henji は常に API キーを送るため、サーバーが
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

優先順位は `api-key-cmd`、`api-key-env`、`api-key`、プロバイダー既定の環境変数です。
`api-key-cmd` はシェルを介さず直接実行するため、`$USER` や `$(whoami)` は展開されません。
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

### 重要なフラグ

`henji -h` が完全な一覧です。代表的なものは、`--api` / `--model`（プロバイダーとモデルの
選択）、`--text` / `--image`（現在のリクエストだけに添付）、`--continue`、`--list`、
`--show`、`--delete`、`--no-cache`、`--output json`、`--json-schema` です。

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

長い会話の継続は履歴全体を再送するため、入力量と料金が増えます。過去の文脈が不要なら新規の
会話を始めてください。

## クラウドプロバイダーと構造化出力

`openai`、`anthropic`、`google`、`groq`、および任意の OpenAI 互換 API を設定できます。
Anthropic と Google はネイティブプロトコルを使い、それ以外は OpenAI 互換プロトコルを使います。

`--output json` は成功時・失敗時を一行 JSON で包むため、スクリプトで安全に扱えます。
`--json-schema <file>` はプロバイダーの構造化出力を使い、クライアント側でも検証します。
小さなローカルモデルでは JSON をコードフェンスで囲むことがあるため、プロンプトに
`raw JSON only, no code fences` と添えると役立ちます。Google 向けスキーマでは
`additionalProperties` を使わないでください。

```sh
git diff | henji --json-schema review.json "review this diff" | jq '.findings[]'
henji --output json "explain this error" < error.log | jq -r '.content[0].text'
```

## 生成、確認、実行

henji にコマンドを提案させることはできますが、出力をそのまま `sh` に接続しないでください。
人が内容を確認してから自分で実行するのが、想定する安全な使い方です。

```sh
henji -R shell "find large files under the current directory" | less
# 確認後、必要なコマンドだけを自分で実行する
```

## 検証とライセンス

開発時の基本検証は `go test ./...`、`go vet ./...`、必要に応じて
`scripts/e2e-gateway-test.sh` です。ライセンスは MIT License です。
