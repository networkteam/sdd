package mcpapp_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"github.com/networkteam/sdd/internal/basefacts"
	mcpserver "github.com/networkteam/sdd/pkg/mcpapp"
)

// captureIntroProse opens the shipped base capture's static intro lane —
// present exactly when that lane serves in full.
const captureIntroProse = "Draft one entry — one idea, dialogue first."

const withheldLanesPrefix = "(lanes served earlier this session: "

// withheldLanes extracts the lane names from a serve's withheld-lanes line;
// nil when the line is absent.
func withheldLanes(t *testing.T, instructions string) []string {
	t.Helper()
	_, rest, found := strings.Cut(instructions, withheldLanesPrefix)
	if !found {
		return nil
	}
	names, _, found := strings.Cut(rest, " — resume_session")
	if !found {
		t.Fatalf("withheld-lanes line lacks its fullReplay breadcrumb: %q", instructions)
	}
	return strings.Split(names, ", ")
}

// requireWithheldLanes asserts the serve collapsed already-served lanes into
// the withheld-lanes line and that the line names each given lane.
func requireWithheldLanes(t *testing.T, serve mcpserver.ServeResult, names ...string) {
	t.Helper()
	got := withheldLanes(t, serve.Instructions)
	if len(got) == 0 {
		t.Fatalf("expected a withheld-lanes line, got instructions %q", serve.Instructions)
	}
	for _, name := range names {
		if !slices.Contains(got, name) {
			t.Fatalf("withheld lanes %v should name %q (instructions %q)", got, name, serve.Instructions)
		}
	}
}

func requireNoWithheldLanes(t *testing.T, serve mcpserver.ServeResult) {
	t.Helper()
	if strings.Contains(serve.Instructions, withheldLanesPrefix) {
		t.Fatalf("expected no withheld-lanes line, got %q", serve.Instructions)
	}
}

func requireInstructionsContain(t *testing.T, serve mcpserver.ServeResult, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if !strings.Contains(serve.Instructions, sub) {
			t.Fatalf("instructions should contain %q, got %q", sub, serve.Instructions)
		}
	}
}

func requireInstructionsExclude(t *testing.T, serve mcpserver.ServeResult, subs ...string) {
	t.Helper()
	for _, sub := range subs {
		if strings.Contains(serve.Instructions, sub) {
			t.Fatalf("instructions should not repeat %q, got %q", sub, serve.Instructions)
		}
	}
}

func requireFullReportSchema(t *testing.T, label string, schema map[string]any) {
	t.Helper()
	if _, stub := schema["served_earlier"]; stub {
		t.Fatalf("%s: report schema should serve in full, got the stub %v", label, schema)
	}
	if _, ok := schema["properties"]; !ok {
		t.Fatalf("%s: a full report schema carries properties, got %v", label, schema)
	}
}

func requireStubReportSchema(t *testing.T, label string, schema map[string]any) {
	t.Helper()
	if _, ok := schema["properties"]; ok {
		t.Fatalf("%s: report schema should be the served-earlier stub, got the full schema", label)
	}
	if _, stub := schema["served_earlier"]; !stub {
		t.Fatalf("%s: the stub must carry served_earlier, got %v", label, schema)
	}
}

// openServe finds the serve for one procedure among a resume's open instances.
func openServe(t *testing.T, resumed mcpserver.ResumeSessionResult, procedure string) mcpserver.ServeResult {
	t.Helper()
	for _, open := range resumed.Open {
		if open.Procedure == procedure {
			return open
		}
	}
	t.Fatalf("no open %s serve in %+v", procedure, resumed.Open)
	return mcpserver.ServeResult{}
}

// callRaw invokes a tool and returns the structured content as a raw map —
// the wire shape, before typed decoding could drop fields the result struct
// does not declare.
func callRaw(t *testing.T, cs *mcp.ClientSession, tool string, args map[string]any) map[string]any {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: tool, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", tool, err)
	}
	if res.IsError {
		t.Fatalf("%s returned tool error: %s", tool, contentText(res))
	}
	raw, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		t.Fatalf("decoding structured content: %v (%s)", err, raw)
	}
	return out
}

// writeAnchorOnlyGraph writes a graph holding just the fixture gap entry — no
// capture override — so the shipped base capture runs and the fixture ID
// resolves as an anchor.
func writeAnchorOnlyGraph(t *testing.T) string {
	t.Helper()
	graphDir := filepath.Join(t.TempDir(), "graph")
	path := filepath.Join(graphDir, "2026/06/01-100000-s-tac-aaa.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(gapEntry), 0644); err != nil {
		t.Fatal(err)
	}
	return graphDir
}

// TestCaptureLanesDedupAcrossInstances pins per-lane instruction dedup across
// capture instances in one session: the first capture serves the static lanes
// in full; a second capture's opening serve collapses them into the
// withheld-lanes line while the typeSystem lane — rendered differently now
// that the overview was served — stays fresh as the pointer.
func TestCaptureLanesDedupAcrossInstances(t *testing.T) {
	// A bare graph dir: the fixture graph plants a simplified capture override,
	// and this test must drive the shipped base capture's lanes.
	env := newTestServer(t, nil, t.TempDir(), "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var first mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &first)
	requireInstructionsContain(t, first, captureIntroProse)
	requireNoWithheldLanes(t, first)

	var second mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &second)
	requireWithheldLanes(t, second, "intro", "grounding", "topics")
	requireInstructionsExclude(t, second, captureIntroProse)
	requireInstructionsContain(t, second, basefacts.OverviewFactID, "already served")
}

// TestReportSchemaDedup pins schema dedup on canonical bytes: the first
// capture serve carries the full report schema, an identical schema on a
// second capture instance's assemble collapses to the served-earlier stub
// naming the step, and fullReplay wipes the memory so the position re-serves
// the schema in full.
func TestReportSchemaDedup(t *testing.T) {
	env := newTestServer(t, nil, t.TempDir(), "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var first mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &first)
	requireFullReportSchema(t, "first capture", first.ReportSchema)

	var second mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &second)
	requireStubReportSchema(t, "second capture", second.ReportSchema)
	if msg, _ := second.ReportSchema["served_earlier"].(string); !strings.Contains(msg, "capture/assemble") {
		t.Fatalf("the stub should name the step whose schema it stands for, got %q", msg)
	}

	var resumed mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{"fullReplay": true}, &resumed)
	fullCaptures := 0
	for _, open := range resumed.Open {
		if open.Procedure != "capture" {
			continue
		}
		if _, stub := open.ReportSchema["served_earlier"]; !stub {
			requireFullReportSchema(t, "replayed capture", open.ReportSchema)
			fullCaptures++
		}
	}
	if fullCaptures == 0 {
		t.Fatalf("fullReplay must re-serve the capture report schema in full, got %+v", resumed.Open)
	}
}

// TestAnchoredCaptureStaticLanesStillDedup pins that dedup stays per lane
// when a start input changes one lane: an anchored second capture serves its
// anchorContext lane in full while the static lanes the first capture already
// delivered stay withheld.
func TestAnchoredCaptureStaticLanesStillDedup(t *testing.T) {
	env := newTestServer(t, nil, writeAnchorOnlyGraph(t), "")
	cs := connect(t, env.srv)
	session := openSession(t, cs).Session

	var plain mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": session, "canonical": "capture"}, &plain)
	requireInstructionsContain(t, plain, captureIntroProse)
	requireNoWithheldLanes(t, plain)

	var anchored mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{
		"session": session, "canonical": "capture",
		"params": map[string]any{"anchor": fixtureGapID},
	}, &anchored)
	requireInstructionsContain(t, anchored, "anchored on "+fixtureGapID)
	requireWithheldLanes(t, anchored, "intro", "grounding", "topics")
	requireInstructionsExclude(t, anchored, captureIntroProse)
}

// TestSchemaNotRecordedOnBaseServes guards toBaseServeResult: a serve mapped
// onto base_junction carries no report schema on the wire — neither the full
// schema nor a served-earlier stub — and after base landings the schema stays
// re-servable in full. The strict poisoning flow (a base serve recording
// schema bytes this connection never received top-level, stubbing a later
// delivering serve) is unreachable through the tool surface: every attach
// path — the start_session door as much as a resume_session attach — serves
// the shell junction top-level, recording its schema, before any base landing
// can occur, and every served-memory wipe (fullReplay, a repeated
// start_session) re-serves it in the same call. So this pins the observable
// halves: the wire shape of both base landings, and the full re-serve after
// them.
func TestSchemaNotRecordedOnBaseServes(t *testing.T) {
	env := newTestServer(t, nil, t.TempDir(), "")
	cs := connect(t, env.srv)
	door := openSession(t, cs)
	requireFullReportSchema(t, "door junction", door.ReportSchema)

	var capture mcpserver.ServeResult
	call(t, cs, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &capture)

	// Park lands the dialogue on the shell junction as a base serve.
	parked := callRaw(t, cs, "park", map[string]any{"session": door.Session, "instance": capture.Instance, "note": "shelve for the base landing"})
	base, ok := parked["base_junction"].(map[string]any)
	if !ok {
		t.Fatalf("park should land on the shell junction, got %v", parked)
	}
	if schema, has := base["report_schema"]; has {
		t.Fatalf("a base_junction serve must carry no report schema, got %v", schema)
	}

	// Abandoning the move lands on the junction again — same wire contract.
	abandoned := callRaw(t, cs, "abandon", map[string]any{"session": door.Session, "instance": capture.Instance, "reason": "test teardown"})
	base, ok = abandoned["base_junction"].(map[string]any)
	if !ok {
		t.Fatalf("abandon should land on the shell junction, got %v", abandoned)
	}
	if schema, has := base["report_schema"]; has {
		t.Fatalf("an abandon base_junction serve must carry no report schema, got %v", schema)
	}

	// The base landings left the schema memory intact: a full replay serves
	// the shell junction schema in full again.
	var resumed mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{"fullReplay": true}, &resumed)
	shell := openServe(t, resumed, "user-dialogue")
	requireFullReportSchema(t, "replayed shell junction", shell.ReportSchema)
}
