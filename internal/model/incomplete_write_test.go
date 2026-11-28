package model_test

import (
	"context"
	"errors"
	"fmt"
	"testing"

	"github.com/networkteam/sdd/internal/model"
)

func TestIncompleteWrite(t *testing.T) {
	cause := fmt.Errorf("summarizing: %w", context.Canceled)
	marked := model.IncompleteWrite(cause)
	if !errors.Is(marked, model.ErrIncompleteWrite) || !errors.Is(marked, context.Canceled) {
		t.Errorf("the mark should match the sentinel and keep the cause, got %v", marked)
	}
	if marked.Error() != cause.Error() {
		t.Errorf("the mark should leave the message unchanged, got %q", marked.Error())
	}
	if model.IncompleteWrite(nil) != nil {
		t.Error("nil should stay nil")
	}
}
