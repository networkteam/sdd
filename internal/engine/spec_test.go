package engine

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

// specFixture builds a procedure entry around the given params/state/steps
// YAML fragments and body.
func specFixture(t *testing.T, machine, body string) *model.Entry {
	t.Helper()
	content := "---\ntype: decision\nlayer: prc\nkind: procedure\ncanonical: testproc\n" +
		machine + "\n---\n\n" + body
	entry, err := model.ParseEntry("20260702-121500-d-prc-spc.md", content)
	if err != nil {
		t.Fatalf("fixture entry: %v", err)
	}
	return entry
}

const minimalSteps = `steps:
    - id: only
      collect: [note]
      transitions:
          - when: hasBody
            to: end(completed)
`

func TestParseSpec_Problems(t *testing.T) {
	tests := []struct {
		name    string
		machine string
		body    string
		wantErr string
	}{
		{
			name: "unknown domain type",
			machine: `state:
    note: {type: paragraph, desc: x}
` + minimalSteps,
			wantErr: `state.note: unknown domain type "paragraph"`,
		},
		{
			name: "state param collision",
			machine: `params:
    note: {type: text, desc: x}
state:
    note: {type: text, desc: x}
` + minimalSteps,
			wantErr: "state.note: collides with a param",
		},
		{
			name: "unknown transition target",
			machine: `state:
    note: {type: text, desc: x}
steps:
    - id: only
      transitions:
          - when: hasBody
            to: nowhere
`,
			wantErr: `transition target "nowhere"`,
		},
		{
			name: "undeclared collect field",
			machine: `state:
    note: {type: text, desc: x}
steps:
    - id: only
      collect: [ghost]
      transitions:
          - when: hasBody
            to: end(completed)
`,
			wantErr: `collect names "ghost"`,
		},
		{
			name: "guard syntax error",
			machine: `state:
    note: {type: text, desc: x}
steps:
    - id: only
      guard: hasBody and
      transitions:
          - when: hasBody
            to: end(completed)
      op: newEntry
`,
			wantErr: "guard",
		},
		{
			name: "chooser without options",
			machine: `state:
    note: {type: text, desc: x}
steps:
    - id: only
      chooser: user
`,
			wantErr: "chooser needs options",
		},
		{
			name: "gate with options",
			machine: `state:
    note: {type: text, desc: x}
steps:
    - id: only
      options:
          - {choice: yes, to: end(completed)}
      transitions:
          - when: hasBody
            to: end(completed)
`,
			wantErr: "options belong to agent/user chooser steps",
		},
		{
			name: "chooser with op",
			machine: `state:
    note: {type: text, desc: x}
steps:
    - id: only
      chooser: user
      op: newEntry
      options:
          - {choice: go, to: end(completed)}
`,
			wantErr: "op runs on gate steps only",
		},
		{
			name: "duplicate step id",
			machine: `state:
    note: {type: text, desc: x}
steps:
    - id: only
      transitions:
          - when: hasBody
            to: end(completed)
    - id: only
      transitions:
          - when: hasBody
            to: end(completed)
`,
			wantErr: "duplicate step id",
		},
		{
			name: "render names missing unit",
			machine: `state:
    note: {type: text, desc: x}
steps:
    - id: only
      render: ghost
      transitions:
          - when: hasBody
            to: end(completed)
`,
			wantErr: `render names unit "ghost"`,
		},
		{
			name: "double otherwise",
			machine: `state:
    note: {type: text, desc: x}
steps:
    - id: only
      transitions:
          - otherwise: end(completed)
          - otherwise: end(abandoned)
`,
			wantErr: "more than one otherwise",
		},
		{
			name: "unknown step key rejected",
			machine: `state:
    note: {type: text, desc: x}
steps:
    - id: only
      colect: [note]
      transitions:
          - when: hasBody
            to: end(completed)
`,
			wantErr: "field colect not found",
		},
		{
			name: "default on a param is rejected",
			machine: `params:
    flag: {type: bool, default: true, desc: x}
state:
    note: {type: text, desc: x}
` + minimalSteps,
			wantErr: "params.flag: default is not supported on params",
		},
		{
			name:    "no machine frontmatter",
			machine: "",
			wantErr: "no params/state/steps frontmatter",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			entry := specFixture(t, tt.machine, tt.body)
			_, err := ParseSpec(entry)
			if err == nil || !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("ParseSpec err = %v, want containing %q", err, tt.wantErr)
			}
		})
	}
}

func TestSpecValidate_RegistryChecks(t *testing.T) {
	reg := NewRegistry()
	mustRegisterCommand(reg, Command{
		Doc: FuncDoc{Name: "writeThing", Doc: "test", Writes: []string{"thingId", "note"}},
		Fn:  func(_ *Context) error { return nil },
	})

	entry := specFixture(t, `state:
    note: {type: text, desc: x}
steps:
    - id: gateStep
      guard: hasNothing and hasBody
      op: writeThing
      inject:
          - {fn: missingQuery}
      transitions:
          - when: alsoMissing
            to: chooseStep
    - id: chooseStep
      chooser: user
      options:
          - {choice: go, call: ghostCommand, to: end(completed)}
`, "")
	spec, err := ParseSpec(entry)
	if err != nil {
		t.Fatal(err)
	}
	problems := strings.Join(spec.Validate(reg), "\n")

	for _, want := range []string{
		`unknown predicate "hasNothing"`,
		`unknown predicate "alsoMissing"`,
		`inject fn "missingQuery" is not a registered query`,
		`"ghostCommand" is not a registered command`,
		// writeThing declares a write into declared state — the engine-
		// written surface must stay disjoint from report-writable state.
		`command "writeThing" writes "note", which collides with declared state`,
	} {
		if !strings.Contains(problems, want) {
			t.Errorf("Validate problems missing %q; got:\n%s", want, problems)
		}
	}
}

func TestParseSpec_UnitsAndOptionalCollect(t *testing.T) {
	entry := specFixture(t, `state:
    note: {type: text, desc: x}
    extra: {type: text, optional: true, desc: y}
steps:
    - id: draft
      collect: [note, "extra?"]
      transitions:
          - when: hasBody
            to: end(completed)
`, `Intro prose (not a unit).

## unit: draft

Do the draft.

## unit: spare

Spare unit text.

## Not a unit

Regular heading.
`)
	spec, err := ParseSpec(entry)
	if err != nil {
		t.Fatal(err)
	}
	if len(spec.Units) != 2 {
		t.Fatalf("units = %v, want draft and spare", spec.Units)
	}
	if !strings.Contains(spec.Units["draft"], "Do the draft.") {
		t.Errorf("draft unit = %q", spec.Units["draft"])
	}
	if strings.Contains(spec.Units["spare"], "Not a unit") {
		t.Errorf("unit must end at the next level-2 heading, got %q", spec.Units["spare"])
	}
	step := spec.StepByID["draft"]
	if len(step.Collect) != 2 || step.Collect[0].Optional || !step.Collect[1].Optional {
		t.Errorf("collect optional markers wrong: %+v", step.Collect)
	}
}
