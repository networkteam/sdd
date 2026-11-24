package application_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/application/types"
)

const (
	revGap  = "20260101-100000-s-tac-gap"
	revPlan = "20260102-100000-d-tac-pln"
	revFact = "20260103-100000-s-cpt-fct"
	revDone = "20260104-100000-s-tac-don"
)

func revisionDoc(id, kind, layer, summary string, extra map[string]any) sdd.EntryDocument {
	fm := map[string]any{"kind": kind, "layer": layer, "summary": summary, "participants": []any{"Ada"}}
	if id[16] == 'd' {
		fm["type"] = "decision"
	} else {
		fm["type"] = "signal"
	}
	for k, v := range extra {
		fm[k] = v
	}
	return sdd.EntryDocument{LogicalPath: "2026/01/" + id[6:8] + "-" + id[9:] + ".md", Frontmatter: fm, Body: "Body of " + summary}
}

// revisionStore serves r1 (gap, plan) and r2 (r1 plus a fact and a done that
// closes the gap and changes the plan's summary); its head is r2.
func revisionStore(t *testing.T) (sdd.GraphStore, *[]sdd.SnapshotReadQuery) {
	t.Helper()
	plan := map[string]any{"intent": "pending", "refs": []any{map[string]any{"id": revGap, "kind": "addresses", "desc": "why"}}}
	r1 := []sdd.EntryDocument{
		revisionDoc(revGap, "gap", "tactical", "Gap alpha", nil),
		revisionDoc(revPlan, "plan", "tactical", "Plan beta", plan),
	}
	r2 := []sdd.EntryDocument{
		r1[0],
		revisionDoc(revPlan, "plan", "tactical", "Plan beta regenerated", plan),
		revisionDoc(revFact, "fact", "conceptual", "Fact alpha", map[string]any{"refs": []any{map[string]any{"id": revGap, "kind": "related"}}}),
		revisionDoc(revDone, "done", "tactical", "Done gamma", map[string]any{"refs": []any{map[string]any{"id": revPlan, "kind": "addresses"}}, "closes": []any{revGap}}),
	}
	snapshots := map[string]*sdd.Snapshot{}
	for rev, docs := range map[string][]sdd.EntryDocument{"r1": r1, "r2": r2} {
		s, err := sdd.BuildSnapshot(t.Context(), sdd.SnapshotData{Project: "base", Revision: rev, Entries: docs})
		if err != nil {
			t.Fatal(err)
		}
		snapshots[rev] = s
	}
	var queries []sdd.SnapshotReadQuery
	return acquiredReadStore{acquire: func(_ context.Context, q sdd.SnapshotReadQuery) (*sdd.AcquiredSnapshot, error) {
		queries = append(queries, q)
		rev := q.ExactRevision
		if rev == "" {
			rev = "r2"
		}
		s, ok := snapshots[rev]
		if rev == "cancelled" {
			return nil, fmt.Errorf("reading history: %w", context.Canceled)
		}
		if !ok {
			return nil, fmt.Errorf("revision %s not in history", rev)
		}
		return &sdd.AcquiredSnapshot{Snapshot: s, Attachments: staticGraphStore{}, Release: func() error { return nil }}, nil
	}}, &queries
}

func revisionApp(t *testing.T) (*sdd.Application, *[]sdd.SnapshotReadQuery) {
	t.Helper()
	store, queries := revisionStore(t)
	return preparationApp(t, acquiredRuntime(t, "base", store), nil, nil), queries
}

var reader = sdd.RequestIdentity{Subject: "reader"}

func TestShowReturnsStructuredGroupsAtExactRevision(t *testing.T) {
	app, queries := revisionApp(t)
	result, err := app.Show(t.Context(), reader, "base", sdd.ShowRequest{IDs: []string{revGap}, Revision: "r2", UpDepth: 1, DownDepth: 1, Budget: types.ShowTreeBudget{MaxChildren: 2}})
	if err != nil {
		t.Fatal(err)
	}
	if (*queries)[0].ExactRevision != "r2" || result.Revision != "r2" {
		t.Fatalf("queried %+v, result revision %q", *queries, result.Revision)
	}
	if len(result.Groups) != 1 {
		t.Fatalf("groups = %d", len(result.Groups))
	}
	group := result.Groups[0]
	primary := group.Primary
	if primary.ID != revGap || primary.ShortID != "s-tac-gap" || primary.Status != "closed-by" || primary.StatusBy != revDone || primary.Body != "Body of Gap alpha" || primary.Kind != "gap" {
		t.Errorf("primary = %+v", primary)
	}
	if len(group.Downstream) != 2 || group.DownstreamTotal != 3 || len(group.DownstreamTruncated) != 1 {
		t.Fatalf("downstream = %+v total %d truncated %+v", group.Downstream, group.DownstreamTotal, group.DownstreamTruncated)
	}
	first := group.Downstream[0]
	if first.TargetID != revPlan || first.Entry == nil || first.Entry.Status != "active" || first.Entry.Body != "" || first.RefKind != "addresses" || first.LinkKind != "addresses" || first.RefDesc != "why" || !reflect.DeepEqual(first.Relations, []string{"refd-by"}) {
		t.Errorf("first downstream = %+v entry %+v", first, first.Entry)
	}
	if related := group.Downstream[1]; related.LinkKind != "related" {
		t.Errorf("related downstream = %+v", related)
	}
	if closing := group.DownstreamTruncated[0]; closing.ID != revDone || closing.RefKind != "" || closing.LinkKind != "closes" {
		t.Errorf("closing downstream = %+v", closing)
	}
	if group.UpstreamTotal != 0 {
		t.Errorf("upstream total = %d", group.UpstreamTotal)
	}

	past, err := app.Show(t.Context(), reader, "base", sdd.ShowRequest{IDs: []string{revGap}, Revision: "r1", DownDepth: 1})
	if err != nil {
		t.Fatal(err)
	}
	if got := past.Groups[0]; got.Primary.Status != "open" || got.DownstreamTotal != 1 || past.Revision != "r1" {
		t.Errorf("r1 group = %+v", got)
	}
}

func TestShowUnknownIDs(t *testing.T) {
	app, _ := revisionApp(t)
	_, err := app.Show(t.Context(), reader, "base", sdd.ShowRequest{IDs: []string{revGap, "s-tac-nop"}})
	var appErr *sdd.ApplicationError
	if !errors.As(err, &appErr) || appErr.Code != sdd.ErrorEntryNotFound || appErr.EntryID != "s-tac-nop" {
		t.Fatalf("unknown ID = %v", err)
	}

	result, err := app.Show(t.Context(), reader, "base", sdd.ShowRequest{IDs: []string{"s-tac-nop", "s-tac-gap"}, OmitMissing: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Groups) != 1 || result.Groups[0].Primary.ID != revGap || !reflect.DeepEqual(result.Missing, []string{"s-tac-nop"}) {
		t.Errorf("omitted = %+v missing %v", result.Groups, result.Missing)
	}
}

func TestChangesDiffsTwoRevisions(t *testing.T) {
	app, _ := revisionApp(t)
	result, err := app.Changes(t.Context(), reader, "base", sdd.ChangesRequest{From: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	want := sdd.ChangesResult{
		From: "r1", To: "r2",
		Added:          []types.AddedEntry{{ID: revFact, RefIDs: []string{revGap}}, {ID: revDone, RefIDs: []string{revPlan, revGap}}},
		StatusChanged:  []string{revGap},
		SummaryChanged: []string{revPlan},
	}
	result.Project = sdd.ProjectRef{}
	if !reflect.DeepEqual(result, want) {
		t.Errorf("changes = %+v, want %+v", result, want)
	}

	var appErr *sdd.ApplicationError
	_, err = app.Changes(t.Context(), reader, "base", sdd.ChangesRequest{From: "r1", To: "cancelled"})
	if !errors.Is(err, context.Canceled) || errors.As(err, &appErr) {
		t.Errorf("cancelled to = %v", err)
	}
	_, err = app.Changes(t.Context(), reader, "base", sdd.ChangesRequest{To: "r2"})
	if !errors.As(err, &appErr) || appErr.Code != sdd.ErrorInvalidArgument {
		t.Errorf("empty from = %v", err)
	}
}

func TestExactRevisionReadsReportUnknownRevisionUnavailable(t *testing.T) {
	app, _ := revisionApp(t)
	reads := map[string]func() error{
		"show": func() error {
			_, err := app.Show(t.Context(), reader, "base", sdd.ShowRequest{IDs: []string{revGap}, Revision: "gone"})
			return err
		},
		"list": func() error {
			_, err := app.ListEntries(t.Context(), reader, "base", sdd.ListEntriesRequest{Revision: "gone"})
			return err
		},
		"changes from": func() error {
			_, err := app.Changes(t.Context(), reader, "base", sdd.ChangesRequest{From: "gone", To: "r2"})
			return err
		},
		"changes to": func() error {
			_, err := app.Changes(t.Context(), reader, "base", sdd.ChangesRequest{From: "r1", To: "gone"})
			return err
		},
	}
	for name, read := range reads {
		var appErr *sdd.ApplicationError
		if err := read(); !errors.As(err, &appErr) || appErr.Code != sdd.ErrorRevisionUnavailable || appErr.Revision != "gone" {
			t.Errorf("%s = %v", name, err)
		}
	}
}

func TestListEntriesFiltersPagesAndFacets(t *testing.T) {
	app, queries := revisionApp(t)
	result, err := app.ListEntries(t.Context(), reader, "base", sdd.ListEntriesRequest{
		Revision: "r2", Types: []string{"s"}, NewestFirst: true, Offset: 1, Limit: 1,
	})
	if err != nil {
		t.Fatal(err)
	}
	if (*queries)[0].ExactRevision != "r2" || result.Revision != "r2" || result.Total != 3 {
		t.Fatalf("revision %q total %d", result.Revision, result.Total)
	}
	if len(result.Entries) != 1 || result.Entries[0].ID != revFact || result.Entries[0].Body == "" {
		t.Errorf("page = %+v", result.Entries)
	}
	wantStatuses := map[string]int{"closed-by": 1, "open": 1, types.StatusNoneLabel: 1}
	if !reflect.DeepEqual(result.Facets.Statuses, wantStatuses) || result.Facets.Types["signal"] != 3 || result.Facets.Layers["conceptual"] != 1 {
		t.Errorf("facets = %+v", result.Facets)
	}

	result, err = app.ListEntries(t.Context(), reader, "base", sdd.ListEntriesRequest{Statuses: []string{"open", "active"}, Terms: []string{"alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	if result.Total != 1 || result.Entries[0].ID != revFact {
		t.Errorf("status+term listing = %+v", result)
	}

	for name, tc := range map[string]struct {
		ids  []string
		want []string
	}{
		"selected": {ids: []string{revDone, revGap, "unknown"}, want: []string{revGap, revDone}},
		"empty":    {ids: []string{}},
	} {
		result, err = app.ListEntries(t.Context(), reader, "base", sdd.ListEntriesRequest{IDs: tc.ids, Statuses: []string{"closed-by", "none"}})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, entry := range result.Entries {
			got = append(got, entry.ID)
		}
		if !slices.Equal(got, tc.want) || result.Total != len(tc.want) {
			t.Errorf("%s: listed %v, total %d", name, got, result.Total)
		}
	}

	for name, request := range map[string]sdd.ListEntriesRequest{
		"unknown status":  {Statuses: []string{"bogus"}},
		"unknown type":    {Types: []string{"x"}},
		"unknown layer":   {Layers: []string{"x"}},
		"negative offset": {Offset: -1},
		"negative limit":  {Limit: -1},
		"invalid term":    {Terms: []string{"("}},
	} {
		_, err = app.ListEntries(t.Context(), reader, "base", request)
		var appErr *sdd.ApplicationError
		if !errors.As(err, &appErr) || appErr.Code != sdd.ErrorInvalidArgument {
			t.Errorf("%s = %v", name, err)
		}
	}
}

func TestSearchReturnsStructuredHitsFilteredByStatus(t *testing.T) {
	app, _ := revisionApp(t)
	result, err := app.Search(t.Context(), reader, "base", sdd.SearchRequest{SyncMode: sdd.SearchSyncNone, Terms: []string{"alpha"}, MaxCitations: 1})
	if err != nil {
		t.Fatal(err)
	}
	if result.Revision != "r2" || len(result.Hits) != 2 || len(result.Hits) != len(result.EntryIDs) {
		t.Fatalf("revision %q hits %+v", result.Revision, result.Hits)
	}
	for i, hit := range result.Hits {
		if hit.Entry.DisplayID != result.EntryIDs[i] || hit.Score <= 0 || len(hit.Citations) == 0 {
			t.Errorf("hit %d = %+v", i, hit)
		}
	}

	result, err = app.Search(t.Context(), reader, "base", sdd.SearchRequest{SyncMode: sdd.SearchSyncNone, Terms: []string{"alpha"}, Statuses: []string{"closed-by"}, Limit: 1})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].Entry.ID != revGap || result.Hits[0].Entry.Status != "closed-by" {
		t.Errorf("closed-by hits = %+v", result.Hits)
	}
}

func TestSearchStatusFilterSupersededByIncludesSupersededEntries(t *testing.T) {
	old := revisionDoc("20260101-100000-d-tac-old", "directive", "tactical", "Directive alpha", map[string]any{"intent": "guiding"})
	successor := revisionDoc("20260102-100000-d-tac-new", "directive", "tactical", "Directive alpha revised", map[string]any{"intent": "guiding", "supersedes": []any{"20260101-100000-d-tac-old"}})
	snapshot, err := sdd.BuildSnapshot(t.Context(), sdd.SnapshotData{Project: "base", Revision: "r1", Entries: []sdd.EntryDocument{old, successor}})
	if err != nil {
		t.Fatal(err)
	}
	app := preparationApp(t, acquiredRuntime(t, "base", staticGraphStore{snapshot: snapshot}), nil, nil)

	result, err := app.Search(t.Context(), reader, "base", sdd.SearchRequest{SyncMode: sdd.SearchSyncNone, Terms: []string{"alpha"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].Entry.ID != "20260102-100000-d-tac-new" {
		t.Errorf("default hits = %+v", result.Hits)
	}
	result, err = app.Search(t.Context(), reader, "base", sdd.SearchRequest{SyncMode: sdd.SearchSyncNone, Terms: []string{"alpha"}, Statuses: []string{"superseded-by"}})
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Hits) != 1 || result.Hits[0].Entry.ID != "20260101-100000-d-tac-old" || result.Hits[0].Entry.Status != "superseded-by" {
		t.Errorf("superseded-by hits = %+v", result.Hits)
	}
}
