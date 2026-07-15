# 画像入力・テキスト添付の実装計画

作成日: 2026-07-15  
状態: 実装前・要件合意済み

## 目的

henji に、テキスト添付と画像入力を明示的に分けた Unix フィルター向けの入力機構を追加する。

```sh
git diff | henji \
  --text requirements.txt \
  --image screen.png \
  "要件と画面を踏まえ、この変更をレビューして"
```

画像入力はプロバイダーへ送るだけであり、henji 自身は外部ツールを実行しない。

## 確定したCLI仕様

### `--text FILE`

- 現在の `-f` / `--file` を廃止し、`--text` に置き換える。
- UTF-8 テキストを1ファイルだけ添付できる。短縮形は付けない。元ファイルは **3 MiB以下**。
- NUL byte を含む、または不正な UTF-8 の入力は明示的にエラーにする。
- 同一コマンドで複数回指定した場合はエラーにする。

### `--image FILE`

- 画像を1ファイルだけ添付できる。短縮形は付けない。
- 対応形式は JPEG、PNG、WebP のみ。
- 判定には拡張子ではなく magic byte を使う。
- 元ファイルの上限は **3 MiB**。性能測定の結果により、将来より小さくする可能性がある。
- 非対応形式、空または短い入力、サイズ超過、読込失敗、複数指定は明示的にエラーにする。

### 組み合わせ

- `--text` と `--image` は同時に指定できる。
- stdin も併用できる。
- 送信するユーザー入力の論理順序は、**指示 → text → image → stdin** とする。フラグの並び順には依存させない。

## 非対象（初版）

- stdin からの画像自動判別
- 自動画像変換
- GIF、HEIC / HEIF、SVG、BMP、TIFF
- 複数画像、画像URL
- PDF、音声、動画

## データモデルとキャッシュ

`internal/proto.Message` は現在 `Content string` のみを持ち、会話キャッシュはこれを gob で保存している。

- 既存の `Content string` は残し、text / image を順序付きで表す `Parts` を追加する。
- 画像付きの user message では、`Content` に画像を除くテキスト結合値を残す。既存のタイトル生成、表示、テキスト専用経路との互換性を保つ。
- provider formatter は `Parts` がある場合にそれを使い、ない場合は従来の `Content` を使う。
- 画像バイト列・ローカルパスは会話キャッシュに保存しない。
- 保存前に画像partを `[image omitted from saved conversation]` 相当の表示マーカーへ置換する。`--show` はそのマーカーを表示する。
- `--continue` / `-C` は過去画像をAPIへ再送しない。画像を再び参照させる利用者は、その実行で `--image` を再指定する。
- 既存 gob を読めることを回帰テストで固定する。DB migration やキャッシュ全消去は行わない。

## プロバイダー変換

- OpenAI互換: `image_url` に MIME type 付きの base64 data URI を入れる。
- Anthropic: base64 image block を入れる。
- Gemini: `inline_data` に MIME type と base64 data を入れる。
- cached image marker は、いかなる provider request にも入れない。

## Vision対応の明示設定

API名やモデル名から画像対応を推測しない。特に OpenAI互換のローカルgatewayでは、実際の対応状況を名前だけで判断できないためである。

- `Model` に `vision: true` を追加する。
- `vision` の未指定は `false` とする。
- 画像付きリクエストはHTTP送信前に検査し、`vision: true` でないモデルなら明示的に失敗する。
- 画像を落としてテキストだけで実行するフォールバックは作らない。
- `vision: true` でもgatewayが実際には画像を拒否した場合は、そのAPIエラーを返す。
- `config_template.yml` の既定モデルには `vision: true` を追加しない。確認済みのモデルだけを利用者が有効化する。

例:

```yaml
models:
  my-vision-model:
    vision: true
```

## 実装手順

1. `Config` の添付パスを `textPath` と `imagePath` に分離し、`--text` / `--image` を登録する。`-f` / `--file` は登録しない。
2. UTF-8テキスト読込を `readTextInput` として整理し、画像読込・3 MiB制限・magic byte検査を行う `readImageInput` を追加する。
3. `proto.Message` に型付き content parts を追加し、入力組み立てを prompt / text / image / stdin の順へ変更する。画像だけのリクエストも有効にする。
4. OpenAI互換、Anthropic、Geminiの各formatterをcontent parts対応にする。
5. 最大入力文字数・schema retry・model fallback でも画像partを黙って落とさず、同じ入力を再送できるようにする。
6. キャッシュ書込の直前に画像データを除去してマーカーへ置換し、表示ではマーカーを安全に出す。
7. HTTP送信前の `vision: true` 判定を追加する。
8. unit test、provider mock test、E2E、README、cookbook、内蔵manualを更新する。

## 検証計画

- flag: `--text` / `--image` の重複拒否、廃止済み `-f` / `--file` の拒否。
- input: 有効UTF-8、NUL、不正UTF-8、JPEG/PNG/WebP、拡張子偽装、GIF/HEIC風入力、3 MiB超過、読込失敗。
- ordering: prompt / text / image / stdin の順、画像だけの入力、既存テキスト専用入力。
- cache: gobに画像bytesもパスも残らないこと、画像マーカーの表示、旧キャッシュの読込。
- providers: 各mock requestで正しいMIME type、base64、content順を検証し、マーカーが送信されないことを確認する。
- capability: `vision: false` または未指定ではHTTP送信前に失敗すること。
- E2E: 通常のgateway検証では `vision: false` のfail-closedを確認する。実画像検証は任意の `VISION_MODEL` / gateway設定を使う別ケースにする。

## 実画像E2Eの設定扱い

実画像E2Eでは、現在の `henji.yml` をリポジトリ外の一時ディレクトリへ `0600` でコピーする。テスト対象モデルだけに `vision: true` を加え、その一時設定を `XDG_CONFIG_HOME` 経由で使う。

- APIキーを含む可能性があるため、コピーはリポジトリへ置かない。
- 設定内容やキーをログへ出さない。
- unit testは秘密情報を含まないfixture設定だけを使う。

## 保留事項

- 3 MiBの共通添付上限は named constant へ閉じ込める。`--no-limit` では解除しない。
- 実測でレイテンシやgateway制限が問題になれば、上限を下げる。
- PDFなど新しいメディアは、今回追加するcontent partsを拡張して別途設計する。
