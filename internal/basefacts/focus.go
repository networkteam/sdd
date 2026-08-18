package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// FocusFactID is the stable identity of the focus-kind authoring fact. Its
// timestamp is a fixed authoring stamp, not a live clock.
const FocusFactID = "20260818-110300-s-prc-foc"

//go:embed templates/focus.md
var focusTemplate string

func focusFactContent() (string, error) {
	tmpl, err := template.New("focus").Option("missingkey=error").Parse(focusTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing focus fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: focusMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering focus fact template: %w", err)
	}
	return rendered.String(), nil
}

func focusMechanics() string {
	var b strings.Builder
	b.WriteString("## Mechanics\n\n")
	kinds := make([]string, 0, len(model.DecisionKindValues()))
	for _, k := range model.DecisionKindValues() {
		kinds = append(kinds, string(k))
	}
	fmt.Fprintf(&b, "Focus is a decision kind (the decision kinds: %s).\n\n", strings.Join(kinds, ", "))
	fmt.Fprintf(&b, "Enforced at capture: %s.\n", model.FocusInvolvementRule)
	return b.String()
}
