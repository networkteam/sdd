package sdd

import (
	"fmt"
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

	return g
}

// LoadGraph reads all .md files from dir and builds the graph.
func LoadGraph(dir string) (*Graph, error) {
	dirEntries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading graph dir: %w", err)
	}

	var entries []*Entry
	for _, de := range dirEntries {
		if de.IsDir() || !strings.HasSuffix(de.Name(), ".md") {
			continue
		}

		data, err := os.ReadFile(filepath.Join(dir, de.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", de.Name(), err)
		}

		entry, err := ParseEntry(de.Name(), string(data))
		if err != nil {
			return nil, fmt.Errorf("parsing %s: %w", de.Name(), err)
		}

		entries = append(entries, entry)
	}

	g := NewGraph(entries)
	g.graphDir = dir
	return g, nil
}

// ActiveDecisions returns decisions that are not closed and not superseded.
func (g *Graph) ActiveDecisions() []*Entry {
	closed := g.closedSet()
	superseded := g.supersededSet()

	var active []*Entry
	for _, e := range g.Entries {
		if e.Type != TypeDecision {
			continue
		}
		if !closed[e.ID] && !superseded[e.ID] {
			active = append(active, e)
		}
	}
	return active
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
func (g *Graph) FilterOpen(typ EntryType, layer Layer) []*Entry {
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

// GraphDir returns the directory the graph was loaded from.
func (g *Graph) GraphDir() string {
	return g.graphDir
}
