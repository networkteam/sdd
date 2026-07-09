package model

import (
	"fmt"
	"strings"
	"testing"
)

const otherRepo = "example.com/team/other"

// multiFixture assembles a local graph with a cross-repo ref into a member
// graph. The member holds a chain (rem ← base) where base is an embedded
// entry also present in the local graph, plus a superseded entry with a
// successor.
func multiFixture(t *testing.T) (*Graph, *Graph) {
	t.Helper()

	baseLocal := entry("20260301-090000-s-cpt-bas")
	baseLocal.Embedded = true
	local := NewGraph([]*Entry{
		baseLocal,
		entry("20260410-100200-d-tac-ccc"),
	})
	local.ByID["20260410-100200-d-tac-ccc"].Refs = []Ref{
		{ID: otherRepo + ":20260401-090000-d-cpt-rem", Kind: RefKindGroundedIn, Desc: "remote basis"},
	}

	baseMember := entry("20260301-090000-s-cpt-bas")
	baseMember.Embedded = true
	rem := entry("20260401-090000-d-cpt-rem", withSummary("Remote directive."))
	rem.Refs = []Ref{{ID: "20260301-090000-s-cpt-bas", Kind: RefKindGroundedIn}}
	old := entry("20260331-080000-d-cpt-old")
	succ := entry("20260401-100000-d-cpt-suc", withSupersedes("20260331-080000-d-cpt-old"))
	member := NewGraph([]*Entry{baseMember, rem, old, succ})

	NewMultiGraph(local, []string{otherRepo}, func(repoID string) (*Graph, error) {
		if repoID == otherRepo {
			return member, nil
		}
		return nil, nil
	})
	return local, member
}

func TestMultiGraph_ResolveAcross(t *testing.T) {
	local, member := multiFixture(t)

	e, owner, ok := local.ResolveAcross(otherRepo + ":20260401-090000-d-cpt-rem")
	if !ok || owner != member || e.ID != "20260401-090000-d-cpt-rem" {
		t.Fatalf("cross-repo resolution failed: ok=%v owner=%p e=%+v", ok, owner, e)
	}

	// Bare IDs resolve in the graph at hand.
	if _, owner, ok := local.ResolveAcross("20260410-100200-d-tac-ccc"); !ok || owner != local {
		t.Error("bare ID must resolve locally")
	}
	// Unknown repo and missing entry do not resolve.
	if _, _, ok := local.ResolveAcross("example.com/team/unknown:20260401-090000-d-cpt-rem"); ok {
		t.Error("unknown repo must not resolve")
	}
	if _, _, ok := local.ResolveAcross(otherRepo + ":20260401-090000-d-cpt-gon"); ok {
		t.Error("missing remote entry must not resolve")
	}
}

func TestResolveUnionID(t *testing.T) {
	local, _ := multiFixture(t)

	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"local short id", "d-tac-ccc", "20260410-100200-d-tac-ccc"},
		{"local full id", "20260410-100200-d-tac-ccc", "20260410-100200-d-tac-ccc"},
		{"foreign short id expands to prefixed", "d-cpt-rem", otherRepo + ":20260401-090000-d-cpt-rem"},
		{"foreign unprefixed-full expands to prefixed", "20260401-090000-d-cpt-rem", otherRepo + ":20260401-090000-d-cpt-rem"},
		// The embedded base entry is present in both local and the member;
		// it must dedup to the single local instance (bare), not collide.
		{"embedded entry dedups to local", "s-cpt-bas", "20260301-090000-s-cpt-bas"},
		// Already-prefixed cross-repo IDs pass through verbatim.
		{"cross-repo prefixed passthrough", otherRepo + ":20260401-090000-d-cpt-rem", otherRepo + ":20260401-090000-d-cpt-rem"},
		// Unrecognized / unmatched inputs pass through so the caller's
		// "not found" surface fires against the original text.
		{"unknown short id passthrough", "d-tac-zzz", "d-tac-zzz"},
		{"empty passthrough", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := local.ResolveUnionID(tc.input)
			if err != nil {
				t.Fatalf("ResolveUnionID(%q): %v", tc.input, err)
			}
			if got != tc.want {
				t.Errorf("ResolveUnionID(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// A short ID matching genuinely distinct entries in the local graph and a
// dependency is a real ambiguity: resolution refuses (no local-first
// precedence) and lists the candidates, the local one bare and the foreign one
// in full prefixed form.
func TestResolveUnionID_Ambiguity(t *testing.T) {
	local := NewGraph([]*Entry{entry("20260101-100000-d-tac-dup")})
	member := NewGraph([]*Entry{entry("20260202-200000-d-tac-dup")})
	NewMultiGraph(local, []string{otherRepo}, func(repoID string) (*Graph, error) {
		if repoID == otherRepo {
			return member, nil
		}
		return nil, nil
	})

	_, err := local.ResolveUnionID("d-tac-dup")
	if err == nil {
		t.Fatal("expected ambiguity error for a short ID matching local and a dependency")
	}
	if !strings.Contains(err.Error(), "20260101-100000-d-tac-dup") ||
		!strings.Contains(err.Error(), otherRepo+":20260202-200000-d-tac-dup") {
		t.Errorf("ambiguity error should list both candidates (local bare, foreign prefixed), got: %v", err)
	}
}

func TestMultiGraph_MemberLoadErrorPropagates(t *testing.T) {
	local := NewGraph([]*Entry{entry("20260410-100200-d-tac-ccc")})
	NewMultiGraph(local, []string{"example.com/team/broken"}, func(repoID string) (*Graph, error) {
		return nil, fmt.Errorf("corrupt cache")
	})
	if _, err := local.MemberGraph("example.com/team/broken"); err == nil {
		t.Error("member load error must propagate")
	}
	// ResolveAcross degrades an errored member to unresolved.
	if _, _, ok := local.ResolveAcross("example.com/team/broken:20260401-090000-d-cpt-rem"); ok {
		t.Error("errored member must not resolve")
	}
}

func TestBuildShowTree_CrossRepoResolvedChain(t *testing.T) {
	local, _ := multiFixture(t)

	tree := local.BuildShowTree("20260410-100200-d-tac-ccc", 4, 0, make(map[string]bool), make(map[string]bool))
	if len(tree.Upstream) != 2 {
		t.Fatalf("Upstream = %d items, want 2 (remote node + its embedded basis): %+v", len(tree.Upstream), tree.Upstream)
	}

	remote := tree.Upstream[0]
	if remote.Entry == nil || remote.Entry.ID != "20260401-090000-d-cpt-rem" {
		t.Fatalf("remote node not resolved: %+v", remote)
	}
	if remote.NodeID() != otherRepo+":20260401-090000-d-cpt-rem" {
		t.Errorf("remote NodeID = %q, want repo-prefixed form", remote.NodeID())
	}
	if remote.Status.Kind != StatusActive {
		t.Errorf("remote status = %+v, want active (derived in member graph)", remote.Status)
	}

	// The chain continues inside the member graph; the embedded basis
	// surfaces with its bare ID (binary-scoped, never repo-prefixed).
	embedded := tree.Upstream[1]
	if embedded.Entry == nil || embedded.NodeID() != "20260301-090000-s-cpt-bas" {
		t.Fatalf("embedded basis node = %+v, want bare-ID embedded entry", embedded)
	}
	if embedded.Depth != 2 {
		t.Errorf("embedded basis depth = %d, want 2", embedded.Depth)
	}
}

func TestBuildShowTree_CrossRepoPrimary(t *testing.T) {
	local, _ := multiFixture(t)

	tree := local.BuildShowTree(otherRepo+":20260401-090000-d-cpt-rem", 4, 4, make(map[string]bool), make(map[string]bool))
	if tree == nil {
		t.Fatal("cross-repo primary did not resolve")
	}
	if tree.PrimaryID != otherRepo+":20260401-090000-d-cpt-rem" {
		t.Errorf("PrimaryID = %q, want repo-prefixed form", tree.PrimaryID)
	}
	if len(tree.Upstream) != 1 || tree.Upstream[0].NodeID() != "20260301-090000-s-cpt-bas" {
		t.Errorf("remote primary upstream = %+v, want its embedded basis", tree.Upstream)
	}
}

func TestBuildShowTree_CrossRepoSupersedeTrailQualified(t *testing.T) {
	local, _ := multiFixture(t)
	local.ByID["20260410-100200-d-tac-ccc"].Refs = []Ref{
		{ID: otherRepo + ":20260331-080000-d-cpt-old", Kind: RefKindGroundedIn},
	}

	tree := local.BuildShowTree("20260410-100200-d-tac-ccc", 2, 0, make(map[string]bool), make(map[string]bool))
	if len(tree.Upstream) == 0 {
		t.Fatal("no upstream")
	}
	node := tree.Upstream[0]
	if node.Status.Kind != StatusSupersededBy {
		t.Fatalf("status = %+v, want superseded-by", node.Status)
	}
	if node.Status.By != otherRepo+":20260401-100000-d-cpt-suc" {
		t.Errorf("status.By = %q, want repo-qualified successor", node.Status.By)
	}
	if len(node.SupersedePath) != 2 || node.SupersedePath[1] != otherRepo+":20260401-100000-d-cpt-suc" {
		t.Errorf("SupersedePath = %v, want repo-qualified trail", node.SupersedePath)
	}
}

func TestBuildShowTree_CrossRepoDedupByRepoAndEntry(t *testing.T) {
	local, _ := multiFixture(t)
	// Two local entries both ref the same remote entry: the second render
	// dedups on the (repo, entry) key.
	second := entry("20260410-110000-d-tac-ddd")
	second.Refs = []Ref{{ID: otherRepo + ":20260401-090000-d-cpt-rem", Kind: RefKindRelated}}
	local.Entries = append(local.Entries, second)
	local.ByID[second.ID] = second

	rendered := make(map[string]bool)
	primaries := map[string]bool{"20260410-100200-d-tac-ccc": true, second.ID: true}

	first := local.BuildShowTree("20260410-100200-d-tac-ccc", 4, 0, rendered, primaries)
	if first.Upstream[0].ShownAbove {
		t.Fatal("first render must not dedup")
	}
	dup := local.BuildShowTree(second.ID, 4, 0, rendered, primaries)
	if len(dup.Upstream) == 0 || !dup.Upstream[0].ShownAbove {
		t.Errorf("second render must mark the remote node shown-above: %+v", dup.Upstream)
	}
}
