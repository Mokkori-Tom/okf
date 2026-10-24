package main

import (
	"os"
	"path/filepath"
	"strings"

	"github.com/okfcli/okf/internal/cerr"
	"github.com/okfcli/okf/internal/show"
)

// runEdit opens a page in the editor: `okf edit [bundle] <concept-id | file.md>`.
// Without a terminal (an agent, a pipe) it opens nothing and prints the
// page's path as JSON, so callers can edit the file themselves.
func runEdit(args []string) {
	if err := rejectFlags("edit", args); err != nil {
		exitErr(err)
	}
	if len(args) == 1 {
		args = []string{defaultBundle(), args[0]}
	}
	if len(args) != 2 {
		exitErr(cerr.Usage("usage: okf edit [bundle] <concept-id | file.md>"))
	}
	var file, id string
	if isMarkdownFile(args[1]) {
		file, _ = filepath.Abs(args[1])
		id = strings.TrimSuffix(filepath.Base(file), ".md")
	} else {
		b := mustBundle("edit", args[:1])
		c, err := show.Show(b, strings.TrimSuffix(args[1], ".md"))
		if err != nil {
			exitErr(cerr.Validation("%s", err))
		}
		file, id = c.Path, c.ID
	}

	if !isTerminal(os.Stdin) || !isTerminal(os.Stdout) {
		outputJSON(map[string]any{"command": "edit", "id": id, "path": file, "opened": false})
		return
	}
	if err := openEditor(file); err != nil {
		exitErr(cerr.IO(err, "open editor for %s", file))
	}
}
