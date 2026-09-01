package finders_test

import (
	"context"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/pkg/llm"
)

// Pre-flight reads the per-repo config for its declared-dependency and
// language checks; a nil config would run them blind and silently weaken the
// write gate — the engine write path shipped exactly that (s-tac-uya). The
// gate refuses to run instead.
func TestPreflight_WithoutConfigErrors(t *testing.T) {
	runner := llm.RunnerFunc(func(context.Context, llm.Request) (llm.Result, error) {
		return llm.Result{Text: `{"findings": []}`, Identity: llm.Identity{Provider: "test", Model: "test"}}, nil
	})
	f := finders.New(finders.Options{PreflightRunner: runner})
	proposed := &model.Entry{Type: model.TypeSignal, Layer: model.LayerTactical, Kind: model.KindGap, Content: "observation"}
	_, err := f.Preflight(context.Background(), model.NewGraph(nil), query.PreflightQuery{Entry: proposed})
	if err == nil || !strings.Contains(err.Error(), "config") {
		t.Fatalf("Preflight without config must error naming the missing config, got %v", err)
	}
}
