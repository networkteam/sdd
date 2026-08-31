package sddtest

import (
	"bytes"
	"context"
	"io"
	"slices"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
)

type AccessResolverFixture struct {
	Resolver     sdd.AccessResolver
	Identity     sdd.RequestIdentity
	Principal    sdd.Principal
	Project      sdd.ProjectID
	Dependency   string
	ProjectCount int
}

func RunAccessResolverTests(t *testing.T, factory func(*testing.T) AccessResolverFixture) {
	t.Helper()
	fixture := factory(t)
	principal, err := fixture.Resolver.ResolvePrincipal(t.Context(), fixture.Identity)
	if err != nil {
		t.Fatalf("ResolvePrincipal: %v", err)
	}
	if principal != fixture.Principal {
		t.Fatalf("ResolvePrincipal = %+v, want %+v", principal, fixture.Principal)
	}
	projects, err := fixture.Resolver.ListProjects(t.Context(), principal)
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if len(projects.Projects) != fixture.ProjectCount {
		t.Fatalf("ListProjects returned %d projects, want %d", len(projects.Projects), fixture.ProjectCount)
	}
	if _, err := fixture.Resolver.ResolveProject(t.Context(), principal, fixture.Project, sdd.AccessRead); err != nil {
		t.Fatalf("ResolveProject(read): %v", err)
	}
	if fixture.Dependency != "" {
		if _, err := fixture.Resolver.ResolveDependency(t.Context(), principal, fixture.Project, fixture.Dependency); err != nil {
			t.Fatalf("ResolveDependency: %v", err)
		}
	}
}

type GraphStoreFixture struct {
	Store           sdd.GraphStore
	InitialRevision string
	Batch           sdd.MutationBatch
	Blobs           sdd.StagedBlobReader
	AttachmentEntry string
	AttachmentName  string
	// SecondBatch, when set, exercises the merge-under-append guarantee: it
	// must target paths unrelated to Batch so it can apply cleanly against the
	// revision Batch advanced the store to.
	SecondBatch sdd.MutationBatch
}

func RunGraphStoreTests(t *testing.T, factory func(*testing.T) GraphStoreFixture) {
	t.Helper()
	fixture := factory(t)
	snapshot, err := fixture.Store.Current(t.Context())
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if snapshot == nil || snapshot.Revision() != fixture.InitialRevision {
		t.Fatalf("Current revision = %q, want %q", snapshot.Revision(), fixture.InitialRevision)
	}
	stale, staleErr := fixture.Store.Apply(t.Context(), fixture.InitialRevision+"-stale", fixture.Batch, fixture.Blobs)
	if stale.State != sdd.MutationNotApplied {
		t.Fatalf("stale Apply = %+v (error %v), want not_applied", stale, staleErr)
	}
	applied, err := fixture.Store.Apply(t.Context(), fixture.InitialRevision, fixture.Batch, fixture.Blobs)
	if err != nil {
		t.Fatalf("Apply: %v", err)
	}
	if applied.State != sdd.MutationApplied || applied.Revision == "" {
		t.Fatalf("Apply = %+v, want applied with revision", applied)
	}
	reconciled, err := fixture.Store.Reconcile(t.Context(), fixture.Batch.ID, fixture.Batch.Digest)
	if err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	if reconciled != applied {
		t.Fatalf("Reconcile = %+v, want %+v", reconciled, applied)
	}
	if fixture.AttachmentEntry != "" {
		page, err := fixture.Store.ReadAttachmentPage(t.Context(), fixture.AttachmentEntry, fixture.AttachmentName, 0, 1)
		if err != nil {
			t.Fatalf("ReadAttachmentPage: %v", err)
		}
		if page.Filename != fixture.AttachmentName || len(page.Content) > 1 {
			t.Fatalf("ReadAttachmentPage = %+v", page)
		}
	}
	if fixture.SecondBatch.ID != "" {
		// Merge under append: SecondBatch was prepared against InitialRevision,
		// but the first Apply advanced the store, so that pin is now stale and
		// conflicts. Re-reading the fresh revision and applying there succeeds
		// cleanly — the adapter guarantee the engine's bounded-retry merge
		// depends on. A conflict must leave no ledger record blocking the retry.
		stale, staleErr := fixture.Store.Apply(t.Context(), fixture.InitialRevision, fixture.SecondBatch, fixture.Blobs)
		if stale.State != sdd.MutationNotApplied {
			t.Fatalf("stale merge Apply = %+v (error %v), want not_applied conflict", stale, staleErr)
		}
		fresh, err := fixture.Store.Current(t.Context())
		if err != nil {
			t.Fatalf("Current before merge apply: %v", err)
		}
		merged, err := fixture.Store.Apply(t.Context(), fresh.Revision(), fixture.SecondBatch, fixture.Blobs)
		if err != nil || merged.State != sdd.MutationApplied || merged.Revision == "" {
			t.Fatalf("merge-under-append Apply = %+v, %v", merged, err)
		}
	}
}

type SessionStoreFixture struct {
	Store    sdd.SessionStore
	Metadata sdd.SessionMetadata
	Append   sdd.SessionAppend
}

func RunSessionStoreTests(t *testing.T, factory func(*testing.T) SessionStoreFixture) {
	t.Helper()
	fixture := factory(t)
	created, err := fixture.Store.Create(t.Context(), fixture.Metadata)
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	next, err := fixture.Store.Append(t.Context(), fixture.Metadata.ID, created.Version, fixture.Append)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if next <= created.Version {
		t.Fatalf("Append version = %d, want > %d", next, created.Version)
	}
	if _, err := fixture.Store.Append(t.Context(), fixture.Metadata.ID, created.Version, fixture.Append); err == nil {
		t.Fatal("stale Append unexpectedly succeeded")
	}
	loaded, err := fixture.Store.Load(t.Context(), fixture.Metadata.ID)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Version != next || len(loaded.Events) != len(fixture.Append.Events) {
		t.Fatalf("Load = version %d events %d, want %d/%d", loaded.Version, len(loaded.Events), next, len(fixture.Append.Events))
	}
	if want := fixture.Metadata.Attachment; want != nil {
		got := loaded.Metadata.Attachment
		if got == nil || got.MCPSessionID != want.MCPSessionID || got.ClientName != want.ClientName {
			t.Fatalf("attachment did not round-trip: got %+v, want %+v", got, want)
		}
	}
	listed, err := fixture.Store.List(t.Context(), sdd.SessionFilter{Project: fixture.Metadata.Project})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) == 0 {
		t.Fatal("List did not return the created session")
	}

	// Collection reaches deletion through this contract, so an implementation
	// must delete and must treat an already-absent session as success.
	if err := fixture.Store.Delete(t.Context(), fixture.Metadata.ID); err != nil {
		t.Fatalf("Delete: %v", err)
	}
	if _, err := fixture.Store.Load(t.Context(), fixture.Metadata.ID); err == nil {
		t.Fatal("Load after Delete succeeded, want the session gone")
	}
	if err := fixture.Store.Delete(t.Context(), fixture.Metadata.ID); err != nil {
		t.Fatalf("second Delete = %v, want idempotent success", err)
	}
	remaining, err := fixture.Store.List(t.Context(), sdd.SessionFilter{Project: fixture.Metadata.Project})
	if err != nil {
		t.Fatalf("List after Delete: %v", err)
	}
	for _, item := range remaining {
		if item.Metadata.ID == fixture.Metadata.ID {
			t.Fatal("List still returns the deleted session")
		}
	}
}

type StagedBlobStoreFixture struct {
	Store    sdd.StagedBlobStore
	Session  sdd.SessionRef
	Filename string
	Content  []byte
}

func RunStagedBlobStoreTests(t *testing.T, factory func(*testing.T) StagedBlobStoreFixture) {
	t.Helper()
	fixture := factory(t)
	blob, err := fixture.Store.Stage(t.Context(), fixture.Session, fixture.Filename, bytes.NewReader(fixture.Content))
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if blob.Session != fixture.Session || blob.Filename != fixture.Filename || blob.Size != int64(len(fixture.Content)) {
		t.Fatalf("Stage = %+v", blob)
	}
	stat, err := fixture.Store.Stat(t.Context(), fixture.Session, blob.ID)
	if err != nil || stat != blob {
		t.Fatalf("Stat = %+v, %v; want %+v", stat, err, blob)
	}
	reader, err := fixture.Store.Open(t.Context(), fixture.Session, blob.ID)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(got, fixture.Content) {
		t.Fatalf("Open content = %q, read %v, close %v", got, readErr, closeErr)
	}
	if err := fixture.Store.Retain(t.Context(), fixture.Session, "mutation-1", []string{blob.ID}); err != nil {
		t.Fatalf("Retain: %v", err)
	}

	// Collection enumerates staging areas and deletes them through this
	// contract, so an implementation must surface the session it staged for and
	// must delete a retained blob rather than refusing while a retention holds
	// it — the sweep's rules decide what is safe to remove, not the store's.
	refs, err := fixture.Store.StagedSessions(t.Context())
	if err != nil {
		t.Fatalf("StagedSessions: %v", err)
	}
	if !slices.Contains(refs, fixture.Session) {
		t.Fatalf("StagedSessions = %+v, want it to contain %+v", refs, fixture.Session)
	}
	if err := fixture.Store.DeleteStaged(t.Context(), fixture.Session); err != nil {
		t.Fatalf("DeleteStaged: %v", err)
	}
	if _, err := fixture.Store.Stat(t.Context(), fixture.Session, blob.ID); err == nil {
		t.Fatal("Stat after DeleteStaged succeeded, want the blob gone")
	}
	if err := fixture.Store.DeleteStaged(t.Context(), fixture.Session); err != nil {
		t.Fatalf("second DeleteStaged = %v, want idempotent success", err)
	}
	afterRefs, err := fixture.Store.StagedSessions(t.Context())
	if err != nil {
		t.Fatalf("StagedSessions after DeleteStaged: %v", err)
	}
	if slices.Contains(afterRefs, fixture.Session) {
		t.Fatal("StagedSessions still returns the deleted session")
	}

	// Release on a gone staging area must not resurrect it.
	if err := fixture.Store.Release(t.Context(), fixture.Session, "mutation-1"); err != nil {
		t.Fatalf("Release: %v", err)
	}
}

type EmbeddingExecutorFixture struct {
	Executor sdd.EmbeddingExecutor
	Inputs   []sdd.EmbeddingInput
}

func RunEmbeddingExecutorTests(t *testing.T, factory func(*testing.T) EmbeddingExecutorFixture) {
	t.Helper()
	fixture := factory(t)
	spec, err := fixture.Executor.Spec(t.Context())
	if err != nil {
		t.Fatalf("Spec: %v", err)
	}
	if spec.Fingerprint == "" {
		t.Fatalf("Spec = %+v", spec)
	}
	vectors, err := fixture.Executor.Embed(t.Context(), fixture.Inputs)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vectors) != len(fixture.Inputs) {
		t.Fatalf("Embed returned %d vectors, want %d", len(vectors), len(fixture.Inputs))
	}
	dims := 0
	for i, vector := range vectors {
		if vector.ID != fixture.Inputs[i].ID || len(vector.Values) == 0 {
			t.Fatalf("vector %d = %+v", i, vector)
		}
		if dims == 0 {
			dims = len(vector.Values)
		}
		if len(vector.Values) != dims {
			t.Fatalf("vector %d has %d dimensions, want %d", i, len(vector.Values), dims)
		}
	}
}

type SearchIndexStoreFixture struct {
	Store     sdd.SearchIndexStore
	Namespace sdd.IndexNamespace
	// Chunks is the reconcile set. To exercise the full contract it should
	// carry citation metadata (Body/Breadcrumb/IsSummary/…), distinct entry
	// IDs across at least two entries, and equal-length vectors.
	Chunks []sdd.IndexedChunk
	Query  []float32
	// Reopen returns a store backed by the same state — the same instance for
	// an in-memory store, a fresh handle over the same directory for a
	// persistent store. When nil the suite reuses Store (no reopen assertion).
	Reopen func() sdd.SearchIndexStore
}

func RunSearchIndexStoreTests(t *testing.T, factory func(*testing.T) SearchIndexStoreFixture) {
	t.Helper()
	fixture := factory(t)
	ctx := t.Context()

	if err := fixture.Store.Reconcile(ctx, fixture.Namespace, "r1", fixture.Chunks, nil); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	manifest, err := fixture.Store.Manifest(ctx, fixture.Namespace)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if len(manifest) != len(fixture.Chunks) {
		t.Fatalf("Manifest returned %d chunks, want %d", len(manifest), len(fixture.Chunks))
	}

	hits, err := fixture.Store.Nearest(ctx, []sdd.IndexNamespace{fixture.Namespace}, fixture.Query, len(fixture.Chunks))
	if err != nil {
		t.Fatalf("Nearest: %v", err)
	}
	if len(hits) == 0 {
		t.Fatal("Nearest returned no hits")
	}
	byChunk := map[string]sdd.IndexedChunk{}
	for _, chunk := range fixture.Chunks {
		byChunk[chunk.Chunk.ID] = chunk
	}
	for _, hit := range hits {
		if hit.Namespace != fixture.Namespace {
			t.Fatalf("Nearest returned unauthorized namespace %+v", hit.Namespace)
		}
		want, ok := byChunk[hit.ChunkID]
		if !ok {
			t.Fatalf("Nearest returned unknown chunk %q", hit.ChunkID)
		}
		// Complete citation metadata round-trip.
		if hit.EntryID != want.Chunk.EntryID {
			t.Errorf("hit %q EntryID = %q, want %q", hit.ChunkID, hit.EntryID, want.Chunk.EntryID)
		}
		if hit.Body != want.Chunk.Body {
			t.Errorf("hit %q Body = %q, want %q", hit.ChunkID, hit.Body, want.Chunk.Body)
		}
		if !slices.Equal(hit.Breadcrumb, want.Chunk.Breadcrumb) {
			t.Errorf("hit %q Breadcrumb = %v, want %v", hit.ChunkID, hit.Breadcrumb, want.Chunk.Breadcrumb)
		}
		if hit.IsSummary != want.Chunk.IsSummary || hit.IsAttachment != want.Chunk.IsAttachment {
			t.Errorf("hit %q summary/attachment = %v/%v, want %v/%v", hit.ChunkID, hit.IsSummary, hit.IsAttachment, want.Chunk.IsSummary, want.Chunk.IsAttachment)
		}
		if hit.SourceAttachmentPath != want.Chunk.SourceAttachmentPath {
			t.Errorf("hit %q SourceAttachmentPath = %q, want %q", hit.ChunkID, hit.SourceAttachmentPath, want.Chunk.SourceAttachmentPath)
		}
	}

	// Entry-manifest reporting (optional capability): one ref per stored
	// (entry, version) pair, no migration or embedding triggered. Each
	// reconciled chunk's (EntryID, EntryHash) must be reported as present.
	if manifestCap, ok := fixture.Store.(sdd.SearchIndexEntryManifest); ok {
		refs, err := manifestCap.IndexedEntries(ctx, fixture.Namespace)
		if err != nil {
			t.Fatalf("IndexedEntries: %v", err)
		}
		wantVersions := map[[2]string]bool{}
		for _, chunk := range fixture.Chunks {
			wantVersions[[2]string{chunk.Chunk.EntryID, chunk.Chunk.EntryHash}] = true
		}
		gotVersions := map[[2]string]bool{}
		for _, ref := range refs {
			gotVersions[[2]string{ref.EntryID, ref.EntryHash}] = true
		}
		for pair := range wantVersions {
			if !gotVersions[pair] {
				t.Errorf("IndexedEntries missing (entry, version) %v", pair)
			}
		}
	}

	// Monotonic reconciliation without revision invalidation, and no
	// filter-shaped deletion: reconciling a NEW entry under an unrelated
	// revision adds it and leaves every prior chunk in place — nothing is
	// dropped merely because it was absent from this reconcile set, and the
	// revision string never invalidates stored vectors.
	dim := len(fixture.Chunks[0].Vector)
	extraVec := make([]float32, dim)
	extraVec[0] = 1
	extra := sdd.IndexedChunk{Chunk: sdd.CanonicalChunk{
		ID: "sddtest-extra#summary", EntryID: "sddtest-extra", ContentHash: "extra",
		Text: "extra text", Body: "extra body", IsSummary: true,
	}, Vector: extraVec}
	if err := fixture.Store.Reconcile(ctx, fixture.Namespace, "r2-unrelated", []sdd.IndexedChunk{extra}, nil); err != nil {
		t.Fatalf("second Reconcile: %v", err)
	}
	manifest, err = fixture.Store.Manifest(ctx, fixture.Namespace)
	if err != nil {
		t.Fatalf("Manifest after adding an entry: %v", err)
	}
	if len(manifest) != len(fixture.Chunks)+1 {
		t.Fatalf("reconcile of a new entry changed stored chunk count to %d, want %d (monotonic, no filter-shaped deletion)", len(manifest), len(fixture.Chunks)+1)
	}

	// Dimension mismatch: a query vector of the wrong length must error where
	// it meets the store, not return silently wrong hits.
	if _, err := fixture.Store.Nearest(ctx, []sdd.IndexNamespace{fixture.Namespace}, append(append([]float32(nil), fixture.Query...), 0), len(fixture.Chunks)); err == nil {
		t.Error("Nearest with a mismatched-dimension query should error")
	}

	// Reopen persistence: a store reopened over the same backing state still
	// answers with the reconciled chunks.
	reopen := fixture.Reopen
	if reopen == nil {
		reopen = func() sdd.SearchIndexStore { return fixture.Store }
	}
	reopened := reopen()
	reopenedHits, err := reopened.Nearest(ctx, []sdd.IndexNamespace{fixture.Namespace}, fixture.Query, len(fixture.Chunks))
	if err != nil {
		t.Fatalf("Nearest after reopen: %v", err)
	}
	if len(reopenedHits) == 0 {
		t.Fatal("reopened store returned no hits — reconciled chunks did not persist")
	}

	// Multi-version accumulation (per-version stores only): reconciling the
	// first entry under a NEW version — new entry hash, new chunk IDs — ADDS a
	// version rather than replacing the old one, so the store reports both. This
	// is the shared store's branch-divergence guarantee at the adapter boundary.
	// Runs last so the earlier monotonic count assertion is unaffected.
	if manifestCap, ok := fixture.Store.(sdd.SearchIndexEntryManifest); ok && len(fixture.Chunks) > 0 && fixture.Chunks[0].Chunk.EntryHash != "" {
		first := fixture.Chunks[0].Chunk
		v2vec := make([]float32, len(fixture.Chunks[0].Vector))
		v2vec[0] = 1
		v2 := sdd.IndexedChunk{Chunk: sdd.CanonicalChunk{
			ID: first.EntryID + "#v-conformancev2#summary", EntryID: first.EntryID, EntryHash: "conformance-v2",
			ContentHash: "conformance-v2", Text: first.Text, Body: first.Body, IsSummary: true,
		}, Vector: v2vec}
		if err := fixture.Store.Reconcile(ctx, fixture.Namespace, "r-newversion", []sdd.IndexedChunk{v2}, nil); err != nil {
			t.Fatalf("reconcile of a new version: %v", err)
		}
		refs, err := manifestCap.IndexedEntries(ctx, fixture.Namespace)
		if err != nil {
			t.Fatalf("IndexedEntries after new version: %v", err)
		}
		versionsOfFirst := 0
		for _, ref := range refs {
			if ref.EntryID == first.EntryID {
				versionsOfFirst++
			}
		}
		if versionsOfFirst < 2 {
			t.Errorf("entry %q reports %d versions after adding one, want >= 2 (monotonic accumulation, no delete-on-change)", first.EntryID, versionsOfFirst)
		}
	}
}

type MutationFinalizerFixture struct {
	Finalizer sdd.MutationFinalizer
	Applied   sdd.AppliedMutation
}

func RunMutationFinalizerTests(t *testing.T, factory func(*testing.T) MutationFinalizerFixture) {
	t.Helper()
	fixture := factory(t)
	if fixture.Finalizer.Name() == "" {
		t.Fatal("Name returned empty")
	}
	for range 2 {
		if err := fixture.Finalizer.Finalize(context.Background(), fixture.Applied); err != nil {
			t.Fatalf("idempotent Finalize: %v", err)
		}
	}
}
