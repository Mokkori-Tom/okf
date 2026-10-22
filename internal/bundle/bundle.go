// Package bundle walks an OKF bundle directory and loads all concept documents.
package bundle

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/okfcli/okf/internal/concept"
)

// Bundle is a loaded OKF knowledge bundle.
type Bundle struct {
	Root         string             // absolute path to the bundle root
	Concepts     []*concept.Concept // all concept documents, sorted by ID
	conceptByID  map[string]*concept.Concept
	Reserved     []*concept.Concept // index.md / log.md files (parsed if present)
	reservedByID map[string]*concept.Concept
	Unreadable   []LoadError // files and directories skipped because they could not be read
}

// LoadError records a file or directory that could not be read. The rest of
// the bundle still loads; validate reports each one.
type LoadError struct {
	Path string // bundle-relative, slash-separated
	Err  error
}

// Load walks a bundle directory and parses every .md file.
// Reserved filenames (index.md, log.md) are separated into Reserved.
// Loading is lenient so that one bad file never hides the rest: concepts with
// missing or malformed frontmatter are kept with FrontmatterErr set (see
// concept.ParseLenient), and files that cannot be read are listed in
// Unreadable. Only a missing or unreadable root is an error.
// A root that is a symlink is followed; paths stay under the given root.
func Load(root string) (*Bundle, error) {
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("resolve %s: %w", root, err)
	}
	info, err := os.Stat(absRoot)
	if err != nil {
		return nil, fmt.Errorf("stat bundle root: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("bundle root %s is not a directory", absRoot)
	}
	// WalkDir does not descend into a root that is itself a symlink, so walk
	// the resolved directory and map paths back under absRoot.
	walkRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return nil, fmt.Errorf("resolve bundle root %s: %w", absRoot, err)
	}

	b := &Bundle{
		Root:         absRoot,
		conceptByID:  make(map[string]*concept.Concept),
		reservedByID: make(map[string]*concept.Concept),
	}

	err = filepath.WalkDir(walkRoot, func(walked string, d fs.DirEntry, err error) error {
		rel, rerr := filepath.Rel(walkRoot, walked)
		if rerr != nil {
			return fmt.Errorf("rel path %s: %w", walked, rerr)
		}
		relPath := filepath.ToSlash(rel)
		if err != nil {
			if walked == walkRoot {
				return err
			}
			b.Unreadable = append(b.Unreadable, LoadError{Path: relPath, Err: err})
			if d != nil && d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() {
			// Skip hidden directories like .git
			if walked != walkRoot && strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		// Skip hidden files (editor drafts, backups) the same way hidden
		// directories are skipped: they are not part of the bundle.
		if !strings.HasSuffix(d.Name(), ".md") || strings.HasPrefix(d.Name(), ".") {
			return nil
		}
		path := filepath.Join(absRoot, rel)

		// index.md / log.md may lack frontmatter, and broken frontmatter in
		// them is reported by validate, like for concepts.
		c, perr := concept.ParseLenient(path, relPath)
		if perr != nil {
			b.Unreadable = append(b.Unreadable, LoadError{Path: relPath, Err: perr})
			return nil
		}
		if concept.ReservedNames[strings.ToLower(d.Name())] {
			b.Reserved = append(b.Reserved, c)
			b.reservedByID[c.ID] = c
			return nil
		}
		b.Concepts = append(b.Concepts, c)
		b.conceptByID[c.ID] = c
		return nil
	})
	if err != nil {
		return nil, err
	}

	sort.Slice(b.Concepts, func(i, j int) bool {
		return b.Concepts[i].ID < b.Concepts[j].ID
	})
	return b, nil
}

// Get returns a concept by ID, or nil if not found.
func (b *Bundle) Get(id string) *concept.Concept {
	return b.conceptByID[id]
}

// HasConcept reports whether a concept with the given ID exists.
func (b *Bundle) HasConcept(id string) bool {
	_, ok := b.conceptByID[id]
	return ok
}

// HasReserved reports whether a reserved file (index.md or log.md) with the
// given ID exists, e.g. "sub/index" for /sub/index.md.
func (b *Bundle) HasReserved(id string) bool {
	_, ok := b.reservedByID[id]
	return ok
}
