package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestExpandTemplate(t *testing.T) {
	now := time.Date(2026, 10, 26, 6, 8, 9, 0, time.Local)
	cases := []struct{ in, title, want string }{
		{"{{title}} {{date}} {{time}}", "メモ", "メモ 2026-10-26 06:08"},
		{"{{date:YYYY年M月D日 (ddd)}} {{time:HH:mm:ss}}", "x", "2026年10月26日 (Mon) 06:08:09"},
		{"{{ date+1w:YYYY-MM-DD }} {{date-1M:YYYY-MM}} {{date+1y:YY}}", "x", "2026-11-02 2026-09 27"},
		{"{{date:[Week] YYYY dddd}}", "x", "Week 2026 Monday"},
		// A dated title moves the base date; offsets cross month and year ends.
		{"[[{{date-1d:YYYY-MM-DD}}]] [[{{date+1d:YYYY-MM-DD}}]]", "2026-03-01", "[[2026-02-28]] [[2026-03-02]]"},
		{"{{date+1d}}", "2026-12-31", "2027-01-01"},
		// Not a real date: falls back to now.
		{"{{date}}", "2026-02-31", "2026-10-26"},
		{"{{unknown}} {{date", "x", "{{unknown}} {{date"},
	}
	for _, c := range cases {
		if got := expandTemplate(c.in, c.title, now); got != c.want {
			t.Errorf("expand(%q, %q) = %q, want %q", c.in, c.title, got, c.want)
		}
	}
}

func TestE2E_NewFromTemplate(t *testing.T) {
	dir := t.TempDir()
	writeFile(t, filepath.Join(dir, "Templates", "log.md"), "---\ntype: Log\ntags: [記録]\ncreated: \"{{date}}\"\n---\n\n# {{title}}\n\n前日: [[{{date-1d}}]]\n")
	cmd := exec.Command(okfBin, "new", dir, "2026-03-01", "--template", "log", "--tag", "上書き", "--body", "-") //nolint:gosec // test binary
	cmd.Stdin = strings.NewReader("---\nmachine: ogma\n---\n\n追記\n")
	if out, err := cmd.Output(); err != nil {
		t.Fatalf("new --template: %v; %s", err, out)
	}
	raw, err := os.ReadFile(filepath.Join(dir, "2026-03-01.md"))
	if err != nil {
		t.Fatal(err)
	}
	s := string(raw)
	for _, want := range []string{"type: Log\n", "  - 上書き\n", `created: "2026-03-01"` + "\n", "machine: ogma\n", "---\n\n# 2026-03-01\n\n前日: [[2026-02-28]]\n\n追記\n"} {
		if !strings.Contains(s, want) {
			t.Errorf("lacks %q:\n%s", want, s)
		}
	}
	if strings.Contains(s, "記録") {
		t.Errorf("--tag should win over the template's tags:\n%s", s)
	}
	if out, code := runOKFIn(t, dir, nil, "new", dir, "y", "--template", "nope"); code == 0 || !strings.Contains(out, "not found") {
		t.Errorf("missing template: exit %d; out=%s", code, out)
	}
	if out, _ := runOKFIn(t, dir, nil, "__complete", "templates", dir); out != "log\n" {
		t.Errorf("template completion: %q", out)
	}
}

func TestE2E_DevNullIsNotATerminal(t *testing.T) {
	dir := t.TempDir()
	marker := filepath.Join(dir, "editor-ran")
	editor := filepath.Join(dir, "ed.sh")
	writeFile(t, editor, "#!/bin/sh\ntouch "+marker+"\n")
	if err := os.Chmod(editor, 0o755); err != nil {
		t.Fatal(err)
	}
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatal(err)
	}
	defer devnull.Close()
	cmd := exec.Command(okfBin, "new", dir, "page") //nolint:gosec // test binary
	cmd.Stdin, cmd.Stdout = devnull, devnull
	cmd.Env = append(os.Environ(), "EDITOR="+editor)
	if err := cmd.Run(); err != nil {
		t.Fatalf("new: %v", err)
	}
	if _, err := os.Stat(marker); err == nil {
		t.Error("the editor ran with stdin and stdout on /dev/null")
	}
}
