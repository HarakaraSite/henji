# Decisions API対応の実装計画

作成日: 2026-10-09

状態: 実装・モック検証・コードレビュー完了。OpenRouter・OpenAIの最小実API確認は成功。

調査時点のコード: `16380ec`

## 目的

独立した `henji decision` サブコマンドを追加し、OpenRouterとOpenAIのDecisions APIを使ってテキスト・画像を判断できるようにする。

既存の翻訳・要約などの生成処理との互換性を維持する。特に、Hayari・Shirushiが依存するstdout、stderr、終了コード、JSON Schema検証、トークン指定の契約を維持する。

この文書は実装計画の記録であり、未確定の設計案を製品仕様として扱わない。実装に伴って確定した設計は `docs/design/`、利用方法はREADME・cookbook・内蔵マニュアルへ反映する。

## 合意済みの要件

- コマンド名は `henji decision` とする。
- OpenRouter Decisions APIとOpenAI Decisions APIに対応する。
- 判断対象のテキストはstdinから読み込む。質問は `--questions questions.json`、画像は `--image image.png` で指定する。
- 質問ファイルは各APIのネイティブ形式を使う。共通の質問形式への正規化は行わない。
- `-a` と `-m` でプロバイダ・モデルを指定する。
- 認証にはAPIキーを使い、既存の `api-key-cmd`・`api-key-env`・`api-key` の仕組みを利用する。
- 一回の起動で一つの対象を判断する。同じ対象への複数質問は、一回のAPIリクエストで送る。
- 成功時はAPIの応答全体をJSONとしてstdoutに出す。Henjiの共通ラッパーを付けず、API固有のフィールドも保持する。
- APIが正常応答として返したrefusalも、そのままstdoutに出してexit 0とする。意味の解釈は呼び出し元が行う。
- 設定・認証・通信・APIエラーでは、stdoutを空にし、stderrへ人間向けの詳細を出してexit 1とする。
- 会話履歴の保存・継続は行わない。連続処理や判断結果を次の入力に使う処理は、shellまたは呼び出し元で行う。
- 既存の生成コマンドの動作・設定・出力契約を維持する。

呼び出しの形:

```sh
henji decision -a openrouter -m typesafe/jev-1.13 \
  --questions questions.json < input.txt
```

画像を使う場合は、対応モデルを選んで `--image image.png` を追加する。確定した設定・登録方法は [decision CLI設計](../design/decision-cli.md) を参照する。

## APIの違いと実装方針

| 項目 | OpenRouter | OpenAI |
| --- | --- | --- |
| 標準の接続先 | `https://openrouter.ai/api/alpha/decisions` | `https://api.openai.com/v1/decisions` |
| 判断対象 | `state` | `input` |
| 質問 | 名前をキーにしたオブジェクト | 質問の配列 |
| 画像 | `state`内の画像要素 | `input`内の画像要素 |
| 応答の扱い | 応答JSON全体を保持 | 応答JSON全体を保持 |

送信処理はプロバイダごとに分ける。既存のChat Completionsリクエスト組立ては使わず、Decisions APIに必要なフィールドを送る。

OpenAI側は現在のOpenAI SDKのDecisions機能を使い、HTTP本文全体を検証した上でSDKの生JSONから応答を取得する。OpenRouter側は専用のHTTP処理でネイティブ形式を送受信する。

参照仕様（2026-10-09の調査時点）:

- [OpenAI Decisionsガイド](https://developers.openai.com/api/docs/guides/decisions)
- [OpenAI Decisions APIリファレンス](https://developers.openai.com/api/reference/resources/decisions/methods/create)
- [OpenRouter Decisionsガイド](https://openrouter.ai/docs/guides/community/jev)
- [OpenRouter画像判断ガイド](https://openrouter.ai/docs/guides/community/multimodal-decisions)

## レビュー後に合意した設計方針

### 設定読込みと会話保存先の初期化を分離する

設定ファイル・環境変数の読込みと、会話保存先の解決・ディレクトリ作成・DB準備を分ける。`decision`は設定読込みだけを実行し、会話保存先への書込み可否に依存させない。通常の生成コマンドは従来どおり両方を実行し、既存の動作・エラー契約を維持する。

現在の `ensureConfig()` は会話キャッシュディレクトリを無条件に作るため、DB初期化の分岐だけでは不十分である。検証では、設定ファイルを正常に読み込める一方で会話保存先が作成不能な環境でも、decisionがモックAPIへ送信して正常終了できることを確認する。

設定ファイル自体の作成・読込みと、会話キャッシュの作成は区別する。

### 接続先は `decision-base-url` で設定する

既存の生成用 `base-url` と分け、Decisions用には `decision-base-url` を使う。設定値はベースURLとし、送信処理がその末尾に `decisions` を付加する。OpenAI側はSDKのパス付加を使い、OpenRouter側も同じ設定の意味でURLを構築する。

| プロバイダ | 既定の `decision-base-url` |
| --- | --- |
| OpenAI | `https://api.openai.com/v1` |
| OpenRouter | `https://openrouter.ai/api/alpha` |

標準の2社は設定を省略可能にする。独自gatewayではベースURLと `decision-protocol: openai` または `openrouter` を明示できる構成とした。

末尾のスラッシュの有無に対応し、`decisions`が重複したり、gatewayのパス接頭辞が失われたりしないことをモックAPIで確認する。

### Henjiの読込み上限とモデル固有の制約を区別する

画像は既存の1枚・JPEG/PNG/WebP・3MiB以内という読込み上限を維持する。これはHenjiの読込み上限であり、API・モデルが受理できるサイズの保証値ではない。

モデル固有の画像サイズ・文章長・切り詰めの挙動は利用文書で説明する。2026-10-09に確認した[OpenRouter画像判断ガイド](https://openrouter.ai/docs/guides/community/multimodal-decisions)では、Clef/Clef Flashは約300KB未満の画像を推奨し、`state`の文章は先頭約2,000トークン以降をエラーなしで切り捨てるとされている。

Henjiにモデル固有の上限を固定実装せず、自動での画像縮小・再圧縮・文章切り詰めも行わない。必要な画像加工や文章分割は呼び出し元で行う。APIが入力を拒否した場合は、そのAPIエラーを通常の失敗契約で返す。

## 実装で確定した設計項目

未確定だった項目は、実装時に次のように具体化した。詳細な現行仕様は [decision CLI設計](../design/decision-cli.md) を参照する。

- プロバイダ別名: `decision-protocol` でOpenAI形式・OpenRouter形式を指定する。
- モデル選択: `-m` は必須で、生成用の既定モデルを継承しない。テキスト用モデルIDは事前登録不要で、選択APIの設定済み別名は解決する。`-a` の省略時は設定のAPIを使う。
- 入力: 初回はstdinを自然文として扱う。OpenRouterが受け付ける構造化された `state` の入力モードは別途検討する。
- 再試行: `max-retries` は初回後の追加試行回数（0なら一回）とする。SDK内の再試行を無効にし、共通処理で一時的なHTTP・通信失敗を再試行する。
- 画像対応の指定: 選択APIのモデル設定で既存の `vision: true` を共用する。

## 実装順序

### 1. CLI・設定の契約を確定する

- [x] 上記の設定項目とモデル選択方法を決める。
- [x] `decision-base-url` の既定値・gateway用のプロトコル種別設定を追加し、既存の生成用 `base-url` と分ける。
- [x] `decision`専用のフラグ、質問ファイル、入力、成功・失敗の契約を設計文書にまとめる。
- [x] OpenRouterとOpenAIそれぞれの質問ファイルの例を、公式仕様と照合する。

### 2. サブコマンドと入力処理を作る

- [x] `decision`専用の実行処理を追加する。
- [x] 設定読込みと会話保存先の解決・ディレクトリ作成・DB準備を分離する。通常の生成コマンドは従来どおり初期化し、decisionでは会話保存先の初期化を省く。
- [x] stdinを既存の生成用整形処理へ渡さず読み込む。既存のキャンセル対応読込みを共用する。
- [x] 質問ファイルの読込み、JSON構文と各APIに必要な形式の確認を実装する。
- [x] APIキー解決・モデル選択などの共用範囲を限定し、通常の生成経路の変更を小さくする。

### 3. OpenRouterのテキスト判断を実装する

- [x] `model`・`state`・`questions` を専用のDecisions APIへ送る。
- [x] 同じ対象への複数質問を一回のリクエストで送る。
- [x] 応答全体を受信してJSONを確認した後、未知のフィールドも保持してstdoutに出す。
- [x] 認証・プロキシ・キャンセル・API再試行に対応する。

### 4. OpenAIのテキスト判断を実装する

- [x] OpenAI SDKのDecisions機能を使い、`input`と質問配列を送る。
- [x] SDKの生JSONを使い、API固有のフィールドを保持する。
- [x] 両プロバイダで成功・refusal・エラーの終了コードと出力先を揃える。
- [x] 設定の `output: json` や `HENJI_OUTPUT` に左右されず、decision専用の出力契約を適用する。

現在の共通エラー処理は `output: json` のときstdoutにエラーJSONを出すため、decisionの失敗時にはこの処理を使わないよう分岐させる。引数・設定読込み段階の失敗も確認する。

### 5. 画像判断を追加する

- [x] 既存の画像読込み・形式判定・サイズ確認を共用する。
- [x] OpenRouterとOpenAIそれぞれの画像入力形式へ変換する。
- [x] テキストと画像の併用を、一つの判断対象として送る。
- [x] モデルの画像対応設定と、APIが画像を拒否したときのエラーを確認する。
- [x] 読み込んだ画像・文章を自動で縮小・再圧縮・切り詰めずに送る。

### 6. テスト・利用文書を整える

- [x] モックAPIで各プロバイダのURL、認証、送信形式、複数質問、画像を確認する。
- [x] `decision-base-url` の末尾スラッシュとgatewayのパス接頭辞を確認し、送信先の `decisions` が重複しないことを確認する。
- [x] ネイティブJSON、未知の応答フィールド、refusalのexit 0を確認する。
- [x] 設定・認証・通信・API失敗時のstdoutが空で、stderrに詳細が出ることを確認する。
- [x] 既存のJSON出力設定がdecisionの出力に影響しないことを確認する。
- [x] キャンセルと再試行回数を確認する。
- [x] 会話DB・履歴・キャッシュを開いたり作成したりしないことを確認する。
- [x] 設定は読込み可能・会話保存先は作成不能な環境でも、decisionがモックAPIで正常終了できることを確認する。
- [x] コマンドの振分けと、通常の生成に渡す引用済みの `"decision …"` というプロンプトの扱いを確認する。
- [x] 既存のHayari・Shirushi向けCLIテストを実行し、JSON出力・エラー・トークン指定の互換性を確認する。
- [x] README、help、cookbook、`henji docs`、質問ファイルの例を更新する。
- [x] Henjiの画像読込み上限とAPI・モデル固有の制約を区別して説明し、必要な画像加工・文章分割は呼び出し元で行うことを記載する。
- [x] 内蔵マニュアルのフラグ検証をサブコマンドに対応させ、文量の制約も確認する。
- [x] `go test ./...` と `go vet ./...` を実行し、コード・テストをレビューする。

## 初回の範囲と後続候補

初回の実装対象は、上記二つのDecisions APIとテキスト・画像判断とする。

- Responses API対応は別段階で進める。
- 通常のChat Completionsを使うTev1への対応は後続候補とする。
- 常駐処理、JSONL入力ループ、会話履歴を使う連続判断は初回に含めず、shell・呼び出し元で組み立てる。
- stdinから構造化された `state` を送るモードは、必要な入力契約を整理して別途検討する。

## 検証結果

- 通常・批判的な計画レビュー後の3点を実装に反映した。
- コードレビューで、OpenAI SDKが最初のJSON値だけを読むことによる末尾データの見落としを検出し、HTTP本文全体のUTF-8・JSON検証で修正した。余分なJSON・末尾の不正データを含む応答の失敗を確認した。
- レビュアによる解消確認後、必須指摘は残っていない。
- `go test ./...`、`go vet ./...`、`go build` が成功した。両APIのモック送受信、画像、proxy、キー取得コマンド、HTTPキャンセル、および既存Hayari・Shirushi向けCLI契約を確認した。
- `go test -race ./...` は環境のCGO無効・Cコンパイラ不足により実行できなかった。`CGO_ENABLED=1` で試しても `gcc` が見つからずビルドできない。race検査は未確認として残す。
- 2026-10-09、利用者の許可を受け、OpenRouterに架空の短文・2問をまとめた1リクエストを送信した。`typesafe/jev-1.13`（応答モデル `typesafe/jev-1.13-20260917`）で成功し、`urgent` は `noul: 0.92`、`route` は `choice: "review"` を返した。
- OpenRouterの実API確認で、exit 0、ネイティブJSONと末尾改行、stderrなし、会話保存先が使えない状態での成功を確認した。設定を `output: json` にしても共通ラッパーは付かなかった。`max-retries: 0` で自動再試行は行っていない。応答の `usage.cost` は `0.0000147`。
- 同日、キー登録後にOpenAI `gpt-6-luna` へ同じ短文・2問をまとめた1リクエストを送信し、成功した。`urgent` は `predicate` の `probability: 1.0`、`route` は `choice: "review"` を返した。応答の `usage` は入力290トークン・出力0トークン・合計290トークンだった。
- OpenAIでもexit 0、ネイティブJSONと末尾改行、stderrなし、会話保存先が使えない状態での成功、`output: json` でも共通ラッパーが付かないことを確認した。`max-retries: 0` で自動再試行は行っていない。これで許可された各プロバイダ1回の実行は完了した。
- 画像・refusal・エラー経路はモックで確認済みだが、今回の最小実API確認では対象にしていない。

## 実プロバイダ確認と完了条件

実装・モックテスト・レビュー後、利用者は両プロバイダの最小実APIテストを許可した。今回の確認は、架空の短文に2問をまとめて各プロバイダ1回、OpenRouter `typesafe/jev-1.13` とOpenAI `gpt-6-luna` に送信する範囲とする。画像などの追加実API確認は、別途必要になったときに実行範囲を決める。以前の生成API確認への許可は流用していない。

完了条件は、両APIへの送信、ネイティブJSON出力、失敗時の出力契約、画像判断、履歴を使わない動作、および既存の翻訳・要約の互換性が確認され、利用方法が文書化されていることとする。

コミット・プッシュ・リリースは、この計画の保存や実装とは別の依頼に従う。
