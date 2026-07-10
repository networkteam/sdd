package command

import "fmt"

// RepoAddCmd connects a repo with two writes: the committed dependency
// declaration in the current repo's .sdd/config.yaml (what this graph
// needs — portable, go.mod-style) and the per-user resolution
// {repo_id, clone_url} in the user-global config (how this machine reaches
// it). Re-running on an existing connection is the upgrade path that adds a
// missing declaration.
type RepoAddCmd struct {
	// CloneURL is the git URL to clone from (ssh or https — per-user
	// choice; the derived repo_id is identical either way).
	CloneURL string

	// OnAdded fires after the connection is registered, carrying the
	// verified repo identity and the cache location.
	OnAdded func(repoID, cacheDir string)

	// OnDeclared fires when the dependency declaration is ensured in the
	// current repo's committed config; alreadyDeclared reports whether it
	// was present before. Not fired outside an sdd repo (global-only
	// registration is legitimate — resolution without a dependent graph).
	OnDeclared func(repoID string, alreadyDeclared bool)
}

// Validate checks the command's required fields.
func (c *RepoAddCmd) Validate() error {
	if c.CloneURL == "" {
		return fmt.Errorf("clone URL is required")
	}
	return nil
}

// RepoRemoveCmd drops a declared cross-repo dependency from the current
// repo's committed .sdd/config.yaml and commits that change. It is
// project-scoped and reference-safety-guarded: because entries are immutable,
// a referenced dependency is permanent, so removal is refused when any entry
// in this graph still holds a cross-repo ref into the target — dropping the
// declaration would strand those refs and retroactively break resolve-or-block.
// The per-user global connection and its on-disk cache are left untouched;
// machine-level teardown is a separate, future command surface.
type RepoRemoveCmd struct {
	RepoID string

	// Force drops the dependency even when local entries still reference the
	// target, stranding those refs. Required to proceed past the ref-safety
	// guard; the stranded refs are named through OnStranded, never silently.
	Force bool

	// OnRemoved fires after the dependency is dropped from the committed
	// config and the change is committed.
	OnRemoved func(repoID string)

	// OnStranded fires when Force drops a still-referenced dependency,
	// carrying the refs the removal stranded so the override stays loud.
	OnStranded func(repoID string, stranded []StrandedRef)
}

// StrandedRef names a local entry whose cross-repo reference into a removed
// dependency is left dangling by a forced `sdd repo remove`.
type StrandedRef struct {
	// EntryID is the local entry that holds the reference.
	EntryID string
	// RefID is the full cross-repo ref it points at (repo-id:entry-id).
	RefID string
	// Kind is the reference kind (e.g. builds-on, grounded-in).
	Kind string
}

// Validate checks the command's required fields.
func (c *RepoRemoveCmd) Validate() error {
	if c.RepoID == "" {
		return fmt.Errorf("repo ID is required")
	}
	return nil
}

// RepoSyncCmd force-pulls connected repo caches: the named repos, or every
// connected repo when RepoIDs is empty. Missing caches clone lazily.
type RepoSyncCmd struct {
	RepoIDs []string

	// OnSynced fires per repo after its cache is fresh.
	OnSynced func(repoID string)
}
