package finders_test

import (
	"slices"
	"testing"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

func textEntry(id, title, summary string) *model.Entry {
	return &model.Entry{ID: id, Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindGap, Title: title, Summary: summary, Content: summary}
}

func textSearchIDs(t *testing.T, entries []*model.Entry, q query.SearchQuery) []string {
	t.Helper()
	f := finders.NewSearchFinder(finders.SearchFinderOptions{Graph: model.NewGraph(entries), GraphDir: t.TempDir()})
	res, err := f.Search(t.Context(), q)
	if err != nil {
		t.Fatalf("Search: %v", err)
	}
	var ids []string
	for _, e := range res.Entries {
		ids = append(ids, e.Entry.ID)
	}
	slices.Sort(ids)
	return ids
}

func TestSearchFinder_TextModeMatchesTheTitle(t *testing.T) {
	t.Parallel()
	entries := []*model.Entry{
		textEntry("20260101-100000-d-tac-inv", "Price cache invalidation on write", "Invalidate the price cache on every write."),
		textEntry("20260101-100001-s-tac-oth", "Session token expiry", "Tokens expire after a day."),
	}

	ids := textSearchIDs(t, entries, query.SearchQuery{Terms: []string{"invalidation"}})
	if !slices.Equal(ids, []string{"20260101-100000-d-tac-inv"}) {
		t.Errorf("a term only the title holds should find that entry alone, got %v", ids)
	}
}
