# mods フォーク 修正ロードマップ

作成日: 2026-06-21  
更新日: 2026-06-26 v6（PR#14〜#16 完了・リファクタ検討項目を次バージョンセクションに追加）  
対象ブランチ: main（Codex作業済みコミット `23810ab` 以降）

---

## 0. このフォークの目的と公開方針

### 背景

charmbracelet/mods は 2026-03-09 にアーカイブされた。本家 README はフォークでのメンテ継続を明示的に歓迎している。

### 作者の主目的

**ローカル LLM 中心 + MCP 連携の CLI ツール**

- 軽作業（fish スクリプト組み立て等）をローカル LLM（mlx-lm/MLX = OpenAI 互換エンドポイント、Ollama）に任せる
- Web 検索・調査・翻訳・要約もローカル LLM + MCP で処理する

### 公開方針

修正後に公開し、他ユーザーの利用を想定する。公開物としての価値は「mods の正しく動く後継」であること。  
「ローカル LLM 優先」は**着手順の話**であって、完了範囲を狭める話ではない。

### 優先度判断の基準

1. **公開前に必須**: Tier 1（全プロバイダのクラッシュ修正）は作者が使わない google/anthropic も含めて完遂する。他ユーザーが使うため省略不可。
2. **着手順**: 主経路（ollama, openai互換）を先に、副次（google, anthropic）を後に。
3. **プロバイダ削除なし**: Cohere は本家由来で削除済み。以降は全プロバイダを維持する。

### 別トラック：role 設定タスク

fish スクリプト生成 role・翻訳 role・要約 role は `mods.yml` の `roles:` 設定だけで実現できる見込み（コード改造不要）。これらはこの roadmap のバグ修正 PR とは独立した「設定・ドキュメント整備タスク」として管理する。設定だけで賄えるかはドッグフーディング後に判断し、使い勝手の問題が出た時点で初めて機能追加 PR を検討する。

### 公開時の必須作業（コード修正とは別）★ **次フェーズ**

- **README 更新**: アーカイブ済み本家からの fork である旨、本家が fork でのメンテを歓迎している事実、このフォークが何を変えたか（Cohere 削除・クラッシュ修正・依存更新・MCP 接続キャッシュ・MaxToolCalls）の要約、LICENSE（MIT）の著作権表示維持と改変部分の表示追加。README に「ローカル MCP 多用時の推奨設定例（`max-tool-calls: 50`）」を記載し、用途に応じてユーザーが設定するよう案内する。
- **モジュール名変更**: `go install github.com/<user>/<新名称>` の形で配布する方針は確定。ただし module パスの変更は内部 import 全体に波及するため、**公開直前にまとめて実施**（今は触らない）。

### バージョニング方針

このリポジトリは本家の git 履歴（`v0.1.1`〜`v1.8.1` 等の旧タグ）をそのまま引き継いでいるため、フォーク独自のタグと本家タグの番号が衝突する（例: 本家の `v0.2.0` は2023年のコミットを指しており、フォーク側で同じ番号を新規に使うと上書きになってしまう）。

- **公開前（検証中）**: `v0.9.x` を使う（例: `v0.9.0` = PR#17 `--output json` + PR#18 `max-input-chars` バグ修正）。本家タグと衝突しない範囲で、公開前の動作確認・CI疎通確認用として刻む。
- **公開時**: `v2.0.0` を打つ。本家の `v1.8.1` 系譜を引き継ぎつつ、モジュールパス変更・Cohere削除・環境変数プレフィックス変更（`MODS_`→`HENJI_`）等の破壊的変更を経た独立フォークとしてメジャーバージョンを上げる。`v2.x` 系タグは本家履歴に存在しないため衝突しない。
- 公開直前タスク（README更新・モジュール名変更）が完了した時点で `v2.0.0` タグを作成しリリースする。

---

## 1. Codexで実施済み（参考）

- Cohere provider削除
- Ollama stream完了バグ修正（`finalized`フラグ追加）
- Google/Gemini 会話保存が空になる修正
- cache deleteをidempotentに
- MCP tool callとloadMsg URLにtimeout付与
- prompt echo (`-P`フラグ) の修正

---

## 2. 優先順位付き修正リスト（PR#1〜#13 完了、PR#14〜#15 追加）

### Tier 1: クラッシュ / データ競合（✅ 全件完了）

**1-D. OpenAI `CallTools()` の空 Choices panic** ✅ `3ef59dc`
- 箇所: `internal/openai/openai.go:121`
- 問題: `s.message.Choices[0]` に無条件アクセス。`Next()` が一度も true を返さない場合（空ストリーム）に `CallTools()` が呼ばれると index out of range panic。
- 対処: `if len(s.message.Choices) == 0 { return nil }` を `CallTools()` 冒頭に追加。1行変更。

**1-B. Ollama `Current()` の busy-loop + `s.err` data race** ✅ `6789fbc`
- 問題: `default:` で即 `ErrNoContent` を返すノンブロッキング設計のため高速ループが発生。goroutine からの `s.err` 書き込みも無保護で data race。
- 対処: `Current()` をブロッキング受信に変更。goroutine は errCh 経由でエラーを送信、`s.err` 書き込みは `Current()` のみ。

**1-C. Ollama `Close()` の closed channel panic** ✅ `6789fbc`（1-B と同一 PR）
- 対処: goroutine が `defer close(respCh)` を所有。`Close()` は `cancelFn()` を呼ぶだけ。

**1-A. `cancelRequest` への並行 append+range — data race** ✅ `61e5b7a`
- 対処: `cancelRequest` フィールドを完全削除。MCP context は即時 cancel、ToolCaller は `defer cancel()`。

**1-E. Google `Close()` の nil response panic + `resp.Body` リーク** ✅ `24f40b5`
- 対処: `Close()` に nil ガード追加。エラーレスポンス時に `resp.Body.Close()` を追加。

**1-F. Google API エラー時の二重nilパニック**（2026-07-01発見、PR#19）
- 箇所: `internal/google/google.go` の `googleSendRequestStream`（315-336行目）と `Request`（133-137行目）と `handleErrorResp`（172-183行目）
- 問題1: `googleSendRequestStream` がAPIエラー時（HTTPリクエスト失敗・非2xxレスポンス）に `new(Stream)`（ゼロ値、`reader`が`nil`）を返すが、`isFinished`もゼロ値`false`のまま。呼び出し元 `receiveCompletionStreamCmd`（mods.go:491）は `Next()`（`!isFinished`）が`true`を返すため `Current()` を呼び、`nil`の`reader`への `ReadBytes` 呼び出しでpanic（goroutine crash）。`Request()` 内のリクエスト構築失敗パス（133-137行目）も同型のバグを持つ。PR#4（1-E）はClose()のnilガードのみ対処しており、この`Current()`側のクラッシュは見逃されていた
- 問題2（問題1修正で新たに露見）: `handleErrorResp` がGoogleのエラーレスポンスをOpenAI SDKの `openai.Error` 型に無理やり詰めているが、`Request`/`Response`フィールドを設定していない。`(*openai.Error).Error()` はこれらのフィールドを前提としており（`r.Request.Method`等）、エラーメッセージを描画しようとした瞬間に別のnilパニックが発生する。問題1の修正で初めて到達可能になり顕在化した
- 実害: Google/Gemini APIのエラー（認証失敗・不正モデル名・レート制限等）が**すべて**、適切なエラーメッセージではなくクラッシュになっていた。実際のGemini APIキーで動作確認中に発見
- 対処: (1) `googleSendRequestStream`のエラーパスと`Request`のリクエスト構築失敗パスで `isFinished: true` を設定 (2) `handleErrorResp` で `errRes.Request = resp.Request` / `errRes.Response = resp` を設定
- 回帰テスト: `internal/google/google_test.go` に `TestSendRequestStreamAPIErrorNoPanic` を追加（修正前は失敗することを確認済み）

**1-G. Anthropic `temperature`/`top_p` 同時指定で常に400エラー**（2026-07-01発見、PR#21）
- 箇所: `internal/anthropic/anthropic.go` の `Request`（40-46行目）
- 問題: `Config.Temperature`と`Config.TopP`が両方非ゼロのとき、両方を無条件でAnthropicリクエストボディに設定していた。Anthropic APIは`temperature`と`top_p`の同時指定を拒否する（`400 Bad Request: temperature and top_p cannot both be specified for this model`）。同梱の`config_template.yml`はグローバルに`temp: 1.0`・`topp: 1.0`の両方を設定しているため、**デフォルト設定のまま使うと全てのAnthropicリクエストが必ず失敗する**
- 実害: OpenAI/Google/Ollamaでは温度・top_pの同時指定が許容されるため今まで見逃されていた。実際のAnthropic APIキーで動作確認中に発見
- 対処: `Temperature`が設定されている場合は`TopP`を送らないよう`else if`に変更
- 回帰テスト: `internal/anthropic/anthropic_test.go`（新規）に`TestRequestOmitsTopPWhenTemperatureSet`を追加（修正前は失敗することを確認済み）

---

### Tier 2: 正確性・ロジックバグ（✅ 全件完了）

**2-A. tool call ループ上限なし** ✅ `3852672`
- 対処: `Config.MaxToolCalls int`（デフォルト 0 = 無制限）追加。`toolCallRound` フィールドを `completionOutput` で propagate。YAML テンプレート・flag 追加済み。

**2-B. Google `Current()` エラーで `isFinished` が未設定** ✅ `ea52b5d`
- 対処: 全エラーパスで `s.isFinished = true` を設定。

**2-C. グローバル `config` への直参照** ✅ `ea52b5d`（2-B と同一 PR）
- 対処: 3箇所を `cfg.MCPTimeout`、`cfg.FormatAs` に統一。

~~**2-D. `ptrOrNil` の 0 値扱い**~~ → **廃止**  
~~**2-E. `retry()` の blocking sleep**~~ → **廃止**

---

### Tier 2（新規追加）: コードレビュー指摘

**R-1. `max-completion-tokens` が request に渡っていない** ⬜ PR#14
- 箇所: `config.go:150`（`MaxCompletionTokens`）、`mods.go:374`（コメントのみ）、`internal/proto/proto.go:76`（フィールドなし）、`internal/openai/openai.go`
- 問題: `Config.MaxCompletionTokens` は YAML/env から読まれるが `proto.Request` に渡されず、OpenAI リクエストにも設定されない。`mods.go:374` のコメント「We do set max_completion_tokens instead」は実装が伴っていない嘘コメント。o1 系モデルでは `max_tokens` を 0 にするが `max_completion_tokens` が設定されないためトークン上限が完全に無効になる。加えて `config_template.yml:229` 等に model レベルの `max-completion-tokens` が記述されているが `Model` struct にフィールドがないため YAML 上は無視される。
- 対処:
  1. `proto.Request` に `MaxCompletionTokens *int64` を追加
  2. `mods.go` で `cfg.MaxCompletionTokens > 0` のとき `request.MaxCompletionTokens` に設定
  3. `internal/openai/openai.go` で `MaxCompletionTokens` を OpenAI リクエストに渡す
  4. `Model` struct に `MaxCompletionTokens int64 yaml:"max-completion-tokens"` を追加し、model レベル設定も有効化（オプション）
  5. `mods.go:374` の誤コメントを実装に合わせて修正
- リスク: 中（3ファイル変更。o1 以外への副作用注意）
- 発見元: コードレビュー指摘 P1

**R-2. `api-key-env` と `api-key-cmd` の優先順位バグ** ⬜ PR#14（R-1 と同一 PR）
- 箇所: `mods.go:445`
- 問題: `api.APIKeyCmd == ""` 条件のせいで、両方設定されると `api-key-env` がスキップされ `api-key-cmd` が使われる。`docs/notes/feature-requirements.md:125-127` の仕様は `api-key-env` > `api-key-cmd` の順。
- 対処: `&& api.APIKeyCmd == ""` の条件を削除。1行変更。env を試してから cmd にフォールバックする流れになる。
- リスク: 低（1行削除。両方設定している既存ユーザーは動作変化するが、仕様への準拠が優先）
- 発見元: コードレビュー指摘 P2

---

### Tier 2.5: MCP 接続キャッシュ（✅ 完了）

**3-A. MCP 接続を毎回再生成している** ✅ `5ba3e98`
- 対処: `mcpClientPool`（mutex + map）を `mcp.go` に追加。`Mods` に `mcpPool *mcpClientPool` フィールド、`quit()` で `closeAll()`。`(m *Mods) mcpTools` / `(m *Mods) toolCall` がセッション内でクライアントを再利用。package-level `mcpTools`（`--mcp-list-tools` 用）は従来通り都度作成・Close。

---

### Tier 3: 設計改善（✅ 全件完了）

**3-B. MCP context と LLM request context の混在** ✅ `d36df0d`
- 対処: コメントで意図を明示。

**3-C. `cancelRequest` の cancel 蓄積** ✅ `61e5b7a`（1-A と統合済み）

**3-D. `Config.System` フィールド削除** ✅ `d36df0d`
- 対処: フィールド削除、後方互換コメント追加。

**3-E. `stream` 変数名とパッケージ名のシャドウ** ✅ `ea52b5d`（2-B と同一 PR）
- 対処: 変数名を `s` に変更。

---

### Tier 4: 依存ライブラリ更新（✅ 全件完了）

| PR | 内容 | コミット | 実績 |
|---|---|---|---|
| #5 | x/net + x/crypto CVE 更新 | `49f0731` | コード変更なし |
| #6 | sqlite v1.46.1 → v1.53.0 | `78ed0f3` | コード変更なし |
| #9 | anthropic-sdk-go v1.26.0 → v1.50.1 | `a87bf70` | コード変更なし（高リスク予測は外れ） |
| #10 | mcp-go v0.45.0 → v0.54.1 | `5ba3e98` | API 互換のまま |
| #11 | ollama v0.17.7 → v0.30.8 | `bd13dac` | コード変更なし（API 互換確認済み） |
| #13 | glamour v0.10.0→v1.0.0, huh v0.8.0→v1.0.0 | `7c79cb0` | コード変更なし（高リスク予測は外れ） |

---

## 3. 修正のグルーピング（参考）

> **PR番号の正式な定義はセクション4が一次情報。** このセクションは「なぜ同一PRにまとめるか」の理由を補足する参考資料。

### グループ A — Tier 1 クラッシュ系（完了）

| 含む修正 | PR |
|---|---|
| 1-D（OpenAI 1行ガード） | #1 |
| 1-B + 1-C（Ollama channel 設計変更） | #2 |
| 1-A + 3-C（cancelRequest 削除 + defer cancel） | #3 |
| 1-E（Google nil panic + body リーク） | #4 |

### グループ B — Tier 2 正確性系（完了）

| 含む修正 | PR |
|---|---|
| 2-B + 2-C + 3-E（Google isFinished / global config / stream 変数名） | #7 |
| 2-A（MaxToolCalls 追加） | #8 |

### グループ C — Tier 2.5〜3 設計改善（完了）

- **3-A + M5 + 4-5（mcp-go）→ PR#10**: 接続キャッシュ設計変更と SDK 更新を同時実施。
- 3-C は 1-A と統合（PR#3）
- 3-B、3-D は PR#12 でまとめて完了

### グループ D — 依存更新バッチ（完了）

| 含む更新 | PR |
|---|---|
| 4-1（x/net）+ 4-2（x/crypto） | #5 |
| 4-3（sqlite）| #6 |
| 4-4（anthropic-sdk-go）| #9 |
| 3-A + M5 + 4-5（mcp-go）| #10 |
| 4-6（ollama）| #11 |
| 4-7（glamour + huh）| #13 |

---

## 4. 推奨実施順（✅ 全 PR 完了）

```
── 公開前に必須（Tier 1：全プロバイダのクラッシュ修正）──────────────────────
PR #1  ✅ 3ef59dc  1-D                  OpenAI Choices guard（1行）
PR #2  ✅ 6789fbc  1-B + 1-C            Ollama channel 設計変更
PR #3  ✅ 61e5b7a  1-A + 3-C            cancelRequest: 削除 + defer cancel
PR #4  ✅ 24f40b5  1-E                  Google nil panic + body リーク

── 依存セキュリティ ──────────────────────────────────────────────────────────
PR #5  ✅ 49f0731  Dep [1]              x/net + x/crypto CVE 更新
PR #6  ✅ 78ed0f3  Dep [2]              sqlite 更新

── 正確性・ロジック修正 ───────────────────────────────────────────────────────
PR #7  ✅ ea52b5d  2-B + 2-C + 3-E     Google isFinished / global config / stream 変数名
PR #8  ✅ 3852672  2-A                  MaxToolCalls 追加（デフォルト 0 = 無制限）

── 依存更新（単独 PR）───────────────────────────────────────────────────────
PR #9  ✅ a87bf70  Dep [3]              anthropic-sdk-go 更新

── MCP 強化 ──────────────────────────────────────────────────────────────────
PR #10 ✅ 5ba3e98  3-A + M5 + Dep [4]  MCP 接続キャッシュ + errgroup 改善 + mcp-go 更新

── 残り依存更新 ──────────────────────────────────────────────────────────────
PR #11 ✅ bd13dac  Dep [5]              ollama 更新

── 後回し設計改善 ────────────────────────────────────────────────────────────
PR #12 ✅ d36df0d  3-B + 3-D            context コメント整理 / Config.System 削除
PR #13 ✅ 7c79cb0  Dep [6]              glamour + huh メジャー更新

── コードレビュー指摘修正 ────────────────────────────────────────────────────
PR #14 ✅ 73284c0  R-1 + R-2            max-completion-tokens 配線 + api-key 優先順位修正
PR #15 ✅ 9839de9  doc                  overview.md の陳腐化修正
PR #16 ✅ 649e925  refactor             mcp.go グローバル config 参照を *Config 引数渡しに統一

── 新機能 ────────────────────────────────────────────────────────────────────
PR #17 ✅ 7749ccc+bdc20b9  feature   --output json 追加（AI→AI連携向けJSON出力モード、Phase 1）

── コードレビュー指摘修正（実ゲートウェイ検証で発見）────────────────────────
PR #18 ✅ 53a95ad  fix              MaxChars=0（未設定）時に入力プロンプトが空文字列へ切り詰められるバグ修正

── Google/Geminiプロバイダのクラッシュ修正（実APIキーでの検証で発見）───────
PR #19 ✅ 24eaca8  fix              Google API エラー応答時の二重のnilパニックを修正

── config_template.yml メンテナンス ──────────────────────────────────────────
PR #20 ✅ dc1eafe  doc              Gemini モデル一覧を "-latest" 追従エイリアス3つに更新（旧 gemini-1.5-*-latest は404で廃止済み）

── Anthropicプロバイダの400エラー修正（実APIキーでの検証で発見）───────────
PR #21 ✅ 9fe9070  fix              temperature と top_p 同時指定で常に400エラーになるバグ修正

── config_template.yml メンテナンス（続き）───────────────────────────────────
PR #22 ✅ (未コミット)  doc          Anthropic モデル一覧を最新世代のみに更新（sonnet-5, opus-4.8, fable-5, haiku-4.5）

── 公開直前（コード修正後）★ 次フェーズ ──────────────────────────────────────
       ⬜ README 更新          上流との関係・変更内容・推奨設定（max-tool-calls等）の記載
       ⬜ モジュール名変更     go install 用に module パスを一括変更（import 全体に波及）
```

---

## 5. 次バージョンで検討（リファクタリング候補）

Opus によるコードレビュー（2026-06-26）で「今は触らなくてよいが将来負債になりうる」と判定された項目。公開後・メンテナンスの余裕ができた段階で検討する。

### R-A. `mods.go` の分割（781行、責務が4つ混在）

自然な分割境界が4つ存在する:

| 責務 | 現在の箇所 | 移動先候補 |
|---|---|---|
| Bubble Tea モデル層（`Init`/`Update`/`View`/viewport） | mods.go:38〜255, 642〜670 | `tui.go` |
| completion 実行層（`startCompletionCmd`・`resolveModel`・`receiveCompletionStreamCmd`） | mods.go:272〜537 | `completion.go` |
| キャッシュ/会話ID解決層（`findCacheOpsDetails` 等） | mods.go:539〜640 | `cache_ops.go` |
| 小ユーティリティ（`removeWhitespace`・`cutPrompt`・`ptrOrNil` 等） | mods.go:672〜780 | `util.go` |

特に `startCompletionCmd`（177行・1関数）は API 種別ごとのクライアント構築・proxy 設定・トークン調整・MCP・ストリーム開始まで全部抱えており、プロバイダ追加のたびに膨らむ構造的負債。

### R-B. Stream ステートマシンの重複

openai/anthropic/ollama の `Stream` が `done`/`factory`/`Next` での再ストリーム生成という同型パターンを各自実装している。tool call ラウンドの state 遷移が3箇所に分散しており、1箇所修正して他を直し忘れるリスクがある。ただし各 SDK の型が異なるため安易な統一は逆効果になりうる。実害が出た段階で検討。

### R-C. 命名の小さな不整合

- `proto.Request.ToolCaller`（フィールド）/ `stream.CallTool`（関数）/ `completionOutput.errh`（略称）が混在。`proto`/`stream` は公開に近いため揃える価値がある
- `Mods.content`（`[]string` の未フラッシュ生出力バッファ）と `completionOutput.content`（chunk文字列）が同名で紛らわしい。前者は `pendingRaw` 等の意味的な名前の方が役割に合う

### R-D. `Config` 構造体のフィールド分離

`config.go:142-201` で YAML タグ付き永続設定値（`Format`・`MaxTokens` 等）とランタイム専用フラグ（`ShowHelp`・`Dirs`・`cacheReadFromID` 等）が1つの構造体に混在し、インデントも乱れている。「永続設定」と「セッション状態」を型で分離すると、グローバル `config` 問題の再発防止にもなる。大規模変更のため次バージョン向け。

### R-E. MCP ツール名のセパレータ問題（`mcp.go:225`）

MCP ツール名を `{サーバ名}_{ツール名}` 形式で広告・逆引きしているが、区切り文字が `_` のためサーバ名自体に `_` を含むと誤ルーティングする。

- `strings.Cut(name, "_")` は最初の `_` で分割するため、`my_server` サーバの `list` ツールが `my` サーバの `server_list` として解釈される
- 逆引き時は `m.Config.MCPServers[sname]` で存在チェックされるため越境は起きないが、`my_server` を使う設定では機能しなくなる
- セキュリティ実害（権限昇格・越境）はなく純粋な命名バグ

修正方針: 区切り文字を衝突しにくい文字列（例: `__` ダブルアンダースコア、または `-`）に変えるか、広告側でエスケープして逆引き時にアンエスケープする。変更すると既存の会話キャッシュとの後方互換が壊れる可能性があるため、移行処理も含めて設計すること。

出典: セキュリティスキャン（2026-06-26）Medium 指摘を Opus が「機能バグ、セキュリティ実害なし」と再評価。

---

## 6. 廃止・後回しの根拠（旧Tier）

| 項目 | 判断 | 理由 |
|---|---|---|
| 2-D（ptrOrNil の 0 値） | **廃止** | `config_template.yml` で `temp:1.0 / topp:1.0 / topk:50` と正の値が設定済み。デフォルト通りに使う限り実害なし |
| 2-E（retry blocking sleep） | **廃止** | `retry()` は tea.Cmd goroutine 内から呼ばれるため `time.Sleep` は Update ループをブロックしない。race条件も実害なし |
| P3（overview.md 陳腐化） | PR#15 | コードバグではなくメモのズレ。PR#7 で修正済みの Google `Messages()` の記述が残っている。PR#15（doc 修正）で対処 |

---

## 7. 主要ファイル一覧

| ファイル | 関連修正 | 状態 |
|---|---|---|
| `mods.go` | 1-A, 2-A, 2-C, 3-B, 3-C | ✅ |
| `internal/ollama/ollama.go` | 1-B, 1-C | ✅ |
| `internal/google/google.go` | 1-E, 2-B, 3-E | ✅ |
| `internal/openai/openai.go` | 1-D | ✅ |
| `internal/anthropic/anthropic.go` | 4-4 | ✅（コード変更不要だった） |
| `mcp.go` | 3-A, M5 | ✅ |
| `config.go` | 3-D | ✅ |
| `go.mod` | 4-1 〜 4-7 | ✅ |

---

## 8. 検証メモ（調査で判明した事実）

- `receiveCompletionStreamCmd` は `tea.Cmd`（goroutine）として実行される → `Current()` のブロッキング化は安全
- `retry()` は tea.Cmd goroutine 内から呼ばれる → `time.Sleep` は Update ループをブロックしない
- `config_template.yml`: `temp:1.0 / topp:1.0 / topk:50`（すべて正値）→ 2-D は不要
- `format.go:97` に `// anthropic v1.5 removed this method` コメントあり → 4-4 は「高」リスクと予測したが、実際は v1.26→v1.50 でコード変更不要だった
- `errgroup.Group`（素）が `mcp.go:66` で確認済み → PR#10 で `errgroup.WithContext` に変更
- `main.go:748`: `_ = cache.Delete(id)` が L4 の実体（DB保存失敗時の孤立cache削除エラーを無視）
- ollama v0.17.7 と v0.30.8 の API 構造体は同一 → PR#11 は単純 version bump（コード変更不要）
- anthropic-sdk-go v1.26→v1.50: コード変更不要（高リスク予測は外れた）
- glamour v0.10→v1.0 / huh v0.8→v1.0: コード変更不要（高リスク予測は外れた）
- mcp-go v0.45→v0.54.1: `go.sum` に追加エントリ（jsonschema-go, santhosh-tekuri/jsonschema）が必要。`go get` でサブモジュールも指定すれば解決
- MCP 接続キャッシュ（PR#10）: `mcpClientPool` を `mcp.go` に定義し、`Mods` に `*mcpClientPool` ポインタを持たせることで `mods.go` への mcp-go import 追加を回避できた
- `max-completion-tokens`（R-1）: `Config.MaxCompletionTokens` は YAML/env から読まれるが `proto.Request` にフィールドなし → OpenAI リクエストに未設定。`mods.go:374` コメントは実装と乖離。o1 系モデルでトークン上限が完全無効になる実害あり。`config_template.yml:229` 等の model レベル設定も `Model` struct にフィールドなく無視
- `api-key-env` 優先順位（R-2）: `mods.go:445` の `&& api.APIKeyCmd == ""` 条件により、両方設定時に env がスキップされ cmd が使われる。仕様書の順序（env → cmd）と逆。1行削除で修正可能
- `go test ./... -cover` の `covdata` ツール欠落エラー: Go toolchain 側の問題でコード失敗ではない（`go test -race` は全通過）
- `mcp.go` グローバル `config` 参照（PR#16）: `enabledMCPs`・`isMCPEnabled` はグローバル `config` を読んでいたが、`(m *Mods) toolCall` もメソッドでありながら同様。本番は `m.Config == &config` で同一ポインタのため無害だが、テスト時に別 `Config` を注入すると MCP ロジックが壊れる。`*Config` を引数に変えて解消（`649e925`）
- Opus リファクタリングレビュー（2026-06-26）: `stream.Stream` インターフェース・並行処理・エラーハンドリングは問題なし。主要な設計問題は `mcp.go` グローバル参照のみ（PR#16 で解消）。残りは次バージョン候補（セクション5）
- セキュリティスキャン（2026-06-26）: Medium 2件・Low 3件の指摘。Google API key の URL 埋め込み（Medium）は `x-goog-api-key` ヘッダ化で修正（`d2b4c40`）。残り4件は Opus が評価し、セキュリティ実害なしと判定: sha.go regex 非アンカー（URL 注入経路なし）・google.go 行長無制限（自己設定エンドポイントのみ）・MaxToolCalls=0 無制限（意図的仕様）・MCP ツール名衝突（機能バグとして R-E に記録）
- `MaxChars` サイレント切り詰めバグ（PR#18、2026-07-01発見）: `stream.go:47` の `content[:mod.MaxChars]` は、モデル個別 `max-input-chars` もグローバル `max-input-chars` も未指定（共に0）のとき `content[:0]` で入力プロンプトを完全に切り詰める。エラーは出ず空メッセージがそのままAPIに送信される。実ローカルゲートウェイ（mlx-community/gemma-4-E2B-it-qat-4bit）に対し `--output json` を検証中、テンプレートに無い新規APIを最小構成で追加したところ実際に踏んだ。Opus によるレビューで、本家コミット `ff9a598`（"fix: use default max input settings"）由来の古いロジック欠陥でありフォークでの新規発生ではないと判定。同梱の `config_template.yml` がグローバル・モデル個別ともに `max-input-chars` を設定しているため通常利用では顕在化しない。修正: `mod.MaxChars > 0` を条件に追加し、未設定(0)を無制限として扱う（`--no-limit`/`NoLimit` の意味論と整合）
