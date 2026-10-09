# henji decision の設計・入出力契約

作成日: 2026-10-09

## 目的と対象

`henji decision` はOpenRouterとOpenAIの専用Decisions APIを呼ぶ独立したサブコマンドである。通常の生成コマンドの翻訳・要約、JSON Schema検証、トークン指定、会話保存の契約を維持する。

一回の起動で一つの対象を判断し、同じ対象への複数質問は一回のAPIリクエストにまとめる。先行する判断結果に依存する処理や複数対象の処理は、shellまたは呼び出し元が別の起動として組み立てる。

Responses API、通常のChat Completionsを使うTev1、会話履歴の継続、常駐・JSONL入力ループは今回の対象外である。

## CLIと入力

```sh
henji decision -a openrouter -m typesafe/jev-1.13 \
  --questions docs/examples/decision-openrouter-questions.json < input.txt

henji decision -a openai -m gpt-6-luna \
  --questions docs/examples/decision-openai-questions.json < input.txt
```

- `-m` / `--model` は必須。生成用の `default-model` や `HENJI_MODEL` は継承しない。
- `-a` / `--api` はAPI設定名。省略時は `default-api` / `HENJI_API` を使う。いずれもなければ指定を求めるエラーになる。
- テキスト用のモデルIDはYAMLへの事前登録を要求しない。選択したAPIのモデル設定にある別名は解決する。固定の対応モデル一覧は持たず、利用可否はAPIが判断する。
- `--questions FILE` は必須で、一回だけ指定できる。OpenRouterは名前付き質問オブジェクト、OpenAIは質問配列を使う。JSON構文・UTF-8・非空のトップ構造と各質問のobject形式を確認し、質問内容の意味的な検証はAPIに任せる。質問の未知フィールドも送信時に保持する。
- stdinはUTF-8の自然文としてそのまま渡す。インデント、生成用の役割・指示追加、`max-input-chars` による切り詰めを行わない。JSON文字列がstdinに来ても構造化 `state` として自動解釈しない。
- `--image FILE` はJPEG/PNG/WebPを1枚、元ファイル3MiB以内で受け付ける。形式は既存のmagic byte検査を使う。選択モデルに `vision: true` が必要である。
- テキストだけ、画像だけ、テキストと画像の併用を受け付ける。テキストも画像もなければエラーになる。端末から対話入力を待たない。
- 引数の位置に判断対象の文章は受け付けない。通常の生成プロンプトがdecisionで始まる場合は `henji "decision …"` と引用する。
- 通常の生成用フラグ（`--output`・`--json-schema`・`--continue` など）はdecisionのフラグではない。

## 設定と認証

API設定に `decision-protocol` と `decision-base-url` を追加する。

| API設定名 | 既定プロトコル | 既定ベースURL |
| --- | --- | --- |
| `openai` | `openai` | `https://api.openai.com/v1` |
| `openrouter` | `openrouter` | `https://openrouter.ai/api/alpha` |

標準のAPI名は設定エントリがない場合も、既定値とプロバイダ既定環境変数で利用できる。独自API名では設定エントリと `decision-protocol: openai` または `openrouter` が必要である。ベースURLを省略すると、指定プロトコルの標準URLを使う。

送信先はベースURLの末尾に `decisions` を付加する。末尾スラッシュを許容し、gatewayのパス接頭辞を保持する。設定には完全な `/decisions` エンドポイントではなくベースURLを記す。既存の生成用 `base-url` はdecisionの送信先に使わない。

```yaml
apis:
  gateway:
    decision-protocol: openrouter
    decision-base-url: http://localhost:8080/proxy/api/alpha
    api-key-env: GATEWAY_API_KEY
    models:
      image-decision-model:
        aliases: [image-decision]
        vision: true
```

キーの優先順位は `api-key-cmd` → `api-key-env` → `api-key` → プロトコルの既定環境変数とする。既定環境変数はOpenAIが `OPENAI_API_KEY`、OpenRouterが `OPENROUTER_API_KEY`。キー取得コマンドの失敗時は下位の取得元へフォールバックしない。コマンドはshellを介さず実行し、キャンセルに対応する。

`http-proxy` / `HENJI_HTTP_PROXY` を利用する。`max-retries` / `HENJI_MAX_RETRIES` は初回要求後の追加試行回数とし、0なら一回だけ送る。408・409・429・5xxと一時的な通信失敗を再試行する。待機は100msから最大1秒までの指数backoffで、待機中もキャンセルできる。SDK内の再試行は無効にし、共通の一つの再試行処理で回数を管理する。

モデルのフォールバック、JSON Schemaによる再生成、入力の修正・縮小は行わない。

## プロバイダごとの送信形式

| 項目 | OpenRouter | OpenAI |
| --- | --- | --- |
| テキスト | `state`の文字列 | `input`の文字列 |
| 画像併用 | `state`のトップ配列に自然文の文字列と `image_url` 要素 | user messageのcontentに `input_text` と `input_image` |
| 画像内容 | MIME付きbase64 data URL | MIME付きbase64 data URL |
| 質問 | 名前をキーにした `questions` オブジェクト | `questions` 配列 |
| 実装 | 専用HTTP処理 | OpenAI SDKのDecisions機能 |

生成用のsampling、role、`max_tokens`、`max_completion_tokens`、`response_format`、stream設定を送らない。

## stdout・stderr・終了コード

- APIの正常応答全体をJSONとしてstdoutに出し、末尾改行を付ける。Henjiの共通ラッパー・Markdown・ANSI装飾を付けない。
- 応答の全フィールド、未知フィールド、数値の表現を保持する。SDKがJSON値の外側の空白を取り除くことはある。
- JSONを完全に受信・確認するまでstdoutへ書かない。空・不正・途中で切れたJSON応答は失敗になる。
- 正常応答として返されたrefusalもそのまま出力し、exit 0とする。回答の意味やrefusalの判断は呼び出し元が行う。
- 設定・引数・入力・認証・通信・API失敗はstdoutを空にして、stderrに人間向けの詳細を出し、exit 1とする。
- 設定の `output: json` / `HENJI_OUTPUT` はこの契約を変更しない。進捗や保存通知を出さない。
- `Ctrl-C` はstdin待機、キー取得コマンド、HTTP要求、再試行待機をキャンセルする。

stdout自身への書込み失敗や呼び出し元による強制終了の場合は、完成JSONの出力を保証できない。

## 履歴と初期化

設定ファイル・環境変数の読込みを、会話保存先の解決・ディレクトリ作成・DB準備から分離する。decisionは会話DB・履歴・キャッシュを開いたり作ったりせず、会話保存先が作成不能でも実行できる。設定ファイル自体の準備は通常どおり行う。

通常の生成コマンドは従来どおり会話保存先を初期化する。

## モデル固有の制約

3MiBはHenjiが読み込める画像の上限であり、全モデルで送信が成功する保証値ではない。画像の縮小・再圧縮や文章分割は呼び出し元で行う。

2026-10-09の[OpenRouter画像判断ガイド](https://openrouter.ai/docs/guides/community/multimodal-decisions)では、Clef/Clef Flashは約300KB未満の画像を推奨し、`state`の文章は先頭約2,000トークン以降をエラーなしで切り捨てるとされている。Henjiにモデル固有の上限を固定実装せず、利用するモデルの現行仕様を参照する。

## 参照

- [OpenAI Decisions](https://developers.openai.com/api/docs/guides/decisions)
- [OpenRouter Decisions API](https://openrouter.ai/docs/api/api-reference/alphadecisions/submit-a-decisions-request)
- [OpenRouter画像判断](https://openrouter.ai/docs/guides/community/multimodal-decisions)
- [実装計画](../notes/decision-implementation-plan-2026-10-09.md)
