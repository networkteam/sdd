package llmops

import (
	"context"
	"embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/llm"
)

//go:embed summary_templates/*.tmpl
var summaryTemplates embed.FS

// SummarizeResult holds the generated summary.
type SummarizeResult struct {
	Summary string
}

// Summarize generates a summary for a single entry using the LLM runner.
// Summaries are derived on demand with no staleness tracking (d-cpt-4qi), so
// this always regenerates — the caller decides when to invoke it.
func Summarize(ctx context.Context, runner llm.Runner, entry *model.Entry, graph *model.Graph) (*SummarizeResult, error) {
	req, err := RenderSummaryPrompt(entry, graph)
	if err != nil {
		return nil, fmt.Errorf("rendering summary prompt: %w", err)
	}

	output, err := runner.Run(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("running summary generator: %w", err)
	}

	summary := strings.TrimSpace(output.Text)

	return &SummarizeResult{
		Summary: summary,
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
func RenderSummaryPrompt(entry *model.Entry, graph *model.Graph) (llm.Request, error) {
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

	addRelated(model.RefIDs(entry.Refs), "refs")
	addRelated(entry.Closes, "closes")
	addRelated(entry.Supersedes, "supersedes")

	if len(parts) > 0 {
		sctx.RelatedEntries = strings.Join(parts, "\n\n---\n\n")
	}

	tmpl, err := template.ParseFS(summaryTemplates, "summary_templates/*.tmpl")
	if err != nil {
		return llm.Request{}, fmt.Errorf("parsing summary templates: %w", err)
	}

	var sysB, userB strings.Builder
	if err := tmpl.ExecuteTemplate(&sysB, "summary_system", sctx); err != nil {
		return llm.Request{}, fmt.Errorf("executing summary_system template: %w", err)
	}
	if err := tmpl.ExecuteTemplate(&userB, "summary_user", sctx); err != nil {
		return llm.Request{}, fmt.Errorf("executing summary_user template: %w", err)
	}

	return llm.Request{
		Purpose:      llm.PurposeSummarize,
		SystemPrompt: strings.TrimSpace(sysB.String()),
		UserPrompt:   strings.TrimSpace(userB.String()),
	}, nil
}

// FormatEntryForPrompt formats an entry as readable text for inclusion in a prompt.
//
// Refs rendering is conditional on the entry's ref shape, per d-tac-4ub:
//
//   - All-legacy (every ref has Kind == RefKindUnknown — bare-string YAML
//     fallback): render the flat `Refs: id1, id2` format. Legacy refs carry no
//     kind or desc, so the object form would only add empty `(kind: unknown)`
//     noise.
//   - Object-form (any ref carries a capturable kind): render multi-line
//     `  - id (kind: K): desc` so the LLM sees ref metadata, enabling the
//     ref-meta consistency check (see ref_meta_consistency.tmpl).
//
// Mixed entries (some refs legacy, some object) get the multi-line form —
// the presence of any object-form ref signals an entry authored under the
// new contract; rendering uniformly preserves clarity.
//
// This renders canonical (parse-resolved) ref kinds — the single form both
// pre-flight and summary generation consume.
func FormatEntryForPrompt(e *model.Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ID: %s\n", e.ID)
	fmt.Fprintf(&b, "Type: %s\n", e.Type)
	fmt.Fprintf(&b, "Layer: %s\n", e.Layer)
	if e.Kind != "" {
		fmt.Fprintf(&b, "Kind: %s\n", e.Kind)
	}
	if len(e.Refs) > 0 {
		if allLegacyRefs(e.Refs) {
			fmt.Fprintf(&b, "Refs: %s\n", strings.Join(model.RefIDs(e.Refs), ", "))
		} else {
			b.WriteString("Refs:\n")
			for _, r := range e.Refs {
				fmt.Fprintf(&b, "  - %s (kind: %s)", r.ID, r.Kind)
				if r.Desc != "" {
					fmt.Fprintf(&b, ": %s", r.Desc)
				}
				b.WriteByte('\n')
			}
		}
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

// allLegacyRefs reports whether every ref in the slice is a legacy bare-string
// ref (Kind == RefKindUnknown). Used by FormatEntryForPrompt to fall back to
// the flat ref rendering for such entries, whose refs carry no kind/desc worth
// showing in object form.
func allLegacyRefs(refs []model.Ref) bool {
	for _, r := range refs {
		if r.Kind != model.RefKindUnknown {
			return false
		}
	}
	return true
}
