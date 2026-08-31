// Lane composition of the shipped capture procedure's assemble unit, below
// the MCP layer: serves arrive un-deduped, so InstructionLanes carries every
// non-empty lane and Instructions stays their complete join plus diagnostics.
package proctest_test

import (
	"regexp"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/proctest"
	sdd "github.com/networkteam/sdd/pkg/application"
)

func laneNames(serve *sdd.WorkflowServe) []string {
	names := make([]string, 0, len(serve.InstructionLanes))
	for _, lane := range serve.InstructionLanes {
		names = append(names, lane.Name)
	}
	return names
}

func hasLane(serve *sdd.WorkflowServe, name string) bool {
	for _, lane := range serve.InstructionLanes {
		if lane.Name == name {
			return true
		}
	}
	return false
}

// laneText returns the named lane's rendered text, failing when the serve
// does not carry the lane.
func laneText(t *testing.T, serve *sdd.WorkflowServe, name string) string {
	t.Helper()
	for _, lane := range serve.InstructionLanes {
		if lane.Name == name {
			return lane.Text
		}
	}
	t.Fatalf("serve carries no lane %q, lanes = %v", name, laneNames(serve))
	return ""
}

func TestCapture_AssembleServesDeclaredLanes(t *testing.T) {
	_, session := newCaptureWorld(t, "lanes-declared")

	serve := session.Start(t, "capture", nil)
	proctest.RequireStep(t, serve, "assemble")

	for _, want := range []string{"intro", "typeSystem", "grounding", "topics"} {
		if !hasLane(serve, want) {
			t.Errorf("assemble serve missing lane %q, lanes = %v", want, laneNames(serve))
		}
	}
	// An unanchored capture renders anchorContext empty, so the lane drops.
	if hasLane(serve, "anchorContext") {
		t.Errorf("unanchored assemble must not carry anchorContext, lanes = %v", laneNames(serve))
	}

	// Below the MCP layer, Instructions is still the complete joined unit.
	if !strings.Contains(serve.Instructions, "Draft one entry") {
		t.Errorf("Instructions missing intro text:\n%s", serve.Instructions)
	}
	if !strings.Contains(serve.Instructions, "Labels in use across active entries:") {
		t.Errorf("Instructions missing topics header:\n%s", serve.Instructions)
	}
}

func TestCapture_TopicsLaneIsBareLabels(t *testing.T) {
	world := proctest.NewWorld(t, proctest.WithEntries(
		&model.Entry{
			ID: "20260601-100000-s-tac-one", Type: model.TypeSignal, Kind: model.KindGap, Layer: model.LayerTactical,
			Summary: "A signal on two topics.", Content: "A signal on two topics.",
			Topics: proctest.MustTopics("cli/ux", "agent/ux"),
		},
		&model.Entry{
			ID: "20260601-110000-d-tac-two", Type: model.TypeDecision, Kind: model.KindDirective, Layer: model.LayerTactical, Intent: model.IntentPending,
			Summary: "A directive sharing one topic.", Content: "A directive sharing one topic.",
			Topics: proctest.MustTopics("cli/ux", "graph/topics"),
		},
	))
	session := world.Open(t, "lanes-topics")

	serve := session.Start(t, "capture", nil)
	proctest.RequireStep(t, serve, "assemble")

	topics := laneText(t, serve, "topics")
	for _, label := range []string{"cli/ux", "agent/ux", "graph/topics"} {
		if !regexp.MustCompile(`(?m)^` + regexp.QuoteMeta(label) + `$`).MatchString(topics) {
			t.Errorf("topics lane missing bare label line %q:\n%s", label, topics)
		}
	}
	if strings.Contains(topics, "heat") {
		t.Errorf("topics lane must carry no heat column:\n%s", topics)
	}
	if counts := regexp.MustCompile(`(?m)^\s*\d+\s+\S+`); counts.MatchString(topics) {
		t.Errorf("topics lane must carry no count columns:\n%s", topics)
	}
}

func TestCapture_AnchoredCaptureCarriesAnchorLane(t *testing.T) {
	_, session := newCaptureWorld(t, "lanes-anchored")

	serve := session.Start(t, "capture", map[string]any{"anchor": captureRefID})
	proctest.RequireStep(t, serve, "assemble")

	anchor := laneText(t, serve, "anchorContext")
	if want := "anchored on " + captureRefID; !strings.Contains(anchor, want) {
		t.Errorf("anchorContext lane missing %q:\n%s", want, anchor)
	}
}
