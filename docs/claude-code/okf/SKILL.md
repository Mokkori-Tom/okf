---
name: okf
description: >
  ユーザーのノート（OKF バンドル、場所は $OKF_BUNDLE）を okf コマンドで検索・閲覧・作成・検証する。
  ユーザーの環境・過去の作業・設定・検証結果、このマシンのツールの挙動について聞かれたとき、
  メモやノートを残してと言われたとき、OKF / Obsidian のノートやバンドルを扱うときは、
  自分の知識や grep より先にこれを使う。
---

# okf

ノートは OKF バンドル（Markdown + YAML frontmatter）。操作は必ず `okf` で行い、ファイルを直接 grep・編集しない。

- パイプ経由（Agent からの実行）では出力は常に JSON。エラーは `{"error": {"kind": ...}}`、終了コード 4 は使い方の誤り。
- バンドル引数は省略できる（`$OKF_BUNDLE`、なければカレントディレクトリ）。
- オプションは推測しない。ここに無いことは `okf schema <command>` で確かめる（`--help` より短く正確）。

## 調べる

```sh
okf search 電力 上限            # 語は AND。日本語・かな/カナ・全角半角の揺れを吸収
okf search --tag gpu --type Note
okf show notes/gpu              # 本文と frontmatter
okf backlinks notes/gpu         # このページを参照しているページ
```

- 検索結果の `matches` は該当行の抜粋だけ。答える前に必ず `okf show <id>` で本文を読む。一覧だけで「ノートに無い」と判断しない。
- `list` / `search` / `graph` は JSON で最初の 50 件まで返す。`count` が総数、`truncated: true` なら残りがある。
  `--limit 0` で全件取るより、語・`--tag`・`--type` で絞り込む。

## 書く

```sh
okf new notes/<id> --type Note --title "..." --description "一行要約" --tag a,b --body - <<'MD'
本文（Markdown）
MD
```

- 1 回の呼び出しで完成したページを作る。エディタは開かない。
- 既存のページは上書きも削除もしない。`already exists` で止まったら、別の ID にするか既存ページへの追記でよいかをユーザーに聞く。
- 書いたあとは `okf validate` でエラーが無いことを確かめる（警告は直さなくてよい）。

## 点検する

```sh
okf validate        # errors > 0 なら findings の rule と message を見て直す
okf lint            # 推奨フィールドの不足（警告のみ）
okf graph           # node_count, isolated（孤立ページ）など
```
