package mcpapp_test

import (
	"context"
	"strings"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
	mcpserver "github.com/networkteam/sdd/pkg/mcpapp"
)

// multiAccess is a composition serving several projects to one principal. It
// carries the composition-wide session and staged-blob stores so a restarted
// server (newServerOver) finds the sessions the previous one wrote.
type multiAccess struct {
	order    []sdd.ProjectID
	runtimes map[sdd.ProjectID]*sdd.ProjectRuntime
	sessions sdd.SessionStore
	blobs    sdd.StagedBlobStore
}

func (a multiAccess) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	return sdd.Principal{Subject: identity.Subject}, nil
}

func (multiAccess) ResolveParticipant(context.Context, sdd.Principal, sdd.ProjectID) (string, error) {
	return "Tester", nil
}

func (a multiAccess) ListProjects(context.Context, sdd.Principal) (sdd.ProjectList, error) {
	var list sdd.ProjectList
	for _, id := range a.order {
		list.Projects = append(list.Projects, sdd.ProjectSummary{ProjectRef: a.runtimes[id].Project(), CanRead: true, CanWrite: true, State: sdd.ProjectReady})
	}
	return list, nil
}

func (a multiAccess) ResolveProject(_ context.Context, _ sdd.Principal, project sdd.ProjectID, _ sdd.Access) (*sdd.ProjectRuntime, error) {
	runtime := a.runtimes[project]
	if runtime == nil {
		return nil, &sdd.ApplicationError{Code: sdd.ErrorProjectUnavailable, Message: "project unavailable"}
	}
	return runtime, nil
}

func (multiAccess) AuthorizeSession(ctx context.Context, request sdd.SessionAccessRequest) error {
	return sdd.OwnerOnly(ctx, request)
}

func (multiAccess) ResolveDependency(context.Context, sdd.Principal, sdd.ProjectID, string) (*sdd.ProjectRuntime, error) {
	return nil, &sdd.ApplicationError{Code: sdd.ErrorProjectUnavailable, Message: "dependency unavailable"}
}

func newProjectRuntime(t *testing.T, id sdd.ProjectID) *sdd.ProjectRuntime {
	t.Helper()
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: id, GraphDir: writeFixtureGraph(t)})
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: id, DisplayName: "Project " + string(id)}, DefaultBranch: "main",
		Graph: graph,
		LLM: pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) {
			return pkgllm.Result{Text: `{"findings":[]}`, Identity: pkgllm.Identity{Provider: "test", Model: "test"}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

func newMultiAccess(t *testing.T, ids ...sdd.ProjectID) multiAccess {
	t.Helper()
	sessions, err := localadapter.NewFilesystemSessionStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	access := multiAccess{order: ids, runtimes: map[sdd.ProjectID]*sdd.ProjectRuntime{}, sessions: sessions, blobs: blobs}
	for _, id := range ids {
		access.runtimes[id] = newProjectRuntime(t, id)
	}
	return access
}

// newMultiProjectServer builds a wrapper over the given projects with no
// pinned Options.Project, the shape a hosted composition takes (d-tac-1z6).
func newMultiProjectServer(t *testing.T, ids ...sdd.ProjectID) *mcpserver.Server {
	t.Helper()
	return newServerOver(t, newMultiAccess(t, ids...))
}

// newServerOver hosts a fresh server over existing project runtimes — the
// restart scenario for a multi-project composition.
func newServerOver(t *testing.T, access multiAccess) *mcpserver.Server {
	t.Helper()
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	application, err := sdd.NewApplication(sdd.ApplicationOptions{
		Access: access, Sessions: access.sessions, StagedBlobs: access.blobs, Clock: sdd.ClockFunc(func() time.Time { return now }),
	})
	if err != nil {
		t.Fatal(err)
	}
	server, err := mcpserver.New(mcpserver.Options{Application: application, LocalIdentity: sdd.RequestIdentity{Subject: "tester"}, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return server
}

// TestStartSessionSelectsProject: one endpoint serves every project. With
// several accessible and none named, start_session lists them instead of
// opening; named, it opens a session bound to that project under a bare
// session-ID handle, and every other tool resolves the project from the
// session the handle names (d-cpt-yjc).
func TestStartSessionSelectsProject(t *testing.T) {
	srv := newMultiProjectServer(t, "alpha", "beta")
	cs := connect(t, srv)

	var listing mcpserver.ServeResult
	call(t, cs, "start_session", map[string]any{}, &listing)
	if listing.Status != mcpserver.StatusProjectRequired || listing.Session != "" {
		t.Fatalf("ambiguous project should list projects without opening a session, got %+v", listing)
	}
	var ids []string
	for _, project := range listing.Projects {
		ids = append(ids, project.ID)
	}
	if strings.Join(ids, ",") != "alpha,beta" {
		t.Fatalf("listing should name both projects in order, got %v", ids)
	}
	if !strings.Contains(listing.Instructions, "project") {
		t.Fatalf("listing should instruct choosing a project, got %q", listing.Instructions)
	}

	var door mcpserver.ServeResult
	call(t, cs, "start_session", map[string]any{"project": "beta"}, &door)
	if door.Status != "running" || door.Project != "beta" || !strings.HasPrefix(door.Session, "s_") {
		t.Fatalf("named project should open a session under a bare session-ID handle, got %+v", door)
	}

	var info mcpserver.InfoResult
	call(t, cs, "info", map[string]any{"session": door.Session}, &info)
	if info.Project != "beta" {
		t.Fatalf("reads should run in the session's project, got %+v", info)
	}
	var shown mcpserver.ShowResult
	call(t, cs, "show", map[string]any{"session": door.Session, "ids": []string{fixtureGapID}}, &shown)
	if !strings.Contains(shown.Entries, fixtureGapID) {
		t.Fatalf("show through the session handle should render the entry, got %q", shown.Entries)
	}

	msg := callExpectError(t, cs, "start_session", map[string]any{"project": "gamma"})
	if !strings.Contains(msg, "unavailable") {
		t.Fatalf("an inaccessible project should be refused, got %q", msg)
	}
}

// TestHandleCarriesProjectAcrossConnections: the tools that load a session
// from its store read the project from the session's own record — a fresh
// connection, and a restarted server, resume, read, and tear down sessions in
// the right project from the bare handle alone (d-cpt-yjc).
func TestHandleCarriesProjectAcrossConnections(t *testing.T) {
	access := newMultiAccess(t, "alpha", "beta")
	srv := newServerOver(t, access)
	first := connect(t, srv)
	var door mcpserver.ServeResult
	call(t, first, "start_session", map[string]any{"project": "beta", "label": "beta work"}, &door)
	var capture mcpserver.ServeResult
	call(t, first, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &capture)

	second := connect(t, srv)
	var resumed mcpserver.ResumeSessionResult
	call(t, second, "resume_session", map[string]any{"session": door.Session}, &resumed)
	if resumed.Session != door.Session || resumed.Project != "beta" {
		t.Fatalf("resume through the handle should land in beta, got %+v", resumed)
	}
	var shown mcpserver.ShowResult
	call(t, second, "show", map[string]any{"session": door.Session, "ids": []string{fixtureGapID}}, &shown)
	if !strings.Contains(shown.Entries, fixtureGapID) {
		t.Fatalf("resumed session should read its project, got %q", shown.Entries)
	}

	// A server that does not hold the session replayed loads it by ID and
	// reads the project from the session's record — no project on the
	// handle, no inference.
	restarted := newServerOver(t, access)
	call(t, connect(t, restarted), "resume_session", map[string]any{"session": door.Session}, &resumed)
	if resumed.Session != door.Session || resumed.Project != "beta" {
		t.Fatalf("resume through the handle after a restart should land in beta, got %+v", resumed)
	}

	var torn mcpserver.AbandonResult
	call(t, connect(t, restarted), "abandon", map[string]any{"session": door.Session, "reason": "test teardown"}, &torn)
	if !torn.Abandoned || torn.Session != door.Session {
		t.Fatalf("teardown by handle should name the session, got %+v", torn)
	}
}

// TestSingleProjectCompositionInfersProject: with one accessible project and
// no pinned Options.Project, start_session infers it — the local sdd serve
// shape stays a one-call door.
func TestSingleProjectCompositionInfersProject(t *testing.T) {
	srv := newMultiProjectServer(t, "solo")
	cs := connect(t, srv)
	var door mcpserver.ServeResult
	call(t, cs, "start_session", map[string]any{}, &door)
	if door.Status != "running" || door.Project != "solo" || !strings.HasPrefix(door.Session, "s_") {
		t.Fatalf("a sole project should be inferred, got %+v", door)
	}
	var resumed mcpserver.ResumeSessionResult
	call(t, connect(t, srv), "resume_session", map[string]any{"session": door.Session}, &resumed)
	if resumed.Session != door.Session || resumed.Project != "solo" {
		t.Fatalf("the handle should resume the sole project's session, got %+v", resumed)
	}
}
