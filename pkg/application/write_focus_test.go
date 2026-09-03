package application_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/presenters"
	"github.com/networkteam/sdd/internal/query"
	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

// End-to-end write-path coverage for the slice-2 focus kind: a focus flows
// through CreateEntry carrying involvement triples, focus-level actors, and a
// when range onto the entry; it persists, parses back with full fidelity
// (including the unset-vs-explicit-empty actors distinction), and renders
// through the existing as-focus-block view with no new rendering semantics.

// createFocusTarget writes a directive the focus involvement can resolve
// against, returning its ID and the advanced binding (each write bumps the
// binding generation — the next write must carry the fresh one).
func createFocusTarget(t *testing.T, app *sdd.Application, identity sdd.RequestIdentity, binding sdd.SessionBinding) (string, sdd.SessionBinding) {
	t.Helper()
	target, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "directive", Layer: "tactical", Intent: "pending", Confidence: "high",
		Body: "A directive the focus advances.",
	})
	if err != nil || target.EntryID == "" {
		t.Fatalf("target CreateEntry = %+v, err %v", target, err)
	}
	return target.EntryID, target.Binding
}

func TestCreateEntry_FocusPersistsInvolvementActorsAndWhen(t *testing.T) {
	app, identity, binding, dir := newIdentityWriteApp(t)
	targetID, binding := createFocusTarget(t, app, identity, binding)

	focus, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "focus", Layer: "tactical", Confidence: "high",
		Body:        "Advance the target directive this cycle — the current focus.",
		FocusActors: []string{"Christopher"},
		FocusWhen:   &model.FocusWhen{From: "2026-01-01", To: "2026-03-01"},
		Involvement: []model.Involvement{{
			Target: targetID,
			Actors: []string{"Christopher"}, ActorsSet: true,
			When: &model.FocusWhen{From: "2026-02-01"},
		}},
	})
	if err != nil || focus.EntryID == "" {
		t.Fatalf("focus CreateEntry = %+v, err %v", focus, err)
	}

	e := loadEntryByID(t, dir, focus.EntryID)
	if !e.IsFocus() {
		t.Fatalf("persisted entry is not a focus: kind=%s", e.Kind)
	}
	if len(e.FocusActors) != 1 || e.FocusActors[0] != "Christopher" {
		t.Errorf("focus actors = %v, want [Christopher]", e.FocusActors)
	}
	if e.FocusWhen == nil || e.FocusWhen.From != "2026-01-01" || e.FocusWhen.To != "2026-03-01" {
		t.Errorf("focus when = %+v, want 2026-01-01→2026-03-01", e.FocusWhen)
	}
	if len(e.Involvement) != 1 {
		t.Fatalf("involvement = %v, want one triple", e.Involvement)
	}
	inv := e.Involvement[0]
	if inv.Target != targetID {
		t.Errorf("involvement target = %q, want %q", inv.Target, targetID)
	}
	if !inv.ActorsSet || len(inv.Actors) != 1 || inv.Actors[0] != "Christopher" {
		t.Errorf("involvement actors = %v (set %v), want [Christopher]", inv.Actors, inv.ActorsSet)
	}
	if inv.When == nil || inv.When.From != "2026-02-01" {
		t.Errorf("involvement when = %+v, want from 2026-02-01", inv.When)
	}
}

func TestCreateEntry_FocusPreservesActorsSetDistinction(t *testing.T) {
	app, identity, binding, dir := newIdentityWriteApp(t)
	targetID, binding := createFocusTarget(t, app, identity, binding)

	focus, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "focus", Layer: "tactical", Confidence: "high",
		Body: "A focus spanning two targets with different actor postures.",
		Involvement: []model.Involvement{
			// Unset: inherits the focus-level default (no actors here → nil).
			{Target: targetID},
			// Explicit empty: deliberately unattributed / pull-available.
			{Target: targetID, Actors: []string{}, ActorsSet: true},
		},
	})
	if err != nil || focus.EntryID == "" {
		t.Fatalf("focus CreateEntry = %+v, err %v", focus, err)
	}

	e := loadEntryByID(t, dir, focus.EntryID)
	if len(e.Involvement) != 2 {
		t.Fatalf("involvement = %v, want two triples", e.Involvement)
	}
	if e.Involvement[0].ActorsSet {
		t.Errorf("first involvement should be unset (inherit), got ActorsSet=true actors=%v", e.Involvement[0].Actors)
	}
	if !e.Involvement[1].ActorsSet {
		t.Errorf("second involvement should be explicit-empty (pull-available), got ActorsSet=false")
	}
	if len(e.Involvement[1].Actors) != 0 {
		t.Errorf("second involvement actors = %v, want explicit empty", e.Involvement[1].Actors)
	}
}

func TestCreateEntry_FocusRendersThroughAsFocusBlock(t *testing.T) {
	app, identity, binding, dir := newIdentityWriteApp(t)
	targetID, binding := createFocusTarget(t, app, identity, binding)

	focus, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "focus", Layer: "tactical", Confidence: "high",
		Body:        "Advance the target directive — rendered through the existing focus block.",
		FocusWhen:   &model.FocusWhen{From: "2026-01-01", To: "2026-03-01"},
		Involvement: []model.Involvement{{Target: targetID}},
	})
	if err != nil || focus.EntryID == "" {
		t.Fatalf("focus CreateEntry = %+v, err %v", focus, err)
	}

	// Build a graph from the two engine-written entries and render it through
	// the existing view pipeline — no focus-specific presenter code is added by
	// this slice; the write path just has to feed the established surface.
	g := model.NewGraph([]*model.Entry{
		loadEntryByID(t, dir, targetID),
		loadEntryByID(t, dir, focus.EntryID),
	})
	layout, err := query.ParseLayout("kind(focus):active:expand(involvement):as-focus-block")
	if err != nil {
		t.Fatal(err)
	}
	result, err := finders.New(finders.Options{}).OnGraph(g).View(query.ViewQuery{Layout: layout})
	if err != nil {
		t.Fatal(err)
	}
	var buf bytes.Buffer
	presenters.RenderView(&buf, result)
	out := buf.String()
	if !strings.Contains(out, focus.EntryID) {
		t.Errorf("focus block should render the focus, got %q", out)
	}
	if !strings.Contains(out, targetID) {
		t.Errorf("focus block should render the involvement target, got %q", out)
	}
	if !strings.Contains(out, "2026-01-01") {
		t.Errorf("focus block should render the focus when, got %q", out)
	}
}

// TestWorkflowCaptureFocusPersistsThroughRealNewEntry drives a focus draft
// through the real capture procedure over the workflow engine — report,
// playback confirm, summary verify — so the store→model type assertions in
// runWorkflowNewEntry that read involvement/focusActors/focusWhen off the engine
// store are exercised for real, not just proven by the compiler. A wrong
// assertion would silently drop the optional focus fields; this catches it.
func TestWorkflowCaptureFocusPersistsThroughRealNewEntry(t *testing.T) {
	const targetID = "20260717-120000-d-tac-tgt"
	dir := t.TempDir()
	writeFocusWorkflowTarget(t, dir, targetID)

	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: dir})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := localadapter.NewFilesystemSessionStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main", Graph: graph,
		LLM: pkgllm.RunnerFunc(func(_ context.Context, request pkgllm.Request) (pkgllm.Result, error) {
			identity := pkgllm.Identity{Provider: "test", Model: "test"}
			if request.Purpose == pkgllm.PurposePreflight || request.Purpose == pkgllm.PurposeWritingGuide {
				return pkgllm.Result{Text: `{"findings":[]}`, Identity: identity}, nil
			}
			return pkgllm.Result{Text: "A focus advancing the target directive this cycle.", Identity: identity}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{Access: &runtimeAccessResolver{runtime: runtime}, Sessions: sessions, StagedBlobs: blobs})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher"}
	workflow, _, err := application.OpenWorkflow(t.Context(), identity, "example", sdd.WorkflowOpenRequest{ClientName: "focus-workflow"})
	if err != nil {
		t.Fatal(err)
	}

	started, err := workflow.Start(t.Context(), identity, sdd.WorkflowStartRequest{Canonical: "capture"})
	if err != nil {
		t.Fatal(err)
	}
	instance := started.Instance

	serve := advanceWorkflow(t, workflow, identity, instance, map[string]any{
		"body":        "Advance the target directive to done this cycle — the current focus.",
		"entryKind":   "focus",
		"layer":       "tactical",
		"confidence":  "high",
		"widenReport": "checked the graph for an active focus over this target before drafting",
		"focusActors": []any{"Christopher"},
		"focusWhen":   map[string]any{"from": "2026-01-01", "to": "2026-03-01"},
		"involvement": []any{
			// Actors omitted: inherits the focus-level default.
			map[string]any{"target": targetID},
			// Explicit empty: deliberately unattributed / pull-available.
			map[string]any{"target": targetID, "actors": []any{}},
		},
	})
	if serve.Step != "playback" {
		t.Fatalf("focus report should reach playback, got %q", serve.Step)
	}

	serve = advanceWorkflow(t, workflow, identity, instance, map[string]any{
		"chooser": "playback", "choice": "confirm", "userWords": "confirm",
	})
	if serve.Step != "verifySummary" {
		t.Fatalf("focus confirm should reach verifySummary, got %q", serve.Step)
	}

	serve = advanceWorkflow(t, workflow, identity, instance, map[string]any{
		"chooser": "verifySummary", "choice": "faithful", "fields": map[string]any{"fidelityNote": "faithful"},
	})
	focusID, _ := serve.Produced["entryId"].(string)
	if focusID == "" {
		t.Fatalf("capture produced no entryId: %+v", serve.Produced)
	}

	e := loadEntryByID(t, dir, focusID)
	if !e.IsFocus() {
		t.Fatalf("persisted entry is not a focus: kind=%s", e.Kind)
	}
	// The optional focus-level fields must survive the store→model seam — a
	// wrong type assertion in runWorkflowNewEntry would drop them silently.
	if len(e.FocusActors) != 1 || e.FocusActors[0] != "Christopher" {
		t.Errorf("focus actors = %v, want [Christopher]", e.FocusActors)
	}
	if e.FocusWhen == nil || e.FocusWhen.From != "2026-01-01" || e.FocusWhen.To != "2026-03-01" {
		t.Errorf("focus when = %+v, want 2026-01-01→2026-03-01", e.FocusWhen)
	}
	if len(e.Involvement) != 2 {
		t.Fatalf("involvement = %v, want two triples", e.Involvement)
	}
	if e.Involvement[0].ActorsSet {
		t.Errorf("first involvement should inherit (unset actors), got ActorsSet=true")
	}
	if !e.Involvement[1].ActorsSet || len(e.Involvement[1].Actors) != 0 {
		t.Errorf("second involvement should be explicit-empty (pull-available), got set=%v actors=%v", e.Involvement[1].ActorsSet, e.Involvement[1].Actors)
	}
}

func writeFocusWorkflowTarget(t *testing.T, dir, id string) {
	t.Helper()
	rel, err := model.IDToRelPath(id)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntype: decision\nkind: directive\nlayer: tactical\nintent: pending\nsummary: A directive the focus advances.\n---\n\nThe target directive a focus involvement resolves against.\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
