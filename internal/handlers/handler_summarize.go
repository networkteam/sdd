package handlers

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"golang.org/x/sync/errgroup"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
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
// One call writes an entry's summary and title together. An --all run that
// finds a summary without a title asks for the title alone, and one that finds
// a title without a summary keeps the title, so what was set by hand survives
// the fill.
//
// When cmd.ExplicitText or cmd.ExplicitTitle is set, the LLM is bypassed
// entirely: the supplied text is written on a single named entry.
func (h *Handler) Summarize(ctx context.Context, cmd *command.SummarizeCmd) error {
	graph, err := h.reader.CurrentGraph(h.graphDir)
	if err != nil {
		return fmt.Errorf("loading graph: %w", err)
	}

	// Whether this is an --all run (no entry IDs) is decided before ID
	// resolution reassigns cmd.EntryIDs below.
	isAll := len(cmd.EntryIDs) == 0

	// Explicit-text path: single entry, no LLM call.
	if cmd.ExplicitText != nil || cmd.ExplicitTitle != nil {
		if len(cmd.EntryIDs) != 1 {
			return fmt.Errorf("--text and --title require exactly one entry ID (no --all, no multi-entry batches)")
		}
		return h.summarizeExplicit(graph, cmd.EntryIDs[0], cmd)
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

	// --all fills only what is missing unless --force regenerates every
	// entry; named entries always regenerate. Decided before any worker
	// writes, so the planned count is known up front.
	regenerate := !isAll || cmd.Force
	var planned []*model.Entry
	for _, entry := range entries {
		if !regenerate && entry.Summary != "" && entry.Title != "" {
			if cmd.OnSkipped != nil {
				cmd.OnSkipped(entry.ID)
			}
			continue
		}
		planned = append(planned, entry)
	}
	if cmd.OnPlanned != nil {
		cmd.OnPlanned(len(planned))
	}

	var finder *finders.SearchFinder
	if len(planned) > 0 {
		finder, err = h.openTitleSearch(ctx, graph)
		if err != nil {
			return err
		}
	}

	for _, entry := range planned {
		g.Go(func() error {
			// Search and render under read lock — other workers may be
			// writing their own entries' summaries and titles concurrently,
			// and those feed this prompt and the search's candidates.
			graphMu.RLock()
			titles, searchErr := similarTitles(ctx, finder, entry)
			var description *llmops.Description
			var renderErr error
			if searchErr == nil {
				description, renderErr = llmops.NewDescription(entry, graph, h.language, regenerate, titles)
			}
			graphMu.RUnlock()
			if searchErr != nil {
				return searchErr
			}
			if renderErr != nil {
				return fmt.Errorf("rendering summary for %s: %w", entry.ID, renderErr)
			}
			titleOnly := description.TitleOnly()

			// LLM calls without the graph lock; the injected runner bounds them.
			result, err := description.Run(ctx, h.llmRunner)
			if err != nil {
				return fmt.Errorf("summarizing %s: %w", entry.ID, err)
			}
			graphMu.RLock()
			retitle, err := description.Retitle(result)
			graphMu.RUnlock()
			if err != nil {
				return fmt.Errorf("rendering title for %s: %w", entry.ID, err)
			}
			if retitle != nil {
				retitled, err := retitle.Run(ctx, h.llmRunner)
				if err != nil {
					return fmt.Errorf("titling %s: %w", entry.ID, err)
				}
				result.Title = retitled.Title
			}

			// Apply the answer under write lock, then format and write file
			// under read lock so FormatFrontmatter sees a consistent entry.
			graphMu.Lock()
			if regenerate || entry.Title == "" {
				entry.Title = result.Title
			}
			if !titleOnly {
				entry.Summary = result.Summary
			}
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

			switch {
			case titleOnly && cmd.OnTitled != nil:
				cmd.OnTitled(entry.ID, result.Title)
			case !titleOnly && cmd.OnSummarized != nil:
				cmd.OnSummarized(entry.ID, result.Summary)
			}
			return nil
		})
	}

	// A failed entry still commits what the others wrote in one batch: a
	// rerun skips entries already filled, so an uncommitted file would never
	// be committed.
	runErr := g.Wait()
	if h.committer != nil && len(commitPaths) > 0 {
		msg := fmt.Sprintf("sdd: summarize %d entries", len(commitPaths))
		if len(cmd.EntryIDs) == 1 {
			msg = fmt.Sprintf("sdd: summarize %s", cmd.EntryIDs[0])
		}
		if err := h.committer.Commit(msg, commitPaths...); err != nil {
			return errors.Join(runErr, fmt.Errorf("git commit: %w", err))
		}
	}
	return runErr
}

// summarizeExplicit writes a user-supplied summary, title or both on a single
// entry, bypassing the LLM.
func (h *Handler) summarizeExplicit(graph *model.Graph, idArg string, cmd *command.SummarizeCmd) error {
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

	if cmd.ExplicitText != nil {
		summary := strings.TrimSpace(*cmd.ExplicitText)
		if summary == "" {
			return fmt.Errorf("--text value is empty after trimming whitespace")
		}
		entry.Summary = summary
	}
	if cmd.ExplicitTitle != nil {
		title := model.NormalizeTitle(*cmd.ExplicitTitle)
		if title == "" {
			return fmt.Errorf("--title value is empty after trimming whitespace, quotes and trailing punctuation")
		}
		if strings.ContainsAny(title, "\r\n") {
			return fmt.Errorf("--title value spans more than one line")
		}
		entry.Title = title
	}

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

	if cmd.ExplicitText != nil && cmd.OnSummarized != nil {
		cmd.OnSummarized(entry.ID, entry.Summary)
	}
	if cmd.ExplicitTitle != nil && cmd.OnTitled != nil {
		cmd.OnTitled(entry.ID, entry.Title)
	}
	return nil
}
