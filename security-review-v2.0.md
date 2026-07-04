# henji v2.0 セキュリティレビュー

レビュー日: 2026-07-04  
対象リビジョン: `a6a881ce0f1f6f5d5519abb30e136e544af9cd6e`  
対象: v2.0 のランタイムコード、設定処理、テスト、Go依存関係、リリースワークフロー

## 結論

現実的な攻撃・事故経路として、Medium 2件、Low 1件を確認した。Critical / High は確認されなかった。

v2.0 リリース前に優先して確認すべきなのは次の2点である。

1. MCPツールをモデル判断だけで実行する現在の設計に、個別承認またはツール単位の許可ポリシーがない。
2. `max-tool-calls` の上限判定がツール実行後にあり、上限を1ラウンド超えて副作用が発生する。

| ID | Severity | 内容 | Confidence |
|---|---|---|---|
| HENJI-SEC-001 | Medium | 未信頼コンテンツが承認なしでMCPツール実行へ到達できる | High |
| HENJI-SEC-002 | Medium | MCP実行上限を超えたラウンドが停止判定前に実行される | High |
| HENJI-SEC-003 | Low | 旧バージョンの緩い設定ファイル権限がv2で修復されない | High |

## 対応状況（2026-07-04追記、Opusセッションでの独立検証・ユーザー判断）

3件とも根拠・severityともに独立検証で妥当と確認済み（検証記録は本セッション外）。

- **HENJI-SEC-001: 2026-07-05、MCP機能自体を完全削除したことで解消。** `mcp-security-design-discussion.md`での検討の結果Option A（MCP完全廃止）を採用。`mcp.go`、providerのtool-call経路（`internal/openai`/`internal/anthropic`のCallTools等）、`proto.Request.Tools`/`ToolCaller`、`stream.Stream`のtool-callメソッド、`MaxToolCalls`/`MCPServers`等のconfig/flag、`mcp-go`依存を全て削除。原因コードそのものが存在しなくなったため、本findingは対応不要になった。
- **HENJI-SEC-002: 修正済み（2026-07-04）、その後2026-07-05のMCP完全削除で該当コード自体が消滅。** 修正時点では `Stream.PendingToolCalls()` を追加し `MaxToolCalls` の上限判定を `CallTools()` 実行前に行うよう変更し、副作用ゼロを回帰テストで確認していたが、MCP削除に伴いtool-call round制御・`MaxToolCalls`・`PendingToolCalls`/`CallTools` 自体を削除したため、現在は該当コードが存在しない。
- **HENJI-SEC-003: 修正済み。** `securePermissions`（`config_unix.go`/`config_windows.go`）を追加し、既存設定ファイルの権限をv2起動時に自動修復。非所有ファイルは読取前に拒否。Windowsは現時点でno-op（README に明記）。

## HENJI-SEC-001: 未信頼コンテンツが承認なしでMCPツール実行へ到達できる

Severity: **Medium**  
CWE: CWE-441 (Unintended Proxy or Intermediary / Confused Deputy)

### 根拠

- henji はCLI引数、標準入力、取得したHTTPコンテンツ、過去の会話をモデルへ渡す。これらには第三者が混入したプロンプトインジェクションが含まれ得る。
- OpenAI互換およびAnthropicのモデルが返したツール名・引数は、`mcp.go` の `toolCall` へ渡される。
- `toolCall` はサーバーが設定済みか、サーバー全体が無効化されていないかだけを確認し、個別ツールの許可やユーザー確認を行わず `cli.CallTool` を実行する。

主要箇所:

- `mods.go:390-411` — MCPツールをモデルリクエストへ公開
- `internal/openai/openai.go:155-168` — モデルのtool callを実行
- `internal/anthropic/anthropic.go:111-122` — モデルのtool callを実行
- `mcp.go:223-251` — サーバー単位の確認後、`cli.CallTool` へ到達

### 現実的なインシデント例

ユーザーがfilesystemやGitHub等のMCPを有効にして、外部リポジトリ、Issue、ログ、Webページをhenjiへ解析させる。その内容に埋め込まれた命令へモデルが従うと、ファイル読取、データ送信、Issue変更などがユーザー権限で実行される。

MCP設定自体は運用者の明示操作だが、「能力を有効にしたこと」と「モデルが選んだ個別の副作用を許可したこと」が分離されていないため、設定済みであることだけでは十分な防御にならない。

### テスト観点

MCPの接続・実行に関する専用テストがなく、次の境界が未検証である。

- 副作用ツールが承認なしでは `CallTool` に到達しないこと
- read-onlyツールとwriteツールを分離できること
- 非対話モードで安全な既定値になること
- `--mcp-disable` と個別許可ポリシーの組み合わせ

### 推奨対応

- MCP実行前にユーザー確認を入れるか、ツール単位のallowlist/read-only policyを導入する。
- 非対話実行では、明示的な許可ポリシーがない副作用ツールをモデルへ公開しない。
- サーバー名、ツール名、引数、承認結果を監査可能な形で記録する。

## HENJI-SEC-002: MCP実行上限を超えたラウンドが停止判定前に実行される

Severity: **Medium**  
CWE: CWE-691 (Insufficient Control Flow Management)

### 根拠

`mods.go:571` で `msg.stream.CallTools()` を実行した後、`mods.go:582-585` で次ラウンドが `MaxToolCalls` を超えるか判定している。

しかし、`CallTools` は保留中の呼び出しを列挙するだけではない。OpenAI実装は `internal/openai/openai.go:162-168`、Anthropic実装は `internal/anthropic/anthropic.go:114-122` で、メソッド内部から実際にツールを呼ぶ。最終的に `mcp.go:251` の `cli.CallTool` へ到達する。

そのため上限を `N` とした場合、`N+1` ラウンド目のツールが実行された後で上限エラーになる。1ラウンドに複数tool callが含まれる場合、その複数件が全て実行され得る。

### 現実的なインシデント例

ユーザーが暴走防止のため `max-tool-calls: 10` を設定していても、11ラウンド目にモデルが返した削除・更新・送信処理が先に実行され、その後でhenjiが停止する。表示上は上限到達エラーになるが、副作用は取り消されない。

### テスト上の問題

`mods_test.go:76-126` の `TestMaxToolCallsLimit` はこの不具合を検出できない。

- fakeの `CallTools()` は `ToolCallStatus` を返すだけで、実際の `ToolCaller` 呼出回数を記録しない。
- テストは戻り値が `modsError` になることだけを確認している。
- 「上限超過時に副作用が0件であること」を確認していない。

### 推奨対応

- 上限到達時は `CallTools()` を呼ぶ前に停止する。
- 必要ならpending tool callsの検査と実行を別APIへ分離する。
- テストではステータスだけでなく、fake `ToolCaller` の呼出回数が増えないことを確認する。
- 1ラウンドに複数callがあるケースと、`0`による無制限ケースも維持確認する。

## HENJI-SEC-003: 旧バージョンの緩い設定ファイル権限がv2で修復されない

Severity: **Low**  
CWE: CWE-732 (Incorrect Permission Assignment for Critical Resource)

### 根拠

- 新規設定は `config.go:317` で `0600` を指定して作成される。
- 一方、`config.go:305-311` の `writeConfigFile` は、ファイルが既に存在する場合にpermission bitsを検査・修正せず終了する。
- `config_test.go` のfixtureは常に既存設定を `0600` で作成するため、旧版からの移行ケースを検証していない。

### 再現結果

一時XDGディレクトリに既存の `henji.yml` を `0644` で作成し、未変更のv2バイナリで `--list-models` を実行した。終了後もファイルモードは `0644` のままだった。

### 現実的な影響

旧版由来の設定へ平文APIキーを保存しており、共有開発ホスト上で別アカウントが設定パスを辿れる場合、そのアカウントがキーを読み取れる。単一ユーザー端末やホームディレクトリ自体が非公開の環境では成立しないため、SeverityはLowとした。

### 推奨対応

- Unixでは既存設定を読む前に所有者とpermission bitsを検査し、group/other bitsを除去する。
- 非所有ファイルや `chmod` 失敗時は、秘密情報を読む前に明確に失敗させる。
- `0644` の既存fixtureが起動後に `0600` になる移行テストを追加する。
- WindowsではACLを含むプラットフォーム固有挙動を別途定義する。

## テスト・静的解析・依存関係の結果

| 検証 | 結果 |
|---|---|
| `go test -count=1 -timeout 30s ./...` | 成功 |
| `go vet ./...` | 成功 |
| `govulncheck -show verbose ./...` | 到達可能な既知脆弱性なし |
| 旧設定ファイル権限の実CLI再現 | `0644` が維持されることを確認 |

既存テストはプロバイダーのリクエスト構築、レスポンス処理、キャッシュ、DB、設定値、JSON Schema等を広くカバーしている。一方、セキュリティ上重要な次の回帰テストが不足している。

- MCPツールの個別承認・拒否
- 上限到達後の実副作用件数
- 旧設定ファイル権限の移行
- MCPサーバー無効化とモデル由来tool callの統合動作

## 調査したがfindingとしなかった項目

- `load.go` のHTTP/file読込: ローカルCLI利用者がURLまたはパスを明示指定する機能であり、リモート攻撃者がURLを渡すサービス境界はないため、単独のSSRF/任意ファイル読取とは判定しなかった。
- `api-key-cmd`: 設定を変更できる運用者の明示機能であり、shellを介さずargvとして直接実行される。失敗時にコマンド出力をエラー表示へ含めない。
- SQLite: 可変値はbindされており、SQLインジェクション経路は確認されなかった。
- Go依存関係: `govulncheck` で到達可能な既知脆弱性は確認されなかった。
- リリースActionのmutable major tag: SHA pinningは推奨するが、finding化には上流Actionタグの侵害・悪意ある差し替えが必要となる。今回の「非現実的な想定を除外」という条件に合わせ、hardening項目に留めた。

## スコープと制約

- 52件のソース、テスト、設定、ワークフローファイルをレビューした。
- `docs/notes/` とVHS用 `examples/` は出荷ランタイム・テストではないため除外した。
- 既存の未コミット変更（`README.md`、`examples.md`、`features.md`）は変更していない。
- コード修正は実施していない。
- MCPの破壊的な実ツール呼び出しは安全上実行せず、コードパスとテスト構造で検証した。
- 単独レビューのため、独立した複数レビュー担当による結果の二重確認は実施していない。
