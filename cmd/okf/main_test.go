package main

import (
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/okfcli/okf/internal/cerr"
	"github.com/okfcli/okf/internal/concept"
)

// --- issue #31: commands without flags must still reject flags as usage ---

func TestRejectFlags(t *testing.T) {
	if err := rejectFlags("index", []string{"./bundle"}); err != nil {
		t.Fatalf("plain path rejected: %v", err)
	}
	if err := rejectFlags("index", nil); err != nil {
		t.Fatalf("empty args rejected: %v", err)
	}
	for _, args := range [][]string{
		{"--nope", "./bundle"},
		{"./bundle", "--check"},
		{"-x"},
	} {
		err := rejectFlags("index", args)
		if err == nil {
			t.Fatalf("args %v: want usage error", args)
		}
		if err.Kind != cerr.KindUsage {
			t.Errorf("args %v: kind = %v, want usage", args, err.Kind)
		}
		if !strings.Contains(err.Message, "unknown index flag: -") {
			t.Errorf("args %v: message = %q", args, err.Message)
		}
	}
}

// --- issue #27: a non-concept .md is a validation error naming the file ---

func TestLoadBundle_PlainMarkdownLoads(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.md"), "# Demo\n")
	writeFile(t, filepath.Join(dir, "README.md"), "# Demo project\n")

	b, err := loadBundle(dir)
	if err != nil {
		t.Fatalf("plain markdown should load leniently: %v", err)
	}
	c := b.Get("README")
	if c == nil || !errors.Is(c.FrontmatterErr, concept.ErrNoFrontmatter) || c.DisplayTitle() != "Demo project" {
		t.Errorf("README concept = %+v", c)
	}
}

func TestE2E_OneBadFileDoesNotStopTheBundle(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.md"), "---\nokf_version: [\n---\n# Demo\n")
	writeFile(t, filepath.Join(dir, "a.md"), "---\ntype: T\ntitle: A\n---\n\nbody")
	writeFile(t, filepath.Join(dir, "secret.md"), "---\ntype: T\n---\n\nbody")
	if err := os.Chmod(filepath.Join(dir, "secret.md"), 0); err != nil {
		t.Fatal(err)
	}
	if f, err := os.Open(filepath.Join(dir, "secret.md")); err == nil {
		f.Close()
		t.Skip("running with permissions that can read a mode-000 file")
	}

	out, code := runOKF(t, "list", dir)
	if code != 0 || !strings.Contains(out, `"count": 1`) {
		t.Errorf("list: exit %d; out=%s", code, out)
	}
	out, code = runOKF(t, "validate", dir)
	if code != cerr.ExitCodeValidation {
		t.Errorf("validate: exit %d, want %d; out=%s", code, cerr.ExitCodeValidation, out)
	}
	for _, want := range []string{`"rule": "okf/bundle/unreadable"`, `"concept_id": "secret"`, `"rule": "okf/frontmatter/invalid"`, `"concept_id": "index"`} {
		if !strings.Contains(out, want) {
			t.Errorf("validate output lacks %s: %s", want, out)
		}
	}
}

func TestE2E_SymlinkedRoot(t *testing.T) {
	real := t.TempDir()
	writeFile(t, filepath.Join(real, "notes", "a.md"), "---\ntype: T\ntitle: 電力\n---\n\nbody")
	link := filepath.Join(t.TempDir(), "kb")
	if err := os.Symlink(real, link); err != nil {
		t.Skip("symlinks unsupported:", err)
	}
	out, code := runOKF(t, "list", link)
	if code != 0 || !strings.Contains(out, `"count": 1`) {
		t.Fatalf("list via symlink: exit %d; out=%s", code, out)
	}
	if out, _ := runOKF(t, "show", link, "notes/a"); !strings.Contains(out, `"path": "`+filepath.Join(link, "notes", "a.md")+`"`) {
		t.Errorf("show path is not under the symlinked root: %s", out)
	}
	if _, code := runOKF(t, "validate", link); code != 0 {
		t.Errorf("validate via symlink: exit %d", code)
	}
	if out, code := runOKF(t, "index", link); code != 0 || !strings.Contains(out, `"count": 2`) {
		t.Errorf("index via symlink: exit %d; out=%s", code, out)
	}
}

func TestE2E_EmptyBundleFailsValidate(t *testing.T) {
	dir := t.TempDir()
	out, code := runOKF(t, "validate", dir)
	if code != cerr.ExitCodeValidation || !strings.Contains(out, `"rule": "okf/bundle/empty"`) {
		t.Errorf("validate empty: exit %d; out=%s", code, out)
	}
}

func TestLoadBundle_MissingDirIsIOError(t *testing.T) {
	_, err := loadBundle(filepath.Join(t.TempDir(), "nope"))
	if err == nil || err.Kind != cerr.KindIO {
		t.Fatalf("want io error, got %v", err)
	}
}

// --- end to end: the built binary's JSON envelope and exit code ---

var okfBin string

func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "okf-test-bin")
	if err != nil {
		panic(err)
	}
	okfBin = filepath.Join(dir, "okf")
	if runtime.GOOS == "windows" {
		okfBin += ".exe"
	}
	build := exec.Command("go", "build", "-o", okfBin, ".")
	if out, err := build.CombinedOutput(); err != nil {
		panic("build okf: " + err.Error() + "\n" + string(out))
	}
	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type envelope struct {
	Error struct {
		Kind    string `json:"kind"`
		Code    int    `json:"code"`
		Message string `json:"message"`
		Hint    string `json:"hint"`
	} `json:"error"`
}

func runOKF(t *testing.T, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(okfBin, args...) //nolint:gosec // test runs the binary built in TestMain
	out, err := cmd.Output()
	code := 0
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		code = exitErr.ExitCode()
	} else if err != nil {
		t.Fatalf("run %v: %v", args, err)
	}
	return string(out), code
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestE2E_UnknownFlagIsUsageError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "---\ntype: T\n---\n\nbody")
	for _, c := range []string{"index", "list", "graph", "search", "validate", "lint"} {
		out, code := runOKF(t, c, "--nope", dir)
		if code != cerr.ExitCodeUsage {
			t.Errorf("%s --nope: exit %d, want %d; out=%s", c, code, cerr.ExitCodeUsage, out)
		}
		var env envelope
		if err := json.Unmarshal([]byte(out), &env); err != nil {
			t.Errorf("%s --nope: bad json %q", c, out)
			continue
		}
		if env.Error.Kind != "usage" || env.Error.Code != 400 {
			t.Errorf("%s --nope: envelope %+v", c, env.Error)
		}
		if !strings.Contains(env.Error.Message, "--nope") {
			t.Errorf("%s --nope: message %q does not name the flag", c, env.Error.Message)
		}
	}
	for _, c := range []string{"show", "backlinks"} {
		out, code := runOKF(t, c, dir, "--nope")
		if code != cerr.ExitCodeUsage {
			t.Errorf("%s dir --nope: exit %d, want %d; out=%s", c, code, cerr.ExitCodeUsage, out)
		}
	}
	// Sanity: the bundle itself still loads.
	if _, code := runOKF(t, "list", dir); code != 0 {
		t.Errorf("list: exit %d, want 0", code)
	}
}

func TestE2E_PlainMarkdownIsAWarning(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "index.md"), "# Demo\n")
	writeFile(t, filepath.Join(dir, "README.md"), "# Demo project\n\n電力の記録\n")

	out, code := runOKF(t, "list", dir)
	if code != 0 || !strings.Contains(out, `"title": "Demo project"`) {
		t.Errorf("list: exit %d; out=%s", code, out)
	}
	if out, code := runOKF(t, "search", dir, "電力"); code != 0 || !strings.Contains(out, `"count": 1`) {
		t.Errorf("search: exit %d; out=%s", code, out)
	}

	out, code = runOKF(t, "validate", dir)
	if code != 0 {
		t.Errorf("validate: exit %d, want 0 (missing frontmatter is a warning); out=%s", code, out)
	}
	if !strings.Contains(out, `"rule": "okf/frontmatter/missing"`) || !strings.Contains(out, `"concept_id": "README"`) {
		t.Errorf("validate does not report README: %s", out)
	}
	// One warning, not a field-by-field list.
	if strings.Contains(out, "type-required") || strings.Contains(out, "title-recommended") {
		t.Errorf("field checks ran on a plain note: %s", out)
	}
}

func TestE2E_BrokenFrontmatterIsAnError(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "---\ntype: T\n\nbody without closing delimiter\n")
	if out, code := runOKF(t, "search", dir, "closing"); code != 0 || !strings.Contains(out, `"count": 1`) {
		t.Errorf("search: exit %d; out=%s", code, out)
	}
	out, code := runOKF(t, "validate", dir)
	if code != cerr.ExitCodeValidation || !strings.Contains(out, `"rule": "okf/frontmatter/invalid"`) {
		t.Errorf("validate: exit %d; out=%s", code, out)
	}
}

func TestE2E_BrokenLinkDoesNotFailValidate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "---\ntype: T\ntitle: A\ndescription: d\ntags: [x]\n---\n\nSee [b](/b.md).")
	out, code := runOKF(t, "validate", dir)
	if code != 0 {
		t.Fatalf("exit %d, want 0; out=%s", code, out)
	}
	var rep struct {
		Valid    bool `json:"valid"`
		Warnings int  `json:"warnings"`
		Errors   int  `json:"errors"`
	}
	if err := json.Unmarshal([]byte(out), &rep); err != nil {
		t.Fatalf("bad json %q", out)
	}
	if !rep.Valid || rep.Errors != 0 || rep.Warnings != 1 {
		t.Errorf("report = %+v, want valid with one warning", rep)
	}
}

func TestE2E_IndexListsPlainMarkdown(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "a.md"), "---\ntype: T\ntitle: A\n---\n\nbody")
	writeFile(t, filepath.Join(dir, "README.md"), "# Demo project\n")

	if out, code := runOKF(t, "index", dir); code != 0 {
		t.Fatalf("index: exit %d; out=%s", code, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "index.md"))
	if err != nil || !strings.Contains(string(raw), "Demo project") {
		t.Errorf("index.md does not list README by its heading: %q %v", raw, err)
	}
}
