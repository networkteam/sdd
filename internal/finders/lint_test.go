package finders_test

import (
	"strings"
	"testing"

	"github.com/networkteam/sdd/internal/engine"
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

// findingsFor filters a result's findings down to one entry ID.
func findingsFor(res *query.LintResult, entryID string) []query.LintFinding {
	var out []query.LintFinding
	for _, f := range res.Findings {
		if f.EntryID == entryID {
			out = append(out, f)
		}
	}
	return out
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

	res, err := f.OnGraph(g).Lint(query.LintQuery{})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}

	if got := findingsFor(res, declaredEntry.ID); len(got) != 0 {
		t.Errorf("declared-dependency ref should not warn, got %+v", got)
	}
	got := findingsFor(res, undeclaredEntry.ID)
	if len(got) != 1 || got[0].Category != "graph" || got[0].Severity != query.LintError {
		t.Fatalf("undeclared cross-repo ref should raise 1 graph error, got %+v", got)
	}
	if !strings.Contains(got[0].Message, undeclaredRepo) {
		t.Errorf("finding message should name the repo, got %q", got[0].Message)
	}
}

// TestLint_LoadErrorsCountedAndReported pins that unreadable entries recorded
// on the graph surface as error findings, so the CLI exit code reflects them.
func TestLint_LoadErrorsCountedAndReported(t *testing.T) {
	clean := lintEntry(t, "20260101-120000-s-prc-ok", "A clean summarized entry.")
	g := model.NewGraphWithLoadIssues([]*model.Entry{clean}, []model.LoadIssue{
		{Ref: "20260101-120001-s-prc-bad", Message: "parsing frontmatter: unexpected token"},
	})
	f := finders.New(finders.Options{Config: &model.PerRepoConfig{}})

	res, err := f.OnGraph(g).Lint(query.LintQuery{})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}

	got := findingsFor(res, "20260101-120001-s-prc-bad")
	if len(got) != 1 || got[0].Code != "load-error" || got[0].Severity != query.LintError {
		t.Fatalf("findings = %+v, want one load-error error finding", got)
	}
	if res.Errors() == 0 {
		t.Fatal("Errors() = 0, want the load error counted")
	}
}

// TestLint_MissingSummaryOnly covers the on-demand summary model (d-cpt-4qi):
// lint flags entries with no summary and stays silent on entries that have one.
func TestLint_MissingSummaryOnly(t *testing.T) {
	withSummary := lintEntry(t, "20260101-120000-s-prc-aaa", "An existing summary.")
	missing := lintEntry(t, "20260101-120001-s-prc-bbb", "")

	g := model.NewGraph([]*model.Entry{withSummary, missing})
	f := finders.New(finders.Options{Config: &model.PerRepoConfig{}})

	res, err := f.OnGraph(g).Lint(query.LintQuery{})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}

	if got := findingsFor(res, withSummary.ID); len(got) != 0 {
		t.Errorf("summarized entry should have no findings, got %+v", got)
	}
	got := findingsFor(res, missing.ID)
	if len(got) != 1 || !strings.Contains(got[0].Message, "missing summary") {
		t.Fatalf("missing-summary entry should have 1 finding, got %+v", got)
	}
}

// TestLint_ProcedureRuntimeProvider pins the procedure-runtime provider: a
// graph-resident spec that fails to load is an error, an overshooting one an
// advisory that never flips the exit code, and without a registry the
// provider is skipped.
func TestLint_ProcedureRuntimeProvider(t *testing.T) {
	procEntry := func(id, canonical, machine string) *model.Entry {
		content := "---\ntype: decision\nlayer: prc\nkind: procedure\ncanonical: " + canonical +
			"\nsummary: A procedure.\n" + machine + "\n---\n\n## unit: draft\n\nGuidance.\n"
		e, err := model.ParseEntry(id+".md", content)
		if err != nil {
			t.Fatalf("ParseEntry(%s): %v", id, err)
		}
		return e
	}
	const overBudget = `state:
    report: {type: text, desc: x}
steps:
    - id: draft
      collect: [report]
      inject:
          - {fn: wide, maxBytes: 50000}
      transitions:
          - when: hasBody
            to: end(completed)`
	const broken = `state:
    report: {type: text, desc: x}
steps:
    - id: draft
      collect: [report]
      inject:
          - {fn: unknownQuery}
      transitions:
          - when: hasBody
            to: end(completed)`

	reg := engine.NewRegistry()
	if err := reg.RegisterQuery(engine.Query{
		Doc: engine.FuncDoc{Name: "wide", Doc: "t"},
		Fn:  func(*engine.Context, map[string]any) (any, error) { return "", nil },
	}); err != nil {
		t.Fatal(err)
	}

	big := procEntry("20260101-130000-d-prc-big", "bigproc", overBudget)
	bad := procEntry("20260101-130001-d-prc-bad", "badproc", broken)
	g := model.NewGraph([]*model.Entry{big, bad})

	res, err := finders.New(finders.Options{ProcedureRegistry: reg, Config: &model.PerRepoConfig{}}).OnGraph(g).Lint(query.LintQuery{})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	gotBig := findingsFor(res, big.ID)
	if len(gotBig) != 1 || gotBig[0].Code != "serve-budget" || gotBig[0].Severity != query.LintAdvisory {
		t.Fatalf("over-budget spec findings = %+v, want one serve-budget advisory", gotBig)
	}
	gotBad := findingsFor(res, bad.ID)
	if len(gotBad) != 1 || gotBad[0].Code != "spec-load" || gotBad[0].Severity != query.LintError {
		t.Fatalf("broken spec findings = %+v, want one spec-load error", gotBad)
	}
	if res.Errors() != 1 {
		t.Errorf("Errors() = %d, want the advisory excluded from the exit-flipping count", res.Errors())
	}

	// Without a registry the provider is skipped entirely.
	res, err = finders.New(finders.Options{Config: &model.PerRepoConfig{}}).OnGraph(model.NewGraph([]*model.Entry{big, bad})).Lint(query.LintQuery{})
	if err != nil {
		t.Fatalf("Lint: %v", err)
	}
	for _, f := range res.Findings {
		if f.Category == "procedure-runtime" {
			t.Errorf("provider must be skipped without a registry, got %+v", f)
		}
	}
}
