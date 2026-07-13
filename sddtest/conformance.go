package sddtest

import (
	"bytes"
	"context"
	"io"
	"slices"
	"testing"

	"github.com/networkteam/sdd"
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
	listed, err := fixture.Store.List(t.Context(), sdd.SessionFilter{Project: fixture.Metadata.Project})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed) == 0 {
		t.Fatal("List did not return the created session")
	}
}

type StagedBlobStoreFixture struct {
	Store    sdd.StagedBlobStore
	Owner    sdd.BlobOwner
	Filename string
	Content  []byte
}

func RunStagedBlobStoreTests(t *testing.T, factory func(*testing.T) StagedBlobStoreFixture) {
	t.Helper()
	fixture := factory(t)
	blob, err := fixture.Store.Stage(t.Context(), fixture.Owner, fixture.Filename, bytes.NewReader(fixture.Content))
	if err != nil {
		t.Fatalf("Stage: %v", err)
	}
	if blob.Owner != fixture.Owner || blob.Filename != fixture.Filename || blob.Size != int64(len(fixture.Content)) {
		t.Fatalf("Stage = %+v", blob)
	}
	stat, err := fixture.Store.Stat(t.Context(), fixture.Owner, blob.ID)
	if err != nil || stat != blob {
		t.Fatalf("Stat = %+v, %v; want %+v", stat, err, blob)
	}
	reader, err := fixture.Store.Open(t.Context(), fixture.Owner, blob.ID)
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	got, readErr := io.ReadAll(reader)
	closeErr := reader.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(got, fixture.Content) {
		t.Fatalf("Open content = %q, read %v, close %v", got, readErr, closeErr)
	}
	if err := fixture.Store.Retain(t.Context(), fixture.Owner, "mutation-1", []string{blob.ID}); err != nil {
		t.Fatalf("Retain: %v", err)
	}
	if err := fixture.Store.Release(t.Context(), fixture.Owner, "mutation-1"); err != nil {
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
	if spec.Fingerprint == "" || spec.Dimensions <= 0 {
		t.Fatalf("Spec = %+v", spec)
	}
	vectors, err := fixture.Executor.Embed(t.Context(), fixture.Inputs)
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(vectors) != len(fixture.Inputs) {
		t.Fatalf("Embed returned %d vectors, want %d", len(vectors), len(fixture.Inputs))
	}
	for i, vector := range vectors {
		if vector.ID != fixture.Inputs[i].ID || len(vector.Values) != spec.Dimensions {
			t.Fatalf("vector %d = %+v", i, vector)
		}
	}
}

type SearchIndexStoreFixture struct {
	Store     sdd.SearchIndexStore
	Namespace sdd.IndexNamespace
	Revision  string
	Chunks    []sdd.IndexedChunk
	Query     []float32
}

func RunSearchIndexStoreTests(t *testing.T, factory func(*testing.T) SearchIndexStoreFixture) {
	t.Helper()
	fixture := factory(t)
	if err := fixture.Store.Reconcile(t.Context(), fixture.Namespace, fixture.Revision, fixture.Chunks, nil); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	manifest, err := fixture.Store.Manifest(t.Context(), fixture.Namespace)
	if err != nil {
		t.Fatalf("Manifest: %v", err)
	}
	if len(manifest) != len(fixture.Chunks) {
		t.Fatalf("Manifest returned %d chunks, want %d", len(manifest), len(fixture.Chunks))
	}
	hits, err := fixture.Store.Nearest(t.Context(), []sdd.IndexNamespace{fixture.Namespace}, fixture.Query, len(fixture.Chunks))
	if err != nil {
		t.Fatalf("Nearest: %v", err)
	}
	for _, hit := range hits {
		if hit.Namespace != fixture.Namespace {
			t.Fatalf("Nearest returned unauthorized namespace %+v", hit.Namespace)
		}
	}
}

type LLMExecutorFixture struct {
	Executor           sdd.LLMExecutor
	Request            sdd.LLMRequest
	RequiredCapability string
}

func RunLLMExecutorTests(t *testing.T, factory func(*testing.T) LLMExecutorFixture) {
	t.Helper()
	fixture := factory(t)
	capabilities, err := fixture.Executor.Capabilities(t.Context())
	if err != nil {
		t.Fatalf("Capabilities: %v", err)
	}
	if fixture.RequiredCapability != "" && !slices.Contains(capabilities, fixture.RequiredCapability) {
		t.Fatalf("Capabilities %v do not contain %q", capabilities, fixture.RequiredCapability)
	}
	result, err := fixture.Executor.Execute(t.Context(), fixture.Request)
	if err != nil {
		t.Fatalf("Execute: %v", err)
	}
	if result.ExecutorFingerprint == "" {
		t.Fatal("Execute returned an empty executor fingerprint")
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
