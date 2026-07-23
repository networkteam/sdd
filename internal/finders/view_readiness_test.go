package finders

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// Executor-level checks for the readiness layout macro (d-tac-e55): the four
// capped lanes run against a real graph, participants:brief composes with the
// participants block render, and the guiding-directive lanes split by layer.

func readinessGraph() *model.Graph {
	actor := actorEntry("Christopher", nil)
	strategic := entry("20260501-100000-d-stg-str",
		withKind(model.KindDirective), withIntent(model.IntentGuiding), withContent("go host-neutral"))
	conceptual := entry("20260501-110000-d-cpt-cpt",
		withKind(model.KindDirective), withIntent(model.IntentGuiding), withContent("CQRS layering"))
	aspiration := entry("20260501-120000-d-stg-asp",
		withKind(model.KindAspiration), withContent("a shared, searchable reasoning record"))
	return model.NewGraph([]*model.Entry{actor, strategic, conceptual, aspiration})
}

func TestView_Readiness_FourLanesParticipantsBriefAndLayerSplit(t *testing.T) {
	g := readinessGraph()
	f := New(Options{})
	result, err := f.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayoutAndExpand(t, "readiness")})
	if err != nil {
		t.Fatalf("View(readiness): %v", err)
	}
	if len(result.Sections) != 4 {
		t.Fatalf("readiness sections: got %d, want 4", len(result.Sections))
	}

	// Lane 1: participants:brief composes — as-participants-block render with
	// Brief set (the open question from readiness-layout §Notes).
	lane1 := result.Sections[0]
	if lane1.Render != "as-participants-block" {
		t.Fatalf("lane 1 render = %q, want as-participants-block", lane1.Render)
	}
	if !lane1.Brief {
		t.Errorf("participants:brief should set Brief on the section")
	}
	block, ok := lane1.Data.(model.ParticipantsBlock)
	if !ok || len(block.Groups) != 1 || block.Groups[0].Actor.Canonical != "Christopher" {
		t.Fatalf("lane 1 should render the Christopher actor, got %+v", lane1.Data)
	}

	// Lane 3 (strategic guiding) holds the strategic directive alone; lane 4
	// (conceptual guiding) holds the conceptual one — the layer()/intent()
	// split composes (readiness-layout §Notes).
	strategicIDs := laneIDs(result.Sections[2])
	if !containsID(strategicIDs, "20260501-100000-d-stg-str") || containsID(strategicIDs, "20260501-110000-d-cpt-cpt") {
		t.Errorf("strategic lane = %v, want only the strategic guiding directive", strategicIDs)
	}
	conceptualIDs := laneIDs(result.Sections[3])
	if !containsID(conceptualIDs, "20260501-110000-d-cpt-cpt") || containsID(conceptualIDs, "20260501-100000-d-stg-str") {
		t.Errorf("conceptual lane = %v, want only the conceptual guiding directive", conceptualIDs)
	}
}

func TestView_Readiness_NotOfferedAtSectionStart(t *testing.T) {
	// readiness is a layout macro, invalid at section start. Using it there
	// (readiness:brief is two functions, so the layout-macro path never fires)
	// must error, and the section-start hint must not advertise readiness as a
	// valid section macro — the self-contradicting hint the split fixes.
	g := readinessGraph()
	f := New(Options{})
	_, err := f.OnGraph(g).View(query.ViewQuery{Layout: mustParseLayoutAndExpand(t, "readiness:brief")})
	if err == nil {
		t.Fatal("readiness at section start should error")
	}
	_, hint, ok := strings.Cut(err.Error(), "section start:")
	if !ok {
		t.Fatalf("error should carry a section-start macro hint, got %q", err)
	}
	if strings.Contains(hint, "readiness") {
		t.Errorf("section-start macro hint must not list readiness, got %q", hint)
	}
}

func laneIDs(s query.SectionResult) []string {
	flat, ok := s.Data.(model.FlatList)
	if !ok {
		return nil
	}
	ids := make([]string, 0, len(flat.Entries))
	for _, e := range flat.Entries {
		ids = append(ids, e.ID)
	}
	return ids
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
