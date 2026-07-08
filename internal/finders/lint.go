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
func (f *Finder) Lint(q query.LintQuery) (*query.LintResult, error) {
	if q.Graph == nil {
		return nil, fmt.Errorf("graph is required")
	}

	validateSummaries(q.Graph)

	entries := q.Graph.Lint()
	total := 0
	for _, e := range entries {
		total += len(e.Warnings)
	}
	return &query.LintResult{Entries: entries, TotalIssues: total}, nil
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
		manifest, err := index.LoadManifest(repos.IndexDir(cacheDir))
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
