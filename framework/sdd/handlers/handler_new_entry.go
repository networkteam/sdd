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
	"github.com/networkteam/resonance/framework/sdd/query"
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
		path, err := saveStdinAttachment(h.graphDir, stdinAtt.Target, stdinAtt.Data)
		if err != nil {
			fmt.Fprintf(h.stderr, "warning: could not save stdin attachment: %v\n", err)
			return
		}
		fmt.Fprintf(h.stderr, "stdin attachment saved (%s): %s\n", reason, path)
		fmt.Fprintf(h.stderr, "  retry: cat %s | sdd new ... --attach -:%s\n", path, stdinAtt.Target)
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
	model.ValidateEntry(entry, graph)
	if len(entry.Warnings) > 0 {
		for _, w := range entry.Warnings {
			fmt.Fprintf(h.stderr, "error: %s\n", w.Message)
		}
		return fmt.Errorf("validation failed: %d issue(s)", len(entry.Warnings))
	}

	// Pre-flight validation (skipped or dispatched to the finder).
	if cmd.SkipPreflight {
		entry.Preflight = "skipped"
		fmt.Fprintf(h.stderr, "warning: pre-flight validation skipped\n")
	} else {
		timeout := cmd.PreflightTimeout
		if timeout == 0 {
			timeout = 120 * time.Second
		}
		pctx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		// Stage proposed attachments in memory so the validator can read
		// the plan's AC section before the files are written to disk.
		proposedAtt, err := stageProposedAttachments(cmd.Attachments, entry.Attachments)
		if err != nil {
			return fmt.Errorf("staging attachments for pre-flight: %w", err)
		}
		result, err := h.reader.Preflight(pctx, query.PreflightQuery{
			Entry:               entry,
			Graph:               graph,
			ProposedAttachments: proposedAtt,
			Model:               cmd.PreflightModel,
			Timeout:             timeout,
		})
		if err != nil {
			return fmt.Errorf("pre-flight error: %w (use --skip-preflight to bypass)", err)
		}
		// Always display findings — medium/low are informational signal,
		// not suppressed — but only block when severity warrants it.
		blocking := 0
		for _, f := range result.Findings {
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

	// Generate summary if an LLM runner is available.
	if h.llmRunner != nil {
		sTimeout := 60 * time.Second
		sctx, scancel := context.WithTimeout(ctx, sTimeout)
		result, err := llm.Summarize(sctx, h.llmRunner, entry, graph, true)
		scancel()
		if err != nil {
			fmt.Fprintf(h.stderr, "warning: summary generation failed: %v\n", err)
		} else if result != nil {
			entry.Summary = result.Summary
			entry.SummaryHash = result.SummaryHash
		}
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
		cmd.OnNewEntry(id)
	}
	return nil
}

// stageProposedAttachments returns a map from graph-dir-relative attachment
// paths (matching entry.Attachments) to the attachment content. Stdin
// attachments read from the in-memory Data field; file attachments read from
// disk. Returns nil when the command has no attachments. The slices are
// expected to be parallel — BuildEntry appends entry.Attachments in the
// same order as cmd.Attachments.
func stageProposedAttachments(cmdAtts []command.Attachment, entryAttPaths []string) (map[string][]byte, error) {
	if len(cmdAtts) == 0 {
		return nil, nil
	}
	if len(cmdAtts) != len(entryAttPaths) {
		return nil, fmt.Errorf("attachment count mismatch: cmd has %d, entry has %d", len(cmdAtts), len(entryAttPaths))
	}
	staged := make(map[string][]byte, len(cmdAtts))
	for i, a := range cmdAtts {
		if a.Source == "-" {
			staged[entryAttPaths[i]] = a.Data
			continue
		}
		data, err := os.ReadFile(a.Source)
		if err != nil {
			return nil, fmt.Errorf("reading attachment %s: %w", a.Source, err)
		}
		staged[entryAttPaths[i]] = data
	}
	return staged, nil
}

