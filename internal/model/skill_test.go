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

func findSkillEntry(t *testing.T, entries []SkillBundleEntry, relPath string) SkillBundleEntry {
	t.Helper()
	for _, e := range entries {
		if e.RelPath == relPath {
			return e
		}
	}
	t.Fatalf("entry %q not found", relPath)
	return SkillBundleEntry{}
}

func TestResolveSkillIncludes_ReplacesMarkerWithBody(t *testing.T) {
	entries := []SkillBundleEntry{
		{Skill: "sdd", RelPath: "references/frag.md", Content: []byte("VOCAB LINE ONE\nVOCAB LINE TWO\n")},
		{Skill: "sdd", RelPath: "references/host.md", Content: []byte("before\n<!-- sdd:include references/frag.md -->\nafter\n")},
	}
	out, err := ResolveSkillIncludes(entries)
	if err != nil {
		t.Fatalf("ResolveSkillIncludes: %v", err)
	}
	host := findSkillEntry(t, out, "references/host.md")
	if bytes.Contains(host.Content, []byte("sdd:include")) {
		t.Errorf("marker not resolved:\n%s", host.Content)
	}
	for _, want := range []string{"before", "VOCAB LINE ONE", "VOCAB LINE TWO", "after"} {
		if !bytes.Contains(host.Content, []byte(want)) {
			t.Errorf("resolved content missing %q:\n%s", want, host.Content)
		}
	}
}

func TestResolveSkillIncludes_MissingTargetErrors(t *testing.T) {
	entries := []SkillBundleEntry{
		{Skill: "sdd", RelPath: "host.md", Content: []byte("<!-- sdd:include references/nope.md -->\n")},
	}
	if _, err := ResolveSkillIncludes(entries); err == nil {
		t.Fatal("want error for missing include target, got nil")
	}
}

func TestResolveSkillIncludes_NoMarkerUnchanged(t *testing.T) {
	entries := []SkillBundleEntry{
		{Skill: "sdd", RelPath: "plain.md", Content: []byte("no markers here\n")},
	}
	out, err := ResolveSkillIncludes(entries)
	if err != nil {
		t.Fatalf("ResolveSkillIncludes: %v", err)
	}
	if !bytes.Equal(out[0].Content, entries[0].Content) {
		t.Error("content changed though there was no marker")
	}
}

func TestResolveSkillIncludes_StripsIncludedFrontmatter(t *testing.T) {
	entries := []SkillBundleEntry{
		{Skill: "sdd", RelPath: "references/frag.md", Content: []byte("---\nstamp: x\n---\nFRAGMENT BODY\n")},
		{Skill: "sdd", RelPath: "host.md", Content: []byte("<!-- sdd:include references/frag.md -->\n")},
	}
	out, err := ResolveSkillIncludes(entries)
	if err != nil {
		t.Fatalf("ResolveSkillIncludes: %v", err)
	}
	host := findSkillEntry(t, out, "host.md")
	if bytes.Contains(host.Content, []byte("stamp: x")) {
		t.Errorf("included frontmatter should be stripped:\n%s", host.Content)
	}
	if !bytes.Contains(host.Content, []byte("FRAGMENT BODY")) {
		t.Errorf("included body missing:\n%s", host.Content)
	}
}

// TestResolveSkillIncludes_ResolvedContentDriftStable proves the install writer
// and drift detection agree on the *resolved* content: a resolved entry written
// with stamps and parsed back classifies as Current against that same resolved
// entry. This is the property that keeps included skills out of false "modified"
// state — both paths consume Load's resolved output.
func TestResolveSkillIncludes_ResolvedContentDriftStable(t *testing.T) {
	entries := []SkillBundleEntry{
		{Skill: "sdd", RelPath: "references/frag.md", Content: []byte("FRAGMENT\n")},
		{Skill: "sdd", RelPath: "host.md", Content: []byte("---\nname: host\n---\nbefore\n<!-- sdd:include references/frag.md -->\nafter\n")},
	}
	out, err := ResolveSkillIncludes(entries)
	if err != nil {
		t.Fatalf("ResolveSkillIncludes: %v", err)
	}
	host := findSkillEntry(t, out, "host.md")
	hash := ComputeSkillHash(host.Content)
	rendered, err := RenderSkillFile(host, "v1", hash)
	if err != nil {
		t.Fatalf("RenderSkillFile: %v", err)
	}
	installed := ParseSkillFile("/tmp/host.md", rendered)
	if got := ComputeSkillStatus(host, installed); got != SkillStatusCurrent {
		t.Errorf("resolved entry should classify Current, got %s", got)
	}
}
