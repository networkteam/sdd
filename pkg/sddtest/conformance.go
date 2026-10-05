package sddtest

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/pkg/application"
	"github.com/networkteam/sdd/pkg/llm/embed"
)

type AccessResolverFixture struct {
	Resolver  sdd.AccessResolver
	Identity  sdd.RequestIdentity
	Principal sdd.Principal
	// Participant is the name the principal must resolve to in Project.
	Participant  string
	Project      sdd.ProjectID
	Dependency   sdd.RepoID
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
	participant, err := fixture.Resolver.ResolveParticipant(t.Context(), principal, fixture.Project)
	if err != nil {
		t.Fatalf("ResolveParticipant: %v", err)
	}
	if participant != fixture.Participant {
		t.Fatalf("ResolveParticipant = %q, want %q", participant, fixture.Participant)
	}
	if fixture.Dependency != "" {
		if _, err := fixture.Resolver.ResolveDependency(t.Context(), principal, fixture.Project, fixture.Dependency); err != nil {
			t.Fatalf("ResolveDependency: %v", err)
		}
	}
}

// GraphStoreFixture drives the publication conformance of a graph store: the
// store must implement sdd.PublicationStore. Entry is one complete entry
// publication under EntryKey; Document is an entry-less document the suite
// creates, replaces and removes under the three DocumentKeys.
type GraphStoreFixture struct {
	Store           sdd.GraphStore
	InitialRevision string
	Entry           sdd.MutationBatch
	EntryID         string
	EntryKey        sdd.PublicationKey
	Blobs           sdd.StagedBlobReader
	AttachmentEntry string
	AttachmentName  string

	DocumentPath        string
	DocumentContent     []byte
	DocumentReplacement []byte
	DocumentKeys        [3]sdd.PublicationKey
}

// RunGraphStoreTests checks the guarantees every composition's store owes the
// engine's recorded-intent publication (d-tac-n47, d-tac-wgw): a key publishes
// once and repeats return the original, a replacement conditions on the
// document it replaces, and removing an absent document succeeds.
func RunGraphStoreTests(t *testing.T, factory func(*testing.T) GraphStoreFixture) {
	t.Helper()
	fixture := factory(t)
	ctx := t.Context()
	snapshot, err := fixture.Store.Current(ctx)
	if err != nil {
		t.Fatalf("Current: %v", err)
	}
	if snapshot == nil || snapshot.Revision() != fixture.InitialRevision {
		t.Fatalf("Current revision = %q, want %q", snapshot.Revision(), fixture.InitialRevision)
	}
	publisher, ok := fixture.Store.(sdd.PublicationStore)
	if !ok {
		t.Fatalf("store %T does not implement sdd.PublicationStore", fixture.Store)
	}

	entryPath := fixture.Entry.Changes[0].LogicalPath
	entryID := fixture.EntryID
	if _, found, err := publisher.LookupEntryPublication(ctx, fixture.EntryKey, entryID); err != nil || found {
		t.Fatalf("LookupEntryPublication before publish = found %v, %v; want absent", found, err)
	}
	published, err := publisher.PublishEntry(ctx, fixture.EntryKey, fixture.Entry, fixture.Blobs)
	if err != nil {
		t.Fatalf("PublishEntry: %v", err)
	}
	if published.Document.LogicalPath != entryPath || published.Revision == "" {
		t.Fatalf("PublishEntry = %+v, want document at %s with a revision", published, entryPath)
	}
	repeated, err := publisher.PublishEntry(ctx, fixture.EntryKey, fixture.Entry, fixture.Blobs)
	if err != nil {
		t.Fatalf("repeated PublishEntry: %v", err)
	}
	if repeated.Document.LogicalPath != published.Document.LogicalPath || repeated.Document.Body != published.Document.Body {
		t.Fatalf("repeated PublishEntry = %+v, want the original %+v", repeated.Document, published.Document)
	}
	if _, found, err := publisher.LookupEntryPublication(ctx, fixture.EntryKey, entryID); err != nil || !found {
		t.Fatalf("LookupEntryPublication after publish = found %v, %v; want found", found, err)
	}
	if fixture.AttachmentEntry != "" {
		page, err := fixture.Store.ReadAttachmentPage(ctx, fixture.AttachmentEntry, fixture.AttachmentName, 0, 1)
		if err != nil {
			t.Fatalf("ReadAttachmentPage: %v", err)
		}
		if page.Filename != fixture.AttachmentName || len(page.Content) > 1 {
			t.Fatalf("ReadAttachmentPage = %+v", page)
		}
	}

	if fixture.DocumentPath == "" {
		return
	}
	create, replace, remove := fixture.DocumentKeys[0], fixture.DocumentKeys[1], fixture.DocumentKeys[2]
	if current, err := publisher.ReadDocument(ctx, fixture.DocumentPath); err != nil || !current.Absent {
		t.Fatalf("ReadDocument before create = %+v, %v; want absent", current, err)
	}
	created, err := publisher.PublishDocument(ctx, create, sdd.DocumentMutation{LogicalPath: fixture.DocumentPath, Content: fixture.DocumentContent, Message: "create"})
	if err != nil {
		t.Fatalf("PublishDocument create: %v", err)
	}
	if created.Absent || !bytes.Equal(created.Content, fixture.DocumentContent) || created.Revision == "" {
		t.Fatalf("PublishDocument create = %+v", created)
	}
	if current, err := publisher.ReadDocument(ctx, fixture.DocumentPath); err != nil || !bytes.Equal(current.Content, fixture.DocumentContent) {
		t.Fatalf("ReadDocument after create = %+v, %v", current, err)
	}
	again, err := publisher.PublishDocument(ctx, create, sdd.DocumentMutation{LogicalPath: fixture.DocumentPath, Content: fixture.DocumentContent, Message: "create"})
	if err != nil || !bytes.Equal(again.Content, fixture.DocumentContent) {
		t.Fatalf("repeated PublishDocument create = %+v, %v; want the original", again, err)
	}
	if looked, found, err := publisher.LookupDocumentPublication(ctx, create, fixture.DocumentPath); err != nil {
		t.Fatalf("LookupDocumentPublication: %v", err)
	} else if found && !bytes.Equal(looked.Content, fixture.DocumentContent) {
		t.Fatalf("LookupDocumentPublication = %+v, want the created content", looked)
	}
	other := create
	other.Sequence++
	existing, err := publisher.PublishDocument(ctx, other, sdd.DocumentMutation{LogicalPath: fixture.DocumentPath, Content: fixture.DocumentReplacement, Message: "create over"})
	if err != nil || !bytes.Equal(existing.Content, fixture.DocumentContent) || existing.Absent {
		t.Fatalf("PublishDocument create over a present document = %+v, %v; want the present document", existing, err)
	}
	if current, err := publisher.ReadDocument(ctx, fixture.DocumentPath); err != nil || !bytes.Equal(current.Content, fixture.DocumentContent) {
		t.Fatalf("a creation over a present document must leave it: %+v, %v", current, err)
	}
	_, err = publisher.PublishDocument(ctx, replace, sdd.DocumentMutation{LogicalPath: fixture.DocumentPath, Content: fixture.DocumentReplacement, ExpectedBlob: sdd.GitBlobID([]byte("something else")), Message: "replace"})
	var appErr *sdd.ApplicationError
	if !errors.As(err, &appErr) || appErr.Code != sdd.ErrorGraphConflict {
		t.Fatalf("PublishDocument replace with a stale precondition = %v, want %s", err, sdd.ErrorGraphConflict)
	}
	if current, err := publisher.ReadDocument(ctx, fixture.DocumentPath); err != nil || !bytes.Equal(current.Content, fixture.DocumentContent) {
		t.Fatalf("a refused replacement must leave the document unchanged: %+v, %v", current, err)
	}
	replaced, err := publisher.PublishDocument(ctx, replace, sdd.DocumentMutation{LogicalPath: fixture.DocumentPath, Content: fixture.DocumentReplacement, ExpectedBlob: sdd.GitBlobID(fixture.DocumentContent), Message: "replace"})
	if err != nil || !bytes.Equal(replaced.Content, fixture.DocumentReplacement) {
		t.Fatalf("PublishDocument replace = %+v, %v", replaced, err)
	}
	if current, err := publisher.ReadDocument(ctx, fixture.DocumentPath); err != nil || !bytes.Equal(current.Content, fixture.DocumentReplacement) {
		t.Fatalf("ReadDocument after replace = %+v, %v", current, err)
	}
	removed, err := publisher.PublishDocument(ctx, remove, sdd.DocumentMutation{LogicalPath: fixture.DocumentPath, Message: "remove"})
	if err != nil || !removed.Absent || removed.Revision == "" {
		t.Fatalf("PublishDocument remove = %+v, %v; want absent", removed, err)
	}
	if current, err := publisher.ReadDocument(ctx, fixture.DocumentPath); err != nil || !current.Absent {
		t.Fatalf("ReadDocument after remove = %+v, %v; want absent", current, err)
	}
	absentKey := remove
	absentKey.Sequence++
	if again, err := publisher.PublishDocument(ctx, absentKey, sdd.DocumentMutation{LogicalPath: fixture.DocumentPath, Message: "remove again"}); err != nil || !again.Absent || again.Revision != "" {
		t.Fatalf("removing an absent document = %+v, %v; want absent with nothing to complete", again, err)
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
	if created.Version != 0 {
		t.Fatalf("Create version = %d, want zero before the first event", created.Version)
	}
	appendData := fixture.Append
	appendData.Events = slices.Clone(fixture.Append.Events)
	suppliedTime := time.Date(1900, 1, 1, 0, 0, 0, 0, time.UTC)
	for i := range appendData.Events {
		appendData.Events[i].Sequence = 999
		appendData.Events[i].CreatedAt = suppliedTime
	}
	next, err := fixture.Store.Append(t.Context(), fixture.Metadata.ID, created.Version, appendData)
	if err != nil {
		t.Fatalf("Append: %v", err)
	}
	if want := uint64(len(appendData.Events)); next != want {
		t.Fatalf("Append version = %d, want event tip %d", next, want)
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
	for i, event := range loaded.Events {
		if event.Sequence != uint64(i)+1 || event.CreatedAt.IsZero() || event.CreatedAt.Equal(suppliedTime) {
			t.Fatalf("event %d lacks store-assigned sequence/time: %+v", i, event)
		}
		var gotPayload, wantPayload any
		if err := json.Unmarshal(event.Payload, &gotPayload); err != nil {
			t.Fatalf("decode stored event %d: %v", i, err)
		}
		if err := json.Unmarshal(fixture.Append.Events[i].Payload, &wantPayload); err != nil {
			t.Fatalf("decode fixture event %d: %v", i, err)
		}
		if event.Code != fixture.Append.Events[i].Code || !reflect.DeepEqual(gotPayload, wantPayload) {
			t.Fatalf("event %d changed its code or JSON payload", i)
		}
	}
	if fixture.Append.Metadata != nil && !reflect.DeepEqual(loaded.Metadata, *fixture.Append.Metadata) {
		t.Fatal("metadata and events did not commit together")
	}
	changed := loaded.Metadata
	changed.Label = "must not persist"
	for _, tt := range []struct {
		name string
		data sdd.SessionAppend
	}{
		{name: "empty append"},
		{name: "metadata-only append", data: sdd.SessionAppend{Metadata: &changed}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := fixture.Store.Append(t.Context(), fixture.Metadata.ID, next, tt.data); err == nil {
				t.Fatal("empty append succeeded")
			}
			unchanged, err := fixture.Store.Load(t.Context(), fixture.Metadata.ID)
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(loaded, unchanged) {
				t.Fatal("rejected append changed stored events or metadata")
			}
		})
	}
	again, err := fixture.Store.Load(t.Context(), fixture.Metadata.ID)
	if err != nil || !reflect.DeepEqual(again, loaded) {
		t.Fatalf("reloading changed assigned event positions/timestamps: %v", err)
	}
	if want := fixture.Metadata.Attachment; want != nil {
		got := loaded.Metadata.Attachment
		if got == nil || got.Subject != want.Subject || got.ClientName != want.ClientName {
			t.Fatalf("attachment did not round-trip: got %+v, want %+v", got, want)
		}
	}
	listed, err := fixture.Store.List(t.Context(), sdd.SessionFilter{Project: fixture.Metadata.Project})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(listed.Sessions) == 0 {
		t.Fatal("List did not return the created session")
	}

	// Collection pages through List: ID order is the cursor, Next names where a
	// page stopped, and EndedBefore selects on the recorded ending alone.
	second, third := fixture.Metadata, fixture.Metadata
	second.ID += "-page-b"
	third.ID += "-page-c"
	for _, metadata := range []sdd.SessionMetadata{second, third} {
		if _, err := fixture.Store.Create(t.Context(), metadata); err != nil {
			t.Fatalf("Create(%s): %v", metadata.ID, err)
		}
	}
	endedAt := time.Now().UTC().Round(0)
	ended := third
	ended.Ended = &sdd.SessionEnd{Act: sdd.SessionConcluded, EndedAt: endedAt}
	if _, err := fixture.Store.Append(t.Context(), third.ID, 0, sdd.SessionAppend{Metadata: &ended, Events: []sdd.StoredEvent{{CodecVersion: 1, Code: "ended", Payload: json.RawMessage(`{}`)}}}); err != nil {
		t.Fatalf("ending %s: %v", third.ID, err)
	}
	var paged []sdd.SessionID
	for cursor := sdd.SessionID(""); ; {
		page, err := fixture.Store.List(t.Context(), sdd.SessionFilter{Project: fixture.Metadata.Project, After: cursor, Limit: 1})
		if err != nil {
			t.Fatalf("List page after %q: %v", cursor, err)
		}
		if len(page.Sessions) > 1 {
			t.Fatalf("List page holds %d sessions, want at most the Limit of 1", len(page.Sessions))
		}
		for _, item := range page.Sessions {
			paged = append(paged, item.Metadata.ID)
		}
		if page.Next == "" {
			break
		}
		if page.Next <= cursor {
			t.Fatalf("List Next %q did not advance past %q", page.Next, cursor)
		}
		cursor = page.Next
	}
	if !slices.IsSortedFunc(paged, func(a, b sdd.SessionID) int { return strings.Compare(string(a), string(b)) }) {
		t.Fatalf("List pages out of ID order: %v", paged)
	}
	for _, id := range []sdd.SessionID{fixture.Metadata.ID, second.ID, third.ID} {
		if !slices.Contains(paged, id) {
			t.Fatalf("List pages = %v, missing %s", paged, id)
		}
	}
	before := endedAt.Add(time.Second)
	endedPage, err := fixture.Store.List(t.Context(), sdd.SessionFilter{Project: fixture.Metadata.Project, EndedBefore: &before})
	if err != nil {
		t.Fatalf("List EndedBefore: %v", err)
	}
	if len(endedPage.Sessions) != 1 || endedPage.Sessions[0].Metadata.ID != third.ID {
		t.Fatalf("List EndedBefore = %+v, want only the ended session %s", endedPage.Sessions, third.ID)
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
	for _, item := range remaining.Sessions {
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
	if blob.Filename != fixture.Filename || blob.Size != int64(len(fixture.Content)) {
		t.Fatalf("Stage = %+v", blob)
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
	replacementContent := append(bytes.Clone(fixture.Content), []byte("\nUpdated staged content.")...)
	replacement, err := fixture.Store.Stage(t.Context(), fixture.Session, fixture.Filename, bytes.NewReader(replacementContent))
	if err != nil {
		t.Fatalf("Stage the same filename again: %v", err)
	}
	if replacement.ID == blob.ID {
		t.Fatal("restaging reused an immutable blob ID")
	}
	for _, staged := range []struct {
		id      string
		content []byte
	}{
		{blob.ID, fixture.Content},
		{replacement.ID, replacementContent},
	} {
		reader, err := fixture.Store.Open(t.Context(), fixture.Session, staged.id)
		if err != nil {
			t.Fatalf("Open after restaging: %v", err)
		}
		got, readErr := io.ReadAll(reader)
		closeErr := reader.Close()
		if readErr != nil || closeErr != nil || !bytes.Equal(got, staged.content) {
			t.Fatalf("blob %s changed after restaging: content=%q read=%v close=%v", staged.id, got, readErr, closeErr)
		}
	}

	other := fixture.Session
	other.Session += "-other"
	if reader, err := fixture.Store.Open(t.Context(), other, blob.ID); err == nil {
		_ = reader.Close()
		t.Fatal("Open accepted a blob from another session")
	}
	otherBlob, err := fixture.Store.Stage(t.Context(), other, fixture.Filename, bytes.NewReader(fixture.Content))
	if err != nil {
		t.Fatalf("Stage other session: %v", err)
	}
	if err := fixture.Store.DeleteStaged(t.Context(), fixture.Session); err != nil {
		t.Fatalf("DeleteStaged: %v", err)
	}
	if reader, err := fixture.Store.Open(t.Context(), fixture.Session, blob.ID); err == nil {
		_ = reader.Close()
		t.Fatal("Open after DeleteStaged succeeded")
	}
	if err := fixture.Store.DeleteStaged(t.Context(), fixture.Session); err != nil {
		t.Fatalf("second DeleteStaged: %v", err)
	}
	reader, err = fixture.Store.Open(t.Context(), other, otherBlob.ID)
	if err != nil {
		t.Fatalf("Open other session after DeleteStaged: %v", err)
	}
	got, readErr = io.ReadAll(reader)
	closeErr = reader.Close()
	if readErr != nil || closeErr != nil || !bytes.Equal(got, fixture.Content) {
		t.Fatalf("other session content = %q, read %v, close %v", got, readErr, closeErr)
	}
}

type EmbedderFixture struct {
	Embedder embed.Embedder
	Texts    []string
}

// RunEmbedderTests checks the embed.Embedder contract: a stable non-empty
// fingerprint, one non-empty vector per text in order, all of equal length.
func RunEmbedderTests(t *testing.T, factory func(*testing.T) EmbedderFixture) {
	t.Helper()
	fixture := factory(t)
	fingerprint := fixture.Embedder.Fingerprint()
	if fingerprint == "" {
		t.Fatal("Fingerprint is empty")
	}
	result, err := fixture.Embedder.Embed(t.Context(), embed.Request{Purpose: embed.PurposeDocument, Texts: fixture.Texts})
	if err != nil {
		t.Fatalf("Embed: %v", err)
	}
	if len(result.Vectors) != len(fixture.Texts) {
		t.Fatalf("Embed returned %d vectors, want %d", len(result.Vectors), len(fixture.Texts))
	}
	dims := 0
	for i, vector := range result.Vectors {
		if len(vector) == 0 {
			t.Fatalf("vector %d is empty", i)
		}
		if dims == 0 {
			dims = len(vector)
		}
		if len(vector) != dims {
			t.Fatalf("vector %d has %d dimensions, want %d", i, len(vector), dims)
		}
	}
	if fixture.Embedder.Fingerprint() != fingerprint {
		t.Fatal("Fingerprint changed across a call")
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
