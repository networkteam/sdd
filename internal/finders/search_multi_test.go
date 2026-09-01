package finders

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/repos"
	"github.com/networkteam/sdd/internal/repos/repostest"
)

// writeEntryFile writes a minimal graph entry under dir's YYYY/MM layout.
func writeEntryFile(t *testing.T, dir, id, body string) {
	t.Helper()
	rel, err := model.IDToRelPath(id)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(dir, filepath.FromSlash(rel))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	content := "---\ntype: signal\nlayer: tac\nkind: gap\n---\n\n" + body + "\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestMultiSearchTextAcrossRepos runs a text-mode cross-graph search over a
// local graph plus one connected repo cache: remote hits merge in
// repo-tagged, and embedded entries surface only from the local side.
func TestMultiSearchTextAcrossRepos(t *testing.T) {
	const repoID = "example.com/team/other"
	loc := repos.Locations{
		ConfigPath: filepath.Join(t.TempDir(), "config.yaml"),
		CacheRoot:  t.TempDir(),
	}
	reg := repos.NewRegistry(loc)

	localDir := t.TempDir()
	writeEntryFile(t, localDir, "20260601-120000-s-tac-loc", "A local gap about telemetry pipelines.")

	cacheDir, err := reg.CacheDir(repoID)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(cacheDir, ".git"), 0o755); err != nil {
		t.Fatal(err)
	}
	remoteGraphDir := filepath.Join(cacheDir, ".sdd/graph")
	writeEntryFile(t, remoteGraphDir, "20260601-130000-s-tac-rem", "A remote gap about telemetry dashboards. telemetry telemetry")

	cfg := &repos.GlobalConfig{}
	if err := cfg.AddRepo(repos.ConnectedRepo{RepoID: repoID, CloneURL: "https://" + repoID}); err != nil {
		t.Fatal(err)
	}
	repostest.WriteConfig(t, loc.ConfigPath, cfg)

	f := New(Options{Repos: reg, Config: &model.PerRepoConfig{}})
	g, err := f.CurrentGraph(localDir)
	if err != nil {
		t.Fatal(err)
	}

	local := NewSearchFinder(SearchFinderOptions{Graph: g, GraphDir: localDir, Repos: reg})
	q := query.SearchQuery{
		Terms:    []string{"telemetry"},
		AllRepos: true,
	}
	res, err := MultiSearch(context.Background(), local, q)
	if err != nil {
		t.Fatal(err)
	}

	byID := map[string]query.SearchEntry{}
	for _, se := range res.Entries {
		byID[se.DisplayID()] = se
	}
	if _, ok := byID["20260601-120000-s-tac-loc"]; !ok {
		t.Errorf("local hit missing: %+v", res.Entries)
	}
	remote, ok := byID[repoID+":20260601-130000-s-tac-rem"]
	if !ok {
		t.Fatalf("remote hit missing or not repo-prefixed: %+v", res.Entries)
	}
	if remote.RepoID != repoID {
		t.Errorf("remote hit RepoID = %q", remote.RepoID)
	}

	// Embedded (binary-scoped) entries surface exactly once even though
	// they load into both graphs: no result may carry a repo-prefixed
	// embedded entry.
	for id, se := range byID {
		if se.Entry.Embedded && se.RepoID != "" {
			t.Errorf("embedded entry surfaced from member graph: %s", id)
		}
	}

	// An explicitly named unconnected repo errors rather than narrowing.
	q.AllRepos = false
	q.Repos = []string{"example.com/team/unknown"}
	if _, err := MultiSearch(context.Background(), local, q); err == nil {
		t.Error("unconnected named repo must error")
	}
}
