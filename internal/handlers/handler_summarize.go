package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
)

// Summarize executes a SummarizeCmd: loads the graph, determines which
// entries need summaries, generates them via the LLM, and writes updated
// frontmatter back to disk. Batch runs (--all or multiple entries) use an
// errgroup with SetLimit(concurrency) — for remote providers the factory
// applies a rate limiter on top. A short-lived mutex guards graph reads
// and writes; the LLM call itself runs without the lock.
func (h *Handler) Summarize(ctx context.Context, cmd *command.SummarizeCmd) error {
	graph, err := h.reader.LoadGraph(h.graphDir)
	if err != nil {
		return fmt.Errorf("loading graph: %w", err)
	}

	// Determine which entries to process.
	var entries []*model.Entry
	if len(cmd.EntryIDs) > 0 {
		resolvedIDs, err := graph.ResolveIDs(cmd.EntryIDs)
		if err != nil {
			return err
		}
		cmd.EntryIDs = resolvedIDs
		for _, id := range resolvedIDs {
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

	timeout := cmd.Timeout
	if timeout == 0 {
		timeout = 60 * time.Second
	}
	concurrency := cmd.Concurrency
	if concurrency < 1 {
		concurrency = model.DefaultLLMConcurrency
	}

	var (
		graphMu     sync.RWMutex
		pathsMu     sync.Mutex
		commitPaths []string
	)

	// Plain errgroup (not WithContext): a single entry's timeout must not
	// cancel the siblings. g.Wait still returns the first error, but
	// in-flight workers complete or time out individually using ctx.
	var g errgroup.Group
	g.SetLimit(concurrency)

	for _, entry := range entries {
		g.Go(func() error {
			// Render prompt and check hash under read lock — other workers
			// may be writing their own entries concurrently.
			graphMu.RLock()
			req, renderErr := llm.RenderSummaryPrompt(entry, graph)
			var currentHash string
			if renderErr == nil {
				currentHash = llm.ComputePromptHash(req.Combined())
			}
			existingHash := entry.SummaryHash
			graphMu.RUnlock()
			if renderErr != nil {
				return fmt.Errorf("rendering summary for %s: %w", entry.ID, renderErr)
			}

			if !cmd.Force && existingHash == currentHash {
				if cmd.OnSkipped != nil {
					cmd.OnSkipped(entry.ID)
				}
				return nil
			}

			// LLM call without the graph lock.
			ectx, cancel := context.WithTimeout(ctx, timeout)
			defer cancel()
			output, err := llm.Run(ectx, h.llmRunner, req, "summarize")
			if err != nil {
				return fmt.Errorf("summarizing %s: %w", entry.ID, err)
			}
			summary := strings.TrimSpace(output.Text)

			// Apply summary under write lock, then format and write file
			// under read lock so FormatFrontmatter sees a consistent entry.
			graphMu.Lock()
			entry.Summary = summary
			entry.SummaryHash = currentHash
			graphMu.Unlock()

			relPath, err := model.IDToRelPath(entry.ID)
			if err != nil {
				return fmt.Errorf("computing path for %s: %w", entry.ID, err)
			}
			filePath := filepath.Join(h.graphDir, relPath)

			graphMu.RLock()
			fileContent := model.FormatFrontmatter(entry) + "\n" + entry.Content + "\n"
			graphMu.RUnlock()

			if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
				return fmt.Errorf("writing %s: %w", filePath, err)
			}

			pathsMu.Lock()
			commitPaths = append(commitPaths, filePath)
			pathsMu.Unlock()

			if cmd.OnSummarized != nil {
				cmd.OnSummarized(entry.ID, summary)
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return err
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
