# `--text` / `--image` 実機確認手順

作成日: 2026-07-16
対象: `b27cf5f` 以降

## 目的

`--text`、`--image`、両方とstdinの併用、および会話継続時の添付再指定を、実際のvision対応モデルまたはgatewayで確認する。

unit testは別途通過済みである。この手順では実際の設定・認証・画像payloadを通す。

## 事前条件

- 実際にvision入力を受け付けるモデル／gatewayが設定済みであること。
- テスト画像は3 MiB以下のJPEG、PNG、またはWebPであること。
- 設定ファイルにAPIキーが含まれていても、この手順の一時コピーをリポジトリへ置かないこと。

`vision: true` はモデルがvision対応であることをhenjiへ明示する設定であり、非対応モデルを対応化するものではない。

## 一時環境の準備

```sh
go build -o /tmp/henji .

TEST_CONFIG=$(mktemp -d)
TEST_DATA=$(mktemp -d)
mkdir -p "$TEST_CONFIG/henji"
cp ~/.config/henji/henji.yml "$TEST_CONFIG/henji/henji.yml"
chmod 600 "$TEST_CONFIG/henji/henji.yml"
$EDITOR "$TEST_CONFIG/henji/henji.yml"
```

編集した一時設定で、今回使うモデルへ `vision: true` を加える。

```yaml
apis:
  local:
    models:
      your-vision-model:
        vision: true
```

`local` と `your-vision-model` は実際のAPI名・モデルIDへ置き換える。APIキーや一時設定の中身をシェル履歴、ログ、リポジトリへ出さない。

## 確認1: テキスト添付

```sh
XDG_CONFIG_HOME="$TEST_CONFIG" XDG_DATA_HOME="$TEST_DATA" \
  /tmp/henji --text README.md "このファイルの目的を一文で説明して"
```

期待結果:

- 正しい要約が返る。
- 3 MiB超のテキストは明示的なエラーになる。

## 確認2: 画像添付

```sh
XDG_CONFIG_HOME="$TEST_CONFIG" XDG_DATA_HOME="$TEST_DATA" \
  /tmp/henji --image screenshot.png "この画像に何が写っている？"
```

期待結果:

- 画像の内容を反映した応答が返る。
- `vision: true` を外すと、HTTP送信前に明示的なエラーになる。

## 確認3: `--text`、`--image`、stdinの併用

```sh
git diff | XDG_CONFIG_HOME="$TEST_CONFIG" XDG_DATA_HOME="$TEST_DATA" \
  /tmp/henji --text requirements.txt --image screenshot.png \
  "要件と画面を踏まえて、この差分をレビューして"
```

期待結果:

- テキスト添付、画像、stdin由来の差分をすべて踏まえた応答が返る。

## 確認4: 会話継続と添付非保存

```sh
XDG_CONFIG_HOME="$TEST_CONFIG" XDG_DATA_HOME="$TEST_DATA" \
  /tmp/henji --title image-check --text README.md --image screenshot.png "画面を要約して"

XDG_CONFIG_HOME="$TEST_CONFIG" XDG_DATA_HOME="$TEST_DATA" \
  /tmp/henji --continue image-check --text README.md --image screenshot.png "右上だけ詳しく説明して"

XDG_CONFIG_HOME="$TEST_CONFIG" XDG_DATA_HOME="$TEST_DATA" \
  /tmp/henji --show image-check
```

期待結果:

- 継続時、`--text` と `--image` を再指定した場合にだけ添付内容を再び参照できる。
- `--show` には `[text attachment omitted from saved conversation]` と
  `[image omitted from saved conversation]` が表示される。
- 一時キャッシュは `TEST_DATA` 配下だけに作られ、通常の会話履歴には影響しない。

## 終了後

不要になった一時設定・キャッシュは削除する。設定ファイルにAPIキーがある可能性があるため、内容を確認せず共有・コミットしない。
