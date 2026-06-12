package model

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"testing"
)

func TestCanonicalizeFrontmatter_SortsKeys(t *testing.T) {
	a := CanonicalizeFrontmatter(map[string]any{"b": 1, "a": 2, "c": 3})
	b := CanonicalizeFrontmatter(map[string]any{"c": 3, "b": 1, "a": 2})
	if !bytes.Equal(a, b) {
		t.Errorf("same keys in different insertion order should canonicalize identically:\n a=%s\n b=%s", a, b)
	}
	// Verify it's actually sorted.
	var parsed map[string]json.RawMessage
	if err := json.Unmarshal(a, &parsed); err != nil {
		t.Fatalf("canonical form is not valid JSON: %v", err)
	}
}

func TestCanonicalizeFrontmatter_NestedMaps(t *testing.T) {
	fm := map[string]any{
		"outer": map[string]any{"z": 1, "a": 2},
		"list":  []any{"x", "y"},
	}
	got := CanonicalizeFrontmatter(fm)
	// Expect sorted keys at both levels.
	want := `{"list":["x","y"],"outer":{"a":2,"z":1}}`
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestCanonicalizeFrontmatter_YamlMapAnyAny(t *testing.T) {
	// yaml.v3 occasionally produces map[any]any for deeply nested decodes;
	// the canonicalizer must normalize these to sorted string-keyed maps.
	fm := map[string]any{
		"nested": map[any]any{"a": 1, "b": 2},
	}
	got := CanonicalizeFrontmatter(fm)
	want := `{"nested":{"a":1,"b":2}}`
	if string(got) != want {
		t.Errorf("got %s, want %s", got, want)
	}
}

func TestComputeSkillHash_StableAcrossStamping(t *testing.T) {
	// The embedded content (no stamps) and a re-stamped installation must
	// hash to the same value — that's what lets a pristine file be detected
	// as pristine after an install round-trip.
	embedded := []byte(`---
name: sdd
description: Signal-Dialogue-Decision helper
---
# SDD

Body content here.
`)
	embeddedHash := ComputeSkillHash(embedded)

	stamped, err := RenderSkillFile(SkillBundleEntry{Content: embedded}, AgentClaude, "v0.2.0", embeddedHash)
	if err != nil {
		t.Fatalf("RenderSkillFile: %v", err)
	}
	stampedHash := ComputeSkillHash(stamped)

	if embeddedHash != stampedHash {
		t.Errorf("stamped file should hash identically to embedded:\n  embedded = %s\n  stamped  = %s", embeddedHash, stampedHash)
	}
}

func TestComputeSkillHash_ChangesWithBody(t *testing.T) {
	a := []byte("---\nname: x\n---\nbody one\n")
	b := []byte("---\nname: x\n---\nbody two\n")
	if ComputeSkillHash(a) == ComputeSkillHash(b) {
		t.Errorf("hash should differ when body differs")
	}
}

func TestComputeSkillHash_ChangesWithFrontmatter(t *testing.T) {
	a := []byte("---\nname: x\n---\nbody\n")
	b := []byte("---\nname: y\n---\nbody\n")
	if ComputeSkillHash(a) == ComputeSkillHash(b) {
		t.Errorf("hash should differ when non-stamp frontmatter differs")
	}
}

func TestComputeSkillStatus_Missing(t *testing.T) {
	entry := SkillBundleEntry{Content: []byte("body\n")}
	if got := ComputeSkillStatus(entry, nil); got != SkillStatusMissing {
		t.Errorf("got %s, want %s", got, SkillStatusMissing)
	}
}

func TestComputeSkillStatus_Current(t *testing.T) {
	embedded := []byte("---\nname: a\n---\nbody\n")
	entry := SkillBundleEntry{Content: embedded}
	hash := ComputeSkillHash(embedded)
	rendered, err := RenderSkillFile(entry, AgentClaude, "v0.2.0", hash)
	if err != nil {
		t.Fatalf("RenderSkillFile: %v", err)
	}
	installed := ParseSkillFile("/tmp/a", rendered)

	if got := ComputeSkillStatus(entry, installed); got != SkillStatusCurrent {
		t.Errorf("got %s, want %s", got, SkillStatusCurrent)
	}
}

func TestComputeSkillStatus_Pristine(t *testing.T) {
	// Simulate an old install: stamped with an older embedded content, now
	// the embedded content has changed.
	oldEmbedded := []byte("---\nname: a\n---\nold body\n")
	oldHash := ComputeSkillHash(oldEmbedded)
	installedBytes, err := RenderSkillFile(SkillBundleEntry{Content: oldEmbedded}, AgentClaude, "v0.1.0", oldHash)
	if err != nil {
		t.Fatalf("RenderSkillFile: %v", err)
	}
	installed := ParseSkillFile("/tmp/a", installedBytes)

	newEmbedded := SkillBundleEntry{Content: []byte("---\nname: a\n---\nnew body\n")}

	if got := ComputeSkillStatus(newEmbedded, installed); got != SkillStatusPristine {
		t.Errorf("got %s, want %s", got, SkillStatusPristine)
	}
}

func TestComputeSkillStatus_UnstampedMatchingIsPristine(t *testing.T) {
	// First-run adoption: an existing unstamped file that byte-matches the
	// embedded bundle should refresh silently, not reflexively prompt.
	embedded := []byte("---\nname: a\n---\nbody\n")
	installed := ParseSkillFile("/tmp/a", embedded) // no stamps injected
	if got := ComputeSkillStatus(SkillBundleEntry{Content: embedded}, installed); got != SkillStatusPristine {
		t.Errorf("got %s, want %s", got, SkillStatusPristine)
	}
}

func TestComputeSkillStatus_UnstampedDifferentIsModified(t *testing.T) {
	embedded := []byte("---\nname: a\n---\nembedded body\n")
	edited := []byte("---\nname: a\n---\nedited body\n")
	installed := ParseSkillFile("/tmp/a", edited)
	if got := ComputeSkillStatus(SkillBundleEntry{Content: embedded}, installed); got != SkillStatusModified {
		t.Errorf("got %s, want %s", got, SkillStatusModified)
	}
}

func TestComputeSkillStatus_Modified(t *testing.T) {
	// User edited an installed file — computed hash no longer matches stored.
	embedded := []byte("---\nname: a\n---\nbody\n")
	entry := SkillBundleEntry{Content: embedded}
	hash := ComputeSkillHash(embedded)
	rendered, err := RenderSkillFile(entry, AgentClaude, "v0.1.0", hash)
	if err != nil {
		t.Fatalf("RenderSkillFile: %v", err)
	}
	// Simulate user edit: append text to the body.
	edited := append(bytes.Clone(rendered), []byte("extra body text\n")...)
	installed := ParseSkillFile("/tmp/a", edited)

	if got := ComputeSkillStatus(entry, installed); got != SkillStatusModified {
		t.Errorf("got %s, want %s", got, SkillStatusModified)
	}
}

func TestSplitFrontmatter_NoFrontmatter(t *testing.T) {
	content := []byte("# Just markdown\n\nno frontmatter here.\n")
	fm, body := splitFrontmatter(content)
	if fm != nil {
		t.Errorf("expected nil frontmatter, got %v", fm)
	}
	if !bytes.Equal(body, content) {
		t.Errorf("body should equal content when no frontmatter")
	}
}

func TestRenderSkillFile_RoundTripsThroughParse(t *testing.T) {
	embedded := []byte("---\nname: x\ndescription: d\n---\nbody\n")
	rendered, err := RenderSkillFile(SkillBundleEntry{Content: embedded}, AgentClaude, "v0.2.0", "deadbeef")
	if err != nil {
		t.Fatalf("RenderSkillFile: %v", err)
	}
	sf := ParseSkillFile("/tmp/x", rendered)
	if sf.StoredVersion != "v0.2.0" {
		t.Errorf("StoredVersion: got %q, want v0.2.0", sf.StoredVersion)
	}
	if sf.StoredHash != "deadbeef" {
		t.Errorf("StoredHash: got %q, want deadbeef", sf.StoredHash)
	}
}

// TestRenderSkillFile_CodexNestsStampsUnderMetadata proves the Codex render
// keeps stamps out of the top-level frontmatter (which the Agent Skills
// standard rejects) by nesting them under metadata:, while the read and hash
// paths still recognise them — so a fresh Codex install classifies as Current.
func TestRenderSkillFile_CodexNestsStampsUnderMetadata(t *testing.T) {
	embedded := []byte("---\nname: sdd\ndescription: d\ncompatibility: Designed for OpenAI Codex\n---\nbody\n")
	hash := ComputeSkillHash(embedded)
	rendered, err := RenderSkillFile(SkillBundleEntry{Content: embedded}, AgentCodex, "v0.2.0", hash)
	if err != nil {
		t.Fatalf("RenderSkillFile: %v", err)
	}

	fm, _ := splitFrontmatter(rendered)
	if _, top := fm[SkillStampVersion]; top {
		t.Error("codex render placed sdd-version at the top level")
	}
	meta, ok := asStringMap(fm["metadata"])
	if !ok {
		t.Fatalf("codex render did not produce a metadata map: %v", fm)
	}
	if meta[SkillStampVersion] != "v0.2.0" || meta[SkillStampHash] != hash {
		t.Errorf("stamps not nested under metadata: %v", meta)
	}

	installed := ParseSkillFile("/tmp/sdd", rendered)
	if installed.StoredVersion != "v0.2.0" || installed.StoredHash != hash {
		t.Errorf("ParseSkillFile did not read nested stamps: %+v", installed)
	}
	if got := ComputeSkillStatus(SkillBundleEntry{Content: embedded}, installed); got != SkillStatusCurrent {
		t.Errorf("codex rendered entry should classify Current, got %s", got)
	}
}

// TestRenderedEntryDriftStable proves the install writer and drift detection
// agree on bundle content: an entry written with stamps and parsed back
// classifies as Current against that same entry. This is the property that keeps
// freshly-installed skills out of false "modified" state.
func TestRenderedEntryDriftStable(t *testing.T) {
	host := SkillBundleEntry{Skill: "sdd", RelPath: "host.md", Content: []byte("---\nname: host\n---\nbefore\nbody\nafter\n")}
	hash := ComputeSkillHash(host.Content)
	rendered, err := RenderSkillFile(host, AgentClaude, "v1", hash)
	if err != nil {
		t.Fatalf("RenderSkillFile: %v", err)
	}
	installed := ParseSkillFile("/tmp/host.md", rendered)
	if got := ComputeSkillStatus(host, installed); got != SkillStatusCurrent {
		t.Errorf("rendered entry should classify Current, got %s", got)
	}
}

func TestSkillInstallDir_PerAgentSubpath(t *testing.T) {
	cases := []struct {
		target AgentTarget
		scope  Scope
		want   string
	}{
		{AgentClaude, ScopeProject, filepath.Join("/repo", ".claude", "skills")},
		{AgentCodex, ScopeProject, filepath.Join("/repo", ".agents", "skills")},
		{AgentClaude, ScopeUser, filepath.Join("/home", ".claude", "skills")},
		{AgentCodex, ScopeUser, filepath.Join("/home", ".agents", "skills")},
	}
	for _, tc := range cases {
		got, err := SkillInstallDir(tc.target, tc.scope, "/repo", "/home")
		if err != nil {
			t.Fatalf("SkillInstallDir(%s, %s): %v", tc.target, tc.scope, err)
		}
		if got != tc.want {
			t.Errorf("SkillInstallDir(%s, %s) = %q, want %q", tc.target, tc.scope, got, tc.want)
		}
	}
}

func TestParseAgentTarget(t *testing.T) {
	for _, s := range []string{"claude", "codex"} {
		got, err := ParseAgentTarget(s)
		if err != nil {
			t.Errorf("ParseAgentTarget(%q) unexpected error: %v", s, err)
		}
		if string(got) != s {
			t.Errorf("ParseAgentTarget(%q) = %q, want %q", s, got, s)
		}
	}
	if _, err := ParseAgentTarget("gemini"); err == nil {
		t.Error(`ParseAgentTarget("gemini") should error on an unknown target`)
	}
}
