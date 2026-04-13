package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/networkteam/resonance/framework/sdd/command"
	"github.com/networkteam/resonance/framework/sdd/model"
)

// StartWIP creates a WIP marker for cmd.EntryID, commits it, and optionally
// creates + checks out a derived git branch. Validates that the entry
// exists in the graph and warns (via OnExclusiveCollision) when another
// exclusive marker already covers the entry.
func (h *Handler) StartWIP(ctx context.Context, cmd *command.StartWIPCmd) error {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}

	g, err := h.reader.LoadGraph(h.graphDir)
	if err != nil {
		return fmt.Errorf("loading graph: %w", err)
	}
	if _, ok := g.ByID[cmd.EntryID]; !ok {
		return fmt.Errorf("entry not found: %s", cmd.EntryID)
	}

	markers, err := h.reader.LoadWIPMarkers(h.graphDir)
	if err != nil {
		return fmt.Errorf("loading WIP markers: %w", err)
	}
	if existing, ok := model.HasExclusiveMarker(markers, cmd.EntryID); ok {
		if cmd.OnExclusiveCollision != nil {
			cmd.OnExclusiveCollision(existing)
		}
	}

	var branchName string
	if cmd.Branch {
		branchName = model.DeriveBranchName(cmd.EntryID, cmd.Description)
	}

	markerID := model.GenerateWIPMarkerID(cmd.Participant)
	marker := &model.WIPMarker{
		ID:          markerID,
		Entry:       cmd.EntryID,
		Participant: cmd.Participant,
		Exclusive:   cmd.Exclusive,
		Branch:      branchName,
		Content:     cmd.Description,
	}

	markerPath := filepath.Join(h.graphDir, model.WIPMarkerPath(markerID))
	if err := os.MkdirAll(filepath.Dir(markerPath), 0755); err != nil {
		return fmt.Errorf("creating wip directory: %w", err)
	}

	content := model.FormatWIPMarker(marker)
	if err := os.WriteFile(markerPath, []byte(content), 0644); err != nil {
		return fmt.Errorf("writing marker: %w", err)
	}

	if cmd.OnStarted != nil {
		cmd.OnStarted(markerID, markerPath)
	}

	// Commit marker on current branch (before any branch creation, so the
	// marker is visible on main for coordination).
	if h.committer != nil {
		msg := fmt.Sprintf("sdd: wip start %s (%s)", cmd.EntryID, cmd.Participant)
		if err := h.committer.Commit(msg, markerPath); err != nil {
			fmt.Fprintf(h.stderr, "warning: git commit failed: %v\n", err)
		}
	}

	if branchName != "" {
		if h.brancher == nil {
			return fmt.Errorf("--branch requested but no Brancher configured")
		}
		if err := h.brancher.Checkout(branchName, true); err != nil {
			return fmt.Errorf("creating branch %s: %w", branchName, err)
		}
		if cmd.OnBranchCreated != nil {
			cmd.OnBranchCreated(branchName)
		}
	}

	return nil
}
