package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// AspirationFactID is the stable identity of the aspiration-kind authoring fact. Its
// timestamp is a fixed authoring stamp, not a live clock.
const AspirationFactID = "20260818-110400-s-prc-asp"

//go:embed templates/aspiration.md
var aspirationTemplate string

func aspirationFactContent() (string, error) {
	tmpl, err := template.New("aspiration").Option("missingkey=error").Parse(aspirationTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing aspiration fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: aspirationMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering aspiration fact template: %w", err)
	}
	return rendered.String(), nil
}

func aspirationMechanics() string {
	var b strings.Builder
	b.WriteString("## Mechanics\n\n")
	kinds := make([]string, 0, len(model.DecisionKindValues()))
	for _, k := range model.DecisionKindValues() {
		kinds = append(kinds, string(k))
	}
	fmt.Fprintf(&b, "Aspiration is a decision kind (the decision kinds: %s).\n\n", strings.Join(kinds, ", "))
	b.WriteString("An aspiration carries no per-kind fields of its own; the common frontmatter and body carry it.\n")
	return b.String()
}
