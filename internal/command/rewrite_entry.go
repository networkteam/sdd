package command

import (
	"fmt"

	"github.com/networkteam/sdd/internal/model"
)

// RewriteEntryCmd captures intent to atomically change an entry's type and/or
// kind. The handler renames the file (when type changes), rewrites frontmatter,
// updates every inbound refs/closes/supersedes reference across the graph, and
// commits the result as one unit. Bypasses pre-flight — this is a mechanical
// fix per d-cpt-e1i, not a capture that warrants LLM validation.
type RewriteEntryCmd struct {
	EntryID  string          // full ID (or short form resolved before handler)
	NewType  model.EntryType // target type
	NewKind  model.Kind      // target kind
	Message  string          // optional commit message override
	DryRun   bool            // when true: validate and report; do not write or commit
	NoCommit bool            // when true: write changes to disk but skip git commit

	// OnRewritten fires after a successful rewrite (real or dry-run) with the
	// old id, new id, and the list of inbound entry ids whose references were
	// updated. For richer data, the caller issues a follow-up query.
	OnRewritten func(oldID, newID string, inboundUpdated []string)
}

// Validate checks command-level invariants. Graph-resolved checks (entry
// exists, new ID doesn't collide with an existing entry) are the handler's job.
func (c *RewriteEntryCmd) Validate() error {
	if c.EntryID == "" {
		return fmt.Errorf("entry id is required")
	}
	if c.NewType == "" {
		return fmt.Errorf("--type is required")
	}
	if _, ok := model.TypeAbbrev[c.NewType]; !ok {
		return fmt.Errorf("invalid type: %s", c.NewType)
	}
	if c.NewKind == "" {
		return fmt.Errorf("--kind is required")
	}
	if !model.IsValidKindForType(c.NewType, c.NewKind) {
		return fmt.Errorf("invalid kind %q for type %s", c.NewKind, c.NewType)
	}
	return nil
}
