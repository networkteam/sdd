package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// PlanFactID is the stable identity of the plan-kind authoring fact. Its
// timestamp is a fixed authoring stamp, not a live clock.
const PlanFactID = "20260818-100000-s-prc-spc"

//go:embed templates/plan.md
var planTemplate string

func planFactContent() (string, error) {
	tmpl, err := template.New("plan").Option("missingkey=error").Parse(planTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing plan fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: planMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering plan fact template: %w", err)
	}
	return rendered.String(), nil
}

func planMechanics() string {
	kinds := make([]string, 0, len(model.DecisionKindValues()))
	for _, k := range model.DecisionKindValues() {
		kinds = append(kinds, string(k))
	}
	var b strings.Builder
	b.WriteString("## Mechanics\n\n")
	fmt.Fprintf(&b, "Plan is a decision kind (the decision kinds: %s).\n\n", strings.Join(kinds, ", "))
	fmt.Fprintf(&b, "Structural requirement, enforced at capture: %s.\n", model.PlanAcceptanceRequirement)
	return b.String()
}
