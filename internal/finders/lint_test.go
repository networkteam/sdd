package finders_test

import (
	"testing"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
)

// lintEntry builds a minimal parsed entry that either carries a summary or not.
func lintEntry(t *testing.T, id, summary string) *model.Entry {
	t.Helper()
	fm := "---\ntype: signal\nlayer: process\nkind: gap\n"
	if summary != "" {
		fm += "summary: " + summary + "\n"
	}
	fm += "---\n\nBody.\n"
	e, err := model.ParseEntry(id+".md", fm)
	if err != nil {
		t.Fatalf("ParseEntry(%s): %v", id, err)
	}
	return e
}

// lintRefEntry builds a parsed entry carrying a single cross-repo ref.
func lintRefEntry(t *testing.T, id, refID string) *model.Entry {
	t.Helper()
	fm := "---\ntype: signal\nlayer: process\nkind: gap\n" +
		"summary: Has a cross-repo ref.\n" +
		"refs:\n  - id: " + refID + "\n    kind: related\n" +
		"---\n\nBody.\n"
	e, err := model.ParseEntry(id+".md", fm)
	if err != nil {
		t.Fatalf("ParseEntry(%s): %v", id, err)
	}
	return e
}

// TestLint_CrossRepoRefUndeclared flags a ref into a repo missing from the
// declared dependencies, and stays silent when the dependency is declared —
// the standing counterpart to capture-time resolve-or-block.
func TestLint_CrossRepoRefUndeclared(t *testing.T) {
	const declaredRepo = "github.com/networkteam/declared"
	const undeclaredRepo = "github.com/networkteam/undeclared"

	declaredEntry := lintRefEntry(t, "20260101-120000-s-prc-aaa", declaredRepo+":20260101-100000-s-tac-xxx")
	undeclaredEntry := lintRefEntry(t, "20260101-120001-s-prc-bbb", undeclaredRepo+":20260101-100000-s-tac-yyy")

	g := model.NewGraph([]*model.Entry{declaredEntry, undeclaredEntry})
	f := finders.New(finders.Options{Config: &model.PerRepoConfig{Dependencies: []string{declaredRepo}}})

	res, err := f.Lint(query.LintQuery{Graph: g})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}

	warnings := map[string][]model.Warning{}
	for _, e := range res.Entries {
		warnings[e.ID] = e.Warnings
	}

	// The ref into the declared dependency raises no cross-repo warning.
	for _, w := range warnings[declaredEntry.ID] {
		if w.Field == "refs" {
			t.Errorf("declared-dependency ref should not warn, got %+v", w)
		}
	}

	// The ref into the undeclared repo raises exactly one refs warning naming it.
	var refWarnings []model.Warning
	for _, w := range warnings[undeclaredEntry.ID] {
		if w.Field == "refs" {
			refWarnings = append(refWarnings, w)
		}
	}
	if len(refWarnings) != 1 {
		t.Fatalf("undeclared cross-repo ref should raise 1 refs warning, got %d: %+v", len(refWarnings), refWarnings)
	}
	if refWarnings[0].Value != undeclaredRepo {
		t.Errorf("warning value = %q, want %q", refWarnings[0].Value, undeclaredRepo)
	}
}

// TestLint_MissingSummaryOnly covers the on-demand summary model (d-cpt-4qi):
// lint flags entries with no summary and stays silent on entries that have one.
// There is no hash check, so no "stale summary hash" or "summary exists but no
// hash" warning is ever produced.
func TestLint_MissingSummaryOnly(t *testing.T) {
	withSummary := lintEntry(t, "20260101-120000-s-prc-aaa", "An existing summary.")
	missing := lintEntry(t, "20260101-120001-s-prc-bbb", "")

	g := model.NewGraph([]*model.Entry{withSummary, missing})
	f := finders.New(finders.Options{})

	res, err := f.Lint(query.LintQuery{Graph: g})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}

	warnings := map[string][]model.Warning{}
	for _, e := range res.Entries {
		warnings[e.ID] = e.Warnings
	}

	// The summarized entry carries no warnings.
	if w := warnings[withSummary.ID]; len(w) != 0 {
		t.Errorf("summarized entry should have no warnings, got %+v", w)
	}

	// The missing-summary entry carries exactly one "missing summary" warning.
	mw := warnings[missing.ID]
	if len(mw) != 1 {
		t.Fatalf("missing-summary entry should have 1 warning, got %d: %+v", len(mw), mw)
	}
	if mw[0].Field != "summary" {
		t.Errorf("warning field = %q, want %q", mw[0].Field, "summary")
	}

	// No summary_hash warnings anywhere — the hash check is gone.
	for _, e := range res.Entries {
		for _, w := range e.Warnings {
			if w.Field == "summary_hash" {
				t.Errorf("unexpected summary_hash warning on %s: %+v", e.ID, w)
			}
		}
	}
}
