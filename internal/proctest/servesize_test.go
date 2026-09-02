package proctest_test

import (
	"fmt"
	"os"
	"sort"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/baseprocedures"
	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/proctest"
	sdd "github.com/networkteam/sdd/pkg/application"
)

// provisionalServeBudgetBytes is the byte budget one automatic serve targets
// until the calibrated default lands (d-tac-qwc): the ~10K-token host output
// floor (20260719-122547-s-tac-40d) at roughly 3.5 bytes per token.
const provisionalServeBudgetBytes = 36000

// knownOversize names the (procedure, step) serves measured over budget on
// the realistic fixture — the fix list this measurement exists to produce
// (d-tac-rzi). Entries leave this map as bounding slices land; a stale entry
// (now under budget) fails the test so the list can only shrink.
var knownOversize = map[string]bool{}

type serveRow struct {
	Proc, Step, Largest string
	Total               int
}

type recorder struct {
	rows  []serveRow
	steps map[string]map[string]bool
}

func (r *recorder) add(proc, step string, total int, largest string) {
	r.rows = append(r.rows, serveRow{Proc: proc, Step: step, Total: total, Largest: largest})
	if r.steps == nil {
		r.steps = map[string]map[string]bool{}
	}
	if r.steps[proc] == nil {
		r.steps[proc] = map[string]bool{}
	}
	r.steps[proc][step] = true
}

// serve records one WorkflowServe's wire-relevant weight: the composed
// instructions plus the schema and produced parts from the engine's per-part
// accounting; the largest single part names where the bytes come from.
func (r *recorder) serve(serve *sdd.WorkflowServe) {
	if serve == nil {
		return
	}
	total := len(serve.Instructions)
	largest := engine.PartSize{}
	for _, p := range serve.Sizes {
		if p.Part == "schema" || p.Part == "produced" {
			total += p.Bytes
		}
		if p.Bytes > largest.Bytes {
			largest = p
		}
	}
	name := fmt.Sprintf("%s (%dB)", largest.Part, largest.Bytes)
	if largest.Part == "" {
		name = "instructions"
	}
	r.add(serve.Procedure, serve.Step, total, name)
	if serve.Base != nil {
		r.serve(serve.Base)
	}
}

func (r *recorder) framing(blocks []string) {
	total := 0
	largest, largestLen := "", 0
	for i, b := range blocks {
		total += len(b)
		if len(b) > largestLen {
			largest, largestLen = fmt.Sprintf("block[%d] (%dB)", i, len(b)), len(b)
		}
	}
	r.add("user-dialogue", "framing", total, largest)
}

func (r *recorder) report(t *testing.T) {
	t.Helper()
	rows := append([]serveRow(nil), r.rows...)
	sort.Slice(rows, func(i, j int) bool { return rows[i].Total > rows[j].Total })
	var b strings.Builder
	b.WriteString("measured automatic serves (desc):\n")
	for _, row := range rows {
		fmt.Fprintf(&b, "  %-16s %-14s %7dB  largest: %s\n", row.Proc, row.Step, row.Total, row.Largest)
	}
	t.Log(b.String())
}

// assertBudget fails for any measured serve over the provisional budget that
// is not on the known-oversize list, and for stale list entries now under it.
func (r *recorder) assertBudget(t *testing.T) {
	t.Helper()
	over := map[string]int{}
	for _, row := range r.rows {
		key := row.Proc + "/" + row.Step
		if row.Total > provisionalServeBudgetBytes && row.Total > over[key] {
			over[key] = row.Total
		}
	}
	for key, total := range over {
		if !knownOversize[key] {
			t.Errorf("serve %s measures %dB, over the %dB budget and not on the known-oversize list", key, total, provisionalServeBudgetBytes)
		}
	}
	for key := range knownOversize {
		if _, still := over[key]; !still {
			t.Errorf("known-oversize entry %s now measures under budget — remove it", key)
		}
	}
}

// logCoverage names the spec steps no walk has measured — the harness's open
// debt toward "every automatic serve of the shipped procedures" (d-tac-rzi).
func (r *recorder) logCoverage(t *testing.T) {
	t.Helper()
	entries, err := baseprocedures.Entries()
	if err != nil {
		t.Fatal(err)
	}
	var missing []string
	for _, entry := range entries {
		spec, err := engine.ParseSpec(entry)
		if err != nil {
			t.Fatalf("parsing shipped spec %s: %v", entry.ID, err)
		}
		for _, step := range spec.Steps {
			if !r.steps[spec.Canonical][step.ID] {
				missing = append(missing, spec.Canonical+"/"+step.ID)
			}
		}
	}
	sort.Strings(missing)
	if len(missing) > 0 {
		t.Logf("steps not yet measured by any walk (%d):\n  %s", len(missing), strings.Join(missing, "\n  "))
	}
}

// walkAll drives every shipped procedure over the world far enough to reach
// its heavy serves, recording each serve it receives.
func walkAll(t *testing.T, world *proctest.World, rec *recorder, anchor string) {
	t.Helper()
	ctx := t.Context()

	workflow, door, err := world.App.OpenWorkflow(ctx, world.Identity, "proctest", sdd.WorkflowOpenRequest{ClientName: "servesize-door"})
	if err != nil {
		t.Fatal(err)
	}
	rec.serve(door)
	blocks, err := workflow.Framing(ctx, world.Identity)
	if err != nil {
		t.Fatal(err)
	}
	rec.framing(blocks)

	// catch-up: the compose serve, then the junction after the briefing.
	s := world.Open(t, "servesize-catchup")
	serve := s.Start(t, "catch-up", nil)
	rec.serve(serve)
	rec.serve(s.Report(t, serve.Instance, map[string]any{"briefing": "*Focus.*\n\n1. Continue.\n\n**What do you want to move forward?**"}))

	// engage: anchor prompt, then the chain-serving brief step.
	s = world.Open(t, "servesize-engage")
	serve = s.Start(t, "engage", nil)
	rec.serve(serve)
	rec.serve(s.Report(t, serve.Instance, map[string]any{"anchor": anchor, "goal": "measure the chains"}))

	// capture: the assemble serve (type overview, topic labels).
	s = world.Open(t, "servesize-capture")
	rec.serve(s.Start(t, "capture", nil))

	// groom: the sweep serve (wip inject).
	s = world.Open(t, "servesize-groom")
	rec.serve(s.Start(t, "groom", nil))

	// implementation, evaluate: anchor-seeded starts reach their first
	// chain-serving step directly.
	s = world.Open(t, "servesize-implementation")
	rec.serve(s.Start(t, "implementation", map[string]any{"anchor": anchor}))
	s = world.Open(t, "servesize-evaluate")
	rec.serve(s.Start(t, "evaluate", map[string]any{"anchor": anchor}))

	// explore: primaries with full bodies.
	s = world.Open(t, "servesize-explore")
	rec.serve(s.Start(t, "explore", map[string]any{"targets": []any{anchor}, "goal": "measure the primaries"}))

	// interview and bootstrap openings.
	s = world.Open(t, "servesize-interview")
	rec.serve(s.Start(t, "interview", map[string]any{"goal": "measure the ask serve", "widenReport": "searched the fixture; no tensions"}))
	s = world.Open(t, "servesize-bootstrap")
	rec.serve(s.Start(t, "bootstrap", nil))
}

func TestServeSizesShippedProcedures(t *testing.T) {
	world, hub := proctest.NewRealisticWorld(t, proctest.DefaultShape())
	rec := &recorder{}
	walkAll(t, world, rec, hub)
	rec.report(t)
	rec.logCoverage(t)
	rec.assertBudget(t)
}

// TestServeSizeCalibration measures against a real graph instead of the
// generated fixture — the calibration instrument behind the default budget
// numbers (d-tac-rzi). Reads only; sessions land in temp stores.
func TestServeSizeCalibration(t *testing.T) {
	dir := os.Getenv("SDD_SERVESIZE_GRAPH")
	if dir == "" {
		t.Skip("set SDD_SERVESIZE_GRAPH to a graph dir (and optionally SDD_SERVESIZE_ANCHOR to an entry ID) to calibrate")
	}
	world := proctest.NewWorld(t, proctest.WithGraphDir(dir))
	rec := &recorder{}
	ctx := t.Context()

	workflow, door, err := world.App.OpenWorkflow(ctx, world.Identity, "proctest", sdd.WorkflowOpenRequest{ClientName: "calibration-door"})
	if err != nil {
		t.Fatal(err)
	}
	rec.serve(door)
	blocks, err := workflow.Framing(ctx, world.Identity)
	if err != nil {
		t.Fatal(err)
	}
	rec.framing(blocks)

	s := world.Open(t, "calibration-catchup")
	rec.serve(s.Start(t, "catch-up", nil))
	s = world.Open(t, "calibration-capture")
	rec.serve(s.Start(t, "capture", nil))
	s = world.Open(t, "calibration-groom")
	rec.serve(s.Start(t, "groom", nil))

	if anchor := os.Getenv("SDD_SERVESIZE_ANCHOR"); anchor != "" {
		s = world.Open(t, "calibration-engage")
		serve := s.Start(t, "engage", nil)
		rec.serve(serve)
		rec.serve(s.Report(t, serve.Instance, map[string]any{"anchor": anchor, "goal": "calibration measurement"}))
		s = world.Open(t, "calibration-implementation")
		rec.serve(s.Start(t, "implementation", map[string]any{"anchor": anchor}))
		s = world.Open(t, "calibration-evaluate")
		rec.serve(s.Start(t, "evaluate", map[string]any{"anchor": anchor}))
		s = world.Open(t, "calibration-explore")
		rec.serve(s.Start(t, "explore", map[string]any{"targets": []any{anchor}, "goal": "calibration measurement"}))
	}
	rec.report(t)
}
