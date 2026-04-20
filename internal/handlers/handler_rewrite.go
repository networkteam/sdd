package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
)

// RewriteEntry executes a RewriteEntryCmd: resolves the target entry, computes
// its new ID from the requested type, walks the graph for inbound references,
// renames the file (and attachment directory) when the type changes, rewrites
// frontmatter on the target and every inbound entry, and commits atomically.
// Bypasses pre-flight per the plan: rewrite is a mechanical operation, not a
// capture.
//
// DryRun returns without touching disk after reporting intended changes via
// the OnRewritten callback. NoCommit writes to disk but skips the git commit.
func (h *Handler) RewriteEntry(ctx context.Context, cmd *command.RewriteEntryCmd) error {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}

	logger := slogutils.FromContext(ctx)

	graph, err := h.reader.LoadGraph(h.graphDir)
	if err != nil {
		return fmt.Errorf("loading graph: %w", err)
	}

	resolvedID, err := graph.ResolveID(cmd.EntryID)
	if err != nil {
		return fmt.Errorf("resolving entry id: %w", err)
	}

	target, ok := graph.ByID[resolvedID]
	if !ok {
		return fmt.Errorf("entry not found: %s", cmd.EntryID)
	}

	newID, err := model.RewriteID(resolvedID, cmd.NewType)
	if err != nil {
		return fmt.Errorf("computing new id: %w", err)
	}

	// No-op guard: same type and same kind means nothing to rewrite.
	if newID == resolvedID && target.Kind == cmd.NewKind {
		return fmt.Errorf("rewrite is a no-op: entry already has type %s kind %s", cmd.NewType, cmd.NewKind)
	}

	if newID != resolvedID {
		if _, collides := graph.ByID[newID]; collides {
			return fmt.Errorf("rewrite would collide: entry %s already exists", newID)
		}
	}

	// Inbound reference walk: every entry that refs/closes/supersedes the
	// target will have that pointer rewritten to newID. Graph.Downstream
	// provides this directly — see d-cpt-e7i plan "inbound-reference finder".
	inbound := graph.Downstream(resolvedID)
	inboundIDs := make([]string, 0, len(inbound))
	for _, e := range inbound {
		inboundIDs = append(inboundIDs, e.ID)
	}
	sort.Strings(inboundIDs)

	logger.Info("rewrite plan",
		"old_id", resolvedID,
		"new_id", newID,
		"new_kind", cmd.NewKind,
		"inbound_updates", len(inbound),
	)

	if cmd.DryRun {
		if cmd.OnRewritten != nil {
			cmd.OnRewritten(resolvedID, newID, inboundIDs)
		}
		return nil
	}

	// Prepare the rewritten target entry. Summary is invalidated by the type/
	// kind change — clear it and let a later `sdd summarize` repopulate.
	rewritten := *target
	rewritten.ID = newID
	rewritten.Type = cmd.NewType
	rewritten.Kind = cmd.NewKind
	rewritten.Summary = ""
	rewritten.SummaryHash = ""

	oldRelPath, err := model.IDToRelPath(resolvedID)
	if err != nil {
		return fmt.Errorf("computing old path for %s: %w", resolvedID, err)
	}
	newRelPath, err := model.IDToRelPath(newID)
	if err != nil {
		return fmt.Errorf("computing new path for %s: %w", newID, err)
	}
	oldFilePath := filepath.Join(h.graphDir, oldRelPath)
	newFilePath := filepath.Join(h.graphDir, newRelPath)

	var commitPaths []string

	// Move the main file and (if present) attachment directory when the
	// on-disk path changes — i.e. the type character flipped.
	if newFilePath != oldFilePath {
		if h.mover == nil {
			return fmt.Errorf("Mover is required for rewrites that change entry type")
		}
		if err := h.mover.Move(oldFilePath, newFilePath); err != nil {
			return fmt.Errorf("moving entry file: %w", err)
		}

		oldAttachRel, err := model.AttachDirRelPath(resolvedID)
		if err == nil {
			oldAttachDir := filepath.Join(h.graphDir, oldAttachRel)
			if info, statErr := os.Stat(oldAttachDir); statErr == nil && info.IsDir() {
				newAttachRel, err := model.AttachDirRelPath(newID)
				if err != nil {
					return fmt.Errorf("computing new attachment dir: %w", err)
				}
				newAttachDir := filepath.Join(h.graphDir, newAttachRel)
				if err := h.mover.Move(oldAttachDir, newAttachDir); err != nil {
					return fmt.Errorf("moving attachment directory: %w", err)
				}
				// Refresh attachment paths on the rewritten entry so they point
				// at the new directory when frontmatter is serialized.
				for i, p := range rewritten.Attachments {
					rewritten.Attachments[i] = filepath.Join(newAttachRel, filepath.Base(p))
				}
			}
		}
	}

	// Persist the rewritten target at its new location.
	targetContent := model.FormatFrontmatter(&rewritten) + "\n" + rewritten.Content + "\n"
	if err := os.WriteFile(newFilePath, []byte(targetContent), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", newFilePath, err)
	}
	commitPaths = append(commitPaths, newFilePath)

	// Rewrite inbound references on every dependent entry.
	for _, e := range inbound {
		changed := rewriteRefs(e, resolvedID, newID)
		if !changed {
			continue
		}
		relPath, err := model.IDToRelPath(e.ID)
		if err != nil {
			return fmt.Errorf("computing path for inbound %s: %w", e.ID, err)
		}
		inboundPath := filepath.Join(h.graphDir, relPath)
		// Invalidate the summary — the references it was computed from have changed.
		e.Summary = ""
		e.SummaryHash = ""
		content := model.FormatFrontmatter(e) + "\n" + e.Content + "\n"
		if err := os.WriteFile(inboundPath, []byte(content), 0644); err != nil {
			return fmt.Errorf("writing inbound %s: %w", inboundPath, err)
		}
		commitPaths = append(commitPaths, inboundPath)
	}

	if !cmd.NoCommit && h.committer != nil {
		msg := cmd.Message
		if msg == "" {
			msg = fmt.Sprintf("sdd: rewrite %s → %s (%s %s)", resolvedID, newID, cmd.NewType, cmd.NewKind)
		}
		if err := h.committer.Commit(msg, commitPaths...); err != nil {
			fmt.Fprintf(h.stderr, "warning: git commit failed: %v\n", err)
		}
	}

	if cmd.OnRewritten != nil {
		cmd.OnRewritten(resolvedID, newID, inboundIDs)
	}
	return nil
}

// rewriteRefs replaces every occurrence of oldID with newID across the entry's
// refs, closes, and supersedes fields. Returns true if any field was mutated.
func rewriteRefs(e *model.Entry, oldID, newID string) bool {
	changed := false
	for i, id := range e.Refs {
		if id == oldID {
			e.Refs[i] = newID
			changed = true
		}
	}
	for i, id := range e.Closes {
		if id == oldID {
			e.Closes[i] = newID
			changed = true
		}
	}
	for i, id := range e.Supersedes {
		if id == oldID {
			e.Supersedes[i] = newID
			changed = true
		}
	}
	return changed
}
