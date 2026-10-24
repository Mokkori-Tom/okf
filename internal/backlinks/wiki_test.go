package backlinks

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/okfcli/okf/internal/bundle"
	"github.com/okfcli/okf/internal/validate"
)

func load(t *testing.T, files map[string]string) *bundle.Bundle {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := bundle.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	return b
}

func sorted(xs []string) []string { sort.Strings(xs); return xs }

func TestBacklinks_Wikilinks(t *testing.T) {
	b := load(t, map[string]string{
		"target.md":       "# Target\n",
		"plain.md":        "see [[target]]\n",
		"alias.md":        "see [[target|別名]] and [[target#見出し]]\n",
		"embed.md":        "![[target]]\n",
		"withmd.md":       "[[target.md]]\n",
		"folder/deep.md":  "by name from a folder: [[Target]]\n",
		"md.md":           "classic [t](target.md)\n",
		"code.md":         "`[[target]]`\n\n```\n[[target]]\n```\n",
		"image.md":        "![[target.png]]\n",
		"other.md":        "[[nothing-here]]\n",
		"sub/target.md":   "# Sub target\n",
		"sub/sibling.md":  "[[target]] prefers its own folder\n",
		"sub/explicit.md": "[[/target]] [[sub/target]]\n",
	})
	got := sorted(Backlinks(b, "target"))
	want := []string{"alias", "embed", "folder/deep", "md", "plain", "sub/explicit", "withmd"}
	if len(got) != len(want) {
		t.Fatalf("backlinks(target) = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("backlinks(target) = %v, want %v", got, want)
		}
	}
	if got := sorted(Backlinks(b, "sub/target")); len(got) != 2 || got[0] != "sub/explicit" || got[1] != "sub/sibling" {
		t.Errorf("backlinks(sub/target) = %v", got)
	}
}

func TestResolveIn_ShortestPathWins(t *testing.T) {
	b := load(t, map[string]string{
		"a/b/c/x.md": "", "z/x.md": "", "from/here.md": "",
	})
	got := validate.ResolveIn(b, "from/here", validate.Link{Target: "x", Wiki: true})
	if got != "z/x" {
		t.Errorf("got %q, want z/x", got)
	}
	if got := validate.ResolveIn(b, "from/here", validate.Link{Target: "b/c/x", Wiki: true}); got != "a/b/c/x" {
		t.Errorf("partial path: got %q", got)
	}
}
