package llm

import (
	"context"
	"embed"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

//go:embed writingguide_templates/*.tmpl
var writingGuideTemplates embed.FS

// GuideSeverity weighs a writing-guide finding. Nothing gates on it: the
// levels steer the drafting agent's attention (substantive → take it up in
// dialogue, minor → fold in or ignore). Mirrored in the query package.
type GuideSeverity string

const (
	GuideSubstantive GuideSeverity = "substantive"
	GuideMinor       GuideSeverity = "minor"
)

// GuideAxis is the closed set of writing-guide check axes (d-cpt-7zr rendered
// for the guide; see writing_guide.tmpl). Closed on purpose: stable identifiers
// are what the calibration corpus banks against — pre-flight's free-form
// categories proved untrackable across runs.
var guideAxes = map[string]bool{
	"stranding":  true,
	"dilution":   true,
	"conflation": true,
	"pointing":   true,
	"form":       true,
}

// guideRepairs is the closed set of repair directions — direction only,
// never replacement text.
var guideRepairs = map[string]bool{
	"cut":      true,
	"write-in": true,
	"split":    true,
	"point":    true,
	"reword":   true,
}

// GuideFinding is a single writing-guide observation. Reasoning leads —
// it is the finding's work; axis, repair, and severity are its conclusions.
type GuideFinding struct {
	Reasoning string
	Axis      string
	Quote     string
	Repair    string
	Severity  GuideSeverity
}

// WritingGuideResult holds the parsed findings from a writing-guide run.
// An empty Findings slice means the draft passed clean.
type WritingGuideResult struct {
	Findings []GuideFinding
}

// WritingGuide runs the writing guide against a draft entry in isolation:
// the prompt carries the draft and the entry-craft instructions, and nothing
// else — no dialogue, no graph context. That absence is the instrument
// (d-cpt-20r): only a reader outside the dialogue can run the stands-alone
// test. Returns an error only for infrastructure failures.
func WritingGuide(ctx context.Context, runner Runner, entry *model.Entry, closureTargets []model.ClosureTarget) (*WritingGuideResult, error) {
	req, err := renderWritingGuidePrompt(entry, closureTargets)
	if err != nil {
		return nil, fmt.Errorf("rendering writing-guide prompt: %w", err)
	}

	output, err := Run(ctx, runner, req, "writing-guide")
	if err != nil {
		return nil, fmt.Errorf("running writing guide: %w", err)
	}

	result, err := parseWritingGuideResult(output.Text)
	if err != nil {
		return nil, fmt.Errorf("parsing writing-guide result: %w", err)
	}
	return result, nil
}

var (
	writingGuideTmplOnce sync.Once
	writingGuideTmpl     *template.Template
	writingGuideTmplErr  error
)

func parsedWritingGuideTemplates() (*template.Template, error) {
	writingGuideTmplOnce.Do(func() {
		writingGuideTmpl, writingGuideTmplErr = template.ParseFS(writingGuideTemplates, "writingguide_templates/*.tmpl")
	})
	return writingGuideTmpl, writingGuideTmplErr
}

// renderWritingGuidePrompt renders the two-part request: the system block is
// the byte-stable guide text (cacheable prefix across sequential captures,
// same invariant as pre-flight's universal preamble), the user block carries
// only the formatted draft.
func renderWritingGuidePrompt(entry *model.Entry, closureTargets []model.ClosureTarget) (Request, error) {
	tmpl, err := parsedWritingGuideTemplates()
	if err != nil {
		return Request{}, fmt.Errorf("parsing writing-guide templates: %w", err)
	}

	data := struct{ Draft string }{Draft: formatDraftForWritingGuide(entry, closureTargets)}

	var sysB, userB strings.Builder
	if err := tmpl.ExecuteTemplate(&sysB, "writing_guide_system", nil); err != nil {
		return Request{}, fmt.Errorf("executing writing_guide_system template: %w", err)
	}
	if err := tmpl.ExecuteTemplate(&userB, "writing_guide_user", data); err != nil {
		return Request{}, fmt.Errorf("executing writing_guide_user template: %w", err)
	}

	return Request{
		SystemPrompt: strings.TrimSpace(sysB.String()),
		UserPrompt:   strings.TrimSpace(userB.String()),
	}, nil
}

// formatDraftForWritingGuide renders the draft for the isolation-scoped
// prompt. Deliberately narrower than FormatEntryForPrompt: no ID, since drafts
// have none yet. Refs stay in (the pointing axis judges whether each ref's
// relationship reaches the body's narrative), and attachments appear as
// filename pointers only, so a body mentioning its attachment is not
// misjudged as a dangling reference.
//
// Closure edges carry one summary sentence per target, not the target's body.
// Without them the guide cannot tell which act the draft performs and read a
// correctly-pointing retirement done as stranded (s-tac-fu8); with the bodies
// it would be judging the neighborhood, which is the graph-fit guide's scope
// (d-cpt-20r).
func formatDraftForWritingGuide(e *model.Entry, closureTargets []model.ClosureTarget) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Type: %s\n", e.Type)
	fmt.Fprintf(&b, "Layer: %s\n", e.Layer)
	if e.Kind != "" {
		fmt.Fprintf(&b, "Kind: %s\n", e.Kind)
	}
	if e.Intent != "" {
		fmt.Fprintf(&b, "Intent: %s\n", e.Intent)
	}
	if len(e.Refs) > 0 {
		b.WriteString("Refs:\n")
		for _, r := range e.Refs {
			fmt.Fprintf(&b, "  - %s (kind: %s)", r.ID, r.Kind)
			if r.Desc != "" {
				fmt.Fprintf(&b, ": %s", r.Desc)
			}
			b.WriteByte('\n')
		}
	}
	if len(closureTargets) > 0 {
		b.WriteString("Closure edges:\n")
		for _, t := range closureTargets {
			fmt.Fprintf(&b, "  - %s %s", t.Relation, t.ID)
			if t.Kind != "" {
				fmt.Fprintf(&b, " (%s %s)", t.Kind, t.Type)
			}
			b.WriteByte('\n')
			if t.Summary != "" {
				fmt.Fprintf(&b, "    %s\n", t.Summary)
			}
		}
	}
	if len(e.Attachments) > 0 {
		fmt.Fprintf(&b, "Attachments: %s\n", strings.Join(e.Attachments, ", "))
	}
	fmt.Fprintf(&b, "\n%s", e.Content)
	return b.String()
}

// parseWritingGuideResult parses the guide's JSON response. The LLM is asked
// to respond with:
//
//	{"findings": [{"reasoning": "...", "axis": "...", "quote": "...", "repair": "...", "severity": "substantive|minor"}]}
//
// Empty findings array means the draft passed clean. Tolerates prose around
// the JSON object (shared extractJSONObject); malformed JSON, values outside
// the closed axis/repair/severity sets, and empty required fields are errors,
// so infrastructure failures stay distinct from findings.
func parseWritingGuideResult(output string) (*WritingGuideResult, error) {
	jsonText, err := extractJSONObject(output)
	if err != nil {
		return nil, err
	}

	var resp struct {
		Findings []struct {
			Reasoning string `json:"reasoning"`
			Axis      string `json:"axis"`
			Quote     string `json:"quote"`
			Repair    string `json:"repair"`
			Severity  string `json:"severity"`
		} `json:"findings"`
	}
	if err := json.Unmarshal([]byte(jsonText), &resp); err != nil {
		return nil, fmt.Errorf("parsing writing-guide JSON: %w", err)
	}

	findings := make([]GuideFinding, 0, len(resp.Findings))
	for i, f := range resp.Findings {
		axis := strings.ToLower(strings.TrimSpace(f.Axis))
		if !guideAxes[axis] {
			return nil, fmt.Errorf("finding %d: unknown axis %q", i, f.Axis)
		}
		repair := strings.ToLower(strings.TrimSpace(f.Repair))
		if !guideRepairs[repair] {
			return nil, fmt.Errorf("finding %d: unknown repair %q", i, f.Repair)
		}
		severity, err := parseGuideSeverity(f.Severity)
		if err != nil {
			return nil, fmt.Errorf("finding %d: %w", i, err)
		}
		if strings.TrimSpace(f.Reasoning) == "" {
			return nil, fmt.Errorf("finding %d: reasoning is empty", i)
		}
		if strings.TrimSpace(f.Quote) == "" {
			return nil, fmt.Errorf("finding %d: quote is empty", i)
		}
		findings = append(findings, GuideFinding{
			Reasoning: f.Reasoning,
			Axis:      axis,
			Quote:     f.Quote,
			Repair:    repair,
			Severity:  severity,
		})
	}

	return &WritingGuideResult{Findings: findings}, nil
}

func parseGuideSeverity(s string) (GuideSeverity, error) {
	switch strings.ToLower(strings.TrimSpace(s)) {
	case "substantive":
		return GuideSubstantive, nil
	case "minor":
		return GuideMinor, nil
	default:
		return "", fmt.Errorf("unknown severity %q", s)
	}
}
