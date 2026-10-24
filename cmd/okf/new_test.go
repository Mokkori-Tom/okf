package main

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/okfcli/okf/internal/cerr"
)

func runOKFIn(t *testing.T, dir string, env []string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(okfBin, args...) //nolint:gosec // test runs the binary built in TestMain
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	out, err := cmd.Output()
	if ee, ok := err.(*exec.ExitError); ok {
		return string(out), ee.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return string(out), 0
}

func TestE2E_NewCreatesValidConcept(t *testing.T) {
	dir := t.TempDir()
	out, code := runOKFIn(t, dir, []string{"OKF_AUTHOR=human:tester"},
		"new", ".", "notes/電力", "--title", "電力: 上限の記録", "--tag", "GPU, 冷却")
	if code != 0 {
		t.Fatalf("new: exit %d; out=%s", code, out)
	}
	var res struct {
		Path  string `json:"path"`
		ID    string `json:"id"`
		Title string `json:"title"`
	}
	if err := json.Unmarshal([]byte(out), &res); err != nil {
		t.Fatalf("bad json %q", out)
	}
	if res.ID != "notes/電力" || res.Title != "電力: 上限の記録" {
		t.Errorf("got %+v", res)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "notes", "電力.md"))
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"type: Note\n", "- GPU\n", "- 冷却\n", "by: human:tester\n", "\n# 電力: 上限の記録\n"} {
		if !strings.Contains(string(raw), want) {
			t.Errorf("file lacks %q:\n%s", want, raw)
		}
	}

	out, code = runOKFIn(t, dir, nil, "validate", ".")
	if code != 0 || !strings.Contains(out, `"errors": 0`) {
		t.Errorf("validate: exit %d; out=%s", code, out)
	}

	// Never overwrite.
	if _, code := runOKFIn(t, dir, nil, "new", "notes/電力"); code != cerr.ExitCodeValidation {
		t.Errorf("second new: exit %d, want %d", code, cerr.ExitCodeValidation)
	}
}

func TestE2E_NewRejectsEscapingAndReservedIDs(t *testing.T) {
	dir := t.TempDir()
	for _, id := range []string{"../x", "/abs", "a/../b", "index", "sub/log"} {
		if _, code := runOKFIn(t, dir, nil, "new", ".", id); code != cerr.ExitCodeUsage {
			t.Errorf("new %q: exit %d, want %d", id, code, cerr.ExitCodeUsage)
		}
	}
}

func TestE2E_SearchTermsAndDefaultBundle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gpu.md"), "---\ntype: Note\ntitle: GPU\n---\n\n電力上限は150W\n")
	writeFile(t, filepath.Join(dir, "memo.md"), "---\ntype: Note\ntitle: メモ\n---\n\n牛乳\n")
	other := t.TempDir()

	check := func(name, out string, code int) {
		t.Helper()
		if code != 0 {
			t.Fatalf("%s: exit %d; out=%s", name, code, out)
		}
		var res struct {
			Count   int `json:"count"`
			Results []struct {
				ID      string `json:"id"`
				Matches []struct {
					Line int    `json:"line"`
					Text string `json:"text"`
				} `json:"matches"`
			} `json:"results"`
		}
		if err := json.Unmarshal([]byte(out), &res); err != nil {
			t.Fatalf("%s: bad json %q", name, out)
		}
		if res.Count != 1 || res.Results[0].ID != "gpu" || len(res.Results[0].Matches) != 1 || res.Results[0].Matches[0].Line != 6 {
			t.Errorf("%s: got %+v", name, res)
		}
	}
	out, code := runOKFIn(t, other, nil, "search", dir, "電力")
	check("explicit bundle", out, code)
	out, code = runOKFIn(t, dir, nil, "search", "上限")
	check("cwd bundle", out, code)
	out, code = runOKFIn(t, other, []string{"OKF_BUNDLE=" + dir}, "search", "150w")
	check("OKF_BUNDLE", out, code)

	// A path-looking first argument that does not exist is an I/O error,
	// not a search term.
	if _, code := runOKFIn(t, other, nil, "search", "./missing", "電力"); code != cerr.ExitCodeIO {
		t.Errorf("missing bundle: exit %d, want %d", code, cerr.ExitCodeIO)
	}
}

func TestE2E_HumanOutput(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "gpu.md"), "---\ntype: Note\ntitle: GPUのファン\n---\n\nファンカーブを適用\n")
	out, code := runOKFIn(t, dir, nil, "search", "--human", "ふぁん")
	if code != 0 || !strings.Contains(out, "gpu  GPUのファン\n") || !strings.Contains(out, "   6: ファンカーブを適用\n") {
		t.Errorf("human search: exit %d; out=%q", code, out)
	}
	out, _ = runOKFIn(t, dir, []string{"OKF_OUTPUT=human"}, "list")
	if !strings.HasPrefix(out, "gpu  GPUのファン [Note]\n") {
		t.Errorf("human list: %q", out)
	}
	// Human errors go to stderr only; stdout stays empty.
	out, code = runOKFIn(t, dir, nil, "show", "--human", "nope")
	if code == 0 || out != "" {
		t.Errorf("human error: exit %d; stdout=%q", code, out)
	}
	// --json wins over OKF_OUTPUT.
	out, _ = runOKFIn(t, dir, []string{"OKF_OUTPUT=human"}, "list", "--json")
	if !strings.HasPrefix(out, "{") {
		t.Errorf("--json: %q", out)
	}
}

func TestE2E_ShowByFilePath(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "notes", "gpu.md"), "---\ntype: Note\ntitle: GPUのファン\n---\n\nファンカーブを適用\n")
	// Reading one file by path does not load the rest of the bundle.
	writeFile(t, filepath.Join(dir, "plain.md"), "# ただのメモ\n\n本文\n")
	other := t.TempDir()

	var res struct {
		Concept struct {
			ID    string `json:"id"`
			Title string `json:"title"`
			Body  string `json:"body"`
		} `json:"concept"`
	}
	out, code := runOKFIn(t, other, nil, "show", filepath.Join(dir, "notes", "gpu.md"))
	if code != 0 || json.Unmarshal([]byte(out), &res) != nil || res.Concept.Title != "GPUのファン" {
		t.Fatalf("show outside bundle: exit %d; out=%s", code, out)
	}
	if res.Concept.ID != "gpu" {
		t.Errorf("id outside bundle = %q, want gpu", res.Concept.ID)
	}
	out, code = runOKFIn(t, dir, nil, "show", "notes/gpu.md")
	if code != 0 || json.Unmarshal([]byte(out), &res) != nil || res.Concept.ID != "notes/gpu" {
		t.Errorf("show inside bundle: exit %d; out=%s", code, out)
	}
	// Rendered: the title heading is drawn once, as the header.
	out, code = runOKFIn(t, dir, nil, "show", "--human", "plain.md")
	if code != 0 || out != "ただのメモ\n━━━━━━━━━━\nplain.md  no frontmatter\n\n本文\n" {
		t.Errorf("show plain markdown: exit %d; out=%q", code, out)
	}
	out, code = runOKFIn(t, dir, nil, "show", "--human", "--raw", "plain.md")
	if code != 0 || out != "ただのメモ\nplain.md  no frontmatter\n\n# ただのメモ\n\n本文\n" {
		t.Errorf("show --raw: exit %d; out=%q", code, out)
	}
	// The plain file does not stop the bundle from loading for IDs either.
	if _, code := runOKFIn(t, dir, nil, "show", "notes/gpu"); code != 0 {
		t.Errorf("show by id next to plain markdown: exit %d, want 0", code)
	}
}

func TestE2E_CompletionCandidates(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "notes", "電力.md"), "---\ntype: Note\ntitle: A\ntags: [GPU, 冷却]\n---\n\nbody")
	writeFile(t, filepath.Join(dir, "Branden Robinson.md"), "# 人物\n")
	other := t.TempDir()

	out, code := runOKFIn(t, other, []string{"OKF_BUNDLE=" + dir}, "__complete", "ids")
	if code != 0 || out != "Branden Robinson\nnotes/電力\n" {
		t.Errorf("ids: exit %d; out=%q", code, out)
	}
	out, _ = runOKFIn(t, other, nil, "__complete", "tags", dir)
	if out != "GPU\n冷却\n" {
		t.Errorf("tags with explicit bundle: %q", out)
	}
	// A missing bundle yields nothing, quietly.
	if out, code := runOKFIn(t, other, []string{"OKF_BUNDLE=/nonexistent"}, "__complete", "ids"); code != 0 || out != "" {
		t.Errorf("missing bundle: exit %d; out=%q", code, out)
	}

	for _, sh := range []string{"zsh", "bash"} {
		script, code := runOKFIn(t, other, nil, "completion", sh)
		if code != 0 || !strings.Contains(script, "okf __complete") {
			t.Errorf("completion %s: exit %d", sh, code)
			continue
		}
		if _, err := exec.LookPath(sh); err != nil {
			continue
		}
		check := exec.Command(sh, "-n") //nolint:gosec // syntax check of our own script
		check.Stdin = strings.NewReader(script)
		if msg, err := check.CombinedOutput(); err != nil {
			t.Errorf("%s -n: %v\n%s", sh, err, msg)
		}
	}
	if _, code := runOKFIn(t, other, nil, "completion", "fish"); code != cerr.ExitCodeUsage {
		t.Errorf("completion fish: exit %d, want usage error", code)
	}
}

func TestE2E_NewWithBodyInOneCall(t *testing.T) {
	dir := t.TempDir()
	cmd := exec.Command(okfBin, "new", dir, "物理/Kapitsa", "--type", "Person", "--title", "カピッツァ",
		"--description", "ソ連の物理学者", "--tag", "物理,人物", "--status", "draft", "--body", "-") //nolint:gosec // test binary
	cmd.Stdin = strings.NewReader("# カピッツァ\n\n超流動を発見した。\n\n\n")
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("new --body -: %v; out=%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "物理", "Kapitsa.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{"type: Person\n", "title: カピッツァ\n", "description: ソ連の物理学者\n", "  - 人物\n", "status: draft\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("frontmatter lacks %q:\n%s", want, s)
		}
	}
	// Body as given (one title heading, not two), with a single trailing newline.
	if !strings.HasSuffix(s, "---\n\n# カピッツァ\n\n超流動を発見した。\n") || strings.Count(s, "# カピッツァ") != 1 {
		t.Errorf("body not written as given:\n%q", s)
	}
	if out, code := runOKFIn(t, dir, nil, "validate", "."); code != 0 || !strings.Contains(out, `"errors": 0`) {
		t.Errorf("validate: exit %d; out=%s", code, out)
	}

	// --body with inline text.
	if _, code := runOKFIn(t, dir, nil, "new", "short", "--body", "一行だけ"); code != 0 {
		t.Fatalf("new --body text: exit %d", code)
	}
	if raw, _ := os.ReadFile(filepath.Join(dir, "short.md")); !strings.HasSuffix(string(raw), "---\n\n一行だけ\n") {
		t.Errorf("inline body: %q", raw)
	}
}

func TestE2E_SubcommandHelp(t *testing.T) {
	dir := t.TempDir()
	cases := []struct {
		args []string
		want string
	}{
		{[]string{"new", "-h"}, "okf new - "},
		{[]string{"new", "x", "--help"}, "okf new - "},
		{[]string{"search", "-h"}, "okf search - "},
		{[]string{"help", "validate"}, "okf validate - "},
	}
	for _, c := range cases {
		out, code := runOKFIn(t, dir, nil, c.args...)
		if code != 0 || !strings.HasPrefix(out, c.want) || !strings.Contains(out, "Usage: okf ") {
			t.Errorf("%v: exit %d; out=%q", c.args, code, out)
		}
	}
	if out, _ := runOKFIn(t, dir, nil, "new", "-h"); !strings.Contains(out, "--body string") {
		t.Errorf("new -h does not list --body: %s", out)
	}
	// -h never creates anything.
	if entries, _ := os.ReadDir(dir); len(entries) != 0 {
		t.Errorf("-h created files: %v", entries)
	}
}

func TestE2E_NewMergesBodyFrontmatter(t *testing.T) {
	dir := t.TempDir()
	body := "---\ntitle: 本文側のタイトル\ndescription: 本文側\nmachine: ogma\ncreated: 2026-10-24\nsources:\n  - resource: https://example.com/a\n    title: 資料 A\n---\n\n# 記録\n\n本文。\n"
	cmd := exec.Command(okfBin, "new", dir, "log/今日", "--type", "Log", "--description", "フラグ側", "--body", "-") //nolint:gosec // test binary
	cmd.Stdin = strings.NewReader(body)
	if out, err := cmd.Output(); err != nil {
		t.Fatalf("new: %v; out=%s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "log", "今日.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	want := "---\ntype: Log\ntitle: 本文側のタイトル\ndescription: フラグ側\ntags: []\n"
	if !strings.HasPrefix(s, want) {
		t.Errorf("merged head:\n%s\nwant prefix:\n%s", s, want)
	}
	for _, w := range []string{"machine: ogma\n", "created: 2026-10-24\n", "sources:\n  - resource: https://example.com/a\n    title: 資料 A\n", "---\n\n# 記録\n\n本文。\n"} {
		if !strings.Contains(s, w) {
			t.Errorf("lacks %q:\n%s", w, s)
		}
	}
	// Exactly one frontmatter block.
	if strings.Count(s, "\n---\n") != 1 {
		t.Errorf("more than one frontmatter block:\n%s", s)
	}
	if out, code := runOKFIn(t, dir, nil, "validate", "."); code != 0 || !strings.Contains(out, `"errors": 0`) {
		t.Errorf("validate: exit %d; out=%s", code, out)
	}

	// Broken YAML at the top of --body is reported, and nothing is written.
	if _, code := runOKFIn(t, dir, nil, "new", ".", "broken", "--body", "---\ntitle: [\n---\n本文"); code != cerr.ExitCodeValidation {
		t.Errorf("broken body frontmatter: exit %d, want %d", code, cerr.ExitCodeValidation)
	}
	if _, err := os.Stat(filepath.Join(dir, "broken.md")); err == nil {
		t.Error("broken.md was written")
	}
}
