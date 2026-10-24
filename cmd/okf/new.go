package main

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/okfcli/okf/internal/cerr"
)

// newFront is the frontmatter `okf new` writes. Field order is the order a
// person reads and fills it in; description and tags are left as empty
// placeholders because lint recommends them.
type newFront struct {
	Type        string     `yaml:"type"`
	Title       string     `yaml:"title"`
	Description string     `yaml:"description"`
	Tags        []string   `yaml:"tags"`
	Status      string     `yaml:"status,omitempty"`
	Generated   *generated `yaml:"generated,omitempty"`
}

type generated struct {
	By string `yaml:"by"`
	At string `yaml:"at"`
}

const defaultNewType = "Note"

func runNew(args []string) {
	var pos []string
	front := newFront{Type: defaultNewType, Tags: []string{}}
	noEdit := false
	body, hasBody := "", false
	for i := 0; i < len(args); i++ {
		a := args[i]
		value := func() string {
			if i+1 >= len(args) {
				exitErr(cerr.Usage("%s requires a value", a))
			}
			i++
			return args[i]
		}
		switch a {
		case "--title":
			front.Title = value()
		case "--type":
			front.Type = value()
		case "--description":
			front.Description = value()
		case "--tag":
			for _, t := range strings.Split(value(), ",") {
				if t = strings.TrimSpace(t); t != "" {
					front.Tags = append(front.Tags, t)
				}
			}
		case "--status":
			front.Status = value()
		case "--body":
			body, hasBody = value(), true
			if body == "-" {
				raw, err := io.ReadAll(os.Stdin)
				if err != nil {
					exitErr(cerr.IO(err, "read body from stdin"))
				}
				body = string(raw)
			}
		case "--no-edit":
			noEdit = true
		default:
			if strings.HasPrefix(a, "-") {
				exitErr(cerr.Usage("unknown new flag: %s", a))
			}
			pos = append(pos, a)
		}
	}

	var root, id string
	switch len(pos) {
	case 1:
		root, id = defaultBundle(), pos[0]
	case 2:
		root, id = pos[0], pos[1]
	default:
		exitErr(cerr.Usage("usage: okf new [bundle] <concept-id> [--title <title>] [--type <type>] [--tag <a,b>] [--description <text>] [--status <status>] [--body <text>|-] [--no-edit]"))
	}

	id = strings.TrimSuffix(filepath.ToSlash(id), ".md")
	clean := path.Clean(id)
	if id == "" || clean != id || path.IsAbs(id) || strings.HasPrefix(id, "../") || id == ".." {
		exitErr(cerr.Usage("concept id must be a relative path inside the bundle, got %q", id))
	}
	if base := path.Base(id); base == "index" || base == "log" {
		exitErr(cerr.Usage("%s.md is reserved by OKF and cannot be a concept", base))
	}
	if front.Title == "" {
		front.Title = path.Base(id)
	}
	if by := author(); by != "" {
		front.Generated = &generated{By: by, At: time.Now().Format(time.RFC3339)}
	}

	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		exitErr(cerr.IO(fmt.Errorf("not a directory"), "bundle %s", root))
	}
	file := filepath.Join(root, filepath.FromSlash(id)+".md")

	var fm strings.Builder
	enc := yaml.NewEncoder(&fm)
	enc.SetIndent(2)
	if err := enc.Encode(front); err != nil {
		exitErr(cerr.Internal(err, "marshal frontmatter"))
	}
	content := "---\n" + fm.String() + "---\n\n# " + front.Title + "\n\n"
	if hasBody {
		// The body is written as given; a trailing newline is ensured.
		content = "---\n" + fm.String() + "---\n\n" + strings.TrimRight(body, "\n") + "\n"
	}

	if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
		exitErr(cerr.IO(err, "create directory for %s", file))
	}
	// O_EXCL: never overwrite an existing page.
	f, err := os.OpenFile(file, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		if os.IsExist(err) {
			exitErr(cerr.Validation("concept already exists: %s", file))
		}
		exitErr(cerr.IO(err, "create %s", file))
	}
	_, werr := f.WriteString(content)
	if closeErr := f.Close(); werr == nil {
		werr = closeErr
	}
	if werr != nil {
		exitErr(cerr.IO(werr, "write %s", file))
	}

	abs, _ := filepath.Abs(file)
	if !humanOut {
		outputJSON(map[string]any{
			"command": "new",
			"bundle":  root,
			"id":      id,
			"path":    abs,
			"type":    front.Type,
			"title":   front.Title,
		})
		return
	}
	fmt.Println("created " + displayPath(abs))

	if noEdit || hasBody || !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		return
	}
	if err := openEditor(abs); err != nil {
		fmt.Fprintln(os.Stderr, "editor: "+err.Error())
	}
}

// author returns the actor recorded in generated.by (OKF §7): OKF_AUTHOR as
// given, otherwise human:<login name>. Empty means unknown, and the
// generated block is left out rather than invented.
func author() string {
	if a := os.Getenv("OKF_AUTHOR"); a != "" {
		return a
	}
	for _, k := range []string{"USER", "LOGNAME"} {
		if u := os.Getenv(k); u != "" {
			return "human:" + u
		}
	}
	return ""
}

// openEditor opens file in $VISUAL or $EDITOR, falling back to the first of
// nvim, vim, vi, nano on PATH. The editor variable may carry arguments
// ("code -w"), so it runs through sh.
func openEditor(file string) error {
	editor := os.Getenv("VISUAL")
	if editor == "" {
		editor = os.Getenv("EDITOR")
	}
	if editor == "" {
		for _, e := range []string{"nvim", "vim", "vi", "nano"} {
			if _, err := exec.LookPath(e); err == nil {
				editor = e
				break
			}
		}
	}
	if editor == "" {
		return fmt.Errorf("set $EDITOR to open new pages")
	}
	cmd := exec.Command("sh", "-c", editor+` "$1"`, "sh", file) //nolint:gosec // the user's own editor setting
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
