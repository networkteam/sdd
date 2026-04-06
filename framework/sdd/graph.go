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
	graphDir string
}

// LoadGraph reads all .md files from dir and builds the graph.
func LoadGraph(dir string) (*Graph, error) {
	g := &Graph{
		ByID:     make(map[string]*Entry),
		RefsTo:   make(map[string][]string),
		graphDir: dir,
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("reading graph dir: %w", err)
	}

	for _, de := range entries {
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

		g.Entries = append(g.Entries, entry)
		g.ByID[entry.ID] = entry
	}

	// Build reverse index
	for _, e := range g.Entries {
		for _, ref := range e.Refs {
			g.RefsTo[ref] = append(g.RefsTo[ref], e.ID)
		}
	}

	// Sort entries by time
	sort.Slice(g.Entries, func(i, j int) bool {
		return g.Entries[i].Time.Before(g.Entries[j].Time)
	})

	return g, nil
}

// ActiveDecisions returns decisions that are not superseded by another decision.
// A decision is considered superseded if another decision references it.
func (g *Graph) ActiveDecisions() []*Entry {
	var active []*Entry
	for _, e := range g.Entries {
		if e.Type != TypeDecision {
			continue
		}
		superseded := false
		for _, refBy := range g.RefsTo[e.ID] {
			if ref, ok := g.ByID[refBy]; ok && ref.Type == TypeDecision {
				superseded = true
				break
			}
		}
		if !superseded {
			active = append(active, e)
		}
	}
	return active
}

// OpenSignals returns signals that are not referenced by any decision.
func (g *Graph) OpenSignals() []*Entry {
	var open []*Entry
	for _, e := range g.Entries {
		if e.Type != TypeSignal {
			continue
		}
		addressedByDecision := false
		for _, refBy := range g.RefsTo[e.ID] {
			if ref, ok := g.ByID[refBy]; ok && ref.Type == TypeDecision {
				addressedByDecision = true
				break
			}
		}
		if !addressedByDecision {
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

// GraphDir returns the directory the graph was loaded from.
func (g *Graph) GraphDir() string {
	return g.graphDir
}
