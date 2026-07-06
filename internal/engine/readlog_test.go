package engine

import (
	"bytes"
	"strings"
	"testing"
)

// jsonRoundTripEvents serializes events through the writer sink and parses
// them back — replay sees what a real JSONL log would carry, not in-memory
// typed values.
func jsonRoundTripEvents(t *testing.T, events []Event) []Event {
	t.Helper()
	var buf bytes.Buffer
	sink := NewWriterSink(&buf)
	for _, ev := range events {
		if err := sink.Append(ev); err != nil {
			t.Fatal(err)
		}
	}
	parsed, err := ReadEvents(&buf)
	if err != nil {
		t.Fatal(err)
	}
	return parsed
}

func TestReadLog_FoldsDepthAndLogsEvents(t *testing.T) {
	env := newFixtureEnv(t)
	base := len(env.sink.events)

	env.session.LogRead("search", nil, []string{"a", "b", "a", ""})
	if got := env.session.ReadDepthOf("a"); got != ReadSummary {
		t.Fatalf("depth(a) = %q, want summary", got)
	}

	// Full wins over summary and never downgrades.
	env.session.LogRead("show", []string{"a"}, nil)
	if got := env.session.ReadDepthOf("a"); got != ReadFull {
		t.Fatalf("depth(a) = %q, want full", got)
	}
	env.session.LogRead("search", nil, []string{"a"})
	if got := env.session.ReadDepthOf("a"); got != ReadFull {
		t.Fatalf("depth(a) after summary re-read = %q, want full (no downgrade)", got)
	}

	// Empty calls append nothing; real calls append one event each.
	env.session.LogRead("search", nil, nil)
	events := env.sink.events[base:]
	if len(events) != 3 {
		t.Fatalf("read events appended = %d, want 3", len(events))
	}
	for _, ev := range events {
		if ev.Event != EventRead {
			t.Fatalf("event = %s, want read", ev.Event)
		}
		if ev.Instance != "" {
			t.Fatalf("read events are session-level, got instance %q", ev.Instance)
		}
	}
	// Payloads are IDs and metadata only, deduplicated.
	if ids, _ := events[0].Data["summary"].([]string); len(ids) != 2 {
		t.Fatalf("first read event summary IDs = %v, want [a b]", events[0].Data["summary"])
	}
}

func TestReadLog_SurvivesReplay(t *testing.T) {
	env := newFixtureEnv(t)
	env.session.LogRead("show", []string{fixtureRef2ID}, []string{"only-summary"})

	roundTripped := jsonRoundTripEvents(t, env.sink.events)
	resolve := func(string) (*Spec, error) { return env.spec, nil }
	restored, err := env.engine.ReplaySession("s_test", "christopher", roundTripped, resolve, &memorySink{})
	if err != nil {
		t.Fatal(err)
	}
	if got := restored.ReadDepthOf(fixtureRef2ID); got != ReadFull {
		t.Fatalf("replayed depth = %q, want full", got)
	}
	if got := restored.ReadDepthOf("only-summary"); got != ReadSummary {
		t.Fatalf("replayed depth = %q, want summary", got)
	}
}

func TestCapture_RefsInspectedGateHoldsAndNamesIDs(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}

	// Draft refs an entry the session never read in full — the gate holds and
	// the rejection names exactly the un-inspected ID.
	draft := fullDraft()
	draft["refs"] = []any{map[string]any{"id": fixtureRef2ID, "kind": "addresses"}}
	sv, err = env.session.Report(sv.Instance, draft)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assemble" {
		t.Fatalf("step = %s, want assemble (held by refsInspected)", sv.Step)
	}
	var held *FailedPredicate
	for i := range sv.Failing {
		if sv.Failing[i].Name == "refsInspected" {
			held = &sv.Failing[i]
		}
	}
	if held == nil {
		t.Fatalf("failing = %+v, want refsInspected among them", sv.Failing)
	}
	if !strings.Contains(held.Message, fixtureRef2ID) {
		t.Fatalf("rejection must name the un-inspected ID, got %q", held.Message)
	}

	// A summary serve (search header) is not inspection.
	env.session.LogRead("search", nil, []string{fixtureRef2ID})
	sv, err = env.session.Report(sv.Instance, map[string]any{"widenReport": "searched, headers only"})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assemble" {
		t.Fatalf("step = %s, want assemble (summary depth must not pass)", sv.Step)
	}

	// The agent reads the entry through the same free tools; the gate passes
	// on re-report.
	env.session.LogRead("show", []string{fixtureRef2ID}, nil)
	sv, err = env.session.Report(sv.Instance, map[string]any{"widenReport": "searched, then inspected s-tac-raw in full"})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("step = %s, want playback (gate passes after the show)", sv.Step)
	}
}

func TestCapture_RefsInspectedCoversSupersedesAndCloses(t *testing.T) {
	env := newFixtureEnv(t)
	sv, err := env.session.Start(env.spec, map[string]any{"closes": []any{fixtureRef2ID}}, "")
	if err != nil {
		t.Fatal(err)
	}
	sv, err = env.session.Report(sv.Instance, fullDraft())
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "assemble" {
		t.Fatalf("step = %s, want assemble (un-inspected closes target must hold the gate)", sv.Step)
	}

	env.session.LogRead("show", []string{fixtureRef2ID}, nil)
	sv, err = env.session.Report(sv.Instance, map[string]any{"widenReport": "inspected the closes target in full"})
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("step = %s, want playback", sv.Step)
	}
}

func TestCapture_PriorSessionReadsCountForLaterMoves(t *testing.T) {
	// The read set is session-level: what a dispatching move inspected counts
	// for the capture it seeds — no re-show ceremony (d-tac-dbk).
	env := newFixtureEnv(t)
	env.session.LogRead("inject:entryChains", []string{fixtureRef2ID}, nil)

	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	draft := fullDraft()
	draft["refs"] = []any{map[string]any{"id": fixtureRef2ID, "kind": "addresses"}}
	sv, err = env.session.Report(sv.Instance, draft)
	if err != nil {
		t.Fatal(err)
	}
	if sv.Step != "playback" {
		t.Fatalf("step = %s, want playback (parent-session reads count)", sv.Step)
	}
}
