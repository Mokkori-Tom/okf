package main

// Human-readable output. okf stays JSON-first: JSON is what agents, pipes and
// scripts get. Only when stdout is a terminal (or --human / OKF_OUTPUT=human
// is given) do the everyday commands print plain text instead. --json or
// OKF_OUTPUT=json forces JSON on a terminal too.

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/okfcli/okf/internal/concept"
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

// width approximates the terminal width of s: East Asian wide and full-width
// runes take two cells.
func width(s string) int {
	w := 0
	for _, r := range s {
		w++
		if isWide(r) {
			w++
		}
	}
	return w
}

func isWide(r rune) bool {
	return (r >= 0x1100 && r <= 0x115F) ||
		(r >= 0x2E80 && r <= 0xA4CF && r != 0x303F) ||
		(r >= 0xAC00 && r <= 0xD7A3) ||
		(r >= 0xF900 && r <= 0xFAFF) ||
		(r >= 0xFE30 && r <= 0xFE4F) ||
		(r >= 0xFF00 && r <= 0xFF60) ||
		(r >= 0xFFE0 && r <= 0xFFE6) ||
		(r >= 0x1F300 && r <= 0x1FAFF) ||
		(r >= 0x20000 && r <= 0x3FFFD)
}

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
		fmt.Printf("%s  %s %s\n", bold(pad(c.ID, w)), c.Frontmatter.Title, dim("["+c.Frontmatter.Type+"]"))
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
		fmt.Printf("%s  %s\n", bold(c.ID), highlight(c.Frontmatter.Title, terms, 0))
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

func printConcept(c *concept.Concept) {
	fm := c.Frontmatter
	title := fm.Title
	if title == "" {
		title = strings.TrimSuffix(filepath.Base(c.Path), filepath.Ext(c.Path))
	}
	fmt.Println(bold(title))
	meta := []string{displayPath(c.Path)}
	if fm.Type != "" {
		meta = append(meta, "type: "+fm.Type, "status: "+fm.EffectiveStatus())
	} else {
		meta = append(meta, "no frontmatter")
	}
	if len(fm.Tags) > 0 {
		meta = append(meta, "tags: "+strings.Join(fm.Tags, ", "))
	}
	fmt.Println(dim(strings.Join(meta, "  ")))
	if fm.Description != "" {
		fmt.Println(fm.Description)
	}
	fmt.Println()
	fmt.Print(strings.TrimRight(c.Body, "\n") + "\n")
}

func printFindings(findings []validate.Finding, errs, warns int, strict bool) {
	for _, f := range findings {
		sev := f.Severity.String()
		if f.Severity == validate.SeverityError {
			sev = style("31", sev)
		} else {
			sev = style("33", sev)
		}
		fmt.Printf("%s  %s  %s %s\n", sev, bold(f.ConceptID), f.Message, dim("("+f.RuleID+")"))
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
