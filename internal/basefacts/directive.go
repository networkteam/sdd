package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// DirectiveFactID is the stable identity of the directive-kind authoring
// fact. Its timestamp is a fixed authoring stamp, not a live clock.
const DirectiveFactID = "20260815-110000-s-prc-drk"

//go:embed templates/directive.md
var directiveTemplate string

// directiveFactContent renders the directive fact from its embedded template.
// The mechanics block derives from the model declarations that enforce the
// rules, so the served words track the write path with no manual sync.
func directiveFactContent() (string, error) {
	tmpl, err := template.New("directive").Option("missingkey=error").Parse(directiveTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing directive fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: directiveMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering directive fact template: %w", err)
	}
	return rendered.String(), nil
}

func directiveMechanics() string {
	kinds := make([]string, 0, len(model.DecisionKindValues()))
	for _, k := range model.DecisionKindValues() {
		kinds = append(kinds, string(k))
	}
	intents := []string{string(model.IntentPending), string(model.IntentGuiding), string(model.IntentSettled)}
	var b strings.Builder
	b.WriteString("## Mechanics\n\n")
	fmt.Fprintf(&b, "Directive is a decision kind (the decision kinds: %s).\n\n", strings.Join(kinds, ", "))
	fmt.Fprintf(&b, "Intent values: %s.\n\n", strings.Join(intents, ", "))
	fmt.Fprintf(&b, "Enforced at capture: %s.\n\n", model.DirectiveIntentRequirement)
	fmt.Fprintf(&b, "Enforced on lifecycle edges: %s.\n", model.SettledCloseRule)
	return b.String()
}
