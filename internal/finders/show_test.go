package finders

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

func TestShow_ShortIDResolves(t *testing.T) {
	g := model.NewGraph([]*model.Entry{
		entry("20260406-100000-s-stg-aaa"),
		entry("20260406-100100-d-tac-bbb", withKind(model.KindDirective)),
	})
	f := New(nil)

	result, err := f.Show(query.ShowQuery{
		Graph: g,
		IDs:   []string{"d-tac-bbb"},
	})
	if err != nil {
		t.Fatalf("Show: %v", err)
	}
	if len(result.Groups) != 1 {
		t.Fatalf("Groups = %d, want 1", len(result.Groups))
	}
	if got := result.Groups[0].Primary.ID; got != "20260406-100100-d-tac-bbb" {
		t.Errorf("Primary.ID = %q, want resolved full ID", got)
	}
}

func TestShow_AmbiguousShortIDErrors(t *testing.T) {
	g := model.NewGraph([]*model.Entry{
		entry("20260406-100000-s-stg-xyz"),
		entry("20260407-110000-s-stg-xyz"),
	})
	f := New(nil)

	_, err := f.Show(query.ShowQuery{
		Graph: g,
		IDs:   []string{"s-stg-xyz"},
	})
	if err == nil {
		t.Fatal("Show: want error for ambiguous short ID")
	}
	if !strings.Contains(err.Error(), "ambiguous") {
		t.Errorf("error = %v, want ambiguous message", err)
	}
}

func TestShow_NotFoundShortIDPassesThroughError(t *testing.T) {
	g := model.NewGraph([]*model.Entry{
		entry("20260406-100000-s-stg-aaa"),
	})
	f := New(nil)

	_, err := f.Show(query.ShowQuery{
		Graph: g,
		IDs:   []string{"s-stg-zzz"},
	})
	if err == nil {
		t.Fatal("Show: want error for unresolved short ID")
	}
	if !strings.Contains(err.Error(), "entry not found") {
		t.Errorf("error = %v, want entry-not-found message", err)
	}
}
