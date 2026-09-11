package local_test

import (
	"context"
	"testing"

	"github.com/networkteam/sdd/internal/index"
	app "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/local"
)

// Versions the server writes carry the derivation rule, so `sdd index gc`
// groups them without inferring from chunk-ID shape.
func TestPersistentStoreRecordsDerivation(t *testing.T) {
	cacheRoot := canonicalTempDir(t)
	const project = app.ProjectID("derivation")
	const repoKey = "derivation/repo"
	ns := app.IndexNamespace{Project: project, Fingerprint: "fp", Metric: "cosine"}
	store := local.NewPersistentSearchIndexStore(project, cacheRoot, repoKey)
	chunk := app.IndexedChunk{Chunk: app.CanonicalChunk{
		ID: "e1#v-a#summary", EntryID: "e1", EntryHash: "a", ContentHash: "c1", Text: "alpha", Body: "alpha", IsSummary: true,
	}, Vector: []float32{1, 0}}
	if err := store.Reconcile(context.Background(), ns, "r1", []app.IndexedChunk{chunk}, nil); err != nil {
		t.Fatal(err)
	}
	manifest, err := index.LoadManifest(index.StoreDir(cacheRoot, repoKey, ns.Fingerprint))
	if err != nil {
		t.Fatal(err)
	}
	if got := manifest.Entries["e1"].Versions; len(got) != 1 || got[0].Derivation != index.DerivationCurrent {
		t.Fatalf("stored versions = %+v, want one under %q", got, index.DerivationCurrent)
	}
}
