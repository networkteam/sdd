package handlers_test

import (
	"context"
	"fmt"
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
	"github.com/networkteam/sdd/internal/query"
)

// pruneFailReader wraps a real Reader but errors SkillStatus for one target,
// simulating a mid-prune failure so tests can assert the prune ordering.
type pruneFailReader struct {
	handlers.Reader
	failFor model.AgentTarget
}

func (r pruneFailReader) SkillStatus(ctx context.Context, q query.SkillStatusQuery) (*query.SkillStatusResult, error) {
	if q.Target == r.failFor {
		return nil, fmt.Errorf("injected SkillStatus failure for %s", q.Target)
	}
	return r.Reader.SkillStatus(ctx, q)
}

// TestInit_FreshProjectEndToEnd exercises the full Init orchestration on an
// empty directory: .sdd/ tree creation, config + meta files, embedded skill
// extraction with stamps, and the expected callback fanout.
func TestInit_FreshProjectEndToEnd(t *testing.T) {
	tmp := t.TempDir()

	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

	var (
		createdCalled     bool
		metaWrittenCalled bool
		skills            command.SkillInstallResult
	)

	err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Targets:       []model.AgentTarget{model.AgentClaude},
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

	// .gitignore should contain the tmp directory, local config file (so API
	// keys stored locally don't get committed), and the LLM/embedding stats
	// sink (machine-local per-call metrics). The search index lives in the
	// machine-global store, never in the tree — no entry for it.
	gitignore := filepath.Join(tmp, ".gitignore")
	data, err = os.ReadFile(gitignore)
	if err != nil {
		t.Fatalf("read .gitignore: %v", err)
	}
	for _, want := range []string{".sdd/tmp/", ".sdd/config.local.yaml", ".sdd/stats/"} {
		if !strings.Contains(string(data), want) {
			t.Errorf(".gitignore missing %q, got:\n%s", want, data)
		}
	}
}

// TestInit_RendersMultipleAgents verifies a fresh init with several supported
// agents renders each into its own scope directory, records the selection in
// config.yaml, and gives each agent its own profile deviations.
func TestInit_RendersMultipleAgents(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Targets:       []model.AgentTarget{model.AgentClaude, model.AgentCodex},
		Scope:         model.ScopeProject,
	}); err != nil {
		t.Fatal(err)
	}

	for _, rel := range []string{".claude/skills/sdd/SKILL.md", ".agents/skills/sdd/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(tmp, rel)); err != nil {
			t.Errorf("expected rendered skill at %s: %v", rel, err)
		}
	}

	cfgData, err := os.ReadFile(filepath.Join(tmp, model.SDDDirName, "config.yaml"))
	if err != nil {
		t.Fatalf("read config.yaml: %v", err)
	}
	if !strings.Contains(string(cfgData), "supported_agents: [claude, codex]") {
		t.Errorf("config.yaml missing supported_agents selection:\n%s", cfgData)
	}

	// The Codex render must carry its own profile's deviations from the
	// catch-up conditional and the inject helper — not Claude's.
	codexSkill, err := os.ReadFile(filepath.Join(tmp, ".agents/skills/sdd/SKILL.md"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(codexSkill), "Then invoke the `/sdd-catchup` sub-skill via the Skill tool") {
		t.Error("Codex render leaked the Claude branch of the catch-up conditional")
	}
	if !strings.Contains(string(codexSkill), "Then run the `sdd-catchup` skill") {
		t.Error("Codex render missing the else branch of the catch-up conditional")
	}
	if !strings.Contains(string(codexSkill), "Run `sdd info`") {
		t.Error("Codex render missing the instructed-injection form")
	}
}

// TestInit_RegistersMCPServers verifies a fresh project-scope init writes the
// per-agent MCP registration: a .mcp.json entry with alwaysLoad and an
// mcp__sdd__* allow rule for Claude, and a .codex/config.toml forwarding
// SSH_AUTH_SOCK for Codex — and that a repeat init leaves them byte-identical.
func TestInit_RegistersMCPServers(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

	read := func(rel string) string {
		t.Helper()
		data, err := os.ReadFile(filepath.Join(tmp, rel))
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		return string(data)
	}

	var registered []string
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Targets:       []model.AgentTarget{model.AgentClaude, model.AgentCodex},
		Scope:         model.ScopeProject,
		OnMCPRegistered: func(target model.AgentTarget, _ string) {
			registered = append(registered, string(target))
		},
	}); err != nil {
		t.Fatal(err)
	}

	if mcp := read(".mcp.json"); !strings.Contains(mcp, `"sdd"`) || !strings.Contains(mcp, `"alwaysLoad": true`) {
		t.Errorf(".mcp.json missing sdd server or alwaysLoad:\n%s", mcp)
	}
	if settings := read(".claude/settings.json"); !strings.Contains(settings, "mcp__sdd__*") {
		t.Errorf("settings.json missing the sdd allow glob:\n%s", settings)
	}
	if codex := read(".codex/config.toml"); !strings.Contains(codex, "[mcp_servers.sdd]") || !strings.Contains(codex, `env_vars = ["SSH_AUTH_SOCK"]`) {
		t.Errorf("config.toml missing sdd table or SSH_AUTH_SOCK forwarding:\n%s", codex)
	}
	if !slices.Contains(registered, "claude") || !slices.Contains(registered, "codex") {
		t.Errorf("OnMCPRegistered did not fire for both agents, got %v", registered)
	}

	// Idempotent: a repeat init (agents read from config) rewrites nothing.
	before := map[string]string{".mcp.json": read(".mcp.json"), ".claude/settings.json": read(".claude/settings.json"), ".codex/config.toml": read(".codex/config.toml")}
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Scope:         model.ScopeProject,
	}); err != nil {
		t.Fatal(err)
	}
	for rel, want := range before {
		if got := read(rel); got != want {
			t.Errorf("%s changed on idempotent re-init:\n%s", rel, got)
		}
	}
}

// TestInit_UserScopeSkipsMCPRegistration verifies MCP registration is
// project-scope only for now: a user-scope init installs skills but writes no
// registration files into the repo tree.
func TestInit_UserScopeSkipsMCPRegistration(t *testing.T) {
	repo := t.TempDir()
	home := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      repo,
		UserHome:      home,
		BinaryVersion: "v0.2.0",
		Targets:       []model.AgentTarget{model.AgentClaude, model.AgentCodex},
		Scope:         model.ScopeUser,
	}); err != nil {
		t.Fatal(err)
	}
	for _, rel := range []string{".mcp.json", ".codex/config.toml"} {
		if _, err := os.Stat(filepath.Join(repo, rel)); !os.IsNotExist(err) {
			t.Errorf("user scope should not write %s (stat err=%v)", rel, err)
		}
	}
}

// TestInit_ScaffoldsInstructionBridge covers the AGENTS.md / CLAUDE.md bridge:
// it is created when a non-Claude agent is selected and the files are absent,
// CLAUDE.md imports AGENTS.md, and an existing CLAUDE.md is never overwritten.
func TestInit_ScaffoldsInstructionBridge(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

	// A project file that must survive untouched.
	claudePath := filepath.Join(tmp, "CLAUDE.md")
	if err := os.WriteFile(claudePath, []byte("# existing project rules\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	var scaffolded []string
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Targets:       []model.AgentTarget{model.AgentClaude, model.AgentCodex},
		Scope:         model.ScopeProject,
		OnBridgeScaffolded: func(paths []string) {
			scaffolded = append(scaffolded, paths...)
		},
	}); err != nil {
		t.Fatal(err)
	}

	// AGENTS.md created and importable; CLAUDE.md preserved verbatim.
	agents, err := os.ReadFile(filepath.Join(tmp, "AGENTS.md"))
	if err != nil {
		t.Fatalf("AGENTS.md not created: %v", err)
	}
	if !strings.Contains(string(agents), "# AGENTS.md") {
		t.Errorf("AGENTS.md missing expected heading:\n%s", agents)
	}
	if got, _ := os.ReadFile(claudePath); string(got) != "# existing project rules\n" {
		t.Errorf("CLAUDE.md was overwritten: %q", got)
	}
	if len(scaffolded) != 1 || filepath.Base(scaffolded[0]) != "AGENTS.md" {
		t.Errorf("callback should report only the created AGENTS.md, got %v", scaffolded)
	}
}

// TestInit_ClaudeOnlySkipsBridge verifies a Claude-only project gets no
// AGENTS.md / CLAUDE.md scaffold — the bridge only appears when it has a
// non-Claude consumer.
func TestInit_ClaudeOnlySkipsBridge(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Targets:       []model.AgentTarget{model.AgentClaude},
		Scope:         model.ScopeProject,
	}); err != nil {
		t.Fatal(err)
	}

	for _, name := range []string{"AGENTS.md", "CLAUDE.md"} {
		if _, err := os.Stat(filepath.Join(tmp, name)); !os.IsNotExist(err) {
			t.Errorf("%s should not be scaffolded for a Claude-only project", name)
		}
	}
}

// TestInit_GitignoreIdempotent verifies re-running init against an
// already-configured .gitignore does not duplicate entries. Regression guard
// for the housekeeping pass that now runs on every init (not just fresh).
func TestInit_GitignoreIdempotent(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

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
	for _, entry := range []string{".sdd/tmp/", ".sdd/config.local.yaml", ".sdd/stats/"} {
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
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

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
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

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
	oldFile, err := model.RenderSkillFile(model.SkillBundleEntry{Content: oldContent}, model.AgentClaude, "v0.1.0", oldHash)
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

	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})
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

	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

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
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

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
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

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
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

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

// TestInit_PersistsSkillScopeOnFreshInit verifies that the scope chosen on
// initial setup lands in .sdd/config.yaml so subsequent runs (and clones)
// reinstall to the same place without re-prompting.
func TestInit_PersistsSkillScopeOnFreshInit(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Scope:         model.ScopeProject,
		ScopeExplicit: true,
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(filepath.Join(tmp, model.SDDDirName, "config.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := model.ParseConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SkillScope != model.ScopeProject {
		t.Errorf("config.yaml skill_scope: got %q, want %q", cfg.SkillScope, model.ScopeProject)
	}
}

// TestInit_UpgradeUpsertsSkillScope verifies the upgrade path: a config.yaml
// that predates skill_scope (no field) gets the value written on the next
// init, picking up the explicit flag value.
func TestInit_UpgradeUpsertsSkillScope(t *testing.T) {
	tmp := t.TempDir()
	sddDir := filepath.Join(tmp, model.SDDDirName)
	if err := os.MkdirAll(sddDir, 0o755); err != nil {
		t.Fatal(err)
	}
	// Pre-existing config without skill_scope (mimicking an older sdd init).
	preExisting := "# SDD configuration\ngraph_dir: .sdd/graph\n"
	configPath := filepath.Join(sddDir, "config.yaml")
	if err := os.WriteFile(configPath, []byte(preExisting), 0o644); err != nil {
		t.Fatal(err)
	}

	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Scope:         model.ScopeProject,
		ScopeExplicit: true,
	}); err != nil {
		t.Fatal(err)
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := model.ParseConfig(data)
	if err != nil {
		t.Fatal(err)
	}
	if cfg.SkillScope != model.ScopeProject {
		t.Errorf("upgraded config.yaml skill_scope: got %q, want %q", cfg.SkillScope, model.ScopeProject)
	}
	// Original graph_dir entry must round-trip — SetYAMLField preserves
	// surrounding keys and comments.
	if !strings.Contains(string(data), "graph_dir: .sdd/graph") {
		t.Errorf("original graph_dir not preserved; got:\n%s", data)
	}
}

// TestInit_RecordedScopeWinsOverFlagDefault verifies that a re-init with no
// explicit --scope uses the recorded value, even when the caller passes a
// different default in cmd.Scope.
func TestInit_RecordedScopeWinsOverFlagDefault(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

	// First run: record skill_scope: project explicitly.
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Scope:         model.ScopeProject,
		ScopeExplicit: true,
	}); err != nil {
		t.Fatal(err)
	}

	// Second run: caller passes Scope: ScopeUser as a default (no explicit
	// flag). The recorded value must win, and the install dir must reflect it.
	var skills command.SkillInstallResult
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:          tmp,
		BinaryVersion:     "v0.2.0",
		Scope:             model.ScopeUser,
		ScopeExplicit:     false,
		UserHome:          tmp, // unused but required when Scope==User would route
		OnSkillsInstalled: func(r command.SkillInstallResult) { skills = r },
	}); err != nil {
		t.Fatal(err)
	}

	wantPrefix := filepath.Join(tmp, ".claude", "skills")
	if !strings.HasPrefix(skills.InstallDir, wantPrefix) {
		t.Errorf("install dir should reflect recorded project scope; got %q, want prefix %q", skills.InstallDir, wantPrefix)
	}
}

// TestInit_ContradictingExplicitScopeErrors verifies AC 2: when the operator
// passes --scope on a re-init and it disagrees with the value persisted in
// .sdd/config.yaml, the run errors with a message naming the conflict and
// pointing at the file.
func TestInit_ContradictingExplicitScopeErrors(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

	// First run records skill_scope: project.
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Scope:         model.ScopeProject,
		ScopeExplicit: true,
	}); err != nil {
		t.Fatal(err)
	}

	// Second run passes --scope=user explicitly. Must error.
	err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Scope:         model.ScopeUser,
		ScopeExplicit: true,
		UserHome:      tmp,
	})
	if err == nil {
		t.Fatal("expected error when explicit --scope contradicts recorded value, got nil")
	}
	if !strings.Contains(err.Error(), "skill_scope") {
		t.Errorf("error should name skill_scope: %v", err)
	}
	if !strings.Contains(err.Error(), "config.yaml") {
		t.Errorf("error should point at config.yaml: %v", err)
	}
}

// TestInit_MatchingExplicitScopeIsNoop verifies that re-init with the same
// scope explicitly passed (the common case for documented invocations) does
// not error and does not rewrite the config.
func TestInit_MatchingExplicitScopeIsNoop(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Scope:         model.ScopeProject,
		ScopeExplicit: true,
	}); err != nil {
		t.Fatal(err)
	}

	configPath := filepath.Join(tmp, model.SDDDirName, "config.yaml")
	before, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}

	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Scope:         model.ScopeProject,
		ScopeExplicit: true,
	}); err != nil {
		t.Fatalf("matching explicit scope should not error: %v", err)
	}

	after, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(before) != string(after) {
		t.Errorf("config.yaml rewritten on matching scope; before:\n%s\nafter:\n%s", before, after)
	}
}

// TestInit_BumpRaisesMinimumVersion verifies AC 7: `sdd init --bump` from a
// released binary higher than the recorded minimum updates meta.json.
func TestInit_BumpRaisesMinimumVersion(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

	// First run stamps minimum_version v0.5.0.
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.5.0",
		Scope:         model.ScopeProject,
		ScopeExplicit: true,
	}); err != nil {
		t.Fatal(err)
	}

	// Bump from a higher binary.
	var bumped struct{ prev, cur string }
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.6.0",
		Scope:         model.ScopeProject,
		ScopeExplicit: true,
		Bump:          true,
		OnMinimumVersionBumped: func(p, c string) {
			bumped.prev = p
			bumped.cur = c
		},
	}); err != nil {
		t.Fatal(err)
	}
	if bumped.prev != "v0.5.0" || bumped.cur != "v0.6.0" {
		t.Errorf("bump callback got prev=%q cur=%q; want v0.5.0, v0.6.0", bumped.prev, bumped.cur)
	}

	data, err := os.ReadFile(filepath.Join(tmp, model.SDDDirName, model.SchemaMetaFileName))
	if err != nil {
		t.Fatal(err)
	}
	meta, err := model.ParseSchemaMeta(data)
	if err != nil {
		t.Fatal(err)
	}
	if meta.MinimumVersion == nil || *meta.MinimumVersion != "v0.6.0" {
		t.Errorf("minimum_version: got %+v, want v0.6.0", meta.MinimumVersion)
	}
}

// TestInit_BumpEqualIsNoop verifies AC 7's no-op clause: --bump from a
// binary equal to the recorded minimum does not rewrite meta.json.
func TestInit_BumpEqualIsNoop(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.5.0",
		Scope:         model.ScopeProject,
		ScopeExplicit: true,
	}); err != nil {
		t.Fatal(err)
	}

	var unchanged string
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:                  tmp,
		BinaryVersion:             "v0.5.0",
		Scope:                     model.ScopeProject,
		ScopeExplicit:             true,
		Bump:                      true,
		OnMinimumVersionUnchanged: func(c string) { unchanged = c },
	}); err != nil {
		t.Fatal(err)
	}
	if unchanged != "v0.5.0" {
		t.Errorf("unchanged callback got %q; want v0.5.0", unchanged)
	}
}

// TestInit_BumpDevBuildErrors verifies AC 7's dev-build refusal with the
// exact error string the plan calls out.
func TestInit_BumpDevBuildErrors(t *testing.T) {
	tmp := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

	// Fresh init from a dev build leaves minimum_version absent — exactly
	// the case where someone might be tempted to "bump" by accident.
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "dev",
		Scope:         model.ScopeProject,
		ScopeExplicit: true,
	}); err != nil {
		t.Fatal(err)
	}

	err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "dev",
		Scope:         model.ScopeProject,
		ScopeExplicit: true,
		Bump:          true,
	})
	if err == nil {
		t.Fatal("expected error on --bump from dev build, got nil")
	}
	if !strings.Contains(err.Error(), "cannot bump from a dev build") {
		t.Errorf("error should match plan wording: %v", err)
	}
}

// initExistingWithAgents runs a fresh init recording the given agents, so a
// follow-up init exercises the existing-tree --agents path (d-tac-jin).
func initExistingWithAgents(t *testing.T, tmp string, agents ...model.AgentTarget) *handlers.Handler {
	t.Helper()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Targets:       agents,
		Scope:         model.ScopeProject,
	}); err != nil {
		t.Fatalf("seed init: %v", err)
	}
	return h
}

// TestInit_ExplicitAgentsPersistOnExistingTree covers the upgrade-path core of
// d-tac-jin: an explicit --agents on an already-initialized repo writes the
// selection to supported_agents (it used to render but never stick).
func TestInit_ExplicitAgentsPersistOnExistingTree(t *testing.T) {
	tmp := t.TempDir()
	h := initExistingWithAgents(t, tmp, model.AgentClaude)

	// Adopt Codex post-init via an explicit selection.
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Targets:       []model.AgentTarget{model.AgentClaude, model.AgentCodex},
		Scope:         model.ScopeProject,
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := model.ParseConfig(readFile(t, filepath.Join(tmp, model.SDDDirName, "config.yaml")))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.SupportedAgents; len(got) != 2 || got[0] != model.AgentClaude || got[1] != model.AgentCodex {
		t.Errorf("supported_agents = %v, want [claude codex] persisted", got)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".agents/skills/sdd/SKILL.md")); err != nil {
		t.Errorf("codex render missing after adoption: %v", err)
	}
}

// TestInit_ExplicitAgentsReplaceDropsAndPrunes verifies the replace semantics:
// a narrower --agents overwrites the recorded list and prunes the dropped
// agent's pristine renders, firing the prune callback.
func TestInit_ExplicitAgentsReplaceDropsAndPrunes(t *testing.T) {
	tmp := t.TempDir()
	h := initExistingWithAgents(t, tmp, model.AgentClaude, model.AgentCodex)

	var pruned []command.AgentPruneResult
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:            tmp,
		BinaryVersion:       "v0.2.0",
		Targets:             []model.AgentTarget{model.AgentClaude},
		Scope:               model.ScopeProject,
		OnAgentSkillsPruned: func(r command.AgentPruneResult) { pruned = append(pruned, r) },
	}); err != nil {
		t.Fatal(err)
	}

	cfg, err := model.ParseConfig(readFile(t, filepath.Join(tmp, model.SDDDirName, "config.yaml")))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.SupportedAgents; len(got) != 1 || got[0] != model.AgentClaude {
		t.Errorf("supported_agents = %v, want [claude] after replace", got)
	}
	// Codex render fully pruned (the dir holds only sdd skills), Claude intact.
	if _, err := os.Stat(filepath.Join(tmp, ".agents/skills")); !os.IsNotExist(err) {
		t.Errorf(".agents/skills should be removed once empty, stat err = %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, ".claude/skills/sdd/SKILL.md")); err != nil {
		t.Errorf("claude render must survive the prune: %v", err)
	}
	if len(pruned) != 1 || pruned[0].Target != model.AgentCodex {
		t.Fatalf("expected one prune callback for codex, got %+v", pruned)
	}
	if len(pruned[0].Removed) == 0 || len(pruned[0].KeptModified) != 0 {
		t.Errorf("prune result: removed=%d keptModified=%d, want removed>0 kept=0", len(pruned[0].Removed), len(pruned[0].KeptModified))
	}
}

// TestInit_PrunePreservesModifiedSkill verifies the symmetric protection: a
// user-modified file in a dropped agent's render is preserved (and reported),
// while its pristine siblings are removed — mirroring install's refusal to
// overwrite a modified file without --force.
func TestInit_PrunePreservesModifiedSkill(t *testing.T) {
	tmp := t.TempDir()
	h := initExistingWithAgents(t, tmp, model.AgentClaude, model.AgentCodex)

	// User edits one codex skill file after install — its hash now diverges
	// from the stored stamp, so it classifies as Modified.
	modified := filepath.Join(tmp, ".agents/skills/sdd/SKILL.md")
	appendToFile(t, modified, "\n<!-- local edit, must survive prune -->\n")

	var pruned []command.AgentPruneResult
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:            tmp,
		BinaryVersion:       "v0.2.0",
		Targets:             []model.AgentTarget{model.AgentClaude},
		Scope:               model.ScopeProject,
		OnAgentSkillsPruned: func(r command.AgentPruneResult) { pruned = append(pruned, r) },
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(modified); err != nil {
		t.Errorf("modified file must be preserved without --force: %v", err)
	}
	// A pristine sibling in another skill dir is gone.
	if _, err := os.Stat(filepath.Join(tmp, ".agents/skills/sdd-catchup/SKILL.md")); !os.IsNotExist(err) {
		t.Errorf("pristine codex skill should be pruned, stat err = %v", err)
	}
	// The parent dir survives because it still holds the modified file.
	if _, err := os.Stat(filepath.Join(tmp, ".agents/skills")); err != nil {
		t.Errorf(".agents/skills should remain (holds the modified file): %v", err)
	}
	if len(pruned) != 1 {
		t.Fatalf("expected one prune callback, got %+v", pruned)
	}
	if !slices.Contains(pruned[0].KeptModified, modified) {
		t.Errorf("KeptModified should name %s, got %v", modified, pruned[0].KeptModified)
	}
	if len(pruned[0].Removed) == 0 {
		t.Error("pristine siblings should still be removed")
	}
}

// TestInit_PruneForceRemovesModified verifies --force makes prune symmetric
// with a forced overwrite: the modified file is removed too, and the emptied
// directory is cleaned up.
func TestInit_PruneForceRemovesModified(t *testing.T) {
	tmp := t.TempDir()
	h := initExistingWithAgents(t, tmp, model.AgentClaude, model.AgentCodex)

	modified := filepath.Join(tmp, ".agents/skills/sdd/SKILL.md")
	appendToFile(t, modified, "\n<!-- local edit -->\n")

	var pruned []command.AgentPruneResult
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:            tmp,
		BinaryVersion:       "v0.2.0",
		Targets:             []model.AgentTarget{model.AgentClaude},
		Scope:               model.ScopeProject,
		Force:               true,
		OnAgentSkillsPruned: func(r command.AgentPruneResult) { pruned = append(pruned, r) },
	}); err != nil {
		t.Fatal(err)
	}

	if _, err := os.Stat(filepath.Join(tmp, ".agents/skills")); !os.IsNotExist(err) {
		t.Errorf("--force prune should remove the modified file and empty the dir, stat err = %v", err)
	}
	if len(pruned) != 1 || len(pruned[0].KeptModified) != 0 {
		t.Errorf("under --force nothing should be kept, got %+v", pruned)
	}
	if !slices.Contains(pruned[0].Removed, modified) {
		t.Errorf("Removed should include the forced modified file %s, got %v", modified, pruned[0].Removed)
	}
}

// TestInit_BareInitLeavesRecordedAgents guards that a bare init (no --agents)
// on an existing tree neither re-persists nor prunes — the recorded value and
// every render stay put.
func TestInit_BareInitLeavesRecordedAgents(t *testing.T) {
	tmp := t.TempDir()
	h := initExistingWithAgents(t, tmp, model.AgentClaude, model.AgentCodex)

	configPath := filepath.Join(tmp, model.SDDDirName, "config.yaml")
	before := readFile(t, configPath)

	var pruned []command.AgentPruneResult
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:            tmp,
		BinaryVersion:       "v0.2.0",
		Scope:               model.ScopeProject,
		OnAgentSkillsPruned: func(r command.AgentPruneResult) { pruned = append(pruned, r) },
	}); err != nil {
		t.Fatal(err)
	}

	if string(before) != string(readFile(t, configPath)) {
		t.Error("bare init rewrote config.yaml")
	}
	if len(pruned) != 0 {
		t.Errorf("bare init should not prune, got %+v", pruned)
	}
	for _, rel := range []string{".claude/skills/sdd/SKILL.md", ".agents/skills/sdd/SKILL.md"} {
		if _, err := os.Stat(filepath.Join(tmp, rel)); err != nil {
			t.Errorf("render %s should survive a bare init: %v", rel, err)
		}
	}
}

// TestInit_FailedPruneDoesNotAdvanceConfig guards the prune ordering:
// supported_agents must be persisted only AFTER the prune succeeds. If the
// prune fails midway, the recorded set must stay intact so a re-run retries
// the drop — otherwise config would claim an agent is gone while its files
// (partially) remain, and a later bare init would never clean them up.
func TestInit_FailedPruneDoesNotAdvanceConfig(t *testing.T) {
	tmp := t.TempDir()
	initExistingWithAgents(t, tmp, model.AgentClaude, model.AgentCodex)

	// Reader that errors when classifying codex skills, so the prune fails.
	failing := pruneFailReader{Reader: finders.New(finders.Options{}), failFor: model.AgentCodex}
	h := handlers.New(handlers.Options{Reader: failing})

	err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Targets:       []model.AgentTarget{model.AgentClaude},
		Scope:         model.ScopeProject,
	})
	if err == nil {
		t.Fatal("expected init to fail when the prune errors")
	}

	cfg, perr := model.ParseConfig(readFile(t, filepath.Join(tmp, model.SDDDirName, "config.yaml")))
	if perr != nil {
		t.Fatal(perr)
	}
	if got := cfg.SupportedAgents; len(got) != 2 || got[0] != model.AgentClaude || got[1] != model.AgentCodex {
		t.Errorf("failed prune advanced supported_agents to %v; the drop must not be committed until the prune succeeds", got)
	}
}

// TestInit_UserScopeSkipsPrune guards the prune scope: under user (global)
// scope the skill dirs are shared across every project on the machine, so
// dropping an agent must update the recorded list but must NOT delete the
// shared render — another project may rely on it. Prune is project-scope only
// (d-tac-jin).
func TestInit_UserScopeSkipsPrune(t *testing.T) {
	tmp := t.TempDir()
	home := t.TempDir()
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})

	// Seed under user scope with both agents — renders land in the shared home.
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:      tmp,
		BinaryVersion: "v0.2.0",
		Targets:       []model.AgentTarget{model.AgentClaude, model.AgentCodex},
		Scope:         model.ScopeUser,
		ScopeExplicit: true,
		UserHome:      home,
	}); err != nil {
		t.Fatal(err)
	}
	codexSkill := filepath.Join(home, ".agents/skills/sdd/SKILL.md")
	if _, err := os.Stat(codexSkill); err != nil {
		t.Fatalf("seed codex render missing: %v", err)
	}

	// Drop codex under user scope.
	var pruned []command.AgentPruneResult
	if err := h.Init(context.Background(), &command.InitCmd{
		RepoRoot:            tmp,
		BinaryVersion:       "v0.2.0",
		Targets:             []model.AgentTarget{model.AgentClaude},
		Scope:               model.ScopeUser,
		UserHome:            home,
		OnAgentSkillsPruned: func(r command.AgentPruneResult) { pruned = append(pruned, r) },
	}); err != nil {
		t.Fatal(err)
	}

	// The recorded list still drops codex...
	cfg, err := model.ParseConfig(readFile(t, filepath.Join(tmp, model.SDDDirName, "config.yaml")))
	if err != nil {
		t.Fatal(err)
	}
	if got := cfg.SupportedAgents; len(got) != 1 || got[0] != model.AgentClaude {
		t.Errorf("supported_agents = %v, want [claude] persisted under user scope too", got)
	}
	// ...but the shared global render is left untouched, and no prune fires.
	if _, err := os.Stat(codexSkill); err != nil {
		t.Errorf("user-scope drop must not delete the shared codex render: %v", err)
	}
	if len(pruned) != 0 {
		t.Errorf("no prune callback should fire under user scope, got %+v", pruned)
	}
}

func readFile(t *testing.T, path string) []byte {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read %s: %v", path, err)
	}
	return data
}

func appendToFile(t *testing.T, path, suffix string) {
	t.Helper()
	data := readFile(t, path)
	if err := os.WriteFile(path, append(data, []byte(suffix)...), 0o644); err != nil {
		t.Fatalf("append to %s: %v", path, err)
	}
}

// TestInit_RepoIDDerivation covers repo_id recording: fresh init derives it
// from the remote URL, an existing config that predates the field gets an
// upsert, and a recorded value is never overwritten.
func TestInit_RepoIDDerivation(t *testing.T) {
	h := handlers.New(handlers.Options{Reader: finders.New(finders.Options{})})
	base := &command.InitCmd{
		BinaryVersion: "v0.2.0",
		Targets:       []model.AgentTarget{model.AgentClaude},
		Scope:         model.ScopeProject,
	}

	t.Run("fresh init derives and records", func(t *testing.T) {
		tmp := t.TempDir()
		cmd := *base
		cmd.RepoRoot = tmp
		cmd.RemoteURL = "git@github.com:networkteam/other.git"
		var written string
		cmd.OnRepoIDWritten = func(id string) { written = id }
		if err := h.Init(context.Background(), &cmd); err != nil {
			t.Fatal(err)
		}
		if written != "github.com/networkteam/other" {
			t.Errorf("OnRepoIDWritten = %q", written)
		}
		data, err := os.ReadFile(filepath.Join(tmp, ".sdd/config.yaml"))
		if err != nil {
			t.Fatal(err)
		}
		cfg, err := model.ParseConfig(data)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.RepoID != "github.com/networkteam/other" {
			t.Errorf("recorded repo_id = %q", cfg.RepoID)
		}
	})

	t.Run("no remote stays local-only", func(t *testing.T) {
		tmp := t.TempDir()
		cmd := *base
		cmd.RepoRoot = tmp
		if err := h.Init(context.Background(), &cmd); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(tmp, ".sdd/config.yaml"))
		cfg, err := model.ParseConfig(data)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.RepoID != "" {
			t.Errorf("repo_id = %q, want empty for local-only", cfg.RepoID)
		}
	})

	t.Run("upgrade upserts absent field, preserves recorded value", func(t *testing.T) {
		tmp := t.TempDir()
		cmd := *base
		cmd.RepoRoot = tmp
		if err := h.Init(context.Background(), &cmd); err != nil {
			t.Fatal(err)
		}

		// Re-init with a remote: the pre-field config gains repo_id.
		cmd2 := *base
		cmd2.RepoRoot = tmp
		cmd2.RemoteURL = "https://github.com/networkteam/other.git"
		var written string
		cmd2.OnRepoIDWritten = func(id string) { written = id }
		if err := h.Init(context.Background(), &cmd2); err != nil {
			t.Fatal(err)
		}
		if written != "github.com/networkteam/other" {
			t.Errorf("upgrade OnRepoIDWritten = %q", written)
		}

		// A third init with a different remote must not overwrite.
		cmd3 := *base
		cmd3.RepoRoot = tmp
		cmd3.RemoteURL = "https://example.com/team/moved.git"
		cmd3.OnRepoIDWritten = func(id string) { t.Errorf("recorded repo_id overwritten with %q", id) }
		if err := h.Init(context.Background(), &cmd3); err != nil {
			t.Fatal(err)
		}
		data, _ := os.ReadFile(filepath.Join(tmp, ".sdd/config.yaml"))
		cfg, err := model.ParseConfig(data)
		if err != nil {
			t.Fatal(err)
		}
		if cfg.RepoID != "github.com/networkteam/other" {
			t.Errorf("repo_id after re-init = %q", cfg.RepoID)
		}
	})
}
