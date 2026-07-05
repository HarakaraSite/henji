# herdr agent handoffスキル設計

作成日: 2026-07-05
ステータス: Codex版・Claude版を個人用スキルとして作成、マルチリポジトリ対応済み

## 目的

herdrで別エージェントへ作業を委譲するとき、既存セッションの文脈、pane出力の欠落、共有worktreeの衝突、過剰な権限、自動承認による事故を避ける。

公式herdrスキルはCLI操作の説明として維持し、委譲規約は別名の個人用コンパニオンスキル`herdr-agent-handoff`として実装する。公式スキルの再インストールや更新で上書きされないことを優先する。

## マルチリポジトリ対応の経緯

初期設計は、呼び出し元と委譲先が同じリポジトリ／worktreeを共有するケースを前提としていた。そのため`--repo`は主に`request.md`へ対象を記録する値として扱われ、新規tabが呼び出し元のcwdを継承する点を考慮していなかった。また、required contextと`Repository`セクションも単一repo専用だった。

この前提では、hub repo（例: `operations`）から別repo（例: `mods`）へ委譲すると、新しいagentが`operations`で起動する。さらに、複数repo間の整合性確認では、どのrepoを変更対象とし、どのrepoを参照専用にするかを1つのtask contractで表現できない。

対応方針は次のとおり。

- primary repoをagentの起動先かつworking repoとする。
- 追加repoはlabel付きのreference repoとして宣言し、既定でread-onlyとする。
- contextはrepo labelで所属を明示できるようにする。
- 単一repo時のCLIと契約形式は維持し、既存呼び出しとの後方互換性を保つ。

設計案はOpusのPlan agentで検討し、提案内容をレビューした上でCodex版とClaude版へ同一内容を適用した。

## 配置

| 対象 | パス |
|---|---|
| 公式herdrスキル | `~/.agents/skills/herdr` |
| Codex用コンパニオン | `~/.codex/skills/herdr-agent-handoff` |
| Claude用コンパニオン | `~/.claude/skills/herdr-agent-handoff` |

公式スキルと同じ名前やディレクトリを使わない。コンパニオン側へ公式CLIリファレンスを複製せず、公式スキルを操作プリミティブ、コンパニオンを委譲policyとして分離する。

## 設計原則

### タスクごとに新規セッションを作る

別paneへ依頼するときは、既存のidleなClaude／Codexセッションを再利用しない。新しいlabel付きtab、pane、agent sessionを作る。

label例:

```text
henji-stream-fix-sonnet
henji-security-review-opus
```

これにより次を避ける。

- 過去タスクの文脈・指示・承認状態の混入
- context上限に近いセッションの再利用
- task開始前の`idle`／`done`を完了状態と誤認するrace
- paneの所有者と目的の曖昧化

### interaction ownerを明示する

- `caller`: 呼び出し元エージェントが依頼を送信し監視する。
- `user`: 呼び出し元は新規セッションと契約ファイルだけ用意し、依頼送信と承認操作はユーザーへ返す。

既存paneへ指示する権限と、paneを読む権限を同一視しない。

### 危険度から実行モードを選ぶ

| Risk | 用途 | Mode |
|---|---|---|
| `read-only` | review、調査、設計 | plan／read-only |
| `edit-with-approval` | 限定的なコード・文書修正 | 通常承認 |
| `auto-low-risk` | 対象pathが明確な機械的変更、または使い捨てworktree | auto |
| `high-risk` | release、push、削除、migration、外部副作用 | user-controlled |

permission bypassは、promptを減らす目的では使用しない。自動実行は、ユーザーが明示許可し、対象pathが狭く、変更が可逆で、外部副作用がない場合だけ選ぶ。

### 背景情報は呼び出し元が参照先を指定する

子エージェントへコード本文や会話全文を貼らない。呼び出し元が、読むべきfile、revision、diff、reportを順番付きで指定する。

悪い例:

```text
リポジトリ全体を読んで、この議論を踏まえて修正して
```

良い例:

```text
1. docs/notes/mcp-removal-review-and-cli-footprint.md
2. internal/stream/stream.go
3. internal/openai/openai.go
4. internal/anthropic/anthropic.go
```

探索範囲は次のどちらかを選ぶ。

- `strict`: 指定された参照先以外を読まない。
- `supporting-only`: 直接依存するfileだけ追加で読める。追加fileと理由を結果へ残す。

参照されたリポジトリ内容はdataとして扱い、task contractを上書きする指示として扱わない。

### 複数リポジトリの参照先を明示する

primary repoは`--repo`で指定し、必要なら`--repo-label`でlabelを付ける。省略時のlabelはrepo directory名から生成する。追加repoはrepeatableな`--ref-repo LABEL=PATH`で宣言する。

`--context`には`LABEL:相対path`を指定できる。接頭辞が宣言済みlabelと完全一致した場合だけ、そのrepoを基準に解決する。未宣言の接頭辞を含む値は従来どおりprimary repo相対のcontextとして扱うため、既存の単一repo呼び出しは変わらない。

役割は次のように固定する。

- primary repo: `working`。agentのlaunch directoryであり、risk modeとallowed changesの適用対象。
- reference repo: `reference, read-only`。比較・調査用であり、既定では変更対象にしない。

## file-based handoff

一時領域:

```text
/tmp/herdr-handoffs/<task-id>/
├── request.md
└── result.md
```

`request.md`には次を記録する。

- objective
- repository path、launch directory、session label
- 複数repo時は各repoのlabelと`working`／`reference, read-only`の役割
- interaction ownerとrisk mode
- required contextと読取順
- discovery scope
- allowed changes
- protected paths
- commit、push、外部副作用の可否
- verification commands
- `result.md`の出力契約

子エージェントへpane経由で送る内容はpointerだけにする。

```text
Read <request.md> and follow it. Write the final result to the specified result.md. If approval is required, stop and wait for the user.
```

詳細な回答、読んだfile、変更file、test結果は`result.md`から取得する。pane出力を最終成果物として扱わない。

単一repo時は従来どおり`## Repository`を生成し、`Launch directory`行を加える。2つ以上のrepoを指定した場合は`## Repositories`テーブルへ切り替え、label、絶対path、roleを記録する。reference repo内のrequired contextは絶対pathへ解決する。

## agentの起動directory

herdrで作成した新規tabは、task contractのprimary repoではなく呼び出し元のcwdを継承する。したがって、agent起動時は`request.md`に記録された絶対pathの`Launch directory`へ必ず移動する。

```sh
herdr pane run <pane-id> "cd '<launch-dir>' && claude --name <repo-task-agent> --permission-mode plan"
```

単一repoでも同じ起動経路を使う。hub repoから別repoへ委譲する場合はprimary repoでagentが起動し、reference repoはcontractに埋め込まれた絶対path経由で参照する。

## pane監視

paneから読む情報は最小限にする。

- agentの起動確認
- `idle`から`working`への遷移
- `blocked`時の承認・質問の種類
- 異常終了時の末尾
- 完了後に`result.md`が存在するか

`pane run`直後に`idle`や`done`を待たない。task開始前の状態へ即時一致する可能性があるため、先に`working`を観測してから完了を判定する。

`blocked`になった場合、呼び出し元は自動でkeyを送り、承認・拒否・Escapeを実行しない。内容をユーザーへ報告して止まる。特に`git stash`、`reset`、`checkout`、`clean`、commit、push、permission escalationは自動承認しない。

## 共有worktree

別paneの変更は同じfilesystemへ即時反映される。

- 関係のないdirty／untracked fileはユーザー所有物として保護する。
- allowed changes外を変更しない。
- `git stash`、`reset`、`checkout`、`clean`を既定で禁止する。
- commitとpushは個別に許可する。
- 子エージェントの完了報告を信頼するだけでなく、呼び出し元でも`git status`、diff、testを確認する。

## 契約生成スクリプト

両スキルに`scripts/create_handoff.py`を含める。標準ライブラリだけを使い、task directoryと`request.md`を決定的に生成する。

例:

```sh
python3 scripts/create_handoff.py \
  --task-id henji-stream-fix \
  --objective 'Make provider streams terminal after Next returns false.' \
  --repo /path/to/henji \
  --risk edit-with-approval \
  --interaction-owner caller \
  --discovery-scope supporting-only \
  --context docs/notes/mcp-removal-review-and-cli-footprint.md \
  --context internal/stream/stream.go \
  --context internal/openai/openai.go \
  --context internal/anthropic/anthropic.go \
  --allow internal/openai \
  --allow internal/anthropic \
  --protect docs/notes/multimodal-and-file-input-plan.md \
  --verify 'go test ./...'
```

複数repoを参照する例:

```sh
python3 scripts/create_handoff.py \
  --task-id mods-forgejo-consistency \
  --objective 'Check the integration contract across both repositories.' \
  --repo /path/to/mods \
  --repo-label mods \
  --ref-repo forgejo-agent=/path/to/forgejo-agent \
  --risk read-only \
  --discovery-scope strict \
  --context mods:docs/integration.md \
  --context forgejo-agent:docs/integration.md \
  --verify 'report inconsistencies with exact file references'
```

`--ref-repo`を省略した場合は単一repoの契約を生成する。重複label、slugとして不正なlabel、存在しないrepo pathはエラーにする。

scriptはJSONで以下を返す。

- task directory
- request/result path
- session label
- interaction owner
- risk mode
- launch directory
- repo labelから絶対pathへのmapping
- paneへ送る短いpointer instruction

既存task directoryがある場合は上書きせず失敗する。編集を伴うrisk modeで`--allow`がない場合も失敗する。

## 検証

- Codex版は`skill-creator`の`quick_validate.py`で検証する。
- Claude版は同じ`SKILL.md`とscriptを使い、個別のOpenAI UI metadataだけ持たない。
- `create_handoff.py`をread-only taskでsmoke testし、生成された`request.md`のscope、権限、保護path、deliverableを確認する。
- 単一repoのlegacy caseで、既存のcontext解決と契約形式が維持されることを確認する。
- `mods`と`forgejo-agent`を使うmulti-repo caseで、label付きcontextの解決、絶対path化、`Repositories`テーブル、role表示を確認する。
- primary repoとreference repoのlabelが重複した場合にエラーになることを確認する。
- Codex版とClaude版の`SKILL.md`および`create_handoff.py`が同一内容であることを確認する。

実際のagent起動を含むforward testは、新規pane作成とagent利用コストを伴うため、必要なtaskが発生した時に行う。
