package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// DiscriminationFactID is the stable identity of the kind-discrimination fact —
// the tests that settle which kind to draft, held in one home rather than one
// view per kind (d-cpt-fpm). Its timestamp is a fixed authoring stamp.
const DiscriminationFactID = "20260818-120000-s-prc-dsc"

//go:embed templates/discrimination.md
var discriminationTemplate string

func discriminationFactContent() (string, error) {
	tmpl, err := template.New("discrimination").Option("missingkey=error").Parse(discriminationTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing discrimination fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: discriminationMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering discrimination fact template: %w", err)
	}
	return rendered.String(), nil
}

// discriminationMechanics renders the closed kind set from the running
// version's enumeration, so the prose's "all fourteen" cannot drift from the
// kinds that actually exist. The per-kind questions stay with the type-system
// introduction, which owns them.
func discriminationMechanics() string {
	names := func(kinds []model.Kind) string {
		out := make([]string, 0, len(kinds))
		for _, k := range kinds {
			out = append(out, string(k))
		}
		return strings.Join(out, ", ")
	}
	var b strings.Builder
	b.WriteString("## Mechanics\n\n")
	fmt.Fprintf(&b, "The kinds are a closed set. Signal kinds: %s.\n\n", names(model.SignalKindValues()))
	fmt.Fprintf(&b, "Decision kinds: %s.\n", names(model.DecisionKindValues()))
	return b.String()
}
