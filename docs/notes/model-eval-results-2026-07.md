# モデル別 henji 利用能力テスト結果（2026-07-04）

計画: [[model-eval-plan]]（`docs/notes/model-eval-plan.md`）  
実施条件: --help 簡素化コミット `3eb221f` 後のビルド / 隔離ディレクトリ `/tmp/henji-eval/<session>/` /
情報源は `henji -h`・`henji docs`・実行結果のみ / ゲートウェイ2種（local=gemma-4-E2B、cloud=gpt-5.5）/
skip-permissions 無人実行・逐次

## 総合結果: 3ドライバーとも全タスク成功（12/12）

| | Opus 4.8 | Sonnet 5 | Haiku 4.5 |
|---|---|---|---|
| タスク1 local/cloud | ✅/✅ | ✅/✅ | ✅/✅ |
| タスク2 local/cloud | ✅/✅ | ✅/✅ | ✅/✅ |
| 所要時間(概算) | 約6分 | 実質数分※1 | 約5.5分 |
| 失敗コマンド数 | 1 | 1 | 3 |
| フラグの捏造 | なし | なし | なし |
| 会話継続の方式 | `-t` 命名 → `-c <title>` | `--output json` の conversation_id → `-c <id>` | `-t` 命名 → `-c <title>` |
| docs を読んだ契機 | タスク2の前に予習 | タスク2の詳細確認 | `-m openai` 失敗時 |

※1 Sonnet はユーザー設定の `Bash(rm*)` 確認ルール（bypassでも優先）で中間ファイル掃除時に
一時停止 → オーケストレーター(Fable)が承認のみ実施（henji のヒントは与えていない）。
待ち時間は所要時間から除外。

## 各ドライバーの特徴

- **Opus 4.8**: タスク2に入る前に docs を予習し、「小型モデルはフェンスで包む」という
  警告を読んで **事前に** プロンプトへ "raw JSON only" を仕込み、落とし穴を回避。
  `cat -n` で行番号を渡して `line` フィールドの精度を上げる工夫も。唯一の失敗は
  `-m 5.5` 単独指定（エラーメッセージから即回復）。
- **Sonnet 5**: フェンス問題に実際に遭遇（schema 3回リトライ失敗 → exit 1）し、docs の
  記載どおりの対策で解決。stdout/stderr を分離保存して出力の清浄性を検証するなど
  手順が最も慎重。継続は conversation_id 方式（docs の "A typical agent loop" どおり）。
  ※RESULT.md の自己申告は「Sonnet 4.6」だが `/status` で claude-sonnet-5 と確認済み
  （モデルの自己誤認。テスト妥当性に影響なし）。
- **Haiku 4.5**: 障害に最多の3回遭遇（`-m openai` 誤指定 / フェンス問題 /
  gpt-5.5 の strict 出力での `additionalProperties: false` 必須）が、**3回とも docs を
  参照して自力解決**。cloud で findings 12件と最多。レポートに軽微な混同あり
  （strict 要件を「ローカル側の要件」と誤記）だが成果物は全て有効。

## ゲートウェイ側（local vs cloud）の観測

- **フェンス問題は gemma E2B で再現性あり**: Sonnet・Haiku が遭遇、Opus は予習で回避。
  docs の警告と "raw JSON only" 対策はそのまま機能した。
- **レビュー網羅性**: 同じ `sample.go` に対し local は一貫して4件、cloud は 8〜12件。
  仕込んだ3点（SQLi・リソースリーク・無保護map書き込み）は local でも概ね検出。
- **gpt-5.5 の strict 構造化出力**は schema に `additionalProperties: false` を要求
  （Haiku のみ遭遇 — Opus/Sonnet の schema は最初から要件を満たしていた）。

## v2.0.0 基準への評価

基準: 「AI が自律実行時に henji を能動的に発見・選択・実行するに足る」

- **おおむね達成と判断できる材料が揃った**。最小構成の Haiku 4.5 でも、-h → docs の
  二段階だけで全タスクを完遂し、3種類の障害から docs を根拠に回復した。
- 全ドライバーが「-h で骨格 → docs で詳細」という想定どおりの導線を辿った。
  簡素化後の -h でも不足の訴えは「説明が短すぎる」ではなく「特定の挙動の一言が欲しい」
  レベルに収まっている。
- 残る弱点は下記バックログの (1)(2)(5)。特に (1) は3セッション全員が同じ場所で
  つまずいており、修正価値が最も高い。

## 改善バックログ（3セッションの提案を統合、検証済み）

1. **`-a`/`-m` ペア必須の導線強化**（3/3 が指摘・全員が実際に失敗）:
   `-m <model>` 単独でデフォルト外 API のモデルを指定した時のエラーに
   「try: -a <api> -m <model>」相当のヒントを足す、または -h の Examples に
   クラウド切り替え例を1つ追加。エラーメッセージ改善の方が効果的か。
2. **継続時のモデル復元挙動を docs に明記**: フラグ無し `-c` は会話保存時の
   API/モデルを DB から復元する（**Fable が実機検証済み**: goreview-cloud を
   フラグ無し継続 → model=gpt-5.5 で応答）。Opus はこれを知らず「毎回 -a/-m
   再指定が必要」と誤解したまま完走した。1行の明記で無駄な再指定と誤解を防げる。
3. **`--json-schema` 単独時の出力仕様を -h に一言**（Opus）: エンベロープ無しの
   検証済み生 JSON が stdout に出ること。
4. **schema リトライの動作説明**（Sonnet）: 失敗時にどんな訂正指示がモデルに
   送られるかを docs に1段落。「なぜ3回とも同じ失敗をしたのか」が見えなかった。
5. **`--list-models --output json` にローカル/クラウド判別材料**（3/3 が類似指摘）:
   テキスト出力の alias `[local]` は JSON にも `aliases` として入っているが意味論が
   無い。`base_url` または API 種別を JSON に足すとスクリプトから判別可能になる。
6. **OpenAI strict の `additionalProperties: false` 要件を docs に明記**（Haiku）。
7. **継続の2方式（`-t`/`-c` タイトル方式と conversation_id 方式）を docs で並記**
   （Opus・Haiku はタイトル方式、Sonnet は ID 方式に到達。どちらも有効だが
   相互参照が無い）。

## 生データ

- 各セッションの RESULT.md・成果物: `/tmp/henji-eval/{opus,sonnet,haiku}/`
  （/tmp のため再起動で消える。恒久保存が必要ならコピーすること）
- 検収: 全 task2-findings JSON は形式検証パス（local 4件×3、cloud 8/8/12件）
