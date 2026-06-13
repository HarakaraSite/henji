# `--delete-older-than` 修正メモ

## 対象

Issue #633: `--delete-older-than` が指定期間より古い会話だけを削除するべきところ、条件次第で新しい会話まで削除対象に入る可能性があった。

実装箇所は以下。

- `main.go`
  - `deleteConversationOlderThan()`
  - `db.ListOlderThan(config.DeleteOlderThan)` の結果を表示/確認し、対象会話を DB と cache から削除する。
- `db.go`
  - `convoDB.ListOlderThan(t time.Duration)`
  - `updated_at < cutoff` で削除候補を取得する。

## timestamp の扱い

会話 DB の table は `db.go` の `openDB()` で作られる。

- `created_at` はない。
- 時刻として保持しているのは `updated_at` のみ。
- insert 時の default は SQLite の `strftime('%Y-%m-%d %H:%M:%f', 'now')`。
- update 時は `CURRENT_TIMESTAMP`。
- SQLite の `now` / `CURRENT_TIMESTAMP` は UTC。

つまり、会話の古さ判定は「作成日時」ではなく「最終更新日時」で行われている。これは既存 CLI 仕様を変えないため、そのまま維持する。

## 原因

修正前の `ListOlderThan` は cutoff を以下のように作っていた。

```go
time.Now().Add(-t)
```

この値を `time.Time` のまま SQLite query parameter に渡していた。

一方、DB に保存される `updated_at` は SQLite が生成する UTC の日時文字列。ローカル timezone が UTC 以外の場合、たとえば JST では `time.Now().Add(-1h)` のローカル時刻が、SQLite に保存された UTC 時刻より約 9 時間進んだ値として比較される。

そのため、実際には 30 分前の会話でも、DB 上の UTC 文字列とローカル cutoff の比較では「1 時間より古い」と判定されることがあり、同日の新しい会話まで削除候補に入る可能性があった。

## 修正

DB スキーマや CLI 仕様は変えず、`ListOlderThan` の cutoff だけを SQLite timestamp と同じ基準に揃えた。

```go
cutoff := time.Now().UTC().Add(-t).Format(sqliteTimestampFormat)
```

これにより、`updated_at` と cutoff の比較がどちらも UTC の `YYYY-MM-DD HH:MM:SS` 系文字列で行われる。

## テスト

`db_test.go` に `TestListOlderThanUsesSQLiteTimestampFormat` を追加した。

テストでは timezone を JST に固定し、DB には SQLite と同じ UTC timestamp 文字列を入れる。

- 2 時間前の会話
- 30 分前の会話
- `ListOlderThan(1 * time.Hour)`

という条件で、2 時間前の会話だけが返ることを検証する。
