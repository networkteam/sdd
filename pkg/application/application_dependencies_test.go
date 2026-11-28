package application_test

import (
	"context"
	"errors"
	"io"
	"path/filepath"
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/application/types"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

type staticGraphStore struct {
	snapshot   *sdd.Snapshot
	attachment string
}

func (s staticGraphStore) Current(context.Context) (*sdd.Snapshot, error) { return s.snapshot, nil }
func (s staticGraphStore) ReadAttachmentPage(_ context.Context, _ string, filename string, offset int64, limit int) (sdd.AttachmentPage, error) {
	content := []byte(s.attachment)
	end := int(offset) + limit
	if end > len(content) {
		end = len(content)
	}
	return sdd.AttachmentPage{Filename: filename, Content: content[offset:end], Offset: offset, NextOffset: int64(end), TotalSize: int64(len(content)), More: end < len(content)}, nil
}

type noBlobStore struct{}

func (noBlobStore) Stage(context.Context, sdd.SessionRef, string, io.Reader) (sdd.StagedBlob, error) {
	return sdd.StagedBlob{}, nil
}
func (noBlobStore) Open(context.Context, sdd.SessionRef, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (noBlobStore) DeleteStaged(context.Context, sdd.SessionRef) error { return nil }

type noSessionStore struct{}

func (noSessionStore) Create(context.Context, sdd.SessionMetadata) (sdd.StoredSession, error) {
	return sdd.StoredSession{}, nil
}
func (noSessionStore) Load(context.Context, sdd.SessionID) (sdd.StoredSession, error) {
	return sdd.StoredSession{}, nil
}
func (noSessionStore) List(context.Context, sdd.SessionFilter) (sdd.SessionPage, error) {
	return sdd.SessionPage{}, nil
}
func (noSessionStore) Append(context.Context, sdd.SessionID, uint64, sdd.SessionAppend) (uint64, error) {
	return 0, nil
}
func (noSessionStore) Delete(context.Context, sdd.SessionID) error { return nil }

type multiAccessResolver struct {
	base       *sdd.ProjectRuntime
	dependency *sdd.ProjectRuntime
	deny       bool
}

func (r *multiAccessResolver) ResolvePrincipal(_ context.Context, identity sdd.RequestIdentity) (sdd.Principal, error) {
	return sdd.Principal{Subject: identity.Subject}, nil
}
func (r *multiAccessResolver) ResolveParticipant(context.Context, sdd.Principal, sdd.ProjectID) (string, error) {
	return "Christopher", nil
}
func (r *multiAccessResolver) ListProjects(context.Context, sdd.Principal) (sdd.ProjectList, error) {
	return sdd.ProjectList{Projects: []sdd.ProjectSummary{{ProjectRef: r.base.Project(), CanRead: true, State: sdd.ProjectReady}}}, nil
}
func (r *multiAccessResolver) ResolveProject(context.Context, sdd.Principal, sdd.ProjectID, sdd.Access) (*sdd.ProjectRuntime, error) {
	return r.base, nil
}
func (r *multiAccessResolver) AuthorizeSession(ctx context.Context, request sdd.SessionAccessRequest) error {
	return sdd.OwnerOnly(ctx, request)
}

func (r *multiAccessResolver) ResolveDependency(context.Context, sdd.Principal, sdd.ProjectID, sdd.RepoID) (*sdd.ProjectRuntime, error) {
	if r.deny {
		return nil, &sdd.ApplicationError{Code: sdd.ErrorReadDenied, Message: "secret policy detail"}
	}
	return r.dependency, nil
}

// TestApplicationLoadsDependencyPartiallyWithUnreadableEntry is the dependency
// graceful-degradation proof: a connected repo whose graph carries one
// undecodable document no longer collapses the whole dependency to
// "unavailable" (application.go snapshotWithDependenciesFrom). BuildSnapshot
// loads the parseable entries and records the failure as a load issue, so a
// cross-repo Show of a valid foreign entry still resolves.
func TestApplicationLoadsDependencyPartiallyWithUnreadableEntry(t *testing.T) {
	baseSnapshot, err := sdd.BuildSnapshot(t.Context(), sdd.SnapshotData{Project: "base", Revision: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	foreignSnapshot, err := sdd.BuildSnapshot(t.Context(), sdd.SnapshotData{
		Project: "example.org/dep", Revision: "r1",
		Entries: []sdd.EntryDocument{{
			LogicalPath: "2026/07/13-040000-s-tac-dep.md",
			Frontmatter: map[string]any{"type": "signal", "kind": "gap", "layer": "tactical", "summary": "Foreign dependency fixture."},
			Body:        "The authorized dependency body is visible through the base application.",
		}},
		// A file the store could not decode — carried as data, not an abort.
		Unreadable: []sdd.DocumentIssue{{LogicalPath: "2026/07/13-050000-s-tac-bad.md", Message: "yaml: unterminated flow sequence"}},
	})
	if err != nil {
		t.Fatalf("BuildSnapshot must not abort on an unreadable document: %v", err)
	}
	if health := foreignSnapshot.Health(); health.LoadErrors != 1 {
		t.Fatalf("dependency snapshot Health should report 1 load error, got %+v", health)
	}

	llm := pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) {
		return pkgllm.Result{Identity: pkgllm.Identity{Provider: "test", Model: "test"}}, nil
	})
	dependency, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example.org/dep"}, Graph: staticGraphStore{snapshot: foreignSnapshot},
		LLM: llm,
	})
	if err != nil {
		t.Fatal(err)
	}
	base, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "base"}, Dependencies: []sdd.RepoID{"example.org/dep"}, Graph: staticGraphStore{snapshot: baseSnapshot},
		LLM: llm,
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{Access: &multiAccessResolver{base: base, dependency: dependency}, Sessions: noSessionStore{}, StagedBlobs: noBlobStore{}})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher"}
	show, err := application.Show(t.Context(), identity, "base", sdd.ShowRequest{IDs: []string{"example.org/dep:20260713-040000-s-tac-dep"}})
	if err != nil || !strings.Contains(showText(show), "authorized dependency body") {
		t.Fatalf("cross-project Show over a partially-loaded dependency = %q, %v", showText(show), err)
	}
}

type unreadableGraphStore struct{}

func (unreadableGraphStore) Current(context.Context) (*sdd.Snapshot, error) {
	return nil, errors.New("snapshot unreadable")
}
func (unreadableGraphStore) ReadAttachmentPage(context.Context, string, string, int64, int) (sdd.AttachmentPage, error) {
	return sdd.AttachmentPage{}, errors.New("snapshot unreadable")
}

// TestUnreadableClosureMemberFailsOnlyReadsSelectingIt: base declares a, a
// declares b and d, d declares c, and b's snapshot cannot be read, so the walk
// meets b before c. Reads naming a or c succeed, and a session in base may
// work in c; a read selecting b, alone or through all repos, fails
// (20260903-153224-s-cpt-ir1).
func TestUnreadableClosureMemberFailsOnlyReadsSelectingIt(t *testing.T) {
	llm := pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) {
		return pkgllm.Result{Identity: pkgllm.Identity{Provider: "test", Model: "test"}}, nil
	})
	snapshot := func(project sdd.ProjectID, documents ...sdd.EntryDocument) *sdd.Snapshot {
		s, err := sdd.BuildSnapshot(t.Context(), sdd.SnapshotData{Project: project, Revision: "r1", Entries: documents})
		if err != nil {
			t.Fatal(err)
		}
		return s
	}
	gap := func(path string) sdd.EntryDocument {
		return sdd.EntryDocument{
			LogicalPath: path,
			Frontmatter: map[string]any{"type": "signal", "kind": "gap", "layer": "tactical", "summary": "A zebrafinch gap."},
			Body:        "A zebrafinch gap.",
		}
	}
	runtimes := map[sdd.ProjectID]*sdd.ProjectRuntime{}
	for _, project := range []struct {
		id           sdd.ProjectID
		graph        sdd.GraphStore
		dependencies []sdd.RepoID
	}{
		{id: "base", graph: staticGraphStore{snapshot: snapshot("base")}, dependencies: []sdd.RepoID{"dep.example/a"}},
		{id: "dep.example/a", graph: staticGraphStore{attachment: "a attachment", snapshot: snapshot("dep.example/a", gap("2026/07/13-040000-s-tac-dep.md"))}, dependencies: []sdd.RepoID{"dep.example/b", "dep.example/d"}},
		{id: "dep.example/b", graph: unreadableGraphStore{}},
		{id: "dep.example/d", graph: staticGraphStore{snapshot: snapshot("dep.example/d")}, dependencies: []sdd.RepoID{"dep.example/c"}},
		{id: "dep.example/c", graph: staticGraphStore{snapshot: snapshot("dep.example/c", gap("2026/07/12-040000-s-tac-dpe.md"))}},
	} {
		runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{Project: sdd.ProjectRef{ID: project.id}, DefaultBranch: "main", Graph: project.graph, Dependencies: project.dependencies, LLM: llm})
		if err != nil {
			t.Fatal(err)
		}
		runtimes[project.id] = runtime
	}
	access := &compositionAccess{runtimes: runtimes, permissions: map[string]map[sdd.ProjectID]compositionPermission{
		"alice": {"base": {read: true}, "dep.example/a": {read: true}, "dep.example/b": {read: true}, "dep.example/c": {read: true}, "dep.example/d": {read: true}},
	}}
	root := t.TempDir()
	sessions, err := localadapter.NewFilesystemSessionStoreAt(filepath.Join(root, "sessions"))
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(filepath.Join(root, "blobs"))
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{Access: access, Sessions: sessions, StagedBlobs: blobs})
	if err != nil {
		t.Fatal(err)
	}
	alice := sdd.RequestIdentity{Subject: "alice"}
	search := func(repos []sdd.RepoID, all bool) (sdd.SearchResult, error) {
		return application.Search(t.Context(), alice, "base", sdd.SearchRequest{SyncMode: sdd.SearchSyncNone, Terms: []string{"zebrafinch"}, Repos: repos, AllRepos: all})
	}

	if result, err := search([]sdd.RepoID{"dep.example/a"}, false); err != nil || !strings.Contains(searchText(result), "20260713-040000-s-tac-dep") {
		t.Errorf("search naming a = %q, %v", searchText(result), err)
	}
	if result, err := search([]sdd.RepoID{"dep.example/a"}, false); err != nil || !hasHit(result.Hits, "dep.example/a:20260713-040000-s-tac-dep") {
		t.Errorf("structured hits naming a = %+v, %v", result.Hits, err)
	}
	if list, err := application.ListEntries(t.Context(), alice, "base", sdd.ListEntriesRequest{Repos: []sdd.RepoID{"dep.example/a"}}); err != nil || list.Total != 1 || list.Entries[0].DisplayID != "dep.example/a:20260713-040000-s-tac-dep" {
		t.Errorf("listing naming a = %+v, %v", list, err)
	}
	if _, err := application.ListEntries(t.Context(), alice, "base", sdd.ListEntriesRequest{Repos: []sdd.RepoID{"dep.example/b"}}); err == nil {
		t.Error("listing naming b succeeded")
	}
	if result, err := search([]sdd.RepoID{"dep.example/c"}, false); err != nil || !strings.Contains(searchText(result), "20260712-040000-s-tac-dpe") {
		t.Errorf("search naming c = %q, %v", searchText(result), err)
	}
	workflow, _, err := application.OpenWorkflow(t.Context(), alice, "base", sdd.WorkflowOpenRequest{ClientName: "alice-mcp"})
	if err != nil {
		t.Fatal(err)
	}
	if project, _, _, err := workflow.ReadScope(t.Context(), alice, "dep.example/c"); err != nil || project != "dep.example/c" {
		t.Errorf("read scope in c = %q, %v", project, err)
	}
	if result, err := application.View(t.Context(), alice, "base", sdd.ViewRequest{Layout: "top(5)", Repos: []sdd.RepoID{"dep.example/a"}}); err != nil || !strings.Contains(result.Sections, "repo: dep.example/a") {
		t.Errorf("view naming a = %q, %v", result.Sections, err)
	}
	if page, err := application.ReadAttachment(t.Context(), alice, "base", sdd.ReadAttachmentRequest{EntryID: "dep.example/a:20260713-040000-s-tac-dep", Filename: "evidence.md", MaxBytes: 64}); err != nil || string(page.Page.Content) != "a attachment" {
		t.Errorf("attachment of a = %+v, %v", page, err)
	}
	if _, err := search([]sdd.RepoID{"dep.example/b"}, false); err == nil {
		t.Error("search naming b succeeded")
	}
	if _, err := search(nil, true); err == nil {
		t.Error("search across all repos succeeded")
	}
}

func TestApplicationResolvesAuthorizedDependenciesWithoutLeakingDenials(t *testing.T) {
	baseSnapshot, err := sdd.BuildSnapshot(t.Context(), sdd.SnapshotData{Project: "base", Revision: "r1"})
	if err != nil {
		t.Fatal(err)
	}
	foreignDocument := sdd.EntryDocument{
		LogicalPath: "2026/07/13-040000-s-tac-dep.md",
		Frontmatter: map[string]any{"type": "signal", "kind": "gap", "layer": "tactical", "summary": "Foreign dependency fixture."},
		Body:        "The authorized dependency body is visible through the base application.",
	}
	foreignSnapshot, err := sdd.BuildSnapshot(t.Context(), sdd.SnapshotData{Project: "example.org/dep", Revision: "r1", Entries: []sdd.EntryDocument{foreignDocument}})
	if err != nil {
		t.Fatal(err)
	}
	llm := pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) {
		return pkgllm.Result{Identity: pkgllm.Identity{Provider: "test", Model: "test"}}, nil
	})
	dependency, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "example.org/dep"}, Graph: staticGraphStore{snapshot: foreignSnapshot, attachment: "foreign attachment"},
		LLM: llm,
	})
	if err != nil {
		t.Fatal(err)
	}
	base, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "base"}, Dependencies: []sdd.RepoID{"example.org/dep"}, Graph: staticGraphStore{snapshot: baseSnapshot},
		LLM: llm,
	})
	if err != nil {
		t.Fatal(err)
	}
	resolver := &multiAccessResolver{base: base, dependency: dependency}
	application, err := sdd.NewApplication(sdd.ApplicationOptions{Access: resolver, Sessions: noSessionStore{}, StagedBlobs: noBlobStore{}})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher"}
	qualified := "example.org/dep:20260713-040000-s-tac-dep"
	show, err := application.Show(t.Context(), identity, "base", sdd.ShowRequest{IDs: []string{qualified}})
	if err != nil || !strings.Contains(showText(show), "authorized dependency body") {
		t.Fatalf("cross-project Show = %q, %v", showText(show), err)
	}
	page, err := application.ReadAttachment(t.Context(), identity, "base", sdd.ReadAttachmentRequest{EntryID: qualified, Filename: "evidence.md", MaxBytes: 64})
	if err != nil || string(page.Page.Content) != "foreign attachment" || page.Project.ID != "base" {
		t.Fatalf("cross-project ReadAttachment = %+v, %v", page, err)
	}

	resolver.deny = true
	_, denied := application.Show(t.Context(), identity, "base", sdd.ShowRequest{IDs: []string{qualified}})
	_, unknown := application.Show(t.Context(), identity, "base", sdd.ShowRequest{IDs: []string{"unknown.example:20260713-040000-s-tac-dep"}})
	if denied == nil || unknown == nil || !strings.HasPrefix(denied.Error(), "entry not found:") || !strings.HasPrefix(unknown.Error(), "entry not found:") {
		t.Fatalf("denied and unknown dependencies leaked different results: denied=%v unknown=%v", denied, unknown)
	}
}

func hasHit(hits []types.SearchHit, displayID string) bool {
	for _, hit := range hits {
		if hit.Entry.DisplayID == displayID {
			return true
		}
	}
	return false
}
