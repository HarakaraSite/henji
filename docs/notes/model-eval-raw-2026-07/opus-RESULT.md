# henji 利用能力テスト結果
- ドライバーモデル: claude-opus-4-8 (Claude Opus 4.8)
- 日時: 2026-07-04 16:48 JST

## 情報源の利用

### `-h` を読んだタイミングと、それだけで書けたコマンド
- 最初（作業開始直後）に `./henji -h` を実行。
- `-h` だけで書けたコマンド:
  - `./henji --list-models` / `./henji --list-models --output json`（設定済みモデルの確認）
  - タスク1の基本要約（`cat access.log | ./henji -q "..."`）。`-q` が「スピナー・成功メッセージを stderr で隠す」ことは `-h` の記述から判断。
  - モデル指定（`-m`）とプロバイダ指定（`-a`）の存在、`--json-schema`・`--continue`・`--title` の存在も `-h` で把握。
  - `-h` の Examples から「stdout は応答のみで安全にパイプできる」設計と `--json-schema` の使い方の当たりを付けた。

### `docs` を読んだタイミングと、docs が必要になった理由
- タスク2に入る前に `./henji docs` を読んだ。
- 必要になった理由:
  - **出力契約の確定**: 「stdout は応答のみ / スピナー・保存通知・エラーは stderr」「TTY 以外では ANSI なしのプレーンテキスト」という契約を明文で確認し、要約テキストだけを確実に保存できると裏付けたかった。
  - **`-m` と `-a` の関係**: `-h` では `-a` の説明が「pair with -m/--model」だけで、default-model が特定 API に属するため `-a openai -m 5.5` のように必ずペアで指定する必要がある、という点が docs で明確になった（最初にこれを読む前は `-m 5.5` 単独で失敗した）。
  - **構造化出力の3段階**: `--format-as json`（未検証）/ `--output json`（エンベロープ）/ `--json-schema`（ネイティブ構造化出力＋クライアント検証＋リトライ）の違いと、`--json-schema` 単独ならエンベロープなしで検証済み JSON がそのまま stdout に出ることを確認。タスク2ステップ1に最適と判断。
  - **会話継続の作法**: `-t <title>` で命名 →`-c <title>` で継続、継続時は履歴全体が再送される、という仕様を確認。ステップ2を「同じ会話の続き」にする方法を確定できた。
  - 小さいローカルモデルは JSON をコードフェンスで包む/スキーマ無視の可能性がある、との注意書きを見て、プロンプトに「raw JSON only, no code fences」を加えた。

## タスク1

### ローカルモデル
- 成否: **成功** → `task1-summary.local.txt`
- 試行コマンド一覧（時系列）:
  1. `cat access.log | ./henji -q "Summarize this access log concisely."` → 成功。`-q` により stderr は空、stdout は要約テキスト（Markdown）のみでクリーン。そのまま保存。
- つまずいた点と解決方法: 特になし。`-q` で装飾・スピナーは stdout に混入しないことを最初の1回で確認できた。

### クラウドモデル
- 成否: **成功** → `task1-summary.cloud.txt`
- 試行コマンド一覧（時系列）:
  1. `cat access.log | ./henji -q -m 5.5 "Summarize this access log concisely."` → **失敗**。`ERROR: The API endpoint gateway does not contain the model 5.5`。default API（gateway）にはローカルモデルしか無いため。
  2. `cat access.log | ./henji -q -a openai -m 5.5 "Summarize this access log concisely."` → 成功。stdout に要約のみ、stderr は空。
- つまずいた点と解決方法: `-m 5.5` 単独ではエラー。クラウドモデル `gpt-5.5` は `openai` API 配下なので、`-a openai` とペア指定して解決（docs の「pair -a with -m」記述と一致）。

## タスク2

### ローカルモデル
- 成否: **成功** → `task2-findings.local.json`（有効な JSON, findings 4件）/ `task2-detail.local.txt`
- 試行コマンド一覧（時系列）:
  1. ステップ1: `cat -n sample.go | ./henji -q -t goreview-local --json-schema <schema> "Review this Go source code ... Return raw JSON only, no code fences. The 'line' field must be the source line number ..."` → 成功。指定スキーマ通りの JSON を出力。`cat -n` で行番号を渡し、`line` を正確に埋めさせた。`jq .` で妥当性検証後に保存。
  2. ステップ2: `./henji -q -c goreview-local "Take the SQL injection finding (line 20) ... Explain in detail how to fix it in Go ..."` → 成功。ステップ1の指摘を踏まえた続きの回答（パラメータ化クエリでの修正コードと理由）を得て保存。
- つまずいた点と解決方法: 特になし。事前に「raw JSON only」をプロンプトへ入れておいたため、ローカル小型モデルでもコードフェンス混入・スキーマ違反は発生しなかった。会話は `-t` で命名 →`-c` で継続でき、`./henji -l` で継続（更新）を確認。

### クラウドモデル
- 成否: **成功** → `task2-findings.cloud.json`（有効な JSON, findings 8件）/ `task2-detail.cloud.txt`
- 試行コマンド一覧（時系列）:
  1. ステップ1: `cat -n sample.go | ./henji -q -a openai -m 5.5 -t goreview-cloud --json-schema <schema> "Review this Go source code ... Return raw JSON only ..."` → 成功。スキーマ通りの JSON、`jq` 検証後に保存。
  2. ステップ2: `./henji -q -a openai -m 5.5 -c goreview-cloud "From the findings you just reported, pick the SQL injection issue and explain in detail how to fix it ..."` → 成功。「あなたが今報告した findings の中から」と参照でき、line 20 を特定して継続回答。文脈保持を確認。
- つまずいた点と解決方法: 継続（`-c`）時も新しいターンをクラウドで処理させるため `-a openai -m 5.5` を再指定する必要がある点に注意（無指定だと default のローカルモデルで応答してしまう）。ペア指定を毎回付けて解決。

## ローカル/クラウドの違いで気づいたこと
- **モデルの所属 API**: ローカル `mlx-community/gemma-4-E2B-it-qat-4bit`（alias `local`）は default の `gateway` API、クラウド `gpt-5.5`（alias `5.5`）は `openai` API。クラウド利用時は `-a openai -m 5.5` のペア指定が必須。
- **要約の質・粒度**: クラウド（gpt-5.5）は総リクエスト数57・ステータスコード内訳・時間帯・トップIPなど定量的に集計した密度の高い要約を返した。ローカル（gemma）は定性的な観察（チェックアウトの500エラー、スキャン痕跡など）中心で、数値の網羅性はクラウドに劣る。
- **コードレビューの網羅性**: 同じ `sample.go` に対しローカルは4件（SQLi・nil db パニック・データ競合・エラー無視）、クラウドは8件（上記に加え、部分読み込み・fd リーク・エラー詳細のクライアント露出・`fmt.Fprintf` 経由の XSS など）を検出。クラウドの方が明らかにカバレッジが広い。
- **構造化出力の安定性**: 「raw JSON only」を明示した結果、ローカルでも一度でスキーマ準拠 JSON を返せた。docs が警告する「小型モデルのフェンス混入/スキーマ無視」は今回のプロンプト工夫で回避できた。

## henji への改善提案
- **-h に足りなかった情報**:
  - `-a`（API）と `-m`（model）の関係が「pair with -m」だけで、なぜペアが必須か（default-model が特定 API に属する）が -h だけでは分かりにくい。`-m <alias>` 単独指定でエラーになる典型例を Examples に一つ入れると親切。
  - `--json-schema` を単独指定した場合の出力（エンベロープ無しの検証済み生 JSON が stdout に出る）と、`--output json` と組み合わせた場合の違いが -h だけでは読み取れない。
  - 会話継続（`-c`/`-C`）で新ターンを別モデルにしたいとき、`-a/-m` を再指定しないと default に戻る、という挙動への一言があると良い。
- **docs に足りなかった情報・分かりにくかった箇所**:
  - 「A typical agent loop」は `--output json` + `conversation_id` 方式のみ紹介。`-t <title>` で命名して `-c <title>` 継続する方式（今回採用）も並記されると、生 JSON をファイル保存しつつ継続する用途に辿り着きやすい。
  - 継続時にどのモデル/API が使われるか（履歴は再送されるが応答モデルは -a/-m の指定に従う）が明記されておらず、実験で確かめる必要があった。
  - `line` のような「入力の行番号」を正確に埋めさせるには入力側で行番号付与（`cat -n` 等）が要る、という実務的コツはモデル側の責任範囲だが、構造化出力の pitfalls に補足があると良い。
- **その他**:
  - `--list-models --output json` の出力に、各モデルがローカルかクラウドか（`[local]` 相当のメタ）が JSON 側にも入ると、スクリプトからローカル/クラウドを自動判別しやすい（テキスト出力には `[local]` が付くが JSON には無い）。
