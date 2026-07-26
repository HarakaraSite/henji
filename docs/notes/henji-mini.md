# `henji-mini` 検討メモ

状態: 検討段階。実装・公開・既存 henji の機能削除は未決定。

## ねらい

`henji` 本体とは別に、Unix パイプラインで一回だけ LLM に問い合わせるための小さな CLI を
検討する。設定・会話・表示機能を持つ本体を置き換えるものではなく、依存・起動経路・責務を
絞った別の実行対象にする。

想定する最小の流れは次のとおり。

```text
引数 + stdin → OpenAI API → stdout
```

## 仮の最小スコープ

- OpenAI API への一回限りの要求
- 引数と stdin を入力にし、応答チャンクを stdout へ出す
- API エラーと診断は stderr、成功時は終了コード `0`
- `OPENAI_API_KEY`、モデル指定、必要なら base URL と最大 token 数
- 必要なら機械利用向けの `--output json`

会話保存、SQLite、キャッシュ、YAML 設定、roles、Markdown の組み込み描画、モデル選択 UI は
最初のスコープから外す。Markdown はプレーンなまま stdout に流し、整形表示が必要なら利用者が
`glow` などの外部ビューアへ渡す。

```sh
henji-mini "explain this error" < error.log
henji-mini "summarize this" < report.txt > answer.md && glow answer.md
```

`glow -` は stdin を EOF まで読み切ってから描画するため、ストリーム表示には使わない。
直接実行時はプレーン Markdown を逐次表示し、保存後に `glow` で読む使い方を想定する。

## 同一リポジトリでの配置案

同じ Go module・同じリポジトリ内に別の build target として置ける。

```text
.
├── main.go                 # henji 本体
└── cmd/
    └── henji-mini/
        └── main.go
```

```sh
go build -o henji .
go build -o henji-mini ./cmd/henji-mini
```

Go は対象プログラムが import するパッケージだけをリンクする。したがって、同じ `go.mod` に
本体用の SQLite・Anthropic・Google・glamour 等が残っていても、mini が import しなければ
mini バイナリには含まれない。

`go build -tags mini .` のような build tag で同じ root package を切り替える方法もあるが、
現行の root `package main` には本体機能が多数集まっている。除外対象ファイルごとに tag を
付ける必要があり、将来のタグ漏れを招きやすい。まずは `cmd/henji-mini` を独立させる方が
保守しやすい。

## 本体との関係

- 本体の Azure OpenAI / Azure AD 廃止と、組み込み Markdown レンダリング廃止は別の整理項目。
- mini の初期版を OpenAI 公式 API 専用にするか、OpenAI 互換 endpoint も受け入れるかは未決定。
- 本体とのコード共有は急がない。共有する価値が確認できた小さな部分だけを `internal/` へ
  切り出す。
- 現行のタグリリース workflow は `henji` だけを配布する。mini を配布する場合は、同じタグの
  release asset に `henji-mini-*` を追加するか、別のリリース体系を採るかを決める。

## 検証観点

- 実 API 呼び出しを含む最大 RSS、バイナリサイズ、起動時間を現行 henji と同じ条件で比較する。
- stdin、長い入力、SSE の keepalive、取消、API エラーで stdout/stderr/終了コードの契約を確認する。
- リクエスト・応答を保存しないことを確認する。

## 未決定事項

1. OpenAI 公式 API 専用か、OpenAI 互換 endpoint を含めるか。
2. `--output json`、`--json-schema`、画像入力を初期版に含めるか。
3. 設定ファイルなしを徹底するか、`--base-url` など少数の環境変数だけを許すか。
4. 本体と同一タグで配布するか、mini 専用のタグ・リリースを持つか。
