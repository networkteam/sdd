package model

// MergeEmbedded appends binary-shipped embedded entries to entries, skipping
// any whose ID already appears on disk. A project owns its graph directory, so
// an on-disk entry shadows an embedded one of the same ID — this is how a
// project customizes a base procedure or base fact (supersession on the same
// ID). onDisk must hold the IDs already loaded from disk.
//
// This is the single home for the disk-wins merge semantics shared by every
// graph load path; changing how embedded entries merge happens here, once.
func MergeEmbedded(entries []*Entry, onDisk map[string]bool, embedded []*Entry) []*Entry {
	for _, e := range embedded {
		if !onDisk[e.ID] {
			entries = append(entries, e)
		}
	}
	return entries
}
