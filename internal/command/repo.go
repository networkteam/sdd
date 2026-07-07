package command

import "fmt"

// RepoAddCmd registers a connected repo: clone its cache, verify the target
// declares the derived repo_id in its committed config, and record
// {repo_id, clone_url} in the user-global config.
type RepoAddCmd struct {
	// CloneURL is the git URL to clone from (ssh or https — per-user
	// choice; the derived repo_id is identical either way).
	CloneURL string

	// OnAdded fires after the connection is registered, carrying the
	// verified repo identity and the cache location.
	OnAdded func(repoID, cacheDir string)
}

// Validate checks the command's required fields.
func (c *RepoAddCmd) Validate() error {
	if c.CloneURL == "" {
		return fmt.Errorf("clone URL is required")
	}
	return nil
}

// RepoRemoveCmd drops a connected repo from the user-global config. The
// cache directory is left on disk (read-only data; a re-add reuses it).
type RepoRemoveCmd struct {
	RepoID string

	// OnRemoved fires after the connection is dropped.
	OnRemoved func(repoID string)
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
