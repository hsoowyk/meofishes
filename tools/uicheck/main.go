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
	pkg, _ := conf.Check(path, fset, files, nil)
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
