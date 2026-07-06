// Package baseprocedures embeds the base procedure entries shipped with the
// sdd binary. Base procedures are graph entries compiled into the release:
// always loaded into every graph, correct independent of graph content — the
// framework's playbook moves must not depend on any project's entries. A
// project customizes a move by superseding a chain head through normal
// capture discipline; a newer sdd version ships successor entries the same
// way, and lint flags the resulting fork when both compete (project head
// wins for execution).
package baseprocedures

import (
	"embed"
	"fmt"
	"io/fs"
	"sort"
	"strings"

	"github.com/networkteam/sdd/internal/model"
)

//go:embed entries
var entriesFS embed.FS

// entriesDir is the embedded directory holding base procedure entry files,
// named by full entry ID (flat — no YYYY/MM nesting; the ID carries the
// date). A README.md documents the directory and is skipped by the loader.
const entriesDir = "entries"

// Entries parses every embedded base procedure entry, marked Embedded so
// write-side surfaces (summary regeneration, missing-summary lint, lint --fix,
// rewrite) skip them. The embedded set is compile-time constant, so an error
// here means a broken build — callers fail hard.
func Entries() ([]*model.Entry, error) {
	return load(entriesFS)
}

// load parses base procedure entries from the entries dir of fsys. Split
// from Entries so tests can exercise the loader against fixture filesystems.
func load(fsys fs.FS) ([]*model.Entry, error) {
	files, err := fs.ReadDir(fsys, entriesDir)
	if err != nil {
		return nil, fmt.Errorf("reading base procedure dir %s: %w", entriesDir, err)
	}

	var entries []*model.Entry
	for _, f := range files {
		if f.IsDir() || f.Name() == "README.md" {
			continue
		}
		if !strings.HasSuffix(f.Name(), ".md") {
			return nil, fmt.Errorf("base procedure dir contains non-entry file %s (entries are <full-id>.md)", f.Name())
		}
		data, err := fs.ReadFile(fsys, entriesDir+"/"+f.Name())
		if err != nil {
			return nil, fmt.Errorf("reading base procedure %s: %w", f.Name(), err)
		}
		entry, err := model.ParseEntry(f.Name(), string(data))
		if err != nil {
			return nil, fmt.Errorf("parsing base procedure %s: %w", f.Name(), err)
		}
		if !entry.IsProcedure() {
			return nil, fmt.Errorf("base entry %s is %s %s — only kind: procedure decisions ship as base entries", entry.ID, entry.Kind, entry.Type)
		}
		entry.Embedded = true
		entries = append(entries, entry)
	}

	sort.Slice(entries, func(i, j int) bool { return entries[i].ID < entries[j].ID })
	return entries, nil
}
