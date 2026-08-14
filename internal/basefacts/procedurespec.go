package basefacts

import (
	_ "embed"
	"fmt"
	"strings"
	"text/template"

	"github.com/networkteam/sdd/internal/engine"
)

// ProcedureSpecFactID is the stable identity of the procedure spec reference
// fact — how to write the frontmatter the engine executes. Unindexed, like
// the per-kind authoring facts: it is reached through the procedure kind's
// fact and the capture lane's teasers, not the pull-side index.
const ProcedureSpecFactID = "20260814-100000-s-prc-psr"

//go:embed templates/procedurespec.md
var procedureSpecTemplate string

// procedureSpecFactContent renders the spec reference from its embedded
// template. The variable types and end targets derive from the engine
// declarations that enforce them; the live ability inventory is deliberately
// not baked in — the fact teaches pulling it from the function registry,
// which serves the running version's truth.
func procedureSpecFactContent() (string, error) {
	tmpl, err := template.New("procedurespec").Option("missingkey=error").Parse(procedureSpecTemplate)
	if err != nil {
		return "", fmt.Errorf("parsing procedure spec fact template: %w", err)
	}
	types := make([]string, 0, len(engine.BaseTypeValues()))
	for _, t := range engine.BaseTypeValues() {
		types = append(types, string(t))
	}
	var rendered strings.Builder
	data := struct{ VarTypes, EndTargets string }{
		VarTypes:   strings.Join(types, ", "),
		EndTargets: fmt.Sprintf("`%s` or `%s`", engine.EndCompleted, engine.EndAbandoned),
	}
	if err := tmpl.Execute(&rendered, data); err != nil {
		return "", fmt.Errorf("rendering procedure spec fact template: %w", err)
	}
	return rendered.String(), nil
}
