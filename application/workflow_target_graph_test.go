package application

import (
	"context"
	"testing"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/model"
)

type workflowTargetGraphStore struct{ snapshot *Snapshot }

func (s workflowTargetGraphStore) Current(context.Context) (*Snapshot, error) { return s.snapshot, nil }
func (workflowTargetGraphStore) Apply(context.Context, string, MutationBatch, StagedBlobReader) (ApplyResult, error) {
	return ApplyResult{}, nil
}
func (workflowTargetGraphStore) Reconcile(context.Context, string, string) (ApplyResult, error) {
	return ApplyResult{}, nil
}
func (workflowTargetGraphStore) ReadAttachmentPage(context.Context, string, string, int64, int) (AttachmentPage, error) {
	return AttachmentPage{}, nil
}

type workflowTargetAcquirer struct {
	graphs       map[string]GraphStore
	acquisitions int
	releases     int
}

func (a *workflowTargetAcquirer) Acquire(_ context.Context, target MutationTarget) (*AcquiredTarget, error) {
	a.acquisitions++
	return &AcquiredTarget{
		Target: target,
		Graph:  a.graphs[target.Branch],
		Release: func() error {
			a.releases++
			return nil
		},
	}, nil
}

type workflowTargetAccess struct{ runtime *ProjectRuntime }

func (a workflowTargetAccess) ResolvePrincipal(_ context.Context, identity RequestIdentity) (Principal, error) {
	return Principal{Subject: identity.Subject, Participant: "Christopher"}, nil
}
func (a workflowTargetAccess) ListProjects(context.Context, Principal) (ProjectList, error) {
	return ProjectList{}, nil
}
func (a workflowTargetAccess) ResolveProject(context.Context, Principal, ProjectID, Access) (*ProjectRuntime, error) {
	return a.runtime, nil
}
func (a workflowTargetAccess) ResolveDependency(context.Context, Principal, ProjectID, string) (*ProjectRuntime, error) {
	return nil, nil
}

func TestWorkflowContextUsesBranchTargetForSummaryAndPredicates(t *testing.T) {
	const entryID = "20260717-120000-s-tac-wrk"
	base := workflowTargetSnapshot(t, "base-r1", nil)
	work := workflowTargetSnapshot(t, "work-r1", []EntryDocument{{
		LogicalPath: "2026/07/17-120000-s-tac-wrk.md",
		Frontmatter: map[string]any{
			"type": "signal", "kind": "gap", "layer": "tactical", "summary": "Stored only on the work branch.",
		},
		Body: "The target-aware workflow fixture exists only on the work branch.",
	}})
	targets := &workflowTargetAcquirer{graphs: map[string]GraphStore{
		"work": workflowTargetGraphStore{snapshot: work},
	}}
	runtime := &ProjectRuntime{options: ProjectRuntimeOptions{
		Project: ProjectRef{ID: "example"}, DefaultBranch: "main",
		Graph: workflowTargetGraphStore{snapshot: base}, Targets: targets,
	}}
	app := &Application{access: workflowTargetAccess{runtime: runtime}}
	workflow := &WorkflowSession{
		app: app, project: "example", identity: RequestIdentity{Subject: "christopher"}, ctx: t.Context(),
	}
	graphs := &workflowGraphs{workflow: workflow}
	workflow.graphs = graphs

	store := workflowTargetStore(t, map[string]any{"captureBranch": "work"})
	if err := store.WriteEngine("entryId", entryID); err != nil {
		t.Fatal(err)
	}
	graph, err := graphs.CurrentFor(store)
	if err != nil {
		t.Fatal(err)
	}
	if graph.ByID[entryID] == nil || base.graph.ByID[entryID] != nil {
		t.Fatalf("target graph entry: work=%v base=%v", graph.ByID[entryID], base.graph.ByID[entryID])
	}
	if targets.acquisitions != 1 || targets.releases != 1 {
		t.Fatalf("target lifecycle acquisitions=%d releases=%d", targets.acquisitions, targets.releases)
	}

	registry := engine.NewRegistry()
	if err := workflow.registerWorkflowQueries(registry); err != nil {
		t.Fatal(err)
	}
	query, ok := registry.Query("generatedSummary")
	if !ok {
		t.Fatal("generatedSummary query is not registered")
	}
	summary, err := query.Fn(&engine.Context{Store: store, Graph: graph}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if summary != "Stored only on the work branch." {
		t.Fatalf("generated summary = %q", summary)
	}

	workStore := workflowTargetStore(t, map[string]any{"workBranch": "work"})
	workGraph, err := graphs.CurrentFor(workStore)
	if err != nil {
		t.Fatal(err)
	}
	if workGraph.ByID[entryID] == nil {
		t.Fatal("workBranch did not resolve the work graph")
	}
	if targets.acquisitions != 1 || targets.releases != 1 {
		t.Fatalf("cached target lifecycle acquisitions=%d releases=%d", targets.acquisitions, targets.releases)
	}
}

func TestFactIndexQueryReturnsModelRowsWithoutDependencies(t *testing.T) {
	localIndex, err := model.NewFactIndex("Local cue", "cli/view")
	if err != nil {
		t.Fatal(err)
	}
	topic, err := model.ParseTopicPath("cli/view")
	if err != nil {
		t.Fatal(err)
	}
	local := model.NewGraph([]*model.Entry{{
		ID: "20260719-120000-s-tac-loc", Type: model.TypeSignal, Layer: model.LayerTactical,
		Kind: model.KindFact, Topics: []model.TopicPath{topic}, Index: localIndex,
	}})
	remoteIndex, err := model.NewFactIndex("Remote cue", "cli/view")
	if err != nil {
		t.Fatal(err)
	}
	remote := model.NewGraph([]*model.Entry{{
		ID: "20260719-120100-s-tac-rem", Type: model.TypeSignal, Layer: model.LayerTactical,
		Kind: model.KindFact, Topics: []model.TopicPath{topic}, Index: remoteIndex,
	}})
	model.NewMultiGraph(local, []string{"example/remote"}, func(string) (*model.Graph, error) { return remote, nil })

	workflow := &WorkflowSession{}
	registry := engine.NewRegistry()
	if err := workflow.registerWorkflowQueries(registry); err != nil {
		t.Fatal(err)
	}
	factIndex, ok := registry.Query("factIndex")
	if !ok || !factIndex.ServeSafe {
		t.Fatalf("factIndex registration = %+v", factIndex)
	}
	value, err := factIndex.Fn(&engine.Context{Graph: local}, nil)
	if err != nil {
		t.Fatal(err)
	}
	rows, ok := value.([]FactIndexRow)
	if !ok || len(rows) != 1 || rows[0].ID != "20260719-120000-s-tac-loc" {
		t.Fatalf("factIndex rows = %#v", value)
	}
	// The query result crosses the application boundary as plain, serializable
	// strings \u2014 Topic in its canonical slash-joined form, never a model type.
	if rows[0].Topic != "cli/view" {
		t.Fatalf("factIndex topic = %q, want canonical string form", rows[0].Topic)
	}

	// A malformed enrollment (block-injecting title) is not rejected at the
	// read boundary \u2014 it loads with a warning and is quietly omitted, so the
	// serve still succeeds and the injected content never reaches the result.
	malicious := model.NewGraph([]*model.Entry{{
		ID: "20260719-120200-s-tac-bad", Type: model.TypeSignal, Layer: model.LayerTactical,
		Kind: model.KindFact, Topics: []model.TopicPath{topic},
		Index: &model.FactIndex{Title: "Cue\u2028## Injected block", Topic: topic},
	}})
	value, err = factIndex.Fn(&engine.Context{Graph: malicious}, nil)
	if err != nil {
		t.Fatalf("factIndex on malformed enrollment errored instead of omitting: %v", err)
	}
	if rows, ok := value.([]FactIndexRow); !ok || len(rows) != 0 {
		t.Fatalf("factIndex included a malformed enrollment: %#v", value)
	}
}

func workflowTargetSnapshot(t *testing.T, revision string, entries []EntryDocument) *Snapshot {
	t.Helper()
	snapshot, err := BuildSnapshot(t.Context(), SnapshotData{
		Project: "example", Revision: revision, Entries: entries,
	})
	if err != nil {
		t.Fatal(err)
	}
	return snapshot
}

func workflowTargetStore(t *testing.T, values map[string]any) *engine.Store {
	t.Helper()
	entry, err := model.ParseEntry("20260717-120100-d-prc-tst.md", `---
type: decision
kind: procedure
layer: process
canonical: target-test
state:
  captureBranch: {type: text, optional: true}
  workBranch: {type: text, optional: true}
steps:
  - id: start
    chooser: agent
    options:
      - {choice: finish, to: end(completed)}
---

Target-aware graph test procedure.
`)
	if err != nil {
		t.Fatal(err)
	}
	spec, err := engine.LoadSpec(entry, engine.NewRegistry())
	if err != nil {
		t.Fatal(err)
	}
	store := engine.NewStore(spec)
	if err := store.SetStart(values); err != nil {
		t.Fatal(err)
	}
	return store
}
