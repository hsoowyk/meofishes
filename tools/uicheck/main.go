// Command uicheck type-checks meofishes packages that import miqt WITHOUT
// compiling Qt: miqt is parsed from source with its function bodies dropped
// and cgo faked (go/types FakeImportC). It needs little memory (~300 MB), so
// it is safe to run on small machines where `go build` of miqt would OOM.
//
//	go run ./tools/uicheck ./internal/ui ./cmd/meofishes
package main

import (
	"fmt"
	"go/ast"
	"go/build"
	"go/importer"
	"go/parser"
	"go/token"
	"go/types"
	"os"
	"os/exec"
	"path/filepath"
	"runtime/debug"
	"sort"
	"strings"
)

var (
	fset    = token.NewFileSet()
	cache   = map[string]*types.Package{}
	modRoot string
	modName = "meofishes"
	gc      types.Importer
)

func goList(args ...string) string {
	out, err := exec.Command("go", append([]string{"list", "-e"}, args...)...).Output()
	if err != nil {
		fmt.Fprintln(os.Stderr, "go list", args, err)
		os.Exit(2)
	}
	return strings.TrimSpace(string(out))
}

type imp struct{}

func (imp) Import(path string) (*types.Package, error) { return importPkg(path) }

func importPkg(path string) (*types.Package, error) {
	if p, ok := cache[path]; ok {
		return p, nil
	}
	var dir string
	light := false
	switch {
	case strings.HasPrefix(path, "github.com/mappu/miqt"):
		dir = goList("-f", "{{.Dir}}", path)
		light = true
	case path == modName || strings.HasPrefix(path, modName+"/"):
		dir = filepath.Join(modRoot, strings.TrimPrefix(strings.TrimPrefix(path, modName), "/"))
	default:
		p, err := gc.Import(path)
		if err == nil {
			cache[path] = p
		}
		return p, err
	}
	p, err := checkDir(path, dir, light, false)
	cache[path] = p
	return p, err
}

func checkDir(path, dir string, light, report bool) (*types.Package, error) {
	ctx := build.Default
	ctx.CgoEnabled = true
	ents, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	var files []*ast.File
	for _, e := range ents {
		n := e.Name()
		if !strings.HasSuffix(n, ".go") || strings.HasSuffix(n, "_test.go") {
			continue
		}
		if ok, _ := ctx.MatchFile(dir, n); !ok {
			continue
		}
		mode := parser.SkipObjectResolution
		if !light {
			mode |= parser.ParseComments
		}
		f, err := parser.ParseFile(fset, filepath.Join(dir, n), nil, mode)
		if err != nil {
			return nil, err
		}
		if light {
			for _, d := range f.Decls {
				if fd, ok := d.(*ast.FuncDecl); ok {
					fd.Body = nil
				}
			}
		}
		files = append(files, f)
	}
	var errs []string
	conf := types.Config{Importer: imp{}, FakeImportC: true, IgnoreFuncBodies: light,
		Error: func(err error) {
			if report {
				errs = append(errs, err.Error())
			}
		}}
	if light {
		conf.Error = func(error) {} // C types in miqt signatures are unknowable; ignore
	}
	var info *types.Info
	if !light {
		info = &types.Info{Selections: map[*ast.SelectorExpr]*types.Selection{}, Types: map[ast.Expr]types.TypeAndValue{},
			Defs: map[*ast.Ident]types.Object{}, Uses: map[*ast.Ident]types.Object{}}
	}
	pkg, _ := conf.Check(path, fset, files, info)
	if info != nil && report {
		errs = append(errs, lintOverrides(files, info)...)
	}
	if light {
		debug.FreeOSMemory()
	}
	if report && len(errs) > 0 {
		sort.Strings(errs)
		for _, e := range errs {
			fmt.Println(e)
		}
		return pkg, fmt.Errorf("%d errors", len(errs))
	}
	return pkg, nil
}

func main() {
	modRoot = goList("-m", "-f", "{{.Dir}}")
	gc = importer.ForCompiler(fset, "gc", nil)
	args := os.Args[1:]
	if len(args) == 0 {
		args = []string{"./internal/ui", "./cmd/meofishes"}
	}
	bad := false
	for _, a := range args {
		dir, _ := filepath.Abs(a)
		rel, _ := filepath.Rel(modRoot, dir)
		path := modName
		if rel != "." {
			path += "/" + filepath.ToSlash(rel)
		}
		if _, err := checkDir(path, dir, false, true); err != nil {
			fmt.Fprintf(os.Stderr, "%s: %v\n", path, err)
			bad = true
		} else {
			fmt.Printf("%s: ok\n", path)
		}
	}
	if bad {
		os.Exit(1)
	}
}

// lintOverrides finds virtual-method overrides (OnXxxEvent(func(super …)))
// called through a base class of the object, which miqt rejects at run time
// ("can only override virtual methods for directly constructed types"):
//
//	f := qt.NewQFrame2(); w := f.QWidget; w.OnMousePressEvent(…)   // panics
//	f.QWidget.OnMousePressEvent(…)                                  // panics
//
// The override must be set on the type that was constructed (f.On…).
func lintOverrides(files []*ast.File, info *types.Info) []string {
	const qtPkg = "github.com/mappu/miqt/qt"
	qtNamed := func(t types.Type) *types.Named {
		if p, ok := t.(*types.Pointer); ok {
			t = p.Elem()
		}
		n, ok := t.(*types.Named)
		if ok && n.Obj().Pkg() != nil && n.Obj().Pkg().Path() == qtPkg {
			return n
		}
		return nil
	}
	isOverride := func(fn *types.Func) bool {
		if !strings.HasPrefix(fn.Name(), "On") {
			return false
		}
		sig, ok := fn.Type().(*types.Signature)
		if !ok || sig.Params().Len() != 1 {
			return false
		}
		cb, ok := sig.Params().At(0).Type().(*types.Signature)
		if !ok || cb.Params().Len() == 0 {
			return false
		}
		_, first := cb.Params().At(0).Type().(*types.Signature) // func(super func(…), …)
		return first && cb.Params().At(0).Name() == "super"
	}
	// base reports "X.Base" when e selects the embedded base class Base of a
	// qt type X (f.QWidget), following one level of local variables.
	var defs = map[types.Object]ast.Expr{}
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			if as, ok := n.(*ast.AssignStmt); ok && len(as.Lhs) == len(as.Rhs) {
				for i, l := range as.Lhs {
					if id, ok := l.(*ast.Ident); ok {
						if o := info.Defs[id]; o != nil {
							defs[o] = as.Rhs[i]
						}
					}
				}
			}
			return true
		})
	}
	var base func(e ast.Expr, depth int) string
	base = func(e ast.Expr, depth int) string {
		switch x := e.(type) {
		case *ast.ParenExpr:
			return base(x.X, depth)
		case *ast.Ident:
			if depth > 0 {
				if d, ok := defs[info.Uses[x]]; ok {
					return base(d, depth-1)
				}
			}
		case *ast.SelectorExpr:
			sel := info.Selections[x]
			if sel == nil || sel.Kind() != types.FieldVal {
				return ""
			}
			fv, ok := sel.Obj().(*types.Var)
			if !ok || !fv.Embedded() {
				return ""
			}
			outer, inner := qtNamed(info.Types[x.X].Type), qtNamed(fv.Type())
			if outer != nil && inner != nil && outer.Obj().Name() != inner.Obj().Name() {
				return outer.Obj().Name() + "." + inner.Obj().Name()
			}
		}
		return ""
	}
	var out []string
	for _, f := range files {
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok {
				return true
			}
			se, ok := call.Fun.(*ast.SelectorExpr)
			if !ok {
				return true
			}
			sel := info.Selections[se]
			if sel == nil || sel.Kind() != types.MethodVal {
				return true
			}
			fn, ok := sel.Obj().(*types.Func)
			if !ok || fn.Pkg() == nil || fn.Pkg().Path() != qtPkg || !isOverride(fn) {
				return true
			}
			if b := base(se.X, 2); b != "" {
				out = append(out, fmt.Sprintf("%s: %s is set through the base class %s: miqt panics — call it on the constructed object", fset.Position(se.Sel.Pos()), fn.Name(), b))
			}
			return true
		})
	}
	return out
}
