package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	sdd "github.com/networkteam/sdd/application"
	localadapter "github.com/networkteam/sdd/local"
)

// These tests pin the monotonic, entry-presence reconciliation of the
// persistent vector index through the real public adapter. They assert on
// embedding counters and store identity — never elapsed time — so a
// regression that re-embeds the whole graph on every search shows up as a
// non-zero document-embed count.

// countingEmbeddings records how many document and query inputs it embedded,
// and the IDs of the document inputs, so a test can prove which entries were
// (re-)embedded. Vectors key off the "alpha"/"beta" tokens so ranking is
// deterministic; length is constant so chromem never sees a dimension change.
type countingEmbeddings struct {
	mu          sync.Mutex
	docEmbeds   int
	queryEmbeds int
	docInputIDs []string
}

type branchVectorTargets struct {
	mu           sync.Mutex
	graph        sdd.GraphStore
	acquisitions int
	releases     int
	active       bool
}

func (t *branchVectorTargets) Acquire(_ context.Context, target sdd.MutationTarget) (*sdd.AcquiredTarget, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.acquisitions++
	t.active = true
	return &sdd.AcquiredTarget{
		Target: target,
		Graph:  t.graph,
		Release: func() error {
			t.mu.Lock()
			defer t.mu.Unlock()
			t.releases++
			t.active = false
			return nil
		},
	}, nil
}

type failingAttachmentGraphStore struct {
	sdd.GraphStore
	err error
}

func (s failingAttachmentGraphStore) ReadAttachmentPage(context.Context, string, string, int64, int) (sdd.AttachmentPage, error) {
	return sdd.AttachmentPage{}, s.err
}

func (e *countingEmbeddings) Spec(context.Context) (sdd.EmbeddingSpec, error) {
	return sdd.EmbeddingSpec{Fingerprint: "counter/v1"}, nil
}

func (e *countingEmbeddings) Embed(_ context.Context, inputs []sdd.EmbeddingInput) ([]sdd.EmbeddingVector, error) {
	e.mu.Lock()
	defer e.mu.Unlock()
	out := make([]sdd.EmbeddingVector, len(inputs))
	for i, in := range inputs {
		if in.Purpose == sdd.EmbeddingQuery {
			e.queryEmbeds++
		} else {
			e.docEmbeds++
			e.docInputIDs = append(e.docInputIDs, in.ID)
		}
		out[i] = sdd.EmbeddingVector{ID: in.ID, Values: keywordVec(in.Text)}
	}
	return out, nil
}

func (e *countingEmbeddings) reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.docEmbeds, e.queryEmbeds, e.docInputIDs = 0, 0, nil
}

func keywordVec(text string) []float32 {
	tl := strings.ToLower(text)
	v := []float32{0, 0, 0.1}
	if strings.Contains(tl, "alpha") {
		v[0] = 1
	}
	if strings.Contains(tl, "beta") {
		v[1] = 1
	}
	return v
}

func writeCounterEntry(t *testing.T, graphDir, id, summary, body string) {
	t.Helper()
	dir := filepath.Join(graphDir, id[:4], id[4:6])
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntype: signal\nkind: gap\nlayer: tactical\nconfidence: high\nparticipants:\n  - Christopher\n"
	content += "summary: " + summary + "\n---\n\n" + body + "\n"
	if err := os.WriteFile(filepath.Join(dir, id[6:]+".md"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

const counterProject = sdd.ProjectID("counter")

func newCounterApp(t *testing.T, graphDir, cacheRoot string, embeddings sdd.EmbeddingExecutor) *sdd.Application {
	t.Helper()
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: counterProject, GraphDir: graphDir})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := localadapter.NewFilesystemSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project:     sdd.ProjectRef{ID: counterProject, DisplayName: "Counter"},
		Graph:       graph,
		Sessions:    sessions,
		StagedBlobs: blobs,
		Embeddings:  embeddings,
		SearchIndex: localadapter.NewPersistentSearchIndexStore(counterProject, cacheRoot, "counter/repo"),
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
			ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) {
				return sdd.LLMResult{ExecutorFingerprint: "test"}, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	application, err := sdd.NewApplication(&runtimeAccessResolver{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	return application
}

func counterSearch(t *testing.T, application *sdd.Application, phrase string) sdd.SearchResult {
	t.Helper()
	res, err := application.Search(t.Context(), sdd.RequestIdentity{Subject: "christopher"}, counterProject, sdd.SearchRequest{Phrase: phrase, Limit: 5, MaxCitations: 3})
	if err != nil {
		t.Fatalf("Search(%q): %v", phrase, err)
	}
	return res
}

func seedCounterGraph(t *testing.T) (graphDir, cacheRoot string) {
	t.Helper()
	graphDir = t.TempDir()
	cacheRoot = t.TempDir()
	writeCounterEntry(t, graphDir, "20260101-100000-s-tac-aaa", "Alpha topic summary", "## Section\nThe alpha entry is about alpha matters.")
	writeCounterEntry(t, graphDir, "20260101-100001-s-tac-bbb", "Beta topic summary", "## Section\nThe beta entry is about beta matters.")
	return graphDir, cacheRoot
}

func TestVectorSearchPreIndexedGraphEmbedsOnlyQuery(t *testing.T) {
	graphDir, cacheRoot := seedCounterGraph(t)
	emb := &countingEmbeddings{}
	app := newCounterApp(t, graphDir, cacheRoot, emb)

	// First search warms the index — every entry's chunks embed once.
	first := counterSearch(t, app, "alpha")
	if !strings.Contains(first.Results, "s-tac-aaa") {
		t.Fatalf("first search did not surface the alpha entry: %q", first.Results)
	}
	if emb.docEmbeds == 0 {
		t.Fatal("first search embedded no documents — the graph was never indexed")
	}

	emb.reset()
	second := counterSearch(t, app, "alpha")
	if emb.docEmbeds != 0 {
		t.Errorf("second search re-embedded %d documents against a warm index; want 0", emb.docEmbeds)
	}
	if emb.queryEmbeds != 1 {
		t.Errorf("second search made %d query embeds; want exactly 1", emb.queryEmbeds)
	}
	if !strings.Contains(second.Results, "s-tac-aaa") {
		t.Errorf("second search lost the alpha entry: %q", second.Results)
	}
}

func TestVectorSearchUnrelatedRevisionChangeEmbedsNothing(t *testing.T) {
	graphDir, cacheRoot := seedCounterGraph(t)
	emb := &countingEmbeddings{}
	app := newCounterApp(t, graphDir, cacheRoot, emb)
	counterSearch(t, app, "alpha") // warm

	// Bump the graph revision without adding an indexable entry: a non-entry
	// file changes the directory hash but not the entry set. Reconciliation
	// ignores revision, so no document re-embeds.
	if err := os.WriteFile(filepath.Join(graphDir, "unrelated.txt"), []byte("revision bump"), 0o644); err != nil {
		t.Fatal(err)
	}
	emb.reset()
	counterSearch(t, app, "alpha")
	if emb.docEmbeds != 0 {
		t.Errorf("revision change triggered %d document embeds; want 0 (revision is not a freshness token)", emb.docEmbeds)
	}
	if emb.queryEmbeds != 1 {
		t.Errorf("query embeds = %d; want 1", emb.queryEmbeds)
	}
}

func TestVectorSearchNewEntryEmbedsOnlyItsChunks(t *testing.T) {
	graphDir, cacheRoot := seedCounterGraph(t)
	emb := &countingEmbeddings{}
	app := newCounterApp(t, graphDir, cacheRoot, emb)
	counterSearch(t, app, "alpha") // warm

	const newID = "20260101-100002-s-tac-ccc"
	writeCounterEntry(t, graphDir, newID, "Gamma topic summary", "## Section\nThe gamma entry mentions alpha in passing.")
	emb.reset()
	counterSearch(t, app, "alpha")

	if emb.docEmbeds == 0 {
		t.Fatal("adding an entry embedded nothing")
	}
	for _, id := range emb.docInputIDs {
		if !strings.HasPrefix(id, newID) {
			t.Errorf("embedded a chunk %q not belonging to the new entry %q", id, newID)
		}
	}
}

func TestVectorSearchRestartEmbedsNothing(t *testing.T) {
	graphDir, cacheRoot := seedCounterGraph(t)
	warm := newCounterApp(t, graphDir, cacheRoot, &countingEmbeddings{})
	counterSearch(t, warm, "alpha") // warm the on-disk store

	// A fresh process: new application, new adapter, same cache directory.
	emb := &countingEmbeddings{}
	restarted := newCounterApp(t, graphDir, cacheRoot, emb)
	res := counterSearch(t, restarted, "alpha")
	if emb.docEmbeds != 0 {
		t.Errorf("restart re-embedded %d documents; want 0 (the on-disk index is warm)", emb.docEmbeds)
	}
	if emb.queryEmbeds != 1 {
		t.Errorf("restart query embeds = %d; want 1", emb.queryEmbeds)
	}
	if !strings.Contains(res.Results, "s-tac-aaa") {
		t.Errorf("restart lost the alpha entry: %q", res.Results)
	}
}

func TestVectorSearchNarrowerFilterNoDeletesNoReembed(t *testing.T) {
	graphDir, cacheRoot := seedCounterGraph(t)
	emb := &countingEmbeddings{}
	app := newCounterApp(t, graphDir, cacheRoot, emb)
	counterSearch(t, app, "alpha") // warm

	before := indexedEntryCount(t, cacheRoot)

	emb.reset()
	// A narrower request (decisions only, though the graph holds signals) must
	// neither re-embed nor delete stored vectors.
	if _, err := app.Search(t.Context(), sdd.RequestIdentity{Subject: "christopher"}, counterProject,
		sdd.SearchRequest{Phrase: "alpha", Type: "d", Limit: 5, MaxCitations: 1}); err != nil {
		t.Fatalf("filtered Search: %v", err)
	}
	if emb.docEmbeds != 0 {
		t.Errorf("filtered search re-embedded %d documents; want 0", emb.docEmbeds)
	}
	if after := indexedEntryCount(t, cacheRoot); after != before {
		t.Errorf("filtered search changed indexed entry count from %d to %d (a filter must never delete)", before, after)
	}
}

func TestVectorSearchAttachmentHitPreservesCitation(t *testing.T) {
	graphDir := t.TempDir()
	cacheRoot := t.TempDir()
	const id = "20260101-100000-s-tac-aaa"
	writeCounterEntry(t, graphDir, id, "Alpha topic summary", "## Section\nAlpha overview.")
	attachDir := filepath.Join(graphDir, id[:4], id[4:6], id[6:])
	if err := os.MkdirAll(attachDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(attachDir, "design.md"), []byte("# Design\n\nThe alpha design note elaborates the alpha approach in detail."), 0o644); err != nil {
		t.Fatal(err)
	}

	app := newCounterApp(t, graphDir, cacheRoot, &countingEmbeddings{})
	res := counterSearch(t, app, "alpha")
	if !strings.Contains(res.Results, "[attachment: ") || !strings.Contains(res.Results, "design.md") {
		t.Errorf("attachment hit did not render a source citation: %q", res.Results)
	}
}

func TestBranchVectorAndHybridSearchUseSelectedAttachmentAuthorityAndRelease(t *testing.T) {
	const id = "20260101-100000-s-tac-aaa"
	baseDir := t.TempDir()
	branchDir := t.TempDir()
	for _, graphDir := range []string{baseDir, branchDir} {
		writeCounterEntry(t, graphDir, id, "Neutral attachment summary", "## Section\nNeutral entry body.")
		attachDir := filepath.Join(graphDir, id[:4], id[4:6], id[6:])
		if err := os.MkdirAll(attachDir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(baseDir, id[:4], id[4:6], id[6:], "design.md"), []byte("# Design\n\nalpha runtime authority bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(branchDir, id[:4], id[4:6], id[6:], "design.md"), []byte("# Design\n\nbeta branch authority bytes"), 0o644); err != nil {
		t.Fatal(err)
	}
	base, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: counterProject, GraphDir: baseDir})
	if err != nil {
		t.Fatal(err)
	}
	branch, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: counterProject, GraphDir: branchDir})
	if err != nil {
		t.Fatal(err)
	}
	targets := &branchVectorTargets{graph: branch}
	app := newBranchCounterApp(t, base, targets, t.TempDir(), &countingEmbeddings{})
	identity := sdd.RequestIdentity{Subject: "christopher"}

	for name, request := range map[string]sdd.SearchRequest{
		"vector": {Phrase: "beta", Branch: "work", Limit: 5, MaxCitations: 3},
		"hybrid": {Terms: []string{"beta"}, Phrase: "beta", Branch: "work", Limit: 5, MaxCitations: 3},
	} {
		result, err := app.Search(t.Context(), identity, counterProject, request)
		if err != nil {
			t.Fatalf("%s search: %v", name, err)
		}
		if !strings.Contains(result.Results, "beta branch authority bytes") || strings.Contains(result.Results, "alpha runtime authority bytes") {
			t.Fatalf("%s search used wrong attachment authority:\n%s", name, result.Results)
		}
	}
	targets.mu.Lock()
	acquisitions, releases, active := targets.acquisitions, targets.releases, targets.active
	targets.mu.Unlock()
	if acquisitions != 2 || releases != 2 || active {
		t.Fatalf("success lifecycle acquisitions=%d releases=%d active=%v", acquisitions, releases, active)
	}

	attachmentErr := errors.New("selected attachment read failed")
	failingTargets := &branchVectorTargets{graph: failingAttachmentGraphStore{GraphStore: branch, err: attachmentErr}}
	failing := newBranchCounterApp(t, base, failingTargets, t.TempDir(), &countingEmbeddings{})
	_, err = failing.Search(t.Context(), identity, counterProject, sdd.SearchRequest{
		Phrase: "beta", Branch: "work", Limit: 5, MaxCitations: 3,
	})
	if !errors.Is(err, attachmentErr) {
		t.Fatalf("attachment failure = %v", err)
	}
	failingTargets.mu.Lock()
	acquisitions, releases, active = failingTargets.acquisitions, failingTargets.releases, failingTargets.active
	failingTargets.mu.Unlock()
	if acquisitions != 1 || releases != 1 || active {
		t.Fatalf("error lifecycle acquisitions=%d releases=%d active=%v", acquisitions, releases, active)
	}
}

func newBranchCounterApp(t *testing.T, base sdd.GraphStore, targets sdd.TargetAcquirer, cacheRoot string, embeddings sdd.EmbeddingExecutor) *sdd.Application {
	t.Helper()
	sessions, err := localadapter.NewFilesystemSessionStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStore(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project: sdd.ProjectRef{ID: counterProject, DisplayName: "Counter"}, DefaultBranch: "main",
		Graph: base, Targets: targets, Sessions: sessions, StagedBlobs: blobs,
		Embeddings:  embeddings,
		SearchIndex: localadapter.NewPersistentSearchIndexStore(counterProject, cacheRoot, "counter/branch"),
		LLM: sdd.LLMExecutorFuncs{
			CapabilitiesFunc: func(context.Context) ([]string, error) { return nil, nil },
			ExecuteFunc: func(context.Context, sdd.LLMRequest) (sdd.LLMResult, error) {
				return sdd.LLMResult{ExecutorFingerprint: "test"}, nil
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	app, err := sdd.NewApplication(&runtimeAccessResolver{runtime: runtime})
	if err != nil {
		t.Fatal(err)
	}
	return app
}

func TestVectorSearchIgnoresPersistedRowAbsentFromGraph(t *testing.T) {
	graphDir, cacheRoot := seedCounterGraph(t)
	app := newCounterApp(t, graphDir, cacheRoot, &countingEmbeddings{})
	counterSearch(t, app, "beta") // index both entries

	// Remove the beta entry from the graph; its rows remain in the store.
	if err := os.Remove(filepath.Join(graphDir, "2026", "01", "01-100001-s-tac-bbb.md")); err != nil {
		t.Fatal(err)
	}
	res := counterSearch(t, app, "beta")
	if strings.Contains(res.Results, "s-tac-bbb") {
		t.Errorf("a stored hit whose entry no longer exists was surfaced: %q", res.Results)
	}
}

// indexedEntryCount reads the persistent manifest through the adapter and
// returns how many entries are indexed for the counter namespace.
func indexedEntryCount(t *testing.T, cacheRoot string) int {
	t.Helper()
	store := localadapter.NewPersistentSearchIndexStore(counterProject, cacheRoot, "counter/repo")
	refs, err := store.IndexedEntries(t.Context(), sdd.IndexNamespace{Project: counterProject, Fingerprint: "counter/v1", Metric: "cosine"})
	if err != nil {
		t.Fatalf("IndexedEntries: %v", err)
	}
	return len(refs)
}
