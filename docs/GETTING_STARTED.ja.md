# はじめかた(フォーク版 okf)

[Mokkori-Tom/okf](https://github.com/Mokkori-Tom/okf) の `ja-human` ブランチは、
本家 okfcli/okf に「人が端末で使う」ための機能を足したものです。
Markdown のノートを作り、日本語で探し、読むことが CLI だけでできます。
エージェントやスクリプトからは、本家と同じ JSON で使えます。

## 1. 入れる

Go 1.27 以上が必要です。フォーク版は `go install github.com/Mokkori-Tom/okf/...` では入りません
(モジュール名が本家のままのため)。clone してビルドします。

```bash
git clone -b ja-human https://github.com/Mokkori-Tom/okf.git
cd okf
go build -o ~/.local/bin/okf ./cmd/okf      # Linux / macOS
go build -o $PREFIX/bin/okf ./cmd/okf       # Termux
```

Termux の Go が古いときは、PC でビルドして持っていきます。

```bash
CGO_ENABLED=0 GOOS=android GOARCH=arm64 go build -o okf-android-arm64 ./cmd/okf
# できたファイルを Termux の $PREFIX/bin/okf に置き、chmod +x する
```

更新は `git pull` してから、同じ `go build` をもう一度実行します。

## 2. ノートの置き場所(bundle)を決める

bundle は、ノート(`.md`)を入れるディレクトリです。既にある Obsidian の vault をそのまま使えます。
シェルの設定(`~/.zshrc` や `~/.bashrc`)に書いておくと、どこからでも bundle を省略できます。

```bash
export OKF_BUNDLE="$HOME/notes"
```

`OKF_BUNDLE` が無いときは、今いるディレクトリが bundle になります。
symlink の先にある bundle も、そのままの path で使えます。

## 3. 使う

```bash
okf new 日記/2026-10-22 --tag 日記     # ページを作って $EDITOR で開く(上書きはしない)
okf new gpu-fan --title "GPUのファン調整" --description "電力上限の記録"
okf search 電力 上限                   # 部分一致・AND。ひらがな/カタカナ、全角/半角の英数字の違いを吸収
okf search --tag 日記                  # タグで絞る(--type も使える)
okf show gpu-fan                       # ID で読む
okf show 日記/2026-10-22.md            # ファイルの path でも読める
okf list                               # 一覧
okf backlinks gpu-fan                  # このページへリンクしているページ
okf validate                           # OKF の規格どおりかを検査
```

- 端末では人が読みやすい表示になり、パイプやエージェントからは JSON が出ます。
  `--human` / `--json`、または `OKF_OUTPUT=human|json` で切り替えを固定できます。
  `okf show x | less` のようにパイプへ流すと JSON になるので、人が読むときは `--human` を付けます。
- エディタは `$VISUAL`、次に `$EDITOR`、無ければ nvim・vim・vi・nano の順で探します。
- `okf new` の作者は `$OKF_AUTHOR`、無ければ `human:$USER` が `generated.by` に入ります。

## 4. Obsidian の vault から移るとき

書き換えなくても、そのまま list・search・show できます。

| ノートの状態 | 読み込み・検索 | `okf validate` |
|---|---|---|
| frontmatter なし | 使える | 警告1件(`okf/frontmatter/missing`) |
| frontmatter はあるが `type` なし | 使える | 警告1件(`okf/frontmatter/type-required`) |
| `type` あり | 使える | 規格どおり厳密に検査 |
| frontmatter の YAML が壊れている | 使える | エラー(`okf/frontmatter/invalid`) |
| 読めないファイル | 飛ばす(stderr に警告) | エラー(`okf/bundle/unreadable`) |

タイトルが無いノートは、本文の最初の `# 見出し`、それも無ければファイル名で表示します。
OKF に寄せたいノートから順に `type`(例: `Note`)と `title` を書き足していけば十分です。
`status` は `draft`・`stable`・`deprecated` のどれかにします。
独自の進み具合を残したいときは、`progress:` など別のキーに書きます(OKF は独自キーを許しています)。

## 5. 困ったとき

- **何も見つからない / `validate` が `okf/bundle/empty` で失敗する**:bundle の path が違います。
  `okf list --human` で件数を確かめてください。
- **`search` の第1引数が検索語にならない**:第1引数が既存のディレクトリか、`/` を含むか `.`・`~` で始まると
  bundle の path として扱います。検索語だけを書くときは `OKF_BUNDLE` を設定します。
- **規格の詳細**:[OKF SPEC](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md)、
  コマンドの機械向け説明は `okf schema`。
