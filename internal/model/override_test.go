package model_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func closedFact(t *testing.T, id string) *model.Entry {
	t.Helper()
	entry := &model.Entry{
		ID: id, Type: model.TypeSignal, Layer: model.LayerProcess, Kind: model.KindFact,
		Override: model.OverrideClosed,
		Content:  "A generated reference fact.",
	}
	return entry
}

// TestSupersedeOverrideClosedRefused pins the type-system-fact guard
// (d-tac-9be): a supersede targeting an override-closed fact blocks at the
// shared write boundary, recognized by the target's declared property.
func TestSupersedeOverrideClosedRefused(t *testing.T) {
	target := closedFact(t, "20260812-170000-s-prc-fct")
	graph := model.NewGraph([]*model.Entry{target})

	c := &model.EntryConstruction{
		ID: "20260814-120000-s-prc-new", Type: model.TypeSignal, Layer: model.LayerProcess, Kind: model.KindFact,
		Supersedes: []string{target.ID},
		Body:       "A project copy trying to replace the generated fact.",
	}
	_, findings := c.ValidateForWrite(graph)
	found := false
	for _, f := range findings {
		if f.Field == "supersedes" && strings.Contains(f.Message, "override: closed") {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected an override-closed supersede finding, got %+v", findings)
	}
}

func TestSupersedeOpenFactAllowed(t *testing.T) {
	target := closedFact(t, "20260812-170000-s-prc-opn")
	target.Override = ""
	graph := model.NewGraph([]*model.Entry{target})

	c := &model.EntryConstruction{
		ID: "20260814-120000-s-prc-new", Type: model.TypeSignal, Layer: model.LayerProcess, Kind: model.KindFact,
		Supersedes: []string{target.ID},
		Body:       "A project override of an ordinary base fact.",
	}
	_, findings := c.ValidateForWrite(graph)
	for _, f := range findings {
		if strings.Contains(f.Message, "override") {
			t.Fatalf("unexpected override finding on an open fact: %+v", f)
		}
	}
}

func TestOverrideOnlyValidOnFact(t *testing.T) {
	entry := &model.Entry{
		ID: "20260814-120000-s-tac-gap", Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindGap,
		Override: model.OverrideClosed,
		Content:  "A gap carrying a fact-only field.",
	}
	c, findings := model.ConstructFromEntry(entry)
	if len(findings) != 1 || findings[0].Field != "override" {
		t.Fatalf("expected one stray-override finding, got %+v", findings)
	}
	if c.Fact != nil {
		t.Errorf("stray override must not populate the fact block: %+v", c.Fact)
	}
}

func TestOverrideValueChecked(t *testing.T) {
	c := &model.EntryConstruction{
		ID: "20260814-120000-s-prc-bad", Type: model.TypeSignal, Layer: model.LayerProcess, Kind: model.KindFact,
		Fact: &model.FactFields{Override: "sealed"},
		Body: "A fact with an unknown override value.",
	}
	findings := c.Validate(nil)
	if len(findings) != 1 || findings[0].Field != "override" {
		t.Fatalf("expected one override-value finding, got %+v", findings)
	}
}

func TestOverrideRoundTrip(t *testing.T) {
	entry := closedFact(t, "20260812-170000-s-prc-rtr")
	fm := model.FormatFrontmatter(entry)
	if !strings.Contains(fm, "override: closed") {
		t.Fatalf("frontmatter misses override: closed:\n%s", fm)
	}
	parsed, err := model.ParseEntry(entry.ID, fm+"\n"+entry.Content)
	if err != nil {
		t.Fatalf("ParseEntry: %v", err)
	}
	if parsed.Override != model.OverrideClosed {
		t.Errorf("Override = %q after round trip, want %q", parsed.Override, model.OverrideClosed)
	}
}
