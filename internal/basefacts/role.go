package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// RoleFactID is the stable identity of the role-kind authoring fact. Its
// timestamp is a fixed authoring stamp, not a live clock.
const RoleFactID = "20260818-110100-s-prc-rol"

//go:embed templates/role.md
var roleTemplate string

func roleFactContent() (string, error) {
	tmpl, err := template.New("role").Option("missingkey=error").Parse(roleTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing role fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: roleMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering role fact template: %w", err)
	}
	return rendered.String(), nil
}

func roleMechanics() string {
	var b strings.Builder
	b.WriteString("## Mechanics\n\n")
	kinds := make([]string, 0, len(model.DecisionKindValues()))
	for _, k := range model.DecisionKindValues() {
		kinds = append(kinds, string(k))
	}
	fmt.Fprintf(&b, "Role is a decision kind (the decision kinds: %s).\n\n", strings.Join(kinds, ", "))
	pinned := make([]string, 0, len(model.ProcessPinnedKinds()))
	for _, k := range model.ProcessPinnedKinds() {
		pinned = append(pinned, string(k))
	}
	fmt.Fprintf(&b, "Layer is pinned to process for these kinds: %s.\n\n", strings.Join(pinned, ", "))
	fmt.Fprintf(&b, "Enforced at capture: %s.\n", model.RoleActorRequirement)
	return b.String()
}
