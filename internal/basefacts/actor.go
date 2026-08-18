package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// ActorFactID is the stable identity of the actor-kind authoring fact. Its
// timestamp is a fixed authoring stamp, not a live clock.
const ActorFactID = "20260818-110000-s-prc-act"

//go:embed templates/actor.md
var actorTemplate string

func actorFactContent() (string, error) {
	tmpl, err := template.New("actor").Option("missingkey=error").Parse(actorTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing actor fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: actorMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering actor fact template: %w", err)
	}
	return rendered.String(), nil
}

func actorMechanics() string {
	var b strings.Builder
	b.WriteString("## Mechanics\n\n")
	kinds := make([]string, 0, len(model.SignalKindValues()))
	for _, k := range model.SignalKindValues() {
		kinds = append(kinds, string(k))
	}
	fmt.Fprintf(&b, "Actor is a signal kind (the signal kinds: %s).\n\n", strings.Join(kinds, ", "))
	pinned := make([]string, 0, len(model.ProcessPinnedKinds()))
	for _, k := range model.ProcessPinnedKinds() {
		pinned = append(pinned, string(k))
	}
	fmt.Fprintf(&b, "Layer is pinned to process for these kinds: %s.\n\n", strings.Join(pinned, ", "))
	fmt.Fprintf(&b, "Enforced at capture: %s. Alias hygiene: %s.\n\n", model.ActorCanonicalRequirement, model.AliasHygieneRule)
	fmt.Fprintf(&b, "Closing rule over signal kinds: %s.\n", model.SignalCloseRule)
	return b.String()
}
