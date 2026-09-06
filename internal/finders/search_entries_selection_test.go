package finders_test

import (
	"context"
	"testing"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/pkg/application/types"
)

type forbiddenPublicationRead struct{ t *testing.T }

func (r forbiddenPublicationRead) EntryPublished(context.Context, types.SearchEntryVersion) (bool, error) {
	r.t.Fatal("ineligible entry reached publication lookup")
	return false, nil
}

func TestDiscoverySelectionSharesEligibility(t *testing.T) {
	const id = "20260101-100000-s-tac-aaa"
	for _, ids := range [][]string{nil, {id}} {
		finder := finders.SearchEntriesFinder{Graph: model.NewGraph([]*model.Entry{{ID: id, Embedded: true}}), EntryIDs: ids, ExcludeEmbedded: true, Store: forbiddenPublicationRead{t: t}}
		for _, err := range finder.Discover(t.Context(), query.SearchDiscoveryCursor{}) {
			t.Fatalf("ineligible entry yielded: %v", err)
		}
	}
}
