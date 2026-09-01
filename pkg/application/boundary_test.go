package application_test

import (
	"go/types"
	"strings"
	"testing"

	"golang.org/x/tools/go/packages"
)

const internalPrefix = "github.com/networkteam/sdd/internal/"

// allowedInternalRefs are tracked exceptions, keyed by exported identifier.
// Each entry must name the commitment that retires it — an exception without
// a retirement path is a leak with a comment.
var allowedInternalRefs = map[string]bool{
	// Transitional composition helper for the CLI's raw-finder write and lint
	// paths; unexported, and this entry dropped, when those paths move onto
	// the application (d-tac-wuy).
	"github.com/networkteam/sdd/pkg/application.ProcedureRegistry": true,
}

// TestExportedSurfaceNamesNoInternalTypes guards the pkg/ boundary: an
// exported signature naming an internal type is unusable from an external
// module, and the compiler reveals that only at the consumer's site
// (s-tac-ah2). Every exported func, method, var, const, struct field, and
// interface method under pkg/... must resolve to public types only.
func TestExportedSurfaceNamesNoInternalTypes(t *testing.T) {
	cfg := &packages.Config{Mode: packages.NeedName | packages.NeedTypes | packages.NeedImports | packages.NeedDeps}
	pkgs, err := packages.Load(cfg, "github.com/networkteam/sdd/pkg/...")
	if err != nil {
		t.Fatal(err)
	}
	if packages.PrintErrors(pkgs) > 0 {
		t.Fatal("packages loaded with errors")
	}
	if len(pkgs) == 0 {
		t.Fatal("no packages matched pkg/...")
	}
	for _, pkg := range pkgs {
		scope := pkg.Types.Scope()
		for _, name := range scope.Names() {
			obj := scope.Lookup(name)
			if !obj.Exported() || allowedInternalRefs[pkg.PkgPath+"."+name] {
				continue
			}
			w := &internalWalker{seen: map[types.Type]bool{}}
			if tn, ok := obj.(*types.TypeName); ok {
				w.checkTypeName(tn)
			} else {
				w.check(obj.Type())
			}
			for _, hit := range w.hits {
				t.Errorf("%s.%s names internal type %s", pkg.PkgPath, name, hit)
			}
		}
	}
}

type internalWalker struct {
	seen map[types.Type]bool
	hits []string
}

// checkTypeName inspects a package-level type definition: its exported
// fields or interface methods, and its exported method set. An alias is
// checked as the type it names.
func (w *internalWalker) checkTypeName(tn *types.TypeName) {
	if tn.IsAlias() {
		w.check(tn.Type())
		return
	}
	named, ok := tn.Type().(*types.Named)
	if !ok {
		w.check(tn.Type())
		return
	}
	w.checkUnderlying(named.Underlying())
	for i := 0; i < named.NumMethods(); i++ {
		if m := named.Method(i); m.Exported() {
			w.check(m.Type())
		}
	}
}

// checkUnderlying applies the exported-members rule to a definition's own
// shape: unexported fields and methods stay free to use internal types.
func (w *internalWalker) checkUnderlying(u types.Type) {
	switch t := u.(type) {
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			if f := t.Field(i); f.Exported() {
				w.check(f.Type())
			}
		}
	case *types.Interface:
		for i := 0; i < t.NumExplicitMethods(); i++ {
			if m := t.ExplicitMethod(i); m.Exported() {
				w.check(m.Type())
			}
		}
		for i := 0; i < t.NumEmbeddeds(); i++ {
			w.check(t.EmbeddedType(i))
		}
	default:
		w.check(u)
	}
}

// check walks a referenced type. A named type from internal/ is a hit; a
// named type from anywhere else is a boundary of its own (checked at its own
// definition when exported) and is not entered.
func (w *internalWalker) check(typ types.Type) {
	typ = types.Unalias(typ)
	if w.seen[typ] {
		return
	}
	w.seen[typ] = true
	switch t := typ.(type) {
	case *types.Named:
		if obj := t.Obj(); obj.Pkg() != nil && strings.HasPrefix(obj.Pkg().Path(), internalPrefix) {
			w.hits = append(w.hits, obj.Pkg().Path()+"."+obj.Name())
		}
	case *types.Pointer:
		w.check(t.Elem())
	case *types.Slice:
		w.check(t.Elem())
	case *types.Array:
		w.check(t.Elem())
	case *types.Chan:
		w.check(t.Elem())
	case *types.Map:
		w.check(t.Key())
		w.check(t.Elem())
	case *types.Signature:
		for i := 0; i < t.Params().Len(); i++ {
			w.check(t.Params().At(i).Type())
		}
		for i := 0; i < t.Results().Len(); i++ {
			w.check(t.Results().At(i).Type())
		}
	case *types.Struct:
		for i := 0; i < t.NumFields(); i++ {
			w.check(t.Field(i).Type())
		}
	case *types.Interface:
		for i := 0; i < t.NumExplicitMethods(); i++ {
			w.check(t.ExplicitMethod(i).Type())
		}
		for i := 0; i < t.NumEmbeddeds(); i++ {
			w.check(t.EmbeddedType(i))
		}
	}
}
