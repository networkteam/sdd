package presenters_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
)

// TestRenderStatusVocabulary verifies that aspirations render in their own
// section distinct from contracts, and that activity-kind decisions surface
// inside Active Decisions (with "activity" visible in the line).
func TestRenderStatusVocabulary(t *testing.T) {
	g := model.NewGraph([]*model.Entry{
		entry("20260406-100000-d-cpt-ctr", withKind(model.KindContract), withContent("A contract.")),
		entry("20260406-100100-d-stg-asp", withKind(model.KindAspiration), withContent("An aspiration.")),
		entry("20260406-100200-d-tac-dir", withKind(model.KindDirective), withContent("A directive.")),
		entry("20260406-100300-d-tac-act", withKind(model.KindActivity), withContent("An activity.")),
	})

	result := &query.StatusResult{
		Graph:       g,
		Contracts:   g.Contracts(),
		Aspirations: g.Aspirations(),
		Plans:       g.Plans(),
		Active:      g.ActiveDecisions(),
	}

	var buf bytes.Buffer
	presenters.RenderStatus(&buf, result)
	out := buf.String()

	// Section headers must appear and aspirations must have their own heading.
	if !strings.Contains(out, "## Contracts") {
		t.Errorf("missing Contracts section:\n%s", out)
	}
	if !strings.Contains(out, "## Aspirations") {
		t.Errorf("missing Aspirations section:\n%s", out)
	}
	if !strings.Contains(out, "## Active Decisions") {
		t.Errorf("missing Active Decisions section:\n%s", out)
	}

	contractsIdx := strings.Index(out, "## Contracts")
	aspirationsIdx := strings.Index(out, "## Aspirations")
	activeIdx := strings.Index(out, "## Active Decisions")
	if contractsIdx >= aspirationsIdx || aspirationsIdx >= activeIdx {
		t.Errorf("expected order Contracts → Aspirations → Active Decisions; got indices %d, %d, %d",
			contractsIdx, aspirationsIdx, activeIdx)
	}

	// Aspirations section must contain the aspiration entry and NOT the contract.
	aspirationsBlock := out[aspirationsIdx:activeIdx]
	if !strings.Contains(aspirationsBlock, "20260406-100100-d-stg-asp") {
		t.Errorf("aspirations section missing aspiration entry:\n%s", aspirationsBlock)
	}
	if strings.Contains(aspirationsBlock, "20260406-100000-d-cpt-ctr") {
		t.Errorf("aspirations section leaked a contract entry:\n%s", aspirationsBlock)
	}

	// Active section must show both directive and activity, and the activity
	// kind must be visible on its line (distinguishing it from the directive).
	activeBlock := out[activeIdx:]
	if !strings.Contains(activeBlock, "20260406-100200-d-tac-dir") {
		t.Errorf("active section missing directive:\n%s", activeBlock)
	}
	if !strings.Contains(activeBlock, "20260406-100300-d-tac-act") {
		t.Errorf("active section missing activity:\n%s", activeBlock)
	}
	if !strings.Contains(activeBlock, "activity decision") {
		t.Errorf("activity line missing 'activity decision' qualifier:\n%s", activeBlock)
	}
	if !strings.Contains(activeBlock, "directive decision") {
		t.Errorf("directive line missing 'directive decision' qualifier:\n%s", activeBlock)
	}
}
