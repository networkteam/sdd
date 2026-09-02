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

// multiAccess is a composition serving several projects to one principal.
type multiAccess struct {
	order    []sdd.ProjectID
	runtimes map[sdd.ProjectID]*sdd.ProjectRuntime
}

func (a multiAccess) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	return sdd.Principal{Subject: identity.Subject, Participant: "Tester"}, nil
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

func (multiAccess) ResolveDependency(context.Context, sdd.Principal, sdd.ProjectID, string) (*sdd.ProjectRuntime, error) {
	return nil, &sdd.ApplicationError{Code: sdd.ErrorProjectUnavailable, Message: "dependency unavailable"}
}

// testClock is the runtimes' shared clock; advancing it lets a session's
// attachment age past the activity window so teardown sees it idle.
type testClock struct{ now time.Time }

func (c *testClock) advance(d time.Duration) { c.now = c.now.Add(d) }

func newProjectRuntime(t *testing.T, id sdd.ProjectID, clock *testClock) *sdd.ProjectRuntime {
	t.Helper()
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: id, GraphDir: writeFixtureGraph(t)})
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
		Project: sdd.ProjectRef{ID: id, DisplayName: "Project " + string(id)}, DefaultBranch: "main",
		Graph: graph, Sessions: sessions, StagedBlobs: blobs, Now: func() time.Time { return clock.now },
		LLM: pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) {
			return pkgllm.Result{Text: `{"findings":[]}`, Identity: pkgllm.Identity{Provider: "test", Model: "test"}}, nil
		}),
	})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}

// newMultiProjectServer builds a wrapper over the given projects with no
// pinned Options.Project, the shape a hosted composition takes (d-tac-1z6).
func newMultiProjectServer(t *testing.T, ids ...sdd.ProjectID) (*mcpserver.Server, *testClock) {
	t.Helper()
	clock := &testClock{now: time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)}
	access := multiAccess{order: ids, runtimes: map[sdd.ProjectID]*sdd.ProjectRuntime{}}
	for _, id := range ids {
		access.runtimes[id] = newProjectRuntime(t, id, clock)
	}
	application, err := sdd.NewApplication(access)
	if err != nil {
		t.Fatal(err)
	}
	server, err := mcpserver.New(mcpserver.Options{Application: application, LocalIdentity: sdd.RequestIdentity{Subject: "tester"}, Version: "test"})
	if err != nil {
		t.Fatal(err)
	}
	return server, clock
}

// TestStartSessionSelectsProject: one endpoint serves every project. With
// several accessible and none named, start_session lists them instead of
// opening; named, it opens a session bound to that project whose handle
// carries the project, and every other tool takes project from the handle.
func TestStartSessionSelectsProject(t *testing.T) {
	srv, _ := newMultiProjectServer(t, "alpha", "beta")
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
	if door.Status != "running" || door.Project != "beta" || !strings.HasPrefix(door.Session, "beta:s_") {
		t.Fatalf("named project should open a session whose handle carries it, got %+v", door)
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

	// A handle naming another project is not this connection's session.
	msg := callExpectError(t, cs, "show", map[string]any{"session": "alpha:" + bareSessionID(door.Session), "ids": []string{fixtureGapID}})
	if !strings.Contains(msg, "not attached") {
		t.Fatalf("a handle with the wrong project must be refused, got %q", msg)
	}

	msg = callExpectError(t, cs, "start_session", map[string]any{"project": "gamma"})
	if !strings.Contains(msg, "unavailable") {
		t.Fatalf("an inaccessible project should be refused, got %q", msg)
	}
}

// TestHandleCarriesProjectAcrossConnections: the doors that load a session
// from its store take the project from the handle — a fresh connection
// resumes, lists, and tears down sessions in the right project.
func TestHandleCarriesProjectAcrossConnections(t *testing.T) {
	srv, clock := newMultiProjectServer(t, "alpha", "beta")
	first := connect(t, srv)
	var door mcpserver.ServeResult
	call(t, first, "start_session", map[string]any{"project": "beta", "label": "beta work"}, &door)
	var capture mcpserver.ServeResult
	call(t, first, "start_procedure", map[string]any{"session": door.Session, "canonical": "capture"}, &capture)

	second := connect(t, srv)
	var listed mcpserver.ListSessionsResult
	call(t, second, "list_sessions", map[string]any{}, &listed)
	if len(listed.Sessions) != 1 || listed.Sessions[0].Session != door.Session || listed.Sessions[0].Project != "beta" {
		t.Fatalf("listing across projects should carry the composed handle and project, got %+v", listed.Sessions)
	}

	var resumed mcpserver.ResumeSessionResult
	call(t, second, "resume_session", map[string]any{"session": door.Session, "userWords": "pick up the beta work", "takeover": true}, &resumed)
	if resumed.Session != door.Session || resumed.Project != "beta" {
		t.Fatalf("resume through the composed handle should land in beta, got %+v", resumed)
	}
	var shown mcpserver.ShowResult
	call(t, second, "show", map[string]any{"session": door.Session, "ids": []string{fixtureGapID}}, &shown)
	if !strings.Contains(shown.Entries, fixtureGapID) {
		t.Fatalf("resumed session should read its project, got %q", shown.Entries)
	}

	// A bare session ID has no project to load from in a multi-project
	// composition: the application reports the ambiguity.
	msg := callExpectError(t, connect(t, srv), "resume_session", map[string]any{"session": bareSessionID(door.Session), "userWords": "pick it up", "takeover": true})
	if !strings.Contains(msg, "more than one accessible project") {
		t.Fatalf("a bare handle in a multi-project composition should be rejected as ambiguous, got %q", msg)
	}

	// Once the attachment has aged past the activity window the session reads
	// idle, and a connection that never held it tears it down by handle.
	clock.advance(24 * time.Hour)
	var torn mcpserver.AbandonResult
	call(t, connect(t, srv), "abandon", map[string]any{"session": door.Session, "reason": "test teardown"}, &torn)
	if !torn.Abandoned || torn.Session != door.Session {
		t.Fatalf("teardown by composed handle should name the session, got %+v", torn)
	}
}

// TestSingleProjectCompositionInfersProject: with one accessible project and
// no pinned Options.Project, start_session infers it — the local sdd serve
// shape stays a one-call door.
func TestSingleProjectCompositionInfersProject(t *testing.T) {
	srv, _ := newMultiProjectServer(t, "solo")
	cs := connect(t, srv)
	var door mcpserver.ServeResult
	call(t, cs, "start_session", map[string]any{}, &door)
	if door.Status != "running" || door.Project != "solo" || !strings.HasPrefix(door.Session, "solo:s_") {
		t.Fatalf("a sole project should be inferred, got %+v", door)
	}
	// A bare handle resolves through the same inference at the door.
	var resumed mcpserver.ResumeSessionResult
	call(t, connect(t, srv), "resume_session", map[string]any{"session": bareSessionID(door.Session), "userWords": "continue", "takeover": true}, &resumed)
	if resumed.Session != door.Session {
		t.Fatalf("a bare handle should resume the sole project's session, got %+v", resumed)
	}
}
