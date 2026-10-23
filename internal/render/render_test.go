package render

import (
	"strings"
	"testing"
)

func plain(md string, width int) string { return Markdown(md, Options{Width: width}) }

func TestInlineHidesMarkup(t *testing.T) {
	got := plain("**太字** と *斜体* と `code` と [[note|別名]] と [[a#見出し]] と [リンク](https://x) と ~~消~~ と ==印== #タグ [^1]", 200)
	want := "太字 と 斜体 と code と 別名 と a › 見出し と リンク と 消 と 印 #タグ [1]\n"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestSnakeCaseIsNotItalic(t *testing.T) {
	if got := plain("use snake_case_name here", 80); got != "use snake_case_name here\n" {
		t.Errorf("got %q", got)
	}
}

func TestHeadingsListsTasks(t *testing.T) {
	md := "# 題\n\n## 節\n\n- 一\n  - 二\n- [ ] やる\n- [x] 済み\n1. 番号\n"
	want := "題\n━━\n\n節\n──\n\n• 一\n  ◦ 二\n[ ] やる\n[x] 済み\n1. 番号\n"
	if got := plain(md, 80); got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestJapaneseWrapAndKinsoku(t *testing.T) {
	// 10 wide runes per line at width 20; "。" must not start a line.
	got := plain("あいうえおかきくけこ。さしすせそ", 20)
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	for _, l := range lines {
		if Width(l) > 20 {
			t.Errorf("line %q is %d cells wide", l, Width(l))
		}
		if strings.HasPrefix(l, "。") {
			t.Errorf("line starts with 。: %q", got)
		}
	}
	// A latin word is not split mid-word when a break exists.
	got = plain("日本語の文章に extraordinarily という単語", 24)
	if strings.Contains(got, "extra\n") || strings.Contains(got, "extraor\n") {
		t.Errorf("word split: %q", got)
	}
}

func TestListContinuationHangs(t *testing.T) {
	got := plain("- あいうえおかきくけこさしすせそ", 20)
	lines := strings.Split(strings.TrimSuffix(got, "\n"), "\n")
	if len(lines) < 2 || !strings.HasPrefix(lines[1], "  ") {
		t.Errorf("no hanging indent: %q", got)
	}
}

func TestCalloutAndQuote(t *testing.T) {
	got := plain("> [!warning] 情報の鮮度\n> 買う前に確認する。\n\n> ただの引用", 80)
	want := "│ 情報の鮮度\n│ 買う前に確認する。\n\n│ ただの引用\n"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestTableAlignsWideText(t *testing.T) {
	got := plain("| 方式 | 電池 |\n|---|---|\n| Wacom EMR | 不要 |\n| AES | 必要 |", 80)
	want := " 方式      │ 電池\n───────────┼──────\n Wacom EMR │ 不要\n AES       │ 必要\n"
	if got != want {
		t.Errorf("got\n%s\nwant\n%s", got, want)
	}
}

func TestTableShrinksToWidth(t *testing.T) {
	got := plain("| a | b |\n|---|---|\n| "+strings.Repeat("長", 30)+" | x |", 30)
	for _, l := range strings.Split(strings.TrimSuffix(got, "\n"), "\n") {
		if Width(l) > 30 {
			t.Errorf("line %q is %d cells wide", l, Width(l))
		}
	}
}

func TestCodeBlockIsVerbatim(t *testing.T) {
	got := plain("```go\nx := **not bold**\n```\n後", 80)
	want := "▏ go\n▏ x := **not bold**\n後\n"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestColorWrapsStyles(t *testing.T) {
	got := Markdown("**b**", Options{Width: 80, Color: true})
	if got != "\x1b[1mb\x1b[0m\n" {
		t.Errorf("got %q", got)
	}
}

func TestLongCodeLineIsCutToWidth(t *testing.T) {
	got := plain("```\n"+strings.Repeat("x", 100)+"\n"+strings.Repeat("y", 18)+"\n```", 20)
	want := "▏ " + strings.Repeat("x", 17) + "…\n▏ " + strings.Repeat("y", 18) + "\n"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}

func TestTooWideTableBecomesRecords(t *testing.T) {
	v := "長長長長長"
	md := "| a | b | c | d | e |\n|---|---|---|---|---|\n| " + strings.Repeat(v+" | ", 4) + v + " |\n| 1 | 2 | 3 | 4 | 5 |"
	got := plain(md, 20)
	want := "a: " + v + "\nb: " + v + "\nc: " + v + "\nd: " + v + "\ne: " + v + "\n" +
		strings.Repeat("─", 20) + "\na: 1\nb: 2\nc: 3\nd: 4\ne: 5\n"
	if got != want {
		t.Errorf("got  %q\nwant %q", got, want)
	}
}
