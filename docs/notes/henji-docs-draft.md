# `henji docs` 実装記録

作成日: 2026-07-04

状態: 実装済み

このファイルは `henji docs` の内容案として作成された。実装後の本文の一次情報は
`internal/docs/docs.md` とし、ここには設計判断と検証結果だけを残す。

## 実装した構成

- `internal/docs/docs.md`: バイナリへ埋め込むプレーンMarkdown本文
- `internal/docs/docs.go`: `//go:embed` と実行時version headingの付与
- `henji docs`: 本文をANSI装飾なしでstdoutへ出力するCobra subcommand
- root help: `Commands:` セクションと `Full manual: henji docs` の誘導
- `docs` 専用の早期起動経路: config読込と会話DB初期化を行わない

## 内容の役割分担

- `henji --help`: フラグ一覧と短い説明のquick reference
- `henji docs`: タスク指向の手順、出力契約、組み合わせ、落とし穴

同じフラグ説明を二箇所で逐語的に管理せず、docs側は利用判断に必要な関係と制約を
説明する。

## 実装前レビューで修正した記述

- `--output json` のerror envelopeはmodel実行経路のエラーに限定。flag、config、
  schema fileなどstartup errorはstderrへ出る
- 引数なしTTYでは対話promptを開き、空stdinの非対話実行はerrorになる
- 自動保存対象を「成功したmodel conversation」に限定
- 環境変数overrideはenv mappingを持つscalar設定に限定し、`apis`、`roles`、
  `mcp-servers`等の構造化設定はYAMLに残る
- Google native providerはMCP tool calling未対応
- `strict:true` はprovider能力の一般論ではなく、henjiが`openai`というAPI名に対して
  送るときだけ有効
- 一般的なlocal gatewayではdummy API keyを使えるが、全local serverがkeyを無視する
  とは限らない
- stdinはargsの後へ空行区切りで追加され、各行がindentされる

## 機械検証

- 本文上限: 12 KiB。超える場合はtopic分割を検討する
- 本文中のlong flagがroot commandに実在することを検査
- version heading、主要section、ANSI escape不在を検査
- configが読めない状態でも`henji docs`が実行できることを実バイナリで確認

本文は実装時点で約8.2 KiB。初版ではtopic subcommandを作らず単一ページとする。
