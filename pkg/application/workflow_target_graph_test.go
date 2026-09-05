package application

import (
	"context"
	"errors"
	"fmt"
	"strings"
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
	err          error
	acquisitions int
	releases     int
}

func (a *workflowTargetAcquirer) Acquire(_ context.Context, target MutationTarget) (*AcquiredTarget, error) {
	a.acquisitions++
	if a.err != nil {
		return nil, a.err
	}
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
	return Principal{Subject: identity.Subject}, nil
}

func (a workflowTargetAccess) ResolveParticipant(context.Context, Principal, ProjectID) (string, error) {
	return "Christopher", nil
}
func (a workflowTargetAccess) ListProjects(context.Context, Principal) (ProjectList, error) {
	return ProjectList{}, nil
}
func (a workflowTargetAccess) ResolveProject(context.Context, Principal, ProjectID, Access) (*ProjectRuntime, error) {
	return a.runtime, nil
}
func (a workflowTargetAccess) AuthorizeSession(ctx context.Context, request SessionAccessRequest) error {
	return OwnerOnly(ctx, request)
}

func (a workflowTargetAccess) ResolveDependency(context.Context, Principal, ProjectID, string) (*ProjectRuntime, error) {
	return nil, nil
}

func TestWorkflowContextUsesBranchTargetForSummaryAndPredicates(t *testing.T) {
	const entryID = "20260717-120000-s-tac-wrk"
	const explicitID = "20260717-120001-s-tac-exp"
	base := workflowTargetSnapshot(t, "base-r1", nil)
	work := workflowTargetSnapshot(t, "work-r1", []EntryDocument{{
		LogicalPath: "2026/07/17-120000-s-tac-wrk.md",
		Frontmatter: map[string]any{
			"type": "signal", "kind": "gap", "layer": "tactical", "summary": "Stored only on the work branch.",
		},
		Body: "The target-aware workflow fixture exists only on the work branch.",
	}})
	explicit := workflowTargetSnapshot(t, "explicit-r1", []EntryDocument{{
		LogicalPath: "2026/07/17-120001-s-tac-exp.md",
		Frontmatter: map[string]any{
			"type": "signal", "kind": "gap", "layer": "tactical", "summary": "Stored only on the explicit branch.",
		},
		Body: "The explicit target must override the session binding.",
	}})
	targets := &workflowTargetAcquirer{graphs: map[string]GraphStore{
		"work":     workflowTargetGraphStore{snapshot: work},
		"explicit": workflowTargetGraphStore{snapshot: explicit},
	}}
	runtime := &ProjectRuntime{options: ProjectRuntimeOptions{
		Project: ProjectRef{ID: "example"}, DefaultBranch: "main",
		Graph: workflowTargetGraphStore{snapshot: base}, Targets: targets,
	}}
	app := &Application{access: workflowTargetAccess{runtime: runtime}}
	workflow := &WorkflowSession{
		app: app, project: "example", identity: RequestIdentity{Subject: "christopher"}, ctx: t.Context(),
		branch: "work",
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

	sessionGraph, err := graphs.CurrentFor(workflowTargetStore(t, nil))
	if err != nil {
		t.Fatal(err)
	}
	if sessionGraph.ByID[entryID] == nil {
		t.Fatal("empty procedure target did not fall back to the session binding")
	}
	explicitGraph, err := graphs.CurrentFor(workflowTargetStore(t, map[string]any{"captureBranch": "explicit"}))
	if err != nil {
		t.Fatal(err)
	}
	if explicitGraph.ByID[explicitID] == nil || explicitGraph.ByID[entryID] != nil {
		t.Fatalf("explicit captureBranch did not override binding: explicit=%v work=%v", explicitGraph.ByID[explicitID], explicitGraph.ByID[entryID])
	}
}

// Branch precedence is one rule for reads and writes: for every combination of
// session binding, explicit branch field, and configured default, both sides
// resolve the same target — an unbound read with no explicit field being the one
// deliberate difference, staying on the runtime graph while the write resolves
// the configured default (20260722-112853-d-tac-ln1).
func TestWorkflowEffectiveTargetPrecedenceIsSharedByReadsAndWrites(t *testing.T) {
	const (
		currentID  = "20260722-120000-s-tac-cur"
		mainID     = "20260722-120001-s-tac-mai"
		workID     = "20260722-120002-s-tac-wrk"
		explicitID = "20260722-120003-s-tac-exp"
	)
	branches := map[string]*Snapshot{
		"main":     workflowTargetSnapshot(t, "main-r1", []EntryDocument{workflowBranchMarker("2026/07/22-120001-s-tac-mai.md")}),
		"work":     workflowTargetSnapshot(t, "work-r1", []EntryDocument{workflowBranchMarker("2026/07/22-120002-s-tac-wrk.md")}),
		"explicit": workflowTargetSnapshot(t, "explicit-r1", []EntryDocument{workflowBranchMarker("2026/07/22-120003-s-tac-exp.md")}),
	}
	graphStores := make(map[string]GraphStore, len(branches))
	for branch, snapshot := range branches {
		graphStores[branch] = workflowTargetGraphStore{snapshot: snapshot}
	}
	current := workflowTargetSnapshot(t, "current-r1", []EntryDocument{workflowBranchMarker("2026/07/22-120000-s-tac-cur.md")})
	runtime := &ProjectRuntime{options: ProjectRuntimeOptions{
		Project: ProjectRef{ID: "example"}, DefaultBranch: "main",
		Graph: workflowTargetGraphStore{snapshot: current}, Targets: &workflowTargetAcquirer{graphs: graphStores},
	}}
	app := &Application{access: workflowTargetAccess{runtime: runtime}}

	tests := []struct {
		binding   string
		field     string
		wantRead  string
		wantWrite string
		wantEntry string
	}{
		{wantWrite: "main", wantEntry: currentID},
		{field: "captureBranch", wantRead: "explicit", wantWrite: "explicit", wantEntry: explicitID},
		{field: "resolvedCaptureBranch", wantRead: "explicit", wantWrite: "explicit", wantEntry: explicitID},
		{field: "workBranch", wantRead: "explicit", wantWrite: "explicit", wantEntry: explicitID},
		{binding: "work", wantRead: "work", wantWrite: "work", wantEntry: workID},
		{binding: "work", field: "captureBranch", wantRead: "explicit", wantWrite: "explicit", wantEntry: explicitID},
		{binding: "work", field: "resolvedCaptureBranch", wantRead: "explicit", wantWrite: "explicit", wantEntry: explicitID},
		{binding: "work", field: "workBranch", wantRead: "explicit", wantWrite: "explicit", wantEntry: explicitID},
	}
	for _, tt := range tests {
		name := fmt.Sprintf("binding=%q field=%q", tt.binding, tt.field)
		t.Run(name, func(t *testing.T) {
			workflow := &WorkflowSession{
				app: app, project: "example", identity: RequestIdentity{Subject: "christopher"}, ctx: t.Context(),
				branch: tt.binding,
			}
			graphs := &workflowGraphs{workflow: workflow}
			workflow.graphs = graphs
			store := workflowTargetStore(t, nil)
			switch tt.field {
			case "":
			case "resolvedCaptureBranch":
				if err := store.WriteEngine(tt.field, "explicit"); err != nil {
					t.Fatal(err)
				}
			default:
				if _, err := store.WriteState(map[string]any{tt.field: "explicit"}); err != nil {
					t.Fatal(err)
				}
			}

			read, fromBinding := workflow.effectiveTarget(store)
			// The project is always the instance's; only the branch varies.
			wantRead := MutationTarget{Project: "example", Branch: tt.wantRead}
			if read != wantRead {
				t.Fatalf("read target = %+v, want %+v", read, wantRead)
			}
			write, writeFromBinding, resolvedDefault, err := workflow.concreteEffectiveTarget(store)
			if err != nil {
				t.Fatal(err)
			}
			if write.Branch != tt.wantWrite || write.Project != "example" {
				t.Fatalf("write target = %+v, want branch %q", write, tt.wantWrite)
			}
			if writeFromBinding != fromBinding {
				t.Fatalf("binding provenance differs: read=%v write=%v", fromBinding, writeFromBinding)
			}
			if resolvedDefault != (read.Branch == "") {
				t.Fatalf("resolvedDefault = %v for read target %+v", resolvedDefault, read)
			}
			if read.Branch != "" && read != write {
				t.Fatalf("read target %+v and write target %+v disagree", read, write)
			}
			graph, err := graphs.CurrentFor(store)
			if err != nil {
				t.Fatal(err)
			}
			if graph.ByID[tt.wantEntry] == nil {
				t.Fatalf("read graph does not carry %s", tt.wantEntry)
			}
			if tt.wantEntry != mainID && graph.ByID[mainID] != nil {
				t.Fatal("read graph fell through to the configured default branch")
			}
		})
	}
}

func TestWorkflowGraphCacheInvalidatesAcrossRebindingAndClear(t *testing.T) {
	const (
		workEntryID     = "20260717-120000-s-tac-wrk"
		explicitEntryID = "20260717-120001-s-tac-exp"
	)
	base := workflowTargetSnapshot(t, "base-r1", nil)
	work := workflowTargetSnapshot(t, "work-r1", []EntryDocument{{
		LogicalPath: "2026/07/17-120000-s-tac-wrk.md",
		Frontmatter: map[string]any{
			"type": "signal", "kind": "gap", "layer": "tactical", "summary": "Stored only on work.",
		},
		Body: "Work branch entry.",
	}})
	explicit := workflowTargetSnapshot(t, "explicit-r1", []EntryDocument{{
		LogicalPath: "2026/07/17-120001-s-tac-exp.md",
		Frontmatter: map[string]any{
			"type": "signal", "kind": "gap", "layer": "tactical", "summary": "Stored only on explicit.",
		},
		Body: "Explicit branch entry.",
	}})
	targets := &workflowTargetAcquirer{graphs: map[string]GraphStore{
		"work":     workflowTargetGraphStore{snapshot: work},
		"explicit": workflowTargetGraphStore{snapshot: explicit},
	}}
	runtime := &ProjectRuntime{options: ProjectRuntimeOptions{
		Project: ProjectRef{ID: "example"}, DefaultBranch: "main",
		Graph: workflowTargetGraphStore{snapshot: base}, Targets: targets,
	}}
	app := &Application{access: workflowTargetAccess{runtime: runtime}}
	workflow := &WorkflowSession{
		app: app, project: "example", identity: RequestIdentity{Subject: "christopher"}, ctx: t.Context(),
		branch: "work",
	}
	graphs := &workflowGraphs{workflow: workflow}
	workflow.graphs = graphs

	graph, err := graphs.CurrentFor(workflowTargetStore(t, nil))
	if err != nil || graph.ByID[workEntryID] == nil {
		t.Fatalf("warm work graph: entry=%v err=%v", graph.ByID[workEntryID], err)
	}
	workflow.setBranch("explicit")
	graph, err = graphs.CurrentFor(workflowTargetStore(t, nil))
	if err != nil || graph.ByID[explicitEntryID] == nil || graph.ByID[workEntryID] != nil {
		t.Fatalf("rebound explicit graph: explicit=%v work=%v err=%v", graph.ByID[explicitEntryID], graph.ByID[workEntryID], err)
	}
	workflow.setBranch("")
	graph, err = graphs.CurrentFor(workflowTargetStore(t, nil))
	if err != nil || graph.ByID[workEntryID] != nil || graph.ByID[explicitEntryID] != nil {
		t.Fatalf("cleared binding graph: work=%v explicit=%v err=%v", graph.ByID[workEntryID], graph.ByID[explicitEntryID], err)
	}
}

func TestWorkflowSessionBindingDriftProvenanceOnlyForBindingTargets(t *testing.T) {
	base := workflowTargetSnapshot(t, "base-r1", nil)
	driftCause := errors.New("registered checkout disappeared")
	targets := &workflowTargetAcquirer{graphs: map[string]GraphStore{}, err: driftCause}
	runtime := &ProjectRuntime{options: ProjectRuntimeOptions{
		Project: ProjectRef{ID: "example"}, DefaultBranch: "main",
		Graph: workflowTargetGraphStore{snapshot: base}, Targets: targets,
	}}
	workflow := &WorkflowSession{
		app: &Application{access: workflowTargetAccess{runtime: runtime}}, project: "example",
		identity: RequestIdentity{Subject: "christopher"}, ctx: t.Context(), branch: "drifted",
	}

	_, err := (&workflowGraphs{workflow: workflow}).CurrentFor(workflowTargetStore(t, nil))
	if err == nil || !strings.Contains(err.Error(), `session is bound to branch "drifted"`) || !strings.Contains(err.Error(), "re-declare the binding or clear it") {
		t.Fatalf("binding drift error = %v", err)
	}
	var acquisition *targetAcquisitionError
	if !errors.As(err, &acquisition) {
		t.Fatalf("binding drift did not preserve acquisition cause: %v", err)
	}
	if !errors.Is(err, driftCause) {
		t.Fatalf("binding drift did not preserve original cause: %v", err)
	}
	if strings.Contains(err.Error(), "no longer resolves to a checkout") {
		t.Fatalf("binding drift overclaimed checkout state: %v", err)
	}

	for _, field := range []string{"captureBranch", "workBranch"} {
		_, explicitErr := (&workflowGraphs{workflow: workflow}).CurrentFor(workflowTargetStore(t, map[string]any{field: "drifted"}))
		if explicitErr == nil {
			t.Fatalf("%s drift unexpectedly succeeded", field)
		}
		if strings.Contains(explicitErr.Error(), "session is bound") {
			t.Fatalf("%s drift was mislabeled as session binding: %v", field, explicitErr)
		}
	}

	for name, readErr := range map[string]error{
		"view": func() error {
			_, err := workflow.app.View(t.Context(), workflow.identity, "example", ViewRequest{Layout: "active:as-list", Branch: "drifted"})
			return err
		}(),
		"show": func() error {
			_, err := workflow.app.Show(t.Context(), workflow.identity, "example", ShowRequest{IDs: []string{"20260717-120000-s-tac-wrk"}, Branch: "drifted"})
			return err
		}(),
		"search": func() error {
			_, err := workflow.app.Search(t.Context(), workflow.identity, "example", SearchRequest{SyncMode: SearchSyncAll, Terms: []string{"routing"}, Branch: "drifted"})
			return err
		}(),
	} {
		if readErr == nil {
			t.Fatalf("explicit free-read %s drift unexpectedly succeeded", name)
		}
		if strings.Contains(readErr.Error(), "session is bound") {
			t.Fatalf("explicit free-read %s was mislabeled as session binding: %v", name, readErr)
		}
	}

	incompleteTargets := &workflowTargetAcquirer{graphs: map[string]GraphStore{"drifted": nil}}
	incompleteRuntime := &ProjectRuntime{options: ProjectRuntimeOptions{
		Project: ProjectRef{ID: "example"}, DefaultBranch: "main",
		Graph: workflowTargetGraphStore{snapshot: base}, Targets: incompleteTargets,
	}}
	incompleteWorkflow := &WorkflowSession{
		app: &Application{access: workflowTargetAccess{runtime: incompleteRuntime}}, project: "example",
		identity: RequestIdentity{Subject: "christopher"}, ctx: t.Context(), branch: "drifted",
	}
	_, incompleteErr := (&workflowGraphs{workflow: incompleteWorkflow}).CurrentFor(workflowTargetStore(t, nil))
	if incompleteErr == nil ||
		!strings.Contains(incompleteErr.Error(), `session is bound to branch "drifted"`) ||
		!strings.Contains(incompleteErr.Error(), "target acquisition returned an incomplete runtime") ||
		strings.Contains(incompleteErr.Error(), "no longer resolves to a checkout") {
		t.Fatalf("incomplete binding target error = %v", incompleteErr)
	}
}

func TestWorkflowWIPRequiresExplicitBaseBranchBeforeCallingApplication(t *testing.T) {
	workflow := &WorkflowSession{}
	registry := engine.NewRegistry()
	if err := workflow.registerWorkflowWIP(registry); err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		command string
		values  map[string]any
	}{
		{command: "wipStart", values: map[string]any{"anchor": "20260717-120000-s-tac-wrk"}},
		{command: "wipDone", values: map[string]any{"wipMarker": "20260717-120000-christopher"}},
	}
	for _, tt := range tests {
		t.Run(tt.command, func(t *testing.T) {
			command, ok := registry.Command(tt.command)
			if !ok {
				t.Fatalf("%s command is not registered", tt.command)
			}
			err := command.Fn(&engine.Context{Store: workflowTargetStore(t, tt.values)})
			if err == nil || err.Error() != "WIP write requires an explicit baseBranch" {
				t.Fatalf("%s error = %v", tt.command, err)
			}
		})
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

func workflowBranchMarker(logicalPath string) EntryDocument {
	return EntryDocument{
		LogicalPath: logicalPath,
		Frontmatter: map[string]any{
			"type": "signal", "kind": "gap", "layer": "tactical", "summary": "Branch-exclusive routing marker.",
		},
		Body: "This marker exists only on " + logicalPath + "'s branch.",
	}
}

func workflowTargetStore(t *testing.T, values map[string]any) *engine.Store {
	t.Helper()
	entry, err := model.ParseEntry("20260717-120100-d-prc-tst.md", `---
type: decision
kind: procedure
layer: process
canonical: target-test
state:
  anchor: {type: text, optional: true}
  baseBranch: {type: text, optional: true}
  captureBranch: {type: text, optional: true}
  workBranch: {type: text, optional: true}
  wipDescription: {type: text, optional: true}
  wipMarker: {type: text, optional: true}
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
