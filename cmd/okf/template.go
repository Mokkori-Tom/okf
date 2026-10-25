package main

// Page templates for `okf new --template`. Templates are markdown files in
// $OKF_TEMPLATES or the bundle's Templates/ (or templates/) folder, written
// with the placeholders of Obsidian's Templates plugin plus date offsets:
//
//	{{title}}  {{date}}  {{time}}  {{date:YYYY-MM-DD}}  {{time:HH:mm}}
//	{{date+1d:YYYY-MM-DD}}  {{date-1w}}  (units: d, w, M, y)
//
// Dates are relative to the page's date when its title is one (2026-10-27,
// as for daily notes), else to now.

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
)

// templateDirs lists where templates are looked up, in order.
func templateDirs(root string) []string {
	var dirs []string
	if d := os.Getenv("OKF_TEMPLATES"); d != "" {
		dirs = append(dirs, d)
	}
	return append(dirs, filepath.Join(root, "Templates"), filepath.Join(root, "templates"))
}

// findTemplate resolves a template given by path or by name (with or without
// .md) in the template folders. It returns "" when there is none.
func findTemplate(root, name string) string {
	if isMarkdownFile(name) {
		return name
	}
	file := name
	if !strings.HasSuffix(strings.ToLower(file), ".md") {
		file += ".md"
	}
	for _, d := range templateDirs(root) {
		if p := filepath.Join(d, file); isMarkdownFile(p) {
			return p
		}
	}
	return ""
}

// templateNames lists the templates in the first template folder that
// exists, for completion.
func templateNames(root string) []string {
	for _, d := range templateDirs(root) {
		entries, err := os.ReadDir(d)
		if err != nil {
			continue
		}
		var names []string
		for _, e := range entries {
			if !e.IsDir() && strings.HasSuffix(e.Name(), ".md") && !strings.HasPrefix(e.Name(), ".") {
				names = append(names, strings.TrimSuffix(e.Name(), ".md"))
			}
		}
		sort.Strings(names)
		return names
	}
	return nil
}

var (
	templateVar = regexp.MustCompile(`\{\{\s*(title|date|time)\s*([+-]\d+[dwMy])?\s*(?::([^}]*))?\}\}`)
	isoDate     = regexp.MustCompile(`^(\d{4})-(\d{2})-(\d{2})$`)
)

// templateBase is the date placeholders are relative to: the page's date
// when its title is YYYY-MM-DD, else now.
func templateBase(title string, now time.Time) time.Time {
	if m := isoDate.FindStringSubmatch(title); m != nil {
		y, _ := strconv.Atoi(m[1])
		mo, _ := strconv.Atoi(m[2])
		d, _ := strconv.Atoi(m[3])
		t := time.Date(y, time.Month(mo), d, now.Hour(), now.Minute(), now.Second(), 0, now.Location())
		if t.Month() == time.Month(mo) && t.Day() == d { // reject 2026-02-31
			return t
		}
	}
	return now
}

// expandTemplate fills in the placeholders of a template.
func expandTemplate(s, title string, now time.Time) string {
	base := templateBase(title, now)
	return templateVar.ReplaceAllStringFunc(s, func(m string) string {
		g := templateVar.FindStringSubmatch(m)
		name, offset, format := g[1], g[2], strings.TrimSpace(g[3])
		if name == "title" {
			return title
		}
		t := base
		if offset != "" {
			n, _ := strconv.Atoi(offset[:len(offset)-1])
			switch offset[len(offset)-1] {
			case 'd':
				t = t.AddDate(0, 0, n)
			case 'w':
				t = t.AddDate(0, 0, 7*n)
			case 'M':
				t = t.AddDate(0, n, 0)
			case 'y':
				t = t.AddDate(n, 0, 0)
			}
		}
		if format == "" {
			format = "YYYY-MM-DD"
			if name == "time" {
				format = "HH:mm"
			}
		}
		return momentFormat(t, format)
	})
}

// momentTokens are the moment.js format tokens supported, longest first so
// that YYYY wins over YY.
var momentTokens = []string{"YYYY", "dddd", "ddd", "YY", "MM", "DD", "HH", "mm", "ss", "M", "D", "H"}

// momentFormat formats t with a moment.js-style pattern, as Obsidian does.
// Text in [brackets] is copied literally.
func momentFormat(t time.Time, f string) string {
	var b strings.Builder
	for i := 0; i < len(f); {
		if f[i] == '[' {
			if end := strings.IndexByte(f[i:], ']'); end > 0 {
				b.WriteString(f[i+1 : i+end])
				i += end + 1
				continue
			}
		}
		matched := false
		for _, tok := range momentTokens {
			if strings.HasPrefix(f[i:], tok) {
				b.WriteString(momentToken(t, tok))
				i += len(tok)
				matched = true
				break
			}
		}
		if !matched {
			b.WriteByte(f[i])
			i++
		}
	}
	return b.String()
}

func momentToken(t time.Time, tok string) string {
	switch tok {
	case "YYYY":
		return strconv.Itoa(t.Year())
	case "YY":
		return t.Format("06")
	case "MM":
		return t.Format("01")
	case "M":
		return strconv.Itoa(int(t.Month()))
	case "DD":
		return t.Format("02")
	case "D":
		return strconv.Itoa(t.Day())
	case "HH":
		return t.Format("15")
	case "H":
		return strconv.Itoa(t.Hour())
	case "mm":
		return t.Format("04")
	case "ss":
		return t.Format("05")
	case "dddd":
		return t.Weekday().String()
	case "ddd":
		return t.Weekday().String()[:3]
	}
	return tok
}
