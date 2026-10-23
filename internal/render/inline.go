package render

import (
	"path"
	"strings"
	"unicode"
)

// span is a run of text drawn with one ANSI style ("" for plain). Styles are
// SGR parameter lists such as "1" or "1;36"; nested styles are joined by ";".
type span struct {
	text string
	sty  string
}

// Inline styles.
const (
	styBold   = "1"
	styItalic = "3"
	styStrike = "9"
	styCode   = "32"
	styLink   = "4;34"
	styTag    = "36"
	styMark   = "30;43"
	styDim    = "2"
)

func join(base, add string) string {
	if base == "" {
		return add
	}
	return base + ";" + add
}

// parseInline turns one line of markdown into styled spans: **bold**,
// *italic*, ~~strike~~, ==highlight==, `code`, [[wikilinks]], [links](url),
// images, #tags and footnote markers. Markup is hidden; link targets are
// dropped in favour of their text, as a rendered page would show them.
func parseInline(s, base string) []span {
	var out []span
	var buf strings.Builder
	flush := func() {
		if buf.Len() > 0 {
			out = append(out, span{buf.String(), base})
			buf.Reset()
		}
	}
	emit := func(spans ...span) {
		flush()
		out = append(out, spans...)
	}
	rs := []rune(s)
	for i := 0; i < len(rs); i++ {
		rest := string(rs[i:])
		prev := ' '
		if i > 0 {
			prev = rs[i-1]
		}
		switch {
		case rs[i] == '\\' && i+1 < len(rs) && unicode.IsPunct(rs[i+1]):
			buf.WriteRune(rs[i+1])
			i++
		case rs[i] == '`':
			n := 1
			for i+n < len(rs) && rs[i+n] == '`' {
				n++
			}
			fence := strings.Repeat("`", n)
			if code, m, ok := enclosed(rs[i+n:], fence); ok {
				emit(span{strings.TrimSpace(code), join(base, styCode)})
				i += n + m - 1
				continue
			}
			buf.WriteString(fence)
			i += n - 1
		case strings.HasPrefix(rest, "![["):
			if inner, n, ok := enclosed(rs[i+3:], "]]"); ok {
				target, _, _ := strings.Cut(inner, "|")
				emit(span{"[" + embedLabel(target) + "]", join(base, styDim)})
				i += 3 + n - 1
				continue
			}
			buf.WriteRune(rs[i])
		case strings.HasPrefix(rest, "[["):
			if inner, n, ok := enclosed(rs[i+2:], "]]"); ok {
				target, alias, hasAlias := strings.Cut(inner, "|")
				label := alias
				if !hasAlias {
					label = strings.Replace(target, "#", " › ", 1)
				}
				emit(span{label, join(base, styLink)})
				i += 2 + n - 1
				continue
			}
			buf.WriteRune(rs[i])
		case strings.HasPrefix(rest, "!["):
			if alt, url, n, ok := mdLink(rs[i+1:]); ok {
				label := alt
				if label == "" {
					label = path.Base(url)
				}
				emit(span{"[image: " + label + "]", join(base, styDim)})
				i += n
				continue
			}
			buf.WriteRune(rs[i])
		case strings.HasPrefix(rest, "[^"):
			if inner, n, ok := enclosed(rs[i+2:], "]"); ok && !strings.ContainsAny(inner, " \t") {
				emit(span{"[" + inner + "]", join(base, styDim)})
				i += 2 + n - 1
				continue
			}
			buf.WriteRune(rs[i])
		case rs[i] == '[':
			if text, _, n, ok := mdLink(rs[i:]); ok {
				flush()
				out = append(out, parseInline(text, join(base, styLink))...)
				i += n - 1
				continue
			}
			buf.WriteRune(rs[i])
		case strings.HasPrefix(rest, "**") || strings.HasPrefix(rest, "__") && !isWordRune(prev):
			if inner, n, ok := enclosed(rs[i+2:], string(rs[i:i+2])); ok && inner != "" {
				flush()
				out = append(out, parseInline(inner, join(base, styBold))...)
				i += 2 + n - 1
				continue
			}
			buf.WriteString(string(rs[i : i+2]))
			i++
		case strings.HasPrefix(rest, "~~"), strings.HasPrefix(rest, "=="):
			sty := styStrike
			if rs[i] == '=' {
				sty = styMark
			}
			if inner, n, ok := enclosed(rs[i+2:], string(rs[i:i+2])); ok && inner != "" {
				flush()
				out = append(out, parseInline(inner, join(base, sty))...)
				i += 2 + n - 1
				continue
			}
			buf.WriteString(string(rs[i : i+2]))
			i++
		case (rs[i] == '*' || rs[i] == '_' && !isWordRune(prev)) && i+1 < len(rs) && !unicode.IsSpace(rs[i+1]):
			if inner, n, ok := enclosed(rs[i+1:], string(rs[i])); ok && inner != "" && !strings.HasSuffix(inner, " ") {
				flush()
				out = append(out, parseInline(inner, join(base, styItalic))...)
				i += 1 + n - 1
				continue
			}
			buf.WriteRune(rs[i])
		case rs[i] == '#' && unicode.IsSpace(prev) && i+1 < len(rs) && isTagRune(rs[i+1]) && !unicode.IsDigit(rs[i+1]):
			j := i + 1
			for j < len(rs) && isTagRune(rs[j]) {
				j++
			}
			emit(span{string(rs[i:j]), join(base, styTag)})
			i = j - 1
		default:
			buf.WriteRune(rs[i])
		}
	}
	flush()
	return out
}

// enclosed finds close in rs and returns the text before it and n, the number
// of runes of that text plus close. A caller at index i that consumed an
// opener of k runes continues after the closer with i += k + n - 1.
func enclosed(rs []rune, close string) (string, int, bool) {
	s := string(rs)
	end := strings.Index(s, close)
	if end < 0 {
		return "", 0, false
	}
	inner := s[:end]
	if strings.Contains(inner, "\n") {
		return "", 0, false
	}
	n := len([]rune(inner)) + len([]rune(close))
	return inner, n, true
}

// mdLink parses "[text](url)" at the start of rs and returns the number of
// runes it spans.
func mdLink(rs []rune) (text, url string, n int, ok bool) {
	if len(rs) == 0 || rs[0] != '[' {
		return "", "", 0, false
	}
	depth := 0
	for i, r := range rs {
		switch r {
		case '[':
			depth++
		case ']':
			depth--
			if depth == 0 {
				if i+1 >= len(rs) || rs[i+1] != '(' {
					return "", "", 0, false
				}
				url, m, ok := enclosed(rs[i+2:], ")")
				if !ok {
					return "", "", 0, false
				}
				return string(rs[1:i]), url, i + 2 + m, true
			}
		}
	}
	return "", "", 0, false
}

func embedLabel(target string) string {
	switch strings.ToLower(path.Ext(target)) {
	case ".png", ".jpg", ".jpeg", ".gif", ".webp", ".svg", ".bmp", ".avif":
		return "image: " + target
	}
	return "embed: " + target
}

func isWordRune(r rune) bool { return unicode.IsLetter(r) || unicode.IsDigit(r) }

func isTagRune(r rune) bool {
	return unicode.IsLetter(r) || unicode.IsDigit(r) || r == '_' || r == '-' || r == '/'
}
