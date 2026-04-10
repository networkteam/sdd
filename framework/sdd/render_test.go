package sdd

import (
	"bytes"
	"strings"
	"testing"
)

func TestRenderShow_SingleEntryNoRefs(t *testing.T) {
	e := entry("20260410-100000-s-tac-aaa", withContent("A signal about something"))
	g := NewGraph([]*Entry{e})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{"20260410-100000-s-tac-aaa"}, false)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	// Primary gets # heading
	if !strings.Contains(out, "# 20260410-100000-s-tac-aaa\n") {
		t.Errorf("expected # heading for primary entry, got:\n%s", out)
	}
	// Full content present
	if !strings.Contains(out, "A signal about something") {
		t.Errorf("expected entry content, got:\n%s", out)
	}
}

func TestRenderShow_SingleEntryWithRefs(t *testing.T) {
	a := entry("20260410-100000-s-stg-aaa", withContent("Root signal"))
	b := entry("20260410-100100-d-cpt-bbb", withContent("Decision based on root"), withRefs("20260410-100000-s-stg-aaa"))

	g := NewGraph([]*Entry{a, b})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{"20260410-100100-d-cpt-bbb"}, false)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	// Primary at #
	if !strings.Contains(out, "# 20260410-100100-d-cpt-bbb\n") {
		t.Errorf("expected # heading for primary, got:\n%s", out)
	}
	// Ref at ##
	if !strings.Contains(out, "## ref: 20260410-100000-s-stg-aaa\n") {
		t.Errorf("expected ## ref heading, got:\n%s", out)
	}
	// Primary appears before ref
	primaryIdx := strings.Index(out, "# 20260410-100100-d-cpt-bbb")
	refIdx := strings.Index(out, "## ref: 20260410-100000-s-stg-aaa")
	if primaryIdx > refIdx {
		t.Errorf("expected primary before ref, primary at %d, ref at %d", primaryIdx, refIdx)
	}
}

func TestRenderShow_DeepChain(t *testing.T) {
	a := entry("20260410-100000-s-stg-aaa", withContent("Level 2 ref"))
	b := entry("20260410-100100-s-cpt-bbb", withContent("Level 1 ref"), withRefs("20260410-100000-s-stg-aaa"))
	c := entry("20260410-100200-d-tac-ccc", withContent("Primary"), withRefs("20260410-100100-s-cpt-bbb"))

	g := NewGraph([]*Entry{a, b, c})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{"20260410-100200-d-tac-ccc"}, false)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	// Primary at #, direct ref at ##, transitive ref at ###
	if !strings.Contains(out, "# 20260410-100200-d-tac-ccc\n") {
		t.Errorf("expected # for primary, got:\n%s", out)
	}
	if !strings.Contains(out, "## ref: 20260410-100100-s-cpt-bbb\n") {
		t.Errorf("expected ## for direct ref, got:\n%s", out)
	}
	if !strings.Contains(out, "### ref: 20260410-100000-s-stg-aaa\n") {
		t.Errorf("expected ### for transitive ref, got:\n%s", out)
	}
}

func TestRenderShow_MultiEntryDedup(t *testing.T) {
	shared := entry("20260410-100000-s-stg-aaa", withContent("Shared ref"))
	first := entry("20260410-100100-d-cpt-bbb", withContent("First primary"), withRefs("20260410-100000-s-stg-aaa"))
	second := entry("20260410-100200-d-cpt-ccc", withContent("Second primary"), withRefs("20260410-100000-s-stg-aaa"))

	g := NewGraph([]*Entry{shared, first, second})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{
		"20260410-100100-d-cpt-bbb",
		"20260410-100200-d-cpt-ccc",
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	// Both primaries at #
	if !strings.Contains(out, "# 20260410-100100-d-cpt-bbb\n") {
		t.Errorf("expected # for first primary, got:\n%s", out)
	}
	if !strings.Contains(out, "# 20260410-100200-d-cpt-ccc\n") {
		t.Errorf("expected # for second primary, got:\n%s", out)
	}

	// Shared ref appears full under first primary
	firstPrimaryIdx := strings.Index(out, "# 20260410-100100-d-cpt-bbb")
	secondPrimaryIdx := strings.Index(out, "# 20260410-100200-d-cpt-ccc")

	firstSection := out[firstPrimaryIdx:secondPrimaryIdx]
	if !strings.Contains(firstSection, "Shared ref") {
		t.Errorf("expected full content for shared ref under first primary, got:\n%s", firstSection)
	}

	// Under second primary, shared ref should be "(shown above)"
	secondSection := out[secondPrimaryIdx:]
	if !strings.Contains(secondSection, "(shown above)") {
		t.Errorf("expected '(shown above)' for shared ref under second primary, got:\n%s", secondSection)
	}
	if strings.Contains(secondSection, "Shared ref") {
		t.Errorf("expected deduped ref to NOT repeat content, got:\n%s", secondSection)
	}
}

func TestRenderShow_ShownBelowForFuturePrimary(t *testing.T) {
	// B refs A, both are primaries. A appears as ref under B first,
	// but should be "(shown below)" since it's a future primary.
	a := entry("20260410-100000-s-stg-aaa", withContent("Entry A content"))
	b := entry("20260410-100100-d-cpt-bbb", withContent("Entry B content"), withRefs("20260410-100000-s-stg-aaa"))

	g := NewGraph([]*Entry{a, b})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{
		"20260410-100100-d-cpt-bbb",
		"20260410-100000-s-stg-aaa",
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	// Under B's section, A should be "(shown below)" — not full content
	firstPrimaryIdx := strings.Index(out, "# 20260410-100100-d-cpt-bbb")
	separatorIdx := strings.Index(out, "---")
	firstSection := out[firstPrimaryIdx:separatorIdx]

	if !strings.Contains(firstSection, "## ref: 20260410-100000-s-stg-aaa\n") {
		t.Errorf("expected ref heading for A under B, got:\n%s", firstSection)
	}
	if !strings.Contains(firstSection, "(shown below)") {
		t.Errorf("expected '(shown below)' for A under B, got:\n%s", firstSection)
	}
	if strings.Contains(firstSection, "Entry A content") {
		t.Errorf("expected A's content to NOT appear under B, got:\n%s", firstSection)
	}

	// A's own primary section should have full content
	secondSection := out[separatorIdx:]
	if !strings.Contains(secondSection, "# 20260410-100000-s-stg-aaa\n") {
		t.Errorf("expected # heading for A as primary, got:\n%s", secondSection)
	}
	if !strings.Contains(secondSection, "Entry A content") {
		t.Errorf("expected full content for A as primary, got:\n%s", secondSection)
	}
}

func TestRenderShow_ShownBelowSkipsSubtree(t *testing.T) {
	// C refs B refs A. C and A are both primaries. A appears as transitive
	// ref under C but should be "(shown below)" with no subtree traversal.
	a := entry("20260410-100000-s-stg-aaa", withContent("Root"))
	b := entry("20260410-100100-s-cpt-bbb", withContent("Middle"), withRefs("20260410-100000-s-stg-aaa"))
	c := entry("20260410-100200-d-tac-ccc", withContent("Primary C"), withRefs("20260410-100100-s-cpt-bbb"))

	g := NewGraph([]*Entry{a, b, c})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{
		"20260410-100200-d-tac-ccc",
		"20260410-100000-s-stg-aaa",
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	// Under C, B should be full (not a primary), A should be "(shown below)"
	separatorIdx := strings.Index(out, "---")
	firstSection := out[:separatorIdx]

	if !strings.Contains(firstSection, "Middle") {
		t.Errorf("expected B's content under C, got:\n%s", firstSection)
	}
	if !strings.Contains(firstSection, "(shown below)") {
		t.Errorf("expected '(shown below)' for A under C, got:\n%s", firstSection)
	}

	// A should appear full as its own primary
	secondSection := out[separatorIdx:]
	if !strings.Contains(secondSection, "Root") {
		t.Errorf("expected A's full content as primary, got:\n%s", secondSection)
	}
}

func TestRenderShow_FollowsCloses(t *testing.T) {
	signal := entry("20260410-100000-s-tac-aaa", withContent("Original signal"))
	action := entry("20260410-100100-a-tac-bbb", withContent("Action closing signal"),
		withCloses("20260410-100000-s-tac-aaa"))

	g := NewGraph([]*Entry{signal, action})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{"20260410-100100-a-tac-bbb"}, false)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	// The closed signal should appear with "closes" label
	if !strings.Contains(out, "## closes: 20260410-100000-s-tac-aaa\n") {
		t.Errorf("expected ## closes heading, got:\n%s", out)
	}
	if !strings.Contains(out, "Original signal") {
		t.Errorf("expected closed signal content, got:\n%s", out)
	}
}

func TestRenderShow_FollowsSupersedes(t *testing.T) {
	old := entry("20260410-100000-d-tac-aaa", withContent("Old decision"))
	replacement := entry("20260410-100100-d-tac-bbb", withContent("New decision"),
		withSupersedes("20260410-100000-d-tac-aaa"))

	g := NewGraph([]*Entry{old, replacement})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{"20260410-100100-d-tac-bbb"}, false)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	// The superseded entry should appear with "supersedes" label
	if !strings.Contains(out, "## supersedes: 20260410-100000-d-tac-aaa\n") {
		t.Errorf("expected ## supersedes heading, got:\n%s", out)
	}
	if !strings.Contains(out, "Old decision") {
		t.Errorf("expected superseded entry content, got:\n%s", out)
	}
}

func TestRenderShow_MixedReferenceTypes(t *testing.T) {
	// An action that refs a decision, closes a signal, and the decision refs the signal
	signal := entry("20260410-100000-s-tac-aaa", withContent("The signal"))
	decision := entry("20260410-100100-d-tac-bbb", withContent("The decision"),
		withRefs("20260410-100000-s-tac-aaa"))
	action := entry("20260410-100200-a-tac-ccc", withContent("The action"),
		withRefs("20260410-100100-d-tac-bbb"),
		withCloses("20260410-100000-s-tac-aaa"))

	g := NewGraph([]*Entry{signal, decision, action})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{"20260410-100200-a-tac-ccc"}, false)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	// Should have ref for decision and closes for signal
	if !strings.Contains(out, "## ref: 20260410-100100-d-tac-bbb\n") {
		t.Errorf("expected ## ref for decision, got:\n%s", out)
	}
	if !strings.Contains(out, "## closes: 20260410-100000-s-tac-aaa\n") {
		t.Errorf("expected ## closes for signal, got:\n%s", out)
	}
	// Signal appears under both (via decision's ref and action's closes)
	// but only full once — second time is "(shown above)"
	signalCount := strings.Count(out, "The signal")
	if signalCount != 1 {
		t.Errorf("expected signal content exactly once, got %d in:\n%s", signalCount, out)
	}
}

func TestRenderShow_Downstream(t *testing.T) {
	target := entry("20260410-100000-s-stg-aaa", withContent("Target signal"))
	d1 := entry("20260410-100100-d-cpt-bbb", withContent("Decision referencing target"), withRefs("20260410-100000-s-stg-aaa"))
	d2 := entry("20260410-100200-a-tac-ccc", withContent("Action closing target"), withCloses("20260410-100000-s-stg-aaa"))

	g := NewGraph([]*Entry{target, d1, d2})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{"20260410-100000-s-stg-aaa"}, true)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	// Target at #
	if !strings.Contains(out, "# 20260410-100000-s-stg-aaa\n") {
		t.Errorf("expected # for target, got:\n%s", out)
	}
	// Downstream entries at ## with "downstream" label
	if !strings.Contains(out, "## downstream: 20260410-100100-d-cpt-bbb\n") {
		t.Errorf("expected ## downstream for d1, got:\n%s", out)
	}
	if !strings.Contains(out, "## downstream: 20260410-100200-a-tac-ccc\n") {
		t.Errorf("expected ## downstream for d2, got:\n%s", out)
	}
}

func TestRenderShow_EntryNotFound(t *testing.T) {
	g := NewGraph([]*Entry{})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{"20260410-100000-s-stg-xxx"}, false)
	if err == nil {
		t.Fatal("expected error for missing entry")
	}
	if !strings.Contains(err.Error(), "not found") {
		t.Errorf("error = %q, want it to mention not found", err.Error())
	}
}

func TestRenderShow_SeparatorBetweenPrimaries(t *testing.T) {
	a := entry("20260410-100000-s-stg-aaa", withContent("First"))
	b := entry("20260410-100100-s-cpt-bbb", withContent("Second"))

	g := NewGraph([]*Entry{a, b})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{
		"20260410-100000-s-stg-aaa",
		"20260410-100100-s-cpt-bbb",
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	if !strings.Contains(out, "\n---\n") {
		t.Errorf("expected --- separator between primaries, got:\n%s", out)
	}
}

func TestRenderShow_BranchingRefs(t *testing.T) {
	d := entry("20260410-100000-s-stg-ddd", withContent("Shared deep ref"))
	b := entry("20260410-100100-s-cpt-bbb", withContent("Branch B"), withRefs("20260410-100000-s-stg-ddd"))
	c := entry("20260410-100200-s-cpt-ccc", withContent("Branch C"), withRefs("20260410-100000-s-stg-ddd"))
	a := entry("20260410-100300-d-tac-aaa", withContent("Primary with two branches"),
		withRefs("20260410-100100-s-cpt-bbb", "20260410-100200-s-cpt-ccc"))

	g := NewGraph([]*Entry{d, b, c, a})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{"20260410-100300-d-tac-aaa"}, false)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	if !strings.Contains(out, "# 20260410-100300-d-tac-aaa\n") {
		t.Errorf("expected # for primary, got:\n%s", out)
	}
	if !strings.Contains(out, "## ref: 20260410-100100-s-cpt-bbb\n") {
		t.Errorf("expected ## for branch B, got:\n%s", out)
	}
	if !strings.Contains(out, "## ref: 20260410-100200-s-cpt-ccc\n") {
		t.Errorf("expected ## for branch C, got:\n%s", out)
	}

	dFullCount := strings.Count(out, "Shared deep ref")
	if dFullCount != 1 {
		t.Errorf("expected D content to appear exactly once, got %d times in:\n%s", dFullCount, out)
	}
	if !strings.Contains(out, "(shown above)") {
		t.Errorf("expected '(shown above)' for D's second appearance, got:\n%s", out)
	}
}

func TestRenderShow_BlankLineAfterHeading(t *testing.T) {
	e := entry("20260410-100000-s-tac-aaa", withContent("Content"))
	g := NewGraph([]*Entry{e})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{"20260410-100000-s-tac-aaa"}, false)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	if !strings.Contains(out, "# 20260410-100000-s-tac-aaa\n\n") {
		t.Errorf("expected blank line after heading, got:\n%q", out)
	}
}

func TestRenderShow_ShownAboveSkipsSubtree(t *testing.T) {
	root := entry("20260410-100000-s-stg-aaa", withContent("Deep root"))
	mid := entry("20260410-100100-s-cpt-bbb", withContent("Middle"), withRefs("20260410-100000-s-stg-aaa"))
	first := entry("20260410-100200-d-tac-ccc", withContent("First primary"), withRefs("20260410-100100-s-cpt-bbb"))
	second := entry("20260410-100300-d-tac-ddd", withContent("Second primary"), withRefs("20260410-100100-s-cpt-bbb"))

	g := NewGraph([]*Entry{root, mid, first, second})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{
		"20260410-100200-d-tac-ccc",
		"20260410-100300-d-tac-ddd",
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	secondPrimaryIdx := strings.Index(out, "# 20260410-100300-d-tac-ddd")
	secondSection := out[secondPrimaryIdx:]

	if !strings.Contains(secondSection, "(shown above)") {
		t.Errorf("expected '(shown above)' for mid under second primary, got:\n%s", secondSection)
	}
	if strings.Contains(secondSection, "20260410-100000-s-stg-aaa") {
		t.Errorf("expected subtree of '(shown above)' entry to be skipped, got:\n%s", secondSection)
	}
}

func TestRenderShow_ShownAboveHasBlankLineAfterHeading(t *testing.T) {
	a := entry("20260410-100000-s-stg-aaa", withContent("Shared"))
	b := entry("20260410-100100-d-cpt-bbb", withContent("First"), withRefs("20260410-100000-s-stg-aaa"))
	c := entry("20260410-100200-d-cpt-ccc", withContent("Second"), withRefs("20260410-100000-s-stg-aaa"))

	g := NewGraph([]*Entry{a, b, c})

	var buf bytes.Buffer
	err := RenderShow(&buf, g, []string{
		"20260410-100100-d-cpt-bbb",
		"20260410-100200-d-cpt-ccc",
	}, false)
	if err != nil {
		t.Fatal(err)
	}

	out := buf.String()

	secondSection := out[strings.Index(out, "---"):]
	refHeading := "## ref: 20260410-100000-s-stg-aaa\n\n(shown above)"
	if !strings.Contains(secondSection, refHeading) {
		t.Errorf("expected blank line between heading and '(shown above)', got:\n%q", secondSection)
	}
}
