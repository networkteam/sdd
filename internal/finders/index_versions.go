package finders

import (
	"context"
	"fmt"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/query"
)

// IndexVersions reports a store's stored versions grouped by derivation rule
// against the graph's current entry states. It reads the manifest under the
// shared lock and sizes each group from the rows' files; nothing is deleted
// here — that is IndexHandler.DropVersions.
func (f *Finder) IndexVersions(ctx context.Context, q query.IndexVersionsQuery) (*query.IndexVersionsResult, error) {
	g, err := f.CurrentGraph(q.GraphDir)
	if err != nil {
		return nil, fmt.Errorf("loading graph: %w", err)
	}
	attachments := chunking.DiskAttachmentReader{GraphDir: q.GraphDir}
	current := make(map[string]string, len(g.Entries))
	for _, entry := range g.Entries {
		if !chunking.IncludeEntry(entry, q.ExcludeEmbedded) {
			continue
		}
		hash, err := chunking.EntryStateHash(ctx, entry, attachments)
		if err != nil {
			return nil, err
		}
		current[entry.ID] = hash
	}

	manifest, err := index.ReadManifestLocked(ctx, q.IndexDir)
	if err != nil {
		return nil, err
	}
	total, err := index.StoreSize(q.IndexDir)
	if err != nil {
		return nil, fmt.Errorf("sizing store %s: %w", q.IndexDir, err)
	}
	result := &query.IndexVersionsResult{Label: q.Label, IndexDir: q.IndexDir, Entries: len(manifest.Entries), Bytes: total}
	for _, g := range manifest.VersionGroups(current) {
		result.Groups = append(result.Groups, query.IndexVersionGroup{
			Name: g.Name, Versions: g.Versions, Entries: g.Entries, Oldest: g.Oldest, Newest: g.Newest,
			Bytes: index.DocumentsSize(q.IndexDir, g.ChunkIDs), Droppable: g.Droppable,
		})
	}
	return result, nil
}
