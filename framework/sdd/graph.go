package sdd

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Graph holds all entries and their reference indexes.
type Graph struct {
	Entries  []*Entry
	ByID     map[string]*Entry
	RefsTo   map[string][]string // reverse index: entry ID -> IDs that reference it
	ClosedBy map[string][]string // reverse index: entry ID -> IDs that close it
	graphDir string
}

// NewGraph builds a graph from the given entries without touching the filesystem.
func NewGraph(entries []*Entry) *Graph {
	g := &Graph{
		Entries:  entries,
		ByID:     make(map[string]*Entry, len(entries)),
		RefsTo:   make(map[string][]string),
		ClosedBy: make(map[string][]string),
	}

	for _, e := range entries {
		g.ByID[e.ID] = e
	}

	// Build reverse indexes
	for _, e := range entries {
		for _, ref := range e.Refs {
			g.RefsTo[ref] = append(g.RefsTo[ref], e.ID)
		}
		for _, c := range e.Closes {
			g.ClosedBy[c] = append(g.ClosedBy[c], e.ID)
		}
	}

	// Sort entries by time
	sort.Slice(g.Entries, func(i, j int) bool {
		return g.Entries[i].Time.Before(g.Entries[j].Time)
	})

	// Validate all entries and populate warnings
	g.validate()

	return g
}

// LoadGraph reads all .md files from dir (hierarchical YYYY/MM/ layout) and builds the graph.
func LoadGraph(dir string) (*Graph, error) {
	var entries []*Entry

	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if IsWIPDir(d) {
			return fs.SkipDir
		}
		if d.IsDir() || !strings.HasSuffix(d.Name(), ".md") {
			return nil
		}

		rel, err := filepath.Rel(dir, path)
		if err != nil {
			return fmt.Errorf("getting relative path for %s: %w", path, err)
		}

		id, err := RelPathToID(rel)
		if err != nil {
			// Skip files that don't match the expected layout
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", path, err)
		}

		entry, err := ParseEntry(id+".md", string(data))
		if err != nil {
			return fmt.Errorf("parsing %s: %w", path, err)
		}

		entries = append(entries, entry)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking graph dir: %w", err)
	}

	// Scan for attachment directories
	for _, e := range entries {
		attachRel, err := AttachDirRelPath(e.ID)
		if err != nil {
			continue
		}
		attachDir := filepath.Join(dir, attachRel)
		files, err := os.ReadDir(attachDir)
		if err != nil {
			continue // no attachment directory
		}
		for _, f := range files {
			if !f.IsDir() {
				e.Attachments = append(e.Attachments, filepath.Join(attachRel, f.Name()))
			}
		}
	}

	g := NewGraph(entries)
	g.graphDir = dir
	return g, nil
}

// ActiveDecisions returns active directive decisions (not closed, not superseded, not contracts).
func (g *Graph) ActiveDecisions() []*Entry {
	closed := g.closedSet()
	superseded := g.supersededSet()

	var active []*Entry
	for _, e := range g.Entries {
		if e.Type != TypeDecision || e.IsContract() {
			continue
		}
		if !closed[e.ID] && !superseded[e.ID] {
			active = append(active, e)
		}
	}
	return active
}

// Contracts returns active contract decisions (not superseded).
// Contracts are never closed — they stay active until superseded.
func (g *Graph) Contracts() []*Entry {
	superseded := g.supersededSet()

	var contracts []*Entry
	for _, e := range g.Entries {
		if e.Type != TypeDecision || !e.IsContract() {
			continue
		}
		if !superseded[e.ID] {
			contracts = append(contracts, e)
		}
	}
	return contracts
}

// OpenSignals returns signals that are not closed and not superseded.
func (g *Graph) OpenSignals() []*Entry {
	closed := g.closedSet()
	superseded := g.supersededSet()

	var open []*Entry
	for _, e := range g.Entries {
		if e.Type != TypeSignal {
			continue
		}
		if !closed[e.ID] && !superseded[e.ID] {
			open = append(open, e)
		}
	}
	return open
}

// RecentActions returns the last n actions by timestamp.
func (g *Graph) RecentActions(n int) []*Entry {
	var actions []*Entry
	for _, e := range g.Entries {
		if e.Type == TypeAction {
			actions = append(actions, e)
		}
	}
	// Entries are already sorted by time, take last n
	if len(actions) > n {
		actions = actions[len(actions)-n:]
	}
	return actions
}

// RefChain returns the entry and all entries it transitively references, in dependency order.
func (g *Graph) RefChain(id string) []*Entry {
	seen := make(map[string]bool)
	var chain []*Entry

	var walk func(string)
	walk = func(eid string) {
		if seen[eid] {
			return
		}
		seen[eid] = true
		e, ok := g.ByID[eid]
		if !ok {
			return
		}
		for _, ref := range e.Refs {
			walk(ref)
		}
		chain = append(chain, e)
	}

	walk(id)
	return chain
}

// Filter returns entries matching the given criteria. Empty values match all.
func (g *Graph) Filter(typ EntryType, layer Layer) []*Entry {
	var result []*Entry
	for _, e := range g.Entries {
		if typ != "" && e.Type != typ {
			continue
		}
		if layer != "" && e.Layer != layer {
			continue
		}
		result = append(result, e)
	}
	return result
}

// FilterOpen returns only open/active entries matching the criteria.
// Open signals = not closed and not superseded.
// Active decisions = not closed and not superseded.
// Actions are always included (they are facts of execution).
// Kind filter applies only to decisions: "contract" or "directive" (empty = all).
func (g *Graph) FilterOpen(typ EntryType, layer Layer, kind Kind) []*Entry {
	closed := g.closedSet()
	superseded := g.supersededSet()

	var result []*Entry
	for _, e := range g.Entries {
		if typ != "" && e.Type != typ {
			continue
		}
		if layer != "" && e.Layer != layer {
			continue
		}
		if kind != "" && e.Type == TypeDecision {
			if kind == KindDirective && e.IsContract() {
				continue
			}
			if kind == KindContract && !e.IsContract() {
				continue
			}
		}
		switch e.Type {
		case TypeSignal, TypeDecision:
			if closed[e.ID] || superseded[e.ID] {
				continue
			}
		}
		result = append(result, e)
	}
	return result
}

// closedSet returns the set of entry IDs that are closed by another entry.
func (g *Graph) closedSet() map[string]bool {
	set := make(map[string]bool)
	for id := range g.ClosedBy {
		set[id] = true
	}
	return set
}

// supersededSet returns the set of entry IDs that are superseded by another entry.
func (g *Graph) supersededSet() map[string]bool {
	set := make(map[string]bool)
	for _, e := range g.Entries {
		for _, s := range e.Supersedes {
			set[s] = true
		}
	}
	return set
}

// Downstream returns entries that reference, close, or supersede the given ID.
// Results are sorted by time (oldest first).
func (g *Graph) Downstream(id string) []*Entry {
	seen := make(map[string]bool)
	var result []*Entry

	add := func(eid string) {
		if seen[eid] {
			return
		}
		if e, ok := g.ByID[eid]; ok {
			seen[eid] = true
			result = append(result, e)
		}
	}

	// Entries that reference this ID
	for _, eid := range g.RefsTo[id] {
		add(eid)
	}

	// Entries that close this ID
	for _, eid := range g.ClosedBy[id] {
		add(eid)
	}

	// Entries that supersede this ID
	for _, e := range g.Entries {
		for _, s := range e.Supersedes {
			if s == id {
				add(e.ID)
			}
		}
	}

	sort.Slice(result, func(i, j int) bool {
		return result[i].Time.Before(result[j].Time)
	})

	return result
}

// GraphDir returns the directory the graph was loaded from.
func (g *Graph) GraphDir() string {
	return g.graphDir
}

// Lint returns all entries that have validation warnings.
func (g *Graph) Lint() []*Entry {
	var result []*Entry
	for _, e := range g.Entries {
		if len(e.Warnings) > 0 {
			result = append(result, e)
		}
	}
	return result
}

// validate checks all entries for integrity issues and populates their Warnings fields.
func (g *Graph) validate() {
	for _, e := range g.Entries {
		ValidateEntry(e, g)
	}
}

// ValidateEntry checks a single entry for integrity issues and populates its Warnings field.
// Used both at lint time (all entries) and at write time (new entry before commit).
func ValidateEntry(e *Entry, g *Graph) {
	validateIDRefs(e, g, "refs", e.Refs)
	validateIDRefs(e, g, "closes", e.Closes)
	validateIDRefs(e, g, "supersedes", e.Supersedes)
	validateCloses(e, g)
	validateSupersedes(e, g)
	validateAttachmentLinks(e)
}

// validateIDRefs checks that all IDs in the given field are well-formed and exist in the graph.
func validateIDRefs(e *Entry, g *Graph, field string, ids []string) {
	for _, id := range ids {
		_, err := ParseID(id)
		if err != nil {
			e.Warnings = append(e.Warnings, Warning{
				Field:   field,
				Value:   id,
				Message: fmt.Sprintf("malformed ID in %s: %s", field, id),
			})
			continue
		}
		if _, ok := g.ByID[id]; !ok {
			e.Warnings = append(e.Warnings, Warning{
				Field:   field,
				Value:   id,
				Message: fmt.Sprintf("dangling ref in %s: %s (entry not found)", field, id),
			})
		}
	}
}

// validateCloses checks type constraints on closes references.
// Valid: decision closes signal, action closes decision, action closes signal.
// Invalid: anything closes action, signal closes anything, decision closes decision.
func validateCloses(e *Entry, g *Graph) {
	for _, id := range e.Closes {
		target, ok := g.ByID[id]
		if !ok {
			continue // already reported by validateIDRefs
		}

		switch {
		case e.Type == TypeSignal:
			e.Warnings = append(e.Warnings, Warning{
				Field:   "closes",
				Value:   id,
				Message: fmt.Sprintf("signal cannot close entries (closes %s %s)", target.Type, id),
			})
		case target.Type == TypeAction:
			e.Warnings = append(e.Warnings, Warning{
				Field:   "closes",
				Value:   id,
				Message: fmt.Sprintf("actions cannot be closed (closes action %s)", id),
			})
		case e.Type == TypeDecision && target.Type == TypeDecision:
			e.Warnings = append(e.Warnings, Warning{
				Field:   "closes",
				Value:   id,
				Message: fmt.Sprintf("decision cannot close another decision — use supersedes instead (closes decision %s)", id),
			})
		}
	}
}

// validateSupersedes checks that supersedes references point at the same entry type.
func validateSupersedes(e *Entry, g *Graph) {
	for _, id := range e.Supersedes {
		target, ok := g.ByID[id]
		if !ok {
			continue // already reported by validateIDRefs
		}

		if target.Type != e.Type {
			e.Warnings = append(e.Warnings, Warning{
				Field:   "supersedes",
				Value:   id,
				Message: fmt.Sprintf("type mismatch in supersedes: %s supersedes %s %s (expected %s)", e.Type, target.Type, id, e.Type),
			})
		}
	}
}

// validateAttachmentLinks checks that markdown links referencing the entry's attachment
// directory point to files that exist in the entry's Attachments list.
func validateAttachmentLinks(e *Entry) {
	if len(e.ID) < 8 {
		return
	}
	shortName := e.ID[6:] // DD-HHmmss-type-layer-suffix
	prefix := "./" + shortName + "/"

	if !strings.Contains(e.Content, prefix) {
		return
	}

	// Build set of known attachment filenames
	knownFiles := make(map[string]bool)
	for _, a := range e.Attachments {
		knownFiles[filepath.Base(a)] = true
	}

	// Find all references to the attachment directory in content
	rest := e.Content
	for {
		idx := strings.Index(rest, prefix)
		if idx < 0 {
			break
		}
		after := rest[idx+len(prefix):]
		// Extract filename until a markdown/whitespace delimiter
		end := strings.IndexAny(after, ") \n\t\"'")
		var filename string
		if end > 0 {
			filename = after[:end]
		} else if end < 0 {
			filename = after // rest of string
		}
		if filename != "" && !knownFiles[filename] {
			e.Warnings = append(e.Warnings, Warning{
				Field:   "attachments",
				Value:   prefix + filename,
				Message: fmt.Sprintf("broken attachment link: %s%s (file not found in attachment directory)", prefix, filename),
			})
		}
		if end < 0 {
			break
		}
		rest = after[end:]
	}
}
