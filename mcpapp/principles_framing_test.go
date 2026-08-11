package mcpapp_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	mcpserver "github.com/networkteam/sdd/mcpapp"
)

// principlesFactID is the shipped working-principles base fact — the words a
// session is primed with. Spelled out here rather than imported so a change to
// the stable ID shows up as a failing expectation on the serving side too.
const principlesFactID = "20260810-190000-s-prc-way"

// TestDoorServesWorkingPrinciplesFirst pins the primed-rather-than-pulled
// position (d-tac-41n AC1): the principles serve in full at session open, ahead
// of the framing lanes that carry the graph's own structure — the counter-balance
// arrives before the structure starts to bias.
func TestDoorServesWorkingPrinciplesFirst(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	door := openSession(t, cs)

	principles := strings.Index(door.Framing, "## Working principles")
	if principles < 0 {
		t.Fatalf("the door must serve the working principles, got %q", door.Framing)
	}
	for _, want := range []string{"The way of thinking", "Goal first", "Expect novelty", "step up"} {
		if !strings.Contains(door.Framing, want) {
			t.Errorf("the principles must serve in full, missing %q", want)
		}
	}
	info := strings.Index(door.Framing, "Local participant:")
	if info < 0 || info > principles {
		t.Errorf("the engine's info block comes first, then the principles (info at %d, principles at %d)", info, principles)
	}
	for _, lane := range []string{"Guiding directives", "Recent graph movement"} {
		if at := strings.Index(door.Framing, lane); at >= 0 && at < principles {
			t.Errorf("lane %q (at %d) must not precede the principles (at %d)", lane, at, principles)
		}
	}
	if !strings.Contains(door.Framing, principlesFactID) {
		t.Errorf("the served principles must name their entry, so a reader can pull or supersede it: %q", door.Framing)
	}
}

// TestProjectFactOverridesServedPrinciples pins the single-home property
// (d-tac-41n AC4): a project fact superseding the base fact changes the served
// words through ordinary graph resolution, with no code change and no shell edit.
func TestProjectFactOverridesServedPrinciples(t *testing.T) {
	graphDir := writeFixtureGraph(t)
	path := filepath.Join(graphDir, "2026/08/11-090000-s-prc-own.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	fact := `---
type: signal
layer: process
kind: fact
confidence: high
topics:
    - principles/interactive
supersedes:
    - ` + principlesFactID + `
summary: This project's own working principles replace the shipped posture.
---

# House rules

**Ask before assuming.** This project's own posture, authored locally.
`
	if err := os.WriteFile(path, []byte(fact), 0644); err != nil {
		t.Fatal(err)
	}

	env := newTestServer(t, nil, graphDir, "")
	cs := connect(t, env.srv)
	door := openSession(t, cs)

	if !strings.Contains(door.Framing, "House rules") || !strings.Contains(door.Framing, "Ask before assuming") {
		t.Fatalf("the project fact's words must serve, got %q", door.Framing)
	}
	if strings.Contains(door.Framing, "The way of thinking") {
		t.Errorf("the superseded base posture must not serve alongside its successor, got %q", door.Framing)
	}
}

// TestSupersedingPrinciplesMidSessionReservesTheLane pins the re-entry half
// (d-tac-41n AC3): with nothing changed a reorient stubs the lane, and once the
// principles are superseded mid-session the changed lane re-serves on its own.
func TestSupersedingPrinciplesMidSessionReservesTheLane(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	cs := connect(t, env.srv)
	door := openSession(t, cs)
	if !strings.Contains(door.Framing, "## Working principles") {
		t.Fatalf("precondition: the door serves the principles, got %q", door.Framing)
	}

	var converged mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{}, &converged)
	if strings.Contains(converged.Framing, "## Working principles") {
		t.Fatalf("an unchanged principles lane must stub like any other, got %q", converged.Framing)
	}

	path := filepath.Join(env.graphDir, "2026/08/11-090000-s-prc-own.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	fact := `---
type: signal
layer: process
kind: fact
confidence: high
topics:
    - principles/interactive
supersedes:
    - ` + principlesFactID + `
summary: A locally superseded posture, written mid-session.
---

# House rules

**Ask before assuming.** Authored while the session was open.
`
	if err := os.WriteFile(path, []byte(fact), 0644); err != nil {
		t.Fatal(err)
	}

	var afterWrite mcpserver.ResumeSessionResult
	call(t, cs, "resume_session", map[string]any{}, &afterWrite)
	if !strings.Contains(afterWrite.Framing, "House rules") {
		t.Fatalf("the changed principles lane must re-serve with the new words, got %q", afterWrite.Framing)
	}
	if strings.Contains(afterWrite.Framing, "Local participant:") {
		t.Errorf("the unchanged info block must stay stubbed (per-lane dedup), got %q", afterWrite.Framing)
	}
}

// TestBareShellServesNoPrinciples pins the shell-owns-selection half
// (d-tac-41n AC5): priming is declared per shell, so a shell that declares no
// principles lane is primed with nothing, however many principles facts exist.
func TestBareShellServesNoPrinciples(t *testing.T) {
	graphDir := filepath.Join(t.TempDir(), "graph")
	path := filepath.Join(graphDir, "2026/07/04-130000-d-prc-bare.md")
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(bareShellProcedure), 0644); err != nil {
		t.Fatal(err)
	}
	env := newTestServer(t, nil, graphDir, "")
	cs := connect(t, env.srv)
	var serve mcpserver.ServeResult
	call(t, cs, "start_session", map[string]any{"shell": "bare-shell"}, &serve)
	if strings.Contains(serve.Framing, "Working principles") || strings.Contains(serve.Framing, "The way of thinking") {
		t.Fatalf("a shell declaring no principles lane must serve none, got %q", serve.Framing)
	}
}

// TestServedPrinciplesAreStableAcrossOpens pins deterministic order (d-tac-41n
// AC8): repeated opens of the same graph serve byte-identical principles, which
// is also what lets the per-lane dedup converge instead of re-serving forever.
func TestServedPrinciplesAreStableAcrossOpens(t *testing.T) {
	env := newTestServer(t, nil, "", "")
	first := laneOf(t, openSession(t, connect(t, env.srv)).Framing)
	for range 3 {
		if got := laneOf(t, openSession(t, connect(t, env.srv)).Framing); got != first {
			t.Fatalf("principles lane differs across opens:\nfirst:\n%s\nlater:\n%s", first, got)
		}
	}
}

// laneOf isolates the principles lane out of a composed framing block: from its
// header up to the blank-line boundary before the next lane's header.
func laneOf(t *testing.T, framing string) string {
	t.Helper()
	start := strings.Index(framing, "## Working principles")
	if start < 0 {
		t.Fatalf("framing carries no principles lane: %q", framing)
	}
	lane, _, _ := strings.Cut(framing[start:], "\n\n## Aspirations")
	return lane
}
