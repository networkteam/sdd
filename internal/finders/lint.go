package finders

import (
	"fmt"

	"github.com/networkteam/sdd/internal/engine"
	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/llm/embed"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/repos"
	"github.com/networkteam/sdd/internal/serveview"
)

// Lint runs the categorized graph-side lint providers (d-cpt-xc3): graph
// integrity (load errors, entry warnings, missing summaries) as error
// findings, and the procedure runtime (spec load, then the serve-budget
// arithmetic) — a spec that fails to load is an error, an overshooting one an
// advisory, because it still runs (d-tac-rzi). Index findings are filled in
// separately by IndexLint, whose inputs the shell resolves.
func (gf *GraphFinder) Lint(_ query.LintQuery) (*query.LintResult, error) {
	if gf.graph == nil {
		return nil, fmt.Errorf("graph is required")
	}

	validateSummaries(gf.graph)
	if err := gf.finder.validateCrossRepoDeps(gf.graph); err != nil {
		return nil, fmt.Errorf("lint: %w", err)
	}

	result := &query.LintResult{}
	for _, issue := range gf.graph.LoadIssues {
		result.Findings = append(result.Findings, query.LintFinding{
			Category: "graph", Code: "load-error", Severity: query.LintError,
			EntryID: issue.Ref, Message: issue.Message,
		})
	}
	for _, e := range gf.graph.Lint() {
		for _, warning := range e.Warnings {
			result.Findings = append(result.Findings, query.LintFinding{
				Category: "graph", Code: "warning", Severity: query.LintError,
				EntryID: e.ID, Message: warning.Message,
			})
		}
	}
	result.Findings = append(result.Findings, gf.procedureRuntimeFindings()...)
	return result, nil
}

// procedureRuntimeFindings is the procedure-runtime lint provider: every
// procedure entry in the graph loads against the registry first — lint never
// parsed specs before, so a broken graph-resident spec surfaced only when the
// engine served it — then the loaded spec runs the worst-case authoring
// arithmetic. Skipped without a registry (read-only shells that cannot
// compose one).
func (gf *GraphFinder) procedureRuntimeFindings() []query.LintFinding {
	registry := gf.finder.procedureRegistry
	if registry == nil {
		return nil
	}
	var findings []query.LintFinding
	budget := serveview.Default()
	for _, entry := range gf.graph.Entries {
		if !entry.IsProcedure() || len(gf.graph.SupersededBy[entry.ID]) > 0 {
			continue
		}
		spec, err := engine.LoadSpec(entry, registry)
		if err != nil {
			findings = append(findings, query.LintFinding{
				Category: "procedure-runtime", Code: "spec-load", Severity: query.LintError,
				EntryID: entry.ID, Message: err.Error(),
			})
			continue
		}
		for _, size := range spec.OverBudget(budget, registry) {
			findings = append(findings, query.LintFinding{
				Category: "procedure-runtime", Code: "serve-budget", Severity: query.LintAdvisory,
				EntryID: entry.ID,
				Message: fmt.Sprintf("step %q sizes to a worst-case %d bytes against the %d-byte serve budget — tighten caps, or declare `serveBudget: %d` on the spec to record the trade",
					size.Step, size.Bytes, spec.EffectiveTotal(budget), size.Bytes),
			})
		}
	}
	return findings
}

// IndexLint appends index-health findings when an embedding provider is
// configured. Loading the manifest and building an embedder are both pure
// operations against config — no graph mutation, no embedding calls (only
// the fingerprint is read). Errors degrade silently to "index not
// configured" so a missing dependency in lint doesn't block graph-side
// validation. Drift is advisory: the next index run or search converges it.
func (f *Finder) IndexLint(q query.IndexLintQuery, result *query.LintResult) {
	if q.Embedding.Provider == "" || q.IndexDir == "" {
		return
	}
	emb, err := embed.New(q.Embedding)
	if err != nil {
		return
	}
	manifest, err := index.LoadManifest(q.IndexDir)
	if err != nil {
		return
	}
	if drift := manifest.MismatchCount(emb.Fingerprint()); drift > 0 {
		result.Findings = append(result.Findings, query.LintFinding{
			Category: "index", Code: "fingerprint-drift", Severity: query.LintAdvisory,
			Message: fmt.Sprintf("%d of %d indexed entries carry a different fingerprint than the configured embedder (%s) — run `sdd index --force` to re-embed (or let `sdd search` lazy-fill)",
				drift, len(manifest.Entries), emb.Fingerprint()),
		})
	}

	f.repoIndexLint(result)
}

// repoIndexLint reports connected repos whose cache index was built under a
// fingerprint other than the shared (global) embedder — a drifted repo
// re-embeds on the next cross-graph search; lint surfaces it so the cost
// isn't a surprise.
func (f *Finder) repoIndexLint(result *query.LintResult) {
	if f.repos == nil {
		return
	}
	gcfg, err := f.repos.Load()
	if err != nil || len(gcfg.Repos) == 0 || gcfg.Embedding.Provider == "" {
		return
	}
	emb, err := embed.New(gcfg.Embedding)
	if err != nil {
		return
	}
	fingerprint := emb.Fingerprint()
	for _, r := range gcfg.Repos {
		cacheDir, err := f.repos.CacheDir(r.RepoID)
		if err != nil || !repos.IsCloned(cacheDir) {
			continue
		}
		manifest, err := index.LoadManifest(index.StoreDir(f.repos.CacheRoot(), r.RepoID, fingerprint))
		if err != nil || len(manifest.Entries) == 0 {
			continue
		}
		if drift := manifest.MismatchCount(fingerprint); drift > 0 {
			result.Findings = append(result.Findings, query.LintFinding{
				Category: "index", Code: "repo-fingerprint-drift", Severity: query.LintAdvisory,
				EntryID: r.RepoID,
				Message: fmt.Sprintf("%d of %d cached entries indexed under a different fingerprint than the global embedder — the next cross-graph search re-embeds them",
					drift, len(manifest.Entries)),
			})
		}
	}
}

// validateCrossRepoDeps flags every local entry holding a cross-repo ref
// whose target repo-id is not in the declared dependencies. This extends the
// resolve-or-block invariant from a capture-time gate into a standing check:
// it catches post-capture violations a fresh capture never would — a
// dependency dropped from .sdd/config.yaml while an entry still references it,
// or a hand-edited ref prefix. Distinct undeclared repo-ids are reported once
// per entry so the finding points at the exact entry and repo. Embedded base
// entries are skipped: they are framework-shipped and never reference a
// project's declared dependencies.
func (f *Finder) validateCrossRepoDeps(graph *model.Graph) error {
	deps, err := f.declaredDependencies()
	if err != nil {
		return err
	}
	declared := make(map[string]bool, len(deps))
	for _, dep := range deps {
		declared[dep] = true
	}
	for _, entry := range graph.Entries {
		if entry.Embedded {
			continue
		}
		seen := map[string]bool{}
		for _, r := range entry.Refs {
			repoID, _, ok := model.SplitCrossRepoID(r.ID)
			if !ok || declared[repoID] || seen[repoID] {
				continue
			}
			seen[repoID] = true
			entry.Warnings = append(entry.Warnings, model.Warning{
				Field:   "refs",
				Value:   repoID,
				Message: fmt.Sprintf("cross-repo ref into %q, which is not a declared dependency in .sdd/config.yaml — declare it with `sdd repo add`, or the ref is stranded", repoID),
			})
		}
	}
	return nil
}

// validateSummaries flags entries that have no summary yet. Under the
// on-demand summary model (d-cpt-4qi) summaries carry no staleness tracking —
// an entry's meaning is fixed at creation, so there is nothing to detect drift
// against; lint only surfaces the absence of a summary.
func validateSummaries(graph *model.Graph) {
	for _, entry := range graph.Entries {
		// Embedded base entries ship their summary with the binary — there
		// is no file to regenerate.
		if entry.Embedded {
			continue
		}
		if entry.Summary == "" {
			entry.Warnings = append(entry.Warnings, model.Warning{
				Field:   "summary",
				Message: "missing summary (run sdd summarize to generate)",
			})
		}
	}
}
