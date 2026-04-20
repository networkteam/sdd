package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/model"
)

// LintFix applies mechanical fixes to graph entries: loads the graph,
// identifies fixable issues, patches files in place, and commits.
func (h *Handler) LintFix(ctx context.Context, cmd *command.LintFixCmd) error {
	graph, err := h.reader.LoadGraph(h.graphDir)
	if err != nil {
		return fmt.Errorf("loading graph: %w", err)
	}

	var commitPaths []string
	fixCount := 0

	for _, e := range graph.Entries {
		fixes := applyFixes(e)
		if len(fixes) == 0 {
			continue
		}

		relPath, err := model.IDToRelPath(e.ID)
		if err != nil {
			return fmt.Errorf("computing path for %s: %w", e.ID, err)
		}
		filePath := filepath.Join(h.graphDir, relPath)
		fileContent := model.FormatFrontmatter(e) + "\n" + e.Content + "\n"

		if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", filePath, err)
		}

		commitPaths = append(commitPaths, filePath)
		fixCount += len(fixes)

		if cmd.OnFixed != nil {
			cmd.OnFixed(e.ID, fixes)
		}
	}

	if len(commitPaths) > 0 && h.committer != nil {
		msg := fmt.Sprintf("sdd: lint --fix applied %d fix(es) to %d entries", fixCount, len(commitPaths))
		if err := h.committer.Commit(msg, commitPaths...); err != nil {
			fmt.Fprintf(h.stderr, "warning: git commit failed: %v\n", err)
		}
	}

	return nil
}

// applyFixes mutates an entry to fix known mechanical issues and returns
// a description of each fix applied. Returns nil if nothing was fixable.
func applyFixes(e *model.Entry) []string {
	var fixes []string

	// Signals and decisions must carry a kind; fall back to the type default.
	if e.Kind == "" {
		if def := model.DefaultKindForType(e.Type); def != "" {
			e.Kind = def
			fixes = append(fixes, fmt.Sprintf("set kind to %s", def))
		}
	}

	return fixes
}
