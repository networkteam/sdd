package engine_test

import (
	"testing"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
)

// TestExampleSpecLoads pins the served worked example to the engine's
// acceptance: the spec reference fact embeds ExampleSpecFrontmatter verbatim,
// so it must parse and validate exactly as an author's file would. The
// entryChains stub mirrors how base-procedure tests stand in for the
// shell-registered query.
func TestExampleSpecLoads(t *testing.T) {
	content := "---\n" +
		"type: decision\n" +
		"layer: prc\n" +
		"kind: procedure\n" +
		"confidence: medium\n" +
		"canonical: example-review\n" +
		"class: move\n" +
		engine.ExampleSpecFrontmatter +
		"---\n\n" +
		"A worked example move.\n\n" +
		"## unit: scope\n\nGather the ground truth.\n\n" +
		"## unit: account\n\nWrite the account.\n\n" +
		"## unit: review\n\nPut the account to the user.\n\n" +
		"## unit: record\n\nRecord the confirmed review.\n"

	entry, err := model.ParseEntry("20260814-110000-d-prc-exr.md", content)
	if err != nil {
		t.Fatalf("ParseEntry: %v", err)
	}

	reg := engine.NewRegistry()
	for _, name := range []string{"entryChains", "viewLayout"} {
		if err := reg.RegisterQuery(engine.Query{
			Doc: engine.FuncDoc{Name: name, Doc: "stub for the shell-registered query"},
			Fn: func(*engine.Context, map[string]any) (any, error) {
				return "", nil
			},
		}); err != nil {
			t.Fatal(err)
		}
	}

	if _, err := engine.LoadSpec(entry, reg); err != nil {
		t.Fatalf("LoadSpec: %v", err)
	}
}
