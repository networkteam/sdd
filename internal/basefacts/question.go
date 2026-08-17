package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// QuestionFactID is the stable identity of the question-kind authoring fact.
// Its timestamp is a fixed authoring stamp, not a live clock.
const QuestionFactID = "20260817-100000-s-prc-qry"

//go:embed templates/question.md
var questionTemplate string

func questionFactContent() (string, error) {
	tmpl, err := template.New("question").Option("missingkey=error").Parse(questionTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing question fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: questionMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering question fact template: %w", err)
	}
	return rendered.String(), nil
}

func questionMechanics() string {
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
	fmt.Fprintf(&b, "Question is a signal kind (the signal kinds: %s).\n\n", strings.Join(kinds, ", "))
	fmt.Fprintf(&b, "Question is an attention kind — those are %s — so while it stays open it sits on the surface where resolution is owed.\n\n", strings.Join(attention, ", "))
	fmt.Fprintf(&b, "Enforced at capture: %s.\n", model.SignalCloseRule)
	return b.String()
}
