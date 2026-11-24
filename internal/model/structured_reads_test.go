package model_test

import (
	"math"
	"reflect"
	"slices"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/application/types"
)

func readEntry(t *testing.T, id string, kind model.Kind, opts ...func(*model.Entry)) *model.Entry {
	t.Helper()
	parts, err := model.ParseID(id)
	if err != nil {
		t.Fatalf("bad test ID %q: %v", id, err)
	}
	e := &model.Entry{
		ID: id, Type: model.TypeFromAbbrev[parts.TypeCode], Layer: model.LayerFromAbbrev[parts.LayerCode],
		Kind: kind, Content: "body of " + id, Summary: "summary of " + id, Time: parts.Time,
	}
	for _, opt := range opts {
		opt(e)
	}
	return e
}

func refs(ids ...string) func(*model.Entry) {
	return func(e *model.Entry) {
		for _, id := range ids {
			e.Refs = append(e.Refs, model.Ref{ID: id})
		}
	}
}

func closes(ids ...string) func(*model.Entry) {
	return func(e *model.Entry) { e.Closes = ids }
}

func by(participants ...string) func(*model.Entry) {
	return func(e *model.Entry) { e.Participants = participants }
}

func TestDiffGraphs(t *testing.T) {
	gap := readEntry(t, "20260101-100000-s-tac-gap", model.KindGap)
	plan := readEntry(t, "20260102-100000-d-tac-pln", model.KindPlan, refs(gap.ID))
	planLater := *plan
	planLater.Summary = "regenerated summary"
	done := readEntry(t, "20260103-100000-s-tac-don", model.KindDone, refs(plan.ID), closes(gap.ID))

	from := model.NewGraph([]*model.Entry{gap, plan})
	to := model.NewGraph([]*model.Entry{gap, &planLater, done})

	diff := model.DiffGraphs(from, to)
	if want := []types.AddedEntry{{ID: done.ID, RefIDs: []string{plan.ID, gap.ID}}}; !reflect.DeepEqual(diff.Added, want) {
		t.Errorf("added = %+v, want %+v", diff.Added, want)
	}
	if want := []string{gap.ID}; !reflect.DeepEqual(diff.StatusChanged, want) {
		t.Errorf("status changed = %v, want %v", diff.StatusChanged, want)
	}
	if want := []string{plan.ID}; !reflect.DeepEqual(diff.SummaryChanged, want) {
		t.Errorf("summary changed = %v, want %v", diff.SummaryChanged, want)
	}
}

func TestDiffGraphsReportsCloserMovingToSuccessor(t *testing.T) {
	gap := readEntry(t, "20260101-100000-s-tac-gap", model.KindGap)
	done := readEntry(t, "20260102-100000-s-tac-don", model.KindDone, closes(gap.ID))
	corrected := readEntry(t, "20260103-100000-s-tac-cor", model.KindDone, closes(gap.ID), func(e *model.Entry) { e.Supersedes = []string{done.ID} })

	from := model.NewGraph([]*model.Entry{gap, done})
	to := model.NewGraph([]*model.Entry{gap, done, corrected})
	if before, after := from.DerivedStatus(gap), to.DerivedStatus(gap); before.Kind != after.Kind || after.By != corrected.ID {
		t.Fatalf("gap status %+v -> %+v", before, after)
	}
	if want := []string{gap.ID, done.ID}; !reflect.DeepEqual(model.DiffGraphs(from, to).StatusChanged, want) {
		t.Errorf("status changed = %v, want %v", model.DiffGraphs(from, to).StatusChanged, want)
	}
}

func listFixture(t *testing.T) *model.Graph {
	t.Helper()
	return model.NewGraph([]*model.Entry{
		readEntry(t, "20260101-100000-s-tac-gap", model.KindGap, by("Ada")),
		readEntry(t, "20260102-100000-d-cpt-dir", model.KindDirective, by("Bob")),
		readEntry(t, "20260103-100000-s-tac-fct", model.KindFact, by("Ada", "Bob")),
		readEntry(t, "20260104-100000-s-tac-don", model.KindDone, by("Ada"), closes("20260101-100000-s-tac-gap")),
	})
}

func listedIDs(list []model.ListedEntry) []string {
	ids := make([]string, len(list))
	for i, item := range list {
		ids[i] = item.Entry.ID
	}
	return ids
}

func TestListEntriesFiltersSortsAndFacets(t *testing.T) {
	g := listFixture(t)
	members := []model.ListMember{{Graph: g}}

	list := model.ListEntries(members, model.EntryListFilter{Types: []model.EntryType{model.TypeSignal}, Participants: []string{"Ada"}}, true)
	if want := []string{"20260104-100000-s-tac-don", "20260103-100000-s-tac-fct", "20260101-100000-s-tac-gap"}; !reflect.DeepEqual(listedIDs(list.Entries), want) {
		t.Errorf("newest first = %v, want %v", listedIDs(list.Entries), want)
	}
	wantFacets := types.EntryFacets{
		Types:    map[string]int{"signal": 3},
		Kinds:    map[string]int{"gap": 1, "fact": 1, "done": 1},
		Layers:   map[string]int{"tactical": 3},
		Statuses: map[string]int{"closed-by": 1, "open": 1, types.StatusNoneLabel: 1},
	}
	if !reflect.DeepEqual(list.Facets, wantFacets) {
		t.Errorf("facets = %+v, want %+v", list.Facets, wantFacets)
	}
	if page := listedIDs(list.Page(1, 1)); !reflect.DeepEqual(page, []string{"20260103-100000-s-tac-fct"}) {
		t.Errorf("page = %v", page)
	}
	if page := list.Page(5, 1); page != nil {
		t.Errorf("page past end = %v", page)
	}

	open, _ := model.ParseStatusLabel("open")
	none, _ := model.ParseStatusLabel(types.StatusNoneLabel)
	terms, err := model.CompileSearchTerms([]string{"BODY OF"})
	if err != nil {
		t.Fatal(err)
	}
	list = model.ListEntries(members, model.EntryListFilter{
		Statuses: []model.StatusKind{open, none}, Terms: terms,
		CreatedAfter: time.Date(2026, 1, 1, 12, 0, 0, 0, time.UTC),
	}, false)
	if want := []string{"20260103-100000-s-tac-fct", "20260104-100000-s-tac-don"}; !reflect.DeepEqual(listedIDs(list.Entries), want) {
		t.Errorf("status/time/term filter = %v, want %v", listedIDs(list.Entries), want)
	}
}

func TestStructuredEntryCarriesDerivedStatus(t *testing.T) {
	g := listFixture(t)
	got := g.StructuredEntry(g.ByID["20260101-100000-s-tac-gap"], "dep", true)
	if got.DisplayID != "dep:20260101-100000-s-tac-gap" || got.RepoID != "dep" {
		t.Errorf("display = %q repo = %q", got.DisplayID, got.RepoID)
	}
	if got.Status != "closed-by" || got.StatusBy != "dep:20260104-100000-s-tac-don" {
		t.Errorf("status = %q by %q", got.Status, got.StatusBy)
	}
	if got.Body != "" || got.Summary == "" {
		t.Errorf("summary-only mapping carried body %q / summary %q", got.Body, got.Summary)
	}
}

func TestDirectRefCountIncludesTruncated(t *testing.T) {
	items := []model.ShowTreeItem{{Depth: 1}, {Depth: 2}, {Depth: 1}}
	if n := model.DirectRefCount(items, []model.TruncatedRef{{ID: "x"}}); n != 3 {
		t.Errorf("direct refs = %d, want 3", n)
	}
}

func TestGraphFilterStatuses(t *testing.T) {
	g := listFixture(t)
	got := g.Filter(model.GraphFilter{Statuses: []model.StatusKind{model.StatusClosedBy}})
	if len(got) != 1 || got[0].ID != "20260101-100000-s-tac-gap" {
		t.Errorf("filtered = %v", got)
	}
}

func TestEntryListPageBounds(t *testing.T) {
	list := model.ListEntries([]model.ListMember{{Graph: listFixture(t)}}, model.EntryListFilter{}, false)
	if page := list.Page(1, math.MaxInt); len(page) != 3 {
		t.Errorf("page with unbounded limit = %v", listedIDs(page))
	}
	if page := list.Page(0, 0); len(page) != 4 {
		t.Errorf("page with zero limit = %v", listedIDs(page))
	}
}

func TestListEntriesEmbeddedOnlyWhenIncludedAndOnce(t *testing.T) {
	base := readEntry(t, "20260101-000000-s-prc-bas", model.KindFact, func(e *model.Entry) { e.Embedded = true })
	local := readEntry(t, "20260102-100000-s-tac-loc", model.KindGap)
	foreign := readEntry(t, "20260103-100000-s-tac-frn", model.KindGap)
	members := []model.ListMember{
		{Graph: model.NewGraph([]*model.Entry{base, local})},
		{Graph: model.NewGraph([]*model.Entry{base, foreign}), Repo: "dep.example/a"},
	}
	if got := listedIDs(model.ListEntries(members, model.EntryListFilter{}, false).Entries); !reflect.DeepEqual(got, []string{local.ID, foreign.ID}) {
		t.Errorf("default listing = %v", got)
	}
	if got := listedIDs(model.ListEntries(members, model.EntryListFilter{IncludeEmbedded: true}, false).Entries); !reflect.DeepEqual(got, []string{base.ID, local.ID, foreign.ID}) {
		t.Errorf("listing with embedded = %v", got)
	}
}

func TestListEntriesByDisplayIDs(t *testing.T) {
	base := readEntry(t, "20260101-000000-s-prc-bas", model.KindFact, func(e *model.Entry) { e.Embedded = true })
	local := readEntry(t, "20260102-100000-s-tac-loc", model.KindGap)
	foreign := readEntry(t, "20260103-100000-s-tac-frn", model.KindGap)
	members := []model.ListMember{
		{Graph: model.NewGraph([]*model.Entry{base, local})},
		{Graph: model.NewGraph([]*model.Entry{base, foreign}), Repo: "dep.example/a"},
	}
	tests := map[string]struct {
		ids  []string
		want []string
	}{
		"local and embedded by bare ID": {ids: []string{base.ID, local.ID}, want: []string{base.ID, local.ID}},
		"foreign by prefixed ID":        {ids: []string{"dep.example/a:" + foreign.ID}, want: []string{foreign.ID}},
		"foreign by bare ID":            {ids: []string{foreign.ID}},
		"empty":                         {ids: []string{}},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := listedIDs(model.ListEntries(members, model.EntryListFilter{IDs: tc.ids}, false).Entries); !slices.Equal(got, tc.want) {
				t.Errorf("listed %v, want %v", got, tc.want)
			}
		})
	}
}

func TestLinkKind(t *testing.T) {
	tests := map[string]struct {
		refKind   model.RefKind
		relations []string
		want      string
	}{
		"stored kind":           {refKind: model.RefKindAddresses, relations: []string{"refs"}, want: "addresses"},
		"stored kind and close": {refKind: model.RefKindAddresses, relations: []string{"refs", "closes"}, want: "addresses"},
		"closes":                {relations: []string{"closes"}, want: "closes"},
		"closed by":             {relations: []string{"closed-by"}, want: "closes"},
		"superseded by":         {relations: []string{"superseded-by"}, want: "supersedes"},
		"legacy unknown":        {refKind: model.RefKindUnknown, relations: []string{"refd-by"}, want: "refs"},
		"no kind":               {relations: []string{"refs"}, want: "refs"},
	}
	for name, tc := range tests {
		t.Run(name, func(t *testing.T) {
			if got := model.LinkKind(tc.refKind, tc.relations); got != tc.want {
				t.Errorf("LinkKind = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestShortID(t *testing.T) {
	for id, want := range map[string]string{"20260102-100000-s-tac-loc": "s-tac-loc", "not-an-id": "not-an-id"} {
		if got := model.ShortID(id); got != want {
			t.Errorf("ShortID(%q) = %q, want %q", id, got, want)
		}
	}
}
