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
シェルの設定に書いておくと、どこからでも bundle を省略できます。
zsh なら `~/.zshrc` ではなく `~/.zshenv` に書きます。エージェントやスクリプトが使う非対話のシェルにも届くためです(bash は `~/.bashrc`)。

```bash
export OKF_BUNDLE="$HOME/notes"
```

Tab でページ名・タグ・type・オプションを補完できるように、補完スクリプトも読み込んでおきます。
zsh(oh-my-zsh や `compinit` の後)と bash に対応しています。

```bash
okf completion zsh >/dev/null 2>&1 && source <(okf completion zsh)
# bash なら: okf completion bash >/dev/null 2>&1 && source <(okf completion bash)
```

前半で補完に対応しているかを確かめるので、古い okf や、okf が入っていない環境では何もしません
(`source <(okf completion bash)` だけだと、古い版のエラー出力を読み込んでシェルがエラーを出します)。

`OKF_BUNDLE` が無いときは、今いるディレクトリが bundle になります。
symlink の先にある bundle も、そのままの path で使えます。

## 3. 使う

```bash
okf new 日記/2026-10-22 --tag 日記     # ページを作って $EDITOR で開く(上書きはしない)
okf new gpu-fan --title "GPUのファン調整" --description "電力上限の記録"
okf new 物理/Kapitsa --type Person --title カピッツァ --tag 物理 --body - < 本文.md
                                       # 本文ごと1回で作る(--body "文字列" も可。エディタは開かない)
                                       # 本文の先頭に --- で囲んだ frontmatter を書くと、そのキー
                                       # (created・sources など)も足される。同じキーはフラグが優先
okf search 電力 上限                   # 部分一致・AND(語を足すほど絞られる)。全角スペース区切りも可。
                                       # ひらがな/カタカナ、全角/半角の英数字の違いを吸収。
                                       # 空白を含む語句そのものを探すときは --text "語 句"
okf search --tag 日記                  # タグで絞る(--type も使える)
okf show gpu-fan                       # ID で読む
okf show 日記/2026-10-22.md            # ファイルの path でも読める
okf edit gpu-fan                       # エディタで開く(show の画面で v を押しても同じ)
okf list                               # 一覧
okf backlinks gpu-fan                  # このページへリンクしているページ
okf validate                           # OKF の規格どおりかを検査
okf new -h                             # どのコマンドも -h / --help でヘルプ(okf help new も同じ)
```

- 端末での `okf show` は、ページを整形して表示します。見出しの `#` は消し、箇条書き・チェックボックス・
  Obsidian の callout・コードブロック・表(日本語の幅で揃え、入りきらなければ1行ずつ「見出し: 値」の形に)・
  太字やリンクを描きます。折り返しは日本語の禁則に従います。
  長いページは `$OKF_PAGER`、次に `$PAGER`、無ければ `less` で開きます。
  書いたままの Markdown を見たいときは `--raw`、ページャーを使わないときは `--no-pager` を付けます。
  ページャーが less のときは、`v` を押すと表示中のページをエディタで開けます。
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
`[[ページ名]]` 形式のリンクも、`okf backlinks`・`okf graph` で数えます。
OKF の規格の外なので `okf validate` は検査しません(まだ書いていないページへのリンクは警告になりません)。
名前だけのリンクは Obsidian と同じく vault のどこからでも探し、同じ名前が複数あれば同じフォルダ、次に短いパスを選びます。
OKF に寄せたいノートから順に `type`(例: `Note`)と `title` を書き足していけば十分です。

上の寛容さは、このフォークが足したものです。bundle が OKF として正しいかを確かめたいときは
`okf validate --strict` を使います。frontmatter や `type` の無いノートはエラーになり、
各項目の検査も行い、`[[wikilink]]` もページごとに警告として出します(`okf lint --strict` も同じ)。
`status` は `draft`・`stable`・`deprecated` のどれかにします。
独自の進み具合を残したいときは、`progress:` など別のキーに書きます(OKF は独自キーを許しています)。

## 5. PicoClaw から使う

[picoclaw/AGENT.md](picoclaw/AGENT.md) を PicoClaw の `workspace/AGENT.md` に置き、`OKF_BUNDLE` を設定して起動します。
PicoClaw の既定の文面に 3 行(okf を使うこと、使い方の調べ方、既存ノートを消さないこと)を足したものです。
最後の行は外さないでください。これが無いと、`okf new` が「already exists」で止まったときに、
モデルが既存のノートを `rm` で消して作り直すことがあります(Qwen3.6-35B-A3B で 3 回中 2 回、この行で 0 回)。

## 6. 困ったとき

- **何も見つからない / `validate` が `okf/bundle/empty` で失敗する**:bundle の path が違います。
  `okf list --human` で件数を確かめてください。
- **`search` の第1引数が検索語にならない**:第1引数が既存のディレクトリか、`/` を含むか `.`・`~` で始まると
  bundle の path として扱います。検索語だけを書くときは `OKF_BUNDLE` を設定します。
- **規格の詳細**:[OKF SPEC](https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md)、
  コマンドの機械向け説明は `okf schema`。
