package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// DoneFactID is the stable identity of the done-kind authoring fact. Its
// timestamp is a fixed authoring stamp, not a live clock; a project overrides
// the fact by superseding this ID.
const DoneFactID = "20260812-170000-s-prc-dnk"

// doneTemplate is the whole-entry template for the done authoring fact:
// frontmatter and hand-written prose live in the reviewable .md file, and
// generated content arrives at named placeholders so regeneration can never
// disturb the prose.
//
//go:embed templates/done.md
var doneTemplate string

// doneFactContent renders the done fact from its embedded template. The
// mechanics block derives from the model declarations that enforce the rules,
// so the served words track the write path with no manual sync.
func doneFactContent() (string, error) {
	tmpl, err := template.New("done").Option("missingkey=error").Parse(doneTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing done fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: doneMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering done fact template: %w", err)
	}
	return rendered.String(), nil
}

func doneMechanics() string {
	kinds := make([]string, 0, len(model.SignalKindValues()))
	for _, k := range model.SignalKindValues() {
		kinds = append(kinds, string(k))
	}
	var b strings.Builder
	b.WriteString("## Mechanics\n\n")
	fmt.Fprintf(&b, "Done is a signal kind (the signal kinds: %s).\n\n", strings.Join(kinds, ", "))
	fmt.Fprintf(&b, "Enforced at capture: %s.\n", model.DoneAnchorRequirement)
	return b.String()
}
