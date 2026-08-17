package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// FactFactID is the stable identity of the fact-kind authoring fact. Its
// timestamp is a fixed authoring stamp, not a live clock.
const FactFactID = "20260816-110000-s-prc-kno"

//go:embed templates/fact.md
var factTemplate string

func factFactContent() (string, error) {
	tmpl, err := template.New("fact").Option("missingkey=error").Parse(factTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing fact fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: factMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering fact fact template: %w", err)
	}
	return rendered.String(), nil
}

func factMechanics() string {
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
	fmt.Fprintf(&b, "Fact is a signal kind (the signal kinds: %s).\n\n", strings.Join(kinds, ", "))
	fmt.Fprintf(&b, "Fact is not an attention kind — those are %s — so an open fact never joins the surface where resolution is owed.\n\n", strings.Join(attention, ", "))
	fmt.Fprintf(&b, "Index enrollment: %s, and %s.\n\n", model.FactIndexKindRule, model.FactIndexTopicRule)
	fmt.Fprintf(&b, "Enforced at capture: %s.\n", model.SignalCloseRule)
	return b.String()
}
