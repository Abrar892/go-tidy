package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCategoryKnownExtensions(t *testing.T) {
	cl := newClassifier(nil)
	cases := map[string]string{
		"photo.JPG":   "images",
		"report.pdf":  "documents",
		"data.xlsx":   "spreadsheets",
		"deck.pptx":   "presentations",
		"song.flac":   "audio",
		"clip.mkv":    "video",
		"backup.zip":  "archives",
		"main.go":     "code",
		"font.woff2":  "fonts",
		"setup.msi":   "installers",
		"notes.txt":   "documents",
		"archive.tar": "archives",
		"mystery.xyz": "other",
		"noextension": "other",
		".hidden":     "other",
		"UPPER.PNG":   "images",
		"script.test": "other",
	}
	for name, want := range cases {
		if got := cl.category(name); got != want {
			t.Errorf("category(%q) = %q, want %q", name, got, want)
		}
	}
}

func TestCategoryExtraRulesOverride(t *testing.T) {
	cl := newClassifier(map[string]string{"blend": "3d-models", ".PSD": "design", "pdf": "ebooks"})
	if got := cl.category("scene.blend"); got != "3d-models" {
		t.Errorf("custom rule: got %q, want 3d-models", got)
	}
	if got := cl.category("art.psd"); got != "design" {
		t.Errorf("dotted custom rule: got %q, want design", got)
	}
	if got := cl.category("book.pdf"); got != "ebooks" {
		t.Errorf("override of built-in: got %q, want ebooks", got)
	}
}

func TestLoadRulesErrors(t *testing.T) {
	dir := t.TempDir()
	bad := filepath.Join(dir, "bad.json")
	if err := os.WriteFile(bad, []byte(`{"a": }`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRules(bad); err == nil {
		t.Error("expected error for malformed JSON")
	}
	empty := filepath.Join(dir, "empty.json")
	if err := os.WriteFile(empty, []byte(`{"a": ""}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := LoadRules(empty); err == nil {
		t.Error("expected error for empty category")
	}
	if _, err := LoadRules(filepath.Join(dir, "missing.json")); err == nil {
		t.Error("expected error for missing file")
	}
}

// seed creates files with the given relative paths and contents.
func seed(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for name, content := range files {
		p := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestOrganizeMovesFilesByCategory(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir, map[string]string{
		"photo.jpg":  "img",
		"report.pdf": "doc",
		"song.mp3":   "aud",
		"notes.txt":  "txt",
		"weird.xyz":  "???",
		"README":     "no ext",
	})

	report, err := Organize(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Moved != 6 {
		t.Fatalf("moved = %d, want 6", report.Moved)
	}
	want := map[string]string{
		"images/photo.jpg":     "img",
		"documents/report.pdf": "doc",
		"audio/song.mp3":       "aud",
		"documents/notes.txt":  "txt",
		"other/weird.xyz":      "???",
		"other/README":         "no ext",
	}
	for path, content := range want {
		got, err := os.ReadFile(filepath.Join(dir, path))
		if err != nil {
			t.Errorf("expected file %s: %v", path, err)
			continue
		}
		if string(got) != content {
			t.Errorf("%s content = %q, want %q", path, got, content)
		}
	}
	// Nothing left loose in the root except the journal.
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		if !e.IsDir() && e.Name() != journalFile {
			t.Errorf("unexpected loose file in root: %s", e.Name())
		}
	}
}

func TestOrganizeDryRunMovesNothing(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir, map[string]string{"a.jpg": "x", "b.pdf": "y"})

	report, err := Organize(dir, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	if !report.DryRun {
		t.Error("report should be marked dry-run")
	}
	if len(plannedMoves(report)) != 2 {
		t.Fatalf("planned moves = %d, want 2", len(plannedMoves(report)))
	}
	for _, name := range []string{"a.jpg", "b.pdf"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("dry run moved %s", name)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, journalFile)); !os.IsNotExist(err) {
		t.Error("dry run must not write a journal")
	}
}

func TestOrganizeNameClashGetsSuffix(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir, map[string]string{
		"images/photo.jpg": "existing",
		"photo.jpg":        "new",
		"photo (1).jpg":    "other",
	})
	// images/ already holds photo.jpg, so the loose photo.jpg must become
	// photo (1).jpg — but that name is also taken, so it becomes (2).
	report, err := Organize(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Moved != 2 {
		t.Fatalf("moved = %d, want 2", report.Moved)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "images/photo (2).jpg")); string(got) != "new" {
		t.Errorf("clash resolution wrong: images/photo (2).jpg = %q", got)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "images/photo (1).jpg")); string(got) != "other" {
		t.Errorf("photo (1).jpg should keep its own content, got %q", got)
	}
}

func TestOrganizeSkipsHiddenAndJournal(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir, map[string]string{".secret": "s", "visible.jpg": "v"})
	report, err := Organize(dir, Options{})
	if err != nil {
		t.Fatal(err)
	}
	if report.Moved != 1 {
		t.Fatalf("moved = %d, want 1 (hidden file skipped)", report.Moved)
	}
	if _, err := os.Stat(filepath.Join(dir, ".secret")); err != nil {
		t.Error("hidden file should stay put")
	}
	// Second run (recursive, so it sees the organized file): the journal
	// and the already-organized file must not move.
	report2, err := Organize(dir, Options{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if report2.Moved != 0 || report2.Unchanged != 1 {
		t.Errorf("second run: moved=%d unchanged=%d, want 0/1", report2.Moved, report2.Unchanged)
	}
}

func TestOrganizeIncludeHidden(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir, map[string]string{".env": "k=v"})
	report, err := Organize(dir, Options{IncludeHidden: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Moved != 1 {
		t.Fatalf("moved = %d, want 1", report.Moved)
	}
	if _, err := os.Stat(filepath.Join(dir, "other/.env")); err != nil {
		t.Errorf("hidden file should move to other/: %v", err)
	}
}

func TestOrganizeExcludes(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir, map[string]string{"keep.jpg": "k", "skip.jpg": "s", "skip.pdf": "s2"})
	report, err := Organize(dir, Options{Excludes: []string{"skip.*"}})
	if err != nil {
		t.Fatal(err)
	}
	if report.Moved != 1 || report.Skipped != 2 {
		t.Fatalf("moved=%d skipped=%d, want 1/2", report.Moved, report.Skipped)
	}
	if _, err := os.Stat(filepath.Join(dir, "skip.jpg")); err != nil {
		t.Error("excluded file should stay put")
	}
}

func TestOrganizeByDate(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir, map[string]string{"old.jpg": "o", "new.jpg": "n"})
	old := time.Date(2023, 4, 2, 10, 0, 0, 0, time.UTC)
	newT := time.Date(2026, 11, 24, 10, 0, 0, 0, time.UTC)
	_ = os.Chtimes(filepath.Join(dir, "old.jpg"), old, old)
	_ = os.Chtimes(filepath.Join(dir, "new.jpg"), newT, newT)

	report, err := Organize(dir, Options{ByDate: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Moved != 2 {
		t.Fatalf("moved = %d, want 2", report.Moved)
	}
	for _, p := range []string{"2023-04/old.jpg", "2026-11/new.jpg"} {
		if _, err := os.Stat(filepath.Join(dir, p)); err != nil {
			t.Errorf("expected %s: %v", p, err)
		}
	}
}

func TestOrganizeRecursiveSkipsManagedDirs(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir, map[string]string{
		"sub/nested.jpg":     "n",
		"images/already.jpg": "a",
		"sub/deep/more.pdf":  "d",
	})
	report, err := Organize(dir, Options{Recursive: true})
	if err != nil {
		t.Fatal(err)
	}
	if report.Moved != 2 {
		t.Fatalf("moved = %d, want 2 (already-organized file untouched)", report.Moved)
	}
	if report.Unchanged != 1 {
		t.Fatalf("unchanged = %d, want 1 (images/already.jpg)", report.Unchanged)
	}
	if _, err := os.Stat(filepath.Join(dir, "images/nested.jpg")); err != nil {
		t.Errorf("nested.jpg should land in images/: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dir, "documents/more.pdf")); err != nil {
		t.Errorf("more.pdf should land in documents/: %v", err)
	}
	if got, _ := os.ReadFile(filepath.Join(dir, "images/already.jpg")); string(got) != "a" {
		t.Error("already-organized file must not be re-moved")
	}
}

func TestOrganizeNotADirectory(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "file.txt")
	if err := os.WriteFile(f, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Organize(f, Options{}); err == nil {
		t.Error("expected error for non-directory root")
	}
	if _, err := Organize(filepath.Join(dir, "missing"), Options{}); err == nil {
		t.Error("expected error for missing root")
	}
}

func TestUndoRestoresFiles(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir, map[string]string{"a.jpg": "A", "b.pdf": "B"})

	if _, err := Organize(dir, Options{}); err != nil {
		t.Fatal(err)
	}
	report, err := Undo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Moved != 2 {
		t.Fatalf("restored = %d, want 2", report.Moved)
	}
	for _, name := range []string{"a.jpg", "b.pdf"} {
		if _, err := os.Stat(filepath.Join(dir, name)); err != nil {
			t.Errorf("%s not restored: %v", name, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dir, journalFile)); !os.IsNotExist(err) {
		t.Error("journal should be deleted after undo")
	}
	// Undoing again must fail: there is no journal left.
	if _, err := Undo(dir); err == nil {
		t.Error("second undo should fail with no journal")
	}
}

func TestUndoSkipsMissingOrOccupied(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir, map[string]string{"a.jpg": "A", "b.pdf": "B"})
	if _, err := Organize(dir, Options{}); err != nil {
		t.Fatal(err)
	}
	// Delete one organized file; occupy the other's original spot.
	if err := os.Remove(filepath.Join(dir, "images/a.jpg")); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "b.pdf"), []byte("squat"), 0o644); err != nil {
		t.Fatal(err)
	}
	report, err := Undo(dir)
	if err != nil {
		t.Fatal(err)
	}
	if report.Moved != 0 || report.Skipped != 2 {
		t.Fatalf("restored=%d skipped=%d, want 0/2", report.Moved, report.Skipped)
	}
}

func TestJournalRoundTrip(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir, map[string]string{"x.png": "x"})
	if _, err := Organize(dir, Options{}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(dir, journalFile))
	if err != nil {
		t.Fatal(err)
	}
	var j journal
	if err := json.Unmarshal(data, &j); err != nil {
		t.Fatalf("journal is not valid JSON: %v", err)
	}
	if len(j.Moves) != 1 || j.Moves[0].From != "x.png" || j.Moves[0].To != "images/x.png" {
		t.Errorf("journal moves = %+v, want one x.png -> images/x.png", j.Moves)
	}
}

func TestReportJSONShape(t *testing.T) {
	dir := t.TempDir()
	seed(t, dir, map[string]string{"a.jpg": "x"})
	report, err := Organize(dir, Options{DryRun: true})
	if err != nil {
		t.Fatal(err)
	}
	data, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(data), `"dry_run":true`) {
		t.Errorf("JSON report should flag dry_run: %s", data)
	}
}
