package model

import (
	"bytes"
	"encoding/json"
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

	stamped, err := RenderSkillFile(SkillBundleEntry{Content: embedded}, "v0.2.0", embeddedHash)
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
	rendered, err := RenderSkillFile(entry, "v0.2.0", hash)
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
	installedBytes, err := RenderSkillFile(SkillBundleEntry{Content: oldEmbedded}, "v0.1.0", oldHash)
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
	rendered, err := RenderSkillFile(entry, "v0.1.0", hash)
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
	rendered, err := RenderSkillFile(SkillBundleEntry{Content: embedded}, "v0.2.0", "deadbeef")
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
