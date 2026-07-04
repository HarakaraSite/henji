# henjiのMCP機能とセキュリティ設計に関する議論

作成日: 2026-07-04
目的: henji v2.0.0におけるMCP機能の扱いについて、セキュリティ、製品設計、Unix CLIとしての一貫性、フットプリントの観点から判断材料を整理する。

## 1. 現在のhenjiの位置づけ

henjiは本来、次のようなUnixフィルターとして理解できる。

```text
stdin → LLMによるテキスト変換 → stdout
```

典型的な利用例:

```sh
cat error.log | henji "原因を説明して"
git diff | henji "セキュリティ上の問題をレビューして"
curl -fsSL 'https://example.com/data' \
  | henji "必要な情報を抽出して" \
  > result.txt
```

このモデルでは、責務が明確に分かれる。

- ネットワーク取得: `curl`など
- ファイル読取: `cat`、`find`、`rg`など
- 構造化データ処理: `jq`など
- AIによる変換: `henji`
- ファイル保存: shellのリダイレクト
- コマンド実行: shellとユーザー

MCPを搭載すると、henjiは単純なテキストフィルターから、モデル判断で外部操作を実行するエージェントランタイムへ部分的に変化する。

## 2. HENJI-SEC-001の内容

### 概要

現在のMCP実装には、ツール単位・引数単位の認可や、呼び出し前のユーザー確認がない。

処理経路は次のとおり。

```text
mcpTools()
  ↓ 有効なMCPサーバーの全ツールをモデルへ公開
モデル
  ↓ ツール名と引数を生成
OpenAI / Anthropic CallTools()
  ↓
stream.CallTool()
  ↓ 承認なしで即時実行
mcp.go toolCall()
  ↓
MCP server
```

`mcp.go`で確認されるのは主に次の2点だけである。

1. モデルが指定したサーバー名が設定に存在するか
2. `isMCPEnabled`によってサーバー全体が有効か

次の制御は存在しない。

- 個別ツールのallow/deny
- read-onlyとwriteツールの区別
- 引数、パス、送信先の検証
- 実行直前のユーザー確認
- 副作用のある操作に対する追加承認
- sandboxやネットワーク送信制限

### 分類

CWE-441のConfused Deputyに相当する。

henjiはユーザー権限と認証情報を持つ一方、実際にどの操作を行うかを未信頼なモデル出力へ委ねる。モデルが未信頼コンテンツの命令へ従うと、henjiがユーザーの代理として特権操作を実行する。

## 3. 現実的な攻撃シナリオ

### A. filesystem MCPによる機密ファイル取得

前提:

- filesystem MCPが有効
- MCPが機密ファイルを読める
- Webページ、ログ、Issue、リポジトリなど未信頼データをhenjiへ渡す

攻撃対象の文書にモデル向けの命令が埋め込まれていると、モデルがfilesystem MCPの読取ツールを呼ぶ可能性がある。

想定される対象:

- `henji.yml`内のAPIキー
- `.env`
- SSH設定や秘密鍵
- クラウド認証情報
- 非公開ソースコード
- 保存された会話

取得した内容はtool resultとしてモデルへ返される。クラウドLLMを利用している場合、機密データがモデルプロバイダーへ送られる可能性がある。

filesystem MCPが限定ディレクトリだけを公開する場合、被害範囲もそのディレクトリへ限定される。

### B. GitHub MCPでの「読む→従う→書く」

前提:

- GitHub MCPが有効
- GitHub tokenにIssue、コメント、ファイル等への書込み権限がある
- henjiへIssueやPRを調査させる

想定経路:

1. 攻撃者がIssue本文へモデル向け命令を埋め込む。
2. henjiがGitHub MCPでIssueを読む。
3. モデルが埋め込まれた命令へ従う。
4. コメント投稿、Issue作成、ファイル変更等のツールを呼ぶ。
5. ツール結果がモデルへ返され、追加操作が続く。

書き込まれた内容を将来の別エージェントが読む場合、prompt injectionが連鎖する可能性もある。ただし、これは自動的にワーム化するという意味ではない。書込み権限、モデルの挙動、別エージェントによる再読込など複数の条件が必要である。

read-only tokenなら、主な影響は非公開情報の漏えいへ限定される。

### C. シェル実行・外部送信ツール

前提:

- 任意または広範なコマンドを実行できるMCPツールが有効
- MCPサーバーが通常のユーザー権限で動作
- 未信頼コンテンツを処理する

モデルがシェルツールへコマンドを渡すと、次の操作が可能になる。

- ファイルの読取、削除、変更
- 環境変数や資格情報の取得
- 外部サーバーへの送信
- Git操作
- スクリプトやパッケージの実行
- 永続化設定の変更

これは一般的な「henjiへネットワークから直接コマンドを送れるRCE」とは異なる。正確には、ユーザーが明示的に有効化したコマンド実行能力を、未信頼コンテンツがモデル経由で無断使用する問題である。

## 4. Severityの考え方

HENJI-SEC-001の基本SeverityはMediumが妥当。ただし、影響は構成によって大きく変わる。

| MCP構成 | 想定される影響 |
|---|---|
| 公開情報のread-only検索 | Low相当まで低下し得る |
| 限定ディレクトリのread-only filesystem | Low〜Medium |
| 機密ファイルを読める | Medium〜High |
| GitHub等への書込み | High相当になり得る |
| 任意シェル実行・外部送信 | 条件次第でCritical級の実害 |

緩和要因:

- MCP有効化はユーザーの明示設定
- `--mcp-disable`でサーバー単位に停止可能
- read-only構成なら被害上限を制御可能
- filesystem MCP側で公開ディレクトリを限定可能

ただし、これらは事前のサーバー単位設定であり、個々の副作用に対する実行時承認ではない。

## 5. MCPを廃止しても残る危険

次のような利用には、MCPがなくても危険がある。

```sh
cat untrusted.txt | henji | sh
```

未信頼ファイルの内容によってモデルが危険なshellコードを出力すると、`sh`がそのまま実行する。

一方、次のコマンドはファイルを生成するだけであり、その時点では実行しない。

```sh
cat untrusted.txt | henji > candidate.sh
```

危険になるのは、後から次のように実行した場合である。

```sh
bash candidate.sh
source candidate.sh
eval "$(henji ...)"
```

MCPの有無による重要な差:

| MCPあり | MCPなし |
|---|---|
| 推論中にモデル判断だけで副作用が発生する | henjiはテキストを返すだけ |
| ユーザーが個別操作を見ない可能性がある | 実行には呼び出し側の追加操作が必要 |
| モデルが内部で実行主体になる | shellとユーザーが実行主体になる |

したがってMCP廃止はリスクをゼロにはしないが、副作用の境界をhenji内部から明示的なshell操作へ戻せる。

## 6. Web・ファイル操作を既存CLIへ任せる案

当初MCPを導入した目的は、Web検索、fetch、ファイル読書きを個別実装する負担を避けることだった。

しかし、ファイル操作はstdin/stdoutと既存CLIで大部分を表現できる。

```sh
cat input.txt | henji "加工して" > output.txt
rg 'ERROR' logs/ | henji "原因ごとに分類して"
find src -type f | henji "構成を説明して"
```

ネットワークアクセスも、固定された処理なら`curl`等へ委譲できる。

```sh
curl -fsSL --max-time 30 'https://example.com/data' \
  | henji "表形式に整形して" \
  > result.txt
```

henji自身へワンライナーの生成を依頼することもできる。

```sh
henji -R shell \
  'URLをcurlで取得し、henjiで要約してresult.txtへ保存するワンライナー'
```

この構成の利点:

- henjiの責務がテキスト変換に限定される
- ネットワーク、TLS、redirect、proxy等を成熟した`curl`へ任せられる
- 操作内容がshell上で明示される
- 人間が生成コマンドを確認してから実行できる
- バイナリサイズと依存関係を削減できる
- MCP固有の認可、sandbox、監査機構が不要になる

## 7. MCPが必要になるケース

MCPが本当に必要なのは、モデルが実行中に次の操作を動的に決定するエージェント型ユースケースである。

- 検索結果を見て次の検索条件を決める
- Issueを読んで関連Issueを追加取得する
- ファイルを調査しながら別ファイルを選択する
- API結果に応じて更新操作を行う
- 複数ツールを反復的に組み合わせる

つまり境界は次のように整理できる。

- 人間が処理手順を決める: shell pipeline
- モデルが次の操作を自律判断する: MCP / agent runtime

後者を採用するなら、henjiは次の機能を本格的に持つ必要がある。

- ツールallowlist/denylist
- read/write/exec等の危険度分類
- ユーザー承認
- 非対話モード向けの宣言的ポリシー
- 引数、パス、URL、送信先の検証
- sandbox
- ネットワーク制限
- 監査ログ
- plan/apply分離

## 8. mods作者の意図について

作者がUnix pipelineを理解していなかったとは断定できない。

履歴上、MCPは2025年5月の大規模なプロバイダー刷新と同時に追加されている。実装された制御はサーバー単位の有効・無効とtimeoutが中心で、ツール単位の承認層は導入されなかった。

考えられる解釈:

- MCPサーバーを設定した時点で、全ツールへ包括的に同意したと考えた
- MCP機能の追加を優先し、agent hostとして必要な認可層までは実装しなかった
- 初期MCPクライアントによくある単純な実装パターンを採用した

証拠から言えるのは認可層がないことまでであり、作者の認識や意図そのものは断定できない。

参考:

- [modsのMCP導入コミット](https://github.com/charmbracelet/mods/commit/c48ad6badc78ae00727cb35264a574250a218a1c)
- [MCP仕様: Security and Trust & Safety](https://modelcontextprotocol.io/specification/2024-11-05/index)

MCP仕様は、ツールを任意コード実行相当として扱い、ホストが実行前の明示的同意と適切なアクセス制御を提供するよう求めている。ただし、プロトコル自体が承認UIやポリシーを強制するわけではなく、実装責任はホスト側にある。

## 9. 他のMCP搭載ツール

全ツールを自動承認するMCPクライアントは、基本的に同じ問題を持つ。

成熟したagent hostは、MCPの上に独自の安全機構を実装している。

- Claude Code: ツール単位のallow/deny、操作承認、permission mode
- VS Code: ツール名と引数の確認、MCPツール単位の承認、tool resultをコンテキストへ入れる前の確認、URL承認、sandbox
- GitHub Copilot CLI: MCPサーバー・ツール単位のallow/deny、自動承認の危険性に関する警告
- Codex型ツール: workspace sandbox、外部書込みや権限昇格の承認、対話的な確認を重ねられる

ただし、`allow-all`、`bypassPermissions`、`yolo`等で安全装置を解除すれば、henjiと近いリスクへ戻る。

参考:

- [MCP Client Best Practices](https://modelcontextprotocol.io/docs/develop/clients/client-best-practices)
- [VS Code approvals and permissions](https://code.visualstudio.com/docs/agents/approvals)
- [GitHub Copilot CLI tool approvals](https://docs.github.com/en/copilot/concepts/agents/copilot-cli/about-copilot-cli)
- [Claude Code security](https://docs.anthropic.com/fr/docs/claude-code/security)

## 10. 常駐型・自律型エージェントとの比較

安全性は概念的には次の要素で決まる。

```text
自律性 × 権限 × 未信頼入力 × 稼働時間
  − sandbox − 承認 − allowlist − 監査
```

Codexのような対話型・作業単位型ツールは、workspace制限、コマンド承認、sandbox、権限昇格の確認、人間による途中確認を配置しやすい。

一方、OpenClawのような常駐型agent runtimeは、構造的にリスクが高くなりやすい。

- メール、チャット、Webなど未信頼入力を自動で読む
- ブラウザ、ファイル、SaaS資格情報を持つ
- 人間が見ていない時間にも動く
- 長期メモリや過去のtool resultが次の判断へ影響する
- 複数人が同一agentの権限を誘導できる場合がある

OpenClaw自身もsandbox、workspaceアクセス制限、security audit等を提供し、このリスクを認識している。しかし、広い権限を与え、通常の個人環境で常駐させる構成は危険度が高い。

参考:

- [OpenClaw Security](https://docs.openclaw.ai/security)

## 11. フットプリントとの関係

MCPはセキュリティ設計だけでなく、バイナリサイズと依存関係にも影響する。

henjiのリリースビルドは約38〜40MBで、主なサイズ要因にはSQLite、プロバイダーSDK、Glamour/Chroma、MCP SDKが含まれる。

MCPを廃止すると期待できる効果:

- MCP SDKと関連依存の削除
- MCP transport、client pool、tool schema変換コードの削除
- tool-call対応のproviderコード簡素化
- agent loopとtool-call保存互換処理の削減可能性
- SEC-001とSEC-002の原因そのものを削除
- ユースケース説明と設定項目の単純化

正確な削減量はMCP削除ブランチをビルドして比較する必要がある。

## 12. HENJI-SEC-002との関係

HENJI-SEC-002は、`max-tool-calls`の上限判定が実際のtool call後に行われる問題である。

- MCPを残す場合: SEC-001とは別に修正が必要
- MCPを廃止する場合: 関連コードと共に問題自体が消える

SEC-001をv2.0.0で保留する判断は、SEC-002を自動的に無効化しない。最終的なMCP方針によって扱いを決める必要がある。

## 13. 現在の暫定判断

### 決定済み

- HENJI-SEC-001は誤検知ではない。
- v2.0.0ではHENJI-SEC-001の対応を保留する。
- 保留は「解決」ではなく、リスク受容として記録する。
- 非現実的な攻撃シナリオではなく、MCP権限と未信頼入力が組み合わさる場合の現実的なリスクである。

### 有力な方向性

- henjiをUnix filterとして明確化する。
- ファイル操作はstdin/stdoutと既存CLIへ任せる。
- ネットワーク取得は`curl`等へ任せる。
- 副作用は人間が明示的に記述・確認するshell pipeline側で発生させる。
- MCP廃止を選択肢として本格検討する。

### 未決事項

- henjiにモデル主導の反復的なツール利用が本当に必要か
- MCP利用者が実際に存在するか、どのツールを使っているか
- read-only MCPだけを残す価値があるか
- MCP完全廃止、read-only限定、本格的な権限制御実装のどれを選ぶか
- v2.0.0で廃止するか、非推奨化期間を設けるか
- MCP削除によるバイナリサイズと起動・ビルド時間の実測値

## 14. 選択肢

### Option A: MCPを完全廃止

利点:

- 製品境界が明確
- SEC-001/002を根本的に除去
- バイナリと依存関係を削減
- 説明、設定、テスト、保守を簡素化
- Unix CLIとして一貫する

欠点:

- モデル主導の反復的ツール利用ができない
- 現在のMCP利用者には破壊的変更

### Option B: read-only MCPに限定

利点:

- 検索・取得用途の一部を維持
- 書込み・シェル実行の危険を抑えられる

欠点:

- 機密情報の読取と外部LLMへの送信リスクは残る
- MCPツールのread-only性をhenjiが信頼・分類する必要がある
- 実装と依存関係は残る

### Option C: MCPをagent機能として本格実装

必要なもの:

- ツール単位の権限
- 対話承認
- headless policy
- sandbox
- path/URL/argument制限
- 監査ログ
- plan/apply

利点:

- 本格的なagent runtimeとして発展可能

欠点:

- henjiの現在の規模と目的に対して実装・保守コストが大きい
- バイナリと依存関係がさらに増える可能性がある

## 15. Fableと議論したい質問

1. henjiの主要価値を「AI対応Unix filter」と「非対話型agent runtime」のどちらに置くべきか。
2. `curl | henji > file`で代替できない、具体的かつ重要なMCPユースケースは何か。
3. モデルが次の操作を動的に選ぶ必要があるユースケースは、henji利用者に本当に必要か。
4. MCPを残す場合、read-onlyを機械的に保証する方法はあるか。ツール自己申告だけで十分か。
5. 非対話CLIで安全なツール承認を設計するとしたら、どのようなpolicy形式が必要か。
6. MCP廃止による機能損失と、セキュリティ・サイズ・保守性の改善は釣り合うか。
7. v2.0.0で即時削除すべきか、deprecatedとして1リリース残すべきか。
8. MCP機能の実利用状況を確認するために、どの利用者・Issue・設定例を調査すべきか。
9. henjiが生成するshell commandを人間が確認して実行する設計を、製品の中心ユースケースとして明文化すべきか。
10. SEC-001をリスク受容したままv2.0.0へ含める場合、最低限どの警告・ドキュメント・既定値変更が必要か。

## 16. 現時点の技術的評価

現時点では、MCP完全廃止が最もhenjiの性格に整合している。

理由:

- ファイルI/Oはstdin/stdoutで代替可能
- 固定的なネットワーク取得は`curl`で代替可能
- shell pipelineが操作内容と承認境界を明示する
- henji内部で副作用を起こさない設計へ戻せる
- agent runtimeに必要な安全基盤を中途半端に実装せずに済む
- バイナリサイズと保守対象を削減できる

ただし、最終判断には実際のMCP利用状況と、shell pipelineでは代替できない具体的ユースケースの確認が必要である。
