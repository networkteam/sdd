package presenters_test

import (
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// Slice 8 covers two new render shapes (as-wip-list, as-participants-
// block) and the section-header path that auto-derive populates. Tests
// stay in a separate file so view_test.go's slice 1-7 surface remains
// focused.

func TestRenderView_AsWipList(t *testing.T) {
	// Single marker: ID, participant, exclusive flag, entry pointer,
	// description. Format mirrors `sdd wip list` so users see the same
	// shape across surfaces.
	g := model.NewGraph(nil)
	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-wip-list",
			Data: model.WipList{Markers: []*model.WIPMarker{
				{
					ID:          "20260507-120000-alice",
					Entry:       "20260101-100000-d-tac-aaa",
					Participant: "Alice",
					Exclusive:   true,
					Content:     "implementing slice 8",
					Time:        time.Date(2026, 5, 7, 12, 0, 0, 0, time.UTC),
				},
			}},
		}},
	}
	got := renderView(result)
	if !strings.Contains(got, "20260507-120000-alice") {
		t.Errorf("missing marker ID in output:\n%s", got)
	}
	if !strings.Contains(got, "Alice") {
		t.Errorf("missing participant in output:\n%s", got)
	}
	if !strings.Contains(got, "[exclusive]") {
		t.Errorf("missing exclusive marker in output:\n%s", got)
	}
	if !strings.Contains(got, "20260101-100000-d-tac-aaa") {
		t.Errorf("missing entry pointer in output:\n%s", got)
	}
}

func TestRenderView_AsWipList_EmptyMessage(t *testing.T) {
	// Empty marker set still renders an explanatory line — keeps the
	// section visible (matters when a layout includes `wip` next to
	// other sections so the absence is visible, not silent).
	g := model.NewGraph(nil)
	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-wip-list",
			Data:   model.WipList{Markers: nil},
		}},
	}
	got := renderView(result)
	if !strings.Contains(got, "No active WIP markers.") {
		t.Errorf("missing empty-state message:\n%s", got)
	}
}

func TestRenderView_AsParticipantsBlock(t *testing.T) {
	// One actor with one bound role. Group header is the canonical;
	// actor entry-line and role entry-line render underneath.
	actor := entry("20260424-100000-s-prc-act",
		withKind(model.KindActor),
		withCanonical("Christopher"),
		withParticipants("Christopher"),
		withConfidence("high"),
		withSummary("Christopher actor"))
	role := entry("20260424-110000-d-prc-rol",
		withKind(model.KindRole),
		withActor("Christopher"),
		withParticipants("Christopher"),
		withConfidence("medium"),
		withSummary("Designer and principal developer"))
	g := model.NewGraph([]*model.Entry{actor, role})

	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-participants-block",
			Data: model.ParticipantsBlock{Groups: []model.ParticipantsGroup{
				{Actor: actor, Roles: []*model.Entry{role}},
			}},
		}},
	}
	got := renderView(result)
	if !strings.Contains(got, "### Christopher") {
		t.Errorf("missing canonical header:\n%s", got)
	}
	if !strings.Contains(got, "Christopher actor") {
		t.Errorf("missing actor entry-line:\n%s", got)
	}
	if !strings.Contains(got, "Designer and principal developer") {
		t.Errorf("missing role entry-line:\n%s", got)
	}
}

func TestRenderView_AsParticipantsBlock_EmptyGroupsSuppressed(t *testing.T) {
	// Grace mode: zero groups → no output (the renderer drops the section
	// entirely so an actorless graph stays quiet).
	g := model.NewGraph(nil)
	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-participants-block",
			Data:   model.ParticipantsBlock{Groups: nil},
		}},
	}
	if got := renderView(result); got != "" {
		t.Errorf("expected suppressed output for empty groups, got:\n%s", got)
	}
}

func TestRenderView_AutoDerivedHeaderRendersAsTitle(t *testing.T) {
	// When SectionResult.Name is set (auto-derived or explicit), it
	// renders as `## <name>` above the section body. Slice 7 added the
	// Name-renders-as-header behaviour; slice 8 verifies auto-derived
	// values flow through unchanged.
	a := entry("20260101-100000-d-tac-aaa",
		withKind(model.KindDirective),
		withParticipants("Christopher"),
		withSummary("Entry A"))
	g := model.NewGraph([]*model.Entry{a})
	result := &query.ViewResult{
		Graph: g,
		Sections: []query.SectionResult{{
			Render: "as-list",
			Name:   "Top by heat (exp-14d)",
			Data:   model.FlatList{Entries: []*model.Entry{a}, Scores: []float64{1.5}},
		}},
	}
	got := renderView(result)
	if !strings.Contains(got, "## Top by heat (exp-14d)") {
		t.Errorf("auto-derived header not rendered:\n%s", got)
	}
}

// renderView helper provided in view_test.go.
