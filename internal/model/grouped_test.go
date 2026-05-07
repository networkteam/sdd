package model_test

import (
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestGrouped_ShapeIsGrouped(t *testing.T) {
	g := model.Grouped{Field: "kind"}
	if got, want := g.Shape(), model.ShapeGrouped; got != want {
		t.Errorf("Shape: got %q, want %q", got, want)
	}
}

func TestFlatList_ShapeIsFlatList(t *testing.T) {
	// Sanity check that the existing shape didn't drift when grouped landed —
	// the executor relies on the two shapes staying distinct constants.
	f := model.FlatList{}
	if got, want := f.Shape(), model.ShapeFlatList; got != want {
		t.Errorf("Shape: got %q, want %q", got, want)
	}
	if model.ShapeFlatList == model.ShapeGrouped {
		t.Errorf("ShapeFlatList and ShapeGrouped collapsed to the same value")
	}
}
