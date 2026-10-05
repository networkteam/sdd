package model

import (
	"context"
	"errors"
	"maps"
	"slices"
	"strings"
	"testing"
)

// declarations resolves each key to the dependencies it declares, recording
// the keys it was asked about; "x" stands for a dependency that cannot be
// resolved, so it declares nothing.
type declarations struct {
	declared map[RepoID][]RepoID
	ctx      context.Context
	asked    []RepoID
}

func (d *declarations) Dependencies(ctx context.Context, k RepoID) ([]RepoID, error) {
	if ctx != d.ctx {
		return nil, errors.New("the walk's context did not reach the resolver")
	}
	d.asked = append(d.asked, k)
	if k == "broken" {
		return nil, errors.New("unreadable config")
	}
	return d.declared[k], nil
}

func TestDependencyClosure(t *testing.T) {
	newResolver := func(ctx context.Context, direct ...RepoID) *declarations {
		return &declarations{ctx: ctx, declared: map[RepoID][]RepoID{
			"root": direct,
			"a":    {"c", "root"},
			"b":    {"c", "d", "x"},
			"c":    {"a"},
			"d":    nil,
			"e":    {"f"},
		}}
	}
	collect := func(t *testing.T, ctx context.Context, r *declarations) []RepoID {
		t.Helper()
		var got []RepoID
		for k, err := range DependencyClosure(ctx, RepoID("root"), r) {
			if err != nil {
				t.Fatal(err)
			}
			got = append(got, k)
		}
		return got
	}

	for _, tt := range []struct {
		name   string
		direct []RepoID
		want   []RepoID
	}{
		{name: "breadth-first, each once, never root", direct: []RepoID{"a", "b"}, want: []RepoID{"a", "b", "c", "d", "x"}},
		{name: "duplicate direct dependencies", direct: []RepoID{"d", "d"}, want: []RepoID{"d"}},
		{name: "no dependencies", direct: nil, want: nil},
		{name: "transitive chain", direct: []RepoID{"e"}, want: []RepoID{"e", "f"}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if got := collect(t, t.Context(), newResolver(t.Context(), tt.direct...)); !slices.Equal(got, tt.want) {
				t.Errorf("closure = %v, want %v", got, tt.want)
			}
		})
	}

	t.Run("a resolver error ends the walk", func(t *testing.T) {
		var got []RepoID
		var errs int
		for k, err := range DependencyClosure(t.Context(), RepoID("root"), newResolver(t.Context(), "broken", "e")) {
			if err != nil {
				errs++
				continue
			}
			got = append(got, k)
		}
		if errs != 1 || !slices.Equal(got, []RepoID{"broken", "e"}) {
			t.Errorf("yielded %v and %d errors, want [broken e] and one error, nothing behind it", got, errs)
		}
	})

	t.Run("breaking out stops the walk", func(t *testing.T) {
		r := newResolver(t.Context(), "a", "b")
		for k := range DependencyClosure(t.Context(), RepoID("root"), r) {
			if k == "b" {
				break
			}
		}
		if !slices.Equal(r.asked, []RepoID{"root"}) {
			t.Errorf("resolver asked about %v after the break, want only [root]", r.asked)
		}
	})
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
	NewMultiGraph(local, []RepoID{dep}, func(repoID RepoID) (*Graph, error) {
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
	scope := []RepoID{dep, deep, gone}

	flatten := func(selected map[RepoID]map[string]bool) string {
		var out []string
		for _, repoID := range slices.Sorted(maps.Keys(selected)) {
			for _, id := range slices.Sorted(maps.Keys(selected[repoID])) {
				out = append(out, CrossRepoID(repoID, id))
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
		NewMultiGraph(broken, []RepoID{bad}, func(RepoID) (*Graph, error) { return nil, errors.New("broken cache") })
		if _, err := CitedAcross(broken, []RepoID{bad}, 1); err == nil {
			t.Error("the load error was swallowed")
		}
	})
}
