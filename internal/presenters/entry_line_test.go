package presenters_test

import (
	"bytes"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
)

// Format contract (d-tac-3yi, refined during implementation dialogue):
// `<id> <layer> <kind>? <type> [confidence: <conf>]? (<participants>) {status: <s>}? <summary>`
// Kind renders as a qualifier alongside layer/type (identity, not attribute).
// Square brackets = stored attrs (confidence); curly braces = derived attrs.

func TestEntryLine_DecisionPlan(t *testing.T) {
	e := entry("20260416-151058-d-tac-n6y",
		withKind(model.KindPlan),
		withConfidence("medium"),
		withParticipants("Christopher", "Claude"),
		withSummary("Add background sync awareness"))
	g := model.NewGraph([]*model.Entry{e})
	got := renderEntryLine(e, g)
	want := "  20260416-151058-d-tac-n6y tactical plan decision [confidence: medium] (Christopher, Claude) {status: active} Add background sync awareness\n"
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
	g := model.NewGraph([]*model.Entry{e})
	got := renderEntryLine(e, g)
	want := "  20260416-151058-d-tac-xyz tactical directive decision [confidence: high] (Christopher, Claude) {status: active} A directive decision\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEntryLine_SignalGapDefault(t *testing.T) {
	// entry() helper defaults signals to KindGap.
	e := entry("20260416-190732-s-prc-omw",
		withConfidence("high"),
		withParticipants("Christopher", "Claude"),
		withSummary("Participant names are inconsistent"))
	g := model.NewGraph([]*model.Entry{e})
	got := renderEntryLine(e, g)
	want := "  20260416-190732-s-prc-omw process gap signal [confidence: high] (Christopher, Claude) {status: open} Participant names are inconsistent\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEntryLine_EmptyParticipants(t *testing.T) {
	// Empty participants still render `()` so the field is uniformly present.
	e := entry("20260407-144751-s-ops-gn0",
		withSummary("A signal with no participants recorded"))
	g := model.NewGraph([]*model.Entry{e})
	got := renderEntryLine(e, g)
	want := "  20260407-144751-s-ops-gn0 operational gap signal () {status: open} A signal with no participants recorded\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEntryLine_ClosedSignal(t *testing.T) {
	sig := entry("20260416-190732-s-prc-omw",
		withConfidence("high"),
		withParticipants("Christopher", "Claude"),
		withSummary("A closed signal"))
	closer := entry("20260417-093934-s-prc-fxn",
		withKind(model.KindDone),
		withCloses(sig.ID),
		withParticipants("Christopher"),
		withSummary("The closing done signal"))
	g := model.NewGraph([]*model.Entry{sig, closer})
	got := renderEntryLine(sig, g)
	want := "  20260416-190732-s-prc-omw process gap signal [confidence: high] (Christopher, Claude) {status: closed-by 20260417-093934-s-prc-fxn} A closed signal\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func TestEntryLine_SupersededDecision(t *testing.T) {
	old := entry("20260410-100000-d-prc-old",
		withConfidence("medium"),
		withParticipants("Christopher"),
		withSummary("A superseded decision"))
	newer := entry("20260411-100000-d-prc-new",
		withSupersedes(old.ID),
		withParticipants("Christopher"),
		withSummary("The replacement"))
	g := model.NewGraph([]*model.Entry{old, newer})
	got := renderEntryLine(old, g)
	want := "  20260410-100000-d-prc-old process directive decision [confidence: medium] (Christopher) {status: superseded-by 20260411-100000-d-prc-new} A superseded decision\n"
	if got != want {
		t.Errorf("got:\n%q\nwant:\n%q", got, want)
	}
}

func renderEntryLine(e *model.Entry, g *model.Graph) string {
	var buf bytes.Buffer
	presenters.EntryLine(&buf, e, g)
	return buf.String()
}
