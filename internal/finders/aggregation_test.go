package finders

import (
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestGroupByKind(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	b := entry("20260101-110000-d-tac-bbb", withKind(model.KindPlan))
	c := entry("20260101-120000-d-tac-ccc", withKind(model.KindDirective))
	d := entry("20260101-130000-s-tac-ddd", withKind(model.KindGap))

	groups, err := groupBy([]*model.Entry{a, b, c, d}, "kind")
	if err != nil {
		t.Fatalf("groupBy: %v", err)
	}

	// Alphabetical group order: directive, gap, plan.
	wantOrder := []string{"directive", "gap", "plan"}
	gotOrder := groupKeys(groups)
	if !equalStrings(gotOrder, wantOrder) {
		t.Errorf("group order: got %v, want %v", gotOrder, wantOrder)
	}

	// Within-group order preserves input order — no per-group ranking in slice 5.
	directive := findGroup(groups, "directive")
	if got, want := idsOf(directive.Entries), []string{a.ID, c.ID}; !equalIDs(got, want) {
		t.Errorf("directive group entries: got %v, want %v", got, want)
	}

	plan := findGroup(groups, "plan")
	if got, want := idsOf(plan.Entries), []string{b.ID}; !equalIDs(got, want) {
		t.Errorf("plan group entries: got %v, want %v", got, want)
	}
}

func TestGroupByLayer(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	b := entry("20260101-110000-d-stg-bbb", withKind(model.KindAspiration))
	c := entry("20260101-120000-d-tac-ccc", withKind(model.KindPlan))

	groups, err := groupBy([]*model.Entry{a, b, c}, "layer")
	if err != nil {
		t.Fatalf("groupBy: %v", err)
	}

	wantOrder := []string{"strategic", "tactical"}
	gotOrder := groupKeys(groups)
	if !equalStrings(gotOrder, wantOrder) {
		t.Errorf("group order: got %v, want %v", gotOrder, wantOrder)
	}
}

func TestGroupByType(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	b := entry("20260101-110000-s-tac-bbb", withKind(model.KindGap))
	c := entry("20260101-120000-d-tac-ccc", withKind(model.KindPlan))

	groups, err := groupBy([]*model.Entry{a, b, c}, "type")
	if err != nil {
		t.Fatalf("groupBy: %v", err)
	}

	wantOrder := []string{"decision", "signal"}
	gotOrder := groupKeys(groups)
	if !equalStrings(gotOrder, wantOrder) {
		t.Errorf("group order: got %v, want %v", gotOrder, wantOrder)
	}
}

func TestGroupByParticipant_MultiValued(t *testing.T) {
	// participant is multi-valued: a co-authored entry lands in each
	// author's bucket, and an entry with no participants drops out.
	solo := entry("20260101-100000-d-tac-aaa", withParticipants("Christopher"))
	coauthored := entry("20260101-110000-s-tac-bbb", withParticipants("Christopher", "Claude"))
	orphan := entry("20260101-120000-s-tac-ccc")

	groups, err := groupBy([]*model.Entry{solo, coauthored, orphan}, "participant")
	if err != nil {
		t.Fatalf("groupBy: %v", err)
	}

	// Alphabetical bucket order; orphan contributes to no bucket.
	if got, want := groupKeys(groups), []string{"Christopher", "Claude"}; !equalStrings(got, want) {
		t.Errorf("group order: got %v, want %v", got, want)
	}

	chris := findGroup(groups, "Christopher")
	if got, want := idsOf(chris.Entries), []string{solo.ID, coauthored.ID}; !equalIDs(got, want) {
		t.Errorf("Christopher group: got %v, want %v", got, want)
	}
	claude := findGroup(groups, "Claude")
	if got, want := idsOf(claude.Entries), []string{coauthored.ID}; !equalIDs(got, want) {
		t.Errorf("Claude group: got %v, want %v", got, want)
	}
}

func TestGroupByUnknownField(t *testing.T) {
	a := entry("20260101-100000-d-tac-aaa", withKind(model.KindDirective))
	_, err := groupBy([]*model.Entry{a}, "summary")
	if err == nil {
		t.Fatalf("expected error for unknown field 'summary'")
	}
	// Error must list the supported fields so users have a recovery path —
	// matches the listed-valid-set pattern for unknown function names.
	for _, want := range []string{"kind", "layer", "type"} {
		if !contains(err.Error(), want) {
			t.Errorf("error %q must list field %q", err.Error(), want)
		}
	}
}

func TestGroupByEmptyInput(t *testing.T) {
	groups, err := groupBy(nil, "kind")
	if err != nil {
		t.Fatalf("groupBy: %v", err)
	}
	if len(groups) != 0 {
		t.Errorf("expected zero groups for empty input, got %d", len(groups))
	}
}

// groupKeys returns each group's Key in current order.
func groupKeys(groups []model.Group) []string {
	out := make([]string, len(groups))
	for i, g := range groups {
		out[i] = g.Key
	}
	return out
}

// findGroup returns the group with the given Key, or a zero Group if absent.
func findGroup(groups []model.Group, key string) model.Group {
	for _, g := range groups {
		if g.Key == key {
			return g
		}
	}
	return model.Group{}
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func contains(haystack, needle string) bool {
	for i := 0; i+len(needle) <= len(haystack); i++ {
		if haystack[i:i+len(needle)] == needle {
			return true
		}
	}
	return false
}
