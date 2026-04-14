package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/networkteam/resonance/framework/sdd/command"
	"github.com/networkteam/resonance/framework/sdd/llm"
	"github.com/networkteam/resonance/framework/sdd/model"
)

// Summarize executes a SummarizeCmd: loads the graph, determines which
// entries need summaries, generates them via the LLM, and writes updated
// frontmatter back to disk. Entries are processed in topological (DAG)
// order so that ref summaries are available before downstream entries.
func (h *Handler) Summarize(ctx context.Context, cmd *command.SummarizeCmd) error {
	graph, err := h.reader.LoadGraph(h.graphDir)
	if err != nil {
		return fmt.Errorf("loading graph: %w", err)
	}

	// Determine which entries to process.
	var entries []*model.Entry
	if len(cmd.EntryIDs) > 0 {
		for _, id := range cmd.EntryIDs {
			e, ok := graph.ByID[id]
			if !ok {
				return fmt.Errorf("entry not found: %s", id)
			}
			entries = append(entries, e)
		}
	} else {
		// --all: process in topological order.
		entries = graph.TopologicalOrder()
	}

	var commitPaths []string

	timeout := cmd.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}

	for _, entry := range entries {
		ectx, cancel := context.WithTimeout(ctx, timeout)
		result, err := llm.Summarize(ectx, h.llmRunner, entry, graph, cmd.Force)
		cancel()
		if err != nil {
			return fmt.Errorf("summarizing %s: %w", entry.ID, err)
		}

		if result == nil {
			// Skipped — hash matched.
			if cmd.OnSkipped != nil {
				cmd.OnSkipped(entry.ID)
			}
			continue
		}

		// Update the in-memory entry so downstream entries see the new summary.
		entry.Summary = result.Summary
		entry.SummaryHash = result.SummaryHash

		// Write updated file to disk.
		relPath, err := model.IDToRelPath(entry.ID)
		if err != nil {
			return fmt.Errorf("computing path for %s: %w", entry.ID, err)
		}
		filePath := filepath.Join(h.graphDir, relPath)
		fileContent := model.FormatFrontmatter(entry) + "\n" + entry.Content + "\n"

		if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
			return fmt.Errorf("writing %s: %w", filePath, err)
		}
		commitPaths = append(commitPaths, filePath)

		if cmd.OnSummarized != nil {
			cmd.OnSummarized(entry.ID, result.Summary)
		}
	}

	// Commit all changes in one batch.
	if h.committer != nil && len(commitPaths) > 0 {
		msg := fmt.Sprintf("sdd: summarize %d entries", len(commitPaths))
		if len(cmd.EntryIDs) == 1 {
			msg = fmt.Sprintf("sdd: summarize %s", cmd.EntryIDs[0])
		}
		if err := h.committer.Commit(msg, commitPaths...); err != nil {
			fmt.Fprintf(h.stderr, "warning: git commit failed: %v\n", err)
		}
	}

	return nil
}
