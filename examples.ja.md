
# henji 用例集

### コードを改善する

ソースコードをhenjiにパイプで渡し、何をしてほしいか指示するだけで、リファクタリング・機能追加・デバッグの選択肢が大きく広がります。

`henji -f "このコードを改善するとしたらどう思う？" < main.go | glow`

### 新機能のアイデアを出す

henjiはソースコード（やREADMEファイル）を元に、全く新しい機能案を考えることもできます。

`henji -f "このツールに10個の新機能を提案して" < main.go | glow`

### ドキュメント作成を手伝ってもらう

henjiは新しいドキュメントの下書きをすぐに作ってくれます。

`henji "rキーを押すと無料でウサギが送られてくる機能について、このreadmeに新しいセクションを書いて" < README.md | glow`

### 動画ファイルを整理する

ファイルシステムはhenjiへの入力として非常に優秀な情報源になります。音楽や動画ファイルがあれば、henjiは`ls`の出力を解析して内容をうまく整理・要約してくれます。

`ls ~/vids | henji -f "これらを年代別に整理してそれぞれ要約して" | glow`

### おすすめを生成してもらう

henjiは手元にあるものを元に、似たジャンルだけでなく、全く別のメディア（例えば持っている映画から音楽のおすすめを出す、など）でもおすすめを生成するのが得意です。

`ls ~/vids | henji -f "これらを元に10個の番組をおすすめして、あまり有名でないものにして" | glow`

`ls ~/vids | henji -f "これらの番組を元に10枚のアルバムをおすすめして、サントラや番組内で使われた曲は除外して" | glow`

### 運勢を占ってもらう

ダウンロードフォルダはいつの間にか収拾のつかないファイルの山になりがちですが、henjiがあればそれを逆手に取れます！

`ls ~/Downloads | henji -f "このファイル群を元に私の運勢を占って" | glow`

### APIを理解する

henjiは`curl`によるAPI呼び出しの出力を解析し、人間が読みやすい形に変換できます。

`curl "https://api.open-meteo.com/v1/forecast?latitude=29.00&longitude=-90.00&current_weather=true&hourly=temperature_2m,relativehumidity_2m,windspeed_10m" 2>/dev/null | henji -f "この気象データを人向けに要約して" | glow`

### コメントを読む（自分で読まなくて済むように）

APIと同じように、henjiは生のHTMLを読んで内容を要約できます。

`curl "https://news.ycombinator.com/item?id=30048332" 2>/dev/null | henji -f "コメントの投稿者たちは何を言っている？" | glow`
