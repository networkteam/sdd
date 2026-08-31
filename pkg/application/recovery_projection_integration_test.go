package application_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
	pkgllm "github.com/networkteam/sdd/pkg/llm"
	localadapter "github.com/networkteam/sdd/pkg/local"
)

// recoveryFixtureProject is the project the real fixture sessions were recorded
// against; the projection filters sessions by project, so it must match.
const recoveryFixtureProject sdd.ProjectID = "github.com/networkteam/sdd"

// copyRecoveryFixtureSessions stages the trimmed real session logs in a
// temporary store directory. The store writes lock files beside each session,
// so it must never be pointed at testdata itself.
func copyRecoveryFixtureSessions(t *testing.T, variants ...string) string {
	t.Helper()
	dir := t.TempDir()
	copied := 0
	for _, variant := range variants {
		source := filepath.Join("testdata", "recovery", variant)
		entries, err := os.ReadDir(source)
		if err != nil {
			t.Fatal(err)
		}
		for _, entry := range entries {
			if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".jsonl") {
				continue
			}
			content, err := os.ReadFile(filepath.Join(source, entry.Name()))
			if err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(filepath.Join(dir, entry.Name()), content, 0o600); err != nil {
				t.Fatal(err)
			}
			copied++
		}
	}
	if copied == 0 {
		t.Fatalf("no fixture sessions found for variants %v", variants)
	}
	return dir
}

func newRecoveryFixtureApplication(t *testing.T, sessionsDir string) *sdd.Application {
	t.Helper()
	graph, err := localadapter.NewFilesystemGraphStore(localadapter.FilesystemGraphStoreOptions{
		Project: recoveryFixtureProject, GraphDir: t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	sessions, err := localadapter.NewFilesystemSessionStoreAt(sessionsDir)
	if err != nil {
		t.Fatal(err)
	}
	blobs, err := localadapter.NewFilesystemStagedBlobStoreAt(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	runtime, err := sdd.NewProjectRuntime(sdd.ProjectRuntimeOptions{
		Project:     sdd.ProjectRef{ID: recoveryFixtureProject, DisplayName: "SDD"},
		Graph:       graph,
		Sessions:    sessions,
		StagedBlobs: blobs,
		LLM: pkgllm.RunnerFunc(func(context.Context, pkgllm.Request) (pkgllm.Result, error) {
			return pkgllm.Result{Identity: pkgllm.Identity{Provider: "test", Model: "test"}}, nil
		}),
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

// TestListRecoveriesReportsNoPendingWritesForRealAppliedLegacyStore drives the
// projection over a session store built from three real session logs, each
// carrying one real v1 intent that the store records as applied with the git
// finalizer succeeded. Nothing in that store is a pending write, so the free
// read projection must surface no actionable items and Info must print no
// recovery notice.
func TestListRecoveriesReportsNoPendingWritesForRealAppliedLegacyStore(t *testing.T) {
	sessionsDir := copyRecoveryFixtureSessions(t, "sessions")
	application := newRecoveryFixtureApplication(t, sessionsDir)
	identity := sdd.RequestIdentity{Subject: "local", Scopes: []string{"project:read"}}

	// Guard: the closed projection must actually see the fixture intents, so
	// that an empty actionable list cannot pass vacuously on an empty store.
	all, err := application.ListRecoveries(t.Context(), identity, recoveryFixtureProject, true)
	if err != nil {
		t.Fatal(err)
	}
	if len(all.Items) != 3 {
		t.Fatalf("ListRecoveries(includeClosed=true) returned %d items, want the 3 fixture intents: %+v", len(all.Items), all.Items)
	}

	open, err := application.ListRecoveries(t.Context(), identity, recoveryFixtureProject, false)
	if err != nil {
		t.Fatal(err)
	}
	if len(open.Items) != 0 {
		var reported []string
		for _, item := range open.Items {
			reported = append(reported, string(item.Session)+"/"+item.MutationID+" state="+string(item.State)+" legacyUnroutable="+boolText(item.LegacyUnroutable))
		}
		t.Errorf("ListRecoveries(includeClosed=false) returned %d actionable items, want 0; every fixture intent is recorded as applied with the git finalizer succeeded:\n  %s",
			len(open.Items), strings.Join(reported, "\n  "))
	}

	info, err := application.Info(t.Context(), identity, recoveryFixtureProject, sdd.InfoRequest{})
	if err != nil {
		t.Fatal(err)
	}
	if info.Recovery != "" {
		t.Errorf("Info().Recovery = %q, want empty: no pending write awaits recovery in this store", info.Recovery)
	}
}

func boolText(value bool) string {
	if value {
		return "true"
	}
	return "false"
}
