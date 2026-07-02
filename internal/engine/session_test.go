package engine

import (
	"bytes"
	"strings"
	"testing"
)

// replayEnv runs a script against a JSONL buffer, then replays the log into
// a fresh session on a fresh engine (fresh fake registry too, so command
// call counters prove replay never re-runs side effects).
func TestSession_SurvivesRestartByLogReplay(t *testing.T) {
	env := newFixtureEnv(t)
	var log bytes.Buffer
	env.session.sink = NewWriterSink(&log)

	// Drive to the pending playback chooser, mid-procedure.
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.session.Report(sv.Instance, fullDraft()); err != nil {
		t.Fatal(err)
	}

	// A second instance that already completed, to prove per-instance
	// terminal state replays.
	sv2, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.session.Report(sv2.Instance, fullDraft()); err != nil {
		t.Fatal(err)
	}
	if _, err := env.session.Answer(sv2.Instance, "playback", "confirm", nil, "yes"); err != nil {
		t.Fatal(err)
	}
	if _, err := env.session.Answer(sv2.Instance, "verifySummary", "faithful",
		map[string]any{"fidelityNote": "fine"}, ""); err != nil {
		t.Fatal(err)
	}
	preCalls := env.newCalls

	// Restart: fresh engine + registry (fresh counters), replay the log.
	env2 := newFixtureEnv(t)
	events, err := ReadEvents(bytes.NewReader(log.Bytes()))
	if err != nil {
		t.Fatal(err)
	}
	resolver := func(canonical string) (*Spec, error) { return env2.spec, nil }
	var log2 bytes.Buffer
	replayed, err := env2.engine.ReplaySession("s_test", "christopher", events, resolver, NewWriterSink(&log2))
	if err != nil {
		t.Fatal(err)
	}

	// Replay is a fold, never a re-run: no command executed.
	if env2.newCalls != 0 {
		t.Fatalf("replay re-ran newEntry %d times — replay must apply logged results only", env2.newCalls)
	}

	// Instance 1: back at the pending playback chooser with its state.
	i1, ok := replayed.Instance(sv.Instance)
	if !ok {
		t.Fatal("instance 1 missing after replay")
	}
	if i1.Step != "playback" || i1.Status != StatusRunning {
		t.Fatalf("i1 = %s/%s, want playback/running", i1.Step, i1.Status)
	}
	if body, _ := i1.Store.Get("body"); body != fullDraft()["body"] {
		t.Errorf("i1 body = %v, want the drafted body", body)
	}

	// Instance 2: completed, with the engine-written entry ID restored.
	i2, ok := replayed.Instance(sv2.Instance)
	if !ok {
		t.Fatal("instance 2 missing after replay")
	}
	if i2.Status != StatusCompleted {
		t.Fatalf("i2 status = %s, want completed", i2.Status)
	}
	if id, _ := i2.Store.Get("entryId"); id != "20260702-130001-s-tac-new" {
		t.Errorf("i2 entryId = %v, want the logged one", id)
	}

	// The replayed session continues: confirm instance 1 through to
	// completion — the confirmation binds to the replayed state.
	svc, err := replayed.Answer(sv.Instance, "playback", "confirm", nil, "resume and capture")
	if err != nil {
		t.Fatal(err)
	}
	if svc.Step != "verifySummary" {
		t.Fatalf("resumed confirm step = %s, want verifySummary", svc.Step)
	}
	if env2.newCalls != 1 {
		t.Fatalf("resumed run should execute newEntry once, got %d", env2.newCalls)
	}
	_ = preCalls

	// New instances allocate fresh handles after the replayed ones.
	sv3, err := replayed.Start(env2.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if sv3.Instance != "i_3" {
		t.Errorf("post-replay instance handle = %s, want i_3", sv3.Instance)
	}
}

func TestReplay_RejectsUnknownLogVersion(t *testing.T) {
	env := newFixtureEnv(t)
	events := []Event{{V: 99, Seq: 1, Event: EventStarted, Instance: "i_1"}}
	_, err := env.engine.ReplaySession("s", "p", events, func(string) (*Spec, error) { return env.spec, nil }, nil)
	if err == nil || !strings.Contains(err.Error(), "version") {
		t.Fatalf("unknown log version must be rejected, got %v", err)
	}
}

func TestReplay_RejectsProcedureChangedUnderneath(t *testing.T) {
	env := newFixtureEnv(t)
	var log bytes.Buffer
	env.session.sink = NewWriterSink(&log)
	if _, err := env.session.Start(env.spec, nil, ""); err != nil {
		t.Fatal(err)
	}
	events, err := ReadEvents(bytes.NewReader(log.Bytes()))
	if err != nil {
		t.Fatal(err)
	}

	env2 := newFixtureEnv(t)
	other := *env2.spec
	other.EntryID = "20260703-000000-d-prc-new"
	_, err = env2.engine.ReplaySession("s_test", "christopher", events,
		func(string) (*Spec, error) { return &other, nil }, nil)
	if err == nil || !strings.Contains(err.Error(), "changed underneath") {
		t.Fatalf("entry-ID mismatch must be rejected, got %v", err)
	}
}

func TestReplay_RejectsTamperedStateValues(t *testing.T) {
	env := newFixtureEnv(t)
	var log bytes.Buffer
	env.session.sink = NewWriterSink(&log)
	sv, err := env.session.Start(env.spec, nil, "")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := env.session.Report(sv.Instance, fullDraft()); err != nil {
		t.Fatal(err)
	}
	// Tamper: rewrite a logged report value into a malformed ref list.
	tampered := strings.Replace(log.String(),
		`"kind":"addresses"`, `"kind":"invented-kind"`, 1)
	events, err := ReadEvents(strings.NewReader(tampered))
	if err != nil {
		t.Fatal(err)
	}
	env2 := newFixtureEnv(t)
	_, err = env2.engine.ReplaySession("s_test", "christopher", events,
		func(string) (*Spec, error) { return env2.spec, nil }, nil)
	if err == nil || !strings.Contains(err.Error(), "ref-kind") {
		t.Fatalf("tampered state must fail type validation on replay, got %v", err)
	}
}

func TestWriterSinkAndReadEvents_RoundTrip(t *testing.T) {
	var buf bytes.Buffer
	sink := NewWriterSink(&buf)
	for i := 1; i <= 3; i++ {
		if err := sink.Append(Event{V: 1, Session: "s", Seq: i, Event: EventServed,
			Instance: "i_1", Data: map[string]any{"step": "assemble"}}); err != nil {
			t.Fatal(err)
		}
	}
	events, err := ReadEvents(&buf)
	if err != nil {
		t.Fatal(err)
	}
	if len(events) != 3 || events[2].Seq != 3 || events[0].Data["step"] != "assemble" {
		t.Fatalf("round trip = %+v", events)
	}
}
