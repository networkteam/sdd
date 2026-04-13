package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/networkteam/resonance/framework/sdd/command"
	"github.com/networkteam/resonance/framework/sdd/model"
)

// FinishWIP removes the WIP marker identified by cmd.MarkerID, commits the
// removal, and (if the marker referenced a branch) deletes the branch when
// merged or when --force is set. Unmerged non-force branches are preserved
// and OnBranchPreserved fires so the CLI can print guidance.
func (h *Handler) FinishWIP(ctx context.Context, cmd *command.FinishWIPCmd) error {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}

	markerPath := filepath.Join(h.graphDir, model.WIPMarkerPath(cmd.MarkerID))
	if _, err := os.Stat(markerPath); err != nil {
		return fmt.Errorf("marker not found: %s", cmd.MarkerID)
	}

	data, err := os.ReadFile(markerPath)
	if err != nil {
		return fmt.Errorf("reading marker: %w", err)
	}
	marker, err := model.ParseWIPMarker(filepath.Base(markerPath), string(data))
	if err != nil {
		return fmt.Errorf("parsing marker: %w", err)
	}

	if err := os.Remove(markerPath); err != nil {
		return fmt.Errorf("removing marker: %w", err)
	}

	if cmd.OnRemoved != nil {
		cmd.OnRemoved(cmd.MarkerID)
	}

	// Stage the deletion and commit. Warn but don't fail when git refuses —
	// the marker is already off disk; commit failure is recoverable.
	if h.committer != nil {
		msg := fmt.Sprintf("sdd: wip done %s", cmd.MarkerID)
		if err := h.committer.Commit(msg, markerPath); err != nil {
			fmt.Fprintf(h.stderr, "warning: git commit failed: %v\n", err)
		}
	}

	// Branch cleanup.
	if marker.Branch != "" {
		if h.brancher == nil {
			return fmt.Errorf("marker references branch %s but no Brancher configured", marker.Branch)
		}
		merged := h.brancher.BranchMerged(marker.Branch)
		switch {
		case merged:
			if err := h.brancher.DeleteBranch(marker.Branch, false); err != nil {
				fmt.Fprintf(h.stderr, "  warning: deleting branch %s: %v\n", marker.Branch, err)
			} else if cmd.OnBranchDeleted != nil {
				cmd.OnBranchDeleted(marker.Branch, false)
			}
		case cmd.Force:
			if err := h.brancher.DeleteBranch(marker.Branch, true); err != nil {
				fmt.Fprintf(h.stderr, "  warning: force-deleting branch %s: %v\n", marker.Branch, err)
			} else if cmd.OnBranchDeleted != nil {
				cmd.OnBranchDeleted(marker.Branch, true)
			}
		default:
			if cmd.OnBranchPreserved != nil {
				cmd.OnBranchPreserved(marker.Branch)
			}
		}
	}

	return nil
}
