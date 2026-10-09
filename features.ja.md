# henji の機能

## 意思決定モデル

`henji decision` はOpenRouter・OpenAIの専用Decisions APIに対応します。一回の起動で
一つの対象を判断し、API応答全体のJSONを返す一問一答です。同じ対象への複数質問は一緒に
送れます。履歴・キャッシュ・会話継続は使いません。連続実行はシェルループを想定し、前の
結果が必要な場合は呼び出し元が次の入力を作ります。意思決定の出力はGlowを使いません。
[README](README.ja.md#decisions-apiによるテキスト画像判断)と
[cookbookの例](docs/cookbook.ja.md#decisions-apiによる判断)を参照してください。

## 基本的な使い方

デフォルトでは：

- モデルの応答は`STDOUT`、進捗・状態メッセージは`STDERR`に出力される
- 成功したモデル会話は、`--no-cache`を指定しない限り最初のプロンプトの1行目をタイトルとして保存される
- 通常の端末表示では、全文生成後に`PATH`上の外部`glow`でMarkdownを整形する。
  Glowがない場合や失敗時は元のMarkdownを表示する
- `--raw`やstdoutのパイプ・リダイレクトはGlowを使わず逐次出力する。
  JSON・JSON Schema出力もGlowを使わない
- TTYでのリクエスト待機中は、`STDERR`に小さな`Generating`スピナーが表示される

### 基本形

最も基本的な使い方はこれです：

```bash
henji '最初の2つの素数'
```

### パイプで渡す

パイプで渡すこともでき、その場合`STDIN`はTTYではなくなります：

```bash
echo 'JSON形式で' | henji '最初の2つの素数'
```

この場合、`henji`は`STDIN`を読み込んでプロンプトに追加します。

### パイプで渡す（出力先）

出力を別のプログラムにパイプで渡すこともでき、その場合`STDOUT`はTTYではなくなります：

```bash
echo 'JSON形式で' | henji '最初の2つの素数' | jq .
```

この場合、応答自体は`STDOUT`にストリーミングされます。`STDOUT`がTTYではないため、スピナーは表示されません。

### タイトルを指定する

カスタムタイトルを設定できます：

```bash
henji --title='タイトル' '最初の2つの素数'
```

### 特定の会話を継続する

名前を付けた会話を継続し、新しいターンを別タイトルで保存できます：

```bash
henji --title='primes' '最初の2つの素数'
henji --continue='primes' --title='primes as json' 'JSON形式にして'
```

### タイトルなしで最新の会話を継続する

```bash
henji '最初の2つの素数'
henji --continue-last 'JSON形式にして'
```

### 特定の会話から継続し、新しいタイトルで保存する

```bash
henji --title='naturals' '最初の5つの自然数'
henji --continue='naturals' --title='naturals.json' 'JSON形式にして'
```

### 会話を分岐させる

`--continue`と`--title`を使うと、会話を分岐させることができます。例えば：

```bash
henji --title='naturals' '最初の5つの自然数'
henji --continue='naturals' --title='naturals.json' 'JSON形式にして'
henji --continue='naturals' --title='naturals.yaml' 'YAML形式にして'
```

これで`naturals`・`naturals.json`・`naturals.yaml`という3つの会話ができます。

## 会話一覧を表示する

過去の会話は以下で一覧表示できます：

```bash
henji --list
# または
henji -l
```

このコマンドは常にタブ区切りの一覧を出力します。表示されたIDまたはタイトルを
`--show`、`--continue`、`--delete`に渡してください。対話的な選択画面はありません。

## 過去の会話を表示する

IDまたはタイトルを指定して過去の会話を表示することもできます。例えば：

```bash
henji --show='naturals'
henji -s='a2e2'
```

タイトルの場合は完全一致が必要です。
IDの場合は先頭4文字だけで構いません。複数の会話にマッチする場合は、1件に絞り込めるまで文字数を増やしてください。

## 会話を削除する

`--show`と同様に、タイトルまたはIDを指定して会話を削除することもできます（フラグは異なります）：

```bash
henji --delete='naturals' --delete='a2e2'
```

これらの操作は取り消せない点に注意してください。
`--delete`フラグを繰り返し指定することで、複数の会話を一度に削除できます。
