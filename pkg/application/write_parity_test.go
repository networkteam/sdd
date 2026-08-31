package application_test

import (
	"errors"
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
)

// Write-surface parity regressions: the engine path historically never ran
// the intent-on-directive and class-on-procedure rules the CLI enforced —
// both surfaces now validate through the construction boundary, so what
// blocks on one blocks on the other (kind-knowledge plan d-tac-9be).

func validationErrorMentions(t *testing.T, err error, field string) {
	t.Helper()
	var validationErr *sdd.ValidationError
	if !errors.As(err, &validationErr) {
		t.Fatalf("err = %v, want *ValidationError", err)
	}
	for _, w := range validationErr.Warnings {
		if w.Field == field {
			return
		}
	}
	t.Fatalf("no finding on field %q in %v", field, validationErr.Warnings)
}

func TestCreateEntry_RequiresIntentOnDirective(t *testing.T) {
	app, identity, binding, _ := newIdentityWriteApp(t)
	_, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "directive", Layer: "tactical", Confidence: "high",
		Body: "A directive drafted without intent.",
	})
	validationErrorMentions(t, err, "intent")
}

func TestCreateEntry_RejectsIntentOnNonDirective(t *testing.T) {
	app, identity, binding, _ := newIdentityWriteApp(t)
	_, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "gap", Layer: "tactical", Intent: "pending", Confidence: "high",
		Body: "A gap carrying a stray intent.",
	})
	validationErrorMentions(t, err, "intent")
}

func TestCreateEntry_RejectsClassOnNonProcedure(t *testing.T) {
	app, identity, binding, _ := newIdentityWriteApp(t)
	_, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "gap", Layer: "tactical", Class: "shell", Confidence: "high",
		Body: "A gap carrying a stray class.",
	})
	validationErrorMentions(t, err, "class")
}

func TestCreateEntry_ShellProcedureIsCapturable(t *testing.T) {
	app, identity, binding, dir := newIdentityWriteApp(t)
	result, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "procedure", Layer: "process", Confidence: "high",
		Canonical: "test-shell", Class: "shell",
		Body: "A shell procedure captured through the engine.",
	})
	if err != nil || result.EntryID == "" {
		t.Fatalf("CreateEntry = %+v, err %v", result, err)
	}
	e := loadEntryByID(t, dir, result.EntryID)
	if !e.IsShellProcedure() {
		t.Fatalf("persisted entry is not a shell procedure: kind=%s class=%s", e.Kind, e.Class)
	}
}

func TestCreateEntry_RejectsEmptyKind(t *testing.T) {
	app, identity, binding, _ := newIdentityWriteApp(t)
	_, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Layer: "tactical", Confidence: "high",
		Body: "A draft with no kind must not become a kindless signal.",
	})
	if err == nil || !strings.Contains(err.Error(), "kind is required") {
		t.Fatalf("err = %v, want kind-required error", err)
	}
}

func TestCreateEntry_ProcedureSpecRoundTrips(t *testing.T) {
	app, identity, binding, dir := newIdentityWriteApp(t)
	spec := map[string]any{
		"params": map[string]any{
			"goalHint": map[string]any{"type": "text", "optional": true, "desc": "what the caller wants examined"},
		},
		"state": map[string]any{
			"synthesis": map[string]any{"type": "text", "desc": "the outcome the run hands back"},
		},
		"steps": []any{
			map[string]any{
				"id":      "examine",
				"collect": []any{"synthesis"},
				"transitions": []any{
					map[string]any{"when": "hasSynthesis", "to": "end(completed)"},
				},
			},
		},
	}
	result, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "procedure", Layer: "process", Confidence: "high",
		Canonical: "test-move", ProcedureSpec: spec,
		Body: "A move captured with its workflow.\n\n## unit: examine\n\nExamine.",
	})
	if err != nil || result.EntryID == "" {
		t.Fatalf("CreateEntry = %+v, err %v", result, err)
	}
	e := loadEntryByID(t, dir, result.EntryID)
	if e.ProcedureSpec == nil || e.ProcedureSpec.Steps.IsZero() {
		t.Fatal("persisted entry lost its workflow frontmatter")
	}
}

func TestCreateEntry_RejectsSpecOnNonProcedure(t *testing.T) {
	app, identity, binding, _ := newIdentityWriteApp(t)
	_, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "gap", Layer: "tactical", Confidence: "high",
		ProcedureSpec: map[string]any{"steps": []any{map[string]any{"id": "x"}}},
		Body:          "A gap carrying a stray workflow declaration.",
	})
	validationErrorMentions(t, err, "procedureSpec")
}

func TestCreateEntry_RejectsUnknownSpecSection(t *testing.T) {
	app, identity, binding, _ := newIdentityWriteApp(t)
	_, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "procedure", Layer: "process", Confidence: "high",
		Canonical: "test-typo", ProcedureSpec: map[string]any{"stepps": []any{map[string]any{"id": "x"}}},
		Body: "A procedure whose workflow declares a typo'd section.",
	})
	validationErrorMentions(t, err, "procedureSpec")
}

func TestCreateEntry_RejectsSpecWithoutSteps(t *testing.T) {
	app, identity, binding, _ := newIdentityWriteApp(t)
	_, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "procedure", Layer: "process", Confidence: "high",
		Canonical: "test-stepless", ProcedureSpec: map[string]any{"state": map[string]any{"synthesis": map[string]any{"type": "text"}}},
		Body: "A procedure whose workflow declares no steps.",
	})
	validationErrorMentions(t, err, "procedureSpec")
}
