package main

// Human-readable output. okf stays JSON-first: JSON is what agents, pipes and
// scripts get. Only when stdout is a terminal (or --human / OKF_OUTPUT=human
// is given) do the everyday commands print plain text instead. --json or
// OKF_OUTPUT=json forces JSON on a terminal too.

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/okfcli/okf/internal/concept"
	"github.com/okfcli/okf/internal/render"
	"github.com/okfcli/okf/internal/search"
	"github.com/okfcli/okf/internal/validate"
)

var (
	humanOut bool // print plain text instead of JSON
	useColor bool // decorate plain text with ANSI colors
)

// setOutputMode decides between JSON and plain text and strips the global
// --json / --human flags from args so commands never see them.
func setOutputMode(args []string) []string {
	tty := isTerminal(os.Stdout)
	humanOut = tty
	switch strings.ToLower(os.Getenv("OKF_OUTPUT")) {
	case "json":
		humanOut = false
	case "human", "text":
		humanOut = true
	}
	kept := args[:0:0]
	for _, a := range args {
		switch a {
		case "--json":
			humanOut = false
		case "--human":
			humanOut = true
		default:
			kept = append(kept, a)
		}
	}
	useColor = humanOut && tty && os.Getenv("NO_COLOR") == ""
	return kept
}

func isTerminal(f *os.File) bool {
	fi, err := f.Stat()
	return err == nil && fi.Mode()&os.ModeCharDevice != 0
}

// --- styling ---

func style(code, s string) string {
	if !useColor || s == "" {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

func bold(s string) string { return style("1", s) }
func dim(s string) string  { return style("2", s) }

// displayPathIn shows p relative to the working directory when it lies
// inside it, else relative to the bundle root, else as is.
func displayPathIn(p, root string) string {
	if d := displayPath(p); d != p {
		return d
	}
	if rel, err := filepath.Rel(root, p); err == nil && !strings.HasPrefix(rel, "..") {
		return rel
	}
	return p
}

// displayPath shows p relative to the working directory when that is shorter.
func displayPath(p string) string {
	wd, err := os.Getwd()
	if err != nil {
		return p
	}
	rel, err := filepath.Rel(wd, p)
	if err != nil || strings.HasPrefix(rel, "..") {
		return p
	}
	return rel
}

func width(s string) int { return render.Width(s) }

func pad(s string, w int) string {
	if d := w - width(s); d > 0 {
		return s + strings.Repeat(" ", d)
	}
	return s
}

func idColumn(ids []string) int {
	w := 0
	for _, id := range ids {
		if n := width(id); n > w {
			w = n
		}
	}
	return min(w, 40)
}

// highlight marks every occurrence of the folded terms in line. Fold maps rune
// for rune, so positions found in the folded line apply to the original.
// Lines wider than maxCells terminal cells are cut around the first match.
func highlight(line string, terms []string, maxCells int) string {
	orig := []rune(line)
	folded := []rune(search.Fold(line))
	mark := make([]bool, len(orig))
	first := -1
	for _, t := range terms {
		tr := []rune(t)
		for i := 0; i+len(tr) <= len(folded); i++ {
			if string(folded[i:i+len(tr)]) == t {
				for j := i; j < i+len(tr); j++ {
					mark[j] = true
				}
				if first < 0 || i < first {
					first = i
				}
			}
		}
	}
	start, end := 0, len(orig)
	if maxCells > 0 && width(line) > maxCells {
		// Keep about a third of the budget as context before the match.
		if first > 0 {
			start = first
			for start > 0 && width(string(orig[start-1:first])) <= maxCells/3 {
				start--
			}
		}
		used := 0
		for end = start; end < len(orig); end++ {
			if used += width(string(orig[end])); used > maxCells {
				break
			}
		}
	}
	var b strings.Builder
	if start > 0 {
		b.WriteString("…")
	}
	for i := start; i < end; {
		j := i
		for j < end && mark[j] == mark[i] {
			j++
		}
		seg := string(orig[i:j])
		if mark[i] {
			seg = style("1;33", seg)
		}
		b.WriteString(seg)
		i = j
	}
	if end < len(orig) {
		b.WriteString("…")
	}
	return b.String()
}

// --- renderers ---

func printConcepts(concepts []*concept.Concept) {
	ids := make([]string, len(concepts))
	for i, c := range concepts {
		ids[i] = c.ID
	}
	w := idColumn(ids)
	for _, c := range concepts {
		typ := c.Frontmatter.Type
		if c.FrontmatterErr != nil {
			typ = "no frontmatter"
		}
		fmt.Printf("%s  %s %s\n", bold(pad(c.ID, w)), c.DisplayTitle(), dim("["+typ+"]"))
	}
	fmt.Fprintln(os.Stderr, dim(fmt.Sprintf("%d found", len(concepts))))
}

const maxHumanLines = 3

func printHits(hits []search.Hit, terms []string) {
	for i, h := range hits {
		if i > 0 {
			fmt.Println()
		}
		c := h.Concept
		fmt.Printf("%s  %s\n", bold(c.ID), highlight(c.DisplayTitle(), terms, 0))
		for k, l := range h.Lines {
			if k == maxHumanLines {
				fmt.Println(dim(fmt.Sprintf("  … +%d lines", len(h.Lines)-k)))
				break
			}
			fmt.Printf("  %s %s\n", dim(fmt.Sprintf("%4d:", l.Number)), highlight(strings.TrimSpace(l.Text), terms, 100))
		}
	}
	fmt.Fprintln(os.Stderr, dim(fmt.Sprintf("%d found", len(hits))))
}

// printConcept shows one page. Unless raw, the body is rendered as markdown
// (headings, lists, callouts, tables, inline styles) for the terminal width,
// and on a terminal long output goes through a pager.
func printConcept(c *concept.Concept, root string, raw, noPager bool) {
	fm := c.Frontmatter
	o := render.Options{Width: termWidth(), Color: useColor}
	var b strings.Builder

	meta := []string{displayPathIn(c.Path, root)}
	if errors.Is(c.FrontmatterErr, concept.ErrNoFrontmatter) {
		meta = append(meta, "no frontmatter")
	} else if c.FrontmatterErr != nil {
		meta = append(meta, "invalid frontmatter")
	} else {
		meta = append(meta, "type: "+fm.Type, "status: "+fm.EffectiveStatus())
	}
	if len(fm.Tags) > 0 {
		meta = append(meta, "tags: "+strings.Join(fm.Tags, ", "))
	}

	body := strings.TrimRight(c.Body, "\n")
	if raw {
		b.WriteString(bold(c.DisplayTitle()) + "\n" + dim(strings.Join(meta, "  ")) + "\n")
		if fm.Description != "" {
			b.WriteString(fm.Description + "\n")
		}
		b.WriteString("\n" + body + "\n")
	} else {
		body = dropTitleHeading(body, c.DisplayTitle())
		b.WriteString(render.Heading(c.DisplayTitle(), 1, o))
		b.WriteString(render.Text(strings.Join(meta, "  "), "2", o))
		if fm.Description != "" {
			b.WriteString(render.Markdown("*"+fm.Description+"*", o))
		}
		if strings.TrimSpace(body) != "" {
			b.WriteString("\n" + render.Markdown(body, o))
		}
	}
	page(b.String(), noPager)
}

// dropTitleHeading removes a leading "# title" line that repeats the title
// already shown in the header.
func dropTitleHeading(body, title string) string {
	trimmed := strings.TrimLeft(body, "\n")
	first, rest, _ := strings.Cut(trimmed, "\n")
	if h, ok := strings.CutPrefix(strings.TrimSpace(first), "# "); ok && strings.TrimSpace(h) == title {
		return rest
	}
	return body
}

// termWidth is the width to render for: $COLUMNS, else the terminal's, else
// 80, capped at 120 so long lines stay readable.
func termWidth() int {
	w := 0
	if n, err := strconv.Atoi(os.Getenv("COLUMNS")); err == nil && n > 0 {
		w = n
	} else {
		w = ttyWidth(os.Stdout)
	}
	if w <= 0 {
		w = 80
	}
	return min(w, 120)
}

// page writes out, through $OKF_PAGER or $PAGER (default less) when stdout
// is a terminal. less gets LESS=FRX unless LESS is set, so short pages print
// directly and colors survive. Falls back to plain output when no pager runs.
func page(out string, noPager bool) {
	if noPager || !isTerminal(os.Stdout) {
		fmt.Print(out)
		return
	}
	pager := os.Getenv("OKF_PAGER")
	if pager == "" {
		pager = os.Getenv("PAGER")
	}
	if pager == "" {
		pager = "less"
	}
	name := strings.Fields(pager)
	if len(name) == 0 || name[0] == "cat" {
		fmt.Print(out)
		return
	}
	if _, err := exec.LookPath(name[0]); err != nil {
		fmt.Print(out)
		return
	}
	cmd := exec.Command("sh", "-c", pager) //nolint:gosec // the user's own pager setting
	cmd.Stdin = strings.NewReader(out)
	cmd.Stdout, cmd.Stderr = os.Stdout, os.Stderr
	cmd.Env = os.Environ()
	if os.Getenv("LESS") == "" {
		cmd.Env = append(cmd.Env, "LESS=FRX")
	}
	if err := cmd.Run(); err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			fmt.Print(out)
		}
	}
}

func printFindings(findings []validate.Finding, errs, warns int, strict bool) {
	for _, f := range findings {
		sev := f.Severity.String()
		if f.Severity == validate.SeverityError {
			sev = style("31", sev)
		} else {
			sev = style("33", sev)
		}
		id := f.ConceptID
		if id == "" {
			id = "(bundle)"
		}
		fmt.Printf("%s  %s  %s %s\n", sev, bold(id), f.Message, dim("("+f.RuleID+")"))
	}
	switch {
	case strict && errs == 0 && warns == 0:
		fmt.Println("valid")
	case strict:
		fmt.Printf("%d errors, %d warnings\n", errs, warns)
	default:
		fmt.Printf("%d warnings\n", len(findings))
	}
}

func printIDs(ids []string) {
	for _, id := range ids {
		fmt.Println(id)
	}
	fmt.Fprintln(os.Stderr, dim(fmt.Sprintf("%d found", len(ids))))
}
