# henji 利用能力テスト結果

- ドライバーモデル: Claude Haiku 4.5
- 日時: 2026-07-04

## 情報源の利用

### `-h` を読んだタイミング
初めにヘルプを確認しました。これだけで以下のコマンドが書けました：
- `--list-models` でモデル一覧を確認
- `-q` オプションで静かな出力
- `-m` / `-a` でモデル・API指定
- `-t` で会話を保存
- `-c` で会話を続行

### `docs` を読んだタイミング
`-h` でモデル指定が失敗した（`-m openai` だけでは駄目）ときに `docs` を確認しました。
docs から以下の情報を得ました：
- API と モデルの対応関係（`-a <api> -m <model>` で指定）
- 構造化出力（`--json-schema`）の詳細
- Markdown フェンス問題の対策（プロンプトに "raw JSON only" 追加）
- OpenAI スキーマ要件（`additionalProperties: false` が必須）

## タスク1

### ローカルモデル

**成否:** ✓ 成功

**試行コマンド一覧（時系列）:**
1. `cat access.log | ./henji -q "この Apache ログを簡潔に要約してください"`

**つまずいた点と解決方法:** なし。初回で成功。

**結果:** `task1-summary.local.txt` に要約テキストのみを保存。

---

### クラウドモデル

**成否:** ✓ 成功

**試行コマンド一覧（時系列）:**
1. `cat access.log | ./henji -m openai -q "..."` → エラー（API指定失敗）
2. `cat access.log | ./henji -a openai -m gpt-5.5 -q "..."` → 成功

**つまずいた点と解決方法:**
- `-m openai` だけではAPI指定が不完全
- `-a openai -m gpt-5.5` で API エンドポイントとモデルを明示する必要があった

**結果:** `task1-summary.cloud.txt` に要約テキストのみを保存。

---

## タスク2

### ローカルモデル

**成否:** ✓ 成功（ステップ1、ステップ2 両方完了）

**試行コマンド一覧（時系列）:**
1. `cat sample.go | ./henji --json-schema review-schema.json -q -t "task2-local" "このGo言語のコードをレビューして..."` → エラー（Markdown フェンス）
2. プロンプト修正：末尾に "raw JSON only, no code fences." を追加
3. `cat sample.go | ./henji --json-schema review-schema.json -q -t "task2-local" "...raw JSON only, no code fences."` → 成功
4. `./henji -c task2-local -q "line 21 の SQL Injection の問題について..."` → 成功

**つまずいた点と解決方法:**
- ローカルモデル（gemma-4）が JSON を Markdown フェンスで包んでいた
- docs に記載されていた対策（プロンプトに "raw JSON only, no code fences" を追加）で解決

**結果:**
- `task2-findings.local.json`: 4件の指摘（SQL Injection高、Data race中、View tracking中、Error handling低）
- `task2-detail.local.txt`: line 21 の SQL Injection について、プリペアドステートメントの修正方法を詳述

---

### クラウドモデル

**成否:** ✓ 成功（ステップ1、ステップ2 両方完了）

**試行コマンド一覧（時系列）:**
1. `cat sample.go | ./henji -a openai -m gpt-5.5 --json-schema review-schema.json -q -t "task2-cloud" "..."` → エラー（スキーマ検証失敗）
   - エラーメッセージ：`additionalProperties` required to be false
2. review-schema.json を修正：`additionalProperties: false` を追加
3. `cat sample.go | ./henji -a openai -m gpt-5.5 --json-schema review-schema.json -q -t "task2-cloud" "..."` → 成功
4. JSON を整形して保存：`... | ./henji ... --output json ... | jq '.content[0].text | fromjson'`
5. `./henji -a openai -m gpt-5.5 -c task2-cloud -q "line 20 の SQL Injection について..."` → 成功

**つまずいた点と解決方法:**
- OpenAI API の構造化出力要件：`additionalProperties: false` が必須
- docs に「Google は additionalProperties を拒否、OpenAI では strict を使う」と記載
- スキーマの オブジェクト定義に `additionalProperties: false` を追加して解決

**結果:**
- `task2-findings.cloud.json`: 12件の指摘（セキュリティ、デザイン、リソース管理など包括的）
- `task2-detail.cloud.txt`: line 20 の SQL Injection について、3つのDB方言別コード例と5つの重要注意点を含めて詳述

---

## ローカル/クラウドの違いで気づいたこと

1. **モデルの指摘数と詳しさ**: クラウドモデル（gpt-5.5）がローカルモデル（gemma-4）よりはるかに多くの指摘（12件 vs 4件）を返し、より細かいセキュリティ・設計問題を検出した。

2. **JSON 出力の挙動**:
   - ローカルモデル：Markdown フェンスで JSON を包み、別途対策が必要
   - クラウドモデル：strict schema を使用し、フェンス処理不要

3. **スキーマ要件の差**:
   - ローカル（OpenAI互換）：`additionalProperties: false` が必須
   - 記載されているように Google との方言差がある可能性

4. **応答の正確性**: クラウドモデルは複数の脆弱性パターン（XSS、race conditions、リソース枯渇、HTTP タイムアウト、平文通信など）を同時に指摘できた。

---

## henji への改善提案

### `-h に足りなかった情報:**
- `-a <api>` と `-m <model>` の組み合わせが必須であること（複数API設定時）
- ローカル gateway の場合、デフォルトで動作すること
- `--json-schema` が API によってスキーマ要件が異なること

### docs に足りなかった情報・分かりにくかった箇所:**
- ローカルモデルの Markdown フェンス問題：現在は "may wrap JSON in Markdown fences" とありますが、より具体的な推奨対策を明記すると良い
- schema ファイル要件：OpenAI（strict: true）と その他の OpenAI互換エンドポイント（strict: false）の差をより明確に説明すると良い

### その他の改善提案:**
- `--list-models` の出力に `[local]` / `[cloud]` タグがあると、モデル選択がより直感的になる
- 会話の保存・続行フロー（`-t` / `-c`）の使用例をより充実させると良い
- 構造化出力の複合使用例（`--json-schema` + `--output json` + `jq`）をドキュメントに加えると良い
