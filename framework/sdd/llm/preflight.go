package llm

import (
	"context"
	"embed"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
	"text/template"
	"time"

	"github.com/networkteam/resonance/framework/sdd/model"
)

//go:embed preflight_templates/*.tmpl
var preflightTemplates embed.FS

// Severity classifies a pre-flight finding. Mirrored in the query package;
// templates describe severity in purely semantic terms.
type Severity string

const (
	SeverityHigh   Severity = "high"
	SeverityMedium Severity = "medium"
	SeverityLow    Severity = "low"
)

// Finding is a single observation from pre-flight validation.
type Finding struct {
	Severity    Severity
	Category    string
	Observation string
}

// PreflightResult holds the parsed findings from a pre-flight validator run.
// An empty Findings slice means the validator reported no findings.
type PreflightResult struct {
	Findings []Finding
}

// HasBlocking reports whether any finding blocks entry creation. Currently
// only SeverityHigh blocks.
func (r *PreflightResult) HasBlocking() bool {
	for _, f := range r.Findings {
		if f.Severity == SeverityHigh {
			return true
		}
	}
	return false
}

// Preflight runs the pre-flight validator against the given entry and graph.
// Returns the parsed result regardless of finding severity. Returns an error
// only for infrastructure failures (runner error, template error, parse error).
func Preflight(ctx context.Context, runner Runner, entry *model.Entry, graph *model.Graph) (*PreflightResult, error) {
	ct := selectCheckType(entry, graph)

	pctx, err := assembleContext(entry, graph, ct)
	if err != nil {
		return nil, fmt.Errorf("assembling pre-flight context: %w", err)
	}

	prompt, err := renderPreflightPrompt(ct, pctx)
	if err != nil {
		return nil, fmt.Errorf("rendering pre-flight prompt: %w", err)
	}

	start := time.Now()
	output, err := runner.Run(ctx, prompt)
	elapsed := time.Since(start)
	if err != nil {
		return nil, fmt.Errorf("running pre-flight validator: %w", err)
	}

	logCallResult(ctx, output.Meta, "preflight", elapsed)

	result, err := parsePreflightResult(output.Text)
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
// Acceptance criteria live inline in plan decision descriptions (as a
// `## Acceptance criteria` markdown section), so they flow through
// ProposedEntry and ClosedEntries without extra fields.
type preflightContext struct {
	ProposedEntry     string
	ReferencedEntries string
	ClosedEntries     string
	SupersededEntries string
	ActiveContracts   string
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
// Attachments are NOT read — AC lives inline in plan descriptions (see
// preflightContext doc). FormatEntryForPrompt includes each entry's
// Attachments path list so the validator agent can optionally read them
// when it deems necessary; pre-flight itself stays prompt-only.
func assembleContext(entry *model.Entry, graph *model.Graph, ct checkType) (*preflightContext, error) {
	pctx := &preflightContext{
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

	// Closed entries. The closed entry's description is included in full
	// via FormatEntryForPrompt — for plans, this carries the AC section
	// inline, so no separate extraction is needed.
	if len(entry.Closes) > 0 {
		var parts []string
		for _, id := range entry.Closes {
			e, ok := graph.ByID[id]
			if !ok {
				continue
			}
			parts = append(parts, FormatEntryForPrompt(e))
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
	if ct == checkDecisionRefs {
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

// renderPreflightPrompt renders the pre-flight prompt for the given check type and context.
// All templates are parsed together so partials (e.g. contracts.tmpl) are available.
func renderPreflightPrompt(ct checkType, pctx *preflightContext) (string, error) {
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

// findingLineRe matches a finding line: `[severity] category: observation`.
// Leading `- ` bullet is stripped before matching. Category is a token with
// no whitespace or colon; observation is the remainder of the line.
var findingLineRe = regexp.MustCompile(`^\[(?i:(high|medium|low))\]\s+([^\s:]+)\s*:\s*(.+)$`)

// parsePreflightResult parses the validator's raw output into a structured
// result. Expected format is one finding per line:
//
//	- [high] category: observation text
//	- [medium] category: observation text
//
// When the validator reports no findings, the output is the literal
// `No findings.` (trailing period and case-insensitive). Any other content
// — unknown severities, malformed lines — returns an error so the tooling
// layer can surface the infrastructure failure distinctly from findings.
func parsePreflightResult(output string) (*PreflightResult, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return nil, fmt.Errorf("empty pre-flight response")
	}

	if isNoFindings(output) {
		return &PreflightResult{}, nil
	}

	var findings []Finding
	for _, raw := range strings.Split(output, "\n") {
		line := strings.TrimSpace(raw)
		if line == "" {
			continue
		}
		if isNoFindings(line) {
			continue
		}
		line = strings.TrimPrefix(line, "- ")
		line = strings.TrimSpace(line)

		m := findingLineRe.FindStringSubmatch(line)
		if m == nil {
			return nil, fmt.Errorf("unexpected finding format: %q", raw)
		}
		sev, err := parseSeverity(m[1])
		if err != nil {
			return nil, err
		}
		findings = append(findings, Finding{
			Severity:    sev,
			Category:    m[2],
			Observation: strings.TrimSpace(m[3]),
		})
	}

	return &PreflightResult{Findings: findings}, nil
}

// isNoFindings reports whether s is the sentinel "No findings." line.
// The trailing period is tolerated as present or absent; comparison is
// case-insensitive after trimming.
func isNoFindings(s string) bool {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, ".")
	return strings.EqualFold(s, "no findings")
}

func parseSeverity(s string) (Severity, error) {
	switch strings.ToLower(s) {
	case "high":
		return SeverityHigh, nil
	case "medium":
		return SeverityMedium, nil
	case "low":
		return SeverityLow, nil
	default:
		return "", fmt.Errorf("unknown severity %q", s)
	}
}
