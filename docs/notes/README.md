# 開発ノート（作業メモ置き場）

このディレクトリは、henji 開発中に残した**日付付きの作業メモ・計画・調査記録**を置く場所です。
時点の観測であり、内容が古くなっていることがあります。

- **現行の設計判断・指針の正**は `docs/design/`（MCP削除の経緯、AIファーストCLI設計指針、セキュリティレビュー等）を参照してください。
- **ユーザー向けドキュメント**は `docs/cookbook.md` および `henji docs`（内蔵マニュアル）が正です。
- ここのメモは「なぜそうしたか」を辿るための開発記録であって、権威ある仕様ではありません。

## 現在のメモ

- `overview.md` — プロジェクト全体像
- `fix-roadmap.md` — 修正ロードマップ（一次情報。PR番号・未着手項目はここ）
- [fix-targets-2026-10-08.md](fix-targets-2026-10-08.md) — 2026-10-08 に再現確認した修正対象5件（保存、エラー処理、JSON 出力、UTF-8、API キー取得）
- [decision-implementation-plan-2026-10-09.md](decision-implementation-plan-2026-10-09.md) — `henji decision` の合意要件、OpenRouter・OpenAI Decisions API対応の実装計画
- [decision-performance-2026-10-09.md](decision-performance-2026-10-09.md) — Glow表示への移行、質問検証・初期化の整理、起動時間の比較計測
- `feature-requirements.md` — 機能要件メモ
- `potential-bugs.md` — 洗い出したバグ候補
- `json-output-plan.md` — `--output json` 設計メモ
- `multimodal-and-file-input-plan.md` — `-f/--file`・`--image` 設計メモ
- `pipeline-executor-ecosystem-idea.md` — 計画→実行→分析パイプラインの周辺エコシステム構想（別リポ前提・アイデア段階）
- `henji-mini.md` — 会話・設定・組み込み描画を持たない小型 CLI の検討メモ（未決定）
- `ai-docs-plan.md` / `henji-docs-draft.md` — 内蔵ドキュメント関連メモ
- `dependency-compatibility.md` — 依存の互換性メモ
- `model-eval-plan.md` / `model-eval-results-2026-07.md` / `model-eval-raw-2026-07/` — モデル別利用能力テストの計画・結果・生データ
