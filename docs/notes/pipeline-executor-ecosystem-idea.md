# 周辺エコシステム構想: 計画→実行→分析パイプライン（別リポジトリ）

ステータス: **アイデア段階のみ**（2026-07-05 ブレスト）。henji本体には手を入れない前提。実装するなら**別リポジトリ**。

## 発端

henjiが`--json-schema`で構造化JSONを吐けることを利用し、次の3段構成を作る:

```
henji(計画=典型JSON) | <エグゼキュータ> | henji(分析)
```

henjiは両端（計画と分析）だけを担い、真ん中の副作用（ネットワーク/実行/検索）は決定的な別ツールに委ねる。
これはMCPを完全削除した設計思想（`stdin→LLM→stdout`の純粋フィルタに保ち、副作用はシェル側に出す）の「正しい続き」。

## 既に踏んでいる実例

proxmoxプロジェクトでのCaddyログ分析が、このパターンの別インスタンス:

```
henji(SQLを書く) | duckdb(実行) | henji(結果を分析)
```

`duckdb`を「JSONを食う汎用エグゼキュータ」に一般化すれば、HTTP取得もfindもSQLも同じ骨格になる。

## 中核アイデア: マニフェスト駆動エグゼキュータ（1本のバイナリ）

候補ツールのほとんどは「**入力JSON → 既存CLIのargv**」への写像でしかない。
→ N個のバイナリを量産せず、**1本のマニフェスト駆動エグゼキュータ**にする。

- ツールごとに小さなマニフェスト（許可フィールド → argvテンプレート）を持つ
- 入力JSONをマニフェストで検証してから実行
- **許可フィールド集合そのものが安全境界**（例: `find`のマニフェストに`-exec`/`-delete`を定義しなければ、LLMがどう出力しても発火し得ない）
- `--dry-run` / `--allow <hostglob>` / gate を全ツール共通で1箇所に実装

これなら find / rg / git-log / curl / duckdb / kubectl-get が、マニフェスト追加だけで増える。

## JSON契約（HTTPの例）

```jsonc
// 入力 (stage1 henji が --json-schema で吐く)
{ "method": "GET", "url": "https://…", "headers": {}, "body": null, "timeout_ms": 5000 }
// 出力 (stage3 henji が食う)
{ "status": 200, "headers": {…}, "body": "…", "elapsed_ms": 123, "error": null }
```

## パターンにハマる候補

### 読み取り専用クエリ系（安全設計がほぼ不要 = 最初に作るべき層）
- `find` — `{path, name, mtime, size, type}`。引数を人間も覚えられないのでNL→specの価値が高い。**`-exec`/`-delete`はspecに含めない**
- `rg`/`grep` — `{pattern, globs, flags}` → マッチ → 「バグを指摘して」
- `git`（log/diff/blame/show の読み取りのみ）— 「この関数を最後に触ったのは誰・なぜ」
- `kubectl get`/`describe`、`aws … describe`/`logs filter`、`gcloud … list` — proxmoxログ分析の延長
- `du`/`ls`/`stat` — 「何がディスクを食ってる」
- SQL（`duckdb`/`sqlite`/`psql` SELECT）— 既出の実例

### 副作用あり（gate/dry-run必須の層）
- HTTP（POST含む）、`git commit`/`push`、`kubectl apply`、パッケージ操作

## 追加の小道具: 「ゲート」パイプ部品

段の間に挟んでJSONを整形表示し、y/nで通過を確認するだけの部品:

```
henji(計画) | gate | <エグゼキュータ> | henji(分析)
```

「Generate → review → run」をそのままパイプの1コンポーネント化。汎用なのでSQL計画にもシェル計画にも使い回せる。
READMEに新設した「見てから実行」節と思想が一致。

## 安全設計の勘所

この構成は「LLMが決めたURL/コマンドを機械が実行する」ので、MCPで問題にしたconfused deputyの構図が“パイプの外側に”再登場する。
違いは**人間が意図的にパイプを組み、間に検査を差し込めること**。だからエグゼキュータ側でそれを既定で楽にする:

- `--dry-run`（既定にしてもよいくらい）
- `--allow`（ホストallowlist、SSRF/内部IP対策）
- `--max-bytes`（レスポンス肥大でstage3のコンテキストが溢れるのを防ぐ）

## 気になる点

- **モデル往復2回** → 対話用途なら許容。バッチ多用ならstage1をローカル小モデル・stage3を賢いモデルと使い分け
- **ローカル小モデルのJSON信頼性** → henji側`--json-schema-retries`で担保。計画段のスキーマは素直に保つ
- **どこまでhenjiに寄せるか** → `henji fetch`のような本体内蔵はしない。別リポの兄弟ツール群が落とし所

## 次アクション（未定）

- まだアイデアのみ。着手する場合は「読み取り専用クエリ系 + マニフェスト駆動 + gate」から。
- henji本体のロードマップ（`fix-roadmap.md`、次は`-f/--file`→`--image`）とは独立トラック。
