package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/model"
)

// AnnotationFactID is the stable identity of the annotation-kind authoring fact. Its
// timestamp is a fixed authoring stamp, not a live clock.
const AnnotationFactID = "20260818-110500-s-prc-ann"

//go:embed templates/annotation.md
var annotationTemplate string

func annotationFactContent() (string, error) {
	tmpl, err := template.New("annotation").Option("missingkey=error").Parse(annotationTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing annotation fact template: %w", err)
	}
	var rendered strings.Builder
	data := struct{ Mechanics string }{Mechanics: annotationMechanics()}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering annotation fact template: %w", err)
	}
	return rendered.String(), nil
}

func annotationMechanics() string {
	var b strings.Builder
	b.WriteString("## Mechanics\n\n")
	kinds := make([]string, 0, len(model.SignalKindValues()))
	for _, k := range model.SignalKindValues() {
		kinds = append(kinds, string(k))
	}
	fmt.Fprintf(&b, "Annotation is a signal kind (the signal kinds: %s).\n\n", strings.Join(kinds, ", "))
	fmt.Fprintf(&b, "Closing rule over signal kinds: %s.\n\n", model.SignalCloseRule)
	fmt.Fprintf(&b, "Enforced at capture: %s.\n\n", model.AnnotationRefsRequirement)
	fmt.Fprintf(&b, "Also enforced: %s.\n", model.AnnotationTopicRequirement)
	return b.String()
}
