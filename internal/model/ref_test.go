package model

import (
	"strings"
	"testing"

	"gopkg.in/yaml.v3"
)

func TestRefUnmarshal_BareString_LegacyFallback(t *testing.T) {
	const doc = `- 20260101-000000-s-cpt-aaa
`
	var refs []Ref
	if err := yaml.Unmarshal([]byte(doc), &refs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("len = %d, want 1", len(refs))
	}
	if refs[0].ID != "20260101-000000-s-cpt-aaa" {
		t.Errorf("ID = %q", refs[0].ID)
	}
	if refs[0].Kind != RefKindUnknown {
		t.Errorf("Kind = %q, want unknown (legacy bare-string fallback)", refs[0].Kind)
	}
	if refs[0].Desc != "" {
		t.Errorf("Desc = %q, want empty", refs[0].Desc)
	}
}

func TestRefUnmarshal_ObjectForm(t *testing.T) {
	const doc = `- id: 20260101-000000-s-cpt-aaa
  kind: addresses
  desc: addresses the gap
`
	var refs []Ref
	if err := yaml.Unmarshal([]byte(doc), &refs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(refs) != 1 {
		t.Fatalf("len = %d, want 1", len(refs))
	}
	want := Ref{ID: "20260101-000000-s-cpt-aaa", Kind: RefKindAddresses, Desc: "addresses the gap"}
	if refs[0] != want {
		t.Errorf("ref = %+v, want %+v", refs[0], want)
	}
}

func TestRefUnmarshal_ObjectForm_NoDesc(t *testing.T) {
	const doc = `- id: 20260101-000000-s-cpt-aaa
  kind: grounded-in
`
	var refs []Ref
	if err := yaml.Unmarshal([]byte(doc), &refs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if refs[0].Desc != "" {
		t.Errorf("Desc = %q, want empty when omitted", refs[0].Desc)
	}
	if refs[0].Kind != RefKindGroundedIn {
		t.Errorf("Kind = %q, want grounded-in", refs[0].Kind)
	}
}

func TestRefUnmarshal_LegacyAliasResolved(t *testing.T) {
	// Legacy on-disk grounds/evidence resolve to grounded-in at parse time, so
	// nothing above the parser sees the old value. History is never rewritten —
	// the alias lives only in the read path. OnDiskKind still reports the raw
	// stored value so the summary prompt can render it verbatim (s-tac-koz).
	for _, kind := range []string{"grounds", "evidence"} {
		t.Run(kind, func(t *testing.T) {
			doc := "- id: 20260101-000000-s-cpt-aaa\n  kind: " + kind + "\n"
			var refs []Ref
			if err := yaml.Unmarshal([]byte(doc), &refs); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if refs[0].Kind != RefKindGroundedIn {
				t.Errorf("legacy kind %q resolved to %q, want grounded-in", kind, refs[0].Kind)
			}
			if got := refs[0].OnDiskKind(); got != RefKind(kind) {
				t.Errorf("OnDiskKind() = %q, want raw on-disk %q", got, kind)
			}
		})
	}
}

func TestRef_OnDiskKind_FallsBackToKind(t *testing.T) {
	// Non-alias parsed refs and in-memory-constructed refs leave onDiskKind
	// empty, so OnDiskKind falls back to Kind. This keeps a parsed non-alias ref
	// byte-equal (==) to its constructed-literal form.
	const doc = "- id: 20260101-000000-s-cpt-aaa\n  kind: addresses\n"
	var refs []Ref
	if err := yaml.Unmarshal([]byte(doc), &refs); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if got := refs[0].OnDiskKind(); got != RefKindAddresses {
		t.Errorf("parsed non-alias OnDiskKind() = %q, want addresses", got)
	}
	want := Ref{ID: "20260101-000000-s-cpt-aaa", Kind: RefKindAddresses}
	if refs[0] != want {
		t.Errorf("parsed non-alias ref %+v must stay == to its literal form %+v", refs[0], want)
	}

	inMem := Ref{ID: "x", Kind: RefKindRelated}
	if got := inMem.OnDiskKind(); got != RefKindRelated {
		t.Errorf("in-memory OnDiskKind() = %q, want related", got)
	}
}

func TestRefUnmarshal_MissingKind_Rejected(t *testing.T) {
	const doc = `- id: 20260101-000000-s-cpt-aaa
`
	var refs []Ref
	err := yaml.Unmarshal([]byte(doc), &refs)
	if err == nil {
		t.Fatal("want error for object form without kind, got nil")
	}
	if !strings.Contains(err.Error(), "missing required `kind`") {
		t.Errorf("error = %q, want mention of missing kind", err.Error())
	}
}

func TestRefUnmarshal_MissingID_Rejected(t *testing.T) {
	const doc = `- kind: grounded-in
`
	var refs []Ref
	err := yaml.Unmarshal([]byte(doc), &refs)
	if err == nil {
		t.Fatal("want error for object form without id, got nil")
	}
	if !strings.Contains(err.Error(), "missing required `id`") {
		t.Errorf("error = %q, want mention of missing id", err.Error())
	}
}

func TestRefUnmarshal_EmptyKind_Rejected(t *testing.T) {
	const doc = `- id: 20260101-000000-s-cpt-aaa
  kind: ""
`
	var refs []Ref
	err := yaml.Unmarshal([]byte(doc), &refs)
	if err == nil {
		t.Fatal("want error for empty kind, got nil")
	}
	if !strings.Contains(err.Error(), "kind") {
		t.Errorf("error = %q, want mention of kind", err.Error())
	}
}

func TestRefUnmarshal_InvalidKind_Rejected(t *testing.T) {
	const doc = `- id: 20260101-000000-s-cpt-aaa
  kind: bogus
`
	var refs []Ref
	err := yaml.Unmarshal([]byte(doc), &refs)
	if err == nil {
		t.Fatal("want error for invalid kind, got nil")
	}
	if !strings.Contains(err.Error(), "invalid kind") {
		t.Errorf("error = %q, want mention of invalid kind", err.Error())
	}
}

func TestRefUnmarshal_EmptyBareString_Rejected(t *testing.T) {
	const doc = `- ""
`
	var refs []Ref
	err := yaml.Unmarshal([]byte(doc), &refs)
	if err == nil {
		t.Fatal("want error for empty string ref, got nil")
	}
}

func TestRefUnmarshal_ClosedSetCoverage(t *testing.T) {
	cases := []struct {
		kind string
		want RefKind
	}{
		{"grounded-in", RefKindGroundedIn},
		{"builds-on", RefKindBuildsOn},
		{"refines", RefKindRefines},
		{"addresses", RefKindAddresses},
		{"surfaces", RefKindSurfaces},
		{"depends-on", RefKindDependsOn},
		{"required-by", RefKindRequiredBy},
		{"related", RefKindRelated},
		{"unknown", RefKindUnknown}, // object-form unknown allowed at parse time for legacy round-trip
	}
	for _, c := range cases {
		t.Run(c.kind, func(t *testing.T) {
			doc := "- id: 20260101-000000-s-cpt-aaa\n  kind: " + c.kind + "\n"
			var refs []Ref
			if err := yaml.Unmarshal([]byte(doc), &refs); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}
			if refs[0].Kind != c.want {
				t.Errorf("Kind = %q, want %q", refs[0].Kind, c.want)
			}
		})
	}
}

func TestRefMarshal_ObjectFormWithOrder(t *testing.T) {
	r := Ref{ID: "20260101-000000-s-cpt-aaa", Kind: RefKindAddresses, Desc: "fix"}
	out, err := yaml.Marshal([]Ref{r})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)
	wantSubstrings := []string{
		"id: 20260101-000000-s-cpt-aaa",
		"kind: addresses",
		"desc: fix",
	}
	for _, sub := range wantSubstrings {
		if !strings.Contains(got, sub) {
			t.Errorf("output missing %q\n%s", sub, got)
		}
	}
	// id must appear before kind, which must appear before desc.
	idAt := strings.Index(got, "id:")
	kindAt := strings.Index(got, "kind:")
	descAt := strings.Index(got, "desc:")
	if idAt >= kindAt || kindAt >= descAt {
		t.Errorf("expected id < kind < desc ordering\n%s", got)
	}
}

func TestRefMarshal_OmitsDescWhenEmpty(t *testing.T) {
	r := Ref{ID: "20260101-000000-s-cpt-aaa", Kind: RefKindGroundedIn}
	out, err := yaml.Marshal([]Ref{r})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)
	if strings.Contains(got, "desc:") {
		t.Errorf("expected no desc field when empty\n%s", got)
	}
}

func TestRefMarshal_LegacyUnknownStillEmitsObjectForm(t *testing.T) {
	// Round-tripping a legacy entry through the writer must produce object
	// form — bare-string output is never emitted, even for kind: unknown.
	r := Ref{ID: "20260101-000000-s-cpt-aaa", Kind: RefKindUnknown}
	out, err := yaml.Marshal([]Ref{r})
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	got := string(out)
	if !strings.Contains(got, "kind: unknown") {
		t.Errorf("expected kind: unknown to round-trip\n%s", got)
	}
	if !strings.Contains(got, "id:") {
		t.Errorf("expected object form, not bare-string\n%s", got)
	}
}

func TestIsValidRefKind(t *testing.T) {
	for _, k := range []RefKind{
		RefKindGroundedIn, RefKindBuildsOn, RefKindRefines, RefKindAddresses,
		RefKindSurfaces, RefKindDependsOn, RefKindRequiredBy, RefKindRelated,
		RefKindUnknown,
	} {
		if !IsValidRefKind(k) {
			t.Errorf("%q should be valid", k)
		}
	}
	// Legacy alias kinds resolve to their canonical form at parse time, so the
	// raw values are not themselves valid in memory.
	for _, k := range []RefKind{RefKindGrounds, RefKindEvidence} {
		if IsValidRefKind(k) {
			t.Errorf("legacy alias %q should not be valid in memory (resolves to grounded-in at parse)", k)
		}
	}
	if IsValidRefKind(RefKind("bogus")) {
		t.Error("bogus should be invalid")
	}
}

func TestIsCapturableRefKind(t *testing.T) {
	for _, k := range []RefKind{
		RefKindGroundedIn, RefKindBuildsOn, RefKindRefines, RefKindAddresses,
		RefKindSurfaces, RefKindDependsOn, RefKindRequiredBy, RefKindRelated,
	} {
		if !IsCapturableRefKind(k) {
			t.Errorf("%q should be capturable", k)
		}
	}
	if IsCapturableRefKind(RefKindUnknown) {
		t.Error("unknown should not be capturable at new-entry time")
	}
	// Legacy alias kinds are rejected for new captures (AC 1) — only their
	// canonical replacement grounded-in is capturable.
	for _, k := range []RefKind{RefKindGrounds, RefKindEvidence} {
		if IsCapturableRefKind(k) {
			t.Errorf("legacy alias %q must not be capturable for new entries", k)
		}
	}
	if IsCapturableRefKind(RefKind("bogus")) {
		t.Error("bogus should not be capturable")
	}
}

func TestRefIDs(t *testing.T) {
	refs := []Ref{
		{ID: "a", Kind: RefKindGroundedIn},
		{ID: "b", Kind: RefKindAddresses},
	}
	got := RefIDs(refs)
	if len(got) != 2 || got[0] != "a" || got[1] != "b" {
		t.Errorf("RefIDs = %v, want [a b]", got)
	}
}

func TestIDOnlyList_RejectsObjectForm(t *testing.T) {
	// closes and supersedes use idOnlyList — they don't support object form.
	const doc = `closes:
  - id: 20260101-000000-s-cpt-aaa
    kind: addresses
`
	var fm struct {
		Closes idOnlyList `yaml:"closes"`
	}
	err := yaml.Unmarshal([]byte(doc), &fm)
	if err == nil {
		t.Fatal("want error for object form on closes, got nil")
	}
	if !strings.Contains(err.Error(), "object form not supported") {
		t.Errorf("error = %q, want clear rejection mentioning object form", err.Error())
	}
}

func TestIDOnlyList_BareStringsAccepted(t *testing.T) {
	const doc = `closes:
  - 20260101-000000-s-cpt-aaa
  - 20260102-000000-s-cpt-bbb
`
	var fm struct {
		Closes idOnlyList `yaml:"closes"`
	}
	if err := yaml.Unmarshal([]byte(doc), &fm); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if len(fm.Closes) != 2 || fm.Closes[0] != "20260101-000000-s-cpt-aaa" {
		t.Errorf("Closes = %v", fm.Closes)
	}
}

// TestEntryFrontmatter_RefsRoundTrip exercises the full pipeline: parse an
// entry with mixed bare-string and object-form refs, then write it back —
// the output must use object form for every ref (including legacy ones,
// which round-trip as kind: unknown).
func TestEntryFrontmatter_RefsRoundTrip(t *testing.T) {
	const onDisk = `---
type: decision
layer: tactical
refs:
  - 20260101-000000-s-cpt-aaa
  - id: 20260102-000000-s-cpt-bbb
    kind: addresses
    desc: resolves the gap
---
body text
`
	e, err := ParseEntry("20260103-000000-d-tac-zzz.md", onDisk)
	if err != nil {
		t.Fatalf("ParseEntry: %v", err)
	}
	if len(e.Refs) != 2 {
		t.Fatalf("len(Refs) = %d, want 2", len(e.Refs))
	}
	if e.Refs[0].Kind != RefKindUnknown {
		t.Errorf("first ref (bare) Kind = %q, want unknown", e.Refs[0].Kind)
	}
	if e.Refs[1].Kind != RefKindAddresses || e.Refs[1].Desc != "resolves the gap" {
		t.Errorf("second ref = %+v", e.Refs[1])
	}

	out := FormatFrontmatter(e)
	if strings.Contains(out, "refs:\n  - 20260101") {
		t.Errorf("writer must not emit bare-string refs even for legacy kind: unknown\n%s", out)
	}
	if !strings.Contains(out, "kind: unknown") {
		t.Errorf("legacy ref should round-trip as kind: unknown\n%s", out)
	}
	if !strings.Contains(out, "kind: addresses") {
		t.Errorf("object-form ref kind missing in output\n%s", out)
	}
}

// TestEntryFrontmatter_ClosesObjectFormRejected verifies that object form on
// closes (or supersedes) parses to a clear error pointing at refs.
func TestEntryFrontmatter_ClosesObjectFormRejected(t *testing.T) {
	const onDisk = `---
type: decision
layer: tactical
closes:
  - id: 20260101-000000-s-cpt-aaa
    kind: addresses
---
body text
`
	_, err := ParseEntry("20260103-000000-d-tac-zzz.md", onDisk)
	if err == nil {
		t.Fatal("want error for object form on closes, got nil")
	}
	if !strings.Contains(err.Error(), "object form not supported") {
		t.Errorf("error = %q, want clear rejection mentioning object form", err.Error())
	}
}
