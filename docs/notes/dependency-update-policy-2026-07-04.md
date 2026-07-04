# 依存ライブラリ調査結果とバージョンアップ方針

調査日: 2026-07-04

## 1. 結論

`go.mod` の直接依存 29 件を Go module proxy の情報と照合した。

- 現在の module path の範囲で最新版: 27 件
- 現在の module path の範囲で更新可能: 2 件
- 別の major module path に安定版があるもの: 6 件
- `go mod tidy -diff`: 差分なし
- 調査による `go.mod` / `go.sum` の変更: なし

同一 major 内の 2 件は小さく分けて先に更新できる。新しい major version がある
6 件は API 互換性が保証されないため、通常の依存更新とは分けて移行する。

## 2. 調査方法

以下を使って、現在選択されているバージョン、同一 module path の最新版、依存元、
module files の整合性を確認した。

```sh
go list -m -u -json all
go list -m -u -f '{{if and .Update (not .Indirect)}}{{.Path}} {{.Version}} -> {{.Update.Version}}{{end}}' all
go mod graph
go mod why -m <module>
go mod tidy -diff
```

`go list -m -u` は現在の module path 内の更新を検出する。`/v2` のように module
path 自体が変わる major update は自動的には表示されないため、主要な直接依存に
ついては新しい major path の `@latest` も個別に確認した。

## 3. 同一 major 内の更新候補

| ライブラリ | 現在 | 最新 | 用途 | リスク |
|---|---:|---:|---|---|
| `github.com/charmbracelet/x/exp/golden` | `v0.0.0-20241011142426-46044092ad91` | `v0.0.0-20260629091435-9c70f75e26a4` | golden test | 低。ただし pseudo-version 間の差分確認が必要 |
| `github.com/lucasb-eyer/go-colorful` | `v1.3.0` | `v1.4.0` | ターミナルの色・gradient | 低〜中。表示確認が必要 |

これらは一括更新せず、原則として 1 件ずつ更新してテストする。問題発生時に原因を
特定しやすくし、CLI表示やTTY判定の回帰を切り分けるためである。

更新済み:

- `github.com/mattn/go-isatty`: `v0.0.20` → `v0.0.22`（2026-07-04）

## 4. 新しい major version がある直接依存

| ライブラリ | 現在 | 最新major | 方針 |
|---|---:|---:|---|
| `github.com/caarlos0/env` | `v9.0.0` | `v11.4.1` | 単独で移行可能。環境変数のdecodeとエラー挙動を確認する |
| `github.com/charmbracelet/bubbles` | `v1.0.0` | `v2.1.0` | Charm系移行としてまとめて評価する |
| `github.com/charmbracelet/bubbletea` | `v1.3.10` | `v2.0.8` | 状態更新、入出力、rendererのAPI差分を重点確認する |
| `github.com/charmbracelet/glamour` | `v1.0.0` | `v2.0.1` | Markdown描画とstyle設定を確認する |
| `github.com/charmbracelet/huh` | `v1.0.0` | `v2.0.3` | 対話入力とキャンセル時のエラー判定を確認する |
| `github.com/charmbracelet/lipgloss` | `v1.1.1`系 pseudo-version | `v2.0.5` | layout、幅計算、色profileを確認する |

Charm系5件は相互に関連し、import path と型の変更が波及する可能性がある。個別に
最新版へ上げるのではなく、専用ブランチで一つの移行作業として扱う。ただしコミットは
機械的な import/API 移行と、挙動修正・表示修正を分ける。

## 5. 現在の module path で最新版だった主要依存

調査時点で、以下には同一 module path の更新候補がなかった。

| ライブラリ | バージョン |
|---|---:|
| `github.com/anthropics/anthropic-sdk-go` | `v1.56.0` |
| `github.com/openai/openai-go` | `v1.12.0` |
| `github.com/mark3labs/mcp-go` | `v0.55.1` |
| `modernc.org/sqlite` | `v1.53.0` |
| `github.com/spf13/cobra` | `v1.10.2` |
| `github.com/spf13/pflag` | `v1.0.10` |
| `github.com/santhosh-tekuri/jsonschema/v6` | `v6.0.2` |
| `golang.org/x/sync` | `v0.21.0` |
| `github.com/stretchr/testify` | `v1.11.1` |

「更新候補なし」は、現在の module path で Go proxy が返す最新版という意味である。
リポジトリの未リリース commit や prerelease への追随は対象外とする。

## 6. 間接依存の扱い

間接依存は henji が直接 import するものではなく、直接依存ライブラリの `go.mod` が
要求するモジュールである。

```text
henji
  └─ 直接依存ライブラリ
       └─ 間接依存ライブラリ
```

間接依存に最新版が存在しても、それだけを理由に手動で上げない。親ライブラリが検証した
依存構成から外れ、コンパイルは通っても実行時の挙動や互換性に問題が出る可能性があるため
である。通常は次の手順で追随させる。

1. 直接依存を目的とリスクが明確な単位で更新する。
2. `go mod tidy` で不要な依存を除去し、必要な間接依存を解決する。
3. `go test ./...` と `go vet ./...` を実行する。
4. provider、MCP、TTY表示など、変更箇所に応じた動作確認を行う。

次の場合は例外として、間接依存を `go.mod` に明示して最低バージョンを引き上げることを
検討する。

- 到達可能なコードに影響する既知の脆弱性がある。
- henji に必要なbug fixが新しい間接依存にしかない。
- 複数の親依存間で最低バージョンの調整が必要になった。

この例外対応では、なぜ親ライブラリの更新では解決できないかをコミットまたはPRに記録する。

## 7. AWS SDK が依存グラフにある理由

AWS SDK v2 は henji が直接使っているものではなく、
`github.com/anthropics/anthropic-sdk-go` から入っている。

```text
henji
  └─ github.com/anthropics/anthropic-sdk-go
       ├─ 通常の Anthropic API client
       ├─ Anthropic AWS Gateway client
       └─ Amazon Bedrock client
            └─ github.com/aws/aws-sdk-go-v2
```

Anthropic SDK は同一モジュール内で AWS Gateway、Amazon Bedrock、SigV4署名、AWSの
credential chainをサポートしており、そのため `go.mod` でAWS SDK群を要求している。

henji は `internal/anthropic` から通常のAnthropic clientだけを利用し、Anthropic SDKの
`aws` や `bedrock` packageをimportしていない。`go mod why` でもAWS SDKについて
`(main module does not need module ...)` と判定される。

したがってAWS SDK群はmodule graphには現れるが、現在のhenjiの機能としてAWSへ接続する
ものではなく、通常はコンパイル対象や最終バイナリにも含まれない。Anthropic SDK側の
module構成なので、henji側の `go mod tidy` だけではmodule graphから除去できない。

## 8. 推奨する更新順序

### Phase 1: 同一majorの低リスク更新

1. `go-isatty` を更新し、TTYあり・なし、pipe入力、raw出力を確認する。✅
2. `go-colorful` を更新し、spinnerとterminal stylingを目視確認する。
3. `x/exp/golden` を更新し、golden testを確認する。

各更新で最低限、以下を実行する。

```sh
go mod tidy
go test ./...
go vet ./...
go build ./...
```

### Phase 2: `caarlos0/env` のmajor移行

Charm系と分離してv9からv11へ移行する。設定ファイル読込後の `HENJI_*` override、
slice、duration、数値、boolのparseと、不正値のエラー表示をテストする。

### Phase 3: Charm系v2移行

Bubble Tea、Bubbles、Lip Gloss、Glamour、Huhをまとめて移行する。unit testだけでなく、
以下のinteractive/visual確認が必要になる。

- TTY上のspinnerとstreaming表示
- Markdown renderingとword wrap
- viewportとwindow resize
- raw出力、pipe出力、`--output json`
- 対話prompt、選択UI、キャンセル
- light/dark terminalでの色とerror表示

### Phase 4: 間接依存と脆弱性の再評価

直接依存更新後にmodule graphを再取得する。間接依存の新旧だけでなく、脆弱性が実際に
henjiから到達可能かを `govulncheck ./...` で確認する。未到達の脆弱性と実行経路上の
脆弱性を区別し、後者だけを優先して対応する。

## 9. 継続運用

- 月次またはリリース前に `go list -m -u all` を実行する。
- provider SDK、MCP、SQLite、terminal UIは別々の更新単位にする。
- major update、provider SDK update、SQLite updateは専用PRにする。
- dependency update PRでは `go.mod` / `go.sum` 以外のAPI追随変更を明示する。
- versionが新しいことだけを理由に間接依存を直接pinしない。
- security fixは通常更新より優先するが、到達可能性と影響範囲を確認する。
