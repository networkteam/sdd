package application_test

import (
	"testing"

	sdd "github.com/networkteam/sdd/pkg/application"
)

func TestPreflightAndCreateEntryAcceptDeclaredCrossRepoRef(t *testing.T) {
	dependency := acquiredRuntime(t, "example.org/dep", staticGraphStore{
		snapshot: acquiredSnapshot(t, "example.org/dep", "r1", "The foreign target entry."),
	})
	f := newWriteFixture(t, writeFixtureOptions{
		Dependencies: []sdd.RepoID{"example.org/dep"}, Dependency: dependency,
	})
	draft := f.identifiedDraft(sdd.EntryDraft{
		Kind: "gap", Layer: "tactical", Confidence: "high",
		Body: "An observation grounded in a foreign entry.",
		Refs: []sdd.EntryRef{{ID: "example.org/dep:20260101-100000-s-tac-aaa", Kind: "grounded-in"}},
	})
	result, err := preflightAndCreateEntry(t, f.app, f.identity, f.binding, draft)
	if err != nil || result.EntryID == "" {
		t.Fatalf("capture = %+v, %v", result, err)
	}
	for _, finding := range result.Findings {
		if finding.Category == "cross-repo-dep-undeclared" || finding.Category == "cross-repo-ref-unresolved" {
			t.Errorf("declared cross-repo ref drew finding %+v", finding)
		}
	}
}
