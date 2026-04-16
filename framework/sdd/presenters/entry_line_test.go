package presenters_test

import (
	"bytes"
	"testing"

	"github.com/networkteam/resonance/framework/sdd/model"
	"github.com/networkteam/resonance/framework/sdd/presenters"
)

// Format contract (d-tac-955):
// `<id> <layer> <kind>? <type> [confidence: <conf>]? (<participants>) <summary>`
// Kind present only for decisions today; confidence on decisions + signals; participants always.

func TestEntryLine_DecisionPlan(t *testing.T) {
	e := entry("20260416-151058-d-tac-n6y",
		withKind(model.KindPlan),
		withConfidence("medium"),
		withParticipants("Christopher", "Claude"),
		withSummary("Add background sync awareness"))
	got := renderEntryLine(e)
	want := "  20260416-151058-d-tac-n6y tactical plan decision [confidence: medium] (Christopher, Claude) Add background sync awareness\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEntryLine_DecisionDirectiveDefault(t *testing.T) {
	// entry() helper defaults decisions to KindDirective when no withKind is passed.
	e := entry("20260416-151058-d-tac-xyz",
		withConfidence("high"),
		withParticipants("Christopher", "Claude"),
		withSummary("A directive decision"))
	got := renderEntryLine(e)
	want := "  20260416-151058-d-tac-xyz tactical directive decision [confidence: high] (Christopher, Claude) A directive decision\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEntryLine_SignalNoKind(t *testing.T) {
	e := entry("20260416-190732-s-prc-omw",
		withConfidence("high"),
		withParticipants("Christopher", "Claude"),
		withSummary("Participant names are inconsistent"))
	got := renderEntryLine(e)
	want := "  20260416-190732-s-prc-omw process signal [confidence: high] (Christopher, Claude) Participant names are inconsistent\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEntryLine_ActionNoKindNoConfidence(t *testing.T) {
	e := entry("20260407-144751-a-ops-gn0",
		withParticipants("Christopher", "Claude"),
		withSummary("Created CLAUDE.md"))
	got := renderEntryLine(e)
	want := "  20260407-144751-a-ops-gn0 operational action (Christopher, Claude) Created CLAUDE.md\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEntryLine_EmptyParticipants(t *testing.T) {
	// Empty participants still render `()` so the field is uniformly present.
	e := entry("20260407-144751-a-ops-gn0",
		withSummary("An action with no participants recorded"))
	got := renderEntryLine(e)
	want := "  20260407-144751-a-ops-gn0 operational action () An action with no participants recorded\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func renderEntryLine(e *model.Entry) string {
	var buf bytes.Buffer
	presenters.EntryLine(&buf, e)
	return buf.String()
}
