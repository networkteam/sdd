package model

import (
	"slices"
	"strings"
)

// EntriesAfter returns stable ID order without changing the graph's entry order.
func (g *Graph) EntriesAfter(id string) []*Entry {
	entries := slices.Clone(g.Entries)
	slices.SortFunc(entries, func(a, b *Entry) int { return strings.Compare(a.ID, b.ID) })
	start, found := slices.BinarySearchFunc(entries, id, func(entry *Entry, id string) int { return strings.Compare(entry.ID, id) })
	if found {
		start++
	}
	return entries[start:]
}
