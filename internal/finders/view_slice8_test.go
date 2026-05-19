package finders

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// Slice 8 coverage: source(wip) + as-wip-list, as-participants-block,
// participants/wip macros, auto-derive per d-tac-jgi, and the new
// rendering/error paths these add. Tests live in a separate file so the
// slice-1-7 view_test.go stays focused on the established surface.

func TestView_SourceWip_AsWipList(t *testing.T) {
	// Two markers in a temp graph dir; source(wip) reads them, the
	// section terminates in as-wip-list, and the result carries both.
	dir := t.TempDir()
	writeMarker(t, dir, "20260507-120000-alice", "20260101-100000-d-tac-aaa", "Alice", true, "first marker")
	writeMarker(t, dir, "20260507-130000-bob", "20260101-110000-d-tac-bbb", "Bob", false, "second marker")

	g := model.NewGraph([]*model.Entry{
		entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective)),
		entry("20260101-110000-d-tac-bbb", withKind(model.KindDirective)),
	})
	layout := mustParseLayout(t, "source(wip):as-wip-list")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout, GraphDir: dir})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	if got, want := len(result.Sections), 1; got != want {
		t.Fatalf("sections: got %d, want %d", got, want)
	}
	section := result.Sections[0]
	if section.Render != "as-wip-list" {
		t.Errorf("render: got %q, want %q", section.Render, "as-wip-list")
	}
	list, ok := section.Data.(model.WipList)
	if !ok {
		t.Fatalf("section data: got %T, want model.WipList", section.Data)
	}
	if got, want := len(list.Markers), 2; got != want {
		t.Fatalf("markers: got %d, want %d", got, want)
	}
}

func TestView_WipMacro_ExpandsToSourceWip(t *testing.T) {
	// The `wip` macro should expand to `source(wip):as-wip-list` and
	// produce identical output to the explicit form.
	dir := t.TempDir()
	writeMarker(t, dir, "20260507-120000-alice", "20260101-100000-d-tac-aaa", "Alice", true, "marker via macro")

	g := model.NewGraph([]*model.Entry{
		entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective)),
	})
	layout := mustParseLayoutAndExpand(t, "wip")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout, GraphDir: dir})
	if err != nil {
		t.Fatalf("View(wip macro): %v", err)
	}
	if got, want := len(result.Sections), 1; got != want {
		t.Fatalf("sections: got %d, want %d", got, want)
	}
	if got, want := result.Sections[0].Render, "as-wip-list"; got != want {
		t.Errorf("render: got %q, want %q", got, want)
	}
}

func TestView_SourceWip_RejectsGraphFilters(t *testing.T) {
	// Filters that are graph-side concepts must error against source(wip)
	// rather than silently no-op. Each error message names the offending
	// primitive so users can locate it in the layout string.
	cases := []struct {
		name    string
		layout  string
		wantSub string
	}{
		{"active", "source(wip):active:as-wip-list", "graph filters"},
		{"kind", "source(wip):kind(plan):as-wip-list", "kind() filters"},
		{"layer", "source(wip):layer(tac):as-wip-list", "graph filters"},
		{"since", "source(wip):since(\"7d\"):as-wip-list", "since()"},
		{"topic", "source(wip):topic(catch-up-scaling):as-wip-list", "topic() filters"},
		{"rank", "source(wip):rank(heat):as-wip-list", "rank()"},
		{"n", "source(wip):n(5):as-wip-list", "n()"},
		{"group", "source(wip):group(by(kind)):as-wip-list", "group()"},
		{"expand", "source(wip):expand(involvement):as-wip-list", "expand()"},
		{"stalled", "source(wip):stalled(0.5):as-wip-list", "stalled()"},
	}
	g := model.NewGraph(nil)
	dir := t.TempDir()
	f := New(Options{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			layout := mustParseLayout(t, tc.layout)
			_, err := f.View(query.ViewQuery{Graph: g, Layout: layout, GraphDir: dir})
			if err == nil {
				t.Fatalf("expected error mentioning %q for %q", tc.wantSub, tc.layout)
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error: got %q, want substring %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestView_SourceWip_RejectsOtherRenders(t *testing.T) {
	g := model.NewGraph(nil)
	dir := t.TempDir()
	f := New(Options{})
	layout := mustParseLayout(t, "source(wip):as-list")
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout, GraphDir: dir})
	if err == nil {
		t.Fatal("expected render-shape error for source(wip):as-list")
	}
	if !strings.Contains(err.Error(), "as-wip-list") {
		t.Errorf("error: got %q, want hint to use as-wip-list", err.Error())
	}
}

func TestView_AsWipList_RequiresSourceWip(t *testing.T) {
	// as-wip-list under source(graph) (the default) errors with a
	// pointer to source(wip) so the user understands they need to switch
	// data sources, not change the render alone.
	g := model.NewGraph(nil)
	f := New(Options{})
	layout := mustParseLayout(t, "as-wip-list")
	_, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err == nil {
		t.Fatal("expected error for as-wip-list with default source")
	}
	if !strings.Contains(err.Error(), "source(wip)") {
		t.Errorf("error: got %q, want hint about source(wip)", err.Error())
	}
}

func TestView_SourceWip_MissingGraphDirErrors(t *testing.T) {
	// Layout uses source(wip) but the query carries no GraphDir — the
	// pre-scan in View() catches this rather than letting LoadWIPMarkers
	// fail with a less informative message.
	g := model.NewGraph(nil)
	f := New(Options{})
	layout := mustParseLayout(t, "wip")
	expanded, err := query.ExpandMacros(layout)
	if err != nil {
		t.Fatalf("ExpandMacros: %v", err)
	}
	_, err = f.View(query.ViewQuery{Graph: g, Layout: expanded /* GraphDir omitted */})
	if err == nil {
		t.Fatal("expected error for source(wip) without GraphDir")
	}
	if !strings.Contains(err.Error(), "graph directory") {
		t.Errorf("error: got %q, want hint about graph directory", err.Error())
	}
}

func TestView_AsParticipantsBlock(t *testing.T) {
	// A graph with one active actor head + one bound role renders as a
	// single participants group. The cascade walks chain history, not
	// the role's stored canonical alone.
	actor := actorEntry("Christopher", nil)
	role := &model.Entry{
		ID:    "20260424-110000-d-prc-rol",
		Type:  model.TypeDecision,
		Kind:  model.KindRole,
		Layer: model.LayerProcess,
		Actor: "Christopher",
		Refs:  refsOf(actor.ID),
	}
	g := model.NewGraph([]*model.Entry{actor, role})

	layout := mustParseLayoutAndExpand(t, "participants")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	section := result.Sections[0]
	if section.Render != "as-participants-block" {
		t.Fatalf("render: got %q, want as-participants-block", section.Render)
	}
	block, ok := section.Data.(model.ParticipantsBlock)
	if !ok {
		t.Fatalf("section data: got %T, want model.ParticipantsBlock", section.Data)
	}
	if got, want := len(block.Groups), 1; got != want {
		t.Fatalf("groups: got %d, want %d", got, want)
	}
	if got, want := block.Groups[0].Actor.Canonical, "Christopher"; got != want {
		t.Errorf("group canonical: got %q, want %q", got, want)
	}
	if got, want := len(block.Groups[0].Roles), 1; got != want {
		t.Errorf("bound roles: got %d, want %d", got, want)
	}
}

func TestView_AsParticipantsBlock_GraceMode(t *testing.T) {
	// Zero active actor heads → empty block. Renderer suppresses output.
	g := model.NewGraph(nil)
	layout := mustParseLayoutAndExpand(t, "participants")
	f := New(Options{})
	result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
	if err != nil {
		t.Fatalf("View: %v", err)
	}
	block, ok := result.Sections[0].Data.(model.ParticipantsBlock)
	if !ok {
		t.Fatalf("section data: got %T, want model.ParticipantsBlock", result.Sections[0].Data)
	}
	if len(block.Groups) != 0 {
		t.Errorf("expected empty groups during grace, got %d", len(block.Groups))
	}
}

func TestView_AsParticipantsBlock_RejectsRankAndN(t *testing.T) {
	// rank() and n() are flat-list concepts that don't apply to the
	// participants block — explicit rejection prevents silent surprise.
	g := model.NewGraph([]*model.Entry{actorEntry("Christopher", nil)})
	f := New(Options{})

	for _, layoutStr := range []string{
		"kind(actor):rank(heat):as-participants-block",
		"kind(actor):n(5):as-participants-block",
	} {
		t.Run(layoutStr, func(t *testing.T) {
			_, err := f.View(query.ViewQuery{Graph: g, Layout: mustParseLayout(t, layoutStr)})
			if err == nil {
				t.Fatal("expected rejection error")
			}
			if !strings.Contains(err.Error(), "as-participants-block") {
				t.Errorf("error: got %q, want as-participants-block context", err.Error())
			}
		})
	}
}

func TestView_AutoDeriveSectionName(t *testing.T) {
	// AC 14 per d-tac-jgi: when no name() supplied and rank set, the
	// section header is synthesized. Each rank shape produces a stable
	// label so future rank-comparison consumers can dispatch on it.
	cases := []struct {
		name    string
		layout  string
		wantHdr string
	}{
		{"heat-default", "rank(heat):as-list", "Top by heat (exp-14d)"},
		{"heat-explicit-decay", "rank(heat(exp-7d)):as-list", "Top by heat (exp-7d)"},
		{"in-degree", "rank(in-degree):as-list", "Top by in-degree"},
		{"by-date", "rank(by(date)):as-list", "Top by date"},
		{"mult", "rank(mult):as-list", "Top by mult (exp-14d)"},
		{"explicit-name-wins", "rank(heat):name(\"Custom\"):as-list", "Custom"},
		{"no-rank-no-name", "active:as-list", ""},
		{"empty-name-clears", "rank(heat):name(\"\"):as-list", ""},
	}
	g := model.NewGraph([]*model.Entry{
		entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective)),
	})
	f := New(Options{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			layout := mustParseLayout(t, tc.layout)
			result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
			if err != nil {
				t.Fatalf("View(%q): %v", tc.layout, err)
			}
			if got := result.Sections[0].Name; got != tc.wantHdr {
				t.Errorf("name: got %q, want %q", got, tc.wantHdr)
			}
		})
	}
}

func TestView_NamePrefixComposesWithRank(t *testing.T) {
	// name-prefix(...) is the macro-side bake; auto-derive composes
	// "<prefix> <suffix>" when rank is set, falls back to prefix-only
	// when not, and explicit name(...) overrides both.
	cases := []struct {
		name    string
		layout  string
		wantHdr string
	}{
		{"prefix+rank", `name-prefix("Top"):rank(heat):as-list`, "Top by heat (exp-14d)"},
		{"prefix+in-degree", `name-prefix("Top"):rank(in-degree):as-list`, "Top by in-degree"},
		{"prefix+by-date", `name-prefix("Done"):rank(by(date)):as-list`, "Done by date"},
		{"prefix-only", `name-prefix("Focus"):as-list`, "Focus"},
		{"explicit-name-wins-over-prefix", `name-prefix("Top"):name("Custom"):rank(heat):as-list`, "Custom"},
		{"prefix-with-spaces", `name-prefix("Topic: infrastructure/cli"):rank(heat):as-list`, "Topic: infrastructure/cli by heat (exp-14d)"},
		{"empty-prefix-with-rank", `name-prefix(""):rank(heat):as-list`, " by heat (exp-14d)"},
		{"empty-prefix-alone", `name-prefix(""):as-list`, ""},
		{"last-write-wins-prefix", `name-prefix("First"):name-prefix("Second"):as-list`, "Second"},
	}
	g := model.NewGraph([]*model.Entry{
		entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective)),
	})
	f := New(Options{})
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			layout := mustParseLayout(t, tc.layout)
			result, err := f.View(query.ViewQuery{Graph: g, Layout: layout})
			if err != nil {
				t.Fatalf("View(%q): %v", tc.layout, err)
			}
			if got := result.Sections[0].Name; got != tc.wantHdr {
				t.Errorf("name: got %q, want %q", got, tc.wantHdr)
			}
		})
	}
}

// writeMarker is a slice-8 test helper that drops one marker file into
// the wip/ subdirectory of the given graph dir. Mirrors what the wip
// command would write so source(wip) sees realistic frontmatter.
func writeMarker(t *testing.T, graphDir, id, entryID, participant string, exclusive bool, content string) {
	t.Helper()
	wipDir := filepath.Join(graphDir, "wip")
	if err := os.MkdirAll(wipDir, 0o755); err != nil {
		t.Fatalf("mkdir wip: %v", err)
	}
	excl := ""
	if exclusive {
		excl = "exclusive: true\n"
	}
	body := "---\nentry: " + entryID + "\nparticipant: " + participant + "\n" + excl + "---\n\n" + content + "\n"
	if err := os.WriteFile(filepath.Join(wipDir, id+".md"), []byte(body), 0o644); err != nil {
		t.Fatalf("writeFile: %v", err)
	}
}
