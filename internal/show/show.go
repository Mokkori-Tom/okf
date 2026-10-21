// Package show retrieves a single concept from a bundle for display.
package show

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/okfcli/okf/internal/bundle"
	"github.com/okfcli/okf/internal/concept"
)

// Show returns the concept with the given ID from the bundle.
// Returns an error if the concept is not found.
func Show(b *bundle.Bundle, conceptID string) (*concept.Concept, error) {
	c := b.Get(conceptID)
	if c == nil {
		return nil, fmt.Errorf("concept not found: %s", conceptID)
	}
	return c, nil
}

// File parses the concept document at path directly, without loading the
// bundle. Its ID is the path relative to root when the file lies inside root,
// else the file name without .md. A file without (or with broken)
// frontmatter is returned with FrontmatterErr set, so plain markdown can
// still be read.
func File(path, root string) (*concept.Concept, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	rel := filepath.Base(abs)
	if absRoot, err := filepath.Abs(root); err == nil {
		if r, err := filepath.Rel(absRoot, abs); err == nil && r != ".." && !strings.HasPrefix(r, ".."+string(filepath.Separator)) {
			rel = r
		}
	}
	c, err := concept.ParseLenient(abs, rel)
	if err != nil {
		return nil, err
	}
	return c, nil
}
