package finders

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/networkteam/resonance/framework/sdd/model"
	"github.com/networkteam/resonance/framework/sdd/query"
)

//go:embed preflight_templates/*.tmpl
var preflightTemplates embed.FS

// Preflight runs the pre-flight validator against the given query.
// Returns the parsed result for both pass and fail cases.
// Returns an error only for infrastructure failures (runner error,
// template error, parse error) — a FAIL result is not an error.
func (f *Finder) Preflight(ctx context.Context, q query.PreflightQuery) (*query.PreflightResult, error) {
	ct := selectCheckType(q.Entry, q.Graph)

	pctx, err := assembleContext(q.Entry, q.Graph, ct)
	if err != nil {
		return nil, fmt.Errorf("assembling pre-flight context: %w", err)
	}

	prompt, err := renderPrompt(ct, pctx)
	if err != nil {
		return nil, fmt.Errorf("rendering pre-flight prompt: %w", err)
	}

	output, err := f.preflightRunner.Run(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("running pre-flight validator: %w", err)
	}

	result, err := parseResult(output)
	if err != nil {
		return nil, fmt.Errorf("parsing pre-flight result: %w", err)
	}
	return result, nil
}

// --- internal helpers ---

// checkType identifies which pre-flight check to run.
type checkType int

const (
	checkClosingAction       checkType = iota // action closing a decision/plan
	checkClosingDecision                      // decision closing signals
	checkDecisionRefs                         // decision with refs, no closes
	checkActionClosesSignals                  // action closing signals directly
	checkSignalCapture                        // signal validation
	checkSupersedes                           // supersedes operation
)

func (c checkType) String() string {
	switch c {
	case checkClosingAction:
		return "closing-action"
	case checkClosingDecision:
		return "closing-decision"
	case checkDecisionRefs:
		return "decision-refs"
	case checkActionClosesSignals:
		return "action-closes-signals"
	case checkSignalCapture:
		return "signal-capture"
	case checkSupersedes:
		return "supersedes"
	default:
		return fmt.Sprintf("unknown(%d)", int(c))
	}
}

var checkTypeTemplates = map[checkType]string{
	checkClosingAction:       "preflight_templates/closing_action.tmpl",
	checkClosingDecision:     "preflight_templates/closing_decision.tmpl",
	checkDecisionRefs:        "preflight_templates/decision_refs.tmpl",
	checkActionClosesSignals: "preflight_templates/action_closes_signals.tmpl",
	checkSignalCapture:       "preflight_templates/signal_capture.tmpl",
	checkSupersedes:          "preflight_templates/supersedes.tmpl",
}

// preflightContext holds all data needed to render a pre-flight prompt template.
type preflightContext struct {
	ProposedEntry     string
	ReferencedEntries string
	ClosedEntries     string
	SupersededEntries string
	ActiveContracts   string
	PlanItems         string
	OpenSignals       string
}

// selectCheckType determines the pre-flight check type from entry properties and graph context.
func selectCheckType(entry *model.Entry, graph *model.Graph) checkType {
	if len(entry.Supersedes) > 0 {
		return checkSupersedes
	}

	if entry.Type == model.TypeAction && len(entry.Closes) > 0 {
		for _, id := range entry.Closes {
			if target, ok := graph.ByID[id]; ok && target.Type == model.TypeDecision {
				return checkClosingAction
			}
		}
		return checkActionClosesSignals
	}

	if entry.Type == model.TypeDecision && len(entry.Closes) > 0 {
		return checkClosingDecision
	}

	if entry.Type == model.TypeDecision {
		return checkDecisionRefs
	}

	return checkSignalCapture
}

// assembleContext gathers graph data needed for the pre-flight prompt.
// It reads plan attachment content from disk when the graph was loaded from a directory.
func assembleContext(entry *model.Entry, graph *model.Graph, ct checkType) (*preflightContext, error) {
	pctx := &preflightContext{
		ProposedEntry: formatEntryForPrompt(entry),
	}

	// Referenced entries
	if len(entry.Refs) > 0 {
		var parts []string
		for _, id := range entry.Refs {
			if e, ok := graph.ByID[id]; ok {
				parts = append(parts, formatEntryForPrompt(e))
			}
		}
		if len(parts) > 0 {
			pctx.ReferencedEntries = strings.Join(parts, "\n\n---\n\n")
		}
	}

	// Closed entries and plan attachment content
	if len(entry.Closes) > 0 {
		var parts []string
		for _, id := range entry.Closes {
			e, ok := graph.ByID[id]
			if !ok {
				continue
			}
			parts = append(parts, formatEntryForPrompt(e))

			if e.IsPlan() && graph.GraphDir() != "" {
				for _, att := range e.Attachments {
					data, err := os.ReadFile(filepath.Join(graph.GraphDir(), att))
					if err != nil {
						continue
					}
					pctx.PlanItems += fmt.Sprintf("\n### Attachment: %s\n\n%s\n", filepath.Base(att), string(data))
				}
			}
		}
		if len(parts) > 0 {
			pctx.ClosedEntries = strings.Join(parts, "\n\n---\n\n")
		}
	}

	// Superseded entries
	if len(entry.Supersedes) > 0 {
		var parts []string
		for _, id := range entry.Supersedes {
			if e, ok := graph.ByID[id]; ok {
				parts = append(parts, formatEntryForPrompt(e))
			}
		}
		if len(parts) > 0 {
			pctx.SupersededEntries = strings.Join(parts, "\n\n---\n\n")
		}
	}

	// Active contracts (always included)
	contracts := graph.Contracts()
	if len(contracts) > 0 {
		var parts []string
		for _, c := range contracts {
			parts = append(parts, formatEntryForPrompt(c))
		}
		pctx.ActiveContracts = strings.Join(parts, "\n\n---\n\n")
	}

	// Open signals (for decision-refs check)
	if ct == checkDecisionRefs {
		signals := graph.OpenSignals()
		if len(signals) > 0 {
			var parts []string
			for _, s := range signals {
				parts = append(parts, formatEntryForPrompt(s))
			}
			pctx.OpenSignals = strings.Join(parts, "\n\n---\n\n")
		}
	}

	return pctx, nil
}

// formatEntryForPrompt formats an entry as readable text for inclusion in a prompt.
func formatEntryForPrompt(e *model.Entry) string {
	var b strings.Builder
	fmt.Fprintf(&b, "ID: %s\n", e.ID)
	fmt.Fprintf(&b, "Type: %s\n", e.Type)
	fmt.Fprintf(&b, "Layer: %s\n", e.Layer)
	if e.Kind != "" && e.Kind != model.KindDirective {
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
	fmt.Fprintf(&b, "\n%s", e.Content)
	return b.String()
}

// renderPrompt renders the pre-flight prompt for the given check type and context.
// All templates are parsed together so partials (e.g. contracts.tmpl) are available.
func renderPrompt(ct checkType, pctx *preflightContext) (string, error) {
	tmplName, ok := checkTypeTemplates[ct]
	if !ok {
		return "", fmt.Errorf("no template for check type %s", ct)
	}

	tmpl, err := template.ParseFS(preflightTemplates, "preflight_templates/*.tmpl")
	if err != nil {
		return "", fmt.Errorf("parsing templates: %w", err)
	}

	var b strings.Builder
	if err := tmpl.ExecuteTemplate(&b, filepath.Base(tmplName), pctx); err != nil {
		return "", fmt.Errorf("executing template %s: %w", tmplName, err)
	}

	return b.String(), nil
}

// parseResult parses the validator's raw output into a structured result.
func parseResult(output string) (*query.PreflightResult, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, fmt.Errorf("empty pre-flight response")
	}

	lines := strings.Split(output, "\n")
	first := strings.TrimSpace(lines[0])

	if strings.EqualFold(first, "PASS") {
		return &query.PreflightResult{Pass: true}, nil
	}

	if strings.EqualFold(first, "FAIL") {
		var gaps []string
		for _, line := range lines[1:] {
			line = strings.TrimSpace(line)
			if strings.HasPrefix(line, "- ") {
				gaps = append(gaps, strings.TrimPrefix(line, "- "))
			} else if line != "" {
				gaps = append(gaps, line)
			}
		}
		return &query.PreflightResult{Pass: false, Gaps: gaps}, nil
	}

	return nil, fmt.Errorf("unexpected pre-flight response: first line must be PASS or FAIL, got %q", first)
}
