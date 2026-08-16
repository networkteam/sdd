package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// InsightFactID is the stable identity of the insight-kind authoring fact. Its
// timestamp is a fixed authoring stamp, not a live clock.
const InsightFactID = "20260816-100000-s-prc-syn"

//go:embed templates/insight.md
var insightTemplate string

func insightFactContent() (string, error) {
	tmpl, err := template.New("insight").Option("missingkey=error").Parse(insightTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing insight fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: insightMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering insight fact template: %w", err)
	}
	return rendered.String(), nil
}

func insightMechanics() string {
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
	fmt.Fprintf(&b, "Insight is a signal kind (the signal kinds: %s).\n\n", strings.Join(kinds, ", "))
	fmt.Fprintf(&b, "Insight is not an attention kind — those are %s — so an open insight never joins the surface where resolution is owed.\n\n", strings.Join(attention, ", "))
	fmt.Fprintf(&b, "Enforced at capture: %s.\n", model.SignalCloseRule)
	return b.String()
}
