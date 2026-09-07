package model_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestBatchesStopsPullingWhenConsumerStops(t *testing.T) {
	pulled := 0
	source := func(yield func(int, error) bool) {
		for i := range 100 {
			pulled++
			if !yield(i, nil) {
				return
			}
		}
	}
	for batch, err := range model.Batches(source, 3) {
		if err != nil || !reflect.DeepEqual(batch, []int{0, 1, 2}) {
			t.Fatalf("%v %v", batch, err)
		}
		break
	}
	if pulled != 3 {
		t.Fatalf("pulled %d", pulled)
	}
}

func TestBatchesRetainsTailBeforeSourceError(t *testing.T) {
	want := errors.New("source failed")
	source := func(yield func(int, error) bool) {
		if !yield(1, nil) {
			return
		}
		yield(0, want)
	}
	var batches [][]int
	var got error
	for batch, err := range model.Batches(source, 3) {
		if err != nil {
			got = err
			break
		}
		batches = append(batches, batch)
	}
	if !errors.Is(got, want) || !reflect.DeepEqual(batches, [][]int{{1}}) {
		t.Fatalf("%v %v", batches, got)
	}
}
