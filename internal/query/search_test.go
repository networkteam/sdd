package query_test

import (
	"strings"
	"testing"
	"unicode/utf8"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/textsplitter"
)

func TestSimilarTitles_NeedsABodyToSearchBy(t *testing.T) {
	if _, ok := query.SimilarTitles(&model.Entry{ID: "20260101-100000-s-tac-aaa", Content: " \n"}); ok {
		t.Error("an entry without a body should have nothing to search by")
	}
	q, ok := query.SimilarTitles(&model.Entry{ID: "20260101-100000-s-tac-aaa", Content: "A body."})
	if !ok || q.Phrase != "A body." || q.Limit != model.SimilarTitleCount || q.SyncMode != query.SearchSyncNone {
		t.Errorf("the query should search the body for the similar-title count without syncing, got %+v", q)
	}
	if q.Admits(&model.Entry{ID: "20260101-100000-s-tac-aaa", Title: "Own"}) || q.Admits(&model.Entry{ID: "20260101-100000-s-tac-bbb"}) {
		t.Error("the query should admit neither the entry itself nor an untitled entry")
	}
	if !q.Admits(&model.Entry{ID: "20260101-100000-s-tac-bbb", Title: "Other"}) {
		t.Error("the query should admit another titled entry")
	}
}

func TestSimilarTitles_LeavesTheLinkedEntriesOut(t *testing.T) {
	entry := &model.Entry{
		ID: "20260101-100000-d-tac-aaa", Content: "A body.",
		Refs:       []model.Ref{{ID: "20260101-100000-s-tac-ref"}},
		Closes:     []string{"20260101-100000-s-tac-cls"},
		Supersedes: []string{"20260101-100000-d-tac-sup"},
	}
	q, ok := query.SimilarTitles(entry)
	if !ok {
		t.Fatal("an entry with a body should have a query")
	}
	for _, id := range []string{"20260101-100000-s-tac-ref", "20260101-100000-s-tac-cls", "20260101-100000-d-tac-sup"} {
		if q.Admits(&model.Entry{ID: id, Title: "Linked"}) {
			t.Errorf("the query should leave out %s, which the prompt lists as a linked entry", id)
		}
	}
}

func TestSimilarTitles_CutsTheBodyToAnIndexedChunk(t *testing.T) {
	long := strings.Repeat("ä", textsplitter.DefaultChunkSize+50)
	q, ok := query.SimilarTitles(&model.Entry{ID: "20260101-100000-s-tac-aaa", Content: long})
	if !ok {
		t.Fatal("an entry with a body should have a query")
	}
	if got := utf8.RuneCountInString(q.Phrase); got != textsplitter.DefaultChunkSize || !strings.HasPrefix(long, q.Phrase) {
		t.Errorf("the query should be the body's head, as long as an indexed chunk, got %d runes", got)
	}
}

func TestSearchQueryAdmitsEveryEntryWithoutGates(t *testing.T) {
	if !(query.SearchQuery{}).Admits(&model.Entry{ID: "20260101-100000-s-tac-aaa"}) {
		t.Error("a query without the Titled and Exclude gates should admit an untitled entry")
	}
}
