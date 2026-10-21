// Package search filters concepts in a bundle by tag, type, or body text.
package search

import (
	"os"
	"sort"
	"strings"

	"github.com/okfcli/okf/internal/bundle"
	"github.com/okfcli/okf/internal/concept"
)

// Filters holds optional filter criteria. Empty fields match everything.
type Filters struct {
	Tag   string   // match if concept has this tag
	Type  string   // match if concept frontmatter type equals this
	Text  string   // match if text appears in title, description, or body (case-insensitive)
	Terms []string // match if every term appears in title, description, or body
}

// Search returns concepts matching all provided filters.
// Empty filters return all concepts.
func Search(b *bundle.Bundle, f Filters) []*concept.Concept {
	terms := f.terms()
	var results []*concept.Concept
	for _, c := range b.Concepts {
		if !matchTag(c, f.Tag) {
			continue
		}
		if !matchType(c, f.Type) {
			continue
		}
		if !matchTerms(c, terms) {
			continue
		}
		results = append(results, c)
	}
	return results
}

// Line is one body line that contains at least one search term.
type Line struct {
	Number int    `json:"line"` // 1-based line number in the file
	Text   string `json:"text"`
}

// Hit is a search result with the lines that matched.
type Hit struct {
	Concept *concept.Concept
	Lines   []Line
	inTitle bool
}

// Find runs Search and, when text terms are given, attaches the matching
// lines and ranks results: concepts whose title holds every term first, then
// by number of matching lines, then by ID. Without terms the ID order of
// Search is kept.
func Find(b *bundle.Bundle, f Filters) []Hit {
	terms := f.terms()
	results := Search(b, f)
	hits := make([]Hit, 0, len(results))
	for _, c := range results {
		h := Hit{Concept: c}
		if len(terms) > 0 {
			h.Lines = matchLines(c.Path, terms)
			h.inTitle = containsAll(Fold(c.DisplayTitle()), terms)
		}
		hits = append(hits, h)
	}
	if len(terms) > 0 {
		sort.SliceStable(hits, func(i, j int) bool {
			if hits[i].inTitle != hits[j].inTitle {
				return hits[i].inTitle
			}
			return len(hits[i].Lines) > len(hits[j].Lines)
		})
	}
	return hits
}

// terms returns the folded, non-empty text terms of the filter (Text plus
// Terms), as used for matching.
func (f Filters) terms() []string {
	raw := append([]string{f.Text}, f.Terms...)
	var out []string
	for _, t := range raw {
		if t = strings.TrimSpace(Fold(t)); t != "" {
			out = append(out, t)
		}
	}
	return out
}

// FoldedTerms is the exported form of terms, for callers that highlight hits.
func (f Filters) FoldedTerms() []string { return f.terms() }

func matchTag(c *concept.Concept, tag string) bool {
	if tag == "" {
		return true
	}
	for _, t := range c.Frontmatter.Tags {
		if Fold(t) == Fold(tag) {
			return true
		}
	}
	return false
}

func matchType(c *concept.Concept, typ string) bool {
	if typ == "" {
		return true
	}
	return strings.EqualFold(c.Frontmatter.Type, typ)
}

// matchTerms reports whether every term appears in the title, description,
// or body. Terms are already folded.
func matchTerms(c *concept.Concept, terms []string) bool {
	if len(terms) == 0 {
		return true
	}
	fm := c.Frontmatter
	return containsAll(Fold(c.DisplayTitle()+"\n"+fm.Description+"\n"+c.Body), terms)
}

func containsAll(folded string, terms []string) bool {
	for _, t := range terms {
		if !strings.Contains(folded, t) {
			return false
		}
	}
	return true
}

// matchLines returns the body lines of the file at path that contain any of
// the folded terms, numbered as in the file so an editor can jump to them.
func matchLines(path string, terms []string) []Line {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	lines := strings.Split(strings.TrimPrefix(string(raw), "\uFEFF"), "\n")
	start := 0
	if len(lines) > 0 && strings.TrimRight(lines[0], "\r") == "---" {
		for i := 1; i < len(lines); i++ {
			if strings.TrimRight(lines[i], "\r") == "---" {
				start = i + 1
				break
			}
		}
	}
	var out []Line
	for i := start; i < len(lines); i++ {
		text := strings.TrimRight(lines[i], "\r")
		folded := Fold(text)
		for _, t := range terms {
			if strings.Contains(folded, t) {
				out = append(out, Line{Number: i + 1, Text: text})
				break
			}
		}
	}
	return out
}
