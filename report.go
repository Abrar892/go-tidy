package main

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// PrintReport renders a human-readable summary of an organize run.
func PrintReport(r *Report, dryRun, quiet bool) {
	if dryRun {
		fmt.Printf("Dry run — nothing moved. Plan for %s:\n", r.Root)
	} else {
		fmt.Printf("Organized %s:\n", r.Root)
	}
	if !quiet {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, m := range r.Moves {
			switch {
			case m.Skipped && m.Reason == "already organized":
				fmt.Fprintf(w, "  =\t%s\t(already in %s)\n", m.From, m.Category)
			case m.Skipped:
				fmt.Fprintf(w, "  -\t%s\t(%s)\n", m.From, m.Reason)
			default:
				fmt.Fprintf(w, "  ->\t%s\t%s\n", m.From, m.To)
			}
		}
		_ = w.Flush()
	}
	fmt.Println(summaryLine(r))
}

// PrintUndoReport renders a human-readable summary of an undo.
func PrintUndoReport(r *Report, quiet bool) {
	fmt.Printf("Undid tidy run in %s:\n", r.Root)
	if !quiet {
		w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
		for _, m := range r.Moves {
			if m.Skipped {
				fmt.Fprintf(w, "  -\t%s\t(%s)\n", m.From, m.Reason)
			} else {
				fmt.Fprintf(w, "  <-\t%s\t%s\n", m.From, m.To)
			}
		}
		_ = w.Flush()
	}
	fmt.Printf("Restored %d file(s)", r.Moved)
	if r.Skipped > 0 {
		fmt.Printf(", %d could not be restored", r.Skipped)
	}
	fmt.Println(".")
}

func summaryLine(r *Report) string {
	var b strings.Builder
	if r.DryRun {
		fmt.Fprintf(&b, "Would move %d file(s)", len(plannedMoves(r)))
	} else {
		fmt.Fprintf(&b, "Moved %d file(s)", r.Moved)
	}
	if r.Unchanged > 0 {
		fmt.Fprintf(&b, ", %d already organized", r.Unchanged)
	}
	if r.Skipped > 0 {
		fmt.Fprintf(&b, ", %d skipped", r.Skipped)
	}
	b.WriteString(".")
	return b.String()
}

func plannedMoves(r *Report) []Move {
	var out []Move
	for _, m := range r.Moves {
		if !m.Skipped {
			out = append(out, m)
		}
	}
	return out
}

// PrintJSON writes the report as indented JSON to stdout.
func PrintJSON(r *Report) error {
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	return enc.Encode(r)
}
