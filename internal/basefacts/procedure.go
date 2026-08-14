package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// ProcedureFactID is the stable identity of the procedure-kind authoring
// fact. Its timestamp is a fixed authoring stamp, not a live clock; a project
// overrides the fact by superseding this ID.
const ProcedureFactID = "20260813-170000-s-prc-prd"

//go:embed templates/procedure.md
var procedureTemplate string

// procedureFactContent renders the procedure fact from its embedded template.
// The generated blocks derive from the declarations that enforce them — the
// model's kind and class enumerations, the engine's domain-type and chooser
// vocabularies — so the served words track the code with no manual sync.
func procedureFactContent() (string, error) {
	tmpl, err := template.New("procedure").Option("missingkey=error").Parse(procedureTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing procedure fact template: %w", err)
	}
	classes, err := procedureClasses()
	if err != nil {
		return "", err
	}
	var rendered strings.Builder
	data := struct{ Classes, Mechanics string }{Classes: classes, Mechanics: procedureMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering procedure fact template: %w", err)
	}
	return rendered.String(), nil
}

// procedureClasses renders the class enumeration with its declared meanings;
// a class shipping without a description fails the render, and with it the
// build.
func procedureClasses() (string, error) {
	var b strings.Builder
	for i, c := range model.ProcedureClassValues() {
		desc := c.Description()
		if desc == "" {
			return "", fmt.Errorf("procedure class %q has no description in the model — write one beside the declaration", c)
		}
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(&b, "- `%s` — %s", c, desc)
	}
	return b.String(), nil
}

func procedureMechanics() string {
	kinds := make([]string, 0, len(model.DecisionKindValues()))
	for _, k := range model.DecisionKindValues() {
		kinds = append(kinds, string(k))
	}
	classes := make([]string, 0, len(model.ProcedureClassValues()))
	for _, c := range model.ProcedureClassValues() {
		classes = append(classes, string(c))
	}
	var b strings.Builder
	b.WriteString("## Mechanics\n\n")
	fmt.Fprintf(&b, "Procedure is a decision kind (the decision kinds: %s), pinned to the process layer.\n\n", strings.Join(kinds, ", "))
	fmt.Fprintf(&b, "Enforced at capture: a required `canonical`; `class`, when set, one of %s (empty means %s).\n\n", strings.Join(classes, ", "), model.ProcedureClassMove)
	fmt.Fprintf(&b, "How to write the spec — the fields, the variable types, the step shapes, a skeleton, and pulling the live ability registry — is the spec reference fact `%s`; reach for it the moment you move from choosing the kind to writing the workflow.\n", ProcedureSpecFactID)
	return b.String()
}
