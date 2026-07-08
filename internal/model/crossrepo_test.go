package model

import (
	"strings"
	"testing"
	"time"
)

func TestSplitCrossRepoID(t *testing.T) {
	tests := []struct {
		in          string
		repoID      string
		entryID     string
		isCrossRepo bool
	}{
		{"github.com/networkteam/other:20260601-120000-s-tac-abc", "github.com/networkteam/other", "20260601-120000-s-tac-abc", true},
		{"20260601-120000-s-tac-abc", "", "", false},
		{"s-tac-abc", "", "", false},
		{"", "", "", false},
		// The separator is the first colon; anything after passes through.
		{"host.com/a:b:c", "host.com/a", "b:c", true},
	}
	for _, tt := range tests {
		repoID, entryID, ok := SplitCrossRepoID(tt.in)
		if repoID != tt.repoID || entryID != tt.entryID || ok != tt.isCrossRepo {
			t.Errorf("SplitCrossRepoID(%q) = (%q, %q, %v), want (%q, %q, %v)",
				tt.in, repoID, entryID, ok, tt.repoID, tt.entryID, tt.isCrossRepo)
		}
	}
}

func TestValidateRepoID(t *testing.T) {
	valid := []string{
		"github.com/networkteam/sdd",
		"gitlab.example.org/group/subgroup/repo",
		"host.com/repo",
	}
	for _, id := range valid {
		if err := ValidateRepoID(id); err != nil {
			t.Errorf("ValidateRepoID(%q): unexpected error: %v", id, err)
		}
	}
	invalid := []string{
		"",
		"norepo",              // no path
		"github.com",          // host only
		"nohost/repo",         // first segment not dotted
		"github.com//repo",    // empty segment
		"github.com/re po",    // invalid char
		"github.com/repo/",    // trailing empty segment
		"git@github.com/repo", // invalid char (@)
	}
	for _, id := range invalid {
		if err := ValidateRepoID(id); err == nil {
			t.Errorf("ValidateRepoID(%q): want error, got nil", id)
		}
	}
}

func TestDeriveRepoID(t *testing.T) {
	tests := []struct {
		in   string
		want string
	}{
		// ssh and https forms of the same remote normalize equal.
		{"git@github.com:networkteam/sdd.git", "github.com/networkteam/sdd"},
		{"https://github.com/networkteam/sdd.git", "github.com/networkteam/sdd"},
		{"https://github.com/networkteam/sdd", "github.com/networkteam/sdd"},
		{"ssh://git@github.com/networkteam/sdd.git", "github.com/networkteam/sdd"},
		{"HTTPS://GitHub.com/networkteam/sdd", "github.com/networkteam/sdd"},
		{"ssh://git@gitlab.example.org:2222/group/sub/repo.git", "gitlab.example.org/group/sub/repo"},
		{"https://github.com/networkteam/sdd/", "github.com/networkteam/sdd"},
	}
	for _, tt := range tests {
		got, err := DeriveRepoID(tt.in)
		if err != nil {
			t.Errorf("DeriveRepoID(%q): unexpected error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("DeriveRepoID(%q) = %q, want %q", tt.in, got, tt.want)
		}
	}
	for _, in := range []string{"", "not-a-url", "/local/path/only", "file:///local/repo"} {
		if got, err := DeriveRepoID(in); err == nil {
			t.Errorf("DeriveRepoID(%q) = %q, want error", in, got)
		}
	}
}

func TestValidateCrossRepoID(t *testing.T) {
	if err := ValidateCrossRepoID("github.com/networkteam/other:20260601-120000-s-tac-abc"); err != nil {
		t.Errorf("valid cross-repo ID rejected: %v", err)
	}
	invalid := []string{
		"20260601-120000-s-tac-abc",                    // no colon
		"nohost:20260601-120000-s-tac-abc",             // bad repo id
		"github.com/networkteam/other:s-tac-abc",       // short entry ID
		"github.com/networkteam/other:not-an-entry-id", // malformed entry ID
	}
	for _, id := range invalid {
		if err := ValidateCrossRepoID(id); err == nil {
			t.Errorf("ValidateCrossRepoID(%q): want error, got nil", id)
		}
	}
}

func TestIsForwardClassRefKind(t *testing.T) {
	forward := []RefKind{RefKindSurfaces, RefKindRequiredBy}
	for _, k := range forward {
		if !IsForwardClassRefKind(k) {
			t.Errorf("IsForwardClassRefKind(%q) = false, want true", k)
		}
	}
	backward := []RefKind{RefKindGroundedIn, RefKindBuildsOn, RefKindRefines,
		RefKindAddresses, RefKindSurfacedBy, RefKindDependsOn, RefKindRelated, RefKindUnknown}
	for _, k := range backward {
		if IsForwardClassRefKind(k) {
			t.Errorf("IsForwardClassRefKind(%q) = true, want false", k)
		}
	}
}

func crossRepoTestEntry(id string, refs []Ref, closes, supersedes []string) *Entry {
	return &Entry{
		ID:         id,
		Type:       TypeSignal,
		Kind:       KindGap,
		Layer:      LayerTactical,
		Time:       time.Date(2026, 6, 1, 12, 0, 0, 0, time.UTC),
		Refs:       refs,
		Closes:     closes,
		Supersedes: supersedes,
	}
}

func warningsContaining(warnings []Warning, substr string) []Warning {
	var out []Warning
	for _, w := range warnings {
		if strings.Contains(w.Message, substr) {
			out = append(out, w)
		}
	}
	return out
}

func TestCrossRepoRefNotDanglingNotIndexed(t *testing.T) {
	crossID := "github.com/networkteam/other:20260601-120000-s-tac-abc"
	e := crossRepoTestEntry("20260602-130000-s-tac-src", []Ref{{ID: crossID, Kind: RefKindGroundedIn}}, nil, nil)
	g := NewGraph([]*Entry{e})

	if warns := warningsContaining(e.Warnings, "dangling"); len(warns) > 0 {
		t.Errorf("cross-repo ref flagged dangling: %v", warns)
	}
	if warns := warningsContaining(e.Warnings, "malformed"); len(warns) > 0 {
		t.Errorf("well-formed cross-repo ref flagged malformed: %v", warns)
	}
	if refs := g.RefsTo[crossID]; len(refs) > 0 {
		t.Errorf("cross-repo ref indexed in RefsTo: %v", refs)
	}
}

func TestMalformedCrossRepoRefWarns(t *testing.T) {
	e := crossRepoTestEntry("20260602-130000-s-tac-src", []Ref{{ID: "nohost:20260601-120000-s-tac-abc", Kind: RefKindGroundedIn}}, nil, nil)
	NewGraph([]*Entry{e})

	if warns := warningsContaining(e.Warnings, "malformed cross-repo ref"); len(warns) != 1 {
		t.Errorf("want 1 malformed cross-repo warning, got warnings: %v", e.Warnings)
	}
}

func TestCrossRepoLifecycleEdgesWarn(t *testing.T) {
	crossID := "github.com/networkteam/other:20260601-120000-s-tac-abc"
	e := crossRepoTestEntry("20260602-130000-s-tac-src", nil, []string{crossID}, []string{crossID})
	g := NewGraph([]*Entry{e})

	if warns := warningsContaining(e.Warnings, "lifecycle edges never cross"); len(warns) != 2 {
		t.Errorf("want lifecycle warnings for closes and supersedes, got: %v", e.Warnings)
	}
	if ids := g.ClosedBy[crossID]; len(ids) > 0 {
		t.Errorf("cross-repo close indexed in ClosedBy: %v", ids)
	}
	if ids := g.SupersededBy[crossID]; len(ids) > 0 {
		t.Errorf("cross-repo supersede indexed in SupersededBy: %v", ids)
	}
}

func TestForwardClassRefsExemptFromDangling(t *testing.T) {
	missing := "20260601-120000-s-tac-gon"
	e := crossRepoTestEntry("20260602-130000-s-tac-src", []Ref{
		{ID: missing, Kind: RefKindSurfaces},
		{ID: missing, Kind: RefKindRequiredBy},
	}, nil, nil)
	NewGraph([]*Entry{e})

	if warns := warningsContaining(e.Warnings, "dangling"); len(warns) > 0 {
		t.Errorf("forward-class refs flagged dangling: %v", warns)
	}

	// A backward-class ref to the same missing target still warns.
	e2 := crossRepoTestEntry("20260602-130001-s-tac-sr2", []Ref{{ID: missing, Kind: RefKindGroundedIn}}, nil, nil)
	NewGraph([]*Entry{e2})
	if warns := warningsContaining(e2.Warnings, "dangling"); len(warns) != 1 {
		t.Errorf("backward-class dangling ref not flagged, warnings: %v", e2.Warnings)
	}
}

func TestResolveIDPassesCrossRepoThrough(t *testing.T) {
	e := crossRepoTestEntry("20260602-130000-s-tac-src", nil, nil, nil)
	g := NewGraph([]*Entry{e})

	// Even a colon form whose repo-id segments could look like a short ID
	// must pass through verbatim.
	inputs := []string{
		"github.com/networkteam/other:20260601-120000-s-tac-abc",
		"github.com/networkteam/other:s-tac-src",
	}
	for _, in := range inputs {
		got, err := g.ResolveID(in)
		if err != nil {
			t.Errorf("ResolveID(%q): unexpected error: %v", in, err)
			continue
		}
		if got != in {
			t.Errorf("ResolveID(%q) = %q, want verbatim pass-through", in, got)
		}
	}
}

func TestResolveRefIDsPreservesCrossRepoRefs(t *testing.T) {
	e := crossRepoTestEntry("20260602-130000-s-tac-src", nil, nil, nil)
	g := NewGraph([]*Entry{e})

	crossID := "github.com/networkteam/other:20260601-120000-s-tac-abc"
	refs, err := g.ResolveRefIDs([]Ref{{ID: crossID, Kind: RefKindGroundedIn, Desc: "remote basis"}})
	if err != nil {
		t.Fatalf("ResolveRefIDs: %v", err)
	}
	if refs[0].ID != crossID || refs[0].Kind != RefKindGroundedIn || refs[0].Desc != "remote basis" {
		t.Errorf("cross-repo ref mutated in resolution: %+v", refs[0])
	}
}
