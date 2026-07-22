package presenters_test

import (
	"bytes"
	"testing"

	"github.com/bradleyjkemp/cupaloy/v2"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
)

// renderShow exercises the finder + presenter together.
func renderShow(t *testing.T, g *model.Graph, ids []string, opts ...showOpt) string {
	t.Helper()
	cfg := showConfig{q: query.ShowQuery{IDs: ids, UpDepth: query.DefaultUpDepth, DownDepth: query.DefaultDownDepth}}
	for _, o := range opts {
		o(&cfg)
	}
	f := finders.New(finders.Options{})
	result, err := f.OnGraph(g).Show(cfg.q)
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	presenters.RenderShow(&buf, result, cfg.render)
	return buf.String()
}

// showConfig threads both query options (graph traversal) and render options
// (presentation) through the renderShow helper.
type showConfig struct {
	q      query.ShowQuery
	render presenters.ShowOptions
}

type showOpt func(*showConfig)

func withUpDepth(d int) showOpt {
	return func(c *showConfig) { c.q.UpDepth = d }
}

func renderWithSummary() showOpt {
	return func(c *showConfig) { c.render.WithSummary = true }
}

func TestRenderShow_SingleEntryNoRefs(t *testing.T) {
	e := entry("20260410-100000-s-tac-aaa", withContent("A signal about something"))
	g := model.NewGraph([]*model.Entry{e})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{e.ID}))
}

func TestRenderShow_UpstreamChain(t *testing.T) {
	root := entry("20260410-100000-s-stg-aaa", withSummary("Root signal about the foundation"))
	mid := entry("20260410-100100-s-cpt-bbb", withSummary("Middle observation building on root"),
		withRefs("20260410-100000-s-stg-aaa"))
	primary := entry("20260410-100200-d-tac-ccc", withContent("Decision based on observations"),
		withRefs("20260410-100100-s-cpt-bbb"))

	g := model.NewGraph([]*model.Entry{root, mid, primary})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{primary.ID}))
}

func TestRenderShow_CrossRepoRefUnresolved(t *testing.T) {
	// A cross-repo ref renders as an upstream leaf with the full prefixed ID
	// and the bracketed unresolved marker; the envelope carries it verbatim.
	primary := entry("20260410-100200-d-tac-ccc", withContent("Decision grounded in a remote entry"))
	primary.Refs = []model.Ref{
		{ID: "github.com/networkteam/other:20260401-090000-d-cpt-rem", Kind: model.RefKindGroundedIn, Desc: "remote basis"},
	}
	g := model.NewGraph([]*model.Entry{primary})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{primary.ID}))
}

func TestRenderShow_DownstreamWithRelations(t *testing.T) {
	target := entry("20260410-100000-s-stg-aaa", withContent("Target signal"))
	refBy := entry("20260410-100100-d-cpt-bbb", withSummary("Decision referencing target"),
		withRefs("20260410-100000-s-stg-aaa"))
	closedBy := entry("20260410-100200-a-tac-ccc", withSummary("Action closing target"),
		withCloses("20260410-100000-s-stg-aaa"))

	g := model.NewGraph([]*model.Entry{target, refBy, closedBy})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{target.ID}))
}

func TestRenderShow_MultiPrimaryDedup(t *testing.T) {
	shared := entry("20260410-100000-s-stg-aaa", withSummary("Shared ref"))
	first := entry("20260410-100100-d-cpt-bbb", withContent("First primary"),
		withRefs("20260410-100000-s-stg-aaa"))
	second := entry("20260410-100200-d-cpt-ccc", withContent("Second primary"),
		withRefs("20260410-100000-s-stg-aaa"))

	g := model.NewGraph([]*model.Entry{shared, first, second})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{first.ID, second.ID}))
}

func TestRenderShow_BranchingWithDedup(t *testing.T) {
	shared := entry("20260410-100000-s-stg-ddd", withSummary("Shared deep ref"))
	b := entry("20260410-100100-s-cpt-bbb", withSummary("Branch B"), withRefs("20260410-100000-s-stg-ddd"))
	c := entry("20260410-100200-s-cpt-ccc", withSummary("Branch C"), withRefs("20260410-100000-s-stg-ddd"))
	primary := entry("20260410-100300-d-tac-aaa", withContent("Primary with two branches"),
		withRefs("20260410-100100-s-cpt-bbb", "20260410-100200-s-cpt-ccc"))

	g := model.NewGraph([]*model.Entry{shared, b, c, primary})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{primary.ID}))
}

func TestRenderShow_CombinedRelationsAndKind(t *testing.T) {
	signal := entry("20260410-100000-s-tac-aaa", withSummary("The signal"))
	contract := entry("20260410-100050-d-tac-ddd", withKind(model.KindContract), withSummary("A contract"))
	plan := entry("20260410-100055-d-tac-eee", withKind(model.KindPlan), withSummary("A plan"))
	done := entry("20260410-100100-s-tac-bbb", withKind(model.KindDone), withContent("Done signal with combined relations"),
		withRefs("20260410-100000-s-tac-aaa", "20260410-100050-d-tac-ddd", "20260410-100055-d-tac-eee"),
		withCloses("20260410-100000-s-tac-aaa"))

	g := model.NewGraph([]*model.Entry{signal, contract, plan, done})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{done.ID}))
}

func TestRenderShow_MaxDepthTruncation(t *testing.T) {
	e0 := entry("20260410-100000-s-stg-aaa", withSummary("Root"))
	e1 := entry("20260410-100100-s-cpt-bbb", withSummary("Level 1"), withRefs("20260410-100000-s-stg-aaa"))
	e2 := entry("20260410-100200-s-tac-ccc", withSummary("Level 2"), withRefs("20260410-100100-s-cpt-bbb"))
	primary := entry("20260410-100300-d-tac-ddd", withContent("Primary"),
		withRefs("20260410-100200-s-tac-ccc"))

	g := model.NewGraph([]*model.Entry{e0, e1, e2, primary})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{primary.ID}, withUpDepth(2)))
}

func TestRenderShow_FallbackFirstSentence(t *testing.T) {
	a := entry("20260410-100000-s-stg-aaa", withContent("First sentence of content.\nSecond line."))
	b := entry("20260410-100100-d-tac-bbb", withContent("Primary"), withRefs("20260410-100000-s-stg-aaa"))

	g := model.NewGraph([]*model.Entry{a, b})
	cupaloy.SnapshotT(t, renderShow(t, g, []string{b.ID}))
}

func TestRenderShow_RefKindAndDesc(t *testing.T) {
	target := entry("20260410-100000-s-stg-aaa", withContent("Target observation"))
	primary := &model.Entry{
		ID:      "20260410-100100-d-tac-bbb",
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Content: "Primary entry referencing the target.",
		Refs: []model.Ref{{
			ID:   target.ID,
			Kind: model.RefKindAddresses,
			Desc: "resolves the target observation",
		}},
		Time: target.Time,
	}
	g := model.NewGraph([]*model.Entry{target, primary})
	out := renderShow(t, g, []string{primary.ID})

	// Envelope: refs render in object form with id, kind, and desc.
	wantEnvelope := []string{
		"refs:",
		"id: 20260410-100000-s-stg-aaa",
		"kind: addresses",
		"desc: resolves the target observation",
	}
	for _, w := range wantEnvelope {
		if !contains(out, w) {
			t.Errorf("envelope missing %q in:\n%s", w, out)
		}
	}

	// Upstream tree: the kind is the verb, desc on an indented sub-line.
	if !contains(out, "- addresses 20260410-100000-s-stg-aaa (gap, open)") {
		t.Errorf("tree missing kind-verb node in:\n%s", out)
	}
}

func TestRenderShow_DownstreamCarriesRefKind(t *testing.T) {
	primary := entry("20260410-100000-s-stg-tgt", withContent("Target"))
	source := &model.Entry{
		ID:      "20260410-100100-d-tac-src",
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Content: "Source decision",
		Refs: []model.Ref{{
			ID:   primary.ID,
			Kind: model.RefKindGroundedIn,
			Desc: "anchors to the standing observation",
		}},
		Time: primary.Time,
	}
	g := model.NewGraph([]*model.Entry{primary, source})
	out := renderShow(t, g, []string{primary.ID})

	// Downstream tree: a refd-by edge carrying a kind renders the kind as the
	// verb (the `# downstream` header carries the incoming direction).
	if !contains(out, "- grounded-in 20260410-100100-d-tac-src (directive, active)") {
		t.Errorf("downstream missing kind-verb node in:\n%s", out)
	}
	if !contains(out, "↳ anchors to the standing observation") {
		t.Errorf("downstream missing why sub-line in:\n%s", out)
	}
}

func TestRenderShow_LegacyRefs(t *testing.T) {
	// Entries on disk with bare-string refs parse as kind: unknown. The
	// envelope marshals them in object form (kind: unknown); the tree renders
	// the generic "refs" verb since there's no meaningful kind to surface.
	target := entry("20260410-100000-s-stg-aaa", withContent("Target"))
	primary := &model.Entry{
		ID:      "20260410-100100-d-tac-bbb",
		Type:    model.TypeDecision,
		Layer:   model.LayerTactical,
		Content: "Primary entry with legacy refs.",
		Refs:    []model.Ref{{ID: target.ID, Kind: model.RefKindUnknown}},
		Time:    target.Time,
	}
	g := model.NewGraph([]*model.Entry{target, primary})
	out := renderShow(t, g, []string{primary.ID})
	if !contains(out, "kind: unknown") {
		t.Errorf("legacy ref should render kind: unknown in the envelope, got:\n%s", out)
	}
	if !contains(out, "- refs 20260410-100000-s-stg-aaa (gap, open)") {
		t.Errorf("legacy ref should render the generic refs verb in the tree, got:\n%s", out)
	}
}

func TestRenderShow_EntryNotFound(t *testing.T) {
	g := model.NewGraph([]*model.Entry{})

	f := finders.New(finders.Options{})
	_, err := f.OnGraph(g).Show(query.ShowQuery{IDs: []string{"20260410-100000-s-stg-xxx"}})
	if err == nil {
		t.Fatal("expected error for missing entry")
	}
}

func TestRenderShow_SummaryOmittedByDefault(t *testing.T) {
	e := entry("20260410-100000-d-tac-aaa",
		withKind(model.KindPlan),
		withSummary("Compact synthesis of the plan."),
		withContent("Body content describing the plan in full detail."))
	g := model.NewGraph([]*model.Entry{e})
	out := renderShow(t, g, []string{e.ID})
	if contains(out, "summary:") {
		t.Errorf("summary should be omitted from the envelope by default:\n%s", out)
	}
	if contains(out, "Compact synthesis of the plan.") {
		t.Errorf("summary text should not appear by default:\n%s", out)
	}
}

func TestRenderShow_SupersededPrimaryTrail(t *testing.T) {
	// A primary superseded through more than one hop renders its full
	// origin→head trail in the envelope status — the supersede-chain
	// resolution the tree-node/envelope status slot must accept.
	origin := entry("20260410-100000-d-tac-aaa", withContent("Origin"))
	mid := entry("20260410-100100-d-tac-bbb", withContent("Mid"), withSupersedes(origin.ID))
	head := entry("20260410-100200-d-tac-ccc", withContent("Head"), withSupersedes(mid.ID))
	g := model.NewGraph([]*model.Entry{origin, mid, head})
	out := renderShow(t, g, []string{origin.ID})
	want := "status: superseded-by 20260410-100100-d-tac-bbb → 20260410-100200-d-tac-ccc"
	if !contains(out, want) {
		t.Errorf("expected supersede trail %q in envelope:\n%s", want, out)
	}
}

func TestRenderShow_EnvelopeIntent(t *testing.T) {
	// A directive's stored intent mirrors into the envelope; a settled
	// directive additionally derives status: settled.
	guiding := entry("20260410-100000-d-stg-gid", withKind(model.KindDirective), withIntent(model.IntentGuiding), withContent("Standing context"))
	settled := entry("20260410-100100-d-tac-set", withKind(model.KindDirective), withIntent(model.IntentSettled), withContent("Born terminal"))
	g := model.NewGraph([]*model.Entry{guiding, settled})

	gOut := renderShow(t, g, []string{guiding.ID})
	if !contains(gOut, "intent: guiding") {
		t.Errorf("guiding directive envelope should mirror stored intent:\n%s", gOut)
	}

	sOut := renderShow(t, g, []string{settled.ID})
	if !contains(sOut, "intent: settled") {
		t.Errorf("settled directive envelope should mirror stored intent:\n%s", sOut)
	}
	if !contains(sOut, "status: settled") {
		t.Errorf("settled directive envelope should derive status settled:\n%s", sOut)
	}
}

func TestRenderShow_SummaryShownWithFlag(t *testing.T) {
	e := entry("20260410-100000-d-tac-aaa",
		withKind(model.KindPlan),
		withSummary("Compact synthesis of the plan."),
		withContent("Body content describing the plan in full detail."))
	g := model.NewGraph([]*model.Entry{e})
	out := renderShow(t, g, []string{e.ID}, renderWithSummary())
	if !contains(out, "summary: Compact synthesis of the plan.") {
		t.Errorf("summary should appear in the envelope with --with-summary:\n%s", out)
	}
}

func TestRenderShow_NestedFactIndex(t *testing.T) {
	index, err := model.NewFactIndex("How to compose graph views", "cli/view")
	if err != nil {
		t.Fatal(err)
	}
	topic, _ := model.ParseTopicPath("cli/view")
	e := entry("20260719-120000-s-tac-idx", withKind(model.KindFact), withContent("Reference body."))
	e.Topics = []model.TopicPath{topic}
	e.Index = index
	g := model.NewGraph([]*model.Entry{e})
	out := renderShow(t, g, []string{e.ID})
	if !contains(out, "index:\n    title: How to compose graph views\n    topic: cli/view\n") {
		t.Fatalf("show output missing nested index:\n%s", out)
	}
}

func TestRenderShowStyled_ColorDisabled(t *testing.T) {
	// Writing to a non-TTY buffer downsamples to Ascii through the colorprofile
	// writer, so the styled output arrives color-free — we assert on its plain
	// structure (envelope, glamour-rendered body, styled tree node).
	root := entry("20260410-100000-s-stg-aaa", withSummary("Root signal about the foundation"))
	primary := entry("20260410-100100-d-tac-ccc",
		withContent("Decision body paragraph rendered through glamour."),
		withRefs("20260410-100000-s-stg-aaa"))
	g := model.NewGraph([]*model.Entry{root, primary})

	f := finders.New(finders.Options{})
	result, err := f.OnGraph(g).Show(query.ShowQuery{IDs: []string{primary.ID}, UpDepth: query.DefaultUpDepth, DownDepth: query.DefaultDownDepth})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	presenters.RenderShowStyled(&buf, result, presenters.ShowOptions{Width: 80})
	out := buf.String()

	for _, want := range []string{
		"id: 20260410-100100-d-tac-ccc",                    // envelope
		"Decision body paragraph rendered through glamour", // glamour body (color-free)
		"related 20260410-100000-s-stg-aaa",                // styled tree node, color stripped
		"# upstream",
	} {
		if !contains(out, want) {
			t.Errorf("styled (color-disabled) output missing %q in:\n%s", want, out)
		}
	}
}

// contains and indexOf are tiny local helpers to keep the assertions readable
// without importing strings just for two calls.
func contains(s, sub string) bool { return indexOf(s, sub) >= 0 }
func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}

func TestRenderShow_EnvelopeKind(t *testing.T) {
	// The envelope mirrors on-disk storage: the kind line reflects e.Kind
	// verbatim (shown when set, omitted when empty) rather than special-casing
	// directive the way the old full block did.
	tests := []struct {
		name     string
		kind     model.Kind
		wantKind string // "" means no kind line should appear
	}{
		{"plan", model.KindPlan, "kind: plan"},
		{"contract", model.KindContract, "kind: contract"},
		{"directive_explicit", model.KindDirective, "kind: directive"},
		{"empty_omitted", "", ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			e := entry("20260410-100000-d-tac-aaa", withKind(tt.kind), withContent("Test"))
			g := model.NewGraph([]*model.Entry{e})
			out := renderShow(t, g, []string{e.ID})
			if tt.wantKind == "" {
				if contains(out, "kind:") {
					t.Errorf("kind line should be omitted for %q:\n%s", tt.name, out)
				}
				return
			}
			if !contains(out, tt.wantKind) {
				t.Errorf("expected %q in envelope:\n%s", tt.wantKind, out)
			}
		})
	}
}
