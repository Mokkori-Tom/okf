// Command okf is a CLI toolkit for the Open Knowledge Format (OKF).
// It validates, lints, indexes, and inspects OKF bundles.
//
// okf is designed agentic-first: all output is JSON on stdout by default,
// `okf schema` emits a complete machine-readable description of every command,
// and all errors are emitted as JSON envelopes with stable exit codes.
// An external AI agent can discover and drive the entire CLI from that one
// command.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/okfcli/okf/internal/backlinks"
	"github.com/okfcli/okf/internal/bundle"
	"github.com/okfcli/okf/internal/cerr"
	"github.com/okfcli/okf/internal/concept"
	"github.com/okfcli/okf/internal/graph"
	"github.com/okfcli/okf/internal/index"
	"github.com/okfcli/okf/internal/initbundle"
	"github.com/okfcli/okf/internal/search"
	"github.com/okfcli/okf/internal/show"
	"github.com/okfcli/okf/internal/validate"
)

var (
	version = "dev"
	commit  = "none"
	date    = "unknown"
)

func main() {
	args := setOutputMode(os.Args[1:])
	if len(args) == 0 {
		printUsage()
		os.Exit(0)
	}

	cmd := args[0]
	rest := args[1:]
	if cmd != "help" && wantsHelp(rest) {
		printCommandHelp(cmd)
		return
	}

	switch cmd {
	case "version", "--version", "-v":
		outputJSON(map[string]any{"name": "okf", "version": version, "commit": commit, "date": date})
	case "help", "--help", "-h":
		if len(rest) > 0 {
			printCommandHelp(rest[0])
			return
		}
		printUsage()
	case "schema":
		runSchema(rest)
	case "validate":
		runValidate(rest, true)
	case "lint":
		runValidate(rest, false)
	case "index":
		runIndex(rest)
	case "graph":
		runGraph(rest)
	case "list":
		runList(rest)
	case "show":
		runShow(rest)
	case "search":
		runSearch(rest)
	case "init":
		runInit(rest)
	case "new":
		runNew(rest)
	case "edit":
		runEdit(rest)
	case "completion":
		runCompletion(rest)
	case "__complete":
		runComplete(rest)
	case "backlinks":
		runBacklinks(rest)
	default:
		exitErr(cerr.Usage("unknown command: %s", cmd))
	}
}

const usage = `okf - Open Knowledge Format toolkit (v%s)

Usage:
  okf <command> [bundle-path]

Output is JSON on stdout when piped (what agents and scripts get) and plain
text on a terminal. --json / --human or OKF_OUTPUT=json|human force a mode.
Diagnostics go to stderr. The bundle path defaults to $OKF_BUNDLE, else the
current directory.

Commands:
  schema [command]            Print machine-readable CLI metadata as JSON
  init <bundle>               Create a new empty OKF bundle
  validate <bundle>           Validate a bundle against the OKF spec
  lint <bundle>               Check recommended fields and style (warnings only)
  index <bundle>              Generate index.md files (progressive disclosure)
  list <bundle>               List all concepts in the bundle
  show <bundle> <concept-id> Show a single concept's full content
  search <bundle> [terms...]  Search concepts by text terms, --tag, --type
  new <bundle> <concept-id>   Create a page: from flags and --body (- = stdin) in one call,
                              or open it in $EDITOR on a terminal (see okf new -h)
  edit <bundle> <concept-id>  Open a page in $EDITOR (v in the show pager does the same)
  completion zsh|bash         Print a shell completion script (page IDs, tags, types)
  backlinks <bundle> <id>    List concepts that link to a given concept
  graph <bundle>             Print cross-link graph statistics
  version                     Print version

Exit codes:
  0  success
  1  validation error (spec violation, bad input)
  2  filesystem or I/O error
  3  internal error (unexpected)
  4  usage error (missing args, unknown command)

OKF spec: https://github.com/GoogleCloudPlatform/knowledge-catalog/blob/main/okf/SPEC.md
`

func printUsage() {
	fmt.Fprintf(os.Stderr, usage, version)
}

// --- shared helpers ---

func outputJSON(v any) {
	out, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: marshal: %v\n", err)
		os.Exit(cerr.ExitCodeInternal)
	}
	fmt.Println(string(out))
}

// exitErr prints a structured JSON error envelope to stdout and exits with
// the mapped exit code.
func exitErr(err error) {
	e := cerr.From(err)
	if e == nil {
		os.Exit(0)
	}
	if !humanOut {
		b, _ := e.ToJSON()
		fmt.Println(string(b))
	}
	fmt.Fprintf(os.Stderr, "error[%s]: %s\n", e.Kind, e.Message)
	if humanOut && e.Hint != "" {
		fmt.Fprintf(os.Stderr, "hint: %s\n", e.Hint)
	}
	os.Exit(e.ExitCode())
}

// rejectFlags returns a usage error when any argument looks like a flag.
// Commands that take no flags call it so a mistyped or unsupported flag is
// reported as a usage mistake rather than being treated as a bundle path
// and failing as I/O (issue #31).
func rejectFlags(cmd string, args []string) *cerr.Error {
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			return cerr.Usage("unknown %s flag: %s", cmd, a)
		}
	}
	return nil
}

// loadBundle loads the bundle at path. Loading is lenient (see bundle.Load),
// so only a missing or unreadable root fails; files that were skipped are
// named on stderr, and validate reports them as errors.
func loadBundle(path string) (*bundle.Bundle, *cerr.Error) {
	b, err := bundle.Load(path)
	if err != nil {
		return nil, cerr.IO(err, "load bundle %s", path)
	}
	for _, u := range b.Unreadable {
		fmt.Fprintf(os.Stderr, "warning: skipped %s: %v\n", u.Path, u.Err)
	}
	return b, nil
}

func mustBundle(cmd string, args []string) *bundle.Bundle {
	if err := rejectFlags(cmd, args); err != nil {
		exitErr(err)
	}
	if len(args) == 0 {
		args = []string{defaultBundle()}
	}
	b, err := loadBundle(args[0])
	if err != nil {
		exitErr(err)
	}
	return b
}

// --- validate / lint ---

func runValidate(args []string, strict bool) {
	// Flags may appear before or after the bundle path
	// (okf validate --format sarif bundle, okf validate bundle --format sarif).
	bundlePath := ""
	format := "json"
	exitZero := false
	strictMode := false
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--strict":
			strictMode = true
		case "--format":
			if i+1 >= len(args) {
				exitErr(cerr.Usage("--format requires a value"))
			}
			format = args[i+1]
			i++
		case "--exit-zero":
			// lint always exits 0, so the flag only exists on validate.
			if !strict {
				exitErr(cerr.Usage("unknown lint flag: --exit-zero"))
			}
			exitZero = true
		default:
			if strings.HasPrefix(args[i], "-") {
				exitErr(cerr.Usage("unknown %s flag: %s", commandName(strict), args[i]))
			}
			if bundlePath != "" {
				exitErr(cerr.Usage("unexpected argument: %s", args[i]))
			}
			bundlePath = args[i]
		}
	}
	if bundlePath == "" {
		bundlePath = defaultBundle()
	}
	if format != "json" && format != "sarif" {
		exitErr(cerr.Usage("--format must be json or sarif, got %q", format))
	}

	b, lerr := loadBundle(bundlePath)
	if lerr != nil {
		exitErr(lerr)
	}
	r := validate.ValidateWith(b, validate.Options{Strict: strictMode})

	// lint reports warnings only; validate reports everything.
	kept := make([]validate.Finding, 0, len(r.Findings))
	for _, f := range r.Findings {
		if !strict && f.Severity == validate.SeverityError {
			continue
		}
		kept = append(kept, f)
	}

	if format == "json" && humanOut {
		printFindings(kept, r.Errors, len(kept)-r.Errors, strict)
	} else if format == "sarif" {
		out, err := validate.SARIF(kept, version)
		if err != nil {
			exitErr(cerr.Internal(err, "marshal sarif"))
		}
		fmt.Println(string(out))
	} else {
		findings := make([]map[string]any, 0, len(kept))
		for _, f := range kept {
			findings = append(findings, map[string]any{
				"concept_id": f.ConceptID,
				"rule":       f.RuleID,
				"severity":   f.Severity.String(),
				"message":    f.Message,
			})
		}

		outputJSON(map[string]any{
			"command":  commandName(strict),
			"bundle":   b.Root,
			"findings": findings,
			"errors":   r.Errors,
			"warnings": r.Warnings,
			"valid":    !r.HasErrors(),
			"strict":   strictMode,
		})
	}

	if strict && r.HasErrors() && !exitZero {
		os.Exit(cerr.ExitCodeValidation)
	}
}

func commandName(strict bool) string {
	if strict {
		return "validate"
	}
	return "lint"
}

// --- index ---

func runIndex(args []string) {
	if err := rejectFlags("index", args); err != nil {
		exitErr(err)
	}
	if len(args) == 0 {
		args = []string{defaultBundle()}
	}
	root := args[0]
	// Load before writing anything so a bundle that does not parse (issue
	// #27) is reported by name instead of being half-indexed and then
	// silently miscounted below.
	if _, lerr := loadBundle(root); lerr != nil {
		exitErr(lerr)
	}
	if err := index.Generate(root); err != nil {
		exitErr(cerr.IO(err, "generate index in %s", root))
	}

	// Collect generated index file paths.
	var indexFiles []string
	b, err := bundle.Load(root)
	if err != nil {
		exitErr(cerr.IO(err, "reload bundle %s after indexing", root))
	}
	for _, r := range b.Reserved {
		if r.ID == "index" || strings.HasSuffix(r.ID, "/index") {
			indexFiles = append(indexFiles, r.Path)
		}
	}

	outputJSON(map[string]any{
		"command":         "index",
		"bundle":          root,
		"indexes_written": indexFiles,
		"count":           len(indexFiles),
	})
}

// --- graph ---

func runGraph(args []string) {
	b := mustBundle("graph", args)
	g := graph.Build(b)
	s := g.Stats()

	density := 0.0
	if s.NodeCount > 1 && s.EdgeCount > 0 {
		density = float64(s.EdgeCount) / float64(s.NodeCount*(s.NodeCount-1)) * 100
	}

	nodes := make([]map[string]string, 0, len(g.Nodes))
	for _, n := range g.Nodes {
		nodes = append(nodes, map[string]string{"id": n.ID, "type": n.Type})
	}
	edges := make([]map[string]string, 0, len(g.Edges))
	for _, e := range g.Edges {
		edges = append(edges, map[string]string{"from": e.From, "to": e.To})
	}

	outputJSON(map[string]any{
		"command":       "graph",
		"bundle":        b.Root,
		"nodes":         nodes,
		"edges":         edges,
		"node_count":    s.NodeCount,
		"edge_count":    s.EdgeCount,
		"isolated":      s.IsolatedNodes,
		"max_backlinks": s.MaxBacklinks,
		"density_pct":   density,
	})
}

// --- list ---

func runList(args []string) {
	b := mustBundle("list", args)
	if humanOut {
		printConcepts(b.Concepts)
		return
	}

	concepts := make([]map[string]string, 0, len(b.Concepts))
	for _, c := range b.Concepts {
		concepts = append(concepts, map[string]string{
			"id":         c.ID,
			"type":       c.Frontmatter.Type,
			"title":      c.DisplayTitle(),
			"status":     c.Frontmatter.EffectiveStatus(),
			"trust_tier": c.Frontmatter.TrustTier(),
		})
	}

	outputJSON(map[string]any{
		"command":  "list",
		"bundle":   b.Root,
		"concepts": concepts,
		"count":    len(b.Concepts),
	})
}

// --- show ---

func runShow(args []string) {
	raw, noPager := false, false
	var pos []string
	for _, a := range args {
		switch a {
		case "--raw":
			raw = true
		case "--no-pager":
			noPager = true
		default:
			pos = append(pos, a)
		}
	}
	args = pos
	if err := rejectFlags("show", args); err != nil {
		exitErr(err)
	}
	if len(args) == 1 {
		args = []string{defaultBundle(), args[0]}
	}
	if len(args) < 2 {
		exitErr(cerr.Usage("usage: okf show [bundle] <concept-id | file.md>"))
	}
	var c *concept.Concept
	root := args[0]
	if isMarkdownFile(args[1]) {
		// A path to an existing file is read directly, without loading the
		// rest of the bundle.
		var err error
		if c, err = show.File(args[1], root); err != nil {
			exitErr(cerr.Validation("%s", err))
		}
		root, _ = filepath.Abs(root)
	} else {
		b := mustBundle("show", args[:1])
		var err error
		if c, err = show.Show(b, strings.TrimSuffix(args[1], ".md")); err != nil {
			exitErr(cerr.Validation("%s", err))
		}
		root = b.Root
	}
	if humanOut {
		printConcept(c, root, raw, noPager)
		return
	}

	var tags []string
	if c.Frontmatter.Tags != nil {
		tags = c.Frontmatter.Tags
	}

	fm := c.Frontmatter
	out := map[string]any{
		"id":          c.ID,
		"path":        c.Path,
		"type":        fm.Type,
		"title":       c.DisplayTitle(),
		"description": fm.Description,
		"resource":    fm.Resource,
		"tags":        tags,
		"body":        c.Body,
		// Trust and lifecycle (OKF v0.2 §5): always present so agents can
		// branch on them without probing for key existence.
		"status":     fm.EffectiveStatus(),
		"trust_tier": fm.TrustTier(),
		"stale":      fm.IsStale(time.Now()),
	}
	if fm.Generated != nil {
		out["generated"] = map[string]any{"by": fm.Generated.By, "at": fm.Generated.At}
	}
	if len(fm.Verified) > 0 {
		verified := make([]map[string]any, 0, len(fm.Verified))
		for _, v := range fm.Verified {
			verified = append(verified, map[string]any{"by": v.By, "at": v.At})
		}
		out["verified"] = verified
	}
	if fm.StaleAfter != "" {
		out["stale_after"] = fm.StaleAfter
	}
	if len(fm.Sources) > 0 {
		sources := make([]map[string]any, 0, len(fm.Sources))
		for _, s := range fm.Sources {
			src := map[string]any{"resource": s.Resource}
			if s.ID != "" {
				src["id"] = s.ID
			}
			if s.Title != "" {
				src["title"] = s.Title
			}
			if s.Author != "" {
				src["author"] = s.Author
			}
			if s.UsageCount != nil {
				src["usage_count"] = *s.UsageCount
			}
			if s.LastModified != "" {
				src["last_modified"] = s.LastModified
			}
			if s.UsageWindow != nil {
				src["usage_window"] = map[string]string{"from": s.UsageWindow.From, "to": s.UsageWindow.To}
			}
			sources = append(sources, src)
		}
		out["sources"] = sources
	}
	if fm.UsageWindow != nil {
		out["usage_window"] = map[string]string{"from": fm.UsageWindow.From, "to": fm.UsageWindow.To}
	}
	// Attested Computation contract (OKF v0.2 §10).
	if fm.Runtime != "" {
		out["runtime"] = fm.Runtime
	}
	if len(fm.Parameters) > 0 {
		params := make([]map[string]any, 0, len(fm.Parameters))
		for _, p := range fm.Parameters {
			params = append(params, map[string]any{"name": p.Name, "type": p.Type, "required": p.Required})
		}
		out["parameters"] = params
	}
	if fm.Computation != "" {
		out["computation"] = fm.Computation
	}
	if fm.Executor != nil {
		out["executor"] = map[string]any{"resource": fm.Executor.Resource, "receipt": fm.Executor.Receipt}
	}
	if fm.Attester != nil {
		out["attester"] = map[string]any{"resource": fm.Attester.Resource}
	}

	outputJSON(map[string]any{
		"command": "show",
		"bundle":  root,
		"concept": out,
	})
}

// --- search ---

func runSearch(args []string) {
	f := search.Filters{}
	var pos []string
	for i := 0; i < len(args); i++ {
		switch args[i] {
		case "--tag":
			if i+1 >= len(args) {
				exitErr(cerr.Usage("--tag requires a value"))
			}
			f.Tag = args[i+1]
			i++
		case "--type":
			if i+1 >= len(args) {
				exitErr(cerr.Usage("--type requires a value"))
			}
			f.Type = args[i+1]
			i++
		case "--text":
			if i+1 >= len(args) {
				exitErr(cerr.Usage("--text requires a value"))
			}
			f.Text = args[i+1]
			i++
		default:
			if strings.HasPrefix(args[i], "-") {
				exitErr(cerr.Usage("unknown search flag: %s", args[i]))
			}
			pos = append(pos, args[i])
		}
	}

	// The first positional argument is the bundle when it is a directory or
	// is written as a path; otherwise every positional is a search term and
	// the default bundle is searched.
	bundlePath := defaultBundle()
	if len(pos) > 0 && (isDir(pos[0]) || looksLikePath(pos[0])) {
		bundlePath, pos = pos[0], pos[1:]
	}
	f.Terms = pos

	b, lerr := loadBundle(bundlePath)
	if lerr != nil {
		exitErr(lerr)
	}

	hits := search.Find(b, f)
	if humanOut {
		printHits(hits, f.FoldedTerms())
		return
	}
	concepts := make([]map[string]any, 0, len(hits))
	for _, h := range hits {
		r := map[string]any{
			"id":    h.Concept.ID,
			"type":  h.Concept.Frontmatter.Type,
			"title": h.Concept.DisplayTitle(),
		}
		if h.Lines != nil {
			lines := h.Lines
			if len(lines) > maxJSONLines {
				lines = lines[:maxJSONLines]
			}
			r["matches"] = lines
			r["match_count"] = len(h.Lines)
		}
		concepts = append(concepts, r)
	}

	outputJSON(map[string]any{
		"command": "search",
		"bundle":  b.Root,
		"filters": f,
		"results": concepts,
		"count":   len(hits),
	})
}

// maxJSONLines caps the matching lines reported per search result.
const maxJSONLines = 5

// defaultBundle is the bundle used when a command is given none.
func defaultBundle() string {
	if b := os.Getenv("OKF_BUNDLE"); b != "" {
		return b
	}
	return "."
}

// isMarkdownFile reports whether p names an existing regular .md file.
func isMarkdownFile(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.Mode().IsRegular() && strings.HasSuffix(strings.ToLower(p), ".md")
}

func isDir(p string) bool {
	info, err := os.Stat(p)
	return err == nil && info.IsDir()
}

// looksLikePath reports whether arg is written as a filesystem path, so a
// mistyped bundle path is reported as missing instead of searched for.
func looksLikePath(arg string) bool {
	return strings.ContainsRune(arg, '/') || strings.HasPrefix(arg, ".") || strings.HasPrefix(arg, "~")
}

// --- init ---

func runInit(args []string) {
	if err := rejectFlags("init", args); err != nil {
		exitErr(err)
	}
	if len(args) == 0 {
		exitErr(cerr.Usage("usage: okf init <bundle-path>"))
	}
	dir := args[0]
	if err := initbundle.Create(dir); err != nil {
		exitErr(cerr.IO(err, "create bundle %s", dir))
	}

	var created []string
	for _, sub := range []string{"tables", "datasets", "playbooks"} {
		created = append(created, sub+"/")
	}

	outputJSON(map[string]any{
		"command":   "init",
		"bundle":    dir,
		"created":   created,
		"index":     "index.md",
		"gitignore": ".gitignore",
	})
}

// --- backlinks ---

func runBacklinks(args []string) {
	if err := rejectFlags("backlinks", args); err != nil {
		exitErr(err)
	}
	if len(args) == 1 {
		args = []string{defaultBundle(), args[0]}
	}
	if len(args) < 2 {
		exitErr(cerr.Usage("usage: okf backlinks [bundle] <concept-id>"))
	}
	b := mustBundle("backlinks", args[:1])
	conceptID := strings.TrimSuffix(args[1], ".md")
	links := backlinks.Backlinks(b, conceptID)
	if humanOut {
		printIDs(links)
		return
	}

	if links == nil {
		links = []string{} // emit empty array, not null
	}

	outputJSON(map[string]any{
		"command":    "backlinks",
		"bundle":     b.Root,
		"concept_id": conceptID,
		"backlinks":  links,
		"count":      len(links),
	})
}

// --- schema ---

// schemaFlag describes a single flag for machine consumption.
type schemaFlag struct {
	Name        string `json:"name"`
	Short       string `json:"short,omitempty"`
	Type        string `json:"type"`
	Default     string `json:"default"`
	Description string `json:"description"`
}

// schemaArg describes a positional argument.
type schemaArg struct {
	Name     string `json:"name"`
	Required bool   `json:"required"`
}

// schemaCommand describes a single CLI command for machine consumption.
type schemaCommand struct {
	Name      string       `json:"name"`
	Short     string       `json:"short"`
	Long      string       `json:"long,omitempty"`
	Flags     []schemaFlag `json:"flags"`
	Args      []schemaArg  `json:"args"`
	Stdout    string       `json:"stdout"`
	ExitCodes []int        `json:"exit_codes"`
}

// schemaRoot is the top-level schema output.
type schemaRoot struct {
	Name        string             `json:"name"`
	Version     string             `json:"version"`
	Description string             `json:"description"`
	Commands    []schemaCommand    `json:"commands"`
	ExitCodes   []cerr.ExitCodeDoc `json:"exit_codes"`
}

func runSchema(args []string) {
	// `okf schema <command>` describes a single command.
	if len(args) > 0 {
		cmd := findSchemaCommand(args[0])
		if cmd == nil {
			exitErr(cerr.Usage("unknown command %q", args[0]))
		}
		outputJSON(cmd)
		return
	}
	outputJSON(buildSchemaRoot())
}

func buildSchemaRoot() schemaRoot {
	return schemaRoot{
		Name:        "okf",
		Version:     version,
		Description: "Go CLI toolkit for the Open Knowledge Format (OKF)",
		Commands:    allSchemaCommands(),
		ExitCodes:   cerr.ExitCodeDocs,
	}
}

func allSchemaCommands() []schemaCommand {
	return []schemaCommand{
		{
			Name:      "schema",
			Short:     "Print machine-readable CLI metadata as JSON",
			Long:      "Outputs a JSON document describing every command, its flags, arguments, output format, and exit codes. Pass a command name to describe just that command.",
			Args:      []schemaArg{{Name: "command", Required: false}},
			Stdout:    "json",
			ExitCodes: []int{cerr.ExitCodeOK, cerr.ExitCodeUsage},
		},
		{
			Name:      "init",
			Short:     "Create a new empty OKF bundle",
			Long:      "Creates a bundle directory with standard subdirectories (tables, datasets, playbooks), a root index.md, and a .gitignore. Fails if the directory already exists.",
			Args:      []schemaArg{{Name: "bundle", Required: true}},
			Stdout:    "json",
			ExitCodes: []int{cerr.ExitCodeOK, cerr.ExitCodeValidation, cerr.ExitCodeIO, cerr.ExitCodeUsage},
		},
		{
			Name:  "validate",
			Short: "Validate a bundle against the OKF spec",
			Long:  "Checks every concept against OKF v0.2: required frontmatter (type), recommended fields (title, description, tags), non-empty body, valid cross-links, the provenance/trust/lifecycle families (sources, generated, verified, status, stale_after), the Attested Computation contract (runtime, parameters, computation, executor, attester), reserved-file structure (index.md, log.md), and legacy v0.1 constructs (timestamp, # Citations). Every finding carries a stable rule ID (okf/<family>/<check>). With --format sarif, stdout is a SARIF 2.1.0 document for CI code scanning. Exits 1 if any errors are found, unless --exit-zero is set. JSON output: findings is the list (concept_id, rule, severity, message); errors and warnings are counts; valid is true when there are no errors. By default notes without frontmatter or type get a single warning and [[wikilinks]] are not checked, so a plain Obsidian vault validates; --strict answers whether the bundle is correct OKF.",
			Flags: []schemaFlag{
				{Name: "strict", Type: "bool", Default: "false", Description: "check as OKF only: notes without frontmatter or type are errors with the full field checks, and [[wikilinks]] are reported. By default such notes get one warning and wikilinks are not checked"},
				{Name: "format", Type: "string", Default: "json", Description: "output format: json or sarif (SARIF 2.1.0)"},
				{Name: "exit-zero", Type: "bool", Default: "false", Description: "always exit 0, even when errors are found (for CI steps that upload findings after the run)"},
			},
			Args:      []schemaArg{{Name: "bundle", Required: true}},
			Stdout:    "json|sarif",
			ExitCodes: []int{cerr.ExitCodeOK, cerr.ExitCodeValidation, cerr.ExitCodeIO, cerr.ExitCodeUsage},
		},
		{
			Name:  "lint",
			Short: "Check recommended fields and style (warnings only)",
			Long:  "Same checks as validate but only emits warnings - errors are suppressed. Findings carry the same stable rule IDs, and --format sarif emits SARIF 2.1.0. Exits 0 even with warnings. JSON output: findings is the list; warnings is a count.",
			Flags: []schemaFlag{
				{Name: "strict", Type: "bool", Default: "false", Description: "check as OKF only: notes without frontmatter or type are errors with the full field checks, and [[wikilinks]] are reported. By default such notes get one warning and wikilinks are not checked"},
				{Name: "format", Type: "string", Default: "json", Description: "output format: json or sarif (SARIF 2.1.0)"},
			},
			Args:      []schemaArg{{Name: "bundle", Required: true}},
			Stdout:    "json|sarif",
			ExitCodes: []int{cerr.ExitCodeOK, cerr.ExitCodeValidation, cerr.ExitCodeIO, cerr.ExitCodeUsage},
		},
		{
			Name:      "index",
			Short:     "Generate index.md files (progressive disclosure)",
			Long:      "Writes index.md into every directory containing concept documents, providing progressive disclosure per OKF spec §6.",
			Args:      []schemaArg{{Name: "bundle", Required: true}},
			Stdout:    "json",
			ExitCodes: []int{cerr.ExitCodeOK, cerr.ExitCodeValidation, cerr.ExitCodeIO, cerr.ExitCodeUsage},
		},
		{
			Name:      "list",
			Short:     "List all concepts in the bundle",
			Long:      "Lists every concept document with its ID, type, title, lifecycle status, and trust tier (unverified, machine-confirmed, or human-reviewed).",
			Args:      []schemaArg{{Name: "bundle", Required: true}},
			Stdout:    "json",
			ExitCodes: []int{cerr.ExitCodeOK, cerr.ExitCodeValidation, cerr.ExitCodeIO, cerr.ExitCodeUsage},
		},
		{
			Name:  "show",
			Short: "Show a single concept's full content",
			Flags: []schemaFlag{
				{Name: "raw", Type: "bool", Default: "false", Description: "human output only: print the markdown body as written instead of rendering it"},
				{Name: "no-pager", Type: "bool", Default: "false", Description: "human output only: do not page long output through $OKF_PAGER / $PAGER / less"},
			},
			Long: "On a terminal the page is rendered and long pages open in a pager; with less, press v to open the page's file in the editor. Accepts a concept ID or a path to an existing .md file; a file path is read directly without loading the rest of the bundle, and a file without frontmatter is shown with empty frontmatter. Returns the concept's ID, file path, frontmatter (type, title, description, resource, tags), trust and lifecycle state (status, trust_tier, stale, generated, verified, stale_after), provenance (sources, usage_window), the Attested Computation contract when present (runtime, parameters, computation, executor, attester), and markdown body as JSON.",
			Args: []schemaArg{
				{Name: "bundle", Required: true},
				{Name: "concept-id", Required: true},
			},
			Stdout:    "json",
			ExitCodes: []int{cerr.ExitCodeOK, cerr.ExitCodeValidation, cerr.ExitCodeIO, cerr.ExitCodeUsage},
		},
		{
			Name:  "search",
			Short: "Search concepts by tag, type, or text",
			Long:  "Filters concepts in a bundle by tag (--tag), frontmatter type (--type), or full-text search in title, description, and body (positional terms and/or --text). Positional terms are split on whitespace (including the full-width space); --text is matched as one phrase. Text matching is substring-based, so it works for languages written without spaces such as Japanese; it ignores case, full-width/half-width ASCII, and katakana/hiragana. Multiple terms and filters are AND-combined. With text terms, results are ranked (title matches first, then by number of matching lines) and each carries up to 5 matching body lines with file line numbers (matches) and the total (match_count). The first positional argument is taken as the bundle when it is a directory or contains a slash; otherwise the bundle is $OKF_BUNDLE or the current directory. With no filters, returns all concepts.",
			Flags: []schemaFlag{
				{Name: "tag", Type: "string", Default: "", Description: "filter by tag (case-insensitive)"},
				{Name: "type", Type: "string", Default: "", Description: "filter by frontmatter type (case-insensitive)"},
				{Name: "text", Type: "string", Default: "", Description: "full-text search in title, description, and body (case-insensitive)"},
			},
			Args: []schemaArg{
				{Name: "bundle", Required: false},
				{Name: "terms", Required: false},
			},
			Stdout:    "json",
			ExitCodes: []int{cerr.ExitCodeOK, cerr.ExitCodeValidation, cerr.ExitCodeIO, cerr.ExitCodeUsage},
		},
		{
			Name:  "new",
			Short: "Create a new concept page",
			Long:  "Writes <bundle>/<concept-id>.md with OKF frontmatter (type, title, description and tags, empty unless given, optional status, generated.by/at from $OKF_AUTHOR or human:$USER) and either a title heading or, with --body, the given markdown. One call creates a finished page: okf new notes/x --type Note --title X --description ... --tag a,b --body - < body.md. Never overwrites: an existing file is a validation error. On a terminal the page is then opened in $VISUAL/$EDITOR unless --no-edit is given; otherwise it prints JSON with the new path. With one positional argument the bundle is $OKF_BUNDLE or the current directory.",
			Flags: []schemaFlag{
				{Name: "title", Type: "string", Default: "<last segment of concept-id>", Description: "concept title"},
				{Name: "type", Type: "string", Default: defaultNewType, Description: "frontmatter type"},
				{Name: "tag", Type: "string", Default: "", Description: "comma-separated tags; may be repeated"},
				{Name: "description", Type: "string", Default: "", Description: "one-line description"},
				{Name: "status", Type: "string", Default: "", Description: "lifecycle status: draft, stable, or deprecated"},
				{Name: "body", Type: "string", Default: "", Description: "page body as markdown, written as given (no title heading is added); - reads it from stdin. A frontmatter block at its top is merged into the page's frontmatter (flags win over it), so any extra keys can be set. Implies --no-edit"},
				{Name: "template", Type: "string", Default: "", Description: "start from a template: a name looked up in $OKF_TEMPLATES, <bundle>/Templates or <bundle>/templates, or a path. Placeholders {{title}}, {{date}}, {{time}}, {{date:YYYY-MM-DD}}, {{date+1d:...}} (d, w, M, y) are filled in; dates are relative to the page's date when its title is YYYY-MM-DD, else to now. Its frontmatter is merged like --body's (template < --body < flags), and --body is appended after it"},
				{Name: "no-edit", Type: "bool", Default: "false", Description: "do not open an editor"},
			},
			Args: []schemaArg{
				{Name: "bundle", Required: false},
				{Name: "concept-id", Required: true},
			},
			Stdout:    "json",
			ExitCodes: []int{cerr.ExitCodeOK, cerr.ExitCodeValidation, cerr.ExitCodeIO, cerr.ExitCodeUsage},
		},
		{
			Name:  "backlinks",
			Short: "List concepts that link to a given concept",
			Long:  "Returns the IDs of all concepts in the bundle that contain a markdown link to the specified concept. Deduplicates multiple links from the same source.",
			Args: []schemaArg{
				{Name: "bundle", Required: true},
				{Name: "concept-id", Required: true},
			},
			Stdout:    "json",
			ExitCodes: []int{cerr.ExitCodeOK, cerr.ExitCodeValidation, cerr.ExitCodeIO, cerr.ExitCodeUsage},
		},
		{
			Name:      "graph",
			Short:     "Print cross-link graph statistics",
			Long:      "Builds the directed cross-link graph from concept markdown links and prints nodes, edges, and summary statistics.",
			Args:      []schemaArg{{Name: "bundle", Required: true}},
			Stdout:    "json",
			ExitCodes: []int{cerr.ExitCodeOK, cerr.ExitCodeValidation, cerr.ExitCodeIO, cerr.ExitCodeUsage},
		},
		{
			Name:  "edit",
			Short: "Open a page in $EDITOR",
			Long:  "Opens the page's file in $VISUAL, $EDITOR, or the first of nvim, vim, vi, nano. Accepts a concept ID or a path to an .md file. Without a terminal (agents, pipes) nothing is opened and the page's path is printed as JSON (opened: false). Inside okf show's pager (less), pressing v opens the shown page the same way.",
			Args: []schemaArg{
				{Name: "bundle", Required: false},
				{Name: "concept-id", Required: true},
			},
			Stdout:    "json",
			ExitCodes: []int{cerr.ExitCodeOK, cerr.ExitCodeValidation, cerr.ExitCodeIO, cerr.ExitCodeUsage},
		},
		{
			Name:      "completion",
			Short:     "Print a shell completion script",
			Long:      "Prints a zsh or bash completion script that completes commands, flags, page IDs (show, backlinks), folders (new), tags (--tag) and types (--type) from the bundle in $OKF_BUNDLE or the current directory. Load it with: source <(okf completion zsh).",
			Args:      []schemaArg{{Name: "shell", Required: true}},
			Stdout:    "text",
			ExitCodes: []int{cerr.ExitCodeOK, cerr.ExitCodeUsage},
		},
		{
			Name:      "version",
			Short:     "Print version",
			Stdout:    "json",
			ExitCodes: []int{cerr.ExitCodeOK},
		},
	}
}

func findSchemaCommand(name string) *schemaCommand {
	for _, c := range allSchemaCommands() {
		if c.Name == name {
			return &c
		}
	}
	return nil
}
