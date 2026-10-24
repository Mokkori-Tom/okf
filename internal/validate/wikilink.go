package validate

import (
	"path"
	"strings"

	"github.com/okfcli/okf/internal/bundle"
)

// extractWikiLinks returns the Obsidian-style [[target]] links in body,
// including embeds (![[target]]), alias ([[a|b]]) and heading ([[a#h]])
// forms. Links inside code spans and fenced code are ignored.
func extractWikiLinks(body string) []Link {
	masked := maskCode(body) // same length as body, code blanked out
	var out []Link
	for i := 0; i < len(masked); {
		j := strings.Index(masked[i:], "[[")
		if j < 0 {
			break
		}
		start := i + j + 2
		k := strings.Index(masked[start:], "]]")
		if k < 0 {
			break
		}
		inner := body[start : start+k]
		if inner != "" && !strings.ContainsAny(inner, "\n[") {
			out = append(out, Link{Text: inner, Target: inner, Wiki: true})
		}
		i = start + k + 2
	}
	return out
}

// attachmentExt are file types a wikilink can point to that are not pages.
var attachmentExt = map[string]bool{
	".png": true, ".jpg": true, ".jpeg": true, ".gif": true, ".webp": true, ".svg": true,
	".bmp": true, ".avif": true, ".pdf": true, ".mp3": true, ".wav": true, ".ogg": true,
	".m4a": true, ".flac": true, ".mp4": true, ".webm": true, ".mov": true, ".mkv": true,
	".csv": true, ".zip": true, ".canvas": true, ".excalidraw": true,
}

// wikiTarget strips the alias, heading and block parts of a wikilink and a
// trailing .md, leaving the page name or path. rooted reports a leading "/",
// which pins the path to the bundle root.
func wikiTarget(raw string) (t string, rooted bool) {
	t, _, _ = strings.Cut(raw, "|")
	if i := strings.IndexAny(t, "#^"); i >= 0 {
		t = t[:i]
	}
	t = strings.TrimSpace(strings.ReplaceAll(t, `\`, "/"))
	rooted = strings.HasPrefix(t, "/")
	return strings.TrimSuffix(strings.TrimPrefix(t, "/"), ".md"), rooted
}

// ResolveIn resolves link to a concept ID within b. Markdown links resolve as
// ResolveLink does. Wikilinks resolve like Obsidian, nearest first: the path
// relative to the linking concept's directory, then the exact path from the
// bundle root, then by page name anywhere in the bundle (case-insensitive),
// preferring the linking concept's directory and then the shortest path. A
// leading "/" ([[/a]]) means the bundle root only. An unresolved wikilink
// returns its name, which callers report as broken; links to attachments and
// to headings of the same page ([[#h]]) return "".
func ResolveIn(b *bundle.Bundle, fromConceptID string, link Link) string {
	if !link.Wiki {
		return resolveLink(fromConceptID, link)
	}
	t, rooted := wikiTarget(link.Target)
	if t == "" || attachmentExt[strings.ToLower(path.Ext(t))] {
		return ""
	}
	dir := dirOf(fromConceptID)
	if dir != "" && !rooted {
		if rel := normalizePath(dir + "/" + t); b.HasConcept(rel) {
			return rel
		}
	}
	if b.HasConcept(t) || b.HasReserved(t) || rooted {
		return t
	}
	matches := b.FindByName(t)
	if len(matches) == 0 {
		return t
	}
	best := matches[0]
	for _, m := range matches[1:] {
		mSame, bestSame := dirOf(m) == dir, dirOf(best) == dir
		switch {
		case mSame != bestSame:
			if mSame {
				best = m
			}
		case len(m) < len(best):
			best = m
		}
	}
	return best
}
