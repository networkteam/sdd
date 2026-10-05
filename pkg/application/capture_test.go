package application_test

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/llm"
)

func TestCreateEntryRetryKeepsIdentityAndSkipsPreparationAfterPublication(t *testing.T) {
	summaryCalls := 0
	f := newWriteFixture(t, writeFixtureOptions{Runner: llm.RunnerFunc(func(_ context.Context, request llm.Request) (llm.Result, error) {
		if request.Purpose == llm.PurposePreflight {
			t.Fatal("CreateEntry must not repeat the procedure's preflight")
		}
		summaryCalls++
		if summaryCalls != 2 {
			return llm.Result{}, errors.New("summary provider unavailable")
		}
		return llm.Result{Text: "The actual published summary."}, nil
	})})
	draft := f.identifiedDraft(sdd.EntryDraft{Kind: "gap", Layer: "tactical", Confidence: "high", Body: "A publication reuses the identity recorded by its invocation."})
	if result, err := f.app.CreateEntry(t.Context(), f.identity, "example", f.binding, draft); err == nil || result.EntryID != draft.EntryID {
		t.Fatalf("failed preparation = %+v, %v", result, err)
	}
	created, err := f.app.CreateEntry(t.Context(), f.identity, "example", f.binding, draft)
	if err != nil || created.EntryID != draft.EntryID {
		t.Fatalf("retry = %+v, %v", created, err)
	}

	// Deliberately unusable preparation proves that publication lookup comes first.
	draft.Kind = "invalid"
	draft.Attachments = []sdd.StagedAttachment{{Filename: "missing.md", BlobID: "missing"}}
	retried, err := f.app.CreateEntry(t.Context(), f.identity, "example", f.binding, draft)
	if err != nil || retried.EntryID != created.EntryID || retried.Summary != created.Summary || summaryCalls != 2 {
		t.Fatalf("published retry = %+v, %v; summary calls = %d", retried, err, summaryCalls)
	}
	if got := loadEntryByID(t, f.graphDir, created.EntryID).Summary; got != "The actual published summary." {
		t.Fatalf("stored summary = %q", got)
	}
}

func TestCreateEntryRetryCompletesRequiredFinalizerWithoutAnotherSummary(t *testing.T) {
	finalizer := &failOnceFinalizer{}
	summaries := 0
	f := newWriteFixture(t, writeFixtureOptions{
		Finalizers: []sdd.MutationFinalizer{finalizer},
		Runner: llm.RunnerFunc(func(context.Context, llm.Request) (llm.Result, error) {
			summaries++
			return llm.Result{Text: "A durable publication still requires composition completion."}, nil
		}),
	})
	draft := f.identifiedDraft(sdd.EntryDraft{Kind: "gap", Layer: "tactical", Confidence: "high", Body: "Required completion belongs before the operation's successful outcome."})
	if _, err := f.app.CreateEntry(t.Context(), f.identity, "example", f.binding, draft); err == nil {
		t.Fatal("failed required finalizer reported success")
	}
	result, err := f.app.CreateEntry(t.Context(), f.identity, "example", f.binding, draft)
	if err != nil || result.EntryID != draft.EntryID || summaries != 1 || finalizer.calls != 2 {
		t.Fatalf("completion retry = %+v, %v; summaries=%d finalizations=%d", result, err, summaries, finalizer.calls)
	}
}

func TestPreflightEntryReturnsFindingsWithoutPublishing(t *testing.T) {
	f := newWriteFixture(t, writeFixtureOptions{Runner: llm.RunnerFunc(func(_ context.Context, request llm.Request) (llm.Result, error) {
		if request.Purpose != llm.PurposePreflight {
			t.Fatal("preflight invoked summary generation")
		}
		return llm.Result{Text: `{"findings":[{"severity":"high","category":"test-blocker","observation":"Revise the draft."}]}`}, nil
	})})
	draft := sdd.EntryDraft{Kind: "gap", Layer: "tactical", Confidence: "high", Body: "Preflight checks a draft without publishing it."}
	result, err := f.app.PreflightEntry(t.Context(), f.identity, "example", f.binding, draft)
	if err != nil || result.Target.Branch != "main" || len(result.Findings) == 0 {
		t.Fatalf("preflight = %+v, %v", result, err)
	}
	if files, err := filepath.Glob(filepath.Join(f.graphDir, "*", "*", "*.md")); err != nil || len(files) != 0 {
		t.Fatalf("preflight entry files = %v, %v", files, err)
	}
}

func TestCaptureUsesAcquiredLanguageAndDependencies(t *testing.T) {
	dependency := acquiredRuntime(t, "example.org/dependency", staticGraphStore{snapshot: acquiredSnapshot(t, "example.org/dependency", "dependency-r1", "Declared source dependency.")})
	var purposes []llm.Purpose
	f := newWriteFixture(t, writeFixtureOptions{
		SourceConfig: &sdd.ProjectConfig{Language: "de", Dependencies: []sdd.RepoID{"example.org/dependency"}},
		Dependency:   dependency,
		Runner: llm.RunnerFunc(func(_ context.Context, request llm.Request) (llm.Result, error) {
			purposes = append(purposes, request.Purpose)
			if !strings.Contains(request.Combined(), "`de`") {
				t.Errorf("%s did not receive the acquired graph language", request.Purpose)
			}
			if request.Purpose == llm.PurposePreflight {
				return llm.Result{Text: `{"findings":[]}`}, nil
			}
			return llm.Result{Text: "Eine Beobachtung mit dem Kontext des ausgewählten Graphen."}, nil
		}),
	})
	draft := f.identifiedDraft(sdd.EntryDraft{
		Kind: "gap", Layer: "tactical", Confidence: "high",
		Body: "Diese Beobachtung bezieht sich auf die deklarierte Abhängigkeit.",
		Refs: []sdd.EntryRef{{ID: "example.org/dependency:20260101-100000-s-tac-aaa", Kind: "grounded-in"}},
	})
	created, err := preflightAndCreateEntry(t, f.app, f.identity, f.binding, draft)
	if err != nil || created.EntryID == "" {
		t.Fatalf("capture with acquired configuration = %+v, %v", created, err)
	}
	if len(purposes) != 2 || purposes[0] != llm.PurposePreflight || purposes[1] != llm.PurposeSummarize {
		t.Fatalf("capture LLM purposes = %v", purposes)
	}
	if got := loadEntryByID(t, f.graphDir, created.EntryID).Summary; got != created.Summary {
		t.Fatalf("stored summary = %q, want %q", got, created.Summary)
	}
}

// failOnceFinalizer fails its first call and succeeds afterwards, so a retry
// proves that required finalizers run on every attempt.
type failOnceFinalizer struct {
	mu    sync.Mutex
	calls int
}

func (*failOnceFinalizer) Name() string { return "fail-once" }
func (f *failOnceFinalizer) Finalize(context.Context, sdd.AppliedMutation) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	if f.calls == 1 {
		return fmt.Errorf("finalizer failed once")
	}
	return nil
}
