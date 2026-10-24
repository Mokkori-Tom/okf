package main

import (
	"fmt"
	"strings"

	"github.com/okfcli/okf/internal/render"
)

// wantsHelp reports whether args ask for help (-h or --help anywhere).
func wantsHelp(args []string) bool {
	for _, a := range args {
		if a == "-h" || a == "--help" {
			return true
		}
	}
	return false
}

// printCommandHelp prints the help for one command as plain text, built from
// the same metadata as `okf schema <command>`. Help is for reading, so it is
// text even when piped; agents that want JSON use okf schema.
func printCommandHelp(name string) {
	c := findSchemaCommand(name)
	if c == nil {
		printUsage()
		return
	}
	o := render.Options{Width: 80}
	usage := "okf " + c.Name
	for _, a := range c.Args {
		if a.Required {
			usage += " <" + a.Name + ">"
		} else {
			usage += " [" + a.Name + "]"
		}
	}
	if len(c.Flags) > 0 {
		usage += " [flags]"
	}
	fmt.Printf("okf %s - %s\n\nUsage: %s\n\n", c.Name, c.Short, usage)
	if c.Long != "" {
		fmt.Print(render.Text(c.Long, "", o))
		fmt.Println()
	}
	if len(c.Flags) > 0 {
		fmt.Println("Flags:")
		w := 0
		for _, f := range c.Flags {
			w = max(w, len(flagLabel(f)))
		}
		for _, f := range c.Flags {
			desc := f.Description
			if f.Default != "" && f.Default != "false" {
				desc += " (default: " + f.Default + ")"
			}
			lines := strings.Split(strings.TrimRight(render.Text(desc, "", render.Options{Width: 80 - w - 4}), "\n"), "\n")
			for i, l := range lines {
				label := ""
				if i == 0 {
					label = flagLabel(f)
				}
				fmt.Printf("  %-*s  %s\n", w, label, l)
			}
		}
		fmt.Println()
	}
	fmt.Println("Global flags: --json, --human (or OKF_OUTPUT=json|human). Bundle defaults to $OKF_BUNDLE, else the current directory.")
}

func flagLabel(f schemaFlag) string {
	if f.Type == "bool" {
		return "--" + f.Name
	}
	return "--" + f.Name + " " + f.Type
}
