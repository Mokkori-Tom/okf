// Package render draws markdown for a terminal: headings, lists, quotes and
// Obsidian callouts, code blocks, tables, and inline styles, with wrapping
// that understands Japanese (wide runes, line-breaking rules). It has no
// dependencies so the okf binary stays small and builds anywhere.
package render

// Width approximates the terminal width of s: East Asian wide and full-width
// runes take two cells, everything else one.
func Width(s string) int {
	w := 0
	for _, r := range s {
		w += RuneWidth(r)
	}
	return w
}

// RuneWidth is the number of terminal cells r takes.
func RuneWidth(r rune) int {
	if IsWide(r) {
		return 2
	}
	return 1
}

// IsWide reports whether r is displayed two cells wide.
func IsWide(r rune) bool {
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
