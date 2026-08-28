package llm

import (
	"fmt"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestParseWritingGuideResult_ValidWithSurroundingProse(t *testing.T) {
	out := "Here is my review:\n```json\n{\"findings\": [{\"reasoning\": \"the body leans on 'the earlier approach' without pointing at it, so the pull fails\", \"axis\": \"Stranding\", \"quote\": \"the earlier approach\", \"repair\": \"write-in\", \"severity\": \"substantive\"}]}\n```\nDone."
	result, err := parseWritingGuideResult(out)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 1 {
		t.Fatalf("findings = %d, want 1", len(result.Findings))
	}
	f := result.Findings[0]
	if f.Axis != "stranding" {
		t.Errorf("axis = %q, want normalized stranding", f.Axis)
	}
	if f.Repair != "write-in" || f.Severity != GuideSubstantive {
		t.Errorf("finding = %+v", f)
	}
}

func TestParseWritingGuideResult_EmptyFindings(t *testing.T) {
	result, err := parseWritingGuideResult(`{"findings": []}`)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Findings) != 0 {
		t.Fatalf("findings = %+v, want none", result.Findings)
	}
}

func TestParseWritingGuideResult_Errors(t *testing.T) {
	cases := []struct {
		name                                     string
		reasoning, axis, quote, repair, severity string
		wantErr                                  string
	}{
		{"unknown axis", "r", "novelty", "q", "cut", "minor", "unknown axis"},
		{"unknown repair", "r", "dilution", "q", "delete", "minor", "unknown repair"},
		{"unknown severity", "r", "dilution", "q", "cut", "high", "unknown severity"},
		{"empty reasoning", " ", "dilution", "q", "cut", "minor", "reasoning is empty"},
		{"empty quote", "r", "dilution", " ", "cut", "minor", "quote is empty"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			payload := `{"findings": [{"reasoning": "` + tc.reasoning + `", "axis": "` + tc.axis +
				`", "quote": "` + tc.quote + `", "repair": "` + tc.repair + `", "severity": "` + tc.severity + `"}]}`
			_, err := parseWritingGuideResult(payload)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("err = %v, want containing %q", err, tc.wantErr)
			}
		})
	}
}

// fakeFactSource serves canned fact bodies; a missing ID errors like the real
// graph-backed source.
type fakeFactSource map[string]string

func (s fakeFactSource) FactBody(id string) (string, error) {
	body, ok := s[id]
	if !ok {
		return "", fmt.Errorf("reference fact %s does not resolve", id)
	}
	return body, nil
}

func testReferenceFacts(kindFactID string) ReferenceFacts {
	return ReferenceFacts{
		Source: fakeFactSource{
			"20260812-180000-s-prc-typ": "# The type system — test stub\n\nTwo types, fifteen kinds.\n\n## Kinds\n\nThe kind list.",
			"20260812-170000-s-prc-dnk": "# Recording completed work — test stub\n\nOne act, one done.",
		},
		TypeSystemFactID: "20260812-180000-s-prc-typ",
		KindFactID:       kindFactID,
	}
}

func TestRenderWritingGuidePrompt_SystemByteStableAcrossDrafts(t *testing.T) {
	a := &model.Entry{Type: model.TypeSignal, Kind: model.KindGap, Layer: model.LayerTactical, Content: "First draft body."}
	b := &model.Entry{Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerProcess, Intent: model.IntentGuiding, Content: "A different body entirely.",
		Refs: []model.Ref{{ID: "20260601-120000-d-tac-ref", Kind: model.RefKind("refines"), Desc: "target"}}}

	reqA, err := renderWritingGuidePrompt(a, nil, testReferenceFacts(""))
	if err != nil {
		t.Fatal(err)
	}
	reqB, err := renderWritingGuidePrompt(b, []model.ClosureTarget{
		{Relation: "closes", ID: "20260601-120000-d-tac-old", Type: model.TypeDecision, Kind: model.KindDirective, Summary: "The commitment this draft retires."},
	}, testReferenceFacts(""))
	if err != nil {
		t.Fatal(err)
	}
	if reqA.SystemPrompt != reqB.SystemPrompt {
		t.Error("system prompt must be byte-stable across drafts (prompt-cache invariant)")
	}
	for _, want := range []string{"stranding", "dilution", "conflation", "pointing", "form", "No findings is a correct outcome"} {
		if !strings.Contains(reqA.SystemPrompt, want) {
			t.Errorf("system prompt missing %q", want)
		}
	}
	if strings.Contains(reqA.SystemPrompt, "First draft body.") {
		t.Error("draft content leaked into the system prompt")
	}
	if !strings.Contains(reqB.UserPrompt, "A different body entirely.") {
		t.Errorf("user prompt missing the draft body:\n%s", reqB.UserPrompt)
	}
	for _, want := range []string{"Kind: directive", "Intent: guiding", "20260601-120000-d-tac-ref (kind: refines): target"} {
		if !strings.Contains(reqB.UserPrompt, want) {
			t.Errorf("user prompt missing %q:\n%s", want, reqB.UserPrompt)
		}
	}
	// The closure edge and its one summary sentence reach the draft: without
	// them the guide cannot tell which act the entry performs (s-tac-fu8).
	for _, want := range []string{"closes 20260601-120000-d-tac-old (directive decision)", "The commitment this draft retires."} {
		if !strings.Contains(reqB.UserPrompt, want) {
			t.Errorf("user prompt missing closure edge %q:\n%s", want, reqB.UserPrompt)
		}
	}
	if strings.Contains(reqA.UserPrompt, "Closure edges") {
		t.Error("a draft with no closure edges must not carry the section")
	}
}

func TestRenderWritingGuidePrompt_JSONStringRulesShared(t *testing.T) {
	// Regression for 20260827-224853-s-tac-giv: the output format teaches
	// standard JSON escaping, never quote avoidance.
	e := &model.Entry{Type: model.TypeSignal, Kind: model.KindGap, Layer: model.LayerTactical, Content: "Body."}
	req, err := renderWritingGuidePrompt(e, nil, testReferenceFacts(""))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(req.SystemPrompt, "**JSON strings.**") || !strings.Contains(req.SystemPrompt, "write a literal `\"` as `\\\"`") {
		t.Errorf("system prompt missing the shared JSON escaping rule:\n%s", req.SystemPrompt)
	}
	if strings.Contains(req.SystemPrompt, "never a literal") {
		t.Error("system prompt still teaches quote avoidance")
	}
}

// TestRenderWritingGuidePrompt_ReferenceFacts pins the framework-knowledge
// slots: the type-system overview renders into the system block with its
// headings demoted under the section, and the drafted kind's authoring fact
// renders into the user block before the draft — absent when the kind has no
// fact, loud when a configured fact does not resolve.
func TestRenderWritingGuidePrompt_ReferenceFacts(t *testing.T) {
	done := &model.Entry{Type: model.TypeSignal, Kind: model.KindDone, Layer: model.LayerTactical, Content: "Draft done body."}

	req, err := renderWritingGuidePrompt(done, nil, testReferenceFacts("20260812-170000-s-prc-dnk"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(req.SystemPrompt, "## What the framework's entries mean") {
		t.Error("system prompt missing the reference-knowledge section")
	}
	if !strings.Contains(req.SystemPrompt, "### The type system — test stub") {
		t.Errorf("system prompt missing the overview body with shifted h1:\n%s", req.SystemPrompt)
	}
	if !strings.Contains(req.SystemPrompt, "#### Kinds") {
		t.Error("overview h2 not demoted to h4")
	}
	if !strings.Contains(req.UserPrompt, "## What a good done entry is") {
		t.Errorf("user prompt missing the kind-fact section:\n%s", req.UserPrompt)
	}
	if !strings.Contains(req.UserPrompt, "### Recording completed work — test stub") {
		t.Error("user prompt missing the kind fact body with shifted h1")
	}
	if kindIdx, draftIdx := strings.Index(req.UserPrompt, "What a good done entry is"), strings.Index(req.UserPrompt, "## Draft entry"); kindIdx > draftIdx {
		t.Error("kind fact must precede the draft")
	}

	gap := &model.Entry{Type: model.TypeSignal, Kind: model.KindGap, Layer: model.LayerTactical, Content: "Draft gap body."}
	reqGap, err := renderWritingGuidePrompt(gap, nil, testReferenceFacts(""))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(reqGap.UserPrompt, "What a good") {
		t.Error("a kind without an authoring fact must not carry the kind-fact section")
	}

	if _, err := renderWritingGuidePrompt(done, nil, testReferenceFacts("20260101-000000-s-prc-mis")); err == nil {
		t.Error("a configured kind fact that does not resolve must fail loud")
	}
	if _, err := renderWritingGuidePrompt(done, nil, ReferenceFacts{}); err == nil {
		t.Error("a missing fact source must fail loud")
	}
}
