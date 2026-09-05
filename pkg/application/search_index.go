package application

import (
	"context"
	"fmt"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/query"
)

type ReconcileSearchIndexCmd = command.ReconcileSearchIndexCmd
type SearchSyncMode = query.SearchSyncMode

const (
	SearchSyncNone  = query.SearchSyncNone
	SearchSyncLocal = query.SearchSyncLocal
	SearchSyncAll   = query.SearchSyncAll
)

// ReconcileSearchIndex maintains the runtime's current graph index. The host
// authorizes the call; this operation does not resolve a request identity.
func (r *ProjectRuntime) ReconcileSearchIndex(ctx context.Context, cmd ReconcileSearchIndexCmd) error {
	namespace, err := r.indexNamespace()
	if err != nil {
		return err
	}
	snapshot, err := r.options.Graph.Current(ctx)
	if err != nil {
		return err
	}
	hashes, err := r.currentEntryHashes(ctx, snapshot, r.options.Graph)
	if err != nil {
		return err
	}
	return r.reconcileSearchSnapshot(ctx, snapshot, r.options.Graph, namespace, hashes, cmd)
}

func (r *ProjectRuntime) indexNamespace() (IndexNamespace, error) {
	if r.options.Embedder == nil || r.options.SearchIndex == nil {
		return IndexNamespace{}, errVectorUnavailable
	}
	fingerprint := r.options.Embedder.Fingerprint()
	if fingerprint == "" {
		return IndexNamespace{}, fmt.Errorf("sdd: embedder returned an empty fingerprint")
	}
	return IndexNamespace{Project: r.options.Project.ID, Fingerprint: fingerprint, Metric: "cosine"}, nil
}

func (r *ProjectRuntime) reconcileSearchSnapshot(ctx context.Context, snapshot *Snapshot, store GraphStore, namespace IndexNamespace, hashes map[string]string, cmd ReconcileSearchIndexCmd) error {
	h := handlers.SearchIndexHandler{
		Store: r.options.SearchIndex, Embedder: r.options.Embedder,
		Graph: snapshot.graph, Revision: snapshot.revision, Namespace: namespace, Hashes: hashes,
		Attachments:     graphStoreAttachmentReader{store: store},
		ExcludeEmbedded: r.options.ExcludeEmbeddedFromIndex,
	}
	return h.Reconcile(ctx, cmd)
}
