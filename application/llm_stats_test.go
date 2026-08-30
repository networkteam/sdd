package application_test

import (
	"context"
	"errors"
	"strings"
	"sync"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/llm"
	localadapter "github.com/networkteam/sdd/local"
)

// The engine path runs its LLM calls through the host's executor. Whatever the
// executor reports must reach the stats sink, because that hand-off dropping
// the metadata is what left every engine-mode call absent from `sdd stats`.

type recordingSink struct {
	mu    sync.Mutex
	calls []llm.CallStat
}

func (s *recordingSink) RecordCall(stat llm.CallStat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls = append(s.calls, stat)
}

func (s *recordingSink) recorded() []llm.CallStat {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]llm.CallStat(nil), s.calls...)
}

func newGuideApp(t *testing.T, execute func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error)) (*sdd.Application, sdd.RequestIdentity) {
	t.Helper()
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: t.TempDir()})
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
		Project: sdd.ProjectRef{ID: "example"}, DefaultBranch: "main", Graph: graph, Sessions: sessions, StagedBlobs: blobs,
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
			IdentityFunc:     func() sdd.LLMIdentity { return sdd.LLMIdentity{Provider: "ollama", Model: "glm-5.3-flash:cloud"} },
			ExecuteFunc:      execute,
		},

		LLMTimeout: time.Minute,
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(&runtimeAccessResolver{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	return application, sdd.RequestIdentity{Subject: "christopher"}
}

func guideDraft() sdd.EntryDraft {
	return sdd.EntryDraft{
		Kind:   "gap",
		Layer:  "cpt",
		Intent: "The engine path recorded no LLM statistics at all.",
		Body:   "Every call through the host executor lost its usage metadata before the stats sink saw it.",
	}
}

func TestWritingGuideCheckRecordsCallStats(t *testing.T) {
	application, identity := newGuideApp(t, func(_ context.Context, request sdd.LLMRequest) (sdd.LLMResult, error) {
		if request.Purpose != "writing-guide" {
			t.Errorf("unexpected purpose %q", request.Purpose)
		}
		return sdd.LLMResult{
			Output:              []byte(`{"findings":[]}`),
			ExecutorFingerprint: "local",
			Usage:               sdd.LLMUsage{InputTokens: 4200, OutputTokens: 90, CacheReadTokens: 3000, CacheCreateTokens: 12},
		}, nil
	})

	sink := &recordingSink{}
	ctx := llm.WithStatsSink(context.Background(), sink)
	if _, err := application.WritingGuideCheck(ctx, identity, "example", guideDraft()); err != nil {
		t.Fatalf("WritingGuideCheck: %v", err)
	}

	calls := sink.recorded()
	if len(calls) != 1 {
		t.Fatalf("recorded %d calls, want 1", len(calls))
	}
	got := calls[0]
	if got.Op != "writing-guide" {
		t.Errorf("op = %q, want writing-guide", got.Op)
	}
	if got.Provider != "ollama" || got.Model != "glm-5.3-flash:cloud" {
		t.Errorf("provider/model = %q/%q, want ollama/glm-5.3-flash:cloud", got.Provider, got.Model)
	}
	if got.InputTokens != 4200 || got.OutputTokens != 90 {
		t.Errorf("tokens = %d/%d, want 4200/90", got.InputTokens, got.OutputTokens)
	}
	if got.CacheReadTokens != 3000 || got.CacheCreateTokens != 12 {
		t.Errorf("cache = %d/%d, want 3000/12", got.CacheReadTokens, got.CacheCreateTokens)
	}
	if got.Error != "" {
		t.Errorf("error = %q, want empty", got.Error)
	}
}

func TestWritingGuideCheckRecordsFailedCall(t *testing.T) {
	application, identity := newGuideApp(t, func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) {
		return sdd.LLMResult{}, errors.New("gollm ollama: timed out")
	})

	sink := &recordingSink{}
	ctx := llm.WithStatsSink(context.Background(), sink)
	if _, err := application.WritingGuideCheck(ctx, identity, "example", guideDraft()); err == nil {
		t.Fatal("WritingGuideCheck: want error")
	}

	calls := sink.recorded()
	if len(calls) != 1 {
		t.Fatalf("recorded %d calls, want 1", len(calls))
	}
	got := calls[0]
	if got.Op != "writing-guide" {
		t.Errorf("op = %q, want writing-guide", got.Op)
	}
	if !strings.Contains(got.Error, "timed out") {
		t.Errorf("error = %q, want it to name the timeout", got.Error)
	}
}
