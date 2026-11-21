// Command i18nkeys extracts every translatable string of the interface
// (the English text passed to tr() and plural(), plus the Shortcuts and
// devTabs tables) into assets/locales/keys.json, and reports, for each
// language file, how many strings are missing.
//
//	go run ./tools/i18nkeys            write keys.json and print coverage
//	go run ./tools/i18nkeys -missing pt-BR   list the strings pt-BR lacks
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

func main() {
	missing := flag.String("missing", "", "print the keys missing in this language")
	flag.Parse()
	keys := map[string]bool{}
	fset := token.NewFileSet()
	files, _ := filepath.Glob("internal/ui/*.go")
	for _, f := range files {
		file, err := parser.ParseFile(fset, f, nil, 0)
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		ast.Inspect(file, func(n ast.Node) bool {
			switch x := n.(type) {
			case *ast.CallExpr:
				id, ok := x.Fun.(*ast.Ident)
				if !ok {
					return true
				}
				var args []ast.Expr
				switch id.Name {
				case "tr":
					args = x.Args[:1]
				case "plural":
					if len(x.Args) == 3 {
						args = x.Args[1:]
					}
				}
				for _, a := range args {
					if s, ok := lit(a); ok {
						keys[s] = true
					}
				}
			case *ast.ValueSpec:
				for _, name := range x.Names {
					if name.Name != "Shortcuts" && name.Name != "devTabs" {
						continue
					}
					ast.Inspect(x, func(n ast.Node) bool {
						cl, ok := n.(*ast.CompositeLit)
						if !ok || len(cl.Elts) != 2 {
							return true
						}
						if s, ok := lit(cl.Elts[1]); ok {
							keys[s] = true
						} else if kv, ok := cl.Elts[1].(*ast.KeyValueExpr); ok {
							if s, ok := lit(kv.Value); ok {
								keys[s] = true
							}
						}
						return true
					})
				}
			}
			return true
		})
	}
	var list []string
	for k := range keys {
		list = append(list, k)
	}
	sort.Strings(list)
	data, _ := json.MarshalIndent(list, "", " ")
	_ = os.WriteFile("assets/locales/keys.json", append(data, '\n'), 0o644)
	fmt.Printf("%d strings → assets/locales/keys.json\n", len(list))

	langs, _ := filepath.Glob("assets/locales/*.json")
	for _, f := range langs {
		var lf struct {
			Code    string            `json:"code"`
			Strings map[string]string `json:"strings"`
		}
		data, _ := os.ReadFile(f)
		if json.Unmarshal(data, &lf) != nil || lf.Code == "" {
			continue
		}
		var miss []string
		for _, k := range list {
			if strings.TrimSpace(lf.Strings[k]) == "" {
				miss = append(miss, k)
			}
		}
		fmt.Printf("%-6s %3d missing\n", lf.Code, len(miss))
		if lf.Code == *missing {
			for _, m := range miss {
				b, _ := json.Marshal(m)
				fmt.Println("  " + string(b))
			}
		}
	}
}

func lit(e ast.Expr) (string, bool) {
	bl, ok := e.(*ast.BasicLit)
	if !ok || bl.Kind != token.STRING {
		return "", false
	}
	s, err := strconv.Unquote(bl.Value)
	return s, err == nil
}
