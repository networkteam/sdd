package application_test

import (
	"context"
	"io"
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
)

type staticGraphStore struct {
	snapshot   *sdd.Snapshot
	attachment string
}

func (s staticGraphStore) Current(context.Context) (*sdd.Snapshot, error) { return s.snapshot, nil }
func (staticGraphStore) Apply(context.Context, string, sdd.MutationBatch, sdd.StagedBlobReader) (sdd.ApplyResult, error) {
	return sdd.ApplyResult{}, nil
}
func (staticGraphStore) Reconcile(context.Context, string, string) (sdd.ApplyResult, error) {
	return sdd.ApplyResult{}, nil
}
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
func (noBlobStore) Stat(context.Context, sdd.SessionRef, string) (sdd.StagedBlob, error) {
	return sdd.StagedBlob{}, nil
}
func (noBlobStore) Open(context.Context, sdd.SessionRef, string) (io.ReadCloser, error) {
	return io.NopCloser(strings.NewReader("")), nil
}
func (noBlobStore) Retain(context.Context, sdd.SessionRef, string, []string) error { return nil }
func (noBlobStore) Release(context.Context, sdd.SessionRef, string) error          { return nil }
func (noBlobStore) StagedSessions(context.Context) ([]sdd.SessionRef, error)       { return nil, nil }
func (noBlobStore) DeleteStaged(context.Context, sdd.SessionRef) error             { return nil }

type noSessionStore struct{}

func (noSessionStore) Create(context.Context, sdd.SessionMetadata) (sdd.StoredSession, error) {
	return sdd.StoredSession{}, nil
}
func (noSessionStore) Load(context.Context, sdd.SessionID) (sdd.StoredSession, error) {
	return sdd.StoredSession{}, nil
}
func (noSessionStore) List(context.Context, sdd.SessionFilter) ([]sdd.StoredSession, error) {
	return nil, nil
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
	return sdd.Principal{Subject: identity.Subject, Participant: "Christopher"}, nil
}
func (r *multiAccessResolver) ListProjects(context.Context, sdd.Principal) (sdd.ProjectList, error) {
	return sdd.ProjectList{Projects: []sdd.ProjectSummary{{ProjectRef: r.base.Project(), CanRead: true, State: sdd.ProjectReady}}}, nil
}
func (r *multiAccessResolver) ResolveProject(context.Context, sdd.Principal, sdd.ProjectID, sdd.Access) (*sdd.ProjectRuntime, error) {
	return r.base, nil
}
func (r *multiAccessResolver) ResolveDependency(context.Context, sdd.Principal, sdd.ProjectID, string) (*sdd.ProjectRuntime, error) {
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
		Sessions: noSessionStore{}, StagedBlobs: noBlobStore{}, LLM: llm,
	})
	if err != nil {
		t.Fatal(err)
	}
	base, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "base"}, Dependencies: []string{"example.org/dep"}, Graph: staticGraphStore{snapshot: baseSnapshot},
		Sessions: noSessionStore{}, StagedBlobs: noBlobStore{}, LLM: llm,
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(&multiAccessResolver{base: base, dependency: dependency})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher"}
	show, err := application.Show(t.Context(), identity, "base", sdd.ShowRequest{IDs: []string{"example.org/dep:20260713-040000-s-tac-dep"}})
	if err != nil || !strings.Contains(show.Entries, "authorized dependency body") {
		t.Fatalf("cross-project Show over a partially-loaded dependency = %q, %v", show.Entries, err)
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
		Sessions: noSessionStore{}, StagedBlobs: noBlobStore{}, LLM: llm,
	})
	if err != nil {
		t.Fatal(err)
	}
	base, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: "base"}, Dependencies: []string{"example.org/dep"}, Graph: staticGraphStore{snapshot: baseSnapshot},
		Sessions: noSessionStore{}, StagedBlobs: noBlobStore{}, LLM: llm,
	})
	if err != nil {
		t.Fatal(err)
	}
	resolver := &multiAccessResolver{base: base, dependency: dependency}
	application, err := sdd.NewApplication(resolver)
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "christopher"}
	qualified := "example.org/dep:20260713-040000-s-tac-dep"
	show, err := application.Show(t.Context(), identity, "base", sdd.ShowRequest{IDs: []string{qualified}})
	if err != nil || !strings.Contains(show.Entries, "authorized dependency body") {
		t.Fatalf("cross-project Show = %q, %v", show.Entries, err)
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
