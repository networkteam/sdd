package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/llmops"
	"github.com/networkteam/sdd/internal/model"
)

// Summarize executes a SummarizeCmd: loads the graph, determines which
// entries to (re)generate, generates them via the LLM, and writes updated
// frontmatter back to disk. Summaries are derived on demand with no staleness
// tracking (d-cpt-4qi): named entries always regenerate, while --all fills
// only entries that have no summary yet (--force regenerates every entry).
// Batch runs use an errgroup with SetLimit(concurrency) — for remote providers
// the factory applies a rate limiter on top. A short-lived mutex guards graph
// reads and writes; the LLM call itself runs without the lock.
//
// When cmd.ExplicitText is set, the LLM is bypassed entirely: the supplied
// text is written as the summary on a single named entry.
func (h *Handler) Summarize(ctx context.Context, cmd *command.SummarizeCmd) error {
	graph, err := h.reader.CurrentGraph(h.graphDir)
	if err != nil {
		return fmt.Errorf("loading graph: %w", err)
	}

	// Whether this is an --all run (no entry IDs) is decided before ID
	// resolution reassigns cmd.EntryIDs below.
	isAll := len(cmd.EntryIDs) == 0

	// Explicit-text path: single entry, no LLM call.
	if cmd.ExplicitText != nil {
		if len(cmd.EntryIDs) != 1 {
			return fmt.Errorf("--text requires exactly one entry ID (no --all, no multi-entry batches)")
		}
		return h.summarizeExplicit(graph, cmd.EntryIDs[0], *cmd.ExplicitText, cmd.OnSummarized)
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
			if e.Embedded {
				return fmt.Errorf("%s is an embedded base entry — its summary ships with the binary and cannot be regenerated here", id)
			}
			entries = append(entries, e)
		}
	} else {
		// --all: process in topological order. Embedded base entries have no
		// file to write a summary into — their summaries ship with the binary.
		for _, e := range graph.TopologicalOrder() {
			if e.Embedded {
				continue
			}
			entries = append(entries, e)
		}
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
			// --all fills only entries that have no summary yet unless
			// --force regenerates every entry; named entries always
			// regenerate. Read under the lock — other workers may be writing
			// their own entries' summaries concurrently.
			graphMu.RLock()
			alreadySummarized := entry.Summary != ""
			graphMu.RUnlock()
			if isAll && !cmd.Force && alreadySummarized {
				if cmd.OnSkipped != nil {
					cmd.OnSkipped(entry.ID)
				}
				return nil
			}

			// Render prompt under read lock — other workers may be writing
			// their own entries' summaries concurrently, and those feed
			// neighbor prose into this prompt.
			graphMu.RLock()
			req, renderErr := llmops.RenderSummaryPrompt(entry, graph)
			graphMu.RUnlock()
			if renderErr != nil {
				return fmt.Errorf("rendering summary for %s: %w", entry.ID, renderErr)
			}

			// LLM call without the graph lock; the injected runner bounds it.
			output, err := h.llmRunner.Run(ctx, req)
			if err != nil {
				return fmt.Errorf("summarizing %s: %w", entry.ID, err)
			}
			summary := strings.TrimSpace(output.Text)

			// Apply summary under write lock, then format and write file
			// under read lock so FormatFrontmatter sees a consistent entry.
			graphMu.Lock()
			entry.Summary = summary
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
			return fmt.Errorf("git commit: %w", err)
		}
	}

	return nil
}

// summarizeExplicit writes user-supplied summary text on a single entry,
// bypassing the LLM.
func (h *Handler) summarizeExplicit(graph *model.Graph, idArg, text string, onSummarized func(id, summary string)) error {
	resolved, err := graph.ResolveIDs([]string{idArg})
	if err != nil {
		return err
	}
	id := resolved[0]
	entry, ok := graph.ByID[id]
	if !ok {
		return fmt.Errorf("entry not found: %s", id)
	}
	if entry.Embedded {
		return fmt.Errorf("%s is an embedded base entry — its summary ships with the binary and cannot be replaced here", id)
	}

	summary := strings.TrimSpace(text)
	if summary == "" {
		return fmt.Errorf("--text value is empty after trimming whitespace")
	}

	entry.Summary = summary

	relPath, err := model.IDToRelPath(entry.ID)
	if err != nil {
		return fmt.Errorf("computing path for %s: %w", entry.ID, err)
	}
	filePath := filepath.Join(h.graphDir, relPath)

	fileContent := model.FormatFrontmatter(entry) + "\n" + entry.Content + "\n"
	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", filePath, err)
	}

	if h.committer != nil {
		msg := fmt.Sprintf("sdd: summarize %s (manual)", entry.ID)
		if err := h.committer.Commit(msg, filePath); err != nil {
			return fmt.Errorf("git commit: %w", err)
		}
	}

	if onSummarized != nil {
		onSummarized(entry.ID, summary)
	}
	return nil
}
