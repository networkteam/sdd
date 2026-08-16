package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// GapFactID is the stable identity of the gap-kind authoring fact. Its
// timestamp is a fixed authoring stamp, not a live clock.
const GapFactID = "20260815-100000-s-prc-gpk"

//go:embed templates/gap.md
var gapTemplate string

// gapFactContent renders the gap fact from its embedded template. The
// mechanics block derives from the model declarations that enforce the rules,
// so the served words track the write path with no manual sync.
func gapFactContent() (string, error) {
	tmpl, err := template.New("gap").Option("missingkey=error").Parse(gapTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing gap fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: gapMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering gap fact template: %w", err)
	}
	return rendered.String(), nil
}

func gapMechanics() string {
	kinds := make([]string, 0, len(model.SignalKindValues()))
	for _, k := range model.SignalKindValues() {
		kinds = append(kinds, string(k))
	}
	attention := make([]string, 0, len(model.OpenAttentionKinds()))
	for _, k := range model.OpenAttentionKinds() {
		attention = append(attention, string(k))
	}
	var b strings.Builder
	b.WriteString("## Mechanics\n\n")
	fmt.Fprintf(&b, "Gap is a signal kind (the signal kinds: %s).\n\n", strings.Join(kinds, ", "))
	fmt.Fprintf(&b, "Attention kinds — open entries of these kinds are the graph's open-signal surface: %s.\n\n", strings.Join(attention, ", "))
	fmt.Fprintf(&b, "Enforced at capture: %s.\n", model.SignalCloseRule)
	return b.String()
}
