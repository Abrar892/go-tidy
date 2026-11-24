package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// journalFile is the undo journal written into every organized directory.
// It records the moves of the most recent non-dry-run so "tidy undo" can
// reverse them. It is itself skipped by every scan.
const journalFile = ".tidy-undo.json"

// journal is the on-disk form of a run's moves.
type journal struct {
	Version   int       `json:"version"`
	CreatedAt time.Time `json:"created_at"`
	Root      string    `json:"root"`
	Moves     []Move    `json:"moves"`
}

// writeJournal persists the performed moves atomically (temp file + rename)
// so a crash mid-write never leaves a half-written journal.
func writeJournal(root string, plan []Move) error {
	var performed []Move
	for _, m := range plan {
		if !m.Skipped {
			performed = append(performed, m)
		}
	}
	j := journal{Version: 1, CreatedAt: time.Now(), Root: root, Moves: performed}
	data, err := json.MarshalIndent(j, "", "  ")
	if err != nil {
		return err
	}
	tmp := filepath.Join(root, journalFile+".tmp")
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, filepath.Join(root, journalFile))
}

// readJournal loads the undo journal for root.
func readJournal(root string) (*journal, error) {
	data, err := os.ReadFile(filepath.Join(root, journalFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("no tidy journal found in %s — nothing to undo (run tidy there first)", root)
		}
		return nil, err
	}
	var j journal
	if err := json.Unmarshal(data, &j); err != nil {
		return nil, fmt.Errorf("journal %s is corrupt: %w", journalFile, err)
	}
	if j.Version != 1 {
		return nil, fmt.Errorf("journal %s has unsupported version %d", journalFile, j.Version)
	}
	return &j, nil
}

// Undo reverses the most recent tidy run in root: every recorded move is
// played backwards (destination -> source), then the journal is deleted so
// the same run cannot be undone twice. Files that have been moved away or
// deleted since are reported as skipped, not fatal.
func Undo(root string) (*Report, error) {
	info, err := os.Stat(root)
	if err != nil {
		return nil, fmt.Errorf("stat %s: %w", root, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s is not a directory", root)
	}
	j, err := readJournal(root)
	if err != nil {
		return nil, err
	}

	report := &Report{Root: root, Moves: []Move{}}
	// Reverse order: undo the last move first, mirroring how the run ran.
	for i := len(j.Moves) - 1; i >= 0; i-- {
		m := j.Moves[i]
		src := filepath.Join(root, m.To)
		dst := filepath.Join(root, m.From)
		if _, err := os.Stat(src); os.IsNotExist(err) {
			report.Moves = append(report.Moves, Move{From: m.To, To: m.From, Category: m.Category, Skipped: true, Reason: "file no longer at organized location"})
			report.Skipped++
			continue
		}
		if _, err := os.Stat(dst); err == nil {
			report.Moves = append(report.Moves, Move{From: m.To, To: m.From, Category: m.Category, Skipped: true, Reason: "original location occupied"})
			report.Skipped++
			continue
		}
		if err := os.Rename(src, dst); err != nil {
			return report, fmt.Errorf("undo move %s -> %s: %w", m.To, m.From, err)
		}
		report.Moves = append(report.Moves, Move{From: m.To, To: m.From, Category: m.Category})
		report.Moved++
	}

	if err := os.Remove(filepath.Join(root, journalFile)); err != nil && !os.IsNotExist(err) {
		return report, fmt.Errorf("undo completed but journal could not be removed: %w", err)
	}
	removeEmptyCategoryDirs(root)
	return report, nil
}

// removeEmptyCategoryDirs prunes category folders left empty by an undo.
// Errors are ignored: a non-empty or busy folder simply stays.
func removeEmptyCategoryDirs(root string) {
	for _, cat := range categoryOrder {
		if cat == "other" {
			continue
		}
		_ = os.Remove(filepath.Join(root, cat))
	}
}
