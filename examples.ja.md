# henji 用例集

### コードを改善する

ソースコードをhenjiにパイプで渡し、何をしてほしいか指示するだけで、リファクタリング・機能追加・デバッグの選択肢が大きく広がります。

`henji -f "このコードを改善するとしたらどう思う？" < main.go | glow`

<p><img src="https://github.com/charmbracelet/mods/assets/25087/738fe969-1c9f-4849-af8a-cde38156ce92" width="900" alt="a GIF of mods offering code refactoring suggestions"></p>

### 新機能のアイデアを出す

henjiはソースコード（やREADMEファイル）を元に、全く新しい機能案を考えることもできます。

`henji -f "このツールに10個の新機能を提案して" < main.go | glow`

<p><img src="https://github.com/charmbracelet/mods/assets/25087/025de860-798a-4ab2-b1cf-a0b32dbdbe4d" width="900" alt="a GIF of mods suggesting feature improvements"></p>

### ドキュメント作成を手伝ってもらう

henjiは新しいドキュメントの下書きをすぐに作ってくれます。

`henji "rキーを押すと無料でウサギが送られてくる機能について、このreadmeに新しいセクションを書いて" < README.md | glow`

<p><img src="https://github.com/charmbracelet/mods/assets/25087/c26a17a9-c772-40cc-b3f1-9189ac682730" width="900" alt="a GIF of mods contributing to a product README"></p>

### 動画ファイルを整理する

ファイルシステムはhenjiへの入力として非常に優秀な情報源になります。音楽や動画ファイルがあれば、henjiは`ls`の出力を解析して内容をうまく整理・要約してくれます。

`ls ~/vids | henji -f "これらを年代別に整理してそれぞれ要約して" | glow`

<p><img src="https://github.com/charmbracelet/mods/assets/25087/8204d06a-8cf1-401d-802f-2b94345dec5d" width="900" alt="a GIF of mods oraganizing and summarizing video from a shell ls statement"></p>

### おすすめを生成してもらう

henjiは手元にあるものを元に、似たジャンルだけでなく、全く別のメディア（例えば持っている映画から音楽のおすすめを出す、など）でもおすすめを生成するのが得意です。

`ls ~/vids | henji -f "これらを元に10個の番組をおすすめして、あまり有名でないものにして" | glow`

`ls ~/vids | henji -f "これらの番組を元に10枚のアルバムをおすすめして、サントラや番組内で使われた曲は除外して" | glow`

<p><img src="https://github.com/charmbracelet/mods/assets/25087/48159b19-5cae-413b-9677-dce8c6dfb6b8" width="900" alt="a GIF of mods generating television show recommendations based on a file listing from a directory of videos"></p>

### 運勢を占ってもらう

ダウンロードフォルダはいつの間にか収拾のつかないファイルの山になりがちですが、henjiがあればそれを逆手に取れます！

`ls ~/Downloads | henji -f "このファイル群を元に私の運勢を占って" | glow`

<p><img src="https://github.com/charmbracelet/mods/assets/25087/da2206a8-799f-4c92-b75e-bac66c56ea88" width="900" alt="a GIF of mods generating a fortune from the contents of a downloads directory"></p>

### APIを理解する

henjiは`curl`によるAPI呼び出しの出力を解析し、人間が読みやすい形に変換できます。

`curl "https://api.open-meteo.com/v1/forecast?latitude=29.00&longitude=-90.00&current_weather=true&hourly=temperature_2m,relativehumidity_2m,windspeed_10m" 2>/dev/null | henji -f "この気象データを人向けに要約して" | glow`

<p><img src="https://github.com/charmbracelet/mods/assets/25087/3af13876-46a3-4bab-986e-50d9f54d2921" width="900" alt="a GIF of mods summarizing the weather from JSON API output"></p>

### コメントを読む（自分で読まなくて済むように）

APIと同じように、henjiは生のHTMLを読んで内容を要約できます。

`curl "https://news.ycombinator.com/item?id=30048332" 2>/dev/null | henji -f "コメントの投稿者たちは何を言っている？" | glow`

<p><img src="https://github.com/charmbracelet/mods/assets/25087/e4d94ef8-43aa-45ea-9be5-fe13e53d5203" width="900" alt="a GIF of mods summarizing the comments on hacker news"></p>
