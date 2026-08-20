// Package publicapi 生成并校验 v1 公开 SDK 包的导出符号清单。
package publicapi

import (
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// Inventory 返回公开 SDK 各包按字母序排列的导出符号清单。
func Inventory(dir string) map[string][]string {
	packages := map[string][]string{
		"sdk":        {"sdk"},
		"sdk/pixiv":  {"sdk/pixiv"},
		"sdk/fanbox": {"sdk/fanbox"},
	}
	inventory := map[string][]string{}
	for name, paths := range packages {
		symbols := map[string]bool{}
		for _, p := range paths {
			collectExported(filepath.Join(dir, filepath.FromSlash(p)), symbols)
		}
		names := make([]string, 0, len(symbols))
		for symbol := range symbols {
			names = append(names, symbol)
		}
		sort.Strings(names)
		inventory[name] = names
	}
	return inventory
}

// Render 把 inventory 渲染成 public-api-inventory golden 的稳定文本。
func Render(inventory map[string][]string) string {
	var out strings.Builder
	for _, pkg := range []string{"sdk", "sdk/pixiv", "sdk/fanbox"} {
		fmt.Fprintf(&out, "## %s\n", pkg)
		for _, symbol := range inventory[pkg] {
			fmt.Fprintf(&out, "- %s\n", symbol)
		}
	}
	return out.String()
}

func collectExported(pkgDir string, symbols map[string]bool) {
	entries, err := os.ReadDir(pkgDir)
	if err != nil {
		return
	}
	fset := token.NewFileSet()
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".go") || strings.HasSuffix(entry.Name(), "_test.go") {
			continue
		}
		file, err := parser.ParseFile(fset, filepath.Join(pkgDir, entry.Name()), nil, 0)
		if err != nil {
			continue
		}
		for _, decl := range file.Decls {
			switch node := decl.(type) {
			case *ast.FuncDecl:
				if node.Name.IsExported() && node.Recv == nil {
					symbols[node.Name.Name] = true
				}
			case *ast.GenDecl:
				for _, spec := range node.Specs {
					switch s := spec.(type) {
					case *ast.TypeSpec:
						if s.Name.IsExported() {
							symbols[s.Name.Name] = true
						}
					case *ast.ValueSpec:
						for _, name := range s.Names {
							if name.IsExported() {
								symbols[name.Name] = true
							}
						}
					}
				}
			}
		}
	}
}
