package finders

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"

	"github.com/networkteam/sdd/internal/query"
)

// ReadAttachment reads one page of an entry's attachment through the shared
// accessor: the entry's frontmatter-listed attachments are the only readable
// set, so callers never touch the on-disk layout (d-tac-d21). Pure read.
func (gf *GraphFinder) ReadAttachment(q query.ReadAttachmentQuery) (*query.ReadAttachmentResult, error) {
	if gf.graph == nil {
		return nil, fmt.Errorf("read attachment: graph is required")
	}
	entry, ok := gf.graph.ByID[q.EntryID]
	if !ok {
		return nil, fmt.Errorf("entry %s not found", q.EntryID)
	}

	available := make([]string, 0, len(entry.Attachments))
	byName := make(map[string]string, len(entry.Attachments))
	for _, rel := range entry.Attachments {
		name := filepath.Base(rel)
		available = append(available, name)
		byName[name] = rel
	}
	sort.Strings(available)

	if len(entry.Attachments) == 0 {
		return nil, fmt.Errorf("entry %s has no attachments", q.EntryID)
	}
	name := q.Name
	if name == "" {
		if len(available) > 1 {
			return nil, fmt.Errorf("entry %s has %d attachments — pass name (one of: %v)", q.EntryID, len(available), available)
		}
		name = available[0]
	}
	rel, ok := byName[name]
	if !ok {
		return nil, fmt.Errorf("entry %s has no attachment %q (available: %v)", q.EntryID, name, available)
	}

	absPath, err := filepath.Abs(filepath.Join(gf.graph.GraphDir(), filepath.FromSlash(rel)))
	if err != nil {
		return nil, fmt.Errorf("resolving attachment path %s: %w", name, err)
	}

	file, err := os.Open(absPath)
	if err != nil {
		return nil, fmt.Errorf("opening attachment %s: %w", name, err)
	}
	defer func() { _ = file.Close() }()

	stat, err := file.Stat()
	if err != nil {
		return nil, fmt.Errorf("stat attachment %s: %w", name, err)
	}
	total := stat.Size()

	offset := max(q.Offset, 0)
	maxBytes := q.MaxBytes
	if maxBytes <= 0 {
		maxBytes = query.DefaultAttachmentPageBytes
	}

	result := &query.ReadAttachmentResult{
		EntryID:    q.EntryID,
		Name:       name,
		Offset:     offset,
		TotalBytes: total,
		Available:  available,
		Path:       absPath,
	}
	if offset >= total {
		return result, nil
	}

	buf := make([]byte, maxBytes)
	n, err := file.ReadAt(buf, offset)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("reading attachment %s: %w", name, err)
	}
	result.Content = string(buf[:n])
	result.NextOffset = offset + int64(n)
	result.More = result.NextOffset < total
	return result, nil
}
