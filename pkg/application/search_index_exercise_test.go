package application_test

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	provider "github.com/networkteam/sdd/internal/llm/embed"
	"github.com/networkteam/sdd/internal/model"
	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/llm"
	"github.com/networkteam/sdd/pkg/llm/embed"
	"github.com/networkteam/sdd/pkg/local"
)

type exerciseStats struct {
	mu       sync.Mutex
	Calls    int `json:"calls"`
	Tokens   int `json:"tokens"`
	Shared   int `json:"shared_batches"`
	MaxItems int `json:"max_items"`
}

func (s *exerciseStats) RecordCall(ctx context.Context, stat llm.CallStat) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.Calls++
	s.Tokens += stat.Usage.InputTokens
	s.MaxItems = max(s.MaxItems, stat.Items)
	if len(embed.Attribution(ctx).Callers) > 1 {
		s.Shared++
	}
}

type exerciseStore struct {
	*local.PersistentSearchIndexStore
	fail bool
}

func (s exerciseStore) PublishEntry(ctx context.Context, v sdd.SearchEntryVersion, rows []sdd.IndexedChunk) error {
	if s.fail && strings.HasSuffix(v.EntryID, "bad") {
		return errors.New("injected entry publication failure")
	}
	return s.PersistentSearchIndexStore.PublishEntry(ctx, v, rows)
}

type exerciseReport struct {
	Published    int           `json:"published"`
	Skipped      int           `json:"skipped"`
	Failed       int           `json:"failed"`
	Calls        int           `json:"calls"`
	Tokens       int           `json:"tokens"`
	Shared       int           `json:"shared_batches"`
	MaxItems     int           `json:"max_items"`
	Peak         int32         `json:"peak_provider_calls"`
	Elapsed      time.Duration `json:"elapsed"`
	QueryLatency time.Duration `json:"query_latency"`
}

func TestEntryIndexingExerciseWorker(t *testing.T) {
	root := os.Getenv("SDD_INDEX_EXERCISE_ROOT")
	if root == "" {
		t.Skip("subprocess helper")
	}
	concurrency, _ := strconv.Atoi(os.Getenv("SDD_INDEX_EXERCISE_CONCURRENCY"))
	graph, err := local.NewFilesystemGraphStore(local.FilesystemGraphStoreOptions{Project: "exercise", GraphDir: filepath.Join(root, "graph")})
	if err != nil {
		t.Fatal(err)
	}
	var inner embed.Embedder
	liveModel := os.Getenv("SDD_LIVE_EMBED_MODEL")
	if liveModel != "" {
		inner, err = provider.New(model.EmbeddingConfig{Provider: "ollama", Model: liveModel, OllamaEndpoint: "http://127.0.0.1:11434"})
		if err != nil {
			t.Fatal(err)
		}
	} else {
		inner = embed.EmbedderFunc{Space: "scripted", Run: func(ctx context.Context, req embed.Request) (embed.Result, error) {
			timer := time.NewTimer(4 * time.Millisecond)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return embed.Result{}, ctx.Err()
			case <-timer.C:
			}
			r := embed.Result{Vectors: make([][]float32, len(req.Texts)), Usage: llm.Usage{InputTokens: len(req.Texts)}}
			for i := range r.Vectors {
				r.Vectors[i] = []float32{1, 1}
			}
			return r, nil
		}}
	}
	stats := &exerciseStats{}
	var active, peak atomic.Int32
	tracked := embed.EmbedderFunc{Space: inner.Fingerprint(), Run: func(ctx context.Context, req embed.Request) (embed.Result, error) {
		n := active.Add(1)
		defer active.Add(-1)
		for {
			old := peak.Load()
			if n <= old || peak.CompareAndSwap(old, n) {
				break
			}
		}
		return inner.Embed(ctx, req)
	}}
	observed := embed.Observed(embed.Bounded(tracked, 2*time.Minute), stats)
	batcher, err := embed.NewBatcher(t.Context(), observed, embed.BatchOptions{MaxItems: 32, MaxBytes: 128 * 1024, BufferItems: 4, Window: 10 * time.Millisecond, Concurrency: concurrency})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := batcher.Close(t.Context()); err != nil {
			t.Error(err)
		}
	}()
	store := exerciseStore{PersistentSearchIndexStore: local.NewPersistentSearchIndexStore("exercise", filepath.Join(root, "index"), "exercise"), fail: os.Getenv("SDD_INDEX_EXERCISE_FAIL") == "1"}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{Project: sdd.ProjectRef{ID: "exercise"}, Graph: graph, SearchIndex: store, Embedder: batcher, ExcludeEmbeddedFromIndex: true, LLM: llm.RunnerFunc(func(context.Context, llm.Request) (llm.Result, error) { return llm.Result{}, nil })})
	if err != nil {
		t.Fatal(err)
	}
	source, err := graph.AcquireSnapshot(t.Context(), sdd.SnapshotReadQuery{})
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := source.Release(); err != nil {
			t.Error(err)
		}
	}()
	jobs := make(chan sdd.SearchEntryDescriptor, 4)
	var mu sync.Mutex
	report := exerciseReport{}
	started := time.Now()
	var workers sync.WaitGroup
	for range 8 {
		workers.Go(func() {
			for entry := range jobs {
				ctx := embed.WithCaller(t.Context(), entry.Version.EntryID)
				err := runtime.IndexSearchEntry(ctx, sdd.IndexSearchEntryCmd{Entry: entry, OnPublished: func(id string, _ int) { mu.Lock(); report.Published++; fmt.Println("COMMITTED", id); mu.Unlock() }})
				if err != nil {
					mu.Lock()
					report.Failed++
					mu.Unlock()
					if !strings.Contains(err.Error(), "injected entry publication failure") {
						t.Error(err)
					}
				}
			}
		})
	}
	for item, err := range runtime.DiscoverSearchEntries(t.Context(), sdd.DiscoverSearchEntriesQuery{Source: source}) {
		if err != nil {
			t.Fatal(err)
		}
		if item.Published {
			report.Skipped++
			continue
		}
		jobs <- item.Entry
	}
	close(jobs)
	queryStart := time.Now()
	if _, err := observed.Embed(t.Context(), embed.Request{Purpose: embed.PurposeQuery, Texts: []string{"How does durable indexing preserve progress?"}}); err != nil {
		t.Fatal(err)
	}
	report.QueryLatency = time.Since(queryStart)
	workers.Wait()
	report.Elapsed = time.Since(started)
	stats.mu.Lock()
	report.Calls = stats.Calls
	report.Tokens = stats.Tokens
	report.Shared = stats.Shared
	report.MaxItems = stats.MaxItems
	stats.mu.Unlock()
	report.Peak = peak.Load()
	raw, err := json.Marshal(report)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "report.json"), raw, 0644); err != nil {
		t.Fatal(err)
	}
}

func TestEntryIndexingInterruptionExercise(t *testing.T) {
	if testing.Short() {
		t.Skip("subprocess interruption exercise")
	}
	for _, concurrency := range []int{1, 2} {
		t.Run(fmt.Sprintf("provider-concurrency-%d", concurrency), func(t *testing.T) {
			root := t.TempDir()
			dir := filepath.Join(root, "graph")
			for i := range 16 {
				suffix := fmt.Sprintf("e%02d", i)
				sections := 6
				if i == 0 {
					sections = 40
				}
				var body strings.Builder
				for section := range sections {
					fmt.Fprintf(&body, "\n## Topic %d\n\n", section)
					body.WriteString(strings.Repeat("Independent entry indexing preserves published progress while other entries retry. A search target fixes the versions selected before preparation. ", 12))
				}
				putSearchEntry(t, dir, suffix, body.String())
			}
			putSearchEntry(t, dir, "bad", "This entry has an injected storage failure.")
			run := func(interrupt, fail bool) exerciseReport {
				cmd := exec.CommandContext(t.Context(), os.Args[0], "-test.run=^TestEntryIndexingExerciseWorker$", "-test.timeout=5m")
				flag := "0"
				if fail {
					flag = "1"
				}
				cmd.Env = append(os.Environ(), "SDD_INDEX_EXERCISE_ROOT="+root, "SDD_INDEX_EXERCISE_CONCURRENCY="+strconv.Itoa(concurrency), "SDD_INDEX_EXERCISE_FAIL="+flag)
				output, err := cmd.StdoutPipe()
				if err != nil {
					t.Fatal(err)
				}
				var stderr strings.Builder
				cmd.Stderr = &stderr
				if err := cmd.Start(); err != nil {
					t.Fatal(err)
				}
				scanner := bufio.NewScanner(output)
				committed := 0
				var transcript strings.Builder
				for scanner.Scan() {
					line := scanner.Text()
					transcript.WriteString(line + "\n")
					if strings.HasPrefix(line, "COMMITTED ") {
						committed++
						if interrupt && committed == 2 {
							if err := cmd.Process.Kill(); err != nil {
								t.Fatal(err)
							}
						}
					}
				}
				err = cmd.Wait()
				if !interrupt && err != nil {
					t.Fatalf("worker: %v\n%s\n%s", err, transcript.String(), stderr.String())
				}
				if interrupt {
					if committed < 2 {
						t.Fatalf("no durable progress before interruption: %s %s", transcript.String(), stderr.String())
					}
					return exerciseReport{Published: committed}
				}
				raw, err := os.ReadFile(filepath.Join(root, "report.json"))
				if err != nil {
					t.Fatal(err)
				}
				var report exerciseReport
				if err := json.Unmarshal(raw, &report); err != nil {
					t.Fatal(err)
				}
				return report
			}
			interrupted := run(true, true)
			resumed := run(false, true)
			if resumed.Skipped < 2 || resumed.Published+resumed.Skipped != 16 || resumed.Failed != 1 {
				t.Fatalf("restart lost progress or sibling isolation: %+v", resumed)
			}
			completed := run(false, false)
			if completed.Skipped != 16 || completed.Published != 1 || completed.Failed != 0 {
				t.Fatalf("final convergence: %+v", completed)
			}
			noop := run(false, false)
			if noop.Skipped != 17 || noop.Published != 0 || noop.Calls != 1 {
				t.Fatalf("published versions re-embedded: %+v", noop)
			}
			t.Logf("model=%q interrupted=%+v resumed=%+v completed=%+v noop=%+v", os.Getenv("SDD_LIVE_EMBED_MODEL"), interrupted, resumed, completed, noop)
		})
	}
}
