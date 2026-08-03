// Command publicapi 打印 v1 公开 SDK 包的导出符号清单，作为 public API
// inventory golden 的生成与校验工具。
package main

import (
	"flag"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

func main() {
	dir := flag.String("dir", ".", "repository root")
	check := flag.Bool("check", false, "verify the generated inventory matches the golden file")
	golden := flag.String("golden", "", "golden file path to write or compare")
	flag.Parse()

	packages := map[string][]string{
		"sdk":        {"sdk"},
		"sdk/pixiv":  {"sdk", "sdk/pixiv"},
		"sdk/fanbox": {"sdk", "sdk/fanbox"},
	}
	inventory := map[string][]string{}
	for name, paths := range packages {
		symbols := map[string]bool{}
		for _, p := range paths {
			collectExported(filepath.Join(*dir, filepath.FromSlash(p)), symbols)
		}
		names := make([]string, 0, len(symbols))
		for symbol := range symbols {
			names = append(names, symbol)
		}
		sort.Strings(names)
		inventory[name] = names
	}

	var out strings.Builder
	for _, pkg := range []string{"sdk", "sdk/pixiv", "sdk/fanbox"} {
		fmt.Fprintf(&out, "## %s\n", pkg)
		for _, symbol := range inventory[pkg] {
			fmt.Fprintf(&out, "- %s\n", symbol)
		}
	}
	content := out.String()

	if *golden != "" {
		if *check {
			existing, err := os.ReadFile(*golden)
			if err != nil {
				fmt.Fprintln(os.Stderr, "read golden:", err)
				os.Exit(1)
			}
			if string(existing) != content {
				fmt.Fprintln(os.Stderr, "public API inventory drifted from golden")
				os.Exit(1)
			}
			fmt.Println("public API inventory matches golden")
			return
		}
		if err := os.WriteFile(*golden, []byte(content), 0o644); err != nil {
			fmt.Fprintln(os.Stderr, "write golden:", err)
			os.Exit(1)
		}
		fmt.Println("wrote golden inventory")
		return
	}
	fmt.Print(content)
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
