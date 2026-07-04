# MCP削除レビューとCLIフットプリント整理

作成日: 2026-07-05

## 目的

MCP完全削除後のコード／テストレビュー結果と、その後に検討したMarkdown表示、Bubble Tea、Huh、会話保存方式の方針を記録する。Sonnetへ独立レビューを依頼する際の引き継ぎ資料も兼ねる。

## 背景と判断

henjiは次のUnix filterとして扱う。

```text
stdin → LLMによるテキスト変換 → stdout
```

MCPは開発・実利用を通じて一度も実際に動かしておらず、設定・接続・tool callまで到達していなかった。一方、安全に維持するにはtool単位の許可、対話承認、非対話時のpolicy、sandbox、監査等が必要になる。この維持コストと実利用が釣り合わないため、MCP機能を完全削除した。

## MCP削除のコードレビュー

対象コミット:

```text
74786a6..f65dd7c
```

削除を確認した範囲:

- `mcp.go`
- MCP server設定とclient pool
- tool一覧取得
- OpenAI／Anthropicへ渡していたtool schema
- `ToolCaller`
- `PendingToolCalls()`／`CallTools()`
- agentic tool loopとcall round制御
- MCP関連config、template、CLI flags
- `mcp-go`直接依存と不要になった推移依存
- README、manual、examplesの利用案内

実行可能なMCP経路は残っていない。HENJI-SEC-001とHENJI-SEC-002は、原因となる機能自体の削除によって解消した。

### 指摘1: 旧MCP会話のtool result表示

重要度: 通常のコードレビューではMedium。セキュリティfindingとしては除外。

MCP時代のgob会話を新しい`proto.Message`へ復号すると、削除された`ToolCalls`は捨てられるが、`Role: "tool"`と`Content`は残る。

`proto.Conversation.String()`から`RoleTool`用の分岐が削除されたため、未知の`tool` roleがgeneric pathへ入り、tool result本文をそのまま表示する。

旧版と新版へ同じ会話cacheを与えてinteractiveな`henji --show`を比較した。

- 旧版: `Ran tool: filesystem_read_file`だけを表示
- 新版: tool resultの`API_TOKEN=legacy-secret-value`を無ラベルで表示

同じOSユーザーが自分のcacheを明示的に`--show`する必要があり、低権限者やremote attackerとの境界はない。そのためセキュリティ脆弱性としては扱わない。ただし、移行時の表示・privacy regressionとしては実在する。

推奨する最小対応:

- MCP時代のgob fixtureを追加する。
- legacy `tool` roleを表示時に明示的にskipまたはredactする方針を決める。
- `--show`の旧cache互換テストを追加する。

### 指摘2: provider streamの再始動処理が残っている

重要度: Low。

`internal/stream.Stream.Next()`は`false`を「これ以上messageがない」終端としている。一方、OpenAIとAnthropicの`Stream`にはMCP tool loop用だった次の状態が残っている。

- `done`
- `request`
- `factory`

一度`Next()`が`false`を返した後にもう一度呼ぶと、同じLLM requestを再送する。OpenAI実装でrequest countが1から2へ増えることを動的に確認した。

現在のcallerは`false`後に再度`Next()`を呼ばないため、現在の利用経路では発生しない。ただしinterface契約と矛盾し、将来の変更で重複リクエストや重複課金を起こし得る。また不要なrequest状態とclosureを保持する。

推奨する最小対応:

- OpenAI／AnthropicからMCP再始動用の状態と分岐を削除する。
- `Next()`が一度`false`になった後は繰り返し`false`を返すテストを両providerへ追加する。
- HTTP request countが1のままであることも確認する。

### テスト・静的検査

| 検査 | 結果 |
|---|---|
| `go test -count=1 -timeout 30s ./...` | 成功 |
| `go test -race -count=1 -timeout 90s ./...` | 成功 |
| `go vet ./...` | 成功 |
| `go mod verify` | 成功 |
| `govulncheck -show verbose ./...` | 既知脆弱性なし |
| `git diff --check 74786a6..HEAD` | `docs/notes/fix-roadmap.md:9`の末尾空白1件で失敗 |

内部設計資料の`docs/notes/overview.md`や`docs/notes/feature-requirements.md`等には、MCPが現存する前提の記述が残っている。履歴資料として維持するのか、現行仕様へ更新するのかを明示する必要がある。

## バイナリサイズ実測

環境: Go 1.26.4、darwin/arm64

| ビルド | MCP削除前 | MCP削除後 | 削減 |
|---|---:|---:|---:|
| 通常 | 56,220,274 bytes | 55,065,378 bytes | 1,154,896 bytes（2.05%） |
| `-ldflags='-s -w'` | 39,221,538 bytes | 38,416,130 bytes | 805,408 bytes（2.05%） |

MCPだけではバイナリは大きく縮まなかった。残る主要な構成要素は次のとおり。

- GlamourとMarkdown rendererの推移依存
- modernc SQLite
- Bubble Tea／HuhとTTY UI関連
- OpenAI／Anthropic等のprovider SDK

## UIとフットプリントの現在方針

### Markdownレンダリング

削除候補とする。

- pipeline利用ではraw text／Markdown sourceをstdoutへ出せればよい。
- terminalでrenderしたい利用者は`henji ... | glow`のように外部合成できる。
- Glamour、Chroma、Goldmark、Bluemonday等の依存削減が期待できる。
- Markdown表示用viewportやresize処理の一部も単純化できる可能性がある。

削除前に、Glamourあり／なしでstripped binary sizeと起動時間を実測する。

### 会話保存とSQLite

会話データ自体は必要であり、現時点ではSQLiteを直ちに削除しない。

現在は概ね次の二層構造になっている。

- 会話本文: gob files
- ID、title、更新日時、model、API等の索引: SQLite

SQLiteを外す場合は、単一JSON index、会話ごとのmetadata file、directory scan等への再設計が必要になる。検索性能、atomic update、破損時の復旧、同時実行を比較してから判断する。

### Bubble Tea

当面維持する。

現在の主な役割:

- streaming状態管理
- spinner
- key inputと終了処理
- window resize
- viewport
- 非同期commandと画面更新

Markdown rendererを削除しても、streaming CLIの状態管理基盤として残す。

### Huh

当面維持する。

過去会話の対話選択は現在も利用しているため、`henji --list`の選択UIを残す。

一方、次の機能は利用実績が薄く削除候補とする。

- 引数なし起動時のAPI選択
- 引数なし起動時のmodel選択
- 対話式prompt入力

Huh自体は内部でBubble Teaを使うため、Huhを残す限りBubble Tea依存削減にはならない。今回の方針はBubble Teaを残すため、この点は問題にならない。

## 次の調査候補

1. Glamour／Markdown rendererを外した試作で、通常・stripped binary size、起動時間、依存module数を比較する。
2. OpenAI／Anthropicのstream再始動処理を削除し、終端性の回帰テストを追加する。
3. 旧MCP会話cacheの扱いを、skip、redact、明示的なlegacy表示のどれにするか決める。
4. API／model／prompt対話フォームの利用実績を確認し、過去会話選択を残したまま削除できる範囲を確定する。
5. 内部設計資料のうち、履歴と現行仕様を区別してMCP記述を整理する。

## Sonnet独立レビュー結果

herdr上に残っていたMCP削除担当のSonnetセッションへこの文書を渡し、コードを変更しない独立レビューを実施した。

### 指摘と重要度

Sonnetは2件とも妥当と判定した。

- 旧MCP会話のtool result表示: Low〜Medium。攻撃者境界がなくセキュリティfindingから除外する判断は妥当だが、token等が実際に表示されるprivacy regressionなので、通常レビューではMedium寄り。
- provider streamの再始動残存: Low。現在のcallerからは再始動しないが、`Stream.Next()`の終端契約に反し、将来の重複requestにつながる。

Sonnetも`Conversation.String()`、gob decode、OpenAI／Anthropicの`Next()`をコード上で確認した。

### 最小追加テスト

1. MCP-era gob fixture（`Role: "tool"`と旧`ToolCalls`を含む）を`--show`相当の経路へ通し、生のtool resultが無ラベルで出ないことを確認する。
2. OpenAI／Anthropicの双方で、`Next()`が一度`false`を返した後も`false`を返し、HTTP request countが増えないことを確認する。

この2本でMCP削除に伴う主要な見落としは概ねカバーできるとの評価だった。

### UI方針

方針は整合している。

- `selectFromList()`による過去会話選択は維持できる。
- `askInfo()`によるAPI／model／prompt入力は独立した単一経路なので削除可能。
- Bubble Tea／Huhを残したままGlamourを削除できる。

ただし、Markdown表示と`glamViewport`による長文scrollは高さ計測で結合している。Glamour削除はrender呼び出しを消すだけではなく、viewport／pagingを残すか廃止するかの設計を伴う。この論点をGlamour削除調査へ追加する。

### 次の削減策

Sonnetの優先順位:

1. Glamour削除。Chroma、Goldmark、Bluemonday、Douceur、Gorilla CSS、Catppuccin等のMarkdown専用推移依存も連動して削除できる。効果は大きいがviewport再設計が必要。
2. OpenAI／Anthropicの`done`、`request`、`factory`削除。削減量は小さいが、レビュー指摘の修正を兼ね、リスクと実装コストが低い。
3. `askInfo()`削除。Huh自体は残るのでサイズ効果は小さいが、コードとテスト対象を減らせる。引数なしTTY起動時の代替挙動を同時に決める。

SQLite代替は、効果に対して検索性能・atomic update・移行のトレードオフが大きいため、上記より優先度を低く維持する。
