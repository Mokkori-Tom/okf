package search

import "testing"

func TestFold(t *testing.T) {
	cases := map[string]string{
		"ファン":     "ふぁん",
		"ＧＰＵ設定":   "gpu設定",
		"ヴァ　ケ":    "ゔぁ け",
		"Revenue": "revenue",
	}
	for in, want := range cases {
		if got := Fold(in); got != want {
			t.Errorf("Fold(%q) = %q, want %q", in, got, want)
		}
	}
}

var jaFiles = map[string]string{
	"gpu.md":  "---\ntype: Note\ntitle: GPUのファン調整\n---\n\n# GPUのファン調整\n\n電力上限は150Wに設定した。\nファンカーブを適用した。\n",
	"memo.md": "---\ntype: Note\ntitle: 買い物メモ\ntags: [生活]\n---\n\n牛乳、ファンヒーターの灯油\n",
	"log2.md": "---\ntype: Note\ntitle: 作業記録\n---\n\nなし\n",
}

func TestSearch_JapaneseSubstring(t *testing.T) {
	b := testBundle(t, jaFiles)
	if got := resultIDs(Search(b, Filters{Terms: []string{"電力上限"}})); len(got) != 1 || got[0] != "gpu" {
		t.Errorf("電力上限: got %v", got)
	}
	// Hiragana finds katakana.
	if got := Search(b, Filters{Terms: []string{"ふぁん"}}); len(got) != 2 {
		t.Errorf("ふぁん: got %d results, want 2", len(got))
	}
}

func TestSearch_TermsAreANDed(t *testing.T) {
	b := testBundle(t, jaFiles)
	got := resultIDs(Search(b, Filters{Terms: []string{"ファン", "灯油"}}))
	if len(got) != 1 || got[0] != "memo" {
		t.Errorf("got %v, want [memo]", got)
	}
	if got := Search(b, Filters{Text: "ファン", Terms: []string{"存在しない"}}); len(got) != 0 {
		t.Errorf("got %d results, want 0", len(got))
	}
}

func TestFind_LinesAndRanking(t *testing.T) {
	b := testBundle(t, jaFiles)
	hits := Find(b, Filters{Terms: []string{"ファン"}})
	if len(hits) != 2 {
		t.Fatalf("got %d hits, want 2", len(hits))
	}
	// gpu has the term in its title, so it ranks first despite the ID order.
	if hits[0].Concept.ID != "gpu" {
		t.Errorf("first hit %s, want gpu", hits[0].Concept.ID)
	}
	// File line numbers: frontmatter is lines 1-4, "# GPUのファン調整" is line 6.
	want := []Line{{6, "# GPUのファン調整"}, {9, "ファンカーブを適用した。"}}
	if got := hits[0].Lines; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Errorf("lines = %+v, want %+v", got, want)
	}
}

func TestFind_NoTermsKeepsOrderAndNoLines(t *testing.T) {
	b := testBundle(t, jaFiles)
	hits := Find(b, Filters{Tag: "生活"})
	if len(hits) != 1 || hits[0].Concept.ID != "memo" || hits[0].Lines != nil {
		t.Errorf("got %+v", hits)
	}
}
