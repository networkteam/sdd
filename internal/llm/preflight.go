package llm

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"sync"
	"text/template"
	"time"

	"github.com/networkteam/sdd/internal/bundledskills"
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
//
// Participant validation is handled by mechanical checks in the finders
// layer (see finders.mechanicalPreflight) — the LLM participant-drift
// rubric is retired per plan d-cpt-d34 AC 9.
//
// configuredLanguage is the graph authoring language (locale code) from
// `.sdd/config.yaml` (empty string when unset — English default). It feeds
// the language-drift check which flags entries whose description prose does
// not match the configured language.
func Preflight(ctx context.Context, runner Runner, entry *model.Entry, graph *model.Graph, configuredLanguage string) (*PreflightResult, error) {
	ct := selectCheckType(entry, graph)

	pctx := assembleContext(entry, graph, ct, configuredLanguage)

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
	checkActorCapture                       // kind: actor signal capture
	checkRoleCapture                        // kind: role decision capture
	checkAnnotationCapture                  // kind: annotation signal capture
	checkFocusCapture                       // kind: focus decision capture
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
	case checkActorCapture:
		return "actor-capture"
	case checkRoleCapture:
		return "role-capture"
	case checkAnnotationCapture:
		return "annotation-capture"
	case checkFocusCapture:
		return "focus-capture"
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
	checkActorCapture:      "actor_capture",
	checkRoleCapture:       "role_capture",
	checkAnnotationCapture: "annotation_capture",
	checkFocusCapture:      "focus_capture",
}

// preflightContext holds all data needed to render a pre-flight prompt template.
// Acceptance criteria live inline in plan decision descriptions (as a
// `## Acceptance criteria` markdown section), so they flow through
// ProposedEntry and ClosedEntries without extra fields.
//
// ConfiguredLanguage feeds the language-drift rubric (see language.tmpl).
// Empty means no language check (English default); a locale code like "de"
// activates the check against the proposed entry's description prose.
type preflightContext struct {
	ProposedEntry      string
	ReferencedEntries  string
	ClosedEntries      string
	SupersededEntries  string
	ActiveContracts    string
	ActiveAspirations  string
	ConfiguredLanguage string
	// RefKindVocabulary is the canonical ref-kind definitions, read from the
	// bundled skill reference (references/ref-kinds.md) — the same single source
	// the skill ships. The ref-meta consistency rubric renders it instead of
	// restating the kinds, so the vocabulary is defined once.
	RefKindVocabulary string
}

// selectCheckType determines the pre-flight check type from entry properties and graph context.
// Dispatch is kind-aware: the same structural shape (signal with closes) routes to different
// templates based on the entry's kind and the closed target's kind.
func selectCheckType(entry *model.Entry, graph *model.Graph) checkType {
	// Identity / structural kinds take precedence — their rubrics focus on
	// frontmatter shape and per-kind prose conventions. Supersessions of
	// these kinds still route here rather than to the generic supersedes
	// template so the templates see kind-specific guidance.
	if entry.IsActor() {
		return checkActorCapture
	}
	if entry.IsRole() {
		return checkRoleCapture
	}
	if entry.IsAnnotation() {
		return checkAnnotationCapture
	}
	if entry.IsFocus() {
		return checkFocusCapture
	}

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
//
// configuredLanguage flows through unchanged.
func assembleContext(entry *model.Entry, graph *model.Graph, ct checkType, configuredLanguage string) *preflightContext {
	pctx := &preflightContext{
		ProposedEntry:      formatProposedEntryForPreflight(entry),
		ConfiguredLanguage: configuredLanguage,
		RefKindVocabulary:  refKindVocabulary(),
	}

	// Referenced entries. Each carries its derived status (active / closed /
	// superseded) so the ref-meta check can distinguish grounded-in (basis),
	// builds-on (closed or next-step), and refines (active, in-place) — those
	// three split on the target's status, which the entry text alone can't
	// reveal. Status is appended here (pre-flight only) rather than in the
	// shared FormatEntryForPrompt, so summary-prompt hashes stay stable.
	//
	// When a ref points at a superseded entry, the literal target is kept as
	// the referenced entry — its superseded status is what the ref-meta check
	// reasons against, so swapping in the active head would flip builds-on /
	// refines judgments. The live head's content is appended alongside (labeled)
	// so the validator can still reason about the current entity rather than
	// being stranded at a retired intermediate. A stale ref is expected under
	// concurrent supersession (the head an author reffed may already be replaced
	// by the time the entry lands), so this is context, not a finding.
	if len(entry.Refs) > 0 {
		var parts []string
		for _, ref := range entry.Refs {
			e, ok := graph.ByID[ref.ID]
			if !ok {
				continue
			}
			parts = append(parts, formatReferencedEntry(graph, e)+refApplicabilityLines(graph, ref, e))
			if rr := graph.ResolveRef(ref.ID); rr.IsStale() {
				if head, ok := graph.ByID[rr.Head()]; ok {
					parts = append(parts, "(live head of "+ref.ID+")\n"+formatReferencedEntry(graph, head))
				}
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

	// Active contracts (always included). Sorted by full ID so the rendered
	// block is byte-identical across captures whenever the contract set is
	// unchanged. Contracts live in the cached universal system preamble (d-tac-fah),
	// so a non-deterministic order would silently break prompt-cache reuse even
	// when nothing about the contract set changed — this sort is the byte-stability
	// invariant, not a cosmetic ordering.
	contracts := graph.Contracts()
	if len(contracts) > 0 {
		sort.Slice(contracts, func(i, j int) bool { return contracts[i].ID < contracts[j].ID })
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

	return pctx
}

// formatProposedEntryForPreflight renders the proposed entry plus its stored
// intent. Intent is appended here — pre-flight only — rather than in the
// shared FormatEntryForPrompt, so it stays out of the summary-generation
// prompt, which has no use for it. The settled-justification rubric reads this
// line to decide whether its guard matches.
func formatProposedEntryForPreflight(e *model.Entry) string {
	s := FormatEntryForPrompt(e)
	if e.Intent != "" {
		s += "\nIntent: " + string(e.Intent)
	}
	return s
}

// formatReferencedEntry renders a ref target for the pre-flight prompt: the
// standard entry rendering plus a `Derived status:` line. The status lets the
// ref-meta consistency check tell grounded-in / builds-on / refines apart —
// kinds that split on whether the target is a basis, a closed prior step, or an
// active commitment refined in place. This is pre-flight only; the summary
// prompt uses FormatEntryForPrompt unchanged, so summary hashes don't drift.
func formatReferencedEntry(graph *model.Graph, e *model.Entry) string {
	return FormatEntryForPrompt(e) + "\nDerived status: " + derivedStatusForPrompt(graph.DerivedStatus(e))
}

// refApplicabilityLines renders the matrix verdict for one ref beneath its
// target's entry block via the ref_applicability template (plan d-tac-tph
// AC 5): the admissible kinds for the target class and the chosen kind's
// cell. Go prepares the data; the prose lives in the template alongside the
// rest of the prompt surface. Kinds outside the capturable set render
// nothing (the mechanical check rejects them), and a template failure
// degrades to no lines rather than blocking capture — the templates are
// embedded, so neither is expected.
func refApplicabilityLines(graph *model.Graph, ref model.Ref, target *model.Entry) string {
	class := model.ClassifyRefTarget(target, graph.DerivedStatus(target))
	cell, ok := model.RefKindApplicability(ref.Kind, class)
	if !ok {
		return ""
	}
	kinds := model.AdmissibleRefKinds(class)
	names := make([]string, len(kinds))
	for i, k := range kinds {
		names[i] = string(k)
	}
	tmpl, err := parsedPreflightTemplates()
	if err != nil {
		return ""
	}
	var sb strings.Builder
	err = tmpl.ExecuteTemplate(&sb, "ref_applicability", struct {
		Class      model.RefTargetClass
		Admissible string
		Chosen     model.RefKind
		Applicable bool
		Note       string
	}{
		Class:      class,
		Admissible: strings.Join(names, ", "),
		Chosen:     ref.Kind,
		Applicable: cell.Applicable,
		Note:       cell.Note,
	})
	if err != nil {
		return ""
	}
	return "\n" + strings.TrimSpace(sb.String())
}

// derivedStatusForPrompt renders a Status as plain prose for the validator
// prompt (not the `{status: ...}` display notation, which is a view concern).
func derivedStatusForPrompt(s model.Status) string {
	switch s.Kind {
	case model.StatusActive:
		return "active"
	case model.StatusOpen:
		return "open"
	case model.StatusClosedBy:
		return "closed by " + s.By
	case model.StatusSupersededBy:
		return "superseded by " + s.By
	case model.StatusCascadeClosedBy:
		return "role retired (bound actor chain closed by " + s.By + ")"
	case model.StatusCascadeOrphan:
		return "orphan role (no matching actor chain)"
	case model.StatusNone:
		return "terminal (done signal — no lifecycle status)"
	default:
		return string(s.Kind)
	}
}

var (
	refKindVocabOnce sync.Once
	refKindVocabText string

	preflightTmplOnce sync.Once
	preflightTmpl     *template.Template
	preflightTmplErr  error
)

// parsedPreflightTemplates parses the embedded template set once and reuses
// it across renders — all partials (including ref_applicability, executed
// per ref during context assembly) resolve from the same set.
func parsedPreflightTemplates() (*template.Template, error) {
	preflightTmplOnce.Do(func() {
		preflightTmpl, preflightTmplErr = template.ParseFS(preflightTemplates, "preflight_templates/*.tmpl")
	})
	return preflightTmpl, preflightTmplErr
}

// refKindVocabulary returns the canonical ref-kind vocabulary from the bundled
// skill reference (references/ref-kinds.md), cached after first read. This is
// the single source the skill also ships, so the rubric never restates the
// kinds. The fragment is embedded in the binary, so a read failure is not
// expected; on error the rubric renders without the injected definitions
// (degraded, non-fatal) rather than blocking capture.
func refKindVocabulary() string {
	refKindVocabOnce.Do(func() {
		if data, err := bundledskills.ReadReference("sdd", "references/ref-kinds.md"); err == nil {
			refKindVocabText = strings.TrimSpace(string(data))
		}
	})
	return refKindVocabText
}

// renderPreflightPrompt renders the pre-flight prompt for the given check type
// and context as a two-part Request. The system block is a byte-stable,
// type-independent universal preamble (validator role, universal partials,
// ref-kind vocabulary, active contracts, output format) shared by every
// substantive check, so sequential captures within a session reuse Anthropic's
// prompt cache (d-tac-fah). The per-type task, rubric, and calibration — plus the
// proposed entry and its related entries — live in the _user block. The two
// structural checks (annotation, focus) keep a lighter bespoke system block:
// they are rare, carry intentionally light rubrics, and forcing the heavy
// universal partials on them risks false positives (e.g. unrelated_refs flagging
// an annotation's membership refs). All templates are parsed together so partials
// are available.
func renderPreflightPrompt(ct checkType, pctx *preflightContext) (Request, error) {
	base, ok := checkTypeTemplates[ct]
	if !ok {
		return Request{}, fmt.Errorf("no template for check type %s", ct)
	}

	tmpl, err := parsedPreflightTemplates()
	if err != nil {
		return Request{}, fmt.Errorf("parsing templates: %w", err)
	}

	// Substantive checks share the universal preamble; the structural checks
	// keep their own light system block.
	systemTmpl := "universal_system"
	if ct == checkAnnotationCapture || ct == checkFocusCapture {
		systemTmpl = base + "_system"
	}

	var sysB, userB strings.Builder
	if err := tmpl.ExecuteTemplate(&sysB, systemTmpl, pctx); err != nil {
		return Request{}, fmt.Errorf("executing template %s: %w", systemTmpl, err)
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
