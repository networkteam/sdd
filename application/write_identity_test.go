package application_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	sdd "github.com/networkteam/sdd/application"
	"github.com/networkteam/sdd/internal/model"
	localadapter "github.com/networkteam/sdd/local"
)

// End-to-end write-path coverage for the slice-1 identity kinds: an actor and a
// role flow through CreateEntry with their kind-specific fields carried onto the
// entry, persist, and pass model validation; a role missing its required actor
// field surfaces the actionable *ValidationError (§7, s-prc-g0j) instead of a
// silent write.

func newIdentityWriteApp(t *testing.T) (*sdd.Application, sdd.RequestIdentity, sdd.SessionBinding, string) {
	t.Helper()
	dir := t.TempDir()
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{Project: "example", GraphDir: dir})
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
			ExecuteFunc: func(_ context.Context, request sdd.LLMRequest) (sdd.LLMResult, error) {
				if request.Purpose == "preflight" {
					return sdd.LLMResult{Output: []byte(`{"findings":[]}`)}, nil
				}
				return sdd.LLMResult{Output: []byte("An identity captured for the project record.")}, nil
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
	identity := sdd.RequestIdentity{Subject: "christopher"}
	return application, identity, openBinding(t, sessions, identity.Subject, "identity-write"), dir
}

func loadEntryByID(t *testing.T, dir, id string) *model.Entry {
	t.Helper()
	rel, err := model.IDToRelPath(id)
	if err != nil {
		t.Fatalf("IDToRelPath(%s): %v", id, err)
	}
	path := filepath.Join(dir, filepath.FromSlash(rel))
	var data []byte
	for attempt := 0; attempt < 100; attempt++ {
		data, err = os.ReadFile(path)
		if err == nil {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if err != nil {
		t.Fatalf("reading persisted entry %s at %s: %v", id, path, err)
	}
	// ParseEntry derives type/layer/time from the filename, so hand it the full
	// ID (the on-disk basename is DD-HHmmss-… under YYYY/MM).
	e, err := model.ParseEntry(id+".md", string(data))
	if err != nil {
		t.Fatalf("parsing persisted entry %s: %v", id, err)
	}
	return e
}

func TestCreateEntry_ActorPersistsWithCanonicalAndAliases(t *testing.T) {
	app, identity, binding, dir := newIdentityWriteApp(t)
	created, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "actor", Layer: "process", Confidence: "high",
		Body:      "Christopher is CEO of networkteam with a full-stack background.",
		Canonical: "Christopher", Aliases: []string{"Chris", "CH"},
	})
	if err != nil || created.EntryID == "" {
		t.Fatalf("actor CreateEntry = %+v, err %v", created, err)
	}
	e := loadEntryByID(t, dir, created.EntryID)
	if !e.IsActor() || e.Canonical != "Christopher" {
		t.Fatalf("persisted entry is not an actor with canonical Christopher: kind=%s canonical=%q", e.Kind, e.Canonical)
	}
	if len(e.Aliases) != 2 || e.Aliases[0] != "Chris" || e.Aliases[1] != "CH" {
		t.Errorf("aliases = %v, want [Chris CH]", e.Aliases)
	}
}

func TestCreateEntry_RolePassesValidationWithBoundActor(t *testing.T) {
	// A role's actor field is model-required (validateRoleFrontmatter rejects an
	// empty one — see the ValidationError test below), so a successful write with
	// Actor set proves the role-linkage wiring carried it onto the entry: had the
	// draft-to-entry wiring dropped it, validation would fail the same way. The
	// resolve-or-block check on the bound actor is the engine assemble gate,
	// covered in the engine tests; here the actor need not pre-exist.
	app, identity, binding, _ := newIdentityWriteApp(t)
	role, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "role", Layer: "process", Confidence: "high",
		Body: "Christopher holds the strategic and conceptual calls on this project.", Actor: "Christopher",
	})
	if err != nil || role.EntryID == "" {
		t.Fatalf("role CreateEntry with a bound actor should pass validation and write, got %+v, err %v", role, err)
	}
}

func TestCreateEntry_RoleMissingActorReturnsActionableValidationError(t *testing.T) {
	app, identity, binding, _ := newIdentityWriteApp(t)
	_, err := app.CreateEntry(t.Context(), identity, "example", binding, sdd.EntryDraft{
		Kind: "role", Layer: "process", Confidence: "high",
		Body: "A role with no bound actor.",
	})
	var verr *sdd.ValidationError
	if !errors.As(err, &verr) {
		t.Fatalf("expected *ValidationError, got %v", err)
	}
	named := false
	for _, w := range verr.Warnings {
		if w.Field == "actor" {
			named = true
		}
	}
	if !named {
		t.Fatalf("ValidationError should name the missing actor field, got %+v", verr.Warnings)
	}
}
