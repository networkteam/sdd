package finders_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// TestLint_FlagsLegacyRefKindUnknown verifies that the lint finder surfaces
// per-ref kind values outside the capturable closed set. Legacy bare-string
// refs parse as RefKindUnknown — the lint layer is where they get flagged so
// the author can re-author with an explicit kind. Per d-tac-cs0 AC 11.
func TestLint_FlagsLegacyRefKindUnknown(t *testing.T) {
	target := &model.Entry{
		ID:    "20260406-100000-s-stg-aaa",
		Type:  model.TypeSignal,
		Layer: model.LayerStrategic,
		Kind:  model.KindGap,
	}
	bad := &model.Entry{
		ID:    "20260406-100100-d-stg-bbb",
		Type:  model.TypeDecision,
		Layer: model.LayerStrategic,
		Kind:  model.KindDirective,
		Refs:  []model.Ref{{ID: target.ID, Kind: model.RefKindUnknown}},
	}
	g := model.NewGraph([]*model.Entry{target, bad})

	f := finders.New(finders.Options{})
	result, err := f.Lint(query.LintQuery{Graph: g})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}

	hit := false
	for _, e := range result.Entries {
		if e.ID != bad.ID {
			continue
		}
		for _, w := range e.Warnings {
			if w.Field == "refs" && strings.Contains(w.Message, "unknown") {
				hit = true
			}
		}
	}
	if !hit {
		t.Fatalf("expected refs warning naming the unknown kind on %s, got entries: %v", bad.ID, entryIDs(result.Entries))
	}
}

func entryIDs(entries []*model.Entry) []string {
	ids := make([]string, len(entries))
	for i, e := range entries {
		ids[i] = e.ID
	}
	return ids
}
