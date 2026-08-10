package finders

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/repos"
	"github.com/networkteam/sdd/internal/repos/repostest"
)

// TestCurrentGraphResolvesConnectedRepo exercises the full seam: a local
// graph whose entry refs a connected repo, the connection registered in the
// user-global config, the remote graph present as a cache clone — all
// resolved through Finder.CurrentGraph's MultiGraph assembly. The registry
// is built over explicit temp locations, the same way the composition root
// injects it in production.
func TestCurrentGraphResolvesConnectedRepo(t *testing.T) {
	const repoID = "example.com/team/other"
	loc := repos.Locations{
		ConfigPath: filepath.Join(t.TempDir(), "config.yaml"),
		CacheRoot:  t.TempDir(),
	}
	reg := repos.NewRegistry(loc)

	// Local graph: one entry with a cross-repo ref.
	localDir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(localDir, "2026/04"), 0o755); err != nil {
		t.Fatal(err)
	}
	local := "---\ntype: decision\nlayer: tac\nkind: directive\nintent: pending\nrefs:\n    - id: " + repoID + ":20260401-090000-d-cpt-rem\n      kind: grounded-in\n---\n\nGrounded in a remote directive.\n"
	if err := os.WriteFile(filepath.Join(localDir, "2026/04/10-100200-d-tac-ccc.md"), []byte(local), 0o644); err != nil {
		t.Fatal(err)
	}

	// Connected-repo cache: a fake clone (a .git marker suffices — the
	// loader only reads the graph files) with one remote entry.
	cacheDir, err := reg.CacheDir(repoID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cacheDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cacheDir, ".sdd/graph/2026/04"), 0o755); err != nil {
		t.Fatal(err)
	}
	remote := "---\ntype: decision\nlayer: cpt\nkind: directive\nintent: guiding\n---\n\nA remote directive.\n"
	if err := os.WriteFile(filepath.Join(cacheDir, ".sdd/graph/2026/04/01-090000-d-cpt-rem.md"), []byte(remote), 0o644); err != nil {
		t.Fatal(err)
	}

	cfg := &repos.GlobalConfig{}
	if err := cfg.AddRepo(repos.ConnectedRepo{RepoID: repoID, CloneURL: "https://" + repoID}); err != nil {
		t.Fatal(err)
	}
	repostest.WriteConfig(t, loc.ConfigPath, cfg)

	f := New(Options{Repos: reg})
	g, err := f.CurrentGraph(localDir)
	if err != nil {
		t.Fatal(err)
	}

	e, owner, ok := g.ResolveAcross(repoID + ":20260401-090000-d-cpt-rem")
	if !ok {
		t.Fatal("connected repo entry did not resolve through CurrentGraph")
	}
	if owner == g {
		t.Error("owner must be the member graph, not the local graph")
	}
	if e.Kind != model.KindDirective {
		t.Errorf("remote entry kind = %q", e.Kind)
	}

	// An unconnected repo stays unresolved.
	if _, _, ok := g.ResolveAcross("example.com/team/unknown:20260401-090000-d-cpt-rem"); ok {
		t.Error("unconnected repo must not resolve")
	}
}
