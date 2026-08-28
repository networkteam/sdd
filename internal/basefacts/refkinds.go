package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// RefKindsFactID is the stable identity of the ref-kind vocabulary fact — how
// entries connect: each kind's meaning, direction, and when it applies. Its
// timestamp is a fixed authoring stamp.
const RefKindsFactID = "20260828-160000-s-prc-rfk"

//go:embed templates/refkinds.md
var refKindsTemplate string

func refKindsFactContent() (string, error) {
	tmpl, err := template.New("refkinds").Option("missingkey=error").Parse(refKindsTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing refkinds fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: refKindsMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering refkinds fact template: %w", err)
	}
	return rendered.String(), nil
}

// refKindsMechanics renders the closed kind set from the running version's
// enumeration, so the fact cannot drift from the kinds capture accepts.
func refKindsMechanics() string {
	kinds := model.RefKindValues()
	names := make([]string, 0, len(kinds))
	for _, k := range kinds {
		names = append(names, string(k))
	}
	var b strings.Builder
	b.WriteString("## Mechanics\n\n")
	fmt.Fprintf(&b, "The ref kinds are a closed set: %s.\n\n", strings.Join(names, ", "))
	b.WriteString("Enforced at capture: every reference carries one of these kinds, and every target must resolve or capture is blocked. Legacy values on disk (grounds, evidence) read as grounded-in and are not capturable.\n")
	return b.String()
}
