package model

import (
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
)

func TestDependencyClosure(t *testing.T) {
	declarations := map[string][]string{
		"a": {"c", "root"},
		"b": {"c", "d", "x"},
		"c": {"a"},
		"d": nil,
		"e": {"f"},
	}
	declared := func(repoID string) ([]string, error) {
		if repoID == "broken" {
			return nil, errors.New("unreadable config")
		}
		// "x" stands for a repo that cannot be resolved: it declares nothing.
		return declarations[repoID], nil
	}

	for _, tt := range []struct {
		name   string
		direct []string
		want   []string
	}{
		{name: "breadth-first, each once, never root", direct: []string{"a", "b"}, want: []string{"a", "b", "c", "d", "x"}},
		{name: "duplicate direct dependencies", direct: []string{"d", "d"}, want: []string{"d"}},
		{name: "no dependencies", direct: nil, want: nil},
		{name: "transitive chain", direct: []string{"e"}, want: []string{"e", "f"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			got, err := DependencyClosure("root", tt.direct, declared)
			if err != nil {
				t.Fatal(err)
			}
			if !slices.Equal(got, tt.want) {
				t.Errorf("closure = %v, want %v", got, tt.want)
			}
		})
	}

	if _, err := DependencyClosure("root", []string{"d", "broken"}, declared); err == nil {
		t.Error("a declaration read error was swallowed")
	}
}

func TestCitedAcross(t *testing.T) {
	const (
		dep   = "example.com/team/dep"
		deep  = "example.com/team/deep"
		other = "example.com/team/other"
		gone  = "example.com/team/gone"
		bad   = "example.com/team/bad"
	)
	base := entry("20260301-090000-s-prc-bas", withEmbedded())
	local := NewGraph([]*Entry{
		entry("20260410-100000-d-tac-loc", withRefs(
			dep+":20260401-100000-d-cpt-aaa",
			dep+":20260301-090000-s-prc-bas",   // a base entry: no repo owns it
			other+":20260401-100000-s-cpt-oth", // outside the scope
			gone+":20260401-100000-s-cpt-gon",  // in scope, not resolvable
			"20260409-100000-s-tac-bar",        // bare: a local entry
		)),
		entry("20260409-100000-s-tac-bar"),
		base,
	})
	depGraph := NewGraph([]*Entry{
		entry("20260401-100000-d-cpt-aaa",
			withRefs("20260331-100000-s-cpt-bbb", deep+":20260201-100000-s-cpt-xxx"),
			withSupersedes("20260330-100000-d-cpt-ccc")),
		entry("20260331-100000-s-cpt-bbb", withCloses("20260329-100000-s-cpt-ddd")),
		entry("20260330-100000-d-cpt-ccc"),
		entry("20260329-100000-s-cpt-ddd"),
		entry("20260328-100000-s-cpt-unc"),
		base,
	})
	deepGraph := NewGraph([]*Entry{entry("20260201-100000-s-cpt-xxx")})
	NewMultiGraph(local, []string{dep}, func(repoID string) (*Graph, error) {
		switch repoID {
		case dep:
			return depGraph, nil
		case deep:
			return deepGraph, nil
		case other:
			return NewGraph([]*Entry{entry("20260401-100000-s-cpt-oth")}), nil
		}
		return nil, nil
	})
	scope := []string{dep, deep, gone}

	flatten := func(selected map[string]map[string]bool) string {
		var out []string
		for _, repoID := range slices.Sorted(maps.Keys(selected)) {
			for _, id := range slices.Sorted(maps.Keys(selected[repoID])) {
				out = append(out, repoID+":"+id)
			}
		}
		return strings.Join(out, " ")
	}
	for _, tt := range []struct {
		name string
		hops int
		want []string
	}{
		{name: "cited only", hops: 0, want: []string{dep + ":20260401-100000-d-cpt-aaa"}},
		{name: "one hop follows refs, supersedes and crossings", hops: 1, want: []string{
			deep + ":20260201-100000-s-cpt-xxx",
			dep + ":20260330-100000-d-cpt-ccc", dep + ":20260331-100000-s-cpt-bbb", dep + ":20260401-100000-d-cpt-aaa",
		}},
		{name: "two hops follow closes", hops: 2, want: []string{
			deep + ":20260201-100000-s-cpt-xxx",
			dep + ":20260329-100000-s-cpt-ddd", dep + ":20260330-100000-d-cpt-ccc", dep + ":20260331-100000-s-cpt-bbb", dep + ":20260401-100000-d-cpt-aaa",
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			selected, err := CitedAcross(local, scope, tt.hops)
			if err != nil {
				t.Fatal(err)
			}
			if got := flatten(selected); got != strings.Join(tt.want, " ") {
				t.Errorf("selected = %s\nwant       %s", got, strings.Join(tt.want, " "))
			}
		})
	}

	t.Run("a member that fails to load is an error", func(t *testing.T) {
		broken := NewGraph([]*Entry{entry("20260410-100000-d-tac-brk", withRefs(bad+":20260401-100000-s-cpt-bad"))})
		NewMultiGraph(broken, []string{bad}, func(string) (*Graph, error) { return nil, errors.New("broken cache") })
		if _, err := CitedAcross(broken, []string{bad}, 1); err == nil {
			t.Error("the load error was swallowed")
		}
	})
}
