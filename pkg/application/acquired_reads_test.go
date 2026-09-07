package application_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/local"
)

type acquiredReadStore struct {
	sdd.GraphStore
	acquire func(context.Context, sdd.SnapshotReadQuery) (*sdd.AcquiredSnapshot, error)
}

func (s acquiredReadStore) AcquireSnapshot(ctx context.Context, q sdd.SnapshotReadQuery) (*sdd.AcquiredSnapshot, error) {
	return s.acquire(ctx, q)
}
func (s acquiredReadStore) Current(context.Context) (*sdd.Snapshot, error) {
	return nil, fmt.Errorf("unpinned Current must not run")
}
func (s acquiredReadStore) ReadAttachmentPage(context.Context, string, string, int64, int) (sdd.AttachmentPage, error) {
	return sdd.AttachmentPage{}, fmt.Errorf("live attachment reader must not run")
}

type attachmentPageFunc func(context.Context, string, string, int64, int) (sdd.AttachmentPage, error)

func (f attachmentPageFunc) ReadAttachmentPage(ctx context.Context, id, name string, offset int64, limit int) (sdd.AttachmentPage, error) {
	return f(ctx, id, name, offset, limit)
}

func acquiredRuntime(t *testing.T, project string, graph sdd.GraphStore, dependencies ...string) *sdd.ProjectRuntime {
	t.Helper()
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: sdd.ProjectID(project)}, Graph: graph, DefaultBranch: "write-default", Language: "runtime-language", Dependencies: dependencies,
		Targets: sdd.TargetAcquirerFunc(func(context.Context, sdd.MutationTarget) (*sdd.AcquiredTarget, error) {
			return nil, fmt.Errorf("mutation acquisition denied")
		}),
		LLM: llm.RunnerFunc(func(context.Context, llm.Request) (llm.Result, error) { return llm.Result{}, nil }),
	})
	if err != nil {
		t.Fatal(err)
	}
	return runtime
}
func acquiredSnapshot(t *testing.T, project, revision, body string) *sdd.Snapshot {
	t.Helper()
	s, err := sdd.BuildSnapshot(t.Context(), sdd.SnapshotData{Project: sdd.ProjectID(project), Revision: revision,
		Config:  sdd.ProjectConfigDocument{Fields: map[string]any{"language": "must-not-be-interpreted", "dependencies": []string{"wrong"}}},
		Entries: []sdd.EntryDocument{{LogicalPath: "2026/01/01-100000-s-tac-aaa.md", Frontmatter: map[string]any{"type": "signal", "kind": "gap", "layer": "tactical", "summary": body}, Body: body}},
	})
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestAcquiredReadsUseReadAuthorityAndRelease(t *testing.T) {
	snapshot := acquiredSnapshot(t, "base", "r1", "Selected source body")
	var releases atomic.Int32
	store := acquiredReadStore{acquire: func(_ context.Context, q sdd.SnapshotReadQuery) (*sdd.AcquiredSnapshot, error) {
		if q.Branch != "work" && q.Branch != "" {
			t.Errorf("branch=%q", q.Branch)
		}
		return &sdd.AcquiredSnapshot{Snapshot: snapshot, Attachments: staticGraphStore{attachment: "pinned bytes"}, Config: &sdd.ProjectConfig{Language: "source-language"}, Release: func() error { releases.Add(1); return nil }}, nil
	}}
	app := preparationApp(t, acquiredRuntime(t, "base", store), nil, nil)
	identity := sdd.RequestIdentity{Subject: "reader"}
	for name, run := range map[string]func() error{
		"show": func() error {
			r, e := app.Show(t.Context(), identity, "base", sdd.ShowRequest{Branch: "work", IDs: []string{"20260101-100000-s-tac-aaa"}})
			if e == nil && !strings.Contains(r.Entries, "Selected source body") {
				t.Error(r)
			}
			return e
		},
		"view": func() error {
			_, e := app.View(t.Context(), identity, "base", sdd.ViewRequest{Branch: "work", Layout: "as-list", OmitRecovery: true})
			return e
		},
		"text search": func() error {
			_, e := app.Search(t.Context(), identity, "base", sdd.SearchRequest{Branch: "work", SyncMode: sdd.SearchSyncNone, Terms: []string{"Selected"}})
			return e
		},
		"attachment": func() error {
			r, e := app.ReadAttachment(t.Context(), identity, "base", sdd.ReadAttachmentRequest{Branch: "work", EntryID: "20260101-100000-s-tac-aaa", Filename: "note.md", MaxBytes: 100})
			if e == nil && string(r.Page.Content) != "pinned bytes" {
				t.Error(r)
			}
			return e
		},
		"snapshot": func() error { _, e := app.CurrentSnapshot(t.Context(), identity, "base"); return e },
		"procedures": func() error {
			_, e := app.Procedures(t.Context(), identity, "base", sdd.ProcedureListRequest{})
			return e
		},
		"info": func() error {
			r, e := app.Info(t.Context(), identity, "base", sdd.InfoRequest{})
			if e == nil && r.Language != "source-language" {
				t.Error(r)
			}
			return e
		},
	} {
		t.Run(name, func(t *testing.T) {
			before := releases.Load()
			if err := run(); err != nil {
				t.Fatal(err)
			}
			if releases.Load() != before+1 {
				t.Fatalf("releases=%d", releases.Load())
			}
		})
	}
}

func TestAcquiredConfigurationIsPerOperationAndHasOneAuthority(t *testing.T) {
	base := acquiredSnapshot(t, "base", "r1", "sourcehometoken")
	dep := acquiredSnapshot(t, "dep", "d1", "sourcedeptoken")
	sourceConfig := &sdd.ProjectConfig{Language: "source-language", Dependencies: []string{}}
	var compatibility atomic.Bool
	var calls atomic.Int32
	store := acquiredReadStore{acquire: func(context.Context, sdd.SnapshotReadQuery) (*sdd.AcquiredSnapshot, error) {
		calls.Add(1)
		config := sourceConfig
		if compatibility.Load() {
			config = nil
		}
		return &sdd.AcquiredSnapshot{Snapshot: base, Config: config, Attachments: staticGraphStore{}, Release: func() error { return nil }}, nil
	}}
	runtime := acquiredRuntime(t, "base", store, "dep")
	dependency := acquiredRuntime(t, "dep", staticGraphStore{snapshot: dep})
	app := preparationApp(t, runtime, dependency, nil)
	id := sdd.RequestIdentity{Subject: "reader"}
	for _, legacy := range []bool{false, true, false} {
		compatibility.Store(legacy)
		info, err := app.Info(t.Context(), id, "base", sdd.InfoRequest{})
		if err != nil {
			t.Fatal(err)
		}
		wantLanguage := "source-language"
		wantEntries := 1
		if legacy {
			wantLanguage = "runtime-language"
			wantEntries = 2
		}
		if info.Language != wantLanguage {
			t.Fatalf("language=%s", info.Language)
		}
		r, err := app.Search(t.Context(), id, "base", sdd.SearchRequest{AllRepos: true, SyncMode: sdd.SearchSyncNone, Terms: []string{"sourcehometoken|sourcedeptoken"}})
		if err != nil {
			t.Fatal(err)
		}
		if len(r.EntryIDs) != wantEntries {
			t.Fatalf("legacy=%v entries=%v", legacy, r.EntryIDs)
		}
	}
	if sourceConfig.Language != "source-language" || len(sourceConfig.Dependencies) != 0 {
		t.Fatal("shared config mutated")
	}
	if calls.Load() != 6 {
		t.Fatalf("acquisitions=%d", calls.Load())
	}
}

func TestAttachmentOperationPinsSourceAcrossBranchAdvance(t *testing.T) {
	first := acquiredSnapshot(t, "base", "r1", "Old")
	second := acquiredSnapshot(t, "base", "r2", "New")
	var advanced atomic.Bool
	entered := make(chan struct{})
	resume := make(chan struct{})
	var releases atomic.Int32
	store := acquiredReadStore{acquire: func(_ context.Context, q sdd.SnapshotReadQuery) (*sdd.AcquiredSnapshot, error) {
		snapshot := first
		body := "old bytes"
		if advanced.Load() {
			snapshot = second
			body = "new bytes"
		}
		reader := attachmentPageFunc(func(ctx context.Context, _ string, name string, _ int64, _ int) (sdd.AttachmentPage, error) {
			if snapshot == first {
				close(entered)
				select {
				case <-resume:
				case <-ctx.Done():
					return sdd.AttachmentPage{}, ctx.Err()
				}
			}
			return sdd.AttachmentPage{Filename: name, Content: []byte(body)}, nil
		})
		return &sdd.AcquiredSnapshot{Snapshot: snapshot, Attachments: reader, Release: func() error { releases.Add(1); return nil }}, nil
	}}
	app := preparationApp(t, acquiredRuntime(t, "base", store), nil, nil)
	request := sdd.ReadAttachmentRequest{Branch: "main", EntryID: "20260101-100000-s-tac-aaa", Filename: "note.md", MaxBytes: 100}
	type outcome struct {
		result sdd.ReadAttachmentResult
		err    error
	}
	done := make(chan outcome, 1)
	go func() {
		r, e := app.ReadAttachment(t.Context(), sdd.RequestIdentity{Subject: "first"}, "base", request)
		done <- outcome{r, e}
	}()
	<-entered
	advanced.Store(true)
	newer, err := app.ReadAttachment(t.Context(), sdd.RequestIdentity{Subject: "second"}, "base", request)
	if err != nil {
		t.Fatal(err)
	}
	close(resume)
	older := <-done
	if older.err != nil {
		t.Fatal(older.err)
	}
	if string(older.result.Page.Content) != "old bytes" || string(newer.Page.Content) != "new bytes" {
		t.Fatal("read sources mixed")
	}
	if releases.Load() != 2 {
		t.Fatalf("releases=%d", releases.Load())
	}
}

func TestAcquiredReadFailuresKeepCleanupErrors(t *testing.T) {
	snapshot := acquiredSnapshot(t, "base", "r1", "Body")
	operationErr := errors.New("source config missing or malformed")
	cleanupErr := errors.New("cleanup failed")
	for _, partial := range []bool{false, true} {
		t.Run(fmt.Sprint(partial), func(t *testing.T) {
			released := 0
			store := acquiredReadStore{acquire: func(context.Context, sdd.SnapshotReadQuery) (*sdd.AcquiredSnapshot, error) {
				source := &sdd.AcquiredSnapshot{Snapshot: snapshot, Attachments: attachmentPageFunc(func(context.Context, string, string, int64, int) (sdd.AttachmentPage, error) {
					return sdd.AttachmentPage{}, operationErr
				}), Release: func() error { released++; return cleanupErr }}
				if partial {
					return source, operationErr
				}
				return source, nil
			}}
			app := preparationApp(t, acquiredRuntime(t, "base", store), nil, nil)
			_, err := app.ReadAttachment(t.Context(), sdd.RequestIdentity{Subject: "reader"}, "base", sdd.ReadAttachmentRequest{EntryID: "20260101-100000-s-tac-aaa"})
			if !errors.Is(err, operationErr) || !errors.Is(err, cleanupErr) || released != 1 {
				t.Fatalf("err=%v released=%d", err, released)
			}
		})
	}
}

func TestLegacyReadsRejectBranchAndCausalSelection(t *testing.T) {
	snapshot := acquiredSnapshot(t, "base", "r1", "Body")
	app := preparationApp(t, acquiredRuntime(t, "base", staticGraphStore{snapshot: snapshot}), nil, nil)
	id := sdd.RequestIdentity{Subject: "reader"}
	if _, err := app.Show(t.Context(), id, "base", sdd.ShowRequest{IDs: []string{"20260101-100000-s-tac-aaa"}}); err != nil {
		t.Fatal(err)
	}
	if _, err := app.Show(t.Context(), id, "base", sdd.ShowRequest{Branch: "main", IDs: []string{"20260101-100000-s-tac-aaa"}}); err == nil {
		t.Fatal("legacy branch read accepted")
	}
	if _, err := app.Search(t.Context(), id, "base", sdd.SearchRequest{IncludesRevision: "r1", SyncMode: sdd.SearchSyncNone, Terms: []string{"Body"}}); err == nil {
		t.Fatal("legacy causal read accepted")
	}
}

func TestAcquiredReadCancellationReleasesSource(t *testing.T) {
	snapshot := acquiredSnapshot(t, "base", "r1", "Body")
	ctx, cancel := context.WithCancel(t.Context())
	released := 0
	store := acquiredReadStore{acquire: func(context.Context, sdd.SnapshotReadQuery) (*sdd.AcquiredSnapshot, error) {
		return &sdd.AcquiredSnapshot{Snapshot: snapshot, Attachments: attachmentPageFunc(func(ctx context.Context, _ string, _ string, _ int64, _ int) (sdd.AttachmentPage, error) {
			cancel()
			return sdd.AttachmentPage{}, ctx.Err()
		}), Release: func() error { released++; return nil }}, nil
	}}
	app := preparationApp(t, acquiredRuntime(t, "base", store), nil, nil)
	_, err := app.ReadAttachment(ctx, sdd.RequestIdentity{Subject: "reader"}, "base", sdd.ReadAttachmentRequest{EntryID: "20260101-100000-s-tac-aaa"})
	if !errors.Is(err, context.Canceled) || released != 1 {
		t.Fatalf("err=%v releases=%d", err, released)
	}
}

func TestSourceConfigurationCannotGrantDependencyAccess(t *testing.T) {
	snapshot := acquiredSnapshot(t, "base", "r1", "Body")
	released := 0
	config := &sdd.ProjectConfig{Dependencies: []string{"dep"}}
	store := acquiredReadStore{acquire: func(context.Context, sdd.SnapshotReadQuery) (*sdd.AcquiredSnapshot, error) {
		return &sdd.AcquiredSnapshot{Snapshot: snapshot, Config: config, Attachments: staticGraphStore{}, Release: func() error { released++; return nil }}, nil
	}}
	access := &multiAccessResolver{base: acquiredRuntime(t, "base", store), deny: true}
	app, err := sdd.NewApplication(sdd.ApplicationOptions{Access: access, Sessions: noSessionStore{}, StagedBlobs: noBlobStore{}})
	if err != nil {
		t.Fatal(err)
	}
	_, err = app.Search(t.Context(), sdd.RequestIdentity{Subject: "reader"}, "base", sdd.SearchRequest{AllRepos: true, SyncMode: sdd.SearchSyncNone, Terms: []string{"Body"}})
	if err == nil || released != 1 {
		t.Fatalf("err=%v releases=%d", err, released)
	}
	if len(config.Dependencies) != 1 || config.Dependencies[0] != "dep" {
		t.Fatal("source config mutated")
	}
}

func TestWorkflowServeUsesOneViewAndRefreshesNextOperation(t *testing.T) {
	first := acquiredSnapshot(t, "base", "r1", "First revision")
	second := acquiredSnapshot(t, "base", "r2", "Second revision")
	var calls atomic.Int32
	var revision atomic.Int32
	store := acquiredReadStore{acquire: func(context.Context, sdd.SnapshotReadQuery) (*sdd.AcquiredSnapshot, error) {
		calls.Add(1)
		snapshot, language := first, "first-language"
		if revision.Load() > 0 {
			snapshot, language = second, "second-language"
		}
		revision.Store(1)
		return &sdd.AcquiredSnapshot{Snapshot: snapshot, Config: &sdd.ProjectConfig{Language: language}, Attachments: staticGraphStore{}, Release: func() error { return nil }}, nil
	}}
	sessions, err := local.NewFilesystemSessionStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	app, err := sdd.NewApplication(sdd.ApplicationOptions{Access: &multiAccessResolver{base: acquiredRuntime(t, "base", store)}, Sessions: sessions, StagedBlobs: noBlobStore{}})
	if err != nil {
		t.Fatal(err)
	}
	identity := sdd.RequestIdentity{Subject: "reader"}
	workflow, _, err := app.OpenWorkflow(t.Context(), identity, "base", sdd.WorkflowOpenRequest{})
	if err != nil {
		t.Fatal(err)
	}
	revision.Store(0)
	for _, want := range []string{"first-language", "second-language"} {
		calls.Store(0)
		blocks, err := workflow.Framing(t.Context(), identity)
		if err != nil {
			t.Fatal(err)
		}
		if calls.Load() != 1 {
			t.Fatalf("one framing acquired %d sources", calls.Load())
		}
		if !strings.Contains(strings.Join(blocks, "\n"), want) {
			t.Fatalf("framing missed %s", want)
		}
	}
}

func TestInfoSelectsRequestedAuthorityWithoutFallback(t *testing.T) {
	snapshot := acquiredSnapshot(t, "base", "r1", "Info source")
	var branches []string
	store := acquiredReadStore{acquire: func(_ context.Context, q sdd.SnapshotReadQuery) (*sdd.AcquiredSnapshot, error) {
		branches = append(branches, q.Branch)
		if q.Branch == "missing" {
			return nil, errors.New("source unavailable")
		}
		language := "en"
		if q.Branch == "work" {
			language = "de"
		}
		return &sdd.AcquiredSnapshot{Snapshot: snapshot, Config: &sdd.ProjectConfig{Language: language}, Attachments: staticGraphStore{}, Release: func() error { return nil }}, nil
	}}
	app := preparationApp(t, acquiredRuntime(t, "base", store), nil, nil)
	identity := sdd.RequestIdentity{Subject: "reader"}
	for _, tc := range []struct{ branch, language string }{{"", "en"}, {"work", "de"}} {
		info, err := app.Info(t.Context(), identity, "base", sdd.InfoRequest{Branch: tc.branch})
		if err != nil || info.Language != tc.language {
			t.Fatalf("info=%+v err=%v", info, err)
		}
	}
	_, err := app.Info(t.Context(), identity, "base", sdd.InfoRequest{Branch: "missing", BranchFromSession: true})
	if err == nil || !strings.Contains(err.Error(), "session is bound") {
		t.Fatalf("missing branch error: %v", err)
	}
	if strings.Join(branches, ",") != ",work,missing" {
		t.Fatalf("unexpected acquisitions: %v", branches)
	}
}
