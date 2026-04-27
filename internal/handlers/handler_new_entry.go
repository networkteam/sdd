package handlers

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/networkteam/slogutils"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/llm"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// NewEntry executes a NewEntryCmd: validates, loads the graph, runs
// pre-flight, writes the entry + attachments, commits, and fires
// cmd.OnNewEntry with the new ID. Stdin attachments are persisted to
// .sdd-tmp/ on pre-flight rejection and on every --dry-run invocation so
// the user can iterate without re-piping heredocs.
//
// Returns only errors. On dry-run success, returns nil without invoking
// the callback (no entry was created).
func (h *Handler) NewEntry(ctx context.Context, cmd *command.NewEntryCmd) (retErr error) {
	if err := cmd.Validate(); err != nil {
		return fmt.Errorf("invalid command: %w", err)
	}

	stdinAtt := cmd.StdinAttachment()
	stdinSaved := false

	// reportSavedStdin persists stdin and prints the retry hint to stderr.
	// Idempotent: only fires once per invocation (stdinSaved flag) so dry-run
	// + reject doesn't print the save twice.
	reportSavedStdin := func(reason string) {
		if stdinAtt == nil || stdinSaved {
			return
		}
		stdinSaved = true
		path, err := saveStdinAttachment(h.stdinTmpDir(), stdinAtt.Target, stdinAtt.Data)
		if err != nil {
			fmt.Fprintf(h.stderr, "warning: could not save stdin attachment: %v\n", err)
			return
		}
		fmt.Fprintf(h.stderr, "stdin attachment saved (%s): %s\n", reason, path)
		fmt.Fprintf(h.stderr, "  retry: sdd new ... --attach %s:%s\n", path, stdinAtt.Target)
	}

	// On dry-run, ensure stdin is saved on every exit path (including early
	// validation failure). Required by d-tac-q5p: save on every dry-run
	// (pass or fail). The idempotency flag inside reportSavedStdin makes
	// this safe to compose with explicit calls later in the flow.
	if cmd.DryRun {
		defer reportSavedStdin("dry-run")
	}

	// Build the entry in-memory.
	suffix, err := model.RandomSuffix(3)
	if err != nil {
		return fmt.Errorf("generating suffix: %w", err)
	}
	id := model.GenerateIDAt(cmd.Type, cmd.Layer, suffix, h.now())

	entry, err := cmd.BuildEntry(id)
	if err != nil {
		return err
	}

	// Load graph and validate entry against it.
	graph, err := h.reader.LoadGraph(h.graphDir)
	if err != nil {
		return fmt.Errorf("loading graph for validation: %w", err)
	}

	// Resolve short-form IDs in refs/closes/supersedes against the graph so
	// validation and all downstream logic see full IDs.
	if entry.Refs, err = graph.ResolveIDs(entry.Refs); err != nil {
		return fmt.Errorf("resolving refs: %w", err)
	}
	if entry.Closes, err = graph.ResolveIDs(entry.Closes); err != nil {
		return fmt.Errorf("resolving closes: %w", err)
	}
	if entry.Supersedes, err = graph.ResolveIDs(entry.Supersedes); err != nil {
		return fmt.Errorf("resolving supersedes: %w", err)
	}

	model.ValidateEntry(entry, graph)
	if len(entry.Warnings) > 0 {
		for _, w := range entry.Warnings {
			fmt.Fprintf(h.stderr, "error: %s\n", w.Message)
		}
		return fmt.Errorf("validation failed: %d issue(s)", len(entry.Warnings))
	}

	// Pre-flight and summary are independent LLM calls. Run them
	// concurrently via errgroup to save 30-60s of wall time per entry.
	// The errgroup context cancels the summary if pre-flight errors out
	// (timeout, LLM failure). Blocking findings are not group errors —
	// the main goroutine handles display and rejection after Wait.
	g, gctx := errgroup.WithContext(ctx)

	var pfResult *query.PreflightResult
	var pfErr error

	if cmd.SkipPreflight {
		entry.Preflight = "skipped"
	} else {
		g.Go(func() error {
			timeout := cmd.PreflightTimeout
			if timeout == 0 {
				timeout = 120 * time.Second
			}
			pctx, cancel := context.WithTimeout(gctx, timeout)
			defer cancel()
			result, err := h.reader.Preflight(pctx, query.PreflightQuery{
				Entry:   entry,
				Graph:   graph,
				Model:   cmd.PreflightModel,
				Timeout: timeout,
			})
			pfResult = result
			pfErr = err
			// Propagate hard errors to cancel the group context (and the
			// summary goroutine). Blocking findings are handled after Wait.
			return err
		})
	}

	var sumResult *llm.SummarizeResult

	if !cmd.DryRun && h.llmRunner != nil {
		g.Go(func() error {
			sctx, scancel := context.WithTimeout(gctx, 60*time.Second)
			defer scancel()
			result, err := llm.Summarize(sctx, h.llmRunner, entry, graph, true)
			if err != nil {
				slogutils.FromContext(gctx).Warn("summary generation failed", "err", err)
				return nil // non-fatal: entry is valid without a summary
			}
			sumResult = result
			return nil
		})
	}

	_ = g.Wait() // error comes from pre-flight; handled below via pfErr

	// Process pre-flight results on the main goroutine (no concurrent stderr writes).
	if cmd.SkipPreflight {
		fmt.Fprintf(h.stderr, "warning: pre-flight validation skipped\n")
	} else if pfErr != nil {
		return fmt.Errorf("pre-flight error: %w (use --skip-preflight to bypass)", pfErr)
	} else {
		blocking := 0
		for _, f := range pfResult.Findings {
			fmt.Fprintf(h.stderr, "  [%s] %s: %s\n", f.Severity, f.Category, f.Observation)
			if f.Severity == query.SeverityHigh {
				blocking++
			}
		}
		if blocking > 0 {
			fmt.Fprintf(h.stderr, "pre-flight validation blocked: %d high-severity finding(s)\n", blocking)
			reportSavedStdin("pre-flight rejected")
			return fmt.Errorf("pre-flight rejected entry")
		}
	}

	// Dry-run: stop before writing. The deferred save fires on return.
	if cmd.DryRun {
		return nil
	}

	// Apply summary from the concurrent goroutine.
	if sumResult != nil {
		entry.Summary = sumResult.Summary
		entry.SummaryHash = sumResult.SummaryHash
	}

	// Write entry file.
	relPath, err := model.IDToRelPath(id)
	if err != nil {
		return fmt.Errorf("computing path for %s: %w", id, err)
	}
	filePath := filepath.Join(h.graphDir, relPath)
	fileContent := model.FormatFrontmatter(entry) + "\n" + entry.Content + "\n"

	if err := os.MkdirAll(filepath.Dir(filePath), 0755); err != nil {
		return fmt.Errorf("creating directories: %w", err)
	}
	if err := os.WriteFile(filePath, []byte(fileContent), 0644); err != nil {
		return fmt.Errorf("writing %s: %w", filePath, err)
	}

	commitPaths := []string{filePath}

	// Copy attachments.
	if len(cmd.Attachments) > 0 {
		attachDirRel, err := model.AttachDirRelPath(id)
		if err != nil {
			return fmt.Errorf("computing attachment dir for %s: %w", id, err)
		}
		attachDir := filepath.Join(h.graphDir, attachDirRel)
		if err := os.MkdirAll(attachDir, 0755); err != nil {
			return fmt.Errorf("creating attachment directory: %w", err)
		}
		for _, a := range cmd.Attachments {
			var data []byte
			if a.Source == "-" {
				data = a.Data
			} else {
				b, err := os.ReadFile(a.Source)
				if err != nil {
					return fmt.Errorf("reading attachment %s: %w", a.Source, err)
				}
				data = b
			}
			dst := filepath.Join(attachDir, a.Target)
			if err := os.WriteFile(dst, data, 0644); err != nil {
				return fmt.Errorf("writing attachment %s: %w", dst, err)
			}
			commitPaths = append(commitPaths, dst)
		}
	}

	// Commit. Warn but don't fail if git refuses.
	if h.committer != nil {
		msg := fmt.Sprintf("sdd: %s %s %s", entry.TypeLabel(), entry.LayerLabel(), entry.ShortContent(72))
		if err := h.committer.Commit(msg, commitPaths...); err != nil {
			fmt.Fprintf(h.stderr, "warning: git commit failed: %v\n", err)
		}
	}

	if cmd.OnNewEntry != nil {
		cmd.OnNewEntry(id, entry.Summary)
	}
	return nil
}
