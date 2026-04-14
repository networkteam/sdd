package presenters_test

import (
	"bytes"
	"testing"

	"github.com/bradleyjkemp/cupaloy/v2"

	"github.com/networkteam/resonance/framework/sdd/finders"
	"github.com/networkteam/resonance/framework/sdd/model"
	"github.com/networkteam/resonance/framework/sdd/presenters"
	"github.com/networkteam/resonance/framework/sdd/query"
)

// renderShow exercises the finder + presenter together.
func renderShow(t *testing.T, g *model.Graph, ids []string, maxDepth int) string {
	t.Helper()
	f := finders.New(nil)
	result, err := f.Show(query.ShowQuery{Graph: g, IDs: ids, MaxDepth: maxDepth})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	presenters.RenderShow(&buf, result)
	return buf.String()
}

func TestRenderShow_SingleEntryNoRefs(t *testing.T) {
	e := entry("20260410-100000-s-tac-aaa", withContent("A signal about something"))
	g := model.NewGraph([]*model.Entry{e})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{e.ID}, 0))
}

func TestRenderShow_UpstreamChain(t *testing.T) {
	root := entry("20260410-100000-s-stg-aaa", withSummary("Root signal about the foundation"))
	mid := entry("20260410-100100-s-cpt-bbb", withSummary("Middle observation building on root"),
		withRefs("20260410-100000-s-stg-aaa"))
	primary := entry("20260410-100200-d-tac-ccc", withContent("Decision based on observations"),
		withRefs("20260410-100100-s-cpt-bbb"))

	g := model.NewGraph([]*model.Entry{root, mid, primary})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{primary.ID}, 0))
}

func TestRenderShow_DownstreamWithRelations(t *testing.T) {
	target := entry("20260410-100000-s-stg-aaa", withContent("Target signal"))
	refBy := entry("20260410-100100-d-cpt-bbb", withSummary("Decision referencing target"),
		withRefs("20260410-100000-s-stg-aaa"))
	closedBy := entry("20260410-100200-a-tac-ccc", withSummary("Action closing target"),
		withCloses("20260410-100000-s-stg-aaa"))

	g := model.NewGraph([]*model.Entry{target, refBy, closedBy})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{target.ID}, 0))
}

func TestRenderShow_MultiPrimaryDedup(t *testing.T) {
	shared := entry("20260410-100000-s-stg-aaa", withSummary("Shared ref"))
	first := entry("20260410-100100-d-cpt-bbb", withContent("First primary"),
		withRefs("20260410-100000-s-stg-aaa"))
	second := entry("20260410-100200-d-cpt-ccc", withContent("Second primary"),
		withRefs("20260410-100000-s-stg-aaa"))

	g := model.NewGraph([]*model.Entry{shared, first, second})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{first.ID, second.ID}, 0))
}

func TestRenderShow_BranchingWithDedup(t *testing.T) {
	shared := entry("20260410-100000-s-stg-ddd", withSummary("Shared deep ref"))
	b := entry("20260410-100100-s-cpt-bbb", withSummary("Branch B"), withRefs("20260410-100000-s-stg-ddd"))
	c := entry("20260410-100200-s-cpt-ccc", withSummary("Branch C"), withRefs("20260410-100000-s-stg-ddd"))
	primary := entry("20260410-100300-d-tac-aaa", withContent("Primary with two branches"),
		withRefs("20260410-100100-s-cpt-bbb", "20260410-100200-s-cpt-ccc"))

	g := model.NewGraph([]*model.Entry{shared, b, c, primary})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{primary.ID}, 0))
}

func TestRenderShow_CombinedRelationsAndKind(t *testing.T) {
	signal := entry("20260410-100000-s-tac-aaa", withSummary("The signal"))
	contract := entry("20260410-100050-d-tac-ddd", withKind(model.KindContract), withSummary("A contract"))
	plan := entry("20260410-100055-d-tac-eee", withKind(model.KindPlan), withSummary("A plan"))
	action := entry("20260410-100100-a-tac-bbb", withContent("Action with combined relations"),
		withRefs("20260410-100000-s-tac-aaa", "20260410-100050-d-tac-ddd", "20260410-100055-d-tac-eee"),
		withCloses("20260410-100000-s-tac-aaa"))

	g := model.NewGraph([]*model.Entry{signal, contract, plan, action})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{action.ID}, 0))
}

func TestRenderShow_MaxDepthTruncation(t *testing.T) {
	e0 := entry("20260410-100000-s-stg-aaa", withSummary("Root"))
	e1 := entry("20260410-100100-s-cpt-bbb", withSummary("Level 1"), withRefs("20260410-100000-s-stg-aaa"))
	e2 := entry("20260410-100200-s-tac-ccc", withSummary("Level 2"), withRefs("20260410-100100-s-cpt-bbb"))
	primary := entry("20260410-100300-d-tac-ddd", withContent("Primary"),
		withRefs("20260410-100200-s-tac-ccc"))

	g := model.NewGraph([]*model.Entry{e0, e1, e2, primary})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{primary.ID}, 2))
}

func TestRenderShow_FallbackFirstSentence(t *testing.T) {
	a := entry("20260410-100000-s-stg-aaa", withContent("First sentence of content.\nSecond line."))
	b := entry("20260410-100100-d-tac-bbb", withContent("Primary"), withRefs("20260410-100000-s-stg-aaa"))

	g := model.NewGraph([]*model.Entry{a, b})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{b.ID}, 0))
}

func TestRenderShow_EntryNotFound(t *testing.T) {
	g := model.NewGraph([]*model.Entry{})

	f := finders.New(nil)
	_, err := f.Show(query.ShowQuery{Graph: g, IDs: []string{"20260410-100000-s-stg-xxx"}})
	if err == nil {
		t.Fatal("expected error for missing entry")
	}
}

func TestWriteEntryFull_KindDisplayed(t *testing.T) {
	tests := []struct {
		name     string
		kind     model.Kind
		wantKind string
	}{
		{"plan", model.KindPlan, "Kind:   plan"},
		{"contract", model.KindContract, "Kind:   contract"},
		{"directive_omitted", model.KindDirective, ""},
		{"empty_omitted", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := entry("20260410-100000-d-tac-aaa", withKind(tt.kind), withContent("Test"))
			var buf bytes.Buffer
			presenters.WriteEntryFull(&buf, e)
			cupaloy.SnapshotT(t, buf.String())
		})
	}
}
