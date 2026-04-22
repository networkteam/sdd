package handlers_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/bundledskills"
	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/handlers"
	"github.com/networkteam/sdd/internal/model"
)

// TestInit_FreshProjectEndToEnd exercises the full Init orchestration on an
// empty directory: .sdd/ tree creation, config + meta files, embedded skill
// extraction with stamps, and the expected callback fanout.
func TestInit_FreshProjectEndToEnd(t *testing.T) {
	tmp := t.TempDir()

	h := handlers.New(handlers.Options{Reader: finders.New(nil)})

	var (
		createdCalled     bool
		metaWrittenCalled bool
		skills            command.SkillInstallResult
	)

	err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Target:        model.AgentClaude,
		Scope:         model.ScopeProject,
		OnCreated: func(sddDir, graphDir string) {
			createdCalled = true
		},
		OnMetaWritten:     func(string) { metaWrittenCalled = true },
		OnSkillsInstalled: func(r command.SkillInstallResult) { skills = r },
	})
	if err != nil {
		t.Fatal(err)
	}
	if !createdCalled {
		t.Error("OnCreated did not fire on fresh init")
	}
	if !metaWrittenCalled {
		t.Error("OnMetaWritten did not fire on fresh init")
	}

	// meta.json content — semver binary version should yield minimum_version.
	metaPath := filepath.Join(tmp, model.SDDDirName, model.SchemaMetaFileName)
	data, err := os.ReadFile(metaPath)
	if err != nil {
		t.Fatalf("read meta.json: %v", err)
	}
	meta, err := model.ParseSchemaMeta(data)
	if err != nil {
		t.Fatal(err)
	}
	if meta.GraphSchemaVersion != model.CurrentGraphSchemaVersion {
		t.Errorf("GraphSchemaVersion: got %d, want %d", meta.GraphSchemaVersion, model.CurrentGraphSchemaVersion)
	}
	if meta.MinimumVersion == nil || *meta.MinimumVersion != "v0.2.0" {
		t.Errorf("MinimumVersion: got %+v, want v0.2.0", meta.MinimumVersion)
	}

	if len(skills.Installed) == 0 {
		t.Error("no skills reported as installed")
	}
	if len(skills.Refreshed)+len(skills.Overwritten)+len(skills.SkippedModified)+len(skills.Current) != 0 {
		t.Errorf("unexpected non-installed categories on fresh init: %+v", skills)
	}

	// .gitignore should contain both the tmp directory and the local config
	// file so API keys stored locally don't get committed.
	gitignore := filepath.Join(tmp, ".gitignore")
	data, err = os.ReadFile(gitignore)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	for _, want := range []string{".sdd/tmp/", ".sdd/config.local.yaml"} {
		if !strings.Contains(string(data), want) {
			t.Errorf(".gitignore missing %q, got:\n%s", want, data)
		}
	}
}

// TestInit_GitignoreIdempotent verifies re-running init against an
// already-configured .gitignore does not duplicate entries. Regression guard
// for the housekeeping pass that now runs on every init (not just fresh).
func TestInit_GitignoreIdempotent(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(nil)})

	run := func() {
		if err := h.Init(context.Background(), &command.InitCmd{
			RepoRoot:      tmp,
			BinaryVersion: "v0.2.0",
			Scope:         model.ScopeProject,
		}); err != nil {
			t.Fatal(err)
		}
	}

	run()
	run()

	data, err := os.ReadFile(filepath.Join(tmp, ".gitignore"))
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	for _, entry := range []string{".sdd/tmp/", ".sdd/config.local.yaml"} {
		count := strings.Count(string(data), entry)
		if count != 1 {
			t.Errorf("%q appears %d times in .gitignore; want exactly 1\n%s", entry, count, data)
		}
	}
}

// TestInit_DevBuildSkipsMinimumVersion verifies that a non-semver binary
// version leaves minimum_version absent from meta.json on initial write.
func TestInit_DevBuildSkipsMinimumVersion(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(nil)})

	err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "dev",
		Scope:         model.ScopeProject,
	})
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(tmp, model.SDDDirName, model.SchemaMetaFileName))
	if err != nil {
		t.Fatal(err)
	}
	meta, err := model.ParseSchemaMeta(data)
	if err != nil {
		t.Fatal(err)
	}
	if meta.GraphSchemaVersion != model.CurrentGraphSchemaVersion {
		t.Errorf("GraphSchemaVersion: got %d, want %d", meta.GraphSchemaVersion, model.CurrentGraphSchemaVersion)
	}
	if meta.MinimumVersion != nil {
		t.Errorf("dev build must not stamp minimum_version, got %q", *meta.MinimumVersion)
	}
}

// TestInit_RepeatIsIdempotent verifies that a second Init on a fully
// populated tree fires no write callbacks and classifies every skill as
// Current.
func TestInit_RepeatIsIdempotent(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(nil)})

	// First run: populate.
	err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Scope:         model.ScopeProject,
	})
	if err != nil {
		t.Fatal(err)
	}

	// Second run: should be a no-op.
	var (
		createdFired     bool
		metaWrittenFired bool
		metaPreserved    bool
		skills           command.SkillInstallResult
	)
	err = h.Init(context.Background(), &command.InitCmd{
		RepoRoot:          tmp,
		BinaryVersion:     "v0.2.0",
		Scope:             model.ScopeProject,
		OnCreated:         func(_, _ string) { createdFired = true },
		OnMetaWritten:     func(string) { metaWrittenFired = true },
		OnMetaPreserved:   func(string) { metaPreserved = true },
		OnSkillsInstalled: func(r command.SkillInstallResult) { skills = r },
	})
	if err != nil {
		t.Fatal(err)
	}
	if createdFired {
		t.Error("OnCreated should not fire on repeat init")
	}
	if metaWrittenFired {
		t.Error("OnMetaWritten should not fire on repeat init")
	}
	if !metaPreserved {
		t.Error("OnMetaPreserved should fire when meta.json already exists")
	}
	changed := len(skills.Installed) + len(skills.Refreshed) + len(skills.Overwritten) + len(skills.SkippedModified)
	if changed != 0 {
		t.Errorf("repeat init produced writes: %+v", skills)
	}
	if len(skills.Current) == 0 {
		t.Error("repeat init should classify files as Current")
	}
}

// TestInit_PostUpgradeRefreshesDriftedPristine simulates a bundle content
// change across binary versions: an installed file carries a stored hash
// that matches its own content (user hasn't edited) but differs from the
// current embedded bundle. Init should refresh it silently.
func TestInit_PostUpgradeRefreshesDriftedPristine(t *testing.T) {
	tmp := t.TempDir()

	// Pick any bundle entry and install a substitute file at its target
	// path, stamped as if from a prior bundle version (different body).
	bundle, err := bundledskills.Load(model.AgentClaude)
	if err != nil {
		t.Fatal(err)
	}
	entry := bundle.Entries[0]

	oldContent := []byte("---\nname: " + entry.Skill + "\n---\nprior bundle body\n")
	oldHash := model.ComputeSkillHash(oldContent)
	oldFile, err := model.RenderSkillFile(model.SkillBundleEntry{Content: oldContent}, "v0.1.0", oldHash)
	if err != nil {
		t.Fatal(err)
	}
	abs := filepath.Join(tmp, ".claude", "skills", entry.Skill, entry.RelPath)
	if err := os.MkdirAll(filepath.Dir(abs), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(abs, oldFile, 0o644); err != nil {
		t.Fatal(err)
	}

	h := handlers.New(handlers.Options{Reader: finders.New(nil)})
	var skills command.SkillInstallResult
	err = h.Init(context.Background(), &command.InitCmd{
		RepoRoot:          tmp,
		BinaryVersion:     "v0.2.0",
		Scope:             model.ScopeProject,
		OnSkillsInstalled: func(r command.SkillInstallResult) { skills = r },
	})
	if err != nil {
		t.Fatal(err)
	}

	if !slices.Contains(skills.Refreshed, abs) {
		t.Errorf("expected %s in Refreshed, got: %+v", abs, skills.Refreshed)
	}

	// Confirm the stamp updated to the new binary version.
	data, err := os.ReadFile(abs)
	if err != nil {
		t.Fatal(err)
	}
	sf := model.ParseSkillFile(abs, data)
	if sf.StoredVersion != "v0.2.0" {
		t.Errorf("StoredVersion: got %q, want v0.2.0", sf.StoredVersion)
	}
}

// TestInit_WritesParticipantAndPreservesLLMBlock exercises the participant-write
// path: an existing config.local.yaml already carries an `llm:` block (from
// d-tac-bes), and init must add the participant key without touching the
// llm block or its nested keys.
func TestInit_WritesParticipantAndPreservesLLMBlock(t *testing.T) {
	tmp := t.TempDir()
	// Pre-populate .sdd/config.local.yaml with an llm block (simulating a
	// repo that already has provider config set before participant rollout).
	sddDir := filepath.Join(tmp, model.SDDDirName)
	if err := os.MkdirAll(sddDir, 0o755); err != nil {
		t.Fatal(err)
	}
	seed := []byte("llm:\n  provider: anthropic\n  model: claude-haiku-4-5-20251001\n  api_keys:\n    anthropic: sk-ant-xxx\n")
	configLocal := filepath.Join(sddDir, "config.local.yaml")
	if err := os.WriteFile(configLocal, seed, 0o644); err != nil {
		t.Fatal(err)
	}

	h := handlers.New(handlers.Options{Reader: finders.New(nil)})

	var gotPath, gotName string
	err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Scope:         model.ScopeProject,
		Participant:   "Christopher",
		OnParticipantWritten: func(path, name string) {
			gotPath = path
			gotName = name
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if gotName != "Christopher" || gotPath == "" {
		t.Errorf("OnParticipantWritten not fired correctly: path=%q name=%q", gotPath, gotName)
	}

	data, err := os.ReadFile(configLocal)
	if err != nil {
		t.Fatal(err)
	}
	content := string(data)
	for _, want := range []string{
		"participant: Christopher",
		"provider: anthropic",
		"model: claude-haiku-4-5-20251001",
		"sk-ant-xxx",
	} {
		if !strings.Contains(content, want) {
			t.Errorf("config.local.yaml missing %q after init:\n%s", want, content)
		}
	}
}

// TestInit_ParticipantIdempotent ensures a re-init with the same participant
// produces no write and no OnParticipantWritten callback.
func TestInit_ParticipantIdempotent(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(nil)})

	// First run: write participant.
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Scope:         model.ScopeProject,
		Participant:   "Christopher",
	}); err != nil {
		t.Fatal(err)
	}

	// Second run with same participant: callback must not fire.
	var fired bool
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:             tmp,
		BinaryVersion:        "v0.2.0",
		Scope:                model.ScopeProject,
		Participant:          "Christopher",
		OnParticipantWritten: func(string, string) { fired = true },
	}); err != nil {
		t.Fatal(err)
	}
	if fired {
		t.Error("OnParticipantWritten fired on idempotent re-init with same name")
	}
}

// TestInit_ParticipantEmptyDoesNotTouchConfig verifies the default path —
// when no participant is supplied and the config file already exists, init
// must not create or modify it.
func TestInit_ParticipantEmptyDoesNotTouchConfig(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(nil)})

	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Scope:         model.ScopeProject,
	}); err != nil {
		t.Fatal(err)
	}

	configLocal := filepath.Join(tmp, model.SDDDirName, "config.local.yaml")
	if _, err := os.Stat(configLocal); !os.IsNotExist(err) {
		t.Errorf("config.local.yaml should not exist when Participant is empty, got err=%v", err)
	}
}

// TestInit_PreservesExistingMeta ensures minimum_version stamped at initial
// creation survives a later init from a different binary version.
func TestInit_PreservesExistingMeta(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(nil)})

	// Initial: stamp minimum_version with v0.2.0.
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot: tmp, BinaryVersion: "v0.2.0", Scope: model.ScopeProject,
	}); err != nil {
		t.Fatal(err)
	}

	// Repeat init from a hypothetical newer binary. minimum_version must
	// not advance automatically — that's a deliberate-maintainer operation.
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot: tmp, BinaryVersion: "v0.9.0", Scope: model.ScopeProject,
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, model.SDDDirName, model.SchemaMetaFileName))
	if err != nil {
		t.Fatal(err)
	}
	meta, err := model.ParseSchemaMeta(data)
	if err != nil {
		t.Fatal(err)
	}
	if meta.MinimumVersion == nil || *meta.MinimumVersion != "v0.2.0" {
		t.Errorf("minimum_version must be preserved as v0.2.0, got %+v", meta.MinimumVersion)
	}
}
