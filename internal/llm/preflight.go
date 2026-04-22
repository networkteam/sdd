package llm

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"
	"time"

	"github.com/networkteam/sdd/internal/model"
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

	pctx := assembleContext(entry, graph, ct)

	req, err := renderPreflightPrompt(ct, pctx)
	if err != nil {
		return nil, fmt.Errorf("rendering pre-flight prompt: %w", err)
	}

	start := time.Now()
	output, err := runner.Run(ctx, req)
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

// checkType identifies which pre-flight check to run. Templates organize per
// check transaction, not per kind — dispatch is kind-aware but the rendered
// prompt stays scoped to the shape of the operation.
type checkType int

const (
	checkClosingDone       checkType = iota // done signal (or legacy action) closing a decision
	checkClosingDecision                    // decision closing signals or stable-kind entries
	checkDecisionRefs                       // decision with refs, no closes
	checkShortLoop                          // done signal (or legacy action) closing a signal directly
	checkDissolution                        // fact/insight signal closing a question
	checkAspirationCapture                  // aspiration decision captured without closes
	checkSignalCapture                      // signal validation
	checkSupersedes                         // supersedes operation
)

func (c checkType) String() string {
	switch c {
	case checkClosingDone:
		return "closing-done"
	case checkClosingDecision:
		return "closing-decision"
	case checkDecisionRefs:
		return "decision-refs"
	case checkShortLoop:
		return "short-loop"
	case checkDissolution:
		return "dissolution"
	case checkAspirationCapture:
		return "aspiration-capture"
	case checkSignalCapture:
		return "signal-capture"
	case checkSupersedes:
		return "supersedes"
	default:
		return fmt.Sprintf("unknown(%d)", int(c))
	}
}

// checkTypeTemplates maps each check type to its template basename. The render
// function derives the system and user template names by appending _system and
// _user. Each check-template file defines both blocks.
var checkTypeTemplates = map[checkType]string{
	checkClosingDone:       "closing_done",
	checkClosingDecision:   "closing_decision",
	checkDecisionRefs:      "decision_refs",
	checkShortLoop:         "short_loop",
	checkDissolution:       "dissolution",
	checkAspirationCapture: "aspiration_capture",
	checkSignalCapture:     "signal_capture",
	checkSupersedes:        "supersedes",
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
	ActiveAspirations string
	OpenSignals       string
}

// selectCheckType determines the pre-flight check type from entry properties and graph context.
// Dispatch is kind-aware: the same structural shape (signal with closes) routes to different
// templates based on the entry's kind and the closed target's kind.
func selectCheckType(entry *model.Entry, graph *model.Graph) checkType {
	if len(entry.Supersedes) > 0 {
		return checkSupersedes
	}

	// Done-kind signals closing entries route by target kind: closing_done for
	// decisions, short_loop for signals. Unusual close patterns within signal
	// closures get flagged by the unusual_close partial.
	isCompletionRecord := entry.Type == model.TypeSignal && entry.Kind == model.KindDone
	if isCompletionRecord && len(entry.Closes) > 0 {
		for _, id := range entry.Closes {
			if target, ok := graph.ByID[id]; ok && target.Type == model.TypeDecision {
				return checkClosingDone
			}
		}
		return checkShortLoop
	}

	// Fact or insight closing an entry — dissolution. The dissolution template
	// targets question closures; non-question targets are flagged as unusual
	// close patterns by the shared partial.
	if entry.Type == model.TypeSignal &&
		(entry.Kind == model.KindFact || entry.Kind == model.KindInsight) &&
		len(entry.Closes) > 0 {
		return checkDissolution
	}

	if entry.Type == model.TypeDecision && len(entry.Closes) > 0 {
		return checkClosingDecision
	}

	if entry.Type == model.TypeDecision {
		if entry.Kind == model.KindAspiration {
			return checkAspirationCapture
		}
		return checkDecisionRefs
	}

	return checkSignalCapture
}

// assembleContext gathers graph data needed for the pre-flight prompt.
// Attachments are NOT read — AC lives inline in plan descriptions (see
// preflightContext doc). FormatEntryForPrompt includes each entry's
// Attachments path list so the validator agent can optionally read them
// when it deems necessary; pre-flight itself stays prompt-only.
func assembleContext(entry *model.Entry, graph *model.Graph, ct checkType) *preflightContext {
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

	// Active aspirations (for aspiration-capture check — the constellation
	// the new aspiration is joining, used to detect tensions).
	if ct == checkAspirationCapture {
		aspirations := graph.Aspirations()
		if len(aspirations) > 0 {
			var parts []string
			for _, a := range aspirations {
				parts = append(parts, FormatEntryForPrompt(a))
			}
			pctx.ActiveAspirations = strings.Join(parts, "\n\n---\n\n")
		}
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

	return pctx
}

// renderPreflightPrompt renders the pre-flight prompt for the given check type
// and context as a two-part Request. The _system block carries the stable
// instructions, calibration, and graph-scoped context (contracts, open signals);
// the _user block carries the proposed entry and its direct refs/closes.
// Splitting the prompt this way lets providers that support prompt caching
// (Anthropic) treat the system portion as a cacheable prefix. All templates
// are parsed together so partials (contracts, verdict, etc.) are available.
func renderPreflightPrompt(ct checkType, pctx *preflightContext) (Request, error) {
	base, ok := checkTypeTemplates[ct]
	if !ok {
		return Request{}, fmt.Errorf("no template for check type %s", ct)
	}

	tmpl, err := template.ParseFS(preflightTemplates, "preflight_templates/*.tmpl")
	if err != nil {
		return Request{}, fmt.Errorf("parsing templates: %w", err)
	}

	var sysB, userB strings.Builder
	if err := tmpl.ExecuteTemplate(&sysB, base+"_system", pctx); err != nil {
		return Request{}, fmt.Errorf("executing template %s_system: %w", base, err)
	}
	if err := tmpl.ExecuteTemplate(&userB, base+"_user", pctx); err != nil {
		return Request{}, fmt.Errorf("executing template %s_user: %w", base, err)
	}

	return Request{
		SystemPrompt: strings.TrimSpace(sysB.String()),
		UserPrompt:   strings.TrimSpace(userB.String()),
	}, nil
}

// parsePreflightResult parses the validator's JSON response into a
// structured result. The LLM is asked to respond with:
//
//	{"findings": [{"severity": "high|medium|low", "category": "...", "observation": "..."}]}
//
// Empty findings array means "no findings". The parser tolerates prose
// surrounding the JSON object (LLM preambles, code fences) by scanning for
// the first balanced {...}. Malformed JSON, missing keys, unknown severity
// values — all return errors so infrastructure failures stay distinct from
// findings.
func parsePreflightResult(output string) (*PreflightResult, error) {
	jsonText, err := extractJSONObject(output)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Findings []struct {
			Severity    string `json:"severity"`
			Category    string `json:"category"`
			Observation string `json:"observation"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(jsonText), &resp); err != nil {
		return nil, fmt.Errorf("parsing pre-flight JSON: %w", err)
	}

	findings := make([]Finding, 0, len(resp.Findings))
	for i, f := range resp.Findings {
		sev, err := parseSeverity(f.Severity)
		if err != nil {
			return nil, fmt.Errorf("finding %d: %w", i, err)
		}
		if f.Category == "" {
			return nil, fmt.Errorf("finding %d: category is empty", i)
		}
		if f.Observation == "" {
			return nil, fmt.Errorf("finding %d: observation is empty", i)
		}
		findings = append(findings, Finding{
			Severity:    sev,
			Category:    f.Category,
			Observation: f.Observation,
		})
	}

	return &PreflightResult{Findings: findings}, nil
}

// extractJSONObject returns the first balanced {...} in the input, skipping
// any surrounding prose or code fences. Returns an error if no object is
// found or braces are unbalanced. String-escape aware so braces inside
// JSON strings don't confuse the balance counter.
func extractJSONObject(output string) (string, error) {
	output = strings.TrimSpace(output)
	if output == "" {
		return "", fmt.Errorf("empty pre-flight response")
	}

	start := strings.Index(output, "{")
	if start < 0 {
		return "", fmt.Errorf("no JSON object found in pre-flight response: %q", output)
	}

	depth := 0
	inString := false
	escape := false
	for i := start; i < len(output); i++ {
		c := output[i]
		if escape {
			escape = false
			continue
		}
		if inString {
			switch c {
			case '\\':
				escape = true
			case '"':
				inString = false
			}
			continue
		}
		switch c {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return output[start : i+1], nil
			}
		}
	}
	return "", fmt.Errorf("unbalanced JSON braces in pre-flight response: %q", output)
}

func parseSeverity(s string) (Severity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
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
