package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// OverviewFactID is the stable identity of the type-system overview fact — the
// indexed introduction to types, kinds, and layers, pointing at each kind's
// authoring fact. Its timestamp is a fixed authoring stamp, not a live clock.
const OverviewFactID = "20260812-180000-s-prc-typ"

// overviewTemplate is the whole-entry template for the overview fact. The kind
// lists render from the model's enumeration at named placeholders, so a new
// kind appears in the served fact the moment it is declared — and fails the
// build until its question is written.
//
//go:embed templates/overview.md
var overviewTemplate string

// kindQuestions is the authored question each kind answers — the
// discrimination cue the generated kind lists pair with the enumeration.
// Prose lives with the declaration it describes; completeness is enforced at
// render, mirroring viewlayout's missing-metadata failure.
var kindQuestions = map[model.Kind]string{
	model.KindGap:        "What needs attention?",
	model.KindFact:       "What do we know?",
	model.KindQuestion:   "What do we not know?",
	model.KindInsight:    "What have we synthesized?",
	model.KindDone:       "What was accomplished?",
	model.KindActor:      "Who is participating?",
	model.KindAnnotation: "What structure lies over other entries?",

	model.KindDirective:  "Which way do we go?",
	model.KindActivity:   "What's next to do?",
	model.KindPlan:       "What must be true when done?",
	model.KindContract:   "What must always hold? — takes no new captures; standing constraints are guiding directives",
	model.KindAspiration: "What are we pulling toward?",
	model.KindRole:       "How does an actor participate here?",
	model.KindFocus:      "What are we attending to in this period, and who is engaged?",
	model.KindProcedure:  "How does a playbook move run?",
}

// overviewFactContent renders the overview fact from its embedded template,
// generating both kind lists from the model enumeration.
func overviewFactContent() (string, error) {
	signalKinds, err := kindList("**Signal kinds** — a signal records something noticed:", model.SignalKindValues())
	if err != nil {
		return "", err
	}
	decisionKinds, err := kindList("**Decision kinds** — a decision records something committed to:", model.DecisionKindValues())
	if err != nil {
		return "", err
	}

	tmpl, err := template.New("overview").Option("missingkey=error").Parse(overviewTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing overview fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ SignalKinds, DecisionKinds string }{signalKinds, decisionKinds}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering overview fact template: %w", err)
	}
	return rendered.String(), nil
}

func kindList(heading string, kinds []model.Kind) (string, error) {
	var b strings.Builder
	b.WriteString(heading + "\n")
	for _, k := range kinds {
		question, ok := kindQuestions[k]
		if !ok || question == "" {
			return "", fmt.Errorf("kind %q has no question in the overview fact — write one in kindQuestions", k)
		}
		fmt.Fprintf(&b, "\n- `%s` — %s", k, question)
	}
	return b.String(), nil
}
