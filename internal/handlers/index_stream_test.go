package handlers_test

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"testing"

	"github.com/networkteam/sdd/internal/chunking"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/pkg/application/types"
	"github.com/networkteam/sdd/pkg/llm/embed"
	"github.com/networkteam/sdd/pkg/local"
)

type streamAttachments struct{ reads map[string]int }

func (a streamAttachments) ReadAttachment(_ context.Context, e *model.Entry, _ string) ([]byte, error) {
	a.reads[e.ID]++
	return []byte("Attachment for " + e.ID), nil
}

type streamStore struct {
	*local.MemorySearchIndexStore
	rows map[string][]types.IndexedChunk
}

func (s streamStore) PublishEntry(ctx context.Context, version types.SearchEntryVersion, rows []types.IndexedChunk) error {
	if err := s.MemorySearchIndexStore.PublishEntry(ctx, version, rows); err != nil {
		return err
	}
	s.rows[version.EntryID] = rows
	return nil
}

func TestReconcilePacksIncrementallyAndRetainsPublishedEntries(t *testing.T) {
	ctx := t.Context()
	ns := types.IndexNamespace{Project: "test", Fingerprint: "test", Metric: "cosine"}
	var entries []*model.Entry
	hashes := map[string]string{}
	attachments := streamAttachments{reads: map[string]int{}}
	for _, id := range []string{"a", "b", "c"} {
		e := &model.Entry{ID: id, Summary: "Summary " + id, Content: "Body " + id, Attachments: []string{id + ".md"}}
		entries = append(entries, e)
		var err error
		hashes[id], err = chunking.EntryStateHash(ctx, e, attachments)
		if err != nil {
			t.Fatal(err)
		}
	}
	clear(attachments.reads)
	store := streamStore{MemorySearchIndexStore: local.NewMemorySearchIndexStore(), rows: map[string][]types.IndexedChunk{}}
	failed := errors.New("provider unavailable")
	var sizes []int
	vectors := map[string]float32{}
	fail := true
	inner := embed.EmbedderFunc{Space: "test", Run: func(_ context.Context, req embed.Request) (embed.Result, error) {
		sizes = append(sizes, len(req.Texts))
		if fail && len(sizes) == 1 && attachments.reads["c"] != 0 {
			t.Fatal("derived later entry before consuming first batch")
		}
		if fail && len(sizes) == 2 {
			return embed.Result{}, failed
		}
		result := embed.Result{}
		for _, text := range req.Texts {
			if vectors[text] == 0 {
				vectors[text] = float32(len(vectors) + 1)
			}
			result.Vectors = append(result.Vectors, []float32{vectors[text], 1})
		}
		return result, nil
	}}
	h := handlers.SearchIndexHandler{Graph: model.NewGraph(entries), Namespace: ns, Hashes: hashes, Store: store, Attachments: attachments, Embedder: handlers.IndexEmbedder{Embedder: inner, BatchSize: 4}}
	if err := h.Reconcile(ctx, command.ReconcileSearchIndexCmd{}); !errors.Is(err, failed) {
		t.Fatalf("error = %v", err)
	}
	if len(store.rows) != 1 || len(store.rows["a"]) != 3 {
		t.Fatalf("partial publication: %v", store.rows)
	}
	if !reflect.DeepEqual(sizes, []int{4, 4}) {
		t.Fatalf("batches = %v", sizes)
	}
	fail = false
	sizes = nil
	clear(attachments.reads)
	if err := h.Reconcile(ctx, command.ReconcileSearchIndexCmd{}); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(sizes, []int{4, 2}) {
		t.Fatalf("resumed batches = %v", sizes)
	}
	if attachments.reads["a"] != 0 {
		t.Fatal("published entry prepared again")
	}
	for id, rows := range store.rows {
		if len(rows) != 3 {
			t.Fatalf("entry %s has %d rows", id, len(rows))
		}
		for _, row := range rows {
			if row.Chunk.EntryID != id || row.Vector[0] != vectors[row.Chunk.Text] {
				t.Fatalf("misrouted vector: %+v", row)
			}
		}
	}
	sizes = nil
	if err := h.Reconcile(ctx, command.ReconcileSearchIndexCmd{}); err != nil || len(sizes) != 0 {
		t.Fatalf("warm reconcile: %v, %v", sizes, err)
	}
}

func TestReconcileEmptyAndOversizedEntry(t *testing.T) {
	ns := types.IndexNamespace{Project: "test", Fingerprint: "test", Metric: "cosine"}
	entries := []*model.Entry{{ID: "empty"}, {ID: "large", Summary: "Summary", Content: "## One\nFirst.\n\n## Two\nSecond.\n\n## Three\nThird."}}
	store := streamStore{MemorySearchIndexStore: local.NewMemorySearchIndexStore(), rows: map[string][]types.IndexedChunk{}}
	calls := 0
	inner := embed.EmbedderFunc{Space: "test", Run: func(_ context.Context, req embed.Request) (embed.Result, error) {
		calls++
		if len(req.Texts) > 2 {
			t.Fatal("oversized provider request")
		}
		if _, exists := store.rows["large"]; exists {
			t.Fatal("entry published before embedding finished")
		}
		out := embed.Result{}
		for range req.Texts {
			out.Vectors = append(out.Vectors, []float32{1, 2})
		}
		return out, nil
	}}
	h := handlers.SearchIndexHandler{Graph: model.NewGraph(entries), Namespace: ns, Store: store, Embedder: handlers.IndexEmbedder{Embedder: inner, BatchSize: 2}}
	if err := h.Reconcile(t.Context(), command.ReconcileSearchIndexCmd{}); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.rows["empty"]; !ok || calls < 2 {
		t.Fatalf("empty publication or oversized splitting missing: %v, %d", store.rows, calls)
	}
	before := calls
	if err := h.Reconcile(t.Context(), command.ReconcileSearchIndexCmd{}); err != nil || calls != before {
		t.Fatalf("retry: %v, calls %d", err, calls)
	}
}

func TestReconcileRejectsInvalidVectors(t *testing.T) {
	for _, vectors := range [][][]float32{nil, {{}}, {{0, 0}}} {
		t.Run(fmt.Sprint(vectors), func(t *testing.T) {
			store := local.NewMemorySearchIndexStore()
			h := handlers.SearchIndexHandler{Graph: model.NewGraph([]*model.Entry{{ID: "a", Summary: "One"}}), Namespace: types.IndexNamespace{Project: "test", Fingerprint: "test", Metric: "cosine"}, Store: store, Embedder: embed.EmbedderFunc{Space: "test", Run: func(context.Context, embed.Request) (embed.Result, error) { return embed.Result{Vectors: vectors}, nil }}}
			if err := h.Reconcile(t.Context(), command.ReconcileSearchIndexCmd{}); err == nil {
				t.Fatal("invalid vectors accepted")
			}
		})
	}
}

type streamGraphReader struct {
	handlers.Reader
	graph *model.Graph
}

func (r streamGraphReader) CurrentGraph(string) (*model.Graph, error) { return r.graph, nil }

func TestCLIIndexPublishesBeforeProgressAndResumes(t *testing.T) {
	entries := []*model.Entry{{ID: "a", Summary: "Summary A", Content: "Body A"}, {ID: "b", Summary: "Summary B", Content: "Body B"}, {ID: "c", Summary: "Summary C", Content: "Body C"}}
	dir := t.TempDir()
	calls := 0
	stop := errors.New("interrupted")
	emb := embed.EmbedderFunc{Space: "test", Run: func(_ context.Context, req embed.Request) (embed.Result, error) {
		calls++
		if calls == 2 {
			return embed.Result{}, stop
		}
		result := embed.Result{}
		for range req.Texts {
			result.Vectors = append(result.Vectors, []float32{1, 2})
		}
		return result, nil
	}}
	h := handlers.NewIndexHandler(handlers.IndexHandlerOptions{GraphDir: t.TempDir(), IndexDir: dir, Reader: streamGraphReader{graph: model.NewGraph(entries)}, Embedder: handlers.IndexEmbedder{Embedder: emb, BatchSize: 3}})
	var planned, published int
	cmd := &command.BuildIndexCmd{OnPlanned: func(n int) { planned = n }, OnEntryIndexed: func(id string, _ int) {
		manifest, err := index.LoadManifest(dir)
		if err != nil || len(manifest.Entries[id].Versions) == 0 {
			t.Fatalf("progress preceded durable manifest: %s %v", id, err)
		}
		published++
	}}
	if err := h.Build(t.Context(), cmd); !errors.Is(err, stop) {
		t.Fatalf("error = %v", err)
	}
	if planned != 3 || published != 1 {
		t.Fatalf("planned %d published %d", planned, published)
	}
	if err := h.Build(t.Context(), cmd); err != nil {
		t.Fatal(err)
	}
	if planned != 2 || published != 3 {
		t.Fatalf("resumed planned %d published %d", planned, published)
	}
}
