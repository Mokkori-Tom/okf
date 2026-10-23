package render

import "strings"

type cell struct {
	r   rune
	sty string
}

// noLineStart are runes that must not begin a line (kinsoku): closing
// brackets and Japanese punctuation stay with the text before them.
const noLineStart = "、。，．,.)）]］｝}〕〉》」』】〙〗〟’”｠»ゝゞーァィゥェォッャュョヮヵヶぁぃぅぇぉっゃゅょゎゕゖ・：；！？!?:;…‥"

// noLineEnd are runes that must not end a line: opening brackets.
const noLineEnd = "(（[［｛{〔〈《「『【〘〖〝‘“｟«"

// wrap breaks spans into lines of at most width cells. Lines may break at
// spaces, and between runes when either side is wide (CJK text has no
// spaces), except where kinsoku forbids it. A word longer than width is cut.
func wrap(spans []span, width int) [][]cell {
	var cells []cell
	for _, sp := range spans {
		for _, r := range sp.text {
			if r == '\t' {
				r = ' '
			}
			cells = append(cells, cell{r, sp.sty})
		}
	}
	if width < 1 {
		width = 1
	}
	var lines [][]cell
	var line []cell
	w := 0
	brk := -1 // index in line after which a break is allowed
	for _, c := range cells {
		cw := RuneWidth(c.r)
		if w+cw > width && len(line) > 0 {
			cut := len(line)
			last := line[len(line)-1].r
			if c.r != ' ' && !canBreak(last, c.r) && brk >= 0 {
				cut = brk + 1
			}
			// Kinsoku: if c may not start a line, pull the previous rune down.
			if cut == len(line) && strings.ContainsRune(noLineStart, c.r) && len(line) > 1 {
				cut--
			}
			head, tail := line[:cut], append([]cell(nil), line[cut:]...)
			lines = append(lines, trimRight(head))
			line = trimLeft(tail)
			w = 0
			for _, t := range line {
				w += RuneWidth(t.r)
			}
			brk = -1
		}
		line = append(line, c)
		w += cw
		if n := len(line); n >= 2 && canBreak(line[n-2].r, line[n-1].r) {
			brk = n - 2
		}
		if c.r == ' ' {
			brk = len(line) - 1
		}
	}
	lines = append(lines, trimRight(line))
	return lines
}

// canBreak reports whether a line may break between a and b.
func canBreak(a, b rune) bool {
	if a == ' ' || b == ' ' {
		return a == ' '
	}
	if !IsWide(a) && !IsWide(b) {
		return false
	}
	return !strings.ContainsRune(noLineStart, b) && !strings.ContainsRune(noLineEnd, a)
}

func trimRight(cs []cell) []cell {
	for len(cs) > 0 && cs[len(cs)-1].r == ' ' {
		cs = cs[:len(cs)-1]
	}
	return cs
}

func trimLeft(cs []cell) []cell {
	for len(cs) > 0 && cs[0].r == ' ' {
		cs = cs[1:]
	}
	return cs
}

// cellsWidth is the display width of cs.
func cellsWidth(cs []cell) int {
	w := 0
	for _, c := range cs {
		w += RuneWidth(c.r)
	}
	return w
}

// paint turns cells into a string, emitting ANSI codes when color is on.
func paint(cs []cell, color bool) string {
	var b strings.Builder
	cur := ""
	for _, c := range cs {
		if color && c.sty != cur {
			if cur != "" {
				b.WriteString("\x1b[0m")
			}
			if c.sty != "" {
				b.WriteString("\x1b[" + c.sty + "m")
			}
			cur = c.sty
		}
		b.WriteRune(c.r)
	}
	if color && cur != "" {
		b.WriteString("\x1b[0m")
	}
	return b.String()
}
