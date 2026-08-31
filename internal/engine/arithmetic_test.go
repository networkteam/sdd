package engine_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/serveview"
)

func arithmeticEntry(t *testing.T, frontmatter, body string) *engine.Spec {
	t.Helper()
	content := "---\ntype: decision\nlayer: prc\nkind: procedure\ncanonical: sizedproc\n" +
		frontmatter + "\n---\n\n" + body
	entry, err := model.ParseEntry("20260831-120000-d-prc-szd.md", content)
	if err != nil {
		t.Fatalf("fixture entry: %v", err)
	}
	spec, err := engine.ParseSpec(entry)
	if err != nil {
		t.Fatalf("ParseSpec: %v", err)
	}
	return spec
}

func arithmeticRegistry(t *testing.T) *engine.Registry {
	t.Helper()
	reg := engine.NewRegistry()
	if err := reg.RegisterQuery(engine.Query{
		Doc:   engine.FuncDoc{Name: "wideQuery", Doc: "test"},
		Bound: engine.QueryBound{Part: serveview.PartText, Cap: serveview.Cap{MaxBytes: 20000}},
		Fn:    func(*engine.Context, map[string]any) (any, error) { return "", nil },
	}); err != nil {
		t.Fatal(err)
	}
	return reg
}

const sizedMachine = `state:
    report: {type: text, desc: x}
steps:
    - id: draft
      collect: [report]
      inject:
          - {fn: wideQuery}
          - {id: capped, fn: wideQuery, maxBytes: 4000}
      transitions:
          - when: hasBody
            to: end(completed)`

func TestWorstCaseSumsSkeletonCapsAndSlots(t *testing.T) {
	body := "## unit: draft\n\nGuidance " + strings.Repeat("x", 991) + "\n\n{{.report}}\n"
	spec := arithmeticEntry(t, sizedMachine, body)
	budget := serveview.Default()

	sizes := spec.WorstCaseServeBytes(budget, arithmeticRegistry(t))
	if len(sizes) != 1 || sizes[0].Step != "draft" {
		t.Fatalf("sizes = %+v", sizes)
	}
	// Skeleton ~1012 + registration cap 20000 + declared cap 4000 + one text
	// slot at the store-value cap.
	want := 20000 + 4000 + budget.Cap(serveview.PartStoreValue).MaxBytes
	if sizes[0].Bytes <= want || sizes[0].Bytes > want+1200 {
		t.Errorf("worst case = %d, want the caps plus the skeleton (just over %d)", sizes[0].Bytes, want)
	}
}

func TestDeclaredServeBudgetSilencesTheFinding(t *testing.T) {
	body := "## unit: draft\n\nGuidance.\n\n{{.report}}\n"
	budget := serveview.Default()
	reg := arithmeticRegistry(t)

	spec := arithmeticEntry(t, sizedMachine, body)
	spec.ServeBudget = 10000 // under the ~26KB worst case: still over
	if over := spec.OverBudget(budget, reg); len(over) != 1 {
		t.Fatalf("OverBudget = %+v, want the draft step named", over)
	}
	spec.ServeBudget = 40000 // the declared trade: silenced
	if over := spec.OverBudget(budget, reg); len(over) != 0 {
		t.Fatalf("OverBudget = %+v, want the declared total to silence it", over)
	}
}

func TestServeBudgetRoundTripsThroughTheEntry(t *testing.T) {
	content := "---\ntype: decision\nlayer: prc\nkind: procedure\ncanonical: sizedproc\nserveBudget: 40000\n" +
		sizedMachine + "\n---\n\n## unit: draft\n\nGuidance.\n"
	entry, err := model.ParseEntry("20260831-120001-d-prc-szb.md", content)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := engine.ParseSpec(entry)
	if err != nil {
		t.Fatal(err)
	}
	if spec.ServeBudget != 40000 {
		t.Errorf("spec.ServeBudget = %d, want the frontmatter value", spec.ServeBudget)
	}
	if !strings.Contains(model.FormatFrontmatter(entry), "serveBudget: 40000") {
		t.Error("FormatFrontmatter must round-trip serveBudget")
	}
}
