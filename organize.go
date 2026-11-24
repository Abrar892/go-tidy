package main

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
)

// Options controls a single organize run.
type Options struct {
	ByDate        bool              // group by YYYY-MM instead of category
	Recursive     bool              // descend into subdirectories
	DryRun        bool              // plan only, move nothing
	JSON          bool              // unused here; kept so callers can pass flags uniformly
	IncludeHidden bool              // organize dotfiles too
	Excludes      []string          // glob patterns matched against the base name
	ExtraRules    map[string]string // user extension -> category overrides
	Quiet         bool
}

// Move is one planned (or performed) file move.
type Move struct {
	From     string `json:"from"`     // path relative to the target directory
	To       string `json:"to"`       // path relative to the target directory
	Category string `json:"category"` // folder the file lands in
	Skipped  bool   `json:"skipped,omitempty"`
	Reason   string `json:"reason,omitempty"`
}

// Report summarizes a run.
type Report struct {
	Root      string `json:"root"`
	ByDate    bool   `json:"by_date"`
	DryRun    bool   `json:"dry_run"`
	Moves     []Move `json:"moves"`
	Moved     int    `json:"moved"`
	Skipped   int    `json:"skipped"`
	Unchanged int    `json:"unchanged"` // files already in the right folder
}

// Organize scans root, plans moves, and (unless DryRun) performs them and
// writes an undo journal. It never overwrites an existing file: on a name
// clash the moved file gets a " (n)" suffix before its extension.
func Organize(root string, opts Options) (*Report, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}

	cl := newClassifier(opts.ExtraRules)
	plan, err := planMoves(root, opts, cl)
	if err != nil {
		return nil, err
	}

	report := &Report{Root: root, ByDate: opts.ByDate, DryRun: opts.DryRun, Moves: plan}
	for _, m := range plan {
		if m.Skipped {
			if m.Reason == "already organized" {
				report.Unchanged++
			} else {
				report.Skipped++
			}
		}
	}
	if opts.DryRun {
		return report, nil
	}

	moved, err := applyMoves(root, plan)
	if err != nil {
		return report, err
	}
	report.Moved = moved
	if moved > 0 {
		if err := writeJournal(root, plan); err != nil {
			return report, fmt.Errorf("moves applied but journal failed: %w", err)
		}
	}
	return report, nil
}

// planMoves walks root and decides where every eligible file belongs.
func planMoves(root string, opts Options, cl *classifier) ([]Move, error) {
	var moves []Move
	// Track destinations claimed by earlier moves in this plan so two
	// files never collide with each other.
	claimed := map[string]bool{}

	walk := func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root {
				// Skip hidden folders unless asked, and (unless
				// recursive) every other subdirectory. Category folders
				// are descended into on recursive runs: files already
				// in the right folder are reported as already organized
				// and left alone, so tidy never re-sorts its own output.
				if !opts.IncludeHidden && strings.HasPrefix(d.Name(), ".") {
					return filepath.SkipDir
				}
				if !opts.Recursive {
					return filepath.SkipDir
				}
			}
			return nil
		}
		name := d.Name()
		if name == journalFile {
			return nil
		}
		if !opts.IncludeHidden && strings.HasPrefix(name, ".") {
			return nil
		}
		if matchesAny(name, opts.Excludes) {
			moves = append(moves, Move{From: rel(root, path), Skipped: true, Reason: "excluded"})
			return nil
		}

		folder := targetFolder(path, d, opts, cl)
		relDir, _ := filepath.Rel(root, filepath.Dir(path))
		// A file already sitting in its target folder needs no move.
		if relDir == folder {
			moves = append(moves, Move{From: rel(root, path), To: rel(root, path), Category: folder, Skipped: true, Reason: "already organized"})
			return nil
		}
		dest := resolveDest(root, folder, name, claimed)
		claimed[dest] = true
		moves = append(moves, Move{From: rel(root, path), To: rel(root, dest), Category: folder})
		return nil
	}

	if opts.Recursive {
		err := filepath.WalkDir(root, walk)
		if err != nil {
			return nil, err
		}
	} else {
		entries, err := os.ReadDir(root)
		if err != nil {
			return nil, err
		}
		for _, e := range entries {
			if e.IsDir() {
				continue
			}
			if err := walk(filepath.Join(root, e.Name()), e, nil); err != nil {
				return nil, err
			}
		}
	}

	// Stable, readable order: by destination folder, then source name.
	sort.Slice(moves, func(i, j int) bool {
		if moves[i].To != moves[j].To {
			return moves[i].To < moves[j].To
		}
		return moves[i].From < moves[j].From
	})
	return moves, nil
}

// targetFolder decides the destination folder for one file.
func targetFolder(path string, d fs.DirEntry, opts Options, cl *classifier) string {
	if opts.ByDate {
		if info, err := d.Info(); err == nil {
			return info.ModTime().Format("2006-01")
		}
		return time.Now().Format("2006-01")
	}
	return cl.category(d.Name())
}

// resolveDest picks a non-colliding destination inside folder. If
// folder/name already exists on disk or is claimed by an earlier planned
// move, it appends " (1)", " (2)", ... before the extension.
func resolveDest(root, folder, name string, claimed map[string]bool) string {
	candidate := filepath.Join(root, folder, name)
	if !claimed[candidate] {
		if _, err := os.Stat(candidate); errors.Is(err, fs.ErrNotExist) {
			return candidate
		}
	}
	ext := filepath.Ext(name)
	base := strings.TrimSuffix(name, ext)
	for i := 1; ; i++ {
		candidate = filepath.Join(root, folder, fmt.Sprintf("%s (%d)%s", base, i, ext))
		if claimed[candidate] {
			continue
		}
		if _, err := os.Stat(candidate); errors.Is(err, fs.ErrNotExist) {
			return candidate
		}
	}
}

// applyMoves performs the planned moves, creating destination folders as
// needed. It returns how many files were moved; on error it stops and
// reports how far it got.
func applyMoves(root string, plan []Move) (int, error) {
	moved := 0
	for _, m := range plan {
		if m.Skipped {
			continue
		}
		src := filepath.Join(root, m.From)
		dst := filepath.Join(root, m.To)
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return moved, fmt.Errorf("create folder %s: %w", filepath.Dir(dst), err)
		}
		if err := os.Rename(src, dst); err != nil {
			return moved, fmt.Errorf("move %s -> %s: %w", m.From, m.To, err)
		}
		moved++
	}
	return moved, nil
}

// matchesAny reports whether name matches any glob in patterns. A bad
// pattern is treated as never matching (patterns are validated at scan
// time only by their effect, so a typo silently matches nothing rather
// than aborting a run mid-way).
func matchesAny(name string, patterns []string) bool {
	for _, p := range patterns {
		if ok, _ := filepath.Match(p, name); ok {
			return true
		}
	}
	return false
}

func rel(root, path string) string {
	r, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return r
}
