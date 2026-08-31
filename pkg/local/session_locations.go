package local

import (
	"fmt"
	"path/filepath"
	"strings"

	"github.com/networkteam/sdd/internal/index"
	app "github.com/networkteam/sdd/pkg/application"
)

// StoreLocation is one directory pair sdd has written session material to,
// together with the identity a log found there is attributed to.
type StoreLocation struct {
	// Name identifies the location in diagnostics.
	Name        string
	Sessions    string
	StagedBlobs string
	Subject     string
	Project     app.ProjectID
}

// SessionLocations returns every location sdd has ever written sessions to,
// primary first. Resolution reads across all of them and appends to whichever
// one answered, so nothing has to move and no home policy is needed.
//
// The order is the write preference: the current repository-identity store,
// then the identity-less machine-global store earlier versions keyed by stable
// root hash alone, then the in-tree directory used before the store went
// machine-global.
func SessionLocations(stateRoot, sddDir, repoID, stableRoot string) ([]StoreLocation, error) {
	if !filepath.IsAbs(stateRoot) {
		return nil, fmt.Errorf("sdd: XDG state root must be absolute: %q", stateRoot)
	}
	if sddDir == "" || stableRoot == "" {
		return nil, fmt.Errorf("sdd: sdd directory and stable repository root are required")
	}

	identityless, err := stateLocation(stateRoot, index.RepoKey("", stableRoot), "local", "identity-less machine-global store")
	if err != nil {
		return nil, err
	}
	inTree := StoreLocation{
		Name:        "in-tree store",
		Sessions:    filepath.Join(sddDir, "sessions"),
		StagedBlobs: filepath.Join(sddDir, "staged-blobs"),
		Subject:     "local",
		Project:     "local",
	}
	if repoID == "" {
		return []StoreLocation{identityless, inTree}, nil
	}

	identified, err := stateLocation(stateRoot, index.RepoKey(repoID, stableRoot), app.ProjectID(repoID), "repository-identity store")
	if err != nil {
		return nil, err
	}
	return []StoreLocation{identified, identityless, inTree}, nil
}

func stateLocation(stateRoot, key string, project app.ProjectID, name string) (StoreLocation, error) {
	sessions, err := confinedStatePath(stateRoot, "sessions", key)
	if err != nil {
		return StoreLocation{}, err
	}
	blobs, err := confinedStatePath(stateRoot, "staged-blobs", key)
	if err != nil {
		return StoreLocation{}, err
	}
	return StoreLocation{
		Name: name, Sessions: sessions, StagedBlobs: blobs, Subject: "local", Project: project,
	}, nil
}

// confinedStatePath derives one store directory below a state-root category,
// rejecting any key that would climb out of it.
func confinedStatePath(stateRoot, category, key string) (string, error) {
	if category == "" || key == "" {
		return "", fmt.Errorf("sdd: session store category and repo key are required")
	}
	if filepath.Base(category) != category || strings.ContainsAny(category, `/\`) {
		return "", fmt.Errorf("sdd: invalid session store category %q", category)
	}
	for _, segment := range strings.Split(filepath.ToSlash(key), "/") {
		if segment == "" || segment == "." || segment == ".." {
			return "", fmt.Errorf("sdd: invalid session store repo key %q", key)
		}
	}
	categoryRoot := filepath.Clean(filepath.Join(stateRoot, category))
	target := filepath.Clean(filepath.Join(categoryRoot, filepath.FromSlash(key)))
	relative, err := filepath.Rel(categoryRoot, target)
	if err != nil || !filepath.IsLocal(relative) {
		return "", fmt.Errorf("sdd: session store path escapes %s category: %q", category, key)
	}
	return target, nil
}
