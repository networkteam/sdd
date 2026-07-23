package finders

import (
	"fmt"

	"github.com/networkteam/sdd/internal/index"
	"github.com/networkteam/sdd/internal/llm/embed"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/repos"
)

// Lint returns every entry in the graph that has at least one warning,
// alongside the total warning count. Pure read — graph validation runs at
// graph-construction time, this just collects the results. Also flags entries
// that are missing a summary.
func (gf *GraphFinder) Lint(_ query.LintQuery) (*query.LintResult, error) {
	if gf.graph == nil {
		return nil, fmt.Errorf("graph is required")
	}

	validateSummaries(gf.graph)
	gf.finder.validateCrossRepoDeps(gf.graph)

	entries := gf.graph.Lint()
	total := len(gf.graph.LoadIssues)
	for _, e := range entries {
		total += len(e.Warnings)
	}
	return &query.LintResult{Entries: entries, TotalIssues: total, LoadErrors: gf.graph.LoadIssues}, nil
}

// IndexLint fills the index-side fields on a LintResult when an embedding
// provider is configured. Loading the manifest and building an embedder are
// both pure operations against config — no graph mutation, no embedding
// calls (only the fingerprint is read). Errors degrade silently to "index
// not configured" so a missing dependency in lint doesn't block graph-side
// validation.
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
	result.IndexConfigured = true
	result.IndexFingerprint = emb.Fingerprint()
	result.IndexEntryCount = len(manifest.Entries)
	result.IndexDriftCount = manifest.MismatchCount(emb.Fingerprint())

	f.repoIndexLint(result)
}

// repoIndexLint reports connected repos whose cache index was built under a
// fingerprint other than the shared (global) embedder — a drifted repo
// re-embeds on the next cross-graph search, and lint makes that pending
// cost visible. Degrades silently like the local index section: lint never
// blocks on cross-repo machinery being absent (including a Finder built
// without the Registry).
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
			result.RepoIndexDrift = append(result.RepoIndexDrift, query.RepoIndexDriftInfo{
				RepoID:     r.RepoID,
				DriftCount: drift,
				EntryCount: len(manifest.Entries),
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
func (f *Finder) validateCrossRepoDeps(graph *model.Graph) {
	declared := make(map[string]bool, len(f.declaredDependencies()))
	for _, dep := range f.declaredDependencies() {
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
