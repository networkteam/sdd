package engine_test

import (
	"testing"

	"github.com/networkteam/sdd/internal/engine"
)

func sizeOf(t *testing.T, sv *engine.Serve, part string) int {
	t.Helper()
	for _, s := range sv.Sizes {
		if s.Part == part {
			return s.Bytes
		}
	}
	t.Fatalf("serve has no size for part %q, sizes: %+v", part, sv.Sizes)
	return 0
}

func TestServeSizesAccountInjectsLanesAndSchema(t *testing.T) {
	reg := engine.NewRegistry()
	if err := reg.RegisterQuery(engine.Query{
		Doc: engine.FuncDoc{Name: "echoQuery", Doc: "test echo"},
		Fn:  func(_ *engine.Context, _ map[string]any) (any, error) { return "injected-result", nil },
	}); err != nil {
		t.Fatal(err)
	}
	body := "## unit: draft\n\n### lane: intro\n\nDraft the note.\n\n### lane: injected\n\nResult: {{.myId}}\n"
	sv := startServe(t, reg, injectMachine("          - {id: myId, fn: echoQuery}"), body)

	if got := sizeOf(t, sv, "inject:myId"); got != len("injected-result") {
		t.Errorf("inject size = %d, want %d", got, len("injected-result"))
	}
	if got := sizeOf(t, sv, "lane:intro"); got != len("Draft the note.") {
		t.Errorf("intro lane size = %d, want %d", got, len("Draft the note."))
	}
	if got := sizeOf(t, sv, "lane:injected"); got != len("Result: injected-result") {
		t.Errorf("injected lane size = %d, want %d", got, len("Result: injected-result"))
	}
	if got := sizeOf(t, sv, "schema"); got <= 2 {
		t.Errorf("schema size = %d, want the marshaled report schema's weight", got)
	}
	if got := sizeOf(t, sv, "diagnostics"); got == 0 {
		t.Errorf("diagnostics size = %d, want the held gate's missing line accounted", got)
	}
}

func TestServeSizesAccountNonStringInjectByEncoding(t *testing.T) {
	reg := engine.NewRegistry()
	rows := []map[string]any{{"id": "a", "title": "one"}, {"id": "b", "title": "two"}}
	if err := reg.RegisterQuery(engine.Query{
		Doc: engine.FuncDoc{Name: "rowsQuery", Doc: "test rows"},
		Fn:  func(_ *engine.Context, _ map[string]any) (any, error) { return rows, nil },
	}); err != nil {
		t.Fatal(err)
	}
	body := "## unit: draft\n\n### lane: rows\n\n{{range .rows}}- {{.id}}\n{{end}}\n"
	sv := startServe(t, reg, injectMachine("          - {id: rows, fn: rowsQuery}"), body)

	if got := sizeOf(t, sv, "inject:rows"); got < 20 {
		t.Errorf("rows inject size = %d, want its JSON encoding's weight", got)
	}
}
