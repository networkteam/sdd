package application_test

import (
	"errors"
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
)

// The seeded graph holds gap signals on the tactical layer (see
// writeCounterEntry). These tests pin that the public Search normalizes the
// abbreviated type/layer forms the MCP tool documents — the class of defect a
// live session caught, where type:"s" silently returned "(no matches)".

func searchFiltered(t *testing.T, app *sdd.Application, req sdd.SearchRequest) (sdd.SearchResult, error) {
	t.Helper()
	return app.Search(t.Context(), sdd.RequestIdentity{Subject: "christopher"}, counterProject, req)
}

func TestSearchFilterNormalizesTypeLayerAndKindAbbreviations(t *testing.T) {
	graphDir, cacheRoot := seedCounterGraph(t)
	app := newCounterApp(t, graphDir, cacheRoot, &countingEmbeddings{})

	cases := []struct {
		name string
		req  sdd.SearchRequest
	}{
		{"type abbrev + kind", sdd.SearchRequest{SyncMode: sdd.SearchSyncAll, Phrase: "alpha", Type: "s", Kind: "gap", Limit: 8, MaxCitations: 1}},
		{"type full name", sdd.SearchRequest{SyncMode: sdd.SearchSyncAll, Phrase: "alpha", Type: "signal", Limit: 8, MaxCitations: 1}},
		{"layer abbrev", sdd.SearchRequest{SyncMode: sdd.SearchSyncAll, Phrase: "alpha", Layer: "tac", Limit: 8, MaxCitations: 1}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			res, err := searchFiltered(t, app, tc.req)
			if err != nil {
				t.Fatalf("Search: %v", err)
			}
			if !strings.Contains(res.Results, "s-tac-aaa") {
				t.Fatalf("filtered search returned no matching signal: %q", res.Results)
			}
		})
	}
}

func TestSearchFilterTextModeFiltersByTypeAbbreviation(t *testing.T) {
	graphDir, cacheRoot := seedCounterGraph(t)
	app := newCounterApp(t, graphDir, cacheRoot, &countingEmbeddings{})

	// Terms-mode (no phrase) runs through the same normalized GraphFilter.
	res, err := searchFiltered(t, app, sdd.SearchRequest{SyncMode: sdd.SearchSyncAll, Terms: []string{"alpha"}, Type: "s", Limit: 8})
	if err != nil {
		t.Fatalf("text Search: %v", err)
	}
	if !strings.Contains(res.Results, "s-tac-aaa") {
		t.Fatalf("text-mode type filter returned no matching signal: %q", res.Results)
	}
	if strings.Contains(res.Results, "s-tac-bbb") {
		t.Errorf("text-mode search leaked the non-matching beta entry: %q", res.Results)
	}
}

func TestSearchFilterUnknownValuesFailLoud(t *testing.T) {
	graphDir, cacheRoot := seedCounterGraph(t)
	app := newCounterApp(t, graphDir, cacheRoot, &countingEmbeddings{})

	cases := []struct {
		name string
		req  sdd.SearchRequest
	}{
		{"unknown type", sdd.SearchRequest{SyncMode: sdd.SearchSyncAll, Phrase: "alpha", Type: "x"}},
		{"unknown kind", sdd.SearchRequest{SyncMode: sdd.SearchSyncAll, Phrase: "alpha", Kind: "gaps"}},
		{"unknown layer", sdd.SearchRequest{SyncMode: sdd.SearchSyncAll, Phrase: "alpha", Layer: "zzz"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := searchFiltered(t, app, tc.req)
			if err == nil {
				t.Fatal("expected a loud error for an unrecognized filter value, got nil (would silently match nothing)")
			}
			var appErr *sdd.ApplicationError
			if !errors.As(err, &appErr) || appErr.Code != sdd.ErrorInvalidArgument {
				t.Fatalf("error = %v, want ApplicationError{Code: invalid_argument}", err)
			}
		})
	}
}
