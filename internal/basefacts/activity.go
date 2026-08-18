package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// ActivityFactID is the stable identity of the activity-kind authoring fact. Its
// timestamp is a fixed authoring stamp, not a live clock.
const ActivityFactID = "20260818-110200-s-prc-dsp"

//go:embed templates/activity.md
var activityTemplate string

func activityFactContent() (string, error) {
	tmpl, err := template.New("activity").Option("missingkey=error").Parse(activityTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing activity fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: activityMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering activity fact template: %w", err)
	}
	return rendered.String(), nil
}

func activityMechanics() string {
	var b strings.Builder
	b.WriteString("## Mechanics\n\n")
	kinds := make([]string, 0, len(model.DecisionKindValues()))
	for _, k := range model.DecisionKindValues() {
		kinds = append(kinds, string(k))
	}
	fmt.Fprintf(&b, "Activity is a decision kind (the decision kinds: %s).\n\n", strings.Join(kinds, ", "))
	b.WriteString("An activity carries no per-kind fields of its own; the common frontmatter and body carry it.\n\n")
	fmt.Fprintf(&b, "Closing rule over signal kinds: %s.\n", model.SignalCloseRule)
	return b.String()
}
