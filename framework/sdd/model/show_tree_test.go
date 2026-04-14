package model

import (
	"testing"
)

func TestBuildShowTree_NoRefs(t *testing.T) {
	e := entry("20260410-100000-s-tac-aaa")
	g := NewGraph([]*Entry{e})

	tree := g.BuildShowTree(e.ID, 4, false, make(map[string]bool), make(map[string]bool))

	if tree.Primary.ID != e.ID {
		t.Errorf("Primary = %q, want %q", tree.Primary.ID, e.ID)
	}
	if len(tree.Upstream) != 0 {
		t.Errorf("Upstream = %d items, want 0", len(tree.Upstream))
	}
	if len(tree.Downstream) != 0 {
		t.Errorf("Downstream = %d items, want 0", len(tree.Downstream))
	}
}

func TestBuildShowTree_UpstreamChain(t *testing.T) {
	a := entry("20260410-100000-s-stg-aaa", withSummary("Root"))
	b := entry("20260410-100100-s-cpt-bbb", withSummary("Middle"), withRefs("20260410-100000-s-stg-aaa"))
	c := entry("20260410-100200-d-tac-ccc", withRefs("20260410-100100-s-cpt-bbb"))

	g := NewGraph([]*Entry{a, b, c})
	tree := g.BuildShowTree(c.ID, 4, false, make(map[string]bool), make(map[string]bool))

	if len(tree.Upstream) != 2 {
		t.Fatalf("Upstream = %d items, want 2", len(tree.Upstream))
	}

	// b at depth 1.
	if tree.Upstream[0].Entry.ID != b.ID {
		t.Errorf("Upstream[0].ID = %q, want %q", tree.Upstream[0].Entry.ID, b.ID)
	}
	if tree.Upstream[0].Depth != 1 {
		t.Errorf("Upstream[0].Depth = %d, want 1", tree.Upstream[0].Depth)
	}
	if !tree.Upstream[0].SummaryOnly {
		t.Error("Upstream[0].SummaryOnly = false, want true")
	}
	assertRelations(t, tree.Upstream[0], "refs")

	// a at depth 2.
	if tree.Upstream[1].Entry.ID != a.ID {
		t.Errorf("Upstream[1].ID = %q, want %q", tree.Upstream[1].Entry.ID, a.ID)
	}
	if tree.Upstream[1].Depth != 2 {
		t.Errorf("Upstream[1].Depth = %d, want 2", tree.Upstream[1].Depth)
	}
}

func TestBuildShowTree_MaxDepthTruncation(t *testing.T) {
	root := entry("20260410-100000-s-stg-aaa", withSummary("Root"))
	a := entry("20260410-100100-s-cpt-bbb", withSummary("A"), withRefs("20260410-100000-s-stg-aaa"))
	b := entry("20260410-100200-s-tac-ccc", withSummary("B"), withRefs("20260410-100100-s-cpt-bbb"))
	primary := entry("20260410-100300-d-tac-ddd", withRefs("20260410-100200-s-tac-ccc"))

	g := NewGraph([]*Entry{root, a, b, primary})
	tree := g.BuildShowTree(primary.ID, 2, false, make(map[string]bool), make(map[string]bool))

	if len(tree.Upstream) != 2 {
		t.Fatalf("Upstream = %d items, want 2 (b + a)", len(tree.Upstream))
	}
	if tree.Upstream[1].Entry.ID != a.ID {
		t.Errorf("Upstream[1].ID = %q, want %q", tree.Upstream[1].Entry.ID, a.ID)
	}
	if len(tree.Upstream[1].Truncated) != 1 {
		t.Errorf("Truncated = %d, want 1", len(tree.Upstream[1].Truncated))
	}
}

func TestBuildShowTree_MaxDepth1(t *testing.T) {
	a := entry("20260410-100000-s-stg-aaa", withSummary("Root"))
	b := entry("20260410-100100-s-cpt-bbb", withSummary("Middle"), withRefs("20260410-100000-s-stg-aaa"))
	c := entry("20260410-100200-d-tac-ccc", withRefs("20260410-100100-s-cpt-bbb"))

	g := NewGraph([]*Entry{a, b, c})
	tree := g.BuildShowTree(c.ID, 1, false, make(map[string]bool), make(map[string]bool))

	if len(tree.Upstream) != 1 {
		t.Fatalf("Upstream = %d items, want 1", len(tree.Upstream))
	}
	if len(tree.Upstream[0].Truncated) != 1 {
		t.Errorf("Truncated = %d, want 1", len(tree.Upstream[0].Truncated))
	}
}

func TestBuildShowTree_CombinedRelations(t *testing.T) {
	a := entry("20260410-100000-s-tac-aaa", withSummary("Signal"))
	b := entry("20260410-100100-a-tac-bbb",
		withRefs("20260410-100000-s-tac-aaa"),
		withCloses("20260410-100000-s-tac-aaa"))

	g := NewGraph([]*Entry{a, b})
	tree := g.BuildShowTree(b.ID, 4, false, make(map[string]bool), make(map[string]bool))

	if len(tree.Upstream) != 1 {
		t.Fatalf("Upstream = %d items, want 1", len(tree.Upstream))
	}
	assertRelations(t, tree.Upstream[0], "refs", "closes")
}

func TestBuildShowTree_BranchingDedup(t *testing.T) {
	shared := entry("20260410-100000-s-stg-aaa", withSummary("Shared"))
	b := entry("20260410-100100-s-cpt-bbb", withSummary("B"), withRefs("20260410-100000-s-stg-aaa"))
	c := entry("20260410-100200-s-cpt-ccc", withSummary("C"), withRefs("20260410-100000-s-stg-aaa"))
	primary := entry("20260410-100300-d-tac-ddd",
		withRefs("20260410-100100-s-cpt-bbb", "20260410-100200-s-cpt-ccc"))

	g := NewGraph([]*Entry{shared, b, c, primary})
	tree := g.BuildShowTree(primary.ID, 4, false, make(map[string]bool), make(map[string]bool))

	// Expected: b(1), shared(2), c(1), shared(2-see above).
	if len(tree.Upstream) != 4 {
		t.Fatalf("Upstream = %d items, want 4", len(tree.Upstream))
	}

	if tree.Upstream[1].Entry.ID != shared.ID {
		t.Errorf("Upstream[1].ID = %q, want shared", tree.Upstream[1].Entry.ID)
	}
	if tree.Upstream[1].ShownAbove {
		t.Error("First occurrence of shared should not be ShownAbove")
	}

	if tree.Upstream[3].Entry.ID != shared.ID {
		t.Errorf("Upstream[3].ID = %q, want shared", tree.Upstream[3].Entry.ID)
	}
	if !tree.Upstream[3].ShownAbove {
		t.Error("Second occurrence of shared should be ShownAbove")
	}
}

func TestBuildShowTree_CrossGroupDedup(t *testing.T) {
	shared := entry("20260410-100000-s-stg-aaa", withSummary("Shared"))
	first := entry("20260410-100100-d-cpt-bbb", withRefs("20260410-100000-s-stg-aaa"))
	second := entry("20260410-100200-d-cpt-ccc", withRefs("20260410-100000-s-stg-aaa"))

	g := NewGraph([]*Entry{shared, first, second})
	rendered := make(map[string]bool)
	primaries := map[string]bool{first.ID: true, second.ID: true}

	tree1 := g.BuildShowTree(first.ID, 4, false, rendered, primaries)
	tree2 := g.BuildShowTree(second.ID, 4, false, rendered, primaries)

	if len(tree1.Upstream) != 1 {
		t.Fatalf("tree1.Upstream = %d, want 1", len(tree1.Upstream))
	}
	if tree1.Upstream[0].ShownAbove {
		t.Error("shared in first group should not be ShownAbove")
	}

	if len(tree2.Upstream) != 1 {
		t.Fatalf("tree2.Upstream = %d, want 1", len(tree2.Upstream))
	}
	if !tree2.Upstream[0].ShownAbove {
		t.Error("shared in second group should be ShownAbove")
	}
}

func TestBuildShowTree_ShownBelowForFuturePrimary(t *testing.T) {
	a := entry("20260410-100000-s-stg-aaa", withSummary("A"))
	b := entry("20260410-100100-d-cpt-bbb", withRefs("20260410-100000-s-stg-aaa"))

	g := NewGraph([]*Entry{a, b})
	primaries := map[string]bool{b.ID: true, a.ID: true}

	tree := g.BuildShowTree(b.ID, 4, false, make(map[string]bool), primaries)

	if len(tree.Upstream) != 1 {
		t.Fatalf("Upstream = %d, want 1", len(tree.Upstream))
	}
	if !tree.Upstream[0].ShownBelow {
		t.Error("a should be ShownBelow (it's a future primary)")
	}
}

func TestBuildShowTree_Downstream(t *testing.T) {
	target := entry("20260410-100000-s-stg-aaa")
	child := entry("20260410-100100-d-cpt-bbb", withSummary("Child"),
		withRefs("20260410-100000-s-stg-aaa"))
	grandchild := entry("20260410-100200-a-tac-ccc", withSummary("Grandchild"),
		withRefs("20260410-100100-d-cpt-bbb"))

	g := NewGraph([]*Entry{target, child, grandchild})
	tree := g.BuildShowTree(target.ID, 4, true, make(map[string]bool), make(map[string]bool))

	if len(tree.Downstream) < 2 {
		t.Fatalf("Downstream = %d items, want >= 2", len(tree.Downstream))
	}

	if tree.Downstream[0].Entry.ID != child.ID {
		t.Errorf("Downstream[0].ID = %q, want %q", tree.Downstream[0].Entry.ID, child.ID)
	}
	if tree.Downstream[0].Depth != 1 {
		t.Errorf("Downstream[0].Depth = %d, want 1", tree.Downstream[0].Depth)
	}
	assertRelations(t, tree.Downstream[0], "refd-by")

	if tree.Downstream[1].Entry.ID != grandchild.ID {
		t.Errorf("Downstream[1].ID = %q, want %q", tree.Downstream[1].Entry.ID, grandchild.ID)
	}
	if tree.Downstream[1].Depth != 2 {
		t.Errorf("Downstream[1].Depth = %d, want 2", tree.Downstream[1].Depth)
	}
}

func TestBuildShowTree_DownstreamNotIncludedByDefault(t *testing.T) {
	target := entry("20260410-100000-s-stg-aaa")
	child := entry("20260410-100100-d-cpt-bbb", withSummary("Child"),
		withRefs("20260410-100000-s-stg-aaa"))

	g := NewGraph([]*Entry{target, child})
	tree := g.BuildShowTree(target.ID, 4, false, make(map[string]bool), make(map[string]bool))

	if len(tree.Downstream) != 0 {
		t.Errorf("Downstream = %d, want 0 (downstream not requested)", len(tree.Downstream))
	}
}

func TestBuildShowTree_DownstreamMaxDepth(t *testing.T) {
	target := entry("20260410-100000-s-stg-aaa")
	child := entry("20260410-100100-d-cpt-bbb", withSummary("Child"),
		withRefs("20260410-100000-s-stg-aaa"))
	grandchild := entry("20260410-100200-a-tac-ccc", withSummary("Grandchild"),
		withRefs("20260410-100100-d-cpt-bbb"))

	g := NewGraph([]*Entry{target, child, grandchild})
	tree := g.BuildShowTree(target.ID, 1, true, make(map[string]bool), make(map[string]bool))

	if len(tree.Downstream) != 1 {
		t.Fatalf("Downstream = %d, want 1", len(tree.Downstream))
	}
	if len(tree.Downstream[0].Truncated) != 1 {
		t.Errorf("Truncated = %d, want 1", len(tree.Downstream[0].Truncated))
	}
}

func TestBuildShowTree_DownstreamRelationTypes(t *testing.T) {
	target := entry("20260410-100000-s-tac-aaa")
	refBy := entry("20260410-100100-d-tac-bbb", withSummary("Refs it"),
		withRefs("20260410-100000-s-tac-aaa"))
	closedBy := entry("20260410-100200-a-tac-ccc", withSummary("Closes it"),
		withCloses("20260410-100000-s-tac-aaa"))
	supersededBy := entry("20260410-100300-d-tac-ddd", withSummary("Supersedes it"),
		withSupersedes("20260410-100000-s-tac-aaa"))

	g := NewGraph([]*Entry{target, refBy, closedBy, supersededBy})
	tree := g.BuildShowTree(target.ID, 4, true, make(map[string]bool), make(map[string]bool))

	if len(tree.Downstream) != 3 {
		t.Fatalf("Downstream = %d, want 3", len(tree.Downstream))
	}

	foundRelations := make(map[string]bool)
	for _, item := range tree.Downstream {
		for _, r := range item.Relations {
			foundRelations[r] = true
		}
	}
	for _, want := range []string{"refd-by", "closed-by", "superseded-by"} {
		if !foundRelations[want] {
			t.Errorf("missing relation %q in downstream", want)
		}
	}
}

func TestBuildShowTree_CombinedDownstreamRelations(t *testing.T) {
	target := entry("20260410-100000-s-tac-aaa")
	both := entry("20260410-100100-a-tac-bbb", withSummary("Both"),
		withRefs("20260410-100000-s-tac-aaa"),
		withCloses("20260410-100000-s-tac-aaa"))

	g := NewGraph([]*Entry{target, both})
	tree := g.BuildShowTree(target.ID, 4, true, make(map[string]bool), make(map[string]bool))

	if len(tree.Downstream) != 1 {
		t.Fatalf("Downstream = %d, want 1", len(tree.Downstream))
	}
	assertRelations(t, tree.Downstream[0], "refd-by", "closed-by")
}

func TestBuildShowTree_AllSummaryOnly(t *testing.T) {
	a := entry("20260410-100000-s-stg-aaa", withSummary("Root"))
	b := entry("20260410-100100-s-cpt-bbb", withSummary("Middle"), withRefs("20260410-100000-s-stg-aaa"))
	c := entry("20260410-100200-d-tac-ccc", withRefs("20260410-100100-s-cpt-bbb"))
	downstream := entry("20260410-100300-a-tac-ddd", withSummary("Down"),
		withRefs("20260410-100200-d-tac-ccc"))

	g := NewGraph([]*Entry{a, b, c, downstream})
	tree := g.BuildShowTree(c.ID, 4, true, make(map[string]bool), make(map[string]bool))

	for i, item := range tree.Upstream {
		if !item.SummaryOnly {
			t.Errorf("Upstream[%d].SummaryOnly = false, want true", i)
		}
	}
	for i, item := range tree.Downstream {
		if !item.SummaryOnly {
			t.Errorf("Downstream[%d].SummaryOnly = false, want true", i)
		}
	}
}

func TestBuildShowTree_CyclePrevention(t *testing.T) {
	a := entry("20260410-100000-s-stg-aaa", withSummary("A"), withRefs("20260410-100100-s-cpt-bbb"))
	b := entry("20260410-100100-s-cpt-bbb", withSummary("B"), withRefs("20260410-100000-s-stg-aaa"))

	g := NewGraph([]*Entry{a, b})
	tree := g.BuildShowTree(a.ID, 10, false, make(map[string]bool), make(map[string]bool))

	if len(tree.Upstream) != 2 {
		t.Fatalf("Upstream = %d, want 2", len(tree.Upstream))
	}
	if tree.Upstream[1].Entry.ID != a.ID {
		t.Errorf("Upstream[1].ID = %q, want %q (cycle back)", tree.Upstream[1].Entry.ID, a.ID)
	}
	if !tree.Upstream[1].ShownAbove {
		t.Error("Cycle-back entry should be ShownAbove")
	}
}

func assertRelations(t *testing.T, item ShowTreeItem, expected ...string) {
	t.Helper()
	if len(item.Relations) != len(expected) {
		t.Errorf("Relations = %v, want %v", item.Relations, expected)
		return
	}
	for i, r := range item.Relations {
		if r != expected[i] {
			t.Errorf("Relations[%d] = %q, want %q", i, r, expected[i])
		}
	}
}
