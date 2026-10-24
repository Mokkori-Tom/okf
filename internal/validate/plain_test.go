package validate

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/okfcli/okf/internal/bundle"
)

func TestValidate_PlainNotesAreWarningsTypedConceptsAreStrict(t *testing.T) {
	dir := t.TempDir()
	files := map[string]string{
		// Obsidian-style: frontmatter without type, free-form status.
		"obsidian.md": "---\ntitle: 作業メモ\ntags: [gpu]\nstatus: 進行中\n---\n\n本文\n",
		"plain.md":    "# ただのメモ\n",
		// Has a type, so it is an OKF concept and gets the full checks.
		"okf.md": "---\ntype: Note\ntitle: OKF\nstatus: 進行中\n---\n\n本文\n",
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := bundle.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	r := Validate(b)

	got := map[string][]Finding{}
	for _, f := range r.Findings {
		got[f.ConceptID] = append(got[f.ConceptID], f)
	}
	if fs := got["obsidian"]; len(fs) != 1 || fs[0].RuleID != RuleTypeRequired || fs[0].Severity != SeverityWarning {
		t.Errorf("obsidian: %+v, want one type-required warning", fs)
	}
	if fs := got["plain"]; len(fs) != 1 || fs[0].RuleID != RuleFrontmatterMissing || fs[0].Severity != SeverityWarning {
		t.Errorf("plain: %+v, want one frontmatter-missing warning", fs)
	}
	strictStatus := false
	for _, f := range got["okf"] {
		if f.RuleID == RuleStatusInvalid && f.Severity == SeverityError {
			strictStatus = true
		}
	}
	if !strictStatus {
		t.Errorf("okf: %+v, want a status-invalid error", got["okf"])
	}
	if r.Errors != 1 {
		t.Errorf("errors = %d, want 1 (only the typed concept)", r.Errors)
	}
}

func TestValidate_WikilinksAreNotChecked(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"a.md": "---\ntype: Note\ntitle: A\ndescription: d\ntags: [x]\n---\n\n[[b]] [[まだ無いページ]] ![[pic.png]] [md](missing.md)\n",
		"b.md": "---\ntype: Note\ntitle: B\ndescription: d\ntags: [x]\n---\n\nbody\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := bundle.Load(dir)
	if err != nil {
		t.Fatal(err)
	}
	// Only the OKF markdown link is checked.
	r := Validate(b)
	if len(r.Findings) != 1 || r.Findings[0].RuleID != RuleLinkBroken || !strings.Contains(r.Findings[0].Message, "missing.md") {
		t.Errorf("findings = %+v", r.Findings)
	}
}

func TestValidate_StrictChecksAsOKF(t *testing.T) {
	dir := t.TempDir()
	for name, content := range map[string]string{
		"plain.md":    "# ただのメモ\n\n[[okf]]\n",
		"obsidian.md": "---\ntitle: 作業メモ\nstatus: 進行中\n---\n\n本文\n",
		"okf.md":      "---\ntype: Note\ntitle: OKF\ndescription: d\ntags: [x]\n---\n\n[[plain]] [[obsidian|別名]]\n",
	} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	b, err := bundle.Load(dir)
	if err != nil {
		t.Fatal(err)
	}

	lenient := Validate(b)
	if lenient.Errors != 0 {
		t.Errorf("default: %d errors, want 0: %+v", lenient.Errors, lenient.Findings)
	}

	strict := ValidateWith(b, Options{Strict: true})
	has := func(id, rule string, sev Severity) bool {
		for _, f := range strict.Findings {
			if f.ConceptID == id && f.RuleID == rule && f.Severity == sev {
				return true
			}
		}
		return false
	}
	for _, w := range []struct {
		id, rule string
		sev      Severity
	}{
		{"plain", RuleFrontmatterMissing, SeverityError},
		{"obsidian", RuleTypeRequired, SeverityError},
		{"obsidian", RuleStatusInvalid, SeverityError}, // full checks run
		{"obsidian", RuleDescriptionRecommended, SeverityWarning},
		{"okf", RuleWikilink, SeverityWarning},
		{"plain", RuleWikilink, SeverityWarning},
	} {
		if !has(w.id, w.rule, w.sev) {
			t.Errorf("strict lacks %s %s %v; findings: %+v", w.id, w.rule, w.sev, strict.Findings)
		}
	}
	for _, f := range strict.Findings {
		if f.RuleID == RuleWikilink && f.ConceptID == "okf" && !strings.HasPrefix(f.Message, "2 [[wikilinks]]") {
			t.Errorf("wikilink message: %q", f.Message)
		}
	}
}
