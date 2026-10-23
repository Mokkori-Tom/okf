package render

import (
	"regexp"
	"strings"
)

// Options control rendering.
type Options struct {
	Width int  // terminal cells per line; values below 20 are raised to 20
	Color bool // emit ANSI styles
}

var (
	headingRe = regexp.MustCompile(`^(#{1,6})\s+(.*?)\s*#*\s*$`)
	hrRe      = regexp.MustCompile(`^\s{0,3}([-*_])(\s*[-*_]){2,}\s*$`)
	listRe    = regexp.MustCompile(`^(\s*)([-*+]|\d{1,9}[.)])\s+(.*)$`)
	taskRe    = regexp.MustCompile(`^\[([ xX])\]\s+(.*)$`)
	tableSep  = regexp.MustCompile(`^\s*\|?\s*:?-{1,}:?\s*(\|\s*:?-{1,}:?\s*)*\|?\s*$`)
	calloutRe = regexp.MustCompile(`^\[!([A-Za-z-]+)\][+-]?\s*(.*)$`)
)

// Heading styles by level (1-6).
var headingStyle = [...]string{"", "1;35", "1;36", "1;33", "1", "1", "1"}

// Markdown renders a markdown body for the terminal.
func Markdown(md string, o Options) string {
	if o.Width < 20 {
		o.Width = 20
	}
	r := renderer{o: o}
	lines := strings.Split(strings.ReplaceAll(md, "\r\n", "\n"), "\n")
	out := r.blocks(lines, o.Width)
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	for len(out) > 0 && out[0] == "" {
		out = out[1:]
	}
	return strings.Join(out, "\n") + "\n"
}

// Heading renders one heading line as Markdown would draw it.
func Heading(text string, level int, o Options) string {
	r := renderer{o: o}
	return strings.Join(r.heading(text, level, o.Width), "\n") + "\n"
}

type renderer struct{ o Options }

func (r renderer) style(sty, s string) string {
	if !r.o.Color || sty == "" || s == "" {
		return s
	}
	return "\x1b[" + sty + "m" + s + "\x1b[0m"
}

// blocks renders lines to painted output lines no wider than width.
func (r renderer) blocks(lines []string, width int) []string {
	var out []string
	blank := func() {
		if len(out) > 0 && out[len(out)-1] != "" {
			out = append(out, "")
		}
	}
	for i := 0; i < len(lines); i++ {
		line := strings.TrimRight(lines[i], " \t")
		trimmed := strings.TrimSpace(line)
		switch {
		case trimmed == "":
			blank()

		case strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~"):
			marker := trimmed[:3]
			lang := strings.TrimSpace(strings.TrimLeft(trimmed, marker[:1]))
			j := i + 1
			for j < len(lines) && !strings.HasPrefix(strings.TrimSpace(lines[j]), marker) {
				j++
			}
			out = append(out, r.code(lines[i+1:min(j, len(lines))], lang, width)...)
			i = j

		case headingRe.MatchString(trimmed) && !strings.HasPrefix(line, "    "):
			m := headingRe.FindStringSubmatch(trimmed)
			blank()
			out = append(out, r.heading(m[2], len(m[1]), width)...)

		case hrRe.MatchString(line):
			blank()
			out = append(out, r.style(styDim, strings.Repeat("─", min(width, 80))))
			blank()

		case strings.HasPrefix(trimmed, "|") && i+1 < len(lines) && tableSep.MatchString(lines[i+1]) && strings.Contains(lines[i+1], "-"):
			j := i + 2
			for j < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[j]), "|") {
				j++
			}
			rows := [][]string{splitRow(lines[i])}
			for _, l := range lines[i+2 : j] {
				rows = append(rows, splitRow(l))
			}
			out = append(out, r.table(rows, width)...)
			i = j - 1

		case strings.HasPrefix(trimmed, ">"):
			j := i
			var inner []string
			for j < len(lines) && strings.HasPrefix(strings.TrimSpace(lines[j]), ">") {
				s := strings.TrimPrefix(strings.TrimSpace(lines[j]), ">")
				inner = append(inner, strings.TrimPrefix(s, " "))
				j++
			}
			out = append(out, r.quote(inner, width)...)
			i = j - 1

		case listRe.MatchString(line):
			out = append(out, r.listItem(listRe.FindStringSubmatch(line), width)...)

		default:
			ind := indentWidth(line)
			pad := strings.Repeat(" ", min(ind, width/2))
			for _, l := range wrap(parseInline(trimmed, ""), width-len(pad)) {
				out = append(out, pad+paint(l, r.o.Color))
			}
		}
	}
	return out
}

func (r renderer) heading(text string, level, width int) []string {
	sty := headingStyle[min(max(level, 1), 6)]
	var out []string
	maxW := 0
	for _, l := range wrap(parseInline(text, sty), width) {
		out = append(out, paint(l, r.o.Color))
		maxW = max(maxW, cellsWidth(l))
	}
	switch level {
	case 1:
		out = append(out, r.style("35", strings.Repeat("━", min(maxW, width))))
	case 2:
		out = append(out, r.style("36", strings.Repeat("─", min(maxW, width))))
	}
	return out
}

func (r renderer) code(lines []string, lang string, width int) []string {
	bar := r.style(styDim, "▏ ")
	var out []string
	if lang != "" {
		out = append(out, bar+r.style(styDim, lang))
	}
	for _, l := range lines {
		l = strings.ReplaceAll(l, "\t", "    ")
		// Code is not re-flowed; overlong lines are cut, leaving one cell
		// for the ellipsis, so the bar stays aligned.
		cs := []cell{}
		w := 0
		fits := Width(l) <= width-2
		for _, c := range l {
			if !fits && w+RuneWidth(c) > width-3 {
				cs = append(cs, cell{'…', styDim})
				break
			}
			cs = append(cs, cell{c, styCode})
			w += RuneWidth(c)
		}
		out = append(out, bar+paint(cs, r.o.Color))
	}
	return out
}

// Callout colors by Obsidian callout type.
var calloutColor = map[string]string{
	"note": "34", "info": "34", "todo": "34", "abstract": "36", "summary": "36", "tldr": "36",
	"tip": "36", "hint": "36", "important": "36",
	"success": "32", "check": "32", "done": "32",
	"question": "33", "help": "33", "faq": "33", "warning": "33", "caution": "33", "attention": "33",
	"failure": "31", "fail": "31", "missing": "31", "danger": "31", "error": "31", "bug": "31",
	"example": "35", "quote": "2", "cite": "2",
}

func (r renderer) quote(inner []string, width int) []string {
	color := "2"
	var head []string
	if len(inner) > 0 {
		if m := calloutRe.FindStringSubmatch(inner[0]); m != nil {
			kind := strings.ToLower(m[1])
			if c, ok := calloutColor[kind]; ok {
				color = c
			} else {
				color = "34"
			}
			title := m[2]
			if title == "" {
				title = strings.ToUpper(kind[:1]) + kind[1:]
			}
			for _, l := range wrap(parseInline(title, join(color, styBold)), width-2) {
				head = append(head, paint(l, r.o.Color))
			}
			inner = inner[1:]
		}
	}
	bar := r.style(color, "│ ")
	var out []string
	for _, l := range head {
		out = append(out, bar+l)
	}
	for _, l := range r.blocks(inner, width-2) {
		out = append(out, strings.TrimRight(bar+l, " "))
	}
	return out
}

var bullets = []string{"•", "◦", "▪"}

func (r renderer) listItem(m []string, width int) []string {
	level := indentWidth(m[1]) / 2
	marker := m[2]
	text := m[3]
	sty := "36"
	if strings.ContainsAny(marker, "-*+") {
		marker = bullets[level%len(bullets)]
	}
	if t := taskRe.FindStringSubmatch(text); t != nil {
		text = t[2]
		if t[1] == " " {
			marker = "[ ]"
		} else {
			marker, sty = "[x]", "32"
			text = "~~" + text + "~~"
		}
	}
	pad := strings.Repeat(" ", min(level*2, width/2))
	prefix := pad + marker + " "
	hang := strings.Repeat(" ", Width(prefix))
	var out []string
	for k, l := range wrap(parseInline(text, ""), width-Width(prefix)) {
		if k == 0 {
			out = append(out, pad+r.style(sty, marker)+" "+paint(l, r.o.Color))
		} else {
			out = append(out, hang+paint(l, r.o.Color))
		}
	}
	return out
}

// splitRow splits a table row on | outside code spans, dropping the outer pipes.
func splitRow(line string) []string {
	s := strings.TrimSpace(line)
	s = strings.TrimPrefix(s, "|")
	if strings.HasSuffix(s, "|") && !strings.HasSuffix(s, `\|`) {
		s = s[:len(s)-1]
	}
	var cells []string
	var b strings.Builder
	inCode := false
	for i := 0; i < len(s); i++ {
		switch {
		case s[i] == '\\' && i+1 < len(s) && s[i+1] == '|':
			b.WriteByte('|')
			i++
		case s[i] == '`':
			inCode = !inCode
			b.WriteByte('`')
		case s[i] == '|' && !inCode:
			cells = append(cells, strings.TrimSpace(b.String()))
			b.Reset()
		default:
			b.WriteByte(s[i])
		}
	}
	return append(cells, strings.TrimSpace(b.String()))
}

func (r renderer) table(rows [][]string, width int) []string {
	ncol := len(rows[0])
	cells := make([][][]span, len(rows))
	natural := make([]int, ncol)
	for i, row := range rows {
		cells[i] = make([][]span, ncol)
		for c := 0; c < ncol; c++ {
			text := ""
			if c < len(row) {
				text = row[c]
			}
			base := ""
			if i == 0 {
				base = styBold
			}
			cells[i][c] = parseInline(text, base)
			w := 0
			for _, sp := range cells[i][c] {
				w += Width(sp.text)
			}
			natural[c] = max(natural[c], w, 1)
		}
	}
	// Shrink the widest columns until the table fits.
	widths := append([]int(nil), natural...)
	avail := width - 3*(ncol-1) - 2
	for sum(widths) > avail {
		k := 0
		for c := range widths {
			if widths[c] > widths[k] {
				k = c
			}
		}
		if widths[k] <= 4 {
			// Too many columns for the screen: show each row as a list of
			// "header: value" lines instead.
			return r.records(cells, width)
		}
		widths[k]--
	}

	sep := r.style(styDim, " │ ")
	var out []string
	for i := range cells {
		wrapped := make([][][]cell, ncol)
		height := 1
		for c := 0; c < ncol; c++ {
			wrapped[c] = wrap(cells[i][c], widths[c])
			height = max(height, len(wrapped[c]))
		}
		for h := 0; h < height; h++ {
			parts := make([]string, ncol)
			for c := 0; c < ncol; c++ {
				var l []cell
				if h < len(wrapped[c]) {
					l = wrapped[c][h]
				}
				parts[c] = paint(l, r.o.Color) + strings.Repeat(" ", max(widths[c]-cellsWidth(l), 0))
			}
			out = append(out, strings.TrimRight(" "+strings.Join(parts, sep), " "))
		}
		if i == 0 {
			rule := make([]string, ncol)
			for c := range rule {
				rule[c] = strings.Repeat("─", widths[c])
			}
			out = append(out, r.style(styDim, "─"+strings.Join(rule, "─┼─")+"─"))
		}
	}
	return out
}

// records draws a table that does not fit as one block per row, each cell
// on its own line after its column header.
func (r renderer) records(cells [][][]span, width int) []string {
	var out []string
	header := cells[0]
	for i, row := range cells[1:] {
		if i > 0 {
			out = append(out, r.style(styDim, strings.Repeat("─", min(width, 40))))
		}
		for c, value := range row {
			label := append(append([]span(nil), header[c]...), span{": ", styBold})
			line := append(label, value...)
			for k, l := range wrap(line, width-2) {
				if k == 0 {
					out = append(out, paint(l, r.o.Color))
				} else {
					out = append(out, "  "+paint(l, r.o.Color))
				}
			}
		}
	}
	return out
}

func indentWidth(s string) int {
	w := 0
	for _, c := range s {
		switch c {
		case ' ':
			w++
		case '\t':
			w += 4
		default:
			return w
		}
	}
	return w
}

func sum(xs []int) int {
	t := 0
	for _, x := range xs {
		t += x
	}
	return t
}

// Text wraps plain text (no markdown) in one style to the width.
func Text(text, sty string, o Options) string {
	var out []string
	for _, l := range wrap([]span{{text, sty}}, max(o.Width, 20)) {
		out = append(out, paint(l, o.Color))
	}
	return strings.Join(out, "\n") + "\n"
}
