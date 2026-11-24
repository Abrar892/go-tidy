// Package main implements tidy, a small CLI that organizes a messy
// directory by moving its files into categorized subfolders.
package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
)

const usage = `tidy — organize a messy directory into categorized folders.

Usage:
  tidy [flags] <directory>
  tidy undo [flags] <directory>

Commands:
  (default)   Move files in <directory> into category folders.
  undo        Reverse the most recent tidy run in <directory>.

Flags:
  -by-date          Group by modification year-month (YYYY-MM) instead of category.
  -recursive        Also organize files in subdirectories.
  -dry-run          Show the plan without moving anything.
  -json             Print the plan/report as JSON instead of a table.
  -exclude glob     Skip files matching glob (repeatable).
  -include-hidden   Also organize hidden files (dotfiles are skipped by default).
  -config path      Load extra extension→category rules from a JSON file.
  -categories       Print the built-in category table and exit.
  -quiet            Only print errors and the final summary line.
  -h, -help         Show this help.

Categories are built from the file extension (images, documents, audio,
video, archives, code, ...). Files with no known extension go to "other".
When the destination already has a file with the same name, tidy appends
" (1)", " (2)", ... before the extension instead of overwriting.

Every run writes a journal (.tidy-undo.json) in the target directory so
"tidy undo" can put everything back.
`

func main() {
	if len(os.Args) > 1 && (os.Args[1] == "undo" || os.Args[1] == "-h" || os.Args[1] == "-help" || os.Args[1] == "--help") {
		if os.Args[1] == "undo" {
			runUndo(os.Args[2:])
			return
		}
		fmt.Print(usage)
		return
	}
	runOrganize(os.Args[1:])
}

func runOrganize(args []string) {
	fs := flag.NewFlagSet("tidy", flag.ExitOnError)
	byDate := fs.Bool("by-date", false, "")
	recursive := fs.Bool("recursive", false, "")
	dryRun := fs.Bool("dry-run", false, "")
	jsonOut := fs.Bool("json", false, "")
	includeHidden := fs.Bool("include-hidden", false, "")
	configPath := fs.String("config", "", "")
	showCategories := fs.Bool("categories", false, "")
	quiet := fs.Bool("quiet", false, "")
	var excludes multiFlag
	fs.Var(&excludes, "exclude", "")
	fs.Usage = func() { fmt.Print(usage) }
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}

	if *showCategories {
		printCategories()
		return
	}
	if fs.NArg() != 1 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	root := fs.Arg(0)

	opts := Options{
		ByDate:        *byDate,
		Recursive:     *recursive,
		DryRun:        *dryRun,
		JSON:          *jsonOut,
		IncludeHidden: *includeHidden,
		Excludes:      excludes,
		Quiet:         *quiet,
	}
	if *configPath != "" {
		rules, err := LoadRules(*configPath)
		if err != nil {
			fatal(err)
		}
		opts.ExtraRules = rules
	}

	report, err := Organize(root, opts)
	if err != nil {
		fatal(err)
	}
	if *jsonOut {
		if err := PrintJSON(report); err != nil {
			fatal(err)
		}
		return
	}
	PrintReport(report, *dryRun, *quiet)
}

func runUndo(args []string) {
	fs := flag.NewFlagSet("tidy undo", flag.ExitOnError)
	jsonOut := fs.Bool("json", false, "")
	quiet := fs.Bool("quiet", false, "")
	fs.Usage = func() { fmt.Print(usage) }
	if err := fs.Parse(args); err != nil {
		os.Exit(2)
	}
	if fs.NArg() != 1 {
		fmt.Fprint(os.Stderr, usage)
		os.Exit(2)
	}
	report, err := Undo(fs.Arg(0))
	if err != nil {
		fatal(err)
	}
	if *jsonOut {
		if err := PrintJSON(report); err != nil {
			fatal(err)
		}
		return
	}
	PrintUndoReport(report, *quiet)
}

// multiFlag collects repeatable -exclude values.
type multiFlag []string

func (m *multiFlag) String() string     { return strings.Join(*m, ",") }
func (m *multiFlag) Set(v string) error { *m = append(*m, v); return nil }

func fatal(err error) {
	fmt.Fprintf(os.Stderr, "tidy: %v\n", err)
	os.Exit(1)
}
