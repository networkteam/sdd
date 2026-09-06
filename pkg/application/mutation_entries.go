package application

import (
	"fmt"
	"slices"

	"github.com/networkteam/sdd/internal/model"
)

// AffectedEntryIDs returns entry selections for post-write discovery, including
// deleted documents and attachment owners. It does not establish durable source:
// queue discovery only after the consumer can reacquire its exact finalized
// revision and attachments. An empty result means no discovery job.
func (m AppliedMutation) AffectedEntryIDs() ([]string, error) {
	var ids []string
	add := func(path string) error {
		id, err := model.EntryIDForArtifactPath(path)
		if err != nil {
			return err
		}
		if id != "" {
			ids = append(ids, id)
		}
		return nil
	}
	for _, change := range m.Batch.Changes {
		if change.Document != nil {
			if change.Document.LogicalPath != change.LogicalPath {
				return nil, fmt.Errorf("sdd: inconsistent mutation document paths")
			}
			id, err := model.EntryIDForArtifactPath(change.LogicalPath)
			if err != nil {
				return nil, err
			}
			if id == "" {
				return nil, fmt.Errorf("sdd: structured document has no entry identity")
			}
		}
		if err := add(change.LogicalPath); err != nil {
			return nil, err
		}
	}
	for _, attachment := range m.Batch.Attachments {
		if err := add(attachment.LogicalPath); err != nil {
			return nil, err
		}
	}
	slices.Sort(ids)
	return slices.Compact(ids), nil
}
