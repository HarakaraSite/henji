# henji の機能

## 基本的な使い方

デフォルトでは：

- すべてのメッセージは`STDERR`に出力される
- すべてのプロンプトは、プロンプトの1行目をタイトルとして保存される
- `STDOUT`がTTYの場合、デフォルトでglamourによる整形が使われる

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

この場合、「Generating」アニメーションは`STDERR`に出力されますが、応答自体は`STDOUT`にストリーミングされます。

### タイトルを指定する

カスタムタイトルを設定できます：

```bash
henji --title='タイトル' '最初の2つの素数'
```

### 最新の会話を継続する

`--continue=title`を使って最新の会話を継続し、新しいタイトルで保存できます：

```bash
henji '最初の2つの素数'
henji --continue='primes as json' 'JSON形式にして'
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
