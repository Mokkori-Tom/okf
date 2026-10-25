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
	"github.com/okfcli/okf/internal/concept"
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
	explicit := map[string]bool{} // frontmatter keys set by flags; they win over --body
	noEdit := false
	body, hasBody := "", false
	templateName := ""
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
			front.Title, explicit["title"] = value(), true
		case "--type":
			front.Type, explicit["type"] = value(), true
		case "--description":
			front.Description, explicit["description"] = value(), true
		case "--tag":
			explicit["tags"] = true
			for _, t := range strings.Split(value(), ",") {
				if t = strings.TrimSpace(t); t != "" {
					front.Tags = append(front.Tags, t)
				}
			}
		case "--status":
			front.Status, explicit["status"] = value(), true
		case "--body":
			body, hasBody = value(), true
			if body == "-" {
				raw, err := io.ReadAll(os.Stdin)
				if err != nil {
					exitErr(cerr.IO(err, "read body from stdin"))
				}
				body = string(raw)
			}
		case "--template":
			templateName = value()
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

	// A template and --body may each start with a frontmatter block; both
	// are merged into the generated one, so pages carry their own keys
	// (created, sources, ...) in one call. Precedence: defaults < template <
	// --body < explicit flags. The page body is the template's, then --body.
	var extras []*yaml.Node
	var parts []string
	if templateName != "" {
		tpl := findTemplate(root, templateName)
		if tpl == "" {
			exitErr(cerr.Usage("template %q not found in %s", templateName, strings.Join(templateDirs(root), ", ")))
		}
		raw, err := os.ReadFile(tpl)
		if err != nil {
			exitErr(cerr.IO(err, "read template %s", tpl))
		}
		x, rest := splitFrontYAML(expandTemplate(string(raw), front.Title, time.Now()), "the template "+tpl)
		extras, parts = append(extras, x), append(parts, rest)
	}
	if hasBody {
		x, rest := splitFrontYAML(body, "--body")
		extras, parts = append(extras, x), append(parts, rest)
	}
	fmNode, err := mergeFront(front, extras, explicit)
	if err != nil {
		exitErr(cerr.Internal(err, "build frontmatter"))
	}
	if err := fmNode.Decode(&front); err != nil {
		exitErr(cerr.Validation("frontmatter of the template or --body: %v", err))
	}
	for i := range parts {
		parts[i] = strings.Trim(parts[i], "\n")
	}
	body = strings.Join(parts, "\n\n")

	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		exitErr(cerr.IO(fmt.Errorf("not a directory"), "bundle %s", root))
	}
	file := filepath.Join(root, filepath.FromSlash(id)+".md")

	var fm strings.Builder
	enc := yaml.NewEncoder(&fm)
	enc.SetIndent(2)
	if err := enc.Encode(fmNode); err != nil {
		exitErr(cerr.Internal(err, "marshal frontmatter"))
	}
	content := "---\n" + fm.String() + "---\n\n# " + front.Title + "\n\n"
	if templateName != "" || hasBody {
		// The body is written as given; a trailing newline is ensured.
		content = "---\n" + fm.String() + "---\n\n" + body + "\n"
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

// splitFrontYAML splits a leading frontmatter block off text and parses it
// as a YAML mapping (nil when there is none). what names the source in errors.
func splitFrontYAML(text, what string) (*yaml.Node, string) {
	fmText, rest, ok := concept.SplitFrontmatter(strings.TrimPrefix(text, "\uFEFF"))
	if !ok {
		return nil, text
	}
	var doc yaml.Node
	if err := yaml.Unmarshal([]byte(fmText), &doc); err != nil {
		exitErr(cerr.Validation("frontmatter at the top of %s is not valid YAML: %v", what, err))
	}
	if len(doc.Content) == 0 {
		return nil, rest
	}
	if doc.Content[0].Kind != yaml.MappingNode {
		exitErr(cerr.Validation("frontmatter at the top of %s must be a YAML mapping (key: value lines)", what))
	}
	return doc.Content[0], rest
}

// mergeFront encodes front as a YAML mapping and merges each extra into it in
// order: keys of an extra replace the current value unless a flag set that
// key, and new keys are appended in their original order with their
// structure intact.
func mergeFront(front newFront, extras []*yaml.Node, explicit map[string]bool) (*yaml.Node, error) {
	var node yaml.Node
	if err := node.Encode(front); err != nil {
		return nil, err
	}
	for _, extra := range extras {
		if extra != nil {
			mergeInto(&node, extra, explicit)
		}
	}
	return &node, nil
}

func mergeInto(node, extra *yaml.Node, explicit map[string]bool) {
	for i := 0; i+1 < len(extra.Content); i += 2 {
		k, v := extra.Content[i], extra.Content[i+1]
		found := false
		for j := 0; j+1 < len(node.Content); j += 2 {
			if node.Content[j].Value == k.Value {
				found = true
				if !explicit[k.Value] {
					node.Content[j+1] = v
				}
				break
			}
		}
		if !found {
			node.Content = append(node.Content, k, v)
		}
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

// editorCommand is $VISUAL, else $EDITOR, else the first of nvim, vim, vi,
// nano on PATH, or "" when there is none. It may carry arguments.
func editorCommand() string {
	if e := os.Getenv("VISUAL"); e != "" {
		return e
	}
	if e := os.Getenv("EDITOR"); e != "" {
		return e
	}
	for _, e := range []string{"nvim", "vim", "vi", "nano"} {
		if _, err := exec.LookPath(e); err == nil {
			return e
		}
	}
	return ""
}

// openEditor opens file in $VISUAL or $EDITOR, falling back to the first of
// nvim, vim, vi, nano on PATH. The editor variable may carry arguments
// ("code -w"), so it runs through sh.
func openEditor(file string) error {
	editor := editorCommand()
	if editor == "" {
		return fmt.Errorf("set $EDITOR to open pages")
	}
	cmd := exec.Command("sh", "-c", editor+` "$1"`, "sh", file) //nolint:gosec // the user's own editor setting
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
