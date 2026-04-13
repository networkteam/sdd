package command

import (
	"fmt"

	"github.com/networkteam/resonance/framework/sdd/model"
)

// StartWIPCmd captures intent to create a WIP marker for a graph entry,
// optionally creating and checking out a git branch in the same step.
type StartWIPCmd struct {
	EntryID     string
	Description string
	Participant string
	Exclusive   bool
	Branch      bool // when true, derive a branch name and create+checkout

	// OnStarted fires after the marker file is written and committed.
	// Callbacks receive only identifiers — the caller queries finders for
	// richer data if needed (consistent with d-cpt-l3s).
	OnStarted func(markerID, markerPath string)

	// OnBranchCreated fires after the git branch is created and checked
	// out. Only invoked when Branch is true.
	OnBranchCreated func(branchName string)

	// OnExclusiveCollision fires when an existing exclusive marker is
	// found for the same entry. The handler doesn't block creation; the
	// callback lets the CLI print a warning.
	OnExclusiveCollision func(existing *model.WIPMarker)
}

// Validate checks required fields.
func (c *StartWIPCmd) Validate() error {
	if c.EntryID == "" {
		return fmt.Errorf("entry ID is required")
	}
	if c.Participant == "" {
		return fmt.Errorf("--participant is required")
	}
	return nil
}

// FinishWIPCmd captures intent to remove a WIP marker (and optionally clean
// up its git branch).
type FinishWIPCmd struct {
	MarkerID string
	Force    bool // when true, force-delete an unmerged branch (discard flow)

	// OnRemoved fires after the marker file is removed and the deletion
	// is committed.
	OnRemoved func(markerID string)

	// OnBranchDeleted fires after a git branch is deleted (merged or
	// force-deleted).
	OnBranchDeleted func(branch string, forced bool)

	// OnBranchPreserved fires when the marker referenced a branch but the
	// branch was unmerged and Force was not set — branch is preserved,
	// callback informs the CLI to print guidance.
	OnBranchPreserved func(branch string)
}

// Validate checks required fields.
func (c *FinishWIPCmd) Validate() error {
	if c.MarkerID == "" {
		return fmt.Errorf("marker ID is required")
	}
	return nil
}
