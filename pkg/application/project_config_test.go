package application_test

import (
	"errors"
	"reflect"
	"testing"
	"testing/fstest"

	sdd "github.com/networkteam/sdd/pkg/application"
)

func TestReadProjectConfigFS(t *testing.T) {
	full := fstest.MapFS{".sdd/config.yaml": {Data: []byte(`repo_id: example.test/a
dependencies:
  - example.test/b
  - example.test/c
default_branch: main
language: de
graph_dir: graph
participant: alice
unknown_key: kept-by-the-parser
`)}}
	got, err := sdd.ReadProjectConfigFS(full)
	if err != nil {
		t.Fatal(err)
	}
	want := sdd.ProjectConfig{
		RepoID: "example.test/a", Dependencies: []string{"example.test/b", "example.test/c"},
		DefaultBranch: "main", Language: "de", GraphDir: "graph",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("config = %+v, want %+v", got, want)
	}

	minimal := fstest.MapFS{".sdd/config.yaml": {Data: []byte("repo_id: example.test/a\n")}}
	got, err = sdd.ReadProjectConfigFS(minimal)
	if err != nil || got.GraphDir != ".sdd/graph" || got.Dependencies != nil {
		t.Fatalf("minimal config = %+v, %v; want the graph dir defaulted", got, err)
	}

	if _, err := sdd.ReadProjectConfigFS(fstest.MapFS{"README.md": {Data: []byte("x")}}); !errors.Is(err, sdd.ErrNotAnSDDProject) {
		t.Fatalf("no config = %v, want ErrNotAnSDDProject", err)
	}
	if _, err := sdd.ReadProjectConfigFS(fstest.MapFS{".sdd/config.yaml": {Data: []byte("repo_id: [\n")}}); err == nil || errors.Is(err, sdd.ErrNotAnSDDProject) {
		t.Fatalf("malformed config = %v, want a parse error", err)
	}
}
