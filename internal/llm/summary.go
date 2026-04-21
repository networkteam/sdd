package llm

import (
	"context"
	"crypto/sha256"
	"embed"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

//go:embed summary_templates/*.tmpl
var summaryTemplates embed.FS

// SummarizeResult holds the generated summary and its prompt hash.
type SummarizeResult struct {
	Summary     string
	SummaryHash string
}

// Summarize generates a summary for a single entry using the LLM runner.
// Returns nil if the entry's stored SummaryHash matches the computed hash
// (skip). Set force to regenerate regardless.
func Summarize(ctx context.Context, runner Runner, entry *model.Entry, graph *model.Graph, force bool) (*SummarizeResult, error) {
	req, err := RenderSummaryPrompt(entry, graph)
	if err != nil {
		return nil, fmt.Errorf("rendering summary prompt: %w", err)
	}

	hash := ComputePromptHash(req.Combined())

	// Skip if hash matches — summary is current.
	if !force && entry.SummaryHash == hash {
		return nil, nil
	}

	start := time.Now()
	output, err := runner.Run(ctx, req)
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("running summary generator: %w", err)
	}

	logCallResult(ctx, output.Meta, "summarize", elapsed)

	summary := strings.TrimSpace(output.Text)

	return &SummarizeResult{
		Summary:     summary,
		SummaryHash: hash,
	}, nil
}

// summaryContext holds all data needed to render the summary prompt template.
type summaryContext struct {
	EntryContent   string
	RelatedEntries string
}

// RenderSummaryPrompt renders the summary prompt for an entry. Returns a
// Request with the full rendered prompt in UserPrompt; the system/user split
// is introduced when templates are refactored (see the plan decision).
func RenderSummaryPrompt(entry *model.Entry, graph *model.Graph) (Request, error) {
	sctx := &summaryContext{
		EntryContent: FormatEntryForPrompt(entry),
	}

	// Collect direct refs, closes, and supersedes entries.
	var parts []string
	seen := make(map[string]bool)

	addRelated := func(ids []string, relation string) {
		for _, id := range ids {
			if seen[id] {
				continue
			}
			seen[id] = true
			e, ok := graph.ByID[id]
			if !ok {
				continue
			}
			// Use summary if available, otherwise full content. Render the
			// layer/kind/type triple (e.g. "strategic insight signal") so the
			// LLM can route on kind — carrying only the bare type would hide
			// whether a referenced signal is a gap, insight, question, etc.
			if e.Summary != "" {
				triple := e.LayerLabel()
				if e.Kind != "" {
					triple += " " + string(e.Kind)
				}
				triple += " " + e.TypeLabel()
				parts = append(parts, fmt.Sprintf("[%s] %s (ID: %s)\nSummary: %s", relation, triple, e.ID, e.Summary))
			} else {
				parts = append(parts, fmt.Sprintf("[%s] %s", relation, FormatEntryForPrompt(e)))
			}
		}
	}

	addRelated(entry.Refs, "refs")
	addRelated(entry.Closes, "closes")
	addRelated(entry.Supersedes, "supersedes")

	if len(parts) > 0 {
		sctx.RelatedEntries = strings.Join(parts, "\n\n---\n\n")
	}

	tmpl, err := template.ParseFS(summaryTemplates, "summary_templates/*.tmpl")
	if err != nil {
		return Request{}, fmt.Errorf("parsing summary templates: %w", err)
	}

	var sysB, userB strings.Builder
	if err := tmpl.ExecuteTemplate(&sysB, "summary_system", sctx); err != nil {
		return Request{}, fmt.Errorf("executing summary_system template: %w", err)
	}
	if err := tmpl.ExecuteTemplate(&userB, "summary_user", sctx); err != nil {
		return Request{}, fmt.Errorf("executing summary_user template: %w", err)
	}

	return Request{
		SystemPrompt: strings.TrimSpace(sysB.String()),
		UserPrompt:   strings.TrimSpace(userB.String()),
	}, nil
}

// FormatEntryForPrompt formats an entry as readable text for inclusion in a prompt.
func FormatEntryForPrompt(e *model.Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ID: %s\n", e.ID)
	fmt.Fprintf(&b, "Type: %s\n", e.Type)
	fmt.Fprintf(&b, "Layer: %s\n", e.Layer)
	if e.Kind != "" {
		fmt.Fprintf(&b, "Kind: %s\n", e.Kind)
	}
	if len(e.Refs) > 0 {
		fmt.Fprintf(&b, "Refs: %s\n", strings.Join(e.Refs, ", "))
	}
	if len(e.Closes) > 0 {
		fmt.Fprintf(&b, "Closes: %s\n", strings.Join(e.Closes, ", "))
	}
	if len(e.Supersedes) > 0 {
		fmt.Fprintf(&b, "Supersedes: %s\n", strings.Join(e.Supersedes, ", "))
	}
	if e.Confidence != "" {
		fmt.Fprintf(&b, "Confidence: %s\n", e.Confidence)
	}
	if len(e.Attachments) > 0 {
		fmt.Fprintf(&b, "Attachments: %s\n", strings.Join(e.Attachments, ", "))
	}
	fmt.Fprintf(&b, "\n%s", e.Content)
	return b.String()
}

// ComputePromptHash returns the hex-encoded SHA-256 hash of the rendered prompt.
// Uses the first 16 bytes (32 hex chars) — enough for collision avoidance.
func ComputePromptHash(prompt string) string {
	h := sha256.Sum256([]byte(prompt))
	return fmt.Sprintf("%x", h[:16])
}
