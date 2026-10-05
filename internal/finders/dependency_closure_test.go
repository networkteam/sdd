package finders_test

import (
	"os"
	"path/filepath"
	"slices"
	"testing"

	"github.com/networkteam/sdd/internal/finders"
	"github.com/networkteam/sdd/internal/model"
	"github.com/networkteam/sdd/internal/query"
	"github.com/networkteam/sdd/internal/repos"
	"github.com/networkteam/sdd/internal/repos/repostest"
)

// TestDependencyClosureReadsCachedConfigs walks the closure through the
// committed configs in the connected-repo caches. A dependency that is
// connected but not cached, or cached but not connected, stays in the closure
// with nothing behind it.
func TestDependencyClosureReadsCachedConfigs(t *testing.T) {
	loc := repos.Locations{ConfigPath: filepath.Join(t.TempDir(), "config.yaml"), CacheRoot: t.TempDir()}
	reg := repos.NewRegistry(loc)
	cacheConfig := func(repoID model.RepoID, config string) {
		t.Helper()
		dir, err := reg.CacheDir(repoID)
		if err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(filepath.Join(dir, ".sdd"), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(dir, ".sdd", "config.yaml"), []byte(config), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	connected := &repos.GlobalConfig{}
	for _, repoID := range []model.RepoID{"example.test/a", "example.test/b", "example.test/uncached", "example.test/broken"} {
		if err := connected.AddRepo(repos.ConnectedRepo{RepoID: repoID, CloneURL: "https://" + string(repoID)}); err != nil {
			t.Fatal(err)
		}
	}
	repostest.WriteConfig(t, loc.ConfigPath, connected)
	cacheConfig("example.test/a", "repo_id: example.test/a\ndependencies: [example.test/b, example.test/local]\n")
	cacheConfig("example.test/b", "repo_id: example.test/b\n")
	cacheConfig("example.test/unconnected", "repo_id: example.test/unconnected\ndependencies: [example.test/hidden]\n")
	cacheConfig("example.test/broken", "repo_id: [not, a, string]\n")

	closure := func(dependencies ...model.RepoID) ([]model.RepoID, error) {
		f := finders.New(finders.Options{Config: &model.PerRepoConfig{RepoID: "example.test/local", Dependencies: dependencies}, Repos: reg})
		return f.DependencyClosure(t.Context(), query.DependencyClosureQuery{})
	}

	got, err := closure("example.test/a", "example.test/uncached", "example.test/unconnected")
	if err != nil {
		t.Fatal(err)
	}
	want := []model.RepoID{"example.test/a", "example.test/uncached", "example.test/unconnected", "example.test/b"}
	if !slices.Equal(got, want) {
		t.Errorf("closure = %v, want %v", got, want)
	}

	if _, err := closure("example.test/broken"); err == nil {
		t.Error("a malformed dependency config was swallowed")
	}
}
