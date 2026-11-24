# tidy — a directory organizer CLI

`tidy` takes a messy directory — the Downloads folder we all pretend doesn't
exist — and sorts its files into categorized subfolders: `images/`,
`documents/`, `audio/`, `video/`, `archives/`, `code/`, and friends. It shows
you the plan before it moves anything, never overwrites a file, and can undo
its last run.

## Why this exists

Every "organize my files" tool I tried either moved things silently with no
way back, or clobbered files that shared a name. `tidy` is built around three
rules:

1. **Preview first.** `--dry-run` prints the full plan; nothing moves until
   you say so.
2. **Never overwrite.** If `images/photo.jpg` already exists, the incoming
   file becomes `images/photo (1).jpg` — and `(2)`, `(3)` if needed.
3. **Undo is a first-class feature.** Every run writes a journal
   (`.tidy-undo.json`) in the target directory; `tidy undo` puts everything
   back, and the journal deletes itself so a run can't be undone twice.

## Install

Requires Go 1.23 or later. No third-party dependencies.

```sh
go build -o tidy .
```

## Usage

```sh
# Organize ~/Downloads by category
tidy ~/Downloads

# See the plan first, move nothing
tidy --dry-run ~/Downloads

# Group by modification month instead (2026-11/, 2023-04/, ...)
tidy --by-date ~/Downloads

# Also organize files inside subfolders
tidy --recursive ~/Downloads

# Skip files matching a glob (repeatable)
tidy --exclude '*.tmp' --exclude '*-part*' ~/Downloads

# Machine-readable plan/report
tidy --dry-run --json ~/Downloads

# Undo the last run in a directory
tidy undo ~/Downloads

# See the built-in extension table
tidy --categories
```

## Design decisions

- **Extension table, not magic bytes.** Classification is a plain
  extension→category map (~90 extensions). It is fast, obvious, and easy to
  extend with `--config rules.json` (`{"blend": "3d-models"}`); user rules
  override the built-ins. Content sniffing would be more accurate, but for a
  sorting tool the extension is what the user already sees.
- **Two-phase plan, then apply.** The scan builds the whole move list first
  (resolving name clashes against both the disk and earlier planned moves),
  and only then does anything move. A failed move mid-run is reported with
  exactly how far it got, and the journal is only written after a successful
  run — so the journal never claims moves that didn't happen.
- **Undo journal, not a trash folder.** Moves are real `rename` calls (fast,
  same filesystem), and the journal records source/destination pairs. Undo
  plays them back in reverse order and refuses to clobber a file that now
  occupies the original spot, or to restore a file that has since been
  deleted — both cases are reported as skipped, not fatal.
- **Idempotent runs.** A file already sitting in its target folder is
  reported as "already organized" and left alone, so re-running `tidy` on a
  sorted directory is a no-op. Recursive runs descend into the category
  folders too, so a file that was manually dropped into the wrong folder
  gets moved to the right one.
- **Zero dependencies.** The standard library covers everything here —
  `flag`, `filepath.WalkDir`, `encoding/json` — so the binary builds anywhere
  Go does.

## Custom categories

```json
{
  "blend": "3d-models",
  "psd": "design",
  "pdf": "ebooks"
}
```

```sh
tidy --config rules.json ~/Downloads
```

Keys may include a leading dot; matching is case-insensitive.

## Tests

```sh
go test ./...
```

16 tests cover classification, clash suffixes, dry-run, excludes, hidden
files, date grouping, recursive scans, and undo (including the missing-file
and occupied-spot edge cases).

## Tech stack

- **Language:** Go 1.23, standard library only
- **Tests:** `go test` with `t.TempDir` fixtures
