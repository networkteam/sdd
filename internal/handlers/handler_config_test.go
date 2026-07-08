package handlers

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/command"
	"github.com/networkteam/sdd/internal/git"
	"github.com/networkteam/sdd/internal/repos"
)

func newConfigTestHandler(t *testing.T) (*Handler, string, string) {
	t.Helper()
	dir := t.TempDir()
	globalPath := filepath.Join(dir, "xdg", "sdd", "config.yaml")
	sddDir := filepath.Join(dir, "repo", ".sdd")
	if err := os.MkdirAll(sddDir, 0o755); err != nil {
		t.Fatal(err)
	}
	reg := repos.NewRegistry(repos.Locations{
		ConfigPath: globalPath,
		CacheRoot:  filepath.Join(dir, "cache"),
	})
	h := New(Options{
		SDDDir: sddDir,
		Repos:  repos.NewManager(reg, git.CLI{}),
	})
	return h, globalPath, sddDir
}

func TestConfigSet_GlobalCreatesAndPreservesComments(t *testing.T) {
	h, globalPath, _ := newConfigTestHandler(t)

	if err := h.ConfigSet(context.Background(), &command.ConfigSetCmd{Target: "global", Key: "participant", Value: "Christopher"}); err != nil {
		t.Fatalf("first set: %v", err)
	}
	data, err := os.ReadFile(globalPath)
	if err != nil {
		t.Fatalf("global file not created: %v", err)
	}
	if !strings.Contains(string(data), "participant: Christopher") {
		t.Errorf("participant not written: %s", data)
	}

	// A user comment survives subsequent sets.
	commented := "# my note\n" + string(data)
	if err := os.WriteFile(globalPath, []byte(commented), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := h.ConfigSet(context.Background(), &command.ConfigSetCmd{Target: "global", Key: "llm.model", Value: "claude-sonnet-4-6"}); err != nil {
		t.Fatalf("second set: %v", err)
	}
	data, _ = os.ReadFile(globalPath)
	if !strings.Contains(string(data), "# my note") {
		t.Errorf("comment lost: %s", data)
	}
	if !strings.Contains(string(data), "model: claude-sonnet-4-6") {
		t.Errorf("nested key not written: %s", data)
	}
}

func TestConfigSet_LocalWritesRepoFile(t *testing.T) {
	h, _, sddDir := newConfigTestHandler(t)
	if err := h.ConfigSet(context.Background(), &command.ConfigSetCmd{Target: "local", Key: "embedding.provider", Value: "ollama"}); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(filepath.Join(sddDir, "config.local.yaml"))
	if err != nil {
		t.Fatalf("local file not created: %v", err)
	}
	if !strings.Contains(string(data), "provider: ollama") {
		t.Errorf("embedding.provider not written: %s", data)
	}
}

// A key foreign to the target layer is rejected before anything lands on
// disk — the schema split doing its job at write time.
func TestConfigSet_RejectsForeignKeys(t *testing.T) {
	h, globalPath, sddDir := newConfigTestHandler(t)

	err := h.ConfigSet(context.Background(), &command.ConfigSetCmd{Target: "global", Key: "repo_id", Value: "a/b"})
	if err == nil || !strings.Contains(err.Error(), "repo_id") {
		t.Errorf("repo_id in global must be rejected, got %v", err)
	}
	if _, statErr := os.Stat(globalPath); !os.IsNotExist(statErr) {
		t.Error("rejected write must not create the global file")
	}

	err = h.ConfigSet(context.Background(), &command.ConfigSetCmd{Target: "local", Key: "bogus.key", Value: "x"})
	if err == nil || !strings.Contains(err.Error(), "bogus") {
		t.Errorf("unknown key in local must be rejected, got %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(sddDir, "config.local.yaml")); !os.IsNotExist(statErr) {
		t.Error("rejected write must not create the local file")
	}
}

// Numeric values land as YAML numbers, not strings — a string in an int
// field would fail the strict schema probe.
func TestConfigSet_TypedScalars(t *testing.T) {
	h, globalPath, _ := newConfigTestHandler(t)
	if err := h.ConfigSet(context.Background(), &command.ConfigSetCmd{Target: "global", Key: "llm.concurrency", Value: "8"}); err != nil {
		t.Fatal(err)
	}
	data, _ := os.ReadFile(globalPath)
	if !strings.Contains(string(data), "concurrency: 8") {
		t.Errorf("concurrency not written as number: %s", data)
	}
}
