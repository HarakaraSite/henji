# 外部ライブラリ互換性チェック

確認日: 2026-06-13

## 実行した確認

```sh
GOCACHE=/private/tmp/codex-go-cache GOMODCACHE=/private/tmp/codex-go-mod-cache go list -m -u all
GOCACHE=/private/tmp/codex-go-cache GOMODCACHE=/private/tmp/codex-go-mod-cache go run golang.org/x/vuln/cmd/govulncheck@latest ./...
```

最初の `go list -m -u all` は sandbox 内の DNS 制限で `proxy.golang.org` を解決できず失敗したため、ネットワーク許可付きで再実行した。

`govulncheck` はローカルに未インストールだったため、`go run golang.org/x/vuln/cmd/govulncheck@latest ./...` で一時実行した。

## 現状

- `go.mod` は `go 1.25.0`。
- 確認時のローカル toolchain は `go1.26.4 darwin/arm64`。
- `go list -m -u all` は完走し、更新候補は多数ある。
- `govulncheck` は exit status 3。到達可能な脆弱性が 11 件、対象 module は 3 つ。

## 到達可能と判定された脆弱性

### `golang.org/x/net`

現行: `v0.46.0`

- `GO-2026-5026`
  - `golang.org/x/net/idna`
  - fixed in `v0.55.0`
  - trace: Google provider の `http.Client.Do` 経由
- `GO-2026-4918`
  - `net/http/internal/http2` in `golang.org/x/net`
  - fixed in `v0.53.0`
  - trace: Google provider の `http.Client.Do` 経由

対応方針:

まず `golang.org/x/net` を少なくとも `v0.55.0` 以上へ上げる。`go list` 上の最新候補は `v0.56.0`。

### `golang.org/x/crypto`

現行: `v0.43.0`

- `GO-2026-5018`
  - `golang.org/x/crypto/ssh`
  - fixed in `v0.52.0`
  - trace: Ollama SDK 経由

対応方針:

`golang.org/x/crypto` を少なくとも `v0.52.0` 以上へ上げる。`go list` 上の最新候補は `v0.53.0`。

### `github.com/ollama/ollama`

現行: `v0.17.7`

`govulncheck` は Ollama module について 8 件を到達可能と判定した。

- `GO-2025-4251`
- `GO-2025-3824`
- `GO-2025-3695`
- `GO-2025-3689`
- `GO-2025-3582`
- `GO-2025-3559`
- `GO-2025-3558`
- `GO-2025-3557`

いずれも fixed version は `N/A` と表示された。`go list` 上の最新候補は `v0.30.8`。

対応方針:

Ollama は自分の利用予定 provider なので、最優先で SDK 更新を試す。ただし vuln DB 上は fixed version が出ていないため、更新だけで `govulncheck` が消えるとは限らない。中長期的には Ollama 専用 SDK 依存をやめて OpenAI compatible endpoint 側に寄せる案が有力。

## 主な direct dependency 更新候補

優先度高:

- `github.com/ollama/ollama`: `v0.17.7` -> `v0.30.8`
- `github.com/mark3labs/mcp-go`: `v0.45.0` -> `v0.54.1`
- `github.com/anthropics/anthropic-sdk-go`: `v1.26.0` -> `v1.50.1`
- `modernc.org/sqlite`: `v1.46.1` -> `v1.52.0`

UI / CLI 系:

- `github.com/charmbracelet/glamour`: `v0.10.0` -> `v1.0.0`
- `github.com/charmbracelet/huh`: `v0.8.0` -> `v1.0.0`
- `github.com/charmbracelet/x/exp/golden`: newer pseudo-version あり
- `github.com/spf13/cobra`: 更新候補なし
- `github.com/spf13/pflag`: 更新候補なし

小さめの補助依存:

- `github.com/caarlos0/duration`: newer pseudo-version あり
- `golang.org/x/sync`: `v0.20.0` -> `v0.21.0`

更新候補なし、または現状維持でよさそう:

- `github.com/adrg/xdg`
- `github.com/caarlos0/env/v9`
- `github.com/caarlos0/go-shellwords`
- `github.com/caarlos0/timea.go`
- `github.com/openai/openai-go`
- `github.com/stretchr/testify`
- `gopkg.in/yaml.v3`

## 推奨する更新順

1. `golang.org/x/net`, `golang.org/x/crypto` を先に上げる。
   - 到達可能な脆弱性の fixed version が明確。
   - provider SDK の大規模 API 変更を伴いにくい。
2. Ollama SDK を単独で更新する。
   - 利用予定 provider なので優先。
   - API 差分と `govulncheck` 結果を確認する。
   - うまくいかない場合、OpenAI compatible 化の検討へ切り替える。
3. MCP SDK を単独で更新する。
   - MCP は中核機能として残す方針。
   - tool schema / client API の破壊的変更がないか見る。
4. Anthropic SDK、SQLite、UI 系を別々の小さい commit で更新する。
   - provider SDK と DB はそれぞれ失敗時の切り戻し単位を分ける。
   - UI 系は表示崩れや TTY 挙動の確認が必要。

## まだ実施していないこと

- 依存更新そのものはまだ行っていない。
- `govulncheck` の verbose 出力確認はまだ行っていない。
- 各 dependency の changelog/API 変更確認はまだ行っていない。
