package sdd

import (
	"context"
	"embed"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"text/template"

	"github.com/networkteam/resonance/framework/sdd/model"
)

//go:embed preflight_templates/*.tmpl
var preflightTemplates embed.FS

// CheckType identifies which pre-flight check to run.
type CheckType int

const (
	CheckClosingAction       CheckType = iota // action closing a decision/plan
	CheckClosingDecision                      // decision closing signals
	CheckDecisionRefs                         // decision with refs, no closes
	CheckActionClosesSignals                  // action closing signals directly
	CheckSignalCapture                        // signal validation
	CheckSupersedes                           // supersedes operation
)

func (c CheckType) String() string {
	switch c {
	case CheckClosingAction:
		return "closing-action"
	case CheckClosingDecision:
		return "closing-decision"
	case CheckDecisionRefs:
		return "decision-refs"
	case CheckActionClosesSignals:
		return "action-closes-signals"
	case CheckSignalCapture:
		return "signal-capture"
	case CheckSupersedes:
		return "supersedes"
	default:
		return fmt.Sprintf("unknown(%d)", int(c))
	}
}

var checkTypeTemplates = map[CheckType]string{
	CheckClosingAction:       "preflight_templates/closing_action.tmpl",
	CheckClosingDecision:     "preflight_templates/closing_decision.tmpl",
	CheckDecisionRefs:        "preflight_templates/decision_refs.tmpl",
	CheckActionClosesSignals: "preflight_templates/action_closes_signals.tmpl",
	CheckSignalCapture:       "preflight_templates/signal_capture.tmpl",
	CheckSupersedes:          "preflight_templates/supersedes.tmpl",
}

// PreflightRunner executes a prompt and returns the model's response.
type PreflightRunner interface {
	Run(ctx context.Context, prompt string) (string, error)
}

// PreflightContext holds all data needed to render a pre-flight prompt template.
type PreflightContext struct {
	ProposedEntry     string
	ReferencedEntries string
	ClosedEntries     string
	SupersededEntries string
	ActiveContracts   string
	PlanItems         string
	OpenSignals       string
}

// PreflightResult holds the parsed validator response.
type PreflightResult struct {
	Pass bool
	Gaps []string
}

// SelectCheckType determines the pre-flight check type from entry properties and graph context.
func SelectCheckType(entry *model.Entry, graph *model.Graph) CheckType {
	if len(entry.Supersedes) > 0 {
		return CheckSupersedes
	}

	if entry.Type == model.TypeAction && len(entry.Closes) > 0 {
		for _, id := range entry.Closes {
			if target, ok := graph.ByID[id]; ok && target.Type == model.TypeDecision {
				return CheckClosingAction
			}
		}
		return CheckActionClosesSignals
	}

	if entry.Type == model.TypeDecision && len(entry.Closes) > 0 {
		return CheckClosingDecision
	}

	if entry.Type == model.TypeDecision {
		return CheckDecisionRefs
	}

	return CheckSignalCapture
}

// AssembleContext gathers graph data needed for the pre-flight prompt.
// It reads plan attachment content from disk when the graph was loaded from a directory.
func AssembleContext(entry *model.Entry, graph *model.Graph, checkType CheckType) (*PreflightContext, error) {
	pctx := &PreflightContext{
		ProposedEntry: FormatEntryForPrompt(entry),
	}

	// Referenced entries
	if len(entry.Refs) > 0 {
		var parts []string
		for _, id := range entry.Refs {
			if e, ok := graph.ByID[id]; ok {
				parts = append(parts, FormatEntryForPrompt(e))
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
			parts = append(parts, FormatEntryForPrompt(e))

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
				parts = append(parts, FormatEntryForPrompt(e))
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
			parts = append(parts, FormatEntryForPrompt(c))
		}
		pctx.ActiveContracts = strings.Join(parts, "\n\n---\n\n")
	}

	// Open signals (for decision-refs check)
	if checkType == CheckDecisionRefs {
		signals := graph.OpenSignals()
		if len(signals) > 0 {
			var parts []string
			for _, s := range signals {
				parts = append(parts, FormatEntryForPrompt(s))
			}
			pctx.OpenSignals = strings.Join(parts, "\n\n---\n\n")
		}
	}

	return pctx, nil
}

// FormatEntryForPrompt formats an entry as readable text for inclusion in a prompt.
func FormatEntryForPrompt(e *model.Entry) string {
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

// RenderPrompt renders the pre-flight prompt for the given check type and context.
// All templates are parsed together so partials (e.g. contracts.tmpl) are available.
func RenderPrompt(checkType CheckType, pctx *PreflightContext) (string, error) {
	tmplName, ok := checkTypeTemplates[checkType]
	if !ok {
		return "", fmt.Errorf("no template for check type %s", checkType)
	}

	// Parse all templates together so {{ template "contracts" . }} works
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

// ParseResult parses the validator's raw output into a structured result.
func ParseResult(output string) (*PreflightResult, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, fmt.Errorf("empty pre-flight response")
	}

	lines := strings.Split(output, "\n")
	first := strings.TrimSpace(lines[0])

	if strings.EqualFold(first, "PASS") {
		return &PreflightResult{Pass: true}, nil
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
		return &PreflightResult{Pass: false, Gaps: gaps}, nil
	}

	return nil, fmt.Errorf("unexpected pre-flight response: first line must be PASS or FAIL, got %q", first)
}

// RunPreflight orchestrates the full pre-flight validation.
// Returns the result for both pass and fail cases.
// Returns an error only for infrastructure failures (runner error, template error, parse error).
func RunPreflight(ctx context.Context, runner PreflightRunner, entry *model.Entry, graph *model.Graph) (*PreflightResult, error) {
	checkType := SelectCheckType(entry, graph)

	pctx, err := AssembleContext(entry, graph, checkType)
	if err != nil {
		return nil, fmt.Errorf("assembling pre-flight context: %w", err)
	}

	prompt, err := RenderPrompt(checkType, pctx)
	if err != nil {
		return nil, fmt.Errorf("rendering pre-flight prompt: %w", err)
	}

	output, err := runner.Run(ctx, prompt)
	if err != nil {
		return nil, fmt.Errorf("running pre-flight validator: %w", err)
	}

	result, err := ParseResult(output)
	if err != nil {
		return nil, fmt.Errorf("parsing pre-flight result: %w", err)
	}

	return result, nil
}
