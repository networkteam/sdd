package meta

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/networkteam/sdd/internal/model"
)

func TestReadLastFetch_MissingReturnsZero(t *testing.T) {
	sddDir := t.TempDir()
	got, err := ReadLastFetch(sddDir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !got.IsZero() {
		t.Errorf("expected zero time for missing marker, got %v", got)
	}
}

func TestTouchLastFetch_CreatesAndUpdates(t *testing.T) {
	sddDir := t.TempDir()

	// First touch creates the file.
	if err := TouchLastFetch(sddDir); err != nil {
		t.Fatalf("first TouchLastFetch: %v", err)
	}
	first, err := ReadLastFetch(sddDir)
	if err != nil {
		t.Fatalf("ReadLastFetch after first touch: %v", err)
	}
	if first.IsZero() {
		t.Fatal("expected non-zero mtime after touch")
	}

	// Verify the file lives at .sdd/tmp/last-fetch.
	expected := filepath.Join(sddDir, "tmp", "last-fetch")
	if _, err := os.Stat(expected); err != nil {
		t.Errorf("expected marker at %s, got %v", expected, err)
	}

	// Second touch updates the mtime.
	if err := os.Chtimes(expected, time.Now().Add(-time.Hour), time.Now().Add(-time.Hour)); err != nil {
		t.Fatalf("backdating mtime: %v", err)
	}
	backdated, _ := ReadLastFetch(sddDir)
	if err := TouchLastFetch(sddDir); err != nil {
		t.Fatalf("second TouchLastFetch: %v", err)
	}
	after, _ := ReadLastFetch(sddDir)
	if !after.After(backdated) {
		t.Errorf("second touch did not bump mtime: before=%v after=%v", backdated, after)
	}
}

// ResolveConfig composes the full overlay: global base under the committed
// config.yaml under config.local.yaml — later layers win, untouched fields
// inherit.
func TestResolveConfig_OverlayOrder(t *testing.T) {
	sddDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sddDir, "config.yaml"), []byte("participant: FromCommitted\ngraph_dir: .sdd/graph\nllm:\n  model: committed-model\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(sddDir, "config.local.yaml"), []byte("llm:\n  model: local-model\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	global := model.BaseConfig{
		Participant: "FromGlobal",
		LLM:         model.LLMConfig{Provider: "ollama", Model: "global-model"},
	}
	cfg, err := ResolveConfig(global, sddDir)
	if err != nil {
		t.Fatalf("ResolveConfig: %v", err)
	}
	if cfg.Participant != "FromCommitted" {
		t.Errorf("committed file should override global participant, got %q", cfg.Participant)
	}
	if cfg.LLM.Model != "local-model" {
		t.Errorf("local file should win over committed and global, got %q", cfg.LLM.Model)
	}
	if cfg.LLM.Provider != "ollama" {
		t.Errorf("untouched field should inherit from global, got %q", cfg.LLM.Provider)
	}
	if cfg.GraphDir != ".sdd/graph" {
		t.Errorf("per-repo field lost: %q", cfg.GraphDir)
	}
}

// With no per-repo config files, global settings still resolve; with no
// layers at all, ResolveConfig keeps the legacy nil "no config" contract.
func TestResolveConfig_GlobalOnlyAndEmpty(t *testing.T) {
	sddDir := t.TempDir()
	cfg, err := ResolveConfig(model.BaseConfig{Participant: "Christopher"}, sddDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg == nil || cfg.Participant != "Christopher" {
		t.Errorf("global-only resolution failed: %+v", cfg)
	}
	cfg, err = ResolveConfig(model.BaseConfig{}, sddDir)
	if err != nil {
		t.Fatal(err)
	}
	if cfg != nil {
		t.Errorf("no layers should resolve to nil, got %+v", cfg)
	}
}

// A parse failure names the offending file — the error carries the path of
// the exact layer that broke, plus the key from the strict decoder.
func TestReadConfig_ErrorNamesFileAndKey(t *testing.T) {
	sddDir := t.TempDir()
	if err := os.WriteFile(filepath.Join(sddDir, "config.local.yaml"), []byte("repos:\n  - repo_id: a/b\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ReadConfig(sddDir)
	if err == nil {
		t.Fatal("global-only key in a per-repo file must fail")
	}
	if !strings.Contains(err.Error(), "config.local.yaml") {
		t.Errorf("error should name the file, got: %v", err)
	}
	if !strings.Contains(err.Error(), "repos") {
		t.Errorf("error should name the key, got: %v", err)
	}
}
