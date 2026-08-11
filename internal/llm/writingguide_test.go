package llm

import (
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

func TestRenderWritingGuidePrompt_SystemByteStableAcrossDrafts(t *testing.T) {
	a := &model.Entry{Type: model.TypeSignal, Kind: model.KindGap, Layer: model.LayerTactical, Content: "First draft body."}
	b := &model.Entry{Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerProcess, Intent: model.IntentGuiding, Content: "A different body entirely.",
		Refs: []model.Ref{{ID: "20260601-120000-d-tac-ref", Kind: model.RefKind("refines"), Desc: "target"}}}

	reqA, err := renderWritingGuidePrompt(a, nil)
	if err != nil {
		t.Fatal(err)
	}
	reqB, err := renderWritingGuidePrompt(b, []model.ClosureTarget{
		{Relation: "closes", ID: "20260601-120000-d-tac-old", Type: model.TypeDecision, Kind: model.KindDirective, Summary: "The commitment this draft retires."},
	})
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
