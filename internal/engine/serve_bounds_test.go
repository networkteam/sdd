package engine_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/serveview"
	"github.com/networkteam/sdd/internal/truncate"
)

func longLines(n int) string {
	var b strings.Builder
	for i := range n {
		b.WriteString(strings.Repeat("x", 40))
		if i < n-1 {
			b.WriteByte('\n')
		}
	}
	return b.String()
}

func TestDeclaredMaxBytesCutsInjectNotAuthoredText(t *testing.T) {
	reg := engine.NewRegistry()
	payload := longLines(50) // ~2050 bytes
	if err := reg.RegisterQuery(engine.Query{
		Doc: engine.FuncDoc{Name: "bigQuery", Doc: "test blob"},
		Fn:  func(*engine.Context, map[string]any) (any, error) { return payload, nil },
	}); err != nil {
		t.Fatal(err)
	}
	body := "## unit: draft\n\n### lane: intro\n\nAuthored intro text stays whole.\n\n### lane: data\n\n{{.big}}\n"
	sv := startServe(t, reg, injectMachine("          - {id: big, fn: bigQuery, maxBytes: 500}"), body)

	if got := serveLaneText(t, sv, "intro"); got != "Authored intro text stays whole." {
		t.Errorf("authored lane changed: %q", got)
	}
	data := serveLaneText(t, sv, "data")
	if len(data) > 500 || strings.Contains(data, "truncated") {
		t.Errorf("data lane = %dB %q — want the bounded payload with no inline notice", len(data), data)
	}
	cutsLane := serveLaneText(t, sv, "cuts")
	if !strings.Contains(cutsLane, "big") || !strings.Contains(cutsLane, "bytes") {
		t.Errorf("cuts lane = %q, want the big inject's cut named", cutsLane)
	}
	if len(sv.Cuts) != 1 || sv.Cuts[0].Part != "big" || sv.Cuts[0].TotalBytes != len(payload) {
		t.Errorf("Serve.Cuts = %+v", sv.Cuts)
	}
}

func TestLegacyArgsMaxBytesFoldsIntoTheCap(t *testing.T) {
	reg := engine.NewRegistry()
	sawMaxBytesArg := false
	if err := reg.RegisterQuery(engine.Query{
		Doc: engine.FuncDoc{Name: "legacyQuery", Doc: "test legacy cap"},
		Fn: func(_ *engine.Context, args map[string]any) (any, error) {
			_, sawMaxBytesArg = args["maxBytes"]
			return longLines(50), nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	body := "## unit: draft\n\n### lane: data\n\n{{.legacyQuery}}\n"
	sv := startServe(t, reg, injectMachine("          - {fn: legacyQuery, args: {maxBytes: 500}}"), body)

	if sawMaxBytesArg {
		t.Error("legacy args.maxBytes must fold into the cap, not reach the query")
	}
	if data := serveLaneText(t, sv, "data"); len(data) > 500 {
		t.Errorf("data lane = %dB, want the legacy cap applied", len(data))
	}
	if len(sv.Cuts) != 1 {
		t.Fatalf("Serve.Cuts = %+v, want the legacy-declared cut", sv.Cuts)
	}
}

func TestCarrierResultUnwrapsWithItsPull(t *testing.T) {
	reg := engine.NewRegistry()
	if err := reg.RegisterQuery(engine.Query{
		Doc: engine.FuncDoc{Name: "listQuery", Doc: "test carrier"},
		Fn: func(*engine.Context, map[string]any) (any, error) {
			return truncate.Head([]string{"kept", "dropped-a", "dropped-b"}, 1, "list:skip(1)"), nil
		},
	}); err != nil {
		t.Fatal(err)
	}
	body := "## unit: draft\n\n### lane: rows\n\n{{range .listQuery}}- {{.}}\n{{end}}\n"
	sv := startServe(t, reg, injectMachine("          - {fn: listQuery}"), body)

	rows := serveLaneText(t, sv, "rows")
	if !strings.Contains(rows, "kept") || strings.Contains(rows, "dropped-a") {
		t.Errorf("rows lane = %q, want only the kept items rendered", rows)
	}
	cutsLane := serveLaneText(t, sv, "cuts")
	if !strings.Contains(cutsLane, "2 of 3 items dropped") || !strings.Contains(cutsLane, "list:skip(1)") {
		t.Errorf("cuts lane = %q, want the drop count and the producer's pull", cutsLane)
	}
}

func TestStoreValueBoundsAtTheUnitContext(t *testing.T) {
	spec := mustParseLanes(t, laneMachine, "## unit: draft\n\n### lane: note\n\n{{.note}}\n")
	eng := engine.New(engine.NewRegistry(), engine.StaticGraphs{Graph: model.NewGraph(nil)})
	long := longLines(100) // ~4100 bytes, over the 2000-byte store-value default
	sv, err := eng.NewSession("s_store", "tester", nopSink{}).Start(spec, map[string]any{"note": long}, "")
	if err != nil {
		t.Fatal(err)
	}
	note := serveLaneText(t, sv, "note")
	if len(note) > 2000 || strings.Contains(note, "truncated") {
		t.Errorf("note lane = %dB — want the store value bounded with no inline notice", len(note))
	}
	if len(sv.Cuts) != 1 || sv.Cuts[0].Part != "store:note" || sv.Cuts[0].TotalBytes != len(long) {
		t.Errorf("Serve.Cuts = %+v, want the store:note cut", sv.Cuts)
	}
}

func TestDraftListRenderCutsAtWholeItems(t *testing.T) {
	machine := `state:
    topics: {type: list<label>, desc: labels}
    goal: {type: text, optional: true, desc: closer}
steps:
    - id: draft
      collect: [topics, "goal?"]
      serveDelta: [topics]
      transitions:
          - when: hasGoal
            to: end(completed)`
	spec := mustParseLanes(t, machine, "## unit: draft\n\nDraft guidance.\n")
	items := make([]any, 300)
	for i := range items {
		items[i] = strings.Repeat("t", 30) // ~9900 rendered bytes, over the 6000-byte draft default
	}
	eng := engine.New(engine.NewRegistry(), engine.StaticGraphs{Graph: model.NewGraph(nil)})
	sv, err := eng.NewSession("s_draftcut", "tester", nopSink{}).Start(spec, map[string]any{"topics": items}, "")
	if err != nil {
		t.Fatal(err)
	}
	draft := serveLaneText(t, sv, "draft")
	if len(draft) > 7000 {
		t.Errorf("draft lane = %dB, want the list render bounded", len(draft))
	}
	var found *struct{ dropped, total int }
	for _, c := range sv.Cuts {
		if c.Part == "draft:topics" {
			found = &struct{ dropped, total int }{c.Dropped, c.Total}
		}
	}
	if found == nil || found.dropped == 0 || found.total != 300 {
		t.Errorf("Serve.Cuts = %+v, want a whole-item draft:topics cut over 300 items", sv.Cuts)
	}
}

func TestRegistrationDefaultCapAppliesWhenSpecIsSilent(t *testing.T) {
	reg := engine.NewRegistry()
	if err := reg.RegisterQuery(engine.Query{
		Doc:   engine.FuncDoc{Name: "boundedQuery", Doc: "test registration default"},
		Bound: engine.QueryBound{Part: serveview.PartText, Cap: serveview.Cap{MaxBytes: 300}},
		Fn:    func(*engine.Context, map[string]any) (any, error) { return longLines(50), nil },
	}); err != nil {
		t.Fatal(err)
	}
	body := "## unit: draft\n\n### lane: data\n\n{{.boundedQuery}}\n"
	sv := startServe(t, reg, injectMachine("          - {fn: boundedQuery}"), body)
	if data := serveLaneText(t, sv, "data"); len(data) > 300 {
		t.Errorf("data lane = %dB, want the registration default applied", len(data))
	}
}
